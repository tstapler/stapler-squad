# Architecture Research: backlog-already-implemented

## 1. Current data/control flow (file:line trail)

**Work session emits evidence:**
1. `server/mcp/tools_backlog.go:187` `reportProgress` — per-criterion `note` written via `h.storage.UpdateAcCriterionStatus(ctx, itemID, criteriaIndex, acStatus, note)` (`tools_backlog.go:243`).
   - `UpdateAcCriterionStatus` (`session/storage_backlog.go:~625-664`) mutates **`BacklogItem.acceptance_criteria`** (the live item JSON, `criteria[criterionIndex].Note = note` at `storage_backlog.go:654`) and saves via `BacklogItem.UpdateOneID(...).SetAcceptanceCriteria(...)`. It does **not** touch `ItemSession.ac_snapshot`.
2. `server/mcp/tools_backlog.go:254` `requestReview` — `verification_notes` (free text) persisted onto the **work session's own** `ItemSession` row via `h.storage.UpdateItemSessionVerificationNotes(ctx, itemSession.ID, verificationNotes)` (`tools_backlog.go:319`), where `itemSession` is looked up by `(callerUUID, itemID)` at `tools_backlog.go:287`. Then transitions item `in_progress → review` (`tools_backlog.go:310`).

**Session exit → review gate spawn:**
3. `session/backlog_lifecycle.go:370-411` (`EventExited`-triggered path) and `:414-449` (`TriggerReviewForSession`, used by autonomous driver) both re-fetch `item` via `GetBacklogItem` and `is` (`ItemSessionSummary`) via `GetItemSessionBySessionUUID` — **`is` is the work session's own row**, so `is.VerificationNotes` reflects step 2 (fresh) but `is.AcSnapshot` is whatever was captured **at work-session creation time**, before any `report_progress` calls happened during that session. Then call `l.spawnReviewGate(item, is)` (`:409`, `:448`) → `ReviewGateRunner.Run` (`session/review_gate.go:54`).

**Review gate: diff + prompt assembly:**
4. `session/review_gate.go:82` resolves the worktree via `r.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUUID)` → `wt.WorktreePath`, `wt.BaseCommitSHA`, `wt.BranchName` — **this is already available here**, before any LLM call.
5. `GetGitDiff`/`GetGitDiffRef` (`session/backlog_review.go:257-304`) compute the diff; auto-repair of a broken `base_commit_sha` at `review_gate.go:111-145`; explicit **block-with-FAIL-verdict** if diff computation still fails (`review_gate.go:150-199`) — this path already exists and is analogous to (but distinct from) the empty-diff case in scope here (empty diff ≠ diff computation error).
6. AC snapshot selection: `review_gate.go:236-240`:
   ```go
   acSnapshot, _ := ParseAcCriteria(is.AcSnapshot)
   if len(acSnapshot) == 0 {
       acSnapshot, _ = ParseAcCriteria(item.AcceptanceCriteria)
   }
   ```
   **`is.AcSnapshot` is almost always non-empty** (populated at session spawn), so this fallback to the live, note-bearing `item.AcceptanceCriteria` is effectively dead in the common case — see Finding A below.
7. `BuildReviewPrompt` / `BuildHeadlessReviewPrompt` (`session/backlog_review.go:66-191`) render title/description/AC list/plan/diff, then `writeVerificationEvidenceSection` (`:56-63`) appends `## Verification Evidence` from `verificationNotes` if non-empty. **AC criteria rendering (`:83-85`, `:152-153`) prints only `c.Index` and `c.Text` — `c.Note` is never rendered**, confirming the requirements doc's claim.
8. Empty diff renders literally `"(no diff available)"` (`backlog_review.go:107-108`, `:170-171`).

**Reviewer LLM call (headless path — the only path relevant here since `pool := r.getPool()` is checked first, `review_gate.go:244`):**
9. `pool.CallBlockingWithCost(reviewCtx, headless.FeatureKeyReview, headless.HeadlessReviewSystemPrompt(), headlessPrompt)` (`review_gate.go:252`) — **no `CallOptions`/`WorkDir` passed**, so the subprocess inherits the server process's own cwd, not the item's worktree.
10. `headlessReviewSystemPrompt` (`session/headless/features.go:84-87`) instructs JSON-only output; nothing in it tells the model to look at the codebase — it only knows the diff + optional Verification Evidence text.
11. `ParseHeadlessVerdictResult` (`backlog_review.go:200-224`) parses JSON; on empty diff the model has no basis for anything but UNVERIFIABLE per-criterion (or FAIL if it hallucinates), and `AggregateOutcome` (`session/domain/backlog.go:129-164`) has no PASS path unless every criterion is PASS.

**Verdict → auto-reopen loop:**
12. `review_gate.go:278` `applyVerdictsToACs` writes verdict outcomes back onto AC statuses.
13. `review_gate.go:307`: `overall == FAIL || PARTIAL || UNVERIFIABLE` → `reopener.AutoReopenAfterFailedReview(ctx, item.ID)` (async). No distinction between UNVERIFIABLE-because-empty-diff-but-actually-done and a real FAIL.
14. `server/services/backlog_service_triage.go:55` `maxAutoReworkIterations = 3` caps the loop; item gets parked/notified once exceeded (outside this file's scope per requirements — rework-cap changes are explicitly out of scope).

## 2. ent schema / domain struct — integration points, no schema change needed

- `AcCriterion` (`session/domain/backlog.go:66-71`) **already has** `Note string \`json:"note,omitempty"\`` (index/text/status/note) — this is pure JSON payload inside `BacklogItem.acceptance_criteria` (ent field `session/ent/schema/backlog_item.go:28-30`, `Optional().Comment("JSON []AcCriterion")`) and `ItemSession.ac_snapshot` (`session/ent/schema/item_session.go:33-35`, same JSON shape). **No new ent column is required** — `Note` is already round-tripped through both fields; the gap is purely that `BuildReviewPrompt`/`BuildHeadlessReviewPrompt` don't print it (Finding fixable in `backlog_review.go` only) **and** that the snapshot the reviewer actually sees (`is.AcSnapshot`) is stale relative to where `Note` gets written (`item.AcceptanceCriteria`) — see Finding A.
- `ItemSession.verification_notes` (`item_session.go:39-41`) is already a plain string field, already wired end-to-end (confirmed reaches the prompt via `writeVerificationEvidenceSection`). No schema change needed there either.
- **Confirmed: the "no schema changes" appetite constraint is achievable.** Everything needed (`Note`, `verification_notes`, `WorktreePath` via `GitWorktreeData`) already exists in the ent schema / domain structs. This is a Go-plumbing-and-prompt-only change.

### Finding A — the acceptance-criteria snapshot the reviewer sees is stale, and this blocks the `note` fix on its own
`review_gate.go:236-240`'s `is.AcSnapshot`-first, `item.AcceptanceCriteria`-fallback order means a `note` written mid-session via `report_progress` (which only updates `item.AcceptanceCriteria`, per §1 step 1) **will not reach the reviewer** even after `BuildReviewPrompt` is fixed to render `c.Note`, because `is.AcSnapshot` (captured at spawn, before the note existed) wins the fallback whenever it's non-empty — which is essentially always. Fixing prompt rendering alone is insufficient; the snapshot-selection logic in `review_gate.go:236-240` (and the equivalent path if any exists in `backlog_service_triage.go`'s `TriggerReReview`, `:830-841`) must prefer the live `item.AcceptanceCriteria` (or merge `Note` from it into whichever snapshot is chosen) so the reviewer actually sees what was self-reported. This is a small, targeted Go change, not a schema change — but it is a **required companion fix**, not optional, or the `note`-surfacing requirement silently fails in the common case.

## 3. Worktree access already resolved before the LLM call — WorkDir threading is the gap

- `ReviewGateRunner.Run` already resolves `wt.WorktreePath` via `r.storage.GetWorktreeDataBySessionUUID` at `review_gate.go:82`, well before the headless call at `:252`. The path is sitting right there in scope.
- `headless.CallOptions{WorkDir string}` (`session/headless/caller.go:18-25`) already exists and is plumbed through `ProcessRunner.WithWorkDir` (`session/headless/runner.go:54-58`) → `executor.WithProcessDir(r.workDir)` (`runner.go:96-97`) — i.e., the subprocess `cwd` is genuinely set to that directory when `CallWithOptions`/`CallBlockingWithOptions` is used.
- **This mechanism is already exercised in production for a different feature**: `server/services/backlog_service_triage.go:671-676` calls `s.headlessPool.CallBlockingWithOptions(triageCtx, headless.FeatureKeyTriage, headless.HeadlessTriageSystemPrompt(), triagePrompt, headless.CallOptions{WorkDir: itemRepoPath})`, and `headlessTriageSystemPrompt` (`headless/features.go:99`) explicitly tells the model "You have full filesystem write access to the artifact directory" — and this demonstrably works today (triage writes `plan.md` etc. into that directory). That proves the `claude -p` subprocess, when given a `WorkDir`, already has functioning filesystem tool access with **no additional flags** (`runner.go` args never set `--allowedTools`/`--dangerously-skip-permissions`; see §4).
- **Gap:** the two review call sites — `review_gate.go:252` (`CallBlockingWithCost`, no options) and `backlog_service_triage.go:871-873` (`TriggerReReview`, explicit `CallOptions{}` i.e. no WorkDir) — never set `WorkDir`. `CallBlockingWithCost` (`headless/caller.go:457`) doesn't even accept a `CallOptions` parameter today, so wiring this through requires either a new `CallBlockingWithCostAndOptions` variant or adding `opts CallOptions` to the existing signature (2 call sites to update: `review_gate.go`, plus check for other callers of `CallBlockingWithCost`).
- Conclusion: giving the reviewer bounded, scoped filesystem read access for the empty-diff case is a **small, already-precedented plumbing change** — thread `wt.WorktreePath` into a `CallOptions{WorkDir: ...}` on the review call, reusing the exact mechanism triage already relies on. No new worktree-path-resolution code is needed.

## 4. Tool-use pattern: already agentic under the hood, single-shot from Go's perspective

- `session/headless/*` is a thin **subprocess wrapper around the real `claude` CLI binary** (`findClaudeBinary`, `caller.go:67-84`), not a bespoke LLM-API client. The Go code's args (`caller.go:170-181`) are just `-p [--output-format json] --system-prompt ... [--resume <id>] --exclude-dynamic-system-prompt-sections [--model ...]` — there is **no `--allowedTools`, `--disallowedTools`, or `--dangerously-skip-permissions` flag anywhere** in `session/headless/`.
- Because the subprocess *is* the full Claude Code CLI, its internal agentic tool-use loop (read/write/grep/bash) already runs **inside that single `Run()` invocation**, transparent to the Go orchestration layer — Go only ever sees one collected final text result (`CallBlocking`/`CallBlockingWithCost`), never individual tool_use/tool_result turns. This is proven by the triage call (§3) which writes files it wasn't given any Go-level "tool" for.
- **Implication for this project:** "reviewer reads the codebase before verdicting" is a **prompt + WorkDir change, not an agentic-loop build-out**. There is no MCP/tool-definition plumbing needed in `session/headless/` itself — the review call already gets whatever tool access `claude -p` grants by default (evidently full read/write, per the triage precedent) the moment a `WorkDir` is set. The main design decision is **which tools to allow/scope for review** (the existing `reviewSystemPrompt`, `headless/features.go:79`, already explicitly says "Do not write any code. Do not modify any files." for the interactive/tool-based `BuildReviewPrompt` path — that instruction pattern extends naturally to "you may read files under the worktree to verify already-implemented criteria, but must not modify them"). No flag exists today to *restrict* the reviewer to read-only at the process level; enforcement is currently prompt-only (an existing, accepted pattern in this codebase, not a new risk introduced by this feature).
- Open item to verify empirically before implementation: whether `claude -p` running as the systemd service user actually auto-approves tool calls with zero interactive prompt in this environment (permission-mode/trust settings), or whether the triage precedent works only because writes inside a scratch artifacts dir are treated differently than reads across a full worktree. Recommend a smoke test (headless call with `WorkDir` set to a real worktree, prompt asking it to `cat` a known file) during Phase 5 implementation before relying on it for the review gate.

## 5. Event-Command-Policy table

This is a self-contained backend state machine (one bounded context, no cross-team/cross-service actors, no human approval step in this particular slice), so a full EventStorming pass is more ceremony than the problem needs. A condensed table is still useful to make the auto-reopen policy trigger explicit, since that's the mechanism this feature must not trip incorrectly:

| Domain Event | Policy trigger | Command | Actor/System |
|---|---|---|---|
| `CriterionProgressReported` (note attached) | none (data write only) | `UpdateAcCriterionStatus` | Work session (agent) via `report_progress` |
| `ReviewRequested` (verification_notes attached) | Item `in_progress → review` | `TransitionBacklogItemStatus`, `UpdateItemSessionVerificationNotes` | Work session (agent) via `request_review` |
| `WorkSessionExited` | if item now `review` and gate not skipped → spawn review | `spawnReviewGate` → `ReviewGateRunner.Run` | `BacklogLifecycleListener` (EventExited hook) |
| `DiffComputed` (possibly empty) | if empty **and** (this feature) evidence present → allow codebase-read path instead of auto-UNVERIFIABLE | (new) grant `WorkDir` to reviewer call | `ReviewGateRunner` |
| `ReviewVerdictRecorded` (PASS) | → push branch, open PR | `onPass` → `pushAndCreatePR` | `ReviewGateRunner` / `BacklogLifecycleListener` |
| `ReviewVerdictRecorded` (FAIL/PARTIAL/UNVERIFIABLE) | → auto-reopen for rework (up to cap, out of scope to change) | `AutoReopenAfterFailedReview` | `AutoReopenSpawner` |
| `ReworkCapHit` (out of scope) | → notify human | `notifyReworkCapHit` | `BacklogService` |

The only new event this feature introduces is effectively a refinement of `ReviewVerdictRecorded`: today "empty diff" always routes through the FAIL/PARTIAL/UNVERIFIABLE policy; the fix makes the reviewer's verdict itself grounded (via note + verification_notes + optional codebase read) so a genuinely-already-done item can legitimately reach `ReviewVerdictRecorded(PASS)` without touching the auto-reopen policy at all — i.e., the intervention point is entirely upstream of step "ReviewVerdictRecorded", inside prompt construction and the reviewer's information access, not in the state machine or policies themselves.

## Summary of architectural implications for Phase 3 planning

1. **No ent/proto schema changes needed.** `AcCriterion.Note` and `ItemSession.verification_notes` already exist and already round-trip through the DB.
2. **Two Go changes are required together for `note` to reach the reviewer**, not one: (a) render `c.Note` in `BuildReviewPrompt`/`BuildHeadlessReviewPrompt` (`backlog_review.go`), and (b) fix the AC-snapshot selection in `review_gate.go:236-240` (and `backlog_service_triage.go`'s re-review path) to prefer/merge the live `item.AcceptanceCriteria` instead of the stale `is.AcSnapshot`. Skipping (b) makes (a) a no-op in the common case.
3. **Worktree-scoped codebase read access for the empty-diff case is a small, precedented change**: thread `wt.WorktreePath` (already resolved in `review_gate.go:82`) into a `CallOptions{WorkDir: ...}` on the review's headless call — reusing exactly the mechanism `backlog_service_triage.go`'s triage call already uses successfully. Requires extending `CallBlockingWithCost`'s signature (or adding a `*WithOptions` sibling) since it currently has no `CallOptions` parameter.
4. **No agentic-loop build-out needed** — the `claude -p` subprocess already runs the full Claude Code tool-use loop internally; Go only sees the final text. The design lever is prompt instructions (what the reviewer is told it may/may not do) plus `WorkDir` scoping, not new MCP tool wiring in `session/headless/`.
5. **Anti-gaming stays enforceable at the prompt layer**, consistent with the existing `reviewSystemPrompt`'s "vague claims are not evidence" instruction (`headless/features.go:80`) — extend that same specific-and-checkable-claims bar to "if you read the codebase to verify, cite the exact file/line/symbol you looked at" so a false "already implemented" claim without corroborating evidence still fails, same mechanism already governing `verification_notes` today.
6. **Verify empirically** (not just from static code reading) that `claude -p` with `WorkDir` set grants read access with no extra permission flags, the same way it demonstrably grants write access for triage — do this before finalizing the plan's reviewer-prompt design.
