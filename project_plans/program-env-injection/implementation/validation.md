# Validation Plan: program-env-injection

**Date**: 2026-10-10
**Complexity**: 2
**Inputs**: `implementation/plan.md` (Ready for implementation), `requirements.md` (AC1-AC5).
**Scope of this document**: test design only. Nothing here was executed; existence of cited tests and helpers was checked by `grep` at HEAD `56857ba09`.
**Status update (2026-10-10, supersedes any 'AC4b UNVERIFIED / NOT executed' wording below):** AC4b was EXECUTED against a real `claude` 2.1.296 with local listeners (evidence.md E9): a global `settings.json` `env` beats an ambient env var (the #852 shape), and `--settings` `env` beats the `settings.json` `env` block. Limits: the CLI flag was run directly (the tmux-launch delivery half is AC4a / E8), and no org-managed settings were present. Backlog criterion 4 therefore rests on AC4a (E6, E8) plus AC4b (E9).
**Triad iteration 2 update**: mirrors plan.md "Repair log (triad iteration 2)". HEAD pins in this document and the plan (`56857ba09`, `9ef8fbc68`) predate the current HEAD (`572df53f8`, docs-only commits in between); plan Task 1.1.0b re-confirms every cited line before the first edit. T5-T9 are tagged AC4a, not AC4. AC4 reaches the backlog gate as criterion 4 (`criteria_index=3`) only under the plan's Story 1.1.3 "How the item reaches review" rule (AC4a executed plus recorded owner acceptance of AC4b). Worktree model: one worktree per agent plus a coordinator integration worktree with a merge step (G9 below).
**Iteration 1 update**: mirrors plan.md "Repair log (validate iteration 1)": real-tmux runs go through the `RealTmuxGate` (plan Task 1.1.0b), AC4 is split into AC4a (executed) and AC4b (UNVERIFIED, not executed), and gap G1's test (T17, plan Task 1.3.2c) is accepted into the plan.

## Happy Path Scenario

Given a server at HEAD (where, before `cdfd4e5cf2`, only `STAPLER_SESSION_UUID` reached `tmux new-session`) and a custom program `netflix-model-gateway` registered through `UpsertProgramConfig` with `Env: {ANTHROPIC_BASE_URL: http://127.0.0.1:47000, SSQ_PROGRAM_ENV_PROBE: probe-7f3a91}`, when the user creates a `SESSION_TYPE_NEW_WORKTREE` session with that program, then `tmux show-environment -t <session>` lists both keys and `printenv SSQ_PROGRAM_ENV_PROBE` typed into the pane prints `probe-7f3a91`.

That scenario is `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (T1 below). Every other test is a variation of one layer of it: resolver (T2), argv (T3, T4), `--settings` (T5-T9).

## Verification of cited tests and helpers

Checked with `grep` against the worktree. "Exists" = found at the cited location; "To create" = file or symbol absent today and created by the named task.

| Symbol | Location | Status |
|---|---|---|
| `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` | `server/services/session_service_create_test.go:673` | Exists (edited by 1.1.0a, 1.1.1b, 1.1.1c). Current assertion messages `tmux show-environment must carry the program's env` (:731) and `printenv inside the pane must show the program's env` (:746) match the plan's AC3 text |
| `TestCreateSession_should_NotLogEnvVarValues_When_EnvVarsProvided` | `server/services/session_service_create_test.go:884` | Exists |
| `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` | `session/instance_tmux_test.go:1202` | Exists |
| `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars` / `_CarriesResolvedEnvVars` | `session/instance_tmux_test.go:1232` / `:1248` | Exists |
| `TestBuildClaudeCommand_IncludesSettingsEnvOverride` | `session/instance_tmux_test.go:1278` | Exists |
| `TestStart_OwnerMatch_ReusesWithoutKilling` | `session/tmux/tmux_ownership_test.go:104` | Exists |
| `TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed` | `session/instance_tmux_test.go:136` | Exists |
| `TestToInstanceData_PreservesBackend` (model for T12) | `session/instance_serialization_test.go:15` | Exists |
| `TestExpandEnvVars_OmitsKey_WhenVarNotSetInEnvironment` (STK-3 cover) | `config/defaults_test.go:395` | Exists |
| Helpers: `seedCustomProgram` (:1192), `newCreateTestService` (:753), `destroyCreatedSession` (:785), `createTestStorage`, `initGitRepoWithCommit`, `envtest.NewIsolatedStateDir`, `wait.RequireEventually`, `wait.ScaleTimeout`, `safeexec.CommandContext`, `MockCmdExec`, `ownerMismatchFixture` (:27), `createMockExecutorForMissingSession` (`session_recovery_test.go:349`), `NewMockPtyFactory`, `newTmuxSessionWithSocket`, `NewTmuxSessionWithDeps`, `WithRegistry`, `TmuxPrefix`, `TmuxSession.newSessionArgs` (`tmux_session_start.go:637`), `Instance.resolveExtraEnvVars` (:710) / `buildExtraEnv` (:729) / `claudeSettingsEnvOverrideArgs` (:756) / `wireTmuxSession` (:771) | various | Exist. Note `safeexec` is not yet imported in `session_service_create_test.go` (Task 1.1.0a adds it); `session/instance_tmux_test.go` already imports it |
| `TestNewSessionArgs_EmitsExtraEnvPairs`, `TestNewSessionArgv_BothCreationPaths_CarryExtraEnv`, `TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell`, `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess`, `TestResolveExtraEnvVars_*` (3, incl. T17), `TestInstanceData_RoundTripDropsEnvVars`, `useAvailablePrograms_should_UseProgramIdNotCommandAsValue_When_IdAndCommandDiffer` | n/a | **To create** (files `session/tmux/new_session_args_extra_env_test.go`, `session/tmux/new_session_argv_creation_paths_test.go`, `session/instance_program_env_semantics_test.go`, `server/services/session_service_create_settings_env_test.go` do not exist; the hostile-values test goes into existing `session/instance_tmux_test.go`, the Jest `it` into existing `web-app/src/lib/hooks/useAvailablePrograms.test.ts`, which today has two `it` cases) |
| Historical shape for AC5 | `git show cdfd4e5cf2^:session/instance_tmux.go` | Confirmed: line 578 `if i.UUID != "" {`, line 579 `session.SetExtraEnv([]string{"STAPLER_SESSION_UUID=" + i.UUID})`; no `resolveExtraEnvVars`/`buildExtraEnv` symbol (`grep` found only the one `SetExtraEnv`). `git rev-parse --short cdfd4e5cf2^` = `4dbbe7b40` |
| Feasibility precedent for T4 | `tmux_ownership_test.go` `TestStart_OwnerMismatch_*` and `TestEnsureSessionExistsLocked_OwnerMismatch_*` | Both already drive `Start` and `RestoreWithWorkDir` through `MockCmdExec` plus `NewMockPtyFactory`, so the T4 approach has a working precedent |

## Test Inventory

| ID | Test | File | Type | Status | Tied to |
|---|---|---|---|---|---|
| T1 | `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` | `server/services/session_service_create_test.go:673` | Integration (real tmux) | Exists, modified | AC1, AC2, AC3 |
| T2 | `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` | `session/instance_tmux_test.go:1202` | Unit | Exists | AC1, AC3 (PIT-5 control) |
| T3 | `TestNewSessionArgs_EmitsExtraEnvPairs` (6 table cases; the `;`-suffixed value is characterization only: argv unescaped, tmux `new-session` rc=1, fix is F3) | `session/tmux/new_session_args_extra_env_test.go` | Unit | To create (1.2.1a) | AC2 support |
| T4 | `TestNewSessionArgv_BothCreationPaths_CarryExtraEnv` (2 subtests) | `session/tmux/new_session_argv_creation_paths_test.go` | Unit (`MockCmdExec`) | To create (1.2.2a) | AC2 support |
| T5 | `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars` | `session/instance_tmux_test.go:1248` | Unit | Exists | AC4a (not AC4b) |
| T6 | `TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars` | `session/instance_tmux_test.go:1232` | Unit (error/empty path) | Exists | AC4a |
| T7 | `TestBuildClaudeCommand_IncludesSettingsEnvOverride` | `session/instance_tmux_test.go:1278` | Unit | Exists | AC4a |
| T8 | `TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell` (9 values) | `session/instance_tmux_test.go` | Unit (real `sh -c`) | To create (1.3.1a) | AC4a |
| T9 | `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess` | `server/services/session_service_create_settings_env_test.go` | Integration (real tmux + FakeClaude) | To create (1.1.3c) | AC4a |

AC4b is covered by the executed listener probe E9 (not a Go test): `claude` 2.1.296 shows `--settings` env beats a `settings.json` env block. T5-T9 cover AC4a only.
| T10 | `TestResolveExtraEnvVars_InstanceEnvVarsCopyShadowsLaterProgramEdit` | `session/instance_program_env_semantics_test.go` | Unit (characterization) | To create (1.3.2a) | AC-less (PIT-B) |
| T11 | `TestResolveExtraEnvVars_InstanceWithoutEnvVarsSeesCurrentProgramEnv` | same | Unit (characterization) | To create (1.3.2a) | AC-less (PIT-B) |
| T12 | `TestInstanceData_RoundTripDropsEnvVars` | same | Unit (characterization) | To create (1.3.2b) | AC-less (PIT-B) |
| T13 | `useAvailablePrograms_should_UseProgramIdNotCommandAsValue_When_IdAndCommandDiffer` | `web-app/src/lib/hooks/useAvailablePrograms.test.ts` | Unit (Jest) | To create (1.3.3a) | AC-less (PIT-2b client half) |
| T14 | `TestCreateSession_should_NotLogEnvVarValues_When_EnvVarsProvided` | `server/services/session_service_create_test.go:884` | Integration | Exists; in final gate only | AC-less guard (F1 context) |
| T15 | `TestStart_OwnerMatch_ReusesWithoutKilling` | `session/tmux/tmux_ownership_test.go:104` | Unit | Exists; pin | AC-less (PIT-2e) |
| T16 | `TestInitTmuxSession_ReuseRequiresAliveNotJustConstructed` | `session/instance_tmux_test.go:136` | Unit | Exists; pin | AC-less (PIT-2e) |
| T17 | `TestResolveExtraEnvVars_should_ReturnOnlyInstanceEnv_When_ProgramNotRegistered` (2 subtests) | `session/instance_program_env_semantics_test.go` | Unit (error path) | To create (1.3.2c); accepted G1 | AC1 error path |

Non-test checks (evidence runs, not `go test` functions):

| ID | Check | Task | Evidence |
|---|---|---|---|
| M1 | Mutation: T1 run under `PreFixOverlay`, must FAIL on `tmux show-environment must carry the program's env` | 1.1.2a-b | E3, E4 |
| M2 | Same T1 command without `-overlay`, must PASS; `git status --short` empty | 1.1.2c | E5 |
| M3 | Mutation: T9 under overlay with `claudeSettingsEnvOverrideArgs` returning `"", ""`, must FAIL at the `--settings` assertion; then PASS without | 1.1.3d | E8 |
| M4 | Parent-commit run (`4dbbe7b40` + unconverted HEAD test): coordinator-reported, NO verbatim record in the repo; label as such, not counted toward AC3 unless re-run and gated | 1.1.2b | E4b |
| G0 | `RealTmuxGate`: preflight (`command -v tmux`, `test -x`, `tmux -V`, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`) + `-v` + `gate.sh` (require `--- PASS: <name>`, reject `--- SKIP`) on every real-tmux run (T1, T9, M1-M3, F1) | 1.1.0b | E1, E2, E4, E5, E8 |
| M5 | PIT-5 control: T2 PASSES under `PreFixOverlay` (resolver unit test cannot see the wiring bug) | 1.1.2b | E4 |
| L1 | Custom analyzers clean on touched packages: `norawexec`, `notimesleeptest`, `tmuxsocketscope` (new tests derive tmux argv via `tmux.ResolveSocket(...).Args`), `novartestseam`, `silenttransition` (the last two n/a, checked) | 1.1.0a, every test task | final gate |
| F1 | Flake evidence: T1 `-count=5 -race` | 1.1.1d | E2 |
| D1 | Upstream precedence quote (or literal `UNVERIFIED`) | 1.1.3b | E7 |
| D2 | AC5 historical diff via `git show cdfd4e5cf2^:...` | 1.1.2a | E3 |
| D3 | tmux duplicate `-e` precedence probe (later wins) | 1.4.1a | Task 1.4.1a section |

## Requirement -> Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| AC1: NEW_WORKTREE session with custom program shows env var via `printenv` in pane | `session/instance_tmux_test.go` | T2 `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars` | Unit | Happy path: program env and instance env both appear as `KEY=VALUE`; `STAPLER_SESSION_UUID` present |
| AC1 | `session/tmux/new_session_args_extra_env_test.go` | T3 `TestNewSessionArgs_EmitsExtraEnvPairs` | Unit | Error/edge path: "none" case emits no stray `-e`; hostile value stays one argv element |
| AC1 | `session/instance_program_env_semantics_test.go` | T17 `TestResolveExtraEnvVars_should_ReturnOnlyInstanceEnv_When_ProgramNotRegistered` | Unit | Error path: `Program` ID not in config, no panic, no phantom program env. Added to the plan as Task 1.3.2c (Gap G1 accepted) |
| AC1 | `server/services/session_service_create_test.go` | T1 `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` | Integration | Real `CreateSession` -> real tmux -> `send-keys` `printenv SSQ_PROGRAM_ENV_PROBE` -> captured `ENVPROBE_probe-7f3a91_END` |
| AC2: `tmux show-environment -t <session>` includes the var | `session/tmux/new_session_argv_creation_paths_test.go` | T4 `TestNewSessionArgv_BothCreationPaths_CarryExtraEnv` | Unit | Happy path: `Start` argv carries adjacent `-e ANTHROPIC_BASE_URL=...` and `-e STAPLER_SESSION_UUID=...` after `-e CLAUDECODE=` |
| AC2 | same | T4 subtest `RestoreWithWorkDir_MissingSession` | Unit | Variant path: recreate-path argv carries the same wire-time pairs (pins PIT-C frozen env) |
| AC2 | `session/tmux/tmux_ownership_test.go`, `session/instance_tmux_test.go` | T15, T16 (existing pins) | Unit | Boundary: a live, owner-matched same-name session is reused without `new-session`, so edited program env is not re-applied (intentional) |
| AC2 | `server/services/session_service_create_test.go` | T1 | Integration | `tmux -L <socket> show-environment -t <name>` output contains `ANTHROPIC_BASE_URL=http://127.0.0.1:47000` and `SSQ_PROGRAM_ENV_PROBE=probe-7f3a91` |
| AC3: committed regression test fails on pre-fix commit, passes on HEAD | `server/services/session_service_create_test.go` | T1 under `PreFixOverlay` (M1) | Mutation / Integration | Happy path: red on pre-fix `wireTmuxSession`, assertion message named |
| AC3 | same | T1 without overlay (M2) | Integration | Green on HEAD, clean tree |
| AC3 | `session/instance_tmux_test.go` | T2 under overlay (M5) | Unit | Control: resolver test stays green under the overlay, proving the integration test is the one that detects the wiring regression |
| AC3 | n/a | M4 parent-commit run (`4dbbe7b40`) | Mutation | Literal reading of "fails on the pre-fix commit"; coordinator-executed, pasted not re-run |
| AC3 | `tools/lint/{norawexec,notimesleeptest}` | L1 `bin/linter ./server/services ./session ./session/tmux` | Static | The committed test must survive CI's repo-wide custom lint (5 findings today, 0 after 1.1.0a) |
| **AC4a**: `--settings` flag delivery + hostile-value integrity + no regression of existing #852 unit tests (executed) | `session/instance_tmux_test.go` | T5 `..._CarriesResolvedEnvVars` | Unit | Happy path: flag is `--settings`, value is `{"env":{...}}` of resolved env |
| AC4a | same | T6 `..._EmptyWhenNoEnvVars` | Unit | Error/empty path: plain launch gets no `--settings` |
| AC4a | same | T7 `TestBuildClaudeCommand_IncludesSettingsEnvOverride` | Unit | Override is spliced into the built Claude command |
| AC4a | same | T8 `..._HostileValuesRoundTripThroughShell` | Unit | Error path: `'`, `$(...)`, backticks, `"`, `=`, space, newline, backslash, `${HOME}` survive `sh -c` byte for byte, `pwned` never created |
| AC4a | `server/services/session_service_create_settings_env_test.go` | T9 `TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess` | Integration | FakeClaude records argv through real tmux; `--settings` followed by exact env JSON; `show-environment` carries both keys |
| AC4a | same | T9 under overlay (M3) | Mutation | Test is red when the override is disabled |
| **AC4b**: program env outranks a global `~/.claude/settings.json` `env` block | n/a | D1 doc quote + **E9 executed** | Probe (real `claude`) | **VERIFIED 2026-10-10 (evidence.md E9, scenarios A-D).** Limits: CLI flag run directly; no org-managed settings. No Go test exists for it. |
| AC5: root cause documented with failure mechanism | `requirements.md`, doc comment above T1 | D2 `git show cdfd4e5cf2^:session/instance_tmux.go` | Documentation | Historical block inspected (line 578-579 verified in this run); no executable test exists or is claimed. Wording per plan Story 1.1.2: `config.ResolveProgramConfig` did not exist non-test at `cdfd4e5cf2^` (resolution + env merge arrived together), so the parent-commit run (M4) fails for two reasons and `PreFixOverlay` is an env-only reconstruction |

## UX Acceptance Tests

Not applicable. `project_plans/program-env-injection/design/ux.md` does not exist and the item changes no user-facing surface (test-only and evidence-only). The one client-side test, T13, is a hook mapping unit test, not a UX scenario.

## Migration Test

N/A. Plan "Migration Plan" section: no schema or data changes, so `migration_should_be_reversible` does not apply. T12 characterizes that `EnvVars` is deliberately not persisted; it is not a migration test.

## AC-less Hardening Coverage

| Plan item | Test | What it pins | Not covered (follow-up) |
|---|---|---|---|
| Epic 1.2 argv layer (PIT-5, PIT-C, PIT-D(ii)-(iii), STK-2) | T3, T4 | `-e CLAUDECODE=` first, `ExtraEnv` before `extraEnv`, hostile value is one element, both creation paths | Real tmux for these cases (T1 covers real tmux once); remote `-e` (F4) |
| Story 1.3.1 quoting (PIT-E, BVB-2) | T8 | Round trip through a real shell for 9 values | Large-payload command-length budget (PIT-E-len, deferred) |
| Story 1.3.2 frozen vs reloaded (PIT-B) | T10, T11, T12 | Create-time `EnvVars` copy shadows a later program edit; a bare instance sees the edit; `EnvVars` lost across `ToInstanceData`/`FromInstanceData` | Restart policy itself (F2); these tests are expected to flip when F2 lands |
| Story 1.3.3 client program ID (PIT-2b client half, ARCH-8) | T13 | Hook maps `value: p.id`, `command: p.command` when they differ | Omnibar `dispatch.ts` not re-audited (plan states so) |
| Story 1.4.1 (PIT-D(i), F3 mechanism) | D3 | Later `-e K=second` wins over `K=first` on tmux 3.6a | tmux 3.4 (CI pin) not re-run; stripping reserved keys (F3) |
| Task 1.1.0a lint repair | L1 | Landed test passes `norawexec`, `notimesleeptest` | none |
| Task 1.1.1b probe key | T1 | Unique `SSQ_PROGRAM_ENV_PROBE` defends against rc-file masking (SPECULATIVE per plan) | none |
| Task 1.1.1d flake evidence | F1 | Five consecutive passes under `-race` | Behaviour under heavy runner load beyond `wait.ScaleTimeout` |

Deliberately untested (plan Production-Behaviour Decisions, held out of this item): F1 secret redaction in INFO logs and persisted `LaunchCommand` (T14 exists but, per the plan's PIT-A finding, exits early; whether it covers the `--settings` launch command was not checked in this run), F2 resolve-once and restart policy, F3 reserved-key stripping, F4 remote `-e`, F5 tymux backend env.

## Test Stack

- **Unit (Go)**: stdlib `testing` + `testify` (`require`/`assert`); `MockCmdExec` + `NewMockPtyFactory` for argv capture (`session/tmux`); `envtest.NewIsolatedStateDir` / `t.Setenv("STAPLER_SQUAD_TEST_DIR", ...)` for config isolation; `safeexec.CommandContext` for subprocesses; `wait.RequireEventually` instead of `time.Sleep` (ADR-003).
- **Integration (Go)**: real `SessionService.CreateSession` + real tmux via `tmux.Binary()` on a private socket (`svc.testTmuxServerSocket`); FakeClaude shell script as the program for T9. Skips when no tmux binary resolves; fails (not skips) when tmux exists but the session never reaches `Active`.
- **Unit (web-app)**: Jest + `renderHook` for T13; `pnpm` only.
- **Mutation evidence**: `go test -overlay <json>`; the tracked tree is never modified.
- **Static**: `tools/lint` custom analyzers (`bin/linter`), `golangci-lint` via `make lint`, `jscpd` for the new Jest `it`.
- **E2E / UX**: none (no browser surface changed).
- **Environment**: local tmux is 3.6a (`tmux -V`, run on this machine); CI pins 3.4 (`.github/workflows/build.yml:275,327` per plan). `bin/` does not exist yet in this worktree, so `bin/linter` must be built first.

## Exact Commands

Run from the worktree root. CI form is `-race -short` with `TMUX_BIN="$(pwd)/bin/tmux"` where a pinned tmux build exists. **Every real-tmux run (T1, T9, M1-M3, F1, final gate) goes through the `RealTmuxGate`**: a skipped test exits 0 and prints `ok`, so exit code alone is never evidence.

```bash
# Preflight (plan Task 1.1.0b): resolve to an EXISTING binary; never point at a nonexistent $(pwd)/bin/tmux
export TMUX_BIN="${TMUX_BIN:-$(command -v tmux)}"
test -x "$TMUX_BIN" || { echo "PREFLIGHT FAIL: no tmux"; exit 1; }
"$TMUX_BIN" -V                                   # 3.6a locally; CI pins 3.4
export STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30   # as Makefile:601,682

# Generated code (fresh worktree, plan Task 1.1.0b; gitignored, never committed)
make proto-gen                                   # Makefile:547 (buf generate proto)
make ent-gen                                     # Makefile:564 (ent generate --feature sql/upsert)
test -d server/web/dist || mkdir -p server/web/dist   # embed stub; or `make web-build`

# Gate: $SCRATCH/gate.sh <pass|fail> <TestName> <wantCount> <outfile>  (scratch only)
#   awk: counts '--- PASS: <name> (' (or '--- FAIL:') lines; any '--- SKIP' -> exit 1; count != want -> exit 1
# Run shape:
#   go test ... -v >"$SCRATCH/out.txt" 2>&1; rc=$?; tail -n 60 "$SCRATCH/out.txt"
#   [ $rc -eq 0 ] && "$SCRATCH/gate.sh" pass <Name> <N> "$SCRATCH/out.txt"
#   red runs: "$SCRATCH/gate.sh" fail <Name> 1 "$SCRATCH/out.txt" && grep -F '<assertion text>' "$SCRATCH/out.txt"

# Build the custom linter once (Makefile:814-821 recipe, scoped)
go -C tools/lint build -o "$(pwd)/bin/linter" ./cmd/linter && bin/linter ./server/services ./session ./session/tmux

# AC1, AC2, AC3 (T1) and baseline (E1)
go test ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v   # gate: pass T1 1
go test ./session -run 'EnvOverride|ExtraEnv|SettingsEnv' -count=1 -v

# Flake evidence (F1 / E2)
go test -race -short ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=5 -v   # gate: pass T1 5 (five SKIPs fail the gate)

# AC3 red under PreFixOverlay (M1), control (M5), green on HEAD (M2)
go test -overlay "$SCRATCH/overlay.json" ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 -v   # gate: fail T1 1 + grep assertion text; build failure/SKIP does not count
go test -overlay "$SCRATCH/overlay.json" ./session -run '^TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars$' -count=1
go test ./server/services -run '^TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession$' -count=1 && git status --short

# AC4 (T5-T9, M3)
go test ./session -run 'TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars|TestClaudeSettingsEnvOverrideArgs_EmptyWhenNoEnvVars|TestBuildClaudeCommand_IncludesSettingsEnvOverride|TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars' -count=1 -v
go test -race -short ./session -run '^TestClaudeSettingsEnvOverrideArgs_HostileValuesRoundTripThroughShell$' -count=1 -v
go test -race -short ./server/services -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$' -count=1 -v   # gate: pass T9 1
go test -overlay "$SCRATCH/overlay-settings.json" ./server/services -run '^TestCreateSession_CustomClaudeProgram_SettingsEnvReachesLaunchedProcess$' -count=1 -v   # gate: fail T9 1 + grep '--settings' assertion
# AC4b: EXECUTED (evidence.md E9). Probe: three local HTTP listeners; scratch CLAUDE_CONFIG_DIR settings.json env.ANTHROPIC_BASE_URL=<GLOBAL>;
#   env -i ... claude -p hi [--settings '{"env":{"ANTHROPIC_BASE_URL":"<PROGRAM>"}}']   # the listener that receives the request is the URL used
#   result: settings.json beats ambient env; --settings beats settings.json
#   claude -p --settings '{"env":{"SSQ_PRECEDENCE_PROBE":"cli"}}' 'print $SSQ_PRECEDENCE_PROBE via your Bash tool'   # "cli" proves, "global" disproves

# AC5 (D2)
git show cdfd4e5cf2^:session/instance_tmux.go | grep -n 'SetExtraEnv' -B2 -A1

# Argv layer (T3, T4)
go test -race -short ./session/tmux -run 'TestNewSessionArgs_|TestNewSessionArgv_' -count=1 -v

# Characterization (T10-T12) and client (T13)
go test -race -short ./session -run 'TestResolveExtraEnvVars_|TestInstanceData_RoundTripDropsEnvVars' -count=1 -v   # 3 TestResolveExtraEnvVars_* incl. T17
cd web-app && pnpm exec jest --testPathPatterns="useAvailablePrograms" --no-coverage

# Final gates (Task 1.4.2b)
make lint-custom && make lint
cd web-app && pnpm run lint:duplicates
```

## Coverage Targets and How to Measure

This item adds no production code, so the global line-coverage target is not a meaningful gate here. The measurable target is that every function on the env-injection path is exercised by at least one test above.

| Stack | Command (see block below) | Target |
|---|---|---|
| Go `session` | `session.cover` | `resolveExtraEnvVars`, `buildExtraEnv`, `claudeSettingsEnvOverrideArgs`, `shellQuote` each > 0%; empty, clash and marshal branches exercised; no global 80% claim |
| Go `session/tmux` | `tmux.cover` | `newSessionArgs` 100%; the inline `-e` loop in `start()` (`tmux_session_start.go:222-227`) reached |
| Go `server/services` | `svc.cover` | informational; `wireTmuxSession` callers reached |
| TypeScript/Jest | `jest --coverage` | mapping line (`value: p.id`) covered by an `it` where `id !== command` |

```bash
go test -race -short ./session -run 'TestResolveExtraEnvVars_|TestClaudeSettingsEnvOverrideArgs|TestInstance_BuildExtraEnv|TestBuildClaudeCommand_IncludesSettingsEnvOverride' -coverprofile="$SCRATCH/session.cover"
go tool cover -func="$SCRATCH/session.cover" | grep -E 'resolveExtraEnvVars|buildExtraEnv|claudeSettingsEnvOverrideArgs|shellQuote'

go test -race -short ./session/tmux -run 'TestNewSessionArgs_|TestNewSessionArgv_' -coverprofile="$SCRATCH/tmux.cover"
go tool cover -func="$SCRATCH/tmux.cover" | grep -E 'newSessionArgs|SetExtraEnv'

go test -race -short ./server/services -run 'TestCreateSession_CustomProgramEnvVars|TestCreateSession_CustomClaudeProgram' -coverprofile="$SCRATCH/svc.cover"

cd web-app && pnpm exec jest --testPathPatterns="useAvailablePrograms" --coverage --collectCoverageFrom='src/lib/hooks/useAvailablePrograms.ts'
```

- All public service methods on the path: happy path + error path covered, with the exception recorded as Gap G1.
- External integrations (tmux, shell): unit-mocked (T3, T4) plus two real-tmux integration tests (T1, T9).
- UX criteria: none.

## Gaps and Honest Limits

| # | Gap | Severity | Disposition |
|---|---|---|---|
| G1 | **AC1 error path had no unit test.** `resolveExtraEnvVars` was covered only by T2; an unregistered `Program` ID was asserted nowhere | Low | **RESOLVED (iteration 1)**: accepted; T17 added to the plan as Task 1.3.2c (Story 1.3.2 AC, Traceability, Wave 1 Agent C) |
| G2 | **AC4b (precedence over a global `~/.claude/settings.json` `env` block): CLOSED by E9 (executed, real `claude` 2.1.296).** Residual limits: ran the CLI flag directly rather than through a tmux launch (delivery is AC4a / E8); no org-managed settings present. |
| G3 | **AC5 is documentation, not a test.** D2 proves the historical block; nothing can fail if the doc comment above T1 or `requirements.md` rots | Low | Accepted. The doc comment and requirements text are reviewed by hand in PR |
| G4 | **AC3 red proof uses a hand-built mutant.** `PreFixOverlay` is a reconstruction of the pre-fix `wireTmuxSession`, so it is only as faithful as D2's equivalence check (paste both into E3). M4 is the literal pre-fix-commit run but was executed by the coordinator and needs proto/ent regeneration, so it is paste-only | Low | Overlay primary, M4 corroborating, as the plan states. Contingency: if T1 passes under the overlay, halt (assertion vacuous) |
| G5 | **Real-tmux tests are environment dependent.** T1 and T9 skip when no tmux resolves, so a runner without tmux reports green without coverage; a tmux client/server version mismatch fails them as an environment fault. Local 3.6a differs from the CI pin 3.4, and T1/T9 results were not observed on 3.4 in this plan (PIT-3b/D3 one version only) | Medium | **Mitigated by the `RealTmuxGate` (plan Task 1.1.0b)**: existing `TMUX_BIN`, `-v`, `--- PASS: <name>` required, any `--- SKIP` rejected, `STAPLER_SQUAD_TMUX_CREATE_TIMEOUT_SECONDS=30`. Residual: 3.6a vs CI 3.4 (record `tmux -V` per run, label "3.6a only"); CI log must show T1 and T9 as `PASS`, not `SKIP` |
| G6 | **T9 asserts tmux session env and argv, not the FakeClaude process's own `environ`.** FakeClaude records `"$@"` only. Inheritance from tmux session env to the pane process is relied on from T1's `printenv` (bash program), not re-proved on the Claude path | Low | Optional: have FakeClaude also write `env` to its output file and assert the key; one line, would close it. Not in plan |
| G7 | **Only `SESSION_TYPE_NEW_WORKTREE` is exercised end to end.** Other creation and restore entry points share `wireTmuxSession` (8 call sites, one `SetExtraEnv` caller) and are not individually tested (PIT-1 deferral) | Low | Accepted by the plan; T4's two subtests cover the two argv-construction paths |
| G8 | **F1-F5 have no tests.** Secret exposure in logs/`LaunchCommand`, restart policy, reserved keys, remote, tymux are documented follow-ups, not validated | Accepted | By design (Scope Decision B); characterization tests T10-T12 are written to flip when F2 lands |
| G11 | **Generated code and lint prerequisites in fresh worktrees (triad GAP-1).** `gen/` (proto) and `session/ent/*.go` are gitignored, so a new agent worktree cannot compile `./server/services`/`./session`; `make lint` also needs `server/web/dist`, `golangci-lint` v2 and `shellcheck` (`Makefile:798`) | Process | Plan Task 1.1.0b preflight and Task 1.4.2b prerequisites use the real targets (`make proto-gen ent-gen`, `server/web/dist`); an unsatisfied prerequisite is reported as NOT RUN, not green |
| G12 | **Custom-linter coverage of new tests (triad GAP-3).** `tmuxsocketscope`, `novartestseam`, `silenttransition` are in the same `bin/linter` pass as `norawexec`/`notimesleeptest`; `tmuxsocketscope` flags tmux calls whose argv is not derived from `ResolveSocket`/`Socket.Args` | Low | Plan Task 1.1.0a rule list names all three; Task 1.1.3c builds tmux argv with `tmux.ResolveSocket(...).Args(...)`; `bin/linter` output is the authority |
| G13 | **Wave 1 vs real-tmux runs (triad GAP-2).** Editing agents run only vet + `bin/linter`; the `RealTmuxGate` runs for 1.1.0a, 1.1.1b, 1.1.1c, 1.1.3c execute serially in Wave 2R | Process | Plan Dependency Visualization |
| G14 | **evidence.md ownership (triad GAP-4).** Coordinator is the single writer; agents report text blocks; structure is the E1-E9 table in plan "Evidence file structure" | Process | Plan header and "Evidence file structure" |
| G15 | **tmux 3.4 not run locally (triad GAP-6).** Named CI/wall-clock blocker; fallback wording "Verified on tmux 3.6a only; the CI-pinned tmux 3.4 was not run for this evidence" | Medium | Plan Effort Estimate |
| G16 | **AC4b closure: resolved by executing E9.** AC4 as a whole is supported by AC4a (E6, E8) plus AC4b (E9). F1-F5 are still listed in the PR body for the owner to file. |
| G9 | **Wave-1 package contention.** T3/T4 (`session/tmux`), T8/T10-T12 (`session`), T1/T9 (`server/services`), T13 (`web-app`) must be authored one agent per package or per worktree; one half-written `_test.go` breaks `go test` for the whole package | Process | Plan's Dependency Visualization assigns agents A-D by package, each in its own worktree (generated code per worktree), with a Wave 1.5 merge plus integration `bin/linter` run in the coordinator worktree where Waves 2R/3/4 execute (triad iteration 2, G1) |
| G10 | T4 `RestoreWithWorkDir_MissingSession` costs about 1.5 s of `probeSessionExistsWithRetries` backoff with a mock executor; the capture closure needs a `sync.Mutex` under `-race` | Low | In plan (Task 1.2.2a); recorded here so the reviewer checks both |

## Coverage Summary

- Requirements mapped: **5 of 5** to at least one concrete check with an exact command. Executed: AC1, AC2, AC3, AC4a. AC4b executed (E9, real `claude`; G2 closed). Documentation-verified only: AC5 (G3).
- Test functions: **17** (14 unit, 3 integration), of which 8 exist today (6 unit, 2 integration) and 9 are to be created (8 unit including 1 Jest, 1 integration). Subtest cases inside the new ones: T3 6, T4 2, T8 9, T17 2.
- Non-test evidence checks: 3 mutation/overlay runs (M1, M3, M4) plus 1 control (M5), 1 static gate (L1), 1 flake run (F1), 3 documentation/probe checks (D1-D3). UX acceptance tests: 0 (N/A). Migration test: N/A.
- T17 (G1) accepted into the plan as Task 1.3.2c; no proposed additions remain outside the plan.
