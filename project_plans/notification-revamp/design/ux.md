# UX Design: notification-revamp

Phase 3 design artifact. Scope: the six in-scope items in
`project_plans/notification-revamp/requirements.md` and the exact
components/files named in `project_plans/notification-revamp/implementation/plan.md`
(Epics 2.3, 3.1, 3.2). Read `requirements.md` and `research/ux.md` in full
before this document — mental models, accessibility rationale, and the
"Skip vs. Dismiss vs. Mark read vs. Resolve" verb analysis are not repeated
here, only applied.

Grounding checked directly in this codebase before drafting (not assumed):
`web-app/src/components/ui/Collapsible.tsx` (Radix accordion, `▸` chevron,
real `<button aria-expanded>` triggers — used for every collapsed section
below), `ReviewQueueBadge.tsx` (emoji + text abbreviation + `aria-label`,
already correct — reused, not reinvented), `SubStatusChip.tsx:137-147`
("● Idle" chip, currently suppressed at `SessionRow.tsx:352`),
`NotificationItem.tsx`'s existing `resolvedBadge`/`AutoHandledSection`
branches, and `BottomNav.tsx` (bottom tab bar: Alerts bell with a `9+`-capped
badge, a review-queue nav badge, a "More" sheet — this is the mobile shell
every mobile wireframe below stays inside of).

---

## Surface inventory

| # | Surface | Page | Plan reference |
|---|---|---|---|
| 1 | "Needs a decision" section | Notifications | Story 3.1.2 |
| 2 | Recent activity (grouped/collapsed informational) | Notifications | Story 3.1.2 |
| 3 | Auto-handled section (extended) | Notifications | Story 3.1.3 |
| 4 | Empty "needs a decision" state | Notifications | Story 3.1.2 |
| 5 | Mid-review-race badge/blocked-message | Notifications | Story 2.3.2 |
| 6 | "Needs a decision" priority tier | Review Queue | Story 3.2.1 |
| 7 | "Informational" (Low-priority) collapsed tier | Review Queue | Story 3.2.1 |
| 8 | Empty "needs a decision" state | Review Queue | Story 3.2.1 |
| 9 | Mid-review-race transient banner | Review Queue | Story 2.3.2 |
| 10 | Idle status chip | Sessions list | Story 3.2.2 |
| 11 | Header bell dropdown (`NotificationPanel`) action scoping | Global (mounted every page, `layout.tsx:68`) | Story 3.1.5 |

Eleven interactive surfaces. Headline unread-count capping (Story 3.1.4) is a
one-line numeric change to an existing element, not a new surface — its
acceptance criterion is folded into Surface 1's below.

`NotificationPanel` (Surface 11) was missing from this inventory through
round 2 of Product Triad Review. **That was an oversight, not a deliberate
scoping decision**: Task 3.1.2e's own text already noted, without flagging it
as a live bug, that "`NotificationPanel`'s compact dropdown keeps passing
`removeFromHistory` unchanged" — the dropdown was mentioned in passing while
fixing the Notifications page, never audited as its own surface. Round 3
closes that gap; see Surface 11 below.

---

## Part A — Notifications page

### Surface 1: "Needs a decision" section

**What it is**: `NeedsDecisionSection` (Task 3.1.2b), always the first thing
rendered on the page, always expanded, holding unread notifications where
`isActionableNotification(type)` is true (`approval_needed`, `question`,
`error`, `task_failed`, `warning`).

**Desktop wireframe**:
```
┌─ Notifications ────────────────────────────────────── [ Mark activity read ] ┐
│                                                                            │
│  NEEDS A DECISION · 3                              (aria-live="polite")   │
│  ┌──────────────────────────────────────────────────────────────────┐    │
│  │ ⚠ approval_needed · sess-a1b2c3 · 2m ago                          │    │
│  │ Bash: rm -rf /tmp/scratch                                         │    │
│  │ [ Approve ]  [ Deny ]                                             │    │
│  ├──────────────────────────────────────────────────────────────────┤    │
│  │ ❓ question · sess-d4e5f6 · 8m ago                                │    │
│  │ "Which branch should I target?"                                  │    │
│  │ [ Open session ]                                                  │    │
│  ├──────────────────────────────────────────────────────────────────┤    │
│  │ ✗ error · sess-g7h8i9 · 14m ago                                  │    │
│  │ Test suite failed: 3 failures in ui/                             │    │
│  │ [ Open session ]                                                  │    │
│  └──────────────────────────────────────────────────────────────────┘    │
│                                                                            │
│  ▸ Recent activity (24, collapsed)                                       │
│  ▸ Auto-handled (12, collapsed)                                          │
└────────────────────────────────────────────────────────────────────────┘
```

**Mobile wireframe** (stays inside the existing bottom-tab-bar shell —
`BottomNav.tsx`'s Alerts tab is how the user arrived here):
```
┌──────────────────────────┐
│ ← Notifications          │
│                          │
│ NEEDS A DECISION · 3     │  aria-live="polite"
│ ┌──────────────────────┐ │
│ │ ⚠ approval_needed     │ │
│ │ sess-a1b2c3 · 2m ago  │ │
│ │ Bash: rm -rf /tmp/... │ │
│ │ ┌────────┐┌────────┐ │ │
│ │ │Approve ││ Deny   │ │ │  ≥44×44pt tap targets,
│ │ └────────┘└────────┘ │ │  stacked full-width on
│ └──────────────────────┘ │  narrow viewports
│ ┌──────────────────────┐ │
│ │ ❓ question           │ │
│ │ sess-d4e5f6 · 8m ago  │ │
│ │ "Which branch..."     │ │
│ │ ┌──────────────────┐ │ │
│ │ │  Open session    │ │ │
│ │ └──────────────────┘ │ │
│ └──────────────────────┘ │
│ …                        │
│ ▸ Recent activity (24)   │
│ ▸ Auto-handled (12)      │
│                          │
├──────────────────────────┤
│ [Sessions][Queue•1][Alerts•3][New][More] │ ← BottomNav, unchanged shell
└──────────────────────────┘
```

**Interaction flow**:
1. Page loads → `NeedsDecisionSection` renders first, above the fold, always
   expanded (no click needed to see it — this is the whole point of the IA
   reboot).
2. User acts on an item (Approve/Deny/Open session) → item leaves the
   section immediately (optimistic) and the count decrements. **There is no
   ✕/dismiss control on a `NeedsDecisionSection` item** (Product Triad
   Review round-2 blocker fix): `NotificationItem`'s existing
   `removeFromHistory` control deletes a record from local history with no
   interaction with the underlying approval/session state at all — wiring
   it into this section unchanged would let a user "dismiss" a still-pending
   `approval_needed`/`question`/`error` item while the real decision stays
   unresolved server-side, reproducing at the single-item level the exact
   "items don't stay resolved" failure mode this project's Success Metrics
   rule out for bulk actions (`requirements.md`: "An item leaves the
   needs-a-decision view only by being resolved... or because the
   underlying state that put it there changes"). An item in this section can
   only leave it by being acted on (Approve/Deny/Open session) or by its
   underlying state resolving elsewhere (this surface's edge cases below) —
   never by an unscoped remove-from-history click. `removeFromHistory`
   remains available on Recent Activity and Auto-handled items, where it is
   safe because those items are already read/resolved/informational.
3. Every ~2s poll tick, a genuinely new actionable item can appear at the
   top of the section; the `aria-live="polite"` region announces only the
   updated count — "3 items need a decision," or singular "1 item needs a
   decision" when the count is exactly 1 — not the full list (per
   `research/ux.md`'s accessibility guidance — `polite`, not `assertive`,
   short announcement, region present from first paint; see AC30).
4. Header's unread badge caps at "99+" (Story 3.1.4) — matches `NavBadge.tsx`
   and `BottomNav.tsx`'s existing bell-badge convention exactly (`9+` there,
   `99+` on this page's own header, per Task 3.1.4a's literal cap value).
5. **The bulk "Mark activity read" button never touches this section.** It
   is deliberately renamed from "Mark all read" (Task 3.1.2e) and scoped to
   mark only unread items in Surface 2 (Recent activity) and Surface 3
   (Auto-handled) as read — never an item in `NeedsDecisionSection`. This is
   the fix for the blocker this project's own Product Triad Review flagged:
   the old unscoped "Mark all read" emptied this section in one click,
   including pending `approval_needed`/`question`/`error` items that were
   never actually resolved — reproducing exactly the "items don't stay
   resolved" failure mode this project exists to fix. An item leaves
   `NeedsDecisionSection` only by being acted on (Approve/Deny/Open
   session) or by its underlying state resolving elsewhere (this
   surface's own edge cases below) — never via a bulk read action, and
   never via the per-item ✕ control, which does not exist for items in this
   section (see point 2 above). The button itself is enabled only when
   Recent Activity or Auto-handled has at least one unread item, independent
   of `NeedsDecisionSection`'s own unread count — so it's never shown active
   with nothing it can actually affect. A clearer label was chosen over
   adding a confirmation dialog: once the action can no longer touch
   anything unresolved, there's nothing left for a confirmation to protect
   against, and a label that names the actual scope ("activity," not "all")
   resolves the ambiguity a generic "Mark all read" would otherwise invite.
6. **Accepted trade-off: "Open session" on a `question`/`error`/
   `task_failed`/`warning` item marks it read on click, with no guarantee
   the user actually resolved the underlying issue in the session they were
   taken to.** This is asymmetric with `approval_needed` (where "resolved"
   means an explicit Approve/Deny RPC succeeded) and with `error`/
   `task_failed` specifically (which can self-heal: if the same error
   recurs, Epic 1.1's dedup fix flips the existing record back to unread on
   the next occurrence, so a truly-unresolved recurring error doesn't stay
   silently marked read forever). A `question` has no such backstop — a user
   who opens the session and never actually answers the question leaves it
   marked read with nothing to un-flip it. **Decision: acceptable, not
   fixed.** Opening the session is itself meaningful engagement (the user
   saw the question and chose to act), and this project's own mandate is to
   make the "needs a decision" *view* trustworthy, not to build session-
   completion detection — the review queue's `approval_pending` mechanism
   (Surfaces 6/9) is the actual backstop for anything that requires a
   provable, machine-checkable decision; a chat `question` is inherently a
   softer, human-judgment interaction that this project's Appetite/Rabbit
   Holes scope (`requirements.md`) does not cover extending. Documented here
   per this project's established pattern of writing down accepted
   trade-offs rather than leaving them unstated.

**Error / edge cases**:
- **Approve/Deny RPC fails** (network, backend error): the item stays in the
  section, buttons re-enable, an inline error line renders under the two
  buttons: "Couldn't record your decision — try again." with the same
  Approve/Deny buttons still present (no dead end — the retry path *is* the
  original action, not a separate one).
- **Item resolved elsewhere between poll ticks** (e.g. approved from a second
  device — the mobile-web scenario the requirements screenshots came from):
  next poll tick simply removes it from the list; no error surfaces, since
  nothing failed.
- **Mid-review race** (rule-reconciliation resolves an item the user has
  open): see Surface 5 — never a silent removal.
- **Open session for a `question`/`error`/`task_failed`/`warning` item marks
  it read without a guarantee the underlying issue was actually resolved in
  the session it opens** — see the accepted-trade-off note at the end of
  this surface's Interaction flow.
- **Focus management on removal**: if the item a user just acted on
  (Approve/Deny/Open session) was keyboard-focused, focus moves to
  the next remaining item in `NeedsDecisionSection`; if it was the last item
  in the section, focus moves to the section's own heading/container
  (`NEEDS A DECISION` header) rather than being dropped to `<body>` — the
  standard accessible-list-removal pattern, applying equally to Surface 9's
  ~5s auto-removal on the Review Queue page.
- **Background poll failure** (the ~2s history-fetch itself errors — not a
  per-action RPC failure, which is covered above): show the last successfully
  fetched `NeedsDecisionSection` list/count exactly as it was, with a small
  staleness indicator next to the section heading — "Last updated 3m ago ·
  Retry" — rather than either a full error takeover or, worse, silently
  continuing to show a now-possibly-stale "All caught up." The one thing this
  must never do is let a stale empty state read as current: if the last
  successful fetch was itself empty, the staleness text still renders next to
  it, so "caught up" is never presented as fresher than it actually is.
  "Retry" re-runs the same poll immediately rather than waiting for the next
  ~2s tick. Once a subsequent poll succeeds, the indicator disappears and the
  section resumes normal live updates — this is a lightweight text
  affordance, not a new banner/toast component. Implemented by Task 3.1.2h;
  see AC38.

---

### Surface 2: Recent activity (grouped/collapsed informational section)

**What it is**: everything not actionable-and-unread, grouped by
`(sessionId, notificationType)` via the existing `groupNotifications()`
utility, wrapped in a `CollapsibleSection` (Task 3.1.2c) collapsed by
default. This is Gmail's collapsed-thread move, already half-built in this
codebase (`research/ux.md` §1) — the fix is that it's collapsed by default
and demoted below Surface 1, not that grouping is new.

**Desktop wireframe (expanded)**:
```
│  ▾ Recent activity · 24                                                  │
│    ┌──────────────────────────────────────────────────────────────┐    │
│    │ ✓ task_complete · sess-j1k2l3 · ×6            [collapsed group]│    │
│    │   "Claude turn complete" — last: 3m ago                        │    │
│    ├──────────────────────────────────────────────────────────────┤    │
│    │ ℹ progress · sess-m4n5o6 · ×3                                  │    │
│    │   "Running tests" — last: 11m ago                              │    │
│    └──────────────────────────────────────────────────────────────┘    │
```

**Mobile wireframe**: identical structure, full-width cards, same `▸`/`▾`
accordion trigger — no mobile-specific layout divergence needed here since
it's a vertically-stacked list either way (the existing dense-card mobile
convention already matches this).

**Interaction flow**:
1. Collapsed by default on every page load (not remembered as "was open" —
   consistent, predictable starting state per session, per Task 3.1.2c).
2. Click/tap the header (`Accordion.Trigger`, a real button, not a `div`) →
   expands in place; count in the header stays visible while expanded.
3. Clicking a grouped row expands it to show individual occurrences (Gmail
   thread-expand pattern) — this uses `groupNotifications()`'s existing
   `count = max(occurrenceCount, group.length)`; once the backend dedup fix
   (Epic 1.1) lands, `occurrenceCount` alone is authoritative and the
   `group.length` fallback in that utility becomes dead code to remove in a
   follow-up cleanup, not part of this design.

**Error / edge cases**: no new error states — this section only reads
already-resolved/informational history; there is nothing here that can fail
an in-flight action, since none of its items are actionable.

---

### Surface 3: Auto-handled section (extended)

**What it is**: the existing `AutoHandledSection` (`NotificationItem.tsx`
~line 304), extended per Task 3.1.3a to include records where
`metadata.reconciled === "true"`, not just `notificationType ===
"auto_approved"`. Additive to an existing, already-tested section — not new
IA.

**Wireframe** (desktop and mobile share this layout — it's already a
collapsed list on both):
```
│  ▸ Auto-handled · 12                                                     │
│    (collapsed — expand to see:)                                         │
│    ┌──────────────────────────────────────────────────────────────┐    │
│    │ ✓ Auto-approved · sess-p7q8r9 · Bash: git status               │    │
│    │   Rule: "Allow read-only git checks"                           │    │
│    ├──────────────────────────────────────────────────────────────┤    │
│    │ ✓ Auto-resolved by rule: "Auto-allow safe git status checks"   │    │
│    │   sess-a1b2c3 · was pending 4m before this rule was added      │    │
│    └──────────────────────────────────────────────────────────────┘    │
```

**Interaction flow**:
1. A rule-reconciled item never appears in Surface 1 or Surface 2 —
   `filteredNotifications` excludes `metadata.reconciled === "true"` at the
   same point it excludes `auto_approved` (Task 3.1.3a), so there is exactly
   one place a reconciled item can be seen, matching the "one audit trail"
   framing in `requirements.md`'s Success Metrics.
2. Visually distinguishable from a live auto-approval: live auto-approvals
   say "Auto-approved"; reconciled items say **"Auto-resolved by rule:
   `<name>`"** verbatim (per Story 2.3.2's copy) — never collapsed into the
   same label, since a user auditing later needs to tell "the classifier
   decided this live" from "a rule I added later cleared out a backlog item"
   (`research/ux.md`'s mental-model section on Resolve's permanence).

**Error / edge cases**: none new — this is read-only history, same as
Surface 2.

---

### Surface 4: Empty "needs a decision" state (Notifications page)

**What it is**: `NeedsDecisionSection` rendered with zero items. Per
`requirements.md`'s Success Metrics and `research/ux.md` §4, **this is the
success condition**, not an error or loading state — the entire point of the
project is a page the user trusts enough that "nothing to decide" reads as
"caught up," not "broken."

**Wireframe** (identical shape on desktop/mobile — this is a small, centered
block, not a full-page takeover):
```
┌──────────────────────────────────────────┐
│  ✓  All caught up                         │
│     Nothing needs your attention right now │
│                                            │
│  ▸ Recent activity (24, collapsed)        │
│  ▸ Auto-handled (12, collapsed)           │
└────────────────────────────────────────────┘
```

**Loading-state wireframe** (shown before the first successful poll
response — see Error/edge cases below; identical shape reused verbatim by
Surface 8 on the Review Queue page, per that surface's own note):
```
┌──────────────────────────────────────────┐
│  ░░░░░░░░░░░░░░░░░░░░░░░░  (skeleton)     │
│                                            │
│  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │
│  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │
│  ░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░░  │
└────────────────────────────────────────────┘
```
Three skeleton rows (not a bare spinner) so the shape of the page — a
section, then stacked rows — is recognizable immediately, matching the
existing `⏳ Loading notifications...` treatment's intent
(`NotificationPanel.css`'s `empty`/`emptyIcon` classes) but rendered as
row-shaped placeholders rather than a centered icon+text block, since a
centered "Loading..." message in the same visual slot as "All caught up"
is exactly the ambiguity AC17 exists to prevent — the loading state should
be visually distinct from the empty state at a glance, not just
textually different.

**"Hidden by filter" wireframe (Product Triad Review round-4 blocker fix —
new variant, distinct from both states above)**: `filteredNotifications`
(and therefore `NeedsDecisionSection`'s `notifications` prop) is already
computed by intersecting the full unread-actionable set with the page's own
type/search/hide-backlog filters (`NotificationsPage.tsx:85-109`). A user
who has, say, the type filter set to "Error" while the only unread
actionable items are `approval_needed` sees a *filtered* Needs a Decision
list of zero — but the true, unfiltered count is not zero. Rendering "All
caught up" in that case is a false calm state exactly as dangerous as the
BLOCKER this design already fixed for bulk "Mark activity read" (Surface 1
AC6/AC7): it tells the user nothing needs attention when something does,
directly contradicting the Success Metric. This state is distinct from the
page's pre-existing whole-list `hasActiveFilter` empty state
(`NotificationsPage.tsx:111,247-258`, "🔍 No matching notifications") — and,
**per round 5's correction, now takes precedence over that legacy branch
whenever it applies, not only when `recentActivity`/
`autoHandledNotifications` happens to stay non-empty.** Round 4 fixed this
state only for the case where the *other* tier remains non-empty after
filtering, because that was the only case the legacy
`filteredNotifications.length === 0` check (which sits ahead of
`NeedsDecisionSection` in the page's render order) let through to
`NeedsDecisionSection` at all. A filter that empties *both* tiers at once —
the common case: e.g. the type filter set to "Error" while the only unread
notifications in history are `task_complete` — makes
`filteredNotifications.length === 0` true, so the page's outer branch never
reaches `NeedsDecisionSection` and falls through to the generic "No
matching notifications" message instead, with no signal that
`totalActionableCount` is nonzero. This reproduces the exact false-calm
failure class round 4 already fixed, via the one code path round 4's fix
didn't touch. **The fix is a reorder, not a third variant**: the page's
outer branch must check "is `totalActionableCount > 0` while `needsDecision`
is empty?" *before* checking "is `filteredNotifications` empty?" (see Task
3.1.2c). Once reordered, this state fires whenever a filter hides every
actionable item, whether or not `recentActivity` is also hidden by that same
filter — `recentActivity`/`autoHandledNotifications` are typically, but no
longer necessarily, non-empty when this renders:
```
┌──────────────────────────────────────────┐
│  ⚠  2 items need a decision, but are      │
│     hidden by your filter                 │
│     [ Clear filter ]                      │
│                                            │
│  ▸ Recent activity (24, collapsed)        │
│  ▸ Auto-handled (12, collapsed)           │
└────────────────────────────────────────────┘
```
Singular/plural matches the existing `aria-live` count convention ("1 item
needs a decision, but is hidden by your filter"). "Clear filter" resets
exactly the three state variables `NotificationsPage.tsx`'s own
`hasActiveFilter` already tracks (`searchQuery`, `typeFilter`,
`hideBacklogItems`) — the same reset a future "Clear filter" affordance
elsewhere on this page would need, not a bespoke reset invented for this
one control (Task 3.1.2b/3.1.2c).

**Interaction flow**:
1. Copy is exactly "All caught up" + "Nothing needs your attention right
   now" (Task 3.1.2b/3.1.2d's specified copy) with a checkmark icon — never
   generic "No items found" or a ghost/empty-box illustration (those read as
   "the filter is broken," per `research/ux.md`'s explicit warning) —
   **and only when the true, unfiltered actionable count is 0** (see below).
2. Surface 2 and Surface 3 remain visible and collapsed below it — the empty
   state does not expand to fill the space or hide the audit trail (Task
   3.1.2b, and `research/ux.md`'s "do not collapse the recent-activity
   section into the empty space" guidance). This holds for the "hidden by
   filter" variant too.
3. This state is covered by a golden-fixture test (Task 3.1.2d) so a future
   change to the section's conditional rendering can't silently regress it
   back to a generic empty-state string.
4. **Emptiness is never decided from the filtered list alone.**
   `NeedsDecisionSection` is passed a second count — the unread +
   `isActionableNotification` total computed over the *unfiltered*
   `notificationHistory`, not `filteredNotifications` (Task 3.1.2c) — and
   picks between the two empty states by comparing it to the filtered
   list's own (now possibly zero) length: unfiltered count `0` → calm "All
   caught up"; unfiltered count `> 0` while the filtered list is empty →
   "hidden by filter" (this can only happen when a filter is actually
   active, since with no filter applied the filtered and unfiltered sets
   are identical — no separate `hasActiveFilter` check is needed on the
   component itself). See AC18.
5. **This check runs, and can render, before the page ever evaluates its own
   pre-existing whole-list `filteredNotifications.length === 0` branch
   (round-5 correction)** — that legacy branch is reachable only once this
   one has determined the unfiltered actionable count is itself `0`. Without
   this ordering, a filter that empties `recentActivity` at the same time it
   empties `needsDecision` would skip `NeedsDecisionSection` entirely and
   fall through to the legacy generic message, silently reintroducing the
   false-calm failure this surface exists to prevent. See Task 3.1.2c.

**Error / edge cases**: The one thing to avoid, for either empty variant:
never render "All caught up" while data is still loading (a loading
skeleton, not "All caught up," must show before the first successful poll
response — a false-positive "caught up" during a slow initial load would be
exactly the kind of trust-eroding bug this project exists to prevent), and
never render "All caught up" while a filter is actively hiding a nonzero
number of actionable items (the "hidden by filter" variant above — see
AC18).

---

### Surface 5: Mid-review-race — Notifications-page badge/blocked-message

**What it is**: the notification-side half of Epic 2.3's race handling
(Story 2.3.2, Tasks 2.3.2a/b) — what a user sees on the Notifications page
if rule-reconciliation resolves an approval they still have open, or if
their own click loses that race.

**Flow diagram** (two branches from the same race):
```
User has approval_needed item open in NeedsDecisionSection
                    │
        ┌───────────┴────────────┐
        │                        │
 Reconciliation wins       User's click wins
 (resolves it first)       (arrives first)
        │                        │
        ▼                        ▼
 Item's resolved badge     Normal "✓ Approved"/
 renders on next poll:     "✗ Denied" badge —
 "✓ Auto-resolved by       reconciliation's later
 rule: <name>"             attempt is a no-op
 (not plain "✓ Approved")  (idempotent Remove())
        │
        ▼
 If the user's click was ALREADY in flight when
 reconciliation won: RPC returns CodeFailedPrecondition
 with message "already auto-resolved by rule
 <name> while you were reviewing it — no action
 needed" → caught by useApprovalResolution.ts's
 existing FailedPrecondition branch → rendered in
 the existing blockedApprovals inline-message slot:

 ┌────────────────────────────────────────────┐
 │ ⚠ Already auto-resolved by rule             │
 │   "Auto-allow safe git status checks" while  │
 │   you were reviewing it — no action needed.  │
 │                                    [ Deny ]  │  ← only Deny renders;
 └────────────────────────────────────────────┘     no "Approve anyway" —
                                                      nothing left to
                                                      approve against
                                                      (Task 2.3.2b)
```

**Interaction flow**:
1. The user is never shown a silent disappearance — either the badge updates
   in place with attribution, or (if their own action was mid-flight) they
   get the specific "already auto-resolved by rule `<name>`" message,
   reusing the existing CI-block inline-message UI slot rather than a new
   component (Task 2.3.2a — this is deliberate reuse, not a gap).
2. Because the message routes through the same `blockedApprovals` state the
   CI-block flow already uses, it's visually consistent with an existing,
   already-accessible pattern rather than a bespoke toast.

**Error / edge cases**: this section *is* the edge-case handling for the
race itself. The one failure mode to guard: never fall into the old generic
`catch` branch that used to set a plain `"expired"` badge for this specific
error — Task 2.3.2a's acceptance criterion is exactly that a
`CodeFailedPrecondition` with this message lands in `blockedApprovals`, not
the generic `resolvedApprovals[id] = "expired"` path.

---

### Surface 11: Header bell dropdown (`NotificationPanel`) action scoping

**What it is**: `NotificationPanel.tsx` — the compact, globally-mounted
header bell dropdown (`layout.tsx:68`), present on every page, not just the
Notifications page. It renders a single flat, ungrouped list of
`NotificationItem`s (via `groupNotifications(filteredNotifications)`, no
`NeedsDecisionSection`/`CollapsibleSection` split) plus its own "Mark all
read", "Clear all", and per-item ✕ controls — three controls that,
unaudited, carry the exact same class of bug already fixed (Mark all
read) or specified (✕) for Surface 1, plus a fourth ("Clear all") never
scoped anywhere in this project at all.

**Decision: scope the existing controls in place, do not give this dropdown
its own tiered IA.** A `NeedsDecisionSection`-style pinned top tier is
disproportionate to what this surface is for — a quick glance/triage
affordance reachable from anywhere, not a dedicated review workspace (that's
what the full Notifications page is for). Instead, this surface reuses the
*exact same* `isActionableNotification` predicate (Task 3.1.1a) that already
defines Surface 1's "needs a decision" membership, applied inline per item
and per bulk action rather than via a second component:

**Wireframe** (unchanged layout — only the button labels and per-item ✕
visibility change; no new section, no new visual hierarchy):
```
┌─ Notifications ────────────────────────── [Mark activity read][Clear history] ✕ ┐
│ [Search…]  [All][Approval][Error][Task][Info]                                   │
│ ┌──────────────────────────────────────────────────────────────────────────┐  │
│ │ ⚠ approval_needed · sess-a1b2c3 · 2m ago            (no ✕ — unread,       │  │
│ │ Bash: rm -rf /tmp/scratch                            actionable)          │  │
│ │ [ Approve ]  [ Deny ]                                                     │  │
│ ├──────────────────────────────────────────────────────────────────────────┤  │
│ │ ✓ task_complete · sess-j1k2l3 · 6m ago                              [✕]   │  │
│ └──────────────────────────────────────────────────────────────────────────┘  │
│ ▸ Auto-handled (12)                                                           │
└────────────────────────────────────────────────────────────────────────────┘
```

**Interaction flow**:
1. **"Mark all read" → renamed "Mark activity read"**, identical fix to
   Surface 1's Task 3.1.2e: never marks an unread `approval_needed`/
   `question`/`error`/`task_failed`/`warning` item as read. Enabled only when
   at least one unread non-actionable-or-already-actionable-but-read item
   exists to mark — never shown active with nothing it can affect. Both this
   button and Task 3.1.2e's Notifications-page button call one shared helper
   (`computeScopedMarkReadIds`, Task 3.1.5a) so the scoping rule is defined
   once, not reimplemented per surface.
2. **Per-item ✕ dismiss**: exempted for the same class of item as Surface 1
   (Task 3.1.2b), but applied inline per notification rather than via a
   section component, since this dropdown doesn't split into two visual
   tiers — `removeFromHistory` is passed to each `NotificationItem` only when
   that item is *not* both unread and actionable; otherwise `undefined` is
   passed, using the exact optional-prop mechanism Task 3.1.2b already adds
   to `NotificationItem`. Once the same item is read (or its underlying
   decision resolves), it regains its ✕ control on the very next render —
   this is a live per-render condition, not a one-time flag.
3. **"Clear all" → renamed "Clear history"**, and fixed at the layer where
   it can actually be guaranteed for every caller: `ClearNotificationHistoryRequest`
   has no per-item exclusion or ID list today, only an optional
   `before_timestamp` cutoff (confirmed by reading
   `proto/session/v1/session.proto:1661-1665` and
   `server/notifications/store.go:326-352`'s `Clear()`) — a client-side
   filter could not honor an exclusion the RPC itself doesn't support, since
   `Clear()` deletes by timestamp cutoff across the *entire* store, not a
   caller-supplied ID set. The fix is therefore server-side (Task 3.1.5c):
   `Clear()` gains an unconditional, non-caller-toggleable guarantee that it
   never deletes a record that is both unread and actionable, regardless of
   the `before_timestamp` cutoff. This fixes both `NotificationPanel` and
   `NotificationsPage.tsx`'s own "Clear all" (which had the identical,
   previously-undocumented gap — neither page nor dropdown scoped this
   control before round 3) from one place, with no risk of the two frontend
   call sites drifting out of sync with each other.
4. Because deletion is irreversible — unlike marking something read, which
   only changes a display flag — "Clear history" additionally gets a native
   `confirm()` dialog before it runs, on both surfaces: "Clear N read
   notifications? This can't be undone. Items still needing a decision won't
   be cleared." This is a *stronger* guard than Surface 6's "Skip all" confirm
   (AC9), which was judged sufficient on its own precisely because a skip is
   never durable/permanent — a cleared record has no such backstop, so this
   control gets both the exclusion *and* the confirmation, not a choice
   between them.

**Error / edge cases**: same as Surface 1 for Approve/Deny/Open session
failures (this dropdown reuses the identical `NotificationItem` rendering,
including its inline-retry treatment). "Clear history" failing (network
error) leaves the dialog dismissed and the list unchanged, with no partial-
delete state possible — `Clear()` is a single atomic store operation, not a
per-item loop.

---

## Part B — Review Queue page

### Surface 6: "Needs a decision" priority tier

**What it is**: `ReviewQueuePanel`'s existing per-item list, partitioned
(Task 3.2.1a) so `PriorityUrgent`/`PriorityHigh`/`PriorityMedium` items
render in an always-expanded top section; the existing `groupingStrategy`
(Category/Tag/Branch/etc.) still applies *within* this tier when selected.

**Desktop wireframe**:
```
┌─ Review Queue ───────────────────────────────── [ ⏭ Skip all (2) ] ─────┐
│ Queue: 2 total · 1 urgent, 1 high            [Grouping: None ▾][Filters]│
│                                                                          │
│ NEEDS A DECISION                                                        │
│ ┌──────────────────────────────────────────────────────────────────┐  │
│ │ 🔴 URGENT  approval_pending          sess-a1b2c3 · agent-shell    │  │
│ │ Bash: rm -rf /tmp/scratch                                         │  │
│ │ [ Approve ] [ Deny ] [ Create rule ]                              │  │
│ ├──────────────────────────────────────────────────────────────────┤  │
│ │ 🟡 HIGH    input_required             sess-d4e5f6 · frontend-fix  │  │
│ │ "Which branch should I target?"                                  │  │
│ │ [ Open session ]                                  [ ⏭ Skip ]     │  │
│ └──────────────────────────────────────────────────────────────────┘  │
│                                                                          │
│ ▸ Informational (18, collapsed) — low priority, no action needed       │
└──────────────────────────────────────────────────────────────────────┘
```
*(Priority badge reuses `ReviewQueueBadge`'s existing emoji + text +
`aria-label` pattern verbatim — 🔴 URGENT, 🟡 HIGH, etc. — not a new chip.
No Skip button on the `approval_pending` row: `ReviewQueuePanel.tsx`'s
per-item Skip control is already conditioned on the item carrying no
`pending_approval_id` metadata — an approval-pending item requires an
explicit Approve/Deny, never a skip, and the wireframe now matches that
real, safe behavior instead of drawing the contradicting picture.)*

**Mobile wireframe**:
```
┌──────────────────────────┐
│ ← Review Queue      2·1  │  header count = needs-decision total (not raw queue length)
│ [Grouping ▾] [Filters]   │
│                          │
│ NEEDS A DECISION         │
│ ┌──────────────────────┐ │
│ │ 🔴 URGENT             │ │
│ │ approval_pending      │ │
│ │ sess-a1b2c3           │ │
│ │ Bash: rm -rf /tmp/... │ │
│ │┌───────┐┌───────┐    │ │
│ ││Approve││ Deny  │    │ │  full-width, ≥44pt
│ │└───────┘└───────┘    │ │
│ │[ Create rule ]       │ │  no Skip — approval_pending
│ └──────────────────────┘ │  requires an explicit decision
│ ┌──────────────────────┐ │
│ │ 🟡 HIGH               │ │
│ │ input_required        │ │
│ │ sess-d4e5f6           │ │
│ │ "Which branch..."     │ │
│ │┌────────────────────┐│ │
│ ││   Open session     ││ │
│ │└────────────────────┘│ │
│ │           [ ⏭ Skip ] │ │
│ └──────────────────────┘ │
│ ▸ Informational (18)     │
├──────────────────────────┤
│ [Sessions][Queue•2][Alerts][New][More] │
└──────────────────────────┘
```

**Interaction flow**:
1. `Approve`/`Deny`: existing behavior, unchanged by this project — resolves
   through `ApprovalService.ResolveApproval` as today.
2. `Skip`: per-item, matches the existing `onSkipSession` action; per
   `research/ux.md`'s mental-model analysis, "Skip" keeps its current label
   (its session-scoped, temporary contract already matches Success Metrics'
   "stays out until state materially changes") — the fix (Epic 1.2) is
   making that promise actually hold for idle-class items, not renaming the
   verb.
3. `Create rule`: existing flow, unchanged.
4. `⏭ Skip all (N)`: existing bulk action, gated by a native confirm dialog
   ("Skip all N visible items? This removes them from the review queue.")
   before it runs. **Verified safe against the same failure mode as the old
   unscoped "Mark all read" (Product Triad Review follow-up check, no code
   change needed):** it already excludes every approval-pending item —
   those require an explicit Approve/Deny, never a blanket dismissal — and
   for the other reasons it can reach (`input_required`, `error_state`,
   `tests_failing`, alongside the Informational/Low-priority tier),
   `Determine()` (`session/review_queue_determiner.go`) applies no
   acknowledgment-suppression to any of those three reasons the way it does
   for Idle/Stale. A skip on one of them is therefore never durable: if the
   underlying condition still holds, the item reappears on the very next
   ~2s poll tick regardless of the skip. So unlike the old "Mark all read"
   (which could permanently hide a still-pending decision with no
   reappearance mechanism), "Skip all" cannot silently and durably clear a
   genuinely pending decision — the confirm dialog it already has is
   sufficient, and no additional undo/confirmation is being added here.

**Error / edge cases**:
- **Approve/Deny fails**: same inline-retry treatment as Surface 1 — buttons
  re-enable with an error line, no dead end.
- **Skip fails** (network error): item stays in place, a small inline "Skip
  failed — try again" replaces the button label briefly, button remains
  clickable.
- **Bulk skip partially fails**: existing `ReviewQueuePanel.tsx` behavior
  (`failed` array from `acknowledgeSessions`) is unchanged by this plan —
  already reports which sessions failed rather than an all-or-nothing
  silent failure.
- **Background poll/fetch failure** (the panel's existing 30s fallback poll
  or a `WatchReviewQueue` stream reconnect fails — not a per-action RPC
  failure, which is covered above): today `ReviewQueuePanel.tsx:1196-1205`
  replaces the *entire* panel with a "Failed to load review queue" message
  plus a Retry button the moment `error` is set, discarding whatever items
  were already loaded even though they remain in state untouched
  (`reviewQueueSlice.ts`'s `setError` never clears `reviewQueue`). Per this
  surface's own Success Metric, that full takeover is disproportionate when
  stale-but-real data already exists: show the last-known queue/tiers
  exactly as they were, with the same "Last updated `<Xm ago>` · Retry"
  indicator as Surface 1 (Task 3.1.2h) next to the "Needs a decision"
  heading, and reserve the full "Failed to load" replacement for the case
  where there is no data to fall back on yet (first-load failure). "Retry"
  calls the panel's existing `refresh()`. Implemented by Task 3.2.1d; see
  AC38.

---

### Surface 7: "Informational" (Low-priority) collapsed tier

**What it is**: `PriorityLow` items, in a `CollapsibleSection` collapsed by
default (Task 3.2.1b) — this is where the "47 idle, 39 stale" clutter from
the audited screenshots stops competing with the two items that actually
matter, per the requirements' Problem Statement. Note: idle-reason items are
**not** here — they're removed from the queue entirely (Surface 10); this
tier is for other genuinely-low-priority-but-still-queue-worthy items only.

**Wireframe** (desktop and mobile share this shape — same as Surface 2's
reasoning: it's a stacked list either way):
```
│ ▸ Informational · 18 — low priority, no action needed                   │
│   (collapsed — expand to see:)                                          │
│   ┌────────────────────────────────────────────────────────────────┐  │
│   │ ⚪ LOW  stale · sess-s1t2u3 · no output in 30m, not acknowledged │  │
│   │                                                    [ ⏭ Skip ]   │  │
│   └────────────────────────────────────────────────────────────────┘  │
```

**Interaction flow**: identical Skip/grouping-strategy interactions as
Surface 6, just under a collapsed header by default. Expanding this section
does not change any counts elsewhere (nav badge, page header) — only
`Needs a decision`-tier items count toward those, so expanding for a look
never causes a "count went up" surprise.

**Error / edge cases**: same as Surface 6 (Skip can fail the same way);
no additional cases specific to being collapsed.

---

### Surface 8: Empty "needs a decision" state (Review Queue page)

**What it is**: the Review Queue's equivalent of Surface 4 — same calm,
affirmative treatment (Task 3.2.1c explicitly reuses Task 3.1.2b's pattern),
not a separate visual language invented for this page.

**Wireframe**:
```
┌──────────────────────────────────────────┐
│  ✓  All caught up                         │
│     Nothing needs your attention right now │
│                                            │
│  ▸ Informational (18, collapsed)          │
└────────────────────────────────────────────┘
```

**"Hidden by filter" wireframe (Product Triad Review round-4 blocker fix —
same variant as Surface 4, same reuse discipline)**: `needsDecisionItems`
(Task 3.2.1a) is partitioned from `items` — already the output of
`allFilteredItems`, i.e. *after* the priority-include/exclude, reason,
severity, program, category, tag, PR, diverged, and search filters have all
applied (`ReviewQueuePanel.tsx:446-538`). A user with, say,
`reasonFilter = {IDLE_TIMEOUT}` active while the only remaining
non-Low-priority item's reason is `APPROVAL_PENDING` sees a filtered
`needsDecisionItems` of zero while a real urgent item still exists. This is
distinct from the panel's pre-existing whole-list `hasActiveFilter` empty
state (`:1559-1573`, "No items match the current filter") — that state
originally only fired when `items.length === 0` overall — and, **per round
5's correction, now takes precedence over that legacy branch whenever it
applies, not only when `informationalItems` happens to stay non-empty.**
Round 4 fixed this state only for the case where `informationalItems`
remains non-empty after filtering, because that was the only case the
legacy `items.length === 0` check (which sits ahead of the needs-decision
tier's own empty-state logic in the panel's render order, at `:1556-1559`)
let through. A filter that empties *both* tiers at once — e.g.
`reasonFilter = {IDLE_TIMEOUT}` while the only remaining items in the whole
queue are `APPROVAL_PENDING` — makes `items.length === 0` true, so the
panel's outer branch never reaches the needs-decision tier's own check and
falls through to the legacy "No items match the current filter" message
instead, with no signal that `allNeedsDecisionCount` is nonzero,
reproducing the same false-calm failure class via this code path. **The fix
is a reorder, not a third variant**: the panel's outer branch must check
"is `allNeedsDecisionCount > 0` while `needsDecisionItems` is empty?"
*before* checking "is `items` empty?" (see Task 3.2.1c). Once reordered,
`informationalItems` (and thus `items`) can be non-empty *or* empty while
just the needs-decision tier is hidden — both are covered:
```
┌──────────────────────────────────────────┐
│  ⚠  2 items need a decision, but are      │
│     hidden by your filter                 │
│     [ Clear filter ]                      │
│                                            │
│  ▸ Informational (18, collapsed)          │
└────────────────────────────────────────────┘
```
"Clear filter" reuses `ReviewQueuePanel.tsx`'s existing `clearAllFilters`
(`:884-908`) verbatim — the identical function and button the panel's
whole-list empty state already calls at `:1566-1572` — not a second,
tier-scoped reset.

**Interaction flow / edge cases**: identical to Surface 4 — same copy, same
"don't hide the collapsed section," same golden-fixture-test requirement
(Task 3.2.1c), same "never during initial load" guard, and the same
skeleton-row loading-state wireframe (Surface 4's) reused verbatim rather
than a second, page-specific loading treatment. The emptiness check also
follows Surface 4's rule exactly (AC18): compute `allNeedsDecisionCount` —
`priority !== PriorityLow` over `allItems`, the pool *before* any of the
above display filters apply, not `items` — and compare it against
`needsDecisionItems.length` rather than deciding "caught up" from the
filtered tier alone. Unfiltered count `0` → calm "All caught up"; unfiltered
count `> 0` while `needsDecisionItems` is empty → "hidden by filter", using
the same two-count comparison Surface 4 uses, not a page-specific variant of
it. **This comparison runs, and can render, before the panel ever evaluates
its own pre-existing whole-list `items.length === 0` branch (round-5
correction)** — that legacy branch (and its `hadItems`/fresh-queue
sub-branches) is reachable only once `allNeedsDecisionCount` is itself
determined to be `0`. Without this ordering, a filter that empties
`informationalItems` at the same time it empties `needsDecisionItems` would
skip the tier-aware check entirely and fall through to the legacy generic
message, silently reintroducing the false-calm failure this surface exists
to prevent.

---

### Surface 9: Mid-review-race — Review-Queue transient banner

**What it is**: the review-queue-side half of Epic 2.3 (Task 2.3.2c) — when
a `WatchReviewQueue` `item_removed` event carries `auto_resolved_by_rule`
(new proto field, Task 2.3.1a), the panel shows a named banner in place of
the row for ~5 seconds before it actually disappears, instead of an abrupt
vanish.

**Flow diagram**:
```
Item sess-a1b2c3 open in "Needs a decision" tier
                │
   item_removed event arrives with
   auto_resolved_by_rule = "Auto-allow safe git status checks"
                │
                ▼
┌──────────────────────────────────────────────────────────┐
│ 🔴 URGENT  approval_pending          sess-a1b2c3          │
│ ┌────────────────────────────────────────────────────┐   │
│ │ ✓ Auto-resolved by rule: "Auto-allow safe git       │   │
│ │   status checks" — no action needed                 │   │
│ └────────────────────────────────────────────────────┘   │
│ [ Approve ] [ Deny ]   ← disabled, not hidden, for the    │
│                           ~5s the banner is visible       │
└──────────────────────────────────────────────────────────┘
                │  after ~5s
                ▼
         row removed from the list
         (nav badge count already decremented)
```

**Interaction flow**:
1. Action buttons are **disabled, not removed**, for the banner's ~5s
   window — per `research/ux.md`'s explicit guidance, this is what makes
   the state read as "resolved by the system, visibly" rather than "the
   controls broke."
2. A normal (human-driven) removal — e.g. the user's own Skip, or another
   device's Approve — still shows nothing extra: `RemovalInfo{Reason:
   "user_action"}` carries no rule name, so no banner renders, matching
   today's behavior exactly (Task 2.3.1b's acceptance criterion for the
   unchanged path).
3. If the item was in the collapsed Informational tier (Surface 7) when
   this fires, the banner still renders — expanding that section is not a
   precondition for seeing it disappear correctly, though the user would
   need to have it expanded to see the banner itself; this is an acceptable
   trade-off since a Low-priority item auto-resolving is inherently lower
   urgency to witness in the moment (the audit trail in Surface 3 is the
   backstop).
4. **Multiple simultaneous banners** (e.g. one rule edit reconciling several
   pending items in the same reload pass): each affected row renders its own
   named banner independently and on its own ~5s timer — they stack/list
   normally, in place of their respective rows, exactly like any other
   multi-row list state. No special-case handling, no cap, and no collapsing
   multiple banners into one summary line is needed; each row's disabled
   buttons and named-rule text already exist per-row, so N simultaneous
   banners is the same code path running N times, not a new state to design
   for.
5. **Focus management**: if a row removed by the ~5s auto-removal timer was
   keyboard-focused, focus moves to the next remaining row in the list, or
   to the list's container/heading if it was the last row — same pattern as
   Surface 1's equivalent note, applied here to the timer-driven removal
   instead of an explicit user action.

**Error / edge cases**:
- **Banner fires while the user is mid-click on Approve/Deny**: the RPC
  returns `CodeFailedPrecondition` (same as Surface 5); the panel shows the
  same "already auto-resolved" message inline rather than a generic RPC
  error, then removes the row on the same ~5s timer.
  **Note**: `WatchReviewQueue`'s wire protocol is intentionally frozen to
  exactly this one new field per requirements.md's Out of Scope — this
  banner is not a general-purpose toast system and should not be extended
  to other event types without a separate scoping decision.

---

## Part C — Sessions list

### Surface 10: Idle status chip

**What it is**: `SubStatusChip`'s existing `SubStatus.IDLE` case ("● Idle",
already fully built with its own `aria-label="Session is idle"`) —
`SessionRow.tsx:352` currently filters it out by name; Task 3.2.2b removes
that one filter term. This is a one-line reversal of an existing
suppression, not a new component.

**Wireframe** (Sessions list row, desktop and mobile share the same chip —
only the surrounding row layout differs, which this project does not
touch):
```
Desktop row:
┌────────────────────────────────────────────────────────────────┐
│ ● agent-shell   feature/notif-revamp   ● Idle    2h ago         │
└────────────────────────────────────────────────────────────────┘

Mobile card:
┌──────────────────────────┐
│ agent-shell               │
│ feature/notif-revamp      │
│ ● Idle          2h ago    │
└──────────────────────────┘
```

**Interaction flow**: purely informational — no action attached to the chip
itself (consistent with every other `SubStatusChip` case). Its role is to
give the "ready for next task" signal a home now that idle items no longer
occupy Review Queue slots (Surface 6/7's scope boundary — this chip is the
other half of that same product decision, per ADR-002).

**Error / edge cases**: none — this is a pure read of already-computed
`subStatus`, no new data dependency, no new failure mode.

---

## UX Acceptance Criteria

Grouped by surface; every criterion is checkable by a human clicking through
the built feature, not by reading code.

### Needs a decision (Notifications + Review Queue, Surfaces 1 & 6)

1. On page load, a user can identify whether anything needs their attention
   in **0 clicks** — the section is visible above the fold, expanded, with
   no toggle required.
2. A user can approve, deny, or open the session for any item in the Needs
   a Decision section in **≤ 2 taps/clicks** from page load (1 to locate +
   1 to act, since the section is already expanded).
3. A failed Approve/Deny shows the specific message "Couldn't record your
   decision — try again" and leaves the same Approve/Deny buttons active —
   no dead end, no need to reload the page to retry.
4. The unread/needs-decision count in the page header and the `BottomNav`
   bell badge never disagree by more than one poll cycle (~2s) of each
   other for the same underlying state.
5. The header's unread count caps at "99+" for any value over 99, and shows
   the literal number for 99 and below (Story 3.1.4).
6. **Bulk "Mark activity read" never marks an item in the Needs a Decision
   section as read.** Triggering it with both sections populated leaves
   `NeedsDecisionSection`'s item count and contents unchanged — an item
   leaves that section only by being acted on (Approve/Deny/Open
   session) or by its underlying state resolving elsewhere, never
   via this bulk action (Product Triad Review blocker fix).
7. **No per-item ✕/dismiss control exists on a Needs a Decision item.** A
   user viewing the section has no way to remove an item from it other than
   Approve/Deny/Open session or the item's underlying state resolving
   elsewhere — there is no click that silently clears a still-pending
   `approval_needed`/`question`/`error`/`task_failed`/`warning` item from
   local history while leaving the real decision unresolved (Product Triad
   Review round-2 blocker fix — this applies the same principle AC6 states
   for bulk actions to the single-item case).
8. The bulk button is labeled "Mark activity read" (not "Mark all read")
   and is enabled only when Recent Activity or Auto-handled has at least
   one unread item — independent of the Needs a Decision section's own
   unread count, so it is never shown active with nothing it can affect.
9. "Skip all (N)" on the Review Queue (Surface 6) excludes every
   approval-pending item from the bulk action (they require an explicit
   Approve/Deny) and is gated by a confirm dialog before it runs; for every
   other reason it can affect, a skip that doesn't correspond to a real
   state change is not durable — the item reappears on the next poll cycle
   if its underlying condition still holds, so this action cannot silently
   and permanently clear a still-pending decision.

### Recent activity / Informational tiers (Surfaces 2 & 7)

10. Both sections render **collapsed by default** on every fresh page load —
    never auto-expanded, never remembered as "was open" from a prior visit.
11. A user can expand either section in **1 click/tap** via a real,
    keyboard-focusable button (Tab reaches it; Enter/Space toggles it) —
    not a `div` with a click handler.
12. Expanding or collapsing either section never changes the Needs a
    Decision count, the page-header count, or the `BottomNav` badge count.

### Auto-handled section (Surface 3)

13. A live auto-approval and a rule-reconciled resolution are visually
    distinguishable at a glance: "Auto-approved" vs. "Auto-resolved by
    rule: `<name>`" — never the same label for both.
14. A rule-reconciled item appears in exactly one place (Auto-handled) —
    never simultaneously in the Needs a Decision or Recent Activity
    sections.

### Empty "needs a decision" states (Surfaces 4 & 8)

15. The empty state reads "All caught up" / "Nothing needs your attention
    right now" with a checkmark icon — never "No items found," never an
    empty-box/ghost illustration.
16. The Recent Activity / Informational section remains visible (collapsed)
    directly below the empty state — it never disappears or expands to
    fill the space.
17. During initial data load (before the first successful poll response),
    the page shows the skeleton-row loading state (Surface 4's loading-state
    wireframe, reused verbatim on Surface 8), never the "All caught up"
    empty state — a slow load must not read as a false "you're done," and
    the loading treatment must be visually distinct from the empty state at
    a glance, not only by its text.
18. **The empty state distinguishes "genuinely zero actionable items" from
    "items exist but your filter is hiding them" (Product Triad Review
    round-4 blocker fix).** The check is never based on the filtered/rendered
    Needs a Decision list alone: each page computes a true unfiltered
    actionable count once — Notifications: unread +
    `isActionableNotification` over the full `notificationHistory`, not
    `filteredNotifications`; Review Queue: `priority !== PriorityLow` over
    `allItems`, not the post-filter `items` — and only renders the calm "All
    caught up" copy when that unfiltered count is itself `0`. *Given* an
    active filter (type/search/hide-backlog on Notifications;
    priority-include/exclude/reason/severity/program/category/tag/search on
    Review Queue) that excludes every remaining actionable/needs-decision
    item while at least one such item still exists, *Then* the section shows
    "N item(s) need a decision but are hidden by your filter" with a working
    "Clear filter" control — reusing each page's existing filter-reset
    mechanism (`NotificationsPage`'s new `clearFilters` helper mirroring
    `ReviewQueuePanel.tsx`'s existing `clearAllFilters`, not a second
    independent implementation) — and never the calm "All caught up" text.
    **This holds regardless of whether the other tier (Recent Activity /
    Informational) is also empty after the same filter is applied (Product
    Triad Review round-5 blocker fix).** Concretely, this check must run
    — and be able to render its "hidden by filter" message — *before* each
    page's own pre-existing whole-list empty-state branch
    (`NotificationsPage.tsx:247`'s `filteredNotifications.length === 0`;
    `ReviewQueuePanel.tsx:1559`'s `items.length === 0`), not after it. Prior
    to this correction, a filter that emptied *both* tiers at once fell
    through to that legacy branch's generic "no matching items" message
    instead, bypassing this criterion entirely — the fix is reordering the
    two checks, not adding a third empty-state variant. *Given* a filter
    that excludes every item on the page (both the needs-decision tier and
    the other tier) while at least one needs-decision item still exists
    unfiltered, *Then* the section still shows "N item(s) need a decision
    but are hidden by your filter," never the page's generic
    "No matching notifications" / "No items match the current filter" text.

### Mid-review-race (Surfaces 5 & 9)

19. When rule-reconciliation resolves an item the user has open, the item's
    action buttons become disabled (visibly present, greyed out) — they are
    never silently removed with no explanation.
20. The banner/badge names the specific rule ("Auto-resolved by rule:
    `<name>`") — never a generic "This item was resolved" with no
    attribution.
21. If the user's own Approve/Deny click loses the race, the resulting
    message says "already auto-resolved by rule `<name>` ... no action
    needed" — never a generic "Expired" or a raw RPC error string.
22. No "Approve anyway" / override action renders on a reconciliation-race
    message (there is nothing left to approve against) — only "Deny" (or no
    action) is offered, so the user isn't invited into a dead click.
23. When reconciliation resolves multiple pending items in the same pass,
    each affected row shows its own named banner independently (they
    stack/list normally) — there is no cap on simultaneous banners and no
    collapsing of several into one summary line.

### Idle chip (Surface 10)

24. An idle session's "● Idle" chip is visible on the Sessions list within
    one poll cycle of the session actually going idle.
25. An idle session never simultaneously appears as a Review Queue row —
    the two surfaces are mutually exclusive for the same session at the
    same time.

### Cross-cutting / accessibility (per `research/ux.md` §3, applies to every
surface above)

26. **Keyboard navigation**: every actionable control (Approve, Deny, Skip,
    Open session, Create rule, collapsible section headers) is reachable via
    Tab in a logical order and activatable via Enter/Space — no
    mouse/touch-only interaction exists anywhere in these two pages.
27. **Screen-reader labels**: every priority/status badge carries a
    descriptive `aria-label` (e.g. "URGENT priority: approval pending"), not
    an icon or color alone — matching `ReviewQueueBadge.tsx`'s existing,
    already-correct pattern; any newly introduced priority-tier UI must
    match this standard, not regress to a color-only chip (the exact
    regression the audited screenshot's "plain white circle" flagged).
28. **Color contrast**: all priority/status badge text meets WCAG AA — 4.5:1
    for body text, 3:1 for large text/icons — against both light and dark
    backgrounds, verified for `PriorityLow`/`Medium`/`High`/`Urgent` tokens
    specifically (the CI-enforced Axe Core gate on `web-app/src/` PRs is the
    backstop; this criterion is what a human should also spot-check before
    merge).
29. **Non-color priority encoding**: every priority indicator combines an
    icon/emoji **and** a text abbreviation **and** an `aria-label` — color
    is never the sole channel carrying priority information anywhere on
    either page.
30. **`aria-live` discipline**: the Needs a Decision section's live region
    is `polite` (never `assertive`), present in the DOM from first paint
    (not conditionally mounted only once an item first appears), and
    announces only a short count update ("3 items need a decision," singular
    "1 item needs a decision" when the count is exactly 1) — never
    the full re-rendered list.
31. **Collapsible semantics**: every collapsed section uses a real `<button
    aria-expanded>` trigger (via the existing `Collapsible.tsx`/Radix
    Accordion primitive) whose accessible name includes the section label
    and count (e.g. "Recent activity, 24 items, collapsed") — never a bare
    "24" with no context for a screen-reader user.
32. **No dead ends**: every error state listed above (failed
    approve/deny/skip, mid-review-race message) offers a specific next
    action (retry, deny, or "no action needed") — none of them terminates
    in a state with no visible path forward.
33. **Mobile touch targets**: every actionable button in the mobile
    wireframes (Approve, Deny, Skip, Create rule, Open session) has a
    minimum 44×44pt hit area and does not require a hover state to discover
    (no hover-only affordances anywhere on either page's mobile layout).
34. **Focus management on list removal**: whenever an item under keyboard
    focus is removed from a list — resolved via Approve/Deny/Open
    session (Surface 1), or auto-removed by the ~5s mid-review-race
    banner timer (Surface 9) — focus moves to the next remaining item in
    that list, or to the list's own container/heading if it was the last
    item; focus is never dropped to `<body>` with no indication of where
    keyboard navigation should continue.

### Header bell dropdown scoping (Surface 11) & background poll reliability (Surfaces 1 & 6)

*AC38 is grouped here for numbering stability (appended when authored,
rather than inserted into the "Needs a decision" section above and
renumbering every criterion after it) — it is not otherwise about the
dropdown; it applies to the Needs a Decision section on both the
Notifications page (Surface 1) and the Review Queue (Surface 6), not to
`NotificationPanel`.*

35. **The dropdown's bulk button ("Mark activity read") never marks an
    unread `approval_needed`/`question`/`error`/`task_failed`/`warning` item
    as read** — same rule as AC6, applied to `NotificationPanel` via the same
    shared helper, not a second independent implementation. The button is
    enabled only when it has at least one other unread item to affect.
36. **No per-item ✕/dismiss control renders on an unread actionable item
    anywhere in the dropdown's flat list** — same rule as AC7, applied
    per-item (not per-section, since this surface has no tiered sections).
    The control reappears for that same item the moment it becomes read or
    its underlying decision resolves.
37. **"Clear history" (renamed from "Clear all", on both `NotificationPanel`
    and `NotificationsPage`) never deletes a notification record that is
    both unread and actionable**, and always requires an explicit
    confirmation dialog before running — the exclusion and the confirmation
    both apply; the exclusion alone (unlike AC9's "Skip all," which is
    non-destructive) is not treated as a sufficient guard on its own for an
    irreversible delete.
38. **A background poll/fetch failure for the Needs a Decision section shows
    the last-known list/count with a visible staleness indicator and a retry
    affordance** — never a silent, possibly-stale "All caught up" with no
    indication the data may be out of date, and never (Review Queue
    specifically) a full-panel error takeover that discards already-loaded,
    still-useful stale data when some exists. Applies to both the
    Notifications page's `NeedsDecisionSection` (Surface 1) and the Review
    Queue's needs-decision tier (Surface 6) via the same "Last updated
    `<Xm ago>` · Retry" pattern (Tasks 3.1.2h / 3.2.1d) — one mechanism, not
    two independently designed ones.
