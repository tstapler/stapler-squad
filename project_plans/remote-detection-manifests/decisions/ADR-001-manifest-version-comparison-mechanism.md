# ADR-001: Manifest-Content Version Comparison Mechanism

## Status
Proposed — provisional pending Phase 2 unblock (see `implementation/plan.md`'s gating
structure). This ADR records a recommended decision so a future implementer isn't starting from
zero, but is not binding until re-confirmed at the point Phase 2 actually begins (Task 2.5.1a's
sibling check for this ADR is Task 2.2.1a's "confirm ADR-001 first" note).

## Context

`research/architecture.md` §7 flags this explicitly as unresolved, not decided, by that
research pass:

> Whether the latter needs a full semver dependency or can be a plain monotonic string/integer
> compare is a build-vs-buy question for the next research pass, not resolved here; flagging it
> so the plan phase doesn't silently assume a library that isn't in `go.mod`.

Two version concepts exist and must not be conflated:

1. **Schema version** (`detector-plugins` ADR-003 §"`version` semantics, reserved now") — a
   parser-selection field (`"1"` today, a future `"2"` if the TOML schema itself changes) that
   already has a decided, accepted answer: absent or `"1"` parses as schema v1, anything else is
   rejected at load. **Not what this ADR is about.**
2. **Manifest-content version** — this ADR's subject: given a fetched manifest and a cached one,
   both valid schema v1, which one is "newer"? This is the axis `refreshRemoteManifests`
   (`implementation/plan.md` Epic 2.2) needs to enforce the Never-Downgrade Rule herdr's verified
   implementation uses (`research/pitfalls.md` §3): reject both downgrades and
   version-unchanged-but-content-changed payloads.

`go.mod` has no semver library today (`grep -n "semver\|Masterminds\|hashicorp/go-version"
go.mod` → no match, confirmed in `research/architecture.md` §7 and `research/stack.md` §2). No
atomic-write or file-caching library exists either — this repo's demonstrated convention across
every comparable feature (`research/stack.md`'s summary table) is: prefer stdlib and hand-rolled
logic over a new dependency unless the job is genuinely too complex for that, and this job (parse
"N.N.N", compare segment by segment) is not.

## Decision

**Plain dotted-numeric monotonic string comparison, hand-rolled, no new dependency.**

- Version strings are split on `.`, each segment parsed as a non-negative integer.
- Missing trailing segments are treated as zero (so `"1.2"` and `"1.2.0"` compare equal) —
  matching herdr's own verified behavior (`research/pitfalls.md` §3, fetched and cross-checked
  against `raw.githubusercontent.com/ogulcancelik/herdr/master/src/detect/manifest_update.rs`
  and GitHub issue `ogulcancelik/herdr#677`).
- A non-numeric segment is a hard parse error, not a silent zero or a string-lexical fallback —
  a malformed version string from a compromised or buggy source should fail loudly, not be
  quietly treated as "no version" or sorted unpredictably.
- Comparison returns `-1`/`0`/`1`, consumed by the Never-Downgrade Rule
  (`implementation/plan.md` Epic 2.2, `shouldAcceptManifest`): `< 0` rejects (downgrade),
  `== 0` falls through to a content-hash equality check (reject if content differs under an
  unchanged version — the "content must match version" rule), `> 0` accepts.

### Alternatives considered

| Option | Rejected because |
|---|---|
| `Masterminds/semver` (or similar full semver library) | Would be the first semver dependency in this repo, for a comparison a ~20-line dotted-integer compare already covers correctly. `research/build-vs-buy.md` §1 already rejected every general-purpose remote-config/versioning library on the same "wrong scale for the job" grounds; a semver library specifically is narrower than those but the same principle applies — full semver (pre-release tags, build metadata, `^`/`~` range operators) solves a much larger problem (package-manager-style dependency resolution) than "is A newer than B" for a two-party single-file feed. |
| HTTP `ETag`/`If-None-Match` conditional GETs as the sole freshness signal | Genuinely simpler for "did anything change" (`research/stack.md` §2 notes stdlib `net/http` supports this natively), but doesn't answer the *content* question the Never-Downgrade Rule needs — an ETag changing tells you bytes changed, not whether the new bytes represent a higher, equal, or lower semantic version. Worth adopting **in addition** as a bandwidth optimization (skip re-downloading/re-validating an unchanged ETag) at implementation time, but does not replace an explicit version field comparison. |
| Fetched-at timestamp / "newest wins" (no version field at all) | Fragile against clock skew between the publishing host and the fetching client, and gives no protection against a downgrade — a manifest with a later `fetched_at` but semantically older/broken content would win, which is exactly the failure mode the Never-Downgrade Rule exists to prevent. |

## Consequences

- No new `go.mod` dependency.
- The comparison function (`compareManifestVersion`, Task 2.2.1a) is a single, small,
  fully-unit-testable function with no external behavior to mock.
- If a future need for pre-release/build-metadata semantics emerges (unlikely for a
  single-publisher feed), this decision would need revisiting — flagged here rather than
  silently precluded, since "no library" is a decision proportional to *today's* narrow need,
  not a permanent constraint.
- Re-confirm this decision at Phase 2 unblock time (Task 2.2.1a) in case the manifest source
  chosen by ADR-002 turns out to have its own existing versioning convention (e.g. if the source
  becomes git-tag-based, tag names might already be valid semver and this decision would still
  hold, just informed by that context).
