# Architecture Research: Agy and OpenCode TUI Detection Patterns

## Research Scope

This document records findings from source code analysis, testdata inspection, and web
research on what TUI output strings Antigravity CLI (agy v1.0.15) and OpenCode emit
in each detectable state. The goal is to inform pattern additions to `AgyDetector`
and `OpencodeDetector`.

---

## Agy (Antigravity CLI) TUI

### Shared TUI Codebase with Gemini CLI

Agy is a proprietary Google fork of Gemini CLI. The CLAUDE.md comment confirms:
> "Agy uses the same TUI codebase as Gemini CLI, so the patterns are equivalent"

This is further confirmed by matching test fixtures: the `gemini_needs_approval.txt`
fixture shows the exact `Allow execution of:` and `● 1. Allow once` strings that
both tools share.

### Confirmed Patterns (from Gemini fixture files, applicable to agy)

| State | TUI String | File |
|-------|-----------|------|
| Active | `= Running Agent... (ctrl+o to expand)` | `gemini_active.txt` |
| Active | `⌃" Thinking... (esc to cancel, 10s)` | `gemini_active.txt` |
| Idle | `> ▌` (readline cursor on empty line) | `gemini_idle.txt` |
| Idle | `[INSERT]` in status bar | `gemini_idle.txt` |
| NeedsApproval | `Action Required` header | `gemini_needs_approval.txt` |
| NeedsApproval | `Allow execution of: 'xxx'?` | `gemini_needs_approval.txt` |
| NeedsApproval | `● 1. Allow once` | `gemini_needs_approval.txt` |
| NeedsApproval | `2. No, suggest changes (esc)` | `gemini_needs_approval.txt` |

### Agy-Specific Ready/Working Patterns (unverified, inferred from existing detector)

The existing `AgyDetector.Patterns()` includes:
- Ready: `(?:◇|✓).*(?:Ready|ready)` — suggests a "◇ Ready" status indicator
- Processing: `(?:✦|⏲).*(?:Working|working)` — suggests "✦ Working" indicator

These may be status line text that appears beneath the input prompt (separate from the
main conversation area). Neither has a corresponding testdata fixture, so correctness
is unverified. The `✦` spinner glyph appears in the CHANGELOG for related features.

### Agy CHANGELOG Clues (v1.0.13–v1.0.15)

From the agy CHANGELOG:
- **1.0.8**: "Added display of quota usage and execution mode in the status line"
- **1.0.11**: "Added a dynamic exit hint in the status line" + `ctrl+c` interrupts
- **1.0.13**: "removed the 'Resume in the same project' hint line, leaving only the
  standard resume command to simplify exit output"
- **1.0.15**: "Introduced a new interactive status indicator below the input box that
  displays active subagents and background tasks in real-time"

The v1.0.15 subagent status indicator is a new UI element. It may produce detectable
strings for `WaitingForAgent` or `Active` states, but no specifics are available
without a real capture.

### What Is Missing for Agy

| State | Gap | Action Needed |
|-------|-----|---------------|
| Idle | No `agy_idle` pattern; Gemini idle patterns (`> ▌`, `[INSERT]`) would work | Capture real agy idle terminal state |
| Active | No `agy_active` pattern; Gemini active strings likely match | Capture real agy active terminal state |
| Error | Unknown error display strings | Capture agy showing an API/tool error |
| Success | Unknown completion indicator | Capture agy after a task completes |
| NeedsApproval | Existing patterns look correct per shared codebase | Low priority |

**Critical gap**: There are no agy-specific fixture files in `session/detection/testdata/`.
All detection currently relies on Gemini-shared patterns in the global PatternSet. Agy
states will be misidentified unless binary-specific patterns or confirmed shared
patterns are added and tested with real agy captures.

---

## OpenCode TUI

### TUI Architecture

OpenCode TUI is a JavaScript/TypeScript SPA using `@opentui/solid` (SolidJS + OpenTUI
rendering). It is completely separate from Gemini/agy's Ink-based React TUI.

The TUI renders a bordered box layout:
```
╭─ opencode ─────────────────────────────────────────────────────────────────────╮
│                                                                                 │
│  > user's message                                                               │
│                                                                                 │
│  ⠙ Thinking...                                                                 │
│                                                                                 │
╰─────────────────────────────────────────────────────────────────────────────────╯
```

### Confirmed Patterns (from testdata fixtures and source code)

#### Active / Processing (`⠙ Thinking...`, `→ Tool`)

From `opencode_active.txt` and `detector_opencode_test.go`:
- `⠙ Thinking...` — braille spinner + "Thinking..." during LLM generation
- `Thinking: <text>` — with colon, appears inside `┃` prefixed reasoning output
- `┃  Thinking: analyzing the codebase` — bar-prefixed thinking string
- `→ Read <filename>` — tool action arrow for file read
- `→ Write <filename>` — tool action arrow for file write
- `→ Edit <filename>` — tool action arrow for file edit
- `→ Create <filename>` — tool action arrow for file create
- `→ Delete <filename>` — tool action arrow for file delete

The existing `opencode_arrow_action` pattern (`→\s*(Read|Write|Edit|Create|Delete)\b`)
already covers this. The `⠙ Thinking...` spinner is caught by a global Thinking
pattern, not an opencode-specific one.

#### Active / Executing (`esc interrupt`)

From `detector_opencode_test.go`:
- `esc interrupt` — appears when a command is executing
- `(esc to interrupt)` — alternative format
- `esc to interrupt` — plain text format

The global `esc\s+(to\s+)?(interrupt|cancel)` pattern catches this as `StatusExecuting`.

#### NeedsApproval (old sst/opencode format)

From `opencode_needs_approval.txt`:
```
╭─ Permission Required ──────────────────────────────────────────────────────────╮
│                                                                                 │
│  Tool: bash                                                                     │
│  Command: git push origin main                                                  │
│                                                                                 │
│  [ Allow (a) ]  [ Allow for session (s) ]  [ Deny (d) ]                      │
│                                                                                 │
╰────────────────────────────────────────────────────────────────────────────────╯
```
Pattern: `\[\s*Allow\s*\([aA]\)\s*\]` — already in `OpencodeDetector.NeedsApproval`

#### NeedsApproval (new anomalyco/opencode format, per `permission.tsx` source)

The newer fork (`anomalyco/opencode`) renders permission with options:
- `Allow once`, `Allow always`, `Reject`

Pattern in `detector_opencode_test.go` confirms this is captured as `StatusInputRequired`
(not NeedsApproval), via `(?i)Allow\s+once.*Allow\s+always|Allow\s+always.*Allow\s+once`.
The `bar_prefixed_permission` test: `"┃  Allow once   Allow always   Reject"`.

#### InputRequired (bar-prefixed numbered options)

From `detector_opencode_test.go`:
```
┃  4. Icons:
┃      - Do you have existing app icons?
```
Pattern: `┃\s*\d+\.\s+\w` — already in `OpencodeDetector.InputRequired`

The `┃` (U+2503 BOX DRAWINGS HEAVY VERTICAL) is the key discriminator: it appears when
opencode renders a selection prompt or question in the footer area, not in body prose.

### OpenCode Footer Strings (from `footer.tsx` source)

The footer always shows these, regardless of state (not useful for state detection):
- Directory path (left side)
- `• N LSP` — LSP count with green dot when active
- `⊙ N MCP` — MCP count with green (success) or red (error) dot
- `△ N Permission(s)` — warning triangle when there are pending permissions
- `/status` — at the right when connected
- `Get started /connect` — when not connected

The `△ N Permissions` could be used as a NeedsApproval signal, but it's less precise
than the permission dialog box itself.

### What Is Missing for OpenCode

| State | Gap | Notes |
|-------|-----|-------|
| Ready/Idle | No pattern for the empty input prompt state | The TUI shows just the box with `>` cursor; no distinctive idle string identified |
| Error | No pattern | Need to identify what opencode renders for errors (API errors, tool failures) |
| Success | No pattern | After a task completes, opencode returns to idle; no explicit "done" indicator found |
| Active (spinner variants) | Only `→` arrows and `Thinking:` covered | The braille spinner chars `⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋` indicate processing; not in detector |

**For Idle/Ready**: From `footer.tsx`, no distinctive idle-specific string was found.
The TUI appears to show just the bordered input box with no spinner. A broad pattern
matching the box border `╭─ opencode` or the absence of any active indicator would be
needed — but absence-of-pattern detection is not supported by the current PatternSet.

**For Error**: The `footer.tsx` shows `⊙` in error color for MCP failures but not
general agent errors. The `permission.tsx` shows `△ Reject permission` for rejected
permissions. No clear error string was found in the inspected source.

**For Success**: No explicit completion text was identified in the source code. After
a task completes, opencode likely shows the response in the conversation body and
returns to the idle input state. A token/cost summary similar to Claude's `$0.00 •`
pattern was not found in the opencode source.

---

## Summary of Pattern Gaps

### Agy (priority: fill gaps for real agy state differentiation)

| Category | Current | Missing |
|----------|---------|---------|
| Ready | `(?:◇|✓).*(?:Ready|ready)` (unverified) | Fixture file |
| Processing | `(?:✦|⏲).*(?:Working|working)` (unverified) | Fixture file |
| NeedsApproval | `Yes, allow once`, `Allow execution of:` | Confirmed correct |
| Idle | **EMPTY** | `> ▌`, `[INSERT]` (from Gemini shared codebase) |
| Active | **EMPTY** | `= Running Agent...`, `Thinking... (esc to cancel` |
| Error | **EMPTY** | Unknown — need real agy error capture |
| Success | **EMPTY** | Unknown — need real agy completion capture |

### OpenCode (priority: add Idle/Error/Success)

| Category | Current | Missing |
|----------|---------|---------|
| Processing | `→ Read/Write/Edit/Create/Delete` | Braille spinner `[⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋].*` patterns |
| NeedsApproval | `[ Allow (a) ]` (old format) | New format `Allow once.*Allow always` already in InputRequired |
| InputRequired | `┃ N.` bar-prefixed | `Allow once.*Allow always` (new permission format) |
| Ready/Idle | **EMPTY** | No distinctive idle string found |
| Error | **EMPTY** | Unknown — need real opencode error capture |
| Success | **EMPTY** | Unknown — need real opencode completion capture |

---

## Recommendations for Implementation

### R3: Agy detection patterns

1. **Add Idle patterns** (safe, based on confirmed shared Gemini TUI code):
   - `> ▌` (readline cursor) — identical to `gemini_readline_prompt`
   - `[INSERT]` in status bar — identical to Gemini idle state
   - These should be added to `AgyDetector.Patterns().Idle`

2. **Add Active patterns** (likely but unverified):
   - `= Running Agent...` — shared Gemini string
   - `Thinking\.\.\. \(esc to cancel` — shared Gemini string
   - These should be added to `AgyDetector.Patterns().Active`

3. **Error and Success**: Cannot add patterns without real agy captures. Leave empty
   and mark as TODO in the detector.

4. **Create testdata/agy_idle.txt and testdata/agy_active.txt** fixture stubs. Mark
   them as requiring real capture from `tmux capture-pane -p` on a running agy session.

### R5: OpenCode detection patterns

1. **Add spinner Active pattern**:
   - `[⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋].*Thinking` — braille spinner + text
   - Add to `OpencodeDetector.Patterns().Active`

2. **No Idle/Ready pattern found** — the opencode idle state appears to be just an
   empty input prompt with no distinctive string. Detection must rely on absence of
   other patterns (currently results in `StatusUnknown`). This is acceptable behavior.

3. **Error and Success**: Leave empty until real captures are available.

4. **Permission format compatibility**: The existing `NeedsApproval` covers old format;
   new format (`Allow once / Allow always`) is already in `InputRequired`. No change
   needed for either.

---

## Key Source Files Examined

- `session/detection/binaries/agy.go` — current agy detector
- `session/detection/binaries/gemini.go` — current gemini detector (shared patterns)
- `session/detection/binaries/opencode.go` — current opencode detector
- `session/detection/testdata/gemini_active.txt` — confirmed agy-applicable patterns
- `session/detection/testdata/gemini_idle.txt` — confirmed agy-applicable idle patterns
- `session/detection/testdata/opencode_active.txt` — confirmed opencode active state
- `session/detection/testdata/opencode_needs_approval.txt` — confirmed opencode permission
- `session/detection/detector_opencode_test.go` — unit test cases with embedded strings
- `session/detection/snapshot_test.go` — snapshot test coverage summary
- GitHub: `google-gemini/gemini-cli` — Footer.tsx (status bar structure)
- GitHub: `anomalyco/opencode` — permission.tsx + footer.tsx (UI strings)
- GitHub: `google-antigravity/antigravity-cli` — CHANGELOG.md (TUI feature history)
