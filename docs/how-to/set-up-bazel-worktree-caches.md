# Set up Bazel caches for many worktrees

How the worktrees share Bazel state, how to tune it for one machine, and how to install the daily cleanup timer.
Commands are proposals until listed in `tools/bzl/verified-commands.txt`.

## What is shared and what is not

| State | Scope | Set in |
|---|---|---|
| Disk cache `~/.cache/stapler-squad-bazel/disk` (GC cap 100 GB, 30 days) | all worktrees | `.bazelrc` |
| Repository cache `~/.cache/stapler-squad-bazel/repo` | all worktrees | `.bazelrc` |
| Output base `~/.cache/bazel/_bazel_$USER/<hash>` | one per worktree path | Bazel default |

The caches sit under `~/.cache`, outside `~/.stapler-squad`, so a cache cleanup can never touch the live service's
SQLite state. Keep them on the same filesystem as the output bases; `make bazel-doctor` check 3 warns when they differ
(hits are then copied, not reflinked).

## Personal overrides: `user.bazelrc`

`.bazelrc` ends with `try-import %workspace%/user.bazelrc`; the file is gitignored. Put per-machine flags there, for
example a lower `--local_resources=memory=` on a small host. Do not set a `startup` heap in `~/.bazelrc`: the wrapper
passes `-Xmx2g` on the command line, and `make bazel-doctor` check 7 warns about a `~/.bazelrc` heap or
`/etc/bazel.bazelrc`. Test environment flags stay in the committed `.bazelrc` on purpose so an agent cannot point
tests at live state. Read the `bazel-perf` skill's memory math before changing `--jobs` or memory limits.

## Idle shutdown

Each worktree runs its own Bazel server. The wrapper starts it with `--max_idle_secs=30` so about a hundred idle
worktrees do not each keep a JVM; `.bazelrc` sets `--max_idle_secs=300` for callers that bypass the wrapper. Shut a
server down by hand with `bazel shutdown` from inside its worktree. <!-- proposal -->

## Optional per-worktree output base

Setting `BAZEL_WORKTREE_OUTPUT_BASE=1` makes the wrapper pass an absolute `--output_base=<worktree>/.bazel-out`, so
`git worktree remove` takes the output base with it. It is the fallback when the default layout leaks disk (spike S6);
`.bazel-out` holds read-only files, so delete it only after `chmod -R u+w`. The default is the shared
`~/.cache/bazel` layout.

## Reclaim orphan output bases

Removing a worktree leaves its output base behind. Dry run first, then apply:

<!-- proposal -->
```bash
bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh
bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh --apply --tmp
```

The script never touches a base whose workspace exists, the disk cache or the repository cache. See the
`worktree-disk-cleanup` skill for the rules it follows.

## Install the daily cleanup timer (owner-run)

Agents write the unit files; you run the install because it changes user systemd state. Run it from the main
checkout, not a linked worktree (the unit follows a symlink to the checkout):

```bash
bash scripts/systemd/install-bazel-timers.sh --dry-run   # prints every command
bash scripts/systemd/install-bazel-timers.sh
systemctl --user is-enabled bazel-orphan-outputbases.timer   # prints: enabled
```

The service runs the script above with `--apply --tmp` under `nice -n 10`, then
`tools/bzl/bazel-disk-alarm.sh` (`ExecStartPost`), which prints `ALARM: <path> <du|df> <value>` and fails the unit
when `~/.cache/bazel` is above 150 GB, the disk cache above its 100 GB cap, or a cache filesystem above 85% full.
`scripts/systemd/install-bazel-timers.sh --uninstall` removes it. If the alarm trips on the disk cache, its built-in
GC is not running (it only runs while a server is idle). `tools/ci/bazel-trend.sh` prints `bazel_disk_alarm` and the
three size gauges for the weekly trend job; on a hosted runner those describe the runner, not this machine.
