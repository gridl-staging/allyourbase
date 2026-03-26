import Foundation

public struct AYBError: Error, LocalizedError {
    public let status: Int
    public let message: String
    public let code: String?
    public let data: [String: Any]?
    public let docUrl: String?

    public init(
        status: Int,
        message: String,
        code: String? = nil,
        data: [String: Any]? = nil,
        docUrl: String? = nil
    ) {
        self.status = status
        self.message = message
        self.code = code
        self.data = data
        self.docUrl = docUrl
    }

    public var errorDescription: String? {
        "AYBError(status: \(status), message: \(message), code: \(code ?? "n/a"))"
    }

    public static func from(response: HTTPResponse) -> AYBError {
        let body = AYBJSON.parse(response.body)
        var message = response.statusText
        var code: String?
        var data: [String: Any]?
        var docUrl: String?

        if let dictionary = body as? [String: Any] {
            if let rawMessage = dictionary["message"] as? String, !rawMessage.isEmpty {
                message = rawMessage
            }
            if let rawCode = dictionary["code"] {
                code = AYBError.stringCode(rawCode)
            }
            if let rawDoc = dictionary["doc_url"] as? String {
                docUrl = rawDoc
            } else if let rawDoc = dictionary["docUrl"] as? String {
                docUrl = rawDoc
            }
            if let rawData = dictionary["data"], !(rawData is NSNull) {
                data = rawData as? [String: Any]
            }
        }

        return AYBError(
            status: response.statusCode,
            message: message,
            code: code,
            data: data,
            docUrl: docUrl
        )
    }

    private static func stringCode(_ value: Any) -> String? {
        if let intValue = value as? Int {
            return String(intValue)
        }
        if let number = value as? NSNumber {
            return String(number.intValue)
        }
        if let string = value as? String, !string.isEmpty {
            return string
        }
        return nil
    }
}
