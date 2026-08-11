# ADR-006: Async Event Loop Patterns in BubbleTea Applications

**Status:** Accepted
**Date:** 2025-01-17
**Authors:** Claude Code Assistant

## Context

BubbleTea applications use an event-driven architecture with a single main event loop goroutine that processes all UI messages and state updates. **Blocking this main event loop causes severe UI lag, input unresponsiveness, and poor user experience.**

During performance optimization work, we discovered multiple critical anti-patterns where `time.Sleep()` calls were blocking BubbleTea's main event loop for hundreds of milliseconds at a time:

```go
// ❌ BLOCKING ANTI-PATTERN - Never do this!
func badTickCommand() tea.Cmd {
    return func() tea.Msg {
        time.Sleep(500 * time.Millisecond) // BLOCKS MAIN UI THREAD!
        return someMessage{}
    }
}
```

This caused:
- **500ms UI freezes** every time the command executed
- **Unresponsive input** during sleep periods
- **Choppy navigation** and rendering
- **Poor user experience** especially with multiple tickers

## Decision

**CRITICAL RULE:** Never use `time.Sleep()` or any blocking operation within `tea.Cmd` functions.

### ✅ Correct Async Pattern

Always use `tea.Tick()` for timed operations:

```go
// ✅ CORRECT ASYNC PATTERN
func goodTickCommand() tea.Cmd {
    return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
        return someMessage{}
    })
}
```

### Implementation Requirements

1. **All tickers must use `tea.Tick()`** instead of `time.Sleep()`
2. **All background operations must be non-blocking** in the Update cycle
3. **Expensive operations must use goroutines** with message-based results
4. **Code reviews must check for blocking patterns**

### Fixed Patterns

We fixed these critical blocking patterns:

| File Location | Pattern | Fix |
|---------------|---------|-----|
| `tickUpdateMetadataCmd` | `time.Sleep(500ms)` | `tea.Tick(500ms, ...)` |
| `tickSessionDetectionCmd` | `time.Sleep(intervalMs)` | `tea.Tick(intervalMs, ...)` |
| `tickTerminalSizeCheckCmd` | `time.Sleep(250ms)` | `tea.Tick(250ms, ...)` |
| Preview tickers | `time.Sleep(50-100ms)` | `tea.Tick(50-100ms, ...)` |

## Consequences

### Benefits
- **Responsive UI**: No more 500ms freezes during navigation
- **Smooth input handling**: All key presses processed immediately
- **Better performance**: Main event loop never blocks
- **Scalable architecture**: Multiple tickers don't interfere

### Risks
- **Developer education**: Team must understand this critical pattern
- **Code review vigilance**: Must catch blocking patterns in reviews
- **Migration effort**: Existing blocking code needs fixing

## Compliance

### Code Review Checklist
- [ ] No `time.Sleep()` calls in `tea.Cmd` functions
- [ ] All tickers use `tea.Tick()` pattern
- [ ] Background operations use goroutines + messages
- [ ] No blocking I/O in Update() method

### Detection
```bash
# Search for potential blocking patterns
grep -r "time\.Sleep.*tea\.Msg" .
grep -r "time\.Sleep.*Millisecond" . | grep -v "_test.go"
```

### Monitoring
- Navigation benchmarks must show consistent <100ms response times
- UI responsiveness testing with multiple active sessions
- Performance regression testing for blocking patterns

## References

- [BubbleTea Documentation](https://github.com/charmbracelet/bubbletea)
- [Go Concurrency Patterns](https://golang.org/doc/effective_go.html#concurrency)
- Related: [ADR-003: No Static Sleeps in Tests](003-no-static-sleeps-in-tests.md)

---

**⚠️ CRITICAL**: Violating this pattern causes immediate, severe UI performance degradation. This is a **zero-tolerance** architectural requirement.