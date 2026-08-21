# Research: UX Patterns for Alias / Command Palette Features

**Date**: 2026-06-20

## UX Review Findings

A full UX review of the alias pattern design was conducted on 2026-06-20. Findings:

### Critical (must fix before shipping)

**ALIAS-001**: Unrecognized `@alias-name` must not fall through to session search. Users who type a typo get session search results for `@typo`, which is confusing. The AliasDetector must claim all `^@[\w-]` input and return either `AliasResolved` or `AliasNotFound`.

**ALIAS-002**: Show an inline chip/badge as soon as a valid alias prefix resolves. Without this, users have no confirmation that `@myp` matched alias `myproj`. The badge should appear as the user types, not only after pressing Enter.

### High Severity

**ALIAS-003**: `@` is already the branch separator in `path@branch` syntax. Two meanings for one character risks user confusion. Mitigate with clear placeholder text and documentation.

**ALIAS-004**: `:` is overloaded — `new:<path>` (prefix), `repo:branch` (suffix), `@alias:branch` (suffix). Define the rule clearly in documentation and hints.

**ALIAS-005**: Omnibar placeholder must include `@alias` as an example input type. Currently the placeholder doesn't mention aliases, making the feature invisible to new users.

**ALIAS-006**: Empty state when no aliases are configured needs copy and a call-to-action. Typing `@` alone with zero aliases should show guidance, not an empty dropdown.

### Medium Severity

**ALIAS-007**: No in-app alias creation — JSON-only workflow. This is a v1 constraint; plan "Save session as alias" for v2.

**ALIAS-008**: "General" as the default group name for ungrouped aliases may read as a user-defined group. Consider rendering ungrouped aliases above all groups with no header.

**ALIAS-009**: Case sensitivity must be specified in the UI. Normalize to lowercase on config write; match case-insensitively.

**ALIAS-010**: Label extent must be specified in the grammar. Recommendation: everything after the alias name/branch and before the first `--` token is the label (may contain spaces).

### Low Severity

**ALIAS-011**: `@` requires Shift key on US keyboards — minor friction vs `/` or `~`.
**ALIAS-012**: Alias ordering within groups is unspecified; use config-file order for v1.
**ALIAS-013**: Config parse errors should surface in the alias palette empty state.

## Command Palette UX Patterns (from VS Code, Warp, GitHub)

### Two-phase display (browse vs. search)

The most validated pattern (VS Code, Warp, Raycast) is:
- **Idle / browse** (`@` alone): grouped sections with headers, full descriptions visible
- **Active filtering** (`@wo`): flat list, groups hidden, fuzzy matching across all aliases

This serves two user modes:
- **Recall mode**: user knows the alias name and types it directly → flat fuzzy is fastest
- **Discovery mode**: user browsing aliases they set up weeks ago → grouped sections help

### Alias resolution chip

When the user has typed enough to unambiguously match an alias, show a chip indicating the resolved alias name and its description. This pattern is used by GitHub's `@mention` suggestions and Slack's `/command` hints.

Example: typing `@myp` shows chip `@myproj — ~/code/myproj [claude]` before the user finishes typing.

### Empty and error states

- **No aliases configured**: "No aliases yet — add them in config.json"
- **Alias not found**: "No alias '@foo' — did you mean '@foobar'?" (fuzzy suggest nearest match)
- **Config parse error**: "Alias config failed to load — check config.json for syntax errors"

## UX Acceptance Criteria

1. User can invoke a saved alias in ≤ 2 keystrokes after `@` (for aliases with ≤ 3-char names)
2. Typing `@` alone opens the alias palette within 100ms
3. Unrecognized alias shows an error state, not session search results
4. Grouped sections are shown when browsing; groups collapse when filtering
5. Empty-state copy is present and actionable
6. Omnibar placeholder includes `@alias` as an example input
7. Alias resolution chip appears as user types (not only on submit)
8. All alias palette interactions are keyboard-navigable (arrow keys to navigate, Enter to select, Escape to dismiss)
9. Screen reader: alias palette has `role="listbox"` or `role="menu"` with appropriate `aria-label`
