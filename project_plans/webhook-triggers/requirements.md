# Requirements: Webhook / Event-Driven Session Creation and Lifecycle Callbacks

**Source**: Migrated from `TylerStaplerAtFanatics/stapler-squad#47` (backlog item
`d963144c-494b-458e-b833-a0f65e7d1559`). No interactive ideation interview was run —
this doc is derived directly from the backlog item's title/description and codebase
investigation (non-interactive SDD pipeline mode).

## Problem

Every stapler-squad session today is created by a human explicitly clicking "New
Session" or invoking `create_session`/`create_backlog_item`. The existing "rules
engine" (`server/services/rules_service.go`, `RulesStore`, `ApprovalRule` ent schema)
is purely **reactive**: it only evaluates inbound agent tool-call requests for
auto-approval. There is no mechanism today for an external event (a GitHub push, a
JIRA ticket, a generic webhook) to *create* a session, and no mechanism for a
session's lifecycle transitions to notify an external system or chain into a new one.

**Correction (found during Phase 2 research, see `research/architecture.md` and
`research/build-vs-buy.md`):** the cron half of this problem is *already solved*.
`github.com/robfig/cron/v3` is a direct dependency and `server/workflows/scheduler.go`
+ the `Workflow` ent schema already implement a working, DB-backed, hot-reloadable
cron scheduler that calls the real `CreateSession` RPC and stamps `WorkflowId` for
source attribution — `WorkflowService.CreateWorkflow`/`UpdateWorkflow` already
hot-swap the running scheduler with zero restart. What's genuinely missing is: (a) an
inbound webhook HTTP receiver (`github_push`, generic `webhook` types) with signature
verification — zero HMAC usage exists anywhere in the repo today — and (b) outbound
lifecycle callbacks and pipeline chaining. This significantly narrows the build: `cron`
triggers should extend the existing `Workflow`/`Scheduler` machinery, not duplicate it.

## Why This Matters

Competitive tools (Dorothy, Tutti, Sortie — per the original issue's competitive
scan) all support proactive, event-triggered agent sessions and/or step-chaining.
Without this, stapler-squad requires a human in the loop for every unit of work,
capping throughput and preventing "background automation engine" use cases (nightly
audits, issue-triggered implementation, CI-failure-triggered fixes, plan→implement→
review→merge pipelines).

## Goals

1. **Inbound triggers**: allow external events to create a new session automatically:
   - `github_push` (repo + branch match)
   - `cron` (schedule string)
   - `webhook` (generic inbound HTTP endpoint, with an event-type + optional label
     filter and a Go-template prompt)
2. **Outbound callbacks**: allow session lifecycle events (complete, stale, queue-item
   created) to POST to an external URL.
3. **Pipeline chaining**: allow one session's completion to trigger a new session,
   optionally passing the completed session's output as context (plan → implement →
   review → merge without manual handoff).
4. Keep human oversight intact — triggers create sessions/backlog items through the
   existing approval/review path, they do not bypass it by default.

## Non-Goals (this pass)

- A full workflow DAG/orchestration UI (existing `create_workflow`/`run_workflow`
  MCP tools already give manual chaining; this item only adds automatic triggering
  of that chaining from lifecycle events).
- Building a general-purpose webhook receiver framework for arbitrary third-party
  integrations beyond GitHub/JIRA/generic webhook + cron.
- Authentication/identity federation for external callers beyond a shared-secret /
  HMAC signature check.
- Auto-merge or auto-deploy — out of scope, unrelated to trigger/callback plumbing.

## Functional Requirements

### Inbound triggers
- FR1: A `triggers` config (JSON, likely under existing `config/` JSON persistence
  patterns) defines zero or more trigger specs: `type` (`github_push` | `cron` |
  `webhook`), match criteria per type, and a `prompt` or `prompt_template`.
- FR2: `github_push` triggers require a receiver endpoint that verifies GitHub's
  webhook HMAC signature (`X-Hub-Signature-256`) before acting.
- FR3: `cron` triggers run on an in-process scheduler (evaluated against a
  standard 5-field cron expression) and fire session creation via the same path as
  a manual `create_session`/`create_backlog_item` call.
- FR4: Generic `webhook` triggers expose a per-trigger HTTP path
  (`/webhooks/<slug>` or similar), verify a shared-secret/HMAC signature, support an
  `event` field match and an optional `label_filter`, and render `prompt_template`
  with the inbound JSON payload (Go `text/template`, matching the issue's
  `{{issue.key}}`-style syntax).
- FR5: Trigger-created sessions/backlog items are tagged with their originating
  trigger (source attribution) for auditability, consistent with
  `feedback_document_ai_decisions_in_edge_cases` (AI/automated actions must be
  visibly attributable, not silent).
- FR6: Trigger config supports enable/disable without a full redeploy (matches the
  existing `dynamic-rule-reload` plan's precedent for the approval-rules engine —
  reuse that reload mechanism if applicable rather than building a second one).

### Outbound callbacks
- FR7: `on_session_complete`, `on_session_stale`, `on_queue_item_created` (and any
  other lifecycle events already emitted internally — check `session/` lifecycle
  hooks) each accept a URL; on the corresponding lifecycle transition, stapler-squad
  POSTs a JSON payload describing the event.
- FR8: Outbound callback delivery is best-effort with bounded retry (do not block
  session lifecycle transitions on callback delivery; a slow/dead external endpoint
  must not stall the session state machine).
- FR9: Failed callback deliveries are logged/surfaced, not silently dropped (same
  visibility principle as FR5).

### Pipeline chaining
- FR10: A session can declare a "next" trigger/prompt to fire on its own
  completion, with the completed session's output (or a summary of it) interpolated
  into the next session's prompt — this is really FR7 (`on_session_complete`)
  wired back into FR3's session-creation path, not a separate mechanism.

## Acceptance Criteria (initial — see triage JSON for final list)

- AC1: A GitHub push to a configured repo/branch creates a new session with the
  configured prompt, verified via HMAC signature check (invalid/missing signature
  is rejected).
- AC2: A cron-scheduled trigger fires a new session at the scheduled time without
  manual intervention.
- AC3: A generic webhook trigger with a matching `event` and `label_filter` creates
  a session using the rendered `prompt_template`; non-matching events/labels are
  ignored.
- AC4: On session completion, a configured `on_session_complete` URL receives a POST
  with session outcome data; delivery failure does not block or corrupt the
  session's own state transition.
- AC5: A session can be configured to trigger a follow-up session on its own
  completion, with the prior session's output available to the new session's prompt.
- AC6: All trigger-created sessions/backlog items are visibly attributed to their
  originating trigger (name/type) in the UI/API, not indistinguishable from
  manually created ones.
- AC7: Trigger and callback configuration can be added/edited/disabled without
  restarting the service.
- AC8: Malformed or unauthenticated inbound webhook requests are rejected with an
  appropriate HTTP status and do not create sessions.

## Constraints / Conventions to Follow

- New RPCs/config touch the **feature registry** (`.claude/rules/feature-registry.md`)
  and, if any UI surface is added, the **session creation registry**
  (`.claude/rules/session-creation-registry.md`) if trigger-created sessions
  introduce a new `SessionType` or bypass existing creation paths.
- Prefer `go-git` over shelling out where trigger evaluation touches git state
  (`.claude/rules/prefer-go-git-over-subshells.md`).
- Avoid Java/Spring-shaped abstractions — no speculative `TriggerProvider` interface
  with one implementation; see `.claude/rules/interface-pollution-checklist.md`.
- New e2e coverage required per `.claude/rules/e2e-test-conventions.md` if a UI
  surface (trigger/callback config page) is added.

## Open Questions

See `suggestions` in the final triage JSON output — flagged there rather than here
since no user is present to answer them in this session.
