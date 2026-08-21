# Pitfalls Research — backlog-triage-pipeline

**Date**: 2026-05-18  
**Scope**: Known risks, gotchas, race conditions, and failure modes.

---

## P1 — Auto-trigger goroutine race (R1)

**Risk**: If `TriggerTriage` is called as a goroutine from `CreateBacklogItem`, the parent `ctx` may be cancelled before the goroutine completes (HTTP request finishes → context cancelled → triage session spawn fails silently).

**Mitigation**: Detach context: `go func() { triageCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second); defer cancel(); s.TriggerTriage(triageCtx, ...) }()`. Never pass the request context to background goroutines.

**Alternative**: Call `TriggerTriage` synchronously before returning from `CreateBacklogItem`. This costs ~200ms but is simpler and avoids the context pitfall entirely. Recommended unless benchmarks show unacceptable latency.

---

## P2 — Double-trigger: auto + manual (R1)

**Risk**: User creates an item (auto-triage fires) and then clicks "Trigger Triage" in the UI before the first triage session finishes. This creates two concurrent `ItemSession` records with `role=triage` for the same item.

**Mitigation**: Add a guard in `TriggerTriage`:
```go
sessions, _ := s.storage.ListItemSessions(ctx, item.ID)
for _, is := range sessions {
    if is.SessionRole == "triage" && is.EndedAt == nil {
        return nil, connect.NewError(connect.CodeAlreadyExists, 
            fmt.Errorf("triage session already running"))
    }
}
```
Frontend should also hide the "Trigger Triage" button while `triageStatus === "running"`.

---

## P3 — `TriggerTriage` requires `repo_path` (R1)

**Risk**: `TriggerTriage` has a hard precondition: `item.RepoPath == ""` → returns `CodeFailedPrecondition`. Items created without a `repo_path` will fail auto-triage silently (if called from goroutine) or return an error (if synchronous).

**Mitigation**: In auto-trigger logic, check `item.RepoPath != ""` before calling `TriggerTriage`. If empty, skip auto-triage (set `triage_triggered = false` in response). Document in API that `skip_triage=true` is implied when `repo_path` is unset.

---

## P4 — `triage_result` JSON schema drift (R3)

**Risk**: `submitTriageResult` writes `{"summary": "...", "suggestions": [...]}` to `ItemSession.triage_result`. If `R3` adds `clarifying_questions` or other fields to the stored JSON but the Go unmarshal struct in `itemSessionToProto` doesn't match, fields will be silently dropped.

**Mitigation**: Define a canonical Go struct `triageResultJSON` (unexported, in `backlog_service.go` or a shared package) used for both serialization (`tools_backlog.go` write) and deserialization (`backlog_service.go` read). Never unmarshal triage JSON into `map[string]interface{}` in the mapper — use the typed struct.

---

## P5 — Ent schema migration for existing records (R3)

**Risk**: `triage_result` is already a column on `item_sessions` (verified in `item_session.go`). No new ent migration is needed for R3. However, if the proto `TriageResult` message unmarshal fails for any reason, `itemSessionToProto` must log + continue rather than returning an error (degraded rendering is better than a broken detail view).

**Mitigation**: Wrap unmarshal in `if is.TriageResult != "" { if err := json.Unmarshal(...); err != nil { log.Warn(...) } }` — existing pattern used for `PerCriterion` in `itemSessionToProto` lines ~161–174.

---

## P6 — localStorage dismissed state not shared across devices (R4)

**Risk**: If a user dismisses the triage review panel on one device/browser, it will reappear on another device. This is expected behavior for localStorage but may confuse users who already applied suggestions.

**Mitigation**: Once suggestions are applied (R5) or status transitions away from "idea", the panel condition (`item.triageStatus === "completed" && item.status === "idea"`) becomes false regardless of localStorage state. The dismissed state is therefore only needed for the "skip" path while item is still in "idea". Acceptable for v1.

---

## P7 — Apply suggestions optimistic concurrency failure (R5)

**Risk**: Between the user opening the triage panel and clicking "Apply", another process may have updated the item (e.g., manual edit, status transition). `UpdateBacklogItem` with `expected_updated_at` will return `CodeAborted`.

**Mitigation**: `UpdateBacklogItemRequest` supports `expected_updated_at` — use the `item.updatedAt` returned when the detail was last loaded. On `CodeAborted`, show an error: "Item was modified — please reload and try again." Provide a "Retry" button that calls `load()` then re-opens the apply flow. This is the same pattern as the existing optimistic concurrency in `transitionStatus`.

---

## P8 — Notification missing item title (R6)

**Risk**: `submitTriageResult` currently does not load the backlog item. To include the item title in the notification message, a DB read is required. This adds latency inside the MCP tool handler.

**Mitigation**: Read item title with `h.storage.GetBacklogItem(ctx, itemID)` before building the notification. This is a single indexed UUID lookup (O(1) by PK). If it fails, fall back to `"Item <itemID>"` as the title — notification should not fail because of a title lookup error.

---

## P9 — EventBus threading through MCP server (R6)

**Risk**: `mcp.NewCore()` currently takes 4 parameters (store, svc, sbMgr, storage). Adding `eventBus` changes this signature, which may break `mcp/server.go` callers, `server.go`, and `dependencies.go`.

**Mitigation**: Audit all callers of `mcp.NewCore()` and `mcp.NewHTTPHandler()` before changing the signature. Alternatively, add `eventBus` as an option on the `backlogHandlers` struct with a setter, keeping `NewCore` signature stable. The option-setter approach is safer for existing tests.

---

## P10 — `submit_triage_result` role check and existing tests (R6)

**Risk**: `submitTriageResult` has a strict `itemSession.SessionRole != "triage"` guard. Any test that constructs a `backlogHandlers` and calls `submitTriageResult` must use a triage-role item session. Existing tests in `tools_backlog_test.go` may not cover the notification path.

**Mitigation**: Check `tools_backlog_test.go` for coverage gaps before adding notification logic. Add a test case that verifies the eventBus `Publish` is called when `submitTriageResult` succeeds. Use a mock/stub `EventBus` in test setup.

---

## P11 — Vagueness prompt + auto-triage timing (R2)

**Risk**: The vagueness prompt (R2) is shown after `createBacklogItem` resolves. If auto-triage is async (goroutine), triage may complete before the user sees or responds to the vagueness prompt. The user clicks "Refine before triage" but triage has already finished.

**Mitigation**: The "Run triage anyway" and "Refine before triage" options should check the current `triageStatus` (poll or use item from response). If `triageStatus` is already `"completed"` by the time the prompt is shown, dismiss it automatically and show the triage review panel (R4) instead.

**Simpler mitigation**: Use synchronous auto-trigger (not goroutine). Then `triage_triggered` in the response is reliable and triage has not started executing yet (the triage session is spawned but Claude hasn't processed anything). The vagueness prompt is always shown before real triage work begins.

---

## P12 — `triageStatus` "failed" state not tracked (R4)

**Risk**: The current `triageStatus` derivation in `useBacklogService.ts` (lines ~160–165) marks a session as `"completed"` if `endedAt` is set, regardless of whether triage succeeded or failed. If the triage session crashed, the UI would incorrectly show the review panel with no triage result.

**Mitigation**: In `mapBacklogItem`, check that `triageSession.endedAt !== undefined && linkedSession.triageResult?.summary` is non-empty before setting `triageStatus = "completed"`. If the session ended but `triageResult` is null/empty, set `triageStatus = "failed"` so the UI can show an appropriate message.

---

## P13 — `buildTriagePrompt` clarifying questions + one-shot session lifecycle (R7)

**Risk**: Triage sessions are spawned with `oneShot = true` (`CreateDirectorySession(..., true)`). One-shot sessions terminate after the agent finishes. If the agent asks a clarifying question and pauses to wait for user input, the session may be killed by the one-shot timeout before the user responds.

**Mitigation**: Investigate whether `oneShot=true` sessions have a timeout or are simply flagged for cleanup after natural exit. If they have a hard timeout, R7 (clarifying questions that require waiting) may require `oneShot=false` for triage sessions. This is a significant behavioral change — needs confirmation with session lifecycle code (`session/instance.go` and `tmux` session management). For v1, the prompt can instruct the AI: "If the user does not respond within 5 minutes, continue with available context" to reduce session lifetime.

---

## P14 — Proto field numbering and wire compatibility

**Risk**: Adding fields to existing proto messages must use the next available field number and never reuse deleted numbers. Current `ItemSession` message has fields 1–10. Adding `triage_result` as field 11 is safe. `CreateBacklogItemRequest` has fields 1–8 — add `skip_triage = 9`. `CreateBacklogItemResponse` has field 1 — add `triage_triggered = 2`.

**Mitigation**: Run `make generate-proto` after every proto change. Check generated TypeScript bindings compile correctly before committing. Do not use `reserved` fields — just document the numbering in the proto comment.
