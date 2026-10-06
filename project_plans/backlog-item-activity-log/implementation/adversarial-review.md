# Adversarial Review: backlog-item-activity-log

**Date**: 2026-08-18
**Verdict**: CLEAN
**Round**: 2 (re-review after blocker/concern remediation)

## Prior Blocker/Concern Verification

- [x] Blocker 1 fix verified: `upsertItem`'s real reducer body (`web-app/src/lib/store/backlogItemsSlice.ts:48-72`, comment at 55-61, ternary at 62-67) matches the plan's citation exactly; Task 6.2.3a's patch-object extension sits inside the one shared reducer that every clobber-risk event kind (`verdictRecorded`, `statusChanged`, `sessionAttached`, `itemUpdated` — confirmed at `web-app/src/lib/hooks/useWatchBacklogItems.ts:284-311`, all dispatching `upsertItem`) funnels through, so the fix is comprehensive by construction, not per-kind. `itemArchived` never calls `upsertItem` at all (line 312-314), and the new `activityNoteAdded` case deliberately bypasses it too (dispatches `appendActivityNote` instead, per ADR-002) — neither needs the guard.
- [x] Blocker 2 fix verified: `backlogItemMatchesFilters` (`server/services/backlog_service_events.go:213-224`) checks exactly `item.Status` (against `status_filter`) and `item.RepoPath` (against `category_filter`) and nothing else — confirmed by re-reading the live function. Task 2.1.2a's plan to populate both fields on the snapshot before calling `publishItemChangedSnapshot` will satisfy this filter.
- [x] Blocker 3 fix verified: `waitForBacklogEvent` spans exactly `server/mcp/tools_backlog.go:577-666` as corrected (the preceding consts/helpers span 435-576, confirmed: `eventTypeAny`-family consts 441-449, `WaitForBacklogEventResult` 452-466, `backlogEventKindFilterValue` 470-487, `buildMatchedWaitResult` 491-523, `currentStateWaitResult` 532-561). The matching loop's cited lines 652-663 are byte-for-byte accurate. Task 4.4.1a's early `continue` (placed right after `payload := evt.BacklogItemPayload` at line 655) sits before both the item-ID check (656) and the `eventTypeFilter` comparison (660), so it excludes `ChangeActivityNoteAdded` unconditionally, including under the default `event_type: "any"` filter — closing the gap correctly.
- [x] Concern 1 fix verified: `SanitizeForAgentContext(s string, maxLen int) string` (`session/backlog_context.go:16-18`) delegates to `sanitizeField`, which strips HTML tags first and only truncates `if len(s) > maxLen`. Tag-stripping can only shorten a string, and the plan's order-of-operations already length-caps the raw message at 2000 chars before calling `SanitizeForAgentContext(trimmed, 2000)` — so the post-strip string is guaranteed `<= 2000` chars and the truncation branch can never fire for valid input.
- [x] Concern 2 fix verified: `ent.IsConstraintError` is a real, established idiom in this package (`session/ent_pipeline_mode_repository.go:50-52` maps it to `ErrConflict`; `session/storage.go:514` uses it to distinguish a genuine failure from an update-in-place case). `session.ErrNotFound` is a real sentinel (`session/repository.go:16`), and the exact `errors.Is(err, session.ErrNotFound)` → `ErrItemNotFound` mapping Task 5.1.2b plans to reuse already exists verbatim in `waitForBacklogEvent` itself (`tools_backlog.go:620-621`), confirming the pattern is idiomatic and will fire correctly once Task 2.1.2a wraps the FK violation.
- [x] Concern 3 fix verified: `TestGetBacklogItem_ReturnsItemWithEnvelope` exists exactly at `server/mcp/tools_backlog_test.go:280` with the `newTestBacklogStorage`/`makeToolReq`/`handler.getBacklogItem`/`TextContent`-extraction style Epic 8.6 says it will follow.
- [x] Concern 4 fix verified: ADR-001's Consequences → Negative section now contains the explicit "[Named explicitly post-review, 2026-08-18 — Concern 4 ...]" paragraph naming the doubled eager-load-edge surface as an accepted tradeoff.

All seven prior findings are addressed by real code/doc changes traceable to live source, not just asserted. Proto field-number and file-touch-point citations added in the plan's "Deviations" section were also independently spot-checked and are accurate: `oneof event` at `backlog.proto:756`, `seq = 8` at line 782 (message-level, outside the oneof), `snapshot_complete = 9` at line 773 (next free = 10, confirmed), `plan_rejected_at = 34` at line 204 (next free = 35, confirmed), `BacklogProgressNote` message spans 141-147, `BacklogItemUpdatedEvent` spans 823-828, `BacklogChangeTriageProgressUpdated` at `pkg/events/types.go:53` and `session/backlog_item_change.go:29`, `server/events/forward.go`'s alias const block at lines 29-38, `ListProgressNotesForItem` ends at `session/ent_repository_backlog.go:2071`, `Storage.ListProgressNotesForItem` at `session/storage.go:1329-1335`, `backlogItemToData` at line 178 with the `ProgressNotes` eager-load-propagation block at 236-241, and `GetBacklogItem` spans 337-361 with `WithProgressNotes` at 349-351 — all exact matches.

## Blockers

(none)

## Concerns

(none)

## Minors

- `publishItemChangedSnapshot`'s own doc comment (`session/ent_repository_backlog.go:1616-1620`) still reads "Only `DeleteBacklogItem` should call this directly" — once Task 2.1.2a lands, `AppendActivityNote` becomes a second sanctioned direct caller (deliberately, since ADR-002's event never carries a full item and there's nothing for `attachItemSessionsForPublish` to usefully re-query). The plan documents this reasoning in Task 2.1.2a's own doc-comment instructions but never adds a task to update the shared function's comment to name the second caller, so a future reader of `publishItemChangedSnapshot` in isolation will be told something no longer true. Purely cosmetic — no behavior or test depends on that sentence — but worth a one-line fix alongside Task 2.1.2a to keep the "only" claim honest.
- (Carried forward, still accurate) Task 1.1.1c's ~4 min estimate for ent regen + build-drift resolution remains optimistic relative to the plan's own sizing convention, especially for a first-time required edge + composite index.
