# ADR-002: Remote Source Trust and Pinning

## Status
Proposed — provisional pending Phase 2 unblock (see `implementation/plan.md`'s gating
structure and Task 2.5.1a, which re-confirms or supersedes this ADR at the point Phase 2 actually
begins).

## Context

`research/architecture.md` §6 draws a sharp line `detector-plugins`' own ADR-004 (plugin trust
boundary) did not have to consider, because ADR-004 only reasoned about **locally-authored**
files a user deliberately placed on their own disk:

> What ADR-004 does *not* cover, because local-file authorship made it moot: a
> **semantic-spoofing** threat... remote detection patterns are not just "match this regex,"
> they carry a `status` field... that downstream UI treats as an approval gate. A compromised
> or malicious remote source doesn't need a regex-engine exploit to cause harm; it only needs
> to publish a manifest where a pattern that should be tagged `needs_approval` is tagged
> `success` instead.

This is a strictly different threat than anything ADR-004's resource-consumption model covers
(RE2 linear-time matching forecloses ReDoS; that reasoning is source-independent and unchanged
by this ADR — see `architecture.md` §6, first paragraph). This ADR is scoped narrowly to: given
that *some* remote fetch will happen (if Phase 2 ever unblocks), what transport and pinning
posture is proportionate?

`research/pitfalls.md` §1 lays out five mitigation tiers in increasing cost order (TLS-only →
size/shape limits → pinned commit SHA → content hash verification → cryptographic signing) and
recommends a specific point on that scale for this project's actual risk profile: single
maintainer, no untrusted third-party publishers, no telemetry, no server fleet.

## Decision

**HTTPS-only transport (mandatory, not configurable) + pinned-commit-SHA fetch against this
project's own `raw.githubusercontent.com` content (or an equivalent zero-maintenance static
host), reusing the existing `detector-plugins` structural validator unchanged. Full
cryptographic signing is explicitly rejected as disproportionate.**

### What's adopted

1. **HTTPS-only, no plaintext fallback.** `RemoteFetcher.Fetch` (`implementation/plan.md` Task
   2.1.2b) rejects any `http://` source URL before attempting a connection. Near-zero cost, blocks
   passive MITM/on-path tampering. Table-stakes per `research/pitfalls.md` §1.
2. **Reuse the existing schema/resource-cap validator unmodified.** Fetched bytes go through the
   *exact same* `parsePluginFile`/`validatePluginFile` pipeline (`plugins.go:50,204`) local
   files already do — same `DisallowUnknownFields()` strictness (ADR-001 of `detector-plugins`),
   same ten-status-value validation, same `maxPatternsPerPlugin`/`maxRegexLength`/
   `maxPluginFileSize` caps (`detector-plugins` ADR-004). This is a reuse point, not a new
   decision — `architecture.md` §6 already establishes it.
3. **Pinned-commit-SHA fetch, not a floating branch/`latest` ref.** The default source URL
   embeds a specific 40-character commit SHA (e.g.
   `https://raw.githubusercontent.com/<owner>/<repo>/<sha>/manifest.toml`), not `.../main/...`.
   This closes the "any single `git push` — or a compromised maintainer GitHub account — changes
   detection behavior for every running instance on its next fetch" gap, at near-zero marginal
   cost since the fetch code needs *some* version identifier already (ADR-001's manifest-content
   version is a separate axis; the SHA pin is about *which bytes get fetched at all*, not about
   comparing two already-fetched candidates).
4. **No content-hash-verification side-channel, no cryptographic signing.** Both rejected — see
   below.

### What's explicitly rejected, and why

| Tier | Rejected because |
|---|---|
| Content hash verification (manifest ships alongside a `.sha256`, or a signed index carries the hash) | Needs a second trusted channel to carry the hash — either baked into the client binary (which defeats "update detection without a release," the feature's entire point) or itself fetched, which just moves the trust problem one hop without closing it. The pinned-SHA approach already gives content-addressing for free (a git commit SHA *is* a content hash of the tree, transitively) without a second channel. |
| Cryptographic signing (GPG/minisign/sigstore, public key baked into the client) | `requirements.md`'s own "Rabbit Holes" section rules this out explicitly: "Designing a full plugin marketplace / signing infrastructure — far beyond what a personal-scale tool with no telemetry needs." `research/pitfalls.md` §1 names the specific reason it's overkill *here*, not just "signing is always overkill": the pinned-SHA approach and "edit the local `.toml` by hand" (Phase 1, already re-landed) share the *identical* actual trust root — Tyler's own GitHub push access. Signing would defend against a threat (a compromised publish pipeline) that is not materially different from a threat already implicitly accepted for every other part of this repo's release process (anyone with push access to `main` can already ship anything, signed or not). Adding signing here without addressing that broader acceptance would be inconsistent, not more secure in practice — and would add real, ongoing cost (key generation, rotation, a signing step in the publish workflow, and now a key-compromise threat to plan for) for a threat model that doesn't justify it at this project's scale. |

### Residual risk this decision does not close

The **semantic-spoofing** threat itself — a structurally valid, correctly-signed-by-nobody
manifest that quietly weakens `needs_approval`/`input_required` detection — is not fully closed
by transport security or schema validation, because both operate on syntax, not semantics. No
automated check in this plan catches "this regex used to match an approval prompt and now
matches nothing." Pinning limits this to "an intentional decision by the SHA-bumping operator
(Tyler) to adopt specific new bytes," which converts the residual risk from "any future fetch
could silently change" to "a specific reviewed bump, at a specific time, chosen deliberately" —
proportionate given the trust-root equivalence argument above, but named here explicitly rather
than implied as fully solved.

## Consequences

- No new cryptographic dependency, no key-management story, no signing step in any publish
  workflow.
- Bumping the pin (adopting a new manifest version) is a manual, single-config-value operator
  action — by design; this is not a "publish and every instance updates within N minutes"
  system, it's closer to "publish, then deliberately tell instances to trust the new bytes,"
  which is the honest floor for a single-maintainer, no-fleet project per `research/pitfalls.md`
  §4's operational-burden analysis.
- `requirements.md` Open Question #2 (same-repo raw content vs. a separate data repo) is not
  resolved by this ADR — either satisfies the pinned-SHA + HTTPS-only + schema-reuse decision
  equally; that question is left for Task 2.5.1a to resolve against whatever's true at Phase 2
  unblock time.
- Re-confirm this decision at Phase 2 unblock time (Task 2.5.1a) — if the project's risk profile
  has changed (e.g. a second contributor with independent push access, or a move toward a
  genuine community-contribution model per `requirements.md`'s explicitly out-of-scope "PRs to a
  separate data repo" idea), the "shared trust root" argument against signing would need
  re-evaluation, since it depends specifically on today's single-maintainer posture.
