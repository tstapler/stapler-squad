# ADR-001: Global-Only Claude-Settings Scope for v1 (No Per-Worktree Overlay)

**Status**: Accepted
**Date**: 2026-08-06
**Project**: dynamic-rule-reload

## Context

There is exactly one shared `*classifier.RuleBasedClassifier` per stapler-squad server
process (`pkg/classifier/classifier.go:379-387`). `Rule` has no project/path-scoping field.
stapler-squad, however, runs many concurrent sessions across different git worktrees
(`session/git/` worktree management), each potentially checked out to a different branch
with its own `<worktree>/.claude/settings.json`.

`LoadClaudeSettingsRules(projectDir string)` (`server/services/claude_settings_parser.go:59`)
already accepts a single `projectDir` and reads that directory's `.claude/settings.json` /
`.claude/settings.local.json` at priority 170/180 (above the global paths at 150/160). Once
wired into the live classifier (this project's Phase 2), a rule loaded via any one
`projectDir` is evaluated against **every** session on the server, regardless of which
worktree that session is actually running in.

A per-request `ClassificationContext` (built via `h.classifier.BuildContext(payload.Cwd)`)
already carries `Cwd`/`RepoRoot`/`IsWorktree` per `Classify()` call — the hook a true
per-worktree overlay would need — but wiring rule evaluation through that context to select
a worktree-specific rule subset is a materially bigger change: it requires either
per-request rule filtering inside `Classify()` (touching the hot classification path) or a
per-worktree classifier instance (multiplying the fsnotify-watch surface by session count).

## Decision

For v1, `NewSessionService` calls `LoadClaudeSettingsRulesDetailed(cwd)` where `cwd` is the
**stapler-squad server process's own working directory** (`os.Getwd()`), not any individual
session's worktree path. In practice this means:

- `~/.claude/settings.json` (global, priority 150)
- `~/.claude/settings.local.json` (global-local, priority 160)
- `<server's own cwd>/.claude/settings.json` (priority 170) — relevant when stapler-squad
  itself is launched from within a git checkout that has project-level settings
- `<server's own cwd>/.claude/settings.local.json` (priority 180)

are loaded and watched. No per-session-worktree `.claude/settings.json` is loaded or
watched by this project.

## Consequences

- **Positive**: matches the literal scope of the original ask (global rule updates without
  restart); zero changes to the hot `Classify()` path; the fsnotify watch surface stays at
  2-4 files for the life of the process, not O(number of live worktrees).
- **Negative**: a session running in worktree `W` with its own `<W>/.claude/settings.json`
  will **not** have that file's rules applied — only the server's own cwd's project settings
  and the global settings apply to all sessions uniformly. This is a real capability gap
  relative to the original issue's spirit (per-project tuning), but it is the gap the
  original issue's author would have hit regardless, since nothing in the retired
  `WatchAndReload` design or the current dead `LoadClaudeSettingsRules` call handled
  per-worktree scoping either.
- **Follow-up (explicitly future work, not silently dropped)**: a true per-worktree overlay
  would evaluate `LoadClaudeSettingsRules(ctx.RepoRoot)` at classify-time (using the
  `RepoRoot` already available on `ClassificationContext`) rather than at
  classifier-construction-time, likely with a small per-`RepoRoot` cache to avoid re-parsing
  settings files on every `Classify()` call. This is a distinct, larger project and is not
  scheduled by this ADR.

## Alternatives Considered

1. **Per-request project overlay** (evaluate `LoadClaudeSettingsRules(ctx.RepoRoot)` inside
   `Classify()`, using the existing `RepoRoot` field) — more correct for the multi-worktree
   case, but touches the hot classification path and requires either a cache or accepting a
   syscall-per-classification cost; rejected for v1 as materially bigger than this project's
   stated scope (requirements.md explicitly asks for a scoped fix, not a redesign of rule
   evaluation).
2. **Per-worktree classifier instances** — one `RuleBasedClassifier` per active session,
   watching that session's own worktree settings — rejected: multiplies the watcher count by
   live-session count, and DB-backed (`user`) and `seed` rules would need to be duplicated
   into every instance too, since they're currently shared globally.
