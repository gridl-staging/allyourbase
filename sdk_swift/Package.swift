// swift-tools-version: 6.0
import PackageDescription

let developerFrameworksPath = "/Library/Developer/CommandLineTools/Library/Developer/Frameworks"
let testingPluginPath = "/Library/Developer/CommandLineTools/usr/lib/swift/host/plugins/testing"

let package = Package(
    name: "Allyourbase",
    platforms: [.macOS(.v14), .iOS(.v16)],
    products: [
        .library(name: "Allyourbase", targets: ["Allyourbase"]),
        .executable(name: "AllyourbaseTestRunner", targets: ["AllyourbaseTestRunner"]),
    ],
    targets: [
        .target(
            name: "Allyourbase",
            path: "Sources/Allyourbase"
        ),
        // Standalone executable test runner using Swift Testing.
        // Run via: swift run AllyourbaseTestRunner
        // (xctest bundle runner not needed; works in CLT-only environments)
        .executableTarget(
            name: "AllyourbaseTestRunner",
            dependencies: ["Allyourbase"],
            path: "Tests/AllyourbaseTests",
            swiftSettings: [
                .unsafeFlags([
                    "-F", developerFrameworksPath,
                    "-Xfrontend", "-disable-cross-import-overlays",
                    "-load-plugin-library", "\(testingPluginPath)/libTestingMacros.dylib",
                ]),
            ],
            linkerSettings: [
                .unsafeFlags([
                    "-F", developerFrameworksPath,
                    "-framework", "Testing",
                    "-Xlinker", "-rpath",
                    "-Xlinker", developerFrameworksPath,
                ]),
            ]
        ),
    ]
)
