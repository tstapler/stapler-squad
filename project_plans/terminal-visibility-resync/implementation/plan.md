# Implementation Plan: terminal-visibility-resync

**Feature**: Force a clean xterm.js repaint when a backgrounded browser tab regains
visibility/focus while a terminal stream is still connected but silently
render-desynced, and fall back to real reconnection when it's actually
disconnected — closing the gap left by the existing disconnect-only
`NEXT_PUBLIC_RECONNECT_V2` listener.
**Date**: 2026-07-24
**Status**: Ready for implementation (revised after BLOCKED architecture +
adversarial review — see "Revision Notes" below)
**ADRs**: None — see "ADR decision" note at the end of this document for why.
**System type**: Small, targeted bug fix wiring together existing-but-unwired
hooks in a React/TypeScript frontend (Next.js 15 / React 19 / xterm.js 6). Not
a new subsystem — no new dependency, no schema/data changes, no new service
boundary. Total novel logic is ~50-150 lines across 4 files plus tests.

---

## Revision Notes (post-review, 2026-07-24)

Both the architecture review and the adversarial review returned **BLOCKED**
on the first draft of this plan. This revision resolves every Blocker and
Concern from both files. Architecture and adversarial review independently
converged on the same structural fix (extract the new logic into a standalone
hook) and the same in-flight-`connect()` race — treated below as one fix
satisfying two findings, not two separate fixes.

| # | Finding (source) | Resolution |
|---|---|---|
| 1 | Architecture Blocker: `connect`/`disconnect` declared `() => void` but are `async` | `TerminalStreamResult` types widened to `Promise<void>`-returning (Task 1.1.1d); `npx tsc --noEmit` re-run in Phase 2 after `TerminalOutput.tsx`/`useVisibilityResync.ts` are edited (Task 2.1.7) |
| 2 | Adversarial Blocker 1: no cleanup of resync/watchdog refs on session switch | The extracted hook owns a `[sessionId]`-keyed cleanup effect (Story 2.1.6) |
| 3 | Adversarial Blocker 2: `connect()` race / toothless regression test | Disconnected-branch fallback gated on `terminalState !== 'CONNECTING' \|\| 'LOADING'` in addition to `!isConnected`; Task 2.1.3c replaced with a real single-call assertion |
| 4 | Both reviews' structural Concern: extract to a hook | New `web-app/src/components/sessions/useVisibilityResync.ts`, called by `TerminalOutput.tsx`; `useTerminalStream.ts` and `XtermTerminal.tsx` untouched |
| 5 | Architecture Concern: Epic 1.2/`useDebouncedCallback` mismatch | The hook's debounce now calls `useDebouncedCallback` directly instead of hand-rolling an equivalent ref+timer |
| 6 | Adversarial Concern: try/catch + arm order | `requestFullResync(true)` wrapped in try/finally; watchdog timer armed in the `finally` block unconditionally |
| 7 | Adversarial Concern: rapid flap re-arms watchdog | Connected branch no-ops if `pendingResyncCompletionRef.current` is already `true` |
| 8 | Adversarial Concern: AC1 test can pass for the wrong reason | Added a focus-only test and a >300ms-apart-produces-two-calls test |
| 9 | Adversarial Concern: untested false-positive heuristic | Added a test for unrelated output mid-window not firing the watchdog |
| 10 | Adversarial Concern: scope drift (Phase 5) | Phase 5 (aria-live) dropped entirely; tracked as a separate follow-up |
| — | Nitpicks (comment on heuristic, `sessionId` in logs, cross-reference comment) | Folded into the relevant tasks below |

**Second review pass** (both re-reviews returned CONCERNS, not BLOCKED — proceeding is allowed per the repair-loop exit condition, but the two concerns below were cheap and real, so they're fixed in this revision rather than left open):

| # | Finding (source) | Resolution |
|---|---|---|
| 11 | Architecture re-review Concern: `UseVisibilityResyncParams.terminalState` typed `string` instead of the exported `TerminalState` union, weakening the `CONNECTING`/`LOADING` gate's compile-time safety | Task 2.1.1a now imports and uses `TerminalState` from `useTerminalStream.ts` |
| 12 | Both re-reviews' Concern: session-switch cleanup (Story 2.1.6) clears an already-armed watchdog, but not a debounce timer still pending (not yet fired) when the switch happens — a stale callback can still fire against the new session's `connect`/`disconnect` | Task 2.1.1b now guards `handleVisibilityOrFocusResyncInner` with `if (sessionId !== sessionIdRef.current) return;` (new `sessionIdRef`, Task 2.1.1a) — the frozen closure `sessionId` vs. the live ref catches a mid-flight switch without needing `useDebouncedCallback` to expose a cancel handle. New Task 2.1.6c regression test. |

Residual, explicitly accepted (not fixed, documented as out of scope for this minimal-diff fix — see "Risk Control" below for the first): the `terminalState` gate only incidentally avoids racing the pre-existing V2 listener (200ms debounce < this hook's 300ms, not enforced by any shared coordination), and the stall watchdog's `disconnect().then(connect)` has a narrow window where its own re-entrancy guard (`pendingResyncCompletionRef`) is cleared before `disconnect()`'s up-to-1s resolution actually settles, in principle allowing a second overlapping connect attempt from an unrelated trigger during that window. Both are pre-existing classes of race in the surrounding connection-management code (not introduced by this change, not made meaningfully worse by it), and fixing them requires touching `connect()`/`disconnect()`'s internals in `useTerminalStream.ts` well beyond this fix's scope.

---

## Step 0.5 — CREATIVE pass (alternatives explored)

**Approach A — Extend the existing `useTerminalStream.ts` `NEXT_PUBLIC_RECONNECT_V2`
listener (lines 420-446) to also handle the connected-but-stale case.**
- Strength: reuses one already-registered `visibilitychange`/`focus`-adjacent
  listener instead of adding a second one, minimizing new event-registration
  surface.
- Weakness: `showReconnectButton` is `TerminalOutput.tsx`-local `useState`
  (`useTerminalStream.ts` has no visibility into it), so this approach would
  require threading a new callback prop across the hook boundary and would be
  the first case of hook-internal code *initiating* a `flowControl.*` action
  rather than exposing it for the component to call — breaking the
  established one-directional flow (hook exposes capabilities → component
  decides when to invoke them) that every other `flowControl.*` call site in
  this codebase follows today. It also risks touching code adjacent to the
  explicitly protected V2 listener, raising AC6 regression risk for no
  offsetting benefit.

**Approach B (CHOSEN) — Add a new, independent `visibilitychange`/`focus`
listener + stall watchdog inside `TerminalOutput.tsx`,** consuming the three
newly-exposed `useTerminalStream` functions (`requestFullResync`,
`markResyncComplete`, `markPaneResponseReceived`), using its own distinct
timer refs, and following the same "shared handler + single debounce-timer
ref + guards re-checked at fire time" shape the existing V2 listener already
proves works for this exact multi-event coalescing problem.
- Strength: `showReconnectButton` stays owned and set from a single file, zero
  new prop/callback plumbing, the V2 listener is untouched byte-for-byte
  (satisfies the "leave as-is" requirement directly rather than by
  incidental omission), and it's a natural continuation of the pattern
  `TerminalOutput.tsx` already uses at its connection-state effect (lines
  667-733) — "on a connection-relevant transition, decide what to do next."
- Weakness: two structurally similar `visibilitychange` listeners will now
  exist in the codebase (V2's disconnect-only one, and this feature's
  resync-or-reconnect one), which reads as duplicative to a future maintainer
  unless clearly commented as solving disjoint problems (disconnected-only
  vs. connected-but-stale).

**Approach C — Extract a shared `useVisibilityRefocus` hook that both the V2
reconnect-on-disconnect logic and the new resync-while-connected logic call
into, unifying the two listeners into one.**
- Strength: would reduce total `document`-level `visibilitychange` listener
  registrations from two to one, theoretically the "cleanest" long-term
  shape.
- Weakness: disqualified outright — implementing it requires refactoring the
  V2 listener's internals out of `useTerminalStream.ts`, which directly
  violates the requirement (and AC6) that the three pre-existing triggers,
  including the V2 listener's file, are left byte-for-byte unmodified. Not a
  close call.

**Decision**: Approach B. Both A and C are recorded as rejected alternatives
in the Pattern Decisions table below.

**Post-review refinement of Approach B (2026-07-24)**: both the architecture
and adversarial reviews independently recommended extracting the new
listener/watchdog logic out of `TerminalOutput.tsx` (1704 lines) into a
standalone hook, `web-app/src/components/sessions/useVisibilityResync.ts`,
called by `TerminalOutput.tsx`. This is **not** a switch to Approach C: it
does not touch `useTerminalStream.ts`'s protected `NEXT_PUBLIC_RECONNECT_V2`
listener at all (the disqualifying reason C was rejected), and it does not
touch `XtermTerminal.tsx`. It is a refinement of *where Approach B's own new
code lives* — same event-listener strategy, same guards, same refs, just
owned by a small dedicated hook file in the same directory instead of inlined
into an already-1700-line component. This also resolves the Epic 1.2 mismatch
Concern (§Pattern Decisions) and gives Story 2.1.6's session-switch cleanup a
natural home (a `[sessionId]`-keyed effect inside the hook, rather than
threading cleanup into `TerminalOutput.tsx`'s existing 927-992 session-switch
effect). All references to "the new `useEffect` in `TerminalOutput.tsx`" in
the original draft below are superseded by "the new hook" in Phase 2.

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or
variable name in this change. Names marked "existing" already exist in the
codebase and must be reused verbatim, not renamed.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `requestFullResync(urgent?: boolean)` | Existing `useTerminalFlowControl` function that sends a `CurrentPaneRequest` proto message over the live stream to force the server to re-capture and resend the full tmux pane. `urgent=true` bypasses its internal 2000ms throttle. | Existing (`useTerminalFlowControl.ts:72`); this change only re-exports it from `useTerminalStream`. |
| `markResyncComplete()` | Existing function that clears `isResyncingRef.current` to `false`. | Existing (`useTerminalFlowControl.ts:308`); re-exported, not modified. |
| `markPaneResponseReceived()` | Existing function that clears `waitingForPaneResponseRef.current` to `false`. | Existing (`useTerminalFlowControl.ts:312`); re-exported, not modified. |
| `isResyncingRef` / `waitingForPaneResponseRef` | Existing refs (no React state) tracking whether a resync RPC round-trip is in flight; accessed externally only via `getIsResyncingRef()`/`getWaitingForPaneResponseRef()` getters. | Existing; this change does not add new getters — `disconnect()` already uses `getIsResyncingRef()`. |
| `TerminalStreamResult` | The TypeScript return-type interface of `useTerminalStream` (`useTerminalStream.ts:54-71`), grown by three new always-present fields in this change, **and** with `connect`/`disconnect` widened from declared `() => void` to their actual `Promise<void>`-returning signatures (`connect: (cols?: number, rows?: number) => Promise<void>`, `disconnect: () => Promise<void>`) — a backward-compatible widening; every existing call site already treats them as fire-and-forget. | Existing type, extended + corrected. |
| `useDebouncedCallback<T>(callback, delay)` | Existing (previously dead-code, buggy) hook in `useDebounce.ts` that returns a debounced, referentially-stable version of `callback`. Fixed in this change to use a `useRef` timer id instead of `useState`. **Its first real production call site is `useVisibilityResync.ts`** (below), which calls it directly rather than hand-rolling an equivalent ref+timer pattern — this makes Epic 1.2's "first real consumer" framing true of the code, not just the plan. | Existing, root-cause-fixed; one real call site added in this change (`useVisibilityResync.ts`), zero regression risk to other callers since there are none yet. |
| `useVisibilityResync` | **New** standalone hook, `web-app/src/components/sessions/useVisibilityResync.ts`. Owns the `visibilitychange`/`focus` listener, the debounce (via `useDebouncedCallback`), the stall watchdog, and a `[sessionId]`-keyed cleanup effect. Takes `sessionId`, `isConnected`, `terminalState`, `connect`, `disconnect`, `requestFullResync`, `markResyncComplete`, `markPaneResponseReceived`, `setShowReconnectButton` as parameters; returns `{ notifyResyncOutputReceived }` for `TerminalOutput.tsx`'s `handleOutput` to call. Lives beside `TerminalOutput.tsx`, never imports from or modifies `useTerminalStream.ts`'s protected V2 listener, never imports `XtermTerminal.tsx`. | New. Extraction target for all of Story 2.1.1-2.1.6 (see Step 0.5's "Post-review refinement" note). |
| `handleVisibilityOrFocusResyncInner` | **New** stable (empty-deps `useCallback`) handler inside `useVisibilityResync.ts` that reads all its inputs from refs (never from closure-captured props directly), so it can be passed to `useDebouncedCallback` once and never need to be recreated. Branches on `isConnected`/`terminalState` at fire time. | New, `useVisibilityResync.ts`. Renamed from the original draft's `handleVisibilityOrFocusResync` (now the *inner*, undebounced handler — `useDebouncedCallback`'s returned function is the one actually registered as the event listener) to keep the debounce wiring visible in the name. |
| `resyncStallTimerRef` | **New** ref, inside `useVisibilityResync.ts`, holding the pending `setTimeout` id for the 4000ms stall watchdog armed each time `requestFullResync(true)` is sent while connected. | New, `useVisibilityResync.ts`. Independent clock/ref from `useDebouncedCallback`'s internal `timeoutIdRef` (per pitfalls.md guardrail #3) — the debounce timer and the stall timer are never the same ref. |
| `pendingResyncCompletionRef` | **New** ref-based boolean flag, inside `useVisibilityResync.ts`. Set to `true` the instant `requestFullResync(true)` is called (in a `try/finally`, so a synchronous throw doesn't skip arming the watchdog); read and cleared to `false` either by the stall watchdog firing or by `notifyResyncOutputReceived()` being called (i.e. `TerminalOutput.tsx`'s `handleOutput` received *any* output while the flag is set — the same no-correlation-ID heuristic the pre-existing resize→resync path already relies on). Also gates re-entrancy: the connected branch no-ops (does not re-issue `requestFullResync` or re-arm the watchdog) if this flag is already `true`. **Implementation must include an inline comment at its declaration** noting the no-correlation-ID heuristic and that this is an accepted, tested trade-off (see Task 2.1.5b/2.1.3-tests). | New, `useVisibilityResync.ts`. This is the mechanism that lets the stall watchdog know a resync succeeded, and the re-entrancy guard that prevents rapid visibility flapping from duplicating in-flight resyncs. |
| `notifyResyncOutputReceived` | **New** function returned by `useVisibilityResync()`. Called from `TerminalOutput.tsx`'s `handleOutput` on every output chunk; internally checks `pendingResyncCompletionRef` and clears it (plus the stall timer, plus calling `markResyncComplete()`/`markPaneResponseReceived()`) only when a resync is actually pending. | New, `useVisibilityResync.ts` (defined) / `TerminalOutput.tsx` (called). Replaces the original draft's plan of inlining the completion check directly into `handleOutput`. |
| `RESYNC_DEBOUNCE_MS` | **New** constant, value `300`. The debounce window for coalescing back-to-back `visibilitychange` + `focus` events into a single resync attempt, passed as `useDebouncedCallback`'s `delay` argument. | New, `useVisibilityResync.ts` module scope. |
| `RESYNC_STALL_TIMEOUT_MS` | **New** constant, value `4000`. Time after which a pending resync with no output response is considered stalled. | New, `useVisibilityResync.ts` module scope, per requirements.md's explicit name. |
| `clearAndHome` | The ANSI escape prefix the server prepends to every `CurrentPaneRequest` response: `\x1b[!p` (DECSTR) + `\x1b[2J` (ED2 erase-screen) + `\x1b[H` (CUP cursor-home) = `"\x1b[!p\x1b[2J\x1b[H"`. Mirrors the server's `ansiSnapshotPrefix` constant (`server/services/connectrpc_websocket.go:129`, used verbatim at `:1204` and `:1409`). Both the test constant and the server constant must carry a `// keep in sync with ansiSnapshotPrefix (connectrpc_websocket.go)` / `// keep in sync with clearAndHome (TerminalStreamManager.resync.test.ts)` comment pointing at each other. | Test-only constant, defined locally in the new integration test — this repo's frontend has no shared TS constant for it today, and none is being added since the test is the only consumer. |
| `TerminalStreamManager.write(output: string)` | Existing method (`web-app/src/lib/terminal/TerminalStreamManager.ts:228-248`); the single funnel both ordinary streamed output and resync responses flow through. | Existing, unmodified — this is the AC7 integration test's call target, not `writeInitialContent()`. |

**Glossary term count**: 14 (11 new, 3 existing-but-newly-surfaced/corrected).

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `useVisibilityResync` extraction & placement | Custom-hook extraction (Approach B, refined) — new DOM event listener + watchdog live in a standalone hook, `web-app/src/components/sessions/useVisibilityResync.ts`, called by `TerminalOutput.tsx`, mirroring the existing `TerminalOutput.tsx` connection-state effect's responsibility but isolated for testability | GoF Observer; architecture.md §2; both architecture-review.md and adversarial-review.md independently recommended this refinement | (1) Approach A: initiate the resync from inside `useTerminalStream.ts`. (2) Approach C: unify with the V2 listener into one shared hook. (3) Original Approach B draft: inline everything directly into `TerminalOutput.tsx`'s body. | (1)/(2) both require touching `useTerminalStream.ts`'s protected V2 listener internals (AC6 violation) or new cross-hook prop plumbing for `showReconnectButton`, breaking the codebase's one-directional "hook exposes → component invokes" pattern. (3) does **not** violate AC6 (never touches `useTerminalStream.ts` or `XtermTerminal.tsx`) but adds ~150 lines of stateful listener/watchdog logic to an already-1704-line file and forces every test to mount the full component tree instead of `renderHook`-testing isolated logic — a real testability/SRP cost with no offsetting benefit once a same-directory hook file is just as AC6-safe. |
| `useDebouncedCallback` timer identity | `useRef`-backed timer id + `useCallback`-memoized return, matching the ref-mirroring idiom already used by `isConnectedRef` (`useTerminalStream.ts:98-119`), `connectRef` (`useTerminalStream.ts:104`), and `onDataRef`/`onResizeRef` (`XtermTerminal.tsx:121-126`) | type-driven-design (mutable-non-rendered state belongs in a ref, not `useState`) | Current `useState<NodeJS.Timeout \| null>` implementation | `setTimeoutId` is applied asynchronously by React's batching, so a second same-tick call reads the pre-update stale `timeoutId` and never clears the first timer — the exact double-fire bug requirements.md names. |
| `useVisibilityResync`'s debounce mechanism | Calls `useDebouncedCallback(handleVisibilityOrFocusResyncInner, RESYNC_DEBOUNCE_MS)` directly — the inner handler is a stable, empty-deps `useCallback` that reads all live values via refs, so it never needs to be recreated and `useDebouncedCallback`'s own memoization/ref-timer logic (Epic 1.2) does the actual coalescing | Direct reuse over reimplementation | Hand-rolling an equivalent ref+`setTimeout` clear/set pattern inside the hook (the original draft's approach) | The original draft's Task 2.1.1d never called `useDebouncedCallback`, so Epic 1.2's stated rationale ("first real consumer") was false of the code as written (architecture review Concern). The inner handler's ref-based reads make it compatible with `useDebouncedCallback`'s memoized-callback contract with no loss of freshness — there was no genuine technical blocker, so the default (make the stated relationship true) applies. |
| `connect()`/`disconnect()` declared types | Widen `TerminalStreamResult.connect`/`.disconnect` to their real `Promise<void>`-returning signatures (Task 1.1.1d) | Backward-compatible type widening | Leaving the types as `() => void` and writing `.then()` off an untyped/`any`-cast reference, or avoiding the promise chain entirely | The declared types were simply wrong (architecture review Blocker) — every existing call site already treats these as fire-and-forget, so widening is zero-risk, and the new watchdog code genuinely needs `.then()`. |
| Disconnected-branch fallback gating | Gate on `terminalState !== 'CONNECTING' && terminalState !== 'LOADING'` in addition to `!isConnected`, before calling `connect()`/`setShowReconnectButton(true)` | Minimal guard against the documented `connect()` in-flight race | (1) Add a new `isConnectingRef` guard inside `connect()` itself. (2) Ship the original draft's toothless "no error thrown" test and accept the race undocumented-but-tested | (1) is a larger, riskier change to a shared function with many call sites, for a fix whose minimal-diff constraint favors gating the *new* caller instead. (2) was explicitly called a Blocker by adversarial review — a fix whose entire purpose is repaint correctness cannot ship a new caller capable of opening a duplicate stream with only a non-asserting test. This single gate also resolves the architecture/adversarial Concern about not special-casing `CONNECTING`/`LOADING` — one fix, two findings. |
| Resync-completion detection | Ref-based "next output completes the pending resync" sentinel (`pendingResyncCompletionRef`), exposed to `TerminalOutput.tsx` via the hook's returned `notifyResyncOutputReceived()`, reusing the same no-correlation-ID heuristic `useTerminalFlowControl` already relies on internally (`isResyncingRef`/`waitingForPaneResponseRef`) | Sentinel/heuristic pattern, precedented by the existing resize→resync path (`useTerminalFlowControl.ts:219-237`) | Tag `CurrentPaneRequest`/response with a correlation ID and match on receipt | Requires a server-side proto field addition — out of scope per requirements' "no new dependency, reuse existing hooks" constraint, and the existing resize-triggered resync already ships today on the same imprecise heuristic, so consistency favors reusing it rather than introducing an asymmetric, more-precise mechanism for only this one path. This trade-off is now locked in by an executable test (Task 2.1.3d) rather than left as prose only. |
| Re-entrancy while a resync is already pending | Connected branch no-ops (does not call `requestFullResync` again, does not re-arm the watchdog) whenever `pendingResyncCompletionRef.current === true` | Idempotent-trigger guard | Always re-issuing `requestFullResync`/re-arming the watchdog on every qualifying transition | Rapid hidden→visible flapping (OS-level app switching, multi-monitor focus changes) would otherwise keep resetting the watchdog and firing duplicate `CurrentPaneRequest`s, masking a genuine stall indefinitely (adversarial review Concern). |
| Synchronous-throw safety around `requestFullResync(true)` | `try { requestFullResyncRef.current(true); } finally { <arm stall timer> }` — the watchdog timer is armed unconditionally in the `finally` block, so a synchronous throw is treated the same as a stall (still recoverable via the watchdog) instead of leaving `pendingResyncCompletionRef` stuck `true` forever with nothing to clear it | try/finally over try/catch-and-swallow | try/catch that resets `pendingResyncCompletionRef` to `false` on error and skips arming the timer | requirements.md's underlying goal (self-healing) is better served by treating "threw synchronously" the same as "never got a response" — both recover via the same disconnect+reconnect path 4000ms later — rather than by silently giving up with no recovery attempt at all. |
| Session-switch cleanup | `useVisibilityResync` owns its own `[sessionId]`-keyed `useEffect` cleanup (clears `resyncStallTimerRef`, resets `pendingResyncCompletionRef`, defensively calls `markResyncComplete()`/`markPaneResponseReceived()`) rather than folding into `TerminalOutput.tsx`'s existing 927-992 session-switch effect | Effect-per-owner — the hook that owns the refs also owns their session-scoped cleanup | Threading a cleanup call into `TerminalOutput.tsx`'s existing session-switch effect | Once the logic is extracted into its own hook (see extraction decision above), giving that hook its own `[sessionId]` effect is simpler and keeps the refs' lifecycle fully inside the file that declares them — no action-at-a-distance between two files' effects touching the same refs. |
| Stall watchdog recovery action | Reuse the hook's own guarded `disconnect()`/`connect()` (already serializes against in-flight resyncs via the 500ms retry-if-busy branch in `disconnect()`, `useTerminalStream.ts:359-401`) | Adapter (GoF) — the watchdog adapts a timeout into a call against an existing, already-guarded interface rather than reimplementing socket teardown | A bespoke stream-teardown/reopen sequence written inside the watchdog itself | Reimplementing would duplicate `disconnect()`'s existing "delay disconnect while a resync is in flight" guard and risk a second, un-coordinated teardown path racing the first. |
| Transient-state UI during the 0-4s resync/watchdog window | Reuse the existing `showReconnectBanner` / `reconnectingBanner` div (`TerminalOutput.tsx:735-761`, `:1600-1603`) once `pendingResyncCompletionRef` has been true for ≥2s — **implemented in Story 2.1.8** (`RESYNC_BANNER_DELAY_MS`/`resyncBannerTimerRef`, wired through `notifyResyncOutputReceived` and the session-switch cleanup); triad-review UX lens flagged that the first draft of this plan stated this decision here but shipped no implementing task, silent for the full 0-4s window instead of the researched 0-2s-silent/2-4s-banner split | Strategy re-use — same visible state machine already handles "maybe fine, watch and wait" → "actually reconnecting" | A new, resync-specific "Refreshing terminal…" banner/spinner | Per ux.md: the two states are externally indistinguishable to the user, doubling UI/a11y surface for a distinction only engineers would find interesting; industry precedent (ttyd/mosh/VS Code Remote-SSH complaints) favors silent-success + one reused indicator over a second bespoke one. |
| `document.activeElement` preservation | No `.focus()` call anywhere in the new code path; reconnect UI toggles a sibling banner `<div>`, never the `XtermTerminal` mount key | Explicit non-pattern (absence of behavior) — verified by a dedicated regression test (Story 2.1.4) | Calling `XtermTerminalHandle.focus()` after a successful resync/reconnect, "helpfully" restoring keyboard focus | Explicitly excluded by requirements.md ("Never moves keyboard focus") and AC3; conditionally remounting `XtermTerminal` to show reconnect UI would tear down and recreate xterm's hidden textarea, an easy accidental focus-steal vector flagged in pitfalls.md §6. |

---

## Migration Plan

N/A — no schema, database, or persisted-state changes. This is a pure client-side (`web-app/src`) TypeScript/React change.

---

## Observability Plan

- **Logs**: match the existing bracket-tagged `console.log`/`console.info`/`console.warn` convention already used throughout `useTerminalStream.ts` and `useTerminalFlowControl.ts` (e.g. `[reconnect] stream=terminal trigger=...`, `[useTerminalFlowControl] Requesting full resync...`). Every new `[resync]` log line includes `sessionId=` so cross-session misfires (every mounted pool session resyncs independently off one `visibilitychange` event, by design) are disambiguable in production logs when several sessions fire simultaneously (adversarial review Minor). New log lines, all emitted from `useVisibilityResync.ts`:
  - `` console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus delay=0ms`) `` when the debounced handler fires and the connected branch runs `requestFullResync(true)`.
  - `` console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus fallback=connect`) `` when the disconnected branch runs.
  - `` console.warn(`[resync] sessionId=${sessionId} requestFullResync threw synchronously`, err) `` when the try/finally's catch branch runs.
  - `` console.warn(`[resync] sessionId=${sessionId} stall watchdog fired after ${RESYNC_STALL_TIMEOUT_MS}ms, forcing disconnect+reconnect`) `` when the watchdog forces recovery.
  - `` console.log(`[resync] sessionId=${sessionId} pane response received, resync complete`) `` inside `notifyResyncOutputReceived()` when `pendingResyncCompletionRef` clears normally.
- **Metrics**: none new. This codebase has no frontend metrics/telemetry pipeline for terminal streaming beyond the existing `useAnalytics().track(...)` calls used for discrete user actions (button clicks, etc.) — this is a self-healing background path with no operator-facing dashboard today, and adding one is out of scope for a client-side glue fix. If a future incident makes stall-frequency worth tracking, an `analytics.track('terminal_resync_stall')` call is a one-line fast-follow, not part of this change.
- **Alerts**: no new alerts required — no backend/infra surface changes.

---

## Risk Control

- **Feature flag**: not gated by a new flag. The new listener fires unconditionally (not gated behind `NEXT_PUBLIC_RECONNECT_V2`, which continues to gate only the pre-existing disconnect-only listener in `useTerminalStream.ts` per the "left as-is / additive" requirement). Rationale for going straight to full rollout: the underlying primitives (`requestFullResync`, `connect`, `disconnect`) are already exercised in production today via the resize path and the V2 listener — the only new risk surface is the wiring of *when* they're called, which is covered by the test matrix in Phase 2/3 below.
- **`connect()` in-flight race — fixed for this change's new caller, not globally**: `connect()` (`useTerminalStream.ts:154`) only guards against "already connected" (`if (isConnectedRef.current || !sessionId) return;`), not "a connect already in flight" (`isConnected` only flips `true` on the first message received inside the async stream loop, `useTerminalStream.ts:212-213`). Adversarial review escalated this to a Blocker because this plan's disconnected branch was the first new caller that made the race concretely reachable (e.g. a background tab auto-connects, user refocuses within ~1s, before the first message arrives). **Resolution**: `useVisibilityResync`'s disconnected branch is gated on `terminalState !== 'CONNECTING' && terminalState !== 'LOADING'` in addition to `!isConnected` (Task 2.1.2b), verified by a real single-call regression test (Task 2.1.2c, replacing the original draft's non-asserting Task 2.1.3c). **Residual, still-accepted limitation**: `connect()` itself still has no in-flight guard for callers *other than* this new hook (e.g. a theoretical race between the pre-existing V2 listener and the exponential-backoff auto-reconnect path calling `connect()` in the same tick while genuinely `DISCONNECTED`, not `CONNECTING`). That gap pre-dates this change, is not made worse by it (this hook's own new caller is now gated), and fixing `connect()` globally is out of scope per requirements' minimal-diff constraint.
- **Rollback procedure**: standard revert via PR close + revert commit — the change touches 4 non-test frontend source files (`TerminalOutput.tsx`, `useTerminalStream.ts`, `useDebounce.ts`, and the new `useVisibilityResync.ts`) plus one comment-only line in `server/services/connectrpc_websocket.go` (Task 3.1.1d), no migrations, no behavioral infra impact.
- **Staged rollout**: full rollout on merge (no cohort/percentage rollout mechanism exists in this frontend).

---

## Unresolved Questions

None. Every ambiguity flagged in research (resync-completion heuristic, pool `isVisible` non-gating, watchdog-window UI treatment, `connect()` in-flight guard) has been resolved above (Pattern Decisions table and Risk Control) using the research findings, and is encoded into the stories/tasks below rather than left open. Aria-live scope is no longer ambiguous: Phase 5 has been dropped from this plan entirely (see "Revision Notes" and the removed Phase 5 section) and tracked as a separate follow-up, per adversarial review's scope-drift Concern.

---

## Dependency Visualization

```
Phase 1: Foundation (prerequisites, no user-visible behavior change yet)
  Epic 1.1 Wire resync primitives through useTerminalStream
    Task 1.1.1a  Add 3 fields to TerminalStreamResult interface           ─┐
    Task 1.1.1b  Return the 3 fields from useTerminalStream()             ─┤
    Task 1.1.1d  Widen connect/disconnect declared types to Promise<void> ─┴─► unblocks Epic 2.1
    Task 1.1.1c  First tsc --noEmit + existing-test pass
  Epic 1.2 Fix useDebouncedCallback root cause (AC2)
    Task 1.2.1a  Replace useState timer id with useRef          ─┐
    Task 1.2.1b  Memoize return via useCallback                 ─┴─► unblocks Story 2.1.1
    Task 1.2.1c  Same-tick double-fire regression test (AC2)

Phase 2: Visibility/Focus Resync (depends on Phase 1)
  Epic 2.1 New `useVisibilityResync` hook (web-app/src/components/sessions/useVisibilityResync.ts),
           called by TerminalOutput.tsx — NOT inlined into it, per architecture-review.md and
           adversarial-review.md's independently-converged extraction recommendation.
    Story 2.1.1  Hook skeleton + connected branch: debounced requestFullResync via
                 useDebouncedCallback (AC1)                                            ─┐
    Story 2.1.2  Disconnected branch: connect() + showReconnectButton, gated on         │
                 terminalState !== CONNECTING/LOADING (AC4, resolves connect() race)    │
    Story 2.1.3  Stall watchdog: try/finally arm-unconditionally, re-entrancy no-op (AC5)├─► all
    Story 2.1.4  Focus preservation, verified via renderHook + focus spy (AC3)          │  live in
    Story 2.1.5  Resync-completion detection: hook returns notifyResyncOutputReceived(),│  the hook
                 called from TerminalOutput.tsx's handleOutput                          │
    Story 2.1.6  Session-switch cleanup: hook's own [sessionId]-keyed effect (fixes      │
                 adversarial review Blocker 1)                                          │
    Story 2.1.8  Reconnecting-banner reuse after 2s pending (fixes triad-review UX      │
                 Concern — Pattern Decisions' promise had no implementing task)          ┘
    Story 2.1.7  tsc --noEmit re-verification after TerminalOutput.tsx + hook are edited
  Epic 2.2 Protect existing triggers (AC6) — verification only, no code change
    Task 2.2.1a  Scoped git diff review of XtermTerminal.tsx + the V2 listener block

Phase 3: Full-repaint integration proof (independent of Phase 2's listener wiring;
         only depends on existing TerminalStreamManager.write(), unmodified)
  Epic 3.1 Real TerminalStreamManager + real xterm Terminal test (AC7)

Phase 4: Manual verification (depends on Phases 1-3 being merged/buildable)
  Epic 4.1 Manual repro + PR description (AC0)
```

Phase 5 (aria-live wiring) from the original draft has been dropped entirely —
see "Phase 5 — dropped" note after Phase 4 below.

---

## Phase 1: Foundation

### Epic 1.1: Wire `requestFullResync`/`markResyncComplete`/`markPaneResponseReceived` through `useTerminalStream`, and correct `connect`/`disconnect`'s declared types

**Goal**: Make the three already-implemented `useTerminalFlowControl` functions callable from `TerminalOutput.tsx`, which today only has access to `sendInput`, `resize`, `requestScrollback`, `sendFlowControl` from the hook's return value. Also fix `TerminalStreamResult.connect`/`.disconnect`'s declared types, which are wrong today (declared `() => void`, actually `async`) — a pre-existing bug that this plan's Phase 2 watchdog is the first code to actually need corrected (architecture review Blocker).

#### Story 1.1.1: Expose the resync trio from `useTerminalStream`'s return value
**As a** `TerminalOutput` component, **I want** `requestFullResync`, `markResyncComplete`, and `markPaneResponseReceived` available from `useTerminalStream()`'s return object, **so that** I can trigger and track a full pane resync from the new visibility/focus hook without any new prop plumbing.
**Acceptance Criteria**:
- `useTerminalStream()`'s returned object includes `requestFullResync`, `markResyncComplete`, `markPaneResponseReceived`, each identical in identity/behavior to the corresponding `flowControl.*` function.
  - *Given* a rendered `useTerminalStream` hook instance, *When* the consuming component destructures `requestFullResync` from the hook's return value and calls `requestFullResync(true)` while connected, *Then* the call has the exact same effect as calling `flowControl.requestFullResync(true)` directly today (a `CurrentPaneRequest` proto message is pushed onto the stream) — because the returned function *is* `flowControl.requestFullResync`, not a wrapper.
**Files**: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 1.1.1a: Extend the `TerminalStreamResult` interface (~2 min)
- In `web-app/src/lib/hooks/useTerminalStream.ts`, add three fields to the `TerminalStreamResult` interface (currently lines 54-71, immediately after `handleManualReconnect: () => void;` at line 70):
  ```ts
  requestFullResync: (urgent?: boolean) => void;
  markResyncComplete: () => void;
  markPaneResponseReceived: () => void;
  ```
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 1.1.1b: Return the three fields from the hook body (~2 min)
- In the same file's `return { ... }` statement (currently lines 457-473), add after `handleManualReconnect,` (line 472):
  ```ts
  requestFullResync: flowControl.requestFullResync,
  markResyncComplete: flowControl.markResyncComplete,
  markPaneResponseReceived: flowControl.markPaneResponseReceived,
  ```
- Do not touch the `NEXT_PUBLIC_RECONNECT_V2` listener at lines 420-446 or any other existing line in this file.
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 1.1.1d: Widen `connect`/`disconnect`'s declared types to `Promise<void>` (~3 min) — resolves architecture review Blocker
- In the `TerminalStreamResult` interface (same block as Task 1.1.1a), change:
  ```ts
  connect: () => void;
  disconnect: () => void;
  ```
  to:
  ```ts
  connect: (cols?: number, rows?: number) => Promise<void>;
  disconnect: () => Promise<void>;
  ```
  matching the actual `async function connect(...)`/`async function disconnect()` implementations already in this file (`useTerminalStream.ts:154` and neighboring `disconnect` definition) — the declared types were simply wrong; the implementations were never changed.
- This is a backward-compatible widening: `grep -n "disconnect()" web-app/src/components/sessions/TerminalOutput.tsx web-app/src/lib/hooks/useTerminalStream.ts` confirms every existing call site today treats `connect()`/`disconnect()` as fire-and-forget (no `.then()`/`await`/return-value use) — none of them break when the declared return type gains a `Promise<void>` they don't consume. Phase 2's new stall watchdog (Story 2.1.3) is the first caller in the codebase that actually chains `.then()` off `disconnect()`, and needs this widened type to type-check.
- Files: `web-app/src/lib/hooks/useTerminalStream.ts`

##### Task 1.1.1c: First type-check and existing-test regression pass (~3 min)
- Run `cd web-app && npx tsc --noEmit` to confirm the interface/return-object additions and the widened `connect`/`disconnect` types compile.
- Run `cd web-app && npx jest useTerminalStream.test.ts` to confirm no existing test in that suite (which asserts on the shape of the returned object in places) breaks from the additive/widening change.
- Note: this is the *first* of two `tsc --noEmit` passes in this plan — a second pass runs in Phase 2 (Task 2.1.7a) after `TerminalOutput.tsx` and the new `useVisibilityResync.ts` are edited, since those are the files that actually consume the widened `Promise<void>` types via `.then()` (architecture review Blocker: "add a `tsc --noEmit` verification task to Phase 2 — nothing in Phase 2 re-verifies after `TerminalOutput.tsx` is edited").
- Files: none (verification task).

---

### Epic 1.2: Fix `useDebouncedCallback` root cause (AC2)

**Goal**: Replace the `useState`-backed timer id in `useDebouncedCallback` with a `useRef`-backed one, and memoize the returned callback, so two calls landing in the same JS tick collapse into exactly one scheduled invocation — the prerequisite for AC1's same-tick dedup test to be able to pass.

#### Story 1.2.1: `useDebouncedCallback` uses a ref-backed timer id and returns a stable callback
**As a** caller of `useDebouncedCallback` (the new visibility/focus resync handler, first real consumer), **I want** two same-tick calls to collapse into one scheduled invocation and the returned function identity to be stable across renders, **so that** the resync handler doesn't double-fire and can safely be used inside dependency arrays.
**Acceptance Criteria** (AC2):
- `useDebouncedCallback`'s timer id is stored in a `useRef`, not `useState`.
  - *Given* a component calling `const debounced = useDebouncedCallback(cb, 300)`, *When* `debounced()` is invoked twice synchronously in the same tick (e.g. both a `visibilitychange` and a `focus` DOM event dispatched back-to-back before any timer fires), *Then* `cb` is invoked exactly once, ~300ms after the second call — not twice.
- The returned callback is referentially stable (memoized via `useCallback`) across re-renders when `callback` and `delay` are unchanged.
  - *Given* a component re-rendering with the same `callback` reference and `delay` value passed to `useDebouncedCallback`, *When* the component re-renders twice in a row, *Then* `Object.is(debouncedFromRenderOne, debouncedFromRenderTwo)` is `true`.
**Files**: `web-app/src/lib/hooks/useDebounce.ts`

##### Task 1.2.1a: Replace `useState<NodeJS.Timeout | null>` with `useRef` (~3 min)
- In `web-app/src/lib/hooks/useDebounce.ts`, in `useDebouncedCallback` (lines 31-56):
  - Replace `const [timeoutId, setTimeoutId] = useState<NodeJS.Timeout | null>(null);` (line 35) with `const timeoutIdRef = useRef<ReturnType<typeof setTimeout> | null>(null);` (matching the `ReturnType<typeof setTimeout>` idiom used in `useTerminalStream.ts:102-103` and `useTerminalFlowControl.ts:45`, not `NodeJS.Timeout`, for portability consistency with the rest of the codebase's newer refs).
  - Update the import line (line 1) to add `useRef` alongside the existing `useState, useEffect`: `import { useState, useEffect, useRef, useCallback } from 'react';` (also add `useCallback`, needed by Task 1.2.1b).
- Files: `web-app/src/lib/hooks/useDebounce.ts`

##### Task 1.2.1b: Rewrite the body to read/clear/set the ref, wrapped in `useCallback` (~4 min)
- Replace the body of `useDebouncedCallback` (lines 37-53) with:
  ```ts
  const debouncedCallback = useCallback(
    ((...args: Parameters<T>) => {
      if (timeoutIdRef.current) {
        clearTimeout(timeoutIdRef.current);
      }
      timeoutIdRef.current = setTimeout(() => {
        timeoutIdRef.current = null;
        callback(...args);
      }, delay);
    }) as T,
    [callback, delay]
  );

  useEffect(() => {
    return () => {
      if (timeoutIdRef.current) {
        clearTimeout(timeoutIdRef.current);
      }
    };
  }, []);
  ```
- Note the cleanup effect now has an empty dependency array (was `[timeoutId]`) since it only needs to run once on unmount and reads the ref's *current* value at cleanup time, not a stale snapshot — this is intentional and matches the ref-mirror idiom (the ref is mutable, so the empty-deps cleanup still sees the latest timer id).
- Files: `web-app/src/lib/hooks/useDebounce.ts`

##### Task 1.2.1c: Write the same-tick regression test (AC2) (~5 min)
- Create `web-app/src/lib/hooks/__tests__/useDebounce.test.ts` (new file — no existing test file for this hook today).
- Test `useDebouncedCallback_should_invokeCallbackExactlyOnce_When_calledTwiceInSameTick`:
  ```ts
  import { renderHook, act } from '@testing-library/react';
  import { useDebouncedCallback } from '../useDebounce';

  describe('useDebouncedCallback', () => {
    beforeEach(() => { jest.useFakeTimers(); });
    afterEach(() => { jest.useRealTimers(); });

    it('useDebouncedCallback_should_invokeCallbackExactlyOnce_When_calledTwiceInSameTick', () => {
      const cb = jest.fn();
      const { result } = renderHook(() => useDebouncedCallback(cb, 300));

      act(() => {
        result.current('first');
        result.current('second');
      });
      act(() => { jest.advanceTimersByTime(300); });

      expect(cb).toHaveBeenCalledTimes(1);
      expect(cb).toHaveBeenCalledWith('second');
    });

    it('useDebouncedCallback_should_returnStableIdentity_When_callbackAndDelayUnchanged', () => {
      const cb = jest.fn();
      const { result, rerender } = renderHook(() => useDebouncedCallback(cb, 300));
      const first = result.current;
      rerender();
      expect(result.current).toBe(first);
    });
  });
  ```
- Files: `web-app/src/lib/hooks/__tests__/useDebounce.test.ts` (new)

---

## Phase 2: Visibility/Focus Resync (extracted hook)

### Epic 2.1: New `useVisibilityResync` hook

**Goal**: Implement `useVisibilityResync`, a standalone hook (`web-app/src/components/sessions/useVisibilityResync.ts`) that registers a `document`'s `visibilitychange` + `window`'s `focus` listener (debounced via `useDebouncedCallback`), then branches on `isConnected`/`terminalState` — full resync when connected (AC1), gated direct-reconnect fallback when disconnected (AC4) — with a 4000ms stall watchdog (AC5), never touching focus (AC3), a `[sessionId]`-keyed cleanup effect (fixes adversarial review Blocker 1), and a returned `notifyResyncOutputReceived()` for `TerminalOutput.tsx`'s `handleOutput` to call (needed by AC5).

This entire Epic supersedes the original draft's plan to inline everything into a single `TerminalOutput.tsx` `useEffect` — both architecture-review.md and adversarial-review.md independently recommended the extraction (Step 0.5's "Post-review refinement" note). `TerminalOutput.tsx` itself is touched only to: (a) call the hook, (b) call `notifyResyncOutputReceived()` inside `handleOutput`. `useTerminalStream.ts`'s V2 listener and `XtermTerminal.tsx` remain untouched, satisfying AC6 exactly as the original inline draft did.

#### Story 2.1.1: Hook skeleton + connected branch — debounced full resync via `useDebouncedCallback` (AC1)
**As a** user returning to a backgrounded tab whose terminal stream is still connected, **I want** the pane to be resynced automatically, **so that** I never act on stale/corrupted rendered content.
**Acceptance Criteria** (AC1):
- A `visibilitychange`→`'visible'` transition and a `window focus` event, even dispatched back-to-back in the same tick, trigger exactly one `requestFullResync(true)` call per ~300ms debounce window.
  - *Given* `useVisibilityResync` is rendered via `renderHook` with `isConnected: true`, *When* `document.visibilitychange` and `window focus` both fire synchronously in the same tick (matching real-browser tab-refocus behavior), and then 300ms elapse, *Then* the mocked `requestFullResync` is called exactly once (not zero, not two).
- A `focus` event alone (no `visibilitychange`) independently triggers a call — proving the `focus` listener is actually wired, not merely riding along with a `visibilitychange`-only test (adversarial review Concern).
- Two transitions spaced more than 300ms apart produce two separate `requestFullResync` calls — proving this is a debounce window, not some other single-call guard (adversarial review Concern).
**Files**: `web-app/src/components/sessions/useVisibilityResync.ts` (new)

##### Task 2.1.1a: Create the hook file — types, constants, refs, ref-mirror effect (~5 min)
- Create `web-app/src/components/sessions/useVisibilityResync.ts`:
  ```ts
  import { useCallback, useEffect, useRef } from 'react';
  import { useDebouncedCallback } from '@/lib/hooks/useDebounce';
  import type { TerminalState } from '@/lib/hooks/useTerminalStream';

  const RESYNC_DEBOUNCE_MS = 300;
  const RESYNC_STALL_TIMEOUT_MS = 4000;
  // Delay before surfacing the existing reconnecting-banner UI for a pending
  // resync (Story 2.1.8) — shorter than RESYNC_STALL_TIMEOUT_MS so a slow
  // resync gets a visible affordance well before the 4s forced-reconnect path.
  const RESYNC_BANNER_DELAY_MS = 2000;

  export interface UseVisibilityResyncParams {
    sessionId: string;
    isConnected: boolean;
    terminalState: TerminalState;
    connect: (cols?: number, rows?: number) => Promise<void>;
    disconnect: () => Promise<void>;
    requestFullResync: (urgent?: boolean) => void;
    markResyncComplete: () => void;
    markPaneResponseReceived: () => void;
    setShowReconnectButton: (value: boolean) => void;
    /** Reuses the existing 2s reconnecting-banner UI (Pattern Decisions:
     * "Transient-state UI during the 0-4s resync/watchdog window") once a
     * connected-branch resync has been pending ≥2s. Optional so tests that
     * don't care about this affordance can omit it. */
    setShowReconnectBanner?: (value: boolean) => void;
  }

  export interface UseVisibilityResyncResult {
    notifyResyncOutputReceived: () => void;
  }

  export function useVisibilityResync(params: UseVisibilityResyncParams): UseVisibilityResyncResult {
    const {
      sessionId, isConnected, terminalState, connect, disconnect,
      requestFullResync, markResyncComplete, markPaneResponseReceived, setShowReconnectButton,
      setShowReconnectBanner,
    } = params;

    const resyncStallTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    // Distinct ref/timer from resyncStallTimerRef and useDebouncedCallback's own
    // internal timer (pitfalls.md guardrail: never share a ref between timers).
    // Fires 2s into a pending resync to surface the existing reconnecting-banner
    // UI (Story 2.1.8) — separate from the 4s stall watchdog above.
    const resyncBannerTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    // No-correlation-ID heuristic: cleared by the *next* output of any kind (see
    // notifyResyncOutputReceived below), not specifically the CurrentPaneRequest
    // response — same imprecise heuristic the pre-existing resize->resync path
    // already relies on (isResyncingRef/waitingForPaneResponseRef). Accepted
    // trade-off, locked in by Task 2.1.3d's test; worst case bounded by the
    // RESYNC_STALL_TIMEOUT_MS watchdog below.
    const pendingResyncCompletionRef = useRef(false);

    const isConnectedRef = useRef(isConnected);
    const terminalStateRef = useRef(terminalState);
    const connectRef = useRef(connect);
    const disconnectRef = useRef(disconnect);
    const requestFullResyncRef = useRef(requestFullResync);
    const markResyncCompleteRef = useRef(markResyncComplete);
    const markPaneResponseReceivedRef = useRef(markPaneResponseReceived);
    const setShowReconnectButtonRef = useRef(setShowReconnectButton);
    const setShowReconnectBannerRef = useRef(setShowReconnectBanner);
    // Mirrors the *latest* sessionId on every render, independent of which
    // sessionId a given `handleVisibilityOrFocusResyncInner` closure was created
    // for. Lets a stale, already-scheduled debounced/watchdog callback detect
    // mid-flight that the session has since switched and abort instead of
    // firing against the new session's connect()/disconnect() (2nd-review
    // architecture/adversarial finding: session-switch cleanup, Story 2.1.6,
    // only tears down state for a resync/watchdog that had already started —
    // it doesn't cancel a debounce timer that's still pending when the switch
    // happens, since useDebouncedCallback exposes no cancel handle).
    const sessionIdRef = useRef(sessionId);

    useEffect(() => {
      isConnectedRef.current = isConnected;
      terminalStateRef.current = terminalState;
      connectRef.current = connect;
      disconnectRef.current = disconnect;
      requestFullResyncRef.current = requestFullResync;
      markResyncCompleteRef.current = markResyncComplete;
      markPaneResponseReceivedRef.current = markPaneResponseReceived;
      setShowReconnectButtonRef.current = setShowReconnectButton;
      setShowReconnectBannerRef.current = setShowReconnectBanner;
      sessionIdRef.current = sessionId;
    });

    const clearStallTimer = useCallback(() => {
      if (resyncStallTimerRef.current) {
        clearTimeout(resyncStallTimerRef.current);
        resyncStallTimerRef.current = null;
      }
    }, []);

    const clearBannerTimer = useCallback(() => {
      if (resyncBannerTimerRef.current) {
        clearTimeout(resyncBannerTimerRef.current);
        resyncBannerTimerRef.current = null;
      }
    }, []);

    // ... handleVisibilityOrFocusResyncInner, useDebouncedCallback wiring, watchdog,
    // session-switch cleanup, banner timer (Story 2.1.8), and
    // notifyResyncOutputReceived are added in Tasks 2.1.1b, 2.1.3a, 2.1.6a,
    // 2.1.8a, 2.1.5a respectively.
  }
  ```
  (No dependency array on the ref-mirror effect — runs every render, mirroring every value; cheap since it's just ref assignment, same shape as `connectRef.current = connect;` at `useTerminalStream.ts:352` which also runs unconditionally every render.)
- Files: `web-app/src/components/sessions/useVisibilityResync.ts` (new)

##### Task 2.1.1b: Implement the connected branch and wire it through `useDebouncedCallback` (~6 min)
- Inside `useVisibilityResync`, add:
  ```ts
  const handleVisibilityOrFocusResyncInner = useCallback(() => {
    if (document.visibilityState !== 'visible') return;
    // Abort if the session changed since this debounced call was scheduled —
    // e.g. visibilitychange fires while viewing session A, the debounce timer
    // is armed, then the user switches to session B before the 300ms elapses.
    // `sessionId` here is this closure's frozen value (a real useCallback dep);
    // `sessionIdRef.current` is the latest live value. A mismatch means this is
    // a stale callback that must not act on the new session's connect/disconnect
    // (2nd-review finding: Story 2.1.6's cleanup only handles a resync/watchdog
    // that already started, not one still pending in the debounce window).
    if (sessionId !== sessionIdRef.current) return;

    if (isConnectedRef.current) {
      // Rapid-flap re-entrancy guard — see Story 2.1.3.
      if (pendingResyncCompletionRef.current) return;

      pendingResyncCompletionRef.current = true;
      try {
        console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus delay=0ms`);
        requestFullResyncRef.current(true);
      } catch (err) {
        console.warn(`[resync] sessionId=${sessionId} requestFullResync threw synchronously`, err);
      } finally {
        // Arm unconditionally — even on a synchronous throw — so the watchdog
        // remains the single recovery path. See Story 2.1.3.
        clearStallTimer();
        resyncStallTimerRef.current = setTimeout(() => {
          resyncStallTimerRef.current = null;
          if (pendingResyncCompletionRef.current) {
            pendingResyncCompletionRef.current = false;
            markResyncCompleteRef.current();
            markPaneResponseReceivedRef.current();
            console.warn(`[resync] sessionId=${sessionId} stall watchdog fired after ${RESYNC_STALL_TIMEOUT_MS}ms, forcing disconnect+reconnect`);
            clearBannerTimer();
            disconnectRef.current().then(() => connectRef.current());
          }
        }, RESYNC_STALL_TIMEOUT_MS);
        // Surface the existing reconnecting-banner UI once a resync has been
        // pending ≥2s — see Story 2.1.8. Independent of, and shorter than, the
        // 4s stall watchdog above; reuses `TerminalOutput.tsx`'s already-shipped
        // banner rather than adding a new resync-specific indicator (Pattern
        // Decisions: "Transient-state UI during the 0-4s resync/watchdog window").
        clearBannerTimer();
        resyncBannerTimerRef.current = setTimeout(() => {
          resyncBannerTimerRef.current = null;
          if (pendingResyncCompletionRef.current) {
            setShowReconnectBannerRef.current?.(true);
          }
        }, RESYNC_BANNER_DELAY_MS);
      }
    } else {
      // Don't take the disconnected fallback mid-handshake. See Story 2.1.2.
      if (terminalStateRef.current === 'CONNECTING' || terminalStateRef.current === 'LOADING') return;
      console.info(`[resync] sessionId=${sessionId} trigger=visibility-or-focus fallback=connect`);
      connectRef.current();
      setShowReconnectButtonRef.current(true);
    }
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  // Epic 1.2's useDebouncedCallback (ref-backed timer, memoized return) IS the
  // debounce mechanism here — not a hand-rolled setTimeout/ref pair — making
  // Epic 1.2's "first real consumer" framing true of the code (resolves
  // architecture review Concern "Epic 1.2 vs Task 2.1.1d mismatch").
  const debouncedResync = useDebouncedCallback(handleVisibilityOrFocusResyncInner, RESYNC_DEBOUNCE_MS);

  useEffect(() => {
    document.addEventListener('visibilitychange', debouncedResync);
    window.addEventListener('focus', debouncedResync);
    return () => {
      document.removeEventListener('visibilitychange', debouncedResync);
      window.removeEventListener('focus', debouncedResync);
    };
  }, [debouncedResync]);

  useEffect(() => {
    return () => {
      clearStallTimer();
      clearBannerTimer();
    };
  }, [clearStallTimer, clearBannerTimer]);
  ```
- `handleVisibilityOrFocusResyncInner` is deliberately stable across renders except when `sessionId`/`clearStallTimer`/`clearBannerTimer` change (`clearStallTimer`/`clearBannerTimer` are themselves empty-deps `useCallback`s, so in practice only a `sessionId` change recreates it) so `useDebouncedCallback`'s own memoization doesn't churn on every render.
- Files: `web-app/src/components/sessions/useVisibilityResync.ts`

##### Task 2.1.1c: Wire the hook into `TerminalOutput.tsx` (~4 min)
- In `web-app/src/components/sessions/TerminalOutput.tsx`, after the existing `useTerminalStream(...)` destructure (line 432), import and call:
  ```ts
  import { useVisibilityResync } from './useVisibilityResync';
  // ...
  const { notifyResyncOutputReceived } = useVisibilityResync({
    sessionId: effectiveSessionId,
    isConnected,
    terminalState,
    connect,
    disconnect,
    requestFullResync,
    markResyncComplete,
    markPaneResponseReceived,
    setShowReconnectButton,
    setShowReconnectBanner,
  });
  ```
  (Also update the `useTerminalStream(...)` destructure to add `requestFullResync, markResyncComplete, markPaneResponseReceived,` alongside the existing `handleManualReconnect: handleHookReconnect` — same shape as the original draft's Task 2.1.1b, just feeding the new hook instead of local refs. `setShowReconnectBanner` already exists in `TerminalOutput.tsx` — Story 2.1.8 wires it in.)
- `notifyResyncOutputReceived` is consumed in Story 2.1.5's Task 2.1.5b (`handleOutput`).
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 2.1.1d: Regression test — same-tick dedup, connected branch (AC1) (~5 min)
- Create `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts` using `renderHook`/`act` from `@testing-library/react`.
- Test `useVisibilityResync_should_callRequestFullResyncExactlyOnce_When_visibilityAndFocusFireInSameTick`:
  ```ts
  import { renderHook, act } from '@testing-library/react';
  import { useVisibilityResync } from '../useVisibilityResync';

  function makeParams(overrides: Partial<Parameters<typeof useVisibilityResync>[0]> = {}) {
    return {
      sessionId: 's1',
      isConnected: true,
      terminalState: 'STABLE',
      connect: jest.fn().mockResolvedValue(undefined),
      disconnect: jest.fn().mockResolvedValue(undefined),
      requestFullResync: jest.fn(),
      markResyncComplete: jest.fn(),
      markPaneResponseReceived: jest.fn(),
      setShowReconnectButton: jest.fn(),
      setShowReconnectBanner: jest.fn(),
      ...overrides,
    };
  }

  describe('useVisibilityResync', () => {
    beforeEach(() => { jest.useFakeTimers(); });
    afterEach(() => { jest.useRealTimers(); });

    it('useVisibilityResync_should_callRequestFullResyncExactlyOnce_When_visibilityAndFocusFireInSameTick', () => {
      const params = makeParams();
      renderHook(() => useVisibilityResync(params));

      act(() => {
        Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
        document.dispatchEvent(new Event('visibilitychange'));
        window.dispatchEvent(new Event('focus'));
      });
      act(() => { jest.advanceTimersByTime(300); });

      expect(params.requestFullResync).toHaveBeenCalledTimes(1);
      expect(params.requestFullResync).toHaveBeenCalledWith(true);
    });
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts` (new)

##### Task 2.1.1e: Regression test — focus alone triggers a call (AC1, adversarial review Concern) (~3 min)
- In the same file, add `useVisibilityResync_should_callRequestFullResyncOnce_When_onlyFocusEventFires`: same setup, but dispatch only `window.dispatchEvent(new Event('focus'))` (no `visibilitychange`), advance 300ms, assert `requestFullResync` called once. Proves the `window.addEventListener('focus', ...)` registration actually works, independent of the `visibilitychange` listener.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.1f: Regression test — transitions >300ms apart produce two calls (AC1, adversarial review Concern) (~3 min)
- In the same file, add `useVisibilityResync_should_callRequestFullResyncTwice_When_transitionsAreMoreThan300msApart`: dispatch `visibilitychange`, advance 400ms, dispatch `focus`, advance 400ms; assert `requestFullResync` called exactly twice. Proves this is a debounce window, not a permanent single-call latch.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.2: Disconnected branch — direct reconnect fallback, gated against the in-flight `connect()` race (AC4, resolves adversarial review Blocker 2 / Concern)
**As a** user returning to a backgrounded tab whose stream silently/gracefully disconnected (`error === null`) while hidden, **I want** reconnection to start immediately on refocus rather than waiting for the existing 5s reconnect-button timer, **so that** I'm not left staring at a dead terminal longer than necessary — **without** opening a second stream if a connect is already in flight.
**Acceptance Criteria** (AC4):
- A qualifying visibility/focus transition while `isConnected === false` **and** `terminalState` is not `CONNECTING`/`LOADING` calls `connect()` directly and sets `showReconnectButton(true)`.
  - *Given* `useVisibilityResync` with `isConnected: false, terminalState: 'DISCONNECTED'`, *When* a `visibilitychange`→`'visible'` transition fires and 300ms elapse, *Then* the mocked `connect` is called exactly once and `setShowReconnectButton(true)` is called.
- A qualifying transition while `terminalState === 'CONNECTING'` (mid-handshake, `isConnected` still `false`) does **not** call `connect()` a second time — this is the fix for the concretely-reachable duplicate-stream race adversarial review escalated to a Blocker: a background tab's `autoConnect` fires `connect()`, the user refocuses within ~1s before the first message arrives, and the old disconnected-branch fallback would call `connect()` again.
  - *Given* `useVisibilityResync` with `isConnected: false, terminalState: 'CONNECTING'`, *When* a qualifying transition fires and 300ms elapse, *Then* `connect` is called **zero** additional times (asserting the gate actually suppresses the call, not just "no error thrown" — replaces the original draft's toothless Task 2.1.3c).
**Files**: `web-app/src/components/sessions/useVisibilityResync.ts` (implemented as part of Task 2.1.1b's `else` branch)

##### Task 2.1.2a: (implementation) — see Task 2.1.1b's `else` branch above; no separate task, the gate is `terminalStateRef.current === 'CONNECTING' || terminalStateRef.current === 'LOADING'`.

##### Task 2.1.2b: Regression test — disconnected fallback calls connect + shows reconnect button (AC4) (~4 min)
- In `useVisibilityResync.test.ts`, add `useVisibilityResync_should_callConnectAndShowReconnectButton_When_visibilityFiresWhileDisconnected`:
  - `makeParams({ isConnected: false, terminalState: 'DISCONNECTED' })`.
  - Dispatch `visibilitychange` with `visibilityState: 'visible'`, advance 300ms.
  - Assert `connect` called once; assert `setShowReconnectButton` called once with `true`.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.2c: Regression test — `CONNECTING` gate suppresses the duplicate `connect()` call (AC4, replaces original Task 2.1.3c) (~5 min)
- In `useVisibilityResync.test.ts`, add `useVisibilityResync_should_notCallConnect_When_visibilityFiresWhileTerminalStateIsConnecting`:
  - `makeParams({ isConnected: false, terminalState: 'CONNECTING' })`.
  - Dispatch `visibilitychange` with `visibilityState: 'visible'`, advance 300ms.
  - Assert `connect` was called **zero** times, and `setShowReconnectButton` was **not** called with `true`.
  - Add a second variant with `terminalState: 'LOADING'` covering the other mid-handshake state named in Risk Control.
- This test actually asserts the gate suppresses the duplicate call (real regression coverage), unlike the original draft's Task 2.1.3c, which was explicitly designed to be unable to fail.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.3: Stall watchdog — try/finally arm-unconditionally, re-entrancy no-op (AC5, resolves adversarial review Concerns "try/catch + arm order" and "rapid flap re-arms watchdog")
**As a** user whose resync RPC round-trip never completes (server slow, connection degraded), **I want** the client to notice and force a real reconnect rather than waiting forever, **so that** the terminal doesn't get stuck silently mid-resync — and rapid tab-flapping must not mask a genuine stall or duplicate in-flight resync requests.
**Acceptance Criteria** (AC5):
- If a resync is requested while connected but no output arrives within `RESYNC_STALL_TIMEOUT_MS` (4000ms), the watchdog force-clears `pendingResyncCompletionRef` (and calls `markResyncComplete()`/`markPaneResponseReceived()`) and runs `disconnect().then(() => connect())`.
- If output arrives before 4000ms (via `notifyResyncOutputReceived()`, Story 2.1.5), the watchdog must NOT fire `disconnect()`/`connect()`.
- The watchdog timer is armed in a `finally` block unconditionally — even if `requestFullResync(true)` throws synchronously — so a throw can't leave `pendingResyncCompletionRef` stuck `true` forever with nothing to ever clear it (adversarial review Concern: the original draft armed the timer only after a successful call returned).
- A qualifying transition while a resync is already pending (`pendingResyncCompletionRef.current === true`) is a no-op: it does not call `requestFullResync` again and does not reset/re-arm the watchdog timer. Without this, rapid hidden→visible→hidden→visible cycling (OS-level app switching, multi-monitor focus changes) would keep resetting the watchdog and firing duplicate `CurrentPaneRequest`s, masking a genuine stall indefinitely (adversarial review Concern).
**Files**: `web-app/src/components/sessions/useVisibilityResync.ts` (implemented in Task 2.1.1b), test file below.

##### Task 2.1.3a: Regression test — stall fires (AC5, stall case) (~5 min)
- In `useVisibilityResync.test.ts`, add `useVisibilityResync_should_forceDisconnectThenConnect_When_resyncStallsPastWatchdogTimeout`:
  - `makeParams()` (connected), dispatch `visibilitychange`, advance 300ms (resync fires) — do NOT call `notifyResyncOutputReceived`.
  - `await act(async () => { jest.advanceTimersByTime(4000); })`.
  - Assert `disconnect` called once; after flushing the resolved promise, assert `connect` called once.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.3b: Regression test — completes in time, watchdog does not fire (AC5, success case) (~5 min)
- Add `useVisibilityResync_should_notForceDisconnect_When_resyncCompletesBeforeWatchdogTimeout`:
  - `makeParams()`, dispatch `visibilitychange`, advance 300ms (resync fires).
  - Capture `const { result } = renderHook(...)`, then call `result.current.notifyResyncOutputReceived()` at simulated t=1000ms.
  - Advance remaining time past the original 4000ms mark.
  - Assert `disconnect` was never called.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.3c: Regression test — synchronous throw from `requestFullResync` still arms the watchdog (~4 min)
- Add `useVisibilityResync_should_stillArmWatchdog_When_requestFullResyncThrowsSynchronously`:
  - `makeParams({ requestFullResync: jest.fn(() => { throw new Error('boom'); }) })`.
  - Dispatch `visibilitychange`, advance 300ms — assert no uncaught error propagates out of the debounced handler.
  - `await act(async () => { jest.advanceTimersByTime(4000); })` — assert `disconnect` is called (the watchdog still fired despite the throw), proving `pendingResyncCompletionRef` wasn't left permanently stuck by the try/finally ordering.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.3d: Regression test — unrelated output mid-window prevents the watchdog from firing (locks in the accepted no-correlation-ID heuristic) (~4 min)
- Add `useVisibilityResync_should_notFireWatchdog_When_unrelatedOutputArrivesMidWindow`:
  - `makeParams()`, dispatch `visibilitychange`, advance 300ms (resync fires, `pendingResyncCompletionRef` pending).
  - Call `result.current.notifyResyncOutputReceived()` to simulate *unrelated* live output arriving mid-window (not necessarily the resync's own response — per the documented heuristic, any output clears the flag).
  - Advance past 4000ms; assert `disconnect` was never called.
  - Comment in the test explicitly noting this locks in the Pattern Decisions table's accepted trade-off as executable behavior (adversarial review Concern: "no test for its documented failure mode").
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.3e: Regression test — rapid flap does not re-arm the watchdog or duplicate the resync call (~4 min)
- Add `useVisibilityResync_should_notReissueResyncOrRearmWatchdog_When_pendingResyncAlreadyOutstanding`:
  - `makeParams()`, dispatch `visibilitychange`, advance 300ms (first resync fires, `requestFullResync` called once).
  - Dispatch `visibilitychange` again (simulating rapid hidden→visible flap), advance another 300ms.
  - Assert `requestFullResync` was still only called **once** total (the second transition no-op'd because a resync was already pending).
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.4: Focus preservation (AC3)
**As a** user typing in another part of the page (e.g. a search box) when a background resync/reconnect fires, **I want** my keyboard focus to remain exactly where it was, **so that** the resync never silently steals my next keystrokes.
**Acceptance Criteria** (AC3):
- The hook never calls `.focus()` anywhere; an automated test focuses a sibling element in the test DOM and asserts `document.activeElement` is unchanged across a full resync + watchdog cycle.
  - *Given* a test DOM containing a focused sibling `<input>` (unrelated to anything the hook touches — the hook renders no DOM itself), *When* `useVisibilityResync` is rendered and a `visibilitychange`→`'visible'` transition fires, the 300ms debounce elapses, and the 4000ms stall watchdog also fires and completes its `disconnect()`/`connect()` cycle, *Then* `document.activeElement` is still the sibling input at every checkpoint.
**Files**: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.4a: Grep-verify no `.focus()` call exists in the hook (~2 min)
- Run `git diff main -- web-app/src/components/sessions/useVisibilityResync.ts | grep -n '\.focus('` and confirm zero matches in added (`+`) lines. Mechanical verification step, not a code change.
- Files: none (verification task).

##### Task 2.1.4b: Write the focus-preservation regression test (~5 min)
```ts
it('useVisibilityResync_should_notStealFocus_When_resyncAndWatchdogFire', async () => {
  const sibling = document.createElement('input');
  document.body.appendChild(sibling);
  sibling.focus();
  expect(document.activeElement).toBe(sibling);

  const params = makeParams();
  renderHook(() => useVisibilityResync(params));

  act(() => {
    Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
    document.dispatchEvent(new Event('visibilitychange'));
    jest.advanceTimersByTime(300);
  });
  expect(document.activeElement).toBe(sibling);

  await act(async () => { jest.advanceTimersByTime(4000); });
  expect(document.activeElement).toBe(sibling);

  document.body.removeChild(sibling);
});
```
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.5: Resync-completion detection — `notifyResyncOutputReceived()` returned by the hook, called from `TerminalOutput.tsx`'s `handleOutput`
**As a** stall watchdog, **I want** to know when the pending resync's response arrives, **so that** I don't force an unnecessary disconnect/reconnect for a resync that actually succeeded.
**Acceptance Criteria** (supports AC5; not independently numbered but required for AC5's "completes in time" branch to be observable):
- Calling `notifyResyncOutputReceived()` clears `pendingResyncCompletionRef`, calls `markResyncComplete()`/`markPaneResponseReceived()`, and clears the stall timer, the first time it's called after a resync was requested; calling it when no resync is pending is a no-op.
**Files**: `web-app/src/components/sessions/useVisibilityResync.ts`, `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 2.1.5a: Implement `notifyResyncOutputReceived` in the hook (~3 min)
- Inside `useVisibilityResync`, add (returned from the hook):
  ```ts
  const notifyResyncOutputReceived = useCallback(() => {
    if (pendingResyncCompletionRef.current) {
      pendingResyncCompletionRef.current = false;
      clearStallTimer();
      clearBannerTimer();
      // The success path never flips isConnected false, so the pre-existing
      // isConnected-driven banner effect (TerminalOutput.tsx:736-761) won't
      // hide a banner shown by the 2s timer above — hide it explicitly here.
      setShowReconnectBannerRef.current?.(false);
      markResyncCompleteRef.current();
      markPaneResponseReceivedRef.current();
      console.log(`[resync] sessionId=${sessionId} pane response received, resync complete`);
    }
  }, [sessionId, clearStallTimer, clearBannerTimer]);

  return { notifyResyncOutputReceived };
  ```
- Files: `web-app/src/components/sessions/useVisibilityResync.ts`

##### Task 2.1.5b: Call `notifyResyncOutputReceived()` from `TerminalOutput.tsx`'s `handleOutput` (~3 min)
- In `web-app/src/components/sessions/TerminalOutput.tsx`, `handleOutput` (currently lines 409-421), immediately after `manager.write(output);` (line 419), add:
  ```ts
  notifyResyncOutputReceived();
  ```
- Note: `handleOutput` is a `useCallback` with deps `[getOrCreateStreamManager]` (line 421) — `notifyResyncOutputReceived` is a stable (`useCallback`-memoized inside the hook) reference for the lifetime of a given `sessionId`, so add it to `handleOutput`'s dependency array (`[getOrCreateStreamManager, notifyResyncOutputReceived]`) for correctness even though in practice its identity rarely changes.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 2.1.5c: Cross-check against the RESIZING-state output queue (~3 min)
- Verify (read-only check, per pitfalls.md race #3) that when `terminalStateRef.current === 'RESIZING'`, `handleOutput` returns early (line 412-415, `pendingOutputDuringResizeRef.current.push(output); return;`) *before* reaching the `notifyResyncOutputReceived()` call added in Task 2.1.5b — confirm the call is placed after `manager.write(output)` (i.e., after the RESIZING early-return), not before it. If output arrives while RESIZING, the completion check intentionally does not fire yet; it fires when the queued output is flushed via the `RESIZING→STABLE` transition effect (lines 450-464) **only if** that flush path also calls `handleOutput` — confirm it does not (it calls `manager.write(chunk)` directly per line 459, bypassing `notifyResyncOutputReceived`). Document this as a known, acceptable edge case: a resync requested during a resize-triggered `RESIZING` window may leave `pendingResyncCompletionRef` uncleared until the *next* `handleOutput` call after `STABLE` resumes — bounded in the worst case by the 4000ms watchdog, which is exactly what the watchdog exists for.
- Files: none (verification/documentation task).

##### Task 2.1.5d: Regression test — `notifyResyncOutputReceived` clears pending state (~4 min)
- In `useVisibilityResync.test.ts`, add `useVisibilityResync_should_clearPendingStateAndCallMarkFunctions_When_notifyResyncOutputReceivedCalledWhilePending`:
  - `makeParams()`, dispatch `visibilitychange`, advance 300ms (resync pending).
  - Call `result.current.notifyResyncOutputReceived()`.
  - Assert `markResyncComplete` and `markPaneResponseReceived` each called once.
  - Advance past 4000ms; assert `disconnect` never called (stall timer was cleared).
- Add `useVisibilityResync_should_noOp_When_notifyResyncOutputReceivedCalledWithNoPendingResync`: call `notifyResyncOutputReceived()` with no prior resync fired; assert `markResyncComplete`/`markPaneResponseReceived` are not called.
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.6: Session-switch cleanup — `[sessionId]`-keyed effect (resolves adversarial review Blocker 1)
**As a** user switching between sessions while a resync/watchdog is pending for the previous one, **I want** the stale watchdog to never fire against the newly-active session's `connect`/`disconnect`, **so that** switching sessions never triggers an unwanted forced reconnect of an unrelated session.
**Acceptance Criteria**:
- When `sessionId` changes, the hook clears `resyncStallTimerRef`, resets `pendingResyncCompletionRef` to `false`, and defensively calls `markResyncComplete()`/`markPaneResponseReceived()` — all for the *previous* session's closure, before any of the new session's values are mirrored into refs.
  - *Given* a resync is pending (`pendingResyncCompletionRef.current === true`, stall timer armed) for `sessionId: 'a'`, *When* the hook is re-rendered with `sessionId: 'b'` (matching how `TerminalOutput.tsx` reuses one long-lived component instance across session switches, never remounting), *Then* the stall timer that would have fired against `'a'` never fires `disconnect()`/`connect()` for session `'b'` — asserted by advancing timers 4000ms past the original watchdog deadline after the session switch and confirming `disconnect`/`connect` were never called.
**Files**: `web-app/src/components/sessions/useVisibilityResync.ts`

##### Task 2.1.6a: Implement the `[sessionId]`-keyed cleanup effect (~4 min)
- Inside `useVisibilityResync`, add (near the other effects):
  ```ts
  // sessionId-keyed cleanup: a watchdog/resync armed for the previous session
  // must never fire against the next one's connect()/disconnect() (adversarial
  // review Blocker 1 / research features.md race #4).
  useEffect(() => {
    return () => {
      clearStallTimer();
      clearBannerTimer();
      pendingResyncCompletionRef.current = false;
      setShowReconnectBannerRef.current?.(false);
      markResyncCompleteRef.current();
      markPaneResponseReceivedRef.current();
    };
  }, [sessionId, clearStallTimer, clearBannerTimer]);
  ```
  This effect's cleanup function runs when `sessionId` changes (before the effect re-runs for the new `sessionId`) *and* on unmount — both are the correct times to tear down session-scoped pending state. The `setShowReconnectBannerRef.current?.(false)` call is defensive here: it only matters if a banner was actually shown by *this* session's pending resync before the switch; if the new session's own `isConnected`-driven banner effect wants it shown for a different reason, it will re-set it independently.
- Files: `web-app/src/components/sessions/useVisibilityResync.ts`

##### Task 2.1.6b: Regression test — session switch clears a pending watchdog (~5 min)
- In `useVisibilityResync.test.ts`, add `useVisibilityResync_should_clearPendingWatchdog_When_sessionIdChangesWhileResyncPending`:
  ```ts
  it('useVisibilityResync_should_clearPendingWatchdog_When_sessionIdChangesWhileResyncPending', async () => {
    const paramsA = makeParams({ sessionId: 'a' });
    const { rerender } = renderHook((p) => useVisibilityResync(p), { initialProps: paramsA });

    act(() => {
      document.dispatchEvent(new Event('visibilitychange'));
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      jest.advanceTimersByTime(300);
    });
    expect(paramsA.requestFullResync).toHaveBeenCalledTimes(1);

    const paramsB = makeParams({ sessionId: 'b' });
    rerender(paramsB);

    await act(async () => { jest.advanceTimersByTime(4000); });

    expect(paramsA.disconnect).not.toHaveBeenCalled();
    expect(paramsA.connect).not.toHaveBeenCalled();
    expect(paramsB.disconnect).not.toHaveBeenCalled();
    expect(paramsB.connect).not.toHaveBeenCalled();
    expect(paramsA.markResyncComplete).toHaveBeenCalled();
    expect(paramsA.markPaneResponseReceived).toHaveBeenCalled();
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

##### Task 2.1.6c: Regression test — mid-debounce session switch never fires against the new session (~5 min)
- In `useVisibilityResync.test.ts`, add `useVisibilityResync_should_notFireResync_When_sessionIdChangesWhileDebouncePending`:
  ```ts
  it('useVisibilityResync_should_notFireResync_When_sessionIdChangesWhileDebouncePending', () => {
    const paramsA = makeParams({ sessionId: 'a' });
    const { rerender } = renderHook((p) => useVisibilityResync(p), { initialProps: paramsA });

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    // Switch sessions BEFORE the 300ms debounce elapses — the debounce timer
    // armed for 'a' is still pending when this happens.
    const paramsB = makeParams({ sessionId: 'b' });
    rerender(paramsB);

    act(() => { jest.advanceTimersByTime(300); });

    // The stale debounced call (armed while viewing 'a') must not act on
    // either session once it fires against a mismatched sessionIdRef.
    expect(paramsA.requestFullResync).not.toHaveBeenCalled();
    expect(paramsB.requestFullResync).not.toHaveBeenCalled();
  });
  ```
  This is distinct from Task 2.1.6b: 2.1.6b covers a resync that already *started* before the switch (watchdog armed, cleaned up by Story 2.1.6's effect); this covers a resync that hadn't fired *yet* — still inside the debounce window — when the switch happens, which only the `sessionId !== sessionIdRef.current` guard in Task 2.1.1b catches (Story 2.1.6's effect cleanup doesn't run until a resync/watchdog has actually started).
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.8: Surface the existing reconnecting-banner UI after 2s of a pending resync (fixes triad-review UX Concern — Pattern Decisions' banner-reuse promise had no implementing task)

**As a** user whose backgrounded-tab resync takes longer than a couple seconds to resolve, **I want** a visible sign that the app is working on it before the 4s watchdog forces a real reconnect, **so that** the terminal doesn't appear frozen during a slow resync — closing the gap between what the Pattern Decisions table already promised ("Reuse the existing `showReconnectBanner`/`reconnectingBanner` div... once `pendingResyncCompletionRef` has been true for ≥2s") and what the original task list actually implemented (nothing — the resync path was silent for the full 0-4s window).

**Acceptance Criteria** (supports UX research's silent-0-2s/banner-2-4s design; not one of requirements.md's numbered ACs, since it's UX polish on top of AC5's watchdog, not new functional scope):
- If a connected-branch resync is still pending 2000ms after being requested, `setShowReconnectBanner(true)` is called exactly once (via the existing `reconnectingBanner` div already rendered in `TerminalOutput.tsx:1600-1604` — no new UI element).
  - *Given* `useVisibilityResync` is rendered with `isConnected: true` and a `setShowReconnectBanner` spy, *When* a resync is triggered and 2000ms elapse with no `notifyResyncOutputReceived()` call, *Then* `setShowReconnectBanner` is called with `true` exactly once.
- If the resync completes (via `notifyResyncOutputReceived()`) before 2000ms elapses, the banner timer never fires and `setShowReconnectBanner` is never called.
  - *Given* the same setup, *When* `notifyResyncOutputReceived()` is called at t=500ms, *Then* advancing time past 2000ms does not call `setShowReconnectBanner`.
- If the resync completes *after* the banner was shown (between 2s and 4s), `setShowReconnectBanner(false)` is called — since the success path never flips `isConnected` false, the pre-existing `isConnected`-driven banner-hide effect would otherwise leave a stale "Reconnecting terminal…" banner visible after a resync that actually succeeded.
  - *Given* the banner was shown at t=2000ms, *When* `notifyResyncOutputReceived()` is called at t=3000ms, *Then* `setShowReconnectBanner` is called with `false`.
- The banner timer is a distinct ref/timer from both the debounce timer (`useDebouncedCallback`'s internal ref) and the stall watchdog (`resyncStallTimerRef`) — per pitfalls.md's "never share a ref between timers" guardrail — and is cleared by the session-switch cleanup (Story 2.1.6) and by the stall watchdog firing (so a forced disconnect doesn't leave a dangling banner-timer alongside it).
**Files**: `web-app/src/components/sessions/useVisibilityResync.ts` (implementation landed in Tasks 2.1.1a/b, 2.1.5a, 2.1.6a above — this story's remaining task is the dedicated test coverage), `web-app/src/components/sessions/TerminalOutput.tsx`

##### Task 2.1.8a: Wire `setShowReconnectBanner` through in `TerminalOutput.tsx` (~2 min)
- `TerminalOutput.tsx` already declares `const [showReconnectBanner, setShowReconnectBanner] = useState(false);` (line ~100) and already renders the `reconnectingBanner`/`hardFailedBanner` divs off `showReconnectBanner` (lines 1600-1608) — no new state or UI needed. Task 2.1.1c already passes `setShowReconnectBanner` into `useVisibilityResync(...)`'s params; this task is just the explicit confirmation/no-op check that no second banner state was introduced.
- Files: `web-app/src/components/sessions/TerminalOutput.tsx` (verification only — no diff beyond Task 2.1.1c's).

##### Task 2.1.8b: Regression test — banner shown after 2s pending, hidden on late completion (~5 min)
- In `useVisibilityResync.test.ts`, add:
  ```ts
  it('useVisibilityResync_should_showReconnectBanner_When_resyncPendingPast2Seconds', () => {
    const params = makeParams();
    renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300); // debounce elapses, resync fires
    });
    expect(params.setShowReconnectBanner).not.toHaveBeenCalled();

    act(() => { jest.advanceTimersByTime(2000); });
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(true);
  });

  it('useVisibilityResync_should_notShowReconnectBanner_When_resyncCompletesBefore2Seconds', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300);
    });
    act(() => { jest.advanceTimersByTime(500); result.current.notifyResyncOutputReceived(); });
    act(() => { jest.advanceTimersByTime(2000); });

    expect(params.setShowReconnectBanner).not.toHaveBeenCalled();
  });

  it('useVisibilityResync_should_hideReconnectBanner_When_resyncCompletesAfterBannerShown', () => {
    const params = makeParams();
    const { result } = renderHook(() => useVisibilityResync(params));

    act(() => {
      Object.defineProperty(document, 'visibilityState', { value: 'visible', configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
      jest.advanceTimersByTime(300);
    });
    act(() => { jest.advanceTimersByTime(2000); }); // banner shown
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(true);

    act(() => { jest.advanceTimersByTime(1000); result.current.notifyResyncOutputReceived(); });
    expect(params.setShowReconnectBanner).toHaveBeenCalledWith(false);
  });
  ```
  `makeParams()` needs a `setShowReconnectBanner: jest.fn()` field added to its base object (Task 2.1.1d's helper).
- Files: `web-app/src/components/sessions/__tests__/useVisibilityResync.test.ts`

---

#### Story 2.1.7: `tsc --noEmit` re-verification (resolves architecture review Blocker's Phase-2 requirement)
**As a** reviewer, **I want** confirmation that `TerminalOutput.tsx` and the new `useVisibilityResync.ts` type-check cleanly against the widened `Promise<void>` `connect`/`disconnect` types, **so that** the `disconnect().then(() => connect())` watchdog chain (which requires the Task 1.1.1d widening) is proven to compile, not just assumed to.
**Acceptance Criteria**:
- `npx tsc --noEmit` passes with zero errors after all of Phase 2's edits to `TerminalOutput.tsx` and `useVisibilityResync.ts`.
**Files**: none (verification only).

##### Task 2.1.7a: Run the second `tsc --noEmit` pass (~2 min)
- Run `cd web-app && npx tsc --noEmit` after Stories 2.1.1-2.1.6 are implemented. Confirm zero errors, specifically checking that `disconnectRef.current().then(() => connectRef.current())` type-checks against the widened `Promise<void>` return type from Task 1.1.1d.
- Files: none (verification task).

---

### Epic 2.2: Protect existing triggers (AC6)

**Goal**: Prove the three pre-existing triggers (mount-time `fitAddon.fit()`, `ResizeObserver`-driven fit, `handleManualReconnect`) were not touched by any task above, and that `useTerminalStream.ts`'s protected V2 listener block specifically is unmodified (the file as a whole *does* change, legitimately, via Epic 1.1's additive/widening edits — the scoped check below targets only the protected block, not the whole file).

#### Story 2.2.1: Scoped diff confirms zero changes to protected code
**As a** reviewer, **I want** proof that `XtermTerminal.tsx`, the V2 listener block in `useTerminalStream.ts`, and `TerminalOutput.tsx`'s `handleManualReconnect` are unmodified, **so that** I can trust the three pre-existing full-resync/refit triggers still behave exactly as before.
**Acceptance Criteria** (AC6):
- A scoped `git diff` against `XtermTerminal.tsx` shows zero changes; the `NEXT_PUBLIC_RECONNECT_V2` listener block in `useTerminalStream.ts` (lines 420-446) is unmodified; `handleManualReconnect` in `TerminalOutput.tsx` (lines 1010-1015) is unmodified. `useVisibilityResync.ts` being a wholly new file is expected and not a violation — AC6 protects three specific *existing* triggers, not "zero new files."
  - *Given* the full branch diff after all Phase 1/2/3 tasks are complete, *When* `git diff main -- web-app/src/components/sessions/XtermTerminal.tsx` is run, *Then* the output is empty; *When* `git diff main -- web-app/src/lib/hooks/useTerminalStream.ts | grep -A30 "NEXT_PUBLIC_RECONNECT_V2"` is run, *Then* the matched hunk shows no `+`/`-` lines inside the listener's registration/handler body (only the unrelated Epic 1.1 additions elsewhere in the file are expected); and `git diff main -- web-app/src/components/sessions/TerminalOutput.tsx` shows no changes within the `handleManualReconnect` function body.
**Files**: none (verification only — this story produces no code).

##### Task 2.2.1a: Run and record the scoped diffs (~3 min)
- Run `git diff main -- web-app/src/components/sessions/XtermTerminal.tsx` and confirm no output.
- Run `git diff main -- web-app/src/lib/hooks/useTerminalStream.ts | grep -A30 "NEXT_PUBLIC_RECONNECT_V2"` and confirm no `+`/`-` lines inside the listener body.
- Run `git diff main -- web-app/src/components/sessions/TerminalOutput.tsx | grep -A6 'handleManualReconnect = useCallback'` and confirm the matched hunk shows no `+`/`-` lines inside the function body.
- Paste all three command outputs into the PR description per AC0's requirement below.
- Files: none.

---

## Phase 3: Full-repaint integration proof (AC7)

### Epic 3.1: Real `TerminalStreamManager` + real xterm `Terminal` integration test

**Goal**: Prove, against a real (non-mocked) `TerminalStreamManager` and a real `@xterm/xterm` `Terminal` instance, that writing the exact `clearAndHome + content` payload the server sends for a `CurrentPaneRequest` response produces a genuine full repaint over stale/corrupted prior content — not a mocked call-count assertion.

#### Story 3.1.1: Integration test against `TerminalStreamManager.write()`
**As a** developer verifying the resync mechanism, **I want** a test that seeds a real xterm buffer with stale content, writes the real server-shaped resync payload, and reads back the real rendered buffer, **so that** AC7 is proven against actual xterm.js ANSI-parsing behavior, not an assumption about it.
**Acceptance Criteria** (AC7):
- Seeding stale/corrupted content, then calling `manager.write(clearAndHome + freshContent)`, results in the real xterm `Terminal`'s buffer containing only the fresh content, verified via `terminal.buffer.active.getLine(n).translateToString()`.
  - *Given* a real `@xterm/xterm` `Terminal` instance and a real `TerminalStreamManager` wrapping it (`new TerminalStreamManager(terminal, jest.fn())`), where `manager.write("STALE LINE ONE\r\nSTALE LINE TWO — CORRUPTED\r\n")` has already been called (simulating what a backgrounded tab's coalesced/dropped deltas would leave on screen), *When* `manager.write(clearAndHome + "FRESH LINE ONE\r\nFRESH LINE TWO\r\n")` is called, where `clearAndHome = "\x1b[!p\x1b[2J\x1b[H"` (matching `ansiSnapshotPrefix` in `server/services/connectrpc_websocket.go:129` exactly), *Then* `terminal.buffer.active.getLine(0).translateToString(true)` equals `"FRESH LINE ONE"` and `terminal.buffer.active.getLine(1).translateToString(true)` equals `"FRESH LINE TWO"`, with no leftover glyphs from "STALE LINE" anywhere in the visible buffer (checked by scanning all populated lines up to `terminal.buffer.active.length` for the substring `"STALE"` and `"CORRUPTED"`, asserting neither is found).
**Files**: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.resync.test.ts` (new)

##### Task 3.1.1a: Create the test file and instantiate a real `Terminal` + `TerminalStreamManager` (~4 min)
- Create `web-app/src/lib/terminal/__tests__/TerminalStreamManager.resync.test.ts`.
- Import `{ Terminal } from '@xterm/xterm'` (real, unmocked — do not add a `jest.mock('@xterm/xterm', ...)` call in this file) and `{ TerminalStreamManager } from '../TerminalStreamManager'`.
- In a `beforeEach`, construct `const terminal = new Terminal({ cols: 80, rows: 24, allowProposedApi: true });` and `const manager = new TerminalStreamManager(terminal as unknown as import('../TerminalStreamManager').ITerminal, jest.fn());`. Do not call `terminal.open(...)` — xterm.js's core `Terminal.write()`/buffer/parser machinery operates headlessly without a DOM container; only the renderer addons (canvas/webgl/dom) require `open()`, and none are loaded here.
- Add `afterEach(() => { terminal.dispose(); });`.
- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.resync.test.ts`

##### Task 3.1.1b: Write the seed-then-resync-then-assert test body (~5 min)
- Define `const clearAndHome = "\x1b[!p\x1b[2J\x1b[H";` at the top of the test (module- or describe-scope constant) with a comment reading `// keep in sync with ansiSnapshotPrefix (connectrpc_websocket.go)` — matches `ansiSnapshotPrefix` in `server/services/connectrpc_websocket.go:126-129` (`ansiDECSTR + ansiEraseScreen + ansiCursorHome`) exactly. This is a manually-duplicated constant with no compile-time link (adversarial review Minor), so the comment is the only cross-reference; see Task 3.1.1d for the matching comment on the server side.
- Write the test body per the Given-When-Then in Story 3.1.1's Acceptance Criteria above. Because `RedrawThrottler.process()` only throttles chunks matching `/^\x1b\[\d+A(?:\x1b\[2K|\x1b\[J)/` (cursor-up + erase) and neither the stale seed content nor `clearAndHome` (which starts with `\x1b[!p`, not `\x1b[<N>A`) match that pattern, both `write()` calls pass through `writeDirectWithFlowControl` synchronously — no `jest.useFakeTimers()`/`advanceTimersByTime` is needed for the writes themselves to take effect on the buffer. If any assertion needs to wait for xterm's internal async write completion callback, wrap the write + assertion in `await new Promise<void>(resolve => manager.write(payload); /* terminal.write's callback param fires after parse */)` — prefer using the two-arg `terminal.write(data, callback)` awaited via a promise wrapper if a flake is observed in Task 3.1.1c, rather than adding fake timers (which would reintroduce the throttle-timing complexity this payload shape avoids).
- Files: `web-app/src/lib/terminal/__tests__/TerminalStreamManager.resync.test.ts`

##### Task 3.1.1c: Run the test and confirm it exercises real xterm parsing (~3 min)
- Run `cd web-app && npx jest TerminalStreamManager.resync.test.ts`.
- Confirm the test fails if `clearAndHome` is temporarily omitted from the second `write()` call in a local scratch edit (sanity check that the test actually distinguishes "cleared" from "not cleared" — revert the scratch edit after confirming). This is a manual verification step during authoring, not a permanent test.
- Files: none (verification).

##### Task 3.1.1d: Add the matching cross-reference comment on the server side (~2 min, backend nitpick)
- In `server/services/connectrpc_websocket.go`, at the `ansiSnapshotPrefix` constant declaration (line 129), add a one-line comment above it: `// keep in sync with clearAndHome (web-app/src/lib/terminal/__tests__/TerminalStreamManager.resync.test.ts)` — so a future change to the server's escape sequence surfaces the test-side duplicate to whoever edits it, closing the loop in both directions (adversarial review Minor). This is the one task in this plan that touches a Go file; it is a comment-only change with zero behavioral impact.
- Files: `server/services/connectrpc_websocket.go`

---

## Phase 4: Manual verification (AC0)

### Epic 4.1: Manual repro and PR documentation

#### Story 4.1.1: Manual repro recorded in the PR description
**As a** reviewer, **I want** a recorded manual repro of the fix, **so that** AC0 (which is explicitly not automatable — it's about real browser tab-backgrounding behavior) is verifiably satisfied.
**Acceptance Criteria** (AC0):
- Returning to a backgrounded tab whose pane was mid-TUI-redraw shows a clean terminal with no manual action, verified by manual repro and recorded in the PR description.
  - *Given* a running `stapler-squad` instance with an active Claude session mid-way through a TUI option-picker redraw, *When* the browser tab is backgrounded for at least 30 seconds (long enough for Chrome's background-tab timer throttling to coalesce/drop control-mode deltas per the root-cause description in `requirements.md`) and then refocused, *Then* the terminal pane shows the current, correct tmux pane content within roughly one debounce+RPC round-trip (well under a second in the common case) with no visible stale/overlapping glyphs and no manual reconnect click required — captured as a before/after screen recording or screenshot pair and pasted into the PR description, alongside the AC6 scoped-diff output from Task 2.2.1a.
**Files**: none (process/documentation task, no source files).

##### Task 4.1.1a: Perform the manual repro against a local build (~5 min, outside normal task-sizing since it requires a live session — run once at the end of implementation)
- Run `make restart-web`, open a session with an active TUI (e.g. Claude's own option picker), background the tab for 30+ seconds, refocus, confirm clean repaint. Explicitly note in the write-up whether any visible flash/flicker was observed during the repaint (triad-review UX/Product Concern: the AC7 integration test proves buffer correctness but can't detect visible flicker — this manual step is the only check for it).
- Repeat once with an actual OS-level suspend/resume (close the laptop lid or use the OS's sleep function for at least a minute, not just tab-backgrounding) rather than only simulating it via tab-switching — pre-mortem P2 #2 and the triad-review Product lens both flagged that a genuinely suspended machine is closer to the real-world trigger in the original bug report than a merely-backgrounded-but-awake tab, and the two can behave differently (e.g. a dead half-open socket that `isConnected` doesn't reflect). Note whether the 4s stall watchdog actually recovers, or whether recovery takes longer than promised — record this as a known-limitation follow-up if it does, don't block the PR on it.
- Record the result (screen recording or before/after screenshots) and paste into the PR description together with the AC6 diff output (Task 2.2.1a's three `git diff` results).
- Files: none.

---

## Phase 5 — dropped (scope drift, adversarial review Concern)

The original draft's Phase 5 (`aria-live`/`role` wiring on the existing
`reconnectingBanner`/`hardFailedBanner` divs) has been **removed from this
plan entirely**, not merely deprioritized. Requirements.md's "In scope"
section lists exactly three items; accessibility/ARIA wiring is not among
them, and no acceptance criterion (AC0-AC7) references it. Bundling an
"optional phase" into the same plan/PR diluted the tightly-scoped-bugfix
story this plan uses to justify skipping an ADR ("touches N files, trivially
reversible").

**Disposition**: track `aria-live`/`role` wiring on `reconnectingBanner`/
`hardFailedBanner` as a separate, independent follow-up ticket. It has no
dependency on anything in Phases 1-4 above and can land before, after, or
alongside this PR without coordination — the original draft's Story 5.1.1
acceptance criteria and Task 5.1.1a's exact diff (roles/attributes on the two
banner `<div>`s) remain valid and reusable verbatim when that follow-up is
picked up; they are simply no longer part of *this* plan's task list or ACs.

---

## ADR decision note

No ADR is written for this change. Per `sdd:3-plan`'s guidance, ADRs are for
non-standard, hard-to-reverse architectural choices. Every decision in this
plan (Pattern Decisions table above) either (a) reuses an existing,
already-shipped idiom verbatim (ref-mirroring, debounced-listener shape,
banner reuse), or (b) is trivially reversible by reverting a small PR with no
schema/infra/API-contract impact. This includes the post-review hook
extraction (`useVisibilityResync.ts`): it is a placement/testability
refinement of the already-chosen Approach B, not a new architectural
direction — it introduces no new pattern beyond "custom hook co-located with
its consumer," an idiom already used throughout `web-app/src/lib/hooks/` and
`web-app/src/components/sessions/`. `build-vs-buy.md` already validated "no
new dependency" is correct, not just assumed. None of these choices meet the
bar for a recorded architecture decision.
