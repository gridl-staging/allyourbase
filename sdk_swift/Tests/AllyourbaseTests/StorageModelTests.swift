import Foundation
import Testing
@testable import Allyourbase

@Suite("StorageModelTests")
struct StorageModelTests {

    @Test("StorageObject decodes from canonical shape")
    func storageObjectDecodesFromCanonicalShape() throws {
        let data = try dataFromJSON(ContractFixtures.storageObject)
        let json = try AYBJSON.expectDictionary(AYBJSON.parse(data), "StorageObject")

        let obj = try StorageObject(from: json)

        #expect(obj.id == "file_abc123")
        #expect(obj.bucket == "uploads")
        #expect(obj.name == "document.pdf")
        #expect(obj.size == 1024)
        #expect(obj.contentType == "application/pdf")
        #expect(obj.userId == "usr_1")
        #expect(obj.createdAt == "2026-01-01T00:00:00Z")
        #expect(obj.updatedAt == "2026-01-02T12:30:00Z")
    }

    @Test("StorageObject accepts null userId and updatedAt")
    func storageObjectAcceptsNullFields() throws {
        let payload: [String: Any] = [
            "id": "file_xyz",
            "bucket": "public",
            "name": "test.txt",
            "size": 100,
            "contentType": "text/plain",
            "userId": NSNull(),
            "createdAt": "2026-01-01T00:00:00Z",
            "updatedAt": NSNull(),
        ]

        let data = try dataFromJSON(payload)
        let json = try AYBJSON.expectDictionary(AYBJSON.parse(data), "StorageObject")

        let obj = try StorageObject(from: json)

        #expect(obj.id == "file_xyz")
        #expect(obj.userId == nil)
        #expect(obj.updatedAt == nil)
    }

    @Test("StorageListResponse decodes items and totalItems")
    func storageListResponseDecodesFromCanonicalShape() throws {
        let data = try dataFromJSON(ContractFixtures.storageListResponse)
        let json = try AYBJSON.expectDictionary(AYBJSON.parse(data), "StorageListResponse")

        let response = try StorageListResponse(from: json)

        #expect(response.totalItems == 2)
        #expect(response.items.count == 2)

        let first = response.items[0]
        #expect(first.id == "file_1")
        #expect(first.name == "doc1.pdf")
        #expect(first.size == 1024)
        #expect(first.userId == "usr_1")

        let second = response.items[1]
        #expect(second.id == "file_2")
        #expect(second.name == "image.png")
        #expect(second.size == 2048)
        #expect(second.userId == nil)
    }
}
