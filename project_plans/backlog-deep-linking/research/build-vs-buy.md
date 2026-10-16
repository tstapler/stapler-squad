# Build vs. Buy: backlog-deep-linking

**Agent**: 6 (Build vs. Buy)
**Scope**: type-prefixed ULID IDs, `ssq://` deep-link scheme, in-app + OS-level resolution, cross-host handoff.

## Baseline check

`go.mod` currently pulls in only `github.com/google/uuid v1.6.0` (used by the ent schema's
`field.UUID("id", uuid.UUID{}).Default(uuid.New)` per `session/ent/schema/backlog_item.go:22-23`).
No ULID/KSUID/XID dependency, no URL-scheme-registration code, and no `.plist`/`.desktop` files
exist anywhere in the repo today (`grep -rli "ulid\|CFBundleURLTypes\|x-scheme-handler"` and
`find . -iname "*.plist" -o -iname "*.desktop"` both came back empty except an unrelated SSRF
helper). This is greenfield for all four sub-questions below.

## 1. ULID generation library

### `github.com/oklog/ulid/v2`

- **Pros**: The de facto reference implementation of the [ULID spec](https://github.com/ulid/spec) in Go. Apache-2.0 license (compatible with this repo). Zero non-stdlib dependencies. Actively maintained (oklog org, used by Grafana Loki, Prometheus-adjacent tooling, and widely embedded as a transitive dep across the Go ecosystem — high scrutiny). API is small and matches the exact need here: `ulid.MustNew(ulid.Timestamp(time.Now()), entropySource)` returns a 26-char Crockford-base32 string, sortable lexically, with a `Parse`/`ParseStrict` round-trip and a `MonotonicReader` for same-millisecond ordering (needed since backlog items can be created in rapid succession within a request). Encodes creation time in the ID itself, which the requirements ask for directly ("sortable, encodes creation time" — requirements.md line 42).
- **Cons**: One more dependency to vet/pin (minor — Apache-2.0, no known CVEs, last tagged release actively used in production elsewhere). ULID's 128-bit binary form doesn't map onto `uuid.UUID` 1:1 without a shim if ent's generated code expects that Go type — this is a schema-layer integration cost, not a library-quality problem (see Rabbit Holes in requirements.md).
- **Verdict**: **Recommended.** This is exactly the kind of "battle-tested, narrowly-scoped library" case — small surface area, high correctness bar (monotonic entropy, base32 alphabet edge cases), already solved well by a widely-used dependency.

### `github.com/segmentio/ksuid`

- **Pros**: Similar sortable-ID goal (27-char base62, embeds a timestamp), MIT license, mature.
- **Cons**: Different encoding (base62 vs. Crockford base32) and different timestamp epoch/resolution (second, not millisecond) than ULID — the requirements explicitly ask for ULID by name and by example format (`bl_01J...`, which is the standard ULID Crockford-base32 shape), not KSUID's shape. Adopting KSUID here would mean the "ULID" in the feature name and every worked example in requirements.md is wrong.
- **Verdict**: **Not recommended** — right idea, wrong ID format for what was actually asked for.

### `github.com/rs/xid`

- Also a sortable ID scheme (12-byte Mongo-ObjectID-style), but again not ULID-shaped (20-char base32, different structure, no monotonic-entropy guarantee tunable the way `oklog/ulid`'s `MonotonicReader` is). Same objection as ksuid: doesn't match the spec requirements.md asks for.
- **Verdict**: **Not recommended** for the same reason.

## 2. Type-prefixed ID convention (Stripe-style)

There is no general-purpose, well-maintained Go library that wraps an arbitrary ID generator with
a Stripe-style type prefix (`bl_`, `sess_`, etc.) as a first-class abstraction — this is normally a
project's own 10–20 line convention, not something worth taking a dependency for. (Searched for
`xid`/prefix-wrapper packages; the closest hits are either abandoned single-file GitHub repos with
no tests/CI, or full ID-management frameworks — e.g. Segment's internal tooling — that pull in far
more than needed for "prepend a string, validate on parse.")

- **Recommended approach**: a small custom type in the same spirit as this repo's existing
  `RepoRef`/`AccountRef` newtype pattern (`.claude/rules/primitive-obsession-checklist.md`) — a
  `BacklogItemID` (or generic `PrefixedULID`) type with unexported fields, a validating
  `NewBacklogItemID()` constructor that calls `oklog/ulid` internally, and a `String()`/`Parse()`
  pair that handles the `bl_` prefix. This is maybe 30–40 lines plus tests, well within
  hand-rolling territory, and keeps the type-safety benefits the repo's own conventions already
  value (can't be constructed from a bare `uuid.UUID` or session ID by accident).
- **Verdict**: **Build (small custom wrapper), not buy** — no viable library target exists, and the
  amount of code needed is small enough that pulling a dependency would violate the "unjustified
  generic / speculative abstraction" spirit of this repo's own checklists for a problem this size.

## 3. OS-level URL scheme registration helpers

- Searched for Go packages that wrap macOS `CFBundleURLTypes`/`Info.plist` registration or Linux
  `.desktop`/`x-scheme-handler` MIME registration. None exist with meaningful adoption — this is
  inherently OS-specific, low-level, and small in scope (write a `.plist` key or a `.desktop` file
  and call `xdg-mime`/`update-desktop-database` on Linux, `lsregister`/an app-bundle rebuild on
  macOS). The handful of GitHub hits are single-purpose CLI tools bundling this logic inline
  (e.g. app installers), not reusable libraries.
- This confirms the requirements doc's own assumption (Rabbit Holes, requirements.md line 57):
  **hand-rolled is correct here.** The harder problem flagged there — reconciling `CFBundleURLTypes`
  registration with the bare-binary + systemd/launchd deployment model — is a packaging/architecture
  question, not a library-selection one; no OSS package solves "make a systemd-launched Go binary
  behave like a `.app` bundle for URL-scheme purposes."
- **Verdict**: **Build.** Not recommended to search further for a library here; the packaging
  question flagged in requirements.md's Rabbit Holes is the real risk, not the registration
  mechanics themselves.

## 4. SaaS / managed deep-linking API (Branch.io-style)

- **Assessment**: Services like Branch.io, AppsFlyer, or Firebase Dynamic Links exist to solve
  *mobile app* deep-linking with install attribution, deferred deep linking (open the App
  Store/Play Store and still land on the right screen post-install), and cross-platform (iOS/
  Android/web) fallback logic. None of that applies here:
  - stapler-squad is a **local dev tool** running on a small number of machines a single user (or
    small team) controls directly — there is no app store, no install funnel, no attribution need.
  - The requirements explicitly scope out mobile support (requirements.md line 52).
  - The link resolution problem here is "which of *my own* known hosts owns this item" (workspace
    peers), not "route an anonymous user's browser to the mobile app they should have." A hosted
    attribution service has no visibility into `list_workspace_peers`'s private, self-managed peer
    registry and would add an external network dependency (and a data-residency/privacy question —
    backlog item hostnames going through a third-party SaaS) for a problem it isn't shaped to solve.
  - Cost/complexity: integrating one of these SaaS SDKs would be net-additive complexity for zero
    of the actual hard problems this feature has (cross-host handoff via a private peer registry,
    OS scheme registration for a systemd-launched binary).
- **Verdict**: **Not recommended.** No hosted deep-linking service fits a single/small-team
  local-dev-tool use case with a private peer registry; this is architecturally mobile-app-shaped
  tooling applied to the wrong problem.

## 5. LLM-generated ULID implementation vs. `oklog/ulid`

Hand-rolling ULID encoding/decoding means correctly implementing, from scratch:

- Crockford base32 encoding/decoding (a *non-standard* base32 alphabet — easy to transpose two
  characters, e.g. its exclusion of `I`, `L`, `O`, `U` to avoid visual ambiguity with `1`, `0`, is a
  detail an LLM-generated implementation could plausibly get subtly wrong, e.g. miscounting the
  alphabet index for a boundary byte).
- 48-bit millisecond timestamp packing into the high bits, with correct byte-boundary math (ULID's
  10-byte timestamp + 10-byte randomness split doesn't align on a byte multiple when base32-encoded
  to 26 characters — the reference implementation has to special-case the first character's mask).
- Monotonic entropy increment on same-millisecond collision (needed here — batch backlog item
  creation could plausibly generate two IDs within the same millisecond) — this is exactly the kind
  of "looks right at a glance, wrong on the edge case" logic (overflow handling when the entropy
  bytes are already at max) that this repo's own `fix-flaky-tests-dont-defer.md` rule exists to
  catch *after* it ships as a rare, hard-to-reproduce bug (a non-monotonic ID under high creation
  rate would silently break the "sortable" success metric requirements.md asks for, without ever
  throwing an error).
- Round-trip correctness (`Parse` rejecting malformed input) matters directly for security here:
  parsed IDs come from URLs that may be pasted from Slack/other tools (requirements.md's security
  classification note), so a permissive or buggy parser is an input-validation gap, not just a
  cosmetic bug.

`oklog/ulid` has already had these edge cases exercised by wide production use and is a five-minute
`go get` away. There is no correctness, licensing, or maintenance reason to hand-roll this — the
only work saved by not depending on it is trivial, and the downside (a subtly wrong sort order or
parser) directly undermines two of the four requirements success metrics (sortability, safe external
parsing).

- **Verdict**: **Recommended: adopt `oklog/ulid`, do not hand-roll.** This is a textbook "battle-tested
  library beats LLM-generated encoding logic" case — the correctness risk (base32 alphabet, byte-
  packing, monotonic overflow) is exactly the class of narrow, edge-case-heavy logic where hand-rolled
  implementations silently diverge from spec.

## 6. Fork or adapt a comparable OSS project

- Looked for comparable dev-tool deep-linking resolution logic to adapt (e.g. `code --file`,
  Raycast deep links, JetBrains Toolbox's `jetbrains://` scheme, `zed://` links). All of these are
  single-host by design (open this file/project on *this* machine) — none solve the cross-host
  handoff-via-peer-registry problem that is the actually novel part of this feature. Their OS-level
  registration mechanics (register a custom scheme against a `.app` bundle or `.desktop` file) are
  the well-trodden, already-hand-rollable part (see §3), not something worth forking a whole
  project to obtain — there's no isolable "resolution engine" component to extract from any of them;
  the scheme-registration boilerplate is a few dozen lines each and OS/toolchain-specific enough
  (different bundle identifiers, different plist structure per app) that adapting someone else's
  checked-in `Info.plist`/`.desktop` file saves little over writing this repo's own from the
  `.claude/docs/codesigning.md` pattern already in place.
- **Verdict**: **Not recommended.** No fork target exists whose core value (cross-host resolution
  via a private peer registry) overlaps with this feature; the parts that do overlap (OS scheme
  registration boilerplate) are cheap enough to write directly.

## Summary Table

| Option | Verdict |
|---|---|
| `oklog/ulid/v2` for ULID generation | **Recommended** |
| `segmentio/ksuid` | Not recommended (wrong ID shape) |
| `rs/xid` | Not recommended (wrong ID shape) |
| Type-prefixed ID wrapper library | Not recommended — build a small custom type (repo's `RepoRef`/`AccountRef` pattern) |
| OS URL-scheme registration library | Not recommended — none exist; hand-roll `.plist`/`.desktop` generation |
| Branch.io-style SaaS deep-linking | Not recommended — mobile-attribution-shaped, wrong problem entirely |
| Hand-rolled ULID encode/decode | Not recommended — real correctness risk (base32 alphabet, byte packing, monotonic overflow) vs. adopting `oklog/ulid` |
| Fork/adapt comparable OSS dev-tool deep-linking | Not recommended — no project's core value (cross-host peer handoff) overlaps enough to be worth adapting |

## Net recommendation for Phase 3 planning

Add `github.com/oklog/ulid/v2` as a new go.mod dependency (Apache-2.0, no conflicts with existing
deps). Build everything else — the type-prefix wrapper, the `ssq://` URL parsing/routing, the OS
scheme registration files, and the cross-host handoff logic — as new first-party code following
this repo's existing newtype and interface-placement conventions. No SaaS integration, no library
fork.
