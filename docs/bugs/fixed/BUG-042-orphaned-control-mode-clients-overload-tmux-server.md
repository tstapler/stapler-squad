# BUG-042: Orphaned tmux Control-Mode Clients Accumulate Across Restarts, Eventually Overload the tmux Server and Cause the UI to Oscillate Connected/Disconnected Forever [SEVERITY: High]

**Status**: ✅ FIXED (2026-07-25, commit `b6e76be7d`, merged to main)
**Discovered**: 2026-07-23, investigating a report that "the install broke" (a session reported an error) and, live, that the web UI keeps flipping between connected/disconnected without ever settling.
**Impact**: Every time the service restarts while a session has an active control-mode connection (e.g. `stapler-squad-bklg`, the always-open coordinator session), the new process spawns a fresh `tmux -C attach-session` client for that session but has no way to know about — or kill — the client the *previous* process instance spawned for the same session. `--tmux-keep-server` intentionally keeps the tmux server (and therefore that orphaned client) alive across the restart, so each cycle leaks one more client. Live, this had reached **36 duplicate `tmux -C attach-session -t staplersquad_stapler-squad-bklg` processes**, all still connected to the same tmux server (PID 3640794, up since 2026-07-19T11:14:57), plus ~25 more spread across other sessions. The accumulated clients degrade the tmux server enough that new client connections intermittently fail with tmux's own `server exited unexpectedly` message even though the server process is alive and serving existing connections. Symptom in the frontend: the terminal/session websocket connects, control mode intermittently fails against the overloaded server, the frontend treats that as a disconnect and retries, sometimes succeeding — so the UI oscillates between connected and disconnected and never stabilizes.

## Live Evidence

```
$ ps aux | grep "attach-session -t staplersquad_stapler-squad-bklg" | wc -l
36

$ tmux list-sessions          # run against the real default socket, same one the service uses
server exited unexpectedly    # intermittent — the server (PID 3640794) is provably still alive via lsof

$ lsof /tmp/tmux-1000/default | grep CONNECTED | wc -l
70+                            # far more connected clients than any healthy single-session server should have

$ cat /proc/3640794/cgroup
0::/user.slice/user-1000.slice/user@1000.service/app.slice/stapler-squad.service
$ ps -o lstart -p 3640794
Sun Jul 19 11:14:57 2026       # this tmux server instance has outlived at least one service restart already
```

Note: `journalctl --user -u stapler-squad` has **no entries at all between 2026-07-18 17:58:15 and now** — the last `user-1000.journal` file on disk stops at that timestamp (`journalctl --list-boots` shows no boot record covering this window either). So there is no log evidence for exactly how many restarts happened or what triggered them during the window this accumulated — that's a separate, host-level journald problem worth its own look, but it means this bug's restart count ("at least one, probably many") can't be pinned down precisely from logs, only inferred from the live process count.

## Proof (not just correlation)

Live process counts and an intermittent `tmux list-sessions` failure are circumstantial on their own, so this was verified two more ways before filing:

**1. Reproduced the exact symptom in the browser, live**, via `claude-in-chrome` against `http://localhost:8543/`. The terminal panes cycle `Connected` → `Disconnected · Reconnecting (attempt 1/3)...` → stuck, repeatedly. The browser console shows a real `ConnectError` mid-cycle, not just a generic drop:

```
ConnectError: [internal] tmux session missing and restore failed: failed to create tmux session 'staplersquad_stapler-squad-bklg': exit status 1
```

**2. Traced that exact string to the two lines that produce it**, which together spell out the false-negative race:

- `server/services/connectrpc_websocket.go:563-579` — before starting control mode, it calls `tmuxSession.DoesSessionExistNoCache()`; if that says "false", it assumes the session died and calls `RestoreWithWorkDir()` to recreate it, wrapping any failure as `"tmux session missing and restore failed: %w"` (line 577).
- `session/tmux/tmux.go:1871-1903` (`DoesSessionExistNoCache`) — runs `tmux list-sessions` directly; **any** failure of that command (including `server exited unexpectedly`) is treated as "session does not exist" (line 1888: `return false, nil`), not as "I don't know."
- `session/tmux/tmux.go:1070-1097` (`RestoreWithWorkDir`'s create path) — acting on that false negative, it runs `tmux new-session -d -s staplersquad_stapler-squad-bklg ...`. The session is *not actually gone* (it has 36 real clients attached), so tmux refuses with a duplicate-session error. The code already anticipated this exact race — line 1087's comment reads *"Session creation failed - but it might be because the session already exists (DoesSessionExist may have timed out and returned false incorrectly)"* — and has a fallback: invalidate the cache and re-check `DoesSessionExist()` (line 1091). But when the server is this degraded, **that recheck fails too**, so the fallback gives up and returns the exact error the browser showed (line 1096).

**3. Reproduced the underlying mechanism in an isolated, disposable tmux server** — no production process touched. Built a fresh tmux 3.6a server on a private socket (`tmux -L bugtest042`), created one session, confirmed `list-sessions` worked (exit 0). Then spawned 45 concurrent `tmux -C attach-session` clients against that one session — the same shape as the leaked processes, just compressed into one moment instead of accumulating over days:

```
$ for i in $(seq 1 45); do tmux -L bugtest042 -C attach-session -t testsess >client_$i.log 2>&1 </dev/null & done
...
$ for i in $(seq 1 45); do tail -1 client_$i.log; done | sort | uniq -c
      1 %client-session-changed client-3800683 $0 testsess
     44 %exit server exited unexpectedly
```

44 of the 45 concurrent control-mode attach attempts got `server exited unexpectedly` — the sandbox server crashed outright within about a second of the batch landing, reproducing the identical failure string independently observed against the real production socket. Sandbox torn down afterward (`kill-server`, temp files removed); nothing in the live service was touched by this step.

Taken together: many concurrent `-C attach-session` clients crash a tmux 3.6a server (step 3) → the app treats any `list-sessions` failure against a dead-but-not-yet-recovered server as "session doesn't exist" (step 2) → it tries to recreate an already-live session, collides, and the collision's own fallback check fails too because the server is still down → the exact `ConnectError` the browser shows (step 1) → frontend treats that as a drop and retries into the same race, forever.

## Root Cause

`session/tmux/control_mode.go`'s `StartControlMode()`/`StopControlMode()` refcounting (lines 58–128, 133–235) is correct *within the lifetime of one `TmuxSession` Go object* — concurrent callers on the same object properly share one underlying process via `controlModeRefCount`. But that object lives only as long as the service process does. On restart, session state is rehydrated from disk (`FromInstanceData`, see `main.go`'s "runtime" phase comment at ~line 269-274) into brand-new `TmuxSession` objects with `controlModeRefCount == 0` and `controlModeCmd == nil`. The only call site that invokes `StartControlMode()` (`server/services/connectrpc_websocket.go:581`) has no way of knowing a client already exists for that tmux session from the previous process incarnation — it just forks a new one. The old client process is never a child of the new process, was deliberately not killed by `--tmux-keep-server`'s design (see `.claude/rules/tmux-keep-server-on-restart.md`), and nothing else ever targets it for cleanup. Every restart with an actively-viewed session leaks exactly one more client.

## Suggested Fix

At startup, before any session's control mode is (re)started — right after `tmux.EnsureServerRunning("")` in `main.go`'s runtime phase — enumerate existing tmux clients (`tmux list-clients -F '#{client_tty} #{client_pid}'`, or the control-mode-flavored equivalent for non-tty `-C` clients) and kill every one of them. This is safe specifically at startup: a freshly-started process cannot have spawned any control-mode clients yet, so *any* client already attached at that moment is by definition a leftover from a prior process instance. This reconciles the leak on every boot without needing to persist client PIDs across restarts or change the refcounting model itself. Add a regression test that starts two sequential fake "server lifetimes" against the same tmux session and asserts only one control-mode client remains attached after the second one starts control mode.

## Recommended Routing

`sdd:fix-bug` — self-contained fix in the startup sequence (`main.go`) plus `session/tmux` for the client-enumeration/kill helper. Before shipping, manually verify against the live box: after the fix, a `make install-service` restart while `stapler-squad-bklg`'s UI is open should leave exactly one `tmux -C attach-session` client for that session, not two.
