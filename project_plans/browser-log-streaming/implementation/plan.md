# Browser Log Streaming — Implementation Plan

_Status_: Draft  
_Created_: 2026-05-02  
_Requirements_: `../requirements.md`  
_Research_: `../research/`

---

## Summary

One epic, three stories, seven tasks. No proto changes. No new packages. Follows the existing
`telemetry_handler.go` + `usePushNotifications.ts` patterns exactly.

---

## Epic: Browser Log Streaming (E-BLS)

Enable client-side console output to be forwarded to the server's structured log so that mobile
browser bugs can be diagnosed from `staplersquad.log` without DevTools.

---

## Story 1: Go HTTP endpoint (S-BLS-1)

**Acceptance**: `POST /api/v1/client-logs` returns 204 for well-formed requests; entries appear in
`staplersquad.log` prefixed `[client-log]`; malformed / oversized / rate-limited requests return
appropriate 4xx without crashing.

### Task 1.1 — `server/handlers/browser_log_handler.go`

**File**: `server/handlers/browser_log_handler.go`  
**Package**: `handlers` (same package as `telemetry_handler.go`)

#### Types

```go
// clientLogEntry is a single captured browser log entry.
type clientLogEntry struct {
    Level     string `json:"level"`      // "log" | "warn" | "error" | "debug"
    Message   string `json:"message"`
    Timestamp string `json:"timestamp"`  // ISO 8601
    URL       string `json:"url"`
    UserAgent string `json:"userAgent"`
    SessionID string `json:"sessionId,omitempty"`
}

// clientLogsRequest is the JSON body for POST /api/v1/client-logs.
type clientLogsRequest struct {
    Entries []clientLogEntry `json:"entries"`
}
```

#### Struct and constructor

```go
// BrowserLogHandler handles POST /api/v1/client-logs.
type BrowserLogHandler struct {
    limiter *rateLimiter // reuse rateLimiter from telemetry_handler.go (same package)
}

// NewBrowserLogHandler creates a new BrowserLogHandler.
func NewBrowserLogHandler() *BrowserLogHandler {
    return &BrowserLogHandler{
        limiter: &rateLimiter{
            resetAt: time.Now().Add(time.Minute),
        },
    }
}
```

Note: `rateLimiter` is defined in `telemetry_handler.go` and is accessible within the same
`handlers` package — no duplication or export needed.

#### Handler function

```go
// HandleClientLogs handles POST /api/v1/client-logs.
func (h *BrowserLogHandler) HandleClientLogs(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }
    if !h.limiter.allow() {
        http.Error(w, "Too many requests", http.StatusTooManyRequests)
        return
    }

    r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

    var req clientLogsRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid JSON body", http.StatusBadRequest)
        return
    }
    if len(req.Entries) == 0 {
        w.WriteHeader(http.StatusNoContent)
        return
    }
    if len(req.Entries) > 200 {
        http.Error(w, "entries exceeds maximum of 200", http.StatusBadRequest)
        return
    }

    for _, entry := range req.Entries {
        h.logEntry(entry)
    }
    w.WriteHeader(http.StatusNoContent)
}
```

#### Log helper

```go
// logEntry writes a single browser log entry to the server log.
func (h *BrowserLogHandler) logEntry(e clientLogEntry) {
    // Sanitize all string fields: strip newlines to prevent log injection.
    msg  := sanitizeLogField(e.Message, 200)
    ua   := sanitizeLogField(e.UserAgent, 80)
    sid  := sanitizeLogField(e.SessionID, 64)
    lvl  := sanitizeLogField(e.Level, 16)
    url  := sanitizeLogField(e.URL, 256)

    // Route to appropriate log level; server always owns the severity prefix.
    logger := log.InfoLog
    if lvl == "error" {
        logger = log.ErrorLog
    }
    logger.Printf("[client-log] %s %s %s (url: %s ua: %s)",
        lvl, sid, msg, url, ua)
}

// sanitizeLogField strips control characters (newline, carriage return, tab)
// and truncates to maxLen runes. Returns the safe string.
func sanitizeLogField(s string, maxLen int) string {
    s = strings.ReplaceAll(s, "\n", `\n`)
    s = strings.ReplaceAll(s, "\r", `\r`)
    s = strings.ReplaceAll(s, "\t", `\t`)
    runes := []rune(s)
    if len(runes) > maxLen {
        return string(runes[:maxLen]) + "…"
    }
    return s
}
```

#### Full imports

```go
import (
    "encoding/json"
    "net/http"
    "strings"

    "github.com/tstapler/stapler-squad/log"
)
```

---

### Task 1.2 — Register route in `server/server.go`

**File**: `server/server.go`  
**Location**: immediately after the existing telemetry block (lines 347–350).

```go
// Register browser log streaming handler for client-side log forwarding.
browserLogHandler := handlers.NewBrowserLogHandler()
srv.mux.HandleFunc("POST /api/v1/client-logs", browserLogHandler.HandleClientLogs)
log.InfoLog.Printf("Registered browser log handler at POST /api/v1/client-logs")
```

No new imports in `server.go` — `handlers` package is already imported.

---

### Task 1.3 — Go unit tests

**File**: `server/handlers/browser_log_handler_test.go`  
**Package**: `handlers`

Test functions (mirrors `telemetry_handler_test.go` structure):

| Function | Verifies |
|---|---|
| `TestBrowserLog_EmptyEntries` | 204 on empty entries slice |
| `TestBrowserLog_ValidSingleEntry` | 204 on valid single entry |
| `TestBrowserLog_ValidBatch` | 204 on batch of 50 valid entries |
| `TestBrowserLog_MethodNotAllowed` | 405 on GET |
| `TestBrowserLog_MalformedJSON` | 400 on invalid JSON |
| `TestBrowserLog_TooManyEntries` | 400 when entries > 200 |
| `TestBrowserLog_OversizedBody` | 400 when body > 64 KB |
| `TestBrowserLog_RateLimit` | 429 after 100 requests in same window |
| `TestBrowserLog_LogInjection` | message with `\n` is sanitized before log write |
| `TestBrowserLog_MessageTruncation` | message > 200 chars is truncated with `…` |
| `TestBrowserLog_UAShortened` | userAgent > 80 chars is truncated |

Test helper pattern (same as telemetry):

```go
func postClientLogs(h *BrowserLogHandler, body any) *httptest.ResponseRecorder {
    b, _ := json.Marshal(body)
    req := httptest.NewRequest(http.MethodPost, "/api/v1/client-logs", bytes.NewReader(b))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    h.HandleClientLogs(w, req)
    return w
}
```

---

## Story 2: Frontend hook `useBrowserLogStream` (S-BLS-2)

**Acceptance**: When `enabled: true`, the hook installs console interceptors and POSTs batched
entries to `/api/v1/client-logs` every 5 s or at 50-entry threshold. When `enabled: false`,
originals are restored, timers cancelled, buffer cleared, and no HTTP calls are made.

### Task 2.1 — `web-app/src/lib/hooks/useBrowserLogStream.ts`

**File**: `web-app/src/lib/hooks/useBrowserLogStream.ts`

#### Interfaces

```ts
export interface BrowserLogEntry {
  level: "log" | "warn" | "error" | "debug";
  message: string;
  timestamp: string;   // ISO 8601
  url: string;
  userAgent: string;
  sessionId?: string;
}

export interface UseBrowserLogStreamOptions {
  /** Whether log streaming is active. Hook is a no-op when false. */
  enabled: boolean;
  /** Optional session ID to tag entries. */
  sessionId?: string;
  /** Override endpoint for testing. Defaults to '/api/v1/client-logs'. */
  endpoint?: string;
}
```

#### Full hook skeleton

```ts
"use client";

import { useEffect, useRef } from "react";

const MAX_BUFFER = 50;
const MAX_MSG_LEN = 200;
const FLUSH_INTERVAL_MS = 5_000;
const DEFAULT_ENDPOINT = "/api/v1/client-logs";

export function useBrowserLogStream(options: UseBrowserLogStreamOptions): void {
  const enabledRef  = useRef(options.enabled);
  const sessionRef  = useRef(options.sessionId);
  const endpointRef = useRef(options.endpoint ?? DEFAULT_ENDPOINT);

  // Keep refs current without triggering re-effects
  useEffect(() => { enabledRef.current  = options.enabled; },   [options.enabled]);
  useEffect(() => { sessionRef.current  = options.sessionId; }, [options.sessionId]);
  useEffect(() => { endpointRef.current = options.endpoint ?? DEFAULT_ENDPOINT; },
            [options.endpoint]);

  useEffect(() => {
    // SSR guard
    if (typeof window === "undefined") return;
    if (!options.enabled) return;

    const buffer: BrowserLogEntry[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let intercepting = false;

    // Capture originals BEFORE patching
    const originals = {
      log:   console.log.bind(console),
      warn:  console.warn.bind(console),
      error: console.error.bind(console),
      debug: console.debug.bind(console),
    };

    function truncate(s: string, max: number): string {
      if (s.length <= max) return s;
      return s.slice(0, max) + "…";
    }

    function argsToMessage(args: unknown[]): string {
      try {
        return args.slice(0, 5).map(a =>
          typeof a === "object" ? JSON.stringify(a) : String(a)
        ).join(" ");
      } catch {
        return args.slice(0, 5).map(String).join(" ");
      }
    }

    function enqueue(level: BrowserLogEntry["level"], args: unknown[]): void {
      if (!enabledRef.current) return;
      if (intercepting) return;          // reentrancy guard
      intercepting = true;
      try {
        const entry: BrowserLogEntry = {
          level,
          message: truncate(argsToMessage(args), MAX_MSG_LEN),
          timestamp: new Date().toISOString(),
          url: window.location.href,
          userAgent: navigator.userAgent,
          sessionId: sessionRef.current,
        };
        buffer.push(entry);
        if (buffer.length >= MAX_BUFFER) {
          flush();
        } else if (!flushTimer) {
          flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);
        }
      } finally {
        intercepting = false;
      }
    }

    function flush(): void {
      if (flushTimer) { clearTimeout(flushTimer); flushTimer = null; }
      if (buffer.length === 0) return;
      const entries = buffer.splice(0);
      const body = JSON.stringify({ entries });
      fetch(endpointRef.current, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body,
        keepalive: true,
      }).catch(() => {});   // fire-and-forget — never recurse into console
    }

    // Install interceptors
    console.log   = (...a) => { originals.log(...a);   enqueue("log",   a); };
    console.warn  = (...a) => { originals.warn(...a);  enqueue("warn",  a); };
    console.error = (...a) => { originals.error(...a); enqueue("error", a); };
    console.debug = (...a) => { originals.debug(...a); enqueue("debug", a); };

    // window.onerror
    const prevOnError = window.onerror;
    window.onerror = (msg, src, line, col, err) => {
      originals.error("[onerror]", msg, src, line, col, err?.stack);
      enqueue("error", [String(msg), `${src}:${line}:${col}`, err?.stack ?? ""]);
      return prevOnError?.(msg, src, line, col, err) ?? false;
    };

    // unhandledrejection
    function onUnhandled(e: PromiseRejectionEvent): void {
      originals.error("[unhandledrejection]", e.reason);
      enqueue("error", ["UnhandledRejection", String(e.reason)]);
    }
    window.addEventListener("unhandledrejection", onUnhandled);

    // Page-unload beacon
    function onBeforeUnload(): void {
      if (buffer.length === 0) return;
      const entries = buffer.splice(0);
      const body = JSON.stringify({ entries });
      // sendBeacon with Blob (ensures application/json content-type)
      const blob = new Blob([body], { type: "application/json" });
      if (!navigator.sendBeacon(endpointRef.current, blob)) {
        // Fallback for browsers that don't support sendBeacon with Blob
        fetch(endpointRef.current, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body,
          keepalive: true,
        }).catch(() => {});
      }
    }
    window.addEventListener("beforeunload", onBeforeUnload);

    // Start periodic flush
    flushTimer = setTimeout(flush, FLUSH_INTERVAL_MS);

    // Cleanup: restore originals, cancel timer, clear buffer
    return () => {
      console.log   = originals.log;
      console.warn  = originals.warn;
      console.error = originals.error;
      console.debug = originals.debug;
      window.onerror = prevOnError;
      window.removeEventListener("unhandledrejection", onUnhandled);
      window.removeEventListener("beforeunload", onBeforeUnload);
      if (flushTimer) { clearTimeout(flushTimer); flushTimer = null; }
      buffer.splice(0);   // discard buffered entries on disable
    };
  }, [options.enabled]);  // re-run only when enabled changes
}
```

**Key design decisions**:
- `useRef` for `enabled`/`sessionId`/`endpoint` keeps them fresh inside the closure without
  re-installing the interceptors on every render.
- The `useEffect` dependency is `[options.enabled]` only — installing/removing interceptors is the
  only side-effect that needs to change when the flag flips.
- `intercepting` is a closure-local boolean (not a ref) — it only needs to be coherent within a
  single synchronous call stack.
- Originals are captured at install time (inside the effect body), restoring them in cleanup is
  safe even if multiple effects run sequentially.
- `argsToMessage` wraps `JSON.stringify` in try/catch to handle circular references (pitfall 5).

**Singleton note** (pitfall 5): if `TerminalOutput` is ever mounted more than once simultaneously
(session pool scenario), multiple hook instances would each patch `console.*`. Since each install
captures the *then-current* `console.log` as its `originals.log`, the interceptors chain
automatically (innermost effect patches the already-patched method from the outer effect). Cleanup
unwinds in reverse order (React's cleanup ordering). This is correct behaviour without a separate
singleton guard for the current usage pattern where `TerminalOutput` is rendered once per visible
session.

---

### Task 2.2 — Frontend hook tests

**File**: `web-app/src/lib/hooks/__tests__/useBrowserLogStream.test.ts`

Test cases:

| Test name | Verifies |
|---|---|
| `disabled_should_not_patch_console` | No patching when `enabled: false` |
| `enabled_should_patch_all_four_methods` | `console.{log,warn,error,debug}` replaced |
| `enabled_should_call_through_to_originals` | Patched methods still invoke originals |
| `enqueue_should_truncate_long_messages` | Messages > 200 chars get `…` suffix |
| `enqueue_should_flush_at_buffer_cap` | 50th entry triggers immediate flush |
| `flush_should_POST_to_endpoint` | `fetch` called with correct URL and body |
| `flush_should_not_throw_on_fetch_failure` | `fetch` rejection is silently swallowed |
| `toggle_off_should_restore_originals` | Console methods reset after `enabled → false` |
| `toggle_off_should_clear_buffer` | Buffer emptied after disable |
| `toggle_off_should_cancel_timer` | `clearTimeout` called on disable |
| `onerror_should_be_intercepted` | `window.onerror` assignment captured |
| `unhandledrejection_should_be_intercepted` | `addEventListener` called |
| `reentrancy_guard_should_prevent_infinite_loop` | Recursive `console.error` does not re-enqueue |
| `circular_ref_arg_should_not_throw` | Circular object in args falls back to `String()` |
| `session_id_included_in_entry` | `sessionId` from options appears in posted entry |

Pattern (mirrors `usePushNotifications.test.ts`):

```ts
import { renderHook, act } from "@testing-library/react";
import { useBrowserLogStream } from "../useBrowserLogStream";

describe("useBrowserLogStream", () => {
  let mockFetch: jest.Mock;
  const origLog   = console.log;
  const origWarn  = console.warn;
  const origError = console.error;
  const origDebug = console.debug;

  beforeEach(() => {
    mockFetch = jest.fn().mockResolvedValue({ ok: true } as Response);
    global.fetch = mockFetch;
    jest.useFakeTimers();
  });

  afterEach(() => {
    console.log   = origLog;
    console.warn  = origWarn;
    console.error = origError;
    console.debug = origDebug;
    jest.useRealTimers();
    jest.restoreAllMocks();
    delete (global as Record<string, unknown>).fetch;
  });
  // ...
});
```

---

## Story 3: TerminalOutput UI toggle (S-BLS-3)

**Acceptance**: "📡 Log Stream" button appears in the `devOnly` toolbar cluster on desktop
(≥769 px), is hidden on ≤768 px. Toggle persists in `localStorage`. Green styling when active.
Hook is called with the current `sessionId` and the toggle state.

### Task 3.1 — State and handler addition in `TerminalOutput.tsx`

**File**: `web-app/src/components/sessions/TerminalOutput.tsx`

1. **New import** at the top of the file (alongside existing hook imports):
   ```ts
   import { useBrowserLogStream } from "@/lib/hooks/useBrowserLogStream";
   ```

2. **New state** — immediately after the `debugMode` state block (line ~147):
   ```ts
   // Remote log streaming state
   const [logStreamEnabled, setLogStreamEnabled] = useState(() => {
     if (typeof window !== "undefined") {
       return localStorage.getItem("stapler-squad-remote-debug") === "true";
     }
     return false;
   });
   ```

3. **Hook call** — after the `useTerminalStream` call block, before the first render return:
   ```ts
   useBrowserLogStream({ enabled: logStreamEnabled, sessionId });
   ```

4. **Handler** — immediately after `handleToggleDebug` (line ~759):
   ```ts
   const handleToggleLogStream = useCallback(() => {
     const next = !logStreamEnabled;
     setLogStreamEnabled(next);
     if (typeof window !== "undefined") {
       if (next) {
         localStorage.setItem("stapler-squad-remote-debug", "true");
       } else {
         localStorage.removeItem("stapler-squad-remote-debug");
       }
     }
   }, [logStreamEnabled]);
   ```

5. **Button JSX** — in the `toolbarExpanded` block, directly after the Debug button (line ~999),
   before the Record button:
   ```tsx
   <button
     className={`${styles.toolbarButton} ${styles.devOnly} ${logStreamEnabled ? styles.debugActive : ''}`}
     onClick={handleToggleLogStream}
     title={logStreamEnabled
       ? "Stop forwarding console logs to server"
       : "Forward console logs to server (Remote Debug)"}
     aria-label={logStreamEnabled ? "Disable remote log streaming" : "Enable remote log streaming"}
     style={logStreamEnabled ? { backgroundColor: '#2a4', color: 'white', fontWeight: 'bold' } : {}}
   >
     📡 {logStreamEnabled ? 'Log Stream ON' : 'Log Stream'}
   </button>
   ```

**localStorage key**: `stapler-squad-remote-debug` (matches FR-5 requirement).

---

### Task 3.2 — Component tests / smoke test

**File**: `web-app/src/components/sessions/__tests__/TerminalOutput.logstream.test.tsx`
(or appended to any existing `TerminalOutput.test.tsx` if one exists)

Test cases (using `@testing-library/react` + `jest`):

| Test name | Verifies |
|---|---|
| `renders_log_stream_button_on_desktop` | Button rendered in expanded toolbar |
| `log_stream_button_hidden_on_mobile` | `devOnly` class applied; CSS hides it (style assertion) |
| `toggle_on_sets_localStorage` | `localStorage.setItem` called with correct key+value |
| `toggle_off_removes_localStorage` | `localStorage.removeItem` called |
| `button_shows_ON_label_when_active` | Text content contains "Log Stream ON" |
| `button_shows_green_style_when_active` | `backgroundColor` is `#2a4` when active |
| `hook_receives_sessionId_prop` | `useBrowserLogStream` mock called with `sessionId` |

Mock `useBrowserLogStream` at the module level for component tests so the hook's side effects
(console patching) don't bleed into the test environment.

---

## File Change Summary

| File | Change type | Story |
|---|---|---|
| `server/handlers/browser_log_handler.go` | New | S-BLS-1, T-1.1 |
| `server/handlers/browser_log_handler_test.go` | New | S-BLS-1, T-1.3 |
| `server/server.go` | Edit (4 lines added) | S-BLS-1, T-1.2 |
| `web-app/src/lib/hooks/useBrowserLogStream.ts` | New | S-BLS-2, T-2.1 |
| `web-app/src/lib/hooks/__tests__/useBrowserLogStream.test.ts` | New | S-BLS-2, T-2.2 |
| `web-app/src/components/sessions/TerminalOutput.tsx` | Edit (import + state + call + handler + button) | S-BLS-3, T-3.1 |
| `web-app/src/components/sessions/__tests__/TerminalOutput.logstream.test.tsx` | New | S-BLS-3, T-3.2 |

Total: 5 new files, 2 edited files.

---

## Flagged Choices and Caveats

### FC-1: Handler path — `client-logs` vs `browser-logs`

Requirements (FR-3) say `POST /api/v1/client-logs`. The stack research document mentions
`/api/v1/browser-logs` as the route. The requirements document is authoritative — use
`/api/v1/client-logs`. Both the handler and the frontend default endpoint must use this path.

### FC-2: `rateLimiter` reuse across package

`rateLimiter` is unexported (`rateLimiter` not `RateLimiter`). Because `browser_log_handler.go` is
in the same `handlers` package as `telemetry_handler.go`, it can reference `rateLimiter` directly
without exporting. This is correct and intentional — no change needed.

### FC-3: `debugActive` CSS class is a semantic no-op

`TerminalOutput.css.ts` line 138: `export const debugActive = style({})`. The active visual
styling is applied via inline `style=` on the button (same as the existing Debug button). The new
button follows this identical pattern. No CSS change needed.

### FC-4: localStorage key

Requirements say `stapler-squad-remote-debug` (FR-5). Use that exact key. The architecture
research suggests `browser-log-stream` as an alternative — disregard; requirements win.

### FC-5: No singleton guard for multi-mount

The pitfalls research flags the multiple-mount scenario. For the current usage (one `TerminalOutput`
per visible session panel), chaining interceptors via React's effect ordering is safe. If a pool
of sessions ever renders multiple `TerminalOutput` instances simultaneously, a module-level
reference counter should be added. This is deferred as a known limitation, noted in a code comment.

### FC-6: `argsToMessage` argument count limit

The hook limits args to 5 (`args.slice(0, 5)`) and truncates each to 200 chars. FR-2 only
specifies a total message cap of 200 chars. The implementation applies truncation to the joined
string, so the effective cap is 200 chars regardless of how many args are combined. This is
stricter than strictly required but prevents edge-case large payloads.

---

## Counts

- Epics: 1
- Stories: 3
- Tasks: 7
- New files: 5
- Edited files: 2
- Flagged choices: 6
