# Adversarial Review: Workflow History, Archiving & Pause Memory Optimization

**Reviewer posture:** Assume every guard condition is wrong until proven otherwise. Assume every missing test is a shipped bug.

---

## Issue 1 — Double-archive race in `maybeAutoArchive` (REAL BUG — low severity)

**Location:** `server/services/session_service.go` lines ~3616–3626

**Code:**
```go
func (s *SessionService) maybeAutoArchive(inst *session.Instance) {
    if inst == nil || inst.WorkflowID == "" || inst.ArchivedAt != nil {
        return
    }
    now := time.Now()
    inst.ArchivedAt = &now
    ...
}
```

**Problem:** The `inst.ArchivedAt != nil` guard and the subsequent `inst.ArchivedAt = &now` write are not protected by `inst.stateMutex`. The `autoArchiveListener.OnLifecycleEvent` spawns a goroutine for each `EventExited` event. If two `EventExited` events fire concurrently (possible during a controller-exit / forced-kill race), two goroutines can both read `inst.ArchivedAt == nil`, both pass the guard, and both write different `now` values to `inst.ArchivedAt`. The result is a benign double-write (same field, two close timestamps) rather than corruption, but it causes `SaveInstances` to be called twice, doubling DB writes and potentially producing a misleading audit timestamp.

**Risk:** Low — the outcome is a redundant DB write and a slightly incorrect archive timestamp, not data loss. But the code has an undocumented data race that the Go race detector will flag.

**Fix:** Wrap the read-check-write in `inst.stateMutex.Lock()` / `Unlock()`, or use a `sync/atomic` compare-and-swap on a separate `uint32` archived flag.

---

## Issue 2 — `ListSessions` default filter excludes archived sessions: impact on internal callers (CONCERN — medium severity)

**Location:** `server/services/session_service.go` lines ~700–707

**Code:**
```go
if inst.ArchivedAt != nil && !req.Msg.IncludeArchived {
    continue
}
```

**Problem:** The `ListSessions` RPC handler now silently drops archived sessions unless `IncludeArchived: true`. This is a behavioral breaking change for any caller that goes through the RPC layer expecting a full session set.

**Audit of affected callers:**
- Internal callers using `reviewQueuePoller.GetInstances()` (direct in-memory slice) are unaffected.
- `loadInstancesWithWiring` calls `s.storage.LoadInstances()` directly — unaffected.
- The `WatchSessions` streaming path reads live instances directly — unaffected.
- **`listSessionsByWorkflow` in `useSessionService.ts`** passes `includeArchived: true` — correct, avoids the filter.
- Any future internal Go caller that uses `ListSessions` as a library function (e.g., a background job added later) will silently miss archived sessions.

**Finding:** The current implementation is safe for the existing call sites because all internal Go callers bypass the RPC handler and the one frontend caller that needs archived sessions passes `includeArchived: true`. However, the risk is latent: the filter is not documented in the RPC contract (proto comment only says "Exclude archived sessions"), and future contributors may add an internal `ListSessions` call without knowing to pass the flag.

**Recommendation:** Add a comment to the `ListSessions` handler stating that internal callers should use `storage.LoadInstances` directly, and add `include_archived` to the proto field comment.

---

## Issue 3 — Resume reinit: TmuxBackend type assertion silently skips dead-session rebuild for non-tmux backends (CONCERN — low severity in production, medium in tests)

**Location:** `session/instance.go` lines ~1088–1097

**Code:**
```go
if tb, ok := i.processManager.(*TmuxBackend); ok {
    // ... SetSession, SetExtraEnv
}
if err := i.pm().Start(worktreePath); err != nil {
```

**Problem:** The type assertion `i.processManager.(*TmuxBackend)` silently no-ops for any process manager that is not `*TmuxBackend`. The subsequent `i.pm().Start(worktreePath)` call will then reuse the existing (stale) session object, launching the session **without** the `--resume <uuid>` flag. For production systems where all managed sessions use `TmuxBackend` this is harmless. For test fakes or future alternative backends (e.g., a hypothetical `DockerBackend`) the resumed session would start a fresh Claude conversation silently.

**Finding:** A missing type assertion failure means the dead-tmux path degrades gracefully to "wrong behavior, no error" rather than "correct error, no behavior." This is specifically flagged in `research/pitfalls.md` but no guard (log line or returned error) was added for the non-TmuxBackend case.

**Fix:** Add `else { log.Warn("resume: processManager is not a TmuxBackend, skipping session reinit", "type", fmt.Sprintf("%T", i.processManager)) }` so operators and test authors can see when the reinit is skipped.

---

## Issue 4 — "Show archived" toggle in main session list was in-scope per task doc but not implemented (BLOCKED scope gap)

**Location:** `docs/tasks/workflow-history-and-archiving.md` Story 4 (`Task 4.1` and `Task 4.2`)

The task document lists Story 4 (archive button in session list + "Show archived" filter toggle) as in-scope. The `requirements.md` explicitly defers it:

> "Show archived" toggle in main session list (deferred)

The implementation follows `requirements.md` and does not implement Story 4. The `archiveSession` and `unarchiveSession` hooks exist in `useSessionService.ts` but are not wired to any UI. There is no way for a user to manually archive a session or recover an archived session from the UI. The only archiving that occurs is automatic (workflow sessions on exit).

**Impact:** Users cannot manually archive sessions to clean up the list (US-3 acceptance criteria not met). Users cannot view or recover archived sessions via the UI (no "Show archived" toggle anywhere).

**Verdict for this gap:** The requirements doc explicitly defers this, so it is not a bug in the implementation — but it is a shipped scope gap that the task document implied was in-scope. This needs a JIRA ticket or a follow-up PR. The PR description should call this out explicitly.

---

## Issue 5 — Run button does not navigate to created session (minor UX gap)

**Location:** `web-app/src/components/workflows/WorkflowsPanel.tsx` lines ~135–139

**Code:**
```typescript
async function handleRun(wf: WorkflowProto) {
    setRunningId(wf.id);
    await runWorkflow({ id: wf.id });
    setRunningId(null);
}
```

**Problem:** `runWorkflow` returns a `sessionId` (string | null) but `handleRun` discards the return value. US-1 acceptance criterion states: "After creation, the session is visible in the session list." The requirement also implies (from the task doc): "navigates to created session." No navigation occurs.

**Impact:** The Run button triggers session creation and shows a spinner, but the user stays on the Workflows page with no indication of where to find the session. The session does appear in the main list, satisfying US-1's minimal criterion, but the "jump to it immediately" language in the acceptance criteria is not met.

**Fix:** `const sessionId = await runWorkflow({ id: wf.id }); if (sessionId) router.push(`/?session=${sessionId}`);`

---

## Issue 6 — Ent codegen flag verification

**Constraint from CLAUDE.md:**
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

**Verification:** The `session/ent/schema/session.go` has the correct new fields. The generated `session/ent/session.go` (not directly read, but inferred from `ent_repository.go` calling `SetWorkflowID`, `ClearWorkflowID`, `SetArchivedAt`, `ClearArchivedAt` without compile errors) must have been generated correctly. The presence of `UpsertRule` and similar methods in the repository implies the `--feature sql/upsert` flag was used. **Confidence: high** that codegen was run correctly, but the flag cannot be directly verified from git history without inspecting the `go.sum` or build logs.

**Recommendation:** No action required if `make build` passes cleanly. If the build is not confirmed passing, run `make build` before merging.

---

## Issue 7 — `RecentRuns` loads archived sessions but shows no visual indicator (minor UX)

**Location:** `WorkflowsPanel.tsx` lines ~42–43

```typescript
const sessions = await listSessionsByWorkflow(workflowId, true);
setRuns(sessions.slice(-5).reverse());
```

The `includeArchived: true` flag is correct — archived workflow sessions should appear in run history. However, the run row renders only status badge + title + timestamp with no "archived" indicator. A session that was auto-archived (status `STOPPED`, `archived_at` set) will appear identical to a session that simply stopped but was not archived. This is cosmetically minor but could confuse users who see a stopped session in run history and cannot tell whether it was archived.

---

## Summary

| Issue | Severity | Type |
|---|---|---|
| Double-archive race in `maybeAutoArchive` | Low | Data race (benign double-write) |
| `ListSessions` default filter impact on future callers | Medium | Latent breakage risk |
| Resume reinit silent skip for non-TmuxBackend | Low (prod) / Medium (tests) | Missing observability |
| "Show archived" toggle not implemented | Scope gap | Missing US-3 UI |
| Run button does not navigate to session | Minor | Partial US-1 acceptance |
| Ent codegen flag: inferred correct, not directly verified | Informational | Build hygiene |
| No "archived" indicator in RecentRuns | Minor | UX gap |

---

## Verdict

**CONCERNS**

The implementation is functionally correct for the happy path: workflow sessions auto-archive on exit, run history is queryable, the Run button works, Pause frees memory, Resume reattaches correctly. No data is lost and no crash paths are introduced.

However, four issues should be resolved before shipping:

1. The data race in `maybeAutoArchive` should be fixed (add mutex around the nil-check + write).
2. The Run button should navigate to the created session (one-line fix).
3. The non-TmuxBackend silent skip in the Resume path should get a warn log.
4. Story 4 (session list archive button + filter toggle) should be explicitly called out as deferred in the PR description, with a follow-up ticket created, since the task doc listed it as in-scope.

None of these are BLOCKING for a beta/internal release, but item 1 will be flagged by `go test -race` and item 2 is a UX regression from the acceptance criteria.
