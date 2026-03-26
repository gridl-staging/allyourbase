import Testing

// Entry point for standalone test runner (used in CLT-only environments without xctest).
// Swift Testing discovers @Test functions at compile time; __swiftPMEntryPoint runs them.
await Testing.__swiftPMEntryPoint() as Never
