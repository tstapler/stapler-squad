# macOS Service Restart Can Leave Orphaned `stapler-squad` Processes Running, Cancelling Sessions One-by-One

`make install-service` on macOS (`scripts/install-service.sh`'s `install_macos()`) is designed so `launchctl bootout` blocks until the old process fully exits before the new one starts (lines 400-408). In practice this guarantee does not always hold: old processes can survive the restart, keep running unmanaged by launchd, and race the new process over the same tmux server and `sessions.json` state — which surfaces to the user as live sessions getting cancelled one at a time as each stale process's tmux control-mode connection independently drops.

## Evidence (VERIFIED, captured 2026-08-05)

```
$ ps -o pid,ppid,lstart,command -p 12278,71952,121,79445
  PID  PPID STARTED                      COMMAND
  121     1 Mon Jul 27 14:02:49 2026     ./stapler-squad --tmux-keep-server
12278     1 Thu Jul 23 16:25:33 2026     ./stapler-squad --tmux-keep-server
71952     1 Wed Jul 22 16:22:39 2026     ./stapler-squad --tmux-keep-server
79445     1 Wed Aug  5 10:07:45 2026     .../stapler-squad --remote-access --tmux-keep-server --profile --profile-port 6060
```

Four separate `stapler-squad` server binaries were running simultaneously, one per day going back to Jul 22 — i.e. this has happened on **multiple separate restarts**, not once. All four show `PPID 1`: none of them are children of launchd: they were reparented to `init` once orphaned, meaning launchd lost track of them entirely.

```
$ launchctl print gui/$(id -u)/com.stapler-squad | grep -E "state|pid|last exit"
state = not running
last exit code = 0
job state = exited
```

launchd believes the service is **not running** at all, while PID 79445 is in fact alive and is the process actually bound to port 8543:

```
$ lsof -nP -iTCP:8543 -sTCP:LISTEN
COMMAND   PID     USER   ...
stapler-  79445   tstapler ...  TCP 127.0.0.1:8543 (LISTEN)
stapler-  79445   tstapler ...  TCP [::1]:8543 (LISTEN)
```

So the process actually serving the live app is untracked by launchd — a fifth, independent desync from the four-orphans finding above.

## Root-cause hypothesis (structural, INFERRED — not confirmed against this exact incident's logs)

`session/instance.go:780-824`'s `instanceOnExitCallback` transitions a session from `Active` to `Stopped` independently, per-session, whenever its tmux control-mode pipe reports *any* exit reason (`session/tmux/control_mode.go:320,484,496`). Each `Instance` owns its own `TmuxSession` and thus fires this callback independently. If a stale orphaned process (like the four above) is still holding tmux control-mode connections for sessions it thinks it owns, and those connections drop one at a time (e.g. as the orphan's own tmux server is torn down, or as the new process's `--tmux-keep-server` steals the shared tmux server out from under it), each drop fires `instanceOnExitCallback` independently on that stale process — which would explain sessions going `Stopped` one-by-one rather than all at once.

**Gap, named explicitly**: grepping `~/.stapler-squad/logs/staplersquad.log` for this callback's own emitted strings (`"unexpected exit detected via control mode"`, `"control-mode-pipe-closed"`, `"session exited unexpectedly"`, etc.) across the entire log returned zero hits. Either the log's retained window doesn't cover the actual incident (the earliest line in the log at inspection time was `2026-08-05T14:26:04`, after the suspected restart), or a different code path than the one hypothesized above is responsible. This mechanism should be treated as the leading structural hypothesis, not a confirmed cause.

Two apparent "cancel" log entries found during this investigation were **debunked** as evidence: `GetCurrentStatus: non-active result` DEBUG lines carry a `tail_snippet` field that is a raw terminal-buffer screen-scrape, not application state — both instances found were self-referential artifacts of this very investigation being typed into a terminal, not real events.

## Why `launchctl bootout` isn't actually blocking here

`install_macos()`'s comment (`scripts/install-service.sh:401-404`) states the intent explicitly: block on `bootout` so the old and new processes never race over the tmux server. The four-orphan evidence above shows that intent failing in practice — `bootout` unblocking (or timing out to a hard kill) without the old process's PID actually terminating, most likely because a `--tmux-keep-server` process's graceful-shutdown path (session/tmux cleanup, state flush) doesn't complete inside whatever grace period launchd allows before `bootout` returns/escalates. This is a hypothesis pointing at where to look next (e.g. instrument the shutdown path, or replace the trust-`bootout` pattern with an explicit post-bootout PID poll/kill loop), not a confirmed fix.

## What to check before assuming a fresh restart is clean

```bash
ps aux | command grep -i "stapler-squad" | command grep -v grep   # look for >1 process
launchctl print gui/$(id -u)/com.stapler-squad | grep -E "state|pid"
lsof -nP -iTCP:8543 -sTCP:LISTEN
```

If more than one `stapler-squad` binary is running, or `launchctl print` disagrees with what's actually bound to 8543, kill the extras manually (`kill <pid>`) before trusting session state — orphans can keep writing to the same `sessions.json` and `~/.stapler-squad/` state directory as the live instance.
