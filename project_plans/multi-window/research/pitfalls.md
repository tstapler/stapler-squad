# Multi-Window: Pitfalls and Risks

Research for the "window" layer above the existing pane-tiling engine
(`web-app/src/lib/pane/`), per `project_plans/multi-window/requirements.md`.
Builds on `project_plans/cockpit-tiling/research/04-pitfalls.md` (that doc's
Ctrl+W / Ctrl+- / ShortcutContext / mobile-breakpoint findings apply
unchanged — not re-derived here).

## 1. localStorage schema migration (v1 → v2)

### Existing code, read directly (`web-app/src/lib/pane/usePaneLayout.ts`)

- `loadPaneLayout()` already fails closed: `if (parsed.version !== 1) return null`,
  plus explicit field checks (`!parsed.root`, `typeof focusedPaneId !== "string"`,
  `!("zoomedPaneId" in parsed)`). Any of these failing returns `null`, and the
  caller falls back to a fresh default layout — this is exactly the "fallback
  that preserves... doesn't silently corrupt" pattern the requirements ask
  for, and it's already proven in this codebase. **Reuse this shape for v2,
  don't invent a new one.**
- **Pitfall — strict `!== 1` becomes a trap once v2 exists.** The moment
  `PersistedPaneLayout.version` can be `1` or `2`, a naive migration that does
  `if (parsed.version === 1) { wrap into Window 1 }` and otherwise assumes v2
  will silently produce `undefined`/garbage for a v3 or corrupted-version
  value unless there's an explicit `else return null` (or an explicit
  unknown-version branch). Write the version check as a switch/exhaustive
  match, not an if/else pyramid, so a genuinely unrecognized version value
  fails closed instead of falling through.
- **Pitfall — the "wrap v1 into Window 1" migration must run exactly once,
  synchronously, before first render**, and must itself be defensive: if the
  v1 `root`/`focusedPaneId` fields are itself malformed (a corrupted v1 blob),
  `validateAndRepair`-style repair must run *after* wrapping, not skip
  because "it's an old format, trust it." Add the fixture test the
  requirements call for (`project_plans/multi-window/requirements.md`'s
  Rabbit Holes: "a test that loads a real v1 fixture") as a literal captured
  `localStorage.getItem("cockpit.paneLayout")` value from this repo's current
  schema, not a hand-typed approximation — hand-typed fixtures tend to drift
  from what the real serializer actually produces (e.g. this codebase's
  `PaneNode` discriminated union nests `first`/`second`, easy to get subtly
  wrong by hand).
- **Pitfall — partial-write corruption is already possible today and gets
  worse with a bigger blob.** `savePaneLayout()` does a single synchronous
  `localStorage.setItem(STORAGE_KEY, JSON.stringify(layout))` with no atomicity
  guarantee across the write. A tab crash, OS-level force-quit, or the
  browser hitting a quota error mid-`setItem` can leave a truncated/invalid
  JSON string. Today (`version: 1`, single tree) `JSON.parse` on that throws
  and is caught (`catch { return null }`), so this already degrades to "start
  over from empty" — acceptable for a single tree, much less acceptable once
  the value holds *every* window a user has open. **Recommendation:** for v2,
  either (a) keep a single-key write (simplest, matches existing pattern,
  accept "corrupt = default back to Window 1 empty" as the same worst case
  cockpit-tiling already ships with), or (b) if window count/blob size makes
  that risk feel worse in practice, write windows under per-window sub-keys
  (`cockpit.window.<id>` + a small `cockpit.windowOrder` index) so one
  truncated write only loses one window, not all of them — but this adds
  real complexity (index/window consistency, orphaned per-window keys on
  ungraceful close) that the Appetite (3–6 weeks) may not have room for.
  Default to (a) unless testing surfaces it as a real problem.

### Cross-tab concurrent writes — NOT covered by `04-pitfalls.md`; derived here

`project_plans/cockpit-tiling/research/04-pitfalls.md` does not address
concurrent writes to `cockpit.paneLayout` from two tabs of the same origin —
it was written before that was in scope. Confirmed by reading
`web-app/src/lib/pane/usePaneReducer.ts:105-114`: the debounced save
(300ms) has no `window.addEventListener("storage", ...)` listener anywhere
in `web-app/src/lib/pane/` or `web-app/src/components/pane/` (grepped, no
hits) — each tab writes blind, unaware of what any other tab last wrote.

This is a real, reachable scenario the requirements' own Baseline section
calls out ("Opens multiple browser tabs pointed at the same app... actually
shared across tabs since localStorage is per-origin"), and multi-window
makes the failure mode worse, not just present:

- **Lost-update race (last-write-wins).** Tab A and Tab B both load the same
  `cockpit.paneLayout` v2 blob at T0. User edits Window 3 in Tab A; 300ms
  later Tab A writes the *entire* windows array back. Meanwhile Tab B, still
  holding its T0 snapshot, edits Window 1 and writes its own full array at
  T0+310ms — silently overwriting Tab A's Window 3 edit with the stale T0
  copy. No error, no conflict signal; the user just loses work in whichever
  tab wrote first, and won't notice until they switch back to it.
- **This is strictly worse for multi-window than for a single pane tree**
  because the blob being clobbered is now "every window," not "the one tree
  you're looking at" — a user who opened a second browser tab specifically
  to work on a *different* window (a plausible thing to do, since windows
  are the feature's own answer to "juggling several unrelated contexts")
  is the exact user most likely to hit this.
- **Recommended mitigation (matches Risk Control's "no feature flag, mitigate
  the specific risk instead" posture):** add a `window.addEventListener("storage", handler)`
  listener that, on seeing a `cockpit.paneLayout` change from another tab,
  either (a) re-reads and merges only if the current tab has no in-flight
  unsaved edit (compare a monotonic revision counter stored alongside the
  layout, not wall-clock time — clock skew/backdating makes wall-clock
  compares unreliable), or (b), the cheaper option that fits a 3–6 week
  appetite better: on save, read-check-write — read the current stored value
  immediately before writing, and if its revision counter has moved past
  what this tab last loaded, log a client-side warning (per the
  Observability Requirements' existing "log client-side errors" pattern) and
  skip the write rather than clobbering. This doesn't reconcile the two tabs'
  edits, but it converts silent data loss into a detectable, logged event —
  proportional to a feature with no dedicated oncall alert. True
  cross-tab reconciliation (broadcast + merge) is out of scope for the
  appetite; note it as a known limitation rather than solving it.
- The existing single-tree `validateAndRepair` pass (called against the
  current session list on load) still needs to run per-window after
  migration/every reload — stale `sessionId`s inside a *background* window's
  tree are just as real a problem as in the focused one, and easier to miss
  in testing since that window isn't visibly rendered.

## 2. Keyboard shortcut: `Ctrl+<number>` is not safe on either major platform

The Rabbit Holes section already flags this as needing a "quick feasibility
check" before committing — that check comes back negative for `Ctrl+<number>`
as specified:

- **Windows/Linux (Chrome, Firefox, Edge):** `Ctrl+1` through `Ctrl+8` are
  browser-chrome-level "switch to tab N by position" shortcuts, and `Ctrl+9`
  jumps to the last tab. These are **not** ordinary page-level `keydown`
  bindings — they're intercepted by the browser's own tab-strip UI before
  a page's `keydown` listener reliably gets a chance to `preventDefault()`
  them, unlike `Ctrl+W` (which `04-pitfalls.md` §3 already found to be only
  *partially* interceptable). In practice, `event.preventDefault()` in
  `ShortcutRegistry.dispatch()` does **not** stop the browser from switching
  its own tab on `Ctrl+1..9` in Chrome/Firefox/Edge on Windows/Linux — this
  is a stronger, OS/browser-chrome-owned binding than the ones already
  documented as merely risky.
- **macOS:** `Ctrl+<number>` (1 through 4, sometimes more depending on
  configured Spaces count) is macOS's own **Mission Control "switch to
  Desktop N"** shortcut by default (System Settings → Keyboard → Keyboard
  Shortcuts → Mission Control). This fires at the OS level, *before* any
  browser or page JavaScript ever sees a `keydown` event — there is no
  `preventDefault()` that reaches it at all, page-side interception is not
  merely unreliable, it's structurally impossible. (Chrome/Safari on macOS
  bind tab-switching to `Cmd+<number>` instead, so on Mac the *browser*
  collision moves to `Cmd`, but the *OS* collision on `Ctrl` is still there
  and is worse because it can't be worked around in-page at all.)
- **Conclusion: `Ctrl+<number>` should not ship as specified.** It collides
  with browser-chrome tab switching on Windows/Linux (interceptable in
  theory, unreliable in practice per the existing `Ctrl+W`/`Ctrl+-` findings)
  and with OS-level Mission Control on macOS (not interceptable at all).
  Recommended fallback, consistent with `04-pitfalls.md`'s own escape-hatch
  pattern for `Ctrl+-` ("fallback to `Ctrl+Shift+H`"): use
  **`Ctrl+Alt+<number>`** or **`Alt+<number>`** for window-jump — neither
  collides with a documented default browser or macOS OS-level binding.
  `Ctrl+n`/`Ctrl+p` (next/previous) has the same Windows/Linux browser-chrome
  risk to check (Firefox binds `Ctrl+Tab`/`Ctrl+Shift+Tab` for this, not
  `Ctrl+n`/`Ctrl+p`, so those two are likely safer than the numbered jump —
  but `Ctrl+n` is "new browser window" in most browsers, another chrome-level
  binding, and needs the same scrutiny before committing).
- Whatever binding is chosen, register it under the new `"cockpit"`
  `ShortcutContext` (already added to `web-app/src/lib/shortcuts/shortcutRegistry.ts:1`
  alongside `"omnibar"`) so it shows up in the `?` overlay and follows the
  same terminal-context carve-out logic `dispatch()` already applies
  (`shortcutRegistry.ts:102-104`).

## 3. Swipe gestures on touch devices

- **Browser back/forward swipe navigation.** Safari on iOS (and Chrome on
  Android, to a lesser degree via "overscroll navigation") treats a
  left-edge or right-edge horizontal swipe as "go back"/"go forward" in
  history. A window-switch swipe gesture implemented as a generic
  `touchstart`/`touchmove`/`touchend` listener on the window tab strip or
  its container will race with this **unless the swipe-catching region
  avoids the viewport edges** (roughly the outer 20–30px on iOS Safari,
  per Apple's own edge-swipe-back gesture) or the app opts out via
  `overscroll-behavior-x: none` / `touch-action: pan-y` CSS on the
  containing scroll region — the latter must be scoped narrowly (the window
  tab strip's own bounding box, not the whole page) or it will also break
  legitimate vertical scrolling elsewhere.
- **Conflict with horizontal scroll inside a pane's own content.** Several
  pane content types plausibly need native horizontal scroll or drag:
  a wide terminal buffer, a diff/code view, or (per this codebase) the
  xterm.js terminal's own touch handling in
  `web-app/src/lib/hooks/useTerminalGestures.ts` — which already runs its
  own 5-state gesture machine (`IDLE → PENDING → SCROLLING | SELECTING | TAPPING`)
  directly on touch events inside a pane, explicitly because two independent
  touch handlers on the same element previously caused "double-scroll" bugs
  (see that file's header comment, "Replaces the conflicting `useTouchScroll`
  + `useMobileTerminalGestures` hooks"). A window-level swipe recognizer
  must not add a *second* competing touch handler inside pane content — it
  should only claim the gesture from the window tab-strip container itself
  (a fixed-height strip, not the pane content area), the same scoping this
  codebase already learned the hard way for terminal gestures. If the swipe
  needs to work from anywhere in the window (not just the tab strip), a
  distance/velocity/axis-lock threshold is needed to disambiguate "vertical
  scroll of pane content" from "horizontal window-swipe" before claiming the
  touch — copying the existing `PENDING` state's disambiguation logic in
  `useTerminalGestures.ts` rather than writing a new one from scratch.
- **Accessibility fallback is not optional.** A swipe gesture has no
  keyboard or switch-control equivalent by definition — users who navigate
  by keyboard, screen reader, or switch access cannot perform it at all. The
  requirements already provide the accessible path (tab-strip click,
  `Ctrl+<number>`-or-fallback) — the risk is implementation drift where the
  tab-strip buttons end up mouse/touch-only in practice (e.g. missing
  `role="tab"`/keyboard focus handling). `MobilePaneTabStrip.tsx` (the
  existing pane-level analog) already does this correctly — `role="tablist"`,
  `role="tab"`, `aria-selected`, real `<button>` elements — the window tab
  strip should copy that pattern exactly, not the swipe gesture's semantics.

## 4. Tab-strip UI at scale (many windows on a narrow viewport)

Read `web-app/src/components/pane/MobilePaneTabStrip.tsx` and its CSS
(`web-app/src/styles/pane/mobilePaneTabStrip.css.ts`) directly — this is the
existing analog the requirements point to ("following the same visual
language as the existing mobile stacked-pane tab row").

- **Current overflow handling: horizontal scroll only, no affordances.**
  `mobileTabStrip` is `display: flex; overflowX: "auto"` with the scrollbar
  hidden (`scrollbarWidth: "none"`, `::-webkit-scrollbar { display: none }`).
  There is **no**:
  - auto-scroll-into-view of the active tab when it's off-screen (no `ref`-based
    `scrollIntoView()` call anywhere in the component),
  - visual affordance that more tabs exist off-screen (no fade/gradient edge,
    no chevron/overflow-menu),
  - any cap on tab count or fallback to a dropdown/menu once tabs exceed the
    viewport width.
  This is fine at pane-tab scale (a handful of panes per window, and the
  component already early-returns `null` for ≤1 leaf) but the requirements
  explicitly expect "low double digits" windows — a scenario this exact
  pattern has never been exercised at. **10+ items in a hidden-scrollbar,
  no-indicator `overflow-x: auto` strip is a known discoverability trap**:
  users don't realize they can scroll (no scrollbar, no gradient hint), and
  there's no way to jump to window 11 except scrubbing through the strip by
  touch/trackpad drag one screen-width at a time. Recommend adding at least
  one of: an auto-scroll-into-view effect on active-window change (cheap,
  same complexity class as the existing component), and/or a "▾ show all
  windows" overflow affordance once tab count exceeds what fits — do not
  ship a straight copy of `MobilePaneTabStrip`'s overflow behavior unchanged
  and assume it scales.
- **Two tab strips stacked on one narrow viewport (window strip + pane
  strip) is a distinct risk already named in the requirements' Rabbit
  Holes** ("Mobile tab-strip density") — confirmed real by reading the CSS:
  `mobileTabStrip` is a fixed `height: "40px"` bar. Two of these stacked
  (window strip + pane strip, both bottom-anchored per the requirements' "bottom
  on mobile" placement) consume 80px of permanently fixed vertical chrome on
  a small phone viewport, on top of whatever the terminal/session header
  already reserves. This is a genuine budget problem, not just a visual
  one — worth measuring against real small-viewport heights (e.g. iPhone SE
  at 667px tall) during design, not assumed to "just fit."

## 5. React pitfalls: array of independent reducer-driven trees

The Feasibility Risks section already names the core architectural
boundary (wrapping `usePaneReducer` to manage an array of `PaneState`
instances + an active-window pointer). Specific failure modes to design
against:

- **Stale closures across the window array.** If the wrapping hook holds
  `windows: PaneState[]` and hands each window's pane-tree renderer a
  `dispatch` closure captured at render time (e.g. `(action) => dispatchForWindow(windowId, action)`),
  any memoization (`useMemo`/`useCallback`) that captures `windowId` or
  `windows` by value rather than through a ref will go stale the moment
  windows are reordered or closed — a classic "background window's dispatch
  still points at an index that now belongs to a different window" bug.
  Prefer keying dispatch by a stable window `id`, never by array index, and
  route all window-array mutations through a single reducer (`windows` as
  reducer state, not `useState` + manual splicing) so there's one
  authoritative update path instead of N independent `usePaneReducer`
  instances each racing to update a shared parent array.
- **`key` prop pitfalls on reorder/close.** If window tabs (or the window
  content areas themselves, if more than the active one stays mounted — see
  next point) are rendered with `key={index}` instead of `key={window.id}`,
  closing window 2 of 5 will cause React to reuse component instances for
  the wrong window (DOM/state gets reassigned to whatever window now
  occupies that index), which for a pane tree means a background window's
  *entire* xterm/session state could visually "jump" to a different window's
  identity mid-render. This is the single most important key-prop rule here:
  **window id, never array position, as the `key`.** (The existing
  `MobilePaneTabStrip.tsx:37` already does this correctly for panes —
  `key={l.id}`, not index — confirm the window-level equivalent follows the
  same rule; don't regress it.)
- **Memory leaks from unmounted-but-retained background window state.**
  The requirements require instant (<100ms) switch with the previous
  window's "exact layout... no rebuild required" — which argues for keeping
  every window's `PaneState` (and by extension every background window's
  session/terminal subscriptions) alive in memory even while not rendered,
  rather than unmounting and reconstructing on switch. That's the right call
  for the perceived-latency SLO, but it directly reintroduces the exact
  problem `04-pitfalls.md` §1/§2 already flag for panes: xterm.js terminal
  instances and their WebSocket subscriptions are expensive, and a
  `visibility: hidden` (not unmounted) background window keeps every one of
  its terminals' sockets and `ResizeObserver`s live and consuming resources
  indefinitely. With "low double digits" of windows possible, and each
  window potentially containing its own multi-pane tree, this could mean
  dozens of simultaneously-open WebSocket connections and xterm instances at
  once, most invisible. Recommend an explicit policy decision (not left
  implicit, matching the Rabbit Holes' own "easy to hand-wave, easy to get
  wrong" framing for last-window semantics): either (a) cap how many
  background windows keep live terminal subscriptions and degrade older
  ones to a frozen/reconnect-on-focus state, or (b) accept the resource cost
  as bounded by realistic window counts and document it, but do not leave
  it undecided — this is exactly the kind of thing that's invisible in a
  demo with 2 windows and only surfaces as a slow-leak complaint once real
  users accumulate 10+.

## Summary of top risks (ranked)

1. **`Ctrl+<number>` as specified is unsafe on every target platform** — OS-level
   (macOS Mission Control) and browser-chrome-level (Windows/Linux tab
   switching) collisions that page-level `preventDefault()` cannot reliably
   or ever block. Must change the binding before implementation, not after
   discovering it in QA.
2. **Cross-tab concurrent localStorage writes are a pre-existing,
   undocumented gap that multi-window makes materially worse** — no
   `storage` event listener exists today, and the requirements' own Baseline
   section already surfaces the scenario (two tabs, same origin, same key)
   without resolving it. Needs at minimum a detect-and-log guard before ship.
3. **Retained background-window state (for the <100ms switch SLO) has an
   unbounded resource-growth shape** — every open window's terminals/sockets
   staying live is the right latency trade-off but needs an explicit cap or
   degrade policy, or it becomes a slow memory/connection leak invisible
   until a user accumulates many windows over a long session.
