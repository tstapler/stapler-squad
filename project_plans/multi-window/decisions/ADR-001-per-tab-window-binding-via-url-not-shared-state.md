# ADR-001: Per-tab active window is derived from the URL, not a shared reducer field

**Date**: 2026-09-23
**Status**: Accepted

## Context

`research/architecture.md` (Phase 2, written before the per-tab URL requirement was
added to `requirements.md`) recommends a `WindowState = { windows: NamedWindow[],
activeWindowId: WindowId }` shape, with `SWITCH_WINDOW` as a reducer action that
mutates the shared `activeWindowId`. That shape assumes exactly one "current window"
per browser origin.

`requirements.md`'s Scope now adds: "the active window is addressable via a URL query
param (`?window=<id>`), so different browser tabs of the same app can independently
display different windows... only *which window this tab is currently looking at* is
per-tab (driven by the URL, not a shared `activeWindowId`)." A shared `activeWindowId`
field — whether in the reducer or in the persisted localStorage blob — cannot satisfy
this: two tabs would contend over one shared pointer, and switching in one tab would
move the other tab's view, which is the exact bug requirements.md explicitly rules out
(Success Metrics: "switching windows in one tab does not move the other tab").

## Decision

Drop `activeWindowId` from `WindowsState` (the shared, persisted, reducer-owned state)
entirely. The persisted shape is just `{ version: 2, revision: number, windows:
NamedWindow[] }` — no active pointer.

The "which window is this tab showing" concept (`currentWindowId`) is derived,
per-tab, from:
1. `?window=<id>` URL query param, if present and it names a window that still exists.
2. Else, a `lastFocusedWindowId` hint — a single small localStorage value (not part of
   the versioned `windows` blob, no revision guard needed since it's a single scalar
   with no partial-tree data to lose) — if it still names a window that exists.
3. Else, `windows[0].id` (first window in array order, i.e. what a fresh v1→v2
   migration produces as "Window 1").

Switching windows (tab click, leader-key shortcut, swipe) is **URL navigation**
(`router.replace` with an updated `?window=` param in the current tab only), not a
dispatched reducer action. This mirrors the existing `?session=`/`?tab=` pattern
already implemented in `web-app/src/app/page.tsx`'s `updateUrl` callback.

## Consequences

- Two tabs on different windows never contend over one field — there is no shared
  field to contend over. This satisfies the per-tab requirement by construction,
  not by conflict resolution.
- Window *content* edits (rename, pane splits within a window) still write to the
  single shared `windows` array and do need the cross-tab revision guard (see
  ADR-002) — this ADR only removes the *active-window pointer* from that shared
  state, not the windows themselves.
- A window can be closed in one tab while another tab's URL still points at it. That
  tab must detect the mismatch (`currentWindowId` resolution falls through step 1 to
  step 2/3) and self-correct its own URL — implemented as a `router.replace` effect,
  the same "self-healing URL" pattern `page.tsx` already uses to strip a consumed
  `?newPane=true` param.
- `research/architecture.md`'s `WindowAction` sketch (`SWITCH_WINDOW` as a dispatched
  action) is superseded: there is no `SWITCH_WINDOW` case in `windowReducer.ts`.
