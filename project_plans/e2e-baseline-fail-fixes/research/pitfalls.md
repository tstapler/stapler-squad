# E2E Baseline Fail Fixes — Pitfalls Research

## 1. keyring.MockInit() safety

**Source:** `github.com/zalando/go-keyring@v0.2.8/keyring_mock.go` and `jules/keychain.go`

`MockInit()` replaces the package-level `provider` variable with a fresh `*mockProvider{mockStore: nil}`. The store is an in-memory `map[string]map[string]string` with no mutex of its own.

**State reset between test runs:** No. `MockInit()` is called once per process. The mock store persists for the entire server process lifetime. There is no built-in reset between individual e2e test cases.

**Goroutine safety:** The `mockProvider`'s map has no internal mutex. However, `KeyringTokenSource.resolveSync` serializes all concurrent Get calls through `sfGroup.Do` (singleflight), so only one goroutine ever calls `mockProvider.Get` at a time per coalesced burst. Concurrent `Set`/`Delete` calls via `SetJulesAPIKey`/`DeleteJulesAPIKey` are also bounded by `raceKeyringOp`'s single goroutine per op. In practice, e2e tests do not set a Jules API key through the UI, so the store stays empty throughout the run — the data race surface is not exercised.

**Cross-contamination between e2e tests:** Negligible in practice. The only contamination vector would be an e2e test that calls `SetJulesAPIKey` through the Jules settings UI; none of the failing specs (alias-settings, local-file-browser, jules-dispatch) do this. The store remains empty → every `APIKey()` call returns `ErrJulesNotConfigured` throughout the run.

**5-minute circuit breaker (jules/keychain.go):** The circuit breaker (`defaultCircuitCooldown = 5 * time.Minute`) opens only on a **timeout** (`timedOut == true` in `resolveSync`). In mock mode `mockProvider.Get` returns immediately with `keyring.ErrNotFound` — not a timeout. The circuit therefore never opens. The TTL cache (`defaultCacheTTL = 5 * time.Minute`) also does not apply: the cache write path in `resolveSync` is guarded by `case err == nil`, so an `ErrNotFound` result is never cached. Each `APIKey()` call does one mock-Get synchronously, pays zero cost, and returns `ErrJulesNotConfigured`. No 5-minute hang risk.

**Where to call MockInit():** Insert `keyring.MockInit()` immediately after the `if testModeFlag {` block in `main.go` (around line 175, after the `STAPLER_SQUAD_TEST_DIR` env-var is set). It must run before any `KeyringTokenSource` is constructed or any HTTP handler is registered, which is satisfied at that point in the startup sequence.

---

## 2. Multiple server instances (E2E_WORKERS=N)

`keyring.MockInit()` replaces a **package-level variable** (`provider`) inside the `go-keyring` package. That variable lives in the memory of one process. Each e2e worker spawns a **separate OS process** via `global-setup.ts`; they share no address space and no package-level state. Each process independently calls `MockInit()` at startup, initializing its own empty mock store.

There is no persistence mechanism for the mock store (no file, no IPC). Process restarts also start fresh — there is no state carried over.

**Verdict:** Completely safe for multi-worker e2e runs. No cross-process contamination is possible.

---

## 3. Playwright getByLabel on aria-labelledby sections

**Source:** `web-app/src/components/settings/AliasesManager.tsx` lines 391–395, `tests/e2e/pages/SettingsPage.ts` line 35

The alias form is:
```html
<section aria-labelledby="alias-form-title">
  <h3 id="alias-form-title">New Alias</h3>  <!-- or "Edit Alias: {name}" -->
  ...
</section>
```

Playwright's `getByLabel(text)` resolves `aria-labelledby` by looking up the referenced element and reading its text content, as specified in ARIA and implemented in Playwright since v1.27. This is reliable.

The regex `/^(New|Edit) Alias/` matches both "New Alias" and "Edit Alias: e2e-edit" (the latter starts with "Edit Alias"). Both cases are covered.

**AC-02 risk:** `SettingsPage.aliasFormSave()` returns `page.getByLabel(/^(New|Edit) Alias/).getByRole("button", { name: "Save" })`. After `clickEditAlias("e2e-edit")` opens the edit form, the h3 reads "Edit Alias: e2e-edit". The regex matches. The Save button inside the section is found via `getByRole`. This chain should work.

**Actual AC-02 failure cause:** The spec does `settings.saveAlias()` to create the alias, then `settings.clickEditAlias("e2e-edit")` to open the edit panel. `saveAlias()` calls `aliasFormSave().click()` then waits for `getByText(/saved/i)`. If the success toast appears and disappears before the `expect` assertion, `saveAlias()` could throw. But this is a pre-existing concern unrelated to the label change. The more likely AC-02 issue is that `clickNewAlias()` → save closes the form, and subsequently `clickEditAlias` re-opens it in edit mode — if the form animation/transition hasn't completed, the section's label may still be in transition. No hard evidence; treat as low risk.

---

## 4. Tag chip text "e2e-tagx"

**Source:** `web-app/src/components/settings/AliasesManager.tsx` lines 551–562

The tag chip renders:
```tsx
<span className={tagClass}>
  {t}                              {/* text node: "e2e-tag" */}
  <button aria-label="Remove tag e2e-tag">
    {/* analytics-exempt */}
    x
  </button>
</span>
```

The span's **combined text content** (normalized) is `"e2e-tagx"` — the tag name plus the button's literal text `"x"`.

**Playwright `getByText("e2e-tag")` behavior:** Playwright's `getByText` with a plain string uses `exact: false` by default (substring matching). `"e2e-tag"` is a substring of `"e2e-tagx"`, so the locator finds the span and `toBeVisible()` **passes**. The assertion as written does not fail due to the "x" suffix.

**However:** if a future Playwright version changes the default to `exact: true`, the same locator would fail to find the span (whose text is `"e2e-tagx"`, not `"e2e-tag"`). The safer long-term assertion is a regex:
```ts
await expect(page.getByText(/e2e-tag/)).toBeVisible();
```
Or scope via ARIA role:
```ts
await expect(page.getByRole("listitem").filter({ hasText: "e2e-tag" })).toBeVisible();
```
Or assert via the remove button's aria-label (which does not include "x"):
```ts
await expect(page.getByLabel("Remove tag e2e-tag")).toBeVisible();
```

**Verdict for AC-05:** The test as written (`getByText("e2e-tag")`) passes today because substring matching finds the span. This is not currently a source of failure in the baseline. If the spec is rewritten to use `{ exact: true }`, it would need the fix above.

---

## 5. Baseline JSON regeneration

**Source:** `scripts/e2e-baseline-compare.ts`, `tests/e2e/baseline/next-e2e-baseline.json`, `tests/e2e/baseline/bun-expected-diff.json`

**What `next-e2e-baseline.json` is:** A static snapshot recorded from the last Next.js build run before Next was removed from CI. It classifies each spec as `"pass"`, `"fail"`, or `"flaky"`. There is **no regeneration command** — the script has no `--write` or `--update-baseline` flag. Any change to this file is a manual JSON edit.

**Command to gate against the baseline (read-only):**
```bash
ts-node scripts/e2e-baseline-compare.ts \
  --baseline tests/e2e/baseline/next-e2e-baseline.json \
  --expected-diff tests/e2e/baseline/bun-expected-diff.json \
  --bun-reports path/to/bun-report.json
```
This exits 0 on pass, 1 on gate failure.

**Route-focus-and-announce rows:** The comparator handles "baseline entry whose test no longer exists in the Bun run" as `severity: "report"` (not `"fail"`) — R7 in `e2e-baseline-compare.test.ts`. Deleting the tests (done in PR #1385) is sufficient to stop these rows from blocking CI; the baseline JSON does not need to be regenerated to remove them. They simply appear in the stdout summary as informational notes with no effect on exit code.

**bun-expected-diff.json:** This file lists specs expected to fail in the Bun build (Playwright gate rule F6): "a listed spec that now passes and was not removed — gate failure." Once alias-settings, jules-dispatch, and local-file-browser are fixed, their entries must be **removed** from `bun-expected-diff.json`. Failing to remove them would cause a gate failure on the first CI run after the fix.

**Risk of capturing flaky rows as "pass":** Since `next-e2e-baseline.json` is never regenerated from a live run, there is no risk of a flaky run promoting broken tests to `class: pass`. The baseline is a one-time historical artifact. The only baseline-class changes that could introduce regressions are manual JSON edits — which should be reviewed in the PR diff.

**Summary of changes needed to `bun-expected-diff.json`:** When specs are fixed, remove their entries from `bun-expected-diff.json`. The `bun-expected-diff.json` file is the only file that needs updating beyond the code/test fixes themselves (and the `next-e2e-baseline.json` entries for route-focus-and-announce can be left as-is since the comparator handles missing tests gracefully).
