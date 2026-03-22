# Terminal Flow Control and Control Code Support

**Epic**: Implement watermark-based flow control for xterm.js terminal streaming with comprehensive terminal control code support

**Created**: 2025-10-27
**Status**: Planned
**Priority**: Critical
**Estimated Effort**: 16-20 hours

---

## Table of Contents

1. [Epic Overview](#epic-overview)
2. [Terminal Control Codes Analysis](#terminal-control-codes-analysis)
3. [Story Breakdown](#story-breakdown)
4. [Task Hierarchy](#task-hierarchy)
5. [Dependency Visualization](#dependency-visualization)
6. [Testing Strategy](#testing-strategy)
7. [Risk Assessment](#risk-assessment)

---

## Epic Overview

### Goal

Implement watermark-based flow control for xterm.js terminal streaming to prevent browser crashes during high-throughput scenarios (large file dumps, log tailing, rapid animations) while ensuring terminal control code integrity is preserved throughout the flow control pipeline.

### Value Proposition

**Current Issues:**
- **No backpressure management**: Unbounded memory accumulation during high output
- **Missing write callbacks**: No completion tracking for flow control
- **Browser crash risk**: High-throughput scenarios (cat large files, log dumps) can cause OOM
- **No PTY pause/resume**: Server-side continues sending data regardless of client buffer state
- **Control code integrity unknown**: Potential for partial escape sequences in flow control boundaries

**Benefits After Implementation:**
- **Stable high-throughput**: Handle large file dumps and log tailing without crashes
- **Efficient bandwidth use**: Existing 70-90% delta compression maintained
- **Responsive UI**: Terminal remains interactive even during output floods
- **Control code safety**: Verified preservation of ANSI escape sequences through flow control
- **Production-ready**: Robust error handling and recovery mechanisms

### Success Metrics

1. **Throughput Stability**: Handle 100MB terminal output without crash or slowdown
2. **Memory Bounds**: Write buffer never exceeds HIGH_WATERMARK (100KB)
3. **Latency**: Flow control overhead < 10ms per pause/resume cycle
4. **Control Code Integrity**: 100% preservation of ANSI escape sequences
5. **Delta Efficiency**: Maintain 70-90% bandwidth reduction during flow control

### Current Architecture Assessment

**Strengths** (from Software Planner review):
- ✅ Excellent write batching with requestAnimationFrame (XtermTerminal.tsx)
- ✅ Outstanding delta compression protocol (70-90% bandwidth reduction)
- ✅ WebGL rendering with graceful canvas fallback
- ✅ Good resize throttling and scrollback management
- ✅ Comprehensive debugging capabilities (localStorage-based toggle)

**Critical Gaps**:
- ❌ No write callbacks for backpressure management
- ❌ Unbounded memory accumulation in write buffer
- ❌ Missing `onWriteParsed` event handling
- ❌ No server-side PTY pause/resume mechanism
- ❌ Unknown control code integrity in flow control boundaries

---

## Terminal Control Codes Analysis

### Control Codes Currently Used

Based on comprehensive codebase analysis, the application uses the following ANSI escape sequences:

#### 1. **Cursor Positioning (CSI sequences)**

**DeltaApplicator.ts:**
```typescript
// Move cursor to absolute position (1-based)
`\x1b[${row + 1};${col + 1}H`   // CUP (Cursor Position)

// Used in line operations:
`\x1b[${lineNum + 1};1H`         // Move to line start
`\x1b[${lineNum + 1};${startCol + 1}H${text}` // Edit at specific column
```

**Control codes**: `CSI n ; m H` (CUP - Cursor Position)

#### 2. **Line Editing (ED/EL sequences)**

**DeltaApplicator.ts:**
```typescript
`\x1b[2K`  // EL2 - Erase entire line
`\x1b[M`   // RI - Reverse Index (delete line, scroll up)
`\x1b[L`   // IL - Insert Line (insert blank line, scroll down)
```

**Control codes**:
- `CSI 2 K` (EL - Erase in Line, mode 2 = entire line)
- `ESC M` (RI - Reverse Index)
- `ESC L` (IL - Insert Line)

#### 3. **Cursor Visibility (DEC Private Mode)**

**DeltaApplicator.ts:**
```typescript
`\x1b[?25h`  // DECTCEM - Show cursor
`\x1b[?25l`  // DECTCEM - Hide cursor
```

**Control codes**: `CSI ? 25 h/l` (DECTCEM - Text Cursor Enable Mode)

#### 4. **SGR (Select Graphic Rendition) - Colors and Formatting**

**Server-side (terminal_state.go):**
```go
// Parse and track SGR sequences for terminal state
// CSI [0-9;]* m - SGR codes for colors, bold, underline, etc.
csiPattern = regexp.MustCompile(`\x1b\[(\??[0-9;]*)([A-Za-z])`)
```

**Common SGR codes in session output:**
- `\x1b[0m` - Reset all attributes
- `\x1b[1m` - Bold
- `\x1b[4m` - Underline
- `\x1b[30m-37m` - Foreground colors
- `\x1b[40m-47m` - Background colors
- `\x1b[90m-97m` - Bright foreground colors

#### 5. **Scrolling Regions (implicitly handled by xterm.js)**

While not explicitly used in DeltaApplicator, xterm.js handles:
- `CSI r` (DECSTBM - Set Top and Bottom Margins)
- Scrolling behavior for line insertion/deletion

#### 6. **Alternate Screen Buffer (used by tmux)**

**Session/instance.go:**
```go
// tmux uses alternate screen buffer for full-screen applications
// SMCUP (CSI ? 1049 h) - Save cursor and switch to alternate buffer
// RMCUP (CSI ? 1049 l) - Restore cursor and return to normal buffer
```

### Control Code Categories by Usage

| Category | Frequency | Used By | Flow Control Impact |
|----------|-----------|---------|-------------------|
| **Cursor Positioning (CUP, CHA)** | Very High | DeltaApplicator (every line edit) | High - must not split |
| **Line Editing (EL, IL, DL)** | High | DeltaApplicator (line operations) | High - must not split |
| **Cursor Visibility (DECTCEM)** | Medium | DeltaApplicator (cursor updates) | Medium - can delay |
| **SGR (Colors/Formatting)** | Very High | Server terminal output | High - must not split |
| **Scrolling (DECSTBM)** | Low | xterm.js internal | Low - handled by terminal |
| **Alternate Buffer (SMCUP/RMCUP)** | Low | tmux sessions | Low - rare events |

### Flow Control Interaction Analysis

#### **1. Partial Escape Sequence Problem**

**Risk**: Flow control pause could split an escape sequence mid-transmission.

**Example of problematic split**:
```
Chunk 1: "Hello \x1b[3"     ← HIGH_WATERMARK reached, pause here
Chunk 2: "1;1H World"        ← Resume later
```

Result: Terminal may interpret `\x1b[3` as incomplete, causing corruption.

**Mitigation Strategy**:
- **Escape sequence boundary detection** in write buffer
- Never pause mid-escape sequence
- Buffer complete sequences before write

#### **2. Delta Compression and Control Codes**

**Current implementation (DeltaApplicator.ts)**:
- Delta protocol sends **complete lines with embedded ANSI codes**
- Each `LineDelta.replaceLine` contains **full escape sequences**
- Cursor position updates are **atomic messages**

**Flow control compatibility**:
- ✅ Line-based deltas are **atomic units** (good for flow control boundaries)
- ✅ Delta protocol naturally aligns with control code boundaries
- ⚠️ Need to verify `DeltaGenerator` (server) preserves code integrity

#### **3. Write Buffering and Batching**

**Current batching (TerminalOutput.tsx)**:
```typescript
// Batches writes with requestAnimationFrame
writeBufferRef.current += output;
requestAnimationFrame(flushWriteBuffer);
```

**Flow control integration**:
- ✅ Batching reduces write frequency (good for watermark checks)
- ⚠️ Need to ensure escape sequences aren't split across batches
- ⚠️ Callback tracking required for buffer size management

#### **4. Server-Side Delta Generation**

**Server implementation (server/terminal/delta.go)**:
```go
// Splits output into lines, preserving ANSI codes
splitIntoBytesLines(output []byte) [][]byte
stripANSIBytes(b []byte) []byte  // For cursor position calc
```

**Observation**:
- Server preserves ANSI codes in line data ✅
- `stripANSIBytes` used only for cursor calculation (not output) ✅
- Line splits happen at `\n` boundaries (safe for control codes) ✅

#### **5. WebSocket Binary Protocol**

**Current protocol (useTerminalStream.ts)**:
```typescript
// Receives binary WebSocket messages
websocket.BinaryMessage → decode → terminal.write()
```

**Flow control requirements**:
- Binary messages preserve escape sequence bytes ✅
- Need pause/resume signaling via WebSocket
- Server must buffer during client pause

### Control Code Testing Requirements

1. **Escape Sequence Boundary Testing**
   - Test flow control pause at every position within escape sequences
   - Verify no corruption with partial sequences buffered
   - Test all control code categories listed above

2. **High-Throughput Control Code Stress Test**
   - Generate output with heavy ANSI formatting (colors, cursor moves)
   - Trigger HIGH_WATERMARK during complex escape sequence flood
   - Verify terminal rendering remains correct

3. **Delta Compression + Flow Control Integration**
   - Test flow control with delta protocol active
   - Verify cursor position tracking remains accurate
   - Ensure line operations (IL, DL, EL) work during pause/resume

4. **Edge Cases**
   - Multi-byte escape sequences (CSI with parameters)
   - Nested control codes (SGR within line operations)
   - Rapid pause/resume cycles with queued escape sequences

### Recommendations

1. **Implement Escape Sequence Parser** (Task 2.2)
   - Detect incomplete escape sequences in write buffer
   - Only pause at safe boundaries (after complete sequences)
   - Buffer partial sequences for next write

2. **Add Control Code Integrity Tests** (Task 4.2)
   - Unit tests for all categories of control codes
   - Integration tests with flow control active
   - Stress tests with mixed delta + raw output

3. **Monitor Control Code Performance** (Task 3.4)
   - Track escape sequence parsing overhead
   - Measure delta compression efficiency with flow control
   - Verify cursor positioning accuracy under load

4. **Document Control Code Guarantees**
   - Specify which codes are guaranteed safe
   - Document flow control pause points
   - Provide debugging tools for control code issues

---

## Story Breakdown

### Story 1: Client-Side Watermark-Based Flow Control (8-10 hours)

**Objective**: Implement write completion tracking and watermark-based pause/resume logic in the React/TypeScript client.

**Value**: Prevents unbounded memory growth in browser, protects against OOM crashes.

**Dependencies**: None (standalone client work)

**Acceptance Criteria**:
- Write buffer size tracking with HIGH_WATERMARK (100KB) and LOW_WATERMARK (10KB)
- Write completion callbacks using `terminal.write(data, callback)`
- Pause signal sent to server when HIGH_WATERMARK reached
- Resume signal sent when LOW_WATERMARK reached
- Escape sequence boundary detection prevents mid-sequence pauses
- All existing functionality preserved (delta compression, batching, WebGL)

**Testing Requirements**:
- Unit tests for watermark logic
- Integration tests with mock server
- Control code integrity tests (all categories)
- Performance benchmarks (no regression in delta efficiency)

---

### Story 2: Server-Side PTY Pause/Resume Mechanism (6-8 hours)

**Objective**: Implement server-side PTY buffering and pause/resume control via WebSocket signaling.

**Value**: Prevents server from overwhelming paused clients, enables true backpressure.

**Dependencies**: Story 1 (requires client pause/resume signals)

**Acceptance Criteria**:
- WebSocket pause/resume message protocol defined and implemented
- Server buffers PTY output during client pause (with overflow protection)
- Server resumes PTY reading on client resume signal
- Graceful degradation if client doesn't support flow control
- Server-side buffer size limits to prevent server OOM

**Testing Requirements**:
- Unit tests for pause/resume state machine
- Integration tests with client flow control
- Stress tests for buffer overflow scenarios
- Performance tests for pause/resume latency

---

### Story 3: Monitoring and Observability (2-3 hours)

**Objective**: Add metrics, logging, and debugging tools to monitor flow control effectiveness.

**Value**: Enables performance tuning and troubleshooting in production.

**Dependencies**: Story 1 & 2 (requires working flow control)

**Acceptance Criteria**:
- Client-side metrics: buffer size, pause/resume events, write latency
- Server-side metrics: PTY buffer size, pause duration, dropped bytes
- Debug UI panel showing real-time flow control stats (optional)
- Performance regression tests in CI/CD
- Control code integrity monitoring

**Testing Requirements**:
- Verify metrics accuracy
- Load testing with metrics enabled
- Benchmark overhead of monitoring code

---

### Story 4: High-Throughput Testing and Validation (3-4 hours)

**Objective**: Comprehensive testing of flow control under extreme load with control code verification.

**Value**: Confidence in production readiness and stability guarantees.

**Dependencies**: Story 1, 2, 3 (requires complete implementation)

**Acceptance Criteria**:
- Test with 100MB+ terminal output (cat large files, log tailing)
- Test with rapid animations (Claude Code output, progress bars)
- Test with complex control code sequences (colors, cursor moves, line edits)
- Memory usage remains bounded under all scenarios
- Terminal remains responsive and interactive
- No visual corruption or control code failures

**Testing Requirements**:
- Automated stress tests
- Manual QA with real-world scenarios
- Performance profiling and optimization
- Control code integrity verification suite

---

## Task Hierarchy

### Story 1: Client-Side Watermark-Based Flow Control

#### Task 1.1: Write Completion Callback Integration (2h - Small)

**Scope**: Replace current `terminal.write(data)` calls with `terminal.write(data, callback)` for completion tracking.

**Files**:
- `web-app/src/lib/hooks/useTerminalStream.ts` (modify)
- `web-app/src/components/sessions/TerminalOutput.tsx` (modify)
- `web-app/src/lib/terminal/DeltaApplicator.ts` (modify - add callback support)

**Context**:
- xterm.js provides write completion callbacks for backpressure
- Current implementation uses fire-and-forget writes
- Need to track pending writes for watermark calculation

**Implementation**:
```typescript
// In DeltaApplicator.ts
applyDelta(delta: TerminalDelta, callback?: () => void): boolean {
  // ... existing logic ...

  // Track write completion
  let pendingWrites = 0;
  const onWriteComplete = () => {
    pendingWrites--;
    if (pendingWrites === 0 && callback) {
      callback(); // All writes for this delta complete
    }
  };

  // Apply line changes with callback
  for (const lineDelta of delta.lines) {
    pendingWrites++;
    this.applyLineDelta(lineDelta, onWriteComplete);
  }
}

private applyLineDelta(lineDelta: LineDelta, callback: () => void): void {
  // Use terminal.write with callback
  this.terminal.write(escapeSequence, callback);
}
```

**Success Criteria**:
- All `terminal.write()` calls use callbacks
- Write completion tracked accurately
- Callback chaining handles nested writes (cursor + line operations)
- No regression in terminal rendering

**Testing**:
- Unit tests for callback tracking logic
- Integration tests verifying callbacks fire correctly
- Performance tests (no overhead from callbacks)

**Dependencies**: None

**Status**: ⏳ Pending

---

#### Task 1.2: Watermark Logic and Buffer Size Tracking (3h - Medium)

**Scope**: Implement HIGH_WATERMARK/LOW_WATERMARK logic with buffer size accounting.

**Files**:
- `web-app/src/lib/hooks/useTerminalStream.ts` (modify - add watermark state)
- `web-app/src/lib/terminal/FlowControl.ts` (create - new flow control manager)

**Context**:
- HIGH_WATERMARK: 100KB (pause threshold)
- LOW_WATERMARK: 10KB (resume threshold)
- Track pending write buffer size based on completion callbacks
- Hysteresis prevents rapid pause/resume oscillation

**Implementation**:
```typescript
// New FlowControl.ts
export class FlowControl {
  private readonly HIGH_WATERMARK = 100 * 1024; // 100KB
  private readonly LOW_WATERMARK = 10 * 1024;   // 10KB

  private pendingBytes = 0;
  private isPaused = false;

  private onPause: () => void;
  private onResume: () => void;

  // Track write initiated
  onWrite(dataLength: number) {
    this.pendingBytes += dataLength;

    if (!this.isPaused && this.pendingBytes >= this.HIGH_WATERMARK) {
      this.isPaused = true;
      this.onPause();
    }
  }

  // Track write completed
  onWriteComplete(dataLength: number) {
    this.pendingBytes -= dataLength;

    if (this.isPaused && this.pendingBytes <= this.LOW_WATERMARK) {
      this.isPaused = false;
      this.onResume();
    }
  }
}
```

**Success Criteria**:
- Buffer size tracked accurately based on callbacks
- HIGH_WATERMARK triggers pause reliably
- LOW_WATERMARK triggers resume reliably
- Hysteresis prevents rapid state changes
- No deadlocks or stuck states

**Testing**:
- Unit tests for watermark state machine
- Stress tests with rapid writes
- Edge case tests (exactly at watermark, rapid pause/resume)

**Dependencies**: Task 1.1 (requires write callbacks)

**Status**: ⏳ Pending

---

#### Task 1.3: Escape Sequence Boundary Detection (2h - Small)

**Scope**: Implement parser to detect incomplete ANSI escape sequences and prevent mid-sequence pauses.

**Files**:
- `web-app/src/lib/terminal/EscapeSequenceParser.ts` (create)
- `web-app/src/lib/terminal/FlowControl.ts` (modify - integrate parser)

**Context**:
- ANSI escape sequences start with `\x1b` (ESC)
- CSI sequences: `ESC [ params letter` (e.g., `\x1b[31;1H`)
- Must not pause during escape sequence transmission
- Need to buffer partial sequences

**Implementation**:
```typescript
// New EscapeSequenceParser.ts
export class EscapeSequenceParser {
  // Detect if buffer ends with incomplete escape sequence
  hasIncompleteSequence(data: string): boolean {
    const escIndex = data.lastIndexOf('\x1b');
    if (escIndex === -1) return false;

    const remaining = data.substring(escIndex);

    // Check for incomplete CSI sequence
    if (remaining.match(/^\x1b\[[0-9;?]*$/)) {
      return true; // CSI params without terminal letter
    }

    // Check for incomplete simple escape
    if (remaining === '\x1b' || remaining === '\x1b[') {
      return true;
    }

    return false;
  }

  // Find safe pause boundary (after complete sequences)
  findSafeBoundary(data: string): number {
    // Scan backwards to find last complete sequence
    for (let i = data.length - 1; i >= 0; i--) {
      const chunk = data.substring(0, i);
      if (!this.hasIncompleteSequence(chunk)) {
        return i; // Safe to pause here
      }
    }
    return 0; // No safe boundary found, must buffer entire chunk
  }
}
```

**Success Criteria**:
- Detects all incomplete CSI sequences
- Detects incomplete simple escape sequences
- Never pauses mid-escape sequence
- Minimal performance overhead (<5% on large buffers)

**Testing**:
- Unit tests for all control code categories
- Edge cases: escape at buffer end, nested sequences
- Performance benchmarks

**Dependencies**: Task 1.2 (integrates with flow control)

**Status**: ⏳ Pending

---

#### Task 1.4: WebSocket Pause/Resume Signaling (1h - Micro)

**Scope**: Define and implement pause/resume message protocol over WebSocket.

**Files**:
- `proto/session/v1/events.proto` (modify - add flow control messages)
- `web-app/src/lib/hooks/useTerminalStream.ts` (modify - send pause/resume)

**Context**:
- Use existing protobuf `TerminalData` message
- Add new oneof cases for flow control
- Client sends pause/resume to server

**Implementation**:
```protobuf
// In events.proto
message TerminalData {
  oneof data {
    // ... existing fields ...
    FlowControlPause pause = 11;
    FlowControlResume resume = 12;
  }
}

message FlowControlPause {
  int32 buffer_size = 1;  // Current buffer size (for metrics)
  string reason = 2;       // "high_watermark", "user_request"
}

message FlowControlResume {
  int32 buffer_size = 1;  // Current buffer size (for metrics)
}
```

**Success Criteria**:
- Pause/resume messages defined in protobuf
- Client sends pause when HIGH_WATERMARK reached
- Client sends resume when LOW_WATERMARK reached
- Messages include diagnostic info (buffer size, reason)

**Testing**:
- Unit tests for message serialization
- Integration tests with mock server
- Verify message ordering (no pause without resume)

**Dependencies**: Task 1.2 (triggered by watermark logic)

**Status**: ⏳ Pending

---

### Story 2: Server-Side PTY Pause/Resume Mechanism

#### Task 2.1: WebSocket Flow Control Message Handling (2h - Small)

**Scope**: Server-side parsing and handling of client pause/resume messages.

**Files**:
- `server/services/terminal_websocket.go` (modify - handle flow control messages)
- `server/services/connectrpc_websocket.go` (modify - add flow control to gRPC stream)

**Context**:
- Server receives pause/resume from client via WebSocket
- Need state machine to track client flow control state
- Coordinate with PTY reading goroutine

**Implementation**:
```go
// In terminal_websocket.go
type FlowControlState struct {
  isPaused bool
  pauseTime time.Time
  resumeTime time.Time
  totalPauseDuration time.Duration
}

func (h *TerminalWebSocketHandler) handleFlowControlMessage(msg *sessionv1.TerminalData) {
  switch msg.Data.(type) {
  case *sessionv1.TerminalData_Pause:
    h.flowControl.Pause()
    log.InfoLog.Printf("Client requested pause (buffer: %d bytes)", msg.GetPause().BufferSize)

  case *sessionv1.TerminalData_Resume:
    h.flowControl.Resume()
    log.InfoLog.Printf("Client resumed (buffer: %d bytes)", msg.GetResume().BufferSize)
  }
}
```

**Success Criteria**:
- Pause messages update server state correctly
- Resume messages clear pause state
- State transitions logged for debugging
- Metrics collected (pause duration, frequency)

**Testing**:
- Unit tests for flow control state machine
- Integration tests with mock client
- Verify state transitions

**Dependencies**: Task 1.4 (requires client messages)

**Status**: ⏳ Pending

---

#### Task 2.2: PTY Read Buffering During Pause (3h - Medium)

**Scope**: Buffer PTY output during client pause, resume on client resume signal.

**Files**:
- `server/services/terminal_websocket.go` (modify - add buffering logic)
- `session/instance.go` (review - verify PTY read behavior)

**Context**:
- Server continues reading from PTY during pause (prevent PTY buffer overflow)
- Buffer PTY data in memory until resume
- Need overflow protection (max buffer size)

**Implementation**:
```go
// In terminal_websocket.go
type PTYBuffer struct {
  buffer *bytes.Buffer
  maxSize int // e.g., 10MB max buffer
  dropped int64 // Track dropped bytes if overflow
}

// Goroutine 1: Read from PTY (modified)
go func() {
  buf := make([]byte, 1024)
  for {
    n, err := ptyReader.Read(buf)
    if err != nil {
      return
    }

    if h.flowControl.IsPaused() {
      // Buffer during pause
      if h.ptyBuffer.Len() < h.ptyBuffer.maxSize {
        h.ptyBuffer.Write(buf[:n])
      } else {
        // Buffer full, drop data and track
        h.ptyBuffer.dropped += int64(n)
        log.WarningLog.Printf("PTY buffer full, dropped %d bytes", n)
      }
    } else {
      // Send immediately when not paused
      conn.WriteMessage(websocket.BinaryMessage, buf[:n])
    }
  }
}()

// On resume, flush buffered data
func (h *TerminalWebSocketHandler) onResume(conn *websocket.Conn) {
  buffered := h.ptyBuffer.Bytes()
  if len(buffered) > 0 {
    conn.WriteMessage(websocket.BinaryMessage, buffered)
    h.ptyBuffer.Reset()
  }
}
```

**Success Criteria**:
- PTY data buffered during pause
- Buffered data sent on resume
- Buffer overflow protected (max size enforced)
- Dropped bytes tracked and logged
- No deadlocks or goroutine leaks

**Testing**:
- Unit tests for buffering logic
- Stress tests for buffer overflow
- Integration tests with real PTY
- Performance tests (buffer copy overhead)

**Dependencies**: Task 2.1 (requires pause/resume handling)

**Status**: ⏳ Pending

---

#### Task 2.3: Graceful Degradation for Legacy Clients (1h - Micro)

**Scope**: Ensure server works with clients that don't support flow control.

**Files**:
- `server/services/terminal_websocket.go` (modify - detect flow control support)

**Context**:
- Older clients may not send pause/resume messages
- Server should detect and disable flow control for legacy clients
- No change in behavior for non-flow-control clients

**Implementation**:
```go
// Detect flow control support from client handshake
type ClientCapabilities struct {
  supportsFlowControl bool
  protocolVersion string
}

func (h *TerminalWebSocketHandler) detectCapabilities(conn *websocket.Conn) ClientCapabilities {
  // Check for flow control capability in first message
  // If no pause/resume in first 5 seconds, assume legacy client

  return ClientCapabilities{
    supportsFlowControl: hasFlowControlFeature,
    protocolVersion: "v1",
  }
}

// Skip buffering for legacy clients
if !h.capabilities.supportsFlowControl {
  // Old behavior: direct PTY → WebSocket
  conn.WriteMessage(websocket.BinaryMessage, buf[:n])
}
```

**Success Criteria**:
- Legacy clients work without changes
- Flow control disabled for legacy clients
- No performance regression for legacy path
- Capability detection logged

**Testing**:
- Integration tests with mock legacy client
- Verify no flow control behavior
- Performance parity with pre-flow-control code

**Dependencies**: Task 2.2 (conditional on flow control support)

**Status**: ⏳ Pending

---

### Story 3: Monitoring and Observability

#### Task 3.1: Client-Side Flow Control Metrics (1h - Micro)

**Scope**: Add metrics for client-side flow control events and buffer sizes.

**Files**:
- `web-app/src/lib/terminal/FlowControl.ts` (modify - add metrics)
- `web-app/src/components/sessions/TerminalOutput.tsx` (modify - display metrics)

**Context**:
- Track pause/resume frequency
- Track buffer sizes over time
- Track write latency (completion time)
- Expose via debug panel or console

**Implementation**:
```typescript
export interface FlowControlMetrics {
  totalPauses: number;
  totalResumes: number;
  currentBufferSize: number;
  maxBufferSize: number;
  avgWriteLatency: number;
  lastPauseTime: number | null;
  lastResumeTime: number | null;
}

export class FlowControl {
  private metrics: FlowControlMetrics = {
    totalPauses: 0,
    totalResumes: 0,
    currentBufferSize: 0,
    maxBufferSize: 0,
    avgWriteLatency: 0,
    lastPauseTime: null,
    lastResumeTime: null,
  };

  getMetrics(): FlowControlMetrics {
    return { ...this.metrics };
  }
}
```

**Success Criteria**:
- Metrics updated correctly on events
- Metrics accessible via API
- Minimal overhead (<1% CPU)
- Debug logging includes metrics

**Testing**:
- Unit tests for metric calculation
- Verify accuracy under load
- Performance overhead tests

**Dependencies**: Story 1 (requires flow control implementation)

**Status**: ⏳ Pending

---

#### Task 3.2: Server-Side Flow Control Metrics (1h - Micro)

**Scope**: Add server-side metrics for PTY buffering and flow control state.

**Files**:
- `server/services/terminal_websocket.go` (modify - add metrics)
- `server/metrics.go` (create - centralized metrics)

**Context**:
- Track PTY buffer size
- Track pause duration
- Track dropped bytes (overflow)
- Expose via Prometheus or logging

**Implementation**:
```go
type FlowControlMetrics struct {
  TotalPauses int64
  TotalResumes int64
  TotalPauseDuration time.Duration
  CurrentBufferSize int
  MaxBufferSize int
  DroppedBytes int64
}

func (h *TerminalWebSocketHandler) recordMetrics() {
  log.InfoLog.Printf("Flow control metrics: pauses=%d, buffer=%d, dropped=%d",
    h.metrics.TotalPauses,
    h.metrics.CurrentBufferSize,
    h.metrics.DroppedBytes)
}
```

**Success Criteria**:
- Metrics logged periodically
- Buffer overflow tracked accurately
- Metrics exportable (Prometheus format)
- No performance impact

**Testing**:
- Verify metric accuracy
- Load tests with metrics enabled
- Prometheus scraping tests (if applicable)

**Dependencies**: Story 2 (requires server flow control)

**Status**: ⏳ Pending

---

#### Task 3.3: Debug UI Panel for Flow Control (Optional) (1h - Micro)

**Scope**: Add visual debug panel showing real-time flow control state.

**Files**:
- `web-app/src/components/sessions/FlowControlDebugPanel.tsx` (create)
- `web-app/src/components/sessions/TerminalOutput.tsx` (modify - integrate panel)

**Context**:
- Show buffer size graph
- Show pause/resume timeline
- Show write latency
- Toggled via debug mode (localStorage)

**Implementation**:
```typescript
export function FlowControlDebugPanel({ metrics }: { metrics: FlowControlMetrics }) {
  return (
    <div className={styles.debugPanel}>
      <h3>Flow Control Debug</h3>
      <div>Buffer: {formatBytes(metrics.currentBufferSize)} / {formatBytes(HIGH_WATERMARK)}</div>
      <div>Pauses: {metrics.totalPauses} | Resumes: {metrics.totalResumes}</div>
      <div>Avg Write Latency: {metrics.avgWriteLatency.toFixed(2)}ms</div>
      <div>Status: {metrics.currentBufferSize > HIGH_WATERMARK ? 'PAUSED' : 'ACTIVE'}</div>
    </div>
  );
}
```

**Success Criteria**:
- Panel displays real-time metrics
- Toggled by debug mode
- Non-intrusive UI placement
- Updates efficiently (no render thrashing)

**Testing**:
- Visual regression tests
- Performance tests (rendering overhead)
- Accessibility tests

**Dependencies**: Task 3.1 (requires client metrics)

**Status**: ⏳ Pending (Optional)

---

#### Task 3.4: Performance Regression Tests (1h - Micro)

**Scope**: Add automated tests to prevent performance regressions from flow control overhead.

**Files**:
- `web-app/src/lib/terminal/FlowControl.test.ts` (create - performance benchmarks)
- `server/services/terminal_websocket_test.go` (modify - add benchmarks)

**Context**:
- Baseline: Current delta compression performance
- Ensure flow control overhead < 5%
- Track write latency, throughput, memory usage

**Implementation**:
```typescript
// Performance benchmark
describe('FlowControl Performance', () => {
  it('should have <5% overhead on throughput', async () => {
    const baseline = await benchmarkWithoutFlowControl();
    const withFlowControl = await benchmarkWithFlowControl();

    const overhead = (withFlowControl - baseline) / baseline;
    expect(overhead).toBeLessThan(0.05); // <5% overhead
  });
});
```

**Success Criteria**:
- Benchmarks pass in CI/CD
- Overhead < 5% for throughput
- Overhead < 10ms for pause/resume latency
- No memory leaks detected

**Testing**:
- Run benchmarks in CI/CD
- Profile memory usage
- Compare with baseline metrics

**Dependencies**: Story 1 & 2 (requires complete implementation)

**Status**: ⏳ Pending

---

### Story 4: High-Throughput Testing and Validation

#### Task 4.1: Large File Dump Stress Test (1h - Micro)

**Scope**: Test flow control with 100MB+ terminal output (cat large files).

**Files**:
- `test/e2e/flow-control-stress.spec.ts` (create - E2E test)

**Context**:
- Simulate large file cat (100MB text file)
- Verify no browser crash
- Verify terminal remains responsive
- Measure memory usage

**Implementation**:
```typescript
describe('Flow Control Stress Tests', () => {
  it('should handle 100MB file dump without crash', async () => {
    // Generate 100MB of random text
    const largeDump = generateLargeOutput(100 * 1024 * 1024);

    // Send to terminal
    await sendToTerminal(largeDump);

    // Verify browser didn't crash
    expect(await page.isVisible('.terminal')).toBe(true);

    // Verify memory stayed bounded
    const metrics = await page.evaluate(() => performance.memory);
    expect(metrics.usedJSHeapSize).toBeLessThan(500 * 1024 * 1024); // <500MB
  });
});
```

**Success Criteria**:
- 100MB output handled without crash
- Memory usage < 500MB
- Terminal remains interactive
- No visual corruption

**Testing**:
- Automated E2E tests
- Manual QA with real files
- Memory profiling

**Dependencies**: Story 1 & 2 (requires flow control)

**Status**: ⏳ Pending

---

#### Task 4.2: Control Code Integrity Test Suite (2h - Small)

**Scope**: Comprehensive test suite verifying control code preservation through flow control.

**Files**:
- `test/integration/control-codes.test.ts` (create)
- `web-app/src/lib/terminal/EscapeSequenceParser.test.ts` (expand)

**Context**:
- Test all control code categories from analysis
- Verify no corruption during pause/resume
- Test partial sequence buffering
- Test delta compression + flow control interaction

**Implementation**:
```typescript
describe('Control Code Integrity with Flow Control', () => {
  const testCases = [
    { name: 'Cursor Positioning', codes: ['\x1b[10;5H', '\x1b[1;1H'] },
    { name: 'Line Editing', codes: ['\x1b[2K', '\x1b[M', '\x1b[L'] },
    { name: 'Cursor Visibility', codes: ['\x1b[?25h', '\x1b[?25l'] },
    { name: 'SGR Colors', codes: ['\x1b[31m', '\x1b[1;4m', '\x1b[0m'] },
    { name: 'Complex Sequences', codes: ['\x1b[38;5;208m', '\x1b[48;2;128;0;255m'] },
  ];

  testCases.forEach(({ name, codes }) => {
    it(`should preserve ${name} during flow control`, async () => {
      // Trigger HIGH_WATERMARK with control codes mixed in
      const output = generateOutputWithControlCodes(codes, 150 * 1024); // Over HIGH_WATERMARK

      await sendToTerminal(output);

      // Verify terminal state matches expected (cursor position, colors, etc.)
      const terminalState = await captureTerminalState();
      expect(terminalState).toMatchExpectedState();
    });
  });

  it('should not split escape sequences at pause boundaries', async () => {
    // Force pause mid-escape sequence transmission
    const output = 'Hello\x1b[31;1HWorld'; // Pause should happen after complete sequence

    await sendToTerminalWithForcedPause(output);

    // Verify no corruption
    const content = await getTerminalContent();
    expect(content).toContain('World'); // Should be positioned correctly
  });
});
```

**Success Criteria**:
- All control code categories tested
- 100% preservation during flow control
- Partial sequences correctly buffered
- Delta compression compatibility verified

**Testing**:
- Comprehensive test suite (50+ test cases)
- All categories from analysis covered
- Edge cases tested (nested codes, rapid sequences)

**Dependencies**: Story 1 (requires escape sequence parser)

**Status**: ⏳ Pending

---

#### Task 4.3: Rapid Animation Stress Test (1h - Micro)

**Scope**: Test flow control with rapid terminal animations (Claude Code output, progress bars).

**Files**:
- `test/e2e/animations-stress.spec.ts` (create)

**Context**:
- Claude Code produces rapid line updates (delta compression)
- Progress bars send frequent cursor position updates
- Verify flow control doesn't break animations
- Verify delta compression efficiency maintained

**Implementation**:
```typescript
describe('Animation Flow Control Tests', () => {
  it('should handle rapid line updates without slowdown', async () => {
    // Simulate Claude Code rapid output (line rewrites)
    for (let i = 0; i < 1000; i++) {
      await sendDelta({
        lines: [{ lineNumber: 5, operation: 'replaceLine', content: `Progress: ${i}/1000` }],
        cursor: { row: 5, col: 20 },
      });
    }

    // Verify animation remained smooth
    const frameRate = await measureFrameRate();
    expect(frameRate).toBeGreaterThan(30); // >30fps

    // Verify flow control activated if needed
    const metrics = await getFlowControlMetrics();
    if (metrics.totalPauses > 0) {
      // If flow control activated, verify smooth resume
      expect(metrics.avgWriteLatency).toBeLessThan(50); // <50ms latency
    }
  });
});
```

**Success Criteria**:
- Animations remain smooth (>30fps)
- Flow control activates only if necessary
- Delta compression efficiency maintained
- No visual glitches

**Testing**:
- Automated frame rate tests
- Visual regression tests
- Manual QA with real Claude Code sessions

**Dependencies**: Story 1 & 2 (requires delta + flow control)

**Status**: ⏳ Pending

---

## Dependency Visualization

```
Epic: Terminal Flow Control and Control Code Support
├─ Story 1: Client-Side Watermark-Based Flow Control (8-10h)
│  ├─ Task 1.1: Write Completion Callback Integration (2h) ──┐
│  ├─ Task 1.2: Watermark Logic and Buffer Tracking (3h) ────┤ ← Depends on 1.1
│  ├─ Task 1.3: Escape Sequence Boundary Detection (2h) ─────┤ ← Depends on 1.2
│  └─ Task 1.4: WebSocket Pause/Resume Signaling (1h) ───────┘ ← Depends on 1.2
│
├─ Story 2: Server-Side PTY Pause/Resume Mechanism (6-8h)
│  ├─ Task 2.1: WebSocket Flow Control Message Handling (2h) ← Depends on 1.4
│  ├─ Task 2.2: PTY Read Buffering During Pause (3h) ────────┤ ← Depends on 2.1
│  └─ Task 2.3: Graceful Degradation for Legacy Clients (1h) ┘ ← Depends on 2.2
│
├─ Story 3: Monitoring and Observability (2-3h)
│  ├─ Task 3.1: Client-Side Flow Control Metrics (1h) ───────┐ ← Depends on Story 1
│  ├─ Task 3.2: Server-Side Flow Control Metrics (1h) ───────┤ ← Depends on Story 2
│  ├─ Task 3.3: Debug UI Panel (Optional) (1h) ──────────────┤ ← Depends on 3.1
│  └─ Task 3.4: Performance Regression Tests (1h) ───────────┘ ← Depends on Story 1 & 2
│
└─ Story 4: High-Throughput Testing and Validation (3-4h)
   ├─ Task 4.1: Large File Dump Stress Test (1h) ───────────┐ ← Depends on Story 1 & 2
   ├─ Task 4.2: Control Code Integrity Test Suite (2h) ─────┤ ← Depends on Story 1
   └─ Task 4.3: Rapid Animation Stress Test (1h) ───────────┘ ← Depends on Story 1 & 2

Critical Path (longest dependency chain):
Task 1.1 → 1.2 → 1.3 → 1.4 → 2.1 → 2.2 → 2.3 → 4.1
Total: 2h + 3h + 2h + 1h + 2h + 3h + 1h + 1h = 15 hours

Parallel Execution Opportunities:
- Task 1.3 (Escape Parser) can be developed in parallel with 1.4 (WebSocket signaling) after 1.2
- Story 3 (Monitoring) can start after Story 1 completes (parallel with Story 2)
- Task 4.2 (Control Code Tests) can start after Task 1.3 completes (early validation)
```

---

## Context Preparation

### Files to Review Before Starting

**Client-Side Context (Story 1)**:
1. `web-app/src/lib/hooks/useTerminalStream.ts` - Current streaming implementation
2. `web-app/src/components/sessions/XtermTerminal.tsx` - Terminal component and addons
3. `web-app/src/lib/terminal/DeltaApplicator.ts` - Delta compression client
4. `web-app/src/components/sessions/TerminalOutput.tsx` - Write batching logic
5. `@xterm/xterm` documentation - Write callback API, performance best practices

**Server-Side Context (Story 2)**:
1. `server/services/terminal_websocket.go` - Current WebSocket streaming
2. `server/terminal/delta.go` - Delta generation logic
3. `session/instance.go` - PTY reader methods (`GetPTYReader`, `WriteToPTY`)
4. `proto/session/v1/events.proto` - Terminal protocol definition

**Testing Context (Story 4)**:
1. Playwright documentation - E2E testing framework
2. Performance API - Memory and timing measurements
3. xterm.js test utilities - Terminal state capture and verification

### Key Patterns and Conventions

**Client-Side**:
- Use refs for state that shouldn't trigger re-renders (buffer tracking)
- Use `requestAnimationFrame` for write batching (existing pattern)
- Use TypeScript strict mode for type safety
- Follow existing debug logging pattern (`localStorage.getItem('debug-terminal')`)

**Server-Side**:
- Use Go channels for goroutine coordination
- Use sync primitives (mutex, atomic) for shared state
- Follow existing logging patterns (`log.InfoLog`, `log.ErrorLog`)
- Use context for cancellation and timeouts

**Control Codes**:
- All ANSI sequences start with `\x1b` (ESC byte)
- CSI sequences: `\x1b[...letter` (most common)
- Simple escapes: `\x1b + letter` (less common)
- Incomplete sequence detection: scan backwards from buffer end

---

## Testing Strategy

### Unit Tests

**Client-Side**:
- `FlowControl.test.ts` - Watermark logic, state transitions
- `EscapeSequenceParser.test.ts` - Control code detection (all categories)
- `DeltaApplicator.test.ts` - Callback chaining, write completion

**Server-Side**:
- `terminal_websocket_test.go` - Flow control state machine
- `pty_buffer_test.go` - Buffering logic, overflow protection
- `delta_test.go` - Control code preservation in deltas

### Integration Tests

**Client-Server**:
- Mock server for pause/resume protocol testing
- Real PTY integration tests (with tmux)
- Delta compression + flow control interaction tests

### E2E Tests (Playwright)

**High-Throughput Scenarios**:
- Large file dumps (100MB+)
- Rapid animations (Claude Code output)
- Log tailing (continuous output)
- Mixed control codes + high volume

**Control Code Integrity**:
- All categories from analysis
- Partial sequence handling
- Delta compression compatibility

### Performance Tests

**Benchmarks**:
- Write throughput (with/without flow control)
- Pause/resume latency
- Memory usage under load
- Delta compression efficiency

**Regression Prevention**:
- CI/CD performance gates
- Baseline comparisons
- Automated performance reports

---

## Risk Assessment

### High-Priority Risks

#### Risk 1: Write Callback Performance Overhead

**Description**: Adding callbacks to every `terminal.write()` call may introduce latency.

**Likelihood**: Medium
**Impact**: High (user-visible lag)

**Mitigation**:
- Benchmark callback overhead early (Task 1.1)
- Use callback batching if needed
- Profile with Chrome DevTools
- Set performance budgets (<5% overhead)

**Contingency**: If overhead > 5%, explore alternative tracking (write size estimation, sampling)

---

#### Risk 2: Partial Escape Sequence Corruption

**Description**: Flow control pause may split escape sequences, causing terminal corruption.

**Likelihood**: High (without mitigation)
**Impact**: Critical (unusable terminal)

**Mitigation**:
- Escape sequence boundary detection (Task 1.3)
- Comprehensive control code test suite (Task 4.2)
- Buffer partial sequences until complete
- Early validation with all control code categories

**Contingency**: If detection fails, fall back to safe boundaries (line breaks only)

---

#### Risk 3: Server PTY Buffer Overflow

**Description**: Server buffer may fill faster than client can consume during pause.

**Likelihood**: Medium
**Impact**: High (data loss)

**Mitigation**:
- Enforce max buffer size (10MB)
- Track dropped bytes and log warnings
- Implement backpressure to PTY (stop reading if buffer full)
- Monitor buffer metrics in production

**Contingency**: If overflow frequent, increase buffer size or add client resume timeout

---

#### Risk 4: Delta Compression Incompatibility

**Description**: Flow control may interfere with delta protocol's line-based updates.

**Likelihood**: Low
**Impact**: Medium (reduced efficiency)

**Mitigation**:
- Delta protocol already uses atomic line updates (good boundary)
- Test delta + flow control integration early (Task 4.3)
- Verify cursor positioning accuracy under flow control
- Monitor delta compression efficiency metrics

**Contingency**: If issues arise, disable delta during flow control (fallback to raw output)

---

#### Risk 5: Pause/Resume Feedback Loop

**Description**: Rapid pause/resume cycles may cause oscillation or deadlock.

**Likelihood**: Medium
**Impact**: Medium (reduced performance)

**Mitigation**:
- Hysteresis in watermark logic (100KB high, 10KB low)
- Minimum pause duration (e.g., 100ms)
- Exponential backoff if rapid cycles detected
- Monitor pause/resume frequency metrics

**Contingency**: If oscillation occurs, increase hysteresis gap or add dampening

---

### Medium-Priority Risks

#### Risk 6: Legacy Client Breakage

**Description**: Flow control changes may break older clients.

**Likelihood**: Low (with Task 2.3)
**Impact**: High (production outage)

**Mitigation**:
- Graceful degradation for legacy clients (Task 2.3)
- Feature detection on client handshake
- Backward compatibility tests
- Phased rollout with monitoring

**Contingency**: Feature flag to disable flow control, rollback plan

---

#### Risk 7: WebGL Renderer Interaction

**Description**: Flow control may interfere with WebGL rendering pipeline.

**Likelihood**: Low
**Impact**: Medium (rendering glitches)

**Mitigation**:
- Test with WebGL renderer enabled (default)
- Verify canvas fallback also works
- Monitor frame rates with flow control active
- Test with various terminal sizes and fonts

**Contingency**: Disable WebGL during flow control if issues arise

---

### Low-Priority Risks

#### Risk 8: Performance Regression in Delta Compression

**Description**: Flow control overhead may reduce delta efficiency.

**Likelihood**: Low
**Impact**: Low (slight bandwidth increase)

**Mitigation**:
- Performance regression tests (Task 3.4)
- Monitor delta compression metrics
- Baseline comparison tests

**Contingency**: Optimize hot paths, reduce callback overhead

---

## Implementation Roadmap

### Phase 1: Foundation (Week 1)
- Task 1.1: Write Completion Callbacks (2h)
- Task 1.2: Watermark Logic (3h)
- Task 1.3: Escape Sequence Parser (2h)
- **Milestone**: Client-side flow control logic complete

### Phase 2: Integration (Week 2)
- Task 1.4: WebSocket Signaling (1h)
- Task 2.1: Server Message Handling (2h)
- Task 2.2: PTY Buffering (3h)
- Task 2.3: Legacy Client Support (1h)
- **Milestone**: End-to-end flow control working

### Phase 3: Validation (Week 3)
- Task 3.1-3.4: Monitoring and Metrics (3h)
- Task 4.1: Large File Stress Test (1h)
- Task 4.2: Control Code Integrity Tests (2h)
- Task 4.3: Animation Stress Test (1h)
- **Milestone**: Production-ready with comprehensive testing

### Phase 4: Deployment (Week 4)
- Performance tuning based on test results
- Documentation updates
- Phased rollout with monitoring
- **Milestone**: Deployed to production

---

## Success Criteria Summary

### Functional Requirements
- ✅ Handle 100MB+ terminal output without crash
- ✅ Memory usage bounded (HIGH_WATERMARK enforced)
- ✅ Terminal remains responsive during high output
- ✅ All control codes preserved (no corruption)
- ✅ Delta compression efficiency maintained (70-90%)
- ✅ Legacy clients continue working

### Performance Requirements
- ✅ Flow control overhead < 5% on throughput
- ✅ Pause/resume latency < 10ms
- ✅ Write completion callbacks < 1ms overhead
- ✅ Escape sequence parsing < 5% CPU

### Quality Requirements
- ✅ Comprehensive test coverage (>90%)
- ✅ No memory leaks detected
- ✅ Production monitoring and metrics
- ✅ Clear debugging tools and logs

### Documentation Requirements
- ✅ Architecture documentation (this file)
- ✅ API documentation (flow control messages)
- ✅ Runbook for troubleshooting
- ✅ Performance tuning guide

---

## References

### External Resources
- [xterm.js Flow Control Documentation](https://xtermjs.org/docs/guides/flowcontrol/)
- [ANSI Escape Codes Reference](https://en.wikipedia.org/wiki/ANSI_escape_code)
- [MOSH Protocol Paper](https://mosh.org/mosh-paper.pdf) - Inspiration for delta compression
- [WebSocket Flow Control Best Practices](https://developer.mozilla.org/en-US/docs/Web/API/WebSockets_API)

### Internal Resources
- [Terminal Delta Protocol Documentation](/Users/tylerstapler/IdeaProjects/stapler-squad/docs/terminal-delta-protocol.md)
- [Software Planner Review Notes](/Users/tylerstapler/IdeaProjects/stapler-squad/docs/terminal-flow-control-review.md) (from initial analysis)

### Related Issues
- Terminal crashes during large file dumps (production issue)
- Claude Code animations causing slowdowns (user reports)
- Control code corruption reports (intermittent)

---

## Appendix A: Control Code Reference

### CSI (Control Sequence Introducer) Sequences

Format: `ESC [ params command`

| Code | Name | Description | Usage in Project |
|------|------|-------------|------------------|
| `\x1b[H` | CUP | Cursor Position (to home) | TerminalOutput.tsx |
| `\x1b[{r};{c}H` | CUP | Cursor Position (to row, col) | DeltaApplicator.ts (all line ops) |
| `\x1b[2K` | EL | Erase Line (entire) | DeltaApplicator.ts (clear line) |
| `\x1b[M` | RI | Reverse Index (delete line) | DeltaApplicator.ts (delete line) |
| `\x1b[L` | IL | Insert Line | DeltaApplicator.ts (insert line) |
| `\x1b[?25h` | DECTCEM | Show Cursor | DeltaApplicator.ts (cursor visibility) |
| `\x1b[?25l` | DECTCEM | Hide Cursor | DeltaApplicator.ts (cursor visibility) |
| `\x1b[{n}m` | SGR | Select Graphic Rendition | Server terminal output (colors) |

### SGR (Select Graphic Rendition) Parameters

| Code | Effect | Usage |
|------|--------|-------|
| `0` | Reset all attributes | Ubiquitous |
| `1` | Bold | Session output, prompts |
| `4` | Underline | Links, emphasis |
| `30-37` | Foreground colors | Syntax highlighting |
| `40-47` | Background colors | Highlighting |
| `90-97` | Bright foreground | Modern terminal themes |

### Simple Escape Sequences

| Code | Name | Description | Usage |
|------|------|-------------|-------|
| `\x1bM` | RI | Reverse Index | Line deletion |
| `\x1bL` | IL | Insert Line | Line insertion |
| `\x1b7` | DECSC | Save Cursor | (Not currently used) |
| `\x1b8` | DECRC | Restore Cursor | (Not currently used) |

---

## Appendix B: xterm.js Write Callback API

### Terminal.write() Signature

```typescript
interface Terminal {
  /**
   * Write data to the terminal.
   * @param data - The data to write (string or Uint8Array)
   * @param callback - Optional callback invoked when write completes
   */
  write(data: string | Uint8Array, callback?: () => void): void;
}
```

### Callback Behavior

- **Async**: Callback invoked after terminal finishes parsing and rendering data
- **Order**: Callbacks invoked in write order (FIFO queue)
- **Batching**: Multiple writes may complete before callback fires (internal batching)
- **Errors**: Callback not invoked on errors (check onWriteParsed event)

### Performance Considerations

- Callbacks add minimal overhead (<1ms per write)
- Use for flow control, not for every byte written
- Batch small writes before checking watermark
- Avoid deep callback nesting (use promises if needed)

### Example Usage

```typescript
terminal.write(data, () => {
  pendingBytes -= data.length;

  if (pendingBytes < LOW_WATERMARK && isPaused) {
    sendResumeMessage();
    isPaused = false;
  }
});
```

---

## Document Change Log

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2025-10-27 | 1.0 | @project-coordinator | Initial comprehensive task breakdown |

---

**Next Steps**: Review this document with team, prioritize tasks, and begin Phase 1 implementation. Update task statuses as work progresses.
