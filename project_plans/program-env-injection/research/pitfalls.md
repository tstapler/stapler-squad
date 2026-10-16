# Pitfalls: custom program env must reach the spawned tmux session

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509
Prior research: `project_plans/program-env-not-applied/research/pitfalls.md` (called "prior" below).
Code reviewed at branch HEAD `453c8098f`. Confidence labels: VERIFIED = source opened or command run
this session; INFERRED = reasoned from verified code, not executed.

## Current pipeline (VERIFIED, `session/instance_tmux.go`)

- `resolveExtraEnvVars` (710-724): `i.Snapshot()` -> `config.LoadConfig()` -> `ResolveProgramConfig(cfg, snap.Program)`
  env, then `snap.EnvVars` overlaid (instance wins).
- `buildExtraEnv` (729-739): `STAPLER_SESSION_UUID` + the resolved map, as `KEY=VALUE` strings.
- `claudeSettingsEnvOverrideArgs` (756-767): same map as `--settings '<json>'`, called from `buildClaudeCommand` (480).
- `wireTmuxSession` (771-801): builds a new `TmuxSession`, `SetExtraEnv` only when `len(extraEnv) > 0` (794-796).
- tmux layer: `-e` flags appended as argv elements in `session/tmux/tmux_session_start.go:225-230` (Start) and
  `:638-648` (`newSessionArgs`, recreate path).

## 1. Findings carried over from prior research

| Prior finding | Status | Evidence |
|---|---|---|
| 1. Every `wireTmuxSession` call site is a separate place the bug can live | **Still holds, line numbers drifted.** Sites are now `session/instance.go:2399`, `:2600`, `session/instance_tmux.go:822` (via `initTmuxSession`), and `session/instance_serialization.go:467,472,536,550,563`. Prior cited `instance.go:2252/2453`, which no longer match. | grep of `wireTmuxSession(` |
| 2a. Independent `Snapshot()` calls across one launch can disagree | **Still holds, and is wider than stated.** One create/restart does `Snapshot()` at 355 (`buildLaunchCommand`), 712 (`resolveExtraEnvVars`), 730 (`buildExtraEnv`), 772 (`wireTmuxSession`), plus `config.LoadConfig()` at 359, 714, and again via `claudeSettingsEnvOverrideArgs`->714. So `-e` flags and the `--settings` JSON in the same launch are resolved in separate calls and a concurrent `UpsertProgramConfig`/`SetProgram` can make them differ. Design response: resolve the env map once per launch and pass it to both consumers. | `instance_tmux.go:355,359,712,714,730,757,772` |
| 2b. Field-name mismatch | **Still ruled out.** `ProgramConfig.Env` -> `ResolvedProgram.EnvVars` (`config/defaults.go:211-222`); regression test passes `Env:` through the real RPC and passes (requirements.md). | defaults.go:211 |
| 2c. Config loaded from a different scope than the writer | **Still holds, partially mitigated in test only.** `resolveExtraEnvVars` still calls bare `config.LoadConfig()` (714). The regression test shares one dir via `envtest.NewIsolatedStateDir(t)` (`session_service_create_test.go:~690`), so it proves agreement only inside one process/scope. | instance_tmux.go:714 |
| 2d. Silent swallow on an error branch | **Still holds.** `claudeSettingsEnvOverrideArgs` degrades to `("","")` with a `log.Warn` on marshal failure (763-765); `SetExtraEnv` skipped when empty (794). Neither can fail for `map[string]string` in practice. | instance_tmux.go:761-765 |
| 2e. Reuse skips the wiring, so env is stale ("leading suspect") | **Still holds as a design hazard, but is NOT the cause of the original bug** (that was missing merge code, per requirements.md). Two reuse points: `initTmuxSession` returns early when `HasSession() && (IsAlive()\|\|IsBackendProcessAlive())` (807-810); and `TmuxSession.Start` reuses a same-named existing tmux session without running `new-session`, so no `-e` is applied (`tmux_session_start.go:~189-196`, log "tmux session already exists, reusing"). Env changed in `UpsertProgramConfig` therefore does not reach a live/reused session until it is killed and recreated. Document as expected behaviour. | instance_tmux.go:806-810; tmux_session_start.go:189-196 |
| 3. `-e` is argv, not shell-parsed | **Still holds.** argv append at `tmux_session_start.go:225-230`. Remote runner shell-quotes each argv element itself (`session/tmux/ssh_runner.go:597-617`), so values with spaces/`$` survive there too (INFERRED for the full remote path; helper opened, end-to-end remote not exercised). | files cited |
| 3. `update-environment` / global env could beat `-e` | **Resolved: does not hold.** Ran tmux 3.6a on a private socket: server global env `BAR=global-bar`, `update-environment FOO`, client env `FOO=client`, `new-session -e FOO=explicit -e BAR=explicit-bar -e 'JSON={"env":{"K":"v"}}'`. `show-environment -t` printed `FOO=explicit`, `BAR=explicit-bar`, `JSON={"env":{"K":"v"}}`. A control session without `-e` got `FOO=client` (update-environment applies only absent `-e`). | command run this session (VERIFIED, one tmux version) |
| 4. #852 is precedence, not injection; do not extend `claudeSettingsEnvOverrideArgs` | **Still holds.** Function reuses `resolveExtraEnvVars()` (757), so it cannot diverge in *content* (only in timing, see 2a). | instance_tmux.go:756-757 |
| 5. Unit tests on `&Instance{}` literals miss wiring | **Still holds for the unit tests; no longer holds for the suite as a whole.** `session/instance_tmux_test.go:1202-1278` still build literals and call the resolver directly (`TestInstance_BuildExtraEnv_...`, `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars`). The wiring gap is now covered by `TestCreateSession_CustomProgramEnvVars_ReachesTmuxSession` (`session_service_create_test.go:673`): real `UpsertProgramConfig` -> real `CreateSession` -> real tmux, asserting `show-environment` and in-pane `printenv`. Prior's table-driven coverage of every wire site and fake-runner argv capture were not implemented (no test calls `newSessionArgs` or captures `-e`; only `tmux_ownership_test.go` uses `SetExtraEnv`). | files cited |

## 2. New pitfalls found in current code

### A. Env values leak into logs and storage via the launch command (VERIFIED, high)
For a Claude program, `buildClaudeCommand` embeds `--settings '{"env":{"ANTHROPIC_BASE_URL":"...","TOKEN":"..."}}'`
(480-481, 766) in the launch command, and that whole string is then:
- logged at INFO: `instance_tmux.go:817` (`"program", enrichedProgram`), `tmux_session_start.go:384` and `:671`
  (`"program", programWithHistory` / `program`);
- stored in `Instance.LaunchCommand` (817) and persisted as `launch_command` (`session/instance_serialization.go:165`,
  `session/storage.go:160-162`);
- returned to clients: `server/adapters/instance_adapter.go:124`.

`TestCreateSession_should_NotLogEnvVarValues_When_EnvVarsProvided` (`session_service_create_test.go:884`) does not
catch this: it supplies an invalid `ResumeId` and asserts `require.Error`, so it exits before any launch-command
log. It only guards the `[CreateSession] request shape` debug line (`session_service_create.go:613-624`, keys only).
Design options: log a redacted command (replace the `--settings` value), or stop putting env in argv by writing a
0600 settings file; add a test that runs a real create with a secret value and greps captured logs and the
persisted `LaunchCommand`. Also note `-e KEY=VALUE` values are visible in the `tmux` client process argv (`ps`),
same exposure class as before; not new.

### B. Two stores of program env, and restart semantics differ by path (VERIFIED)
- At create time `resolveSessionDefaults` copies the program's env into the instance's `EnvVars` when the key is
  not already set (`session_service_create.go:280-290`). `${VAR}` expansion (`config.ExpandEnvVars`,
  `defaults.go:244-266`) happens then, against the server process env, and a key whose `${VAR}` is unset is
  silently dropped (log.Warn at 260, no value logged).
- `resolveExtraEnvVars` also resolves the program env fresh from `LoadConfig()` and overlays `snap.EnvVars` on top
  (instance wins, 715-722). So on any in-process restart/resume the create-time copy wins over later edits to the
  program config; a removed program key stays; a rotated token is not picked up.
- **`EnvVars` is not persisted**: no reference in `session/storage.go` `InstanceData` or
  `instance_serialization.go` (grep). After a server restart a reloaded session has empty `EnvVars`, so program env
  is re-resolved from current config (fresh) while request-level `env_vars` are lost. The same program therefore
  gets different effective env depending on whether the session restarted in-process or across a server restart.
  Pick one policy deliberately (requirements.md does not state it) and test both.

### C. Restore paths wire with the raw program ID, relaunch re-resolves the command but not `-e` (VERIFIED + INFERRED)
`instance_serialization.go:467,472,536,550,563` call `wireTmuxSession(instance.Program)` (the custom ID, not a
launch command); `WithProgramProvider(i.currentLaunchCommand)` (779) makes `recreateMissingSession` re-resolve
the *command* at relaunch (`tmux.go:1097-1104`, `tmux_session_start.go:~628`). But `t.extraEnv` was frozen at
wire time (`tmux.go:1238-1240`). If the program env is edited between wire and relaunch, the relaunched command's
`--settings` JSON (fresh) and `-e` set (stale) disagree (INFERRED; not executed). Same root as 2a.

### D. Key collision (VERIFIED)
Collision is resolved in Go before rendering: instance `EnvVars` replace program keys in one map (720-722), so tmux
never sees duplicate `-e` keys; `TestInstance_BuildExtraEnv_...` (`instance_tmux_test.go:1202-1230`) asserts it.
Remaining collisions not handled: (i) `STAPLER_SESSION_UUID` is appended first in `buildExtraEnv` (732-734) and a
user key of the same name would come later and win in tmux (`-e` later wins is INFERRED, not tested), breaking the
ownership check in `tmux_ownership.go:~44-52`; (ii) `CLAUDECODE=` is hard-coded first in the argv
(`tmux_session_start.go:222`) and a user key `CLAUDECODE` also overrides it; (iii) `TmuxSession.ExtraEnv`
(exported, `tmux.go:251-253`) and `extraEnv` are both applied (225-230), so a second writer to either is silent.
Design: reserve/strip `STAPLER_*` and `CLAUDECODE` keys from user maps, or assert order in a test.

### E. Shell quoting of `--settings` (VERIFIED code, partial test coverage)
`shellQuote` (433-435) POSIX single-quotes and escapes `'` as `'\''`, so JSON containing `$`, backticks or quotes
is inert. The final command also goes through `env HISTFILE=... <program>` (`tmux_session_start.go:221`) then tmux's
shell. Gaps: the existing test strips the outer quotes and unmarshals (`instance_tmux_test.go:1266-1272`), which
would break on a value containing `'` and no test uses such a value; no test covers newline or non-UTF-8 bytes;
tmux command-length limit exists (see `session/instance_tmux_command_length_test.go`), and a large `--settings`
payload spends that budget. Add a table case with `'`, `$(...)`, spaces, and `=` in values, executed through a real
shell (as `instance_tmux_command_length_test.go:108` does with `-e`).

### F. Claude settings.json precedence (#852) (VERIFIED from code comment; docs not re-fetched)
Inherited env loses to a settings-file `env` block, hence `--settings` (comment at 741-755). Gaps: a non-Claude
program (e.g. the regression test's `bash`) gets `-e` only, which is correct; org-managed settings still outrank
`--settings` per the cited doc, so env can still be overridden by managed policy and nothing detects it. The
regression test uses `Command: "bash"`, so it does not exercise the `--settings` path at all; AC4 is covered only
by `TestBuildClaudeCommand_IncludesSettingsEnvOverride` (`instance_tmux_test.go:1278`) at string level.

### G. Regression-test flakiness (VERIFIED by reading test, not by repeated runs)
`session_service_create_test.go:673-748`:
- Not parallel; uses `tmux.Binary()` (honours `TMUX_BIN`, `session/tmux/binary.go:16-20`, but the embedded-build
  variant extracts a binary first, `binary_embedded.go:34-46`) and `-L svc.testTmuxServerSocket`
  (`session_service.go:913`, per-PID per-service counter). The test's own `exec` calls use `-L socket` but not the
  gate used by production code, so a concurrent production tmux call on the same socket is theoretical only.
- The skip guard is only `LookPath`; a tmux client/server version mismatch (documented in
  `session/tmux/binary_resolution.go:38-39`) would surface as `Stopped`, which the test correctly fails rather than
  skips. Good, but it makes the test environment-sensitive: failure there is not a regression.
- Readiness: 30 s poll for `Active`, then up to 15 s re-sending `send-keys` every 300 ms. Re-sending is safe for
  the assertion but appends many `echo` lines; `capture-pane -J` with a default 80-col pane can wrap a long probe
  line (mitigated by `-J`). It asserts on a `bash` prompt-less pane, so a user `~/.bashrc` that exports
  `ANTHROPIC_BASE_URL` could mask the injection (the pane inherits the user's HOME). INFERRED, not demonstrated;
  consider `env -i`-style command or a unique key name instead of the real `ANTHROPIC_BASE_URL`.
- `show-environment -t` asserts the session table; with a real `ANTHROPIC_BASE_URL` already in the server's
  global env the session value still wins (verified in section 1 experiment).
- Status `Stopped` poll breaks early only on `Active`/`Stopped`; `Paused`/other terminal states would wait the full
  30 s then fail with a clear message.

## 3. Recommendations for design

1. Resolve the env map once per launch (single `Snapshot()` + single `LoadConfig()`) and feed `-e` and `--settings`
   from it; stops 2a/C divergence.
2. Decide and document restart semantics (B): frozen-at-create vs re-resolve-each-launch. Either way, persist or
   deliberately drop `EnvVars`, and stop merging program env into `EnvVars` at create if re-resolve is chosen.
3. Redact `--settings` env values from the three INFO logs and from persisted/returned `LaunchCommand` (A), with a
   real-launch secret-leak test.
4. Protect `STAPLER_SESSION_UUID`/`CLAUDECODE` from user override (D).
5. Add tests: argv capture via `WithCommandRunner` asserting `-e KEY=VALUE` in `new-session`; quote-hostile values
   through a real shell (E); a Claude-program variant of the real-tmux test; a reload-then-restart variant (B/C).
