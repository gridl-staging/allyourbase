import Foundation
@testable import Allyourbase

final class MockWebSocketConnection: WebSocketConnection, @unchecked Sendable {
    private let lock = NSLock()
    private var queue: [Result<String, Error>] = []
    private var waiters: [CheckedContinuation<String, Error>] = []
    private(set) var sentMessages: [String] = []
    private(set) var pingCount = 0
    private(set) var closeCount = 0

    private func withLock<T>(_ body: () -> T) -> T {
        lock.lock()
        defer { lock.unlock() }
        return body()
    }

    func send(text: String) async throws {
        _ = withLock {
            sentMessages.append(text)
        }
    }

    func receive() async throws -> String {
        try await withCheckedThrowingContinuation { continuation in
            lock.lock()
            if !queue.isEmpty {
                let next = queue.removeFirst()
                lock.unlock()
                continuation.resume(with: next)
                return
            }
            waiters.append(continuation)
            lock.unlock()
        }
    }

    func ping() async throws {
        _ = withLock {
            pingCount += 1
        }
    }

    func close() async {
        let outstanding: [CheckedContinuation<String, Error>] = withLock {
            closeCount += 1
            let copy = waiters
            waiters.removeAll()
            return copy
        }
        for waiter in outstanding {
            waiter.resume(throwing: CancellationError())
        }
    }

    func enqueueIncoming(text: String) {
        lock.lock()
        if !waiters.isEmpty {
            let waiter = waiters.removeFirst()
            lock.unlock()
            waiter.resume(returning: text)
            return
        }
        queue.append(.success(text))
        lock.unlock()
    }

    func enqueueIncoming(json: [String: Any]) {
        let data = (try? JSONSerialization.data(withJSONObject: json, options: [])) ?? Data("{}".utf8)
        enqueueIncoming(text: String(decoding: data, as: UTF8.self))
    }

    func enqueueError(_ error: Error) {
        lock.lock()
        if !waiters.isEmpty {
            let waiter = waiters.removeFirst()
            lock.unlock()
            waiter.resume(throwing: error)
            return
        }
        queue.append(.failure(error))
        lock.unlock()
    }

    func sentJSON(at index: Int) throws -> [String: Any] {
        let message = withLock { sentMessages[index] }
        let data = Data(message.utf8)
        let parsed = AYBJSON.parse(data)
        return try AYBJSON.expectDictionary(parsed, "MockWebSocketConnection.sentJSON")
    }
}

final class MockWebSocketTransport: WebSocketTransport, @unchecked Sendable {
    private(set) var connectURLs: [URL] = []
    private(set) var connectHeaders: [[String: String]] = []
    private var queue: [Result<MockWebSocketConnection, Error>] = []

    func enqueue(connection: MockWebSocketConnection) {
        queue.append(.success(connection))
    }

    func enqueue(error: Error) {
        queue.append(.failure(error))
    }

    func connect(url: URL, headers: [String: String]) async throws -> any WebSocketConnection {
        connectURLs.append(url)
        connectHeaders.append(headers)
        guard !queue.isEmpty else {
            throw MockTransportError.missingResponse
        }
        let next = queue.removeFirst()
        switch next {
        case let .success(connection):
            return connection
        case let .failure(error):
            throw error
        }
    }
}
