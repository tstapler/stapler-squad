# ADR-002: Live Activity-Note Events Carry a Single New Entry, Not a Whole-`BacklogItem` Snapshot

**Status**: Accepted (Auto Mode — no open questions)
**Date**: 2026-08-18

## Context

`WatchBacklogItems`'s existing live-update pattern (`server/services/backlog_service_events.go`'s
`convertEventToBacklogItemEvent`, lines 274-340 at research time) converts most
`BacklogChangeKind` values — including `BacklogChangeItemUpdated` and
`BacklogChangeTriageProgressUpdated` — into a single wire variant,
`sessionv1.BacklogItemEvent_ItemUpdated`, which carries a **full `BacklogItem` snapshot**
(`Item: protoItem`) plus `UpdatedFields`. The frontend's `upsertItem` reducer
(`web-app/src/lib/store/backlogItemsSlice.ts:48-76`, verified against the live file)
**replaces** `state.items[id]` wholesale with that snapshot, with exactly one carved-out
anti-clobber guard — for `itemSessions` specifically (lines 55-67), added because a
partially-loaded event push racing a fully-loaded one was an already-observed failure
mode for that field.

**There is no equivalent guard for `progressNotes`.** Combined with a confirmed gap in
`AppendProgressNote` (`session/ent_repository_backlog.go:2033-2049`, verified — it never
calls `publishItemChanged` at all, so today `progressNotes` never actually rides a live
`ItemUpdated` event with a stale/empty value from that specific write path), architecture.md
Finding 4e documents the general shape of the risk: any live snapshot whose `Item` was
built from a `Save()` result that wasn't eager-loaded with a given child edge will carry
an empty list for that edge, and `upsertItem` will stomp a previously-populated list down
to empty until the next full re-fetch silently repairs it. Reusing the whole-item-snapshot
`ItemUpdated` pattern for the new activity-note-posted event would either inherit this
risk outright (if the eager-load is missed on some call path) or force a new, permanently-
maintained anti-clobber guard for `activityNotes` mirroring the `itemSessions` one.

## Decision

Give the new capability its own dedicated `BacklogChangeKind` (`ChangeActivityNoteAdded`
in `session/backlog_item_change.go`, `BacklogChangeActivityNoteAdded` in
`pkg/events/types.go`) and its own **new oneof variant** on the existing
`BacklogItemEvent` proto message — `BacklogItemActivityNoteAddedEvent` — carrying only
`item_id` and the single new `BacklogActivityNote` entry, never the whole `BacklogItem`.
On the frontend, add an explicit `case "activityNoteAdded":` to
`useWatchBacklogItems.ts`'s switch that dispatches a new, dedicated reducer action
(`appendActivityNote`, in `backlogItemsSlice.ts`) which appends the single note to
`state.items[itemId].activityNotes` — a targeted array append, never a call to
`upsertItem`/a wholesale item replace.

This mirrors an already-proven pattern in this exact codebase:
`applyInlineVerdict` (`web-app/src/lib/hooks/useWatchBacklogItems.ts:99-119`, verified)
already patches a specific sub-field (the just-recorded verdict) from an event's own
inline payload onto the embedded item, specifically as a defense-in-depth measure
independent of whether the embedded item's eager-load was complete.

## Consequences

**Positive**:

- Structurally cannot inherit the `progressNotes` clobber-risk class: since the new
  event never carries a full `Item`, there is no whole-item replace for a partial
  `activityNotes` load to race against in the first place. This is "additive and
  isolated," per the task brief — it does not fix the pre-existing `progressNotes` bug
  (out of scope), but it introduces zero new instances of that bug class.
- No new eager-load cost pressure on `ListBacklogItems`/every other `ItemUpdated`
  publisher: the activity-note write path doesn't need to eager-load `activity_notes`
  onto the item at all for live delivery to work correctly (it still does, for the
  `GetBacklogItem`/initial-fetch path, per decision #4 in the plan).
- Matches an existing, reviewed pattern (`applyInlineVerdict`) rather than inventing a
  new client-side merge strategy.

**Negative**:

- **[Fixed post-review, 2026-08-18 — see Blocker 1 in `implementation/adversarial-review.md`]**
  This ADR's Context section frames the anti-clobber problem as specific to the new
  `activityNoteAdded` event, but the first adversarial review pass found the claim
  incomplete: `activity_notes` is also added directly onto the `BacklogItem` proto message
  itself (Task 3.1.1b), and every *other* `BacklogChangeKind`
  (`ChangeStatusTransition`, `ChangeVerdictRecorded`, `ChangeSessionAttached`,
  `ChangeItemUpdated`, `ChangeTriageProgressUpdated`) still publishes via
  `publishItemChanged` → `attachItemSessionsForPublish`
  (`session/ent_repository_backlog.go:1600-1602,1721-1731`), which re-populates only
  `ItemSessions`, never `ActivityNotes` — so their wire events embed a full `protoItem`
  whose `activity_notes` is empty. Without a fix, the very next status transition,
  verdict, session-attach, or item-field update on an item would wholesale-replace the
  frontend's stored item via `upsertItem` and wipe any activity notes accumulated via the
  new `appendActivityNote` reducer, back to `[]`, until the next full refetch silently
  repaired it — reproducing architecture.md Finding 4e's bug class for `activityNotes`,
  by every non-activity-note event, not by this ADR's own event kind. **Fix**: a targeted
  extension of `upsertItem`'s existing `itemSessions` anti-clobber guard
  (`web-app/src/lib/store/backlogItemsSlice.ts:55-67`) to also preserve `activityNotes`
  under the identical condition (existing has entries, incoming's is empty) — see
  `implementation/plan.md` Story 6.2.3 / Task 6.2.3a, with regression coverage in Story
  8.5.1 (Task 8.5.1b). This is additive to this ADR's own single-entry-event design, not
  a replacement for it: the dedicated `activityNoteAdded` event still never carries a
  full item snapshot, so it still cannot itself trigger this clobber; the guard extension
  closes the gap for every *other* event kind that does.
- One more oneof variant and one more explicit case in both
  `convertEventToBacklogItemEvent`'s Go switch and `useWatchBacklogItems.ts`'s TS
  switch — both switches are exactly the fan-out points pitfalls.md flags as prone to
  silently swallowing an unhandled kind (`convertEventToBacklogItemEvent` has no
  `default` and lives in a package excluded from the `exhaustive` linter;
  `useWatchBacklogItems.ts`'s switch has an explicit `default: break;`). This is a real,
  named cost — mitigated by an explicit regression test on each side (see
  implementation/plan.md's tests epic) asserting the new event kind reaches its handler,
  since neither compiler nor linter will catch a missed case here.
- A client relying solely on the `WatchBacklogItems` live stream — never calling
  `GetBacklogItem` — that connects mid-session will not see prior activity notes until
  its next full snapshot/fetch, since the live event only ever carries the newest single
  entry, not history. This is inherent to any single-entry-append design and matches how
  `applyInlineVerdict`'s inline verdict patch already behaves (it doesn't backfill prior
  verdict history either) — not a new limitation this feature introduces.

## Alternatives Considered

**Reuse `ChangeItemUpdated`/`BacklogItemEvent_ItemUpdated` with a full item snapshot**
(mirroring `progress_notes`'s exact wire path). Rejected as the primary path because it
requires either (a) eager-loading `activity_notes` on every `BacklogItemData` passed into
`publishItemChanged` from every call site that might publish an `ItemUpdated`/
`TriageProgressUpdated` event — a correctness-by-convention obligation with no compiler
enforcement, exactly the kind of thing that already silently regressed for
`progress_notes` in the `AppendProgressNote` path — or (b) adding a brand-new
`activityNotes`-specific anti-clobber guard to `upsertItem` mirroring `itemSessions`'s,
which only mitigates the symptom (a stale/empty snapshot clobbering good data) rather
than avoiding the cause (an unrelated write publishing a full-item snapshot it wasn't
authoritative for). architecture.md's own Finding 5d recommendation notes both options
are "viable" from a research standpoint; this ADR picks the option that closes off the
whole bug class by construction rather than the one that requires an additional, easy-to-
forget guard alongside every future edge added to `BacklogItem`.
