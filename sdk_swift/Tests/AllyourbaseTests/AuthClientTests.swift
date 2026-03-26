import Foundation
import Testing
@testable import Allyourbase

struct AuthClientTests {
    @Test func registerGeneratesExpectedRequest() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.auth.register(email: "test@example.com", password: "secret123")

        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/register")
        #expect(request.method.rawValue == "POST")

        let body = try #require(request.body)
        let payload = try #require(JSONSerialization.jsonObject(with: body) as? [String: String])
        #expect(payload["email"] == "test@example.com")
        #expect(payload["password"] == "secret123")
        #expect(lowercasedLookup(request.headers, "content-type") == "application/json")
    }

    @Test func loginGeneratesExpectedRequest() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.auth.login(email: "test@example.com", password: "secret123")

        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/login")
        #expect(request.method.rawValue == "POST")
    }

    @Test func meUsesAuthTokenWhenAvailable() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: [
            "id": "usr_1",
            "email": "test@example.com",
        ]))

        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore(accessToken: "jwt_abc", refreshToken: "refresh_abc")
        )

        _ = try await client.auth.me()

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "GET")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_abc")
    }

    @Test func refreshPostsRefreshTokenAndStoresNewTokens() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))

        let tokenStore = InMemoryTokenStore(accessToken: "jwt_old", refreshToken: "refresh_old")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )

        let response = try await client.auth.refresh()

        #expect(response.token == "jwt_stage3")
        #expect(response.refreshToken == "refresh_stage3")
        #expect(client.token == "jwt_stage3")
        #expect(client.refreshToken == "refresh_stage3")
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/refresh")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_old")
        let payload = try JSONSerialization.jsonObject(with: try #require(request.body), options: []) as? [String: String]
        #expect(payload?["refreshToken"] == "refresh_old")
    }

    @Test func logoutClearsTokensAndEmitsSignedOut() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_abc", refreshToken: "refresh_abc")
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: tokenStore
        )

        var emitted: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in emitted.append(event) }

        try await client.auth.logout()

        #expect(client.token == nil)
        #expect(client.refreshToken == nil)
        #expect(emitted == [.signedOut])
        let request = try #require(transport.requests.last)
        #expect(request.url.absoluteString == "https://api.example.com/api/auth/logout")
        #expect(request.method.rawValue == "POST")
        #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer jwt_abc")
    }

    @Test func missingRefreshTokenSkipsNetwork() async {
        let transport = MockTransport()
        let client = AYBClient(
            Stage3TestBootstrap.baseURL,
            transport: transport,
            tokenStore: InMemoryTokenStore()
        )

        do {
            _ = try await client.auth.refresh()
            Issue.record("refresh should fail without refresh token")
        } catch let error as AYBError {
            #expect(error.status == 400)
            #expect(error.code == "auth/missing-refresh-token")
            #expect(transport.requests.isEmpty)
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
    }

    @Test func tokenLifecycleForSignupAndSignout() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        transport.enqueue(StubResponse(status: 200, json: ["id": "usr_1", "email": "test@example.com"]))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        var sessionEvents: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in sessionEvents.append(event) }

        _ = try await client.auth.login(email: "test@example.com", password: "secret123")
        #expect(client.token == "jwt_stage3")

        try await client.auth.logout()
        // requests[1] is the logout request, which should carry the jwt_stage3 token acquired at login
        assertTransportRequestHasAuthorization(transport.requests[1], token: "jwt_stage3")
        #expect(client.token == nil)
        #expect(client.refreshToken == nil)
        #expect(sessionEvents == [.signedIn, .signedOut])

        _ = try await client.auth.me()
        let lastRequest = try #require(transport.requests.last)
        #expect(lowercasedLookup(lastRequest.headers, "Authorization") == nil)
    }

    @Test func signInEmitsSignedInAndRefreshEmitsTokenRefreshed() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let tokenStore = InMemoryTokenStore(accessToken: "jwt_init", refreshToken: "refresh_init")
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport, tokenStore: tokenStore)

        var events: [AuthStateEvent] = []
        _ = client.onAuthStateChange { event, _ in
            events.append(event)
        }

        _ = try await client.auth.login(email: "test@example.com", password: "secret")
        _ = try await client.auth.refresh()

        #expect(events == [.signedIn, .tokenRefreshed])
    }

    @Test func unsubscribeAuthListenerDuringEmit() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.authResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        var observed: [AuthStateEvent] = []
        var unsubscribe: (() -> Void)?

        unsubscribe = client.onAuthStateChange { event, _ in
            observed.append(event)
            unsubscribe?()
        }

        _ = try await client.auth.login(email: "test@example.com", password: "secret")

        #expect(observed == [.signedIn])
    }
}

private func assertTransportRequestHasAuthorization(_ request: HTTPRequest, token: String) {
    #expect(lowercasedLookup(request.headers, "Authorization") == "Bearer \(token)")
}
