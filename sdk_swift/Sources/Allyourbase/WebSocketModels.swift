import Foundation

public enum WebSocketClientMessageType: String {
    case auth
    case subscribe
    case unsubscribe
    case channelSubscribe = "channel_subscribe"
    case channelUnsubscribe = "channel_unsubscribe"
    case broadcast
    case presenceTrack = "presence_track"
    case presenceUntrack = "presence_untrack"
    case presenceSync = "presence_sync"
}

public enum WebSocketServerMessageType: String {
    case connected
    case reply
    case event
    case broadcast
    case presence
    case error
    case system
}

public struct WebSocketClientMessage {
    public let type: WebSocketClientMessageType
    public let ref: String?
    public let token: String?
    public let tables: [String]?
    public let filter: String?
    public let channel: String?
    public let event: String?
    public let payload: [String: Any]?
    public let selfBroadcast: Bool?
    public let presence: [String: Any]?

    public init(
        type: WebSocketClientMessageType,
        ref: String? = nil,
        token: String? = nil,
        tables: [String]? = nil,
        filter: String? = nil,
        channel: String? = nil,
        event: String? = nil,
        payload: [String: Any]? = nil,
        selfBroadcast: Bool? = nil,
        presence: [String: Any]? = nil
    ) {
        self.type = type
        self.ref = ref
        self.token = token
        self.tables = tables
        self.filter = filter
        self.channel = channel
        self.event = event
        self.payload = payload
        self.selfBroadcast = selfBroadcast
        self.presence = presence
    }

    public func toDictionary() -> [String: Any] {
        var dictionary: [String: Any] = ["type": type.rawValue]
        if let ref, !ref.isEmpty {
            dictionary["ref"] = ref
        }
        if let token, !token.isEmpty {
            dictionary["token"] = token
        }
        if let tables, !tables.isEmpty {
            dictionary["tables"] = tables
        }
        if let filter, !filter.isEmpty {
            dictionary["filter"] = filter
        }
        if let channel, !channel.isEmpty {
            dictionary["channel"] = channel
        }
        if let event, !event.isEmpty {
            dictionary["event"] = event
        }
        if let payload {
            dictionary["payload"] = payload
        }
        if let selfBroadcast {
            dictionary["self"] = selfBroadcast
        }
        if let presence {
            dictionary["presence"] = presence
        }
        return dictionary
    }
}

public struct WebSocketServerMessage: @unchecked Sendable {
    public let type: WebSocketServerMessageType
    public let ref: String?
    public let clientId: String?
    public let status: String?
    public let message: String?
    public let action: String?
    public let table: String?
    public let record: [String: Any]?
    public let oldRecord: [String: Any]?
    public let channel: String?
    public let event: String?
    public let payload: [String: Any]?
    public let presenceAction: String?
    public let presence: [String: Any]?
    public let presences: [String: [String: Any]]?
    public let presenceConnId: String?

    public init(from json: [String: Any]) throws {
        let typeRaw = try AYBJSON.requiredString(json, ["type"], "WebSocketServerMessage.type")
        guard let type = WebSocketServerMessageType(rawValue: typeRaw) else {
            throw AYBDecodingError.invalidType("WebSocketServerMessage.type=\(typeRaw)")
        }
        self.type = type
        self.ref = AYBJSON.optionalString(json, ["ref"])
        self.clientId = AYBJSON.optionalString(json, ["client_id", "clientId"])
        self.status = AYBJSON.optionalString(json, ["status"])
        self.message = AYBJSON.optionalString(json, ["message"])
        self.action = AYBJSON.optionalString(json, ["action"])
        self.table = AYBJSON.optionalString(json, ["table"])
        self.record = AYBJSON.optionalDictionary(json, "record")
        self.oldRecord = AYBJSON.optionalDictionary(json, "oldRecord") ?? AYBJSON.optionalDictionary(json, "old_record")
        self.channel = AYBJSON.optionalString(json, ["channel"])
        self.event = AYBJSON.optionalString(json, ["event"])
        self.payload = AYBJSON.optionalDictionary(json, "payload")
        self.presenceAction = AYBJSON.optionalString(json, ["presence_action", "presenceAction"])
        self.presence = AYBJSON.optionalDictionary(json, "presence")
        self.presences = WebSocketServerMessage.decodePresences(json["presences"])
        self.presenceConnId = AYBJSON.optionalString(json, ["presence_conn_id", "presenceConnId"])
    }

    private static func decodePresences(_ raw: Any?) -> [String: [String: Any]]? {
        guard let raw, !(raw is NSNull), let dictionary = raw as? [String: Any] else {
            return nil
        }
        var result: [String: [String: Any]] = [:]
        for (key, value) in dictionary {
            if let nested = value as? [String: Any] {
                result[key] = nested
            }
        }
        return result
    }
}
