# Research: Pitfalls for the visibility/focus resync fix

Scope: what commonly goes wrong when adding a debounced `visibilitychange`/`focus`
resync handler + stall watchdog to an xterm.js terminal stream, and what this
implementation must be explicitly designed against. Grounded in web-platform
behavior, React 18 semantics, and codebase-specific patterns found in
`useTerminalStream.ts`, `useTerminalFlowControl.ts`, `useDebounce.ts`, and
`XtermTerminal.tsx`.

## 1. Background-tab timer throttling — is the debounce itself a hazard?

Chrome and Firefox both throttle timers in *hidden* tabs:
- After ~5s hidden, background tabs are limited to firing timers roughly once
  per second; after ~5 min hidden, Chrome moves eligible tabs into "intensive
  throttling" (~1 wake/minute) unless the page holds an exemption (audible
  audio, WebRTC, a Page Lifecycle "frozen" opt-out, etc.).
- `requestAnimationFrame` callbacks simply do not run at all while the
  document is hidden (rAF is tied to paint, which is suspended) — not
  relevant here since the fix uses `setTimeout`/`document.visibilitychange`,
  not rAF, but worth naming to preempt an obvious alternative-design
  suggestion.

**Does this affect the new 300ms debounce timer?** Practically no, and this is
worth stating explicitly in the design so nobody "fixes" it later: the timer
in scope here is *armed by* the `visibilitychange`→`'visible'` (or `focus`)
event, which by definition only fires once the tab is already foregrounded
again. There is no window in this design where a `setTimeout` is armed while
hidden and expected to fire while still hidden — throttling of hidden-tab
timers is a non-issue for *this* debounce. The place throttling *does* matter
is upstream of this fix and already named in requirements.md: the WebSocket
stream itself coalesces/drops control-mode deltas while backgrounded, which
is the root cause the resync compensates for, not something the debounce
needs to work around.

**What to design against instead:** the *existing* `NEXT_PUBLIC_RECONNECT_V2`
listener in `useTerminalStream.ts:420-446` reuses `terminalDebounceTimerRef`
with its own 200ms delay and its own `handleVisibilityOrOnline`. The new
handler must not share that ref or collide with that listener's timer
lifecycle — use a distinct ref name (e.g. `resyncDebounceTimerRef`) so the two
listeners' independent debounce windows can't clobber each other if both are
active in the same tick (both fire off the same underlying `visibilitychange`
event). Confirm this at review time — grep for `terminalDebounceTimerRef` uses
before adding the new one.

**Also verify:** Chrome's back/forward cache (bfcache) and some OS-level tab
discarding can cause `visibilitychange` to fire on restore *without* a prior
`'hidden'` transition being observed by the still-alive JS context (page was
frozen, not just hidden) — the handler must not assume it saw the
`'hidden'` transition; it should react purely to the `document.visibilityState
=== 'visible'` check on each event, matching the existing V2 listener's
pattern at line 425 (`if (document.visibilityState !== "visible" && ev.type
!== "online") return;`).

## 2. React 18 StrictMode double-invocation

Confirmed: `web-app/next.config.ts:15` sets `reactStrictMode: true`. In dev,
StrictMode intentionally double-invokes effects (mount → cleanup → mount) to
surface missing cleanup. Concretely:

- If the new `useEffect` registers `document.addEventListener('visibilitychange',
  handler)` / `window.addEventListener('focus', handler)` **without** returning
  a cleanup function that calls the matching `removeEventListener`, StrictMode
  dev mode will double-register the listener, and every real
  visibility/focus event will fire the handler twice — which, given the
  known `useDebouncedCallback` bug (stale `timeoutId` from `useState`), is
  exactly the double-fire scenario AC1/AC2 are testing against. Get the
  cleanup wrong and the bug reproduces even after the debounce fix, just
  from listener duplication instead of timer staleness.
- The existing V2 listener (lines 420-446) already gets this right — it
  returns a cleanup that calls both `removeEventListener`s and clears
  `terminalDebounceTimerRef`. Mirror that shape exactly for the new handler.
- This only matters for **dev-mode StrictMode and RTL's `act()`-wrapped
  render in tests that don't disable it** — production builds do not
  double-invoke. But `web-app/jest.config` / RTL default rendering does not
  automatically apply React's dev double-invoke behavior the way the browser
  dev server does (RTL uses whatever React build is resolved, typically dev
  build with `NODE_ENV=test`), so **the double-registration risk is real in
  the Jest test suite too**, not just manual browser dev testing. Test setup
  must assert the listener count is exactly 1 by checking effect symmetry
  (mount/unmount pairs), not just "did the handler eventually work."

## 3. Race conditions between the watchdog's disconnect/connect and other reconnect paths

Three independent reconnect-triggering mechanisms will coexist:
1. The existing exponential-backoff auto-reconnect in `useTerminalStream.ts`
   (`terminalBackoffRef`, driven by stream error/close events).
2. The existing `NEXT_PUBLIC_RECONNECT_V2` visibility/online listener
   (lines 420-446) — reconnects only when `!isConnectedRef.current`.
3. The new stall watchdog's `disconnect().then(() => connect())` (per
   requirements.md, fires when a resync doesn't complete within 4000ms).

**Key existing guard to lean on:** `disconnect()` (line 359) already checks
`isDisconnectingRef.current || isResyncingRef.current` and self-reschedules
via `setTimeout(() => disconnect(), 500)` if a resync is in flight — i.e.
there is already a serialization point. The new watchdog's `disconnect()`
call goes through this same guarded function, so it inherits the
retry-if-busy behavior for free *if and only if* it calls the same
`disconnect` reference (via the hook's returned callback, not a bespoke
reimplementation).

**What can still go wrong:**
- **Connect-storm**: if the watchdog's `connect()` (post-disconnect) races
  the V2 listener's `connect()` (fired independently off the same
  `visibilitychange` event, since `!isConnectedRef.current` briefly holds
  true during the watchdog's own disconnect step), both could call
  `connectRef.current()` back-to-back. `connect()` needs the same kind of
  in-flight guard `disconnect()` has (check something equivalent to
  `isConnectedRef.current || isDisconnectingRef.current` before opening a
  new stream) — verify this guard exists in the current `connect()`
  implementation (line ~155: `if (isConnectedRef.current || !sessionId)
  return;` — this only guards against *already connected*, not against a
  *connect already in flight*; a second overlapping call before the first
  resolves is not obviously blocked). This is the single highest-value
  thing to verify/test before shipping: fire the watchdog and the V2
  listener's disconnected-path in the same tick and assert only one
  `streamTerminal()` call happens.
- **Ordering dependency**: `disconnect().then(() => connect())` assumes
  `disconnect()`'s promise doesn't resolve until any pending
  resync-in-progress retry loop (the 500ms self-reschedule) has actually
  finished. Confirm `disconnect()`'s returned promise chains through the
  `setTimeout(() => disconnect(), 500)` branch (i.e. `return
  disconnect()` inside that branch, not a fire-and-forget) — if it doesn't,
  `connect()` could run while the old stream is still torn down mid-flight,
  reopening a socket the abort/cleanup hasn't fully unwound yet.
- **Debounce vs. watchdog timer interaction**: the 300ms UI debounce and the
  4000ms stall watchdog are on different clocks with different purposes;
  don't let them share a ref/variable (same class of bug as issue #1 above —
  keep watchdog timer state, e.g. `resyncStallTimerRef`, fully separate from
  `resyncDebounceTimerRef`).

## 4. Stale closures / ref-mirroring pattern

The codebase already has an established, working pattern to copy exactly —
do not invent a new one:
- `useTerminalStream.ts:98-119` — `isConnectedRef` mirrored from `isConnected`
  state via a dedicated `useEffect([isConnected])`, comment: *"sync ref before
  state setter to prevent reconnect guard race"* (line 313). This is the
  template for reading `isConnected` inside a listener registered with an
  empty dependency array.
- `useTerminalStream.ts:104` — `connectRef` holds the latest `connect`
  identity so a long-lived listener (registered once, line 446 empty deps)
  always calls the current `connect`, not a captured stale one from mount
  time. The new handler needs the equivalent for `connect`, `disconnect`,
  and `requestFullResync` — none of these should be pulled directly into the
  new `useEffect`'s closure if the effect has `[]` deps; mirror each into a
  ref the same way, or add them to the effect's dependency array as stable
  `useCallback`s (the codebase leans toward the ref-mirror approach given
  `connectRef`'s existing precedent).
- `XtermTerminal.tsx:121-126` — `onDataRef`/`onResizeRef` mirrored from props
  via a small `useEffect` each render, read as `.current` inside handlers
  registered once. Same pattern, prop-sourced instead of hook-return-sourced.
- **Specific trap in this fix**: `requestFullResync`, `markResyncComplete`,
  and `markPaneResponseReceived` are being newly exposed from
  `useTerminalStream`'s return value per requirements.md item 3. Their
  identities will change across renders unless memoized inside
  `useTerminalFlowControl`/`useTerminalStream` — check whether they're
  wrapped in `useCallback` at the source before assuming a ref-mirror in the
  consumer effect is even necessary; if they're already stable, no ref is
  needed for *those specific* functions, only for the ones proven unstable
  (e.g. anything closing over the current `sessionId` or component state).

## 5. Testing pitfalls: JSDOM + fake timers + visibilitychange

The codebase already has working precedent for this exact simulation —
`web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts:678-731` and
`TerminalOutput.reconnect.test.tsx:131-146`. Follow that pattern:

```js
jest.useFakeTimers();
Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
document.dispatchEvent(new Event('visibilitychange'));
```

Known gotchas to watch for, several already implicitly solved by the existing
tests but worth stating for whoever writes the new ones:
- `document.visibilityState` is a **read-only getter** on the real DOM
  `Document` prototype — plain assignment (`document.visibilityState =
  'visible'`) silently no-ops in some environments; `Object.defineProperty`
  with `configurable: true` (so it can be redefined again in a later test /
  reset in `afterEach`) is required, matching the existing test file's usage.
- Must restore/reset the defined property between tests (or redefine it each
  time) — a `configurable: true` property from a prior test can leak into
  the next test file if not reset, causing a `visibilityState` of `'visible'`
  to persist when a later test expects `'hidden'`.
- `jest.useFakeTimers()` freezes `setTimeout`/`Date.now()`; the 300ms debounce
  and 4000ms watchdog both need explicit `jest.advanceTimersByTime(...)` (or
  `jest.runOnlyPendingTimers()`) calls in the test to actually fire — a test
  that dispatches the event and immediately asserts will see nothing happen
  and could give a false negative that reads as "handler never fires"
  rather than "timer never advanced."
- `act()` wrapping: state updates that happen inside the debounced callback
  (e.g. `setShowReconnectButton(true)`) must be wrapped in
  `act(() => { jest.advanceTimersByTime(300); })` or RTL will warn/fail on
  "update not wrapped in act" — this is easy to miss because the timer
  advance itself isn't the state update, the callback *inside* it is.
- `new Event('visibilitychange')` vs a `CustomEvent` — plain `Event` is
  correct and matches spec/existing test usage; no need for `CustomEvent`
  since no event-specific payload is read (handler branches purely on
  `document.visibilityState`).
- Testing "both events in the same tick" (AC1) requires dispatching both
  `visibilitychange` and `focus` synchronously before any
  `advanceTimersByTime` call, then advancing once — if the test advances
  timers between the two dispatches, it isn't actually testing the same-tick
  coalescing behavior the debounce fix is meant to guarantee.
- Cross-test pollution: because `document` is a shared JSDOM instance across
  tests in the same file (not reset per-test unless the test config resets
  modules), a leftover listener from a test that didn't unmount the
  component/hook can fire in a later, unrelated test and cause flaky
  failures far from the actual bug — always unmount/cleanup in
  `afterEach`.

## 6. Focus-stealing pitfalls to avoid

Requirements.md explicitly states: *"Never moves keyboard focus"* (in-scope
item 1) and AC3 requires an automated test that focuses a sibling input and
asserts `document.activeElement` is unchanged across the resync. Concrete
DOM/React APIs in this codebase that could accidentally do this if touched
carelessly:

- **`XtermTerminal.tsx:891`** — the imperative handle exposes a `focus()`
  method that calls `terminalRef.current?.focus()` (xterm.js's own
  `Terminal.focus()`, which moves focus to xterm's hidden textarea). The
  resync path must not call this. It's easy to reach for by accident because
  a "full resync" mentally maps to "re-fit and refocus the terminal," but
  that's exactly the behavior explicitly excluded.
- **`fitAddon.fit()`** (called on mount, and via the `ResizeObserver` path,
  and inside `handleFontSize`/`handleFontFamily` effects at lines 851/860) —
  `fit()` itself doesn't move focus, but it's the mechanism the *existing*
  resize-triggered resync uses, and per "Out of scope," this exact path
  (`XtermTerminal.tsx`'s mount-fit, ResizeObserver-fit) must be left
  byte-for-byte unmodified — don't route the new resync through `fit()` at
  all; it goes through `requestFullResync()`'s RPC path instead, which is
  server round-trip + `write()`, not a local DOM refit.
- **`tabIndex` on the container div / any conditional re-render that changes
  `containerRef`'s attributes** — nothing in the current code suggests the
  new handler needs to touch container props, but a natural implementation
  mistake would be to gate a "reconnecting" UI state (e.g.
  `showReconnectButton`) with a conditional render that unmounts/remounts
  the `XtermTerminal` subtree (different key, or wrapped in `{condition &&
  <XtermTerminal />}` instead of a persistent sibling banner) — remounting
  it would tear down and recreate xterm's hidden textarea, which can steal
  focus back to whatever the new instance's initial focus state is.
  `showReconnectButton(true)` should only toggle a banner/button rendered
  *alongside* the terminal, not the terminal's own mount key.
- **`terminal.write()`/`writeln()`** (used to deliver the resync's
  `clearAndHome + content` payload) does not move focus by itself in
  xterm.js — safe to call regardless of current focus target — but
  confirm no code path also calls `.focus()` immediately after `.write()`
  as some "helpful" convenience (grep `XtermTerminal.tsx`/
  `TerminalStreamManager` for any `.focus()` call sequenced near output
  writes before this change ships).
- **Test coverage for this**: AC3's "focus a sibling input, trigger resync,
  assert `document.activeElement` unchanged" is the right shape — extend it
  to also cover the *watchdog's* `disconnect()`/`connect()` path (item 5
  above), since a full stream teardown/rebuild is a more invasive operation
  than a simple resync and is a plausible place for a "helpfully" added
  refocus-on-reconnect to sneak in during implementation.

## Summary of concrete guardrails to design/test against

1. Use a distinct timer ref name for the new debounce (not
   `terminalDebounceTimerRef`, which the existing V2 listener already owns).
2. New `useEffect` must return a cleanup removing both listeners — verify no
   double-registration under StrictMode (dev) and in Jest/RTL.
3. Route both `disconnect()` and `connect()` calls through the hook's
   existing guarded implementations, not new bespoke calls; specifically
   verify `connect()` guards against a second call while one is already in
   flight (not just "already connected") before relying on it to prevent a
   connect-storm with the V2 listener.
4. Ref-mirror any non-memoized value (`isConnected`, and any of
   `connect`/`disconnect`/`requestFullResync`/etc. found to be unstable
   across renders) using the existing `isConnectedRef`/`connectRef`/
   `onDataRef` pattern — don't close over hook-returned values directly in
   an effect with narrow deps.
5. Tests: `Object.defineProperty(document, 'visibilityState', {value, configurable: true})`
   + `jest.useFakeTimers()` + explicit `advanceTimersByTime` + `act()`
   wrapping, following the existing precedent in
   `useTerminalStream.test.ts:678-731`; reset the property between tests.
6. Never call `XtermTerminalHandle.focus()` / `terminal.focus()` from the
   resync or watchdog path; never remount `XtermTerminal` (change its key or
   conditionally unmount it) to show reconnect UI — use a sibling banner.
