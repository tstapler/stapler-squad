# Terminal Performance Optimization - ATOMIC Task Breakdown

**Epic Goal:** Transform terminal streaming from full-output transmission to efficient delta compression with xterm.js integration, achieving 70-90% bandwidth reduction and 10-100x rendering performance improvement.

**Business Value:**
- Dramatically improved user experience for remote terminal sessions
- Reduced server bandwidth costs by 5-10x (MOSH-style compression)
- Support for larger scale deployments with more concurrent users
- Professional terminal UX (copy/paste, search, selection, theming)

**Success Metrics:**
- Bandwidth usage reduced by 70-90% (measured via WebSocket traffic)
- Terminal rendering consistently <16ms per frame (60fps)
- Memory usage bounded to <100MB per session (circular buffer)
- Zero Chrome performance violations
- Terminal features: copy, paste, search, selection all working

---

## Epic Decomposition

### Story 1: xterm.js Integration Foundation (Week 1)
**Value:** Replace DOM-based terminal with canvas rendering for 10-100x performance gain
**Scope:** Integrate xterm.js library and migrate existing TerminalOutput component

### Story 2: Circular Buffer & Memory Management (Week 1-2)
**Value:** Prevent memory leaks and provide predictable performance for long-running sessions
**Scope:** Implement bounded terminal history with efficient scrollback

### Story 3: Delta Compression Protocol (Week 2-4)
**Value:** 70-90% bandwidth reduction via differential updates (MOSH-style)
**Scope:** Protocol changes, server-side diff generation, client-side patch application

### Story 4: Advanced Terminal Features (Week 4-5)
**Value:** Professional terminal experience with search, themes, and persistence
**Scope:** Terminal search, theme support, session recording/replay

---

## Story 1: xterm.js Integration Foundation

### Objectives
- Replace React-based TerminalOutput with xterm.js canvas rendering
- Maintain existing WebSocket streaming architecture
- Preserve all current functionality (input, resize, scrollback)
- Achieve 60fps rendering with zero performance violations

### Dependencies
- None (foundation for all other stories)

---

### Task 1.1: Install xterm.js Dependencies (1h - Micro)

**Scope:** Add xterm.js npm packages to web-app

**Files:**
- `web-app/package.json`
- `web-app/package-lock.json`

**Context:**
- xterm.js provides canvas-based terminal emulation
- Need core package + addons for features we'll implement later
- WebGL renderer addon for maximum performance

**Implementation:**
```bash
cd web-app
npm install xterm xterm-addon-fit xterm-addon-web-links xterm-addon-webgl xterm-addon-search
```

**Success Criteria:**
- ✅ All xterm.js packages installed
- ✅ No TypeScript compilation errors
- ✅ Build succeeds with `make restart-web`

**Testing:**
- Build web-app: `make restart-web`
- Verify no errors in browser console

**Dependencies:** None

**INVEST Validation:**
- ✅ Independent: No dependencies
- ✅ Negotiable: Package versions can be adjusted
- ✅ Valuable: Enables all xterm.js features
- ✅ Estimable: Standard npm install (1h max)
- ✅ Small: Single package.json change
- ✅ Testable: Build succeeds

---

### Task 1.2: Create XtermTerminal Component Wrapper (2h - Small)

**Scope:** Create React component wrapping xterm.js Terminal instance

**Files:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (new)
- `web-app/src/components/sessions/XtermTerminal.module.css` (new)

**Context:**
- xterm.js uses imperative API, need React lifecycle integration
- Component must handle terminal creation, mounting, and cleanup
- Need refs for terminal instance and container div
- Must support dynamic resizing

**Implementation:**
```typescript
interface XtermTerminalProps {
  onData?: (data: string) => void;
  onResize?: (cols: number, rows: number) => void;
}

export function XtermTerminal({ onData, onResize }: XtermTerminalProps) {
  const terminalRef = useRef<Terminal | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Initialize terminal on mount
  // Setup addons (fit, webgl)
  // Attach event handlers
  // Cleanup on unmount
}
```

**Success Criteria:**
- ✅ Terminal renders in container div
- ✅ Terminal displays "Hello xterm.js" test message
- ✅ Terminal resizes with container
- ✅ onData callback fires when user types
- ✅ No memory leaks (component unmount cleanup)

**Testing:**
- Create test page that renders XtermTerminal
- Type in terminal, verify onData callback
- Resize browser, verify terminal adjusts
- Check browser DevTools memory tab for leaks

**Dependencies:** Task 1.1 (xterm.js installed)

**INVEST Validation:**
- ✅ Independent: Self-contained component
- ✅ Negotiable: Can adjust API surface
- ✅ Valuable: Core wrapper enabling all features
- ✅ Estimable: Standard React component (2h)
- ✅ Small: Single component file
- ✅ Testable: Visual rendering + callbacks

---

### Task 1.3: Integrate XtermTerminal with useTerminalStream (3h - Medium)

**Scope:** Connect xterm.js to existing WebSocket streaming hook

**Files:**
- `web-app/src/components/sessions/TerminalOutput.tsx` (replace)
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)
- `web-app/src/lib/hooks/useTerminalStream.ts` (minor changes)

**Context:**
- Replace old DOM-based TerminalOutput with XtermTerminal
- Hook up WebSocket output to terminal.write()
- Hook up terminal input to sendInput()
- Handle scrollback by writing to terminal buffer
- Remove RAF batching (xterm.js handles internally)

**Implementation:**
```typescript
// In TerminalOutput.tsx replacement
const { output, sendInput, resize, scrollbackLoaded } = useTerminalStream({...});
const terminalRef = useRef<Terminal | null>(null);

// Write output to terminal (replace setState)
useEffect(() => {
  if (terminalRef.current && output) {
    terminalRef.current.write(output);
  }
}, [output]);

// Handle user input
const handleData = (data: string) => {
  sendInput(data);
};
```

**Success Criteria:**
- ✅ Terminal displays live WebSocket output
- ✅ User input sent to server via WebSocket
- ✅ Terminal resize messages sent to server
- ✅ Scrollback displays correctly on load
- ✅ No performance violations in Chrome DevTools
- ✅ Terminal renders at 60fps during heavy output

**Testing:**
- Start session, verify terminal connects
- Type commands, verify they execute
- Watch high-frequency output (e.g., `yes`), verify smooth rendering
- Check Chrome DevTools performance tab
- Verify scrollback loads on page refresh

**Dependencies:**
- Task 1.2 (XtermTerminal component)
- Existing useTerminalStream hook

**INVEST Validation:**
- ✅ Independent: Uses existing hook API
- ✅ Negotiable: Can adjust integration points
- ✅ Valuable: Core terminal functionality working
- ✅ Estimable: Standard integration (3h)
- ✅ Small: 3 files, clear interfaces
- ✅ Testable: Visual + functional tests

---

### Task 1.4: Add WebGL Renderer for Maximum Performance (1h - Micro)

**Scope:** Enable xterm.js WebGL rendering addon

**Files:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)

**Context:**
- WebGL renderer provides ~2x performance over canvas
- Gracefully falls back to canvas if WebGL unavailable
- Simple addon integration with error handling

**Implementation:**
```typescript
import { WebglAddon } from 'xterm-addon-webgl';

useEffect(() => {
  const terminal = terminalRef.current;
  if (!terminal) return;

  try {
    const webglAddon = new WebglAddon();
    terminal.loadAddon(webglAddon);
    console.log('WebGL renderer enabled');
  } catch (e) {
    console.warn('WebGL not available, using canvas fallback:', e);
  }
}, []);
```

**Success Criteria:**
- ✅ WebGL renderer loads successfully
- ✅ Terminal performance improved (check DevTools)
- ✅ Graceful canvas fallback if WebGL fails
- ✅ Console logs renderer type for debugging

**Testing:**
- Verify WebGL enabled in console logs
- Test on device without WebGL support (canvas fallback)
- Compare rendering performance before/after

**Dependencies:** Task 1.3 (XtermTerminal integrated)

**INVEST Validation:**
- ✅ Independent: Addon integration
- ✅ Negotiable: Optional performance enhancement
- ✅ Valuable: 2x rendering performance
- ✅ Estimable: Simple addon (1h)
- ✅ Small: Single useEffect addition
- ✅ Testable: Performance comparison

---

### Task 1.5: Implement FitAddon for Dynamic Resizing (2h - Small)

**Scope:** Auto-resize terminal to fit container dimensions

**Files:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)
- `web-app/src/components/sessions/TerminalOutput.module.css` (update)

**Context:**
- FitAddon calculates optimal terminal size for container
- Need ResizeObserver to detect container size changes
- Must debounce resize events to avoid thrashing
- Send resize messages to server via WebSocket

**Implementation:**
```typescript
import { FitAddon } from 'xterm-addon-fit';

const fitAddon = useRef<FitAddon>(new FitAddon());

// Fit terminal to container on mount and resize
useEffect(() => {
  const terminal = terminalRef.current;
  if (!terminal) return;

  terminal.loadAddon(fitAddon.current);

  const resizeObserver = new ResizeObserver(
    debounce(() => {
      fitAddon.current.fit();
      const { cols, rows } = terminal;
      onResize?.(cols, rows);
    }, 150)
  );

  if (containerRef.current) {
    resizeObserver.observe(containerRef.current);
  }

  return () => resizeObserver.disconnect();
}, [onResize]);
```

**Success Criteria:**
- ✅ Terminal auto-resizes when browser window resized
- ✅ Terminal fills container exactly (no overflow)
- ✅ Resize messages sent to server
- ✅ Server terminal PTY resized correctly
- ✅ Debouncing prevents resize thrashing
- ✅ No layout shifts or flashing

**Testing:**
- Resize browser window, verify terminal adjusts
- Check WebSocket messages for resize events
- Run `tput cols; tput lines` in terminal, verify matches
- Test rapid resizing (no performance issues)

**Dependencies:** Task 1.3 (XtermTerminal integrated)

**INVEST Validation:**
- ✅ Independent: Self-contained resize logic
- ✅ Negotiable: Debounce timing adjustable
- ✅ Valuable: Professional responsive behavior
- ✅ Estimable: Standard addon usage (2h)
- ✅ Small: Single useEffect + CSS
- ✅ Testable: Visual + functional

---

### Task 1.6: Remove Legacy TerminalOutput and RAF Batching (1h - Micro)

**Scope:** Clean up old React-based terminal implementation

**Files:**
- `web-app/src/components/sessions/TerminalOutput.tsx` (delete/replace)
- `web-app/src/lib/hooks/useTerminalStream.ts` (simplify)

**Context:**
- Old DOM-based rendering no longer needed
- RAF batching not needed (xterm.js handles internally)
- useTerminalStream can return raw output for xterm.write()
- Remove outputBufferRef, pendingUpdateRef, scheduleOutputUpdate

**Implementation:**
- Delete old TerminalOutput component (if not reused for XtermTerminal wrapper)
- Remove RAF batching logic from useTerminalStream
- Simplify to direct text output: `setOutput(text)` or callback-based

**Success Criteria:**
- ✅ No unused code remaining
- ✅ useTerminalStream simplified
- ✅ All tests still pass
- ✅ Terminal functionality unchanged
- ✅ Build size reduced (removed unused code)

**Testing:**
- Full regression test of terminal features
- Verify no runtime errors
- Check bundle size reduction

**Dependencies:** Task 1.3 (XtermTerminal working)

**INVEST Validation:**
- ✅ Independent: Pure deletion task
- ✅ Negotiable: Can keep as fallback
- ✅ Valuable: Code simplification
- ✅ Estimable: Straightforward cleanup (1h)
- ✅ Small: Delete/simplify specific lines
- ✅ Testable: Existing tests pass

---

### Task 1.7: E2E Testing and Performance Validation (2h - Small)

**Scope:** Comprehensive testing of xterm.js integration

**Files:**
- Create `web-app/src/components/sessions/XtermTerminal.test.tsx`
- Update manual testing checklist

**Context:**
- Verify all user-facing functionality works
- Measure performance improvements
- Document before/after metrics
- Validate against success criteria

**Test Coverage:**
- Unit tests for component lifecycle
- Integration tests for WebSocket connection
- Performance benchmarks (Chrome DevTools)
- Cross-browser testing (Chrome, Firefox, Safari)

**Success Criteria:**
- ✅ All automated tests pass
- ✅ Performance metrics documented:
  - Frame rendering time <16ms (60fps)
  - Memory usage <100MB per session
  - Zero Chrome performance violations
- ✅ Manual testing checklist completed
- ✅ Cross-browser compatibility verified

**Testing Checklist:**
```markdown
- [ ] Terminal renders on page load
- [ ] User input works (typing, Enter, arrows)
- [ ] High-frequency output smooth (run `yes`)
- [ ] Terminal resizes with window
- [ ] Scrollback loads correctly
- [ ] Copy/paste works (browser native)
- [ ] ANSI colors display correctly
- [ ] Performance <16ms per frame
- [ ] No memory leaks (10min session)
- [ ] Works in Chrome, Firefox, Safari
```

**Dependencies:** All previous tasks (1.1-1.6)

**INVEST Validation:**
- ✅ Independent: Testing-only task
- ✅ Negotiable: Test coverage adjustable
- ✅ Valuable: Quality assurance
- ✅ Estimable: Standard testing (2h)
- ✅ Small: Test file + checklist
- ✅ Testable: All tests pass

---

## Story 2: Circular Buffer & Memory Management

### Objectives
- Prevent unbounded memory growth from terminal output
- Maintain fixed scrollback history (e.g., 10,000 lines)
- Provide efficient line-based access for search/features
- Integrate with xterm.js buffer management

### Dependencies
- Story 1 complete (xterm.js integrated)

---

### Task 2.1: Design Circular Buffer Data Structure (2h - Small)

**Scope:** Create efficient circular buffer for terminal lines

**Files:**
- `web-app/src/lib/terminal/CircularBuffer.ts` (new)
- `web-app/src/lib/terminal/CircularBuffer.test.ts` (new)

**Context:**
- Need O(1) append, O(1) indexed access
- Fixed capacity (e.g., 10,000 lines)
- Store line metadata (timestamp, sequence number)
- Support efficient iteration for search

**Implementation:**
```typescript
export class CircularBuffer<T> {
  private buffer: T[];
  private head = 0;
  private size = 0;

  constructor(private capacity: number) {
    this.buffer = new Array(capacity);
  }

  push(item: T): void {
    // O(1) append with wraparound
  }

  get(index: number): T | undefined {
    // O(1) indexed access
  }

  forEach(callback: (item: T, index: number) => void): void {
    // Efficient iteration
  }
}

interface TerminalLine {
  text: string;
  sequence: number;
  timestamp: number;
}
```

**Success Criteria:**
- ✅ CircularBuffer class with generic type
- ✅ O(1) push operation
- ✅ O(1) indexed get operation
- ✅ Efficient forEach iteration
- ✅ Comprehensive unit tests
- ✅ Performance validated with 10k+ items

**Testing:**
- Unit tests for all operations
- Performance benchmark: 10,000 pushes <1ms
- Verify memory usage constant after filling

**Dependencies:** None (foundation for Story 2)

**INVEST Validation:**
- ✅ Independent: Self-contained data structure
- ✅ Negotiable: Capacity configurable
- ✅ Valuable: Enables all memory management
- ✅ Estimable: Standard algorithm (2h)
- ✅ Small: Single class file
- ✅ Testable: Unit tests + benchmarks

---

### Task 2.2: Integrate Circular Buffer with xterm.js (3h - Medium)

**Scope:** Use CircularBuffer to limit xterm.js scrollback

**Files:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)
- `web-app/src/lib/hooks/useTerminalStream.ts` (modify)

**Context:**
- xterm.js has built-in buffer, but can grow unbounded
- Set xterm.js scrollback option to fixed size
- Mirror terminal output in CircularBuffer for features
- Keep buffers in sync (terminal write + buffer append)

**Implementation:**
```typescript
const terminal = new Terminal({
  scrollback: 10000,  // Match CircularBuffer capacity
  // ... other options
});

const lineBuffer = useRef(new CircularBuffer<TerminalLine>(10000));

// When terminal output received
const handleTerminalData = (text: string) => {
  terminal.write(text);

  // Parse lines and append to circular buffer
  const lines = parseTerminalLines(text);
  lines.forEach(line => {
    lineBuffer.current.push({
      text: line,
      sequence: sequenceCounter++,
      timestamp: Date.now(),
    });
  });
};
```

**Success Criteria:**
- ✅ Terminal scrollback limited to 10,000 lines
- ✅ Memory usage plateaus at ~50MB per session
- ✅ CircularBuffer stays synchronized with terminal
- ✅ Old lines discarded automatically
- ✅ No performance degradation over time
- ✅ Scrolling works correctly (oldest lines accessible)

**Testing:**
- Generate 50,000 lines of output
- Verify memory stays <100MB
- Verify scrollback contains only latest 10,000 lines
- Check Chrome DevTools memory profiler

**Dependencies:** Task 2.1 (CircularBuffer implemented)

**INVEST Validation:**
- ✅ Independent: Uses existing components
- ✅ Negotiable: Buffer size adjustable
- ✅ Valuable: Prevents memory leaks
- ✅ Estimable: Integration + parsing (3h)
- ✅ Small: 2 files, clear logic
- ✅ Testable: Memory profiling + unit tests

---

### Task 2.3: Add Configurable Buffer Size Setting (2h - Small)

**Scope:** Make scrollback buffer size user-configurable

**Files:**
- `web-app/src/lib/config/terminalConfig.ts` (new)
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)
- `web-app/src/app/settings/page.tsx` (modify if settings page exists)

**Context:**
- Different users have different memory/history tradeoffs
- Allow configuration: 1,000 / 10,000 / 50,000 / unlimited
- Store in localStorage
- Apply on terminal initialization

**Implementation:**
```typescript
interface TerminalConfig {
  scrollbackLines: number;
  theme: 'dark' | 'light';
  fontSize: number;
}

const DEFAULT_CONFIG: TerminalConfig = {
  scrollbackLines: 10000,
  theme: 'dark',
  fontSize: 14,
};

export function useTerminalConfig(): [TerminalConfig, (config: Partial<TerminalConfig>) => void] {
  // Load from localStorage
  // Provide update function
}
```

**Success Criteria:**
- ✅ Configuration stored in localStorage
- ✅ Terminal applies config on mount
- ✅ Config changes take effect immediately
- ✅ Invalid values rejected (validation)
- ✅ Default config provides sensible experience

**Testing:**
- Change buffer size, verify terminal respects it
- Test with various values (1k, 10k, 50k)
- Verify localStorage persistence
- Test invalid input handling

**Dependencies:** Task 2.2 (buffer integrated)

**INVEST Validation:**
- ✅ Independent: Configuration layer
- ✅ Negotiable: UI can vary
- ✅ Valuable: User customization
- ✅ Estimable: Standard config (2h)
- ✅ Small: Config file + integration
- ✅ Testable: Manual + unit tests

---

### Task 2.4: Memory Leak Testing and Documentation (2h - Small)

**Scope:** Comprehensive memory leak validation

**Files:**
- `web-app/docs/terminal-memory-management.md` (new)
- Create long-running test scenarios

**Context:**
- Validate memory usage over extended sessions
- Document memory characteristics
- Provide monitoring guidance
- Establish performance baselines

**Test Scenarios:**
- 1-hour session with continuous output
- 10 sessions running concurrently
- Session start/stop cycle (50 iterations)
- High-frequency output stress test

**Success Criteria:**
- ✅ No memory leaks over 1-hour session
- ✅ Memory usage plateaus at expected level
- ✅ Concurrent sessions scale linearly
- ✅ Session cleanup releases all memory
- ✅ Documentation provides monitoring guide

**Documentation Content:**
- Expected memory usage per session
- How to profile memory in Chrome DevTools
- Warning signs of memory leaks
- Configuration guidance for memory-constrained devices

**Testing:**
- Chrome DevTools Memory profiler
- Heap snapshot comparison
- Performance monitor over time
- Cross-browser validation

**Dependencies:** Task 2.3 (config integrated)

**INVEST Validation:**
- ✅ Independent: Testing and docs only
- ✅ Negotiable: Test duration adjustable
- ✅ Valuable: Quality assurance + documentation
- ✅ Estimable: Standard testing (2h)
- ✅ Small: Tests + markdown doc
- ✅ Testable: Automated + manual tests

---

## Story 3: Delta Compression Protocol

### Objectives
- Implement MOSH-style differential updates
- Reduce bandwidth by 70-90% for typical terminal usage
- Maintain backwards compatibility during rollout
- Support efficient terminal state synchronization

### Dependencies
- Story 1 complete (xterm.js foundation)
- Story 2 optional (circular buffer recommended)

---

### Task 3.1: Design Delta Compression Protocol (3h - Medium)

**Scope:** Define protobuf messages for terminal state diffs

**Files:**
- `proto/session/v1/events.proto` (modify)
- `docs/terminal-delta-protocol.md` (new)

**Context:**
- MOSH sends screen state diffs, not raw output
- Need to represent cursor position, line changes, insertions, deletions
- Support incremental acknowledgment for reliability
- Balance compression ratio vs CPU cost

**Protocol Design:**
```protobuf
message TerminalDelta {
  uint64 from_state = 1;     // Previous state version
  uint64 to_state = 2;       // New state version
  repeated LineDelta lines = 3;
  CursorPosition cursor = 4;
  bool full_sync = 5;        // Full state (not delta)
}

message LineDelta {
  uint32 line_number = 1;
  oneof operation {
    string replace_line = 2;   // Full line replacement
    LineEdit edit = 3;         // Character-level edit
    bool delete_line = 4;
    InsertLine insert = 5;
  }
}

message LineEdit {
  uint32 start_col = 1;
  uint32 end_col = 2;
  string text = 3;
}
```

**Success Criteria:**
- ✅ Protocol supports all terminal operations
- ✅ Delta messages smaller than full output
- ✅ Documentation explains design rationale
- ✅ Backwards compatibility path defined
- ✅ Error recovery mechanism specified

**Deliverables:**
- Updated `events.proto` with new messages
- Protocol documentation with examples
- Compression ratio estimates
- Compatibility matrix

**Dependencies:** None (design phase)

**INVEST Validation:**
- ✅ Independent: Design-only task
- ✅ Negotiable: Message structure flexible
- ✅ Valuable: Foundation for compression
- ✅ Estimable: Protocol design (3h)
- ✅ Small: Proto changes + docs
- ✅ Testable: Protocol validation

---

### Task 3.2: Implement Server-Side Terminal State Tracker (4h - Large)

**Scope:** Track terminal screen state for diff generation

**Files:**
- `session/terminal_state.go` (new)
- `session/terminal_state_test.go` (new)

**Context:**
- Need to maintain current terminal screen state (25 lines x 80 cols typical)
- Parse ANSI escape codes to update state
- Track cursor position, colors, attributes
- Generate diffs between states

**Implementation:**
```go
type TerminalState struct {
    Lines  []TerminalLine
    Cursor Position
    Width  int
    Height int
}

type TerminalLine struct {
    Cells []TerminalCell
}

type TerminalCell struct {
    Char  rune
    Style CellStyle
}

func (ts *TerminalState) ApplyOutput(data []byte) {
    // Parse ANSI codes, update state
}

func (ts *TerminalState) GenerateDelta(previous *TerminalState) *TerminalDelta {
    // Compare states, generate minimal diff
}
```

**Success Criteria:**
- ✅ Accurate terminal state tracking
- ✅ Correct ANSI escape code parsing
- ✅ Delta generation produces minimal diffs
- ✅ Performance <1ms for typical diff
- ✅ Comprehensive unit tests
- ✅ Handles edge cases (resize, clear screen, etc.)

**Testing:**
- Unit tests for ANSI parsing
- Diff generation correctness tests
- Performance benchmarks
- Integration test with real tmux output

**Dependencies:** Task 3.1 (protocol designed)

**INVEST Validation:**
- ✅ Independent: Server-side only
- ✅ Negotiable: Implementation approach flexible
- ✅ Valuable: Enables compression
- ✅ Estimable: Complex but bounded (4h)
- ✅ Small: 4 files max, single concern
- ✅ Testable: Extensive unit tests

---

### Task 3.3: Update StreamTerminal RPC to Support Deltas (3h - Medium)

**Scope:** Add delta mode to terminal streaming

**Files:**
- `server/services/terminal_websocket.go` (modify)
- `proto/session/v1/session.proto` (modify request options)

**Context:**
- Add capability negotiation (client requests delta mode)
- Maintain backwards compatibility (full output mode)
- Stream deltas instead of raw output
- Handle state synchronization errors (fallback to full sync)

**Implementation:**
```go
type StreamTerminalRequest struct {
    SessionId string
    EnableDeltaCompression bool  // New field
}

func (s *SessionServiceImpl) StreamTerminal(stream ConnectStream) error {
    useDelta := req.EnableDeltaCompression

    if useDelta {
        termState := NewTerminalState()
        // Send deltas
    } else {
        // Send raw output (current behavior)
    }
}
```

**Success Criteria:**
- ✅ Delta mode enabled via request flag
- ✅ Backwards compatibility maintained
- ✅ Delta messages transmitted correctly
- ✅ Error handling for state desync
- ✅ Performance equal or better than raw mode

**Testing:**
- Test delta mode on/off
- Verify backwards compatibility
- Test error recovery (corrupted state)
- Performance comparison

**Dependencies:** Task 3.2 (state tracker implemented)

**INVEST Validation:**
- ✅ Independent: RPC modification only
- ✅ Negotiable: Negotiation mechanism adjustable
- ✅ Valuable: Enables compression feature
- ✅ Estimable: RPC update (3h)
- ✅ Small: 2 files, clear scope
- ✅ Testable: Integration tests

---

### Task 3.4: Implement Client-Side Delta Application (3h - Medium)

**Scope:** Apply server deltas to xterm.js terminal

**Files:**
- `web-app/src/lib/terminal/DeltaApplicator.ts` (new)
- `web-app/src/lib/terminal/DeltaApplicator.test.ts` (new)
- `web-app/src/lib/hooks/useTerminalStream.ts` (modify)

**Context:**
- Receive delta messages from WebSocket
- Apply line edits to xterm.js terminal
- Update cursor position
- Handle full sync fallback

**Implementation:**
```typescript
export class DeltaApplicator {
  constructor(private terminal: Terminal) {}

  applyDelta(delta: TerminalDelta): void {
    // For each line delta
    delta.lines.forEach(lineDelta => {
      switch (lineDelta.operation.case) {
        case 'replaceLine':
          this.replaceLine(lineDelta.lineNumber, lineDelta.operation.value);
          break;
        case 'edit':
          this.editLine(lineDelta.lineNumber, lineDelta.operation.value);
          break;
        // ... other operations
      }
    });

    // Update cursor
    this.terminal.write(/* cursor move sequences */);
  }
}
```

**Success Criteria:**
- ✅ Deltas correctly applied to terminal
- ✅ Terminal display matches server state
- ✅ Cursor position synchronized
- ✅ Performance <1ms per delta
- ✅ Handles edge cases (screen resize, etc.)

**Testing:**
- Unit tests for delta operations
- Integration test with real deltas
- Visual verification (terminal matches expected)
- Performance benchmarks

**Dependencies:** Task 3.3 (server sends deltas)

**INVEST Validation:**
- ✅ Independent: Client-side only
- ✅ Negotiable: Application strategy adjustable
- ✅ Valuable: Completes compression pipeline
- ✅ Estimable: Delta application (3h)
- ✅ Small: 2 files, focused logic
- ✅ Testable: Unit + integration tests

---

### Task 3.5: Enable Delta Compression in useTerminalStream (2h - Small)

**Scope:** Integrate delta mode into WebSocket hook

**Files:**
- `web-app/src/lib/hooks/useTerminalStream.ts` (modify)
- `web-app/src/lib/config/terminalConfig.ts` (add option)

**Context:**
- Add `enableDeltaCompression` option to hook
- Request delta mode during handshake
- Route delta messages to DeltaApplicator
- Fall back to raw output if unsupported

**Implementation:**
```typescript
interface UseTerminalStreamOptions {
  // ... existing options
  enableDeltaCompression?: boolean;
}

export function useTerminalStream({
  enableDeltaCompression = true,  // Enable by default
  ...
}: UseTerminalStreamOptions) {
  // Send handshake with delta flag
  // Handle delta vs output messages
}
```

**Success Criteria:**
- ✅ Delta mode enabled by default
- ✅ Fallback to raw mode if not supported
- ✅ Hook API unchanged for existing users
- ✅ Configuration option in terminal config
- ✅ Works with all existing features

**Testing:**
- Test with delta-enabled server
- Test with legacy server (fallback)
- Verify all terminal features work
- Performance validation

**Dependencies:** Task 3.4 (delta applicator)

**INVEST Validation:**
- ✅ Independent: Hook modification
- ✅ Negotiable: Default behavior adjustable
- ✅ Valuable: Enables compression by default
- ✅ Estimable: Hook integration (2h)
- ✅ Small: Single file modification
- ✅ Testable: Integration tests

---

### Task 3.6: Bandwidth Measurement and Validation (2h - Small)

**Scope:** Measure compression effectiveness

**Files:**
- `web-app/src/lib/terminal/BandwidthMonitor.ts` (new)
- `docs/terminal-compression-benchmarks.md` (new)

**Context:**
- Track WebSocket bytes sent/received
- Compare delta mode vs raw mode
- Generate compression statistics
- Document real-world bandwidth savings

**Monitoring:**
```typescript
class BandwidthMonitor {
  private bytesSent = 0;
  private bytesReceived = 0;

  recordMessage(size: number, direction: 'sent' | 'received') {
    // Track bandwidth usage
  }

  getStats(): BandwidthStats {
    return {
      bytesSent: this.bytesSent,
      bytesReceived: this.bytesReceived,
      compressionRatio: this.calculateRatio(),
    };
  }
}
```

**Benchmark Scenarios:**
- Typical interactive session (vim, ls, cd)
- High-frequency output (yes, cat large file)
- Mixed workload (build process)
- Long-running session (1 hour)

**Success Criteria:**
- ✅ Bandwidth monitor accurately tracks data
- ✅ Compression ratio 70-90% for typical usage
- ✅ Documentation shows benchmarks
- ✅ Comparison with MOSH performance
- ✅ Real-world validation data

**Dependencies:** Task 3.5 (delta mode enabled)

**INVEST Validation:**
- ✅ Independent: Monitoring only
- ✅ Negotiable: Metrics adjustable
- ✅ Valuable: Validates compression benefit
- ✅ Estimable: Monitoring + docs (2h)
- ✅ Small: Single file + benchmark doc
- ✅ Testable: Automated benchmarks

---

## Story 4: Advanced Terminal Features

### Objectives
- Implement terminal search functionality
- Add theme support (dark/light modes)
- Enable session recording and replay
- Provide professional UX polish

### Dependencies
- Story 1 complete (xterm.js foundation)
- Story 3 optional (compression recommended)

---

### Task 4.1: Implement Terminal Search with xterm-addon-search (2h - Small)

**Scope:** Add search functionality to terminal

**Files:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)
- `web-app/src/components/sessions/TerminalSearchBar.tsx` (new)

**Context:**
- xterm-addon-search provides built-in search
- Add UI for search input and navigation
- Support regex and case-sensitive search
- Highlight search results in terminal

**Implementation:**
```typescript
import { SearchAddon } from 'xterm-addon-search';

const searchAddon = useRef(new SearchAddon());

// Load addon
terminal.loadAddon(searchAddon.current);

// Search controls
const handleSearch = (query: string, options: SearchOptions) => {
  searchAddon.current.findNext(query, options);
};
```

**Success Criteria:**
- ✅ Search input UI integrated
- ✅ Results highlighted in terminal
- ✅ Next/previous navigation works
- ✅ Regex and case-sensitive options
- ✅ Keyboard shortcuts (Ctrl+F)

**Testing:**
- Search for various patterns
- Test regex patterns
- Verify highlighting
- Test keyboard shortcuts

**Dependencies:** Story 1 (xterm.js)

**INVEST Validation:**
- ✅ Independent: Addon integration
- ✅ Negotiable: UI can vary
- ✅ Valuable: Professional feature
- ✅ Estimable: Standard addon (2h)
- ✅ Small: 2 files, clear scope
- ✅ Testable: Manual testing

---

### Task 4.2: Add Theme Support (Light/Dark Modes) (2h - Small)

**Scope:** Implement terminal color themes

**Files:**
- `web-app/src/lib/config/terminalThemes.ts` (new)
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)

**Context:**
- Define theme configurations (colors, cursor style)
- Support system preference detection
- Allow user override in settings
- Apply theme to terminal instance

**Implementation:**
```typescript
interface TerminalTheme {
  background: string;
  foreground: string;
  cursor: string;
  // ... other colors
}

const THEMES = {
  dark: { background: '#1e1e1e', foreground: '#cccccc', ... },
  light: { background: '#ffffff', foreground: '#333333', ... },
  monokai: { ... },
  solarized: { ... },
};

// Apply theme
terminal.options.theme = THEMES[themeName];
```

**Success Criteria:**
- ✅ Dark and light themes defined
- ✅ Theme applied to terminal
- ✅ System preference detection
- ✅ User can override theme
- ✅ Theme persists in config

**Testing:**
- Test all themes visually
- Verify system preference detection
- Test theme switching

**Dependencies:** Task 2.3 (config system)

**INVEST Validation:**
- ✅ Independent: Theme configuration
- ✅ Negotiable: Colors adjustable
- ✅ Valuable: User customization
- ✅ Estimable: Config + apply (2h)
- ✅ Small: Theme file + integration
- ✅ Testable: Visual verification

---

### Task 4.3: Terminal Session Recording Infrastructure (4h - Large)

**Scope:** Record terminal sessions for replay

**Files:**
- `web-app/src/lib/terminal/SessionRecorder.ts` (new)
- `session/recording.go` (new - server-side storage)
- `proto/session/v1/session.proto` (add recording methods)

**Context:**
- Record all terminal I/O with timestamps
- Store as asciicast v2 format (asciinema compatible)
- Support recording start/stop/pause
- Server-side or client-side recording

**Implementation:**
```typescript
export class SessionRecorder {
  private events: RecordingEvent[] = [];
  private startTime: number = 0;
  private isRecording: boolean = false;

  start(): void {
    this.startTime = Date.now();
    this.isRecording = true;
  }

  recordOutput(data: string): void {
    if (!this.isRecording) return;

    this.events.push({
      time: (Date.now() - this.startTime) / 1000,
      type: 'o',
      data,
    });
  }

  export(): string {
    return JSON.stringify({
      version: 2,
      width: 80,
      height: 25,
      events: this.events,
    });
  }
}
```

**Success Criteria:**
- ✅ Recording captures all terminal I/O
- ✅ Timestamps accurate
- ✅ Export to asciicast v2 format
- ✅ Recording can be paused/resumed
- ✅ Server-side storage optional
- ✅ Compatible with asciinema player

**Testing:**
- Record a session
- Export and verify format
- Play back in asciinema player
- Test pause/resume

**Dependencies:** Story 1 (xterm.js)

**INVEST Validation:**
- ✅ Independent: Recording infrastructure
- ✅ Negotiable: Storage location adjustable
- ✅ Valuable: Enables replay feature
- ✅ Estimable: Recording system (4h)
- ✅ Small: 3 files, focused scope
- ✅ Testable: Format validation

---

### Task 4.4: Terminal Session Replay Player (3h - Medium)

**Scope:** Play back recorded terminal sessions

**Files:**
- `web-app/src/components/sessions/TerminalReplay.tsx` (new)
- `web-app/src/lib/terminal/ReplayPlayer.ts` (new)

**Context:**
- Load asciicast recording
- Play back at original or adjustable speed
- Support play/pause/seek controls
- Use xterm.js for rendering

**Implementation:**
```typescript
export class ReplayPlayer {
  private events: RecordingEvent[];
  private currentIndex = 0;
  private playbackSpeed = 1.0;

  constructor(
    private terminal: Terminal,
    recording: string
  ) {
    const cast = JSON.parse(recording);
    this.events = cast.events;
  }

  play(): void {
    // Schedule events based on timestamps
  }

  pause(): void {
    // Pause playback
  }

  seek(timeSeconds: number): void {
    // Jump to time position
  }
}
```

**Success Criteria:**
- ✅ Recordings play back correctly
- ✅ Playback speed adjustable (0.5x-2x)
- ✅ Play/pause/seek controls work
- ✅ Timeline scrubber for seeking
- ✅ Supports asciicast v2 format

**Testing:**
- Play various recordings
- Test speed adjustment
- Test seek to various positions
- Verify display matches original

**Dependencies:** Task 4.3 (recording infrastructure)

**INVEST Validation:**
- ✅ Independent: Player component
- ✅ Negotiable: UI adjustable
- ✅ Valuable: Enables replay UX
- ✅ Estimable: Player implementation (3h)
- ✅ Small: 2 files, player logic
- ✅ Testable: Manual playback tests

---

### Task 4.5: Add Web Links Addon (1h - Micro)

**Scope:** Make URLs in terminal clickable

**Files:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (modify)

**Context:**
- xterm-addon-web-links detects URLs
- Makes them clickable with Ctrl+Click
- Opens in new tab
- Simple addon integration

**Implementation:**
```typescript
import { WebLinksAddon } from 'xterm-addon-web-links';

const webLinksAddon = new WebLinksAddon();
terminal.loadAddon(webLinksAddon);
```

**Success Criteria:**
- ✅ URLs detected and underlined on hover
- ✅ Ctrl+Click opens link in new tab
- ✅ Works with various URL formats

**Testing:**
- Output URLs in terminal (echo "https://...")
- Verify hover underline
- Test Ctrl+Click

**Dependencies:** Story 1 (xterm.js)

**INVEST Validation:**
- ✅ Independent: Simple addon
- ✅ Negotiable: N/A (standard addon)
- ✅ Valuable: Nice UX feature
- ✅ Estimable: Trivial addon (1h)
- ✅ Small: 2 lines of code
- ✅ Testable: Manual click test

---

### Task 4.6: Polish and Documentation (2h - Small)

**Scope:** Final polish and comprehensive docs

**Files:**
- `web-app/README.md` (update terminal section)
- `docs/terminal-features.md` (new comprehensive guide)
- Various CSS polish

**Context:**
- Document all terminal features
- Provide usage examples
- Screenshot/GIF demonstrations
- Configuration reference

**Documentation Sections:**
- Getting started
- Features overview (search, themes, recording)
- Configuration options
- Keyboard shortcuts reference
- Troubleshooting guide
- Performance tuning

**Success Criteria:**
- ✅ Comprehensive feature documentation
- ✅ All configuration options documented
- ✅ Screenshots/GIFs for major features
- ✅ Keyboard shortcuts reference
- ✅ Code examples provided

**Dependencies:** All previous tasks complete

**INVEST Validation:**
- ✅ Independent: Documentation only
- ✅ Negotiable: Content adjustable
- ✅ Valuable: User guidance
- ✅ Estimable: Documentation (2h)
- ✅ Small: Markdown files
- ✅ Testable: Docs review

---

## Dependency Visualization

```
Story 1: xterm.js Integration
├─ Task 1.1 (Install deps) ────────────────┐
│  ├─ Task 1.2 (XtermTerminal component)   │
│  │  ├─ Task 1.3 (useTerminalStream)      │
│  │  │  ├─ Task 1.4 (WebGL)               │
│  │  │  ├─ Task 1.5 (FitAddon)            │ (All parallel)
│  │  │  └─ Task 1.6 (Cleanup)             │
│  │  └─────────────────────────────────────┘
│  └─ Task 1.7 (E2E Testing) ◄─────────────── (After all)
│
Story 2: Circular Buffer ◄─────────────────── (After Story 1)
├─ Task 2.1 (CircularBuffer class) ──────────┐
│  ├─ Task 2.2 (Integration)                 │
│  │  ├─ Task 2.3 (Config)                   │ (Sequential)
│  │  └─ Task 2.4 (Testing)                  │
│  └─────────────────────────────────────────┘
│
Story 3: Delta Compression ◄────────────────── (After Story 1)
├─ Task 3.1 (Protocol design) ───────────────┐
│  ├─ Task 3.2 (Server state) ───────────────┤
│  │  ├─ Task 3.3 (RPC update)               │ (Sequential)
│  │  │  ├─ Task 3.4 (Client delta)          │
│  │  │  │  ├─ Task 3.5 (Hook integration)   │
│  │  │  │  └─ Task 3.6 (Validation)         │
│  └─────────────────────────────────────────┘
│
Story 4: Advanced Features ◄──────────────────── (After Story 1)
├─ Task 4.1 (Search) ─────────────────────────┐
├─ Task 4.2 (Themes) ─────────────────────────┤ (All parallel)
├─ Task 4.3 (Recording) ──────────────────────┤
│  └─ Task 4.4 (Replay) ◄──────────────────┤
├─ Task 4.5 (Web Links) ──────────────────────┤
└─ Task 4.6 (Docs) ◄──────────────────────────┘ (After all)
```

---

## Parallel Execution Opportunities

### Phase 1 - Foundation (Week 1)
**Parallel:**
- Story 1: Tasks 1.1-1.3 (xterm.js integration) - Developer A
- Story 2: Task 2.1 (CircularBuffer design) - Developer B

### Phase 2 - Performance (Week 1-2)
**Parallel:**
- Story 1: Tasks 1.4-1.6 (addons + cleanup) - Developer A
- Story 2: Tasks 2.2-2.3 (buffer integration + config) - Developer B

### Phase 3 - Compression (Week 2-3)
**Parallel:**
- Story 3: Tasks 3.1-3.2 (protocol + server) - Developer A
- Story 4: Tasks 4.1-4.2 (search + themes) - Developer B

### Phase 4 - Advanced (Week 3-4)
**Parallel:**
- Story 3: Tasks 3.3-3.5 (compression completion) - Developer A
- Story 4: Tasks 4.3-4.4 (recording + replay) - Developer B

### Phase 5 - Polish (Week 4-5)
**Sequential:**
- Story 3: Task 3.6 (validation)
- Story 4: Task 4.5-4.6 (final features + docs)
- Story 1: Task 1.7 (comprehensive E2E)
- Story 2: Task 2.4 (memory testing)

---

## Context Preparation Guide

### For Story 1 (xterm.js Integration)

**Required Reading:**
1. [xterm.js Documentation](https://xtermjs.org/)
2. Current `useTerminalStream.ts` implementation
3. Existing `TerminalOutput.tsx` component
4. WebSocket streaming architecture

**Files to Review:**
- `web-app/src/lib/hooks/useTerminalStream.ts`
- `web-app/src/components/sessions/TerminalOutput.tsx`
- `proto/session/v1/events.proto` (TerminalData messages)
- `server/services/terminal_websocket.go` (streaming implementation)

**Mental Model:**
- xterm.js uses canvas rendering (not React/DOM)
- Imperative API (not declarative React)
- Terminal is stateful (buffer, cursor position)
- WebSocket provides bidirectional byte stream

### For Story 2 (Circular Buffer)

**Required Reading:**
1. Circular buffer data structure overview
2. xterm.js buffer management API
3. Memory profiling in Chrome DevTools

**Files to Review:**
- `web-app/src/components/sessions/XtermTerminal.tsx` (from Story 1)
- Chrome memory profiling documentation

**Mental Model:**
- Fixed-size buffer with wraparound
- O(1) operations required for performance
- Memory usage should plateau at capacity
- Terminal scrollback vs search buffer considerations

### For Story 3 (Delta Compression)

**Required Reading:**
1. MOSH protocol whitepaper
2. Protobuf oneof usage
3. ANSI escape code parsing

**Files to Review:**
- `proto/session/v1/events.proto`
- `server/services/terminal_websocket.go`
- `web-app/src/lib/hooks/useTerminalStream.ts`

**Mental Model:**
- Terminal is grid of cells (rows x cols)
- State tracking for diff generation
- Delta = minimal changes to transform prev → current
- Compression ratio depends on change frequency

### For Story 4 (Advanced Features)

**Required Reading:**
1. xterm.js addons documentation
2. asciicast v2 format specification

**Files to Review:**
- `web-app/src/components/sessions/XtermTerminal.tsx`
- xterm.js addon examples

**Mental Model:**
- Addons extend terminal functionality
- Recording = timestamped event stream
- Replay = scheduled terminal.write() calls

---

## INVEST Validation Matrix

| Task | Independent | Negotiable | Valuable | Estimable | Small | Testable | Status |
|------|-------------|------------|----------|-----------|-------|----------|--------|
| 1.1  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 1.2  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 1.3  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 1.4  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 1.5  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 1.6  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 1.7  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 2.1  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 2.2  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 2.3  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 2.4  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 3.1  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 3.2  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 3.3  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 3.4  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 3.5  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 3.6  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 4.1  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 4.2  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 4.3  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 4.4  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 4.5  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |
| 4.6  | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | Ready |

**All tasks validated against enhanced INVEST criteria** ✅

---

## Integration Checkpoints

### Checkpoint 1: xterm.js Foundation (After Story 1)
**Validation:**
- [ ] Terminal renders correctly in all browsers
- [ ] WebSocket streaming works end-to-end
- [ ] All input/output functionality preserved
- [ ] Zero performance violations in Chrome
- [ ] Memory usage stable over 10-minute session

### Checkpoint 2: Memory Management (After Story 2)
**Validation:**
- [ ] Scrollback limited to configured size
- [ ] Memory plateaus at expected level
- [ ] No memory leaks over 1-hour session
- [ ] Performance unchanged with circular buffer

### Checkpoint 3: Delta Compression (After Story 3)
**Validation:**
- [ ] Bandwidth reduced by 70-90%
- [ ] Terminal display matches raw mode exactly
- [ ] Error recovery works (fallback to full sync)
- [ ] Performance equal or better than raw mode

### Checkpoint 4: Complete System (After Story 4)
**Validation:**
- [ ] All features working together
- [ ] Comprehensive documentation complete
- [ ] Production-ready performance
- [ ] User acceptance testing passed

---

## Implementation Timeline

**Total Estimated Time:** 58 hours (1.5 months at 40% allocation)

| Week | Story | Tasks | Hours | Parallel |
|------|-------|-------|-------|----------|
| 1 | Story 1 | 1.1-1.3 | 12h | Yes (2 devs) |
| 1-2 | Story 1 + 2 | 1.4-1.6, 2.1-2.2 | 11h | Yes (2 devs) |
| 2 | Story 2 + 4 | 2.3-2.4, 4.1-4.2 | 8h | Yes (2 devs) |
| 2-3 | Story 3 | 3.1-3.2 | 7h | Partial (design parallel) |
| 3 | Story 3 + 4 | 3.3-3.4, 4.3 | 10h | Yes (2 devs) |
| 3-4 | Story 3 + 4 | 3.5-3.6, 4.4 | 7h | Yes (2 devs) |
| 4 | Story 4 + Polish | 4.5-4.6, 1.7 | 5h | Sequential |

**Critical Path:** Story 1 → Story 3 (compression depends on xterm.js)
**Parallel Paths:** Story 2 (memory) and Story 4 (features) mostly independent

---

## Risk Mitigation

### Risk 1: xterm.js Integration Complexity
**Mitigation:** Start with simple wrapper (Task 1.2), validate early (Task 1.3)
**Fallback:** Keep old TerminalOutput as emergency rollback option

### Risk 2: Delta Compression Protocol Complexity
**Mitigation:** Design protocol first (Task 3.1), validate with prototypes
**Fallback:** Feature flag to disable compression, fall back to raw mode

### Risk 3: Performance Regressions
**Mitigation:** Performance testing after each story (checkpoints)
**Fallback:** Revert problematic changes, keep working features

### Risk 4: Browser Compatibility Issues
**Mitigation:** Test in Chrome, Firefox, Safari during Story 1
**Fallback:** Document known limitations, provide workarounds

---

## Success Metrics

### Performance
- ✅ Terminal rendering: <16ms per frame (60fps sustained)
- ✅ Memory usage: <100MB per session (long-running)
- ✅ Bandwidth: 70-90% reduction with delta compression
- ✅ Zero Chrome performance violations

### Functionality
- ✅ All existing terminal features working
- ✅ Search, themes, recording, replay implemented
- ✅ Copy/paste, selection, scrollback working
- ✅ Cross-browser compatibility (Chrome, Firefox, Safari)

### Quality
- ✅ Comprehensive test coverage (>80%)
- ✅ Documentation complete and accurate
- ✅ No memory leaks detected
- ✅ Production deployment successful

---

## References

- [xterm.js Documentation](https://xtermjs.org/)
- [MOSH Protocol Paper](https://mosh.org/mosh-paper.pdf)
- [asciicast v2 Format](https://github.com/asciinema/asciinema/blob/develop/doc/asciicast-v2.md)
- [Chrome Performance Best Practices](https://developer.chrome.com/docs/lighthouse/performance/)
- [Circular Buffer Algorithm](https://en.wikipedia.org/wiki/Circular_buffer)

---

## Next Steps

1. **Review and approve** this task breakdown
2. **Assign developers** to parallel tracks (Story 1 + Story 2)
3. **Create GitHub issues** for each task (use task IDs)
4. **Setup project board** with dependency visualization
5. **Begin Story 1, Task 1.1** - Install xterm.js dependencies

**Ready to start implementation!** 🚀
