# Core Domain Decomposition — Implementation Plan

## Context

Corrected scope (see `requirements.md` background and `research/architecture.md`): `session_service.go` is not an undecomposed monolith — 26 focused service files already exist and most RPC methods already delegate to them. Real remaining scope: extract 4 new services for the clusters with no existing sibling to delegate to (checkpoints, autonomous-driver orchestration, terminal I/O, feature flags), delegate 3 more methods to their already-existing sibling services (`ListBranches`→`workspaceSvc`, `ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions`→`workflowSvc`) — the latter found, along with the two new-service clusters, by an independent adversarial review (see Epic 1 Stories 1.4-1.6) — normalize tmux output at `Preview()`'s single exit point covering both of its branches (corrected from an earlier fallback-only design — see Epic 2), audit ~30 wiring setters, and close the governance gap with ADRs (done — see `decisions/`).

**Post-first-review correction**: an independent adversarial review (`implementation/adversarial-review.md`) found Epic 2's original design normalized only `Preview()`'s `CapturePaneContent()` fallback branch, while `approval_handler.go`'s documented motivating use case (autonomous-mode approval) actually exercises the *other* branch (the in-memory PTY buffer), which was wrongly assumed to already be normalized. Epic 2 below reflects the corrected design: normalize once, downstream of both branches. The review also found 6 additional undecomposed clusters in `session_service.go` beyond the original two; Epic 1 below adds Stories 1.4-1.6 to cover them.

Sequencing: Epic 1 (`session_service.go` decomposition) and Epic 2 (tmux normalization) have **no dependency on `instance-actor-concurrency`** and can start immediately, in parallel with that project — except Story 1.2 (`AutonomousOrchestrationService`), which touches `onAutonomousDriverComplete`/`buildTurnCallback`, the same methods `instance-actor-concurrency`'s Epic 5 targets; Story 1.2 carries its own phase-check coordination gate (mirroring Story 3.1's) for that reason. Epic 3 (`Instance` field grouping) explicitly waits for that project's Epic 1 (additive `InstanceSnapshot`) to land first.

---

## Epic 1: Extract Remaining Undecomposed Clusters from `session_service.go`

Covers `CheckpointService`/`AutonomousOrchestrationService` (Stories 1.1-1.2, the two clusters identified before the adversarial review), the `Set*`/`Get*` wiring audit (Story 1.3), and the four additional clusters the adversarial review found (Stories 1.4-1.6: `ListBranches`/`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` delegated to existing services, plus new `TerminalService`/`FeatureFlagService`).

### Story 1.1: `CheckpointService`

**As a** maintainer, **I want** checkpoint RPC logic in its own service, **so that** it follows this repo's established delegate pattern (ADR-001) instead of living inline in `session_service.go`.

#### Task 1.1a: Create `server/services/checkpoint_service.go`
- `NewCheckpointService(storage *session.Storage) *CheckpointService`, matching `project_service.go`'s exact shape.
- Move `CreateCheckpoint`, `ListCheckpoints`, `ClearConversationState` logic in verbatim (no behavior change) — confirm during the move whether these call `session.Instance`'s own `CreateCheckpoint()`/checkpoint methods (session package, not server/services) and preserve that call signature exactly, since `instance-actor-concurrency`'s Epic 4 will convert those methods to actor-routed commands independently.
- `SessionService` gains a `checkpointSvc *CheckpointService` field, wired in the constructor next to `projectSvc`; the 3 RPC methods become delegates.

#### Task 1.1b: Move/create `checkpoint_service_test.go`
- Relocate existing checkpoint tests from wherever they currently live (verify actual location — do not assume `session_service_test.go` is a single file; check for a `*_checkpoint_test.go` naming pattern first, matching this repo's apparent per-concern test file convention).

#### Task 1.1c: Verify no RPC contract change
- `go build ./... && make test` passes; run any e2e test exercising checkpoint creation/listing unmodified.

### Story 1.2: `AutonomousOrchestrationService`

**As a** maintainer, **I want** autonomous-driver lifecycle management in its own service, **so that** the largest remaining undelegated cluster (~250 lines) in `session_service.go` follows the established pattern.

**Coordination gate (mirrors Task 3.1a/3.1b)**: `instance-actor-concurrency/implementation/plan.md`'s Epic 5 (`server/services/session_service.go` + background-goroutine writers, an atomic unit) explicitly names `onAutonomousDriverComplete` and `buildTurnCallback` as its own conversion targets — cited by section, not a frozen line range, since cross-project line citations have already gone stale once (an earlier draft of this gate cited `:1837-1864`, which as of this correction actually pointed at unrelated Epic 4 content): see that plan's Epic 5 → **Story 5.3 ("PR-status + review-queue writer cluster")**, whose Task 5.3b is titled "Convert `buildTurnCallback`/`onAutonomousDriverComplete` in `session_service.go` to commands" (as of this correction, Epic 5's header is at ~line 2005 and Story 5.3's method-naming text/task is at ~lines 2080-2117 — re-verify by section name, not these numbers, since that file shifts frequently and both plans already acknowledge this) — the identical methods Task 1.2a/1.2b move here. **Before starting Task 1.2a, confirm `instance-actor-concurrency`'s current phase.** If that project has started Epic 5 (or is about to), coordinate landing order with it first (either this project's Story 1.2 lands and Epic 5 rebases onto the new file location, or Epic 5 lands first and Story 1.2 moves the already-actor-routed version) — do not let both land independently in the same window, since both rewrite the same method bodies and a naive merge will silently drop one side's change. As of this plan's writing, `instance-actor-concurrency` has not yet landed its own Epic 1 (`InstanceSnapshot` doesn't exist — verified, see `session/instance.go`), so it is nowhere near Epic 5 and there is no active conflict today; this gate exists so a future implementer picking up Story 1.2 in isolation doesn't miss the constraint ADR-002 already documents in prose.

#### Task 1.2a: Create `server/services/autonomous_orchestration_service.go`
- Shape per `research/architecture.md` §3: `NewAutonomousOrchestrationService(pool *headless.Pool, bus *events.EventBus) *AutonomousOrchestrationService`, a `drivers map[string]*session.AutonomousDriver` field with its own narrow mutex (registry membership only — not `Instance` state).
- Move `registerDriver`, `stopAndDeregisterDriver`, `StopDriverForSession`, `buildTurnCallback`, `StartAutonomousDriverForInstance`, `StartAutonomousDriverWithTimeout` verbatim.
- Consolidate the 5 `wireXCallback` methods (`wireAutoArchiveCallback`, `wireSessionExitedPublisher`, `wireStatusChangeCallback`, `wireRateLimitCallbacks`, `wireClaudeSessionIDCallback`) into one `WireCallbacks(inst *session.Instance)` method, or keep them as 5 separate methods if implementation finds they're called independently at different points (verify actual call sites before consolidating — don't force a single entry point if the current 5 call sites are genuinely independent).

#### Task 1.2b: Move `onAutonomousDriverComplete`
- **Do not fix** the unguarded `Instance` field writes inside it (`AutonomousMode`/`AutonomousTurn`/`AutonomousMaxTurns`/`AutonomousOutcome`) — that's `instance-actor-concurrency`'s Epic 5 scope (ADR-002). Move the method as-is, including its current (unguarded) writes, into the new service. Add a code comment at the call site noting the pending fix in the other project, so a future reader doesn't mistake this move for a concurrency fix.
- Same coordination gate as Task 1.2a applies here — this is the exact method `instance-actor-concurrency`'s Epic 5 also rewrites; re-confirm that project's phase immediately before this task if any time has elapsed since Task 1.2a's check.

#### Task 1.2c: `SessionService` wiring + delegate reduction
- `SessionService` gains `autonomousSvc *AutonomousOrchestrationService`; `StartAutonomousDriverForInstance`/`StartAutonomousDriverWithTimeout`/`StopDriverForSession` become delegates.

#### Task 1.2d: Verify no RPC contract change
- `go build ./... && make test`; manually exercise starting/stopping an autonomous-mode session through the UI.

#### Task 1.2e: Move/create `autonomous_orchestration_service_test.go`
- Satisfies R1.7 (colocated test file per extracted service), mirroring Task 1.1b's checkpoint test-file task. An existing `server/services/autonomous_integration_test.go` covers the current inline behavior — relocate/rename it to `autonomous_orchestration_service_test.go` if its tests exercise the moved methods directly, or extend it with tests targeting the new `AutonomousOrchestrationService` type if it doesn't. Verify actual coverage during the move; don't assume the existing file already satisfies R1.7 just because it exists.

### Story 1.3: Audit `Set*`/`Get*` wiring methods

#### Task 1.3a: Catalog all ~30 setters/getters with their actual callers
- For each, determine: is the dependency available at `SessionService` construction time via `server/dependencies.go`'s staged builders? If yes, move to constructor injection and delete the setter. If the dependency is genuinely only available after some other initialization step completes (verify, don't assume), document why and leave as a setter.
- Getters that only expose an internal dependency to another part of the codebase: check whether the caller could instead receive that dependency directly via its own constructor injection, removing the need to reach through `SessionService`.

#### Task 1.3b: Apply the conversions found safe in 1.3a
- Incremental — convert one setter/getter at a time, `go build` after each, since these often have multiple call sites across `main.go`/`server/dependencies.go`.

### Story 1.4: Delegate `ListBranches`/`ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` to Existing Services

**As a** maintainer, **I want** the remaining undelegated VCS and workflow methods routed through their already-established sibling services, **so that** `ListBranches` stops being the odd one out next to `GetVCSStatus`/`GetWorkspaceInfo`/`ListWorkspaceTargets`/`SwitchWorkspace` (all four already delegate to `workspaceSvc`), and `ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` stop being the odd ones out next to `CreateWorkflow`/`UpdateWorkflow`/`DeleteWorkflow`/`ListWorkflows`/`RunWorkflow` (all five already delegate to `workflowSvc`). Found by the first adversarial review (§1) as inventory the original pass missed; per ADR-001's own rule these already exceed the 30-line business-logic threshold (`ListBranches` 99 lines, `ArchiveWorkflowSessions` 55 lines, `DeleteWorkflowFailedSessions` 46 lines).

#### Task 1.4a: Move `ListBranches` (`session_service.go:2985-3081`) into `workspace_service.go`
- Move the `$HOME` path-validation guard, the in-process TTL branch cache (`s.branchCache`), and the `safeexec`-wrapped `git for-each-ref` subprocess call (2s timeout) verbatim into a new `WorkspaceService.ListBranches` method. Verify whether the branch cache should live on `WorkspaceService` itself (most likely, since it's the new owner) or needs to stay reachable from `SessionService` for another reason — check for other internal callers of `s.branchCache` before moving it.
- `SessionService.ListBranches` becomes a delegate to `s.workspaceSvc.ListBranches(...)`.

#### Task 1.4b: Move `ArchiveWorkflowSessions`/`DeleteWorkflowFailedSessions` (`session_service.go:4275-4369`) into `workflow_service.go`
- Move the direct `ent` bulk-update queries (`entClient.Session.Update().Where(...)`) and in-memory poller-instance mutation verbatim into `WorkflowService`. These are workflow-scoped (both take `WorkflowId`) and sit a few hundred lines away from the already-delegated `CreateWorkflow`/`UpdateWorkflow`/`DeleteWorkflow`/`ListWorkflows`/`RunWorkflow` — no reason for these two to be the exception.
- `SessionService`'s two RPC methods become delegates to `s.workflowSvc`.

#### Task 1.4c: Verify no RPC contract change
- `go build ./... && make test`; exercise `ListBranches`, `ArchiveWorkflowSessions`, and `DeleteWorkflowFailedSessions` through the UI (branch dropdown, workflow session archival/cleanup).

### Story 1.5: Extract `TerminalService` for `GetTerminalSnapshot`/`WriteToSession`

**As a** maintainer, **I want** the terminal read/write RPC logic in its own service, **so that** the shared instance-lookup-fallback-chain logic (poller → external discovery) these two methods duplicate has one home instead of being reimplemented inline twice.

Judgment call (per this review's finding): unlike `ListBranches`/`ArchiveWorkflowSessions`, these two methods (`GetTerminalSnapshot` 47 lines at `:3575-3617`, `WriteToSession` 48 lines at `:3622-3665`) have no existing sibling service to fold into — none of the 24 already-extracted services own live-instance terminal I/O. They are, however, clearly cohesive with each other: both resolve the target instance via the identical poller-then-external-discovery fallback chain before doing their actual work (`GetTerminalSnapshot` calls `Preview()` with graceful degradation and line-trimming; `WriteToSession` runs a goroutine + `select`/timeout around `SendKeys`). Per ADR-001's rule ("any single operation exceeding ~30 lines of business logic, or a cohesive cluster of 3+ related operations, gets its own file"), both individually exceed the line threshold and are cohesive as a pair — this warrants a new `server/services/terminal_service.go`, not folding into an unrelated existing file.

#### Task 1.5a: Create `server/services/terminal_service.go`
- `NewTerminalService(reviewQueuePoller *ReviewQueuePoller, externalDiscovery ExternalDiscovery) *TerminalService` (or the actual dependency types `GetTerminalSnapshot`/`WriteToSession` currently close over — verify exact types during implementation).
- Factor the shared instance-lookup fallback chain (poller → external discovery → not-found error) into one private helper method both `GetTerminalSnapshot`/`WriteToSession` call, rather than duplicating it.
- Move `GetTerminalSnapshot`/`WriteToSession` logic verbatim otherwise (no behavior change).
- Verify whether the already-inline `StreamTerminal` RPC (`session_service.go:1942`) shares the same lookup chain and would belong in this service too — if it does, note it as a candidate for a follow-up task rather than silently expanding this story's scope; if its streaming setup is materially different, leave it inline as originally scoped.

#### Task 1.5b: Move/create `terminal_service_test.go`
- Colocated test file per R1.7/ADR-001, covering both methods and the shared lookup-fallback helper (including its not-found and controller-vs-fallback branches).

#### Task 1.5c: Verify no RPC contract change
- `go build ./... && make test`; exercise a terminal snapshot fetch and a `WriteToSession` keystroke through the UI.

### Story 1.6: Extract `FeatureFlagService`

**As a** maintainer, **I want** feature-flag read/write RPC logic in its own service, **so that** `UpdateFeatureFlag` (62 lines) stops exceeding ADR-001's own 30-line threshold while living inline.

`GetFeatureFlags`/`UpdateFeatureFlag` (`session_service.go:4007-4093`, 28/62 lines) form a full cluster: known-flag validation, `config.LoadConfig()`/`cfg.SetFeatureFlag()` persistence, and in-process `FeatureController` toggling. This is a self-contained concern with no natural fit in any of the 24 existing services (it isn't workspace/workflow/project/github-scoped) — per ADR-001's rule, a single operation past the 30-line threshold with no existing home gets its own file. This is a new service, not a fold-in.

#### Task 1.6a: Create `server/services/feature_flag_service.go`
- `NewFeatureFlagService(cfg *config.Config, controller *features.FeatureController) *FeatureFlagService` (verify exact constructor dependencies from the current inline implementation).
- Move `GetFeatureFlags`/`UpdateFeatureFlag` logic verbatim: known-flag validation, `config.LoadConfig()`/`SetFeatureFlag()` persistence, `FeatureController` toggling.
- `SessionService` gains a `featureFlagSvc *FeatureFlagService` field; both RPC methods become delegates.

#### Task 1.6b: Move/create `feature_flag_service_test.go`
- Colocated test file per R1.7/ADR-001.

#### Task 1.6c: Verify no RPC contract change
- `go build ./... && make test`; exercise toggling a feature flag through the UI.

---

## Epic 2: Tmux Content Normalization (ADR-003)

### Story 2.1: Route Both of `Instance.Preview()`'s Branches Through `PTYNormalizer`

**Corrected design (post-adversarial-review)**: the original Task 2.1b normalized only the `CapturePaneContent()` fallback branch, on the premise that the primary branch (`ctrl.GetRecentOutput()`, the in-memory PTY buffer) was "already normalized via `ClaudeController`'s own processing." Independent verification found that premise **false**: the circular buffer `GetRecentOutput()` reads from is filled by `session/response_stream.go`'s `streamLoop` with a raw, untransformed copy of `readBuf[:n]` (`response_stream.go:259-284`; the escape-code-parser call there is explicitly commented `"passthrough - doesn't modify data"`, `response_stream.go:277`). Neither branch normalizes today. Worse, `approval_handler.go`'s autonomous-mode approval path (`approval_handler.go:318-322`, gated on `h.autonomousChecker(sessionID)`) only runs for a live, controller-managed session — i.e. it always takes the *primary* branch, never the fallback. A fix scoped to the fallback branch alone would not touch the content this call site actually receives. See ADR-003's Amendment for the full trace.

The corrected fix: restructure `Preview()` itself so both branches feed a single `content string` local, then normalize once at the function's single return point — downstream of the branch selection, so it covers whichever branch was taken. This is still "normalize at the existing `Preview()` boundary, no new interface" (R2.1 unchanged) — it just closes the gap in both branches instead of one.

#### Task 2.1a: Verify all current callers of `Preview()`
- `grep -rn "\.Preview()" --include=*.go` across the repo; confirm none rely on raw (non-normalized) tmux/PTY bytes in a way this change would break (per ADR-003's flagged risk — now a wider blast radius since both branches are affected, not just the fallback). Confirmed during planning: callers are `server/services/approval_handler.go`, `session/autonomous_driver.go`, `session/session_driver.go`, `session/review_queue_poller.go`, `server/services/session_service.go`'s `GetTerminalSnapshot`, and `testutil/session.go` — all do line-oriented text classification or line-trimming on the result, none rely on raw ANSI. Re-verify this list at implementation time in case new callers were added since planning.
- Explicitly confirm this change does **not** touch the separate raw-byte terminal-streaming/scrollback path (`server/services/connectrpc_websocket.go`'s `CapturePaneContentRaw()`/`scrollbackManager`, `response_stream.go`'s `broadcast()`), which needs unmodified ANSI bytes for live terminal rendering and must keep working exactly as today — that path does not call `Preview()` at all, so it is unaffected by construction, but note this explicitly as an acceptance check since it's the reason normalization happens inside `Preview()` rather than upstream at `response_stream.go`'s buffer-write point.

#### Task 2.1b: Apply normalization to both branches at `Preview()`'s single return point
- In `session/instance_terminal.go`'s `Preview()` (currently lines 103-125): restructure so the `ctrl.GetRecentOutput()` branch and the `i.pm().CapturePaneContent()` branch both assign into one `content string` local instead of each doing an early `return`, then pass that single value through `session/detection`'s `PTYNormalizer{}.Normalize(...)` (already an exported value-type method — no wrapper needed, confirmed by reading `session/detection/normalizer.go`) before the function's one final return. Concretely:
  ```go
  func (i *Instance) Preview() (string, error) {
      if !i.started || i.Status == Paused || i.Status == Stopped {
          return "", nil
      }

      var content string
      if ctrl := i.GetController(); ctrl != nil {
          content = string(ctrl.GetRecentOutput(0))
      } else {
          c, err := i.pm().CapturePaneContent()
          if err != nil {
              return "", nil
          }
          content = c
      }

      return detection.PTYNormalizer{}.Normalize(content), nil
  }
  ```
- `session` package already imports `session/detection` elsewhere (confirmed: `session/claude_controller.go`, `session/autonomous_driver.go`, `session/instance.go`, and ~20 other files already do this) and `session/detection` does not import `session` — no import cycle.

#### Task 2.1c: Test
- Existing `normalizer_test.go` coverage should already validate the normalization logic itself; add tests on **both** of `Preview()`'s branches confirming normalized (not raw) output — one exercising the controller-attached path (mock/stub `ClaudeController` returning ANSI-laden bytes via `GetRecentOutput`) and one exercising the `CapturePaneContent()` fallback path, using this repo's existing mock-tmux test infrastructure (`buildWithMockTmux`, per patterns already used in `session/comprehensive_session_creation_test.go`). The controller-branch test is the one that closes the actual gap this review found — do not skip it in favor of only re-testing the fallback branch that was already covered by the original (incomplete) design.

### Story 2.2: `classifier.go` — no action pending further evidence

#### Task 2.2a: One-time re-verification during implementation
- Before closing this project, re-run a targeted `git log` co-change check scoped to `pkg/classifier/classifier.go`/`session/tmux/tmux.go` alone (not the full 1000-commit window) to see the actual commits — if they reveal a real dependency this research missed, open a follow-up decision; if not (likely, per `research/architecture.md` §5's finding of zero direct references), document the non-finding in this story and close it.

---

## Epic 3: `Instance` Field Grouping (BLOCKED on `instance-actor-concurrency` Epic 1)

**Do not start until `instance-actor-concurrency`'s `InstanceSnapshot`/`buildSnapshot()` (that project's Epic 1) has landed.**

### Story 3.1: Group `InstanceSnapshot`'s fields into named sub-structs

#### Task 3.1a: `GitHubIntegration` sub-struct
- Per `instance-actor-concurrency/research/architecture.md` §1.2's already-grouped comment block (GitHub PR/URL integration fields), extract `GitHubPRNumber`, `GitHubPRURL`, `GitHubOwner`, `GitHubRepo`, `GitHubSourceRef`, `ClonedRepoPath`, `MainRepoPath`, `IsWorktree`, `GitHubIsFork`, `GitHubPRIsDraft`, `GitHubPRState`, `GitHubPRPriority`, `GitHubApprovedCount`, `GitHubChangesReqCount`, `GitHubCheckConclusion`, `GitHubPRStatusTerminal`, `LastPRStatusCheck` into a named, embedded `GitHubIntegration` struct within `InstanceSnapshot`.
- Coordinate with `instance-actor-concurrency`: this touches the same struct their Epic 4/5 (state-machine core, `UpdatePRStatus`) actively works on — confirm that project's current phase before starting, to avoid a straight merge conflict on the same lines.

#### Task 3.1b: `AutonomousModeState` sub-struct
- `AutonomousMode`, `AutonomousTurn`, `AutonomousMaxTurns`, `AutonomousOutcome` into a named sub-struct. Same coordination caveat as 3.1a — this is exactly the field set `AutonomousOrchestrationService` (Epic 1 of this project) and `instance-actor-concurrency`'s Epic 5 both also touch; land this only after both of those have stabilized.

---

## Task Summary

| Epic | Stories | Tasks | Primary files |
|---|---|---|---|
| 1 — Checkpoint/Autonomous extraction + expanded cluster inventory | 6 | 19 | `server/services/checkpoint_service.go` (new), `server/services/autonomous_orchestration_service.go` (new), `server/services/terminal_service.go` (new), `server/services/feature_flag_service.go` (new), `server/services/workspace_service.go`, `server/services/workflow_service.go`, `server/services/session_service.go` |
| 2 — Tmux normalization | 2 | 4 | `session/instance_terminal.go`, `session/detection/normalizer.go`, `session/response_stream.go` (verify only, no change expected), `server/services/approval_handler.go` (verify only, no change expected) |
| 3 — Instance field grouping (blocked) | 1 | 2 | `session/instance.go` / `instance-actor-concurrency`'s `InstanceSnapshot` |

**Total**: 3 Epics, 9 Stories, 25 Tasks.

(Recount rationale: Epic 1 = Story 1.1 (3: 1.1a-c) + Story 1.2 (5: 1.2a-e, adding 1.2e for R1.7) + Story 1.3 (2: 1.3a-b) + Story 1.4 (3: 1.4a-c, new) + Story 1.5 (3: 1.5a-c, new) + Story 1.6 (3: 1.6a-c, new) = 19. Epic 2 = 2.1a-c + 2.2a = 4 (unchanged). Epic 3 = 3.1a-b = 2 (unchanged). Total tasks = 19+4+2 = 25; total stories = 6+2+1 = 9. This also corrects the first adversarial review's finding that the prior table's Epic 1 count of 10/total of 16 didn't match its own enumerated tasks — 9/15 was the corrected count before Stories 1.4-1.6 and Task 1.2e were added.)

## Open Decisions

| # | Decision | Status |
|---|---|---|
| 1 | Whether `wireXCallback`'s 5 methods consolidate into one `WireCallbacks` or stay separate | Deferred to Task 1.2a implementation-time verification of actual call sites |
| 2 | Whether `GetProviderLimits`/`GetDetectionEvents` warrant extraction | Deferred per ADR-002 — not in this project's committed scope |
| 3 | Exact landing order relative to `instance-actor-concurrency`'s epics for Epic 1/3 of this project | Epic 1 Story 1.2 (`AutonomousOrchestrationService`) now carries its own explicit phase-check coordination gate (mirroring Story 3.1's), since `instance-actor-concurrency`'s Epic 5 targets the identical `onAutonomousDriverComplete`/`buildTurnCallback` methods — confirm that project's current phase immediately before starting Task 1.2a/1.2b. Epic 1's other stories (1.1, 1.3-1.6) and Epic 2 remain fully unblocked — no dependency, land whenever ready. Epic 3 here: hard-blocked on that project's Epic 1, and Task 3.1a/3.1b specifically should land after that project's Epic 4/5 stabilize to avoid repeated merge conflicts on the same struct |
| 4 | Whether `GetTerminalSnapshot`/`WriteToSession`'s new `TerminalService` should also absorb the already-inline `StreamTerminal` RPC | Deferred to Task 1.5a implementation-time verification of whether `StreamTerminal` shares the same instance-lookup fallback chain — not committed scope for this project unless that verification finds a clean fit |
