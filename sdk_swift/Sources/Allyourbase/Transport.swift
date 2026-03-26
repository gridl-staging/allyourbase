import Foundation

public enum HTTPMethod: String {
    case get = "GET"
    case post = "POST"
    case patch = "PATCH"
    case delete = "DELETE"
}

public struct HTTPRequest {
    public let url: URL
    public let method: HTTPMethod
    public let headers: [String: String]
    public let body: Data?

    public init(url: URL, method: HTTPMethod, headers: [String: String], body: Data?) {
        self.url = url
        self.method = method
        self.headers = headers
        self.body = body
    }
}

public struct HTTPResponse {
    public let statusCode: Int
    public let statusText: String
    public let headers: [String: String]
    public let body: Data

    public init(statusCode: Int, statusText: String, headers: [String: String], body: Data) {
        self.statusCode = statusCode
        self.statusText = statusText
        self.headers = headers
        self.body = body
    }
}

public protocol HTTPTransport {
    func send(_ request: HTTPRequest) async throws -> HTTPResponse
}

public protocol SSEConnection {
    func byteStream() -> AsyncThrowingStream<UInt8, Error>
    func cancel()
}

public protocol SSETransport {
    func connect(_ request: HTTPRequest) async throws -> any SSEConnection
}

public protocol WebSocketConnection: Sendable {
    func send(text: String) async throws
    func receive() async throws -> String
    func ping() async throws
    func close() async
}

public protocol WebSocketTransport: Sendable {
    func connect(url: URL, headers: [String: String]) async throws -> any WebSocketConnection
}

public final class URLSessionHTTPTransport: HTTPTransport {
    private let session: URLSession
    private let timeout: TimeInterval

    public init(timeout: TimeInterval = 30.0, session: URLSession? = nil) {
        self.timeout = timeout
        if let session {
            self.session = session
        } else {
            let config = URLSessionConfiguration.default
            config.timeoutIntervalForRequest = timeout
            config.timeoutIntervalForResource = timeout
            self.session = URLSession(configuration: config)
        }
    }

    public func send(_ request: HTTPRequest) async throws -> HTTPResponse {
        var urlRequest = URLRequest(url: request.url)
        urlRequest.httpMethod = request.method.rawValue
        urlRequest.httpBody = request.body
        urlRequest.timeoutInterval = timeout

        for (name, value) in request.headers {
            urlRequest.setValue(value, forHTTPHeaderField: name)
        }

        let (data, urlResponse) = try await session.data(for: urlRequest)
        guard let httpResponse = urlResponse as? HTTPURLResponse else {
            throw URLError(.badServerResponse)
        }

        var headers: [String: String] = [:]
        for (name, value) in httpResponse.allHeaderFields {
            headers[String(describing: name)] = String(describing: value)
        }

        return HTTPResponse(
            statusCode: httpResponse.statusCode,
            statusText: HTTPURLResponse.localizedString(forStatusCode: httpResponse.statusCode),
            headers: headers,
            body: data
        )
    }
}

private final class URLSessionSSEConnection: SSEConnection, @unchecked Sendable {
    private let bytes: URLSession.AsyncBytes
    private let lock = NSLock()
    private var isCancelled = false
    private var streamTask: Task<Void, Never>?

    init(bytes: URLSession.AsyncBytes) {
        self.bytes = bytes
    }

    private func isConnectionCancelled() -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return isCancelled
    }

    private func markCancelled() {
        lock.lock()
        isCancelled = true
        lock.unlock()
    }

    private func replaceStreamTask(_ task: Task<Void, Never>?) -> Task<Void, Never>? {
        lock.lock()
        defer { lock.unlock() }
        let previous = streamTask
        streamTask = task
        return previous
    }

    private func currentStreamTask() -> Task<Void, Never>? {
        lock.lock()
        defer { lock.unlock() }
        return streamTask
    }

    func byteStream() -> AsyncThrowingStream<UInt8, Error> {
        AsyncThrowingStream { continuation in
            let task = Task {
                do {
                    for try await byte in bytes {
                        if isConnectionCancelled() {
                            continuation.finish()
                            return
                        }
                        continuation.yield(byte)
                    }
                    continuation.finish()
                } catch is CancellationError {
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
            replaceStreamTask(task)?.cancel()
        }
    }

    func cancel() {
        markCancelled()
        currentStreamTask()?.cancel()
    }
}

public final class URLSessionSSETransport: SSETransport {
    private let session: URLSession
    private let timeout: TimeInterval

    public init(timeout: TimeInterval = 30.0, session: URLSession? = nil) {
        self.timeout = timeout
        if let session {
            self.session = session
        } else {
            let config = URLSessionConfiguration.default
            config.timeoutIntervalForRequest = timeout
            config.timeoutIntervalForResource = timeout
            self.session = URLSession(configuration: config)
        }
    }

    public func connect(_ request: HTTPRequest) async throws -> any SSEConnection {
        var urlRequest = URLRequest(url: request.url)
        urlRequest.httpMethod = request.method.rawValue
        urlRequest.httpBody = request.body
        urlRequest.timeoutInterval = timeout
        for (name, value) in request.headers {
            urlRequest.setValue(value, forHTTPHeaderField: name)
        }

        let (bytes, response) = try await session.bytes(for: urlRequest)
        guard let httpResponse = response as? HTTPURLResponse else {
            throw URLError(.badServerResponse)
        }
        guard (200..<300).contains(httpResponse.statusCode) else {
            throw AYBError(
                status: httpResponse.statusCode,
                message: HTTPURLResponse.localizedString(forStatusCode: httpResponse.statusCode)
            )
        }
        return URLSessionSSEConnection(bytes: bytes)
    }
}

private final class URLSessionWebSocketConnection: WebSocketConnection, @unchecked Sendable {
    private let task: URLSessionWebSocketTask

    init(task: URLSessionWebSocketTask) {
        self.task = task
    }

    func send(text: String) async throws {
        try await task.send(.string(text))
    }

    func receive() async throws -> String {
        let message = try await task.receive()
        switch message {
        case let .string(text):
            return text
        case let .data(data):
            return String(decoding: data, as: UTF8.self)
        @unknown default:
            throw URLError(.cannotParseResponse)
        }
    }

    func ping() async throws {
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            task.sendPing { error in
                if let error {
                    continuation.resume(throwing: error)
                    return
                }
                continuation.resume()
            }
        }
    }

    func close() async {
        task.cancel(with: .goingAway, reason: nil)
    }
}

public final class URLSessionWebSocketTransport: WebSocketTransport, @unchecked Sendable {
    private let session: URLSession

    public init(session: URLSession? = nil) {
        self.session = session ?? URLSession(configuration: .default)
    }

    public func connect(url: URL, headers: [String: String] = [:]) async throws -> any WebSocketConnection {
        var request = URLRequest(url: url)
        for (name, value) in headers {
            request.setValue(value, forHTTPHeaderField: name)
        }
        let task = session.webSocketTask(with: request)
        task.resume()
        return URLSessionWebSocketConnection(task: task)
    }
}

public final class RequestBuilder {
    public let baseURL: URL

    public init(baseURL: URL) {
        self.baseURL = baseURL
    }

    private func normalizedPath(_ path: String) -> String {
        if path.isEmpty {
            return "/"
        }
        if path.hasPrefix("/") {
            return path
        }
        return "/\(path)"
    }

    private func deterministicQueryItems(_ query: [String: String]) -> [URLQueryItem] {
        return query.keys.sorted().map { key in
            URLQueryItem(name: key, value: query[key])
        }
    }

    public func makeURL(
        path: String,
        query: [String: String] = [:],
        queryItems: [URLQueryItem] = []
    ) throws -> URL {
        let basePath = normalizedPath(path)
        let base = baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard URLComponents(string: base) != nil else {
            throw RequestBuilderError.invalidBaseURL(base)
        }

        let pathToSet = base + basePath
        guard var components = URLComponents(string: pathToSet) else {
            throw RequestBuilderError.invalidPath(pathToSet)
        }

        if !queryItems.isEmpty {
            components.queryItems = queryItems
        } else if !query.isEmpty {
            components.queryItems = deterministicQueryItems(query)
        }
        guard let url = components.url else {
            throw RequestBuilderError.unableToBuildURL
        }
        return url
    }

    public func buildRequest(
        path: String,
        method: HTTPMethod,
        query: [String: String] = [:],
        queryItems: [URLQueryItem] = [],
        headers: [String: String] = [:],
        body: Any? = nil,
        bearerToken: String? = nil
    ) throws -> HTTPRequest {
        let url = try makeURL(path: path, query: query, queryItems: queryItems)
        var mergedHeaders: [String: String] = ["Accept": "application/json"]
        for (key, value) in headers {
            mergedHeaders[key] = value
        }

        if let bearerToken {
            mergedHeaders["Authorization"] = "Bearer \(bearerToken)"
        }

        let encodedBody: Data?
        if let body {
            if mergedHeaders["Content-Type"] == nil {
                mergedHeaders["Content-Type"] = "application/json"
            }
            encodedBody = try RequestBodyEncoder.encode(body)
        } else {
            encodedBody = nil
        }

        return HTTPRequest(url: url, method: method, headers: mergedHeaders, body: encodedBody)
    }
}

public enum RequestBuilderError: Error {
    case invalidBaseURL(String)
    case invalidPath(String)
    case unableToBuildURL
}

public enum RequestBodyEncoder {
    public static func encode(_ body: Any) throws -> Data {
        if let data = body as? Data {
            return data
        }
        if let string = body as? String {
            return Data(string.utf8)
        }
        return try JSONSerialization.data(withJSONObject: body, options: [.fragmentsAllowed])
    }
}

// Helper to build multipart/form-data content for file uploads
public struct MultipartBody {
    public let data: Data
    public let contentType: String
    
    private init(data: Data, boundary: String) {
        self.data = data
        self.contentType = "multipart/form-data; boundary=\(boundary)"
    }
    
    public static func build(
        file: Data,
        fieldName: String = "file",
        filename: String? = nil,
        contentType: String? = nil,
        additionalFields: [String: String] = [:]
    ) -> MultipartBody {
        let boundary = "Boundary-\(UUID().uuidString)"
        var body = Data()
        
        // Add additional fields first
        for (key, value) in additionalFields {
            body.append(string: "--\(boundary)\r\n")
            body.append(string: "Content-Disposition: form-data; name=\"\(key)\"\r\n")
            body.append(string: "\r\n")
            body.append(string: "\(value)\r\n")
        }
        
        // Add file field
        body.append(string: "--\(boundary)\r\n")
        var contentDisposition = "Content-Disposition: form-data; name=\"\(fieldName)\""
        if let filename = filename {
            contentDisposition += "; filename=\"\(filename)\""
        }
        body.append(string: "\(contentDisposition)\r\n")
        
        if let contentType = contentType {
            body.append(string: "Content-Type: \(contentType)\r\n")
        }
        body.append(string: "\r\n")
        body.append(file)
        body.append(string: "\r\n")
        
        // Close boundary
        body.append(string: "--\(boundary)--\r\n")
        
        return MultipartBody(data: body, boundary: boundary)
    }
}

// Helper extension to append string to data
extension Data {
    mutating func append(string: String) {
        if let data = string.data(using: .utf8) {
            self.append(data)
        }
    }
}
