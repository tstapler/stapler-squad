# Research: Existing Audit-Trail Patterns & Respawn-Event Timeline Design

## 1. Existing audit-trail patterns — imitate, don't reinvent

Two near-identical precedents already exist and both are the right model for the new
respawn-event timeline:

### `BacklogStatusEvent` (workflow/status-transition audit trail)
- **Schema**: `session/ent/schema/backlog_status_event.go` — append-only, `id` (UUID),
  `item_id` (plain UUID field, not a live join target for anything but its own
  `item` edge), `from_status`, `to_status`, `triggered_by` (defaults `"user"`), optional
  `note`, immutable `created_at`. Indexed on `(item_id, created_at)` — exactly the access
  pattern a timeline needs (fetch-by-item, ordered by time).
- **Proto**: `BacklogStatusEvent` in `proto/session/v1/backlog.proto` (lines 86–98),
  eagerly loaded as `repeated BacklogStatusEvent status_events = 20` on `BacklogItem`, so
  no separate RPC round-trip is needed to view it — it comes back with `GetBacklogItem`.
- **Frontend**: `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx`. Uses
  `CollapsibleSection` (collapsed by default) + `useShowMore` (caps to 8 most-recent items,
  "Show N more" button, per-item/section `localStorage`-persisted expansion choice so a
  chronically-cycled item doesn't force a re-click every time the card reopens).
  **Deliberately always renders**, even with zero events, showing "No status history
  recorded." instead of hiding the section — the comment explicitly documents this is
  because older items predate the audit-trail feature (#198) and hiding it made the
  feature look broken. This is a directly transferable lesson for the respawn-event
  section: items that predate this feature will have zero respawn rows and should show an
  explicit empty state, not disappear.

### `BacklogProgressNote` (implementer's `report_progress` audit trail)
- Same shape: append-only, proto message eagerly loaded as `progress_notes = 27` on
  `BacklogItem`, rendered by `ProgressHistorySection.tsx` with the identical
  `CollapsibleSection` + `useShowMore(cap=8)` pattern. Difference: this one **does**
  hide itself when empty (`if (item.progressNotes.length === 0) return null;`) — the two
  existing sections actually disagree on empty-state handling, which is worth resolving
  deliberately for the new section rather than copying either one by accident.

### Verdict for the respawn-event table
Copy the `BacklogStatusEvent` schema shape almost verbatim: UUID id, `item_id` FK-by-value
+ edge back to `BacklogItem` (cascades on item delete, matching existing behavior — see
§2a), a `reason` string, `triggered_by`/source call-site tag, immutable `created_at`, index
on `(item_id, created_at)`. Store `triggering_session_uuid` and `resulting_session_uuid` as
**plain string fields, not FK edges to `ItemSession`** — see §2a for why. Eagerly load as
`repeated RespawnEvent respawn_events = <next field #>` on `BacklogItem` exactly like
`status_events`/`progress_notes`, so it rides along on the existing `GetBacklogItem` RPC
with no new endpoint required. Render with `CollapsibleSection` + `useShowMore(cap=8)`,
matching `WorkflowHistorySection`'s "always show, explicit empty state" behavior (not
`ProgressHistorySection`'s hide-when-empty) — a respawn timeline is exactly the kind of
"why do I keep seeing this item come back" data point where an explicit "never
auto-respawned" is more informative than a vanished section, similar to the stuck-items
"parked" state already surfaced by `StuckItem.tsx`.

## 2. Edge cases

### (a) Session deleted/archived after being logged as a respawn target
Confirmed via `session/ent_repository_backlog.go` (`DeleteBacklogItem`, ~line 790–825):
`ItemSession` DB rows are **only hard-deleted in cascade with their parent `BacklogItem`**
(along with their `ReviewVerdict` rows) — never independently when a tmux session is
archived/killed. Session archival (`archive-on-terminal`, tmux pane kill) does not delete
the `ItemSession` row; it's exactly what populates `end_reason` on it. So:
- A respawn-event row that references a `triggering_session_uuid`/`resulting_session_uuid`
  by plain string will almost always still resolve to a live `ItemSession` row, because
  session archival ≠ row deletion.
- The only case where the referenced session row is truly gone is whole-item deletion —
  at which point the respawn-event rows themselves are also being deleted in the same
  cascade (same `item_id` FK), so there's no dangling reference to handle.
- **Design implication**: still don't use a hard FK/edge to `ItemSession` for the
  session-uuid fields (unlike `item_id`, which does get a real edge). Store them as plain
  strings, matching how `BacklogStatusEvent`/`BacklogProgressNote` avoid FK-ing into
  anything except their owning item. This avoids needing ON DELETE semantics for a
  relationship that in practice is almost never broken, and avoids a join the UI doesn't
  need (the event already carries denormalized display data — reason/timestamp — the
  session uuid is just a cross-reference, not something the timeline needs to hydrate).

### (b) Rapid respawns / many events per item — pagination
Already solved by the existing pattern: `useShowMore(itemId, sectionKey, items, cap=8)`
caps default rendering to the 8 most recent, with a "Show N more" button whose
expanded-state choice persists per item+section in `localStorage`. Requirements doc
estimates "respawn events per item in the tens" (Non-functional Requirements section) —
well within what an eagerly-loaded array on `GetBacklogItem` can carry without a separate
paginated RPC. No new pagination mechanism needed; reuse `useShowMore` verbatim.

### (c) Item transitions to done/archived — should respawn history still be visible?
No code found that hides `status_events`/`progress_notes` based on item status — both
sections render unconditionally off `item.statusEvents`/`item.progressNotes` regardless of
`item.status` or `item.archivedAt`. `BacklogItem.archived_at` is a soft marker (the item
row itself isn't deleted on archive — only `DeleteBacklogItem`, a distinct explicit action,
hard-deletes and cascades). Precedent strongly favors: respawn history should remain
visible after archive/done, same as workflow/progress history — it's the record of what
was tried, which is most valuable precisely when reviewing a *finished* item's path to
completion (e.g. "this shipped, but only after being auto-respawned 4 times for stale
work").

## 3. Unstated needs — filter/search vs. chronological glance

No existing history section (`WorkflowHistorySection`, `ProgressHistorySection`, or the
stuck-items page `StuckItem.tsx`/`StuckItemDetail.tsx`) has any filter, search, or
grouping control — all three are flat, reverse-chronological (or chronological, capped)
lists with a single "show more" affordance and nothing else. The stuck-items page
(`/unfinished`'s `StuckItemsSection`) groups by *reason* as its organizing structure but
still has no text search. Given:
- Solo user, single-instance internal tool (no team/cross-user filtering need).
- Respawn events per item is "tens," not hundreds/thousands (per requirements' own
  Non-functional Requirements) — a flat capped list stays scannable at that volume.
- Every other audit-trail widget in this codebase deliberately stayed at
  "chronological + show-more," never grew filter UI even after shipping and presumably
  being used.

This strongly signals a plain chronological list (newest-first, capped, expandable) is
sufficient for v1 — building filter/search would be scope creep not asked for by any
existing usage pattern, consistent with the requirements doc's own "Rabbit Holes" section
warning against analytics/aggregation creep.

## 4. Comparable industry patterns (CI/CD retry UIs) — what's worth borrowing

Patterns from GitHub Actions ("Re-run jobs" → per-attempt history), CircleCI/Buildkite
(retry count badge + reason tooltip), and Kubernetes (`CrashLoopBackOff` restart counter)
that map cleanly onto this solo-user tool without importing enterprise weight:

- **A compact "retried N times" badge at the summary level**, with the full reason-by-reason
  breakdown only on expand — mirrors GitHub Actions' collapsed-by-default run-attempt list
  and is already the exact shape of `Collapsible`/`useShowMore` in this codebase. No new
  interaction pattern needed, just apply the existing one to the new data.
- **Distinguish "still retrying / backing off" from "gave up"** — GitHub Actions and
  Kubernetes both surface a clear terminal state (workflow marked failed after max retries;
  pod marked `CrashLoopBackOff` vs mid-backoff). This codebase already has the equivalent
  concept: `StuckItem.tsx`'s `isParked` (computed as `remediationAttempts >=
  MAX_REMEDIATION_ATTEMPTS`, mirroring the backend's 5-attempt backoff schedule
  30m/2h/8h/24h/72h) plus its "Retry now (disabled — remediation attempts exhausted; use
  Reset to try again)" messaging. The board card / Sessions section should reuse this same
  parked-vs-active framing rather than inventing new terminology — this is one of the
  "Success Metrics" the requirements doc calls out verbatim ("distinguish 'actively being
  retried... backing off' from 'stalled with no further automatic action'").
- **Reason attached to each attempt, not just an aggregate count** — CI tools show *why*
  each retry happened (timeout vs. flaky test vs. infra), not just "attempt 3 of 5". This is
  exactly what the new respawn-event row's `reason` field should carry per-event (mirrors
  `BacklogStatusEvent.note` for the analogous "why did this transition happen" need), rather
  than only incrementing a counter the way `BacklogStuckState.remediation_attempts` does
  today (a counter with no per-attempt reason — the acknowledged gap this item exists to
  close).
- **What NOT to borrow**: none of GitHub Actions/CircleCI/Kubernetes' retry UIs expose
  aggregate analytics (mean-time-to-green, retry-success-rate) in the primary retry view —
  that lives in separate insights/dashboards, if at all. This matches the requirements
  doc's explicit Out-of-Scope call-out ("Aggregate/analytics rollups... flagged as a
  possible future item, not required here") — no borrowing needed there, just confirmation
  the industry pattern agrees with the existing scope boundary.

## Key files referenced

- `session/ent/schema/backlog_status_event.go` — schema model to copy
- `session/ent/schema/backlog_stuck_state.go` (lines ~59–69) — `remediation_attempts`/
  `next_remediation_at` field comments, useful for the parked/backoff framing
- `proto/session/v1/backlog.proto` (lines 66–143) — `ItemSession`, `BacklogStatusEvent`,
  `BacklogProgressNote`, `BacklogItem` message shapes
- `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx` — primary model to
  imitate (always-render + explicit empty state)
- `web-app/src/components/backlog/detail/ProgressHistorySection.tsx` — secondary model
  (hides when empty — a divergence to resolve deliberately, not copy by accident)
- `web-app/src/lib/hooks/useShowMore.ts` — reusable pagination/cap hook, no new mechanism
  needed
- `web-app/src/components/backlog-stuck/StuckItem.tsx` (lines ~66–71, 204–224) —
  `MAX_REMEDIATION_ATTEMPTS`/`isParked` framing to reuse for "backing off vs. stalled"
- `session/ent_repository_backlog.go` (`DeleteBacklogItem`, ~lines 790–825) — confirms
  `ItemSession` rows are cascade-deleted only with their parent item, never independently
  on archival — informs the FK-vs-plain-string design decision in §2a
