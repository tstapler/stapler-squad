# Implementation Plan: backlog-deep-linking

**Appetite:** Large (3-6 weeks) — per `requirements.md`.
**Inputs:** `requirements.md`, `research/{stack,features,architecture,pitfalls,ux,build-vs-buy}.md`, user steering (gossip-based Workspace Host Registry; additive `public_id` reconciliation — see ADR-002, ADR-001).

## Step 0.5 — Creative Alternatives Considered

| Component | Alternative A | Alternative B | Alternative C | Chosen | Why |
|---|---|---|---|---|---|
| ID storage | Additive `public_id` column (keep `id` UUID) | Change `id` field type to string newtype | Derive external ID purely at API layer from UUID (no new column) | **A** | Zero blast radius to ~40 existing call sites; matches existing string-PK precedent (`analytics_event.go`, `shell.go`); doesn't force a schema-wide migration for a requirement that only constrains the *external* ID shape. See ADR-001. |
| Cross-host registry | Static admin-maintained host list | Gossip-style self-announcing registry advertised over a new endpoint on the existing `--remote-port` HTTP server | Centralized registry service | **B** | Direct user steering. Static list requires manual upkeep and has no staleness signal; centralized service adds a new single point of failure inappropriate for a personal/small-team local tool. **Architecture review finding (2026-08-19): there is no existing inter-host polling/heartbeat transport in this codebase to piggyback on (`session/workspace_peers.go` is purely local — DB reads + local `tmux list-sessions`), so Epic 3 builds a new minimal HTTP transport rather than extending one.** See ADR-002. |
| macOS OS-level scheme registration | Build real `.app` bundle now | Defer macOS OS registration, ship Linux only + in-app resolution everywhere | Skip Linux too, ship in-app-only on both | **B** | **Investigation finding (ADR-003):** `make install-service`'s macOS path (`.claude/docs/codesigning.md` "How it works") ships a bare Mach-O binary launched by a LaunchAgent — `Info.plist` is embedded into the `__TEXT/__info_plist` section via `CGO_LDFLAGS="-sectcreate ..."`, and the doc is explicit that `Info.plist` is deliberately kept *out* of the binary's directory specifically to stop `codesign` from auto-detecting a bundle layout (which would seal this repo's 80k+ files as bundle resources and break `codesign --verify` on every untracked-file change). `CFBundleURLTypes` scheme registration, however, is a Launch Services function (`lsregister`) that only discovers URL-scheme handlers by scanning real `.app` bundles on disk — a Mach-O binary with an embedded `__info_plist` section but no bundle directory structure is invisible to it. So the two are in direct tension: making this binary registrable would require exactly the bundle layout `codesign`'s working setup deliberately avoids, which is a packaging-model change, not a deep-linking task, and one `.claude/docs/bundling-tmux.md`'s single-binary-embedded-tmux direction argues against reversing. That justifies deferring macOS registration as a real, appetite-driven scope cut rather than an assumed one. **Flag: this scope cut contradicts requirements.md's Success Metrics line ("clickable ... on macOS and Linux dev machines") and In-Scope bullet naming macOS registration explicitly — it should be confirmed with the user before this plan is treated as final** (see Unresolved Questions #3). Linux registration (plain `.desktop` file, no bundle requirement) remains a near-free win. |
| Link resolution surface | Server-side resolve route (`/api/deep-link/resolve`) | Client-side-only resolution in React router | Both, with server as source of truth | **A, exposed to C over time** | Server-side keeps resolution logic (registry lookup, liveness check) out of the browser bundle and reusable by the `--open-url` CLI subcommand; client calls the same endpoint rather than duplicating registry-lookup logic. |

## Domain Glossary

| Term | Definition |
|---|---|
| `BacklogItemID` | New newtype (unexported fields, `NewBacklogItemID()`/`ParseBacklogItemID()` constructors) wrapping an `oklog/ulid/v2`-generated ULID with a `bl_` prefix (`bl_01J...`). Distinct from the entity's internal `uuid.UUID` primary key. |
| `public_id` | The new additive, unique-indexed string column on `BacklogItem` storing the `BacklogItemID`'s string form. The externally-shareable identifier; the internal `id` (UUID) is unchanged. See ADR-001. |
| `HostIdentity` | A durable, immutable-per-install identifier (`host_01J...`) a stapler-squad instance mints on first run and persists in `~/.stapler-squad/host_identity.json`. Presented to peers as proof of "which install this is," independent of network address. |
| `AdvertisedAddress` | One or more `host:port` strings a `HostIdentity` claims to be reachable on (its bind/`--remote-port` address, never `localhost`). Advertised, not statically configured. |
| Workspace Host Registry | The local, per-instance table mapping `HostIdentity → {AdvertisedAddress[], LastSeenAt}`, built and kept current via gossip-style advertisement exchange. The trust boundary `ssq://` cross-host resolution consults — never resolves a hostname it hasn't itself observed advertised. Distinct bounded context from `WorkspacePeer` (session/workspace_peers.go), which is same-instance-only. |
| Advertisement record | The gossiped payload `{HostIdentity, AdvertisedAddress[], AdvertisedAt}` a host periodically broadcasts and re-propagates on receipt (bounded fan-out) to converge the registry across hosts that haven't directly met. |
| Registry TTL | The number of missed advertisement cycles after which a Workspace Host Registry entry is pruned as stale, distinguishing "known but currently unreachable" from "never registered." |
| `ssq://` URL | The deep-link scheme: `ssq://<hostname>/<type>/<version>/<id>` — `<hostname>` resolved against the Workspace Host Registry, `<id>` a type-prefixed ID such as `bl_...`. |
| Deep-link resolver | The server-side route (`GET /api/deep-link/resolve`) that parses an `ssq://` (or its `https://<host>:<port>/resolve?...` in-app equivalent) URL, resolves `<hostname>` via the registry, and either serves the local item or returns a redirect/handoff payload naming the resolved `AdvertisedAddress`. |
| `--open-url` subcommand | A new Cobra subcommand (`stapler-squad --open-url ssq://...`) that translates a scheme URL into a local `http://localhost:8543/...` navigation and shells to the OS opener (`open`/`xdg-open`) — invoked by the OS when a registered scheme handler is triggered, not by end users directly. |
| Dual-ID lookup | The storage-layer contract change: any lookup-by-ID path (`session/storage_backlog.go`, `session/ent_repository_backlog.go`) must accept either the legacy UUID `id` or the new `public_id` string, so pre-existing `?item=<uuid>` links keep working indefinitely per requirements.md. |
| Version-mismatch link | A `ssq://` link using a URL/ID shape newer than the binary resolving it understands (per `ux.md`'s edge case table) — must fail with a clear "update stapler-squad" message, not a silent misparse. |

## Pattern Decisions

| Decision | Pattern/Approach | Alternative Rejected | Reason |
|---|---|---|---|
| ID storage reconciliation | Additive `public_id` string column, `id` UUID untouched (ADR-001) | Change `id` field type to `field.String` + newtype | ~40 call-site migration cost vs. zero-blast-radius additive column; 2/3 research docs + existing schema precedent agree |
| Cross-host registry | Gossip-style self-announcing registry, advertised via a **new** endpoint on the existing `--remote-port` HTTP server (ADR-002) | Static admin-maintained host list; centralized registry service; piggybacking on an existing peer-polling loop (no such cross-host loop exists — see ADR-002 Decision) | Direct user steering toward self-announcing hosts; avoids new SPOF; static list has no staleness signal; `--remote-port` (wired in `main.go:1055` `startRemoteAccess`, routes registered via `server/auth/handlers.go:22` `RegisterRoutes`) is the one genuinely cross-host-reachable HTTP surface already in this codebase, so the new endpoint rides on an existing *server*, not an existing *polling loop* |
| macOS OS-level registration | Deferred to follow-up; Linux `.desktop` only in v1 (ADR-003) | Build real `.app` bundle now | `install-service`'s macOS binary is deliberately non-bundle (embedded `__info_plist` Mach-O section, per `.claude/docs/codesigning.md`) specifically to avoid `codesign` bundle-sealing failures; `CFBundleURLTypes` registration needs a real `.app` bundle Launch Services can scan, so building one is a packaging-model change, not a deep-linking task. **Contradicts requirements.md's Success Metrics/In-Scope macOS wording — flagged for user confirmation, not a unilateral rewrite of the requirement.** |
| `BacklogItemID` type shape | Newtype with unexported fields + validating constructor, following `RepoRef`/`AccountRef` (`.claude/rules/primitive-obsession-checklist.md`) | Bare prefixed string with no type wrapper | Prevents accidental construction from a raw UUID/session-ID string; matches this repo's established convention |
| `HostIdentity`/registry persistence | New JSON file under `~/.stapler-squad/`, flock-guarded write, following `config/state.go`'s existing pattern | New SQLite table / new ent schema | Registry data is small, instance-local, and doesn't need relational queries — matches `state.json`/`instances.json` precedent rather than adding ORM surface for a few KB of data |
| ULID generation | `github.com/oklog/ulid/v2` (new dependency) | Hand-rolled Crockford base32 + monotonic entropy; `ksuid`/`xid` | Reference implementation avoids real correctness risk in base32 alphabet/byte-packing/monotonic overflow (`build-vs-buy.md` §1, §5); ksuid/xid are the wrong ID shape |
| Interface placement for registry consumers | Deep-link resolver defines a narrow `HostResolver` interface it consumes; the registry package doesn't declare "implements" anything | Registry package exports a broad `Registry` interface next to its implementation | Interfaces belong in the consumer package per `.claude/rules/interface-pollution-checklist.md` |
| Copy-link UI | Upgrade existing `BacklogItemDetail.tsx:1257-1277` buttons in place (new URL value, dynamic `aria-label`/`aria-live`) | Build new UI component | Feature already exists and matches the recommended UX pattern (`ux.md`); this is an upgrade, not new UI |

## Migration Plan

1. **Additive ent migration**: add `public_id` (`field.String`, `Optional()` at the schema level to allow a phased rollout, `Unique()`, indexed) to `session/ent/schema/backlog_item.go`. Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` per this repo's required generate command, then `go build ./...`.
2. **Backfill task**: a one-time (idempotent, safe to re-run) startup task that finds `BacklogItem` rows with no `public_id` set — using whichever predicate Story 1.2 task 3 verifies against the generated ent code (`public_id.IsNil()` if `.Nillable()`/NULL-backed, or `public_id == ""` only if verified to round-trip that way) — and populates them with a freshly minted `BacklogItemID`. Runs on server start behind a check that skips work when no such rows exist (cheap no-op on every start after the first).
3. **Enforce going forward**: once backfill is confirmed complete in an environment, a follow-up (not in this feature's v1 scope, flagged in Unresolved Questions) can tighten `public_id` to `NotEmpty()` at the schema level. v1 ships with it optional-but-always-populated-by-application-code, since ent schema constraint tightening is itself a migration event best done separately once backfill is observed to have run.
4. **No migration required** for `id` (UUID) — untouched per ADR-001.
5. **Host identity / registry** are new files, not migrations — no existing state touched.

## Observability Plan

- **Structured log events** (existing `log` package conventions): `host_identity.generated` (once, first run), `host_advertisement.sent`/`host_advertisement.received` (debug level, high frequency — must not spam at info), `host_registry.entry_expired` (info, on TTL prune), `deep_link.resolved`/`deep_link.resolve_failed` (info/warn, with reason: not-registered / unreachable / malformed / version-mismatch / item-not-found).
- **Metrics** (if an existing metrics pipeline is present under `server/` — verify at implementation time): counters for `deep_link_resolve_total{result=...}` and `host_registry_size` gauge, to catch a registry that never converges (stuck at 1) or a resolve-failure spike after a release.
- **User-visible observability**: the Workspace Host Registry's current contents should be inspectable (a debug/settings panel or CLI flag, e.g. `stapler-squad --list-known-hosts`) so a user can diagnose "why didn't my link resolve" without reading logs — this directly serves the security requirement that resolution is auditable, not a black box.

## Risk Control

| Risk | Mitigation |
|---|---|
| Gossip registry never converges / silently stuck | Registry-size metric + `--list-known-hosts` inspection command; TTL pruning tested with a fake clock, not wall-clock sleeps |
| Redirect built from untrusted hostname (spoofing) | Resolution only ever reads from the local registry that was itself populated by observed advertisements — never parses trust from the incoming URL itself (ADR-002, pitfalls.md) |
| ent codegen staleness after additive schema change | Explicit `make ent-gen && go build ./...` verification step in the relevant task, not reliance on the stamp file alone |
| Backfill task run concurrently by multiple instances sharing state dir | Guard with the same `flock`-based approach `config/state.go` already uses for other state files |
| macOS users get a degraded experience (no OS-level open) | Explicitly documented in ADR-003 and this plan's Unresolved Questions, not silently dropped; in-app resolution still fully functional |
| Old `?item=<uuid>` links break | Dual-ID lookup contract (Domain Glossary) is a hard acceptance criterion on the storage-layer stories below, tested explicitly |
| Full gossip/anti-entropy scope creep | v1 scope frozen to advertisement + bounded re-gossip + TTL prune only; fuller SWIM-style properties explicitly deferred (ADR-002, Unresolved Questions) |

## Unresolved Questions

1. ~~**Existing peer-transport reuse**~~ **Resolved (architecture review, 2026-08-19):** confirmed there is no existing inter-host polling/heartbeat mechanism in `session/` to piggyback on — `session/workspace_peers.go`'s `WorkspacePeer`/`ListWorkspacePeers` is purely local (reads this instance's own DB, cross-checks liveness via local `tmux list-sessions`/`show-environment`; no `net.Dial`/`net/http` client to another host anywhere under `session/`). Epic 3 builds a new minimal HTTP-based advertisement endpoint on the existing `--remote-port` server (`main.go:1055` `startRemoteAccess`, `server/auth/handlers.go:22` `RegisterRoutes`) instead. See Story 3.2.
2. **Fuller gossip protocol** (multi-hop convergence guarantees, cryptographic peer auth, failure detection beyond TTL) is out of v1 scope — filed as a follow-up feature once real-world convergence behavior is observed, per ADR-002.
3. **macOS OS-level scheme registration** (real `.app` bundle + `CFBundleURLTypes`) is deferred per ADR-003, backed by a concrete finding (Step 0.5 table): `install-service`'s macOS binary is intentionally non-bundle to keep `codesign` from bundle-sealing this repo's 80k+ files (`.claude/docs/codesigning.md`), and `CFBundleURLTypes` registration requires a real `.app` bundle Launch Services can scan — needs its own packaging-focused planning pass. **This deferral contradicts requirements.md's Success Metrics ("clickable ... on macOS and Linux dev machines") and In-Scope bullet naming macOS registration explicitly — flag for explicit user confirmation before treating this scope cut as final, since it is not something this plan can unilaterally decide away.**
4. **`public_id` schema tightening to `NotEmpty()`** after backfill is confirmed complete in all environments — deferred to a small follow-up task, not blocking this feature's ship.
5. **Metrics pipeline location**: Observability Plan assumes an existing metrics pipeline under `server/`; needs a quick verification at Epic 1 start (if none exists, structured logs alone satisfy v1, metrics become a stretch item).
6. **Session-ID stretch goal declined for this pass.** requirements.md's Scope lists applying the same type-prefixed-ULID scheme to session IDs as a stretch goal "if it can be done without disrupting existing session ID consumers," with the decision explicitly deferred to Phase 3 (Open Questions #4). Decision: **not picked up in this pass.** The backlog-item work already touches shared ID-handling surface area (dual-ID dispatch across ~40 call sites, ent schema, deep-link parsing) large enough on its own for the Large appetite; extending the same migration to sessions — a separate ID-consumer surface with its own call sites — is deferred to its own follow-up rather than folded in here. This is a scope cut, not an oversight (cross-artifact-consistency review, adversarial-review.md Minors).

## Dependency Visualization

```
Epic 1: BacklogItemID + public_id column
  |
  |-- Epic 2: ssq:// parsing + in-app resolution   (needs BacklogItemID to construct/parse links)
  |     |
  |     +-- Epic 5: Error/edge-case UX             (needs resolver's failure modes to exist)
  |
  +-- Epic 3: Workspace Host Registry (gossip)      (independent of Epic 1's ID work; can run in parallel)
        |
        +-- Epic 2 (cross-host branch): resolver consults registry for <hostname> lookups
        |
        +-- Epic 4: OS-level scheme registration (Linux) + --open-url subcommand
              (needs Epic 2's resolver endpoint to exist; independent of Epic 3's registry internals)
```

Epics 1 and 3 have no dependency on each other and can be worked in parallel. Epic 2 depends on
both (ID shape from Epic 1, registry lookups from Epic 3) but its same-host resolution path can
start as soon as Epic 1's `BacklogItemID` type exists, before Epic 3 finishes. Epic 4 depends
only on Epic 2's resolver route existing. Epic 5 threads through all of them at the end.

---

## Phase 1: Foundations

### Epic 1: Type-Prefixed Sortable Backlog Item IDs

#### Story 1.1: `BacklogItemID` newtype and generation

- **Given** a new backlog item is being created, **when** the create path runs, **then** a
  `BacklogItemID` is generated via `oklog/ulid/v2` with monotonic entropy and rendered as
  `bl_01J...`.
- **Given** a `BacklogItemID` string is parsed via `ParseBacklogItemID`, **when** the input is
  malformed (wrong prefix, invalid Crockford base32, wrong length), **then** parsing returns a
  descriptive error, never a partially-valid ID.

Tasks:
1. Add `github.com/oklog/ulid/v2` to `go.mod`/`go.sum` (`go get github.com/oklog/ulid/v2`, `go mod tidy`). *(1 file)*
2. Create `session/backlog_item_id.go`: `BacklogItemID` struct (unexported `ulid.ULID` field), `NewBacklogItemID() BacklogItemID` using a monotonic entropy source seeded once at process start. *(1 file)*
3. Add `(BacklogItemID) String() string` (renders `bl_` + Crockford base32) and `ParseBacklogItemID(s string) (BacklogItemID, error)` (validates prefix + delegates to `ulid.ParseStrict`) to `session/backlog_item_id.go`. *(1 file)*
4. Write `session/backlog_item_id_test.go`: round-trip test (`New` → `String` → `Parse` → equal), malformed-input rejection table test, monotonic-ordering test for same-millisecond generation. *(1 file)*
5. **(Pre-mortem P1 #3)** Wrap the monotonic entropy source in a mutex (or use `ulid.Monotonic` per-goroutine with a shared locked reader, per the library's documented pattern) — `oklog/ulid/v2`'s monotonic entropy source is not safe for concurrent use without external synchronization, and uncoordinated concurrent creation (parallel API requests, workflow-driven bulk creation) risks colliding/non-monotonic IDs that violate both the sortability guarantee and the `Unique()` constraint on `public_id`. *(1 file, same as task 2)*
6. **(Pre-mortem P1 #3)** Add a concurrent-generation test to `session/backlog_item_id_test.go`: N goroutines each calling `NewBacklogItemID()`, assert all IDs are unique and monotonically ordered — not just the same-millisecond single-threaded case in task 4. *(1 file, same as task 4)*

#### Story 1.2: Additive `public_id` schema column

- **Given** the ent schema change is applied, **when** `make ent-gen && go build ./...` runs,
  **then** the build succeeds with `public_id` available as a `Unique()`, indexed, optional
  string field, and the existing `id` field is unchanged.

Tasks:
1. Add `field.String("public_id").Optional().Unique()` plus a matching entry in `Indexes()` to `session/ent/schema/backlog_item.go`. *(1 file)*
2. Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`; confirm `go build ./...` passes; do not commit generated output (already gitignored per repo convention). *(0 files committed — verification step)*
3. **Verify unset-field representation using the generated ent code (not a manual DB insert):** create a `BacklogItem` row without setting `public_id` via the generated ent client, then read it back through the generated client and inspect the resulting Go value — confirm whether `Optional()` (without `.Nillable()`) yields `""` (empty string) or ent's codegen instead requires `.Nillable()` to distinguish "unset" from `""` at all (i.e. without it, ent may store/read back a Go zero value that's indistinguishable from a deliberately-empty string, or the column may be NULL in the DB while the generated field type coerces it to `""`). Decide explicitly whether to add `.Nillable()` to the schema field based on this finding, and record the decision (NULL-representable via `*string` + `IsNil()`, vs. plain `string` read back as `""`) in a comment on the schema field. This decision is the input Story 1.4's backfill query must match. *(1 file, `session/ent/schema/backlog_item.go` if `.Nillable()` is added; otherwise a comment-only change)*
4. Add a unit test in `session/ent_repository_backlog_test.go` asserting a row can be created with `public_id` unset (nil-safe) and separately with a value set, confirming the uniqueness constraint fires on a duplicate. *(1 file)*
5. **Add a centralizing accessor** `(*BacklogItemData) PublicID() (BacklogItemID, bool)` (on whichever type wraps the ent-generated row for application code — `session/backlog_item_id.go` or a new small file) that encapsulates the empty-string/NULL-as-absent check decided in task 3, returning `(zero value, false)` when absent. This closes the illegal-state-representable gap of comparing `public_id == ""` ad hoc at each call site — Story 1.3's dual-ID dispatcher and Story 1.4's backfill query must use this accessor rather than a direct `== ""`/`IsNil()` comparison. *(1 file)*

#### Story 1.3: Dual-ID lookup at the storage layer

- **Given** a lookup-by-ID call arrives with a legacy UUID string, **when** it's routed through
  the storage layer, **then** it resolves via the existing `uuid.Parse` path unchanged.
- **Given** a lookup-by-ID call arrives with a `bl_...` `public_id` string, **when** it's
  routed through the storage layer, **then** it resolves via the new `public_id` column without
  attempting `uuid.Parse` on it first.

Tasks:
0. **(Pre-mortem P1 #2)** Before wiring the dispatcher anywhere, grep every `uuid.Parse`/`uuid.UUID` call site that touches `BacklogItem` IDs (not just the ~40 estimated in the storage layer — include MCP tool lookups, notification links, workflow item references, and search/filter-by-id paths). Enumerate each site explicitly as its own sub-task here before implementation starts; "the dispatcher is wired into the primary path" is not sufficient evidence of coverage. *(discovery only, 0 files)*
1. Add a small dispatcher helper in `session/storage_backlog.go` (e.g. `resolveBacklogItemLookup(raw string) (lookupKind, error)`) that inspects the `bl_` prefix to decide UUID-vs-public_id branch, without touching the ~40 existing `uuid.Parse` call sites' UUID branch logic. Where this dispatch needs to check whether a loaded row's `public_id` is set, use Story 1.2 task 5's `PublicID() (BacklogItemID, bool)` accessor rather than comparing to `""` directly. *(1 file)*
2. Wire the dispatcher into the primary `GetBacklogItem`-style read path in `session/storage_backlog.go`, adding the `public_id` branch alongside the existing UUID branch. *(1 file, same as above — counted once)*
3. Mirror the same dispatch in `session/ent_repository_backlog.go`'s equivalent lookup method. *(1 file)*
4. **(Pre-mortem P1 #2)** Wire the same dispatcher into every additional call site enumerated in task 0 (MCP tool lookups, notification links, workflow item references, search/filter-by-id), each with its own regression test — do not rely on the primary read path alone. *(N files, per task 0's enumeration)*
5. Add `session/storage_backlog_test.go` cases: lookup by legacy UUID succeeds (regression), lookup by `public_id` succeeds, lookup by malformed string returns a clean not-found/invalid error (not a panic), plus one regression test per additional call site from task 4. *(1 file)*

#### Story 1.4: Backfill task for pre-existing rows

- **Given** the server starts with existing `BacklogItem` rows that have no `public_id`,
  **when** the backfill task runs, **then** every such row is assigned a freshly minted
  `BacklogItemID`, and re-running the task on an already-backfilled dataset is a no-op.
- **Note:** "no `public_id`" must be queried using whichever representation Story 1.2 task 3
  determined ent actually persists for an unset field — `public_id.IsNil()` if `.Nillable()`
  was added (NULL-backed), or `public_id == ""` only if Story 1.2's verification confirmed
  ent reliably round-trips unset as an empty string. Do not assume `== ""` without that
  verification — an unverified assumption here risks a silent no-op backfill (see ADR
  discussion in Story 1.2). The database-level predicate used in the query itself is
  necessarily ent-query syntax (`IsNil()`/`== ""`), but any in-Go code deciding "is this row's
  `public_id` absent" (outside the query builder itself) must go through Story 1.2 task 5's
  `PublicID() (BacklogItemID, bool)` accessor rather than a direct comparison.

Tasks:
1. Add a `BackfillBacklogItemPublicIDs(ctx)` function in `session/storage_backlog.go` that queries rows using the predicate Story 1.2 task 3 determined (`public_id.IsNil()` or `public_id == ""` — not assumed) for the DB-level filter, then confirms absence per row via Story 1.2 task 5's `PublicID()` accessor (not a second raw `== ""` check) before assigning new IDs and writing them back in a batch. *(1 file)*
2. Wire the call into server startup (`server/server.go` or equivalent init path), guarded so it only queries (cheap) rather than unconditionally writing on every boot. *(1 file)*
3. Add `session/storage_backlog_test.go` (or a new `_backfill_test.go`) case: seed rows with empty `public_id`, run backfill, assert all populated and unique; run backfill again, assert no rows changed (idempotency). *(1 file)*

---

## Phase 2: Link Format and In-App Resolution

### Epic 2: `ssq://` URL Scheme and Resolver

#### Story 2.1: URL scheme parsing

- **Given** a well-formed `ssq://<hostname>/<type>/<version>/<id>` string, **when** parsed,
  **then** hostname, type, version, and ID components are extracted via stdlib `net/url`
  (authority-position hostname parsing, no new dependency).
- **Given** a malformed or truncated `ssq://` string, **when** parsed, **then** parsing returns
  a typed error distinguishing "malformed" from "unsupported version" (version-mismatch link).

Tasks:
1. Create `session/deeplink/url.go`: `ParseDeepLink(raw string) (DeepLink, error)` struct with `Hostname`, `ItemType`, `Version`, `ID string` fields, using `net/url.Parse` + path-segment splitting. **The version check is folded into `ParseDeepLink` itself** — `ParseDeepLink` returns `ErrUnsupportedVersion` directly (as part of parsing, not a separate downstream validation pass) for any `<version>` this binary doesn't recognize, so an unsupported-version `DeepLink` value is never constructible/returned by the parser in the first place. *(1 file)*
2. ~~Add version-check logic as a separate pass~~ — superseded by task 1 (architecture review: parse-at-boundary gap on `Version`, resolved by folding the check into the parser instead of a second validation pass). No separate downstream version-validation call site should exist. *(0 files — task removed)*
3. Write `session/deeplink/url_test.go`: valid-URL table test, malformed-URL table test (truncated, missing segments, wrong scheme), version-mismatch test asserting `ParseDeepLink` itself returns `ErrUnsupportedVersion` (not a caller-side check). *(1 file)*

#### Story 2.2: Server-side deep-link resolver route

- **Given** a same-host `ssq://` link (hostname matches this instance's own `HostIdentity`
  advertisement), **when** `GET /api/deep-link/resolve` is called with it, **then** the local
  `BacklogItem` (looked up via Story 1.3's dual-ID dispatcher) is returned directly.
- **Given** a cross-host `ssq://` link, **when** resolved, **then** the response names the
  resolved `AdvertisedAddress` for the client to navigate to, or a clear "not registered" /
  "unreachable" error per the registry's liveness check (depends on Epic 3).

Tasks:
1. Add `srv.mux.HandleFunc("GET /api/deep-link/resolve", ...)` handler in `server/services/` (new file, e.g. `deep_link_resolver.go`), parsing the query param via `ParseDeepLink` from Story 2.1. *(1 file)*
2. Implement the same-host branch: dispatch to Story 1.3's lookup, return item payload or 404 with a distinct "item deleted/archived" reason per `ux.md`'s edge case table. *(1 file, same as above)*
3. Stub the cross-host branch behind a narrow `HostResolver` interface (defined here, consumer-side, per the Interface Placement pattern decision) so Epic 3 can implement it independently. *(1 file, same as above)*
4. Add `server/services/deep_link_resolver_test.go`: same-host resolve success, item-not-found, malformed-URL 400, version-mismatch 400 with distinct error body. *(1 file)*

#### Story 2.3: Upgrade existing Copy Link/Copy ID UI

- **Given** a user clicks "Copy Link" on a backlog item, **when** the link is copied, **then**
  its value is the new `ssq://<this-host>/backlog/v1/<public_id>` form (or the documented
  `https://` in-app equivalent), not the old `?item=<uuid>` query-string form.
- **Given** the copy action succeeds, **when** a screen reader is active, **then** the
  "Copied" confirmation is announced via a dynamic `aria-label`/`aria-live="polite"` region,
  not just a static label (closing `ux.md`'s flagged accessibility gap).

Tasks:
1. Update `web-app/src/components/backlog/BacklogItemDetail.tsx`'s `handleCopy("link", ...)` call site (~line 1271) to build the new link format instead of `${window.location.origin}/backlog?item=${item.id}`. *(1 file)*
2. Add a dynamic `aria-live="polite"` status region (or dynamic `aria-label` swap) reflecting `copiedField` state, replacing the static `aria-label="Copy shareable link"` that doesn't change post-copy. *(1 file, same as above)*
3. **Switch the displayed ID and "Copy ID" affordance to `public_id`**, per ADR-001's "the only identifier used in ... the UI's 'Copy Link'/'Copy ID' affordance" — update the `idText` span (~line 1257) and the `handleCopy("id", item.id)` call site (~line 1264) to read/copy `item.publicId` (or equivalent field) instead of `item.id`. Fall back to the UUID `item.id` for items with no `public_id` yet (pre-backfill/legacy rows), so the display never renders empty. *(1 file, same as above)*
4. Update/add a test in `web-app/src/components/backlog/BacklogItemDetail.test.tsx` (or equivalent) asserting the copied link value matches the new format, the accessible name changes after copy, the displayed/copied ID is the `public_id` when present, and falls back to the UUID when `public_id` is absent. *(1 file)*

---

## Phase 3: Cross-Host Resolution

### Epic 3: Workspace Host Registry (Gossip)

#### Story 3.1: `HostIdentity` generation and persistence

- **Given** a stapler-squad instance starts for the first time, **when** no
  `host_identity.json` exists, **then** a new `HostIdentity` is minted and persisted; **when**
  it starts again, **then** the same identity is loaded, not regenerated.

Tasks:
1. Create `session/host_identity.go`: `HostIdentity` type (wraps a `bl_`-style-prefixed ULID, e.g. `host_01J...`), `LoadOrCreateHostIdentity(stateDir string) (HostIdentity, error)`. *(1 file)*
2. Persist via the same flock-guarded JSON write pattern as `config/state.go` (new file `~/.stapler-squad/host_identity.json`, or extend `config/state.go` directly if that's a cleaner fit at implementation time). *(1-2 files)*
3. Add `session/host_identity_test.go`: first-run creates and persists; second load returns identical identity; corrupted file produces a clear error, not a silent regenerate. *(1 file)*
4. **(Adversarial review — advertisement integrity, ADR-002)** Generate an Ed25519 keypair (`crypto/ed25519`, stdlib) alongside `HostIdentity` on first run; persist both keys in the same `host_identity.json` (private key never leaves the instance, never logged). Expose `(HostIdentity) Sign(payload []byte) []byte` and a package-level `VerifyAdvertisement(pubKey ed25519.PublicKey, payload, sig []byte) bool`. *(same file as task 1)*
5. Extend `session/host_identity_test.go`: keypair is generated once and stable across reloads; `Sign`/`VerifyAdvertisement` round-trip; tampered payload or wrong key fails verification. *(same file as task 3)*

#### Story 3.2: Advertisement exchange (new HTTP transport on `--remote-port`)

- **Given** two instances each expose the new advertisement endpoint on their `--remote-port`
  HTTP server, **when** each completes an advertisement exchange cycle, **then** each has the
  other's `HostIdentity` and `AdvertisedAddress` in its local Workspace Host Registry within a
  bounded number of cycles.
- **Given** an instance stops advertising, **when** `Registry TTL` cycles elapse with no
  refresh, **then** its entry is pruned, not retained indefinitely.

**Cost basis note:** this story builds a new minimal HTTP-based advertisement transport, not an
extension of an existing loop — see the architecture review finding (Unresolved Question 1,
now resolved) that no existing inter-host polling/heartbeat mechanism exists in this codebase.
Estimate/appetite for this story should be read against "new transport" (endpoint + client +
scheduling loop), not "extend an existing loop with a bigger payload."

Tasks:
1. **Confirm `--remote-port` HTTP server is the integration point:** verify (`main.go:1055`
   `startRemoteAccess`, `server/auth/handlers.go:22` `RegisterRoutes`) that this second HTTPS
   server is the only genuinely cross-host-reachable HTTP surface in this codebase, and record
   in a code comment at the new endpoint's registration site that this is where advertisement
   gossip is served — closing Unresolved Question 1 with an implementation-time confirmation,
   not a fresh investigation. *(0 files committed — verification step)*
2. Add a new advertisement endpoint (e.g. `POST /internal/host-advertisement`) registered
   alongside the existing routes in `server/auth/handlers.go`'s `RegisterRoutes` (or a sibling
   file in `server/auth/`), accepting `{HostIdentity, AdvertisedAddress[], AdvertisedAt, PublicKey, Signature}` and
   returning this instance's own signed advertisement record. *(1-2 files)*
3. Add a client-side scheduling loop (new minimal poller — no existing loop to extend) that
   signs its own advertisement with `HostIdentity.Sign` (task 4 of Story 3.1) and periodically
   POSTs it to each currently-known `AdvertisedAddress`, recording the response. *(1 file)*
4. Create `session/host_registry.go`: in-memory + persisted `map[HostIdentity]RegistryEntry{AdvertisedAddress[], LastSeenAt, PublicKey}`, `Advertise(entry)` (upsert + refresh `LastSeenAt`), `Prune(ttl)`.
   **(Adversarial review — advertisement integrity, ADR-002)** `Advertise` must TOFU-pin: on
   the first advertisement seen for a given `HostIdentity`, record its `PublicKey` as trusted;
   on every later advertisement for that identity, verify `Signature` against the pinned key via
   `VerifyAdvertisement` and reject (drop, log, do not upsert) the entry if verification fails or
   the claimed `PublicKey` differs from the one on file. *(1 file)*
5. Wire bounded re-gossip: on receiving an advertisement for a `HostIdentity` not already known, re-broadcast it once to this instance's own known peers (bounded fan-out, no infinite loop — track an already-seen set per advertisement round). *(1 file, same file as task 3)*
6. Add `session/host_registry_test.go`: advertise-then-lookup, TTL-based prune with a fake clock (no wall-clock sleep), re-gossip converges a 3-node fake topology within a pinned, explicit cycle count (not an unbounded "N" — set the actual number once the re-gossip fan-out/interval are implemented). Add an HTTP-handler test for the new endpoint (task 2) covering malformed payload and successful exchange.
   **(Adversarial review — advertisement integrity, ADR-002)** Add TOFU-pinning cases: first
   advertisement for a new `HostIdentity` is accepted and pins its key; a later advertisement
   from the same identity with a mismatched `PublicKey` or invalid `Signature` is rejected and
   does not overwrite the existing registry entry; a valid re-advertisement signed by the
   pinned key is accepted and refreshes `LastSeenAt`. *(2 files)*
7. **(Pre-mortem P1 #1)** Before Epic 3 is marked done, run one manual validation between two machines on genuinely different networks (not same-LAN, not same-host processes, not a VPN/Tailscale-style overlay that masks the real topology) and confirm advertisement/resolution actually converges — the fake in-process 3-node test in task 6 does not exercise real NAT/routing conditions. Document the result and any NAT/VPN reachability limitation explicitly in user-facing docs (e.g. a note near the `ssq://` feature's README/help text) so "known but unreachable" reads as an expected outcome of the network topology, not a bug. *(manual test + doc update, 0-1 files)*

#### Story 3.3: Registry-backed cross-host resolution

- **Given** a `ssq://` link's hostname resolves to a known, non-expired registry entry,
  **when** resolved, **then** the resolver returns that entry's `AdvertisedAddress` for
  client-side navigation.
- **Given** a hostname has no registry entry, **when** resolved, **then** the resolver returns
  a distinct "not registered" error (never a synthesized guess at an address).
- **Given** a hostname has a registry entry but a bounded liveness check times out, **when**
  resolved, **then** the resolver returns a distinct "known but unreachable" error.

Tasks:
1. Implement Epic 2 Story 2.2's `HostResolver` interface stub against `session/host_registry.go`'s lookup, in `server/services/deep_link_resolver.go`. *(1 file)*
2. Add a bounded-timeout liveness check (short HTTP HEAD/health-check against the resolved `AdvertisedAddress`) distinguishing "not registered" from "registered but unreachable." *(1 file, same as above)*
3. Add `server/services/deep_link_resolver_test.go` cases: known-live entry resolves, unknown hostname returns "not registered," known-but-unreachable entry (fake server that never responds) returns the distinct timeout error within a bounded test duration. *(1 file)*
4. Add the `--list-known-hosts` CLI inspection command (Observability Plan) as a small Cobra subcommand printing the local registry's contents. *(1 file)*

---

## Phase 4: OS-Level Integration

### Epic 4: `--open-url` Subcommand and Linux Scheme Registration

#### Story 4.1: `--open-url` subcommand

- **Given** `stapler-squad --open-url ssq://<hostname>/backlog/v1/<id>` is invoked, **when** it
  runs, **then** it translates the scheme URL to a local `http://localhost:8543/...` navigation
  target and shells to the OS opener (`open` on macOS, `xdg-open` on Linux) via `safeexec`.

Tasks:
1. Add `--open-url` flag/subcommand handling in the Cobra command tree (wherever `stapler-squad`'s root command is defined), calling `ParseDeepLink` from Story 2.1. *(1 file)*
2. Implement the translate-and-shell logic: build the `http://localhost:8543/...` URL, shell to `open`/`xdg-open` via `safeexec.CommandContext` (no Go equivalent exists for launching the OS's default browser — subshell is justified per `.claude/rules/prefer-go-git-over-subshells.md`'s "still fine" exception). *(1 file, same as above)*
3. Add a test exercising the URL-translation logic directly (not the actual OS shell-out, which is mocked/stubbed). *(1 file)*

#### Story 4.2: Linux `.desktop` scheme registration

- **Given** `make install-service` runs on Linux, **when** it completes, **then** a
  `.desktop` file with `MimeType=x-scheme-handler/ssq;` is installed and
  `update-desktop-database`/`xdg-mime default` have registered it, idempotently (safe to
  re-run).

Tasks:
1. Add `.desktop` file generation (template with `Exec=stapler-squad --open-url %u`) in the install path (Makefile target or a small Go helper invoked from it). *(1-2 files)*
2. Add idempotent registration calls (`xdg-mime default stapler-squad.desktop x-scheme-handler/ssq`, `update-desktop-database`) via `safeexec`, guarded to skip if already registered. *(1 file, same as above)*
3. Add a test (or a documented manual verification step, since this genuinely requires a Linux desktop environment) confirming re-running registration doesn't duplicate entries. *(1 file)*

---

## Phase 5: Edge Cases and Polish

### Epic 5: Error States and Edge-Case UX

#### Story 5.1: Error state UI for each resolver failure mode

- **Given** any of the resolver's distinct failure reasons (item deleted/archived, host
  unreachable, host not registered, malformed link, version-mismatch link), **when** the UI
  receives it, **then** a distinct, `role="status"`-based message is shown per `ux.md`'s
  "wrong workspace" mental model (not a generic 404), reusing `InlineError.tsx`/
  `TriageErrorBanner.tsx` patterns.

Tasks:
1. Add a `DeepLinkErrorBanner` (or extend `InlineError.tsx`) component in `web-app/src/components/` mapping each backend error reason to a distinct message + icon. *(1 file)*
2. Wire the resolver-consuming page (wherever `ssq://`/resolved links land, e.g. a `web-app/src/app/resolve/page.tsx` or extension of the existing backlog page) to render this banner on each failure branch. *(1 file)*
3. Add a component test asserting each of the five error reasons renders distinct, accessible copy. *(1 file)*

#### Story 5.2: Backward-compatibility regression coverage

- **Given** an old `?item=<uuid>` link, **when** opened on the upgraded app, **then** it
  resolves identically to before this feature shipped, with no warning or migration prompt.

Tasks:
1. Add an end-to-end regression test in `tests/e2e/` (feature-annotated per `.claude/rules/e2e-test-conventions.md`) opening a legacy `?item=<uuid>` URL and asserting the item detail view loads normally. *(1 file)*
2. Add a corresponding Go-level regression test confirming Story 1.3's dual-ID dispatcher never routes a valid UUID through the `public_id` branch. *(1 file)*

---

## Step 6 Summary

- **Epics:** 5
- **Stories:** 14
- **Tasks:** 51
- **Domain Glossary terms:** 12
- **Flagged/steering-driven choices:**
  - **Workspace Host Registry**: built as a **gossip-based, self-announcing registry** per direct user steering (each host mints a durable `HostIdentity`, advertises `AdvertisedAddress[]`), explicitly rejecting the research-recommended static admin-maintained host list. Scoped to fit the Large appetite by **building a new minimal HTTP-based advertisement transport on the existing `--remote-port` HTTP server** (confirmed by architecture review to be the only genuinely cross-host-reachable surface already in this codebase — no existing inter-host polling/heartbeat loop exists to piggyback on) rather than a bespoke socket protocol, and by explicitly deferring full anti-entropy/SWIM-style convergence guarantees to a flagged follow-up (ADR-002, Unresolved Question 2) rather than silently downgrading back to a static list.
  - **ID storage reconciliation**: resolved in favor of the **additive `public_id` string column**, leaving the existing `id` UUID field and all ~40 `uuid.Parse`/`uuid.UUID` call sites untouched — rejecting `architecture.md`'s alternative of changing `id`'s field type itself, because of the call-site blast radius, the lack of any requirement forcing the internal PK's type to change, and agreement from two of three research docs plus existing string-PK precedent in this schema (ADR-001).
  - Also flagged as a deliberate scope cut (not silent): macOS OS-level scheme registration deferred to a follow-up (ADR-003); `public_id` schema tightening to `NotEmpty()` deferred until backfill is confirmed complete (Unresolved Question 4).

## Changelog

- **2026-08-19 — Adversarial-review patch (3 blockers):**
  1. **ADR-003 macOS scope cut evidence:** Step 0.5's macOS row and the Pattern Decisions table's macOS row now cite the concrete conflict between `install-service`'s non-bundle Mach-O binary (`.claude/docs/codesigning.md`, embedded `__info_plist` section, deliberately kept out of bundle layout to avoid `codesign` sealing this repo's 80k+ files) and `CFBundleURLTypes`'s requirement that Launch Services scan a real `.app` bundle. Both rows and Unresolved Question 3 now flag that this deferral contradicts requirements.md's Success Metrics and In-Scope wording and needs explicit user confirmation, not a unilateral plan-level decision.
  2. **Story 3.2 piggyback-loop spike:** added Task 0 (must run before Tasks 1-4) requiring an explicit enumeration/decision of which existing `session/` polling loop advertisement gossip piggybacks on (or that a new minimal poller is needed), recorded in a code comment before implementation proceeds — resolves Unresolved Question 1 at the right time instead of leaving it assumed. Added a one-line caveat that the task list is contingent on this spike's outcome.
  3. **Story 1.2/1.4 NULL-vs-empty-string verification:** added Story 1.2 Task 3, verifying via generated ent code (not a manual DB insert) whether an unset `public_id Optional()` field round-trips as `""` or requires `.Nillable()`/NULL handling, with the decision recorded on the schema field. Story 1.4's given/when/then, backfill task description, and the Migration Plan's backfill step now reference that decision (`IsNil()` or `== ""`) instead of asserting `== ""` unconditionally, closing the silent-no-op risk.

- **2026-08-19 — Architecture-review patch (1 blocker, 3 concerns):**
  1. **Story 3.2/ADR-002 piggyback premise (blocker):** removed the false "extend existing peer polling" premise (`session/workspace_peers.go` is purely local — verified no inter-host network transport exists anywhere in `session/`). Step 0.5's Cross-host registry row, the Pattern Decisions table's Cross-host registry row, Unresolved Question 1 (now marked resolved), and Story 3.2's tasks (renumbered 1-6) are rewritten around building a new minimal HTTP-based advertisement endpoint on the existing `--remote-port` server (`main.go:1055` `startRemoteAccess`, `server/auth/handlers.go:22` `RegisterRoutes`) instead of a spike to find a loop to piggyback on. Step 6 Summary's flagged-choices bullet updated to match.
  2. **Story 2.3 vs. ADR-001 (concern):** added Story 2.3 Task 3, switching the displayed ID and "Copy ID" button (`BacklogItemDetail.tsx` `idText` span ~line 1257, `handleCopy("id", item.id)` ~line 1264) to `public_id` with a documented UUID fallback for pre-backfill/legacy items, so the story no longer contradicts ADR-001. Task 4 (test) updated to cover both cases.
  3. **Story 2.1 `ParseDeepLink` version check (concern):** folded the `ErrUnsupportedVersion` check into `ParseDeepLink` itself (Task 1) instead of a separate downstream validation pass (old Task 2, now removed/superseded), so an unsupported-version `DeepLink` can never be returned by the parser. Task 3's test description updated to assert this directly.
  4. **Story 1.2/1.4 `public_id` empty-string sentinel (concern):** added Story 1.2 Task 5, a `(*BacklogItemData) PublicID() (BacklogItemID, bool)` accessor centralizing the empty-string/NULL-as-absent check; Story 1.3's dispatcher task and Story 1.4's backfill note/task now require using that accessor for in-Go absence checks instead of ad hoc `== ""` comparisons (the ent query predicate itself is unaffected).
