# Architecture Research: foreground-fast-reconnect

**Date**: 2026-08-06
**Prior art consulted**: `project_plans/client-reconnect/research/architecture.md` (origin of `terminalBackoffRef`, `BackoffState`, `NEXT_PUBLIC_RECONNECT_V2`, the visibilitychange reconnect effect at `useTerminalStream.ts:432-458`, and the AbortController lifecycle notes reused below).

---

## 1. Where the connect-timeout race belongs

**Answer: inside the message-processing IIFE, racing the wait for the first yielded message — not wrapping the `clientRef.current.streamTerminal(...)` call site.**

`clientRef.current.streamTerminal(messageQueueRef.current, { signal })` (`useTerminalStream.ts:209-212`) returns an `AsyncIterable` synchronously — it does not itself await a network round-trip. The actual connection latency only manifests once the `for await (const msg of stream)` loop (`useTerminalStream.ts:220`) starts pulling, and specifically while waiting for the *first* message (`firstMessage` flag, `useTerminalStream.ts:219-227`), which is what currently flips `isConnected`/`terminalState` from `CONNECTING` to `LOADING`. Wrapping the `streamTerminal(...)` call itself in a timeout would race against a promise that resolves immediately and catch nothing.

### AbortController: reuse `abortControllerRef.current`, don't add a second one

Only one signal consumer exists per attempt (`streamTerminal`'s `{ signal: abortControllerRef.current.signal }`, line 211). The connect-timeout's only job is to call `.abort()` on that same controller if `firstMessage` hasn't arrived in time — `.abort()` is idempotent, and this is exactly the mechanism `disconnect()` already uses for its own 1s force-abort (`useTerminalStream.ts:392-400`). Introducing a second `AbortController` would require somehow combining two signals into the one `signal` field `streamTerminal` accepts (there's no such field for a second signal), so it would have to be `AbortSignal.any([...])`-composed or manually chained — added complexity with no benefit, since both timeout-abort and disconnect-abort want the identical effect (kill this attempt's stream). **Recommendation: one controller, timeout calls the same `.abort()`.**

Critical detail: capture the controller in a **local const at attempt start**, not by re-reading `abortControllerRef.current` when the timer fires:

```ts
abortControllerRef.current = new AbortController();
const attemptController = abortControllerRef.current; // local capture
...
const connectTimeoutTimer = setTimeout(() => {
  console.warn(`[reconnect] stream=terminal connect-timeout-exceeded ms=${connectTimeoutMs}`);
  attemptController.abort();
}, connectTimeoutMs);
```
If a later attempt has since replaced `abortControllerRef.current` with a new controller, a stale timer firing against the old (already-dead) `attemptController` is a harmless no-op abort on an object nobody references anymore — it cannot cross-abort the new attempt. Reading `abortControllerRef.current` live at fire-time would risk exactly that cross-abort.

### Where to clear the timer

Two places, both required:
1. Inside the `if (firstMessage) { ... }` block (`useTerminalStream.ts:221-227`) — clear as soon as the first message arrives, *before* `firstMessage = false`. This is the only place that stops the timer from firing on an otherwise-healthy, already-established connection (clearing only in `finally` would leave the timer live for the entire remaining life of the stream, since `finally` doesn't run until the stream fully ends).
2. Defensively in the outer `finally` block (`useTerminalStream.ts:322-353`) — covers the case where the stream errors/ends before `firstMessage` ever fires, so no dangling timer outlives the attempt.

### What happens when the timeout fires (fits the existing retry path with zero new plumbing)

`attemptController.abort()` causes the `for await` loop to throw. That throw is caught by the existing `catch (err)` (line 313): `getWsCloseCode(err)` returns `null` for an abort (no `ws-close-code` metadata), so it skips the non-retriable-code branch and falls through to `handleError(err)`. The `finally` block then sees `NEXT_PUBLIC_RECONNECT_V2 === "true" && shouldReconnectRef.current && !isDisconnectingRef.current` (all true — this is a timeout, not a user `disconnect()`) and schedules the next attempt via `terminalBackoffRef.current.next()` exactly like any other dropped connection (lines 331-352). **No new reconnect-scheduling code is needed — the connect-timeout only needs to turn "attempt is taking too long" into "abort now," and the pre-existing error/finally machinery does the rest.** This directly satisfies AC6's "connect-timeout-abandonment triggering retry."

---

## 2. Actual prop-drilling path (verified — corrects the requirements doc's stated touchpoints)

The requirements doc lists the chain as `useTerminalStream ← TerminalOutput.tsx ← XtermTerminal.tsx ← SessionDetailView.tsx/SessionDetail.tsx`. That is **not** what the code does:

- `useTerminalStream` is called in exactly one place: `TerminalOutput.tsx:456`.
- `XtermTerminal.tsx` (`grep` confirmed, no `isVisible` prop, no `useTerminalStream` import) is a **leaf display component** — it owns the xterm.js instance and renders decoded output/handles input events, but has no knowledge of connection state or streaming. It does not need to be touched.
- The real chain is: **`SessionDetailView.tsx` → `TerminalOutput.tsx` → `useTerminalStream`**.

### `isVisible` already *is* the "foreground" signal — reuse it, don't invent a new concept

`TerminalOutput.tsx` already has an `isVisible?: boolean` prop (`TerminalOutput.tsx:65`), and `SessionDetailView.tsx` already computes and passes it at all three call sites where `TerminalOutput` is rendered:

- Main session terminal, pooled for keep-alive: `isVisible={poolId === session.id}` (`SessionDetailView.tsx:702`)
- External mux-backed pool: `isVisible={poolPath === session.externalMetadata?.muxSocketPath}` (`SessionDetailView.tsx:683`)
- Shell PTY tabs: `isVisible={activeTabId === shellKey}` (`SessionDetailView.tsx:781`)

This is precisely "the terminal the user just switched to" — `SessionDetailView` keeps every pooled terminal's `TerminalOutput`/`useTerminalStream` instance mounted simultaneously (comment at `SessionDetailView.tsx:746-748`: "matching the TerminalOutput keep-alive pattern"), toggling only `visibility`/`pointer-events` CSS and this `isVisible` flag as the user switches tabs/sessions. Today `isVisible` only drives a fit+focus effect inside `TerminalOutput.tsx:949-956`. It is semantically identical to the requirements' `isSelected`.

**Recommended wiring: thread the existing `isVisible` prop through as `foreground` into `useTerminalStream`, rather than adding a second, parallel prop that would need to be kept in sync with it.** Concretely:
- `TerminalOutput.tsx`: pass `foreground: isVisible` into the `useTerminalStream({...})` call at line 456.
- No changes needed to `SessionDetailView.tsx`'s three call sites — they already pass `isVisible` correctly for this purpose.
- No changes needed to `XtermTerminal.tsx`.

This also means AC4 ("SessionDetailView/XtermTerminal passes `foreground={isSelected}`") is already satisfied structurally by existing code — the only gap is threading `isVisible` one hop further, from `TerminalOutput` into `useTerminalStream`. Update the plan to reflect the real touchpoint list (2 files: `TerminalOutput.tsx`, `useTerminalStream.ts`) rather than 3.

---

## 3. Data flow: does `foreground` need to be read as changing state, and does the mount effect need it as a dependency?

**No — the mount/cleanup effect at `useTerminalStream.ts:416-430` (deps `[sessionId, autoConnect]`) must *not* gain `foreground` as a dependency.** That effect's job is "connect on mount, disconnect on unmount/session-change"; adding `foreground` would make it re-run (disconnect + reconnect) every time the user switches tabs, which is exactly the kind of disruption `isVisible`'s existing fit-only effect (`useTerminalStream.ts` analog, `TerminalOutput.tsx:949-956`) deliberately avoids by using its own separate effect. `foreground` only needs to be *read* at the moment a connect attempt begins (inside `connect()`), not to *drive* new connects/disconnects.

**Pattern to follow: same shape as the existing `isConnectedRef` sync effect (`useTerminalStream.ts:126-128`) plus a transition check, as its own standalone effect — not folded into the visibilitychange effect or the mount effect:**

```ts
const foregroundRef = useRef(foreground ?? false);
const foregroundConnectAttemptRef = useRef(0); // see §4 — deliberately independent of terminalBackoffRef

useEffect(() => {
  const wasForeground = foregroundRef.current;
  foregroundRef.current = foreground ?? false;
  if (!wasForeground && foregroundRef.current) {
    // AC3: false→true transition resets the (existing) backoff delay counter too,
    // matching the precedent already set by the visibilitychange handler
    // (useTerminalStream.ts:442) and handleManualReconnect (line 465).
    terminalBackoffRef.current.reset();
    foregroundConnectAttemptRef.current = 0;
  }
}, [foreground]);
```

This is a third, independent `useEffect`, alongside (not merged into) the mount effect and the visibilitychange effect — consistent with this file's existing convention of one narrowly-scoped effect per concern (ref-sync effect, mount/cleanup effect, visibilitychange effect are already three separate effects for three separate concerns).

`connect()` itself reads `foregroundRef.current` synchronously when computing `connectTimeoutMs`, which is safe because `connect` is always invoked through `connectRef.current` (kept in sync every render at line 364) or directly as a stable `useCallback` — either way it observes the latest ref value at call time, not a stale closure. **`foreground` should be added to `connect`'s own dependency array only if the callback body reads the `foreground` param directly; since it reads `foregroundRef.current` instead, no new dependency is required and `connect`'s identity stays stable.**

---

## 4. The connect-timeout attempt counter must be independent of `terminalBackoffRef` — a discovered gotcha

`connect()` unconditionally calls `terminalBackoffRef.current.reset()` on **every** invocation, including automatic reconnect attempts (`useTerminalStream.ts:166`, inside the guard at the very top of `connect()`). Trace what this means across a real failure sequence:

1. First `connect()` (any caller): `reset()` → `attempt = 0`. Stream fails. `finally` block: `attempt(0) >= 5`? No → `terminalBackoffRef.current.next()` computes a delay from `attempt = 0` and increments to `attempt = 1`; schedules `setTimeout(() => connectRef.current(), delay)`.
2. Timer fires → `connectRef.current()` → `connect()` runs again → **`reset()` fires again at entry, overwriting `attempt` back to `0`** before the next `finally` block ever gets to read it.
3. Every subsequent automatic retry repeats step 2: the delay computed by `.next()` is always based on `attempt = 0`, and the `attempt >= 5` give-up check in the `finally` block (line 334) can never observe an `attempt` greater than what `.next()` itself set two lines earlier in the *same* `finally` call — i.e. it can never reach 5. **In the current code, `terminalBackoffRef.current.attempt`, read at `connect()`-entry time, is always 0 in practice**, because `reset()` runs before anything else gets a chance to observe a nonzero value.

No existing test in `useTerminalStream.test.ts`'s `describe('useTerminalStream — auto-reconnect ...')` block (lines 382-762) currently exercises 3+ consecutive automatic reconnect failures or asserts growing delay / give-up-at-5 behavior, so this has shipped unnoticed. **This is out of scope to fix here** (fixing it would change existing, tested backoff-delay semantics and expand this change's blast radius well beyond "add a connect timeout") — but it is disqualifying for the naive approach of gating "first 2 attempts" on `terminalBackoffRef.current.attempt`, since that value is always 0 at the point `connect()` would need to read it.

**Recommendation: add a dedicated, independent counter, `foregroundConnectAttemptRef` (shown in §3), that is:**
- incremented once per `connect()` attempt (anywhere in `connect()`'s synchronous setup, e.g. right after computing `connectTimeoutMs`), regardless of `foreground`'s value — cheap, and its value is meaningless when `foreground` is false since it's never read in that branch.
- reset to `0` specifically on the `foreground` false→true transition effect (§3), which is what AC3 ("foreground false→true transition resets backoff attempt counter") is actually asking for: not the pre-existing `terminalBackoffRef` semantics (those already get `.reset()` for other reasons — visibility change, manual reconnect — and per the trace above are *already* effectively always-0 on every attempt), but a **new** counter whose entire purpose is "how many connect attempts have happened since this terminal most recently became foreground," which is exactly what determines whether attempt N gets the fast or the normal connect-timeout.

```ts
// inside connect(), after the existing terminalBackoffRef.current.reset() call:
const useFastConnectTimeout = foregroundRef.current && foregroundConnectAttemptRef.current < 2;
const connectTimeoutMs = useFastConnectTimeout ? FOREGROUND_CONNECT_TIMEOUT_MS /* ~1200-1500 */ : NORMAL_CONNECT_TIMEOUT_MS /* ~3500 */;
foregroundConnectAttemptRef.current += 1;
```

Open question for the planning phase (not resolved by this research, flagging explicitly rather than silently picking one): should `foregroundConnectAttemptRef` also reset to `0` on a *successful* connect (first message received), so that a terminal which used up its 2 fast attempts during an initial flaky startup still gets 2 fresh fast attempts on some later disconnect while still foreground? The AC text doesn't say, and the herdr-web source file cited in the requirements (`web/src/terminalReconnectPolicy.ts`) lives in a different repository, not this one, so it can't be inspected from here to settle it by precedent.

---

## 5. Which reconnect path(s) this applies to (AC5)

**This feature is exclusively part of the `NEXT_PUBLIC_RECONNECT_V2` hook-level path inside `useTerminalStream.ts`.** Confirmed by reading both paths in full:

- **V2 path** (`process.env.NEXT_PUBLIC_RECONNECT_V2 === "true"`): all automatic reconnect logic — `shouldReconnectRef`, `terminalBackoffRef`/`BackoffState`, the `finally`-block retry scheduling (lines 330-352), and the visibilitychange/online listener (lines 432-458) — lives entirely inside `useTerminalStream.ts`. This is the only path with a concept of "automatic reconnect attempt" for the connect-timeout to attach to, and the only place `foreground` can meaningfully change attempt timing.
- **Pre-flag path** (`NEXT_PUBLIC_RECONNECT_V2` unset/falsy, the default per `web-app/.env.local.example:1`): confirmed by reading `TerminalOutput.tsx:759-770` — when the flag is off, a dropped connection does **not** automatically retry at all. `TerminalOutput.tsx`'s own `reconnectTimeoutRef` only starts a 5-second timer that flips `setShowReconnectButton(true)` (line 767), i.e. it surfaces a manual "reconnect" button for the user to click; it never calls `connect()` itself. There is no automatic attempt sequence in this path for a connect-timeout to shorten.

Since `NEXT_PUBLIC_RECONNECT_V2` defaults off, **this feature has no observable effect until V2 is enabled** — consistent with AC0 ("no behavior change when omitted") as long as the new `foreground` option and its internal logic are only consulted from inside the `if (process.env.NEXT_PUBLIC_RECONNECT_V2 === "true" ...)` branches that already gate all other V2-only behavior (finally-block retry scheduling, visibilitychange listener). The `connect()`-entry computation of `connectTimeoutMs`/`foregroundConnectAttemptRef` increment can stay outside that `if`, since it's inert (an unused local + an incremented ref) when V2 is off and no reconnect ever gets scheduled to consume it — but the `setTimeout` that starts the connect-timeout race itself should be skipped (or at minimum made a no-op) when V2 is off, to avoid a spurious `.abort()` firing against a connection that this feature isn't supposed to be managing outside the V2 flag's scope. **Recommend gating the entire connect-timeout timer setup behind the same `NEXT_PUBLIC_RECONNECT_V2` check used elsewhere in this file**, for consistency with how every other piece of this hook's auto-reconnect behavior is flagged.

---

## 6. Summary of concrete integration points for the plan phase

| File | Change |
|---|---|
| `web-app/src/lib/hooks/useTerminalStream.ts` | Add `foreground?: boolean` to `UseTerminalStreamOptions`; add `foregroundRef` + `foregroundConnectAttemptRef` + their transition-effect (§3); compute `connectTimeoutMs` in `connect()` and race it against the first-message wait using the existing `abortControllerRef` (§1), gated behind `NEXT_PUBLIC_RECONNECT_V2` (§5) |
| `web-app/src/components/sessions/TerminalOutput.tsx` | Pass `foreground: isVisible` into its existing `useTerminalStream({...})` call (line 456) — `isVisible` is already threaded in correctly from `SessionDetailView.tsx`, no new prop needed here beyond this one-line addition |
| `web-app/src/components/sessions/SessionDetailView.tsx` | **No change** — already passes `isVisible` correctly at all 3 `TerminalOutput` call sites |
| `web-app/src/components/sessions/XtermTerminal.tsx` | **No change** — does not own `useTerminalStream`, is a display-only leaf; the requirements doc's inclusion of this file as a touchpoint does not match the actual component ownership |
| `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts` | New `describe` block(s) for: fast vs normal timeout selection by `foreground` + attempt count, `foregroundConnectAttemptRef` reset on false→true transition, and connect-timeout-driven abort triggering the existing retry path (reuse fake-timer patterns already established in the `auto-reconnect` describe block, lines 382-762) |
