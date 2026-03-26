import Foundation
import Testing
@testable import Allyourbase

struct RealtimeModelTests {
    @Test
    func realtimeEventDecodesCanonicalShapeWithOldRecord() throws {
        let payload: [String: Any] = [
            "action": "UPDATE",
            "table": "posts",
            "record": ["id": "rec_1", "title": "new"],
            "oldRecord": ["id": "rec_1", "title": "old"],
        ]

        let event = try RealtimeEvent(from: payload)

        #expect(event.action == "UPDATE")
        #expect(event.table == "posts")
        #expect(event.record["id"] as? String == "rec_1")
        #expect(event.oldRecord?["title"] as? String == "old")
    }

    @Test
    func realtimeEventAcceptsSnakeCaseOldRecord() throws {
        let payload: [String: Any] = [
            "action": "DELETE",
            "table": "posts",
            "record": ["id": "rec_2"],
            "old_record": ["id": "rec_2", "title": "legacy"],
        ]

        let event = try RealtimeEvent(from: payload)
        #expect(event.oldRecord?["title"] as? String == "legacy")
    }

    @Test
    func realtimeOptionsDefaultsMatchContract() {
        let options = RealtimeOptions()
        #expect(options.maxReconnectAttempts == 5)
        #expect(options.jitterMax == 0.1)
        #expect(!options.reconnectDelays.isEmpty)
    }

    @Test
    func realtimeOptionsSanitizeInvalidInputs() {
        let options = RealtimeOptions(
            maxReconnectAttempts: -2,
            reconnectDelays: [],
            jitterMax: -0.5
        )

        #expect(options.maxReconnectAttempts == 0)
        #expect(options.reconnectDelays.count == 1)
        #expect(abs(options.reconnectDelays[0] - 0.25) < 0.0001)
        #expect(options.jitterMax == 0)
    }
}
