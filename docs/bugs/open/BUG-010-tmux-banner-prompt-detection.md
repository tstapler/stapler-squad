# BUG-010: Tmux Banner and Prompt Detection Failures [SEVERITY: High]

**Status**: 🔍 Investigating
**Discovered**: 2025-12-05 during test stabilization work
**Impact**: Session startup detection broken, tests timeout waiting for prompts

## Problem Description

Tests that verify tmux session startup and prompt detection are failing. The system cannot reliably detect when a tmux session is ready for commands.

**Failing tests** (suspected):
1. Tests that wait for shell prompt
2. Tests that detect command completion
3. Tests that read terminal output
4. Session readiness detection

**Symptoms**:
- Tests timeout waiting for shell prompt
- Banner messages (motd, etc.) interfere with prompt detection
- Command output parsing fails
- Session appears stuck in "Starting" state

## Reproduction

```bash
# Run tests that interact with tmux
go test ./session/tmux -v

# Likely output: Timeouts or failures in prompt detection
```

**Expected**: Tests detect prompt quickly and reliably
**Actual**: Tests timeout or fail to detect prompt

## Root Cause Analysis

**Investigation required** - Potential causes:

### 1. Shell Initialization Interference

```bash
# Shell initialization files may output banners
~/.bashrc, ~/.zshrc, ~/.profile
# May print: MOTD, updates, warnings, etc.

# These messages appear before prompt
# Interfere with prompt detection regex
```

**Example interference**:
```
Last login: Thu Dec 5 10:30:00 2025
You have new mail.
Updates available: 42 packages
$ ← Actual prompt buried in noise
```

### 2. Prompt Format Variations

```bash
# Different shells have different prompt formats
bash: user@host:dir$
zsh: %n@%m %~ %#
fish: user@host dir>

# Test expectations may assume specific format
# Actual prompt doesn't match regex
```

### 3. Timing Issues

```go
// Race condition: tmux session not fully initialized
// Prompt appears after test starts reading
// Or prompt detection regex runs before output available
```

**Timeline issue**:
1. Test starts tmux session
2. Test immediately tries to read prompt ❌ Too early
3. Shell still initializing
4. Prompt appears later (missed by test)

### 4. Terminal Escape Sequences

```bash
# Prompts may contain ANSI escape codes
# Color codes: \033[1;32m
# Cursor control: \033[H
# These break simple string matching
```

**Example**:
```
\033[1;32muser@host\033[0m:\033[1;34m~/dir\033[0m$ ← Prompt with colors
```

Regex expecting `$` alone will miss colored prompt.

### 5. PTY Configuration Issues

```go
// PTY might not be properly configured
// Echo settings, line buffering, etc.
// Causes output to arrive in unexpected ways
```

## Files Affected (Unknown)

Investigation needed to determine affected files:
- `session/tmux/` - tmux integration code
- `session/tmux_test.go` - Failing tests (if exists)
- `session/instance.go` - Session readiness detection (possibly)
- `testutil/` - Test utilities for tmux (possibly)

**Context boundary**: ⚠️ Unknown (requires investigation)

## Investigation Steps

### Phase 1: Capture Actual Output (30 minutes)

```bash
# Create tmux session manually
tmux new-session -d -s test_session

# Capture all output
tmux capture-pane -t test_session -p > tmux_output.txt

# Check for banners
head -20 tmux_output.txt

# Find prompt
grep -E '\$|%|>' tmux_output.txt
```

### Phase 2: Analyze Detection Logic (1 hour)

```go
// Find prompt detection code
grep -r "prompt" session/tmux/
grep -r "PS1" session/

// Check regex patterns
grep -r "regexp\|Regexp" session/tmux/

// Review timeout logic
grep -r "timeout\|Timeout" session/tmux/
```

### Phase 3: Test Different Shells (30 minutes)

```bash
# Test with minimal shell config
tmux new-session -d -s test_bash bash --norc --noprofile
tmux new-session -d -s test_zsh zsh -f

# Test prompt detection with clean shells
# Compare with default shell behavior
```

### Phase 4: Fix Detection Logic (2-3 hours)

Based on findings, implement robust detection:

**Option 1: Custom Prompt Marker**
```bash
# Set known prompt in tmux session
export PS1="STAPLER_SQUAD_READY> "

# Detection becomes trivial
waitFor("STAPLER_SQUAD_READY> ")
```

**Option 2: Strip ANSI Codes**
```go
// Remove escape sequences before matching
func stripANSI(s string) string {
    re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
    return re.ReplaceAllString(s, "")
}
```

**Option 3: Sentinel Command**
```bash
# Send unique command after session start
echo "READY_MARKER_12345"

# Wait for marker in output
waitFor("READY_MARKER_12345")
```

**Option 4: Timeout with Retry**
```go
// Try multiple detection methods
func waitForPrompt(session *tmux.Session, timeout time.Duration) error {
    // Try method 1: Prompt regex
    // Try method 2: Sentinel command
    // Try method 3: Just wait fixed time
    // Return when any succeeds
}
```

## Expected Fix Outcomes

After investigation and fixes:
- Prompt detection works reliably across shells ✅
- Tests don't timeout waiting for prompt ✅
- Banner interference handled gracefully ✅
- Different prompt formats supported ✅
- Tests pass on different systems/shells ✅

## Impact Assessment

**Severity**: **High**
- **User-Facing**: Indirect (session startup may be slow or unreliable)
- **Data Loss**: No
- **Workaround**: Disable banner messages, use specific shell
- **Frequency**: Every session creation, every test run
- **Scope**: All tmux-based sessions, entire test suite

**Priority**: P2 - Blocks test stabilization, may affect production

**Timeline**:
- Phase 1 (Capture output): 30 minutes
- Phase 2 (Analyze logic): 1 hour
- Phase 3 (Test shells): 30 minutes
- Phase 4 (Fix): 2-3 hours
- **Total**: 4-5 hours

## Prevention Strategy

**Robust detection**:
1. Support multiple prompt formats (bash, zsh, fish)
2. Strip ANSI codes before pattern matching
3. Use sentinel commands for readiness detection
4. Add timeout with progressive fallback methods

**Test infrastructure**:
1. Use controlled shell environment (--norc)
2. Set known prompt format (PS1)
3. Add debug logging for detection attempts
4. Test on multiple shells in CI

**Documentation**:
1. Document shell compatibility requirements
2. Explain prompt detection mechanism
3. Provide troubleshooting guide for failures
4. Add FAQ for common issues

## Related Issues

- **BUG-009**: Session package test failures (high, open)
- **BUG-008**: Category rendering in tests (critical, open)
- **Test Stabilization Epic**: See `docs/tasks/test-stabilization-and-teatest-integration.md`

## Additional Notes

**Common pitfalls**:

1. **MOTD on servers**: SSH sessions may show large banners
2. **Package manager updates**: apt/yum show available updates
3. **tmux message bar**: tmux itself shows messages at bottom
4. **Shell startup errors**: Errors in .bashrc appear before prompt
5. **Slow initialization**: Network mounts, large history files

**Best practice solution**:

Use **sentinel command approach** (most reliable):

```go
// Start session
session.Start()

// Send unique marker command
session.SendCommand("echo 'READY_MARKER_" + uniqueID + "'")

// Wait for marker in output
session.WaitForOutput("READY_MARKER_" + uniqueID, 5*time.Second)

// Session guaranteed ready
```

**Why this works**:
- Doesn't depend on prompt format
- Ignores banner messages
- Shell must be ready to execute command
- Unique marker prevents false matches
- Works across all shells

**Recommendation**: Implement sentinel command approach as primary method, with prompt regex as fallback.

---

**Bug Tracking ID**: BUG-010
**Related Feature**: tmux Integration (session/tmux/)
**Fix Complexity**: Medium (2-3 files, detection logic)
**Fix Risk**: Medium (core session functionality)
**Blocked By**: Investigation needed (Phase 1-2)
**Blocks**: Test stabilization, session reliability
**Related To**: BUG-009 (session test failures)
