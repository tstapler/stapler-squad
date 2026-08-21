# Requirements: backlog-workflow-engine

**Date**: 2026-05-19
**Bounded Context**: Backlog / project management
**Type**: Feature addition (to existing backlog service)
**Jira**: —

---

## Problem Statement

The backlog feature has two related gaps:

1. **No AI refinement loop state.** When a new item is vague, triage fires but there's no
   dedicated state to represent "the AI is iterating with the user to flesh this out." Items
   either stay in `idea` (no visual feedback) or jump straight to `ready` (skipping the loop).

2. **Hardcoded status enum.** Every team's workflow is different. The current
   `idea → ready → in_progress → review → done → archived` progression is hardcoded across
   proto, Go, and React. Adding or renaming a state requires coordinated changes in 8+ files.
   Users have no way to model their actual process (e.g. `design → spec → dev → qa → ship`).

These compound: even if we add `refining` today, the same problem recurs for the next custom
state anyone wants.

---

## Users / Consumers

- **Primary**: Solo operators managing their own backlog (most common current usage)
- **Secondary**: Small teams (2–5 people) sharing a workspace
- **Tertiary**: AI agents (triage, execution sessions) that trigger transitions and may be
  blocked by gates

---

## Success Metrics

All three of the following must be true for this project to be complete:

1. **`refining` state ships and works end-to-end** — triage AI can loop on clarifying
   questions while an item sits in `refining`; item advances to `idea` or `ready` when the AI
   is satisfied or the user manually advances it.

2. **Users can define custom states** — add, rename, reorder, and delete states via API and
   UI; the default workflow ships out of the box and is editable.

3. **Gates are configurable on transitions** — users can attach one or more gate conditions
   to any transition and toggle them on/off without code changes.

---

## Constraints

- No deadline
- Backward-compatible: all existing items in `idea / ready / in_progress / review / done /
  archived` must work without a data migration or manual intervention
- The default workflow must be identical to the current hardcoded one so existing users see no
  change until they opt in to customization

---

## Scope

### In Scope

**S1 — `refining` status (built on the new engine)**
- New lifecycle state between `idea` and `ready`
- Entered automatically when triage posts clarifying questions
- Triage AI can loop: post question → user answers → follow-up question → stays in `refining`
- User can manually advance to `idea` (abandon refinement) or `ready` (skip remaining questions)
- Triage transitions item to `idea` (with populated AC) when satisfied

**S2 — WorkflowConfig data model**
- DB-persisted graph: `states[]`, `transitions[]`, `gates[]`
- One `WorkflowConfig` per workspace (scoped to instance, not global)
- Ships with a built-in default config matching the current hardcoded workflow
- Proto message + ent schema + Go service layer

**S3 — Custom state CRUD**
- API: create, update, delete, reorder states
- Validation: no orphaned transitions when a state is deleted; at least one state must remain
- UI: settings page for managing states (list + inline edit)

**S4 — Workflow builder UI**
- Visual graph editor: states as nodes, transitions as directed edges
- Drag to reorder, click to add/remove transitions
- Gate configuration panel per transition (add/remove/toggle gates)

**S5 — Gate types**

| Gate | Description |
|------|-------------|
| **Field gate** | A named item field must be non-empty (e.g. description, AC count ≥ N) |
| **Triage gate** | Triage session must have completed successfully |
| **Approval gate** | A human must click an explicit "Approve" button in the UI |
| **Custom condition** | User-defined expression (e.g. `ac_count >= 2`, `description_length >= 80`) |
| **Command gate** | A shell command must exit 0 (e.g. `make test`, `npm run lint`) |
| **CI gate** | All required GitHub PR checks must be green for the linked PR |

Gates are per-transition, can be toggled enabled/disabled individually, and support an
`enforcement` mode: `blocking` (hard stop) vs `warning` (advisory, user can override).

---

## Out of Scope

No explicit exclusions were called out. The following are implicitly out of scope for this
project and should be treated as follow-on work:

- **Automation triggers**: auto-advancing items when a condition becomes true (e.g. "when CI
  goes green, auto-transition to done") — this is a separate event-driven rules engine
- **Cross-item dependencies**: "blocked by item Y" — separate dependency graph feature
- **Multi-workflow support**: different workflows per project/label — single workspace-level
  config only for now
- **Webhook gates**: calling an external HTTP endpoint as a gate — out for this iteration

---

## Open Questions

1. **Migration strategy**: How should existing items whose current `status` value doesn't
   exist in a customized workflow graph be handled? (Likely: preserve the raw string value,
   surface a warning in the UI)

2. **Command gate execution context**: Where does the command gate run — on the server (risky,
   needs sandboxing) or triggered client-side? Should it be scoped to the item's worktree?

3. **CI gate integration**: Does this require GitHub App auth, or can it reuse an existing
   token? What's the polling/webhook model?

4. **Conflict resolution**: If two users transition the same item simultaneously, which wins?
   (Likely: last-write-wins with optimistic locking, consistent with current behavior)

5. **Default workflow editability**: Should the default built-in workflow be editable, or
   should users create a copy? (Recommendation: editable in place with a "reset to default"
   escape hatch)
