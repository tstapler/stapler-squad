# Findings: Architecture — Status Wiring & Approval Flow

Created: 2026-04-14

## Summary

The codebase has two distinct wiring gaps: (1) Backend pattern-detected terminal states (10-state `DetectedStatus` from `session/detection/detector.go`) are computed but **never propagated to clients** — the frontend sees only a coarse 7-state enum. (2) The approval flow steals focus with a modal that pops over the current session view, blocking work. Both problems are solvable with additive changes: extend `SessionStatusChangedEvent` proto to carry detected status + context string, wire `InstanceStatusManager` to emit it, and replace the modal `ApprovalPanel` with a non-modal side drawer + toast.

**Dominant trade-off**: Schema change cost vs. information richness. Extending the proto is minimally disruptive (optional fields, old clients ignore), but requires `make generate-proto` and careful backward-compat testing. The payoff is immediately surfacing already-computed rich state to users.

## Current State (from codebase analysis)

### Status Layers

**Enum-based status** (what frontend sees today):
- `session/instance.go`: `Status` enum — Running, Ready, Loading, Paused, NeedsApproval, Creating, Stopped (7 states)
- `proto/session/v1/`: `SessionStatus` enum — `SESSION_STATUS_RUNNING`, etc.
- `SessionStatusChangedEvent` carries only `old_status` + `new_status` (both enum)

**Detected status** (what backend computes but never sends):
- `session/detection/detector.go`: `PatternDetector` — analyzes ANSI-stripped terminal output via priority-ordered regex
- Returns `DetectedStatus`: StatusUnknown, StatusReady, StatusProcessing, StatusNeedsApproval, StatusInputRequired, StatusError, StatusTestsFailing, StatusIdle, StatusActive, StatusSuccess (10 states)
- Priority: Error > TestsFailing > Success > NeedsApproval > InputRequired > Active > Processing > Idle > Ready
- `DetectWithContext()` returns status + human-readable description string
- **Gap**: Detector output is never written to Session model or emitted to clients

### Current Streaming Architecture

**WatchSessions stream**: `WatchSessions(WatchSessionsRequest) → stream SessionEvent`
- `SessionEvent` is a oneof: SessionCreatedEvent, SessionUpdatedEvent, SessionDeletedEvent, SessionStatusChangedEvent, UserInteractionEvent, ApprovalResponseEvent, NotificationEvent
- `SessionUpdatedEvent` carries full Session message + `updated_fields` array
- Frontend already merges full Session on `SessionUpdatedEvent` — adding new proto fields will flow through automatically

**Terminal streaming**: `StreamTerminal` handles PTY I/O separately. No status or detection data attached.

**InstanceStatusManager**: Already exists in `session_service.go:40`. Manages status transitions. The right place to wire pattern detection.

### Current Approval Flow

**Backend** (`ApprovalHandler`):
- HTTP hook blocks waiting for `ApprovalStore.Resolve()` — up to 4-minute timeout
- Stores pending approval in `~/.stapler-squad/config/pending_approvals.json`
- Broadcasts `NotificationEvent` to clients via `WatchSessions` stream
- Handler unblocks and returns decision JSON to Claude when resolved

**Frontend** (`ApprovalPanel`, `ApprovalCard`):
- Modal-based UI appears as overlay
- `ListPendingApprovals` RPC fetches approvals on mount
- `WatchReviewQueue` streams review queue events (separate from approvals)
- Approval pops as full-width overlay — steals focus even while user is actively working elsewhere
- No badge/indicator on session cards — user must notice the approval panel appeared

## Options Surveyed

### 1. Status Propagation: Push vs. Pull vs. Event-Driven

**Option A: REST Polling** — Frontend periodically calls `GetSession`. High latency, thundering herd with many clients. ❌

**Option B: Push via periodic status updates** — Server runs detector on schedule, emits `SessionUpdatedEvent`. Simple but wastes compute with no subscribers.

**Option C: Event-driven `StatusChangedEvent` extension (Recommended)** — Extend `SessionStatusChangedEvent` to add `detected_status` (new enum or string) + `detected_context` (human description). Emit only when detected state changes. Integrates with existing event stream.

**Option D: Hybrid — add fields to Session proto** — Add `detected_status` + `detected_context` to Session message itself. `SessionUpdatedEvent` (which carries full Session) propagates automatically. Most backward-compatible.

**Recommendation**: Option C + D combined — extend both the event and the Session model. Additive, backward-compatible, integrates with existing streaming.

### 2. React State Management

**useState + useEffect polling** — Simple but polling-based. Inefficient. ❌

**React Query** — Not in codebase yet. Still polling-based.

**Zustand** — Not in codebase. Would require new dependency and migration.

**Redux Toolkit (already used — ADR-008) (Recommended)** — Create `sessionSlice` with `extraReducers` handling `SessionUpdatedEvent`. Add WatchSessions listener middleware that dispatches `updateSession(payload)` on each event. Consistent with existing architecture.

### 3. Non-Interrupting Approval Flow Patterns

**Option A: Global notification tray** — Persistent queue in nav; badge count; user drains when ready. Non-blocking, but user may not notice.

**Option B: Floating badge/FAB** — Small corner button with approval count. Can get lost.

**Option C: Inline approval on session card** — Approval badge on card; expand in-place. Good for triage but requires user to find the right card.

**Option D: Side drawer (Recommended)** — Approval panel slides in from right; doesn't overlay content; main session list visible behind it. `onClick` outside drawer doesn't auto-close (intentional — prevents accidental dismissal). Only X button or approve/deny closes.

**Option E: Toast + persistent tray** — Brief toast with Approve/Deny buttons; also persists in notification history.

**Recommendation**: Option D (side drawer) + Option E toast notification on arrival + Option A persistent tray for history. Drawer for active approval; tray for audit trail.

### 4. Go Backend Status Emission

**Per-session status channel** — Buffered chan per instance. Requires fan-out to broadcast.

**EventBus publish** — Reuse existing `events.EventBus`. Heavy bus already used; needs careful topic routing.

**InstanceStatusManager (Recommended)** — Already exists. Add `OnPatternDetected(ctx, sessionID, detected, context)` method. Manager emits `SessionStatusChangedEvent` to subscribers. Debounce 200ms to prevent spam on high-frequency output.

**Scrollback callback** — Extra indirection via `OnStatus` callback. Decoupled but dated pattern.

### 5. Approval Unblocking

Keep HTTP blocking (`ApprovalHandler` blocks on `ApprovalStore.Resolve()` channel). The problem is UI, not protocol. Non-modal drawer solves the UX issue without changing the Claude integration contract. 4-minute timeout is documented and acceptable.

## Trade-off Matrix

| Approach | Status Latency | Complexity | Backward Compat | UX Impact | Effort |
|---|---|---|---|---|---|
| Status event extension (C+D) | <100ms | Medium | High (optional fields) | Good (real-time) | 4-6h |
| Redux streaming middleware | <100ms | Low | High | Good | 3-4h |
| Side drawer approval | <10ms feel | Low | N/A (UI only) | Excellent | 2-3h |
| Toast + persistent tray | <10ms feel | Medium | N/A | Excellent | 4-5h |
| InstanceStatusManager wiring | <500ms | Low | High | Good | 2-3h |

## Risk and Failure Modes

**Status pattern mismatch** — Frontend shows "Active" but session is actually paused.
- Cause: Stale scrollback; detector runs on old data.
- Mitigation: Always send timestamp with detected status; frontend age-checks and shows "stale" indicator after 30s.

**Approval times out without user knowing** — 4-minute timeout elapses silently; Claude gets error.
- Mitigation: Toast stays on screen until dismissed; show countdown timer ("Expires in 2:45"); notification tray shows "timed out" entry.

**Pattern detector false positives** — Regex matches prose (e.g., "Error: " in normal log output).
- Cause: Over-broad patterns.
- Mitigation: Existing detector already ANSI-strips and uses priority ordering. Add snapshot tests against real Claude/Aider output logs.

**WebSocket spam on rapid output** — Detector runs every scrollback line; emits event for each status change (Ready → Active → Idle → Ready in 200ms).
- Mitigation: Debounce at 200ms in `InstanceStatusManager`; only emit when detected state changes (compare to last emitted).

**Client-side state divergence** — Backend detects Active, but client cached Ready from old update.
- Mitigation: `SessionUpdatedEvent` carries full Session; client always full-merges, not incremental patches.

**Double-approval race condition** — User approves via drawer AND via terminal simultaneously.
- Mitigation: Disable approval buttons after click (optimistic UI); handle "already resolved" error gracefully; add idempotency key to ResolveApproval RPC.

## Migration and Adoption Cost

**Phase 1 — Backend proto + InstanceStatusManager wiring (4-6h)**
- Add `detected_status` (string or enum) + `detected_context` to `SessionStatusChangedEvent` and Session message
- Add `OnPatternDetected()` to `InstanceStatusManager`
- Wire scrollback watcher → `manager.OnPatternDetected()`
- Debounce at 200ms; only emit on state change
- Run `make generate-proto`
- Risk: Low (additive; old clients ignore new fields)

**Phase 2 — Frontend status display (1-2h)**
- `SessionCard` reads `detected_context` from Session if present
- Display below enum status: "Running → Executing code" or "Ready → Waiting for your input"
- Risk: Very low (read-only display)

**Phase 3 — Approval UI redesign (4-6h)**
- Replace modal `ApprovalPanel` with side drawer (CSS + component refactor)
- Add toast notification on `NotificationEvent` arrival (auto-dismiss 30s or on action)
- Add notification tray (history list) accessible from header badge
- Show countdown timer on each pending approval
- Risk: Medium (UX touch; test on multiple screen sizes)

**Phase 4 — Redux streaming middleware (2-3h, optional)**
- Create `sessionSlice` with `updateSession` action
- Add WatchSessions listener middleware dispatching on each `SessionUpdatedEvent`
- Risk: Low (isolated slice; leverages ADR-008)

**Total**: 11-17h. Phases 1 and 2 can run in parallel with Phase 3.

**Rollback**: All changes are additive. Old proto fields ignored by old clients. UI changes are isolated to ApprovalPanel component — can feature-flag behind `APPROVAL_DRAWER_ENABLED`.

## Operational Concerns

**Observability**:
- Log `InstanceStatusManager.OnPatternDetected()` calls with pattern name matched (e.g., "matched esc_to_interrupt")
- Metrics: `status_change_count{detected_status=...}`, `approval_resolve_latency_ms`
- Add debug endpoint: `ExportDetectedPatterns(sessionID) → YAML` — shows recent pattern matches

**Performance**:
- Pattern detector is CPU-bound regex; currently on ~2KB scrollback per event
- Concern: High-frequency output (progress bars) may spike CPU
- Mitigation: Debounce to max once per 200ms; only run on last 100 lines of scrollback

**Backward compatibility**:
- Optional proto fields: old clients ignore; old servers send empty fields → frontend falls back to enum status
- Approval schema: additive new proto fields; old `ApprovalStore` JSON format preserved

## Prior Art and Lessons Learned

**From codebase ADRs**:
- ADR-007 (Enum-Based State Transitions): Apply same enum-first pattern to detected status in proto
- ADR-008 (Redux Toolkit): Streaming middleware dispatching `updateSession` is the natural extension
- ADR-006 (Async Event Loop): `EventBus` and `InstanceStatusManager` are the right emitters

**From industry** [TRAINING_ONLY - verify]:
- VSCode/JetBrains status bars: Update on event (not poll); never modal; pattern of status intent + status context is universal
- tmux status line: Status never blocks user action — always visible but never intrusive
- GitHub Actions: Approval on workflow page (not global interrupt) reduces context-switching
- HTTP blocking for approval: Slack and GitHub both use blocking approach for webhook approvals; UX concern is entirely in the notification layer, not the protocol

**Lessons**:
- Status should be event-driven and never modal-blocking
- Pattern matching requires precision + test coverage against real output
- Separate "session lifecycle" (enum) from "current activity" (pattern-detected context) — both are valuable
- Approval blocking protocol is fine; fix the notification UI, not the protocol

## Open Questions

- [ ] Debounce interval: 200ms or 500ms? Profile with real high-frequency sessions (progress bars). — blocks backend implementation
- [ ] Detected status: Separate proto enum or plain string? Enum gives type safety; string gives extensibility for YAML-configured patterns. — blocks proto design
- [ ] Approval countdown: Show `seconds_remaining` in ApprovalCard? Requires backend to send expiry timestamp. — blocks Phase 3
- [ ] Multiple pending approvals: Show all in drawer list or just most urgent? — blocks drawer UX design

## Recommendation

**Primary architecture**:
1. **Backend**: Extend `SessionStatusChangedEvent` + Session proto with `detected_status` + `detected_context`. Wire `InstanceStatusManager.OnPatternDetected()` to scrollback watcher. Debounce 200ms. Emit only on state change.
2. **Frontend**: Redux `sessionSlice` + WatchSessions middleware dispatching `updateSession`. `SessionCard` displays `detected_context` below enum status.
3. **Approval**: Side drawer (non-modal) + arrival toast (30s auto-dismiss) + persistent notification tray with history. Keep HTTP blocking protocol unchanged.
4. **Observability**: Pattern match logging, approval latency metric, debug export endpoint.

**Conditions that would change this recommendation**: If the pattern detector needs per-user customizable patterns (not just YAML config), a more flexible string-based detected_status field is preferable to an enum. If the session count scales beyond 100, viewport culling on snapshot subscriptions becomes required before Phase 1.

## Pending Web Searches

1. `"protobuf optional fields backward compatibility oneof evolution"` — verify proto extension approach
2. `"Redux middleware WebSocket streaming real-time updates React 2024"` — confirm streaming middleware pattern
3. `"non-modal side drawer React accessibility focus management"` — verify drawer doesn't trap keyboard focus
4. `"terminal pattern detection false positive rate ANSI stripping"` — validate existing detector approach
5. `"HTTP long-polling approval webhook Claude tool use timeout"` — confirm 4min timeout is standard
