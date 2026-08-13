# Adversarial Review: client-reconnect

**Date**: 2026-06-23
**Revision**: 4 (post-B6/B7-patch)
**Verdict**: CONCERNS

---

## Resolved Blockers (all revisions)

- [x] **B1**: `websocket-transport.ts` close code gap — Story 1.1.3 Task 2 now covers the terminal transport with the same `onclose` close-code propagation pattern and explicitly notes the `it-ws/client` attachment-point risk.
- [x] **B2**: `isConnectedRef.current` React batching race — Story 3.1.2 Task 0 sets `isConnectedRef.current = false` directly at the top of the `finally` block, before state setters.
- [x] **B3**: `await listSessions` inside `handleSessionEvent` deadlock — Story 2.2.2 uses `needsFullResyncRef` with a fire-and-forget `.then()` placed in `startStream` after the `for await` loop exits.
- [x] **B4**: `event?.type` free variable — both `handleVisibilityOrOnline` functions now declare `(ev: Event)` parameter and use `ev.type !== "online"`. Confirmed in Story 2.1.3 Task 1 and Story 3.1.3 Task 1.
- [x] **B5**: Recursive await in `finally` — Story 3.1.2 Task 1 now uses `setTimeout(() => { if (shouldReconnectRef.current && !isDisconnectingRef.current) connect(); }, delay)` with explicit commentary explaining why `setTimeout` over `await` is required. Confirmed correct.
- [x] **B6**: Dual reconnect scheduling (`catch` + `finally` both schedule `setTimeout` on retriable errors) — Story 3.1.2 Task 2 now explicitly states "do not schedule a reconnect here" in `catch`, with the bold callout: "**Important**: the `catch` block in task 2 must NOT schedule its own `setTimeout` reconnect. All scheduling lives exclusively in `finally`." For non-retriable codes: `catch` sets `shouldReconnectRef.current = false` → `finally` guard evaluates false → no reconnect. For retriable codes: `catch` does nothing → single reconnect scheduled in `finally`. Exactly one callback per attempt.
- [x] **B7**: `watchSessions` captured in stale closure inside `handleVisibilityOrOnline` — Story 2.1.3 Task 1 now adds `watchSessionsRef = useRef(watchSessions)` updated unconditionally on every render, and the `useCallback([], [])` closure calls `watchSessionsRef.current?.(watchOptionsRef.current)` instead of capturing `watchSessions` directly. This is the canonical React "stable accessor via ref" pattern. The stable handler reference is preserved for `removeEventListener`, and the latest `watchSessions` version is always used at call time.

---

## Blockers

*None.*

---

## Concerns

- [ ] **C1: `websocket-transport.ts` Task 2 under-specified — `it-ws/client` wrapping may prevent direct `onclose` access** (carried from Rev 2)

  Story 1.1.3 Task 2 says "find `ws.onclose` in the `fromWebSocket` generator (line ~47) and apply the same pattern," then hedges: "identify the correct close event attachment point." The `it-ws/client` library wraps the native `WebSocket` and may consume socket events internally without exposing `CloseEvent`. If the attachment point is inaccessible, the implementer may fall back to wrapping the thrown error, which cannot carry the close code.

  Recommendation: add a spike task to Story 1.1.3 — read `websocket-transport.ts` lines 40–80 before implementation and confirm whether `stream.socket.onclose` is accessible; if not, document the fallback.

- [ ] **C2: `needsFullResyncRef` `.then()` guard is weaker than the plan claims** (carried from Rev 2)

  Story 2.2.2 Task 1 code: `void clientRef.current?.listSessions({}).then(r => { if (shouldReconnectRef.current) dispatch(setSessions(r.sessions)); })`. The plan prose says this is "gated on the generation counter," but the code only checks `shouldReconnectRef.current`. If a new `startStream` call (generation N+1) starts before the fetch resolves, the dispatch fires into the new generation's session state.

  Fix: capture `const resyncGeneration = myGeneration` before the resync block and add `&& streamGenerationRef.current === resyncGeneration` to the `.then()` guard.

- [ ] **C3: Second reconnect path in `TerminalOutput.tsx` (lines 677–727) still not gated** (carried from Rev 2)

  Story 3.1.2 Task 3 gates the explicit reconnect `useEffect` at lines 779–791 behind `NEXT_PUBLIC_RECONNECT_V2 !== "true"`. The `previousConnectionStateRef` tracking block at lines 677–727 that sets `reconnectTimeoutRef` on disconnect is not mentioned. When the feature flag is on, both the new hook-level reconnect and the component-level `reconnectTimeoutRef` mechanism fire concurrently.

  Recommendation: Story 3.1.2 Task 3 must explicitly audit and gate both reconnect mechanisms.

- [ ] **C4: Terminal scrollback after reconnect has no story** (carried from Rev 2)

  Requirements state "re-request scrollback after reconnection." Story 3.2.1 adds a "--- reconnected ---" separator but nothing about calling `requestScrollback()`. Without this the terminal will appear blank after every auto-reconnect.

  Recommendation: add a task to Story 3.2.1 to call `requestScrollback()` on reconnect, or document the "clear and replay" decision in Unresolved Questions before Phase 3 implementation starts.

- [ ] **C5: Non-awaited `connect()` call has no error handler** (carried from Rev 2)

  In Story 3.1.2 Task 1's reconnect block, `connect()` is called without `.catch()`. If the reconnect attempt throws synchronously before its first `await`, the error is a silently swallowed unhandled promise rejection.

  Recommendation: `connect().catch(e => console.error("[reconnect] terminal connect failed:", e))`.

- [ ] **C6: `connect` stability in `useTerminalStream` visibilitychange effect dep array** (carried from Rev 3)

  Story 3.1.3 Task 1 instructs "include `connect` in the dependency array." If `connect` is not wrapped in `useCallback` (or has many frequently-changing deps), the effect re-registers listeners on every render, causing spurious cleanup+re-register cycles. The plan does not confirm `connect` is a stable `useCallback`. During rapid state updates (e.g. terminal output arriving) this could produce a window where one listener has been removed and its replacement not yet added, silently dropping a visibility event. Not a blocker — React guarantees cleanup runs before re-registration — but should be confirmed during implementation.

- [ ] **C7: Story 4.1.2 and Story 2.2.2 target the same code area without cross-reference** (carried from Rev 3)

  Story 4.1.2 Task 1 says "add generation check after both `listSessions` awaits (lines 805 and 824)." Story 2.2.2 has already added a new `listSessions` call (the seq resync path, fire-and-forget `.then()`). An implementer reading Story 4.1.2 after implementing Story 2.2.2 may apply the generation guard to the *new* `.then()` call (which uses `.then()` not `await` and cannot use an `await`-based guard) rather than the two *original* pre-loop `await` calls at lines 805/824. The plan never cross-references these explicitly.

  Recommendation: add a note to Story 4.1.2 Task 1 clarifying it targets the original pre-loop `await listSessions` calls at lines 805 and 824, not the new post-loop `.then()` from Story 2.2.2 (which uses a generation capture instead).

- [ ] **C8: Always-visible tab: staleness never detected after removing `setInterval`** (carried from Rev 3)

  Story 2.1.3 Task 2 removes the `setInterval` staleness detector entirely and replaces it with `visibilitychange` + `online` event-driven detection. For a tab that remains continuously visible while a WebSocket stream silently dies (e.g. load balancer idle timeout, proxy dropping the connection without a close frame), neither event fires. The stream appears connected but delivers no events indefinitely. The original `setInterval` would have caught this within ~5 seconds.

  Recommendation: retain a coarse `setInterval` (30 s interval) as a backstop for always-visible tabs. The event-driven approach is the primary fast-path; the interval is the fallback for silent failures. These are complementary, not exclusive.

- [ ] **C9: `shouldReconnectRef.current = true` set before early-return guards in `connect()`** (escalated from M4, Rev 3)

  Story 3.1.1 Task 2 sets `shouldReconnectRef.current = true` at the very top of `connect()`, before the `if (isConnectedRef.current || !sessionId) return` guard. With the `visibilitychange` handler now calling `connect()` directly (Story 3.1.3), this path is hit during normal operation. If `disconnect()` set `shouldReconnectRef.current = false` (intentional teardown) and then the visibilitychange handler fires and calls `connect()`, the ref is reset to `true` before the guard check — overriding user intent. Subsequent stream errors would then trigger auto-reconnect despite an intentional disconnect.

  Fix: move `shouldReconnectRef.current = true` to after the early-return guards, not before them.

---

## Minors

- **M1** (carried): Story 2.1.3 Task 1 acceptance criteria says "if `!isConnected`" (implies Redux selector). Ensure the implementation uses `!isConnectedRef.current` (the ref), not the selector, to avoid stale closure.
- **M2** (carried): Story 3.2.1 Task 1 appends "--- reconnected ---" via `onOutput`, which silently no-ops when `onOutput` is absent (shell PTY path). Use `getTerminal()?.write(…)` or `metrics.scheduleOutputUpdate` instead.
- **M3** (carried): Two `aria-live="polite"` regions will both announce "Reconnecting…" simultaneously during a network drop (terminal banner + ConnectionIndicator live region). Low impact but worth coordinating.
- **M5** (carried): `NEXT_PUBLIC_RECONNECT_V2` is inlined at Next.js build time. Setting it in a container's runtime environment after the build has no effect. Add a comment to `.env.example` warning about this.
- **M6** (carried): Story 1.1.3 Task 1 treats `ev.wasClean === true` as a non-retriable clean close. WebSocket code 1001 (Going Away — server restart/pod eviction) sets `ev.wasClean = true` because the close handshake completed, but it is a retriable condition. The plan's comment "clean close or intentional abort" is factually incorrect about `wasClean` semantics. Fix: remove `ev.wasClean` from the condition. Use `signal?.aborted || ev.code === 1000` only. Code 1001 should be retriable.
- **M7** (carried): Story 2.2.2 Task 1 uses `r.sessions` in `.then(r => dispatch(setSessions(r.sessions)))` while Story 4.1.2 uses `sessions.sessions`. These are different variable names for the same response field — not a bug but visually inconsistent. Verify the generated proto field name against `ListSessionsResponse` and use a consistent variable name across both sites.
