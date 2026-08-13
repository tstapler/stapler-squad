# Pitfalls Research — Datadog-Like Log Viewer

**Date**: 2026-05-14
**Scope**: React log viewer with virtual scrolling, live tail, iOS Safari, ANSI rendering, search highlight

---

## 1. Pitfall Inventory

### P-01: Virtual Scroller with Dynamic Row Heights (CRITICAL)

**Description**: The current `SessionLogsTab` and `LogsPage` render a plain `<table>` with no virtualization. The expandable row requirement (FR-5) means row heights change dynamically when a row is expanded. Fixed-height virtual scrollers (react-window `FixedSizeList`) cannot handle this at all — they use a single row height for all rows. Variable-height scrollers (react-window `VariableSizeList`, `@tanstack/react-virtual`, react-virtuoso) require measuring and caching each row's height, which they do via a resize observer per row.

**Severity**: High. Without virtualization, 10 000 lines will allocate ~10 000 DOM nodes (roughly 400–700 KB of DOM memory) and cause layout reflows on every update.

**Mitigation**:
- Use `react-virtuoso` (the only popular library designed for variable-height rows with dynamic expansion). It wraps a ResizeObserver around every rendered item and recalculates positions incrementally. It also supports the "follow output" (stick to bottom) pattern natively via `followOutput`.
- Alternatively, use `@tanstack/react-virtual` with `measureElement` (requires React 18 ref callback) — handles dynamic heights but requires more manual wiring.
- Do NOT use `react-window` `VariableSizeList` with dynamic expansion: it requires calling `resetAfterIndex(i)` every time row `i` changes height, and computing all heights above `i` is O(n) in the worst case.
- The existing codebase has no virtual scroller installed (`package.json` has no react-window, react-virtual, react-virtuoso, or @tanstack/virtual). A new dependency is required.

**Recommendation**: `react-virtuoso` — handles variable heights, sticky headers, follow-output, and grouped lists. Used in production by major log viewers (including Grafana Loki's UI).

---

### P-02: React Re-render Trap — Appending to Large Array State (HIGH)

**Description**: The current pattern in `LogsPage` and `SessionLogsTab` is `setLogs(prev => [...prev, ...entries])`. In React 18, this causes the full logs array to be passed as a new reference, triggering re-renders of any child that receives the array as a prop without memoization. With 5 000 items, the spread `[...prev, ...entries]` is itself O(n) on every live-tail tick (every 2–3 seconds), allocating a new array each time.

**Severity**: High. At 10 000+ lines with 2-second live-tail intervals, this is a measurable jank source (1–5ms per update in Chrome's scheduler, potentially 16ms+ with a large DOM).

**Mitigation**:
- Use `useRef` to store the canonical log array and only trigger a re-render via `useState` for a version counter (`setVersion(v => v + 1)`). Child components read from the ref, not from state.
- Or use a reducer with structural sharing: `useReducer(logsReducer, [])` where `append` creates a new array from the existing reference only when the render actually needs it.
- With a virtual scroller, only the visible window re-renders; the key discipline is that `data[index]` lookups via index must be O(1). Prefer an array (not a Map) for indexing by position.
- Do NOT batch live-tail updates inside `useEffect` timers that run synchronously — wrap in `startTransition()` (React 18) so log appends are interruptible and don't block user input.

---

### P-03: Live Tail Scroll Anchoring — Content Shift Jank (HIGH)

**Description**: When new log lines are appended at the bottom while the user is in "follow" mode, the browser's default behavior is `overflow-anchor: auto`, which tries to anchor the scroll position to a visible element. When content is prepended (older history) this anchoring causes visible scroll jumps. When content is appended, the user expects auto-scroll to the new bottom, but with a virtual scroller the scroll container's `scrollHeight` changes and the anchor behavior can fight the programmatic `scrollToBottom()` call.

**Severity**: High. This is the most common complaint in log viewer UIs — the "scroll to bottom" button flickers or the view jumps when new content arrives.

**Mitigation**:
- Set `overflow-anchor: none` on the scroll container to disable browser anchor and handle anchoring manually.
- Implement two modes: (a) "following" — always call `scrollToBottom()` after each batch write, using `requestAnimationFrame` to run after the DOM update; (b) "paused" — lock scroll position and show a "N new lines" indicator.
- Detect the "pause" transition: when `scrollTop + clientHeight < scrollHeight - threshold (e.g., 50px)`, the user has scrolled up — transition to paused mode. This is already partially implemented in the `useLiveTail` hook but not wired to scroll position.
- `react-virtuoso` has a built-in `followOutput` prop that implements this pattern correctly; it handles the scroll anchor internally and exposes `atBottom` state.
- Never use `scrollIntoView()` on the last row — it causes layout thrashing on every append.

---

### P-04: `position: sticky` + Horizontal Scroll in iOS Safari (HIGH)

**Description**: FR-2 requires the line number gutter and log level badge to be `position: sticky` on the left while the message body scrolls horizontally. This is a well-documented iOS Safari bug: when the scroll container has `overflow-x: auto` or `overflow-x: scroll`, `position: sticky` elements with `left: 0` stop working on iOS Safari ≤ 16. The element scrolls away with the content instead of staying pinned.

**Root cause**: iOS Safari only supports `sticky` along the scroll axis matching the containing scroll block. A container with `overflow-x: scroll` does not create a valid sticky scroll container for `left`-axis sticky in older Safari. This affects iOS 15 and 16; iOS 17+ partially improved it but still has edge cases with `overflow: auto` on parent containers.

**Severity**: High for mobile target. The entire FR-2 requirement fails silently on iOS Safari < 17.

**Mitigation options**:
1. **Split the table into two overlapping `<div>`s**: a narrow fixed-left column (line no + level badge) with `overflow: hidden` and a wide scrollable column (message body). The fixed column never scrolls horizontally; the wide column scrolls with `overflow-x: auto`. Absolute-position the fixed column on top of the scrollable column. This is the approach used by Google Sheets, VS Code's editor gutter, and most production log viewers.
2. **CSS Grid with `subgrid`**: Place the sticky columns in a grid that does not participate in horizontal scroll. Only available in Safari 16+, so still requires fallback.
3. **Avoid `sticky` entirely**: Use `position: absolute` on the left column within a positioned scroll wrapper and use scroll event listeners to keep it pinned. More complex but broadest compatibility.

**Recommendation**: The split-div approach (#1) is the most compatible. The existing `SessionLogsTab.css.ts` already uses `position: sticky` on `thead` — this works only vertically (top-axis scroll) which is safe. The new horizontal-sticky requirement needs the split approach.

---

### P-05: iOS Safari Momentum Scroll and `visualViewport` (MEDIUM)

**Description**: iOS Safari's momentum scrolling (inertia after swipe) can interfere with programmatic `scrollTop` assignments during live-tail follow mode. If `scrollTop` is set via JS during a momentum scroll animation, Safari ignores the JS assignment or cancels the scroll mid-flight, causing the view to appear to "bounce back" away from the bottom. Additionally, the virtual keyboard causes the `window.innerHeight` to change but NOT `window.visualViewport.height` changes fire at a different time, causing flash-of-incorrect-height on the scroll container.

**Severity**: Medium. Degrades UX on iPhone but not a blocker.

**Mitigation**:
- Use `el.scrollTop = el.scrollHeight` instead of `el.scrollTo({ behavior: 'smooth' })` for programmatic live-tail anchoring. Smooth scrolling conflicts with momentum.
- Listen to `visualViewport.resize` (not `window.resize`) for keyboard appearance, as the existing `TerminalOutput.tsx` already does. Apply the same pattern to the log viewer scroll container.
- Wrap scroll container with `overscroll-behavior-y: contain` to prevent scroll chaining to the parent page (prevents the whole page from bouncing when the log list hits top/bottom).
- Do not rely on `-webkit-overflow-scrolling: touch` — deprecated in iOS 13+ and removed.

---

### P-06: ANSI Escape Code Security — XSS via `dangerouslySetInnerHTML` (HIGH)

**Description**: Log messages from tmux terminal sessions will contain ANSI escape codes (color, bold, cursor movement, etc.). The existing codebase already uses `ansi-to-html@0.7.2` with `escapeXML: true` in `useTerminalSnapshot.ts` and `SessionCard.tsx`. This library converts ANSI codes to `<span>` tags with inline `style` attributes. The `escapeXML: true` flag escapes `<`, `>`, `&`, `"`, `'` in the text content, preventing XSS in the message body.

**However, three risks remain**:

1. **OSC/hyperlink sequences**: ANSI OSC 8 sequences create hyperlinks (`ESC]8;;URL\atext`). `ansi-to-html` converts these to `<a href="URL">` tags. If the URL is `javascript:alert(1)`, this is an XSS vector. The library does not filter `href` schemes.
2. **Partial/truncated sequences**: Log lines truncated mid-ANSI-sequence (e.g., `ESC[31` with no `m`) can cause the parser to emit garbage HTML or get stuck in sequence-consuming state, corrupting subsequent lines.
3. **Large ANSI payloads**: A log line with thousands of color resets causes `ansi-to-html` to emit thousands of `<span>` tags per line, each a DOM node — this is a DoS for the renderer.

**Severity**: High (XSS), Medium (corruption), Medium (DoS).

**Mitigation**:
- For OSC 8 hyperlinks: either strip OSC sequences entirely before passing to `ansi-to-html` (use a pre-processing regex: `/\x1b\][^\x07]*(?:\x07|\x1b\\)/g` to remove all OSC), or use `ansi-regex` to enumerate sequence types and filter `javascript:` hrefs post-conversion with DOMPurify.
- Use DOMPurify on the output of `ansi-to-html` before inserting via `dangerouslySetInnerHTML`. DOMPurify is already a common dep in React apps; configure it with `ALLOWED_TAGS: ['span', 'a']` and `ALLOWED_ATTR: ['style', 'href']` plus `FORCE_BODY: false`. Add a hook to block `javascript:` hrefs: `DOMPurify.addHook('afterSanitizeAttributes', node => { if (node.href?.startsWith('javascript:')) node.removeAttribute('href'); })`.
- Cap rendered ANSI spans per line: strip all ANSI codes beyond the first N (e.g., 50) color changes on a single line before passing to `ansi-to-html`.
- The log viewer for application logs (structured logs from `~/.stapler-squad/logs/`) is lower risk since it comes from the Go logger which does not emit ANSI. For session logs (terminal output), ANSI must be treated as untrusted.

---

### P-07: Search Highlight Performance at 10k+ Lines (MEDIUM)

**Description**: FR-3 requires real-time search highlighting (< 100ms for 10 000 lines). The current implementation uses `debouncedSearch` (300ms) and re-fetches from the backend on every query change. For a virtual scroller with client-side highlighting, the highlight pass must happen before render. Naive approaches — creating new React elements with highlighted spans for every visible row on every keystroke — will block the main thread for >100ms at scale.

**Severity**: Medium. The 100ms target is tight for naive implementations.

**Mitigation**:
- Keep search filtering server-side for the primary result set (already done via the API's `searchQuery` param). Client-side highlighting is only for visible rows in the virtual window — typically 20–50 rows. Highlighting 50 rows with a regex replace is <1ms.
- For pure client-side highlight (if the backend doesn't support it), use a Web Worker for the filtering pass (build an index of matched line indices), then only highlight the visible window in the main thread.
- Use `String.prototype.indexOf` in a loop rather than `RegExp.exec` in a loop for case-insensitive plain-text matching — up to 3x faster for short patterns at 10k lines.
- Avoid creating React elements inside the highlight function; emit an array of `{text: string, highlight: boolean}` segments and render them with a pure presentational component that React can memoize with `React.memo`.
- Do not use `dangerouslySetInnerHTML` for highlight rendering — it bypasses React reconciliation and causes full DOM replacement on every keystroke. Use the segment array approach instead.

---

### P-08: Row Expansion Breaks Virtual Scroller Item Cache (MEDIUM)

**Description**: Virtual scrollers that cache item heights (all variable-height implementations) must invalidate their cache when a row expands or collapses. If the cache is stale, all rows below the expanded row will be at wrong scroll offsets, causing content to appear at incorrect positions.

**Severity**: Medium. The visual result is rows "jumping" when expanding/collapsing.

**Mitigation**:
- `react-virtuoso` handles this automatically — it uses ResizeObserver on each rendered item and updates its internal position map reactively.
- If using `@tanstack/react-virtual`, call `virtualizer.measureElement(element)` in a `useEffect` after expansion state changes. The `measureElement` approach uses a resize observer and updates positions incrementally.
- Never manually pass heights to the scroller if they change dynamically — always delegate to the library's measurement mechanism.
- Store expansion state outside the virtual scroller (in a `Set<number>` of expanded indices in the parent component), not inside the item component itself. Item components are unmounted/remounted as they scroll out/in of view; local state is lost.

---

### P-09: React Key Stability for Animated Log Rows (LOW)

**Description**: Using the array index as the React `key` for log rows (current pattern: `logs.map((log, index) => ...)`) causes React to reconcile against the wrong element when new logs are prepended (live-tail inserts at index 0 → all keys shift). This causes full re-animation of all rows. For appended logs (live tail at the bottom), it is safe.

**Severity**: Low for append-only live tail. Medium if older history is prepended.

**Mitigation**:
- Use a stable unique key for each log entry. The backend's `LogEntry` proto should expose a sequence number or timestamp+source composite. Use that as the key.
- If the backend does not provide stable IDs, generate a client-side UUID on receipt and store it alongside the log entry.

---

### P-10: Table Header Sticky + iOS Safari (MEDIUM)

**Description**: `SessionLogsTab.css.ts` already uses `position: sticky; top: 0` on `thead` inside a scrollable `<table>`. This works for vertical stickiness on desktop and recent iOS. However, the table header inside an `overflow: auto` container can "detach" on iOS Safari 15 when the table is inside a flex container with `min-height: 0`. The sticky fails silently — the header scrolls away with the content.

**Severity**: Medium. Affects iOS Safari 15.

**Mitigation**:
- Ensure the scroll container ancestor chain has no element with `overflow: hidden` that would clip the sticky context.
- The containing scroll div must have an explicit `height` (or `max-height`), not just `flex: 1`. The `min-height: 0` flex child pattern is required for the flex container but the scroll container itself must have `height: 100%` or a fixed height.
- Test sticky headers on an actual iPhone (iOS 15+), not just Chrome DevTools mobile emulation — Chrome DevTools does not accurately simulate iOS Safari's stacking context rules.

---

## 2. iOS Safari-Specific Issues

| Issue | Affected Version | Impact | Mitigation |
|-------|-----------------|--------|-----------|
| `position: sticky` breaks inside `overflow-x: scroll` containers | iOS ≤ 16 | FR-2 (sticky gutter) fails entirely | Split-column layout (fixed + scrollable div) |
| Momentum scroll ignores programmatic `scrollTop` assignment mid-flight | All iOS Safari | Live tail "bounce back" | Set `scrollTop` synchronously, never async; use `overflow-anchor: none` |
| `visualViewport` resize fires 200–400ms after keyboard appears | All iOS Safari | Log container shrinks before keyboard settles | Debounce viewport resize handler by 400ms (pattern already in TerminalOutput.tsx) |
| `<table>` `thead` sticky fails in flex containers with `min-height: 0` | iOS Safari 15 | Table header scrolls away | Explicit height on scroll container |
| Touch events fire before `pointerdown` (300ms delay on non-fast-tap elements) | iOS Safari < 16 | Log row taps feel sluggish | Add `touch-action: manipulation` to row elements; use `onPointerDown` not `onClick` for immediate response |
| `user-select: none` on rows prevents text selection in log messages | All iOS | Users cannot copy log text | Only apply `user-select: none` to non-message columns (gutter, level badge) |
| Horizontal swipe on a row (FR-6 quick actions) conflicts with page back-navigation gesture | All iOS | Swipe right on a log row triggers browser back | Use `touch-action: pan-y` on the swipeable row to allow vertical scroll but block horizontal swipe hijacking the browser |

---

## 3. ANSI Code Security Considerations

### Current State in Codebase

- `ansi-to-html@0.7.2` is installed and used in two places:
  - `web-app/src/lib/hooks/useTerminalSnapshot.ts`: `new Convert({ escapeXML: true })` — safe for text content XSS
  - The output is injected via `dangerouslySetInnerHTML` in `SessionCard.tsx` (line 592)
- `escapeXML: true` protects against `<script>` injection in message text
- No DOMPurify, no OSC-sequence stripping, no href filtering

### Recommended Security Stack for Log Viewer ANSI Rendering

```
raw log string
  → strip OSC sequences (regex: /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g)
  → strip cursor-movement sequences (not relevant for log display, only color/bold needed)
  → ansi-to-html with { escapeXML: true }
  → DOMPurify.sanitize(html, { ALLOWED_TAGS: ['span', 'a'], ALLOWED_ATTR: ['style', 'href'] })
  → dangerouslySetInnerHTML
```

### Safe vs. Unsafe Libraries

| Library | XSS Safe | OSC Hyperlinks | Notes |
|---------|----------|---------------|-------|
| `ansi-to-html` (current) | Yes (with `escapeXML: true`) | Emits unsafe `<a href>` | Add DOMPurify post-pass; strip OSC beforehand |
| `ansi_up` (npm) | Yes | Strips OSC | Alternative; slightly smaller bundle |
| Custom regex strip | Yes (no HTML emitted) | N/A — strips all ANSI | Safest; loses color. Appropriate for plain-text mode |
| xterm.js (current for terminal) | Yes (canvas-based) | N/A — renders to canvas | Not suitable for log table rows |

### Recommendation for Log Viewer

Application logs (`~/.stapler-squad/logs/`) come from the Go structured logger and will not contain ANSI escape codes — render as plain text, no ANSI library needed.

Session logs (terminal scrollback) will contain ANSI color codes from Claude Code output. Use the pipeline above with DOMPurify. Stripping OSC sequences before passing to `ansi-to-html` eliminates the hyperlink XSS risk without needing DOMPurify's URL filtering.

---

## 4. Performance Traps Table

| Trap | Location | Cost | Fix |
|------|----------|------|-----|
| `[...prev, ...entries]` spread on every live-tail tick | `LogsPage`, `SessionLogsTab` | O(n) array allocation every 2s | Structural sharing via reducer; or mutable ref + version counter |
| Full table re-render on any state change | `LogsPage` JSX | All N rows reconciled | Virtualize + `React.memo` row component |
| `scrollHeight - scrollTop - clientHeight < 100` scroll handler without RAF throttle | `LogsPage` (line 181–189) | Runs on every scroll pixel, may trigger multiple `loadMoreLogs()` | Throttle with `requestAnimationFrame` or `useRef` debounce flag |
| Search: regex applied to all 10k entries on every keystroke | client-side filter in `LogsPage` | 10k iterations × regex overhead | Backend filtering + client-side highlight only in visible window |
| `ansi-to-html` per row on every render | Future ANSI rendering | String parsing on every render | Memoize per-log-entry with `useMemo` or stable cache keyed by log ID |
| `dangerouslySetInnerHTML` for highlight | If naively implemented | Full DOM subtree replacement per row on keypress | Use React element array (text segments) instead |
| `new Date()` in `formatRelativeTime` called for every row in render | `LogsPage` (lines 496–499) | 10k `Date` constructions on every render | Memoize per-entry; recompute only when timestamp changes |
| `getLevelClass(log.level)` switch in render | `LogsPage` (lines 252–263) | Minor but called N times per render | Precompute level class at fetch time and store with log entry |
| `setTimeout + requestAnimationFrame` for scroll-to-bottom during live tail | Future live tail impl | Frame delay can cause visible jump | Prefer synchronous `scrollTop = scrollHeight` or react-virtuoso's built-in `followOutput` |

---

## 5. Testing Recommendations

### Unit Tests (Jest/RTL)

1. **ANSI XSS guard**: Test that a log message containing `<script>alert(1)</script>` and `"><img src=x onerror=alert(1)>` renders safely (no `<script>` in output).
2. **OSC hyperlink filtering**: Test that `\x1b]8;;javascript:alert(1)\aclick me\x1b]8;;\a` does not emit a `javascript:` href.
3. **Partial ANSI sequence handling**: Test that a string ending mid-sequence (`\x1b[31`) does not throw and emits readable text.
4. **Scroll anchor state transitions**: Mock `scrollTop`/`scrollHeight`/`clientHeight` and verify the live-tail "following" ↔ "paused" state machine transitions correctly.
5. **Expansion state persistence**: Render a virtual list, expand row 5, scroll so row 5 is off-screen, scroll back — verify row 5 is still expanded.
6. **React key stability**: Prepend entries to the log array and verify no existing row DOM node is replaced (use `getByTestId` + `toBe` on a ref stored before prepend).

### Integration Tests (Playwright)

1. **iOS Safari sticky header**: On a real device or BrowserStack with iOS 15/16 Safari, scroll the log table and verify the header remains visible.
2. **Horizontal sticky gutter**: On iOS Safari, scroll right in the log table and verify the line number column stays pinned.
3. **Live tail follow mode**: Enable live tail, wait for 3 polling cycles, verify the viewport is at the bottom each time.
4. **Live tail pause on scroll-up**: Enable live tail, manually scroll up, verify the "Live tail paused" indicator appears and `scrollTop` no longer auto-advances.
5. **Search performance**: Load 10 000 log entries (mock via intercepted API), type a 5-character search query, verify the highlight appears within 100ms (use `performance.now()` markers in the component).
6. **Touch target size**: Use Playwright's Axe or a custom check to verify all interactive log row elements have a bounding box >= 44×44px on a 390px-wide viewport.

### Manual Testing Checklist

- [ ] Load page with 1000 log entries — no frame drops (Chrome DevTools Performance tab shows no >50ms tasks)
- [ ] Live tail ON + scroll to middle → "Paused" indicator appears, live tail icon changes
- [ ] Tap "Jump to Latest" → smooth scroll to bottom, live tail resumes
- [ ] Expand row → row height increases, rows below shift down without jump
- [ ] Collapse row → height returns, no layout shift above the row
- [ ] On iPhone (real device): horizontal scroll in log table → gutter stays pinned
- [ ] Search "error" → all matching lines highlighted within one frame of last keystroke
- [ ] Copy button on expanded row → clipboard contains raw text (no HTML entities)
- [ ] Rotate iPhone → log table reflows correctly, virtual scroller recalculates visible window
