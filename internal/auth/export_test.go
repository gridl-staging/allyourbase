package auth

// HashTokenForTest exposes hashToken for integration tests so they don't
// reimplement the hashing logic and silently diverge if it changes.
var HashTokenForTest = hashToken

// GenerateTOTPCodeForTest exposes generateTOTPCode for integration tests.
var GenerateTOTPCodeForTest = generateTOTPCode

// TOTPPeriodForTest exposes totpPeriod for integration tests.
const TOTPPeriodForTest = totpPeriod

// AppTenantIDForTest exposes appTenantID for unit tests.
var AppTenantIDForTest = appTenantID
