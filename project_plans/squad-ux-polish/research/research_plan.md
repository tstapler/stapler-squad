# Research Plan: Squad UX Polish

**Requirements source**: `project_plans/squad-ux-polish/requirements.md`
**Date**: 2026-04-17

## Subtopics

### 1. Stack
**Focus**: What new libraries or storage mechanisms are needed to implement prompt persistence, session templates, and a review queue UI?

**Search strategy**:
- Prompt/command history storage patterns in Go CLI apps and web apps
- Lightweight embedded storage options for small datasets in Go (beyond the existing JSON files)
- React virtual list / infinite scroll libraries for review queue UI
- Template engine patterns for session config templates

**Axes for trade-off matrix**: Bundle size / Go dependency weight, Complexity to integrate, Persistence guarantees, Existing pattern match

**Search cap**: 4 queries

**Output**: `research/findings-stack.md`

---

### 2. Features
**Focus**: How do comparable tools handle batch session creation, prompt injection at start, and completed-work review/merge queues? What UI patterns work best?

**Tools to survey**:
- Gastown / Refinery concept (Yegge's multi-agent fleet)
- Cursor (AI-native IDE) — session/task management
- claude-flow (multi-agent orchestration)
- tmuxinator / tmuxp (tmux session templates)
- GitHub CLI workflow (PR creation from terminal)
- Linear/Jira — batch issue creation patterns

**Axes**: Batch creation UX, Prompt-at-start support, Review/merge workflow, Discoverability

**Search cap**: 5 queries

**Output**: `research/findings-features.md`

---

### 3. Architecture
**Focus**: Data model design for "project" (session grouping), review queue state machine integration, and prompt library storage.

**Key questions**:
- Where does `Project` fit in the entity hierarchy? (workspace → project → session, or session → project via tags?)
- How does the review queue state machine work? (completed → queued-for-review → merged/closed)
- Should prompt library be global config or per-project?
- How does batch session creation interact with existing `CreateSession` RPC?
- PR creation: GitHub CLI vs direct API?

**Axes**: Backwards compatibility, Extensibility, Implementation complexity, Storage impact

**Search cap**: 4 queries

**Output**: `research/findings-architecture.md`

---

### 4. Pitfalls
**Focus**: What goes wrong with batch session creation, prompt delivery, and automated merge flows?

**Key risks to research**:
- Batch session creation: tmux session naming collisions, git worktree conflicts
- Prompt delivery timing: session must be fully attached before prompt is sent; race conditions
- PR creation: auth token scoping, rate limits, branch protection rules blocking automation
- Review queue: stale sessions, diverged branches, merge conflicts surfaced late
- Template drift: templates becoming stale as the codebase evolves

**Axes**: Severity, Likelihood, Mitigation available, Detection difficulty

**Search cap**: 4 queries

**Output**: `research/findings-pitfalls.md`

---

## Synthesis Target

After all 4 subagents complete: `research/synthesis.md`

The synthesis feeds `/plan:feature` (Phase 3) and informs ADR decisions for:
- Project data model (grouping strategy)
- Prompt persistence storage approach
- Review queue state machine design
- Batch session creation API shape
