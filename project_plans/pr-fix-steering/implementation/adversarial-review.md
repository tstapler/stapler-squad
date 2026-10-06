# Adversarial Review: pr-fix-steering
**Date**: 2026-08-26
**Verdict**: CONCERNS

## Prior Blockers — All Verified Fixed

- **Unbounded `SendCommandImmediate` hang leaking `steerInFlight`.** Verified against the live
  file: `server/services/session_service.go:2944-2971`'s sibling non-autonomous branch already
  bounds `SendKeys` with a goroutine + `context.WithTimeout(ctx, 5*time.Second)` + `select`
  (comment at `:2950-2952`); the autonomous branch (`:2934-2943`) is still today's unbounded
  `log.Warn`-and-continue. Task 1.1.2a's code sketch mirrors the sibling pattern exactly for the
  autonomous branch, wraps the timeout error around `timeoutCtx.Err()` so `UpdateSession`'s
  `errors.Is(err, context.DeadlineExceeded)` routes it correctly, and Task 1.1.2b adds
  `TestSteerInstance_AutonomousSendCommandImmediateHangs_TimesOutRatherThanBlockingForever`
  (asserts `errors.Is(err, context.DeadlineExceeded)` on a controller that blocks past the
  timeout). Story 1.1.2's acceptance criteria include the matching GWT. Fixed.
- **`buildReasonSignature` empty-header false-dedup.** Verified the real call site
  (`session/backlog_lifecycle_pr.go:1395`) builds exactly the header-less
  `fmt.Sprintf("PR #%d (%s) was closed without merging...")` string the plan's Task 2.1.1c
  models its fixtures on. Task 2.1.1b's fallback (`if len(headers)==0 { headers =
  []string{strings.TrimSpace(fixContext)} }`) makes two *different* header-less messages produce
  different single-element signatures (different trimmed strings → `.equal()` false) and two
  *identical* header-less messages produce the same one (`.equal()` true) — the exact two
  behaviors required. Story 2.1.1's acceptance criteria and Task 2.1.1c's two named tests
  (`..._DifferentMessagesProduceDifferentSignatures` / `..._IdenticalMessagesProduceEqualSignatures`)
  cover both directions. Fixed.
- **Story 1.1.2 silently breaking the "Give Direction" dialog.** New Story 1.1.3 fixes
  `handleSteerAutonomousSession` (`web-app/src/app/page.tsx:292-295`, verified unchanged from the
  prior review's description) and `SessionActionsOverflow.tsx`'s Enter/Send handlers (`:504`,
  `:519`, verified unchanged) to await the result and only close/clear on success. The fix reuses
  `useNotifications()`'s `addNotification` — verified this is a real, already-used mechanism, not
  invented: `SessionList.tsx:429,890,897` imports and calls it the same way for a failed bulk
  delete. The Dependency Visualization (plan.md:83-86) and Epic 1.1's goal text were updated to
  read "SessionService + its web-app caller" / "...and fix the one existing web-app caller,"
  so Phase 1 is no longer framed as server-only. Fixed, with one residual type-accuracy nit —
  see Minors.

## Blockers

None — all prior blockers resolved and verified.

## Resolved Since This Review Round

**Not re-verified by this review pass itself — fixed in a plan.md revision made after this
review ran, and confirmed true by re-reading the current plan.md before writing this note (see
`.claude/rules/fix-flaky-tests-dont-defer.md`-style discipline: don't silently drop a stale
finding without saying why).**

- **The `SteerFailed`/`RespawnBlockedActive` mutual-exclusion fix's non-atomicity across two
  overlapping reconcile ticks (originally flagged below as a "New finding").** The concern was:
  the `resolveSteerFailedLogged` calls in the three degrade branches (nil `sessionSteerer`,
  not-live, dedup/debounce suppression) executed *before* `steerInFlight`'s `LoadOrStore` guard,
  which only wrapped the delivery path — so a degrade-branch tick and a concurrent
  success/failure-branch tick could interleave their `MarkStuck`/`ResolveStuck` calls with no
  synchronization, letting both `StuckReasonSteerFailed` and `StuckReasonRespawnBlockedActive`
  end up open simultaneously (exactly what Story 4.3.2/ADR-002 exist to prevent). **Fix,
  verified**: plan.md's Task 4.2.1a now performs `steerInFlight.LoadOrStore` as the very first
  statement of `steerActiveSessionForPRFix`, with `defer steerInFlight.Delete` immediately after
  — ahead of the nil-`sessionSteerer`/not-live checks (Task 4.2.1a) and the
  dedup/debounce checks (Task 4.2.1b), not just the delivery call (Task 4.2.1c). The guard now
  covers the method's entire body, per pre-mortem.md's P2 #2 finding, which was applied to
  plan.md after this adversarial-review round had already been written. Task 5.3.1h's
  `..._SteerInFlight_PreventsDuplicateConcurrentSend` test (including its degrade-path sub-case)
  is the regression coverage for this.

- **`StuckReasonSteerFailed`'s new chip label/icon/CSS class vs. requirements.md's "Out of
  Scope: New UI for steer history" (originally flagged below as a "Carried forward, still
  unaddressed" Concern).** The concern was: ADR-002's Consequences section only asserted "this
  is called out here specifically so it isn't mistaken for scope creep" without reconciling that
  assertion against the requirements doc's own out-of-scope line. **Fix, verified**: ADR-002's
  Consequences section (`project_plans/pr-fix-steering/decisions/ADR-002-steer-failed-stuck-reason.md`,
  final bullet) now resolves this directly rather than disclaiming it — verified against the
  actual `BlockerChip.tsx` component (`web-app/src/components/backlog/BlockerChip.tsx`): it
  renders purely off `item.reason` via `getStuckReasonIcon`/`getStuckReasonLabel`/
  `getStuckReasonClass` (`stuckReason.ts`'s `Record<StuckReason, T>` lookup maps), the same
  generic mechanism that already renders `StuckReasonRespawnBlockedActive` today. Adding
  `StuckReasonSteerFailed` is one more entry in that existing lookup table, not a new component,
  screen, or history/timeline view — which is what requirements.md's "Out of Scope: New UI for
  steer history" actually excludes. The ADR also names its own reconciliation's shelf life: if a
  future implementer finds `BlockerChip.tsx` has been refactored to require new component code
  per `StuckReason` (not just a new map entry), this reconciliation no longer holds and should be
  re-flagged.

## Concerns

None outstanding — the one open concern from this review round (the `StuckReasonSteerFailed`/
`BlockerChip` scope-tension item) was resolved; see "Resolved Since This Review Round" above.

**Resolved and verified** (of the 3 concerns explicitly re-checked): `StuckReasonSteerFailed`/
`StuckReasonRespawnBlockedActive` mutual exclusion for the single-call case (the remaining gap
under concurrency is also now resolved — see "Resolved Since This Review Round" above); the
missing Success-Metric-#2 test
(`TestAutoReopenForPRFix_ActiveWorkSession_ReSteersOnReasonChange_EvenWithinCooldown`, named in
Task 5.3.1d); and the missing success-path notification test
(`TestAutoReopenForPRFix_ActiveWorkSession_SuccessfulSteer_PublishesInfoNotificationAndResolvesRespawnBlockedActive`,
named in Task 5.3.1g). The `reasonSignature`/`PRStatus.render()` coupling concern is also
resolved via Task 2.1.1d's pinning test and doc-comment cross-reference.

## Minors

- **New**: the `onSteerAutonomousSession` prop type remains declared as
  `(sessionId: string, message: string) => void` in every intermediate layer the callback passes
  through (`SessionRow.tsx:66`, `SessionCard.tsx:170`, `SessionList.tsx:80,120`,
  `CockpitActionsContext.ts:24`) — Story 1.1.3's file list only touches `page.tsx` and
  `SessionActionsOverflow.tsx`. Verified this is harmless at *runtime*: every intermediate usage
  (`PaneHeader.tsx:75`'s `(id, msg) => cockpit.onSteerAutonomousSession(id, msg)`, and the plain
  pass-throughs in `SessionRow.tsx:571`, `SessionList.tsx:183,1418,1602`, `SessionCard.tsx:1039`,
  `PaneSplitRenderer.tsx:193`) is either an expression-bodied wrapper (implicit return, forwards
  the real `Promise<boolean>`) or a direct pass-through — none discards the return value. But the
  stale `void` typing could mislead a future maintainer into treating the callback as
  fire-and-forget. Worth updating the four type declarations for accuracy, not required for
  correctness.
- **Carried forward, still not done** (low priority — same as the prior review's assessment):
  the `steerConflictDebounce.Store` race between two overlapping reconcile ticks (Task 4.2.1b)
  still has no one-line comment noting it's an accepted, self-healing race rather than an
  oversight. (The `steerInFlight`-scope mutual-exclusion race that used to be in the same class
  is no longer applicable — it's resolved, see "Resolved Since This Review Round" above — so
  this is now the only remaining instance of this kind of race, not two candidates a single
  comment could cover.)
- **Resolved, verified**: the MCP `steer_session` glossary-scoping minor (the `steerInstance`
  Domain Glossary entry, plan.md:27, now explicitly notes the MCP tool "resolves the instance and
  calls `SendKeys`/`RunWithResume` directly... unaffected by anything in this plan") and the
  rabbit-holes cross-reference minor (Unresolved Questions' third bullet, plan.md:78, now cites
  `research/build-vs-buy.md` §3 by name).
