# Findings: Features

## Summary

Three features are under consideration: (1) an inline separator shorthand (`name > prompt`), (2) slash-command prefixes to switch session type inline (`/oneoff`, `/worktree`, etc.), and (3) a keyboard shortcut to cycle through session types from the omnibar input.

All three have strong prior art in widely-used tools. The separator shorthand closely matches Todoist's natural-language inline syntax. The slash-command prefix matches Slack, Notion, and Linear's command-palette patterns. The keyboard cycling already exists in the codebase at the radio-group level (arrow keys in `SessionTypeRadioGroup`) but needs to be exposed at the omnibar input level.

The core UX tension across all three: power-user ergonomics vs. beginner discoverability. Todoist-style inline syntax is invisible until you know about it; tools that succeed with it always pair it with a visible affordance (ghost text, tooltip, or sidebar hint).

---

## Options Surveyed

### Feature 1: Inline separator shorthand

**Option A — `>` separator** (Todoist-style chevron)
Todoist uses a right-pointing chevron to mean "then" or "and do." Maps naturally to "session name, then start with this prompt." `>` is not a shell redirect in this context (omnibar is not a shell). Visual metaphor: `>` suggests "pass to" or "pipe into."

**Option B — `|` pipe separator**
Familiar from shell and functional programming. Readable. But `|` visually competes with path separators and is less natural as "attach a prompt to a name."

**Option C — `:` colon separator**
Used by VS Code quick-open (`file:line`), browsers (`url:query`), and GitHub search (`is:open`). Risk: colons appear in path-like inputs and the existing `PathWithBranchDetector` uses `@` but future patterns could add `:`.

**Option D — ` -- ` double-dash separator**
CLI idiom. Requires two characters plus spaces. Too verbose for an omnibar.

**Option E — `\n` newline (Shift+Enter)**
Alfred and some launchers treat a second line as note/subtext. Cleanest visual separation. Keyboard cost: chord required. Conflicts with "submit on Enter" unless Shift+Enter is explicitly intercepted.

### Feature 2: Slash-command prefix for session type switching

**Option A — `/oneoff`, `/worktree`, `/dir`, `/existing`**
Matches Slack's `/slash-command` convention. Slash at position 0 is unambiguous — no existing detector matches it (`NewSessionDetector` uses `new/` with slash after the word, not before). Slash is muscle memory for Slack/Notion users.

**Option B — `!`-prefixed mode shortcuts (`!oneoff`, `!worktree`)**
Bang-prefix convention from DuckDuckGo bangs and npm. Unambiguous but less learnable.

**Option C — Tab-activated mode picker overlay**
Pressing Tab opens a floating mode selector. Adds UI complexity; is essentially Option D below.

**Option D — Cycle on Tab (no inline syntax)**
Tab cycles through session types without any slash syntax. Simple. Zero parsing. Risks conflicting with Tab autocomplete for paths if that is ever added.

### Feature 3: Keyboard shortcut to cycle through session types

**Option A — Tab (while in omnibar input, creation panel visible)**
Simplest. Mirrors VS Code quick-open. Risk: Tab is the standard "move focus to next field" HTML key; `preventDefault` required.

**Option B — Ctrl+Tab**
High conflict risk; browsers intercept Ctrl+Tab for browser-tab switching before the page sees it.

**Option C — Alt+Arrow (Up/Down)**
Consistent with existing `SessionTypeRadioGroup` arrow-key behavior. Low conflict risk. Less discoverable.

**Option D — Dedicated shortcut with visible UI hint (e.g., `⌘K` or `⌘/`)**
Best discoverability. Higher implementation cost.

---

## Trade-off Matrix

| Option | Discoverability | Parsing complexity | Conflict risk | Power-user ceiling | Prior art strength |
|---|---|---|---|---|---|
| `>` separator | Low (invisible) | Low | None in this context | High | Strong (Todoist) |
| `\|` separator | Low | Low | Low | High | Medium |
| `:` separator | Low | Low | Medium (future path patterns) | High | Strong (VS Code, GitHub) |
| `\n` separator | Medium | None | Low | Low | Medium |
| `/oneoff` prefix | Medium | Low | None | High | Strong (Slack, Notion, Linear) |
| `!oneoff` prefix | Low | Low | None | Medium | Weak |
| Tab cycling | High | None | Medium (form focus) | Medium | Strong (VS Code) |
| Ctrl+Tab cycling | Medium | None | High (browser intercepts) | Medium | Weak |
| Alt+Arrow cycling | Low | None | Low | Medium | Weak |

---

## Risk and Failure Modes

### Separator shorthand

**Ambiguous inputs.** The character `>` does not appear in any current detector pattern (verified by reading `detector.ts`). Splitting on the first `>` will not conflict with GitHub URL detectors, PathWithBranchDetector, or LocalPathDetector. If a user types a session name containing `>` (e.g., a markdown-heading-like label `> Note`), the right half will be treated as the first prompt. This is acceptable: `>` is rare in session names and the split is visually obvious.

**Multi-`>` inputs.** `name > part1 > part2` — must split on only the first `>`. Text after the first separator belongs entirely to the prompt. Implement as `indexOf(">")` split, not a global `split(">")`.

**Empty segments.** `> prompt` (no name) or `name >` (no prompt) must be handled gracefully: treat empty name as "untitled" or skip separator parsing altogether when either side is empty.

### Slash-command prefix

**Conflicts with existing `NewSessionDetector`.** `NewSessionDetector` (priority 35) matches `new/` — a word then slash. A `/oneoff` prefix starts with `/`, not a word. These are orthogonal. No conflict.

**Conflicts with path inputs.** `LocalPathDetector` matches `/absolute/path`. A slash followed by a non-command word (e.g., `/home`) must fall through to `LocalPathDetector`, not be treated as a mode command. The slash-command parser must only match exact registered keywords (`/oneoff`, `/worktree`, `/dir`, `/existing`). Unrecognized `/foo` falls through as-is.

**Discoverability.** Users who never read documentation will never discover slash commands. Mitigation: ghost text or a hint shown when the omnibar is first focused.

### Tab cycling

**Browser focus management.** Capturing `Tab` in the omnibar input prevents advancing focus through creation panel fields below. `preventDefault` is required, and must be conditional: only activate Tab cycling when the creation panel is shown and no results dropdown is open.

**Dropdown interaction.** When the session-search results dropdown is open, Tab may be expected to select the focused result. Tab cycling for session types must be suppressed when the dropdown is visible.

---

## Migration and Adoption Cost

**Separator shorthand.** Zero breaking changes. The feature is purely additive: existing inputs without `>` parse identically. The `initial_prompt` field (#15) already exists in `CreateSessionRequest` proto and the threading pipeline exists up to `OmnibarContext`. The only missing piece is a `firstPrompt` field in `OmnibarFormState` and a textarea in `OmnibarCreationPanel`. Cost: low.

**Slash-command prefix.** Requires adding a new detector (or pre-detector guard) before the `DetectorRegistry`. Existing detectors are unaffected. Maps cleanly onto the existing `SESSION_TYPES` array and the radio group's `onChange` handler. The `NewSessionDetector` (priority 35) is the model to follow. Cost: low-medium.

**Tab cycling.** Requires a `keydown` handler on the omnibar input, gated on creation-panel visibility. Connects to `OmnibarFormState.sessionType` via the same state path that `SessionTypeRadioGroup` already uses. Cost: low.

---

## Operational Concerns

**Ghost text / hints.** Without discoverable affordances, the separator and slash commands will have near-zero adoption from new users. A ghost text suffix in the input (e.g., `my session > first prompt`) disappears on first keypress and never annoys repeat users. This is the standard Todoist approach.

**Session name derivation order.** When a user types `my feature > implement auth`, the slug generator must receive only the pre-separator text (`my feature`), not the full string.

**Slash-command + separator interaction.** What happens with `/oneoff my feature > prompt`? The slash command is consumed first (sets session type to one-off), the remainder `my feature > prompt` is parsed for name + separator. Order: slash-command stripping runs before separator parsing.

**Keyboard shortcut conflict audit.** Tab cycling must be conditional: active only when the creation panel is shown and no autocomplete dropdown is open.

---

## Prior Art and Lessons Learned

### Todoist inline syntax [TRAINING_ONLY - verify details]

Todoist allows a single task input field to embed metadata inline: `@label`, `#project`, `!!1`–`!!4` for priority, and natural language dates.

UX principles that make it work:
1. **Inline parsing is real-time**: as the user types, matched tokens are highlighted or converted to chips. Immediate feedback that the parser understood the input.
2. **Escape hatch**: pressing Escape cancels the inline match.
3. **Ghost text onboarding**: the input placeholder walks users through the syntax.
4. **No mode switching required**: the parser handles all variants simultaneously.

Lesson for Stapler Squad: the `>` separator should produce real-time visual feedback. As soon as `>` is typed, split the input into two visually distinct zones (name | prompt). This mirrors Todoist's chip-based inline feedback.

### Linear command palette [TRAINING_ONLY - verify details]

Linear's `⌘K` palette supports `/` prefix for filtering by resource type (`/issue`, `/project`) and first-token-as-filter. Slash tokens are consumed; the remainder is treated as a query. This is exactly the pattern for `/oneoff` in Stapler Squad.

### Raycast [TRAINING_ONLY - verify details]

Raycast uses Tab to confirm a selected command and advance to its next argument input. The Stapler Squad proposal maps this to Tab confirming a mode selection and cycling to the next type, which is consistent with the mental model.

### Alfred [TRAINING_ONLY - verify details]

Alfred uses `>` as a shell command trigger prefix (at position 0). This is the only case where `>` has a prior meaning in a launcher UI. However, Alfred's `>` is a prefix at position 0; the Stapler Squad `>` is a separator anywhere in the string. No user confusion is expected because Stapler Squad's omnibar is not a generic launcher.

### VS Code Quick Open (`⌘P`) [TRAINING_ONLY - verify details]

VS Code supports inline modifiers in quick open: `:line` suffix to jump to line, `@symbol` to filter by symbol, `#` for workspace-wide symbol search. The `:` separator pattern maps to "name:attribute" rather than "name > content," but the UX model is the same.

### Slack slash commands [TRAINING_ONLY - verify details]

Slack's `/remind`, `/status`, `/away` establish the definitive prior art for `/keyword` as a mode switch in a single-line input. Properties: recognized only at position 0; autocomplete appears immediately when `/` is typed; pressing Space after `/command` confirms and enters argument mode.

---

## Open Questions

1. **Tab conflict with dropdown results.** Does `Omnibar.tsx` use Tab to navigate search results in the dropdown? If yes, Tab cycling must be suppressed when the dropdown is visible.

2. **Ghost text implementation.** Does the current omnibar input use a plain `placeholder` attribute or a layered ghost-text overlay?

3. **Slash-command discovery.** Should typing `/` show an inline autocomplete popup with available commands?

4. **Separator highlight style.** When `>` is detected, how should name vs. prompt zones be visually distinguished?

5. **Empty name + separator.** If the user types `> implement auth` (no name before separator), should the system auto-generate a session name from the first words of the prompt?

---

## Recommendation

**Use `>` as the separator, `/keyword` for session type switching, and Tab (gated) for cycling.**

Rationale:
- `>` has the strongest "pass output to next stage" metaphor for developers. It is unambiguous in the omnibar context. Its visual scan-ability is high: a user sees at a glance where the name ends and the prompt begins.
- `/keyword` is the industry standard for mode switching in single-line inputs (Slack, Notion, Linear, GitHub search). The `NewSessionDetector` already establishes the slash-prefix pattern in this codebase; `/oneoff` is the natural extension.
- Tab cycling is the simplest cycling implementation, consistent with VS Code Quick Open. The conflict risk is manageable: gate Tab cycling on creation-panel visibility and suppress it when any dropdown is open.

**Implementation priority order:**
1. Auto-populate session name from bare omnibar text (unblocks the separator feature)
2. `>` separator shorthand (highest UX value, lowest complexity)
3. Tab cycling for session types (no parser needed, pure event handler)
4. `/keyword` prefix commands (most complex; implement last)

**Discoverability must be addressed.** Recommend a ghost-text placeholder in the omnibar input showing the separator syntax.

---

## Pending Web Searches

1. `todoist inline syntax natural language UX principles 2024` — verify chip-based real-time feedback behavior
2. `raycast launcher keyboard shortcut tab cycling session type 2024` — confirm whether Raycast uses Tab for argument cycling
3. `linear command palette slash command filter UX 2024` — verify `/` prefix behavior in Linear
4. `alfredapp greater-than prefix shell trigger 2024` — confirm Alfred's `>` shell-trigger convention
5. `command palette inline syntax separator prior art UX 2025` — broader survey for newer launchers
