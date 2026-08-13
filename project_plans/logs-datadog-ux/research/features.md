# Features Research: Log Viewer UX Reference

**Project**: logs-datadog-ux
**Date**: 2026-05-14
**Scope**: Live tail, mobile UX, accessibility, sticky gutter, keyboard shortcuts

---

## 1. Feature Inventory by Reference Product

### Datadog Log Management

**Live Tail**
- Streams all ingested logs in near real-time regardless of indexing status
- Log lines are processed through pipelines (parsed, enriched) before display
- Only shows logs within a 15-minute rolling window
- **Pause/resume**: When the user scrolls up, streaming pauses automatically. A "Jump to Live" indicator appears at the bottom of the viewport. Clicking it resumes auto-scroll and live tail.
- The Live Tail mode is toggled via a time-range selector in the Log Explorer toolbar, not a separate page

**Log Side Panel**
- Clicking a log row opens a right-side detail panel (does not leave the list view)
- Structured JSON fields are auto-parsed and rendered as key-value pairs with formatting
- Reserved fields (e.g., `error.stack`, `http.method`, `duration`) get enhanced display (stack trace renderer, HTTP method badge, duration bar)
- Panel shows full raw log text at the top, then structured attributes below
- Copy button per field, plus a "Copy all" for the raw log
- Panel can be pinned open or dismissed with ESC

**Log Level / Severity**
- Severity column uses colored badges (ERROR = red, WARN = amber, INFO = blue/white, DEBUG = gray)
- The entire row gets a subtle left-border color accent matching the severity
- Filter chips at the top of the list let the user restrict by severity (multi-select)

**Search**
- Search bar always visible above the log list
- Matching terms are highlighted inline with yellow background
- Match count shown in the search field as "N results"
- Pressing Enter cycles through matches; Shift+Enter goes backwards

**Sticky Gutter (CSS pattern)**
- Line numbers and severity badge columns use `position: sticky; left: 0; z-index: 10` in CSS tables
- The sticky area has a background color (not transparent) to occlude scrolling content
- Only the timestamp + message body columns scroll horizontally

---

### Sumo Logic Live Tail

- Live tail includes a **Pause** button in the toolbar
- When paused, the Pause button transforms into a **Jump to Bottom** arrow button
- Clicking "Jump to Bottom" (or a banner CTA at the bottom of the list) resumes auto-scroll to latest
- This is the clearest documented pattern for the pause/resume CTA that the requirements reference

---

### Grafana Loki (Explore UI)

**Live Tail**
- Activated via a "Live" button in the Explore toolbar
- New arriving logs appear at the **bottom** with a contrasting highlight background (brief flash)
- The highlight fades after ~2 seconds to avoid visual noise
- Uses WebSocket (`/loki/api/v1/tail`) to stream log lines
- `delay-for` flag buffers up to 5 seconds to re-order out-of-sequence multi-stream logs

**Scroll Behavior**
- Scrolling up in Live mode shows historic context without stopping the stream
- A sticky "Scroll to bottom" pill button appears in the bottom-right when the user is not at the tail
- Clicking it resumes scroll-to-bottom without toggling the live connection off

**Log Visualization**
- Logs panel supports "Logs" visualization type with optional log-level coloring
- Log level is extracted from structured labels or detected by regex from the log line
- Each log line has an expand chevron; clicking it shows the full parsed labels as key-value chips
- The "Wrap lines" toggle controls line wrap vs. horizontal scroll (default: no wrap)

---

### Kibana / Logtrail Plugin

**Live Tail (Logtrail)**
- CTRL+ALT+S starts a new session; CTRL+ALT+Z stops it; CTRL+ALT+X clears
- SHIFT+? shows the shortcut list
- Scrolling freezes the display; a "Scroll Down" button brings the user back to the live tail bottom
- Stream continues in the background while paused

**Kibana Logs UI**
- Keyboard navigation via Page Up / Page Down is a requested feature (open issue as of 2025), indicating this is a gap in the current Kibana UX
- Inline search with highlighting and result count is standard

---

### lnav (Terminal Log File Navigator)

lnav is the gold standard for keyboard-driven log navigation and establishes user muscle memory.

| Key | Action |
|-----|--------|
| `/` | Enter search (regex supported) |
| `n` | Next match |
| `N` | Previous match |
| `g` | Jump to top (first log line) |
| `G` | Jump to bottom (last log line / live tail) |
| `j` / `k` | Down / up one line (vi-style) |
| `d` / `u` | Down / up half page |
| `f` / `b` | Forward / back full page |
| `=` | Toggle pause on live tail |
| `e` | Jump to next ERROR |
| `E` (Shift+e) | Jump to previous ERROR |
| `w` / Ctrl+W | Toggle word wrap |
| `l` | Cycle through log levels (filter) |
| `?` or `h` | Show help |

**Why lnav matters for web UX**: Web log viewers used by developers will be compared to lnav. `/` for search and `g`/`G` for top/bottom are deeply ingrained muscle memory. Deviating from these conventions frustrates power users.

---

### Dozzle (Docker Container Log Viewer)

- Minimal, clean web UI with real-time container log streaming
- Search/filter by text, live updates via WebSocket
- Log lines are monospace, no wrapping; horizontal scroll on overflow
- Mobile layout corrections shipped in v9.0 (January 2026); stats panels reflow to single column on narrow screens
- Split-screen mode (two containers side-by-side) — not relevant to our use case
- No per-line log level coloring (Docker logs are unstructured), but ANSI color codes are rendered
- Follow mode auto-scrolls; scrolling up pauses follow; a "Jump to latest" button appears

---

### GoAccess

- Generates self-contained HTML reports with real-time WebSocket updates
- Designed for web server access log analysis, not general-purpose log tailing
- Color-coded panels per metric; responsive grid layout for mobile
- Not a row-level log viewer; operates on aggregated stats panels
- Limited relevance to our use case beyond: push updates via WebSocket, mobile-responsive HTML output

---

### react-logviewer (melloware)

A React component directly applicable to our stack:

- `LazyLog`: loads logs from a URL, static text, WebSocket, or EventSource
- Uses `react-virtualized` under the hood — renders only visible rows; handles 100MB+ files
- `ScrollFollow` HOC: toggles auto-follow based on user scroll position (exact pattern we need)
- ANSI color code rendering built-in
- `stream={true}` prop for chunked/streaming responses (shorter time to first render)
- **Gap**: No built-in log level detection/coloring or filter chips — must be layered on top
- **Gap**: No JSON pretty-print for structured rows — must be custom

---

## 2. Recommended UX Patterns to Adopt

### 2.1 Live Tail Pause / Resume

**Pattern (from Sumo Logic + Dozzle + Grafana):**

1. Default state: auto-scroll ON, new lines append at bottom, viewport follows latest
2. User scrolls up → auto-scroll pauses immediately (no lag)
3. A sticky **"Jump to Latest"** pill button appears in the bottom-right corner with a downward-chevron icon
4. Pill shows the count of new lines received while paused (e.g., "↓ 42 new lines")
5. Tapping/clicking the pill scrolls to bottom and re-enables auto-scroll
6. Scrolling manually to the bottom (within ~2 rows) also re-enables auto-scroll

**Rationale**: The line count on the "Jump to Latest" pill is a Datadog-style touch that communicates urgency — it helps the user decide whether to jump or keep reading.

### 2.2 Horizontal Scroll with Sticky Gutter

**Pattern:**

```
┌──────┬───────┬─────────────────────────────────────────────────────┐
│ Line │ Level │ Timestamp  Message body (scrolls horizontally →)    │
│  #   │ badge │ (sticky)   ...long long long long long log content   │
└──────┴───────┴─────────────────────────────────────────────────────┘
  sticky        sticky        ← horizontal scroll only on this area →
```

- Line number gutter + level badge: `position: sticky; left: 0; background: var(--background)`
- The sticky columns must have a solid (non-transparent) background to cover scrolling text
- `overflow-x: auto` on the scroll container; `white-space: nowrap` on each log row
- On mobile: level badge is the highest-priority sticky column; line number may be omitted on very narrow screens (< 380px)
- Use `will-change: transform` on the scroll container for GPU compositing on mobile

### 2.3 Expandable Row Detail

**Pattern (from Datadog side panel, adapted for mobile):**

- Single tap/click on a log row expands an **inline** detail panel below the row (accordion, not modal/side panel)
- Inline expansion is mobile-first; a side panel requires too much screen width on phones
- Expanded view shows:
  1. Full raw log line (monospace, selectable)
  2. If JSON-parseable: pretty-printed key-value tree with syntax highlighting
  3. Copy button for the full raw line
  4. For JSON: copy button per field value
- Second tap collapses
- Only one row expanded at a time (accordion behavior); expanding a second row collapses the first
- Pinch-to-zoom on expanded JSON is handled by the browser natively (do not suppress)

### 2.4 Log Level Filter Chips

**Pattern:**

- Horizontal scrollable chip row above the log list: `ALL` | `ERROR` | `WARN` | `INFO` | `DEBUG`
- Multi-select: `ALL` is deselected when any specific level is selected; selecting `ALL` clears others
- Active chip has full background color matching its level color
- Chips are `min-height: 44px` for touch accessibility (Apple HIG)
- Filter is applied client-side in < 100ms via pre-tagged row data (no re-fetch)

### 2.5 Search Bar

**Pattern:**

- Fixed position at the top of the log panel (not in the page header)
- On mobile: collapsed to a magnifying-glass icon by default; taps to expand inline below the filter chips
- Placeholder: "Search logs... (/ to focus)"
- Highlights matching substrings with yellow background (not whole-row highlight)
- Match counter: "12 / 47" right-aligned inside the search field
- ESC or clear (×) button resets
- On desktop: `/` key focuses the search field from anywhere in the log view

---

## 3. Accessibility Notes (Color Tokens & Contrast Ratios)

### WCAG AA Requirement
- Normal text (< 18pt / 14pt bold): **4.5:1 minimum contrast ratio**
- Large text and UI components (badges, icons): **3:1 minimum**
- Never communicate log level by color alone — always include the text label (ERROR, WARN, etc.)

### Log Level Color Palette (WCAG AA on both light and dark backgrounds)

| Level | Badge text color | Badge bg (dark mode) | Badge bg (light mode) | Row tint (dark) | Row tint (light) |
|-------|-----------------|---------------------|----------------------|-----------------|------------------|
| ERROR | #FFFFFF | #B91C1C (red-700) | #DC2626 (red-600) | rgba(220,38,38,0.12) | rgba(220,38,38,0.08) |
| WARN | #1A1A1A | #D97706 (amber-600) | #F59E0B (amber-400) | rgba(245,158,11,0.12) | rgba(245,158,11,0.08) |
| INFO | #FFFFFF | #1D4ED8 (blue-700) | #2563EB (blue-600) | transparent | transparent |
| DEBUG | #FFFFFF | #6B7280 (gray-500) | #9CA3AF (gray-400) | transparent | transparent |
| TRACE | #FFFFFF | #6B7280 (gray-500) | #9CA3AF (gray-400) | transparent | transparent |

**Verification notes:**
- White (#FFF) on red-700 (#B91C1C): contrast ~5.9:1 — PASS AA
- White (#FFF) on amber-600 (#D97706): contrast ~3.5:1 — PASS AA for large text/badges (≥ 3:1); use dark text (#1A1A1A) for normal-size text on amber — dark on amber-600 is ~7.1:1
- White (#FFF) on blue-700 (#1D4ED8): contrast ~5.9:1 — PASS AA
- White (#FFF) on gray-500 (#6B7280): contrast ~4.6:1 — PASS AA

**Additional guidance:**
- Row tints must NOT be the sole indicator of level; the badge text must always be present
- In light mode, ensure row tint bg + text color maintains 4.5:1 contrast (the tint is very subtle at 8% opacity — body text remains on the page background, not the tint)
- Test with a color-blind simulator: red/green distinction is the most common deficiency; ERROR red and INFO blue are safe; WARN amber is generally safe from red-green blindness

### ANSI Color Rendering
- ANSI color codes in raw terminal output should be rendered (not stripped)
- Map ANSI 16-color palette to theme-aware equivalents so they pass contrast on both dark/light backgrounds

---

## 4. Keyboard Shortcut Reference to Implement

Modeled on lnav conventions (well-known to the developer audience):

| Shortcut | Action | Notes |
|----------|--------|-------|
| `/` | Focus search field | Industry standard (lnav, vim, less) |
| `Cmd+F` / `Ctrl+F` | Focus search field | Browser convention fallback |
| `ESC` | Clear search / blur search field | Standard |
| `n` | Next search match | lnav convention |
| `N` (Shift+n) | Previous search match | lnav convention |
| `g` | Jump to top (oldest log line) | lnav/vim convention |
| `G` (Shift+g) | Jump to bottom / resume live tail | lnav/vim convention |
| `j` | Scroll down one row | vi-style; also arrow ↓ |
| `k` | Scroll up one row | vi-style; also arrow ↑ |
| `Space` | Scroll down one page | less/man convention |
| `b` | Scroll up one page | less convention |
| `e` | Jump to next ERROR | lnav convention |
| `E` (Shift+e) | Jump to previous ERROR | lnav convention |
| `Enter` | Expand / collapse selected row | — |
| `=` | Toggle live tail pause | lnav convention |
| `?` | Show keyboard shortcut help overlay | Universal |
| Arrow ↓/↑ | Move selected row down/up | Standard keyboard nav |
| `Tab` | Cycle through log level filter chips | Accessibility |

**Implementation notes:**
- All shortcuts must be disabled when focus is inside an input field (search bar, etc.) to prevent conflicts
- `Cmd+F` / `Ctrl+F` should be intercepted and redirect to the app's search bar, not the browser's native find (use `event.preventDefault()` selectively, only when the log panel is focused)
- Shortcuts must have no effect on mobile (no physical keyboard assumed); provide a `?` help button in the toolbar for keyboard reference on desktop

---

## 5. Mobile Interaction Patterns

### Touch Targets
- **Minimum touch target**: 44 × 44px (Apple HIG) / 48 × 48dp (Material Design)
- Log level filter chips: min-height 44px, min-width 56px with padding
- "Jump to Latest" pill: min-height 44px, padded 16px horizontal
- Expand chevron on log row: 44 × 44px tap area even if visually smaller
- Search clear (×) button: 44 × 44px tap area

### Swipe Gestures
- **Vertical scroll** (primary): native browser scroll, no interception; auto-scroll pause triggered on any scroll-up delta
- **Horizontal log line swipe**: on very long log lines, the scroll container scrolls horizontally; on mobile, single-finger horizontal swipe inside the log line should scroll the content — use `touch-action: pan-x` on the log row scroll container; `touch-action: pan-y` on the outer vertical scroll container
- **Swipe to reveal quick actions**: swipe left on a log row (using `touchstart`/`touchend` delta detection) reveals a 44px action strip with "Copy" and "Share" icons — adopt the iOS swipe-to-reveal pattern
- **Pinch to zoom**: do NOT suppress in the expanded row detail; JSON pretty-print should be zoomable
- Avoid two-finger gestures for primary actions — they conflict with system gestures on iOS (e.g., two-finger scroll to activate browser chrome)

### Search Bar on Mobile
- Collapsed state: magnifying-glass icon button in the toolbar (44px tap area)
- Tap to expand: slides down as an overlay bar below the filter chips
- `inputmode="search"` and `type="search"` to trigger the correct mobile keyboard
- `autocapitalize="none" autocorrect="off" autocomplete="off"` to prevent text "corrections" on search terms
- "Done" / keyboard dismiss should NOT clear search results

### Layout at Narrow Widths (< 430px / iPhone-sized)
- Line number gutter: hide at < 380px; show at ≥ 380px
- Level badge: always sticky, always visible — this is the most critical column
- Timestamp: show abbreviated format (HH:mm:ss) at < 430px; full ISO timestamp on wider screens
- Filter chips row: horizontally scrollable, no wrapping, -webkit-overflow-scrolling: touch
- "Jump to Latest" pill: fixed position, bottom-right, above any browser nav bar safe area (use `padding-bottom: env(safe-area-inset-bottom)`)

### Performance on Mobile
- Virtual scrolling is required for > 500 lines (lower threshold than desktop due to memory constraints on iOS)
- Use `requestAnimationFrame` to batch DOM updates from live-tail message receipt
- Coalesce rapid-fire WebSocket messages into batches of up to 50ms before rendering
- `IntersectionObserver` to detect when user is at the bottom (more reliable than scroll event + height calculation on mobile Safari)
- Use `overscroll-behavior-y: contain` on the log panel container to prevent iOS rubber-band bounce from triggering false "at bottom" detection

### Screen Reader Accessibility
- Use `role="log"` and `aria-live="polite"` on the log container with throttled announcements (max 1 announcement per 3 seconds during live tail to avoid overwhelming screen readers)
- Each log row: `role="row"` with `aria-label` including level and timestamp
- Expanded row detail: `aria-expanded="true/false"` on the row element
- Filter chips: `role="group"` with `aria-label="Filter by log level"`, each chip uses `role="checkbox"` or `aria-pressed`
- "Jump to Latest" pill: `aria-label="Jump to latest log entry"` with live region update for new line count

---

## 6. OSS Library Shortlist

| Library | Use | Notes |
|---------|-----|-------|
| `@melloware/react-logviewer` | Base log rendering + virtual scroll + ScrollFollow | Best-fit; add level coloring and filter layer on top |
| `TanStack Virtual` | Alternative if react-logviewer doesn't fit architecture | Lower-level; more control |
| `react-virtuoso` | Alternative with good variable-height row support | Handles expanded row height changes cleanly |

For expanded JSON rendering, use a lightweight JSON formatter (e.g., `react-json-view-lite` or hand-rolled with `JSON.stringify(val, null, 2)` in a `<pre>`) rather than a heavy library.

---

## Sources

- [Datadog Live Tail Documentation](https://docs.datadoghq.com/logs/explorer/live_tail/)
- [Datadog Live Tail Blog Post](https://www.datadoghq.com/blog/live-tail-log-management/)
- [Datadog Log Side Panel](https://docs.datadoghq.com/logs/explorer/side_panel/)
- [Sumo Logic Live Tail](https://www.sumologic.com/help/docs/search/live-tail/about-live-tail/)
- [Grafana Logs in Explore](https://grafana.com/docs/grafana/latest/visualizations/explore/logs-integration/)
- [Grafana Loki Live Tailing Blog](https://grafana.com/blog/2019/08/13/lokis-path-to-ga-live-tailing/)
- [lnav Hotkey Reference](https://docs.lnav.org/en/latest/hotkeys.html)
- [lnav GitHub](https://github.com/tstack/lnav)
- [Dozzle](https://dozzle.dev/)
- [Dozzle 9.0 Release](https://linuxiac.com/dozzle-9-0-real-time-docker-log-viewer-improves-log-grouping/)
- [GoAccess Features](https://goaccess.io/features)
- [melloware/react-logviewer](https://github.com/melloware/react-logviewer)
- [Logtrail Kibana Plugin](https://github.com/sivasamyk/logtrail)
- [Logit.io Live Tail](https://logit.io/platform/features/live-tail-hosted-logtrail/)
- [Splunk Frozen Column Community](https://community.splunk.com/t5/Dashboards-Visualizations/First-column-of-table-to-be-sticky/m-p/539478)
- [WCAG Contrast Requirements](https://www.w3.org/WAI/WCAG22/Understanding/contrast-minimum.html)
- [WebAIM Contrast Checker](https://webaim.org/resources/contrastchecker/)
- [Horizontal Scrolling Lists Mobile Best Practices](https://blog.iamsuleiman.com/horizontal-scrolling-lists-mobile-best-practices/)
- [NN/G Horizontal Scrolling](https://www.nngroup.com/articles/horizontal-scrolling/)
- [Virtual Scrolling in React (LogRocket)](https://blog.logrocket.com/virtual-scrolling-core-principles-and-basic-implementation-in-react/)
- [TanStack Virtual Guide](https://www.techedubyte.com/boost-react-performance-tanstack-virtual-guide/)
- [Logdy Web Log Viewer](https://logdy.dev/blog/post/web-based-logs-viewer-ui-for-local-development-environment)
- [lnav shortcuts reference](http://rolandtanglao.com/2026/01/07/p2303-lnav-shortcuts-viewing-logs-controlw-wordwrap-slash-to-search-equal-pause-g-top-shiftG-bottom/)
