# Implementation Plan: Logs UX — Datadog/Splunk-Like Experience

**Date**: 2026-05-14
**Branch**: `stapler-squad-logs-mobile`
**Requirements**: `project_plans/logs-datadog-ux/requirements.md`

---

## Technology Decisions (Synthesis)

| Decision | Choice | Rationale |
|---|---|---|
| Virtual scroll library | `react-virtuoso` | `followOutput` handles live-tail anchoring natively; ResizeObserver-based height measurement handles expandable rows; `overscan` mitigates iOS rubber-band; 32 KB gzip is justified by eliminating ~150 lines of hand-rolled scroll management |
| Sticky gutter on iOS | Split-column layout (fixed div + scrollable div with JS scroll-sync) | `position: sticky` inside `overflow-x: auto` is broken on iOS Safari ≤ 16 (P-04); split-column is the pattern used by VS Code, Google Sheets, and production log viewers |
| Search highlight | Native `String.includes()` filter + regex-split `<mark>` wrapping | Zero-dependency; filtering 10k lines takes < 5ms; highlight only runs on the ~30-50 visible virtualized rows |
| ANSI safety | Strip OSC sequences (regex pre-pass) → `ansi-to-html { escapeXML: true }` → DOMPurify post-pass | Eliminates OSC 8 `javascript:` href XSS (P-06); matches existing `useTerminalSnapshot.ts` pattern |
| State management | `useLogViewer` custom hook with `useState` / `useReducer` local to `LogViewer` | Logs are ephemeral, not shared; existing `page.tsx` and `SessionLogsTab` already use local state |
| Live tail | Existing `useLiveTail` polling hook (2s interval) wrapped inside `useLogViewer` | NFR-3 (no new endpoints preferred); polling every 2-3s is imperceptible; streaming RPC deferred |
| Log appending | Mutable ref + version counter (`useRef` for array, `useState` for version) | Avoids O(n) `[...prev, ...new]` spread on every live-tail tick (P-02) |
| CSS | vanilla-extract `.css.ts` colocated with components; tokens from `theme.css.ts` | Required by ADR-009 / CLAUDE.md rules |
| Backend change | Add `repeated string levels` to `GetLogsRequest` proto (replaces single `level` string) | Enables true multi-level filtering server-side; existing workaround silently misses entries beyond `limit` |

> **ADR required**: The choice of `react-virtuoso` over a hand-rolled virtual scroller (which the architecture agent recommended) is a meaningful dependency decision. An ADR should be filed at `docs/adr/010-react-virtuoso-log-viewer.md` before or during Epic 1 implementation.

---

## Epics

---

### Epic 1: Foundation — LogViewer Component Scaffold

**Goal**: Create the shared `LogViewer` component shell with proper file structure, vanilla-extract styles, and wire up both surfaces (`app/logs/page.tsx` and `SessionLogsTab.tsx`) to render it instead of their current `<table>` implementations. No new UX features yet — just structural replacement with equivalent behavior.

#### Stories

- **S1.1**: Create the `LogViewer` component directory structure with all new files as empty/stub exports so the TypeScript compiler passes from day one.
- **S1.2**: Wire `app/logs/page.tsx` to render `<LogViewer source="app" />` instead of the existing `<table>` block, keeping all existing query params and filter state threaded through.
- **S1.3**: Wire `SessionLogsTab.tsx` to render `<LogViewer source="session" sessionId={sessionId} />` instead of its current table, keeping the existing log fetch behavior.
- **S1.4**: File ADR-010 documenting the `react-virtuoso` dependency decision.

#### Tasks

- [ ] InstallReactVirtuoso — `web-app/package.json` — Run `npm install react-virtuoso` and confirm version in package-lock.json; verify bundle size impact against 5 MB limit in size-limit config
- [ ] CreateLogViewerStub — `web-app/src/components/logs/LogViewer.tsx` — Create component accepting `{ source: "app" | "session"; sessionId?: string }` props; initially renders a `<div>` placeholder; export from `components/logs/index.ts`
- [ ] CreateLogViewerStyles — `web-app/src/components/logs/LogViewer.css.ts` — Scaffold vanilla-extract file with container style using `vars` tokens from `web-app/src/styles/theme.css.ts`; define layout vars for toolbar height, gutter width
- [ ] CreateUseLogViewerHook — `web-app/src/lib/hooks/useLogViewer.ts` — Define `LogViewerState` interface and stub hook that calls existing `useLiveTail` and `getLogs` RPC; re-export existing live tail and fetch logic so no behavior changes yet
- [ ] CreateLogParserUtil — `web-app/src/lib/logs/logParser.ts` — Create module with stub functions: `detectLevel(line: string): LogLevel`, `highlightMatches(text: string, query: string): React.ReactNode[]`, `parseAnsi(raw: string): string` (OSC strip + ansi-to-html + DOMPurify pipeline); no implementation yet, just typed signatures
- [ ] CreateVirtualLogListStub — `web-app/src/components/logs/VirtualLogList.tsx` — Stub component wrapping `react-virtuoso` `<Virtuoso>`; accepts `data: LogEntry[]`, renders each item as a plain `<div>` with the log message; no styling
- [ ] CreateVirtualLogListStyles — `web-app/src/components/logs/VirtualLogList.css.ts` — Vanilla-extract styles for the scroll container; set `height: 100%`, `overflow: hidden`, `overscroll-behavior-y: contain`
- [ ] CreateLogRowStub — `web-app/src/components/logs/LogRow.tsx` — Stub component accepting `{ entry: LogEntry; index: number; isExpanded: boolean; onToggle: () => void }`; renders entry.message in a div; no styling
- [ ] CreateLogRowStyles — `web-app/src/components/logs/LogRow.css.ts` — Vanilla-extract styles file with placeholders for: row container, sticky gutter column, scrollable body column, level badge
- [ ] CreateExpandedLogDetailStub — `web-app/src/components/logs/ExpandedLogDetail.tsx` — Stub accordion panel that shows `entry.message` in a `<pre>`; styled with `ExpandedLogDetail.css.ts`
- [ ] CreateExpandedLogDetailStyles — `web-app/src/components/logs/ExpandedLogDetail.css.ts` — Vanilla-extract styles for the detail panel; background distinct from row background using `vars` tokens
- [ ] CreateJumpToLatestButtonStub — `web-app/src/components/logs/JumpToLatestButton.tsx` — Stub component with `{ newLineCount: number; onClick: () => void }` props; renders a fixed-position pill; styled with `JumpToLatestButton.css.ts`
- [ ] CreateJumpToLatestButtonStyles — `web-app/src/components/logs/JumpToLatestButton.css.ts` — Vanilla-extract fixed-position pill: `position: fixed`, `bottom`, `right`, `z-index`, safe-area inset for iOS home bar (`paddingBottom: 'env(safe-area-inset-bottom)'`)
- [ ] CreateLogViewerToolbarStub — `web-app/src/components/logs/LogViewerToolbar.tsx` — Stub toolbar component that renders existing `SearchWithHistory`, `LevelFilterChips` (new), `LiveTailToggle`, `ExportButton`; no logic yet
- [ ] CreateLevelFilterChipsStub — `web-app/src/components/logs/LevelFilterChips.tsx` — Stub chip row: `ALL | ERROR | WARN | INFO | DEBUG`; accepts `{ active: string[]; onChange: (levels: string[]) => void }`; styled with `LevelFilterChips.css.ts`
- [ ] CreateLevelFilterChipsStyles — `web-app/src/components/logs/LevelFilterChips.css.ts` — Vanilla-extract: horizontal flex row, `overflow-x: auto`, chip base style with `min-height: 44px`, level-specific color variants using `recipe()`
- [ ] UpdateIndexExports — `web-app/src/components/logs/index.ts` — Export all new components alongside existing ones
- [ ] ReplaceLogsPageTable — `web-app/src/app/logs/page.tsx` — Remove the `<table>` / `<tbody>` block (lines ~450-600); render `<LogViewer source="app" />` in its place; thread existing filter state (searchQuery, levelFilters, timeRange, liveTailEnabled) as props to LogViewer; keep page-level layout and header
- [ ] UpdateLogsPageStyles — `web-app/src/app/logs/page.css.ts` — Remove row/cell/thead styles that are now owned by LogViewer; keep page-level layout styles
- [ ] ReplaceSessionLogsTabTable — `web-app/src/components/sessions/SessionLogsTab.tsx` — Replace inner `<table>` / filter state with `<LogViewer source="session" sessionId={sessionId} />`; keep session-tab container and tab-panel wrapper
- [ ] UpdateSessionLogsTabStyles — `web-app/src/components/sessions/SessionLogsTab.css.ts` — Remove table/row styles now owned by LogViewer; keep tab-panel container styles
- [ ] FileADR010 — `docs/adr/010-react-virtuoso-log-viewer.md` — Document decision: react-virtuoso chosen over hand-rolled virtual scroller; trade-offs: 32 KB gzip vs. `followOutput` + dynamic height management; alternatives considered: @tanstack/virtual, custom CSS-only scroller

---

### Epic 2: Virtual Scroll + Live Tail

**Goal**: Replace the non-virtualized list rendering with `react-virtuoso`, implement live-tail follow mode with pause-on-scroll-up, and add the "Jump to Latest" pill with new-line count.

#### Stories

- **S2.1**: Implement `VirtualLogList` using `react-virtuoso` with `followOutput` for live tail auto-scroll; connect to `LogViewer`'s data array.
- **S2.2**: Implement the live-tail pause state machine: detect scroll-up, transition to "paused" mode, show "Jump to Latest" pill with queued-line count, resume on pill tap or scroll-to-bottom.
- **S2.3**: Fix the O(n) live-tail append performance issue using mutable ref + version counter pattern.

#### Tasks

- [ ] ImplementVirtualLogList — `web-app/src/components/logs/VirtualLogList.tsx` — Use `<Virtuoso data={entries} itemContent={renderItem} followOutput="smooth" overscan={200} />` where `followOutput` is enabled only when `isFollowing`; pass `atBottomStateChange` callback to detect scroll-up; wire `ref` for programmatic `scrollToIndex`
- [ ] ImplementFollowOutputProp — `web-app/src/components/logs/VirtualLogList.tsx` — Set `followOutput` to `"smooth"` when `isFollowing === true`, `false` when paused; expose `onAtBottomStateChange(atBottom: boolean)` prop so `LogViewer` can toggle pause state
- [ ] ImplementLiveTailPauseState — `web-app/src/lib/hooks/useLogViewer.ts` — Add `isFollowing: boolean` and `queuedNewLineCount: number` state; on `atBottom = false` transition: set `isFollowing = false`, increment counter; on "Jump to Latest" click: flush counter, set `isFollowing = true`; on `atBottom = true` transition: auto-resume `isFollowing = true` (clears counter)
- [ ] ImplementJumpToLatestButton — `web-app/src/components/logs/JumpToLatestButton.tsx` — Render pill with downward-chevron icon and `newLineCount` display; show only when `!isFollowing && newLineCount > 0`; `onClick` calls `onJumpToLatest()`; add `aria-label="Jump to latest log entry, {n} new lines"`
- [ ] ImplementMutableRefAppend — `web-app/src/lib/hooks/useLogViewer.ts` — Replace `setLogs(prev => [...prev, ...entries])` with: `logsRef.current = [...logsRef.current, ...entries]; setVersion(v => v + 1);`; child `VirtualLogList` reads from `logsRef.current` via `data` prop passed down (ref value at render time); wrap live-tail state updates in `startTransition()` (React 18) to keep updates interruptible
- [ ] ImplementOverscanForIOS — `web-app/src/components/logs/VirtualLogList.tsx` — Set `overscan={300}` (pixels) to buffer rows above and below the viewport; prevents blank flash during iOS momentum scroll; value chosen to cover ~8 rows at comfortable density
- [ ] ImplementOverscrollContain — `web-app/src/components/logs/VirtualLogList.css.ts` — Add `overscrollBehaviorY: 'contain'` and `overflowAnchor: 'none'` to scroll container style to prevent iOS rubber-band from triggering false "at bottom" detection (P-03, P-05)
- [ ] ConnectLiveTailToggle — `web-app/src/components/logs/LogViewerToolbar.tsx` — Wire existing `LiveTailToggle` component to `useLogViewer`'s `liveTailEnabled` and `setLiveTailEnabled`; when `liveTailEnabled` turns off, also set `isFollowing = false`
- [ ] WireAriaLiveLog — `web-app/src/components/logs/VirtualLogList.tsx` — Add `role="log"` and `aria-live="polite"` to the scroll container wrapper; add `aria-label="Log output"` on the container; throttle announcements: only update an `aria-live` text node once per 3 seconds during live tail

---

### Epic 3: Search, Filter, and Log-Level Coloring

**Goal**: Implement client-side search with inline `<mark>` highlighting and match count, log-level detection with WCAG AA color scheme applied to level badges and row tints, and the level filter chips (multi-select).

#### Stories

- **S3.1**: Implement `logParser.ts` utility functions for level detection, search highlight segmentation, and the ANSI processing pipeline.
- **S3.2**: Apply log-level coloring to `LogRow` (badge + row tint) using vanilla-extract `recipe()` variants.
- **S3.3**: Implement the search bar with real-time client-side filtering, match highlighting in visible rows, and a match counter display.
- **S3.4**: Implement level filter chips with multi-select logic and `ALL` chip toggling behavior.
- **S3.5**: Add keyboard shortcuts for search and navigation.

#### Tasks

- [ ] ImplementLevelDetection — `web-app/src/lib/logs/logParser.ts` — Export `detectLevel(line: string): "ERROR" | "WARN" | "INFO" | "DEBUG" | "TRACE" | "UNKNOWN"`; regex patterns: `/\b(ERROR|ERR)\b/i`, `/\bWARN(?:ING)?\b/i`, `/\bINFO\b/i`, `/\bDEBUG\b/i`, `/\bTRACE\b/i`; first match wins; precompile regexes as module-level constants; return value stored on `LogEntry` at fetch time in `useLogViewer`
- [ ] ImplementHighlightSegments — `web-app/src/lib/logs/logParser.ts` — Export `segmentText(text: string, query: string): Array<{ text: string; highlight: boolean }>`; use case-insensitive `String.prototype.indexOf` in a loop (not RegExp) for performance; empty query returns `[{ text, highlight: false }]`; consume `useDebounce` (existing) for query input
- [ ] ImplementAnsiPipeline — `web-app/src/lib/logs/logParser.ts` — Export `renderAnsi(raw: string): string`; pipeline: (1) strip OSC sequences via `/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g`, (2) `new AnsiToHtml({ escapeXML: true }).toHtml(stripped)`, (3) `DOMPurify.sanitize(html, { ALLOWED_TAGS: ['span', 'a'], ALLOWED_ATTR: ['style', 'href'] })`; cap ANSI spans: if output has > 50 `<span>` tags per line, re-run with plain text fallback; install `dompurify` if not present (check package.json first)
- [ ] ImplementLevelBadgeRecipe — `web-app/src/components/logs/LogRow.css.ts` — Define vanilla-extract `recipe()` for `levelBadge` with variants matching each log level; use WCAG AA color values from features.md §3: ERROR `#B91C1C`/`#DC2626`, WARN `#D97706`/`#F59E0B` (dark text `#1A1A1A` for WARN), INFO `#1D4ED8`/`#2563EB`, DEBUG/TRACE `#6B7280`/`#9CA3AF`; define `rowTint` style using `rgba` tints from features.md §3; use `vars` tokens where equivalents exist, raw values otherwise
- [ ] ImplementLogRow — `web-app/src/components/logs/LogRow.tsx` — Full row implementation: (1) narrow fixed gutter div with line number and level badge (sticky via split-column layout — see Epic 5 for the split-column; for now apply `position: sticky; left: 0` for desktop), (2) wide horizontally-scrollable div with `white-space: nowrap` containing timestamp + message; apply `levelBadge` recipe class by detected level; apply `rowTint` class; `onPointerDown` (not `onClick`) to handle iOS 300ms delay (P-04 note); `aria-expanded={isExpanded}` on the row element; `role="row"`; `data-testid={`log-row-${index}`}`
- [ ] ImplementSearchBar — `web-app/src/components/logs/LogViewerToolbar.tsx` — Replace stub search area with controlled input: `type="search"`, `inputMode="search"`, `autoCapitalize="none"`, `autoCorrect="off"`, `autoComplete="off"`, placeholder `"Search logs... (/ to focus)"`; match counter `"12 / 47"` right-aligned inside the input via CSS `padding-right`; clear (×) button with 44px tap area; mobile collapsed state toggled by `isSearchExpanded` flag; wire `value` and `onChange` to `useLogViewer`'s `searchQuery` state
- [ ] ImplementClientSideFilter — `web-app/src/lib/hooks/useLogViewer.ts` — Add `filteredLogs` derived value: filter `logsRef.current` by `searchQuery` using `String.prototype.includes` (case-insensitive via `.toLowerCase()`); filter by `levelFilters` array (skip if empty or `["ALL"]`); count matches; run in `useMemo` keyed on `[version, searchQuery, levelFilters]`; pass `filteredLogs` (not `logsRef.current`) to `VirtualLogList`
- [ ] ImplementMatchCount — `web-app/src/lib/hooks/useLogViewer.ts` — Expose `matchCount: number` (length of `filteredLogs` when `searchQuery` is non-empty) and `totalCount: number`; pass to `LogViewerToolbar` for display in search field
- [ ] ImplementHighlightInRow — `web-app/src/components/logs/LogRow.tsx` — Call `segmentText(entry.message, searchQuery)` to get segments; render as `<span>` array where highlighted segments are wrapped in `<mark>`; use `React.memo` on the segment rendering sub-component; do NOT use `dangerouslySetInnerHTML` for this (P-07)
- [ ] ImplementLevelFilterChips — `web-app/src/components/logs/LevelFilterChips.tsx` — Full implementation: `ALL` chip deselects all specific levels; clicking a specific level selects it (multi-select) and deselects `ALL`; chips use `role="group"` on wrapper, `aria-pressed` on each chip; active chip uses full background color per level from `levelBadge` recipe; `min-height: 44px` on all chips
- [ ] ImplementKeyboardShortcuts — `web-app/src/components/logs/LogViewer.tsx` — Add `useKeyboard` (existing hook) listener scoped to the log viewer container: `/` → focus search input (prevent default browser find); `ESC` → blur/clear search; `g` → scroll to top (`virtuosoRef.current?.scrollToIndex(0)`); `G` → scroll to bottom + resume follow; `j`/`ArrowDown` → scroll down one row; `k`/`ArrowUp` → scroll up one row; `Space` → page down; `b` → page up; `e` → next ERROR (`findNextIndex`); `E` → previous ERROR; `=` → toggle live tail; `?` → show shortcut help overlay; disable all shortcuts when focus is inside an input field
- [ ] ImplementShortcutHelpOverlay — `web-app/src/components/logs/ShortcutHelpOverlay.tsx` — Modal showing keyboard shortcut table from features.md §4; shown via `?` key or help button in toolbar; `aria-modal`, `role="dialog"`, ESC to close; styled with `ShortcutHelpOverlay.css.ts`
- [ ] AddCmdFInterception — `web-app/src/components/logs/LogViewer.tsx` — Listen for `Cmd+F` / `Ctrl+F` on the log viewer container; `event.preventDefault()` then focus search input; only when the log panel has focus (use `containsFocus` check via `document.activeElement`)

---

### Epic 4: Expandable Rows + JSON Detail

**Goal**: Implement the accordion row expansion pattern for log detail viewing, with JSON pretty-printing for structured log entries and copy-to-clipboard functionality.

#### Stories

- **S4.1**: Implement expansion state management in `useLogViewer` and connect it to `VirtualLogList` / `LogRow` so react-virtuoso can measure changed heights.
- **S4.2**: Implement `ExpandedLogDetail` with JSON pretty-print and copy buttons; handle the iOS pinch-to-zoom case.
- **S4.3**: Wire keyboard navigation for row expansion (`Enter` to expand/collapse, arrow keys to move selection).

#### Tasks

- [ ] ImplementExpansionState — `web-app/src/lib/hooks/useLogViewer.ts` — Add `expandedRowIndex: number | null` to state (not a Set — only one row expanded at a time per accordion behavior); expose `toggleRow(index: number): void` that sets `expandedRowIndex` to `index` if not already expanded, or `null` if it is; store this in a `useRef` that does NOT trigger re-renders of all rows — only the previously-expanded and newly-expanded indices need updating; use `react-virtuoso`'s item key mechanism for stable identity
- [ ] ImplementExpandedLogDetail — `web-app/src/components/logs/ExpandedLogDetail.tsx` — Full implementation: (1) `rawLine` section: `<pre>` with raw message, `user-select: text` (allow copy), copy-to-clipboard button using `navigator.clipboard.writeText`; (2) if `isJson(entry.message)`: parse JSON and render as expandable key-value tree using recursive component (no external library — use `JSON.stringify(val, null, 2)` in a `<pre>` block as initial approach, annotate with TODO for future syntax highlighting); (3) copy-per-field buttons for JSON mode; pinch-to-zoom: do NOT set `touch-action: none` — let browser handle zoom naturally on the `<pre>` content
- [ ] ImplementJsonDetection — `web-app/src/lib/logs/logParser.ts` — Export `tryParseJson(text: string): object | null`; trim whitespace, attempt `JSON.parse`, return parsed object or `null`; memoize per log entry ID in a `Map` (module-level cache, capped at 500 entries to prevent unbounded growth)
- [ ] ConnectExpansionToVirtuoso — `web-app/src/components/logs/VirtualLogList.tsx` — Pass `expandedRowIndex` to `itemContent` render function; when a row is expanded, render `<LogRow>` followed by `<ExpandedLogDetail>` as a fragment; `react-virtuoso` with ResizeObserver will automatically measure the new height and reposition rows below (P-08 mitigation — no manual cache invalidation needed)
- [ ] ImplementKeyboardRowNav — `web-app/src/components/logs/LogViewer.tsx` — Track `selectedRowIndex` state (separate from `expandedRowIndex`); `j`/`ArrowDown` increments, `k`/`ArrowUp` decrements, `Enter` calls `toggleRow(selectedRowIndex)`; `e`/`E` find next/previous index where `entry.level === "ERROR"`; call `virtuosoRef.current?.scrollToIndex({ index, behavior: 'smooth', align: 'start' })` after updating selected index
- [ ] AddRowAriaAttributes — `web-app/src/components/logs/LogRow.tsx` — Add `aria-expanded={isExpanded}`, `aria-selected={isSelected}`, `tabIndex={isSelected ? 0 : -1}`, `onKeyDown` for Enter/Space to trigger `onToggle`; each row gets `data-testid={`log-row-${index}`}` for Playwright

---

### Epic 5: Mobile UX Polish

**Goal**: Implement the iOS/Android-specific UX improvements: split-column layout for sticky gutter, swipe-to-reveal quick actions, search bar collapse/expand on narrow screens, proper safe-area insets, and touch target sizing.

#### Stories

- **S5.1**: Replace the CSS-only sticky gutter with the split-column layout (fixed left div + scrollable right div with synchronized scroll position) to fix the iOS Safari `position: sticky` bug (P-04).
- **S5.2**: Implement swipe-to-reveal quick actions on log rows (Copy, Share) using the existing `useSwipe` hook.
- **S5.3**: Implement the collapsible search bar for narrow screens (< 430px) and responsive layout adjustments.
- **S5.4**: Apply safe-area insets, touch-action directives, and visualViewport resize handling.

#### Tasks

- [ ] ImplementSplitColumnLayout — `web-app/src/components/logs/LogRow.tsx` + `web-app/src/components/logs/LogRow.css.ts` — Replace single-div row with: outer `position: relative` wrapper; inner `<div className={gutterCol}>` (fixed-width, `position: absolute; left: 0; top: 0; height: 100%; z-index: 10; overflow: hidden; background: var(--background)`) containing line number + level badge; inner `<div className={bodyCol}>` with `overflow-x: auto; white-space: nowrap; padding-left: {gutterWidth}px; touch-action: pan-x` containing timestamp + message; JS scroll sync between sibling rows is NOT needed because each row is independent — the gutter just covers the left edge of the scrollable body; this is the "overlay" variant of split-column
- [ ] ImplementGutterWidthToken — `web-app/src/components/logs/LogRow.css.ts` — Define `gutterWidth` as a CSS custom property on the LogViewer container (e.g., `--log-gutter-width: 88px`) so the body padding-left matches; expose a JS constant `GUTTER_WIDTH_PX = 88` for programmatic use; hide line number column below 380px via vanilla-extract `@media` query, adjusting `--log-gutter-width` to `44px` (level badge only)
- [ ] ImplementSwipeReveal — `web-app/src/components/logs/LogRow.tsx` — Use existing `useSwipe` hook from `web-app/src/lib/hooks/useSwipe.ts`; on left-swipe delta > 60px: slide the row body 88px left to reveal a 88px action strip with Copy (clipboard icon) and Share (share icon) buttons, each 44px wide; `touch-action: pan-y` on the outer row wrapper (allows vertical scroll, prevents browser back-nav from firing on right swipe); tap anywhere outside to close; `aria-label` on each action button
- [ ] ImplementMobileSearchCollapse — `web-app/src/components/logs/LogViewerToolbar.tsx` — Below 430px viewport width (detected via CSS `@media` in `.css.ts` and a `useMediaQuery` hook or `ResizeObserver`): render search bar as collapsed magnifying-glass icon button (44px × 44px); on tap: add class `searchExpanded` which slides the search input down as an absolutely-positioned overlay below the filter chips row; `autoFocus` on the input when expanded; "Done" button dismisses (does NOT clear search)
- [ ] ImplementSafeAreaInsets — `web-app/src/components/logs/JumpToLatestButton.css.ts` — Add `paddingBottom: 'env(safe-area-inset-bottom, 0px)'` to the fixed pill so it clears the iOS home indicator; similarly add bottom safe-area padding to the overall log viewer container footer area
- [ ] ImplementTouchActionDirectives — `web-app/src/components/logs/LogRow.css.ts` — `touch-action: manipulation` on all interactive row elements (level badge, line number) to eliminate iOS 300ms tap delay; `touch-action: pan-y` on the outer row wrapper; `touch-action: pan-x` on the inner scrollable body div
- [ ] ImplementVisualViewportHandler — `web-app/src/components/logs/LogViewer.tsx` — Add `window.visualViewport?.addEventListener('resize', onViewportResize)` (not `window.resize`) to recalculate scroll container height when iOS keyboard appears/disappears; debounce handler by 400ms (matching pattern in existing `TerminalOutput.tsx`); update a CSS custom property `--log-container-height` used by the scroll container instead of setting inline style
- [ ] ImplementTimestampAbbreviation — `web-app/src/components/logs/LogRow.tsx` — Show abbreviated timestamp `HH:mm:ss` on screens < 430px; full ISO timestamp on wider screens; detect via CSS `@media` applied to timestamp span class, using a responsive style in `LogRow.css.ts` (no JS media query needed — pure CSS)
- [ ] ImplementFilterChipsScroll — `web-app/src/components/logs/LevelFilterChips.css.ts` — Set `overflowX: 'auto'`, `-webkit-overflow-scrolling: touch` (for legacy iOS), `scrollbarWidth: 'none'` + `'::-webkit-scrollbar': { display: 'none' }` to create a swipeable chip row that hides its scrollbar; use `display: flex; flexWrap: 'nowrap'` so chips never wrap to a second line

---

### Epic 6: Wire Up Both Surfaces (App Logs + Session Logs)

**Goal**: Ensure `LogViewer` behaves correctly in both the full-page application logs view and the embedded session logs tab, handling differences in available controls (e.g., TimeRangePicker only for app logs) and data sources.

#### Stories

- **S6.1**: Implement `LogViewer` prop-driven conditional rendering so app-logs-only controls (TimeRangePicker, full toolbar) are shown for `source="app"` and hidden for `source="session"`.
- **S6.2**: Add multi-level filter support to the backend `GetLogs` RPC to replace the current client-side workaround.
- **S6.3**: Validate that existing page-level header, breadcrumbs, and layout in `app/logs/page.tsx` still render correctly with `LogViewer` as a child.
- **S6.4**: Validate that `SessionLogsTab` embedded in `SessionDetailView` still fits correctly in the tab panel layout.

#### Tasks

- [ ] ImplementSourceConditionalUI — `web-app/src/components/logs/LogViewer.tsx` — Render `<TimeRangePicker>` and `<ActiveFilterPills>` only when `source === "app"`; render a condensed toolbar for `source === "session"`; both share `<SearchBar>`, `<LevelFilterChips>`, `<LiveTailToggle>`
- [ ] AddMultiLevelFilterProto — `proto/session/v1/session.proto` — In `GetLogsRequest`: add `repeated string levels = N` (next available field number) alongside the existing `string level = ...`; both fields coexist for backward compatibility during transition; run `make generate-proto` after
- [ ] UpdateBackendMultiLevelFilter — `server/services/utility_service.go` — In `parseLogs()` (or wherever level filtering is applied): if `req.Levels` is non-empty, filter for any matching level (OR logic); fall back to `req.Level` (single) if `req.Levels` is empty for backward compatibility; update the `GetLogs` handler delegation in `server/services/session_service.go` if needed
- [ ] UpdateFrontendMultiLevelCall — `web-app/src/lib/hooks/useLogViewer.ts` — Pass `levels: levelFilters` (array) in the `getLogs` RPC call instead of single `level`; remove the client-side multi-level filter workaround that was missing entries beyond `limit`
- [ ] UpdateSessionDetailViewLayout — `web-app/src/components/sessions/SessionLogsTab.tsx` — Ensure the tab panel container still provides an explicit `height` (or `flex: 1; min-height: 0`) so react-virtuoso has a bounded scroll container; add `height: 100%` to the LogViewer wrapper inside the tab (P-10 mitigation)
- [ ] ValidateLogsPageLayout — `web-app/src/app/logs/page.tsx` — Ensure the page-level `<main>` container provides explicit height for the LogViewer (e.g., `height: calc(100vh - var(--header-height))`); confirm `overflow: hidden` on any ancestor doesn't clip the virtuoso scroll container; manually test with 1000 and 10000 mock entries
- [ ] UpdateUseLiveTailIntegration — `web-app/src/lib/hooks/useLogViewer.ts` — Reuse existing `useLiveTail` hook passing `sessionId` (or `undefined` for app logs); connect its `isLiveTailEnabled` / `interval` to the `LogViewer`'s `isFollowing` state; on each live-tail poll that appends new entries: if `!isFollowing`, increment `queuedNewLineCount`; if `isFollowing`, scroll to bottom via `virtuosoRef.current?.scrollToIndex({ index: 'LAST', behavior: 'auto' })`

---

### Epic 7: Tests + Registry Update

**Goal**: Write unit tests (Jest/RTL) covering the security-critical ANSI pipeline and the core state machine logic, write Playwright e2e tests for the key user journeys, and update the feature registry.

#### Stories

- **S7.1**: Write Jest unit tests for `logParser.ts` ANSI security pipeline, highlight segmentation, level detection, and scroll anchor state transitions.
- **S7.2**: Write Jest/RTL unit tests for `useLogViewer` hook state machine (live-tail pause/resume, filter logic, expansion state).
- **S7.3**: Write Playwright e2e tests for the key user journeys: live tail, search, filter, expand, and mobile layout.
- **S7.4**: Update the feature registry (`docs/registry/`) with the new log-viewer frontend feature entry.

#### Tasks

- [ ] TestAnsiXssGuard — `web-app/src/lib/logs/__tests__/logParser.test.ts` — Test: `renderAnsi('<script>alert(1)</script>')` does not include `<script>` in output; `renderAnsi('"><img src=x onerror=alert(1)>')` does not include `onerror`; use `@testing-library/jest-dom` matchers
- [ ] TestOscHyperlinkStripping — `web-app/src/lib/logs/__tests__/logParser.test.ts` — Test: `renderAnsi('\x1b]8;;javascript:alert(1)\x07click me\x1b]8;;\x07')` does not contain `javascript:` in output; verifies the OSC pre-strip regex fires before `ansi-to-html`
- [ ] TestPartialAnsiSequence — `web-app/src/lib/logs/__tests__/logParser.test.ts` — Test: `renderAnsi('text\x1b[31')` (truncated sequence) does not throw and returns a string containing "text"
- [ ] TestLevelDetection — `web-app/src/lib/logs/__tests__/logParser.test.ts` — Parameterized tests for `detectLevel()`: `"2026-01-01 ERROR foo"` → `"ERROR"`, `"[WARN] bar"` → `"WARN"`, `"no level"` → `"UNKNOWN"`, `"debug mode"` → `"DEBUG"`, `"ERR: connection refused"` → `"ERROR"`
- [ ] TestHighlightSegments — `web-app/src/lib/logs/__tests__/logParser.test.ts` — Test `segmentText("hello world", "world")` returns `[{text:"hello ", highlight:false}, {text:"world", highlight:true}]`; test empty query returns single non-highlighted segment; test case-insensitive match; test query not found returns single non-highlighted segment
- [ ] TestScrollAnchorStateMachine — `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts` — Using `renderHook` from `@testing-library/react`: (1) initial state has `isFollowing: true`; (2) calling `onAtBottom(false)` transitions `isFollowing` to `false` and increments `queuedNewLineCount`; (3) calling `jumpToLatest()` transitions `isFollowing` back to `true` and resets counter; (4) calling `onAtBottom(true)` (natural scroll to bottom) also transitions `isFollowing` to `true`
- [ ] TestExpansionStateAccordion — `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts` — Test `toggleRow(3)` sets `expandedRowIndex = 3`; calling `toggleRow(3)` again sets it to `null`; calling `toggleRow(3)` then `toggleRow(7)` sets it to `7` (previous collapses)
- [ ] TestMultiLevelFilter — `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts` — Mock `getLogs` returning entries with mixed levels; set `levelFilters = ["ERROR", "WARN"]`; verify `filteredLogs` contains only ERROR and WARN entries; set `levelFilters = []` or `["ALL"]`; verify all entries returned
- [ ] E2ETestLiveTail — `tests/e2e/log-viewer.spec.ts` — `// @feature logs:view`; `test.describe('log-viewer', ...)`: test `log-viewer_should_autoScrollToBottom_When_liveTailEnabled`; mock API to return logs in batches; assert viewport at bottom after each batch; assert `JumpToLatestButton` not visible
- [ ] E2ETestLiveTailPause — `tests/e2e/log-viewer.spec.ts` — Test `log-viewer_should_showJumpToLatest_When_userScrollsUp`; scroll up by 300px; assert `[data-testid="jump-to-latest"]` is visible; click it; assert viewport returns to bottom; assert button disappears
- [ ] E2ETestSearch — `tests/e2e/log-viewer.spec.ts` — Test `log-viewer_should_highlightMatches_When_searchQueryEntered`; type "error" in search bar; assert `<mark>` elements appear in visible rows; assert match counter shows non-zero count
- [ ] E2ETestLevelFilter — `tests/e2e/log-viewer.spec.ts` — Test `log-viewer_should_filterToErrorOnly_When_errorChipSelected`; click ERROR chip; assert only rows with `[data-testid^="log-row"]` that have the ERROR badge class are visible; click ALL chip; assert all rows visible
- [ ] E2ETestRowExpansion — `tests/e2e/log-viewer.spec.ts` — Test `log-viewer_should_expandRow_When_rowClicked`; click first log row; assert `[aria-expanded="true"]` on that row; assert detail panel visible below; click second row; assert first row collapsed, second expanded
- [ ] E2EMobileLayout — `tests/e2e/log-viewer.spec.ts` — Set viewport to 390×844 (iPhone 14); assert search bar is in collapsed state (icon only); tap search icon; assert input appears; assert filter chips row is horizontally scrollable (check `scrollWidth > clientWidth`); assert touch targets ≥ 44px for all chips (via Axe bounding box check)
- [ ] UpdateFrontendRegistry — `docs/registry/features/frontend/log-viewer.json` — Create feature entry: `{ "id": "log-viewer", "type": "frontend", "component": "LogViewer", "filePath": "web-app/src/components/logs/LogViewer.tsx", "tested": true, "testIds": ["log-viewer_should_autoScrollToBottom_When_liveTailEnabled", "log-viewer_should_showJumpToLatest_When_userScrollsUp", "log-viewer_should_highlightMatches_When_searchQueryEntered", "log-viewer_should_filterToErrorOnly_When_errorChipSelected", "log-viewer_should_expandRow_When_rowClicked"], "lastModified": "2026-05-14T00:00:00Z" }`
- [ ] UpdateBackendRegistry — `docs/registry/features/backend/` — Update the existing `GetLogs` backend feature entry: set `tested: true`, add test function names from the new e2e tests; if a new per-feature JSON file is needed for the multi-level filter proto change, create `docs/registry/features/backend/logs-multi-level-filter.json`
- [ ] RunRegistryGenerate — shell — Run `make registry-generate` and commit any changed files in `docs/registry/features/`; run `make registry-diff` first to preview

---

## File Creation Summary

### New Files

| File | Epic |
|---|---|
| `web-app/src/components/logs/LogViewer.tsx` | E1 |
| `web-app/src/components/logs/LogViewer.css.ts` | E1 |
| `web-app/src/components/logs/LogViewerToolbar.tsx` | E1, E3 |
| `web-app/src/components/logs/VirtualLogList.tsx` | E1, E2 |
| `web-app/src/components/logs/VirtualLogList.css.ts` | E1, E2 |
| `web-app/src/components/logs/LogRow.tsx` | E1, E3, E5 |
| `web-app/src/components/logs/LogRow.css.ts` | E1, E3, E5 |
| `web-app/src/components/logs/ExpandedLogDetail.tsx` | E4 |
| `web-app/src/components/logs/ExpandedLogDetail.css.ts` | E4 |
| `web-app/src/components/logs/JumpToLatestButton.tsx` | E1, E2 |
| `web-app/src/components/logs/JumpToLatestButton.css.ts` | E1, E2 |
| `web-app/src/components/logs/LevelFilterChips.tsx` | E1, E3 |
| `web-app/src/components/logs/LevelFilterChips.css.ts` | E1, E3 |
| `web-app/src/components/logs/ShortcutHelpOverlay.tsx` | E3 |
| `web-app/src/components/logs/ShortcutHelpOverlay.css.ts` | E3 |
| `web-app/src/lib/hooks/useLogViewer.ts` | E1, E2, E3, E6 |
| `web-app/src/lib/logs/logParser.ts` | E1, E3, E4 |
| `web-app/src/lib/logs/__tests__/logParser.test.ts` | E7 |
| `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts` | E7 |
| `tests/e2e/log-viewer.spec.ts` | E7 |
| `docs/adr/010-react-virtuoso-log-viewer.md` | E1 |
| `docs/registry/features/frontend/log-viewer.json` | E7 |

### Modified Files

| File | Epic | Change |
|---|---|---|
| `web-app/package.json` | E1 | Add `react-virtuoso` dependency |
| `web-app/src/components/logs/index.ts` | E1 | Export new components |
| `web-app/src/app/logs/page.tsx` | E1, E6 | Replace table with `<LogViewer source="app" />` |
| `web-app/src/app/logs/page.css.ts` | E1 | Remove row/cell styles now in LogViewer |
| `web-app/src/components/sessions/SessionLogsTab.tsx` | E1, E6 | Replace table with `<LogViewer source="session" />` |
| `web-app/src/components/sessions/SessionLogsTab.css.ts` | E1 | Remove table styles now in LogViewer |
| `proto/session/v1/session.proto` | E6 | Add `repeated string levels` to `GetLogsRequest` |
| `server/services/utility_service.go` | E6 | Update `parseLogs()` for multi-level filter |

---

## Commit Message Conventions

Follow Conventional Commits as required by `CLAUDE.md`:

| Epic | Prefix | Example |
|---|---|---|
| E1 scaffolding | `feat:` | `feat(logs): scaffold LogViewer component and virtual list` |
| E2 virtual scroll | `feat:` | `feat(logs): implement react-virtuoso with live tail follow mode` |
| E3 search/filter | `feat:` | `feat(logs): add client-side search highlight and level filter chips` |
| E4 expandable rows | `feat:` | `feat(logs): implement expandable row accordion with JSON detail` |
| E5 mobile UX | `feat:` | `feat(logs): split-column sticky gutter and mobile touch polish` |
| E6 wire-up | `feat:` | `feat(logs): wire LogViewer to app logs page and session logs tab` |
| E6 proto change | `feat:` | `feat(proto): add repeated levels field to GetLogsRequest` |
| E7 tests | `test:` | `test(logs): add e2e log viewer spec and unit tests for logParser` |
| E7 registry | `chore:` | `chore(registry): register log-viewer frontend feature` |
| ADR | `docs:` | `docs(adr): ADR-010 react-virtuoso for log viewer virtual scroll` |

---

## ADR Flag

**ADR-010 required**: `react-virtuoso` (32 KB gzip) vs. hand-rolled virtual scroller (0 KB) vs. `@tanstack/virtual` (4 KB gzip).

- The architecture agent recommended a hand-rolled CSS-only virtual scroller or `@tanstack/virtual`.
- The pitfalls agent recommended `react-virtuoso` specifically for P-01 (dynamic row heights) and P-03 (live tail scroll anchoring).
- **Synthesis decision**: `react-virtuoso` wins because: (a) `followOutput` eliminates ~120 lines of hand-rolled scroll anchor management that is fragile on iOS (P-03); (b) ResizeObserver-based height measurement eliminates the need to call `resetAfterIndex` on every expansion (P-08); (c) 32 KB is 0.6% of the 5 MB bundle budget — acceptable given the UX complexity it solves.
- The ADR should document this trade-off and the conditions under which it would be revisited (e.g., if bundle size constraints tighten below 1 MB).
