# Requirements: Omni Bar Session Search

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-14

## Problem Statement

The Omnibar (`Cmd+K`) is currently a **session creation tool only** — it has no way to jump to an existing session. Users who want to navigate to a session they already created must close the Omnibar, scroll the session list, and click manually. Session creation from known repos is also tedious because users must retype full paths even for repositories they use every day. The directory tree traversal (for path completion) is expensive and uncached, causing perceptible latency on repeated opens.

Two user flows are broken:
1. **Jump to existing session** — no keyboard-first way to navigate to a running/paused session by name, branch, or path
2. **Create session from recent repo** — no "recent repos" shortcut; full path must be retyped every time

## Success Criteria

1. User can reach any existing session via Omnibar in ≤3 keystrokes after `Cmd+K`
2. User can start a new session on any previously-used repo in ≤5 keystrokes after `Cmd+K`
3. Path completion results appear in <100ms on repeated opens (cache hit) and <500ms cold (first open after restart)
4. Fuzzy search matches feel natural — "myfeat" matches "my-feature-branch", "squad" matches "stapler-squad"

## Scope

### Must Have (MoSCoW)

- **Session search in Omnibar** — fuzzy search across existing sessions (by title, branch, path, tags, program); selecting a result navigates to that session
- **Recent repos quick-pick** — list of recently-used repo paths displayed when the input is empty or starts with a non-path query; selecting one pre-fills the path input for fast new-session creation
- **Fuzzy matching quality** — proper fuzzy algorithm (e.g., fzf-style scoring: consecutive character bonus, start-of-word bonus) replacing naive substring for both session search and path completion
- **Directory tree cache** — cache the directory tree listing between calls, keyed by root path + mtime; avoids full filesystem traversal on repeated Omnibar opens
- **Keyboard shortcut to open** — `Cmd+K` (already implemented) opens the Omnibar from anywhere in the app

### Nice to Have

- Unified result list mixing sessions and repo paths (ranked by relevance)
- Session result shows status badge (Running, Paused) inline
- Recent repos show last-used timestamp
- Persistent cache across server restarts (disk-backed)

### Out of Scope

- Global OS-level hotkey (system-wide, not just when app is focused)
- AI/LLM-powered suggestions or recommendations
- Non-session navigation (Settings, Help, etc.)
- Mobile / responsive layout
- Remote path support (SSH, network mounts)

## Constraints

**Tech stack (already decided):**
- Frontend: React + TypeScript, ConnectRPC for API calls, vanilla-extract CSS (new components)
- Backend: Go, ConnectRPC/Protobuf
- Existing BM25 search engine in `session/search/` — must extend or replace with fuzzy-capable engine
- Existing `path_completion_service.go` — must add caching layer
- Existing `Omnibar.tsx` with `usePathCompletions`, `usePathHistory`, `useWorktreeSuggestions` hooks
- `PathCompletionDropdown.tsx` component already exists

**Cache strategy:** Open question — to be decided in research. Options: in-memory with TTL, disk-backed JSON, or background refresh on startup.

**Search scope:** Session search operates on the in-memory session store (already loaded); no new persistence layer needed.

**Dependencies:** Builds on top of completed `omni-bar-path-completion` work (path completion is done; session search is the new layer).

## Context

### Existing Work

The path completion phase (`project_plans/omni-bar-path-completion/`) is complete:
- `usePathCompletions` hook with debounced API calls
- `PathCompletionDropdown` component with keyboard navigation
- `path_completion_service.go` backend for directory listing and path validation
- `usePathHistory` and `useWorktreeSuggestions` hooks

What's NOT done (this project covers):
1. Session search/navigation from Omnibar (was explicitly out of scope in path-completion project)
2. Recent repos shortcut for session creation
3. Fuzzy matching quality (current search is BM25 / substring; not fzf-quality)
4. Directory tree caching (no caching layer currently in `path_completion_service.go`)

### Stakeholders

- Solo practitioner (Tyler) — primary user and developer
- Workflow: Heavy multi-session usage across many repositories; frequent context-switching between running sessions

## Research Dimensions Needed

- [ ] Stack — fuzzy matching libraries for Go and TypeScript; caching patterns for filesystem data
- [ ] Features — survey VS Code Command Palette, Raycast, Linear search UX patterns; how do they blend "navigate to existing" + "create new" in one input?
- [ ] Architecture — where does session search live (client-side filter vs server RPC?); unified result ranking; cache invalidation for directory tree
- [ ] Pitfalls — stale cache after directory changes; keyboard UX conflicts between session-select mode and path-completion mode; result ranking relevance drift
