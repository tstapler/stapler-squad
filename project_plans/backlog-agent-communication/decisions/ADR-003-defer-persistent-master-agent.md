# ADR-003: Defer a Persistent "Master Agent"; Use headless.Pool as the Extension Seam

**Status**: Proposed
**Date**: 2026-07-23
**Context**: project_plans/backlog-agent-communication

## Context

The operator's brief asks what "a central Master agent" would concretely mean in
this architecture for the "ask for help" escalation path (dimension 3), and asks
this to be investigated/designed, not assumed. Two shapes were considered:

1. **A distinguished, always-on orchestrating session** — a persistent process
   (or persistent tmux/Claude Code session) that stays alive specifically to
   receive and triage escalations, separate from the human.
2. **On-demand, short-lived triage** — reuse `headless.Pool` (already used to spawn
   one-shot review sessions) to spawn a short-lived "look at this escalation and
   summarize/diagnose it for the human" session at the moment `request_help` is
   called, with no persistent process between escalations.
3. **No agent intermediary at all** — `request_help` goes straight to a human via
   `Notifier`/`/unfinished`, with no agent triage step.

## Decision

For this plan's scope: **option 3** (no agent intermediary) for the MVP, with
**option 2** documented as the concrete extension seam if/when a Master-agent-like
capability is actually needed — explicitly **not** option 1.

Reasoning:
- **Option 1 is new infrastructure** — a persistent process is a genuinely new
  operational concern (What restarts it? What if it crashes mid-triage? Does it
  count toward the "no new infrastructure" constraint in requirements.md?) for a
  capability whose actual near-term value is unproven. Given this is a
  solo-operator system with an explicit low-operational-overhead constraint,
  building persistent infrastructure speculatively is the wrong default.
- **Option 3 is the simplest correct MVP**: `request_help`'s `reason` and
  `attempted_remediation` fields (Story 5.1.1) already give the human everything a
  triage agent would otherwise summarize — for a single-operator system checking
  `/unfinished` in batches, an extra AI-generated summary layer between the raw
  report and the human adds latency and a new failure surface (what if the triage
  session itself gets stuck or produces a wrong diagnosis?) without a clearly
  proven need.
- **Option 2 remains available without building it now**: `headless.Pool` already
  exists and is exercised daily for review sessions — spawning a triage session
  from `request_help`'s handler the same way `TriggerReviewForSession` spawns a
  review session would be a small, well-understood addition *if* future
  experience shows raw reports aren't enough. This ADR names that seam explicitly
  so a future iteration doesn't have to re-derive it.

## Consequences

- **Positive**: dimension 3 ships without any new infrastructure, consistent with
  every other epic in this plan.
- **Positive**: the extension path is documented, not foreclosed — a future
  "Master agent" (in the on-demand-triage sense) is a natural evolution of
  existing plumbing (`headless.Pool`), not a research question that has to be
  re-opened from scratch.
- **Negative / accepted trade-off**: without any agent triage step, the human sees
  raw agent-reported reasoning verbatim, which may occasionally be lower-quality
  or less actionable than an AI-summarized version would be. Accepted because the
  `reason`/`attempted_remediation` schema (Story 5.1.1) already requires concrete,
  non-vague content at the point of escalation — the summarization value a triage
  agent would add is expected to be marginal relative to the operational cost of
  standing up option 1, and option 2 remains available if this assumption proves
  wrong in practice.
- **Explicit non-goal**: this ADR does not rule out a future "Master agent" in
  option 1's sense entirely — it rules it out for *this plan's* scope, on the
  grounds that nothing in the research surfaced a concrete need for
  always-on-ness specifically (every triage/review/help-response need identified
  is naturally on-demand and short-lived).
