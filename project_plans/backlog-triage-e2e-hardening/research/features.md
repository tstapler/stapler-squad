# Feature Research: backlog-triage-e2e-hardening

## 1. UI Guard Behavior

### "Mark Ready" Pattern (Existing Reference)

In `BacklogItemDetail.tsx` (lines 580–598), "Mark Ready" is guarded as follows:

```tsx
<button
  disabled={actionLoading || item.acCriteria.length === 0}
  aria-disabled={item.acCriteria.length === 0}
  title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
  data-testid="backlog-action-mark-ready"
>
  Mark Ready
</button>
```

Three attributes work together: `disabled`, `aria-disabled`, and `title` (tooltip). The `title` only appears when the guard condition is true. This is the exact pattern to replicate for Trigger Triage.

### "Trigger Triage" Current State (Bug)

`Trigger Triage` appears in **two status blocks**:
- Lines 590–597: inside the `idea` status block
- Lines 602–609: inside the `ready` status block

Both are currently:
```tsx
<button
  disabled={actionLoading}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

No `repoPath` guard at all. The fix must be applied to both locations identically.

### Fix Pattern to Apply

```tsx
<button
  disabled={actionLoading || !item.repoPath}
  aria-disabled={!item.repoPath}
  title={!item.repoPath ? "Set a repository path before triggering triage" : undefined}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

Note: `item.repoPath` is typed as `string | undefined` in the `BacklogItem` interface (line 74 of `useBacklogService.ts`). The guard `!item.repoPath` correctly handles both `""` and `undefined`.

### Edit Mode and repoPath Persistence

**Edit mode** in `BacklogItemDetail.tsx` (lines 375–408) renders `<BacklogItemForm initialValues={item} ... />` with `handleUpdateItem` as the `onSubmit`. `BacklogItemForm` includes a `RepoPathInput` field (lines 163–184) that is populated from `initialValues?.repoPath` (line 36 of `BacklogItemForm.tsx`).

**Important validation quirk**: `BacklogItemForm.validate()` (lines 46–55) only requires `repoPath` for **new items** (`!initialValues?.id`). When editing an existing item, `repoPath` is optional — the user can clear it and save. This is intentional (they may not have set it yet).

**Backend persistence**: `UpdateBacklogItem` handler in `backlog_service.go` (lines 566–569) applies the update only when `req.Msg.RepoPath != ""`. So:
- If the user sets a non-empty `repoPath` in the edit form, it **is persisted**.
- If the user clears `repoPath` to empty and saves, it **is NOT persisted** (the empty string is ignored by the update handler). There is no way to clear `repoPath` once set via the edit form.

**Frontend hook**: `useBacklogService.ts` (line 389) passes `data.repoPath` directly into the RPC. Since `BacklogItemForm` always includes the current value of `repoPath` in the form state, an edit that leaves `repoPath` populated will correctly persist it.

**Conclusion**: The edit→set repoPath→save flow works correctly end-to-end. A user can add `repoPath` to an existing item via edit mode, and the next `TriggerTriage` call will succeed.

---

## 2. Parser Edge Cases

### Prompt Instruction

`BuildHeadlessTriagePrompt` (line 80–83 of `backlog_triage.go`) instructs:

> "After all files are written, output ONLY a JSON object (no other text before or after)"

In practice, headless `-p` mode (`claude -p ...`) does **not** reliably produce pure JSON only. Claude frequently emits preamble like "I'll analyze this..." or a closing note, especially when the task involves multi-step agent work (research → plan → output). The "ONLY" instruction compliance rate in headless mode is low for complex multi-step prompts.

### Current Parser Behavior (`ParseHeadlessTriageResult`)

The current implementation (lines 92–116):
1. `strings.TrimSpace(raw)` — trims leading/trailing whitespace.
2. Checks if the trimmed text starts with ` ``` ` — if so, strips the opening fence and closing fence.
3. Passes the result to `json.Unmarshal`.

**What it handles correctly:**
- Pure JSON: `{"summary":"..."}` ✓
- Fenced at start: ` ```json\n{...}\n``` ` ✓ (strips fence)

**What it FAILS on:**
- **Preamble + fenced block**: `"Triage complete.\n\n```json\n{...}\n```"` — first `if` is false (doesn't start with ` ``` `), passes raw preamble + JSON to Unmarshal → **parse error**
- **Preamble + raw JSON**: `"Here is the analysis:\n\n{...}"` — `TrimSpace` does not remove preamble, Unmarshal sees `"Here is..."` → **parse error**
- **Multiple JSON blocks**: The current parser would take the entire trimmed text (without preamble stripping) and fail. There's no logic to find the last (or first) JSON block.
- **Truncated JSON**: `{"summary":"abc","suggestions":[` — Unmarshal returns a syntax error. Parser currently wraps this as-is into the error message.

### Recommended Fix

Replace the current fence-stripping logic with a two-strategy approach:

**Strategy A — Find JSON block in output (handles all preamble cases):**
1. Find the last occurrence of ` ```json ` or ` ``` ` followed by `{`.
2. Extract the content between the opening and closing fence.
3. If no fence found, find the **last** `{` that opens a JSON object and extract from there to end.
4. Use "last occurrence" not "first" — Claude sometimes emits a partial attempt before the final corrected JSON.

**Strategy B — Fallback direct Unmarshal:**
If neither fence extraction nor `{` scanning finds parseable JSON, return the original error.

**Truncated JSON**: Return an error with a clear message: `"ParseHeadlessTriageResult: output appears truncated"`. Do not attempt partial parsing.

**Which JSON block to use (first vs last):** Use the **last** complete JSON block. In multi-step prompts, Claude may emit intermediate JSON-like output during research phases. The final Step 4 output is always last.

### Proposed Robust Extraction Logic (pseudocode)

```
raw → TrimSpace
if contains "```":
    find last "```json" or "```" that precedes a "{"
    extract content between that fence and its closing "```"
    → candidate
else:
    find last "{" in raw
    extract from that index to end
    → candidate

json.Unmarshal(candidate) → result or error
```

---

## 3. E2E Test Scope

### Existing Test Structure

`tests/e2e/backlog.spec.ts` has five `describe` blocks:
- `Empty State` — 4 tests, all require a **clean backlog** (fail loudly if items exist)
- `Item Creation and List` — 3 tests, some require clean backlog
- `Filter Zero State` — 2 tests
- `Page Navigation` — 2 tests
- `Status Transitions` — 2 tests (one fixme)

### Shared Data Risks

The `Empty State` tests are the only ones that are fragile about pre-existing items. The triage happy-path test creates its own item and only needs the item to exist — it doesn't conflict with or require a clean backlog.

**No existing tests touch `TriggerTriage`** — there are no `backlog-action-trigger-triage` selectors in the current spec. The triage flow is entirely untested at the e2e level.

### Self-Contained Test Design

The triage e2e test can and should be fully self-contained:

1. **Create item** via `backlogPage.openNewItemForm()` + `fillNewItemForm()` with a repoPath.
   - **Problem**: `BacklogPage.fillNewItemForm()` does not support setting `repoPath`. The `data-testid="backlog-repo-path-input"` is in the form, but `fillNewItemForm()` in `BacklogPage.ts` doesn't expose a `repoPath` option. The test will need to directly locate and fill the repo path input, or extend `fillNewItemForm()`.
   
2. **Open item detail** via `backlogPage.openItemDetail(itemTitle)`.

3. **Verify guard**: assert `Trigger Triage` is enabled (item has repoPath), confirm `Mark Ready` is disabled (no AC).

4. **Trigger triage**: click `[data-testid="backlog-action-trigger-triage"]`.

5. **Wait for running state**: poll for `[data-testid="backlog-item-detail"]` to show `TriageLoadingIndicator` (the detail pane auto-polls every 5s while `triageStatus === "running"`).

6. **Wait for completion**: poll until triage result panel or failed banner appears (timeout ~120s given Claude headless can take 30–90s).

7. **Assert success**: `TriageReviewPanel` with `data-testid` selectors should appear.

### Key Constraint: repoPath must be a real path on the test machine

The backend validates `repoPath` is a real directory. For CI, the test should use a path that always exists, e.g. the repo root itself (`process.env.STAPLER_SQUAD_TEST_REPO_PATH` or a well-known temp dir). The test should skip gracefully if no valid path is available.

### Recommended Test Description

```typescript
// @feature backlog:trigger-triage
test.describe('Triage', () => {
  test('e2e:backlog-triage-happy-path - Item with repoPath triggers triage and shows review panel', async ({ page }) => {
    // Creates item with repoPath, triggers triage, waits for completion
  });

  test('e2e:backlog-triage-gate-no-repo-path - Trigger Triage button is disabled when repoPath is empty', async ({ page }) => {
    // Creates item without repoPath, verifies button is disabled with tooltip
  });
});
```

The gate test is fast (no LLM call) and can run reliably in CI. The happy-path test is slow and may need a longer timeout.

---

## Summary of Key Findings

### Finding 1: UI Guard Pattern
Copy the exact `Mark Ready` guard pattern (`disabled`, `aria-disabled`, `title`) and apply it to both Trigger Triage buttons (idea status block and ready status block). The guard condition is `!item.repoPath`. The edit form already includes `RepoPathInput` and the backend already persists non-empty updates — the full user flow for "edit item → add repoPath → trigger triage" is functional end-to-end.

### Finding 2: Parser Fix Strategy
The parser fails for any output with preamble (the most common real-world case for complex multi-step prompts). The fix should scan for the **last** fenced JSON block, fall back to the last `{` in the output, then unmarshal. "Last occurrence" is correct because Claude may emit intermediate JSON during research phases. Truncated JSON should error clearly, not silently produce partial results.

### Finding 3: E2E Test Design
No existing tests touch the triage flow — no conflicts. The `BacklogPage` helper needs a small extension to support `repoPath` in `fillNewItemForm()`. The triage test should be in its own `describe('Triage')` block, with a fast gate-check test (no LLM) and a slow happy-path test (with LLM). The happy path must use a real filesystem path for `repoPath` and set a long timeout (≥120s).
