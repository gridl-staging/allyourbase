import Foundation
@testable import Allyourbase

final class MockSSEConnection: SSEConnection {
    private let lock = NSLock()
    private var continuation: AsyncThrowingStream<UInt8, Error>.Continuation?
    private(set) var cancelled = false

    func byteStream() -> AsyncThrowingStream<UInt8, Error> {
        AsyncThrowingStream { continuation in
            lock.lock()
            self.continuation = continuation
            let isCancelled = cancelled
            lock.unlock()
            if isCancelled {
                continuation.finish()
            }
        }
    }

    func cancel() {
        lock.lock()
        cancelled = true
        let continuation = self.continuation
        lock.unlock()
        continuation?.finish()
    }

    func send(text: String) {
        let bytes = Array(text.utf8)
        lock.lock()
        let continuation = self.continuation
        lock.unlock()
        for byte in bytes {
            continuation?.yield(byte)
        }
    }

    func finish() {
        lock.lock()
        let continuation = self.continuation
        lock.unlock()
        continuation?.finish()
    }
}

final class MockSSETransport: SSETransport {
    enum Behavior {
        case connect(MockSSEConnection)
        case fail(Error)
    }

    private(set) var requests: [HTTPRequest] = []
    private var queue: [Behavior] = []

    func enqueue(connection: MockSSEConnection) {
        queue.append(.connect(connection))
    }

    func enqueue(error: Error) {
        queue.append(.fail(error))
    }

    func connect(_ request: HTTPRequest) async throws -> any SSEConnection {
        requests.append(request)
        guard !queue.isEmpty else {
            throw MockTransportError.missingResponse
        }
        let next = queue.removeFirst()
        switch next {
        case let .connect(connection):
            return connection
        case let .fail(error):
            throw error
        }
    }
}
