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

## Addendum 1 (2026-08-17): wrong forwarding target and wrong production hook-injection entry
point, both corrected

Epic 5.1 (`session/sshremote/approval_relay.go`, commit `732738b33`) and Epic 5.2
(`server/services/hook_injector.go`, uncommitted at the time of this review) were built on two
incorrect assumptions, both confirmed by direct code reading during a review pass rather than by
observed failure — this addendum records the correction, not a symptom fix.

**Wrong assumption #1: `session.ExternalApprovalMonitor` was the right forwarding target.**
That subsystem (`session/external_approval.go`) exists for a completely different purpose:
regex-based approval *detection* over raw terminal text output from EXTERNAL tools (e.g. Aider),
keyed by `instance.ExternalMetadata.MuxSocketPath`. It has nothing to do with Claude Code's own
`PermissionRequest` hook mechanism, which already has its own local implementation — `server/
services/approval_handler.go`'s `ApprovalHandler.HandlePermissionRequest`, a full `net/http.
HandlerFunc` that does secret-scanning, domain-age checks, rule-based classification, and — if
none of those resolve it — queues the request for manual review and **blocks the HTTP request**
open until a human decides, then writes a `hookDecisionResponse` JSON body back as the response.
Feeding a relayed request into `ExternalApprovalMonitor.IngestRelayedApproval` (added in the Epic
5.1 commit) meant it never reached any of that logic at all — no secret scan, no classifier, no
manual-review UI wiring, nothing. `IngestRelayedApproval`/`SourceRemote` and their dedicated test
file were deleted; grep confirmed no other caller existed anywhere in the repo.

**Wrong assumption #2: `hook_injector.go`'s `InjectHooksConfig` (plural) was the production
hook-injection entry point.** It isn't. `server/services/session_service.go`'s real
`CreateSession` flow calls a different, hand-rolled function: `InjectHookConfig` (singular,
`approval_handler.go`) — confusingly similar name, genuinely separate implementation, confirmed
the two never call each other. Epic 5.2's remote-aware `RemoteHookTarget`/`WithRemoteHookTarget`
option was correctly designed but wired onto the wrong function; nothing threaded it into the
one CreateSession actually calls. Before this fix, `InjectHookConfig` ran unconditionally for
every session including remote ones, doing plain `os.ReadFile`/`os.WriteFile` against a path
that only exists on the remote host — silently installing NO hook at all for remote sessions,
with only a warning log to show for it.

**What changed:**

- `relayedApprovalPayload`'s `Request` field is now `json.RawMessage` (was `detection.
  ApprovalRequest`) — the raw, untouched bytes Claude Code wrote to the hook's stdin, passed
  through unmodified rather than decoded into the wrong type.
- `RemoteApprovalRelay` takes a `PermissionRequestHandler` (`HandlePermissionRequest(w, r)`,
  defined in `session/sshremote` — the consumer package, satisfying `.claude/rules/
  interface-pollution-checklist.md`'s "define where consumed" rule — and satisfied structurally
  by `*server/services.ApprovalHandler` with zero changes there) instead of `*session.
  ExternalApprovalMonitor`. `handleConnection` builds a synthetic `*http.Request` from the raw
  payload bytes, calls the handler (which BLOCKS until a human decision or its own timeout), and
  writes the response body back onto the SAME connection before closing it. This merges request
  and response into one blocking round trip, which is simpler than — and subsumes — what was
  separately planned as Epic 5.3 (response delivery): once the target moved from a fire-and-
  forget monitor ingest to a handler that already blocks and returns a response, there was no
  remaining reason to split request and response into two half-duplex pieces.
- `RemoteApprovalRelayTarget.SessionKey` is renamed `StableSessionID`: it's no longer an
  `ExternalApprovalMonitor` lookup key (that subsystem is no longer involved) but the literal
  value written into the synthetic request's `X-CS-Session-ID` header, which must equal
  `session.Instance.GetStableID()` for `ApprovalHandler.resolveSessionID` to correlate the
  request correctly.
- `approval_handler.go`'s local-only JSON-merge logic is extracted into `mergeHookEntryIntoSettings`
  (existing bytes + a pre-built `hookEntry` + an "already present" predicate → merged JSON),
  reused by both `InjectHookConfig` (unchanged local behavior, now delegating to the shared
  helper) and a new `InjectHookConfigRemote` (reads/writes `settings.local.json` via
  `tmux.CommandRunner.Run`/`Start` against the remote host instead of local `os` calls, routing
  the generated hook at the relay's socket via `hook_injector.go`'s existing
  `remoteApprovalHookCommand`/socat mechanism) — one implementation of the merge/repair logic,
  not two that could drift.
- `server/services/session_service.go`'s `CreateSession` now branches on `instance.IsRemote()`:
  local sessions call `InjectHookConfig` exactly as before; remote sessions call a new
  `setupRemoteApprovalHooks`, which constructs and starts a per-session `*RemoteApprovalRelay`
  (wired to the same `*ApprovalHandler` registered at `/api/hooks/permission-request`, via a new
  `SessionService.SetPermissionRequestHandler` — server.go wiring mirrors the existing
  `SetRemoteDeps` pattern) and calls `InjectHookConfigRemote` with its bearer token. The relay is
  stored on the `Instance` (`SetRemoteApprovalRelay`) and stopped during `destroyChain()` —
  `session/tmux` gained an exported `DefaultSSHClientPool()` accessor so the relay can subscribe
  to the SAME pooled `*ssh.Client` the session's terminal streaming already dialed, per this
  ADR's core decision.

**Known gap, deliberately out of scope:** a remote session's relay is an in-memory Go construct
tied to `context.Background()` and explicit `Stop()` on session teardown — it does not survive a
server process restart, unlike the remote tmux session itself (which is designed to survive
one). Reconnecting existing remote sessions' relays on server restart would need its own wiring
pass (likely alongside `loadInstancesWithWiring`) and was judged out of scope for this
correction, whose brief was specifically "wire it into `CreateSession`."

Verified: `go build ./...`, `go vet ./session/... ./server/...` clean; `go test ./session/...
./server/services/... -race -count=1` — see the accompanying implementation report for exact
output; `gofmt -l` clean on every changed file.
