# Research: UX — Backlog Item Unarchive (Agent 5)

## 1. Button placement and pattern to copy

`web-app/src/components/backlog/detail/ActionsSection.tsx` is a flat sequence of
`item.status === "<x>"` conditional blocks (idea, ready, queued, in_progress, review,
done), each rendering one or more `<button className={styles.actionButton} ...>`
elements, followed by an always-visible "Backward transitions" block
(`send_back_idea`/`send_back_ready`, lines 344–369) and a final always-visible Delete
button (lines 371–379). **There is currently no `item.status === "archived"` branch at
all** — confirmed by reading the full file (385 lines); archived items fall through to
render only the backward-transitions block (which excludes "archived" from its status
list) and the Delete button. So today an archived item's detail view offers only
Delete, nothing else.

The correct pattern to copy is the `done` block (lines 320–341): two buttons,
`archive` and `reopen`, each following the exact same shape:

```tsx
<button
  className={styles.actionButton}
  onClick={() => onAction("reopen")}
  disabled={actionLoading !== null}
  aria-busy={actionLoading === "reopen"}
  data-testid="backlog-action-reopen"
>
  <ActionButtonLabel pending={actionLoading === "reopen"} label="Re-open to Review" />
</button>
```

An `item.status === "archived"` block, added as a new branch (sibling to the `done`
block, before the backward-transitions block, since "archived" isn't in that block's
status list and shouldn't be — see §2), should render one `Unarchive` button in
identical shape: `styles.actionButton` (not danger — it's a restorative action, same
tier as `reopen`), `disabled={actionLoading !== null}`, `aria-busy={actionLoading ===
"unarchive"}`, `data-testid="backlog-action-unarchive"`, wrapped in
`<ActionButtonLabel pending={actionLoading === "unarchive"} label="Unarchive" />`.

**Session-side reference is incomplete — do not assume a working example exists.** The
requirements doc cites `UnarchiveSession` as the working precedent. That RPC + handler
+ `useSessionService.ts` hook (`unarchiveSession` at line 614) do exist, but a repo-wide
search found **no button, menu item, or component anywhere in `web-app/src/components/`
that calls `unarchiveSession`** — the only reference to the symbol outside the hook
itself is a jest mock in `ConnectionIndicator.test.tsx`. `SessionList.tsx`'s "Show
archived" toggle (line ~1075, `data-testid="show-archived-toggle"`) only filters the
list to reveal archived sessions; it has no restore affordance once they're visible.
So there is no existing pixel-for-pixel UI pattern for archive→active restoration in
this codebase — the `done` block's `reopen` button (a same-page, same-shape "move
backward" action) is the closest and correct model to copy, not a session-list restore
button that doesn't exist.

## 2. Mental model: "restore to idea," not "restore to prior status"

Per the requirements doc, unarchive restores an item to `idea` status (re-triage),
not back to whatever status it held before archiving (e.g. `done`). This needs a label
that doesn't imply full state restoration.

Industry comparables split into two camps:
- **"Undo the hide, keep the object's other state"** — Gmail's "Move to Inbox" restores
  an archived email to Inbox untouched (labels, read state preserved) since Gmail
  archive never changed any state but list membership. Trello's "Send to board" restores
  an archived card to its last list. GitHub's "Reopen" on a closed issue returns it to
  `open`, not to whatever milestone/project state it had — closer to this codebase's
  model, since "open" is itself the front of the queue, not a specific stage.
- **"Undo the hide, re-enter at the front of a process"** — Notion's "Restore" from
  Trash puts a page back exactly where it was in the hierarchy (closer to Gmail's
  model, not applicable here since this is a *workflow stage* being reset, not a
  location).

This codebase's choice (archived → idea) is closest to GitHub's reopen semantics
applied to a multi-stage pipeline: an archived backlog item was, by definition, done
being worked (it can only be archived from `done`, based on the `archive` button's
placement in the `done` block) — so "restore" doesn't mean "go back to `done`," it
means "this needs a decision again," i.e., re-triage. **Label implication:** the button
should say **"Unarchive"**, not "Restore" or "Reopen" — "Reopen" is already used
(line 331, the `done`→`review` action) and reusing it for a different target status
(`idea` vs `review`) would collide with an established meaning in the same file.
"Unarchive" is also the exact verb GitHub, Trello, and this codebase's own
`UnarchiveSession` RPC already use, so it's consistent both externally and internally.
A `title` tooltip should disambiguate the destination, matching the pattern used for
`send_back_idea` (line 351: `title="Reset to Idea and clear plan approval so triage can
re-run"`) — e.g. `title="Restore to Idea for re-triage"` — so a user who expects
"unarchive = goes back to done" isn't surprised.

## 3. Accessibility conventions already in file (copy exactly)

- **`aria-busy={actionLoading === "<action>"}`** on every mutating button, tied to the
  same `actionLoading` state string used in the `onClick` action name — present on all
  buttons except the two read-only exceptions (`View Session` is an `<a>`, and
  `manual_review` doesn't set aria-busy since it just opens a form, not an async call).
- **`disabled={actionLoading !== null}`** — every button disables itself while *any*
  action is in flight (not just itself), preventing concurrent mutations. Unarchive
  must follow this exactly: `disabled={actionLoading !== null}`.
- **`aria-disabled` + `title`** pattern is reserved for buttons with a *precondition*
  beyond "another action is running" (e.g. `mark_ready` needs `acCriteria.length > 0`,
  `spawn_session` needs `canSpawnSession`). Unarchive has no such precondition (any
  `archived` item can always be unarchived) — so it should use only `disabled`/
  `aria-busy`, no `aria-disabled`/conditional `title`, matching `reopen` and
  `override_done` exactly, not `mark_ready`.
- **`data-testid="backlog-action-<snake-or-kebab-action>"`** — every action button has
  one; the convention is kebab-case matching the button's purpose, not necessarily the
  `onAction` string verbatim (e.g. action `send_back_idea` → testid
  `backlog-action-send-back-idea`). Use `data-testid="backlog-action-unarchive"` for
  action string `"unarchive"`.
- **Container-level**: the whole panel is `role="group" aria-label="Item actions"`
  (line 78) — no per-section aria-label needed for the new branch.
- **Keyboard nav**: all actions are plain `<button>` elements (native focus + Enter/Space
  activation, included in normal tab order) except `View Session` which is an `<a>` —
  no custom keyboard handling exists anywhere in this file, so none should be added for
  Unarchive; native semantics suffice and match every sibling action.
- **Terminal-state short-circuit**: note the `terminalState` prop (lines 79–87) replaces
  *all* actions including Delete with a single `InlineNotice` once an item is archived
  **by an external actor** (another tab/session) — this is a live-update guard, not the
  same thing as `item.status === "archived"` on initial load. The new Unarchive button
  must render in the normal `item.status === "archived"` branch (reached when the
  detail view is *opened* on an already-archived item), which is a different code path
  than the `terminalState` notice (reached via a live watch after the view was already
  open). Confirm in the plan phase whether `terminalState === "archived"` should also
  offer Unarchive, or intentionally stays notice-only (current behavior for all other
  external archive/removal races) — this doc flags it, doesn't resolve it, since it's
  a scope/plan decision, not a UX-research one.

## 4. Confirmation-dialog wording for archive

Current delete confirm (`BacklogItemDetail.tsx:542`):
```js
confirm("Permanently delete this item and all its history? This cannot be undone.")
```
This wording's power comes from two irreversibility markers: "Permanently" and "This
cannot be undone." — both are true statements about delete and both would now be
**false** statements about archive, since AC1 of this same requirements doc makes
archive reversible via Unarchive. Copying delete's wording verbatim onto archive would
be actively misleading (a user declining to archive because they think it's permanent,
when it isn't) — the opposite failure mode from the one this feature is fixing (users
not being warned at all). Recommended wording, matching delete's
question-then-consequence structure but accurate to archive's real (now mild)
consequence:

```js
confirm("Archive this item? It will be hidden from the default list, but can be restored later.")
```

This keeps the same `confirm()` mechanism (AC0's bar — "matching the pattern used for
delete" — is satisfied by reusing the native `confirm()` call at the same call site,
`case "archive":` in `handleAction`, line 538), while calibrating the wording to
archive's actual (lower) severity. A lighter, single-sentence confirm without a
"cannot be undone" claim is the correct choice specifically *because* archive is being
made reversible in this same change — using delete's exact irreversibility phrasing
would misinform users about the very feature being shipped.

## 5. Job-to-be-done: archive-then-regret / un-hiding

Two distinct triggers for this action surfaced by the requirements framing:
1. **Immediate regret** — user clicks Archive (previously a single click, no confirm)
   on the wrong item, or on the right item but changes their mind seconds later. AC0's
   confirmation step is the primary mitigation for this case (catches it before the
   state change happens at all); Unarchive is the recovery path for when the confirm
   didn't stop them (or they confirmed correctly, then genuinely changed their mind
   later).
2. **Delayed re-need** — user archived a `done` item as "finished, no longer relevant,"
   then later realizes the same idea should be revisited (a related item comes up, a
   duplicate is filed, priorities shift). This is why restoring to `idea` (re-triage)
   is the right target, not restoring to `done`: the item's original completion is
   stale information by the time anyone reopens it — it needs to re-enter the pipeline
   as something to be assessed again, not be treated as still-finished work. This
   matches the requirements doc's explicit design choice and is consistent with how
   `send_back_idea` already frames the same target status via its title text ("Reset to
   Idea and clear plan approval so triage can re-run").

Both jobs are reached the same way today: `BacklogPage`'s "Show Archived" checkbox
(`web-app/src/app/backlog/page.tsx:698`, `showArchivedLabel` styled checkbox) reveals
archived items in the list, user clicks into the item's detail view, and (post-fix)
finds the Unarchive button there. No list-level bulk-restore affordance is in scope
(per requirements doc's "Out of scope" section) — the single-item detail-view button is
the correct, minimal surface for both jobs.

## Sources

- `web-app/src/components/backlog/detail/ActionsSection.tsx` (full file, 385 lines)
- `web-app/src/components/backlog/BacklogItemDetail.tsx:59-75` (ACTION_SUCCESS_MESSAGES),
  `:486-571` (handleAction switch, confirm() at line 542)
- `web-app/src/components/backlog/detail/ActionButtonLabel.tsx` (full file)
- `web-app/src/components/backlog/detail/ActionsSection.queuedPlanApproval.test.tsx`
  (existing test conventions for this component)
- `web-app/src/lib/hooks/useSessionService.ts:111,614-620,1109` (`unarchiveSession` hook,
  unused by any component)
- `web-app/src/components/sessions/SessionList.tsx:1075-1083` (`show-archived-toggle`,
  no restore action)
- `web-app/src/app/backlog/page.tsx:297-300,381,698-706` (`showArchived` toggle,
  client-side filter)
- `project_plans/backlog-item-unarchive/requirements.md` (source ask + acceptance
  criteria)
