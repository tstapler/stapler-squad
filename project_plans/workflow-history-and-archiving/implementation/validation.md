# Validation Plan: Workflow History, Archiving & Pause Memory Optimization

**Date:** 2026-06-12
**Based on:** requirements.md, plan.md (post-implementation), adversarial-review.md

---

## Requirements Coverage Map

### US-1: Run a workflow from the Workflows page

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| Each workflow row has a "Run ▶" button that calls `RunWorkflow` RPC | Jest: render `WorkflowsPanel`, stub `runWorkflow`, click "▶ Run" button, assert `runWorkflow` called with correct `{ id }` | Unit |
| Button shows a loading state while session is being created | Jest: assert button is disabled and shows `…` while `runningId === wf.id` | Unit |
| After creation, session is visible in session list | Integration: call `RunWorkflow` RPC directly, then `ListSessions`; assert returned list contains the new session ID | Integration |
| **[PARTIAL — adversarial Issue 5]** "Jump to it immediately" — Run button navigates to created session | Manual: click Run on a workflow, verify browser navigates to `/?session=<id>`. **Currently not implemented** — blocked by adversarial Issue 5; must be addressed before AC is fully met | Manual |

### US-2: See recent run history per workflow

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| Each workflow row has a ▸ "Recent Runs" toggle | Jest: render `WorkflowsPanel`, assert toggle element is present for each workflow row | Unit |
| Expanding shows status badge, session title, timestamp | Jest: mock `listSessionsByWorkflow` returning 3 sessions, expand accordion, assert status badge + title + timestamp rendered for each | Unit |
| Each run is a clickable link to the session | Jest: assert each run row contains an anchor/link whose `href` references the session ID | Unit |
| Empty state shown when no runs exist | Jest: mock `listSessionsByWorkflow` returning `[]`, expand, assert empty-state copy is rendered | Unit |
| Data loads lazily on first expand | Jest: assert `listSessionsByWorkflow` is NOT called on initial render, only after toggle click | Unit |
| Slice to last 5 runs | Jest: mock 8 sessions returned, assert only 5 items rendered | Unit |

### US-3: Archive completed sessions

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| `ArchiveSession` RPC sets `archived_at` on a session | Go unit: call `ArchiveSession` handler, assert returned session has non-nil `archived_at` and in-memory `inst.ArchivedAt != nil` | Unit |
| `UnarchiveSession` RPC clears `archived_at` | Go unit: archive then unarchive, assert `inst.ArchivedAt == nil` | Unit |
| Default `ListSessions` excludes archived sessions | Go unit: archive a session, call `ListSessions` with default request (no `include_archived`), assert session absent from result | Unit |
| `include_archived: true` filter reveals archived sessions | Go unit: same setup, call `ListSessions` with `IncludeArchived: true`, assert session present | Unit |
| **[DEFERRED — no UI]** Archive button in session list | Follow-up ticket only — see Deferred section below | — |
| **[DEFERRED — no UI]** "Show archived" filter toggle in session list | Follow-up ticket only — see Deferred section below | — |

### US-4: Auto-archive workflow sessions on completion

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| Sessions with `workflow_id` are auto-archived on exit | Go unit: create fake `Instance` with `WorkflowID != ""`, fire `EventExited` on `autoArchiveListener`, assert `ArchivedAt` set and `SaveInstances` called | Unit |
| Auto-archive fires via lifecycle hook, not poll | Code review: verify `autoArchiveListener.OnLifecycleEvent` dispatches `go maybeAutoArchive(inst)` — no ticker/poll path | Unit |
| Non-workflow sessions are never auto-archived | Go unit: create `Instance` with `WorkflowID == ""`, fire `EventExited`, assert `ArchivedAt` is nil | Unit |
| Already-archived sessions are not double-archived | Go unit: set `inst.ArchivedAt != nil`, fire `EventExited`, assert `SaveInstances` NOT called again | Unit |
| **[BUG — adversarial Issue 1]** Race condition: double `EventExited` does not produce data race | Go race test: run `go test -race` against `maybeAutoArchive` with concurrent goroutines; must pass clean. Requires mutex fix first | Unit |

### US-5: Link sessions back to their originating workflow

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| `workflow_id` optional UUID field on Session (ent schema + proto) | Build verification: `make build` succeeds; `Session` proto has fields 60 + 61; ent schema compiles | Integration |
| `FireNow` passes `workflow_id` when creating sessions | Go unit: stub `CreateSession`, call `FireNow(wf)`, assert `CreateSessionRequest.WorkflowId == wf.ID.String()` | Unit |
| `ListSessions` accepts `workflow_id` filter | Go unit: create 2 sessions with different `WorkflowID` values, call `ListSessions` with `workflow_id` filter set to one, assert only matching session returned | Unit |
| Field is orphan-safe (plain string, no FK edge) | Code review: verify `session/ent/schema/session.go` has no `edge.From("workflow", ...)` — only `field.String("workflow_id").Optional()` | Unit |
| Serialization round-trip preserves `workflow_id` and `archived_at` | Go unit: construct `Instance` with both fields set, call `ToInstanceData()` → `FromInstanceData()`, assert fields preserved | Unit |

### US-6: Pause kills tmux to free memory

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| `Pause` kills tmux session after stopping controller | Go unit: mock `TmuxBackend` with `KillSession` spy; call `Pause()`; assert `KillSession` called after controller stop | Unit |
| Claude session UUID persisted before kill | Go unit: assert `wireClaudeSessionIDSavedCallback` fires and UUID is non-empty in DB before `KillSession` is called | Unit |
| Falls back to `DetachSafely` if kill fails | Go unit: mock `KillSession` to return error; assert `DetachSafely` called next, no fatal error returned | Unit |
| Kill failure does not leave zombie tmux | Manual: start a session, pause it with `KillSession` failing (simulated), verify no orphan tmux process via `tmux list-sessions` | Manual |

### US-7: Resume correctly identifies Claude session after restart

| Acceptance Criterion | Test / Verification | Type |
|---|---|---|
| `Resume()` dead-tmux path reinitializes `TmuxSession` from `claudeSession.ConversationUUID` | Go unit: construct `Instance` with known UUID in `claudeSession.ConversationUUID`, simulate dead tmux (nil session), call `Resume()`; assert `TmuxBackend.SetSession` called with session containing `--resume <uuid>` | Unit |
| `--resume <uuid>` flag injected for Claude programs | Go unit: inspect `buildLaunchCommand(uuid)` output for Claude program, assert `--resume uuid` present in command string | Unit |
| Latest stored UUID used on resume, not stale launch command | Go unit: update `claudeSession.ConversationUUID` in DB to a newer value after initial start, call `Resume()`, assert new UUID used in launch command | Unit |
| Non-TmuxBackend logs a warning instead of silently skipping | Go unit / code review: verify the `else` branch after TmuxBackend type assertion emits a Warn log — **currently missing per adversarial Issue 3** | Unit |

---

## Non-Functional Requirements

| Requirement | Verification |
|---|---|
| `ArchiveSession` / `UnarchiveSession` respond in < 200ms | Integration benchmark: call each RPC in a Go benchmark test against an in-memory store with 1000 sessions; assert p99 < 200ms. Alternatively, manual timing with `curl` against a running instance. | 
| Auto-archive must not block exit event path | Code review: `autoArchiveListener.OnLifecycleEvent` dispatches `go maybeAutoArchive(inst)` — goroutine, not inline call. Verified in plan.md Task 2.2 description. |
| `ListSessions` default filter change must not break existing callers | Audit: enumerate all callers of `ListSessions` (Go service + TypeScript hooks). Verify: `reviewQueuePoller` uses `GetInstances()` directly, `WatchSessions` reads live instances directly, `listSessionsByWorkflow` passes `includeArchived: true`. Document in handler comment per adversarial Issue 2. |
| Pause kill must not leave zombie tmux on kill failure | Fallback path calls `DetachSafely` (see US-6 test above). Manual kill-failure simulation verifies no orphan (see US-6 manual test). |
| Data race in `maybeAutoArchive` flagged by race detector | `go test -race ./server/services/...` must pass after mutex fix (adversarial Issue 1). |
| Ent codegen ran with `--feature sql/upsert` | `make build` passes cleanly; `UpsertRule`, `SetWorkflowID`, `ClearWorkflowID`, `SetArchivedAt`, `ClearArchivedAt` exist without compile errors. |

---

## Deferred Items (explicitly out of scope — not missing)

The following items are **intentionally deferred** and must be tracked as follow-up tickets. They are listed here to confirm they are known omissions, not oversights.

| Item | Source of deferral | Follow-up |
|---|---|---|
| "Show archived" toggle in main session list (Story 4 session list UI) | `requirements.md` Out of Scope section. Conflict with task doc noted in adversarial Issue 4; task doc was wrong, requirements.md governs. | Create JIRA ticket: "Archived session UI — filter toggle + archive button in session list" |
| Run button navigation to created session | Adversarial Issue 5 flags this as a partial US-1 gap. `runWorkflow` returns `sessionId` but `WorkflowsPanel` discards it. | Create JIRA ticket or fix before merge (one-line change) |
| Non-TmuxBackend warn log on Resume reinit skip | Adversarial Issue 3 — low prod severity, medium test severity. | Fix preferred before merge (one-line `log.Warn`) |
| Bulk archive operations | `requirements.md` Out of Scope | Future epic |
| Hard-delete / TTL for archived sessions | `requirements.md` Out of Scope | Future epic |
| Workflow run notifications / webhooks | `requirements.md` Out of Scope | Future epic |
| "Archived" indicator in RecentRuns accordion | Adversarial Issue 7 — minor UX gap | Future PR |

---

## Issues Requiring Resolution Before Merge

The following issues from the adversarial review must be addressed before this feature ships:

| # | Issue | Severity | Required action |
|---|---|---|---|
| 1 | Data race in `maybeAutoArchive` (nil-check + write not under mutex) | Low (benign double-write) but race-detector-flagged | Fix: wrap nil-check and write in `inst.stateMutex.Lock()`; add `go test -race` to CI gate for this service |
| 2 | `ListSessions` default filter — latent risk for future internal callers | Medium | Add handler comment directing internal callers to `storage.LoadInstances`; add `include_archived` description to proto field comment |
| 3 | Resume reinit silent skip for non-TmuxBackend | Low (prod) / Medium (tests) | Add `else { log.Warn(...) }` branch |
| 5 | Run button discards `sessionId`, does not navigate | Minor UX | Add `if (sessionId) router.push(...)` — one-line fix |

Issue 4 ("Show archived" toggle not implemented) is a **scope gap documented as deferred**, not a blocking bug. The adversarial review label of "BLOCKED scope gap" refers to task-doc inconsistency, not a blocking implementation defect. Requirements.md governs.

---

## Test Case Inventory

| Story | Unit | Integration | Manual | Total |
|---|---|---|---|---|
| US-1 (Run workflow) | 2 | 1 | 1 | 4 |
| US-2 (Recent Runs) | 6 | 0 | 0 | 6 |
| US-3 (Archive RPCs) | 4 | 0 | 0 | 4 |
| US-4 (Auto-archive) | 4 | 0 | 0 | 4 |
| US-5 (Workflow linkage) | 4 | 1 | 0 | 5 |
| US-6 (Pause kills tmux) | 3 | 0 | 1 | 4 |
| US-7 (Resume UUID) | 3 | 0 | 0 | 3 |
| Non-functional | 2 | 1 | 1 | 4 |
| **Total** | **28** | **3** | **3** | **34** |

---

## Readiness Gate

### Criteria check

**1. Every user story has at least one acceptance criterion mapped to a test or verification step**

- US-1: 4 test/verification steps mapped. PASS (with caveat that navigation AC is currently unmet — tracked as follow-up)
- US-2: 6 unit tests. PASS
- US-3: 4 unit tests. PASS (UI deferred, documented)
- US-4: 4 unit tests + race test. PASS (race test requires Issue 1 fix)
- US-5: 5 tests. PASS
- US-6: 4 tests. PASS
- US-7: 3 tests. PASS (Issue 3 observability fix needed)

All 7 stories: covered. PASS.

**2. No BLOCKED items in adversarial review**

The adversarial review Issue 4 is labeled "BLOCKED scope gap" but its verdict section explicitly states it is "not a bug in the implementation." It is a requirements-vs-task-doc inconsistency resolved in favor of `requirements.md`. The implementation plan's Deferred section explicitly lists the session list archive UI as not implemented.

**Verdict:** No true blocking defects. Issue 1 (data race) is the highest-priority fix but is "low severity." PASS with required fixes (Issues 1, 3, 5 before merge).

**3. Deferred items explicitly documented**

- `plan.md` Deferred section lists: session list archive toggle, bulk archive, TTL/hard-delete, notifications, `runWorkflow` navigation gap.
- This validation.md Deferred section confirms the same items with follow-up ticket guidance.
- Adversarial Issue 4 resolved: `requirements.md` governs, task doc was in error.

PASS.

**4. Non-functional requirements addressed**

- Latency (< 200ms Archive RPCs): benchmark test specified. PASS.
- Goroutine safety (auto-archive non-blocking): verified by code inspection + race test. CONCERNS (Issue 1 race must be fixed).
- Caller impact (`ListSessions` default filter): internal callers audited and safe; documentation fix recommended. PASS with comment fix.
- Zombie tmux prevention: fallback-to-detach path tested + manual verification. PASS.

---

## Readiness Gate Verdict: **CONCERNS — fix Issues 1, 3, 5 before merge**

The feature is functionally sound. Run history, auto-archiving, pause memory optimization, and resume UUID correction all work on the happy path. No data loss or crash paths exist.

Three targeted fixes are required:

1. **Issue 1** — Add mutex around `maybeAutoArchive` nil-check + write. Without it `go test -race` will flag a data race.
2. **Issue 3** — Add `log.Warn` for non-TmuxBackend in Resume reinit. One-line observability fix.
3. **Issue 5** — Wire `router.push(/?session=<id>)` in `handleRun`. Completes the "jump to it immediately" AC for US-1.

After these three fixes, the gate is **PASS**.
