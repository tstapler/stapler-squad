# Research: Pitfalls — backlog-diagnose-and-nudge

Research Agent 4 (Pitfalls), SDD Phase 2.

## 1. Prior incidents in this repo (git log + project_plans)

VERIFIED via `git log --all -i --grep` and reading the named commits/plans.

- **Crash-loop restart storm, unbounded memory growth** — commit `b28e78fc0`
  ("fix(session): stop crash-loop restarts and cap unbounded status-event
  reload", 2026-09-16). A session whose `Start()` kept failing retried
  forever; the restart-storm detector only *logged*, it never blocked.
  Confirmed via pprof: 93% of a 7.6GB live heap after ~80 minutes of one
  stuck item's retry storm, because `attachStatusEventsForPublish`
  rebroadcast the item's *entire* growing status history on every retry.
  Fix: `checkRestartStorm()` now actually blocks `Start()` for a
  self-clearing cooldown once a threshold is hit, and history is capped at
  200 events. **Relevance**: a nudge-cap/cooldown that only logs (not
  blocks) is not a cap — this repo has already shipped and had to fix
  exactly that failure shape once.
- **Cross-item spend-cap race + config-freeze in an autonomous dispatch
  service** — commit `5a187795b` ("fix(jules): stop dispatch service from
  freezing config + close cross-item spend-cap race"). `JulesDispatchService`
  held a frozen `*config.Config` snapshot, so live consent/cap edits never
  took effect without a restart; separately, `checkSpendGuards` read the
  open-session count under only a per-item mutex, so two *different* items
  dispatched concurrently near the ceiling could both observe a stale
  under-limit count and both proceed — overshooting the hard ceiling ADR-004
  promises. Fixed with a package-level `julesSpendGuardMu` serializing
  check-then-reserve across items. **Relevance**: this feature's "nudge cap"
  is the same shape of guard (check current state, then act) and needs the
  same check-then-reserve atomicity if diagnose dispatches can run
  concurrently across items — a per-item lock is not enough by itself.
- **Mass session resurrection on redeploy** — `project_plans/superseded-rework-session-retirement/requirements.md:28-39`:
  a `make install-service` redeploy resurrected ~17 stale sessions across 3
  backlog items in seconds (e.g. one item resurrected rounds r3,r4,r6-r10 when
  only r10 was current). Real cost: N concurrent Claude processes competing
  for the same API budget on every restart, "plausibly contributed to a
  weekly rate-limit exhaustion the same day." This is the closest prior-art
  match to this feature's own "old session generates handoff -> new session
  -> old session torn down" flow, and its unresolved gaps are detailed in
  section 3 below.
- Ambient pattern across `git log`: multiple independent `fix(github):
  rate-limit`/`quota`/OTLP-message-size-exceeded commits (`b38c9ca46`,
  `f4dff9107`, `10c055b8f`, `fc7b5fcc8`) show this codebase repeatedly
  under-budgeted for "a background/automated process calls an external API
  or emits telemetry more than expected." A dispatched diagnostic LLM
  session is exactly this shape (external API calls, log/telemetry volume)
  and has no existing budget precedent of its own (see Feasibility Risks in
  requirements.md: "no empirical basis yet" for the nudge cap, unlike
  `AutonomousDriver`'s `maxTurns` default of 20,
  [`session/autonomous_driver.go:126-127`](session/autonomous_driver.go#L126-L127),
  justified inline as bounding "a runaway/looping orchestrator"
  ([`session/autonomous_driver.go:359`](session/autonomous_driver.go#L359)).

## 2. Known-bad status detection (StatusReady) — confirmed still present in code

VERIFIED by reading `session/session_driver.go:625` and `session/detection/idle.go:234`.

`detection.StatusReady` is still referenced in several live code paths —
`session/autonomous_driver.go:590`, `session/claude_controller.go:795,1092`,
`session/instance_status.go:202,246`, `session/scroll_gate.go:95`,
`session/detection/idle.go:234` — despite project memory recording it as dead
(never produced by `MatchLines`). `session/session_driver.go:625`'s own
comment concedes the ambiguity: *"StatusReady is a `.*` catch-all; StatusIdle
is the precise signal."* This means:

- Any code that treats `StatusReady` as equivalent to confirmed-idle (several
  of the above call sites do, via `==` checks) is trusting a status this
  codebase's own commentary flags as an imprecise catch-all, not dead in the
  sense of unreachable — dead in the sense of "not the signal you think it
  is." **This feature's idle-gate must key on `detection.StatusIdle`
  specifically**, not any status set that includes `StatusReady`, or the
  "confirmed idle before nudging" gate inherits this ambiguity silently.

## 3. superseded-rework-session-retirement: gaps are CONFIRMED STILL OPEN, not closed by 02bac89d2

VERIFIED by reading `project_plans/superseded-rework-session-retirement/requirements.md`
and `implementation/plan.md`, and by inspecting commit `02bac89d2` directly.

- `02bac89d2` ("route OverrideVerdict/AttachSessionToItem transitions through
  the injected engine") is an **unrelated** fix — it routes two status
  *transition* checks through `ConfiguredWorkflowEngine` instead of the
  static `CanTransitionBacklog` map. It does **not** touch session spawning,
  archival, or the ADR-001 guards. It does not close any of the gaps below.
- The shipped fix (ADR-001, `project_plans/superseded-rework-session-retirement/decisions/ADR-001-archived-at-is-the-auto-restore-guard.md`,
  PRs referenced in that project's plan.md) is **defense-in-depth at
  restore/revive/retry time** — six guards keyed on `ArchivedAt`/`IsArchived()`
  that stop an already-stale session from being resurrected when the server
  restarts or a retry loop fires. It is explicitly **not** a fix to the spawn
  sites that create the staleness in the first place.
- `implementation/plan.md:1148` states this outright as an "Out of scope"
  line item: *"Patching the spawn entry points that bypass
  `spawnSessionAfterGates` (`AttachSessionToItem`, `TriggerReReview`,
  review-gate spawn, triage, manual verdict, Jules reservation) ... Six spawn
  sites is a separate, larger change."* The reasoning given is that the
  `ArchivedAt` guards make a *missed* archive harmless at restore/retry time
  — but that harmlessness is scoped to restore/retry, not to steady-state
  operation between restarts.
- `requirements.md:146-154` lists the full known-bypass set with file:line
  citations: `AttachSessionToItem` (`backlog_service_sync.go:103` — "the
  biggest blind spot": arbitrary user title, no `-rN` suffix, no archive
  call at all), review-gate spawn (`session/review_gate.go:444`),
  `TriggerReReview` (`:2989,:2860,:2896`), triage
  (`backlog_service_trigger_triage.go:321`), manual verdict
  (`backlog_service_lifecycle.go:1212`), Jules reservation
  (`jules_dispatch_service.go:324`), plus non-spawn mutators
  `RemediateStaleWorkSession` (`:2010`) and `forceResetItem` (`:1122`).
- `implementation/follow-ups.md` (Phase 6 review pass) records five
  additional deferred items, most relevantly: **follow-up #1** — there is
  still no lint/structural enforcement that "every automated
  start/revive/retry must consult `IsArchived()`"; the six guards are "a
  convention, not a ratchet — a seventh auto-lifecycle path can be added
  tomorrow with no archived check and nothing fails," and designing the
  predicate for such a lint rule was explicitly deferred as its own design
  task. **Follow-up #3** — `archiveItemWorkSessions`
  (`session/backlog_lifecycle_archive.go:137`) is still not idempotent
  across repeat calls for the same item (made harmless, not correct, by
  guard 4).

**Why this is a live pitfall for backlog-diagnose-and-nudge specifically**:
this feature's "Stale retry-session cleanup" (old session -> handoff summary
-> new session -> old session torn down) is, structurally, an **eighth
spawn/retirement entry point** in exactly the shape follow-up #1 warns about.
Two concrete failure modes follow directly from the gap being open, not
theoretical:

1. If this feature's cleanup path archives the old session by any means
   *other* than the same mechanism `spawnSessionAfterGates` step 12c uses
   (setting `ArchivedAt` correctly, in the right order relative to the new
   session's creation), it reproduces the exact "N concurrent live sessions
   for one item" shape the original incident was — even though the
   guard-in-restore machinery would eventually clean it up at the *next*
   restart, not before. The requirements.md's own success criteria ("must
   NEVER delete git commits/branch/worktree contents") shows the team is
   already alert to the destructive-cleanup risk; it should be extended to
   "must set `ArchivedAt` via the same path/ordering as `spawnSessionAfterGates`,
   not a bespoke one," given six other bespoke paths are the known list of
   things that went wrong here before.
2. Because there is no lint ratchet (follow-up #1), nothing will fail CI or
   review if this feature's new dispatch/nudge code path also fails to
   consult `IsArchived()`/archive correctly — the same way the original six
   bypasses accumulated silently over time. This argues for either (a)
   explicitly building this feature's cleanup on top of the *same* archival
   primitive `spawnSessionAfterGates` uses (reuse, not reinvent — matching
   this project's own stated "Reuse existing primitives" constraint,
   `requirements.md:90-92`), or (b) treating "add the lint analyzer from
   follow-up #1" as an in-scope task for this feature, since it is the one
   feature explicitly adding a *new* autonomous session-lifecycle-mutating
   path since that gap was documented.

## 4. Feasibility risks — verified against code

- **HandoffSummaryGenerator timeout**: VERIFIED —
  `var handoffSummaryTimeout = 60 * time.Second`
  (`session/handoff_summary_service.go:40`), used at
  `session/handoff_summary_service.go:384` via
  `context.WithTimeout(ctx, handoffSummaryTimeout)`. On error (including
  timeout), `g.failStage(ctx, sourceSessionID, sourceSessionTitle,
  "generation", err, now, false)` is called (`:388`), which per the file's
  own naming writes a `HandoffSummaryStatusError`-shaped row rather than
  leaving the row silently absent or half-written. **Design implication**:
  the cleanup step must check the resulting `HandoffSummary` row's status
  field before killing the old session's tmux/terminal state — a `false`
  return from generation (timeout or LLM failure) must abort or retry the
  teardown, not proceed on a bare "the call returned" signal. This is
  exactly the "read a mutation back before claiming it happened" discipline
  this repo already expects elsewhere.
- **AutonomousDriver turn-cap precedent**: VERIFIED — default `maxTurns = 20`
  (`session/autonomous_driver.go:126-127`), with an inline comment at `:359`
  noting a turn still counts against the cap even when the loop is a
  "runaway/looping orchestrator" — i.e., the cap exists specifically as a
  runaway-loop breaker, not just a cost control. The nudge-cap this feature
  needs has, per requirements.md's own Rabbit Holes section, "no empirical
  basis yet" — meaning it should ship conservative (low) and adjustable via
  the same live-settable feature-flag mechanism the Risk Control section
  already calls for, rather than guessing a number and hard-coding it.
- **MCP server reachability**: reproduced firsthand in this research
  session — the `stapler-squad` MCP server failed to connect
  (`ECONNREFUSED`) mid-session, exactly as requirements.md's Feasibility
  Risks section anticipated. This is not a hypothetical: a dispatched
  diagnostic/nudge agent that calls `write_to_session`/`resume_session` via
  MCP and gets a connection failure must distinguish "tool call failed, no
  write happened" from "write happened, response lost" — the latter is
  indistinguishable from success at the call site per this repo's own
  Evidence-and-Claims discipline, and is especially dangerous here because a
  retried write to a session that already received the first one could
  double-nudge or corrupt terminal state.

## 5. General pitfalls for autonomous-remediation / self-healing systems (domain knowledge)

- **False-positive "safe to act" gates**: the single most common failure
  mode in self-healing systems is treating "no recent activity" as "not
  working" when it's actually "still working, slowly" (long tool call,
  network wait, human reviewing in another tab). This is explicitly named
  in requirements.md's Rabbit Holes and is compounded here by the
  `StatusReady`-ambiguity finding in section 2 — the detection substrate
  itself has known imprecision at exactly the boundary this feature needs
  to be precise about.
- **Compounding automation**: an autonomous agent that "fixes" a stuck item
  by nudging a session, and the nudge itself causes a new kind of stuck
  state (e.g., the target session was idle because it was *done* and
  awaiting human merge, not because it crashed) is a classic actuator/sensor
  mismatch — acting on a proxy signal (idle) for the real target (broken).
  Filing a bug or posting a diagnostic note when inconclusive (already in
  scope) is the correct mitigation; the risk is scope creep in the
  diagnostic agent's own judgment of "inconclusive" drifting toward
  optimistic nudging over time as it "learns" nudging is the cheap option.
- **Identity/routing bugs invalidate the trust boundary "nudge the right
  session" depends on**: requirements.md names `ce71ad1a` (tmux
  session-name collision causing cross-session message delivery) as the
  bug that prompted this whole feature. That is not incidental — it is
  direct evidence that the specific substrate this feature's core safety
  gate depends on ("target session's UUID/identity re-verified via
  Snapshot() immediately before the write") has had a real, recent defect
  in exactly that trust boundary. Re-verifying identity immediately before
  the write (already in scope) is necessary but, per `instance-lock-free-reads.md`'s
  guidance in this repo, must go through `Snapshot()` — not a raw field
  read — to avoid reintroducing the class of race that rule was written to
  eliminate. A second, related live item, `e6c2a88e` ("work sessions lose
  --mcp-config across restarts"), is further evidence the session
  substrate's state can silently drift out from under long-lived
  assumptions — relevant because a diagnostic agent's dispatch decision may
  be made from a bundle assembled some time before the write actually
  executes.
- **Autonomous write systems need a kill switch that actually stops writes,
  not just new dispatches**: the crash-loop incident (`b28e78fc0`) shows this
  repo's first attempt at a cap (log-only) was insufficient — it didn't
  block the very thing it was meant to prevent. The feature flag in Risk
  Control ("default OFF... disabling it reverts to diagnose-and-report-only")
  should be verified to actually gate the write call itself (e.g., inside
  `resume_session`/`steer_session`/`write_to_session`'s dispatch path), not
  just gate whether a new diagnostic session is *started* — otherwise an
  in-flight diagnostic agent that already decided to nudge before the flag
  flipped off could still execute the write.

## Summary of design-against items

1. Nudge cap/cooldown must actually **block** the write path when hit, not
   merely log (precedent: `b28e78fc0`), and must be **live-settable**, not a
   hardcoded default with no empirical basis.
2. Idle-check for "safe to nudge" must key on `detection.StatusIdle`
   specifically, never a set that includes the ambiguous `StatusReady`
   catch-all (`session/session_driver.go:625`).
3. Session-identity re-verification immediately before write must use
   `Snapshot()`, never a raw field, per `.claude/rules/instance-lock-free-reads.md`,
   given `ce71ad1a`'s recent identity/routing defect in this exact area.
4. Handoff-then-cleanup must check the `HandoffSummary` row's resulting
   status (ready vs. error/timeout) before tearing down the old session —
   `failStage` on the 60s timeout writes an error row that a naive
   "did the call return" check would miss.
5. Stale-session cleanup must reuse the same archival primitive/ordering
   `spawnSessionAfterGates` uses, not invent a ninth bespoke path — the
   `superseded-rework-session-retirement` project's six known bypasses are
   proof this class of bug accumulates silently and has no CI-enforced
   ratchet (follow-up #1 there is still unimplemented).
6. Concurrent dispatch across multiple backlog items needs a shared
   guard (not per-item), matching the fix already applied to
   `JulesDispatchService`'s spend-cap race (`5a187795b`) for the same
   check-then-act shape.
7. MCP/tool-call failures during a write must not be treated as "no-op on
   failure" — a lost response after a real write risks a double-nudge on
   retry.
8. The kill-switch feature flag must gate the write call itself, not just
   new dispatch starts, so an in-flight agent can't act after the flag is
   flipped off.
