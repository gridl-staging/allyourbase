import Foundation

public struct AYBConfiguration {
    public let baseURL: URL
    public let apiKey: String?
    public let timeout: TimeInterval
    public let maxRetries: Int
    public let retryDelay: TimeInterval

    public init(
        baseURL: URL,
        apiKey: String? = nil,
        timeout: TimeInterval = 30,
        maxRetries: Int = 0,
        retryDelay: TimeInterval = 0
    ) {
        self.baseURL = baseURL
        self.apiKey = apiKey
        self.timeout = timeout
        self.maxRetries = maxRetries
        self.retryDelay = retryDelay
    }
}

public struct AuthSession {
    public let token: String
    public let refreshToken: String

    public init(token: String, refreshToken: String) {
        self.token = token
        self.refreshToken = refreshToken
    }
}

public enum AuthStateEvent: String {
    case signedIn = "SIGNED_IN"
    case signedOut = "SIGNED_OUT"
    case tokenRefreshed = "TOKEN_REFRESHED"
}

public typealias AuthStateListener = (AuthStateEvent, AuthSession?) -> Void

public final class AYBClient {
    public let configuration: AYBConfiguration
    public let transport: HTTPTransport
    public let sseTransport: SSETransport
    public let wsTransport: WebSocketTransport
    public let tokenStore: TokenStore
    private let requestBuilder: RequestBuilder
    private let listenersLock = NSLock()
    private var listeners: [UUID: AuthStateListener] = [:]

    public lazy var auth: AuthClient = AuthClient(client: self)
    public lazy var records: RecordsClient = RecordsClient(client: self)
    public lazy var storage: StorageClient = StorageClient(client: self)
    public lazy var realtime: RealtimeClient = RealtimeClient(client: self)

    public init(
        _ baseURL: String,
        apiKey: String? = nil,
        transport: HTTPTransport? = nil,
        sseTransport: SSETransport? = nil,
        wsTransport: WebSocketTransport? = nil,
        tokenStore: TokenStore? = nil,
        timeout: TimeInterval = 30,
        maxRetries: Int = 0,
        retryDelay: TimeInterval = 0
    ) {
        let normalizedBaseURL = baseURL
            .trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard let parsedURL = URL(string: normalizedBaseURL) else {
            preconditionFailure("Invalid base URL: \(baseURL)")
        }

        let store = tokenStore ?? InMemoryTokenStore()
        if let apiKey {
            store.save(accessToken: apiKey, refreshToken: nil)
        }
        let cfg = AYBConfiguration(
            baseURL: parsedURL,
            apiKey: apiKey,
            timeout: timeout,
            maxRetries: maxRetries,
            retryDelay: retryDelay
        )

        self.configuration = cfg
        self.tokenStore = store
        self.transport = transport ?? URLSessionHTTPTransport(timeout: timeout)
        self.sseTransport = sseTransport ?? URLSessionSSETransport(timeout: timeout)
        self.wsTransport = wsTransport ?? URLSessionWebSocketTransport()
        self.requestBuilder = RequestBuilder(baseURL: parsedURL)
    }

    public var token: String? {
        tokenStore.accessToken()
    }

    public var refreshToken: String? {
        tokenStore.refreshToken()
    }

    public func setTokens(_ token: String, refreshToken: String) {
        tokenStore.save(accessToken: token, refreshToken: refreshToken)
    }

    public func clearTokens() {
        tokenStore.clear()
    }

    public func setApiKey(_ apiKey: String) {
        tokenStore.save(accessToken: apiKey, refreshToken: nil)
    }

    public func clearApiKey() {
        tokenStore.clear()
    }

    public func onAuthStateChange(_ listener: @escaping AuthStateListener) -> () -> Void {
        let id = UUID()
        listenersLock.lock()
        listeners[id] = listener
        listenersLock.unlock()

        return {
            self.listenersLock.lock()
            defer { self.listenersLock.unlock() }
            self.listeners.removeValue(forKey: id)
        }
    }

    func emitAuthState(_ event: AuthStateEvent) {
        let session: AuthSession?
        if let token = tokenStore.accessToken(), let refreshToken = tokenStore.refreshToken() {
            session = AuthSession(token: token, refreshToken: refreshToken)
        } else {
            session = nil
        }

        listenersLock.lock()
        let snapshot = listeners.values
        listenersLock.unlock()

        for listener in snapshot {
            listener(event, session)
        }
    }

    public func request<T>(
        _ path: String,
        method: HTTPMethod = .get,
        query: [String: String] = [:],
        queryItems: [URLQueryItem] = [],
        headers: [String: String] = [:],
        body: Any? = nil,
        skipAuth: Bool = false,
        decode: (Any?) throws -> T
    ) async throws -> T {
        let bearerToken = skipAuth ? nil : tokenStore.accessToken()
        let request = try requestBuilder.buildRequest(
            path: path,
            method: method,
            query: query,
            queryItems: queryItems,
            headers: headers,
            body: body,
            bearerToken: bearerToken
        )

        let response = try await sendWithRetries(request)

        guard (200..<300).contains(response.statusCode) else {
            throw AYBError.from(response: response)
        }

        let payload: Any?
        if response.body.isEmpty || response.statusCode == 204 {
            payload = nil
        } else {
            payload = AYBJSON.parse(response.body)
        }

        return try decode(payload)
    }

    private func sendWithRetries(_ request: HTTPRequest) async throws -> HTTPResponse {
        var attempt = 0
        while true {
            do {
                return try await transport.send(request)
            } catch {
                if error is CancellationError || Task.isCancelled {
                    throw error
                }
                if attempt >= configuration.maxRetries {
                    throw error
                }
                attempt += 1
                if configuration.retryDelay > 0 {
                    try Task.checkCancellation()
                    let delay = UInt64(configuration.retryDelay * 1_000_000_000)
                    try await Task.sleep(nanoseconds: delay)
                }
            }
        }
    }

    public func requestJSON(_ path: String, method: HTTPMethod = .get) async throws -> [String: Any] {
        return try await request(path, method: method, decode: { value in
            try AYBJSON.expectDictionary(value, "requestJSON")
        })
    }
}

public final class AuthClient {
    private unowned let client: AYBClient

    init(client: AYBClient) {
        self.client = client
    }

    public func register(email: String, password: String) async throws -> AuthResponse {
        try await authenticate(path: "/api/auth/register", email: email, password: password)
    }

    public func login(email: String, password: String) async throws -> AuthResponse {
        try await authenticate(path: "/api/auth/login", email: email, password: password)
    }

    private func authenticate(path: String, email: String, password: String) async throws -> AuthResponse {
        let response: AuthResponse = try await client.request(
            path,
            method: .post,
            body: ["email": email, "password": password],
            decode: AuthResponse.decode
        )
        client.setTokens(response.token, refreshToken: response.refreshToken)
        client.emitAuthState(.signedIn)
        return response
    }

    public func logout() async throws -> Void {
        let refreshToken = try requireRefreshToken()
        _ = try await client.request(
            "/api/auth/logout",
            method: .post,
            body: ["refreshToken": refreshToken],
            decode: { _ in () }
        )
        client.clearTokens()
        client.emitAuthState(.signedOut)
    }

    public func me() async throws -> User {
        return try await client.request(
            "/api/auth/me",
            decode: { json in
                let dictionary = try AYBJSON.expectDictionary(json, "auth.me")
                return try User(from: dictionary)
            }
        )
    }

    public func refresh() async throws -> AuthResponse {
        let refreshToken = try requireRefreshToken()
        let response: AuthResponse = try await client.request(
            "/api/auth/refresh",
            method: .post,
            body: ["refreshToken": refreshToken],
            decode: AuthResponse.decode
        )
        client.setTokens(response.token, refreshToken: response.refreshToken)
        client.emitAuthState(.tokenRefreshed)
        return response
    }

    private func requireRefreshToken() throws -> String {
        guard let refreshToken = client.refreshToken, !refreshToken.isEmpty else {
            throw AYBError(
                status: 400,
                message: "Missing refresh token",
                code: "auth/missing-refresh-token"
            )
        }
        return refreshToken
    }
}

public final class RecordsClient {
    private unowned let client: AYBClient

    init(client: AYBClient) {
        self.client = client
    }

    public func list(_ collection: String, params: ListParams? = nil) async throws -> ListResponse<[String: Any]> {
        let queryItems = params?.toQueryItems() ?? []
        return try await client.request(
            "/api/collections/\(collection)",
            method: .get,
            queryItems: queryItems,
            decode: { json in
                guard let payload = json else {
                    throw AYBDecodingError.invalidType("ListResponse")
                }
                return try ListResponse.decode(payload, decodeItem: { (item: [String: Any]) in item })
            }
        )
    }

    public func get(_ collection: String, _ id: String, params: GetParams? = nil) async throws -> [String: Any] {
        let queryItems = params?.toQueryItems() ?? []
        return try await client.request(
            "/api/collections/\(collection)/\(id)",
            queryItems: queryItems,
            decode: { json in
                try AYBJSON.expectDictionary(json, "records.get")
            }
        )
    }

    public func create(_ collection: String, data: [String: Any]) async throws -> [String: Any] {
        return try await client.request(
            "/api/collections/\(collection)",
            method: .post,
            body: data,
            decode: { json in
                try AYBJSON.expectDictionary(json, "records.create")
            }
        )
    }

    public func update(_ collection: String, id: String, data: [String: Any]) async throws -> [String: Any] {
        return try await client.request(
            "/api/collections/\(collection)/\(id)",
            method: .patch,
            body: data,
            decode: { json in
                try AYBJSON.expectDictionary(json, "records.update")
            }
        )
    }

    public func delete(_ collection: String, id: String) async throws {
        _ = try await client.request(
            "/api/collections/\(collection)/\(id)",
            method: .delete,
            decode: { _ in () }
        )
    }

    public func batch(_ collection: String, operations: [BatchOperation]) async throws -> [BatchResult<[String: Any]>] {
        let payload = ["operations": operations.map { $0.toDictionary() }]
        return try await client.request(
            "/api/collections/\(collection)/batch",
            method: .post,
            body: payload,
            decode: { json in
                let results = try AYBJSON.expectArray(json, "records.batch")
                return try results.map { raw in
                    try BatchResult.decode(raw, decodeBody: { body in
                        try body.map { try AYBJSON.expectDictionary($0, "batch.body") }
                    })
                }
            }
        )
    }
}

public final class StorageClient {
    private unowned let client: AYBClient

    init(client: AYBClient) {
        self.client = client
    }

    public func upload(
        bucket: String,
        data: Data,
        name: String? = nil,
        contentType: String? = nil
    ) async throws -> StorageObject {
        let multipartData = MultipartBody.build(
            file: data,
            fieldName: "file",
            filename: name,
            contentType: contentType
        )

        return try await client.request(
            "/api/storage/\(bucket)",
            method: .post,
            headers: ["Content-Type": multipartData.contentType],
            body: multipartData.data,
            decode: { json in
                try StorageObject(from: try AYBJSON.expectDictionary(json, "storage.upload"))
            }
        )
    }

    public func downloadUrl(bucket: String, name: String) -> String {
        return "\(client.configuration.baseURL)/api/storage/\(bucket)/\(name)"
    }

    public func delete(bucket: String, name: String) async throws -> Void {
        _ = try await client.request(
            "/api/storage/\(bucket)/\(name)",
            method: .delete,
            decode: { _ in () }
        )
    }

    public func list(bucket: String, prefix: String? = nil, limit: Int? = nil, offset: Int? = nil) async throws -> StorageListResponse {
        var query: [String: String] = [:]
        if let prefix {
            query["prefix"] = prefix
        }
        if let limit {
            query["limit"] = String(limit)
        }
        if let offset {
            query["offset"] = String(offset)
        }

        return try await client.request(
            "/api/storage/\(bucket)",
            method: .get,
            query: query,
            decode: { json in
                try StorageListResponse(from: try AYBJSON.expectDictionary(json, "storage.list"))
            }
        )
    }

    public func getSignedUrl(bucket: String, name: String, expiresIn: Int = 3600) async throws -> String {
        let relativePath: String = try await client.request(
            "/api/storage/\(bucket)/\(name)/sign",
            method: .post,
            body: ["expiresIn": expiresIn],
            decode: { json in
                let dictionary = try AYBJSON.expectDictionary(json, "storage.sign")
                return try AYBJSON.requiredString(dictionary, ["url"], "storage.sign.url")
            }
        )

        if relativePath.hasPrefix("/") {
            return "\(client.configuration.baseURL)\(relativePath)"
        }
        return relativePath
    }
}
