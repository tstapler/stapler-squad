# Research: Comparable Systems for Backlog Management Feature

**Date**: 2026-05-10  
**Author**: Research Agent  
**Scope**: Comparable products and open-source projects informing the Stapler Squad backlog-management feature

---

## Summary

- **Gastown and Beads both failed because they treated agents as reliable executors rather than unreliable collaborators**: Gastown's plate-spinning monitoring burden, silent drift, and runaway costs expose what happens when orchestration has no human checkpoints. Beads' DAG-backed memory is the right idea but requires explicit CLAUDE.md prompting to activate and degrades when context grows, proving that external state alone is insufficient without session-start/end lifecycle hooks.
- **Linear's opinionated 5-state lifecycle (Triage → Backlog → In Progress → Done + Canceled) is the right model to steal**, combined with its automation-driven status transitions (PR opened → In Progress, PR merged → Done). The gate on `idea→ready` requiring non-empty AC maps exactly to Linear's Triage → Backlog promotion requiring human sign-off.
- **LangGraph's `interrupt()` pattern — serialized checkpoint, suspend execution, await human `Command(resume=...)` — is the only production-validated human-in-the-loop primitive worth implementing**; every other framework (CrewAI, AutoGen) bolts it on as an afterthought and breaks under async conditions.

---

## Comparable Systems Analysis

### Gastown (Steve Yegge, gastownhall/gastown)

**What it is**: A multi-agent workspace manager ("Kubernetes for AI coding agents") that runs dozens of Claude Code instances simultaneously, orchestrated by specialized persistent agents (Mayor, Deacon, Witness, Refinery).

**What it tried to do**: Fully autonomous coding at scale — spawn many parallel worker agents, have a Witness agent verify output, Deacon monitor for stalled agents, Refinery merge completed work, Mayor coordinate overall task allocation.

**Key agent roles**:
- **Mayor**: orchestrates work allocation across workers
- **Witness**: validates agent output against acceptance criteria
- **Deacon**: monitors for stuck/stalled agents
- **Refinery**: manages merge queue and conflict resolution

**Known shortcomings**:
1. **Monitoring is plate-spinning**: Users must cycle through each worker agent to check status; there's no unified visibility into multi-agent state. The system's frenetic energy creates cognitive overload.
2. **Planning becomes the bottleneck**: Gastown churns through implementation plans so fast that design and planning can't keep up. The engine has no backpressure mechanism.
3. **Runaway costs with no safety rails**: $100/hour burn rates, auto-merged failing tests, agents working on wrong things. No per-task budget cap or approval gate before expensive operations.
4. **Vibe-coded architecture**: 100% vibecoded — the creator never reviewed the code. Integration bugs (e.g., hardcoded `gt-` prefix but stored with `hq-` prefix on beads) caused cascading failures.
5. **Agent drift with no detection**: After ~10 minutes or ~50 messages in context, agents "lose the plot" — forgetting what they fixed and what they're supposed to do next. No telemetry to detect this without reading every terminal.
6. **No consumer readiness**: Described by its creator as "you probably don't want to use it yet." Requires users to operate "as if managing a very fast, very junior dev team."

**Relevance to Stapler Squad**: Gastown proves that the Deacon/Witness roles (monitoring + validation) are necessary components of any multi-agent system. Stapler Squad's review gate is the Witness pattern implemented safely: it runs post-hoc rather than inline, reducing cost and race conditions.

---

### Beads (Steve Yegge, gastownhall/beads)

**What it is**: A Git-backed, DAG-structured issue tracker designed specifically for AI agents — a "memory upgrade" for coding agents that externalizes task state outside the LLM context window.

**What it tried to do**: Solve context loss across sessions by storing tasks in a version-controlled database (initially Git, later Dolt for branching SQL). Agents query Beads at session start to understand what's done and what's next; they update Beads at session end with progress.

**Architecture**:
- Dolt (version-controlled SQL) as the backing store for atomic cell-level merges
- DAG dependency model: tasks have explicit blocking/blocked-by relationships
- "Ready work" detection: automatically surfaces tasks with no blocking dependencies and highest priority
- Memory decay: closed tasks get summarized to free context window
- Hash-based IDs to reduce merge collisions across multi-agent/multi-branch workflows

**Known shortcomings**:
1. **Agents won't query Beads unprompted**: Claude and other agents ignore Beads unless CLAUDE.md explicitly instructs them to check it at session start and end. This is a protocol compliance problem: the tool is optional, not structural.
2. **Context degradation weakens compliance**: As the session context grows, CLAUDE.md instructions lose relative weight. Agents stop following the "Land the Plane" (session-end sync) protocol mid-session, leaving Beads out of sync.
3. **No automatic session-end sync**: `bd sync` must be triggered manually or via hooks; there's no reliable mechanism to guarantee it runs before session termination.
4. **Performance ceiling at 200-500 tasks**: Query performance degrades noticeably beyond 200 tasks per project database.
5. **Early-stage instability**: Schema migrations are occasional; version upgrades require manual data adjustments.
6. **Merge conflicts still occur**: Hash-based IDs reduce but do not eliminate conflicts when multiple agents edit overlapping items.
7. **No human approval layer**: Beads tracks state but has no built-in human checkpoint before a task moves from Ready to In Progress. Agents can self-assign and start work without a human "spawn approved" gate.

**Key insight for Stapler Squad**: Beads is right that task state must live outside the agent's context window. But the failure mode — agents not checking Beads — reveals that read/write to task state must be structurally enforced (via session spawn injection and completion hooks), not relied upon as agent behavior. The review gate and MCP `report_progress` tool fill this gap.

---

### Linear

**Backlog and state model**: Linear's conceptual model centers on the **issue** as the atomic unit of work. Issues move through a five-category state machine:

```
Triage → Backlog → Unstarted (Todo) → Started (In Progress / In Review) → Completed (Done)
                                                                        → Canceled
```

- **Triage**: inbox for unreviewed issues created by integrations or non-team members; issues must be promoted by a human before entering the team's backlog. This is a hard gate.
- **Backlog**: accepted but not yet scheduled. Priority, labels, estimates live here.
- **In Progress**: actively being worked. Linear's GitHub integration auto-transitions issues here when a branch with the issue ID is pushed.
- **Done / Canceled**: terminal states. PR merge → Done is automated via integration.

**What makes the UX work**:
1. **Keyboard-first navigation** with real-time sync — zero latency between state changes
2. **Opinionated defaults, not infinite customization**: Linear intentionally restricts workflow customization (you can't rename state categories, only label them), which reduces decision fatigue
3. **Cycle-based focus**: Cycles (1–8 week sprints) pull items from backlog into a focused view; backlog is the infinite queue, cycle is the committed slice
4. **Triage as a forcing function**: Triage creates a mandatory review step that prevents unreviewed noise from polluting the team's working backlog
5. **Status-PR bi-directional sync**: copying a git branch name or opening a PR automatically updates issue status — agents following the same pattern could do this via hooks

**State model worth borrowing**: The Triage→Backlog gate with human sign-off maps directly to the `idea→ready` transition requiring non-empty AC and explicit promotion. Linear's automation-driven status (branch push → In Progress, PR merge → Done) maps to session spawn → `in_progress`, session completion hook → `review`.

---

### GitHub Projects v2

**Issue model**: GitHub Projects v2 decouples the **issue** (content: title, body, labels, assignees, comments) from the **project item** (metadata: custom fields, status, iteration). An issue can exist in multiple projects with independent status per project.

**Custom fields**: Text, number, date, single-select (status), iteration (sprint-like). Custom status fields are the primary mechanism for workflow state — not the issue's native open/closed state.

**Built-in automation**:
- Item added to project → status = "Todo"
- Issue closed → status = "Done"  
- PR merged → status = "Done"
- PR opened → status = "In Progress" (via custom workflow)

**Limitations relevant to Stapler Squad**:
1. **Custom field changes don't appear in issue timeline**: Status changes live at the project level, not in the issue's audit log. Workaround is webhooks + GitHub Actions to post comments on the issue.
2. **No enforcement of transitions**: Any field can be set to any value at any time — there are no transition guards (unlike Linear's Triage gate or Jira's workflow conditions).
3. **No built-in human checkpoint for project item state**: Automation fires on events (close, merge) but there's no "require human approval before moving to Done" primitive.

**Architecture insight**: The decoupling of issue content from project metadata is worth copying — a `BacklogItem` row stores content (title, description, AC, labels) while a separate `SessionItemLink` stores per-session metadata (verdict, attempt number, duration). This prevents content churn from corrupting the audit trail.

---

### Plane.so

**What it is**: Open-source (AGPL-3.0) alternative to Jira/Linear/Monday, self-hostable, built with Django backend and React frontend.

**State model**: Work items have customizable states grouped into five fixed categories: `Backlog`, `Unstarted`, `Started`, `Completed`, `Cancelled`. Unlike Linear, Plane allows custom state names within categories. Modules (Epics) and Cycles (Sprints) are first-class concepts.

**Architecture**:
- Django REST API backend
- React SPA frontend
- PostgreSQL primary store
- Celery for async task processing (syncs, webhooks, notifications)
- Redis for cache and Celery broker

**Relevant patterns**:
1. **Intake (Triage equivalent)**: Plane's "Intake" feature gates external issue submissions (from forms, integrations) before they enter the project backlog — same mandatory review gate as Linear Triage
2. **Cycle/Module separation**: Cycles are time-boxed commitments; Modules are theme-based groupings. This maps to a potential Stapler Squad concept of a "sprint" (a prioritized ordered slice of the backlog) vs "epic" (a group of related items)
3. **Custom state transitions with no enforcement**: Same limitation as GitHub Projects — no transition guards in the community edition; workflow conditions are an enterprise feature

**Open-source insight**: Plane's Celery-based async processing for syncs (equivalent to GitHub Issues sync in Stapler Squad) is a reasonable pattern but may be over-engineered for a single-user SQLite-based system. A simpler polling loop with jitter is sufficient for MVP.

---

### SWE-bench / SWE-agent

**What it is**: A research benchmark (SWE-bench) and accompanying agent framework (SWE-agent) for evaluating AI coding agents on real GitHub issues from production repositories.

**Task representation**: Each benchmark instance is a structured record: repository, issue description, codebase snapshot (git commit), test suite, and a human-verified patch. Issues are "fully specified" — augmented with additional context, requirements, and interface documentation to ensure resolvability.

**Human-in-the-loop checkpoints** (SWE-bench Pro, 2025):
1. **Manual environment construction**: humans set up reproducible test environments per issue
2. **Human augmentation of issue description**: vague GitHub issues are enriched with explicit requirements, interface constraints, and acceptance criteria — the "ready" gate
3. **Human verification of tests**: humans validate that included tests are relevant and non-flaky before the agent sees them

**Context injection mechanism**: The task description passed to the agent is the human-augmented problem statement, not the raw GitHub issue. This is the key finding: raw issues are insufficient; structured enrichment is required before agent execution.

**Agent architecture**: SWE-agent uses an Agent-Computer Interface (ACI) — a structured shell environment with specialized tools (file viewer with syntax highlighting, line editor, test runner). Agents interact only through this controlled interface, not raw bash.

**Performance ceiling**: Even GPT-5 achieves only ~23% pass@1 on SWE-bench Pro's long-horizon tasks. This validates the requirements' emphasis on human review gates — autonomous completion is unreliable for complex tasks.

**Key insight**: SWE-bench's "human augmentation before agent execution" is exactly the `idea→ready` transition — a human must enrich the vague idea with AC before spawning a session. The agent-facing context (structured task description) is not the same as the user-facing backlog item (raw notes/ideas).

---

### AI Agent Orchestration Frameworks (LangGraph, CrewAI, AutoGen)

#### LangGraph

**State model**: Directed graph where nodes are functions/agents and edges are conditional transitions. **State is typed and persisted at every node** via a pluggable checkpointer (SQLite, Postgres, Redis).

**Human-in-the-loop pattern**: The `interrupt()` function is LangGraph's canonical HITL primitive:
```python
# Inside a node:
human_response = interrupt({"question": "Approve this action?", "details": action_details})
# Graph execution suspends here; state is checkpointed

# External caller resumes:
graph.invoke(Command(resume={"approved": True}), config={"thread_id": thread_id})
```

**Why it works**: The entire graph state snapshot is persisted before the interrupt, so the graph can be resumed from an arbitrary future point — even across process restarts. This is fundamentally different from async callbacks because state recovery is guaranteed.

**Failure modes**:
- Without a checkpointer, interrupts fail at compile time
- Time-travel (resuming from an earlier checkpoint) can create inconsistency if external side effects (API calls) are not idempotent
- Long-running suspended graphs accumulate storage cost

#### CrewAI

**State model**: Sequential task output chaining — each task's output is automatically passed as context to the next task in the crew. Crews can run in `sequential` or `hierarchical` process modes.

**Context passing**: Tasks specify a `context` parameter listing other tasks whose outputs they depend on. Inputs are passed at `kickoff(inputs={...})`.

**Human-in-the-loop**: No native interrupt support. Human interaction requires a custom `HumanInputTool` that blocks the agent's tool loop waiting for stdin input — fragile in async/multi-agent contexts.

**Failure modes**:
- `kickoff(inputs=...)` not reliably passed to all agents (documented bug in community forums)
- Agents going into tool-call loops during execution with no timeout
- Context from early tasks dilutes when crews have many sequential tasks (context window pressure)
- CrewAI Flows (2025 addition) improve state management but are a second abstraction layer on top of Crews, adding complexity

#### AutoGen (Microsoft)

**State model**: Conversation-based — agents exchange messages in a shared "conversation thread." State is implicit in the message history.

**Human-in-the-loop**: `HumanProxyAgent` pattern — a special agent that routes messages to a human operator and blocks until a reply is received. Works for single-session interactive use but poorly suited to async/web contexts.

**Failure modes**:
- Message history grows unboundedly; no built-in compaction
- No explicit task dependency model — all coordination is through conversational negotiation (agents can talk past each other)
- The proxy pattern breaks in distributed/async deployments where stdin is unavailable

**Overall framework conclusions**:
- LangGraph is the only framework with production-grade HITL primitives (checkpoint-based interrupt/resume)
- All frameworks share the problem that task state is too tightly coupled to conversation history
- None natively model a "backlog item" that persists across multiple agent sessions/attempts

---

## State Models Worth Borrowing

### 1. Linear's 5-Category State Machine with Hard Gates

```
idea → ready → in_progress → review → done
              ↑ gate: AC required    ↑ gate: verdict required or override
              
→ archived (from any state)
```

- `idea→ready`: human must add non-empty AC (enforced server-side, not just UI)
- `in_progress→review`: triggered automatically by session completion hook (not manual)
- `review→done`: requires gate verdict of PASS, or human override with reason
- Any state can transition to `archived` (soft delete)

**Why it works**: Gates prevent status theater — items can't drift forward without meeting criteria. Automation handles the happy path; humans only need to intervene at exception points.

### 2. GitHub Projects v2 Content/Metadata Separation

Separate the **item** (immutable content: title, description, AC, source) from **per-attempt metadata** (session link, verdict, duration). Multiple session attempts against the same item don't pollute the item's history — they accumulate as audit records on the `SessionItemLink` join table.

### 3. Beads' DAG Dependency Model (simplified)

For MVP, a simple `blocks`/`blocked_by` many-to-many relationship on `BacklogItem` suffices. This enables:
- "Ready work" detection: items with no unresolved blocking items and status = `ready`
- Future sprint planning: a cycle can pull all ready items with no blocking dependencies

Don't adopt Beads' Dolt/Git backing store — SQLite via ent ORM is already the project's persistence layer.

### 4. LangGraph's Checkpoint-Before-Interrupt Pattern

The review gate and agent approval request should be modeled as:
1. Persist state snapshot (item transitions to `review`, verdict record created as PENDING)
2. Send notification to human
3. Await human response (the notification contains an action link, not a blocking call)
4. On human response, apply transition (→ `done` or → `in_progress` for rework)

This is not LangGraph itself — it's the conceptual pattern: **always persist before suspending, always resume from persisted state**.

---

## Context Injection Patterns

### Pattern 1: CLAUDE.md Prepend (Recommended for MVP)

When spawning a session from a backlog item, generate a temporary file and prepend it to the session's CLAUDE.md or inject it as session instructions.

```markdown
# Backlog Item Context

**Item**: BL-042 — Refactor session storage to use ent ORM
**Priority**: High (P1)
**Status**: in_progress

## Why This Matters
The current boltdb storage is a bottleneck preventing schema evolution. 
This work unblocks the notification system (BL-043).

## Acceptance Criteria
- [ ] All session CRUD operations use ent ORM
- [ ] No boltdb imports remain in session/ package
- [ ] Existing tests pass without modification
- [ ] Migration script handles existing data

## Notes
- ADR-007 documents the ent decision
- The `session/ent/schema/` directory has the existing schema as reference
- Do NOT touch `config/` package in this work

## MCP Tools Available
- `report_progress(item_id, criteria_index, status, note)` — update AC checkboxes
- `request_review(item_id, message)` — pause and notify the human
- `get_backlog_item(item_id)` — retrieve this item's full context
```

**Advantages**: Works with existing session infrastructure; agents read CLAUDE.md automatically on session start and after compaction; no new protocol required.

**Limitation**: As sessions grow, CLAUDE.md instructions lose relative context weight (the Beads problem). Mitigate with MCP `get_backlog_item` as a re-orientation tool.

### Pattern 2: MCP Resource Pull (Complementary)

Expose the backlog item as an MCP resource (`backlog://items/{id}`). The agent can pull it on demand when re-orienting. This complements Pattern 1 rather than replacing it:
- CLAUDE.md injection: loaded automatically, ensures agent starts with context
- MCP resource: available on-demand for re-orientation mid-session

### Pattern 3: Context File in Worktree (Anti-Pattern — avoid)

Placing a `.backlog-context.md` file in the worktree directory creates a permanent artifact in the git history, pollutes the diff, and requires cleanup logic. Avoid for context injection; use only for item-specific notes that should persist across multiple sessions.

### SWE-bench Lesson Applied

SWE-bench's human augmentation step maps to the `idea→ready` gate: the triage agent (or the human manually) must enrich the vague idea with concrete AC, notes, and scope boundaries before the item is marked `ready`. The agent-facing CLAUDE.md injection should contain the enriched version, not the raw user notes.

---

## Human-in-the-Loop Patterns

### Pattern 1: Pre-Spawn Approval Gate (Spawn Gate)

Before a session is created from a backlog item, require explicit human approval (or "spawn from item" is the approval gesture). Never auto-spawn without a human click/confirmation.

**State**: `ready → in_progress` only via explicit user action (spawn button or CLI command)  
**Rationale**: Gastown's failure — agents starting work on wrong items — stems from fully autonomous dispatch. The spawn gate is the highest-value HITL checkpoint.

### Pattern 2: Agent-Initiated Review Request (Async Pause)

When an agent hits a blocker or completes work, it calls `request_review(item_id, message)` via MCP. The system:
1. Creates a `PendingReview` record with the agent's message
2. Sends a notification (existing Stapler Squad notification infrastructure)
3. The human sees the notification, clicks through to the session, reviews, and responds
4. The system updates the item state based on the response

**Key property**: The agent is not blocked waiting for stdin. It can either terminate (if it has completed its work) or continue with other subtasks while waiting. The MCP tool is fire-and-notify, not fire-and-block.

**Anti-pattern to avoid**: The AutoGen `HumanProxyAgent` pattern where the agent blocks on a synchronous human response. This creates stuck sessions and requires timeout logic.

### Pattern 3: Post-Session Review Gate (Completion Gate)

On session completion (detected via existing session lifecycle hooks), the gate automatically:
1. Runs `git diff base_branch..HEAD` in the worktree
2. Passes the diff + item AC to an LLM reviewer call (short-lived Go goroutine, not a full agent session)
3. Produces per-AC-item verdict: `PASS | FAIL | UNVERIFIABLE`
4. Stores verdict on `SessionItemLink`
5. Notifies human with verdict summary

**Executor choice**: A Go service calling the Anthropic API directly (not a full Claude Code session) is the right approach for the gate. It's cheaper, faster, and avoids the overhead of session creation/teardown for what is effectively a structured evaluation call.

**Rationale**: Full agent sessions for review (as Gastown's Witness does) are expensive and slow. A direct API call with a focused prompt evaluating a diff against structured criteria is more reliable and predictable.

### Pattern 4: LangGraph-Style Checkpoint Before Suspend

When the review gate produces a non-PASS verdict:
1. Persist the verdict to `SessionItemLink.verdict`
2. Transition item to `review` status
3. Notify human
4. Await human override action

The item stays in `review` until the human acts. The system does not auto-transition to `done` or `in_progress`. This matches LangGraph's "persist checkpoint, then await `Command(resume=...)`" pattern — without requiring LangGraph itself.

---

## What NOT to Copy

### 1. Gastown's Fully Autonomous Drain
**Problem**: No human approval before agents start work leads to runaway costs, wrong-item execution, and impossible-to-audit state.  
**Don't build**: A "drain queue" that automatically picks the highest-priority ready item and spawns a session. Always require an explicit human spawn action for MVP.

### 2. Beads' Reliance on Agent Compliance for State Updates
**Problem**: Beads requires agents to query and update it via `bd sync`. Agents stop doing this as context grows. State goes stale.  
**Don't build**: A system where the only mechanism for updating task state is agent-initiated MCP calls. The session completion hook (server-side) must transition item status even if the agent never called `report_progress`.

### 3. CrewAI's Sequential Context Chaining for Long Sessions
**Problem**: Passing the full output of task N as context to task N+1 creates unbounded context growth. Agents in long chains lose focus on the original goal.  
**Don't build**: A system that injects the entire session transcript as context for a review pass. The review gate should receive only the structured diff + AC list, not the full conversation.

### 4. AutoGen's Blocking HumanProxy for Review
**Problem**: Blocking the agent process on a synchronous human response is fragile — sessions time out, processes die, and the agent can't do other work while waiting.  
**Don't build**: Any pattern where agent execution is paused waiting for a human response via stdin/polling. Use fire-and-notify + async state transition instead.

### 5. GitHub Projects v2's Unguarded State Transitions
**Problem**: Any field can be set to any value at any time with no transition validation. Status becomes meaningless theater (items marked Done without any output, items in In Progress with no session).  
**Don't build**: A system where the UI allows free-form status changes without enforcing the state machine. All `→done` transitions must go through the gate or explicit override. Enforce this server-side in the ConnectRPC handler.

### 6. Linear's Cycle/Sprint Complexity for MVP
**Problem**: Linear's cycle model (separate from backlog, with committed vs. candidate items) adds UI and data model complexity that is unnecessary for single-user MVP.  
**Don't build**: Cycles, sprints, velocity tracking, or burndown charts. The backlog itself with priority ordering is sufficient. Leave room in the data model (a nullable `CycleID` field on `BacklogItem`) for future addition.

### 7. Plane's Celery-Based Async Processing
**Problem**: Celery (or any distributed task queue) is over-engineered for a single-user, single-process Go server with SQLite.  
**Don't build**: A separate worker process for GitHub sync or notification dispatch. Use goroutines with a simple in-process scheduler (ticker-based) for the MVP sync loop. Upgrade to a persistent queue only if multi-user/multi-instance becomes a requirement.

### 8. Beads' Dolt/Git-as-Database
**Problem**: Using Git or Dolt as a database for task state adds branching/merge complexity that is unnecessary when ent/SQLite is already the project's persistence layer.  
**Don't build**: A file-based or Git-backed task store alongside the existing ent schema. All backlog state goes in SQLite via ent following existing patterns.

---

## Open Questions — Recommended Answers Based on Research

**Q1: Should the triage agent be a persistent background session, on-demand, or event-driven?**  
**Recommendation: On-demand (user-triggered), not persistent.** A persistent background session burns API cost continuously (Gastown's $100/hour problem) and has no work to do between user interactions. Trigger the triage agent when the user clicks "Help me refine this item" — spawn a short-lived Claude Code session with the item context injected, let it output suggestions, then terminate. The session is disposable.

**Q2: How does item context reach the spawned session?**  
**Recommendation: Pattern 1 (CLAUDE.md prepend) as primary, Pattern 2 (MCP resource) as supplemental.** Generate the context block at spawn time, write it to the session's working directory as `.stapler-backlog-context.md`, and reference it from CLAUDE.md via `@.stapler-backlog-context.md`. This survives compaction because CLAUDE.md re-reads the file. The MCP `get_backlog_item` tool provides re-orientation on demand.

**Q3: Who runs the review gate — dedicated agent session or Go service calling LLM API directly?**  
**Recommendation: Go service calling LLM API directly.** Agent sessions have creation/teardown overhead, require tmux management, and are expensive for what is a single structured evaluation call. A Go function that constructs a prompt (diff + AC list), calls the Anthropic API with a structured output schema, and parses the response is faster, cheaper, and more reliable. Cap token budget per review call.

**Q4: How do we detect silent agent drift without reading every terminal line?**  
**Recommendation: Git diff polling on a timer + MCP heartbeat.** Every N minutes (configurable, default 5), check `git diff HEAD` in the session's worktree. If no changes have been committed in the last M minutes (configurable, default 30), surface a "possibly stuck" indicator in the UI. This is passive and cheap. Complement with an MCP `report_progress` call from the agent as an optional active heartbeat. Do NOT attempt to parse terminal output for drift detection — too brittle.

---

*Research complete. Key sources available in the Sources section.*
