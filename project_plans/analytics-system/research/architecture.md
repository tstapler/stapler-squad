# Architecture Research: Analytics System

## 1. EventBus — Subscriber Pattern

**File:** `pkg/events/bus.go`, `pkg/events/types.go`

### How it works

`EventBus` is a thread-safe pub/sub bus backed by buffered Go channels (default buffer size 100 per subscriber). `Subscribe(ctx)` returns a `<-chan *Event` and an ID; the subscription is auto-cleaned when the context is cancelled. `Publish(event)` is non-blocking — if a subscriber's channel is full the event is dropped for that subscriber silently. `Close()` drains all subscribers on graceful shutdown.

### Events already published (capturable by analytics)

| EventType | When fired | Key payload fields |
|---|---|---|
| `session.created` | Session creation | `Session` (full instance) |
| `session.updated` | Property changes | `Session`, `UpdatedFields` |
| `session.deleted` | Deletion | `SessionID` |
| `session.status_changed` | Status transition | `SessionID`, `OldStatus`, `NewStatus` |
| `session.user_interaction` | User sends input | `SessionID`, `InteractionType`, `Context` |
| `session.acknowledged` | User acks session | `SessionID`, `Context` (reason) |
| `session.approval_response` | Tool approval/deny | `SessionID`, `Approved`, `Context` |
| `session.notification` | Toast notification | `SessionID`, notification fields |

### Can backend analytics be a subscriber?

Yes — the pattern is already established. `server/notifications/subscriber.go` and `server/push/subscriber.go` both call `bus.Subscribe(ctx)` at startup and run a goroutine that ranges over the channel. The analytics subscriber should follow the same pattern:

```go
// server/analytics/subscriber.go
func StartAnalyticsSubscriber(ctx context.Context, bus *events.EventBus, provider AnalyticsProvider) {
    ch, _ := bus.Subscribe(ctx)
    go func() {
        for {
            select {
            case event, ok := <-ch:
                if !ok { return }
                // map events.Event → analytics.Event and call provider.Record(ctx, event)
            case <-ctx.Done():
                return
            }
        }
    }()
}
```

The notifications subscriber uses a coalescing buffer (500ms ticker) to batch rapid-fire events — analytics should NOT coalesce; it should record every distinct event. The subscriber is wired in `server/server.go` inside `wireDepsIntoServer()` after `deps.EventBus` is available.

---

## 2. ent ORM Schema Pattern

**Files:** `session/ent/schema/error_event.go`, `session/ent/schema/classificationanalytics.go`, `session/ent/schema/session.go`

### Complete entity pattern

A minimal complete ent entity has:
1. `struct { ent.Schema }` embedding
2. `Fields() []ent.Field` — typed field definitions
3. `Indexes() []ent.Index` — composite or single-field indexes
4. Optionally `Edges() []ent.Edge` for foreign-key relationships

### How JSON is stored

`field.Strings("python_imports").Optional()` — ent stores string slices as a JSON array in SQLite automatically. For `map[string]string` labels, the pattern should be `field.JSON("labels", map[string]string{}).Optional()`. The `ClassificationAnalytics` schema uses `field.Strings()` for arrays; for the `AnalyticsEvent.labels` field we should use `field.JSON`.

### How indexes are defined

```go
func (ErrorEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("last_seen"),           // single field
        index.Fields("acknowledged"),
    }
}
```

Composite indexes: `index.Fields("event_name", "created_at")`. The `ClassificationAnalytics` schema shows 5 separate single-field indexes for high-cardinality filter fields.

### Proposed AnalyticsEvent schema

```go
// session/ent/schema/analytics_event.go
package schema

import (
    "time"
    "entgo.io/ent"
    "entgo.io/ent/schema/field"
    "entgo.io/ent/schema/index"
)

type AnalyticsEvent struct{ ent.Schema }

func (AnalyticsEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").StorageKey("id").Unique().NotEmpty(),   // UUID, caller-supplied
        field.String("event_name").NotEmpty(),
        field.String("event_category").NotEmpty(),                  // user_action|performance|navigation|rpc
        field.String("session_id").Optional(),
        field.Int64("duration_ms").Optional().Nillable(),
        field.String("page").Optional(),
        field.String("component").Optional(),
        field.JSON("labels", map[string]string{}).Optional(),
        field.Time("created_at").Default(time.Now).Immutable(),
    }
}

func (AnalyticsEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("event_name"),
        index.Fields("event_category"),
        index.Fields("session_id"),
        index.Fields("created_at"),
    }
}
```

**Critical:** Always generate with the `--feature sql/upsert` flag (per CLAUDE.md):
```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

---

## 3. Existing Telemetry Infrastructure

### Backend handler (`server/handlers/telemetry_handler.go`)

`TelemetryHandler` is a simple struct with a sliding-window rate limiter (100 req/min). `HandleTelemetry` decodes a `telemetryRequest` JSON body and **only logs it** — no persistence. Registered at:
```go
srv.mux.HandleFunc("POST /api/telemetry", telemetryHandler.HandleTelemetry)
```

The shape of `telemetryRequest`: `{event, duration_ms, session_id, timestamp, labels}` — this is the wire format that must remain backward-compatible. The new `/api/analytics` endpoint should accept a superset that adds `event_category`, `page`, and `component`.

Upgrading telemetry to forward to analytics: the `TelemetryHandler` needs an injected `AnalyticsProvider` dependency and should call `provider.Record(ctx, event)` after logging. Constructor changes from `NewTelemetryHandler()` to `NewTelemetryHandler(provider AnalyticsProvider)`.

### Route registration pattern

All HTTP handlers are registered in `wireDepsIntoServer()` in `server/server.go` using:
```go
srv.mux.HandleFunc("POST /api/analytics", analyticsHandler.HandlePost)
srv.mux.HandleFunc("GET /api/analytics/summary", analyticsHandler.HandleSummary)
```

Go 1.22 method+path syntax (`"POST /api/..."`) is already in use (see the image upload handler at line 361). New endpoints should follow this pattern.

The handler struct is created in `wireDepsIntoServer` and registered there directly; no separate `RegisterRoutes` method is needed (though some handlers like `EscapeCodeHandler` do use that pattern).

### Handler as standalone struct or with RegisterRoutes

Two patterns exist:
- Direct `HandleFunc` calls in `wireDepsIntoServer` (telemetry, server-info, approval)
- `handler.RegisterRoutes(srv.mux)` method pattern (pushHandler, hookReceiver, escapeCodeHandler)

For the analytics handler, given it registers two routes (`POST /api/analytics` and `GET /api/analytics/summary`), the `RegisterRoutes(mux)` pattern is more appropriate.

---

## 4. Frontend React Context Pattern

**Files examined:** `OmnibarContext.tsx`, `NotificationContext.tsx`, `ThemeContext.tsx`, `SessionServiceContext.tsx`

### Standard pattern

All contexts follow the same structure:
1. Define a typed interface (`XxxContextValue`)
2. `createContext<XxxContextValue | null>(null)` — null default forces provider check
3. `useXxx()` hook with `if (!context) throw new Error("must be within Provider")`
4. `XxxProvider` component with `"use client"` directive at the top

### AnalyticsContext design

```typescript
// web-app/src/lib/contexts/AnalyticsContext.tsx
"use client";

import { createContext, useContext, ReactNode } from "react";

export interface AnalyticsProvider {
  track(event: AnalyticsEvent): void;
  flush?(): Promise<void>;
}

interface AnalyticsContextValue {
  provider: AnalyticsProvider;
  track: AnalyticsProvider["track"];
}

const AnalyticsContext = createContext<AnalyticsContextValue | null>(null);

export function useAnalytics(): AnalyticsContextValue {
  const ctx = useContext(AnalyticsContext);
  if (!ctx) throw new Error("useAnalytics must be used within AnalyticsProvider");
  return ctx;
}

export function AnalyticsProvider({ provider, children }: { provider: AnalyticsProvider; children: ReactNode }) {
  return (
    <AnalyticsContext.Provider value={{ provider, track: provider.track.bind(provider) }}>
      {children}
    </AnalyticsContext.Provider>
  );
}
```

Note: `AnalyticsProvider` as a React component name conflicts with the `AnalyticsProvider` interface name. Convention from requirements is to call the interface `AnalyticsProvider` and the React component `AnalyticsProviderRoot` or `AnalyticsContextProvider`.

### Where to mount

Based on `SessionServiceContext.tsx`, global providers mount at the layout level (`app/layout.tsx`). The `AnalyticsProvider` should wrap below `ThemeProvider` but above feature contexts.

---

## 5. API Routing Pattern

### How `/api` routing works

`server.go` uses `http.ServeMux` directly — no external router. Go 1.22 method syntax is used for disambiguation:
```go
srv.mux.HandleFunc("POST /api/telemetry", handler.HandleTelemetry)  // method-specific
srv.mux.Handle("/api/...", handler)                                    // prefix match
```

ConnectRPC handlers are registered at `/api/session.v1.SessionService/...` (prefix mount via `http.StripPrefix`).

For REST-style analytics endpoints, register two exact patterns:
```go
analyticsHandler := analytics.NewHTTPHandler(sqliteProvider)
analyticsHandler.RegisterRoutes(srv.mux)
// Internally:
//   mux.HandleFunc("POST /api/analytics", h.HandlePost)
//   mux.HandleFunc("GET /api/analytics/summary", h.HandleSummary)
```

The handler is constructed and wired in `wireDepsIntoServer()` after `deps.Storage` is available (since `SQLiteAnalyticsProvider` needs the ent client).

### Dependencies flow

```
ServerDependencies.Storage (ent client)
    └── SQLiteAnalyticsProvider
            ├── HandlePost  → POST /api/analytics
            └── HandleSummary → GET /api/analytics/summary

TelemetryHandler(provider=SQLiteAnalyticsProvider)
    └── POST /api/telemetry  (backward-compat, forwards to provider)
```

---

## 6. Client-Side Batching Pattern

**File:** `web-app/src/lib/telemetry/rpcTiming.ts`

### Current pattern

`createRpcTimingInterceptor()` is a ConnectRPC `Interceptor` that:
1. Calls `performance.mark("rpc:<Method>:start")` before the call
2. In `finally`: calls `performance.measure("rpc:<Method>", { start, detail: {method, url, ok, durationMs} })`
3. Only logs to `console.debug` in non-production; **never POSTs anywhere**

### Upgrade strategy to batch-POST

The interceptor should be upgraded to also enqueue the timing data and flush it to `/api/analytics` in batches:

```typescript
// Pattern: module-level queue + debounced flush
const rpcQueue: AnalyticsEvent[] = [];
let flushTimer: ReturnType<typeof setTimeout> | null = null;

function enqueueRpcEvent(event: AnalyticsEvent) {
  rpcQueue.push(event);
  if (!flushTimer) {
    flushTimer = setTimeout(() => {
      flushRpcQueue();
      flushTimer = null;
    }, 2000); // flush every 2s
  }
  if (rpcQueue.length >= 20) {
    // flush immediately if batch is large enough
    if (flushTimer) clearTimeout(flushTimer);
    flushRpcQueue();
    flushTimer = null;
  }
}

function flushRpcQueue() {
  const batch = rpcQueue.splice(0);
  if (batch.length === 0) return;
  fetch('/api/analytics', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ events: batch }),
    keepalive: true,  // survives page unload
  }).catch(() => {});
}
```

The interceptor becomes a factory that captures an `AnalyticsProvider` reference:

```typescript
export function createRpcTimingInterceptor(analytics?: AnalyticsProvider): Interceptor {
  return (next) => async (req) => {
    // ... existing Performance API marks ...
    // In finally:
    analytics?.track({
      event_name: `rpc.${method}`,
      event_category: "rpc",
      duration_ms: durationMs,
      labels: { method, ok: String(ok) },
    });
  };
}
```

The `HttpAnalyticsProvider` batches internally; individual `track()` calls are fire-and-forget on the provider side.

### WebVitals upgrade

`WebVitalsReporter.tsx` currently uses `useReportWebVitals` from Next.js. It should be upgraded to call `useAnalytics().track(...)` with `event_category: "performance"` for each CWV metric (LCP, FCP, CLS, TTFB, INP).

---

## Key Integration Points Summary

| Component | Integration Point | Notes |
|---|---|---|
| EventBus analytics subscriber | `wireDepsIntoServer()` after `deps.EventBus` is available | Follow `notifications.StartSubscriber` pattern |
| `AnalyticsEvent` ent entity | New file `session/ent/schema/analytics_event.go` | Use `field.JSON` for labels |
| `POST /api/analytics` | `wireDepsIntoServer()`, `analyticsHandler.RegisterRoutes(mux)` | After `deps.Storage` |
| `GET /api/analytics/summary` | Same handler, same registration call | Query params: from, to, category |
| `TelemetryHandler` upgrade | Inject `AnalyticsProvider`, forward after logging | Backward-compat: no schema change |
| `AnalyticsContext` | `app/layout.tsx`, wraps all children | `HttpAnalyticsProvider` in prod, `Console` in dev |
| `rpcTiming.ts` upgrade | Accept optional `AnalyticsProvider` arg | Batches to `/api/analytics` |
| `WebVitalsReporter` upgrade | Call `useAnalytics().track()` for each CWV | `event_category: "performance"` |
