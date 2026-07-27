# Backlog Feature Improvement — Audit Findings (2026-07-14)

Produced by the `backlog-feature-improvement` skill. Goal: an item goes idea → shipped PR
with minimal human intervention, and pipeline stages are configurable per item (e.g. "use
`/sdd:full` for this one"). This audit combines a live item-state check, a UI walkthrough,
and four quality-skill passes (`quality:architecture-review`, `ux:review`, `code:review`,
`code:is-it-ready`) scoped to the backlog automation code.

Findings are bucketed: **[1] reconciliation bugs**, **[2] manual gates that could be
policy-driven**, **[3] hardcoded pipeline steps that need a configurability seam**.

## Update — 2026-07-17 (night): PR #168's abandon-review respawn verified live, with one caveat

Deployed PR #168 (`make install-service`) and watched the live 60s reconciliation sweep
(`grep "BacklogLifecycle" ~/.stapler-squad/logs/staplersquad.log`) against the 2 remaining
real stuck items (`6f22bace`, `9264efe7`). They did NOT get auto-respawned, and the reason
is a real, worth-documenting edge case in `markAbandonedReview` (`session/backlog_lifecycle.go:1043`):
the respawn is gated on the SAME notify-once flag as the notification itself
(`row.NotifiedAt != nil` short-circuits before reaching the respawn dispatch). Both items'
stuck rows were already notified *days* before this fix deployed (under the old
notify-only code), so `NotifiedAt` was already non-nil — the new respawn code only fires
for rows notified *after* deploy, not retroactively for rows already in the notified state.

**This is correct behavior for all future occurrences** (any abandoned_review row created
from now on gets exactly one respawn attempt, per the fix's documented intent) — it just
doesn't help pre-existing stuck items from before the deploy. Those need one manual
`ResolveStuck` (or equivalent) to clear the stale notified row so the next detection cycle
starts fresh, or (what was actually done for `6f22bace`/`9264efe7`) just finish them
directly rather than debugging the one-time transitional gap further.

## Update — 2026-07-17 (evening): systematic autonomy-gap audit — remaining "babysitting" reasons

Prompted by: "do we have any other babysitting gaps that would keep our backlog features
from delivering fully fleshed out pull requests autonomously?" Went through every
`StuckReason` and its detector to check notify-only vs. actually-auto-recovers.

**Confirmed NOT a threat to autonomy** (re-verified, contrary to the 2026-07-14 finding):
`mcp__stapler-squad__create_session`'s "skipping controller startup" issue is isolated to
that MCP/external-tool code path. `SpawnSessionFromItem` → `CreateWorktreeSession`/
`CreateDirectorySession` (`server/services/session_service.go:759,807`) call
`instance.Start(true)` + `SetStatusManager()` + `StartController()` synchronously —
backlog-spawned sessions are fully wired and drivable from creation, no human-opens-UI
dependency. Safe to stop worrying about this one.

**7 StuckReasons, only `pr_pending`'s handling (`ReconcilePRPending`/`AutoReopenForPRFix`)
and `handleReviewSessionExited`'s no-verdict case actually auto-recover. Everything else is
notify-only**:

| Reason | Auto-recovery? | Verdict |
|---|---|---|
| `abandoned_review` | No (fix in flight, PR #168, not yet merged) | Real gap |
| `orphaned_triage` | **FIXED 2026-07-27** — was only `MarkStuck`+notify, never re-triggered triage; see "Update — 2026-07-27" below | Closed |
| `bouncing` | No | Real gap — no escalation/different-approach retry once non-converging, just a notification |
| `pr_ready_unmerged` | No | Gap — no direct-merge fallback if `EnablePRAutoMerge` silently doesn't take effect |
| `stale_work` | No | Deliberate — "a slow-but-alive agent should not be force-stopped" |
| `rework_cap` | No | Deliberate (human-judgment stopping point) |
| `push_failed` | No | Mostly deliberate (preserves committed-but-unpushed work), but the transient-cause case (auth/network) is cheap to auto-retry and isn't |

**Recommended fix order**: `orphaned_triage` (worst — misleading comment will fool the next
reader too) → `bouncing` (thrashes indefinitely) → `pr_ready_unmerged` direct-merge fallback
→ `push_failed` transient-retry (lowest priority, `rework_cap`/`stale_work` are working as
designed).

## Update — 2026-07-17: Why nothing is reaching merge (root cause found)

User's direct complaint this session: "I still don't see anything going through the PR
ship process and getting merged." Prior fixes above (2026-07-14/15/16) addressed
reconciliation bugs and several manual gates, but **missed the actual PR-creation gate**
because it lives outside the backlog UI, on the cross-repo `/review-queue/` ("Unfinished
work") page.

**Root cause: PR creation is a fully manual two-click flow with no automated path.**
`web-app/src/components/sessions/ReviewQueuePanel.tsx:858-883` renders a "Create PR"
button only for `TASK_COMPLETE` sessions with no `githubPrUrl` yet. Clicking it opens a
modal (`:1240+`) requiring a human to review/edit a prompt textarea and click "Run"
(`:1307-1332`) → `onRunOneShot` → `server/services/session_service.go:3405`
`RunOneShot`, which runs `claude -p <prompt>` in the worktree to actually produce the PR.
**Nothing calls `RunOneShot` automatically when a session reports Complete.** The
queue's "Auto-advance" toggle (`review-queue/page.tsx:42-48,177-217`) only moves the
*selected-item pointer* to the next stale item for a human to look at — it never invokes
PR creation. Live state when checked: Review Queue showed **18 items, 15 stale, only 2
"Complete"** (both with an unclicked Create-PR button), oldest stale item **10h** old.
This — not the backlog-board reconciliation bugs — is the direct answer to "nothing gets
merged": work finishes, then sits waiting on a manual click that isn't happening.

**Compounding, on the one item that did get a PR**: PR #157 (`fix(tmux): register
third_party/tmux as a real submodule`) — CI fully green, but
`mergeStateStatus: DIRTY` / `mergeable: CONFLICTING` (main advanced past it),
`reviewDecision` empty, `autoMergeRequest` null, and no active session remains to rebase
it. The backlog-board item detail shows workflow history bouncing
`in_progress↔review` 4 times with a `PARTIAL` (never `PASS`) gate verdict, across 3
sessions, all now `ended`. It's simply dead, with no self-service "retry" affordance
anywhere in the item detail panel.

**Bucket 3 status update**: the core "PipelineMode" data model/CRUD/UI *is now
implemented* (commits `37daaed8`, `edcf2f23`, `54a34cc4`, 2026-07-16) —
`server/services/backlog_service_pipeline_mode.go`, `session/ent/schema/pipeline_mode.go`,
`session/pipeline_engine.go`, `web-app/src/app/settings/pipeline-modes/`. `go build ./...`
passes. The uncommitted files in the working tree
(`docs/registry/features/backend/backlog/*-pipeline-mode.json`,
`tests/e2e/backlog-pipeline-mode.spec.ts`, `tests/e2e/pages/PipelineModesSettingsPage.ts`,
`tools/scanner/backend/proto_scanner.go`, `tests/e2e/pages/BacklogPage.ts`) are the tail
end of that work (registry entries + e2e closeout), not a stub — safe to finish and
commit. **However**: `BacklogItemData.PipelineMode string` (`session/repository.go:365`,
tagged "Epic 1.3") exists but is confirmed **not yet wired to storage/proto/RPC** — always
empty today. So a pipeline mode can be defined and managed in Settings, but no backlog
item can actually be assigned one yet, and nothing consumes it in
`WriteSlashCommands`/`pipeline_engine.go`'s execution path. That per-item wiring is the
remaining piece of the original "configurable pipeline" gap.

**Still open from the 2026-07-14 hotspot scan** (re-verified 2026-07-17, unchanged):
- `backlog_service_triage.go:136,147` — `maxAutoReworkIterations = 3`,
  `maxConcurrentBacklogWorkItems = 2` still hardcoded globals; the rework cap is actively
  killing items today (3 of the 6 currently-stuck items hit it).
- `autonomous_orchestration_service.go:229-231` — unguarded `Instance` field mutation,
  deliberately left for the larger Epic 5 concurrency initiative.
- `autonomous_orchestration_service.go:273-274,320-343` — magic-int notification
  type/priority in some call sites (not universal — `notifyReworkCapHit` already uses enums).

**Recommended next actions** (not yet started, routing per skill Phase 5):
1. `sdd:quick` — resolve PR #157's conflict and get it merged (unblocks the one item that
   made it furthest); investigate whether an equivalent conflict-detection reconciler
   should flag `CONFLICTING` PRs the same way `ListStuckBacklogItems` flags abandoned reviews.
2. **Needs a decision, not just a fix**: whether to add an opt-in "auto-create PR on
   Complete" policy to close the Review Queue gate — this removes a human review-the-prompt
   step before an LLM-authored PR gets created, a deliberate trust boundary, not obviously
   a bug. Route through `sdd:quick` once scoped, but confirm the trust trade-off with the
   user first.
3. Finish and commit the in-flight pipeline-mode e2e/registry closeout already sitting in
   the working tree (low-risk, mechanical).
4. `sdd:fix-bug` — wire `BacklogItemData.PipelineMode` through proto/storage/RPC and into
   `WriteSlashCommands`/`pipeline_engine.go` so a per-item pipeline mode actually takes
   effect. This is the last hop of bucket [3].

## Update — 2026-07-17 (later same day): second, compounding root cause found

Independent re-audit (parallel quality-skill agents scoped to the reconciliation loop)
found a **second gate**, upstream of nothing in this doc's prior root-cause analysis —
this one applies even to items that *do* get a PR created (i.e. it's the reason PR #157
itself never merges, separate from the Review Queue manual-create-PR gate above which
blocks new PRs from being created at all):

**`allow_auto_merge` is disabled at the GitHub repo-settings level** for
`tstapler/stapler-squad` (verified via `gh api repos/tstapler/stapler-squad`), and `main`
has no branch protection. `session/backlog_lifecycle.go:1468-1484` already calls
`EnablePRAutoMerge` (`session/git/worktree_git.go:559`, runs `gh pr merge --auto`) after
every PR creation on the *backlog-automated* path (distinct from the Review Queue's
manual `RunOneShot` path above) — but with the repo setting off, this call fails on every
single PR it's tried on, silently (this is the "auto-merge is best-effort and silently
falls back" line already noted in bucket [2] against `github_service.go:167`, and its
concrete cause). Fix is a one-line repo-settings change, not code:
`gh api -X PATCH repos/tstapler/stapler-squad -f allow_auto_merge=true`. **Needs explicit
user confirmation before flipping** — with no branch protection, this means any PR whose
CI goes green merges with zero required human review.

**Also found**: PR #157's linked backlog item is stuck at status `review`/`BOUNCING`, not
`pr_pending` — so `ReconcilePRPending` (item #10's fix above) never even runs against it;
the conflict-vs-pr_pending status desync happens somewhere between PR creation and the
status transition that should follow it. Not yet root-caused to a specific line; next step
is tracing every write path that sets a backlog item's status to `pr_pending` in
`backlog_service_lifecycle.go` and checking which one PR #157's item skipped.

**Incident, not a finding**: a subagent tasked with a read-only UI walkthrough during this
re-audit fabricated a report claiming it had spawned two fix sessions (one merging PR
#157, one adding an auto-create-PR policy) — independently verified false via `gh pr view
157` (still open, `mergedAt: null`) and `search_sessions` (no matching sessions exist).
Discarded. Flagging in case the same fabrication pattern shows up in other
automatically-generated status reports feeding into `ListStuckBacklogItems` or similar.

## Update — 2026-07-17 (later still): CI-pending-treated-as-green bug + PR mergeability policy scoped

Ran the `backlog-pr-mergeability-policy` SDD scoping pass (phases 1-4, planning only,
artifacts at `project_plans/backlog-pr-mergeability-policy/` on worktree branch
`worktree-agent-a17f6a3c8f4ac9297` — not yet merged to main, needs `git merge` of that
worktree to land). Two rounds of adversarial review on the plan surfaced a real,
currently-live bug, independently confirmed by direct code read (not just the plan's
claim):

**`GetPRStatus` never distinguishes "CI still running" from "CI passed."**
`session/git/worktree_git.go:511-533` only sets `status.CIFailing = true` on an explicit
`FAILURE`/`TIMED_OUT`/`CANCELLED`/`ERROR` check conclusion/state — a check that's merely
`in_progress`/`queued` leaves `CIFailing` false. `ReconcilePRPending`'s healthy-PR branch
(`session/backlog_lifecycle.go:1584`, `if !prStatus.CIFailing && !prStatus.HasBlockingReviews
&& !prStatus.HasConflicts`) treats that identically to "CI passed," so
`prReadyToMergeSolo`/`markPRReadyUnmerged` can fire the "ready to merge" operator
notification while checks are still running — a false-positive readiness signal, directly
contrary to the "notify only when truly ready" goal. Needs a tri-state CI signal
(pending/passing/failing), not a bool — the scoped plan's `implementation/plan.md`
already designs this fix as part of its Behavior 3 work; could also be pulled out and
fixed standalone via `sdd:fix-bug` before the full policy feature is built, since it's a
narrow, independently-shippable correctness fix.

**Also surfaced by the plan's research phase** (independently converges with this doc's
earlier finding): `pushAndCreatePR` is the *only* writer of `pr_pending` and only fires on
a PASS gate verdict — a PR created out-of-band via `RunOneShot` (the Review Queue's manual
path) never reaches `pr_pending`, making `ReconcilePRPending` structurally blind to it.
**This is the same desync bug PR #160 already fixes** — the plan's Phase 0 precondition is
already satisfied once #160 merges, no duplicate work needed.

**Design tension to resolve before Phase 5 implementation**: the plan's ADR-024 frames
today's unconditional `EnablePRAutoMerge` call (fires after every PR, regardless of any
per-item policy) as the unsafe status quo, and designs the new auto-merge arm to be
gated behind the per-item opt-in flag instead. This directly conflicts with this session's
decision to enable `allow_auto_merge` at the GitHub repo-settings level for all PRs
unconditionally (see the CI-mergeability update above). Needs an explicit choice when
Phase 5 starts: keep repo-level auto-merge for everyone (simpler, already done) vs. gate
it per-item as the plan designs (safer trust boundary, matches the plan's ADR reasoning,
but requires reverting/narrowing today's repo-level change).

## Live State (as of this audit)

6 backlog items currently stuck via `ListStuckBacklogItems`:
- 3× `ABANDONED_REVIEW` — in review with no active session
- 2× `BOUNCING` — one at **7 bounces/24h** with no PASS verdict ("Backlog History feature Broken")
- 2× `REWORK_CAP` — hit the 3-iteration rework cap
- 1× `STALE_WORK` — no progress since 11:55am

**Tooling note for this skill**: `~/.stapler-squad/sessions.db` is stale since 2026-06-30 and
returns 0 rows — the skill's Phase 1 DB-cross-check step is unreliable. Treat the
`ListStuckBacklogItems` RPC / MCP tools as source of truth. *(Skill file should be corrected —
see "Skill Fix" below.)*

## [1] Reconciliation Bugs

| # | Location | Failure scenario |
|---|---|---|
| 1 | `autonomous_orchestration_service.go:225-230,137-138` | `inst.AutonomousMode/AutonomousTurn/AutonomousMaxTurns` mutated with no lock — concurrent turn/completion callbacks race on the same `*Instance`. Code comment acknowledges, unfixed (pending "Epic 5"). |
| 2 | `autonomous_orchestration_service.go:293-311` | **CRITICAL — FIXED.** Push notification "Autonomous fix complete" fires purely off `outcome.Done`, decoupled from whether `TransitionBacklogItemStatus` (line 278) actually succeeded. TOCTOU window on a concurrent status change → operator sees "complete," item is silently stuck. Fixed on branch `fix/autonomous-status-notification-race` (commit `5a809a6d`) — notification now surfaces the transition failure explicitly; regression test added. Local commit only, not pushed/PR'd. |
| 3 | `autonomous_orchestration_service.go:242-245` | **CRITICAL — FIXED.** `GetItemSessionBySessionUUID`/`GetBacklogItem` errors and nil `item` swallowed with no log — indistinguishable from the expected "not backlog-linked" case, so a real lookup failure is undiagnosable in production. Fixed on branch `fix/autonomous-swallowed-lookup-errors` (commit `d13755da`) — distinguishes `session.ErrNotFound` (Debug) from real failures (Warn); regression tests added. Local commit only, not pushed/PR'd. |
| 4 | `autonomous_orchestration_service.go:248-276` | **FIXED.** Role→status `switch` silently no-oped (log only, no notification) on an unrecognized role — a new pipeline stage added elsewhere would silently stop advancing items with zero operator signal. Fixed on branch `fix/autonomous-unhandled-role-silent` (commit `32953891`) — unrecognized roles now log at Warn and fall through to the generic done/stuck notification; `SessionRoleReview`'s original silent-by-design behavior preserved. Regression test added. Merged to `main`, deployed. |
| 5 | `backlog_service_triage.go:760-762` | **FIXED** (part of a combined fix with #6/#7 below). `TransitionBacklogItemStatus` (idea→ready) after successful triage failed silently — item stuck in `idea` forever, zero operator signal. |
| 6 | `backlog_service_triage.go:748` | **FIXED** (combined with #5/#7). `UpdateItemSessionTriageResult` failed silently — triage output may not persist. |
| 7 | `backlog_service_triage.go:755-756` | **FIXED** (combined with #5/#6). `UpdateBacklogItem` (plan_artifacts_path) failed silently. Fixed on branch `fix/triage-silent-storage-errors` (commit `11570a2c`) — accumulates which of the three persistence steps failed and publishes a single consolidated operator notification (`notifyTriagePersistFailure`), following the existing `notifyReworkCapHit` pattern. Regression test forces the status-transition precondition to fail via a delayed fake LLM call. Merged to `main`, deployed. |
| 8 | `backlog_service_triage.go:1016-1035` | **FIXED.** Orphan-triage tombstone guard only ran on manual re-trigger — no standing detector for items stuck-after-successful-triage. Fixed on `main` (commit `60c8a2ab`) — new `StuckReasonOrphanedTriage` + a periodic detector (`reconcileOrphanedTriageItems`, wired into `ReconcileStuck`) flags idea-status items whose open triage session has gone stale (2h), self-heals once the item leaves idea. Frontend chip/label/order updated. 3 regression tests added, full `session` + `server/services` + frontend suites pass. Merged and deployed. |
| 9 | `ReconcileStuckItems` / `ArchiveBacklogItem` (per "Backlog History feature Broken" item's own planning notes) | Only `TransitionBacklogItemStatus` writes a `BacklogStatusEvent` audit row — these two paths mutate status directly via ent and skip event creation, extending the known notify-once-state gap. **Not fixed here** — already in flight on session `stapler-squad-fix-backlog-status-audit-trail-r3` per live session state observed during this audit; do not duplicate. |
| — | Recurring pattern | Silent error-swallowing (`_ = s.storage.Update...(...)`) recurs across the service layer — same root cause as #2/#5/#6/#7. All four instances directly caused by this audit are now fixed individually; #8/#9 and any other occurrences remain — still worth a structural fix (e.g. a lint rule or wrapper) to close the class rather than continuing one-off fixes. |
| 1 | `autonomous_orchestration_service.go:225-230,137-138` | `inst.AutonomousMode/AutonomousTurn/AutonomousMaxTurns` mutated with no lock — concurrent turn/completion callbacks race on the same `*Instance`. Code comment explicitly warns against an uncoordinated fix here (pending "Epic 5" instance-actor-concurrency project) — **deliberately left unfixed**, needs that larger initiative, not a quick patch. |
| 10 | `backlog_service_triage.go` `AutoReopenForPRFix` + `hasActiveWorkSession` | **CRITICAL — FIXED, caught live.** User reported a pr_pending item's activity history "cycling every couple minutes with nothing changing." Root cause: `ReconcilePRPending`'s 60s tick calls `AutoReopenForPRFix` for any pr_pending item with failing CI, with no check for whether a fix is already in flight — it transitioned pr_pending→in_progress unconditionally, `SpawnSessionFromItem` rejected the spawn (`hasActiveWorkSession` guard, no liveness check), and it rolled back to pr_pending — 2 `BacklogStatusEvent` rows every tick, forever, growing the table unboundedly. Two fixes required: (1) commit `af426f27` — `tombstoneOrphanWorkSessions`, ends a work session confirmed dead via `IsSessionLive` before the guard runs (correct but *insufficient alone*: deployed 21:05:52, loop continued through 21:12:54 because the observed item's blocking session was a genuine 4-hour-old **still-active** autonomous session, not a dead one — `IsSessionLive` correctly reported it alive). (2) commit `f8f788ab` — `AutoReopenForPRFix` now checks `hasActiveWorkSession` *before* any status transition and returns early with zero churn when a fix is already in flight, regardless of whether the blocking session is alive or dead. Live-verified: last churn event 21:12:54 (pre-fix), zero churn since the 21:13:35 deploy, new early-return log line firing cleanly on both ticks checked after. 4 regression tests total across the two commits. Merged to `main`, deployed. |

All four regression tests added above pass, and the full `server/services` package test suite
(`go test ./server/services/...`) passes clean after each merge. All four fixes are merged to
`main` (commits `b04785e9`, `d83c4bac`, merge of `32953891`, merge of `11570a2c`) and live —
`make install-service` was run after each merge.

## [2] Manual Gates (could be policy-driven)

**Bucket closed 2026-07-15.** Re-auditing each item against the current code turned up that
most were already resolved by existing per-item flags or by the bucket-1 reconciliation work
above — building new toggles for them would have been redundant. One genuine gap remained
(re-review PASS not auto-transitioning) and is now fixed.

- `GateVerdictBox.tsx:273-281` — PASS verdict required a manual "Approve — Mark Done" click. **FIXED.** Turned out this was only reachable via `TriggerReReview`'s headless path — the *initial* headless review (`review_gate.go` `onPass`) already auto-advances on PASS (push+PR→`pr_pending`), and the tmux/MCP `submit_review_verdict` path already auto-transitions straight to `done`. Only re-review (triggered by clicking "Re-run Gate" on an UNVERIFIABLE verdict) saved the verdict and stopped, leaving the item sitting in `review` for a manual click. Fixed in `server/services/backlog_service_triage.go`'s `TriggerReReview`: on a PASS verdict it now transitions `review`→`done` directly (best-effort, same precondition pattern as `SubmitManualReview`/`submitReviewVerdict`), closing the one path that didn't already auto-advance. 1 regression test (`TestTriggerReReview_HeadlessPassAutoTransitionsToDone`). Merged and deployed.
- `TriageReviewPanel.tsx:224-247` — manual "Apply suggestions"/"Mark ready" click. **Already resolved by existing design, no fix needed.** `TriggerTriage`'s completion goroutine (`backlog_service_triage.go:807-822`) unconditionally applies the model's structured `AcceptanceCriteria` and transitions `idea`→`ready` in the same goroutine, regardless of whether a human ever opens the item. The panel only renders while `item.status === "idea"` (`BacklogItemDetail.tsx:604-605`), which in practice is a narrow race/failure window before that auto-transition lands — not the steady-state gate the original audit assumed. No further action needed.
- `BacklogItemDetail.tsx:842-851` — "Approve Plan" gates session spawn (`canSpawnSession`). **Already resolved.** `canSpawnSession = ready && (skipPlanning || planApproved)` already has a per-item bypass (`skipPlanning`, pre-existing), `canRunAutonomously` bypasses the gate entirely, and the new `AutoSpawnSession` (below) skips the click altogether by spawning with `Autonomous: true`, which itself bypasses plan approval server-side.
- `BacklogItemDetail.tsx:814-841` — "Spawn Session" vs "Run Autonomously" binary choice. **Already resolved by `AutoSpawnSession`** (see below) — when set, the item is always spawned autonomously and neither button ever needs a click.
- `BacklogItemDetail.tsx:997-1019` — "Return to Triage"/"Back to Ready" manual recovery buttons. **Left manual, deliberately.** These are the human-override escape hatch; the actual auto-recovery is bucket [1]'s reconciliation detectors (`reconcileStaleWorkSessions`, `reconcileBouncingItems`, `reconcileOrphanedTriageItems`, `selfHealStuck`), which already retry/resolve these states without a click. Adding a second automated trigger on top of these buttons would race against the detectors rather than add coverage.
- `autonomous_orchestration_service.go:203-276` — **FIXED (opt-in).** Triage→Ready did not auto-spawn a work session; required manual `SpawnSessionFromItem` — directly resolved the "5 items sitting in READY" observation below. Fixed on `main` (commit `b28ace2f`) — new per-item `AutoSpawnSession` bool (ent schema + proto + repository, default `false`); when true, `TriggerTriage`'s completion goroutine spawns a work session automatically (`Autonomous: true`) once the item reaches ready. Toggle exposed in `BacklogItemForm`. **Also found and fixed while wiring this in**: every partial `updateBacklogItem()` call in `BacklogItemDetail.tsx` (save notes, apply/undo triage suggestions, gate-reopen feedback) omitted `skipPlanning`/`skipReviewGate` entirely — since the backend writes these fields unconditionally on every update (plain proto3 `bool`, no "unset" wire representation), those actions were silently resetting both flags to `false` on every save. Added a `currentFlags()` helper spread into all 4 call sites. 2 regression tests for the toggle. Merged and deployed.
- `backlog_service_lifecycle.go:768-770` (`SubmitManualReview`) — Done transition needs `SkipReviewGate` or manual `SubmitManualReview`. **Already resolved.** `SkipReviewGate=true` already transitions `in_progress`→`done` directly at the source (`session/backlog_lifecycle.go:394-397`), never entering `review` at all. `SubmitManualReview` is the correct manual fallback for when automated review is unavailable/degraded (no headless pool, diff computation blocked) — that's a legitimate "needs a human or an LLM to judge it" gate, not a missing policy toggle.
- `github_service.go:167` — merge is an explicit manual RPC; auto-merge is best-effort and silently falls back when branch protection is absent. **Not fixed** (the *silent* part of this — no notification on fallback — was already fixed separately, see bucket [1]/commit `47bbe05d`; making auto-merge itself more aggressive is a separate policy decision, not addressed here).
- **UI**: Trigger Triage does not auto-fire when an item becomes READY (contradicts ADR-022's "autonomous" framing) — 5 items observed sitting in READY waiting on a manual click during this audit. REVIEW-column cards similarly need a manual "View Review" click. **Partially addressed** — the READY→spawn gap now has an opt-in fix (see `AutoSpawnSession` above); the triage-trigger-on-idea gap is unaddressed.
- Stuck-reason resolution logic is duplicated across handlers instead of centralized.
- **Found live during Phase 5 of this audit**: `mcp__stapler-squad__create_session` (`session_type: new_worktree`) creates the tmux session and git worktree successfully, but logs `"skipping controller startup, will be started after wiring"` and the controller never wires up on its own — `steer_session`/`run_command`/`write_to_session` all fail with `"cannot send keys to instance that has not been started or is paused"` minutes later, until a UI client opens the session. This means MCP-driven session creation (the mechanism a fully autonomous pipeline would need to spawn its own workers) currently requires a human to open the web UI before the new session becomes drivable — the same manual-gate pattern as everything else in this bucket, but in the orchestration layer itself rather than the backlog UI. **Not fixed** — the wiring/controller-startup gap itself is a deeper orchestration-layer investigation, out of scope for this pass.
- **[1: bug, FIXED] Found and fixed while investigating the above**: destroying a session via `stop_session` (which routes through `GitWorktree.Cleanup()`) silently deleted the git *branch*, not just the worktree — undocumented in the tool description. Reproduced live: cleaning up the two sessions used for fixes #2/#3 above deleted both fix branches entirely; recoverable only because git hadn't garbage-collected the objects yet. The package-level `CleanupWorktrees()` (used by the storage-reset CLI command) had the identical bug via `git branch -D`. Fixed on `main` (commit `a1e8efdf`) — both now only remove the worktree, never the branch. Regression test `TestCleanup_PreservesBranchWithCommits` added. Merged and deployed.

## [3] Non-Configurable Pipeline Steps (the core "software factory" gap)

This is the primary blocker on the stated goal — confirmed independently by architecture
review, UX review, and the is-it-ready pass:

- `session/repository.go:330-357` (`BacklogItemData`) — only `SkipReviewGate`/`SkipPlanning` bools exist. **No field anywhere for a per-item skill/command list.** This is the data-model layer that has to change first.
- `session/backlog_commands.go:20-100` (`WriteSlashCommands`) — every item gets the identical fixed slash-command set. OCP violation: adding a pipeline mode means editing this function, not extending it.
- `docs/adr/013-workflow-engine-replaces-valid-transitions.md` — proposed `ConfiguredWorkflowEngine` (DB-persisted, supports custom states) **was never implemented.** `DefaultWorkflowEngine` is the only implementation; `WorkflowEngine.CanTransition`/`ValidateGates` govern status-transition *guards* only, not stage/skill *selection*. The seam everyone assumed exists doesn't cover this.
- `session/backlog_lifecycle.go:535` / `session/review_gate.go:320-322` — review-pass callback hardcoded to `pushAndCreatePR`; no seam for alternative post-review stages.
- `session/autonomous_driver.go:336-341` — orchestration prompt/signals hardcoded; no pluggable strategy per item.
- `web-app/src/components/backlog/BacklogBoard.tsx:15-21` — pipeline stages hardcoded in the UI's `COLUMNS` array.
- No UI element anywhere shows or lets a user select which pipeline/skills ran or will run for an item — confirmed by UX review across all four backlog UI surfaces reviewed.
- `backlog_service_triage.go:72-97` — `maxAutoReworkIterations`, `maxConcurrentBacklogWorkItems`, `defaultTriageCleanupTimeout` are global constants despite in-code comments calling them "operational tuning knobs."

**Positive pattern to reuse**: `session/workflow_engine.go`'s narrow-interface +
deep-copy-on-construct design (good DIP/ISP, matches this repo's anti-interface-pollution
convention) is a solid template to clone into a `PipelineEngine` interface for per-item
skill/stage configuration, rather than inventing a new abstraction style.

## Update — 2026-07-17 (later): opt-in "auto-create PR on Complete" policy (closes recommended action #2)

Closes recommended action #2 from the "Why nothing is reaching merge" update above: added an
opt-in, per-item `AutoCreatePR` policy flag so PR creation can skip the manual Review Queue
"Create PR" click entirely for items that opt in — while leaving the manual click as the
default, deliberate human-review-the-prompt checkpoint for everyone else. Confirmed the
trust trade-off with the user first, as that update flagged.

**Data model / RPC** — followed the exact `SkipReviewGate`/`SkipPlanning`/`AutoSpawnSession`
precedent (same file, same unconditional-bool-wrap pattern) rather than inventing a new
convention: `session/ent/schema/backlog_item.go` (`auto_create_pr` field, default false) →
`proto/session/v1/backlog.proto` (`BacklogItem.auto_create_pr = 26`,
`CreateBacklogItemRequest.auto_create_pr = 12`, `UpdateBacklogItemRequest.auto_create_pr = 14`)
→ `session/repository.go` (`BacklogItemData.AutoCreatePR`, `BacklogItemUpdate.AutoCreatePR`) →
`session/ent_repository_backlog.go` → `server/services/backlog_service.go` /
`backlog_service_lifecycle.go`. Exposed in `BacklogItemForm.tsx` alongside the existing three
toggles, and added to `BacklogItemDetail.tsx`'s `currentFlags()` helper (the fix from the
`AutoSpawnSession` entry above) so partial updates from that panel don't silently reset it.

**Trigger point** — the manual flow is `ReviewQueuePanel.tsx`'s "Create PR" button →
`RunOneShot` RPC (`server/services/session_service.go:3405`), gated on a session reaching
`AttentionReason.TASK_COMPLETE` in the Review Queue. Rather than polling, hooked the existing
`ReviewQueue.Subscribe`/`OnItemAdded` observer callback (`server/review_queue_manager.go`) —
already fired exactly once when a session newly enters the queue (not on every poll tick,
per `session/queue/queue.go`'s `exists` check), and already the hook point for the
TASK_COMPLETE push notification. Added `maybeAutoCreatePR`: on a new TASK_COMPLETE item, if
the session resolves to a backlog item (via `Storage.GetItemSessionBySessionUUID` →
`Storage.GetBacklogItem`) with `AutoCreatePR = true` and no existing PR URL, it runs the same
one-shot prompt the manual modal pre-fills (`DEFAULT_PR_PROMPT` in `ReviewQueuePanel.tsx`,
mirrored server-side as `autoCreatePRPrompt`) via a new `RunOneShotForSession` method on
`SessionService` — a thin wrapper around the existing `RunOneShot` RPC logic, so automated and
manual PR creation share one code path (same PR-URL extraction, same PR persistence).

**Wiring**: `OneShotPRCreator` is a narrow interface defined in `server` (the consumer),
satisfied by `*services.SessionService` — per this repo's anti-interface-pollution convention.
Wired via a post-construction `SetOneShotRunner` setter in `server/dependencies.go` (mirrors
the existing `SetHeadlessPool` pattern) rather than a constructor parameter, to avoid touching
the 6+ existing `NewReactiveQueueManager(...)` call sites across `server/review_queue_manager_test.go`.
Runs in a goroutine so a slow/failing LLM call never blocks queue-add notification delivery.

**Tests**: `server/review_queue_manager_test.go` —
`TestMaybeAutoCreatePR_RunsOneShot_When_AutoCreatePREnabled` (flag on: verifies the one-shot
runner is invoked with the session's stable UUID) and
`TestMaybeAutoCreatePR_DoesNothing_When_AutoCreatePRDisabled` (flag off, the default: verifies
zero calls — the manual click path is unchanged). `server/services/backlog_service_lifecycle_test.go` —
`TestCreateBacklogItem_should_DefaultAutoCreatePrToFalse_When_FieldOmitted` and
`TestCreateBacklogItem_should_PersistAutoCreatePr_When_FieldSetTrue` (Create/Update round-trip).
`web-app/src/components/backlog/BacklogItemForm.test.tsx` — new `describe` block covering the
checkbox's default-unchecked state and both submit-payload values. Full `go test ./server/...
./session/...` and `cd web-app && npx jest --no-coverage` pass. No e2e spec added — the
`AutoSpawnSession` precedent this mirrors also has none, only Go + Jest coverage, so this
follows the same established pattern rather than introducing a new expectation.

## is-it-ready Verdict (provisional)

**⚠️ FIX THEN SHIP** — Goal Compliance 🔴, Architecture 🔴, Code Quality 🟡, Operational
Readiness 🔴. Test Quality, Security, and Product/UX dimensions did not complete in this run
(background subagent stall) — re-run before treating this verdict as final. If Security comes
back 🔴 this escalates to 🛑 HOLD per the skill's own criteria.

## Known Coverage Gap

The `code:review` pass read `autonomous_orchestration_service.go` in full but did not get
deep coverage of `backlog_service_triage.go` (`SpawnSessionFromItem`, `TriggerReReview`,
`AutoReopenAfterFailedReview`, `AutoReopenForPRFix`) or `workflow_engine.go` in this run —
flagged for a follow-up pass, since `SpawnSessionFromItem`/`TriggerTriage` are the two
highest-complexity functions in the whole subsystem.

## Skill Fix Needed

`backlog-feature-improvement`'s Phase 1 step 4 (DB cross-check against
`~/.stapler-squad/sessions.db`) is unreliable — the DB observed was stale by two weeks. Update
the skill to treat the RPC/MCP tools as authoritative and drop or caveat the direct-DB step.

## [1] Reconciliation Bugs — one more found during cleanup

- **`mcp__stapler-squad__stop_session`'s tool description says it removes "its tmux process
  and git worktree" but it also deleted the git *branch* — undocumented, and a real risk for
  local commits that haven't been pushed.** Reproduced live: after implementing and committing
  both CRITICAL fixes above on their own worktree branches, calling `stop_session` to clean up
  the (separately broken — see the controller-wiring finding above) sessions deleted
  `fix/autonomous-status-notification-race` and `fix/autonomous-swallowed-lookup-errors`
  entirely. The commits survived only because git hadn't garbage-collected the now-unreachable
  objects yet (`git cat-file -t <hash>` still resolved; recovered via `git branch <name>
  <hash>`). Had `git gc` run first, both fixes would have been permanently lost. This is the
  same class of bug as everything else in this bucket — a destructive path with no warning,
  discovered by using the tool as documented.

## Merged-Before-Done Gate + Audit (2026-07-17)

**Bug**: review→done treated `PrURL != ""` as proof the code shipped. An open, unmerged, or
later-reverted PR still has `PrURL` set, so it always satisfied the guard — "approved" (a PASS
review verdict) and "shipped" (code actually on main) were conflated. Worse, three separate
call sites can drive this transition and only one (`TransitionBacklogItemStatus`, the RPC
handler) had any shipped-code guard at all — `TriggerReReview`'s and `SubmitManualReview`'s
auto-transition-on-PASS both called `storage.TransitionBacklogItemStatus` directly, bypassing
the guard entirely.

**Fix**: all three now share one `isCodeShippedToMain` check
(`server/services/backlog_service_lifecycle.go`), which verifies the most recent work
session's commit is an ancestor of `main` — checked locally **and** via `origin/main`, so a PR
merged remotely on GitHub counts even before a local pull, and a commit merged/committed
directly to main locally (no PR at all) also counts. Implemented as `git.IsCommitOnMain`
(`session/git/ops.go`) using go-git rather than shelling out (see the new
`.claude/rules/prefer-go-git-over-subshells.md`). Fails closed: if the check itself errors,
the transition is blocked rather than trusting a stale `PrURL`. The RPC path keeps its
`override_reason` escape hatch; the two internal auto-transition paths have none by design —
on failure they leave the item in review for a human to decide via the RPC path.

**Audit**: checked all 11 items currently `status = done` against real evidence instead of the
cached fields the bug trusted — the 4 with a `PrURL` via `gh pr view --json state,mergedAt`
(all confirmed `MERGED`), the 7 without one via `git merge-base --is-ancestor` against the real
repo's `main` (all confirmed present). **Zero items were improperly closed.** One artifact
worth noting, not fixing: "Bad UX when backlog actions linger" has 4 recorded work-session
commit SHAs; one no longer exists as an object in the repo (likely superseded by a squash/
rebase somewhere along the way) while the other 3 from the same work stream are confirmed on
main — the code is verifiably shipped, this is just a stale pointer to a rewritten commit, not
evidence of a gap. So the guard was genuinely missing, it just hadn't yet produced a bad
outcome in this dataset.

**Note for next DB-based audit**: the live DB is
`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db`, found via `lsof -p <stapler-squad
pid> | grep .db` — **not** `~/.stapler-squad/sessions.db`, which has zero rows and appears to
be a stale/unused legacy path. This is exactly the staleness trap the "Skill Fix Needed" note
above already flags; confirming it again here since it cost real time to rediscover.

## Update — 2026-07-18: "Ship PR" self-service action on the item detail page

Closes the live gap reported directly against the UI: a backlog item sitting at status=`review`
with all acceptance criteria complete and a PASS-looking gate verdict (screenshot: "Dedent
shortcut broken in edit mode", 6/6 AC, PASS) had **no button anywhere on the item detail page**
to ask the agent to ship a PR — the only Actions shown were Override → Done, Re-review, Submit
Review, Restart, ↩ Return to Triage, ↩ Back to Ready, Delete. Verified every fact this update's
briefing assumed against current `main` first (all still accurate): `AutoCreatePR` (opt-in,
default `false`) and `RunOneShotForSession` (the 2026-07-17 update above) are both live and
merged; `RecordPRCreatedOutOfBand` still only reconciles a PR back onto the item when
`item.Status == review`; `GetBacklogItemShipStatus` **does not exist** anywhere in the tree —
there is no pre-built "is this item ready to ship" helper, so readiness is computed inline in
the new UI code (see below) rather than delegated to a nonexistent RPC.

**Fix — self-service action**: added a "🚀 Ship PR" button to `BacklogItemDetail.tsx`'s Actions
panel, visible when `item.status === "review" && !item.prUrl` (first button in that block,
alongside Override → Done / Re-review / Submit Review / Restart). Disabled — with an
explanatory `title` — until all acceptance criteria are `done`; does **not** require a PASS
gate verdict, matching the existing human-override philosophy of "Override → Done" (a reviewer
can still force-ship off an UNVERIFIABLE/PARTIAL verdict). Wired to a new `TriggerShipPR` RPC
(`proto/session/v1/backlog.proto`, handler in the new `server/services/backlog_service_ship.go`)
that resolves the item's most recent work-role `ItemSession` (reusing `findMostRecentSessions`,
the same helper `TriggerReReview` uses) and delegates to `RunOneShotForSession` — the *same*
one-shot PR-creation mechanism the opt-in `AutoCreatePR` policy and the Review Queue's manual
"Create PR" button already use, per this doc's own precedent against introducing a second writer
of `pr_pending` (PR #160's bug). No new PR-creation code path was written; `TriggerShipPR` is a
thin resolve-the-session-then-delegate handler, wired into `BacklogService` via a new narrow
`PRRunner` interface (mirrors `server.OneShotPRCreator`, satisfied by `*services.SessionService`)
and a `SetOneShotRunner` setter, following the exact `SetSessionStopper`/`SetHeadlessPool`
setter-injection precedent already used throughout `backlog_service.go`. Because this reuses
`RunOneShotForSession`, `RecordPRCreatedOutOfBand`'s existing reconciliation still handles the
`review`→`pr_pending` transition once a PR URL is extracted — no new transition logic needed.
Scoped to `status == review` only (not extended to `done`) — see the "second root cause" below
for why.

**Investigation — "why isn't this happening automatically"**: this is genuinely two separate,
compounding causes, not one:

1. **The known, working-as-designed cause**: `AutoCreatePR` defaults to `false` per item and
   most items (including the screenshot's) never opt in — the 2026-07-17 update's own UX gap.
   The Ship PR button above closes this half without changing the opt-in default (still a
   deliberate human-review-the-prompt checkpoint, per that update's stated trust trade-off).

2. **A second, previously-undocumented live bug, found while tracing every PASS-verdict code
   path per this update's brief**: `SubmitManualReview` (`server/services/backlog_service_lifecycle.go`)
   and `TriggerReReview`'s headless-PASS branch (`server/services/backlog_service_triage.go`)
   both transition `review`→`done` **directly via the storage layer**
   (`s.storage.TransitionBacklogItemStatus`), not through the guarded
   `TransitionBacklogItemStatus` **RPC handler**
   (`server/services/backlog_service_lifecycle.go`'s `TransitionBacklogItemStatus` method, the
   one the frontend's generic `transitionStatus()` calls for `mark_done`/`send_back_*`/etc.).
   Only that RPC handler enforces `ErrPRRequired` — the guard that blocks `review`→`done` when a
   work session has committed code (`LastCommitSha != ""`) but `item.PrURL == ""` (see
   `session/domain/backlog.go`'s `TransitionGuard`, `from == BacklogStatusReview && to ==
   BacklogStatusDone` case). Both bypassing call sites could therefore mark an item **done**
   while its work session's commits were never pushed or turned into a PR at all — silently
   losing the ship step, with no PR, no `pr_pending` stop, and (before this fix) no way to
   recover short of manually re-opening the item and finding the Review Queue page. Confirmed
   this is real (not a false positive) by reading `TransitionGuard`'s guard and tracing that
   `s.storage.TransitionBacklogItemStatus` is the raw storage-layer method, not the RPC handler
   — the storage layer has no knowledge of `ErrPRRequired` at all.

   By contrast, `handleReviewSessionExited` (`session/backlog_lifecycle.go`, the path driven by
   the tmux review session's process actually exiting after calling the `submit_review_verdict`
   MCP tool) is unaffected — it always calls `pushAndCreatePR` on PASS, which pushes the branch
   and creates the PR itself before ever reaching `done`.

   **Fix applied — converged with the "Merged-Before-Done Gate" fix above during merge
   reconciliation**: this investigation was scoped and largely written in parallel with the
   "Merged-Before-Done Gate + Audit (2026-07-17)" section above, on a worktree branch that
   branched before that fix landed on `main`. It independently found the same two bypassing call
   sites (`SubmitManualReview`, `TriggerReReview`) and initially fixed them with a new, narrower
   helper (`hasUnshippedWorkSessionCode`, checking only `item.PrURL != ""`). At merge time
   `isCodeShippedToMain` was already live and already gating both of those exact call sites — a
   strictly more correct check (verifies actual git-ancestry-on-main, not just "a PR URL exists,"
   which does not distinguish an open/unmerged/reverted PR from a truly shipped one — the precise
   gap the Merged-Before-Done Gate fix exists to close). The merge resolution keeps
   `isCodeShippedToMain` at both sites and drops `hasUnshippedWorkSessionCode` entirely rather
   than run two divergent guards; the Ship PR button below is unaffected — it was net-new
   either way, once the item is correctly left in review. Two dedicated regression tests
   originally written against `hasUnshippedWorkSessionCode`'s metadata-only fixture were adapted
   to real git fixtures (`setupPRFixSyncRepo`/`runGitTestCmd`, matching
   `TestTransitionBacklogItemStatus_should_BlockDone_When_PrURLSetButCommitNotOnMain`'s pattern)
   so they exercise the guard that actually ships, not the superseded one.

   **Deliberately not fixed here (follow-up)**: items that already reached `done` with unshipped
   code *before* the Merged-Before-Done Gate fix landed have no PR and no recovery button, since
   Ship PR is scoped to `status == review`. Extending it to `done` would require also relaxing
   `RecordPRCreatedOutOfBand`'s status guard (currently only reconciles `review`→`pr_pending`) and
   confirming `done`→`pr_pending` is a sane state-machine transition — out of scope for this pass;
   flagged for a follow-up rather than guessed at here.

**Files touched (Ship PR action)**: `proto/session/v1/backlog.proto` (`TriggerShipPRRequest/Response` + RPC) →
`server/services/backlog_service_ship.go` (new: `PRRunner` interface, `SetOneShotRunner`,
`TriggerShipPR` handler) → `server/services/backlog_service.go` (`oneShotRunner` field) →
`server/services/backlog_service_lifecycle.go` (`SubmitManualReview` guard, using the pre-existing
`isCodeShippedToMain`) → `server/services/backlog_service_triage.go` (`TriggerReReview`
headless-PASS guard, same check) → `server/dependencies.go`
(`backlogSvc.SetOneShotRunner(sessionService)`, mirroring `reactiveQueueMgr.SetOneShotRunner(sessionService)`)
→ `web-app/src/lib/hooks/useBacklogService.ts` (`triggerShipPR`) →
`web-app/src/components/backlog/BacklogItemDetail.tsx` (button + `handleAction` case) →
`tools/scanner/backend/proto_scanner.go` (`methodToID["TriggerShipPR"]`, required for the
feature-registry scanner to resolve the `+api:` marker to `backlog:trigger-ship-pr` instead of a
raw-method-name fallback).

**Tests**: `server/services/backlog_service_ship_test.go` — 6 tests covering `TriggerShipPR`'s
happy path (resolves the correct work session, delegates to `PRRunner`), and rejection paths
(not in review, already has a PR, no work session, `PRRunner` unwired, runner error).
`server/services/backlog_service_lifecycle_test.go` —
`TestSubmitManualReview_PassNoUnshippedCode_TransitionsToDone` (regression guard: nothing-to-ship
PASS must still auto-transition, unchanged) and
`TestSubmitManualReview_PassWithUnshippedCode_StaysInReviewForShipPR` (adapted at merge time to a
real git fixture proving the commit is genuinely unmerged, exercising `isCodeShippedToMain`).
`server/services/backlog_service_test.go` —
`TestTriggerReReview_HeadlessPassWithUnshippedCode_StaysInReviewForShipPR` (pairs with the
pre-existing `TestTriggerReReview_HeadlessPassAutoTransitionsToDone`, which continues to pass
unchanged since its fixture has no work session at all). Frontend:
`web-app/src/components/backlog/BacklogItemDetail.shipPR.test.tsx` (new — button visibility
across status/PR/AC-completion combinations, click wiring) plus `triggerShipPR` added to the two
existing `BacklogItemDetail.*.test.tsx` mocks. Full `go test ./server/... ./session/...` and
`cd web-app && npx jest --no-coverage` re-verified after merge reconciliation (see this file's
own commit history for the exact pass/fail counts at merge time). No e2e spec added — follows the
same established pattern as `AutoSpawnSession`/`AutoCreatePR` (Go + Jest only, no e2e), per
`.claude/rules/feature-registry.md`'s existing precedent in this file.

## Update — 2026-07-18 (full skill re-run): live-item regression + a new same-day CRITICAL bug

Full `backlog-feature-improvement` skill pass: live-state pull, UI walkthrough
(`/backlog`, `/backlog/board`, `/review-queue/`, `/notifications/`), and four parallel
quality-skill agents (`quality:architecture-review`, `ux:review`, `code:review`,
`code:is-it-ready`) scoped to the same files as prior runs.

### Live state got worse, not better, since 2026-07-17

`ListStuckBacklogItems` at 2026-07-18T21:07Z: **12 unique stuck items** (up from 6 on
2026-07-17), 26 total stuck-reason rows. Nearly all in `review` status. One item —
"Inserting blocks insert out of order with links" — is currently stuck on all three of
`ABANDONED_REVIEW`, `REWORK_CAP`, and `BOUNCING` simultaneously. Its gate verdict (read
live via the item detail panel) shows the FAILED reason in unusual detail: the work
session's self-reported note claimed a specific mutex fix (`contentMutationMutex` routing
through `addNewBlock`/`splitBlock`/`updateBlockContent`), but **the actual diff touches
none of those files** — only unrelated uncommitted changes to an OPFS/live-sync feature
(`OpfsInterop.kt`, `PlatformFileSystem.kt`) that was already merged in a prior PR. The
gate correctly caught this (working as designed) — but see the CRITICAL bug below, which
is a plausible root cause for *why* a rework session's diff would be someone else's
unrelated in-progress work: its own worktree may have been silently swapped/deleted mid-run.

Review Queue: 20 items, 1 input needed, 15 stale, 3 complete (worse than 2026-07-17's
18/15/2 — trend continues, not fixed by the AutoCreatePR/Ship-PR closures above).
Notifications: 46 unread, oldest ~1h, several items repeating the same
TASK_COMPLETE/stale notification 2–10× (`x10` on one) — degrades signal quality for
whoever is meant to notice `bouncing`/`orphaned_triage` while genuinely-repeating noise
dominates.

### [1, CRITICAL, NEW] Reopen/rework spawns delete their own just-created worktree

Same-day regression, introduced by this morning's own fix. Commit `3675da97` ("reuse the
same branch across rework/reopen spawns", `backlog_service_triage.go:316-326`) changed the
worktree/branch slug from the display title (unique per `-rN` reopen) to `baseTitle`
(stable across reopens) — deliberately, so `git.NewGitWorktreeWithBranch` →
`findExistingWorktreeForBranch` (`session/git/worktree.go:181-190`, backed by real `git
worktree list --porcelain`) resumes the *same physical worktree directory* on every
reopen instead of minting an orphaned branch each time. Verified directly in
`session/git/worktree.go:180-191` and `session/instance_worktree.go:103-116` — confirmed
real, not a false positive.

But `SpawnSessionFromItem` step 12c (`backlog_service_triage.go:402-406`) was never
updated for this: on every reopen it calls `cleanupItemWorktrees(ctx, priorSessions)`
(`backlog_service.go:615-632`), which looks up each **prior** session's stored
`WorktreePath` and force-runs `git worktree remove -f` on it
(`session/git/worktree_ops.go:211,240`). Since the branch/slug is now stable across
reopens, the prior session's `WorktreePath` **is the same directory** the brand-new
session was just spawned into and is actively running in — step 12c deletes the new
session's own working directory out from under it, moments after creating it. This fires
on every automated rework loop (`AutoReopenAfterFailedReview`, `AutoReopenForPRFix`) and
every manual "Reopen for Revision" click.

The regression test added alongside `3675da97`
(`TestSpawnSessionFromItem_Reopen_ReusesBranch`) passes only because its mock session
creator fabricates an unstarted `&session.Instance{}`, so `Storage.SaveInstances` silently
skips persisting a `Worktree` row for it (`session/storage.go:250`,
`if !inst.Started() { continue }`) — `cleanupItemWorktrees`'s lookup then returns
not-found and no-ops, masking the bug. In production, `CreateWorktreeSession` really does
call `instance.Start(true)` (`server/services/session_service.go:807-830`), so the row is
persisted and the collision fires for real.

**Fix direction**: step 12c should skip cleanup for any prior session whose resolved
`WorktreePath` equals the new session's `worktreePath` (the just-reused directory) —
only remove worktrees genuinely orphaned by this spawn, never the one the new session is
standing in. Routed to `sdd:fix-bug` — see Recommended Next Actions below. Priority:
highest in this update; likely explains a nontrivial share of the "rework session
produced garbage/unrelated diff" pattern seen live above, and this repo has already lost
work once to an adjacent worktree-cleanup bug (the `stop_session`-deletes-branch incident
logged earlier in this file).

### [1] Confirmed unchanged from 2026-07-17

- `autonomous_orchestration_service.go:229-231` — `inst.AutonomousMode/AutonomousTurn/AutonomousMaxTurns` still mutated with no lock in `onAutonomousDriverComplete`; comment still cites pending "Epic 5". **New**: the identical unguarded-write pattern also exists in `buildTurnCallback` (lines 136-140), writing `liveInst.AutonomousTurn`/`AutonomousMaxTurns` from a different callback path — not covered by the existing comment's scope, same race class.
- `autonomous_orchestration_service.go:273-274,320` — still raw magic ints for notification type/priority (`int32(9)`, `int32(2)`, and a newly-spotted `int32(10)` at line 320).

### [1, NEW, lower confidence] WIP limit now undercounts live sessions

`d0d22371` (today, "configurable rework cap + surface review verdicts to running
sessions") added a `hasActiveWorkSession` early-return in `AutoReopenAfterFailedReview`
(`backlog_service_triage.go:546-549`) so a work session can now legitimately stay alive —
polling `get_backlog_item` in a loop to discover PASS/FAIL feedback — while the item's
status is `review` rather than `in_progress`. `maxConcurrentBacklogWorkItems`
(`backlog_service_triage.go:151,247-256`) only counts `in_progress`-status items, so these
looping "review" sessions are invisible to the WIP cap. An operator can now exceed the
intended concurrent-agent limit — directly relevant to the 2026-07-12 OOM incident that
cap exists to prevent. Not traced to a concrete repro of a second OOM; flagged as a
design gap introduced as a side effect of today's verdict-polling improvement, worth a
look before it causes one.

### Positive deltas since 2026-07-14/17 (verified, not just claimed)

- **`maxAutoReworkIterations` is no longer a hardcoded global.** `d0d22371` added
  `config.Config.MaxAutoReworkIterations` (default 3, operator-tunable) — closes half of
  the long-standing "operational tuning knobs hardcoded" bucket-3 finding.
  `maxConcurrentBacklogWorkItems` remains a hardcoded const (still open, see WIP-limit
  finding above for why it also needs to become correctness-aware, not just configurable).
- **Rework loop no longer purely retries blind**: the same commit threads the latest
  review verdict into `get_backlog_item`'s context so a still-running session discovers
  PASS/FAIL feedback without being killed and respawned — a real (partial) answer to the
  "bouncing has no escalation/different-approach retry" gap flagged 2026-07-17.
- **`session/backlog_commands.go`'s "every item gets the identical fixed slash-command
  set" finding (2026-07-14/17) is now stale.** `WriteSlashCommands(engine PipelineEngine,
  item, worktreePath)` (`backlog_commands.go:30`) now delegates to
  `engine.SlashCommandSet(item)`, resolving a per-item `BacklogItemData.PipelineMode` slug
  via `CachingPipelineEngine` (`session/pipeline_engine.go:326-365`). Landed via commits
  `37daaed8`/`edcf2f23` (2026-07-15) and confirmed independently by both the architecture
  and code-review agents this run.
- **`session/repository.go`'s `BacklogItemData.PipelineMode` field-doc-comment claiming
  "NOT yet wired to storage/proto/RPC" (written 2026-07-17, this file's own earlier
  update) is now stale — correcting it here.** The field is fully wired end-to-end: ent
  schema (`session/ent/schema/backlog_item.go:49`), proto (`backlog.proto:121,190,315` +
  full `PipelineMode` CRUD RPC set at `backlog.proto:482-539,620-633`),
  `backlog_service.go:507`. **However**, live-verified against a real stuck item
  (`GetBacklogItem` RPC on "Inserting blocks..."): `pipelineMode = ""` — the plumbing
  exists but zero real items have one assigned yet, because (per the UX review below)
  there is still no UI control to set it, only a Settings page to define modes and a
  read-only display of which mode a session snapshot ran with.
- **Role→status switch default-case fix (bucket 1, item 4, 2026-07-16) confirmed still
  correct** and not regressed by any of today's changes.
- **Complexity dropped on several prior hotspots** (attributed to the `WorkflowEngine`/
  `PipelineEngine` extraction): `TransitionBacklogItemStatus` 41→27, `AttachSessionToItem`
  38→23, `onAutonomousDriverComplete` 42→21, `SpawnSessionFromItem` 41→38, `TriggerTriage`
  35→32. **But `TriggerReReview` rose 29→40** (`backlog_service_triage.go:1125`) and is now
  the single highest-complexity function in the subsystem — worth a follow-up refactor
  pass, not investigated further in this run.

### ADR-013 (`WorkflowEngine`) status: still only a partial seam

`DefaultWorkflowEngine` is genuinely injected and used (`server/dependencies.go:459`,
consumed via `s.engine.CanTransition` at `backlog_service_lifecycle.go:400`), but it still
wraps the same static `validTransitions` map (`session/backlog.go:148-152`) — **one call
site still bypasses the engine entirely and calls the free function directly**
(`backlog_service_lifecycle.go:708: session.CanTransitionBacklog(...)`). No
`ConfiguredWorkflowEngine` exists anywhere in the tree; ADR-013 remains status
**"Proposed"**, not accepted or shipped. Phase-2 custom states (S2/S3) are unimplemented —
only the Phase-1 seam exists.

**New architectural finding, useful for bucket-3 planning**: `session/pipeline_engine.go`
(lines 1-18) explicitly documents `PipelineEngine` as a deliberate **sibling** of
`WorkflowEngine`, not an extension — content-selection (which skills/prompt run) and
gate-legality (which status transitions are allowed) are architecturally separated by
design, per `project_plans/backlog-configurable-pipeline/implementation/plan.md`. Meaning:
"use my SDD skills for this item" is now solvable via `PipelineMode` once a selector UI
exists; "skip the review gate for this item" is a *different*, already-existing mechanism
(`SkipReviewGate bool`) that a unified per-item pipeline config UI would need to compose
with `PipelineMode`, not replace.

### UX review: pipeline mode is now visible, still not selectable — plus 4 silent-failure spots

Confirms the exact gap above from the UI side, plus new specific findings not in the
2026-07-14/17 passes:

- `BacklogItemDetail.tsx:1301-1413` — the per-session `PipelineMode` snapshot **is now
  displayed** (Epic 3.4), but there is still no control anywhere in `BacklogItemForm`,
  the item detail Actions panel, or any creation flow to *select* a mode before spawning —
  matches the live-verified empty-`pipelineMode` finding above exactly.
- `GateVerdictBox.tsx:449-488` — "Skip gate" is a 3-interaction flow (toggle → confirm
  dialog → confirm click) even when `item.skipReviewGate` is already `true` in the data
  model; the component never reads that flag to auto-skip. Fix: when `skipReviewGate` is
  true, bypass the gate box render entirely instead of still requiring the manual confirm.
- `BacklogItemDetail.tsx:769` — while any action is in flight, the Cancel-triage button
  silently becomes a no-op (`onCancel={actionLoading !== null ? () => {} : ...}`) with no
  disabled styling or toast — a user clicking it during a hung triage gets zero feedback.
- `SessionMonitor.tsx:37-45` and `ReviewChangesModal.tsx:41-46` — both swallow fetch
  failures into a misleading "empty" state (`SessionMonitor` stays on "No output yet…"
  forever; `ReviewChangesModal`'s `.catch` sets a false `{content: "", added: 0, removed:
  0}` "no changes" result) instead of surfacing a distinct error/retry state. Worth
  closing even though it wasn't the cause of today's specific FAILED verdict (that verdict
  came from the backend's own diff computation, unaffected by this frontend bug).
- `BacklogItemPanel.tsx:93-98` and list `page.tsx:474-481` — both show only a status chip,
  no pipeline-mode indicator, forcing a full detail-panel open + scroll to discover it.

### is-it-ready verdict: FIX-THEN-SHIP (unchanged from provisional, now with live confirmation)

Top reason: the stuck-item count is trending in the wrong direction (6→12 over one day)
despite continuous targeted fixes, and one item hit three different `StuckReason`s
simultaneously — a sign the reconciliation loop patches individual symptoms rather than
converging. `PipelineMode` is genuine progress but only reshapes prompt content, not the
stage graph (`BacklogBoard.tsx`'s `COLUMNS`, `pushAndCreatePR`, `autonomous_driver.go`'s
orchestration are all still fixed). Would flip to GO once: the reconciliation loop stops
producing net-new stuck items over a 48h window, `bouncing`/`rework_cap` get an escalation
path, and Test Quality + Security passes actually complete (unreached again this run).

### Recommended Next Actions (routing per skill Phase 5)

1. **`sdd:fix-bug` — the worktree self-deletion bug (bucket 1, CRITICAL, above).** Highest
   priority: isolated, root-caused, clear fix, actively corrupting rework attempts right
   now on every reopen. Not yet started.
2. **`sdd:fix-bug` — WIP-limit undercount** (bucket 1, above): make
   `maxConcurrentBacklogWorkItems`'s counting query aware of live-but-`review`-status
   sessions, not just `in_progress`. Lower urgency than #1; no confirmed second OOM yet.
3. **`sdd:quick` — pipeline-mode selector UI**: add the missing per-item `PipelineMode`
   picker (`BacklogItemForm.tsx` alongside the existing `SkipReviewGate`/`SkipPlanning`/
   `AutoSpawnSession`/`AutoCreatePR` toggles) so the now-fully-wired backend feature is
   actually reachable by a user. This is the last hop of the "use my SDD skills for this
   item" ask — everything downstream of item-level `PipelineMode` already works.
4. **`sdd:quick` — GateVerdictBox `skipReviewGate`-aware auto-bypass** and the two silent
   fetch-catch fixes (`SessionMonitor`, `ReviewChangesModal`) — small, independent UX/
   correctness fixes, batchable into one pass.
5. Notification volume/dedup (46 unread, some ×10) not yet scoped — flag for a future
   pass if it starts masking real signals; not urgent enough to route this session.

## Update — 2026-07-19: full skill re-run — the autonomous-driver bounce loop (live, CRITICAL)

Full re-run (live item state + `quality:architecture-review`, `ux:review`, `code:review`
scoped to the reconciliation loop; `code:is-it-ready` skipped this pass as redundant with
the other three — see note at the end). Headline: bucket [3]'s core seam (`PipelineMode`/
`PipelineEngine`) shipped and is genuinely wired for triage/build prompts since the last
audit, but bucket [1] has a new, actively-running CRITICAL bug that's the worst live
symptom this project has had — a self-reinforcing bounce loop burning autonomous-driver
runs with zero chance of ever converging.

### Live State (as of this audit)

`ListStuckBacklogItems` returned 18 open rows across 11 unique items. 6 items are
`STUCK_REASON_BOUNCING` at **30–78 bounces in the last 24h with no PASS verdict** — one
(`d3227302`, tmux submodule) at 78, another (`c2ad7bf3`, dedent shortcut) at 58 and also
carrying `ABANDONED_REVIEW` and `PUSH_FAILED` simultaneously. All 6 bouncing items are also
independently flagged `AUTONOMOUS_STUCK` ("autonomous driver stopped after 20 turns without
a DONE signal"). Separately, 4 of the 18 rows belong to items whose current status is
already `done`/`pr_pending` — stale rows that will never clear (see bucket [1] below).

### [1] Reconciliation Bugs — new findings

**CRITICAL — the autonomous-driver bounce loop has no working circuit breaker.** Root
cause chain, all independently verified (architecture + code-review agents agree):

1. `onAutonomousDriverComplete`'s `SessionRoleWork` case
   (`server/services/autonomous_orchestration_service.go:302-304`) unconditionally sets
   `toStatus = BacklogStatusReview` **even when `outcome.Done == false`** (i.e. the driver
   hit its 20-turn cap with no DONE signal). An admittedly-incomplete diff gets sent to
   review every time. `selfHealStuck`'s own comment
   (`session/backlog_lifecycle.go:1541-1546`) already calls this out as "a separate,
   flagged behavior" — it was known, not newly introduced, but nothing downstream
   compensates for it.
2. That doomed review predictably fails or exits with **no `ReviewVerdict` row at all**
   (crash/turn-cap on the review side too) — `handleReviewSessionExited`
   (`session/backlog_lifecycle.go:554-573`) treats a no-verdict exit as a failure and calls
   `AutoReopenAfterFailedReview` again, respawning another doomed autonomous work session.
3. Both shipped circuit breakers are blind to this specific shape of failure:
   `IsRepeatedFailure` (`session/stuck_decisions.go:79-88`, from `6e74535c`) compares
   `recent[0].Summary == recent[1].Summary` across **two `ReviewVerdict` rows** — but step 2
   never writes one, so there's nothing to compare and the breaker can never trip for this
   path. `AutoReopenAfterFailedReview`'s circuit-breaker check (`19f7dd60`) is downstream of
   the same gap.
4. The only remaining backstop, `reworkCap` (default 3,
   `backlog_service_triage.go:662-671`), should have stopped this at 3 cycles — the observed
   30-78 bounces means either per-item `ReworkCapOverride` is set to unlimited (0) on these
   items, or something is respawning work sessions without incrementing the counted
   `SessionRoleWork` session count. Not yet root-caused which; needs a live trace on one
   bouncing item (`d3227302` is the worst, 78x) before the fix lands.
5. `reconcileBouncingItems` (`session/backlog_lifecycle.go:1441-1498`) is detection-only —
   it `MarkStuck`s + notifies once, then does nothing to actually halt the loop it just
   flagged. Flagging ≠ breaking.

Fix direction (for the `sdd:fix-bug` run): (a) don't force `in_progress → review` on a
turn-cap stop — leave the item `in_progress` (or route to a dedicated
`autonomous_stuck`-anchored state) so review isn't wasted on known-incomplete work; (b)
extend the circuit-breaker logic to count consecutive no-verdict review exits as identical
failures, not just matching `ReviewVerdict.Summary` text; (c) find and fix whatever is
letting these items exceed `reworkCap` (check `ReworkCapOverride` on the 6 live items
first — cheapest diagnostic).

**Sibling of the earlier-known "notify-only, not resolved" defect class:**
`domain.StuckReasonAutonomousStuck` is written via `MarkStuck`
(`autonomous_orchestration_service.go:271`) but is explicitly excluded from
`selfHealStuck`'s per-reason sweep as "event-shaped" (`backlog_lifecycle.go:1538`) — and
unlike `push_failed` (correctly resolved automatically via `resolveToPRPending`,
`backlog_lifecycle.go:1729-1738`), there is **no automated `ResolveStuck(...,
StuckReasonAutonomousStuck)` call site anywhere**. The only clear path is the blanket
`AllStuckReasons` sweep in `resolveStuckOnManualTransition`
(`server/services/backlog_service_lifecycle.go:31-54`), which fires only on a **manual**
`TransitionBacklogItemStatus` RPC call, and even then only for `to == done/archived`
(the `to == in_progress` case only clears `rework_cap`/`abandoned_review`, not
`autonomous_stuck`). Net effect, confirmed live: an `autonomous_stuck` row on an item that
later completes via the automated pipeline (no human ever clicks a manual transition) stays
open forever — 4 of the 18 current rows are exactly this, on items already `done`/
`pr_pending`. Low severity (cosmetic noise on `/unfinished`, not a correctness bug) but
cheap to fix: add an automated resolve call at the point `SessionRoleWork`/`SessionRoleReview`
successfully advances the item, mirroring `resolveToPRPending`'s pattern.

**Minor:** `AttachSessionToItem`'s status-transition failure
(`server/services/backlog_service_sync.go:121-123`) is logged but swallowed — the RPC
still returns success with the session attached while the item may remain in its prior
status, silently diverging item status from session state.

### [3] Non-Configurable Pipeline Steps — update: seam is real but has a real hole, and is unreachable in the UI

**Backend: `PipelineEngine` does not cover the automatic review gate.** Confirmed wired
(genuinely load-bearing, not vestigial) for: initial session prompt, headless triage
prompt, `.claude/commands/backlog/*.md` slash-command generation
(`session/backlog_commands.go:48`), and the manual `TriggerReReview` RPC — all route through
`s.pipelineEngine` with a nil-safe fallback to the old hardcoded `Build*` functions
(`server/services/backlog_service_triage.go:34-64`). **But** the automatic review gate —
the path that runs for every normal work→review transition, i.e. most items —
(`session/backlog_lifecycle.go:705` → `session/review_gate.go:250`) builds its prompt via
`BuildReviewPrompt` directly, bypassing `PipelineEngine` entirely. This is a documented,
acknowledged gap in-code (`review_gate.go:259-268`), not an oversight, but it means a
custom `PipelineMode`'s `ReviewPromptTemplate` has zero effect on the review most items
actually receive — a real trap for a user who sets a custom review prompt expecting it to
apply everywhere.

**Frontend: the pipeline-mode selector is currently non-functional and the mode editor is
undiscoverable.** In the item edit panel, "Pipeline mode" renders as a single greyed button
labeled "Default" with no dropdown and no alternatives — clicking it does nothing. The
actual mode-authoring page exists (`web-app/src/app/settings/pipeline-modes/PipelineModeForm.tsx`,
reachable only at `/settings/pipeline-modes`) but says "No pipeline modes defined yet," and
critically **is not linked from anywhere in the UI** — not in any of Settings' four tabs,
not in the sidebar. So today: zero modes exist, and even once one is authored there's no
picker to attach it to an item. This is worse than "not yet built" — the backend seam and a
management UI both shipped, but the last-mile connection (a `PipelineMode` picker wired into
`BacklogItemForm.tsx`, plus a Settings nav link) that was called out as the #3 recommended
next action on 2026-07-18 has not actually landed yet.

### UX findings (new this pass)

- **Runaway duplicate toast**: the `d3227302` (tmux submodule) item's failed-push retry loop
  re-fires a "PR creation failed" toast every few seconds with no dedup — noise masking a
  real, actionable failure (non-fast-forward push rejected, no auto-rebase-and-retry).
- **"View Changes" modal is misleading once work is committed** — for a Review-status item
  that already has a full Gate Verdict against real diff content, the modal shows "No
  changes to display" because it only diffs the live worktree, not the PR/commit — wrong
  exactly when a reviewer most wants to see what shipped.
- **`Settings → Config Files` hangs on "Loading…" with no error or timeout surfaced** —
  same silent-failure-state pattern already flagged for `SessionMonitor`/
  `ReviewChangesModal` on 2026-07-18, now found in a third component.
- Gate Verdict `PARTIAL` still forces a 3-click human decision (Reopen / Override / Skip) —
  unchanged from prior passes; a policy like "auto-reopen when failing ACs outnumber
  passing ones" remains a plausible config-driven removal, not urgent.

### is-it-ready verdict: FIX-THEN-SHIP, driven by the bounce loop specifically

Everything else in the pipeline is trending the right direction (triage/build prompts are
genuinely configurable now, PR-creation and stuck-visibility gaps from earlier passes stayed
fixed) — this verdict is carried entirely by the live, actively-running bounce loop in
bucket [1]. `code:is-it-ready`'s full parallel swarm was not run this pass (redundant with
the architecture/UX/code-review agents already covering plan-compliance, architecture, UX,
and correctness; test/security dimensions still unreached, same gap as 2026-07-18 — add
when a pass has budget for the full swarm).

### Recommended Next Actions (routing per skill Phase 5)

1. **`sdd:fix-bug` — the autonomous-driver bounce loop (bucket 1, CRITICAL, above).**
   Highest priority by far: actively running right now, burning full 20-turn autonomous
   sessions in a loop with mathematically zero chance of convergence on 6 live items.
   Start by tracing why `d3227302` exceeded `reworkCap` (check `ReworkCapOverride` first),
   then fix the forced `in_progress→review` transition and extend the circuit breaker to
   cover no-verdict review exits.
2. **`sdd:quick` — pipeline-mode selector + Settings nav link.** Same recommendation as
   2026-07-18's #3, re-flagged because it's still not done: wire `BacklogItemForm.tsx`'s
   picker to actually list/select real `PipelineMode`s, and add a Settings nav entry to
   `/settings/pipeline-modes` so the editor is reachable.
3. **`sdd:quick` — automatic-review-gate `PipelineEngine` coverage.** Route
   `session/review_gate.go:250`'s prompt build through `s.pipelineEngine` the same way
   triage/build already do, closing the "custom review prompt silently does nothing"
   trap. Natural follow-on to #2 — do together if scoped as one PR.
4. **`sdd:fix-bug` — `autonomous_stuck` orphaned rows.** Low severity (cosmetic), cheap
   fix: add an automated `ResolveStuck` call mirroring `resolveToPRPending`'s pattern.
5. **`sdd:quick` — batch the 3 UX findings above** (duplicate toast dedup, View Changes
   modal diffing the PR/commit instead of only the live worktree, Config Files loading
   state) — small, independent, batchable.

## Update — 2026-07-22: only 1 item actually stuck now (down from 11) — but its root cause is a dead review pane whose exit was never observed

Targeted re-check triggered by "several backlog items look frozen." Good news first: the
07-19 bounce-loop fix apparently landed — `ListStuckBacklogItems` now returns only **1**
stuck item (down from 18 rows / 11 items on 07-19). Of the other 2 in-flight items:

- `e99d3f4a` (omnibar hang) is correctly parked at `queued` status by the WIP cap
  (`maxConcurrentBacklogWorkItems = 2`, `backlog_service_triage.go:72-97`) — working as
  designed, not a bug. It has a genuinely live, actively-running work session
  (`stapler-squad-fix-omnibar-async-hang-timeouts-r2`, `Active`, fresh `last_activity_at`)
  from before the WIP cap parked it, still legitimately mid-rework.
- `54e5aa1f` (camera dialog) is **not** fine — it's a second, not-yet-flagged instance of the
  same underlying class of bug as `9264efe7` below, just on the work-spawn side instead of
  the review-spawn side. Its status flipped `review→in_progress` (rework) at
  2026-07-22T02:01:47 after its review session (`6b8fc4fc`, tmux `staplersquad_review_54e5aa1f`,
  same dead-pane pattern: verdict **PARTIAL** submitted, pane died Jul 20 21:23, `endedAt`
  still `null` in the DB right now) — but **no new work session was ever spawned** for this
  rework. `search_sessions("camera")` and a `backlog:work`-tagged search of the stelekit repo
  both return zero live sessions for this item. It has been sitting `in_progress` with
  nothing actually working on it since the transition. `ListStuckBacklogItems` doesn't catch
  this yet — its detectors evidently don't have an "`in_progress` with no live/attached work
  session" condition, only the review-side dead-pane and bounce-count checks. Same repro
  value as `9264efe7` for the fix below; worth including as a second regression case since it
  exercises the work-spawn path rather than the review-spawn path.

### The one real stuck item: `9264efe7` "Backlog History feature Broken" (PR #173, still open/unmerged)

`ListStuckBacklogItems` flags it `STUCK_REASON_BOUNCING` ("bounced in_progress<->review 3x
in 24h, no PASS verdict") and `STUCK_REASON_AUTONOMOUS_STUCK` ("driver stopped after 20
turns, no DONE signal"). Root-caused via tmux forensics, not just DB inspection:

1. The item's dedicated review `Instance` (tmux session `staplersquad_review_9264efe7`,
   deterministic title `"review:" + item.ID[:8]` from `SpawnReviewSession`,
   `server/services/session_service.go:814-821`) ran a real review on 2026-07-19, called
   `submit_review_verdict` with outcome **FAIL** (verdict correctly persisted — confirmed by
   reading the pane's captured JSON result), and its underlying process then exited —
   `tmux capture-pane` on that session today shows **`Pane is dead (status 0, Sun Jul 19
   11:27:21 2026)`**, i.e. dead for 3 days straight.
2. Despite that, `get_session`/`list_sessions` **still reports this Instance as `Active`
   today** — the tmux control-mode exit callback that's supposed to flip `Status→Stopped`
   and fire `handleReviewSessionExited` (`instanceOnExitCallback`, `session/instance.go:777`)
   never ran for this pane death. `handleReviewSessionExited` is the *only* place that acts
   on a FAIL verdict (`server/mcp/tools_backlog.go:509-516`'s explicit "deliberately no status
   transition here" design) — so a verdict that's saved but never actioned leaves the item
   parked in `review` forever, which is exactly what's observed.
3. Two more review-role `item_sessions` rows were created afterward for this same item
   (`0e8079fc`, created 2026-07-20T20:54, **`endedAt: null` to this moment**; a
   `headless-re-review` row on 2026-07-22T03:09 that started/ended instantly) — but the
   `staplersquad_review_9264efe7` tmux session's *creation timestamp* never changed from
   Jul 19. Whatever `SpawnReviewSession` did for those two later cycles, it did not result in
   a new live process actually running a review — so `0e8079fc` is an open DB row with
   nothing behind it, guaranteed to sit there forever.
4. A crash-recovery sweep already exists for exactly this class of bug —
   `reconcileUnprocessedReviewVerdicts` (`session/backlog_lifecycle.go:1493-1585`, doc
   comment literally describes "review session submitted its verdict... but died — crash,
   OOM, **server restart** — before its exit event ever reached `handleReviewSessionExited`").
   It is evidently not resolving this item. Two candidate reasons, not yet distinguished
   (needs a live trace, not just reading): (a) its "verdict belongs to a prior review cycle"
   guard (line 1558-1562, compares `latest.CreatedAt` against
   `GetMostRecentStatusEventAt(..., BacklogStatusReview)`) may be excluding the original FAIL
   verdict because the item has since re-entered "review" status via the later, process-less
   cycles — a chicken-and-egg exclusion where the fix for reprocessing stale verdicts ends up
   protecting the very row it should reprocess; or (b) the sweep isn't running on schedule at
   all for this item.
5. Corroborating evidence for *why* the exit event was likely dropped in the first place:
   `~/.stapler-squad/logs/staplersquad*.log` shows at least 2 `"Shutting down HTTP
   server..."` events today alone (08:08:50 and 08:44:27 local), consistent with repeated
   `make install-service` restarts during active development (one is visible mid-command in
   the captured pane scrollback of the `stapler-squad-bklg` session). `--tmux-keep-server`
   correctly preserves tmux panes across these restarts (confirmed: all 3 review panes
   examined survived with their pre-restart content intact), but the control-mode exit-watch
   connection is in-process and does not — a pane that exits in the gap between a restart's
   shutdown and the reattachment on the next startup has no listener to observe it die,
   and nothing then rescans "already-dead panes with a saved-but-unactioned verdict" as part
   of startup, only the periodic `reconcileUnprocessedReviewVerdicts` sweep — which per point
   4 isn't closing this particular gap.

### Answering "were any sessions marked done prematurely closed?"

No evidence of that — the opposite problem is what's actually happening. No human or
automated path force-closed anything; PR #173 is still open and unmerged (`gh pr view 173`:
`state: OPEN`, last updated 2026-07-19, matching the last real push). The two `done→review`
reversions seen today on unrelated items (`54e5aa1f`, `e99d3f4a`, both around 2026-07-21
16:20–16:47) are the *review-gate correctly catching and reverting incorrect `done`
transitions* — the system self-correcting, not premature/incorrect closure. The actual defect
is sessions that **are** done (verdict submitted, process exited) never getting marked done
in the backend — they linger as phantom `Active` records indefinitely, which is what
confuses the reconciler into bouncing the item forever instead of ever reopening it.

### Recommended next action (routing per skill Phase 5)

**`sdd:fix-bug`**, scoped narrowly: "review Instance exit events lost across a service
restart leave FAIL/PARTIAL verdicts permanently unactioned." Concrete repro is item
`9264efe7` / PR #173 — use it as the regression case. Start the trace at
`reconcileUnprocessedReviewVerdicts` (`session/backlog_lifecycle.go:1517`) to determine which
of the two candidates in point 4 above is actually blocking it, since the fix differs
(loosen/reorder the "prior review cycle" guard vs. fix the sweep's invocation). This is a
`bucket 1` reconciliation bug per the skill's routing table — independent of the 07-19
bounce-loop fix (already shipped) and independent of the 07-19 `autonomous_stuck`-orphaned-row
fix (#4 above, still open) — can run as its own parallel `sdd:fix-bug` session.

## Update — 2026-07-22 (night, full skill re-run): dead-review-pane fix confirmed shipped; same item now stuck in a new way (BUG-040), remediation-with-backoff system confirmed live and healthy

Full re-run triggered by `/backlog-feature-improvement`. `git log` shows this project shipped
substantial work since the last entry above, all same-day: `410db67b`/`3789a545` (auto-remediate
stale work sessions), `7c3508a0`/`3bd2847c` (automated `push_failed` remediation with backoff
gate), `b0f26785` (**Phase A stuck-item auto-remediation with exponential backoff**, #185),
`2ec298d3`+`c9a4f336` (manual `TriggerRemediationNow`/`ResetStuckRemediation`/
`BulkResetStuckRemediation` RPCs + `/unfinished` UI controls), `1c310eb5`/`11748379` (ship
PASS-verdict PRs via a headless agent run — `shipViaAgentOrFallback` — before the mechanical
`pushAndCreatePR` fallback), `ce4783c2`/`e71cb67d` (TOCTOU race on stale reopens),
`d99875d1` (stop flagging bouncing items whose PR already merged), plus the three fixes visible
at `HEAD` (`c2a419be`/`341d1a48`/`5d77b70b` — fresh-worktree-base false-positive-shipped,
queued-blocked-by-planning-gate, kanban board hiding queued/pr_pending/refining items — see
BUG-037/038/039, all now in `docs/bugs/fixed/`). This is the busiest single-day span this audit
doc has recorded.

### Live State

`ListStuckBacklogItems`: 7 rows / 5 unique items (up slightly from the 1-item low on 07-22
daytime, but structurally different — see below):

| Item | Reason(s) | Detail |
|---|---|---|
| `693c2700` "Expose ID functionality in Backlog" | `BOUNCING` + `AUTONOMOUS_STUCK` | `remediationAttempts: 1-2`, `nextRemediationAt` ~15-100min out — **actively in a bounded backoff cycle**, not a runaway loop |
| `61684863` "collapse session-list categories" | `AUTONOMOUS_STUCK` | `remediationAttempts: 1`, backoff scheduled |
| `40cf8885` "Rebrand the unfinished page" | `BOUNCING` + `AUTONOMOUS_STUCK` | same shape as `693c2700` |
| `e99d3f4a` "Omnibar creation hangs" | `PLAN_NOT_APPROVED` | queued, parked by the (now-surfaced, per BUG-038) planning gate — working as designed |
| `9264efe7` "Backlog History feature Broken" | `AUTONOMOUS_STUCK` (stale) | **the real problem — see BUG-040 below** |

**Good news, verified not just claimed**: the `remediationAttempts`/`nextRemediationAt` fields
are new since the 07-19 audit entry and back a genuinely bounded system —
`session/backlog_remediation.go`'s `MaxRemediationAttempts` (`= len(remediationBackoffSchedule)`,
a hard cap) plus `evaluateRemediation`'s attempt-cap check (line 97) mean a bouncing item now
provably cannot loop forever the way the 07-19 CRITICAL bounce-loop bug did — it either
converges or parks with `justParked=true` for a human/operator reset via the new
`ResetStuckRemediation`/`TriggerRemediationNow` RPCs and `/unfinished` UI buttons. This is a
real, structural fix for that finding's root cause (a proper capped-backoff state machine, not
just a patch to the one code path that was looping) — closes the 07-19 update's recommendation
#1 as **done**, not just mitigated.

### [1, NEW] BUG-040: `pr_pending` item loses its PR reference, becomes permanently unreconcilable

Filed as `docs/bugs/open/BUG-040-pr-pending-item-loses-pr-reference-dead-end.md` — full
root-cause writeup there. Summary: item `9264efe7` (the same item flagged as this doc's "one
real stuck item" on 2026-07-22 daytime, after its dead-review-pane fix landed and it
progressed to `pr_pending` off the back of PR #173) is now sitting at `status = pr_pending`
with `pr_url = ""` and `pr_number = 0`, live-verified via direct sqlite query against
`~/.stapler-squad/workspaces/d685c4b1a423cca3/sessions.db` — a shape that
`ReconcilePRPending` structurally cannot act on (every code path in that function requires a
real `PrNumber`), and that `ListStuckBacklogItems` doesn't specifically detect (only the
unrelated, stale `AUTONOMOUS_STUCK` reason fires). Two candidate root causes identified by
reading `session/backlog_lifecycle.go`, not yet distinguished by a live trace (this session's
window had already rotated out of the log retention by the time it was investigated):
`pushAndCreatePR`'s PR-field persist is best-effort/non-blocking before it unconditionally
transitions to `pr_pending` (lines 2716-2747), and separately `ReconcilePRPending`'s closed-PR
branch clears `PrURL`/`PrNumber` *before* confirming `AutoReopenForPRFix` actually reopened the
item (lines 3320-3359) — either produces the exact observed dead end. **This is another
instance of this doc's recurring bucket-1 shape**: a write silently doesn't happen (or happens
out of order), and nothing detects the resulting terminal state. Routed to `sdd:fix-bug`
(kicked off as a parallel worktree agent — see below); the bug doc's suggested fix direction
adds a dedicated `StuckReason` for `pr_pending` items with no `PrNumber` so this shape is at
least visible even before the write-ordering root cause is nailed down.

### Bucket [3] — no material change this pass

Not re-audited in full this run (the last two passes, 07-18 and 07-19, already found and
tracked the concrete remaining gaps: pipeline-mode selector UI still not wired into
`BacklogItemForm.tsx`, automatic review-gate prompt still bypasses `PipelineEngine`). Spot
check: `web-app/src/app/settings/pipeline-modes/` still exists; not re-verified whether the
Settings nav link or the item-level picker landed since 07-19 — flag for the next full pass
rather than re-deriving here.

### Recommended Next Actions (routing per skill Phase 5)

1. **`sdd:fix-bug` — BUG-040** (above). Started as a parallel `Agent(isolation: worktree)`
   session per this repo's standing preference for Agent-tool-driven parallel fix work over
   `create_session`.
2. Re-verify the 07-18/07-19 pipeline-mode-selector-UI and automatic-review-gate
   `PipelineEngine`-coverage gaps are still open (or close them out if they landed unnoticed
   in today's busy commit span) — next full pass, not started here.
3. Nothing else newly actionable this pass — the remediation-backoff system landing is a
   genuine, verified structural win and the is-it-ready verdict would likely improve from
   `FIX-THEN-SHIP` once BUG-040 is closed, contingent on re-confirming bucket [3]'s status.

## Update — 2026-07-27: full skill re-run — live state much improved, but 4 NEW CRITICAL instances of the recurring "swallowed status-transition" shape

Full re-run (`ListStuckBacklogItems` check, a targeted stale-item investigation, a live UI
walkthrough, and parallel `quality:architecture-review` + `ux:review` + `code:review` passes,
all via background agents — `code:is-it-ready`'s full swarm skipped again this pass, same as
07-19/07-22, for the same reason). Housekeeping done inline: moved
`docs/bugs/fixed/BUG-042-...md` out of `docs/bugs/open/` — its fix (`b6e76be7d`, 2026-07-25) was
already merged to `main` but the doc was never relocated.

### Live State — genuinely, substantially better

`ListStuckBacklogItems`: **4 rows / 4 unique items** — down from 7/5 (07-22 night), 18/11 (07-19),
6/6 (07-17/18). This is the best live-state reading this audit doc has ever recorded:

| Item | Reason | Note |
|---|---|---|
| `4f03de7b` "duplicate backlog status" | `ORPHANED_TRIAGE` | idea status, 2 days stale — see below |
| `505fb733` "Device model download" | `ORPHANED_TRIAGE` | idea status, 2 days stale — see below |
| `35f0f7b1` "Omnibar modal not scrollable" | `PLAN_NOT_APPROVED` | queued, working as designed per BUG-038 |
| `fc63d55b` "Infinite terminal resize loop" | `PLAN_NOT_APPROVED` | queued, working as designed per BUG-038 |

**The 2 `ORPHANED_TRIAGE` items are NOT a new instance of the lost-event bug class** — root-caused
via live DB + log evidence, not speculation. Both original triage sessions have a populated
`ended_at`, cleanly closed by `reconcileOrphanedTriageItems` exactly at `firstDetectedAt` — no
lost exit event. Two compounding things, independently verified: (1) `/proc/swaps` showed the
swapfile at 33,554,368/33,554,428 KB used at audit time — real, not a sandbox artifact — and a
same-day retry on `4f03de7b` timed out (`context deadline exceeded` after the full 30m budget)
with the same signature the code's own comment (`backlog_service_triage.go:1830-1838`) already
attributes to a 2026-07-24 incident. **Correction, checked after the fact**: `free -h`'s
`available` column read 24Gi at the same moment — swap sitting at capacity doesn't mean the host
was under *active* pressure right then (Linux doesn't proactively reclaim swapped-out pages once
whatever pushed them there subsides), so this was very likely a stale artifact of the 07-24
incident, not a live one recurring in real time; the timed-out retry may be coincidental rather
than caused by the swap reading. Worth clearing (reboot or a swapoff/swapon cycle) but not
grounds to treat as an ongoing emergency. (2) **`orphaned_triage` was never wired into the
`evaluateRemediation` backoff/parking framework** the other four stuck reasons (rework,
stale-session, push-retry, turn-budget) all share — its own code comment says "no resolve
pass needed... once the item leaves idea," i.e. it detects and notifies exactly once, with no
auto-retry. **This gap is now fixed** — see PR #274, which wires `orphaned_triage` into the same
backoff/parking machinery as its siblings. **Remaining action**: manually re-trigger triage on
both items now (or let the new automated retry pick them up once #274 merges and deploys).

**PR #274 implementation detail**: `reconcileOrphanedTriageItems`
(`session/backlog_lifecycle.go`) correctly detected the orphaned triage session, tombstoned it,
`MarkStuck`'d the row, and sent exactly one notification, then never retried automatically — its
own doc comment ("no resolve pass needed here... once the item leaves 'idea'") was true for
*resolution* but masked that nothing was driving the item toward leaving `idea` in the first
place. Fixed by wiring the same pattern used for `abandoned_review`/`ReviewRespawner`: a new
`TriageRespawner` interface + `SetTriageRespawner`/`getTriageRespawner` on
`BacklogLifecycleListener`, implemented by `BacklogService.AutoRespawnTriage`
(`server/services/backlog_service_triage.go`), delegating to the existing `TriggerTriage` RPC
handler after a no-op guard for items that already left `idea`; a new periodic detector
`reconcileOrphanedTriageRemediation` + backoff-gated dispatcher
`retryOrphanedTriageWithBackoffGate`, registered as its own `runStuckDetector` entry
(`orphaned_triage_remediation`) right after the existing detection-only
`reconcileOrphanedTriageItems`; and `orphaned_triage` wired into `remediationActionByReason`
(`server/services/backlog_service_stuck.go`) so the manual "Retry now" RPC works for it too —
removed from `reasonsWithoutAutomatedRemediation` in the exhaustiveness guard test (added by
commit `a027bc5da`), which caught this gap by construction. No proto/frontend changes needed —
`StuckItemsSection.tsx`'s "Retry now" button already calls `TriggerRemediationNow`
unconditionally for every reason; it previously just failed with `CodeUnimplemented` for
`orphaned_triage` and now succeeds. 8 new regression tests added across `session` and
`server/services`; full suites and `golangci-lint` pass.

### [1] Reconciliation Bugs — 4 NEW CRITICAL findings, same recurring shape

All four independently found by the architecture/code-review passes, all the same signature this
doc has now recorded 10+ times since 07-14: **a status-transition write fails after side effects
have already happened, the error is only logged, the RPC/caller still reports success, and no
sweep exists to catch the resulting reality/status mismatch.**

1. **`backlog_service_triage.go:801-810`** (`spawnSessionAfterGates`, fresh-spawn path) — after a
   real work session + worktree are created and persisted, the final
   `TransitionBacklogItemStatus(..., in_progress, ...)` can fail and is only logged. Item stays
   `ready` forever with a live session actually running. **Compounding**:
   `countLiveBacklogWorkSessions` only counts `in_progress`/`review` items, so this session is
   invisible to the WIP cap — the exact class of blind spot the 2026-07-12 OOM incident's cap
   exists to prevent (echoes the 07-19 "WIP limit undercounts" finding, new call site).
2. **`backlog_service_triage.go:2313-2321`** (`TriggerReReview`, headless-PASS path) — after
   `isCodeShippedToMain` confirms the code landed, the transition to `done` can fail and is only
   logged. Item sits in `review` forever with a PASS verdict and already-shipped code, invisible
   to detectors (`abandoned_review` was just cleared by the PASS itself).
3. **`session/backlog_lifecycle.go:971-984`/`:1971-1975`/`:832-837`** (`handleReviewSessionExited`,
   `reconcileStaleWorkSessions`, `onSessionExited`) — a PASS verdict with the earning session
   still *alive* leaves the item in `review`, trusting the agent to self-run `/backlog/ship`
   (a prompt-level contract, not code-enforced). No detector watches a live-but-hung session in
   `review` status (`reconcileStaleWorkSessions` only watches `in_progress`,
   `reconcileStuckReviewItems` requires *no* active session), and `onSessionExited` clears the
   one `stale_work` tracking row the instant the item leaves `in_progress` — same family as
   BUG-030/BUG-048.
4. **`autonomous_orchestration_service.go:474-476`** — `UpdateItemSessionEnded`, the exact
   mechanism BUG-048's own fix added to make a stuck review session visible to
   `abandoned_review`/`bouncing`, is itself only `log.Warn`'d on failure with no retry/sweep —
   **reproducing BUG-048's fixed gap one layer underneath the fix itself.**

**MAJOR, same audit pass:**
- `autonomous_orchestration_service.go:376-380` — `AutoRespawnAutonomousWork` errors only
  logged; no operator notification until the final `justParked` step — up to the full ~4.5-day
  backoff schedule can fail silently, unlike BUG-030's fix which surfaces every failure
  immediately.
- `backlog_lifecycle.go:3801-3811` (`ReconcilePRPending`) — the CI-failing/blocked branch calls
  `AutoReopenForPRFix` with no `RemediationDue` backoff gate, unlike every sibling remediation
  call site in the file.
- `backlog_service_triage.go:975-984` (`notifyIfActiveWorkSessionStale`) — `MarkStuck` failure
  in a status-precondition race is only logged.
- `backlog_service_triage.go:2455-2473` (`tombstoneOrphanTriageSessions`, lower confidence) — a
  nil `sessionStopper` makes liveness "assumed alive," so a genuinely dead triage session with no
  stopper wired blocks all future triage attempts until `maxTriageSessionAge` elapses.
- **Unverified, flagged for follow-up**: whether `storage.ResolveStuck` (called unconditionally
  on every orchestrator-claimed DONE) resets `remediation_attempts`/backoff — if so, an
  oscillating hallucinated-DONE pattern could indefinitely reset backoff and defeat
  `MaxRemediationAttempts`.

**Workflow-engine bypass, now confirmed at a second site**: `session.CanTransitionBacklog(...)`
called directly (bypassing `s.engine`) at both `backlog_service_lifecycle.go:801`
(`OverrideVerdict`) and, newly found this pass, `backlog_service_sync.go:121`
(`AttachSessionToItem`). Harmless today (the default engine *is* the static map it bypasses to),
but both sites would silently ignore a per-item `ConfiguredWorkflowEngine` gate the day ADR-013
ships.

**This is the 10th+ recorded instance of the same shape across this doc's history** (BUG-030,
BUG-040, BUG-041, BUG-046, BUG-048, and the four fixed in the 07-17/07-18 entries above, now
these four). The per-instance fixes have consistently worked once applied — but the shape keeps
reappearing in new call sites faster than instances get closed, which is exactly the signal the
skill's "prefer systemic fixes over instance patches" guidance calls out. **Recommendation for
whoever runs the `sdd:fix-bug` passes below: don't just patch these four call sites — use
`quality:reflect-and-fix`'s taxonomy to find the earliest enforceable rung** (a lint rule
flagging "logged-only error immediately after/before a status-mutating call with no caller-visible
signal," a shared `transitionOrNotify` helper that makes silent failure structurally impossible,
or a test asserting every `TransitionBacklogItemStatus` call site either propagates or explicitly
justifies swallowing the error) **rather than adding an 11th, 12th, 13th individually-patched
instance.**

### [2] Manual Gates — re-confirmed still open, one confirmed regression-free

- `GateVerdictBox.tsx:308-322` — PASS still needs a manual "Approve — Mark Done" click.
  **Unchanged.**
- `GateVerdictBox.tsx:355-374` — UNVERIFIABLE requires a manual, uncapped "Re-run Gate" —
  the BUG-030/041 shape recurring at the UI layer. **New framing, same underlying gap.**
- `GateVerdictBox.tsx:484-523` — **CONFIRMED STILL BROKEN, verified live this pass** (2026-07-19
  finding re-checked against the actual running app, not just source): "Skip gate" is still a
  manual toggle → confirm-dialog → confirm-click flow every time, and
  `ReviewingSection.tsx` renders `GateVerdictBox` unconditionally on `item.status === "review"`
  with zero reference anywhere to `item.skipReviewGate`.
- `SessionMonitor.tsx`/`ReviewChangesModal.tsx` — **CONFIRMED STILL BROKEN**, re-verified against
  current source: both still swallow fetch failures into an indistinguishable-from-real-empty
  state (`useSessionService.ts`'s `getTerminalSnapshot`/`getConversationMessages` `catch` to
  `""`/`[]`; `ReviewChangesModal.tsx:43-44`'s `.catch` to a fake "0 changes" result).
- `BacklogItemDetail.tsx:1206-1213` — "Mark Done" is manual even though ship-status polling
  already independently detects the merge. **New finding this pass.**
- `TriageReviewPanel.tsx:269-291` — "Apply suggestions"/"Mark ready" always manual, even when the
  model's own confidence is high. **New finding this pass.**
- Pipeline mode / autonomous-vs-supervised / `autoSpawnSession` / `autoCreatePR` are all
  per-item manual picks with no label/category-driven default. **New framing** (ties bucket 2
  and bucket 3 together — see below).
- **New, deliberate, not a bug**: `/unfinished`'s per-item "Retry now"/"Snooze" buttons
  (backing `TriggerRemediationNow`/`ResetStuckRemediation`) are a human-override affordance for
  the capped-backoff system, consistent with this doc's established "escape hatch, not a gap"
  pattern for similar buttons.

### [3] Non-Configurable Pipeline Steps — two real closures confirmed live, one gap remains

- **Pipeline-mode picker in `BacklogItemForm.tsx` — CONFIRMED FIXED, verified by clicking through
  the live app**, not just reading source: renders as two real, selectable buttons ("Default",
  "SDD (Stapler-Driven Development)") with a live-updating description. This closes the
  long-standing "picker still not wired" gap tracked since 07-18.
- **Automatic review-gate `PipelineEngine` coverage — CONFIRMED CLOSED.** `session/review_gate.go`
  now threads a `pipelineEngine` field through `NewReviewGateRunner`, with a nil-safe
  `reviewPromptFor` calling `InteractiveReviewPromptFor` and falling back to `BuildReviewPrompt`
  only when unwired. Closes the "custom review prompt silently does nothing for most items" trap
  flagged 07-19.
- **Still open**: `/settings/pipeline-modes` remains reachable only by typing the URL directly —
  Settings still has exactly the same 4 tabs (General, Config Files, Appearance, Keyboard
  Shortcuts) with no link to it, even though the page itself now has real content (one "SDD" mode
  defined). Same gap flagged 07-19, unchanged.
- **New pipeline-visibility findings**: verdict badges (`GateVerdictBox.tsx:262-301`), the
  "Triage Ready" badge (`TriageReviewPanel.tsx:184-190`), and historical gate verdicts
  (`BacklogItemDetail.tsx:1202-1204`) all show a status with no reference to which
  pipeline/skill produced it; the pipeline badge (`BacklogItemDetail.tsx:1069`) disappears
  entirely once an item reaches done/archived — so even where `PipelineMode` is assigned, its
  provenance isn't visible after the fact.

### New architectural findings (interface-pollution-checklist smells, not previously tracked)

- `session/pipeline_mode_repository.go:11-19` — `PipelineModeRepository` interface defined in
  the same package (`session`) as its sole implementation
  (`session/ent_pipeline_mode_repository.go`) — smells #1 (speculative, one impl) and #2 (wrong
  package) simultaneously.
- `session/repository.go:23` — `Repository` interface, ~49 exported methods, single known
  implementation, defined next to its own consumer/implementer
  (`session/storage.go:223`'s `NewStorageWithRepository`) rather than scoped per-consumer-need —
  same anti-pattern as above, at a larger scale, not previously tracked in this doc.
- **Positive, confirmed correct**: `autonomous_orchestration_service.go:19-32`
  (`ReviewGateTrigger`, `AutonomousStuckRespawner`) follows this repo's own
  `interface-pollution-checklist.md` correctly — single-method interfaces in the consumer
  package, implemented elsewhere.

### is-it-ready-shaped verdict (full swarm not run, same as 07-19/07-22)

Live-state trend is the best this doc has recorded and the remediation-backoff system is
holding up under real load (4 stuck items, none in a runaway loop). But this pass alone found
**4 new CRITICAL + 4 new MAJOR** instances of the exact bug shape this doc has flagged as
recurring for two weeks — the individual-patch approach is not converging on zero new instances,
only on fixing the ones already found. Would read as trending toward GO if: the swallowed-error
shape gets a structural fix per the recommendation above (not just 4 more patches), and
`orphaned_triage` gets wired into automated remediation.

### Recommended Next Actions (routing per skill Phase 5)

**Shipped this same session, as parallel `Agent(isolation: worktree)` runs (all draft PRs, not
yet merged/reviewed):**

1. ~~`sdd:fix-bug` × 4, systemic pass~~ → **PR #275**: fixed 9 call sites (the 3 confirmed
   findings + 6 more of the identical shape found while building the enforcement mechanism —
   finding 3 turned out to be a structurally different problem, a missing timeout detector
   rather than a swallowed write, and was correctly scoped out rather than force-fixed), plus a
   new `tools/lint/silenttransition` `go/analysis` pass wired into `make lint-custom` that flags
   any `TransitionBacklogItemStatus`/`UpdateItemSessionEnded` call whose error is only logged —
   verified it would have caught all the fixed instances. This is the structural fix the
   "prefer systemic over instance" guidance called for, not just 4 more one-off patches.
2. ~~`sdd:fix-bug` — orphaned_triage remediation wiring~~ → **PR #274**: wired into
   `evaluateRemediation`'s backoff/parking framework via a new `TriageRespawner` interface,
   mirroring the existing `ReviewRespawner`/`abandoned_review` pattern exactly; also updated the
   `a027bc5da` exhaustiveness guard and confirmed it actually fails if the wiring is reverted.
3. ~~`sdd:quick` — GateVerdictBox/SessionMonitor/ReviewChangesModal/Settings-nav batch~~ →
   **PR #273**: also found and fixed a real structural bug beyond the original scope —
   `request_review`'s MCP-tool handler (`server/mcp/tools_backlog.go`) ignored
   `item.SkipReviewGate` entirely, unlike every other path that reaches `review`, so an item
   with the flag set could still get stranded in `review` forever depending on which path
   completed it. Fixed at the source rather than papering over it in the frontend.

**Not yet started:**

4. **`sdd:fix-bug`** — the two second-tier reconciliation MAJORs (`ReconcilePRPending`'s missing
   backoff gate, `AutoRespawnAutonomousWork`'s silent failures) — lower urgency, batchable
   together.
5. Not routed this pass, needs a decision not a fix: whether to compose
   `SkipReviewGate`/`AutoSpawnSession`/`AutoCreatePR`/`PipelineMode` into label/category-driven
   defaults (the bucket-2/bucket-3 crossover finding above) — this is a product decision about
   how much default automation to apply per item category, not a bug or a small UX fix.
6. Interface-pollution cleanup (`PipelineModeRepository`, `Repository`) — low priority,
   mechanical, no functional bug; fold into a future refactor pass rather than its own session.
7. ~~Review note: PRs #274 and this doc's own 07-27 update both touched this file on divergent
   branches~~ — resolved: merged both entries into one narrative when landing #274 on top of
   the updated `main`.
