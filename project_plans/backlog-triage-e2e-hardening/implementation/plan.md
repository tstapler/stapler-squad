# Implementation Plan: backlog-triage-e2e-hardening

**Feature**: Fix two confirmed backlog triage bugs (UI gate + parser) and add e2e test coverage
**Date**: 2026-06-23
**Status**: Ready for implementation
**ADRs**: ADR-001-triage-parser-brace-scan.md

---

## Dependency Visualization

```
Phase 1 (parallel)
  Task A: Parser fix (Go)       ──────────────────┐
  Task B: UI gate (3 locations) ──────────────────┤
                                                   ▼
                                          Phase 2: Unit tests (Go)
                                                   │
                                          Phase 3: E2e tests (Playwright)
                                          (depends on UI gate being shipped)
```

Phase 1 tasks A and B are independent — they touch different files and can be
implemented in any order or concurrently. Phase 2 (unit tests for the parser)
depends on Phase 1A being done. Phase 3 (e2e tests) depends on Phase 1B being
done so the disabled-button assertion is meaningful.

---

## Phase 1: Bug Fixes

### Epic 1.1: Fix `ParseHeadlessTriageResult` parser

**Goal**: Replace the brittle leading-fence-only strip with the brace-scan pattern
already used by `ParseHeadlessVerdictResult`, so preamble text before the JSON
block is correctly skipped.

#### Story 1.1.1: Adopt brace-scan JSON extraction in the triage parser

**As a** developer operating the triage pipeline, **I want** `ParseHeadlessTriageResult`
to tolerate preamble text before the JSON block, **so that** multi-step triage runs
that emit research text before the final JSON do not silently fail with an empty
`triage_result`.

**Acceptance Criteria**:
- Pure JSON input (no fences, no preamble) continues to parse correctly.
- Input with a leading triple-backtick fence continues to parse correctly.
- Input with arbitrary preamble text followed by a JSON block parses correctly,
  returning the JSON block contents.
- Input with intermediate JSON during research phases and a final JSON block uses
  the LAST JSON block (brace-scan with `strings.LastIndex`).
- Input with no JSON block at all returns an error.
- All existing tests in `session/backlog_triage_test.go` still pass.

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/backlog_triage.go`

##### Task 1.1.1a: Replace `ParseHeadlessTriageResult` implementation (~3 min)

Replace the body of `ParseHeadlessTriageResult` (lines 92–116 in
`session/backlog_triage.go`) with the brace-scan pattern from
`ParseHeadlessVerdictResult` (lines 161–182 in `session/backlog_review.go`).

Exact steps:
1. Open `session/backlog_triage.go`.
2. Replace the entire function body of `ParseHeadlessTriageResult` with:
   ```go
   func ParseHeadlessTriageResult(raw string) (HeadlessTriageResult, error) {
       start := strings.Index(raw, "{")
       end := strings.LastIndex(raw, "}")
       if start == -1 || end <= start {
           preview := raw
           if len(preview) > 200 {
               preview = preview[:200] + "..."
           }
           return HeadlessTriageResult{}, fmt.Errorf("ParseHeadlessTriageResult: no JSON object found in output (raw: %q)", preview)
       }

       var result HeadlessTriageResult
       if err := json.Unmarshal([]byte(raw[start:end+1]), &result); err != nil {
           preview := raw
           if len(preview) > 200 {
               preview = preview[:200] + "..."
           }
           return HeadlessTriageResult{}, fmt.Errorf("ParseHeadlessTriageResult: JSON parse error: %w (raw: %q)", err, preview)
       }
       if len(result.Tasks) > maxHeadlessTriageTasks {
           result.Tasks = result.Tasks[:maxHeadlessTriageTasks]
       }
       return result, nil
   }
   ```
3. The `strings` import is already present; no new imports needed.
4. Run `go build ./session/...` to confirm it compiles.

Files: `session/backlog_triage.go`

---

### Epic 1.2: Add `repoPath` guard to Trigger Triage UI

**Goal**: Disable the "Trigger Triage" button in all three locations when
`item.repoPath` is empty/undefined, matching the disabled-button pattern already
used by the "Mark Ready" button.

#### Story 1.2.1: Guard Trigger Triage in BacklogItemDetail (idea status)

**As a** user viewing a backlog item in "idea" status, **I want** the "Trigger Triage"
button to be visually disabled with a tooltip when `repoPath` is not set, **so that**
I get clear feedback instead of a confusing server error.

**Acceptance Criteria**:
- Button has `disabled={actionLoading || !item.repoPath}`.
- Button has `aria-disabled={!item.repoPath}`.
- Button has `title="Set repository path first"` when `!item.repoPath`.

**Files**:
- `/home/tstapler/Programming/stapler-squad/web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.2.1a: Add repoPath guard to idea-status Trigger Triage button (~2 min)

Edit lines ~590–597 in `BacklogItemDetail.tsx` (the `item.status === "idea"` block).

Change:
```tsx
<button
  className={styles.actionButton}
  onClick={() => handleAction("trigger_triage")}
  disabled={actionLoading}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

To:
```tsx
<button
  className={styles.actionButton}
  onClick={() => handleAction("trigger_triage")}
  disabled={actionLoading || !item.repoPath}
  aria-disabled={!item.repoPath}
  title={!item.repoPath ? "Set repository path first" : undefined}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

#### Story 1.2.2: Guard Trigger Triage in BacklogItemDetail (ready status)

**As a** user viewing a backlog item in "ready" status, **I want** the "Trigger Triage"
button to be disabled with a tooltip when `repoPath` is not set.

**Acceptance Criteria**: Same three-attribute pattern as Story 1.2.1.

**Files**:
- `/home/tstapler/Programming/stapler-squad/web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.2.2a: Add repoPath guard to ready-status Trigger Triage button (~2 min)

Edit lines ~603–610 in `BacklogItemDetail.tsx` (the `item.status === "ready"` block).

Change:
```tsx
<button
  className={styles.actionButton}
  onClick={() => handleAction("trigger_triage")}
  disabled={actionLoading}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

To:
```tsx
<button
  className={styles.actionButton}
  onClick={() => handleAction("trigger_triage")}
  disabled={actionLoading || !item.repoPath}
  aria-disabled={!item.repoPath}
  title={!item.repoPath ? "Set repository path first" : undefined}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

#### Story 1.2.3: Guard Trigger Triage in BacklogItemCard (board view)

**As a** user viewing the backlog board, **I want** the "Trigger Triage" card action
to be disabled when `repoPath` is not set, so the board view matches the detail pane.

**Acceptance Criteria**:
- `getActionSpec` for the `"ready"` case sets `disabled: !item.repoPath`.
- The `disabled` field propagates to the card's action button (confirm `button disabled={actionSpec.disabled || isTriageRunning}` at line ~125 already picks it up).

**Files**:
- `/home/tstapler/Programming/stapler-squad/web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 1.2.3a: Add repoPath guard to getActionSpec ready case (~2 min)

Edit the `getActionSpec` function in `BacklogItemCard.tsx` — the `case "ready":` branch.

Change:
```tsx
case "ready":
  return { label: "Trigger Triage", action: "trigger_triage" };
```

To:
```tsx
case "ready":
  return { label: "Trigger Triage", action: "trigger_triage", disabled: !item.repoPath, ariaLabel: !item.repoPath ? "Set repository path first to trigger triage" : undefined };
```

Confirm the button render at line ~123-127 already passes `disabled={actionSpec.disabled || isTriageRunning}` and `aria-label={isTriageRunning ? ... : actionSpec.label}`. If `ariaLabel` is added to the spec type, thread it through; otherwise the existing `aria-label` is sufficient.

Files: `web-app/src/components/backlog/BacklogItemCard.tsx`

---

## Phase 2: Unit Tests (Go parser)

### Epic 2.1: Extend triage parser unit tests

**Goal**: Add test cases that prove the new brace-scan implementation handles preamble
text and intermediate JSON blocks, and that no-JSON input returns an error.

#### Story 2.1.1: Cover preamble and multi-block scenarios

**As a** developer, **I want** unit tests that cover the new brace-scan paths in
`ParseHeadlessTriageResult`, **so that** regressions are caught by `go test ./session/...`
before they reach CI.

**Acceptance Criteria**:
- New test `TestParseHeadlessTriageResult_PreambleBeforeJSON` passes.
- New test `TestParseHeadlessTriageResult_PreambleBeforeFencedJSON` passes (preamble text + ```json fence).
- New test `TestParseHeadlessTriageResult_IntermediateJSONIgnored` passes (uses last JSON block).
- New test `TestParseHeadlessTriageResult_NoJSON` passes (expects error).
- Existing tests (`_ValidJSON`, `_StripsMarkdownFences`, `_InvalidJSON`, `_CapsTasksAt12`,
  `_EmptySuggestionsOK`) still pass.

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/backlog_triage_test.go`

##### Task 2.1.1a: Add four new unit tests (~4 min)

Append the following test functions to `session/backlog_triage_test.go`:

```go
func TestParseHeadlessTriageResult_PreambleBeforeJSON(t *testing.T) {
    raw := "Here is my analysis of the backlog item.\nSome additional notes.\n" +
        `{"summary":"preamble ok","suggestions":[{"text":"s","rationale":"r"}]}`
    result, err := ParseHeadlessTriageResult(raw)
    require.NoError(t, err)
    assert.Equal(t, "preamble ok", result.Summary)
}

func TestParseHeadlessTriageResult_PreambleBeforeFencedJSON(t *testing.T) {
    // Most common real-world case: Claude outputs text, then a fenced block.
    raw := "Triage complete. Here is the result:\n\n" +
        "```json\n" +
        `{"summary":"fenced ok","suggestions":[]}` +
        "\n```"
    result, err := ParseHeadlessTriageResult(raw)
    require.NoError(t, err)
    assert.Equal(t, "fenced ok", result.Summary)
}

func TestParseHeadlessTriageResult_IntermediateJSONIgnored(t *testing.T) {
    // Simulates multi-step triage: intermediate JSON from research phase,
    // then the real final JSON. Parser should use the LAST JSON block.
    raw := `{"summary":"intermediate","suggestions":[]}` +
        "\n\nFinal output:\n" +
        `{"summary":"final","suggestions":[{"text":"real","rationale":"yes"}]}`
    result, err := ParseHeadlessTriageResult(raw)
    require.NoError(t, err)
    assert.Equal(t, "final", result.Summary)
    require.Len(t, result.Suggestions, 1)
    assert.Equal(t, "real", result.Suggestions[0].Text)
}

func TestParseHeadlessTriageResult_NoJSON(t *testing.T) {
    _, err := ParseHeadlessTriageResult("No JSON here at all.")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "ParseHeadlessTriageResult")
}
```

Run with: `go test ./session/... -run TestParseHeadlessTriageResult`

Files: `session/backlog_triage_test.go`

---

## Phase 3: E2e Tests

### Epic 3.1: Playwright test for Trigger Triage UI gate

**Goal**: A Playwright test asserts that the "Trigger Triage" button is disabled for an
item with no `repoPath`, confirming the UI gate from Epic 1.2 is wired correctly.

#### Story 3.1.1: Add `fillNewItemForm` repoPath option to BacklogPage helper

**As a** test author, **I want** `BacklogPage.fillNewItemForm` to accept an optional
`repoPath` string, **so that** I can create items with a repo path set in one call.

**Acceptance Criteria**:
- `fillNewItemForm(title, { repoPath: '/some/path' })` fills the repo path field.
- Existing calls without `repoPath` are unaffected.

**Files**:
- `/home/tstapler/Programming/stapler-squad/tests/e2e/pages/BacklogPage.ts`

##### Task 3.1.1a: Extend fillNewItemForm signature and body (~3 min)

Edit `BacklogPage.ts` — the `fillNewItemForm` method (lines 133–148).

Change the `options` type from:
```ts
options?: { priority?: number; addAcCriterion?: string }
```
To:
```ts
options?: { priority?: number; addAcCriterion?: string; repoPath?: string }
```

Add after the `addAcCriterion` block:
```ts
if (options?.repoPath) {
  const repoPathInput = this.page.locator('[data-testid="backlog-repo-path-input"]');
  await repoPathInput.fill(options.repoPath);
}
```

Files: `tests/e2e/pages/BacklogPage.ts`

#### Story 3.1.2: Write Triage UI gate e2e tests

**As a** developer, **I want** an e2e test that asserts the "Trigger Triage" button is
disabled on an item with no repoPath, **so that** the UI gate regression is caught in CI.

**Acceptance Criteria**:
- Test `e2e:backlog-triage-gate-disabled` creates an item without `repoPath`, opens
  detail pane, and asserts the Trigger Triage button is disabled.
- Test cleans up the created item in `test.afterEach` so it does not pollute empty-state tests.
- Tests are in a new `test.describe('Triage')` block inside the outer
  `test.describe('Backlog')` block.
- The `// @feature` header lists `backlog:trigger-triage`.

**Files**:
- `/home/tstapler/Programming/stapler-squad/tests/e2e/backlog.spec.ts`

##### Task 3.1.2a: Add Triage describe block with gate test (~5 min)

At the end of the `test.describe('Backlog', () => { ... })` block in
`tests/e2e/backlog.spec.ts`, add:

```ts
test.describe('Triage', () => {
  let createdItemTitle: string;

  test.afterEach(async ({ request }) => {
    // Archive the item created during the test to keep the backlog clean for
    // empty-state tests. Uses ArchiveBacklogItem (not DeleteBacklogItem — that
    // RPC does not exist). Archived items are hidden from the default list view.
    if (!createdItemTitle) return;
    try {
      const listRes = await request.post(
        `${BASE_URL}/session.v1.BacklogService/ListBacklogItems`,
        { data: {} }
      );
      const body = await listRes.json();
      const item = (body.items ?? []).find(
        (i: { title: string; id: string }) => i.title === createdItemTitle
      );
      if (item) {
        await request.post(
          `${BASE_URL}/session.v1.BacklogService/ArchiveBacklogItem`,
          { data: { id: item.id } }
        );
      }
    } catch {
      // Best-effort cleanup — do not fail the test on cleanup errors.
    }
  });

  test('e2e:backlog-triage-gate-disabled - Trigger Triage button disabled when repoPath is empty', async ({ page }) => {
    const backlogPage = new BacklogPage(page);
    createdItemTitle = `triage-gate-test-${Date.now()}`;

    // Create an item WITHOUT repoPath via the empty-state inline form.
    // The modal form (BacklogItemForm) requires repoPath for new items, so
    // we must use the empty-state path which only collects title + priority.
    // Guard: if the backlog already has items, the empty state won't show —
    // use the "+ New Item" modal only when items already exist, and skip
    // repoPath by leaving it blank (if form allows it without validation).
    //
    // Simplest reliable path: use createItemFromEmptyState only when empty;
    // otherwise fall back to page-level API call to create item with empty repoPath.
    const rowCount = await backlogPage.getTableRows().count();
    if (rowCount === 0) {
      await backlogPage.createItemFromEmptyState(createdItemTitle);
    } else {
      // Items exist — create via API directly with empty repoPath.
      await page.request.post(
        `${BASE_URL}/session.v1.BacklogService/CreateBacklogItem`,
        { data: { title: createdItemTitle, priority: 3, repoPath: '', skipTriage: true } }
      );
      await page.reload();
      await page.waitForSelector('[data-testid="backlog-table-row"]');
    }

    // Open the detail pane.
    await backlogPage.openItemDetail(createdItemTitle);
    await expect(backlogPage.getItemDetailPane()).toBeVisible();

    // The Trigger Triage button should be disabled because repoPath is empty.
    const triggerBtn = page.locator('[data-testid="backlog-action-trigger-triage"]');
    await expect(triggerBtn).toBeDisabled();
    await expect(triggerBtn).toHaveAttribute('aria-disabled', 'true');
    await expect(triggerBtn).toHaveAttribute('title', 'Set repository path first');
  });
});
```

**NOTE**: The `page.request` API is available in Playwright test context for direct HTTP calls
without a separate `request` fixture. Verify `${BASE_URL}/session.v1.BacklogService/CreateBacklogItem`
is the correct ConnectRPC endpoint by checking `proto/session/v1/backlog.proto` before implementing.

Also update the `// @feature` annotation at the top of the file to include
`backlog:trigger-triage`:
```ts
// @feature backlog — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['backlog-create-item'],
  FEATURE_CATALOG['backlog-list-items'],
  FEATURE_CATALOG['backlog-transition-status'],
  FEATURE_CATALOG['backlog-spawn-session'],
  FEATURE_CATALOG['backlog-trigger-triage'],
] as const;
```

(Add `backlog-trigger-triage` to the feature catalog if it does not exist yet —
see `docs/registry/frontend-features.json` and `web-app/src/lib/features.ts`.)

Files: `tests/e2e/backlog.spec.ts`

---

## Phase 4: Feature Registry Update

### Epic 4.1: Update registry entries

**Goal**: Keep `docs/registry/` in sync with the new triage UI feature.

#### Story 4.1.1: Register backlog-trigger-triage in the feature registries

**As a** developer reviewing coverage gaps, **I want** the triage UI feature tracked
in the feature registry, **so that** coverage-gaps.json does not grow.

**Acceptance Criteria**:
- `docs/registry/frontend-features.json` has an entry for `backlog-trigger-triage`
  with `tested: true` and `testIds` referencing `e2e:backlog-triage-gate-disabled`.
- `docs/registry/coverage-gaps.json` does not gain new entries.

**Files**:
- `/home/tstapler/Programming/stapler-squad/docs/registry/frontend-features.json`

##### Task 4.1.1a: Add frontend registry entry (~2 min)

Open `docs/registry/frontend-features.json` and add:

```json
{
  "id": "backlog-trigger-triage",
  "type": "frontend",
  "description": "Trigger Triage button in BacklogItemDetail and BacklogItemCard with repoPath guard",
  "component": "BacklogItemDetail / BacklogItemCard",
  "filePaths": [
    "web-app/src/components/backlog/BacklogItemDetail.tsx",
    "web-app/src/components/backlog/BacklogItemCard.tsx"
  ],
  "tested": true,
  "testIds": ["e2e:backlog-triage-gate-disabled"],
  "lastModified": "2026-06-23T00:00:00Z"
}
```

Run `make registry-diff` to preview, then `make registry-generate` if the tooling
auto-fills entries from source markers (add `// +feature: backlog:trigger-triage`
in the first 10 lines of `BacklogItemDetail.tsx` first).

Files: `docs/registry/frontend-features.json`

---

## Verification Checklist

After all phases are complete, run:

```bash
# 1. Go build + unit tests
make build && go test ./session/... -run TestParseHeadlessTriageResult -v

# 2. Full CI pipeline
make quick-check

# 3. E2e smoke (requires test server running)
# STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
# cd tests/e2e && npx playwright test backlog.spec.ts --grep "triage"

# 4. Registry diff
make registry-diff
```

All checks must pass before opening the PR.
