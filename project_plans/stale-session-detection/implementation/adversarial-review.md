# Adversarial Review: stale-session-detection

**Date**: 2026-08-06
**Verdict**: CONCERNS (was BLOCKED; re-reviewed after plan.md fix — see Resolution Log)

## Resolution Log (post-initial-review plan.md edits)

- **BLOCKER (config never reaching running `StaleSessionNotifier`) — RESOLVED.**
  `StaleSessionNotifier` no longer captures a `*config.Config` pointer; `checkAll()`
  now calls `config.LoadConfig()` as its first line every tick (Task 4.1.1a/4.1.1c),
  matching remediation option (b) from the original finding. Risk Control section's
  claim rewritten to match. A new test (Task 4.1.1e,
  `checkAll_should_ObserveConfigChange_When_ConfigFileChangesBetweenTicks`) closes the
  "no test asserting live reload" gap the original finding called out. Re-reviewed by a
  scoped subagent pass — confirmed no stale reference to the old constructor shape
  survives elsewhere in plan.md.
- **Concern (pause/resume dedup never cleared) — RESOLVED.** `checkAll()`'s non-ACTIVE
  branch now clears the `notifiedSessions` entry on any ACTIVE→non-ACTIVE transition
  (not only on idle-time recovery) before `continue`-ing. New test:
  `checkAll_should_ReNotify_When_SessionPausesThenResumesStillStale` (Task 4.1.1e).
- **Concern (Status header "Ready for implementation" contradicted an open
  human-owned naming decision) — RESOLVED.** ADR-001 Decision 2 (Status: Accepted)
  settles the approval-rule field name as `min_session_idle_minutes`; ADR-001's
  Alternatives-Considered clause was corrected to remove language describing this as
  still pending confirmation (it previously read as an open gate even though the ADR's
  header already said Accepted — an internal inconsistency, now fixed). Unresolved
  Questions in plan.md is now "None."
- **Remaining Concerns not fixed (deliberately, per repair-loop scope — Blockers only
  require a fix; Concerns are noted debt, consistent with `.claude/rules/`'s "no
  silent caps" but not blocking):** frontend `useStaleSessionConfig` still fetches once
  on mount with no refetch-on-save (Concern 3 below), and the notifier has no periodic
  heartbeat log beyond startup + per-fire (Concern 4 below). Both remain open as
  documented, lower-risk debt.

---

## Original Review

## Summary — the four scrutiny questions

**Does the fourth threshold repeat the 37/41 incident?** Not on its own — Decision 1's
rationale table is honest ("best-effort estimate," explicitly not reused from the other
three) and correctly avoids the root cause of the original incident (a threshold tuned
for one purpose reused for another). *However*, the BLOCKER below (config changes never
reach the running `StaleSessionNotifier`) recreates the **preconditions** for a similar
incident through a different mechanism: a user who finds the 30-minute default too noisy
and lowers it via Settings will see no effect on the live notifier until a restart, and
may reasonably conclude the fix "didn't work" and try increasingly aggressive values —
exactly the kind of uncoordinated, undocumented tuning-by-trial-and-error ADR-001
(review-gate-stale-session-rework) was written to stop.

**Does the notify-once dedup hold up?** The edge-triggered/re-arm design is correct for
the tested case (idle → stale → produces output → recovers → re-arms). It is **not**
correct for pause/resume: `checkAll()`'s per-instance loop (Task 4.1.1c) skips
non-`ACTIVE` instances with an early `continue`, before reaching the mu-guarded
clear-on-recovery branch — so a session that gets notified stale, is then paused, and is
later resumed still past threshold will never re-notify, because its `notifiedSessions`
entry was never cleared while paused. See Concerns.

**Does the approval-rule fail-closed claim hold, or could unset fields fail open?**
Verified against the actual code, not just the ADR's description — it holds. `matchesRule`'s
planned check (`rule.MinSessionIdleMinutes > 0 && ctx.SessionIdleMinutes < int(rule.MinSessionIdleMinutes)`
→ `return false`) means a zero-value `ctx.SessionIdleMinutes` (both the "no live instance"
case *and* the ordinary "session genuinely just produced output" case collapse to the
same `0`) can never satisfy a `MinSessionIdleMinutes > 0` condition — the rule fails to
match and falls through, exactly as ADR-001 Decision 2 claims. `BuildContext` allocates a
fresh `ClassificationContext` per call (`pkg/classifier/classifier.go:652`), so there's no
pooled-struct leakage between requests either. This part of the plan is sound.

**Does the 60s re-render tick have cleanup/perf risk?** No material risk found. The
pattern matches 9 existing `setInterval` hooks under `web-app/src/lib/hooks/` (close to
the plan's claimed "10"), Task 2.3.1b specifies cleanup on unmount, and recomputing
`isSessionStale` over 5-10 in-memory sessions every 60s is negligible cost. Not flagged
below.

## Blockers

- [ ] **`StaleSessionConfig` changes never reach the running `StaleSessionNotifier` —
      the plan's stated kill switch and "no restart needed" rollback claim are false as
      scoped, and this reproduces an already-diagnosed bug class in this exact codebase.**
      `server/server.go:607` loads `cfg := config.LoadConfig()` **once** at startup and
      passes that single long-lived pointer to `HibernationSweeper`/
      `SessionRetentionSweeper` (and, per Task 4.2.1a, would pass it to
      `StaleSessionNotifier` too). `UpdateGlobalDefaults`
      (`server/services/defaults_service.go:122`) does its own **fresh**
      `cfg := config.LoadConfig()`, mutates and `SaveConfig`s *that* copy, and never
      touches the startup pointer — confirmed by `config.LoadConfig()`
      (`config/config.go:782`) re-reading `config.json` from disk on every call, with no
      cache, no `SIGHUP`/reload handler, and no generic "hot config" mechanism anywhere
      in the codebase (`grep` for `ReloadConfig`/`SIGHUP` returns nothing). This exact
      staleness pattern was already found and fixed — narrowly, for exactly two fields —
      by `DefaultsService.sharedBacklogCfg`/`SetSharedBacklogConfig`
      (`server/services/defaults_service.go:35-55,142-151`), whose own doc comment cites
      "PR #199 review F1" and explicitly warns: *"That pattern alone is why
      BacklogService's long-lived cfg pointer... never observed a raised WIP cap until a
      restart: nothing ever wrote back into BacklogService's instance."* Task 1.2.1d
      does not add `StaleSession.ThresholdMinutes`/`NotifyEnabled` to that (or any)
      propagation mechanism. Net effect: a user who toggles "notify_enabled: false" in
      Settings to silence a noisy notifier (Risk Control's own stated escape hatch) will
      see the change persist to `config.json` and reflected in `GetSessionDefaults`
      responses (so the Settings form itself looks like it "took"), while the actual
      background `StaleSessionNotifier` keeps firing on the old in-memory value until
      the process restarts — and per `.claude/rules/tmux-keep-server-on-restart.md`, a
      restart of this service kills every live tmux session unless
      `--tmux-keep-server` is passed, making "just restart" a nontrivial ask on this
      project's own deployment.
      **Recommendation**: either (a) wire `StaleSession.ThresholdMinutes`/`NotifyEnabled`
      through the same `sharedBacklogCfg`-style shared-instance propagation
      `UpdateGlobalDefaults` already has for `MaxAutoReworkIterations`/
      `MaxConcurrentBacklogWorkItems`, or (b) have `StaleSessionNotifier.checkAll()`
      call `config.LoadConfig()` itself each tick (cheap — it's a local file read every
      60s, matching `GetSessionDefaults`'s own pattern) instead of holding a
      construction-time snapshot. Update the Risk Control section's claim once fixed, and
      add a test asserting a config change is observed by the *running* notifier without
      a process restart — the current plan has no such test.

## Concerns

- [ ] **Notification dedup is never cleared for a session that leaves `ACTIVE` status,
      so a notified-stale session that is paused and later resumed (still idle past
      threshold) silently fails to re-notify.** `checkAll()`'s design (Task 4.1.1c) is:
      `for inst := range GetInstances(): skip (continue) if status != ACTIVE; else
      under mu: if idle > threshold and not in notifiedSessions, notify + record; if
      idle <= threshold and in notifiedSessions, delete (re-arm)`. The `continue` for
      non-`ACTIVE` instances means the re-arm/clear branch is unreachable while paused —
      the entry simply sits in `notifiedSessions` unchanged. Story 4.1.1's own
      acceptance criteria test the ACTIVE→PAUSED transition only for "no *new*
      notification fires" (correct), never for "does the map entry get cleared / does a
      *later* resume-while-still-stale re-fire" (it won't, and nothing in the plan tests
      or acknowledges this). Pause-then-resume is a normal, expected user workflow
      (Settings/manual pause to think, then resume), not an edge case — a user who
      dismisses one stale notification, pauses to intervene, then resumes and walks away
      again will get no second warning even though the session is, by the feature's own
      definition, freshly stale again.
      **Recommendation**: clear (or at least re-evaluate) the `notifiedSessions` entry on
      any ACTIVE→non-ACTIVE transition, not only on idle-time recovery, so a later
      ACTIVE-and-still-stale observation re-fires. Add a test for exactly this sequence.

- [ ] **The plan's own Status header ("Ready for implementation") contradicts its
      Unresolved Questions section, which names a real, task-blocking, human-owned
      decision still outstanding.** The proto field name for the approval-rule condition
      (`min_session_idle_minutes` vs. the source issue's `session_age_minutes`) is
      explicitly marked "blocks Story 5.1 (proto field naming) — owner: Tyler (confirm
      before Task 5.1.1a)." A plan that gates a specific implementation task on an
      unconfirmed decision from a named human owner is not actually "ready" for that
      phase — starting Phase 5 before this is confirmed risks a mid-implementation
      rename across proto/ent/classifier/frontend (5+ files per Epics 5.1-5.5), the
      exact kind of rework SDD's validate-before-implement gate exists to prevent.
      **Recommendation**: resolve this before Phase 5 work starts (a one-line
      confirmation), or explicitly scope Phase 5 as blocked/deferred in the plan rather
      than implicitly bundled into an overall "ready" status.

- [ ] **Frontend threshold/notify config is fetched once on mount
      (`useStaleSessionConfig`, Task 2.3.1a) with no re-fetch or invalidation on save.**
      A user who changes the threshold via `GlobalDefaultsForm` (Epic 1.3) while a
      `SessionList` is already open in another tab/window won't see the badge/grouping
      react to the new value without a manual page reload. This is a smaller instance of
      the same "config change doesn't propagate to already-running consumers" family as
      the Blocker above, and should be fixed with the same mental model in mind — e.g.
      invalidate/refetch on the `UpdateGlobalDefaults` response, or poll
      `useStaleSessionConfig` on the same cadence as the Epic 2.3 tick.

- [ ] **No observability for the notifier's own liveness beyond a single startup log
      line.** Per the Observability Plan, `StaleSessionNotifier.Start()` logs once at
      startup and once per notification fire — there's no periodic heartbeat/health log
      to notice if the ticker goroutine silently stops (panic-recovered exit, deadlock on
      `n.mu`, etc.). Given single-user/no-alerting is an accepted constraint elsewhere in
      this codebase, this is not a blocker, but worth a one-line periodic debug log
      (e.g. every N ticks) so a `journalctl` grep can distinguish "notifier alive, no
      stale sessions" from "notifier died silently."

## Minors

- `notifiedSessions map[string]time.Time` has no eviction path for sessions that are
  deleted/archived while stale-and-notified (only cleared on recovery-while-still-`ACTIVE`
  or, presumably, process restart) — unbounded but slow-growing memory on a long-uptime
  single-user instance; low real-world impact, worth a TODO if the map is ever observed
  growing.
- Frontend badge uses a strict `>` comparison
  (`Date.now() - timestampMs > thresholdMinutes * 60_000`) while the backend classifier
  condition is effectively inclusive at the boundary (`ctx.SessionIdleMinutes <
  rule.MinSessionIdleMinutes` fails, i.e. `>=` matches). The plan already flags this as
  an open boundary-case decision for Task 2.1.1d to pin down — since the two live on
  independently-configured threshold values by design, this is cosmetic, not a
  correctness risk, but should be resolved and documented rather than left implicit.
- `int(inst.GetTimeSinceLastMeaningfulOutput().Minutes())` (Task 5.4.1a) truncates toward
  zero, so a session idle 59m59s reads as `59`, one minute short of a `MinSessionIdleMinutes:
  60` rule. Harmless at this threshold granularity; undocumented.
- Epic 1.3 (Settings UI form fields) is scope beyond requirements.md's literal ask
  ("config-driven... surfaced in `config/`" — satisfiable by hand-editing `config.json`
  alone). It's low-effort and follows an existing form's exact pattern, so not worth
  blocking on, but it is the one place this plan's scope grew past the stated minimum.
