# Cost attribution: eliminate the "unattributed" bucket

## Problem
Insights "Cost by Stage" shows ~$1.55k of ~$2.14k as unattributed (measured 2026-09-20 via
`GetInsightsSummary{includeOrphans:true}`: $1,246 orphan transcripts + $307 associated-to-no-role).

## Root cause
Attribution = transcript -> live `sessions` row (Associator) -> `ItemSession.session_uuid` -> role/item.
`ItemSession` keeps no Claude conversation UUID, and the session row is deleted on completion
(828 `item_sessions` rows point at UUIDs no longer in `sessions`), so the link is lost.
Work-session rows also never record `estimated_cost_usd`; headless triage/review rows do.

## Stories
1. **Persist conversation UUID (this PR).** `item_sessions.conversation_uuid`; stamped from
   `claude_sessions` inside `EntRepository.Delete` (single choke point, covers every deletion path);
   Insights resolves role/item by conversation UUID when the live-session lookup misses.
2. **Headless triage/review (done).** `StreamChunk.ConversationID` carries the result line's
   `session_id`; `CallOptions.OnConversationID` hands it to the caller. Triage writes it via
   `UpdateItemSessionConversationUUID` (its row exists before the call); headless re-review sets
   `ItemSessionData.ConversationUUID` at creation. Fold-in dedupe skips rows matched by conversation
   UUID. Not covered: other `CallBlocking` callers that write no per-call ItemSession (intent,
   approval, gate custom check, autonomous driver) and the Gemini adapter.
3. **Deleted backlog items (not started).** `DeleteBacklogItem` deletes the item's `ItemSession` rows,
   so attribution dies with the item. Options: soft-delete, or a durable cost ledger. Needs a decision.
4. **Tighten path-prefix match (not started).** `associateRecord` strategy 2 matches a session rooted at
   `/home/tstapler` to ~290 unrelated transcripts.
5. **UI bucket split (not started).** Separate "external (not stapler-squad)" from "unattributed".
6. **Backfill existing rows (done, low yield).** `InsightsService.BackfillConversationUUIDs` runs once
   per process on the first summary request. Links a worktree transcript to an unlinked, non-live
   ItemSession (real UUID) created 0-3 min before the transcript's first message, only if exactly one
   candidate fits. Measured offline against live data (2026-09-20): links 3 orphan transcripts, $208 of the
   $1,553 unattributed. The rest have no ItemSession within the window: they are user-started sessions
   (e.g. `steam-controls`, `steering`, `ci-speed` worktrees), not backlog work.
7. **Label non-backlog sessions (done).**
   - Sessions table: `pathBasename` (`insightsFormatters.ts`) returns the worktree title instead of the
     opaque id.
     - Server-side grouping: `groupUnattributed` (`insights_service.go`) reuses the existing
     `RoleCostBreakdown.items` shape — for role `""`, items are now keyed by
     `sessionDisplayTitle(ProjectPath)` (same worktree-title regex as the frontend's, kept in sync by
     comment) instead of one blob keyed by item_id `""`. Synthetic IDs are prefixed `untracked:` so they
     can never collide with a real backlog-item UUID. New `UnattributedByTitleTable` renders it below the
     "Cost by Stage" chart.
