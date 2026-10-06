# Adversarial Review: quota-aware-backlog-gating
**Date**: 2026-08-10
**Verdict**: RESOLVED (re-checked during `sdd:4-validate`, 2026-08-10) — all three Blockers below
are addressed by the current plan.md text (Task 2.2.1a's "Scope correction" note, Task 1.2.1a's
`isLoading` handling in `computeHeadroom`, and Task 2.1.2b's explicit `!hard` resume gate). Concerns
and Minors below remain open as non-blocking follow-ups; none gate implementation readiness.

## Blockers

- [x] **RESOLVED — plan.md Task 2.2.1a now carries a "Scope correction (was a Blocker in
  adversarial review)" note with the corrected `server/dependencies.go:1152-1225` range, and moves
  only the two `tokenStore` construction/`Start` lines early, leaving `backlogSvc.SetTokenStore`/
  ArtifactExtractor wiring in place.** Original finding: **The boot-sequence reordering (Epic 2.2,
  Task 2.2.1a) is mis-scoped and, as literally
  specified, will not compile.** The task cites the block to relocate as
  `server/dependencies.go:1150-1170`, but the actual `if homeDir, homeDirErr := ...; homeDirErr ==
  nil { ... }` block spans **lines 1152–1225** (verified by brace-matching the real file) and
  includes `backlogSvc.SetTokenStore(tokenStore, pricing)` (line 1174) and the ArtifactExtractor
  wiring. `backlogSvc` is declared via `:=` at line 1017 — itself *after* the proposed new
  insertion point (immediately before `syncRegistry := session.NewDefaultRegistry()` at line
  1001). Moving the block "identical" as instructed would reference `backlogSvc` before its
  declaration, a compile error, not a "regression to grep for." Task 2.2.1b's mitigation ("grep for
  regressions, run `make build`") would in fact catch this immediately, but the task as written
  gives the implementer wrong line numbers and an instruction ("keep everything inside the block
  identical") that is impossible to fulfill without further design decisions the plan never makes
  (what exactly moves vs. stays, and where `backlogSvc.SetTokenStore` gets called from once split).
  **Recommendation**: rewrite Task 2.2.1a to move only the `tokenStore := tokens.NewTokenStore(...)`
  + `tokenStore.Start(...)` pair (the two lines `QuotaGate` actually needs early), explicitly leave
  the `backlogSvc.SetTokenStore`/InsightsService/ArtifactExtractor wiring at its current position,
  and correct the cited line range.

- [x] **RESOLVED — plan.md's Task 1.2.1a `computeHeadroom` now takes an `isLoading bool` parameter
  and forces `Valid: false` immediately when true, closing the boot-time/restart-adjacent gap (see
  Domain Glossary's `computeHeadroom` entry and Task 2.1.2b's call site passing
  `g.tokenStore.IsLoading()`).** Original finding: **Even with the reordering fixed, the boot-time synchronous `Reconcile` call (Task 2.2.2a)
  provides no real protection — both signals are structurally guaranteed to be empty/near-empty at
  that exact call site.** `TokenStore.Start()` (`session/tokens/store.go:65-76`) launches the
  initial directory walk in a background goroutine (`go ts.walkAndEnqueue(ctx)`) and returns
  immediately; `GetAll()` (`store.go:99-110`) returns whatever's in the cache *right now*, with no
  wait and no check of `IsLoading()`. A `Reconcile` call fired synchronously moments after `Start()`
  will see a cache that's empty or barely populated regardless of the user's actual quota state, so
  `computeHeadroom` will read as artificially healthy every time. The plan's own Task 2.2.2a
  acceptance criterion independently concedes the hard signal is *also* always empty at boot
  ("this can't happen pre-boot since `rateLimits` starts empty every process start"). So the one
  concrete benefit Epic 2.2 is built to deliver — "if quota state is already bad, it's disabled
  again within the same boot sequence rather than waiting up to 60s" — cannot happen with either
  signal as currently wired. This isn't a minor edge case; it's the stated purpose of the epic's
  riskiest task (the reordering above), and neither `computeHeadroom` nor `Reconcile` reference
  `TokenStore.IsLoading()` anywhere in the plan. **Recommendation**: either drop the boot-time
  synchronous `Reconcile` call and its reordering prerequisite entirely (rely on the 60s ticker,
  which is what actually protects against extended bad-quota windows), or have `computeHeadroom`/
  `Reconcile` treat `tokenStore.IsLoading() == true` as `!estimate.Valid` (skip soft-signal
  evaluation, matching the existing "uncalibrated" no-op path) so the boot call is at least honest
  about not having data yet instead of silently reading "healthy."

- [x] **RESOLVED — plan.md's Task 2.1.2b now gates the resume branch on `!hard` unconditionally
  (explicitly flagged inline as "the BLOCKER fix"), and Story 2.1.2/2.1.3 add the requested GWT case
  plus Task 2.1.3c's dedicated regression test asserting `Enable()` is never called on any tick while
  `hard` remains true.** Original finding: **`Reconcile`'s hard-override guard (`hard && backlogCtrl.IsEnabled()`, Task 2.1.2a) lets the
  soft-signal resume path fire while the hard/reactive signal is still active**, contradicting
  ADR-001's explicit design ("hard signal ... unconditional, hysteresis-free hard override, always
  wins") and the Pattern Decisions table's stated control flow (`if hard { ... } else if soft { ...
  }` — a plain mutual exclusion, not one additionally gated on `IsEnabled()`). Walk the sequence:
  tick N — hard fires, `IsEnabled()` was `true`, so `hard && IsEnabled()` is true, `Disable()` runs,
  `pausedByQuota = true`, early return. Tick N+1 — the same rate-limit event is still inside
  `RateLimitWindowMinutes` (`hard` is still `true`), but `IsEnabled()` is now `false` (already
  paused), so `hard && IsEnabled()` is **false** — the early-return guard in 2.1.2a doesn't fire,
  and per Task 2.1.2b's own text ("If not hard-overridden: compute estimate ...") execution falls
  through into the soft-threshold logic. If headroom happens to read healthy (plausible — see the
  boot-time finding above, or simply because the rate limit was per-session/local and the
  aggregate-window heuristic is independently healthy), the soft-resume branch's condition
  (`!IsEnabled() && pausedByQuota && PctRemaining >= resumeThreshold`) is satisfied and, after
  `ConsecutiveTicksToResume` ticks, **`Enable()` fires while a rate limit is still within its
  window** — the exact scenario ADR-001 says the hard signal exists to prevent. Requested stress
  test #4 ("Reconcile hard-override + soft-threshold interaction when BOTH are true simultaneously")
  has no corresponding acceptance criterion or test case anywhere in Stories 2.1.2/2.1.3 — only
  hard-alone and soft-alone paths are covered. **Recommendation**: change the guard to gate resume
  eligibility on `!hard` unconditionally (not `!(hard && IsEnabled())`), and add the GWT case:
  "given `pausedByQuota==true`, hard signal still active, and healthy headroom for
  `ConsecutiveTicksToResume` ticks, `Enable()` is never called while `hard` remains true."

## Concerns

- [ ] **ADR-001's combined-signal design ships with its proactive half permanently unverified at
  merge time**, which undercuts the stated rationale for choosing the more complex Option C over
  the simpler, already-converged-on Option B. `AssumedWindowTokenBudget` defaults to `0` (soft
  signal inert) per Risk Control, and Phase 4's live smoke test (Epic 4.2, Story 4.2.1) only
  exercises the hard/reactive path (`recordRateLimitEvent` debug hook) — no task calibrates a real
  `AssumedWindowTokenBudget` and runs `computeHeadroom` against genuine `TokenStore` data before
  ship. So at the moment this merges, the feature *is* functionally Option B (reactive-only) while
  having paid the full build/test/review cost of Option C's two-signal design, its hysteresis
  interaction complexity (see Blocker 3), and the extra `status_detail`/notification wording branch
  for a signal nobody has run live yet. Given the appetite is Medium (1–2 weeks), this is a
  meaningful chunk of budget spent on a code path with zero live verification and a required
  post-ship manual step (picking a budget number) before it does anything more than the fallback
  increment requirements.md already scoped as acceptable on its own.

- [ ] **`computeHeadroom` never checks `TokenStore.IsLoading()`** even outside the boot-time case —
  any restart-adjacent reconcile tick (not just the very first one) can read a partially-populated
  cache while the background walk is still catching up on a large `~/.claude/projects/` history,
  silently under-counting usage and over-reporting headroom during that window. No task or
  acceptance criterion addresses this failure mode (see Blocker 2 for the boot-specific instance).

- [ ] **Epic 3.2 (`status_detail` + Settings UI status line) is arguably broader than
  requirements.md's explicit scope.** Success Metrics and the Scope section describe visibility as
  "notifications... not a persistent browsable view," with an explicit carve-out: "unless research
  finds this trivial to fold into existing capacity-alert UI." The plan instead adds a new proto
  field, a new `FeatureFlagService` provider-registration mechanism, and new frontend
  render/test code — folded into the existing generic Feature Flags row (not the capacity-alert UI
  the carve-out names), and is a non-trivial four-story epic (3.2.1–3.2.4, ~10 tasks) inside a
  1–2 week appetite. Defensible as a UX improvement, but it's additive scope beyond the literal
  ask and should be confirmed with the user rather than assumed, especially given Blocker 1/2 above
  already put pressure on the appetite.

- [ ] **`Epic 2.2`'s Story 2.2.3 ticker relocation is a second, independent large reordering** (the
  `go func(){ ticker := time.NewTicker(...) ... }()` block moves from before `backlogCtrl` exists to
  after `quotaGate`/`SetQuotaGate` are constructed) layered on top of Blocker 1's `TokenStore` move
  in the same file, same PR. Two structural reorderings of boot-critical goroutine-launch code,
  verified only by "grep + `make build`," is thin verification for a file this central — a
  regression here (e.g. a goroutine capturing a variable before its final value, or a subtle
  behavior change in when `ReconcileStuck` starts relative to other initialization) would not
  necessarily show up as a compile error the way Blocker 1 does. Recommend an explicit integration
  test (or at minimum a manual boot-log walkthrough) asserting relative ordering of the log lines
  each block emits, not just a successful build.

## Minors

- The `status_detail` proto field *is* fully specified as a story with tasks (Epic 3.2, Stories
  3.2.1–3.2.4) — the review brief's concern that it might be "just named in passing" in the Domain
  Glossary does not hold; it's properly built out. (Noted as a non-finding for completeness.)
- `BacklogController.Disable()` (`session/feature_controller.go:69-86`) was verified directly
  against source, not just asserted: it only calls `listener.SetEnabled(false)` and cancels/stops
  the `SyncLoop` — it never touches a live `*session.Instance`. The plan's Non-Goal claim in Epic
  4.1 is accurate.
- `events.NewNotificationEvent`'s signature (`pkg/events/types.go:223-245`) and the
  `backlog_notifier.go` synthetic-key coalescing precedent (`itemID` as `sessionID`,
  `server/notifications/subscriber.go:141-154` explicitly documents this pattern) both check out —
  Task 3.1.1a's call shape is accurate, not just plausible-sounding.
- Task 2.1.2a leaves the choice of "implement the real notify call now vs. a `// TODO(3.1)` stub"
  to the implementer's judgment mid-task ("prefer implementing the full call here... if sequencing
  tasks strictly") — fine for a single-PR feature, but worth tightening to one explicit choice so
  two different implementers (or subagents, if run via `subagent-driven-development`) don't diverge
  on ordering.
