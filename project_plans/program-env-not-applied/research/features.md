# Research: Comparable "config → spawned subprocess env" Features

## The injection pipeline (as it exists today)

Two parallel mechanisms feed environment into a spawned session's tmux `new-session`:

1. **`session/instance_tmux.go`'s `buildExtraEnv()`/`resolveExtraEnvVars()`** (lines 579-614) —
   combines `STAPLER_SESSION_UUID`, custom-program env (`config.ResolveProgramConfig(cfg,
   snap.Program).EnvVars`, only when `IsCustom`), and instance-level `EnvVars` (instance wins on
   key collision — see the doc comment on `resolveExtraEnvVars`, lines 579-584). Result is stored
   via `session.SetExtraEnv(extraEnv)` into `TmuxSession.extraEnv` (private field,
   `session/tmux/tmux.go:1172-1176`).
2. **DISPLAY (VNC) and CDP_PORT injection** (`session/instance_vnc.go`, `session/instance_cdp.go`,
   and the restart/resume branches in `session/instance.go` lines 1538, 1545, 1643, 1650, 1810,
   1817, 1906, 1913) — append directly to `TmuxSession.ExtraEnv` (**exported** field,
   `session/tmux/tmux.go:251-253`), a structurally distinct field from `extraEnv`.

Both fields are consumed identically at the point tmux is actually invoked —
`session/tmux/tmux_session_start.go`:
- fresh session creation, `start()` lines 214-220: `newSessionArgs := []string{"new-session", ...}` then
  loops `t.ExtraEnv` then `t.extraEnv`, each appended as `"-e", kv"`.
- `recreateMissingSession`'s `newSessionArgs()` helper, lines 617-623: identical double loop.

There is exactly **one** code path for `tmux new-session` regardless of control-mode
(`STAPLER_SQUAD_USE_CONTROL_MODE`) vs legacy polling — `TmuxSession.Start()`/`StartWithCleanup()`
both funnel through the same private `start()` (`tmux_session_start.go:28-43`). Control mode only
changes how *output* is read afterward, not how the session is created, so it is not a separate
env-injection path to worry about.

`buildExtraEnv()`/`SetExtraEnv()` is wired into the actual `TmuxSession` object by
**`wireTmuxSession()`** (`instance_tmux.go:644-676`), called from:
- `initTmuxSession()` (fresh `SESSION_TYPE_NEW_WORKTREE`/directory creation path, line 697)
- 5 call sites in `session/instance_serialization.go` (lines 463, 468, 521, 535, 548) — one per
  restored `Status` branch (`Paused`, `Stopped`, `Hibernated`, `Crashed`, and the generic
  active-restore `else`), all passing `instance.Program` and gated on
  `processManager.(*TmuxBackend)`.
- 2 call sites in `session/instance.go` (lines 2252, 2453) inside restart/relaunch flows.

## A directly comparable, already-fixed bug: GitHub issue #852

`claudeSettingsEnvOverrideArgs()` (`instance_tmux.go:616-642`) exists specifically because a
custom program's env vars reaching tmux's `-e` flags was **not sufficient** — Claude Code's own
`~/.claude/settings.json` `env` block takes precedence over a plain inherited process env var of
the same name, so `ANTHROPIC_BASE_URL` set via tmux `-e` was silently discarded whenever a
user/org settings file set the same key (confirmed live against a Netflix-wrapper-installed
settings.json). The fix re-injects the *same* `resolveExtraEnvVars()` map via a `--settings`
CLI flag on the `claude` launch command itself, so both paths must always agree (see the doc
comment explicitly calling this out, lines 582-584).

This is strong precedent that "the resolver function returns the right map" is not the same
claim as "the env var actually reaches the process" — that gap is exactly what burned issue #852,
and the same class of gap is what this bug report describes one layer earlier (tmux, not
settings.json).

## Existing test coverage — and the exact gap

All existing coverage is **unit-level**, asserting on the *string content* the resolver/builder
functions return — none spin up a real tmux session and check its live environment table
(`tmux show-environment`) or a spawned process's actual `os.Environ()`:

- `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars`
  (`session/instance_tmux_test.go:1206-1231`) — asserts `instance.buildExtraEnv()`'s returned
  `[]string` contains the expected `KEY=VALUE` pairs, including the instance-vs-program collision
  case. Never touches `TmuxSession` or tmux itself.
- `TestClaudeSettingsEnvOverrideArgs_CarriesResolvedEnvVars` / `_EmptyWhenNoEnvVars`
  (lines 1236-1277) — same, for the `--settings` JSON payload.
- `TestBuildClaudeCommand_IncludesSettingsEnvOverride` (lines 1279-1298) — one step closer:
  asserts the assembled `buildLaunchCommand()` string contains `--settings` and the var name.
  Still string-matching an unexecuted command line, not tmux state.
- `grep -rln "tmux show-environment\|ShowEnvironment\|show_environment" --include="*.go" .` —
  **zero matches** repo-wide. No test anywhere asserts against a live tmux session's environment
  table, which is exactly the diagnostic command the bug report itself used to confirm the
  failure (`tmux show-environment -t <session>` shows no override at all).

This confirms the task's framing: the regression test this bug needs is an **integration test**
that (a) registers a custom program via the real `UpsertProgramConfig` path (not a direct
`config.SaveConfig` splice like `seedCustomProgram`), (b) creates a real `SESSION_TYPE_NEW_WORKTREE`
session using it, and (c) either shells out to `tmux show-environment -t <session>` or spawns
`printenv VAR` inside the session and reads back the pane output — mirroring how the reporter
manually confirmed the bug. `seedCustomProgram` (`instance_tmux_test.go:1196-1204`) is also worth
noting as a *possible confound*: it forces `STAPLER_SQUAD_TEST_DIR` (priority-1 in
`config.GetConfigDirForDir`, `config/config.go:104-170`), which bypasses the
workspace-preference/`STAPLER_SQUAD_WORKSPACE_MODE` resolution path (priorities 4-5) entirely —
so if the real bug is a mismatch between the config directory `UpsertProgramConfig` writes to and
the one `resolveExtraEnvVars()`'s `config.LoadConfig()` reads from (both go through
`GetConfigDir()` → `GetConfigDirForDir("")` → `resolveDefaultConfigDir` →
`workspacepath.ResolveDefaultDir`), none of the existing unit tests can see it — worth checking
as part of root-causing this, though confirming that is implementation-phase work, not this
research task's scope. `config.LoadConfig()` itself does not cache in memory (re-reads
`configPath` from disk every call, `config/config.go:1203-1219`), so a stale in-process cache is
ruled out as the cause.

## Edge cases to cover in the fix + regression test

1. **Custom vs. built-in programs** — `resolveExtraEnvVars()` only pulls program-level env when
   `config.ResolveProgramConfig(cfg, snap.Program).IsCustom` is true (`instance_tmux.go:590`); a
   built-in program (e.g. plain `"claude"`) must still get instance-level `EnvVars` and
   `STAPLER_SESSION_UUID`, just no program-level map.
2. **Instance-level `EnvVars` overriding program-level `EnvVars`** — `resolveExtraEnvVars()`
   applies program-level entries first, then overwrites with instance-level on key collision
   (map keyed by name, `instance_tmux.go:588-597`); already has a passing unit test
   (`TestInstance_BuildExtraEnv_...`'s `CLASH` case) but no integration-level equivalent.
3. **Resume/restore vs. fresh creation** — the 5 branches in
   `session/instance_serialization.go` (`Paused`, `Stopped`, `Hibernated`, `Crashed`, generic
   active-restore) all call `wireTmuxSession(instance.Program)`, which re-derives
   `buildExtraEnv()` from the current `Snapshot()`/`Program` at restore time — a program's env
   vars edited *after* a session was originally created should be picked up on next
   resume/restart, and a regression test should cover at least one of these five paths, not just
   fresh `initTmuxSession()`.
4. **`ExtraEnv` (exported, DISPLAY/CDP) vs. `extraEnv` (private, program/instance vars) not
   clobbering each other** — both are independently appended as `-e` flags in the same
   `newSessionArgs` loop (`tmux_session_start.go:214-220`, `617-623`); a session with both a VNC
   DISPLAY assignment and a custom program's `ANTHROPIC_BASE_URL` should get both, and neither
   field's setter (`SetExtraEnv` for `extraEnv`, direct `sess.ExtraEnv = append(...)` for
   `ExtraEnv`) should overwrite the other.
5. **Remote/SSH-backed sessions** — `session/tmux/remote_env.go`'s `wrapRemoteCommand` prefixes
   remote-bound tmux invocations with `env -u TMUX TERM=xterm-256color` when
   `CommandRunner.IsRemote()`, but this wraps the *tmux binary invocation itself*, not the
   `new-session -e ...` argv construction — the `-e` flags should be unaffected, but this is
   exactly the kind of adjacent-but-different mechanism worth an explicit test given it's the
   only other place env normalization happens for tmux launches. No existing test asserts `-e`
   flags survive `wrapRemoteCommand`'s rewrite.
6. **`recreateMissingSession` path** — a session whose tmux server died and gets silently
   recreated (`newSessionArgs()` helper, `tmux_session_start.go:611-623`) must carry the same
   `-e` flags as the original `start()` path; currently duplicated logic (two near-identical
   loops), so a fix or regression test should ideally cover both call sites, or the duplication
   itself refactored into one shared helper.

## Files referenced

- `session/instance_tmux.go:579-676` (`resolveExtraEnvVars`, `buildExtraEnv`,
  `claudeSettingsEnvOverrideArgs`, `wireTmuxSession`)
- `session/tmux/tmux.go:129-130,251-253,1172-1176` (`extraEnv`/`ExtraEnv` field definitions,
  `SetExtraEnv`)
- `session/tmux/tmux_session_start.go:209-221,611-623` (`-e` flag construction, both call sites)
- `session/instance_serialization.go:460-549` (resume/restore `wireTmuxSession` call sites)
- `session/instance.go:1065-1145` (`NewInstance` — confirms `Program`/`EnvVars` are set directly
  on construction, no snapshot race for the fresh-creation path)
- `session/instance.go:1207-1226` (`Snapshot()` — lazy-builds via `buildSnapshot(i)` if unset)
- `config/config.go:94-193` (`GetConfigDir`/`GetConfigDirForDir`/`resolveDefaultConfigDir` —
  workspace-hash resolution priority list) and `config/config.go:1203-1219` (`LoadConfig` — no
  in-memory caching, reads disk every call)
- `session/instance_tmux_test.go:1196-1298` (existing unit tests: `seedCustomProgram` helper,
  `TestInstance_BuildExtraEnv_IncludesCustomProgramAndInstanceEnvVars`,
  `TestClaudeSettingsEnvOverrideArgs_*`, `TestBuildClaudeCommand_IncludesSettingsEnvOverride`)
- `session/tmux/remote_env.go` (`wrapRemoteCommand` — SSH/remote env normalization, distinct
  mechanism from `-e` flag injection)
