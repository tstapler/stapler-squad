# Architecture Review: session-revive-uuid-loss
**Date**: 2026-08-06
**Verdict**: CONCERNS

## Constitution Check

`docs/adr/ADR-000-architecture-constitution.md` does not exist in this repo
(confirmed: `find docs/adr -iname "*constitution*"` returns nothing). No
constitution constraints apply — skipping to the three-lens review.

## Blockers

- [ ] **Story 1.1.1 / Task 1.2.1c — new field documented as lock-protected, but written unlocked from a path that already races against a real concurrent reader.**
  Task 1.1.1b's plan text says to doc-comment `recoverySuppressed`,
  `everHadConversationHistory`, and `LastReviveOutcome` as "guarded by
  claudeSessionMu, same lock order as HistoryFilePath." But Task 1.2.1c then
  has `tryExtractConversationUUID()` set `i.everHadConversationHistory = true`
  inside its existing **unlocked** direct-mutation block
  (`session/instance_claude.go:356-362`), explicitly instructing "do not
  introduce a new lock here... match the existing unlocked-write style
  exactly." This isn't a theoretical race: `session/history_linker.go:232`
  (`correlateSession`, called from both `ScanAll()` and the 5s-tick
  `scanAllSessions()` poller — a goroutine outside the actor) already calls
  `inst.HasClaudeSession()`, which takes `claudeSessionMu.RLock()` on
  `i.claudeSession`, concurrently with any in-flight actor-goroutine cold
  restore's unlocked write inside `tryExtractConversationUUID()`. That race
  on `ConversationUUID`/`HistoryFilePath` is pre-existing debt the plan
  correctly declines to fix here (architecture.md's documented
  inconsistency) — but extending the *same* unsynchronized write to a new
  field, while telling the next reader in a doc comment that the field *is*
  claudeSessionMu-protected, is a plan-introduced defect: it invites a
  future caller to read `everHadConversationHistory` under `RLock()` alone
  and trust it, and it now feeds a persisted field, a proto wire field, and
  a user-visible notification — higher stakes than the field it's modeled
  on.
  **Remediation**: pick one, not both — (a) since Task 1.2.1c is already
  touching this exact block, move the write under the same `i.mu` nesting
  `ClearConversationState`/`SetHistoryInfo` already use (small, scoped, and
  it closes the race for the two fields this fix needs to be correct rather
  than merely matching pre-existing debt), or (b) if intentionally kept
  unlocked for consistency with the existing UUID/HistoryFilePath fields,
  correct Task 1.1.1b's doc comment to state the *actual* invariant ("no
  claudeSessionMu protection when set via tryExtractConversationUUID's
  direct-mutation path — same actor-serialization-only guarantee as
  ConversationUUID; concurrent readers via HasClaudeSession()/HistoryLinker
  may observe a stale value for one poll cycle") instead of asserting a
  guarantee the code doesn't provide.

## Concerns

- [ ] **Task 1.3.1a — `coldRestoreOutcome.Resume bool` is a derivable duplicate of `Outcome`, reintroducing the exact illegal-state risk the plan's own Pattern Decisions table argues against elsewhere.**
  `Resume` is true iff `Outcome ∈ {ResumeLive, ResumeRecovered}` — nothing
  else. The Pattern Decisions row "Durable field shape" explicitly rejects
  two independent booleans in favor of one enum specifically *because* "an
  enum makes the 4 mutually-exclusive states exhaustively switchable...
  instead of requiring callers to reason about invalid boolean
  combinations." `coldRestoreOutcome` reintroduces that same problem one
  level down: nothing stops a future edit from returning `{Resume: true,
  Outcome: ReviveOutcomeFreshExpected}`, and callers (Task 1.4.1b/1.5.1b)
  read both fields in different places. It's also functionally inert today —
  the actual `--resume` flag in the launch command is driven by
  `i.claudeSession.ConversationUUID` being populated (via
  `HasClaudeSession()` inside `initTmuxSession()`), not by `outcome.Resume`;
  the field exists only to pick a log line, at the cost of a field that can
  disagree with the enum it's redundant with.
  **Remediation**: drop the `Resume` field; add
  `func (o ReviveOutcome) ShouldResume() bool { return o == ReviveOutcomeResumeLive || o == ReviveOutcomeResumeRecovered }`
  and use `outcome.Outcome.ShouldResume()` at the two call sites. One method,
  zero new duplicated state.

- [ ] **Task 1.3.1b — `prepareColdRestore()`'s correctness depends on an unenforced, comment-only precondition.**
  Step 3 of the decision body calls `i.tryExtractConversationUUID()` wholesale
  and relies on the comment "the live-PID fast path inside it is a
  guaranteed no-op here since the caller already confirmed
  `!i.pm().IsAlive()`" — true today only because every call site happens to
  sit inside the `if !i.pm().IsAlive()` branch. Nothing inside
  `prepareColdRestore()` itself checks this; a future refactor that calls it
  from a new location (or reorders the surrounding branch) would silently
  start taking the live-PID branch with no compiler or runtime signal.
  Reusing `tryExtractConversationUUID()`'s *dual*-purpose body for a
  function that only ever wants its *path*-only half is exactly the kind of
  implicit temporal coupling this codebase's own locking-discipline comments
  (`ClearConversationState`, `SetHistoryInfo`) otherwise take pains to make
  explicit and self-documenting.
  **Remediation**: either add a defensive check/log at the top of
  `prepareColdRestore()` (`if i.pm().IsAlive() { log.Error("prepareColdRestore called with live pane"); ... }`),
  or extract the path-only scan into its own small helper that both
  `tryExtractConversationUUID()` and `prepareColdRestore()` call, instead of
  routing the new callers through the old function's full branch.

- [ ] **Task 2.3.1b — reuses `rateLimitLinkedItemID`, a feature-specific name, for an unrelated notification.**
  `onColdRestoreLostHistory` calls `s.rateLimitLinkedItemID(inst)` to look up
  the linked backlog item — a perfectly fine behavior reuse (best-effort,
  bounded, matches the pattern this repo wants), but the function's name
  encodes "rate limit," a concept this notification has nothing to do with.
  A future maintainer grepping for rate-limit code, or reading this
  notification's implementation cold, will be misled about what it depends
  on and why.
  **Remediation**: rename `rateLimitLinkedItemID` →
  `linkedItemIDForInstance` (or similar feature-neutral name) as part of
  this change, updating its two existing call sites
  (`onRateLimitDetected`/`onRateLimitRecovery`) alongside the new one. Purely
  mechanical, no behavior change.

- [ ] **Epic 1.1 — three new conversation-recovery fields are added to `Instance` and mutated from four separate methods, continuing a pattern of enforcing a shared invariant by convention rather than by type.**
  `recoverySuppressed`, `everHadConversationHistory`, and
  `LastReviveOutcome` join `ConversationUUID`/`HistoryFilePath` as fields
  whose correctness depends on `ClearConversationState`, `SetHistoryInfo`,
  `tryExtractConversationUUID`, and the new `prepareColdRestore` all
  remembering to touch the right subset in the right order (Pattern
  Decisions row 4 already names this exact risk for the two call sites this
  plan unifies — the same risk exists across these four *mutators*, not just
  the two call sites). Not a blocker for this scoped fix, since the plan's
  own tests (Story 4.1.1/4.1.3) directly cover the interactions that would
  break if a mutator forgot a field.
  **Recommendation** (fast-follow, not required for this PR): a small
  `ConversationRecoveryState` value type on `Instance` with
  `MarkCaptured()`/`MarkCleared()`/`ConsumeSuppression() bool` methods would
  let the "these fields change together" invariant be enforced once, in one
  place, instead of at every mutator call site.

## Nitpicks

- `startLocked` and `start()` remain two near-duplicate ~90-line functions
  requiring mirrored edits (Epic 1.4 / Epic 1.5). This plan is the second
  change to require careful hand-synced edits across both — the original bug
  this fix addresses was partly enabled by exactly this kind of drift
  (stack.md's noted comment divergence between the two blocks). The plan
  already identifies and accepts this cost explicitly in Step 0.5's
  alternative-(a) analysis; flagging only as a candidate follow-up (unify
  into one function + thin wrapper) rather than something to fix here.
- Pattern selection elsewhere in the plan is sound and worth calling out
  positively: reusing `LifecycleEvent`/`fireLifecycleEvent` + `onRateLimitRecovery`'s
  exact shape for the new notification (Observer pattern, no new mechanism
  invented), the proto-enum + `assertNever`-style frontend switch matching
  `DetectedStatus`, and the additive-only storage/proto migration are all
  consistent, low-risk reuses of existing precedent — no changes needed
  there.
- Build-vs-buy conclusion (`research/build-vs-buy.md`, "build, no new
  dependency") is consistent with the plan as written — confirmed no new
  library, SDK, or service is introduced anywhere in the task list.
