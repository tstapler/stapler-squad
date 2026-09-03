# Research: Architecture — Plan Approval UX

**Dimension**: Architecture | **Phase**: 2 — Research

## 1. Current State Model (ground truth, with file:line)

### Data fields

| Field | ent schema | Domain struct | Partial-update struct |
|---|---|---|---|
| `plan_approved` (bool, default false) | `session/ent/schema/backlog_item.go:55-56` | `BacklogItemData.PlanApproved` (`session/repository.go:392`) | `BacklogItemUpdate.PlanApproved *bool` (`session/repository.go:520`) |
| `plan_approved_at` (nillable time) | `backlog_item.go:57-59` | `BacklogItemData.PlanApprovedAt` (`repository.go:393`) | `BacklogItemUpdate.PlanApprovedAt *time.Time` (`repository.go:521`) |
| `plan_artifacts_path` (optional string) | `backlog_item.go:67-68` | `BacklogItemData.PlanArtifactsPath` (`repository.go:394`) | `BacklogItemUpdate.PlanArtifactsPath *string` (`repository.go:522`) |
| `skip_planning` (bool, default false) | `backlog_item.go:41-42` | `BacklogItemData.SkipPlanning` (`repository.go:355`) | `BacklogItemUpdate.SkipPlanning *bool` (`repository.go:506`) |

**There is no `plan_rejected` / `plan_rejection_reason` / `plan_status` field anywhere** — only a boolean `plan_approved`. Today's state space is binary (approved / not-approved), not the four states (no plan / pending / approved / changes-requested) success-criterion 1 asks for. This is the central schema gap: a new field is required, either a `plan_rejected_reason string` + reuse of `plan_approved=false` for "changes requested", or a proper `plan_status` enum (`none|pending|approved|changes_requested`) replacing/augmenting the current bool. The plan phase must choose one — the "reuse existing bool + new reason field" path is backward compatible (existing `ApprovePlanRequest{item_id}` callers keep working) and matches the repo's own additive-migration convention seen in `pipeline_mode`/`category`'s doc comments (`repository.go:370-391`).

Persistence: `session/ent_repository_backlog.go` maps all four fields both on `Create` (mapping domain → ent, lines 285-289) and `Update` (partial-update presence checks, lines 593-606 domain-mapper, 696-708 second mapper — there are two distinct write paths in this file, both must be touched for any new field).

### Gate enforcement — corrects a claim in requirements.md

Requirements.md states the gate "only blocks `DequeueNextQueuedItems`." **This is not quite accurate** — the raw gate check is applied consistently at **two** call sites, both funneling through the same helper:

1. `server/services/backlog_service_triage.go:438` — `SpawnSessionFromItem` (direct/user-initiated spawn):
   ```go
   if !isReopen && !item.SkipPlanning && !item.PlanApproved && !req.Msg.Autonomous {
   ```
2. `server/services/backlog_service_triage.go:656` — `spawnSessionAfterGates`, the shared helper called both by `SpawnSessionFromItem` (line 463) **and** by `DequeueNextQueuedItems` (line 614):
   ```go
   if !item.SkipPlanning && !item.PlanApproved && !autonomous {
   ```

So an item cannot get a work session spawned — whether by direct click or by dequeue — without `PlanApproved || SkipPlanning || Autonomous`. The gate itself is uniform.

**What is actually inconsistent** is *stuck-item detection/notification*, not gate enforcement: `reconcilePlanNotApprovedItems` (`session/backlog_lifecycle.go:2521-2568`) only lists items with `Statuses: []string{BacklogStatusQueued}` (line 2522-2524) — an item sitting in `ready` status with an unapproved plan that nobody ever tries to spawn (no concurrency-cap collision, so it never reaches `queued`) gets **no staleness detection, no `StuckReasonPlanNotApproved` mark, and no notification at all**. Only items that happen to collide with the concurrency cap and get queued are proactively flagged. This is the real "gate feels decorative" gap — silence, not bypass — and should be the framing carried into the plan phase rather than "the gate doesn't apply outside dequeue."

Bypass paths (both call sites, by design, not a bug):
- `isReopen == true` (line 438 only) — re-opening a reviewed item for revision skips planning re-approval, since the item already passed through Ready once.
- `SkipPlanning == true` — explicit per-item opt-out.
- `Autonomous == true` — `AutonomousDriver` runs its own planning loop; a human never reviews a plan for autonomous items by design (`backlog_service_triage.go:432-434`, `:2101-2104`).

### Registry status

`docs/registry/features/backend/backlog/approve-plan.json` and `.../trigger-triage.json` already exist — confirms `.claude/rules/feature-registry.md` was followed for the existing RPCs. Any new RPC (`RejectPlan`, a plan-content-read RPC) needs its own new per-feature JSON file there; any new frontend feature (rejection UI, markdown viewer) needs a `docs/registry/features/frontend/*.json` entry. Standard `make registry-generate` afterward.

### Session-creation-registry — not applicable, but adjacent

`.claude/rules/session-creation-registry.md`'s 7 touchpoints govern `SessionType` / `CreateSessionRequest` — plan approval doesn't add a session creation mode, so none of the 7 touchpoints apply directly. The one adjacent connection: the planning gate (`session_service.go`... actually `backlog_service_triage.go:438,656`) is a *precondition* checked before `SpawnSessionFromItem` reaches session creation, not a session-creation mode itself — no change needed there unless the plan phase decides rejection should also block `Autonomous` spawns (it currently doesn't, and nothing in requirements.md asks it to).

## 2. Frontend integration points

| Concern | File:line | Current behavior |
|---|---|---|
| Approve button | `web-app/src/components/backlog/detail/ActionsSection.tsx:160-170` (ready status), `:176-187` (queued status) | Renders only when `item.planArtifactsPath && !item.planApproved`; no visible state once approved — the button simply disappears, exactly as requirements.md describes. `canSpawnSession` derivation at `ActionsSection.tsx:59-61`. |
| Plan artifacts display | `web-app/src/components/backlog/detail/PlanArtifactsSection.tsx:13-23` | Renders `item.planArtifactsPath` as inert `<code>` text inside a `CollapsibleSection`; returns `null` entirely if the path is empty (line 14). No fetch, no content rendering. |
| RPC call | `web-app/src/lib/hooks/useBacklogService.ts:771-780` (`approvePlan`), `:744-770` (`triggerTriage(id, feedback?)`) | `approvePlan(id)` takes no reason parameter — matches the proto's `ApprovePlanRequest{item_id}` (single field). `triggerTriage` already threads an optional `feedback` string through to the RPC (line 748). |
| Action dispatch | `web-app/src/components/backlog/BacklogItemDetail.tsx:513-514` | `case "approve_plan": await approvePlan(item.id);` — no reject case exists. |
| Existing "feedback textbox + submit" UI precedent | `web-app/src/components/backlog/GateVerdictBox.tsx:33-34,110-123,175-178,384-390` | `onReopen(feedback: string)` renders a labeled `<textarea id="reopen-feedback">`, focuses it on open, and calls the handler with the trimmed text on submit. This is the closest existing UI pattern to copy for a "Reject Plan / Request Changes" affordance — same shape (button → inline form → textarea → submit), just a new instance keyed to the plan-approval action instead of the review-gate action. |
| Existing "feedback → next AI run" wiring precedent | `web-app/src/components/backlog/BacklogItemDetail.tsx:702-727` (`handleRetriggerTriage`, `handleRefineTriage`) | `handleRefineTriage(feedback)` calls `triggerTriage(item.id, feedback)` directly — feedback flows as a first-class RPC field. Contrast with `handleGateReopen` (`BacklogItemDetail.tsx:805-819`), which instead **appends the feedback as a freeform timestamped note** (`[Revision feedback <ts>]\n<feedback>`) onto `item.notes` rather than passing it as a structured RPC field. Two different existing conventions for "how does user feedback reach the AI" — see §3 below for which one plan-rejection should follow. |

## 3. Data flow: how does feedback reach the next AI run? (two existing precedents)

### Precedent A — structured RPC field (triage refinement): the stronger template

```
User types feedback in TriageReviewPanel
  → handleRefineTriage(feedback)                         BacklogItemDetail.tsx:720-727
  → triggerTriage(item.id, feedback)                      useBacklogService.ts:744-770
  → clientRef.current.triggerTriage({itemId, feedback})   → RPC TriggerTriageRequest.feedback (backlog.proto:432-435)
  → TriggerTriage handler:
      feedback := strings.TrimSpace(req.Msg.Feedback)                    backlog_service_triage.go:1911
      priorResult, havePrior := findPriorTriageResult(existingSessions)  backlog_service_triage.go:1912
      // rejects if feedback given but no prior completed triage exists  backlog_service_triage.go:1913-1916
      nextIteration := priorResult.Iteration + 1                        backlog_service_triage.go:1917
      triagePrompt = session.BuildHeadlessRetriagePrompt(
          item, artifactAbsPath, priorResult, feedback)                 backlog_service_triage.go:1946-1951
  → BuildHeadlessRetriagePrompt (session/backlog_triage.go:104-144) embeds
    the feedback verbatim into the LLM prompt under a "## User feedback"
    heading (line 130-133), instructs the LLM to revise plan.md/validation.md
    in place using the existing research as-is (unless feedback flags the
    research itself as wrong, in which case only the affected research file
    is rewritten — lines 139-143)
  → headlessPool.CallBlocking(...) runs the LLM headlessly against
    itemRepoPath (NOT artifactAbsPath — the working dir is the repo, the
    artifact dir is passed as a prompt-embedded path the LLM reads/writes
    directly)                                                            backlog_service_triage.go:2006-2024
  → result.Feedback = feedback; result.Iteration = iteration persisted
    onto the new ItemSession's triage_result JSON                        backlog_service_triage.go:2060-2061,2075
  → plan_artifacts_path is re-set to the same artifactAbsPath (unchanged —
    files are revised in place, not moved) and status idea→ready          backlog_service_triage.go:2080-2095
```

This is the **template to replicate for plan-level feedback**: a structured `feedback`/`reason` field on the request, persisted onto a result record (iteration history), fed into a purpose-built prompt-builder function analogous to `BuildHeadlessRetriagePrompt`, consumed by the *same* `TriggerTriage` → headless-LLM → artifact-rewrite pipeline (no new pipeline needed — `TriggerTriage(feedback)` already **is** "regenerate the plan with feedback"). A `RejectPlan(item_id, reason)` RPC does **not** need to itself call the LLM; it should (a) persist the rejection state + reason, and (b) either (i) leave the actual regeneration to the user's next manual "Trigger Triage" click (reason surfaced in the UI to be copy/pasted or auto-populated into the existing feedback box), or (ii) auto-invoke `TriggerTriage(feedback: reason)` server-side as part of `RejectPlan`. Sizing/deciding (i) vs (ii) is a plan-phase call — (i) is lower-risk (reuses `TriggerTriage` unchanged, no new LLM-invocation code path) and keeps the "user stays in control of when an LLM call happens" property TriggerTriage already has (it is never auto-triggered without an explicit user action *except* `AutoSpawnSession`, which is opt-in). (ii) is more convenient but couples `RejectPlan`'s request lifecycle to a 7-30 minute LLM call (see `triageCallBudget`, referenced at `backlog_service_triage.go:2002`) — `RejectPlan` would need the exact same async-goroutine-with-in-flight-guard shape `TriggerTriage` already has (`triageInFlight` map, `backlog_service_triage.go:1884-1893`), which is substantial complexity to duplicate versus just calling `TriggerTriage` internally.

### Precedent B — freeform note append (gate reopen): weaker, avoid for this feature

`handleGateReopen` (`BacklogItemDetail.tsx:805-819`) appends `[Revision feedback <timestamp>]\n<feedback>` onto `item.notes` via `updateBacklogItem`, then transitions status and spawns a fresh work session that presumably reads `notes` as part of its prompt context. This pattern loses structure (no iteration count, no queryable "reason for this specific rejection," mixed in with all other notes text) and doesn't fit the plan-approval case, where the artifact-rewrite pipeline (`TriggerTriage`) already has a clean structured-field slot (`feedback`) purpose-built for exactly this. **Recommendation: do not use the notes-append pattern for plan rejection — follow Precedent A.**

## 4. New capability: reading plan artifact *content* into the browser

Confirmed (cross-checking `stack.md`'s finding): the existing `GetFileContent` RPC (`proto/session/v1/session.proto:229-232`, request shape `session.proto:1586-1592`) is **scoped to a session's worktree**, not to a backlog item's plan-artifacts directory:

```go
// server/services/file_service.go:293-303
ws, err := fs.workspace.GetWorkspace(req.Msg.SessionId)   // requires a live session
basePath := ws.EffectivePath
fullPath, err := resolveAndValidatePath(basePath, req.Msg.Path)  // path-traversal guard, file_service.go:106-115
```

Plan artifacts live under `~/.stapler-squad/triage-artifacts/<item-id>/` (`triageBase`/`artifactAbsPath`, `backlog_service_triage.go:1921-1926`), which is **not** a session workspace — the triage headless LLM call's `WorkDir` is set to `itemRepoPath` (the repo, `backlog_service_triage.go:2023`), not the artifact dir, and by the time a user is reviewing the plan the triage `ItemSession` may have long since ended (no live workspace to resolve via `GetWorkspace` at all). `GetFileContent` genuinely cannot serve `plan.md` today.

**Required new backend capability**: either (a) a new RPC, e.g. `GetPlanArtifactContent(item_id, filename) → {content, ...}` resolving against `item.PlanArtifactsPath` directly (mirroring `resolveAndValidatePath`'s traversal guard, reusing the same binary/size/truncation handling already in `file_service.go`'s `GetFileContent`), or (b) extending `GetFileContentRequest` with an alternate `item_id` + implicit base-path-from-`PlanArtifactsPath` resolution mode instead of always requiring `session_id`. (a) is cleaner — it keeps `GetFileContent`'s contract ("always a live session workspace") intact and avoids adding a second, mutually-exclusive addressing mode to one request message. This is squarely an Architecture-dimension finding (as stack.md already flagged) and blocks Success Criterion 4 regardless of which markdown-rendering library choice is made.

## 5. Event-Command-Policy table (EventStorming)

This domain has multiple actors (User, headless-LLM system, periodic reconciliation policy) and multiple state transitions with feedback loops — not simple CRUD — so a full table is warranted.

| Domain Event | Policy (trigger condition) | Command | Actor / System |
|---|---|---|---|
| `TriageCompleted` (plan.md written, `plan_artifacts_path` set, status `idea`→`ready`) | User clicked "Trigger Triage" on an item with no/stale plan | `TriggerTriage` (fresh, `feedback=""`) | User → headless LLM (async) |
| `PlanPendingReview` *(implicit today — no explicit event/state, just `PlanApproved=false` + non-empty `PlanArtifactsPath`)* | `TriageCompleted` fired | *(none — UI shows Approve button; no auto-notify)* | — |
| `PlanApproved` (`plan_approved=true`, `plan_approved_at=now`) | User reviewed plan content, clicked Approve | `ApprovePlan` (`backlog_service_lifecycle.go:617-657`) | User |
| **`PlanRejected` (NEW — no current event/field)** | User reviewed plan content, found it insufficient | **`RejectPlan(item_id, reason)` (NEW RPC)** | User |
| `PlanRejectedFeedbackReady` *(NEW, derived)* | `PlanRejected` fired with non-empty reason | *(none automatically — surfaces reason in UI, pre-fills next Trigger Triage's feedback box per §3 option (i))* | — |
| `PlanRegenerated` (plan.md rewritten in place, `iteration+1`, feedback persisted onto `TriageResult`) | User (re-)clicked Trigger Triage, optionally carrying rejection reason as `feedback` | `TriggerTriage(feedback)` (existing, `backlog_service_triage.go:1840-2120`) | User → headless LLM (async) |
| **`ApprovalInvalidated` (NEW — does not exist today, see §6 consistency gap)** | `PlanRegenerated` fired while `plan_approved` was still `true` from a prior approval | **Should be**: clear `plan_approved`/`plan_approved_at` as part of the same update at `backlog_service_triage.go:2080-2086` | System (triage completion handler) |
| `SpawnBlockedByPlanGate` | Spawn/dequeue attempted with `!SkipPlanning && !PlanApproved && !Autonomous` | `SpawnSessionFromItem` / `spawnSessionAfterGates` returns `CodeFailedPrecondition` | System (gate check) |
| `ItemQueuedWithUnapprovedPlan` (marked stuck, `StuckReasonPlanNotApproved`, notified) | Item in `queued` status, `!SkipPlanning && !PlanApproved`, `QueuedAt` older than `planApprovalStaleness` (5 min) | `MarkStuck` + `notify` (`session/backlog_lifecycle.go:2521-2567`) | System (periodic `reconcilePlanNotApprovedItems`, runs only over `queued` items — see §1 gap) |
| `PlanApprovalGateBypassed` | `Autonomous=true` or `SkipPlanning=true` or (spawn path only) `isReopen=true` | — (no gate command invoked) | User (opts in) / System (`AutonomousDriver`) |

## 6. Consistency requirements

### 6a. Does `PlanApproved` auto-reset on regeneration? — **No, and this is a real gap**

Confirmed by reading the triage-completion write path directly: `backlog_service_triage.go:2080-2086` only updates `PlanArtifactsPath` (re-set to the unchanged directory) and applies AC updates (`applyTriageACToUpdate`) — it **never touches `PlanApproved`/`PlanApprovedAt`**. Concretely: if a user approves a plan, then later re-triggers triage with feedback (refining an already-approved plan — nothing today prevents this; the gate only blocks *spawning*, not *re-triaging*), `plan_approved` silently stays `true` even though `plan.md`'s content just changed underneath the stored approval. The gate (`!item.PlanApproved`) would then pass a spawn against content the user never actually reviewed in its current form. This is a genuine correctness gap independent of anything requirements.md flagged directly (requirements.md's pitfalls section asks about line-comment staleness, but this is a coarser, more fundamental staleness: the approval boolean itself outlives the content it approved).

**Recommendation for the plan phase**: when `TriggerTriage` completes a feedback-driven refine (`feedback != ""`) — or arguably any regeneration, fresh or refined, though a fresh first-triage can't have a stale approval since `PlanApproved` starts `false` — clear `PlanApproved`/`PlanApprovedAt` as part of the same `UpdateBacklogItem` call at `backlog_service_triage.go:2081`. This is a one-line addition (`update.PlanApproved = &falseVal` style, mirroring the existing pointer-based partial-update convention) once the plan phase confirms the product decision ("does refining an approved plan un-approve it?" — the architecturally correct answer is yes, matching the "approval is a claim about specific content" semantics the field's own name implies).

### 6b. Should a rejection reason auto-clear once addressed? — plan-phase product decision, two viable designs

Two structurally different designs, both consistent with the codebase's existing conventions:

1. **Ephemeral reason** (mirrors `TriggerTriageRequest.feedback` itself, which is a one-shot input, not stored as an ongoing field — it's persisted only as a historical record inside each `ItemSession.TriageResult.feedback`, `backlog.proto:62`). Under this design, `RejectPlan(reason)` writes the reason into a new `ItemSession`-scoped or status-event-scoped record (see `BacklogStatusEventData` — `session/repository.go`'s `StatusEvents` field, already used for status-transition history) rather than a durable `BacklogItemData` field. Once `TriggerTriage(feedback)` runs, the reason has done its job and there's nothing to "clear" — it's already historical. Simpler, and consistent with never needing a `plan_rejection_reason` column at all.
2. **Durable field until re-approval**: add `plan_rejection_reason string` (or similar) to `BacklogItemData`/ent schema, set by `RejectPlan`, and explicitly cleared by `ApprovePlan` (and per 6a, by the next successful regeneration). This makes "why was this rejected" visible in the UI even after the user has moved past the rejection dialog (e.g. shown as a persistent banner until the next approval), at the cost of one more nullable column and clear-on-two-different-events logic.

Given `PlanArtifactsSection`/`ActionsSection` currently have **no** persistent history surface at all for approval events (requirements.md point 1: "There is no timeline/history entry recording who/when approved it") — reusing the existing `StatusEvents`/status-event-timeline machinery (design 1, or a hybrid: ephemeral field + a timeline entry) is likely the better fit, since it solves Success Criterion "Approval/rejection history visible in the item's status/progress timeline" (Should Have) for free rather than requiring a second, separate persistent-reason field to also design a clearing policy for. Flag this explicitly to the plan phase as the two candidate designs to choose between, since it also determines whether `BacklogItemUpdate` needs a new pointer field at all (Constraint: "additive UX, not a breaking schema change unless research finds otherwise" — design 1 needs zero schema changes beyond what a `RejectPlan` RPC's own status-event write already requires).

## 7. Summary of concrete integration points for the plan phase

- **New RPC**: `RejectPlan(item_id, reason)` in `proto/session/v1/backlog.proto` (alongside `ApprovePlanRequest`/`Response` at lines 442-448) + handler in `server/services/backlog_service_lifecycle.go` (alongside `ApprovePlan` at lines 613-657) + registry entry.
- **New RPC**: plan-artifact content read (`GetPlanArtifactContent` or similar) — new proto message + `server/services/` handler reusing `resolveAndValidatePath`'s traversal-guard pattern from `file_service.go:106-115`, resolving against `item.PlanArtifactsPath` instead of a session workspace.
- **Schema decision** (plan phase must choose): reuse `plan_approved=false` + new ephemeral reason record (status-event-based) vs. new `plan_status` enum vs. new durable `plan_rejection_reason` column — see §6b.
- **Consistency fix**: clear `PlanApproved`/`PlanApprovedAt` inside the triage-completion write at `backlog_service_triage.go:2081` when regenerating a previously-approved plan (§6a) — this is a bug fix independent of new-feature scope and should probably ship regardless of which rejection-UX design is chosen.
- **Frontend**: `PlanArtifactsSection.tsx` needs a content-fetch (new RPC) + `react-markdown`/`remark-gfm` render (already-proven pattern at `DescriptionSection.tsx`, no new dependency — confirmed in `stack.md`). `ActionsSection.tsx` needs a persistent approval-state indicator (not just a vanishing button) and a reject affordance modeled on `GateVerdictBox`'s `onReopen` textarea pattern (`GateVerdictBox.tsx:384-390`). `useBacklogService.ts` needs a `rejectPlan(id, reason)` method mirroring `approvePlan`/`triggerTriage`'s existing shape.
- **Gate scope**: no change required to `backlog_service_triage.go:438`/`:656` gate logic itself (already uniform per §1) — but `reconcilePlanNotApprovedItems` (`session/backlog_lifecycle.go:2521`) should be reconsidered for scope-widening (currently `queued`-only) if the plan phase wants proactive staleness detection for `ready`-status items too, addressing the real "silent forever" gap identified in §1.
