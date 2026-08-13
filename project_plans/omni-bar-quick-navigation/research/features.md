# Research: Feature Landscape — Omni Bar Quick Navigation

**Date**: 2026-04-21
**Dimension**: Features / UX Patterns

---

## Part 1: Codebase Survey

### 1. Current Session Creation Form (`Omnibar.tsx`)

**Form fields:**
- **Session Name** (required): Text input with auto-fill from detected repo name
- **Session Type Selector**: Dropdown with three options:
  - "Create New Worktree" (default)
  - "Use Existing Worktree"
  - "Directory Only (No Worktree)"
- **Branch Management** (conditional):
  - Checkbox: "Use session name as branch name"
  - Branch input field (optional if checkbox enabled)
  - Worktree path selector (dropdown or text input)
- **Working Directory** (optional): Subdirectory specification
- **Advanced Options** (collapsible):
  - Program selector (dropdown)
  - Category field (text input)
  - Auto-yes checkbox

**Current creation workflow:**
- Detection-based mode switching — input type detection triggers creation mode
- Full form appears below the input when in creation mode
- Two-phase submission: form validation → `Cmd+Enter` to create

**Assessment:** The current form is too heavy for the fast-path use case. The "compact inline addon panel" pattern from Raycast is the right model — show only session type + branch, hide everything else behind a disclosure.

---

### 2. Current Session List Actions

**Confirmed redundancy** (from SessionCard/SessionActions components):
- **Fork from Checkpoint**: Modal dialog, forks from a git checkpoint with title input
- **Duplicate**: Session copy (full state)
- **Clone/Fork**: Multiple overlapping "spawn a new session from this one" operations

**Standard actions also present:** Delete, Pause/Resume, Rename, Edit Tags

**Resolution needed:** Consolidate Fork + Duplicate + Clone → single **"Clone session"** action surfaced via the omnibar action registry. The git checkpoint fork is distinct (preserves specific git state) and may remain as a power-user option behind Advanced.

---

### 3. Current Keyboard Shortcuts in Omnibar Footer

**Already implemented (discovery mode):**
- `↑↓`: Navigate results
- `↵`: Jump to session
- `Tab`: Complete path (in creation mode with dropdown)
- `Esc`: Two-phase close (clear highlight → close modal)
- `⌘↵`: Create session (creation mode)

**What's missing:**
- `Cmd+N`: Direct creation mode entry
- `new/` prefix: Force creation mode
- `Tab` to cycle session type options (not just complete paths)
- Mode badge/indicator (Jump | Create) with click-to-toggle
- Arrow key navigation when focused inside the creation form

---

### 4. ARIA / Accessibility Assessment

**Currently implemented correctly:**
- `role="combobox"` on input with `aria-autocomplete="list"`
- `aria-expanded` (dynamically toggled)
- `aria-controls` pointing to result listbox
- `aria-activedescendant` computed via `getHighlightedItemId()`
- `role="listbox"` on result list with `role="option"` on items
- `aria-selected` on highlighted items

**Missing from WCAG combobox pattern:**
- `Home`/`End` key support (jump to first/last result)
- `Escape` should strictly close dropdown first, then modal on second press (currently two-phase but may not match ARIA spec exactly)

---

## Part 2: Industry UX Patterns

### VS Code Command Palette (`Cmd+Shift+P`)

**Pattern: Prefix-based mode switching**
- `>` prefix → commands; bare text → file search; `@` → symbols; `#` → workspace symbols
- Prefix is visible in the search box and changes result set
- Keyboard: ↑↓ navigate, Enter selects, Escape closes

**Lesson for stapler-squad:** `new/` prefix for creation mode is directly borrowed from this pattern. Prefixes must be visually distinctive in the input.

---

### Linear's Command Palette (`Cmd+K`)

**Pattern: Unified command dispatch with action badges**
- Single search for navigation, creation, mutation, and settings
- Action type badges show what will happen (Issue, Filter, Action)
- No explicit "mode toggle" — context determines available actions
- "Create new" appears as a pinned action at top or bottom

**Lesson:** Unified dispatch reduces cognitive load vs explicit mode switching. The action registry pattern is the right architecture. Footer hint text updates dynamically based on highlighted item.

---

### Raycast

**Pattern: Inline forms without modal nesting**
- Form appears inline below search — no modal-on-modal
- Form fields navigable with Tab and arrow keys
- Submit with Enter or Cmd+Enter
- Escape dismisses form, returns to search results

**Lesson:** This is the exact pattern for the creation addon panel. The form should live inside the omnibar overlay, not spawn a separate modal. Tab moves: search input → session type → branch → program → submit.

---

### IntelliJ Shift+Shift (Global Search)

**Pattern: Blended navigation with category tabs**
- Results span: Files, Classes, Symbols, Actions, Settings
- Tab/Shift+Tab cycles through result categories
- Action descriptions and shortcuts shown inline
- No explicit mode switching — category is inherent to result type

**Lesson:** Result category badges (Session | Repo | Action) replace explicit mode indicators. Users scan visually, not by remembering which "mode" they're in.

---

## Synthesized Keyboard Interaction Spec

```
Discovery mode:
  ↑↓           → navigate results (all categories)
  Tab          → accept highlighted repo result → enter creation form
  ↵            → jump to session OR begin creation (if no session selected)
  Esc          → clear highlight (first press) → close modal (second press)

Creation mode (inline addon panel):
  Tab          → cycle: session name → session type → branch → program → submit button
  Shift+Tab    → cycle backward
  ↑↓           → (text fields: normal editing behavior)
  Esc          → return to discovery mode (clear creation form)

Global:
  Cmd+K        → open/close omnibar
  Cmd+N        → open directly in creation mode (bypass discovery)
  new/         → prefix in search forces creation mode
```

---

## Action Registry Architecture Pattern

Based on Linear + VS Code patterns, the correct model is a **unified action registry** with discriminated union enforcement:

```typescript
interface OmnibarAction {
  id: string;
  type: 'navigate' | 'create' | 'mutate' | 'command';
  label: string;
  icon?: string;
  category: 'session' | 'repo' | 'system';
  execute: () => void | Promise<void>;
  isVisible?: () => boolean;
  // For 'create' type: inline form component
  formComponent?: React.ComponentType<OmnibarFormProps>;
}
```

TypeScript discriminated union enforces exhaustive handling — new action types cause compile errors if not handled in the renderer, preventing silent omission.

---

## Key Findings Summary

1. The current creation form is too heavy; the inline addon panel (Raycast pattern) is the right approach.
2. ARIA structure is already correct; keyboard handlers need Tab cycling and Cmd+N additions.
3. Fork/duplicate/clone redundancy is real and should be resolved to one "Clone" action.
4. Mode badge is valuable for clarity but may be replaced by action-type badges on results (Linear pattern).
5. The `new/` prefix is a natural extension of the existing detector pattern.
6. A unified action registry (not a separate "creation mode") reduces long-term complexity.
