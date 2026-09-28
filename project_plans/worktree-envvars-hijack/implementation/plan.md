# Implementation Plan: worktree-envvars-hijack

**Feature**: Close the `SESSION_TYPE_NEW_WORKTREE` silent-fallback / cross-session
conversation-hijack bug: confirm the unverified root-cause hypotheses, fix the
one already-verified defect (stale `instanceRootDir`), and make every
"session silently lands in the wrong, already-occupied directory" failure
mode loud instead of silent.
**Date**: 2026-09-24
**Status**: Ready for implementation
**ADRs**: None — every design choice below reuses an existing, already-tested
pattern in this codebase (see Pattern Decisions); none of it is a novel
architectural commitment that would need a durable decision record.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `Instance` | The live, in-memory representation of one agent session (`session/instance.go`). | Existing type. |
| `SessionType` | `config.SessionType` (a `string`-backed enum): `SessionTypeDirectory`, `SessionTypeNewWorktree`, `SessionTypeExistingWorktree`, `SessionTypeNewProject`, `SessionTypeOneOff`. | Existing type; this plan adds no new session type. |
| `resolveSessionType` | `server/services/session_service.go`'s function mapping a `CreateSessionRequest` + resolved branch to a `session.SessionType`. | Existing; Story 3.1 changes its signature to return an error. |
| `setupFirstTimeWorktree` | `Instance` method (`session/instance_worktree.go`) that switches on `i.SessionType` and creates (or skips) a git worktree during first-time session creation. | Existing; Story 3.2 hardens its `default:` arm. |
| `Workspace` | Value struct (`session/types.go`) naming the four path concepts a session can have: `RepoRoot`, `WorktreeDir`, `ActiveDir`, `ExistingDir`. | Existing; this plan's guards read `Workspace().ActiveDir`/`RepoRoot`, never a raw field, per `.claude/rules/instance-lock-free-reads.md`. |
| `ActiveDir` | `Workspace().ActiveDir` (or the `Instance.ActiveDir()` accessor): the worktree directory if the session has one, else the repo root. | The correct field for "does this session's working directory collide with another session's" — see `instance-lock-free-reads.md`'s `#801` precedent for why not `ExistingDir`. |
| `RepoRoot` | `Workspace().RepoRoot`: the bare repository path the session was created against. | Used by Story 3.3's structural guard as the "did a real worktree happen" baseline. |
| `runBackgroundResolutionPipeline` | `SessionService` method (`server/services/session_creation_pipeline.go`) — the one goroutine that runs GitHub-URL resolution → defaults → worktree setup → tmux startup for every `CreateSession` call, and makes the one terminal status write. | Existing; Story 2.1 edits this function directly. Epic 3.3's guards **no longer** live here (relocated into `startLocked` — see Epic 3.3's relocation note, the Blocker-1 remediation) — the pipeline's only Epic-3.3-related change is classifying `p.instance.Start(true)`'s returned error into a specific `metricsOutcome`/`failureReason` at its existing failure branch (`:258-263`), not a new guard/terminal-write call site. |
| `creationPipelineParams` | Struct bundling everything `runBackgroundResolutionPipeline` needs, captured in `CreateSession`'s synchronous prefix. | Existing; `instanceRootDir` is one of its fields. |
| `instanceRootDir` | The pipeline's local variable holding the root directory `InjectHookConfig`/`StartSessionDriver` operate against. | Existing, but currently stale for a non-GitHub-URL session (Story 2.1's fix target). Unrelated to Epic 3.3 post-relocation — the structural/collision guards read `Workspace()`/`basePath` inside `startLocked` directly, not this pipeline-local variable. |
| `pipelineOutcome` | Struct `{status, failureReason, metricsOutcome}` the pipeline's `terminal()` closure takes. | Existing; Epic 3.3's guard failures now surface through the pipeline's *existing* `startErr != nil` branch (`session_creation_pipeline.go:258-263`), which already builds one of these — no new call site. |
| `OtherLiveSessionInsideWorktree` | `SessionService` method (`server/services/session_service.go:1236`) reporting whether some other live, backend-process-alive session's real cwd is inside a given worktree path. | Existing, already tested (pause/stop/delete guards); Story 3.3.2 calls it from inside the new `Instance.SetPreSpawnCollisionGuard` callback closure (still defined in `server/services`, just invoked earlier via the callback) — no change to its own contract. Its silent `("", false)` return when `s.reviewQueuePoller == nil` is unchanged; the callback closure now logs a warning in that case (see Tech Debt Disposition). |
| `RefuseIfWorktreeSharedWithOtherLiveSession` | `SessionService` method wrapping `OtherLiveSessionInsideWorktree` into a ready-to-return RPC error. | Existing; not reused directly (it returns a `connect.Error`, and the callback needs a plain `error`) — Story 3.3.2 calls the lower-level `OtherLiveSessionInsideWorktree` instead and builds its own `error`. |
| `Instance.SetPreSpawnCollisionGuard` | **New** injected callback on `Instance` (`session/instance.go`): domain-defined port `func(worktreePath string) error`, wired by `SessionService.wireCallbacks` before `Start()` is called, invoked by `startLocked`'s `firstTimeSetup` branch immediately after the worktree path resolves and before `i.initTmuxSession()`/`i.pm().Start()` spawn anything. | New — Epic 3.3's Blocker-1 remediation. Mirrors the existing callback-injection shape `SessionService.wireCallbacks` already uses for its other pre-`Start()` injections (`SetTaggingEngine`, `SetMCPServerURLProvider`, etc. — `server/services/session_service.go:1831`) and `ProcessManager.SetOnExitCallback` (wired inside `startLocked` itself at `session/instance.go:1496`/`:1753`) — not a novel pattern, the same shape applied to a new port. (Not `Instance.SetStatusManager`: despite a superficially similar callback-injection shape, every real call site wires it *after* `Start()`, deliberately, to avoid a documented PTY EIO bug — see the Pattern Decisions table's Cross-session guard row for the correction.) |
| `GitWorktreeManager` | Existing **concrete struct** type (`session/git_worktree_manager.go:28`) — `Instance.gitManager`'s actual field type. A same-named `GitManager` *interface* also exists (`session/git_worktree_manager.go:511`) but has zero other usages anywhere in the codebase and cannot be assigned to the concrete-typed field, so it is not a usable test seam despite the name. | Existing; Epic 4's new fault-injection tests (4.4/4.5) instead force a chosen `HasWorktree()`/`GetWorktreePath()` result via the already-exported `Instance.SetGitWorktree(*git.GitWorktree)` + `git.NewGitWorktreeFromStorage(...)` — the same seam `server/services/session_service_worktree_guard_test.go`'s `newWorktreeGuardTarget` already uses for an equivalent worktree-collision test, not an interface-based double. |
| `DetectByPath` / `tryExtractConversationUUID` | `session/history_detector.go:154` / `session/instance_claude.go:380` — path-keyed, ownership-blind "most recently modified JSONL" conversation-UUID recovery, with no per-session ownership check. | Existing; new Epic 1.4 audits and closes its cross-session ownership blindness — `research/pitfalls.md`'s Designed-against checklist item 5 names this as the concrete mechanism that could explain the reporter's literal "saw another session's conversation" symptom, and no story in the plan touched it before this repair pass. |
| `InjectHookConfig` | `server/services/approval_handler.go:988` — writes the Claude Code HTTP-hook `settings.local.json` into a session's root directory. | Existing; the observable side effect Story 4.2's regression test asserts on. |
| `StartSessionDriver` | `session/session_driver.go:154` — starts the goroutine that types a session's initial prompt into its terminal. | Existing; also consumes the stale `instanceRootDir` today. |
| `buildSnapshot` / `i.snapshot.Store` | The republish step that makes a mutated `Instance` field visible to lock-free readers (`Snapshot()`). | Existing pattern (`instance-lock-free-reads.md`); Story 3.2 extends it to `setupFirstTimeWorktree` cases that currently skip it. |
| `ModeIsNewWorktree` | The new subtest name this plan adds to `TestCreateSession_should_ReachActiveViaPipeline` (Story 4.1), mirroring the existing `ModeIsDirectory`/`ModeIsOneOff`/`ModeIsRestart` subtests. | New identifier — use exactly this name in code. |
| `assertReachesActiveViaPipeline` | Existing shared test helper (`session_service_test.go`) asserting a `CreateSession` response reaches `Active` via `svc.awaitCreationTerminal`. | Reused as-is by Story 4.1. |
| Hypothesis #1 / #2 / #3 | The three ranked root-cause hypotheses from `research/architecture.md`'s "Top 3 root-cause hypotheses" section: #1 client-side request divergence (speculative), #2 branch-collision-with-main-checkout self-heal gap (plausible, incomplete), #3 stale `instanceRootDir` (verified, non-differentiating). | Referenced by number throughout Epic 1; #3 is what Epic 2 fixes. |

---

## Pattern Decisions

**Overall fix strategy** (Step 0.5 creative pass):

| Approach | Strength | Weakness |
|---|---|---|
| A. Symptom patch only — ship Epic 2's verified stale-`instanceRootDir` fix and stop | Minimal, low-risk, ships in one PR | `research/architecture.md` Q4 already proves `GetSession` never reads `instanceRootDir` — this fix cannot explain the reported `ActiveDir`-collapses-to-bare-repo symptom, so the reporter's actual bug would likely still reproduce. Explicitly the pattern requirements.md's own "Alternatives Considered" already rejects ("defensive patch with no confirmed root cause"). |
| B. Defense-in-depth across every failure layer (chosen) — fix the verified bug, close both silent-fallback-to-Directory layers, add a structural post-worktree assertion + a reused collision guard, and pursue root-cause confirmation in parallel | Closes the isolation vector regardless of which hypothesis (#1/#2/#3) turns out to be the true differentiator — satisfies requirements.md's Success Metrics even if Epic 1 never fully nails the exact trigger | Touches more call sites than a single targeted fix (all of `setupFirstTimeWorktree`'s cases), so review surface and sequencing risk are larger |
| C. Block all fixes on root-cause confirmation first | Guarantees a precisely targeted eventual fix, no "wasted" defensive code | `research/architecture.md` already exhausted the backend static-analysis angle without finding the differentiator (Q1/Q2/Q3/Q4 all closed clean); the leading hypothesis is client-side and this plan has no access to the reporter's live environment to confirm it — blocking indefinitely on an unconfirmable hypothesis is unacceptable for a severe, already-reported isolation bug |

Chosen: **B**. Rejected A and C are recorded above rather than in the table below (that table is for component-level patterns).

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `resolveSessionType`'s unrecognized-enum-value handling | Fail-fast at the boundary (Parse-Don't-Validate) — return `(SessionType, error)` instead of a default value | Type-driven design | Keep silently defaulting to `SessionTypeDirectory` (status quo) | This exact silent-substitution shape has already caused three prior incidents in this repo (`9dbda9d2e`, `9037ed5e0`, `43be58416` — `research/pitfalls.md` §1); a fourth instance is the thing to actively avoid, not tolerate. |
| `setupFirstTimeWorktree`'s unrecognized-`SessionType`-value handling | Split `SessionTypeDirectory` into its own case; make the true `default:` return an error | Same as above | Leave `SessionTypeDirectory` and "any unrecognized type" sharing one silent no-op arm (status quo) | `SessionTypeDirectory` is a legitimate, common, correct outcome and must not become an error; only a genuinely off-nominal value (which today's code cannot distinguish from Directory) should. |
| Stale `instanceRootDir` (Epic 2) | Recompute the derived value at the point of use, immediately after the mutation (`Start()`) that invalidates the earlier capture | General "don't cache a derived value across a mutation boundary" discipline (PoEAA's caution on stale derived data) | Thread a lazy accessor/closure instead of a plain string | The pipeline already refreshes this exact variable this exact way for the `deferredGitHubURL` branch (`session_creation_pipeline.go:198`) — matching that existing idiom is a one-line diff; a lazy-accessor refactor would touch every consumer of `creationPipelineParams.instanceRootDir` for a one-line staleness bug. |
| Cross-session directory-collision guard + structural no-worktree guard (Epic 3.3) — **relocated during plan repair, see below** | Move both checks into `startLocked` (`session/instance.go`), BEFORE `i.initTmuxSession()`/`i.pm().Start()` spawn any process: the structural check (`ActiveDir == RepoRoot`) is an unconditional in-domain check needing no service-layer data; the cross-session collision check runs inside a new injected callback, `Instance.SetPreSpawnCollisionGuard(func(worktreePath string) error)`, wired by `SessionService.wireCallbacks` before `Start()`, whose closure calls the existing `OtherLiveSessionInsideWorktree` | Callback injection (existing precedent, see Reason column); Fowler's Guard Clause; DRY | *(as originally planned, before this repair)* Leave both checks in `runBackgroundResolutionPipeline`, running only after `p.instance.Start(true)` returns | **Architecture-review Blocker 1 / Adversarial-review Blocker 1 fix.** Verified by direct read that `startLocked` (`session/instance.go:1467-1699`) spawns the tmux session and `claude` subprocess synchronously — `i.initTmuxSession()` at `:1607`, `i.pm().Start(startPath)` at `:1654` — before `Instance.Start()` returns. A check placed after `p.instance.Start(true)` in the pipeline (the original plan) can only detect a collision after the rogue process already exists, not prevent it — contradicting requirements.md's explicit "not just detect it after the fact" constraint, and leaving a detected collision's tmux/process running indefinitely since the pipeline's only failure action was a bare `commitTerminalStatus` write with no `i.Kill()`. The callback-injection shape is not new: `SessionService.wireCallbacks` (`server/services/session_service.go:1831`, called strictly before every `Start()`/`Start(true)` call — confirmed at the pipeline, `CreateDirectorySession`, `CreateWorktreeSession`, and `loadInstancesWithWiring`) and `ProcessManager.SetOnExitCallback` (wired inside `startLocked` at `session/instance.go:1496`/`:1753`) are both existing domain-defined ports wired by the service/process layer before/around `Start()` — reusing that exact shape avoids inverting the `session` (domain) → `server/services` (application) dependency direction a direct `OtherLiveSessionInsideWorktree` call from `startLocked` would create (it reads `s.reviewQueuePoller`, a `server/services` field). (`Instance.SetStatusManager` is deliberately NOT cited as this precedent, despite a superficially similar shape: every real call site — `session_creation_pipeline.go:312`, `session_service.go:1707`/`:1773` — wires it *after* the adjacent `Start(true)` call, per each site's own comment: "Wire the status manager and start the controller AFTER Start() returns... Starting the controller inside Start() caused immediate PTY EIO." An earlier plan draft cited `SetStatusManager` here; `wireCallbacks` is the correct, verified before-`Start()` precedent.) On error, the guard's failure returns through `startLocked`'s existing `setupErr`/`defer` cleanup (`session/instance.go:1511-1518`), which already calls `i.Kill()` — no new bare-status-write failure path needed, and the pipeline's existing `startErr != nil` branch (`session_creation_pipeline.go:258-263`) already routes any `Start()` error to a `Failed` terminal write via the existing `terminal()` chokepoint. `OtherLiveSessionInsideWorktree`/`RefuseIfWorktreeSharedWithOtherLiveSession` still supply the underlying canonicalized, liveness-filtered scan (already tested: `TestOtherLiveSessionInsideWorktree_*`, `server/services/liveness_consolidation_test.go`) — only the call site moved earlier via the callback, not the scanning logic itself. This guard call is unconditional on `i.SessionType` (nested only inside Story 3.3.1's `if i.gitManager.HasWorktree() {...}` block) — deliberately: the underlying collision risk (spawning into a directory another live session already owns) applies to any first-time session that resolves a real worktree, not `SessionTypeNewWorktree` alone. See Risk Control's scope claim and Story 3.3.2 below. |
| "No real worktree happened" detection (Epic 3.3) | Structural assertion: for `SessionTypeNewWorktree`, `basePath == i.Path` (equivalent to `Workspace().ActiveDir == Workspace().RepoRoot`) checked **inside `startLocked`, before `i.initTmuxSession()`** (relocated — see row above) | Type-driven design (an illegal state — "new-worktree session with no worktree" — made detectable, not just unrepresentable) | Export `session/git`'s private `getWorktreeDirectory()` and check `ActiveDir` is a path-prefix of it | `getWorktreeDirectory()` is unexported and internal to `session/git`; exporting it purely for a secondary validation would be a larger, riskier surface change than comparing `i.Path` against `basePath` (`startLocked`'s own local, `session/instance.go:1588-1599`), the same invariant `Workspace()` publishes post-hoc. Checked pre-spawn rather than post-`Start()` for the same Blocker-1 reason as the row above — this check needs no service-layer data, so it does not need the callback, only the relocation. |
| `DetectByPath`'s cross-session ownership blindness (new Epic 1.4) | Extend the collision-check primitive Epic 3.3 introduces (a live-instance-by-path lookup) to gate `tryExtractConversationUUID`'s DetectByPath fallback: don't attribute a JSONL to `i` when another live instance is the more plausible current owner of that path | DRY; reuse over new subsystem (same reasoning as the rows above) | Leave `DetectByPath` unaudited (status quo — the original plan's gap, per Adversarial-review Blocker 2) | `research/pitfalls.md`'s Designed-against checklist item 5 names this exact mechanism as the concrete explanation for the reporter's literal "tmux pane showed another session's conversation" symptom. Epic 3's directory-collision guards alone don't close it: it's a separate, path-keyed conversation lookup, not a worktree-resolution decision, and remains reachable even after Epic 3.3 closes the `SessionTypeNewWorktree`-vs-`SessionTypeNewWorktree` collision case (e.g. via `SessionTypeDirectory`'s legitimate path-sharing, or the `ExistingDir`-falls-back-to-`RepoRoot` case `.claude/rules/instance-lock-free-reads.md` documents as normal). |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `resolveSessionType`'s proto-enum `default:` arm (`session_service.go:3121-3122`) | Silently converts any unrecognized `sessionv1.SessionType` value to `SessionTypeDirectory`, no log/error — one of "two independent silent-fallback-to-Directory layers stacked" per `research/pitfalls.md` §3a | **Refactor-first** | This feature (Story 3.1) is squarely about closing exactly this silent-fallback shape; deferring it would mean shipping Epic 3 without actually closing the layer research identified as "the strongest concrete lead this research task turned up." |
| `setupFirstTimeWorktree`'s Go-switch `default:` arm (`session/instance_worktree.go:194-210`) | Same silent-fallback shape, one layer down; also mutates `i.gitManager`/`i.Branch` with no `i.mu.Lock()` and no snapshot republish (`research/pitfalls.md` §3a/§3b) | **Refactor-first** | Same reasoning; additionally this function is the one `.claude/rules/instance-lock-free-reads.md` already fixed for a different field (`i.Path`) — leaving this second gap in the same function while fixing the first would be inconsistent within the same rule's scope. |
| `session_service.go:2958`'s pre-`Start()` `instanceRootDir` capture | Verified real bug (architecture.md Q5): only refreshed inside the `deferredGitHubURL` branch, stale for every plain `SessionTypeNewWorktree` session | **Refactor-first** | Already VERIFIED by direct code read, independently fixable, and directly named in this project's requirements.md — no reason to isolate or defer it. |
| `session.WorkspacePeersBlockForPath` / the `#801` workspace-peers nudge feature | Not a violation — a working, tested, differently-scoped mechanism (workspace-key granularity: "any peer in this repo family," expected/normal) that `instance-lock-free-reads.md` already fixed once (the `ActiveDir`/`ExistingDir` split) | **Not touched — extend as-is elsewhere, do not conflate** | This plan's collision guard (Story 3.3) answers a different question — "does this exact `ActiveDir` collide with another live session's exact `ActiveDir`" — and must not reuse the peers nudge's workspace-key-level matching, which would false-positive on every normal multi-worktree session. Recorded here so a future reader doesn't merge the two mechanisms. |
| `OtherLiveSessionInsideWorktree`'s silent `("", false)` return when `s.reviewQueuePoller == nil` (`server/services/session_service.go:1237`) | Pre-existing, narrow escape hatch, now reused by Story 3.3.2's `SetPreSpawnCollisionGuard` callback closure — flagged by Adversarial-review Concern 3 as risky to inherit silently for a severity-scoped guard | **Extend as-is, but log** | Restructuring `OtherLiveSessionInsideWorktree`'s own contract would also change behavior for its existing callers (pause/stop/delete guards), which is out of this bug fix's scope. Story 3.3.2's callback closure instead `log.Warn`s when it observes `s.reviewQueuePoller == nil` before returning "no collision," so a misconfigured/degenerate `SessionService` construction (should not happen in production, per `wireCallbacks`'s universal wiring) is visible in logs rather than silently permissive. |
| `DetectByPath`'s ownership-blind most-recently-modified JSONL selection (`session/history_detector.go:154`) | Confirmed-reachable from a not-yet-populated new `Instance` via `SwitchWorkspace` (`session/instance_workspace.go:75-80`) and `ClaudeAdapter.Import` (`session/claude_adapter.go:57-61`) — found during this plan-repair pass; see new Epic 1.4 | **Refactor-first** | This is the literal mechanism `research/pitfalls.md`'s Designed-against checklist names for the reporter's "saw another session's conversation" symptom; deferring it would leave requirements.md's "No cross-session attach" Success Metric unsatisfied even after Epic 3's directory-collision guards land, since this is a separate, path-keyed code path Epic 3 doesn't touch. |

---

## Migration Plan

N/A — no schema or data changes. `InstanceSnapshot` already carries every field this plan reads (`SessionType`, `Path`/`RepoRoot` via `Snapshot()`).

## Observability Plan
- **Logs**: Story 1.3 adds one new Debug-level structured log line at the top of `CreateSession` (the literal wire-level `session_type`/`branch`/`working_dir`/`envVars` key set/`profile`/`skip_defaults`/`restart_from_session_id`/`existing_worktree`/`alias_name` fields) so a future recurrence doesn't need a fresh repro to see what the server actually received. Every new `log.Error`/`log.Warn` this plan adds (Stories 3.1-3.3) follows the existing `"[session pipeline] ..."` / `"[CreateSession] ..."` prefix convention already used in `session_creation_pipeline.go`/`session_service.go`.
- **Metrics**: Story 3.3's new Failed outcomes route through the pipeline's existing `terminal()` closure, so they already get `RecordSessionCreationMetrics(ctx, o.metricsOutcome, ...)` for free — use a new, distinct `metricsOutcome` value per new failure class (e.g. `"WorktreeResolutionFailed"`, `"DirectoryCollision"`) rather than reusing the generic `SessionCreationOutcomeFailed` alone, so these new failure modes are distinguishable in dashboards from pre-existing ones (GitHub resolution failure, startup error).
- **Alerts**: None new — this plan makes existing silent failures loud within the existing terminal-status/metrics machinery; no new alerting channel is needed for a bug-fix-scoped change.

## Risk Control
- **Feature flag**: None. Every change either (a) fixes an already-verified bug with no behavioral ambiguity (Epic 2), or (b) converts an already-silent failure into a loud one for cases that, by the requirements doc's own Constraints, must never have been silently succeeding in the first place (Epic 3) — there is no "old silent behavior" worth preserving behind a flag. **Scope correction (plan-repair pass 3):** Epic 3.3's collision guard is *not* scoped to `SessionTypeNewWorktree` alone — Story 3.3.2's guard call is unconditional on `SessionType`, gated only by Story 3.3.1's `i.gitManager.HasWorktree()` check. This is intentional, not a gap: it applies to every `firstTimeSetup` session that resolves a real worktree — `SessionTypeNewWorktree`, `SessionTypeExistingWorktree`, and the worktree sub-branch of `SessionTypeNewProject` — because the underlying risk (spawning a process into a directory another live session already owns) is a property of "did this session resolve a worktree," not of which request shape asked for one; artificially gating the guard to `SessionTypeNewWorktree` would leave the other two worktree-bearing types exposed to the identical hijack vector this epic exists to close. It still cannot regress the many legitimate `SessionTypeDirectory` sessions that intentionally share a path: `SessionTypeDirectory` never has a `gitManager` worktree, so `i.gitManager.HasWorktree()` is always `false` for it and the guard call — nested inside that same `if` block — is never reached.
- **Rollback procedure**: Standard `git revert` of the merged PR(s) — no data migration, no flag state, no external dependency to unwind.
- **Staged rollout**: Not applicable to a single-binary, self-hosted tool with one deployed instance per user (per this repo's own deployment model — see `make install-service`'s warning about restarting the single live service). Ship behind normal PR review + `make ready`/`make ci` gates, per this repo's existing workflow.
- **Lock-order safety of Epic 3.3/1.4's guard closures (pre-mortem Failure #3, P3 — verified during plan repair, no deadlock risk):** `startLocked` (`session/instance.go:1467-1699`) never acquires `i.mu` anywhere in its body — confirmed by direct read of the full function; the only lock it takes is the unrelated `i.pmMu` via `i.pm()` (`session/instance_tmux.go:227-239`), and only for the instance's own process-manager accessor. The new guard call sites (Story 3.3.2's `SetPreSpawnCollisionGuard`, Story 1.4.2's `SetConversationOwnershipGuard`) run between `basePath = i.gitManager.GetWorktreePath()` resolving and `i.initTmuxSession()` (`:1599-1607`), also outside any `i.mu` critical section. The closures' cross-instance calls — `inst.IsBackendProcessAlive()` and `inst.GetCurrentWorkingDirectory()` (`session/instance_tmux.go:888-911`) — both route only through `inst.pm()`, i.e. that OTHER instance's own `pmMu`, never `i.mu` on either side. So the pre-mortem's feared AB-BA pattern (instance A holding its own lock while waiting on B's, and vice versa) cannot occur: neither `startLocked` nor the guard closures hold any lock of the calling instance while acquiring a lock on the target instance, and each instance's `pmMu` only ever guards that same instance's own process-manager field. Pre-mortem Failure #3 is resolved by this verification, not by a code change — no lock reordering is needed.

## Unresolved Questions

**Plan-repair note (addresses Adversarial-review Concern "does not hold as scoped"):** the original Pattern B claim — "Epic 3's guards close the vector regardless of which hypothesis turns out to be the true differentiator" — was flagged as false-as-scoped because Epic 3.3's guards ran after process spawn (Blocker 1) and because `DetectByPath`'s ownership blindness was untouched (Blocker 2). Both are now fixed in this plan: Epic 3.3's guards are relocated into `startLocked`, before any spawn (true prevention, not detection), and new Epic 1.4 closes the `DetectByPath` gap. Pattern B's claim now holds as scoped — recorded here so a future reader doesn't need to re-derive it from the two blockers' remediation text.

Also strengthening (Adversarial-review Minor 2): Task 1.1.1b's and Task 1.2.1b's checkbox updates below are a **blocking gate**, not documentation of intent — Epic 3.4/3.5's conditional/dropped status must be resolved and recorded in this file before Epic 4 (regression tests) is considered complete, regardless of how the implementation work is distributed across subagents.

- [ ] Does Hypothesis #2 (branch-name collision with the *main checkout*, hitting `findLiveWorktreeForBranch`'s un-self-healed gap) actually reproduce as a **silent** `Active`, or does it correctly propagate as a `Failed` error as `research/architecture.md`'s own static reading predicts? — blocks **Story 3.4 (conditional)** — owner: whoever executes Story 1.1's repro test; if CONFIRMED silent, Story 3.4 (harden `findLiveWorktreeForBranch`'s main-checkout blindness) must be added to Epic 3 before Epic 3 is considered complete; if REFUTED (propagates loudly as expected), Story 3.4 is dropped entirely — no code change needed there.
- [x] Does the web-app's envVars/advanced-options request-construction code (or an MCP tool's) actually drop/alter `session_type` or `branch` when populated — confirming Hypothesis #1? — **REFUTED (Task 1.2.1, 2026-09-27).** Stronger finding than "constructed identically regardless of envVars state": neither audited client path can populate `env_vars` on `CreateSessionRequest` at all, so no envVars-conditioned coupling with `session_type`/`branch` is possible in either. (1) Web-app Omnibar: `OmnibarSessionData` (`web-app/src/components/sessions/Omnibar.tsx:195-230`) has no `envVars` field and the creation form has no advanced-options envVars panel; `OmnibarContext.tsx`'s `handleCreateSession` (`web-app/src/lib/contexts/OmnibarContext.tsx:242-283`) computes `sessionType`/`branch` from `data.isNewProject`/`data.sessionType`/`data.branch` only, with no envVars involved; and `useSessionService.ts`'s `createSession` (`web-app/src/lib/hooks/useSessionService.ts:361-396`) builds the ConnectRPC request body from an explicit, enumerated field list that omits `envVars` entirely (would be silently dropped even if a caller passed one) while threading `branch`/`sessionType` straight through unconditionally (`:373`, `:380`). (2) MCP tool: `server/mcp/tools_lifecycle.go`'s `createSessionWithAwaitTimeout` (`:122-201`) re-confirmed directly — its parameter schema (`:56-63`) exposes no env-var argument at all, and the `sessionv1.CreateSessionRequest` it constructs (`:190-201`) sets only `Title`, `Path`, `Branch`, `Program`, `SessionType`, `Tags` — `EnvVars` is never set, so no coupling is possible here either. Story 3.5 is **dropped** — Epic 3's defensive guards (3.1-3.3) are the sole closure for the isolation vector, per the requirements doc's Success Metrics ("either a real isolated worktree is created, or ... surfaces a visible ... error"). Note: since neither audited path can set `env_vars` at all, if the original repro genuinely had a non-empty `envVars` field, its request must have originated from a caller outside this audit's scope (e.g. a different MCP server/orchestration client) — worth flagging to whoever owns the original repro, but out of scope for this plan's file set.
- [ ] Neither hypothesis may confirm (both refuted, as Epic 1's static research already leans toward) — in that case the true differentiator between Request A and Request B remains formally unknown. This is an acceptable outcome for this plan: requirements.md's Success Metrics are written to be satisfiable either way (root cause identified with a file:line citation OR the "No silent worktree-creation skip"/"No cross-session attach" metrics are structurally satisfied), and Epic 3's guards close the vector structurally regardless of the exact trigger. — owner: plan approver, decide at Epic 1's completion whether to accept "vector closed, exact differentiator unconfirmed" as done, or escalate appetite per requirements.md's own escalation clause.

**Pre-mortem P1 remediation (Failure #1):** the "Epic 3's guards close the
vector regardless of hypothesis" claim above only covers Hypotheses #1/#2 —
it does not, on its own, rule out an undiscovered third trigger that
legitimately resolves `SessionType` to `SessionTypeDirectory` upstream of
`startLocked` (e.g. via program resolution), which would make Epic 3's
`SessionType`-gated guards never fire. New, unconditional **Epic 1.5**
re-verifies (building on `research/architecture.md`'s Q1/Q2, which already
show `program` cannot influence `resolveSessionType`'s inputs) that no such
path exists today, AND adds a permanent invariant metric/Warn log so any
future, currently-unknown third trigger is observable in production instead
of silently bypassing Epic 3 — closing the gap pre-mortem Failure #1
identified without waiting on a hypothetical fourth research pass.
- [ ] Task 1.5.1a's re-verification note — blocks nothing (Epic 1.5 is
  unconditional and independent), but its conclusion should be checked
  against Q1/Q2 before Epic 1 is considered fully closed — owner: whoever
  executes Epic 1.5.

## Dependency Visualization

```
Epic 1 (Confirm Root Cause) ─┬─> Story 1.1 (Hypothesis #2 repro test)  ──┐
                              ├─> Story 1.2 (client-code audit, Hyp #1) ─┼─> [Unresolved Questions gate]
                              ├─> Story 1.3 (diagnostic logging)         │      │
                              ├─> Story 1.4 (DetectByPath ownership      │      ├──> Story 3.4 (conditional, if #2 confirmed)
                              │   audit + fix — unconditional, not       │      └──> Story 3.5 (conditional, if #1 confirmed)
                              │   gated on #1/#2)                       │
                              └─> Story 1.5 (unexamined-third-trigger    │
                                  re-verification + invariant metric —  │
                                  unconditional, pre-mortem P1 fix)     │
                                                                         │
Epic 2 (Fix stale instanceRootDir) ── independent, no dependency ──────────────────────┐
  Story 2.1 (re-derive after Start())                                                  │
                                                                                        ▼
Epic 3 (Defensive hardening) ── independent of Epic 1's outcome, AND        [merge point]
  independent of Epic 2 (see note below) ──────────────────────────────────────┐
  Story 3.1 (resolveSessionType error) ──┐                                     │
  Story 3.2 (setupFirstTimeWorktree      │                                     │
             error + snapshot fix)  ─────┤                                     │
  Story 3.3 (session/instance.go's       │                                     │
             startLocked guards:         │                                     │
             structural + collision,     │                                     │
             pre-spawn — see relocation  │                                     │
             note) ──────────────────────┴─────────────────────────────────────┘
Epic 4 (Regression tests) ── depends on Epics 1.4, 1.5, 2 & 3 landing first ──> [ship]
  Story 4.1 (ModeIsNewWorktree + envVars + collision sim + conversation-UUID disjointness)
  Story 4.2 (Epic 2 regression: InjectHookConfig targets worktree)
  Story 4.3 (Epic 3.1/3.2 regression: unrecognized SessionType → Failed)
  Story 4.4 (NEW — session-package unit test: SetPreSpawnCollisionGuard actually
             prevents spawn, deterministic, no fault-injection needed)
  Story 4.5 (NEW — server/services integration test: wireCallbacks actually wires
             the guard end-to-end via SetGitWorktree(NewGitWorktreeFromStorage(...))
             forcing a fabricated worktree-path collision)
```

Epics 2 and 3 do not depend on Epic 1's outcome and can be implemented in
parallel with it. **Relocation note (Blocker-1 remediation):** Story 3.3's
guards moved from `runBackgroundResolutionPipeline` (which would have made
them depend on Epic 2's Task 2.1.1a — the undeclared dependency
Architecture-review Concern 2 flagged) into `session/instance.go`'s
`startLocked`, which reads only its own local `basePath`/`i.Path` and the
new `SetPreSpawnCollisionGuard` callback — genuinely independent of Epic 2's
`instanceRootDir` refresh now, not just documented as such. Epic 4 depends
on Epics 2 and 3 (it asserts on their fixes) and now also on Epic 1.4 (it
asserts DetectByPath's ownership fix). Epic 1's two conditional follow-up
stories (3.4, 3.5) are gated on its own findings, not on Epics 2-4; Epic 1.4
is NOT conditional — it runs unconditionally (see Epic 1.4's Goal).

---

## Epic 1: Confirm Root Cause

**Goal**: Resolve requirements.md's Success Metric "repro confirmed or
refuted with evidence" for the two remaining, unverified hypotheses
(`research/architecture.md`'s #1 and #2) before treating the investigation as
closed. Runs first / in parallel with Epics 2-3; gates the two conditional
stories in Epic 3.

### Epic 1.1: Confirm or refute the branch-collision hypothesis (#2)

**Goal**: Determine whether a branch already checked out in the *main*
checkout (not a linked worktree) causes `setupFirstTimeWorktree`'s
`SessionTypeNewWorktree` case to silently succeed against the bare repo path,
or whether — as the static trace predicts — it correctly fails loudly.

#### Story 1.1.1: Controlled Go test reproducing the main-checkout branch collision
**As a** developer investigating this bug, **I want** an executable test that
puts a branch in conflict between a live `SessionTypeDirectory` session (main
checkout) and a new `SessionTypeNewWorktree` request for the same branch, **so
that** Hypothesis #2 is confirmed or refuted with a passing/failing test, not
just static reading.

**Acceptance Criteria**:
- A new Go test in `server/services/session_service_test.go` (or a new file
  `server/services/session_service_worktree_hijack_test.go` if the existing
  file is judged too large to extend further) creates a real git repo
  (`initGitRepoWithCommit`, `server/services/git_fixture_test.go:78`), seeds a
  live instance checked out on branch `"collide-branch"` *in the bare repo
  itself* (not a linked worktree — e.g. via `go-git`'s `Worktree.Checkout`
  against a new branch ref, mirroring how a real `SessionTypeDirectory`
  session's tmux pane would leave the main checkout), then calls
  `CreateSession` with `SessionType: SESSION_TYPE_NEW_WORKTREE, Branch:
  "collide-branch"` against the same repo path.
  - *Given* a bare repo at `repoDir` with a live, non-terminal instance whose
    actual git HEAD is `collide-branch`, *When* `CreateSession` is called
    with `Path: repoDir, SessionType: SESSION_TYPE_NEW_WORKTREE, Branch:
    "collide-branch"`, *Then* the resulting session reaches either (a)
    `session.Failed` with a `FailureReason` mentioning the worktree/branch
    conflict (Hypothesis #2 REFUTED for its "no error" half, matching
    `research/architecture.md`'s prediction), or (b) `session.Active` with
    `inst.Workspace().ActiveDir == repoDir` (Hypothesis #2 CONFIRMED — a real,
    previously-unknown silent-swallow gap).
- The test's result is recorded verbatim (pass/fail and which branch fired)
  in a short note at the top of this file's Epic 3.4 section reference (or in
  the PR description) — not left only in CI output — so Story 3.4's
  conditional status can be resolved without re-running the test.
**Files**: `server/services/session_service_test.go` (or new
`session_service_worktree_hijack_test.go`), reusing
`server/services/git_fixture_test.go`.

##### Task 1.1.1a: Write the branch-collision repro test (~5 min)
- Add the test described above, following `TestCreateSession_should_ReachActiveViaPipeline`'s `ModeIsRestart` subtest's pattern for seeding a second, pre-existing live instance (`fix.storage.AddInstance` + real tmux via `fix.svc.testTmuxServerSocket`), and `initGitRepoWithCommit` for the repo fixture.
- Run it with `go test ./server/services -run TestCreateSession_MainCheckoutBranchCollision -v` and record the outcome.
- Files: `server/services/session_service_test.go` (or new file), `server/services/git_fixture_test.go` (read-only reuse).

##### Task 1.1.1b: Record the confirmed/refuted verdict (~2 min)
- Update this plan's Unresolved Questions checkbox for Hypothesis #2 with the actual result and, if CONFIRMED, add a concrete Story 3.4 task list (currently a placeholder — see Epic 3.4 below) describing the exact fix `findLiveWorktreeForBranch`/`nativeFindLiveWorktreeForBranch` needs.
- Files: `project_plans/worktree-envvars-hijack/implementation/plan.md`.

### Epic 1.2: Audit the client-side request-construction path (Hypothesis #1)

**Goal**: Close the one blind spot `research/architecture.md` explicitly
flagged as out of its scope — whether the web-app's envVars/advanced-options
panel (or an MCP tool) constructs a `CreateSessionRequest` with a
different/missing `session_type` or `branch` when envVars is populated.

#### Story 1.2.1: Read and trace the web-app's session-creation request builder
**As a** developer confirming root cause, **I want** to know whether
populating the envVars/advanced-options UI panel can change what
`session_type`/`branch` field values actually get sent, **so that**
Hypothesis #1 is confirmed or refuted with a file:line citation instead of
staying speculative.

**Acceptance Criteria**:
- The web-app's session-creation form component (Omnibar's new-session flow)
  and its submit handler are read in full, specifically tracing how
  `envVars`/advanced-options state interacts with the `session_type` and
  `branch` fields of the constructed `CreateSessionRequest`.
  - *Given* the web-app's Omnibar create-session form with the
    envVars/advanced-options panel expanded and populated, *When* the form is
    submitted, *Then* this task's finding is one of: (a) `session_type` and
    `branch` are constructed identically regardless of envVars state
    (Hypothesis #1 REFUTED — cite the exact component/function read), or (b)
    a specific code path is found where populating envVars can leave
    `session_type` unset or `branch` empty (Hypothesis #1 CONFIRMED — cite
    file:line and the exact condition).
- The MCP tool path (`server/mcp/tools_lifecycle.go`, already spot-checked by
  `research/architecture.md` as "maps `session_type` independently of
  envVars/program with no coupling") is re-confirmed with a direct read, not
  re-relied-upon from the prior spot-check alone.
**Files**: `web-app/src/` (exact file TBD by the audit — likely the Omnibar's
session-creation form/hook), `server/mcp/tools_lifecycle.go` (read-only
re-confirmation).

##### Task 1.2.1a: Locate and read the Omnibar's create-session submit path (~5 min)
- Search `web-app/src/` for the component/hook that builds the
  `CreateSessionRequest` proto client-side (likely near
  `session-create-new-worktree.spec.ts`'s tested UI, per
  `docs/reference/session-creation-registry.md`'s touchpoint list).
- Trace envVars/advanced-options state through to the final request object.
- Files: `web-app/src/` (component TBD).

##### Task 1.2.1b: Re-confirm the MCP tool path and record the verdict (~3 min)
- Read `server/mcp/tools_lifecycle.go`'s `session_type`/`branch`/`envVars`
  handling directly (not just accept the prior research's spot-check).
- Update this plan's Unresolved Questions checkbox for Hypothesis #1 with the
  result; if CONFIRMED, note the exact file:line for a follow-up fix (Story
  3.5, likely scoped as its own `sdd:fix-bug` cycle since it's client-side
  and outside this plan's primary Go file set).
- Files: `server/mcp/tools_lifecycle.go`,
  `project_plans/worktree-envvars-hijack/implementation/plan.md`.

### Epic 1.3: Add low-cardinality diagnostic logging for future recurrence

**Goal**: Even if Epic 1.1/1.2 close without a confirmed differentiator,
leave a cheap, permanent diagnostic so a future recurrence doesn't require a
fresh repro to see what the server actually received.

#### Story 1.3.1: Log the wire-level CreateSessionRequest shape at Debug level
**As an** on-call developer debugging a future isolation report, **I want**
the exact `session_type`/`branch`/`working_dir`/envVars-key-set/`profile`/
`skip_defaults`/`restart_from_session_id`/`existing_worktree`/`alias_name`
fields logged for every `CreateSession` call, **so that** I don't need to
reproduce the bug live to see what the client actually sent.

**Acceptance Criteria**:
- `CreateSession` logs one Debug-level structured line, immediately after the
  Title/path validation at the top of the function, containing every field
  named above. envVars values themselves are NOT logged (only the key set) —
  they can carry secrets like API base URLs/tokens.
  - *Given* a `CreateSessionRequest` with `SessionType:
    SESSION_TYPE_NEW_WORKTREE, Branch: "test/x", EnvVars: {"ANTHROPIC_BASE_URL":
    "http://127.0.0.1:47000"}`, *When* `CreateSession` is called with
    `STAPLER_SQUAD_LOG_LEVEL=debug`, *Then* `logs/staplersquad.log` contains
    one JSON line with `session_type="SESSION_TYPE_NEW_WORKTREE"`,
    `branch="test/x"`, and `env_var_keys=["ANTHROPIC_BASE_URL"]` (not the
    value).
**Files**: `server/services/session_service.go`.

##### Task 1.3.1a: Add the Debug-level request-shape log line (~4 min)
- Insert `log.Debug("[CreateSession] request shape", "session_type", req.Msg.SessionType, "branch", req.Msg.Branch, "working_dir", req.Msg.WorkingDir, "env_var_keys", slices.Collect(maps.Keys(req.Msg.EnvVars)), "profile", req.Msg.Profile, "skip_defaults", req.Msg.SkipDefaults, "restart_from_session_id", req.Msg.RestartFromSessionId, "existing_worktree", req.Msg.ExistingWorktree, "alias_name", req.Msg.AliasName)` right after the Title/path validation block (`session_service.go:2312-2317`).
- Files: `server/services/session_service.go`.

### Epic 1.4: Audit and close `DetectByPath`'s cross-session ownership blindness

**Goal**: Close the gap Adversarial-review Blocker 2 identified: `research/pitfalls.md`'s
Designed-against checklist item 5 names `session/history_detector.go`'s
`DetectByPath` — a path-keyed scan for "the most recently modified" JSONL
with no per-session ownership check, feeding `buildLaunchCommand`'s
`claudeSessionID` — as the concrete mechanism that could explain the
reporter's literal symptom (a new session's tmux pane showing another live
session's conversation content). No story in the original plan's Epics 1-4
touched it. Unlike Epic 3.4/3.5, this epic is **not conditional** — it
always runs, because this plan-repair pass's own re-read (below) already
found reachable call sites, not just a hypothetical gap.

**Findings already confirmed during plan repair** (Task 1.4.1a's starting
point, not a full substitute for it):
- `recoverConversationBeforeLaunch` (`session/instance.go:1421-1426`) is a
  no-op whenever `firstTimeSetup` is true — confirmed by direct read, this
  matches requirements.md's existing baseline claim that a brand-new
  `CreateSession` call never reaches `DetectByPath` through this path.
- `startLocked`'s `ColdRestore` branch's direct call
  (`session/instance.go:1568`, `i.tryExtractConversationUUID()`) only runs
  when `firstTimeSetup` is false — also not reachable for a brand-new
  session's own `Start()` call.
- `SwitchWorkspace` (`session/instance_workspace.go:75-80`) calls
  `i.tryExtractConversationUUID()` unconditionally on every workspace
  switch — **reachable** for a session created moments earlier if a
  workspace switch is requested before its own conversation UUID is known.
- `ClaudeAdapter.Import` (`session/claude_adapter.go:57-61`) calls
  `inst.tryExtractConversationUUID()` whenever
  `inst.GetClaudeConversationUUID() == ""` — true for every session between
  creation and its own conversation being detected — **reachable** for a
  UI/API caller reading conversation history for a just-created session
  (e.g. a history preview racing session creation).

#### Story 1.4.1: Complete the reachability audit
**As a** developer closing the cross-session-attach vector, **I want** every
call site of `tryExtractConversationUUID`/`DetectByPath` reachable from a new
or young `Instance` enumerated and classified, **so that** Story 1.4.2 fixes
a fully-scoped problem instead of only the two call sites this repair pass
happened to find.

**Acceptance Criteria**:
- Every call site of `tryExtractConversationUUID` and `DetectByPath` is
  grep-enumerated and classified as reachable-from-a-new-Instance or not,
  expanding on (and confirming or correcting) the four findings above.
- For each reachable call site, determine whether its actual
  collision precondition — two live instances whose `GetEffectiveRootDir()`
  resolves to the same directory at the moment the call runs — can occur at
  all *after* Epic 3.3's guard lands. Epic 3.3.2's guard is unconditional on
  `SessionType` (see Risk Control's scope correction), so it makes it
  structurally impossible for any two worktree-bearing first-time sessions
  (`SessionTypeNewWorktree`, `SessionTypeExistingWorktree`, or
  `SessionTypeNewProject`'s worktree sub-branch, in any combination) to ever
  share an `ActiveDir`; the remaining risk is scoped to `SessionTypeDirectory`'s
  legitimate by-design path sharing and the `ExistingDir`-falls-back-to-
  `RepoRoot` case `.claude/rules/instance-lock-free-reads.md` documents as
  normal — confirm this scoping directly rather than assuming it.
**Files**: `session/instance_workspace.go`, `session/claude_adapter.go`,
`session/instance.go`, `session/history_detector.go`,
`session/instance_claude.go` (read-only audit).

##### Task 1.4.1a: Enumerate and classify every call site (~10 min)
- `grep -rn "tryExtractConversationUUID\|DetectByPath" session/` and classify
  each result per the AC above.
- Files: read-only.

##### Task 1.4.1b: Determine post-Epic-3.3 collision reachability and record the verdict (~10 min)
- For each reachable call site from Task 1.4.1a, confirm whether its
  collision precondition survives Epic 3.3's guard, per the AC above.
- Record the verdict in this plan (update this section with the final list).
- Files: `project_plans/worktree-envvars-hijack/implementation/plan.md`.

#### Story 1.4.2: Close any confirmed ownership gap
**As a** user with multiple concurrent sessions, **I want**
`tryExtractConversationUUID`'s `DetectByPath` fallback to never attribute a
JSONL conversation file to an `Instance` that isn't its actual owner, **so
that** the literal "new session showed another session's conversation"
symptom is closed at its source, not just at the directory-resolution layer
Epic 3 addresses.

**Acceptance Criteria**:
- `tryExtractConversationUUID`'s `DetectByPath` fallback
  (`session/instance_claude.go:408-430`) does not adopt a detected JSONL's
  conversation UUID for `i` when another live `Instance` is a more plausible
  current owner of that same effective path. This needs a genuinely new
  query — not a reuse of `OtherLiveSessionInsideWorktree` (Story 3.3.2's
  guard), which only checks path/liveness overlap and has no concept of
  "does that other session already own this conversation UUID." Specified
  concretely, at the same level of detail as Story 3.3.2's closure:
  - New `SessionService` method, mirroring `OtherLiveSessionInsideWorktree`'s
    shape (`server/services/session_service.go:1236`) but keyed on UUID+path
    together, not path alone:
    ```go
    // ConversationOwnedByOtherLiveSession reports whether conversationUUID is
    // already the ConversationUUID of some OTHER currently-live,
    // backend-process-alive Instance (any UUID besides selfUUID) whose own
    // GetCurrentWorkingDirectory() resolves to path. Unlike
    // OtherLiveSessionInsideWorktree, path overlap alone is not enough to
    // block -- the sibling must also already own this exact UUID -- so the
    // legitimate SessionTypeDirectory path-sharing case (Story 1.4.1) is
    // never blocked from a session detecting its OWN first conversation,
    // only from silently adopting a sibling's.
    func (s *SessionService) ConversationOwnedByOtherLiveSession(selfUUID, conversationUUID, path string) (ownerUUID string, ownedByOther bool) {
        if s.reviewQueuePoller == nil || conversationUUID == "" || path == "" {
            return "", false
        }
        cleanTarget, err := filepath.Abs(path)
        if err != nil {
            return "", false
        }
        cleanTarget = git.CanonicalizeWorktreePath(cleanTarget)
        for _, inst := range s.reviewQueuePoller.GetInstances() {
            if inst == nil || inst.UUID == selfUUID || !inst.IsBackendProcessAlive() {
                continue
            }
            if inst.GetClaudeConversationUUID() != conversationUUID {
                continue
            }
            cwd, cwdErr := inst.GetCurrentWorkingDirectory()
            if cwdErr != nil || cwd == "" {
                continue
            }
            cleanCwd, absErr := filepath.Abs(cwd)
            if absErr != nil {
                continue
            }
            if git.CanonicalizeWorktreePath(cleanCwd) == cleanTarget {
                return inst.UUID, true
            }
        }
        return "", false
    }
    ```
  - New `Instance` field + setter, same callback-injection shape as
    `SetPreSpawnCollisionGuard` (a *different* port — a different data
    shape, not a reuse of the same field): `Instance.SetConversationOwnershipGuard(guard
    func(candidateUUID, path string) (ownerUUID string, ownedByOther bool))`,
    wired by `wireCallbacks` alongside `SetPreSpawnCollisionGuard`:
    `inst.SetConversationOwnershipGuard(func(candidateUUID, path string) (string, bool) {
    return s.ConversationOwnedByOtherLiveSession(inst.UUID, candidateUUID, path) })`.
  - In `tryExtractConversationUUID`'s `DetectByPath` branch
    (`session/instance_claude.go:408-431`), immediately after `info` is
    confirmed non-nil (post the existing `clearedAt` check) and before
    falling through to the shared assignment block at `:443-456`: if
    `i.conversationOwnershipGuard != nil`, call
    `i.conversationOwnershipGuard(info.ConversationUUID, effectivePath)`; if
    `ownedByOther`, set `info = nil` and `log.Debug` the owning session's
    UUID instead of adopting it.
  - *Given* two live Instances, A (pre-existing, real conversation UUID `X`,
    JSONL at path `P`) and B (freshly created, no UUID yet, whose
    `GetEffectiveRootDir()` also resolves to `P` — the legitimate
    `SessionTypeDirectory`-path-sharing case Story 1.4.1 confirmed can still
    occur), *When* B's `tryExtractConversationUUID` fallback runs, *Then* B
    does not silently adopt UUID `X` — it finds nothing (no silent
    cross-attach), logging why at Debug level.
  - *Given* the same setup but A is no longer alive (already exited), *When*
    B's `tryExtractConversationUUID` fallback runs, *Then* B may still adopt
    `X` as before (no regression to the legitimate single-owner recovery
    case this fallback exists for).
  - **cold-start-uuid-loss regression case (pre-mortem Failure #4, P2).**
    Verified during plan repair: `Instance.UUID` is stable across a restart —
    `fromInstanceData` reloads it verbatim (`UUID: data.UUID`,
    `session/instance_serialization.go:262`) rather than regenerating it, and
    `startLocked`'s `ColdRestore` branch (tmux dead, `firstTimeSetup=false`)
    reuses that same in-memory `*Instance` object to call
    `i.tryExtractConversationUUID()` (`session/instance.go:1568`), not a new
    object — so `ConversationOwnedByOtherLiveSession`'s own `inst.UUID ==
    selfUUID` exclusion already skips comparing the restarting instance
    against itself. *Given* a session S undergoing `ColdRestore` (its tmux
    session died, `firstTimeSetup=false`, S's own prior
    `ClaudeConversationUUID` was lost and needs re-detection via
    `DetectByPath`) whose own `*Instance` object is present in
    `s.reviewQueuePoller.GetInstances()` at the moment
    `tryExtractConversationUUID`'s new ownership-guard call runs, *When* the
    guard iterates live instances looking for another owner of the
    UUID `DetectByPath` just found, *Then* S's own record is skipped via the
    `inst.UUID == selfUUID` check and S successfully re-adopts its own
    conversation UUID — the guard must never block a session's restart-time
    self-recovery of its own conversation. This scoped verification does not
    cover a hypothetical second, distinct `*Instance` object sharing S's UUID
    (no such path was found reachable during this repair pass); Task 1.4.2a's
    implementer should re-confirm no such path exists before treating this
    case as fully closed.
- A regression test seeds a live session with a real, fake JSONL (known
  UUID) and a second session whose effective root resolves to the same
  directory, asserting the second session's `claudeSession.ConversationUUID`
  never gets silently set to the first's UUID via `DetectByPath` while the
  first is still alive.
- A second regression test drives the `ColdRestore` self-recovery case above:
  seed a live, self-registered `Instance` (registered in the test's
  `reviewQueuePoller` double, same UUID before and after) with a real, fake
  JSONL at its own effective root and no `ClaudeConversationUUID` set yet,
  call `startLocked`'s `ColdRestore` path (or `tryExtractConversationUUID`
  directly, whichever is more directly testable), and assert the instance
  successfully adopts its own JSONL's UUID — never blocked by
  `ConversationOwnedByOtherLiveSession` treating its own record as "other."
**Files**: `session/instance_claude.go`, `session/instance.go` (new
`SetConversationOwnershipGuard` field/setter), `server/services/session_service.go`
(new `ConversationOwnedByOtherLiveSession` method + `wireCallbacks` wiring),
new test in `session/instance_claude_test.go` or `session/history_linker_test.go`.

##### Task 1.4.2a: Add the ownership check (~8 min)
- Implement `ConversationOwnedByOtherLiveSession`, `SetConversationOwnershipGuard`,
  the `wireCallbacks` wiring, and the `tryExtractConversationUUID` call site
  described above, gated on Task 1.4.1's confirmed scope (skip entirely if
  Task 1.4.1b concludes no reachable collision precondition survives Epic
  3.3). Before wiring, re-confirm (per the cold-start-uuid-loss AC above)
  that no code path registers a second, distinct `*Instance` object sharing
  a restarting session's UUID in `reviewQueuePoller` at the same time.
- Files: `session/instance_claude.go`, `session/instance.go`,
  `server/services/session_service.go`.

##### Task 1.4.2b: Add the regression tests (~10 min)
- Add both tests described in Story 1.4.2's AC: the cross-session
  non-adoption test, and the `ColdRestore` self-recovery test proving the new
  guard never blocks a session from re-detecting its own conversation UUID
  during a restart (pre-mortem Failure #4).
- Files: `session/instance_claude_test.go` (or nearest existing test file for
  `tryExtractConversationUUID`/`DetectByPath`).

### Epic 1.5 (NEW — pre-mortem P1 remediation): Close the unexamined-third-trigger gap

**Goal**: Pre-mortem Failure #1 (P1) observed that Epic 3's guards are gated
on `i.SessionType == SessionTypeNewWorktree` / `gitManager.HasWorktree()`, so
if the true production trigger is a mechanism *upstream* of `startLocked`
that legitimately resolves `SessionType` to `SessionTypeDirectory` — e.g.
`config.ResolveProgramConfig`'s program-specific resolution, named
"unexamined territory" in requirements.md's Rabbit Holes — the session
proceeds as a by-design-legitimate directory share and none of Epic 3's
guards ever fire, undermining Pattern B's "regardless of hypothesis" claim
for any trigger outside #1/#2. This epic is **unconditional**, like Epic 1.4,
not gated on Story 1.1/1.2's verdicts.

**Already substantially closed by prior research — this epic formalizes and
extends it, not starts from zero**: `research/architecture.md`'s Q1 (full
trace of every `sessionType`/`i.SessionType` reassignment site between
`resolveSessionType()` and `instanceOpts := session.InstanceOptions{...}`)
and Q2 (`config.ResolveProgramConfig`'s `ResolvedProgram` struct has no
`Path`/`WorkingDir`/`SessionType` field at all — verified via direct read of
`config/defaults.go:203-227`) already show `program` cannot influence
`resolveSessionType`'s inputs (`msg.SessionType`, `msg.ExistingWorktree`,
`branch` — none of which `ResolveProgramConfig`/`FindProgramConfig` ever
touch) for **any** program value, built-in or custom. Story 1.5.1 below is a
targeted re-verification of that specific claim (cheap, since the trace
already exists) rather than a fresh investigation, plus the observability
addition pre-mortem's Prevention text actually asked for, which Q1/Q2 did not
include.

#### Story 1.5.1: Re-verify `program` cannot influence `resolveSessionType`'s inputs, and add an invariant metric for the unanticipated case

**As a** maintainer relying on Epic 3's guards to close this vector for any
trigger, not just Hypotheses #1/#2, **I want** (a) explicit confirmation that
no code path lets `program`/custom-program resolution change
`req.Msg.SessionType`/`req.Msg.Branch`/`req.Msg.ExistingWorktree` before
`resolveSessionType` reads them, and (b) an observable signal if a
`SESSION_TYPE_NEW_WORKTREE` request ever resolves to a `Directory`-shaped
`Instance` anyway, **so that** an undiscovered third trigger is caught by
monitoring instead of silently sailing through Epic 3's `SessionType`-gated
guards.

**Acceptance Criteria**:
- A one-paragraph note is added to `research/architecture.md` (or this
  story, either is fine) citing `config/defaults.go`'s `ResolveProgramConfig`/
  `FindProgramConfig`/`ResolvedProgram` in full and stating explicitly: no
  field of `ResolvedProgram` is ever assigned to `resolvedPath`,
  `instanceOpts.Path`, `instanceOpts.WorkingDir`, or the local `sessionType`
  variable in `CreateSession` (`server/services/session_service.go`) — cite
  the exact lines checked.
  - *Given* `server/services/session_service.go`'s `CreateSession` function
    body between the `program != ""` custom-program-resolution block
    (`~2521-2531`) and the `instanceOpts := session.InstanceOptions{...}`
    literal (`~2835`), *When* every write to `resolvedPath`/`sessionType` in
    that range is enumerated, *Then* none of them read from
    `resolvedProg`/`ResolvedProgram` — confirming program choice cannot alter
    which `SessionType`/path a `SESSION_TYPE_NEW_WORKTREE` request resolves
    to, for `program="claude"`, a custom program, or any other value.
- A new low-cardinality counter metric (e.g.
  `session_creation_worktree_type_mismatch_total`) increments whenever a
  request with `req.Msg.SessionType == SESSION_TYPE_NEW_WORKTREE` produces an
  `Instance` whose final `i.SessionType != SessionTypeNewWorktree` at the
  point `startLocked` is entered — this should be structurally impossible per
  the AC above and Q1, so a nonzero count in production is itself the signal
  that an unanticipated third trigger exists.
  - *Given* a `CreateSessionRequest` with `SessionType:
    SESSION_TYPE_NEW_WORKTREE`, *When* `startLocked` is entered and
    `i.SessionType` is read, *Then* if it is anything other than
    `SessionTypeNewWorktree`, the counter increments and a Warn-level log line
    fires with the instance's title/UUID — never silently.
**Files**: `research/architecture.md` (note), `session/instance.go`
(`startLocked`, metric emission), `server/services/session_creation_pipeline.go`
or wherever this repo's metrics helper (`RecordSessionCreationMetrics`,
referenced in `research/architecture.md`) is defined.

##### Task 1.5.1a: Add the re-verification note (~3 min)
- Add the paragraph described above to `research/architecture.md`, citing
  `config/defaults.go:203-227` verbatim.
- Files: `project_plans/worktree-envvars-hijack/research/architecture.md`.

##### Task 1.5.1b: Add the invariant metric + Warn log (~5 min)
- In `startLocked` (`session/instance.go`), immediately after
  `hadUUIDBeforeRecovery := i.HasClaudeSession()` (or another point before
  the `firstTimeSetup` branch), compare the *original* requested session type
  (threaded through from `CreateSession` if not already available on
  `Instance`) against `i.SessionType`; on mismatch for the
  `NEW_WORKTREE`-requested case, emit the counter + Warn log.
- Files: `session/instance.go`, metrics helper file (exact path TBD by
  implementer — follow `RecordSessionCreationMetrics`'s existing convention).

---

## Epic 2: Fix the Verified Stale `instanceRootDir` Bug

**Goal**: Independently fix the one bug `research/architecture.md` Q5 already
VERIFIED by direct code read — `instanceRootDir` is captured before
`Start()` runs and never refreshed for a plain (non-GitHub-URL)
`SessionTypeNewWorktree` session, so `InjectHookConfig`/`StartSessionDriver`
operate against the bare repo path instead of the freshly-created worktree.

### Epic 2.1: Re-derive `instanceRootDir` after `Start()` returns

#### Story 2.1.1: Refresh `instanceRootDir` unconditionally post-`Start()`
**As a** session-isolation-conscious maintainer, **I want**
`InjectHookConfig`/`StartSessionDriver` to always receive the instance's
actual post-worktree-creation root directory, **so that** a plain
`SessionTypeNewWorktree` session's hook config never gets written into (and
overwrite) a different, already-live session's `settings.local.json` in the
bare repo directory.

**Acceptance Criteria**:
- `runBackgroundResolutionPipeline` re-derives `instanceRootDir :=
  p.instance.GetEffectiveRootDir()` unconditionally immediately after
  `p.instance.Start(true)` returns successfully — not only inside the
  `deferredGitHubURL` branch (which runs *before* `Start()`, so even its own
  refresh at `session_creation_pipeline.go:198` predates worktree creation).
  - *Given* a `CreateSession` call with `SessionType: SESSION_TYPE_NEW_WORKTREE,
    Path: "/repo", Branch: "feature-x"` and no GitHub URL, *When* the pipeline
    reaches `InjectHookConfig(instanceRootDir, ...)` (`session_creation_pipeline.go:296`),
    *Then* `instanceRootDir` equals the freshly-created worktree path (e.g.
    `/repo-worktrees/feature-x_<timestamp>`), not `/repo`.
**Files**: `server/services/session_creation_pipeline.go`.

##### Task 2.1.1a: Add the post-`Start()` refresh (~3 min)
- In `runBackgroundResolutionPipeline`, immediately after the
  `if startErr := p.instance.Start(true); startErr != nil { ... }` block
  (`session_creation_pipeline.go:258-263`) and before
  `p.instance.SetCreationProgress("")` (`:266`), add:
  `instanceRootDir = p.instance.GetEffectiveRootDir()`.
- Leave the existing `deferredGitHubURL` branch's own refresh at `:198`
  untouched — it becomes vestigial (always overwritten by this later,
  unconditional refresh) but harmless, and removing it is out of scope for
  this bug fix.
- Files: `server/services/session_creation_pipeline.go`.

---

## Epic 3: Close the Silent-Failure / Cross-Attach Vector Defensively

**Goal**: Regardless of which Epic 1 hypothesis (if any) is confirmed, make
every currently-silent "this session did not get the isolated directory it
was supposed to" outcome loud, per requirements.md's Success Metrics ("No
silent worktree-creation skip", "No cross-session attach") and
`research/pitfalls.md`'s "Designed-against checklist".

### Epic 3.1: `resolveSessionType`'s unrecognized-enum-value fallback

#### Story 3.1.1: Return an error instead of silently defaulting to Directory
**As a** caller of `CreateSession`, **I want** an unrecognized
`session_type` enum value to fail the request loudly, **so that** a future
proto-version skew or client bug can never silently downgrade a
worktree-isolation request into an unisolated directory session.

**Acceptance Criteria**:
- `resolveSessionType(msg, branch)` returns `(session.SessionType, error)`;
  its inner `switch`'s `default:` arm (currently `return
  session.SessionTypeDirectory` at `session_service.go:3121-3122`) returns
  `("", fmt.Errorf("unrecognized session_type %v", msg.SessionType))`
  instead.
- `CreateSession`'s one call site (`session_service.go:2537`) propagates a
  non-nil error as `connect.NewError(connect.CodeInvalidArgument, err)`,
  matching the existing validation-error idiom used elsewhere earlier in the
  same function for duplicate-title/bad-`resume_id`/`auto_approve`
  (`session_service.go:2312-2338`).
  - *Given* a `CreateSessionRequest` whose `session_type` field holds an
    enum value this server's proto doesn't recognize (e.g. a future
    `SESSION_TYPE_*` added by a newer client), *When* `CreateSession` is
    called, *Then* the RPC returns `connect.CodeInvalidArgument` with no
    session created — never a silently-downgraded `SessionTypeDirectory`
    session.
  - *Given* a `CreateSessionRequest` with `session_type:
    SESSION_TYPE_NEW_WORKTREE` (a recognized value), *When* `CreateSession`
    is called, *Then* behavior is unchanged — `resolveSessionType` returns
    `(session.SessionTypeNewWorktree, nil)` exactly as before.
**Files**: `server/services/session_service.go`.

##### Task 3.1.1a: Change `resolveSessionType`'s signature and default arm (~4 min)
- Edit `resolveSessionType` (`session_service.go:3108-3132`): change return
  type to `(session.SessionType, error)`; wrap every existing `return
  session.SessionType...` in the explicit-switch branches with a `, nil`;
  change the `default:` arm to return `"", fmt.Errorf(...)`.
- Files: `server/services/session_service.go`.

##### Task 3.1.1b: Update the one call site to propagate the error (~3 min)
- At `session_service.go:2537`, change `sessionType :=
  resolveSessionType(req.Msg, branch)` to `sessionType, sessionTypeErr :=
  resolveSessionType(req.Msg, branch)` followed by `if sessionTypeErr != nil
  { return nil, connect.NewError(connect.CodeInvalidArgument, sessionTypeErr) }`.
- Files: `server/services/session_service.go`.

**PR note** (Adversarial-review Minor 3): this changes behavior for a
*future* client sending a newer proto `session_type` enum value this server
doesn't yet recognize — today silently downgraded to `SessionTypeDirectory`,
after this story it hard-fails the RPC with `CodeInvalidArgument`. This is
the intended fix, but call it out explicitly in the PR description for
anyone running a newer client against an older server during a rolling
deploy.

### Epic 3.2: `setupFirstTimeWorktree`'s unrecognized-type fallback + missing snapshot republish

#### Story 3.2.1: Split `SessionTypeDirectory` out of the catch-all `default:` arm
**As a** session-isolation-conscious maintainer, **I want** an `Instance`
that somehow reaches `setupFirstTimeWorktree` with a `SessionType` outside
the four explicitly-handled values to fail loudly, **so that** the second of
the two "silent-fallback-to-Directory" layers `research/pitfalls.md` §3a
identified is also closed, not just the first (Epic 3.1).

**Acceptance Criteria**:
- `setupFirstTimeWorktree`'s `switch i.SessionType` (`session/instance_worktree.go:60-210`)
  gets an explicit `case SessionTypeDirectory:` carrying today's directory-session
  body (log + `CreateIfMissing` handling + `SetWorktree(nil)`/`Branch = ""`),
  and a separate `default:` arm that returns `fmt.Errorf("setupFirstTimeWorktree:
  unrecognized session type %q for session %q", i.SessionType, i.Title)`.
  - *Given* an `Instance` with `SessionType: SessionTypeDirectory`, *When*
    `setupFirstTimeWorktree` runs, *Then* behavior is byte-for-byte unchanged
    from today (no worktree, `CreateIfMissing` honored, returns `nil`).
  - *Given* an `Instance` constructed with an off-nominal `SessionType`
    value (e.g. `session.SessionType("")` or any string outside the five
    known constants), *When* `setupFirstTimeWorktree` runs, *Then* it
    returns a non-nil error (which `finishFirstTimeSetup`/`Start()`/the
    pipeline already propagate to a `Failed` terminal status today — no
    change needed there) instead of silently proceeding with no worktree.
**Files**: `session/instance_worktree.go`.

##### Task 3.2.1a: Split the `default:` arm (~4 min)
- In `setupFirstTimeWorktree` (`session/instance_worktree.go:194-210`),
  rename the existing `default:` case to `case SessionTypeDirectory:` (drop
  the "and unknown types" half of its comment), and add a new `default:`
  arm below it returning the error described above.
- Files: `session/instance_worktree.go`.

#### Story 3.2.2: Republish the snapshot for every case, not just `SessionTypeNewWorktree`
**As a** concurrent reader of `Instance` state (`GetSession`, the
Workspace-Peers panel, etc.), **I want** every `setupFirstTimeWorktree` case
to publish its `i.gitManager`/`i.Branch` mutation through the same
lock-guarded snapshot republish the `SessionTypeNewWorktree` case already
uses, **so that** no case can leave a stale pre-mutation state observable to
a lock-free reader — closing the second, independent gap
`research/pitfalls.md` §3b identified in the same function
`.claude/rules/instance-lock-free-reads.md` already partially fixed.

**Acceptance Criteria**:
- The `SessionTypeExistingWorktree` (both local and remote sub-branches),
  `SessionTypeNewProject` (both the worktree and no-worktree sub-branches),
  and `SessionTypeDirectory` cases each wrap their `i.gitManager.SetWorktree(...)`/
  `i.Branch = ...` writes in `i.mu.Lock()`/`i.mu.Unlock()`, mirroring the
  existing `SessionTypeNewWorktree` case's pattern
  (`session/instance_worktree.go:81-89`).
- A single, shared `i.mu.Lock(); snap := buildSnapshot(i); i.mu.Unlock();
  i.snapshot.Store(snap)` runs once, after the `switch` block, covering
  every case that doesn't already republish inline — including removing
  `SessionTypeNewProject`'s early `return nil` (`:189`) at its worktree
  sub-branch so both of that case's sub-branches fall through to the shared
  republish instead of one bypassing it.
  - *Given* an `Instance` with `SessionType: SessionTypeDirectory` and a
    concurrent goroutine calling `inst.Snapshot()` immediately after
    `setupFirstTimeWorktree` returns, *When* the read happens, *Then* the
    returned `InstanceSnapshot.Branch` is `""` (the post-mutation value),
    never the pre-call value — confirmed by `go test -race`, which reported
    no race on this path before or after (the write is now lock-guarded) but
    would have shown a stale-read risk in review without the republish.
**Files**: `session/instance_worktree.go`.

##### Task 3.2.2a: Lock-guard the three under-guarded cases' mutations (~5 min)
- Wrap `SessionTypeExistingWorktree`'s local (`:154-155`) and remote
  (`:145-146`, `:133-142`) `i.gitManager.SetWorktree`/`i.Branch` writes, and
  `SessionTypeNewProject`'s two sub-branches (`:186-187`, `:191-192`), each in
  their own `i.mu.Lock()`/`i.mu.Unlock()` pair.
- Files: `session/instance_worktree.go`.

##### Task 3.2.2b: Add the shared post-switch snapshot republish (~4 min)
- Remove `SessionTypeNewProject`'s early `return nil` at `:189`, restructuring
  its worktree/no-worktree sub-branches as an `if`/`else` that both fall
  through to the switch's end. Per Adversarial-review Minor 1: this touches
  a case (`SessionTypeNewProject`) with no direct connection to this bug's
  repro shape — after restructuring, re-read both sub-branches' control flow
  to confirm the only observable behavior change is the snapshot republish
  itself, nothing else.
- After the `switch` block (before the final `return nil` at `:211`), add
  `i.mu.Lock(); snap := buildSnapshot(i); i.mu.Unlock(); i.snapshot.Store(snap)`.
- Leave `SessionTypeNewWorktree`'s existing inline republish (`:81-89`)
  untouched — the resulting double-publish for that one case is a harmless,
  idempotent extra atomic store, not worth the extra diff to dedupe.
- Files: `session/instance_worktree.go`.

### Epic 3.3: Pre-spawn structural guard + cross-session collision guard (relocated into `startLocked`)

**Relocation note (Architecture-review Blocker 1 / Adversarial-review
Blocker 1 remediation):** the original plan placed both of this epic's
guards in `runBackgroundResolutionPipeline`, running only after
`p.instance.Start(true)` returned. Verified by direct read that
`startLocked` (`session/instance.go:1467-1699`, called synchronously by
`Instance.Start()`) already spawns the tmux session and `claude` subprocess
internally — `i.initTmuxSession()` at `:1607`, `i.pm().Start(startPath)` at
`:1654` — before returning. A guard placed after `Start()` returns can only
detect a collision after the rogue process already exists, not prevent it,
which contradicts requirements.md's explicit "must ensure the new-session
code path can never write into an existing session's tmux pane/conversation,
not just detect it after the fact" constraint, and — because the pipeline's
only failure action was a bare `commitTerminalStatus` write with no
`i.Kill()` — would leave a detected collision's tmux session and `claude`
process running indefinitely.

Both stories below are therefore rewritten to run **inside `startLocked`**,
between `basePath = i.gitManager.GetWorktreePath()` resolving
(`session/instance.go:1599`) and `i.initTmuxSession()`
(`session/instance.go:1607`) — i.e. before any tmux/process spawn happens at
all. A failure from either check sets `setupErr` and returns, routing
through `startLocked`'s existing `defer` cleanup (`session/instance.go:1511-1518`),
which already calls `i.Kill()` — reusing that teardown rather than inventing
a new failure path. The pipeline's existing `if startErr :=
p.instance.Start(true); startErr != nil { ... }` branch
(`server/services/session_creation_pipeline.go:258-263`) already routes any
such error to a `Failed` terminal write via the pipeline's one `terminal()`
chokepoint — no new pipeline-level guard/terminal-write call site is needed;
the only pipeline-side change is classifying the returned error into a
specific `metricsOutcome`/`failureReason` (Task 3.3.3a below) instead of the
generic `"StartupError"`, so Story 3.3.1/3.3.2's failures stay distinguishable
in dashboards per the Observability Plan's original intent.

#### Story 3.3.1: Fail loudly, before spawn, if a `SessionTypeNewWorktree` session has no real worktree
**As a** session-isolation-conscious maintainer, **I want** `startLocked`
itself to assert, structurally and BEFORE spawning anything, that a
`SessionTypeNewWorktree` session's resolved worktree path actually differs
from its repo root, **so that** any future silent-worktree-skip bug — even
one Epic 3.1/3.2 don't anticipate — is caught at the one chokepoint every
first-time session-start passes through, per requirements.md's Success
Metric "No silent worktree-creation skip," and is caught early enough to
actually prevent the spawn, not just record that it happened.

**Acceptance Criteria**:
- In `startLocked`'s `firstTimeSetup` branch, immediately after `basePath =
  i.gitManager.GetWorktreePath()` (`session/instance.go:1599`) and before
  `i.initTmuxSession()` (`:1607`): if `i.SessionType ==
  SessionTypeNewWorktree` and `basePath == i.Path` (the structural
  equivalent of `Workspace().ActiveDir == Workspace().RepoRoot`, computed
  from locals already in scope — no need to call `Snapshot()`/`Workspace()`
  from inside the actor command), set `setupErr =
  fmt.Errorf("%w: session %q resolved to no real worktree (basePath == repo
  root %q)", session.ErrWorktreeResolutionFailed, i.Title, i.Path)` and
  `return setupErr` — this fires the existing `defer`'s `i.Kill()` cleanup
  and never reaches `i.initTmuxSession()`/`i.pm().Start()`. **Stable-tag
  note (UX-review gap #6, addressed during plan repair):** the detailed
  `%q`-formatted title/path only ever reaches the log line and the internal
  `error` value — never a user-visible field directly. `Task 3.3.3a` matches
  this error via `errors.Is(startErr, session.ErrWorktreeResolutionFailed)`
  and classifies it to the fixed, short `"WorktreeResolutionFailed"` string
  for the wire-level `failureReason`/`metricsOutcome`, so the value the
  frontend's `FailureReason` switch (Epic 3.6) actually matches on is never
  the raw formatted Go error string.
  - *Given* a session with `SessionType: SessionTypeNewWorktree` whose
    `gitManager` somehow ended up with no worktree set (any future bug this
    plan's Epic 3.1/3.2 didn't anticipate), *When* `startLocked` reaches this
    check, *Then* `Start()` returns a non-nil error wrapping
    `session.ErrWorktreeResolutionFailed`, no tmux session or `claude`
    process is ever spawned for this instance, and (via the pipeline's
    existing `startErr != nil` branch) the session reaches `status: Failed`
    with `failure_reason == "WorktreeResolutionFailed"` (the stable tag, not
    the detailed message) — never `status: Active` with `ActiveDir ==
    RepoRoot`.
  - *Given* a normal, successful `SessionTypeNewWorktree` session, *When*
    `startLocked` reaches this check, *Then* `basePath != i.Path` and
    execution proceeds unchanged to `i.initTmuxSession()`/`i.pm().Start()`/`Active`.
**Files**: `session/instance.go`.

##### Task 3.3.1a: Add the pre-spawn structural assertion (~4 min)
- In `startLocked`'s `firstTimeSetup` branch, immediately after `basePath =
  i.gitManager.GetWorktreePath()` (`session/instance.go:1599`), add the
  check described above, wrapping a new exported sentinel
  `session.ErrWorktreeResolutionFailed = errors.New("resolved to no real
  worktree")` (defined alongside `Instance.SetPreSpawnCollisionGuard` in
  `session/instance.go`) rather than a bare `fmt.Errorf` with no matchable
  sentinel — Task 3.3.3a's classification depends on `errors.Is` finding it.
- Files: `session/instance.go`.

#### Story 3.3.2: Refuse to spawn any worktree-bearing first-time session whose resolved worktree collides with a live sibling
**As a** user with multiple concurrent sessions, **I want** any new
first-time session that resolves a real git worktree — `SessionTypeNewWorktree`
primarily, but also `SessionTypeExistingWorktree` and the worktree
sub-branch of `SessionTypeNewProject`, since the collision risk is identical
regardless of how the worktree was requested (see Risk Control's scope
correction) — to refuse to spawn its tmux session/`claude` process at all
(rather than spawn it and merely mark the RPC `Failed` afterward) if its
resolved worktree path is already another live session's active directory,
**so that** requirements.md's "No cross-session attach" guarantee holds
structurally and *before the fact*, not just by construction after — closing
`research/pitfalls.md` §3c/checklist-item-4 the way its own wording requires.

**Acceptance Criteria**:
- New `Instance` field + setter, mirroring the existing `wireCallbacks`
  pre-`Start()` callback-injection precedent (`server/services/session_service.go:1831`):
  `Instance.SetPreSpawnCollisionGuard(guard func(worktreePath string) error)`,
  storing `guard` on the instance (no lock needed — set once, before `Start()`
  is ever called, same lifecycle as `wireCallbacks`'s other injections
  (`SetTaggingEngine`, `SetMCPServerURLProvider`, etc.) and `SetOnExitCallback`
  — not `SetStatusManager`, which is deliberately wired *after* `Start()`
  everywhere it's called; see the Pattern Decisions table's Cross-session
  guard row).
- The guard call added below is unconditional on `i.SessionType` — it is
  gated only by Story 3.3.1's `i.gitManager.HasWorktree()` check, so it
  applies to `SessionTypeNewWorktree`, `SessionTypeExistingWorktree`, and the
  worktree sub-branch of `SessionTypeNewProject` alike, and is structurally
  unreachable for `SessionTypeDirectory` (which never has a worktree). This
  is intentional scope, not an oversight — see Risk Control.
- In `startLocked`'s `firstTimeSetup` branch, immediately after Story 3.3.1's
  structural check passes (so `basePath` is confirmed a real,
  distinct-from-`i.Path` worktree path) and still before `i.initTmuxSession()`:
  if `i.preSpawnCollisionGuard != nil`, call `i.preSpawnCollisionGuard(basePath)`;
  on a non-nil error, set `setupErr = fmt.Errorf("worktree collision check
  failed: %w", err)` and `return setupErr` — same `i.Kill()`-cleanup path as
  Story 3.3.1.
- `SessionService.wireCallbacks` (`server/services/session_service.go:1831`,
  called from every `CreateSession`/`CreateDirectorySession`/`CreateWorktreeSession`
  path and from the pipeline at `session_creation_pipeline.go:252` — always
  BEFORE `p.instance.Start(true)` at `:258`) additionally calls
  `inst.SetPreSpawnCollisionGuard(func(worktreePath string) error {
  blockingUUID, blocked := s.OtherLiveSessionInsideWorktree(inst.UUID, worktreePath);
  if !blocked { return nil }; return fmt.Errorf("%w: blocked by session %s",
  session.ErrDirectoryCollision, blockingUUID) })`. **Stable-tag fix
  (UX-review gap #5, addressed during plan repair):** the original draft of
  this closure returned `fmt.Errorf("DirectoryCollision: worktree already in
  use by session %s", blockingUUID)` — a plain, unwrapped string with the
  blocking session's raw UUID interpolated directly into it. If that
  formatted string became the wire-level `failureReason` value verbatim (as
  Task 3.3.3a's original wording — `"DirectoryCollision: ..."` — implied),
  two problems follow: (1) the frontend's `FailureReason` switch (Epic 3.6)
  can only match a fixed, short string, never a value with an embedded UUID
  suffix, so it would always fall through to the generic default case; (2)
  an internal session UUID would leak into a user-facing field. Fixed by
  wrapping a new exported sentinel `session.ErrDirectoryCollision =
  errors.New("worktree already in use by another live session")` (defined
  in `session/instance.go`): the blocking UUID stays only in the wrapped
  error's `.Error()` text (used for the log line), while Task 3.3.3a
  classifies via `errors.Is(startErr, session.ErrDirectoryCollision)` to the
  fixed, short `"DirectoryCollision"` string for the actual
  `failureReason`/`metricsOutcome` value. If `s.reviewQueuePoller == nil`
  when this closure runs (mirrors `OtherLiveSessionInsideWorktree`'s own
  silent-`false` escape hatch — see Tech Debt Disposition), `log.Warn`
  before returning `nil`, so a misconfigured/degenerate `SessionService` is
  visible in logs instead of silently permissive.
  - *Given* a live, backend-process-alive session A whose real tmux pane cwd
    is `/repo-worktrees/feature-x_123`, *When* a new `CreateSession` call
    with `SessionType: SESSION_TYPE_NEW_WORKTREE` somehow resolves its own
    worktree path to that exact same path (any future bug — the self-heal
    gap in Hypothesis #2, a timestamp-collision edge case, or anything
    else), *Then* `startLocked` never calls `i.initTmuxSession()`/`i.pm().Start()`
    for the new session — no tmux pane or `claude` process is ever created
    for it — `Start()` returns an error wrapping `session.ErrDirectoryCollision`,
    and (via the pipeline's existing `startErr != nil` branch) the session
    reaches `status: Failed` with `failure_reason == "DirectoryCollision"`
    (the stable tag, containing no UUID) while session A's own tmux
    pane/process/status are completely untouched.
  - *Given* two independent `SessionTypeNewWorktree` sessions with genuinely
    distinct, freshly-created worktree paths (the normal case), *When* both
    are created, *Then* neither is blocked by the other.
- **TOCTOU race between the collision check and spawn (Engineering-review
  gap #1 / pre-mortem Failure #2, P2 — addressed during plan repair).**
  `startLocked`'s call to `i.preSpawnCollisionGuard(basePath)` above (via
  `OtherLiveSessionInsideWorktree`) only detects a collision against a
  sibling session whose backend process is *already* alive
  (`inst.IsBackendProcessAlive()`). Two near-simultaneous `CreateSession`
  calls for the same `SessionTypeExistingWorktree` path (not
  timestamp-suffixed, unlike `SessionTypeNewWorktree` — see the Domain
  Glossary) run on two independent `*Instance` actors with no shared lock
  between them: verified during plan repair that (a) each `Instance` has its
  own actor/mailbox (`liveInstance`, `session/instance.go:631-633`) that
  only serializes commands against *that* instance, not across instances,
  and (b) the only cross-`CreateSession` mutex in this codebase,
  `BatchCreateSessions`'s per-repo `repoMutexes` (`server/services/session_service.go:5459-5476`),
  is scoped to the batch-creation RPC only and is never consulted by the
  single-session `CreateSession` path both guards above run under. So both
  calls' `startLocked` invocations can reach the guard check concurrently,
  each observe zero other live sessions yet (neither has spawned), both
  pass, and both proceed to spawn into the identical directory — this is a
  real, not merely theoretical, gap; option (b) ("negligible/already closed,
  document why") does not hold, so this is fixed per option (a): a real
  per-path claim.
  - New `SessionService` field `inFlightWorktreeSpawns sync.Map` (canonicalized
    absolute worktree path → claiming session UUID), guarding the
    check-through-spawn window itself rather than relying solely on the
    post-hoc liveness scan above.
  - New, additive `Instance` callback (does not replace
    `SetPreSpawnCollisionGuard` — a second, independent guard call), mirroring
    the same pre-`Start()` callback-injection shape:
    `Instance.SetWorktreeSpawnReservation(claim func(worktreePath string)
    (release func(), err error))`.
  - In `startLocked`, call the reservation claim FIRST — immediately after
    Story 3.3.1's structural check passes and before the existing
    `i.preSpawnCollisionGuard` call — so the atomic claim closes the race
    window before the (still useful, for an already-settled sibling)
    liveness-based check even runs. On success, `defer release()` so the
    reservation is released whether `startLocked` returns via an error below
    (case: `i.preSpawnCollisionGuard` itself still finds a collision) or via
    the normal `Active` transition at the end of the function — the
    reservation only needs to outlive the check-through-spawn window, not
    the process's whole lifetime, since `IsBackendProcessAlive()`-based
    detection takes over for any future `CreateSession` call once this one's
    tmux/process is actually up.
  - `wireCallbacks` wires the claim as:
    `inst.SetWorktreeSpawnReservation(func(worktreePath string) (func(), error) {
    cleanTarget, err := filepath.Abs(worktreePath); if err != nil { return nil, nil };
    cleanTarget = git.CanonicalizeWorktreePath(cleanTarget);
    if _, loaded := s.inFlightWorktreeSpawns.LoadOrStore(cleanTarget, inst.UUID); loaded {
    return nil, fmt.Errorf("%w: claimed by an in-flight spawn", session.ErrDirectoryCollision) };
    return func() { s.inFlightWorktreeSpawns.Delete(cleanTarget) }, nil })`.
    A resolve error (`err != nil` from `filepath.Abs`) falls through to `nil,
    nil` (no reservation, no error) rather than blocking the session on an
    unrelated path-resolution failure — the existing liveness-based guard
    and Story 3.3.1's structural check still run regardless.
  - *Given* two goroutines calling `CreateSession` with `SessionType:
    SESSION_TYPE_EXISTING_WORKTREE` against the identical worktree path at
    the same time, *When* both reach `startLocked`'s reservation-claim call,
    *Then* exactly one `LoadOrStore` succeeds (returns `loaded == false`) and
    proceeds to spawn; the other observes `loaded == true`, returns an error
    wrapping `session.ErrDirectoryCollision` before calling
    `i.initTmuxSession()`/`i.pm().Start()`, and reaches `status: Failed` with
    `failure_reason == "DirectoryCollision"` — never two live tmux
    panes/`claude` processes in the same directory.
**Files**: `session/instance.go` (new fields/setters, `startLocked` call
sites, `ErrDirectoryCollision`/`ErrWorktreeResolutionFailed` sentinels),
`server/services/session_service.go` (`wireCallbacks`,
`inFlightWorktreeSpawns` field).

##### Task 3.3.2a: Add `Instance.SetPreSpawnCollisionGuard` and the `startLocked` call site (~5 min)
- Add the field, setter, and `startLocked` call described above, plus the
  `session.ErrDirectoryCollision` sentinel used by both this closure and
  Task 3.3.2c below.
- Files: `session/instance.go`.

##### Task 3.3.2b: Wire the guard in `wireCallbacks` (~4 min)
- Add the `inst.SetPreSpawnCollisionGuard(...)` call described above to
  `wireCallbacks` (`server/services/session_service.go:1831`), alongside the
  existing `wireRateLimitCallbacks`/tagging-engine wiring already in that
  function (not `SetStatusManager`, which `wireCallbacks` never calls — see
  the Pattern Decisions table's Cross-session guard row), including the
  `reviewQueuePoller == nil` log-and-allow fallback.
- Files: `server/services/session_service.go`.

##### Task 3.3.2c: Close the TOCTOU race with a per-path spawn reservation (~8 min)
- Add `SessionService.inFlightWorktreeSpawns sync.Map`,
  `Instance.SetWorktreeSpawnReservation`, and the `startLocked` claim
  call (ordered before the existing `i.preSpawnCollisionGuard` call)
  described above.
- Add the `wireCallbacks` wiring for the claim closure.
- Files: `session/instance.go`, `server/services/session_service.go`.

##### Task 3.3.3a: Classify `Start()`'s returned error into a specific pipeline `metricsOutcome`/`failureReason` (~4 min)
- At the pipeline's existing failure branch
  (`server/services/session_creation_pipeline.go:258-263`), before calling
  `terminal(pipelineOutcome{session.Failed, "StartupError",
  SessionCreationOutcomeFailed})`, use `errors.Is(startErr,
  session.ErrWorktreeResolutionFailed)` / `errors.Is(startErr,
  session.ErrDirectoryCollision)` — sentinel matching, not string
  parsing/prefixing — to select the fixed, short `"WorktreeResolutionFailed"`
  (Story 3.3.1) or `"DirectoryCollision"` (Story 3.3.2/3.3.2c) string
  instead of the generic `"StartupError"`. **Never interpolate the detailed
  error text (title/path/blocking-UUID) into the `failureReason`/`metricsOutcome`
  value itself** — those stay fixed, short, stable tags so (a) they match
  the frontend's `FailureReason` switch (Epic 3.6) and (b) no internal UUID
  or path leaks into a user-facing field; the detailed `startErr.Error()`
  text is still logged in full at this same call site, unchanged, for
  debugging.
- Files: `server/services/session_creation_pipeline.go`.

### Epic 3.4 (CONDITIONAL — gated on Story 1.1's verdict): Harden the main-checkout self-heal blindness

**Goal**: Only executed if Story 1.1.1's test CONFIRMS Hypothesis #2 (a
main-checkout branch collision silently succeeds rather than erroring). If
Story 1.1.1 REFUTES it (the collision fails loudly as
`research/architecture.md` predicts), this epic is dropped entirely — no code
change needed.

**Placeholder acceptance criteria** (to be made concrete once confirmed):
`findLiveWorktreeForBranch`/`nativeFindLiveWorktreeForBranch`
(`session/git/worktree_ops.go:356-397`) must either become aware of the main
checkout (so it can self-heal that case too) or the failure path around it
must be fixed to always propagate loudly if self-heal genuinely cannot apply
— whichever Story 1.1.1's concrete failure mode indicates.

### Epic 3.5 (CONDITIONAL — gated on Story 1.2's verdict): Fix confirmed client-side request divergence

**Goal**: Only executed if Story 1.2.1 CONFIRMS Hypothesis #1 (envVars
populated in the web-app/MCP client-side form drops/alters `session_type` or
`branch`). If REFUTED, this epic is dropped.

**Placeholder acceptance criteria** (to be made concrete once confirmed and
file:line'd by Story 1.2.1): the client-side request-construction bug found
must be fixed at its source; likely scoped as a follow-up `sdd:fix-bug` cycle
given it lives outside this plan's primary Go backend file set.

### Epic 3.6 (NEW — UX-review gap #4, unconditional): Wire the new `FailureReason` values into the frontend

**Goal**: Epic 3.3 introduces two new server-side failure classifications —
`"WorktreeResolutionFailed"` (Story 3.3.1) and `"DirectoryCollision"` (Story
3.3.2/3.3.2c, now emitted as a fixed stable tag per the gap #5/#6 fixes
above) — but nothing in the original plan updated the frontend's two
`FailureReason` switches to recognize them. Verified during plan repair by
reading both files: today, any unrecognized `failureReason` string falls
through to each switch's generic `default:` case
(`web-app/src/lib/utils/sessionFailure.ts:32-33`'s `"Session creation
failed."` and `web-app/src/lib/contexts/NotificationContext.tsx:45-46`'s
same generic string) — so without this epic, a user who hits either new
failure mode sees the same undifferentiated message the pre-Epic-3.3
generic `"StartupError"` already produced, even though the server now knows
precisely which severe isolation-safety refusal occurred.

#### Story 3.6.1: Add explicit cases for `WorktreeResolutionFailed`/`DirectoryCollision` to both frontend switches
**As a** user whose session creation was refused by the new worktree-safety
guards, **I want** a specific, actionable message instead of the generic
"Session creation failed," **so that** I understand my session was
deliberately blocked to protect isolation, not just failed for an unknown
reason.

**Acceptance Criteria**:
- `web-app/src/lib/utils/sessionFailure.ts`'s `FailureReason` union type
  (`:11-15`) gains two new members: `"WorktreeResolutionFailed"` and
  `"DirectoryCollision"`.
- `getFailureMessage` (`:24-35`) gains two new `case` arms, inserted before
  the existing `default:` (`:32`):
  - `case "WorktreeResolutionFailed": return "Couldn't create an isolated
    worktree — refusing to start.";`
  - `case "DirectoryCollision": return "Refused: another active session is
    already using this directory.";`
- `web-app/src/lib/contexts/NotificationContext.tsx`'s
  `getFailureReasonToastMessage` (`:35-48`) gains matching cases, inserted
  before its own `default:` (`:45`), with toast-appropriate conversational
  wording distinct from the card copy above (per this function's own
  existing doc comment, `:27-33`, which requires the two contexts to differ
  in wording while covering the same reason set) — e.g.:
  - `case "WorktreeResolutionFailed": return "Couldn't set up an isolated
    workspace for this session. It was not started.";`
  - `case "DirectoryCollision": return "Blocked: another session is already
    active in that directory.";`
  - *Given* a `Session` with `failure_reason: "DirectoryCollision"`, *When*
    `SessionCard`/`SessionRow` render its persistent Failed-state copy via
    `getFailureMessage`, *Then* the user sees "Refused: another active
    session is already using this directory." — not "Session creation
    failed."
  - *Given* the same session transitions to `SESSION_STATUS_FAILED` and a
    toast fires, *When* `getFailureReasonToastMessage` runs, *Then* the
    toast shows "Blocked: another session is already active in that
    directory." — the toast-specific wording, not the card copy verbatim.
- This story depends on Epic 3.3's failure values actually being the fixed,
  short strings `"WorktreeResolutionFailed"`/`"DirectoryCollision"` on the
  wire (gap #5/#6 fixes above) — a raw, UUID-interpolated string would never
  match either switch's `case` arm.
**Files**: `web-app/src/lib/utils/sessionFailure.ts`,
`web-app/src/lib/contexts/NotificationContext.tsx`.

##### Task 3.6.1a: Add the two new cases to both switches (~5 min)
- Add the `FailureReason` union members and the four new `case` arms (two
  per file) described above.
- Files: `web-app/src/lib/utils/sessionFailure.ts`,
  `web-app/src/lib/contexts/NotificationContext.tsx`.

---

## Epic 4: Regression Tests

**Goal**: Per `research/build-vs-buy.md`'s recommendation, add integration
tests at the `server/services` level (not E2E — the bug is async
server-side state with no UI decision point) proving Epics 2 and 3 actually
close the vectors they claim to. Per Architecture-review Concern 1 and
Adversarial-review Concern 1, this epic must also actually drive Story
3.3.1/3.3.2's new pre-spawn guards into their failure branches (Epic 4.4,
4.5) — the original plan's Epic 4.1 alone never did, since a normal new-
worktree session's path is structurally guaranteed not to collide (its
timestamp-suffixed naming, `research/architecture.md`'s Q3 finding), so it
never exercised the guards' `blocked=true`/structural-failure paths. Per
Adversarial-review Concern 2, Story 4.1.1 is also tightened to assert
conversation-content isolation (`claudeSessionID`/`HistoryFilePath`
disjointness), not just directory-path isolation, matching requirements.md's
literal Success Metric wording.

### Epic 4.1: `ModeIsNewWorktree` end-to-end coverage

#### Story 4.1.1: Add the missing `SESSION_TYPE_NEW_WORKTREE` + `EnvVars` + collision-sim subtest
**As a** maintainer, **I want** the existing pipeline-completion test suite
to cover the exact combination the original bug report used (`SESSION_TYPE_NEW_WORKTREE`
+ non-empty `EnvVars` + `Program: "claude"`, against a repo already hosting a
live session), **so that** this specific regression can never silently
reappear.

**Acceptance Criteria**:
- A new `ModeIsNewWorktree` subtest is added to
  `TestCreateSession_should_ReachActiveViaPipeline`
  (`server/services/session_service_test.go:4277`), following the existing
  `ModeIsRestart` subtest's pattern for seeding a pre-existing live instance
  (`fix.storage.AddInstance` + real tmux via `fix.svc.testTmuxServerSocket`)
  and `initGitRepoWithCommit` for a real git repo fixture.
  - *Given* a real git repo at `repoDir` (via `initGitRepoWithCommit`) with a
    pre-existing live `SessionTypeDirectory` instance already running there
    **and seeded with a real, fake conversation JSONL** (known UUID `preExistingUUID`,
    placed under `~/.claude/projects/<encoded(repoDir)>/` per
    `TestHistoryFileDetector_DetectByPath_*`'s fixture pattern in
    `session/history_detector_test.go`), *When* `fix.svc.CreateSession` is
    called with `Path: repoDir, Program: "claude", SessionType:
    SESSION_TYPE_NEW_WORKTREE, Branch: "epic61-new-worktree", EnvVars:
    {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}`, *Then*
    `assertReachesActiveViaPipeline` observes `status: Active`, the resulting
    instance's `Workspace().ActiveDir != repoDir` (a real, distinct worktree
    path), `inst.EnvVars["ANTHROPIC_BASE_URL"] == "http://127.0.0.1:47000"`,
    **and the new instance's `claudeSession.ConversationUUID` (if any is set
    at all) is never `preExistingUUID`, and its `HistoryFilePath` (if any) is
    never the pre-existing session's JSONL path** — the conversation-content-
    isolation half of requirements.md's Success Metric ("asserts the new
    session's pane/conversation content is disjoint from the first"), not
    just directory-path isolation. Real PTY/pane-content capture is not
    attempted here (would be non-deterministic per
    `deterministic-fast-tests`); asserting on `ConversationUUID`/
    `HistoryFilePath` is the deterministic proxy for pane content, since
    `buildLaunchCommand`'s `--resume claudeSessionID` and tmux's own attach
    target are exactly what those two fields drive.
**Files**: `server/services/session_service_test.go`.

##### Task 4.1.1a: Write the `ModeIsNewWorktree` subtest (~5 min)
- Add the subtest inside `TestCreateSession_should_ReachActiveViaPipeline`,
  reusing `initGitRepoWithCommit(t, repoDir)` for the repo and the
  `ModeIsRestart` subtest's `fix.storage.AddInstance(&session.Instance{...})`
  pattern to seed the pre-existing live session, plus a fake JSONL fixture
  for that pre-existing session per the Given above.
- Assert per the Given-When-Then above; also assert the pre-existing
  session's own `ActiveDir`/status/`ConversationUUID` are unaffected (still
  `Active`, still its own path, still `preExistingUUID`) — proving no
  cross-attach occurred in either direction.
- Files: `server/services/session_service_test.go`.

### Epic 4.2: Regression test for Epic 2's stale-`instanceRootDir` fix

#### Story 4.2.1: Assert `InjectHookConfig` targets the worktree, not the bare repo
**As a** maintainer, **I want** a test proving the Claude Code hook config
gets written into the session's actual worktree directory, **so that** Epic
2's fix can never silently regress back to writing into (and clobbering) a
sibling session's hook config in the bare repo.

**Acceptance Criteria**:
- A new test (in `server/services/session_creation_pipeline_test.go`,
  following `TestBackgroundResolutionPipeline_should_SkipTerminalWrite_When_EpochAlreadyBumped`'s
  pattern of calling `fix.svc.runBackgroundResolutionPipeline` directly)
  creates a real `SessionTypeNewWorktree` instance via
  `session.CreateManagedInstance`, runs the real pipeline, and asserts
  `.claude/settings.local.json` exists under the resolved worktree
  directory and does NOT exist under the bare repo directory (unless it
  already existed there from an unrelated fixture step).
  - *Given* a `SessionTypeNewWorktree` instance created against real repo
    `repoDir` with branch `"feature-hook-test"`, *When*
    `runBackgroundResolutionPipeline` completes successfully, *Then*
    `filepath.Join(inst.Workspace().WorktreeDir, ".claude",
    "settings.local.json")` exists, and
    `filepath.Join(repoDir, ".claude", "settings.local.json")` does not.
**Files**: `server/services/session_creation_pipeline_test.go`.

##### Task 4.2.1a: Write the hook-config-location regression test (~5 min)
- Add the test described above.
- Files: `server/services/session_creation_pipeline_test.go`.

### Epic 4.3: Regression test for Epic 3.1/3.2's loud-failure fix

#### Story 4.3.1: Assert an unrecognized `SessionType` fails loudly, not silently
**As a** maintainer, **I want** a test proving an off-nominal `SessionType`
value produces a hard error from both `resolveSessionType` and
`setupFirstTimeWorktree`, **so that** Epic 3's fix for the "two stacked
silent-fallback-to-Directory layers" (`research/pitfalls.md` §3a) can never
silently regress.

**Acceptance Criteria**:
- A unit test for `resolveSessionType` asserts a non-nil error for an
  out-of-range `sessionv1.SessionType` value (e.g. `sessionv1.SessionType(99)`),
  alongside the existing `TestResolveSessionType_*` tests in
  `session_service_create_test.go`.
  - *Given* `msg.SessionType == sessionv1.SessionType(99)` (an unrecognized
    value), *When* `resolveSessionType(msg, "")` is called, *Then* it
    returns a non-nil error and the zero-value `SessionType`.
- A unit test for `setupFirstTimeWorktree` asserts a non-nil error for an
  `Instance` with `SessionType: session.SessionType("bogus")`.
  - *Given* `inst.SessionType == session.SessionType("bogus")`, *When*
    `inst.setupFirstTimeWorktree()` is called, *Then* it returns a non-nil
    error mentioning `"bogus"`.
**Files**: `server/services/session_service_create_test.go`,
`session/instance_worktree_test.go`.

##### Task 4.3.1a: Add `resolveSessionType`'s unrecognized-value unit test (~3 min)
- Add alongside `TestResolveSessionType_ExplicitNewWorktree` et al. in
  `session_service_create_test.go`.
- Files: `server/services/session_service_create_test.go`.

##### Task 4.3.1b: Add `setupFirstTimeWorktree`'s unrecognized-value unit test (~3 min)
- Add to `session/instance_worktree_test.go`, near the existing
  `TestWorkspace_*` tests.
- Files: `session/instance_worktree_test.go`.

### Epic 4.4 (NEW): Session-package unit test proving Story 3.3.1/3.3.2's guards actually prevent spawn

**Goal**: Close Architecture-review Concern 1 / Adversarial-review Concern 1
— the original plan's only regression coverage (Epic 4.1) never drove the
new guards into their failure branches, because a normal worktree path is
structurally guaranteed not to collide (timestamp suffix) and the guards'
own precondition (a broken worktree resolution) isn't naturally reachable
through the public `CreateSession` API. Test the guards directly at the
`session` package level instead, where they're now directly injectable
(`SetPreSpawnCollisionGuard`) — deterministic, no fault-injection-of-a-real-
git-worktree-collision needed.

#### Story 4.4.1: Unit-test `startLocked`'s pre-spawn guards in isolation
**As a** maintainer, **I want** a test that proves
`Instance.SetPreSpawnCollisionGuard`'s error return actually prevents
`i.initTmuxSession()`/`i.pm().Start()` from ever running, **so that** Story
3.3.2's "refuse to spawn, not just detect" guarantee has direct test
coverage of the exact mechanism it depends on, independent of whether a real
collision is reachable through the full `CreateSession` path.

**Acceptance Criteria**:
- A new test in `session/instance_test.go` (or nearest existing
  `TestInstance_Start*`/`startLocked`-adjacent test file) constructs a real
  `Instance` and forces `gitManager`'s `HasWorktree()`/`GetWorktreePath()`
  result via `inst.SetGitWorktree(git.NewGitWorktreeFromStorage(repoPath,
  worktreePath, title, branch, sha))` (`session/instance_worktree.go:540`,
  `session/git/worktree.go:193`) — the same exported, no-I/O seam
  `server/services/session_service_worktree_guard_test.go`'s
  `newWorktreeGuardTarget` already uses for an equivalent collision test —
  **not** a `GitManager`-interface test double (that interface exists at
  `session/git_worktree_manager.go:511` but `Instance.gitManager`'s actual
  field type is the concrete `GitWorktreeManager` struct, which a value
  merely satisfying the interface cannot be assigned to; see Domain
  Glossary). For this collision-guard case, use `SessionType:
  SessionTypeExistingWorktree`: `startLocked`'s firstTimeSetup branch only
  calls the real, I/O-performing `gitManager.Setup()` when `SessionType !=
  SessionTypeExistingWorktree`, so `ExistingWorktree` lets the forced
  `GetWorktreePath()` value reach the guard check without a real git repo.
  Then call `inst.SetPreSpawnCollisionGuard(func(string) error { return
  errors.New("forced collision") })` before `inst.Start(true)`.
  - *Given* the guard above, *When* `inst.Start(true)` is called, *Then* it
    returns a non-nil error wrapping `"forced collision"`, `inst.pm().IsAlive()`
    is `false` (no tmux/process was ever spawned), and `inst.Status` never
    transitions to `Active`.
- A second case in the same test proves Story 3.3.1's structural check
  independently, using `SessionType: SessionTypeNewWorktree` (the only type
  that check applies to) against a real repo fixture
  (`initGitRepoWithCommit`, as Epic 4.1 already uses) — `SessionTypeNewWorktree`
  routes through the real `gitManager.Setup()` call before the structural
  comparison runs, so this case needs a real repo, unlike the
  `SessionTypeExistingWorktree` case above. `inst.Start(true)` returns a
  non-nil error mentioning "resolved to no real worktree", and no process is
  spawned.
**Files**: `session/instance_test.go` (or equivalent).

##### Task 4.4.1a: Write the guard-prevention unit test (~8 min)
- Add the two cases described above: the `SessionTypeExistingWorktree` +
  `SetGitWorktree`-forced-path case for Story 3.3.2's collision guard (no
  real repo needed), and the `SessionTypeNewWorktree` + real-repo-fixture
  case for Story 3.3.1's structural check.
- Files: `session/instance_test.go`.

### Epic 4.5 (NEW): server/services integration test proving `wireCallbacks` actually wires the guard end-to-end

**Goal**: Epic 4.4 proves the guard mechanism works in isolation; this epic
proves `SessionService.wireCallbacks` actually wires it before every
`Start()` call in production, so a future refactor that reorders or drops
the `wireCallbacks` call relative to `Start()` (the exact class of ordering
bug Architecture-review Concern 2 flagged for the original, now-superseded
pipeline placement) would be caught by this suite, not just by reading the
code.

#### Story 4.5.1: End-to-end collision test via `SetGitWorktree`
**As a** maintainer, **I want** a `server/services`-level test that forces a
genuine worktree-path collision through the real `wireCallbacks` →
`Instance.Start()` path, **so that** the wiring itself (not just the guard's
own logic) is regression-tested.

**Acceptance Criteria**:
- A new test in `server/services/session_creation_pipeline_test.go` seeds a
  live session A with a known, fixed `ActiveDir` (via
  `fix.storage.AddInstance` + real tmux, as `ModeIsRestart`/Story 4.1.1
  already do), then constructs a new `SessionTypeExistingWorktree` instance
  (mirroring Epic 4.2's `session.CreateManagedInstance` +
  `fix.svc.runBackgroundResolutionPipeline` pattern) and, before running the
  pipeline, calls `inst.SetGitWorktree(git.NewGitWorktreeFromStorage(repoPath,
  sessionA.Workspace().ActiveDir, title, branch, sha))`
  (`session/instance_worktree.go:540`, `session/git/worktree.go:193` — the
  same exported seam `server/services/session_service_worktree_guard_test.go`'s
  `newWorktreeGuardTarget` already uses, **not** the nonexistent
  `GitManager`-interface double an earlier plan draft described here; see
  Epic 4.4's same correction and the Domain Glossary). `SessionTypeExistingWorktree`
  is deliberately used instead of `SessionTypeNewWorktree`: `startLocked`'s
  real, I/O-performing `gitManager.Setup()` call only runs when `SessionType
  != SessionTypeExistingWorktree`, so this choice removes the real
  git-worktree naming's timestamp uniqueness for this one test — per
  Adversarial-review Concern 1's own suggested seam — without needing a
  second real git repo, and (per the Risk Control scope correction) the
  collision guard applies to `SessionTypeExistingWorktree` exactly as it does
  to `SessionTypeNewWorktree`, so this remains a faithful test of the same
  guard/wiring.
  - *Given* the setup above, *When* `fix.svc.runBackgroundResolutionPipeline`
    runs for the new instance, *Then* the new instance's `Start()` fails with
    a `DirectoryCollision` error, the pipeline's existing `startErr != nil`
    branch commits `status: Failed`, no tmux session/process was ever
    created for the new instance, and session A's own status/`ActiveDir`/tmux
    pane are completely unaffected.
**Files**: `server/services/session_creation_pipeline_test.go`.

##### Task 4.5.1a: Write the end-to-end wiring regression test (~8 min)
- Add the test described above.
- Files: `server/services/session_creation_pipeline_test.go`.

### Epic 4.6 (NEW — Engineering-review gap #1 / pre-mortem Failure #2, P2): Concurrency regression test for the TOCTOU race fix

**Goal**: Epic 4.4/4.5 prove the (liveness-based) collision guard works and
is wired correctly, but neither drives two genuinely concurrent
`CreateSession` calls at the same path — the exact shape of Task 3.3.2c's
TOCTOU fix. This epic adds that missing concurrency coverage directly, per
pre-mortem Failure #2's own suggested test shape.

#### Story 4.6.1: Two simultaneous `CreateSession` calls for the same `SessionTypeExistingWorktree` path — only one reaches `Active`
**As a** maintainer, **I want** a test that fires two goroutines calling
`CreateSession` for the identical worktree path at the same time, **so
that** Task 3.3.2c's per-path reservation (`inFlightWorktreeSpawns`) is
proven to close the race, not just argued to close it from a static read.

**Acceptance Criteria**:
- A new test in `server/services/session_creation_pipeline_test.go` (or
  `session_service_test.go`, whichever the implementer judges the better
  fit given the existing `ModeIsNewWorktree`/`ModeIsRestart` patterns) seeds
  a real repo (`initGitRepoWithCommit`) with a pre-created worktree
  directory (so both goroutines can target `SessionType:
  SESSION_TYPE_EXISTING_WORKTREE` against the identical, non-timestamped
  path — the scenario Story 3.3.2's own AC names as most reachable), then
  launches two goroutines that both call `fix.svc.CreateSession` (or
  `runBackgroundResolutionPipeline` directly, mirroring Epic 4.5's pattern)
  for that same path simultaneously (e.g. released by a shared start
  channel/`sync.WaitGroup` to maximize the chance both reach the guard
  check concurrently), then waits for both to reach a terminal status.
  - *Given* the setup above, *When* both `CreateSession` calls run
    concurrently, *Then* exactly one of the two resulting sessions reaches
    `status: Active`, and the other reaches `status: Failed` with
    `failure_reason == "DirectoryCollision"` — never both `Active` (which
    would mean two live tmux panes/`claude` processes sharing one worktree,
    the exact bug this epic exists to close), and never both `Failed` (which
    would mean the reservation leaked and blocked a legitimate solo
    creation).
  - Run under `go test -race` (already this repo's default per
    `deterministic-fast-tests`) to additionally confirm the new
    `inFlightWorktreeSpawns sync.Map` claim/release has no data race.
**Files**: `server/services/session_creation_pipeline_test.go` (or
`session_service_test.go`).

##### Task 4.6.1a: Write the concurrent-collision regression test (~8 min)
- Add the test described above.
- Files: `server/services/session_creation_pipeline_test.go` (or
  `session_service_test.go`).
