import Foundation
import Testing
@testable import Allyourbase

struct QuerySerializationTests {
    @Test func requestBuilderSortsDictionaryQueryItems() throws {
        let builder = RequestBuilder(baseURL: URL(string: "https://api.example.com")!)
        let url = try builder.makeURL(path: "/api/test", query: ["z": "9", "a": "1", "m": "5"])

        #expect(url.query == "a=1&m=5&z=9")
    }

    @Test func listParamsDeterministicSerialization() {
        let params = ListParams(
            page: 2,
            perPage: 10,
            sort: "-created_at",
            filter: "published=true",
            search: "swift",
            fields: "id,title",
            expand: "author",
            skipTotal: true
        )

        let query = params.toQueryItems().map { "\($0.name)=\($0.value ?? "")" }

        #expect(
            query == [
                "page=2",
                "perPage=10",
                "sort=-created_at",
                "filter=published=true",
                "search=swift",
                "fields=id,title",
                "expand=author",
                "skipTotal=true",
            ]
        )
    }

    @Test func getParamsDeterministicSerialization() {
        let params = GetParams(fields: "id,title", expand: "author")
        #expect(
            params.toQueryItems().map { "\($0.name)=\($0.value ?? "")" } == ["fields=id,title", "expand=author"]
        )
    }

    @Test func recordsListEncodesExpectedQueryAndPath() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 200, json: ContractFixtures.listResponse))

        let client = AYBClient(Stage3TestBootstrap.baseURL, transport: transport)

        _ = try await client.records.list(
            "posts",
            params: ListParams(
                page: 2,
                perPage: 10,
                sort: "-created_at",
                filter: "published=true",
                search: "swift",
                fields: "id,title",
                expand: "author",
                skipTotal: true
            )
        )

        let request = try #require(transport.requests.first)
        #expect(request.url.path == "/api/collections/posts")
        #expect(
            request.url.query == "page=2&perPage=10&sort=-created_at&filter=published%3Dtrue&search=swift&fields=id,title&expand=author&skipTotal=true"
        )
    }
}
