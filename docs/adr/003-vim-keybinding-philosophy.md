# ADR 003: Vim as North Star for Keybindings and TUI Design

## Status
Accepted

## Context
Claude-squad is a terminal-based user interface (TUI) application that requires intuitive and efficient keybindings for session management, navigation, and organization. As the application grows in complexity, we need consistent principles to guide keybinding choices and interface design decisions.

## Decision
We will use **Vim as the north star** for all keybinding and TUI design decisions in stapler-squad.

### Core Principles:

1. **Modal Thinking**: Different modes should have context-appropriate keybindings
2. **Mnemonic Keys**: Use memorable, logical key mappings (e.g., 's' for search, 'f' for filter)
3. **Vim Navigation**: Prefer Vim-style navigation where applicable (h/j/k/l, though arrows are also supported)
4. **Composable Actions**: Actions should be composable and predictable
5. **Escape for Cancel**: ESC should consistently cancel/exit current mode
6. **Single-Key Commands**: Prefer single keystrokes over key combinations where possible

### Specific Applications:

- **Navigation**: Support both arrow keys and Vim keys (j/k for up/down)
- **Search**: 's' to search, ESC to cancel, Enter to apply
- **Filtering**: Single letters for filters (f for filter paused)
- **Actions**: Mnemonic single keys (c for clear, r for resume, etc.)
- **Categories**: Space to toggle (similar to file managers), arrows to expand/collapse
- **Modal Overlays**: ESC always cancels, Enter confirms

## Rationale

1. **Familiar to Target Users**: Developers using terminal applications often know Vim
2. **Efficiency**: Single-key commands are faster than key combinations
3. **Consistency**: Vim provides well-established patterns for TUI interaction
4. **Scalability**: Vim's modal approach scales well as features are added
5. **Muscle Memory**: Users can leverage existing Vim knowledge

## Alternatives Considered

1. **Emacs-style keybindings**: Rejected due to heavy reliance on Ctrl combinations
2. **IDE-style shortcuts**: Rejected as less suitable for terminal environment  
3. **Custom system**: Rejected due to lack of user familiarity and established patterns

## Consequences

### Positive:
- Intuitive interface for terminal users
- Efficient single-key commands  
- Consistent mental model for new features
- Leverages existing user knowledge

### Negative:
- May be less familiar to non-Vim users
- Need to document keybindings clearly
- Some actions may require creative mapping to maintain mnemonic value

## Implementation Guidelines

1. **New Features**: Always consider Vim-style interaction patterns first
2. **Key Conflicts**: Resolve using Vim's precedence and modal thinking
3. **Documentation**: Provide both help screens and quick reference
4. **User Testing**: Validate keybindings with developers familiar with Vim
5. **Fallbacks**: Support arrow keys alongside Vim keys for broader accessibility

## Examples

- `j/k` or `↓/↑` for navigation
- `s` for search (like Vim's search)
- `esc` for cancel (universal Vim pattern)
- `/` could be alternative search (classic Vim)
- `space` for toggle actions (common in file managers)
- Single letter commands: `c` clear, `f` filter, `r` resume

This ADR establishes Vim as our guiding philosophy while maintaining accessibility for users less familiar with Vim through arrow key support and clear documentation.