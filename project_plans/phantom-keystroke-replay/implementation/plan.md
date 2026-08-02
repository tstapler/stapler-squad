# Implementation Plan: phantom-keystroke-replay

Backlog item: `04089969-0f19-499c-be34-2e8bcfc4f13e`

## Scope recap

Server-side root cause (session_driver.go auto-answer resend loop, #165) is
**already fixed on `main`** via `3546c2b12` and `c0e6c4ce6` — no changes to
`session/session_driver.go`'s approval logic in this plan. Remaining scope is
client-side reconnect hardening (AC3, AC4) plus regression coverage tying the
already-shipped server fix to this backlog item (AC1, AC2), a Go bounded
read-goroutine exit test (AC4), and a manual repro pass (AC5). AC6 is
satisfied by requirements.md's Non-Goals section; verified, no task.

---

## Step 0.5 — Alternatives considered (creative pass)

Three high-level approaches were brainstormed for the client-side reconnect
fix before committing to one. Full detail — including a fourth,
component-placement alternative for the drop-signal UI — is in the Pattern
Decisions table below; summary:

1. **Monotonic connection-epoch counter** (chosen) — a `useRef(0)` incremented
   at every `connect()` attempt, compared at every checkpoint after an
   `await`. Strength: direct precedent already proven in this exact codebase
   (`useSessionService.ts`'s `streamGenerationRef`, guarding the sibling
   `WatchSessions` stream against the identical overlapping-reconnect race).
   Weakness: needs care to place the increment synchronously before the first
   `await` and to guard *every* resumption point (loop body, catch, finally),
   not just the happy path — a well-understood, not novel, risk.
2. **AbortController-signal-only guard** (rejected) — rely solely on
   `abortControllerRef.current.signal.aborted` instead of adding a new epoch
   ref. Strength: no new primitive. Weakness: an `AbortSignal` is a
   single-shot boolean, not an identity — it can tell "has *a* newer attempt
   superseded me" only if exactly one swap ever happens, but cannot
   distinguish attempt N+1 from N+2 when three overlapping `connect()` calls
   race (the triple-rapid-connect case AC4 explicitly requires coverage for).
   `useTerminalStream.ts` already has `abortControllerRef` for network
   cancellation; overloading it as a JS-closure-invalidation signal too
   conflates two different jobs and still needs a per-attempt captured
   reference to compare against, which is the same complexity as an epoch
   ref without its determinism.
3. **Serialize all `connect()` calls through a promise-chain mutex** (rejected)
   — prevent overlap by construction rather than detecting-and-ignoring it.
   Strength: eliminates the race class entirely. Weakness: adds latency to
   legitimate rapid reconnects (visibility + online firing close together —
   exactly the ticket's flapping scenario) by queueing them behind each
   other, and introduces a new async-serialization primitive not used by any
   sibling stream in this codebase. Over-engineered for what pitfalls.md and
   architecture.md both characterize as "small, targeted hardening, not a
   new subsystem."

---

## Domain Glossary

| Term | Definition | Realized as |
|---|---|---|
| Connection epoch | A monotonically increasing integer identifying one `connect()` attempt's lifetime, from entry to its `finally` block completing. `disconnect()` also reads (never increments) this same counter to detect that a newer `connect()` has superseded it mid-disconnect. | `connectionEpochRef: React.MutableRefObject<number>` in `useTerminalStream.ts` |
| Current epoch / `epoch` | The epoch value captured locally by a specific `connect()` invocation immediately after that invocation's entry guard (`isConnectedRef.current \|\| isConnectingRef.current \|\| !sessionId`) has passed, and before any `await` — a call that the guard blocks returns before ever reaching this increment, so a guard-blocked no-op never consumes an epoch or orphans the real in-flight attempt; `disconnect()` captures the counter at its own entry (without incrementing) for the same comparison purpose. | `const epoch = ++connectionEpochRef.current` (connect, placed after the entry guard); `const epochAtDisconnectStart = connectionEpochRef.current` (disconnect) |
| Superseded attempt | A `connect()` invocation whose captured `epoch` no longer equals `connectionEpochRef.current` — a newer attempt has since started. The same comparison, applied to `disconnect()`'s captured value, detects a `connect()` that started and is now the current epoch while `disconnect()` was still awaiting. | Comparison `epoch !== connectionEpochRef.current` at each checkpoint |
| Drop count | The number of `TerminalData` messages discarded from a `MessageQueue`'s internal buffer at the moment `close()` is called. | Return value of `MessageQueue.close(): number` |
| `DroppedInputEvent` / `recordDrop` | A smart constructor making the domain-illegal `count <= 0` state unrepresentable at the point a drop is reported: `recordDrop(count: number): DroppedInputEvent \| null` returns `null` for `count <= 0`, otherwise `{ count, at: Date.now() }`. Used at all three drop-reporting call sites (`connect()`'s close-before-install, `disconnect()`'s close, `useTerminalFlowControl`'s `onDrop`) instead of each duplicating a `> 0` guard — a type-driven-design smart-constructor / parse-don't-validate pattern. | `function recordDrop(count: number): DroppedInputEvent \| null` in `useTerminalStream.ts`; `type DroppedInputEvent = { count: number; at: number }` |
| Dropped-input event | A single reported drop occurrence — one `recordDrop()` result — exposed by the hook as `droppedInputEvent` for the UI layer. This is **not** itself a coalesced/running total across renders; `useTerminalStream` reports each occurrence as it happens and performs no accumulation of its own *across distinct batches*. The one exception is `reportDrop`'s same-React-batch merge guard (Task 4.1.1.3, added during plan repair per pre-mortem.md Failure #3, P2): if two of the hook's three drop call sites fire within the same synchronous React batch, their counts are merged into a single combined event so neither is silently lost to React's last-write-wins batching — this is a correctness fix for same-tick occurrences, not a substitute for `InputDropBadge`'s cross-render coalescing. | `droppedInputEvent: DroppedInputEvent \| null` returned from `useTerminalStream`; merge guard via `dropBatchRef` inside `reportDrop` |
| Drop-and-signal badge | The visible + audible UI signal shown when input is dropped. Owns **all** coalescing across occurrences — both the visual running-total pill and triggering the assertive announcement — so `useTerminalStream`/`TerminalOutput.tsx` never coalesce or announce on their own. | `InputDropBadge` component (`web-app/src/components/sessions/InputDropBadge.tsx`) |
| Assertive announcement | The `aria-live="assertive"` screen-reader announcement accompanying a drop, distinguishing it from routine `polite` connection-status chatter. Invoked by `InputDropBadge` itself (via its own `useLiveRegion()` call), not by a `TerminalOutput`-level effect keyed on the hook's raw event. | `useLiveRegion().announce(...)` with `politeness="assertive"`, `role="alert"`, called from inside `InputDropBadge` |
| Coalescing window | The time window — "however long the badge is continuously visible" (its dwell timer, reset on each new occurrence) — during which multiple `droppedInputEvent` occurrences are batched into a single running-total badge update **and** a single re-triggered announcement per content change, rather than one announcement per occurrence. | Local timer/ref state inside `InputDropBadge`, which also owns invoking `announce()` for the window |
| Bounded read-goroutine exit | The guarantee that the WebSocket input-forwarding goroutine (Goroutine 2 of `streamViaControlMode`) terminates within a fixed timeout once its underlying connection closes, rather than leaking indefinitely. | `controlModeReadLoop` + `readWG sync.WaitGroup` + `waitWithTimeout` in `server/services/connectrpc_websocket.go` / `_test.go` |
| `controlModeReadLoop` | Extracted function implementing Goroutine 2's read-and-classify loop, isolated from `session.Instance`/tmux dependencies so it is directly testable against a real (tmux-free) `*websocket.Conn`. | New method on `ConnectRPCWebSocketHandler` |
| Read-loop WaitGroup (`readWG`) | Instrumentation making Goroutine 2's exit externally observable for tests, without changing its read/error-handling behavior. | `var readWG sync.WaitGroup` in `streamViaControlMode` |
| Regression backlog item | This work's tracking ID, referenced in new test doc comments so future readers can trace *why* a given assertion exists. | `04089969-0f19-499c-be34-2e8bcfc4f13e` |

---

## Pattern Decisions

| Decision | Chosen pattern | Alternative Rejected | Reason |
|---|---|---|---|
| Client reconnect race guard | Monotonic `connectionEpochRef` (mirrors `useSessionService.ts:185,829,833`) | AbortController-signal-only guard | Signal is a single-shot boolean, cannot distinguish 3 overlapping attempts (AC4's triple-rapid-connect case); see Step 0.5 #2. |
| Client reconnect race guard | (same) | Promise-chain mutex serializing `connect()` calls | Adds latency to legitimate rapid reconnects during the exact flapping scenario this bug is about; new primitive with no precedent in this codebase; see Step 0.5 #3. |
| `disconnect()` vs. in-flight `connect()` race | `disconnect()` captures `connectionEpochRef.current` at entry (read-only, no increment — it isn't starting a new attempt) and gates its post-`await` state mutations (`setIsConnected(false)`, decoder resets) on that captured value still matching `connectionEpochRef.current`; the `isDisconnectingRef.current = false` bookkeeping reset always runs regardless | Leave `disconnect()` unguarded and rely solely on `shouldReconnectRef.current = false` (set synchronously at `disconnect()`'s entry) to make the race harmless | `shouldReconnectRef` only stops a *future* auto-reconnect from being scheduled — it does nothing to prevent `disconnect()`'s own pending `await` continuation from later clobbering state that a *different*, already-in-flight `connect()` call (e.g. the visibility/online listener firing between `disconnect()`'s entry and its `await` resolving) has since set. Both architecture-review.md and adversarial-review.md independently flagged this gap (the latter noting it was also raised, and left unresolved, in this exact codebase's own prior unmerged adversarial review); three independent findings on the same gap is treated as strong signal that it needs a code-level guard, not just a documented justification in ADR-001. |
| `MessageQueue.close()` drop semantics | Clear `this.queue = []` in place, return dropped count | Add a separate `abandon()`/`discard()` method distinct from `close()` | No caller in this codebase ever wants a superseded/closing queue to still flush buffered input afterward (confirmed: `close()` has one call shape, always reconnect- or disconnect-triggered, both of which need discard semantics per ADR-023's "one-shot MessageQueue" rationale) — a second method is speculative API surface with no real second caller (interface-pollution-checklist smell #1). |
| Drop-signal UI placement | New `InputDropBadge`, mounted per-terminal-instance in `TerminalOutput.tsx` | Extend the existing global `ConnectionIndicator` (`components/layout/`) to also show input-drop state | `ConnectionIndicator` is one instance per app shell representing session-list-wide connection status (no `sessionId` prop); input-drop is inherently per-open-terminal state owned by `useTerminalStream`. Reaching a global component would require prop-drilling/new context to carry per-session state into a component that structurally isn't per-session. ux.md research explicitly recommends keeping the two "distinct and complementary." |
| No new interface/abstraction | Patch `MessageQueue` and `useTerminalStream` in place; no `Manager`/`Service` wrapper | — | Per `.claude/rules/interface-pollution-checklist.md`: `MessageQueue` is already a minimal single-purpose class with exactly one real shape of caller; wrapping it or introducing an interface would be a speculative interface (smell #1) with no second implementation in sight. Confirmed independently by `research/build-vs-buy.md`. |
| Coalescing/debounce ownership | Local state inside `InputDropBadge` (no new custom hook) owns **both** the visual running-total pill **and** triggering the assertive `announce()` call for its coalescing window — `useTerminalStream`/`TerminalOutput.tsx` never compute a running total or call `announce()` themselves | (1) A shared `useCoalescedAnnouncement()` hook; (2) hook-level running-total accumulation in `useTerminalStream` (`setDroppedInputEvent` computing a cumulative count itself) with `TerminalOutput.tsx` firing `announce()` from a `useEffect` keyed on raw `droppedInputEvent?.at` | (1) Single consumer today; a generic hook for one call site is an unjustified generic (interface-pollution-checklist smell #5) — write the concrete version first. (2) This was the plan's original (buggy) design: an inline `prev?.at === Date.now()` comparison at the hook level never matched (two different `Date.now()` calls), so counts never accumulated, and a raw per-`at` effect fired one assertive announcement per drop instead of one per coalescing window — directly contradicting research/ux.md §4's "coalesce, don't spam" requirement. Splitting ownership across two layers also let the spoken count (latest event only) diverge from the displayed running total. Both reviews (architecture-review.md, adversarial-review.md) independently flagged this; consolidating ownership entirely in `InputDropBadge` fixes both the bug and the split-responsibility smell. |
| Illegal-state prevention for drop reports | `recordDrop(count: number): DroppedInputEvent \| null` smart constructor, used at every call site that reports a drop | Each call site duplicating its own `count > 0` guard before constructing `{ count, at: Date.now() }` inline | A domain-illegal `count <= 0` `DroppedInputEvent` was representable at the type level even though `InputDropBadge` already treats `null` as the "no event" sentinel — per type-driven-design's smart-constructor/parse-don't-validate pattern, a single helper makes the illegal state unconstructable and is also more reviewable than a bespoke inline updater (this is exactly the kind of bug a shared helper would have caught — see the coalescing-ownership row above). |
| `LiveRegion` accessibility role | Add an optional `role` prop (default `"status"`, backward compatible) so `InputDropBadge` can pass `role="alert"`; `InputDropBadge` renders its own (visually hidden) `<LiveRegion>` instance internally, alongside the visible pill, rather than `TerminalOutput.tsx` rendering a separate `LiveRegion` driven by a hook-level effect | Compose a second, parallel live-region primitive for the assertive case | `LiveRegion`/`useLiveRegion` currently has **zero real consumers** (`ConnectionIndicator.tsx` hand-rolls its own `aria-live` div instead of importing it — confirmed by grep, no import statement exists). `InputDropBadge` becomes its first real adopter; extending the existing shared primitive with a role prop is minimal and keeps this codebase from growing a second live-region pattern, per pitfalls.md's explicit warning against ad-hoc bespoke patterns. Keeping the `LiveRegion` instance inside `InputDropBadge` (rather than split across `InputDropBadge` for visuals and `TerminalOutput.tsx` for the announcement) is what makes single-owner coalescing possible — see the coalescing-ownership row above. |
| Go read-goroutine testability | Extract `controlModeReadLoop` (blocking-read + exit classification only; business-logic dispatch stays a caller-supplied closure) | Test the pattern in isolation with a synthetic goroutine that merely *mirrors* the shape, without touching production code | pitfalls.md explicitly warns that a test proving only "the pattern is correct" (not "this codebase's actual code uses the pattern correctly") repeats the exact false-positive-marking failure mode that already happened once on this ticket (AC3 was marked done against code that didn't do what was claimed). Extracting real production logic into a directly-callable, tmux-free function ties the test to the actual code path. |
| Go bounded-exit enforcement | Test-only: instrument `readWG` to make existing (already-correct, per architecture.md) exit-on-`conn.Close()` behavior observable; do **not** add a blocking `readWG.Wait()` call inside `streamViaControlMode` before it returns | Add a synchronous bounded-wait inside `streamViaControlMode` before returning | `HandleWebSocket`'s `defer conn.Close()` (the thing that actually unblocks Goroutine 2's `ReadMessage()`) only runs **after** `streamViaControlMode` returns, one stack frame up. A blocking wait added before that return would deadlock/timeout on every single normal disconnect, since the very close that would satisfy the wait hasn't happened yet — this would add multi-second latency (or spurious timeout logs) to the common case. Flagged explicitly in this plan's Unresolved Questions as a real, but out-of-scope-for-this-ticket, latent goroutine-leak vector (a `doneChan` close triggered by something other than the connection itself dying). |

---

## Migration Plan

Omitted — no schema, proto, or persisted-data changes in this plan.

---

## Observability Plan

- **Client**: existing `console.info("[reconnect] stream=terminal ...")` /
  `console.warn` logging in `useTerminalStream.ts` (lines 319, 340) is left
  as-is; add one new log line at the point a drop is detected —
  `console.warn('[useTerminalStream] dropped N buffered input message(s) on close/supersede', { sessionId, count })`
  — so a drop is visible in browser devtools even before/without the visual
  badge rendering (e.g. if a component consumer forgets to mount the badge).
- **Client UI**: the `InputDropBadge` + assertive `LiveRegion` announcement
  *are* the primary observability surface for end users, per AC3 — this is
  the feature, not incidental logging.
- **Go server**: `controlModeReadLoop`'s existing `log.Error("[streamViaControlMode] WebSocket read error", ...)` line is preserved unchanged by the extraction (Phase 5). No new production log lines are added — `readWG` is test-only instrumentation with no runtime logging (see Pattern Decisions: no blocking wait, so nothing to warn about on the happy path).
- **Test-only observability**: the new Go test asserts on `waitWithTimeout`'s boolean return (matching `TestWaitWithTimeout`'s existing convention), not on log output — keeps the test deterministic and fast (`-race` clean).

---

## Risk Control

| Risk | Mitigation |
|---|---|
| Rewriting `MessageQueue.test.ts`'s `'should yield messages in order'` test (currently asserts the buggy behavior) could silently narrow coverage if not replaced with an equally strong assertion. | Task 2.1.1.2 explicitly rewrites it to assert the *opposite* (queued-but-unsent messages are dropped after `close()`), and Task 2.1.1.3 adds a same-tick-push-during-close race test per pitfalls.md §1's "resolve-callback races with close" warning. |
| A generation-ref port that increments only once (either only in `connect()` or only inside the message-loop IIFE) silently under-protects, per pitfalls.md's explicit warning about `useSessionService.ts`'s *double*-increment shape. | Story 3.1.1's tasks explicitly mirror both increment points: once in `connect()`, immediately after its entry guard passes (not before it — see Task 3.1.1.1 and pre-mortem.md Failure #1 — and not a second time inside the message-loop IIFE), and epoch comparisons at all three checkpoints named in architecture.md §1 (loop body, catch, finally) — not just one. |
| Fixing `MessageQueue.close()` in isolation without exercising `useTerminalStream`'s actual usage of it reproduces the same "fixed the class, not the usage" gap that let AC3 get marked done incorrectly once already. | Story 3.2.1's "queued-message-drop-on-close interleaving" Jest test exercises `useTerminalStream` (via `renderHook`), not `MessageQueue` in isolation — per pitfalls.md §5. |
| `aria-live="assertive"` is unproven in this codebase (no existing shipped consumer) — cross-browser/AT behavior risk. | Story 4.2.3 includes an explicit accessibility-focused Jest test asserting `role="alert"` + `aria-live="assertive"` + `aria-atomic="true"` are present; full manual AT verification is called out as a follow-up in Unresolved Questions (out of scope for automated CI). |
| Rapid-repeat drops within `useLiveRegion`'s fixed 1000ms auto-clear window can silently swallow a second announcement (pitfalls.md §4, "clearImmediately" race). | Coalescing lives in `InputDropBadge`, not raw `announce()` calls — every drop within the debounce window updates a running count and re-triggers a single, content-changing (`"N keystrokes not sent"`) announcement, so identical-string suppression by AT is avoided by construction. |
| Three independent hook-level drop call sites (reconnect-close, `disconnect()`-close, flow-control `onDrop`) can each call `setDroppedInputEvent` within the same React 18 synchronous batch; React's last-write-wins batching means the earlier occurrence's count would be silently lost before it ever reaches `InputDropBadge`'s coalescing logic (pre-mortem.md Failure #3, P2). | Task 4.1.1.1's `reportDrop` helper merges same-batch occurrences via `dropBatchRef`, reset only after the batch's resulting render commits (Task 4.1.1.3's Jest test proves the merge, added during plan repair). |
| Go: extracting `controlModeReadLoop` is a refactor of live production code (not just additive) — regression risk to the two other structurally similar handlers (`streamShellViaControlMode`, `streamViaTmuxCapturePane`) if touched by mistake. | Phase 5 scope is explicitly `streamViaControlMode` only, matching requirements.md's file listing; the other two duplicated handlers are named as a flagged follow-up in Unresolved Questions, not touched here (Non-Goals: "general reconnect/re-render stability work beyond what's needed... is out of scope"). |
| Task 1.1.2.1's AC2 regression test could reimplement `session_driver.go`'s `approvalAwaitingClear` threading rule inline in the test instead of driving the real production method that does this threading — the same "tested the pattern, not production code" gap that already caused a documented FAIL on this ticket once (pre-mortem.md Failure #5, P2). | Task 1.1.2.1 (repair pass) first extracts `processApprovalTick` from `runSessionDriverWithPrompt`'s inline `NeedsApproval` block, mirroring Phase 5's `controlModeReadLoop` extraction precedent; Task 1.1.2.2's test then drives `processApprovalTick` directly across simulated ticks rather than re-implementing its rule. |
| AC5 manual repro requires actually inducing the "not started or paused" condition, not a generic network-offline toggle (per Constraints). | Story 6.1.1's task uses `pause_session`/`resume_session` MCP tools against a live tmux-backed session while its terminal WebSocket is open in a real browser tab, which drives `session/instance_tmux.go:471-472`'s `!i.started.Load() \|\| i.Status == Paused` → `"session not started or paused"` path directly — the exact error string from the ticket's log excerpt. |
| A session with frequent short, benign reconnects (e.g. flaky wifi with quick auto-recovery) could show `InputDropBadge` repeatedly — each firing is a real (true-positive) drop, but `count` is legitimately small (often `1`) per blip, so the cumulative effect across many quick, self-healing blips can feel noisy/alarmist relative to how inconsequential each individual drop is — a UX risk under real-world network conditions this plan's automated tests can't reproduce. | Accepted tradeoff, not fixed here: correctness over silence — a false-negative silent loss is worse than an occasional true-positive badge (see `design/ux.md`'s Surface C residual-risk note). Flagged as something to watch in production usage; a dismiss-suppression window (e.g. "don't show for N seconds after the last dismiss") would trade correctness for quiet and is scope creep beyond this ticket's bug-fix mandate — left as a potential follow-up UX ticket if it proves annoying in practice. |
| AC5's repro must exercise the actual client-side fix (Phases 2-4), not just server-side `pause_session` behavior — an MCP-level `write_to_session`/`send_control` call bypasses the browser's WebSocket/`MessageQueue`/epoch-guard path entirely and proves nothing about the shipped fix. This was also this plan's own first repair attempt's mistake, corrected in this pass (pre-mortem.md Failure #2, P1). | Task 6.1.1.1 pins the mechanism concretely: `claude-in-chrome` browser-automation tools type a distinguishable marker string via real keyboard events into the live xterm DOM element in a real browser tab, with `pause_session`/`resume_session` MCP calls alternated tightly around the typing across all 5-10 cycles — best-effort adversarial timing, not guaranteed simultaneity; an observed `InputDropBadge`/drop-signal firing on at least one cycle is a required pass/fail assertion, and zero cycles showing the signal requires a tighter re-run or an explicit "inconclusive" outcome rather than a silent pass. |

---

## Unresolved Questions

1. **`doneChan` closing for a reason unrelated to the WebSocket connection itself dying** (e.g. an output-side send error to a channel other than `errChan`) is a real, separate goroutine-leak vector named in pitfalls.md §3, distinct from what AC4 asks this plan to test (which is specifically "exit is bounded once the connection closes"). Not fixed here — flagged as a follow-up. If it recurs, the fix is a proactive `stream.conn.Close()`/`SetReadDeadline` call at the point `streamViaControlMode` is about to return, which this plan's Pattern Decisions table explicitly rejected adding *unconditionally* (would break the common case's latency).
2. **`streamShellViaControlMode` and `streamViaTmuxCapturePane`** (`connectrpc_websocket.go:~1104`, `~1494`) are structurally duplicated goroutine-coordination code with the same `doneChan`/`select`/`default`/`ReadMessage()` shape as `streamViaControlMode`'s Goroutine 2, per pitfalls.md §3's explicit "one fix, N-1 near-duplicates unaudited" warning (itself citing this exact ticket's server-side history as the precedent for that failure mode). This plan intentionally does not extend the `controlModeReadLoop` extraction/test to those two handlers — out of scope per Non-Goals — but a future ticket should audit whether they need the same test coverage.
3. **Should the badge auto-dismiss timing (visual dwell) be a fixed constant or configurable?** ux.md recommends "a few seconds, tunable" but doesn't pin an exact number; this plan picks 4000ms (matching the rough order of magnitude research suggested, roughly 4x `useLiveRegion`'s 1000ms announcement-clear window so the visual badge outlives the announcement) as a starting value — flagged in case product/UX wants a different default after real usage.

**Resolved during plan repair (2026-08-02):** whether `useTerminalFlowControl.sendInput`'s
pre-existing silent early-return (`useTerminalFlowControl.ts:143`,
`if (!pushMessageRef.current || !isConnectedRef.current) return;`) should also
feed the drop signal was previously listed here pending stakeholder
confirmation. Resolved in-scope: requirements.md's Remaining confirmed gap
section and AC3 both describe the drop-and-signal requirement in terms of
*any* input silently lost around a disconnect boundary, not only input that
made it as far as `MessageQueue` before being discarded — a keystroke
rejected at the flow-control layer because the hook already knows it's
disconnected is the same class of silent input loss AC3 is about. Task
4.1.1.2 now implements this unconditionally rather than as a
confirmation-gated task; see that task and the adversarial-review.md nitpick
it addresses for why the other five near-identical guards in
`useTerminalFlowControl.ts` (resize/scrollback, not keystrokes) are
deliberately left unwired.

---

## Dependency Visualization

```
Phase 1 (AC1/AC2 regression, Go)         Phase 2 (MessageQueue fix, TS)
        │                                         │
        │                                         ▼
        │                              Phase 3 (epoch guard, TS)
        │                                 │              │
        │                                 ▼              ▼
        │                         Phase 4 (drop UI)   Phase 3 Jest tests
        │                                 │
        ▼                                 ▼
Phase 5 (Go bounded-exit test)   Phase 6 (manual repro, AC5)
        │                                 │
        └───────────────┬─────────────────┘
                         ▼
              Phase 7 (AC6 confirmation — no code)
```

Notes:
- Phase 1 (Go, `session_driver_test.go`) has no code dependency on Phases 2–4
  (different package, different file) and can run fully in parallel with them.
- Phase 3 depends on Phase 2 (the epoch guard's "unconditionally close the
  previous queue" behavior calls the fixed `close()`, and Phase 3's Jest
  tests assert on the fixed drop semantics).
- Phase 4 depends on Phase 3 (the `droppedInputEvent` state Phase 4 renders
  is produced by Phase 3's close()-call-site wiring).
- Phase 5 (Go) has no dependency on Phases 2–4 (different language/package)
  but is sequenced after Phase 1 only because both touch Go test files and
  are easiest to review as one batch; no actual coupling.
- Phase 6 (manual repro) should run last among the client-affecting phases —
  it's the end-to-end confirmation that Phases 2–4 actually closed the gap,
  so it must follow them, but has no dependency on Phase 1 or Phase 5.
- Phase 7 requires nothing (already satisfied by requirements.md); listed
  last purely for completeness of the AC-to-phase mapping.

---

## Phase 1 — Server-Side Regression Coverage for Already-Fixed Root Cause

Covers **AC1** (already satisfied — verification only) and **AC2** (already
satisfied by `#146`/`c0e6c4ce6`; this phase ties it to this backlog item with
a new test, not a re-fix).

### Epic 1.1 — Confirm AC1 and add a backlog-scoped regression test for AC2

#### Story 1.1.1 — AC1: confirm the duplication mechanism was already established with direct runtime evidence

No code task — AC1 is checked off in requirements.md and its Root Cause
section documents the direct runtime evidence (`3546c2b12`, `c0e6c4ce6`,
observed log lines) gathered during this project's Phase 0 research. This
story exists only to record the Given-When-Then for completeness.

**AC1 Given-When-Then:**
- **Given** session `mbr-skills` in the original bug report, with terminal
  stream flapping through connect → "stopped" → reconnect,
- **When** the SDD Phase 0 investigation (this project's `requirements.md`
  Root Cause section) traced the log lines `[streamViaControlMode]
  capture-pane failed, sending stopped notice   err="session not started or
  paused"` back to `session_driver.go`'s pre-`c0e6c4ce6` `NeedsApproval`
  polling loop resending `"1\r"` on every tick the stale preview buffer still
  showed the dialog,
- **Then** the duplication mechanism is confirmed with direct runtime
  evidence (the two merged commit messages `3546c2b12`/`c0e6c4ce6`
  explicitly cite issue #165 and this failure mode) — no further Phase 0
  gate work is required before Phase 1/2 implementation proceeds.

##### Task 1.1.1.1 — No action (documentation-only confirmation)

Nothing to implement; requirements.md already records this. Skip directly to
Story 1.1.2.

#### Story 1.1.2 — AC2: add a regression test tying the `awaitingClear` latch fix to this backlog item

`session/session_driver_test.go:165-183` already has `TestShouldApprovePromptOnce`
covering issue #165 at the pure-function level (3 isolated assertions on
`shouldApprovePromptOnce(approvalVisible, awaitingClear)`). It is not tied to
this backlog item and doesn't exercise the *stateful sequence* across many
polling ticks the way `session_driver.go:428-451`'s actual loop does
(`approvalAwaitingClear` threaded call-to-call).

**Corrected during plan repair (pre-mortem.md Failure #5, P2):** the original
version of this story's task hand-reimplemented the loop's state-threading
rule (`approvalAwaitingClear = approvalAwaitingClear && approvalVisible`)
*inline in the test* instead of driving the real production method that does
this threading — the same "tested the pattern, not the production code path"
gap Phase 5's `controlModeReadLoop` extraction was explicitly designed to
avoid, and the precise failure mode (AC3 marked done against code that didn't
do what was claimed) that already happened once on this exact ticket. No
directly-testable extraction point existed for this logic (it was inline in
`runSessionDriverWithPrompt`'s `NeedsApproval` block, lines 428-451), so this
story now adds one first, mirroring Phase 5's precedent exactly: pure
decision/state-threading logic extracted into a new function; the single
side-effecting `SendKeys` call stays a caller-supplied closure. The new test
then drives that real extracted function directly, not a hand-copied rule.

**AC2 Given-When-Then:**
- **Given** a simulated driver poll sequence for backlog item
  `04089969-0f19-499c-be34-2e8bcfc4f13e`: tick 1 `NeedsApproval` visible,
  `approvalAwaitingClear=false`; ticks 2–5 `NeedsApproval` still visible
  (simulating the PTY redraw lag the ticket's log lines show); tick 6 dialog
  no longer visible,
- **When** `processApprovalTick(previewErr, output, allowedPath,
  approvalAwaitingClear, sendKeys)` — Task 1.1.2.1's extracted production
  function, not a re-implementation of its rule — is called once per tick,
  threading each call's returned `awaitingClear` value into the next call's
  input exactly as `runSessionDriverWithPrompt`'s real loop does,
- **Then** the `sendKeys` closure passed to `processApprovalTick` is invoked
  exactly once across the 6 calls (tick 1) — asserting, through the actual
  production state-threading function, that the same-dialog resend which
  produced the original ticket's repeated `"1"` does not happen across the
  full 6-tick simulated flap.

##### Task 1.1.2.1 — Extract `processApprovalTick` from `runSessionDriverWithPrompt`'s inline `NeedsApproval` handling

*(Added during plan repair to close pre-mortem.md Failure #5, P2 — mirrors
the Phase 5 `controlModeReadLoop` extraction precedent in this same plan:
pure logic extracted into a directly-testable function; the side-effecting
I/O call stays a caller-supplied closure.)*

- File: `session/session_driver.go`.
- Extract the inline `NeedsApproval` handling currently at lines 430-451
  (inside `runSessionDriverWithPrompt`'s `if mgr := inst.GetStatusManager();
  mgr != nil { if si := mgr.GetStatus(inst); si.ClaudeStatus ==
  detection.StatusNeedsApproval { ... } }` block) into:
  ```go
  // processApprovalTick implements one poll tick's directory-access approval
  // handling (see #165) — the state-threading rule that decides whether to
  // approve a NeedsApproval prompt and how approvalAwaitingClear carries into
  // the next tick. sendKeys performs the actual side-effecting SendKeys call
  // and is caller-supplied so this function stays pure and directly testable
  // against a simulated tick sequence with a fake Preview() output, with no
  // live tmux session required — mirrors controlModeReadLoop's extraction in
  // Phase 5 (pure logic extracted, I/O stays a closure at the call site).
  func processApprovalTick(previewErr error, output string, allowedPath string, awaitingClear bool, sendKeys func() error) bool {
      if previewErr != nil || output == "" {
          return awaitingClear
      }
      approvalVisible := shouldApprovePrompt(output, allowedPath)
      if shouldApprovePromptOnce(approvalVisible, awaitingClear) {
          if err := sendKeys(); err != nil {
              return awaitingClear // unchanged on a failed send — matches original inline semantics
          }
          return true
      }
      return awaitingClear && approvalVisible
  }
  ```
- Update the call site to:
  ```go
  if mgr := inst.GetStatusManager(); mgr != nil {
      if si := mgr.GetStatus(inst); si.ClaudeStatus == detection.StatusNeedsApproval {
          approvalAwaitingClear = processApprovalTick(previewErr, output, allowedPath, approvalAwaitingClear, func() error {
              err := inst.SendKeys("1\r")
              if err != nil {
                  log.Warn("SessionDriver: failed to approve prompt", "session", inst.Title, "err", err)
              } else {
                  log.Info("SessionDriver: approved directory-access prompt", "session", inst.Title)
              }
              return err
          })
      }
  }
  ```
- This is a pure logic move: the approve decision, the `"1\r"` `SendKeys`
  call, both log lines, and the `awaitingClear` threading rule — including
  the failed-send case leaving `awaitingClear` untouched — are all preserved
  exactly; only the shape changes from inline to an extracted, directly-
  callable function.
- Run: `go build ./session/...` to confirm the extraction compiles cleanly.

##### Task 1.1.2.2 — Add `TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn` to `session/session_driver_test.go`, driving `processApprovalTick` directly

- File: `session/session_driver_test.go` (add after `TestShouldApprovePromptOnce`, ~line 184).
- Doc comment references backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e`,
  quotes the ticket's log excerpt, and notes this test drives Task 1.1.2.1's
  `processApprovalTick` directly rather than reimplementing its threading
  rule (per pre-mortem.md Failure #5, P2).
- Implements the 6-tick simulation above by calling `processApprovalTick`
  once per tick (package-visible in this test file — no new exported API),
  with a `sendKeys` stub that returns `nil` and records its invocation
  count, and threads each call's returned `awaitingClear` into the next
  call's `awaitingClear` argument.
- Fake `Preview()` output: ticks 1-5 use output text containing the
  NeedsApproval dialog (`approvalVisible == true` via `shouldApprovePrompt`);
  tick 6 uses output with the dialog text removed (`approvalVisible ==
  false`).
- Assert the `sendKeys` stub was invoked **exactly once**, on tick 1 —
  proving, through the real production state-threading function, that the
  same-dialog resend which produced the original ticket's repeated `"1"`
  does not happen across the full 6-tick simulated flap.
- Naming note (unchanged from the prior version of this task): the real
  production mechanism is `shouldSendOnce`, an edge-triggered latch keyed on
  `awaitingClear` (`session_driver.go:661-673`) — not a time-based cooldown.
  `session_driver.go`'s own doc comment explicitly disclaims one: "A fixed
  time-based cooldown is deliberately not used here." The test name and
  requirements.md's Root Cause section are corrected to match (see
  requirements.md diff).
- Run: `go test ./session/... -run TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn -v`

---

## Phase 2 — `MessageQueue.close()` Drop-on-Close Fix

Covers the `MessageQueue` half of **AC3**.

### Epic 2.1 — Fix `close()` to discard buffered-but-unsent messages

#### Story 2.1.1 — Clear the queue and report a drop count from `close()`

**AC3 (MessageQueue half) Given-When-Then:**
- **Given** a `MessageQueue` instance with two buffered `TerminalData`
  messages (`push({sessionId: "sess-abc", data: {case: "input", value: {data: "l"}}})`
  then `push({sessionId: "sess-abc", data: {case: "input", value: {data: "s"}}})`,
  neither yet drained by the async iterator),
- **When** `close()` is called,
- **Then** `close()` returns `2` (the drop count), `isClosed()` returns
  `true`, and iterating the queue with `for await (const msg of queue)`
  yields **zero** messages — neither `"l"` nor `"s"` is ever sent.

##### Task 2.1.1.1 — Edit `MessageQueue.close()` in `web-app/src/lib/terminal/MessageQueue.ts:55-63`

- Change signature to `close(): number`.
- Inside, before touching `this.resolve` (keeping the invariant "closed
  queues are always empty" enforced atomically, per pitfalls.md §1's
  "queue clearing must happen atomically with the closed flag" warning):
  ```ts
  close(): number {
    this.closed = true;
    const dropped = this.queue.length;
    this.queue = [];
    if (this.resolve) {
      this.resolve(create(TerminalDataSchema, { sessionId: "", data: { case: undefined } }));
      this.resolve = null;
    }
    return dropped;
  }
  ```
- Update the class-level doc comment (lines 1-18) to note `close()` now
  discards buffered input and returns the count dropped.

##### Task 2.1.1.2 — Rewrite the conflicting test in `web-app/src/lib/terminal/__tests__/MessageQueue.test.ts:34-51`

- Rename `'should yield messages in order'` (currently asserts the buggy
  "both messages still yielded after close()" behavior) to
  `'should drop buffered messages when close() is called before they are drained'`.
- New assertions: push 2 messages, call `close()`, assert `received` has
  length `0`, and assert `close()`'s return value is `2`.
- Do **not** touch `'push after close' → no-op` or `'should unblock a
  waiting iterator'` (lines 76-90, 108-122) — these already assert target
  semantics per stack.md's confirmation.

##### Task 2.1.1.3 — Add a same-tick push-during-close race test

- New test in the `describe('close', ...)` block:
  `'should not deliver a message pushed in the same synchronous tick close() runs'`
  — covers pitfalls.md §1's "resolve-callback races with close" warning
  (a `push()` landing between `closed = true` and the iterator's next check).
- Shape: start the async iterator (`iterPromise`), call `queue.close()`
  immediately followed (same microtask) by `queue.push(msg)`, await
  `iterPromise`, assert `received` is empty (the post-close `push()` is
  already a documented no-op per line 29's `if (this.closed) return;` guard —
  this test makes that guarantee explicit for the close-then-immediately-push
  ordering specifically, not just late/deferred pushes).
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="MessageQueue.test"`

---

## Phase 3 — `useTerminalStream.connect()` Connection-Epoch Guard

Covers the `useTerminalStream` half of **AC3** and the epoch-guard portion of
**AC4**.

### Epic 3.1 — Introduce `connectionEpochRef` and gate all state-mutating checkpoints

#### Story 3.1.1 — Add the epoch ref, gate the loop/catch/finally/`disconnect()` continuation, collapse the queue-close asymmetry

**AC3 (epoch guard half) Given-When-Then:**
- **Given** `connect()` is called for session `sess-flap-1` (attempt A);
  its entry guard (`isConnectedRef.current || isConnectingRef.current ||
  !sessionId`) passes, so attempt A proceeds past the guard and *only then*
  increments the epoch to `epoch = 1`,
- **When** attempt A's `for await` loop receives its first message —
  synchronously flipping `isConnectingRef.current` back to `false`
  (line 222) — and, before React's `isConnectedRef` ref-sync `useEffect`
  has had a chance to run off the queued `setIsConnected(true)`,
  `connect()` is called again for the same session (attempt B): attempt
  B's entry guard also passes in this window (`isConnectingRef.current`
  is now `false` and the stale `isConnectedRef.current` is still `false`),
  so attempt B proceeds past its own guard and increments the epoch to
  `epoch = 2`,
- **Then** attempt A's subsequent checkpoints (its `catch`/`finally`, and
  any further messages delivered on its own stream) see `epoch (1) !==
  connectionEpochRef.current (2)` and skip all
  `setIsConnected`/`setTerminalState`/reconnect-scheduling — only attempt
  B's outcome is ever reflected in `isConnected`/`terminalState`, and
  attempt B's `MessageQueue` (not attempt A's) is the one installed in
  `messageQueueRef.current` for all subsequent `sendInput` calls. A
  **guard-blocked** call — one where `isConnectedRef.current ||
  isConnectingRef.current` is still `true` at the moment it's attempted —
  is a distinct case, covered by Task 3.2.1.0: it must return before ever
  reaching the epoch increment, so it can never orphan the real in-flight
  attempt (pre-mortem.md Failure #1).

##### Task 3.1.1.1 — Declare `connectionEpochRef` and increment only after `connect()`'s entry guard passes

- File: `web-app/src/lib/hooks/useTerminalStream.ts`.
- Add `const connectionEpochRef = useRef(0);` near `isConnectingRef`
  (line 106), with a doc comment mirroring `useSessionService.ts:184`'s
  ("Monotonically-increasing... checked at every await checkpoint").
- At the top of `connect()` (line 162), leave the existing entry guard —
  `if (isConnectedRef.current || isConnectingRef.current || !sessionId)
  return;` — completely unchanged and untouched, and add
  `const epoch = ++connectionEpochRef.current;` as the **first statement
  after** that guard (i.e. only a call that the guard actually lets
  through ever executes this line — place it alongside/immediately after
  `isConnectingRef.current = true;`). This must **not** be placed before
  the guard: pre-mortem.md Failure #1 (P1) identified that incrementing
  before the guard means a guard-blocked call (one that hits the guard's
  `return` because `isConnectedRef.current || isConnectingRef.current` is
  already `true`) would still bump `connectionEpochRef.current` — orphaning
  the real in-flight attempt, since that attempt's own captured `epoch`
  would now permanently mismatch `connectionEpochRef.current` at every
  later checkpoint (`firstMessage`, `catch`, `finally`), even though no
  second real attempt ever started to complete the handoff. Placing the
  increment after the guard means only a call that actually proceeds past
  the guard ever consumes an epoch value. Still synchronous, still before
  any `await` within `connect()`'s own body, per pitfalls.md's
  "increment-point placement" warning — just after the guard rather than
  before it. This is the single increment (unlike `useSessionService.ts`'s
  double-increment shape — see ADR-001 for why `useTerminalStream.ts` only
  needs one, given its entry guard already fully prevents concurrent
  starts, unlike `watchSessions()` which can be called by multiple
  independent external callers).

##### Task 3.1.1.2 — Unconditionally close the previous `messageQueueRef.current` before installing a new one

- File: `web-app/src/lib/hooks/useTerminalStream.ts:185`.
- Replace `messageQueueRef.current = new MessageQueue();` with:
  ```ts
  const droppedCount = messageQueueRef.current?.close() ?? 0;
  if (droppedCount > 0) {
    console.warn(`[useTerminalStream] dropped ${droppedCount} buffered input message(s) on reconnect`, { sessionId });
  }
  messageQueueRef.current = new MessageQueue();
  ```
  This collapses the disconnect-path (`disconnect()` line 388, already calls
  `close()`) vs. implicit-reconnect-path (previously did *not* close the old
  queue) asymmetry named in architecture.md §1's "Resource-lifetime guard."
- Wire `droppedCount` into the drop-signal state added in Phase 4 (this task
  only performs the close/log; Phase 4 adds the `setDroppedInputEvent` call
  so Phase 3 stays scoped to the epoch/queue mechanics).

##### Task 3.1.1.3 — Gate the `for await` loop's `firstMessage` branch on `epoch`

- File: `web-app/src/lib/hooks/useTerminalStream.ts:220-227`.
- Wrap the `if (firstMessage) { ... }` block's body: before calling
  `setIsConnected(true)`/`setScrollbackLoaded(true)`/`setTerminalState('LOADING')`,
  add `if (epoch !== connectionEpochRef.current) return;` as the loop's
  first statement inside the `for await`, per architecture.md §1's
  "Iteration guard" — a stale generation must not mutate connection state
  even if it still receives a message.

##### Task 3.1.1.4 — Gate the `catch` and `finally` blocks on `epoch`

- File: `web-app/src/lib/hooks/useTerminalStream.ts:313-353`.
- In the `catch` block (line 313), before `handleError(err)`/hard-failure
  state mutations, add the same `if (epoch !== connectionEpochRef.current) return;`
  short-circuit — per pitfalls.md's explicit warning that guarding only the
  success path and letting a stale generation's error handler still run is
  the most common version of this bug.
- In the `finally` block (line 322), split into two parts: resource cleanup
  (decoder resets) always runs regardless of epoch (per architecture.md §1:
  "resource cleanup must happen regardless of which epoch is current"); the
  user-visible state mutation (`setIsConnected(false)`,
  `setTerminalState('DISCONNECTED')`, and the entire
  `NEXT_PUBLIC_RECONNECT_V2` reconnect-scheduling block, lines 331-352) is
  gated behind `if (epoch === connectionEpochRef.current) { ... }` — a stale
  generation's `finally` must not schedule a reconnect timer for an attempt
  that's already been superseded.

**`disconnect()` vs. in-flight `connect()` Given-When-Then** (both
architecture-review.md and adversarial-review.md, the latter also noting this
exact gap was raised and left unresolved in this codebase's own prior
unmerged adversarial review — three independent findings, treated as strong
signal a code-level guard is needed, not just a documented justification):
- **Given** `disconnect()` is called for session `sess-flap-1` while no
  reconnect is currently scheduled (`connectionEpochRef.current` is `3`,
  captured by `disconnect()` at entry as `epochAtDisconnectStart = 3`), and
  `disconnect()` reaches its `await new Promise(...)` at line 392,
- **When**, before that `await` resolves, an independent `connect()` call
  (e.g. the visibility/online listener firing, or a manual-reconnect action)
  starts and completes far enough to increment `connectionEpochRef.current`
  to `4` and call `setIsConnected(true)`,
- **Then** `disconnect()`'s post-`await` continuation (line 409-412) detects
  `epochAtDisconnectStart (3) !== connectionEpochRef.current (4)` and skips
  `setIsConnected(false)` and the decoder resets — it must not silently flip
  a freshly-(re)connected UI back to "disconnected" or corrupt the new
  connection's in-flight streaming-decoder state — while still always
  resetting `isDisconnectingRef.current = false` (bookkeeping, not
  connection state, so it is not gated).

##### Task 3.1.1.5 — Gate `disconnect()`'s post-`await` continuation on `connectionEpochRef`

- File: `web-app/src/lib/hooks/useTerminalStream.ts:371-413`.
- At the top of `disconnect()` (before the `shouldReconnectRef.current =
  false` line, so an early-returning call still captures a value — mirroring
  Task 3.1.1.1's increment-before-guard placement discipline), add
  `const epochAtDisconnectStart = connectionEpochRef.current;` — a read-only
  capture, **not** an increment; `disconnect()` isn't starting a new attempt,
  it's checking whether one has started since.
- After the `await new Promise<void>((resolve) => { ... })` block (line
  392-407), split the post-await continuation:
  ```ts
  isDisconnectingRef.current = false; // bookkeeping — always reset regardless of epoch

  if (epochAtDisconnectStart === connectionEpochRef.current) {
    setIsConnected(false);
    textDecoderRef.current = new TextDecoder();
    scrollbackDecoderRef.current = new TextDecoder();
  }
  ```
  Connection-state mutation and decoder resets are gated (a newer `connect()`
  already owns both); `isDisconnectingRef.current`'s reset always runs so a
  future `disconnect()` call is never permanently blocked by a stale
  in-progress flag.
- Update ADR-001 with a short addendum documenting that `disconnect()`
  participates in the same epoch counter as a reader (never an incrementer)
  — see `decisions/ADR-001-terminal-stream-single-increment-epoch-guard.md`.

---

### Epic 3.2 — Jest regression coverage for the epoch guard

#### Story 3.2.1 — Overlapping-connect, triple-rapid-connect, and drop-on-close interleaving tests

**AC4 (Jest epoch-guard portion) Given-When-Then:**
- **Given** `useTerminalStream({ sessionId: 'sess-race-1', autoConnect: false })`
  rendered via `renderHook`, with `mockStreamTerminal` configured to return a
  fresh `makePushStream()` on each call,
- **When** the test calls `result.current.connect()` (attempt A), pushes a
  first message on attempt A's stream so its entry guard's
  `isConnectingRef.current` flips back to `false` (production behavior,
  line 222) without an intervening render flush of the `isConnectedRef`
  ref-sync effect, then — while `isConnectedRef.current` is still
  stale-`false` — calls `result.current.connect()` again (attempt B, whose
  entry guard now also passes) and pushes a first message onto *attempt
  B's* stream,
- **Then** `result.current.isConnected` ends up reflecting only attempt
  B's outcome — attempt A's `MessageQueue` instance was `.close()`-d
  (Task 3.1.1.2) and its later checkpoints are no-ops — proving the first
  (superseded) attempt's post-supersession state mutations were skipped. A
  **separate** guard-blocked-call test (Task 3.2.1.0) proves the converse:
  a `connect()` call that the entry guard actually blocks (called while
  `isConnectingRef.current` is still `true` for a still-in-flight attempt)
  must **not** consume an epoch or orphan that in-flight attempt — see
  pre-mortem.md Failure #1 (P1).

##### Task 3.2.1.0 — `connect_should_notOrphanInFlightAttempt_When_secondCallIsBlockedByEntryGuard`

*(Required by pre-mortem.md Failure #1 (P1): write and pass this test as
direct proof that Task 3.1.1.1's increment-after-guard placement is
correct, before relying on any other epoch-guard test in this story.)*

- File: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`, new
  `describe('useTerminalStream — connection epoch guard', ...)` block.
- Configure `mockStreamTerminal` so attempt A's returned stream never
  yields a message during the test (i.e. attempt A is deliberately left
  genuinely CONNECTING — `isConnectingRef.current === true` — for the
  test's duration).
- Call `connect()` once (attempt A, left in-flight). While attempt A is
  still in-flight, call `connect()` a second time — this call must be
  blocked by the existing entry guard.
- Assert:
  - (a) `mockStreamTerminal` was invoked **exactly once**, not twice — the
    guard-blocked second call never reached the point that opens a stream
    (and therefore never reached the epoch increment either).
  - (b) After then pushing a message on attempt A's (only) stream, attempt
    A's `firstMessage` branch still runs normally and `isConnected`
    becomes `true` — proving the guard-blocked call did not bump
    `connectionEpochRef.current` out from under attempt A. If it had,
    attempt A's checkpoint would see `epoch !== connectionEpochRef.current`
    and silently no-op forever, leaving `isConnected` stuck `false`.

##### Task 3.2.1.1 — `connect_should_ignoreStaleGenerationMessages_When_secondConnectSupersedesFirstAfterFirstMessageRefSyncLag`

- Same describe block as Task 3.2.1.0.
- Enhance the module-level `MessageQueue` mock (lines 57-65) to track
  `close` calls with a per-instance id (e.g. `static instanceCount = 0`,
  each instance records its own id and whether `.close()` was invoked on
  it) so the test can assert the first instance was `.close()`-d and the
  second instance is the one `sendInput` targets.
- Configure `mockStreamTerminal` to return `pushStreamA.iterable` on call 1
  and `pushStreamB.iterable` on call 2.
- Call `connect()` (attempt A), push a message on `pushStreamA` so attempt
  A's own entry-guard flags flip (`isConnectingRef.current` back to
  `false`) without flushing the `isConnectedRef` ref-sync effect in
  between, then call `connect()` again (attempt B — its entry guard passes
  in this window) and push a message on `pushStreamB`. Assert `isConnected`
  reflects only attempt B's message, and that `pushStreamA`'s
  `MessageQueue` instance was `.close()`-d.

##### Task 3.2.1.2 — `connect_should_notThrow_When_threeAttemptsSupersedeAcrossRepeatedRefSyncLagWindows`

- Same describe block. Repeats Task 3.2.1.1's supersession mechanism twice
  in a row — call `connect()` (A), push A's first message (opens the
  guard), call `connect()` (B) in the lag window, push B's first message
  (re-opens the guard), call `connect()` (C) in *that* lag window — with
  three distinct `mockStreamTerminal` return values (A, B, C). Asserts no
  exception is thrown/no unhandled promise rejection, and that only the
  *third* (`C`) call's `firstMessage` results in `isConnected === true` —
  covers pitfalls.md's explicit "must decide whether triple-reconnect
  should also proactively close superseded generations" question (answered:
  yes, via Task 3.1.1.2's unconditional `close()`, verified here by
  asserting instances A and B were both closed).

##### Task 3.2.1.3 — `useTerminalStream_should_dropQueuedInput_When_reconnectSupersedesQueueWithPendingMessages`

- Same describe block (hook-level, per pitfalls.md §5's explicit "must
  exercise the hook, not just the queue class" requirement — this
  complements, does not replace, Phase 2's queue-level tests).
- Sequence: `connect()` (attempt A, gets `messageQueueRef` instance #1) →
  push attempt A's first message so its own entry-guard flags open the
  ref-sync lag window (per Task 3.2.1.1's mechanism) → simulate a
  queued-but-undelivered push on instance #1 (via the mock's `push` spy
  recording a call) → call `connect()` again (attempt B, whose entry guard
  passes in that window) before attempt A's instance #1 queue has been
  drained → assert instance #1's `.close` was called (Task 3.1.1.2's
  unconditional close) and instance #2 is the one installed — proving
  queued input from a superseded attempt is discarded, not carried forward
  into the new queue.

##### Task 3.2.1.4 — `disconnect_should_notClobberNewerConnect_When_reconnectCompletesWhileDisconnectStillAwaiting`

- Same describe block. Covers Task 3.1.1.5's `disconnect()`-vs-`connect()`
  interleaving guard (both architecture-review.md and adversarial-review.md
  flagged this gap; not covered by Tasks 3.2.1.1-3.2.1.3, which are all
  connect()-vs-connect()).
- Sequence: render the hook with `autoConnect: false`; call `connect()`
  (attempt A) and let it reach `isConnected === true` (push a message on
  `pushStreamA`); call `disconnect()` but do **not** yet let its internal
  `await new Promise(...)` resolve (mock `setTimeout`/use fake timers so the
  test controls when it resolves); while `disconnect()`'s promise is still
  pending, call `connect()` again (attempt B) and let it also reach
  `isConnected === true` (push a message on `pushStreamB`); only then let
  `disconnect()`'s pending promise resolve.
- Assert: after `disconnect()`'s continuation runs, `isConnected` is still
  `true` (attempt B's state was not clobbered back to `false`), and the
  decoder state was not reset a second time out from under attempt B —
  proving `disconnect()`'s stale continuation recognized it had been
  superseded and skipped its state mutations per
  `epochAtDisconnectStart !== connectionEpochRef.current`.
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalStream.test"`

---

## Phase 4 — Drop-and-Signal UI

Covers the drop-and-signal portion of **AC3**.

### Epic 4.1 — Surface `droppedInputEvent` from `useTerminalStream`

#### Story 4.1.1 — Wire drop detection from both silent-drop points into hook state

**AC3 (signal half) Given-When-Then:**
- **Given** `useTerminalStream` connected for session `sess-flap-1`, then the
  connection is superseded by a reconnect while 3 `TerminalData` input
  messages are still buffered in the old `MessageQueue`,
- **When** `connect()`'s Task 3.1.1.2 close-before-install runs and
  `close()` returns `3`,
- **Then** the hook's returned `droppedInputEvent` becomes
  `{ count: 3, at: <timestamp> }` on the next render, distinct from `null`
  (its value on a clean disconnect with an empty queue — the "silent on the
  normal case" constraint). This is a **single reported occurrence** — the
  hook performs no accumulation across *distinct* occurrences (that is
  `InputDropBadge`'s job, Epic 4.2); the one exception is same-React-batch
  merging (Task 4.1.1.3), which exists only to prevent two occurrences that
  land in the same synchronous batch from silently overwriting each other —
  see that task for why this is not the same thing as `InputDropBadge`'s
  cross-render coalescing.

##### Task 4.1.1.1 — Add `droppedInputEvent` state and a `reportDrop` helper, wired from Task 3.1.1.2's close call

- File: `web-app/src/lib/hooks/useTerminalStream.ts`.
- Add a module-level type and smart constructor (per the Domain Glossary /
  Pattern Decisions "Illegal-state prevention for drop reports" entry),
  above the hook definition:
  ```ts
  type DroppedInputEvent = { count: number; at: number };

  /** Reports one drop occurrence. Returns null for count <= 0 so a
   *  domain-illegal "0 keystrokes not sent" event can never be constructed. */
  function recordDrop(count: number): DroppedInputEvent | null {
    return count > 0 ? { count, at: Date.now() } : null;
  }
  ```
- Add `const [droppedInputEvent, setDroppedInputEvent] = useState<DroppedInputEvent | null>(null);`
  near the other state declarations (line ~96).
- Add a `reportDrop` helper inside the hook (near `droppedInputEvent`'s state
  declaration) — **all three** drop call sites (this task's reconnect-close,
  `disconnect()`'s close, and Task 4.1.1.2's `onDrop`) call `reportDrop`
  instead of calling `setDroppedInputEvent` directly:
  ```ts
  // Same-React-batch merge guard (Task 4.1.1.3 / pre-mortem.md Failure #3,
  // P2): the three drop call sites below can fire within the same
  // synchronous React 18 batch (e.g. a reconnect's queue-close and a
  // rejected keystroke landing in the same tick). Two setState calls to the
  // same state in one batch mean only the *last* value survives to the next
  // render — a naive `setDroppedInputEvent(dropEvent)` at each call site
  // would silently drop the earlier occurrence's count. dropBatchRef
  // accumulates same-batch occurrences; the effect below resets it once the
  // batch's single resulting render has committed, so the *next* distinct
  // occurrence starts its own count rather than accumulating onto a stale one.
  const dropBatchRef = useRef<DroppedInputEvent | null>(null);

  function reportDrop(count: number) {
    const event = recordDrop(count);
    if (!event) return;
    const merged = dropBatchRef.current
      ? { count: dropBatchRef.current.count + event.count, at: event.at }
      : event;
    dropBatchRef.current = merged;
    setDroppedInputEvent(merged);
  }

  useEffect(() => {
    // Runs after the batch that produced this droppedInputEvent has
    // committed — safe to reset here because any same-batch reportDrop()
    // calls have already run and merged synchronously before this effect
    // fires. Skips the `droppedInputEvent === null` mount case (nothing to
    // reset).
    if (droppedInputEvent !== null) {
      dropBatchRef.current = null;
    }
  }, [droppedInputEvent]);
  ```
- In Task 3.1.1.2's block, replace the bare `console.warn` with:
  ```ts
  if (droppedCount > 0) {
    reportDrop(droppedCount);
    console.warn(`[useTerminalStream] dropped ${droppedCount} buffered input message(s) on reconnect`, { sessionId });
  }
  ```
- Also apply the identical `reportDrop(...)`-based pattern inside
  `disconnect()` (`useTerminalStream.ts:387-390`) where
  `messageQueueRef.current.close()` is already called — an explicit
  user-initiated disconnect with pending input must fire the same signal.
- Add `droppedInputEvent` to the returned `TerminalStreamResult` object
  (line ~469) and its interface (line ~54), typed as `DroppedInputEvent | null`.
- Do **not** attempt to accumulate a running count across *distinct*
  occurrences at this layer (e.g. no cross-render running-total logic) —
  each call site reports its own single occurrence via `reportDrop`;
  `InputDropBadge` (Story 4.2.1) still owns turning a *sequence of separate*
  occurrences (across renders/batches) into a running total and a coalesced
  announcement. `reportDrop`'s merge only fires within a single synchronous
  batch, which is a correctness fix (preventing silent undercounting), not a
  substitute for `InputDropBadge`'s cross-render coalescing.

##### Task 4.1.1.2 — Wire `useTerminalFlowControl.sendInput`'s pre-existing silent drop into the same signal

*(Resolved in-scope during plan repair — see Unresolved Questions' "Resolved"
note. requirements.md's Remaining confirmed gap / AC3 describe silent input
loss around a disconnect boundary generally, not only loss that reaches
`MessageQueue` before being discarded; a keystroke rejected at the
flow-control layer because the hook already knows it's disconnected is the
same class of loss.)*
- File: `web-app/src/lib/hooks/useTerminalFlowControl.ts:142-143`.
- Add an `onDrop?: () => void` option to `UseTerminalFlowControlOptions`;
  call it in `sendInput`'s early return (`if (!pushMessageRef.current ||
  !isConnectedRef.current) { onDrop?.(); return; }`).
- File: `web-app/src/lib/hooks/useTerminalStream.ts` — pass
  ```ts
  onDrop: () => reportDrop(1)
  ```
  into the `useTerminalFlowControl({...})` call (line 144) — a single
  reported occurrence via the same `reportDrop` helper used by Task 4.1.1.1
  (which also absorbs this call site into the same-batch merge guard — this
  call site is one of the two most likely to race with the reconnect-close
  call site in the same tick, e.g. a keystroke rejected at the exact moment
  a reconnect supersedes the connection), no inline accumulation logic (the
  previous inline `prev?.at === Date.now() ? prev.count : 0` attempt
  compared two different `Date.now()` calls and effectively never matched —
  deleted entirely per architecture-review.md, not fixed in place).
- Note (per adversarial-review.md's nitpick): `useTerminalFlowControl.ts` has
  six near-identical `if (!pushMessageRef.current || !isConnectedRef.current)
  return;` guards (lines 143, 171, 198, 234, 252, 294, 319 per grep). Only
  the `sendInput` occurrence (line 143, keystroke input) is wired to
  `onDrop` here — the other five guard resize/scrollback requests, not
  keystrokes, and are deliberately left unwired as out of scope for this
  "phantom keystroke" ticket. A future reader should not read this as an
  incomplete fix.

##### Task 4.1.1.3 — Jest test: two same-batch `reportDrop` calls merge into one combined occurrence

*(Added during plan repair — pre-mortem.md Failure #3, P2: `InputDropBadge`'s
coalescing assumes one `setDroppedInputEvent` call per React render, but the
three independent drop call sites can fire within the same React 18 batch;
without Task 4.1.1.1's `reportDrop`/`dropBatchRef` merge guard, only the last
value would survive to the next render, silently undercounting. This test
proves the guard works at the hook level — `InputDropBadge`'s own coalescing
tests, Task 4.2.3.2, only ever drive it via sequential controlled re-renders
and would not have caught this.)*

- File: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`, same
  `describe('useTerminalStream — connection epoch guard', ...)` block is not
  appropriate here (this isn't an epoch scenario) — add a new
  `describe('useTerminalStream — drop reporting', ...)` block.
- Test: render the hook (`autoConnect: false`), `connect()` and reach
  `isConnected === true`. Inside a **single** `act(() => { ... })` block,
  synchronously trigger two of the hook's drop call sites — e.g. call the
  test's captured `onDrop` callback (simulating `useTerminalFlowControl`'s
  Task 4.1.1.2 call site) and then, in the same synchronous callback,
  directly invoke a reconnect that runs Task 3.1.1.2's close-before-install
  path with a queue that has 2 buffered messages — so both `reportDrop` calls
  execute before React flushes any render.
- Assert: after the `act()` block, `result.current.droppedInputEvent`
  reflects the **combined** count of both occurrences (`1` from the flow-
  control drop + `2` from the reconnect-close drop = `3`), not just the
  last-called site's own count.
- Assert (regression guard for the merge-reset half): a **subsequent**,
  separately-timed `reportDrop` call (outside any `act()` batch shared with
  the first two, e.g. in the next `act()` block after a render has
  committed) produces a **fresh** `droppedInputEvent` reflecting only its
  own count — proving `dropBatchRef` was reset after the first batch's
  commit and the merge guard does not accumulate indefinitely across
  unrelated occurrences.
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="useTerminalStream.test"`

---

### Epic 4.2 — `InputDropBadge` component

#### Story 4.2.1 — Build the component, following `GitHubBadge.tsx`'s structural template

`InputDropBadge` owns **all** coalescing across drop occurrences — both the
visual running-total pill and triggering the assertive announcement itself
(per architecture-review.md's remediation: `useTerminalStream` and
`TerminalOutput.tsx` must not compute a running total or call `announce()`).

##### Task 4.2.1.1 — Create `web-app/src/components/sessions/InputDropBadge.tsx`

- Props: `{ droppedInputEvent: DroppedInputEvent | null }` (import the type
  from `useTerminalStream.ts`, or a shared location if one already exists
  for hook-adjacent types).
- Structural template: `GitHubBadge.tsx` — plain function component, renders
  `null` when `droppedInputEvent === null` (matches `GitHubBadge`'s
  `if (!hasPR && !hasRepo) return null;` idiom), `title`/`aria-label` built
  from state.
- Internal state: `useRef<ReturnType<typeof setTimeout> | null>(null)` for
  the visual dwell/fade timer (4000ms, per Unresolved Question 3) and a
  `useRef<number>(0)` running total for the current coalescing episode,
  reset when a new `droppedInputEvent` arrives after the previous badge has
  fully faded (`at` gap larger than the dwell window).
- **Announcement ownership**: call `const { message, announce } =
  useLiveRegion();` inside this component. Whenever a new
  `droppedInputEvent` prop arrives while the badge is already visible (i.e.
  the dwell timer has not yet fired), update the running-total ref, update
  the pill's rendered text, reset the dwell timer to a fresh 4000ms, **and**
  call `announce(...)` once with the updated running-total string (e.g.
  `"3 keystrokes not sent — connection interrupted"`) — this is the single
  point that fires an announcement; nothing outside this component calls
  `announce()`. When a new `droppedInputEvent` starts a fresh episode (badge
  was not visible / prior episode's dwell timer had already fired), the
  running total resets to that event's own `count` before rendering +
  announcing.
- Render, alongside the visible pill, this component's own (visually
  hidden, `srOnly`) `<LiveRegion message={message} politeness="assertive"
  role="alert" />` instance — the `LiveRegion` DOM node lives inside
  `InputDropBadge`, not in `TerminalOutput.tsx` (see Task 4.2.2.2).
- Renders a small pill: icon (reuse a simple "no-entry"/circle-slash inline
  SVG, `aria-hidden="true"`), text `"N keystroke(s) not sent"`, no color-only
  signal (pairs icon + text per `.claude/rules` a11y convention already used
  by `MemoryPressureCallout`).
- Uses `zIndex.floatingTerminalUI` (`theme-contract.css.ts:215`, already the
  named slot for terminal-anchored floating UI — no new slot needed).
- No focus-stealing (per ux.md §3 — no `autoFocus`, no `tabIndex` on mount).
- **Unmount safety (ux.md AC-RESOLVE-2):** the dwell-timer `useEffect` must
  return a cleanup function that calls `clearTimeout` on the pending timer
  ref — if the component unmounts while a timer is pending (e.g. the
  terminal closes mid-flap), the timer must not fire a `setState` call
  against an unmounted component. Covered by Task 4.2.3.1's unmount test.

##### Task 4.2.1.2 — Create `web-app/src/components/sessions/InputDropBadge.css.ts`

- Vanilla-extract, per `.claude/rules/css-architecture.md`. Structural
  template: `GitHubBadge.css.ts` (`style()` calls importing `vars` from
  `@/styles/theme-contract.css`, no hardcoded hex, no `var(--...)` strings).
- Variants: default pill style using `vars.color.warning`/`vars.color.textPrimary`
  (a drop is a warning-severity event, not a hard error) — no bespoke color.

#### Story 4.2.2 — Mount the badge in `TerminalOutput.tsx`

**AC3 (mount/wiring) Given-When-Then:**
- **Given** `TerminalOutput` rendered for `sessionId = "sess-flap-1"` with an
  active `useTerminalStream` connection,
- **When** the hook's `droppedInputEvent` transitions from `null` to
  `{ count: 3, at: 1700000000000 }`,
- **Then** `InputDropBadge` renders visibly inside `styles.terminal`
  (alongside the existing `reconnectingBanner`/`hardFailedBanner` overlays,
  `TerminalOutput.tsx:1652-1662`) **and**, from inside `InputDropBadge`
  itself (not from any `TerminalOutput.tsx`-level effect), a `LiveRegion`
  with `politeness="assertive"` `role="alert"` announces `"3 keystrokes not
  sent — connection interrupted"` exactly once for this coalesced episode —
  `TerminalOutput.tsx` itself contains no `announce()`/`LiveRegion` wiring
  of its own for this feature.

##### Task 4.2.2.1 — Extend `LiveRegion` with an optional `role` prop

- File: `web-app/src/components/ui/LiveRegion.tsx`.
- Add `role?: "status" | "alert"` prop, default `"status"` (backward
  compatible — no existing real consumer to break, confirmed via grep).
- Pass through to the rendered `<div role={role} ...>` (line 22).

##### Task 4.2.2.2 — Mount `InputDropBadge` in `TerminalOutput.tsx`

- File: `web-app/src/components/sessions/TerminalOutput.tsx`.
- Destructure `droppedInputEvent` from the `useTerminalStream(...)` call
  (line 456).
- Render `<InputDropBadge droppedInputEvent={droppedInputEvent} />` inside
  `styles.terminal` (near line 1652, alongside the reconnect banners).
- Do **not** add a `useLiveRegion()`/`announce()` call or a separate
  `<LiveRegion>` element at this layer — per Story 4.2.1, `InputDropBadge`
  owns its own announcement internally. (This replaces the plan's earlier,
  buggy design of a `TerminalOutput`-level `useEffect` keyed on
  `droppedInputEvent?.at` calling `announce()` on every raw event — that
  design fired one assertive interruption per drop during a flapping
  episode instead of coalescing, contradicting research/ux.md §4; see
  architecture-review.md and adversarial-review.md.)

#### Story 4.2.3 — Jest tests for `InputDropBadge`

##### Task 4.2.3.1 — `InputDropBadge.test.tsx`

- File: `web-app/src/components/sessions/InputDropBadge.test.tsx`.
- Tests: `renders null when droppedInputEvent is null`; `renders a pill with
  the drop count when droppedInputEvent is set`; `has no color-only signal
  (icon + text both present)`; `does not set focus/tabIndex on mount`
  (queries `document.activeElement` is unchanged after render).
- Unmount-safety test (ux.md AC-RESOLVE-2): render with a `droppedInputEvent`
  set (dwell timer pending), `unmount()` before the 4000ms timer fires,
  advance fake timers past 4000ms, assert no React `act()`/"state update on
  an unmounted component" warning is logged (spy on `console.error`) — proves
  the dwell-timer `useEffect`'s cleanup function actually clears the timer.
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="InputDropBadge.test"`

##### Task 4.2.3.2 — Coalescing and assertive-announcement assertions

*(Rewritten during plan repair — the original scope asserted that a second
`droppedInputEvent` with a new `at` "still produces a new announcement,"
i.e. it tested that per-event spam was the correct behavior. Both
architecture-review.md and adversarial-review.md flagged this as locking in
the exact "spam risk" research/ux.md §4 explicitly warns against. Rewritten
to assert coalescing instead.)*
- File: `web-app/src/components/sessions/InputDropBadge.test.tsx` (same file
  as Task 4.2.3.1, since coalescing/announcement now lives entirely inside
  `InputDropBadge`).
- Test: render with a first `droppedInputEvent={{ count: 1, at: t0 }}`;
  assert one assertive announcement fired with text containing `"1
  keystroke"`. Re-render (props update, simulating a second occurrence
  arriving while the badge is still visible, `at: t0 + 800`) with
  `droppedInputEvent={{ count: 2, at: t0 + 800 }}`; assert the pill's
  rendered text now shows the **accumulated** total (`"3 keystrokes"`, i.e.
  `1 + 2`), and a **second** announcement fired containing `"3 keystrokes"`
  — not `"2 keystrokes"` (the raw latest-event count) and not a repeat of
  `"1 keystroke"`. Re-render again with a third occurrence (`count: 1`) and
  assert the total becomes `4` and a third announcement contains `"4
  keystrokes"`.
- Test: assert the **number of announcements fired** across N rapid
  `droppedInputEvent` prop updates within the dwell window equals N (one per
  content change), never fewer (proves it isn't silently dropping updates)
  and never more (proves no per-byte/per-render spam) — bounding the
  announcement count to actual content changes, per research/ux.md §4 and
  AC-SR-3.
- Test: a `role="alert"` element with `aria-live="assertive"` and
  `aria-atomic="true"` is present whenever `droppedInputEvent` is non-null
  (covers the DOM-shape assertion the original task also intended).
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="InputDropBadge.test"`

---

## Phase 5 — Go: Bounded Read-Goroutine Exit Test

Covers the Go portion of **AC4**.

### Epic 5.1 — Extract a testable read loop and prove bounded exit

#### Story 5.1.1 — Extract `controlModeReadLoop`, instrument with `readWG`

**AC4 (Go bounded-exit) Given-When-Then:**
- **Given** a real (loopback, tmux-free) WebSocket connection pair created
  via `createTestWebSocketPair(t)` (`connectrpc_websocket_test.go:20`), and
  `controlModeReadLoop` running in a goroutine tracked by
  `readWG.Add(1)`/`defer readWG.Done()`,
- **When** the test calls `clientConn.Close()` (simulating
  `HandleWebSocket`'s `defer conn.Close()`, `connectrpc_websocket.go:348`,
  which is what actually unblocks a live deployment's Goroutine 2),
- **Then** `waitWithTimeout(&readWG, 2*time.Second)` returns `true` — the
  read loop observably exits within the bound, not indefinitely blocked in
  `ReadMessage()`.

##### Task 5.1.1.1 — Extract `controlModeReadLoop` in `server/services/connectrpc_websocket.go`

- Replace lines 947-1087 (Goroutine 2's full body, "Read from WebSocket and
  handle input/commands") with a call to a new method:
  ```go
  func (h *ConnectRPCWebSocketHandler) controlModeReadLoop(
      conn *websocket.Conn,
      sessionID string,
      doneChan <-chan struct{},
      errChan chan<- error,
      handleEnvelope func(envelope *protocol.Envelope),
  ) {
      for {
          select {
          case <-doneChan:
              return
          default:
              _, message, err := conn.ReadMessage()
              if err != nil {
                  if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
                      errChan <- nil
                  } else {
                      log.Error("[streamViaControlMode] WebSocket read error", "session", sessionID, "err", err)
                      errChan <- err
                  }
                  return
              }
              envelope, _, err := protocol.ParseEnvelope(message)
              if err != nil {
                  log.Error("[streamViaControlMode] failed to parse envelope", "err", err)
                  continue
              }
              if envelope.Flags&protocol.EndStreamFlag != 0 {
                  errChan <- nil
                  return
              }
              if len(envelope.Data) == 0 {
                  continue
              }
              handleEnvelope(envelope)
          }
      }
  }
  ```
- This is a pure logic move: `envelope`/error classification is unchanged
  (matches original lines 954-981 exactly); only the input/resize/scrollback
  business logic (original lines 983-1084) becomes the `handleEnvelope`
  closure body at the call site, still closing over `instance`, `snap`,
  `tmuxSessionName`, `resizeCh` exactly as before — no new param threading
  needed for those.

##### Task 5.1.1.2 — Wire `readWG` at the `streamViaControlMode` call site

- File: `server/services/connectrpc_websocket.go`, near line 746 (alongside
  `errChan`/`doneChan` declarations): add `var readWG sync.WaitGroup`.
- Replace the `go func() { ... }()` at line 948 with:
  ```go
  readWG.Add(1)
  go func() {
      defer readWG.Done()
      h.controlModeReadLoop(stream.conn, sessionID, doneChan, errChan, func(envelope *protocol.Envelope) {
          var incomingData sessionv1.TerminalData
          if err := proto.Unmarshal(envelope.Data, &incomingData); err != nil {
              log.Error("[streamViaControlMode] failed to unmarshal TerminalData", "err", err)
              return
          }
          // ... unchanged input/resize/scrollback handling (original lines 990-1084) ...
      })
  }()
  ```
- No behavior change to the final `select { case err := <-errChan: return
  err; case <-doneChan: return nil }` (lines 1091-1096) — per Pattern
  Decisions, deliberately **not** adding a `readWG.Wait()` here (would
  deadlock against the caller's later `conn.Close()`).

#### Story 5.1.2 — Regression test

##### Task 5.1.2.1 — `TestControlModeReadLoop_BoundedExitOnConnClose` in `connectrpc_websocket_test.go`

- Subtest 1, `"exits within bound when underlying connection closes"`:
  build `serverStream, clientConn, cleanup := createTestWebSocketPair(t)`;
  start `controlModeReadLoop` in a goroutine wrapped in `readWG`; call
  `clientConn.Close()`; assert `waitWithTimeout(&readWG, 2*time.Second)` is
  `true`.
- Subtest 2, `"exits immediately on EndStream envelope without closing the connection"`:
  same setup; from the test, write an `EndStreamFlag`-flagged envelope via
  `clientConn.WriteMessage(...)` (using `protocol.CreateEnvelope`, matching
  `readEnvelopeFromClient`'s existing helper style at
  `connectrpc_websocket_test.go:52`); assert `waitWithTimeout` returns
  `true` **and** the value received on `errChan` is `nil`.
- Doc comment on the test function references backlog item
  `04089969-0f19-499c-be34-2e8bcfc4f13e` and AC4.
- Run: `go test ./server/services/... -run TestControlModeReadLoop_BoundedExitOnConnClose -race -v`

##### Task 5.1.2.2 — Re-run the existing `connectrpc_websocket_test.go` suite to confirm zero regressions from the extraction

- Task 5.1.1.1's extraction is a refactor of live production code (envelope
  parse/EndStream/error classification moved verbatim into
  `controlModeReadLoop`, per the Pattern Decisions table's "pure logic move"
  claim) — that claim must be verified by actually running the pre-existing
  27 tests in this file, not just the new test above (per
  adversarial-review.md: nothing in the original Phase 5 scope directed this
  explicitly, only implied by a later `make test`/`make ci`).
- Run: `go test ./server/services/... -run TestStreamViaControlMode -race -v`
  (or the equivalent invocation covering the full existing test set in
  `connectrpc_websocket_test.go` — confirm the exact test-name pattern that
  matches all 27 pre-existing tests before running, since some may not share
  the `TestStreamViaControlMode` prefix).
- All 27 must pass unchanged; any failure indicates the extraction altered
  behavior, not just structure, and must be fixed before Phase 5 is
  considered complete.

---

## Phase 6 — Manual Verification (AC5)

### Epic 6.1 — Reproduce the ticket's exact flapping condition and confirm no phantom keystrokes

#### Story 6.1.1 — Manual repro via browser-automation-driven real keystrokes alternated with `pause_session`/`resume_session`

*(Corrected during this plan-repair pass — see pre-mortem.md Failure #2, P1.
An earlier repair attempt on this task used MCP `write_to_session`/
`send_control` calls timed tight against `pause_session` for the "typing"
step. That was itself a mistake: an MCP-level pty write bypasses the
browser's WebSocket/`MessageQueue`/epoch-guard path entirely and proves
nothing about whether the actual client-side fix (Phases 2-4 of this plan)
works — it only exercises tmux/`pause_session` server behavior. This story
now requires real browser-automation-driven keystrokes into the live DOM
terminal, per this repo's `CLAUDE.md` "Manual/interactive testing without
touching the live deployed instance" pattern and the `claude-in-chrome` MCP
tools.)*

**AC5 Given-When-Then:**
- **Given** a live tmux-backed session created via
  `mcp__stapler-squad__create_session` (attaches a real `SessionDriver`,
  unlike omnibar-created sessions — avoiding the `dca931a04`
  wrong-session-creation-path mistake this ticket has hit before) on a
  throwaway manual-test instance, with its terminal open in a real Chrome
  tab navigated to and controlled via `claude-in-chrome` browser-automation
  tools,
- **When**, for each of 5-10 cycles, browser automation clicks into the live
  xterm terminal DOM element and types a distinguishable counted marker
  string (e.g. `cycle-01-marker`, `cycle-02-marker`, ... — a different,
  greppable token per cycle so a duplicate/replayed occurrence is
  unambiguous) via real keyboard events, with `mcp__stapler-squad__pause_session`
  called as tightly as possible around/during that typing (immediately
  followed by `mcp__stapler-squad__resume_session`) — this is a **best-
  effort adversarial** attempt to land the browser-driven keystrokes
  in-flight or just-queued at the moment the pause takes effect, not
  guaranteed simultaneity, since a single sequential agent cannot make
  typing and the MCP pause call literally simultaneous — driving both
  `session/instance_tmux.go:471-472`'s `!i.started.Load() || i.Status ==
  Paused` branch (producing the exact `"session not started or paused"`
  error string from the ticket's log excerpt) and, on at least some of the
  5-10 cycles, the client-side epoch-guard/`MessageQueue`-drop mechanism
  this session's Phase 2-4 work delivers — exercised through the real
  browser WebSocket path, not an MCP shortcut around it,
- **Then** `mcp__stapler-squad__read_session_output` confirms each cycle's
  marker string appears **at most once** in the pane content (never
  duplicated/replayed, and never appears as a phantom repeat of a prior
  cycle's marker or a bare `"1"`), **and**, after each cycle, the DOM is
  checked (via `claude-in-chrome` `read_page`/`find`) for the
  `InputDropBadge` element — the recorded outcome explicitly states whether
  `InputDropBadge`/its assertive announcement was observed to fire for at
  least one cycle — this is a required, checked pass/fail assertion, not an
  optional note: if zero cycles out of 5-10 show any evidence of hitting the
  race window (no `InputDropBadge`/drop signal ever fires across the whole
  run), the repro must be re-run with tighter alternation between typing and
  `pause_session`/`resume_session` calls, or the AC5 outcome must be
  explicitly flagged **inconclusive** rather than silently reported as a
  pass (pre-mortem.md Failure #2, P1).

##### Task 6.1.1.1 — Run the repro against a manual test instance using real browser automation

- Per this repo's `CLAUDE.md` "Manual/interactive testing without touching
  the live deployed instance" section: build and run a throwaway instance —
  `go build -o /tmp/ssq-manual-test . && PORT=8999
  STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test
  --tmux-keep-server &` — **do not** use `make install-service` (would kill
  the live deployed instance's sessions and tmux server).
- Create a session via `mcp__stapler-squad__create_session` targeting that
  manual instance (a throwaway directory session, not one-off, so it has a
  real tmux pane and a real `SessionDriver` attached).
- Using `claude-in-chrome` MCP tools (`tabs_create_mcp`/`navigate` to open a
  tab at `http://localhost:8999`, then `find`/`computer`/`read_page` to
  locate and interact with the UI), navigate to that session's terminal view
  and confirm the xterm terminal DOM element is visible with its WebSocket
  connected before starting cycles.
- **Concrete typing mechanism (pre-mortem.md Failure #2, P1 — and the fix to
  this plan's own prior repair mistake):** each cycle's marker is typed via
  `claude-in-chrome` browser automation issuing real keyboard events against
  the live xterm DOM element in the actual browser tab — **not** any MCP
  `write_to_session`/`send_control` pty-write shortcut. An MCP-level write
  bypasses the browser's WebSocket/`MessageQueue`/epoch-guard path entirely
  and would prove nothing about whether the shipped client-side fix (Phases
  2-4) works, only about tmux/`pause_session` behavior in isolation.
- Execute, one step per cycle (repeat 5-10 times):
  1. Use `claude-in-chrome` (`computer`/`find`) to click into the xterm
     terminal DOM element, ensuring it has keyboard focus.
  2. Use `claude-in-chrome` browser automation to type that cycle's
     distinguishable marker string (e.g. `cycle-0N-marker`) via real
     keyboard events.
  3. As tightly as possible around/during that typing — no deliberate
     delay, no other tool call in between where avoidable — call
     `mcp__stapler-squad__pause_session` for the session's `id`. This is a
     best-effort adversarial pairing, not guaranteed simultaneity: browser
     automation and the MCP call are issued by the same sequential agent
     and cannot be made literally concurrent.
  4. Call `mcp__stapler-squad__resume_session` for the same `id`, no
     deliberate delay.
  5. Use `claude-in-chrome` `read_page`/`find` to check the DOM for the
     `InputDropBadge` element and record whether it is present/visible for
     this cycle (evidence the client-side drop path was actually exercised
     through the real browser WebSocket path on this cycle, not just
     tmux/`pause_session` behavior in isolation).
  6. Call `mcp__stapler-squad__read_session_output` and confirm this cycle's
     marker string appears **at most once** (not more than once — a
     duplicate is the phantom-replay bug reopening).
- After all 5-10 cycles: if `InputDropBadge` (or its assertive announcement,
  if independently observable) was observed for **at least one** cycle, the
  race window was demonstrably exercised through the real client path at
  least once — record AC5 as a pass (assuming no duplicate markers were
  observed). If the drop signal **never** fired across the full run, do not
  report a silent pass: either re-run with tighter alternation between
  typing and the `pause_session`/`resume_session` calls, or explicitly
  record the AC5 outcome as **inconclusive** and say so in the
  `report_progress` note — a clean transcript with no observed drop signal
  is not, by itself, proof the fix works.
- This browser-automation repro is a deliberate improvement over both the
  two prior unmerged branches' repro procedures and this plan's own earlier
  (mistaken) MCP-write-based repair attempt — it uses
  `mcp__stapler-squad__create_session` (avoiding the `dca931a04`
  wrong-session-creation-path mistake and its resulting missing
  `SessionDriver`) and drives the fix through the actual browser client path
  the shipped code runs in production, not a server-side shortcut around it.
- Record the outcome (pass/fail/inconclusive, any anomalies observed, and
  per-cycle marker/badge results — including whether the drop signal fired
  at least once across the run) via `mcp__stapler-squad__report_progress`
  with `item_id=04089969-0f19-499c-be34-2e8bcfc4f13e`, `criteria_index=4`
  (AC5 is the 5th criterion, 0-indexed), `status` reflecting the outcome —
  per this repo's `backlog:done-4`/`backlog:fail-4` convention. Do not report
  `status=done` if the drop-signal evidence is inconclusive per the rule
  above.

**Resolution path if inconclusive after the re-run/tighter-loop attempt**
(added per triad readiness review — Product gap 1): if, after re-running with
tighter alternation, the drop signal still never fires across the full run,
record the finding via `report_progress` with `status=fail` (the tool's
`status` enum is `pass`/`fail`/`in_progress` only — there is no
`inconclusive` value, so this is the closest honest status; it must **not**
be reported as `pass`) and an honest one-line `note` explicitly saying the
outcome is inconclusive, plus what was tried (e.g. "INCONCLUSIVE: 5 cycles +
5 tighter-loop re-run cycles, zero InputDropBadge observations;
browser-automation timing likely too coarse to land inside the race window").
An inconclusive AC5 outcome does **not**
block shipping this PR: AC1/AC2 (the ticket's actual user-facing symptom) are
independently already fixed on `main` and verified by Phase 1's isolated
regression test (`TestApprovalAwaitingClearLatch_PreventsPhantomReplayAcrossReconnectChurn`),
and AC3/AC4 (the client-side hardening this plan adds) are independently
verified by the Jest (`useTerminalStream.test.ts`, `InputDropBadge.test.tsx`,
`MessageQueue.test.ts`) and Go (`TestControlModeReadLoop_BoundedExitOnConnClose`)
regression suites — neither depends on AC5's live-browser timing race
succeeding. An inconclusive AC5 only means the live evidence for that one
specific criterion is weaker than desired, not that the shipped fix is
unverified. If this happens, flag it explicitly in the PR description for
human awareness — do not let it pass unmentioned.

---

## Phase 7 — AC6 Confirmation (No Code)

### Epic 7.1 — Confirm multi-tab concurrent input remains explicitly out of scope

#### Story 7.1.1 — Verify Non-Goals section covers it

**AC6 Given-When-Then:**
- **Given** `project_plans/phantom-keystroke-replay/requirements.md`'s
  Non-Goals section (lines 104-107),
- **When** this plan's diff ships without touching multi-tab/multi-window
  concurrent-input handling for the same session,
- **Then** AC6 remains satisfied exactly as already documented — no task
  needed; a future duplicate/lost-input report from two simultaneously
  attached tabs is explicitly a distinct problem, not a regression of this
  work.

##### Task 7.1.1.1 — Grep-verified confirmation (no production code change)

Make the "verified, no task" claim an actual, repeatable check rather than
an assumption (per adversarial-review.md):

- Run `grep -rn "BroadcastChannel\|localStorage" web-app/src/lib/hooks/useTerminalStream.ts web-app/src/lib/terminal/MessageQueue.ts`
  and confirm no matches — this diff's actual scope (single-hook,
  single-queue, single-Go-handler) never introduces cross-tab shared state
  (a `BroadcastChannel`, a shared `localStorage` key, or similar) that would
  make multi-tab behavior implicitly in-scope.
- Confirm `requirements.md`'s Non-Goals section wording is unchanged by this
  session's diff (still explicitly names concurrent multi-tab/multi-window
  input as out of scope).
- No production code or doc change required beyond this check; record the
  grep's empty result as the AC6 evidence.
