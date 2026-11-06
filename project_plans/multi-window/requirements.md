# Requirements: multi-window

**Date**: 2026-09-23
**Type**: feature addition
**Complexity**: 3 — system design

## Problem Statement
The cockpit pane-tiling engine (`web-app/src/lib/pane/`) manages exactly one pane tree at a time, persisted under the single localStorage key `cockpit.paneLayout`. Users juggling several unrelated work contexts (e.g. "reviewing PR #x" vs. "debugging session y" vs. "monitoring three long-running agents") have nowhere to put them but the same flat tree — they either resize/split increasingly cramped panes to fit everything, or repeatedly tear down and rebuild the layout as they context-switch. There is no way to group related panes together and set that group aside while working on something else, then come back to it exactly as it was.

## Baseline
Today, a user with multiple concurrent work contexts either:
- Crams all of them into one pane tree via nested splits, leaving individual panes very small once more than 2-3 are open, or
- Repeatedly splits/closes/reassigns panes to reuse the same tree for different contexts, losing the previous arrangement each time, or
- Opens multiple browser tabs pointed at the same app hoping for isolation, but gets none: `cockpit.paneLayout` lives in localStorage, which is shared per-origin, so every tab shows the identical layout and stomps on the others' edits.

## Users / Consumers
Desktop and mobile users of the stapler-squad web cockpit who work across multiple concurrent session contexts — the same audience as cockpit-tiling, now needing a second axis of organization above panes.

## Success Metrics
- A user can create a new window, build an independent pane layout in it, and switch back to a previous window's exact layout (same splits, sizes, assigned sessions/tabs) in under 2 seconds of interaction — no rebuild required.
- Existing single-layout users see zero disruption on first load after ship: their current layout becomes Window 1 automatically.
- All window-switching interactions (tab bar click, leader-key sequence, swipe on mobile) are reachable without a full page reload.
- A user can open a second browser tab, navigate it to a different window's URL (`?window=<id>`), and both tabs stay independently on their own window — switching windows in one tab does not move the other tab.
- `make ci` passes; Jest coverage for the window-level reducer/state machine matches the bar already set for the pane reducer (`usePaneReducer`/`paneReducer` tests).

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- Builds directly on the existing pane-tiling data model (`PaneState`, `paneReducer`, `paneTypes`, `paneUtils`) — this is an additional layer above it (a collection of named `PaneState` trees), not a replacement. Note: per Phase 3 planning, `usePaneReducer` itself (the hook, as opposed to the reducer/types it wraps) is *not* reused — `PaneTilingContainer` takes `paneState`/`dispatch` as props from the new window layer instead of calling that hook internally, per the "Isolate via seam" tech debt disposition. See `implementation/plan.md`'s Unresolved Questions for whether the now-unused hook is deleted in this PR or a follow-up.
- No backend/API changes; this is a client-side, localStorage-only feature like cockpit-tiling.
- Must not break existing single-window users' saved layouts — auto-migrate the current `cockpit.paneLayout` value into "Window 1" on first load after ship (transparent, no user action required).

## Non-functional Requirements
- **Performance SLO**: window switch must feel instant — no visible re-render jank, target < 100ms perceived switch time.
- **Scalability**: not applicable — bounded by how many windows a single user creates (expect low double digits at most).
- **Security classification**: internal — no new data leaves the browser; same classification as cockpit-tiling.
- **Data residency**: no special requirements — localStorage only, no server round-trip.

## Scope
### In Scope
- A new "window" layer: a named, ordered collection of independent pane trees (each window contains its own `PaneState`, reusing the existing pane engine unmodified).
- Window switcher UI: a tab bar (top on desktop, bottom on mobile) showing all open windows, following the same visual language as the existing mobile stacked-pane tab row.
- Create, close, and switch between windows.
- Rename windows (double-click/long-press a tab), similar to tmux `Ctrl+b ,`.
- Keyboard shortcuts for window switching: a tmux-style leader-key sequence — `Alt+W` (decided in Phase 3; deliberately *not* `Ctrl+B`, tmux's own default prefix, nor `Ctrl+A`, its common `screen`/remap alternative, since this app's whole purpose is displaying live tmux-backed terminal panes and a `Ctrl`-based leader would collide with a user's own real tmux prefix keystrokes sent into a pane), held or tapped then followed by a digit to jump to a specific window, or `n`/`p` to cycle next/previous — chosen over a bare `Ctrl+<number>` modifier because that combo is claimed at the browser-chrome level (Windows/Linux tab switching) and the OS level (macOS Mission Control), neither interceptable via `preventDefault()` (confirmed in Phase 2 research: `research/pitfalls.md`, `research/features.md`). Registered through the existing `ShortcutRegistry` and shown in the `?` shortcut overlay.
- Swipe left/right gesture to switch windows on mobile/touch, alongside the tab strip.
- Full persistence: all windows, their pane trees, order, names, and which window is active survive a page reload (extends the existing `cockpit.paneLayout` persistence scheme).
- Automatic, transparent migration of any existing single-tree `cockpit.paneLayout` value into "Window 1" the first time a user loads the app after this ships.
- "Close window" confirmation when it's the last remaining window with unsaved... (n/a — everything is local state, so no unsaved-data concept beyond the tree itself); still needs a decision on what happens when the last window is closed (see Rabbit Holes).
- **Per-tab window binding**: the active window is addressable via a URL query param (`?window=<id>`), so different browser tabs of the same app can independently display different windows. The shared window *list* (which windows exist, their pane trees, names, order) still lives in localStorage and is visible to every tab; only *which window this tab is currently looking at* is per-tab (driven by the URL, not a shared "activeWindowId"). Opening the app with no `?window=` param falls back to the last-focused window (or Window 1 on first load).

### Out of Scope
- Cross-device or server-side sync of windows (same non-goal as cockpit-tiling — localStorage only).
- Named/saved window *presets* that can be recalled by name across browsers/devices (distinct from simply renaming a window you already have open).
- Drag-and-drop reordering of window tabs (may follow later; not required for this version).
- Any change to the pane-tiling engine's own behavior (splits, resize, zoom, tab assignment within a pane) — this feature only adds the layer above it.

## Rabbit Holes
- **Migration correctness**: the existing `PersistedPaneLayout` (`version: 1`) schema must be versioned forward (e.g. `version: 2` wrapping an array of named windows) without silently dropping a user's current layout if migration logic has a bug — needs an explicit fallback path and a test that loads a real v1 fixture.
- **Keyboard shortcut collisions**: resolved in Phase 2 research — a bare `Ctrl+<number>` is unusable (browser-chrome tab-switching on Windows/Linux, OS-level Mission Control on macOS, neither interceptable). The leader-key sequence adopted instead was decided in Phase 3 as `Alt+W`, chosen to avoid colliding both with existing pane shortcuts (`Ctrl+\`, `Ctrl+-`, `Ctrl+W`, `Ctrl+Z`, focus/resize arrows) and — the more critical collision, per `implementation/pre-mortem.md` P1 #1 — with tmux's own real prefix keys (`Ctrl+B` default, `Ctrl+A` common remap), since a `Ctrl`-based leader would otherwise arm this app's outer leader mode whenever a user sends a genuine tmux prefix keystroke into a terminal pane.
- **Last-window semantics**: deciding what happens when a user closes the only remaining window (block it, auto-create a fresh blank window, or something else) — easy to hand-wave in planning, easy to get wrong in implementation.
- **Mobile tab-strip density**: with both windows (new) and the existing mobile stacked-pane tab row (existing, for panes within a window) potentially both visible at once on a small viewport, the two navigation layers must stay visually distinguishable — this is new information architecture, not just "add another row of tabs."
- **Rename UX on touch**: long-press-to-rename must not conflict with the swipe-to-switch gesture on the same tab strip.
- **Cross-tab write concurrency, now load-bearing rather than edge-case**: Phase 2 pitfalls research (`research/pitfalls.md`) already flagged that concurrent localStorage writes to `cockpit.paneLayout` are currently unhandled. Per-tab window binding makes this a routine occurrence rather than a rare one — two tabs on *different* windows will both debounce-save the *same* shared window-list localStorage key whenever either one edits any window (e.g. a rename, a pane split). Needs the read-check-write revision-counter guard the pitfalls research recommended, plus a live-update mechanism (e.g. a `storage` event listener) so a tab viewing Window 2 picks up a rename that happened to Window 2 from another tab without requiring a manual reload.

## Alternatives Considered
- Sidebar window list instead of a tab strip — rejected in favor of a tab-bar/tab-strip approach that mirrors tmux's own window list and the existing mobile stacked-pane tab row, keeping the two related concepts visually consistent (see research notes from Phase 1 interview: NN/G tabs guidance, tmux window-as-browser-tabs pattern, mobile tmux client patterns from mobux/tuimux).
- Cross-device synced named layout presets — deferred; out of scope for this version, may be a future project once this ships.

## Feasibility Risks
- The pane engine's reducer/state (`usePaneReducer`) currently assumes a single `PaneState` instance wired directly into `PaneTilingContainer`; introducing a window layer means this hook (or a new wrapping hook) must manage an array of `PaneState` instances and an active-window pointer without changing the pane reducer's own internal behavior — a boundary that needs to be gotten right early (Phase 3 planning) to avoid rework.
- Mobile viewport real estate: fitting both a window tab strip and (when panes are stacked) a pane tab row on a small screen without feeling cramped is a genuine UX risk, not just a layout tweak.

## Observability Requirements
Standard client-side error logging is sufficient — no new metrics or alerts. If a migration failure occurs (v1 → v2 schema), it should log a client-side error (existing logging pattern) so it's visible in browser error monitoring, but does not need a dedicated oncall alert.

## Risk Control
No feature flag — same risk profile as cockpit-tiling (client-side only, no server round-trip). Mitigate migration risk instead with: an explicit schema version check, a fallback that preserves the raw old data untouched if migration parsing fails, and a unit test that loads a real pre-migration `cockpit.paneLayout` fixture and asserts it becomes Window 1 correctly.

## Open Questions
*(all three resolved by Phase 2 research — carried into Phase 3 planning, not re-opened)*
- ~~Exact behavior when the last window is closed~~ → resolved: auto-recreate a blank Window 1 rather than blocking the close (tmux precedent; this app has no "end session" analog). See `research/features.md`.
- ~~Whether `Ctrl+<number>` is available on all target platforms~~ → resolved: no, it's claimed at the browser-chrome level (Windows/Linux) and OS level (macOS Mission Control), neither interceptable. Replaced with a leader-key sequence (see Scope and Rabbit Holes above). See `research/pitfalls.md`, `research/features.md`, `research/stack.md`.
- ~~Exact visual treatment for keeping the window tab strip and pane tab row distinguishable~~ → resolved: a second, independently-labelled `role="tablist"` (`aria-label="Window switcher"` vs. the existing pane strip's own label) mirroring `MobilePaneTabStrip.tsx`'s existing accessibility pattern, with distinct styling tokens. See `research/ux.md`, `research/stack.md`.
