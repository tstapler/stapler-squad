# Build vs Buy: E2E Keychain Stub

Researched: 2026-11-05  
Branch: tstapler/triage-b21fb6da-77cb-4877-bf8b-3d82e7514ba5

---

## 1. `keyring.MockInit()` (buy) vs custom in-memory stub (build)

**What MockInit() does** (verified via `keyring_mock.go`):

```go
func MockInit() {
    provider = &mockProvider{}
}
```

It replaces the package-level `provider` global with an in-memory `mockProvider` backed by
`map[string]map[string]string`. The map is keyed first by service name, then by account name. All
three service namespaces used in the codebase (`"stapler-squad"`, `"stapler-squad-jules"`,
`"stapler-squad-ssh"`) coexist safely in the same map — different string keys, no collisions.

**Completeness.** `MockInit()` covers all four operations the codebase uses: `Get`, `Set`,
`Delete`, `DeleteAll`. `jules/keychain.go` only calls `keyringGet`/`keyringSet`/`keyringDelete`,
all three of which are thin wrappers over the same provider interface. Nothing MockInit() cannot
satisfy.

**Existing precedent.** `github/test_hermetic.go` already calls `keyring.MockInit()` in a
package-level `init()` for all test binaries that import the `github` package. The comment at
line 33 reads: "Tests that need tokens call `keyring.MockInit()` themselves, which replaces this
empty store." The pattern is already established; the e2e server startup is the one gap.

**Maintenance.** Zero maintenance overhead — `MockInit()` is part of go-keyring's public API and
is covered by its own test file (`keyring_mock_test.go`). It won't drift from the production
`Get`/`Set`/`Delete` signatures.

**License.** MIT. No restriction on use in test infrastructure.

**Decision: buy.** Call `keyring.MockInit()` in `main.go` immediately after the
`if testModeFlag { ... }` block (around line 174). A custom in-memory stub would duplicate the
exact implementation already in the library and add a maintenance surface with zero benefit.

---

## 2. D-Bus Secret Service in CI vs in-memory stub at startup

Installing and running a real Secret Service daemon (`gnome-keyring-daemon --daemonize
--components=secrets`) in a CI Docker container is technically feasible but carries meaningful
operational cost:

- Requires `dbus-launch` or a D-Bus session socket before the daemon can start. On a headless
  container, `DBUS_SESSION_BUS_ADDRESS` is unset; wiring it into the test runner entrypoint adds
  fragile shell setup.
- `gnome-keyring-daemon` prompts to unlock its keystore on first use unless the keystore is
  pre-seeded or the `--unlock` flag is passed with an empty password — another setup step.
- The daemon must be alive for the full test run; a crash or early exit causes all subsequent
  keyring reads to hang until `defaultKeyringTimeout` (5 s) fires, then the circuit breaker opens
  for 5 min — producing false failures or long timeouts in affected tests.
- None of this complexity buys anything: the e2e tests do not need real credential persistence;
  they need the keychain call to not hang.

**Decision: in-memory stub.** `keyring.MockInit()` eliminates the D-Bus dependency entirely with
one line of code. Installing a real Secret Service daemon in CI is strictly dominated by this
option.

---

## 3. Environment variable fallback: `JULES_API_KEY=e2e-stub-key`

The credential resolution chain in `server/services/credentials.go` places
`EnvVarCredentialSource` first (line 118), ahead of `JulesCredentialSource`. `EnvVarCredentialSource.Resolve`
checks `os.Getenv("JULES_API_KEY")` for provider `"jules"` (line 182). If that env var is set,
the keychain is never consulted for Jules credentials.

**Feasibility:** Setting `JULES_API_KEY=e2e-stub-key` in the test server's environment (e.g. via
`PORT=... JULES_API_KEY=e2e-stub-key ./stapler-squad --test-mode`) would suppress keychain
access for the Jules credential source specifically. It would not, however, suppress the GitHub
token keychain read (`github/keychain.go`, service `"stapler-squad"`) or the SSH remote keystore
(`session/sshremote/keystore.go`, service `"stapler-squad-ssh"`), both of which also call
go-keyring on Linux and can hang on a D-Bus socket that is absent in CI.

**Gap:** The env var approach is Jules-only. It leaves two other keychain call sites exposed.
`keyring.MockInit()` stubs the global provider, covering all three namespaces in one call.

**Additional concern:** The Jules feature is disabled by default in test mode unless
`cfg.Jules.Enabled` is true (dependencies.go line 2008). With the feature disabled, `julesKeys.APIKey()`
is never called at startup (line 2011 guard), so the keychain hang that env var would prevent
does not actually occur for Jules in a standard e2e run. The D-Bus hang risk in CI comes from
the GitHub token source (`github/http_client.go` calls `keyring.Get("stapler-squad", ...)` on
every token cache miss) and the SSH keystore — neither of which `JULES_API_KEY` affects.

**Decision: insufficient.** The env var approach is too narrow. `keyring.MockInit()` is the
right fix.

---

## 4. `keyring.MockInit()` design: in-memory or disk? Cross-domain conflicts?

**In-memory, per-process.** The `mockProvider` struct holds a plain Go map (`map[string]map[string]string`).
No file I/O, no D-Bus socket, no IPC. The store is scoped to the process lifetime and starts
empty on every `MockInit()` call. Source: `keyring_mock.go` lines 3–65 (verified above).

**Conflict risk.** The global `provider` variable is package-level in `go-keyring`. Calling
`MockInit()` once replaces it process-wide; every subsequent `keyring.Get/Set/Delete` call in
that process hits the in-memory map regardless of which package made the call. This is the
intended behavior — it is exactly what `github/test_hermetic.go` relies on.

The three service namespaces in this binary are:
- `"stapler-squad"` — GitHub token (`github/keychain.go`)
- `"stapler-squad-jules"` — Jules API key (`jules/keychain.go`)
- `"stapler-squad-ssh"` — SSH remote credentials (`session/sshremote/keystore.go`)

After `MockInit()`, each starts empty and independent. A `keyring.Get("stapler-squad-jules", "api-key")`
call returns `ErrNotFound`, which `jules/keychain.go` translates to `ErrJulesNotConfigured` — the
same result as "Jules not configured," causing the Jules feature to degrade gracefully (logged at
Info, feature skipped), which is the correct behavior for e2e tests that don't set up a Jules key.

**Risk of double MockInit().** `github/test_hermetic.go`'s `init()` only runs in test binaries
(`testing.Testing() == true`). The e2e server is a production binary started by
`global-setup.ts` with `--test-mode`; `testing.Testing()` returns false there, so the `init()`
in `github/test_hermetic.go` does not fire. The `MockInit()` call in `main.go` would be the
only one.

**Verdict: safe.** One `MockInit()` call in `main.go` under `if testModeFlag` replaces the
provider for all three credential domains simultaneously, starts them all empty, and introduces
no conflicts between domains.

---

## Summary recommendation

Call `keyring.MockInit()` in `main.go` immediately after line 185 (`log.Info("Test mode enabled: ...")`),
still inside the `if testModeFlag` block. This:

1. Eliminates the D-Bus Secret Service dependency for all three keychain namespaces in one line.
2. Uses a library-supplied, MIT-licensed, already-tested implementation — zero custom code to maintain.
3. Leaves Jules in `ErrJulesNotConfigured` (graceful degradation) rather than a 5 s timeout that
   opens a circuit breaker and pollutes test logs.
4. Matches the existing precedent in `github/test_hermetic.go`.

No other approach (D-Bus daemon, `JULES_API_KEY` env var, custom stub) is competitive on
implementation cost, coverage, or operational safety.
