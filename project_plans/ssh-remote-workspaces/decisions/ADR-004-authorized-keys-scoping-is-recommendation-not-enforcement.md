# ADR-004: `authorized_keys` Forced-Command Scoping Is a Documented Recommendation, Not an App-Enforced Guarantee

**Status**: Accepted
**Date**: 2026-08-06
**Project**: ssh-remote-workspaces

## Context

`research/pitfalls.md` §3 establishes the blast radius of a compromised SSH private key: "full
shell access to the remote host as that [SSH] user," full stop. The requirements' `base_path`
scoping concept is a client-side/application-level convention only — Stapler Squad choosing to
confine its *own* worktree operations under a base path — and provides **zero** enforcement
against a stolen key used directly with `ssh`/`scp` outside the app entirely.

OpenSSH supports genuine server-side scoping via `authorized_keys` line restrictions:
`command="<wrapper-script>",restrict,pty` — `restrict` (OpenSSH 7.2+) disables agent/port/X11
forwarding by default, `pty` re-enables the PTY allocation this feature needs for interactive
tmux, and the forced `command=` wrapper can constrain the key to only `tmux`/`git` operations
scoped under one directory prefix. This is the *only* mechanism that actually reduces blast
radius below "full shell as the user" — but it lives entirely in the remote host's own sshd
configuration, which Stapler Squad (a client-side Go process dialing out) cannot inspect,
enforce, or verify is correctly applied on the far end.

## Decision

Treat `authorized_keys` forced-command scoping as a **generated recommendation surfaced during
remote onboarding**, not an enforced guarantee. Stapler Squad's remote-onboarding flow (Phase 3,
Epic 3.2/3.3) generates the recommended `authorized_keys` line (including the `command=`
wrapper and `restrict,pty` flags) for the user to copy onto the remote host themselves. The app
does not — and architecturally cannot — verify that the line was actually installed, is still
present, or wasn't since modified/removed on the remote host. This gap is documented explicitly
in the UI (Settings → Remotes onboarding copy) and in this ADR, not silently assumed away.

## Alternatives Considered

- **Silently document `base_path` as sufficient scoping, without mentioning
  `authorized_keys`.** Rejected: this is the actual security gap `research/pitfalls.md`
  identifies — `base_path` alone provides no protection against a stolen key used outside the
  app. Omitting the `authorized_keys` guidance would ship a feature whose stated security
  posture (scoped access) doesn't match its actual posture (full shell access) without ever
  surfacing that gap to the user who could close it.
- **Have Stapler Squad attempt to remotely install/verify the `authorized_keys` line itself
  (e.g. via an initial privileged bootstrap SSH session).** Rejected for v1: requires a
  *separate*, more-privileged credential than the scoped key being installed (a chicken-and-egg
  problem — the app would need broader access than the key it's trying to restrict, at least
  transiently), and installing lines into a user's `authorized_keys` file programmatically is a
  meaningfully larger trust footprint than generating text for the user to review and paste
  themselves. Worth revisiting only if repeated onboarding friction from the manual step proves
  it's a real adoption blocker — not built speculatively now.
- **Skip forced-command scoping entirely and rely only on per-host, per-purpose keys (one
  keypair per remote, not shared) as the sole mitigation.** Rejected as the *only* mitigation:
  per-host keys (already planned, `research/pitfalls.md` §3) limit blast radius to "one host,"
  not "one host, scoped to one directory and one command set" — the two mitigations are
  complementary, not substitutes, and omitting the stronger one when it's known and available
  would be a deliberate downgrade with no upside.

## Consequences

- The remote-onboarding UI (Phase 6, Epic 6.1) must display the recommended `authorized_keys`
  line as generated, copyable text, with an explicit statement that Stapler Squad cannot verify
  it was applied.
- Security documentation (or the Settings → Remotes help text) must state plainly: "a
  compromised key grants full shell access to this host as `<user>` unless you've applied the
  recommended `authorized_keys` restriction yourself." This is a "secure by default, but only if
  the user follows the onboarding flow" gap — flagged, not hidden.
- No new code path attempts to modify the remote host's `authorized_keys` file; onboarding is
  copy-paste, matching the trust model of every other credential this app already surfaces to
  the user (e.g. GitHub PAT scopes) rather than one it silently manages end-to-end.
