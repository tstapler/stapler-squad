# ADR-001: Lazy Scrollback Delivery Strategy

**Status**: Accepted
**Date**: 2026-04-16
**Deciders**: Tyler Stapler

---

## Context

Large Claude Code sessions produce MBs of terminal output. The current `StreamTerminal` handler sends every entry in the `CircularBuffer` on attach via `GetAll()`. For sessions running hours-long tasks, this causes multi-second stalls before the first terminal paint.

The backend `CircularBuffer` already exposes `GetLastN(n)` and `GetRange(fromSeq, limit)` with monotonic sequence numbers (`GetOldestSequence()`, `GetNewestSequence()`). The frontend xterm terminal is configured with `scrollback: 0` — tmux owns history, not xterm. The `StreamTerminal` bidirectional stream is the existing channel for initial scrollback delivery.

Two delivery strategies were evaluated:

- **Option A (Phase-1 tail-only)**: Change `StreamTerminal` to call `GetLastN(500)` instead of `GetAll()` on attach. No scroll-up loading. Ships in 3 days.
- **Option B (Full lazy loading)**: Add `GetScrollback(sessionId, fromSeq, limit)` unary RPC. On scroll-to-top trigger in the browser, fetch 500 older lines and prepend to xterm. Ships in 3–4 weeks.
- **Option C (Stream extension)**: Extend the existing bidirectional `StreamTerminal` with a new message type that requests historical ranges in-band.

---

## Decision

**Implement in two phases:**

**Phase 1**: Cap `StreamTerminal` initial payload to the last 500 entries via `CircularBuffer.GetLastN(500)`. No new RPC. No scroll-up loading.

**Phase 2** (separate PR, separate epic): Add `GetScrollback(sessionId, fromSeq, limit)` as a standalone unary RPC. On xterm viewport scroll reaching the top, the frontend calls `GetScrollback` with `fromSeq = oldestVisibleSeq - 1` and prepends the result. Include cursor-state preservation using save/restore escape sequences around the prepend write.

Option C (stream extension) is rejected: it adds message-type complexity to the already-complex bidirectional stream and makes the feature harder to test in isolation.

---

## Rationale

Phase 1 fixes 80% of the pain (large sessions loading slowly) with minimal risk. The change is a one-line backend modification to the StreamTerminal attach path — no new RPC, no frontend changes beyond removing the infinite scroll expectation.

Phase 2 is the complete solution but carries cursor-corruption risk (see Known Issues in the main feature plan) that requires careful testing. Phasing prevents that risk from blocking the Phase 1 win.

The `CircularBuffer.sequence` field is monotonically increasing and is not reset across buffer wraps — `GetOldestSequence()` returns the correct floor for Phase 2 range requests. However, sequences ARE reset on server restart (the field starts at 0 each boot), so Phase 2 must include a "full-redraw" flag in the response when the server sequence space has been reset.

---

## Consequences

**Positive:**
- Phase 1 ships in ~3 days with near-zero risk.
- Phase 2 can be planned independently once Phase 1 is in production.
- No changes to the ConnectRPC proto schema for Phase 1.
- Backend `CircularBuffer` methods are already correct and tested.

**Negative / Accepted costs:**
- Phase 1 discards scrollback older than 500 entries for the browser view (backend ring buffer retains all entries up to its configured max size).
- Phase 2 has cursor-corruption risk during prepend that must be mitigated with save/restore escape sequences before `terminal.write()`.
- After a server restart, Phase 2 clients must detect the sequence gap and trigger a full redraw rather than a partial prepend.

---

## Alternatives Not Chosen

**Option C (Stream extension)**: Rejected because adding a new message type to the bidirectional stream requires both client and server changes, increases stream state complexity, and makes the feature much harder to unit-test.

**Full lazy loading in Phase 1**: Rejected because cursor-state correctness during prepend requires dedicated testing that would delay the Phase 1 ship.
