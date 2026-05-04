# Stack Research: Go HTTP Handler Patterns

## Route Registration in server/server.go

Routes are registered on a plain `http.ServeMux` stored as `srv.mux`. Two patterns coexist:

1. **Direct `HandleFunc`** — most endpoints, e.g.:
   ```go
   srv.mux.HandleFunc("POST /api/v1/upload-image", handler.HandleUpload)
   srv.mux.HandleFunc("POST /api/telemetry", telemetryHandler.HandleTelemetry)
   ```
   The Go 1.22 method-prefix form (`"POST /path"`) is used for mutation endpoints.

2. **`Handle` (returns `http.Handler`)** — for handlers that implement `http.Handler` (ConnectRPC, MCP), e.g.:
   ```go
   srv.mux.Handle("/mcp", mcpHTTPHandler)
   ```

3. **`RegisterRoutes` helper** — a few services (EscapeCodeHandler, HookReceiver, CircuitBreakerHandler) call `RegisterRoutes(mux)` on themselves, which lets the handler self-describe its route set:
   ```go
   escapeCodeHandler.RegisterRoutes(srv.mux)
   ```

4. **`RegisterHTTPHandler` / `RegisterConnectHandler`** — thin wrappers on `srv.mux.Handle`, used by code outside the main `Start` flow:
   ```go
   srv.RegisterHTTPHandler("POST /foo", handler)
   ```

**Auth middleware** wraps the entire mux; the auth middleware exempts `/auth/`, `/login`, `/health`, `/_next/`, and `/favicon` by prefix. Every other path — including all `/api/*` routes — requires an `cs_auth` session cookie or Bearer token when auth is enabled. The new `/api/v1/browser-logs` endpoint will therefore be automatically protected by the existing auth middleware (no extra wiring needed). The telemetry handler (`POST /api/telemetry`) is NOT in the exempt list, so it is also auth-protected.

**Middleware chain** (outermost → innermost):
```
otelhttp → Logging → CORSWithOrigins → Compress → [Auth] → mux
```

## Closest Reference Handler: telemetry_handler.go

`server/handlers/telemetry_handler.go` is the canonical model for a new browser-log endpoint:

- Package: `handlers` (import `"github.com/tstapler/stapler-squad/server/handlers"`)
- Struct `TelemetryHandler` holds a `*rateLimiter`
- `NewTelemetryHandler()` constructor wires the rate limiter
- `HandleTelemetry` is registered as `"POST /api/telemetry"` in `server.go`
- Request body capped with `http.MaxBytesReader(w, r.Body, 64*1024)` (64 KB)
- JSON decode → validate → sanitize → `log.InfoLog.Printf(...)`
- Rate limiter: 100 req/min sliding window (`rateLimiter` struct with `sync.Mutex`, `count int`, `resetAt time.Time`)

Registration in server.go (lines 348-350):
```go
telemetryHandler := handlers.NewTelemetryHandler()
srv.mux.HandleFunc("POST /api/telemetry", telemetryHandler.HandleTelemetry)
log.InfoLog.Printf("Registered telemetry handler at POST /api/telemetry")
```

## Log Package

Located at `log/log.go`. Key exported symbols:

| Symbol | Type | Usage |
|---|---|---|
| `log.InfoLog` | `*log.Logger` | `log.InfoLog.Printf("browser_log %s", ...)` |
| `log.WarningLog` | `*log.Logger` | warnings |
| `log.ErrorLog` | `*log.Logger` | errors |
| `log.DebugLog` | `*log.Logger` | debug |
| `log.ForSession(id)` | `*SessionLogger` | session-scoped logging |

The package initialises a rotating file writer (`lumberjack`) + optional `os.Stderr` console writer. Level filtering is per-stream. Log messages automatically get an instance-ID prefix (`[pid-NNN-TIMESTAMP]` or `[INSTANCE_ID]`).

For browser log entries the handler should use `log.InfoLog.Printf` (or `log.DebugLog`) after sanitising the message, following the same pattern as telemetry_handler.go line 96.

## Summary

- **New handler file**: `server/handlers/browser_log_handler.go` (mirrors `telemetry_handler.go` in the same package)
- **Route**: `"POST /api/v1/browser-logs"` registered in `server.go` the same way telemetry is
- **Auth**: inherited automatically from the global auth middleware — no extra wiring
- **Log package**: use `log.InfoLog.Printf` with sanitised input (strip `\n`/`\r` to prevent log injection)
- **Body cap**: `http.MaxBytesReader(w, r.Body, 64*1024)` — same 64 KB cap as telemetry
