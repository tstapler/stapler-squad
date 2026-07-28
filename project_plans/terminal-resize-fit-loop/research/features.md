# Feature Research: Terminal Resize/Fit Feedback Loop Convergence

Agent 2 (Features). Scope: existing patterns to reuse, edge cases beyond the 7 ACs,
unstated user needs, and prior findings from `terminal-jank`/`terminal-robustness`.

## 1. Existing resize-convergence / dedup patterns in this codebase

**No existing shallow-equal / value-dedup utility exists.** Searched `web-app/src/lib/utils/`
(`datetime.ts`, `fileIcons.ts`, `frecency.ts`, `notificationGrouping.ts`, `notificationMapping.ts`,
`notificationStorage.ts`, `parseDiff.ts`, `retry.ts`, `rtkQueryError.ts`, `timestamp.ts`,
`truncateMiddle.ts`) and grepped for `shallowEqual`/`areEqual` across `web-app/src` — nothing.
**The (cols,rows)-equality check for AC3 and the integer-cols/rows-equality check for AC2 must be
written fresh** (small pure functions, e.g. `dimensionsEqual(a, b)`); there is nothing to import.

**Time-based debounce precedent**: `web-app/src/lib/hooks/useDebounce.ts` exports a generic
`useDebounce<T>(value, delay)` and `useDebouncedCallback`. These are React-state-based debouncers
(re-render on settle) and are *not* used by `XtermTerminal.tsx` today — the ResizeObserver handler
there hand-rolls its own `setTimeout`-based debounce (150ms flat delay, see below) plus a double
`requestAnimationFrame` after the timeout fires, specifically to survive iOS Safari reflow timing
(cites xterm.js issue #3895 in a comment at `XtermTerminal.tsx` ~487). Any new gating logic should
extend this existing hand-rolled timer, not introduce `useDebouncedCallback` — switching to a
React-render-cycle debouncer would add a re-render dependency the current imperative-refs design
avoids.

**RAF single-in-flight-token pattern** (relevant precedent for building the oscillation-burst
detector's bookkeeping): `web-app/src/lib/hooks/useTerminalMetrics.ts` (~line 99) uses
`pendingUpdateRef.current === null` as a guard before scheduling a new
`requestAnimationFrame(flushOutputBuffer)`, and cancels+reschedules on large buffers. This is the
established idiom in this codebase for "at most one pending scheduled unit of work" — the same
shape the resize debounce timer already uses (`resizeTimeout` local var + `clearTimeout` on
re-entry) and that the new oscillation counter should follow (ref-based rolling window, not React
state, to avoid re-renders on every resize tick).

**Other ResizeObserver usages** (for pattern comparison, not reuse — each solves a different
problem):
- `web-app/src/lib/hooks/useSplitContainerSize.ts` — bare ResizeObserver → `setState` on every
  `contentRect` change, no debounce, no dedup. Used only for pane-nudge-resize UI feedback
  (low-frequency, human-triggered), so the lack of gating is currently harmless there but is not a
  model to copy for the terminal fit loop.
- `web-app/src/components/layout/BottomNav.tsx`, `web-app/src/lib/contexts/NavigationContext.tsx`,
  `web-app/src/components/sessions/FileTree.tsx`, `web-app/src/components/pane/PaneSplitRenderer.tsx`
  (~line 330, publishes strip height) — all simple `ResizeObserver → setState`, no debounce/dedup
  logic worth reusing.
- `web-app/src/components/sessions/TerminalOutput.tsx` also has an `onResize`-adjacent listener:
  a `visualViewport.resize` handler (documented in `terminal-robustness/research/pitfalls.md`
  "Race 4", ~line 656) that calls `xtermRef.current?.fit()` directly, independent of the
  ResizeObserver path. **This is a second, uncoordinated fit() trigger that can race the
  ResizeObserver-driven one on iOS when the virtual keyboard opens/closes** — worth flagging to the
  architecture/plan agents since gating only the ResizeObserver path (as ACs 1-3 specify) leaves
  this second entry point ungated. Out of explicit scope per requirements, but the oscillation
  detector (AC4) should probably count fit() calls regardless of trigger source so this path also
  benefits.

**No RPC-dedup pattern exists elsewhere to model `useTerminalFlowControl.resize()`'s fix on.**
`resize()` (`useTerminalFlowControl.ts` ~403-455) currently tracks only `lastResizeTimeRef` (a
timestamp) — there is no `lastColsRef`/`lastRowsRef` anywhere in the hook. AC3 requires adding
that value-tracking state; grepped sibling hooks (`useSessionRepoPaths.ts`, `useSessionService.ts`,
`useSessionNotifications.ts`) for a comparable "skip RPC if value unchanged" idiom and found none —
this will be new code, not a refactor-to-reuse.

## 2. Edge cases beyond the 7 ACs

- **Zero-size guard already exists** at `XtermTerminal.tsx` ~503-505 (the
  `else if ((widthChanged || heightChanged) && (width === 0 || height === 0))` branch logs and
  no-ops). This guard runs *before* any new value-based gating would run, so it's compatible as-is;
  just confirm the new AC2 gate (integer cols/rows comparison) doesn't get evaluated on a
  zero-size entry (it currently can't, since fit() is never called in that branch — cols/rows
  can't be proposed from a 0×0 container in the first place per
  `terminal-jank/research/pitfalls.md` line 16-22: `FitAddon.proposeDimensions()` returns
  `undefined` on zero cell dimensions).
- **Rapid mount/unmount during resize**: the `resizeTimeout` var and `resizeObserver` are both
  scoped inside the mount effect and cleaned up in the returned cleanup function (clearTimeout +
  `resizeObserver.disconnect()`), so a pending debounce timer today is correctly cancelled on
  unmount. Any new oscillation-detector state (rolling window of recent (cols,rows) samples) must
  live in the same effect-scoped closure or a ref cleared in the same cleanup path, or it will leak
  a stale rolling-window array across remounts of the *same* component instance (React strict-mode
  double-invoke in dev, or a session pane genuinely remounting).
- **Multiple `XtermTerminal` instances resizing simultaneously — currently possible, but only in
  test/stress harnesses, not real navigation flows.** Grepped all `<XtermTerminal` render sites:
  it's rendered exactly once, inside `TerminalOutput.tsx`, which itself is rendered exactly once,
  inside `SessionDetailView.tsx` (single active session view). `PaneSplitRenderer.tsx` does **not**
  render `TerminalOutput`/`XtermTerminal` at all (confirms the requirements doc's "Out of Scope:
  building an in-page tiled/split-pane layout — doesn't exist in this codebase"). So the
  requirements' "3 terminal sessions/tabs" repro scenario refers to **3 separate browser tabs**
  each running their own `SessionDetailView`/`XtermTerminal` instance — not 3 terminals sharing one
  page's DOM/CPU budget as siblings. This matters for the oscillation-burst detector: it can be
  scoped per-component-instance (module-local ref inside the mount effect) with no cross-instance
  coordination needed, since instances never share a tab.
- **Font-size-change effect interaction (AC-adjacent, not in the 7 ACs but explicitly flagged in
  the task)**: `XtermTerminal.tsx` ~572 has `useEffect(() => { ...; setTimeout(() =>
  fitAddonRef.current?.fit(), 0); }, [fontSize])` — same pattern exists for `fontFamily` (~580).
  This calls `fit()` directly, **bypassing the ResizeObserver handler entirely**, so it is not
  gated by AC2's integer-cols/rows check. However, `fit()` changing the terminal's cell metrics
  *will* change container-relative sizing and can itself trigger a subsequent ResizeObserver
  firing (container reflow from font metric change) — meaning a font-size change could seed the
  very sub-cell WebGL wobble the oscillation detector exists to catch (AC4's premise: "Actual
  pixels per column: 8.45px vs Expected: 8.33px" mismatch is a *glyph metric* discrepancy, and
  font-size changes are exactly when glyph metrics are recomputed). Recommend the plan explicitly
  decide whether the font-size-change `fit()` call should also feed the oscillation-burst counter
  (same counter, two entry points) rather than only counting ResizeObserver-triggered fits —
  otherwise a user cycling font size could trigger the WebGL wobble without ever tripping the
  fallback.
- **`terminal.onResize` firing without a *new* proposed dimension**: xterm.js's own `onResize`
  event fires whenever `terminal.resize(cols, rows)` is called, even if called with the same
  values it already has (no internal no-op short-circuit is guaranteed across xterm.js versions).
  AC3 gates `useTerminalFlowControl.resize()` on "equals last value SENT" — note this is
  deliberately *not* "equals current terminal cols/rows", since the flow-control hook doesn't have
  direct access to the terminal instance; it only sees what `XtermTerminal`'s `resizeDisposable`
  (the `terminal.onResize` subscription, disposed in the same cleanup block as the
  ResizeObserver) forwards to it as call arguments. Double-gating exists by design (AC2 gates
  fit()-triggering-onResize at the container-measurement layer; AC3 gates the RPC at the
  value-comparison layer) — both are needed because `onResize` can theoretically fire from causes
  other than the gated ResizeObserver path (e.g. the font-size effect's direct `fit()` call above).

## 3. Unstated user needs

- **Sticky vs. resettable WebGL fallback**: the requirements don't say whether the canvas
  fallback (AC4) should persist for the lifetime of the tab/session or reset on remount. Given the
  existing `webglAddon.onContextLoss` handler (`XtermTerminal.tsx` ~264-281) already disposes the
  WebGL addon *without* re-creating it on the next mount (WebGL is only attempted once, in the
  mount effect's IIFE) — the codebase's existing precedent is "WebGL loss is not recovered within
  an instance's lifetime, and a fresh mount gets a fresh WebGL attempt." Recommend the new
  oscillation-triggered fallback follow the same precedent (per-mount, not persisted to
  localStorage/cache) for consistency, unless the user has been burned by the same
  machine/browser repeatedly hitting the wobble across sessions — in which case a
  `TerminalDimensionCache`-style per-session localStorage flag (that cache already stores
  `cellWidth`/`cellHeight`/`fontSize`/`fontFamily` per session, see
  `web-app/src/lib/terminal/TerminalDimensionCache.ts`) would be the natural place to also persist
  "this session's terminal previously hit WebGL oscillation, skip WebGL on reconnect" — worth
  raising as an open question for the architecture/plan phase rather than assuming either default.
- **Observability**: AC4 explicitly requires "a console.error backstop when no WebGL addon exists
  to dispose," which the requirements author clearly intends as a debugging signal, not user-facing
  UI. No structured telemetry/analytics pipeline for frontend perf events was found in
  `web-app/src/lib/` (searched `notificationGrouping.ts`/`notifications.ts` — these are
  user-facing toast notifications, not telemetry). Given this project's `.claude/docs/benchmarks.md`
  and OpenTelemetry integration (`.claude/docs/opentelemetry.md`) are backend-focused, the simplest
  consistent choice is a `console.warn`/`console.error` (matching the existing
  `[XtermTerminal]`-prefixed logging convention already used pervasively in this file) rather than
  inventing a new frontend telemetry channel — this is a Go-server-side-out-of-scope item per the
  requirements anyway ("Server-side (Go) resize RPC handling changes" is out of scope).
- **ADR requirement (AC4)**: the requirements mandate "Decision documented in an ADR." Existing
  ADR precedent in this problem space: `project_plans/terminal-jank/decisions/ADR-001-terminal-instance-pool.md`,
  `ADR-002-xterm-upgrade-6.0.md`, `ADR-003-cold-start-quiescence.md`, and
  `project_plans/terminal-robustness/decisions/ADR-012` through `ADR-014`. The new ADR for this
  project should follow the same numbering-within-project convention
  (`project_plans/terminal-resize-fit-loop/decisions/ADR-001-...md`) established by those two
  sibling projects.

## 4. Prior findings from `terminal-jank` and `terminal-robustness` (cited, not re-derived)

These two prior SDD projects already did deep research on this exact ResizeObserver/FitAddon/WebGL
surface. Their findings are directly load-bearing for this project's design and should be treated
as established fact, not re-researched:

- **`project_plans/terminal-jank/research/pitfalls.md` (Pitfall 1, ~line 16-36)**:
  `FitAddon.proposeDimensions()` returns `undefined` when the container is hidden/zero-size (0×0
  reports NaN internally). The current zero-size guard in `XtermTerminal.tsx` (~503) is the
  correct existing mitigation; confirms AC2's "sub-cell pixel deltas are a no-op" design should sit
  *after* this existing zero-size check, not replace it.
- **`project_plans/terminal-jank/research/pitfalls.md` (Pitfall 2, ~line 41-90)**: WebGL context
  limit is a *separate* failure mode from the sub-cell wobble this project targets — browsers cap
  ~16 WebGL contexts/page (as low as 8 on integrated graphics); `onContextLoss` fires ~3s after
  `webglcontextrestored` fails, with **no automatic canvas fallback** built into xterm.js. The
  current code (`XtermTerminal.tsx` ~264-281) already subscribes to `onContextLoss` and disposes
  the addon on loss — this is the *existing* partial mitigation for the context-limit problem, and
  is architecturally the same code path AC4 wants to reuse/extend for the oscillation-triggered
  fallback (i.e., AC4's fallback is "trigger the same dispose-on-loss logic, but from an
  oscillation-count trip-wire instead of only from the real `onContextLoss` event"). Note: the
  current `onContextLoss` handler holds `webglAddon` only in the IIFE closure — there is **no
  `webglAddonRef`** stored on the component for later access. AC4's oscillation detector runs from
  the *ResizeObserver* handler, a different closure, and will need its own ref to the WebGL addon
  instance to dispose it — this ref does not exist yet and must be added.
- **`project_plans/terminal-jank/research/pitfalls.md` (Race 3, ~line 146-159)**: flags that a
  hidden/background terminal's ResizeObserver can still fire and send a resize RPC "which may not
  be the active session" — recommends gating backend resize calls on an "is this session active"
  predicate. Not one of the 7 ACs here, but directly adjacent; worth a note-for-later since this
  project's AC3 value-dedup (skip RPC if cols/rows unchanged) would incidentally suppress *some*
  but not all of that spurious-background-resize traffic (it only dedups by value, not by
  active/inactive session state).
- **`project_plans/terminal-robustness/research/pitfalls.md` (Races 1-2, ~line 75-90)**: **this is
  the direct predecessor bug report for the current 150ms flat debounce.** It documents that an
  older "double `fitAddon.fit()` on mount" plus an "adaptive 10ms/250ms debounce" caused two
  resize RPCs within 100ms and a too-early fire that raced tmux quiescence. The current 150ms flat
  debounce + double-rAF (visible in `XtermTerminal.tsx` comments ~476-483) is the fix that shipped
  from that project. This confirms the debounce *timing* itself is not the current bug — the
  problem being solved by *this* project (terminal-resize-fit-loop) is orthogonal: it's about
  *value*-based gating (same cols/rows recomputed repeatedly) rather than *timing* gating (fit()
  firing too soon). Do not re-tune the 150ms constant as part of this fix.
- **`project_plans/terminal-robustness/research/pitfalls.md` (Race 4, ~line 99-105)**: confirms
  the `visualViewport.resize` → `fit()` path in `TerminalOutput.tsx` (~line 656) as a second,
  ResizeObserver-independent fit() trigger on iOS keyboard show/hide — same finding as section 2
  above, cited here since terminal-robustness already flagged it as a known race with the primary
  ResizeObserver path.
- **`project_plans/terminal-robustness/research/features.md` (~line 73-87)**: industry precedent
  (VS Code, Kilo-Org cloud terminal, ttyd) for debouncing ResizeObserver→fit() at 150-200ms and
  sending "a single, final resize rather than dozens of intermediate ones" — this is exactly what
  the current 150ms debounce already implements; the gap this project closes is the *value*-based
  case (settled timing, but same value repeating) that none of that industry precedent explicitly
  addresses either (their examples assume genuinely-changing dimensions during a CSS transition,
  not a stable sub-cell wobble).
- No prior project (`terminal-jank`, `terminal-robustness`, or any other `project_plans/*`) has
  researched or implemented an oscillation-burst counter / rolling-window trip-wire pattern — AC4
  is genuinely new design surface, not a rediscovery of prior work.

## Key files referenced

- `web-app/src/components/sessions/XtermTerminal.tsx` (~440-510 ResizeObserver/fit gating,
  ~255-281 WebGL load + onContextLoss, ~566-582 font-size/font-family effects)
- `web-app/src/lib/hooks/useTerminalFlowControl.ts` (~403-455 `resize()`)
- `web-app/src/lib/hooks/useDebounce.ts` (existing generic debounce hooks, not currently used by
  the terminal resize path)
- `web-app/src/lib/hooks/useTerminalMetrics.ts` (~99 RAF single-in-flight-token precedent)
- `web-app/src/lib/terminal/TerminalDimensionCache.ts` (per-session localStorage cache; natural
  home if a sticky WebGL-fallback flag is wanted)
- `web-app/src/lib/hooks/useSplitContainerSize.ts`, `web-app/src/components/pane/PaneSplitRenderer.tsx`
  (other ResizeObserver usages, none with dedup logic worth reusing)
- `web-app/src/components/sessions/TerminalOutput.tsx` (~656 `visualViewport.resize` → `fit()`,
  second uncoordinated fit() trigger)
- `project_plans/terminal-jank/research/pitfalls.md`, `project_plans/terminal-jank/decisions/`
- `project_plans/terminal-robustness/research/pitfalls.md`, `research/features.md`,
  `project_plans/terminal-robustness/decisions/`
