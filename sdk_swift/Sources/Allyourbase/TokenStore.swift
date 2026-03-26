import Foundation

public protocol TokenStore {
    func accessToken() -> String?
    func refreshToken() -> String?
    func save(accessToken: String?, refreshToken: String?)
    func clear()
}

public final class InMemoryTokenStore: TokenStore {
    private var token: String?
    private var refreshTokenValue: String?
    private let lock = NSLock()

    public init(accessToken: String? = nil, refreshToken: String? = nil) {
        self.token = accessToken
        self.refreshTokenValue = refreshToken
    }

    public func accessToken() -> String? {
        lock.lock()
        defer { lock.unlock() }
        return token
    }

    public func refreshToken() -> String? {
        lock.lock()
        defer { lock.unlock() }
        return refreshTokenValue
    }

    public func save(accessToken: String?, refreshToken: String?) {
        lock.lock()
        defer { lock.unlock() }
        token = accessToken
        refreshTokenValue = refreshToken
    }

    public func clear() {
        save(accessToken: nil, refreshToken: nil)
    }
}
