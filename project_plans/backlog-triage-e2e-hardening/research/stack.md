# Stack Research: backlog-triage-e2e-hardening

## 1. Go JSON Extraction from Free-Form Text

### Recommended approach: `strings.Index` / `strings.LastIndex` bracket search

The codebase already uses this pattern in `session/backlog_review.go` lines 162-170
(`ParseHeadlessVerdictResult`):

```go
start := strings.Index(text, "{")
end := strings.LastIndex(text, "}")
if start == -1 || end <= start {
    return error sentinel
}
json.Unmarshal([]byte(text[start:end+1]), &v)
```

This is the correct approach for `ParseHeadlessTriageResult` because:
- **Handles preamble**: skips any natural-language text before the JSON object.
- **Handles trailing text**: `strings.LastIndex` finds the final `}`, which closes the outermost object even if prose follows.
- **Handles nested objects correctly**: first `{` is always the outermost open brace when the LLM emits a single top-level JSON object. `strings.LastIndex` for `}` correctly finds the outermost close brace because nested braces are always interior.
- **Simple, zero-dependency**: no regex, no decoder gymnastics.

### Why not `json.Decoder` streaming?

`json.Decoder` started at the first `{` would work for well-formed JSON, but:
- It returns an error if there is trailing non-JSON text after the object (the decoder reads until EOF).
- It requires rewinding/splitting the string to find where to start.
- Adds complexity with no advantage for this single-object case.

### Why not regex?

- Regex cannot correctly match nested balanced braces.
- Go's `regexp` package does not support recursive/balancing patterns.
- Only viable for flat/known-depth JSON, which is fragile.

### Edge cases to be aware of

- **JSON array top-level**: The HeadlessTriageResult is always an object (`{...}`), never an array, so bracket `[`/`]` detection is not needed here.
- **JSON string containing `}`**: `strings.LastIndex` for `}` could in theory land inside a string value. However, `json.Unmarshal` will reject malformed JSON if the slice ends mid-string, so the real `}` (which always comes after all string values in a well-formed object) will still be found by `LastIndex` in valid LLM output. This is the same pattern already shipping in `ParseHeadlessVerdictResult` with no issues.
- **Multiple JSON objects in output**: `strings.Index` finds the first `{` and `strings.LastIndex` finds the last `}`. If the LLM emits two objects, this would attempt to parse the outer span — which is not valid JSON. In practice the prompt instructs "output ONLY a JSON object" so this is not a real risk.

### Conclusion

Apply the identical `strings.Index` / `strings.LastIndex` pattern from `ParseHeadlessVerdictResult` to `ParseHeadlessTriageResult`. No new imports needed; `strings` and `encoding/json` are already imported in `session/backlog_triage.go`.

---

## 2. React Disabled Button with Tooltip Pattern

### Existing codebase pattern (canonical reference)

`BacklogItemDetail.tsx` lines 580-588 show the `Mark Ready` button's pattern:

```tsx
<button
  className={styles.actionButton}
  onClick={() => handleAction("mark_ready")}
  disabled={actionLoading || item.acCriteria.length === 0}
  aria-disabled={item.acCriteria.length === 0}
  title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
  data-testid="backlog-action-mark-ready"
>
  Mark Ready
</button>
```

The `Trigger Triage` button (lines 590-597 for `idea` status, 602-609 for `ready` status) currently only has `disabled={actionLoading}` — no `aria-disabled`, no `title`.

### Why `disabled` + `title` works here (not a tooltip library issue)

The `title` attribute on HTML buttons works even when `disabled` because `title` is shown by the browser via native tooltip on hover, and native tooltips do fire on disabled elements in all major browsers (Chrome, Firefox, Safari). The `mouseenter` event that most JS tooltip libraries rely on is indeed blocked for disabled buttons — but since the codebase uses native `title` (not a React tooltip library), this is not a problem.

The `aria-disabled` attribute is set separately from the `disabled` attribute in the `Mark Ready` button because:
- `disabled` prevents clicks and form submission.
- `aria-disabled="true"` without `disabled` lets the element remain focusable (useful for screen readers to discover and read the tooltip/title).
- The codebase sets both: `disabled` blocks the click, `aria-disabled` signals intent to assistive technology.

### Fix for `Trigger Triage` button

For both `idea` and `ready` status variants, apply the same pattern:

```tsx
<button
  className={styles.actionButton}
  onClick={() => handleAction("trigger_triage")}
  disabled={actionLoading || !item.repoPath}
  aria-disabled={!item.repoPath}
  title={!item.repoPath ? "Set a repo path before triggering triage" : undefined}
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

Note: `item.repoPath` is the TypeScript field name — confirm the exact field name used in the `BacklogItem` type in the frontend (likely `repoPath` from the proto snake_case → camelCase conversion).

---

## 3. Playwright Test Patterns for Async Backend Operations

### What the existing tests use

From `tests/e2e/backlog.spec.ts`:
- **Primary pattern**: `await expect(locator).toBeVisible()` with default or explicit timeout (e.g., `{ timeout: 10000 }`).
- **Status transitions**: `await expect(detailStatus).toHaveAttribute('aria-label', 'Status: Ready', { timeout: 10000 })` — waits up to 10 s for the attribute to change.
- **Form/network operations**: `await expect(modal).not.toBeVisible()` after submit — waits for modal close as proxy for network completion.
- **Element existence**: `await page.waitForSelector('[data-testid="backlog-table-row"]')` — one instance using `waitForSelector` directly.
- **Anti-pattern avoided**: no `waitForTimeout` anywhere (the spec file explicitly comments "Replaces waitForTimeout" on one line).
- **Test ID discipline**: all locators use `data-testid` or `aria-label` attributes, never CSS class selectors.

### Mock/deterministic mode for triage tests

There is **no `--mock-triage` flag** in `main.go`. The only test-related flag is `--test-mode` (line 617 of `main.go`), which sets an isolated data directory but does not mock the headless pool.

The e2e test server is started with:
```
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server
```

For triage e2e tests, options for deterministic behavior are:
1. **Backend stub via dependency injection**: `BacklogService.SetHeadlessPool(pool)` accepts any `headless.PoolClient` interface. A test binary could inject a mock pool that returns a fixed JSON response. This requires a test-mode binary variant or a new `--mock-headless-pool` flag.
2. **Item without repoPath**: Test that the button is correctly disabled by creating an item via the empty-state form (which never sets `repo_path`) and asserting the button is `aria-disabled`.
3. **Integration test only at Go level**: The full triage flow (headless pool → LLM → parse → store) is already unit-tested in `server/services/backlog_service_test.go`. The e2e test can verify the UI gate without exercising the actual LLM call.

### Recommended e2e test structure for this bug fix

```typescript
test.describe('TriggerTriage', () => {
  test('e2e:backlog-trigger-triage-disabled-no-repo-path - Button disabled when item has no repo_path', async ({ page }) => {
    // Create item via empty-state form (never sets repo_path)
    // Open detail pane
    // Assert button has aria-disabled="true"
    // Assert button has title attribute with explanatory text
    // Assert clicking does NOT fire handleAction (button is disabled)
  });
});
```

No `waitForTimeout`, no CSS selectors, timeout of 10 s on async state assertions.

---

## Summary of Key Findings

- **JSON extraction**: Use `strings.Index`/`strings.LastIndex` bracket search — this is already the established pattern in `ParseHeadlessVerdictResult` and handles nested objects and LLM preamble correctly. Apply identically to `ParseHeadlessTriageResult`.
- **Disabled button + tooltip**: The `Mark Ready` button (lines 580-588) is the exact template: `disabled` + `aria-disabled` + `title`. Native `title` works on disabled buttons (no JS tooltip library involved), so no workaround for `mouseenter` blocking is needed.
- **E2e test patterns**: `expect(locator).toHaveAttribute(...)` with explicit timeout is the norm; no `waitForTimeout`. There is no existing mock-triage server flag — the cleanest e2e test for the UI gate creates an item without a `repo_path` and asserts `aria-disabled="true"` without needing to invoke the actual headless pool.
