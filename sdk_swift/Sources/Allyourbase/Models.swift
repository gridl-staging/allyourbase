import Foundation

public enum AYBDecodingError: Error {
    case missingField(String)
    case invalidType(String)
}

public enum AYBJSON {
    public static func parse(_ data: Data) -> Any? {
        guard !data.isEmpty else {
            return nil
        }

        return (try? JSONSerialization.jsonObject(with: data, options: []))
    }

    public static func expectDictionary(_ value: Any?, _ context: String) throws -> [String: Any] {
        guard let dictionary = value as? [String: Any] else {
            throw AYBDecodingError.invalidType(context)
        }
        return dictionary
    }

    public static func expectArray(_ value: Any?, _ context: String) throws -> [Any] {
        guard let array = value as? [Any] else {
            throw AYBDecodingError.invalidType(context)
        }
        return array
    }

    public static func requiredString(_ dictionary: [String: Any], _ keys: [String], _ context: String) throws -> String {
        for key in keys {
            if let value = dictionary[key], !(value is NSNull) {
                if let string = value as? String {
                    return string
                }
            }
        }
        throw AYBDecodingError.missingField("\(context): missing \(keys.joined(separator: ", "))")
    }

    public static func optionalString(_ dictionary: [String: Any], _ keys: [String]) -> String? {
        for key in keys {
            guard let value = dictionary[key], !(value is NSNull) else {
                continue
            }
            if let string = value as? String {
                return string
            }
        }
        return nil
    }

    public static func optionalBool(_ dictionary: [String: Any], _ keys: [String]) -> Bool? {
        for key in keys {
            guard let value = dictionary[key], !(value is NSNull) else {
                continue
            }
            if let bool = value as? Bool {
                return bool
            }
        }
        return nil
    }

    public static func requiredInt(_ dictionary: [String: Any], _ keys: [String]) throws -> Int {
        for key in keys {
            guard let value = dictionary[key], !(value is NSNull) else {
                continue
            }
            if let int = value as? Int {
                return int
            }
            if let number = value as? NSNumber {
                return number.intValue
            }
        }
        throw AYBDecodingError.missingField("\(keys.joined(separator: ", "))")
    }

    public static func requiredDictionary(_ dictionary: [String: Any], _ key: String) throws -> [String: Any] {
        guard let raw = dictionary[key], !(raw is NSNull) else {
            throw AYBDecodingError.missingField("\(key)")
        }
        guard let object = raw as? [String: Any] else {
            throw AYBDecodingError.invalidType("\(key)")
        }
        return object
    }

    public static func optionalDictionary(_ dictionary: [String: Any], _ key: String) -> [String: Any]? {
        guard let raw = dictionary[key], !(raw is NSNull) else {
            return nil
        }
        return raw as? [String: Any]
    }

    public static func optionalAny(_ dictionary: [String: Any], _ key: String) -> Any? {
        guard let value = dictionary[key], !(value is NSNull) else {
            return nil
        }
        return value
    }
}

private enum QueryItemFactory {
    static func string(_ name: String, _ value: String?) -> URLQueryItem? {
        value.map { URLQueryItem(name: name, value: $0) }
    }

    static func int(_ name: String, _ value: Int?) -> URLQueryItem? {
        value.map { URLQueryItem(name: name, value: String($0)) }
    }

    static func trueOnly(_ name: String, _ value: Bool?) -> URLQueryItem? {
        guard value == true else {
            return nil
        }
        return URLQueryItem(name: name, value: "true")
    }
}

public struct ListParams {
    public let page: Int?
    public let perPage: Int?
    public let sort: String?
    public let filter: String?
    public let search: String?
    public let fields: String?
    public let expand: String?
    public let skipTotal: Bool?

    public init(
        page: Int? = nil,
        perPage: Int? = nil,
        sort: String? = nil,
        filter: String? = nil,
        search: String? = nil,
        fields: String? = nil,
        expand: String? = nil,
        skipTotal: Bool? = nil
    ) {
        self.page = page
        self.perPage = perPage
        self.sort = sort
        self.filter = filter
        self.search = search
        self.fields = fields
        self.expand = expand
        self.skipTotal = skipTotal
    }

    public func toQueryItems() -> [URLQueryItem] {
        return [
            QueryItemFactory.int("page", page),
            QueryItemFactory.int("perPage", perPage),
            QueryItemFactory.string("sort", sort),
            QueryItemFactory.string("filter", filter),
            QueryItemFactory.string("search", search),
            QueryItemFactory.string("fields", fields),
            QueryItemFactory.string("expand", expand),
            QueryItemFactory.trueOnly("skipTotal", skipTotal),
        ].compactMap { $0 }
    }
}

public struct GetParams {
    public let fields: String?
    public let expand: String?

    public init(fields: String? = nil, expand: String? = nil) {
        self.fields = fields
        self.expand = expand
    }

    public func toQueryItems() -> [URLQueryItem] {
        return [
            QueryItemFactory.string("fields", fields),
            QueryItemFactory.string("expand", expand),
        ].compactMap { $0 }
    }
}

public struct AuthResponse {
    public let token: String
    public let refreshToken: String
    public let user: User

    public init(token: String, refreshToken: String, user: User) {
        self.token = token
        self.refreshToken = refreshToken
        self.user = user
    }

    public init(from json: [String: Any]) throws {
        self.token = try AYBJSON.requiredString(json, ["token"], "token")
        self.refreshToken = try AYBJSON.requiredString(json, ["refreshToken"], "refreshToken")
        self.user = try User(from: try AYBJSON.requiredDictionary(json, "user"))
    }

    public static func decode(_ json: Any) throws -> AuthResponse {
        return try AuthResponse(from: try AYBJSON.expectDictionary(json, "AuthResponse"))
    }
}

public struct User {
    public let id: String
    public let email: String
    public let emailVerified: Bool?
    public let createdAt: String?
    public let updatedAt: String?

    public init(
        id: String,
        email: String,
        emailVerified: Bool? = nil,
        createdAt: String? = nil,
        updatedAt: String? = nil
    ) {
        self.id = id
        self.email = email
        self.emailVerified = emailVerified
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    public init(from json: [String: Any]) throws {
        self.id = try AYBJSON.requiredString(json, ["id", "userId", "user_id"], "User.id")
        self.email = try AYBJSON.requiredString(json, ["email", "emailAddress", "email_address"], "User.email")
        self.emailVerified = AYBJSON.optionalBool(json, ["emailVerified", "email_verified"]) 
        self.createdAt = AYBJSON.optionalString(json, ["createdAt", "created_at", "created"])
        self.updatedAt = AYBJSON.optionalString(json, ["updatedAt", "updated_at", "updated"])
    }
}

public struct ListMetadata {
    public let page: Int
    public let perPage: Int
    public let totalItems: Int
    public let totalPages: Int

    public init(page: Int, perPage: Int, totalItems: Int, totalPages: Int) {
        self.page = page
        self.perPage = perPage
        self.totalItems = totalItems
        self.totalPages = totalPages
    }
}

public struct ListResponse<T> {
    public let items: [T]
    public let metadata: ListMetadata

    public init(items: [T], page: Int, perPage: Int, totalItems: Int, totalPages: Int) {
        self.items = items
        self.metadata = ListMetadata(
            page: page,
            perPage: perPage,
            totalItems: totalItems,
            totalPages: totalPages,
        )
    }

    public static func decode(_ json: Any, decodeItem: ([String: Any]) throws -> T) throws -> ListResponse<T> {
        let dictionary = try AYBJSON.expectDictionary(json, "ListResponse")
        let itemsRaw = try AYBJSON.expectArray(dictionary["items"], "ListResponse.items")

        let parsedItems: [T] = try itemsRaw.map { itemRaw in
            let item = try AYBJSON.expectDictionary(itemRaw, "ListResponse.item")
            return try decodeItem(item)
        }

        return ListResponse(
            items: parsedItems,
            page: try AYBJSON.requiredInt(dictionary, ["page"]),
            perPage: try AYBJSON.requiredInt(dictionary, ["perPage"]),
            totalItems: try AYBJSON.requiredInt(dictionary, ["totalItems"]),
            totalPages: try AYBJSON.requiredInt(dictionary, ["totalPages"])
        )
    }
}

public struct BatchOperation {
    public let method: String
    public let id: String?
    public let body: [String: Any]?

    public init(method: String, id: String? = nil, body: [String: Any]? = nil) {
        self.method = method
        self.id = id
        self.body = body
    }

    public func toDictionary() -> [String: Any] {
        var payload: [String: Any] = ["method": method]
        if let id {
            payload["id"] = id
        }
        if let body {
            payload["body"] = body
        }
        return payload
    }
}

public struct BatchResult<T> {
    public let index: Int
    public let status: Int
    public let body: T?

    public init(index: Int, status: Int, body: T?) {
        self.index = index
        self.status = status
        self.body = body
    }

    public static func decode(_ json: Any, decodeBody: ([String: Any]?) throws -> T?) throws -> BatchResult<T> {
        let dictionary = try AYBJSON.expectDictionary(json, "BatchResult")
        return BatchResult(
            index: try AYBJSON.requiredInt(dictionary, ["index"]),
            status: try AYBJSON.requiredInt(dictionary, ["status"]),
            body: try decodeBody(AYBJSON.optionalDictionary(dictionary, "body"))
        )
    }
}

public struct StorageObject {
    public let id: String
    public let bucket: String
    public let name: String
    public let size: Int
    public let contentType: String
    public let userId: String?
    public let createdAt: String?
    public let updatedAt: String?

    public init(
        id: String,
        bucket: String,
        name: String,
        size: Int,
        contentType: String,
        userId: String? = nil,
        createdAt: String? = nil,
        updatedAt: String? = nil
    ) {
        self.id = id
        self.bucket = bucket
        self.name = name
        self.size = size
        self.contentType = contentType
        self.userId = userId
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    public init(from json: [String: Any]) throws {
        self.id = try AYBJSON.requiredString(json, ["id"], "StorageObject.id")
        self.bucket = try AYBJSON.requiredString(json, ["bucket"], "StorageObject.bucket")
        self.name = try AYBJSON.requiredString(json, ["name"], "StorageObject.name")
        self.size = try AYBJSON.requiredInt(json, ["size"])
        self.contentType = try AYBJSON.requiredString(json, ["contentType", "content_type"], "StorageObject.contentType")
        self.userId = AYBJSON.optionalString(json, ["userId", "user_id"])
        self.createdAt = AYBJSON.optionalString(json, ["createdAt", "created_at"])
        self.updatedAt = AYBJSON.optionalString(json, ["updatedAt", "updated_at"])
    }
}

public struct StorageListResponse {
    public let items: [StorageObject]
    public let totalItems: Int

    public init(items: [StorageObject], totalItems: Int) {
        self.items = items
        self.totalItems = totalItems
    }

    public init(from json: [String: Any]) throws {
        let itemsArray = try AYBJSON.expectArray(json["items"], "StorageListResponse.items")
        self.items = try itemsArray.map { item in
            try StorageObject(from: try AYBJSON.expectDictionary(item, "StorageObject"))
        }
        self.totalItems = try AYBJSON.requiredInt(json, ["totalItems"])
    }
}

public struct RealtimeEvent {
    public let action: String
    public let table: String
    public let record: [String: Any]
    public let oldRecord: [String: Any]?

    public init(
        action: String,
        table: String,
        record: [String: Any],
        oldRecord: [String: Any]? = nil
    ) {
        self.action = action
        self.table = table
        self.record = record
        self.oldRecord = oldRecord
    }

    public init(from json: [String: Any]) throws {
        self.action = try AYBJSON.requiredString(json, ["action"], "RealtimeEvent.action")
        self.table = try AYBJSON.requiredString(json, ["table"], "RealtimeEvent.table")
        self.record = try AYBJSON.requiredDictionary(json, "record")
        self.oldRecord = AYBJSON.optionalDictionary(json, "oldRecord") ?? AYBJSON.optionalDictionary(json, "old_record")
    }
}

public struct RealtimeOptions {
    public let maxReconnectAttempts: Int
    public let reconnectDelays: [TimeInterval]
    public let jitterMax: TimeInterval

    public init(
        maxReconnectAttempts: Int = 5,
        reconnectDelays: [TimeInterval] = [0.25, 0.5, 1.0, 2.0, 4.0],
        jitterMax: TimeInterval = 0.1
    ) {
        self.maxReconnectAttempts = max(0, maxReconnectAttempts)
        self.reconnectDelays = reconnectDelays.isEmpty ? [0.25] : reconnectDelays.map { max(0, $0) }
        self.jitterMax = max(0, jitterMax)
    }
}
