import Foundation
import Testing
@testable import Allyourbase

struct ContractFixtureTests {
    @Test func authResponseFixtureDecodesFromCanonicalShapes() throws {
        let response = try AuthResponse.decode(ContractFixtures.authResponse)

        #expect(response.token == "jwt_stage3")
        #expect(response.refreshToken == "refresh_stage3")
        #expect(response.user.id == "usr_1")
        #expect(response.user.email == "dev@allyourbase.io")
        #expect(response.user.emailVerified == true)
        #expect(response.user.createdAt == "2026-01-01T00:00:00Z")
        #expect(response.user.updatedAt == nil)
    }

    @Test func listFixtureMetadataAndItemsPreserveOrder() throws {
        let response = try ListResponse.decode(ContractFixtures.listResponse) { item in
            item
        }

        #expect(response.metadata.totalItems == 2)
        let firstItem = try #require(response.items.first)
        #expect(firstItem["id"] as? String == "rec_1")
        let lastItem = try #require(response.items.last)
        #expect(lastItem["id"] as? String == "rec_2")
    }

    @Test func optionalHelpersFallThroughOnTypeMismatch() throws {
        // If "createdAt" exists with the wrong type, "created_at" should still be found
        let json: [String: Any] = [
            "id": "usr_2",
            "email": "test@example.com",
            "createdAt": 12345,
            "created_at": "2026-02-02T00:00:00Z",
            "emailVerified": "yes",
            "email_verified": true,
        ]
        let user = try User(from: json)
        #expect(user.createdAt == "2026-02-02T00:00:00Z")
        #expect(user.emailVerified == true)
    }

    @Test func recordFixtureAcceptsSnakeCaseMapping() throws {
        let fixture = ContractFixtures.recordPayload

        #expect(fixture["author_id"] as? Int == 1)

        let response: [String: Any] = [
            "items": [fixture],
            "page": 1,
            "perPage": 1,
            "totalItems": 1,
            "totalPages": 1,
        ]

        let list = try ListResponse.decode(response) { item in
            item
        }
        #expect(list.items[0]["created_at"] as? String == "2026-01-01T00:00:00Z")
    }

    @Test func storageObjectFixtureDecodesFromCanonicalShape() throws {
        let object = try StorageObject(from: ContractFixtures.storageObject)
        #expect(object.id == "file_abc123")
        #expect(object.bucket == "uploads")
        #expect(object.name == "document.pdf")
    }

    @Test func storageListFixtureDecodesNullVariantsFromCanonicalShape() throws {
        let response = try StorageListResponse(from: ContractFixtures.storageListResponse)
        #expect(response.totalItems == 2)
        #expect(response.items.count == 2)
        #expect(response.items[0].userId == "usr_1")
        #expect(response.items[0].updatedAt == nil)
        #expect(response.items[1].userId == nil)
        #expect(response.items[1].updatedAt == nil)
    }

    @Test func errorFixtureDecodesNumericCodeFromCanonicalShape() throws {
        let error = try decodeError(ContractFixtures.errorWithNumericCode, status: 403)
        #expect(error.status == 403)
        #expect(error.message == "forbidden")
        #expect(error.code == "403")
        #expect(error.docUrl == "https://allyourbase.io/docs/errors#forbidden")
        #expect(error.data?["resource"] as? String == "posts")
    }

    @Test func errorFixtureDecodesStringCodeFromCanonicalShape() throws {
        let error = try decodeError(ContractFixtures.errorWithStringCode)
        #expect(error.status == 400)
        #expect(error.message == "Missing refresh token")
        #expect(error.code == "auth/missing-refresh-token")
        #expect(error.data?["detail"] as? String == "refresh token not available")
    }

    @Test func realtimeEventFixtureDecodesFromCanonicalShape() throws {
        let event = try RealtimeEvent(from: ContractFixtures.realtimeEvent)
        #expect(event.action == "UPDATE")
        #expect(event.table == "posts")
        #expect(event.record["id"] as? String == "rec_1")
        let oldRecord = try #require(event.oldRecord)
        #expect(oldRecord["title"] as? String == "before")
    }

    private func decodeError(
        _ payload: [String: Any],
        status: Int = 400
    ) throws -> AYBError {
        let body = try JSONSerialization.data(withJSONObject: payload)
        let response = HTTPResponse(
            statusCode: status,
            statusText: "bad request",
            headers: [:],
            body: body
        )
        return AYBError.from(response: response)
    }
}
