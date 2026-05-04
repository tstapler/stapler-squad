## Existing Patterns

All patterns live in `session/detection/detector.go` in `getDefaultPatterns()`, with ANSI stripping applied before matching (`stripANSI()` via `\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07`).

### Active patterns (session IS working — remove from queue)
| Pattern name | Regex | Trigger |
|---|---|---|
| `esc_to_interrupt` | `esc\s+(to\s+)?(interrupt\|cancel)` | Claude showing "esc to interrupt" or "esc to cancel" |
| `synthesizing` | `(?i)Synthesizing\.{0,3}` | Claude synthesizing response |
| `running_status` | `Running\.{3,}` | Running... text |
| `progress_indicators` | `[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏★].*(?:ing\|Processing\|Working\|...)` | Spinner chars + action verb |
| `tool_execution_active` | `(?i)(Executing\|Verifying\|Testing\|Building\|Deploying).*\(esc` | Tool with interrupt option |

### Processing patterns (session IS working)
| Pattern name | Regex |
|---|---|
| `thinking` | `(?im)^\s*\W{0,3}\s*(thinking\|processing\|analyzing\|working)\b` |
| `tool_use` | `(?im)^\s*(Reading\|Writing\|Editing\|Executing\|Running)\s+[./\w]` |
| `opencode_arrow_action` | `→\s*(Read\|Write\|Edit\|Create\|Delete)\b` |
| `gemini_working` | `(?:✦\|⏲).*(?:Working\|working)` |

### Idle patterns (session waiting for input)
| Pattern name | Regex | Meaning |
|---|---|---|
| `insert_mode` | `—\s*INSERT\s*—` | Claude Code INSERT mode (idle prompt) |
| `command_prompt` | `\$\s*$` | Shell prompt at end of output |
| `vim_normal_mode` | `—\s*NORMAL\s*—` | Vim NORMAL mode |
| `claude_shortcuts_prompt` | `\?\s+for shortcuts` | Claude Code idle, shows "? for shortcuts" |
| `bracket_insert_mode` | `\[INSERT\]` | Gemini/editor INSERT bracket format |

### Key "idle" signal: `> ` Claude Code prompt
The `> ` input prompt is NOT explicitly listed as a pattern. The idle prompt is captured by `claude_shortcuts_prompt` (`\?\s+for shortcuts`) and by `insert_mode`. There is no direct `> \s*$` pattern. This is a gap.

### Key "idle" signal: Cost summary line
The `\$\d+\.\d+ •` cost summary pattern is NOT in the codebase at all. The task completion (`StatusSuccess`) patterns detect visual markers like `✻ \w+ for \d+[hms]` (verb + duration) and `✓ Successfully completed`, but NOT the cost line. This is a gap.

## Claude Code Output Taxonomy

Based on the patterns and snapshot tests (`session/detection/testdata/`), here is the full taxonomy of observable Claude Code states:

### Definitively "working" (Active/Processing)
- `esc to interrupt` or `esc to cancel` — the canonical signal Claude is generating
- Spinner characters (`⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏`) with action verb
- `Thinking...` / `Processing...` / `Analyzing...` at start of line
- Tool calls: `Reading /path`, `Writing /path`, `Executing command`
- `Synthesizing...`

### Definitively "idle" / ready for next task
- `— INSERT —` or `? for shortcuts` — Claude Code input prompt
- `✻ <verb> for <duration>` (e.g. `✻ Spent for 45s`) — task complete + cost stats
- `$ ` at end of output — shell prompt returned

### Definitively "needs attention"
- `Yes, allow reading` / `No, and tell Claude` — approval dialog
- `❯ 1. Yes` / `● 1. Yes` numbered selector — AskUserQuestion prompt
- Error patterns at line start

### Ambiguous / not yet covered
- `\$\d+\.\d+ •` cost line — final summary line indicating task complete, not currently detected as a specific pattern; would indicate "idle after task"
- Silence after last output — recency heuristic only, no explicit pattern
- "Ebbing..." — not found in any patterns (mentioned in requirements but absent from codebase)

## Missing Patterns

1. **Claude Code `> ` prompt**: The actual input prompt character `> ` followed by cursor is not explicitly matched. The `claude_shortcuts_prompt` (`? for shortcuts`) pattern covers the status bar but the `>` readline prompt itself has no dedicated match.

2. **Cost summary line** `\$\d+\.\d+ •`: This specific line (e.g. `$0.42 • 3 tool uses`) is the clearest indicator that Claude has finished a full turn and returned to idle. Not currently detected. Should map to `StatusSuccess` or `StatusReady`.

3. **"Ebbing..."**: Mentioned in the requirements as a thinking indicator. Not found anywhere in the codebase's patterns or comments. Likely a newer Claude Code UI state.

4. **Turn completion silence**: The existing `StalenessThreshold = 2 minutes` is too coarse; a session that just finished in 1 minute appears stale/idle to the queue. A recency-based "just completed" signal (last output < 30s + idle prompt detected) is not modeled as a specific state.

5. **Multiline spinner sequences**: Claude sometimes emits sequences like `⠋ Thinking\r⠙ Thinking\r...` using carriage returns. These only work correctly if ANSI is properly stripped AND carriage-return-overwriting is handled. Currently the detector strips ANSI but does NOT handle `\r` overwriting — the stripped text may contain repeated spinner lines that could confuse pattern matching.

## Recommendations

1. **Add cost summary pattern** to `StatusSuccess` (highest confidence "task done" signal):
   ```yaml
   success:
     - name: cost_summary_line
       pattern: '\$\d+\.\d+\s+•'
       description: "Claude cost summary — task completed"
       priority: 22
   ```

2. **Add explicit `> ` prompt pattern** to `StatusIdle`:
   ```yaml
   idle:
     - name: claude_readline_prompt
       pattern: '^>\s*$'
       description: "Claude Code readline input prompt"
       priority: 16
   ```

3. **Add "Ebbing..." to Processing patterns** (verify with real Claude output capture first):
   ```yaml
   processing:
     - name: ebbing
       pattern: '(?i)Ebbing\.{0,3}'
       description: "Claude ebbing (internal state)"
       priority: 10
   ```

4. **Handle carriage return overwriting**: Before ANSI stripping, replace `\r` (without `\n`) with `\n` or strip to the last non-overwritten version of each line.

5. **Add golden-state snapshot tests** for:
   - `claude_idle_with_cost_summary.txt` → `StatusSuccess` 
   - `claude_after_turn_completion.txt` → `StatusIdle`
   - The `claude_idle_ready.txt` fixture already exists but should also cover the `? for shortcuts` prompt specifically
