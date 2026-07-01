# Backlog Feature — End-to-End User Journey (as of 2026-07-01)

Read-only trace of the backlog feature from a real user's perspective: UI entry point →
RPC → backend code path → reachability → implementation status → autonomy/guardrail notes.

Repo root: `/home/tstapler/.stapler-squad/workspaces/d685c4b1a423cca3/worktrees/stapler-squad-bklof_18be35f4c19c4b24`

---

## Step 1 — Enabling the backlog feature flag

**(a) UI entry point:** `/settings/features` page (`web-app/src/app/settings/features/page.tsx`).
Renders a real toggle switch per flag returned by the backend, with `backlog` given the
friendly label "Backlog" via `FEATURE_META` (`page.tsx:24-26`). Clicking the switch calls
`setFlag(name, !enabled)`.

**(b) RPC:** `FeatureFlagsContext.tsx:62-74` → `client.updateFeatureFlag({name, enabled})`
(ConnectRPC `SessionService.UpdateFeatureFlag`).

**(c) Backend path:** `server/services/session_service.go` `UpdateFeatureFlag` handler persists
to `config.json` via `cfg.SetFeatureFlag("backlog", true)`. `config/feature_flags_test.go`
confirms default is `false` and persistence round-trips correctly.

**(d) Reachable:** Yes — genuinely reachable purely through the UI. No manual config-file
edit, env var, or restart required ("Changes take effect immediately — no restart needed",
per the page's own subtitle).

**(e) Status:** Implemented.

**(f) Autonomy:** N/A (pure human action).

---

## Step 2 — Navigating to the Backlog page

**(a) UI entry point:** Desktop nav (`web-app/src/components/ui/Navigation.tsx:21,26`)
conditionally includes a "Backlog" link only `if (backlogEnabled)` via
`useFeatureFlag("backlog")`. Mobile bottom nav / drawer nav pull from a shared
`web-app/src/lib/nav-pages.ts:52` entry tagged `featureFlag: "backlog"`, consumed by
`BottomNav.tsx` / `DrawerNav.tsx`.

**(b) RPC:** None directly — `web-app/src/app/backlog/layout.tsx:9-17` reads the already-loaded
`useFeatureFlags()` context. While flags are loading it renders nothing; once loaded, if
`flags["backlog"]` is falsy it calls `router.replace("/")` and renders nothing (silent
redirect, no explanatory message).

**(c) Backend path:** Flag value fetched at app load via `GetFeatureFlags`.

**(d) Reachable:** Yes, once the flag is on (Step 1). Correctly gated in both desktop and
mobile chrome. Note: an unconditional "Review Queue" nav item exists alongside "Backlog" and
is NOT flag-gated (`Navigation.tsx:25`) — see Step 9.

**(e) Status:** Implemented.

**(f) Autonomy:** N/A.

---

## Step 3 — Empty state / creating the first item

**(a) UI entry point:** `BacklogEmptyState.tsx:61-195` — headline, a 5-node lifecycle diagram
(idea → ready → in progress → review → done), and a "+ Create First Item" CTA (`:108-116`)
that reveals an inline mini-form with only **Title** (required) and **Priority**
(select, default `3` = P3/Medium) (`:64,157-172`).

**(b) RPC:** Submitting calls `onCreateItem({title, priority})`
(`web-app/src/app/backlog/page.tsx:339,254-272`) → `createBacklogItem` from
`useBacklogService.ts:280,353-373` → ConnectRPC `BacklogService.CreateBacklogItem`.

**(c) Backend path:** `server/services/backlog_service.go:396-454`. Requires only non-empty
`title`; defaults to `session.DefaultBacklogPriority` if priority is `0`.

**(d) Reachable:** Yes, purely via UI clicks.

**(e) Status:** Implemented, but with a **real gap**: the handler auto-triggers triage only
`if !SkipTriage && created.RepoPath != "" && s.headlessPool != nil` (`backlog_service.go:434-448`).
The empty-state mini-form never collects `repoPath` (only the full `BacklogItemForm.tsx`
does, where `repoPath` is required for non-edit items — `BacklogItemForm.tsx:51-52,169-181`).
So items created via the "first item" CTA silently skip auto-triage, with no warning shown
in the empty-state UI about this consequence.

**(f) Autonomy:** N/A at this step (creation is pure human input), but sets up whether Step 6
can run automatically later.

E2E coverage exists: `tests/e2e/backlog.spec.ts` — `e2e:backlog-empty-state-renders`,
`e2e:backlog-empty-form-opens/-cancel/-submit`, `e2e:backlog-create-item`,
`e2e:backlog-default-priority`.

---

## Step 4 — Creating an item generally / item display

**(a) UI entry point:** Full `BacklogItemForm.tsx` (via "+ New Item" once the board is
non-empty) collects title, description, priority, repo path (required for triage),
acceptance criteria. `BacklogItemCard.tsx` and `BacklogItemBadge.tsx` render each item on the
board.

**(b) RPC:** Same `CreateBacklogItem` as Step 3; updates go through `updateBacklogItem`
(`useBacklogService.ts`).

**(c) Backend path:** `backlog_service.go:396-454` (create), `UpdateBacklogItem` (update).

**(d) Reachable:** Yes.

**(e) Status:** Implemented. `BacklogItemBadge.tsx:30-54` renders a status chip for all 7
lifecycle states (idea/refining/ready/in_progress/review/done/archived,
`STATUS_CLASS` at `:14-22`) plus an AC done/total counter and truncated title.
`BacklogItemCard.tsx` separately renders the priority badge (P1–P5) and a status-derived
action button. Both status and priority are genuinely visible on the board, not just in the
API response.

**(f) Autonomy:** N/A.

---

## Step 5 — External source ingestion (GitHub issues/PRs)

**(a) UI entry point:** **None.** No component, hook method, or MCP tool exposes
`CreateItemSource` / `ListItemSources` / `UpdateItemSource` / `DeleteItemSource` /
`TriggerSync` / `GetSyncHistory`. `grep` across `web-app/src` for `ItemSource` matches only
the generated proto file (`web-app/src/gen/session/v1/backlog_pb.ts`).

**(b) RPC:** Backend RPCs are fully implemented (`server/services/backlog_service.go:760-876`
for source CRUD), but two are stubs: `TriggerSync` (`:1589-1594`) and `GetSyncHistory`
(`:1598-1602`) both return `connect.CodeUnimplemented`.

**(c) Backend path:** On server startup, if `cfg.GetFeatureFlag("backlog")` is true,
`server/dependencies.go:781-792` calls `backlogCtrl.Enable()`
(`session/feature_controller.go:43`), which constructs a `SyncLoop`
(`session/backlog_sync.go:27-46`) with a real `time.Ticker` firing every 15 minutes
(`defaultSyncInterval`, `:15,50-62`). It calls `runAllSources` → `SyncOne` → the GitHub plugin's
`Fetch` (`session/backlog_plugin_github.go`, `backlog_plugin_github_prs.go`, registered in
`NewDefaultRegistry`, `backlog_plugin.go:52-57`) → `storage.CreateBacklogItem`/`UpdateBacklogItem`
with local-wins merge (`backlog_sync.go:77-254`). But this ticker only has anything to act on
if an `ItemSource` row already exists — and nothing can create one through the UI.

**(d) Reachable:** **No.** The chain source-config-UI → sync-trigger-UI → items-appear has two
missing UI links (source config, manual sync) and one broken backend RPC (`TriggerSync`).
A developer must manually insert an `ItemSource` via a raw ConnectRPC call (e.g. `buf curl`)
for the automatic 15-minute `SyncLoop` to have anything to sync. Once seeded, ingestion then
proceeds automatically and silently, with zero UI visibility into sync history.

**(e) Status:** (i) GitHub plugin backend logic — implemented. (ii) Source configuration UI —
not implemented. (iii) Sync-trigger UI — not implemented (and the underlying RPC is itself
unimplemented). (iv) Automatic/background sync scheduling — implemented and correctly wired
to the feature flag lifecycle.

**(f) Autonomy:** The 15-minute sync loop runs unattended once seeded — this is autonomous by
design (a poller), and the only guardrail is the local-wins merge policy preventing the sync
from clobbering user edits. There is no rate-limit/backoff visible from the UI side since
there is no UI at all for this step.

---

## Step 6 — Automated triage

**(a) UI entry point:** "Trigger Triage" button in `BacklogItemDetail.tsx:601-611,618-625`,
rendered when item status is `idea` or `ready`. Disabled with `title="Set repository path
first"` when `!item.repoPath` (confirmed by e2e test
`e2e:backlog-triage-gate-disabled`, `tests/e2e/backlog.spec.ts:457-485`). If the item's
description is thin, `VaguenessPromptModal.tsx` forces an explicit choice ("Refine" vs.
"Proceed") before triage runs (`VaguenessPromptModal.tsx:16-21` — "No escape-key dismissal:
the user must choose one of the two explicit options").

**(b) RPC:** `triggerTriage(itemId)` (`useBacklogService.ts:289,463-470`) →
`BacklogService.TriggerTriage`.

**(c) Backend path:** `server/services/backlog_service.go:1101-1170+`. Explicitly
user/RPC-invoked — **not** a silent background job. Guards: item must be `idea`/`ready`
(`:1118-1123`), `repoPath` required (`:1125-1129`), orphan-aware re-trigger guard for stale
triage sessions (`:1131-1153`). Per **ADR-022** (`docs/adr/ADR-022-headless-triage-over-autonomous-driver.md`,
Accepted 2026-06-22), triage runs via a direct headless-LLM-pool call
(`pool.CallBlockingWithOptions`, bounded by `triageSem` cap 8), not via the tmux-based
`AutonomousDriver` — that path was tried first and abandoned due to four concrete failure
modes (prompt mismatch, silent nil-pool gate, 5-min per-turn timeout firing mid-run, missing
completion signal). The prompt instructs Claude to write plan artifacts to
`docs/tasks/<slug>/` and return a JSON result parsed by `ParseHeadlessTriageResult`
(`session/backlog_triage.go:94`).

**(d) Reachable:** Yes, purely via UI click, assuming the flag is on and `repoPath` is set.

**(e) Status:** Implemented. Progress is shown via `TriageLoadingIndicator.tsx` (polling
every 5s while `triageStatus === "running"`, `BacklogItemDetail.tsx:100-104`), with a cancel
action (`cancelTriage`) and a failure banner (`InlineError`, `:544-548`) offering retry.

**(f) Autonomy:** The triage LLM call itself runs unattended once triggered (no
tool-call-by-tool-call human approval inside triage) — bounded by the headless pool's
timeout/error handling and a concurrency cap of 8. The output (plan + AC suggestions) is
never auto-applied; see Step 7.

---

## Step 7 — Plan approval / triage review

**(a) UI entry point:** `TriageReviewPanel.tsx`, rendered inline in `BacklogItemDetail.tsx:508-520`
when `triageStatus === "completed" && status === "idea"` and not previously dismissed
(dismissal persisted in `localStorage`, `TriageReviewPanel.tsx:10-21`). Shows the diff of
suggested AC criteria (`TriageDiffSection.tsx`) with **Apply** / **Undo** (7s undo toast,
`:74`) and **Skip** actions. Separately, once status is `ready`, an **"Approve Plan"** button
appears (`BacklogItemDetail.tsx:654-663`) if `item.planArtifactsPath` is set.

**(b) RPC:** `approvePlan(itemId)` (`useBacklogService.ts:291,490-496`) →
`BacklogService.ApprovePlan`.

**(c) Backend path:** `server/services/backlog_service.go:716-756`. Preconditions: rejects
with `FailedPrecondition` if `PlanArtifactsPath == ""` ("no plan artifacts found — run
TriggerTriage first", `:732-734`) or if the path doesn't exist on disk (`:736-739`). On
success it purely flips `PlanApproved = true` and stamps `PlanApprovedAt` — **there is no
automated re-validation of plan quality/content**; approval is a rubber stamp gated only on
"did triage produce a directory on disk."

**(d) Reachable:** Yes, purely via UI click — this is a real, clickable human gate, not
MCP/API-only.

**(e) Status:** Implemented. `TestApprovePlan_MissingPlanArtifactsPath_ReturnsFailedPrecondition`
and `TestApprovePlan_HappyPath_SetsPlanApprovedAndTimestamp` cover it at the backend level
(no corresponding e2e test exists — `backlog.spec.ts` stops at the triage-gate-disabled check).

**(f) Autonomy:** This is the intended human-in-the-loop checkpoint before autonomous
execution (Step 8): `canSpawnSession` in `BacklogItemDetail.tsx:382-384` requires
`item.status === "ready" && (item.skipPlanning || item.planApproved)` — the Spawn/Run
Autonomously buttons are disabled until this gate passes (or `skipPlanning` bypasses it,
which is a config/data flag with no UI toggle found).

---

## Step 8 — Autonomous session execution

**(a) UI entry point:** "Spawn Session" and "Run Autonomously" buttons in
`BacklogItemDetail.tsx:626-653`, both `disabled={!canSpawnSession}` (see Step 7 gate).
Live output surfaces inline via `SessionMonitor.tsx`, rendered in
`BacklogItemDetail.tsx:792-810` for the currently-active linked session matching the item's
lifecycle phase.

**(b) RPC:** `spawnSessionFromItem(itemId, {autonomous: true})`
(`useBacklogService.ts:288,444-455`) → `BacklogService.SpawnSessionFromItem`.

**(c) Backend path:** `server/services/backlog_service.go:883,966` →
`StartAutonomousDriverForInstance` (`server/services/session_service.go:782-795`) →
`session/autonomous_driver.go:66-137`, a turn-based LLM orchestrator that injects prompts
into a tmux session until the LLM emits a `DONE:` signal or `maxTurns` (default 20) is hit.
Runtime guardrails: `maxTurns` cap, 5-minute per-turn idle timeout, 4-hour rate-limit cap
(`autonomous_driver.go:258-291`), and prompt-injection defense via XML delimiters
(`:334-336`).

**(d) Reachable:** Yes, purely via UI click, once the plan-approval gate (Step 7) is passed.

**(e) Status:** Implemented for **work-session execution** (this step). Caveat: ADR-022
documents that this exact same `AutonomousDriver` mechanism was tried and abandoned for
**triage** specifically due to fragility (Step 6 now bypasses it entirely). It is not
documented as broken for work-session execution, but the ADR is direct evidence the
mechanism has known failure modes in this codebase and should not be assumed rock-solid.

**(f) Autonomy — the key human-in-the-loop question:** Once spawned, "Run Autonomously"
executes tool calls **without per-call human approval** inside that session (that's the
literal purpose of the button, contrasted with plain "Spawn Session" which presumably keeps
normal approval prompts — not confirmed by this trace). What stops it from going wrong
unattended: (1) the plan-approval gate that must pass before this button is even clickable
(Step 7); (2) `maxTurns`/timeout/rate-limit caps in the driver; (3) the mandatory review gate
that runs automatically afterward (Step 9) before anything is considered "done." There is no
mid-flight kill switch surfaced in the UI beyond the general session Delete action
(`BacklogItemDetail.tsx:765-782`, with a confirm dialog).

---

## Step 9 — Review queue

**(a) UI entry point:** Two distinct things exist under the "review" umbrella:

  1. A **generic, backlog-agnostic** Review Queue: `/review-queue` page
     (`web-app/src/app/review-queue/page.tsx`), `ReviewQueuePanel`, backed by
     `session/review_queue*.go` and `ReviewQueueDeterminer`. Its nav entry
     (`Navigation.tsx:25`) is **not** feature-flag gated — visible regardless of the backlog
     flag.
  2. A **backlog-specific pre-merge review gate**: `session/backlog_review.go` implements a
     secret-scanner + LLM review (`RunPreGateSecurityCheck`, `secretPatterns`), invoked via
     `spawnReviewGate` automatically from `onSessionExited`
     (`session/backlog_lifecycle.go:159,229`) — i.e. it fires on its own when the autonomous
     work session ends, no click required. Its verdict surfaces in `BacklogItemDetail.tsx`
     via `GateVerdictBox.tsx` (Approve / Reopen / Override-with-reason / Skip-gate actions,
     `BacklogItemDetail.tsx:550-565,272-340`), shown when `item.status === "review"`.

**(b) RPC:** Generic queue: `WatchReviewQueue` stream + `GetReviewQueue` REST fallback
(per ADR-021, see below). Backlog gate actions: `transitionStatus`, `overrideVerdict`
(`useBacklogService.ts:292,502-513`).

**(c) Backend path:** Generic queue trigger is `session/review_queue_determiner.go:37-46`
— purely generic session state (idle + uncommitted changes/attention reason), with **no
reference to backlog item IDs or status** at all. A backlog-spawned autonomous session lands
in this generic queue like any other session, but the queue UI shows no link back to the
originating backlog item. Backlog gate: `backlog_lifecycle.go` state-machine transitions
(`idea/ready/in_progress/review/done/archived`, currently a hard-coded `validTransitions` map
in `session/backlog.go` — see Step 11).

**(d) Reachable:** Yes for the backlog-specific gate (`GateVerdictBox` actions are real,
clickable). Partially for the generic queue — reachable, but not backlog-item-aware, so a
user watching the generic Review Queue page cannot tell which entries came from backlog work.

**(e) Status:** Generic review queue — implemented but not backlog-aware (partial
integration). Backlog-specific pre-merge review gate — implemented and automatic.

**(f) Autonomy:** The security-scan + LLM review that produces the gate verdict runs
**without human involvement** the moment the work session exits (`onSessionExited`
auto-triggers `spawnReviewGate`). The human-in-the-loop control point is entirely
*after* the fact: the user must Approve, Reopen, Override (with a typed reason), or
explicitly Skip the gate via `GateVerdictBox` before the item can reach `done`. Per **ADR-021**
(`docs/adr/021-review-queue-working-state-selector-join.md`, Accepted), the generic queue's
live status display was hardened to avoid the panel and session card showing divergent
states — a UX-correctness fix, not a safety gate.

---

## Step 10 — Completion / archive

**(a) UI entry point:** "Archive" button, `BacklogItemDetail.tsx:698-707`, shown when
`item.status === "done"`. A "Re-open to Review" button is shown alongside it
(`:708-716`, transitions back to `review`).

**(b) RPC:** `archiveBacklogItem(itemId)` (`useBacklogService.ts:282,407-413`) →
`BacklogService.ArchiveBacklogItem`.

**(c) Backend path:** `server/services/backlog_service.go:606-627` — soft-deletes via an
`archived_at` timestamp (not a hard delete; there is no `DeleteBacklogItem` RPC in the proto
at all, confirmed by a comment in `tests/e2e/backlog.spec.ts:441-442`).

**(d) Reachable:** Yes, purely via UI click.

**(e) Status:** Implemented and functional — despite `docs/registry/features/backlog.json`
(via `web-app/src/lib/features/features/backlog.ts`) labeling `backlog-archive-item` as
`"experimental"` with an empty `testIds` array. That registry entry is stale/inaccurate
relative to the actual code; the feature itself works and is wired into the UI.

**(f) Autonomy:** N/A — pure human action.

---

## Step 11 — Status event audit log

**(a) UI entry point:** "Workflow" section in `BacklogItemDetail.tsx:815-832` — a timeline
(`role="list"`, `aria-label="Status history"`) rendering each transition as
`fromStatus → toStatus`, a formatted timestamp, and a `· user` suffix when human-triggered.
**This is genuinely visible in the UI**, not backend-only.

**(b) RPC:** Implicit — returned as part of `GetBacklogItem` (`item.statusEvents`).

**(c) Backend path:** Ent schema `session/ent/schema/backlog_status_event.go:14-27` — fields
`id (uuid)`, `item_id`, `from_status`, `to_status`, `triggered_by`, `created_at`.

**(d) Reachable:** Yes, automatically populated on every transition; no user action required
to generate it, and it's visible without any extra click once an item has transitioned at
least once.

**(e) Status:** Implemented. Related: **ADR-013**
(`docs/adr/013-workflow-engine-replaces-valid-transitions.md`) proposes replacing the
current hard-coded `validTransitions` map (`session/backlog.go`) with an injectable
`WorkflowEngine` interface to support a new `refining` state and future custom states —
**this ADR's status is "Proposed," not "Accepted."** The state machine driving Step 11's
events today is still the older static map/`TransitionGuard` design, not the ADR's proposed
architecture.

**(f) Autonomy:** N/A — this is a passive audit trail, not an actor.

---

## Notes on the task's assumed references vs. what actually exists

The investigation brief referenced "ADR-013 review-gate-executor" — no ADR with that name
exists. The closest real artifacts are: **ADR-022** (headless triage replacing
`AutonomousDriver` for Step 6), **ADR-021** (review queue live-status selector join, Step 9),
and **ADR-013** (workflow-engine-replaces-valid-transitions, Proposed, Step 11) — three
different, unrelated ADRs. The task also assumed `server/features/backlog.go` contains the
`BacklogApprovePlan` RPC handler; it actually only contains static feature-registry metadata
(titles/descriptions/status) — the real handler is in `server/services/backlog_service.go`.

---

## What a user can actually do today

A user can turn the backlog feature on from Settings → Feature Flags with one click, see the
"Backlog" nav item appear, land on an empty board with a clear onboarding CTA, create an item
manually (title + priority, or the fuller form with description/AC/repo path), watch it
render on the board with a live status/priority badge, trigger AI triage on it (if a repo path
is set), review and selectively apply the AI's suggested acceptance criteria, approve the
resulting plan with one click, spawn either a supervised or fully autonomous AI session to
execute it while watching its output live inline, see an automatic post-execution review-gate
verdict appear with Approve/Reopen/Override/Skip actions, and finally archive the completed
item — with every status transition permanently logged and visible in a "Workflow" timeline on
the item itself. This is a substantially more complete single-item lifecycle than the
feature-registry JSON files (`docs/registry/features/backlog.json`,
`web-app/src/lib/features/features/backlog.ts`) suggest — most of those entries are marked
`"experimental"` with empty `testIds` and `componentPaths: []`, which is stale relative to the
actual, fairly polished `BacklogItemDetail.tsx` implementation.

## Where the journey breaks down / requires developer intervention

The entire "external source" half of the feature (Step 5) — the very thing that lets a
backlog exist without a human hand-typing every item — has no UI at all: creating a GitHub
`ItemSource`, and even manually forcing a sync, both require a raw ConnectRPC call from a
terminal, and the manual-sync RPC (`TriggerSync`) isn't even implemented server-side yet. A
developer must seed at least one `ItemSource` by hand before the otherwise-fully-automatic
15-minute sync loop does anything. Two secondary rough edges compound this: items created via
the polished empty-state CTA silently skip auto-triage because that mini-form never asks for
`repo_path` (no warning shown), and the generic Review Queue page — which is where a
backlog-spawned session's review request will actually surface alongside every other
session's — has no visible link back to the backlog item that spawned it, so a user relying
on that page alone would have to correlate sessions manually. E2E test coverage
(`tests/e2e/backlog.spec.ts`) stops at "Trigger Triage button is disabled when repoPath is
empty" — nothing in CI actually exercises a full triage completion, plan approval, autonomous
spawn, or review-gate resolution end-to-end, so regressions in that back half of the journey
would not be caught automatically today.
