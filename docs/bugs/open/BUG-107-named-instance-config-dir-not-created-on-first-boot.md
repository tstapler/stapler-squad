# BUG-107: A brand-new `STAPLER_SQUAD_INSTANCE`'s config directory isn't created before the first config/discovery-config save attempts it (SEVERITY: Low)

**Status**: 🐛 Open
**Discovered**: 2026-09-12, exercising a fresh named instance while investigating BUG-106.

## Problem Description

The very first boot of a never-before-used `STAPLER_SQUAD_INSTANCE=<name>` logs:

```
WARN failed to save default config err="failed to create temp config file: open
.../.stapler-squad/instances/<name>/config.json.<tmp>: no such file or directory"
WARN failed to save default discovery config err="open .../.stapler-squad/instances/<name>/discovery.json: no such file or directory"
```

twice, then self-heals (a later write succeeds once something else has created the directory).
`config.GetConfigDirForDir`'s "Priority 2: Explicit instance ID" branch returns
`filepath.Join(baseDir, "instances", instanceID)` without ever calling `os.MkdirAll` on it —
unlike its "Priority 1: Test directory override" branch, which does. Every other config-dir
consumer either creates the directory as a side effect of some other write, or runs later than
whatever does, which is why this has stayed a benign, self-healing warning rather than a hard
failure — but it's a real gap in `GetConfigDirForDir`, not a race in whatever calls it.

## Fix Approach

Mirror Priority 1's `os.MkdirAll(dir, 0750)` in the Priority 2 (explicit instance ID) branch of
`GetConfigDirForDir` (`config/config.go`). Low risk, but touches a very hot path (every
`config.LoadConfig()` call resolves this), so give it its own test run of the full `config`
package plus at least one real multi-instance manual boot before shipping, rather than folding it
into an unrelated change.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md`'s fix
(`session/tymux/daemon_config.go`'s `resolveSocketPath`) works around this same gap locally with
its own `os.MkdirAll`, since tymuxd needs its socket directory to exist synchronously and can't
rely on a later, unrelated write racing ahead of it the way the config/discovery saves above do.
