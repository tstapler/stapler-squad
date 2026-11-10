# Protect the live service from Bazel (owner-run)

Bazel runs on the same host as the live `:8543` service. Until the Bazel conversion ships its
final protection, this page gives the one thing that keeps a Bazel memory spike from killing the
service. The agents did not apply it: it restarts the service and changes how every agent
session is OOM-ranked, so you pick the moment.

## What Bazel already does

`tools/bazel` (the Bazelisk wrapper) runs Bazel in a `systemd-run --user --scope` with
`MemoryMax=14G` and sets its own `oom_score_adj` to 500, so Bazel is a likelier OOM victim than
normal processes. Check: `bash tools/bzl/wrapper_test.sh`.

## Read this before changing anything: the unit is already tuned the other way

`systemctl --user cat stapler-squad.service` (checked 2026-11-01) shows two settings:

| Source | Value | Intent |
|---|---|---|
| `~/.config/systemd/user/stapler-squad.service` | `OOMScoreAdjust=-500` | protect the server |
| `~/.config/systemd/user/stapler-squad.service.d/oom.conf` | `OOMScoreAdjust=1000` | "Claude sessions run under this unit; make them the kernel OOM-killer's first pick so the desktop survives memory pressure" |

The drop-in wins, so the running service has `oom_score_adj` 1000 (`bash scripts/check-live-oom-protect.sh`
prints `effective_oom_score_adj=1000`). Every agent session inherits it. Protecting the server
(`-900`) and keeping the sessions sacrificial are the same knob on the same unit: you cannot have
both on this unit. Decide which you want:

- Keep as is: Bazel (500) is killed before the service and its sessions only if their score
  is lower than 500, which it is not (1000). Under pressure the service and sessions die first.
- Protect the service: sessions stop being the first victims and the desktop loses that margin. This is the owner's
  choice (next section).

## Owner decision: protect the service

Decided: protect the service and continue after a child OOM kill. That is `OOMScoreAdjust=-900` plus
`OOMPolicy=continue`. `scripts/install-service.sh` now generates both in the unit it writes
(`sh scripts/install-service.sh --dry-run` prints them), but the `oom.conf` drop-in above still wins on the score until
you replace it, and the unit change only lands at the next owner-run `make install-service`. Agents applied nothing.

## Apply (owner-run)

Drop-ins apply in lexical order and later files win. `oom-protect.conf` sorts before `oom.conf`
(`-` is before `.`), so it would be silently overridden. Either edit `oom.conf`, or name the new
file to sort after it:

```bash
mkdir -p ~/.config/systemd/user/stapler-squad.service.d
cat > ~/.config/systemd/user/stapler-squad.service.d/zz-oom-protect.conf <<'CONF'
[Service]
OOMScoreAdjust=-900
OOMPolicy=continue
CONF
systemctl --user daemon-reload
systemctl --user restart stapler-squad   # tmux sessions survive: the unit runs --tmux-keep-server
bash scripts/check-live-oom-protect.sh   # exit 0 when configured and effective are <= -500
```

Unit name and restart notes: `docs/how-to/manage-systemd-service.md`.

## A negative value may not take effect for a user manager

A systemd user manager has no `CAP_SYS_RESOURCE`. Measured 2026-11-01 on this host: a transient
service started with `systemd-run --user -p OOMScoreAdjust=-900` ran with `oom_score_adj` 100
(the user manager's own score), not -900. So `zz-oom-protect.conf` may leave the effective value
at 100 even though `systemctl --user show` reports -900. The check script reads
`/proc/<pid>/oom_score_adj` for that reason and exits 3 until the effective value is
at most -500.

If the effective value stays at 100, the fallback (INFERRED, not tried) is to run the service as
a system unit with `User=tstapler`: `sudo systemctl edit stapler-squad.service` on a system-level
copy of the unit, where the system manager can set a negative score. Moving the unit is a larger
change than this page covers.

## Not done by agents

Nothing in `~/.config/systemd` was changed. The generated-unit half of Task 9.1b is in
`scripts/install-service.sh`; the drop-in above is the owner-run half.
