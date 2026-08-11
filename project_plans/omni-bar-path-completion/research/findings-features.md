# Path Completion UX Patterns — Feature Research

Research date: 2026-04-07

---

## 1. fzf UX Model

### Core Algorithm
fzf uses a **Smith-Waterman-inspired fuzzy matching algorithm** with a scoring model that rewards:
- Consecutive character matches (strong bonus)
- Camel-case / word-boundary matches (medium bonus)
- Start-of-string matches (medium bonus)
- Penalty for gaps between matched characters

The scoring is not simple substring search — it ranks results so that `pro/my-repo` scores higher than `something/myrepo` when the user types `myr`.

### Key UX Affordances
- **Real-time filtering**: List updates on every keystroke with no debounce (feels instant because ranking is O(n) with a compiled C core)
- **Multi-select**: `Tab` marks an item; `Shift-Tab` unmarks. A counter shows selected count. The selection model is additive, not exclusive.
- **Preview pane**: Optional right-side panel showing file contents or command output for the highlighted item. Triggered by `--preview` flag.
- **Color highlighting**: Matched characters highlighted in a distinct color within each result row. Non-matching characters shown at lower contrast.
- **"Best match first"** ordering: The highest-scoring match is always at the top. This differs from shells where exact prefix matches come first.
- **No pagination**: All results scroll in a single virtual list.

### Path-specific Behavior (fzf + fd/find)
When used for directory completion (`ALT+C` in the default keybindings):
- Shows only directories, full absolute paths
- Pressing Enter `cd`s into the selection
- Path depth is visually implicit (no indentation, just the full path string)

### Relevant Takeaways for Omnibar
- Fuzzy scoring that prioritizes consecutive matches is the right model
- Highlight matched characters inline — this is the primary visual feedback that the filter is working
- Multi-select with Tab is not needed for single-path Omnibar

---

## 2. Fish Shell Path Completion

### What Makes It Feel "Native"

#### Ghost Text / Autosuggestion
- As the user types, fish shows a **grey "ghost" completion** extending the current token inline, after the cursor
- The ghost text represents the **single most-likely completion** — it is not a dropdown, it is inline
- Pressing `→` (right arrow) or `End` accepts the full ghost suggestion
- This is a fundamentally different model from a dropdown: one suggestion, zero UI chrome

#### Tab Behavior — Two Modes
1. **Single Tab**: Completes to the longest common prefix of all matching completions. If only one match exists, completes fully.
2. **Double Tab** (or Tab when prefix is fully shared): Opens a scrollable completion list below the prompt.

#### Path-Specific Completion Details
- Completes one **path segment at a time**. Typing `/usr/` and pressing Tab completes the next segment only.
- Fish does **not** fuzzy-match path segments by default — it uses prefix matching per segment.

#### Visual Indicators
- **Blue text** for directories, **normal text** for files — color encodes type without icons
- No explicit "valid path" indicator — the completion list only shows paths that exist

### Relevant Takeaways for Omnibar
- Ghost text (inline suggestion) is extremely powerful for single-match scenarios — consider showing it when there is exactly one match
- Tab-to-complete-longest-common-prefix is the right first-press behavior; show list on second press or when prefix is ambiguous
- Color-coding directories vs files is a low-cost, high-value affordance

---

## 3. VS Code QuickPick / File Picker

### QuickPick Architecture
VS Code's `QuickPick` API is the base for:
- Command Palette (Cmd+Shift+P)
- Go to File (Cmd+P)
- Workspace symbol picker

### Fuzzy Matching Model
- VS Code uses a **CamelCase / word-boundary aware fuzzy filter**
- The filter scores: exact match > prefix match > camelCase boundary match > fuzzy match
- Matched characters are highlighted with **bold or colored spans** within each item label
- Items are sorted by score, then by MRU (most recently used) within the same score tier

### Path Truncation
For long paths in `Go to File`:
- The **filename** is shown at full contrast, left-aligned
- The **directory path** is shown at lower contrast, right-aligned or below the filename
- Long directory prefixes are **ellipsized from the left**: `…/projects/my-app/src/components/Button.tsx`
- This prioritizes the filename (what the user cares about) over the directory (context)

### Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate items |
| `Enter` | Accept highlighted item |
| `Escape` | Dismiss picker |
| `Tab` | Accept partial completion |

Notably, **Tab is not used for cycling** in VS Code's QuickPick — it is used for "confirm partial". The arrows own navigation.

### Relevant Takeaways for Omnibar
- Separate the filename (high contrast, prominent) from the directory path (low contrast, secondary)
- Left-ellipsize long directory paths to keep the end of the path visible
- Arrow keys navigate; Enter confirms. Tab should complete, not cycle.
- MRU ordering within same-score tier improves perceived intelligence

---

## 4. Web Autocomplete Components

### Downshift (headless)
- `useCombobox` hook manages: `inputValue`, `highlightedIndex`, `selectedItem`, `isOpen`
- Keyboard model: `↑`/`↓` navigate, `Enter` selects highlighted item, `Escape` closes, `Tab` closes (browser default behavior unless overridden)
- Does **not** provide fuzzy matching — consumers supply their own filter function
- ARIA roles: `role="combobox"` on input, `role="listbox"` on dropdown, `role="option"` on each item

### React Aria Combobox (`@react-aria/combobox`)
- More opinionated than Downshift; provides built-in fuzzy filter via `useFilter`
- `ListBox` component handles virtualization for large lists
- Keyboard model matches WAI-ARIA Combobox pattern 1.2:
  - `↑`/`↓`: navigate list
  - `Enter`: select
  - `Escape`: close list (first press), clear input (second press)
  - `Tab`: move focus out (closes list, does not select)

### WAI-ARIA Combobox Pattern (authoritative spec)
The ARIA spec distinguishes two autocomplete models:
1. **`aria-autocomplete="list"`**: dropdown shows suggestions; input is not changed until user selects
2. **`aria-autocomplete="both"`**: dropdown shows suggestions AND input shows inline completion (ghost text). The inline portion is selected text so the user can type over it.

The `"both"` model corresponds to fish's ghost text behavior — it is supported in web via `input.value = fullCompletion` with the added portion as a selection range.

### Relevant Takeaways for Omnibar
- Use `useCombobox` from Downshift or React Aria for correct ARIA semantics — do not hand-roll
- `Tab` in web comboboxes typically moves focus out; override this to "accept completion" for path input
- The `aria-autocomplete="both"` pattern is the web-standard way to implement ghost text inline completion
- Virtualize the list if showing >100 results

---

## 5. Tab Completion Semantics

### "Complete to Longest Common Prefix" (shell model)
- On Tab, find the longest string that is a prefix of every current match and complete to that prefix
- Example: Input `proj`, matches only `projects/` → completes fully to `projects/`
- Strengths: Deterministic, no UI required for the common case (single match). Reduces keystrokes.
- Weaknesses: Can feel like nothing happened if the LCP is already the input. Requires a second Tab to see options.

### "Cycle Through Options" (zsh default with `menu-complete`)
- First Tab inserts the first match. Second Tab replaces with the second match.
- Annoying when there are many matches — user must cycle through all of them.

### Recommendation for a Web Omnibar
The web environment favors the **dropdown model** because:
1. No native terminal completion infrastructure exists
2. Users expect a visual dropdown from web search boxes, address bars, etc.
3. The dropdown can be dismissed with Escape, which is a familiar pattern

However, **borrowing the LCP behavior for Tab** is better than cycling:
- Tab should complete to the longest common prefix of all visible matches
- If only one match exists, Tab completes it fully
- Arrow keys navigate the dropdown
- Enter confirms the highlighted item
- Escape dismisses without confirming

---

## Recommended Patterns for This Omnibar

### Matching Algorithm
Use **Smith-Waterman-style fuzzy matching** (fzf model):
- Score consecutive matches highly (rewards typing a contiguous substring)
- Bonus for matching at path segment boundaries (`/` prefix)
- Bonus for matching at start of input
- Penalty for gaps
- Show matched characters highlighted in a distinct color

A lightweight JavaScript implementation: `fzf-for-js` (npm) or `fuse.js` with `includeMatches: true`.

### Keyboard Interaction Model

| Key | Action |
|-----|--------|
| **Any character** | Filter list in real time |
| **`↓`** | Move highlight to first/next item in list |
| **`↑`** | Move highlight to previous item; if at top, return focus to input |
| **`Tab`** | Complete to longest common prefix of visible matches; if one match, complete fully |
| **`Enter`** | Accept highlighted item (or current input if list is empty) |
| **`Escape`** | Close dropdown; if already closed, clear input |
| **`/`** (typed) | Trigger immediate re-completion for the new segment |

### Visual Design

#### Path Display in Dropdown
- **Filename / last segment**: High contrast, left-aligned, larger or bolder
- **Directory prefix**: Muted color (e.g., 60% opacity), right-aligned or below
- Left-ellipsize long prefixes: `…/projects/my-repo/src` → visible end is most relevant
- Matched characters: highlighted with a distinct background or bold + color

#### Inline Ghost Text
When exactly one match exists, show the unconfirmed remainder of the path as ghost text (light grey) inline in the input, after the cursor. This follows fish's autosuggestion model and the `aria-autocomplete="both"` pattern.

#### Path Existence Indicator
- **Green checkmark** (✓): Path exists on the filesystem and is accessible
- **Red X** (✗): Path does not exist
- **Spinner / clock**: Existence check in flight (debounced ~150ms after last keystroke)
- Indicator shown at the right edge of the input field, not in the dropdown

#### Directory vs File Differentiation
- Append `/` to directory entries in the completion list (shell convention)
- Optionally use a folder icon (📁) — but text-only (`/` suffix) is more keyboard-friendly

### UX Flow Summary
1. User opens Omnibar (Cmd+K), input focused
2. User types `/Us` → dropdown opens showing fuzzy-matched paths; ghost text shows `/Users/` if unambiguous
3. User presses Tab → input completes to `/Users/`
4. User types `ty` → dropdown filters to paths under `/Users/` containing `ty`
5. User presses `↓` to navigate into the list, `↑` to return to input
6. Existence indicator updates ~150ms after typing stops
7. User presses Enter → path accepted, Omnibar closes

---

## Sources

- fzf README and source: https://github.com/junegunn/fzf
- Fish shell interactive docs: https://fishshell.com/docs/current/interactive.html
- VS Code QuickPick API: https://code.visualstudio.com/api/references/vscode-api#QuickPick
- WAI-ARIA Combobox pattern: https://www.w3.org/WAI/ARIA/apg/patterns/combobox/
- Downshift useCombobox: https://www.downshift-js.com/use-combobox
- React Aria Combobox: https://react-spectrum.adobe.com/react-aria/ComboBox.html
