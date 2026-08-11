# Research: Stack — backlog-session-thrashing

Scope: the CURRENT ACTUAL technology/mechanism stack behind backlog work-session
execution, turn budgeting, and session lifecycle. Research only, no proposed
solutions. All line numbers verified against the worktree at
`/home/tstapler/Programming/stapler-squad/.claude/worktrees/agent-abe72f86b9e38a6f2`
on 2026-07-25.

## 1. `session/autonomous_driver.go` — how `AutonomousDriver.run()` works

### What "turn" means

A "turn" is **one orchestrator-LLM-call + one injected prompt into the target
tmux session**, not a raw LLM conversation turn inside the target session and
not a tool-call count. Concretely, one iteration of the loop at
`session/autonomous_driver.go:192-268` does:

1. Read the target session's terminal tail (`d.inst.Preview()`, line 202).
2. Build a prompt embedding `<goal>`, `<session_output>`, and `Turn N/maxTurns`
   (`buildOrchestrationPrompt`, lines 403-411).
3. Make ONE blocking call to a *separate* headless "orchestrator" LLM
   (`d.headlessPool.CallBlocking(...)`, line 210) — this is a distinct `claude -p`
   subprocess call, not the target session itself.
4. Parse the orchestrator's reply as either `NEXT_MESSAGE: <text>` or
   `DONE: <reason>` (`parseOrchestrationResponse`, lines 414-423).
5. If not done, inject `<text>` into the target tmux session via two raw PTY
   writes (`SendKeys(nextMsg)` then `SendKeys(EnterKeySequence)`, lines
   249-257) and wait (up to 5 minutes, line 262) for the target session to go
   idle again before the next iteration.

So "20 turns" = at most 20 orchestrator-LLM-decides-then-injects-one-message
cycles for the entire work session, each of which can itself trigger an
arbitrarily long amount of real work (tool calls, sub-turns, etc.) inside the
target Claude Code session between injections. This is the mechanism the
requirements doc's "turn-budget exhaustion" problem statement refers to.

### The loop and its exit paths

`for turnCount := 0; turnCount < d.maxTurns; turnCount++` (line 192). Exits by:
- **`ctx.Err() != nil`** → break, silently (line 193-195) — no `Done`, no
  `Stuck` marker set explicitly here; falls through to the post-loop "not
  done" handling (see below), so a caller-cancelled driver (e.g. server
  shutdown) still gets reported as `Stuck` with reason `"max turns reached"`
  even though it did not actually exhaust turns — the loop just has no
  distinct exit reason for context cancellation vs. turn exhaustion.
- **Rate limit wait exceeded / cancelled** (`waitForRateLimitClear` error,
  line 198-200) → break, same generic fallthrough as above.
- **LLM call error** (line 211-214) → break, same fallthrough.
- **Malformed orchestrator response** (`parseErr != nil`, lines 216-221) →
  `continue` (does NOT break or return) but still consumes a `turnCount`
  iteration of the budget — a malformed-response loop burns through the
  20-turn budget without ever injecting a real message. Tracked via
  `malformedResponseCount` for the final "stuck" reason string only (line
  272-273), not treated as a separate failure mode by the orchestration
  service.
- **`DONE:` reply** (line 223-234) → returns immediately with
  `AutonomousDriverOutcome{Done: true, Reason, PRUrl, Turns: turnCount+1}`,
  fires `fireCompletion` and returns — the only path that does not fall
  through to the post-loop block.
- **`SendKeys` failure** (writing prompt or Enter key) → break, same
  fallthrough (lines 249-256).
- **Natural loop exhaustion** (`turnCount == d.maxTurns` reached) → falls out
  of the `for` naturally, same fallthrough.

**Post-loop fallthrough** (lines 270-276): any exit that isn't a `DONE` reply
sets
```go
outcome = AutonomousDriverOutcome{Stuck: true, Reason: reason, Turns: d.maxTurns}
```
where `reason` is `"max turns reached"` or
`"max turns reached (%d malformed orchestrator responses)"`. **Every non-DONE
exit path is reported to the completion callback as "max turns reached"**,
including context cancellation, LLM call errors, rate-limit-wait timeout, and
SendKeys failures — the `Turns` field is hardcoded to `d.maxTurns` in this
branch regardless of how many iterations actually ran (`turnCount` is not
captured here). This conflates several structurally different failure modes
into one `Stuck`/`"max turns reached"` signal by the time it reaches
`onAutonomousDriverComplete`.

### The `maxTurns` default — confirmed 20

`session/autonomous_driver.go:64-69`:
```go
// pool must not be nil; maxTurns ≤ 0 defaults to 20.
func NewAutonomousDriver(inst *Instance, pool HeadlessPoolClient, goal string, maxTurns int, opts ...DriverOption) *AutonomousDriver {
	if maxTurns <= 0 {
		maxTurns = 20
	}
```
Every production call site passes literal `0` for `maxTurns`, so **every
autonomous driver in this codebase runs with exactly 20 turns, unconditionally
— there is no config knob, proto field, or per-item override for this
value**. Confirmed call sites (all pass `0`):
- `server/services/autonomous_orchestration_service.go:189` (`StartAutonomousDriverForInstance`)
- `server/services/autonomous_orchestration_service.go:207` (`StartAutonomousDriverWithTimeout`)
- `server/services/session_service.go:1573`

There is no `AutonomousMaxTurns`/`MaxTurns`/`TurnBudget` field anywhere in
`config/config.go`, `proto/session/v1/*.proto`, or any JSON config schema —
confirmed via repo-wide grep. The only places `AutonomousMaxTurns` appears
besides the driver are read-only mirrors of the *current run's* configured
value for UI display, not settable inputs:
- `session/instance.go:169-170` — `Instance.AutonomousMaxTurns int32` field, doc'd as "the configured max turns for the current run"
- `session/instance_snapshot.go:65,170` — snapshot passthrough
- `session/instance_actor_setters.go:74-119` — `SetAutonomousTurn(turn, maxTurns int32)` setter, and cleared to 0 on stop
- `server/adapters/instance_adapter.go:41` — passthrough into whatever proto/snapshot layer consumes it

This directly confirms and refines the sibling agent's lead: **"20 turns" is
real and current**, hardcoded as a hardcoded hardcoded literal in one place
(`session/autonomous_driver.go:68`), not sourced from any config file the
sibling's "verify against actual code" caveat asked to check.

### `TurnCallback` — what fires per turn

`session/autonomous_driver.go:37`: `type TurnCallback func(turn, maxTurns int, prompt string)`.
Fired only on a successful `NEXT_MESSAGE` injection (`d.fireTurnCallback(turnCount+1, d.maxTurns, nextMsg)`,
line 259) — NOT fired on malformed-response retries, so an operator watching
turn-callback-driven notifications sees fewer events than actual budget
consumption. `server/services/autonomous_orchestration_service.go:158-180`
(`buildTurnCallback`) uses this to (a) mirror `turn`/`maxTurns` onto the live
`*session.Instance` and publish a `SessionUpdatedEvent`, and (b) publish a
low-priority `NotificationEvent` titled `"Autonomous turn %d/%d"` with a
120-char-truncated preview of the injected prompt.

## 2. `server/services/autonomous_orchestration_service.go` — completion handling

All identifiers below confirmed to exist at the cited lines.

- **`onAutonomousDriverComplete`** — `autonomous_orchestration_service.go:228-546`.
  Dispatches on `is.Role` (`session.SessionRoleTriage` / `SessionRoleWork` /
  `SessionRoleReview` / default).
- **`hasActiveWorkSession`** — NOT in this file; lives in
  `server/services/backlog_service_triage.go:884-891`:
  ```go
  func hasActiveWorkSession(priorSessions []session.ItemSessionSummary) bool {
      for _, ps := range priorSessions {
          if ps.Role == session.SessionRoleWork && ps.EndedAt == nil {
              return true
          }
      }
      return false
  }
  ```
  Pure DB-row check as the sibling agent's lead states — `EndedAt == nil` is
  the only liveness signal; no tmux/process check is consulted here at all.
- **`spawnInFlight`** — `server/services/backlog_service.go:164` (field
  declaration), doc comment at lines 138-163 explaining the TOCTOU race it
  closes (2026-07-19 incident, item `d3227302`, two concurrent work
  `ItemSession` rows). It's a bare `sync.Map` used as a per-item mutex-like
  guard (`LoadOrStore`/`Delete`), consulted in `SpawnSessionFromItem` at
  `server/services/backlog_service_triage.go:363-368`.
- **`countLiveBacklogWorkSessions`** —
  `server/services/backlog_service_triage.go:854-880`. Counts items in
  `in_progress` OR `review` status; for `review`-status items it additionally
  calls `hasActiveWorkSession` on that item's sessions (since
  `AutoReopenAfterFailedReview` can leave a work session alive polling for a
  verdict after status flips to `review` — comment lines 848-853 explains this
  was previously undercounted).
- **`tombstoneOrphanWorkSessions`** —
  `server/services/backlog_service_triage.go:2400-2429`. Marks an open
  work-role `ItemSession` as ended if `s.sessionStopper.IsSessionLive(...)`
  reports false — i.e. this is the actual tmux/process-liveness cross-check
  that `hasActiveWorkSession` itself lacks, but it is invoked as a *separate,
  prior* step by callers (e.g. `SpawnSessionFromItem`'s
  `spawnSessionAfterGates` path, `AutoRespawnAutonomousWork` at
  `backlog_service_triage.go:1291`) — it is not baked into
  `hasActiveWorkSession` itself, so any call site that checks
  `hasActiveWorkSession` without first calling `tombstoneOrphanWorkSessions`
  is still vulnerable to a dead-but-DB-live session blocking a spawn. If
  `sessionStopper` is nil, this function is a no-op (conservative "assume
  alive" — line 2401-2403).
- **`notifyIfActiveWorkSessionStale`** —
  `server/services/backlog_service_triage.go:950-1000ish` (function starts
  950; see full doc comment 906-949). Uses
  `s.sessionStopper.TimeSinceLastMeaningfulOutput(active.SessionUUID)` — a
  *different* staleness primitive than turn count — compared against
  `maxReworkBlockStaleness = 15 * time.Minute` (line 904). Explicitly scoped
  to the `AutoReopenAfterFailedReview` path only (review-status staleness),
  NOT wired into the turn-cap/`SessionRoleWork` stuck path in
  `onAutonomousDriverComplete`. Confirms the sibling agent's lead: this
  primitive exists and is reused for exactly one purpose today (rework-block
  staleness), not for turn-cap decisions.
- **`RemediationDue`** — `session/backlog_remediation.go:168-193`. Shared
  backoff gate; backoff schedule confirmed at lines 31-37:
  `[30m, 2h, 8h, 24h, 72h]`, with `MaxRemediationAttempts = int32(len(...))
  = 5` (line 45). `evaluateRemediation` (lines 96-110) is the pure
  decision function; `RemediationDue` (168-193) is the DB-integrated,
  side-effecting wrapper that atomically records the attempt. Confirmed used
  by `AutonomousOrchestrationService.onAutonomousDriverComplete` at
  `autonomous_orchestration_service.go:358` for the
  `domain.StuckReasonAutonomousStuck` reason, gating the
  `AutonomousStuckRespawner.AutoRespawnAutonomousWork` call.

### What happens on cap-exceeded (`SessionRoleWork`, `!outcome.Done`)

`autonomous_orchestration_service.go:326-384`. On a turn-cap stop (any
non-`DONE` exit — see the "conflated exit reasons" finding in section 1), the
item is **left `in_progress`** (not forced into `review`) and:
1. A durable `autonomous_stuck` row is written/reopened via `MarkStuck` +
   `MarkStuckNotified` just above (lines 293-301), unconditionally (not gated
   by the backoff below — this happens on *every* turn-cap occurrence).
2. `RemediationDue(ctx, itemID, domain.StuckReasonAutonomousStuck)` is called
   synchronously (line 358) to gate whether a respawn should actually fire.
3. If `due`, `AutoRespawnAutonomousWork` is dispatched in a **new goroutine**
   (`go func() { ... }`, lines 376-380) — asynchronous, fire-and-forget, only
   logged on error.
4. If `justParked` (5th attempt), a `"Auto-rework paused"` notification fires
   (lines 363-374) referencing `session.MaxRemediationAttempts` (5) in the
   body text.
5. If not `due` (still in backoff), nothing further happens this occurrence —
   logged at Info only (line 382).

So the actual current behavior is: **turn-cap-without-DONE never retries
silently on every occurrence** — it's gated by the same 5-attempt,
30m→2h→8h→24h→72h backoff as every other stuck reason, confirming the
sibling agent's "Features" research. What's absent is any *earlier* signal —
this backoff decision only fires *after* a full 20-turn budget has already
been burned; there is no mid-run early-exit for "no progress happening."

### `AutoRespawnAutonomousWork` (the respawn action itself)

`server/services/backlog_service_triage.go:1267-1307+`. Before respawning:
- Re-checks item is still `in_progress` (line 1276-1280).
- Re-tombstones orphan work sessions (line 1291) then re-checks
  `hasActiveWorkSession` (line 1292-1295) — belt-and-suspenders against a
  race with a second respawn attempt.
- Checks the **rework cap** (`effectiveReworkCap(item)`, default from
  `config.Config.MaxAutoReworkIterationsOrDefault()` = **3**, see section 3)
  against `workCount` (total work-role `ItemSession` rows ever created for
  this item, not just live ones) — if at/over cap, notifies
  `notifyReworkCapHit` and does **not** respawn (lines 1297-1307). This is a
  *separate* cap from `MaxRemediationAttempts` (5): the rework cap bounds how
  many times a NEW work session may be spawned for the item at all (across
  its whole history), the remediation cap bounds how many *automated retry
  attempts* the backoff gate will grant.

## 3. Turn/budget/WIP config keys — confirmed via repo-wide grep

No `MaxTurns`/`TurnBudget`/`turn_limit`/`TurnCap` config field exists anywhere
(proto, JSON, `config/config.go`) — the 20-turn autonomous-driver budget is a
pure Go literal (`session/autonomous_driver.go:68`) with zero external
configurability. The config surface that DOES exist governs adjacent but
distinct concerns:

| Config field | File:line | Default | Governs |
|---|---|---|---|
| `MaxAutoReworkIterations` | `config/config.go:302-307` | 3 (`config/config.go:568-580`, `MaxAutoReworkIterationsOrDefault`) | Max number of *work sessions ever spawned* for one backlog item (rework cap) |
| `MaxConcurrentBacklogWorkItems` | `config/config.go:308-311` | 2, hard ceiling 10 (`config/config.go:583-602`) | Global WIP cap across all backlog items (`countLiveBacklogWorkSessions`) |

Both are runtime-mutable via `DefaultsService.UpdateGlobalDefaults`
(`server/services/defaults_service.go:130-131,148-149`) and read under
`BacklogService.cfgMu` (`server/services/backlog_service.go:120-128,
268-283`) — unlike the autonomous-driver turn cap, which cannot be changed
without a code change and rebuild.

`MaxRemediationAttempts` (session/backlog_remediation.go:45) — 5, derived from
`len(remediationBackoffSchedule)` — is also a hardcoded var, not
config-file-driven, though it's a `var` not `const` (mutable at the Go level,
e.g. by tests) rather than a JSON-configurable setting.

## 4. Every session-spawning code path for backlog work items

Enumerated by tracing every call to `SpawnSessionFromItem`
(`server/services/backlog_service_triage.go:330-425`, the sole RPC/entry
point that actually creates a work `ItemSession` + tmux session) and its
internal delegate `spawnSessionAfterGates`:

1. **`SpawnSessionFromItem` RPC** (`backlog_service_triage.go:330`) — the
   direct ConnectRPC handler, called by the frontend "Run"/"Run Autonomously"
   button and by `req.Msg.Force` retriggers.
2. **`DequeueNextQueuedItems`** — calls `spawnSessionAfterGates` directly
   (bypassing the `SpawnSessionFromItem` RPC's own gates a second time per
   the comment at line 396-398), invoked from:
   - `BacklogLifecycleListener.onSessionExited`'s `go l.triggerDequeue(...)`
     (per `backlog_service.go:130-136` doc comment)
   - The periodic `ReconcileStuck` sweep tick
3. **`AutoRespawnAutonomousWork`** (`backlog_service_triage.go:1267+`) — the
   turn-cap-without-DONE respawn path from
   `onAutonomousDriverComplete` (section 2 above), itself funnels back into
   `SpawnSessionFromItem`-equivalent logic (confirm exact call inside this
   function extends past the read window; it is gated by `spawnInFlight`,
   `hasActiveWorkSession`, and the rework cap before spawning).
4. **`AutoReopenAfterFailedReview`** — referenced throughout
   `backlog_service_triage.go` (e.g. line 355-356, 906-949) as one of three
   "funnel to SpawnSessionFromItem" callers alongside `AutoRespawnAutonomousWork`
   and `AutoReopenForPRFix`; fires when a review verdict is FAIL and the item
   is reopened for another work round.
5. **`AutoReopenForPRFix`** — the third funnel caller (named alongside the
   above at `backlog_service_triage.go:355-357`, and again at line 1287-1290)
   for reopening work after a PR-fix-needed signal.
6. **Manual "Force" retrigger** — `req.Msg.Force` path inside
   `SpawnSessionFromItem` itself (lines 374-381), calls `forceResetItem` then
   falls through to the same gates.

All six paths are serialized per-item by the single `spawnInFlight sync.Map`
guard (`backlog_service.go:164`) inside `SpawnSessionFromItem` — but paths 3-5
that call `spawnSessionAfterGates` directly (bypassing the RPC's own
`LoadOrStore` at line 363) may or may not go through the same guard depending
on whether `spawnSessionAfterGates` itself re-acquires it (outside this read's
window — worth a direct follow-up read of `spawnSessionAfterGates`'s body if
the planning phase needs to confirm this).

## 5. Persistence — `session/ent/schema` and config

- **`session/ent/schema/item_session.go`** (full contents above) — the
  `ItemSession` ent entity persists `session_role` (work/triage/review),
  `started_at`/`ended_at` (liveness signal `hasActiveWorkSession` reads),
  `last_commit_sha`/`last_commit_at`/`commit_count_since_spawn`,
  `last_file_touch_at`/`last_progress_at`, `estimated_cost_usd`. **There is no
  turn-count or turn-budget field on `ItemSession`** — turn progress
  (`AutonomousTurn`/`AutonomousMaxTurns`) exists only as transient in-memory
  fields on the live `*session.Instance` (`session/instance.go:169-170`),
  reset to 0 on driver stop (`session/instance_actor_setters.go:119`) and
  never written to any ent entity. If the server restarts mid-run, the
  in-flight turn count is lost entirely (consistent with
  `backlog_stuck_state.go`'s `grace_boot_time` comment explicitly
  acknowledging "in-flight AutonomousDriver goroutines are lost on restart").
- **`session/ent/schema/backlog_stuck_state.go`** (full contents above) — the
  durable stuck-state row. One row per `(item_id, reason)` (unique index,
  line 90), resolve-in-place model (no history retained). Fields relevant to
  turn-cap thrashing: `remediation_attempts` (int32, default 0, cap 5),
  `next_remediation_at` (backoff timer), `grace_boot_time` (restart-grace,
  one free pass per boot). This is the only durable record that a work
  session hit its turn cap — it stores the *fact* and *attempt count*, not
  the *turn count* that triggered it (that's embedded only in the
  free-text `context` field via the `MarkStuck` call's message string,
  `autonomous_orchestration_service.go:294-297`: `"autonomous driver stopped
  after %d turns without a DONE signal (%s)"`).
- **`config/config.go`** — JSON-persisted process config (`MaxAutoReworkIterations`,
  `MaxConcurrentBacklogWorkItems` as covered in section 3); no turn-budget
  field.

## 6. Versions / libraries

From `go.mod` (`go 1.26.3`):
- **LLM client**: no Anthropic SDK dependency at all. Both the autonomous
  orchestrator LLM calls (`session/autonomous_driver.go:210`,
  `d.headlessPool.CallBlocking`) and the backlog triage/review LLM calls go
  through `session/headless/` (`runner.go:1-2`: *"Package headless provides a
  subprocess-based interface for running claude -p headlessly"*) —
  i.e. the orchestrator drives the target session by shelling out to the same
  `claude` CLI binary product (`ProcessRunner`, `runner.go:26-55`, using
  `github.com/tstapler/stapler-squad/executor` to start the subprocess), not
  a Go/Python Anthropic API client library. This means "turn" cost/latency is
  bounded by a full `claude -p` cold-start + response cycle per orchestrator
  decision, not a lightweight API call.
- **tmux control mode**: no third-party tmux Go library (`gotmux` or similar)
  appears in `go.mod`/`go.sum` — confirmed by grep. Control-mode
  parsing/session management is bespoke, implemented across
  `session/tmux_backend.go`, `session/tmux_process_manager.go`,
  `session/external_tmux_streamer.go`, `session/pty_subscriber.go`, using
  `github.com/creack/pty v1.1.24` for the underlying PTY plumbing.
- Other stack-relevant deps: `entgo.io/ent v0.14.5` (ORM backing
  `ItemSession`/`BacklogStuckState`), `connectrpc.com/connect v1.19.0`
  (RPC layer for `SpawnSessionFromItem` etc.), `github.com/puzpuzpuz/xsync/v4
  v4.5.0` (referenced elsewhere in the codebase for concurrent maps, though
  `spawnInFlight` itself uses the stdlib `sync.Map`, not xsync, per its own
  doc comment reasoning at `backlog_service.go:156-163`).

## Open threads for the planning phase (not solutions — just unresolved facts)

- `spawnSessionAfterGates`'s body was not read in this pass — needs
  confirmation on whether it re-acquires `spawnInFlight` for the
  `DequeueNextQueuedItems`/`AutoRespawnAutonomousWork`/`AutoReopenAfterFailedReview`/
  `AutoReopenForPRFix` call paths that bypass the `SpawnSessionFromItem` RPC's
  own `LoadOrStore`.
- The post-loop fallthrough in `AutonomousDriver.run()` (section 1) reports
  context-cancellation, LLM-call-failure, rate-limit-timeout, and
  SendKeys-failure exits identically to true turn-cap exhaustion (all as
  `Stuck: true, Reason: "max turns reached", Turns: d.maxTurns`) — a fact,
  not a fix, but directly relevant to "the system's response to hitting that
  cap... is not well understood" from the requirements doc, since several of
  these are not turn-cap events at all yet are indistinguishable from one by
  the time they reach `onAutonomousDriverComplete`.
