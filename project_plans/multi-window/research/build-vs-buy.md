# Build vs. Buy: Window Layer (Tab Strip, Swipe, State/Persistence)

Research for `project_plans/multi-window/requirements.md`. Scope: the window-switcher
tab strip UI, swipe-gesture handling, and the windows-array + persistence/migration state
machine sitting above the existing pane engine (`web-app/src/lib/pane/`).

## 1. Existing OSS library or framework

### 1a. Tab-strip / window-switcher UI

`@radix-ui/react-tabs` (`^1.1.13`) is **already a direct dependency**
(`web-app/package.json:76`), and is already used in the codebase at
`web-app/src/app/settings/page.tsx`. It's a headless, accessible (ARIA
`tablist`/`tab`/`tabpanel`, roving tabindex, arrow-key navigation) primitive
that provides a controlled `value`/`onValueChange` API — a good structural fit
for "which window is active."

However, the existing **mobile pane tab strip**
(`web-app/src/components/pane/MobilePaneTabStrip.tsx`) does *not* use Radix
Tabs — it's a ~50-line hand-rolled `role="tablist"`/`role="tab"` button row
styled with vanilla-extract (`web-app/src/styles/pane/mobilePaneTabStrip.css.ts`).
It manually implements the same ARIA contract Radix Tabs would give for free,
without the keyboard arrow-navigation or focus-management logic.

Radix Tabs does **not** solve the two hardest parts of this feature:
swipe gestures and rename-on-long-press. Its `TabsTrigger` renders a fixed
`<button>` element (see [radix-ui/primitives#1701](https://github.com/radix-ui/primitives/issues/1701),
an open request to allow custom elements) and has no swipe support at all —
confirmed via Radix's own docs, which describe swipe support only on the
**Toast** and **Slider** primitives, not Tabs. Adopting it would mean writing
custom swipe/long-press logic on top of it anyway, while also conforming to
its DOM/CSS contract — not clearly less work than extending the existing
hand-rolled `MobilePaneTabStrip` pattern the codebase already has for the
sibling "pane tab row" concept.

**Pros**: zero new dependency (already installed and used elsewhere); solid
accessibility baseline (keyboard nav, ARIA) for free; consistent with the one
other tabbed UI in the app (Settings page).
**Cons**: no swipe/gesture support (would still need custom code); fixed
`<button>` trigger element complicates the long-press-to-rename interaction
(would need a wrapping element or an `asChild`-style workaround); doesn't
match the *visual* pattern this feature is explicitly asked to mirror — the
existing hand-rolled `MobilePaneTabStrip`, not Settings' Radix tabs.
**Verdict**: **Viable, but not clearly better than extending the existing
pattern.** If reuse is preferred for its accessibility guarantees, wrap
`Tabs.Root`/`Tabs.List` and layer swipe/long-press on top exactly as would be
done with a hand-rolled strip. Given requirement 4 below (an existing, closer
pattern already in this codebase), recommend the hand-rolled path instead.

### 1b. Swipe-gesture handling

No swipe library is currently a dependency. The closest existing code is
hand-rolled touch-event handling for a different purpose: `useTerminalGestures`
(`web-app/src/lib/hooks/useTerminalGestures.ts`) — a 5-state (`IDLE → PENDING →
SCROLLING | SELECTING | TAPPING`) touch state machine for xterm.js
scroll/select/tap, and `touchDrag.ts`'s `rafThrottlePoint` helper for
per-frame-throttled touch coordinates. Its header comment records an explicit
ADR (ADR-012) choosing raw `TouchEvent` over `PointerEvent` because
`PointerEvent` fires `pointercancel` on iOS mid-gesture. That same
codebase-specific constraint applies to any new swipe handler for the window
tab strip.

Ecosystem options as of September 2026:

| Library | Maintenance | Bundle | Fit |
|---|---|---|---|
| `react-swipeable` (Formidable) | Actively maintained; latest 7.0.2; ~841K weekly downloads | Small, single-purpose (swipe-only) | Simplest API for exactly "swipe left/right to switch," but touch+mouse abstraction may not match the iOS `pointercancel` workaround already baked into `useTerminalGestures` |
| `@use-gesture/react` (pmndrs) | Actively maintained (successor to deprecated `react-use-gesture`) | Larger — supports drag/pinch/wheel/hover, more than needed here | Overkill for a single swipe-left/right requirement; useful only if future gestures (pinch-to-preview windows?) are anticipated |
| Hand-rolled (extend `useTerminalGestures`'s pattern) | N/A — matches existing in-repo pattern | Zero added bytes | Reuses the same `TouchEvent`-based, iOS-`pointercancel`-safe approach already validated in this codebase |

**Pros of pulling in a library**: less code to write and test from scratch;
`react-swipeable` in particular has a simple, well-documented hook API.
**Cons**: neither library is used anywhere else in this codebase — this
would be a new dependency purely for one directional swipe gesture, on a repo
that has already solved (and documented, via ADR-012) the exact touch-event
subtlety (iOS `pointercancel`) a generic library doesn't advertise handling
one way or the other. Bundle-size discipline matters here — `web-app`
enforces a hard 5 MB total JS budget via `size-limit` in `package.json`, and
`make ci`'s size-limit check would need to absorb any new dependency.
**Verdict**: **Not recommended to add a new dependency.** The gesture surface
needed (single-axis swipe left/right on a tab strip, not free-form
drag/pinch) is narrow enough that a small, purpose-built `useWindowSwipe`
hook — following `useTerminalGestures`'s `TouchEvent`-based, RAF-throttled
pattern instead of a generic library's touch+mouse+pen abstraction — is both
less code and lower risk than integrating and testing a new dependency
against this repo's already-known iOS gesture quirk.

## 2. SaaS/managed API

Not applicable. This is a purely client-side UI/state feature — no backend,
no network calls, no server round-trip (per the requirements' explicit
constraint: "No backend/API changes... client-side, localStorage-only").
There is no SaaS surface to evaluate.

## 3. LLM-generated implementation vs. battle-tested library

The window-layer reducer (array of windows, each a `PaneState`, plus an
active-window pointer) is a straightforward generalization of the existing,
already-tested `paneReducer` pattern
(`web-app/src/lib/pane/paneReducer.ts`, 266 lines, with 466 lines of tests in
`__tests__/paneReducer.test.ts`). A `windowReducer` that dispatches
window-scoped actions (`CREATE_WINDOW`, `CLOSE_WINDOW`, `SWITCH_WINDOW`,
`RENAME_WINDOW`) and, for pane-scoped actions, delegates to `paneReducer` on
the active window's `PaneState` is well within the "clearly fine to
hand-write" category — it's the same shape of code the team has already
proven out, and the requirements explicitly frame it as "reusing the existing
pane engine unmodified." No external state-machine library (e.g. XState) is
warranted for an array-plus-pointer reducer this size; that would be adding
formalism disproportionate to the problem, mirroring this repo's own
`@reduxjs/toolkit` dependency already being used for coarser app state while
pane/window state stays as local `useReducer` — consistent with the existing
architectural boundary.

The part that **does** carry outsized correctness risk relative to its
apparent simplicity is the **localStorage schema migration** (v1 single-tree
`PersistedPaneLayout` → v2 array-of-named-windows), for reasons specific to
this codebase, not migrations in general:

- `usePaneLayout.ts`'s `loadPaneLayout()` currently does hard version-gating
  (`if (parsed.version !== 1) return null;` — `web-app/src/lib/pane/usePaneLayout.ts:64`).
  A naive v2 rollout that just bumps this constant would make
  `loadPaneLayout()` return `null` for every v1 user's saved layout on first
  load post-ship — silently discarding it — unless the migration path is
  added *before* or *alongside* the version bump.
- The requirements' own Rabbit Holes and Risk Control sections already flag
  this as the single highest-risk area and mandate: an explicit version
  check, a fallback that preserves the raw pre-migration data untouched if
  migration parsing fails, and a unit test loading a **real v1 fixture**
  asserting correct promotion to "Window 1." That's a strong existing
  test precedent to extend: `usePaneReducer.persistence.test.ts` and
  `usePaneLayout.test.ts` already test load/save round-trips end to end, so
  the new migration test slots into an established pattern rather than
  needing a new one invented from scratch.
- This is a one-way, one-time data transformation with no undo — get it
  wrong and a real user's saved layout is gone. That risk profile (not the
  code's line count) is what justifies extra scrutiny — e.g., have the
  migration function tested in isolation with multiple fixture shapes
  (well-formed v1, `null`, malformed JSON, a v1 payload with unknown extra
  fields) before it's wired into `loadPaneLayout()`.

**Verdict**: **LLM-generated/hand-written implementation is fine for the
window reducer itself (Recommended)**; the **migration function specifically
warrants the extra fixture-based test coverage the requirements already
call for (Viable, with mandatory scrutiny)** — not because the code is
complex, but because the failure mode (silent data loss for existing users)
is asymmetric and irreversible.

## 4. Fork or adapt

`web-app/src/components/pane/MobilePaneTabStrip.tsx` is the closest existing
implementation and the best adaptation target, for reasons the requirements
document itself calls out (Alternatives Considered: "keeping the two related
concepts visually consistent" with "the existing mobile stacked-pane tab
row"):

- It's already a `role="tablist"`/`role="tab"` button row, sized and styled
  via vanilla-extract recipes (`mobileTabStrip`, `mobileTabButton`,
  `mobileAddPaneButton` in `web-app/src/styles/pane/mobilePaneTabStrip.css.ts`)
  that this feature is required to visually echo.
- It takes a `forwardRef` — already set up for a parent to attach gesture
  listeners to the DOM node (as `useTerminalGestures` does with
  `containerRef`), which is exactly the integration point a swipe handler
  needs.
- It's a small, focused component (61 lines) with a narrow prop surface
  (`leaves`, `focusedPaneId`, `sessions`, `onFocus`, `onAddPane`) — cheap to
  copy into a new `WindowTabStrip` component with an analogous prop surface
  (`windows`, `activeWindowId`, `onSwitch`, `onCreate`, `onClose`,
  `onRename`) rather than write from a blank file.

No component in `web-app/src/components/` implements a *window*-level (as
opposed to pane-level) switcher today — this is a new component, but one that
should be built by adapting `MobilePaneTabStrip`'s structure and its sibling
CSS file's conventions, not invented independently. Long-press-to-rename and
swipe-to-switch are both new interactions not present in `MobilePaneTabStrip`
today (it's click-only), so those two behaviors need new code regardless of
which base component is chosen — but the tab-strip shell, ARIA pattern, and
styling approach can be forked directly.

**Verdict**: **Recommended.** Fork `MobilePaneTabStrip.tsx` (and its
`.css.ts` file) into a new `WindowTabStrip` component; add swipe (via a new
`useWindowSwipe` hook modeled on `useTerminalGestures`'s `TouchEvent`
state-machine pattern, not a new dependency) and long-press-to-rename as new
behavior on top of that forked shell.

## Summary of Verdicts

| Component | Decision | Verdict |
|---|---|---|
| Tab-strip UI | Fork `MobilePaneTabStrip.tsx` (not Radix Tabs) | Recommended |
| Radix Tabs as the tab-strip base | Available as fallback if hand-rolled ARIA work becomes a burden | Viable |
| Swipe gestures | Hand-rolled `useWindowSwipe`, modeled on `useTerminalGestures` | Recommended |
| `react-swipeable` / `@use-gesture/react` | New dependency for a single directional gesture | Not recommended |
| SaaS/managed API | N/A — client-side only feature | N/A |
| Window reducer (array + active pointer) | Hand-written, extending `paneReducer`'s pattern | Recommended |
| Migration (v1 → v2 schema) logic | Hand-written but with mandatory extra fixture-based test scrutiny per the requirements' own Risk Control section | Viable, with scrutiny |
| XState or other state-machine library | Disproportionate formalism for an array + pointer reducer | Not recommended |
