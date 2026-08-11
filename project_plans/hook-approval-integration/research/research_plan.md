# Research Plan: Hook Approval Integration

**Created**: 2026-04-07
**Based on**: requirements.md

## Subtopics

### 1. Stack — Existing codebase internals
**Goal**: Understand how the review queue currently works; what `EventBus` events exist for approvals; how `ApprovalStore` integrates (or doesn't) with queue logic.

**Strategy**: Code exploration only — no web search needed. Read:
- `server/services/` — ApprovalStore, ApprovalHandler, ReviewQueueChecker
- Session service / review queue query logic
- EventBus event types related to approvals
- Frontend components that render the review queue

**Search cap**: 5 codebase searches
**Output file**: `findings-stack.md`

---

### 2. Features — Cross-view state sync patterns
**Goal**: Understand how other tools handle cross-view consistency for action items; what UX patterns exist for approval/action queues.

**Strategy**: Web search (Brave) for patterns in workflow UIs, multi-tab state sync, optimistic updates in React.

**Search cap**: 4 web searches
**Output file**: `findings-features.md`

---

### 3. Architecture — SSE/EventBus vs polling for approval broadcast
**Goal**: Determine the right pattern for broadcasting approval state changes to all connected views. Understand how the frontend currently receives review queue updates.

**Strategy**: Code exploration (how SSE/EventBus is currently used for other real-time updates) + web search for best practices on SSE-based invalidation.

**Search cap**: 3 codebase + 2 web searches
**Output file**: `findings-architecture.md`

---

### 4. Pitfalls — Race conditions, orphaned approvals, concurrent approvals
**Goal**: Identify potential failure modes in the approval resolution flow; document defensive strategies.

**Strategy**: Code analysis (race-prone patterns in ApprovalHandler/ApprovalStore) + web search for Go race condition patterns in concurrent approval systems.

**Search cap**: 3 codebase + 2 web searches
**Output file**: `findings-pitfalls.md`

---

## Parallelization

Subtopics 1, 3, 4 are primarily codebase research — spawn as parallel agents.
Subtopic 2 requires web search — can run in parallel with the others.

Synthesis: Read all 4 findings files after all agents complete.
