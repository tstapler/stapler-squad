# Implementation Plan: backlog-item-activity-log

**Feature**: An ungated `post_backlog_update` MCP tool letting any session (with or
without `STAPLER_SESSION_UUID`, linked or not) post a free-form, timestamped, attributed
note to a backlog item; readable via `get_backlog_item`, `GetBacklogItem`, the live
`WatchBacklogItems` stream, and a new web UI section.
**Date**: 2026-08-18
**Status**: Ready for implementation
**ADRs**: ADR-001 (sibling table, not extending `BacklogProgressNote`), ADR-002
(single-entry event payload, not whole-item snapshot)

---

## Revisions after adversarial review (round 1, 2026-08-18)

The first adversarial review pass (`implementation/adversarial-review.md`) returned
**BLOCKED** on 3 blockers and raised 4 concerns. All seven are incorporated below, each
verified against live source (not research-doc citations) before writing the fix. One
correction to the review's own citations: `wait_for_backlog_event` is fully contained in
`server/mcp/tools_backlog.go:577-666`, not `435-666` — lines 435-576 are the file's
`eventTypeAny`-family consts, `WaitForBacklogEventResult`, `backlogEventKindFilterValue`,
`buildMatchedWaitResult`, and `currentStateWaitResult` helpers the function calls, not the
function body itself. The event-matching loop's cited lines (652-663) were exact.

- **Blocker 1** (ADR-002's anti-clobber claim false for every other event kind) — fixed by
  extending `upsertItem`'s existing `itemSessions` anti-clobber guard
  (`web-app/src/lib/store/backlogItemsSlice.ts:55-67`) to also cover `activityNotes`, same
  shape, same file. New Story 6.2.3 / Task 6.2.3a. Test added to Story 8.5.1 (Task 8.5.1b).
  ADR-002's Consequences section updated to name this fix explicitly.
- **Blocker 2** (`AppendActivityNote`'s sparse snapshot breaks `WatchBacklogItems`
  status/category filters) — fixed by having `AppendActivityNote` do a cheap
  `.Select(FieldStatus, FieldRepoPath).Only(ctx)` read before publishing, populating those
  two fields (confirmed by reading `backlogItemMatchesFilters`,
  `server/services/backlog_service_events.go:213-224`, which checks exactly `item.Status`
  and `item.RepoPath` — "category" has no dedicated field, it's matched against
  `RepoPath`). Task 2.1.2a rewritten; test added to Story 8.2.1 (Task 8.2.1c).
- **Blocker 3** (`wait_for_backlog_event` is an unaudited 4th `BacklogChangeKind` touch
  point) — fixed by an explicit early-`continue` for `events.BacklogChangeActivityNoteAdded`
  in the event-matching loop (`server/mcp/tools_backlog.go:655`), so this kind can never
  satisfy any `event_type` filter including `"any"`. New Epic 4.4 / Story 4.4.1 / Task
  4.4.1a. Regression test added as Epic 8.4's Story 8.4.2.
- **Concern 1** (sanitization only at render time, not persist time per requirements.md's
  literal text) — fixed by adding an HTML-tag-strip step (`session.SanitizeForAgentContext(message, 2000)`,
  confirmed signature in `session/backlog_context.go:14-18`; `maxLen=2000` never truncates
  a message already validated ≤2000 chars) to Task 5.1.2a's write path, before persisting.
  Test added to Story 8.3.1 (Task 8.3.1c is now the FK test; sanitization test is Task
  8.3.1d).
- **Concern 2** (no FK-violation → `ErrItemNotFound` mapping for a nonexistent `item_id`) —
  fixed at the repository layer: `AppendActivityNote` (Task 2.1.2a) now detects
  `ent.IsConstraintError(err)` (the established idiom in this file, see
  `session/ent_pipeline_mode_repository.go:50`) on the `Create()` call and wraps it as
  `session.ErrNotFound`, which Task 5.1.2b's already-planned `errors.Is(err,
  session.ErrNotFound)` → `ErrItemNotFound` mapping then correctly catches. Test added to
  Story 8.3.1 (Task 8.3.1c).
- **Concern 3** (zero test coverage for `get_backlog_item`'s Activity Log rendering) —
  fixed by new Epic 8.6, following `TestGetBacklogItem_ReturnsItemWithEnvelope`'s exact
  style (confirmed present at `server/mcp/tools_backlog_test.go:280`).
- **Concern 4** (ADR-001's duplication doubles the eager-load-edge surface Blocker 1 has to
  guard) — documentation-only fix: new paragraph in ADR-001's Consequences → Negative
  section naming this tradeoff explicitly.

Round 2 adversarial review (post-fix) returned **CLEAN**: 0 blockers, 0 concerns, 2
minors (a stale `publishItemChangedSnapshot` doc comment claiming only
`DeleteBacklogItem` calls it directly — cosmetic, no test/behavior depends on it — noted
for the implementer to touch in passing during Task 2.1.2a rather than a separate task).

## Revisions after validation (Phase 4, 2026-08-18)

The Phase 4 validation pass (`implementation/validation.md`) found 20/27 requirement/AC
stories directly test-mapped, 5 covered only by a build/CI check (acceptable), and 2
genuine test-coverage gaps, plus 3 file-path/placement corrections. All 5 incorporated:

- **Gap 1** (validation.md's numbering — the `ent.IsConstraintError` FK-violation branch
  added to Task 2.1.2a during blocker remediation had no test — the "nonexistent
  item_id" test only reaches the earlier `Select(...).Only(ctx)` short-circuit, never the
  `Create()`-time branch, and the actual delete-between-steps race is impractical to
  trigger deterministically) — fixed by extracting the branch into its own
  `mapAppendActivityNoteCreateError` function (Task 2.1.2a, revised) and unit-testing
  that function directly in isolation (new Task 8.2.1e), rather than trying to reproduce
  the race in a test.
- **Gap 2** (validation.md's numbering — no test proves `GetBacklogItem`'s eager-load →
  `BacklogItemData.ActivityNotes` → `backlogItemToProto` mapper actually carries
  `ActivityNotes` end-to-end — a separate code path from `get_backlog_item`'s MCP-tool
  rendering in Epic 8.6, which calls `ListActivityNotesForItem` directly and never
  touches it) — fixed by two tests: a repository-level eager-load test (new Task
  8.2.1d) and a proto-mapper test extending the sibling
  `TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent`
  (confirmed live at `server/services/backlog_service_test.go:839`, new Epic 8.7).
- **Correction 1** (Task 8.4.1b should extend the existing table-driven
  `TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries`,
  not create a new standalone test function) — already applied to Task 8.4.1b/Story 8.4.1.
- **Correction 2** (Epic 8.1's cascade-delete test belongs in
  `session/ent_repository_backlog_test.go`, not a nonexistent `session/ent/schema/*_test.go`
  convention) — already applied to Story 8.1.1's file path.
- **Correction 3** (the two frontend test files live under `__tests__/` subdirectories) —
  verified directly against the live repo (`web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts`,
  `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts` both confirmed to exist)
  and corrected throughout Epic 8.5 and the file-touch summary.

---

## Deviations found while validating research citations against source

All decisions in the task brief hold. Three precise corrections to the research docs'
citations, both incorporated into the tasks below:

1. **A new `BacklogChangeKind` touches THREE Go files, not two.** Research (stack.md,
   architecture.md Finding 4b) named `session/backlog_item_change.go` and
   `pkg/events/types.go`. Verified: `server/services/backlog_item_event_publisher.go`
   actually imports `github.com/tstapler/stapler-squad/server/events` (not
   `pkg/events` directly), and that package
   (`server/events/forward.go`) is a hand-maintained transparent type/const alias
   layer over `pkg/events` — it re-exports every `BacklogChangeKind` constant by name
   (lines 30-38). A new constant added only to `pkg/events/types.go` compiles fine
   but is invisible through the `server/events` import path any `server/services` code
   uses; `server/events/forward.go` needs the new constant added too (Task 4.1.3).
2. **The new `BacklogItemEvent` oneof variant's proto field number is `10`, not the
   next-looking small integer.** Verified `proto/session/v1/backlog.proto:751-780`:
   the oneof already uses field numbers 2-7 for its six variants, field `8` is taken
   by the message-level `seq` field (not part of the oneof), and field `9` is already
   used by `snapshot_complete` — chosen specifically to skip `8`. The next free number
   for the whole message is `10` (Task 3.1.2).
3. **`BacklogItem`'s next free field number is confirmed `35`** (highest in use is
   `plan_rejected_at = 34`, `backlog.proto:204`) — matches stack.md's citation exactly,
   no correction needed there.

No other citation from stack.md/features.md/architecture.md/pitfalls.md needed
correction; `request_review`'s length-cap check is at `tools_backlog.go:888` (function
starts at line 865) and `create_backlog_item`'s "Not role/item-gated" description text
is at `tools_backlog.go:2524` — both close enough to research's cited line numbers to
require no plan change, called out here only because exact lines are used below.

---

## Dependency Visualization

```
Phase 1: Ent Schema
   Epic 1.1 (schema file + edge + regen)
        │
        ▼
Phase 2: Repository / Storage layer
   Epic 2.1 (DTO + repo methods + storage wrappers + eager-load)
        │
        ├─────────────────────────────┐
        ▼                             ▼
Phase 3: Proto + codegen        Phase 4: Event bus wiring (Go)
   Epic 3.1 (messages)             Epic 4.1 (BacklogChangeKind x3 files)
        │                             │
        ▼                             ▼
   Epic 3.2 (Go↔proto mapper)     Epic 4.2 (convertEventToBacklogItemEvent case)
        │                             │
        └──────────────┬──────────────┘
                        ▼                Epic 4.3 (repo publish call — needs
                Phase 5: MCP tool         BacklogChangeKind from 4.1 + DTO
                   Epic 5.1 (tool)        from 2.1; feeds into 5.1's handler
                   Epic 5.2 (rendering)   which calls AppendActivityNote)
                        │
                        ▼
                Phase 6: Frontend
                   Epic 6.1 (mapper chain — needs proto/gen from Phase 3)
                   Epic 6.2 (reducer + watch-stream case — needs Phase 4's
                             new oneof variant, generated into TS bindings
                             by Phase 3's proto-gen)
                   Epic 6.3 (UI component — needs 6.1 + 6.2)
                        │
                        ▼
                Phase 7: Registry
                   Epic 7.1 (make registry-generate)
                        │
                        ▼
                Phase 8: Tests (spans every layer above)
```

Phase 3 and Phase 4 are independent of each other (both depend only on Phase 2's DTO)
and can be implemented in either order, but Phase 4's Epic 4.3 (the actual publish call
inside `AppendActivityNote`) needs Epic 4.1's new `BacklogChangeKind` to exist first.
Phase 5 needs both Phase 3 (proto messages for rendering/registration) and Phase 4
(so `AppendActivityNote` actually publishes). Phase 6 needs Phase 3's generated
TypeScript bindings and Phase 4's new oneof variant name. Phase 7 and Phase 8 come last.

---

## Phase 1: Ent Schema

### Epic 1.1: `BacklogActivityNote` ent type
**Goal**: A new, empty-by-default table exists, generated and building cleanly, with no
behavior wired to it yet.

#### Story 1.1.1: Schema file, parent edge, regen
**As a** developer, **I want** a `BacklogActivityNote` ent schema modeled on
`BacklogProgressNote`, **so that** later phases have a table to write to.
**Acceptance Criteria**:
- `session/ent/schema/backlog_activity_note.go` defines `id`, `item_id`, `message`
  (required, non-optional), `author_session_uuid` (optional), `author_session_title`
  (optional), `created_at` (`Default(time.Now)`, `Immutable()`), an edge back to
  `BacklogItem`, and a `(item_id, created_at)` composite index.
- `BacklogItem.Edges()` gains `edge.To("activity_notes", BacklogActivityNote.Type)` with
  `entsql.OnDelete(entsql.Cascade)`.
- `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` (run
  from `session/ent/`) regenerates cleanly; `go build ./...` passes.
**Files**: `session/ent/schema/backlog_activity_note.go` (new),
`session/ent/schema/backlog_item.go`, `session/ent/` (generated, committed wholesale).

##### Task 1.1.1a: Write the `BacklogActivityNote` schema file (~5 min)
- Create `session/ent/schema/backlog_activity_note.go`, copying
  `session/ent/schema/backlogprogressnote.go`'s structure (package, imports, doc-comment
  style) but with fields: `field.UUID("id", uuid.UUID{}).Default(uuid.New)`,
  `field.UUID("item_id", uuid.UUID{})`, `field.String("message")` (no `.Optional()` —
  this tool's only payload), `field.String("author_session_uuid").Optional()`,
  `field.String("author_session_title").Optional()`,
  `field.Time("created_at").Default(time.Now).Immutable()`.
- `Edges()`: `edge.From("item", BacklogItem.Type).Ref("activity_notes").Field("item_id").Unique().Required()`.
- `Indexes()`: `index.Fields("item_id", "created_at")`.
- Doc comment on the type: explain it's the append-only, ungated sibling to
  `BacklogProgressNote` — see ADR-001 — never written by the role-gated tools.
- Files: `session/ent/schema/backlog_activity_note.go`

##### Task 1.1.1b: Add the parent-side edge on `BacklogItem` (~2 min)
- In `session/ent/schema/backlog_item.go`'s `Edges()`, add
  `edge.To("activity_notes", BacklogActivityNote.Type).Annotations(entsql.OnDelete(entsql.Cascade))`,
  placed next to the existing `edge.To("progress_notes", ...)` line for locality.
- Files: `session/ent/schema/backlog_item.go`

##### Task 1.1.1c: Regenerate ent code and build (~4 min)
- Run `cd session/ent && go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`
  (the exact command from `session/ent/generate.go`'s `//go:generate` directive — do
  NOT omit `--feature sql/upsert`, it breaks `UpsertRule`-style generated methods
  elsewhere).
- Run `go build ./...` from repo root; fix any generated-code drift.
- Commit note (for the eventual PR, not this task): stage all of `session/ent/`
  together with the two schema files per CLAUDE.md's ent workflow.
- Files: `session/ent/` (generated files — do not hand-edit), verification only, no new
  hand-written files.

---

## Phase 2: Repository / Storage Layer

### Epic 2.1: Domain DTO, repository methods, storage wrappers, eager-load
**Goal**: `AppendActivityNote`/`ListActivityNotesForItem` exist end-to-end through
`Storage`, and `GetBacklogItem` returns the full log on every fetch.

#### Story 2.1.1: `ActivityNoteData` DTO and ent→DTO mapper
**As a** developer, **I want** a domain DTO for one activity-note row, **so that**
callers above the ent layer never see `*ent.BacklogActivityNote` directly.
**Acceptance Criteria**:
- `ActivityNoteData{ID, Message, AuthorSessionUUID, AuthorSessionTitle, CreatedAt}`
  exists in `session/repository.go`, doc-commented next to `ProgressNoteData`.
- `activityNoteToData(n *ent.BacklogActivityNote) ActivityNoteData` exists in
  `session/ent_repository_backlog.go`, mirroring `progressNoteToData` exactly.
**Files**: `session/repository.go`, `session/ent_repository_backlog.go`

##### Task 2.1.1a: Add `ActivityNoteData` DTO (~2 min)
- In `session/repository.go`, add the `ActivityNoteData` struct immediately after
  `ProgressNoteData` (around line 417), with the same doc-comment convention ("domain
  DTO replacing `*ent.BacklogActivityNote` in Storage returns").
- Files: `session/repository.go`

##### Task 2.1.1b: Add `activityNoteToData` mapper (~2 min)
- In `session/ent_repository_backlog.go`, add `activityNoteToData` immediately after
  `progressNoteToData` (around line 161), following its exact shape (`n.ID.String()`,
  direct field copies, `n.CreatedAt`).
- Files: `session/ent_repository_backlog.go`

#### Story 2.1.2: `AppendActivityNote` / `ListActivityNotesForItem` repository methods
**As a** developer, **I want** repository methods that insert and list activity notes,
**so that** the MCP tool and `GetBacklogItem` have something to call.
**Acceptance Criteria**:
- `AppendActivityNote(ctx, itemID, authorSessionUUID, authorSessionTitle, message string) error`
  does a bare `Create()` (no read-modify-write), then best-effort publishes a
  `ChangeActivityNoteAdded` event carrying only the new note (see Phase 4 — this task
  only wires the call, Phase 4 supplies the `BacklogChangeKind`/field it needs).
- **[Blocker 2 fix]** Before publishing, `AppendActivityNote` does a cheap, read-only,
  field-scoped query for the item's `Status` and `RepoPath` (the exact two fields
  `backlogItemMatchesFilters`, `server/services/backlog_service_events.go:213-224`,
  checks — `status_filter` against `item.Status`, `category_filter` against `item.RepoPath`,
  since there is no dedicated "category" field on `BacklogItemData`) and populates them on
  the snapshot passed to `publishItemChangedSnapshot`. This is a read, not a
  read-modify-write, so it introduces no new concurrency hazard.
- **[Concern 2 fix]** A foreign-key-violation error from the `Create()` call (nonexistent
  `item_id`) is detected via `ent.IsConstraintError(err)` (the established idiom in this
  package — see `session/ent_pipeline_mode_repository.go:50`,
  `session/storage.go:514`) and wrapped as `session.ErrNotFound`
  (`fmt.Errorf("backlog item %s not found: %w", itemID, ErrNotFound)`), not left as an
  opaque wrapped SQL error. This is what makes Task 5.1.2b's planned
  `errors.Is(err, session.ErrNotFound)` → `ErrItemNotFound` mapping actually fire instead
  of falling through to `ErrInternalError`.
- `ListActivityNotesForItem(ctx, itemID string) ([]ActivityNoteData, error)` returns all
  notes for an item ordered `Asc(created_at)`.
**Files**: `session/ent_repository_backlog.go`

##### Task 2.1.2a: Write `AppendActivityNote` (~8 min — revised up from ~5 min for the two fixes below)
- Add immediately after `ListProgressNotesForItem` (ends at line 2071, confirmed live),
  in a new `// --- Activity note history (ADR-001) ---` section.
- Body, in order:
  1. Parse `itemID` to UUID.
  2. **[Blocker 2 fix]** Read the item's filter-relevant scalar fields only —
     `r.client.BacklogItem.Query().Where(backlogitem.ID(parsedItemID)).Select(backlogitem.FieldStatus, backlogitem.FieldRepoPath).Only(ctx)`
     — into a local `*ent.BacklogItem` with just those two fields populated. If this read
     itself returns `ent.IsNotFound`, short-circuit and return `fmt.Errorf("backlog item
     %s not found: %w", itemID, ErrNotFound)` immediately (cheaper and clearer than
     waiting for the `Create()` FK violation below, and covers the same "item doesn't
     exist" case).
  3. `r.client.BacklogActivityNote.Create().SetItemID(parsedItemID).SetMessage(message).SetAuthorSessionUUID(authorSessionUUID).SetAuthorSessionTitle(authorSessionTitle).Save(ctx)`.
  4. **[Concern 2 fix]** On error from step 3, call a small extracted helper
     `mapAppendActivityNoteCreateError(err error, itemID string) error`: checks
     `ent.IsConstraintError(err)` first (guards the race where the item was deleted
     between steps 2 and 3) — if true, wraps as `session.ErrNotFound`; otherwise wraps as
     a plain `fmt.Errorf("failed to append activity note for item %s: %w", itemID, err)`.
     Extracting this as its own top-level function (rather than inlining the branch) is
     what makes Gap 2's unit test (Task 8.2.1d below) possible without needing to
     reproduce the actual delete-between-steps-2-and-3 race in a test — the race is
     real but non-deterministic to trigger; the error-mapping logic it depends on is not,
     and testing that in isolation is what actually proves the branch is wired correctly.
  5. Wrap the created row via `activityNoteToData`.
  6. **[Blocker 2 fix]** Call
     `r.publishItemChangedSnapshot(&BacklogItemData{ID: itemID, Status: itemFields.Status, RepoPath: itemFields.RepoPath}, BacklogItemChange{Kind: ChangeActivityNoteAdded, ActivityNote: &note})`
     — still deliberately `publishItemChangedSnapshot` (no full eager-load), not the
     `publishItemChanged` convenience wrapper: this event's payload never carries a full
     item snapshot (ADR-002), so there is nothing for `attachItemSessionsForPublish` to
     usefully eager-load beyond the two scalar fields already fetched in step 2.
- Doc comment: explicitly note this is the one activity-note write path invoked by the
  ungated `post_backlog_update` tool, that (unlike `AppendProgressNote`) it DOES publish
  an event because there is no separate criterion-status write alongside it to carry that
  responsibility, and that the `Status`/`RepoPath` read in step 2 exists solely so
  `backlogItemMatchesFilters` doesn't silently drop this event for `WatchBacklogItems`
  callers with a non-empty filter (e.g. `web-app/src/app/review-queue/page.tsx`'s
  `statusFilter`) — see Blocker 2 in `implementation/adversarial-review.md`.
- Files: `session/ent_repository_backlog.go`

##### Task 2.1.2b: Write `ListActivityNotesForItem` (~3 min)
- Mirror `ListProgressNotesForItem` exactly: parse UUID, query
  `r.client.BacklogActivityNote.Query().Where(backlogactivitynote.HasItemWith(backlogitem.ID(parsedItemID))).Order(ent.Asc(backlogactivitynote.FieldCreatedAt)).All(ctx)`,
  map each via `activityNoteToData`.
- Files: `session/ent_repository_backlog.go`

#### Story 2.1.3: `Storage` wrapper methods
**As a** developer, **I want** `*Storage` passthrough methods, **so that** MCP handlers
(which hold a `*session.Storage`, not an `*EntRepository`) can call them.
**Acceptance Criteria**: `Storage.AppendActivityNote` and
`Storage.ListActivityNotesForItem` exist, type-asserting to `*EntRepository` and
returning a clear error otherwise, mirroring `Storage.AppendProgressNote`/
`Storage.ListProgressNotesForItem` exactly.
**Files**: `session/storage.go`

##### Task 2.1.3a: Add the two `Storage` wrapper methods (~3 min)
- In `session/storage.go`, immediately after `ListProgressNotesForItem` (around line
  1334), add `AppendActivityNote` and `ListActivityNotesForItem`, each doing the
  `er, ok := s.repo.(*EntRepository)` type-assert-and-forward pattern used by their
  progress-note siblings.
- Files: `session/storage.go`

#### Story 2.1.4: Eager-load on `GetBacklogItem` + `BacklogItemData` field
**As a** reader of `get_backlog_item`/`GetBacklogItem`, **I want** the full activity log
returned on every fetch, **so that** "read the log back" works without a separate RPC.
**Acceptance Criteria**: `EntRepository.GetBacklogItem` eager-loads
`activity_notes` ordered by `created_at`; `BacklogItemData` gains an
`ActivityNotes []ActivityNoteData` field populated by `backlogItemToData`.
**Files**: `session/ent_repository_backlog.go`, `session/repository.go` (or wherever
`BacklogItemData` is defined — confirm in Task 2.1.4a).

##### Task 2.1.4a: Add `ActivityNotes` field to `BacklogItemData` + populate in `backlogItemToData` (~4 min)
- Locate `BacklogItemData`'s struct definition (same file as `backlogItemToData`,
  `session/ent_repository_backlog.go` — confirm exact line when editing) and add
  `ActivityNotes []ActivityNoteData` next to the existing `ProgressNotes` field.
- In `backlogItemToData` (line 178), populate `ActivityNotes` from
  `item.Edges.ActivityNotes` the same way `ProgressNotes` is populated from
  `item.Edges.ProgressNotes` (mapping each through `activityNoteToData`).
- Files: `session/ent_repository_backlog.go`

##### Task 2.1.4b: Add `WithActivityNotes` eager-load to `GetBacklogItem` (~3 min)
- In `EntRepository.GetBacklogItem` (`session/ent_repository_backlog.go:337-361`), add
  `.WithActivityNotes(func(q *ent.BacklogActivityNoteQuery) { q.Order(ent.Asc(backlogactivitynote.FieldCreatedAt)) })`
  alongside the existing `.WithProgressNotes(...)` call.
- Files: `session/ent_repository_backlog.go`

---

## Phase 3: Proto + Codegen

### Epic 3.1: New proto messages
**Goal**: `BacklogActivityNote` and `BacklogItemActivityNoteAddedEvent` exist on the
wire, generated into both Go and TypeScript bindings.

#### Story 3.1.1: `BacklogActivityNote` message + `BacklogItem.activity_notes = 35`
**As a** client of `GetBacklogItem`, **I want** the full activity log in the response,
**so that** the web UI and any MCP caller can render it without a second call.
**Acceptance Criteria**: `proto/session/v1/backlog.proto` defines
`message BacklogActivityNote { string id; string message; string author_session_uuid;
string author_session_title; google.protobuf.Timestamp created_at; }` and adds
`repeated BacklogActivityNote activity_notes = 35;` to `BacklogItem`.
**Files**: `proto/session/v1/backlog.proto`

##### Task 3.1.1a: Add the `BacklogActivityNote` message (~2 min)
- In `proto/session/v1/backlog.proto`, immediately after the `BacklogProgressNote`
  message (line 147), add the new message with fields numbered 1-5 in the order given
  above, with a doc comment explaining it's the ungated sibling log (cross-reference
  ADR-001 by filename in the comment, not by full path).
- Files: `proto/session/v1/backlog.proto`

##### Task 3.1.1b: Add `activity_notes = 35` field to `BacklogItem` (~2 min)
- Immediately after `plan_rejected_at = 34;` (line 204), add
  `repeated BacklogActivityNote activity_notes = 35;` with a one-line comment noting it
  rides `GetBacklogItem`'s existing eager-load, same convention as `progress_notes`'s
  comment at line 179.
- Files: `proto/session/v1/backlog.proto`

#### Story 3.1.2: `BacklogItemActivityNoteAddedEvent` message + oneof field 10
**As a** live-stream consumer, **I want** a dedicated single-entry event message,
**so that** the wire event never carries a full item snapshot (ADR-002).
**Acceptance Criteria**: `message BacklogItemActivityNoteAddedEvent { string item_id;
BacklogActivityNote note; }` exists; `BacklogItemEvent`'s `oneof event` gains
`BacklogItemActivityNoteAddedEvent activity_note_added = 10;`.
**Files**: `proto/session/v1/backlog.proto`

##### Task 3.1.2a: Add the event message (~2 min)
- Immediately after `BacklogItemUpdatedEvent` (line 828), add
  `message BacklogItemActivityNoteAddedEvent { string item_id = 1; BacklogActivityNote note = 2; }`
  with a doc comment citing ADR-002: deliberately no `item`/`is_snapshot` field —
  every post is a genuine live event, there is no snapshot-replay concept for this kind
  since the full log already rides `GetBacklogItem`'s `activity_notes`.
- Files: `proto/session/v1/backlog.proto`

##### Task 3.1.2b: Add the oneof variant at field 10 (~2 min)
- In `BacklogItemEvent`'s `oneof event` (line 756), add
  `BacklogItemActivityNoteAddedEvent activity_note_added = 10;` as the last entry, with
  a one-line comment noting field 8 is `seq` (message-level, not in this oneof) and 9 is
  `snapshot_complete`, so 10 is next.
- Files: `proto/session/v1/backlog.proto`

#### Story 3.1.3: Regenerate
**As a** developer, **I want** `make proto-gen` run once after all proto edits land,
**so that** Go and TypeScript bindings exist for later phases.
**Acceptance Criteria**: `make proto-gen` succeeds; `gen/proto/go/session/v1/` and
`web-app/src/gen/session/v1/backlog_pb.ts` reflect the new messages/fields; `go build ./...`
still passes.
**Files**: `gen/proto/go/session/v1/` (generated), `web-app/src/gen/session/v1/` (generated)

##### Task 3.1.3a: Run `make proto-gen` and verify build (~4 min)
- Run `make proto-gen` from repo root (after Tasks 3.1.1a/b and 3.1.2a/b are both
  landed, so a single regen picks up every new message/field at once).
- Run `go build ./...`; spot-check the generated TS file contains
  `BacklogActivityNote` and `activityNoteAdded`.
- Files: generated only, no hand-edits.

### Epic 3.2: Go↔proto mapper extension
**Goal**: `GetBacklogItem`'s response actually carries the new field.

#### Story 3.2.1: Extend `backlogItemToProto`'s mapper
**As a** client, **I want** `activity_notes` populated on every `BacklogItem` proto
built from a `BacklogItemData` that has them, **so that** the eager-load in Phase 2
actually reaches the wire.
**Acceptance Criteria**: `server/services/backlog_service.go`'s item-mapper function
populates `p.ActivityNotes` from `item.ActivityNotes` whenever non-empty, mirroring the
existing `ProgressNotes` block exactly.
**Files**: `server/services/backlog_service.go`

##### Task 3.2.1a: Add the `activity_notes` mapping block (~3 min)
- In `server/services/backlog_service.go`, immediately after the existing
  `if len(item.ProgressNotes) > 0 { ... p.ProgressNotes = protoNotes }` block (lines
  785-796), add the equivalent block for `item.ActivityNotes` →
  `p.ActivityNotes`, constructing each `*sessionv1.BacklogActivityNote{Id, Message,
  AuthorSessionUuid, AuthorSessionTitle, CreatedAt: timestamppb.New(...)}`.
- Files: `server/services/backlog_service.go`

---

## Phase 4: Event Bus Wiring (Go)

### Epic 4.1: New `BacklogChangeKind` across all three touch points
**Goal**: `ChangeActivityNoteAdded` exists and is mapped correctly end-to-end — the
deviation found during validation (three files, not two) is fully addressed here.

#### Story 4.1.1: `session.BacklogChangeKind` + `BacklogItemChange.ActivityNote`
**As a** repository method, **I want** a `ChangeActivityNoteAdded` kind and an
`ActivityNote` field to populate, **so that** `AppendActivityNote` (Task 2.1.2a) can
describe its own mutation.
**Acceptance Criteria**: `session/backlog_item_change.go` gains
`ChangeActivityNoteAdded BacklogChangeKind = "activity_note_added"` and
`BacklogItemChange.ActivityNote *ActivityNoteData`.
**Files**: `session/backlog_item_change.go`

##### Task 4.1.1a: Add the constant and struct field (~3 min)
- Add `ChangeActivityNoteAdded` to the `const` block (after `ChangeTriageProgressUpdated`,
  line 29), with a doc comment: "emitted by AppendActivityNote (ADR-001's sibling
  table) — carries only the new note via ActivityNote, never OldStatus/NewStatus/etc."
- Add `ActivityNote *ActivityNoteData` field to `BacklogItemChange` (after `Verdict`,
  line 63), doc-commented "populated only when Kind == ChangeActivityNoteAdded."
- Files: `session/backlog_item_change.go`

#### Story 4.1.2: `events.BacklogChangeKind` + `BacklogItemEventPayload.ActivityNote`
**As a** cross-package adapter, **I want** the mirrored wire-side enum value and
payload field, **so that** the adapter (Story 4.1.4) has something to convert into.
**Acceptance Criteria**: `pkg/events/types.go` gains
`BacklogChangeActivityNoteAdded BacklogChangeKind = "activity_note_added"` and
`BacklogItemEventPayload.ActivityNote *session.ActivityNoteData`.
**Files**: `pkg/events/types.go`

##### Task 4.1.2a: Add the constant and payload field (~3 min)
- Mirror Task 4.1.1a exactly: add `BacklogChangeActivityNoteAdded` to the `const` block
  (after `BacklogChangeTriageProgressUpdated`, line 53) and `ActivityNote
  *session.ActivityNoteData` to `BacklogItemEventPayload` (after `Verdict`, line ~83).
- Files: `pkg/events/types.go`

#### Story 4.1.3: `server/events/forward.go` alias export (the deviation fix)
**As a** `server/services` file importing the `server/events` alias package, **I want**
the new constant re-exported, **so that** `mapBacklogChangeKind` (which lives in
`server/services` and imports `server/events`, not `pkg/events` directly) can reference
it.
**Acceptance Criteria**: `server/events/forward.go`'s `BacklogChangeKind constants`
block includes `BacklogChangeActivityNoteAdded = pkgevents.BacklogChangeActivityNoteAdded`.
**Files**: `server/events/forward.go`

##### Task 4.1.3a: Add the alias constant (~2 min)
- In `server/events/forward.go`'s second `const` block (lines 30-38), add
  `BacklogChangeActivityNoteAdded = pkgevents.BacklogChangeActivityNoteAdded` as the
  last entry.
- Files: `server/events/forward.go`

#### Story 4.1.4: `mapBacklogChangeKind` switch case
**As a** publisher, **I want** the explicit switch to know about the new kind, **so
that** an unmapped-kind panic never fires for a legitimate activity-note post.
**Acceptance Criteria**: `mapBacklogChangeKind` in
`server/services/backlog_item_event_publisher.go` has a
`case session.ChangeActivityNoteAdded: return events.BacklogChangeActivityNoteAdded`
branch; `PublishItemChanged`'s payload construction copies `change.ActivityNote` through.
**Files**: `server/services/backlog_item_event_publisher.go`

##### Task 4.1.4a: Add the switch case and payload field copy (~3 min)
- In `mapBacklogChangeKind` (line 64), add the new case immediately after
  `case session.ChangeTriageProgressUpdated:` (line 78).
- In `PublishItemChanged`'s payload literal (lines 43-54), add
  `ActivityNote: change.ActivityNote,` alongside the existing field copies.
- Files: `server/services/backlog_item_event_publisher.go`

### Epic 4.2: `convertEventToBacklogItemEvent` switch case
**Goal**: The wire event actually gets built with a non-nil oneof for this kind — the
sharpest risk pitfalls.md names (a silently-empty oneof) is closed here.

#### Story 4.2.1: New case building `BacklogItemEvent_ActivityNoteAdded`
**As a** `WatchBacklogItems` subscriber, **I want** a real event delivered, **so that**
the frontend has something to dispatch.
**Acceptance Criteria**: `convertEventToBacklogItemEvent`
(`server/services/backlog_service_events.go`) gains a
`case events.BacklogChangeActivityNoteAdded:` building
`&sessionv1.BacklogItemEvent_ActivityNoteAdded{ActivityNoteAdded: &sessionv1.BacklogItemActivityNoteAddedEvent{ItemId: itemID, Note: activityNoteDataToProto(payload.ActivityNote)}}`
— never touching `protoItem` (ADR-002).
**Files**: `server/services/backlog_service_events.go`

##### Task 4.2.1a: Add the switch case (~4 min)
- In `convertEventToBacklogItemEvent`'s switch (lines 274-333), add the new case
  immediately after the `events.BacklogChangeItemUpdated, events.BacklogChangeTriageProgressUpdated`
  case (after line 319), before `events.BacklogChangeItemArchived`.
- Write a small local helper `activityNoteDataToProto(n *session.ActivityNoteData) *sessionv1.BacklogActivityNote`
  in the same file (nil-safe: return nil if `n == nil`), reusing the same field mapping
  as Task 3.2.1a's inline construction (or factor Task 3.2.1a's block into this shared
  helper and call it from both places — prefer the shared helper to avoid drift between
  the two call sites).
- Files: `server/services/backlog_service_events.go`

### Epic 4.3: Repository publish call (cross-reference only)
**Goal**: Confirm Task 2.1.2a's `publishItemChangedSnapshot` call now has everything it
needs (the `BacklogChangeKind` from Epic 4.1) to compile and actually deliver an event.

#### Story 4.3.1: Build verification
**As a** developer, **I want** `go build ./...` to pass with Phase 2 + Phase 4 both
landed, **so that** the full Go-side chain (insert → publish → convert) type-checks.
**Acceptance Criteria**: `go build ./...` passes with no changes needed to Task 2.1.2a's
code (it was written against the `ChangeActivityNoteAdded` name Epic 4.1 defines).
**Files**: none new — verification task only.

##### Task 4.3.1a: Build and smoke-check (~2 min)
- Run `go build ./...` from repo root once both Phase 2 and Phase 4 are landed.
- Files: none.

### Epic 4.4: Exclude `ChangeActivityNoteAdded` from `wait_for_backlog_event` [Blocker 3 fix]
**Goal**: `wait_for_backlog_event` — a tool whose own doc comment says it exists to
replace polling loops waiting on "a review verdict or status change" — never wakes a
blocked caller for a purely informal `post_backlog_update` call, regardless of the
caller's `event_type` filter (including the default `"any"`).

#### Story 4.4.1: Early-skip in the event-matching loop
**As a** session blocked in `wait_for_backlog_event`, **I want** activity-note events to
be structurally unable to satisfy my wait, **so that** I never mistake an informal note
for the status/verdict change I'm actually waiting on.
**Acceptance Criteria**:
- Verified live: `waitForBacklogEvent`'s event-matching loop is
  `server/mcp/tools_backlog.go:630-665` (the function itself spans 577-666; the review's
  cited "435-666" also includes the preceding `eventTypeAny`-family consts and helper
  functions at 435-576, not just the loop). The kind-filter switch
  (`backlogEventKindFilterValue`) is at 470-487; the match logic the review cited is
  exactly right at 652-663.
- A new `case events.BacklogChangeActivityNoteAdded: continue` (or an equivalent explicit
  `if` guard) is added to the loop **before** the existing `payload.Item == nil ||
  payload.Item.ID != itemID` check (line 656) and the `eventTypeFilter` comparison (line
  660) — not folded into `backlogEventKindFilterValue`'s existing `default: return
  string(kind)` case, since that default is exactly the mechanism that currently lets
  this kind silently pass every filter including `"any"`.
- Existing behavior for every other kind (`verdict_recorded`, `status_changed`,
  `item_archived`, `item_removed`, `item_updated`, `session_attached`) is unchanged — this
  is a targeted exclusion, not a rewrite of the matching logic.
**Files**: `server/mcp/tools_backlog.go`

##### Task 4.4.1a: Add the early-skip (~3 min)
- In `waitForBacklogEvent`'s `for { select { ... case evt, ok := <-eventCh: ... } }` loop,
  immediately after `payload := evt.BacklogItemPayload` (line 655), add:
  ```go
  if payload.Kind == events.BacklogChangeActivityNoteAdded {
  	continue
  }
  ```
- Add a doc comment on this line (or immediately above the loop) explaining why: an
  activity note is an informal, ungated comment, never an official status/verdict signal
  this tool exists to replace polling for — see Blocker 3 in
  `implementation/adversarial-review.md`.
- Files: `server/mcp/tools_backlog.go`

---

## Phase 5: MCP Tool

### Epic 5.1: `post_backlog_update` tool
**Goal**: Any session can call `post_backlog_update` and have it durably recorded,
regardless of `STAPLER_SESSION_UUID` presence or item linkage.

#### Story 5.1.1: Identity-resolution helpers on `backlogHandlers`
**As a** tool handler, **I want** `session_id`-param resolution and best-effort
UUID→title lookup, **so that** provenance is captured without ever failing the call.
**Acceptance Criteria**: `backlogHandlers` gains `findInstanceByID` (mirrors
`goalHandlers.findInstanceByID`, `tools_goal.go:371-382`, erroring only on a store
load failure or unmatched `session_id`) and `findInstanceByUUID` (mirrors
`goalHandlers.findInstanceByUUID`, `tools_goal.go:384-397`, returns `nil` on any
failure — never used to reject a call).
**Files**: `server/mcp/tools_backlog.go`

##### Task 5.1.1a: Add `findInstanceByID` and `findInstanceByUUID` to `backlogHandlers` (~5 min)
- Add both methods to `server/mcp/tools_backlog.go`, placed near the top of the file
  alongside the other identity helpers (`callerSessionUUID`/`callerSessionUUIDForAudit`,
  lines 46-70), copying `tools_goal.go:371-397`'s bodies verbatim but on the
  `*backlogHandlers` receiver (`h.store.LoadInstances()` — `backlogHandlers` already
  has a `store session.InstanceStore` field, no new dependency needed).
- Files: `server/mcp/tools_backlog.go`

#### Story 5.1.2: Handler implementation
**As a** caller, **I want** `postBacklogUpdate` to validate, resolve identity, and
persist, **so that** the tool actually does what its description promises.
**Acceptance Criteria**:
- `item_id` required, validated as UUID (`validateUUID`, matching every other tool).
- `message` required; rejected (INVALID_ARGUMENT) if empty after `strings.TrimSpace`,
  or if `len(message) > 2000` (mirroring `request_review`'s cap,
  `tools_backlog.go:888`).
- **[Concern 1 fix]** Order of operations, per requirements.md's literal text
  ("Sanitizes/validates the free-form text before persisting"): (1) trim whitespace,
  (2) reject if empty, (3) reject if raw length > 2000 chars, (4) strip HTML tags via
  `session.SanitizeForAgentContext(trimmed, 2000)` (confirmed signature
  `SanitizeForAgentContext(s string, maxLen int) string`, `session/backlog_context.go:16` —
  calling it with `maxLen=2000` only strips tags, since the message was already validated
  ≤2000 chars in step 3 and `sanitizeField` only truncates when `len(s) > maxLen`), (5)
  persist the stripped text. This is in addition to, not instead of, Task 5.2.1a's
  existing render-time truncation-to-500-chars for the `get_backlog_item` text envelope.
- `session_id` optional; if given, resolved via `findInstanceByID` (its own error
  propagated as-is, matching `set_session_goal`'s pattern).
- If `session_id` absent: identity comes from `callerSessionUUIDForAudit(ctx)` (never
  errors); if that returns something other than `"manual"`, best-effort resolve a title
  via `findInstanceByUUID` (empty title on a miss — never fails the call).
- **No `GetItemSessionBySessionAndItem` call anywhere in this handler** — this is the
  one deliberate difference from every gated tool, and the whole point of the feature.
- **No item-status check** — posting is allowed regardless of the item's current status
  (matches every other write path's behavior per pitfalls.md's "not checked, presumably
  allowed" finding).
- Calls `h.storage.AppendActivityNote(ctx, itemID, authorUUID, authorTitle, sanitizedMessage)`;
  maps `session.ErrNotFound` to `ErrItemNotFound`, anything else to `ErrInternalError`.
  **[Concern 2 fix]** This mapping only actually fires because Task 2.1.2a's
  `AppendActivityNote` now wraps an FK-violation `Create()` error as `session.ErrNotFound`
  via `ent.IsConstraintError` — without that repository-layer fix, this `errors.Is` check
  would never match and every nonexistent-`item_id` call would fall through to the
  `ErrInternalError` branch with a raw SQL/ent message.
- Logs via `%q`-quoted `log.InfoLog.Printf` (matching `request_review`'s log-injection
  precedent, `tools_backlog.go:978`), never `%s` for the raw message.
**Files**: `server/mcp/tools_backlog.go`

##### Task 5.1.2a: Write `postBacklogUpdate`'s validation + identity resolution (~6 min — revised up from ~5 min for the sanitize-before-persist fix)
- Add the function in `server/mcp/tools_backlog.go`, placed near
  `getBacklogItem`/`createBacklogItem` for locality (e.g. immediately before
  `create_backlog_item`'s section, around line 1730).
- Implement `item_id` validation, `message` trim/length checks, and the
  `session_id`-param-else-`callerSessionUUIDForAudit`-plus-best-effort-title resolution
  described in the story's acceptance criteria.
- **[Concern 1 fix]** After the length check and before calling
  `h.storage.AppendActivityNote`, apply `sanitized := session.SanitizeForAgentContext(trimmed, 2000)`
  and pass `sanitized` (not the raw trimmed message) to `AppendActivityNote` in Task
  5.1.2b — persist-time sanitization, not just Task 5.2.1a's render-time truncation.
- Files: `server/mcp/tools_backlog.go`

##### Task 5.1.2b: Wire the storage call, error mapping, and result (~4 min)
- Complete `postBacklogUpdate`: call `h.storage.AppendActivityNote(...)`, map errors,
  log with `%q`, return an `okResult` with the new note's id/created_at (small result
  struct, e.g. `PostBacklogUpdateResult{MCPResult; NoteID string; CreatedAt string}`).
- Files: `server/mcp/tools_backlog.go`

#### Story 5.1.3: Tool registration and description
**As an** LLM caller, **I want** the tool's description to state plainly that it's
ungated and non-authoritative, **so that** pitfalls.md's confusability risk is
mitigated at the source.
**Acceptance Criteria**: `registerBacklogTools` registers `post_backlog_update` with
`item_id` (required), `message` (required), `session_id` (optional) parameters, and a
description that (a) states "Not role/item-gated — callable from any session, with or
without STAPLER_SESSION_UUID, whether or not it is linked to this item" (mirroring
`create_backlog_item`'s convention, `tools_backlog.go:2524`), and (b) states "This is an
informal note, not an official verdict or progress mark — it never changes item status,
AC-criterion state, or review verdicts."
**Files**: `server/mcp/tools_backlog.go`

##### Task 5.1.3a: Register the tool (~3 min)
- In `registerBacklogTools` (inside the file's tool-registration block, e.g. near
  `create_backlog_item`'s registration around line 2523), add the
  `s.AddTool(mcpgo.NewTool("post_backlog_update", ...), h.postBacklogUpdate)` block with
  the description and parameters from the story's acceptance criteria.
- Files: `server/mcp/tools_backlog.go`

### Epic 5.2: `get_backlog_item` rendering
**Goal**: The text envelope shows the activity log in its own clearly-labeled section,
sanitized and capped, never confusable with the verdict section.

#### Story 5.2.1: `## Activity Log` section
**As an** agent reading `get_backlog_item`'s output, **I want** a distinct, capped,
sanitized activity-log section, **so that** I can see prior notes without unbounded
context growth or confusion with official verdicts.
**Acceptance Criteria**:
- New section heading `"## Activity Log"`, placed after `"## Latest Review Verdict"`
  and before the role-aware workflow guidance block (`tools_backlog.go`'s existing
  section order, ~line 365).
- Each entry rendered as `"- note from %s at %s: %s"` where the first `%s` is the
  author's title (falling back to the raw UUID if title is empty, falling back to
  `"manual"` if there's no UUID at all), the second is a formatted timestamp, and the
  third is `session.SanitizeForAgentContext(message, 500)`.
- Caps rendering at the **last 20 entries** (`maxContextExtrasEntries`-style constant,
  matching `session/backlog_review.go:107-114`'s cap), appending
  `"(N older entries not shown)"` when truncated. Storage itself stays unbounded.
- Never reuses the `"## Latest Review Verdict"` heading, the `"Outcome:"` line format,
  or the `"Criterion N (...)"` line format (pitfalls.md's confusability mitigation).
**Files**: `server/mcp/tools_backlog.go`

##### Task 5.2.1a: Fetch and render the section (~5 min)
- In `getBacklogItem` (`server/mcp/tools_backlog.go:290-`), after the existing "Latest
  Review Verdict" block (ends ~line 365) and before the role-aware guidance switch, add
  a new block: call `h.storage.ListActivityNotesForItem(ctx, itemID)` (best-effort —
  log a warning and skip the section on error, never fail the whole call), then render
  up to the last 20 entries (most recent first or oldest first — match
  `ListActivityNotesForItem`'s natural `Asc(created_at)` order for consistency with
  `ProgressHistorySection`'s oldest-first UI convention) with the format above.
- Define the 20-entry cap as a small named constant near the function (e.g.
  `maxActivityLogEntriesRendered = 20`), not a bare magic number.
- Files: `server/mcp/tools_backlog.go`

---

## Phase 6: Frontend

### Epic 6.1: Domain mapper chain
**Goal**: `BacklogItem.activityNotes` flows from the generated proto type through to the
domain type components consume.

#### Story 6.1.1: `useBacklogService.ts` proto→domain mapping
**As a** React component, **I want** a domain `ActivityNote` type and mapper, **so
that** I never touch the raw proto shape directly.
**Acceptance Criteria**: `useBacklogService.ts` imports
`BacklogActivityNote as BacklogActivityNoteProto`, defines `ActivityNote {id, message,
authorSessionUuid, authorSessionTitle, createdAt?}`, defines `mapActivityNote`, adds
`activityNotes: ActivityNote[]` to the `BacklogItem` domain interface, and populates it
in `mapBacklogItem` from `p.activityNotes`.
**Files**: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 6.1.1a: Add the proto import, interface, and mapper function (~4 min)
- In `web-app/src/lib/hooks/useBacklogService.ts`, add the proto import next to
  `BacklogProgressNote as BacklogProgressNoteProto` (line 17), add the `ActivityNote`
  interface next to `ProgressNote` (line 284), and add `mapActivityNote` next to
  `mapProgressNote` (line 419), following its exact field-by-field shape (including the
  same `n.createdAt ? new Date(...).toISOString() : undefined` timestamp conversion).
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 6.1.1b: Wire the field into the `BacklogItem` interface and `mapBacklogItem` (~3 min)
- Add `activityNotes: ActivityNote[]` to the `BacklogItem` domain interface (next to
  `progressNotes: ProgressNote[]`, line 171).
- In `mapBacklogItem` (line 456), add
  `activityNotes: (p.activityNotes ?? []).map(mapActivityNote),` next to the existing
  `progressNotes: (p.progressNotes ?? []).map(mapProgressNote),` line (534).
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

### Epic 6.2: Live-update wiring (Redux + watch stream)
**Goal**: A new note posted anywhere shows up live, without a whole-item clobber risk.

#### Story 6.2.1: `appendActivityNote` reducer
**As a** Redux store, **I want** a targeted single-note append action, **so that** a
live activity-note event never replaces `state.items[id]` wholesale (ADR-002).
**Acceptance Criteria**: `backlogItemsSlice.ts` gains an `appendActivityNote` reducer
that, given `{itemId, note}`, appends `note` to `state.items[itemId].activityNotes`
(no-op if the item isn't in the store yet — matches `removeItem`'s existing "item not
present" tolerance) via a shallow-copied array, never touching any other field.
**Files**: `web-app/src/lib/store/backlogItemsSlice.ts`

##### Task 6.2.1a: Add the `appendActivityNote` reducer and export (~4 min)
- In `web-app/src/lib/store/backlogItemsSlice.ts`'s `reducers` object, add
  `appendActivityNote(state, action: PayloadAction<{ itemId: string; note: BacklogActivityNote }>)`
  next to `upsertItem`/`removeItem` (lines 48-80): if `state.items[itemId]` exists,
  replace it with `{ ...existing, activityNotes: [...(existing.activityNotes ?? []), note] }`;
  otherwise no-op (there's no full item to attach the note to yet — the next
  `GetBacklogItem`/`ListBacklogItems` refresh will include it).
- Export it alongside `upsertItem`/`removeItem` (line 84).
- Import the generated `BacklogActivityNote` proto type at the top of the file next to
  the existing `BacklogItem` type import (line 3).
- Files: `web-app/src/lib/store/backlogItemsSlice.ts`

#### Story 6.2.2: `useWatchBacklogItems.ts` new switch case
**As a** connected client, **I want** the new `activityNoteAdded` event case handled
explicitly, **so that** pitfalls.md's "silently swallowed unhandled case" risk is closed
for this specific kind.
**Acceptance Criteria**: `useWatchBacklogItems.ts`'s switch on `event.event.case` gains
`case "activityNoteAdded": dispatch(appendActivityNote({ itemId: event.event.value.itemId, note: event.event.value.note }))`
placed before the existing `default: break;` (line 327).
**Files**: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

##### Task 6.2.2a: Add the switch case and import (~3 min)
- Add `import { ..., appendActivityNote } from "@/lib/store/backlogItemsSlice";` to the
  existing import (wherever `upsertItem`/`removeItem` are imported).
- Add the new `case "activityNoteAdded":` immediately before the `default: break;`
  (currently line 327), guarding on `event.event.value.note` being truthy before
  dispatching (mirror the `if (item) dispatch(...)` null-guard style used by the
  `itemUpdated`/`statusChanged` cases, lines 303-310).
- Files: `web-app/src/lib/hooks/useWatchBacklogItems.ts`

#### Story 6.2.3: Extend `upsertItem`'s anti-clobber guard to cover `activityNotes` [Blocker 1 fix]
**As a** connected client, **I want** a status transition, verdict, session-attach, or any
other item-level event to never wipe out previously-received activity notes, **so that**
ADR-002's "structurally cannot inherit the `progressNotes` clobber-risk class" claim is
actually true for `activityNotes` too — not just for the new `activityNoteAdded` event
itself, but for every *other* kind, which still publishes via `publishItemChanged` →
`attachItemSessionsForPublish` (`session/ent_repository_backlog.go:1600-1602,1721-1731`,
which re-populates only `ItemSessions`, never `ActivityNotes`) and therefore embeds a full
`protoItem` (`convertEventToBacklogItemEvent`, `server/services/backlog_service_events.go:274-319`)
whose `activity_notes` field is empty.
**Acceptance Criteria**:
- Verified live: `upsertItem`'s reducer body is
  `web-app/src/lib/store/backlogItemsSlice.ts:48-72`; the existing `itemSessions`
  anti-clobber guard (comment at 55-61, logic at 62-67) reads:
  ```ts
  const nextItem =
    existing &&
    (existing.itemSessions?.length ?? 0) > 0 &&
    (incoming.itemSessions?.length ?? 0) === 0
      ? { ...incoming, itemSessions: existing.itemSessions }
      : incoming;
  ```
- The guard is extended to also preserve `activityNotes` under the identical condition
  (existing has entries, incoming's `activityNotes` is empty/absent) — same shape, same
  file, no new pattern invented. Any other field on `incoming` is left untouched (the
  guard patches only `itemSessions`/`activityNotes`, nothing else).
- A wholesale item-replace event (e.g. a status transition's `ItemUpdated`/`StatusChanged`
  wire event, which carries a full `protoItem` with an empty `activity_notes`) no longer
  wipes an item's previously-stored `activityNotes` in the Redux store.
**Files**: `web-app/src/lib/store/backlogItemsSlice.ts`

##### Task 6.2.3a: Extend the guard (~4 min)
- In `upsertItem`'s reducer body, replace the single `itemSessions`-only ternary with a
  small patch-object built from both independent guards, e.g.:
  ```ts
  let nextItem = incoming;
  if (existing) {
    const patch: Partial<BacklogItem> = {};
    if ((existing.itemSessions?.length ?? 0) > 0 && (incoming.itemSessions?.length ?? 0) === 0) {
      patch.itemSessions = existing.itemSessions;
    }
    if ((existing.activityNotes?.length ?? 0) > 0 && (incoming.activityNotes?.length ?? 0) === 0) {
      patch.activityNotes = existing.activityNotes;
    }
    if (Object.keys(patch).length > 0) {
      nextItem = { ...incoming, ...patch };
    }
  }
  ```
  (Exact variable names/structure at implementation time may differ; the requirement is
  that both guards fire independently, mirroring the existing `itemSessions` guard's
  logic/shape, not a new pattern.)
- Update the existing guard's doc comment (lines 55-61) to describe both fields it now
  protects, keeping the existing rationale ("a partially-loaded event push racing a
  fully-loaded one") and extending it to name `activityNotes` alongside `itemSessions`.
- Files: `web-app/src/lib/store/backlogItemsSlice.ts`

### Epic 6.3: UI component
**Goal**: `BacklogItemDetail` renders the activity log in its own section.

#### Story 6.3.1: `ActivityLogSection.tsx`
**As a** user viewing a backlog item, **I want** to see the activity log, **so that**
I can read notes left by any session.
**Acceptance Criteria**: `web-app/src/components/backlog/detail/ActivityLogSection.tsx`
exists, structurally cloned from `ProgressHistorySection.tsx` (collapsible via
`CollapsibleSection`, `useShowMore` capped at 8, renders `item.activityNotes`), with a
`// +feature: backlog-activity-log` marker in its first 10 lines, and renders each entry
as `"<author title or UUID or 'manual'> · <timestamp>"` plus the message — never reusing
`ProgressHistorySection`'s "Criterion #N · status" meta-line format.
**Files**: `web-app/src/components/backlog/detail/ActivityLogSection.tsx` (new)

##### Task 6.3.1a: Write `ActivityLogSection.tsx` (~5 min)
- Create the file, copying `ProgressHistorySection.tsx`'s structure: `"use client"`
  directive, `// +feature: backlog-activity-log` marker near the top, `SHOW_MORE_CAP = 8`,
  `useShowMore(item.id, "activity-log", item.activityNotes, SHOW_MORE_CAP)`, early
  return `null` if `item.activityNotes.length === 0`, `CollapsibleSection
  sectionKey="activity-log" title="Activity Log"`, one `<div role="list"
  aria-label="Backlog item activity log">` per entry with `role="listitem"`, meta-line
  showing author (title, falling back to a truncated UUID, falling back to "manual")
  and `formatDate(n.createdAt)`, message text below.
- Files: `web-app/src/components/backlog/detail/ActivityLogSection.tsx`

#### Story 6.3.2: Wire into `BacklogItemDetail.tsx`
**As a** developer, **I want** the new section rendered on the detail page, **so that**
Story 6.3.1's component is actually reachable.
**Acceptance Criteria**: `BacklogItemDetail.tsx` imports `ActivityLogSection`, adds a
`activityLogExpanded`/`setActivityLogExpanded` pair via `useSectionExpandState(itemId,
"activity-log", false)`, adds `"activity-log"` to the existing section-key bookkeeping
array (line 424-425), and renders `<ActivityLogSection item={item}
defaultExpanded={activityLogExpanded} />` next to (not merged into)
`<ProgressHistorySection .../>` (line 1571).
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 6.3.2a: Import and wire the new section (~4 min)
- Add `import { ActivityLogSection } from "./detail/ActivityLogSection";` next to the
  `ProgressHistorySection` import (line 57).
- Add `const [activityLogExpanded, setActivityLogExpanded] = useSectionExpandState(itemId, "activity-log", false);`
  next to `progressHistoryExpanded` (line 364), and add its entry to the section-key
  array at lines 424-425.
- Render `<ActivityLogSection item={item} defaultExpanded={activityLogExpanded} />`
  immediately after `<ProgressHistorySection item={item} defaultExpanded={progressHistoryExpanded} />`
  (line 1571).
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

## Phase 7: Registry

### Epic 7.1: Feature registry regen
**Goal**: CI's registry-diff gate passes with the new frontend component registered.

#### Story 7.1.1: `make registry-generate`
**As a** CI pipeline, **I want** the new component's registry entry committed, **so
that** `build.yml`'s `git diff --exit-code docs/registry/features/` step doesn't fail.
**Acceptance Criteria**: `make registry-generate` produces a new
`docs/registry/features/frontend/backlog-activity-log.json` (or similar, scanner-derived
name) entry for `ActivityLogSection.tsx`; `make registry-diff` is clean afterward. No
backend registry entry is needed (no new RPC — `GetBacklogItem`'s existing handler is
unchanged in identity, only its response message grew a field; pitfalls.md's registry
finding confirms MCP tools aren't scanned at all).
**Files**: `docs/registry/features/frontend/` (generated)

##### Task 7.1.1a: Run regen and commit the diff (~3 min)
- Run `make registry-generate` from repo root after Epic 6.3 lands.
- Run `make registry-diff` to confirm clean.
- Files: `docs/registry/features/frontend/*.json` (generated, new file).

---

## Phase 8: Tests

### Epic 8.1: Ent schema test
**Goal**: The new table's basic shape (insert, cascade delete, index) is proven.

#### Story 8.1.1: `BacklogActivityNote` schema test
**Acceptance Criteria**: A test creates a `BacklogItem`, appends a
`BacklogActivityNote` via the generated ent client directly, confirms it's queryable,
and confirms deleting the parent `BacklogItem` cascade-deletes the note row.
**Files**: `session/ent_repository_backlog_test.go` (existing file — corrected per
validation.md's Correction #2: `session/ent/schema/*_test.go` is not an established
convention in this repo; the most similar existing cascade-delete test,
`TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted`, lives
here at line 587, using `repo.GetEntClient().<Type>.Query().Count(ctx)` to prove the row
is gone — follow that exact idiom).

##### Task 8.1.1a: Write the schema/cascade test (~5 min)
- `TestBacklogActivityNote_should_CascadeDelete_When_ParentItemDeleted`: create item,
  create note, delete item, assert note is gone (`ent.IsNotFound`).
- Files: same file as above.

### Epic 8.2: Repository append/list test
**Goal**: `AppendActivityNote`/`ListActivityNotesForItem` behave correctly, including
the event-publish side effect.

#### Story 8.2.1: Repository-level tests
**Acceptance Criteria**:
- `TestAppendActivityNote_should_PersistAndBeListable_When_Called`: append two notes,
  list, assert both present in `created_at` order.
- `TestAppendActivityNote_should_PublishActivityNoteAddedEvent_When_Called`: wire a
  test `ItemChangePublisher` double (mirroring existing repository test patterns for
  `UpdateAcCriterionStatus`'s publish assertion), append a note, assert exactly one
  `ChangeActivityNoteAdded` event was published with the correct `ActivityNote.Message`.
- **[Blocker 2 fix test]**
  `TestAppendActivityNote_should_PopulateStatusAndRepoPathOnPublishedSnapshot_When_Called`:
  create an item with a known non-default `Status` and `RepoPath`, append a note, assert
  the published event's `BacklogItemPayload.Item.Status`/`.RepoPath` match the item's
  actual values (not zero-value) — directly proving
  `backlogItemMatchesFilters` (`server/services/backlog_service_events.go:213-224`) would
  not drop this event for a `WatchBacklogItems` caller with a non-empty `status_filter`/
  `category_filter`.
**Files**: `session/ent_repository_backlog_test.go` (existing file — add tests here,
matching `TestAppendProgressNote`/`TestUpdateAcCriterionStatus`'s neighboring test
style).

##### Task 8.2.1a: Write the persist+list test (~5 min)
- Files: `session/ent_repository_backlog_test.go`

##### Task 8.2.1b: Write the publish-event test (~5 min)
- Files: `session/ent_repository_backlog_test.go`

##### Task 8.2.1c: Write the Status/RepoPath-on-snapshot test (~4 min) [Blocker 2 fix]
- Files: `session/ent_repository_backlog_test.go`

##### Task 8.2.1d: Write `TestGetBacklogItem_should_ReturnActivityNotes_When_ItemHasThem` (~4 min) [Gap 2 fix, validation.md]
- Half of validation.md's Gap 1 recommendation (the other half is Epic 8.7's
  proto-mapper test) — this half proves Task 2.1.4b's `WithActivityNotes(...)`
  eager-load actually populates `BacklogItemData.ActivityNotes`, a distinct code path
  from Epic 8.6's `get_backlog_item` rendering (which calls `ListActivityNotesForItem`
  directly and never exercises `GetBacklogItem`'s eager-load at all).
- Create an item, append notes via `AppendActivityNote`, call `GetBacklogItem`, assert
  the returned `BacklogItemData.ActivityNotes` is populated and ordered
  (`Asc(created_at)`, matching `ListActivityNotesForItem`'s own order).
- Files: `session/ent_repository_backlog_test.go`

##### Task 8.2.1e: Write `mapAppendActivityNoteCreateError` unit tests (~4 min) [Gap 1 fix, validation.md]
- The one FK-violation branch in Task 2.1.2a's step 4 (`ent.IsConstraintError(err)` →
  `session.ErrNotFound`) has no existing test path that reaches it: `TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist`
  (Task 8.3.1c below) only exercises step 2's `Select(...).Only(ctx)` short-circuit,
  never step 3's `Create()` call, because a nonexistent `item_id` is caught before
  `Create()` is ever attempted — the FK-violation branch only fires in the narrow race
  where the item is deleted *between* steps 2 and 3, which is impractical to trigger
  deterministically in a test. Since Task 2.1.2a extracts the error-mapping logic into
  its own function (`mapAppendActivityNoteCreateError(err error, itemID string) error`),
  test that function directly and in isolation instead of trying to reproduce the race:
  - `TestMapAppendActivityNoteCreateError_should_ReturnErrNotFound_When_ErrIsConstraintViolation`:
    construct or obtain a synthetic error satisfying `ent.IsConstraintError` (mirror
    however this repo's existing tests construct one — check
    `session/ent_repository_backlog_test.go`/`session/ent_pipeline_mode_repository_test.go`
    for a precedent of testing an `ent.IsConstraintError` branch; if none exists, the
    ent-generated `*sqlgraph.ConstraintError` type can be constructed directly), assert
    `errors.Is(result, session.ErrNotFound)`.
  - `TestMapAppendActivityNoteCreateError_should_WrapPlainError_When_ErrIsNotConstraintViolation`:
    pass a plain `errors.New("boom")`, assert the result wraps it (via `errors.Is`/
    `errors.Unwrap`) without being misclassified as `ErrNotFound`.
- Files: `session/ent_repository_backlog_test.go`

### Epic 8.3: MCP tool tests
**Goal**: `post_backlog_update` proven callable from every identity configuration the
feature exists to support, and proven to still validate input correctly.

#### Story 8.3.1: `postBacklogUpdate` handler tests
**Acceptance Criteria** (one test per case, table-driven where the shape allows per
`golang-testing` skill conventions):
- Session with `STAPLER_SESSION_UUID` matching the item's assigned session succeeds
  (mirrors a gated tool's happy path, but via the new tool).
- Session with `STAPLER_SESSION_UUID` NOT linked to the item still succeeds (the core
  behavior difference from every gated tool) — assert the persisted note's
  `AuthorSessionUUID` matches the caller's UUID.
- No `STAPLER_SESSION_UUID` at all (context has none) still succeeds — assert the
  persisted note's `AuthorSessionUUID` is empty/absent and no error is returned.
- `session_id` param provided (overriding context) resolves via `findInstanceByID` and
  is used for provenance instead of the context UUID.
- Empty message (and whitespace-only message) is rejected with `ErrInvalidArgument`.
- Message over 2000 chars is rejected with `ErrInvalidArgument`.
- **Regression guard**: a parallel test confirms `report_progress`/`request_review`
  called under the same "unlinked session" or "no session UUID" conditions still return
  `PERMISSION_DENIED` exactly as before (proves this feature didn't loosen the existing
  gates — requirements.md's explicit success metric).
- **[Concern 2 fix test]** `TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist`:
  call with a well-formed but nonexistent `item_id`, assert the result's error code is
  `ErrItemNotFound`, not `ErrInternalError` — proving `ent.IsConstraintError`
  detection in `AppendActivityNote` (Task 2.1.2a) and the handler's `errors.Is` mapping
  (Task 5.1.2b) work end to end.
- **[Concern 1 fix test]** `TestPostBacklogUpdate_should_StripHTMLTagsBeforePersisting_When_MessageContainsMarkup`:
  post a message containing an HTML tag (e.g. `"<script>alert(1)</script>hello"`), then
  read the note back via `ListActivityNotesForItem` and assert the **persisted** message
  has the tag stripped — proving sanitization happens before `Create()`, not only at
  `get_backlog_item` render time.
**Files**: `server/mcp/tools_backlog_test.go` (existing file — add near
`TestReportProgress`/`TestCreateBacklogItem`'s tests).

##### Task 8.3.1a: Write the six `postBacklogUpdate` success/validation cases (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 8.3.1b: Write the gated-tools-still-rejected regression test (~4 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 8.3.1c: Write the nonexistent-item_id → `ErrItemNotFound` test (~3 min) [Concern 2 fix]
- Files: `server/mcp/tools_backlog_test.go`

##### Task 8.3.1d: Write the persist-time sanitization test (~3 min) [Concern 1 fix]
- Files: `server/mcp/tools_backlog_test.go`

### Epic 8.4: Event-bus coverage tests (both Go switches)
**Goal**: Directly close pitfalls.md's "silently swallowed unhandled kind" risk with an
explicit assertion, not reliance on manual testing.

#### Story 8.4.1: `mapBacklogChangeKind` and `convertEventToBacklogItemEvent` coverage
**Acceptance Criteria**:
- `TestMapBacklogChangeKind_should_MapActivityNoteAdded_When_Called`: asserts
  `mapBacklogChangeKind(session.ChangeActivityNoteAdded) == events.BacklogChangeActivityNoteAdded`
  (no panic).
- **[Corrected per validation.md Correction #1]** A new `{name: "activity_note_added",
  ...}` table case, appended to the existing table-driven
  `TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries`
  (confirmed live at `server/services/backlog_service_events_test.go:563`, table literal
  closes at line 666) — NOT a new standalone test function. The case's `payload` sets
  `Kind: events.BacklogChangeActivityNoteAdded` and a populated `ActivityNote`; its
  `check` func asserts `ev.GetActivityNoteAdded()` is non-nil and its `ItemId`/`Note`
  fields match the payload — directly proving the oneof is never left empty for this
  kind, using the same `t.Run` subtest + `t.Parallel()` shape as every other case in the
  table (e.g. the `"item_archived"` case immediately before it).
**Files**: `server/services/backlog_item_event_publisher_test.go`,
`server/services/backlog_service_events_test.go` (both existing files).

##### Task 8.4.1a: Write the `mapBacklogChangeKind` test (~3 min)
- Files: `server/services/backlog_item_event_publisher_test.go`

##### Task 8.4.1b: Add the `activity_note_added` table case (~4 min — corrected per validation.md Correction #1)
- Append a new `{name: "activity_note_added", ...}` entry to the existing table literal
  inside `TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries`
  (`server/services/backlog_service_events_test.go:563`, table closes at line 666) —
  do NOT create a separate `TestConvertEventToBacklogItemEvent_should_BuildActivityNoteAddedVariant_...`
  function; that would fork the established one-table-per-kind pattern for no reason.
- Files: `server/services/backlog_service_events_test.go`

#### Story 8.4.2: `wait_for_backlog_event` never wakes on an activity-note event [Blocker 3 fix]
**As a** session blocked in `wait_for_backlog_event(event_type: "any")`, **I want** a
regression test proving an activity note can never be mistaken for the status/verdict
change I'm waiting on, **so that** Epic 4.4's early-skip is proven, not just implemented.
**Acceptance Criteria**:
- `TestWaitForBacklogEvent_should_NotWake_When_ActivityNoteAddedFires`: start
  `waitForBacklogEvent` with `event_type: "any"` in a goroutine (using
  `testAfterWaitSubscribeHook` to land the publish deterministically inside the
  subscribe→precheck race window, mirroring this file's existing race-avoidance pattern),
  concurrently call the equivalent of `post_backlog_update` (or publish a
  `ChangeActivityNoteAdded` event directly via the test's `ItemChangePublisher`/event bus
  double) on the same item, assert the call does NOT return early with
  `EventReceived: true` for that event (it should still be waiting, or time out).
- `TestWaitForBacklogEvent_should_StillWake_When_StatusTransitionFires`: same harness,
  but the concurrent event is a real status transition — assert the call DOES return
  `EventReceived: true` with the correct `EventKind` — proving Epic 4.4's fix didn't
  regress the tool's actual purpose.
**Files**: `server/mcp/tools_backlog_test.go` (existing file — follow whatever existing
`TestWaitForBacklogEvent_*` tests already establish for driving this race
deterministically; confirm exact harness during implementation).

##### Task 8.4.2a: Write the not-woken-by-activity-note test (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 8.4.2b: Write the still-woken-by-status-transition regression test (~4 min)
- Files: `server/mcp/tools_backlog_test.go`

#### Story 8.4.3: `backlogItemMatchesFilters` still passes for an activity-note event [Blocker 2 fix]
**As a** `WatchBacklogItems` caller with a non-empty `status_filter`/`category_filter`
(e.g. `web-app/src/app/review-queue/page.tsx`'s real shipping filter), **I want** an
activity-note-posted event on a matching item to still reach me live, **so that**
`post_backlog_update` calls aren't silently invisible on the Review Queue page.
**Acceptance Criteria**: `TestBacklogItemMatchesFilters_should_MatchActivityNoteEvent_When_StatusFilterSet`
(or equivalent, alongside this file's existing `backlogItemMatchesFilters`/streaming
tests): construct a `session.BacklogItemData` with `Status`/`RepoPath` populated (as
`AppendActivityNote`'s Blocker-2 fix now guarantees), assert `backlogItemMatchesFilters`
returns `true` for a `WatchBacklogItemsRequest` whose `status_filter`/`category_filter`
matches those values — and, for contrast, construct one with the pre-fix empty
`BacklogItemData{ID: itemID}` shape and assert it would have returned `false` (documents
the bug this fix closes, doesn't just test the fixed state in isolation).
**Files**: `server/services/backlog_service_events_test.go` (existing file).

##### Task 8.4.3a: Write the filter-still-matches test (~4 min)
- Files: `server/services/backlog_service_events_test.go`

### Epic 8.5: Frontend tests
**Goal**: The reducer and component are proven, including the anti-clobber property
ADR-002 exists to guarantee.

#### Story 8.5.1: `appendActivityNote` reducer test
**Acceptance Criteria**:
- `appendActivityNote` appends to an existing item's `activityNotes` without touching
  any other field (assert `itemSessions`/`progressNotes`/etc. on the item are
  reference-unchanged or value-unchanged after the action).
- `appendActivityNote` on an item not yet in the store is a no-op (store unchanged).
- **[Blocker 1 fix test]** "a wholesale item-replace event with empty activityNotes
  preserves previously-stored activityNotes": seed the store with an item that has
  `activityNotes` populated, dispatch `upsertItem` with an incoming item whose
  `activityNotes` is empty/absent (simulating a status-transition/`ItemUpdated` event's
  full-`protoItem` payload, which never carries `activity_notes` per Blocker 1), assert
  the stored item's `activityNotes` after the dispatch still equals the original
  populated list — directly exercising Task 6.2.3a's extended guard (this is the guard
  extension, not `appendActivityNote`, so this test must dispatch `upsertItem`, not
  `appendActivityNote`).
- For contrast, a wholesale item-replace event whose incoming `activityNotes` IS
  populated (a genuine all-notes-still-present resnapshot) replaces the stored value with
  the incoming one, not the stale existing one — proves the guard only masks the known-bad
  empty-partial-load case, not real updates (mirrors the `itemSessions` guard's own
  documented limit).
**Files**: `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts` (existing file, if present
— else colocated new `*.test.ts` file matching this repo's existing frontend test
convention; confirm exact existing filename during implementation).

##### Task 8.5.1a: Write the reducer tests (~5 min)
- Files: `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts`

##### Task 8.5.1b: Write the `upsertItem` activityNotes-anti-clobber tests (~5 min) [Blocker 1 fix]
- Files: `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts`

#### Story 8.5.2: `useWatchBacklogItems.ts` new-case test
**Acceptance Criteria**: A test drives a mock stream emitting an `activityNoteAdded`
event and asserts `appendActivityNote` was dispatched with the correct payload
(mirroring how existing cases like `itemUpdated`/`verdictRecorded` are already tested in
this hook's test file).
**Files**: `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts` (existing file).

##### Task 8.5.2a: Write the new-case dispatch test (~4 min)
- Files: `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts`

#### Story 8.5.3: `ActivityLogSection` rendering test
**Acceptance Criteria**: Renders with 0 notes → returns `null` (component absent from
DOM). Renders with N notes → all visible up to the 8-item cap, "Show N more" appears
above the cap and reveals the rest on click (mirroring
`ProgressHistorySection.test.tsx`'s existing assertions, if such a file exists — confirm
during implementation and follow its exact test structure).
**Files**: `web-app/src/components/backlog/detail/ActivityLogSection.test.tsx` (new).

##### Task 8.5.3a: Write the rendering + show-more tests (~5 min)
- Files: `web-app/src/components/backlog/detail/ActivityLogSection.test.tsx`

### Epic 8.6: `get_backlog_item`'s Activity Log rendering tests [Concern 3 fix]
**Goal**: Task 5.2.1a's actual text-envelope output is directly exercised — heading
placement, per-entry format, the 20-entry cap, sanitization, and non-collision with the
verdict section's formats — closing the one MCP-facing acceptance criterion in Story
5.2.1 that had no corresponding test.

#### Story 8.6.1: `getBacklogItem`'s `## Activity Log` section
**As a** maintainer, **I want** direct assertions on the rendered Activity Log section,
**so that** a future change to `getBacklogItem`'s rendering can't silently break the
format without a test catching it.
**Acceptance Criteria** (follow `TestGetBacklogItem_ReturnsItemWithEnvelope`'s exact
style — confirmed present at `server/mcp/tools_backlog_test.go:280`, using
`newTestBacklogStorage(t)` + `storage.CreateBacklogItem` + `makeToolReq` +
`handler.getBacklogItem` + extracting `result.Content[0].(mcpgo.TextContent).Text`):
- `"## Activity Log"` appears in the text output, positioned after `"## Latest Review
  Verdict"` (when a verdict exists) and before the role-aware guidance block.
- With zero activity notes, the `"## Activity Log"` heading does not appear at all
  (matches the "Latest Review Verdict" section's own only-if-present convention).
- Each entry renders as `"- note from %s at %s: %s"` with the documented author-fallback
  chain (title → raw UUID → `"manual"`).
- Posting more than 20 notes: only the last 20 render, plus a line containing
  `"(N older entries not shown)"` with the correct count.
- A note containing an HTML tag renders with the tag stripped (proves
  `session.SanitizeForAgentContext(message, 500)` is applied at render time, per Task
  5.2.1a — independent of Concern 1's persist-time stripping, which this test doesn't
  need to re-verify).
- The section's output never contains the literal strings `"Outcome:"` or `"Criterion "`
  when there is no review verdict (proves no accidental format collision with the verdict
  section — e.g. a naive shared-helper refactor could not silently reuse those lines for
  activity notes without this test catching it).
**Files**: `server/mcp/tools_backlog_test.go` (existing file — add near
`TestGetBacklogItem_ReturnsItemWithEnvelope`).

##### Task 8.6.1a: Write the heading-placement + per-entry-format tests (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 8.6.1b: Write the 20-entry-cap + truncation-message test (~4 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 8.6.1c: Write the sanitization + no-format-collision tests (~4 min)
- Files: `server/mcp/tools_backlog_test.go`

### Epic 8.7: `GetBacklogItem` eager-load → proto mapping integration test [Gap 2 fix, validation.md]
**Goal**: Prove `activity_notes` actually flows end-to-end through the *other* read
path — `EntRepository.GetBacklogItem`'s eager-load (Task 2.1.4b) →
`BacklogItemData.ActivityNotes` (Task 2.1.4a) → `backlogItemToProto`'s mapper (Task
3.2.1a) → the wire `GetBacklogItem` RPC response — which is a completely separate code
path from `get_backlog_item`'s MCP-tool rendering (Epic 8.6, which calls
`ListActivityNotesForItem` directly and never touches this path at all). The sibling
field `ProgressNotes` already has exactly this test
(`TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent`,
confirmed live at `server/services/backlog_service_test.go:839`) but `ActivityNotes`
does not, so a bug in either Task 2.1.4b's eager-load or Task 3.2.1a's mapper block
would ship with zero test coverage.

#### Story 8.7.1: Extend the existing audit-trail proto-mapping test
**Acceptance Criteria**: `TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent`
(or a directly adjacent, identically-structured test if the existing one's name/scope
shouldn't be stretched to cover a third unrelated concept — confirm during
implementation which reads cleaner) additionally seeds one `BacklogActivityNote` on the
test item and asserts the resulting proto's `ActivityNotes` field is non-empty with the
correct `Message`/`AuthorSessionUuid`/`AuthorSessionTitle`/`CreatedAt` values — mirroring
exactly how the existing test already asserts this for `StatusEvents`/`ProgressNotes`.
**Files**: `server/services/backlog_service_test.go` (existing file).

##### Task 8.7.1a: Extend or add the `ActivityNotes` proto-mapping assertion (~4 min)
- Files: `server/services/backlog_service_test.go`

---

## Summary of file touch points (all phases)

**New files**: `session/ent/schema/backlog_activity_note.go`,
`web-app/src/components/backlog/detail/ActivityLogSection.tsx`,
`web-app/src/components/backlog/detail/ActivityLogSection.test.tsx`. Epic 8.1's
cascade-delete test is NOT a new file (per validation.md's Correction #2) — it's added
to the existing `session/ent_repository_backlog_test.go`, already listed below.

**Modified files**: `session/ent/schema/backlog_item.go`, `session/ent/` (generated),
`session/repository.go`, `session/ent_repository_backlog.go`, `session/storage.go`,
`proto/session/v1/backlog.proto`, `gen/proto/go/session/v1/` (generated),
`web-app/src/gen/session/v1/` (generated), `server/services/backlog_service.go`,
`session/backlog_item_change.go`, `pkg/events/types.go`, `server/events/forward.go`,
`server/services/backlog_item_event_publisher.go`,
`server/services/backlog_service_events.go`, `server/mcp/tools_backlog.go`,
`web-app/src/lib/hooks/useBacklogService.ts`,
`web-app/src/lib/store/backlogItemsSlice.ts`,
`web-app/src/lib/hooks/useWatchBacklogItems.ts`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`,
`docs/registry/features/frontend/` (generated), `session/ent_repository_backlog_test.go`,
`server/mcp/tools_backlog_test.go`, `server/services/backlog_item_event_publisher_test.go`,
`server/services/backlog_service_events_test.go`,
`web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts`,
`web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts`.
