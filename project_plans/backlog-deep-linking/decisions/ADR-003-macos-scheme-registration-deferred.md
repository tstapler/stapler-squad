# ADR-003: Defer macOS `ssq://` OS-level scheme registration; ship Linux `.desktop` + in-app resolution for v1

## Status
Accepted

## Context

`requirements.md` asks for `ssq://` links to be openable at the OS level (clicking a link in
Slack/a terminal should hand off to the running stapler-squad instance). `research/stack.md`
and `research/pitfalls.md` both confirm a hard blocker on macOS: `CFBundleURLTypes`
registration with Launch Services requires a real `.app` bundle with an `Info.plist`; the
current macOS deployment is a bare Mach-O binary using the `__TEXT/__info_plist` section
embedding trick, which macOS's codesigning/TCC-grant persistence relies on
(`.claude/docs/codesigning.md`) but which Launch Services does **not** treat as a registerable
bundle for URL-scheme purposes — these are confirmed-independent subsystems
(`pitfalls.md`). Building a real `.app` wrapper changes the packaging/deployment model
(`make install-service`, systemd/launchd unit paths, codesigning identity) well beyond this
feature's own scope.

Linux has no equivalent blocker: a `.desktop` file with `MimeType=x-scheme-handler/ssq;`
plus `xdg-mime default`/`update-desktop-database` registration works against the existing
bare-binary deployment as-is.

## Decision

**Ship OS-level `ssq://` scheme registration for Linux only in v1** (`.desktop` file generation
+ idempotent `xdg-mime`/`update-desktop-database` invocation via `safeexec`, run once at
`make install-service` time or on first binary start). **Defer macOS OS-level registration** to
a follow-up that first resolves the `.app`-bundle packaging question as its own piece of work —
it is out of this feature's scope to also redesign macOS packaging.

For macOS in v1, `ssq://` links remain resolvable **only** through in-app means: pasting the
link into the web UI's own resolver route, or a documented manual "open with stapler-squad" a
user can attempit via `open -a`. No OS-level double-click/browser-hand-off support on macOS in
v1. This is flagged in plan.md's Unresolved Questions and Risk Control, not silently dropped.

## Alternatives Rejected

**Build a real `.app` bundle for macOS as part of this feature.** Rejected: `pitfalls.md` and
`stack.md` both treat this as a significant, separately-scoped packaging change (new build
step, new codesigning identity considerations layered on top of the existing TCC-grant
self-signed cert setup in `.claude/docs/codesigning.md`, changes to how `make
install-service` produces and installs the binary). Folding it into this feature risks blowing
the Large appetite on packaging work orthogonal to deep-linking itself.

**Skip Linux registration too, ship in-app-only resolution on all platforms.** Rejected: Linux
registration is cheap (no blocker exists) and directly satisfies the requirement for at least
one platform; skipping a nearly-free win to keep platform symmetry isn't a good trade.

## Consequences

- macOS users get the `ssq://` link format and in-app resolution, but not OS-level
  double-click support, in v1 — a real gap relative to the original ask, explicitly called out
  rather than glossed over.
- A follow-up item (packaging: real macOS `.app` bundle + `CFBundleURLTypes` registration,
  reconciled with the existing codesigning/TCC setup) should be filed separately once this
  feature ships, not bundled in.
