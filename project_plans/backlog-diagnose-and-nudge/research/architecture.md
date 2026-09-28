# Architecture Research: backlog-diagnose-and-nudge

Research Agent 3 (Architecture), SDD Phase 2. Repo: `stapler-squad`, worktree
`stapler-squad-backlog-devbug_18d8252ebf303182`.

## 1. Prior art — status and what it settles

### 1.1 `superseded-rework-session-retirement` (PRs #808, #810)

The requirements doc names six spawn entry points that bypass
`spawnSessionAfterGates` -> `archiveItemWorkSessions`: `AttachSessionToItem`
(`server/services/backlog_service_sync.go`), review-gate spawn
(`session/review_gate.go`), `TriggerReReview`, triage, manual verdict, Jules
reservation, `RemediateStaleWorkSession`, `forceResetItem`.
`implementation/plan.md:1148` explicitly deferred patching all six as "a
separate, larger change" and instead shipped `ArchivedAt` guards as
defense-in-depth so a *missed* archive is harmless at restore/revive/retry
time (`decisions/ADR-001-archived-at-is-the-auto-restore-guard.md`).

**What actually closed the gap** (and this is the key finding, not in the
plan doc): PR #808 (`7cb2c74f4`, 2026-09-14) added
`server/services/superseded_session_sweeper.go` — a 60s-ticker,
`ListBacklogItems`/`ListItemSessions`-driven reconciler that finds, for every
`in_progress`/`review` item, any tmux-backed session that
`findSupersededSessions` (latest-`CreatedAt`-wins tie-break, the same helper
`TriggerReReview` uses) determines is not the current round for its
item+role, and archives it via `ArchiveSessionByUUID` — **unless**
`stopper.IsSessionLive(uuid)` says otherwise, in which case it's skipped for
that tick (never hard-kills a session someone may be steering; re-evaluated
every tick). This is a *convergent, no-special-case* fix: instead of
patching six call sites individually, it treats "session belongs to a
superseded round" as a property to reconcile continuously, closing the gap
for all six regardless of which one left the orphan behind, and for
pre-existing rows too (not just future ones).

**Recommendation for this feature**: extend `SupersededSessionSweeper`
(or a structurally identical sibling reconciler), do not build a parallel
stale-session mechanism. Two gaps remain relative to this feature's scope:

- **No handoff-summary step.** `ArchiveSessionByUUID`
  (`server/services/session_service.go:1105`) is a pure archive — sets
  status stopped, publishes `SessionArchivedEvent`, falls back to storage —
  it never calls `HandoffSummaryGenerator`. The requirement's
  "old session generates handoff summary -> hands to new session -> old
  session torn down" sequencing does not exist yet anywhere in this
  codepath. This feature must insert a `GenerateAndPersist` +
  poll-for-`READY` step *before* the sweeper's (or an equivalent nudge-path)
  archive call — see §2.
- **Scope mismatch.** The sweeper only ever looks at `in_progress`/`review`
  items and only ever compares rounds *within one item's session history*
  (same item+role, newer-supersedes-older). A backlog item that is stuck
  outside those two statuses, or a session that's simply idle-and-stalled
  with no newer round to supersede it, is invisible to it. This feature's
  "stale retry-session cleanup" is a different predicate (idle + no forward
  progress, not "a newer round exists") layered on the same archive
  mechanics, not a re-run of the same query.

### 1.2 `context-compression` (PR #612) — `HandoffSummaryGenerator`

Confirmed API surface (`session/handoff_summary_service.go`):

- `NewHandoffSummaryGenerator(entClient, pool)` — constructed once, holds an
  `inFlight sync.Map` for per-session dedup.
- `BeginGeneration(ctx, sourceSessionID, sourceSessionTitle) (release, startedAt, started, err)`
  acquires the dedup guard and writes a `generating` row.
- `GenerateAndPersist(ctx, sourceSessionID, sourceSessionTitle, release, now)`
  — **always invoked as a detached goroutine** (line 339 comment), bounded by
  `handoffSummaryTimeout = 60 * time.Second` (a `var`, not `const`, so tests
  can shrink it). On timeout or pool failure it upserts an `error` row via
  `upsertHandoffSummaryError`/`failStage`, never leaves the row stuck in
  `generating`.
- `FindRowBySessionID(ctx, sessionID)` / `ReconcileStaleness(ctx, row)` — the
  read/poll side. `ReconcileStaleness` flips a `generating` row older than
  `staleGenerationTimeout` (shared const from `session_summary_service.go`)
  to `error`.
- Row statuses: `pending -> generating -> {ready, error}`
  (`HandoffSummaryStatus*` consts).

**Design consequence for this feature**: because `GenerateAndPersist` is
fire-and-forget (detached goroutine, not something the caller can `await`),
the handoff-then-cleanup sequence cannot be "call generate, then
synchronously archive." It must be: call `BeginGeneration`, dispatch
`GenerateAndPersist`, then **poll `FindRowBySessionID` until `ready` or
`error`** (or a caller-side timeout past `handoffSummaryTimeout`) before
tearing down the old session. This is a second poll loop distinct from the
diagnostic agent's own dispatch-and-poll loop — do not conflate them (this is
also literally the requirements doc's "Rabbit Hole": "/compact vs
HandoffSummaryGenerator are two different data flows"). The feasibility risk
("60s timeout, resolves to ERROR") must be handled explicitly: on `error`,
the spec says never delete git commits/branch/worktree contents regardless,
so the fallback on `error` is almost certainly "archive anyway, without a
handoff note" (or "post a diagnostic note that handoff generation failed"),
not "block cleanup forever." That fallback decision belongs in Phase 3
planning, not assumed here — flag it as an open question.

### 1.3 `AutonomousDriver`'s existing nudge primitive

`session/autonomous_driver.go` already implements a full nudge-orchestration
loop that this feature's "nudge safety gates" and "nudge cap/cooldown"
requirements describe almost verbatim:

- `maxTurns` (default 20, `NewAutonomousDriver` line 125-127) — the
  "turn-cap precedent" the requirements point to for nudge-cap tuning.
- `isDuplicateNudge`/`lastSentNudge`/`nudgeCooldown` (3 min production
  default per the line-369 comment) — suppresses repeat nudges within a
  cooldown window and re-arms once the session goes idle again. This is
  functionally identical to the requested "nudge cap/cooldown."
  `nudge_dedup_test.go` holds the pure-function tests for this logic
  (line 65 comment) — reuse that test harness's shape.
  - `isIdleStatus` (line 589-591): `StatusIdle || StatusReady || StatusSuccess`.
  - `isImmediateStatus` (line 595-602): `StatusNeedsApproval`,
    `StatusInputRequired`, `StatusError`, `StatusTestsFailing` bypass the
    idle-settle window entirely.
  - An `idleSettleWindow`/`idleSince` debounce requires the status to *stay*
    idle for a settle period before a nudge fires — not just be idle on one
    poll (lines 560-586). This is the "safe to nudge" vs. "actively working,
    just slow" distinction the requirements' Rabbit Holes section calls out
    as unsolved; `AutonomousDriver` already solved it once.

**Discrepancy to flag, not resolve here**: `isIdleStatus` treats
`detection.StatusReady` as idle-equivalent, but project memory
(`instinct_detection_status_ready_dead_code.md`) says `StatusReady` is "never
produced by `MatchLines`" in the newer detection path and recommends gating
unattended PTY writes on `StatusIdle` + a `StatusContext` allowlist instead.
`session/detection/detector.go` in this checkout still emits `StatusReady`
from several branches (lines 481, 534, 622-688), so the memory note's claim
may be scoped to a specific call path (`MatchLines`) rather than the whole
detector. The requirements text says the nudge gate must be
"`detection.StatusIdle` AND identity-reverified" (singular status, not the
three-way `isIdleStatus` union) — Phase 3 must explicitly decide whether to
(a) reuse `isIdleStatus` as-is for consistency with the proven
`AutonomousDriver` precedent, or (b) narrow to bare `StatusIdle` per the
requirements' literal wording, and verify against current `detector.go`
behavior with a live test, not by re-reading the memory note.

**Recommendation**: extend/reuse `AutonomousDriver`'s idle-settle-window +
cooldown + dedup machinery (or factor its cadence logic into a shared helper
`AutonomousDriver` and the new diagnostic-nudge dispatcher both call) rather
than re-deriving cooldown/cap semantics from scratch. The diagnostic agent's
dispatch loop is a different orchestrator (LLM-driven bug-file/nudge/note
triage, not a fixed goal-directed loop), but the *safety* layer underneath
"is it safe to inject text into this pane right now" is the same problem
`AutonomousDriver` already has a tested answer to.

### 1.4 `.claude/rules/instance-lock-free-reads.md` — identity re-verification

The rule requires reading mutable `Instance` fields via `Snapshot()`
(`i.Snapshot().Path`, etc.), never the raw field, because actor setters in
`session/instance_actor_setters.go` mutate under `i.mu.Lock()` from
background goroutines. The nudge gate's "identity-reverified (Snapshot()-based)
immediately before the write" requirement maps directly onto this: re-fetch
`inst.Snapshot().SessionUUID` (or equivalent) right before calling
`resume_session`/`steer_session`/`write_to_session`'s underlying
`SendKeys`/`SubmitContentWithEnter` call, and compare against the UUID the
diagnostic agent believes it's targeting — not a UUID captured once at the
start of the diagnostic agent's turn (which the rule's whole rationale says
can go stale mid-turn).

### 1.5 `ce71ad1a` — the bug this conversation just filed, and its in-flight fix

This is the most load-bearing prior art for the failure-mode analysis in §4.
Root cause and fix are **already committed**, but on an unmerged branch:

- Fix commit: `6c9026e81` "fix(session): verify tmux pane ownership before
  reattach/kill (ce71ad1a)", on branch
  `backlog/stapler-squad-verify-tmux-session-ownership`. **Not an ancestor
  of `origin/main`** (`git merge-base --is-ancestor 6c9026e81 origin/main`
  fails) and not present on this worktree's branch
  (`stapler-squad-backlog-devbug`) either — it is in-flight, separate work.
- Root cause per the commit message: "A stale tmux session left behind under
  a name a new, unrelated Instance also resolves to was silently reattached
  to on nothing but a name match, letting one Instance's writes land in
  another's live pane."
- The fix adds `session/tmux/tmux_ownership.go`:
  `ReadSessionOwnerUUID(ctx, socket, name)` reads back the
  `STAPLER_SESSION_UUID` marker tmux stamped on the pane at creation (via
  `tmux show-environment`) — "the same read-back mechanism `orphan_sweep.go`
  and `workspace_peers.go` already use to identify sessions." A
  `TmuxSession`-level `verifyExistingSessionOwner(ctx)` compares the pane's
  actual marker against `expectedOwnerUUID()` before any reattach/reuse;
  **a present-but-different or absent marker is treated as untrusted, not
  grandfathered in** — this is the exact "fail closed" posture this
  feature's nudge gate needs.
- This fix operates at the **tmux-session-reuse layer** (session creation,
  `start()`/`ensureSessionExistsLocked`, `AttachToExisting`,
  `KillTmuxSessionByTitle`). It does **not** touch the MCP write path.

**Confirmed by reading the write path directly**
(`server/mcp/tools_terminal.go:277` `writeToSession`,
`server/mcp/tools_terminal.go:664` `steerSession`,
`server/mcp/tools_lifecycle.go:399` `resumeSession`): all three resolve the
target via a `findInstance(sessionID)`-style lookup (four near-duplicate
implementations exist across `tools_vcs.go:157`, `tools_backlog.go:77/86`,
`tools_goal.go:374/390/404/410`, `tools_terminal.go:734` — a minor
primitive-obsession smell worth flagging to Phase 3, not this feature's job
to fix) and then call `session.SubmitContentWithEnter`/`SendKeysWithTimeout`
directly against the resolved `*Instance`. **There is currently no
pane-ownership re-check anywhere in this write path** — a human operator
watching the session is today's only backstop. `ce71ad1a`'s fix, even once
merged, protects the *tmux-session-reuse* layer, not this write layer.

**Architectural implication**: this feature's nudge gate must independently
add a `verifyExistingSessionOwner`/`ReadSessionOwnerUUID`-equivalent check
**at the write call site**, immediately before `SendKeys`, not rely on
`ce71ad1a`'s fix to cover it by proxy. The two checks answer different
questions at different layers:

| Layer | Question | Mechanism |
|---|---|---|
| Instance object (Go heap) | Does this `*Instance` still believe it is session X? | `inst.Snapshot()` (instance-lock-free-reads.md) |
| tmux pane (OS process) | Does the actual pane still carry session X's marker? | `ReadSessionOwnerUUID` (`session/tmux/tmux_ownership.go`, once `backlog/stapler-squad-verify-tmux-session-ownership` merges) |

A nudge that only checks the first is exactly as exposed as `ce71ad1a` was:
the Instance object can be internally self-consistent while the underlying
pane has been silently swapped out from under it (tmux server restart,
external `tmux kill-session`+recreate racing the marker check, another
Instance's `ensureSessionExistsLocked` recreating under the same sanitized
name). Both checks are required; neither subsumes the other.

## 2. Event-Command-Policy table (EventStorming)

Actors: **Tyler** (initiates diagnose, owns the flag), **Diagnose Agent**
(dispatched headless session), **Target Session** (the stuck session being
read/nudged), **Reconciler** (ticker-driven sweeper, extends
`SupersededSessionSweeper`'s pattern).

| Event | Triggering Command | Actor | Policy (business rule gating the command) |
|---|---|---|---|
| `DiagnoseRequested` | `RequestDiagnosis(itemID)` | Tyler (via `StuckItemDetail.tsx`/`BacklogItemDetail.tsx` "Diagnose" button) | Feature flag `nudge_execution_enabled`-equivalent gates only the *nudge/write* path, not the diagnose/read path — diagnosis itself is read-only and always allowed. |
| `ContextBundleAssembled` | `AssembleDiagnosticBundle(itemID)` | new service (item description/AC/status/history, review verdicts, `Snapshot()` state, logs, git diff) | Bundle capped at ~250k tokens (`session/tokens`); over budget triggers `HandoffSummaryGenerator`-based compaction for cross-session handoff, native `/compact` for the diagnostic agent's own in-session growth — these are two distinct compaction paths per an explicit Rabbit Hole; never conflate. |
| `DiagnosticAgentDispatched` | dispatch headless session with bundle | Diagnose Agent (new) | MCP server ECONNREFUSED mid-dispatch is a documented failure mode (Feasibility Risk) — dispatch must fail visibly (structured log + surfaced error), not silently retry into a stuck state. |
| `BugFiled` | `create_backlog_item` | Diagnose Agent | Existing MCP tool, no new gate — filing a bug is not a write to a live session. |
| `DiagnosticNoteFiled` | `post_backlog_update` | Diagnose Agent | Used when diagnosis is inconclusive; existing tool, no new gate. |
| `NudgeAttempted` | `resume_session`/`steer_session`/`write_to_session` | Diagnose Agent | **Gate (all must hold):** (1) feature flag ON (default OFF per Risk Control); (2) target `detection` status is idle per §1.3's open question (bare `StatusIdle` vs. `isIdleStatus`'s three-way union), sustained through an idle-settle window, not a single poll; (3) Instance-level identity reverified via `Snapshot()` immediately before write (§1.4); (4) tmux-pane-level identity reverified via `ReadSessionOwnerUUID`/equivalent immediately before write (§1.5) — independent of (3); (5) nudge cap/cooldown not exceeded for this session (extend `AutonomousDriver`'s `lastSentNudge`/cooldown machinery, §1.3). |
| `NudgeCapHit` | (no-op / abort) | Reconciler or Diagnose Agent | Structured log event required (requirements: "structured logging for every dispatch/nudge/bug-filed/cap-hit event"); falls back to `post_backlog_update` diagnostic note rather than filing nothing. |
| `IdentityMismatchDetected` | abort write | Diagnose Agent | Fail closed (per `verifyExistingSessionOwner`'s "not grandfathered in" posture) — abort the nudge, log at high severity, file a diagnostic note or bug rather than retry against the same target. Never fall back to "write anyway." |
| `HandoffSummaryRequested` | `BeginGeneration` + `GenerateAndPersist` | Reconciler (stale-session cleanup path) | Only for the "stale retry-session cleanup" scope, not the nudge-a-live-session scope — distinct from `NudgeAttempted`. Dispatched as detached goroutine per `HandoffSummaryGenerator`'s actual API (§1.2); durability of the `ready` row must be confirmed via poll before proceeding to teardown. |
| `HandoffSummaryReady` / `HandoffSummaryFailed` | poll `FindRowBySessionID` | Reconciler | On `ready`: proceed to `SessionTornDown`. On `error` (incl. the documented 60s-timeout path): Phase 3 must decide the fallback (archive-without-summary vs. block) — flagged as open, not resolved by this research. |
| `SessionTornDown` | extend `archiveIfNotLive`/`ArchiveSessionByUUID` | Reconciler | Never delete git commits/branch/worktree contents (explicit requirement) — this is purely a session/tmux-pane teardown, identical postcondition to `SupersededSessionSweeper`'s existing archive call; never fires while `stopper.IsSessionLive(uuid)` is true (never hard-kill a session someone may be steering, §1.1's `pitfalls.md #2` precedent). |
| `DispatchOrNudgeLogged` | structured log write | all of the above | Every dispatch/nudge/bug-filed/cap-hit event, per explicit requirement — model this as a cross-cutting policy applied to every command above, not a separate event per command. |

## 3. Integration points summary

- **`session` package (`Instance`/`Snapshot`)**: read-side integration is
  `Snapshot()` calls for the context bundle and for identity reverification;
  no new mutable state should be added to `Instance` itself — nudge-cap/
  cooldown state can live in the dispatcher (mirroring
  `AutonomousDriver`'s in-memory `lastSentNudge`, which is per-call-scoped,
  not persisted on `Instance`).
- **Backlog service**: `server/services/backlog_service.go`/
  `backlog_service_triage.go` own `archiveItemWorkSessions`/
  `spawnSessionAfterGates`; the stale-session-cleanup half of this feature
  is naturally a sibling of `superseded_session_sweeper.go` in
  `server/services/`, sharing its `supersededSessionStore`-style narrow
  interface pattern rather than depending on the full `BacklogService`.
- **MCP tool layer**: the nudge path rides the existing
  `resume_session`/`steer_session`/`write_to_session` tools
  (`server/mcp/tools_lifecycle.go`, `tools_terminal.go`) — the safety gate
  is new logic inserted at those call sites (or a new internal helper both
  the diagnostic agent's dispatch and these tools call through), not a new
  tool surface. `create_backlog_item`/`post_backlog_update`
  (`server/mcp/tools_backlog.go:2868,2907`) are reused as-is.
- **Notification system**: not directly researched by this pass (see UX/
  product research agents) — but every `NudgeCapHit`/`IdentityMismatchDetected`
  event should route through whatever channel already surfaces stuck-item
  state to `StuckItemsSection.tsx`, per the "document AI decisions in edge
  cases" project memory (self-heal/auto-close actions should post a visible
  comment + notify, not act silently) — directly applicable here since
  nudging is fully autonomous with no human approval gate.
- **Feature flags**: `server/services/feature_flag_service.go` is the
  existing live-settable flag mechanism (confirmed present) — use it for
  the Risk Control's "nudge-execution path gated behind a live-settable
  feature flag, default OFF," consistent with project memory's "rollout
  flags: live-settable, no env vars."

## 4. Failure-mode analysis: autonomous-write aspect as a compliance-sensitive operation

Treating "a session writing to another live session with no human approval"
as a compliance-sensitive control, the relevant control is the `NudgeAttempted`
gate in §2 (idle-status + dual identity-reverification + cap/cooldown). Each
gate component is analyzed for race conditions and blast radius below.

### 4.1 Idle-status race (TOCTOU between status check and write)

**Failure**: `detection` status is read as idle, but between that read and
the actual `SendKeys` call, the target session transitions to actively
working (a human resumes typing, or the session's own agent starts a new
tool call). `AutonomousDriver`'s `idleSettleWindow` mitigates the *single
noisy poll* version of this by requiring sustained idleness, but does not
eliminate the gap between "last observed idle" and "PTY write dispatched" —
there is no atomic check-and-write primitive in this design; detection is a
side-channel (scrollback pattern matching), not a lock the target session
participates in.

**Blast radius**: a nudge lands mid-thought in an actively-working session.
Best case: the injected text is visually confusing but harmless (the target
session's own agent treats it as an unexpected human interjection and
adapts). Worst case: the injected text lands mid-keystroke-sequence into a
different context than the diagnostic agent assumed (e.g. into a `vim`
insert-mode buffer, a password prompt, or between a partially-typed shell
command and its Enter) — this class of corruption is exactly what BUG-047/
BUG-031's existing guards (`session.EnterKeySequence`,
`SubmitDriverContent`'s two-write discipline, both cited at
`tools_terminal.go:306-310`) were built to prevent for *legitimate* writes,
but those guards protect keystroke *sequencing*, not *timing against a
racing human*. **This is not fully closable** — bound it, don't promise to
eliminate it: keep the idle-settle window at least as long as
`AutonomousDriver`'s, and treat "state changed between check and write" as
an acceptable residual risk documented in the plan, with the identity
re-verification (§4.2) as the harder backstop for the more severe case.

### 4.2 Identity-reverification race — the `ce71ad1a`-class failure

**Failure**: the diagnostic agent resolves `sessionID` -> `*Instance` at
context-bundle-assembly time (possibly minutes earlier, given the ~250k
token bundle and dispatch latency). Between that resolution and the nudge
write, the underlying tmux pane is torn down and a *new, unrelated* session
is created that happens to reuse the same sanitized tmux name (this is
exactly `ce71ad1a`'s mechanism — "a name a new, unrelated Instance also
resolves to"). If the nudge gate only re-checks `Instance.Snapshot()` (Go
heap state) and not the tmux pane's actual `STAPLER_SESSION_UUID` marker,
the Instance object can report a self-consistent UUID while the OS-level
pane underneath has already changed hands.

**Race window specifics**: `ce71ad1a`'s incident occurred through the
tmux-session-reuse path (`ensureSessionExistsLocked`/`AttachToExisting`),
which the in-flight `6c9026e81` fix closes *at that layer*. But per §1.5,
the MCP write path (`writeToSession`/`steerSession`/`resumeSession`) has its
own independent resolve-then-write window with **no ownership check today**,
fix or no fix. A second Instance could legitimately be created (e.g. a
retried backlog item spawning session `foo-r3` after `foo-r2`'s tmux pane
was killed but its Go `*Instance` object lingered in a stale in-memory
index) inside the window between the diagnostic agent's bundle assembly and
its nudge dispatch — this is a plausible, not contrived, race given the
diagnostic agent's own bundle-assembly latency is a documented risk
(250k-token bundles, HandoffSummaryGenerator's 60s timeout budget suggests
multi-second-to-minute round trips are expected elsewhere in this system).

**Blast radius**: identical in kind to `ce71ad1a` — the diagnostic agent's
nudge (autonomous, unapproved) is delivered into a live pane belonging to a
*different* backlog item/session than the one diagnosed. Consequences
compound because this is autonomous: unlike the human-observed original
incident, there is no operator present to notice a misdirected message
immediately — it could sit in the wrong session's scroll-back until that
session's own agent (or Tyler) reacts to text that makes no sense in
context, potentially causing that unrelated session to take a wrong action
of its own (e.g. treating the misdirected nudge as a legitimate instruction
and acting on it, compounding the blast radius from "misdelivered message"
to "misdelivered message causes real side effects in an unrelated
session/branch/PR").

**Mitigation** (ties to §2's `NudgeAttempted` gate and §1.5's table): require
*both* the Snapshot()-based Instance check and a fresh
`ReadSessionOwnerUUID`/pane-marker check performed as the last two
operations before the `SendKeys` call, with no I/O (context bundle
assembly, LLM calls, RPC round trips) in between the check and the write.
Any non-nil error from the marker read must be treated as "unverifiable,
abort" (mirroring `ReadSessionOwnerUUID`'s own documented contract:
"Callers must treat any non-nil err as... never as an implicit 'no owner,
safe to proceed.'"), not "verification skipped, proceed anyway." This makes
the nudge gate strictly more conservative than a human operator, who can
use visual/contextual judgment a marker check cannot replicate — an
acceptable and arguably correct trade-off given "no human approval."

### 4.3 Nudge-cap/cooldown race (concurrent dispatch)

**Failure**: if a future iteration allows more than one diagnostic agent (or
the diagnostic agent and `AutonomousDriver`) to target the same session
concurrently, an in-memory-only cooldown tracker (mirroring
`AutonomousDriver`'s `lastSentNudge`, which is a local loop variable, not
shared/persisted state) would not coordinate across dispatchers — two
independent nudges could both pass their own cooldown check and double-nudge
the same idle session in quick succession.

**Blast radius**: lower severity than §4.2 (same-session double-nudge, not
cross-session misdelivery) but still a "no human approval" runaway-repeat
risk — the exact failure class `AutonomousDriver`'s `maxTurns`/cooldown
exists to bound for its own loop. Requirements' "Nudge-cap tuning has no
empirical basis yet" Rabbit Hole compounds this: an untuned cap plus no
cross-dispatcher coordination is a plausible path to notification/log spam
or repeated interruption of a working session.

**Mitigation**: this feature's design should treat the nudge cap/cooldown as
**per-session state that must be checkable across dispatchers**, not
reproduce `AutonomousDriver`'s current per-loop-instance scoping verbatim.
Phase 3 should decide whether cooldown state is persisted (a DB row keyed by
session UUID, checked/updated atomically) or whether the design simply
guarantees single-dispatcher-per-session as an invariant enforced elsewhere
(e.g. the same `findConfirmedLiveWorkSession`/8b-guard pattern
`spawnSessionAfterGates` already uses to prevent concurrent spawns for one
item, per `backlog_service_triage.go:1271-1427`) — reusing that existing
concurrency guard is likely lower-risk than inventing new atomic cooldown
state.

### 4.4 Overall compliance framing

None of the three races above are eliminable with a single check — they are
inherent to a side-channel-observed (scrollback pattern matching, not a
transactional lock) distributed system with human-speed and LLM-speed actors
sharing the same mutable resource (a tmux pane). The correct compliance
posture, consistent with `verifyExistingSessionOwner`'s "not grandfathered
in" precedent, is **fail closed on any ambiguity** and log every abort as a
first-class structured event (already required) so that a pattern of aborts
is itself visible and actionable — turning an unavoidable race into a
bounded, observable risk rather than a silent one. The single highest-value
structural mitigation available today is reusing `ce71ad1a`'s
`ReadSessionOwnerUUID` mechanism at the MCP write layer, since that layer
currently has zero ownership verification and is exactly where this
feature's new autonomous write path lands.
