# Research Plan: Omni Bar Path Completion

Created: 2026-04-07
Status: Complete

## Subtopics

### 1. Features — Path Completion UX Patterns
**Goal:** Survey comparable tools (fzf, fish shell, VS Code file picker, zsh) for UX patterns in interactive path completion
**Search strategy:**
- fzf path completion behavior (fuzzy matching, preview, keybindings)
- Fish shell path completion UX
- VS Code QuickPick file picker patterns
- Web-based path completion components (react-autocomplete, downshift)
**Search cap:** 4 searches
**Output:** `research/findings-features.md`

### 2. Architecture — Client/Server Path Completion Design
**Goal:** Design patterns for debounce strategy, caching, streaming vs batch, API shape
**Search strategy:**
- Debounce patterns for autocomplete API calls
- Path completion API design (prefix vs fuzzy, response shape)
- ConnectRPC streaming vs unary for autocomplete
- Go filesystem listing with context cancellation
**Search cap:** 4 searches
**Output:** `research/findings-architecture.md`

### 3. Pitfalls — Known Failure Modes
**Goal:** Document known edge cases: permissions, symlinks, tilde expansion, slow NFS, race conditions
**Search strategy:**
- Path completion edge cases (symlinks, permissions, network paths)
- Tilde expansion in Go / browser path inputs
- Race conditions debounced inputs React
- NFS/slow filesystem path listing timeout patterns
**Search cap:** 4 searches
**Output:** `research/findings-pitfalls.md`
