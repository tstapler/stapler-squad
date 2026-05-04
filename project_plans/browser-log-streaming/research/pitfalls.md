# Pitfalls Research: Risks and Mitigations

## 1. Infinite Recursion: console.error Called Inside the Interceptor

**Risk**: The interceptor itself (or the fetch call it triggers) may internally call `console.error` — e.g., a network error causes the browser to log to console, which re-enters the interceptor, which makes another fetch, which fails, which logs again.

**Mitigation**: Use a reentrancy guard (boolean flag) that disables the interceptor while it is executing the enqueue/flush code:

```ts
let intercepting = false;

console.error = (...args) => {
  originals.error(...args);  // always call through first
  if (intercepting) return;  // bail if we're inside the interceptor
  intercepting = true;
  try {
    enqueue({ level: 'error', args });
  } finally {
    intercepting = false;
  }
};
```

The call-through to `originals.error` is intentionally placed *before* the reentrancy guard so that even if enqueue itself logs, the original output is never suppressed.

**Also**: Inside the `flush` function, never use intercepted console methods. Only use `originals.warn(...)` or `originals.error(...)` for internal errors so flushing cannot re-trigger itself.

## 2. Race Condition: Toggle-Off While a Flush Is In-Flight

**Risk**: User clicks "Log Stream OFF" while a `setTimeout`-debounced flush or a `fetch` is in flight. The `enabled` ref may be stale when the flush callback runs. The handler receives the last batch, sets state to disabled, but then a queued timer fires and enqueues more entries or issues another fetch.

**Mitigation**: Use a `useRef` for the enabled state inside the hook (not just React state, which is async), so the flush callback always reads the latest value:

```ts
const enabledRef = useRef(options.enabled);
useEffect(() => { enabledRef.current = options.enabled; }, [options.enabled]);

function enqueue(entry: LogEntry) {
  if (!enabledRef.current) return;  // fast-path check
  // ...
}
```

On toggle-off cleanup:
1. `clearTimeout(flushTimer)` — cancel the pending debounced flush
2. Do NOT flush remaining entries on toggle-off unless explicitly desired (avoids sending a partial log after the user said stop)
3. Clear the buffer: `buffer.splice(0)`

In `useEffect` cleanup (component unmount or `enabled → false`):
```ts
useEffect(() => {
  if (!options.enabled) {
    if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
    bufferRef.current = [];
    return;
  }
  // ... install interceptors ...
  return () => {
    // restore originals, clear timer, clear buffer
    console.log = originals.log;
    console.warn = originals.warn;
    console.error = originals.error;
    if (flushTimerRef.current) clearTimeout(flushTimerRef.current);
    bufferRef.current = [];
  };
}, [options.enabled]);
```

Note: the `fetch` itself may still complete after toggle-off. This is benign — `fetch` is fire-and-forget and the server will log the last batch. The important thing is that no **new** enqueues happen after toggle-off.

## 3. Large Payloads Crashing the Handler

**Risk**: A burst of verbose logging (e.g., `console.log` inside a tight loop, a large object dump) could create a payload that:
- Exceeds the server's `MaxBytesReader` cap → `413 Request Entity Too Large`
- Takes too long to serialize → blocks the main thread
- Contains very long strings → excessive memory on the server

**Mitigations**:

**Frontend**:
- Cap buffer at N entries (e.g., 50): when the buffer is full, evict the oldest entry (ring buffer) rather than growing unboundedly
- Truncate individual log argument strings: `String(arg).slice(0, 512)` (or 1024) per argument
- Limit number of arguments per entry: `args.slice(0, 5)`
- Total payload guard: if serialized batch exceeds e.g. 32 KB, split and send first half, discard the rest (or drop oldest)

**Backend** (Go handler):
- `http.MaxBytesReader(w, r.Body, 64*1024)` — same 64 KB cap as the existing telemetry handler
- Limit number of entries per request: reject if `len(req.Entries) > 200`
- Truncate message strings server-side before logging: `msg[:min(len(msg), 512)]`
- Rate limiter: reuse the same `rateLimiter` pattern from `telemetry_handler.go` (100 req/min per process)

**Reference**: `telemetry_handler.go` lines 71-91 show all three defences: MaxBytesReader, label count check, and event name sanitization.

## 4. Missing CSRF Protection on the New Endpoint

**Risk**: The new `POST /api/v1/browser-logs` endpoint could be called by a malicious page on a different origin (CSRF). A forged request could flood the server logs with attacker-controlled content or cause log injection.

**Analysis of existing protections**:
- Primary defence: `SameSite=Strict` on the `cs_auth` session cookie (set in `server/auth/handlers.go` line 269). Any cross-origin `POST` request will not include the session cookie, so the global auth middleware will reject it with 401.
- Secondary defence: CORS headers echo the requesting origin. The auth middleware returns 401 for unauthenticated requests before any handler logic runs.
- There is no token-based CSRF check on `/api/telemetry` or `/api/v1/upload-image` — both rely solely on the session-cookie + SameSite mechanism. The new endpoint should follow the same pattern.

**When auth is disabled**: On localhost-only deployments auth is typically disabled (`authMiddleware = nil`). In that case there is no auth at all — but the server only listens on `localhost:8543`, which is not reachable from other origins' scripts (same-origin restriction applies). Browser scripts on `evil.example.com` cannot reach `localhost:8543` due to browser CORS policy — the preflight `OPTIONS` would fail because the CORS middleware returns the origin header, and `localhost:8543` would not match an external origin in a cross-origin request context.

**Log injection risk**: Even with auth protection, the handler should sanitize all string values before logging (strip `\n`, `\r`, control characters) to prevent an authenticated user from injecting fake log lines. This is already done in `telemetry_handler.go` lines 93-95:
```go
safeEvent := strings.ReplaceAll(req.Event, "\n", `\n`)
safeEvent = strings.ReplaceAll(safeEvent, "\r", `\r`)
```
Apply the same sanitization to every log entry message in the new handler.

**Recommendation**: No additional CSRF protection needed beyond what already exists. The new endpoint should:
1. Require a valid `cs_auth` session cookie (provided automatically by the auth middleware)
2. Use `MaxBytesReader` + entry count limit (prevents body-based DoS)
3. Sanitize all string values before writing to the log file (prevents log injection)

## 5. Additional Smaller Risks

**SSR / server-side rendering**: The hook must guard all browser globals (`window`, `console`, `navigator`) with `typeof window !== 'undefined'`. Next.js renders components server-side; any unguarded access to `window` will crash the server-side render. The existing pattern in `TerminalOutput.tsx` line 143-147 is the template.

**Multiple simultaneous mounts**: If `TerminalOutput` is mounted multiple times (e.g., session pool), multiple hook instances would each install their own console interceptors, causing multiple patches. The interceptors should be installed at the module level (outside the hook) or behind a singleton guard, with only the enabled-count tracked per hook instance. Alternative: use a module-level reference counter.

**Symbol/circular-reference args**: `JSON.stringify` will throw on circular references. Wrap the serialization in a try/catch and fall back to `String(arg)`.

**Performance on very noisy consoles**: `TerminalOutput.tsx` itself calls `console.log` extensively (e.g., `[TerminalOutput] Terminal resized...`). When log streaming is enabled, every terminal resize would enqueue a log entry. The debounce + ring buffer cap ensures this does not explode, but the user should be warned that enabling log streaming on a noisy terminal session will produce verbose output.
