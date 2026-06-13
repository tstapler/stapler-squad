# Pitfalls: Adding ✦ and ⏺ to Active Detection Patterns

## 1. Where ✦ and ⏺ Appear Outside Detection Test Files

### ✦ (U+2726 BLACK FOUR POINTED STAR)

**Go files** (non-test):
- `session/detection/detector.go` line 435: already used in the `gemini_working` Processing pattern — `(?:✦|⏲).*(?:Working|working)`

**TSX files** (frontend UI — irrelevant to PTY detection):
- `web-app/src/components/history/HistoryEntryCard.tsx` line 95: renders `✦` as a dirty-worktree indicator
- `web-app/src/components/shared/VcsStatusDisplay.tsx` line 44: renders `"✦ Uncommitted changes"` text
- `web-app/src/components/sessions/SessionRow.tsx` line 86: returns `"✦"` as the program icon for Claude sessions
- `web-app/src/components/sessions/ReviewQueuePanel.tsx` line 696: renders `"✦ Create Rule"` button text

**Key finding**: ✦ is already captured by the existing `gemini_working` Processing pattern. The frontend uses it only in rendered UI components, not in PTY output — so it cannot cause false positives in terminal detection.

### ⏺ (U+23FA BLACK CIRCLE FOR RECORD)

**Go files** (non-test):
- Not present in any production `.go` file. Only appears in `session/review_queue_determiner_test.go` line 171 in the test string `"esc to interrupt\n⏺ Recording"`.

**TSX files**:
- `web-app/src/components/sessions/TerminalOutput.tsx` line 1500: renders `'⏺️ Record'` (note: this is the emoji variant U+23FA + U+FE0F, not the bare symbol) as a button label in the UI toolbar.

**Key finding**: ⏺ (bare U+23FA) does not appear in any production Go code today. In Claude Code's actual PTY output it is used as a tool-action bullet:
- In-progress: `⏺ Bash(go test ./...)` (appears while the tool call is executing)
- Completed: `✓ Bash(go test ./...)` (the `⏺` line is replaced by `✓` when done)

---

## 2. False-Positive Risk: ⏺ on Completed Tool Lines

Claude Code's TUI renders tool actions in two phases:

1. **Dispatching** (tool call starts): `⏺ Bash(go test ./...)` — the ⏺ bullet appears.
2. **Completed** (tool call finishes): The ⏺ line is **replaced in the scrollback** with `✓ Bash(go test ./...)`.

**However**, the detection window is the last 4096 bytes (`StatusDetectionTailBytes = 4096`). This means:

- If a tool call completes quickly and the `✓` line appears before the next poll, only `✓` is visible — no false positive.
- If the session is **idle** but the last 4096 bytes of scrollback contains old `⏺ ...` lines that were never overwritten (e.g., tool calls from a previous interaction that are in the tail window), adding a bare `⏺` pattern would fire `StatusActive` even when the session is waiting for input.

This is the primary false-positive risk: **stale ⏺ lines in the tail window from previous turns**.

Claude Code does not always replace ⏺ lines in-place via `\r`. When tool output is multi-line or when the UI scrolls, old `⏺ Verb(...)` lines remain in the scrollback buffer. The detection window of 4096 bytes may encompass several previous turns.

---

## 3. How the Detector Handles ✓ <verb> Completion Lines

The existing patterns interact with `✓` as follows:

**Success patterns** (checked after Error and TestsFailing, before NeedsApproval):
- `task_complete`: `(?i)(✓ Successfully completed|...)`
- `success_checkmark`: `(?i)✓.*(?:complete|done|success|finished)`

**Active patterns** (checked after NeedsApproval and InputRequired):
- `progress_indicators`: `[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏★].*(?:ing|Processing|Working|Executing|...)`

**The `✓ Bash(go test ./...)` line** (a completed tool call with a command name):
- Does NOT match `success_checkmark` (no "complete/done/success/finished" word)
- Does NOT match `task_complete` (no "Successfully completed" etc.)
- Does NOT match `progress_indicators` (no action gerund — "Bash" is not a gerund)
- The `✓` completed-tool line is effectively **invisible** to the current detection patterns

**Conclusion**: There is no existing "done" pattern that would override an Active detection triggered by `⏺`. The completed-tool `✓ Bash(...)` line produces `StatusUnknown` (no match) or `StatusReady` (catch-all `.*`). Under `DetectFromLines` semantics, `StatusReady` is a low-priority fallback — so if any `⏺` line also matches in the same window, Active would win. This makes unanchored `⏺` detection especially risky.

---

## 4. Detection Model: Per-Line vs Multi-Line Window

**`Detect()` / `DetectRecent()`** — operates on the entire tail as a single string block.
- All regex patterns run against the concatenated text.
- A `⏺` anywhere in the last 4096 bytes would match.
- This is the primary code path used in `review_queue_poller.go` (line 523).

**`DetectFromLines()`** — line-by-line, reverse order (most-recent-first), stops at first specific match.
- Iterates lines from bottom of scrollback up.
- Returns immediately on the first non-Ready, non-Unknown match.
- If the most recent line is `? for shortcuts` (Idle), and an earlier line has `⏺ Bash(...)`, the Idle line would win (stops at first specific match going upward from bottom).
- This is used in contexts where line-by-line ordering matters (e.g., inside CR-segment scanning).

**Critical implication for `Detect()` / `DetectRecent()`**: Both scan the entire tail window as a flat string. An old `⏺` from three turns ago that happens to be within the 4096-byte tail would cause a false Active detection. `DetectFromLines()` is safer but is not the primary code path for the "is session active?" check in the review queue poller.

---

## 5. Priority and Ordering Between Patterns

Detection priority order (highest to lowest), from the `Detect()` method:

1. Error
2. TestsFailing
3. Success
4. NeedsApproval
5. InputRequired
6. **Active** ← ⏺ would land here
7. Processing ← ✦ is currently placed here (gemini_working)
8. Idle
9. Ready (catch-all)

**For ✦**: Already exists as a Processing pattern. Moving it to Active would change its priority upward (checked before Processing). As a Processing pattern it is currently below Active, so `esc to interrupt` (Active) already overrides ✦.

**For ⏺ as Active**: Active is checked after Success, NeedsApproval, and InputRequired, but before Processing, Idle, and Ready. A false-positive ⏺ detection from stale scrollback would prevent the real session state (e.g., Idle = waiting for input) from being reported.

---

## 6. Recommended Pattern Specificity

### For ✦ (U+2726)

**Current situation**: `(?:✦|⏲).*(?:Working|working)` in Processing — this is already specific enough. ✦ alone followed by "Working" is unambiguous.

**Risk if adding bare ✦ to Active**: ✦ appears in the frontend's rendered HTML as a VCS dirty indicator and Claude session icon. These strings ("✦ Uncommitted changes", "✦") could theoretically appear in terminal output if a user pastes them. Low risk, but non-zero.

**Recommendation**: Do not add bare `✦` to Active. Keep it in Processing under `gemini_working` with the `Working` anchor. If ✦ as an Active indicator is needed, anchor it: `✦.*\(esc` or `✦.*Working` with explicit context.

### For ⏺ (U+23FA)

**Do NOT add bare `⏺` as an Active pattern.** The risks are:

1. **Stale scrollback**: Any `⏺ Verb(...)` from a previous turn within the 4096-byte tail fires Active even when the session is idle and waiting for input.
2. **No completion suppression**: The `✓ Verb(...)` completion line is invisible to existing patterns — it does not produce a Success or any other status that would override the false Active.
3. **`Detect()` is the hot path**: The review queue poller calls `DetectRecent()` which calls `Detect()` on the tail block — not `DetectFromLines()`. A single ⏺ character anywhere in the tail is enough to fire.

**Recommended patterns if ⏺ detection is desired**:

Option A — require `(esc to interrupt)` on the same line:
```
⏺[^✓\n]*\(esc\s+(to\s+)?(interrupt|cancel)\)
```
This fires only when ⏺ appears alongside the interrupt hint, which Claude Code renders only during an in-progress tool call.

Option B — require both ⏺ and `esc to interrupt` anywhere in the tail (looser, but acceptable since `esc to interrupt` is already an Active pattern):
This is already handled by the existing `esc_to_interrupt` pattern — ⏺ adds no additional signal here.

Option C — anchor to the line ending with an open parenthesis (tool in progress), not yet closed with ✓:
```
^⏺\s+\w+\([^)]*$
```
This is fragile and depends on the exact rendering format.

**Practical recommendation**: Do not add ⏺ to Active patterns at all. The existing `esc_to_interrupt` pattern already covers the definitive "Claude is actively executing" signal. ⏺ without `(esc to interrupt)` means the tool call has already completed or was never running; ⏺ with `(esc to interrupt)` is already caught by `esc_to_interrupt`. Adding ⏺ independently provides no true benefit and introduces the stale-scrollback false-positive risk.
