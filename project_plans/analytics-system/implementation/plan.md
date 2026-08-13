# Analytics System — Implementation Plan

## Overview

6 epics, 18 stories, 72 tasks. Implement the full analytics platform in dependency order: storage first, then server-side provider interface, then the frontend adapter layer, then the ESLint plugin, and finally upgrade the existing telemetry callsites.

ADR candidates are flagged inline with `[ADR-NEEDED]`.

---

## ADR Candidates

| ID | Decision |
|----|----------|
| ADR-010 | Separate `analytics.db` SQLite file (vs. sharing `sessions.db`) to prevent write contention |
| ADR-011 | Client-side batching in `HttpAnalyticsProvider` (25 events / 2 s flush) as the primary rate-control mechanism |
| ADR-012 | Local ESLint plugin (`eslint-plugin-analytics`) as a `file:` workspace package rather than inline `no-restricted-syntax` rules |
| ADR-013 | ent ORM for `AnalyticsEvent` (vs. raw SQLite) — consistency with existing schema management |

---

## Epic 1: Backend Storage — `AnalyticsEvent` ent entity + separate DB

**Goal**: Create the `AnalyticsEvent` ent schema, generate all ORM code, open a dedicated `analytics.db` connection, and implement the `SQLiteAnalyticsProvider` that writes to it. No HTTP surface yet; this epic is pure storage.

**[ADR-010]** Use `analytics.db` (not `sessions.db`) to avoid single-writer contention on the shared ent client (`MaxOpenConns(1)`).

### Story 1.1: Define `AnalyticsEvent` ent schema

**Files to create**:
- `session/ent/schema/analytics_event.go`

**Tasks**:

1. **Create `session/ent/schema/analytics_event.go`**
   - Define `AnalyticsEvent struct{ ent.Schema }` following the pattern in `session/ent/schema/error_event.go`
   - Fields: `id` (string UUID, caller-supplied, unique, not empty), `event_name` (string, not empty), `event_category` (string, not empty — values: `user_action`, `performance`, `navigation`, `rpc`), `session_id` (string, optional), `duration_ms` (int64, optional, nillable), `page` (string, optional), `component` (string, optional), `labels` (`field.JSON("labels", map[string]string{})`, optional), `created_at` (time, default `time.Now`, immutable)
   - Indexes: single-field indexes on `event_name`, `event_category`, `session_id`, `created_at`; plus composite index on `("event_name", "created_at")` for summary queries
   - Use `field.String("event_category")` (not `field.Enum(...)`) to avoid needing a migration when adding categories later — validate at the service layer
   - Import `"time"` and `"entgo.io/ent/schema/index"` following existing schema imports

2. **Run ent code generation**
   - Command (run from repo root): `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
   - Verify generated files appear: `session/ent/analytics_event.go`, `session/ent/analytics_event_create.go`, `session/ent/analytics_event_query.go`, `session/ent/analytics_event_update.go`, `session/ent/analytics_event_delete.go`, `session/ent/analytics_event_client.go` (and internal predicate files)
   - Run `make build` to confirm no compile errors

3. **Commit all generated files together**
   - Stage everything under `session/ent/` in one commit — partial commits break the build

**Expected test file**: `session/ent/schema/analytics_event_test.go` (schema validation — see Story 1.3)

---

### Story 1.2: Open a dedicated `analytics.db` ent client

**Files to modify**:
- `server/dependencies.go` (or wherever `ServerDependencies` is defined — likely `server/server.go`)

**Files to create**:
- `server/analytics/db.go`

**Tasks**:

1. **Create `server/analytics/db.go`** — package `analytics`
   - Function signature: `func OpenAnalyticsDB(ctx context.Context, dataDir string) (*ent.Client, error)`
   - Opens `filepath.Join(dataDir, "analytics.db")` using the same DSN pattern as the existing ent client: `"file:<path>?_journal_mode=WAL&_synchronous=NORMAL&_foreign_keys=on"`
   - Sets `db.SetMaxOpenConns(1)` on the underlying `*sql.DB` to enforce SQLite single-writer semantics
   - Calls `client.Schema.Create(ctx)` at startup to auto-migrate the `AnalyticsEvent` table
   - Returns the `*ent.Client`; caller is responsible for `defer client.Close()`

2. **Add `AnalyticsEntClient *ent.Client` field to `ServerDependencies`**
   - Modify the `ServerDependencies` struct (in `server/dependencies.go` or `server/server.go`)
   - Initialize it in the function that builds `ServerDependencies`, calling `analytics.OpenAnalyticsDB(ctx, cfg.DataDir())`
   - Close it in the server shutdown path alongside the existing ent client

**Expected test file**: `server/analytics/db_test.go` (opens DB in temp dir, verifies `AnalyticsEvent` table is created)

---

### Story 1.3: Implement `SQLiteAnalyticsProvider` (Go)

**Files to create**:
- `server/analytics/provider.go` — Go interface definition
- `server/analytics/sqlite_provider.go` — SQLite implementation
- `server/analytics/log_provider.go` — log-only implementation
- `server/analytics/retention.go` — retention enforcement goroutine

**Tasks**:

1. **Define Go `AnalyticsProvider` interface in `server/analytics/provider.go`**
   ```go
   package analytics

   import "context"

   type Event struct {
       ID           string
       EventName    string
       EventCategory string
       SessionID    string
       DurationMs   *int64
       Page         string
       Component    string
       Labels       map[string]string
   }

   type AnalyticsProvider interface {
       Record(ctx context.Context, event Event) error
   }
   ```

2. **Implement `SQLiteAnalyticsProvider` in `server/analytics/sqlite_provider.go`**
   - Struct: `SQLiteAnalyticsProvider { client *ent.Client }`
   - Constructor: `NewSQLiteAnalyticsProvider(client *ent.Client) *SQLiteAnalyticsProvider`
   - Method: `Record(ctx context.Context, event Event) error`
     - Uses `client.AnalyticsEvent.Create()` to insert the event
     - Generates a UUID if `event.ID` is empty (`github.com/google/uuid`)
     - Sets all fields from `event`; skips nil-optional fields

3. **Implement `LogAnalyticsProvider` in `server/analytics/log_provider.go`**
   - Struct with no dependencies; `Record` logs via `log.InfoLog.Printf` and returns nil
   - Used in tests and as a fallback if the DB fails to open

4. **Implement retention enforcement in `server/analytics/retention.go`**
   - Function: `StartRetentionEnforcer(ctx context.Context, client *ent.Client, maxRows int, maxAgeDays int)`
   - Runs a goroutine with a 1-hour ticker
   - On each tick, deletes rows older than `maxAgeDays` days; then if row count still exceeds `maxRows`, deletes oldest rows until count is within limit
   - Use ent's `client.AnalyticsEvent.Delete().Where(...)` predicates
   - Default values: `maxRows=100_000`, `maxAgeDays=90` (configurable from `config.json` — see Story 2.2)

**Expected test files**:
- `server/analytics/sqlite_provider_test.go` — integration test: `Record` inserts row, query confirms it
- `server/analytics/retention_test.go` — inserts rows exceeding limit, runs enforcer, verifies deletion

---

## Epic 2: Backend HTTP Handler + Analytics Go Provider Wiring

**Goal**: Expose `POST /api/analytics` (batch ingest) and `GET /api/analytics/summary` (aggregation). Wire the `SQLiteAnalyticsProvider` into `wireDepsIntoServer`, upgrade `TelemetryHandler` to forward events, and add the EventBus analytics subscriber.

**[ADR-011]** The HTTP endpoint accepts a batch body (`{ "events": [...] }`) — one HTTP call per flush — because the client-side `HttpAnalyticsProvider` batches before sending.

### Story 2.1: Implement `AnalyticsHTTPHandler` with two routes

**Files to create**:
- `server/handlers/analytics_handler.go`
- `server/handlers/analytics_handler_test.go`

**Tasks**:

1. **Define request/response types in `analytics_handler.go`**
   - `analyticsEventRequest` struct (matches frontend `AnalyticsEvent` shape):
     ```go
     type analyticsEventRequest struct {
         ID           string            `json:"id,omitempty"`
         EventName    string            `json:"name"`
         EventCategory string           `json:"category"`
         DurationMs   *int64            `json:"duration_ms,omitempty"`
         SessionID    string            `json:"session_id,omitempty"`
         Page         string            `json:"page,omitempty"`
         Component    string            `json:"component,omitempty"`
         Labels       map[string]string `json:"labels,omitempty"`
     }
     type analyticsBatchRequest struct {
         Events []analyticsEventRequest `json:"events"`
     }
     ```
   - `analyticsSummaryResponse` struct (matches FR-5 JSON shape)

2. **Implement `AnalyticsHandler` struct**
   - Fields: `provider analytics.AnalyticsProvider`, `limiter *rateLimiter` (new instance, cap 1000/min — `[ADR-011]`)
   - Constructor: `NewAnalyticsHandler(provider analytics.AnalyticsProvider) *AnalyticsHandler`
   - Method: `RegisterRoutes(mux *http.ServeMux)`
     - `mux.HandleFunc("POST /api/analytics", h.HandlePost)`
     - `mux.HandleFunc("GET /api/analytics/summary", h.HandleSummary)`

3. **Implement `HandlePost`**
   - Check rate limiter (1000/min); return 429 if exceeded
   - Cap body to 512 KB (`http.MaxBytesReader`)
   - Decode `analyticsBatchRequest`
   - Validate: `len(events) == 0` → 400; `len(events) > 100` → 400 ("batch too large")
   - Validate each event: `name` required, `category` must be one of the four valid values
   - Sanitize `event_name` (strip `\n`/`\r`) following existing telemetry handler pattern
   - Call `h.provider.Record(ctx, event)` for each event; log errors but continue (don't fail the batch on a single row error)
   - Respond 204 on success

4. **Implement `HandleSummary`**
   - Parse query params: `from` (ISO 8601), `to` (ISO 8601), `category` (optional filter)
   - Default `from`: 7 days ago; default `to`: now
   - Query `AnalyticsEvent` rows in the time window using ent predicates; load into memory and aggregate in Go:
     - Top events: count by `event_name`, with average `duration_ms`
     - RPC latency: for `event_category="rpc"`, compute p50/p95/p99 of `duration_ms` per `event_name`
     - Page views: count by `page` where `event_category="navigation"`
     - Total event count
   - Return 200 JSON with `analyticsSummaryResponse`

5. **Write tests in `analytics_handler_test.go`**
   - `TestAnalytics_MissingName` — 400 when `name` absent
   - `TestAnalytics_InvalidCategory` — 400 on unknown category
   - `TestAnalytics_BatchTooLarge` — 400 when >100 events
   - `TestAnalytics_ValidBatch` — 204, events persisted (use `SQLiteAnalyticsProvider` with in-memory SQLite via temp file)
   - `TestAnalytics_RateLimit` — 429 after 1000 requests
   - `TestAnalytics_Summary` — inserts known events, calls summary endpoint, validates aggregates
   - `TestAnalytics_MethodNotAllowed` — 405 on wrong method for each route

---

### Story 2.2: Config — retention and rate-limit settings

**Files to modify**:
- `config/config.go` (add `AnalyticsMaxRows`, `AnalyticsMaxAgeDays`, `AnalyticsRateLimit` fields)

**Tasks**:

1. **Add analytics config fields to `config.go`**
   - `AnalyticsMaxRows int` — default `100_000`
   - `AnalyticsMaxAgeDays int` — default `90`
   - Add accessor methods: `AnalyticsMaxRowsOrDefault() int`, `AnalyticsMaxAgeDaysOrDefault() int`
   - Persist in `config.json` alongside existing fields (ent auto-migrate handles schema; config change is additive)

2. **Thread config values to retention enforcer and handler**
   - In `wireDepsIntoServer`, read `cfg.AnalyticsMaxRowsOrDefault()` and `cfg.AnalyticsMaxAgeDaysOrDefault()` when calling `analytics.StartRetentionEnforcer(...)`

---

### Story 2.3: Wire analytics into `wireDepsIntoServer`

**Files to modify**:
- `server/server.go` — `wireDepsIntoServer` function

**Tasks**:

1. **Construct `SQLiteAnalyticsProvider` from `deps.AnalyticsEntClient`**
   ```go
   analyticsProvider := analytics.NewSQLiteAnalyticsProvider(deps.AnalyticsEntClient)
   ```

2. **Construct and register `AnalyticsHandler`**
   ```go
   analyticsHandler := handlers.NewAnalyticsHandler(analyticsProvider)
   analyticsHandler.RegisterRoutes(srv.mux)
   ```

3. **Start the retention enforcer goroutine**
   ```go
   analytics.StartRetentionEnforcer(serverCtx, deps.AnalyticsEntClient,
       cfg.AnalyticsMaxRowsOrDefault(), cfg.AnalyticsMaxAgeDaysOrDefault())
   ```

4. **Start the EventBus analytics subscriber** (see Story 2.4)

---

### Story 2.4: EventBus analytics subscriber

**Files to create**:
- `server/analytics/subscriber.go`
- `server/analytics/subscriber_test.go`

**Tasks**:

1. **Implement `StartAnalyticsSubscriber` in `subscriber.go`**
   - Signature: `func StartAnalyticsSubscriber(ctx context.Context, bus *events.EventBus, provider AnalyticsProvider)`
   - Follows the pattern in `server/notifications/subscriber.go`: call `bus.Subscribe(ctx)`, run a goroutine ranging over the channel
   - Map each `events.Event` type to an `analytics.Event`:
     - `session.created` → `event_name="session.created"`, `event_category="user_action"`, `session_id=event.Session.ID`
     - `session.deleted` → `event_name="session.deleted"`, `event_category="user_action"`, `session_id=event.SessionID`
     - `session.status_changed` → `event_name="session.status_changed"`, `event_category="user_action"`, `labels={"old_status": ..., "new_status": ...}`
     - `session.user_interaction` → `event_name="session.user_interaction"`, `event_category="user_action"`
     - Other event types: log and skip
   - Call `provider.Record(ctx, event)` for each mapped event; log errors but do not crash

2. **Wire in `wireDepsIntoServer` (Story 2.3, task 4)**
   ```go
   analytics.StartAnalyticsSubscriber(serverCtx, deps.EventBus, analyticsProvider)
   ```

3. **Write tests in `subscriber_test.go`**
   - Use a `LogAnalyticsProvider` (captures calls) and a mock EventBus or direct channel injection
   - `TestSubscriber_SessionCreated` — publishes `session.created`, verifies `Record` called with correct fields
   - `TestSubscriber_StatusChanged` — verifies `labels` contains old/new status
   - `TestSubscriber_UnknownEventSkipped` — publishes unknown type, verifies no `Record` call

---

### Story 2.5: Upgrade `TelemetryHandler` to forward events

**Files to modify**:
- `server/handlers/telemetry_handler.go`
- `server/handlers/telemetry_handler_test.go`
- `server/server.go` (constructor call site)

**Tasks**:

1. **Inject `AnalyticsProvider` into `TelemetryHandler`**
   - Change `NewTelemetryHandler()` to `NewTelemetryHandler(provider analytics.AnalyticsProvider) *TelemetryHandler`
   - Add `provider analytics.AnalyticsProvider` field to `TelemetryHandler` struct
   - In `HandleTelemetry`, after the existing log call, forward the event:
     ```go
     _ = h.provider.Record(r.Context(), analytics.Event{
         EventName:     safeEvent,
         EventCategory: "user_action",
         DurationMs:    &durationMs,
         SessionID:     req.SessionId,
         Labels:        req.Labels,
     })
     ```
   - Error from `Record` is logged but does not change the HTTP response (backward compat)

2. **Update `wireDepsIntoServer` call**
   - Change `handlers.NewTelemetryHandler()` to `handlers.NewTelemetryHandler(analyticsProvider)`

3. **Update `telemetry_handler_test.go`**
   - Inject a `LogAnalyticsProvider` (or a test double) in all existing tests to satisfy the new constructor
   - Add: `TestTelemetry_ForwardsToProvider` — valid request, verify provider `Record` was called
   - Add: `TestTelemetry_LogInjectionSanitization` — event name with `\n`, verify sanitized string logged (plugs existing coverage gap noted in pitfalls research)

---

## Epic 3: Frontend `AnalyticsProvider` Adapter Layer

**Goal**: Define the TypeScript `AnalyticsProvider` interface, implement `HttpAnalyticsProvider` and `ConsoleAnalyticsProvider`, create `AnalyticsContext` with `useAnalytics()` hook, expose `usePageView()` helper, and mount the provider in `app/layout.tsx`.

**[ADR-011]** `HttpAnalyticsProvider` batches 25 events or flushes every 2 s — this is the primary mechanism preventing rate-limit exhaustion.

### Story 3.1: Define `AnalyticsProvider` TypeScript interface and types

**Files to create**:
- `web-app/src/lib/analytics/types.ts`

**Tasks**:

1. **Define `AnalyticsEvent` interface** (mirrors backend `analytics.Event` JSON shape):
   ```ts
   export interface AnalyticsEvent {
     name: string;
     category: "user_action" | "performance" | "navigation" | "rpc";
     durationMs?: number;
     sessionId?: string;
     page?: string;
     component?: string;
     labels?: Record<string, string>;
   }
   ```

2. **Define `AnalyticsProvider` interface** (modeled on OpenFeature provider contract):
   ```ts
   export interface AnalyticsProviderMetadata { readonly name: string; }

   export interface AnalyticsProvider {
     readonly metadata: AnalyticsProviderMetadata;
     initialize?(): Promise<void>;
     onClose?(): Promise<void>;
     track(event: AnalyticsEvent): void;
     flush?(): Promise<void>;
   }
   ```

**Expected test file**: none (pure types — tested via consumer tests)

---

### Story 3.2: Implement `ConsoleAnalyticsProvider` and `HttpAnalyticsProvider`

**Files to create**:
- `web-app/src/lib/analytics/ConsoleAnalyticsProvider.ts`
- `web-app/src/lib/analytics/HttpAnalyticsProvider.ts`
- `web-app/src/lib/analytics/__tests__/HttpAnalyticsProvider.test.ts`

**Tasks**:

1. **Implement `ConsoleAnalyticsProvider`** in `ConsoleAnalyticsProvider.ts`
   - `readonly metadata = { name: "ConsoleAnalyticsProvider" }`
   - `track(event)`: `console.debug("[analytics]", event.name, event)`
   - No `flush()` needed (synchronous)

2. **Implement `HttpAnalyticsProvider`** in `HttpAnalyticsProvider.ts`
   - **`[ADR-011]`** Batching config constants: `BATCH_SIZE = 25`, `FLUSH_INTERVAL_MS = 2000`, `MAX_QUEUE_SIZE = 200`
   - Private fields: `queue: AnalyticsEvent[]`, `flushTimer: ReturnType<typeof setTimeout> | undefined`
   - `track(event)`:
     - Push to queue
     - If `queue.length >= BATCH_SIZE`: clear timer, call `void this.flush()`
     - Else if no timer set: schedule `this.flush()` in `FLUSH_INTERVAL_MS`
     - If `queue.length >= MAX_QUEUE_SIZE`: drop oldest event (shift) before pushing (bounded queue)
   - `async flush()`:
     - Clear and reset timer
     - Splice the queue (atomic in single-threaded JS)
     - If batch is empty, return
     - `fetch("/api/analytics", { method: "POST", headers: {...}, body: JSON.stringify({ events: batch }), keepalive: true }).catch(() => {})` — `keepalive: true` survives page unload
   - `onClose()`: call `flush()` — drains queue on provider teardown
   - Register `pagehide` listener in `initialize()` that calls `navigator.sendBeacon("/api/analytics", ...)` as a guaranteed-delivery fallback

3. **Write tests in `HttpAnalyticsProvider.test.ts`**
   - Mock `global.fetch`
   - `should_batch_and_flush_after_25_events` — 25 `track()` calls → 1 `fetch` call with 25-event body
   - `should_flush_after_2s_timer` — 1 `track()` call, advance fake timers by 2001ms → 1 `fetch` call
   - `should_not_exceed_max_queue_size` — 201 `track()` calls without flushing → queue length stays ≤ 200
   - `should_flush_on_close` — call `onClose()` → `fetch` called

---

### Story 3.3: Create `AnalyticsContext` with `useAnalytics()` hook

**Files to create**:
- `web-app/src/lib/contexts/AnalyticsContext.tsx`
- `web-app/src/lib/analytics/usePageView.ts`
- `web-app/src/lib/analytics/__tests__/AnalyticsContext.test.tsx`

**Tasks**:

1. **Implement `AnalyticsContext.tsx`** — follow the pattern in `OmnibarContext.tsx`:
   - `"use client"` directive at top
   - `AnalyticsContextValue` interface: `{ provider: AnalyticsProvider; track: AnalyticsProvider["track"] }`
   - `const AnalyticsContext = createContext<AnalyticsContextValue | null>(null)`
   - `export function useAnalytics(): AnalyticsContextValue` — throws if outside provider
   - `export function AnalyticsContextProvider({ provider, children }: Props)`:
     - Use `useRef` to hold the provider instance (stable identity across renders)
     - Use `useMemo` to produce a stable `contextValue` object — prevents re-renders of all consumers when the parent re-renders
     - `useEffect` to call `provider.initialize?.()` on mount and `provider.onClose?.()` on unmount
   - **Note**: Name the React component `AnalyticsContextProvider` (not `AnalyticsProvider`) to avoid the interface/component name collision documented in architecture research

2. **Implement `usePageView()` hook** in `web-app/src/lib/analytics/usePageView.ts`
   - Calls `usePathname()` from `next/navigation`
   - On pathname change, calls `analytics.track({ name: "page_view", category: "navigation", page: pathname })`
   - Wraps in `useEffect` so it fires after mount and on navigation
   - **Must be used only in client components** — add `"use client"` check (hook itself is client-only)

3. **Write tests in `AnalyticsContext.test.tsx`**
   - `useAnalytics_throws_when_outside_provider` — confirms error thrown
   - `useAnalytics_returns_provider_track` — wraps with `AnalyticsContextProvider`, calls `track()` via hook
   - `provider_initialize_called_on_mount` — mock provider, verify `initialize()` called
   - `provider_onClose_called_on_unmount` — verify `onClose()` called on unmount

---

### Story 3.4: Mount provider in `app/layout.tsx`

**Files to modify**:
- `web-app/src/app/layout.tsx`

**Files to create**:
- `web-app/src/lib/analytics/index.ts` (barrel export)

**Tasks**:

1. **Create barrel export** `web-app/src/lib/analytics/index.ts`
   - Re-export: `AnalyticsProvider`, `AnalyticsEvent`, `useAnalytics`, `AnalyticsContextProvider`, `HttpAnalyticsProvider`, `ConsoleAnalyticsProvider`, `usePageView`

2. **Mount `AnalyticsContextProvider` in `app/layout.tsx`**
   - Import `HttpAnalyticsProvider`, `ConsoleAnalyticsProvider`, `AnalyticsContextProvider`
   - Select provider based on `process.env.NODE_ENV`:
     ```tsx
     const analyticsProvider = process.env.NODE_ENV === "production"
       ? new HttpAnalyticsProvider()
       : new ConsoleAnalyticsProvider();
     ```
   - Wrap app tree: `<AnalyticsContextProvider provider={analyticsProvider}>`
   - Mount **below** `ThemeProvider` and **above** `OmnibarProvider` (follows research guidance on mount order)

3. **Add `usePageView()` to the root layout or a dedicated client component**
   - Create `web-app/src/components/analytics/PageViewTracker.tsx` (client component)
   - Calls `usePageView()` and renders `null`
   - Mount `<PageViewTracker />` inside `AnalyticsContextProvider` in `layout.tsx`

---

## Epic 4: ESLint Local Plugin — 4 Analytics Enforcement Rules

**Goal**: Create `web-app/eslint-plugin-analytics/` as a local `file:` workspace package with 4 rules, unit tests via `RuleTester`, and integration into `.eslintrc.json`.

**[ADR-012]** Local plugin as a `file:` package is the correct approach for 4 rules with tests; `no-restricted-syntax` cannot enforce "adjacent-call-required" patterns.

### Story 4.1: Scaffold the local ESLint plugin package

**Files to create**:
- `web-app/eslint-plugin-analytics/package.json`
- `web-app/eslint-plugin-analytics/index.js`

**Tasks**:

1. **Create `package.json`**:
   ```json
   {
     "name": "eslint-plugin-analytics",
     "version": "1.0.0",
     "main": "index.js",
     "license": "UNLICENSED"
   }
   ```

2. **Create `index.js`** — plugin entry point:
   ```js
   module.exports = {
     rules: {
       "require-on-click":        require("./rules/require-on-click"),
       "require-omnibar-dispatch": require("./rules/require-omnibar-dispatch"),
       "require-page-analytics":  require("./rules/require-page-analytics"),
       "require-rpc-analytics":   require("./rules/require-rpc-analytics"),
     },
   };
   ```

3. **Register as a workspace dependency in `web-app/package.json`**
   - Add to `devDependencies`: `"eslint-plugin-analytics": "file:./eslint-plugin-analytics"`
   - Run `npm install` from `web-app/` to create the symlink in `node_modules`

4. **Enable rules in `web-app/.eslintrc.json`**
   - Add `"analytics"` to `plugins` array
   - Add rules with `"error"` severity:
     ```json
     "analytics/require-on-click": "error",
     "analytics/require-omnibar-dispatch": "error",
     "analytics/require-page-analytics": "error",
     "analytics/require-rpc-analytics": "error"
     ```
   - Add `no-restricted-syntax` entry to ban legacy `lib/telemetry` import:
     ```json
     { "selector": "ImportDeclaration[source.value='@/lib/telemetry']",
       "message": "Use useAnalytics().track() instead of the legacy track() from lib/telemetry." }
     ```

---

### Story 4.2: Rule — `require-on-click`

**Files to create**:
- `web-app/eslint-plugin-analytics/rules/require-on-click.js`
- `web-app/eslint-plugin-analytics/rules/__tests__/require-on-click.test.js`

**Tasks**:

1. **Implement `require-on-click.js`**
   - Visitor: `'JSXAttribute[name.name="onClick"]'`
   - Walk up to `JSXOpeningElement` parent
   - Target elements: `elementName === "button"` OR `elementName === "a"` OR sibling `JSXAttribute` with `name.name === "role"` and string value `"button"`
   - **Spread prop guard (critical — pitfalls research §1.1)**: if any sibling attribute is `JSXSpreadAttribute`, suppress the error (can't statically analyze spread contents)
   - **Exempt comment**: check `context.getSourceCode().getCommentsBefore(node.parent.parent)` (the `JSXElement`) for `// analytics-exempt`; also check inline JSX comment `{/* analytics-exempt */}` via `JSXExpressionContainer` with string literal
   - Walk up to enclosing function component body; check for any `CallExpression` where `callee` matches `useAnalytics().track` or where `callee.property.name === "track"` on the analytics context value
   - Report with `messageId: "missingTrack"` and message `"onClick handlers on interactive elements must call useAnalytics().track() or be marked // analytics-exempt"`

2. **Write tests in `require-on-click.test.js`** using `RuleTester`
   - Parser options: `{ ecmaVersion: 2020, ecmaFeatures: { jsx: true } }`
   - Valid cases: button with `track()` call, `{/* analytics-exempt */}` comment, `<div onClick>` (not a button), spread props `{...props}` on button
   - Invalid cases: bare `<button onClick={noop}>`, bare `<a onClick={handler}>`, `role="button"` without track

---

### Story 4.3: Rule — `require-omnibar-dispatch`

**Files to create**:
- `web-app/eslint-plugin-analytics/rules/require-omnibar-dispatch.js`
- `web-app/eslint-plugin-analytics/rules/__tests__/require-omnibar-dispatch.test.js`

**Tasks**:

1. **Implement `require-omnibar-dispatch.js`**
   - Strategy: track when inside `dispatchOmnibarAction` function (handle both `FunctionDeclaration[id.name="dispatchOmnibarAction"]` and `VariableDeclarator[id.name="dispatchOmnibarAction"] > ArrowFunctionExpression` — pitfalls research §1.3)
   - Use `:enter` / `:exit` pairs to track scope:
     ```js
     let inTargetFunction = false;
     let switchDepth = 0;
     ```
   - On `SwitchCase` inside the target function's first-level switch (depth === 1):
     - Recursively scan `node.consequent` for a `CallExpression` where `callee.property.name === "track"` or `callee.name === "track"`
     - Check for `// analytics-exempt` comment via `getCommentsBefore(node)`
     - Report if neither found
   - **Nested switch guard (pitfalls §1.3)**: only enforce on `switchDepth === 1` to avoid false positives in inner switches

2. **Write tests in `require-omnibar-dispatch.test.js`**
   - Valid: case with `track(...)` call, case with `// analytics-exempt` comment
   - Invalid: case with no track call (arrow function form and function declaration form)
   - Edge case: nested switch inside a case body — should NOT flag the inner switch cases

---

### Story 4.4: Rule — `require-page-analytics`

**Files to create**:
- `web-app/eslint-plugin-analytics/rules/require-page-analytics.js`
- `web-app/eslint-plugin-analytics/rules/__tests__/require-page-analytics.test.js`

**Tasks**:

1. **Implement `require-page-analytics.js`**
   - Gate on filename: `if (!/\/app\/.*\/page\.tsx?$/.test(context.getFilename())) return {};`
   - Visitor: `'ExportDefaultDeclaration'`
   - Check whether the entire source file's AST body contains a `CallExpression` for `usePageView()` or `useAnalytics().track` with first argument string `"page_view"`
   - Check for file-level `// analytics-exempt` comment (first comment in the file)
   - Report on the `ExportDefaultDeclaration` node

2. **Write tests in `require-page-analytics.test.js`**
   - Requires setting `filename` on the `RuleTester` test case: `{ code: ..., filename: "/app/sessions/page.tsx" }`
   - Valid: page with `usePageView()`, page with `useAnalytics().track("page_view", ...)`, non-page file (no `app/` in path)
   - Invalid: page without either call

---

### Story 4.5: Rule — `require-rpc-analytics`

**Files to create**:
- `web-app/eslint-plugin-analytics/rules/require-rpc-analytics.js`
- `web-app/eslint-plugin-analytics/rules/__tests__/require-rpc-analytics.test.js`

**Tasks**:

1. **Implement `require-rpc-analytics.js`**
   - Detect calls to hooks imported from `@/lib/hooks/useSessionService` or hooks returned by `useSessionService` (e.g., `createSession`, `listSessions`, `deleteSession`)
   - **File-path exclusions (critical — pitfalls research §1.2)**: skip files matching:
     - `lib/contexts/` (provider components — `OmnibarContext.tsx`, `SessionServiceContext.tsx`)
     - `lib/hooks/` (custom hook files — `useSessionService.ts`, `useSessionActions.ts`)
   - Walk up to the enclosing React component function (first function ancestor where name starts with capital letter or is a default export)
   - Check if any `CallExpression` with `callee.property.name === "track"` or `callee.name === "track"` exists anywhere in that component's function body (including inside `useCallback` and `useEffect` — pitfalls §1.2)
   - Check for `// analytics-exempt` comment near the RPC hook call
   - Report if not found

2. **Write tests in `require-rpc-analytics.test.js`**
   - Valid: component calling `createSession` + `track(...)` in the same function scope
   - Valid: `track(...)` inside a `useCallback` within the same component
   - Valid: file in `lib/contexts/` path (skipped)
   - Invalid: component calling `createSession` without any `track(...)` call
   - Edge case: `// analytics-exempt` suppresses the error

---

## Epic 5: Upgrade Web Vitals + RPC Timing to Persist Events

**Goal**: Upgrade `WebVitalsReporter.tsx` to POST CWV metrics to `/api/analytics` and upgrade `rpcTiming.ts` to enqueue RPC latency events via the `AnalyticsProvider`. Both use `useAnalytics()` to get the active provider — they do not call `fetch` directly.

### Story 5.1: Upgrade `WebVitalsReporter.tsx`

**Files to modify**:
- `web-app/src/components/telemetry/WebVitalsReporter.tsx`

**Tasks**:

1. **Accept `analytics` provider as a prop** (to keep the component pure and testable):
   - Change signature to `export function WebVitalsReporter({ analytics }: { analytics: Pick<AnalyticsProvider, "track"> })`
   - Alternatively: call `useAnalytics()` directly since `WebVitalsReporter` is already a client component
   - **Decision**: call `useAnalytics()` directly (simpler, consistent with other callsites); document that the component must be inside `AnalyticsContextProvider`

2. **Upgrade `handleVital` to call `analytics.track()`**:
   ```ts
   analytics.track({
     name: `web_vital.${metric.name.toLowerCase()}`,
     category: "performance",
     durationMs: Math.round(metric.value),
     labels: { rating: metric.rating, id: metric.id },
   });
   ```
   - Keep the existing `performance.mark(...)` call for DevTools compatibility
   - Keep the existing `console.debug(...)` call in non-production

3. **Update the mount site** in `app/layout.tsx` or wherever `WebVitalsReporter` is mounted
   - No change needed if using `useAnalytics()` internally (provider is in context)

---

### Story 5.2: Upgrade `rpcTiming.ts` interceptor

**Files to modify**:
- `web-app/src/lib/telemetry/rpcTiming.ts`

**Files to create**:
- `web-app/src/lib/telemetry/__tests__/rpcTiming.test.ts`

**Tasks**:

1. **Change `createRpcTimingInterceptor` signature to accept an optional `AnalyticsProvider`**:
   ```ts
   export function createRpcTimingInterceptor(analytics?: Pick<AnalyticsProvider, "track">): Interceptor
   ```
   - In the `finally` block, call:
     ```ts
     analytics?.track({
       name: `rpc.${method}`,
       category: "rpc",
       durationMs,
       labels: { method, ok: String(ok) },
     });
     ```
   - Keep all existing `performance.mark`/`performance.measure` calls unchanged (additive change only)

2. **Update the interceptor instantiation site** (wherever `createRpcTimingInterceptor()` is called — likely `web-app/src/lib/transport/` or a provider setup file)
   - Pass the active provider: `createRpcTimingInterceptor(analyticsProvider)`
   - The provider is obtained from module-level singleton or threaded through component props; check the existing call site to determine the cleanest approach

3. **Write tests in `rpcTiming.test.ts`**
   - Mock `global.performance`
   - `should_track_rpc_event_on_success` — interceptor wraps a successful call; verify `analytics.track` called with `category: "rpc"`, `durationMs` > 0, `labels.ok === "true"`
   - `should_track_rpc_event_on_error` — interceptor wraps a failing call; verify `labels.ok === "false"`
   - `should_not_throw_when_analytics_absent` — call without passing analytics arg; no error

---

## Epic 6: Wire Existing Telemetry Callsites to New Provider

**Goal**: Replace all remaining direct calls to `track()` from `web-app/src/lib/telemetry.ts` with `useAnalytics().track(...)`, then mark `lib/telemetry.ts` for deprecation. The ESLint `no-restricted-syntax` rule added in Epic 4 will block new imports of the legacy module once all existing callsites are migrated.

### Story 6.1: Audit and migrate existing `track()` callsites

**Files to modify**: all files currently importing from `@/lib/telemetry`

**Tasks**:

1. **Find all existing callsites**
   - Search for `from '@/lib/telemetry'` and `from "../lib/telemetry"` and `from "../../lib/telemetry"` across `web-app/src/`
   - Expected callsites: `WebVitalsReporter.tsx` (migrated in Epic 5), `rpcTiming.ts` (migrated in Epic 5), possibly a handful of component files

2. **For each callsite, replace with `useAnalytics().track(...)`**
   - Adapt the call signature: old `track(event, durationMs, labels, sessionId)` → new `{ name: event, category: "user_action", durationMs, labels, sessionId }`
   - If the callsite is in a non-React context (utility function, not a component), create a thin wrapper or restructure to accept the provider as a parameter

3. **Add `// analytics-exempt` comments to any callsites that are intentionally not tracked**
   - Example: error boundary catch handlers where `useAnalytics()` context may not be available

---

### Story 6.2: Deprecate `lib/telemetry.ts` legacy module

**Files to modify**:
- `web-app/src/lib/telemetry.ts`

**Tasks**:

1. **Add `@deprecated` JSDoc comment** to `track()` function:
   ```ts
   /**
    * @deprecated Use useAnalytics().track() from @/lib/analytics instead.
    * This function remains for backward compatibility with the /api/telemetry endpoint.
    * Do not add new callsites — the analytics/require-on-click ESLint rule will block it.
    */
   ```

2. **Do not delete the file** — the `/api/telemetry` HTTP endpoint still exists and is backward-compatible. The function can remain as a thin shim but should not grow new callers.

---

### Story 6.3: Update feature registry

**Files to modify**:
- `docs/registry/features/analytics.json` (create new per-feature file)

**Tasks**:

1. **Create `docs/registry/features/analytics.json`** per the feature registry rules in `.claude/rules/feature-registry.md`:
   ```json
   {
     "id": "analytics",
     "type": "both",
     "description": "Analytics event ingestion and summary API",
     "backend": {
       "rpcs": [],
       "handlers": ["POST /api/analytics", "GET /api/analytics/summary"]
     },
     "frontend": {
       "components": ["AnalyticsContextProvider", "WebVitalsReporter", "PageViewTracker"],
       "hooks": ["useAnalytics", "usePageView"]
     },
     "tested": false,
     "testIds": [],
     "lastModified": "2026-05-09T00:00:00Z"
   }
   ```
   - Set `"tested": true` and populate `testIds` after writing tests (Stories 2.1, 3.2, 3.3, 4.x)

2. **Run `make registry-generate`** to update the aggregate registry and commit all changed files

---

## Implementation Order and Dependencies

```
Epic 1 (Storage)
    └── Epic 2 (Backend HTTP + Wiring)    ← depends on Epic 1
            └── Epic 5 (Upgrade Telemetry callsites — backend side)
Epic 3 (Frontend Provider)
    └── Epic 5 (Upgrade Web Vitals + RPC Timing)
            └── Epic 6 (Wire existing callsites)
Epic 4 (ESLint Plugin)                   ← can start in parallel with Epics 1–3
                                            but must be the LAST thing enabled in CI
                                            (after all callsites are migrated in Epic 6)
```

**Safe order for a single engineer**:
1. Epic 1 → Epic 2 (Stories 2.1, 2.2, 2.3, 2.4, 2.5) — backend is fully self-contained
2. Epic 3 (Stories 3.1–3.4) — frontend layer
3. Epic 5 (Stories 5.1–5.2) — upgrade existing callsites
4. Epic 6 (Stories 6.1–6.3) — migrate remaining callsites + registry
5. Epic 4 (Stories 4.1–4.5) — enable ESLint rules only after all callsites are compliant

---

## Key Constraints Summary

| Constraint | Where it applies |
|-----------|-----------------|
| Separate `analytics.db` (not `sessions.db`) | Epic 1, Story 1.2 — `OpenAnalyticsDB` opens a dedicated file |
| `HttpAnalyticsProvider` batches 25 events / 2 s flush | Epic 3, Story 3.2 — `BATCH_SIZE=25`, `FLUSH_INTERVAL_MS=2000` |
| `/api/analytics` rate limit: 1000/min (not 100/min) | Epic 2, Story 2.1 — new `rateLimiter` instance in `AnalyticsHandler` |
| `/api/telemetry` rate limit unchanged at 100/min | Epic 2, Story 2.5 — existing limiter untouched |
| `ent generate` must include `--feature sql/upsert` | Epic 1, Story 1.1 — per CLAUDE.md |
| ESLint spread-prop false positive guard | Epic 4, Story 4.2 — detect `JSXSpreadAttribute` siblings |
| ESLint file-path exclusions for `require-rpc-analytics` | Epic 4, Story 4.5 — skip `lib/contexts/` and `lib/hooks/` |
| React render-loop safety: stable context value | Epic 3, Story 3.3 — `useRef` + `useMemo` in provider |
| All `AnalyticsEvent` non-core fields marked `.Optional()` | Epic 1, Story 1.1 — per pitfalls research §3 |
| Commit all generated ent files together | Epic 1, Story 1.1 — partial commits break the build |
