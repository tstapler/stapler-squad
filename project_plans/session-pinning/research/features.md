# Research: Session Pinning — Feature Landscape

## 1. Existing "flag/toggle a session, surface it specially" precedent in this codebase

### Schema (`session/ent/schema/session.go`)
`hidden`, `auto_yes`, `is_expanded`, `autonomous_mode`, `one_shot` are all plain
`field.Bool(...).Default(false)` fields directly on `Session`. `archived_at` is the
one *soft-delete-shaped* precedent: `field.Time("archived_at").Optional().Nillable()`
(nil = not archived). `pinned` should follow the `hidden`/`auto_yes` boolean shape per
the requirements doc — confirmed correct, no reason to use a timestamp instead (see
§4 for the "pin order" question this implies).

### Proto (`proto/session/v1/types.proto`)
**Correction to the requirements doc**: it states "63 is highest in use as of writing."
That's stale — `archived_at = 63` is not the highest field. The actual highest field
in the `Session` message (checked in full, message spans lines 9–232) is
**`workspace_key = 71`**. Next free field number is **72**, not 64.

### RPC precedent — `ArchiveSession`/`UnarchiveSession` (`server/services/session_service.go:4285`)
This is structurally the right pattern to mirror for `PinSession`/`UnpinSession`:
look up the live instance via `FindLiveInstance`, mutate via an actor setter,
`s.storage.SaveInstances([]*session.Instance{inst})`. `UnarchiveSession` is the
simpler of the two (`inst.SetArchivedAt(nil)`) — a `SetPinned(bool)` setter on
`Instance` (mirroring `SetAutoYes`, `SetCategory` in
`session/instance_actor_setters.go`) is the right shape, not a `SetPinnedAtIfNil`
CAS variant — pin/unpin is a direct user toggle, not a race-prone "first writer
wins" transition like `SetArchivedAtIfNil`.

**Important caveat found in code, not in the requirements doc**: `archiveSession`/
`unarchiveSession` are implemented end-to-end (proto → Go handler → the
`useSessionService.ts` hook, lines 600–618) but **the hook functions are not called
from any UI component**. `grep -rn "archiveSession" web-app/src` turns up only the
hook definition and a Jest mock in `ConnectionIndicator.test.tsx` — no
`SessionCard.tsx`, `SessionList.tsx`, or `SessionDetailView.tsx` call site. Archiving
today happens via backend automation only (`session/backlog_lifecycle.go`,
workflow auto-archive in `session_service.go`'s `maybeAutoArchive`), not a user
click. So requirement #6 ("mirrors existing action-menu affordances such as
archive/hide") has **no existing wired example to literally copy** on the frontend
side — the planning phase needs to design the menu-entry UI from scratch (probably
closest to how `onDelete`/`onRename` are wired as props into `SessionCard.tsx`,
which are the only mutation affordances actually present on the card today), not
just "add a `Pin` case next to `Archive`" as the requirements implies.

`hidden` has **no dedicated RPC at all** — it's not in `UpdateSessionRequest`
(`proto/session/v1/session.proto:579-617`) and no `HideSession`/`SetHidden` handler
exists in `session_service.go`. So of the two "existing toggle" fields the
requirements cite as precedent, only `archived_at` has a working RPC pair, and even
that pair is UI-orphaned. `PinSession`/`UnpinSession` would be the *first* boolean
session flag with a fully wired create-to-click path in this codebase.

### Session list rendering / grouping (`web-app/src/components/sessions/SessionList.tsx`, `web-app/src/lib/grouping/strategies.ts`)
Pipeline: `filteredSessions` (search/category/tag/archived filters, line ~530) →
`sortedSessions` (line 598, sorts by name/createdAt/updatedAt/lastActivity/tokenCost)
→ `groupedSessions` (line 648, calls `groupSessions(sortedSessions, groupingStrategy)`)
→ `flatItems`/`cardFlatSessions` for the row/card virtualizers.

`GroupingStrategy` (`web-app/src/lib/grouping/strategies.ts:7-17`) actually has
**10** values today — Category, Tag, Branch, Path, Program, Status, SessionType,
Project, Workflow, None — not the 8 CLAUDE.md's architecture table describes (stale
doc, worth a note but out of scope here). `groupSessions()` returns
`GroupedSessions[]` (`{groupKey, displayName, sessions}`), consumed identically by
both the row-mode (`flatItems`) and card-mode (`cardFlatSessions`/`cardGroupCounts`)
virtualizers. **A "Pinned" section is naturally a synthetic group prepended to the
`groupedSessions` array** (or a wholly separate render before the grouped list,
outside `groupingStrategy`, drawing from `filteredSessions` filtered on
`session.pinned` and short-circuiting normal group membership) — the array-of-groups
shape already supports "one section on top, independent of the active sort/group
choice" without new virtualizer plumbing. The existing "Show archived" toggle
pattern (filtered out client-side at `filteredSessions`, line 579) is the wrong
model to imitate for pinned — pinned sessions must appear both in their normal
group/sort position is **not** desired per requirement #4 ("pinned section above the
... list, containing all pinned sessions regardless of status/recency") — this
implies pinned sessions should be **excluded from the normal grouped/sorted list**
once shown at top (avoid duplicate rendering), which needs an explicit decision in
planning: dedupe by excluding `pinned` sessions from `groupedSessions`, or render
them in both places. Recommend the former (matches every industry example in §2).

### Actor-setter precedent (`session/instance_actor_setters.go`)
`SetAutoYes`, `SetCategory`, `SetArchivedAt` are the direct-boolean/string-field
pattern to copy; each takes the raw value, mutates the field, and (per file-header
convention elsewhere in this codebase) the caller is responsible for the
actor-safety wrapper already present in this file (all setters in this file already
run through the actor). A `SetPinned(v bool)` following the exact shape of
`SetAutoYes` (`instance_actor_setters.go:329`) is the correct addition — no new
getter/setter ceremony needed per the interface-pollution-checklist rule already in
this repo's `.claude/rules/`.

## 2. Industry comparables (reasoned from known public product behavior, not fetched — UNVERIFIED against current live UI, label accordingly)

| Product | Pattern | Notes |
|---|---|---|
| **Slack** (pinned messages/channels) | Pinned items get a dedicated "📌 X pinned items" affordance per-channel; pinned *channels/DMs* section sits above the regular sidebar list, sorted by most-recently-active among pinned, not alphabetically. No hard limit on pinned channels; pinned messages historically capped (~100/channel), a moderation/perf limit rather than a UX one. |
| **Browser pinned tabs** (Chrome/Firefox) | Pinned tabs collapse to icon-only, move to the leftmost strip, survive window/session restore, order is user-drag-reorderable (not a magic sort). No visible count limit; a lot of pins just get very small. |
| **VS Code pinned editor tabs** | Pinned tab moves to the start of the tab strip, gets a dot/pin badge instead of the close (x) button, and is excluded from "Close Others"/"Close All" — the "you must explicitly unpin before it goes away" affordance is the important part for stapler-squad: does pinning a session change its eligibility for bulk-archive/bulk-delete actions? (see §3 edge cases). |
| **GitHub pinned repos** | Hard cap: **exactly 6** pins per profile — a deliberate curation-forcing limit, not a technical one. Order is drag-reorderable by the user; no auto-sort. |
| **herdr-web `agentPins.ts`** (per requirements doc — internal reference, not independently verified here) | Pinned panes surface at top of sidebar; described as the direct reference implementation for this feature's UX. |

**Shared conventions across all of the above:**
1. Pinned items render in a visually distinct section/position **above** the normal
   list, not just badge-decorated in place.
2. Most products with a *pin* concept (vs. a full manual-reorder concept) sort the
   pinned section by **pin recency** (most-recently-pinned first) or leave it
   user-orderable — almost none of them sort pinned items by the *same* criteria as
   the main list (e.g. alphabetical or last-activity), because the whole point of
   pinning is to escape the normal sort.
3. A visible pin badge/icon on the item itself (a filled pin icon replacing or
   alongside the item's existing badges) is universal.
4. Cap-or-no-cap is a deliberate design choice, not an accident: GitHub caps hard
   (curation), Slack/browsers/VS Code don't (utility list, not curation). Given
   stapler-squad's requirements doc explicitly says "pin ordering/reordering ... out
   of scope," a **cap is not required by the stated scope** and would be new,
   unrequested behavior — flag as an open question in §4, don't build it
   speculatively.
5. None of these examples persist pin order via a separate "pin timestamp" field if
   they don't need custom ordering — several (Slack channel list, VS Code) simply
   use "most recently pinned first," which requires *some* timestamp. Since this
   requirements doc's schema precedent (`hidden`, `auto_yes`, `is_expanded`) is
   "plain boolean, no timestamp," pinning a session gives **no stable multi-pin
   ordering** for free — see §4.

## 3. Edge cases

- **Pinning an archived session**: Requirements doc explicitly punts this to
  research ("pinning archived sessions (decide in research)"). Current behavior:
  `filteredSessions` excludes `archivedAt`-set sessions by default (line 579)
  regardless of any other flag — so today, an archived+pinned session would
  **vanish from the pinned section too** unless the "Pinned" section is
  deliberately exempted from the archived filter (mirroring requirement #4's
  "regardless of ... status"). Recommend: pinning an archived session is either (a)
  disallowed — unpin is forced on archive, mirroring VS Code's model where a
  pinned+closed tab doesn't really exist — or (b) allowed, and the Pinned section is
  explicitly exempt from the `showArchived` filter (shows archived-pinned sessions
  always). (a) is simpler and matches "archived sessions are meant to disappear";
  recommend auto-unpin on archive as the default, called out for the requirements
  owner to confirm, since it's the one item requirements deferred to research.
- **Pinning a session that's later deleted**: No special handling needed — deletion
  already removes the `Session` ent row entirely; `pinned` disappears with it. No
  orphan-pin state is possible given the boolean-field-on-entity design (unlike, say,
  a separate `PinnedSessionIDs []string` list that could go stale — another reason
  the "plain field, not a separate join/list" schema choice in the requirements is
  correct).
- **Workflow-spawned sessions**: No special interaction found — `workflow_id` and
  `pinned` are orthogonal fields on the same entity. One thing to check in planning:
  `ArchiveWorkflowSessions` (`session_service.go`) bulk-archives every session under a
  workflow — should a pinned session under that workflow be skipped/require
  confirmation, similar to the archived-session question above? Same auto-unpin
  recommendation applies for consistency.
- **Session type change**: No code path found where `session_type` changes on an
  existing session post-creation (creation-time only, per
  `.claude/rules/session-creation-registry.md`) — not an applicable edge case for
  pinning.
- **Multi-session bulk actions** (`BulkActions.tsx`): Bulk archive/hide/delete exist
  today (confirmed component exists, not read in full this pass). A bulk "Pin
  selected" / "Unpin selected" action is a natural, low-cost addition given the
  existing bulk-action UI surface, but is **not in the stated functional
  requirements** (#1 says "pinned and unpinned by the user from the UI" without
  specifying single vs. bulk) — flag as a scope question for planning, don't
  assume it's included.
- **Does "hidden" + "pinned" conflict?**: `hidden` "excludes [the session] from the
  default session list and review queue" per its schema comment
  (`session.go:102-104`). If a session is both `hidden` and `pinned`, does it show
  in the new Pinned section? Given `hidden`'s docstring is unconditional ("excluded
  from the default session list"), the safest, most consistent reading is that
  `hidden` should win — a hidden+pinned session stays out of the Pinned section too,
  same logic as the archived case above. This needs an explicit decision recorded in
  the plan, since it's not addressed in the requirements at all (only the
  archived-session variant was flagged for research).

## 4. Unstated user needs (candidates to raise with the requirements owner, not assumed in scope)

- **Pin count limit**: Not requested, and the "out of scope: pin
  ordering/reordering" line implies the team is deliberately keeping this feature
  minimal. Recommend **no cap** for v1 (Slack/VS Code/browser model, not GitHub's
  curation model) — matches "boolean field, no extra state" schema minimalism.
- **Pinned section ordering**: The requirements' explicit "out of scope: pin
  ordering/reordering" only rules out *manual drag reordering*. It does not specify
  what stable order the Pinned section uses absent reordering. Since `pinned` is a
  plain boolean with no timestamp, the only two options without adding new fields
  are: (a) apply the *same* sort as the main list (name/createdAt/updatedAt/
  lastActivity — whatever `sortField` is currently set to) to the pinned subset too,
  or (b) add a `pinned_at` timestamp for "most recently pinned first," which is a
  schema field the requirements doc doesn't mention. Recommend (a) — reuse the
  existing `sortField`/`sortDir` state already computed in `SessionList.tsx` rather
  than introducing new schema — cheapest option consistent with "no speculative
  fields," and can be revisited if users ask for recency-of-pin ordering later.
- **Keyboard shortcut**: Not mentioned in requirements; no existing session-level
  keyboard shortcuts were found for hide/archive either (would need to check
  `web-app/src/lib/hooks` for a global-keybinding hook if this becomes a real ask;
  not investigated further here since there's no existing precedent to mirror and
  it's not in scope).
- **Pin from omnibar search results**: The omnibar detector/action registries
  (`.claude/rules/feature-testing-registry.md`) are for *creating/navigating*
  sessions, not mutating existing ones — no existing "toggle a flag on a search
  result" affordance exists in `OmnibarResultList.tsx` today. Out of scope unless
  explicitly requested; would be new interaction surface, not a natural extension.

## Key files for planning phase

- `session/ent/schema/session.go` — add `pinned` field here
- `proto/session/v1/types.proto` — `Session` message, next free field = **72**
  (correcting requirements doc's "63")
- `proto/session/v1/session.proto` — add `PinSession`/`UnpinSession` RPC pair near
  `ArchiveSession`/`UnarchiveSession` (`session.proto` line ~579 region)
- `server/services/session_service.go:4285` — `ArchiveSession`/`UnarchiveSession` to
  mirror structurally
- `session/instance_actor_setters.go:329` (`SetAutoYes`) — pattern for `SetPinned`
- `web-app/src/lib/hooks/useSessionService.ts:600-618` — pattern for
  `pinSession`/`unpinSession`, but note: wiring an actual UI call site is new work,
  not a copy of an existing wired call site (archive isn't wired either)
- `web-app/src/components/sessions/SessionList.tsx:648` (`groupedSessions`) — where
  a synthetic "Pinned" group gets prepended
- `web-app/src/lib/grouping/strategies.ts` — `GroupedSessions[]` shape to reuse for
  the pinned section
- `web-app/src/components/sessions/SessionCard.tsx:98-138` — only `onDelete`/
  `onRename` mutation props exist today; `onTogglePin` would be a new prop following
  that same shape
