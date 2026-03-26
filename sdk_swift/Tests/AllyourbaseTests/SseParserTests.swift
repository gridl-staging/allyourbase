import Foundation
import Testing
@testable import Allyourbase

struct SseParserTests {
    @Test
    func parsesConnectedAndDataMessages() async throws {
        let source = """
        event: connected
        data: {"clientId":"abc"}

        event: INSERT
        data: {"action":"INSERT","table":"posts","record":{"id":"rec_1"}}

        """
        let stream = byteStream(from: source)
        let parser = SseParser(bytes: stream)

        var messages: [SseMessage] = []
        for try await message in parser.messages() {
            messages.append(message)
        }

        #expect(messages.count == 2)
        #expect(messages[0].event == "connected")
        #expect(messages[0].data == #"{"clientId":"abc"}"#)
        #expect(messages[1].event == "INSERT")
        #expect(messages[1].data?.contains(#""action":"INSERT""#) == true)
    }

    @Test
    func ignoresHeartbeatAndMalformedLines() async throws {
        let source = """
        :heartbeat
        malformed-line
        event: UPDATE
        data: {"action":"UPDATE","table":"posts","record":{"id":"rec_2"}}

        """
        let parser = SseParser(bytes: byteStream(from: source))

        var messages: [SseMessage] = []
        for try await message in parser.messages() {
            messages.append(message)
        }

        #expect(messages.count == 1)
        #expect(messages[0].event == "UPDATE")
    }
}

private func byteStream(from text: String) -> AsyncThrowingStream<UInt8, Error> {
    AsyncThrowingStream { continuation in
        for byte in text.utf8 {
            continuation.yield(byte)
        }
        continuation.finish()
    }
}
