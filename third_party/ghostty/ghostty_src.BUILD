package(default_visibility = ["//visibility:public"])

# Marker for the source root (the rule runs `zig build` from this file's directory).
exports_files(["build.zig"])

# macos/ and images/ hold the Xcode project and assets (some names contain spaces) that a
# wasm32-freestanding lib-vt build never reads.
filegroup(
    name = "srcs",
    srcs = glob(
        ["**"],
        exclude = [
            "macos/**",
            "images/**",
        ],
    ),
)
