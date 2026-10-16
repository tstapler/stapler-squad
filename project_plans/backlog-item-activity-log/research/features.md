# Research: Feature Landscape — backlog-item-activity-log

## 1. Does an existing "notes"/"activity"/"log" concept already exist? — YES, TWO of them

This is the single most important finding: **the exact shape needed already exists, twice**,
as append-only, per-item, timestamped ent tables with an edge back to `BacklogItem`. The new
feature should be a third instance of this established pattern, not a new invention.

### 1a. `BacklogProgressNote` — the closest structural analog

Schema: `session/ent/schema/backlogprogressnote.go:18-59`

```go
type BacklogProgressNote struct{ ent.Schema }

func (BacklogProgressNote) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", ...).Default(uuid.New),
		field.UUID("item_id", ...),
		field.Int("criterion_index").Min(-1), // -1 = item-level, not per-criterion
		field.String("note").Optional().Comment("... stored unbounded here."),
		field.String("status"),               // pending/in_progress/done/fail
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}
```

- Edge: `session/ent/schema/backlogprogressnote.go:44-51` — `edge.From("item", BacklogItem.Type).Ref("progress_notes").Field("item_id").Unique().Required()`
- Index: `index.Fields("item_id", "created_at")` (`backlogprogressnote.go:54-58`) — built exactly for "all notes for this item, in order" queries, which is exactly what an activity log needs.
- `BacklogItem` edge declaration with cascade delete: `session/ent/schema/backlog_item.go:180-181`.
- Doc comment explicitly frames it as history-preservation: "Unlike `BacklogItem.AcceptanceCriteria` (which stores only the *current* note per criterion, overwritten on each call), this table preserves the full history so a reviewer can see the entire timeline of notes" (`backlogprogressnote.go:13-17`).
- Write path: `EntRepository.AppendProgressNote` (`session/ent_repository_backlog.go:2033-2049`) — a bare `Create()` insert, called from `Storage.AppendProgressNote` (`session/storage.go:1319-1325`), called from the `report_progress` MCP handler (`server/mcp/tools_backlog.go:834-836`) as a best-effort side write ("a failure here must not fail the call that already succeeded above").
- Read path: `EntRepository.ListProgressNotesForItem` (`session/ent_repository_backlog.go:2053-2071`), ordered `Asc(created_at)`. Also eager-loaded via `GetBacklogItem`'s `WithProgressNotes(...)` (`session/ent_repository_backlog.go:349`, `623`).
- Domain DTO: `ProgressNoteData{ID, CriterionIndex, Note, Status, CreatedAt}` (`session/repository.go:408-417`) — **no author/session-identity field**.
- Proto: `BacklogProgressNote` message, `proto/session/v1/backlog.proto:137-141` (repeated field `progress_notes = 27` on the item message, line 179); mapped in `server/services/backlog_service.go:785-797`.
- UI: `web-app/src/components/backlog/detail/ProgressHistorySection.tsx` — collapsible, capped to 8 most-recent visible with a "Show N more" button (`SHOW_MORE_CAP = 8`, lines 15, 24-29).

### 1b. `BacklogStatusEvent` — the second instance of the same pattern

Schema: `session/ent/schema/backlog_status_event.go:14-51`

```go
type BacklogStatusEvent struct{ ent.Schema }
// fields: id, item_id, from_status, to_status, triggered_by (default "user"),
// note (optional, nillable), created_at (immutable)
```

- Doc comment: "an append-only log of backlog item status transitions" (line 13).
- Edge + cascade delete: `session/ent/schema/backlog_item.go:176-177`.
- Index: `index.Fields("item_id", "created_at")` (`backlog_status_event.go:47-50`) — identical indexing strategy to `BacklogProgressNote`.
- Write path: `recordStatusEvent(ctx, evClient, itemID, fromStatus, toStatus, triggeredBy, note)` helper (`session/ent_repository_backlog.go:42-58`), called from every status-transition code path (lines 1148, 1195, 1383, 1580) — it's a shared helper, not duplicated per call site.
- `TriggeredBy` values are a small fixed enum, **not free identity**: `TriggeredByUser`, `TriggeredBySystem`, `TriggeredByAgent`, `TriggeredByGitHubSync` (`session/backlog.go:93-96`) — this is the closest existing precedent for an "actor" field, but it identifies a *category* of actor, not a specific session UUID/title.
- Domain DTO: `BacklogStatusEventData{ID, FromStatus, ToStatus, TriggeredBy, Note *string, CreatedAt}` (`session/repository.go:398-406`).
- Proto: `BacklogStatusEvent` message, `proto/session/v1/backlog.proto:123-135` (repeated field `status_events = 20`); mapped in `server/services/backlog_service.go:767-781`.
- UI: `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx` — same collapsible/show-more-8 pattern; explicitly "Always renders (never hides itself for zero events)" for backward compatibility with pre-audit-trail items (lines 22-26).

### 1c. A related-but-different "notes" field — do NOT confuse with the above

`BacklogItem.notes` (`session/ent/schema/backlog_item.go:78-79`, `field.String("notes").Optional()`) is a **single mutable scalar field**, not a log. It's user-edited free text with an inline-edit UI (`web-app/src/components/backlog/detail/NotesSection.tsx`) — overwritten on save (`data-testid="backlog-notes-save"`), no history, no attribution, no per-entry timestamps. This is NOT the concept to reuse for an append-only attributed log; it answers a different question ("what's the current freeform note on this item"), not "who said what and when."

### 1d. Conclusion for design

**Reuse the `BacklogProgressNote`/`BacklogStatusEvent` shape exactly**: a new `BacklogActivityNote` (or similar) ent entity with `item_id` (edge to `BacklogItem`, cascade delete), a free-form `message`/`note` field, `created_at` (immutable), and — the one thing neither existing table has — genuine **author identity** (session UUID + session title), since neither `ProgressNoteData` nor `BacklogStatusEventData` carries anything beyond a coarse `TriggeredBy` category string. This is the one genuinely new piece of shape needed; everything else (table structure, index, edge, cascade-delete annotation, domain DTO, proto message + repeated field, service-layer mapping, MCP eager-load option, UI collapsible-section-with-show-more component) has a direct, copyable precedent.

## 2. Prior art in project_plans/

### `project_plans/session-notes/decisions/ADR-001-note-field-on-session-not-sibling-table.md`
Directly relevant precedent for the "sibling table vs. scalar field" design fork, resolved in the *opposite* direction from what applies here: that ADR chose a **scalar field on `Session`** over a `SessionGoal`-style sibling table specifically because the session note is **single-owner, single-writer** (the user editing from the UI), with no history requirement. Deciding factor stated explicitly: "who writes the field." The activity-log feature is the inverse case — **multi-writer, append-only, history-required** — which is exactly the `SessionGoal`/`BacklogProgressNote`/`BacklogStatusEvent` shape that ADR calls the "wrong" analog for a single scalar. This ADR is useful precisely because it draws the boundary: sibling table when multi-writer + append-only + structured; scalar field when single-owner + last-write-wins. The new feature is unambiguously on the sibling-table side of that line.

### `project_plans/backlog-agent-communication/` (planning-only, requirements + research + plan; some resulting tools since shipped)
Problem statement's gap #1 ("Agents have no structured way to hand rich context forward... or receive structured findings back") and its `ADR-001-agent-initiated-stuck-reason-rows.md` are relevant precedent for "reuse an existing table for a new agent-initiated trigger" reasoning: that ADR chose to **extend `StuckReason`/`BacklogStuckState` with new agent-initiated enum values** rather than build a parallel table, on the grounds that "an agent-initiated call ... is not fundamentally different ... from the data model's perspective." Same reasoning applies here: an ungated free-form note is not fundamentally different, data-model-wise, from a `BacklogProgressNote`/`BacklogStatusEvent` row — what differs is *who's allowed to call the write path*, not the row shape. That plan's Phase 3 (`report_pr_created` — Story 3.1.1) appears to have since shipped (it's now one of the gated tools listed in this feature's own requirements.md), confirming this repo's pattern of "plan a new MCP tool + ent field, land it as a standalone PR."

No requirements/plan artifact in this directory addresses free-form, un-gated posting — its scope was structured handoff fields, infra-bug reporting, and escalation, not what this feature needs.

### `project_plans/backlog-operator-feedback-loop/`
Addresses the **opposite direction** of communication: a *human operator* steering/answering a running session (via `steer_session`, `RejectPlan`), not a session posting a note onto a backlog item. Not directly reusable for this feature's read/write shape, but confirms `steer_session`/plan-approval flows are the established mechanism for human→agent communication, distinct from the agent→item logging this feature needs. No overlap or contradiction with this feature's design space.

### `project_plans/backlog-event-driven-updates/decisions/ADR-001-separate-notifier-and-backlog-item-event-channels.md`
Directly load-bearing for the "push to `WatchBacklogItems`" requirement. Establishes two **deliberately separate** event channels sharing one `pkg/events.EventBus`:
1. `session.Notifier`/`EventBusNotifier` — alert-worthy, human-notification-shaped, coalesced 500ms by `(sessionID, notificationType)` key.
2. `session.ItemChangePublisher`/`BacklogItemEvent` (a `oneof` of status-changed / verdict-recorded / session-attached / item-updated / item-archived / item-removed) — routine state transitions, never coalesced, consumed by `WatchBacklogItems` for live UI upsert.

A new activity-log post is a **routine, must-never-be-coalesced state change** (every posted note is state the UI must reflect, per the ADR's own framing of channel #2's traffic), so it belongs on the `ItemChangePublisher`/`BacklogItemEvent` channel, not `Notifier`. The existing `BacklogChangeKind` enum (`session/backlog_item_change.go:13-30`) already has `ChangeItemUpdated` ("emitted when item fields ... change") which a new note-append could ride without inventing a new wire-event kind — or a new `ChangeActivityNoteAdded` kind could be added, following the same enum-extension pattern `ChangeTriageProgressUpdated` used (`backlog_item_change.go:26-29`, itself noted as "Converts to the existing item_updated wire event, not a new proto message" — i.e. there's already a documented precedent for *not* adding a new proto message per new mutation kind).

### `project_plans/session-notes/` research/architecture.md, ux.md — not separately re-read in depth beyond the ADR above; the ADR is the load-bearing artifact for this feature's design question and was read in full.

## 3. Edge cases and failure modes — what precedent exists, and what's an open gap

- **Archived/done item receiving an update**: **No existing gated tool checks item status before writing.** Grepped `server/mcp/tools_backlog.go` for `item.Status ==`/`BacklogStatusDone`/`BacklogStatusArchived` — the only status-aware branches are in `waitForBacklogEvent` (terminal-state detection for polling, lines 501/534/549) and PR-related reassignment logic (lines 1528, 1542, 1991), not in `report_progress`/`request_review`/`report_blocked` write paths. **This is a genuine open gap, not a solved precedent** — the new tool's plan phase must decide explicitly whether posting to an archived/done item is allowed (current sibling behavior for `report_progress` et al. is "not checked, presumably allowed since the item is filtered out before the tool would normally be called").
- **Empty or huge message**: Precedent is a **hard length cap checked in the MCP handler**, not schema-level truncation for these specific fields. Every comparable free-form field enforces its own cap in `tools_backlog.go`: `request_review`'s `message` ≤ 2000 chars (`tools_backlog.go:868`), `verification_notes` ≤ 4000 chars (line 876), `report_blocked`'s `rationale` ≤ 2000 chars (line 1059), `report_duplicate`'s `reason` ≤ 1000 chars (line 1945), `report_pr_created`'s `summary` ≤ 1000 chars (line 1482). 2000 chars is the modal cap for a "freeform explanation" field; 1000 for a shorter "why" field. `BacklogProgressNote.note` itself is explicitly **stored unbounded** at the ent-schema level ("Rendered call sites are responsible for truncation ... stored unbounded here", `backlogprogressnote.go:35`) — truncation is a render-time (`SanitizeForAgentContext`/`sanitizeField`, `session/backlog_context.go:16-27`, cap 500-2000 depending on call site) not a persist-time concern in this codebase's existing pattern. Empty-string handling: no comparable field appears to reject empty explicitly (report_progress's own `note` is read via `args["note"].(string)` with no non-empty check, `tools_backlog.go:806`) — precedent leans toward allowing empty, though a free-form "note" tool whose only content is the note text arguably should require non-empty (unlike `report_progress`, where `note` is secondary to the status change).
- **Concurrent posts from two sessions**: **Not a race in the existing pattern**, because both `AppendProgressNote` and `recordStatusEvent` are plain `INSERT`s with a server-generated UUID `id` and `created_at` — no read-modify-write, no compare-and-swap, no shared mutable state between concurrent callers. A new append-only note table inherits this safety automatically as long as the write stays a bare `Create()` call (as opposed to, say, `UpdateAcCriterionStatus`, which *does* read-modify-write a JSON blob and would need locking if made concurrent — but that's not this feature's write pattern).
- **Session UUID that doesn't exist / spoofed identity**: No existing gated tool validates that `STAPLER_SESSION_UUID` corresponds to a *real* session before trusting it as an audit-trail value — `callerSessionUUIDForAudit` (`server/mcp/tools_goal.go:65-70`) explicitly documents accepting **any** string, including the `"manual"` sentinel for callers with no session at all, precisely because "a manual/external MCP client... is a legitimate caller." The gated tools (`report_progress` et al.) don't validate the UUID's realness either — they validate that it's *linked to the item* via `GetItemSessionBySessionAndItem` (`tools_backlog.go:809`), which incidentally also proves the session exists, but that's a side effect of the link check, not a dedicated identity-verification step. Since this feature's entire point is to accept callers **not** linked to the item, that check can't be reused as-is; the plan phase should decide whether to resolve the session title via `session_id`/`STAPLER_SESSION_UUID` on a best-effort basis (falling back to an "unknown"/"manual" sentinel, mirroring `callerSessionUUIDForAudit`'s existing pattern) rather than attempting to "verify" a claimed identity that has no verification precedent anywhere else in this codebase.
- **Unbounded growth / max stored entries**: **No cap exists at the storage layer for either existing table** — `ListProgressNotesForItem`/status-event equivalents fetch every row unconditionally. The only caps that exist are at two *consumption* layers, not storage: (1) `maxContextExtrasEntries = 20` (`session/backlog_review.go:107-114`) caps how many are rendered into a review-session's LLM prompt, explicitly reasoned as "an item with many rework cycles or a long report_progress history could otherwise produce an unbounded prompt"; (2) `SHOW_MORE_CAP = 8` in both `ProgressHistorySection.tsx`/`WorkflowHistorySection.tsx` caps default UI visibility behind a "Show N more" button. Precedent is therefore: **do not cap storage; cap rendering/display**, consistently with both existing tables.
- **Does posting change item status/timestamps ("last activity")?**: Status transitions already bump `updated_at` via ent's `UpdateDefault` (`session/ent/schema/backlog_item.go:165-167`), but that only fires on an actual `UPDATE` to the `BacklogItem` row. A pure insert into a sibling table (as both existing precedents do) does **not** touch `BacklogItem.updated_at` — `AppendProgressNote` never updates the parent row. If "last activity" visibility is wanted, precedent shows no existing mechanism does this automatically for sibling-table inserts; it would be a new decision, not something to assume is already handled.

## 4. Unstated user needs (observational only — not a recommendation to build more)

- **Distinguishing official verdict/status events from free-form notes in one timeline**: today the UI already keeps these as two *separate* sections (`ProgressHistorySection` for `report_progress` history, `WorkflowHistorySection` for status transitions) rather than one merged feed — so users are already accustomed to per-kind sections, not a unified activity feed. A new free-form note log would most naturally become a *third* parallel section (e.g. "Activity" or "Notes") next to these two, matching the established UI convention, rather than requiring a new merged-timeline component.
- **Seeing who (role) posted, not just session UUID/title**: `BacklogStatusEvent.TriggeredBy`'s existing enum (`user`/`system`/`agent`/`github_sync`) shows this codebase already values a coarse *category* of actor being visible at a glance, distinct from a raw UUID. A user glancing at an ad hoc dev session's note vs. a spawned `role="work"` session's note would likely want the same at-a-glance distinction (e.g. "ad hoc session" vs. "work session `abc123`") — worth noting as a plausible want, without recommending a full role-tracking system be built for v1.
- **Filtering/searching updates**: no existing precedent for filtering either `progress_notes` or `status_events` client-side or server-side beyond the per-item scope and the 8-item show-more cap — there is no search box, no per-criterion filter, nothing. Given the explicit non-goal ("A full threaded-comments/reply system... Start from 'append a timestamped, attributed note; read the log back.'"), this is worth naming as a plausible future want but is squarely out of scope for v1, consistent with both existing sibling tables never having grown filter/search UI themselves despite being live for a while.
