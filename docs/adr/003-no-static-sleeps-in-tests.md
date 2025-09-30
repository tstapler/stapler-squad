# ADR-003: No Static Sleeps in Tests

## Status
Accepted

## Context
Our test suite contains numerous instances of static `time.Sleep()` calls to wait for asynchronous operations like tmux session creation, git operations, and UI updates. This creates several problems:

1. **Flakiness**: Tests may pass or fail depending on system load and timing
2. **Slow execution**: Tests always wait for the maximum time, even when conditions are met early
3. **Maintenance burden**: Sleep durations need constant tuning for different environments
4. **Poor developer experience**: Developers lose confidence in test results

### Current Problematic Patterns
```go
// Anti-pattern: Static sleeps
time.Sleep(500 * time.Millisecond)
content, err := tmuxSession.CapturePaneContent()

// Anti-pattern: Manual retry loops with sleeps
for i := 0; i < 5; i++ {
    content, err = tmuxSession.CapturePaneContent()
    if err == nil && content != "" {
        break
    }
    time.Sleep(200 * time.Millisecond)
}
```

## Decision
We will eliminate all static `time.Sleep()` calls from tests and replace them with proper wait utilities that:

1. **Poll with exponential backoff** for better performance
2. **Have configurable timeouts** for different test environments
3. **Provide clear error messages** when conditions are not met
4. **Are reusable** across different test scenarios

## Implementation

### Wait Utilities
Create `testutil` package with polling utilities:

```go
// Wait for a condition to be true
func WaitForCondition(condition func() bool, timeout time.Duration) error

// Wait for tmux session to be ready
func WaitForTmuxSession(session *tmux.TmuxSession, timeout time.Duration) error

// Wait for content to be available
func WaitForContent(getter func() (string, error), validator func(string) bool, timeout time.Duration) error
```

### Test Environment Configuration
```go
var (
    DefaultTimeout = 10 * time.Second
    FastTimeout    = 2 * time.Second  // For unit tests
    SlowTimeout    = 30 * time.Second // For integration tests
)
```

### Preferred Patterns
```go
// Good: Use wait utilities
err := testutil.WaitForTmuxSession(tmuxSession, testutil.DefaultTimeout)
require.NoError(t, err)

// Good: Poll with validation
content, err := testutil.WaitForContent(
    func() (string, error) { return tmuxSession.CapturePaneContent() },
    func(content string) bool { return strings.Contains(content, expectedPath) },
    testutil.DefaultTimeout,
)
require.NoError(t, err)
```

## Consequences

### Positive
- **Faster tests**: Conditions are checked as soon as they're ready
- **More reliable**: Tests adapt to system performance variations
- **Better error messages**: Clear indication of what condition failed
- **Easier debugging**: Logs show polling attempts and final state

### Negative
- **Initial refactoring effort**: Need to update existing tests
- **Slight complexity increase**: More utility functions to maintain
- **Learning curve**: Developers need to use new patterns

## Migration Strategy

1. **Phase 1**: Create testutil package with wait utilities
2. **Phase 2**: Update failing integration tests to use new utilities
3. **Phase 3**: Gradually refactor remaining tests
4. **Phase 4**: Add linting rule to prevent new static sleeps

## Monitoring
- Track test execution time improvements
- Monitor test flakiness reduction
- Ensure no new static sleeps are introduced

## References
- [Google Testing Blog: Test Flakiness](https://testing.googleblog.com/2016/05/flaky-tests-at-google-and-how-we.html)
- [Go Testing Best Practices](https://go.dev/doc/tutorial/add-a-test)
- [Testcontainers Wait Strategies](https://testcontainers.com/guides/wait-strategies/)