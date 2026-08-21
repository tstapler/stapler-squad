# Features Research: Browser Console Interception Patterns

## Existing Codebase Patterns

### No existing console interception
The codebase has **no existing monkey-patching of console methods**. All `console.log/warn/error` calls in the codebase are direct calls — none are intercepted or forwarded. This is greenfield territory.

### Existing telemetry hook (`web-app/src/lib/telemetry.ts`)
The closest existing pattern is `track()` in `telemetry.ts`:
```ts
export function track(event, durationMs, labels?, sessionId?) {
  fetch('/api/telemetry', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ event, duration_ms, session_id, timestamp, labels }),
  }).catch(() => {}); // fire-and-forget; never throw
}
```
This is the reference for fire-and-forget, never-throw frontend→server log shipping.

### Existing hooks for reference structure
`web-app/src/lib/hooks/`:
- `usePushNotifications.ts` — uses `useEffect` + `useCallback` + `useState`, guards with `typeof window !== 'undefined'`, ref-tracks callbacks to avoid stale closures
- `useLiveTail.ts` — uses `useRef` for the interval handle and the fetch callback, cleans up in `useEffect` return
- Both hooks follow: SSR-safe guard → `useEffect` for side effects → `useRef` for mutable state → cleanup in return

## Console Interception Pattern (Safe Monkey-Patching)

### Core technique
```ts
const originals = {
  log:   console.log.bind(console),
  warn:  console.warn.bind(console),
  error: console.error.bind(console),
};

// CRITICAL: bind originals BEFORE patching
console.log = (...args) => {
  originals.log(...args);    // always call through
  enqueue({ level: 'log', args });
};
```

**Key safety rule**: capture and bind original methods before replacing them. The interceptor MUST call through to the original — swallowing console output would break developer experience.

### window.onerror
```ts
const prevOnError = window.onerror;
window.onerror = (msg, src, line, col, err) => {
  enqueue({ level: 'error', args: [msg, { src, line, col, stack: err?.stack }] });
  return prevOnError?.(msg, src, line, col, err) ?? false;
};
```
`window.onerror` fires for uncaught errors in scripts. Return `false` (or `undefined`) to let the browser's default error reporting continue.

### unhandledrejection
```ts
const onUnhandled = (e: PromiseRejectionEvent) => {
  enqueue({ level: 'error', args: ['UnhandledRejection', String(e.reason)] });
};
window.addEventListener('unhandledrejection', onUnhandled);
// cleanup: window.removeEventListener('unhandledrejection', onUnhandled)
```

### navigator.sendBeacon for page-unload
`fetch` is killed when the page unloads; `sendBeacon` survives unload/navigation:
```ts
window.addEventListener('beforeunload', () => {
  if (buffer.length > 0) {
    navigator.sendBeacon('/api/v1/browser-logs', JSON.stringify({ entries: buffer }));
  }
});
```
`sendBeacon` is always `POST`, `Content-Type: text/plain` (the spec), or `Blob`/`FormData`. For JSON use `new Blob([body], { type: 'application/json' })`.

Note: `sendBeacon` is not available in older iOS Safari (< 11.1) or some WebViews. Fallback to synchronous `fetch` with `keepalive: true` which also survives navigation:
```ts
fetch('/api/v1/browser-logs', { method: 'POST', body, keepalive: true }).catch(() => {});
```

## Batching / Debounce Pattern

Forwarding every console call individually would flood the server. Recommended approach:
1. Maintain a ring buffer capped at N entries (e.g. 100).
2. Use a `setTimeout` debounce (e.g. 1s) to flush the batch.
3. Flush immediately on page unload via `beforeunload`.
4. On toggle-off, cancel the pending timeout and optionally flush remaining entries.

```ts
const buffer: LogEntry[] = [];
let flushTimer: ReturnType<typeof setTimeout> | null = null;

function enqueue(entry: LogEntry) {
  if (!enabled) return;
  buffer.push(entry);
  if (buffer.length >= MAX_BUFFER) flush();  // eager flush on overflow
  if (!flushTimer) {
    flushTimer = setTimeout(flush, DEBOUNCE_MS);
  }
}

function flush() {
  if (flushTimer) { clearTimeout(flushTimer); flushTimer = null; }
  if (buffer.length === 0) return;
  const entries = buffer.splice(0);
  fetch('/api/v1/browser-logs', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ entries }),
    keepalive: true,
  }).catch(() => {});
}
```

## Existing React Hooks in web-app/src/lib/hooks/ — Structural Patterns

All hooks in this directory follow a consistent pattern:
1. `"use client"` directive at top
2. Import `useEffect`, `useRef`, `useCallback`, `useState` from react
3. `typeof window !== 'undefined'` guard for SSR
4. `useRef` for imperative handles (timers, abort controllers, callbacks)
5. Cleanup returned from `useEffect` (remove event listeners, clear timers)
6. No direct throws — errors are caught and placed in state or silently swallowed

The new `useBrowserLogStream` hook should follow this exact structure.
