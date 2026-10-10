# Requirements: Custom program env vars reach the spawned session process

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509

The earlier planning set for this item is `project_plans/program-env-not-applied/`
(research, plan, adversarial review, validation, pre-mortem). This file restates the
requirements for the `program-env-injection` run; `implementation/plan.md` is the
authoritative plan and supersedes the earlier one where they differ.

## Who and why

Operators who register a custom program (e.g. `netflix-model-gateway`, command
`claude`, `env: {ANTHROPIC_BASE_URL: http://127.0.0.1:47000}`) to route one program
through a local proxy. If the env map is silently ignored, sessions talk to the ambient
endpoint instead of the proxy: wrong backend, wrong credentials, no error shown.
Success = the registered env is observable in the pane's process and in tmux's
session environment, and a test fails if that wiring regresses.

## Problem

A custom program registered via `UpsertProgramConfig` with an `env` map persisted
correctly, but its env vars were absent from the spawned session: `printenv` in the
pane and `tmux show-environment` both showed only ambient values.

## Root cause

Before `cdfd4e5cf2` (`buildExtraEnv`/`wireTmuxSession` introduced), `initTmuxSession`
called `SetExtraEnv` with only `STAPLER_SESSION_UUID` (`git show cdfd4e5cf2^:session/instance_tmux.go`
lines 578-580), and `config.ResolveProgramConfig` did not exist outside tests, so
nothing merged a program's `EnvVars` into the tmux `-e` set. `resolveExtraEnvVars` /
`claudeSettingsEnvOverrideArgs` came later in `5da2af7bf` (#852). Today
`wireTmuxSession` calls `buildExtraEnv()` + `SetExtraEnv` before the session starts and
`resolveExtraEnvVars()` reads `config.LoadConfig()`, the same loader the API writes via.

## Acceptance criteria

1. A `SESSION_TYPE_NEW_WORKTREE` session created with a custom program shows the
   registered env var via `printenv` in the pane (pane text, not only tmux's table).
2. `tmux show-environment -t <session>` includes the var.
3. A committed regression test fails on the pre-fix code and passes on HEAD, shown by
   an explicit red/green run, and the test is lint-clean under `make lint-custom`.
4. No regression to `claudeSettingsEnvOverrideArgs()` (#852):
   - **4a (executable):** the `--settings` override is delivered intact (hostile values
     included) through a real shell/tmux for a Claude program, and the existing
     #852 unit tests still pass.
   - **4b (not executable locally):** program env wins over a global
     `~/.claude/settings.json` `env` block. Documented upstream, UNVERIFIED here; closing it
     needs a credentialed real-`claude` probe (plan E9). AC4 as a whole is NOT satisfied
     until 4b is run or the owner explicitly accepts it as unverified.
5. Root cause documented with the failure mechanism (above), tied to the pre-fix source.

## Evidence status (update as tasks run)

| AC | Status | Evidence |
|----|--------|----------|
| 1, 2 | Observed once on HEAD | `go test ./server/services -run TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession -count=1 -v` printed `--- PASS` (2026-10-10). Pane text not yet logged; plan Task 1.1.1b adds it. |
| 3 | Partly | Pre-fix run on `cdfd4e5cf2^` (`4dbbe7b40`) failed on `tmux show-environment must carry the program's env` (coordinator-run, unconverted test, no verbatim record in repo). Overlay red/green and lint-clean not done: the landed test has 5 blocking lint findings (plan Task 1.1.0a). |
| 4a | Unit tests exist and pass | `go test ./session -run 'EnvOverride|ExtraEnv|SettingsEnv' -count=1`: ok. End-to-end Claude-program test not written yet (plan Task 1.1.3c). |
| 4b | Not run | See above. |
| 5 | Documented | `git show cdfd4e5cf2^` block above. |

## Out of scope (recorded; tracked as plan F1-F5, filing decision is the owner's)

- F1 env values/secrets in INFO logs, persisted `launch_command`, API/UI.
- F2 frozen-vs-reloaded env on restart (restart policy).
- F3 reserved keys `STAPLER_SESSION_UUID` / `CLAUDECODE` overridable by program env.
- F4 remote execution targets get no `-e`.
- F5 tymux gRPC backend skipping `wireTmuxSession` (inferred, not traced).
- The companion directory-collision bug named in the item (no separate ID known).
- Redesign of the Program Configurations UI/API.

## Risky assumption

Real tmux in CI behaves like local tmux 3.6a (CI pins 3.4); a skipped real-tmux test must
never be counted as a pass (plan Task 1.1.0b gate).
