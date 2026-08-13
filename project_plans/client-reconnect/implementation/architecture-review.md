# Architecture Review: client-reconnect

**Date**: 2026-06-23
**Revision**: 3 (post-ARCH-B3-patch)
**Verdict**: CONCERNS — 0 blockers, 3 concerns, 3 nitpicks

---

## Resolved (all revisions)

- [x] **ARCH-B1** (Rev 1): `useDebouncedCallback` in Story 2.1.3 — `debounceTimerRef` + `useCallback(…, [])` produces a stable function reference that `removeEventListener` correctly matches.
- [x] **ARCH-B2** (Rev 1): Abort signal path in Story 1.1.3 — `if (signal?.aborted || ev.wasClean || ev.code === 1000) { push(null); }` guard prevents spurious `ConnectError` on every `stopWatching()` call.
- [x] **ARCH-B3** (Rev 2): `event?.type` resolving to deprecated `window.event` global — both handlers now declare `(ev: Event)` and use `ev.type`. Story 2.1.3 Task 1 uses `useCallback((ev: Event) => { ... ev.type ... }, [])`. Story 3.1.3 Task 1 uses `const handleVisibilityOrOnline = (ev: Event) => { ... ev.type ... }`. TypeScript will enforce the parameter at compile time. `online` trigger is no longer dead code.
- [x] **CONCERN-2** (Rev 2): Recursive `connect()` self-call inside `finally` creating an unbounded async promise chain — Story 3.1.2 Task 1 now uses `setTimeout(() => { if (shouldReconnectRef.current && !isDisconnectingRef.current) { connect(); } }, delay)`. The `finally` block returns synchronously; the callback fires outside the async frame. No recursive promise accumulation.

---

## Blockers

None.

---

## Concerns

### CONCERN-1 — Story 3.1.3 (Task 1): `connect` in effect dependency array causes constant listener re-registration; use `connectRef` forwarding pattern instead

**Status**: Not addressed in Revision 3.

**Detail**: The plan still directs "Include `connect` in the dependency array (it is a `useCallback` with stable identity)." This claim is unsafe — `connect`'s dep array includes `sessionId`, `shellId`, `onShellStatusChange`, `getTerminal`, `onError`, `onScrollbackReceived`, `onOutput`, `streamingMode`, `flowControl`, `metrics`, `handleError`, `initialCols`, and `initialRows`. Props like `flowControl` and `metrics` likely return new object references each render, meaning `connect` is not stable. Any identity change fires the `useEffect`, removes and re-adds the listeners, and causes listener churn under parent re-renders. In React StrictMode this doubles.

**Remediation**: Use a `connectRef` forwarding pattern so the listener effect has an empty dep array:

```ts
const connectRef = useRef(connect);
useEffect(() => { connectRef.current = connect; }, [connect]);

// In Story 3.1.3 useEffect:
const handleVisibilityOrOnline = useCallback((ev: Event) => {
  if (document.visibilityState !== "visible" && ev.type !== "online") return;
  if (shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current) {
    terminalBackoffRef.current.reset();
    connectRef.current(); // always current connect, no captured snapshot
  }
}, []);

useEffect(() => {
  if (process.env.NEXT_PUBLIC_RECONNECT_V2 !== "true") return;
  document.addEventListener("visibilitychange", handleVisibilityOrOnline);
  window.addEventListener("online", handleVisibilityOrOnline);
  return () => {
    document.removeEventListener("visibilitychange", handleVisibilityOrOnline);
    window.removeEventListener("online", handleVisibilityOrOnline);
  };
}, []); // no connect dep needed
```

---

### CONCERN-3 — Story 2.2.2 (Task 1): `listSessions({})` on seq backwards-jump ignores active filter — produces a temporary session list mismatch

**Status**: Not addressed in Revision 3.

**Detail**: The `listSessions` call is written as `clientRef.current?.listSessions({})` with no arguments. If the active watch stream has a `categoryFilter` or `statusFilter`, the re-fetch returns all sessions while the resuming stream delivers only filtered events. The result is a briefly incorrect session list — extra sessions visible that should be hidden.

**Remediation**: Pass `watchOptionsRef.current` to the re-fetch:

```ts
void clientRef.current?.listSessions({
  category: watchOptionsRef.current?.categoryFilter,
  status:   watchOptionsRef.current?.statusFilter,
}).then(r => {
  if (shouldReconnectRef.current && streamGenerationRef.current === myGeneration) {
    dispatch(setSessions(r.sessions));
  }
}).catch(() => {/* best-effort */});
```

---

### CONCERN-4 — Story 3.2.1 (Task 2): `position: absolute` banner inside xterm.js container may be clipped by GPU compositing layer; missing `zIndex` named token

**Status**: Not addressed in Revision 3.

**Detail**: The CSS architecture rules (`.claude/rules/css-architecture.md`) prohibit `position: absolute` overlays without auditing ancestor `transform`/`will-change`. The xterm.js canvas renderer applies `will-change: transform` or `transform: translateZ(0)` for GPU compositing, which creates a new stacking context that clips `position: absolute` children to its bounding box — the banner may be invisible in practice.

Additionally, the plan cites `BrowserTab.css.ts`'s `reconnectingBanner` as a reference template. That file uses a raw `zIndex: 10` number — a project CSS architecture violation (`vars.zIndex.<slot>` required).

**Remediation**:
1. Before writing `TerminalOutput.css.ts` styles, audit the xterm.js wrapper element chain for `transform`, `filter`, or `will-change`. If any ancestor has these properties, switch to `createPortal(..., document.body)` with `position: fixed` instead of `position: absolute`.
2. Add a named slot to `theme-contract.css.ts` (e.g. `vars.zIndex.terminalOverlay`) and reference it instead of the magic number `10`.
3. Do not copy `BrowserTab.css.ts`'s raw `zIndex` number.

---

## Nitpicks

- **Story 1.1.1**: `BackoffState.next()` increments `attempt` as a side effect inside a value-returning method — violates Command-Query Separation. The interface is acceptable but JSDoc must explicitly document the side effect. The pure `jitteredDelay(attempt)` export (Story 1.1.1 Task 2) covers "preview without advancing." No code change required beyond clear JSDoc.

- **Story 2.1.3 (Task 3)**: `reconnectAttemptCount` exposed through `SessionServiceContextValue` as a plain `number` loses the "never reconnected" vs "zero extra reconnects" distinction. Acceptable for this scope; consider `{ count: number; lastAt: number | null }` in a follow-up.

- **Story 4.2.1**: The `.env.example` comment should note that `NEXT_PUBLIC_RECONNECT_V2=false` is semantically equivalent to absent at runtime (Next.js build-time replacement evaluates both as falsy), but "absent = not opted in" communicates intent more clearly to future readers than `false`.
