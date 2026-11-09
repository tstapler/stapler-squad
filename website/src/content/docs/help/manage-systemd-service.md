---
title: 'systemd User Service'
description: 'The stapler-squad service is installed as a user unit, not a system unit.'
---

# systemd User Service

The stapler-squad service is installed as a **user** unit, not a system unit.

## Correct commands

```bash
systemctl --user status stapler-squad
systemctl --user restart stapler-squad
systemctl --user stop stapler-squad
journalctl --user -u stapler-squad -f
journalctl --user -u stapler-squad -n 50 --no-pager
```

## Why `systemctl` (without `--user`) fails

Running `systemctl status stapler-squad` (no `--user`) returns:
```
Unit stapler-squad.service could not be found.
```

The unit lives under `~/.config/systemd/user/`, not `/etc/systemd/system/`.

## Why `--user` may fail in a terminal

`systemctl --user` communicates over D-Bus (`DBUS_SESSION_BUS_ADDRESS`). If the terminal was opened outside the desktop session (e.g. SSH, a tmux pane started before login), the env var may be unset and you'll get:
```
Failed to get properties: Transport endpoint is not connected
```

Fix: set the variable explicitly before running the command, or use `make install-service` which handles this via the Makefile and runs in the correct context.

## Prefer `make install-service`

`make install-service` builds the web UI + Go binary and restarts the service in one step. Prefer it over manual `systemctl --user restart` to ensure the binary is up to date.

Under `BUILD_SYSTEM=bazel` it builds `--config=release` with Bazel instead and verifies the copied binary before it
stops the old unit; the order and the owner-run rule are in [build-with-bazel](build-with-bazel.md). Agents never run
either form to test a change (`sh scripts/install-service.sh --dry-run` prints the unit on Linux).

## OOM settings in the generated unit

`scripts/install-service.sh` writes `OOMScoreAdjust=-900` and `OOMPolicy=continue` into `[Service]`. `OOMPolicy=continue`
keeps the unit (and the tmux server and sessions it holds) alive when the kernel kills one child; the default stops the
whole cgroup. A negative score may be ignored by a user manager and an operator drop-in such as `oom.conf` overrides it;
see [protect-live-service-from-bazel](protect-live-service-from-bazel.md) for the check and the owner-run drop-in.
