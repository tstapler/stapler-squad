# History Browser Performance Improvement Plan

## Executive Summary

This plan addresses performance bottlenecks in the Claude History Browser, a web-based UI component that displays and searches through Claude conversation history. The current implementation suffers from excessive re-renders, suboptimal data loading patterns, and lack of virtualization for large history lists.

**Key Performance Issues Identified:**
1. Full list re-rendering on every state change
2. No virtualization for large history entries (500+ items)
3. Redundant API calls and cache invalidation
4. Client-side filtering of large datasets
5. Expensive useMemo computations on every render
6. Multiple useEffect dependencies causing cascade re-renders

---

## Requirements Analysis

### Functional Requirements

| ID | Requirement | Priority | Acceptance Criteria |
|----|-------------|----------|---------------------|
| FR-1 | History list displays conversation entries with metadata | Must | Entries show name, model, message count, timestamps, project |
| FR-2 | Filter entries by model, date range, and text search | Must | Filter results update within 100ms for metadata search |
| FR-3 | Full-text search across conversation content | Must | Search results display with highlighted snippets |
| FR-4 | Group entries by date, project, or model | Should | Grouping transitions smoothly without jank |
| FR-5 | Preview messages for selected entry | Must | Preview loads within 200ms of selection |
| FR-6 | Navigate entries with keyboard (j/k, arrows) | Should | Navigation feels instantaneous (<50ms response) |
| FR-7 | View all messages in modal dialog | Must | Modal opens within 100ms, scrolls smoothly |
| FR-8 | Export conversation to JSON | Could | Export completes within 5 seconds for 1000 messages |
| FR-9 | Resume session from history entry | Must | Session creation succeeds with history ID |

### Non-Functional Requirements

| ID | Requirement | Target | Current Baseline |
|----|-------------|--------|------------------|
| NFR-1 | Initial page load time | <1s for 500 entries | ~3-5s (unoptimized) |
| NFR-2 | Filter/search response time | <100ms | ~300-500ms |
| NFR-3 | Scroll performance (FPS) | 60fps sustained | Drops to ~30fps on large lists |
| NFR-4 | Memory usage | <50MB for 1000 entries | ~150MB (full DOM) |
| NFR-5 | Time to interactive | <2s | ~4-5s |
| NFR-6 | Lighthouse performance score | >80 | ~45 (estimated) |
| NFR-7 | Backend search latency | <200ms for 10k messages | ~500ms+ (full sync on each search) |

---

## Current Architecture Analysis

### Frontend Architecture (page.tsx)

\`\`\`
HistoryBrowserPage (1189 lines)
├── State Management
│   ├── 15+ useState hooks (excessive re-renders)
│   ├── 7 localStorage persistence useEffects
│   └── 6 useMemo computations (some expensive)
├── Data Loading
│   ├── loadHistory() - fetches 500 entries at once
│   ├── loadEntryDetail() - parallel fetch for entry + preview
│   └── loadMessages() - fetches all messages for modal
├── Filtering/Grouping
│   ├── filteredEntries (client-side filter on 500 items)
│   ├── groupedEntries (recomputes on filter change)
│   └── flatEntries (derived from grouped for navigation)
└── UI Components
    ├── Entry List (renders ALL 500 cards)
    ├── Detail Panel (re-renders on selection)
    └── Messages Modal (renders ALL messages)
\`\`\`

### Backend Architecture (session_service.go)

\`\`\`
SessionService
├── History Cache
│   ├── historyCache (*ClaudeSessionHistory)
│   ├── historyCacheTTL (5 minutes)
│   └── getOrRefreshHistoryCache() - mutex protected
├── Search Engine
│   ├── IncrementalSync() - syncs on every search
│   ├── BM25 scoring
│   └── Disk persistence (IndexStore)
└── Message Loading
    ├── GetMessagesFromConversationFile()
    └── Walks filesystem to find session file
\`\`\`

### Identified Bottlenecks

#### Frontend Bottlenecks

1. **No List Virtualization**
   - All 500+ entry cards rendered in DOM
   - Each card has onClick, onKeyDown handlers
   - Category groups add nesting complexity
   - Impact: ~100ms per reflow, high memory usage

2. **Cascading State Updates**
   - Filter changes trigger: filteredEntries -> groupedEntries -> flatEntries -> render
   - Each persistence useEffect runs independently
   - Selection change triggers entry detail fetch + re-render

3. **Expensive Computations**
   - \`uniqueModels\` scans all 500 entries on every render
   - \`groupedEntries\` rebuilds Map and sorts on every filter change
   - \`flatEntries\` flattens groups on every grouping change
   - \`filteredMessages\` filters on every keystroke in modal

4. **Search Mode Switching**
   - Metadata mode: client-side filter
   - Full-text mode: separate API + results component
   - Switching resets state and triggers new fetches

#### Backend Bottlenecks

1. **History Reload on Cache Miss**
   - Walks entire ~/.claude/projects/ directory
   - Parses each JSONL file for metadata
   - O(n) where n = number of conversation files

2. **IncrementalSync on Every Search**
   - Computes changes vs indexed state
   - May trigger full rebuild on metadata version mismatch
   - Holds write lock during sync

3. **Message File Discovery**
   - \`GetMessagesFromConversationFile()\` walks filesystem
   - Searches for session ID by scanning first 5 lines of each file
   - No cached file path mapping

---

## Architecture Decisions

### ADR-1: Implement Virtual Scrolling for Entry List

**Context:** The entry list renders all 500+ items in DOM, causing poor scroll performance and high memory usage.

**Decision:** Use \`@tanstack/react-virtual\` (formerly react-virtual) to virtualize the entry list.

**Rationale:**
- Renders only visible items (~10-15) plus buffer
- Maintains scroll position through data updates
- Well-tested, small bundle size (~3KB)
- Supports variable height rows (for grouped headers)

**Consequences:**
- Entry cards must have predictable heights or measure on render
- Grouped view requires flattened data structure with header rows
- Keyboard navigation needs virtual scroll integration

### ADR-2: Consolidate State with useReducer

**Context:** 15+ useState hooks cause excessive re-renders and complex dependency chains.

**Decision:** Consolidate filter/UI state into useReducer with batch updates.

**Rationale:**
- Single state update per user action
- Predictable state transitions
- Easier to debug and test
- Can batch localStorage persistence

**Consequences:**
- Refactor component to dispatch actions
- Create typed action creators
- Migration path for existing state

### ADR-3: Implement Server-Side Pagination

**Context:** Loading 500 entries at once delays initial render and wastes bandwidth.

**Decision:** Add cursor-based pagination to ListClaudeHistory RPC.

**Rationale:**
- Load 50 entries initially, fetch more on scroll
- Reduces initial payload from ~500KB to ~50KB
- Server can maintain cursor efficiently

**Consequences:**
- Backend changes to support cursor/offset
- Frontend infinite scroll implementation
- Consider impact on client-side filtering

### ADR-4: Add Session File Path Index

**Context:** \`GetMessagesFromConversationFile()\` walks filesystem to find session files.

**Decision:** Maintain in-memory index mapping session ID to file path.

**Rationale:**
- O(1) file lookup vs O(n) directory walk
- Index can be built during initial history load
- Negligible memory overhead

**Consequences:**
- Index must be updated on history reload
- Handle deleted/moved files gracefully

---

## Epic-Level Analysis

### Epic 0: Observability Infrastructure (Prerequisite)

**Objective:** Establish comprehensive OpenTelemetry instrumentation to capture metrics and traces for data-driven performance optimization decisions.

**Rationale:** Before optimizing, we need empirical data on actual bottlenecks. This epic instruments the backend to export telemetry to Datadog APM, enabling identification of real performance issues vs. theoretical concerns.

**Success Metrics:**
- Traces captured for all history-related RPC endpoints
- Database query durations tracked with operation names
- File system operation latencies measured
- P50/P95/P99 latency baselines established
- Datadog APM dashboards showing request flow

**Dependencies:** None (must be completed first)

**Risks:**
- Minor performance overhead from instrumentation (~1-2%)
- OTLP exporter configuration may vary by deployment environment


### Epic 1: Frontend List Virtualization

**Objective:** Render only visible entry cards to improve scroll performance and reduce memory usage.

**Success Metrics:**
- Scroll FPS: 30fps -> 60fps sustained
- DOM node count: ~2000 -> ~100
- Memory usage: 150MB -> 50MB
- Time to interactive: 4s -> 1.5s

**Dependencies:** None

**Risks:**
- Variable height rows in grouped view may complicate virtualization
- Keyboard navigation integration requires careful implementation

### Epic 2: State Management Optimization

**Objective:** Reduce unnecessary re-renders through consolidated state management.

**Success Metrics:**
- Re-renders per filter change: ~5 -> 1
- Time to filter 500 entries: 300ms -> 100ms
- Component update count: 50% reduction

**Dependencies:** None

**Risks:**
- Migration complexity from useState to useReducer
- Potential regression in persistence behavior

### Epic 3: Server-Side Pagination & Caching

**Objective:** Reduce initial load time and enable efficient large dataset handling.

**Success Metrics:**
- Initial payload: 500KB -> 50KB
- Time to first paint: 3s -> 1s
- Backend search latency: 500ms -> 200ms

**Dependencies:** Epic 1 (virtualization enables pagination UX)

**Risks:**
- Client-side filtering becomes impractical with pagination
- Need to handle cursor invalidation on history changes

### Epic 4: Backend Performance Optimization

**Objective:** Optimize history loading, file discovery, and search index sync.

**Success Metrics:**
- History reload time: 2s -> 500ms
- Message file lookup: 500ms -> 10ms
- Search index sync: 1s -> 100ms (incremental)

**Dependencies:** None

**Risks:**
- Index persistence format changes may require migration
- File path changes between reloads need handling

---

## Story-Level Breakdown

### Epic 0: Observability Infrastructure

#### Story 0.1: OpenTelemetry SDK Setup

**As a** developer,
**I want** OpenTelemetry tracing and metrics configured in the server,
**So that** I can capture performance data for analysis.

**Acceptance Criteria:**
- [ ] OpenTelemetry SDK initialized on server startup
- [ ] OTLP exporter configured for Datadog Agent ingestion
- [ ] Service name, version, and environment attributes set
- [ ] Graceful shutdown flushes pending telemetry
- [ ] Configuration via environment variables (OTEL_EXPORTER_OTLP_ENDPOINT, etc.)

**Story Points:** 3

#### Story 0.2: HTTP/ConnectRPC Endpoint Instrumentation

**As a** developer,
**I want** all RPC endpoints automatically instrumented,
**So that** I can see request latencies and error rates in Datadog.

**Acceptance Criteria:**
- [ ] otelconnect middleware added to ConnectRPC handlers
- [ ] HTTP server wrapped with otelhttp middleware
- [ ] Span attributes include: method, path, status code, user agent
- [ ] Error spans capture error messages
- [ ] Request/response size metrics captured

**Story Points:** 2

#### Story 0.3: Database Query Instrumentation

**As a** developer,
**I want** SQLite query durations tracked with operation context,
**So that** I can identify slow queries and N+1 patterns.

**Acceptance Criteria:**
- [ ] Custom spans created for SQLiteRepository operations
- [ ] Span names reflect operation (e.g., "sqlite.ListSessions", "sqlite.GetSession")
- [ ] Query durations recorded as span duration
- [ ] Row counts captured as span attributes
- [ ] Transaction boundaries visible in traces

**Story Points:** 3

#### Story 0.4: History Loading Instrumentation

**As a** developer,
**I want** Claude history loading operations traced,
**So that** I can identify file system bottlenecks.

**Acceptance Criteria:**
- [ ] ClaudeSessionHistory.Refresh() creates parent span
- [ ] Individual file parsing creates child spans
- [ ] File path and size captured as attributes
- [ ] Error files logged with span events
- [ ] Directory walk duration measured

**Story Points:** 3

#### Story 0.5: Search Engine Instrumentation

**As a** developer,
**I want** search engine operations traced,
**So that** I can measure index sync and query performance.

**Acceptance Criteria:**
- [ ] SearchEngine.Search() creates span with query info
- [ ] IncrementalSync() captures sync duration and change counts
- [ ] Index operations (BuildIndex, LoadIndex, SaveIndex) traced
- [ ] BM25 scoring duration captured
- [ ] Result counts and query time as span attributes

**Story Points:** 2

#### Story 0.6: Custom Metrics for Business KPIs

**As a** developer,
**I want** custom metrics for history browser operations,
**So that** I can create Datadog dashboards with business-relevant data.

**Acceptance Criteria:**
- [ ] Histogram: history_entries_count (gauge of total entries)
- [ ] Histogram: history_load_duration_ms
- [ ] Histogram: search_query_duration_ms
- [ ] Counter: search_queries_total (with result_count bucket)
- [ ] Histogram: message_file_size_bytes
- [ ] Gauge: search_index_document_count

**Story Points:** 2


### Epic 1: Frontend List Virtualization

#### Story 1.1: Extract Entry Card Component

**As a** developer,
**I want** the entry card to be a separate memoized component,
**So that** it only re-renders when its props change.

**Acceptance Criteria:**
- [ ] EntryCard component extracted from inline JSX
- [ ] Component wrapped with React.memo()
- [ ] Props interface defined with proper types
- [ ] Visual appearance unchanged
- [ ] onClick handler properly memoized

**Story Points:** 2

#### Story 1.2: Implement Virtual Scroller Container

**As a** user,
**I want** smooth 60fps scrolling through history entries,
**So that** I can navigate large history lists without lag.

**Acceptance Criteria:**
- [ ] @tanstack/react-virtual installed and configured
- [ ] Entry list renders only visible items
- [ ] Scroll position maintained through filter changes
- [ ] Grouped view renders header rows in virtual list
- [ ] Performance benchmark shows 60fps sustained scroll

**Story Points:** 5

#### Story 1.3: Integrate Keyboard Navigation with Virtual Scroll

**As a** user,
**I want** keyboard navigation (j/k, arrows) to work with virtualized list,
**So that** I can efficiently navigate without using the mouse.

**Acceptance Criteria:**
- [ ] Arrow up/down scroll selected item into view
- [ ] j/k navigation works across groups
- [ ] Focus ring visible on selected item
- [ ] Navigation feels instantaneous (<50ms)

**Story Points:** 3

### Epic 2: State Management Optimization

#### Story 2.1: Create History Browser State Reducer

**As a** developer,
**I want** consolidated state management with useReducer,
**So that** state updates are predictable and batched.

**Acceptance Criteria:**
- [ ] State interface defined with all filter/UI state
- [ ] Action types enumerated with proper typing
- [ ] Reducer function handles all state transitions
- [ ] Initial state loaded from localStorage
- [ ] Existing functionality preserved

**Story Points:** 5

#### Story 2.2: Batch localStorage Persistence

**As a** developer,
**I want** localStorage writes batched with debounce,
**So that** filter changes don't cause excessive storage writes.

**Acceptance Criteria:**
- [ ] Single useEffect handles all persistence
- [ ] 500ms debounce on state changes
- [ ] Persistence handles partial state updates
- [ ] Hydration mismatch prevented

**Story Points:** 2

#### Story 2.3: Optimize Derived State Computations

**As a** developer,
**I want** filtered/grouped entries computed efficiently,
**So that** filter changes respond within 100ms.

**Acceptance Criteria:**
- [ ] useMemo dependencies minimized to essential
- [ ] Memoization cache hits on unchanged inputs
- [ ] uniqueModels computed once per entries change
- [ ] Filtering uses early termination where possible

**Story Points:** 3

### Epic 3: Server-Side Pagination & Caching

#### Story 3.1: Add Pagination to ListClaudeHistory RPC

**As a** developer,
**I want** the history list API to support pagination,
**So that** clients can load data incrementally.

**Acceptance Criteria:**
- [ ] Cursor-based pagination added to proto definition
- [ ] Server returns page of entries with next cursor
- [ ] Limit parameter respected (default 50, max 100)
- [ ] Backwards compatible with existing clients

**Story Points:** 3

#### Story 3.2: Implement Infinite Scroll in Frontend

**As a** user,
**I want** more history entries to load as I scroll down,
**So that** I can browse all my history without waiting.

**Acceptance Criteria:**
- [ ] Initial load fetches first 50 entries
- [ ] Scroll near bottom triggers next page load
- [ ] Loading indicator shown during fetch
- [ ] All entries eventually loadable
- [ ] Virtual scroll integrates with pagination

**Story Points:** 5

#### Story 3.3: Server-Side Filtering Support

**As a** user,
**I want** filters applied server-side for accurate results,
**So that** I see correct results even with pagination.

**Acceptance Criteria:**
- [ ] Model filter passed to ListClaudeHistory
- [ ] Date filter passed to server
- [ ] Server returns filtered count
- [ ] Client-side filtering disabled when server filtering active

**Story Points:** 3

### Epic 4: Backend Performance Optimization

#### Story 4.1: Add Session File Path Index

**As a** developer,
**I want** session IDs mapped to file paths,
**So that** message loading doesn't require directory walking.

**Acceptance Criteria:**
- [ ] Index built during history reload
- [ ] O(1) lookup by session ID
- [ ] Index updated on history cache refresh
- [ ] Missing file handled gracefully

**Story Points:** 3

#### Story 4.2: Optimize History Reload with Parallel Parsing

**As a** developer,
**I want** conversation files parsed in parallel,
**So that** history reload completes faster.

**Acceptance Criteria:**
- [ ] Worker pool for file parsing (4-8 workers)
- [ ] Results aggregated thread-safely
- [ ] Reload time reduced by 3-4x
- [ ] Error handling per file

**Story Points:** 5

#### Story 4.3: Lazy Search Index Sync

**As a** developer,
**I want** search index sync deferred until needed,
**So that** history listing is faster.

**Acceptance Criteria:**
- [ ] Index sync not triggered by ListClaudeHistory
- [ ] First search triggers incremental sync
- [ ] Background sync after initial load
- [ ] Sync status exposed for UI indicator

**Story Points:** 3

---

## Atomic Task Decomposition

### Epic 0: Observability Infrastructure

#### Task 0.1.1: Add OpenTelemetry Dependencies
**Duration:** 30 minutes
**Files:** 
- `go.mod`

**Implementation:**
1. Add direct dependencies:
   ```
   go.opentelemetry.io/otel
   go.opentelemetry.io/otel/sdk
   go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
   go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc
   go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
   connectrpc.com/otelconnect
   ```
2. Run `go mod tidy`

**Verification:** Dependencies resolve, `go build` succeeds

#### Task 0.1.2: Create Telemetry Package
**Duration:** 2 hours
**Files:**
- `server/telemetry/telemetry.go` (new)
- `server/telemetry/config.go` (new)

**Implementation:**
1. Create TelemetryConfig struct with OTLP endpoint, service name, environment
2. Create InitTelemetry() function that:
   - Creates OTLP trace exporter
   - Creates OTLP metric exporter
   - Configures TracerProvider with batch processor
   - Configures MeterProvider
   - Sets global providers
   - Returns shutdown function
3. Support environment variable configuration:
   - OTEL_EXPORTER_OTLP_ENDPOINT (default: localhost:4317)
   - OTEL_SERVICE_NAME (default: claude-squad)
   - OTEL_ENVIRONMENT (default: development)

**Verification:** Telemetry initializes without error, shutdown works

#### Task 0.1.3: Integrate Telemetry into Server Startup
**Duration:** 1 hour
**Files:**
- `server/server.go`
- `main.go`

**Implementation:**
1. Call InitTelemetry() early in server startup
2. Store shutdown function for graceful cleanup
3. Call shutdown in server Close() method
4. Add startup log showing telemetry endpoint

**Verification:** Server starts with telemetry, exports to local collector

#### Task 0.2.1: Add otelconnect Middleware
**Duration:** 1 hour
**Files:**
- `server/server.go`

**Implementation:**
1. Import connectrpc.com/otelconnect
2. Create otelconnect.NewInterceptor()
3. Add interceptor to ConnectRPC handler options
4. Ensure interceptor captures both unary and streaming calls

**Verification:** Traces appear for RPC calls in Datadog

#### Task 0.2.2: Wrap HTTP Server with otelhttp
**Duration:** 1 hour
**Files:**
- `server/server.go`

**Implementation:**
1. Import go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp
2. Wrap main HTTP handler with otelhttp.NewHandler()
3. Configure span naming convention
4. Add request/response metrics

**Verification:** HTTP middleware traces visible, includes non-RPC endpoints

#### Task 0.3.1: Create Database Tracing Helper
**Duration:** 1 hour
**Files:**
- `session/sqlite_tracing.go` (new)

**Implementation:**
1. Create tracedDB struct wrapping *sql.DB
2. Implement QueryContext, ExecContext, BeginTx with span creation
3. Add span attributes: operation, table (if parseable), row_count
4. Use consistent naming: "sqlite.{operation}"

**Verification:** Database operations create child spans

#### Task 0.3.2: Instrument SQLiteRepository
**Duration:** 2 hours
**Files:**
- `session/sqlite_repository.go`

**Implementation:**
1. Accept tracer in constructor or use global
2. Wrap high-level methods (List, Get, Create, Update) with spans
3. Add span events for significant operations
4. Capture row counts in span attributes

**Verification:** Repository traces show query hierarchy

#### Task 0.4.1: Instrument History Loading
**Duration:** 2 hours
**Files:**
- `session/history.go`

**Implementation:**
1. Add tracer to ClaudeSessionHistory struct
2. Create span in Refresh() method
3. Create child spans for directory walking
4. Create child spans for file parsing
5. Add attributes: file_count, total_size, error_count

**Verification:** History loading shows file-level breakdown

#### Task 0.5.1: Instrument Search Engine
**Duration:** 1.5 hours
**Files:**
- `session/search_engine.go`

**Implementation:**
1. Add tracer to SearchEngine struct
2. Instrument Search() with query and result count
3. Instrument IncrementalSync() with change counts
4. Instrument BuildIndex() with document counts
5. Add query_time as span attribute (already computed)

**Verification:** Search operations traced with details

#### Task 0.6.1: Create Custom Metrics
**Duration:** 2 hours
**Files:**
- `server/telemetry/metrics.go` (new)

**Implementation:**
1. Define meter and instruments on init
2. Create histograms for duration measurements
3. Create counters for operation counts
4. Create gauges for current state (index size, entry count)
5. Expose RecordHistoryLoad(), RecordSearch(), etc.

**Verification:** Metrics visible in Datadog metrics explorer

#### Task 0.6.2: Integrate Metrics Recording
**Duration:** 1 hour
**Files:**
- `server/services/session_service.go`
- `session/search_engine.go`
- `session/history.go`

**Implementation:**
1. Import metrics package
2. Record durations after operations complete
3. Record counts and sizes as attributes
4. Use defer for timing measurements

**Verification:** Dashboard shows metric data


### Epic 1: Frontend List Virtualization

#### Task 1.1.1: Create EntryCard Component File
**Duration:** 1 hour
**Files:** 
- \`web-app/src/components/history/EntryCard.tsx\`
- \`web-app/src/components/history/EntryCard.module.css\`

**Implementation:**
1. Create new component file with props interface
2. Move entry card JSX from page.tsx
3. Move relevant styles to module CSS
4. Export memoized component

**Verification:** Component renders identical to inline version

#### Task 1.1.2: Integrate EntryCard into Page
**Duration:** 1 hour
**Files:**
- \`web-app/src/app/history/page.tsx\`
- \`web-app/src/components/history/index.ts\`

**Implementation:**
1. Import EntryCard component
2. Replace inline JSX with component
3. Pass selection state and handlers as props
4. Add to barrel export

**Verification:** Visual diff shows no changes

#### Task 1.2.1: Install and Configure Virtual Scroller
**Duration:** 2 hours
**Files:**
- \`web-app/package.json\`
- \`web-app/src/app/history/page.tsx\`

**Implementation:**
1. Install @tanstack/react-virtual
2. Create virtualizer with useVirtualizer hook
3. Configure item size estimator
4. Wrap entry list container with virtual scroll

**Verification:** Only ~15 DOM nodes rendered for entry list

#### Task 1.2.2: Flatten Grouped Entries for Virtualization
**Duration:** 2 hours
**Files:**
- \`web-app/src/app/history/page.tsx\`
- \`web-app/src/lib/hooks/useHistoryState.ts\` (new)

**Implementation:**
1. Create flattened item type (entry or header)
2. Compute flattened list from grouped entries
3. Render header rows as separate item type
4. Handle variable heights for headers vs entries

**Verification:** Grouped view scrolls smoothly with header rows

#### Task 1.2.3: Implement Scroll-to-Item Navigation
**Duration:** 2 hours
**Files:**
- \`web-app/src/app/history/page.tsx\`

**Implementation:**
1. Get scrollToIndex from virtualizer
2. Call on selection change
3. Handle group boundaries in navigation
4. Add smooth scroll animation

**Verification:** Keyboard navigation scrolls selected item into view

### Epic 2: State Management Optimization

#### Task 2.1.1: Define State and Action Types
**Duration:** 2 hours
**Files:**
- \`web-app/src/lib/hooks/useHistoryReducer.ts\` (new)

**Implementation:**
1. Define HistoryBrowserState interface
2. Define action type union
3. Create action creator functions
4. Add JSDoc comments

**Verification:** Types compile without errors

#### Task 2.1.2: Implement Reducer Function
**Duration:** 3 hours
**Files:**
- \`web-app/src/lib/hooks/useHistoryReducer.ts\`

**Implementation:**
1. Implement reducer with switch on action type
2. Handle all filter state transitions
3. Handle selection state
4. Handle loading/error states

**Verification:** Unit tests pass for all action types

#### Task 2.1.3: Migrate Page to useReducer
**Duration:** 3 hours
**Files:**
- \`web-app/src/app/history/page.tsx\`

**Implementation:**
1. Replace useState calls with useReducer
2. Update event handlers to dispatch
3. Update derived state computations
4. Remove redundant useEffect hooks

**Verification:** All functionality works, fewer re-renders

#### Task 2.2.1: Implement Batched Persistence Hook
**Duration:** 2 hours
**Files:**
- \`web-app/src/lib/hooks/usePersistedState.ts\` (new)

**Implementation:**
1. Create hook with debounced save
2. Handle hydration on mount
3. Handle partial state serialization
4. Cleanup on unmount

**Verification:** localStorage writes debounced, state persists

### Epic 3: Server-Side Pagination

#### Task 3.1.1: Update Proto Definition
**Duration:** 1 hour
**Files:**
- \`proto/session/v1/session.proto\`

**Implementation:**
1. Add cursor field to ListClaudeHistoryRequest
2. Add next_cursor to ListClaudeHistoryResponse
3. Add has_more boolean
4. Generate protobuf code

**Verification:** Proto compiles, Go/TS types generated

#### Task 3.1.2: Implement Pagination in Go Service
**Duration:** 3 hours
**Files:**
- \`server/services/session_service.go\`

**Implementation:**
1. Parse cursor from request
2. Slice entries based on cursor position
3. Generate next cursor from last entry
4. Return has_more flag

**Verification:** API returns paginated results

#### Task 3.2.1: Add Pagination State to Frontend
**Duration:** 2 hours
**Files:**
- \`web-app/src/app/history/page.tsx\`
- \`web-app/src/lib/hooks/useHistoryReducer.ts\`

**Implementation:**
1. Add cursor and hasMore to state
2. Add LOAD_MORE action type
3. Append new entries to existing
4. Update hasMore from response

**Verification:** State tracks pagination correctly

#### Task 3.2.2: Implement Intersection Observer for Load More
**Duration:** 2 hours
**Files:**
- \`web-app/src/app/history/page.tsx\`

**Implementation:**
1. Create ref for sentinel element
2. Set up IntersectionObserver
3. Trigger loadMore on intersection
4. Handle loading state

**Verification:** Scrolling near bottom loads more entries

### Epic 4: Backend Performance

#### Task 4.1.1: Add File Path to History Entry
**Duration:** 2 hours
**Files:**
- \`session/history.go\`

**Implementation:**
1. Add FilePath field to ClaudeHistoryEntry
2. Store file path during parseConversationFile
3. Index file paths with session IDs
4. Use cached path in GetMessagesFromConversationFile

**Verification:** Message loading uses direct file path

#### Task 4.2.1: Implement Parallel File Parsing
**Duration:** 3 hours
**Files:**
- \`session/history.go\`

**Implementation:**
1. Create bounded worker pool (runtime.NumCPU())
2. Dispatch file paths to workers
3. Aggregate results with mutex
4. Handle per-file errors

**Verification:** History reload 3x faster

---

## Testing Strategy

### Testing for Epic 0: Observability Infrastructure

| Component | Test Cases | Coverage Target |
|-----------|------------|-----------------|
| telemetry.InitTelemetry() | Init, shutdown, config | 90% |
| otelconnect middleware | RPC tracing | Integration |
| SQLite tracing | Query spans | 80% |
| Search instrumentation | Search spans | 80% |
| Custom metrics | Record functions | 85% |

**Integration Testing:**

1. **Local OTEL Collector Test:**
   - Run `otel-collector` locally with debug exporter
   - Verify spans and metrics exported correctly
   - Check span hierarchy (parent-child relationships)

2. **Datadog Agent Test:**
   - Configure Datadog Agent with OTLP ingestion
   - Verify traces appear in Datadog APM
   - Verify metrics appear in Datadog Metrics Explorer

**Configuration Reference:**

```bash
# OTLP Endpoint (Datadog Agent default)
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317

# Service identification
OTEL_SERVICE_NAME=claude-squad
OTEL_SERVICE_VERSION=1.0.0

# Environment tagging
OTEL_RESOURCE_ATTRIBUTES=deployment.environment=production,service.namespace=claude

# Sampling (optional, for high-volume)
OTEL_TRACES_SAMPLER=parentbased_traceidratio
OTEL_TRACES_SAMPLER_ARG=0.1
```

**Datadog Agent Configuration (datadog.yaml):**

```yaml
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
```


### Unit Tests

| Component | Test Cases | Coverage Target |
|-----------|------------|-----------------|
| useHistoryReducer | All action types, edge cases | 90% |
| EntryCard | Render, click, keyboard | 80% |
| usePersistedState | Save, load, debounce | 85% |
| Virtual scroll integration | Navigation, scroll | 75% |

### Integration Tests

| Scenario | Test Method |
|----------|-------------|
| Filter + paginate | Playwright E2E |
| Keyboard navigation | Playwright E2E |
| Full-text search | API integration |
| History reload performance | Go benchmark |

### Performance Tests

| Metric | Test Method | Target |
|--------|-------------|--------|
| Scroll FPS | Chrome DevTools | 60fps |
| Memory usage | Chrome DevTools | <50MB |
| Initial load | Lighthouse | <1.5s |
| Filter response | React Profiler | <100ms |

---

## Known Issues

### Potential Bug: Virtual Scroll Height Calculation

**Description:** Variable height entry cards (with/without project path) may cause incorrect scroll height calculation.

**Mitigation:**
- Use \`estimateSize\` callback with average height
- Implement \`measureElement\` for exact measurements
- Add CSS to ensure consistent card heights

**Files Likely Affected:**
- \`web-app/src/app/history/page.tsx\`

**Prevention Strategy:**
- Use fixed heights for cards OR
- Measure on render with measureElement ref

### Potential Bug: Stale Cursor on Filter Change

**Description:** Pagination cursor may become invalid if history changes between page loads.

**Mitigation:**
- Reset cursor on filter change
- Handle 404 gracefully by restarting from first page
- Consider offset-based fallback

**Files Likely Affected:**
- \`server/services/session_service.go\`
- \`web-app/src/app/history/page.tsx\`

**Prevention Strategy:**
- Cursor encodes timestamp + offset for resilience
- Server validates cursor and returns error if invalid

### Potential Bug: Race Condition in Parallel File Parsing

**Description:** Concurrent access to shared entries slice during parallel file parsing.

**Mitigation:**
- Use mutex-protected append
- Or use channel to collect results
- Ensure proper synchronization

**Files Likely Affected:**
- \`session/history.go\`

**Prevention Strategy:**
- Run with \`-race\` flag during testing
- Use sync.Mutex for shared state
- Benchmark with concurrent load

### Potential Bug: Memory Leak in Intersection Observer

**Description:** IntersectionObserver not disconnected on unmount could leak.

**Mitigation:**
- Cleanup observer in useEffect return
- Use ref for observer instance
- Check for already disconnected state

**Files Likely Affected:**
- \`web-app/src/app/history/page.tsx\`

**Prevention Strategy:**
- Test component mount/unmount cycles
- Verify no memory growth in DevTools

### Potential Bug: Hydration Mismatch with Server Components

**Description:** localStorage values read on client may differ from server-rendered defaults.

**Mitigation:**
- Use \`useEffect\` for client-only localStorage reads (already done)
- Ensure initial state matches server render
- Add hydration flag to delay persistence reads

**Files Likely Affected:**
- \`web-app/src/app/history/page.tsx\`

**Prevention Strategy:**
- Test with SSR enabled
- Verify no hydration warnings in console

---

## Implementation Phases

### Phase 0: Observability Infrastructure (2-3 days) [NEW - PREREQUISITE]
- OpenTelemetry SDK setup (Task 0.1.1-0.1.3)
- ConnectRPC/HTTP instrumentation (Task 0.2.1-0.2.2)
- Database and service instrumentation (Task 0.3.1-0.5.1)
- Custom metrics (Task 0.6.1-0.6.2)

**Value:** Data-driven optimization decisions, baseline metrics established, Datadog APM integration

### Phase 1: Quick Wins (1-2 days)
- Extract EntryCard component (Task 1.1.1, 1.1.2)
- Define reducer types (Task 2.1.1)
- Add file path index (Task 4.1.1)

**Value:** Foundation for larger changes, 20% performance improvement

### Phase 2: Virtual Scrolling (2-3 days)
- Install and configure virtual scroller (Task 1.2.1)
- Flatten grouped entries (Task 1.2.2)
- Integrate keyboard navigation (Task 1.2.3)

**Value:** 60fps scroll, 70% memory reduction

### Phase 3: State Optimization (2 days)
- Implement reducer (Task 2.1.2)
- Migrate page to useReducer (Task 2.1.3)
- Batched persistence (Task 2.2.1)

**Value:** 50% fewer re-renders, cleaner code

### Phase 4: Pagination (3 days)
- Proto changes (Task 3.1.1)
- Server pagination (Task 3.1.2)
- Frontend infinite scroll (Task 3.2.1, 3.2.2)

**Value:** 80% faster initial load

### Phase 5: Backend Optimization (2 days)
- Parallel file parsing (Task 4.2.1)
- Lazy search index sync (Task 4.3.1)

**Value:** 3x faster history reload

---

## Success Metrics

| Metric | Baseline | Target | Measurement Method |
|--------|----------|--------|-------------------|
| **Observability coverage** | 0% | 100% | Trace completeness |
| **Trace latency overhead** | N/A | <5ms | Benchmark comparison |
| Initial load time | 3-5s | <1.5s | Lighthouse + **APM** |
| Scroll FPS | 30fps | 60fps | Chrome DevTools |
| Memory usage | 150MB | <50MB | Chrome DevTools |
| Filter response | 300ms | <100ms | React Profiler |
| Re-renders per action | 5+ | 1-2 | React DevTools |
| DOM node count | ~2000 | <200 | Chrome DevTools |
| Backend search latency | 500ms | <200ms | **APM P95 latency** |
| History reload | 2s | <500ms | **APM trace duration** |

---

## References

### OpenTelemetry & Observability
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/) - Official documentation
- [OTLP Exporter Configuration](https://opentelemetry.io/docs/languages/sdk-configuration/otlp-exporter/) - Environment variables
- [Datadog OTLP Ingestion](https://docs.datadoghq.com/opentelemetry/setup/otlp_ingest_in_the_agent/) - Agent setup
- [otelconnect](https://pkg.go.dev/connectrpc.com/otelconnect) - ConnectRPC instrumentation
- [otelhttp](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) - HTTP instrumentation

### Frontend Performance
- [React Virtual](https://tanstack.com/virtual/latest) - List virtualization
- [React Profiler](https://react.dev/reference/react/Profiler) - Performance measurement
- [Lighthouse](https://developer.chrome.com/docs/lighthouse/) - Web performance auditing
- [Chrome DevTools Performance](https://developer.chrome.com/docs/devtools/performance/) - Runtime analysis
