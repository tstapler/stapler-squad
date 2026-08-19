# ADR-002: Workspace Host Registry uses self-announcing gossip, not a static config list

## Status
Accepted

## Context

`ssq://<hostname>/<type>/<version>/<id>` deep links need to resolve `<hostname>` to a live,
trusted stapler-squad instance when the link is opened on a *different* machine than the one
that owns the item ("cross-host handoff"). No such registry exists today:
`session/workspace_peers.go`'s `WorkspacePeer` is same-instance, session/worktree-scoped only
— it has no hostname, URL, port, or trust/auth field (confirmed in `architecture.md` Section 2
and independently in `pitfalls.md` Section 3). There is no other inter-host transport anywhere
under `session/`.

`architecture.md`'s pre-steering recommendation was a pragmatic v1: a manually-maintained list
of known hosts in `config.json`, admin-edited. This is the recommendation being overridden.

The user gave explicit steering, directly to the planning coordinator, overriding that
recommendation:

> "Each host needs to generate an identity that it can present for identifying who they are if
> we use like a gossip protocol, they can also advertise the hostnames they are listening
> on/for."

This requires: (1) each host mints and persists a durable identity, (2) hosts advertise their
own reachable hostname(s)/address(es) to each other, (3) propagation is peer-to-peer
(gossip-style), not a static admin-maintained list, and (4) this must fit inside the feature's
Large (3-6 week) appetite — it is one component of a larger feature, not a standalone
distributed-systems project.

## Decision

**Build a minimal-viable gossip-style Workspace Host Registry, scoped down from full
anti-entropy gossip to fit the Large appetite, as a new bounded context distinct from
`WorkspacePeer`.**

Two new pieces:

1. **`HostIdentity`** — generated once per stapler-squad instance on first run (a `bl_`-style
   prefixed ULID, e.g. `host_01J...`, using the same `oklog/ulid/v2` dependency as
   `BacklogItemID`), persisted in `~/.stapler-squad/host_identity.json` following the same
   `config/state.go` persistence conventions (flock-guarded JSON write) already used for
   `state.json`/`instances.json`. Immutable for the life of the install; regenerating it is a
   deliberate reset action, not something that happens on restart.

   Alongside the identity, each instance also generates an **Ed25519 keypair** on first run,
   persisted in the same file. The private key never leaves the instance; the public key is
   included in every advertisement record (see below) so peers can verify it came from the
   holder of that identity's private key.

2. **A minimal new HTTP advertisement transport on the existing `--remote-port` server, not a
   piggyback.** `session/workspace_peers.go`'s `WorkspacePeer` is same-instance,
   session/worktree-scoped state — it has no hostname, URL, port, or trust/auth field, and a
   repo-wide check found no existing inter-host `http.Client`/`net.Dial` usage in `session/*.go`
   to extend. There is no existing inter-host transport to piggyback on, so this ADR corrects
   its original premise: the advertisement exchange is a new, minimal HTTP endpoint registered
   on the process's existing `--remote-port` server (`startRemoteAccess` in `main.go:1055`,
   routes registered via `RegisterRoutes` in `server/auth/handlers.go:22`) — not a new standalone
   wire protocol or listener, and not an extension of `WorkspacePeer`/workspace-peer polling.
   Full anti-entropy gossip (failure detection, multi-hop propagation, membership convergence
   proofs — e.g. a SWIM-style protocol) remains explicitly out of scope for v1: it would consume
   most of the Large appetite on infrastructure this feature doesn't need to justify. Instead:
   - Each host periodically broadcasts a small **advertisement record**:
     `{HostIdentity, AdvertisedAddress[], AdvertisedAt, PublicKey, Signature}` —
     `AdvertisedAddress` is one or more `host:port` strings the instance believes it's
     reachable on (its own configured `--remote-port`/bind address, not the loopback
     `localhost:8543` default, since that's useless cross-host). `Signature` is an Ed25519
     signature over `{HostIdentity, AdvertisedAddress[], AdvertisedAt}` produced with the
     advertiser's private key; `PublicKey` is that instance's public key.
   - **Trust-on-first-use (TOFU) verification.** The first time a receiving host observes a
     given `HostIdentity`, it records the accompanying `PublicKey` as that identity's
     trusted key. Every subsequent advertisement claiming the same `HostIdentity` is verified
     against the recorded key and **dropped if the signature doesn't verify or the claimed
     `PublicKey` differs from the one on file** — a peer cannot impersonate a `HostIdentity`
     it has never held the private key for, closing the "compromised/misbehaving peer
     redirects a link to an attacker-controlled host" gap identified in the adversarial
     review. This does **not** protect the very first advertisement for a never-before-seen
     identity (TOFU's known limitation — there's nothing yet to verify against); that
     residual risk is accepted for v1 per the same-LAN/trusted-workspace threat model already
     stated in Consequences below, and is a natural on-ramp to key pinning or an
     out-of-band identity exchange if a stronger model is ever needed.
   - Advertisements propagate via `POST` calls to the new `/internal/host-advertisement`-style
     endpoint on each known host's `--remote-port` server — direct exchange between hosts that
     already know about each other, re-gossiped to a bounded fan-out on receipt so a link
     between any two previously-unaware hosts still converges within a few cycles. This
     satisfies the "hosts self-announce their identity and address" steering without requiring
     a wholly new listener/protocol — it reuses the port and routing already provisioned for
     remote access.
   - Each host maintains a local **Workspace Host Registry** table
     (`HostIdentity → {AdvertisedAddress[], LastSeenAt}`), persisted the same way as
     `HostIdentity`, pruned by a TTL (an entry not re-advertised within N missed cycles is
     dropped, not immediately, to tolerate transient network blips).
   - `ssq://<hostname>/...` resolution looks up `hostname` (matched against a host's
     advertised addresses, not a raw copy of the URL's hostname segment) in this local
     registry; per `pitfalls.md` Section 3, a redirect is only ever built from an entry
     **already present and current** in the local trusted registry — never synthesized
     directly from the incoming link's hostname. A bounded liveness check (short timeout) then
     distinguishes "known but currently unreachable" from "never registered" for the UX error
     states `ux.md` specifies.

The fuller gossip protocol properties intentionally **not** built in v1 — multi-hop
convergence guarantees, cryptographic peer authentication beyond "we've directly seen this
host advertise," membership failure detection beyond TTL expiry — are flagged as a stretch
goal / explicit follow-up in plan.md's Unresolved Questions, not silently dropped.

## Alternatives Rejected

**Static, admin-maintained host list in `config.json` (`architecture.md`'s original
recommendation).** Rejected per direct user steering: the whole point of the ask is hosts that
self-announce their identity and address, not a list a human must keep in sync by hand across
every machine whenever an address changes (dynamic IP, new machine, port change). A static list
also has no notion of "current/reachable" — it's either right or silently stale, whereas
gossip's periodic re-advertisement is a built-in staleness signal.

**Centralized registry service (a small dedicated server/DB all hosts register against).**
Rejected: introduces a new single point of failure and a new piece of infrastructure to deploy
and keep running, for a personal/small-team local-dev tool where the whole premise (per
`build-vs-buy.md`'s rejection of hosted deep-linking SaaS) is that the user's own machines are
the only trust boundary. It also doesn't match the user's explicit request for a
peer-to-peer/self-announcing design.

**Full SWIM-style gossip protocol with membership convergence guarantees, built from
scratch.** Rejected for v1 as disproportionate to the Large appetite: this feature needs "which
of my own machines currently owns this hostname," not general-purpose cluster membership.
Flagged explicitly as a stretch/follow-up rather than silently downgraded to a static list, per
the steering's own constraint.

## Consequences

- New persisted state: `~/.stapler-squad/host_identity.json` (one `HostIdentity` per install)
  and a Workspace Host Registry store (new file or new section of existing state, TBD at
  implementation time — see plan.md Unresolved Questions) holding advertised-peer records.
- New minimal HTTP transport: an advertisement-receiving endpoint on the existing
  `--remote-port` server (see Decision above) plus a new periodic background task that sends
  advertisements to known hosts — needs the same graceful-shutdown handling as other background
  loops in `session/`.
- Security: registry entries are only ever trusted if learned via direct or re-gossiped
  advertisement from a host this instance has previously observed — resolution never trusts an
  incoming URL's hostname claim on its own (`pitfalls.md` requirement). Advertisements are
  Ed25519-signed and verified against a TOFU-pinned public key per `HostIdentity`, so a peer
  cannot forge a later advertisement for an identity it doesn't hold the private key for.
  First-contact TOFU (accepting whichever key first claims a new identity) is an accepted
  residual risk consistent with the same-LAN/trusted-workspace threat model below, not a gap
  left undocumented.
- The v1 scope explicitly does not solve NAT traversal, cross-network discovery, or
  authentication beyond same-LAN/same-Tailscale-style-network reachability — hosts must
  already be able to reach each other's advertised address at the network layer; this registry
  only solves "which address is currently valid for which identity," not "make unreachable
  hosts reachable."
