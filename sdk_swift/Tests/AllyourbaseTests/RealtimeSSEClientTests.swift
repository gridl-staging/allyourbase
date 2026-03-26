import Foundation
import Testing
@testable import Allyourbase

private actor SleepRecorder {
    private var values: [TimeInterval] = []

    func append(_ value: TimeInterval) {
        values.append(value)
    }

    func allValues() -> [TimeInterval] {
        values
    }

    func count() -> Int {
        values.count
    }
}

struct RealtimeSSEClientTests {
    @Test
    func subscribeBuildsCorrectURLWithTablesTokenAndFilter() async throws {
        let transport = MockSSETransport()
        let connection = MockSSEConnection()
        transport.enqueue(connection: connection)

        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_token", refreshToken: "refresh")
        )
        let realtime = RealtimeClient(
            client: client,
            sseTransport: transport,
            options: RealtimeOptions(maxReconnectAttempts: 0, reconnectDelays: [0.01], jitterMax: 0),
            jitterProvider: { _ in 0 },
            sleep: { _ in }
        )

        let unsubscribe = realtime.subscribe(tables: ["posts", "comments"], filter: "status='published'") { _ in }
        defer { unsubscribe() }

        try await waitUntil { !transport.requests.isEmpty }

        let request = try #require(transport.requests.first)
        #expect(request.url.path == "/api/realtime")
        let components = try #require(URLComponents(url: request.url, resolvingAgainstBaseURL: false))
        let queryItems = components.queryItems ?? []
        #expect(queryItems.first(where: { $0.name == "tables" })?.value == "posts,comments")
        #expect(queryItems.first(where: { $0.name == "token" })?.value == "jwt_token")
        #expect(queryItems.first(where: { $0.name == "filter" })?.value == "status='published'")
    }

    @Test
    func parsedEventsAreDeliveredToCallback() async throws {
        let transport = MockSSETransport()
        let connection = MockSSEConnection()
        transport.enqueue(connection: connection)
        let client = AYBClient(Stage3TestBootstrap.baseURL)

        let realtime = RealtimeClient(
            client: client,
            sseTransport: transport,
            options: RealtimeOptions(maxReconnectAttempts: 0, reconnectDelays: [0.01], jitterMax: 0),
            jitterProvider: { _ in 0 },
            sleep: { _ in }
        )

        var events: [RealtimeEvent] = []
        let unsubscribe = realtime.subscribe(tables: ["posts"]) { event in
            events.append(event)
        }
        defer { unsubscribe() }

        try await waitUntil { !transport.requests.isEmpty }
        connection.send(text: """
        event: connected
        data: {"clientId":"abc"}

        event: INSERT
        data: {"action":"INSERT","table":"posts","record":{"id":"rec_1"}}

        """)
        connection.finish()

        try await waitUntil { !events.isEmpty }
        #expect(events.count == 1)
        #expect(events[0].action == "INSERT")
        #expect(events[0].table == "posts")
        #expect(events[0].record["id"] as? String == "rec_1")
    }

    @Test
    func unsubscribeCancelsConnection() async throws {
        let transport = MockSSETransport()
        let connection = MockSSEConnection()
        transport.enqueue(connection: connection)
        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(
            client: client,
            sseTransport: transport,
            options: RealtimeOptions(maxReconnectAttempts: 0, reconnectDelays: [0.01], jitterMax: 0),
            jitterProvider: { _ in 0 },
            sleep: { _ in }
        )

        let unsubscribe = realtime.subscribe(tables: ["posts"]) { _ in }
        try await waitUntil { !transport.requests.isEmpty }
        unsubscribe()

        #expect(connection.cancelled == true)
    }

    @Test
    func reconnectionUsesSteppedDelaysWithJitter() async throws {
        let transport = MockSSETransport()
        transport.enqueue(error: URLError(.networkConnectionLost))
        transport.enqueue(error: URLError(.networkConnectionLost))
        transport.enqueue(error: URLError(.networkConnectionLost))

        let recorder = SleepRecorder()
        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(
            client: client,
            sseTransport: transport,
            options: RealtimeOptions(maxReconnectAttempts: 2, reconnectDelays: [0.10, 0.20], jitterMax: 0.05),
            jitterProvider: { _ in 0.03 },
            sleep: { nanos in
                await recorder.append(TimeInterval(nanos) / 1_000_000_000)
            }
        )

        let unsubscribe = realtime.subscribe(tables: ["posts"]) { _ in }
        defer { unsubscribe() }

        try await waitUntil { transport.requests.count == 3 }
        let sleeps = await recorder.allValues()
        #expect(sleeps.count == 2)
        #expect(abs(sleeps[0] - 0.13) < 0.0001)
        #expect(abs(sleeps[1] - 0.23) < 0.0001)
    }

    @Test
    func authFailureStopsReconnection() async throws {
        let transport = MockSSETransport()
        transport.enqueue(error: AYBError(status: 401, message: "Unauthorized"))
        let recorder = SleepRecorder()

        let client = AYBClient(Stage3TestBootstrap.baseURL)
        let realtime = RealtimeClient(
            client: client,
            sseTransport: transport,
            options: RealtimeOptions(maxReconnectAttempts: 5, reconnectDelays: [0.01], jitterMax: 0.01),
            jitterProvider: { _ in 0.005 },
            sleep: { _ in
                await recorder.append(0)
            }
        )

        let unsubscribe = realtime.subscribe(tables: ["posts"]) { _ in }
        defer { unsubscribe() }

        try await waitUntil { transport.requests.count == 1 }
        // Give background loop a chance to schedule a retry if logic is wrong.
        try await Task.sleep(nanoseconds: 20_000_000)
        #expect(transport.requests.count == 1)
        #expect(await recorder.count() == 0)
    }
}
