# Flow Control Implementation - Test Plan

## Overview

This document describes how to test the xterm.js flow control implementation following best practices from:
- https://xtermjs.org/docs/guides/flowcontrol/
- https://xtermjs.org/docs/api/terminal/classes/terminal/

## Implementation Status

### ✅ Completed (Client-Side)

1. **Write Callbacks** - Terminal writes use callbacks to track completion
2. **Watermark Tracking** - HIGH/LOW watermark thresholds (100KB/10KB)
3. **Escape Sequence Parser** - Prevents splitting ANSI codes mid-sequence

### ⏭️ Pending

4. **FlowControl Protobuf Message** - Server communication protocol
5. **WebSocket Pause/Resume** - Send signals to server
6. **Server-Side PTY Buffering** - Server honors pause/resume

## Current Behavior

**What Works:**
- ✅ Write callbacks track when xterm.js completes parsing data
- ✅ Watermark increases when data is written, decreases when parsed
- ✅ Console warnings when HIGH_WATERMARK (100KB) exceeded
- ✅ Console logs when LOW_WATERMARK (10KB) reached
- ✅ Escape sequences (colors, cursor positioning) are not split
- ✅ Debug logging shows write completion and watermark levels

**What's NOT Implemented Yet:**
- ❌ Actual pause/resume signals to server (logs "TODO" messages)
- ❌ Server-side PTY buffering control
- ❌ Flow control over WebSocket

**Current Limitation:**
The client-side watermark tracking and logging works, but the server continues sending data even when the client exceeds HIGH_WATERMARK. This is expected until Tasks 4-7 are completed.

## Manual Testing Procedure

### Prerequisites

1. **Enable Debug Mode** (shows flow control logs):
   ```javascript
   // In browser console:
   localStorage.setItem('debug-terminal', 'true');
   ```

2. **Server Running**:
   ```bash
   cd /Users/tylerstapler/IdeaProjects/claude-squad
   ./claude-squad
   ```

3. **Web UI Open**:
   - Navigate to http://localhost:8543
   - Click "New Session" (or press `n` in TUI)

### Test Case 1: Write Callback Tracking

**Purpose**: Verify write callbacks are working

**Steps**:
1. Open browser console (F12)
2. Enable debug mode: `localStorage.setItem('debug-terminal', 'true')`
3. Refresh the page
4. In the terminal session, type: `echo "Hello World"`
5. Press Enter

**Expected Results**:
```
[XtermWrite] { writeCount: 1, dataLength: 13, timeSinceLastWrite: "0.00ms", ... }
[FlowControl] Write completed { bytes: 13, watermark: 0, paused: false, ... }
```

**Success Criteria**:
- ✅ `[XtermWrite]` log appears for each write
- ✅ `[FlowControl] Write completed` appears after write
- ✅ `watermark` value matches byte count

### Test Case 2: Watermark Threshold Detection

**Purpose**: Verify HIGH_WATERMARK detection (100KB limit)

**Steps**:
1. Ensure debug mode is enabled
2. In terminal, run the test script:
   ```bash
   bash /tmp/test-flow-control.sh
   ```
3. Watch browser console for flow control messages

**Expected Results**:
```
[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: 102400 bytes)
[FlowControl] Write completed { watermark: 95000, paused: true, ... }
[FlowControl] Write completed { watermark: 88000, paused: true, ... }
...
[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: 9500 bytes)
```

**Success Criteria**:
- ✅ Warning log when watermark exceeds 100,000 bytes
- ✅ `paused: true` flag set correctly
- ✅ Resume log when watermark drops below 10,000 bytes
- ✅ Terminal rendering remains smooth (no freezing)

### Test Case 3: Escape Sequence Integrity

**Purpose**: Verify ANSI codes are not split mid-sequence

**Steps**:
1. Run script with colored output:
   ```bash
   for i in {1..100}; do echo -e "\x1b[31mRed\x1b[0m \x1b[32mGreen\x1b[0m"; done
   ```
2. Observe terminal output visually

**Expected Results**:
- ✅ Colors render correctly (red "Red", green "Green")
- ✅ No garbled escape sequences visible (no `[31m` text shown)
- ✅ No partial sequences logged in console

**Success Criteria**:
- ✅ All text is properly colored
- ✅ No visible escape sequence characters
- ✅ Smooth rendering without flicker

### Test Case 4: High-Throughput Stress Test

**Purpose**: Verify system handles large data volumes

**Steps**:
1. Run dictionary dump (200KB+):
   ```bash
   cat /usr/share/dict/words
   ```
2. Monitor watermark in console logs

**Expected Results**:
```
[FlowControl] HIGH WATERMARK EXCEEDED - Pausing stream (watermark: 105000 bytes)
[FlowControl] Write completed { watermark: 98000, pending: 5, ... }
[FlowControl] Write completed { watermark: 91000, pending: 4, ... }
...
[FlowControl] LOW WATERMARK REACHED - Resuming stream (watermark: 9000 bytes)
```

**Success Criteria**:
- ✅ Multiple pause/resume cycles observed
- ✅ No browser tab crash or freeze
- ✅ Terminal remains responsive to user input
- ✅ All dictionary words render completely

### Test Case 5: Cursor Positioning Sequences

**Purpose**: Verify CSI sequences (cursor movement) are handled

**Steps**:
1. Run cursor positioning test:
   ```bash
   for i in {1..10}; do echo -e "\x1b[10;20HPosition $i"; done
   ```
2. Observe cursor movement

**Expected Results**:
- ✅ Text appears at consistent position (row 10, column 20)
- ✅ No visible partial CSI sequences like `[10;20H`
- ✅ Smooth cursor updates

**Success Criteria**:
- ✅ Cursor positioning works correctly
- ✅ No garbled CSI sequences
- ✅ Debug logs show complete sequences processed

## Automated Testing (Future)

### Unit Tests for EscapeSequenceParser

Create test file: `web-app/src/lib/terminal/__tests__/EscapeSequenceParser.test.ts`

```typescript
import { EscapeSequenceParser } from '../EscapeSequenceParser';

describe('EscapeSequenceParser', () => {
  let parser: EscapeSequenceParser;

  beforeEach(() => {
    parser = new EscapeSequenceParser();
  });

  test('buffers partial CSI sequence', () => {
    const chunk1 = 'Hello \x1b[31';  // Partial: \x1b[31
    const result1 = parser.processChunk(chunk1);
    expect(result1).toBe('Hello ');
    expect(parser.getBuffered()).toBe('\x1b[31');

    const chunk2 = 'mWorld';  // Completes: \x1b[31m
    const result2 = parser.processChunk(chunk2);
    expect(result2).toBe('\x1b[31mWorld');
  });

  test('passes complete sequences through', () => {
    const chunk = 'Hello \x1b[31mRed\x1b[0m World';
    const result = parser.processChunk(chunk);
    expect(result).toBe(chunk);  // No buffering needed
  });

  test('handles OSC sequences with BEL terminator', () => {
    const chunk1 = 'Title: \x1b]0;My';  // Partial OSC
    const result1 = parser.processChunk(chunk1);
    expect(result1).toBe('Title: ');

    const chunk2 = ' Title\x07Done';  // Complete with BEL
    const result2 = parser.processChunk(chunk2);
    expect(result2).toBe('\x1b]0;My Title\x07Done');
  });
});
```

### Integration Tests

```typescript
// Test watermark tracking
test('watermark exceeds HIGH threshold', async () => {
  const terminal = createTestTerminal();
  const largeData = 'x'.repeat(150000);  // 150KB

  terminal.write(largeData);

  // Check console for HIGH_WATERMARK warning
  expect(console.warn).toHaveBeenCalledWith(
    expect.stringContaining('HIGH WATERMARK EXCEEDED')
  );
});
```

## Performance Metrics

### Expected Metrics (from xterm.js docs)

- **Write Latency**: p95 < 100ms
- **Watermark Range**: 10KB - 100KB (optimal for 60fps rendering)
- **Memory Growth**: Capped at HIGH_WATERMARK + scrollback
- **Callback Overhead**: < 5% CPU

### Measurement Commands

```javascript
// Measure write performance
console.time('write100KB');
terminal.write('x'.repeat(100000), () => {
  console.timeEnd('write100KB');
});

// Monitor memory
console.log('Memory:', performance.memory?.usedJSHeapSize / 1024 / 1024 + 'MB');
```

## Known Limitations (Current Phase)

1. **Server-Side**: Server does NOT pause PTY when client watermark exceeded
   - Server continues sending data at full speed
   - Client buffers data until LOW_WATERMARK reached
   - **Mitigation**: Use high scrollback buffer (10,000 lines)

2. **WebSocket Buffering**: No backpressure to server yet
   - WebSocket may accumulate messages if client is slow
   - **Mitigation**: Browser WebSocket buffer (usually 1-2MB)

3. **No Circuit Breaker**: If watermark never drops, no automatic recovery
   - **Mitigation**: Manual browser refresh (planned for Story 3)

## Next Steps

After successful testing, implement:

1. **Task 4**: Add FlowControl message to protobuf
2. **Task 5**: Implement WebSocket pause/resume signaling
3. **Story 2**: Server-side PTY buffering control

## Success Criteria for Phase 1

- ✅ Write callbacks functioning correctly
- ✅ Watermark tracking with HIGH/LOW detection
- ✅ Escape sequences not split mid-sequence
- ✅ Console logs show flow control events
- ✅ No browser crashes with large output (200KB+)
- ✅ Terminal remains responsive during high throughput

## References

- xterm.js Flow Control Guide: https://xtermjs.org/docs/guides/flowcontrol/
- xterm.js Terminal API: https://xtermjs.org/docs/api/terminal/classes/terminal/
- Task Documentation: `/Users/tylerstapler/IdeaProjects/claude-squad/docs/tasks/terminal-flow-control.md`
