# Architecture Research: backlog-triage-e2e-hardening

## 1. TriggerTriage Data Flow

### Full call chain

```
BacklogItemDetail.tsx  (line 592, 605)
  onClick={() => handleAction("trigger_triage")}
    → handleAction (line 122) case "trigger_triage"
      → triggerTriage(item.id)          [useBacklogService.ts]
        → clientRef.current.triggerTriage({ itemId: id })  [ConnectRPC]
          → BacklogService.TriggerTriage() [server/services/backlog_service.go:1101]
            → item.RepoPath == "" check   [line 1126]
```

The board page (`web-app/src/app/backlog/board/page.tsx`) has the same flow:
```
BacklogBoard → BacklogItemCard → onAction("trigger_triage", itemId)
  → board page handleAction (line 37) → triggerTriage(itemId)
```

### `item.repoPath` availability in the UI

- `BacklogItem` type (useBacklogService.ts line 74): `repoPath?: string` — optional, may be undefined/empty.
- Mapped from proto at line 245: `repoPath: p.repoPath || undefined`.
- `item` is fully available in `BacklogItemDetail` when the buttons are rendered; `item.repoPath` is present (possibly empty string or undefined).

### The bug: where the gate is missing

**BacklogItemDetail.tsx lines 590-597 (status === "idea") and 603-610 (status === "ready")**:
```tsx
<button
  onClick={() => handleAction("trigger_triage")}
  disabled={actionLoading}           // ← no repoPath guard
  data-testid="backlog-action-trigger-triage"
>
  Trigger Triage
</button>
```

Neither occurrence checks `item.repoPath`. The fix is to add `disabled={actionLoading || !item.repoPath}` (and a matching `title` tooltip) to both buttons.

**BacklogItemCard.tsx** (board view, line 33): The `getActionSpec` for `"ready"` status returns `{ label: "Trigger Triage", action: "trigger_triage" }` — no `disabled` guard based on `repoPath`. The fix here is to add `disabled: !item.repoPath` to that case.

## 2. ParseHeadlessTriageResult Architecture

### Current implementation (session/backlog_triage.go lines 90-116)

```go
func ParseHeadlessTriageResult(raw string) (HeadlessTriageResult, error) {
    text := strings.TrimSpace(raw)
    if strings.HasPrefix(text, "```") {
        lines := strings.SplitN(text, "\n", 2)
        if len(lines) == 2 {
            text = lines[1]
        }
        if idx := strings.LastIndex(text, "```"); idx >= 0 {
            text = strings.TrimSpace(text[:idx])
        }
    }
    // json.Unmarshal(text)...
}
```

**The brittleness**: The fence strip is gated on `strings.HasPrefix(text, "```")`. If the LLM emits any prose before the fence (e.g., "Here is the JSON:\n```json\n{...}\n```"), the prefix check fails and the raw prose string is passed to `json.Unmarshal`, which returns a parse error.

### How ParseHeadlessVerdictResult handles this (session/backlog_review.go lines 161-182)

`ParseHeadlessVerdictResult` uses a completely different, more robust strategy: it searches for the outermost JSON object in the text using `strings.Index(text, "{")` and `strings.LastIndex(text, "}")`, extracting the substring between them. This tolerates prose before and after the JSON, and completely ignores fences.

**There is no shared utility** between the two parsers. `ParseHeadlessTriageResult` uses the prefix-strip approach; `ParseHeadlessVerdictResult` uses the brace-scan approach. The fix for the triage parser is to adopt the same brace-scan strategy from `ParseHeadlessVerdictResult`.

### Existing test coverage (session/backlog_triage_test.go)

The test file covers:
- `TestParseHeadlessTriageResult_ValidJSON` — bare JSON, no fences
- `TestParseHeadlessTriageResult_StripsMarkdownFences` — leading ` ```json ` fence (the happy path that works)
- `TestParseHeadlessTriageResult_InvalidJSON` — malformed JSON
- `TestParseHeadlessTriageResult_CapsTasksAt12` — task cap logic
- `TestParseHeadlessTriageResult_EmptySuggestionsOK` — empty suggestions

**Missing test**: no test for prose-before-fence (`"Here is the JSON:\n```json\n{...}\n```"`). This is the brittleness case that needs a new test.

## 3. E2E Test Server Mock Capability

### How the test server starts (tests/e2e/helpers/test-server.ts lines 61-70)

```ts
this.process = spawn(this.config.buildPath, [
  '--test-mode',
  '--test-dir', this.config.testDir,
  '--tmux-keep-server',
], { env: { PORT: ... } });
```

Only three flags are used: `--test-mode`, `--test-dir`, `--tmux-keep-server`. There is **no `--mock-triage` flag** or similar. The test server runs the real binary with no headless pool mock.

### Headless pool initialization in backlog_service.go

At line 1174-1177, TriggerTriage returns `CodeUnimplemented` if `s.headlessPool == nil`:
```go
if s.headlessPool == nil {
    return nil, connect.NewError(connect.CodeUnimplemented,
        fmt.Errorf("headless pool not available — ensure claude binary is installed"))
}
```

The headless pool is only initialized when the `claude` binary is available on the system. In test environments where `claude` is not installed, `headlessPool` is nil and `TriggerTriage` will return an error before reaching the actual triage logic.

### Recommended e2e test approach

A `--mock-triage` server flag is not yet the right approach because:
1. The test server has no mechanism to inject pre-configured item state with triage results.
2. The UI gate bug (Bug 1) can be fully tested **without triggering the RPC** — an e2e test can verify the button is `disabled` when `repoPath` is absent.
3. For parser robustness (Bug 2), unit tests in Go are the right layer; no e2e test needed.

The practical e2e strategy is:
- **For Bug 1**: Create a backlog item without a `repoPath` (or with it empty), navigate to the detail page, and assert `data-testid="backlog-action-trigger-triage"` is disabled. Then set `repoPath` and assert it becomes enabled.
- **For the RPC call path**: Either seed an item with a `repoPath` pointing at the test's own temp directory (which exists on the filesystem), or mock at the API boundary using Playwright route interception for `triggerTriage`.

The test server's `seedDemoData` calls `go run ./tests/demo/seed`, which is where pre-configured backlog items with/without `repoPath` could be seeded deterministically without a `--mock-triage` flag.

## 4. BacklogItemCard Triage Guard Analysis

### Current state of BacklogItemCard.tsx (lines 22-48)

```typescript
function getActionSpec(item: BacklogItem): ActionSpec {
  switch (item.status) {
    case "ready":
      return { label: "Trigger Triage", action: "trigger_triage" };
      // ^ NO disabled: !item.repoPath guard
    case "idea":
      return {
        label: "Mark Ready",
        action: "mark_ready",
        disabled: item.acCriteria.length === 0,  // idea has a guard (AC count)
      };
    ...
  }
}
```

The button at line 123-136 respects `actionSpec.disabled` and `isTriageRunning`:
```tsx
disabled={actionSpec.disabled || isTriageRunning}
```

So adding `disabled: !item.repoPath` to the `"ready"` case in `getActionSpec` is sufficient to fix the card — no other changes to the render logic are needed.

### Board page (`web-app/src/app/backlog/board/page.tsx`)

The board page `handleAction` (line 31-53) routes `"trigger_triage"` to `triggerTriage(itemId)` with no guard. The guard must live in `getActionSpec` in `BacklogItemCard.tsx` (which `BacklogBoard` renders), not in the board page handler. The board page itself does not need changes beyond what the card component enforces.

## Summary of Fix Targets

| Bug | File | What to Change |
|-----|------|---------------|
| UI gate (Bug 1) — detail page | `web-app/src/components/backlog/BacklogItemDetail.tsx` lines 590-597, 603-610 | Add `disabled={actionLoading \|\| !item.repoPath}` + `title` tooltip to both "Trigger Triage" buttons |
| UI gate (Bug 1) — card/board | `web-app/src/components/backlog/BacklogItemCard.tsx` `getActionSpec` "ready" case | Add `disabled: !item.repoPath` |
| Parser (Bug 2) | `session/backlog_triage.go` `ParseHeadlessTriageResult` | Replace prefix-strip strategy with brace-scan strategy from `ParseHeadlessVerdictResult` |
| Parser test | `session/backlog_triage_test.go` | Add test for prose-before-fence input |
| E2e | `tests/e2e/` (new file) | Assert disabled state for items without repoPath; use Playwright route interception for RPC call coverage |
