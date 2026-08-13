# Requirements: Backlog Management Layer

**Project**: backlog-management  
**Date**: 2026-05-10  
**Status**: Draft

---

## Problem Statement

Stapler Squad is excellent at spawning isolated agent sessions and managing their lifecycle, but it has no higher-level coordination layer. Users manage what to work on and in what order entirely in their head. There is no mechanism to:

- Maintain a prioritized, context-rich backlog of work items
- Help agents stay on task with acceptance criteria baked into their context
- Detect when agents drift, get stuck, or finish work
- Enforce quality gates before marking work complete
- Pull tasks from external sources (GitHub Issues) without manual copy-paste

**Inspired by, and learning from the failures of, Gastown and Beads:**

| Problem | What went wrong | How we avoid it |
|---------|----------------|-----------------|
| Lost task context | Tasks lost their "why" and AC as agents worked | AC injected into session context + preserved in DB |
| Poor visibility | Hard to see done vs. planned | Structured status lifecycle with session linkage |
| Agent drift | Agents silently worked on wrong things | Post-run review gate validates diff against AC |
| No human checkpoints | System ran too autonomously | Explicit approve gates at spawn + review + close |

---

## Goals

1. **Curated backlog**: Users can create, refine, and prioritize work items with rich context (why, acceptance criteria, notes, labels)
2. **Agent-assisted triage**: An embedded agent helps flesh out vague items, estimates effort, suggests ordering, and proposes what to work on next
3. **Session spawning with context**: Approved items become sessions with their full context injected (via CLAUDE.md or session instructions)
4. **Live monitoring + notifications**: Running sessions are linked to backlog items; users are notified when agents ask questions or need review
5. **Acceptance criteria enforcement**: Post-session review gate validates outcomes against criteria before items are closed
6. **Plugin architecture for sources**: GitHub Issues is the first external source; the architecture allows new sources without core changes

---

## Non-Goals (MVP)

- Full autonomous drain (agent picks and runs items without approval)
- Linear, Jira, or other external source plugins (post-MVP)
- Sprint planning, velocity tracking, burndown charts
- Multi-user / team collaboration (single-user Stapler Squad is current scope)
- AI-generated acceptance criteria without human review

---

## User Stories

### Core Backlog Management

**US-1**: As a user, I can create a backlog item with a title, description, acceptance criteria, labels, and priority so that work intent is captured with enough context for an agent to execute it.

**US-2**: As a user, I can view my backlog organized by status (Idea → Ready → In Progress → Review → Done) so that I can see the state of all planned work at a glance.

**US-3**: As a user, I can edit, reorder, and archive backlog items so that the backlog stays clean and reflects current priorities.

**US-4**: As a user, I can ask the backlog agent to help me flesh out a vague item (expand description, suggest acceptance criteria, identify dependencies) so that items reach "Ready" quality before sessions are spawned.

### Session Integration

**US-5**: As a user, I can spawn a Stapler Squad session directly from a backlog item so that the session receives full context (description, AC, notes) injected into its working environment.

**US-6**: As a user, I am notified when an agent working on a backlog item asks a question or hits a blocker, so that I can unblock it without polling the terminal.

**US-7**: As a user, I can see which backlog item a running session is working on, and the session's progress against acceptance criteria, from the session list view.

### Review Gate

**US-8**: As a user, when an agent session completes, a review gate automatically runs that evaluates the session's output (git diff, test results) against the item's acceptance criteria and presents a PASS/FAIL/PARTIAL verdict before the item is marked Done.

**US-9**: As a user, I can override a review gate verdict (mark Done despite PARTIAL, or reopen despite PASS) so that I retain final authority.

**US-10**: As a user, I can trigger a manual re-review of any completed item's linked session at any time.

### GitHub Issues Integration

**US-11**: As a user, I can configure a GitHub repository as an issue source so that open issues are synced to my backlog as items.

**US-12**: As a user, I can map GitHub issue labels to backlog priorities and see sync status (last synced, skipped, conflicts).

**US-13**: As a user, changes I make to a synced backlog item (status, notes) are not overwritten by future syncs; only fields I have not locally modified are refreshed.

### Agent API (MCP / Hooks)

**US-14**: As an agent running inside a Stapler Squad session, I can call an MCP tool to report progress against acceptance criteria so that the backlog item's live status is updated.

**US-15**: As an agent, I can call an MCP tool to request human review/approval and pause my work, so that the human sees a notification and can respond before I continue.

**US-16**: As a hook, I can trigger the review gate to run on session completion without requiring explicit user action.

---

## Acceptance Criteria (Feature-Level)

### Backlog CRUD
- [ ] Create item with title, description, markdown AC list, labels (multi), priority (1–5), optional source link
- [ ] Status state machine: `idea → ready → in_progress → review → done | archived`
- [ ] Transitions: `idea→ready` requires non-empty AC; `in_progress→review` triggered by session completion hook; `review→done` requires gate pass or manual override
- [ ] List view with filter by status, label, priority; sort by priority/updated

### Session Linkage
- [ ] `BacklogItem` has `[]SessionID` (one item can have multiple attempt sessions)
- [ ] Active session shows item title and AC completion badge in session list
- [ ] Spawning from item creates session with item context file injected

### Review Gate
- [ ] Gate runs `git diff` against base branch, maps changes to AC items
- [ ] Gate produces structured verdict: per-AC item `PASS|FAIL|UNVERIFIABLE`
- [ ] Verdict stored on the session-item link, viewable in UI
- [ ] Human override stored with reason

### GitHub Sync
- [ ] Configurable per-repo: org/repo, label filters, sync interval
- [ ] Conflict model: local-wins for user-modified fields (description, AC, priority)
- [ ] Sync log with per-item result (created, updated, skipped, error)

### MCP Extensions
- [ ] `report_progress(itemId, criteria_index, status, note)` — updates live status
- [ ] `request_review(itemId, message)` — sends notification, pauses agent (via approval hook)
- [ ] `get_backlog_item(itemId)` — returns full item context for self-orientation

---

## Technical Constraints

- **Storage**: ent/SQLite — new schemas via ent ORM following existing patterns in `session/ent/`
- **Backend**: Go, ConnectRPC — new service file(s) in `server/services/`
- **Frontend**: React SPA + vanilla-extract CSS — new components in `web-app/src/components/`
- **MCP**: Extend or add to the existing `stapler-squad-mcp-server` project plan
- **Additive only**: No breaking changes to existing session management; backlog is opt-in
- **Proto**: New `.proto` file for backlog domain; regenerate with `make generate-proto`

---

## Open Questions

1. **Agent lifecycle model**: Should the triage agent be a persistent background session, on-demand, or event-driven? (Research should recommend)
2. **Context injection mechanism**: How does item context reach the spawned session? Options: (a) generate a `.backlog-context.md` in the worktree, (b) pass as session instructions via `CLAUDE.md` prepend, (c) MCP resource the agent can pull
3. **Review gate executor**: Who runs the review — a dedicated short-lived agent session, or a Go service that calls an LLM API directly?
4. **Drift detection**: How do we detect silent agent drift without reading every terminal line? (polling git diff? token budget tracking?)

---

## Risky Assumptions

These are the bets the feature makes that could invalidate the design if wrong:

| # | Assumption | How to falsify | Mitigation |
|---|---|---|---|
| A-1 | Users will trust LLM review gate verdicts enough to act on them without re-reading the entire diff | Users bypass the gate >50% of the time ("Skip gate, mark done") even when items have AC | Adversarial prompt framing + mandatory citations make verdicts legible; human override is always available |
| A-2 | GitHub Issues is the right first external source (vs Linear or Jira, which are common in professional settings) | Zero GitHub sources configured after 30 days of availability | Scoped post-MVP; plugin architecture means adding Linear is additive, not a rewrite |
| A-3 | A structured backlog provides enough value over a markdown TODO file for a single-user workflow | User abandons the feature within 2 weeks and reverts to a plaintext list | "Idea → session running" must be faster and less friction than the markdown alternative |
| A-4 | Context injection (initial prompt + slash commands) is sufficient for agents to stay on task without human re-orientation | Review gate PARTIAL rate > 60% across first 20 sessions | `.backlog-context.md` and `get_backlog_item` provide re-orientation; drift detection surfaces stuck sessions early |

---

## Success Metrics

| Metric | Target | Instrumentation |
|---|---|---|
| Time from item creation to first session spawned | < 2 minutes (median) | Server logs: `CreateBacklogItem` timestamp → `SpawnSessionFromItem` timestamp; logged as `backlog_spawn_latency_ms` |
| Review gate verdict coverage | > 90% of completed work sessions have a verdict | `ItemSession` records with `session_role="work"` and `ended_at` set; verdict coverage = those with a linked `ReviewVerdict` |
| Context retention | Backlog items have non-empty AC when they reach `in_progress` | `SpawnSessionFromItem` handler logs `ac_count` at spawn; alert if 0 (item bypassed the `idea→ready` guard) |
| Regression guard | Zero existing session creation flows broken | IT-011 smoke test in CI; manual test on every release: create/pause/resume/stop session via Omnibar with no backlog config |

*Baseline*: No prior measurement exists (feature is net-new). The 2-minute target is set by the "faster than markdown" bar — creating a GitHub issue + copying it into a session takes ~3–5 minutes manually.
