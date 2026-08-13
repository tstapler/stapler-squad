# Research Plan: History Page Revamp

**Created**: 2026-04-12
**Input**: `project_plans/history-page-revamp/requirements.md`

## Current State Summary (from codebase audit)

Key observations that shape what to research:

- **Frontend**: React/Next.js page at `web-app/src/app/history/page.tsx`. Loads `limit: 500` sessions eagerly in a single RPC call. No virtual scrolling. Detail panel requires click to populate.
- **Backend**: `SearchService` in `server/services/search_service.go` — full TTL-cached history load from disk (`session.NewClaudeSessionHistoryFromClaudeDir()`). BM25 full-text search exists. `ClaudeHistoryEntry` proto lacks branch, diff count, and status fields.
- **Fork/resume**: `handleResumeSession` creates a session in the same project path. No worktree-picker, no directory selector — hardcoded to `entry.project`.
- **Message preview**: Already fetches last 5 messages on selection; shown in right-side detail panel. Not inline on cards.
- **Search**: Both metadata search (client-side filter) and full-text BM25 search exist but are in separate modes.

## Research Subtopics

### 1. Stack (→ `research/stack.md`)

**Goal**: Identify the best React list virtualization approach and any missing proto fields.

**Search strategy**:
- `TanStack Virtual react list virtualization 2024 2025`
- `react-window vs tanstack virtual performance comparison`
- `next.js app router virtualized list server components`
- `connectrpc streaming pagination go backend`

**Scope limit**: 4 searches max

**Output focus**:
- Which virtualization library fits best given Next.js App Router + no bundler constraints
- Whether streaming (server-side pagination via ConnectRPC streaming) vs cursor-based pagination is the right approach for 200ms budget
- Any ANSI stripping libraries for safe message preview rendering

---

### 2. Features (→ `research/features.md`)

**Goal**: Survey UX patterns in comparable tools for session identification and fork/clone workflows.

**Search strategy**:
- `Linear issue list UX session context identification design`
- `GitHub Codespaces fork branch workflow UX pattern`
- `Cursor AI conversation history UX session resume`
- `Claude.ai conversation sidebar history UX patterns 2025`
- `developer tool session history card design rich metadata`

**Scope limit**: 5 searches max

**Output focus**:
- What metadata comparable tools surface per item in a history list
- How fork/branch/clone actions are presented (modal vs inline vs dropdown)
- Inline message preview patterns (expand-in-place vs slide-out panel)
- Any patterns for "still running" vs "completed" session differentiation

---

### 3. Architecture (→ `research/architecture.md`)

**Goal**: Design the API shape, state management, and component architecture for the revamp.

**Search strategy**:
- `cursor pagination go connectrpc streaming history list`
- `react tanstack query infinite scroll connectrpc`
- `react expandable card inline message preview performance pattern`
- `worktree picker modal react UX clone branch flow`

**Scope limit**: 4 searches max

**Output focus**:
- Cursor-based vs offset pagination for `ListClaudeHistory` — which to add to proto
- What new proto fields `ClaudeHistoryEntry` needs (branch, status, diff_count, last_commit)
- Component breakdown: which pieces are new vs refactor of existing
- How to handle the fork action: new `ForkHistorySession` RPC vs reusing `CreateSession` with extra params

---

### 4. Pitfalls (→ `research/pitfalls.md`)

**Goal**: Identify known failure modes before implementation.

**Search strategy**:
- `react virtual scroll edge cases anchor scroll position bugs`
- `git worktree conflicts same branch multiple worktrees`
- `ANSI escape code stripping react text preview security`
- `next.js app router useEffect infinite loop data fetching`

**Scope limit**: 4 searches max

**Output focus**:
- Virtual scroll scroll-anchor drift, keyboard navigation edge cases
- Worktree fork conflicts (branch already checked out, detached HEAD)
- ANSI codes in message text: stripping vs rendering, XSS surface
- Stale cache issues when history is updated while page is open

## Parallelization

All 4 subtopics are independent — spawn simultaneously:

| Agent | Output file |
|---|---|
| stack | `research/stack.md` |
| features | `research/features.md` |
| architecture | `research/architecture.md` |
| pitfalls | `research/pitfalls.md` |
