# ADR-002: Completion Dropdown Architecture — Extend Omnibar vs Reuse AutocompleteInput

Status: Accepted
Date: 2026-04-08

## Context

The Omnibar main input needs a path completion dropdown. An existing `AutocompleteInput` component (`web-app/src/components/ui/AutocompleteInput.tsx`) provides client-side substring filtering with keyboard navigation. However, the Omnibar input is structurally different: it has a type indicator icon, detection badge, and is the entry point for a multi-step form — not a standalone field.

Two options: (A) replace the Omnibar's `<input>` with `<AutocompleteInput>`, or (B) build a dedicated `PathCompletionDropdown` component that renders as a portal below the Omnibar input.

## Decision

**(B) Build a dedicated `PathCompletionDropdown` component** rendered as a child within the Omnibar's `inputContainer` div, positioned absolutely below the input.

The component receives entries from a new `usePathCompletions` hook and handles its own keyboard navigation. The Omnibar's `handleKeyDown` delegates to the dropdown when it is visible (arrow keys, Tab, Enter, Escape).

## Rationale

1. **AutocompleteInput assumes ownership of the `<input>` element** — it renders its own `<input>` and manages focus, onChange, etc. The Omnibar needs to retain control of its input (for detection, debouncing, type indicators).
2. **Path completions are server-driven** — AutocompleteInput does client-side filtering on a static `suggestions[]` prop. Path completions require a server round-trip on every keystroke (debounced), making the data flow fundamentally different.
3. **Visual requirements differ** — path completions need: directory icon vs file icon, match highlighting, directory-prefix muting, trailing `/` for directories, and truncation indicators. AutocompleteInput renders plain text.
4. **Keyboard model differs** — Tab should insert the longest common prefix (shell-style), not cycle through suggestions. Enter should accept the highlighted item AND navigate into that directory.

## Consequences

- New component: `PathCompletionDropdown.tsx` (~150 lines) + CSS module.
- Omnibar.tsx gains ~30 lines of keyboard delegation logic.
- `AutocompleteInput` remains untouched (used in SessionWizard).
- Future path fields (Working Directory, Existing Worktree) can reuse the hook + dropdown.
