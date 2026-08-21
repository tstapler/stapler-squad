# Architecture Review: remote-detection-manifests
**Date**: 2026-08-06
**Verdict**: CONCERNS. Blocker resolved 2026-08-06 during `/sdd:4-validate` (see checkbox note
below) — no BLOCKER items remain open.

Scope note: Phase 1 is a verbatim re-land of an already-reviewed design (`detector-plugins`,
which has its own `architecture-review.md`) and introduces no new architecture — this review
found nothing to add there. All findings below are about Phase 2's design, evaluated as "if and
when built" per the task brief; none of them re-argue whether Phase 2 should start now (the
plan's own BLOCKED gating already covers that). ADR-001 and ADR-002 are both `Proposed`, not
`Accepted` — this is exactly the right time to raise type-level findings against them, before
they're finalized at Task 2.5.1a / re-confirmed at Task 2.2.1a.

## Constitution Violations
No constitution file found (`docs/adr/ADR-000-architecture-constitution.md` does not exist in
this repo) — skipped.

## Blockers

- [x] **RESOLVED 2026-08-06** — `plan.md`'s Domain Glossary, Task 2.2.1a, and Task 2.2.2a now
  specify `ManifestVersion` as a value type (`ParseManifestVersion`/`.Compare`) and
  `shouldAcceptManifest` returns the `AcceptDecision` sum type instead of free-form strings.
  **Epic 2.2 / ADR-001 — `ManifestVersion` is never elevated to a type; version comparison
  validates instead of parses.** `compareManifestVersion(a, b string) (int, error)` (Task
  2.2.1a) re-parses and re-validates both the cached and fetched version strings on *every*
  comparison call, and `shouldAcceptManifest(cachedVersion, cachedContent, fetchedVersion
  string, fetchedContent []byte) (accept bool, reason string, err error)` (Task 2.2.2a) passes
  two bare, adjacent `string` parameters (`cachedVersion`, `fetchedVersion`) that are trivially
  swappable at any call site with zero compiler protection — the exact "two primitives of the
  same type mixed up" smell the `type-driven-design` skill calls out. The Domain Glossary in
  `implementation/plan.md` (line 73) explicitly names `ManifestVersion` as a term-of-art
  concept, but no code in the plan ever constructs it as a type — it stays a raw `string`
  end-to-end. Concretely, this also produces a real drift already visible in the plan text
  itself: the Phase 2 Observability Plan (`implementation/plan.md` line 143-145) lists rejection
  reasons as `"stale version"|"validation failed"|"downgrade rejected"`, while Story 2.2.2's own
  acceptance criteria (lines 578-589) use `"downgrade rejected"` and `"version unchanged but
  content differs"` — two different vocabularies for what should be the same fixed set of
  outcomes, because nothing forces them to agree (both are free-form strings, not values of a
  shared enum). If implemented as specced, the log-line-grep debugging story the Observability
  Plan promises breaks on day one because the two sections don't emit the same reason strings.
  **Remediation**: introduce a `ManifestVersion` value type (Technique 2, smart constructor) —
  `func ParseManifestVersion(s string) (ManifestVersion, error)` that does the dotted-numeric
  parse-and-validate exactly once, at the point a manifest is first parsed (alongside
  `parsePluginFile`/`validatePluginFile`'s existing validation pass), not at every compare call.
  Give it a `func (v ManifestVersion) Compare(other ManifestVersion) int` method with no error
  return (construction already proved both operands are valid), eliminating the malformed-input
  error path from the hot comparison call entirely. Separately, replace `shouldAcceptManifest`'s
  `(accept bool, reason string, err error)` return with a small sum type (Technique 3) — e.g.
  an `AcceptDecision` interface with `accepted{}`, `rejectedDowngrade{}`,
  `rejectedContentMismatch{}`, `rejectedValidationFailed{err}` variants — so the Observability
  Plan's log-line mapping is an exhaustive switch over a closed set the compiler checks, instead
  of two independently-typed free-form strings that can silently diverge (as they already have).
  This should be resolved before ADR-001's Status moves from `Proposed` to `Accepted`
  (Task 2.2.1a's "confirm ADR-001 first" gate is the natural place to fix it).

## Concerns

- [ ] **Epic 2.4, Story 2.4.2 — no documented seam for injecting a fake fetcher, but Task 2.4.2d
  requires one.** Task 2.4.2a's signature is `func refreshRemoteManifests(ctx context.Context,
  remoteCacheDir string)` — no `sourceURL` parameter (needed since `RemoteFetcher.Fetch(ctx,
  sourceURL)`, Task 2.1.2c, requires one) and no fetcher parameter or interface. Yet Task 2.4.2d
  says the "exactly one goroutine" test needs to "inject a counting `RemoteFetcher` double" to
  observe fetch-call-count without a race. As specced, `RemoteFetcher` is a concrete struct
  (`type RemoteFetcher struct { client *http.Client }`, Task 2.1.2a) with no interface defined
  anywhere a consumer could substitute a double — this repo's own convention
  (`.claude/rules/interface-pollution-checklist.md`) is that interfaces belong in the *consumer*
  package, scoped to what that consumer needs, not speculative interfaces next to the
  implementation. **Remediation**: give `refreshRemoteManifests` an explicit `sourceURL string`
  parameter plus either (a) a small consumer-defined interface — `type manifestFetcher
  interface { Fetch(ctx context.Context, sourceURL string) ([]byte, error) }` — satisfied
  implicitly by `*RemoteFetcher`, with `refreshRemoteManifests` taking one as a parameter, or (b)
  a package-level `var fetchManifest = (*RemoteFetcher).Fetch`-style function variable swapped in
  tests. Either closes the gap Task 2.4.2d currently assumes exists but nothing else in the plan
  provides.

- [ ] **Epic 2.3, Task 2.3.1a — `rebuildSnapshot`'s hardcoded 2-arg signature was chosen over a
  slice/layer parameter for simplicity, but the plan's own Pattern Decisions table (line 94)
  already names this as approach (a)'s stated weakness** ("a `rebuildSnapshot` signature
  generalization touching code three call sites depend on"). This is a reasonable trade-off for
  *two* sources (built-in is handled separately via `DefaultRegistry()`, not through
  `rebuildSnapshot`'s directory args) and the plan correctly rejects a `[]Layer` parameter as
  premature (Technology validation table, line 106: "a new merge function would duplicate logic
  `MergedRegistry` already has"). Flagging only because the signature will need a third manual
  edit (plus every call site) if any future source (e.g. a workspace-shared plugin dir) is ever
  added — worth a one-line comment at the `rebuildSnapshot` declaration noting the two-dir shape
  is deliberate-for-now, not accidental, so a future contributor doesn't need to re-derive that
  reasoning before deciding whether to generalize further.

- [ ] **Epic 2.5, Story 2.5.1 — the pinned source URL stays a raw `string` validated only at
  `Fetch`-call time (the `https://` prefix check, Task 2.1.2b), not at config-load time.** A
  malformed or accidentally-`http://` source URL can sit in config/state for the entire process
  lifetime and only surface as a rejection deep inside the first background fetch attempt,
  logged as a generic fetch failure rather than a config-validation error surfaced at startup.
  **Remediation**: validate the source URL (scheme is `https`, and optionally that it matches
  the pinned-SHA URL shape ADR-002 requires) once at config load / `InitRemoteManifests`'s
  synchronous half, returning a clear config error early, rather than deferring the check to the
  first network attempt. This is a small addition to Task 2.4.1a, not a new component.

## Nitpicks

- `DetectorProvenance()`'s existing `map[string]string` uses `""` as a sentinel for "built-in"
  (inherited, unchanged design from Phase 1/`detector-plugins`). Phase 2 extends the same
  untyped-sentinel convention to a third source (remote cache path vs. local path vs. `""`) —
  pre-existing debt, not introduced by this plan, but worth noting that a small `type
  Provenance struct { Source ProvenanceSource; Path string }` (with `ProvenanceSource` as a
  built-in/remote/local enum) would remove the implicit `""`-means-built-in convention the next
  time this map is touched. Not worth a standalone story for this plan alone.
- `RemoteCacheDir()` and `RemoteFetcher` living in one file (`remote_manifests.go`) alongside
  version-compare, accept-decision, init, and background-refresh logic mirrors this repo's
  existing `plugins.go` (which already combines parse+validate+load) — consistent with the
  codebase's established per-feature-file convention, not a smell here, but if the file grows
  much past what Phase 2 already plans, consider splitting fetch/version-compare/init into
  separate files along the same seams the "Concerns" items above would create anyway
  (`manifestFetcher` interface, `ManifestVersion` type).
- Epic 2.3's documented asymmetry (a remote-directory scan failure must not block local
  detectors loading, while a local-directory scan failure still does) is a good design decision,
  explicitly reasoned and tested (Task 2.3.1a/1c) — noted here only as a positive: this is
  exactly the kind of business-rule-encoded-as-behavior the type-driven-design lens looks for,
  and the plan already got it right without needing a sum-type nudge.
