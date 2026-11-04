# First build in a fresh worktree

One journey, end to end: create a worktree, build, and know what to expect. Bazel is provisional until cutover, so
commands are proposals until listed in `tools/bzl/verified-commands.txt`.

## 1. Create the worktree and build

<!-- proposal -->
```bash
git worktree add ~/code/github.com/tstapler/stapler-squad.worktrees/<name> -b <branch> origin/main
cd ~/code/github.com/tstapler/stapler-squad.worktrees/<name>
make build
```

`make build` goes through `tools/bazel`. Each worktree gets its own Bazel output base (a hash of the path); the disk
cache and repository cache are shared, so a second worktree reuses the first one's compiled outputs.

## 2. What the first build costs

There is no recorded cold-worktree median yet: `cold_worktree_wall_s` in `tools/bzl/baseline/measured.json` is
`UNMEASURED: PENDING-QUIET-HOST` (the collector `tools/bzl/baseline/worktree-cost.sh` refuses to run above a load
average of 2.0; see `project_plans/bazel-conversion/implementation/baselines.md`). The target to hold it to is T3:
a second cold build with the shared cache at most 25% of the first. Until measured, the only observations are single
spike runs from 2026-11-01 on a loaded host (one run each, not medians): a cold build of the proto targets took 62.8 s
(837 actions) and the first `//tools/bzl/tmux:tmux_bin` build took 80 s (5294 actions); both are in
`spike-results.md`. When the wrapper detects a cold output base it prints a note that the first build takes about a
minute and later ones are cached.

## 3. When other Bazel runs hold the slots

At most two Bazel invocations run host-wide (`BAZEL_WRAPPER_SLOTS`). Yours queues and prints, once and then as a
heartbeat, a message from `tools/bzl/errors.yaml`:

<!-- errors.yaml:E7 -->
bazel: waiting for a build slot (position <p> of <n>, ETA ~<s>s, gives up after <T>s). Another Bazel run is using the host; this is not a hang.

The queue gives up after 900 s (`BAZEL_SLOT_TIMEOUT`) with exit code 75 and a message that points at
`make bazel-doctor`. Exit 76 means the wrapper detached a long build (`BAZEL_BG=1` detaches at once); poll it with
`bash tools/bzl/bg-status.sh` and never start a second build while it still prints `running`.

## 4. Where state lands

| What | Where |
|---|---|
| `bazel-bin`, `bazel-out`, `bazel-testlogs` | Symlinks in the worktree root, all gitignored |
| Output base | `~/.cache/bazel/_bazel_$USER/<hash>` (removed by `bash .claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh --apply` once the worktree is gone) |
| Disk cache, repository cache | `~/.cache/stapler-squad-bazel/disk` and `.../repo`, shared by every worktree |
| Captured wrapper logs | `.bazel-logs/` in the worktree, gitignored |

## 5. When `make sync` is needed

Only for plain `go` and gopls, which need generated code (`gen/`, `session/ent`). Bazel builds generate it itself, so
`make build` never needs a prior `make sync`.

## 6. When something looks wrong

Run `make bazel-doctor` first. It runs nine checks (version, remote cache, disk, slot holders, state locations,
generated code, memory and load, BUILD freshness, merge driver) and each failure names its remedy. The per-error
pages are in [troubleshoot-bazel](troubleshoot-bazel.md).
