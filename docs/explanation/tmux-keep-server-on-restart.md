# Service Restarts No Longer Kill tmux Sessions

`make install-service` / `systemctl --user restart stapler-squad` does **not** kill the tmux server or any live session — including the one running your current Claude Code conversation. Both `scripts/install-service.sh`'s Linux systemd unit and its macOS LaunchAgent deploy with `--tmux-keep-server`, so the tmux server outlives the `stapler-squad` process restart and every session resumes rather than getting rebuilt.

This was a real, confirmed bug on Linux (see History below), fixed in #180. It is not a live risk today — this doc exists so a regression gets caught immediately if the flag is ever dropped, not to warn you off routine restarts.

## How to re-verify if you suspect regression

```bash
# Linux
grep -o -- '--tmux-keep-server' ~/.config/systemd/user/stapler-squad.service

# macOS
grep -A1 -- '--tmux-keep-server' ~/Library/LaunchAgents/*.plist
```

Either should print the flag. If it's missing, the generator in `scripts/install-service.sh` (`install_linux()`'s `ExecStart` / `install_macos()`'s `ProgramArguments`) regressed — fix it there, not by hand-editing the deployed unit, since the next `make install-service` would just overwrite a manual patch.

## History (fixed, kept for context)

The Linux systemd unit's `ExecStart` in `install_linux()` originally never included `--tmux-keep-server`, while `install_macos()`'s LaunchAgent already hardcoded it (`<string>--tmux-keep-server</string>` in its `ProgramArguments`). On Linux, every service restart tore down the tmux server along with it.

Confirmed live at the time: after running `make install-service` twice in one session, `tmux list-sessions` showed every session — including `staplersquad_stapler-squad-bklg`, the session running that very Claude Code conversation — recreated from scratch about 5 minutes after each restart timestamp. All scrollback and in-flight session state was lost; sessions were rebuilt fresh rather than resumed, and reconciliation only papered over it minutes later.

**Wrong (what `install_linux()` used to generate):**
```
ExecStart=$bin_path --remote-access$extra_flags
```

**Right (what it generates now, matching `install_macos()`):**
```
ExecStart=$bin_path --remote-access --tmux-keep-server$extra_flags
```
