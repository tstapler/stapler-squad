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
   - **4b (executed, evidence.md E9):** program env wins over a global
     `~/.claude/settings.json` `env` block. Verified with a real `claude` CLI against local
     listeners (no credentials needed).
5. Root cause documented with the failure mechanism (above), tied to the pre-fix source.

## Evidence status (update as tasks run)

| AC | Status | Evidence |
|----|--------|----------|
| 1, 2 | Observed once on HEAD | `go test ./server/services -run TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession -count=1 -v` printed `--- PASS` (2026-10-10). Pane text not yet logged; plan Task 1.1.1b adds it. One run, no SKIP gate, no `tmux -V` recorded: this row stays "Observed once" and is NOT promoted to VERIFIED until plan evidence E1 (and E5) land with the `RealTmuxGate` verdict and tmux version. |
| 3 | Partly | Pre-fix run on `cdfd4e5cf2^` (`4dbbe7b40`) failed on `tmux show-environment must carry the program's env` (coordinator-run, unconverted test, no verbatim record in repo). Overlay red/green and lint-clean not done: the landed test has 5 blocking lint findings (plan Task 1.1.0a). |
| 4a | Unit tests exist and pass | `go test ./session -run 'EnvOverride|ExtraEnv|SettingsEnv' -count=1`: ok. End-to-end Claude-program test not written yet (plan Task 1.1.3c). |
| 4b | Executed 2026-10-10 | Real `claude` 2.1.296 against local listeners: `--settings` env beats a global `settings.json` env block, which itself beats an ambient env var (evidence.md E9). Limits: ran the flag directly, no org-managed settings. |
| 5 | Documented | `git show cdfd4e5cf2^` block above. |

## Out of scope (recorded; tracked as plan F1-F5, filing decision is the owner's)

- F1 env values/secrets in INFO logs, persisted `launch_command`, API/UI.
- F2 frozen-vs-reloaded env on restart (restart policy).
- F3 reserved keys `STAPLER_SESSION_UUID` / `CLAUDECODE` overridable by program env.
- F4 remote execution targets get no `-e`.
- F5 tymux gRPC backend skipping `wireTmuxSession` (inferred, not traced).
- The companion directory-collision bug named in the item (no separate ID known).
- Redesign of the Program Configurations UI/API.

## Backlog criteria mapping

The backlog item's acceptance criteria are numbered 1-5 and map 1:1 to AC1-AC5 above
(backlog criterion 4 = AC4; `report_progress` `criteria_index` is 0-based, so AC4 =
`criteria_index=3`). `request_review` fails closed unless every criterion is `pass`
(`docs/reference/backlog-completion-gate-and-cleanup.md`). AC4 is reported `pass` on the strength
of AC4a (E6, E8) plus AC4b (E9, executed against a real `claude` CLI); the earlier session's
`pass` mark was made on AC4a unit tests only and is now backed by E9.

## Risky assumptions

1. Real tmux in CI behaves like local tmux 3.6a (CI pins 3.4); a skipped real-tmux test must
   never be counted as a pass (plan Task 1.1.0b gate).
2. The original report ("env silently ignored") was caused by the pre-`cdfd4e5cf2` code and
   not by a route this item leaves out of scope. Nothing records on which build or path the
   symptom was first observed, so the user-visible symptom may still reproduce on HEAD via:
   edit-after-create then reuse of a live session (`initTmuxSession` reuse guard), in-process
   restart with frozen `EnvVars` (F2), the tymux gRPC backend (F5, inferred), or remote
   execution targets (F4). The PR body MUST name these four as residual symptom routes so
   closing the item is a deliberate owner decision (plan Task 1.4.2a). Cheap check: ask the
   reporter/owner which build and creation path the original observation used.
3. The companion directory-collision bug named in the item is not covered here and has no
   known ID; the PR body MUST name it so it is filed or linked rather than dropped.
