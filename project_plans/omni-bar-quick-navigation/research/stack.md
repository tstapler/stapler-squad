# Research: Technology Stack — Omni Bar Quick Navigation

**Date**: 2026-04-21
**Dimension**: Stack

---

## 1. Keyboard Handling Patterns

**Current implementation** (`Omnibar.tsx:362-466`):
- Arrow keys (↑↓) navigate results in both discovery and creation modes
- `Tab` accepts path completion or extends to longest common prefix
- `Enter` selects highlighted result
- `Escape` two-phase: dismiss dropdown → close modal
- `Cmd+Enter` submits creation form
- Global `Cmd+K` / `Ctrl+K` listener in `OmnibarContext.tsx:50-67`

**Pattern used:**
- `onKeyDown` handler directly on the input element
- `useCallback` memoization to prevent circular dependencies
- 150ms debounced input detection (`Omnibar.tsx:283`)
- `useRef`-based `handleSubmit` to avoid declaration-order issues (`Omnibar.tsx:91-93`)

**Handler layering structure:**
```
1. Discovery mode result navigation (arrow keys, Enter, Escape)
2. Dropdown visible navigation (arrow keys, Tab, Enter, Escape)
3. Global handlers (Escape to close, Cmd+Enter to submit)
```

**No custom `useKeyboard` hook exists** — keyboard handling is inline in the Omnibar component.

---

## 2. OmnibarAction / Command Registry

**Current state: No typed action registry exists.**

Session actions are hard-coded in `Omnibar.tsx`:
- `handleSessionSelect()` (line 325) — navigates to session
- `handleRepoSelect()` (line 333) — pre-fills path for creation
- `onCreateNew()` callback (line 674) — switches to creation mode

The `OmnibarSessionData` interface (`Omnibar.tsx:33-49`) is type-safe for session creation data but there is no pluggable command/action pattern. New actions must be manually wired into the component.

**Opportunity:** An `OmnibarAction` registry interface would convert hard-coded handlers into a dispatch table. Each action registers its type, label, icon, executor, and optional inline form component.

---

## 3. CSS / Styling Patterns

**Framework: vanilla-extract (`.css.ts` files)**

All Omnibar styles live in `web-app/src/components/sessions/Omnibar.css.ts` using:
- `style()` for simple classes
- `keyframes()` for animations (fadeIn, slideDown, spin)
- `globalStyle()` for cascading selectors
- Theme tokens from `@/styles/theme-contract.css` via `vars`

**Theme token references in Omnibar:**
- `vars.color.cardBackground`, `vars.color.borderColor`
- `vars.color.textPrimary`, `vars.color.textMuted`, `vars.color.textSecondary`
- `vars.color.primary`, `vars.color.accentHover`, `vars.color.accentBg`
- `vars.color.errorBg`, `vars.color.error`
- `vars.color.hoverBackground`, `vars.color.warningBg`, `vars.color.warning`

**Existing addon panel patterns available:**
- Detection badge (`Omnibar.css.ts:98-125`) — icon + label, positioned within modal
- Form body (`Omnibar.css.ts:127-132`) — flex column layout with gap
- Field groups (`Omnibar.css.ts:134-175`) — label + input pairs
- Collapsible sections (`Omnibar.css.ts:215-249`) — accordion for advanced options
- Footer (`Omnibar.css.ts:251-264`) — flex row action buttons

**New inline creation panel must use `.css.ts` — not `.module.css`** (per `css-architecture.md` ADR).

---

## 4. TUI / BubbleTea Codebase

**Location:** `cmd/` directory
**Size:** 23 Go files, ~3,964 lines

**BubbleTea event handlers found in:**
- `cmd/commands/system.go` — system events (help, quit, escape, tab, confirm, resize)
- `cmd/commands/git.go` — git events (status, stage, unstage, toggle, diff, commit, push, pull)
- `cmd/commands/vc.go` — version control events

**TUI-specific CLI flags:** Require audit before deletion (see pitfalls research).

**Assessment:** The TUI is event-handler code wired to BubbleTea's `Msg` types. The actual BubbleTea framework integration is in `cmd/` and is not deeply intertwined with the Go web server. Deletion should be possible without affecting the web server startup path, but a feature audit is needed first.

---

## 5. State Management

**Redux (RTK) slices:**
- `bulkSelectionReducer`
- `reviewQueueReducer`
- `sessionsReducer`

**Omnibar state:** Entirely local (`useState` in `Omnibar.tsx:56-85`):
```typescript
input, detection, sessionName, program, category, autoYes, showAdvanced,
sessionType, branch, useTitleAsBranch, existingWorktree, workingDir,
isSubmitting, error, dropdownIndex, dropdownDismissed, mode, resultHighlightIndex
```

**Sessions data:** Read from Redux via `useAppSelector(selectAllSessions)` (`Omnibar.tsx:188`).

**Open/close state:** Managed in `OmnibarContext.tsx` (not Redux).

**No `OmnibarSlice` exists.** Adding the action registry and mode state should remain local to the component (or Context) — no Redux slice needed.

---

## 6. ARIA / Accessibility

**Complete listbox pattern already implemented:**

| ARIA attribute | Location | Value |
|---|---|---|
| `role="dialog"` | `Omnibar.tsx:582` | Modal wrapper |
| `aria-modal="true"` | `Omnibar.tsx:583` | Traps focus |
| `aria-label="Session source input"` | Input | Screen reader label |
| `aria-autocomplete="list"` | Input | Listbox suggestions |
| `aria-expanded` | Input | Dynamic: results visible? |
| `aria-controls` | Input | Points to result listbox |
| `aria-activedescendant` | Input | Highlighted item id |
| `role="listbox"` | `OmnibarResultList.tsx:70` | Result container |
| `role="option"` | `OmnibarSessionResult.tsx:73` | Each result item |
| `aria-selected` | Result items | `isHighlighted` prop |
| `role="presentation"` + `aria-hidden` | Section headers | Decorative only |

**Missing from strict WCAG combobox pattern:**
- `Home`/`End` key support (jump to first/last result)
- `Escape` should close listbox before closing modal (currently close-modal on first Escape)

---

## 7. Installed Packages Relevant to This Feature

| Package | Version | Status |
|---|---|---|
| `fuse.js` | `^7.3.0` | ✅ Installed — fuzzy search for sessions and repos |
| `react` | `^19.0.0` | ✅ React 19 |
| `next` | `15.3.2` | ✅ Next.js 15 |
| `@radix-ui/react-dialog` | `^1.1.15` | ✅ Available but NOT used for Omnibar (custom modal) |
| `@vanilla-extract/css` | `^1.20.1` | ✅ CSS framework |
| `@vanilla-extract/recipes` | `^0.5.7` | ✅ Available (not yet used in Omnibar) |
| `react-hook-form` | `^7.63.0` | ✅ Installed but NOT used in Omnibar |
| `zod` | `^4.1.11` | ✅ Available for schema validation |
| `@reduxjs/toolkit` | `^2.11.2` | ✅ State management |
| `cmdk` | — | ❌ Not installed |
| `downshift` | — | ❌ Not installed |

**No command palette library is installed.** The Omnibar is built from first principles — full control over behavior and styling, no external abstractions to work around.

---

## Key Architectural Insights

1. **Two-mode architecture is already in place** (`discovery` / `creation`) — new work extends it, not replaces it.
2. **Keyboard event layering** (result navigation → dropdown navigation → global) must be preserved; new Tab-cycling for session type selection slots into the "dropdown navigation" layer.
3. **Fuse.js is already installed** — no new dependencies needed for fuzzy matching.
4. **No cmdk/downshift/radix Command** — the action registry must be built from scratch, but there are no external libraries to conflict with.
5. **All new CSS goes in `.css.ts` files** — no exceptions per the project's CSS architecture ADR.
6. **The inline creation panel can reuse existing form field styles** from `Omnibar.css.ts` — the patterns (field groups, collapsible, footer) already exist.
