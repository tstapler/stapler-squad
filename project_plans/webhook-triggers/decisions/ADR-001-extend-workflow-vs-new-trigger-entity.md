# ADR-001: Extend `Workflow` with a `trigger_type` discriminator instead of a new `Trigger` entity

**Status**: Accepted
**Date**: 2026-08-06
**Context**: `project_plans/webhook-triggers`

## Context

`webhook-triggers` needs to persist three new kinds of "thing that creates a session
automatically": `github_push`, `webhook`, and (already-existing, needs richer match/template
support) `cron`. Phase 2 research unanimously found that a full cron-scheduling stack
(`robfig/cron/v3` dependency, `server/workflows/scheduler.go`'s `Scheduler`, and the
`ent.Workflow` schema) already ships in production, driven today by the
`create_workflow`/`run_workflow` MCP tools. `Scheduler.FireNow` already builds a
`CreateSessionRequest`, calls the real `SessionService.CreateSession`, and stamps
`WorkflowId` on the created session for attribution. `WorkflowService.CreateWorkflow`/
`UpdateWorkflow` already hot-swap the running scheduler with zero restart.

Three storage shapes were considered for the new trigger types:

- **(A) Extend `Workflow`**: add a `trigger_type` string field plus new optional columns
  (`github_repo`, `github_branch`, `webhook_slug`, `webhook_secret_encrypted`,
  `event_filter`, `label_filter`, `prompt_template`) to the existing `ent.Workflow` entity.
  All four trigger types (`cron`/`github_push`/`webhook`/`manual`) are rows in one table,
  fired through one `Scheduler`.
- **(B) New `Trigger` entity referencing `Workflow`**: a `Trigger` row holds
  activation/match config (cron expression, webhook slug/secret, GitHub repo/branch) and
  points at a `Workflow` row for the "what to do" part (command, prompt, target directory,
  session type).
- **(C) Fully separate `Trigger`/`Callback` entities, decoupled from `Workflow`, with an
  independent scheduler**: a parallel subsystem that does not touch `Workflow` at all.

## Decision

**Adopt (A): extend `Workflow`** with a `trigger_type` discriminator and the per-type match/
secret/template fields, rather than introducing a new entity.

## Rationale

- A `Trigger` row (option B) *is* a `Workflow` row with a different activation mechanism —
  every trigger type still needs to render a prompt, call `CreateSession`, and attribute the
  result. Splitting "activation config" from "what to do" into two joined entities would
  duplicate `Command`/`InputTemplate`/`TargetDirectory`/`SessionType`/`AgentType`/`Model` and
  require a second `FireNow`-equivalent, a second RPC CRUD surface, and a second
  enable/disable mechanism — none of which adds real separation of concerns, since in every
  concrete trigger type in this feature's scope, one activation config maps to exactly one
  action config (no evidence of a real many-to-many need between "what fires" and "what
  happens").
- Option C (fully decoupled entities + independent scheduler) was rejected outright: running
  two competing in-process `cron.Cron` instances in one binary is a maintenance footgun with
  no offsetting benefit, and all six Phase 2 research documents (stack, features, architecture,
  pitfalls, ux, build-vs-buy) independently converged on "don't build a second cron engine."
- Extending `Workflow` makes FR6/AC7 ("enable/disable without redeploy") and FR9 (source
  attribution) solved problems for free: `Scheduler.Reload`'s hot-swap and `CreateSessionRequest.
  WorkflowId`'s existing FK already do exactly what a new `Trigger` entity would need to
  reimplement from scratch.
- The known cost of option A — a wider `Workflow` table with several nullable,
  trigger-type-conditional columns — is accepted as the lesser problem. It is mitigated by:
  keeping all new columns `.Optional()` with safe zero defaults (so existing `cron`/`manual`
  rows are unaffected), and by RPC-layer conventions (only trigger-type-relevant fields are
  surfaced/validated per `trigger_type` in `WorkflowService`, per Story 1.1.1 and Task 3.1.1b).
  If a future trigger type introduces genuinely disjoint, non-overlapping config shapes at a
  scale where the wide-table cost outweighs the reuse benefit, that is grounds to revisit this
  decision — not to preempt it now for a speculative future need (per
  `.claude/rules/interface-pollution-checklist.md`'s "discover interfaces, don't design them
  speculatively").

## Consequences

- `session/ent/schema/workflow.go` grows by ~8 fields (Task 1.1.1a) and one migration
  (additive, non-destructive — see plan.md's Migration Plan).
- `Scheduler` gains a `FireTrigger` method generalizing `FireNow` (Task 3.2.1a); existing
  `run_workflow`/cron callers are unaffected (backward-compatible wrapper).
- Existing `Workflow` rows are backfilled with `trigger_type = "cron"` or `"manual"` based on
  their current `CronEnabled` value (Task 1.1.1d) — no behavior change for existing
  cron-workflow users.
- If webhook-specific fields (`webhook_secret_encrypted`, `event_filter`, `label_filter`) are
  later judged to have made `Workflow` too wide for its original cron/manual use case, a
  follow-up migration to split them into a joined `WorkflowTriggerConfig` table remains
  possible without touching `Scheduler`'s external API — this ADR does not foreclose that,
  it only rejects doing it preemptively now.

## Alternatives Rejected

| Alternative | Reason Rejected |
|---|---|
| (B) New `Trigger` entity referencing `Workflow` | Duplicates action-config fields and firing/attribution plumbing for no concrete near-term benefit; a `Trigger` row is structurally identical to a `Workflow` row with a different activation mechanism |
| (C) Fully decoupled `Trigger`/`Callback` entities + independent scheduler | Runs two competing in-process cron engines in one binary; unanimous rejection across all six Phase 2 research documents |
