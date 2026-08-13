# Research: Pitfalls — backlog-session-thrashing

Scope: what has ALREADY gone wrong (bugs, incidents, prior fixes) in this exact
subsystem, plus generic pitfalls for the class of problem (LLM-orchestrated
autonomous worker turn budgets + worker dedup). Research only — no fix design.

---

## 1. BUG-042 — confirmed absent, citation in requirements.md is stale

```
$ find docs/bugs -iname "BUG-042*"
(no output)
$ ls docs/bugs/fixed/ | grep -E '^BUG-0(4[0-9])'
BUG-040-pr-pending-item-loses-pr-reference-dead-end.md
BUG-041-backlog-nudge-retry-never-backs-off.md
BUG-043-chronic-abandoned-review-respawn-failures.md
BUG-044-unbounded-pr-branch-drift-from-main.md
...
```

`docs/bugs/open/` contains only `review-queue-gaps.md` (not a numbered bug).
`docs/bugs/fixed/` numbering jumps `BUG-041 → BUG-043` with no 042 anywhere,
open or fixed. This matches the sibling agent's independent finding
(`project_plans/backlog-session-thrashing/research/features.md`, its own §2)
via a different verification method (this pass used `find -iname`, that pass
used `ls`/grep) — both confirm the same negative. **The requirements.md
citation of BUG-042 is stale/wrong; there is no evidence it was ever filed
under that number.** Treat any reasoning in requirements.md that assumed
BUG-042's content as unsupported.

The four notification-related directories cited in requirements.md
(`project_plans/headless-review-notifications/`, `headless-session-notifications/`,
`review-session-notifications/`, `headless-notification-cleanup/`, plus
`docs/tasks/notifications-for-headless-review-triage-sessions-...`) also do
not exist in this worktree, confirmed independently. Both stale citations
point the same direction: requirements.md's context section was assembled
against a different worktree/session state than the one this project is
actually running in.

---

## 2. Timeline of prior turn-cap / dedup / stuck-detection work (git log)

Chronological, oldest first, scoped to what's directly relevant:

| Date | Commit(s) | What it did |
|---|---|---|
| 2026-07-12 | `91063c43`/`425cbbed` (#150) | First WIP cap: hardcoded `maxConcurrentBacklogWorkItems=2`, added directly after a kernel OOM (57GB/61GB used, swap exhausted) from too many concurrent agent sessions. Rejects fresh spawns with `CodeResourceExhausted`. |
| 2026-07-19 | `0f7d167a`/`55d7ba23` (#182) | Closed the **duplicate-work-session TOCTOU race**: `spawnInFlight` sync.Map guard around `SpawnSessionFromItem`. Confirmed live incident: item `d3227302` had two literal overlapping work-role `ItemSession`s because the read→check→write sequence held no lock. |
| 2026-07-19 | `dd3a287f`/`b6f2a9a0` (#180) | Large multi-part PR. Root-caused a **live CRITICAL bounce loop**: one item bounced `in_progress↔review` **up to 78 times in 24h**, burning full 20-turn autonomous sessions each cycle, because (a) `onAutonomousDriverComplete` forced `in_progress→review` even on a turn-cap stop with no DONE signal, (b) the resulting no-verdict review was invisible to the repeated-failure circuit breaker (`IsRepeatedFailure` needs 2 `ReviewVerdict` rows; a review that writes none is invisible to it), (c) the rework cap itself (raised 3→20 earlier in the same PR) wasn't the binding constraint — 18 real work sessions existed against a cap of 20. Fix: don't transition on a turn-cap stop; respawn via `AonomousStuckRespawner` gated by rework cap; add `IsRepeatedNoVerdictFailure` to close the blind spot. Also added the `StuckReasonAutonomousStuck` durable row (turn-cap visibility) and the "conversions limit" UI surfacing. |
| 2026-07-21 | `91063c43` superseded by `44f77e0b` (#199) | WIP cap made configurable (Settings, clamped [1,10], default still 2) and behavior changed from **reject** to **queue**: hitting the cap now transitions the item to a new `queued` status (FIFO), auto-dequeued as slots free (`onSessionExited` + 60s `ReconcileStuck` safety net). |
| 2026-07-21 | `8e34bbdb` | Code-review follow-up on #199: closed a **plan-gate bypass** (an unapproved-plan item could be queued and later dequeued straight into a spawn with no planning check), a **dequeue race** (unsynchronized `onSessionExited` exit-hook racing the periodic sweep — both could compute `freeSlots` independently and over-dequeue), and config staleness. |
| 2026-07-19/22 | `b0f26785` (#185) | Phase A: the **shared exponential backoff gate** (`session/backlog_remediation.go`), `30m→2h→8h→24h→72h` then park, applied uniformly across every `StuckReason`. Explicitly sized large because "a meaningful fraction of failures are actually the service getting OOM-killed and systemd-restarted" — includes a one-free-attempt-per-boot restart grace. |
| 2026-07-22 | `2e5f8da4` | Split autonomous-driver turn injection into separate paste+submit writes (adjacent robustness fix, not turn-budget sizing). |
| 2026-07-22 | BUG-041 fixed (`session/session_driver.go`) | **392 consecutive failed nudge-sends over ~13 minutes** against a dead tmux pane — a one-shot "nudge sent" latch was only set on the *success* branch of a fallible `SendKeys` call, so a permanent failure (dead pane) retried at full tick speed forever with no backoff and no escalation. Root-cause shape: "guard variable only updated on the happy path of a fallible operation." |
| 2026-07-23 | BUG-043 filed then live-traced and fixed (`e7c82802` → `6316674f`) | **Originally filed hypothesis was wrong** (see §3 below) — real cause was a cross-gate coordination gap, not a `SendKeys`/dead-pane issue. |
| 2026-07-24 | BUG-046 fixed (`session/backlog_lifecycle.go`) | A **notification-spam / wasted-work** bug: `reconcileUnprocessedReviewVerdicts` reprocessed the same dead review session on every 60s tick because it had no way to distinguish "already handled, just gated by backoff" from "never handled" — one item's dedup'd notification reached `occurrence_count: 95` over 94 minutes. |
| 2026-07-24 | BUG-048 fixed (`server/services/autonomous_orchestration_service.go`) | A **review-role `autonomous_stuck` row had no remediation path at all** — `next_remediation_at` could sit hours overdue with nothing ever checking it, because only the work-role branch of `onAutonomousDriverComplete` called `RemediationDue`. Root cause went deeper than the initial filing suspected: `AutonomousDriver.run` never kills the underlying session on a stuck turn-cap exit, so the session never looks "ended," which means **none** of the three candidate responders (`bouncing`, `abandoned_review`, the stale-work sweep) could ever see it — all three require an inactivity signal nothing produced. |
| 2026-07-25 | **`d2b57fc9` (#222)** | **The PR named in requirements.md as related context.** See full analysis in §4. |

---

## 3. BUG-043 as a cautionary tale: the *filed* root cause was wrong

BUG-043 ("chronic `abandoned_review` respawn failures") was filed 2026-07-23
with a specific hypothesis: a `SendKeys`/`SessionDriver` initial-prompt
injection race mirroring BUG-041's dead-pane shape. **A live trace the same
day proved this hypothesis false.** The actual respawn path
(`TriggerReReview`'s headless branch) makes a direct, synchronous LLM call —
no tmux, no `SendKeys` anywhere in that flow — and correctly produced a fresh
FAIL verdict on every attempt. The real defect was that the verdict's only
consumer (`autoReopenWithBackoffGate`) was gated by a **separate**,
independently-clocked `bouncing` backoff that was already deep in its own
cooldown, so `abandoned_review` burned its own 5-attempt budget on
identical, silently-discarded verdicts.

**Relevance to this project**: this is a documented instance of exactly the
class of bug the requirements doc flags as poorly understood — "the system's
response to hitting [a] cap ... is not well understood or well designed."
Two independently-owned gates/caps (here: `abandoned_review`'s attempt
budget and `bouncing`'s backoff clock) governing overlapping conditions,
neither aware of the other's state, is a recurring shape in this codebase —
see also BUG-048's finding of the same "gate exists, nothing revisits it for
this specific exit path" family, and BUG-046's "no way to distinguish
already-handled-but-gated from never-handled." **Any turn-budget/dedup
redesign for this project must explicitly enumerate every gate/cap that
already exists (WIP cap, spawnInFlight, rework cap, remediation backoff,
autonomous turn cap, maxAutoReworkIterations) and state how the new
mechanism composes with each — do not add a fifth uncoordinated clock.**

---

## 4. PR #222 (`d2b57fc9`) — exact diff and what it does/doesn't claim

Confirmed via `git show d2b57fc9 --stat` (2 files changed: `server/services/autonomous_orchestration_service.go` +43/-2, plus a test file +63) and full diff read.

**What it changed**: in `onAutonomousDriverComplete`'s `SessionRoleWork`
case, when the orchestrator LLM (a separate, minimal judge with no
visibility into acceptance criteria, diff state, or whether `request_review`
was called — only a raw terminal-tail snapshot) reports `Done=true`, the
code **no longer** forces `toStatus = BacklogStatusReview`. Root cause cited
in the commit message: the orchestrator hallucinated DONE ~10 minutes into a
still-running SDD workflow after nothing but a `requirements.md` commit,
forcing a premature review against an empty diff, while the real session
kept working and landed real fixes 40+ minutes later — a stale FAIL verdict
had already been recorded by then, confirmed on **two independent items in
one session**.

**What it explicitly does NOT touch**: this fix is scoped only to the
`outcome.Done == true` branch of the work-role case. It does not touch the
turn-cap-without-DONE branch (that logic — `RemediationDue` +
`AutoRespawnAutonomousWork` — predates this PR, from `dd3a287f`/#180). The
commit message's own comment explicitly separates the two concerns: "The
orchestrator's Done reply is still real evidence the driver itself isn't
stuck (looping/hitting the turn cap) ... independent of whether we trust
'DONE' to mean 'ready for review'" — i.e. PR #222 deliberately keeps
`resolveAutonomousStuck` firing on a Done signal even though it no longer
lets that signal drive the status transition.

**Does the PR description or code comments flag turn-cap/duplication as a
known follow-up?** No explicit "TODO: fix turn cap next" statement exists in
the commit message or the added code comments. The commit message frames
this as a complete fix for its specific root cause (orchestrator
hallucinating DONE), not as a partial mitigation. However, the *problem
space* is clearly on the author's mind: the added comment explicitly
contrasts the orchestrator's weak judgment against `request_review`'s
stronger signal, and the requirements.md for this project independently
frames turn-cap exhaustion as the sibling unsolved problem — but that
framing comes from this project's own requirements doc, not from PR #222
itself. **Conclusion: PR #222 does not self-identify turn-cap/duplication as
its own follow-up; the connection is this project's inference, not the
prior PR's stated intent.**

---

## 5. Sibling project plans — directly relevant prior research (skimmed in full)

### `project_plans/backlog-stuck-item-visibility/research/pitfalls.md`
Pre-dates BUG-046/048 but predicts their exact shape:
- §1 flags that `TransitionBacklogItemStatus` is check-then-act (TOCTOU) in
  application code, not a single `UPDATE ... WHERE` — "this existing gap is
  the backdrop any new 'mark as stuck' write lands on." Directly relevant if
  this project adds any new durable state for turn-budget tracking.
- §3 flags "notification storm on first reconcile pass after a migration" —
  relevant if this project changes what counts as "stuck" and a backfill
  pass would re-flag every currently-affected item at once.
- §1 also flags: stuck-state clearing must be wired into *every* code path
  that moves an item off the stuck status, not just the periodic tick — "if
  clearing only happens on the next 60s tick's 'not stuck anymore' branch,
  there's up to a 60s window where the UI shows stale state." Confirmed
  materialize in this exact shape by BUG-046 (fixed after this pitfalls doc
  was written).

### `project_plans/review-gate-stale-session-rework/research/pitfalls.md`
- §1: three different staleness thresholds already coexist in this codebase
  for structurally similar "is this session stuck" questions — `2 min`
  (Review Queue badge), `5 min` (proposed, review-queue-state-detection's
  own FR-3), `2 hours` (`maxWorkSessionStaleness`), and (per that project's
  own fix) a fourth, `15 min` (`maxReworkBlockStaleness`). Explicitly warns:
  "three different subsystems independently reaching for three different
  numbers ... is itself a pitfall ... this fix risks becoming a fourth
  uncoordinated number." **Directly applicable**: any new turn-budget
  sizing this project introduces is a strong candidate to become a fifth
  uncoordinated number unless explicitly reconciled against the existing
  set (documented at `backlog_service_triage.go:893-904` per that research).
- §2: hard invariant, traced to a **documented past incident** ("the
  stop_session-deletes-branch incident") — automated remediation must
  *never* force-kill or bypass a live session, only observe and flag. Any
  turn-budget redesign that considers "kill and respawn on budget
  exhaustion" must preserve this — the existing pattern is respawn only
  after the driver *itself* gives up (turn cap reached, driver returns), not
  proactive termination of a still-driving session.
- §7: time-based logic in this codebase should take an injectable `now`
  parameter (established pattern: `staleWork(lastProgress, time.Now())`),
  not call `time.Now()` inline — testability constraint for any new
  budget/staleness comparison logic.

---

## 6. Generic pitfalls for LLM-orchestrated autonomous worker turn budgets + dedup

Independent of this codebase, drawn from the general shape of the problem
(this section is deliberately not repeating the codebase-specific findings
above):

1. **Wrong counter for the budget.** "Turns" can mean LLM API round-trips,
   tool-call count, or wall-clock time — these diverge sharply when a single
   turn contains a large tool call (a slow `Bash`, a big file read/write).
   This codebase's `AutonomousDriver.maxTurns` counts orchestrator
   round-trips (a fixed loop counter), not tool calls or wall-clock — a
   budget redesign should be explicit about which counter it's changing,
   since "increase the turn cap" and "increase the wall-clock budget" are
   different fixes for potentially different symptoms.
2. **Off-by-one / boundary ambiguity in "turns remaining" checks** — e.g.
   whether the check happens before or after the turn that pushes the
   counter over the limit changes whether the *last* turn's output is ever
   evaluated for a DONE signal. Worth an explicit unit test asserting the
   exact turn at which the budget triggers, not just "eventually stops."
3. **Race between a stuck-checker cron/sweep and a live orchestrator poll.**
   This codebase has already hit this exact shape twice (the `d3227302`
   spawn race, closed by `spawnInFlight`; the dequeue race in #199, closed
   by `dequeueMu`) — any new periodic sweep interacting with turn-budget
   state should assume a concurrent live-path write is possible and needs
   the same atomic-guard treatment, not a fresh ad hoc lock per feature.
4. **Notification/duplicate-spawn interaction with retry-on-crash logic.**
   A crash mid-turn and a legitimate turn-cap exhaustion can look identical
   to a naive retry policy; conflating them risks either (a) retrying a
   genuinely-exhausted task forever thinking it's transient infra flakiness,
   or (b) treating real infra flakiness (an LLM API 500, a network blip) as
   task difficulty and burning budget on it. This codebase's own
   `AutonomousDriver.run` already collapses "LLM call itself errored" and
   "turn budget exhausted" into the same `Stuck: true` outcome shape (see
   sibling research §3 item 6) — worth flagging as a known, not-yet-closed
   version of this exact generic pitfall.
5. **Silent infinite retry loops from a guard variable only updated on the
   happy path.** BUG-041's confirmed root cause, generically stated: `if
   !attempted { try(); if err == nil { attempted = true } }` — a failure
   leaves the guard in "never attempted" state forever, so a permanent
   failure retries at full tick speed indefinitely with no backoff. Any new
   turn-budget-adjacent state machine should audit every boolean/timestamp
   latch for this exact shape.
6. **Budget resets on resume masking true elapsed effort.** Not yet
   confirmed as a live bug in this codebase, but a real risk given the
   remediation-respawn design: each `AutoRespawnAutonomousWork` call spawns
   a *fresh* driver with a fresh `maxTurns` budget (confirmed:
   `AutonomousDriver`'s turn counter is a local loop variable, reset on
   construction, not persisted/accumulated across respawns per the code
   read in sibling research §1). This means an item that's been respawned 5
   times under the remediation backoff schedule has actually consumed
   5×maxTurns of real budget, but nothing surfaces that *cumulative* figure
   to an operator — only the current attempt's turn count. If this project
   designs an "adaptive" budget, decide explicitly whether it adapts based
   on cumulative effort across all respawns for the item, or resets per
   attempt (and if it resets, whether that's the right choice given the
   remediation cap already exists specifically to bound total respawn
   count).
7. **Orchestrator "judge" hallucinating Done from partial/truncated
   terminal output.** This is the exact, now-fixed BUG behind PR #222 —
   confirmed live, twice, in one session. The general lesson (stated
   explicitly in that PR's own comments): prefer structured, explicit
   completion signals from the worked-on process itself (a tool call like
   `request_review`, a commit, a file write) over an LLM's holistic judgment
   of a raw terminal snapshot, whenever both are available. Any new
   "is this actually done / actually stuck" heuristic this project adds
   should be checked against this same principle — a *second* LLM-judged
   heuristic layered on the same weak observation channel (raw terminal
   tail) is a plausible way to reintroduce a sibling bug to #222's, just
   applied to "stuck" classification instead of "done" classification.

---

## 7. Resource-exhaustion / security risk assessment

**Could unbounded duplicate spawning exhaust tmux sessions, disk (worktrees),
or LLM API cost if not caught?** Yes, structurally, but multiple independent
backstops already exist and are individually confirmed by live incidents:

- **tmux/process exhaustion**: directly caused the 2026-07-12 OOM incident
  that motivated the *first* WIP cap (`91063c43`) — "too many concurrent
  agent sessions" was root-caused to actual kernel OOM (57GB/61GB used, swap
  exhausted). The systemd unit was additionally hardened in `dd3a287f`
  (`MemoryHigh`/`MemoryMax` at 60%/80% of system RAM, `OOMScoreAdjust=-500`)
  specifically so a *future* burst is contained to the service's cgroup
  instead of taking the whole box down again — i.e. the current design
  assumes duplicate/runaway spawning **will** happen again and is
  defense-in-depth, not purely preventive.
- **Does the WIP cap of 2 (default, now configurable 1-10 via `44f77e0b`)
  actually bound this risk today?** Partially, and with a known gap: the cap
  bounds *aggregate* concurrent work sessions across all items
  (`countLiveBacklogWorkSessions`), but does **not** by itself bound
  *per-item* duplication — that's `spawnInFlight`'s job, a completely
  separate mechanism (§2, Layer 2 in sibling research). A regression in
  `spawnInFlight` alone (e.g. a new respawn call site added that bypasses
  `SpawnSessionFromItem` and writes to storage directly) could still produce
  N duplicate sessions for one item while staying under the aggregate WIP
  cap, if N ≤ 2 — the two mechanisms are complementary, not redundant, and
  neither alone is sufficient. **This project's design should explicitly
  verify every current and any newly-added respawn call site funnels
  through `SpawnSessionFromItem` (and therefore `spawnInFlight`)** — the
  sibling features research flagged this as "needs verification" rather
  than confirmed.
- **Disk (worktrees)**: not directly investigated in this pass (out of
  scope — no git-history hits for a worktree-disk-exhaustion incident
  specifically tied to duplicate sessions), but each work session gets its
  own git worktree per `CLAUDE.md`'s architecture section, so N duplicate
  sessions for one item would mean N worktrees on disk until cleaned up.
  Worth a targeted check in the architecture research dimension for whether
  worktree cleanup is keyed off session end (and therefore would leak N-1
  worktrees if N-1 duplicate sessions are killed rather than cleanly ended)
  — not confirmed as a live incident here, flagged as an open question.
- **LLM API cost**: no rate-limiter or cost-cap specific to backlog
  automation was found in this pass (the only rate-limiting hit was
  `github.DefaultRateLimiter`, which governs GitHub API calls, not LLM
  calls — see sibling stuck-item-visibility pitfalls §4). A duplicate-spawn
  regression would translate directly into duplicate LLM spend with no
  independent cost-side backstop beyond the session-count caps above; this
  is the least-defended of the three named risks (tmux/OOM has a cgroup
  memory hard-limit as a last resort; disk was not confirmed either way;
  LLM cost has no limiting mechanism found at all beyond "fewer sessions ⇒
  less spend").

**Single biggest resource-exhaustion/correctness risk found**: the
combination of (a) `spawnInFlight` being a per-process, in-memory-only
`sync.Map` (explicitly documented as intentional — "this is a single-process
server ... an in-process guard is sufficient" per `0f7d167a`'s own code
comment) with (b) no verification in this research pass that every
automated respawn path (`AutoReopenAfterFailedReview`,
`AutoRespawnAutonomousWork`, `AutoReopenForPRFix`, `AutoRespawnReview`)
actually funnels through the guarded `SpawnSessionFromItem` entry point
rather than calling storage/session-creation directly. If even one respawn
call site bypasses it, the exact `d3227302` incident (two literal
overlapping work sessions for one item) reproduces immediately for that call
site, uncapped by anything except the aggregate WIP limit — and each
duplicate consumes real LLM spend with no independent cost backstop.
