import Foundation
import Testing
@testable import Allyourbase

struct WebSocketModelTests {
    @Test
    func clientMessageTypesMatchBackendProtocol() {
        #expect(WebSocketClientMessageType.auth.rawValue == "auth")
        #expect(WebSocketClientMessageType.subscribe.rawValue == "subscribe")
        #expect(WebSocketClientMessageType.unsubscribe.rawValue == "unsubscribe")
        #expect(WebSocketClientMessageType.channelSubscribe.rawValue == "channel_subscribe")
        #expect(WebSocketClientMessageType.channelUnsubscribe.rawValue == "channel_unsubscribe")
        #expect(WebSocketClientMessageType.broadcast.rawValue == "broadcast")
        #expect(WebSocketClientMessageType.presenceTrack.rawValue == "presence_track")
        #expect(WebSocketClientMessageType.presenceUntrack.rawValue == "presence_untrack")
        #expect(WebSocketClientMessageType.presenceSync.rawValue == "presence_sync")
    }

    @Test
    func serverMessageTypesMatchBackendProtocol() {
        #expect(WebSocketServerMessageType.connected.rawValue == "connected")
        #expect(WebSocketServerMessageType.reply.rawValue == "reply")
        #expect(WebSocketServerMessageType.event.rawValue == "event")
        #expect(WebSocketServerMessageType.broadcast.rawValue == "broadcast")
        #expect(WebSocketServerMessageType.presence.rawValue == "presence")
        #expect(WebSocketServerMessageType.error.rawValue == "error")
        #expect(WebSocketServerMessageType.system.rawValue == "system")
    }

    @Test
    func clientMessageEncodesRefPayloadAndSelfFlag() {
        let message = WebSocketClientMessage(
            type: .broadcast,
            ref: "ref_1",
            channel: "room:1",
            event: "new_message",
            payload: ["text": "hello"],
            selfBroadcast: true
        )

        let dictionary = message.toDictionary()
        #expect(dictionary["type"] as? String == "broadcast")
        #expect(dictionary["ref"] as? String == "ref_1")
        #expect(dictionary["channel"] as? String == "room:1")
        #expect(dictionary["event"] as? String == "new_message")
        #expect(dictionary["self"] as? Bool == true)
        #expect((dictionary["payload"] as? [String: Any])?["text"] as? String == "hello")
    }

    @Test
    func serverMessageDecodesPresenceSyncPayload() throws {
        let json: [String: Any] = [
            "type": "presence",
            "channel": "room:1",
            "presence_action": "sync",
            "presences": [
                "conn_a": ["userId": "u1"],
                "conn_b": ["userId": "u2"],
            ],
        ]

        let message = try WebSocketServerMessage(from: json)
        #expect(message.type == .presence)
        #expect(message.channel == "room:1")
        #expect(message.presenceAction == "sync")
        #expect(message.presences?["conn_a"]?["userId"] as? String == "u1")
    }
}
