# Research Synthesis: Session State Visibility & Triage UX

Created: 2026-04-14
Sources: findings-stack.md, findings-features.md, findings-architecture.md, findings-pitfalls.md

## Decision Required

How to surface rich session state to users (terminal snapshot, accurate status, non-interrupting approval flow) using the existing streaming and detection infrastructure already in the codebase.

## Context

Research revealed the codebase is significantly more built-out than the problem statement implied. Three key discoveries change the implementation scope:

1. **Status detection is complete** — `session/detection/detector.go` has a full 10-state `DetectedStatus` (StatusUnknown, StatusReady, StatusProcessing, StatusNeedsApproval, StatusInputRequired, StatusError, StatusTestsFailing, StatusIdle, StatusActive, StatusSuccess) with YAML-configurable patterns and compiled regex cache. It runs today. It is just not wired to the frontend.

2. **Terminal snapshot infrastructure exists** — `@xterm/addon-serialize` is already in `package.json`. `ScrollbackRequest`/`ScrollbackResponse` messages are already in `events.proto`. The backend already has `CurrentPaneRequest`/`CurrentPaneResponse` for capturing last N lines of tmux pane output with optional ANSI escape codes. `ansi-to-html@0.7.2` is already in `package.json`.

3. **Review queue reconnect is already partially handled** — `ReviewQueueItemAddedEvent.is_snapshot = true` exists specifically to prevent duplicate notifications on reconnect. But `WatchSessions` (which drives session card status) does **not** have an equivalent mechanism. This is where the P0 stale-status-after-reconnect issue lives.

The scope shifts from "build new infrastructure" to "wire existing infrastructure + fix two streaming bugs + replace a modal with a drawer."

## Options Considered

### Status propagation

| Option | Latency | Integration Cost | Risk |
|---|---|---|---|
| Add `detected_status` + `detected_context` to `SessionStatusChangedEvent` | <100ms | Low (proto extension + Go wiring) | Low (optional fields, backward-compat) |
| Add fields to Session proto (field 42+) | <100ms | Low (Session already in SessionUpdatedEvent) | Low |
| REST polling | ~1s | Medium | Medium (architectural mismatch) |

### Terminal snapshot rendering

| Option | Bundle cost | Memory/card | Scroll perf (20+ cards) | Integration cost |
|---|---|---|---|---|
| `ansi-to-html` (already installed) | 0 (in deps) | ~10 KB | 60 FPS (HTML DOM) | Trivial |
| `@xterm/xterm` per card | +280 KB | 500 KB+ | 15-30 FPS (20× canvas) | High |
| Pre-rendered HTML from backend | 0 | ~10 KB | 60 FPS | Medium (Go ANSI→HTML) |

### Snapshot delivery

| Option | Notes |
|---|---|
| `CurrentPaneResponse` via `StreamTerminal` | Already exists. Sends last N lines of tmux pane with ANSI codes. Can be requested once (not streamed). |
| `ScrollbackResponse` | Already exists. More flexible (by sequence number). |
| New snapshot RPC | Unnecessary — infrastructure exists. |

### Approval UX

| Option | Interruptiveness | Discoverability | Preserves context |
|---|---|---|---|
| Current modal | 5/5 (full interrupt) | 5/5 | 0/5 |
| Side drawer (non-modal) | 1/5 | 4/5 | 5/5 |
| Badge + dedicated queue page | 1/5 | 3/5 | 5/5 |
| Toast + persistent tray | 2/5 | 4/5 | 5/5 |

## Dominant Trade-off

**Minimal new code vs. maximum impact**. The infrastructure exists. The dominant tension is between:
- "Wire it directly" (use existing `CurrentPaneRequest` for snapshots, extend the existing `SessionStatusChangedEvent`) which is fast but requires careful sequencing
- "Build it right" (new snapshot RPC, clean separation) which is slower but more maintainable

Given that requirements include both accuracy and UX, **wire existing infrastructure first, fix the two P0 streaming bugs, then make the approval flow non-interrupting**. The stack recommendation is settled.

## Recommendation

**Choose: Wire `DetectedStatus` through existing streaming + `CurrentPaneResponse` for snapshots + side drawer for approvals**

### Specific decisions:

**1. Status**: Add `detected_status` (string matching `DetectedStatus` constant names) + `detected_context` (human description) to `SessionStatusChangedEvent` in `events.proto` (fields 4 and 5). Wire `InstanceStatusManager` to call `detector.DetectWithContext()` on new scrollback, debounce 200ms, emit only on state change. Display `detected_context` in `SessionCard` below the enum status badge.

**2. Terminal snapshot**: Use `CurrentPaneRequest` (already in `StreamTerminal`) to fetch last 20 lines with `include_escapes: true`. Render with `ansi-to-html` (already installed). Cache rendered HTML in React state with 5-second TTL. Show in `SessionCard` as a fixed-height (120px, overflow:hidden) preview pane, on-hover or as a collapsible section.

**3. Streaming reconnect fix (P0)**: On `WatchSessions` reconnect, call `ListSessions()` before re-subscribing to populate full current state. Alternatively, add `is_initial_snapshot` flag to `SessionUpdatedEvent` (mirrors the pattern already used in `ReviewQueueItemAddedEvent.is_snapshot`). Fix the staleness indicator: track `lastEventTime`; if `Date.now() - lastEventTime > 15_000ms`, show "status stale" badge on all session cards.

**4. Approval UX**: Replace the existing `ApprovalPanel` modal with a right-side non-modal drawer. Add a badge in the top navigation showing pending approval count. Toast notification (30s auto-dismiss or on action) on `NotificationEvent` arrival of type `NEEDS_APPROVAL`. Approval protocol (HTTP blocking + `ApprovalStore`) unchanged — only the UI layer changes.

**5. Stable React keys (P1 pitfall fix)**: `Session.id` is currently the session title (see `types.proto` comment: "Unique identifier (uses title as ID for now)"). If title is used as React key, renaming causes key thrashing. Either use a stable hash of title, or fix the ID to be immutable — the right fix is to generate a UUID at creation time and use it as `id`.

### Because:

The `DetectedStatus` detector is production-quality (YAML-configurable, compiled regex, 9 test files). Wiring it takes hours, not days. The `CurrentPaneRequest` message already supports `lines` count, `include_escapes`, and `target_cols/rows` for correct terminal width rendering — it was designed for exactly this use case. Using `ansi-to-html` (6KB, already installed) vs. xterm.js per card (280KB, 500KB memory) is a decisive performance advantage with minimal fidelity loss (cursor movement codes dropped, which is irrelevant for read-only snapshots).

### Accept these costs:
- 5-second snapshot staleness (vs. real-time terminal streaming) — acceptable since full real-time view is one click away
- Cursor movement codes dropped in snapshot (progress bars may look slightly off) — acceptable for triage use case
- Approval drawer requires 2-3 clicks vs. 1-click modal — acceptable since the current modal steals focus

### Reject these alternatives:
- **xterm.js per session card**: 20 canvas instances at 500KB each = 10MB memory, 15-30 FPS scroll. Decisive perf loss.
- **Pre-rendering ANSI→HTML on backend**: Adds Go-side HTML generation complexity with no UX benefit over client-side `ansi-to-html`.
- **New snapshot RPC**: `CurrentPaneRequest` already handles this use case with more options.
- **Keep modal approval**: Directly contradicts the primary requirement (non-interrupting approvals).
- **REST polling for status**: Architectural mismatch with ConnectRPC-only codebase; higher latency.

## Open Questions Before Committing

- [ ] **Session ID stability**: Is `Session.id = session.title` a known limitation, or is there a stable UUID generated at creation that's just not exposed in proto? — check `session/instance.go`. This affects whether key thrashing is a current bug or a future risk.
- [ ] **`CurrentPaneRequest` call from web UI**: Can it be called outside of an active `StreamTerminal` session, or does it require an open terminal stream? — determines whether snapshot fetch needs a separate mechanism.
- [ ] **`InstanceStatusManager` location and interface**: Confirm where in `session_service.go` it lives and whether adding `OnPatternDetected()` is straightforward.
- [ ] **Approval expiry**: Is `PendingApproval.ExpiresAt` populated and accurate? — determines whether expiry check is a code change or just enabling existing logic.

## Sources

- `project_plans/visualize-state/research/findings-stack.md` — ANSI rendering and snapshot delivery options
- `project_plans/visualize-state/research/findings-features.md` — CI dashboard and approval UX patterns
- `project_plans/visualize-state/research/findings-architecture.md` — codebase analysis, status wiring patterns
- `project_plans/visualize-state/research/findings-pitfalls.md` — failure modes, P0 streaming bugs, mitigation priorities
- Direct codebase review: `proto/session/v1/events.proto`, `proto/session/v1/types.proto`, `session/detection/detector.go`, `web-app/package.json`
