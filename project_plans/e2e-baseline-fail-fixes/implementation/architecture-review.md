# Architecture Review: e2e-baseline-fail-fixes
**Date**: 2026-11-05
**Verdict**: CONCERNS (0 blockers, 1 concern)

## Blockers
_(none)_

## Concerns

- [ ] **main.go — direct go-keyring import for test-mode** — The plan adds `keyring.MockInit()` directly in `main.go`'s `if testModeFlag` block, which requires `main.go` to import `github.com/zalando/go-keyring` for a test-only operation. `jules/keychain.go` already owns the go-keyring coupling for the Jules subsystem; placing the call in `main.go` scatters that ownership upward. **Recommendation:** add a thin `jules.InitTestMode()` (or `jules.MockKeychain()`) function that calls `keyring.MockInit()` internally, then call that from `main.go`. This keeps the go-keyring import contained in the package that already uses it, and `main.go` only imports `jules` (which it already does for the keychain-dependent startup path).

## Nitpicks

- **Build tag is not the right guard here.** The plan correctly uses a runtime flag (`--test-mode`) rather than a `//go:build e2e` tag. The codebase's own precedent (`github/test_hermetic.go` lines 29–37) uses a runtime `testing.Testing()` guard with no build tag, so a build-tag approach would be inconsistent. No change needed.

- **`github/test_hermetic.go` pattern confirms "test code in production binary" is accepted.** `test_hermetic.go` is not a `_test.go` file; it ships in the production binary and gates on `hermeticInTests()` at runtime. The plan's pattern is structurally identical. Not a new anti-pattern.

- **No ADR-000 constitution found.** `docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo; no higher-level constraint to check.

## Evidence

- `main.go:174–186` — existing `if testModeFlag` block; calls only `os.Setenv`, no library imports for test-mode.
- `github/test_hermetic.go:29–37` — `init()` with `keyring.MockInit()`, runtime-guarded, no build tag, compiles into the production binary (confirmed: not `_test.go`).
- `github/main_test.go:26–32` — `TestMain` calls `keyring.MockInit()` before the test suite; standard `testing.M` pattern.
- `jules/keychain.go:10` — `go-keyring` already imported in the `jules` package; the right delegation target.
