---
name: backlog-feature-improvement
description: Audit the stapler-squad backlog automation/reconciliation feature — current item state, the UI, and the code — using ux:review, quality:architecture-review, code:review, and code:is-it-ready, to find gaps blocking the goal of a fully autonomous, user-configurable end-to-end delivery pipeline (e.g. letting a user pick which skills, like their SDD skills, run per item) — then routes each finding through the matching SDD entry point (sdd:fix-bug / sdd:quick / sdd:full) to research, plan, and implement the fix. Use for periodic health checks, when items report stuck/silently-failing, or when asked how to get closer to a "software factory."
---

# Backlog Feature Improvement

The backlog feature's end goal: an item goes idea → shipped PR with minimal human
intervention, and the pipeline stages (triage → plan → implement → review → merge) are
configurable per item — a user can say "use `/sdd:full` for this one" or "skip review gate
for this one." Today the pipeline is fixed and hardcoded (see Known Findings below). This
skill audits state, UI, and code against that goal and produces a prioritized gap list.

## Before You Start — Check Prior Art

Many investigations already exist. Skim before re-deriving:

- `project_plans/backlog-stuck-item-visibility/` — root causes: auto-merge disabled, silent rework cap, non-durable notify-once state
- `project_plans/backlog-triage-autonomous/`, `backlog-triage-e2e-hardening/`, `backlog-service-refactor/`, `backlog-ux/`, `backlog-pr-conflict-detection/`, `backlog-cross-platform-audit/`, `backlog-management/`
- `docs/adr/ADR-022-headless-triage-over-autonomous-driver.md` — why triage is a direct headless pool call (`pool.CallBlockingWithOptions`), not a tmux-driven `AutonomousDriver`
- `docs/adr/013-workflow-engine-replaces-valid-transitions.md` — `session/workflow_engine.go` is the intended seam for custom/configurable states (S2/S3 in that ADR); check whether it's wired up yet or still just replacing the static map

## Known Findings (2026-07-14 hotspot scan — verify still true before reporting as new)

**Reconciliation-loop hotspots** (churn × complexity), highest risk first:
- `server/services/backlog_service_triage.go` — `SpawnSessionFromItem` (complexity 41), `TriggerTriage` (35), `TriggerReReview` (29). This file *is* the reconciliation loop.
- `server/services/autonomous_orchestration_service.go` — `onAutonomousDriverComplete` (complexity 42, single highest score): closes the loop between an autonomous driver run and backlog status.
- `server/services/backlog_service_lifecycle.go` — `TransitionBacklogItemStatus` (41): the state-machine gate.
- `server/services/backlog_service_sync.go` — `AttachSessionToItem` (38).

**Live bugs:**
- `autonomous_orchestration_service.go:225-230` — `inst.AutonomousMode/AutonomousTurn/AutonomousMaxTurns` mutated with no lock (code comment acknowledges: "unguarded, pending instance-actor-concurrency Epic 5"). Concurrent turn/completion callbacks race on the same `*Instance`.
- `autonomous_orchestration_service.go:253-262,306-311` — notification type/priority use raw magic ints (`int32(9)`, `int32(2)`) instead of proto enum constants; a future proto renumbering breaks these silently.
- `autonomous_orchestration_service.go:248-276` — role→status mapping is a hardcoded `switch`; a new session role added elsewhere falls into `default: no transition` with only a log line — a new pipeline stage silently stops advancing items with nothing surfaced to the operator.

**Hardcoded where it should be configurable (the core "software factory" gap):**
- `session/backlog_commands.go:20-60` (`WriteSlashCommands`) — every item gets the same fixed slash-command set. No hook for per-item pipeline choice.
- `session/repository.go:330-357` (`BacklogItemData`) — only `SkipReviewGate`/`SkipPlanning` bools exist as toggles. No field anywhere for a user-specified skill/command list per item — this has to be added at the data-model layer first.
- `backlog_service_triage.go:72-97` — `maxAutoReworkIterations = 3`, `maxConcurrentBacklogWorkItems = 2`, `defaultTriageCleanupTimeout` are global constants despite in-code comments calling them "operational tuning knobs."

## Phase 1 — Audit Current Item State

Goal: find items stuck/looping/silently stalled *right now*, not just theoretical bugs.

1. `mcp__stapler-squad__search_sessions` / `list_sessions` for active backlog-linked sessions
2. `mcp__stapler-squad__get_backlog_item` for individual items surfaced as stuck
3. RPC — **this is the authoritative source, not the SQLite DB** (see caveat below):
   ```bash
   curl -s -X POST "http://localhost:8543/api/session.v1.BacklogService/ListStuckBacklogItems" \
     -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" -d '{}'
   ```
4. Optional DB cross-check: `~/.stapler-squad/sessions.db` — `backlog_items`, `backlog_status_events`, `backlog_stuck_states`, `item_sessions`. **Caveat (found 2026-07-14): this file can go stale for weeks while the live server serves current data from elsewhere** — a run that day found `sessions.db` frozen since 2026-06-30 returning 0 rows while the RPC returned live current items. Check the DB's mtime before trusting it; if stale, skip this step and rely on the RPC/MCP tools alone rather than burning time locating the live DB path.

Flag: items where `session/workflow_engine.go` / `session/backlog.go` allow a transition into a dead-end state, or a status-event log that flaps (idea→ready→idea repeatedly).

## Phase 2 — Walk the UI

Load `/backlog` and `/backlog/board` (`make install-service` first if the server isn't running) with the claude-in-chrome tool. Click through: board → item card → item detail panel → triage review panel → review-changes modal → GitHub PR picker.

For each screen, note:
- Any step requiring a human click to continue an otherwise-automatable flow — that's a manual gate blocking full autonomy
- Loading states that hang with no error surfaced (same class as ADR-022's `headlessPool` nil gate)
- Whether item detail shows *what pipeline/skills ran* for that item, or only a static status — if a user can't see or choose what ran, it isn't configurable yet

Key components: `web-app/src/components/backlog/{BacklogBoard,BacklogItemPanel,BacklogItemDetail,TriageReviewPanel,ReviewChangesModal,GateVerdictBox,SessionMonitor}.tsx`; routes: `web-app/src/app/backlog/{page,board/page,layout}.tsx`.

## Phase 3 — Apply Quality Skills

Run each scoped to backlog code, not the whole repo:

| Skill | Scope | What to look for |
|---|---|---|
| `quality:architecture-review` | `server/services/backlog_service*.go`, `autonomous_orchestration_service.go`, `session/workflow_engine.go`, `session/backlog.go`, `session/repository.go` | SOLID/Clean Architecture gaps; specifically whether pipeline-stage selection is injected (per ADR-013's `WorkflowEngine` seam) or hardcoded |
| `ux:review` | `/backlog`, `/backlog/board`, item detail + review flows | Friction requiring human judgment where policy/config could decide instead |
| `code:review` | Recent diffs touching `backlog_*`, `autonomous_orchestration_service.go`, `workflow_engine.go` | Correctness bugs in the reconciliation loop — races, missed error handling, silent catches |
| `code:is-it-ready` | Treat "backlog feature = the product" | GO/HOLD/FIX-THEN-SHIP verdict against the autonomous-factory goal |

## Phase 4 — Synthesize

Combine into one gap list, bucketed:

1. **Reconciliation bugs** — state-machine or notifier bugs causing stuck/lost items (file:line, failure scenario)
2. **Manual gates** — places a human must act that a policy/config could replace
3. **Non-configurable pipeline steps** — hardcoded skill/command choice, no hook for user-supplied instructions (e.g. "use my SDD skills for this item type") — the core ask; start from `BacklogItemData` needing a per-item skill/command-list field

For bucket 1 findings specifically, also name the **recurring shape**, not just the instance —
this project's reconciliation bugs keep recurring in a small number of shapes across audit
passes (2026-07-14 through present): a spawn call silently no-ops instead of erroring, a
crash-recovery sweep's own exclusion guard defeats the exact case it should catch, an event
(exit callback, status transition) is lost across a service restart with no catch-up path,
notify-once state that's marked but never resolved automatically. If a new finding matches
one of these shapes, say so explicitly — it's evidence the systemic fix from a prior pass
didn't actually close the class, only patched the instance.

Write the raw findings to `docs/tasks/backlog-feature-improvement.md` (Implementation-Plan format, per this repo's task-doc convention) as the audit record, then route each bucket per Phase 5.

## Phase 5 — Route Fixes Through SDD

Don't hand-implement fixes directly off the audit — route each bucket through the SDD entry point that matches its size, so fixes get research, an ADR when architecture changes, and a validation gate before code is written:

| Bucket | Entry point | Why |
|---|---|---|
| 1. Reconciliation bugs (isolated, no architecture change — e.g. the unguarded `*Instance` mutation, the magic-int notification constants) | `sdd:fix-bug`, one run per bug | Root cause → targeted fix → regression test; independent bugs can run as parallel sessions |
| 2. Manual gates (UX/workflow friction, no data-model change) | `sdd:quick` | Fits in one context window, skips heavyweight artifacts |
| 3. Non-configurable pipeline steps (the core software-factory gap — new `BacklogItemData` field, proto changes, `WorkflowEngine` wiring, UI to surface/select it) | `sdd:full`, seeded with a hand-written `requirements.md` | Touches data model + proto + backend + UI together — the same "many touchpoints must move in lockstep" shape as `.claude/rules/session-creation-registry.md`; deserves an architecture review and ADR before code |

For bucket 3, write `project_plans/backlog-configurable-pipeline/requirements.md` directly from the Phase 4 findings — skip the `sdd:1-ideate` interview, since the audit already answered *what* and *why* — then start SDD at `sdd:2-research`. Keep each requirement traceable back to the specific hotspot/UI/architecture finding that motivated it.

Per `.claude/rules/sdd-planning-artifacts-commit.md`: commit `project_plans/backlog-configurable-pipeline/` before the session ends, even if implementation hasn't started yet.

### Prefer systemic fixes over instance patches

This feature has been audited repeatedly (see the dated `## Update` sections in
`docs/tasks/backlog-feature-improvement.md`) and the same bug *shapes* keep resurfacing under
new item IDs — a strong signal that prior fixes closed the instance, not the class. Every
`sdd:fix-bug` run against a bucket-1 finding now includes a mandatory Phase D
("Reflect — fix the class, not the instance") that classifies the root cause via
`quality:reflect-and-fix`'s taxonomy and implements enforcement at the earliest achievable
level (type → lint → test), not just a regression test for that one item. When routing a
finding here:

- Check whether it matches a shape already named in this doc's prior updates before treating
  it as new — if so, the fix must explain why the earlier attempt didn't close the class, and
  address that gap, not just this occurrence.
- Prefer a fix that removes the failure mode structurally (e.g. a shared spawn helper that
  can't silently no-op, a sweep whose exclusion guard is unit-tested against the exact
  self-defeating case found here) over a targeted patch at the one call site the audit
  happened to find.
- If a bucket-1 fix ships without closing the class, note that explicitly in the next audit
  pass rather than silently re-discovering the same shape as a "new" bug.
