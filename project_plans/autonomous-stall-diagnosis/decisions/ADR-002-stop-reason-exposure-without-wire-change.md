# ADR-002: Expose the typed stop reason without changing the wire contract

**Status**: Accepted (plan phase)
**Date**: 2026-10-13
**Feature**: autonomous-stall-diagnosis
**Deciders**: planning phase (SDD Phase 3)

## Context

AC2 requires each autonomous stop to be recorded in an observable form. The existing
`autonomous_outcome` string (proto `types.proto` field 67) is compared strictly to `"done"` and
`"stuck"` in `web-app/src/components/sessions/SessionCard.tsx` (lines 877 and 888); no other
web-app file reads it (grep of `web-app/src` for `autonomousOutcome`/`autonomous_outcome`,
excluding `gen/`, 2026-10-13).

## Options considered

### A. Widen `autonomous_outcome` to `stuck:idle`, `stuck:error`, ...
- **Weakness**: silently breaks the two strict comparisons (badge disappears). Rejected.

### B. New proto field `autonomous_stop_reason`
- **Strength**: first-class UI field.
- **Weakness**: proto regeneration for Go, TS and mobile clients, plus UI work nobody asked for.
  Deferred; revisit only if the owner wants a badge variant.

### C. In-memory `AutonomousStopReason` on the instance snapshot + durable stuck-row context + log (CHOSEN)
- `autonomous_outcome` keeps the closed set `""|done|stuck`.
- The typed reason is on `AutonomousModeState` (in-memory, actor setter), in one structured log
  line per stop, and in the `autonomous_stuck` row's `context` (durable, already rendered as the
  "why" in `StuckItemDetail.tsx`).

## Decision

Option C; no proto or ent change, no new component, RPC or field. The only web-app change is one
CSS-only rule (`overflow-wrap: anywhere` on `StuckItemDetail`'s `.value`, plan Task 3.1.4d) so a long
unbroken tail token cannot overflow the card at 360 px.

- `cancelled_external` maps to outcome `""` (not `"stuck"`): an externally requested stop is not
  a stall, and it removes the false "Stuck" badge on paused/deleted/review-handoff sessions.
- `cancelled_shutdown` with `autonomous_driver_readoption` ON leaves outcome and `AutonomousMode`
  untouched (the run is a boot re-adoption candidate, ADR-001); it records only
  `AutonomousStopReason`. With the flag OFF it takes today's instance writes (mode cleared, outcome
  `stuck`) via `SetAutonomousStopped`, so the wire outcome set stays `""|done|stuck` either way.
- Nudge and re-supervise leave outcome `""` and mode true for the whole window; the instance never
  shows `stuck` for a stop that is being recovered in place.
- `AutonomousStopReason` is typed `domain.StopReason` end to end (driver outcome, setter,
  snapshot).
- The row `context` leads with a plain-language sentence composed from one typed copy table (what
  stopped, what the session was doing, what the system did, whether the operator must act), then
  ` | ` and the `key=value` detail (`stop_reason`, `state`, `action`, `nudge=n/cap`, ...). The UI
  renders the string verbatim, so the lead sentence is what makes the existing "Why:" line readable
  without a component change. Codes appear only after the separator and in the log line.
- The row also records the **chosen action** (`nudged|respawned|resupervised|none`, with the
  transient `nudging` between plan time and the end of a nudge). A successful nudge **annotates**
  the open row (`action=nudged nudge=n/cap`) and does not resolve it: the row holds the lifetime
  nudge counter the diagnose agent and the claim key on, and the existing DONE path closes it. This
  is a design choice with a cost: the item can still appear in the Unfinished tab after a
  successful nudge, with a lead sentence saying no action is needed.
- The pane tail is a *sink-bound* value: it goes to exactly one sink (the stuck row context),
  sanitized there; never to the log line or the push notification body. The tail is untrusted
  text (it can originate in a page the agent read): `StuckItemDetail.tsx` renders it as a React
  text node, and `DiagnosticBundleInput` (`session/diagnose_bundle.go:93-105`) has no stuck-row
  context field, so the diagnose bundle does not ingest it (verified 2026-10-13). If a future
  change adds the row context to the bundle, wrap it in the bundle's untrusted-data fence.

## Observability surface for AC2 (explicit)

| Stop | Where an operator can read the typed reason |
|------|---------------------------------------------|
| Backlog-linked stall (work role), Respawn/None/Nudge | `autonomous_stuck` row `context` (`stop_reason=... state=...`), returned as `StuckBacklogItem.context` (proto field 9) by the existing `ListStuckBacklogItems` RPC and rendered by `StuckItemDetail.tsx:102` in the Unfinished tab; plus the `[AutonomousDriver] stopped` log line |
| Resupervise, `cancelled_external`, `cancelled_shutdown`, non-backlog sessions | log line only (`stop_reason`, `state`, `action`) plus in-memory `Snapshot().Autonomous.AutonomousStopReason` (not on the wire) |

No new RPC or proto field. AC2 is verified through the stuck-row context (RPC) and the log line;
the in-memory field is a test seam, not an operator surface.

### Accepted: no operator cue for Resupervise or for cancels (UX G3)

| Stop | Why no cue is added | What bounds the invisibility |
|------|---------------------|------------------------------|
| Resupervise (a working pane re-attached) | A notification or row per re-attach is noise for a session that is working; adding a stuck row would also put a recovering session in the Unfinished tab, the opposite of the goal | At most 3 re-attaches, about 16 minutes in total; the chain ends in a later stop (typed row) or the cap-reached row that carries `last_stop_reason`; every stop logs `resupervise=n/3` |
| `cancelled_external` (operator stop, delete, toggle-off, review hand-off) | The operator caused it; the instance shows outcome `""` instead of a false "Stuck" badge | n/a |
| `cancelled_shutdown` | A graceful restart is not a stall; flag ON keeps the mode for re-adoption, flag OFF leaves only an in-memory badge in the exiting process (plan Risk Control) | Process lifetime |

If the soak shows operators asking "is it still being supervised?", the smallest cue is
`resupervise=n/3` in the session card's existing autonomous tooltip, which needs one proto field
(option B above); not planned.

## Consequences

- The reason is lost on process restart (like `AutonomousOutcome` today, which is not
  persisted: absent from `session/storage.go` `InstanceData` and the ent schema). The stuck row
  is the durable record.
- A future UI badge needs one additive proto field; nothing here blocks it.
