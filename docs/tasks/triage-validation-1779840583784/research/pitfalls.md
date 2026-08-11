# Pitfalls and Gaps — Triage Pipeline Test Coverage

This document identifies specific risks and test coverage gaps in the triage pipeline.
It is input to the test plan for the backlog item "Validate that the triage pipeline
correctly injects a prompt into Claude and receives results."

Sources examined:
- `server/services/backlog_service_test.go`
- `server/services/backlog_service.go` (TriggerTriage, buildTriagePrompt)
- `server/mcp/tools_backlog.go` (submitTriageResult)
- `server/mcp/tools_backlog_test.go`
- `tests/e2e/backlog.spec.ts`

---

## 1. What is NOT tested in backlog_service_test.go

### What IS tested (for orientation)

- `TestCreateBacklogItem_SkipsTriageWhenSkipTriageTrue` — skip_triage=true prevents spawn
- `TestCreateBacklogItem_SkipsTriageWhenRepoPathEmpty` — no repo_path prevents spawn
- `TestTriggerTriage_DoubleTriggerGuard` — CodeAlreadyExists when triage session already live
- `TestTriggerTriage_HungSessionTimedOut` — hung session tombstoned after maxTriageDuration
- `TestTriggerTriage_ConcurrentCallsSameItem` — sync.Map TOCTOU guard blocks second caller
- `TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet` — auto-trigger attempted when repo_path set (uses errSessionCreator, verifies graceful failure)
- `TestItemSessionToProto_MapsTriageResult` — valid triage JSON deserialized correctly
- `TestItemSessionToProto_HandlesInvalidTriageResultJSON` — malformed JSON yields nil TriageResult without panic

### What is NOT tested

GAP-01: No test calls `TriggerTriage` with a successful `mockSessionCreator` and verifies
the returned `ItemSession.SessionRole == "triage"` and `ItemSession.SessionUUID == inst.UUID`.
The happy-path session creation and DB record linkage is untested end-to-end.

GAP-02: No test verifies the actual prompt content passed to `CreateDirectorySession`.
`mockSessionCreator` records `calls[].prompt`, but no test asserts on that field.
The `buildTriagePrompt` function is not unit-tested in isolation. There is no assertion
that the prompt contains `item_id`, the artifact path, the item title/description, or
the `submit_triage_result` instruction. If the prompt format regresses, no test catches it.

GAP-03: No test verifies that the `ItemSession` record created by `TriggerTriage` carries
the correct `item_id` linkage. The happy path is only reachable in tests where
`mockSessionCreator` returns a real `*session.Instance{UUID: ...}`. The current test that
reaches `CreateDirectorySession` (`TestCreateBacklogItem_AutoTriggersTriageWhenRepoPathSet`)
uses `errSessionCreator` which always errors before the `CreateItemSession` call — so the
linkage is never exercised by tests.

GAP-04: The `triage:` title prefix and `backlog:triage` tag applied to the spawned session
are not asserted anywhere. Tests cannot catch a regression where the title is malformed
or the tag is missing.

GAP-05: The `oneShot` / `hidden` arguments passed to `CreateDirectorySession` from
`TriggerTriage` are not verified. The current `mockCreateCall` struct captures `oneShot`
but no test checks its value for the triage path.

GAP-06: `TriggerTriage` on an item already in `ready` status transitions it back to `idea`
(step 3c). This behavior is untested — no test sets up a ready item, calls TriggerTriage,
and verifies the item is reset to idea.

GAP-07: The `KillTmuxSessionByTitle` call (step 4.5) is called on `mockSessionStopper`
when a stopper is wired, but no test verifies it is invoked with the correct title
`"triage:" + slug` before spawn.

---

## 2. Prompt Injection Path

### Where the prompt is set

`TriggerTriage` (backlog_service.go line 1179) calls:

```go
triagePrompt := buildTriagePrompt(item, artifactAbsPath, slug)
```

Then at line 1184:

```go
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, !useAutonomous /*oneShot*/, true /*hidden*/)
```

The prompt is the third positional argument to `CreateDirectorySession`. In the real
implementation (`session_service.go` line 597), this maps to `opts.Prompt`, which becomes
the positional `-p` CLI argument to claude (via `buildLaunchCommand` when `OneShot=true`)
or is sent as the initial user message via the session driver.

### GAP-08: No test verifies the prompt reaches the Instance

`buildTriagePrompt` is a private package-level function. It is not unit-tested. No test
calls `buildTriagePrompt` directly and asserts on the output. No test verifies that the
value passed to `CreateDirectorySession.prompt` equals the output of `buildTriagePrompt`.

The prompt must contain:
- `item_id: <uuid>` — so Claude passes the correct ID to `submit_triage_result`
- `artifactAbsPath` — so Claude writes research files to the correct location
- `submit_triage_result` — the MCP tool name Claude must call

None of these are asserted by any test. A copy-paste error or refactor could produce
a prompt missing the item_id, causing the MCP call to fail or reference the wrong item.

### How the prompt reaches Claude (real path)

In the non-test path: `CreateDirectorySession` in `session_service.go` sets
`opts.Prompt = prompt`. `session.NewInstance` stores this. When `oneShot=true`,
`buildLaunchCommand` in `instance_tmux.go` emits `-p <prompt> --output-format json`.
When `autonomousStarter != nil` (the normal production path), `oneShot=false` and the
prompt is delivered by `StartSessionDriver` / `AutonomousDriver` as the first user message.

There is no unit test covering this full injection chain. It requires tmux/process
infrastructure.

---

## 3. MCP Result Path — submit_triage_result

### What IS tested in tools_backlog_test.go

- `TestSubmitTriageResult_PublishesNotificationOnSuccess` — verifies the EventBus receives
  a notification with `item_id` in metadata and "Triage complete" title.
- `TestSubmitTriageResult_NoNotificationWhenEventBusNil` — verifies no panic when eventBus is nil.

### GAP-09: No test verifies the triage result is persisted to storage

Neither test calls `storage.UpdateItemSessionTriageResult` indirectly and then reads back
the value to confirm it was stored. The test checks the EventBus notification but not that
the `triage_result` JSON field on the `ItemSession` row was actually written with the
correct content (summary, suggestions, tasks).

### GAP-10: No test for role enforcement in submit_triage_result

`submitTriageResult` at line 452 checks `itemSession.SessionRole != "triage"` and returns
`PERMISSION_DENIED`. No test exercises this guard — a session with role="work" calling
`submit_triage_result` is untested.

### GAP-11: No test for plan_artifact_path persistence

`submitTriageResult` optionally calls `storage.UpdateBacklogItem` to set
`plan_artifacts_path` when `plan_artifact_path` arg is non-empty. No test passes this
argument and verifies the field is written to the BacklogItem row.

### GAP-12: No test for tasks > 12 truncation

`submitTriageResult` truncates `tasks` to 12 entries silently. No test verifies this
cap behavior.

---

## 4. Session-Item Linkage

### GAP-13: No test verifies item_id ↔ session_uuid linkage after TriggerTriage succeeds

`TriggerTriage` creates an `ItemSession` record (step 9, line 1194-1199) linking
`item.ID` to `inst.UUID` with `SessionRole=triage`. No test in `backlog_service_test.go`
reaches this code path with a successful creator — `errSessionCreator` always returns
an error before `CreateItemSession` is called.

Therefore: if `CreateItemSession` were called with the wrong `ItemID` or wrong `SessionUUID`,
no unit test would catch it.

### GAP-14: submit_triage_result permission check depends on correct linkage

`submitTriageResult` calls `GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)` to
confirm the session is linked before persisting. This is the only gate preventing a rogue
session from submitting triage results for an arbitrary item. The linkage created by
`TriggerTriage` must be exactly correct for this check to work.

Since GAP-13 exists (linkage creation is untested), a regression in the item_id passed
to `CreateItemSession` would cause `submit_triage_result` to always return `PERMISSION_DENIED`
for the triage session — silently breaking the pipeline.

---

## 5. Race Conditions — sync.Map TOCTOU

### What IS tested

`TestTriggerTriage_ConcurrentCallsSameItem` tests the `triggerInProgress` sync.Map guard
by manually calling `svc.triggerInProgress.Store(itemID, struct{}{})` to simulate a
concurrent caller. The test verifies `CodeAlreadyExists` is returned.

### GAP-15: The test is not a true concurrent test

The test simulates concurrency by pre-populating the sync.Map before calling
`TriggerTriage`. It does not spawn two goroutines to actually race. This means:
- The test cannot catch a case where the lock is acquired too late (after the orphan
  guard but before spawn).
- A data race between the `LoadOrStore` and `Delete` (defer) is not exercised.

However, this is likely acceptable as a structural test — the sync.Map `LoadOrStore`
atomicity is guaranteed by the standard library. The real risk is the window between the
orphan-guard DB check (step 3b) and the `LoadOrStore` (step 3a). In the current
implementation, `LoadOrStore` is before the orphan guard (step 3a is at line 1097,
step 3b at line 1110), so the lock is held for the entire guard window. This is correct
but is not explicitly tested.

GAP-16: No test verifies that after `TriggerTriage` completes (or errors), the
`triggerInProgress` entry is removed (the `defer Delete` fires). If the defer were
accidentally removed, the item would be permanently locked from re-triggering.

---

## 6. Malformed JSON from Claude

### What IS tested

- `TestItemSessionToProto_HandlesInvalidTriageResultJSON` (backlog_service_test.go line 514)
  verifies that `itemSessionToProto` does not panic on malformed `triage_result` JSON and
  returns `nil` for `TriageResult`.

### What is NOT tested

GAP-17: `submitTriageResult` in `tools_backlog.go` receives suggestions and tasks as
`[]interface{}` from MCP and calls `json.Marshal` + `json.Unmarshal` per item. No test
passes a suggestion or task with unexpected fields (extra keys, wrong types, partial
objects) to verify the handler either rejects them gracefully or accepts them leniently.

GAP-18: No test verifies what happens if Claude calls `submit_triage_result` with
`summary` = "" (empty string). The handler checks `summary != ""` and returns
`ErrInvalidArgument`. This is a plausible Claude failure mode (empty response after
context limit) and is untested in `tools_backlog_test.go`.

GAP-19: No test verifies that `submitTriageResult` returns a structured error (not panic)
when `storage.UpdateItemSessionTriageResult` fails (e.g. DB write error). The handler
logs and returns `errResult(ErrInternalError, ...)` — this error path has no test.

---

## 7. Dead Session Before submit_triage_result

### What happens (code analysis)

If the triage session dies (tmux crash, OOM kill, Claude exits without calling the MCP
tool), the following occurs:

1. `onSessionExited` in `session/backlog_lifecycle.go` fires. For a triage session
   (role != "work"), it calls `UpdateItemSessionEnded` to stamp `ended_at`.
2. The `ItemSession` record is closed with no `triage_result` stored.
3. The `BacklogItem` remains in `idea` status — no status transition is triggered for
   triage exit.
4. The operator sees no notification (no `EventBus.Publish` call fires).
5. `ApprovePlan` will fail with `CodeFailedPrecondition` because
   `plan_artifacts_path` was never set.

GAP-20: No test verifies that a triage session dying without calling `submit_triage_result`
leaves the `ItemSession.ended_at` stamped and the `BacklogItem` in a re-triggerable state.
Specifically: after the session is tombstoned, does `TriggerTriage` correctly identify
the old session as orphaned (via `notLive || timedOut`) and allow a new trigger?

This interacts with GAP-16 — the `ended_at` stamp is what allows re-trigger. If
`UpdateItemSessionEnded` failed silently or was called with the wrong session ID, the
item could be permanently stuck in the "triage running" state.

GAP-21: No test verifies the operator receives any signal (log message, notification,
status change) when a triage session dies without completing. The current implementation
produces no user-visible signal — this is a usability gap, not a crash, but it means
the operator must manually notice the stuck item.

---

## Summary of GAPs

| ID     | Area                     | Severity | Description |
|--------|--------------------------|----------|-------------|
| GAP-01 | Session creation linkage | HIGH     | Happy-path TriggerTriage session UUID linkage untested |
| GAP-02 | Prompt content           | HIGH     | buildTriagePrompt output not asserted; item_id/path/tool name could regress silently |
| GAP-03 | ItemSession linkage      | HIGH     | CreateItemSession called with correct item_id never verified by tests |
| GAP-04 | Session title/tags       | MEDIUM   | "triage:" prefix and "backlog:triage" tag not asserted |
| GAP-05 | oneShot/hidden args      | MEDIUM   | Flag values passed to CreateDirectorySession not asserted |
| GAP-06 | Status reset to idea     | MEDIUM   | ready→idea transition on re-triage not tested |
| GAP-07 | KillTmuxSessionByTitle   | LOW      | Pre-spawn tmux kill not verified with correct title |
| GAP-08 | Prompt → Instance        | HIGH     | No test that prompt value contains required content |
| GAP-09 | Triage result persistence| HIGH     | submit_triage_result does not verify storage write in tests |
| GAP-10 | Role enforcement         | MEDIUM   | submit_triage_result role != "triage" guard not tested |
| GAP-11 | plan_artifact_path write | MEDIUM   | BacklogItem.plan_artifacts_path not verified after submit |
| GAP-12 | Tasks > 12 truncation    | LOW      | Cap at 12 tasks not tested |
| GAP-13 | item_id↔session linkage  | HIGH     | ItemSession linkage after successful TriggerTriage untested |
| GAP-14 | submit permission gate   | HIGH     | Gate depends on correct linkage; linkage creation untested |
| GAP-15 | TOCTOU test realism      | LOW      | Concurrent test does not use goroutines; structural-only |
| GAP-16 | triggerInProgress cleanup| MEDIUM   | defer Delete not verified after TriggerTriage returns |
| GAP-17 | Malformed suggestion JSON| MEDIUM   | Extra/wrong fields in suggestion/task objects not tested |
| GAP-18 | Empty summary            | MEDIUM   | Claude producing empty summary: ErrInvalidArgument path untested |
| GAP-19 | Storage write failure    | MEDIUM   | UpdateItemSessionTriageResult DB error path untested |
| GAP-20 | Dead session re-trigger  | HIGH     | Item re-triggerable after session dies without submitting |
| GAP-21 | No operator signal       | LOW      | No notification when triage dies silently |
