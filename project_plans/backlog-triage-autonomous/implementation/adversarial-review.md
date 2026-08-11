# Adversarial Review: backlog-triage-autonomous

**Date**: 2026-06-22
**Verdict**: CONCERNS (0 blockers / 5 concerns / 4 minors)

---

## Blockers

_None._

---

## Concerns

- [ ] **Pool not wired into BacklogService — silent no-op on deploy** — `dependencies.go` line 755 creates `backlogSvc` with no pool argument: `services.NewBacklogService(storage, sessionService, cfg, workflowEngine)`. The plan adds a `triageSem` field and calls `pool.CallBlockingWithOptions` inside the goroutine, but `BacklogService` has no `headlessPool` field and no `SetHeadlessPool` method — unlike `ApprovalHandler` and `BacklogLifecycleListener` which each have their own wiring call. The plan's Phase 2 task list says "wire in server.go" but provides no concrete code for the `dependencies.go` change or a `SetHeadlessPool` method. If this step is missed, `TriggerTriage` will either panic (nil pool dereference) or silently succeed without doing any LLM work, depending on how the guard is written. **Recommendation**: explicitly add `backlogSvc.SetHeadlessPool(headlessPool)` after line 756 in `dependencies.go` to the task checklist, and add a nil-pool guard in `TriggerTriage` that returns `CodeUnavailable` synchronously (not inside the goroutine where it's invisible to the caller).

- [ ] **Auto-triage from `CreateBacklogItem` bypasses the new headless path** — `CreateBacklogItem` (line 413) gates auto-triage on `s.sessionCreator != nil`, not on the headless pool. After the rewrite, `TriggerTriage` will no longer call `s.sessionCreator`, but the auto-triage trigger still checks `sessionCreator`. If `sessionCreator` is nil (headless-only environment), auto-triage is silently skipped even though the headless pool is available. The plan says nothing about updating this guard. **Recommendation**: update the auto-triage gate in `CreateBacklogItem` to also fire when `s.headlessPool != nil`, or replace both guards with a single `s.canTriage()` helper.

- [ ] **No server-shutdown cancellation for in-flight triage goroutines** — `BacklogLifecycleListener` has a `shutdownCtx` + `Shutdown()` method that cancels in-flight review gate calls on server stop. The plan spawns triage goroutines with a `context.WithTimeout(context.Background(), 30*time.Minute)` child context. There is no equivalent shutdown hook for these goroutines in `BacklogService`. If the server restarts mid-triage, the 30-minute goroutine runs until it hits the database and gets a connection error; the `ItemSession` may never get `UpdateItemSessionEnded` called, leaving it in a permanently open (never-ended) state that will block future re-triggers via the orphan guard. **Recommendation**: add a `shutdownCtx context.Context` / `shutdownCancel context.CancelFunc` pair to `BacklogService` (same pattern as `BacklogLifecycleListener`), derive the 30-min timeout from it, and call `backlogSvc.Shutdown()` during server teardown.

- [ ] **WorkDir + FakeRunner incompatibility breaks the integration test** — `CallWithOptions` with `opts.WorkDir != ""` requires a `*ProcessRunner` (caller.go line 342: `"CallWithOptions: WorkDir requires a ProcessRunner; got %T"`). The integration test plan (Task 4.1.2a) calls `headless.NewPoolWithRunner(cfg, FakeRunner)` and then exercises the goroutine body which calls `pool.CallBlockingWithOptions(..., CallOptions{WorkDir: item.RepoPath})`. This will return an error immediately, not simulate the triage flow. The existing test at `pool_test.go:478` (`TestPool_CallWithOptions_WorkDir_FakeRunner_ReturnsError`) explicitly documents that WorkDir + FakeRunner returns an error. **Recommendation**: either (a) use an empty `WorkDir` in the test and verify goroutine outcomes using the fake's responses, or (b) make `CallWithOptions` accept a `WorkDir` when running with `FakeRunner` for test purposes by checking for a test-only interface.

- [ ] **Type duplication creates a schema-drift risk** — `triageSuggestion` and `triageTask` are defined in `server/mcp/tools_backlog.go` (unexported). `backlog_service.go` already has its own mirrors (`triageSuggestionJSON`, `triageTaskJSON`) with a comment "Must stay in sync." The plan asks `session/headless/features.go` to define a third set (`HeadlessTriageResult` with its own suggestion/task structs). This will be three independent definitions of the same JSON schema. A field rename (e.g., adding `priority` to a task) requires updating all three files simultaneously; there is no compile-time enforcement. **Recommendation**: move the canonical triage payload types to `session/` package (e.g., `session/backlog_triage.go`) and import from there in both `server/mcp/` and `server/services/`. The `session/headless/` package should reference `session.TriageSuggestion` not a local copy.

---

## Minors

- The precondition check in step 8 of the goroutine (`TransitionBacklogItemStatus` with `ExpectedStatus: idea`) will fail silently and log an error if the operator manually moves the item to `refining` or `ready` mid-triage. The item never reaches `ready` via the headless path but is not retried. This is probably acceptable behavior but should be called out in the log message so operators are not confused.

- The plan removes `KillTmuxSessionByTitle` from `TriggerTriage` (Task 2.1.2a). If any stale tmux sessions exist with the `triage:<slug>` name from before the migration (e.g., from a failed previous triage), they will persist in tmux until pruned by the 30-minute reaper. The name collision risk is low since no new tmux sessions are created, but the reaper documentation should be updated to note that triage tmux orphans from pre-migration runs are handled by the existing timer.

- The plan does not address what happens when `ParseHeadlessTriageResult` returns an error (malformed LLM output). The goroutine body (Task 2.1.1c step 5) says "calls `ParseHeadlessTriageResult`" but the error handling is not specified. If parsing fails, the goroutine should persist the raw result as a debug artifact and still call `UpdateItemSessionEnded` before returning, so the operator sees a failure state rather than a permanently open session.

- The plan's "Unresolved Questions" section (line 111) notes that Claude Code headless `-p` may spawn subagents that call `submit_triage_result` MCP tool, which requires `STAPLER_SESSION_UUID`. Phase 3 confirms the headless system prompt won't include that instruction, but if a subagent is still spawned by the headless call and inherits the system prompt context, it could attempt the call anyway. This is low-risk but worth an explicit test: the system prompt should affirmatively say "Do not call submit_triage_result or any MCP tool" (matching the review gate's "Do not write any code. Do not modify any files" approach).
