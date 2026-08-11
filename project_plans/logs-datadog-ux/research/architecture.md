# Architecture Research: Datadog-Like Log Viewer

Date: 2026-05-14
Author: Architecture Research Agent

---

## 1. Current Log Data Flow

### Application Logs (`~/.stapler-squad/logs/stapler-squad.log`)

```
stapler-squad.log (disk)
        │
        ▼
  UtilityService.GetLogs()           server/services/utility_service.go
  - opens log file, reads all lines
  - applies regex parse: level/timestamp/source/message
  - server-side filters: level, time range, search query, offset/limit
  - returns LogEntry[] (most recent first)
        │
        ▼  ConnectRPC unary RPC  (GetLogs)
        │
        ▼
  web-app/src/app/logs/page.tsx
  - calls getLogs({ searchQuery, level, limit, offset, startTime, endTime })
  - stores LogEntry[] in useState
  - re-fetches on filter change (debouncedSearchQuery, levelFilters, timeRange)
  - useLiveTail hook polls via setInterval (2-second default) for live tail
  - infinite-scroll triggers loadMoreLogs via DOM scroll listener
  - client-side multi-level filter (server only supports one level filter)
        │
        ▼
  <table> render in page.tsx
  - flat HTML table, no virtualization
  - rows wrap on long lines (no horizontal scroll per row)
  - expand row → shows logDetail section in a colSpan=6 row below
```

### Session Logs (per-session scrollback / tmux output)

```
tmux PTY (live session)
        │
        ▼
  session/instance.go
  - StartControlMode() → tmux control mode stream
  - SubscribeControlModeUpdates() → channel of raw PTY bytes
        │
        ▼  WebSocket (binary frames, raw PTY / SSP protocol)
        │
  useTerminalStream.ts hook
  - connects to /ws/stream?session_id=... (or /ws/external)
  - receives: scrollback snapshot, live PTY bytes, resize acks
  - feeds bytes into TerminalStreamManager → XtermTerminal
        │
        ▼
  XtermTerminal component (xterm.js)
  - canvas-rendered, not HTML rows
  - this is NOT a structured log viewer — it's a raw terminal emulator
```

### Session "Logs" Tab (structured logs for a session)

```
~/.stapler-squad/logs/<session-title>.log (per-session log file)
        │
        ▼
  UtilityService.GetLogs() with session_id field
  - resolves session title from ID via reviewQueuePoller.FindInstance()
  - calls log.GetSessionLogFilePath() to locate per-session file
  - same parseLogs() pipeline as application logs
        │
        ▼  ConnectRPC unary RPC  (GetLogs)
        │
        ▼
  SessionLogsTab component   web-app/src/components/sessions/SessionLogsTab.tsx
  - simpler version of logs page: search bar + level multi-select + live tail
  - renders as <table> (no virtualization, no horizontal scroll)
  - embedded in SessionDetailView as tab "logs"
  - referenced from SessionDetail.tsx → SessionDetailView.tsx line 529
```

---

## 2. Existing RPC/Component Inventory

### Backend RPCs Relevant to Logs

| RPC | Location | Notes |
|-----|----------|-------|
| `GetLogs` | `server/services/session_service.go` → delegates to `utility_service.go` | Unary, not streaming. Reads entire file each call. Supports: level, search_query, start_time, end_time, limit, offset, session_id |
| `StreamTerminal` (bidirectional) | `server/services/session_service.go` | Raw PTY streaming for xterm.js terminal — NOT structured logs |
| `GetTerminalSnapshot` | `server/services/session_service.go` | Returns last N lines of tmux capture-pane output — raw ANSI, not structured |
| `LogClientEvents` | `server/services/session_service.go` | Receives browser console logs from mobile debug stream — unary, fire-and-forget |

**No streaming/watch RPC exists for logs.** `GetLogs` is a unary snapshot — it reads the whole file on each call. There is no `WatchLogs`, `StreamLogs`, or server-push equivalent.

### Frontend Components Relevant to Logs

| Component/File | Purpose |
|----------------|---------|
| `web-app/src/app/logs/page.tsx` | Application logs full-page viewer (current) |
| `web-app/src/app/logs/page.css.ts` | Styles for above (vanilla-extract) |
| `web-app/src/components/sessions/SessionLogsTab.tsx` | Per-session logs tab (embedded in session detail) |
| `web-app/src/components/sessions/SessionLogsTab.css.ts` | Styles for above |
| `web-app/src/components/sessions/TerminalOutput.tsx` | xterm.js terminal (NOT a log viewer) |
| `web-app/src/components/logs/DensityToggle.tsx` | Row density toggle widget (already exists) |
| `web-app/src/components/logs/ExportButton.tsx` | Log export button (already exists) |
| `web-app/src/components/logs/FilterPill.tsx` | Filter chip display (already exists) |
| `web-app/src/components/logs/SearchWithHistory.tsx` | Search input with history (already exists) |
| `web-app/src/components/logs/TimeRangePicker.tsx` | Time range selector (already exists) |
| `web-app/src/components/shared/LiveTailToggle.tsx` | Live tail enable/pause/interval widget |
| `web-app/src/lib/hooks/useLiveTail.ts` | Live tail polling hook (interval-based, not streaming) |

### Key Architectural Limitations of Current UI

1. **No virtual scrolling**: Both `page.tsx` and `SessionLogsTab` render all log rows into the DOM as `<tr>` elements. With 10k+ lines this causes severe render performance degradation.
2. **Lines wrap**: The `message` CSS column uses `wordBreak: "break-word"` — no horizontal scroll, no nowrap.
3. **No inline search highlight**: Search filtering re-fetches from server; there is no client-side highlight of matched terms within rows.
4. **Live tail is polling, not streaming**: `useLiveTail` runs `setInterval` and calls `fetchLogs()` (full page re-fetch) on each tick. There is no server-push or WebSocket tail.
5. **Multi-level filter is client-side only**: The backend `GetLogs` accepts a single `level` string. Multi-level filtering is applied client-side on the already-paginated response, which can silently miss entries.
6. **No "Jump to Latest" UX**: The page scrolls via a standard `<div>` overflow; there is no pause-on-scroll-up / resume-on-scroll-to-bottom logic.
7. **Log level coloring is per-cell only**: No full-row background tint based on level.

---

## 3. Recommended Architecture for the New Log Viewer

### Design Principle

Both the application logs page and the session logs tab share 95% of their logic. The recommended approach is to build **one shared `LogViewer` component** that accepts a `source` prop (`"app"` | `"session"`) and encapsulates all Datadog-like behavior. Both pages/tabs become thin wrappers.

### Component Tree

```
LogViewer (new — shared component)
  props: { source: "app" | "session"; sessionId?: string; baseUrl: string }
  │
  ├─ LogViewerToolbar (new)
  │    ├─ SearchBar (inline highlight, match count)
  │    ├─ LevelFilterChips (ALL / ERROR / WARN / INFO / DEBUG)
  │    ├─ TimeRangePicker (existing, app logs only)
  │    ├─ LiveTailToggle (existing)
  │    └─ ExportButton (existing)
  │
  ├─ ActiveFilterPills (existing FilterPills — app logs only)
  │
  ├─ JumpToLatestButton (new — sticky, shown when not at bottom)
  │
  └─ VirtualLogList (new — replaces <table>)
       ├─ LogRow (new)
       │    ├─ [gutter] line number — sticky
       │    ├─ [badge] level badge — sticky (no horizontal scroll)
       │    ├─ timestamp — scrolls horizontally
       │    └─ message — no-wrap, scrolls horizontally
       └─ ExpandedLogDetail (new — accordion panel below row)
```

### Placement

| Surface | Change |
|---------|--------|
| `web-app/src/app/logs/page.tsx` | Replace `<table>` rendering + filter logic with `<LogViewer source="app" />` |
| `web-app/src/components/sessions/SessionLogsTab.tsx` | Replace contents with `<LogViewer source="session" sessionId={sessionId} />` |
| `web-app/src/components/logs/LogViewer.tsx` | New shared component (primary implementation target) |
| `web-app/src/components/logs/LogViewer.css.ts` | New vanilla-extract styles |
| `web-app/src/components/logs/VirtualLogList.tsx` | New virtual scroll container |
| `web-app/src/components/logs/LogRow.tsx` | New row component |
| `web-app/src/components/logs/LogRow.css.ts` | New row styles |

---

## 4. Virtual Scrolling Approach

**No new library needed.** The project does not currently have `react-window` or `@tanstack/react-virtual`. Rather than adding a dependency, implement a lightweight CSS-only + JS virtual scroller:

- Maintain a fixed-height container (`height: 100%`, `overflow-y: auto`).
- Calculate `rowHeight` from a single rendered row (constant or density-dependent: 28px compact / 36px comfortable / 48px spacious).
- Use `totalHeight = logs.length * rowHeight` as a spacer div height.
- Track `scrollTop` via `onScroll` and compute `visibleStartIndex = Math.floor(scrollTop / rowHeight)`.
- Render only `visibleWindowSize = Math.ceil(containerHeight / rowHeight) + overscan` rows (overscan = 5).
- Use `transform: translateY(visibleStartIndex * rowHeight)` on the rendered rows.

This handles 10k+ lines with zero extra dependencies and renders < 50 DOM rows at once.

**Alternative**: Add `@tanstack/react-virtual` (lightweight, well-maintained). Decision should be recorded in an ADR. The CSS-only approach is safer given bundle size constraints in `package.json` (5 MB total JS budget).

---

## 5. State Management Approach

**Use local `useState` within `LogViewer`** — not Redux or context.

Rationale:
- Log data is ephemeral and not shared across components.
- The existing `page.tsx` and `SessionLogsTab` both already use local `useState`.
- Redux (`sessionsSlice`) is used for session list state that many components share — logs are local to one viewer.
- A `useLogViewer` custom hook should extract the data-fetching logic (getLogs, pagination, live tail) from the component, keeping JSX clean.

State model inside `useLogViewer`:
```ts
interface LogViewerState {
  logs: LogEntry[];           // all fetched entries
  loading: boolean;
  error: string | null;
  offset: number;
  hasMore: boolean;
  totalCount: number;
  searchQuery: string;        // raw input
  levelFilters: string[];     // active level chips
  timeRange: TimeRange;       // app logs only
  liveTailEnabled: boolean;
  liveTailPaused: boolean;
  expandedRowIndex: number | null;
  // Virtual scroll
  scrollTop: number;
  containerHeight: number;
}
```

**Client-side search** (for FR-3 highlight + match count) should be a derived computation over the `logs` array, not a separate fetch. Filter the `logs[]` array in-memory with `String.includes()` for the visible window. For 10k lines this runs in < 5ms — well within the 100ms NFR-1 budget.

---

## 6. Live Tail Architecture

The current `useLiveTail` hook uses `setInterval` polling. This is sufficient for the requirements — no streaming RPC is needed.

**Pause-on-scroll-up behavior (FR-1)** requires:
1. An `isAtBottom` flag computed in the virtual scroll `onScroll` handler: `isAtBottom = (scrollTop + containerHeight) >= (totalHeight - 10px)`.
2. When `!isAtBottom && liveTailEnabled`, set `liveTailPaused = true` and show "Jump to Latest" button.
3. When user clicks "Jump to Latest" or scrolls to bottom, set `liveTailPaused = false` and scroll to bottom.

This is a **frontend-only change** — no backend modification required.

---

## 7. Backend Changes Required

### For Application Logs

**No new RPC is required.** The existing `GetLogs` unary RPC already supports:
- `session_id` (empty = application log)
- `search_query`, `level`, `start_time`, `end_time`, `limit`, `offset`

**One backend improvement is recommended (optional):**
- Add multi-level filter support: change `level` field from `string` to `repeated string` in `GetLogsRequest`.
- Current workaround: client-side filter after fetch (which misses entries beyond `limit`).
- This requires: proto change → `make generate-proto` → update `parseLogs()` in `utility_service.go`.
- NFR-3 says new endpoints are "acceptable if required for significant UX improvement." Multi-level filter is a clear UX improvement; this is a field addition to an existing message, not a new endpoint.

**For true streaming live tail (optional, not required for MVP):**
- A `WatchLogs(WatchLogsRequest) returns (stream LogEntry)` server-streaming RPC could replace the polling approach.
- Backend would use `fsnotify` or periodic `os.ReadAt` with offset tracking to tail the file.
- This is **not required** for the MVP — polling every 2-3 seconds is acceptable for a log viewer.
- File if desired: `server/services/utility_service.go` + proto changes.

### For Session Logs

**No backend changes required.** `GetLogs` with `session_id` already works and is used by `SessionLogsTab`.

---

## 8. Files That Need to Change

### New Files (create)

| File | Purpose |
|------|---------|
| `web-app/src/components/logs/LogViewer.tsx` | Shared log viewer component |
| `web-app/src/components/logs/LogViewer.css.ts` | Vanilla-extract styles |
| `web-app/src/components/logs/VirtualLogList.tsx` | Virtual scroll container |
| `web-app/src/components/logs/VirtualLogList.css.ts` | Virtual scroll styles |
| `web-app/src/components/logs/LogRow.tsx` | Single log row (sticky gutter + horizontal scroll) |
| `web-app/src/components/logs/LogRow.css.ts` | Log row styles |
| `web-app/src/components/logs/JumpToLatestButton.tsx` | Sticky "Jump to Latest" button |
| `web-app/src/lib/hooks/useLogViewer.ts` | Data fetching + filter + live tail hook |
| `web-app/src/lib/logs/logParser.ts` | Client-side log level detection, search highlight, JSON pretty-print |

### Modified Files

| File | Change |
|------|--------|
| `web-app/src/app/logs/page.tsx` | Replace table + filter logic with `<LogViewer source="app" />` |
| `web-app/src/app/logs/page.css.ts` | Simplify — most styles move to LogViewer |
| `web-app/src/components/sessions/SessionLogsTab.tsx` | Replace with `<LogViewer source="session" sessionId={sessionId} />` |
| `web-app/src/components/sessions/SessionLogsTab.css.ts` | Can be deleted or gutted |
| `web-app/src/components/logs/index.ts` | Export new components |
| `proto/session/v1/session.proto` | Add `repeated string levels` to `GetLogsRequest` (optional, for multi-level filter) |
| `server/services/utility_service.go` | Update `parseLogs()` for multi-level filter if proto changes |
| `docs/registry/frontend-features.json` | Add log-viewer feature entry |
| `docs/registry/backend-features.json` | Update logs entry with `tested: true` when tests added |

### Unchanged Files (no modification needed)

- `server/services/session_service.go` — `GetLogs` delegation stays as-is
- `web-app/src/lib/hooks/useLiveTail.ts` — reused as-is inside `useLogViewer`
- `web-app/src/components/shared/LiveTailToggle.tsx` — reused as-is
- `web-app/src/components/logs/DensityToggle.tsx` — reused as-is
- `web-app/src/components/logs/ExportButton.tsx` — reused as-is
- `web-app/src/components/logs/FilterPill.tsx` — reused as-is
- `web-app/src/components/logs/SearchWithHistory.tsx` — reused as-is
- `web-app/src/components/logs/TimeRangePicker.tsx` — reused as-is
- All terminal streaming code (xterm.js / useTerminalStream) — separate concern

---

## 9. Key Design Decisions

### D1: Client-side search vs. server-side re-fetch

**Decision**: Client-side search against the in-memory `logs[]` array.

Rationale: Server already returns up to 1000 entries per fetch. Filtering 10k lines client-side runs in < 5ms (plain `String.includes` scan). This enables instant highlight as the user types without round-trips, satisfying NFR-1 (< 100ms). The server filter (`search_query`) is still used for the initial load and live tail refresh to keep the dataset bounded — the two mechanisms complement each other.

### D2: Virtual scroll implementation

**Decision**: Custom lightweight virtual scroller (no new library), fixed row height per density setting.

Rationale: Existing bundle size budget is tight (5 MB). Adding `@tanstack/react-virtual` (~15 KB gzipped) is feasible but should be recorded as an ADR. The custom approach is 50 lines of code for fixed-height rows and eliminates any external dependency risk.

### D3: Horizontal scroll for log lines

**Decision**: `white-space: nowrap` on the message cell inside a horizontally-scrolling container. Timestamp and level badge use `position: sticky; left: 0` with a solid background to stay fixed during horizontal scroll.

Rationale: This matches Datadog's layout and the FR-2 requirement. The sticky gutter pattern is CSS-only, no JS needed.

### D4: No new streaming RPC for MVP

**Decision**: Use existing polling-based `useLiveTail` hook with `GetLogs` unary RPC.

Rationale: NFR-3 prefers no new endpoints. Polling every 2-3 seconds is indistinguishable from streaming for a log viewer. A streaming `WatchLogs` RPC can be added in a future iteration if users need sub-second latency.
