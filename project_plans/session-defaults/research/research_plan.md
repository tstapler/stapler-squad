# Research Plan: Session Defaults

## Subtopics

### 1. Stack (codebase analysis)
Strategy: Read Go source files in config/, session/, server/
Search cap: N/A — codebase only, no web search needed
Output: research/stack.md

### 2. Features (external survey)
Strategy: Web search for tmux sessionizer, Wezterm workspaces, Zellij layouts, iTerm2 profiles UX patterns
Search cap: 4 searches max
Output: research/features.md

### 3. Architecture (design)
Strategy: Read codebase for ConnectRPC patterns, proto definitions, React create-session flow; then design
Search cap: Codebase reads primary; 2 web searches if needed
Output: research/architecture.md

### 4. Pitfalls (risk analysis)
Strategy: Codebase reads for config migration patterns + web search for known failure modes
Search cap: 3 searches max
Output: research/pitfalls.md
