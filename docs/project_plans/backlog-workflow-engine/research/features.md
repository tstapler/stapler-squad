# Feature Research: backlog-workflow-engine

## Scope

Research across five dimensions:
1. Industry workflow engine patterns (Linear, GitHub Projects, Jira, Trello)
2. AI refinement loop UX
3. Gate UX patterns (blocking vs warning)
4. Workflow builder UI patterns
5. Edge cases and unstated user needs

---

## 1. Industry Workflow Engine Patterns

### Linear: Opinionated Simplicity

Linear's workflow model is the clearest reference for this feature.

**Structure**: Statuses are grouped into fixed *categories* (Backlog, Todo, In Progress, Done, Canceled). Custom statuses live *within* a category — they don't escape the category's semantic meaning. You can have multiple "In Progress" statuses (e.g. "Waiting for Review", "Blocked"), but they all roll up to the In Progress phase for reporting purposes.

**Configuration surface**: Settings → Teams → Issue statuses. Flat list with drag-to-reorder within category. Single-line edit: name, color, description. No graph visualization.

**Key design insight**: Categories enforce a semantic backbone that survives customization. Users get flexibility without losing the core meaning of "not started / in flight / done". This prevents the Jira anti-pattern of 40 custom statuses that all mean slightly different things.

**Limitations**: Fixed categories create friction for workflows that don't fit Linear's model. A "refining" state (AI clarification loop) doesn't clearly map to Backlog or Todo. Linear's model would call it a Backlog sub-status; the semantic is wrong but the mechanic works.

### Jira: Full Flexibility at a Complexity Cost

**Structure**: Every status belongs to one of three global categories (To Do, In Progress, Done). Unlike Linear, categories are globally shared across all workflows. You can create unlimited custom statuses. The workflow designer is a full visual graph editor.

**Configuration surface**: Workflow designer with graphical state-transition diagram. Transitions are explicit directed edges, each with optional conditions (pre-conditions), validators (post-conditions), and post-functions (actions on transition). These map directly to the "gate" concept in the requirements.

**Jira's gate taxonomy** (directly relevant):
- **Conditions**: check whether the user can perform the transition (permission check, field value check, group membership). Equivalent to *blocking gates*.
- **Validators**: check that required information exists before transition completes. Equivalent to *field gates*.
- **Post-functions**: actions triggered on successful transition (auto-assign, update field, notify). Out of scope here but adjacent to S2.

**Key design insight**: Jira's complexity comes from trying to be a general-purpose workflow engine for arbitrary business processes. For a developer backlog tool, 80% of Jira's capabilities are overhead. The valuable 20%: named transitions (not just "move from A to B"), per-transition gate config, and blocking vs non-blocking enforcement.

**Anti-pattern to avoid**: Jira's "you can only delete statuses from inactive workflows" rule creates maintenance hell. The migration modal pattern (force user to pick a destination status for existing items) is better.

### GitHub Projects: Minimal, Field-Centric

GitHub Projects treats status as just another custom field — it's a single-select with user-defined options. No graph, no transition rules, no gates. The automation layer (built-in workflows + GitHub Actions) can update status on events (PR merged → Done), but there's no concept of blocked transitions.

**Key insight for solo users**: GitHub's approach reveals what solo developers actually need at baseline — a simple ordered list of states with optional automation triggers. The overhead of a full workflow graph is often not worth it for one-person projects. This suggests S4 (visual builder) should be behind a progressive disclosure mechanism, not in the primary path.

### Trello: Labels as Workflow Proxy

Trello has no formal status concept — columns are the state machine. Moving a card between columns is a transition. No gates, no conditions. This is relevant because it shows how far users will go to *avoid* formal workflow configuration: they'd rather drag cards than configure a state machine.

**Insight**: The visual metaphor of "cards moving between columns" is deeply intuitive. If the workflow builder UI can leverage this mental model (states as columns, transitions as valid moves), it will feel familiar without requiring new concepts.

### What Solo Users Actually Need vs Teams

Solo users:
- Want 3-6 states, not 15
- Need quick keyboard-driven status changes (don't want to open a modal)
- Rarely need approval gates — they're both the requester and approver
- Most valuable gate type: **field gate** (did I fill in the AC criteria? did I approve the plan?) — already implemented in `TransitionGuard`
- Audit log is secondary — they remember what they did
- Templates are high-value: "start with a developer backlog workflow" removes the blank-canvas problem

Small teams (2-5):
- Approval gates become valuable (someone else must approve before progressing)
- Audit trail becomes essential — who moved this item and when?
- Bulk status changes become necessary (sprint planning: move all "ready" items to "in progress")
- Workflow templates become a team alignment tool

**Design implication**: Start with the solo user path. Gate types and visual builder are team-facing features that can be progressive-disclosed.

---

## 2. AI Refinement Loop UX

### The Problem with `idea → ready` as a Hard Jump

The current model (idea → ready with AC gate) works for items where the user already knows what they want. It fails when items need elaboration — the user has a vague idea and needs the AI to help crystallize it into AC criteria. The proposed `refining` state fills this gap.

### Patterns Observed in the Wild

**Chat-based (Steve, Linear Asks, Notion AI Chat)**:
- A side panel or full-screen chat interface opens within the item context
- The AI asks clarifying questions; the user responds; the AI suggests updated AC criteria, description, or scope
- Strengths: natural, exploratory, handles ambiguous requirements well
- Weaknesses: the conversation history grows but doesn't produce a structured artifact unless explicitly requested; users often lose track of what was decided

**Inline editing with AI suggestions (Notion AI, GitHub Copilot)**:
- AI suggestions appear inline, adjacent to the field being written
- Ghost text or diff-style presentation (added/removed words highlighted)
- Users accept, reject, or edit suggestions in place
- Strengths: keeps the user in the editing context, low cognitive overhead
- Weaknesses: works well for prose fields (description), poorly for structured fields (AC criteria list)

**Q&A form (Active Discovery, some Jira AI plugins)**:
- AI generates a structured set of questions: "What is the user?", "What is the trigger?", "What is the outcome?", "What are the constraints?"
- User fills in answers, AI synthesizes into requirements
- Strengths: produces consistent, structured output; good for users who don't know where to start
- Weaknesses: feels mechanical, loses nuance, frustrating when questions don't match the problem

### Recommended Pattern for `refining` State

The best fit for stapler-squad's context is a **hybrid**: an in-item chat panel that persists while the item is in `refining`, with the AI proactively suggesting AC criteria edits as structured outputs.

Key design decisions:
- The chat transcript should be stored on the item (not ephemeral) so a triage session that later picks up the item has full context
- AI suggestions for AC criteria should appear as "proposed additions" with accept/reject controls — not auto-applied
- The `refining → ready` transition gate should check that at least one AI-suggested AC was accepted *or* the user manually wrote AC criteria (not just that `refining` was visited)
- Visual indicator on the item card when in `refining` should distinguish it from `idea` (different color/icon) to communicate "AI is engaged" vs "not started"

**Anti-pattern**: Notion AI's "improve writing" feature applied to backlog items is too open-ended. The clarification loop needs to be *goal-directed*: the objective is AC criteria + scope clarity, not better prose.

---

## 3. Gate UX Patterns

### Blocking Gate: The Disabled Button Problem

A blocking gate (must pass before transition is allowed) maps directly to the current `TransitionGuard` pattern. The UI challenge: **disabled buttons are confusing without explanation**.

Research findings from UX studies (Smashing Magazine, Nielsen Norman):
- A disabled button with no explanation creates dead ends — users don't know if the feature is broken or if they're missing a step
- Best practice: show the button in disabled state with an `aria-disabled` + tooltip explaining the prerequisite ("Add at least one AC criterion first")
- Better: show an inline explanation below the button group when the gate is blocking ("Before marking ready: add acceptance criteria")
- Do NOT hide the button entirely — users need to see where they're going

The current implementation in `BacklogItemDetail.tsx` already does this correctly for `mark_ready`:
```tsx
disabled={actionLoading || item.acCriteria.length === 0}
title={item.acCriteria.length === 0 ? "Add at least one AC criterion first" : undefined}
```
This is the pattern to generalize for dynamic gate configurations.

### Warning Gate: Non-Blocking, But Visible

A warning gate (can transition but shouldn't without acknowledgment) needs a different treatment:
- Show the transition button as **fully enabled**
- Show a **yellow inline warning** adjacent to the button: "Recommended: resolve open comments before marking done"
- On click: show a **lightweight confirmation dialog** (not a full blocking modal) — "This item has 2 unresolved comments. Continue anyway?" with Yes/Cancel
- If user confirms, the transition proceeds. Log it to the audit trail.

**Key difference from blocking**: A warning can be dismissed in one extra click. A blocking gate requires the user to go elsewhere, complete the work, and return.

### Gate Status Indicator on the Item Card

For items where a gate is blocking, the item card in the list view should show a small badge or icon to communicate "action required before this can progress". Options:
- Yellow lock icon next to the status badge
- Red/yellow dot on the status pill
- Inline text below the title: "1 gate blocking"

Linear's equivalent: "blocked" status with a tooltip listing blockers. GitHub Issues: "draft" PR concept (not ready to merge). Both signal "not ready to advance" without burying the indicator.

### CI Gate (GitHub PR Checks)

A CI gate that reads GitHub PR check status is particularly complex:
- Polling latency: checks can take minutes; the UI should show "checking..." state, not block indefinitely
- Partial pass: some checks pass, some fail — show each check with its status (pass/fail/running)
- Re-check: provide a "refresh" button rather than auto-polling every N seconds (avoids surprising the user)
- When blocking: show the failing check names + links to the GitHub check run

---

## 4. Workflow Builder UI Patterns

### The Spectrum: List Editor to Full Graph

There are three distinct levels of workflow builder complexity:

**Level 1: Simple List Editor** (Linear's model)
- Ordered list of states within categories
- Click to rename, click + to add, drag to reorder, click × to delete
- No visual of transitions — the sequence is implied by the list order
- Suitable for: small teams, simple linear workflows

**Level 2: State Matrix / Transition Table** (Jira Classic simplified)
- Grid showing which transitions are allowed between which states
- Checkboxes or toggle switches in a matrix layout
- No drag-and-drop graph
- Suitable for: workflows with backtracking and non-linear paths

**Level 3: Visual Graph Editor** (Jira Modern, Retool, n8n)
- States as draggable nodes, transitions as directed edges
- Click on edge to configure gate conditions
- Suitable for: complex multi-team workflows, power users

### Recommendation for S4

For stapler-squad's scope (solo + small team, single workspace workflow), **Level 1** should be the default and primary interface. The requirements include a visual graph editor (S4), but research suggests this adds significant build complexity for marginal benefit to the target user.

**Pragmatic approach**:
- Ship Level 1 (list editor) with S3 (Custom state CRUD)
- S4 (visual builder) can be a later enhancement behind a "Advanced Workflow" toggle
- The graph view is more useful for understanding an existing workflow than for building one from scratch — consider a read-only graph visualization as a cheaper win

### What Makes a Workflow Builder Usable for Non-Technical Users

1. **Start with a template**: "Developer backlog" template (idea → refining → ready → in_progress → review → done → archived) eliminates the blank canvas problem. Users edit from a known good starting point.
2. **Guard rails, not blank canvas**: Require at least one state in each semantic category (not-started, in-progress, terminal). Prevent deletion of the last terminal state.
3. **Immediate feedback**: Preview the transition diagram as states and transitions are edited. Don't require a "save and publish" step to see changes.
4. **Escape hatch**: "Reset to defaults" button — non-destructive (moves items to nearest equivalent state) and one-click recoverable.
5. **No code required**: Gate conditions should be expressed as rule-builder UI (field = value, field is not empty) rather than CEL/JSONata expressions. Command gates are the only place where code is acceptable, and they need a test-run button.

---

## 5. Edge Cases and Unstated User Needs

### Edge Case: Deleting a State with Existing Items

**Pattern from Jira and Azure DevOps**: Block the delete action and show a migration modal.

UI flow:
1. User clicks delete on a state that has N items
2. System shows: "3 items are currently in 'Refining'. Move them to: [dropdown of remaining states] before deleting."
3. User picks destination state → confirm → items migrate atomically → state deleted
4. If user cancels, nothing happens

**Additional guard**: Never allow deletion of built-in terminal states (`archived`). Display them as un-deletable (no delete button, tooltip: "Built-in states cannot be removed").

**Rename is free**: Allow renaming any state including system states except the internal constant (the code `key`, not the display `label`). This is how Linear handles it — the `Done` category has a display name but a stable internal identifier.

### Edge Case: Malformed Custom Condition Expression

For custom condition gates (S5), expression evaluation can fail at author time or at runtime.

**Author-time validation**: Validate the expression when the user saves the gate config. Show inline error: "Expression syntax error at position 12: unexpected token 'AND'". Block save until fixed.

**Runtime evaluation failure**: An expression that was valid at save time may fail at runtime (field referenced no longer exists, etc.):
- Treat evaluation failure as **gate open** (non-blocking) to prevent items from getting permanently stuck
- Log the evaluation error to the item's audit trail: "Gate condition 'foo' could not be evaluated: field 'bar' not found"
- Show a yellow badge on the transition: "Gate condition failed to evaluate — transition allowed"

### Edge Case: Command Gate Timeout

Shell command gates need a timeout policy:
- Default timeout: 30 seconds (long enough for most CI-adjacent checks, short enough to not hang the UI)
- User-configurable per gate: min 5s, max 300s
- On timeout: treat as **gate failed** (blocking) — fail safe, do not auto-advance
- Show in the UI: "Command gate timed out after 30s" with the partial stdout/stderr output
- Provide a "retry" button so the user can re-trigger without navigating away

**Background execution model**: Command gates should execute asynchronously. The transition button should show a spinner state ("Checking gate..."), not block the browser. If the user navigates away, the check should complete in the background and update the item state.

### Edge Case: The `archived` Terminal State

The requirements note `archived` as a terminal state. Design decisions:
- `archived` must always exist and cannot be deleted (it is the escape hatch for soft-deleting items)
- `archived` should be editable in name/color only
- `archived → idea` should remain the only transition out of `archived` (already in `validTransitions`)
- Custom terminal states (e.g., "Won't Fix") should be possible, but they also need the `archived → idea` escape hatch or their own exit transition

### Unstated User Needs

Research across workflow system user communities consistently surfaces these requests:

**1. Per-item transition history / audit trail** (high value, low cost)
The most-requested feature in Linear's community. When debugging "why is this item stuck?", users want to see: `idea → ready (2026-05-10, Tyler) → in_progress (2026-05-11, triage:session-abc) → review (2026-05-12)`. This is a simple append-only log on the item, not a full audit system.

**2. Bulk status change** (high value, low cost)
Sprint planning workflow: select 5 items in `ready` → "Move to In Progress". Currently requires 5 individual transitions. The transition guard still runs per-item (gates must pass for each). Items that fail the guard are listed as skipped with the reason.

**3. Keyboard shortcuts for transitions** (high value for solo users, low cost)
Solo operators working through a backlog want speed. When viewing an item, pressing `R` should trigger "Mark Ready" if available. The current action buttons are already conditionally rendered by status — adding `data-hotkey` attributes or a keyboard shortcut handler is straightforward. Linear's `T` for "Todo", `I` for "In Progress" is the reference pattern.

**4. Workflow templates** (high value, reduces abandonment)
New users confronted with a blank workflow editor abandon it. Pre-built templates:
- "Developer Backlog" (the current hardcoded workflow + `refining`)
- "Bug Triage" (reported → triaging → confirmed → fixing → verifying → closed)
- "Content Pipeline" (draft → review → approved → published → archived)
Templates are read-only starting points that become editable copies.

**5. Transition reason / comment on manual override** (medium value)
When a user triggers a manual override (e.g., `override_done` to skip the review verdict gate), the system should prompt for a required comment: "Why are you overriding? [text field]". This comment is stored in the audit trail. Already partially implemented via `OverrideReason` field in `TransitionGuard`.

**6. "Stuck items" view** (medium value for teams)
A filtered view of items that have been in the same state for longer than a configurable threshold (e.g., 5 days in `refining`). Surfaces items that have fallen through the cracks. Lightweight: a query filter rather than a new data model.

**7. Gate toggle without deletion** (medium value)
Users want to temporarily disable a gate during a migration or exceptional situation without deleting its configuration. A simple enabled/disabled toggle on each gate avoids the "delete and recreate" cycle.

---

## Summary Table: Feature Priority vs Complexity

| Feature | User Value | Build Complexity | Recommendation |
|---|---|---|---|
| `refining` status (S1) | High | Low — add to enum + transition table | Ship in v1 |
| WorkflowConfig data model (S2) | High | Medium — new DB schema + CRUD | Ship in v1 |
| Custom state CRUD API + UI (S3) | High | Medium | Ship in v1 |
| Level 1 list editor (S4 lite) | High | Low | Ship in v1 |
| Per-item transition history | High | Low | Ship in v1 |
| Field gate + triage gate (S5) | High | Low — extends TransitionGuard | Ship in v1 |
| Bulk status change | Medium | Low | Ship in v1 |
| Keyboard shortcuts | High for solo | Low | Ship in v1 |
| Workflow templates | High for onboarding | Low | Ship in v1 |
| Approval gate (S5) | Medium | Medium | v2 |
| Command gate (S5) | Medium | High — async execution, timeout | v2 |
| CI gate / GitHub PR checks (S5) | Medium | High — GitHub API polling | v2 |
| Visual graph editor (S4 full) | Low for solo | High | v3 or skip |
| Custom condition expression gate (S5) | Low for solo | High — expression parser + runtime | v3 or skip |
| "Stuck items" view | Medium | Low | v2 |
| Gate toggle (enabled/disabled) | Medium | Low | v2 |

---

## Key Tensions to Resolve in Planning

**Tension 1: `refining` as first-class state vs custom state**
The requirements treat `refining` as a specific new state (S1), but if custom states (S3) are being built anyway, `refining` could just be the first custom state. The argument for first-class: `refining` has special AI-session semantics (it spawns a clarification session, not a work session). The argument against: premature hardcoding. Recommendation: implement `refining` as a built-in state with special session semantics, but expose its label/color as editable.

**Tension 2: WorkflowConfig persistence vs compile-time enum**
The requirements identify the root cause (hardcoded enum requires 8-file changes). The fix is DB-persisted WorkflowConfig. This means transition validation in `CanTransitionBacklog` and `TransitionGuard` must move from compile-time maps to runtime config lookups. This is a significant refactor of `session/backlog.go` and all callers.

**Tension 3: Visual builder scope**
S4 asks for a "visual graph editor". Research suggests this is the highest complexity, lowest immediate value item for the target user. Plan should explicitly descope or defer the full visual builder in favor of a list editor + read-only graph visualization.
