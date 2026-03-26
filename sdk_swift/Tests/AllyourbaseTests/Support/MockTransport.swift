import Foundation
import Allyourbase

enum MockTransportError: Error {
    case missingResponse
}

final class MockTransport: HTTPTransport {
    struct StubResponse {
        let status: Int
        let body: Data
        let headers: [String: String]

        init(
            status: Int,
            body: Data,
            headers: [String: String] = ["content-type": "application/json"]
        ) {
            self.status = status
            self.body = body
            self.headers = headers
        }

        init(
            status: Int,
            json: Any,
            headers: [String: String] = ["content-type": "application/json"]
        ) {
            self.status = status
            self.body = dataFromJSON(json)
            self.headers = headers
        }
    }

    private(set) var requests: [HTTPRequest] = []
    private var responses: [StubResponse]

    init(responses: [StubResponse] = []) {
        self.responses = responses
    }

    func enqueue(_ response: StubResponse) {
        responses.append(response)
    }

    func send(_ request: HTTPRequest) async throws -> HTTPResponse {
        requests.append(request)
        guard !responses.isEmpty else {
            throw MockTransportError.missingResponse
        }

        let next = responses.removeFirst()
        return HTTPResponse(
            statusCode: next.status,
            statusText: HTTPURLResponse.localizedString(forStatusCode: next.status),
            headers: next.headers,
            body: next.body
        )
    }
}
