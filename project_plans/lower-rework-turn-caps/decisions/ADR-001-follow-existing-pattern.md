# ADR-001: Follow the `max_auto_rework_iterations` Pattern End-to-End

**Date**: 2026-10-07
**Status**: Accepted
**Deciders**: Tyler Stapler

---

## Context

`AutonomousMaxTurns` already exists as a first-class field in `config.Config` with a proper `OrDefault()` accessor and hard ceiling. It is not yet exposed through the Settings UI because it is absent from `SessionDefaultsConfig` (proto), `UpdateGlobalDefaultsRequest` (proto), and `GlobalDefaultsForm.tsx`. Three approaches were considered for wiring it in.

## Decision

Follow the `max_auto_rework_iterations` pattern exactly:
1. Append a new proto field to both messages (field 15 / field 13 — next available in each).
2. Add one line to `sessionDefaultsToProto` reading `AutonomousMaxTurnsOrDefault()`.
3. Add one unconditional assignment in the `UpdateGlobalDefaults` write block.
4. Add state / load / submit / render in `GlobalDefaultsForm.tsx` using identical structure to `maxAutoReworkIterations`.
5. Extend the existing test helpers in `defaults_service_test.go` and `GlobalDefaultsForm.test.tsx`.

## Alternatives Considered

### Alternative B: Generic config-field mapper in `UpdateGlobalDefaults`

Refactor the series of `cfg.X = int(req.Msg.X)` assignments into a table-driven mapper or reflection-based approach.

- **Strength**: eliminates boilerplate for future fields.
- **Weakness**: over-engineering. There are six fields today; the savings are negative when the abstraction's complexity is counted. Reviewers would face a more complex diff for a purely mechanical change. The existing pattern has zero open issues.

### Alternative C: Skip proto; expose via separate admin-only HTTP endpoint

Add a `/api/admin/autonomous-turns` endpoint outside the ConnectRPC stack.

- **Strength**: smaller proto surface area.
- **Weakness**: violates the project's established "all config through proto" convention; the UI can no longer use the generated TS client; any future protocol buffer upgrade or API gateway would miss this field.

## Consequences

- Wire-format is stable: field 15 and field 13 are appended after all existing fields; no existing clients break.
- The "0 means use the server default" convention (already established for `max_auto_rework_iterations`, `max_concurrent_backlog_work_items`, and `stale_session_threshold_minutes`) is extended to `autonomous_max_turns` consistently.
- `AutonomousMaxTurns` is **not** propagated to `sharedBacklogCfg` (unlike the concurrency/rework caps). This is intentional: the turn cap is consumed at driver-start time via `config.LoadConfig()`, so a stored value takes effect on the next item dispatch without any live propagation needed.
- The stale "(3)" comments in `session.proto` and `backlog_service.go` are corrected as part of this same edit pass, bundled because they are in the same files being touched.
