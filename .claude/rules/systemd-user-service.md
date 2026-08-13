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
