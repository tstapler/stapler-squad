# macOS Service Restart Can Leave Orphaned `stapler-squad` Processes Running, Cancelling Sessions One-by-One

`make install-service` on macOS (`scripts/install-service.sh`'s `install_macos()`) relies on `launchctl bootout` blocking until the old process fully exits before the new one starts. In practice this guarantee does not always hold: old processes can survive the restart, keep running unmanaged by launchd, and race the new process over the same tmux server and `sessions.json` state — surfacing to the user as live sessions getting cancelled one at a time as each stale process's tmux control-mode connection independently drops.

## Evidence (VERIFIED, 2026-08-05)

Four separate `stapler-squad` binaries were found running simultaneously (`ps -o pid,ppid,lstart,command`), one per day going back several days, all reparented to `PPID 1` (orphaned from launchd). `launchctl print gui/$(id -u)/com.stapler-squad` reported the service as "not running" while `lsof -nP -iTCP:8543 -sTCP:LISTEN` showed one of the orphans actually bound to the port and serving traffic — launchd had completely lost track of the real running process.

## Root-cause hypothesis (structural, INFERRED — not confirmed against logs from this exact incident)

`session/instance.go`'s `instanceOnExitCallback` (~line 780) transitions a session from `Active` to `Stopped` independently, per-session, whenever its tmux control-mode pipe reports *any* exit reason (`session/tmux/control_mode.go`). Each `Instance` owns its own `TmuxSession`, so if a stale orphaned process is still holding control-mode connections for sessions it thinks it owns, and those connections drop one at a time as the new `--tmux-keep-server` process steals the shared tmux server, each drop fires this callback independently — explaining sessions going `Stopped` one-by-one rather than all at once.

This is the leading hypothesis, not a confirmed cause — a direct log-string grep for this callback's own emitted messages found no hits in the retained log window at the time of investigation. `install_macos()`'s `bootout` call (`scripts/install-service.sh`, ~line 401) is documented to block specifically so old/new processes never race, so the underlying bug is likely in the old process's graceful-shutdown path not completing within launchd's grace period before `bootout` returns/escalates — not in `bootout` itself.

## What to check before assuming a fresh restart is clean

```bash
ps aux | command grep -i "stapler-squad" | command grep -v grep   # look for >1 process
launchctl print gui/$(id -u)/com.stapler-squad | grep -E "state|pid"
lsof -nP -iTCP:8543 -sTCP:LISTEN
```

If more than one `stapler-squad` binary is running, or `launchctl print` disagrees with what's actually bound to 8543, kill the extras manually (`kill <pid>`) before trusting session state — orphans can keep writing to the same `sessions.json` and `~/.stapler-squad/` state directory as the live instance.
