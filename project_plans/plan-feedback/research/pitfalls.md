# Pitfalls: plan-feedback

Research for Phase 2 (Agent 4). All line numbers verified by reading the file
at the stated path in this worktree
(`stapler-squad-plan-feedback_18d5596066cbc649`) on 2026-09-14.

## 1. Orphaned/live sessions — the send-back action does not stop the running session

`tombstoneOrphanTriageSessions` (`server/services/backlog_service_triage.go:3153`)
only tombstones sessions where `is.Role == session.SessionRoleTriage`
(line 3155). It has no effect on a live `work` or `review` role session, and
it only runs from inside `TriggerTriage` — nothing in
`TransitionBacklogItemStatus` (`server/services/backlog_service_lifecycle.go:689-832`)
calls it, or anything like it, at all.

The codebase *does* have an established "stop the superseded session before
starting the next thing" primitive — `archiveItemWorkSessions`
(`server/services/backlog_service.go:1187`, soft-archives + `KillTmuxPaneOnly`
for work/review sessions) paired with `cleanupItemWorktrees`
(`server/services/backlog_service.go:1105`) — but grepping every call site
(`archiveItemWorkSessions(ctx` → `backlog_service.go:1166`,
`backlog_service_triage.go:1089`, `backlog_service_triage.go:3018`) shows it's
wired into exactly three places: `CleanupTerminalItem` (fires only when `to`
is `done`/`archived`, i.e. `session.IsTerminalStatus` —
`session/terminal_status.go:28-33` — which `ready`/`refining`/`idea` are not),
and two rework-respawn paths inside the triage package. **None of them cover
a backward transition from `in_progress`/`review`/`pr_pending`/`done` to
`ready`/`refining`.** So today's `send_back_ready` case
(`BacklogItemDetail.tsx:796-798`) already has this gap; this feature reuses
the same transition and inherits it unless it explicitly closes it.

Concretely: if an operator sends back an `in_progress` item, its work
session's tmux pane keeps running, unarchived, in its own worktree — nothing
kills or notifies it. The 2026-07-29 OOM incident (cited at
`archiveItemWorkSessions`'s doc comment, `backlog_service.go:1170-1186`) was
caused by exactly this class of leak: "dozens of superseded/completed work
AND review sessions still live." This feature reintroduces that shape for
every send-back unless it calls the equivalent tombstone/archive logic (or at
minimum `killEndedWorkSessionPanes`-style cleanup) as part of the combined
action.

**No worktree collision with the *new triage run*** — `TriggerTriage` creates
its own dedicated worktree named `triage-<itemID>` on a distinct branch
(`config.LoadConfig().BranchPrefix + "triage-"+itemID`,
`server/services/backlog_service_trigger_triage.go:427-441`), so the triage
step itself is isolated from whatever the orphaned work session is doing.

**There is a real collision risk one step later**, though: the *next* work
session spawn (after the revised plan is approved) reuses the **same**
worktree/branch slug per item — `resolveSessionPath`
(`server/services/backlog_service_triage.go:1641`) calls
`session.CreateBacklogWorktree(repoPath, slug, baseBranch)`, and
`buildRevisionTitle` (`backlog_service_triage.go:1612-1625`) documents that
"rework rounds share one worktree/branch across their -rN revisions." Before
any new spawn, `SpawnSessionFromItem` calls `tombstoneOrphanWorkSessions`
(`backlog_service_triage.go:905`, `:1720`, `:1967`, `:2178`, `:2322`) and then
checks `hasActiveWorkSession` (`backlog_service_triage.go:1175`, `:1745`) —
if the orphaned session from the send-back is *still genuinely alive*
(tmux pane not killed), `hasActiveWorkSession` returns true and the new spawn
is blocked outright, forcing the operator to separately notice and manually
restart/kill the orphan before they can act on the very plan they just asked
for. If it looks dead (process gone, tmux pane closed on its own),
`tombstoneOrphanWorkSessions` handles it fine. The failure mode is the
in-between state this feature creates: a session that's still technically
running but the operator has mentally already moved past.

**Recommendation for Phase 3**: the combined send-back+feedback action should
call the equivalent of `archiveItemWorkSessions`/`cleanupItemWorktrees` (or a
narrower "kill the pane, leave the worktree for now" variant) for the item's
most recent open work/review session as part of the same handler, mirroring
what `CleanupTerminalItem` already does for terminal transitions.

## 2. Data loss — PlanArtifactsPath reset wipes the DB pointer, not the file

The reset block in `TransitionBacklogItemStatus`
(`backlog_service_lifecycle.go:813-827`) only fires `if to ==
session.BacklogStatusIdea || to == session.BacklogStatusRefining` — it does
**not** fire for a transition to `ready`. This is the concrete evidence
behind the Rabbit Hole's recommendation: targeting `ready` (not `refining`)
as the send-back destination sidesteps this problem entirely, no design
compromise needed.

If `refining` were chosen instead (matching the dead code's literal name):
the reset sets `PlanArtifactsPath` to `""` via `UpdateBacklogItem`
(`backlog_service_lifecycle.go:817-822`) — a DB-only write. Grepping for
`os.RemoveAll`/`os.Remove(` near `triage`/`plan`/`artifact` across
`server/services/*.go` and `session/*.go` returns nothing: **the plan.md file
on disk is never deleted anywhere in this codebase.** So the operator doesn't
lose file content, they lose the DB's pointer to it — the file becomes
orphaned/unreferenced under `~/.stapler-squad/triage-artifacts/<item-id>/`
until the next `TriggerTriage` run reuses that exact same directory
(`artifactAbsPath := filepath.Join(triageBase, item.ID)`,
`backlog_service_trigger_triage.go:280`) and overwrites it in place.

One mitigating nuance: the "prior result" context a feedback-driven retriage
actually revises from does **not** come from `PlanArtifactsPath` at all — it
comes from `findPriorTriageResult` (`backlog_service_trigger_triage.go:732-744`),
which reads the `TriageResult` JSON blob stored on the most recent triage
`ItemSession` row, untouched by the `idea`/`refining` reset. So even under
the `refining` path, retriage's actual content input survives; what's lost is
purely the operator-facing "here's the current approved plan" link/file until
the next triage completes and repopulates `PlanArtifactsPath`. That's still a
real UX regression (nothing to show while retriage runs) but not the
irrecoverable data loss the field name implies.

`ApprovePlan` (`backlog_service_lifecycle.go:838-881`) independently requires
`item.PlanArtifactsPath != ""` and the file to exist on disk
(`os.Stat(item.PlanArtifactsPath)`, line 858) — another reason a `refining`
target (which blanks that field) would need retriage to complete before
`ApprovePlan` becomes usable again, vs. `ready` which never blanks it.

## 3. Cost/safety of one-click retrigger

`TriggerTriage` already has two independent, tested guards against duplicate
LLM calls, both hit before any expensive work happens:

- **In-flight guard**: `s.triageInFlight.LoadOrStore(req.Msg.ItemId, ...)`
  (`backlog_service_trigger_triage.go:238-241`) returns `CodeAlreadyExists`
  synchronously for a second concurrent call on the same item. This is the
  fix for BUG-054 (`docs/bugs/fixed/BUG-054-triage-retrigger-duplicates-genuinely-live-headless-call.md`) —
  before that fix, a retrigger while a headless call was genuinely still
  running silently tombstoned the live call's session row and started a
  redundant duplicate LLM run (confirmed live, wasted ~28 minutes of a real
  triage call). Today a double-submit UI bug would surface as a clean
  `CodeAlreadyExists` error, not a silent duplicate call — this feature
  inherits that protection for free by reusing `TriggerTriage`.
- **Orphan-aware tombstone check**: `tombstoneOrphanTriageSessions`
  (`backlog_service_triage.go:3153`) additionally returns
  `CodeAlreadyExists` if a genuinely live triage session (headless or
  tmux-backed) is found before the in-flight guard is even reached.

**Existing double-submit UI pattern to reuse**: `BacklogItemDetail.tsx` has
one `actionLoading` string state (`useState<string | null>(null)`, line 137)
set synchronously at the top of every action handler
(`setActionLoading(action)`, line 718, or the dedicated-handler equivalents
like `handleRejectPlan`'s `setActionLoading("reject_plan")`, line 994) and
cleared in a `finally`. `ActionsSection.tsx` disables **every** action button
with `disabled={actionLoading !== null}` (verified across ~15 buttons,
e.g. lines 127, 142, 156, 170, 188, 213, 238, 254, 277, 295, 307, 330, 396,
407, 418, 432) — so any button is inert while any other action is in flight,
not just itself. A new "send back with feedback" submit button must follow
this exact pattern (guard on `actionLoading !== null`, set/clear the same
state) rather than inventing new loading state, both for consistency and
because it's the mechanism that already prevents a double-click/accidental
re-click from firing two RPCs.

## 4. Concurrency / stale writes — CAS is available but the closest precedent (`send_back_ready`) doesn't use it

`transitionStatus` in the web-app hook already supports both CAS
preconditions end-to-end:
`useBacklogService.ts:665-680` — `expectedStatus` and `expectedUpdatedAt`
(the latter documented as needing the lossless `updatedAtRaw` protobuf
`Timestamp` field, not the display `updatedAt` ISO string, "the latter is
millisecond-precision and can never exactly match the server's
nanosecond-precision column"). Server-side, `TransitionBacklogItemStatus`
(`backlog_service_lifecycle.go:763-772`) builds a `BacklogItemPrecondition`
from these when set, and the underlying CAS is a single atomic conditional
SQL update, not a read-then-write race — confirmed by BUG-026
(`docs/bugs/fixed/BUG-026-backlog-transition-status-toctou-reopen.md`), a
**High-severity live incident** where a non-atomic CAS (separate `Get()` then
unconditional `UpdateOneID().Save()`) let a stale reconciliation write
silently reopen an already-shipped, `done` item and leave it permanently
stuck. The fix moved the precondition into the same SQL statement
(`session/ent_repository_backlog.go`), and `affected == 0` now surfaces as
`ErrPreconditionFailed` → `CodeAborted` (`backlog_service_lifecycle.go:784-786`).

However, **today's `send_back_ready` case is a bare call with no options**
(`BacklogItemDetail.tsx:796-798`: `await transitionStatus(item.id, "ready")`)
— it does not pass `expectedStatus`/`expectedUpdatedAt` even though the
plumbing exists and the item object already carries `item.status` and
`item.updatedAtRaw` to build them from. This feature is exactly the scenario
BUG-026 warns about in miniature: an operator can spend real time composing
feedback text in a modal while the item moves in another tab (e.g. a
background sweep or a second browser tab ships it to `done`), and without a
precondition the send-back would silently apply against whatever the row now
is. **Recommendation**: pass `expectedStatus: item.status` and
`expectedUpdatedAt: item.updatedAtRaw` on the transition call in this new
action (and note, as a low-risk drive-by, that the existing bare
`send_back_ready`/`send_back_idea` calls have the same gap and could take the
same fix).

Separately, BUG-078 (`docs/bugs/open/BUG-078-triage-in-flight-guard-races-item-ready-status-transition.md`,
still open, Low severity, CI-flake only) documents a narrower race in the
*server's own* "transition to ready, then immediately allow another
`TriggerTriage`" sequence — relevant background if Phase 3's implementation
chains `transitionStatus` → `TriggerTriage` quickly from the UI, though note
this is a different code path (BUG-078 is about a triage goroutine's own
internal cleanup racing a *new* external call) from the existing
`retriggerTriageCore` pattern (`BacklogItemDetail.tsx:705-713`:
`await transitionStatus(...); await triggerTriage(...)`) already used
successfully for "queued" items today with no reported bug.

## 5. Existing e2e test breakage

Grepped `tests/e2e/` for `send_back_ready`, `send_back_refining`, and
`"Back to Ready"` — **zero matches**. No existing Playwright spec references
either action by name or by its rendered button label today. This lowers the
risk flagged in requirements.md's Feasibility Risks section — there is no
e2e coverage of `send_back_ready`'s bare-transition behavior or
`send_back_refining`'s dead-button state to update; new tests can be added
without needing to first locate and rewrite conflicting ones. (This doesn't
rule out coverage via a more generic action-menu snapshot test — worth a
final grep for the action's toast strings, e.g. `"Sent back to"`, during
Phase 5 implementation, but nothing surfaced in this pass.)

## 6. Known bugs/precedent near triage retrigger, plan rejection, send-back

Directly relevant, found via `grep -rl "BUG-0" docs/` cross-referenced with
triage/retrigger/plan-reject/refining keywords:

| Bug | Status | Relevance |
|---|---|---|
| BUG-026 — stale reconciliation writes could reopen an already-shipped item | Fixed | The CAS-precondition case for point 4 above; direct precedent for why this new action should pass `expectedStatus`/`expectedUpdatedAt`. |
| BUG-054 — triage retrigger duplicates a genuinely live headless call | Fixed | Why `TriggerTriage`'s in-flight guard is safe to rely on for point 3's double-submit concern. |
| BUG-078 — `triageInFlight` clear races the item's `ready` transition | Open, Low severity, CI-flake only | Background for chaining transition→retrigger quickly (see point 4). Not blocking — same shape as the already-working `retriggerTriageCore`. |
| BUG-065 — shutdown-orphaned triage sessions missing shutdown attribution | Fixed | Same subsystem (`tombstoneOrphanTriageSessions`); not directly implicated but explains the `reason == "shutdown"` branch visible in that function (backlog_service_triage.go:3183-3190) — worth knowing if a send-back races a server restart. |
| ADR-002 (`project_plans/plan-approval-ux/decisions/ADR-002-reject-plan-manual-retrigger.md`) | Design precedent, not a bug | Already cited in requirements.md; explicitly rejected auto-triggering retriage as a side effect of a status-only write, for the same in-flight/orphan-session/concurrency-semaphore-guard reasons surfaced in points 1, 3, and 4 above — this research corroborates that those guards are real and worth keeping in the explicit two-RPC-calls-from-one-click shape ADR-002's precedent uses, rather than moving them server-side into `TransitionBacklogItemStatus`. |

No bug doc specifically named "plan rejection" or "send-back" beyond
ADR-002's design rationale and the ones above.

## Summary of design implications for Phase 3

1. The combined action must explicitly stop the item's live work/review
   session (tmux pane at minimum — mirror `archiveItemWorkSessions`) as part
   of the same handler; nothing existing does this for a backward
   transition to `ready`/`refining`, and leaving it out reproduces the
   2026-07-29 OOM leak shape and will block the next `spawn_session` via
   `hasActiveWorkSession` until the operator separately intervenes.
2. Target `ready`, not `refining`, per the requirements' Rabbit Hole —
   confirmed by code: the `idea`/`refining` reset block
   (`backlog_service_lifecycle.go:813-827`) never fires for `ready`, so
   `PlanArtifactsPath` (and `ApprovePlan`'s file-on-disk precondition)
   survive untouched, and no `TriggerTriage` guard change is needed
   (`ready` is already accepted, `backlog_service_trigger_triage.go:184`).
3. Reuse the `actionLoading` guard/disable pattern verbatim — it's already
   the repo's standard double-submit defense, and `TriggerTriage`'s
   server-side in-flight guard is a proven second line of defense (BUG-054).
4. Pass `expectedStatus`/`expectedUpdatedAt` on the transition call — the
   plumbing already exists end-to-end and BUG-026 is a concrete High-severity
   precedent for why an unconditional write here is risky, even though
   today's `send_back_ready` doesn't currently do this either.
5. No e2e tests need updating for this change; new coverage is purely
   additive.
