# ADR-001: Claim facts ride a separate gossip channel, not a field on `AdvertisementRecord`

## Status
Accepted

## Context

This item needs to gossip a new kind of fact between hosts — "host X claimed external URL Y,
here's the deep link" — on top of the existing Workspace Host Registry gossip substrate
(`session/host_registry.go`, `session/host_advertiser.go`, documented in
`project_plans/backlog-deep-linking/decisions/ADR-002-gossip-based-host-registry.md`).
`AdvertisementRecord` (`session/host_registry.go:74-80`) already exists, is already broadcast
every `DefaultHostAdvertisementInterval` (5 min) to every known peer, and is already
Ed25519-signed/TOFU-verified — the cheapest-looking option is adding a claim list field to it.

Two of this item's own research passes reach different surface-level framings of the same
question: `research/architecture.md` §1 explicitly evaluates "field on `AdvertisementRecord`"
vs. "separate channel" and recommends separate; `research/build-vs-buy.md`'s summary table
says "extend `AdvertisementRecord`/`HostRegistry`" but that recommendation is about *not
adopting an external gossip library* (memberlist/serf/CRDT), not about the wire-format
question — its own §3 makes the same point `architecture.md` does: the correctness-sensitive
piece is bespoke LWW-by-timestamp logic, independent of which struct carries it over the wire.
`requirements.md`'s "Open questions — resolved by Phase 2 research" section already states the
resolution this ADR formalizes.

## Decision

**A `ClaimRecord` gossiped over its own endpoint (`ClaimAdvertisementEndpointPath`), stored in
its own `ClaimIndex` type (sibling to `HostRegistry`, not a field on `RegistryEntry`), not a
field bolted onto `AdvertisementRecord`/`RegistryEntry`.**

Concretely: `ClaimRecord{ExternalURL string, ClaimingHostID HostID, ItemDeepLink string,
ClaimedAt time.Time, PublicKey ed25519.PublicKey, Signature []byte}`, signed the same way
`AdvertisementRecord.signingPayload()` is (a narrow struct covering only `ExternalURL`,
`ClaimingHostID`, `ItemDeepLink`, `ClaimedAt`), broadcast over a new
`POST /internal/claim-advertisement` endpoint that reuses `HostAdvertiser`'s HTTP client/TLS
config shape (a new `ClaimGossiper` type, not a method added to `HostAdvertiser`) and the same
`isNew`-gated one-hop re-gossip discipline.

## Alternatives Rejected

**Add a `Claims []ClaimRecord` (or similar) field to `AdvertisementRecord`.** Rejected on four
independent grounds, all from `research/architecture.md` §1:
1. **Payload/interval mismatch.** `AdvertisementRecord` is retransmitted in full every 5
   minutes to every peer, forever — right for an O(1) identity payload, wrong for a claim set
   that only grows (every host would retransmit its entire accumulated claim history on every
   tick).
2. **Signing scope.** `signingPayload()` today covers exactly `{HostIdentity,
   AdvertisedAddress, AdvertisedAt}` — a deliberately narrow, rarely-changing set of fields.
   Growing it to cover an open-ended claim list means re-signing (and re-verifying) a
   growing struct on every single broadcast.
3. **Lifecycle mismatch.** A host's liveness (what `RegistryEntry`/TTL model) and a claim's
   validity (tied to the underlying GitHub issue/PR's own state, not to whether the claiming
   host is still advertising) are different clocks — see `research/pitfalls.md` §1's "a live
   host can hold a claim on a dead issue; a dead host's claim on a live issue shouldn't expire
   just because the host went quiet." Reusing `HostRegistry.Prune()`'s TTL for claims would
   conflate them.
4. **Backward compatibility / blast radius.** A field added to `AdvertisementRecord` is a wire
   format and security-surface change to a struct whose only job today is proving identity — a
   bug here risks the identity-gossip channel itself, not just the new claim feature. A
   separate endpoint fails additively (an old peer 404s or ignores it) without any risk to the
   already-battle-tested identity channel.

**Adopt an external gossip library (`hashicorp/memberlist`/`serf`) or a CRDT library for the
claim merge logic.** Rejected per `research/build-vs-buy.md` §1 and §3: these solve a
cluster-membership/UDP-transport problem this repo's handful-of-personal-machines deployment
model doesn't have, and would run a second gossip transport alongside the existing HTTP one for
zero correctness benefit — the actual correctness question (last-write-wins by `ClaimedAt`,
tie-break on `HostID`) is the same ~30-50 lines of logic regardless of transport.

## Consequences

- New persisted state: `claim_index.json`, alongside `host_registry.json`, same
  flock+mutex+atomic-write-then-rename persistence recipe.
- New HTTP surface: `POST /internal/claim-advertisement` on the same `--remote-port` server,
  sibling to `/internal/host-advertisement` (`server/auth/host_advertisement.go`).
- Two independently-evolvable gossip channels share transport/TOFU machinery but not wire
  format — a future change to claim semantics (e.g. adding a `ReleasedAt` field) never risks
  the identity-advertisement channel, and vice versa.
- Slightly more code (a second small gossiper/endpoint pair) than piggybacking would have been,
  traded for isolation — consistent with this project's Tech Debt Disposition (`plan.md`):
  Isolate via seam, not extend the existing `AdvertisementRecord`.
