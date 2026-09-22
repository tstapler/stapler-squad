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
3. **Deleted backlog items (done).** `DeleteBacklogItem` (`session/ent_repository_backlog.go`) writes a
   `DeletedItemSessionCost` ledger row (`session/ent/schema/deleted_item_session_cost.go`, no FK — the item
   and session are both gone by the time a row exists) per `ItemSession` before hard-deleting them. Chose
   the ledger over soft-delete: additive, touches one write site and one read-merge site, can't regress
   `GetAllItemSessionsWithBacklogInfo`'s existing scan or `BacklogItem.title`'s `Unique()` constraint the
   way a soft-delete's unbounded list/query-site blast radius could. `InsightsService` folds ledger rows in
   at both surfaces Story 4.1.4 already feeds, keyed by `conversation_uuid` only (a ledger row's snapshotted
   `session_uuid` can never resolve — that session row was gone before the item was even deleted):
   `sessionMetaForSessions` (lets a still-on-disk transcript recover its role/item via `metaFor`'s
   conversation-UUID fallback) and a new ledger fold-in pass mirroring the 4.1.4 transcript-less pass,
   deduped via a `transcriptCoveredConversationUUIDs` set (same double-count guard shape as Story 1's).
   Caveats: a ledger row with no `conversation_uuid` ever stamped (Gemini triage/review — Story 2's known
   gap) is dropped, not folded in blind, since there's no key to dedupe it against a transcript by; and
   `SessionsTable`'s backlog badge still links to `/backlog?item=<id>`, which 404s for a ledger-sourced row
   since the item is gone — not worth plumbing a "is this a live item" check just for the link.
4. **Tighten path-prefix match (done).** `isPathPrefixMatch` (`session/tokens/association.go`) dropped
   its reverse-direction branch (sessionPath under resultPath): a transcript decoded to a short, generic
   path (e.g. "/home/tstapler" from a `claude` run straight in $HOME) was matching every session rooted
   anywhere under it. Verified against live data: 288 sessions / $90 were misattributed this way — lower
   than the ~290/$307 first estimated, which conflated this bug with other no-role associations.
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
