import Foundation
import Testing
@testable import Allyourbase

@Suite
struct StorageIntegrationTests {
    @Test
    func uploadCreatesValidMultipartRequest() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 200,
                json: [
                    "id": "obj123",
                    "bucket": "test-bucket",
                    "name": "hello.txt",
                    "size": 11,
                    "contentType": "text/plain",
                    "userId": "user123",
                    "createdAt": "2023-01-01T00:00:00Z",
                    "updatedAt": "2023-01-01T00:00:00Z",
                ]
            )
        )

        let client = AYBClient("https://test.example.com", transport: transport)
        client.setTokens("test_token", refreshToken: "refresh")
        let testData = Data("Hello World".utf8)

        let result = try await client.storage.upload(
            bucket: "test-bucket",
            data: testData,
            name: "hello.txt",
            contentType: "text/plain"
        )

        let request = try #require(transport.requests.first)
        #expect(request.method == .post)
        #expect(request.url.path == "/api/storage/test-bucket")
        #expect(lowercasedLookup(request.headers, "authorization") == "Bearer test_token")
        #expect(lowercasedLookup(request.headers, "content-type")?.hasPrefix("multipart/form-data; boundary=") == true)

        let bodyData = try #require(request.body)
        let bodyString = String(decoding: bodyData, as: UTF8.self)
        #expect(bodyString.contains("Content-Disposition: form-data; name=\"file\"; filename=\"hello.txt\""))
        #expect(bodyString.contains("Content-Type: text/plain"))
        #expect(bodyString.contains("Hello World"))

        #expect(result.id == "obj123")
        #expect(result.name == "hello.txt")
        #expect(result.size == 11)
        #expect(result.contentType == "text/plain")
        #expect(result.userId == "user123")
    }

    @Test
    func downloadUrlBuildsCorrectAbsoluteURL() {
        let client = AYBClient("https://test.example.com")
        let url = client.storage.downloadUrl(bucket: "my-bucket", name: "file.pdf")
        #expect(url == "https://test.example.com/api/storage/my-bucket/file.pdf")
    }

    @Test
    func deleteMakesValidRequest() async throws {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 204, json: NSNull()))
        let client = AYBClient("https://test.example.com", transport: transport)
        client.setTokens("test_token", refreshToken: "refresh")

        try await client.storage.delete(bucket: "test-bucket", name: "test-file.txt")

        let request = try #require(transport.requests.first)
        #expect(request.method == .delete)
        #expect(request.url.path == "/api/storage/test-bucket/test-file.txt")
        #expect(lowercasedLookup(request.headers, "authorization") == "Bearer test_token")
    }

    @Test
    func listMakesValidRequestWithQueryParams() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 200,
                json: [
                    "items": [
                        [
                            "id": "obj1",
                            "bucket": "test-bucket",
                            "name": "file1.txt",
                            "size": 100,
                            "contentType": "text/plain",
                            "userId": "user1",
                            "createdAt": "2023-01-01T00:00:00Z",
                            "updatedAt": "2023-01-01T00:00:00Z",
                        ]
                    ],
                    "totalItems": 1,
                ]
            )
        )

        let client = AYBClient("https://test.example.com", transport: transport)
        client.setTokens("test_token", refreshToken: "refresh")

        let result = try await client.storage.list(
            bucket: "test-bucket",
            prefix: "files/",
            limit: 10,
            offset: 5
        )

        let request = try #require(transport.requests.first)
        #expect(request.method == .get)
        #expect(request.url.path == "/api/storage/test-bucket")
        let components = try #require(URLComponents(url: request.url, resolvingAgainstBaseURL: false))
        let queryItems = components.queryItems ?? []
        #expect(queryItems.first(where: { $0.name == "prefix" })?.value == "files/")
        #expect(queryItems.first(where: { $0.name == "limit" })?.value == "10")
        #expect(queryItems.first(where: { $0.name == "offset" })?.value == "5")

        #expect(result.items.count == 1)
        #expect(result.items[0].name == "file1.txt")
        #expect(result.totalItems == 1)
    }

    @Test
    func getSignedUrlConstructsWithBaseURLPrepend() async throws {
        let transport = MockTransport()
        transport.enqueue(
            StubResponse(
                status: 200,
                json: [
                    "url": "/api/storage/test-bucket/test/file?exp=12345&sig=abcd1234",
                ]
            )
        )

        let client = AYBClient("https://test.example.com", transport: transport)
        client.setTokens("test_token", refreshToken: "refresh")

        let signedUrl = try await client.storage.getSignedUrl(
            bucket: "test-bucket",
            name: "test/file",
            expiresIn: 7200
        )

        let request = try #require(transport.requests.first)
        #expect(request.method == .post)
        #expect(request.url.path == "/api/storage/test-bucket/test/file/sign")
        #expect(signedUrl == "https://test.example.com/api/storage/test-bucket/test/file?exp=12345&sig=abcd1234")

        let bodyData = try #require(request.body)
        let bodyDict = try #require(try JSONSerialization.jsonObject(with: bodyData) as? [String: Any])
        #expect(bodyDict["expiresIn"] as? Int == 7200)
    }

    @Test
    func storageOperationsErrorOnNon2xxResponse() async {
        let transport = MockTransport()
        transport.enqueue(StubResponse(status: 404, json: ["message": "Not Found"]))
        let client = AYBClient("https://test.example.com", transport: transport)

        do {
            try await client.storage.delete(bucket: "test-bucket", name: "nonexistent")
            Issue.record("expected delete to throw")
        } catch let error as AYBError {
            #expect(error.status == 404)
        } catch {
            Issue.record("unexpected error type: \(error)")
        }
    }
}
