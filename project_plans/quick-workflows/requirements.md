# Quick Workflows — Requirements

## Feature Summary

Add a "Quick Workflows" system to Stapler Squad that lets users define named workflow templates. Each workflow pre-populates a session with a specific slash command/skill, target directory, input template, session type, and model/agent settings. Workflows are invokable from the omnibar via a short syntax, runnable from a dedicated workflow management panel, and schedulable via cron.

**Motivating use case**: The user has a `/knowledge:synthesize` skill they run against URLs occasionally. They want to type a short trigger in the omnibar (e.g. paste a URL), pick the "knowledge-sync" workflow, and have a session launch with the right directory, command, and URL injected — without manually configuring each time.

---

## User Stories

### US-01 · Omnibar Invocation
As a user, I want to type a workflow shorthand in the omnibar (e.g. `@knowledge-sync https://example.com`) so that a session launches pre-configured with the right skill, directory, and URL injected into the prompt — without manual setup.

**Acceptance Criteria:**
- A new omnibar detector recognises the workflow trigger syntax
- Matching workflows are surfaced as suggestions in the omnibar dropdown
- Selecting a workflow opens the creation panel pre-filled with workflow settings
- If the trigger includes extra text (URL/string), it is injected into the workflow's input template
- The trigger syntax must not conflict with existing omnibar patterns (GitHub URLs, `new:`, path detection, session search)

### US-02 · Workflow Management Panel
As a user, I want a dedicated panel (accessible from the sidebar/nav) to view, create, edit, and delete my saved workflows, so I can manage them without editing JSON manually.

**Acceptance Criteria:**
- A "Workflows" entry appears in the sidebar/navigation
- The panel lists all saved workflows with name, command, directory, and schedule (if any)
- Users can create a new workflow via a form with fields: name, slug, command, target directory, input template, session type, model/agent settings
- Users can edit and delete existing workflows
- Changes persist immediately

### US-03 · Workflow Scheduling (Cron)
As a user, I want to attach a cron schedule to a workflow so that it runs automatically at a defined time, launching a one-off session that executes the command unattended.

**Acceptance Criteria:**
- A workflow can optionally have a cron expression
- On schedule, the system creates a one-off session targeting the workflow's configured directory
- The session runs the configured command/skill and terminates (or can be monitored)
- The user receives a notification (in-app) when a scheduled workflow session starts
- Cron is the default execution mode for scheduled runs; future options (persistent session, notification-only) are architected but not required in v1

### US-04 · Workflow Definition Schema
As a developer, I want a well-defined workflow schema so that the system is extensible and workflows are portable.

**Workflow fields:**
| Field | Type | Required | Description |
|---|---|---|---|
| `id` | string (UUID) | yes | Internal ID |
| `slug` | string | yes | Short name used in omnibar trigger (e.g. `knowledge-sync`) |
| `name` | string | yes | Human-readable display name |
| `description` | string | no | Optional description shown in dropdown/panel |
| `command` | string | yes | Slash command or skill to invoke (e.g. `/knowledge:synthesize`) |
| `targetDirectory` | string | yes | Absolute path for the session's working directory |
| `inputTemplate` | string | no | Template with `{{input}}` placeholder for omnibar text/URL |
| `sessionType` | enum | yes | `directory` \| `one_off` \| `new_worktree` \| `existing_worktree` |
| `model` | string | no | Model override (e.g. `claude-opus-4-8`) |
| `agentType` | string | no | Agent type override |
| `cronExpression` | string | no | Cron schedule (e.g. `0 8 * * 1`) |
| `cronEnabled` | bool | no | Whether the cron is active |
| `createdAt` | timestamp | yes | Creation timestamp |
| `updatedAt` | timestamp | yes | Last update timestamp |

### US-05 · Storage
As a system, workflows must be persisted reliably. The storage mechanism should be determined by research into the trade-offs between extending `config.json`, a separate `workflows.json`, in-database (ent ORM/SQLite), and per-project YAML files. Research should recommend the best approach given the existing architecture.

---

## Non-Goals (v1)

- No drag-and-drop reordering in the panel (sort alphabetically by name)
- No workflow chaining (running one workflow after another)
- No per-run parameter overrides beyond the omnibar text injection
- No export/import of workflows as files (v2)
- No "persistent session" or "notification-only" cron modes (v2)

---

## Constraints

- Omnibar trigger syntax must not conflict with: GitHub URL patterns, `new:` shorthand, `/path` detection, `owner/repo` shorthand, or session search fallback
- Must follow vanilla-extract CSS architecture (ADR-009)
- Must register in the OmnibarAction discriminated union and DetectorRegistry
- Must update `docs/registry/` feature registry files
- New session creation mode (if needed) must follow the 7-touchpoint registry in `.claude/rules/session-creation-registry.md`
- Storage solution must integrate with the existing Go backend and React frontend without breaking existing config/session state
- Proto changes require `make generate-proto`

---

## Success Metrics

- User can define a workflow in < 60 seconds via the management panel
- Omnibar workflow invocation creates a configured session in < 2 seconds (same as normal session creation)
- Cron-scheduled workflows run within ±30 seconds of their scheduled time
- Zero regressions in existing omnibar detection (verified by existing detector tests)
