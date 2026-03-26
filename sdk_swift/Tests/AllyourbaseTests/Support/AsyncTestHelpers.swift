import Foundation
import Testing

func waitUntil(
    timeout: TimeInterval = 0.5,
    interval: TimeInterval = 0.005,
    condition: @escaping () -> Bool
) async throws {
    let deadline = Date().timeIntervalSince1970 + timeout
    while Date().timeIntervalSince1970 < deadline {
        if condition() {
            return
        }
        try await Task.sleep(nanoseconds: UInt64(interval * 1_000_000_000))
    }
    Issue.record("timed out waiting for condition")
}
