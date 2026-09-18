# Validation Plan: backlog-item-activity-log

**Date**: 2026-08-18

This validates plan.md's Phase 8 (Epics 8.1-8.6) against requirements.md's acceptance
criteria and against this repo's actual test files (not just plan.md's citations). Test
names/scenarios are taken verbatim from plan.md where it already specifies them —
no test design was invented from scratch. Three placement/naming corrections and one
concrete coverage gap were found; see "Corrections" and "Gaps Found" below the table.

## Naming convention verification

- **Go**: confirmed by grep against the live files. `server/mcp/tools_backlog_test.go`
  has 78/122 test functions using `TestXxx_should_YYY_When_ZZZ` (e.g.
  `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR`,
  `server/mcp/tools_backlog_test.go:1480`); `session/ent_repository_backlog_test.go` has
  34 of its tests in the same style (e.g.
  `TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted`,
  `session/ent_repository_backlog_test.go:587`). This is the dominant convention for
  every backlog feature added since at least mid-2026, and it's what plan.md's Phase 8
  already uses throughout. Older tests in `_Verb` style (e.g.
  `TestReportProgress_RejectsWhenNoSessionUUID`) predate this convention and are not the
  pattern to follow for new tests.
- **Frontend**: confirmed against `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts`
  and `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts`. Convention is nested
  `describe("sliceOrHookName", () => { describe("methodOrCase", () => { it("plain-English
  sentence, lowercase, no Given/When/Then scaffolding", () => {...}) }) })` — not the Go
  `_should_..._When_` style. Component tests (`ProgressHistorySection.test.tsx`) use a
  single top-level `describe(ComponentName)` with plain-English `it()` strings, some of
  which embed the Go-style name as the *string content* of one `it()` for a specific
  regression (`ProgressHistorySection.test.tsx:41`) — this is the exception, not the
  default; most `it()` strings are plain English.

## Requirement → Test Mapping

| Requirement / AC | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| Story 1.1.1 — schema fields, parent edge, cascade delete | `session/ent_repository_backlog_test.go` (corrected — see Corrections #2) | `TestBacklogActivityNote_should_CascadeDelete_When_ParentItemDeleted` | Unit (ent, sqlite) | Create item, create note via `repo.GetEntClient().BacklogActivityNote.Create()`, delete item, assert note row count is 0 (mirrors `TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted`'s `Count(ctx)` idiom at line 614-616) |
| Story 2.1.2 AC1 — `AppendActivityNote` persists, `ListActivityNotesForItem` returns ordered | `session/ent_repository_backlog_test.go` | `TestAppendActivityNote_should_PersistAndBeListable_When_Called` | Unit (ent, sqlite) | Append two notes, list, assert both present in `created_at` ASC order |
| Story 2.1.2 AC1 — publishes `ChangeActivityNoteAdded` event | `session/ent_repository_backlog_test.go` | `TestAppendActivityNote_should_PublishActivityNoteAddedEvent_When_Called` | Unit (ent + test `ItemChangePublisher` double) | Wire test publisher double, append note, assert exactly one `ChangeActivityNoteAdded` event with correct `ActivityNote.Message` |
| Story 2.1.2 AC2 [Blocker 2 fix] — published snapshot carries real `Status`/`RepoPath`, not zero-values | `session/ent_repository_backlog_test.go` | `TestAppendActivityNote_should_PopulateStatusAndRepoPathOnPublishedSnapshot_When_Called` | Unit | Item with known non-default `Status`/`RepoPath`, append note, assert published `BacklogItemPayload.Item.Status`/`.RepoPath` match (not zero-value) |
| Story 2.1.2 AC3 [Concern 2 fix] — FK violation on nonexistent item maps to `ErrNotFound` at repo layer | Covered indirectly only — see Gaps Found #1 | — | — | Plan tests this only at the MCP-handler layer (`TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist`), not at `AppendActivityNote` directly |
| Story 2.1.4 — `GetBacklogItem` eager-loads `activity_notes`; `BacklogItemData.ActivityNotes` populated | None — see Gaps Found #2 | — | — | No repository-level test asserts `EntRepository.GetBacklogItem(...)`'s returned `BacklogItemData.ActivityNotes` is populated |
| Story 3.1.1/3.1.2 — proto messages exist, build passes | `go build ./...` (Task 3.1.3a, verification-only) | — | Build check | No dedicated proto test; consistent with how every other message in this file is validated (compile-only) |
| Story 3.2.1 — `backlogItemToProto` populates `activity_notes` from `item.ActivityNotes` | None — see Gaps Found #2 | — | — | Sibling field `ProgressNotes` has a dedicated test (`TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent`, `server/services/backlog_service_test.go:839`); no equivalent exists for `ActivityNotes` in Phase 8 |
| Story 4.1.1/4.1.2/4.1.3/4.1.4 — `mapBacklogChangeKind` maps `ChangeActivityNoteAdded` → `BacklogChangeActivityNoteAdded` without panic | `server/services/backlog_item_event_publisher_test.go` | `TestMapBacklogChangeKind_should_MapActivityNoteAdded_When_Called` | Unit | Direct call, assert mapped kind, no panic — new standalone test (no existing `TestMapBacklogChangeKind` function to extend; the sibling switch is only covered indirectly today via `TestBacklogItemEventPublisher_should_publishConvertedEventToBus_When_PublishItemChangedCalled`) |
| Epic 4.2 — `convertEventToBacklogItemEvent` builds non-nil `ActivityNoteAdded` oneof variant | `server/services/backlog_service_events_test.go` (corrected — see Corrections #1) | New table case `{name: "activity_note_added", ...}` added to `TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries` (existing table-driven test, line 563) | Unit (table-driven, `t.Run` subtests, `t.Parallel()`) | `check` func asserts `ev.GetActivityNoteAdded()` non-nil and its `ItemId`/`Note` fields match the payload |
| Epic 4.4 [Blocker 3 fix] — `wait_for_backlog_event` never wakes on an activity-note event | `server/mcp/tools_backlog_test.go` | `TestWaitForBacklogEvent_should_NotWake_When_ActivityNoteAddedFires` | Integration (goroutine + event bus, deterministic race harness) | Start wait with `event_type: "any"`, concurrently fire a `ChangeActivityNoteAdded` event, assert no early `EventReceived: true` |
| Epic 4.4 — regression: real status transition still wakes | `server/mcp/tools_backlog_test.go` | `TestWaitForBacklogEvent_should_StillWake_When_StatusTransitionFires` | Integration | Same harness, real status-transition event, assert `EventReceived: true` with correct `EventKind` |
| Story 4.3.1 — build verification, insert→publish→convert chain type-checks | `go build ./...` (Task 4.3.1a, verification-only) | — | Build check | No dedicated test; compile-only per plan |
| Story 8.4.3 [Blocker 2 fix, filter-bypass regression] — `backlogItemMatchesFilters` still passes for an activity-note event with populated `Status`/`RepoPath` | `server/services/backlog_service_events_test.go` | `TestBacklogItemMatchesFilters_should_MatchActivityNoteEvent_When_StatusFilterSet` | Unit | Populated `BacklogItemData{Status, RepoPath}` → filter matches `true`; contrast with pre-fix empty `BacklogItemData{ID: itemID}` shape → `false`. Note: this is the *first* direct unit test of `backlogItemMatchesFilters` by name — every existing test of that function is indirect via `TestWatchBacklogItems_should_...` end-to-end tests; a new direct unit test is a reasonable, not incorrect, deviation |
| Requirements: ungated tool callable with matching session UUID (baseline happy path) | `server/mcp/tools_backlog_test.go` | one case within Task 8.3.1a's set (name TBD at implementation, e.g. `TestPostBacklogUpdate_should_Succeed_When_SessionMatchesItem`) | Unit | Session UUID matches item's assigned session — succeeds, matching every gated tool's own happy path |
| Requirements success metric — ungated tool works with a **mismatched** session UUID | `server/mcp/tools_backlog_test.go` | e.g. `TestPostBacklogUpdate_should_Succeed_When_SessionNotLinkedToItem` | Unit | Session UUID present but NOT linked to item — succeeds; assert persisted `AuthorSessionUUID` == caller's UUID (the core behavior difference from every gated tool) |
| Requirements success metric — ungated tool works with **no** session UUID at all | `server/mcp/tools_backlog_test.go` | e.g. `TestPostBacklogUpdate_should_Succeed_When_NoSessionUUID` | Unit | No `STAPLER_SESSION_UUID` in context — succeeds; assert persisted `AuthorSessionUUID` empty/absent, no error |
| Requirements — explicit `session_id` param overrides context UUID | `server/mcp/tools_backlog_test.go` | e.g. `TestPostBacklogUpdate_should_UseProvidedSessionID_When_SessionIDParamGiven` | Unit | `session_id` param given, resolved via `findInstanceByID`, used for provenance instead of context UUID |
| Requirements — free-form text validated (empty) | `server/mcp/tools_backlog_test.go` | e.g. `TestPostBacklogUpdate_should_ReturnInvalidArgument_When_MessageEmptyOrWhitespace` | Unit | Empty and whitespace-only message rejected `ErrInvalidArgument` |
| Requirements — free-form text validated (length cap) | `server/mcp/tools_backlog_test.go` | e.g. `TestPostBacklogUpdate_should_ReturnInvalidArgument_When_MessageOverLengthCap` | Unit | Message > 2000 chars rejected `ErrInvalidArgument` |
| Requirements success metric — existing gated tools provably unchanged under the same unlinked/no-UUID conditions | `server/mcp/tools_backlog_test.go` | Task 8.3.1b, e.g. `TestReportProgress_should_StillRejectPermissionDenied_When_SessionUnlinkedOrMissing` (regression guard, parallel to `post_backlog_update`'s success cases) | Regression / Unit | `report_progress`/`request_review` called under identical unlinked/no-UUID conditions still return `PERMISSION_DENIED` |
| Concern 2 fix (handler-level) — nonexistent `item_id` → `ErrItemNotFound`, not `ErrInternalError` | `server/mcp/tools_backlog_test.go` | `TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist` | Unit | Well-formed but nonexistent `item_id` → asserts error code is `ErrItemNotFound` |
| Concern 1 fix — sanitize-before-persist, not just render-time | `server/mcp/tools_backlog_test.go` | `TestPostBacklogUpdate_should_StripHTMLTagsBeforePersisting_When_MessageContainsMarkup` | Unit | Post message containing `<script>...</script>`, read back via `ListActivityNotesForItem`, assert persisted text has tag stripped |
| Concern 3 fix — `get_backlog_item`'s `## Activity Log` section: heading placement | `server/mcp/tools_backlog_test.go` | Task 8.6.1a (name TBD, following `TestGetBacklogItem_ReturnsItemWithEnvelope`'s style, `tools_backlog_test.go:280`) | Unit | `"## Activity Log"` appears after `"## Latest Review Verdict"`, before role-aware guidance; absent entirely with zero notes |
| Concern 3 fix — per-entry format and author-fallback chain | `server/mcp/tools_backlog_test.go` | Task 8.6.1a | Unit | `"- note from %s at %s: %s"` format, fallback chain title → raw UUID → `"manual"` |
| Concern 3 fix — 20-entry cap + truncation message | `server/mcp/tools_backlog_test.go` | Task 8.6.1b | Unit | >20 notes posted → only last 20 render + `"(N older entries not shown)"` with correct count |
| Concern 3 fix — render-time sanitization + no format collision with verdict section | `server/mcp/tools_backlog_test.go` | Task 8.6.1c | Unit | HTML tag stripped at render; output never contains literal `"Outcome:"` / `"Criterion "` when no verdict exists |
| Story 6.1.1 — `ActivityNote` domain type + `mapActivityNote` + `BacklogItem.activityNotes` field | No dedicated unit test named in plan.md — mapper is exercised transitively by Epic 8.5's reducer/hook/component tests, consistent with how `mapProgressNote` itself has no standalone unit test either | — | — | Not a gap: matches existing precedent (`mapProgressNote` untested in isolation) |
| Story 6.2.1 — `appendActivityNote` reducer appends without touching other fields; no-op if item absent | `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts` (corrected path — see Corrections #3) | `describe("appendActivityNote", () => { it("appends to an existing item's activityNotes without touching other fields") / it("is a no-op when the item is not in the store") })` | Unit (Jest, Redux reducer) | Task 8.5.1a |
| Story 6.2.3 / Blocker 1 fix — `upsertItem`'s anti-clobber guard extended to `activityNotes` | `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts` | `describe("upsertItem — activityNotes backstop", () => { it("preserves previously-stored activityNotes when a wholesale item-replace event carries none") / it("replaces activityNotes when the incoming event carries its own non-empty list") })` | Unit (Jest) | Task 8.5.1b — mirrors the existing `itemSessions` backstop `describe` block at `backlogItemsSlice.test.ts:125` |
| Story 6.2.2 — `useWatchBacklogItems.ts` `activityNoteAdded` case dispatches `appendActivityNote` | `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts` (corrected path — see Corrections #3) | new `it("dispatches appendActivityNote when an activityNoteAdded event arrives")`-style case, following the file's existing `itemUpdated`-case test structure (e.g. around line 248) | Unit (Jest, mock stream) | Task 8.5.2a |
| Story 6.3.1 — `ActivityLogSection` renders 0/N notes, 8-item cap + Show More | `web-app/src/components/backlog/detail/ActivityLogSection.test.tsx` (new — path confirmed correct, matches colocated convention of `ProgressHistorySection.test.tsx`) | `describe("ActivityLogSection", () => { it("renders nothing when there are no activity notes") / it("shows a Show More button and reveals hidden notes above the 8-item cap") })` | Unit (Jest + RTL) | Task 8.5.3a — mirrors `ProgressHistorySection.test.tsx`'s exact three-case shape (empty/at-cap/above-cap) |
| Story 6.3.2 — wiring into `BacklogItemDetail.tsx` | Not separately unit-tested per plan.md; implicitly covered by any existing `BacklogItemDetail.test.tsx` smoke test that renders the full detail tree, if one exercises new sections generically | — | — | Consistent with how `ProgressHistorySection`'s own wiring into `BacklogItemDetail.tsx` has no dedicated wiring test either — not a gap |
| Epic 7.1 — registry regen clean | `make registry-diff` (CI gate, not a unit test) | — | CI check | Task 7.1.1a — verification only |

## Test Stack

- **Unit (Go)**: standard library `testing` + `github.com/stretchr/testify` (`v1.11.1`,
  confirmed in `go.mod:42`) — `require`/`assert`. Ent-backed tests use a real in-process
  SQLite ent client (`session.NewEntRepository(session.WithDatabasePath(...))` /
  `createTestEntRepository(t)`), not a mock — this repo does not mock the ORM. Table-driven
  tests use `t.Run` subtests with `t.Parallel()` inside both the parent and subtests
  (confirmed pattern in `TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries`).
  `go.uber.org/goleak` (`v1.3.0`, `go.mod:52`) is imported in `tools_backlog_test.go` for
  goroutine-leak detection on the concurrency-sensitive `wait_for_backlog_event` tests.
- **Integration (Go)**: same `testing` + `testify` stack; "integration" here means
  multi-component (real ent DB + real in-process event bus + concurrent goroutines), not a
  separate framework — e.g. `TestWaitForBacklogEvent_should_NotWake_When_ActivityNoteAddedFires`
  drives a real subscribe/publish race via the file's existing
  `testAfterWaitSubscribeHook` test seam.
- **Frontend unit**: Jest (`ts-jest` preset, `jest-environment-jsdom`,
  `web-app/jest.config.js`) + `@testing-library/react` for component tests
  (`render`/`screen`/`fireEvent`, confirmed in `ProgressHistorySection.test.tsx`). Redux
  slice tests dispatch actions directly against the reducer with no mocking framework.
- **API/E2E**: out of scope. This feature has no dedicated Playwright spec in
  `tests/e2e/` per plan.md, and none is warranted — the feature is fully exercised at the
  MCP-tool, repository, event-bus, and component/reducer levels; `tests/e2e/` specs in
  this repo are reserved for full page-load/user-journey flows, and `BacklogItemDetail.tsx`
  already has extensive component-level coverage instead of an e2e spec for every new
  section (confirmed: no `*.spec.ts` file references `ProgressHistorySection` or any
  sibling detail-page section by name).

## Coverage Targets

- Unit test coverage: >=80% (line) — per repo-wide convention (`make test-coverage`); no
  feature-specific override needed for this scope (7 modified Go files, 2 new; 5 modified
  frontend files, 2 new).
- All public service methods: happy path + error paths — satisfied: `AppendActivityNote`
  (happy path 8.2.1a, error/not-found path deferred to the MCP-handler layer, see Gaps
  Found #1), `ListActivityNotesForItem` (happy path only — no error path exists to test,
  since a nonexistent item legitimately returns an empty slice, not an error, matching
  `ListProgressNotesForItem`'s own behavior).
- All external integrations: unit mocked + at least one integration test — the one
  "external integration" here is the in-process event bus (`ItemChangePublisher`), and
  both a mocked-double test (8.2.1b) and a live-bus integration test (8.4.2's
  `wait_for_backlog_event` tests) exist.

## Corrections

Three placement/naming discrepancies found between plan.md's Phase 8 and the live repo
(none change scope or intent — all are file-path or test-structure corrections a
reviewer would otherwise catch during implementation):

1. **Task 8.4.1b's target is a table-driven test, not a new standalone function.**
   `server/services/backlog_service_events_test.go:563` already has
   `TestConvertEventToBacklogItemEvent_should_buildMatchingOneofVariant_When_KindVaries`,
   a single table-driven test covering every existing `BacklogChangeKind` → oneof-variant
   mapping (`status_changed`, `verdict_recorded`, `session_attached`, `item_updated`,
   `item_archived`, `item_removed`) via `t.Run` subtests. The correct implementation of
   Task 8.4.1b is a new `{name: "activity_note_added", ...}` entry appended to this
   table (before its closing `}` at line 666), not the separate function plan.md names
   (`TestConvertEventToBacklogItemEvent_should_BuildActivityNoteAddedVariant_When_KindIsActivityNoteAdded`).
   A separate function would still pass but would fork the established one-table-per-kind
   pattern for no reason.
2. **Epic 8.1's ent schema/cascade test has no established file to live in — `session/ent/schema/*_test.go` doesn't exist as a pattern anywhere in this repo** (`find session/ent/schema -iname "*_test.go"` returns zero files). Cascade-delete behavior for the most similar existing feature (`BacklogItemDependency`'s `entsql.OnDelete(Cascade)`) is tested in `session/ent_repository_backlog_test.go` (`TestAddBacklogItemDependency_should_UnblockDependent_When_BlockerIsHardDeleted`, line 587), using `repo.GetEntClient().<Type>.Query().Count(ctx)` to prove the row is gone. Epic 8.1's `TestBacklogActivityNote_should_CascadeDelete_When_ParentItemDeleted` should go in `session/ent_repository_backlog_test.go` alongside the Epic 8.2 tests, not in a new `session/ent/schema/backlog_activity_note_test.go` file as plan.md's Story 8.1.1 tentatively names it.
3. **Epic 8.5's two frontend test files are new files under `__tests__/`, not existing files at the paths plan.md names.** Plan.md's Story 8.5.1/8.5.2 say "existing file" at `web-app/src/lib/store/backlogItemsSlice.test.ts` and `web-app/src/lib/hooks/useWatchBacklogItems.test.ts` — both paths are wrong; the real, existing files are `web-app/src/lib/store/__tests__/backlogItemsSlice.test.ts` and `web-app/src/lib/hooks/__tests__/useWatchBacklogItems.test.ts` (confirmed via `find`). Both files DO already exist (contra plan.md's uncertain "if present" hedge) with an established `describe`/`it` structure to extend (`describe("upsertItem — itemSessions backstop", ...)` at line 125 is the direct sibling block for Task 8.5.1b to mirror). Epic 8.5.3's path (`web-app/src/components/backlog/detail/ActivityLogSection.test.tsx`, colocated, not under `__tests__/`) is correct as written — component tests in this repo are colocated (confirmed: 20+ examples directly under `web-app/src/components/backlog/`), only store/hook tests use a `__tests__/` subdirectory.

## Gaps Found — RESOLVED

Both gaps below were closed in plan.md immediately after this validation pass (see
plan.md's "Revisions after validation" section at the top). Left in place verbatim
below for traceability of what was found and why; treat as historical record, not an
open action item.

**Gap 1 → closed by plan.md Task 8.2.1e** (`mapAppendActivityNoteCreateError` unit
tests, testing the extracted error-mapping function directly rather than reproducing
the race).

**Gap 2 → closed by plan.md Task 8.2.1d** (`TestGetBacklogItem_should_ReturnActivityNotes_When_ItemHasThem`,
repository-level eager-load test) **and Epic 8.7** (proto-mapper test extending
`TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent`).

Two real coverage gaps, both real regressions-in-waiting of the same class the
adversarial review already caught twice (a second, unaudited code path silently not
carrying new data) — genuine per the task brief's expectation that these should be rare
but not impossible:

1. **No repository-level test for the Concern-2 FK→`ErrNotFound` mapping in
   `AppendActivityNote` itself.** Plan.md's only test of this behavior is
   `TestPostBacklogUpdate_should_ReturnErrItemNotFound_When_ItemIDDoesNotExist`
   (Task 8.3.1c), which tests it end-to-end through the MCP handler. That's sufficient to
   prove the user-facing behavior, but Task 2.1.2a's repository-layer logic (steps 2's
   short-circuit on `ent.IsNotFound` from the `Select(...).Only(ctx)` read, AND step 4's
   `ent.IsConstraintError` branch on the `Create()` call) has two distinct code paths, and
   only one can realistically be reached by the MCP-level test (a nonexistent `item_id`
   will be caught by step 2's read before ever reaching step 4's `Create()`). The
   race-condition path Concern 2's own doc comment calls out — "guards the race where the
   item was deleted between steps 2 and 3" — has **no test at any layer**, unit or
   otherwise, because it requires deleting the item between the read and the write, which
   the MCP-level test cannot deterministically trigger. Recommend adding one repository
   test that exercises step 4's `ent.IsConstraintError` branch directly (e.g. delete the
   item's row after step 2 could have run, via a lower-level ent call, immediately before
   invoking `Create()` — or, more simply, directly unit test `ent.IsConstraintError`
   handling by constructing a `Create()` call with a raw, never-`Select`-verified bad
   `item_id` against the ent client, bypassing `AppendActivityNote`'s own step-2 guard, to
   prove the FK-violation branch itself maps correctly). This is lower severity than
   Gaps Found #2 (it's a defensive branch for an already-rare race, not a normal-path
   silent-data-loss risk) but is a real, non-hypothetical hole.

2. **No test proves `GetBacklogItem`'s eager-loaded `ActivityNotes` field reaches the
   `GetBacklogItem` RPC response** — the actual "GetBacklogItem/equivalent RPC response"
   exposure requirements.md's Scope section names explicitly (separate from the
   `get_backlog_item` MCP tool's text-envelope rendering, which Epic 8.6 does cover).
   Concretely, two acceptance criteria have zero corresponding Phase 8 test:
   - Story 2.1.4: `EntRepository.GetBacklogItem` eager-loads `activity_notes` and
     `BacklogItemData` gets an `ActivityNotes` field populated by `backlogItemToData`.
   - Story 3.2.1: `backlogItemToProto` populates `p.ActivityNotes` from
     `item.ActivityNotes`.

   This matters because Epic 8.6's `get_backlog_item` MCP-tool tests take a **completely
   separate code path**: Task 5.2.1a's handler calls
   `h.storage.ListActivityNotesForItem(ctx, itemID)` directly, never touching
   `GetBacklogItem`'s eager-load or `backlogItemToProto`'s mapper at all. So a bug in
   either — e.g. the eager-load's `WithActivityNotes(...)` ordering clause silently
   failing, or a missing/misplaced `p.ActivityNotes = protoNotes` assignment in
   `backlog_service.go` (an easy copy-paste slip from the adjacent `ProgressNotes` block
   Task 3.2.1a is explicitly modeled on) — would leave the real `GetBacklogItem` RPC
   response (what `web-app/src/lib/hooks/useBacklogService.ts`'s `mapBacklogItem` and any
   other MCP/RPC client actually consumes) silently empty, while every Phase 8 test still
   passes green. The sibling field `ProgressNotes` has exactly this test already —
   `TestBacklogItemToProto_should_IncludeAuditTrail_When_StatusEventsAndProgressNotesPresent`
   (`server/services/backlog_service_test.go:839`) — asserting `p.ProgressNotes` is
   populated from a `BacklogItemData` with notes set. Recommend two additions before
   implementation starts:
   - A repository-level test (`session/ent_repository_backlog_test.go`) —
     `TestGetBacklogItem_should_ReturnActivityNotes_When_ItemHasThem` — creating an item,
     appending notes via `AppendActivityNote`, calling `GetBacklogItem`, and asserting the
     returned `BacklogItemData.ActivityNotes` is populated and ordered.
   - A proto-mapper test (`server/services/backlog_service_test.go`) —
     `TestBacklogItemToProto_should_IncludeActivityNotes_When_ItemHasThem` — mirroring the
     `ProgressNotes` test's exact shape with `ActivityNotes` substituted.

   This is the higher-severity of the two gaps: it is a normal-path (not race-condition)
   omission in the exact area (a second untested code path for the same new data) that
   both of the adversarial review's real blockers (1 and 2) were about.
