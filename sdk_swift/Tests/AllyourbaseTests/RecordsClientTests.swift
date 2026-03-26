import Foundation
import Testing
@testable import Allyourbase

struct RecordsClientTests {
    @Test func listMethodAndQueryParameters() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.listResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.records.list(
            "posts",
            params: ListParams(
                page: 1,
                perPage: 20,
                sort: "-created_at",
                filter: "published=true",
                search: "swift",
                fields: "id,title",
                expand: "author",
                skipTotal: true
            )
        )

        let request = try #require(transport.requests.last)
        #expect(request.url.path == "/api/collections/posts")
        #expect(request.url.query == "page=1&perPage=20&sort=-created_at&filter=published%3Dtrue&search=swift&fields=id,title&expand=author&skipTotal=true")
        #expect(lowercasedLookup(request.headers, "Authorization") == nil)
    }

    @Test func getReturnsRecordAndDecodesWithoutAuthWhenMissingToken() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.recordPayload))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let record = try await client.records.get(
            "posts",
            "rec-1",
            params: GetParams(fields: "id,title", expand: "author")
        )

        #expect(record["id"] as? String == "rec_1")
        let request = try #require(transport.requests.last)
        #expect(request.url.path == "/api/collections/posts/rec-1")
        #expect(request.url.query == "fields=id,title&expand=author")
        #expect(lowercasedLookup(request.headers, "Authorization") == nil)
    }

    @Test func createRecordPostsJSONBodyAndReturnsRecord() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 201, json: ContractFixtures.recordPayload))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let response = try await client.records.create("posts", data: ["title": "Hello"])

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "POST")
        #expect(request.url.path == "/api/collections/posts")
        #expect(lowercasedLookup(request.headers, "content-type") == "application/json")

        let body = try #require(request.body)
        let decoded = try JSONSerialization.jsonObject(with: body, options: []) as? [String: Any]
        #expect(decoded?["title"] as? String == "Hello")
        #expect(response["id"] as? String == "rec_1")
    }

    @Test func updateRecordUsesPatchAndReturnsUpdatedRecord() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.recordPayload))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.records.update("posts", id: "rec-1", data: ["title": "Updated"])

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "PATCH")
        #expect(request.url.path == "/api/collections/posts/rec-1")
    }

    @Test func deleteRecordUsesDeleteAndNoBody() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        try await client.records.delete("posts", id: "rec-1")

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "DELETE")
        #expect(request.url.path == "/api/collections/posts/rec-1")
        #expect(request.body == nil)
    }

    @Test func batchPostsOperationsAndParsesResults() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 200,
                json: [
                    ["index": 0, "status": 201, "body": ["id": "rec_1", "title": "A"]],
                    ["index": 1, "status": 200, "body": ["id": "rec_2", "title": "B"]],
                    ["index": 2, "status": 204, "body": NSNull()],
                ]
            )
        )

        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)
        let results = try await client.records.batch(
            "posts",
            operations: [
                BatchOperation(method: "create", body: ["title": "A"]),
                BatchOperation(method: "update", id: "rec_2", body: ["title": "B"]),
                BatchOperation(method: "delete", id: "rec_3"),
            ]
        )

        let request = try #require(transport.requests.last)
        #expect(request.method.rawValue == "POST")
        #expect(request.url.path == "/api/collections/posts/batch")

        let payload = try #require(request.body)
        let body = try JSONSerialization.jsonObject(with: payload, options: []) as? [String: Any]
        let operations = body?["operations"] as? [[String: Any]]
        #expect(operations?.count == 3)
        #expect(operations?[1]["id"] as? String == "rec_2")

        #expect(results.count == 3)
        #expect(results[0].status == 201)
        #expect(results[2].body == nil)
        #expect(results[0].body?["id"] as? String == "rec_1")
    }

    @Test func listDecodeErrorsAreSurfacedAsAYBError() async {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 401,
                json: ContractFixtures.errorWithStringCode
            )
        )
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        do {
            _ = try await client.records.list("posts")
            Issue.record("expected failure")
        } catch let error as AYBError {
            #expect(error.status == 401)
            #expect(error.code == "auth/missing-refresh-token")
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
    }

    @Test func listResponseMetadataAndItemsDecoded() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.listResponse))
        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        let response = try await client.records.list("posts")

        #expect(response.metadata.page == 1)
        #expect(response.metadata.perPage == 2)
        #expect(response.items.count == 2)
        #expect(response.items[0]["id"] as? String == "rec_1")
    }
}
