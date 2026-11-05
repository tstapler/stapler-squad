# Validation: e2e-backlog-flow-fixes

## Validation Strategy

For each acceptance criterion, the validation method is a runnable check — either an e2e test
command, a grep, or a baseline diff. No "read the code and trust it" steps.

---

## AC-01: All 11 backlog spec files have zero `fail`-class rows in baseline

**Check:**
```bash
python3 -I -c "
import json
with open('tests/e2e/baseline/next-e2e-baseline.json') as f:
    data = json.load(f)
target = [
  'backlog-connection-indicator.spec.ts','backlog-copy-link.spec.ts',
  'backlog-deep-link-errors.spec.ts','backlog-detail-live-updates.spec.ts',
  'backlog-edit-buffering.spec.ts','backlog-github-sync-settings.spec.ts',
  'backlog-manual-override.spec.ts','backlog-pipeline-mode.spec.ts',
  'backlog-rich-text.spec.ts','backlog-sources-settings.spec.ts','backlog.spec.ts',
]
fails = [s for s in data['specs'] if s['spec'] in target and s['class']=='fail']
print(f'Remaining fail rows: {len(fails)}')
for s in fails: print(f'  {s[\"spec\"]} / {s[\"title\"][:70]}')
"
```
**Pass condition:** output reads `Remaining fail rows: 0`.

---

## AC-02: No `test.fail(true)` empty-state guard in `backlog.spec.ts`

**Check:**
```bash
grep -n "test\.fail(true" tests/e2e/backlog.spec.ts
```
**Pass condition:** any remaining hits are outside the `Empty State` and `Item Creation and
List` describe blocks (or zero hits total). Confirm with:
```bash
grep -c "test\.fail(true" tests/e2e/backlog.spec.ts
```
Acceptable count: 0.

---

## AC-03: `backlog-sources-settings.spec.ts` and `backlog-github-sync-settings.spec.ts` clean up sources

**Static check:**
```bash
grep -n "afterEach\|deleteSource\|DeleteItemSource\|Remove.*Source" \
  tests/e2e/backlog-sources-settings.spec.ts \
  tests/e2e/backlog-github-sync-settings.spec.ts \
  tests/e2e/pages/BacklogSourcesSettingsPage.ts
```
**Pass condition:** `afterEach` block present in both spec files; cleanup call visible.

**Runtime check:** run both spec files twice back-to-back against the same isolated server:
```bash
cd tests/e2e && for i in 1 2; do
  npx playwright test backlog-sources-settings.spec.ts backlog-github-sync-settings.spec.ts \
    --project=chromium --reporter=list 2>&1 | grep -E "passed|failed"
done
```
**Pass condition:** both runs show identical passing tests with no "already exists" failures.

---

## AC-04: `ProduceWellFormedURLWithNoEncodingArtifacts` passes

**Runtime check:**
```bash
cd tests/e2e && npx playwright test backlog-copy-link.spec.ts \
  --project=chromium -g "ProduceWellFormedURL" --reporter=list
```
**Pass condition:** test passes (exit 0).

**Regression check:** confirm the clipboard text starts with `ssq://` and passes the new URL
structure assertion by adding a `console.log(clipboardText)` temporarily in the test and
verifying the output on a local run.

---

## AC-05: `ShowCopyFailedState` is properly annotated

**Static check:**
```bash
grep -n "test\.fail\|test\.skip" tests/e2e/backlog-copy-link.spec.ts
```
**Pass condition:** `test.fail(true` is absent; either `test.skip(true` with a reason is
present, or neither (if the feature is now implemented and the test passes normally).

**Runtime check:**
```bash
cd tests/e2e && npx playwright test backlog-copy-link.spec.ts \
  --project=chromium -g "ShowCopyFailedState" --reporter=list
```
**Pass condition:** test is `skipped` (or `passed` if implemented), not `failed`.

---

## AC-06: `backlog-deep-link-errors.spec.ts` unreachable/unregistered tests pass

**Runtime check:**
```bash
cd tests/e2e && npx playwright test backlog-deep-link-errors.spec.ts \
  --project=chromium -g "unreachableHostLink|unregisteredHostLink" --reporter=list
```
**Pass condition:** both tests pass (exit 0).

**Component verify:** manually inspect `web-app/src/components/backlog/DeepLinkErrorBanner.tsx`
to confirm the `role="status"` or `data-testid="deep-link-error-banner"` element contains the
hostname in its text content (not only in `aria-label`) for `unreachable` and `not-registered`
reason codes.

---

## AC-07: `backlog-connection-indicator.spec.ts` setOffline test is `fixme` or reliably passing

**Static check:**
```bash
grep -n "test\.fixme\|setOffline" tests/e2e/backlog-connection-indicator.spec.ts
```
**Pass condition:** the `indicator flips to Reconnecting` test is wrapped in `test.fixme(true,
...)` with a reason, OR the test passes consistently across 3 runs.

**Runtime check (3 runs):**
```bash
cd tests/e2e && for i in 1 2 3; do
  npx playwright test backlog-connection-indicator.spec.ts \
    --project=chromium --reporter=list 2>&1 | grep -E "flips to Reconnecting"
done
```
**Pass condition:** consistent output (all `✓` or all `fixme`; no mix of pass/fail).

---

## AC-08: Items created by live-update / manual-override / pipeline-mode / rich-text tests are cleaned up

**Static check:**
```bash
grep -n "afterEach\|archiveBacklogItemDirect\|deleteBacklogItemDirect\|DeletePipelineMode" \
  tests/e2e/backlog-detail-live-updates.spec.ts \
  tests/e2e/backlog-manual-override.spec.ts \
  tests/e2e/backlog-pipeline-mode.spec.ts \
  tests/e2e/backlog-rich-text.spec.ts
```
**Pass condition:** each file has an `afterEach` with a cleanup call.

**Runtime check (double-run):** same back-to-back pattern as AC-03 but for these 4 spec files.
**Pass condition:** both runs pass identically.

---

## AC-09: No `waitForTimeout` added

**Check:**
```bash
grep -rn "waitForTimeout" tests/e2e/backlog*.spec.ts tests/e2e/pages/Backlog*.ts
```
**Pass condition:** zero new hits (pre-existing ones, if any, were not added by this PR — verify
with `git diff HEAD | grep waitForTimeout` returning empty).

---

## AC-10: Baseline updated

**Check:**
```bash
python3 -I -c "
import json
with open('tests/e2e/baseline/next-e2e-baseline.json') as f:
    data = json.load(f)
all_fails = [s for s in data['specs'] if s['class']=='fail']
print(f'Total fail rows: {len(all_fails)}')
"
```
**Pass condition:** total fail rows reduced by at least 36 (the ≈38 open `backlog-flows` rows
minus the ones covered by #1208).

---

## Full Regression Gate

After all individual checks pass, run the full e2e suite to confirm no regressions:
```bash
cd tests/e2e && bun run test --project=chromium --reporter=list 2>&1 | tail -20
```
Then compare the new baseline against `main`:
```bash
git diff main -- tests/e2e/baseline/next-e2e-baseline.json | grep '"class"' | sort | uniq -c
```
**Pass condition:** only lines moving from `fail → pass` or `fail → skipped`; no lines moving
from `pass → fail`.
