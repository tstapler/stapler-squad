# Research: Build vs. Buy — Cross-Host Claim/Dedup System

Agent 6 — Build vs. Buy. Backlog item `5fba8e5f-2810-42aa-a32f-58684a768e43`.

## Existing system characterized

`session/host_advertiser.go` and `session/host_registry.go` (ADR-002) are **not** a
UDP/SWIM gossip protocol. The transport is:

- HTTP(S) push, over the same `--remote-port` HTTPS server every host already runs for
  remote access (`AdvertisementEndpointPath = "/internal/host-advertisement"`,
  `session/host_advertiser.go:24`). No new listener, no new port.
- `HostAdvertiser.Run` (`session/host_advertiser.go:75`) ticks every
  `DefaultHostAdvertisementInterval` (5 min, `session/host_registry.go:37`) and POSTs a
  signed `AdvertisementRecord` to every peer currently in `HostRegistry.Snapshot()`.
- Identity is TOFU + Ed25519 (`AdvertisementRecord.Sign`/`Verify`,
  `session/host_registry.go:99-111`); TLS is transport-only (`InsecureSkipVerify: true`,
  intentional — see the comment at `session/host_advertiser.go:53-64`), since peers are
  independently self-signed with no shared CA.
- Fan-out is bounded by construction, not a hop counter: a receiving host only calls
  `ReGossip` (`session/host_advertiser.go:144`) once per *newly learned* identity, giving
  exactly one extra hop per fact rather than a flood.
- State is a flat JSON file (`host_registry.json`) guarded by `flock`
  (`session/host_registry.go:21-31`), pruned via TTL (`DefaultHostRegistryTTL` = 3×
  the advertisement interval = 15 min, `session/host_registry.go:39-44`) — no
  quorum, no consensus, last-write-wins per host entry by construction (each host only
  ever republishes its own record).

`session/workspace_peers.go`'s `ListWorkspacePeers` is unrelated to this — it's local-DB
+ `tmux list-sessions` only, no cross-host component at all (confirmed no HTTP/gossip
code in that file).

This is the system a claim index would extend or ride alongside.

## 1. Existing OSS library — `memberlist`/`serf` or a Go CRDT library

**`hashicorp/memberlist`** (SWIM gossip, UDP+TCP transport, underlies Consul/Nomad):
- License: MPL-2.0. Not MIT/BSD, but MPL-2.0 is file-level weak copyleft — safe to import
  as an unmodified library dependency in a proprietary/personal Go project; it only
  requires that *modifications to MPL-covered files* be shared, not the importing code.
  No blocker for this repo.
- Maturity/maintenance: actively maintained, latest tagged release v0.7.0 (Jul 2026),
  repo activity as recent as Sep 21 2026 (verified via web search of pkg.go.dev/GitHub
  metadata). Battle-tested at Consul/Nomad/Serf scale.
- Fit: **poor for this repo's scale.** SWIM is built for detecting failure/membership
  churn across tens-to-thousands of nodes with probabilistic gossip fanout and
  anti-entropy. stapler-squad's hosts are a handful of personal/small-team machines that
  are mostly *always* known in advance (added once via pairing, not auto-discovered at
  scale) — the problem SWIM solves (efficient membership at scale) doesn't exist here.
- Custom wrapping required: memberlist gives you membership + a `Delegate` interface for
  piggybacking small payloads on gossip messages, but you'd still have to implement the
  claim index's own state machine (what "claiming" means, conflict resolution, delivery
  guarantees for the payload) on top of it — it does not ship a KV store. You'd also need
  a UDP-reachable transport, which is a new requirement: this repo's hosts are currently
  reachable only via the existing HTTPS `--remote-port` surface (host_advertiser.go's own
  comment names that as "the only cross-host-reachable HTTP surface in this codebase").
  Opening UDP between personal machines (often behind different NATs/firewalls, WSL2, work
  laptops) is a materially bigger deployment burden than reusing the HTTPS surface already
  proven to work.
- **Verdict: Not recommended.** Solves a scale problem this repo doesn't have, at the cost
  of a new transport dependency the deployment model doesn't already support.

**`hashicorp/serf`** (membership + gossip-based event broadcast, built on memberlist):
- Same license (MPL-2.0), still maintained (no archive/deprecation notice as of the 2026
  web search — only the standalone serf.io *docs website* was shut down in Oct 2024, the
  Go module itself is still tagged and updated).
- Same transport/scale mismatch as memberlist, plus it's a heavier layer (agent process,
  RPC, event/query semantics) than this repo needs for one lightweight index.
- **Verdict: Not recommended**, same reasoning as memberlist.

**Embedded Go CRDT libraries** (`cedricblondeau/go-lww-element-set`, `simar7/crdt`,
`cshekharsharma/go-crdt`, `DarkInno/crdt/lww`, `brunoga/deep`'s CRDT resolver):
- None have adoption or maturity remotely close to memberlist/serf — low-visibility,
  small/single-maintainer projects, several last touched only for a single Go-module
  version bump. `brunoga/deep`'s CRDT package is the most actively updated (Sep 2026) but
  is a general "deep patch" library repurposed for CRDTs, not a purpose-built claim index.
- Fit: theoretically closer to the actual problem (LWW-element-set is exactly "did host A
  or host B claim this key first"), but adopting an obscure dependency for a single map's
  worth of conflict resolution is a worse trade than writing ~30-50 lines of LWW-by-
  timestamp comparison directly against the existing `AdvertisementRecord`/
  `RegistryEntry` pattern already in `host_registry.go` (which already does exactly this
  kind of "newer `AdvertisedAt` wins" comparison for its own records).
- **Verdict: Not recommended.** The problem is small enough that the dependency-adoption
  cost (vetting, pinning, tracking upstream churn in a low-traffic project) exceeds the
  cost of bespoke LWW logic reusing an existing pattern.

## 2. SaaS/managed API

- **Shared hosted Redis/etcd** (e.g. Upstash Redis, a managed etcd): would give a trivial
  shared KV store (`SETNX issue-url -> host+item` for claim, TTL for staleness) with no
  gossip/merge logic to write at all. But it introduces a new external dependency that
  must be *always reachable* for the claim check to work — directly against this repo's
  personal/small-team, "hosts might be a laptop that's asleep or offline half the day"
  deployment model. It also adds an ongoing cost and a third-party data-residency
  consideration (backlog item titles/URLs, arguably sensitive if repos are private,
  would transit a vendor) for a system whose whole design philosophy elsewhere (TOFU
  Ed25519 identity, self-signed TLS, JSON-on-disk registries) is "no shared
  infrastructure, no accounts, no vendor." That philosophy match matters more here than
  raw convenience.
- **Verdict: Not recommended** for the general case — it's a bigger operational
  dependency than the problem justifies, and it's a poor philosophical fit with the rest
  of the host-registry design.
- **GitHub itself as the shared state store** (assignee/label): see §4 below — this is
  the SaaS option that's actually the strongest fit, because it's a service the repo
  *already* depends on for the underlying issues/PRs, not a new one.

## 3. LLM-generated bespoke logic vs. battle-tested library — for the claim-index merge logic specifically

- The actual correctness-sensitive logic needed is narrow: "given two claim records for
  the same external URL, which one wins, and how do we detect a genuine simultaneous
  claim to flag instead of silently picking one." That is a single LWW comparison
  (`ClaimedAt` timestamp, tie-break on `HostID` string for determinism) — not a general
  distributed-consensus problem. `host_registry.go` already implements exactly this shape
  of decision (`HostRegistry.Advertise`'s `isNew`/replace logic based on `AdvertisedAt`)
  and it has test coverage precedent (`fix-flaky-tests-dont-defer`, injectable `Clock`
  interface at `session/host_registry.go:46-59` already built for deterministic
  timestamp-ordering tests).
- Correctness risk of bespoke LWW-by-timestamp: **low**, provided clock skew is handled
  the same way the existing registry TTL/prune logic already tolerates it (it doesn't
  claim perfect ordering — ADR-002 explicitly tolerates "transient network blips" via
  multi-cycle TTL, not sub-second precision). A genuine same-instant double-claim race is
  explicitly **out of scope** per requirements.md ("Automatically resolving a genuine
  simultaneous-claim race") — so the bespoke logic doesn't even need to solve the hard
  part; it only needs to be no-worse than "last write observed wins, flag if two
  different non-empty claims are ever seen for the same URL."
- Adopting memberlist/serf *for this specific piece* would not reduce that risk — those
  libraries solve membership/failure-detection consistency, not application-level "who
  claimed this GitHub issue" semantics. You'd still write the exact same LWW comparison
  on top of whichever transport carries it; swapping transport doesn't touch the
  correctness question this section is about.
- **Verdict: piggyback on the existing bespoke gossip system.** Extend
  `AdvertisementRecord` (or a sibling record type re-using the same signing/verification
  and JSON-file/TTL/flock machinery) to carry a claim entry, and reuse the same
  `isNew`-style comparison `HostRegistry.Advertise` already performs. Introducing
  memberlist/serf here would mean running two independent gossip systems side by side
  (the existing HTTP one for host identity, a new UDP one for claims) for zero
  correctness benefit — pure added complexity.

## 4. Fork/adapt — GitHub assignee/label as source of truth

- **Current state:** the `github/` package only *reads* labels today
  (`client.go:75,126,367-369`, `client_graphql.go:121-146,322-333` — parsing
  `labels(first: 100)` from PR/issue queries for display and `PRInfo.Labels`). There is
  **no existing mutation path** (`AddLabel`, `AssignIssue`, or similar) anywhere in
  `github/*.go` — confirmed via grep, zero matches for `assignee`/`AddLabel`/`SetLabel`
  outside comments/read paths. Any adapt-GitHub-as-source-of-truth design has to add a
  net-new write call, which (per `.claude/rules/norawghrequest.md`) must go through
  `NewConditionalRequest`/`newGHRequestForHostWithToken`, not a raw `http.NewRequest`.
- **Coverage of the four goals:**
  - Goal 1 (GitHub-issue dedup): **strong fit.** Assigning the issue to a bot/service
    account, or applying a `claimed-by:<host>` label, on successful import makes GitHub
    itself the single always-available check — host B calls the GitHub API before
    importing and sees the assignee/label host A already set. No gossip staleness window
    at all (contrast: the existing 5-min advertisement interval / 15-min TTL means a
    gossip-based claim index has an inherent window where two hosts haven't yet heard
    about each other's claim).
  - Goal 2 (deep link back to owner): **partial.** A label can encode a host ID
    (`claimed-by:homelab-1`) but not a full `ssq://` deep link (issue labels are short,
    fixed-vocabulary strings by GitHub convention; assignee is a GitHub user, not
    arbitrary data). The actual deep link would still need to live somewhere with more
    room — an issue *comment* stamped by the claiming host (mutable-length, but a second
    write call and no structured "current owner" field GitHub exposes for querying) or
    the local claim index. So this satisfies "detect a claim exists" but not "produce the
    deep link" without an added comment-based side channel.
  - Goal 3 (PR provenance): **partial, and only for GitHub-native PRs.** Stamping the
    deep link/host identifier into the PR body or a bot comment on PR creation (as the
    requirements' own "Design directions" section suggests) does trace a PR back to its
    originating host/item — `report_pr_created`/`github_webhook_pr_fix.go`/
    `github_webhook_handler.go` could read that stamp back out. But this only tells you
    provenance for PRs that got far enough to be opened; it does nothing for the earlier
    dedup problem (goal 1) that prevents two hosts from starting duplicate *work* before
    either has opened a PR. So it's a complement to, not a substitute for, a claim
    mechanism that fires at import/pickup time.
  - Goal 4 (non-GitHub-sourced items, ItemSource-synced items): **no coverage.** A
    manually-created or NL-created backlog item, or one synced in via a non-GitHub
    `ItemSource`, has no GitHub issue to label/assign at all — there is nothing for this
    mechanism to attach to. Requirements.md's goal 4 explicitly requires the dedup system
    to "work for any backlog item source," which GitHub-native state structurally cannot
    do.
- **Verdict: Viable, but only as a *supplement* for the GitHub-issue-specific slice of
  goal 1 (and the PR-stamping half of goal 3) — not as a replacement for a host-registry-
  based claim index.** It sidesteps gossip staleness for the one case it covers (real
  strength — zero-latency, no missed-cycle window, and it's infrastructure the repo
  already depends on rather than a new system), but it cannot satisfy goals 2 (full
  deep-link payload) or 4 (non-GitHub sources) at all, and only partially satisfies goal
  3. Recommend layering it as a low-cost first check (issue assignee/label as a fast
  pre-filter before falling back to the gossip claim index) rather than treating it as
  the whole solution.

## Summary table

| Option | License | Maturity | Fit to scale | Verdict |
|---|---|---|---|---|
| `hashicorp/memberlist` | MPL-2.0 | High, active (v0.7.0, Jul 2026) | Poor — solves a scale/transport problem this repo doesn't have | Not recommended |
| `hashicorp/serf` | MPL-2.0 | High, active | Poor, same as memberlist plus heavier | Not recommended |
| Embedded Go CRDT libs | Varies (mostly permissive) | Low — small/single-maintainer | Right shape, wrong maturity | Not recommended |
| Hosted Redis/etcd SaaS | N/A (vendor) | N/A | Poor fit to always-on-vendor-free design philosophy; cost/residency concerns | Not recommended |
| GitHub assignee/label as source of truth | N/A (existing dependency) | High (GitHub API) | Strong for goal 1's GitHub slice; no coverage of goals 2 (full)/4 | Viable (supplement only) |
| Bespoke LWW extension of existing `host_registry.go`/`host_advertiser.go` | N/A (in-repo) | Existing code has test-friendly `Clock` seam already | Matches the repo's actual scale and deployment model exactly | **Recommended** |

## Bottom line

Build on the existing bespoke gossip system (extend `AdvertisementRecord`/
`HostRegistry` with a claim entry, LWW-by-timestamp with tie-break, same
signing/TTL/flock machinery already in place), optionally paired with GitHub
issue-assignee/label as a fast, zero-latency pre-check for the GitHub-import case
specifically. No external gossip library or SaaS dependency is justified at this
repo's scale — memberlist/serf solve a cluster-membership problem this repo doesn't
have, and doing so would mean running two independent gossip transports for the same
job the existing HTTP-based one already does adequately.
