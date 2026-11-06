# Graceful tymuxd handoff (upgrade and downgrade without killing panes)

Status: decisions D1-D6 accepted by the owner on 2026-11-04. The squad-side gate is implemented
(`session/tymux/handoff.go`); the daemon side is in progress in tymux. This page is the squad-facing
summary. The full evaluation (options, tmux prior art, prototype, test plan) lives in tymux:
`project_plans/graceful-handoff/design.md` and `docs/adr/0008-graceful-daemon-handoff.md`
(carried by tymux draft PR https://github.com/tstapler/tymux/pull/85).

## Problem

Replacing the `tymuxd` process kills every pane. The daemon owns each pty master, so when it exits
the kernel sends SIGHUP to each pane's session, and a new daemon cannot adopt anything because no fd
or pid is persisted. Today `ReplaceDaemon` (`session/tymux/version.go`) therefore refuses while
sessions are live, or kills them if the operator passes `allowInterrupt`.

tmux gives no precedent: it never kills a live server, but it has no live upgrade either (a server
exit hangs up every pane; its release notes say to restart the server after an upgrade).

## Decisions (D1-D6)

| # | Decision |
|---|---|
| D1 | Mechanism is an in-place `exec` of the new daemon binary by the old daemon (same pid), with drain-only as the fallback. |
| D2 | The first rollout onto a handoff-capable daemon is lossy (the running daemon has no handoff RPC); no special bootstrap path. |
| D3 | Surviving an unplanned daemon death (SIGKILL, crash) is not a requirement, as with tmux. |
| D4 | Handoff state travels in an unlinked inherited fd, plus a 0600 `--handoff-debug-dump` flag for support. |
| D5 | No separate launchd job for the daemon. |
| D6 | New `AttachEvent.reconnect` arm in the shared client contract. |

## State-transfer protocol

1. Squad installs the target binary by write-temp-then-`rename` (never overwritten in place) at a
   path the daemon can read, then calls `PrepareHandoff{binary_path, expected_version, dry_run:true}`.
   The daemon validates the path (absolute, owned by the daemon uid, not group/world-writable,
   executable) and runs `<binary> --handoff-preflight`. A failure returns `REFUSED` and changes nothing.
2. Squad repeats the call with `dry_run:false`. The daemon freezes mutating RPCs (`UNAVAILABLE` with
   retry-after), sends attached clients `Reconnect{HANDOFF}`, and parks every pane reader between
   chunks so no byte is held outside the screen, replay buffer or kernel pty buffer.
3. The daemon writes each pane's state (screen, scrollback, replay chunks, `output_seq`, restart
   state, child pid) to an unlinked temp file, clears `FD_CLOEXEC` on one master fd per pane, both
   listeners and the socket lock file, and `execve`s the new binary with the original argv plus
   `--adopt-handoff`. The pid does not change, so the children's parent and `waitpid` exit codes survive.
4. The new image adopts the inherited fds, rebuilds each pane, and commits (`handoff_generation`
   increments in `GetServerInfo`). Clients reconnect with `resume_from_seq`; the preserved seq and
   replay buffer make that `resumed_from_buffer` rather than a screen reset.
5. Squad polls `GetServerInfo` directly (not the process-probe fallback, which would read the already
   renamed binary) until the same pid reports the target version with at least as many live sessions
   as before. Any mismatch is reported, never "fixed" by killing. A transport error from the real
   `PrepareHandoff` call is verified the same way, since the daemon execs mid-request. Squad does not
   check `handoff_generation` once the daemon exposes it; the counter advancing (same pid) is the
   proof of a handoff, so a same-version handoff (re-exec the same binary to pick up config, fd-limit
   or state changes) is supported. Until the daemon advertises the counter, a same-version or
   unknown-version handoff is refused rather than falsely verified (fail closed).

Verified by prototype (design.md section 9): the pty master survives `exec` only when CLOEXEC is
cleared, the child never sees SIGHUP, output buffered during the gap arrives gapless, and a client
connecting during the gap is accepted from the inherited listener.

## Version compatibility matrix (upgrade and downgrade)

`graceful_handoff` in `GetServerInfo.capabilities` is the only gate; squad never compares versions
for this (ADR 0003 upstream). Each image advertises a handoff schema read range `[min_read, current]`
and a write range `[min_write, current]`. A handoff is allowed iff the old image's write range
intersects the target's read range; it writes the highest schema in the intersection. Both directions
use the same rule, so a downgrade works exactly as far as the older binary can read what the newer one can still write.

| Running daemon | Target binary | Result |
|---|---|---|
| No `graceful_handoff` (all current releases, e.g. 1.13.1) | any | Lossy replace. Squad refuses while panes are live; `allowInterrupt` is the explicit operator override. This is the D2 one-time loss. |
| Handoff-capable | same version (same or new binary file) | Graceful handoff, verified by `handoff_generation` advancing; refused until the daemon exposes it. |
| Handoff-capable | newer, schemas intersect | Graceful handoff. |
| Handoff-capable | older but handoff-capable, schemas intersect | Graceful handoff (downgrade). |
| Handoff-capable | older or newer, schemas do not intersect | `REFUSED` at preflight; daemon untouched; squad never kills it while panes are live. |
| Handoff-capable | older binary that predates the feature | `REFUSED` (no `--handoff-preflight`); same as above. The downgrade then needs `allowInterrupt`. |

## Failure and rollback

| Failure | Where | Outcome |
|---|---|---|
| Bad path, preflight fails, schemas disjoint | before freeze | `REFUSED`; daemon keeps serving, no pane touched. |
| Freeze times out (default 2 s) | before exec | Unfreeze and abort; daemon keeps serving. |
| `execve` fails | at exec | Old image resumes (CLOEXEC re-armed, readers unfrozen) and reports the errno. |
| New image cannot parse state or panics before commit | after exec | Rollback exec of the retained previous binary with the same fds (one attempt, capped by an `attempt` counter). |
| New image crashes after commit | steady state | An ordinary daemon crash (D3: out of scope). |
| Pane exits during the freeze | freeze to adopt | New image reaps it (still the parent) and the restart policy applies. |

Squad's side of failure: a refused or failed handoff never falls through to `stopListener` or the
respawn path while sessions are live. `allowInterrupt` restores the old kill-and-respawn only when the
handoff provably changed nothing (refused, or unavailable in this build); after an unverified handoff
the daemon may already be the new image, so it is never killed. The rollout service reports refused
and unavailable as `FailedPrecondition` (the "interrupt anyway?" path) and unverified as `Aborted`.

Statements about the daemon in this page (schema ranges, preflight, rollback exec, the 2 s freeze)
are the tymux design (ADR 0008), not behavior this repo can check; they are tracked in the tymux PRs.

## Wire and proto compatibility rules

- Proto changes are additive only (`buf breaking` clean); no `PROTOCOL_VERSION` bump (ADR 0003).
- New: `rpc PrepareHandoff`, `ServerInfo.handoff_generation`, `ServerInfo.handoff_schema_range`,
  `ServerInfo.executable_path`, capability `graceful_handoff`, `AttachEvent.reconnect`.
- A daemon older than the feature answers `PrepareHandoff` with `UNIMPLEMENTED`; squad treats that as
  "no capability", never as an error to retry. (The real adapter must map `UNIMPLEMENTED` to
  `ErrHandoffUnavailable`; the current stub already returns it.)
- A daemon sends `Reconnect{HANDOFF}` only to clients that declared support for it in `Attach`;
  others just see the stream close, as on any restart. The Go and TS clients learn the arm first.
- The handoff state is its own versioned format, separate from the on-disk `CURRENT_SCHEMA_VERSION`;
  new fields are additive with serde defaults.

## Who does what

| Concern | tymux (daemon) | stapler-squad |
|---|---|---|
| Decide if a handoff is safe | Validate path, preflight, schema intersection, freeze handshake | Gate on `graceful_handoff`; dry run first |
| Move the panes | Freeze, export, clear CLOEXEC, `exec`, adopt, commit, rollback exec | Nothing; never signals the daemon during a handoff |
| Install the binary | Reads the path it is given | Atomic rename install; keep the previous binary until success |
| Verify | Increment `handoff_generation`, keep pid | Same pid, generation advanced (or target version when no counter), no fewer live sessions |
| Clients | Send `Reconnect{HANDOFF}`; keep seq and replay buffer | Reconnect loop treats it as not-an-exit |
| Failure policy | Roll back or keep serving | Never kill a daemon with live panes; operator override only |

## Task breakdown

Done in the linked PRs:

- First rollout: the first move onto a handoff-capable daemon is lossy (D2). The owner decided this is
  release-note-only: the first handoff-capable tymux tag's release note says all panes restart; no UI warning.
- tymux: pane adopt primitives (reader freeze, `export_handoff`, `Pane::from_adopted`, adopted
  master/child), with a real-`exec` test (`crates/tymux-core/tests/handoff_exec.rs`). The
  `graceful_handoff` capability is deliberately not advertised yet.
- squad: `ReplaceDaemon` handoff path (`session/tymux/handoff.go`): capability gate, dry run, verify
  by pid, version and live count, no kill on failure, `allowInterrupt` override. The RPC adapter
  fails closed (`ErrHandoffUnavailable`) until the tymux client pin carries `PrepareHandoff`.

Remaining, in order:

1. tymux `tymuxd`: `PrepareHandoff` RPC, mutating-RPC freeze gate, state fd, `--adopt-handoff` startup,
   listener and lock-file adoption, `ServerInfo` fields, capability, proto plus `buf breaking`.
2. tymux: `--handoff-preflight`, schema read/write ranges and the intersection rule, rollback exec
   with the `attempt` cap, `--handoff-debug-dump`, handoff metrics.
3. tymux clients (Go, TS): learn `AttachEvent.reconnect` and the `Attach` capability declaration.
4. tymux: tag a release; squad bumps `TYMUX_VERSION` and replaces the fail-closed adapter with the
   real `PrepareHandoff` call.
5. squad: content-addressed binary install (`tymuxd-<hash12>`, previous kept for rollback),
   drop `Pdeathsig` for the daemon on Linux, client-side version-skew error, metrics.
6. Both: macOS and Linux CI for the handoff integration tests, upgrade and downgrade soak loop.
7. Optional, only if D3 changes: fd keeper or per-pane shims.
