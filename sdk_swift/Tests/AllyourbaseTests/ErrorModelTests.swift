import Foundation
import Testing
@testable import Allyourbase

struct ErrorModelTests {
    @Test func parsesNumericErrorCode() {
        let response = HTTPResponse(
            statusCode: 403,
            statusText: "forbidden",
            headers: ["content-type": "application/json"],
            body: dataFromJSON(ContractFixtures.errorWithNumericCode)
        )

        let error = AYBError.from(response: response)

        #expect(error.status == 403)
        #expect(error.message == "forbidden")
        #expect(error.code == "403")
        #expect(error.data != nil)
        #expect(error.docUrl == "https://allyourbase.io/docs/errors#forbidden")
    }

    @Test func parsesStringErrorCodeWhenNumericUnavailable() {
        let response = HTTPResponse(
            statusCode: 400,
            statusText: "bad request",
            headers: ["content-type": "application/json"],
            body: dataFromJSON(ContractFixtures.errorWithStringCode)
        )

        let error = AYBError.from(response: response)

        #expect(error.status == 400)
        #expect(error.message == "Missing refresh token")
        #expect(error.code == "auth/missing-refresh-token")
        #expect(error.data != nil)
    }

    @Test func fallbackErrorMessageForNonJsonBody() {
        let response = HTTPResponse(
            statusCode: 502,
            statusText: "bad gateway",
            headers: ["content-type": "text/html"],
            body: Data("<html>bad gateway</html>".utf8)
        )

        let error = AYBError.from(response: response)

        #expect(error.status == 502)
        #expect(error.message == "bad gateway")
        #expect(error.code == nil)
        #expect(error.data == nil)
    }
}
