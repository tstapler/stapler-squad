# BUG-116: A second stapler-squad instance's startup cleanup kills the live instance's tmux control-mode clients and deletes its cdp wrapper dirs [SEVERITY: High]

**Status**: ✅ Fixed
**Discovered**: 2026-09-29, isolated-instance smoke test for backlog item `f7201b49-8318-43fb-a22f-0ce1926f9980` (AC 6).

## Problem Description

`CLAUDE.md`'s manual-testing recipe says `STAPLER_SQUAD_INSTANCE=<name>` gives a second instance its own
state and "will not see or affect the live deployed instance". Config, DB and the periodic orphan sweeper are
isolated (`session/orphan_tmux_sweeper.go:65` logs `skipping — isolated instance`), but two startup cleanups are not:

1. `tmux.KillOrphanedControlModeClients("")` (`main.go:430`, impl `session/tmux/tmux.go`) lists every
   control-mode client on the tmux server and kills each one. Its premise, "any control-mode client
   already attached is a leftover from a prior process", is false when a second instance shares the tmux
   server, which the recipe does (`--tmux-keep-server`, default socket).
2. `reconcileOrphanDirs` (`session/cdp/manager.go`) removes every `~/.stapler-squad/cdp-bins/<id>` whose id is
   not in THIS instance's session list. That directory is shared, not per-instance.

## Evidence

Manual instance log (`~/.stapler-squad/instances/claude-manual-test-f7201b49/logs/staplersquad.log`, 2026-09-29,
started 22:21:50 local, VERIFIED):
- `[tmux] killed orphaned control-mode clients from a prior process instance` count=2
- `cdp: ReconcileOrphans: removed orphan wrapper dir` for 4 session ids the manual instance never owned.

Same window on the live instance: tmux baseline 21 sessions, 20 afterward; `stapler-squad-agy-hooks` and
`stapler-squad-fix-mcp-read-output-without-stream` were gone (both present right after the manual instance's
own session was created, 22:22).

## Root Cause

Code path VERIFIED (above). That it caused the two missing sessions is INFERRED, not proven: the log
shows two control-mode clients killed and two sessions missing, and the live log shows tmux
control-mode zombie children for `agy-hooks`, but no log line ties a kill to a session ending.

## Impact

Following the documented manual-testing recipe can degrade the live instance: it loses control-mode streams,
and may lose sessions. Also makes any "isolated instance" test unsafe to run beside real work.

## Suggested Fix

Scope both cleanups to what this instance owns: filter control-mode clients by a marker this process set
(or skip the startup kill entirely when `STAPLER_SQUAD_INSTANCE` is set, as the periodic sweeper does), and
key cdp dirs per instance or skip reconcile when isolated. Add a test that starts two instances on one tmux
server and asserts the first one's clients and dirs survive.

## Resolution

Both startup cleanups are now gated on `config.IsIsolatedInstance()` (named instance, test mode, or
`STAPLER_SQUAD_TEST_DIR`), and each logs a skip message:

- `cleanupOrphanedControlModeClients` in `main.go` replaces the inline `tmux.KillOrphanedControlModeClients("")` call.
- `reconcileOrphanDirs` in `session/cdp/manager.go` returns early, which covers both the real and noop CDP managers.

Tests: `main_cleanup_test.go`, `session/cdp/reconcile_isolation_test.go` (named-instance, test-dir-only, and live cases).
