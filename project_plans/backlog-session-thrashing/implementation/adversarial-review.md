# Adversarial Review: backlog-session-thrashing

**Date**: 2026-07-25
**Verdict**: CONCERNS
**Pass**: 2 (re-review after patch)

Re-verified against the actual code in the worktree (not the plan's or the prior
patch's citations alone): `session/autonomous_driver.go`,
`server/services/autonomous_orchestration_service.go`,
`server/services/backlog_service_triage.go`, `server/services/backlog_service.go`,
`server/mcp/tools_backlog.go`, `server/services/session_service.go`,
`config/config.go`, `server/services/autonomous_orchestration_service_test.go`,
`server/services/backlog_service_test.go`, `server/services/backlog_service_triage_test.go`,
`go.mod`. All file/line citations in the patched plan were checked directly against
current code, not trusted.

## Prior Blocker Verification

- **[PASS] Blocker 1 — Epic 3.1's blanket early-return regressed BUG-048's review
  row-resolution.** The patch does exactly what it claims: Task 3.1.1a only adds
  `outcome.ExitKind != session.DriverExitContextCancelled` to the `MarkStuck`
  guard (current code confirmed at `autonomous_orchestration_service.go:293`,
  `if !outcome.Done {`) and adds a late guard immediately before the generic
  notification block (current code confirmed at line 518, "Fire push notification
  via event bus."). The role-specific `switch is.Role` block (confirmed at lines
  305-487) is explicitly left untouched — Task 3.1.1a states this in so many
  words ("Leave the role-specific switch is.Role block ... completely
  unmodified"). I traced `SessionRoleReview`'s `default` branch (lines 449-477,
  the actual BUG-048 fix, `UpdateItemSessionEnded` at line 474) and confirmed it
  is not gated by anything the patch touches — it still fires unconditionally for
  any non-Done, non-terminal-status review outcome regardless of `ExitKind`. Also
  confirmed `SessionRoleReview` always returns at line 479 before reaching the
  new notification guard at line 518, so the new guard is genuinely unreachable
  for the review path — it can only affect `SessionRoleWork`'s non-return path
  and the unrecognized-role fallthrough, exactly as the plan states. A dedicated
  regression test is specified (Task 3.1.1b,
  `..._StillEndsAbandonedReviewRow_When_ContextCancelled`) that would fail
  against the earlier "blanket early return" revision. This is a real fix, not a
  restated claim.

- **[PASS, with one implementation-discipline caveat — see New Concerns]
  Blocker 2 — `spawnInFlight` relocation narrowed the guarded critical
  section.** The patch reverts the relocation entirely: `SpawnSessionFromItem`'s
  guard (confirmed unchanged at `backlog_service_triage.go:354-368`, acquired
  right after loading the item, released via `defer`, still wrapping
  `forceResetItem`/status validation/planning gate/WIP-cap+`queueBacklogItem`)
  is untouched. `DequeueNextQueuedItems` (confirmed at lines 517-589) currently
  acquires **no** guard at all around its claim+spawn loop — Task 1.1.1b's
  addition of a per-item `spawnInFlight.LoadOrStore`/`Delete` acquisition
  *before* the `transitionWithGuard` CAS claim and held through
  `spawnSessionAfterGates`'s return is a pure addition, not a narrowing, of
  serialization. I traced the dual-acquisition-site design precisely: both call
  sites key the guard off the same `item.ID` on the same `sync.Map`, and
  `LoadOrStore` is atomic — there is no window where both sides can observe
  "not in flight" simultaneously for the same item, regardless of which side
  reaches the map first (whichever wins, the other either gets
  `CodeAlreadyExists` immediately, in `SpawnSessionFromItem`'s case, before any
  status-dependent logic runs — the guard acquisition happens at step "1b",
  before status validation at step 3 — or `continue`s to the next queued
  candidate, in `DequeueNextQueuedItems`'s case). I confirmed the "no
  reentrant/deadlock" claim directly: `DequeueNextQueuedItems` calls
  `spawnSessionAfterGates` (line 575) directly, never `SpawnSessionFromItem`
  itself, so there is no path where one call site tries to acquire the guard a
  second time while already holding it. This design genuinely closes the
  TOCTOU window architecture.md §3b describes (a concurrent `SpawnSessionFromItem`
  reading `status=in_progress` in the gap between Dequeue's CAS claim and its
  `spawnSessionAfterGates` call, and treating it as a legitimate "reopen") without
  reintroducing the narrower-critical-section regression the first blocker
  found. See New Concerns for one implementation-robustness risk this design
  invites (manual multi-exit-point release instead of `defer`).

## Prior Concern Verification

- **[PASS] Concern 3 — Epic 3.3's safety claim scoping.** The patch narrows the
  "driving mechanism already stopped" claim to the `onAutonomousDriverComplete`
  call site only (Story 3.3.1's "Scoped safety justification" paragraph), and I
  independently verified the claim holds for that site: `AutonomousDriver.run()`
  calls `d.fireCompletion` (which invokes the registered `onAutonomousDriverComplete`
  callback) synchronously, in the same goroutine, immediately before returning
  (confirmed `session/autonomous_driver.go:277-278` and the `DONE:`-equivalent
  early return at 232-233) — so by construction the driver loop has already
  exited by the time `onAutonomousDriverComplete` runs for this call path; there
  is no possible interleaving where it is "still mid-turn." `RemediateStaleWorkSession`
  is explicitly flagged as a separate, pre-existing, undemonstrated-safety call
  site (confirmed: `RemediateStaleWorkSession`, `backlog_service_triage.go:1343-1397`,
  never calls `stopAndDeregisterDriver` or checks driver liveness before its own
  kill+end at lines 1384-1394) — not silently folded into the safety argument.

- **[PASS] Concern 4 — best-effort tmux kill should fail closed.** Task 3.3.1a
  specifies calling `s.sessionStopper.IsSessionLive(active.SessionUUID)` after
  the kill attempt and failing closed (returning an error, not ending the row or
  respawning) if the pane still reports live. I confirmed `IsSessionLive` already
  exists on the `SessionStopper` interface consumed by `BacklogService`
  (`backlog_service.go:56`) and is implemented by `SessionService.IsSessionLive`
  (`session_service.go:544-546`, `s.FindLiveInstance(sessionUUID) != nil`) — so
  the design requires no new interface method and is directly implementable as
  described. This is a real behavior change from `RemediateStaleWorkSession`'s
  existing best-effort pattern, and the plan explicitly says so (Task 3.3.1a's
  final paragraph: "This deliberately diverges from `RemediateStaleWorkSession`'s
  existing best-effort pattern ... only the NEW code in `AutoRespawnAutonomousWork`
  ... adopts the stricter fail-closed check"). Two regression tests are specified
  (3.3.1c confirms-dead path, 3.3.1d fails-closed path).

- **[PASS] Concern 5 — Phase 2 loop composition.** Epic 2.5 (new) adds a
  consolidated final-state pseudocode listing (Task 2.5.1a) showing the checked
  order of all three epics' logic each iteration, and a dedicated interaction
  test (Task 2.5.1b,
  `TestAutonomousDriver_MalformedStreakAtSoftCap_AbortsWithMalformedReason_NotSoftCapExtension`
  plus its inverse) exercising exactly the malformed-streak-at-soft-cap scenario
  the prior review named. I checked the consolidated listing against Epic 2.1's
  and 2.2's own task descriptions (the ordering — soft/hard-cap check first,
  then the higher-priority breaks, then malformed-response handling — matches
  both epics' independent descriptions of where their logic slots into the
  loop, e.g. Task 2.3.1b: "Keep the loop's other break conditions ... exactly as
  updated in Task 2.1.1c — they take priority and fire regardless of
  effectiveMaxTurns"). This is a genuine closure of the integration-risk gap,
  not just a restated assertion.

- **[PASS] Concern 6 — regression test determinism.** Task 1.1.2a adds
  `dequeueSpawnPauseHook`, an unexported `var func(itemID string)`, nil by
  default, invoked only in `DequeueNextQueuedItems` after the CAS claim
  succeeds and before `spawnSessionAfterGates`. I confirmed the cited precedent
  (`paneSettlePollInterval`/`paneSettleMaxWait`, `session/autonomous_driver.go:286-290`,
  documented there as `var`s "not consts ... so tests can" control timing) is
  real and matches the pattern being followed. The new test
  (`TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated`) asserts
  the concurrent `SpawnSessionFromItem` call fails fast with `CodeAlreadyExists`
  while `DequeueNextQueuedItems` is deterministically paused inside the guarded
  window — this is a real, repeatable proof the guard is held, not a
  flake-hunting `-count=20` loop. See New Concerns for the hook's own
  test-hygiene risk.

- **[PASS] Concern 7 — call-site audit artifact.** Story 1.1.3 / Task 1.1.3a adds
  an enumerated comment (not just a claim in the plan prose) naming
  `AutoReopenAfterFailedReview`, `AutoRespawnAutonomousWork`, `AutoReopenForPRFix`,
  `TriggerTriage`'s auto-spawn path, and `DequeueNextQueuedItems`. I independently
  greped every one of these citations and confirmed exact accuracy: `s.SpawnSessionFromItem`
  is called directly at `backlog_service_triage.go:1218` (`AutoReopenAfterFailedReview`),
  `:1309` (`AutoRespawnAutonomousWork`), `:1494` (`AutoReopenForPRFix`), and `:1924`
  (`TriggerTriage`); `DequeueNextQueuedItems` calls `s.spawnSessionAfterGates` directly
  at `:575`. All five call sites are covered by one guard or the other, matching
  the audit comment's claim exactly.

- **[PASS] Concern 8 — `stuckReasonForExitKind` default case.** Task 3.2.1a's
  helper has an explicit `default` branch falling back to the original generic
  text (`"autonomous driver stopped after %d turns without a DONE signal (%s)"`),
  with a doc comment explaining `DriverExitReason` is not a closed enum. Task
  3.2.1b adds a direct unit test of the fallback
  (`TestStuckReasonForExitKind_FallsBackToGenericText_When_ExitKindUnset`). This
  is a genuine, specific fix — the prior review's concern is fully addressed.

**Score: 2/2 prior blockers PASS, 6/6 prior concerns PASS.**

## New Blockers

None found.

## New Concerns

- **`DequeueNextQueuedItems`'s per-item guard release is manual multi-exit-point
  `Delete`, not `defer` — more failure-prone than the pattern it's replacing.**
  Task 1.1.1b explicitly avoids a `defer s.spawnInFlight.Delete(item.ID)` at
  guard-acquisition time (reasoning given: a function-scoped defer would hold
  the guard for every already-processed item in the batch until the whole loop
  returns, needlessly blocking a concurrent `SpawnSessionFromItem` call for an
  *earlier* item). That reasoning is correct as far as it goes, but the fix
  proposed — a `release := func(){...}` called at "every exit point" of the loop
  iteration, or an equivalent manual restructuring — is exactly the pattern
  Go's `defer` exists to make unnecessary: a future maintainer adding one more
  early `continue` to this loop body (e.g. a new short-circuit case) can easily
  forget to call `release()` first, silently leaking that item's guard entry
  forever. Because `spawnInFlight` is a `sync.Map` with no TTL/expiry (confirmed:
  the existing doc comment for the field says it's "self-cleaning ... via defer
  on exit," an invariant this new call site would be the first to violate), a
  leaked entry would permanently block *all* future spawns for that one item —
  both the `SpawnSessionFromItem` guard (which is a completely different
  acquisition, but keyed on the same `item.ID`, so it would also see
  `alreadyInFlight` forever) and every later `DequeueNextQueuedItems` pass. This
  is a quiet, hard-to-detect failure mode (no error is raised — the item just
  silently never spawns again) and is exactly the kind of subtle bug a
  code-review pass is likely to miss, since "did every exit path call release()"
  requires manually auditing every `continue`/`break` in the loop body, not
  running a test. **Recommendation**: wrap the guarded portion of each loop
  iteration in an immediately-invoked closure (`func() { ...; defer
  s.spawnInFlight.Delete(item.ID); ... }()`) so `defer` fires at the closure's
  return, not the enclosing function's — this gets the exact per-item release
  semantics the task wants while keeping `defer`'s "released on every exit,
  including a future maintainer's new exit path or a panic" guarantee. This
  also make the pattern consistent with `SpawnSessionFromItem`'s own guard,
  which does use `defer` today.

- **`dequeueSpawnPauseHook` is a bare package-level `var`, not reset defensively
  if a test panics before its `t.Cleanup` runs.** The plan's precedent
  (`paneSettlePollInterval`/`paneSettleMaxWait`) is a pure timing *value* whose
  worst-case misuse is "a test runs slower or faster than intended" — never a
  correctness hazard, since production code paths still complete. `dequeueSpawnPauseHook`
  is qualitatively different: it is a *control-flow* hook that can block a
  goroutine indefinitely (blocking on a channel per Task 1.1.2a's test
  description). If a test using this hook fails an assertion *before* signaling
  the unblock channel (e.g., the new `CodeAlreadyExists` assertion itself
  fails), the paused `DequeueNextQueuedItems` goroutine leaks for the remainder
  of the test binary's life — holding `dequeueMu` for that entire time, which
  would then cause every subsequent test in the same package that calls
  `DequeueNextQueuedItems` (directly or via a code path that triggers it) to
  hang until the test binary's overall timeout fires, producing a confusing
  failure at a distant, unrelated test rather than at the actual culprit. This
  is a real risk specifically because `t.Cleanup` only fires after the test
  function returns normally or via `t.Fatal`/`t.FailNow` — it does not rescue a
  goroutine parked on a channel read that nothing will ever write to.
  **Recommendation**: give the test itself a bounded-time unblock path (e.g. a
  `select` in the hook itself with a timeout, or asserting inside a `t.Cleanup`-
  registered unconditional-unblock-then-check pattern) so a failed assertion
  can't leave the production method's own mutex held for the rest of the test
  run. This is a test-hygiene concern only — it cannot happen in production,
  where the hook is always nil — but it's the kind of thing that turns a
  legitimate new regression test into a source of unrelated CI flakiness if not
  handled at implementation time.

- **`AutoRespawnAutonomousWork`'s new fail-closed error return changes an
  existing at-least-once-attempt caller's retry semantics — not called out in
  the plan.** Task 3.3.1a has `AutoRespawnAutonomousWork` return a non-nil error
  when `IsSessionLive` still reports the pane alive after the kill attempt, so
  the caller "surfaces the failure instead of silently proceeding." I checked
  the two callers: `onAutonomousDriverComplete`'s `SessionRoleWork` branch calls
  it inside a `go func(){ if respawnErr := respawner.AutoRespawnAutonomousWork(...);
  respawnErr != nil { log.Warn(...) } }()` (confirmed, `autonomous_orchestration_service.go:376-380`)
  — the error is only logged, never retried or fed back into the
  `RemediationDue`/backoff-gate accounting that was already consumed
  synchronously *before* this goroutine was dispatched (line 358's
  `RemediationDue` call happens before the `go func()`, so a failed respawn
  attempt still "spends" one backoff-gated attempt with no compensating
  credit). This means a transient `IsSessionLive` false-positive (pane briefly
  reports live right after a kill signal, before tmux/the OS has actually torn
  it down) will silently consume one of the item's limited
  `MaxRemediationAttempts` slots for zero effect — previously (best-effort,
  proceed-regardless) the same transient condition would not have blocked
  anything. This isn't a correctness bug (the plan's fail-closed behavior is
  the right instinct, and worst case the operator sees the item stall one cycle
  longer and the next remediation pass retries), but the plan doesn't discuss
  this interaction with the backoff-gate accounting at all, and a genuinely
  slow-to-die pane (the exact BUG-042 orphaned-control-mode-client scenario the
  plan itself cites as the motivating hazard) could now burn through several
  attempts via this path before `MaxRemediationAttempts` parks the item, purely
  because `IsSessionLive` hasn't caught up yet. **Recommendation**: at minimum,
  note this interaction explicitly in Task 3.3.1a or Story 3.3.1's acceptance
  criteria, and consider one bounded retry/poll of `IsSessionLive` (e.g. 2-3
  short-interval checks) before giving up and returning an error, rather than a
  single immediate check right after the kill call — `KillTmuxPaneOnly` is
  synchronous but tmux/process teardown is not guaranteed instantaneous relative
  to when `inst.KillSession()` returns.

## Minors

- Epic 3.1's revision note (line 363) says the residual imprecision ("`SessionRoleTriage`'s
  own inline 'Triage stuck' notification ... will still fire on an intentional
  cancellation of a triage-role driver") is acceptable because "the only
  confirmed real cancellation caller (`submit_review_verdict`) only ever targets
  review-role sessions." This is true today, but note it silently assumes no
  future caller of `Stop()`/`StopDriverForSession` targets a triage-role
  session — worth a one-line comment at the `SessionRoleTriage` notification
  site itself (not just in this plan) so a future maintainer adding a new
  cancellation caller knows to re-check this assumption, rather than relying on
  a project-plan document nobody will re-read.
- The consolidated loop listing in Task 2.5.1a is documentation-only (as it
  says), so there is a residual (low) risk the three epics' *actual* landed
  code drifts from this listing if a task subagent takes a slightly different
  but equivalent structuring — this is inherent to prose/pseudocode plans and
  not fixable without generating code at plan time, but Task 4.1.1a's full
  regression pass (`go test ./session/... ./server/services/... -race`) would
  catch any behavioral drift, so this is adequately mitigated, just worth
  naming.
- Story 1.1.1's acceptance criterion citing `backlog_service.go`'s `spawnInFlight`
  doc comment as "lines 138-163" is off by one from the current file (the field
  declaration itself is at line 164, comment block 138-163) — trivially close
  enough that Task 1.1.1c's own caveat ("verify against current line numbers at
  implementation time") already covers it. No action needed beyond what's
  already planned.
