# Restarting the Service Kills Every Live tmux Session

`make install-service` / `systemctl --user restart stapler-squad` kills the tmux server and every session in it — including the one running your current Claude Code conversation — unless the service is started with `--tmux-keep-server`.

**Wrong:**
```
# scripts/install-service.sh, install_linux()
ExecStart=$bin_path --remote-access$extra_flags
```

**Right:**
```
# scripts/install-service.sh, install_linux() — match install_macos()'s LaunchAgent,
# which already hardcodes this flag in its ProgramArguments
ExecStart=$bin_path --remote-access --tmux-keep-server$extra_flags
```

## Why

The Linux systemd unit's `ExecStart` in `install_linux()` never included `--tmux-keep-server`, while `install_macos()`'s LaunchAgent already hardcodes it (`<string>--tmux-keep-server</string>` in its `ProgramArguments`). On Linux, every service restart tears down the tmux server along with it.

Confirmed live: after running `make install-service` twice in one session, `tmux list-sessions` showed every session — including `staplersquad_stapler-squad-bklg`, the session running that very Claude Code conversation — recreated from scratch about 5 minutes after each restart timestamp. All scrollback and in-flight session state was lost; sessions were rebuilt fresh rather than resumed, and reconciliation only papered over it minutes later.

Before running `make install-service` (or any command that restarts the service) while sessions are active — including the one you're currently working in — confirm the deployed unit passes `--tmux-keep-server`, or expect every live session to be destroyed and silently rebuilt.
