import Foundation

enum ContractFixtures {
    nonisolated(unsafe) static let authResponse: [String: Any] = [
        "token": "jwt_stage3",
        "refreshToken": "refresh_stage3",
        "user": [
            "id": "usr_1",
            "email": "dev@allyourbase.io",
            "email_verified": true,
            "created_at": "2026-01-01T00:00:00Z",
            "updated_at": NSNull(),
        ],
    ]

    nonisolated(unsafe) static let recordPayload: [String: Any] = [
        "id": "rec_1",
        "title": "Hello",
        "author_id": 1,
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": NSNull(),
    ]

    nonisolated(unsafe) static let listResponse: [String: Any] = [
        "items": [
            [
                "id": "rec_1",
                "title": "First",
            ],
            [
                "id": "rec_2",
                "title": "Second",
            ],
        ],
        "page": 1,
        "perPage": 2,
        "totalItems": 2,
        "totalPages": 1,
    ]

    nonisolated(unsafe) static let errorWithNumericCode: [String: Any] = [
        "code": 403,
        "message": "forbidden",
        "data": [
            "resource": "posts"
        ],
        "doc_url": "https://allyourbase.io/docs/errors#forbidden",
    ]

    nonisolated(unsafe) static let storageObject: [String: Any] = [
        "id": "file_abc123",
        "bucket": "uploads",
        "name": "document.pdf",
        "size": 1024,
        "contentType": "application/pdf",
        "userId": "usr_1",
        "createdAt": "2026-01-01T00:00:00Z",
        "updatedAt": "2026-01-02T12:30:00Z",
    ]

    nonisolated(unsafe) static let storageListResponse: [String: Any] = [
        "items": [
            [
                "id": "file_1",
                "bucket": "uploads",
                "name": "doc1.pdf",
                "size": 1024,
                "contentType": "application/pdf",
                "userId": "usr_1",
                "createdAt": "2026-01-01T00:00:00Z",
                "updatedAt": NSNull(),
            ],
            [
                "id": "file_2",
                "bucket": "uploads",
                "name": "image.png",
                "size": 2048,
                "contentType": "image/png",
                "userId": NSNull(),
                "createdAt": "2026-01-02T00:00:00Z",
                "updatedAt": NSNull(),
            ],
        ],
        "totalItems": 2,
    ]

    nonisolated(unsafe) static let errorWithStringCode: [String: Any] = [
        "code": "auth/missing-refresh-token",
        "message": "Missing refresh token",
        "data": [
            "detail": "refresh token not available"
        ],
    ]

    nonisolated(unsafe) static let realtimeEvent: [String: Any] = [
        "action": "UPDATE",
        "table": "posts",
        "record": [
            "id": "rec_1",
            "title": "after",
        ],
        "oldRecord": [
            "id": "rec_1",
            "title": "before",
        ],
    ]
}
