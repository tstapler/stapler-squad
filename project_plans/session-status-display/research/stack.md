# Stack Research: RE2 Unicode Constraints for Session Status Patterns

## 1. Exact Current `claude_thinking_verb` Pattern

File: `session/detection/detector.go`, inside `getDefaultPatterns()`, `Active` slice.

```
(?m)^[ \t]*[·✢✳✶✻✽●*][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})
```

The accompanying comment (verbatim):

```
// Full macOS spinner frame set: · ✢ ✳ ✶ ✻ ✽ (bounce cycle), * (legacy), ● (reduced-motion).
// Verb char class extends \w with hyphens (Dilly-dallying), apostrophes (Beboppin'),
// and Latin-1 accented chars (Flambéing, Sautéing) — Go RE2 \w = [0-9A-Za-z_] only.
// ^\s* allows leading whitespace so indented spinners (e.g. task manager sub-items)
// are detected: "  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)"
```

Note: The pattern uses `[ \t]*` for leading whitespace (not `\s*`) and `[ \t]+` as the separator after the spinner frame character. The frame character class `[·✢✳✶✻✽●*]` contains **seven literal UTF-8 characters** embedded directly in the Go source string.

---

## 2. Go `regexp` Package — RE2 Unicode Support

Go's `regexp` package implements RE2 syntax (via `golang.org/x/exp/utf8string` internally; the engine is the same RE2 automaton used in re2c/Google RE2). Key facts:

### What works

- **Literal UTF-8 characters in character classes** — Go source files are UTF-8. A character class `[✦⏺]` in a Go raw or interpreted string literal embeds the UTF-8 byte sequence directly. RE2 handles UTF-8 natively and matches codepoints, not raw bytes, so `[✦⏺]` correctly matches a single rune.
- **`\x{NNNN}` hex codepoint escapes** — RE2 supports `\x{2726}` (for ✦ U+2726) and `\x{23FA}` (for ⏺ U+23FA) inside character classes. These are the canonical RE2 way to reference codepoints by number.
- **Unicode character properties via `\p{}`** — RE2 supports POSIX/Unicode category names (`\p{L}`, `\p{Lu}`, etc.) but NOT arbitrary Unicode block names.

### What does NOT work (RE2 caveats)

- **`\uXXXX` escape sequences** — RE2 does **not** support Java/JavaScript-style `\uXXXX` 4-hex-digit escapes. Using `✦` in a Go regexp will be interpreted as the literal text `u2726`, not U+2726.
- **`\U` (8-digit)** — likewise not supported.
- **Lookahead / lookbehind** — not supported (unrelated but worth noting for pattern authors).
- **Backreferences** — not supported.
- **`\w` for non-ASCII** — Go RE2's `\w` is exactly `[0-9A-Za-z_]`. It does NOT match accented Latin-1 chars (é, è, ê, etc.), which is why the current `claude_thinking_verb` pattern spells them out explicitly.

### Conclusion for ✦ (U+2726) and ⏺ (U+23FA)

Both characters can be embedded **directly as literal UTF-8** in the character class:

```
[·✢✳✶✻✽●*✦⏺]
```

This is identical to what the current pattern already does for `·✢✳✶✻✽●*`. Alternatively, the RE2-safe codepoint escape form is:

```
[\x{B7}\x{2726}\x{23FA}\x{2762}\x{2733}\x{2736}\x{273B}\x{273D}\x{25CF}*]
```

The direct literal form is preferred (matches existing code style, more readable).

---

## 3. RE2-Specific Caveats Summary

| Syntax | RE2 Support | Notes |
|---|---|---|
| Literal UTF-8 in `[...]` | YES | Preferred; matches codepoints |
| `\x{NNNN}` hex escape | YES | RE2-standard alternative |
| `\uXXXX` (4-digit) | NO | Interpreted as literal `uXXXX` |
| `\UNNNNNNNN` (8-digit) | NO | Same as above |
| `\p{L}`, `\p{Lu}` etc. | YES | Unicode category properties |
| `\w` matching accented chars | NO | `\w` = `[0-9A-Za-z_]` only |

---

## 4. Proposed New Pattern

The two Gemini CLI characters to add are:

- **✦ U+2726 BLACK FOUR POINTED STAR** — appears in `gemini_working` pattern (`(?:✦|⏲).*(?:Working|working)`) and in the test `"✦ Working..."`. This is Gemini's primary thinking spinner.
- **⏺ U+23FA BLACK CIRCLE FOR RECORD** — a recording/record icon; Gemini uses this in some status lines.

Current `gemini_working` pattern (in `Processing`):
```
(?:✦|⏲).*(?:Working|working)
```

This already handles `✦ Working...` in the `Processing` category. The question is whether `✦` should also be recognized in the `Active` `claude_thinking_verb` pattern when followed by a capitalized verb + ellipsis (i.e., when Gemini uses the same spinner-verb format as Claude).

**Proposed updated `claude_thinking_verb` pattern:**

```
(?m)^[ \t]*[·✢✳✶✻✽●*✦⏺][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3})
```

Change: add `✦` (U+2726) and `⏺` (U+23FA) to the spinner frame character class `[·✢✳✶✻✽●*]`, making it `[·✢✳✶✻✽●*✦⏺]`.

Updated comment to accompany the pattern:

```
// Full macOS/Gemini spinner frame set: · ✢ ✳ ✶ ✻ ✽ (macOS bounce cycle), * (legacy),
// ● (reduced-motion), ✦ (Gemini/agy primary spinner U+2726), ⏺ (Gemini record U+23FA).
// Verb char class extends \w with hyphens (Dilly-dallying), apostrophes (Beboppin'),
// and Latin-1 accented chars (Flambéing, Sautéing) — Go RE2 \w = [0-9A-Za-z_] only.
// Direct UTF-8 embedding in [...] is valid RE2; \uXXXX escapes are NOT supported by RE2.
// ^\s* allows leading whitespace so indented spinners (e.g. task manager sub-items)
// are detected: "  ✽ Roosting… (9m 52s · ↓ 2.8k tokens)"
```

---

## 5. All Pattern Names in `getDefaultPatterns()`

### `Ready`
- `claude_prompt`
- `gemini_ready`

### `Processing`
- `thinking`
- `tool_use`
- `opencode_arrow_action`
- `gemini_working`

### `NeedsApproval`
- `file_permission_claude`
- `proceed_prompt`
- `aider_permission`
- `gemini_permission`
- `gemini_allow_execution`
- `opencode_permission`

### `Error`
- `error_message`
- `connection_error`

### `TestsFailing`
- _(empty — disabled due to false positives)_

### `Idle`
- `insert_mode`
- `claude_readline_prompt`
- `command_prompt`
- `vim_normal_mode`
- `bracket_insert_mode`
- `claude_shortcuts_prompt`

### `Active`
- `esc_to_interrupt`
- `synthesizing`
- `claude_thinking_verb`  ← **target of this feature**
- `running_status`
- `progress_indicators`
- `tool_execution_active`

### `Success`
- `cost_summary_line`
- `verb_duration_completion`
- `task_complete`
- `success_checkmark`
- `finished_successfully`
- `tests_passed`
- `build_success`

### `InputRequired`
- `numbered_option_selector`
- `opencode_bar_prefixed_options`
- `opencode_permission_buttons`

---

## 6. Related Patterns That May Also Need Updating

The `gemini_working` pattern in `Processing` already covers `✦ Working...`:

```
(?:✦|⏲).*(?:Working|working)
```

If Gemini uses `✦ <Verb>…` with non-"Working" verbs in active state, `claude_thinking_verb` with `✦` added would catch those. No other patterns currently reference `✦` or `⏺` except `gemini_working`.

The `progress_indicators` pattern:
```
[✓✔⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏★].*(?:ing|Processing|Working|Executing|Verifying|Testing|Building|Synthesizing)
```
This does not include `✦` or `⏺`. If those are added here too, it would create a broader catch with verb-suffix matching rather than the capitalized-verb + ellipsis match in `claude_thinking_verb`.

## 7. Imports Confirming RE2 Engine

`session/detection/detector.go` imports:
```go
import (
    "fmt"
    "os"
    "regexp"
    "strings"
    "gopkg.in/yaml.v3"
)
```

Only `"regexp"` — the standard library RE2-based engine. No third-party regex library. All patterns compile via `regexp.Compile()`.
