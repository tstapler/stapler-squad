# BUG-112: Production's tymuxd is unpinned (PATH-resolved), and no tagged release has the Unix-socket feature BUG-106's fix depends on [SEVERITY: High]

**Status**: 🐛 Open (gap 2 fixed 2026-09-14; gap 1 remains, blocked on upstream)
**Discovered**: 2026-09-12, wiring CI to fetch a real tymuxd binary for `session/tymux/supervise_integration_test.go` (BUG-106/BUG-110's coverage): the fetched, checksum-pinned `TYMUX_VERSION` release behaved differently from the `tymuxd` this machine had on `$PATH`, which is what this session's earlier live-incident investigation and manual verification actually exercised. Filed upstream as `tstapler/tymux#46`.

## Problem Description

Two distinct, compounding gaps:

**1. `TYMUXD_SOCKET_PATH`/the whole Unix-socket control plane doesn't exist in any tagged
`tymux` release** — not `v1.0.0` (what this repo's `Makefile` pins), and not `v1.1.0` (the
*current latest* release, newer than what's pinned here and still confirmed to lack it).
Confirmed via `strings <binary> | grep TYMUXD_SOCKET_PATH` returning zero matches on both release
binaries, and directly at runtime: two real `tymuxd` v1.1.0 processes given the identical
`TYMUXD_SOCKET_PATH` both started and bound distinct sockets with no collision, no `uds_path` ever
appearing in either's log output, and no socket file ever created on disk — the env var is
silently ignored, not enforced-and-failing. The `/home/tstapler/.local/bin/tymuxd` binary this
session had been using throughout (a newer, unreleased build, distinct checksum from either
release) is where the feature — including the lock BUG-106's investigation was built around
("another tymuxd is already starting against `<path>`") — actually lives. Filed upstream:
`tstapler/tymux#46` (feature missing from releases), `tstapler/tymux#47` (a related robustness gap
found in the same investigation: a process holding the lock can go TCP-listener-dead while staying
alive, matching this repo's own BUG-110 incident).

This is more consequential than "the lock isn't enforced": since the *entire* feature is absent
from both releases, `DaemonConfig.SocketPath` is a complete no-op against a "by the book" build
(pinned release, embedded via `-tags embed_tymux`) — not just less safe. BUG-106's fix only does
anything today because production isn't actually running a released version at all (gap 2, below).

**2. Production doesn't use the pinned/embedded binary at all.** `TymuxdBinary()`
(`session/tymux/binary.go`, the `!embed_tymux` build tag — the *default*) returns the literal
string `"tymuxd"`, resolved via `$PATH` at spawn time (`safeexec.CommandContext(ctx, "tymuxd")`
inside `startDaemonAttempt`). `make install-service` — this repo's own documented, "ALWAYS use
this" production deployment command (`CLAUDE.md`) — builds via the plain `build`/`stapler-squad`
Make target, which has no dependency on `build-embedded-tymux`/the `embed_tymux` tag anywhere in
its chain. ADR-001's entire pinned-release-download design (`scripts/fetch-tymuxd.sh`, checksum
verification, `embed_tymux`) is real and correctly built, but nothing in the standard build/deploy
path actually opts into it — so the live deployed instance's tymux backend behavior depends
entirely on whatever `tymuxd` binary happens to be first on that specific machine's `$PATH`,
un-pinned, unverified, and able to silently drift between machines or over time (exactly how this
session ended up validating BUG-106/BUG-110 against a materially newer, un-pinned build than what
CI's own fetched release would exercise).

## Why this matters

This directly undercuts ADR-001's stated goal (a pinned, checksum-verified, reproducible tymuxd)
and is a real blocker for "safe to default the tymux backend on everywhere": today, whether the
lock-collision fix (BUG-106) or its recovery path (BUG-110) actually matters in practice depends on
an uncontrolled variable — which binary a given machine happens to have on `$PATH` — not on
anything this repo's build actually pins. A machine with an older/no `tymuxd` on `$PATH` gets
different (potentially broken, e.g. this session's own multi-hour production incident) behavior
than one that happens to have a newer build lying around.

## Fix Approach

Two independent decisions:

1. **Bump `TYMUX_VERSION`** once an upstream `tymux` release ships the Unix-socket feature
   (tracked: `tstapler/tymux#46`) — `scripts/tymuxd-checksums.txt` needs a new pinned entry per the
   file's own header instructions once one exists. **Still open.** `TYMUX_VERSION` was bumped to
   the current latest tag, `v1.1.0` (2026-09-14, `Makefile`), but that release still lacks the
   feature (confirmed via the same `strings`/two-processes-no-collision checks as `v1.0.0` — see
   `scripts/tymuxd-checksums.txt`'s v1.1.0 comment block) — this is a version bump, not this gap's
   fix, done because it was the latest available tag regardless.
2. **Make `make install-service` actually embed the pinned binary.** ✅ Fixed 2026-09-14:
   `install-service` now depends on `build-embedded-tymux` instead of plain `build`, and
   `build-embedded`/`build-embedded-tymux` gained the `ensure-tools proto-gen ent-gen
   server/web/dist` prerequisite chain plain `build` always had (a pre-existing gap, since neither
   embedded target had it before). Verified via `make build-embedded-tymux` (clean build, tmux +
   tymuxd both embedded) and the full `session/tymux` unit + `-tags integration` suites passing
   against the resulting embedded v1.1.0 binary. Production no longer falls back to an unpinned
   `$PATH` resolution — it now always deploys the checksum-verified, embedded binary. Note this
   doesn't resolve gap 1: `DaemonConfig.SocketPath` remains inert under the now-correctly-embedded
   v1.1.0 binary, exactly as before — only the deploy path was unpinned, not this feature gap.

Two adjacent upstream gaps found during the same investigation, filed but not blocking this repo's
own fix: `tstapler/tymux#48` (dead-flagged session records accumulate unbounded — 4400+ observed,
present in every version tested) and `tstapler/tymux#49` (no `--version`/`--help` flag, made
confirming which binary has which feature harder than it should be).

`session/tymux/supervise_integration_test.go`'s
`TestIntegration_EnsureDaemonRunning_SharedDefaultSocket_CollisionSurfacesLoudly` now `t.Skip()`s
with a clear message when run against a `tymuxd` that doesn't enforce the lock (rather than hard-
failing every CI run against the currently-pinned release) — see that test's own doc comment. This
is a workaround for the test suite, not a fix for either gap above.

## Related

`docs/bugs/fixed/BUG-106-tymuxd-shared-unix-socket-lock-blocks-concurrent-instances.md`,
`docs/bugs/fixed/BUG-110-stuck-alive-tymuxd-can-never-be-evicted-by-ensuredaemonrunning.md` (both
verified against the un-pinned local build; still confirmed to pass their own regression coverage
against the pinned `v1.0.0` release too, since their fixes don't depend on the lock existing — only
this gap's own "prove the old bug is real" test does), `project_plans/tymux-bundled-integration/decisions/ADR-001-prebuilt-tymuxd-binary-download.md`
(the design this gap undercuts), `.github/actions/fetch-tymuxd/action.yml` (CI's own fetch, added
alongside this discovery).
