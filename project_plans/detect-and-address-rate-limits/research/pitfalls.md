# Pitfalls and Failure Modes: Rate Limit Detection and Auto-Resolution

**Research Date**: 2026-04-05  
**Related Requirements**: `requirements.md`

This document catalogs known failure modes and pitfalls for building an automated rate limit detection and resolution system for LLM sessions.

---

## 1. False Positives — Detecting Rate Limits When There Aren't Any

### 1.1 Generic Error Message Matching

**Problem**: Overly broad regex patterns that match any message containing words like "rate", "limit", "429", or "try again" will trigger on unrelated content.

**Examples**:
- User output mentioning "rate limiting on our side"
- Error messages from other services that use similar terminology
- Log messages showing "trying again" in normal retry contexts

**Mitigation**:
- Require provider-specific context (e.g., "Anthropic", "OpenAI", "Claude" in proximity)
- Match against known dialog structures, not just keywords
- Implement a confidence threshold requiring multiple indicators

### 1.2 ANSI Escape Code Manipulation

**Problem**: Malicious or accidental ANSI escape codes in terminal output can create fake dialog prompts. Attackers could display fake rate limit dialogs to trick automated systems into sending input.

**Reference**: [Trail of Bits: Deceiving users with ANSI terminal codes in MCP](https://blog.trailofbits.com/2025/04/29/deceiving-users-with-ansi-terminal-codes-in-mcp/)

**Mitigation**:
- Strip ANSI codes before pattern matching
- Validate prompt structure (e.g., numbered options with specific format)
- Consider requiring user confirmation for first-time automation

### 1.3 Multi-Line Context Triggers

**Problem**: Pattern matching on individual lines can trigger on partial matches that aren't actual rate limit dialogs.

**Example**:
```
$ grep -i rate limit
warning: rate limit may apply to this command
```

**Mitigation**:
- Match multi-line dialog structures (header + options)
- Require specific layout patterns (e.g., "❯" or numbered options)
- Validate against known dialog templates per provider

---

## 2. False Negatives — Missing Actual Rate Limit Dialogs

### 2.1 Provider Message Format Variations

**Problem**: Each LLM provider uses different message formats. Patterns that work for one provider may not match others.

**Known Formats**:

| Provider | Key Phrases | Timestamp Format | Dialog Options |
|----------|-------------|------------------|----------------|
| Anthropic (Claude Code) | "API Error", "rate_limit_error", "/rate-limit-options" | Variable | "1. Stop and wait", "2. Upgrade" |
| OpenAI | "429", "RateLimitError", "Too Many Requests" | `retry_after` header | Retry prompt |
| Google Gemini | "429", "RESOURCE_EXHAUSTED", "Please wait and try again later" | Variable | N/A (API-level) |

**Reference**: [GitHub Issue #18353](https://github.com/anthropics/claude-code/issues/18353) - Claude Code rate limit options dialog

**Mitigation**:
- Implement provider-specific pattern registries
- Support multiple pattern variants per provider
- Include version-specific patterns (providers change formats)

### 2.2 Internationalization (i18n)

**Problem**: Error messages may appear in different languages based on user locale settings.

**Examples**:
- Spanish: "Límite de tasa excedido"
- French: "Limite de taux dépassée"
- Japanese: "レート制限を超過しました"

**Mitigation**:
- Support locale-aware pattern matching
- Include common translations for key phrases
- Fall back to HTTP status codes (429) as universal indicator

### 2.3 Terminal Display Artifacts

**Problem**: Terminal output may be truncated, wrapped, or reformatted, breaking pattern matching.

**Examples**:
- Long messages wrapped at 80 columns
- Progress bars overwriting rate limit messages
- Cursor positioning commands obscuring text

**Mitigation**:
- Strip terminal control sequences before matching
- Allow for whitespace variation in patterns
- Match on core message content, not exact formatting

### 2.4 Scrollback Buffer Race Conditions

**Problem**: If rate limit dialog appears and is immediately scrolled out of the visible viewport before detection runs, it may be missed.

**Reference**: Terminal scrollback buffering behavior varies by terminal emulator.

**Mitigation**:
- Maintain large scrollback buffer (capture all output)
- Detect rate limits from buffered history, not just current viewport
- Periodically scan recent buffer additions

---

## 3. Timing Issues — Sending Input Too Early or Too Late

### 3.1 Parsed Timestamp Accuracy

**Problem**: Rate limit messages display different timestamp formats that may be difficult to parse accurately.

**Examples**:
- "Rate limit will reset at 3:45 PM"
- "try again in 5 minutes"
- "Reset at: 2026-04-05T15:45:00Z"
- Relative: "in approximately 3 minutes"

**Reference**: [GitHub Issue #35011](https://github.com/anthropics/claude-code/issues/35011) - Surface rate limit details in error messages

**Mitigation**:
- Support multiple timestamp format patterns
- Implement timezone handling
- Add parsing confidence validation
- Consider using max(parsed time, current time + buffer)

### 3.2 Waiting Too Long (Missed Window)

**Problem**: Sending input after the reset window has passed but the session has moved on to a different state.

**Scenarios**:
- User manually intervened between detection and auto-resolution
- Session entered a different waiting state
- Network issues caused session to restart

**Reference**: [GitHub Issue #26699](https://github.com/anthropics/claude-code/issues/26699) - Session permanently stuck after transient rate limit

**Mitigation**:
- Verify session state immediately before sending input
- Implement state validation before triggering
- Add timeout/fallback if session doesn't respond to auto-input

### 3.3 Waiting Too Early (Input Ignored)

**Problem**: Sending "continue" or "1" before the rate limit has actually reset. The input may be consumed or ignored.

**Scenarios**:
- tmux input buffer filled but not yet processed
- Program hasn't checked for rate limit clearance
- Network latency between input and actual reset

**Mitigation**:
- Add buffer time (e.g., wait 5-10 seconds after parsed reset time)
- Implement retry logic if initial input doesn't work
- Verify rate limit is cleared by checking for re-appearance

### 3.4 Clock Skew

**Problem**: System clock differs from provider's clock, causing miscalculation of reset times.

**Mitigation**:
- Use NTP-synchronized system time
- Include network latency estimates in calculations
- Add safety buffer (e.g., wait 30 seconds after parsed reset)

---

## 4. Edge Cases — Dialogs That Don't Match Expected Patterns

### 4.1 Multiple Concurrent Rate Limits

**Problem**: Session hits multiple rate limits (e.g., TPM and RPM) simultaneously, creating complex dialog state.

**Reference**: [GitHub Issue #40621](https://github.com/anthropics/claude-code/issues/40621) - Rate limit reached repeatedly

**Mitigation**:
- Handle nested/sequential rate limit dialogs
- Prioritize resolution by longest wait time
- Track multiple pending resets

### 4.2 Authentication vs. Rate Limit Errors

**Problem**: Authentication errors ("API key invalid") look similar to rate limits but require different handling.

**Distinction**:
- Rate limit: "Please try again later", retry timer visible
- Auth error: "Invalid API key", no retry timer

**Mitigation**:
- Separate pattern sets for auth vs. rate limit
- Validate API key configuration separately
- Don't attempt auto-resolution for auth failures

### 4.3 Quota Exhaustion vs. Rate Limiting

**Problem**: "You exceeded your current quota" (billing issue) is different from rate limiting (temporary).

**Reference**: [OpenAI 429 Error: "You Exceeded Your Current Quota"](https://coldfusion-example.blogspot.com/2026/02/fixing-openai-api-error-429-you.html)

**Distinction**:
- Rate limit: Temporary, resets after time window
- Quota exhausted: Requires payment/action, doesn't auto-reset

**Mitigation**:
- Distinguish between "rate limit" and "quota" messages
- Don't attempt auto-resolution for quota exhaustion
- Log and alert for quota exhaustion errors

### 4.4 Partial/Corrupted Dialogs

**Problem**: Network issues or terminal glitches cause incomplete dialog messages.

**Examples**:
- Truncated error message missing timestamp
- Half-rendered ANSI codes breaking pattern
- Split messages across multiple screen updates

**Mitigation**:
- Require minimum required fields before acting
- Allow partial matches with confidence thresholds
- Implement timeout for incomplete dialogs

### 4.5 Nested Interactive Prompts

**Problem**: Rate limit dialog appears while another interactive prompt is active.

**Example**: User is editing a file when rate limit hits; the rate limit prompt may be nested in the editor interface.

**Mitigation**:
- Detect rate limit regardless of current input context
- Support interrupting other prompts when needed
- Queue resolution actions appropriately

---

## 5. Race Conditions Between Detection and User Interaction

### 5.1 User Intervenes Before Automation

**Problem**: User manually responds to rate limit dialog before automated system can act.

**Scenarios**:
- User types "continue" manually
- User selects different option than automation would
- User restarts the session

**Mitigation**:
- Implement detection cooldown (don't auto-resolve immediately)
- Allow user to disable auto-resolution per-session
- Check for recent user input before sending automated input

### 5.2 Dual Input Conflicts

**Problem**: Both user and automated system send input simultaneously, causing unexpected behavior.

**Reference**: [GitHub Issue #23513](https://github.com/anthropics/claude-code/issues/23513) - Race condition on pane spawn

**Mitigation**:
- Implement input locking/mutex
- Detect user input presence before sending
- Use tmux's `send-keys` with proper sequencing

### 5.3 Detection vs. Stream Latency

**Problem**: Terminal output streaming has latency; detection runs on buffered data that may be stale.

**Reference**: [amux: Detecting When Your AI Agent Dies](https://amux.io/blog/auto-restart-ai-agents/) - Screen-scraping approaches

**Mitigation**:
- Minimize latency between terminal and detection
- Mark detection data with timestamp
- Re-verify state immediately before acting

### 5.4 Session State Changes During Wait

**Problem**: Session is paused, detached, or terminates during the wait period before auto-resolution.

**Reference**: [GitHub Issue #21747](https://github.com/anthropics/claude-code/issues/21747) - Rate limit options repeatedly queried after session pause

**Mitigation**:
- Verify session is running before sending input
- Handle session pause/detach gracefully
- Resume monitoring after session reattach

---

## 6. Implementation-Specific Pitfalls

### 6.1 Scrollback Buffer Overflow

**Problem**: Large session output causes scrollback buffer to drop old content, potentially including rate limit dialog history.

**Mitigation**:
- Implement unbounded or large scrollback buffer
- Stream rate limit detection in real-time, not from buffer
- Persist detection state to survive buffer overflow

### 6.2 Memory Pressure from Continuous Monitoring

**Problem**: Continuous terminal output parsing consumes memory if not properly managed.

**Mitigation**:
- Process output in chunks, not all at once
- Implement sliding window for recent output
- Limit concurrent monitoring sessions

### 6.3 tmux send-keys Timing

**Problem**: Sending keys to tmux session may fail if session is busy or not ready.

**Reference**: [tmux-python race condition issues](https://github.com/tmux-python/libtmux/issues/624)

**Mitigation**:
- Add delays between keystrokes for complex input
- Verify session exists and is running before sending
- Implement retry logic with backoff

### 6.4 Process Lifecycle Management

**Problem**: Detected rate limit but the target process (Claude/Aider) has exited or been replaced.

**Mitigation**:
- Track process PID alongside session
- Validate process is still the expected one before sending input
- Handle process replacement gracefully

---

## 7. Summary of Mitigation Strategies

| Category | Key Mitigation |
|----------|----------------|
| False Positives | Provider-specific patterns, ANSI stripping, confidence thresholds |
| False Negatives | Multiple pattern variants, i18n support, large scrollback buffer |
| Timing Issues | Clock synchronization, safety buffers, state validation |
| Edge Cases | Separate handling for auth/quota, partial match support |
| Race Conditions | Input mutex, user input detection, session state validation |

---

## 8. Open Questions for Design

1. **User Control**: Should users be able to opt-in/out of auto-resolution per session or globally?

2. **Confirmation Mode**: Should auto-resolution require user confirmation for first-time execution?

3. **Logging**: How detailed should rate limit detection/resolution logging be for debugging?

4. **Provider Updates**: How will pattern registries stay current as providers change message formats?

5. **Error Recovery**: What happens when auto-resolution fails — alert user vs. retry vs. ignore?

---

## References

- [GitHub: Claude Code Issue #18353 — /rate-limit-options bug](https://github.com/anthropics/claude-code/issues/18353)
- [GitHub: Claude Code Issue #18980 — Auto-continue feature request](https://github.com/anthropics/claude-code/issues/18980)
- [GitHub: Claude Code Issue #26699 — Session stuck after rate limit](https://github.com/anthropics/claude-code/issues/26699)
- [GitHub: Claude Code Issue #21747 — Rate limit options after session pause](https://github.com/anthropics/claude-code/issues/21747)
- [GitHub: Claude Code Issue #35011 — Surface rate limit details](https://github.com/anthropics/claude-code/issues/35011)
- [Trail of Bits: ANSI terminal code deception](https://blog.trailofbits.com/2025/04/29/deceiving-users-with-ansi-terminal-codes-in-mcp/)
- [amux: Detecting AI agent death by terminal reading](https://amux.io/blog/auto-restart-ai-agents/)
- [OpenAI: How to handle rate limits](https://developers.openai.com/cookbook/examples/how_to_handle_rate_limits/)
