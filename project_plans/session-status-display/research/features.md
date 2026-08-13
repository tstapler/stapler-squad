# Session Status Display — Detection Patterns Research

## Source of Text Fed Into the Detector

The backend captures terminal content via two mechanisms:

1. **Primary path (ClaudeController sessions):** `Instance.Preview()` reads from an in-memory PTY circular buffer via `GetRecentOutput(n)`. The buffer is kept at 4096 bytes (`StatusDetectionTailBytes` / `IdleDetectorConfig.BufferSize`). Content is then tail-sliced to `statusDetectionTailBytes` bytes and run through `filterTmuxMetadata` to strip tmux status-bar lines before pattern matching.

2. **Fallback path (external/attached sessions):** `Instance.Preview()` falls back to `tmux capture-pane` via `CapturePaneContent()`, which returns the current visible pane content as a string.

3. **Detection entrypoint:** `ClaudeController.GetCurrentStatus()` calls `Instance.Preview()`, takes the last `statusDetectionTailBytes` of the result, FNV-64a hashes it (content-change caching), strips tmux metadata, splits into lines, and calls `StatusDetector.DetectWithContextFromLines(lines)` which scans lines **bottom-up** (most recent line first). This ensures fresh idle/active indicators on the last line override stale scrollback from earlier turns.

File: `session/claude_controller.go` — `GetCurrentStatus()` (line ~510)
File: `session/instance_terminal.go` — `Preview()` (line ~103)

---

## Real Claude Code Output — Verbatim Test Case Inputs

### From `testdata/claude_active.txt` (StatusActive)
```
* Moonwalking… (4m 18s · ↓ 2.0k tokens · thinking)
  └ Tip: Use /btw to ask a quick side question without interrupting Claude's current work

> ▌
esc to interrupt                                                    10% until auto-compact
```

### From `testdata/claude_active_task_manager.txt` (StatusActive)
```
● research-workflow(Business carry-on suiter and garment bag research)
  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)
  ⎿  Tip: Use /btw to ask a quick side question without interrupting Claude's current work

❯
esc to interrupt · ↓ to manage  ● main
↑/↓ to select · Enter to view  ◯ research-workflow (+1)  Business carry-on suiter and garment bag research
```

### From `testdata/claude_asterism_active.txt` (StatusActive)
```
✻ Perambulating... (1h 5m 37s · ↑ 5.4k tokens)
 └ Tip: Use /btw to ask a quick side question without interrupting Claude's current work

> ▌
esc to interrupt 10% until auto-compact
```

### From `testdata/claude_asterism_success.txt` (StatusSuccess)
```
✻ Perambulated for 1h 5m

> ▌
? for shortcuts
```

### From `testdata/claude_baked_idle.txt` (StatusSuccess, per-line scan → StatusIdle on "PR #66")
```
◉ Claude resuming /loop wakeup (May 2 11:55pm)

• CI is in progress — CI, Android Benchmark, and Benchmark are
  all running. Waiting for them to complete.

• Three checks still running (CI, Android Benchmark, Benchmark).
  Checking back in ~4 minutes.

◉ Baked for 10s

> ▌
PR #66
```

### From `testdata/claude_cost_summary.txt` (StatusSuccess via `$0.42 •` pattern)
```
● I've completed the implementation. Here's a summary of what was done:

  1. Added the new `WorkingState` enum to the proto schema
  2. Updated the session adapter to map `IdleState` to `WorkingState`
  3. Propagated the field through the review queue service

  The changes are backward compatible — proto default 0 = UNSPECIFIED, which the
  frontend treats the same as the old "no info" state.

> Are there any other changes you'd like me to make?

⎿  $0.42 • 3 tool uses • 1.2k tokens
```

### From `testdata/claude_idle_ready.txt` (StatusIdle)
```
 Both ran without prompting you — your permission mode is set to auto-approve Bash commands. To see the approval dialog, you'd need to either:

  1. Switch to a more restrictive permission mode — in settings, change from "auto-approve all" to something like "approve bash commands"
  2. Use --dangerouslyDisableSandbox false (default cautious mode) when launching Claude Code

What permission mode are you currently running in? You can check with /permissions or look at your Claude Code settings.

>
? for shortcuts
```

### From `testdata/claude_input_required.txt` (StatusInputRequired)
```
 Do you want to run the test suite before committing these changes?

 ❯ 1. Yes
   2. No
   3. Yes, but only run the fast tests
   4. Type here to tell Claude what to do differently
```

### From `testdata/claude_input_required_with_success_scrollback.txt` (StatusInputRequired)
```
✻ Baked for 5m 30s

Here is my analysis of the changes. I've reviewed all files and everything looks good.
The implementation follows the existing patterns and all tests pass.

 Do you want to proceed?

 ❯ 1. Create PR now (Recommended)
   2. Review the changes first
   3. Cancel and make edits
   4. Type here to tell Claude what to do differently

Enter to select · ↑/↓ to navigate · Esc to cancel
```

### From `testdata/claude_needs_approval.txt` (StatusNeedsApproval)
```
 ⎿  Bash(ls -la /home/tstapler/projects/)

   Allow this command to run?

 ❯ 1. Yes, allow once
   2. Yes, allow for this session
   3. No, and tell Claude what to do differently
```

### From `testdata/claude_thinking_verb.txt` (StatusActive — same as claude_active.txt)
```
* Moonwalking… (4m 18s · ↓ 2.0k tokens · thinking)
  └ Tip: Use /btw to ask a quick side question without interrupting Claude's current work

> ▌
esc to interrupt                                                    10% until auto-compact
```

---

## Spinner / Thinking Line Formats

Claude Code uses a **bounce-cycle spinner** during active processing. The spinner frame changes each render tick. All produce StatusActive.

### Spinner frame characters (the `claude_thinking_verb` pattern)

| Frame | Unicode | Name |
|-------|---------|------|
| `·`   | U+00B7  | Middle dot |
| `✢`   | U+2722  | Four teardrop-spoked asterisk |
| `✳`   | U+2733  | Eight spoked asterisk |
| `✶`   | U+2736  | Six pointed black star |
| `✻`   | U+273B  | Asterism (most common) |
| `✽`   | U+273D  | Heavy teardrop-spoked asterisk |
| `●`   | U+25CF  | Black circle (reduced-motion) |
| `*`   | U+002A  | Asterisk (legacy) |

Pattern (from `detector.go`):
```
(?m)^[ \t]*[·✢✳✶✻✽●*][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})
```

### Active spinner line format
```
✻ Perambulating... (1h 5m 37s · ↑ 5.4k tokens)
```
- Spinner frame + space + **capitalized verb** + `...` or `…`
- Optional parenthetical with timing: `(Xm Ys · ↑/↓ Nk tokens)`
- Optional suffix: `· thinking some more`
- Can be indented (task manager): `  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)`

### The `(↓ 2.8k tokens · 5s)` context line
This is the **timing suffix** on a spinner line. Format:
```
(Xh Ym Zs · ↑/↓ N.Nk tokens)
```
or with extended thinking:
```
(Xm Ys · ↓ N.Nk tokens · thinking)
```
The `↓` indicates downloaded tokens, `↑` indicates uploaded. It appears only while the spinner is active. This is part of the `claude_thinking_verb` Active match — the suffix is parenthetical and doesn't affect detection.

### Completion line format (StatusSuccess)
```
✻ Perambulated for 1h 5m
◉ Baked for 10s
```
Pattern: `[✻◉]\s+\w+\s+for\s+\d+[hms]`
- Past-tense verb (random rotation: Baked, Cooked, Pondered, Perambulated, Synthesized, etc.)
- `◉` (U+25C9 FISHEYE) is completion-only; `✻` can be either active (with `...`) or completion (with `for N`)

### Cost summary line (StatusSuccess)
```
⎿  $0.42 • 3 tool uses • 1.2k tokens
```
Pattern: `\$\d+\.\d+\s+•`

---

## Pattern Table

| Pattern Name | What It Matches | Status Produced | Priority |
|---|---|---|---|
| `claude_thinking_verb` | `^[ \t]*[·✢✳✶✻✽●*][ \t]+[A-Z][verb](?:…|\.\.\.)` — spinner + capitalized verb + ellipsis | StatusActive | 26 |
| `esc_to_interrupt` | `esc\s+(to\s+)?(interrupt\|cancel)` | StatusActive | 25 |
| `synthesizing` | `(?i)Synthesizing\.{0,3}` | StatusActive | 25 |
| `running_status` | `Running\.{3,}` | StatusActive | 24 |
| `progress_indicators` | `[✓✔⠋…★].*(?:ing\|Processing\|Working\|…)` | StatusActive | 23 |
| `verb_duration_completion` | `[✻◉]\s+\w+\s+for\s+\d+[hms]` — past-tense verb completion | StatusSuccess | 21 |
| `cost_summary_line` | `\$\d+\.\d+\s+•` — turn cost line | StatusSuccess | 22 |
| `task_complete` | `(?i)(✓ Successfully completed\|Task completed\|I've completed\|All done)` | StatusSuccess | 20 |
| `file_permission_claude` | `(?i)(Yes, allow reading\|Yes, allow writing\|Yes, allow once\|No, and tell Claude)` | StatusNeedsApproval | 20 |
| `proceed_prompt` | `(?i)Do you want to proceed\?` | StatusNeedsApproval | 19 |
| `numbered_option_selector` | `[❯●]\s*\d+\.\s+\w` — ❯ or ● followed by `N. word` | StatusInputRequired | 16 |
| `opencode_bar_prefixed_options` | `┃\s*\d+\.\s+\w` | StatusInputRequired | 17 |
| `error_message` | `(?im)(^\|[.!?]\s+)(error[\s:]\|fatal error\|exception:\|traceback\|panic:)` | StatusError | 30 |
| `connection_error` | `(?im)^.*(connection refused\|network timeout\|network error)` | StatusError | 29 |
| `claude_readline_prompt` | `(?m)^>\s*▌?\s*$` — readline prompt with optional cursor block | StatusIdle | 16 |
| `claude_shortcuts_prompt` | `\?\s+for shortcuts` — "? for shortcuts" idle footer | StatusIdle | 15 |
| `insert_mode` | `—\s*INSERT\s*—` | StatusIdle | 15 |
| `command_prompt` | `\$\s*$` | StatusIdle | 14 |
| `thinking` | `(?im)^\s*\W{0,3}\s*(thinking\|processing\|analyzing\|working)\b` | StatusProcessing | 10 |
| `tool_use` | `(?im)^\s*(Reading\|Writing\|Editing\|Executing\|Running)\s+[./\w]` | StatusProcessing | 9 |
| `claude_prompt` | `.*` — catch-all | StatusReady | 1 |

**Priority order (highest to lowest):** Error > TestsFailing (disabled) > Success > NeedsApproval > InputRequired > Active > Processing > Idle > Ready

---

## Key Distinctions and Edge Cases

### ❯ vs > — critical difference
- `❯` (U+276F HEAVY RIGHT-POINTING ANGLE QUOTATION MARK) appears in Claude's numbered selection prompts → StatusInputRequired
- `>` (U+003E GREATER-THAN SIGN) appears in shell prompts, Gradle output, markdown blockquotes — NOT a selection indicator

### ✻/◉ disambiguation
- `✻ Perambulating...` (with `...`) → StatusActive
- `✻ Perambulated for 5m` (with `for N`) → StatusSuccess
- `◉ Baked for 10s` → StatusSuccess (◉ is completion-only, never active)

### CR-overwrite pattern (task manager overlay)
Claude Code writes `esc to interrupt · ↓ to manage  ● main\r↑/↓ to select · Enter to view  ◯ research-workflow` on a single PTY line. The `\r` causes the task manager UI to visually overwrite the interrupt hint. Detection uses `DetectFromLines` with reverse CR-segment scanning: the last `\r`-segment is authoritative, but earlier segments showing Active/NeedsApproval/Error are promoted if the last segment is only Ready/Unknown.

### Stale scrollback guard
`DetectWithContextFromLines` scans lines bottom-up. A stale `✻ Baked for 5m` in scrollback does NOT prevent detection of a fresh `❯ 1. Yes` selection dialog below it — the dialog is on later lines and is found first.
