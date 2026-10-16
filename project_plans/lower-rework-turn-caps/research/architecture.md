# Architecture Research: lower-rework-turn-caps

## Proto field numbers

### `SessionDefaultsConfig` (proto/session/v1/session.proto:2188)

Fields 1–14 are assigned; **next available: 15**.

| # | Field |
|---|-------|
| 1 | program |
| 2 | auto_yes |
| 3 | tags |
| 4 | env_vars |
| 5 | cli_flags |
| 6 | profiles |
| 7 | directory_rules |
| 8 | one_off_base_dir |
| 9 | new_project_base_dir |
| 10 | max_auto_rework_iterations |
| 11 | max_concurrent_backlog_work_items |
| 12 | stale_session_threshold_minutes |
| 13 | stale_session_notify_enabled |
| 14 | retry_policy |
| **15** | **autonomous_max_turns (new)** |

### `UpdateGlobalDefaultsRequest` (proto/session/v1/session.proto:2273)

Fields 1–12 are assigned; **next available: 13**.

| # | Field |
|---|-------|
| 1 | program |
| 2 | auto_yes |
| 3 | tags |
| 4 | env_vars |
| 5 | cli_flags |
| 6 | one_off_base_dir |
| 7 | new_project_base_dir |
| 8 | max_auto_rework_iterations |
| 9 | max_concurrent_backlog_work_items |
| 10 | stale_session_threshold_minutes |
| 11 | stale_session_notify_enabled |
| 12 | retry_policy |
| **13** | **autonomous_max_turns (new)** |

## `sessionDefaultsToProto` helper

`buildSessionDefaultsConfig` does not exist. The actual helper is
`sessionDefaultsToProto(cfg *config.Config) *sessionv1.SessionDefaultsConfig`
at `server/services/defaults_service.go:536`. It is a standalone package-level
function (not a method), called in two places:
- `GetSessionDefaults` (line 108)
- `UpdateGlobalDefaults` response (line 200)

The new field slots in as a single line inside `sessionDefaultsToProto`:

```go
// #nosec G115 -- small local config knob, far below int32 range.
AutonomousMaxTurns: int32(cfg.AutonomousMaxTurnsOrDefault()),
```

`AutonomousMaxTurnsOrDefault()` already exists in `config/config.go:1084`
(default 30, hard ceiling 200, clamped to the range 1–200).

## `UpdateGlobalDefaults` write pattern

`UpdateGlobalDefaults` (defaults_service.go:138) uses a **load-modify-save** pattern:
1. Fresh `config.LoadConfig()` every call (no long-lived instance).
2. Mutate fields directly on the loaded struct.
3. `config.SaveConfig(cfg)` — atomic write.
4. Post-save: copy two backlog-concurrency fields onto `sharedBacklogCfg` under
   its mutex for live-effect without restart.

The new field follows exactly the same pattern as `max_auto_rework_iterations`:
```go
cfg.AutonomousMaxTurns = int(req.Msg.AutonomousMaxTurns)
```

## Does `AutonomousMaxTurns` need `sharedBacklogCfg` propagation?

**No.** The `sharedBacklogCfg` live-copy exists because `BacklogService` holds a
single `*config.Config` pointer loaded once at process start (server/dependencies.go).
`MaxAutoReworkIterations` and `MaxConcurrentBacklogWorkItems` are read from that
long-lived pointer, so `UpdateGlobalDefaults` copies them over to make a raise
take effect immediately.

`AutonomousMaxTurns` is read via a fresh `config.LoadConfig().AutonomousMaxTurnsOrDefault()`
at every driver start-up:
- `autonomous_orchestration_service.go:217` — `RunAutonomousMode`
- `autonomous_orchestration_service.go:235` — `ResumeAutonomousMode`
- `session_creation_pipeline.go:346` — `SpawnSessionFromItem`

Each of those calls loads config at the moment a new session starts, so a
post-save config value is visible to the very next session launch. No shared-instance
copy needed.

## Tech debt disposition: extend-as-is

`defaults_service.go` is clean and follows a consistent pattern throughout.
The load-modify-save flow is well-understood, and the `sessionDefaultsToProto`
helper is the single place to touch for response encoding. There is no
complexity or churn hotspot that warrants a refactor-first or isolate-via-seam
approach. **Extend-as-is.**

## Integration summary (all touchpoints)

| File | Change |
|------|--------|
| `proto/session/v1/session.proto` | Add `int32 autonomous_max_turns = 15` to `SessionDefaultsConfig`; add `int32 autonomous_max_turns = 13` to `UpdateGlobalDefaultsRequest`; fix stale comments (see requirements.md) |
| `server/services/defaults_service.go` | `sessionDefaultsToProto`: add `AutonomousMaxTurns: int32(cfg.AutonomousMaxTurnsOrDefault())`; `UpdateGlobalDefaults`: add `cfg.AutonomousMaxTurns = int(req.Msg.AutonomousMaxTurns)` |
| `web-app/src/components/settings/GlobalDefaultsForm.tsx` | Add `autonomousMaxTurns` state (init from `defaults.autonomousMaxTurns`), include in submit payload, render numeric input field following `maxAutoReworkIterations` pattern |

No config/config.go changes needed — `Config.AutonomousMaxTurns` field and
`AutonomousMaxTurnsOrDefault()` already exist.
