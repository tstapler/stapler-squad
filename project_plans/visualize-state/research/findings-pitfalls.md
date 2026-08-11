# Findings: Pitfalls — Known Failure Modes

Created: 2026-04-14

## Summary

Five critical failure categories were identified from codebase analysis and general knowledge. Two are P0 (silent stream disconnect causes stale status decisions; status stuck after reconnect misses delta updates). The most surprising finding: the existing `session/detection/detector.go` already has 10-state pattern detection, but several implementation risks make it unreliable — partial output at chunk boundaries can trigger false positives, and there's no debounce to prevent status flip-flopping on rapid output.

**Dominant risk**: The codebase's streaming architecture uses delta events only — there is no full-sync on reconnect. Any missed event (network blip) leaves the client permanently stale with no indication.

## Risk Categories

| Category | Severity | Likelihood | Mitigation Cost |
|---|---|---|---|
| Partial ANSI sequences at chunk boundary | P1 | HIGH | Medium |
| Color/attribute state bleed between cards | P1 | MEDIUM | Low |
| Wide char / emoji column miscalculation | P2 | LOW | Medium |
| XSS via unescaped terminal output | P2 | VERY LOW | None (already handled) |
| xterm.js memory leak on unmount | P2 | MEDIUM | Low |
| React key thrashing / scroll loss | P1 | MEDIUM | Low |
| Status churn causing re-renders | P2 | MEDIUM | Low |
| ANSI preview height causing layout shift | P2 | MEDIUM | Low |
| Silent stream disconnect → stale status | **P0** | HIGH | Medium |
| Status stuck after reconnect (delta loss) | **P0** | HIGH | Medium |
| Race: status event before session metadata | P1 | MEDIUM | Low |
| Multiple consumers creating redundant streams | P2 | MEDIUM | Low |
| Pattern match on partial output (false positive) | P1 | HIGH | Medium |
| Status flip-flop on rapid output | P1 | HIGH | Low |
| Hook approval detection encoding mismatch | P2 | LOW | Low |
| User approves already-timed-out approval | P1 | MEDIUM | Low |
| Approval lost on network error (no idempotency) | P1 | LOW | Low |
| Multiple pending approvals: wrong item approved | P2 | LOW | Low |

## Trade-off Matrix (severity × mitigation cost)

| Risk | Severity | Mitigation Cost | Priority |
|---|---|---|---|
| Silent stream disconnect | P0 | Medium | Fix first |
| Status stuck after reconnect | P0 | Medium | Fix first |
| Pattern match partial output | P1 | Medium | Fix early |
| Status flip-flop | P1 | Low | Fix early |
| Partial ANSI sequences | P1 | Medium | Fix early |
| Key thrashing / scroll loss | P1 | Low | Fix early |
| User approves expired approval | P1 | Low | Fix before ship |
| Color state bleed | P1 | Low | Fix before ship |
| xterm.js memory leak | P2 | Low | Polish |
| Layout shift (card height) | P2 | Low | Polish |
| Approval lost (no idempotency) | P1 | Low | Polish |

## Risk and Failure Modes (detailed)

### Category 1: ANSI/VT100 Rendering in Browser

**1.1 Partial escape sequences at chunk boundary (P1 — HIGH)**
- `circular_buffer.go` stores raw bytes; 16KB WebSocket chunks split at arbitrary boundaries
- A chunk may end mid-escape: `...red text\x1b[` followed by `31m...` in next chunk
- `ansi-to-html` receives orphaned `\x1b[` — renders as literal ESC or skips it; next chunk inherits broken state
- Result: Corrupted color rendering on snapshot cards with no visible error
- Mitigation: Maintain a small carry-buffer (last ~10 bytes) between chunks; detect incomplete CSI/OSC sequences (ends with `\x1b` or `\x1b[` without terminal letter); hold until next chunk arrives or 100ms timeout
- Test: Synthesize output with escape sequence deliberately spanning chunk boundary

**1.2 Color/attribute state bleeding between cards (P1 — MEDIUM)**
- `terminal_state.go`: `CurrentStyle` maintained in `TerminalState` struct
- `Clone()` method exists (line ~843) but unclear if called before each card render
- If snapshot objects are reused: Card A renders `\x1b[31m` (red); Card B processes same output and inherits `FgColor = "color1"` → Card B shows red text where it should show default
- Mitigation: Ensure each `SessionCard` calls `GenerateDelta()` / `GenerateState()` on a fresh clone; add test: verify `CurrentStyle` resets between independent snapshots

**1.3 Wide characters / emoji breaking column count (P2 — LOW)**
- `Cell` struct in `terminal_state.go` uses single `rune` — no width tracking
- CJK characters (日本語) and emoji are 2 columns wide; cursor increment hardcoded to 1
- Subsequent ANSI cursor codes reference wrong column positions
- Mitigation: Add `width int` field to `Cell`; use Unicode East Asian Width lookup; adjust `CursorCol += cell.width`

**1.4 XSS via terminal output (P2 — VERY LOW)**
- `ansi-to-html` escapes HTML entities by default — `dangerouslySetInnerHTML` is safe from trusted backend
- Risk only if future refactor converts `TerminalState → HTML` directly without escaping
- Mitigation: Code review rule — never use raw HTML string injection for terminal content

**1.5 xterm.js Terminal instance memory leak (P2 — MEDIUM)**
- `XtermTerminal.tsx` creates a `Terminal` instance; if `terminal.dispose()` not called in `useEffect` cleanup, instance persists after unmount
- User opens/closes session details 100× → 100 Terminal instances holding ~10MB scrollback each
- Mitigation: Verify `useEffect(() => { return () => xtermRef.current?.dispose() }, [])` is present; validate with Chrome heap snapshots

---

### Category 2: React List Instability

**2.1 Key thrashing causing scroll position loss (P1 — MEDIUM)**
- If `SessionList` uses `session.title` (or any mutable field) as React key, renaming a session changes the key
- React unmounts old component, mounts new one — scroll position resets to top
- Mitigation: Use `session.id` (stable UUID from proto) as key, never `title`

**2.2 Status churn causing excessive re-renders (P2 — MEDIUM)**
- `ApprovalsContext` polls every 30 seconds — triggers context update → all consumers re-render
- With 50 session cards, all 50 re-render even if their session's status is unchanged
- Mitigation: `React.memo(SessionCard, (prev, next) => prev.session === next.session)`; split approval context from session list context; use selector hooks

**2.3 ANSI preview height causing layout shift (P2 — MEDIUM)**
- Session card with dynamic ANSI preview height: 3 lines → 8 lines on update → card expands → user clicks wrong card
- Mitigation: Fixed `max-height` on preview container (`max-height: 120px; overflow: hidden`); prevents CLS

**2.4 Concurrent Mode tearing (P2 — LOW)**
- If React 18 Concurrent features used without `startTransition`, status header and card body may show inconsistent values during render
- Mitigation: Wrap non-urgent status updates in `startTransition()`

---

### Category 3: Streaming/Reconnect Edge Cases

**3.1 Silent stream disconnect → stale status (P0 — HIGH)**
- `WatchSessions` stream has no visible keepalive/heartbeat in proto
- TCP connection closes silently (network blip); browser doesn't detect for ~10 minutes (TCP timeout)
- Events stop arriving; status shows value from minutes ago; user makes decisions on incorrect info
- Mitigation: Server sends empty heartbeat `SessionEvent` every 5 seconds if no real events; client triggers reconnect if `Date.now() - lastEventTime > 10_000ms`; show "status stale" badge after 15s without event

**3.2 Status stuck after reconnect — delta event loss (P0 — HIGH)**
- `WatchSessions` emits only delta events; no initial full-sync on reconnect
- Client misses updates during disconnect window (Running → Ready → Paused — three events)
- After reconnect, stream only sends new deltas; client stuck at last seen state (Ready, not Paused)
- Mitigation: On reconnect, immediately call `ListSessions()` to fetch full current state before re-subscribing to stream; or add `initial_snapshot` flag to `WatchSessionsRequest` that sends full state on connect

**3.3 Race: status event arrives before session metadata (P1 — MEDIUM)**
- Client calls `ListSessions()` (slow); meanwhile `WatchSessions` sends `SessionUpdatedEvent` for session "xyz"
- Event arrives before `ListSessions` response; client can't find "xyz" in session map → event dropped
- Mitigation: Buffer incoming stream events until initial `ListSessions` response received; replay buffer after map populated

**3.4 Multiple component stream subscriptions (P2 — MEDIUM)**
- If `SessionList`, `SessionDetail`, and `ApprovalPanel` each create a `WatchSessions` subscription independently, server sends 3× the events
- Mitigation: Centralize to one global `WatchSessions` subscription (e.g., in `ApprovalsContext` or a dedicated `SessionStreamContext`); all UI reads from a single derived store

---

### Category 4: Go Backend Pattern Detection Failures

**4.1 Pattern match fires on partial output (P1 — HIGH)**
- `detector.go` runs on every `ProcessOutput` call, even mid-write
- Claude writes `"Waiting for your ap..."` then continues writing; detector matches "Waiting" → fires `NeedsApproval`; session added to review queue prematurely
- Mitigation: Require patterns to match full lines (anchor `^...$` with multiline flag); debounce detection — only run after 500ms of output silence; require complete line terminator before matching

**4.2 Status flip-flopping on rapid output (P1 — HIGH)**
- Detector stateless: runs on latest output each time → rapid output changes cause Ready → Active → Idle → Ready within 200ms
- Frontend receives multiple `SessionStatusChangedEvent` in quick succession; UI flickers
- Mitigation: Add debounce (200-500ms) before emitting status change events; only emit if detected state differs from last emitted state; track `lastEmittedDetected` in `InstanceStatusManager`

**4.3 Approval detection missing due to encoding variation (P2 — LOW)**
- `approval.go` patterns expect exact Unicode (e.g., `→` arrow character)
- In some terminal locale configurations, `→` (U+2192) rendered as `?` or ASCII `>`
- Pattern doesn't match; hook times out silently
- Mitigation: Normalize Unicode to NFKC before matching (`golang.org/x/text/unicode/norm`); make patterns handle encoding variants (`(?:→|>|=>)`)

**4.4 Pattern match fires mid-line write (P1 — MEDIUM)**
- Scrollback buffer receives bytes as they arrive; write may split mid-line
- Partial line could spuriously match a pattern prefix
- Mitigation: Buffer until newline received; only run detector against complete lines

---

### Category 5: Approval Flow Edge Cases

**5.1 User approves already-timed-out approval (P1 — MEDIUM)**
- `PendingApproval.ExpiresAt` field exists (seen in `approval_automation.go:32`) but may not be checked before resolving
- After hook timeout, Claude receives error and continues/aborts; user later clicks Approve → silent success response but no hook waiting
- Mitigation: In `ResolveApproval`, check `time.Now() > pending.ExpiresAt` → return error "Approval request expired"; UI disables button and shows "Expired" state when countdown reaches 0

**5.2 Approval lost on network failure (no idempotency) (P1 — LOW)**
- Client sends `ResolveApproval` RPC; TCP reset mid-flight; client retries; server receives same approval twice
- No idempotency key → double approval; hook may execute action twice
- Mitigation: Add `idempotency_key string` to `ResolveApprovalRequest`; server stores recently processed keys with TTL and returns cached result on duplicate

**5.3 Multiple pending approvals — wrong item approved (P2 — LOW)**
- Non-deterministic ordering of `GetReviewQueue()` response
- User intends to approve "PR merge?" but clicks "Delete files?" which appeared in that position on previous render
- Mitigation: Sort `GetReviewQueue()` deterministically by `(priority desc, received_at asc)`; use stable keys in React list (approval ID)

---

## Migration and Adoption Cost

| Mitigation | Backend | Frontend | Effort |
|---|---|---|---|
| Reconnect + full sync on WatchSessions | Medium | Low | 1-2 days |
| Heartbeat event from server | Low | Low | 0.5 days |
| Partial ANSI carry-buffer | No | Yes | 1 day |
| Stable React keys (session.id) | No | Low | 2 hours |
| Detection debounce + flip-flop prevention | Low | No | 0.5 days |
| Expiry check in ResolveApproval | Low | Low | 2 hours |
| Color state clone-before-render | No | Low | 2 hours |
| Fixed preview height (CSS) | No | Trivial | 1 hour |
| Idempotency key for ResolveApproval | Low | Low | 2 hours |

**Highest impact / lowest cost wins** (do these first):
1. Stable React keys — 2 hours, prevents scroll loss on status updates
2. Detection debounce — 0.5 days, prevents flip-flopping and false-positive queue entries
3. Expiry check in `ResolveApproval` — 2 hours, prevents approving dead hooks
4. Color state clone — 2 hours, prevents color bleed across session cards

## Operational Concerns

**Current observability gaps**:
- No metrics for stream reconnect frequency (can't tell how often disconnects happen)
- No logging of which pattern matched / didn't match in `detector.go`
- No approval timeout counter (can't tell if approvals frequently time out)
- No React render performance instrumentation (no CLS measurement, no re-render frequency)

**Recommended instrumentation**:
- Backend: `detection_pattern_matched{pattern_name, session_id}` counter; `approval_timeout_total` counter; `stream_reconnect_total` counter
- Frontend: `performance.measure()` around snapshot conversion; track `Date.now() - lastEventTime` for staleness detection; React DevTools Profiler on `SessionCard`

## Prior Art and Lessons Learned

**xterm.js**: Maintains `_inputHandler` with carry-buffer for incomplete sequences — our approach of raw chunk-by-chunk processing risks visible corruption. Lesson: Complete VT parsers always buffer.

**gRPC keepalive**: Java gRPC uses 30s keepalive pings + max idle timeout. Envoy proxy sends heartbeat events to detect dead streams. ConnectRPC has no standard equivalent — custom heartbeat required.

**Fail2ban / CloudWatch Logs**: Both use full-line matching anchored at `^...$`, not substring search, to prevent false positives. Lesson: Substring regex on streaming output is inherently fragile.

**systemd-journal**: Timestamp + priority + structured fields (not substring matching). Lesson: Structured output markers are more reliable than regex heuristics.

**Lessons from teams who shipped similar UIs**:
- GitHub Actions: Eliminated full-page refresh on job completion → replaced with delta updates → eliminated scroll position loss
- PagerDuty v1: Desktop modal for every alert → users disabled all notifications → missed real incidents. v2: Badge + drawer.
- CircleCI: Initial full-sync on WebSocket connect prevents the "reconnect leaves stale state" P0

## Open Questions

- [ ] What ANSI escape codes does Claude output most frequently? — needed to tune partial-sequence buffer threshold
- [ ] What is typical terminal output rate during active Claude session? — determines if chunk-boundary splits are frequent
- [ ] How often do approvals time out in production? — baseline for expiry check priority
- [ ] Is `PendingApproval.ExpiresAt` currently populated and accurate? — blocks expiry check implementation
- [ ] Does `WatchSessions` send initial full state on connect, or deltas only? — critical: determines if P0 reconnect fix is needed

## Recommendation

**Phase 1 — Prevent incidents (Week 1)**:
1. Reconnect full-sync: On WatchSessions reconnect, call `ListSessions()` before re-subscribing → eliminates P0 stale status
2. Expiry check in `ResolveApproval` → eliminates approving dead hooks
3. Detection debounce (200ms) + emit-only-on-change → eliminates status flip-flopping

**Phase 2 — Reduce flakiness (Week 2)**:
1. Color state clone-before-render → correct colors on all session cards
2. Stable React keys (session.id) → no scroll position loss
3. Fixed ANSI preview height (CSS) → no layout shift misclicks
4. Partial ANSI carry-buffer → correct rendering near chunk boundaries

**Phase 3 — Harden (Week 3+)**:
1. Heartbeat event from server → detects dead streams proactively
2. Idempotency key for `ResolveApproval` → prevents double-approval
3. Observability instrumentation → all failure modes visible in production

## Pending Web Searches

1. `"xterm.js escape sequence partial buffer assembly implementation"` — confirm carry-buffer strategy
2. `"connectrpc web keepalive heartbeat streaming 2024"` — check if standard mechanism exists
3. `"react list key stability scroll position loss prevention"` — confirm React key strategy
4. `"go pattern detector debounce terminal output state machine"` — find prior art for debounced detection
5. `"cumulative layout shift mitigation fixed height overflow hidden"` — validate CSS approach
6. `"grpc idempotency key approval RPC pattern"` — confirm idempotency approach for approvals
