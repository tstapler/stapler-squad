## Current Vulnerabilities

### 1. ANSI stripping is incomplete
`stripANSI()` in `detector.go` uses:
```go
var ansiStripRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`)
```
This covers CSI sequences (`\x1b[...`) and OSC sequences (`\x1b]...\x07`). It does NOT cover:
- `\x1b[` sequences with intermediate bytes (rare but valid: `\x1b[?25l` cursor hide — actually covered since `l` is `[a-zA-Z]`)
- `\x1bO` (SS3) sequences used by some terminals for function keys
- `\x1b=` / `\x1b>` (keypad mode) — these are 2-byte sequences, not matched
- **Most importantly**: carriage returns (`\r`) used for in-place animation. Claude's "Thinking..." spinner emits `⠋ Thinking\r⠙ Thinking\r...` — after ANSI stripping, the `\r` characters remain, causing the text to appear as multiple lines when split on `\n`, while the actual visible content is only the last version. The `Detect()` method operates on the full byte slice without preprocessing `\r`.

### 2. No scrollback tail limit for no-controller sessions
For sessions without an active `ClaudeController`, `getContent()` calls `inst.Preview()` which calls `tmuxManager.CapturePaneContent()`. This returns the **current visible pane** (typically 50 lines), not a tail of the scrollback. Pattern detection therefore only sees what's currently on screen — if the active status line has scrolled off screen, detection will miss it.

For sessions WITH a controller, `GetRecentOutput(0)` returns the entire 1MB PTY circular buffer (from `claude_controller.go` line 113: `NewCircularBuffer(1 * 1024 * 1024)`). The `statusDetectionTailBytes = 4096` constant is defined but `DetectRecent()` is not called — `Detect()` is called directly with the full buffer content from `GetRecentOutput(0)`. This means 1MB of data is scanned for every detection cycle.

### 3. `detectProcessing()` uses bare string matching without ANSI stripping
```go
for _, pattern := range processingPatterns {
    if strings.Contains(recentContent, pattern) {
        return true
    }
}
```
The `content` passed to `detectProcessing()` is the raw terminal content from `Preview()` or `ctrl.GetRecentOutput(0)`, which may still contain ANSI escape sequences. The string "Thinking..." could be interrupted by a color-reset sequence like `\x1b[0m` between the "T" and "hinking". This would cause a false negative.

### 4. Polling delay creates a 2-second window of misclassification
When a session transitions from idle to active (e.g., user sends a message and Claude starts responding), the `ReviewQueuePoller` continues to poll every 2 seconds. During this window, the session may still appear in the queue. The `IdleDetector.RecordActivity()` is called immediately on PTY output, but the queue removal only happens on the next `checkSession()` tick.

This is the core bug described in the requirements: a session generating output is surfaced in the review queue for up to 2 seconds after it starts responding.

### 5. Content cache invalidation race
`getContent()` uses `lastSeenActivity` (a `map[string]time.Time` protected by `cacheMu`) as the cache key for sessions with controllers. The cache is invalidated when `statusInfo.IdleState.LastActivity` advances. However, between when `IdleDetector.RecordActivity()` updates `lastActivity` and when the poller reads `statusInfo.IdleState.LastActivity`, there is a window where the cached (stale) content is used. The mutex scoping is:
```go
// getContent:
rqp.cacheMu.Lock()
lastSeen := rqp.lastSeenActivity[inst.Title]
cached := rqp.cachedContent[inst.Title]
rqp.cacheMu.Unlock()

if lastActivity.Equal(lastSeen) {
    return cached  // ← uses stale content
}
```
The `lastActivity` value is read outside the lock from `statusInfo`, which was fetched before `getContent()` was called. The actual `IdleDetector.lastActivity` may have advanced between those two calls. In practice this is a very short window (sub-millisecond) and is unlikely to cause observable problems, but it is technically a TOCTOU.

## Pitfall Catalog

### P-1: Pattern fragility — Claude Code UI changes
**Risk**: Claude Code periodically changes its UI text. The "Thinking..." pattern requires the word to appear at the start of a line; "esc to interrupt" exact text could become "esc to stop" or similar. The `claude_status_patterns.yaml` file is a stub that overrides the defaults; if it is populated with overly strict patterns, UI changes break detection silently.

**Evidence**: `tests_failing_test.go` line 9: `t.Skip("StatusTestsFailing patterns are disabled to prevent false positives")` — this is a documented case where patterns were too broad and had to be disabled entirely.

**Mitigation**: Snapshot tests in `session/detection/testdata/` — fixture files capture real terminal output. Currently 10 fixtures exist. Populating them (they start as empty stubs) is the golden-state capture infrastructure described in the requirements.

### P-2: False positive — "esc to interrupt" appearing in session content
A session could output the literal text "esc to interrupt" as part of documentation, error messages, or copied text. For example, a test suite printing "Usage: press esc to interrupt execution" would trigger `StatusActive`.

**Current mitigation**: None explicit in the pattern. The `esc_to_interrupt` pattern is `esc\s+(to\s+)?(interrupt|cancel)` — no anchoring to a specific screen region.

**Recommended mitigation**: Only match in the last N lines (tail approach), or add a negative lookahead to exclude common prose contexts. Alternatively, require the pattern to appear on a line that also contains a spinner char or duration indicator.

### P-3: False positive — cost summary line "$X.XX •" appearing in discussion
A user could send Claude a message like "the bill was $3.50 • per unit" which would match a cost summary pattern if one were added. The `•` character is somewhat distinctive but not unique.

**Recommended mitigation**: Require the cost pattern at the end of a line or in the last 3 lines of output: `\$\d+\.\d+\s+•.*$` with `(?m)` and only in the tail.

### P-4: Scrollback limit — old output masking current state
For controller sessions, `GetRecentOutput(0)` returns up to 1MB of data. If a session has been running for a long time and produced significant output, the `statusDetectionTailBytes = 4096` constant is defined but NOT used (see Vulnerability #2 above). The full buffer content is passed to `Detect()`. This means a pattern from 1000 lines ago could match and return a stale status (e.g., "esc to interrupt" from an old tool call that has since completed).

**Fix**: Change detection calls to use `DetectRecent(output, statusDetectionTailBytes)` instead of `Detect(output)`. The constant is already defined — it just needs to be wired in.

### P-5: Debounce delay causing missed transitions
`IdleDetectorConfig.DebounceDelay = 500ms`. State transitions are suppressed for 500ms after a change. If a session rapidly transitions Active → Idle → Active (e.g., a fast tool call), the debounce may cause the "Active" state to be missed, leaving the session in the queue even though it was briefly working.

### P-6: No-controller sessions have coarser detection
Sessions without `ClaudeController` (external terminal attaches, sessions started before the server) use `tmux capture-pane` output. This only shows the current visible pane (50 lines by default), not the PTY stream. Fast-moving output that has already scrolled off screen is invisible to the detector.

### P-7: `LastMeaningfulOutput` update path is complex and fragile
`ReviewState.UpdateTimestamps()` is called only from:
1. `HasUpdated()` in `instance.go` — triggered by `WebSocket streaming` when users view the terminal
2. `UpdateTerminalTimestamps(content, forceUpdate=true)` — triggered by user interactions

If a user is NOT viewing the terminal, `LastMeaningfulOutput` is never updated, and the staleness check (`timeSinceOutput > 2m`) will eventually fire even for actively working sessions. This is mitigated by `IdleDetector.RecordActivity()` being used as the cache invalidation key for controller sessions, but the staleness threshold path checks `inst.GetTimeSinceLastMeaningfulOutput()` which does NOT use `IdleDetector.lastActivity`.

## Mitigation Strategies

| Pitfall | Mitigation |
|---|---|
| P-1: Pattern fragility | Snapshot test fixtures with real output; YAML pattern override file; pattern version tracking |
| P-2: "esc to interrupt" false positive | Require tail-only matching (last 5 lines); require co-occurrence with spinner/progress indicator |
| P-3: Cost line false positive | Anchor to tail of output; require specific format `$\d+\.\d+ •` at EOL |
| P-4: Scrollback masking current state | Use `DetectRecent(output, 4096)` instead of `Detect(output)` — constant is already defined |
| P-5: Debounce missing transitions | Reduce debounce to 200ms or make Active→any transition immediate |
| P-6: No-controller detection | Increase capture-pane line count; add scrollback tail endpoint |
| P-7: `LastMeaningfulOutput` fragility | Wire controller's `RecordActivity()` to also update `inst.LastMeaningfulOutput` directly |

## Test Gaps

1. **E2E tests for review queue are smoke tests only** (`tests/e2e/review-queue.spec.ts`): 3 tests, all UI-only — no test for "working session not in queue", "stale detection", or "false positive from content".

2. **No integration test for the `detectProcessing()` function** — it has 4 signals and hardcoded strings, but no dedicated test file.

3. **`snapshot_test.go` fixture files are empty stubs** — `session/detection/testdata/*.txt` files exist as stubs per `TestSnapshotDetection`. The test fails with a descriptive message if any fixture is empty, requiring manual population. The requirement for "golden-state capture infrastructure" maps exactly to this — the infrastructure exists but the golden states have not been captured.

4. **No test for the poller's 2-second window bug** — the timing-dependent false-positive where a working session appears in the queue for 1-2 seconds is not covered by any test.

5. **No test for ANSI + `\r` combination** — the carriage return pitfall (P-1 + ANSI) has no test case in `detector_test.go`.
