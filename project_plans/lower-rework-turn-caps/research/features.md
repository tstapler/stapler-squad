# Feature Landscape: lower-rework-turn-caps

**Date**: 2026-10-07

## End-to-end pattern: `max_auto_rework_iterations` as template

The full wire-up for an integer global default follows a consistent six-layer pattern. All citations are to the worktree.

### 1. Config layer (`config/config.go`)

`AutonomousMaxTurns int` (field at line 311) already exists with `omitempty` JSON tag. `AutonomousMaxTurnsOrDefault()` (line 1084) clamps to `[1, autonomousMaxTurnsHardCeiling]` (200) and returns `autonomousMaxTurnsDefault` (30) when unset. **No config-layer work is needed for this feature** — the field and both constants are already at evidence-based values per the requirements.

The `OrDefault()` pattern is consistent across all similar fields:
- `MaxAutoReworkIterationsOrDefault()`: returns `DefaultMaxAutoReworkIterations` (5) when ≤0 or nil receiver — no ceiling clamp because rework cap is bounded by practical (not cost) concerns.
- `MaxConcurrentBacklogWorkItemsOrDefault()`: clamps to `[1, maxConcurrentBacklogWorkItemsHardCeiling]` (10) — same shape as `AutonomousMaxTurnsOrDefault()`.

### 2. Proto layer (`proto/session/v1/session.proto`)

Both messages need a new field. Current last-used field numbers:
- `SessionDefaultsConfig` (line 2188): highest field number is 14 (`retry_policy`). Add `autonomous_max_turns = 15`.
- `UpdateGlobalDefaultsRequest` (line 2273): highest is 12 (`retry_policy`). Add `autonomous_max_turns = 13`.

Convention for request fields: "0 = use the server default (N)". The response message `SessionDefaultsConfig` always echoes the resolved value (never 0).

**Stale comment to fix**: Both `SessionDefaultsConfig.max_auto_rework_iterations` (line 2201) and `UpdateGlobalDefaultsRequest.max_auto_rework_iterations` (line 2282) say "server default (3)". The Go constant is `DefaultMaxAutoReworkIterations = 5` (`config/config.go:1059`). Both must read "(5)".

### 3. Service layer (`server/services/defaults_service.go`)

`GetSessionDefaults` (line 102): Calls `sessionDefaultsToProto(cfg)`. The converter (line 536) maps `cfg.MaxAutoReworkIterationsOrDefault()` → proto field. Add `AutonomousMaxTurns: int32(cfg.AutonomousMaxTurnsOrDefault())` alongside the others (line 549 area). The `#nosec G115` comment convention applies — all are small local config knobs.

`UpdateGlobalDefaults` (line 138): Assigns each request field to `cfg` then saves. Add `cfg.AutonomousMaxTurns = int(req.Msg.AutonomousMaxTurns)` at line 150 area, matching the unconditional assignment pattern of `MaxAutoReworkIterations` and `MaxConcurrentBacklogWorkItems`. Zero in the request stores 0 in config.json; `AutonomousMaxTurnsOrDefault()` resolves it back to 30 at read time.

**sharedBacklogCfg propagation is NOT needed for `AutonomousMaxTurns`**: The shared-config mechanism (lines 186-190) exists because `BacklogService` holds a long-lived `*config.Config` pointer that is never reloaded. `AutonomousMaxTurns` is consumed by `autonomous_orchestration_service.go` via `config.LoadConfig().AutonomousMaxTurnsOrDefault()` at driver start time (lines 217, 235) — a fresh load each time. Changing the global setting takes effect for the next driver started with no extra wiring.

### 4. React form (`web-app/src/components/settings/GlobalDefaultsForm.tsx`)

State initialization: `useState(0)` (the same pattern as `maxAutoReworkIterations`, described "set from the server-resolved default on load").

Load (line 62 area): `setAutonomousMaxTurns(defaults.autonomousMaxTurns)`.

Save (line 104 area): include `autonomousMaxTurns` in the `updateGlobalDefaults({...})` call.

UI field:
```tsx
<input
  id="global-autonomous-max-turns"
  type="number"
  min={1}
  max={200}
  className={input}
  value={autonomousMaxTurns}
  onChange={(e) =>
    setAutonomousMaxTurns(Math.min(200, Math.max(1, Number(e.target.value) || 1)))
  }
/>
```
The `max={200}` matches `autonomousMaxTurnsHardCeiling`; `AutonomousMaxTurnsOrDefault()` already enforces this on the server side.

### 5. Frontend tests (`GlobalDefaultsForm.test.tsx`)

`sampleDefaults` (line 32) needs `autonomousMaxTurns: 30`. Tests needed:
- Renders the input with the server-loaded value.
- Submits the loaded value unchanged.
- Submits an edited value.

The existing stale-threshold save-failure test (line 128) is a good model for the "preserves value on failed save" pattern if desired.

### 6. Service tests (`defaults_service_test.go`)

Two tests needed, following the established patterns at lines 210-219:
- `TestGetSessionDefaults_ServesRuntimeAutonomousMaxTurnsDefault` — pins that `GetSessionDefaults` returns `autonomousMaxTurnsDefault` (30), not 0.
- `TestUpdateGlobalDefaults_SetsAutonomousMaxTurns` — verifies round-trip: request field propagates to response and is persisted so a subsequent `GetSessionDefaults` returns the same value.

The zero-means-reset behavior (`TestUpdateGlobalDefaults_ZeroStaleSessionThreshold_UsesServerDefault` at line 234) is implicitly covered by the fact that `UpdateGlobalDefaults` stores 0 unconditionally and `OrDefault()` resolves it — a separate test for this is optional but mirrors the stale-threshold precedent.

## What happens when `autonomous_max_turns` = 0 in `UpdateGlobalDefaultsRequest`?

Zero stores `AutonomousMaxTurns = 0` in config.json (no-op write, matching every other field). `AutonomousMaxTurnsOrDefault()` returns `autonomousMaxTurnsDefault` (30) when the stored value is ≤0. The response always echoes the resolved value (30), never 0. This is the identical "0 = reset to server default" convention used by `max_auto_rework_iterations` and `max_concurrent_backlog_work_items`.

## Per-item `autonomous_max_turns` overrides

There is no `autonomous_max_turns` per-item override field in `backlog.proto`. The only per-item override today is `rework_cap_override` (optional int32, field 28 in `BacklogItemProto` / field 15 in `UpdateBacklogItemRequest`, lines 240/667). The global setting is the sole control for per-session turn budgets; no interaction design is needed.

## Does changing the global turn cap affect already-running sessions?

No. Both `StartAutonomousDriverForInstance` (line 217) and `StartAutonomousDriverWithTimeout` (line 235) call `config.LoadConfig().AutonomousMaxTurnsOrDefault()` at the moment the driver is constructed. A driver's `maxTurns` argument is fixed at construction and passed through to the `TurnCallback` (line 176). An in-flight run that has already started keeps its original cap for the remainder of its turns. The analogous precedent is `RetryPolicyConfig`'s proto comment: "Applies to newly-started retry episodes only — a session already mid-backoff keeps the policy it started with." The same should be documented in the `autonomous_max_turns` field comment.

## Edge cases to handle

| Scenario | Expected behaviour | Basis |
|---|---|---|
| Request sends 0 | Stores 0; response returns 30 | `OrDefault()` zero-check, matches all siblings |
| Request sends 201 (above ceiling) | `AutonomousMaxTurnsOrDefault()` clamps to 200; UI `max={200}` blocks it client-side | `config/config.go:1088` |
| Config file deleted between requests | `config.LoadConfig()` returns zero-value Config; `OrDefault()` returns 30 | nil-receiver guard in `OrDefault()` |
| Already-running driver | Cap unchanged until next driver is started | Fresh `LoadConfig()` at driver start |
| `sharedBacklogCfg` wiring | Not needed for this field | `LoadConfig()` used at driver start, not cached pointer |

## Proto field numbers (confirmed)

| Message | New field | Number |
|---|---|---|
| `SessionDefaultsConfig` | `autonomous_max_turns` | 15 (next after `retry_policy = 14`) |
| `UpdateGlobalDefaultsRequest` | `autonomous_max_turns` | 13 (next after `retry_policy = 12`) |
