# Research: Architecture — backlog-operator-feedback-loop

**Dimension**: Architecture (Agent 3) | **Phase**: 2 — Research

## 0. Summary (read this first)

- **Gap 3 (plan reject/request-changes): REVIVE, don't redesign.** ADR-001/ADR-002's design
  from `project_plans/plan-approval-ux/` is still architecturally sound against current
  `main`. Concrete proof, not just a read-through: `git cherry-pick --no-commit -n bc0955d41`
  (the orphaned implementation commit) applies **cleanly to 8 of 9 touched files**, with
  exactly **one** trivial conflict in `server/services/backlog_service_triage.go` — a
  same-line collision between bc0955d41's `PlanApproved`/`PlanRejectionReason` reset fields
  and an unrelated, later SDD-pipeline-mode change (`pap = filepath.Join(...)` for
  `DefaultSDDPipelineModeSlug`) to the exact same `update := session.BacklogItemUpdate{...}`
  literal. Both edits are additive to different fields of the same struct literal — a 2-minute
  manual merge, not a design rethink. See §1 for the full diff and why 111 commits of drift
  produced almost no real conflict.
- **Gap 1 (question→answer→retriage) needs *zero* backend/proto changes.** The existing
  `TriggerTriageRequest.feedback string` field and `TriggerTriage(feedback)` pipeline
  (`BuildHeadlessRetriagePrompt`) already accept exactly the shape an answered question
  produces — free text. The architecture is a pure frontend composition change: render an
  answer field per rendered question, and on submit, format `"Q: <question>\nA: <answer>"`
  (or a multi-question batch of the same) as the `feedback` string handed to the *existing*
  `triggerTriage(item.id, feedback)` call `TriageReviewPanel.tsx`'s `onRefine` already wires
  up. See §2.
- **Gap 2 (steer from backlog view) has two real architecture blockers requirements.md did
  not anticipate**, both confirmed by reading the actual send-path code, not inferred:
  1. The **existing UI Steer dialog** (`SessionActionsOverflow.tsx`) is not "general-session" at
     all — its menu item is gated `session.autonomousMode &&` (line 723) and its backing RPC
     (`UpdateSession.SteerMessage`, `session_service.go:2012-2016`) hard-rejects with
     `FailedPrecondition` on any non-autonomous session. Reusing it as-is would make the new
     backlog affordance silently useless for ordinary (non-autonomous) work/review sessions —
     the overwhelming majority of backlog sessions.
  2. **Headless triage sessions have no live `session.Instance` at all** — confirmed by an
     explicit, pre-existing code comment (`session/backlog.go:55-66`,
     `IsTmuxBackedSessionRole`): triage runs as "bounded one-shot headless subprocess calls...
     that exit on their own when the call returns, so they were never tracked as a live
     Instance in the first place." Both `steer_session`'s MCP-side `findInstance` and the RPC
     side's `UpdateSession` instance lookup resolve against the *same* `Instance`
     registry (`reviewQueuePoller.GetInstances()`/`LoadInstances()`) — neither can ever find a
     `headless-triage-*` session ID, because no `Instance` exists for it. This directly and
     conclusively answers **open question 3**: steering a headless triage session cannot work
     through either existing steer_session path, structurally, not as a bug to fix but as a
     property of how triage runs. AC6 as written ("a running triage, review, or work session...
     can be steered") is unsatisfiable for the triage case without inventing a new mechanism —
     which AC7 explicitly forbids. See §3.

## 1. Gap 3 — Plan Approval / Request Changes: revive-vs-redesign verdict

### 1.1 Method: don't just read the diff, try to land it

Rather than only diffing ADR text against current `main`, I ran the actual orphaned commit
against current `HEAD` as evidence:

```
$ git merge-base --is-ancestor bc0955d41 HEAD   # exit 1 — confirmed not merged
$ git rev-list --count bc0955d41..HEAD          # 111 — confirmed "111 commits behind"
$ git cherry-pick --no-commit -n bc0955d41
Auto-merging proto/session/v1/backlog.proto
Auto-merging server/services/backlog_service_lifecycle.go
Auto-merging server/services/backlog_service_test.go
Auto-merging server/services/backlog_service_triage.go
CONFLICT (content): Merge conflict in server/services/backlog_service_triage.go
Auto-merging session/backlog_lifecycle.go
Auto-merging session/backlog_lifecycle_stuck_test.go
Auto-merging session/backlog_lifecycle_test.go
Auto-merging session/repository.go
```

(Cherry-pick then aborted/reverted via `git restore --staged --worktree` — no changes land
in this worktree from this exercise; this was a research probe, not an implementation step.)

**8 of 9 touched files auto-merge cleanly** despite 111 commits of drift, including the ent
schema file (`session/ent/schema/backlog_item.go`), the proto file, and the lifecycle handler
file that would hold `RejectPlan`. That is strong, mechanical evidence — not inference — that
nothing structurally incompatible has landed on `main` since Aug 1: the surrounding code
`RejectPlan`/`ApprovePlan`/the ent schema/the repository mapping layer would need to touch
hasn't meaningfully changed shape.

### 1.2 The one real conflict — and why it's cosmetic, not architectural

```go
// server/services/backlog_service_triage.go, TriggerTriage's async completion write
pap := artifactAbsPath
<<<<<<< HEAD (current main, unrelated later change — SDD pipeline mode support)
if item.PipelineMode == session.DefaultSDDPipelineModeSlug {
    pap = filepath.Join(triageWorkDir, "project_plans", result.Title, "implementation")
}
update := session.BacklogItemUpdate{PlanArtifactsPath: &pap}
||||||| parent of bc0955d41
update := session.BacklogItemUpdate{PlanArtifactsPath: &pap}
=======
approvalReset := false
clearedReason := ""
setAt := time.Now()
update := session.BacklogItemUpdate{
    PlanArtifactsPath:   &pap,
    PlanApproved:        &approvalReset,
    PlanRejectionReason: &clearedReason,
    PlanArtifactsSetAt:  &setAt,
}
>>>>>>> bc0955d41
```

Current `main` added SDD-pipeline-mode-aware `pap` computation (a `plan.md` path fix, unrelated
to plan review) to the *same* `update := session.BacklogItemUpdate{...}` literal bc0955d41
also extends. Both changes are correct and additive to different concerns on the same struct
literal — the fix is to keep `main`'s conditional `pap` computation and layer bc0955d41's four
extra fields (`PlanApproved`, `PlanRejectionReason`, `PlanArtifactsSetAt`) onto it. This is
exactly the kind of "two features touched the same 5-line block" collision the codebase's own
task-sizing discipline (plan.md Story 2.4's sequencing note, quoted below) anticipates, not a
sign that the design has rotted:

> "a second uncoordinated edit to the same struct literal is exactly the kind of same-file
> collision the plan's '3-5 files' task-sizing discipline exists to surface, not hide"
> — `project_plans/plan-approval-ux/implementation/plan.md` Story 2.4

### 1.3 Confirmed: none of ADR-001/ADR-002's design exists on `main` today

Direct read of current `server/services/backlog_service_lifecycle.go:746-786` (`ApprovePlan`):
no `RejectPlan` handler anywhere in the file or package; `ApprovePlan` only sets
`PlanApproved`/`PlanApprovedAt`, no `expected_modified_at_unix_ms` freshness check, no
`PlanRejectionReason` clear. `grep -rn "RejectPlan\|plan_rejection_reason\|PlanRejectionReason"`
across the whole repo (excluding `gen/` and `project_plans/`) returns **zero matches**. Current
`proto/session/v1/backlog.proto` still has the original two-field `ApprovePlanRequest { string
item_id = 1; }` (line 464) and `TriggerTriageRequest { item_id; feedback; }` (line 452) exactly
as ADR-001/architecture.md described. This matches requirements.md's own "verified by full-tree
grep, zero matches" claim for Gap 3 — the feature genuinely was never merged, and no competing
implementation has appeared in its place.

### 1.4 Verdict for the plan phase

**Rebase, don't redesign.** Concretely:

1. Cherry-pick `bc0955d41` onto a fresh branch off current `main`.
2. Resolve the one conflict in `backlog_service_triage.go` by keeping both edits (SDD `pap`
   computation + the four reset/timestamp fields) in the same literal.
3. Re-run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
   ./session/ent/schema` per `.claude/rules/ent-schema-generation.md` (the commit's own ent
   codegen output is 111 commits stale and must be regenerated fresh against current schema,
   not reused verbatim — codegen output is not something to cherry-pick and trust blindly even
   though the *hand-written* schema field additions applied cleanly).
4. Re-run `make proto-gen` for the same reason on the generated proto Go/TS bindings.
5. Re-run the full `server/services` and `session` test suites bc0955d41 added
   (`TestRejectPlan_*`, `TestApprovePlan_ClearsExistingRejectionReason`,
   `TestTriggerTriage_RefineWithFeedback_ResetsPlanApproved`, etc.) to confirm they still pass
   against current `main`'s surrounding code, not just that the diff applied.
6. The **frontend half was never implemented** (bc0955d41 is backend-only per its own commit
   message: "Backend half of plan-approval-ux") — `PlanVerdictBox.tsx`,
   `derivePlanReviewStatus`, `ActionsSection.tsx` wiring, and the e2e test from
   `plan-approval-ux/implementation/plan.md` Epics 5-9 still need to be written from that
   plan's spec. This project's own AC3-5 (Approve + Request Changes side-by-side, empty
   rejection blocked, a distinguishable "changes requested" state visible in the item view) are
   almost verbatim ADR-001's 5-state `derivePlanReviewStatus` model — reuse Epics 5-7's
   component design rather than re-deriving a status model from scratch.
7. One scope trim available if Phase 3 wants it: bc0955d41 also includes
   `GetPlanArtifactContent` (rendering plan.md content in-browser) and the
   `expected_modified_at_unix_ms` optimistic-concurrency token. Neither is required by this
   project's stated ACs (1-8) — AC3-5 only need the reject/approve state machine, not
   in-browser plan rendering. These can ship as part of the rebase (they're already written and
   tested) or be deferred; either is a legitimate Phase 3 scope call, not an architecture
   blocker either way.

**No new ADR needed for Gap 3.** ADR-001 and ADR-002 remain the accepted design; the plan phase
should reference them directly (`project_plans/plan-approval-ux/decisions/ADR-001-*.md`,
`ADR-002-*.md`) rather than re-litigate the state-machine question research already closed out
five research documents, two ADRs, and a working implementation ago. The only genuinely new
decision this project's plan phase must make is the frontend wiring plan (which prior SDD run's
`implementation/plan.md` Epics 5-9 already specify in full) and whether to carry
`GetPlanArtifactContent` along (§7 above).

## 2. Gap 1 — Answer a triage question, trigger re-triage

### 2.1 Current state: read-only questions, no per-question identity

`TriageDiffSection.tsx:20-21,88-96` filters `triageResult.suggestions` for
`rationale === "question"` and renders each as inert text (`{q.text}`, keyed by array index —
`web-app/src/components/backlog/TriageDiffSection.tsx:91`). No answer input, no per-question
callback prop exists in `TriageDiffSectionProps` (`text`, `currentCriteria`,
`suggestedSuggestions` only).

**No stable per-question ID exists anywhere in the stack.** `TriageSuggestion` (proto and TS,
`useBacklogService.ts:37-40`) is `{text: string, rationale: string}` — nothing else. Each
triage run's `suggestions` array is a fresh LLM output with no identity that survives past that
one rendered result. This confirms requirements.md's own risk framing (open question 2,
pitfall §2): a durable "answered/unanswered" tracking model would need new schema; a **stateless
answer-at-submit-time** model needs none. Recommend stateless, per the requirements doc's own
default-if-not-proven-cheap guidance — nothing found in this research changes that call.

**A related, previously-unused field**: `TriageResult.clarifyingQuestions: string[]`
(`useBacklogService.ts:51`) is populated end-to-end by the backend
(`server/services/backlog_service.go:547-559`, deriving it from `suggestions[].rationale ==
"question"` for headless triage and reading it directly for MCP-submitted results) but is
**never read anywhere in the frontend** — `TriageDiffSection.tsx` and `TriageReviewPanel.tsx`
both independently re-derive the same question list by filtering `suggestions` a second and
third time. This is dead weight, not a blocker: `clarifyingQuestions` has the exact same
"just a string, no ID" shape as `suggestions[].rationale==="question"`, so it isn't a better
identity substrate — but the plan phase should consider having the new per-question UI read
`triageResult.clarifyingQuestions` (the field that's semantically "the" derived question list)
instead of adding a *fourth* independent `rationale === "question"` filter site. Minor DRY note,
not an architecture decision.

### 2.2 The existing feedback→retriage pipeline already accepts exactly this shape

Traced end to end, confirmed against current line numbers:

```
User submits answer(s)
  → (NEW) TriageDiffSection composes "Q: <question>\nA: <answer>" per answered question
  → (NEW, or reuse onRefine) parent calls triggerTriage(item.id, composedFeedback)
      — TriageReviewPanel.tsx's existing onRefine prop is this exact call shape already
        (handleRefineTriage → triggerTriage(item.id, feedback), BacklogItemDetail.tsx:787)
  → useBacklogService.ts:822 clientRef.current.triggerTriage({itemId, feedback})
  → TriggerTriageRequest.feedback (proto/session/v1/backlog.proto:452-457, unchanged)
  → TriggerTriage handler: feedback := strings.TrimSpace(req.Msg.Feedback)
    (server/services/backlog_service_triage.go — feedback-driven retriage branch, confirmed
    still present: "if feedback != "" { triagePrompt = session.BuildHeadlessRetriagePrompt(...) }")
  → BuildHeadlessRetriagePrompt (session/backlog_triage.go:117-162, confirmed current) embeds
    the feedback string verbatim under "## User feedback" in the LLM prompt — it has never
    parsed or structured this string; free text in, free text in the prompt. A "Q: ... / A: ..."
    block is indistinguishable to this code from any other feedback text a human might type.
```

**This means Gap 1 requires no proto change, no Go handler change, and no
`BuildHeadlessRetriagePrompt` change** — it is a pure frontend composition exercise sitting
entirely on top of the existing `feedback` field, exactly matching requirements.md's own
"Out of scope: Any change to `TriggerTriageRequest.feedback` semantics or
`BuildHeadlessRetriagePrompt` behavior" constraint and the "no new triage mechanism" AC2
constraint, literally: there is no new mechanism, only a new way of constructing the `feedback`
string handed to the *existing* mechanism.

### 2.3 What the frontend does need

- `TriageDiffSection.tsx`: render an answer input per question in `questionSuggestions` (a
  `<textarea>` or single-line `<input>`, keyed by index like the current read-only render), plus
  a way to signal "this question has a pending draft answer" to the parent.
- A new prop on `TriageDiffSectionProps` (or a lift of the answer-collection state up to
  `TriageReviewPanel`) to collect `{questionIndex, questionText, answerText}[]` and produce the
  composed feedback string on submit.
- A new action, distinct from the existing generic "Not quite — give feedback" button
  (`TriageReviewPanel.tsx:301-311`, `triage-refine-toggle-button`): AC1 asks for **submitting an
  answer to a specific rendered question without retyping the question text** — the composed
  feedback string must be built by the component (question text sourced from the already-loaded
  `TriageSuggestion.text`, never retyped by the operator), then handed to the same
  `triggerTriage` call the generic refine button already uses. Whether this is a "Submit Answer"
  button per-question (multiple individual retriage calls) or a single "Submit Answers &
  Re-triage" batch button collecting all answered questions into one `feedback` string is a
  Phase 3 UX/product call, not an architecture constraint — both compose the identical
  `feedback` string shape and hit the identical `triggerTriage` call. Batching is almost
  certainly the better UX (avoids spending N separate ~7-15 minute LLM calls for N questions)
  and costs nothing architecturally.
- `useBacklogService.ts`'s `triggerTriage(id, feedback?)` signature (line 589) needs **no
  change** — the composed string is just a `feedback` value like any other.

### 2.4 Registry / test implications (per `.claude/rules/feature-registry.md`)

No new RPC — `docs/registry/features/backend/backlog/trigger-triage.json` (already exists per
the plan-approval-ux research) should get its `testIds`/`lastModified` touched if new Go-side
retriage tests are added, but this is Gap 1's only backend-registry touch, and it's optional
(no Go behavior actually changes). The new frontend composition logic and per-question answer
UI is a genuinely new frontend feature needing a
`docs/registry/features/frontend/triage-question-answer.json`-style entry and a Playwright spec
per requirements.md AC8/`.claude/rules/e2e-test-conventions.md`.

## 3. Gap 2 — Steer a backlog-linked session from the item detail view

### 3.1 Two independent, pre-existing "steer" implementations (not one)

Requirements.md's framing ("The MCP tool and its UI... exist and work from general session
surfaces") undersells a real fork already in the codebase. There are **two structurally
different existing mechanisms**, not one feature with two entry points:

| | MCP tool `steer_session` | RPC `UpdateSession.steerMessage` |
|---|---|---|
| Entry point | `server/mcp/tools_terminal.go:137-149` (tool schema), `:627-706` (handler) | `server/services/session_service.go:2012-2033` |
| Caller | AI agents via MCP protocol (not reachable from browser JS) | Browser UI (`SessionActionsOverflow.tsx`'s "Give Direction" dialog) |
| Gate | None — works on any live `Instance` found via `findInstance` | **`if !instance.AutonomousMode { return FailedPrecondition }`** (line 2013-2016) |
| Send mechanism | `inst.RunWithResume` (OneShot + Stopped + has conversation UUID) → `claude --resume`; else `inst.SendKeys(msg + "\r")` via tmux pane | `controller.GetController().SendCommandImmediate(msg + "\r")` — a `ClaudeController` command-queue path, not the same primitive as `SendKeys` |
| UI visibility | N/A (agent-only) | Menu item only rendered `session.autonomousMode && onSteerAutonomousSession` (`SessionActionsOverflow.tsx:723`) — **never appears for a normal session today** |

This matters directly for AC7 ("uses the existing `steer_session` path — no parallel steering
implementation"): the browser cannot call the MCP tool at all (different transport), so "the
existing path" for a browser-originated Steer affordance can only mean the RPC path — but that
RPC, as it exists today, only works for autonomous-mode sessions, a narrow subset of what AC6
implies ("a running triage, review, or work session" — none of which are inherently
autonomous-mode). Reusing `UpdateSession.steerMessage` unmodified would make the new backlog
Steer button fail with `FailedPrecondition` for essentially every ordinary work/review session
an operator would actually want to steer.

**Recommendation**: extend `UpdateSession.steerMessage`'s handler
(`session_service.go:2012-2033`) to fall back to `instance.SendKeys(msg + "\r")` — the exact
primitive the MCP tool itself already falls back to for non-OneShot-resumable sessions
(`tools_terminal.go:684-706`) — when `!instance.AutonomousMode`, instead of hard-rejecting. This
is additive (existing autonomous-mode behavior via `SendCommandImmediate` is preserved
unchanged), reuses an already-proven send primitive (`Instance.SendKeys`,
`session/instance_tmux.go:628-634`) rather than inventing a new one, and does **not** touch the
general session list's UI at all — `SessionActionsOverflow.tsx:723`'s
`session.autonomousMode &&` gate on the *menu item* stays as-is, so "the general session list's
Steer dialog is unchanged" (requirements.md's own out-of-scope line) remains true even though
the underlying RPC becomes more permissive. This satisfies AC7's "no parallel implementation"
literally: one RPC, one send-primitive reuse, extended in scope rather than duplicated.

### 3.2 Headless triage sessions cannot be steered by either mechanism — structurally

This is the most consequential finding in this research pass. Read directly from the code, not
inferred:

```go
// session/backlog.go:55-66 (IsTmuxBackedSessionRole doc comment, current HEAD)
// Work and review sessions are tmux-backed. Triage sessions are not: they run
// as bounded one-shot headless subprocess calls (see headlessTriageUUIDPrefix) that
// exit on their own when the call returns, so they were never tracked as a live
// Instance in the first place and have nothing to kill...
func IsTmuxBackedSessionRole(role string) bool {
    return role == SessionRoleWork || role == SessionRoleReview
}
```

`TriggerTriage`'s async completion path (`server/services/backlog_service_triage.go:2344-2361`)
creates only a DB-only bookkeeping row (`s.storage.CreateItemSession(...)`, an `ItemSessionData`)
with `SessionUUID: headlessTriageUUIDPrefix + uuid.New()...` — it never registers a
`session.Instance` anywhere. The actual LLM call runs through
`session/headless.Pool.CallBlocking` (`session/headless/caller.go:487-493`), which drives a bare
`claude` subprocess via a process-runner abstraction — no tmux session, no PTY, no `Instance`
object at all.

Both existing steer paths resolve their target exclusively against the *live Instance registry*:

- MCP `steer_session` → `findInstance` (`tools_terminal.go:713-728`) → `th.live.FindLiveInstance`
  or `store.LoadInstances()`, both operating on `session.Instance`.
- RPC `UpdateSession` → `s.reviewQueuePoller.GetInstances()` /
  `s.loadInstancesWithWiring()` (`session_service.go:1836-1857`), also `session.Instance`.

A `headless-triage-*` session ID will never match an entry in either registry, because no
`Instance` was ever created for it. Both paths return "session not found" (or an equivalent
`connect.CodeNotFound`) for a triage session ID — not a fixable configuration gap, a consequence
of triage's one-shot-subprocess design that predates this project (confirmed via the code
comment's own framing, which already names this distinction for an unrelated concern — session
cleanup — meaning the triage/tmux-backed split is an established, load-bearing invariant
elsewhere in the codebase, not something this project can quietly special-case around).

The frontend already reflects this split structurally, independent of this project:
`classifySessionKind` (`web-app/src/lib/backlog/sessionKind.ts:17-33`) classifies any
`role === "triage"` or `headless-` prefixed session as `"headless_diagnostic"` — one of three
"Synthetic Session" kinds explicitly documented in that file's own comment: *"'work' and
'review' are Real Sessions (backed by an actual session.Instance/tmux/PTY); the other three are
Synthetic Sessions — DB-only rows with no backing Instance, no terminal, nothing to attach to."*
`SessionsSection.tsx:116-165` already renders triage rows as a non-interactive
`CollapsibleSection`/`SessionDiagnosticPanel`, never as the clickable `<a>` used for
work/review — there is no live target to attach a Steer button to even at the rendering level
today.

**Resolution — answers open question 3 directly**: steering a headless, prompt-driven triage
run does **not** behave the same as steering an interactive session, and cannot be made to
without a genuinely new mechanism (e.g., a side-channel file the headless prompt polls
mid-run, or killing and re-invoking the subprocess with amended input) — which would itself be
the "parallel steering implementation" AC7 forbids. **Recommend the plan phase narrow AC6 from
"triage, review, or work" to "review or work"** — i.e., restrict the new Steer affordance in
`SessionsSection.tsx` to `LinkedSession`s where `classifySessionKind(s)` is `"work"` or
`"review"` (the same predicate the component already uses to decide `<a>` vs. synthetic-row
rendering, `SessionsSection.tsx:116`). A triage session's "steering" equivalent is Gap 1's
answer→re-triage flow (cancel this run conceptually, start a new one with feedback) — a
structurally different capability already covered by this project's Gap 1, not `steer_session`.
This is not a scope cut imposed by this research; it is what the two already-existing
mechanisms are physically capable of.

### 3.3 `SessionsSection` doesn't have a full `Session` object either

Secondary, smaller blocker: `SessionActionsOverflow` (the existing Steer dialog component)
requires a full `Session` proto object as its `session` prop (`SessionActionsOverflowProps`,
`SessionActionsOverflow.tsx:35`) — `status`, `title`, `autonomousMode`, etc. `SessionsSection`
only has `LinkedSession` (`useBacklogService.ts:59-84`), a much lighter backlog-specific
projection (`entityId`, `sessionId`, `role`, timestamps, `reviewVerdict`, `triageResult`, cost,
worktree info) with none of those fields. Dropping `SessionActionsOverflow` into
`SessionsSection` unmodified is not a prop-compatible drop-in.

Two viable shapes for the plan phase, both consistent with AC7 ("no parallel steering
*implementation*" — this is about UI surface, not the RPC underneath):

1. **New, lighter dialog** purpose-built for the backlog surface (a `LinkedSession` → steer-only
   modal, mirroring `SessionActionsOverflow`'s existing Steer dialog markup/pattern at
   `SessionActionsOverflow.tsx:432-458` but without the other 90% of that component's menu
   surface it doesn't need) — calls the same `updateSession(sessionId, {steerMessage})` RPC via
   `useSessionService`. Smaller surface area, no dependency on fields `LinkedSession` doesn't
   carry.
2. **Fetch/attach the full `Session` object** per linked work/review session (e.g., via
   `useSessionService`'s existing session list/lookup, keyed by `sessionId`) and reuse
   `SessionActionsOverflow` wholesale. Heavier (N extra lookups or a join against an
   already-loaded session list) for a component whose other 90% of functionality (Restart,
   Delete, Clone, checkpoints, program-change, tag editing) isn't wanted on this surface anyway.

Recommend (1) — it is less code, has no coupling to `SessionActionsOverflow`'s much larger prop
surface, and matches the "lighter subset" framing already present in requirements.md's own
Gap 2 description ("the Steer dialog (or a lighter subset)").

## 4. Event-Command-Policy table (EventStorming)

Three actors — **Operator** (human), **Triage Agent** (headless LLM subprocess), **Plan-Review
State Machine** (system) — with feedback loops at each of the three gaps. Full grammar table
below; entries marked NEW are introduced by this project (all from Gap 3's revived design plus
Gap 1's frontend composition — Gap 2 introduces no new domain events, only a widened RPC gate).

| Domain Event | Policy (trigger condition) | Command | Actor / System |
|---|---|---|---|
| `TriageQuestionRendered` | `TriggerTriage`/`submit_triage_result` completed with ≥1 suggestion `rationale === "question"` | *(none — UI renders read-only today; NEW: renders an answer field per question)* | Triage Agent → Operator |
| **`TriageQuestionAnswered` (NEW, ephemeral — not persisted)** | Operator fills one or more per-question answer fields and submits | **(NEW, frontend-only) compose `feedback = "Q: ...\nA: ..."` per answered question** | Operator |
| `RetriageRequested` (existing, reused unchanged) | `TriageQuestionAnswered` composed a feedback string, OR operator used the pre-existing generic "give feedback" box | `TriggerTriage(item_id, feedback)` (`backlog_service_triage.go`, feedback-driven branch — unchanged) | Operator → Triage Agent (async) |
| `TriageRegenerated` (existing) | `RetriageRequested` completed | Async completion write: `PlanArtifactsPath` (+ NEW, if Gap 3 rebase lands: `PlanApproved=false`, `PlanRejectionReason=""`, `PlanArtifactsSetAt=now`) | System (triage completion handler) |
| `PlanPendingReview` *(implicit — no explicit persisted state, same gap plan-approval-ux's research already found)* | `TriageRegenerated`/first `TriageCompleted` fired | *(none — UI shows Approve/Request-Changes buttons)* | — |
| `PlanApproved` (existing) | Operator reviews plan, clicks Approve | `ApprovePlan` (+ NEW, from bc0955d41: clears `PlanRejectionReason`) | Operator |
| **`PlanChangesRequested` (NEW — revives bc0955d41/ADR-001)** | Operator reviews plan, finds it insufficient, supplies required non-empty reason text | **`RejectPlan(item_id, reason)` (revived RPC)** — persists `PlanRejectionReason`/`PlanRejectedAt`, clears `PlanApproved` at the same write site (closes the spawn-gate-disagreement bug bc0955d41's commit message calls out) | Operator |
| `PlanRevisionFeedbackReady` *(derived)* | `PlanChangesRequested` fired with non-empty reason | *(none automatically — UI surfaces the reason and a "Regenerate with this feedback" CTA that calls the existing `triggerTriage(id, reason)`, per ADR-002 — explicitly NOT auto-invoked)* | — |
| `SpawnBlockedByPlanGate` (existing, unchanged) | Spawn/dequeue attempted with `!SkipPlanning && !PlanApproved && !Autonomous` | `SpawnSessionFromItem`/`spawnSessionAfterGates` returns `FailedPrecondition` | System (gate check, `backlog_service_triage.go:438,656` — untouched by this project) |
| `SteerRequested` (existing RPC, widened scope — NOT a new command) | Operator clicks Steer on a `work`/`review`-classified `LinkedSession` row in `SessionsSection` | `UpdateSession(id, {steerMessage})`, handler widened (this project) to fall back to `Instance.SendKeys` when `!AutonomousMode`, instead of hard-rejecting | Operator → live `Instance` (tmux) |
| **`SteerRejectedNoLiveInstance` (NEW, structural — not a bug to fix)** | Operator attempts to steer a `role === "triage"` session (no `Instance` ever created for it) | `UpdateSession`/MCP `steer_session` both return not-found — **by design, per §3.2**; UI should never offer this action for triage rows in the first place (already true structurally via `classifySessionKind`) | System |
| `SessionSteered` (existing, unchanged send mechanism, new caller) | `SteerRequested` succeeds | `Instance.SendKeys(msg + "\r")` or `ClaudeController.SendCommandImmediate` (autonomous case, unchanged) | System → live agent process |

## 5. Recommendations for Phase 3 (plan)

1. **Gap 3**: rebase `bc0955d41` (see §1.4's 7-step checklist) rather than re-plan from
   scratch. ADR-001/ADR-002 stand as-is; cite them directly. Decide only: (a) carry
   `GetPlanArtifactContent`/optimistic-concurrency along or defer (§1.4 point 7), (b) which of
   `plan-approval-ux/implementation/plan.md`'s Epics 5-9 (frontend) to scope into this
   project's task breakdown verbatim vs. adapt.
2. **Gap 1**: no ADR needed — this is a pure frontend composition task sitting on the existing
   `feedback` field. Phase 3 should specify: per-question vs. batched submission UX (a product
   call, not architecture), and whether to route the new answer-collection state through
   `TriageDiffSection` locally or lift it to `TriageReviewPanel` (existing state-lifting pattern
   for `refineFeedback` at `TriageReviewPanel.tsx:85` is the template either way).
3. **Gap 2 / open question 3 — requires an explicit product decision before implementation**:
   confirm whether AC6 should be narrowed from "triage, review, or work" to "review or work"
   (this research's recommendation, backed by §3.2's structural finding) or whether the item
   should instead scope in a net-new "cancel and re-run triage with feedback" affordance for
   the triage case specifically — which is Gap 1's mechanism, not `steer_session`, and should
   not be built twice under two different names. Recommend an ADR capturing this decision given
   it directly resolves one of the four "open questions" the requirements doc flagged as needing
   Phase 2/3 resolution.
4. **Gap 2 backend change**: widen `UpdateSession.steerMessage`'s handler
   (`session_service.go:2012-2033`) to fall back to `Instance.SendKeys` for non-autonomous
   sessions instead of hard-rejecting (§3.1) — small, additive, no proto change needed (the
   field already exists). This is the one non-trivial backend change Gap 2 needs; everything
   else is new frontend surface calling an existing (now slightly widened) RPC.
5. **Gap 2 frontend**: build a small, purpose-built steer dialog scoped to `LinkedSession`
   (§3.3 option 1) rather than retrofitting `SessionActionsOverflow` to accept a lighter session
   shape — smaller diff, no risk to the existing general-session-list Steer UI.
