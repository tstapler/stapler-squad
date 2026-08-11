# Research: Feature Landscape for `report_duplicate` + `request_review` CAS fix

Source item: `da58b867-bf4e-4720-8fe4-9cfcfa5b6eed`. Codebase state: `server/mcp/tools_backlog.go` (1090 lines), `server/mcp/tools_github.go` (307 lines), `session/*.go`, `github/*.go` at HEAD of this worktree.

## 1. Existing MCP tools — patterns to reuse

### `request_review` (`server/mcp/tools_backlog.go:337-447`)
- Auth: `callerSessionUUID(ctx)` → `ErrPermissionDenied` if unset.
- Link check: `h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID)`, `session.ErrNotFound` → `ErrPermissionDenied` "not linked to item".
- Dirty-worktree guard (`session.IsWorktreeDirty`) rejects with `ErrInvalidArgument` before any mutation — a "belt-and-suspenders" precondition pattern worth mirroring.
- **The CAS bug** (line 414): `precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress)}` is a **hardcoded constant**, not derived from the `item` loaded two lines above (line 404, `item, itemErr := h.storage.GetBacklogItem(...)`). FR1 fix: set `ExpectedStatus: item.Status` (the observed status) instead. This is a one-line change at the call site — `item` is already in scope.
- `SkipReviewGate` handling (lines 394-411) already special-cases the target status (`review` vs `done`) — this logic is orthogonal to the CAS precondition and untouched by FR1/FR2.
- No existing check for "is there already an active review session" — FR2 is wholly new logic to add here.
- Verification notes persisted via `h.storage.UpdateItemSessionVerificationNotes(ctx, itemSession.ID, verificationNotes)` (best-effort, logged on failure, does not fail the call) — FR7 should reuse this exact call for `report_duplicate`'s `duplicate_ref`/`reason`.
- Review gate spawn: `h.reviewTrigger.TriggerReviewForSession(callerUUID)` — fires immediately rather than waiting for the 60s `ReconcileStuck` tick.

### `submit_review_verdict` (lines 458-570)
- Role-gated: `itemSession.Role != "review"` → `ErrPermissionDenied`. Explicitly out of scope to touch (non-goal in requirements.md).
- **Deliberately does not transition status itself** — the comment at lines 548-555 states `BacklogLifecycleListener.handleReviewSessionExited` (`session/backlog_lifecycle.go`) is the sole place that decides what happens after a review session exits (PASS → push+PR+`pr_pending`; FAIL/PARTIAL/UNVERIFIABLE → reopen). This is an important architectural precedent: **status transitions belong at the point a session's role-scoped work concludes, not inside every verdict-recording call.** `report_duplicate` is different — it doesn't have a subsequent "session exits" hook to lean on, so it must transition synchronously itself (per FR3, straight to `review`), the same way `request_review` does.

### `report_pr_created` (lines 623-726) — **the direct model for `report_duplicate`**
- Role-gated to `session.SessionRoleWork` (line 669) — same gate FR6 wants for `report_duplicate`.
- **Idempotency check** (lines 681-686): if `item.Status == pr_pending && item.PrNumber == prNumber`, return success no-op rather than erroring or re-transitioning. `report_duplicate` needs an analogous idempotency story — see §3 "concurrent report_duplicate calls".
- **Pre-network sanity check** (lines 688-697): parses the URL and cross-checks the embedded PR number against the argument *before* any GitHub call — cheap, no-network rejection of a definitely-malformed input. Worth mirroring for `duplicate_ref`.
- **Two-channel verification error split** (lines 707-715) — this is the literal FR4 precedent:
  ```go
  matched, verifyErr := h.verifyPR(ctx, ref.Owner, ref.Repo, prNumber, branch)
  if verifyErr != nil {
      return errResult(ErrInternalError, fmt.Sprintf("could not verify PR #%d against GitHub — retry: %v", prNumber, verifyErr), ""), nil
  }
  if !matched {
      return errResult(ErrInvalidArgument, fmt.Sprintf("PR #%d does not match ... refusing to record it.", prNumber, branch), ""), nil
  }
  ```
  The channel split is driven by `VerifyPRMatchesBranch`'s own three-way contract (see §2) — `(false, nil)` = definitive mismatch → `ErrInvalidArgument`; `(_, err)` = transient lookup failure → `ErrInternalError`. `report_duplicate` needs the same three-way contract from whatever verification function it calls.
- Primary write path: `h.storage.SetBacklogItemPRAndTransition(ctx, itemID, prURL, prNumber, summary)` — bundles the PR-metadata write and the `pr_pending` transition. `report_duplicate` will need its own bundling (or just call `TransitionBacklogItemStatus` + `UpdateItemSessionVerificationNotes` directly, following `request_review`'s pattern instead since the target is `review`, not a new status).

### `submit_triage_result` (lines 730-915)
- Role-gated to `"triage"`. Shows the `eventBus.Publish(events.NewNotificationEvent(...))` pattern for pushing a notification (`NOTIFICATION_TYPE_INPUT_REQUIRED`) — a candidate mechanism if FR10's "eventually surfaces" needs an explicit nudge beyond the passive stuck-item reconciler (see §4).
- Shows the "merge, never silently delete" pattern for `acceptance_criteria` — not directly relevant to this feature but a useful convention reference.

### GitHub tools (`server/mcp/tools_github.go`)
- `VerifyPRMatchesBranch` (lines 272-281) is the type of function `report_duplicate`'s verification will need an analog of, but note its contract only covers **PR-vs-branch matching** — see §2 for why it isn't directly reusable for `duplicate_ref`.
- `detectRepoPath` (lines 285-306) — not directly relevant; only used for `create_session_for_pr`.

## 2. Gap: no existing verifier or URL parser covers issue/commit refs

Two competing GitHub-URL parsers exist in this repo, and they are **not equivalent**:

| | `session.ParseGitHubURL` (`session/repo_path.go:93`) | `github.ParseGitHubRef` (`github/url_parser.go:281`) |
|---|---|---|
| Used by | `report_pr_created` today | Nothing in `server/mcp/` yet |
| `RefType`/`GitHubRefType` values | `Repo`, `Branch`, `PR` only | `PR`, `Branch`, `Repo`, `File`, **`Commit`**, **`Issue`**, `Compare`, `Release` |
| Struct | `GitHubRef{Owner,Repo,Branch,PRNumber,Type}` | `ParsedGitHubRef{..., IssueNumber, CommitSHA, ...}` |

**Finding:** `github.ParseGitHubRef` already has first-class `RefTypeIssue` and `RefTypeCommit` support with `IssueNumber`/`CommitSHA` fields populated. This directly answers the open question "whether `session.ParseGitHubURL` ... already generalize[s] to non-PR refs" — it doesn't, but a parallel, richer parser (`github.ParseGitHubRef`) already does and should be used for `duplicate_ref` instead of `session.ParseGitHubURL`.

**What's still missing:** no verification-against-GitHub function exists for issue or commit refs analogous to `VerifyPRMatchesBranch`/`GetPRForBranch`:
- `github.GetIssue(ctx, owner, repo, number)` (`github/repos.go:270`) exists and returns `(*IssueResult, error)` with typed 401/404/403(rate-limit) handling already broken out in its body (worth copying its status-code triage logic for FR4's error channel split) — this can serve as the issue-existence check.
- **No commit-existence check exists anywhere in `github/*.go`.** `duplicate_ref` accepting a commit SHA URL (FR3/AC3 says "PR/issue/commit URL") needs a new function, e.g. a `GET /repos/{owner}/{repo}/commits/{sha}` call modeled on `GetIssue`'s HTTP/error-handling shape.
- For the PR case, `report_duplicate` can reuse `GetPRInfoCtx`/`GetPRForBranchConditional` (`github/client.go`) rather than `VerifyPRMatchesBranch`, since `VerifyPRMatchesBranch` verifies a PR *against a specific branch* (this session's own branch) — semantically wrong for `report_duplicate`, which verifies an *arbitrary other* PR/issue/commit exists, unrelated to the caller's own branch. `PRInfo.State` (`github/client.go:46`, values are gh CLI's raw state string, e.g. `"OPEN"`/`"CLOSED"`/`"MERGED"`) is available for a "was this actually merged" check if the plan phase decides to require it (see §3, "PR not merged yet").

## 3. Edge cases and failure modes for `report_duplicate` / `request_review` CAS fix

| Edge case | Existing infra that helps | Gap / design decision needed |
|---|---|---|
| **`duplicate_ref` points at a PR that isn't merged yet** | `PRInfo.State` field exists via `GetPRInfoCtx` | AC3/AC4 only require the ref to be **verified to exist**, not merged. No requirement mandates a merge-state check — plan phase must decide whether an existing-but-open/unmerged PR should be accepted (weaker guarantee, matches the literal AC) or rejected (`ErrInvalidArgument`, "not yet merged") to avoid marking an item duplicate against something that could still be abandoned. Origin incident (`fc63d55b`) was a case where the superseding PR *was* already merged. |
| **Ref is an issue, not a PR** | `github.ParseGitHubRef` returns `RefTypeIssue`; `github.GetIssue` exists | Need to route to `GetIssue` in the verification switch; no PR-specific concept (branch/merge) applies — existence alone is likely the correct/only check. |
| **Ref is a commit SHA URL** | `github.ParseGitHubRef` returns `RefTypeCommit`/`CommitSHA` | No commit-existence GitHub call exists yet (see §2) — net-new function needed. |
| **Private/inaccessible repo** | `github.GetIssue` already special-cases 401 vs 404 vs 403-rate-limit in its HTTP handling (`github/repos.go` ~lines 289-300) | A private/inaccessible repo (403 without rate-limit headers, or 404 for a repo the token can't see — GitHub returns 404, not 403, for repos invisible to the token) is **neither a clean "doesn't exist" (ErrInvalidArgument) nor a clean "transient, retry" (ErrInternalError)** — retrying won't help if the caller's `gh` credentials genuinely lack access. Plan phase should decide: treat GitHub's opaque 404-for-invisible-repos as `ErrInvalidArgument` (matches GitHub's own conflation of "doesn't exist" and "you can't see it"), and treat true 403 auth/permission errors as a **third, distinct** message so the agent doesn't retry a permanently-unfixable case. Worth flagging as a plan/validate-phase decision, not a code gap — the underlying HTTP-status handling already exists to build on. |
| **Rate limiting** | `github.RateLimiter` (`github/rate_limit.go`) + `GetIssue`'s explicit `X-RateLimit-Remaining`/`Retry-After` checks already return typed transient errors | Straightforward `ErrInternalError` + retry wording, same as `report_pr_created`'s pattern. |
| **Item already `pr_pending` with a DIFFERENT PR that also needs handling** | `report_pr_created`'s idempotency check only covers the *same* PR number | `report_duplicate` has no precedent for this. If item is `pr_pending` with PR #281 open and the agent calls `report_duplicate` citing PR #272, the item's own now-superseded PR (#281) is left dangling — open on GitHub, orphaned from the item's state (which will show `review` after the transition, with no reference to #281 except whatever is in `item.PrURL`/`PrNumber`, untouched by this tool per FR8's no-schema-change constraint). **No AC requires closing the superseded PR** — this is a real gap in the *outcome*, not just an edge case, see §5. |
| **Concurrent `report_duplicate` calls** (same item, e.g. two calls racing, or racing with a concurrent `request_review`/system-driven transition) | `TransitionBacklogItemStatus`'s CAS precondition (`session/storage.go:736`, `session/ent_repository_backlog.go:869`) already guards this — a second caller's `ExpectedStatus` won't match post-first-transition status and the CAS update affects zero rows, surfaced as a precondition-failure error (see `TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently`, `session/ent_repository_backlog_transition_test.go:78`, and `TestTransitionBacklogItemStatus_should_notPublish_When_CASAffectsZeroRows`, `session/ent_repository_backlog_events_test.go:104`) | As long as `report_duplicate` uses the same `TransitionBacklogItemStatus`+precondition pattern as `request_review`, no new concurrency-control code is needed — the existing CAS primitive already provides exactly-one-winner semantics; the loser just needs a sane error message (`ErrInternalError` or similar, not a crash). |
| **Item already `SkipReviewGate == true`** | `item.SkipReviewGate` field already loaded via `GetBacklogItem` (used by `request_review` at line 409) | FR6/AC6 explicit refusal — trivial to add as an early check once `item` is loaded, mirroring how `request_review` reads the flag (but there it *changes routing*, not refuses; `report_duplicate` must refuse outright per FR6). |
| **Session not linked to item** | `GetItemSessionBySessionAndItem` + `session.ErrNotFound` → `ErrPermissionDenied` pattern used identically in all four existing tools (`requestReview:373-379`, `submitReviewVerdict:502-508`, `reportPRCreated:662-668`, `submitTriageResult:755-761`) | Zero gap — copy the exact 6-line block. |

## 4. Active review-session detection (FR2 / FR6)

No single "does item X have an active review session" helper exists today, but the underlying query shape is already used twice at the ent-repository layer:

- `FindZombieReviewItems` (`session/storage_backlog.go:1043-1060`) filters `itemsession.EndedAtIsNil()` + `itemsession.SessionRoleIn(SessionRoleReview, SessionRoleWork)`.
- `FindDriftedPRItems` (`session/storage_backlog.go:756-771`) uses the identical `EndedAtIsNil()` + role-in filter, described in its comment as mirroring "AutoReopenForPRFix's/AutoRespawnReview's identical `hasActiveWorkSession`/`hasActiveReviewSession` guard" — implying named helpers exist somewhere in `session/backlog_lifecycle.go`, but they were not found as exported/reusable functions in this pass (worth a targeted grep in the plan phase: `hasActiveReviewSession`, `hasActiveWorkSession` as unexported lifecycle-package helpers, likely private to `BacklogLifecycleListener` and not directly callable from `server/mcp`).
- The simplest path for FR2/FR6 without adding a new ent query: `h.storage.ListItemSessions(ctx, itemID)` (`session/storage.go:1098`, backed by `session/storage_backlog.go:138`) returns all `ItemSessionSummary` for the item; filter client-side for `Role == "review" && EndedAt == nil`. Slightly less efficient than a targeted query but requires zero new repository-layer code — consistent with FR8's "no schema changes" spirit (though a new query method would still be pure Go, no schema impact, if preferred for correctness under concurrent load).

**"Active review-role `ItemSession`" is the literal DB-level definition already established** (`EndedAt IS NULL`, `SessionRole = review`) — FR2/FR6 should use exactly this definition rather than inventing a new liveness signal (e.g. a heartbeat or process check), consistent with the row-based semantics used everywhere else in the reconciliation code.

## 5. `TriggeredBy`, audit trail (FR7)

- `session.TriggeredByUser = "user"`, `session.TriggeredBySystem = "system"` (`session/backlog.go:92-93`). **No `TriggeredByAgent` constant exists** — FR7 explicitly requires introducing one (e.g. `TriggeredByAgent = "agent"`).
- `recordStatusEvent(ctx, evClient, itemID, fromStatus, toStatus, triggeredBy, note)` (`session/ent_repository_backlog.go:45`) is called **automatically inside** `TransitionBacklogItemStatus` (`ent_repository_backlog.go:938`) — no extra plumbing needed for the audit row itself; just pass the new `TriggeredByAgent` constant as the `triggeredBy` argument on the `report_duplicate` (and CAS-fixed `request_review`?) call sites. Note: requirements.md's FR7 says "every status transition this feature causes" — worth clarifying in planning whether the FR1-fixed `request_review` pr_pending→review path should also switch from `TriggeredBySystem` to `TriggeredByAgent` (it's arguably agent-triggered too, just pre-existing code used `TriggeredBySystem` uniformly) or whether only the new `report_duplicate` transition gets the new constant. Requirements.md doesn't disambiguate; AC7 only says "every transition made by this feature."
- `UpdateItemSessionVerificationNotes(ctx, itemSessionID, notes)` (`session/storage.go:963`) is the existing, reusable storage path FR7 names for persisting `duplicate_ref`+`reason` — `request_review` already calls this exact method (line 424) for its own `verification_notes` argument, confirming the reuse path is real and low-risk.

## 6. FR5 — "next review pass" messaging nuance

Requirements/AC5 states verification notes are "rendered into the reviewer's prompt only once, at review-gate spawn time (not live-updated mid-session)." This claim traces to `BuildReviewPrompt` (referenced in `request_review`'s own comment at line 421: "so the review gate can surface it in the reviewer's prompt (see `BuildReviewPrompt`)") — confirms the mechanism is real and already documented in-repo, not a research gap. `report_duplicate`'s success-message logic should reuse the same "is there an active review-role ItemSession right now" check from §4 to decide between "will be reviewed" vs "will land on the next review pass" wording — i.e. FR5 and FR2/FR6 share the identical active-session-detection primitive, so it only needs to be built once and consumed by both code paths.

## 7. FR10 — stuck-item surfacing

- `session/domain/backlog.go` already defines `type StuckReason string` with `AllStuckReasons` and constants (confirmed via `session/domain/backlog_test.go`); the stuck-item pipeline (`ReconcileStuck`, `session/backlog_lifecycle.go:1428`, `backfillMarkAndNotify` at line 1307, `MarkStuckNotified` semantics at line 3208) is a mature, existing mechanism — not something this feature needs to build from scratch.
- **Not confirmed in this pass:** whether `pr_pending` is actually one of the statuses `ReconcileStuck` scans for staleness/notification today. `FindDriftedPRItems` explicitly **excludes** `pr_pending` from its own scan (`backlogitem.StatusNotIn(BacklogStatusPRPending, BacklogStatusDone, BacklogStatusArchived)`, `session/storage_backlog.go:756-771`) — that specific reconciler is *not* the one FR10 can lean on for a `pr_pending`-stuck item. The plan/validate phase should grep `session/backlog_lifecycle.go` for whichever `Find*Stuck*` / `Find*PRPending*` function *does* cover `pr_pending` specifically (a `FindStuckReviewItems` name was referenced in a nearby comment at `storage_backlog.go` but its status coverage wasn't verified in this pass) before assuming FR10's "eventually surfaces" claim needs zero new plumbing.
- `submit_triage_result`'s `eventBus.Publish(events.NewNotificationEvent(...))` pattern (lines 891-909) is available as a fallback/supplement if the passive reconciler path turns out not to cover `pr_pending` — an explicit notification could be published directly from `report_duplicate`'s verification-failure-logged path instead of relying solely on the stuck-item reconciler picking it up on its next tick.

## 8. No-schema-change constraint (FR8) — sanity check

Confirmed no `BacklogStatus` enum addition is implied by anything found in this pass: the only statuses referenced anywhere near this feature are the existing `BacklogStatusInProgress`, `BacklogStatusPRPending`, `BacklogStatusReview`, `BacklogStatusDone`, `BacklogStatusArchived` (all pre-existing). `report_duplicate`'s "duplicate" outcome is representable entirely as `status = review` + persisted `verification_notes` (duplicate_ref + reason) — no schema gap found.

## 9. Unstated needs implied by the origin item (`da58b867`), beyond the 10 literal ACs

Read via `get_backlog_item` (full description, not just requirements.md's distilled FRs):

1. **The origin incident's actual mess was never fully addressed by the 10 ACs.** The item's own PR (#281) had to be manually closed by a human after the duplicate was discovered — `report_duplicate` as specified (AC1-10) only routes the *backlog item* to `review`; it does **not** touch the caller's own already-open PR on GitHub (close it, comment on it, or link it to the superseding PR). A human/reviewer must still manually close PR #281-equivalent PRs after every `report_duplicate` call. This is explicitly a non-goal per requirements.md's "No UI changes... unless research finds it's needed" framing doesn't cover this, but it's worth flagging: **the feature declares the backlog item "resolved" while leaving a real, open, doomed PR on GitHub for a human to notice and close** — same class of "requires manual operator intervention" the item's Impact section is complaining about, just shifted from the *item* to the *PR*. Plan phase should at minimum ensure `report_duplicate`'s success message/verification notes make it trivially obvious to the reviewer which PR needs manual closing (item's own `pr_url`/`pr_number`, if already set from a prior `report_pr_created` call, is available in `item` and could be echoed back).
2. **The root scenario is fundamentally a race-detection problem, not just a resolution-tooling problem.** The item's Description explicitly frames the trigger as working "end-to-end" and discovering the duplicate "only after implementing, reviewing, and shipping a full fix" — i.e., by the time `report_duplicate` is even callable, all the wasted work has already happened. `report_duplicate` is reactive cleanup, not prevention. The description explicitly gestures at prevention infrastructure that already exists but isn't leveraged here: `list_workspace_peers` (mentioned directly in the item's Impact section — "as this system explicitly supports, per `list_workspace_peers` showing many...") and `list_github_prs`/`create_session_for_pr` (`server/mcp/tools_github.go`) already give a session visibility into parallel work. None of the 10 ACs ask a work session to *check* these before or during work to avoid the duplicate in the first place. This is explicitly out of scope per requirements.md's non-goals section, but the plan phase / a follow-up item should note that shipping `report_duplicate` alone treats the symptom (no way to *close out* a discovered duplicate) without the cheaper prevention this repo already has the primitives for (checking peers/PRs before starting substantial work).
3. **Priority 3, and the item is self-referential** — this item is itself a backlog item being worked by a session that will eventually need to call `request_review`/`report_pr_created` on itself using the *current*, unfixed tools. Worth double-checking in the validate/implement phase that shipping this fix doesn't require the shipping session to hit the exact bug being fixed (e.g., if this session's own item ends up `pr_pending` and needs a second review round) — though this is more a process note than a code requirement.

## 10. Follow-up: closing three open threads from §4/§7 (second pass)

- **§4's open question is resolved.** `hasActiveReviewSession` **does exist**, exactly as `hasActiveSession`'s doc comment (`session/backlog_lifecycle.go:3344-3350`) claimed, but it's unexported and lives in `server/services/backlog_service_triage.go:1101-1111`:
  ```go
  func hasActiveReviewSession(priorSessions []session.ItemSessionSummary) bool {
      for _, ps := range priorSessions {
          if ps.Role == session.SessionRoleReview && ps.EndedAt == nil {
              return true
          }
      }
      return false
  }
  ```
  This is the literal role-scoped (review-only, not work-or-review) predicate FR2 needs — but it's private to `server/services`, which `server/mcp` cannot import (and even if it could, `server/services` likely imports `server/mcp`'s package tree the other way, risking a cycle — not verified, but the existing pattern of three near-identical implementations, `hasActiveSession` in `session`, and `hasActiveWorkSession`/`hasActiveReviewSession` in `server/services`, already shows this codebase's convention is to **duplicate this 5-line predicate per-package** rather than force a shared export across package boundaries. The plan phase should just write the same 5-line loop inline in `server/mcp/tools_backlog.go` (operating on the `[]session.ItemSessionSummary` from `h.storage.ListItemSessions(ctx, itemID)`, already used elsewhere in this file) rather than trying to export/reuse either existing copy.

- **Message/field length-limit convention (explicitly asked, not covered above):** there is no hard rule, and it's inconsistent even among the newest tools — `request_review`'s `message` is capped at 2000 chars and `verification_notes` at 4000 (`tools_backlog.go:360-370`), `report_pr_created`'s `summary` is capped at 1000 (`tools_backlog.go:657-659`), but `submit_review_verdict`'s `summary` has **no cap at all** (`tools_backlog.go:477-480`) — an apparent oversight, not a deliberate exception. Since FR3 explicitly frames `report_duplicate` as mirroring `report_pr_created`, and `reason` plays the same role there that `summary` plays in `report_pr_created` (a short human-readable justification shown alongside a link, not a large evidence dump like `verification_notes`), the natural convention to copy is **`report_pr_created`'s 1000-char cap**, applied to `reason`. `duplicate_ref` is a URL/short string; a generous-but-bounded cap (200-500 chars) is enough to reject pathological input without needing precedent-matching precision — no existing tool caps a `*_url`-shaped argument today (`report_pr_created`'s `pr_url` has no explicit cap), so this would be a new-but-reasonable addition, not a convention violation.

- **`UpdateItemSessionVerificationNotes` overwrites, it does not append** (`session/storage_backlog.go:397-409`: `SetVerificationNotes(verificationNotes)`, a full replace). FR7 says `duplicate_ref`+`reason` get "persisted in item session verification notes" — the same field `request_review` already writes to on the same `ItemSession` row. If a work session already called `request_review` once (setting `verification_notes`), got bounced back to `in_progress` for rework, and later calls `report_duplicate` on that same `ItemSession`, a naive reuse of this setter will **silently discard** the prior verification notes. The plan phase needs to pick an explicit strategy — read-then-append (`existing + "\n\n---\n\n" + newContent`) is the safer default — rather than leaving it as an implicit overwrite, since nothing in the existing codebase's use of this setter has had to consider this coexistence case before (every current caller is the first and only writer to a given `ItemSession`'s notes).
