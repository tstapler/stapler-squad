# Memory Leak Testing Guide

Comprehensive testing procedures for validating terminal memory stability and performance in the claude-squad web UI.

## Quick Validation Checklist

Before deploying any terminal changes:

- ✅ No memory growth during idle periods (5-10 minutes)
- ✅ Memory stabilizes during continuous output (<100MB per session)
- ✅ No WebGL context warnings in console
- ✅ Frame times consistently <16ms (60fps)
- ✅ Scrollback buffer respects configured limits
- ✅ Terminal recreation only on necessary config changes

## Browser DevTools Memory Profiling

### Setup

1. **Open Chrome DevTools** (F12)
2. **Navigate to Memory tab**
3. **Take baseline heap snapshot**
4. **Clear console** to remove noise

### Test Procedures

#### Test 1: Idle Memory Stability

**Purpose**: Verify no memory leaks during idle periods

**Steps**:
```bash
1. Load application at http://localhost:8543
2. Create a new session (or open existing)
3. Take heap snapshot ("Baseline")
4. Wait 5 minutes with no activity
5. Take heap snapshot ("After 5 min idle")
6. Compare snapshots
```

**Expected Results**:
- Memory growth <5MB over 5 minutes
- No detached DOM nodes accumulating
- No leaked xterm.js Terminal objects

**Red Flags**:
- Memory steadily increasing during idle
- Growing number of Terminal instances
- Accumulating event listeners

#### Test 2: Continuous Output Memory

**Purpose**: Verify memory stabilizes during high-frequency output

**Steps**:
```bash
1. Open session terminal
2. Take heap snapshot ("Baseline")
3. Run high-frequency output: `yes | head -n 10000`
4. Wait for output to complete
5. Take heap snapshot ("After output")
6. Calculate memory delta
```

**Expected Results**:
- Memory increase ~5MB per 1,000 lines (with 10k scrollback)
- Memory stabilizes after scrollback buffer fills
- No unbounded growth with continued output

**Memory Estimates by Configuration**:
```typescript
scrollbackLines: 1,000   → ~5MB memory usage
scrollbackLines: 10,000  → ~50MB memory usage
scrollbackLines: 50,000  → ~250MB memory usage
scrollbackLines: 100,000 → ~500MB memory usage
```

**Red Flags**:
- Memory continues growing beyond scrollback capacity
- Memory doesn't stabilize after buffer fills
- Memory leak in WebSocket message handling

#### Test 3: Terminal Recreation Memory

**Purpose**: Verify old terminal instances are properly disposed

**Steps**:
```bash
1. Open terminal with initial config
2. Take heap snapshot ("Baseline")
3. Change fontSize (triggers terminal recreation)
4. Wait 30 seconds for GC
5. Take heap snapshot ("After fontSize change")
6. Force garbage collection (DevTools → Memory → 🗑️ icon)
7. Take heap snapshot ("After GC")
```

**Expected Results**:
- Old Terminal instances collected by GC
- No increase in detached nodes
- WebGL contexts properly released

**Red Flags**:
- Multiple Terminal instances in heap
- Detached canvas elements
- Event listeners not cleaned up

#### Test 4: Long-Running Session

**Purpose**: Validate memory stability over extended periods

**Steps**:
```bash
1. Create new session
2. Take heap snapshot ("T0 - Start")
3. Run continuous output for 1 hour:
   while true; do echo "$(date) - Test output"; sleep 1; done
4. Take heap snapshots every 15 minutes
5. Monitor memory trend
```

**Expected Results**:
- Memory plateaus after scrollback fills
- No steady upward trend
- GC effectively reclaims old buffers

**Memory Trend Analysis**:
```
T0:  50MB  (Initial)
T15: 70MB  (Scrollback filling)
T30: 85MB  (Scrollback full)
T45: 85MB  (Stable)
T60: 85MB  (Stable)
```

**Red Flags**:
- Continuous upward trend
- Memory doubling over time
- Heap snapshots show accumulating objects

## Performance Testing

### Chrome DevTools Performance Tab

#### Frame Rate Analysis

**Steps**:
```bash
1. Open Performance tab
2. Enable "Screenshots" and "Memory"
3. Start recording
4. Run high-frequency output: `yes | head -n 5000`
5. Stop recording after completion
6. Analyze flame chart
```

**Expected Results**:
- Frame times consistently <16ms (60fps)
- No long tasks >50ms
- Rendering stays on main thread budget

**Red Flags**:
- Frames dropping below 30fps
- Long tasks blocking main thread
- Excessive style recalculation

#### Rendering Performance

**Focus Areas**:
- **Scripting**: Should dominate (xterm.js rendering)
- **Rendering**: Should be minimal (no DOM manipulation)
- **Painting**: Should be minimal (canvas-based)
- **System**: Should be low (efficient operations)

**Performance Metrics**:
```
Target: 60fps (16.67ms per frame)
Acceptable: 30fps (33.33ms per frame)
Poor: <30fps or variable frame times
```

### WebGL Context Monitoring

**Console Checks**:
```javascript
// Check active WebGL contexts
console.log('WebGL contexts:', performance.memory);

// Monitor for context loss
canvas.addEventListener('webglcontextlost', (e) => {
  console.error('WebGL context lost:', e);
});

// Monitor for context restoration
canvas.addEventListener('webglcontextrestored', (e) => {
  console.log('WebGL context restored:', e);
});
```

**Expected**:
- One WebGL context per terminal
- No context loss warnings
- Contexts properly released on cleanup

**Red Flags**:
- "Too many active WebGL contexts" warning
- Context loss during normal operation
- Contexts not released after terminal destruction

## Configuration Testing

### Test Each Preset

Test all configuration presets for memory stability:

```typescript
const presets = [
  'low-memory',      // 1,000 scrollback
  'default',         // 10,000 scrollback
  'high-performance', // 5,000 scrollback
  'debugging',       // 50,000 scrollback
  'accessibility',   // 10,000 scrollback
];
```

**For Each Preset**:
1. Apply preset via `applyConfigPreset(presetName)`
2. Run continuous output test
3. Measure memory usage
4. Verify matches expected capacity
5. Check performance metrics

### Dynamic Configuration Changes

**Test Scenarios**:
```bash
# Test 1: Theme change (should NOT recreate terminal)
localStorage.setItem('claude-squad-terminal-config',
  JSON.stringify({ theme: 'light' }));
window.dispatchEvent(new CustomEvent('terminal-config-changed',
  { detail: loadedConfig }));

# Test 2: Font size change (should NOT recreate terminal)
# Should update dynamically and refit

# Test 3: Scrollback change (SHOULD recreate terminal)
# Should dispose old terminal and create new one
```

**Expected Behavior**:
- Theme/fontSize: Dynamic update, no recreation
- Scrollback/WebGL: Terminal recreation required
- Old instances properly disposed

## Common Memory Leak Patterns

### Pattern 1: Event Listener Leaks

**Symptom**: Memory grows with each terminal recreation

**Detection**:
```javascript
// In Chrome DevTools Console
getEventListeners(window);
getEventListeners(document);
```

**Fix**: Ensure cleanup in useEffect return:
```typescript
useEffect(() => {
  const handler = (e) => { /* ... */ };
  window.addEventListener('event', handler);

  return () => {
    window.removeEventListener('event', handler);
  };
}, []);
```

### Pattern 2: Ref Leaks

**Symptom**: Old Terminal instances in heap after recreation

**Detection**: Heap snapshot shows multiple Terminal objects

**Fix**: Null out refs in cleanup:
```typescript
return () => {
  terminal.dispose();
  terminalRef.current = null;
  fitAddonRef.current = null;
};
```

### Pattern 3: Callback Closure Leaks

**Symptom**: Growing memory from captured props/state

**Detection**: Heap snapshot shows large closure scopes

**Fix**: Use refs for callbacks:
```typescript
const callbackRef = useRef(callback);
useEffect(() => { callbackRef.current = callback; }, [callback]);
```

### Pattern 4: Circular Buffer Not Respecting Capacity

**Symptom**: Memory grows beyond configured limit

**Detection**: Monitor heap size during continuous output

**Fix**: Verify CircularBuffer wraparound logic:
```typescript
// Should wrap and overwrite oldest
this.head = (this.head + 1) % this.capacity;
```

## Automated Testing

### Jest Performance Tests

```typescript
// memory.test.ts
describe('Memory Management', () => {
  it('should not leak memory with multiple renders', () => {
    const { rerender, unmount } = render(<XtermTerminal />);

    for (let i = 0; i < 100; i++) {
      rerender(<XtermTerminal fontSize={14 + (i % 5)} />);
    }

    unmount();

    // Check for cleanup
    expect(getAllTerminalInstances()).toHaveLength(0);
  });
});
```

### Puppeteer Long-Running Tests

```javascript
// long-running.test.js
test('Memory stability over 1 hour', async () => {
  const page = await browser.newPage();
  await page.goto('http://localhost:8543');

  const samples = [];

  for (let i = 0; i < 60; i++) {
    const metrics = await page.metrics();
    samples.push({
      time: i,
      jsHeapUsed: metrics.JSHeapUsedSize,
      jsHeapTotal: metrics.JSHeapTotalSize,
    });

    await page.waitForTimeout(60000); // 1 minute
  }

  // Verify memory plateaus
  const growth = samples[59].jsHeapUsed - samples[10].jsHeapUsed;
  expect(growth).toBeLessThan(50 * 1024 * 1024); // <50MB growth
});
```

## Troubleshooting Guide

### Issue: Memory Continuously Growing

**Diagnosis**:
1. Take heap snapshot and look for growing arrays
2. Check for detached DOM nodes
3. Monitor event listener count
4. Profile with Performance tab

**Common Causes**:
- Output not respecting scrollback limit
- Event listeners not cleaned up
- WebSocket messages accumulating
- Terminal instances not disposed

**Solutions**:
- Verify CircularBuffer capacity enforcement
- Add cleanup in useEffect returns
- Clear message queues on disconnect
- Ensure terminal.dispose() called

### Issue: Poor Frame Rates

**Diagnosis**:
1. Record Performance profile
2. Identify long tasks
3. Check for excessive rendering
4. Monitor style/layout recalculation

**Common Causes**:
- React state updates on every character
- DOM manipulation in render loop
- Expensive component re-renders
- Layout thrashing

**Solutions**:
- Use callback-based output (bypass React state)
- Leverage canvas/WebGL rendering
- Memoize expensive components
- Batch updates with RAF

### Issue: WebGL Context Warnings

**Diagnosis**:
```javascript
// Check context count
document.querySelectorAll('canvas').forEach(canvas => {
  console.log(canvas.getContext('webgl') ? 'Has WebGL' : 'No WebGL');
});
```

**Common Causes**:
- Terminal recreation without cleanup
- Multiple terminals with WebGL
- Browser context limit exceeded

**Solutions**:
- Properly dispose old terminals
- Fallback to canvas for non-primary terminals
- Reduce number of concurrent terminals

## Testing Schedule

### Before Each PR

- ✅ Run automated Jest tests
- ✅ Quick memory check (Test 1: Idle stability)
- ✅ Performance validation (frame rates)

### Before Each Release

- ✅ Full memory test suite (Tests 1-4)
- ✅ All configuration presets tested
- ✅ Long-running session test (1 hour)
- ✅ Cross-browser testing (Chrome, Firefox, Safari)

### Continuous Monitoring

- Monitor production memory metrics
- Track frame rate metrics
- Alert on memory threshold breaches
- Regular long-running session tests

## Acceptance Criteria

Story 2 (Memory Management) is complete when:

- ✅ CircularBuffer tests pass (22/22)
- ✅ Memory stable during idle periods (<5MB growth/5min)
- ✅ Memory plateaus during continuous output
- ✅ Scrollback limits enforced correctly
- ✅ Terminal recreation properly disposes old instances
- ✅ Frame rates consistently >30fps
- ✅ No WebGL context warnings
- ✅ All configuration presets tested
- ✅ Documentation complete

## References

- [Chrome DevTools Memory Profiling](https://developer.chrome.com/docs/devtools/memory-problems/)
- [xterm.js Performance Guide](https://github.com/xtermjs/xterm.js/blob/master/docs/guides/performance.md)
- [React Memory Leaks](https://react.dev/learn/synchronizing-with-effects#how-to-handle-the-effect-firing-twice-in-development)
- [WebGL Context Best Practices](https://www.khronos.org/webgl/wiki/HandlingContextLost)
