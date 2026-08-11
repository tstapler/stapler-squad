# Browser Log Streaming — Requirements

## Problem Statement

When users access stapler-squad from Android or iOS devices, browser DevTools are
unavailable. When the terminal fails to open or ConnectRPC streaming breaks, there
is no visible error and no way for the developer to diagnose what went wrong remotely.
The app needs to stream client-side console output back to the server so that mobile
bugs can be debugged from server logs alone.

## Goals

1. Capture `console.log`, `console.warn`, `console.error`, `console.debug`,
   `window.onerror`, and `unhandledrejection` events in the browser.
2. Batch and POST captured entries to the server over a lightweight JSON HTTP endpoint.
3. Write received entries into the server's structured log so they appear in
   `staplersquad.log` alongside server-side events.
4. Provide a **Remote Debug** toggle in the TerminalOutput toolbar (dev-only, mobile-hidden
   by default) so normal users are not affected.
5. Ensure the feature has zero impact on users who never enable it.

## Non-Goals

- Persisting log entries to the database.
- Real-time streaming (batched polling is sufficient).
- Exposing a public log-viewer UI (server logs are enough for now).
- Capturing network request/response bodies.

## Stakeholders

- Tyler (developer) — primary consumer of server-side logs.
- Mobile users — must not notice any behaviour change unless they enable the toggle.

## Functional Requirements

### FR-1: Log capture

- Intercept `console.log`, `console.warn`, `console.error`, `console.debug`.
- Intercept `window.onerror` (message, source, line, col, error.stack).
- Intercept `window.addEventListener('unhandledrejection', ...)` for Promise errors.
- Each captured entry includes: `level` (log|warn|error|debug), `message` (string),
  `timestamp` (ISO 8601), `url` (current `window.location.href`),
  `userAgent` (`navigator.userAgent`), `sessionId` (from React context if available).

### FR-2: Batching and throttling

- Buffer entries in memory; flush to server every **5 seconds** or when the buffer
  reaches **50 entries**, whichever comes first.
- Message bodies are capped at **200 characters** (truncated with `…` suffix).
- Maximum **1 flush per 5 seconds** regardless of buffer size (rate limit).
- On page unload, attempt a synchronous `navigator.sendBeacon` flush of any buffered entries.

### FR-3: ConnectRPC endpoint

- New RPC on `SessionService` in `proto/session/v1/session.proto`:
  `LogClientEvents(LogClientEventsRequest) returns (LogClientEventsResponse) {}`
- `LogClientEventsRequest` carries a repeated `ClientLogEntry` message with fields:
  `level` (string), `message` (string), `timestamp` (string), `url` (string),
  `user_agent` (string), `session_id` (string).
- `LogClientEventsResponse` is empty.
- Run `make generate-proto` to regenerate Go + TypeScript bindings.
- Implementation goes in `server/services/session_service.go` alongside other RPCs.
- Frontend uses the generated TypeScript ConnectRPC client (same transport already
  used by all other service calls — no new transport setup needed).

### FR-4: Server-side logging

- Each received entry is written to `log.InfoLog` (or `log.ErrorLog` for level=error)
  prefixed with `[client-log]`.
- Format: `[client-log] <level> <sessionId> <message> (ua: <shortened-ua>)`.
- UserAgent is shortened to first 80 characters to avoid log spam.

### FR-5: UI toggle

- A **"📡 Remote Debug"** button in `TerminalOutput.tsx` toolbar, inside the dev-only
  section (hidden on ≤768px alongside Debug, Record, Streaming Mode).
- Toggle state stored in `localStorage` under key `stapler-squad-remote-debug`.
- Default: **off**.
- When toggled on, a green indicator or "ON" label is shown.
- The log interceptors are installed only while the toggle is on (or on-demand
  the first time, then cleaned up on toggle-off).

### FR-6: Graceful degradation

- If the POST to `/api/v1/client-logs` fails (network error, non-2xx), silently
  discard the batch (do not recurse into console.error).
- If the feature is disabled, the original console methods must not be patched.

## Non-Functional Requirements

- **Performance**: the batching overhead must be imperceptible (< 1 ms per intercepted call).
- **Privacy**: no PII beyond what's already visible in normal server logs (URL, UA).
- **Security**: the endpoint must not allow injection of arbitrary log content at
  elevated severity into the server (server always prefixes `[client-log]`).
- **Bundle size**: the client module must be < 2 KB gzipped.

## Acceptance Criteria

| ID    | Criterion |
|-------|-----------|
| AC-1  | With Remote Debug OFF, no console methods are patched and no HTTP calls are made. |
| AC-2  | With Remote Debug ON, `console.error("test")` in the browser results in a `[client-log] error` line in `staplersquad.log` within 10 seconds. |
| AC-3  | A message longer than 200 chars is truncated with `…` in the server log. |
| AC-4  | More than 50 entries within 5 seconds only results in 1 HTTP call (first 50 entries). |
| AC-5  | Toggling Remote Debug OFF removes the console patches and stops HTTP calls. |
| AC-6  | The Remote Debug button is hidden on screens ≤768px (devOnly class). |
| AC-7  | `window.onerror` and `unhandledrejection` errors appear in the server log. |
| AC-8  | The POST endpoint returns 204 and the handler does not crash on malformed JSON. |

## Tech Stack Constraints

- **Backend**: Go, ConnectRPC, existing `log` package. New `LogClientEvents` RPC added
  to `SessionService` in `proto/session/v1/session.proto`; implementation in
  `server/services/session_service.go`. Run `make generate-proto` after proto changes.
- **Frontend**: React + TypeScript, vanilla-extract CSS, existing `TerminalOutput.tsx`.
  New hook `useBrowserLogStream` calls `LogClientEvents` via the existing generated
  TypeScript ConnectRPC client (same client used by all other service calls).
- **Transport**: ConnectRPC unary call — consistent with all other RPC calls in the app.

## Open Questions (resolved)

- **Q: Should logs be visible in a UI panel?** No — server log file is sufficient for now.
- **Q: Should the endpoint require auth?** No — it's local-only and the server only
  logs; there's no privilege escalation risk.
- **Q: WebSocket vs polling?** Batched HTTP POST — simpler, no persistent connection needed.
