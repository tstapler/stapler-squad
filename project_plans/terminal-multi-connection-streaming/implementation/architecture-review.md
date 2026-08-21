# Architecture Review: terminal-multi-connection-streaming
**Date**: 2026-08-20
**Verdict**: CONCERNS

## Constitution Violations
- [ ] None — no `docs/adr/ADR-000-architecture-constitution.md` exists in this repository (confirmed: `docs/adr/` contains ADR-003 through ADR-021+ but no ADR-000).

## Blocker Verification (repair pass)

### 1. Primitive obsession (`RequestResize`/`ResizeVote`/`NegotiatedSize`) — **RESOLVED**
A single `TerminalSize` value object now exists, constructed only via `NewTerminalSize(cols, rows int) (TerminalSize, error)` which rejects non-positive dimensions (plan.md:34, Task 1.3.1a at plan.md:300, AC at plan.md:293). It is used consistently as the sole representation across all three former sites:
- `RequestResize(id SubscriberID, size TerminalSize)` (plan.md:290, Task 1.3.1c at plan.md:306) — no more bare `cols, rows int`.
- `ResizeVote{SubscriberID, TerminalSize}` (plan.md:35, Task 1.3.1b at plan.md:303).
- `NegotiatedSize` — itself typed as a `TerminalSize` (plan.md:36, ADR-002 line 18).
The Domain Glossary (plan.md:34) and Pattern Decisions table (plan.md:64) both name this explicitly as replacing "three independent inline `{Cols, Rows int}` shapes," and ADR-002 restates the same constructor and usage. Internally consistent — no remaining bare-int or independently-inlined-struct usage found by grep across plan.md.

### 2. Import-cycle risk (`session` ↔ `session/streamhub`) — **RESOLVED** (one citation inaccuracy introduced)
- **(a) Cited `Instance` methods are real**: verified against `session/instance_tmux.go` — `ResizePTY` (line 587), `CapturePaneContent` (line 600), `StopControlMode` (line 727), `SubscribeControlModeUpdates` (line 733), `UnsubscribeControlModeUpdates` (line 738), `SetWindowSize` (line 752) all exist with matching signatures. That's 6 real methods for `SessionController`'s 6-method interface.
- **New issue (NITPICK)**: the citation string `session/instance_tmux.go:587,600,722,727,733,738,752` (Domain Glossary plan.md:32, Task 1.3.2a plan.md:336) has **7** line numbers for a 6-method interface. Line 722 is `Instance.StartControlMode` (`session/instance_tmux.go:722`), which is **not** one of `SessionController`'s six methods (`SetWindowSize`/`ResizePTY`/`CapturePaneContent`/`StopControlMode`/`Subscribe`-`UnsubscribeControlModeUpdates`) — `StartControlMode` is instead the method Story 3.1.2 has package `session` call directly (plan.md:545, `Instance.StartControlMode` imports `session/streamhub`), not a method the hub calls on the interface. The stray line number conflates "methods `SessionController` needs" with "the other call site in the same file," which could mislead an implementer into adding `StartControlMode` to the interface unnecessarily. Low severity — doesn't threaten the cycle-avoidance argument itself.
- **(b) Dependency direction is acyclic as described**: `StreamOwnershipLock` and its `xsync.Map[string, *StreamOwnershipLock]` are placed concretely in `session/streamhub` (ADR-003, Task 3.1.1a at plan.md:533), which depends only on the string session name — no dependency on `SessionController`/`Instance`. Package `session`'s `Instance.StartControlMode` imports `session/streamhub` one-way (Task 3.1.2a at plan.md:555). `session/streamhub` never imports package `session` for the concrete type, relying instead on the locally-defined `SessionController` interface satisfied structurally. Story 3.1.2 adds an explicit, executable check for this: `go list -deps ./session/streamhub/...` must not list package `session` (AC at plan.md:551, Task 3.1.2c at plan.md:561) — this is a real, automatable gate, not just an assertion in prose.
- **(c) No remaining "either (a)...or (b)" ambiguous framing**: confirmed by grep — zero hits for that pattern anywhere in plan.md or the three ADRs. Story 3.1.2 now states the package boundary as a labeled, non-optional decision: `**Package boundary (concrete, not "either/or")**` (plan.md:545).

### 3. Batching design ignoring `coalesceBufPool`/`pkg/ansi` precedent — **RESOLVED**
- `server/services/connectrpc_websocket.go:790-845` verified: this range does contain the `coalesceBufPool` coalescing loop (`cbp := coalesceBufPool.Get().(*[]byte)` at 805 through `coalesceBufPool.Put(cbp)` at 845), matching the plan's citation.
- `pkg/ansi` verified as a real, imported package (`server/services/connectrpc_websocket.go:26`, and `pkg/ansi/csi.go` exists on disk).
- Task 2.1.1a (plan.md:443) now explicitly says "adapting the buffer-pooling pattern from `coalesceBufPool`... to avoid per-flush allocation rather than designing a new pooling scheme," and Task 2.1.1b (plan.md:446) says "routing any escape-sequence-sensitive byte handling through the existing `pkg/ansi` filtering rather than reimplementing it." Task 2.1.1e (plan.md:455) adds a dedicated benchmark/test task that explicitly cites the same source lines. The Pattern Decisions table's "Output batching" row (plan.md:60) also names the reuse decision and cites `research/build-vs-buy.md` §3b's original recommendation, closing the citation gap the original blocker found. Language is concrete task-level direction, not a passing mention.

## Blockers

None outstanding from the prior pass. (No new blockers found in the repair diff.)

## Concerns

Carried forward unchanged — the repair pass did not touch any of these:

- [ ] **Story 1.2.1 (plan.md:223-257)** — Still internally inconsistent: the third AC (plan.md:231) says a slow subscriber is evicted "once it exceeds the bounded-queue-full threshold, not before" (implying immediate eviction on a full channel), while Task 1.2.1e (plan.md:249-251) says "grace period then close+remove," mirroring `controlModeSlowSubscriberGrace` (`session/tmux/control_mode.go:51`). Both phrasings are still present verbatim. **Recommendation**: unchanged — pick the grace-period model (matches existing repo precedent) and rewrite the AC to match exactly.
- [ ] **Story 1.3.1 / Story 2.2.2 (plan.md:287-298, 481-498)** — Still no parse-at-boundary step specified for raw `cols`/`rows` values arriving from a browser resize event before they become a `ResizeVote`/`TerminalSize`; grep for "transport boundary"/"malformed" in plan.md returns nothing. The new `NewTerminalSize` constructor (Blocker #1's fix) is a necessary but not sufficient fix here — nothing in `WebSocketTransport`'s AC (Story 2.2.1, plan.md:481) requires it to call `NewTerminalSize` at the point a raw browser value is received, so an unvalidated value could still reach the boundary without going through the constructor. **Recommendation**: unchanged — add an explicit AC requiring `WebSocketTransport`/the connection handler to construct a `TerminalSize` via `NewTerminalSize` immediately on receiving a raw resize event, propagating the constructor's error back to the client rather than forwarding a bad value further inward.
- [ ] **`HubRegistry` (Pattern Decisions, plan.md:54)** — Still no story states whether `HubRegistry` is an explicit constructed/injected value or an implicit package-level global; grep for `NewHubRegistry`/`HubRegistry{` in plan.md returns nothing. **Recommendation**: unchanged — add a task making `HubRegistry` an explicit constructor-created, dependency-injected value.

## Nitpicks

Carried forward unchanged, plus one new item from the repair pass:

- `SubscriberCapability{CanResize, CanWrite bool}` (plan.md:33) is still a boolean pair; not yet an illegal-states problem, but consider a capability-set type if a third dimension is ever added.
- The batch-flush "trigger" (`opportunistic` vs. `ceiling`, plan.md:83, 436, 449) is still a bare string/log-tag, not a typed sum type, despite the plan's otherwise-consistent type-driven-design discipline (`HubLifecycleState`, `StreamPath`, now also `TerminalSize`). Consider a small `FlushReason` enum.
- tmux session names are still passed as bare `string` throughout (`HubRegistry.GetOrCreate`, `AcquireOwnershipLock(sessionName string)`, plan.md:39, 533, 545) while `SubscriberID` gets a newtype for the same "identifier" concern — likely fine if it matches existing `session` package convention, but still worth a one-line confirmation.
- **New**: the `SessionController` method-citation list (`session/instance_tmux.go:587,600,722,727,733,738,752`, plan.md:32, 336) contains an extra line number (722, `StartControlMode`) beyond the 6 methods the interface actually declares — see Blocker Verification #2 above for detail. Trim the citation to the 6 real interface methods, or explicitly note that 722 is cited for a different reason (Story 3.1.2's separate call site) if it's meant to stay.
