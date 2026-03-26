import Foundation

enum Stage3TestBootstrap {
    static let baseURL = "https://api.example.com"
    static let stableRequestTimeout: TimeInterval = 0.5
}

// Convenience typealias so tests can use StubResponse without qualification.
typealias StubResponse = MockTransport.StubResponse

func dataFromJSON(_ value: Any) -> Data {
    if let value = value as? Data {
        return value
    }
    if value is NSNull {
        // NSNull signals "empty body" (e.g. 204 No Content) — return empty Data.
        return Data()
    }
    return (try? JSONSerialization.data(withJSONObject: value, options: [])) ?? Data("{}".utf8)
}

func lowercasedLookup(_ headers: [String: String], _ name: String) -> String? {
    for (key, value) in headers {
        if key.caseInsensitiveCompare(name) == .orderedSame {
            return value
        }
    }
    return nil
}
