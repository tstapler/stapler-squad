# Feature Plan: External Claude Process PTY Interception

## Research Summary

### Teeterm Analysis
[github.com/kcghost/teeterm](https://github.com/kcghost/teeterm) is a C utility that splits I/O of one process into two pseudoterminals (PTYs).

**How it works:**
- Creates two PTYs that simultaneously receive input/output from a launched process
- Generates symlinks `pty0` and `pty1` pointing to these pseudoterminals
- Multiple terminal sessions can interact with the same process

**Limitations:**
- Requires launching the process through teeterm (cannot attach to existing processes)
- Manual invocation required for each process
- Not suitable for discovering/attaching to already-running Claude instances

### Alternative Techniques Evaluated

| Technique | Invasiveness | Works with Running Processes | Read-Only | Cross-Platform |
|-----------|-------------|------------------------------|-----------|----------------|
| **tmux capture-pane** | None | Yes (tmux only) | Yes | Yes |
| **tmux pipe-pane** | None | Yes (tmux only) | Yes | Yes |
| **reptyr (ptrace)** | High | Yes | No (steals PTY) | Linux only |
| **strace -e write** | Medium | Yes | Yes | Linux only |
| **/proc/PID/fd** | Low | Yes | Yes (read) | Linux only |
| **lsof + pty read** | Low | Yes | Yes | macOS/Linux |
| **SCM_RIGHTS FD passing** | None | Cooperative only | N/A | Unix |

### Recommended Approach

A **hybrid multi-tier approach** that leverages existing infrastructure:

1. **Tier 1: tmux-based discovery** (already implemented)
   - Use existing `PTYDiscovery` for tmux sessions
   - Leverage `capture-pane` and `pipe-pane` for streaming

2. **Tier 2: Non-tmux Claude process discovery**
   - Use `ps` + `pgrep` to find Claude processes
   - Parse `/proc/PID/fd` (Linux) or `lsof` (macOS) to find PTY devices

3. **Tier 3: Real-time PTY observation**
   - Use `strace -e write -p PID` for Linux output capture
   - Use tmux `pipe-pane` for streaming to Unix domain socket

---

## Architecture Decision Records

### ADR-001: Use tmux as Primary PTY Interception Layer

**Context:** We need to capture terminal output from external Claude processes without modifying them or requiring special launch procedures.

**Decision:** Use tmux as the primary interception layer for all Claude sessions, including external ones we "adopt."

**Rationale:**
- Claude-squad already has robust tmux integration
- tmux provides `capture-pane` (snapshot) and `pipe-pane` (streaming)
- Cross-platform (macOS, Linux, BSD)
- No ptrace required (avoids security restrictions)
- Non-invasive to target process

**Consequences:**
- External non-tmux Claude processes require "adoption" into a tmux session
- Cannot observe processes that refuse tmux adoption
- Simpler implementation than direct PTY interception

**Patterns Applied:** Adapter Pattern, Strategy Pattern

### ADR-002: Implement Discovery Tiers for Process Detection

**Context:** External Claude processes may be running in various contexts (terminal, tmux, screen, bare shell).

**Decision:** Implement a three-tier discovery system:
1. **Managed Sessions:** Squad-managed tmux sessions (existing)
2. **Tmux External:** Non-squad tmux sessions running Claude
3. **Bare Processes:** Claude processes not in tmux (candidates for adoption)

**Rationale:**
- Existing code handles Tier 1
- Tier 2 extends existing PTYDiscovery patterns
- Tier 3 enables future "adopt into tmux" functionality

**Consequences:**
- Clear separation of discovery modes
- Progressive enhancement possible
- Bare process adoption is optional/advanced feature

**Patterns Applied:** Strategy Pattern, Chain of Responsibility

### ADR-003: Unix Domain Socket for PTY Streaming

**Context:** Need real-time terminal streaming from discovered sessions to web browser.

**Decision:** Use Unix domain sockets with tmux `pipe-pane` for streaming, then relay to WebSocket.

**Rationale:**
- tmux `pipe-pane` can output to any command (including socat/nc)
- Unix sockets are fast and local-only (secure)
- Easy to bridge to existing WebSocket infrastructure
- Matches existing `terminal_websocket.go` patterns

**Consequences:**
- Additional socket management required
- Cleanup on session end needed
- Works well with existing WebSocket handler

**Patterns Applied:** Observer Pattern, Proxy Pattern

---

## Epic Overview

### User Value
Enable users to discover, monitor, and interact with external Claude Code processes running on their machine through the stapler-squad web interface, without requiring those sessions to be started by stapler-squad.

### Success Metrics
- Discover 100% of Claude processes running in tmux sessions (same user)
- Display real-time terminal output in browser with <100ms latency
- Trigger notifications when external sessions need approval
- Zero impact on external process performance

### Scope

**In Scope:**
- Discover external Claude processes in tmux sessions
- View terminal output in web UI (read-only initially)
- Detect approval prompts and trigger notifications
- "Adopt" external tmux sessions into squad management

**Out of Scope (Phase 1):**
- Non-tmux bare process interception (strace/ptrace)
- Input injection to external processes
- Cross-user process discovery
- Windows support

### Constraints
- Must work on macOS and Linux
- Cannot require root/sudo for normal operations
- Must not impact target process stability
- Same-user security boundary only

---

## Story Breakdown

### Story 1: Enhanced External tmux Session Discovery [1 week]

**User Value:** Users can see all Claude instances running in any tmux session on their machine.

**Acceptance Criteria:**
- [ ] Discover Claude processes in default tmux server
- [ ] Discover Claude processes in named tmux servers
- [ ] Display external sessions in web UI with "External" badge
- [ ] Filter external sessions separately from managed sessions
- [ ] Refresh discovery every 5 seconds

**Tasks:**

#### Task 1.1: Extend PTYDiscovery for comprehensive tmux scanning [3h]

**Objective:** Enhance the existing `discoverExternalClaude` to scan all tmux servers.

**Context Boundary:**
- Files: `session/pty_discovery.go`, `session/types.go` (2 files)
- Lines: ~400 lines
- Concepts: tmux socket discovery, process detection

**Prerequisites:**
- Understanding of existing PTYDiscovery implementation
- Knowledge of tmux server socket locations

**Implementation Approach:**
1. Add function to discover all tmux server sockets (`/tmp/tmux-*/default`, etc.)
2. Extend `discoverExternalClaude` to iterate over discovered sockets
3. Add socket auto-discovery option to `PTYDiscoveryConfig`
4. Update refresh mechanism to scan all discovered sockets

**Validation Strategy:**
- Unit tests: Mock tmux command outputs for various socket configurations
- Integration tests: Create tmux sessions on different sockets and verify discovery
- Success criteria: Discovers Claude in default and custom tmux servers

**INVEST Check:**
- Independent: No external dependencies
- Negotiable: Socket discovery patterns can be adjusted
- Valuable: Foundation for external session visibility
- Estimable: 3 hours with high confidence
- Small: Single responsibility (socket scanning)
- Testable: Mock-based unit tests possible

#### Task 1.2: Create ExternalSession protobuf type and API [2h]

**Objective:** Define protobuf messages for external session representation.

**Context Boundary:**
- Files: `proto/session/v1/session.proto`, `proto/session/v1/types.proto` (2 files)
- Lines: ~200 lines
- Concepts: Protobuf schema design, session types

**Prerequisites:**
- Understanding of existing session protobuf schema
- Knowledge of PTYConnection struct

**Implementation Approach:**
1. Add `ExternalSession` message type to proto
2. Add `source` field (managed/external/orphaned) to Session message
3. Add `external_metadata` for tmux socket, original session name
4. Generate Go code with `buf generate`

**Validation Strategy:**
- Unit tests: Proto serialization/deserialization
- Integration tests: API returns correctly typed sessions
- Success criteria: External sessions distinguishable via API

#### Task 1.3: Update SessionService to include external sessions [3h]

**Objective:** Modify session service to return both managed and external sessions.

**Context Boundary:**
- Files: `server/services/session_service.go`, `server/adapters/instance_adapter.go` (2 files)
- Lines: ~500 lines
- Concepts: Service layer, data transformation

**Prerequisites:**
- Task 1.1 and 1.2 completed
- Understanding of existing SessionService

**Implementation Approach:**
1. Inject PTYDiscovery into SessionService
2. Merge discovered external sessions with managed sessions
3. Add source field to session response
4. Add filtering option for source type

**Validation Strategy:**
- Unit tests: Mock PTYDiscovery, verify merged results
- Integration tests: API returns both session types
- Success criteria: ListSessions includes external sessions

#### Task 1.4: Update web UI to display external sessions [2h]

**Objective:** Show external sessions in session list with visual distinction.

**Context Boundary:**
- Files: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/lib/hooks/useSessionService.ts` (2 files)
- Lines: ~300 lines
- Concepts: React components, session display

**Prerequisites:**
- Task 1.3 completed
- Understanding of existing SessionCard component

**Implementation Approach:**
1. Add "External" badge to SessionCard for external sessions
2. Add filter toggle for external sessions
3. Update session list grouping to separate external sessions
4. Style external sessions with distinct visual treatment

**Validation Strategy:**
- Visual verification: Badge appears on external sessions
- E2E tests: Filter toggle works correctly
- Success criteria: Users can distinguish external from managed sessions

---

### Story 2: Real-time Terminal Streaming for External Sessions [1.5 weeks]

**User Value:** Users can view live terminal output from external Claude sessions in the browser.

**Acceptance Criteria:**
- [ ] Stream terminal output from external tmux sessions
- [ ] Display in existing terminal viewer component
- [ ] Latency under 100ms for terminal updates
- [ ] Handle session disconnect gracefully
- [ ] Support multiple concurrent external session streams

**Tasks:**

#### Task 2.1: Implement tmux pipe-pane streaming wrapper [4h]

**Objective:** Create a Go wrapper for tmux pipe-pane that streams output to a channel.

**Context Boundary:**
- Files: `session/tmux/pipe_stream.go` (new), `session/tmux/tmux.go` (2 files)
- Lines: ~350 lines
- Concepts: tmux pipe-pane, Go channels, process management

**Prerequisites:**
- Understanding of tmux pipe-pane command
- Knowledge of Go channel patterns

**Implementation Approach:**
1. Create `PipeStream` struct with output channel
2. Implement `StartPipeStream(sessionName, socket string) (*PipeStream, error)`
3. Use `tmux pipe-pane -t session 'cat >> /tmp/socket'` pattern
4. Read from socket and push to channel
5. Implement graceful cleanup on stop

**Validation Strategy:**
- Unit tests: Mock tmux commands, verify channel output
- Integration tests: Real tmux session streaming
- Success criteria: Terminal output arrives within 100ms

#### Task 2.2: Create ExternalSessionStreamer service [3h]

**Objective:** Service that manages streaming for multiple external sessions.

**Context Boundary:**
- Files: `session/external_streamer.go` (new), `session/pty_discovery.go` (2 files)
- Lines: ~400 lines
- Concepts: Service management, concurrent streams

**Prerequisites:**
- Task 2.1 completed
- Understanding of PTYDiscovery refresh cycle

**Implementation Approach:**
1. Create `ExternalSessionStreamer` struct with stream registry
2. Implement `StartStream(sessionID string) (chan []byte, error)`
3. Implement `StopStream(sessionID string)`
4. Auto-cleanup streams for disconnected sessions
5. Integration with PTYDiscovery refresh

**Validation Strategy:**
- Unit tests: Stream lifecycle management
- Integration tests: Multiple concurrent streams
- Success criteria: Manages 10+ concurrent streams without issues

#### Task 2.3: Add WebSocket endpoint for external session streaming [3h]

**Objective:** WebSocket handler that streams external session output.

**Context Boundary:**
- Files: `server/services/terminal_websocket.go`, `server/server.go` (2 files)
- Lines: ~350 lines
- Concepts: WebSocket, external session handling

**Prerequisites:**
- Task 2.2 completed
- Understanding of existing TerminalWebSocketHandler

**Implementation Approach:**
1. Extend `HandleWebSocket` to detect external sessions
2. Use `ExternalSessionStreamer` for external session PTY access
3. Add session source detection logic
4. Handle read-only mode for external sessions (no input forwarding initially)

**Validation Strategy:**
- Unit tests: WebSocket upgrade and message flow
- E2E tests: Browser receives external session output
- Success criteria: Real-time streaming works in browser

#### Task 2.4: Update web terminal component for external sessions [2h]

**Objective:** Terminal component handles external session streaming.

**Context Boundary:**
- Files: `web-app/src/components/sessions/TerminalOutput.tsx`, `web-app/src/components/sessions/SessionDetail.tsx` (2 files)
- Lines: ~300 lines
- Concepts: xterm.js, WebSocket handling

**Prerequisites:**
- Task 2.3 completed
- Understanding of existing TerminalOutput component

**Implementation Approach:**
1. Add "External Session (Read-Only)" indicator
2. Disable input for external sessions
3. Handle connection/disconnection states
4. Add visual indicator for streaming status

**Validation Strategy:**
- Visual verification: External sessions stream correctly
- E2E tests: Terminal displays real-time output
- Success criteria: Users can watch external Claude sessions

---

### Story 3: Approval Detection and Notifications for External Sessions [1 week]

**User Value:** Users receive notifications when external Claude sessions need approval, matching behavior of managed sessions.

**Acceptance Criteria:**
- [ ] Detect approval prompts in external session output
- [ ] Trigger notification via existing notification system
- [ ] Add external sessions to review queue
- [ ] Support browser notifications for external approvals

**Tasks:**

#### Task 3.1: Extend approval detection for external sessions [3h]

**Objective:** Apply existing approval detection to external session streams.

**Context Boundary:**
- Files: `session/approval_detector.go`, `session/external_streamer.go` (2 files)
- Lines: ~350 lines
- Concepts: Pattern detection, stream processing

**Prerequisites:**
- Task 2.2 completed
- Understanding of existing approval detection

**Implementation Approach:**
1. Add stream processor to `ExternalSessionStreamer`
2. Apply `detectPromptInContent` to streamed output
3. Emit event when approval detected
4. Track approval state per external session

**Validation Strategy:**
- Unit tests: Approval pattern detection in streams
- Integration tests: Event emission on approval detection
- Success criteria: External approvals detected reliably

#### Task 3.2: Add external sessions to review queue [2h]

**Objective:** External sessions with approvals appear in review queue.

**Context Boundary:**
- Files: `session/review_queue.go`, `session/review_queue_poller.go` (2 files)
- Lines: ~300 lines
- Concepts: Review queue, external session integration

**Prerequisites:**
- Task 3.1 completed
- Understanding of review queue implementation

**Implementation Approach:**
1. Extend review queue to accept external session entries
2. Add source field to review queue items
3. Update poller to include external session approvals
4. Handle external session dismissal differently (no auto-respond)

**Validation Strategy:**
- Unit tests: External sessions in queue
- Integration tests: Queue updates on external approvals
- Success criteria: External approvals visible in review queue

#### Task 3.3: Trigger notifications for external session approvals [2h]

**Objective:** Notification system alerts users to external session approvals.

**Context Boundary:**
- Files: `server/events/types.go`, `server/services/session_service.go` (2 files)
- Lines: ~250 lines
- Concepts: Event system, notifications

**Prerequisites:**
- Task 3.2 completed
- Understanding of event/notification system

**Implementation Approach:**
1. Create notification event for external approval
2. Add metadata indicating external source
3. Emit event through EventBus
4. Update notification priority for external sessions

**Validation Strategy:**
- Unit tests: Notification event creation
- E2E tests: Browser notification appears
- Success criteria: Users notified of external approvals

---

### Story 4: Session Adoption (External to Managed) [1 week]

**User Value:** Users can "adopt" external Claude sessions to gain full management capabilities.

**Acceptance Criteria:**
- [ ] "Adopt" button on external sessions
- [ ] Adoption creates managed session wrapper
- [ ] Adopted sessions gain full interaction capabilities
- [ ] Original external session continues to work

**Tasks:**

#### Task 4.1: Design adoption data model [2h]

**Objective:** Define how adopted sessions are represented and stored.

**Context Boundary:**
- Files: `session/types.go`, `session/instance.go` (2 files)
- Lines: ~300 lines
- Concepts: Data modeling, session lifecycle

**Prerequisites:**
- Understanding of Instance struct
- Knowledge of session persistence

**Implementation Approach:**
1. Add `IsAdopted` field to Instance
2. Add `OriginalTmuxSession` for adopted sessions
3. Define adoption metadata (original socket, session name)
4. Update session persistence for adopted sessions

**Validation Strategy:**
- Unit tests: Adoption field serialization
- Integration tests: Adopted session persistence
- Success criteria: Adopted sessions survive restart

#### Task 4.2: Implement session adoption service [4h]

**Objective:** Service to adopt external sessions into managed state.

**Context Boundary:**
- Files: `session/adoption_service.go` (new), `session/storage.go` (2 files)
- Lines: ~400 lines
- Concepts: Session management, state transitions

**Prerequisites:**
- Task 4.1 completed
- Understanding of session storage

**Implementation Approach:**
1. Create `AdoptSession(externalSessionID string) (*Instance, error)`
2. Create Instance wrapper around external tmux session
3. Link to original tmux session for PTY access
4. Persist adoption state
5. Update PTYDiscovery to exclude adopted sessions from external list

**Validation Strategy:**
- Unit tests: Adoption flow
- Integration tests: Adopt and manage external session
- Success criteria: Adopted sessions manageable via squad

#### Task 4.3: Add adoption API endpoint [2h]

**Objective:** REST/gRPC endpoint to trigger session adoption.

**Context Boundary:**
- Files: `server/services/session_service.go`, `proto/session/v1/session.proto` (2 files)
- Lines: ~200 lines
- Concepts: API design, service integration

**Prerequisites:**
- Task 4.2 completed
- Understanding of existing session API

**Implementation Approach:**
1. Add `AdoptSession` RPC to SessionService proto
2. Implement handler calling adoption service
3. Return adopted session details
4. Emit session created event for adopted session

**Validation Strategy:**
- Unit tests: API handler logic
- Integration tests: API adoption flow
- Success criteria: API successfully adopts sessions

#### Task 4.4: Add adoption UI [2h]

**Objective:** UI button to adopt external sessions.

**Context Boundary:**
- Files: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionDetail.tsx` (2 files)
- Lines: ~250 lines
- Concepts: React components, API integration

**Prerequisites:**
- Task 4.3 completed
- Understanding of SessionCard component

**Implementation Approach:**
1. Add "Adopt" button to external session cards
2. Call adoption API on button click
3. Update UI to show adopted status
4. Handle adoption errors gracefully

**Validation Strategy:**
- Visual verification: Adopt button appears
- E2E tests: Adoption flow works end-to-end
- Success criteria: Users can adopt external sessions via UI

---

## Known Issues

### BUG-001: Race Condition in External Session Discovery [SEVERITY: Medium]

**Description:** When multiple tmux servers are scanned simultaneously, there's potential for race conditions in the `connections` slice updates.

**Mitigation:**
- Use mutex protection during discovery
- Implement discovery result aggregation pattern

**Files Likely Affected:**
- `session/pty_discovery.go` - discovery loop
- `session/types.go` - config handling

**Prevention Strategy:**
- Add mutex around connections update
- Use sync.Map for concurrent access
- Add unit tests with race detector

**Related Tasks:** Task 1.1

### BUG-002: tmux Socket Permission Issues [SEVERITY: Low]

**Description:** tmux server sockets in `/tmp/tmux-UID/` may have restricted permissions preventing discovery.

**Mitigation:**
- Check socket permissions before attempting connection
- Log warning for inaccessible sockets

**Files Likely Affected:**
- `session/pty_discovery.go` - socket scanning

**Prevention Strategy:**
- Add permission check before socket access
- Graceful degradation for inaccessible sockets

**Related Tasks:** Task 1.1

### BUG-003: Streaming Memory Leak on Session Disconnect [SEVERITY: High]

**Description:** If external session terminates while streaming, pipe-pane resources may not be cleaned up properly.

**Mitigation:**
- Implement robust cleanup on session end detection
- Use context cancellation for goroutine cleanup

**Files Likely Affected:**
- `session/tmux/pipe_stream.go` - stream management
- `session/external_streamer.go` - stream lifecycle

**Prevention Strategy:**
- Implement finalizer pattern
- Add stream health monitoring
- Include cleanup in test teardown

**Related Tasks:** Task 2.1, Task 2.2

### BUG-004: Approval Detection False Positives in Colored Output [SEVERITY: Medium]

**Description:** ANSI color codes may interfere with approval prompt pattern matching.

**Mitigation:**
- Strip ANSI codes before pattern matching
- Use existing `sanitizeUTF8String` function

**Files Likely Affected:**
- `session/approval_detector.go` - pattern matching
- `session/tmux/tmux.go` - content filtering

**Prevention Strategy:**
- Pre-process output through sanitizer
- Add test cases with ANSI-colored prompts

**Related Tasks:** Task 3.1

---

## Dependency Visualization

```
                     Story 1: Discovery
                           │
    ┌──────────────────────┼──────────────────────┐
    │                      │                      │
    ▼                      ▼                      ▼
[Task 1.1]            [Task 1.2]             [Task 1.3]
PTY Discovery    ───► Proto Types    ───►   Session Service
    │                      │                      │
    └──────────────────────┼──────────────────────┘
                           │
                           ▼
                      [Task 1.4]
                      Web UI Display
                           │
                           │
                     Story 2: Streaming
                           │
    ┌──────────────────────┼──────────────────────┐
    │                      │                      │
    ▼                      ▼                      │
[Task 2.1]            [Task 2.2]                  │
Pipe Stream   ────►  External Streamer            │
                           │                      │
                           ▼                      │
                      [Task 2.3]            [Task 2.4]
                   WebSocket Handler ───► Terminal Component
                           │
                           │
                     Story 3: Notifications
                           │
    ┌──────────────────────┼──────────────────────┐
    │                      │                      │
    ▼                      ▼                      ▼
[Task 3.1]            [Task 3.2]             [Task 3.3]
Approval Detection ─► Review Queue ───► Notifications
                           │
                           │
                     Story 4: Adoption
                           │
    ┌──────────────────────┼──────────────────────┐
    │                      │                      │
    ▼                      ▼                      ▼
[Task 4.1]            [Task 4.2]             [Task 4.3]
Data Model    ────►  Adoption Service ───►   API Endpoint
                                                  │
                                                  ▼
                                             [Task 4.4]
                                             Adoption UI
```

**Parallel Execution Opportunities:**
- Tasks 1.1 and 1.2 can run in parallel
- Tasks 2.1 and 1.4 can run in parallel (after Task 1.3)
- Tasks 3.1, 3.2, 3.3 are sequential
- Story 4 can start after Story 2 completion

---

## Integration Checkpoints

### After Story 1
- [ ] External Claude tmux sessions appear in web UI
- [ ] Sessions are correctly categorized (managed vs external)
- [ ] Discovery works on default and custom tmux sockets
- [ ] API returns merged session list

### After Story 2
- [ ] Real-time terminal output visible in browser for external sessions
- [ ] Streaming latency under 100ms
- [ ] Multiple concurrent streams work correctly
- [ ] Session disconnect handled gracefully

### After Story 3
- [ ] External session approvals trigger notifications
- [ ] External sessions appear in review queue
- [ ] Browser notifications work for external approvals
- [ ] Notification metadata includes external source

### Final (After Story 4)
- [ ] Full adoption flow works end-to-end
- [ ] Adopted sessions fully manageable
- [ ] All acceptance criteria met
- [ ] Test coverage >80%
- [ ] Performance benchmarks achieved

---

## Context Preparation Guide

### Task 1.1 Context
**Files to Load:**
- `session/pty_discovery.go` - Main discovery implementation
- `session/types.go` - PTYDiscoveryConfig and related types

**Concepts to Understand:**
- tmux server socket locations (`/tmp/tmux-*/default`)
- tmux `-L` flag for socket selection
- Existing `discoverExternalClaude` implementation

### Task 2.1 Context
**Files to Load:**
- `session/tmux/tmux.go` - tmux wrapper functions
- `session/tmux/pty.go` - PTY factory

**Concepts to Understand:**
- tmux `pipe-pane` command and options
- Unix domain sockets for local IPC
- Go channel patterns for streaming

### Task 3.1 Context
**Files to Load:**
- `session/tmux/tmux.go` - `detectPromptInContent` function
- `server/events/types.go` - Event types

**Concepts to Understand:**
- Claude approval prompt patterns
- Event emission patterns
- Stream processing in Go

---

## Success Criteria

- [ ] All atomic tasks completed and validated
- [ ] All acceptance criteria met for all stories
- [ ] Test coverage meets requirements (>80%)
- [ ] Performance benchmarks achieved (<100ms streaming latency)
- [ ] Documentation complete and accurate
- [ ] Code review approved
- [ ] No known critical bugs
- [ ] Cross-platform testing passed (macOS + Linux)

---

## References

- [teeterm - PTY multiplexing](https://github.com/kcghost/teeterm)
- [reptyr - PTY stealing via ptrace](https://github.com/nelhage/reptyr)
- [tmux pipe-pane documentation](https://man7.org/linux/man-pages/man1/tmux.1.html)
- [SCM_RIGHTS for FD passing](https://man7.org/linux/man-pages/man7/unix.7.html)
- [Existing PTY Discovery implementation](session/pty_discovery.go)
- [Terminal WebSocket handler](server/services/terminal_websocket.go)