# Research: Features — backlog-session-thrashing

## 1. Existing dedup / WIP-limit mechanisms (already implemented — not a greenfield problem)

The codebase already has a surprisingly mature multi-layer defense against duplicate work
sessions. This is NOT a "build from scratch" problem; it's an audit-and-close-the-gaps problem.

### Layer 1 — `hasActiveWorkSession` (the core liveness check)
`server/services/backlog_service_triage.go:882-891`. Pure function: true iff any
`ItemSessionSummary` has `Role == SessionRoleWork && EndedAt == nil`. "Live" is defined purely
as **an open DB row**, not "tmux pane confirmed alive" — this is the single biggest coverage
gap (see §3 below, "DB says live, tmux is dead"). Used as a hard gate in `spawnSessionAfterGates`
(step 8b, line 658) — returns `CodeAlreadyExists` if a work session is already open for the item.
Also reused for the same purpose in `AutoReopenAfterFailedReview` (line 1146),
`AutoRespawnAutonomousWork` (line 1292), `AutoReopenForPRFix` (line 1432), and
`AutoRespawnReview` (line 1563) — i.e. every automated respawn path funnels through this same
check before attempting a new spawn.

### Layer 2 — `spawnInFlight` (closes the TOCTOU race Layer 1 has on its own)
`server/services/backlog_service.go:138-164` + `SpawnSessionFromItem` step 1b
(`backlog_service_triage.go:354-368`). A `sync.Map` keyed by item ID, `LoadOrStore`/`Delete`
(same idiom as `review_queue_manager.go`'s `autoCreatePRInFlight`). Rationale documented inline:
`hasActiveWorkSession`'s read (`ListItemSessions`) → check → write (`CreateItemSession`)
sequence is not atomic, so two concurrent `SpawnSessionFromItem` calls (autonomous-driver
respawn racing a periodic reconciliation sweep, or a manual retrigger) can both observe "no
active session" before either inserts its row. **Confirmed live 2026-07-19**: item `d3227302`
had two literal overlapping "work" role `ItemSessions`. This is exactly the "concurrency/
duplication" failure mode named in the requirements doc — it already has a documented incident
and a fix. Whether that fix has a coverage gap (e.g. does it cover every spawn call site, or
only `SpawnSessionFromItem`?) needs verification in the architecture/pitfalls research
dimension — worth double-checking `AutoRespawnAutonomousWork` et al. actually route through
`SpawnSessionFromItem` (they appear to, per the grep hits, but should be confirmed by reading
each function body end-to-end).

### Layer 3 — `countLiveBacklogWorkSessions` (global WIP cap, not per-item)
`backlog_service_triage.go:846-880`. Counts items in `in_progress` OR `review` status where a
work session is still open — the `review`-status branch exists specifically because
`AutoReopenAfterFailedReview` intentionally leaves a work session alive (polling for a verdict)
after the item flips back to `review`; a naive `in_progress`-only count silently undercounts and
lets the cap be exceeded. Enforced in `SpawnSessionFromItem` step 4 (line 411-422): at cap, item
is queued (`BacklogStatusQueued`) rather than rejected; `DequeueNextQueuedItems`
(line 492-589, single-flighted via `dequeueMu`) drains the queue FIFO as slots free up. Cap is
`config.Config.MaxConcurrentBacklogWorkItemsOrDefault()` (default 2), added 2026-07-12 after a
kernel OOM from too many concurrent agent sessions — this is orthogonal to (not a fix for) the
per-item duplication problem; it caps aggregate concurrency, not per-item exclusivity.

### Layer 4 — orphan/dead-session cleanup before the guard runs
`tombstoneOrphanWorkSessions` (called at `spawnSessionAfterGates` step 8a, line 646) — ends any
work session that "never reached its normal completion path" (crash, kill, server restart
mid-session) before the `hasActiveWorkSession` check, specifically because a single dead session
would otherwise block every future spawn attempt forever. Cites a live incident: `AutoReopenForPRFix`
retried every ~60s against the same dead session for hours. `killEndedWorkSessionPanes` (step
8a2) additionally closes tmux panes for already-ended rounds. **This is the closest existing
piece of "is it live" logic that goes beyond a raw DB-row check** — but it appears to run only
at spawn time, not continuously, and its exact "orphan" criteria need to be read in full (not
just grepped) to know if it already solves edge case #3 in §3, or only partially.

### Turn-budget mechanism (already has real design history, not naive)
- `AutonomousDriver` (`session/autonomous_driver.go`): `maxTurns` defaults to a **hardcoded 20**
  (`NewAutonomousDriver`, line 66-69) — this is the literal "~20-turn budget" the requirements
  doc calls insufficient. The loop (`run`, line 192) increments a plain counter; on exhaustion
  without a `DONE` signal it returns `AutonomousDriverOutcome{Stuck: true, Reason: "max turns
  reached", Turns: maxTurns}` (line 270-277). No adaptive/progress-based extension exists at
  this layer — it is a pure fixed count.
- **Prior fix (PR #222, commit d2b57fc9)**: `onAutonomousDriverComplete`
  (`server/services/autonomous_orchestration_service.go:325-427`) used to force
  `in_progress → review` whenever the orchestrator claimed `Done=true`, even though the
  orchestrator's completion judgment is admitted (in the code's own comments) to be **far
  weaker** than the real completion signal: the `request_review` MCP tool, which requires the
  work session's own agent to decide the goal is met and rejects uncommitted changes. Confirmed
  live 2026-07-24/25: the orchestrator hallucinated `DONE` ~10 minutes into a still-running SDD
  workflow after only a `requirements.md` commit, forcing a premature review against an empty
  diff while the real session kept working and landed real fixes 40+ minutes later — a stale
  FAIL verdict had already been recorded by then. The fix: if `request_review` was never called,
  leave the item `in_progress` and do nothing (no status change, no second competing driver).
- **Turn-cap-without-DONE handling** (same function, `SessionRoleWork` branch, line 326-384):
  does NOT force review either (same doomed-review rationale, plus a cited historical circuit-
  breaker gap: "a live item bounced 78 times in 24h" before a working circuit breaker existed).
  Instead: `MarkStuck(StuckReasonAutonomousStuck)` (durable, visible in the Unfinished tab) +
  `AutonomousStuckRespawner.AutoRespawnAutonomousWork` — but only if `RemediationDue` (the
  shared exponential backoff gate, see below) says an attempt is due; otherwise it silently
  skips the respawn this tick and waits.
- **Shared remediation backoff gate** (`session/backlog_remediation.go`): every automated
  respawn/retry across ALL `StuckReason`s (not just `autonomous_stuck`) goes through
  `Storage.RemediationDue`, backed by a per-(item, reason) `BacklogStuckState` row tracking
  `remediation_attempts` and `next_remediation_at`. Schedule:
  `30m → 2h → 8h → 24h → 72h`, then **park** (stop auto-retrying, one final notification,
  requires manual `ResetStuckRemediation`/`BulkResetStuckRemediation`). Deliberately sized large
  (revised from an original 5m/15m/1h/4h/12h draft) because a meaningful fraction of "failures"
  are actually the service getting OOM-killed and systemd-restarted — a `restart-grace` check
  (`evaluateRemediation`, `remediationGrantedRestartGrace`) gives one free non-budget-consuming
  attempt per boot for exactly this reason. This means "turn-budget exhaustion" and
  "concurrency/duplication" are **not independent problems** in this codebase — they already
  share one throttle: the same backoff gate that prevents duplicate-spawn storms is what paces
  turn-cap respawns.
- **Separate, smaller cap on the review-side auto-rework loop**: `maxAutoReworkIterations`
  (`server/services/backlog_service.go:278-280`, config default 3, distinct from the 5-attempt
  remediation cap) — `StuckReasonReworkCap` fires when this is hit
  (`session/domain/backlog.go:42-44`). Two different caps for two different loops (review/rework
  cycling vs. autonomous-driver turn-cap-without-DONE respawns) that both terminate into the same
  general "stuck, needs a human" bucket — worth flagging in the design phase as a possible
  simplification target or an intentional separation to preserve (they gate structurally
  different loops, so merging them may not be correct — flag, don't resolve, here).

### The "stuck" state model already exists and is fairly mature
`session/domain/backlog.go` defines `StuckReason` as a closed, validated enum (12 members today:
`pr_ready_unmerged`, `rework_cap`, `abandoned_review`, `stale_work`, `bouncing`, `push_failed`,
`orphaned_triage`, `autonomous_stuck`, `spawn_failed`, `plan_not_approved`, `pr_pending_no_pr`,
`rework_blocked_stale`). Each has a `MarkStuck`/`ResolveStuck` lifecycle backed by a durable
`BacklogStuckState` row, surfaced in an "Unfinished" tab in the UI, and (per
`backlog_remediation.go`) increasingly wired to the shared backoff gate. This is very likely
**already the "one well-defined, operator-visible stuck state" the requirements doc asks for** —
the open question for the design phase is not "build a stuck state" but "is `autonomous_stuck`
specifically well-defined enough, and are there still silent-failure paths that bypass this
system entirely" (see §3).

## 2. Overlapping in-flight planning work

The requirements doc names several `project_plans/`/`docs/tasks/` directories as possibly
overlapping. Checked in this worktree:

- `project_plans/headless-review-notifications/`, `project_plans/headless-session-notifications/`,
  `project_plans/review-session-notifications/`, `project_plans/headless-notification-cleanup/`,
  `docs/tasks/notifications-for-headless-review-triage-sessions-...-never-cleaned-up/` —
  **none of these directories exist in this worktree** (confirmed via direct filesystem lookup,
  not just `ls`/grep miss). They appear in this session's `git status` as untracked (`??`)
  entries, which normally means "present on disk but not in git" — but they are not present on
  disk here either. Most likely explanation: they belong to a different worktree or a since-
  cleaned-up session, and the git-status snapshot handed to this research task is stale relative
  to this worktree's actual contents. **No content to review for overlap.** Flag this
  discrepancy back to whoever assembles the final plan — if that notification work is real and
  live elsewhere, it should be located and checked for overlap before this project finalizes its
  design (a "stuck" notification and a "duplicate session" fix are adjacent surface area).
- `docs/tasks/backlog-stuck-item-auto-remediation.md` — **directly relevant, not overlapping-by-
  coincidence but the actual design doc for the remediation-backoff machinery described in §1**.
  Read in full for the backoff schedule rationale (OOM-restart bursts) and the admin
  reset RPCs (`ResetStuckRemediation`, `BulkResetStuckRemediation`).
- `project_plans/backlog-stuck-item-visibility/` — exists, has a full requirements/research/
  plan/decisions set including `ADR-001-durable-stuck-state-storage-model.md`. This is almost
  certainly the project that built the `BacklogStuckState` durable-row model this project's
  design should build on top of, not duplicate. Worth a full read in the architecture research
  dimension.
- `project_plans/review-gate-stale-session-rework/` — exists, has
  `ADR-001-staleness-threshold-recalibration.md` (the source of the 15-minute
  `maxReworkBlockStaleness` constant vs. the 2-hour `maxWorkSessionStaleness` vs. the Review
  Queue's 5-minute `StalenessThreshold` — three different staleness thresholds for three
  different urgency levels, already deliberately differentiated, documented inline at
  `backlog_service_triage.go:893-904`). Relevant precedent for how this project should think
  about "genuinely stuck" vs. "slow but alive" (see §5).
- **`docs/bugs/open/BUG-042-...md`, cited in this project's own `requirements.md` as adjacent
  context, does not exist** in this worktree. `docs/bugs/` jumps `BUG-041 → BUG-043` with no
  042 anywhere (fixed or open). Either the bug was renumbered/consolidated, the file was deleted
  after being fixed without a corresponding "fixed/" rename, or the reference in `requirements.md`
  is simply wrong. Worth a one-line correction or footnote in the final plan since a reader will
  otherwise go looking for a file that isn't there.

## 3. Edge cases a "one worker per item, adaptive budget" design needs to handle

Ranked roughly by how much existing code already covers them vs. leaves open:

1. **Two orchestration passes race on the same item (TOCTOU on the liveness check).**
   Already handled for the `SpawnSessionFromItem` entry point via `spawnInFlight`
   (§1, Layer 2) — confirmed via a real 2026-07-19 incident. Verify in the architecture pass
   whether every respawn call site actually funnels through that same guarded function or
   whether any bypass it directly against storage.

2. **DB says a work session is "live" (`EndedAt == nil`) but the tmux pane/process is actually
   dead.** This is the sharpest edge in the current design. `hasActiveWorkSession` is a pure DB
   predicate with zero tmux/process awareness. The mitigations that exist
   (`tombstoneOrphanWorkSessions`, `reconcileStaleWorkSessions`'s
   `Instance.TmuxAlive`/`PaneProcessDead` check per `session/backlog_lifecycle.go:58-69`) run on
   a schedule/at spawn time, not synchronously inside the guard itself — so there is necessarily
   a window between "the process died" and "the next sweep notices," during which
   `hasActiveWorkSession` still reports true and blocks a legitimate respawn. Whether that window
   is acceptable or needs tightening (e.g. checking `TmuxAlive` directly inside the spawn guard,
   not just in a separate periodic reconciler) is a live design question.

3. **A session that is alive (tmux pane exists) but producing zero real output — "technically
   live, not actually working."** Explicitly called out and partially solved already:
   `notifyIfActiveWorkSessionStale` (`backlog_service_triage.go:906-995`) distinguishes this from
   a genuine crash and — by explicit repo policy — **never kills the session automatically**; it
   only notifies + marks `StuckReasonReworkBlockedStale`. The repo has apparently already burned
   itself once on "force-stop a slow-but-alive agent" (referenced: "the stop_session-deletes-
   branch incident") and treats auto-killing as strictly worse than leaving a stale session
   running. Any new adaptive-budget design must preserve this policy, not silently reintroduce
   auto-kill.

4. **Operator manually resumes a paused/stopped session.** Not directly covered by anything found
   in this pass — `pause_session`/`resume_session` (MCP tools) were not traced end-to-end in this
   research pass. Open question for architecture research: does a manual resume re-enter
   `hasActiveWorkSession`'s "active" bucket cleanly (good — it should, since the row's `EndedAt`
   presumably stays nil across a pause), or does it risk double-counting against the WIP cap or
   re-triggering a stuck-remediation attempt that's now moot? Needs tracing in the architecture
   dimension, not resolved here.

5. **Backlog item re-triaged / edited while a worker is still active.** Not found in this pass —
   e.g. does changing acceptance criteria or repo_path on an `in_progress` item with a live
   worker get blocked, silently ignored by the running session, or cause a divergence between
   the DB row and what the agent is actually working from? `spawnSessionAfterGates` step 7 takes
   an `acSnapshot` at spawn time (line 630-631) — implies AC changes after spawn are NOT
   propagated to the running session, which may be intentional (snapshot semantics) but should be
   confirmed as a deliberate decision, not a gap, before this project's design assumes either way.

6. **Orchestrator poll/LLM call itself errors or times out** (distinct from the session it's
   driving crashing). `AutonomousDriver.run` (line 210-214) breaks out of the loop on any
   `CallBlocking` error from the headless pool, falling through to the same "not done" outcome
   path as a genuine turn-cap exhaustion (`Stuck: true`, but with `Turns: turnCount+1`, not the
   full `maxTurns` — a subtle but real signal difference between "ran out of budget" and "the
   orchestrator itself broke," currently collapsed into the same `Reason` string ambiguity by the
   caller). Whether callers should treat "LLM call failed" differently from "genuinely exhausted
   budget" (e.g. retry sooner, since it's likely infra flakiness, not task difficulty) is an open
   design question — right now both produce an identical `onAutonomousDriverComplete` code path.

7. **Malformed orchestrator responses** (`parseOrchestrationResponse` fails to parse `NEXT_MESSAGE:`/
   `DONE:`) are retried in-loop (line 216-221, `continue`) without consuming a "wasted" turn
   signal distinctly — they still count against `maxTurns` implicitly since the outer `for`
   loop's counter isn't decremented, but the retry itself doesn't inject a turn into the driven
   session. `malformedResponseCount` is tracked and surfaced in the final stuck reason string
   only — not used to shorten/extend the budget or distinguish "the target session is stuck" from
   "the orchestrator LLM is producing garbage." Worth surfacing as a candidate signal for a
   progress-based termination heuristic (see §4).

8. **A session crashes vs. hits budget vs. completes normally** — these three outcomes are
   already reasonably well distinguished in `AutonomousDriverOutcome` (`Done`, `Stuck`, `Reason`,
   `Turns`) and handled with different downstream logic per role
   (`onAutonomousDriverComplete`'s big switch). The gap is less "are these distinguished" and
   more "is 'stuck' itself further distinguishable into 'ran out of turns while genuinely
   progressing' vs. 'was actually stuck the whole time'" — which is precisely the requirements
   doc's stated problem, and nothing in the current code attempts that distinction; `Turns` is
   the only quantitative signal available today, with no notion of "progress per turn."

## 4. How comparable systems handle single-worker-per-task + adaptive budgets

**Exactly-once / single-worker guarantees (task queues, CI schedulers):** The dominant pattern is
a **lease with a heartbeat and a TTL**, not a boolean "is it running" flag. A worker claims a task
by writing a lease (owner ID + expiry), and must periodically renew it while working; if the lease
expires without renewal, any other worker may reclaim the task, and the original worker (if it
wakes up later) is expected to detect it lost the lease and abandon its work rather than commit
results. This is the mechanism that directly solves edge case #2 above (DB says live, process is
dead) — a lease naturally expires instead of requiring an external process to notice and clean
up a specific dead-session shape. Kubernetes Jobs, Celery's visibility-timeout-based redelivery,
and SQS's visibility timeout are all instances of this pattern. This codebase's current
`hasActiveWorkSession` + periodic tombstone sweep is functionally an approximation of a lease
with a very long, implicit TTL (however long it takes the next reconciliation tick to notice) —
worth naming explicitly as the pattern being approximated, since it clarifies what "tighten the
window" in edge case #2 would actually mean concretely (shorten the effective TTL, or make
renewal explicit via a heartbeat write on every turn/turn-callback).

**Adaptive/progress-based budgets:** CI systems (e.g. Buildkite, GitHub Actions) mostly use a
flat wall-clock timeout, not a step-based adaptive budget — turn-count budgets are more specific
to LLM agent orchestration. Publicly documented approaches from agentic coding tools converge on
a few recurring ideas, worth treating as candidate patterns rather than prescriptions:
- **Progress checkpoints instead of (or alongside) a raw step count** — terminate/escalate based
  on "no meaningful state change (no new commits, no new file writes, no tool calls with real
  side effects) for N consecutive turns" rather than "N turns have elapsed." This maps directly
  onto this codebase's existing `TimeSinceLastMeaningfulOutput` concept
  (`notifyIfActiveWorkSessionStale` already uses exactly this signal, just for a different gate)
  — reusing the same "meaningful output" definition for the turn-budget decision, not just the
  stale-notification decision, is a natural extension to flag for the plan phase.
  - **Cost/step budget with graduated checkpoints** rather than one hard cliff: e.g. warn or
  reduce autonomy at 50%/80% of budget, hard-stop only at 100%+some grace — softens the binary
  "20 turns and done" cutoff into a curve. This codebase's remediation backoff schedule
  (30m/2h/8h/24h/72h) is already this same "graduated escalation" shape, just applied to *retry
  spacing* rather than *turn budget sizing* — the same idea could plausibly extend to the turn
  cap itself (e.g. 20 → 40 → 80 turns across successive genuinely-progressing respawns, capped).
- **Distinguish "no signal" from "negative signal."** A step that produces no new output at all
  is different from a step that actively errors or contradicts a prior step (e.g. a test that
  was passing now fails) — some agentic frameworks weight these differently when deciding whether
  to keep going, since "quiet but working" and "flailing" look identical to a pure turn-counter
  but very different to an output-diff-aware heuristic.

Kept intentionally brief per scope — this is meant to surface transferable vocabulary
(lease/heartbeat, progress-checkpoint, graduated budget) for the architecture/plan phases, not to
be an exhaustive survey of Devin/OpenHands/SWE-agent internals (none of which publish enough
implementation detail publicly to verify specifics beyond the general pattern names above).

## 5. Unstated operator needs implied by the problem statement

- **"Why was this killed?" visibility, distinct from "that it was killed."** The existing
  `StuckReason` enum + `MarkStuck` reason-string free text already captures a why per-occurrence
  (e.g. `"autonomous driver stopped after %d turns without a DONE signal (%s)"` — the `%s` being
  the driver's own `Reason` field). The open gap is less "is there a reason string" and more
  "is the reason string good enough to distinguish 'genuinely stuck' from 'ran out of turns while
  clearly still making progress'" — today `Reason` is always some variant of "max turns reached"
  with no progress signal attached, so an operator reading the Unfinished tab cannot currently
  tell those two cases apart without opening the session transcript themselves.
- **Distinguishing "genuinely stuck" from "slow but progressing"** is the explicit success
  metric in the requirements doc and, per §3/§4, is the one dimension with essentially zero
  existing signal today (only turn count, no diff/output-based progress measure). This is very
  likely the single highest-value net-new capability this project should design, since nearly
  everything else (dedup, backoff pacing, stuck-state visibility, notify-on-stale) already has a
  mature implementation to build on rather than invent.
- **Trusting the orchestrator's own "done" judgment less, not more, over time.** The PR #222 fix
  and its surrounding comments show the team already moving in the direction of "the orchestrator
  LLM's raw judgment is a weak signal; prefer the agent's own explicit protocol
  (`request_review`)" — an implicit operator need (or at least an engineering-team need) is a
  turn-budget/stuck design that leans further into structured completion signals from the worked-
  on session itself (tool calls, commits, `request_review`) rather than adding more LLM-judged
  heuristics on top of the same weak orchestrator-observation channel (a raw terminal-tail
  snapshot, per `buildOrchestrationPrompt`).
- **Bulk recovery after infrastructure incidents, not just per-item recovery.** The remediation
  backoff design already anticipated this (`BulkResetStuckRemediation`, restart-grace passes) —
  worth preserving/extending rather than reinventing per-item-only tooling for any new stuck
  state this project introduces.
