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
| Connection epoch | A monotonically increasing integer identifying one `connect()` attempt's lifetime, from entry to its `finally` block completing. | `connectionEpochRef: React.MutableRefObject<number>` in `useTerminalStream.ts` |
| Current epoch / `epoch` | The epoch value captured locally by a specific `connect()` invocation at entry, before any `await`. | `const epoch = ++connectionEpochRef.current` |
| Superseded attempt | A `connect()` invocation whose captured `epoch` no longer equals `connectionEpochRef.current` — a newer attempt has since started. | Comparison `epoch !== connectionEpochRef.current` at each checkpoint |
| Drop count | The number of `TerminalData` messages discarded from a `MessageQueue`'s internal buffer at the moment `close()` is called. | Return value of `MessageQueue.close(): number` |
| Dropped-input event | A React state value describing the most recent coalesced batch of dropped input, exposed by the hook for the UI layer to render/announce. | `droppedInputEvent: { count: number; at: number } \| null` returned from `useTerminalStream` |
| Drop-and-signal badge | The visible + audible UI signal shown when input is dropped. | `InputDropBadge` component (`web-app/src/components/sessions/InputDropBadge.tsx`) |
| Assertive announcement | The `aria-live="assertive"` screen-reader announcement accompanying a drop, distinguishing it from routine `polite` connection-status chatter. | `useLiveRegion().announce(...)` with `politeness="assertive"`, `role="alert"` |
| Coalescing window | The short (debounced) time window during which multiple drop events are batched into a single badge update + single announcement, rather than one per dropped chunk. | Local timer/ref state inside `InputDropBadge` |
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
| `MessageQueue.close()` drop semantics | Clear `this.queue = []` in place, return dropped count | Add a separate `abandon()`/`discard()` method distinct from `close()` | No caller in this codebase ever wants a superseded/closing queue to still flush buffered input afterward (confirmed: `close()` has one call shape, always reconnect- or disconnect-triggered, both of which need discard semantics per ADR-023's "one-shot MessageQueue" rationale) — a second method is speculative API surface with no real second caller (interface-pollution-checklist smell #1). |
| Drop-signal UI placement | New `InputDropBadge`, mounted per-terminal-instance in `TerminalOutput.tsx` | Extend the existing global `ConnectionIndicator` (`components/layout/`) to also show input-drop state | `ConnectionIndicator` is one instance per app shell representing session-list-wide connection status (no `sessionId` prop); input-drop is inherently per-open-terminal state owned by `useTerminalStream`. Reaching a global component would require prop-drilling/new context to carry per-session state into a component that structurally isn't per-session. ux.md research explicitly recommends keeping the two "distinct and complementary." |
| No new interface/abstraction | Patch `MessageQueue` and `useTerminalStream` in place; no `Manager`/`Service` wrapper | — | Per `.claude/rules/interface-pollution-checklist.md`: `MessageQueue` is already a minimal single-purpose class with exactly one real shape of caller; wrapping it or introducing an interface would be a speculative interface (smell #1) with no second implementation in sight. Confirmed independently by `research/build-vs-buy.md`. |
| Coalescing/debounce ownership | Local state inside `InputDropBadge` (no new custom hook) | A shared `useCoalescedAnnouncement()` hook | Single consumer today; a generic hook for one call site is an unjustified generic (interface-pollution-checklist smell #5) — write the concrete version first. |
| `LiveRegion` accessibility role | Add an optional `role` prop (default `"status"`, backward compatible) so `InputDropBadge` can pass `role="alert"` | Compose a second, parallel live-region primitive for the assertive case | `LiveRegion`/`useLiveRegion` currently has **zero real consumers** (`ConnectionIndicator.tsx` hand-rolls its own `aria-live` div instead of importing it — confirmed by grep, no import statement exists). `InputDropBadge` becomes its first real adopter; extending the existing shared primitive with a role prop is minimal and keeps this codebase from growing a second live-region pattern, per pitfalls.md's explicit warning against ad-hoc bespoke patterns. |
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
| A generation-ref port that increments only once (either only in `connect()` or only inside the message-loop IIFE) silently under-protects, per pitfalls.md's explicit warning about `useSessionService.ts`'s *double*-increment shape. | Story 3.1.1's tasks explicitly mirror both increment points: once at the very top of `connect()` (invalidating any prior attempt still in its `finally`), and epoch comparisons at all three checkpoints named in architecture.md §1 (loop body, catch, finally) — not just one. |
| Fixing `MessageQueue.close()` in isolation without exercising `useTerminalStream`'s actual usage of it reproduces the same "fixed the class, not the usage" gap that let AC3 get marked done incorrectly once already. | Story 3.2.1's "queued-message-drop-on-close interleaving" Jest test exercises `useTerminalStream` (via `renderHook`), not `MessageQueue` in isolation — per pitfalls.md §5. |
| `aria-live="assertive"` is unproven in this codebase (no existing shipped consumer) — cross-browser/AT behavior risk. | Story 4.2.3 includes an explicit accessibility-focused Jest test asserting `role="alert"` + `aria-live="assertive"` + `aria-atomic="true"` are present; full manual AT verification is called out as a follow-up in Unresolved Questions (out of scope for automated CI). |
| Rapid-repeat drops within `useLiveRegion`'s fixed 1000ms auto-clear window can silently swallow a second announcement (pitfalls.md §4, "clearImmediately" race). | Coalescing lives in `InputDropBadge`, not raw `announce()` calls — every drop within the debounce window updates a running count and re-triggers a single, content-changing (`"N keystrokes not sent"`) announcement, so identical-string suppression by AT is avoided by construction. |
| Go: extracting `controlModeReadLoop` is a refactor of live production code (not just additive) — regression risk to the two other structurally similar handlers (`streamShellViaControlMode`, `streamViaTmuxCapturePane`) if touched by mistake. | Phase 5 scope is explicitly `streamViaControlMode` only, matching requirements.md's file listing; the other two duplicated handlers are named as a flagged follow-up in Unresolved Questions, not touched here (Non-Goals: "general reconnect/re-render stability work beyond what's needed... is out of scope"). |
| AC5 manual repro requires actually inducing the "not started or paused" condition, not a generic network-offline toggle (per Constraints). | Story 6.1.1's task uses `pause_session`/`resume_session` MCP tools against a live tmux-backed session while its terminal WebSocket is open, which drives `session/instance_tmux.go:471-472`'s `!i.started.Load() \|\| i.Status == Paused` → `"session not started or paused"` path directly — the exact error string from the ticket's log excerpt. |

---

## Unresolved Questions

1. **Should `useTerminalFlowControl.sendInput`'s pre-existing silent early-return** (`useTerminalFlowControl.ts:143`, `if (!pushMessageRef.current || !isConnectedRef.current) return;`) **also feed the drop signal?** pitfalls.md §5 flags this as a second, older silent-drop path in the same input pipeline that AC3's "visibly and audibly signaled" language arguably also covers (a keystroke typed while already-known-disconnected never reaches `MessageQueue` at all, so `MessageQueue.close()`'s drop-count fix never sees it). This plan's Epic 4.1 wires it in as the more complete reading of AC3, but it's worth confirming with the backlog item's stakeholder that this is in-scope rather than a distinct future ticket, since requirements.md's own "Remaining confirmed gap" section does not name this specific file/line.
2. **`doneChan` closing for a reason unrelated to the WebSocket connection itself dying** (e.g. an output-side send error to a channel other than `errChan`) is a real, separate goroutine-leak vector named in pitfalls.md §3, distinct from what AC4 asks this plan to test (which is specifically "exit is bounded once the connection closes"). Not fixed here — flagged as a follow-up. If it recurs, the fix is a proactive `stream.conn.Close()`/`SetReadDeadline` call at the point `streamViaControlMode` is about to return, which this plan's Pattern Decisions table explicitly rejected adding *unconditionally* (would break the common case's latency).
3. **`streamShellViaControlMode` and `streamViaTmuxCapturePane`** (`connectrpc_websocket.go:~1104`, `~1494`) are structurally duplicated goroutine-coordination code with the same `doneChan`/`select`/`default`/`ReadMessage()` shape as `streamViaControlMode`'s Goroutine 2, per pitfalls.md §3's explicit "one fix, N-1 near-duplicates unaudited" warning (itself citing this exact ticket's server-side history as the precedent for that failure mode). This plan intentionally does not extend the `controlModeReadLoop` extraction/test to those two handlers — out of scope per Non-Goals — but a future ticket should audit whether they need the same test coverage.
4. **Should the badge auto-dismiss timing (visual dwell) be a fixed constant or configurable?** ux.md recommends "a few seconds, tunable" but doesn't pin an exact number; this plan picks 4000ms (matching the rough order of magnitude research suggested, roughly 4x `useLiveRegion`'s 1000ms announcement-clear window so the visual badge outlives the announcement) as a starting value — flagged in case product/UX wants a different default after real usage.

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

#### Story 1.1.2 — AC2: add a regression test tying the cooldown-latch fix to this backlog item

`session/session_driver_test.go:165-183` already has `TestShouldApprovePromptOnce`
covering issue #165 at the pure-function level (3 isolated assertions on
`shouldApprovePromptOnce(approvalVisible, awaitingClear)`). It is not tied to
this backlog item and doesn't exercise the *stateful sequence* across many
polling ticks the way `session_driver.go:428-451`'s actual loop does
(`approvalAwaitingClear` threaded call-to-call). Add a new test that drives
the same stateful sequence a real flapping episode would produce.

**AC2 Given-When-Then:**
- **Given** a simulated driver poll sequence for backlog item
  `04089969-0f19-499c-be34-2e8bcfc4f13e`: tick 1 `NeedsApproval` visible,
  `approvalAwaitingClear=false` → approve fires, `approvalAwaitingClear` set
  `true`; ticks 2–5 `NeedsApproval` still visible (simulating the PTY redraw
  lag the ticket's log lines show); tick 6 dialog no longer visible,
- **When** `shouldApprovePromptOnce(approvalVisible, approvalAwaitingClear)`
  is called once per tick with the loop's real state-threading rule
  (`approvalAwaitingClear = approvalAwaitingClear && approvalVisible` on the
  non-approve branch, mirroring `session_driver.go:447`),
- **Then** exactly one of the 6 calls returns `true` (tick 1) — asserting the
  same-dialog resend that produced the original ticket's repeated `"1"` does
  not happen across the full 6-tick simulated flap.

##### Task 1.1.2.1 — Add `TestApprovalCooldownLatch_PreventsPhantomReplayAcrossReconnectChurn` to `session/session_driver_test.go`

- File: `session/session_driver_test.go` (add after `TestShouldApprovePromptOnce`, ~line 184).
- Doc comment references backlog item `04089969-0f19-499c-be34-2e8bcfc4f13e`
  and quotes the ticket's log excerpt.
- Implements the 6-tick simulation above using only `shouldApprovePromptOnce`
  (already exported/package-visible in this test file) — no new production
  code.
- Run: `go test ./session/... -run TestApprovalCooldownLatch_PreventsPhantomReplayAcrossReconnectChurn -v`

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

#### Story 3.1.1 — Add the epoch ref, gate the loop/catch/finally, collapse the queue-close asymmetry

**AC3 (epoch guard half) Given-When-Then:**
- **Given** `connect()` is called for session `sess-flap-1` (attempt A,
  captured `epoch = 1`), and before attempt A's `for await` loop receives
  its first message, `connect()` is called again for the same session
  (attempt B, captured `epoch = 2`, since `connectionEpochRef.current` was
  incremented to `2` synchronously at B's entry),
- **When** attempt A's stream later delivers a message or its `finally`
  block runs,
- **Then** attempt A's message-processing branch and `finally` block both
  see `epoch (1) !== connectionEpochRef.current (2)` and skip all
  `setIsConnected`/`setTerminalState`/reconnect-scheduling — only attempt
  B's outcome is ever reflected in `isConnected`/`terminalState`, and
  attempt B's `MessageQueue` (not attempt A's) is the one installed in
  `messageQueueRef.current` for all subsequent `sendInput` calls.

##### Task 3.1.1.1 — Declare `connectionEpochRef` and increment at `connect()` entry

- File: `web-app/src/lib/hooks/useTerminalStream.ts`.
- Add `const connectionEpochRef = useRef(0);` near `isConnectingRef`
  (line 106), with a doc comment mirroring `useSessionService.ts:184`'s
  ("Monotonically-increasing... checked at every await checkpoint").
- At the very top of `connect()` (line 162, before `if
  (isConnectedRef.current...) return;`), add
  `const epoch = ++connectionEpochRef.current;` — synchronous, before any
  `await`, per pitfalls.md's "increment-point placement" warning. This is
  the single increment (unlike `useSessionService.ts`'s double-increment
  shape — see ADR-001 for why `useTerminalStream.ts` only needs one, given
  its entry guard already fully prevents concurrent starts, unlike
  `watchSessions()` which can be called by multiple independent external
  callers).

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

---

### Epic 3.2 — Jest regression coverage for the epoch guard

#### Story 3.2.1 — Overlapping-connect, triple-rapid-connect, and drop-on-close interleaving tests

**AC4 (Jest epoch-guard portion) Given-When-Then:**
- **Given** `useTerminalStream({ sessionId: 'sess-race-1', autoConnect: false })`
  rendered via `renderHook`, with `mockStreamTerminal` configured to return a
  fresh `makePushStream()` on each call,
- **When** the test calls `result.current.connect()` twice in immediate
  succession (no `await` between calls) and then pushes a message onto the
  *first* call's stream,
- **Then** `result.current.isConnected` never reflects the first stream's
  message (the hook's `isConnected` state only flips `true` in response to
  the *second* call's stream delivering its first message), proving the
  first (superseded) attempt's `firstMessage` branch was skipped.

##### Task 3.2.1.1 — `connect_should_ignoreStaleGenerationMessages_When_secondConnectSupersedesFirstBeforeFirstMessage`

- File: `web-app/src/lib/hooks/__tests__/useTerminalStream.test.ts`, new
  `describe('useTerminalStream — connection epoch guard', ...)` block.
- Enhance the module-level `MessageQueue` mock (lines 57-65) to track
  `close` calls with a per-instance id (e.g. `static instanceCount = 0`,
  each instance records its own id and whether `.close()` was invoked on
  it) so the test can assert the first instance was `.close()`-d and the
  second instance is the one `sendInput` targets.
- Configure `mockStreamTerminal` to return `pushStreamA.iterable` on call 1
  and `pushStreamB.iterable` on call 2.
- Call `connect()` twice synchronously, then `pushStreamA.push(makeOutputMsg())`,
  assert `isConnected` stays `false` (or whatever pre-first-message default
  is) until `pushStreamB.push(...)` fires.

##### Task 3.2.1.2 — `connect_should_notThrow_When_calledThreeTimesInRapidSuccession`

- Same describe block. Calls `connect()` three times synchronously with
  three distinct `mockStreamTerminal` return values (A, B, C). Asserts no
  exception is thrown/no unhandled promise rejection, and that only the
  *third* (`C`) call's `firstMessage` results in `isConnected === true`
  once `pushStreamC.push(...)` fires — covers pitfalls.md's explicit "must
  decide whether triple-reconnect should also proactively close superseded
  generations" question (answered: yes, via Task 3.1.1.2's unconditional
  `close()`, verified here by asserting instances A and B were both closed).

##### Task 3.2.1.3 — `useTerminalStream_should_dropQueuedInput_When_reconnectSupersedesQueueWithPendingMessages`

- Same describe block (hook-level, per pitfalls.md §5's explicit "must
  exercise the hook, not just the queue class" requirement — this
  complements, does not replace, Phase 2's queue-level tests).
- Sequence: `connect()` (attempt A, gets `messageQueueRef` instance #1) →
  simulate a queued-but-undelivered push on instance #1 (via the mock's
  `push` spy recording a call) → call `connect()` again (attempt B) before
  attempt A's stream delivers anything → assert instance #1's `.close` was
  called (Task 3.1.1.2's unconditional close) and instance #2 is the one
  installed — proving queued input from a superseded attempt is discarded,
  not carried forward into the new queue.
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
  normal case" constraint).

##### Task 4.1.1.1 — Add `droppedInputEvent` state and wire it from Task 3.1.1.2's close call

- File: `web-app/src/lib/hooks/useTerminalStream.ts`.
- Add `const [droppedInputEvent, setDroppedInputEvent] = useState<{ count: number; at: number } | null>(null);`
  near the other state declarations (line ~96).
- In Task 3.1.1.2's block, replace the bare `console.warn` with:
  ```ts
  if (droppedCount > 0) {
    setDroppedInputEvent({ count: droppedCount, at: Date.now() });
    console.warn(...);
  }
  ```
- Also apply the identical pattern inside `disconnect()`
  (`useTerminalStream.ts:387-390`) where `messageQueueRef.current.close()` is
  already called — an explicit user-initiated disconnect with pending input
  must fire the same signal.
- Add `droppedInputEvent` to the returned `TerminalStreamResult` object
  (line ~469) and its interface (line ~54).

##### Task 4.1.1.2 — Wire `useTerminalFlowControl.sendInput`'s pre-existing silent drop into the same signal

*(See Unresolved Question 1 — implemented here as the more complete reading
of AC3; flag for stakeholder confirmation.)*
- File: `web-app/src/lib/hooks/useTerminalFlowControl.ts:142-143`.
- Add an `onDrop?: () => void` option to `UseTerminalFlowControlOptions`;
  call it in `sendInput`'s early return (`if (!pushMessageRef.current ||
  !isConnectedRef.current) { onDrop?.(); return; }`).
- File: `web-app/src/lib/hooks/useTerminalStream.ts` — pass
  `onDrop: () => setDroppedInputEvent((prev) => ({ count: (prev?.at === Date.now() ? prev.count : 0) + 1, at: Date.now() }))`
  into the `useTerminalFlowControl({...})` call (line 144), so a keystroke
  rejected at the flow-control layer (already-known-disconnected) also
  surfaces via the same `droppedInputEvent` the badge consumes.

---

### Epic 4.2 — `InputDropBadge` component

#### Story 4.2.1 — Build the component, following `GitHubBadge.tsx`'s structural template

##### Task 4.2.1.1 — Create `web-app/src/components/sessions/InputDropBadge.tsx`

- Props: `{ droppedInputEvent: { count: number; at: number } | null }`.
- Structural template: `GitHubBadge.tsx` — plain function component, renders
  `null` when `droppedInputEvent === null` (matches `GitHubBadge`'s
  `if (!hasPR && !hasRepo) return null;` idiom), `title`/`aria-label` built
  from state.
- Internal state: `useRef<ReturnType<typeof setTimeout> | null>(null)` for
  the visual dwell/fade timer (4000ms, per Unresolved Question 4) and a
  `useRef<number>(0)` running total for the current coalescing episode,
  reset when a new `droppedInputEvent` arrives after the previous badge has
  fully faded (`at` gap larger than the dwell window).
- Renders a small pill: icon (reuse a simple "no-entry"/circle-slash inline
  SVG, `aria-hidden="true"`), text `"N keystroke(s) not sent"`, no color-only
  signal (pairs icon + text per `.claude/rules` a11y convention already used
  by `MemoryPressureCallout`).
- Uses `zIndex.floatingTerminalUI` (`theme-contract.css.ts:215`, already the
  named slot for terminal-anchored floating UI — no new slot needed).
- No focus-stealing (per ux.md §3 — no `autoFocus`, no `tabIndex` on mount).

##### Task 4.2.1.2 — Create `web-app/src/components/sessions/InputDropBadge.css.ts`

- Vanilla-extract, per `.claude/rules/css-architecture.md`. Structural
  template: `GitHubBadge.css.ts` (`style()` calls importing `vars` from
  `@/styles/theme-contract.css`, no hardcoded hex, no `var(--...)` strings).
- Variants: default pill style using `vars.color.warning`/`vars.color.textPrimary`
  (a drop is a warning-severity event, not a hard error) — no bespoke color.

#### Story 4.2.2 — Mount the badge and assertive announcement in `TerminalOutput.tsx`

**AC3 (mount/wiring) Given-When-Then:**
- **Given** `TerminalOutput` rendered for `sessionId = "sess-flap-1"` with an
  active `useTerminalStream` connection,
- **When** the hook's `droppedInputEvent` transitions from `null` to
  `{ count: 3, at: 1700000000000 }`,
- **Then** `InputDropBadge` renders visibly inside `styles.terminal`
  (alongside the existing `reconnectingBanner`/`hardFailedBanner` overlays,
  `TerminalOutput.tsx:1652-1662`) **and** a `LiveRegion` with
  `politeness="assertive"` `role="alert"` announces `"3 keystrokes not
  sent — connection interrupted"` exactly once for this coalesced episode.

##### Task 4.2.2.1 — Extend `LiveRegion` with an optional `role` prop

- File: `web-app/src/components/ui/LiveRegion.tsx`.
- Add `role?: "status" | "alert"` prop, default `"status"` (backward
  compatible — no existing real consumer to break, confirmed via grep).
- Pass through to the rendered `<div role={role} ...>` (line 22).

##### Task 4.2.2.2 — Mount `InputDropBadge` + `LiveRegion` in `TerminalOutput.tsx`

- File: `web-app/src/components/sessions/TerminalOutput.tsx`.
- Destructure `droppedInputEvent` from the `useTerminalStream(...)` call
  (line 456).
- Add `const { announce } = useLiveRegion();` and a `useEffect` keyed on
  `droppedInputEvent?.at` that calls
  `announce(\`${droppedInputEvent.count} keystroke${droppedInputEvent.count === 1 ? '' : 's'} not sent — connection interrupted\`)`
  only when `at` actually changes (guards the "don't spam" requirement —
  mirrors `ConnectionIndicator.tsx:31-36`'s `prevStateRef`-gated transition
  announcement, not a raw effect on every render).
- Render `<InputDropBadge droppedInputEvent={droppedInputEvent} />` inside
  `styles.terminal` (near line 1652, alongside the reconnect banners) and
  `<LiveRegion message={message} politeness="assertive" role="alert" />`
  nearby (visually hidden, per `LiveRegion`'s existing `srOnly` styling).

#### Story 4.2.3 — Jest tests for `InputDropBadge`

##### Task 4.2.3.1 — `InputDropBadge.test.tsx`

- File: `web-app/src/components/sessions/InputDropBadge.test.tsx`.
- Tests: `renders null when droppedInputEvent is null`; `renders a pill with
  the drop count when droppedInputEvent is set`; `has no color-only signal
  (icon + text both present)`; `does not set focus/tabIndex on mount`
  (queries `document.activeElement` is unchanged after render).
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="InputDropBadge.test"`

##### Task 4.2.3.2 — Accessibility assertions for the assertive announcement

- Add to `TerminalOutput.test.tsx` (or a new focused test file if that file
  is already large — check line count first): a test asserting that when
  `droppedInputEvent` changes, a `role="alert"` element with
  `aria-live="assertive"` and `aria-atomic="true"` appears in the DOM
  containing the drop count, and that a second `droppedInputEvent` with a
  new `at` (same count) still produces a new announcement text (covers
  pitfalls.md §4's "identical string suppressed by AT" risk — assert the
  announced string changes, e.g. includes a running total, not just the
  same static copy twice).

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

---

## Phase 6 — Manual Verification (AC5)

### Epic 6.1 — Reproduce the ticket's exact flapping condition and confirm no phantom keystrokes

#### Story 6.1.1 — Manual repro via `pause_session`/`resume_session` MCP tools

**AC5 Given-When-Then:**
- **Given** a live tmux-backed session created via
  `mcp__stapler-squad__create_session` (e.g. a throwaway directory session,
  not one-off, so it has a real tmux pane) with its terminal WebSocket open
  in a browser tab,
- **When** `mcp__stapler-squad__pause_session` and
  `mcp__stapler-squad__resume_session` are called in rapid alternation
  (5-10 cycles, no delay) against that session's `id` — this drives
  `session/instance_tmux.go:471-472`'s `!i.started.Load() || i.Status ==
  Paused` branch, producing the exact `"session not started or paused"`
  error string from the ticket's log excerpt, and correspondingly flaps the
  terminal WebSocket connect → error → reconnect cycle,
- **Then** `mcp__stapler-squad__read_session_output` shows no repeated
  phantom `"1"` (or any other single-character repeat) appearing in the
  pane content that wasn't actually typed, and (if any input was in flight
  during a pause boundary) the browser shows the `InputDropBadge` +
  assertive announcement rather than silently losing or replaying it.

##### Task 6.1.1.1 — Run the repro against a manual test instance

- Per this repo's `CLAUDE.md` "Manual/interactive testing" section: build to
  `/tmp/ssq-manual-test`, run with `PORT=8999
  STAPLER_SQUAD_INSTANCE=claude-manual-test`, `--tmux-keep-server` — **do
  not** use `make install-service` (would kill the live deployed instance's
  sessions).
- Create a session via `mcp__stapler-squad__create_session` pointed at that
  manual instance (or, if the MCP tools target the live `:8543` instance by
  default, use the browser directly against `:8999` and `read_session_output`/
  manual terminal typing to observe — confirm which is reachable before
  starting).
- Execute the pause/resume alternation from the Given-When-Then above while
  the terminal tab is open and actively receiving keystrokes.
- Record the outcome (pass/fail, any anomalies observed) directly on the
  browser session and via `mcp__stapler-squad__report_progress` with
  `item_id=04089969-0f19-499c-be34-2e8bcfc4f13e`, `criteria_index=4`
  (AC5 is the 5th criterion, 0-indexed), `status` reflecting the outcome —
  per this repo's `backlog:done-4`/`backlog:fail-4` convention.

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

##### Task 7.1.1.1 — No action

Already satisfied; no code or doc change required.
