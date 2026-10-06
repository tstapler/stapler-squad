# Architecture Review: stale-session-detection
**Date**: 2026-08-06
**Verdict**: CONCERNS

## Resolution Log (post-review plan.md edits)

- **Blocker (config-live-reload / Risk Control's false "no restart needed" claim) —
  RESOLVED.** Same underlying finding as adversarial-review.md's BLOCKER (identical
  root cause, independently found by both reviewers). `StaleSessionNotifier` no longer
  captures a `*config.Config` pointer; `checkAll()` calls `config.LoadConfig()` as its
  first line every tick (Task 4.1.1a/4.1.1c) — exactly this review's recommended
  remediation. Risk Control section's claim was rewritten to match. See
  adversarial-review.md's Resolution Log for the fuller account and the new test added
  to close it.
- Concerns (`IsStale` helper-extraction citation precision, uncoordinated frontend/
  backend 60s ticks) left as documented, non-blocking debt per this review's own
  verdict (CONCERNS, not BLOCKED) — the Observability Plan now carries a one-line note
  on the tick-skew Concern (see plan.md).

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repository
(verified: `test -f` returns not found). No constitution-derived blockers apply;
skipping this section's hard-constraint gate.

---

## Blockers

- [ ] **Epic 4.1/4.2 (`StaleSessionNotifier`) + ADR-001/plan.md Risk Control section** —
  the plan claims `notify_enabled: false` (and any threshold edit) applied via the
  Settings UI disables/reconfigures `StaleSessionNotifier` "without a restart needed
  beyond the config reload already supported by `config.LoadConfig()`"
  (`plan.md` Risk Control). This is false given the wiring pattern the plan explicitly
  copies. `DefaultsService.UpdateGlobalDefaults` (`server/services/defaults_service.go:122`)
  calls `cfg := config.LoadConfig()` — a **fresh** object read from disk on every RPC
  call — mutates and saves *that* object, and is disk-durable but never touches the
  long-lived `*config.Config` pointer captured once at server startup
  (`server/server.go:607`, `cfg := config.LoadConfig()`) and handed to
  `NewHibernationSweeper`/`NewSessionRetentionSweeper` (`server.go:684,697`) — the exact
  precedent Epic 4.1's `NewStaleSessionNotifier(poller, cfg, eventBus)` (Task 4.2.1a)
  is modeled on. The codebase already hit this exact bug class for `BacklogService`'s
  `MaxAutoReworkIterations`/`MaxConcurrentBacklogWorkItems` fields and had to patch it
  with an explicit manual propagation block (`defaults_service.go:142-151`,
  `SetSharedBacklogConfig`, cited in-code as "PR #199 review F1"). That propagation
  block only re-syncs those two specific fields — it says nothing about
  `StaleSession.ThresholdMinutes`/`NotifyEnabled`, and the plan doesn't add them to it
  or any equivalent mechanism. Net effect: a `StaleSessionNotifier` constructed with the
  startup-time `cfg` pointer will use the config value it was started with **for the
  life of the process**, silently ignoring any later Settings UI change — including the
  documented `notify_enabled: false` kill switch this plan's own Risk Control section
  relies on as the rollback/safety mechanism, and including the manual-smoke-test step
  in Task 4.2.1b, which lowers the threshold via config edit and expects it to take
  effect without restart.
  - **Remediation**: Have `StaleSessionNotifier.checkAll()` call `config.LoadConfig()`
    at the top of each tick (mirrors `GetSessionDefaults`'s already-correct
    per-request-fresh-read pattern at `defaults_service.go:86`) instead of storing a
    captured `cfg *config.Config` pointer on the struct — cheap, since it's a local JSON
    file read on a 60s ticker. This is simpler than extending `DefaultsService`'s
    `sharedBacklogCfg`-style manual propagation to a third field pair. Either fix the
    claim (state a restart is required, contradicting the plan's own stated design
    goal) or fix the mechanism — don't ship the plan's current text, which asserts a
    behavior that doesn't hold for the wiring pattern it specifies.

## Concerns

- [ ] **Epic 4.1 (`Task 4.1.1a`) vs. `build-vs-buy.md` §4** — `build-vs-buy.md` (Phase 2
  research this plan is required to stay consistent with, per this review's lens 11)
  explicitly recommends: "extract the comparison (not the threshold value) into one
  small shared helper function in the `session` package, parameterized by threshold,
  and have the new card-badge/grouping/notification/approval-rule consumers call it."
  The plan's Domain Glossary lists `IsStale` as a "candidate shared Go helper," then
  Task 4.1.1a explicitly declines to extract it, citing "extracting a helper for exactly
  one new caller would be premature" per the interface-pollution-checklist's
  "unjustified generic" smell — but that checklist smell is about *generic type
  parameters* at a single call site, not plain helper-function extraction, so the
  citation doesn't quite support the conclusion (though the conclusion itself is
  reasonable: `pkg/classifier.matchesRule`'s comparison operates on already-converted
  `int` minutes in a package that structurally cannot import `session/` per Pattern
  Decisions row 3, so a `func IsStale(time.Time, time.Duration) bool` genuinely could
  only serve the one `StaleSessionNotifier` call site — build-vs-buy.md's four-consumer
  framing doesn't account for that boundary). The plan should say this explicitly —
  add a sentence to ADR-001 or Task 4.1.1a's note reconciling *why* it diverges from
  build-vs-buy.md's literal recommendation (classifier/session package boundary makes a
  shared Go helper unreachable from 2 of the 4 envisioned consumers, and the frontend 2
  are already covered by the shared TS helper) rather than leaving a bare "revisit if a
  second caller appears" note that reads as an unexplained deviation from Phase 2
  research to a future reader applying this project's own "rationales need citations
  too" discipline.

- [ ] **Epic 2.3 / `SessionCard.tsx`, `strategies.ts` — time-relative staleness as a pure
  render-time function with a client tick, no server-side debounce** — `isSessionStale`
  is a pure comparison (`Date.now() - timestampMs > thresholdMinutes * 60_000`)
  re-evaluated every 60s by the new tick in `SessionList.tsx` (Task 2.3.1b). This is
  fine for the badge/grouping (informational, explicitly no-flag per Pattern Decisions),
  but `StaleSessionNotifier` (Go side, independent 60s ticker) and the frontend tick are
  two *independently scheduled* re-evaluations of conceptually the same "did this
  session just cross the threshold" event, with no coordination between them (a session
  could show the "Stale" badge for up to ~60s before or after the backend notification
  fires, or vice versa). Not a correctness bug — both converge — but worth a one-line
  note in the Observability Plan or ADR-001 acknowledging the two clocks are
  intentionally uncoordinated (matching this project's own "consistent-with is not
  because-of" evidence discipline: don't let a reader assume badge-appears and
  notification-fires are the same event just because they share a threshold value).

## Nitpicks

- `StaleSessionConfig.ThresholdMinutes int` / `ApprovalRuleProto.min_session_idle_minutes
  int32` are raw primitives (minutes as bare `int`/`int32`), not a named
  `type IdleMinutes int32`. This is a deliberate, explicitly-justified choice (Pattern
  Decisions table: matches `max_auto_rework_iterations`/`require_ci_passing` sibling
  field convention, avoids introducing the only `Duration`-typed field in
  `ApprovalRuleProto`) — correctly documented as a consistency trade-off rather than an
  oversight, so this is a nitpick, not a concern: a future pass consolidating all four
  staleness thresholds (already flagged as accepted future debt in ADR-001's
  Consequences) would be a reasonable point to introduce a shared minutes/duration type
  across all four, but doing it here alone would make this feature's fields
  inconsistent with every sibling field in the same proto messages.
- `Instance.GetStatus()` returns a raw `int` (`session/instance_state.go:257`), forcing
  Task 4.1.1c's pseudocode to write
  `inst.GetStatus() != int(sessionv1.SessionStatus_SESSION_STATUS_ACTIVE)` — an
  int-to-int comparison via an explicit cast from the proto enum, rather than comparing
  two values of the same enum type. This is pre-existing debt in `session.Instance`'s
  API (not introduced by this plan), and the plan correctly conforms to the existing
  convention rather than inventing a one-off typed comparison for this feature alone —
  no action needed here, flagged only for awareness given `session/review_queue_poller.go`
  and `backlog_service_triage.go` do the same cast today.
- ADR-001's Decision 2 zero-collision analysis ("0 means unknown" and "0 means
  genuinely fresh" both correctly fail an `idle < threshold` check) is correct for the
  current `MinSessionIdleMinutes > 0 && ctx.SessionIdleMinutes < threshold` comparison
  direction, but is a fragile invariant: any *future* rule condition using the inverse
  sense (e.g., "match only brand-new sessions, idle < N minutes") would silently
  conflate "unknown" with "genuinely fresh" and produce a false match instead of failing
  closed. Not actionable now (no such condition is being added), but worth a one-line
  comment at the `ClassificationContext.SessionIdleMinutes` field declaration (Task
  5.3.1b) warning future authors of directional conditions about this collision, since
  the current fail-closed guarantee depends on comparison direction, not just on the
  zero value itself.
