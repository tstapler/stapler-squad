# Research: Feature Landscape — backlog-agent-communication

**Date**: 2026-07-23

## Full Current MCP Tool Inventory (backlog-relevant + adjacent)

From `server/mcp/tools_backlog.go`, `tools_goal.go`, `tools_lifecycle.go`,
`tools_vcs.go`, `tools_github.go`, `tools_terminal.go` (`s.AddTool` call sites):

| Tool | File | Direction | What it captures |
|---|---|---|---|
| `get_backlog_item` | tools_backlog.go | orchestrator → agent (read) | item fields incl. latest review verdict |
| `report_progress` | tools_backlog.go | agent → orchestrator | per-criterion status + freeform note |
| `request_review` | tools_backlog.go | agent → orchestrator | message, verification_notes (freeform) |
| `submit_review_verdict` | tools_backlog.go | agent → orchestrator | per-criterion outcome+evidence, summary |
| `submit_triage_result` | tools_backlog.go | agent → orchestrator | AC criteria, plan artifacts path |
| `set_session_goal` / task tree | tools_goal.go | agent → orchestrator (self-report) | goal text, status (idle/working/blocked/done), task tree |
| `get_session_goal` | tools_goal.go | orchestrator → agent (read) | — |
| lifecycle tools (5) | tools_lifecycle.go | mixed | session-level control, not backlog-item-scoped |
| vcs tools (2) | tools_vcs.go | agent → read | git status/diff |
| github tools (2) | tools_github.go | agent → read | PR listing |

Every backlog-scoped write tool is **status-report shaped**: an agent tells the
orchestrator "here is my current state," never "here is context for the *next*
agent" or "something outside my assigned scope is wrong." This confirms the
requirements.md problem framing directly from the code, not just the audit doc.

## Comparable Patterns (industry / prior art for multi-agent handoff & escalation)

- **Blackboard architecture** (classic AI multi-agent pattern): a shared,
  structured data store multiple specialist agents read/write incrementally,
  rather than point-to-point messages. `ItemSession`/`BacklogProgressNote`/
  `ReviewVerdictData` already form a de facto blackboard — the gap is *richness* of
  what's written, not the pattern itself. Recommendation for planning: extend the
  blackboard (add structured fields) rather than introduce point-to-point agent
  messaging, which would be a bigger architectural departure than this project's
  scope calls for.
- **Escalation/circuit-breaker pattern** (SRE/ops tooling: PagerDuty, Opsgenie):
  separates "an alert fired" (detection) from "someone acknowledged / is paging a
  human" (escalation) from "someone resolved it" (resolution), each with its own
  durable state and timestamp. `BacklogStuckState` already models
  detect→(attempt×N)→park, but has no "acknowledge" state and no
  agent-vs-reconciler-initiated distinction — see architecture.md's point on this.
  A minimal escalation-pattern addition: an explicit `acknowledged_at`/
  `acknowledged_by` on the stuck-state row, so a human dismissing a notification is
  distinguishable from it simply timing out.
- **Code review tooling conventions** (GitHub/GitLab suggested-changes, this repo's
  own `code-review` skill): structured severity taxonomy
  (`[BLOCKER]/[CRITICAL]/[MAJOR]/[NIT]`) + file:line anchors + a distinct "reviewer
  disagrees, requests changes" vs. "author disputes the review" state (GitHub's
  own "Resolve conversation" / re-request-review flow is the closest real-world
  analogue to pain point B). Re-review-by-fresh-reviewer with the dispute as
  context is a well-established human-code-review pattern worth adopting directly.
- **Support/ticketing escalation tiers** (Tier 1 → Tier 2 → human): relevant to the
  "Master agent" question — most production support-automation systems that use an
  LLM "first responder" escalate to a *human*, not a bigger LLM, once genuinely
  stuck; a "Master agent" that itself gets stuck has no further escalation target,
  so any Master-agent design must still terminate at a human path as the true
  backstop, matching requirements.md's dimension 4 constraint.

## Edge Cases and Failure Modes to Design Against

- **Dispute-loop abuse**: an implementer agent could dispute every FAIL verdict
  reflexively (its own past failure incentivizes "the reviewer is wrong"). Any
  dispute mechanism needs either a cap (mirrors `MaxRemediationAttempts`) or
  mandatory fresh-reviewer/human adjudication rather than self-adjudication.
- **Escalation spam / alert fatigue**: if "ask for help" is too easy to trigger
  (e.g. any transient tool error), the sole human operator will rationally start
  ignoring it, defeating the purpose (direct parallel to `MaxRemediationAttempts`'s
  existing rationale for backoff — the same discipline should apply to
  human-facing escalation volume, not just automated-retry volume).
- **Escalation without content**: a bare "I'm stuck" with no structured "what did
  you try, what do you need" is nearly as unhelpful as no escalation at all — the
  tool schema itself should require (not just encourage) a reason + attempted
  remediation summary, mirroring `request_review`'s existing
  strongly-encouraged-but-optional `verification_notes` pattern (which BUG-040/
  BUG-045-adjacent findings suggest is under-used when merely optional).
- **Infra-bug reports competing with item-specific stuck rows**: dimension 2
  reports are about the *system*, not one item — they must not silently attach to
  a single `item_id` the way `BacklogStuckState` rows do today, or a genuinely
  systemic issue (e.g. "the reconciler is stuck") gets buried as a footnote on
  whichever item happened to be running when it was noticed.
- **PR visibility timing** (pain point A): PR metadata capture must survive the
  worktree/session lifecycle — BUG-040 and BUG-045 both show the failure mode is
  specifically *state that outlives the code that produced it going stale or
  disappearing* (a worktree reaped, a cached `PrNumber` referencing a since-closed
  PR). Any redesign of PR-metadata capture must treat "the item record is the
  durable source of truth, independent of worktree/session liveness" as a hard
  invariant, not an incidental property.
- **Reviewer identity for re-review** (pain point B): if disputes are adjudicated
  by "a fresh re-reviewer," that reviewer needs the *same* codebase-read
  correctness BUG-045 is about — a dispute re-review inherits that open bug's risk
  surface and should not be designed as if BUG-045 is unrelated.

## Unstated User Needs (beyond the literal requirements)

- **Auditability**: the operator will want to look back at *why* an agent
  escalated or disputed something after the fact, not just see it resolved — every
  new mechanism should write an immutable record (mirrors
  `BacklogProgressNote`'s append-only design), not just a mutable status flag.
- **Low cognitive overhead for the human**: given this is a solo operator running
  many concurrent items (WIP-limited per `feedback_backlog_wip_limit.md` memory:
  cap of 2), new human-facing surfaces must be skimmable in the existing
  `/unfinished` triage flow, not a new place to separately check.
- **Composability with future multi-agent growth**: the operator's own "Master
  agent" phrasing signals they're thinking ahead to more agent-to-agent
  interaction than exists today — even if this plan doesn't build a Master agent
  now, the data model chosen for dimension 1 (structured handoff) should not
  preclude a future consumer that isn't a human (i.e. structured, machine-parseable
  fields, not just richer freeform text).
