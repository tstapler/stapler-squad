# ADR-002: A failed steer gets its own StuckReason, not a reused RESPAWN_BLOCKED_ACTIVE row

**Status**: Accepted
**Date**: 2026-08-26
**Context project**: pr-fix-steering

## Context

Decision #7 (see requirements/architecture research) requires that a failed
`SteerActiveSession` call register a stuck row so it's visible via
`BlockerChip` without opening the notification bell, and explicitly leaves
open whether to reuse `StuckReasonRespawnBlockedActive` (with distinguishing
free-text content) or add a new `StuckReason` constant — architecture
research leans toward *not* adding a new constant for the success case, but
is silent on the failure case.

Checked directly: `StuckReason` is a `Record<StuckReason, T>`-typed lookup on
the frontend
([`web-app/src/components/backlog-stuck/stuckReason.ts:15-81`](../../../web-app/src/components/backlog-stuck/stuckReason.ts#L15-L81)),
so `BlockerChip` renders a *fixed* label/icon/class purely keyed by the enum
value — the free-text `detail` string passed to `MarkStuck` is not what the
chip displays. `STUCK_REASON_LABELS[StuckReason.RESPAWN_BLOCKED_ACTIVE]` is
literally `"Auto-respawn skipped — session active"` with a yellow 🟡 icon
([`stuckReason.ts:30,53`](../../../web-app/src/components/backlog-stuck/stuckReason.ts#L30)).

Reusing that reason for a failed steer would render "skipped" for a case that
is not a skip — the reconciler actively attempted to steer the session and
the delivery itself failed (e.g. `SendKeys`/`SendCommandImmediate` errored).
requirements.md's success metric says this must "stand out" and "not
regress to silence" relative to the old skip-only behavior it replaces; a
label that says "skipped" undersells a strictly worse outcome (attempted and
failed) than the state it replaces.

## Decision

Add a new `StuckReason` constant, `steer_failed`, used only for the failed
`SteerActiveSession` case:

- `session/domain/backlog.go`: `StuckReasonSteerFailed StuckReason = "steer_failed"`, added to `AllStuckReasons` and the `IsValid()` switch.
- `proto/session/v1/backlog.proto`: `STUCK_REASON_STEER_FAILED = 19;` (next free enum value after `STUCK_REASON_BLOCKED_BY_DEPENDENCY = 18`).
- `server/services/backlog_service_stuck.go`: add the case to both `toProtoStuckReason` and `fromProtoStuckReason`.
- `web-app/src/components/backlog-stuck/stuckReason.ts` / `.css.ts`: add a label ("Steer attempt failed"), an error-styled chip (mirroring `chipPushFailed`'s error background/border), and an icon (⛔, matching the other hard-failure reasons `PUSH_FAILED`/`PR_PENDING_NO_PR`).

`StuckReasonRespawnBlockedActive` remains unchanged and is still used for the
existing degrade paths (`SessionSteerer` not wired, session not live, dedup
suppression) — those genuinely are "skipped," so the existing label is
accurate for them.

No new `StuckReason` is added for the *success* case, per architecture
research's existing lean: a successful steer resolves any open
`RESPAWN_BLOCKED_ACTIVE` row (`resolveRespawnBlockedActiveLogged`) and is not
itself a stuck condition.

## Consequences

- Touches five files across three layers (Go domain/proto/adapter, generated
  proto client, TypeScript UI) — more than a pure Go change, but each touch
  is mechanical (one switch case, one enum value, one map entry × 3) and the
  `Record<StuckReason, T>` typing means a missed touchpoint is a **TypeScript
  compile error**, not a silent runtime gap.
- `make proto-gen` must be run after the `.proto` edit; the regenerated
  `gen/` output is not committed (repo convention, see root `CLAUDE.md`).
- A reviewer diffing this change should expect a new enum value, not a
  repurposed one — this is called out here specifically so it isn't
  mistaken for scope creep during review.
- **Reconciling this against requirements.md's "Out of Scope: New UI for steer history"
  (verified directly against
  [`web-app/src/components/backlog/BlockerChip.tsx`](../../../web-app/src/components/backlog/BlockerChip.tsx)):**
  this change adds one new case to the existing `Record<StuckReason, T>` label/icon/class maps
  in `stuckReason.ts` that `BlockerChip` already reads from generically (it renders purely off
  `item.reason` — the same mechanism that already renders `StuckReasonRespawnBlockedActive`
  today) — it is not a new UI component, screen, or history/timeline view. "New UI for steer
  history" in requirements.md means the latter (a log/timeline surface showing past steer
  attempts over time); a new label/icon/CSS-class entry in an already-existing chip's lookup
  table is a different, much smaller thing, and is explicitly what's being done here.

  If a future implementer finds `BlockerChip.tsx` no longer matches this description (e.g. it's
  been refactored to require new component code per `StuckReason`, not just a new map entry),
  this reconciliation no longer holds and should be re-flagged as a real scope question rather
  than assumed still true.
