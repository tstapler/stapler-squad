# ADR-001: WorkingState Enum vs. is_working Bool

**Date**: 2026-05-02
**Status**: Accepted
**Feature**: Review Queue Working-State Detection

## Context

The review queue needs to propagate whether a session is actively working to the frontend. Two viable options were identified in the architecture research:

- **Option A**: Add `bool is_working` to `Session` and `ReviewItem` proto messages
- **Option B**: Add `enum WorkingState` with values `UNSPECIFIED / ACTIVE / PROCESSING / IDLE / WAITING`

A third option (Option C) was also evaluated: derive working state from the existing `detected_status` string field on `SessionStatusChangedEvent`, requiring no new proto fields.

## Decision

Use **Option B** — a `WorkingState` enum.

## Rationale

`StatusActive` (Claude generating output, "esc to interrupt" visible) and `StatusProcessing` (tool use, "Thinking...", no interrupt UI) are meaningfully different from the frontend's perspective:

- `StatusActive` → show an interrupt affordance; session is most definitely busy
- `StatusProcessing` → show a tool-use/thinking indicator; session is busy but in a different phase

A `bool` loses this distinction and forces the frontend to treat both the same. The enum is backward-compatible: proto field default `0 = WORKING_STATE_UNSPECIFIED` is treated identically to the previous "no working-state info" behavior by clients that haven't been updated.

Option C was rejected because it would require the review queue service to subscribe to the session event stream as a second consumer, introducing coupling and additional complexity in the service layer.

## Consequences

- `make generate-proto` must be run and generated files (`session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts`) committed together with the proto changes
- The mapping function `mapIdleStateToWorkingState()` in `server/adapters/instance_adapter.go` is the single authoritative translation from Go `IdleState` to proto `WorkingState`
- Future addition of a new working sub-state (e.g., `WORKING_STATE_STUCK`) is a non-breaking proto addition

## Rejected Alternatives

| Option | Reason Rejected |
|---|---|
| `bool is_working` | Loses Active vs. Processing distinction needed for frontend UI |
| Derive from `detected_status` string | Adds coupling between session event stream and review queue; no schema enforcement |
