import Foundation
import Testing
@testable import Allyourbase

private final class CancellingTransport: HTTPTransport {
    private(set) var sendCount = 0

    func send(_ request: HTTPRequest) async throws -> HTTPResponse {
        sendCount += 1
        throw CancellationError()
    }
}

struct ClientRetryTests {
    @Test func cancellationDoesNotRetry() async {
        let transport = CancellingTransport()
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            maxRetries: 3
        )

        do {
            _ = try await client.request("/api/test", decode: { _ in () })
            Issue.record("request should throw CancellationError")
        } catch is CancellationError {
            #expect(transport.sendCount == 1)
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
    }
}
