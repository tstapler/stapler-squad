---
globs: ["go.mod", "tools/bzl/supported-go.txt"]
---

# Changing go.mod's `go` or `toolchain` Line Needs the bazel-version-upgrades Skill

rules_go picks the Go SDK from `go.mod`, so editing the `go` or `toolchain` directive changes the compiler under every Bazel build, not just native `go`.

Before changing either line:

1. Load the `bazel-version-upgrades` skill and follow `docs/how-to/upgrade-bazel.md`.
2. Run the upgrade canary (`.github/workflows/bazel-upgrade-canary.yml`, manual dispatch) on the new Go version.
3. Only then add the version to `tools/bzl/supported-go.txt`; `bash tools/bzl/check-go-toolchain.sh` fails while `go.mod` names an unlisted version.

Adding or bumping a dependency in `go.mod` does not need this; use `make gazelle` as in the `bazel-stapler-squad` skill.
