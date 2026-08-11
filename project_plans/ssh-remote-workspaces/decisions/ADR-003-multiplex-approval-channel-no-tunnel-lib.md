# ADR-003: Multiplex the Approval Callback Over the Existing SSH Connection — No Reverse-Tunnel Library, No `ssh -R`

**Status**: Accepted
**Date**: 2026-08-06
**Project**: ssh-remote-workspaces

## Context

Requirement: "Approval requests raised by a remote session reach the local dashboard's
existing approval UI and the response is delivered back to unblock the remote agent process"
(requirements.md AC4). Today, `InjectHookConfig` (`server/services/session_service.go:1595`)
generates `curl` commands the agent process itself runs against
`hookBaseURLFn()` (`server/services/hook_injector.go:51`), which resolves to
`http://localhost:8543` by default. On a remote host, that `curl` hits the *remote* machine's
own loopback — nothing is listening there, so the hook silently fails
(`project_plans/ssh-remote-workspaces/research/features.md` §2c).

SSH's channel layer (RFC 4254) is explicitly multiplexed: a single authenticated `ssh.Client`
connection can carry an arbitrary number of independent logical channels. The terminal-
streaming connection (Phase 4) already requires one outbound, authenticated `*ssh.Client` per
remote session — the same connection can carry a second channel dedicated to approval-request
traffic via `direct-streamlocal@openssh.com` (the mechanism `ssh -L`/`-R` use under the hood),
with no new listening port anywhere.

## Decision

Route the approval callback over a second channel on the **same already-open `*ssh.Client`**
used for terminal streaming: the remote agent writes approval-request payloads to a Unix
domain socket on the remote host, and the local `RemoteApprovalRelay` opens a
`direct-streamlocal@openssh.com` channel (via `golang.org/x/crypto/ssh`) to read them,
delivering the payload into the existing `ExternalApprovalMonitor`
(`session/external_approval.go`). No `ssh -R` reverse port forward, no third-party tunneling
library, no new network-reachable listener.

## Alternatives Considered

- **Hand-rolled `ssh -R` reverse port forward (`client.Listen("tcp", ...)`).** Rejected as the
  default: opens a listener bound on the remote host (even if scoped to `127.0.0.1` there via
  `GatewayPorts no`), which is a larger blast radius than channel-reuse and duplicates
  functionality channel-reuse already provides. Documented as a viable fallback only if
  Unix-socket/streamlocal channel-reuse proves insufficient for the approval-callback shape in
  practice (`research/build-vs-buy.md` §4) — not adopted up front.
- **Third-party tunneling library (`jpillora/chisel`, ngrok SDK, inlets).** Rejected: all three
  solve "connect two hosts that don't already have a channel between them" — a problem this
  feature doesn't have, since the terminal-streaming SSH connection already provides that
  channel. `inlets`'s OSS version is additionally unmaintained (superseded by a commercial
  product); `chisel`'s primary real-world use case (punching through NAT/firewalls from an
  external network) is a different threat model than "two ends that already have SSH access to
  each other."
- **A second, independent SSH connection dedicated to approvals.** Rejected: doubles the
  connection-health surface this feature already has to track (Phase 6's
  `RemoteConnectionState`/`RemoteHealthProber`) for no capability gain — the existing
  connection already supports arbitrary additional channels.

## Consequences

- Zero new exposed network surface — directly satisfies the requirements' security framing
  (no new listening port, no static shared secret baked into the remote environment).
- Connection-health tracking only has one connection per remote to watch, not two; an SSH
  channel drop and a terminal-stream drop share the same underlying liveness signal.
- The remote-side agent process needs a Unix-socket (or equivalent local IPC) write path for
  approval requests, which is new design surface (Phase 5, Epic 5.1) — this is a one-time
  design cost, not an ongoing maintenance cost, per `research/build-vs-buy.md` §4.
- `hookBaseURLFn` (Phase 5, Epic 5.2) is overridden per remote session to point the injected
  `curl` hooks at the relay's local socket/endpoint instead of `localhost:8543`, so the fix is
  scoped to remote sessions only — local sessions keep today's behavior unchanged.
