# ADR-001: Add a `public_id` column instead of changing `BacklogItem.id`'s type

## Status
Accepted

## Context

`backlog-deep-linking` needs a new externally-shareable identifier for backlog items:
type-prefixed, sortable, encodes creation time (`bl_01J...`, a ULID with a `bl_` prefix per
`requirements.md`). The item currently has `field.UUID("id", uuid.UUID{}).Default(uuid.New)`
(`session/ent/schema/backlog_item.go:22-23`).

Research surfaced two incompatible recommendations:

- `research/stack.md` and `research/pitfalls.md` recommend leaving `id` as `field.UUID(...)`
  untouched and adding a new, additive `field.String("public_id")` column populated at create
  time. `pitfalls.md` states explicitly: "no ALTER is required, and none should be attempted."
  Both cite `session/ent/schema/analytics_event.go:17` and `shell.go:23` as existing
  string-primary-key precedent elsewhere in this schema, so a string ID column is not a novel
  pattern in this codebase.
- `research/architecture.md` recommends changing `id` itself to `field.String("id")` (no
  `Default`), wrapped in a new `session.BacklogItemID` newtype (following the `RepoRef`/
  `AccountRef` pattern in `.claude/rules/primitive-obsession-checklist.md`). It documents that
  this requires migrating roughly 40 `uuid.Parse`/`uuid.UUID` call sites, confined to
  `session/storage_backlog.go` and `session/ent_repository_backlog.go`.

Both can't be adopted — the schema needs exactly one shape for the field ent's generated code
treats as the primary key.

## Decision

**Add `public_id` as a new, additive `field.String` column on `BacklogItem`. Leave `id` as
`field.UUID("id", uuid.UUID{}).Default(uuid.New)`, unchanged.**

`public_id` is:
- Generated once at creation (`BacklogItemID` newtype wrapping `oklog/ulid/v2`, prefixed
  `bl_`), stored alongside `id`, never regenerated or backfilled onto existing rows.
- Unique-indexed (`session/ent/schema/backlog_item.go`'s `Indexes()`), optional at the ent
  level only until the migration task backfills it (see plan.md's Migration Plan), then
  enforced `NotEmpty` for all new rows going forward.
- The only identifier used in `ssq://` URLs, API responses, and the UI's "Copy Link"/"Copy ID"
  affordance (`web-app/src/components/backlog/BacklogItemDetail.tsx:1257-1277`) going forward.
  The existing UUID `id` remains for internal FK/edge relationships (`session/ent/schema`'s
  edges into `BacklogItem`) and continues to satisfy every existing `uuid.Parse` call site with
  zero code changes there.

## Alternatives Rejected

**Change `id`'s field type to `field.String`, wrap in a `BacklogItemID` newtype
(`architecture.md`'s recommendation).**

Rejected because:
1. **Blast radius.** ~40 `uuid.Parse`/`uuid.UUID` call sites in `session/storage_backlog.go`
   and `session/ent_repository_backlog.go` would all need to change type, plus every edge
   query that joins through `BacklogItem.id` (session/ent's generated `.Where(backlogitem.ID(...))`
   clauses) needs to compile against the new field type. `architecture.md` itself scoped this
   as the highest-cost part of the whole feature.
2. **Forced migration path for zero benefit.** Requirements.md is explicit: "existing UUIDv4
   IDs must work indefinitely, no migration/backfill" is a hard constraint on the *external*
   identifier, but nothing requires the *internal* primary key to also become the external
   identifier's type. Reusing `id` as both the FK target and the externally-parsed ULID
   conflates two concerns (internal referential identity vs. external shareable identity) that
   have different lifecycle and format requirements — the newtype and prefix only matter to
   the outside world.
3. **ent codegen risk.** `architecture.md`'s own Section 6 flags that changing a primary key's
   field type requires a clean `rm -rf session/ent/*.go && make ent-gen && go build ./...`
   rather than trusting the generate-stamp file — a real risk of a broken build if any call
   site is missed, for a change with no corresponding requirement forcing it.
4. **Two of three research docs, plus existing schema precedent, agree on additive.**
   `stack.md`, `pitfalls.md`, and this codebase's own `analytics_event.go`/`shell.go` string-PK
   precedent all point the same direction.

**Do nothing / expose the raw UUID with a `bl_` string prefix at the API layer only (no new
column).** Considered and rejected: it doesn't give the ID a real sortable-by-creation-time
ULID shape (requirements.md's explicit ask), and any consumer that round-trips the ID back
through `uuid.Parse` after stripping the prefix would just be parsing a UUID with extra steps —
none of the actual ULID/sortability value is realized.

## Consequences

- New ent migration: additive column + unique index, no data loss, no `ALTER ... TYPE`.
- All ~40 existing `uuid.Parse`/`uuid.UUID` call sites in `session/storage_backlog.go` and
  `session/ent_repository_backlog.go` are untouched.
- A new, small surface is added instead: `NewBacklogItemID()`/`ParseBacklogItemID()` in
  `session/backlog_item_id.go` (new file), and lookups need to accept *either* an internal
  UUID *or* a `public_id` string at the storage layer boundary (see plan.md Story
  "Dual-ID lookup at the storage layer").
- Every new backlog item write path must populate `public_id` at creation; a startup/lazy
  backfill task must populate it for pre-existing rows that predate this feature (see
  plan.md's Migration Plan) so old items remain linkable.
