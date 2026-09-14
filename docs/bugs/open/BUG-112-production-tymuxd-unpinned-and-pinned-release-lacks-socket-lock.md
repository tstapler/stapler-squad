# BUG-112: Production's tymuxd is unpinned (PATH-resolved), and the pinned release lacks the socket lock BUG-106 fixed against [SEVERITY: High]

**Status**: 🐛 Open
**Discovered**: 2026-09-12, wiring CI to fetch a real tymuxd binary for `session/tymux/supervise_integration_test.go` (BUG-106/BUG-110's coverage): the fetched, checksum-pinned `TYMUX_VERSION` release behaved differently from the `tymuxd` this machine had on `$PATH`, which is what this session's earlier live-incident investigation and manual verification actually exercised.

## Problem Description

Two distinct, compounding gaps:

**1. The pinned `TYMUX_VERSION` (`v1.0.0`, `Makefile`) does not enforce the Unix-socket lock at
all.** Confirmed directly: ran two real `tymuxd` processes from the fetched `v1.0.0` binary
sharing one `TYMUXD_SOCKET_PATH` — both started and both bound their listener, no error. The
`/home/tstapler/.local/bin/tymuxd` binary this session had been using throughout (a newer local
build of `github.com/tstapler/tymux`, distinct checksum) *does* enforce it ("another tymuxd is
already starting against `<path>`", the exact message BUG-106's investigation was built around).
The lock is a real, newer-than-`v1.0.0` upstream behavior — it just isn't in the release this repo
currently pins.

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

Two independent decisions, likely both needed:

1. **Bump `TYMUX_VERSION`** once an upstream `tymux` release ships with the socket lock (or confirm
   with upstream whether it's already tagged and just needs picking up) — `scripts/tymuxd-checksums.txt`
   needs a new pinned entry per the file's own header instructions.
2. **Make `make install-service` (or `build`) actually embed the pinned binary** — either default
   `build`/`stapler-squad` to `build-embedded-tymux`'s behavior, or make the deploy scripts
   explicitly build with `-tags embed_tymux` and fail loudly if `session/tymux/embed/tymuxd` is
   missing, so production can never silently fall back to an unpinned `$PATH` resolution. Needs a
   design decision on backward compatibility (does this change break anyone currently relying on
   the `$PATH` fallback, e.g. a dev machine without network access to fetch the release?) — flagging
   for a decision, not deciding here.

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
