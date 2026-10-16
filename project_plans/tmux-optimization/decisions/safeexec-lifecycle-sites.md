# Remaining `safeexec` call sites in `session/tmux` — why they stay subprocess calls

**Date**: 2026-08-21
**Project**: tmux-optimization

## Context

ADR-001 and ADR-002 migrated the per-tick, per-session hot paths (pane
queries, dirty checks, gh-auth checks, health polling) off subprocess forks.
This doc accounts for the `safeexec.CommandContext` call sites that remain in
`session/tmux/*.go` (non-test) and states why each one is intentionally not a
control-mode/batching migration candidate.

```
grep -rn "safeexec.CommandContext" session/tmux/*.go | grep -v _test.go
```
finds 18 call sites, in `server_registry.go`, `shell_handle.go`, `tmux.go`,
and `zombie_detector.go`.

## Classification

**One-shot lifecycle operations (13 sites)** — fired on a session state
transition (create, kill, resize, option-set), not on a recurring poll tick.
Migrating these to control mode would add CM-protocol risk to code paths that
already run at most once per session lifecycle event, for no measurable
subprocess-volume win:

- `tmux.go`: `has-session` checks (×2, lines 460, 763), `new-session` (774),
  `start-server` (641), `remain-on-exit` set (665), `set-option` (744),
  `kill-session` (2705), `list-sessions`/`list-servers` used for socket
  discovery at startup (367, 411, 2682), `kill -WINCH` resize signal (2387,
  targets the pane's OS process directly — not a tmux command, has no CM
  equivalent).
- `shell_handle.go:68`: spawns the user's shell process itself — inherently a
  one-shot `exec`, not a tmux query.
- `shell_handle.go:201` (`ExitCode()`, uses `display-message -p
  #{pane_dead_status}`): looked like a candidate for the same per-tick
  `display-message` bug ADR-001 fixed via `BatchPaneDeadStatus`, but its only
  caller (`instance_shells.go:224`, inside `watchShellExit`) invokes it
  exactly once, after the shell's PTY hits EOF — not on a polling interval.
  Confirmed via `grep -rn "\.ExitCode()" session/` (single non-generated
  caller). No fan-out risk; left as a subprocess call.

**Periodic but single-call, not per-session fan-out (2 sites)** — these run
on a ticker, unlike the one-shot group above, but each tick issues exactly
one subprocess call regardless of session count, so they don't have the
multiplicative blowup the health-checker bug (ADR-001's Implementation Note)
had:

- `server_registry.go:310` (`syncSessionsLocked`) — one `list-sessions` call
  per registry per resync (triggered by `reconnectLoop`/fast-recheck, not a
  fixed short interval), covering all sessions on that tmux socket at once.
- `zombie_detector.go:38` (`ScanZombies`, driven by `StartZombieWatcher`'s
  ticker) — one `ps -axo` call per tick for the whole process tree, not
  per-session.

**Generic/shared helpers (3 sites)** — `tmux.go:687`, `1050`, `2204`/`2210` —
thin wrappers that dispatch whatever args the caller supplies; not
independently classifiable, but every concrete caller found falls into one of
the two buckets above.

## Conclusion

None of the remaining `session/tmux` `safeexec` call sites are per-session,
per-tick hot paths — that pattern was the health-checker bug fixed by
`BatchPaneDeadStatus` (ADR-001's Implementation Note) and is now closed. The
rest are one-shot lifecycle calls or single-call periodic scans, both of
which have O(1) subprocess cost regardless of session count, and are left as
subprocess calls deliberately rather than migrated to control mode.

## Follow-up (not actioned now)

[tymux](https://github.com/tstapler/tymux) — a lower-level tmux control-mode
Go library — could eventually replace some of these one-shot call sites too,
but that's a larger dependency change than this project's scope. Tracked as a
future consideration, not scheduled.
