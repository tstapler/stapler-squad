# Requirements: Custom program env vars reach the spawned session process

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509

Supersedes nothing: the earlier full planning set for this item lives in
`project_plans/program-env-not-applied/` (research, plan, adversarial review,
validation, pre-mortem). This file restates the requirements for the
`program-env-injection` pipeline run and records the verified root cause.

## Problem

A custom program registered via `UpsertProgramConfig` with an `env` map
(e.g. `ANTHROPIC_BASE_URL`) persisted correctly but its env vars were absent from
the spawned session: `printenv` in the pane and `tmux show-environment` both
showed only ambient values.

## Root cause (verified against code and tests)

Before `cdfd4e5cf2`, `initTmuxSession` called `SetExtraEnv` with only
`STAPLER_SESSION_UUID`; no code path merged
`config.ResolveProgramConfig(...).EnvVars` into the tmux `-e` set. Today:

- `Instance.wireTmuxSession` (`session/instance_tmux.go`) calls `buildExtraEnv()`
  and `session.SetExtraEnv(...)` before the session is started, so the `-e`
  flags are in place at `tmux new-session` time.
- `buildExtraEnv()` -> `resolveExtraEnvVars()` reads `config.LoadConfig()`,
  the same loader the API server writes through, then resolves by
  `snap.Program` and overlays instance-level `EnvVars`.
- `claudeSettingsEnvOverrideArgs()` (#852) reuses `resolveExtraEnvVars()`, so
  the tmux env and the Claude `--settings` override cannot diverge.

## Acceptance criteria

1. A `SESSION_TYPE_NEW_WORKTREE` session created with a custom program shows the
   registered env var via `printenv` in the pane.
2. `tmux show-environment -t <session>` includes the var.
3. A committed regression test fails on the pre-fix commit and passes on HEAD.
4. No regression to `claudeSettingsEnvOverrideArgs()`: program env still wins
   over a global `~/.claude/settings.json` `env` block.
5. Root cause documented with the failure mechanism (above).

## Verification evidence (this run)

- `go test ./server/services -run TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession -count=1` : PASS
  (`server/services/session_service_create_test.go:673`; asserts `show-environment` and in-pane `printenv`).
- `go test ./session -run 'EnvOverride|ExtraEnv|SettingsEnv' -count=1` : ok
  (`session/instance_tmux_test.go:1202-1278`).

## Out of scope

The companion directory-collision bug; redesign of the Program Configurations UI/API.
