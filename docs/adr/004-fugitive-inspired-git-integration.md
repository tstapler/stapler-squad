# ADR 004: Fugitive-Inspired Git Integration

## Status
Accepted

## Context
Claude-squad currently has basic git handoff commands (commit/push, checkout/pause, resume) that use single-key bindings but lack the sophisticated workflow patterns that make git operations intuitive and efficient. The current commands feel disconnected from modern git workflows and don't leverage the interactive, buffer-based patterns that Vim users expect.

Vim-fugitive is widely regarded as the gold standard for Git integration in terminal-based editors, providing an elegant, interactive workflow that feels native to Vim's modal editing paradigm.

## Decision
We will redesign stapler-squad's git integration to follow **vim-fugitive's design patterns and workflow philosophy**.

### Core Principles:

1. **Centralized Git Command Interface**: Use `:G` as the primary entry point for all git operations
2. **Interactive Status-Driven Workflow**: Center git operations around an interactive git status overlay
3. **Context-Sensitive Mappings**: Provide different keybindings within git mode vs normal session mode  
4. **Staging-Centric Operations**: Make staging/unstaging the core of the git workflow
5. **Mnemonic Git Actions**: Use logical, memorable keys within git contexts
6. **Modal Integration**: Git operations should feel like entering a "git mode" within our interface

### Specific Patterns to Adopt:

#### Command Interface:
- **`:G`** - Enter git status interface (like `:Git` in fugitive)
- **`:G <command>`** - Execute arbitrary git commands
- **ESC** - Exit git mode back to session interface

#### Git Status Interface Mappings:
- **`s`** - Stage file/hunk under cursor
- **`u`** - Unstage file/hunk under cursor  
- **`-`** - Toggle staging for file/hunk
- **`U`** - Unstage all changes
- **`cc`** - Create commit (opens commit message interface)
- **`ca`** - Amend last commit
- **`dd`** - Show diff for file under cursor
- **`p`** - Push current branch
- **`P`** - Pull current branch
- **`<CR>`** - Open/edit file under cursor

#### Integration with Session Workflow:
- **Pause Integration**: `c` (checkout) becomes part of git status workflow
- **Resume Integration**: Automatically detect and offer to resume paused sessions
- **Session Context**: Git operations are contextual to the current session

## Rationale

1. **Familiar Patterns**: Developers already using fugitive will have instant familiarity
2. **Workflow Efficiency**: Staging-based workflow is more intuitive than current atomic operations  
3. **Discoverability**: Interactive interface makes git operations self-documenting
4. **Modal Consistency**: Aligns with our vim-centric ADR-003 philosophy
5. **Scalability**: Easier to add new git features within established patterns

## Alternatives Considered

1. **GitHub CLI Integration**: Would require external dependencies and less integration
2. **Lazygit-style Interface**: Too complex for our focused use case
3. **Keep Current Commands**: Lacks sophistication and modern git workflow patterns
4. **Custom Git Abstraction**: Would require significant design work and lack familiarity

## Consequences

### Positive:
- More intuitive git workflows for vim users
- Better integration between git operations and session management
- Easier to discover and learn git features
- Scalable foundation for additional git features
- Consistent with vim-centric design philosophy

### Negative:
- Larger implementation effort than incremental changes
- May be initially unfamiliar to non-fugitive users  
- Requires new overlay/interface components
- Breaking change for existing handoff command users

## Implementation Guidelines

1. **Phased Implementation**: Build git status interface first, then add commands incrementally
2. **Backward Compatibility**: Maintain existing commands during transition period
3. **Documentation**: Provide clear help within git interface
4. **Testing**: Validate with both fugitive users and git newcomers
5. **Integration**: Ensure git mode feels seamless with session management

## Success Metrics

- Git operations become self-discoverable through interface
- Reduced cognitive load for complex git workflows  
- Positive feedback from vim-fugitive users
- Seamless integration with session pause/resume workflow
- Reduced need for external git commands

This ADR establishes fugitive-style git integration as our target state while maintaining compatibility with stapler-squad's session-centric workflow.