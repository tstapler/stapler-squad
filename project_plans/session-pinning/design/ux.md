# UX Design: Session Pinning

Source: `project_plans/session-pinning/requirements.md`, `research/ux.md`,
`implementation/plan.md` (Epics 2.1–2.4). This document specifies the exact
user-facing surfaces the plan builds — no new surfaces invented, no
plan decisions second-guessed (e.g. no persistent card-face pin icon; the
context-menu toggle is the only pin affordance for v1, per plan.md's
"Card-face persistent pin icon" row).

## Surfaces designed (5)

1. Session card/row — pin affordance entry point (the "···" overflow menu trigger)
2. Context menu — Pin/Unpin `menuitemcheckbox` item
3. "Pinned" section header/region in the session list
4. Zero-pinned-sessions state
5. Pin-toggle-failed error state

---

## Surface 1: Session card/row — pin affordance entry point

Per plan.md's resolved decision, there is **no standalone pin icon on the
card face for v1** — the only entry point is the existing "···" (More
session actions) overflow trigger, already present on every `SessionCard`/
`SessionRow`. Pinning does not add a new button to the card; it adds an item
inside a menu that already exists.

### Wireframe — card mode (unpinned session, outside Pinned section)

```
┌──────────────────────────────────────────────────────────┐
│ ● my-feature-branch                              ⋯ [menu] │
│   Working · claude-code · 2m ago                          │
│   feat: add rate limit toggle                             │
└──────────────────────────────────────────────────────────┘
                                                     ▲
                                        existing "···" trigger —
                                        aria-label="More session actions"
                                        (unchanged by this feature)
```

### Wireframe — card mode (session inside the Pinned section)

```
┌──────────────────────────────────────────────────────────┐
│ ● my-feature-branch                              ⋯ [menu] │
│   Working · claude-code · 2m ago                          │
│   feat: add rate limit toggle                             │
└──────────────────────────────────────────────────────────┘
     ▲
     Card itself is visually IDENTICAL to an unpinned card.
     The only signal that this session is pinned is its position
     inside the "Pinned" region (Surface 3) — no badge, no filled
     pin glyph on the face of the card.
```

### Interaction flow

1. User clicks/taps the "···" trigger on any card or row (pinned or not).
2. Existing menu opens (`role="menu"`), positioned per existing overflow-menu logic — unchanged.
3. User sees the Pin/Unpin item among the other menu entries (Surface 2).

### Error/edge cases

- None specific to this surface — it is pre-existing UI, untouched by this feature.

### UX note (non-blocking, does not contradict the plan)

Because there is no on-card indicator, a user who has scrolled past the
Pinned section and is looking at a card lower in the list has **no visual
cue that a given card is pinned** except by its absence from the section
above. This is an accepted trade-off recorded in plan.md ("the Pinned
section's position already communicates pinned state ... within that
section every card is already known-pinned, so a redundant badge adds no
information"). Flagging for awareness, not proposing a change: if user
feedback post-ship indicates confusion (e.g. "why isn't a session showing
in its normal group?"), the cheapest fix is reusing the already-imported
`Pin`/`PinOff` icons as a small `aria-hidden` glyph next to the title in
`SessionCard`/`SessionRow`, gated on `session.pinned` — no new prop needed,
`onTogglePinned` presence isn't required for a read-only badge. Not part of
this design's acceptance criteria; recorded for the backlog only.

---

## Surface 2: Context menu — Pin/Unpin `menuitemcheckbox` item

### Wireframe — menu open, unpinned session

```
┌─────────────────────────────────┐
│  ▶ Resume                       │
│  ⏸ Pause                        │
│  🗑 Delete                       │
│  ──────────────────────────     │
│  📋 Copy branch name             │
│  ⎇ Switch Workspace              │
│  ──────────────────────────     │
│  ▶ Enable auto-resume           │
│  🤖 Run autonomously             │
│  📌 Pin                    ← NEW │
│  ──────────────────────────     │
└─────────────────────────────────┘
```

### Wireframe — menu open, pinned session

```
┌─────────────────────────────────┐
│  ▶ Resume                       │
│  ⏸ Pause                        │
│  🗑 Delete                       │
│  ──────────────────────────     │
│  📋 Copy branch name             │
│  ⎇ Switch Workspace              │
│  ──────────────────────────     │
│  ▶ Enable auto-resume           │
│  🤖 Run autonomously             │
│  📌 Unpin        ← NEW, checked  │
│  ──────────────────────────     │
└─────────────────────────────────┘
```

(📌 above stands in for the lucide-react `Pin`/`PinOff` glyphs the
implementation actually renders — plan.md Task 2.3.1a. Per ux.md §2, this is
intentionally the **one** menu item using an SVG icon component instead of
an emoji glyph like its neighbors; acceptable because it matches the
autonomous-mode toggle immediately above it, which is the item's structural
sibling, not the delete/copy items which are plain `menuitem`s.)

### Interaction flow

| Step | User action | System response |
|---|---|---|
| 1 | Opens "···" menu | Menu renders; Pin/Unpin item appears in the "Mode toggles" group, directly below "Run autonomously" |
| 2 | Reads item state | Unpinned session → "📌 Pin", `aria-checked="false"`. Pinned session → "📌 Unpin" (PinOff glyph), `aria-checked="true"` |
| 3 | Clicks "Pin" | Menu closes immediately. Session **optimistically** moves into the Pinned section on the next render (no spinner, no "pinning..." intermediate state — matches ux.md §4's "every reference product treats unpin as instant and local") |
| 4 | Clicks "Unpin" (on an already-pinned session) | Menu closes immediately. Session **optimistically** leaves the Pinned section and reappears in its normal group |
| 5 | RPC resolves successfully in the background | No visible change — the optimistic state already matches the server's now-confirmed state |
| 6 | RPC fails | See Surface 5 (error state) — item reverts, toast appears |

### Keyboard flow

- Menu already supports arrow-key roving tabindex + Enter/Space activation for its existing `menuitem`/`menuitemcheckbox` entries (autonomous-mode toggle is the structural precedent) — the Pin/Unpin item participates identically, no new keyboard code needed.
- `Escape` closes the menu without triggering the toggle (existing menu behavior, unchanged).
- Screen reader announces: focus moves to the item, its role (`menu item check box`), its checked state, and its label — e.g. "Pin my-feature-branch, menu item check box, not checked" → activates → "Unpin my-feature-branch, menu item check box, checked" (next time the menu opens).

### Error/edge cases at this surface

- **Session gets archived while the menu is open** (e.g. by another action mid-session): out of scope for this design — no evidence any other menu item guards against this race today; Pin/Unpin follows the same (lack of) guard as its siblings.
- **Menu item hidden entirely**: only when `onTogglePinned` prop is not supplied by a parent (defensive `{onTogglePinned && (...)}` guard in the implementation) — this is a wiring completeness concern, not a designed UX state; there is no product scenario where a live, non-archived session should lack this prop.

### UX acceptance criteria — Surface 2

- **AC-2.1**: User can pin a session in **1 click** (open menu is a separate, pre-existing step not unique to this feature) — i.e., exactly 1 click on "Pin" once the menu is open, no confirmation dialog.
- **AC-2.2**: User can unpin a session in **1 click** on "Unpin," no confirmation dialog — matches ux.md §1's "no confirmation on unpin" convention across every comparable product surveyed.
- **AC-2.3**: The menu item exposes `role="menuitemcheckbox"`, `aria-checked` matching `session.pinned`, and a dynamic `aria-label` in the form `"Pin {title}"` / `"Unpin {title}"` — confirmed against plan.md Task 2.3.1a's exact markup, which follows the existing autonomous-mode toggle's pattern (`role="menuitemcheckbox"` + `aria-checked` + dynamic `aria-label` naming the action, not just the state).
- **AC-2.4**: The decorative `Pin`/`PinOff` icon carries `aria-hidden="true"` so screen readers rely solely on the text label + `aria-checked`, never announce a redundant/conflicting icon description.
- **AC-2.5**: `data-testid="session-pin-toggle"` is present on the button element, satisfying the e2e locator convention (`.claude/rules/e2e-test-conventions.md`) — `page.getByTestId("session-pin-toggle")` or `page.getByRole("menuitemcheckbox", { name: /pin/i })` must both resolve to this element.
- **AC-2.6**: Toggling state completes with **zero perceived latency** on the happy path (optimistic update — icon/label and section membership update on the same frame as the click, before the RPC round-trip completes).
- **AC-2.7**: Text contrast for the menu item label meets ≥ 4.5:1 against the menu background — inherited unchanged from the existing `overflowMenuItem` style shared by every other item in this menu (no new color introduced for this item).

---

## Surface 3: "Pinned" section header/region in the session list

### Wireframe — session list with 2 pinned sessions, grouping strategy = Status

```
┌ Session List ──────────────────────────────────────────────┐
│                                                              │
│ ┃ 📌 Pinned                                                 │  ← h2, pinnedSectionTitle
│ ┌──────────────────────────────────────────────────────┐   │     4px left accent border
│ │ ● release-cut                              ⋯          │   │     (vars.color.primary),
│ │   Working · claude-code · 5m ago                      │   │     surfaceSubtle background
│ └──────────────────────────────────────────────────────┘   │
│ ┌──────────────────────────────────────────────────────┐   │
│ │ ⏸ hotfix-login-bug                         ⋯          │   │
│ │   Paused · aider · 1h ago                             │   │
│ └──────────────────────────────────────────────────────┘   │
│                                                              │
│ ▼ Working (3)                                               │  ← existing status group,
│ ┌──────────────────────────────────────────────────────┐   │     unaffected in shape;
│ │ ● feature-x                                ⋯          │   │     release-cut is NOT
│ └──────────────────────────────────────────────────────┘   │     duplicated here
│ ┌──────────────────────────────────────────────────────┐   │
│ │ ● feature-y                                ⋯          │   │
│ └──────────────────────────────────────────────────────┘   │
│                                                              │
│ ▼ Paused (2)                                                │  ← hotfix-login-bug is NOT
│ ┌──────────────────────────────────────────────────────┐   │     duplicated here either
│ │ ⏸ other-paused-session                     ⋯          │   │
│ └──────────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────────────┘
```

`role="region" aria-label="Pinned sessions"` wraps the whole block, mirroring
the existing `role="region" aria-label="No results"` pattern already used
for the filtered-empty state (`SessionList.tsx:1203`).

### Interaction flow

1. User pins a session anywhere in the list (via Surface 2).
2. On the same render, the session is removed from its current status/category/branch/etc. group and re-inserted into the Pinned region, at the position determined by the list's **current active sort** (no separate pin-specific ordering — plan.md's "Pinned-section ordering" row).
3. Every card/row inside the Pinned region is fully interactive — same `onSessionClick`, `onDeleteSession`, `onPauseSession`, etc. as outside the section (plan.md Task 2.4.3a's inline note: "Wire every other handler prop ... identically").
4. If the session's normal group becomes empty as a result (all its members pinned), that group's header simply doesn't render — existing empty-group behavior, unaffected by this feature.
5. Unpinning reverses the move: session leaves the Pinned region, re-enters its normal group at the next render.

### Section ordering semantics (what a user should expect)

- Sessions in the Pinned region are sorted using the **same `sortField`/`sortDir`** currently applied to the rest of the list (e.g. if the user has sorted by "Last activity," pinned sessions are also ordered by last activity **within** the Pinned region — not by "most recently pinned").
- This is a deliberate, requirements-scoped limitation (FR/out-of-scope: "pin ordering/reordering... not drag-to-reorder"). A user who pins 5 sessions should not expect to control their relative order within the section beyond changing the list's global sort.

### Error/edge cases

- **A pinned session is archived**: auto-unpinned server-side (plan.md's auto-unpin invariant) — it disappears from the Pinned region on the same render the archive action completes, with no separate "this was also unpinned" messaging (matches ux.md §4's "no special handling beyond what already happens" for deletion, extended here to archive since the plan resolved archive-implies-unpin).
- **A pinned session is hidden**: never appears in the Pinned region in the first place — `ListSessions` excludes hidden sessions from the payload entirely (hidden-wins invariant), so there's nothing to remove; this is invisible to the user by construction, not a state transition they'd witness.
- **A pinned session is deleted**: disappears from the Pinned region exactly as it disappears from everywhere else — no special messaging (matches ux.md §4).
- **Very long session title inside the Pinned region**: no new truncation behavior specified in the plan — cards/rows reuse existing `SessionCard`/`SessionRow` title-truncation CSS unchanged; the Pinned region imposes no different width constraint than the card's normal container.
- **Many pinned sessions (no cap, per plan.md's "Pin cap: No cap" row)**: the Pinned region can grow arbitrarily large, potentially pushing the first normal group below the fold. This is an accepted trade-off (matches `hidden`/`auto_yes`/`is_expanded`'s uncapped precedent) — not treated as an error state, but worth naming: a user who pins most/all of their sessions gets a Pinned section that behaves like — visually subsumes — the whole list. No collapse/scroll-limit affordance is specified for this region in the plan; the region is not virtualized separately from the rest of the list (plan.md's dependency diagram doesn't call out a separate virtualizer for it), so a very large pinned set inherits whatever performance characteristics the full list has without virtualization for this block specifically. Flagging as a v1 known limitation, not a defect to fix in this design pass.

### UX acceptance criteria — Surface 3

- **AC-3.1**: Pinned sessions are visually distinguishable as a group via a persistent section header ("Pinned") positioned above every other group, on every render where at least one session is pinned.
- **AC-3.2**: No session ever appears twice in the visible list (Pinned region + its normal group simultaneously) — testable by asserting `screen.getAllByText(title)).toHaveLength(1)` for any pinned session's title (mirrors plan.md's own Jest assertion in Task 3.3.2a).
- **AC-3.3**: The region is reachable by screen-reader landmark navigation via `role="region" aria-label="Pinned sessions"`, distinguishing it from the unlabeled/differently-labeled groups below.
- **AC-3.4**: The section header/accent uses only design tokens (`vars.color.primary`, `vars.color.textPrimary`, `vars.color.surfaceSubtle`) — no hardcoded hex values — per `.claude/rules/css-architecture.md`; confirmed against plan.md Task 2.4.2a's actual CSS, which uses only `vars.*` references.
- **AC-3.5**: Section header text (`vars.color.textPrimary` on `vars.color.surfaceSubtle`) meets ≥ 4.5:1 contrast — verified against the light theme's token values (`#0a0a0a` on `#f9fafb`, `web-app/src/styles/theme.css.ts:66,79`), which is near-black on near-white and clears WCAG AA by a wide margin; each additional theme (dark, terminal, synthwave, sepia, etc.) must independently satisfy this — spot-check any new/edited theme file when this ships, since `theme.css.ts` defines `textPrimary`/`surfaceSubtle` per-theme, not globally.
- **AC-3.6**: No dead end — a user in the Pinned region has full access to every other action (resume, pause, delete, etc.) identical to the normal list; nothing in this region is a trap requiring the user to unpin before doing anything else.

---

## Surface 4: Zero-pinned-sessions state

### Wireframe

```
┌ Session List ──────────────────────────────────────────────┐
│                                                              │
│  (no "Pinned" header, no region, no placeholder card —      │
│   the list starts directly at the first normal group)       │
│                                                              │
│ ▼ Working (3)                                                │
│ ┌──────────────────────────────────────────────────────┐   │
│ │ ● feature-x                                ⋯          │   │
│ └──────────────────────────────────────────────────────┘   │
│ ...                                                          │
└──────────────────────────────────────────────────────────────┘
```

### Design decision (explicit — matches plan.md and ux.md §4)

**No empty-state placeholder.** The Pinned section is entirely absent from
the DOM — no header, no "Pin a session to see it here" hint, no collapsed
affordance — when `pinnedSessions.length === 0`. This is a deliberate
divergence from this same list's *filtered*-empty state (Surface would be
"No sessions found" + "Clear filters," `SessionList.tsx:1203`), because the
two are semantically different empty states:

| | Filtered-empty (existing) | Zero-pinned (this feature) |
|---|---|---|
| Trigger | User's active filter matched nothing | User simply hasn't used an optional feature |
| Is it the *entire visible list*? | Yes — nothing else renders | No — the rest of the list renders normally below |
| Should it explain/offer an escape? | Yes — "Clear filters" gives an exit from a state the user may not have intended | No exit needed — there's nothing to escape from |

Showing a persistent empty "Pinned" section for every user who has never
pinned anything would add chrome to the default (majority) experience for a
minority-use feature — the opposite of Nielsen's "aesthetic and minimalist
design" heuristic, and inconsistent with every comparable product surveyed
in ux.md §1 (Slack's pinned panel, VS Code's pinned-tab strip, browser
pinned tabs — none show empty chrome pre-use).

### Interaction flow into/out of this state

1. **App loads, no sessions ever pinned** → Surface 4 (nothing rendered) is the default first-run experience; no onboarding hint is specified in the plan.
2. **User pins their first session** → transitions directly to Surface 3 (section appears with 1 card) on the same render — no intermediate "section is appearing" animation (research explicitly recommends skipping animation for v1, ux.md §1).
3. **User unpins their last remaining pinned session** → transitions back to Surface 4 (section disappears) on the same render.

### UX acceptance criteria — Surface 4

- **AC-4.1**: When `pinnedSessions.length === 0`, no element with `aria-label="Pinned sessions"` exists in the DOM — directly testable (matches plan.md Task 3.3.2a's own assertion: `expect(screen.queryByRole("region", { name: /pinned sessions/i })).not.toBeInTheDocument()`).
- **AC-4.2**: The absence of the Pinned section causes no layout shift/flash for users who never interact with pinning — the normal grouped list renders in the exact position it would occupy with or without this feature present.
- **AC-4.3**: No dead end — trivially satisfied, since there is no state to escape from; noted for completeness of the acceptance-criteria set.

---

## Surface 5: Pin-toggle-failed error state

### Wireframe — optimistic update, then rollback + toast

```
Step 1 (t=0ms, user clicks "Pin"):          Step 2 (t=0ms, same frame):
┌──────────────────────────┐                ┌ Session List ─────────────┐
│  ...                     │                │ ┃ 📌 Pinned                │
│  📌 Pin        ← clicked │   ──────►      │ ┌────────────────────┐    │
│  ──────────────           │                │ │ ● release-cut   ⋯  │    │  optimistic:
└──────────────────────────┘                │ └────────────────────┘    │  card already moved
                                             │ ▼ Working (2)              │
                                             └────────────────────────────┘

Step 3 (RPC fails, e.g. 200-400ms later):
┌ Session List ──────────────┐              ┌──────────────────────────────────┐
│  (no "Pinned" section —    │   + toast:   │ ⚠ Couldn't pin session — try again│
│   card reverted to its     │  ─────────►  └──────────────────────────────────┘
│   normal group)            │                 (NotificationToast.tsx, existing
│ ▼ Working (3)               │                 component/pattern per ux.md §4)
│ ┌────────────────────┐    │
│ │ ● release-cut   ⋯  │    │  ← reverted to
│ └────────────────────┘    │     original group/position
└──────────────────────────────┘
```

### Interaction flow

| Step | System state | User-visible effect |
|---|---|---|
| 1 | User clicks Pin/Unpin | Optimistic dispatch: `session.pinned` flips locally, section membership updates immediately (Surface 3 or 4 transition happens *before* the RPC resolves) |
| 2 | RPC in flight | No loading indicator — matches the "no spinner anywhere in the reference set" finding (ux.md §4) |
| 3a | RPC succeeds | No further visible change — optimistic state was already correct; nothing to reconcile |
| 3b | RPC fails (network error, `CodeFailedPrecondition` from the archived-guard, `CodeNotFound`, `CodeInternal`, etc.) | Local state **rolls back** to the pre-click value (plan.md Task 2.1.1a: `dispatch(upsertSession(previous))`); session moves back out of/into the Pinned section on the very next render. A toast/notification appears via the existing `NotificationToast.tsx` pattern with the RPC's error message (plan.md: `dispatch(setError(err.message ?? "Failed to pin session"))`) |
| 4 | User dismisses toast or it auto-dismisses per existing toast behavior | List remains in its correct (reverted) state; user can retry by opening the menu and clicking Pin/Unpin again — no different from any first attempt |

### Specific failure scenarios and what the user sees

| Failure | Server response | Toast copy (message text is whatever `err.message` resolves to — plan.md gives the fallback string, not a full error-copy spec) | Recovery path |
|---|---|---|---|
| Network drop / server unreachable | Fetch throws | Fallback: "Failed to pin session" / "Failed to unpin session" | Retry via the same menu item — no different flow |
| Session already archived (raced with another archive action) | `connect.CodeFailedPrecondition` | Server-provided error message surfaces via `err.message` (e.g. "cannot pin an archived session: …") | Session was already correctly excluded/removed from the pinnable set on the server; UI rollback restores the accurate (unpinned) state — no additional user action needed beyond acknowledging the toast |
| Session no longer exists | `connect.CodeNotFound` | Server-provided error message | Same as above — rollback reflects reality |
| Internal/storage error | `connect.CodeInternal` | Server-provided error message | Retry; if persistent, this is an operational issue outside this feature's UX scope |

### UX acceptance criteria — Surface 5

- **AC-5.1**: On RPC failure, the toggle's visible state (icon, label, section membership) reverts to match the pre-click server-confirmed state within one render — no state where the UI claims a pin succeeded while the server has actually rejected it.
- **AC-5.2**: Error is surfaced via the existing toast/notification component (`NotificationToast.tsx`), not a blocking modal or dialog — consistent with the low-stakes, instantly-reversible nature of this action (no product reason to interrupt the user's flow for a failed pin toggle).
- **AC-5.3**: Error toast text names the failed action specifically ("Failed to pin session" / "Failed to unpin session," or the server's own message when available) — never a generic "Something went wrong" with no indication of which action failed, so the user can correctly decide whether/how to retry.
- **AC-5.4**: No dead end — the failure path leaves the menu item fully interactive and re-clickable immediately; there is no error state that requires a page reload or leaves the toggle permanently disabled to recover from a single failed attempt.
- **AC-5.5**: The rollback is silent with respect to *other* session state — only the `pinned` field (and its downstream section placement) is touched by the rollback; no other fields on the optimistically-updated session object are altered as a side effect (verifiable against plan.md's `dispatch(upsertSession(previous))`, which restores the entire previous object, not just `pinned`, so this is inherently satisfied by construction).
- **AC-5.6**: User can complete a successful retry in the same number of steps as the original attempt (1 click on the menu item, no extra confirmation step introduced by having failed once) — failure does not make the action "stickier" or add friction to trying again.

---

## Cross-cutting acceptance criteria (apply to all 5 surfaces)

- **Keyboard-navigable**: every interactive element introduced (the Pin/Unpin menu item) is reachable and operable via keyboard alone, using the menu's existing roving-tabindex + Enter/Space activation — no mouse-only interaction exists anywhere in this feature.
- **Screen-reader labels present**: every stateful element has a non-empty, dynamic `aria-label` or accessible name that changes with state (Pin ↔ Unpin, `aria-checked` true/false) — verified against plan.md's exact JSX in Task 2.3.1a.
- **Color contrast ≥ 4.5:1**: all net-new text (section header) uses existing design tokens already vetted for contrast per-theme; no new color values are introduced by this feature (plan.md Task 2.4.2a uses only `vars.*` references, no hex).
- **`role="menuitemcheckbox"` + `aria-checked` + dynamic `aria-label` pattern**: **confirmed** as the toggle design this feature follows — this is the exact pattern from the existing autonomous-mode toggle in `SessionActionsOverflow.tsx:701-720` (cited in research), and plan.md's Task 2.3.1a implements it verbatim for the pin toggle. No deviation.
- **No dead ends**: every surface (including the two states that render *nothing*, Surfaces 1's "no badge" and 4's "no section") leaves the user able to proceed with every other list action; the one surface with a true failure mode (Surface 5) has an explicit, always-available recovery path (retry via the same always-interactive menu item).
- **No confirmation dialog anywhere in this feature**: matches the universal convention found across every comparable product in ux.md §1 — pinning and unpinning are both single-click, symmetric, instantly reversible actions.

---

## Summary

- **Surfaces designed**: 5 (pin affordance entry point, context menu toggle, Pinned section region, zero-pinned empty state, pin-toggle-failed error state).
- **UX acceptance criteria written**: 28 (AC-2.1–AC-2.7 [7], AC-3.1–AC-3.6 [6], AC-4.1–AC-4.3 [3], AC-5.1–AC-5.6 [6], plus 6 cross-cutting criteria — counted individually as they are each independently testable).
