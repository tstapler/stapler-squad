# Architecture Review: hook-classification-fast-path

**Date**: 2026-09-23
**Verdict**: CONCERNS

## Constitution Violations

None; no `docs/adr/ADR-000-architecture-constitution.md` was identified as a governing artifact during review.

## Blockers

None.

## Concerns

- [ ] Story 2.1.1 — Extracting policy from `ApprovalHandler` can accidentally broaden standalone PreToolUse semantics to include PermissionRequest-only secret/domain/live-session/queue behavior — remediation is now explicit in the story: preserve current `ssq-hooks check` semantics and pin the excluded behavior in equivalence tests.
- [ ] Story 2.1.2 — Versioned snapshot publication could become a second rule-composition owner and repeat the lost-update issue documented by dynamic-rule-reload research — keep composition serialized in `RulesService`; snapshot publishing must observe the completed composed snapshot only.
- [ ] Story 3.3.1 — Endpoint metadata crosses session creation, tmux environment, and hook injection layers — define one small immutable launch-metadata value rather than adding independent primitive strings to each API.
- [ ] Story 4.1.1 — A batch method directly on the broad `session.Repository` interface would violate interface segregation — the plan was repaired to define a consumer-owned `AnalyticsBatchSink` and satisfy it structurally.
- [ ] Story 5.1.2 — Dashboard source is in a separate dotfiles repository, so the primary PR cannot atomically land code and dashboard — treat it as a named companion commit and make code-side metrics validation independent of dashboard availability.
- [ ] Story 5.3.1 — A socket-only benchmark would miss the short-lived Go process startup that is part of the SLO — the plan was repaired to require real `ssq-hooks check` subprocess measurements.

## Nitpicks

- Keep `internal/hookipc` limited to transport/value objects; it should not import Ent or approval-queue types.
- Prefer typed constructors for `InstanceFingerprint`, `ProtocolVersion`, and `HookRequestID`; do not expose unchecked string casts.
- Keep cache checksum described as integrity/corruption detection, not authentication; same-user filesystem permissions are the trust boundary.
