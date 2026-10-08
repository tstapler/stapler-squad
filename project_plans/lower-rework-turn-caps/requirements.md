# Requirements: lower-rework-turn-caps

**Date**: 2026-10-07
**Type**: chore / configuration hygiene
**Complexity**: 2 — config constant changes + proto + backend wire-up + UI field

## Problem Statement

Backlog rework is the largest multiplier on quota spend. Two related gaps exist:

1. **`AutonomousMaxTurns` not configurable via UI.** The global turn cap per autonomous
   session (`config.AutonomousMaxTurns`, default 30, `config/config.go:1077`) cannot be
   viewed or changed through the Settings → Global Defaults form or its backing API
   (`GetSessionDefaults`/`UpdateGlobalDefaults`). It is not in `SessionDefaultsConfig`
   (proto) and not in `UpdateGlobalDefaultsRequest` (proto). Operators who want to lower
   the per-session turn budget must edit `~/.stapler-squad/config.json` directly.

2. **Stale proto comments.** `SessionDefaultsConfig.max_auto_rework_iterations` and
   `UpdateGlobalDefaultsRequest.max_auto_rework_iterations` both carry the comment
   "server default (3)" (`session.proto:2202, 2282`) while the Go constant is
   `DefaultMaxAutoReworkIterations = 5` (`config/config.go:1059`). The comment was not
   updated when the default was lowered, so documentation and code diverge.

**Note on prior description claims:** The backlog item's description stated the default
was 20 and the UI fell back to 3. Code investigation shows those numbers are outdated:
the rework cap default is already 5, the UI correctly loads the server-resolved value
(not a hardcoded 3), `autonomousMaxTurnsDefault` is already 30 (not 60), and the cold
retry loop is already bounded at `MaxRemediationColdRetries = 2` with `remediationColdRetryInterval = 7d`.
The remaining work is the two items above.

## Target User

**System operator** — the person responsible for configuring and maintaining a deployed stapler-squad instance. Typically a developer or DevOps engineer who manages the deployment for a small team. They understand what a "turn" means (one agent↔user exchange in an autonomous session) and want to tune the cap to balance cost against task completion without directly editing JSON.

## Success Metric

After this feature ships, an operator can view and change the autonomous session turn cap through the Settings → Global Defaults UI and immediately have that cap honored by all subsequently-started autonomous sessions — without editing `config.json` directly or restarting the service.

## Named Assumptions

1. **Operators find direct `config.json` editing painful enough to warrant a UI field.** This is plausible (it requires SSH access, knowing the file path, and careful JSON editing) but has not been validated with actual operators. If the real pain is not config editing but is instead "I don't know where to look," a contextual hint in the backlog panel would serve the need better than a settings field.

## Scope

### In Scope
- Add `autonomous_max_turns` field to `SessionDefaultsConfig` and
  `UpdateGlobalDefaultsRequest` in `proto/session/v1/session.proto`.
- Wire the new field through `defaults_service.go`:
  `GetSessionDefaults` → reads `cfg.AutonomousMaxTurnsOrDefault()`;
  `UpdateGlobalDefaults` → writes `cfg.AutonomousMaxTurns`.
- Add `autonomousMaxTurns` state + UI field + save wire-up to `GlobalDefaultsForm.tsx`.
- Fix the stale "server default (3)" comments in `session.proto`.
- Update `GlobalDefaultsForm.test.tsx` and `defaults_service_test.go` to cover the
  new field and the corrected comment value.

### Out of Scope
- Changing `DefaultMaxAutoReworkIterations`, `autonomousMaxTurnsDefault`, or
  `autonomousMaxTurnsHardCeiling` — those are already at evidence-based values.
- Changing the cold-retry schedule or `MaxRemediationColdRetries` — already bounded.
- UI for per-item `reworkCapOverride` (already exists in `StuckItemDetail`).
- Cost/token ceilings (separate project: `triage-cost-ceiling`).
- Account-wide quota gating (separate project: `quota-aware-backlog-gating`).

## Acceptance Criteria

1. `SessionDefaultsConfig` in `session.proto` has an `autonomous_max_turns` field; its
   comment states the resolved default value matches `autonomousMaxTurnsDefault` (30).
2. `UpdateGlobalDefaultsRequest` in `session.proto` has an `autonomous_max_turns` field
   with a matching hint comment.
3. `GetSessionDefaults` returns `AutonomousMaxTurnsOrDefault()` in the `autonomous_max_turns`
   field; never 0 in a response.
4. `UpdateGlobalDefaults` accepts and persists `autonomous_max_turns`; 0 in the request
   resets to the system default (same pattern as `max_auto_rework_iterations`).
5. The `GlobalDefaultsForm` shows a numeric input for "Max Autonomous Session Turns"
   populated from the loaded defaults, bounded to `[1, 200]` (matching the hard ceiling).
6. Saving the form sends `autonomous_max_turns` in the `UpdateGlobalDefaults` request.
7. The stale "server default (3)" comments in `session.proto` are updated to "(5)".
8. `GlobalDefaultsForm.test.tsx` has a test that the turn-cap field is rendered with
   the server-provided value.
9. `defaults_service_test.go` covers `autonomous_max_turns` in both `GetSessionDefaults`
   and `UpdateGlobalDefaults` paths.
10. `make build && make test` pass with no new failures.

## Constraints

- Follow the existing `max_auto_rework_iterations` pattern end-to-end (proto → service
  → form) — no new patterns needed.
- `AutonomousMaxTurnsOrDefault()` already clamps to `[1, 200]`; the UI must not allow
  values outside that range (use `min={1} max={200}`).
- Proto field numbers: assign the next available number in each message to avoid
  collisions; run `make proto-gen` after editing the proto.
- All generated `gen/` and `session/ent/` files are gitignored; do not commit them.

## Related
- `config/config.go:1054–1091` — `DefaultMaxAutoReworkIterations`, `AutonomousMaxTurnsOrDefault`
- `proto/session/v1/session.proto:2187–2295` — `SessionDefaultsConfig`, `UpdateGlobalDefaultsRequest`
- `server/services/defaults_service.go` — `GetSessionDefaults`, `UpdateGlobalDefaults`
- `web-app/src/components/settings/GlobalDefaultsForm.tsx` — settings form
- Companion projects: `project_plans/triage-cost-ceiling/`, `project_plans/quota-aware-backlog-gating/`
