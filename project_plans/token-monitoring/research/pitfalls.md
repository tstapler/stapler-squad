# Pitfalls Research — Token Monitoring

## JSONL Parsing Edge Cases

### Partial Writes (Active Sessions)
**Risk:** Claude Code writes JSONL lines incrementally during an active session. A `bufio.Scanner` reading a file mid-write may see a truncated (incomplete) JSON line. If the scanner reads to EOF while a line is being written, the partial line will fail JSON parsing.

**Mitigation:**
- Use `json.Unmarshal` on each line with error swallowing — `if err != nil { continue }` skips malformed lines.
- The requirements explicitly state: "Handle malformed or partial JSONL without crashing" (FR-1).
- Do NOT use streaming JSONL parsers that block on EOF — read the file to the current EOF and stop; do not tail.
- Since requirements scope to "post-session analysis only" (Out of Scope section), parsing during active sessions is not a primary use case. But files are re-parsed after `HistoryFileWatcher` fires, which may fire during an active session.

### Concurrent Writes
**Risk:** While parsing a JSONL file, Claude Code may append new lines to it. Go's `os.Open` + `bufio.Scanner` reads a snapshot up to the current EOF at time of open; new appended data is not visible to the current scan. This is acceptable — the parse captures a point-in-time view.

**Risk 2:** File modification time check for cache invalidation may race — two goroutines might both detect the same file as "changed" and both kick off a parse. Use a `sync.Map` or per-file mutex to serialize re-parses.

### Encoding Issues
**Risk:** JSONL files contain full conversation content including code, terminal output, and arbitrary user text. Non-UTF-8 bytes could appear in content fields if a user pastes binary data. The `encoding/json` package in Go handles invalid UTF-8 in strings by replacing with the Unicode replacement character — this is safe.

**Risk 2:** Very long lines. Terminal output appended to content fields can be large. `bufio.Scanner` has a default max token size of 64KB. Must set a larger buffer: `scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)` (10MB max line size should be generous).

**Risk 3:** JSON depth. Content blocks in `assistant.message.content` can be deeply nested (thinking blocks with long base64 signatures). The default `encoding/json` decoder handles arbitrary depth.

### Incomplete/Corrupted Files
**Risk:** If Claude Code crashes mid-write, the last line of a JSONL file may be incomplete JSON (no closing brace). `json.Unmarshal` will return an error; that line is skipped. All prior lines are valid and processed normally.

## Performance Risks with Large JSONL Files

### File Sizes
The sample file `604fafb4-6791-42a7-9998-ae15d7f557e9.jsonl` is 1.3MB with 500+ messages in the first 500 lines. A 90-day session corpus could have 100+ files totaling hundreds of MB.

**Parsing throughput estimate:**
- `encoding/json` in Go: ~200MB/s on modern hardware
- 1.3MB file: ~6ms to parse
- 100 files × 1.3MB = 130MB → ~650ms single-threaded

**Mitigation:**
- Parse files concurrently (use a worker pool, e.g., 4 goroutines)
- Cache parsed results keyed by (file path, mod time) — don't re-parse unchanged files
- Store only aggregated token sums per file, not per-message data (unless needed for timeline charts)

### Memory Usage
**Risk:** Storing full parse results for 100+ sessions in memory. If each `ParseResult` stores a `TurnTimeline` with 500 entries, and each entry is ~100 bytes, that's 50KB per session × 100 sessions = 5MB. Acceptable.

**Mitigation:** For the turn-level timeline (burn rate chart), only store timestamps + token counts (not message content). Do not store message text in `ParseResult`.

### NFR Requirements
- NFR-1: 10k+ line JSONL must not block UI → use background goroutine
- NFR-2: Dashboard loads < 2s for 90 days → pre-parse + cache satisfies this
- NFR-3: Token data must not be sent to external services → all computation local; explicitly verify no telemetry captures token counts in analytics events

## How Claude Code Hashes Project Paths

**Finding:** Claude does NOT hash the path. It uses a simple character-replacement encoding.

From `session/history_detector.go`:
```go
func ClaudeProjectDirName(projectPath string) string {
    result := make([]byte, len(projectPath))
    for i := 0; i < len(projectPath); i++ {
        c := projectPath[i]
        if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
            result[i] = c
        } else {
            result[i] = '-'
        }
    }
    return string(result)
}
```

Examples:
- `/Users/alice/myproject` → `-Users-alice-myproject`
- `/Users/alice/.hidden/my_project` → `-Users-alice--hidden-my-project`
- Worktree paths like `/Users/tylerstapler/.stapler-squad/workspaces/a21d754799a5c839/worktrees/foo` → `-Users-tylerstapler--stapler-squad-workspaces-a21d754799a5c839-worktrees-foo`

**Pitfall — Collision Risk:** Two different paths that differ only in non-alphanumeric characters could map to the same directory name. E.g., `/Users/alice/my-project` and `/Users/alice/my.project` both map to `-Users-alice-my-project`. This is a known edge case in Claude Code's design (not our bug), but the parser must handle the case where a project directory contains JSONL files from multiple actual paths by using the session UUID as the primary key.

**Pitfall — Worktree paths:** Stapler-squad sessions running in worktrees have paths like `~/.stapler-squad/workspaces/<workspace-id>/worktrees/<branch>`. These create very long, unique project directory names. The existing `ClaudeProjectDirName` function handles this correctly — already used in production.

## Privacy Considerations

### Sensitive Data in JSONL
JSONL files contain the full conversation: user prompts, file contents read by Claude, command outputs, code changes, API keys accidentally pasted, etc.

**Risks for token monitoring feature:**
1. **Dashboard page may display session titles/paths** — these are already shown in the session list, acceptable.
2. **Skill detection reads user message content** — to detect `/skill-name` patterns, the parser must read user message text. This is local processing only.
3. **Tool name extraction reads content arrays** — to find `tool_use` blocks, parser reads `message.content`. No actual content is stored in `ParseResult`, only tool names and token counts.
4. **No content logging** — `ParseResult` must never store `message.content` values, only metadata (tool names, skill names as short strings).

**NFR-3 compliance:** "Token data must not be sent to any external service without explicit user opt-in."
- The existing OpenTelemetry/Datadog integration in stapler-squad (`.claude/docs/opentelemetry.md`) may capture analytics events. Verify that `InsightsService` does not emit telemetry spans that include session-level token counts or cost data without user consent.
- The `useAnalytics` hook in the frontend tracks page views and actions — ensure no token cost data flows through analytics events.

### Session Path Exposure
Dashboard shows session paths (project directories). Paths may reveal home directory structure, project names, etc. This is existing behavior in the session list and is acceptable — stapler-squad is a local tool.

## Token Count vs Billing Discrepancy

### Known Discrepancies
The requirements note: "Anthropic API billing reconciliation (JSONL counts may differ from billed amounts)" is Out of Scope.

**Why counts may differ:**
1. **Thinking tokens:** Extended thinking (`thinking` content blocks) consume tokens but may be billed differently.
2. **Tool result tokens:** Tool results injected into context are counted as input tokens but may appear differently in billing.
3. **Prompt caching:** The `cache_creation_input_tokens` field represents tokens written to cache (billed at cache-write rate). The `cache_read_input_tokens` represents tokens retrieved (billed at cache-read rate). These match the API response, so our cost calculation should be accurate.
4. **Rounding:** Anthropic rounds token counts; our totals may differ by a small amount.
5. **Batch API vs standard:** Not applicable here.

**Mitigation:**
- Display counts as "estimated" in the UI
- The requirements allow 1% tolerance vs claude-insights output for the same files — this means we should match what the JSONL reports, not attempt to match Anthropic billing.
- Add a disclaimer footnote: "Estimates based on API usage metadata. Actual billed amounts may differ."

## Model Name Normalization

**Risk:** JSONL `model` field contains values like:
- `"claude-sonnet-4-6"` (verified from live files)
- `"claude-opus-4-7"` (expected)
- `"claude-haiku-4-5"` (expected)
- Older: `"claude-3-opus-20240229"`, `"claude-3-sonnet-20240229"` (legacy Claude 3)
- Thinking models: `"claude-sonnet-4-6-20250514-thinking"` (hypothetical extended thinking suffix)

**Pitfall:** Model names may include date suffixes, thinking suffixes, or other identifiers that must be mapped to a pricing entry. The pricing table lookup must use prefix matching or a normalization function, not exact string equality.

**Recommendation:** Normalize with a function:
```go
func NormalizeModelFamily(modelID string) string {
    modelID = strings.ToLower(modelID)
    switch {
    case strings.Contains(modelID, "opus-4"):
        return "claude-opus-4"
    case strings.Contains(modelID, "sonnet-4"):
        return "claude-sonnet-4"
    case strings.Contains(modelID, "haiku-4"):
        return "claude-haiku-4"
    // legacy
    case strings.Contains(modelID, "opus-3"):
        return "claude-opus-3"
    // fallback
    default:
        return modelID
    }
}
```

## Subagent Session Handling

**Risk:** Subagent tasks spawn new Claude Code sessions with their own JSONL files in the same or different project directories. The `isSidechain: true` field on JSONL messages marks subagent branches within a single JSONL file. But subagent `Task` calls may also create entirely separate JSONL files.

**Pitfall:** If subagent sessions are in a different project directory (different working directory), the token monitoring feature may see them as separate orphan sessions and double-count tokens that are logically part of a parent session.

**Mitigation (Phase 1):** Count each JSONL file independently. Show subagent sessions separately in the orphan section. Do not attempt subagent linking in MVP — the requirements flag this as FR-2 but it's complex.

**Mitigation (Phase 2):** Use `parentUuid` chain walking within a single JSONL to group sidechain messages with parent session tokens.
