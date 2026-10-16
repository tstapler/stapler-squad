# Requirements: Custom Program's Registered Env Vars Aren't Applied to the Spawned Session Process

item_id: 4bbe28f9-09b2-406d-9a72-bdb24df5d509

## Problem

A custom program registered via `UpsertProgramConfig` (e.g. `{"id": "netflix-model-gateway",
"command": "claude", "env": {"ANTHROPIC_BASE_URL": "http://127.0.0.1:47000"}}`) persists
correctly to the resolved workspace config (`~/.stapler-squad/workspaces/<hash>/config.json`,
`session_defaults.programs[]`), but its `env` map is not applied to the process environment
of a session spawned with that program.

Confirmed via a genuinely isolated `SESSION_TYPE_NEW_WORKTREE` session (real worktree
directory, not a directory-collision case): both the running program and a raw `printenv
ANTHROPIC_BASE_URL` inside the session report the ambient/default value, not the registered
one. `tmux show-environment -t <session>` shows no override for the var at all in the
session-scoped table — i.e., the failure is upstream of tmux, not a precedence problem
between tmux's session environment and something else clobbering it downstream.

This defeats the entire purpose of the "Program Configurations" UI/`UpsertProgramConfig`
API: registering per-program env vars (e.g. routing a specific program through a local
proxy via `ANTHROPIC_BASE_URL`) has no effect.

## Prior Art in This Codebase

The codebase already fixed an adjacent bug in this exact area: `claudeSettingsEnvOverrideArgs()`
(`session/instance_tmux.go:616-642`) was added (GitHub issue #852) because a plain inherited
process env var loses to a global `~/.claude/settings.json`'s own `env` block for Claude Code
specifically — that fix emits a `--settings` CLI flag carrying the same resolved env vars so
they win in Claude Code's own settings-precedence order. That fix assumes the tmux-level `-e`
injection (`buildExtraEnv()` / `resolveExtraEnvVars()`) already works correctly as the baseline
layer. This item's repro shows the baseline layer itself is broken: `tmux show-environment`
found no override, so this is not a recurrence of #852 (a Claude-settings-precedence problem)
— it's a break somewhere before or during tmux session creation.

## Where the Reporter Looked (from the item description)

- `config/defaults.go`'s `ResolveProgramConfig`/`FindProgramConfig` correctly resolve the env
  map from `cfg.SessionDefaults.Programs` given a program ID — verified in isolation.
- `session/instance_tmux.go`'s `buildExtraEnv()` calls `config.ResolveProgramConfig` and
  appends `KEY=VALUE` pairs that `session/tmux/tmux_session_start.go` (~lines 214-219) turns
  into `-e` flags on `tmux new-session`.
- Both look correct in isolation; the reporter did not pin the exact break and suspects the
  wiring between them — timing, whether `buildExtraEnv()` runs before/during `new-session`,
  or whether `config.LoadConfig()` inside `buildExtraEnv()`/`resolveExtraEnvVars()` reads the
  same config the API server wrote to.

## Additional Orientation Gathered During Triage (not yet a root-cause finding — for sdd:2-research to confirm or refute)

- `resolveExtraEnvVars()` (`session/instance_tmux.go:585-599`) reads `snap := i.Snapshot()`,
  then `config.LoadConfig()`, then `config.ResolveProgramConfig(cfg, snap.Program)`. All code
  paths that create a tmux session for an `Instance` (`initTmuxSession` →  `wireTmuxSession` →
  `buildExtraEnv()`) appear to funnel through this single choke point — no alternate
  session-creation path was found that bypasses it.
- `config.LoadConfig()` resolves its config directory via `GetConfigDir()` →
  `GetConfigDirForDir("")`, which is driven by the *server process's* cwd/instance/workspace
  state, not the session's worktree directory — so, at first read, the same server process
  should resolve the same config dir for both the `UpsertProgramConfig` write and the
  `buildExtraEnv()` read. Whether this holds under workspace-mode's preferred-workspace-file
  and per-directory-isolation logic (`config/config.go`'s priority list) needs to be verified,
  not assumed — sdd:2-research should confirm this rather than treat it as ruled out.
- The `TmuxSession` struct that ultimately renders `-e` flags has two parallel fields,
  `ExtraEnv` (exported) and `extraEnv` (unexported) — `session/tmux/tmux_session_start.go`
  appends both to `-e`. Whether `wireTmuxSession()`'s `session.SetExtraEnv(extraEnv)` call
  actually reaches the field consumed at `new-session` time, and whether any other code path
  writes to the *other* field first and gets overwritten (or vice versa), is worth checking.
  Not yet confirmed as the cause.
- Snapshot's own doc comment notes it "also serves pre-publication callers (fromInstanceData):
  it lazily builds one" — worth checking whether `buildExtraEnv()` can run against a
  lazily-built `InstanceSnapshot` whose `Program` field hasn't been populated yet from
  `opts.Program`, which would make `ResolveProgramConfig` resolve against an empty/wrong
  program ID and silently return no custom env.

## Acceptance Criteria (draft — refined further in plan.md / validation.md)

1. Registering a custom program's `env` map via `UpsertProgramConfig` and creating a new
   `SESSION_TYPE_NEW_WORKTREE` session with that `program` results in the registered env
   vars being present and correctly valued in the spawned session's process environment
   (`printenv <VAR>` inside the pane reports the registered value).
2. `tmux show-environment -t <session>` for that session shows the registered var(s) in the
   session-scoped table.
3. A regression test exists that would have caught this bug (integration-level: register a
   program with an env var, create a session, assert the env var reaches the tmux session /
   spawned process — not just that `ResolveProgramConfig` returns the right map in isolation,
   since that unit-level check already existed and did not catch this).
4. No regression to the existing `claudeSettingsEnvOverrideArgs()` (#852) behavior — custom
   program env vars must still win over a global `~/.claude/settings.json` `env` block for
   Claude Code sessions specifically.
5. Root cause is identified and documented (not a symptom-only fix) — per this repo's
   engineering-discipline norm of stating and confirming a root-cause hypothesis before
   changing code.

## Out of Scope

- The companion "directory collision" issue referenced in the bug report ("this isn't a
  directory-collision case (see the companion issue for that separate bug)") — separate item.
- Any redesign of the Program Configurations UI/API surface itself; this is a wiring/data-flow
  bug in an existing, otherwise-correct feature.
