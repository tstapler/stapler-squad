# Research: UX for Session Pinning

Source requirements: `project_plans/session-pinning/requirements.md` (backlog item `9959d36a-01e7-4dce-92bd-74ee87b2c99d`).

## 1. Comparable UX patterns

All of the following are **UNVERIFIED / inferred from general public-product knowledge** — reasoned from memory of widely-used products, not fetched or re-verified against current UI during this research pass. herdr-web's `web/src/agentPins.ts` specifically is **not present in this repo or on this machine** (checked: `find / -iname "*herdr*"` returns nothing), so its behavior is taken solely from the one-line description in the requirements doc ("pinned panes surface at the top of the sidebar") and not independently verified.

| Product | Pin location | Icon/state | Confirmation on unpin | Animation |
|---|---|---|---|---|
| Slack (pinned messages/channels) | Right-click / kebab menu → "Pin to channel"; pinned items surface in a "Pinned" panel accessible from the channel header | Pin icon (📌) fills/solid when pinned, outline when not | No confirmation dialog — single click toggles | Brief highlight flash on pin; item disappears from pinned list immediately on unpin |
| Browser pinned tabs (Chrome/Firefox) | Right-click tab → "Pin tab"; pinned tabs move to the leftmost strip, shrink to icon-only | Pin glyph rotates 45° when tab is pinned (visual metaphor: a physical pin stuck in) | No confirmation | Tab animates/slides to the pinned position |
| VS Code pinned tabs/editors | Right-click tab → "Pin"; pinned tab moves to the front of the tab strip, gets a dot/pin badge, and won't be replaced by "preview mode" single-click opens | Small pin icon in the tab, replacing the dirty-dot indicator | No confirmation | Tab slides to leftmost position |
| GitHub pinned repos | Explicit "Customize your pins" UI on the profile page, drag-to-reorder, max 6 | Pin icon toggle, filled vs outline | No confirmation | None notable |
| macOS/Windows taskbar pinning | Right-click app icon → "Pin to Taskbar/Dock" | Icon itself doesn't change state visually in the dock; the context menu item toggles between "Pin" and "Unpin from Taskbar" | No confirmation | Icon animates into place in the dock/taskbar |
| herdr-web agent-pins (per requirements doc description only) | Pinned panes surface at top of sidebar | Not specified in source doc | Not specified | Not specified |

### Shared conventions across all of these
- **Icon state, not text-only**: pin state is almost universally shown via a two-state icon (outline vs filled, or angle change) rather than a text label alone — text label is secondary/tooltip.
- **No confirmation on unpin**: every reference product treats unpin as a reversible, low-stakes action — no "are you sure?" dialog anywhere. Pin/unpin is symmetric and instantly reversible, which argues for **optimistic UI with no confirmation modal** for this feature too.
- **Pinned location is deterministic and top-anchored**: pinned items always surface at a fixed, predictable position (top of list/sidebar, left of tab strip), never interleaved with the unpinned set. This directly matches FR4 ("Pinned" section above the normal grouped/sorted list).
- **Toggle is reachable from a secondary/context menu**, and in several cases (VS Code, browser tabs) also from a persistent icon on the item itself once you know to look — but the *primary* discovery path is a menu item, matching FR6 ("session card context menu and/or session detail header").
- **Animation is a nice-to-have, not a requirement**: Slack/VS Code/taskbar all animate the item moving into its pinned position, but this is polish, not core interaction contract. Given this repo already virtualizes the session list (`GroupedVirtuoso` in `web-app/src/components/sessions/SessionList.tsx`), an item moving between the flat list and a separate "Pinned" section on pin/unpin is a **list re-render**, not a simple CSS transition — recommend skipping animation for v1 and revisiting only if the re-render reads as jarring in practice.

## 2. User mental models: pin vs. favorite/star/bookmark

- **"Pin" implies location/persistence-at-top** — the user's expectation is specifically "keep this where I can always see it, regardless of what else changes," which matches this feature's stated goal (visible "regardless of recency/status/sort order," per the Problem statement). "Star"/"favorite" carries a softer, more general "I like/care about this" connotation and is often used for *filtering/searching* a large set (e.g. Gmail stars, GitHub starred repos) rather than *repositioning* a small number of items to the top of the current view. "Bookmark" implies "save for later, out of the main flow" (browser bookmarks live in a separate panel, not inline in the tab strip) — the opposite of this feature's intent, which is to keep the pinned session *inline and prominent*, not tucked away.
- Given the functional requirement is specifically about position/visibility survival across sort/status/recency (FR4), **"pin" is the correct mental model and label** — not "favorite" or "star." Using a pin icon and "Pin"/"Unpin" verbiage will match user expectations better than a star.
- **Existing icon usage in this codebase**: grepped `web-app/src/components/` and `web-app/src/lib/` for `Star`, `Pin`, `favorite`, `bookmark` (case-insensitive) — zero matches for an actual Star or Pin *icon component* in the session/backlog UI. The only "Pin" hits are unrelated (a comment about lucide-react's pinned *version* number, and a test comment about pinning a millisecond boundary). **No prior art or naming collision exists in this codebase for pin/star/favorite** — this feature is free to establish the convention from scratch.
- **Icon library**: `lucide-react` v1.14 is the pinned icon dependency (`web-app/package.json:81`, confirmed used elsewhere e.g. `ChevronRight`/`ChevronDown` in `SessionList.tsx:218`). lucide-react ships both a `Pin` and `PinOff` icon — use those rather than emoji, for consistency with the rest of the list-chrome (chevrons, etc.) which already uses lucide icons rather than emoji. Note this is inconsistent with `SessionActionsOverflow.tsx`, which uses raw emoji (📍, 🗑️, ▶️, etc.) for most menu-item icons — see Accessibility section below for the recommendation on which pattern to follow for the *menu item* specifically vs. any persistent on-card icon.

## 3. Accessibility

- **This project's existing toggle pattern** (verified by reading `web-app/src/components/sessions/SessionActionsOverflow.tsx:701-720`, the autonomous-mode toggle menu item):
  ```tsx
  <button
    role="menuitemcheckbox"
    aria-checked={session.autonomousMode}
    aria-label={session.autonomousMode ? `Stop running ${session.title} autonomously` : `Run ${session.title} autonomously`}
    onClick={...}
  >
    <span aria-hidden="true">{session.autonomousMode ? "⏹" : "🤖"}</span>{" "}
    {session.autonomousMode ? "Stop running autonomously" : "Run autonomously"}
  </button>
  ```
  This is the established, repo-native pattern for a stateful toggle inside a dropdown/menu: `role="menuitemcheckbox"` + `aria-checked` (not `aria-pressed`, which is for standalone toggle buttons, not menu items — `menuitemcheckbox` is the ARIA-correct role inside a `menu`/`role="menu"` container). The icon is `aria-hidden` decoration; the real accessible name comes from `aria-label`, which is dynamic and describes the *action* (what will happen), not just the current state — e.g. "Stop running X autonomously" rather than "Autonomous mode: on."
- **Recommendation for the pin toggle**: follow this exact pattern for the context-menu entry —
  ```tsx
  <button
    role="menuitemcheckbox"
    aria-checked={session.pinned}
    aria-label={session.pinned ? `Unpin ${session.title}` : `Pin ${session.title}`}
    data-testid="session-pin-toggle"
    onClick={(e) => { e.stopPropagation(); close(); onTogglePinned(session.id, !session.pinned); }}
  >
    <Pin aria-hidden="true" size={16} /> {session.pinned ? "Unpin" : "Pin"}
  </button>
  ```
  If a *second* pin affordance is added directly on the session card/detail header (FR6 says "and/or"), it should be a standalone icon button, not a menu item — use `aria-pressed` there instead (per ARIA authoring practices, `aria-pressed` is for a standalone toggle button, `aria-checked`/`menuitemcheckbox` only applies inside a menu). Don't reuse `aria-checked` outside a menu context.
- **Keyboard**: no special handling needed beyond what menu items and buttons already get for free — `role="menuitemcheckbox"` inside the existing menu (`role="menu"`) already participates in the arrow-key roving-tabindex navigation this repo's menu presumably implements for its other `menuitemcheckbox` (autonomous mode) and `menuitem` entries; Enter/Space activates. Verify this is generic in whatever menu wrapper component renders `SessionActionsOverflow`'s items (not independently confirmed in this pass — check the menu container implementation during planning).
- **Locator convention**: per `.claude/rules/e2e-test-conventions.md`, any e2e test for this feature must use `data-testid` or ARIA role selectors only — e.g. `page.getByTestId("session-pin-toggle")` or `page.getByRole("menuitemcheckbox", { name: /pin/i })`, never a CSS class.
- **Screen-reader label for the "Pinned" section itself**: give the section container a heading or `aria-label` (e.g. `<h2>Pinned</h2>` or `role="region" aria-label="Pinned sessions"`) so screen-reader users get an announced landmark distinguishing it from the regular grouped list below, mirroring the existing `role="region" aria-label="No results"` pattern already used for the empty state in `SessionList.tsx:1203`.

## 4. Error/edge-case UX

- **Optimistic update + rollback, not pessimistic.** Every comparable product (Slack, VS Code, browser tabs, taskbar) treats pin/unpin as instant and local — there's no spinner or "pinning..." state anywhere in the reference set, which sets a strong user expectation of immediacy. This codebase already has a precedent for this exact pattern: `SessionRow.tsx`'s comment references "optimistic approval suppression" and `clearedSessions`/`SubStatusChip suppression" — i.e. optimistic local state that's reconciled against server state, with a documented suppression/rollback mechanism. Recommend: toggle the icon/section membership immediately on click, fire the RPC, and on failure revert the local state and surface a toast/notification (this repo has `NotificationToast.tsx` / `NotificationPanel.tsx` already for this exact purpose) rather than blocking the UI on the round-trip.
- **Zero pinned sessions**: **hide the "Pinned" section entirely**, don't show an empty-state placeholder. Rationale: (a) none of the reference products (Slack pinned panel, VS Code, browser pinned tabs) show a persistent empty "Pinned" section when nothing is pinned — the affordance appears only once something is pinned; (b) this repo's own convention for the *main* list uses an explicit empty state only when the list itself is the primary content and filters are active (`SessionList.tsx:1203`, `role="region" aria-label="No results"` shown when `filteredSessions.length === 0 && hasActiveFilters`) — that's a "your filter matched nothing" signal, a fundamentally different case from "you haven't used this optional feature yet." A permanently-visible empty "Pinned" section would add persistent chrome for a feature most sessions/users won't touch by default, which every reference product avoids.
- **Pinned session gets archived**: the requirements doc explicitly punts this ("pinning archived sessions (decide in research)" is listed as out-of-scope-pending-decision). Recommendation: **allow pinned sessions to remain pinned when archived, and continue showing them in the Pinned section** even if archived sessions are otherwise filtered out of the main list by default — this matches the core value prop (FR: "regardless of status") and avoids a surprising silent unpin. However, this needs an explicit product decision before planning locks it in, because it interacts with the existing archived-sessions filter (`SessionList.tsx:576`, "Archived filter — hidden by default"): if a pinned+archived session shows in the Pinned section while the main list hides archived sessions by default, that's a deliberate, user-visible exception to the archived filter and should be called out in the plan/requirements, not left implicit.
- **Pinned session gets deleted**: no special handling needed beyond what already happens when a session is deleted elsewhere in the UI — it should simply disappear from the Pinned section along with everywhere else, with no separate "this session was pinned" messaging. Deletion already removes a session from every other view; pinning doesn't need to add friction or a confirmation step to that existing flow.
- **RPC failure surfacing**: use the existing toast/notification pattern (`NotificationToast.tsx`) rather than inventing new error UI — e.g. "Couldn't pin session — try again" — consistent with how other quick actions in this list (rate-limit toggle, autonomous mode) would presumably surface errors, though this pass did not verify their specific failure-path UI in depth.

## 5. Jobs-to-be-done

- **Functional job**: quick, reliable re-access to a small set of sessions the user considers currently important, without having to re-sort/re-filter/re-search the full list every time — directly served by FR4 (dedicated top section, independent of status/recency/sort).
- **Emotional job**: reduce anxiety about losing track of an important in-flight session as the list grows and reorders around normal activity (new sessions pushing old ones down, status changes reshuffling groups). The "survives reloads and browser/device switches" requirement (FR5) is itself in service of this emotional job — a pin that didn't persist would undermine the very reassurance it's meant to provide, which is why FR2 (server-persisted, not localStorage) is the right call rather than a client-only convenience feature.
- **Social job**: **not applicable** — this is a single-user local tool (per the requirements doc's explicit exclusion of "cross-workspace pin sync" from scope, and the general single-operator nature of stapler-squad session management). No other user ever sees another's pin state, so there's no social signaling dimension to this feature, unlike e.g. GitHub's public pinned-repos feature which *does* carry a social/curation job ("here's what I want visitors to see").

## Open questions for planning phase

1. Should archived sessions remain visible in the Pinned section (see §4) — needs an explicit decision, not left to implementation-time judgment.
2. Confirm the menu-item keyboard/roving-tabindex behavior generically applies to a new `menuitemcheckbox` entry (not independently verified against the actual menu wrapper component in this pass).
3. Decide whether a persistent pin icon on the session card itself (in addition to the context-menu entry) is in scope for v1, per FR6's "and/or" — if yes, use `aria-pressed` there per §3, not `aria-checked`.
