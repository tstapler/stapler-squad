# Research: Stack — Backlog Operator Feedback Loop

**Dimension**: Stack | **Phase**: 2 — Research

All findings below verified directly against current HEAD (`86700b173`) unless noted.
No new npm or Go dependency is needed for any of the three sub-features — every
sub-feature reuses an existing RPC, hook, component, or ent/proto convention already
in the codebase.

## Sub-feature 1: Per-question answer UI → `TriggerTriageRequest.feedback`

**Existing plumbing (reuse as-is, per Out of Scope):**
- `TriageSuggestion` (`web-app/src/lib/hooks/useBacklogService.ts:37-40`) is `{ text, rationale }`
  only — **no stable id field**. Questions are addressed today only by array position/exact
  text. `TriageDiffSection.tsx:87-97` renders them by mapping `questionSuggestions` with `key={i}`
  (array index), read-only, no answer affordance.
- `useBacklogService().triggerTriage(id, feedback?)` (`useBacklogService.ts:589,818-831`) →
  `clientRef.current.triggerTriage({ itemId, feedback })` — this is the ConnectRPC call Gap 1
  must land on (per Out of Scope: "no new triage mechanism is introduced").
- `feedback` is a **single opaque string**, not a structured Q&A payload
  (`proto/session/v1/backlog.proto:456-461`, `TriggerTriageRequest { item_id; feedback; }`).
  `server/services/backlog_service_triage.go:2295-2334`: non-empty `feedback` routes to
  `session.BuildHeadlessRetriagePrompt(item, artifactAbsPath, priorResult, feedback)` instead of
  a fresh triage prompt — requires a prior completed triage result to exist
  (`havePrior` check at line 2299).
- **Implication for the UI**: since there's no per-question id and `feedback` is one string,
  the per-question "answer" affordance is a client-side composition problem, not a new backend
  field — build the feedback string as `Q: <question.text>\nA: <answer>` (preserving the
  question↔answer link in the string itself, per AC1's "without retyping the question text")
  and call the existing `triggerTriage(id, composedFeedback)`. No proto change needed for this
  sub-feature's happy path.
- **Existing UI precedent to extend, not replace**: `TriageReviewPanel.tsx`'s `onRefine?:
  (feedback: string) => Promise<void>` prop (line ~48) is the existing "submit feedback → refine"
  callback wired from `BacklogItemDetail.tsx` (~line 721) straight to `triggerTriage`. The new
  per-question answer field is best added as a second call site into this same `onRefine`
  prop/callback (add an answer `<textarea>`/input per question row in `TriageDiffSection.tsx`'s
  `questionsSection` block, wire its submit to the same `onRefine`), not a new prop or a new RPC
  method.
- Field-registry note: `feedback` field number 2 on `TriggerTriageRequest` is unchanged; if
  Phase 3 architecture decides a structured payload IS warranted after all (e.g. to give
  questions stable ids), it would need a new `repeated QuestionAnswer answers = 3` field — flag
  to Architecture, not assumed here since the stack survey found no forcing requirement for it.

## Sub-feature 2: Exposing `steer_session` from `SessionsSection.tsx`

**Two distinct existing "steer" mechanisms — this is the load-bearing finding for this
sub-feature, and it directly resolves Open Question 3 in requirements.md as a hard blocker,
not just an "unverified" risk:**

1. **MCP tool `steer_session`** (`server/mcp/tools_terminal.go:627-706`) — works on *any* session
   type. Sends the message via `inst.SendKeys(message + "\r")` over the PTY (fallback path,
   `tools_terminal.go:683-706`), or via `inst.RunWithResume(...)` (`claude --resume` subprocess)
   if the session is a completed OneShot with a stored Claude conversation UUID
   (`tools_terminal.go:663-679`). **No autonomous-mode gate.**
2. **UI "Steer" dialog** (`SessionActionsOverflow.tsx:432-476,723-727`) — only rendered
   `{onSteerAutonomousSession && session.autonomousMode && (...)}` (line 723), i.e. **the menu
   item itself is hidden unless the session already has `autonomousMode: true`**. Its handler
   (`page.tsx:289-292`, `handleSteerAutonomousSession`) calls
   `updateSession(sessionId, { steerMessage: message })`, which is `UpdateSessionRequest.steer_message`
   (`proto/session/v1/session.proto:625-627`, field 11) — server-side
   (`server/services/session_service.go:2010-2013`) this **hard-fails**
   `connect.CodeFailedPrecondition` with `"steer_message can only be sent to sessions with
   autonomous_mode enabled"` if `!instance.AutonomousMode`.

**Consequence**: backlog-linked triage/review/work sessions (headless, prompt-driven,
`session.SessionTypeDirectory`-class sessions per `.claude/rules/session-creation-registry.md`)
are essentially never running with `autonomousMode: true` in the current pipeline (autonomous
mode is a separate, explicitly-opted-into user session mode — see `Omnibar.tsx`'s
`autonomousMode` toggle). **Wiring `SessionActionsOverflow`'s existing Steer dialog as-is into
`SessionsSection.tsx` would render the menu item but it would never appear for the sessions this
feature targets**, since `session.autonomousMode` will be false. Two ways to satisfy AC6/AC7
("uses the existing `steer_session` path — no parallel steering implementation") without
building a third mechanism:
  - (a) Have the backlog surface call the **MCP-tool-equivalent send-keys path** directly — i.e.
    a thin new UI-triggered RPC that does what `steer_session`'s PTY fallback does
    (`inst.SendKeys`), reusing that exact backend primitive rather than `UpdateSession.steer_message`.
    There is currently **no ConnectRPC-exposed equivalent of the MCP `send_keys`/`steer_session`
    tool** for arbitrary (non-autonomous) sessions — this is a real gap the Architecture
    dimension needs to size, not a UI-only wiring task.
  - (b) Relax the `SessionActionsOverflow` Steer dialog's visibility condition
    (`session.autonomousMode` check) and the backend's `FailedPrecondition` gate to also allow
    steering non-autonomous headless sessions via a different underlying send path selected by
    session kind — larger blast radius, touches a shared component used site-wide.
  Recommendation to Architecture: (a) is smaller blast radius (new handler reusing
  `inst.SendKeys`, doesn't touch the general session list's existing behavior) and matches "no
  parallel steering implementation" more literally (delegates to the same `Instance.SendKeys`
  primitive the MCP tool already uses) — but this is an architecture decision, not resolved here.
- **Type mismatch, not just a gating problem**: `SessionActionsOverflow` takes a full
  `Session` proto object (`session: Session` from `@/gen/session/v1/types_pb`,
  `SessionActionsOverflow.tsx:35`) with many fields (`autonomousMode`, `title`, etc.).
  `SessionsSection.tsx` only has `LinkedSession` (`useBacklogService.ts:59-78`) — a much
  lighter, backlog-specific shape (`entityId`, `sessionId`, `role`, timestamps, verdict,
  triage result, cost) with **no `autonomousMode` field at all**. Dropping
  `SessionActionsOverflow` into `SessionsSection.tsx` verbatim isn't possible without either
  (i) fetching the full `Session` object per linked session (extra RPC calls per row), or
  (ii) building a lighter Steer-only affordance that takes just `sessionId` + a message string
  and calls whatever new/adapted RPC sub-feature 2 lands on — likely (ii), since AC6 only asks
  for "can be steered," not "gets the full session overflow menu."

## Sub-feature 3: Plan reject / request-changes RPC + status

**Prior art is real, tested, and directly on-point — treat it as a strong starting draft, not
a blank slate, but it must be re-applied against current HEAD, not merged as-is (111 commits
stale, confirmed via `git rev-list --count bc0955d41..main` = 111).**

Commit `bc0955d41` (branches `recover/plan-approval-ux`, also present as
`backlog/stapler-squad-plan-approval-ux` / `fix/github-issue-import-and-plan-approval-ux` on
`origin`/`mainrepo`) implements almost exactly Gap 3, verified via `git show --stat bc0955d41`:

- **New `BacklogItem` proto fields** (would need field numbers reassigned — current HEAD's
  `BacklogItem` message already runs through field `34` for unrelated fields added since;
  verify highest in-use number at implementation time rather than reusing `33`/`34` verbatim):
  `plan_rejection_reason` (string), `plan_rejected_at` (`google.protobuf.Timestamp`).
- **`RejectPlanRequest { item_id; reason; expected_modified_at_unix_ms }` /
  `RejectPlanResponse { item }`** — exact shape independently corroborated by this repo's own
  proto conventions (see prior `plan-approval-ux/research/stack.md` Q4, written before this
  commit was known to this session, arriving at the identical `RejectPlanRequest{item_id,
  reason}` shape from first principles). The commit adds one more field this survey's earlier
  draft didn't have: `expected_modified_at_unix_ms` — an optimistic-concurrency token echoed
  from a new `GetPlanArtifactContentResponse.modified_at_unix_ms`, checked via
  `checkPlanArtifactFreshness` (fails closed on stat errors) so a reviewer can't approve/reject
  plan content that was silently regenerated since their tab last loaded it. This addresses a
  real race (AC5-equivalent) and is worth carrying forward even though it's not explicitly
  called out in this item's requirements.md.
- **`GetPlanArtifactContent` RPC** — reads `plan.md`/`requirements.md`/`validation.md`/
  `research/*.md` server-side with an allowlisted-filename guard, returns on-disk mtime. This is
  the "new backend read path" the plan-approval-ux stack research (Q1) independently flagged as
  a gate on rendering plan content in the browser at all — confirms it's a real, already-solved
  gap, not speculative.
- **Backend handler pattern**: `server/services/backlog_service_lifecycle.go`'s `ApprovePlan`
  shape (nil-storage guard → `GetBacklogItem` + `ent.IsNotFound` mapping → precondition checks →
  `session.BacklogItemUpdate` partial update → `backlogItemToProto` response) — `RejectPlan`
  mirrors it exactly; use the same shape, same `// +api: backlog:reject-plan` marker convention.
  Also touches `TriggerTriage`'s regeneration-completion write (resets `plan_approved` and
  clears `plan_rejection_reason` on a fresh plan) and widens `reconcilePlanNotApprovedItems`
  stuck-detection to also flag stale `ready`-status items via a new `plan_artifacts_set_at`
  timestamp field — both are real, non-obvious pieces of "keep state consistent" logic worth
  porting, not just the two new RPCs.
- **ent schema**: new fields `plan_rejection_reason`, `plan_rejected_at`,
  `plan_artifacts_set_at` on `session/ent/schema/backlog_item.go` — regenerate with
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  per `.claude/rules/ent-schema-generation.md` (non-negotiable flag).
- **Porting plan for whoever implements this** (Architecture/Plan dimension, not decided here):
  `git show bc0955d41` (or `git diff main...bc0955d41` scoped to the touched files) as a
  reference/starting diff, but re-derive it against current HEAD file-by-file — do not attempt a
  raw `git cherry-pick`/merge of a 111-commit-stale branch. Files touched (from `git show --stat`):
  `proto/session/v1/backlog.proto`, `gen/proto/go/session/v1/sessionv1connect/backlog.connect.go`
  (regenerate via `make proto-gen`, don't hand-port generated code),
  `server/services/backlog_service.go`, plus (per commit message) lifecycle handler and test
  files not shown in the truncated stat above — re-grep the full commit for the complete file
  list at implementation time.

## Frontend UI pattern for Approve/Request-Changes pairing (AC3)

No existing "paired action buttons with one requiring a text field" component was found beyond
the ad hoc `onApply`/`onSkip`/`onRefine` button row already in `TriageReviewPanel.tsx` — that's
the closest and only precedent (Approve-style `onApply` next to a feedback-gated `onRefine`).
`BacklogItemDetail.tsx:578`'s `ApprovePlan` UI call site is the other place a new "Request
Changes" button belongs beside it. No new component library is needed; this is a same-file
addition following the existing button-row + conditional-textarea-on-click pattern already used
for `onRefine`.

## Recommendation Summary

- **No new npm/Go dependency for any of the 3 sub-features.**
- **Sub-feature 1**: reuse `triggerTriage(id, feedback)` as-is; compose the `Q:/A:` string
  client-side in `TriageDiffSection.tsx`, wire through `TriageReviewPanel`'s existing `onRefine`
  prop. No proto change required for the happy path.
- **Sub-feature 2**: the real finding is that the two existing "steer" mechanisms are not
  interchangeable — the UI dialog's `steer_message` path is hard-gated to
  `autonomousMode: true` sessions server-side (`connect.CodeFailedPrecondition` otherwise),
  while backlog-linked sessions won't have that flag set. Reusing `SessionActionsOverflow`
  verbatim will silently never show the Steer option for backlog sessions. Recommend the
  Architecture dimension size a thin new RPC path that calls the same `Instance.SendKeys`
  primitive the MCP `steer_session` tool's PTY-fallback branch already uses, rather than trying
  to route through `UpdateSession.steer_message`. Also note `LinkedSession` lacks the fields
  `SessionActionsOverflow` needs — a lighter, backlog-scoped Steer affordance (sessionId +
  message only) fits better than embedding the full overflow menu.
- **Sub-feature 3**: don't design from scratch — commit `bc0955d41` on `recover/plan-approval-ux`
  is a complete, tested reference implementation of almost exactly this gap (`RejectPlanRequest
  {item_id, reason, expected_modified_at_unix_ms}` / `RejectPlanResponse{item}`, plus a
  `GetPlanArtifactContent` RPC needed to render plan content at all, plus ent fields
  `plan_rejection_reason`/`plan_rejected_at`/`plan_artifacts_set_at`). It is 111 commits behind
  main and must be re-applied by hand against current file states (particularly proto field
  numbering — current `BacklogItem` message already uses field 34 for something else added
  since), not merged/cherry-picked directly. Also read `project_plans/plan-approval-ux/research/`
  and `decisions/ADR-*.md` in full during Phase 3 planning — that project's ADRs (referenced as
  "see ADR-002" in the commit's `RejectPlanRequest.reason` doc comment) already resolved the
  "does RejectPlan trigger regeneration" design question this item's Open Question 1 restates.
