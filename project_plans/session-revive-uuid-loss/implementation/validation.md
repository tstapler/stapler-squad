# Validation: session-revive-uuid-loss

Scope: the reduced Phase 2 + Phase 3 work identified in `plan-addendum-2026-08-21.md`
(AC1/2/4/5/6 already shipped in #439; only AC3 — the durable, user-visible
fresh-with-lost-history signal — remains).

## Requirement → test coverage map

| AC | Requirement | Coverage | Status |
|----|-------------|----------|--------|
| AC1 | Recovery attempted before resume/fresh decision | `TestColdRestore_WithoutUUID_RecoversFromJSONL`, `TestKillSessionThenStart_DoesNotRebuildLaunchCommand` (shipped, #439) | Done — re-run as regression gate only |
| AC2 | No behavior/latency change when recovery finds nothing | Same tests, negative branch (shipped, #439) | Done — re-run as regression gate only |
| AC3 | Durable, user-visible signal on forced-fresh-with-prior-history | **New**: `TestColdRestore_SignalsFreshLostHistory_When_RecoveryFailsButEverHadHistory` (backend: `LastReviveOutcome` + notification fired); `TestOnColdRestoreLostHistory_PublishesNotification_UnlessHidden` (listener gating); `RevivedContextBadge.test.tsx` (frontend render) | **Outstanding — this item's actual deliverable** |
| AC4 | Symmetric fix at both cold-restore call sites | Shipped via shared `recoverConversationBeforeLaunch` (#439). New `LastReviveOutcome` assignment must be added at both `startLocked` (`instance.go:997-1004`) and `start` (mirror) — covered by AC3's new test running against both entry points, per `plan.md` Story 4.1.2's original intent (retargeted, not duplicated) | Partial — verify new field-set is symmetric too |
| AC5 | Existing tests continue passing; new tests added | `make test` full run pre- and post-change; new tests above | Outstanding for the new tests |
| AC6 | No regression to first-time-setup / explicit fresh-start | `TestColdRestore_NoSignal_When_GenuinelyNeverHadHistory`; existing `TestTryExtractConversationUUID_ClearedAtGuard` (shipped) | Outstanding for the new negative test |

## Pre-implementation checks

- [x] Confirmed `EverHadConversationHistory`/`LastReviveOutcome` do not already exist
  under any name (`grep -rn "ConversationLost\|ReviveOutcome\|EverHadConversationHistory\|ResumeFailed"` — no hits).
  Re-run this grep immediately before starting implementation in case another session
  shipped it in the interim (this backlog has a demonstrated history of parallel/
  duplicate work on this exact bug — 6 `project_plans/` directories touch it).
- [x] Confirmed the notification pipeline this design reuses (`pkg/events.NewNotificationEvent`,
  `onRateLimitRecovery`'s `Hidden`-gated pattern) exists and is wired via
  `wireCallbacks` — read `server/services/session_service.go:4001` pattern directly
  rather than trusting `plan.md`'s citation (not re-verified in this pass; **do this
  before implementation**, since `plan.md`'s other citations were correct but this one
  specific line number should be re-confirmed given the codebase has moved since
  2026-08-06).
- [ ] Confirm `proto/session/v1/types.proto` doesn't already have an enum this collides
  with (`make proto-gen` will fail loudly if so — low risk, cheap check).

## Out of scope (explicitly, per requirements.md non-goals)

- Reducing restart churn (`driverInactivityTimeout`) — separate item.
- Persistence-layer rewrite of `SetHistoryInfo`'s save callback.
- `RunWithResume`'s one-off `-p --resume` flow.
- Cross-session same-directory `DetectByPath` disambiguation (accepted limitation).
