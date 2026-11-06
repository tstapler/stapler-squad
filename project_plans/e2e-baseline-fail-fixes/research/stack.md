# Stack Research — e2e baseline fail fixes

## 1. Playwright version and OS keychain stubbing

**Version:** `@playwright/test: ^1.40.0` (from `tests/e2e/package.json`).

**Keychain stubbing approach for e2e:** The Go binary is a real process launched by `global-setup.ts`; Playwright cannot reach inside it. The `keyringGet`/`keyringSet`/`keyringDelete` package-level vars in `jules/keychain.go` are the seam, but they are unexported — only in-process Go unit tests can reassign them. For the e2e server (`--test-mode`), the right approach is to call `keyring.MockInit()` (see section 2 below) at startup when the server is in isolated/test mode, before `jules.NewKeyringTokenSource()` is constructed in `server/dependencies.go:1996`. Concretely: in `main.go`, after setting `STAPLER_SQUAD_TEST_DIR` (line 182), or in `server/dependencies.go`'s `BuildCoreDependencies` where `config.IsIsolatedInstance()` is already checked (line 1112), call `keyring.MockInit()` from `github.com/zalando/go-keyring`. This swaps the global provider to an in-memory map, eliminating all D-Bus / `org.freedesktop.secrets` calls for the duration of that process. After `MockInit()`, any `SetJulesAPIKey` RPC from a Playwright test will write to memory and be readable immediately.

## 2. Go keyring library

**Library:** `github.com/zalando/go-keyring v0.2.8` (`go.mod:51`).

**In-memory test mode:** Yes — the library ships `keyring.MockInit()` and `keyring.MockInitWithError(err error)` as exported functions in `keyring_mock.go` (in-package, no build tag needed):

```go
// MockInit sets the provider to a mocked memory store
func MockInit() {
    provider = &mockProvider{}
}

// MockInitWithError sets the provider to a mocked memory store that returns
// the given error on all operations
func MockInitWithError(err error) {
    provider = &mockProvider{mockError: err}
}
```

No build tag is required. Calling `keyring.MockInit()` before any `Get`/`Set`/`Delete` call redirects all operations to an in-memory `map[string]map[string]string`. There is no env var in the library itself — the call must be made programmatically by the Go binary at startup.

## 3. playwright.config.ts project structure

Defined in `tests/e2e/playwright.config.ts`. Projects and their spec assignments:

| Project | Spec assignment |
|---|---|
| `visual-matrix`, `visual-cyberpunk77`, `visual-wh40k`, `visual-clean` | `testMatch: **/visual-regression.spec.ts` only |
| `chromium` | All `*.spec.ts` **except** `mobile-webkit.spec.ts`, `perf/**`, `dev-mode-hmr.spec.ts`, `website-demos.spec.ts`, and optionally `terminal-stress/*.spec.ts` when `E2E_SKIP_STRESS=1` |
| `chromium-dom` | Same as `chromium` but also excludes `worker-server-isolation.spec.ts`; uses `--disable-webgl` for DOM renderer path |
| `desktop-firefox`, `desktop-webkit` | `testMatch: **/files-shortcuts.spec.ts` only |
| `mobile-chromium` | `testMatch: **/files-mobile*.spec.ts`, `**/files-large.spec.ts`, `**/files-lazydoc-spike.spec.ts`, `**/files-transport-spike.spec.ts` |
| `mobile-webkit` | Same mobile specs; only registered when `PW_WEBKIT=1` |
| `perf` | `**/perf/**/*.spec.ts`; only registered when `PW_PERF=1` or `--project=perf` |
| `webkit-mobile` | `**/mobile-webkit.spec.ts`; only registered when WebKit binary is installed |

Key constant: `DEV_SERVER_SPECS = '**/dev-mode-hmr.spec.ts'` is excluded from every standard project. `dev-mode-hmr.spec.ts` only runs under the separate `playwright.dev-mode.config.ts`, which is not part of the normal `make ci` / `bun run test` invocation.

**Consequence for failing specs:**
- `alias-settings.spec.ts`, `jules-dispatch.spec.ts`, `local-file-browser.spec.ts`, `route-focus-and-announce.spec.ts` all run under `chromium` and `chromium-dom`.
- `dev-mode-hmr.spec.ts` is already excluded from every standard project — its 6 baseline rows should be in a separate baseline file or excluded from the comparison gate.
- `visual-regression.spec.ts` runs under the four visual projects only, not under `chromium`.

## 4. `--test-mode` / `--test-dir` flag behavior and keychain injection path

From `main.go:69,174-186,907-908`:

```
rootCmd.Flags().BoolVar(&testModeFlag, "test-mode", false, "Run in test mode with isolated data directory")
rootCmd.Flags().StringVar(&testDirFlag, "test-dir", "", "Custom test data directory (defaults to /tmp/stapler-squad-test-<PID>)")
```

When `--test-mode` is set, the server calls `os.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)` before loading config. This makes `config.IsIsolatedInstance()` return `true` (it checks `os.Getenv("STAPLER_SQUAD_TEST_DIR") != ""`). The `IsTestMode()` function only detects `go test` binaries (by inspecting `os.Args`) and returns `false` for the real binary even when `--test-mode` is passed — isolation is entirely via `STAPLER_SQUAD_TEST_DIR`.

**There is no existing flag or env var to inject a custom keyring implementation.** The `keyringGet/keyringSet/keyringDelete` vars are package-level in `jules/keychain.go` (unexported) and are only reassignable by tests in the same package (`keychain_test.go:28-37`). For e2e, the fix is:

**Option A (recommended):** In `main.go`, after `os.Setenv("STAPLER_SQUAD_TEST_DIR", ...)` (line 182), add:
```go
if testModeFlag {
    keyring.MockInit() // in-memory keyring; no D-Bus on headless Linux
}
```
This requires importing `github.com/zalando/go-keyring` in `main.go`.

**Option B:** In `server/dependencies.go` where `config.IsIsolatedInstance()` is already checked (around line 1112), call `keyring.MockInit()` earlier, before `jules.NewKeyringTokenSource()` at line 1996. The `IsIsolatedInstance()` guard there is already the right gate.

**Option C:** Expose the seam via a new env var checked at binary startup, e.g. `STAPLER_SQUAD_JULES_KEYRING_MOCK=1`, to allow selective activation without coupling to `--test-mode`. This is more flexible but adds a new env var surface.

The `jules-dispatch.spec.ts` tests (the `SetJulesAPIKey` RPC → 500 path) need Option A or B in place before the Playwright tests attempt to call `UpdateJulesConfig`/`SetJulesAPIKey` through the Connect API.
