package auth

// HashTokenForTest exposes hashToken for integration tests so they don't
// reimplement the hashing logic and silently diverge if it changes.
var HashTokenForTest = hashToken
