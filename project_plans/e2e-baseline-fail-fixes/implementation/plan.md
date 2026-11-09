# Implementation Plan: e2e-baseline-fail-fixes

**Item:** b21fb6da-77cb-4877-bf8b-3d82e7514ba5  
**Epic count:** 1  
**Story count:** 5  
**Task count:** 14  

## Effort Estimate

| Phase | Stories | Cost Class | Agent Waves | Wall-clock Blockers |
|---|---|---|---|---|
| 1 — Keychain stub | 1.1 | XS (1 file edit, 1 import, verify build) | 1 | Build + e2e run (~8 min) |
| 2 — Alias-settings fixes | 2.1–2.2 | S (1 spec file, 4 locator strings, 1 investigation run) | 1 | Playwright run (~3 min) |
| 3 — Symlink + baseline update | 3.1–3.2 | S (1 investigation + 1 JSON edit, possibly 1 server handler) | 2 (investigate then fix) | Playwright run + jq audit |
| 4 — CI gate | 4.1 | XS (`make quick-check` + e2e run) | 1 | CI run (~12 min) |

Total: ~2 agent waves of parallel-eligible work + 1 sequential investigation gate.  
Overrun checkpoint: if Phase 3 investigation finds a non-trivial server fix is needed (e.g. `filepath.EvalSymlinks` in a file-browser handler), that task may grow to M.

## Success Metrics (PM gate)

**Before**: 26 entries classified `class: "fail"` for the 5 target spec files in `next-e2e-baseline.json`.  
**After**: 0 `class: "fail"` entries for those spec files (20 changed to `"pass"`, 2 removed as `removedFromRun`). Visual-regression rows remain fail and are excluded from this count.  
**Target user**: contributors running `make ci` or the Playwright e2e suite against a Next build.

## Non-functional Requirements (Engineering gate)

- **Thread-safety**: `keyring.MockInit()` replaces the global provider with a plain map. `go-keyring`'s mock implementation (`keyring_mock.go`) uses a mutex on `mockStore` internally — no additional synchronization needed in `main.go`. Confirm with `make test-race` after Phase 1.
- **Route-focus Bun CI**: The two skip-link tests (`lines 65 and 130` of `route-focus-and-announce.spec.ts`) are confirmed deleted in PR #1385. No `isBunBuild` guard is needed — those test names no longer exist in the spec file. The `removedFromRun` classification (severity "report", not gate failure) covers the transition period.
- **jules.InitTestMode() facade**: Architecture review recommends wrapping `keyring.MockInit()` in a `jules.InitTestMode()` shim to keep the keyring import within the `jules/` package. This is a concern, not a blocker. Implementation team should evaluate at code-writing time — both approaches are correct; the facade is preferred for maintainability.

---

## Creative Pass — Approaches Considered

| Approach | Verdict | Reason |
|---|---|---|
| **A. `keyring.MockInit()` at `--test-mode` startup + test locator fixes** | **CHOSEN** | Zero new abstractions. Leverages the library's own test-mode API. Pattern already proven in `github/main_test.go:27`. One call in `main.go` stubs every downstream keychain user (jules, github, ssh). |
| **B. `JULES_API_KEY` env var short-circuit** | Rejected | Only stubs `jules/keychain.go`; leaves `github/keychain.go` and `session/sshremote/keystore.go` exposed to D-Bus on headless Linux. Too narrow. |
| **C. Install gnome-keyring-daemon in CI** | Rejected | Requires dbus-launch, keystore unlock, stable daemon process. High operational cost for what MockInit gives for free. |

---

## Ubiquitous Language

| Term | Definition |
|---|---|
| **TestMode** | Binary startup mode activated by `--test-mode` flag; sets `STAPLER_SQUAD_TEST_DIR`, enabling `config.IsIsolatedInstance()`. |
| **KeychainStub** | In-memory keyring provider installed by `keyring.MockInit()`; replaces the global `go-keyring` provider with a plain `map[string]map[string]string`, no D-Bus. |
| **BaselineRow** | One `{file, project, title, class}` entry in `tests/e2e/baseline/next-e2e-baseline.json`. A row with `class: "fail"` is a known-failing spec from the last Next build. |
| **BunExpectedDiff** | `tests/e2e/baseline/bun-expected-diff.json`; lists Bun-only expected deviations. Currently has `"entries": []`. Rule F6 gates on a listed entry that now passes without being removed. |
| **removedFromRun** | Compare-script classification for a baseline entry whose test no longer exists in the current run; severity `"report"`, not a gate failure. |

---

## Technology and Pattern Decisions

1. **`keyring.MockInit()` is the canonical test-mode keyring seam** (VERIFIED). The library (`github.com/zalando/go-keyring v0.2.8`) exports `MockInit()` which replaces the process-global provider. Used in `github/main_test.go:27` and `github/keychain_test.go:32,51,101`. No new abstraction needed.

2. **Injection point: `main.go` testModeFlag block, before `config.LoadConfig()`** (VERIFIED). Lines 174–186 set `STAPLER_SQUAD_TEST_DIR`. Adding `keyring.MockInit()` here (line 186, before the closing brace) guarantees it fires before any downstream dependency is wired — `BuildRuntimeDeps` reaches `jules.NewKeyringTokenSource()` far later.

3. **Test locator fixes go in the spec, not the component** (VERIFIED). Component labels are WCAG-conformant: `"Advanced"` (component line 588), `"Add variable"` (component line 632), `"Environment variable key 1"` (component line 606), `"Environment variable value 1"` (component line 614). The spec has stale strings from an older draft.

4. **AC-04 has 4 locator mismatches, not 3** (VERIFIED by reading `AliasesManager.tsx` lines 583–634). The research listed 3; a 4th mismatch exists on the value input aria-label. All 4 are fixed together in Story 2.

5. **route-focus skip-link rows are already dead code** (INFERRED from research citing PR #1385 / `c10cb7ebb`). The two baseline entries reference `"route focus and announcements (Next baseline) > skip link: Tab reaches…"` which no longer exists in `tests/e2e/route-focus-and-announce.spec.ts` (confirmed: file has no `skip-link` test at this commit). They will be `removedFromRun` (severity `"report"`) without any spec change. Only the baseline JSON needs updating.

6. **local-file-browser symlink test root cause is INFERRED** as a path-resolution difference (symlink vs real path in the path bar). It is not keychain-related. The plan includes an investigation step before committing to either assertion update or production fix.

---

## ADR Stubs

No novel architectural decisions are introduced. `keyring.MockInit()` follows an established pattern and is scoped entirely to `--test-mode` startup.

---

## Phase 1 — Keychain Stub in `main.go`

**Risk:** Highest value, enables fixes for 12 of the 34 target rows.  
**Scope:** One file, three lines (import + call).

### Story 1.1 — Add `keyring.MockInit()` to TestMode startup

**Given** a `stapler-squad` process launched with `--test-mode` on headless Linux (no D-Bus Secret Service)  
**When** any RPC handler calls into `jules/keychain.go`'s `keyringGet`/`keyringSet`/`keyringDelete`  
**Then** those calls hit the in-memory mock provider, return immediately (no D-Bus), and the 5-minute circuit breaker in `jules/keychain.go:defaultIdentityTimeout` is never opened.

#### Task 1.1.1 — Add import and `keyring.MockInit()` call in `main.go`

File: `main.go`

Change: in the `testModeFlag` block (lines 174–186), after the `log.Info("Test mode enabled…")` line (currently line 185), add:

```go
keyring.MockInit()
```

Add import `"github.com/zalando/go-keyring"` to the import block.

The block after the change:

```go
if testModeFlag {
    testDir := testDirFlag
    if testDir == "" {
        testDir = fmt.Sprintf("/tmp/stapler-squad-test-%d", os.Getpid())
    }
    if err := os.Setenv("STAPLER_SQUAD_TEST_DIR", testDir); err != nil {
        log.Warn("Failed to set STAPLER_SQUAD_TEST_DIR; test mode may use the wrong data directory", "err", err)
    }
    log.Info("Test mode enabled: using isolated data directory", "dir", testDir)
    keyring.MockInit() // stub OS keychain; prevents D-Bus hang on headless Linux
}
```

**Verify:** `go build .` succeeds. No new test file needed — the in-process hermetic coverage already exists in `github/keychain_test.go`.

#### Task 1.1.2 — Confirm jules-dispatch 10 rows pass

After building with Task 1.1.1, run:
```bash
go build . && cd tests/e2e && npx playwright test jules-dispatch.spec.ts --project=chromium
```

Expected: all 5 `jules-dispatch gating` tests pass. That accounts for 10 baseline rows (chromium + chromium-dom).

#### Task 1.1.3 — Confirm local-file-browser "open terminal here" passes

```bash
cd tests/e2e && npx playwright test local-file-browser.spec.ts -g "open terminal here" --project=chromium
```

Expected: test passes. Accounts for 2 of the 4 local-file-browser baseline rows.

---

## Phase 2 — alias-settings Spec Locator Fixes

**Risk:** Low — component source is authoritative; all 4 mismatches are confirmed by reading `AliasesManager.tsx`.

### Story 2.1 — Fix AC-04 locator mismatches

**Given** `alias-settings.spec.ts` AC-04 test running against the test server  
**When** the test attempts to expand the Advanced section and add an env var  
**Then** all 4 locators resolve to their target elements and the test passes without timeout.

#### Task 2.1.1 — Apply 4 locator fixes in `tests/e2e/alias-settings.spec.ts`

File: `tests/e2e/alias-settings.spec.ts`, test `"AC-04 add environment variable via Advanced section"` (lines 58–67).

| Line | Old | New | Source |
|---|---|---|---|
| 61 | `page.getByLabel("Advanced options").check()` | `page.getByLabel("Advanced").check()` | `AliasesManager.tsx:588` — `<span>Advanced</span>` inside `<label>` |
| 62 | `getByRole("button", { name: "Add Variable" })` | `getByRole("button", { name: "Add variable" })` | `AliasesManager.tsx:632` — `Add variable` (lowercase v) |
| 63 | `getByLabel("Environment variable 1 key")` | `getByLabel("Environment variable key 1")` | `AliasesManager.tsx:606` — `aria-label={\`Environment variable key ${i + 1}\`}` |
| 64 | `getByLabel("Environment variable 1 value")` | `getByLabel("Environment variable value 1")` | `AliasesManager.tsx:614` — `aria-label={\`Environment variable value ${i + 1}\`}` |

#### Task 2.1.2 — Run AC-04 and confirm it passes

```bash
cd tests/e2e && npx playwright test alias-settings.spec.ts -g "AC-04" --project=chromium
```

Expected: test passes. Accounts for 2 of the 6 alias-settings baseline rows.

### Story 2.2 — Investigate and resolve AC-02 and AC-05

**Given** the Phase 1 and 2.1 fixes are in place  
**When** AC-02 (edit alias description) and AC-05 (add tag via Enter) run  
**Then** they should pass as-is; if either fails, the root cause is diagnosed from the Playwright trace and fixed.

The research classifies AC-02 and AC-05 as INFERRED failures. Prior code analysis shows:
- AC-02: form uses `aria-labelledby="alias-form-title"` → `"Edit Alias: e2e-edit"` heading. `SettingsPage.aliasFormSave()` uses `getByLabel(/^(New|Edit) Alias/)` which matches — no locator issue apparent. The `Description` input has `id="alias-desc"` with `htmlFor="alias-desc"` label. Test infrastructure looks sound.
- AC-05: `getByText("e2e-tag")` with Playwright's default `exact: false` (substring match) finds `"e2e-tagx"` span. Research confirms AC-05 passes as written.

#### Task 2.2.1 — Run AC-02 and AC-05 in full to confirm pass or reveal failure

```bash
cd tests/e2e && npx playwright test alias-settings.spec.ts -g "AC-02|AC-05" --project=chromium --reporter=html
```

If either test fails, open the HTML report and trace to identify the failing locator or assertion. Fix the specific locator; do not restructure the test.

**Given-When-Then for AC-02:**  
Given alias "e2e-edit" is created and the Edit form is open  
When `page.getByLabel("Description").fill("Updated E2E")` and `saveAlias()` are called  
Then `alias-row-e2e-edit` contains text "Updated E2E"

**Given-When-Then for AC-05:**  
Given the New Alias form is open with name "e2e-tags"  
When `page.getByPlaceholder("Add a tag...").fill("e2e-tag")` and `.press("Enter")` are called  
Then `page.getByText("e2e-tag")` resolves to a visible element (substring match finds the tag chip)

---

## Phase 3 — Symlink Navigation Fix and Baseline Update

### Story 3.1 — Resolve local-file-browser symlink navigation failure

**Given** the test navigates to `/files/?path=<rootDir>` and clicks `link-to-subdir/`  
**When** the file browser navigates into the symlinked directory  
**Then** `file-browser-path-input` has value `<real-subdir>` (the resolved symlink target)

The root cause is INFERRED: the server may return the symlink path (`link-to-subdir`) rather than the resolved real path (`real-subdir`). Two fix paths:

#### Task 3.1.1 — Reproduce and diagnose the symlink navigation failure

```bash
cd tests/e2e && npx playwright test local-file-browser.spec.ts -g "navigates into a symlinked directory" --project=chromium --reporter=html
```

If the test fails:
- Check the actual value of `file-browser-path-input` in the trace
- If value is the symlink path (`link-to-subdir`): locate the server-side `ReadDir`/`StatDir` handler (likely in `server/services/` or `session/`) and add `filepath.EvalSymlinks()` before constructing the response path, OR update the assertion to match the symlink path

If the test passes (keychain was blocking an earlier call that caused the page to fail to load): no code change needed.

#### Task 3.1.2 — Apply the chosen fix (symlinks resolved or assertion updated)

Prefer the server-side fix if the production behavior is clearly wrong (users expect navigation into a symlink to show the real path in the path bar). Use assertion update only if symlink-path-as-displayed is intentional product behavior.

### Story 3.2 — Update `next-e2e-baseline.json`

**Given** the specs above now pass in the Playwright run  
**When** the baseline compare script runs  
**Then** the baseline entries for those specs should reflect `class: "pass"` (not `"fail"`), and the route-focus skip-link entries that reference deleted tests should be removed.

#### Task 3.2.1 — Update baseline entries for now-passing specs

**Pre-mortem P1 gate**: Only mark `local-file-browser.spec.ts` entries as `"pass"` after Task 1.1.3 (open-terminal test) and Task 3.1.1 (symlink test) have confirmed passing in an actual Playwright run. Do NOT mark them pass unconditionally — if either test still fails post-Phase 1, investigate the actual failure before touching the baseline.

File: `tests/e2e/baseline/next-e2e-baseline.json`

Edit the following entries, changing `"class": "fail"` to `"class": "pass"` for each row that has been confirmed passing in the targeted Playwright run:

Confirmed targets (from baseline audit — verify counts via jq before editing):
- `jules-dispatch.spec.ts` — 10 entries (5 tests × chromium + chromium-dom) — confirmed by Task 1.1.2
- `alias-settings.spec.ts` — 6 entries (AC-02, AC-04, AC-05 × chromium + chromium-dom) — confirmed by Tasks 2.1.2 and 2.2.1
- `local-file-browser.spec.ts` — 4 entries (symlink + open-terminal × chromium + chromium-dom) — **confirm with Tasks 1.1.3 and 3.1.1 before editing**

Verification step: before editing, run:
```bash
python3 -I -c "
import json
with open('tests/e2e/baseline/next-e2e-baseline.json') as f:
    d = json.load(f)
target_specs = {'jules-dispatch.spec.ts', 'alias-settings.spec.ts', 'local-file-browser.spec.ts'}
failing = [s for s in d['specs'] if s['spec'] in target_specs and s['class'] == 'fail']
print(f'Found {len(failing)} fail entries to update (expected 20)')
for s in failing:
    print(f\"  {s['spec']} | {s['project']} | {s['title'][:60]}\")
"
```

Procedure: use `python3 -I -c "..."` to load the JSON, mutate the class fields for matching entries, and write back. Do NOT regenerate the full baseline from a live run — the file is static and hand-maintained.

#### Task 3.2.2 — Remove route-focus skip-link entries that reference deleted tests

File: `tests/e2e/baseline/next-e2e-baseline.json`

Remove the 2 entries matching the **exact title** (pre-mortem P1 fix — wrong title was originally listed in plan draft):
- `title` == `"route focus and announcements (Next baseline) > skip link: Tab reaches it, Enter keeps the path and sets the hash"`
- `project`: `chromium` and `chromium-dom`

Verified exact title from baseline JSON on 2026-11-05. Use this jq command to confirm before removing:
```bash
python3 -I -c "
import json
with open('tests/e2e/baseline/next-e2e-baseline.json') as f:
    d = json.load(f)
target = 'route focus and announcements (Next baseline) > skip link: Tab reaches it, Enter keeps the path and sets the hash'
rows = [s for s in d['specs'] if s.get('title') == target]
print(f'Found {len(rows)} entries (expected 2):')
for r in rows: print(f\"  {r['project']}\")
"
```

These tests no longer exist (deleted in PR #1385). They resolve as `removedFromRun` (severity `"report"`, not gate failure) until removed. Removing them prevents perpetual noise in the compare output.

#### Task 3.2.3 — Verify `bun-expected-diff.json` requires no changes

`tests/e2e/baseline/bun-expected-diff.json` currently has `"entries": []`. Since none of the fixed specs were listed in it, no changes are needed. Rule F6 (listed entry that now passes → gate failure) does not apply.

---

## Phase 4 — CI Verification Pass

### Story 4.1 — Full CI gate verification

#### Task 4.1.1 — Run `make quick-check` to verify build, tests, lint

```bash
make quick-check
```

Expected: exits 0.

#### Task 4.1.2 — Run `bun run lint:duplicates` in `web-app/`

```bash
cd web-app && bun run lint:duplicates
```

Expected: exits 0. The alias-settings spec change adds 4 changed lines — no structural duplication introduced.

#### Task 4.1.3 — Run targeted e2e suite to confirm AC-8

```bash
cd tests/e2e && npx playwright test alias-settings.spec.ts jules-dispatch.spec.ts local-file-browser.spec.ts route-focus-and-announce.spec.ts --project=chromium
```

Expected: all tests that were previously-passing continue to pass; all formerly-failing target tests now pass.

---

## Out of Scope (documented)

- **`visual-regression.spec.ts`** (11 total, 4 in baseline as `fail`): These rows are snapshot-comparison failures caused by per-environment rendering differences, not by the root causes addressed here. They are tracked in a separate item and must NOT be touched in this PR.
- **Other baseline-fail specs** (alias:invoke, android-url-bar, approval-ci-block, etc.): Not in the five target spec files; not in scope.
- **D-Bus Secret Service in CI**: Ruled out (see Build vs Buy research). `keyring.MockInit()` renders it unnecessary.

---

## Pattern Decisions Table

| Decision | Choice | Alternatives Rejected |
|---|---|---|
| Keychain stub mechanism | `keyring.MockInit()` at `--test-mode` startup in `main.go` | `JULES_API_KEY` env var (too narrow); OS D-Bus in CI (too heavy) |
| Locator fix ownership | Fix in spec (test file) | Fix in component (would change accessible names that are correct per WCAG 1.3.1) |
| Baseline update mechanism | Manual JSON edit (change `fail` → `pass`, remove deleted-test entries) | Live baseline capture (no such command exists; and the Next build no longer runs) |
| Symlink fix ownership | Investigate at implementation time | Predetermined assertion-update-only or server-fix-only |

---

## Implementation Sequence

```
Phase 1 (keychain stub) → Phase 2 (alias-settings) → Phase 3 (symlink + baseline) → Phase 4 (CI)
```

Phases 1 and 2 are independent; they can be implemented in parallel if two worktrees are used. Phase 3 depends on Phase 1 (the symlink test may already pass once keychain is fixed) and Phase 2 (baseline update should capture AC-02/AC-05 outcome). Phase 4 depends on all three.

---

## Glossary

| Term | Definition |
|---|---|
| TestMode | Binary startup flag `--test-mode`; enables isolated data dir and (after this plan) the KeychainStub. |
| KeychainStub | In-memory `go-keyring` provider installed by `keyring.MockInit()`. Empty map; returns `ErrNotFound` immediately. |
| BaselineRow | `{file, project, title, class}` entry in `next-e2e-baseline.json`. |
| BunExpectedDiff | `bun-expected-diff.json`; lists expected Bun/Next deviations. Currently empty. |
| removedFromRun | Baseline entry classification when the referenced test no longer exists; severity `"report"`, not a gate failure. |
| D-Bus circuit breaker | 5-minute lockout in `jules/keychain.go` after a keychain operation times out waiting for the D-Bus Secret Service daemon. |
