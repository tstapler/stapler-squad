# Architecture Research: e2e-baseline-fail-fixes

**Item:** b21fb6da-77cb-4877-bf8b-3d82e7514ba5

---

## Q1 — keyring.MockInit() injection point

### Where in main.go

The `--test-mode` startup path lives at `main.go:174–186`:

```go
// Enable test mode if flag is set
if testModeFlag {                                   // line 175
    testDir := testDirFlag
    if testDir == "" {
        testDir = fmt.Sprintf("/tmp/stapler-squad-test-%d", os.Getpid())
    }
    if err := os.Setenv("STAPLER_SQUAD_TEST_DIR", testDir); err != nil {
        log.Warn("Failed to set STAPLER_SQUAD_TEST_DIR; ...")
    }
    log.Info("Test mode enabled: using isolated data directory", "dir", testDir)
}                                                   // line 186
```

The correct injection point is **inside this block, after line 185 (the log.Info call)**. `keyring.MockInit()` replaces the entire OS-keyring backend process-wide; calling it here — before `config.LoadConfig()` (line 189) and long before `BuildRuntimeDeps` constructs `jules.NewKeyringTokenSource()` (dependencies.go:1996) — guarantees no real D-Bus call ever fires.

`main.go` currently has **no** `"github.com/zalando/go-keyring"` import. It must be added. The package is already a transitive dependency (`jules/keychain.go` imports it), so no `go get` is needed.

**Recommended change** (extend the testModeFlag block):

```go
if testModeFlag {
    testDir := testDirFlag
    if testDir == "" {
        testDir = fmt.Sprintf("/tmp/stapler-squad-test-%d", os.Getpid())
    }
    if err := os.Setenv("STAPLER_SQUAD_TEST_DIR", testDir); err != nil {
        log.Warn("Failed to set STAPLER_SQUAD_TEST_DIR; test mode may use the wrong data directory", "err", err)
    }
    // Substitute in-memory keyring so D-Bus is never contacted on headless CI.
    keyring.MockInit()
    log.Info("Test mode enabled: using isolated data directory", "dir", testDir)
}
```

Import to add: `"github.com/zalando/go-keyring"`.

### Alternative injection point (server/dependencies.go)

A second option is to call `keyring.MockInit()` right before `jules.NewKeyringTokenSource()` at `server/dependencies.go:1996`, gated on `config.IsIsolatedInstance()`. This is less preferred because:
- It is later in the startup sequence than necessary.
- `server/dependencies.go` does not currently import `go-keyring` directly; adding it there adds coupling to a utility concern.
- It misses any keyring calls that could theoretically be added earlier in `BuildRuntimeDeps` in the future.

The `main.go` approach is preferred: it is the earliest possible point, is self-documenting, and is symmetrical with the existing `STAPLER_SQUAD_TEST_DIR` setup.

---

## Q2 — Other keychain usage in e2e test-mode startup path

`keyring.MockInit()` replaces the **process-global** OS-keyring backend. All three separate keychain subsystems are therefore covered by one call:

| Subsystem | File | Direct call | Covered? |
|---|---|---|---|
| Jules API key | `jules/keychain.go` | `keyring.Get/Set/Delete` via package-level var seams | Yes |
| GitHub token store | `github/keychain.go` | `keyring.Get/List/Delete` | Yes |
| SSH remote credentials | `session/sshremote/keystore.go` | `keyring.Get/Set/Delete` via `keyringMu` | Yes |

`server/services/credentials.go`'s `JulesCredentialSource` delegates to `jules.KeyringTokenSource` (not direct keyring calls); covered transitively.

`server/services/backlog_github_progress_sync.go` and `server/services/github_user_service.go` reference keychain in comments and in `ListKeychainAccounts()`, but those calls are on demand (during RPC handling), not at startup. They are covered by `MockInit()` being process-global.

No additional call sites need individual stubs.

---

## Q3 — route-focus-and-announce: build-type detection

### Current state (HEAD = c10cb7ebb)

PR #1385 ("remove Next.js and the WEB_BUILD dual-build seam") has **already removed** the failing tests:

- Deleted `tests/e2e/helpers/web-build.ts` (which contained `isBunBuild`)
- Removed the `"route focus and announcements (Next baseline)"` describe block from `tests/e2e/route-focus-and-announce.spec.ts`
- Removed the skip-link test `"skip link: Tab reaches it, Enter keeps the path and sets the hash"` that was asserting `mainFocused === false` (a Next-only fact)

The two baseline `fail` rows in `tests/e2e/baseline/next-e2e-baseline.json` are:

```
fail: route focus and announcements (Next baseline) > skip link: Tab reaches it, Enter keeps the path and sets the hash [chromium]
fail: route focus and announcements (Next baseline) > skip link: Tab reaches it, Enter keeps the path and sets the hash [chromium-dom]
```

These tests no longer exist in the spec. `scripts/e2e-baseline-compare.ts:382` handles this case:

```ts
// line 377–382
function removedFromRun(b: BaselineSpecEntry, ctx: GateContext): Finding | null {
  ...
  return { severity: "report", ... message: `baseline class '${b.class}' test is not in this run (deleted or renamed): reported, not a gate failure.` };
}
```

A `severity: "report"` finding does NOT fail the gate. The `removedFromRun()` path is taken for any baseline entry whose test title does not appear in the current run, which is exactly what happens here.

### What this means for the fix

No test-code changes are needed for route-focus-and-announce. The two baseline rows will transition from `fail` to "reported, not a gate failure" automatically once the baseline JSON is regenerated (AC-9). There is no `isBunBuild` guard to add to this spec.

No `isBunBuild` helper exists anywhere in `tests/e2e/helpers/` after PR #1385 (the file was deleted). If a future spec needs to gate on build type, the pattern would be an env-var check (`process.env.WEB_BUILD_TYPE === 'bun'`), but that is not needed here.

---

## Q4 — alias-settings AC-02 and AC-05 failure cause

### Test state machine and DB isolation

Each test has a `test.beforeEach` that calls `settings.goto()` → `page.goto("/settings?tab=general")`. This is a full page navigation, resetting all React component state (form open/closed, form values, error/success banners). The DB is NOT reset between tests (all tests share the same `--test-mode` server instance for the run). Prior test aliases ("e2e-create" from AC-01, etc.) remain in the DB but are not relevant to AC-02 since it uses a fresh alias name "e2e-edit".

The hypothesis that "AC-01 leaves the form open" is **false** — the full page navigation in `beforeEach` closes all form state.

### Confirmed failure: AC-04

Three label mismatches (VERIFIED against `AliasesManager.tsx` source):

| Test locator | Component renders | File:line |
|---|---|---|
| `getByLabel("Advanced options")` | `<span>Advanced</span>` (no "options") | `AliasesManager.tsx:588` |
| `getByRole("button", { name: "Add Variable" })` | button text `Add variable` (lowercase v) | `AliasesManager.tsx:632` |
| `getByLabel("Environment variable 1 key")` | `aria-label="Environment variable key 1"` (word order differs) | `AliasesManager.tsx:606` |

Fix: update the spec (or the component) to use the actual strings.

### AC-02 failure cause (INFERRED — requires test run to confirm)

AC-02 flow differs from passing tests (AC-01, AC-18) in that it performs an **edit** after creating:

1. `saveAlias()` (create "e2e-edit") — same as AC-01, which passes.
2. `clickEditAlias("e2e-edit")` — clicks the Edit button on the alias row.
3. `fillAliasDescription("Updated E2E")` — `page.getByLabel("Description").fill(...)`.
4. `saveAlias()` (edit save).

The requirements.md fix criteria lists "description field label" as one of the locators that "must match." The current `AliasesManager.tsx:419–422` has:

```jsx
<label className={labelClass} htmlFor="alias-desc">
  Description
</label>
<input id="alias-desc" ... />
```

This is a correct `htmlFor`/`id` association; `page.getByLabel("Description")` should resolve it. `ProfilesManager.tsx:272` also has a `<label htmlFor="profile-desc">Description</label>`, but that component's form is conditionally rendered (`useState(false)` initial state) and would not be in the DOM unless the user opens it.

**Most likely root cause**: the `SettingsPage.aliasFormSave()` selector uses `page.getByLabel(/^(New|Edit) Alias/)`. In "Edit" mode the section's `aria-labelledby` heading is `"Edit Alias: e2e-edit"`, not `"Edit Alias"`. Playwright's `getByLabel` with a regex pattern should match this (regex `/^(New|Edit) Alias/` matches `"Edit Alias: e2e-edit"`), but this is worth verifying against actual Playwright behavior with an `aria-labelledby` on a `<section>`. If the section is not found by `getByLabel`, the second `saveAlias()` times out and the test fails at the edit-save step, not at `fillAliasDescription`.

**Action**: Run the test in isolation with `--debug` to capture the exact assertion that times out.

### AC-05 failure cause (INFERRED — requires test run to confirm)

AC-05 flow:

1. `clickNewAlias()` — passes (same as AC-01).
2. `fillAliasName("e2e-tags")` — passes (same as AC-01).
3. `page.getByPlaceholder("Add a tag...").fill("e2e-tag")`.
4. `page.getByPlaceholder("Add a tag...").press("Enter")`.
5. `expect(page.getByText("e2e-tag")).toBeVisible()`.

`AliasesManager.tsx:569` renders `placeholder="Add a tag..."` only inside `{showForm && ...}`. With the alias form open and no other form open, there is exactly one "Add a tag..." input. Steps 3–4 should succeed.

The most likely failure is step 5. The tag chip renders as:

```jsx
<span>
  {t}
  <button aria-label="Remove tag e2e-tag">x</button>
</span>
```

The span's normalized text content is `"e2e-tagx"` (tag text concatenated with the remove button's "x"). If Playwright's `getByText("e2e-tag")` uses exact matching (the default in Playwright 1.56), it would not match `"e2e-tagx"`. The test would then time out waiting for an element with exact text "e2e-tag" that never appears.

Additionally, the form preview hint renders `Preview: @e2e-tags` which does not contain the bare string "e2e-tag" at the character-boundary level unless Playwright does substring matching.

**Action**: Run the test with `--debug` to confirm the step and locator that fails. Fix: scope the locator (`page.getByText("e2e-tag", { exact: false })` or use a test-id on the tag chip, e.g. `data-testid="tag-chip-e2e-tag"`).

---

## Tech-debt disposition

| Fix area | Recommended disposition |
|---|---|
| `main.go` keyring mock | **Extend as-is** — a one-line `keyring.MockInit()` call in the existing `testModeFlag` block. No seam refactoring required; the package-level mock IS the seam. |
| `alias-settings` locators (AC-04 label mismatches) | **Extend as-is** — update the three strings in the spec or the component to match. No structural change needed. |
| `alias-settings` AC-02 and AC-05 | **Extend as-is** after confirming the exact failing assertion. If the `aliasFormSave()` selector is the issue, update `SettingsPage.ts`; if the tag chip text is the issue, add a `data-testid` on the chip and update the spec. |
| `route-focus-and-announce` | **No change needed** — tests removed by PR #1385; baseline compare already handles deleted tests gracefully. Update baseline JSON in AC-9 sweep. |
| Jules keychain circuit-breaker (10 jules-dispatch rows) | **Extend as-is** — `keyring.MockInit()` in `main.go` stubs the OS keychain; the circuit breaker never fires because the mock never hangs. |
