import Foundation
import Testing
@testable import Allyourbase

struct WebSocketRealtimeClientTests {
    @Test
    func connectWebSocketUsesRealtimePathAndTokenQuery() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])

        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_ws", refreshToken: "refresh")
        )
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }

        try await realtime.connectWebSocket()

        let url = try #require(wsTransport.connectURLs.first)
        #expect(url.path == "/api/realtime/ws")
        let components = try #require(URLComponents(url: url, resolvingAgainstBaseURL: false))
        #expect(components.queryItems?.first(where: { $0.name == "token" })?.value == "jwt_ws")
    }

    @Test
    func connectWebSocketPreservesBasePathPrefix() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])

        let client = AYBClient("https://api.example.com/proxy/base")
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }

        try await realtime.connectWebSocket()

        let url = try #require(wsTransport.connectURLs.first)
        #expect(url.path == "/proxy/base/api/realtime/ws")
    }

    @Test
    func subscribeWSAndUnsubscribeSendExpectedMessageShapes() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r2", "status": "ok"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }
        let unsubscribe = try await realtime.subscribeWS(tables: ["posts", "comments"], filter: "status='pub'") { _ in }

        try await waitUntil { connection.sentMessages.count >= 1 }
        let subscribe = try connection.sentJSON(at: 0)
        #expect(subscribe["type"] as? String == "subscribe")
        #expect((subscribe["tables"] as? [String]) == ["posts", "comments"])
        #expect(subscribe["filter"] as? String == "status='pub'")
        #expect((subscribe["ref"] as? String)?.isEmpty == false)

        unsubscribe()
        try await waitUntil { connection.sentMessages.count >= 2 }
        let unsubscribeMessage = try connection.sentJSON(at: 1)
        #expect(unsubscribeMessage["type"] as? String == "unsubscribe")
        #expect((unsubscribeMessage["tables"] as? [String]) == ["posts", "comments"])
    }

    @Test
    func wsEventRoutesToMatchingTableSubscription() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }

        var received: [RealtimeEvent] = []
        _ = try await realtime.subscribeWS(tables: ["posts"]) { event in
            received.append(event)
        }

        connection.enqueueIncoming(json: [
            "type": "event",
            "action": "INSERT",
            "table": "posts",
            "record": ["id": "rec_1"],
        ])
        connection.enqueueIncoming(json: [
            "type": "event",
            "action": "INSERT",
            "table": "comments",
            "record": ["id": "rec_2"],
        ])

        try await waitUntil { received.count == 1 }
        #expect(received[0].table == "posts")
        #expect(received[0].record["id"] as? String == "rec_1")
    }

    @Test
    func channelSubscribeAndUnsubscribeMessageShapes() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r2", "status": "ok"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }

        let leave = try await realtime.channelSubscribe("room:1")
        try await waitUntil { connection.sentMessages.count >= 1 }
        let subscribe = try connection.sentJSON(at: 0)
        #expect(subscribe["type"] as? String == "channel_subscribe")
        #expect(subscribe["channel"] as? String == "room:1")

        leave()
        try await waitUntil { connection.sentMessages.count >= 2 }
        let unsubscribe = try connection.sentJSON(at: 1)
        #expect(unsubscribe["type"] as? String == "channel_unsubscribe")
        #expect(unsubscribe["channel"] as? String == "room:1")
    }

    @Test
    func broadcastRequiresChannelSubscriptionAndRoutesIncomingBroadcast() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }
        try await realtime.connectWebSocket()

        do {
            try await realtime.broadcast(channel: "room:1", event: "new_message", payload: ["text": "x"], self: false)
            Issue.record("expected not-subscribed error")
        } catch let error as AYBError {
            #expect(error.code == "realtime/not-subscribed")
        }

        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])
        _ = try await realtime.channelSubscribe("room:1")

        var receivedEvent: String?
        var receivedPayload: [String: Any] = [:]
        let off = realtime.onBroadcast(channel: "room:1") { event, payload in
            receivedEvent = event
            receivedPayload = payload
        }
        defer { off() }

        connection.enqueueIncoming(json: ["type": "reply", "ref": "r2", "status": "ok"])
        try await realtime.broadcast(channel: "room:1", event: "new_message", payload: ["text": "hello"], self: false)

        connection.enqueueIncoming(json: [
            "type": "broadcast",
            "channel": "room:1",
            "event": "new_message",
            "payload": ["text": "hello"],
        ])

        try await waitUntil { receivedEvent != nil }
        #expect(receivedEvent == "new_message")
        #expect(receivedPayload["text"] as? String == "hello")
    }

    @Test
    func presenceTrackSyncUntrackLifecycle() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r2", "status": "ok"])
        connection.enqueueIncoming(json: [
            "type": "presence",
            "channel": "room:1",
            "presence_action": "sync",
            "presences": [
                "conn_1": ["userId": "u1"],
            ],
        ])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r3", "status": "ok"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r4", "status": "ok"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }

        do {
            try await realtime.presenceTrack(channel: "room:1", state: ["userId": "u1"])
            Issue.record("expected not-subscribed error")
        } catch let error as AYBError {
            #expect(error.code == "realtime/not-subscribed")
        }

        _ = try await realtime.channelSubscribe("room:1")
        try await realtime.presenceTrack(channel: "room:1", state: ["userId": "u1"])
        let synced = try await realtime.presenceSync(channel: "room:1")
        try await realtime.presenceUntrack(channel: "room:1")

        #expect((synced["conn_1"] as? [String: Any])?["userId"] as? String == "u1")
    }

    @Test
    func replyCorrelationWorksWhenRepliesArriveOutOfOrder() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }
        _ = try await realtime.channelSubscribe("room:1")

        async let first: Void = realtime.broadcast(channel: "room:1", event: "e1", payload: ["n": 1], self: false)
        async let second: Void = realtime.broadcast(channel: "room:1", event: "e2", payload: ["n": 2], self: false)

        // Two broadcast messages are now in-flight; reply out of order.
        try await waitUntil { connection.sentMessages.count >= 3 }
        let secondRef = try #require((try connection.sentJSON(at: 2))["ref"] as? String)
        let firstRef = try #require((try connection.sentJSON(at: 1))["ref"] as? String)
        connection.enqueueIncoming(json: ["type": "reply", "ref": secondRef, "status": "ok"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": firstRef, "status": "ok"])

        _ = try await (first, second)
    }

    @Test
    func errorAndSystemMessagesDoNotCrashReceiveLoop() async throws {
        let wsTransport = MockWebSocketTransport()
        let connection = MockWebSocketConnection()
        wsTransport.enqueue(connection: connection)
        connection.enqueueIncoming(json: ["type": "connected", "client_id": "ws-1"])
        connection.enqueueIncoming(json: ["type": "system", "message": "draining"])
        connection.enqueueIncoming(json: ["type": "error", "message": "transient"])
        connection.enqueueIncoming(json: ["type": "reply", "ref": "r1", "status": "ok"])

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(client: client, wsTransport: wsTransport)
        defer { realtime.disconnectWebSocket() }

        _ = try await realtime.channelSubscribe("room:1")
        #expect(connection.sentMessages.count >= 1)
    }
}
