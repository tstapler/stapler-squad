# Research: Technology Stack

## Framework / language versions (web-app/package.json)
- **React**: `^19.0.0`, **react-dom**: `^19.0.0`
- **Next.js**: `15.3.2`
- **TypeScript**: `^5.9.3`, `tsconfig.json` has `"strict": true`, `"target": "ES2020"`, `"jsx": "preserve"`
- Package manager: pnpm 10.27.0
- Other relevant libs already present: `@bufbuild/protobuf ^2.11.0`, `@connectrpc/connect-web ^2.1.1`, `@xterm/xterm ^6.0.0` — no new npm deps needed for this fix, matching the requirements' "no new dependencies" constraint.

## Existing visibilitychange / focus / debounce patterns

Two near-identical implementations of a debounced `visibilitychange`+`online` reconnect handler already exist, both gated behind `process.env.NEXT_PUBLIC_RECONNECT_V2 === "true"`, both using a **ref-based** (not useState-based) debounce timer:

1. **`web-app/src/lib/hooks/useSessionService.ts`** (lines ~179, ~967-996)
   - `debounceTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)` (line 180)
   - `dispatchRef = useRef(dispatch)` — kept in sync every render (line 965: `dispatchRef.current = dispatch`) so the handler itself can stay a `useCallback(..., [])` with empty deps, avoiding stale closures without re-registering the DOM listener on every render.
   - `handleVisibilityOrOnline` (useCallback, `[]` deps, line 970-985): checks `document.visibilityState !== "visible" && ev.type !== "online"` to bail early; clears any pending timer; sets a new 200ms `setTimeout` that clears the ref, checks `shouldReconnectRef.current`, and only reconnects if `!isConnectedRef.current || isStale`.
   - Registration effect (line 987-996): gated on `enabled && NEXT_PUBLIC_RECONNECT_V2 === "true"`; registers both `document.addEventListener("visibilitychange", ...)` and `window.addEventListener("online", ...)`; cleanup clears the timer and removes both listeners.

2. **`web-app/src/lib/hooks/useTerminalStream.ts`** (lines 420-446) — the one requirements.md calls out as pre-existing/untouched. Same shape: inline (non-`useCallback`) handler defined inside a `useEffect` with `[]` deps (eslint-disabled for `exhaustive-deps`), `terminalDebounceTimerRef`, 200ms debounce, only fires `connectRef.current()` when `shouldReconnectRef.current && !isConnectedRef.current && !isDisconnectingRef.current`. This only reconnects when disconnected — it does not address the render-desync-while-connected case this fix targets, confirming the requirements doc's claim.

**Takeaway for implementation**: the codebase's established idiom for a debounced visibility/focus handler is a `useRef`-backed timer id + a stable `useCallback`/inline-in-effect handler driven by refs for all values that would otherwise cause stale closures, registered/deregistered in a `useEffect` whose cleanup clears the timer and removes listeners. The new handler required by requirements.md item 1 should follow this exact shape (adding `window.addEventListener('focus', ...)` alongside `visibilitychange`), NOT a debounced-value pattern.

**`useDebounce.ts` root-cause confirmed**: `web-app/src/lib/hooks/useDebounce.ts` (23 lines total).
- `useDebounce<T>(value, delay)` — plain `useState`+`useEffect`, fine as-is (not in scope).
- `useDebouncedCallback<T>(callback, delay)` (lines 31-56) — **is** `useState`-backed: `const [timeoutId, setTimeoutId] = useState<NodeJS.Timeout | null>(null)`. The returned `debouncedCallback` is a plain closure recreated every render (not wrapped in `useCallback`/`useMemo`), so callers get a new function identity each render and two calls landing in the same tick both read the same stale `timeoutId` state (not yet flushed), so `clearTimeout(timeoutId)` no-ops and both fire — the double-fire bug named in requirements.md. Fix per the established codebase pattern above: replace the `useState<NodeJS.Timeout|null>` with `useRef<ReturnType<typeof setTimeout> | null>(null)`, wrap the returned callback in `useCallback` (or `useMemo`) keyed on `[callback, delay]` for a stable memoized identity.

## `useTerminalFlowControl.ts` API (already implemented, exported, unwired)
Return object (line 319-326) includes:
```ts
requestFullResync: (urgent?: boolean) => void;   // line 22 in the props type, impl at line 72
markResyncComplete: () => void;                   // line 308
markPaneResponseReceived: () => void;             // line 312
```
None of these three are currently re-exported from `useTerminalStream.ts`'s return object (line 457 onward only re-exports `sendInput`, `resize`, `requestScrollback`, `sendFlowControl`, `startRecording`, `stopRecording`, etc. — not the resync trio). Requirements item 3 is simply adding these three to that return object (and to the `useTerminalStream` return-type declaration, and to any consuming component's destructure, e.g. wherever `handleManualReconnect` at line 449-455 is exposed alongside).

## Testing stack (web-app/package.json + jest.config.js)
- **Jest** `^30.2.0`, `jest-environment-jsdom` `^30.2.0`, `ts-jest` preset (not Babel/SWC transform).
- `@testing-library/react` `^16.3.0`, `@testing-library/jest-dom` `^6.9.1`, `@testing-library/user-event` `^14.5.2`.
- `jest.config.js` "web-app" project: `testEnvironment: "jest-environment-jsdom"`, `roots: ["<rootDir>/src"]`, path alias `^@/(.*)$` → `<rootDir>/src/$1`, `setupFilesAfterEnv: ["<rootDir>/jest.setup.js"]`.
- Fake-timer convention used consistently across the hook test suite (`useTerminalFlowControl.test.ts`, `useTerminalStream.test.ts`, `useReviewQueueNotifications.test.ts`, `useSessionNotifications.test.ts`): `jest.useFakeTimers()` in `beforeEach`, then `act(async () => { jest.advanceTimersByTime(<ms>); })` (async `act` wrapper — important since these hooks often have promise-based work queued alongside the timer) or `jest.runOnlyPendingTimers()`. `useTerminalStream.test.ts` already has fake-timer tests around line 390-732 including a `300`ms `advanceTimersByTime` for an existing debounce test (line 680/701/732) — good template for the new visibility-resync handler's ~300ms debounce test and the 4000ms stall-watchdog test.

## Lint/type constraints affecting a useRef-based debounce hook
- `web-app/.eslintrc.json` extends `next/core-web-vitals`, plus custom plugins `boundaries`, `analytics`, `@typescript-eslint`.
- `@typescript-eslint/no-non-null-assertion`: `"warn"` — avoid `timerRef.current!`, use explicit null checks (matches existing code's `if (debounceTimerRef.current) clearTimeout(...)` style).
- `@typescript-eslint/consistent-type-imports`: `"warn"` — import hook prop/return types with `import type { ... }`.
- `boundaries/dependencies`: hooks live under `src/lib/**` (`lib` boundary), which may only import from `ui`, `sessions`, `gen` boundaries — irrelevant here since this fix stays within `src/lib/hooks/`.
- No repo-wide ban on `useEffect` deps disabling — the existing patterns above use `// eslint-disable-next-line react-hooks/exhaustive-deps` for intentionally-empty-dep effects driven by refs; the same escape hatch is idiomatic here.
- `NodeJS.Timeout` type (not `number`) is the established type for `setTimeout` return values in refs (see `useSessionService.ts:180`, `ReturnType<typeof setTimeout>` used in that same line and in `useTerminalStream.ts`'s `terminalDebounceTimerRef`) — use `ReturnType<typeof setTimeout>` for portability, matching existing refs.
