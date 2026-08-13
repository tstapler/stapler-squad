# Rate Limit Dialog Features Research

**Research Date**: 2026-04-05  
**Researcher**: Claude Code (Stapler Squad)  
**Status**: Complete

## Executive Summary

This document catalogs the format and structure of rate limit dialogs from different LLM providers. The research identifies common patterns, dialog structures, and provider-specific behaviors that can be used to detect and automatically address rate limits in terminal-based LLM sessions.

---

## 1. Claude Code (Anthropic)

### Dialog Trigger

Claude Code displays a `/rate-limit-options` command interface when rate limits are hit. This appears as an interactive prompt that requires user input to proceed.

### Dialog Format

```
/rate-limit-options
```

### Options Presented (Numbered Menu)

Based on issue reports, Claude Code typically presents options including:
- **Option 1**: Wait for rate limit to reset (most common for auto-resumption)
- **Option 2**: Switch to a different model
- **Option 3**: Continue anyway (retry immediately)

The exact options may vary based on the rate limit type (RPM, TPM, or quota-based).

### Reset Time Information

Claude Code displays:
- A countdown or timestamp showing when the rate limit will reset
- The error message includes: `"rate_limit_error"` with message `"This request would exceed your account's rate limit. Please try again later."`
- Internal error format: `429 {"type":"error","error":{"type":"rate_limit_error","message":"...","request_id":"req_..."}}`

### Continue Prompt

After selecting "wait" option and the time elapsing, users typically need to type:
- `continue` - Most common
- `please continue` - Alternative
- `carry on` - Less common

**Important**: There is a known issue (#27471) requesting a dedicated `/rate-limit-resume` command to avoid the "thinking trace" problem where Claude misinterprets "continue" as a new task rather than resumption.

### Detection Patterns

| Pattern | Description |
|---------|-------------|
| `/rate-limit-options` | Primary command trigger |
| `rate_limit_error` | Error type in JSON |
| `429` | HTTP status code |
| `request would exceed your account's rate limit` | Error message substring |
| `Request was aborted` | Additional error type |

---

## 2. Aider

### Dialog Format

Aider handles rate limits differently - it typically fails silently or shows brief error messages in the terminal output.

### Error Messages Observed

**Direct Rate Limit Error**:
```
litellm.RateLimitError: ...
```

**Quota Exceeded (OpenRouter)**:
When using OpenRouter with free models, Aider may show no response at all - the session appears to hang without any error message displayed to the user.

**API Error Format**:
```
Error 429: Rate limit exceeded. Please retry after X seconds.
```

### Behavior Characteristics

- Less interactive than Claude Code
- May require manual re-entry of the last command
- Uses `litellm` library which wraps various LLM APIs
- Error handling varies by backend provider

### Detection Patterns

| Pattern | Description |
|---------|-------------|
| `RateLimitError` | Python exception type |
| `Rate limit exceeded` | Common error message |
| `429 Too Many Requests` | HTTP error string |
| `quota` | For quota-based limits |

---

## 3. OpenAI Codex

### Dialog Format

OpenAI Codex displays error messages directly in the terminal without an interactive menu.

### Error Messages

**Retry Exhaustion**:
```
exceeded retry limit, last status: 429 Too Many Requests
```

**With Request ID**:
```
exceeded retry limit, last status: 429 Too Many Requests, request id: <request-id>
```

**Quota Exceeded**:
```
quota has been hit
```

### Behavior Characteristics

- Automatic retry with exponential backoff
- Eventually fails after retry limit is exhausted
- Displays 429 status but may not show reset time
- No interactive continuation prompt - user must retry manually

### Detection Patterns

| Pattern | Description |
|---------|-------------|
| `exceeded retry limit` | Primary trigger |
| `429 Too Many Requests` | HTTP error |
| `quota has been hit` | Alternative trigger |
| `request id:` | Request tracking |

---

## 4. OpenClaw (OpenChat/Lobster)

### Dialog Format

OpenClaw displays an interactive prompt similar to Claude Code.

### Error Message
```
⚠️ API rate limit reached
```

### Behavior Characteristics

- Interactive prompt for user action
- Requires manual intervention to proceed
- Similar menu-based approach to Claude Code

---

## 5. Common Patterns Across Providers

### Detection Strategy

Despite different formats, several common patterns emerge:

1. **HTTP Status Code**: `429` appears across all providers
2. **Error Keywords**:
   - `rate limit`
   - `rate_limit`
   - `quota`
   - `too many requests`
3. **Reset Time Indicators**:
   - `retry after X seconds`
   - Timestamp display
   - Countdown timer

### Continue/Resume Patterns

| Provider | Resume Command(s) | Notes |
|----------|------------------|-------|
| Claude Code | `continue`, `please continue`, `carry on` | Most flexible - any continuation message works |
| Aider | Re-send original command | No built-in resume - must re-enter |
| Codex | Re-run command | Manual retry required |
| OpenClaw | Varies | Menu-driven |

### Interactive Menu Patterns

Providers that use numbered menus (Claude Code, OpenClaw):
- Option typically labeled with number + description: `1) Wait for rate limit to reset`
- Selection made by typing the number and pressing Enter
- After reset time: session returns to input state waiting for user command

---

## 6. Auto-Detection Recommendations

### Core Detection Regex Patterns

```regex
# Primary rate limit detection
/rate-limit-options|rate.?limit|429.*Too.?Many

# Reset time parsing
retry after (\d+) seconds|reset at (\d{1,2}:\d{2})

# Interactive prompt detection
^\d+\).*(wait|retry|continue|switch)
```

### Provider Identification

| Terminal Output Contains | Likely Provider |
|------------------------|-----------------|
| `/rate-limit-options` | Claude Code |
| `RateLimitError` | Aider |
| `exceeded retry limit` | OpenAI Codex |
| `⚠️ API rate limit` | OpenClaw |

### Resume Action Mapping

| Provider | Recommended Auto-Action | Input to Send |
|----------|------------------------|---------------|
| Claude Code | Send `continue` after delay | `continue\n` |
| Aider | Re-send last user message | Requires message history |
| Codex | Re-send last command | Requires command history |
| OpenClaw | Varies by menu | May need number input |

---

## 7. Technical Implementation Notes

### Terminal Output Processing

1. **Stream Analysis**: Parse terminal output in real-time as it's captured by the scrollback buffer
2. **Pattern Matching**: Use regex to identify rate limit dialogs without blocking the stream
3. **State Tracking**: Track whether rate limit mode is active vs. normal operation

### Time Parsing Formats

| Format Example | Parsing Approach |
|--------------|------------------|
| `retry after 60 seconds` | Parse integer after "retry after" |
| `reset at 14:30` | Parse time format HH:MM, calculate wait |
| `4:45 PM EST` | Parse with timezone, convert to local time |

### Edge Cases

1. **Partial Dialogs**: Some rate limit messages may be split across multiple terminal lines
2. **Repeated Messages**: Rate limit dialogs may appear multiple times
3. **Stale State**: After rate limit resets, session may still show old message until user input clears it
4. **Silent Failures**: Some providers (Aider/OpenRouter) may fail without visible error

---

## 8. References

- Claude Code Issue #27471: Feature request for `/rate-limit-resume`
- Claude Code Issue #18353: Rate limit options bug report
- Claude Code Issue #19591: Rate limit spam after ctrl+o
- Aider Issue #3264: OpenRouter free model rate limit
- OpenAI Codex Issue #12776: Retry limit exceeded

---

## 9. Open Questions

1. **Claude Code Menu Options**: Exact option numbers and text vary - need to test actual live session to capture complete menu
2. **Aider Resume**: How does Aider handle resumption - is there any automatic retry or must user re-type?
3. **Provider Updates**: Rate limit dialogs may change with software updates - version-specific patterns may be needed
4. **Multi-Model Sessions**: How does switching models affect rate limit state?

---

*Research completed for Stapler Squad rate limit auto-detection feature.*
