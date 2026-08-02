# Architecture Research: backlog-self-resolve

Builds directly on `project_plans/backlog-stuck-item-visibility/research/architecture.md`
(§1-2, TransitionBacklogItemStatus/precondition/event-table plumbing — since implemented,
see §8 below) and `project_plans/backlog-agent-communication/research/architecture.md`
(report_pr_created / MCP tool call graph). This doc fills the 8 gaps requirements.md asked
for, grounded in the code as it exists today, not as it was proposed in those docs.

## 1. TransitionBacklogItemStatus / BacklogItemPrecondition — current signature

`session/ent_repository_backlog.go:869`:

```go
func (r *EntRepository) TransitionBacklogItemStatus(ctx context.Context, id string,
    toStatus BacklogStatus, precondition *BacklogItemPrecondition, triggeredBy string,
) (*BacklogItemData, error)
```

`session/repository.go:550`:

```go
type BacklogItemPrecondition struct {
    ExpectedStatus    string     // if non-empty, WHERE status = ExpectedStatus
    ExpectedUpdatedAt *time.Time // if non-nil, WHERE updated_at = ExpectedUpdatedAt
    Note              string     // stored in the BacklogStatusEvent audit row
}
```

CAS mechanics (`session/ent_repository_backlog.go:883-917`): the precondition becomes a
`WHERE` clause on the `UPDATE`. If `affected == 0`, the row is re-fetched and the mismatch
is reported as `fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, ...)`
(sentinel `ErrPreconditionFailed`, checkable via `errors.Is`). This is unchanged from what
`backlog-stuck-item-visibility`'s research documented — no drift.

**request_review today (`server/mcp/tools_backlog.go:414`)** hardcodes the precondition:

```go
precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress)}
```

FR1 requires this to instead be pinned to whatever status was actually observed on the
loaded item (`item.Status`, already fetched at line 404 via `h.storage.GetBacklogItem`),
restricted to the two allowed source statuses (`in_progress`, `pr_pending`) — reject
anything else before ever calling `TransitionBacklogItemStatus`, so the CAS predicate is
always exactly what was observed, never a guess.

## 2. TriggeredByAgent constant — does not exist, needs adding

`session/backlog.go:90-94`:

```go
// TriggeredBy values for BacklogStatusEvent records.
const (
    TriggeredByUser   = "user"
    TriggeredBySystem = "system"
)
```

No `TriggeredByAgent` (or similarly named) constant exists anywhere in `session/` or
`session/domain/` — confirmed via `grep -rn "TriggeredBy" session/*.go session/domain/*.go`.
FR7 requires adding `TriggeredByAgent = "agent"` right here, alongside the other two.

`BacklogStatusEvent`/`recordStatusEvent` live at `session/ent_repository_backlog.go:45`:

```go
func recordStatusEvent(ctx context.Context, evClient *ent.BacklogStatusEventClient,
    itemID uuid.UUID, fromStatus, toStatus, triggeredBy, note string)
```

Called once, internally, from `TransitionBacklogItemStatus` (line 938) — callers never call
it directly, they just pass `triggeredBy` through to `TransitionBacklogItemStatus`. So
satisfying FR7 is exactly: pass `session.TriggeredByAgent` as the `triggeredBy` argument on
every `TransitionBacklogItemStatus` call this feature makes (both the generalized
`request_review` and the new `report_duplicate`), instead of `TriggeredBySystem`. No other
plumbing changes needed — `recordStatusEvent` already takes `triggeredBy` as an opaque
string, it doesn't validate it against an enum.

## 3. Detecting "active (unended) review-role ItemSession exists for item X"

No new query is needed — this exact filter already exists in **three** places over the same
`[]ItemSessionSummary` shape (`session/repository.go:285-308`, fields `Role string`,
`EndedAt *time.Time`, both already returned by `Storage.ListItemSessions(ctx, itemID)`,
`session/storage.go:1098`):

- `server/services/backlog_service_triage.go:1104` — `hasActiveReviewSession`:
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
- `server/services/backlog_service_triage.go:926` — `hasActiveWorkSession` (identical
  shape, `SessionRoleWork`).
- `session/backlog_lifecycle.go:3351` — `hasActiveSession` — the same check inlined a
  third time *inside* the `session` package, with an explicit doc comment explaining why:
  "not reusable directly — that package imports session, not the other way around."

`server/mcp` (where `report_duplicate`/`request_review` live) is in the same position as
`backlog_lifecycle.go`: it cannot import `server/services` (that package depends on
`server/mcp` types transitively via the wider `server` tree — importing it back would be a
cycle risk, and there's no existing precedent for `server/mcp` importing `server/services`).
**Recommendation**: add a 4th, tools_backlog.go-local copy of the same one-line filter
(`ps.Role == session.SessionRoleReview && ps.EndedAt == nil`) over
`h.storage.ListItemSessions(ctx, itemID)`'s result — consistent with how the other two
packages each keep their own copy rather than sharing one. This is a query over **existing
ent fields** (`session_role`, `ended_at` — both already on the `ItemSession` schema,
`session/ent/schema/item_session.go:25,30`), so it satisfies FR8 (no schema change) trivially
— it's a Go-level filter, not a new ent predicate/index.

## 4. UpdateItemSessionVerificationNotes signature + confirmation VerificationNotes is read once at spawn

`session/storage.go:963`:
```go
func (s *Storage) UpdateItemSessionVerificationNotes(ctx context.Context, id string, verificationNotes string) error
```
(delegates to `EntRepository.UpdateItemSessionVerificationNotes`, `session/storage_backlog.go:397`
— takes the **ItemSession** UUID, not the BacklogItem UUID; `request_review` already resolves
this via `GetItemSessionBySessionAndItem` before calling it, see `tools_backlog.go:424`).

Confirmed by reading the actual review-gate spawn code, not inferring: `spawnReviewGate`
(`session/backlog_lifecycle.go:1208`) builds the prompt at line 315:
```go
prompt := r.reviewPromptFor(item, acSnapshot, diff, truncated, is.ID, is.VerificationNotes)
```
`is` is the `ItemSessionSummary` passed into `spawnReviewGate` as a parameter — a snapshot
read once, at the moment the review gate is spawned. `reviewPromptFor` →
`BuildReviewPrompt(item, acSnapshot, diff, diffTruncated, itemSessionID, verificationNotes)`
(`session/backlog_review.go:222`) takes `verificationNotes` as a plain `string` parameter —
there is no live re-read, no pointer/callback back into storage, nothing that could pick up
a later `UpdateItemSessionVerificationNotes` write once the reviewer session is already
running. This directly grounds FR5: if `report_duplicate` succeeds while a review-role
session is already active for the item, that reviewer's prompt was already built and will
never see the new verification notes — the success message must say "next review pass,"
not "the current reviewer."

## 5. session.ParseGitHubURL / VerifyPRMatchesBranch — PR-only; a second, more general parser already exists

**`session.ParseGitHubURL`** (`session/repo_path.go:93`, used by `report_pr_created`) only
recognizes 4 shapes: PR URL, branch (`/tree/`) URL, bare repo URL, and `owner/repo[:branch]`
shorthand (`GitHubRefType` enum: `Repo | Branch | PR` — no `Issue`, no `Commit`). It **cannot**
parse an issue or commit URL — passing either through would hit its final `owner/repo`
shorthand regex or fail outright, definitely not producing a usable issue/commit reference.

**`VerifyPRMatchesBranch`** (`server/mcp/tools_github.go:272`):
```go
func VerifyPRMatchesBranch(ctx context.Context, owner, repo string, prNumber int, expectedBranch string) (bool, error)
```
wraps `githubpkg.GetPRForBranch` and is branch-shaped by design (it exists to confirm a
self-reported PR belongs to *this session's own branch* — not applicable to a duplicate
reference, which points at someone else's PR/issue/commit, not this session's branch).
**Not reusable as-is for `report_duplicate`.**

**A second, more capable parser already exists**: `github.ParseGitHubRef` /
`ParseGitHubRefWithHosts` (`github/url_parser.go:281,289` — package `github`, distinct from
`session`, already imported by `server/mcp/tools_github.go`). Its `RefType` enum
(`github/url_parser.go:13-22`) already includes `RefTypePR`, `RefTypeIssue`, `RefTypeCommit`
(plus Branch/Repo/File/Compare/Release), and `ParsedGitHubRef` (`url_parser.go:48-64`) already
carries `PRNumber`, `IssueNumber`, and `CommitSHA` as distinct fields populated per-type. This
is the correct parser for `report_duplicate`'s `duplicate_ref` — it already generalizes to
exactly the three ref kinds FR3 needs, with no changes required to the parser itself.

**Verification calls available per ref type**:
- **PR**: `github.GetPRInfoCtx(ctx, owner, repo, prNumber)` (`github/client.go:247`) — but it
  shells out to `gh pr view` and folds "not found" and "network/auth failure" into the same
  generic `fmt.Errorf`, so it does **not** cleanly support FR4's two-channel error split on
  its own. `githubpkg.GetPRForBranch`'s pattern (used by `VerifyPRMatchesBranch`) — a typed
  `ErrNoPR` sentinel distinct from other errors — is the right model to follow, but
  `GetPRForBranch` verifies by branch, not by number; `report_duplicate` would need either a
  small wrapper around `GetPRInfoCtx` that maps the `gh` CLI's "no such PR" stderr text to a
  typed not-found error, or (cleaner, HTTP-based, matching the pattern below) a new
  `GetPR(ctx, owner, repo, number)` sibling to `GetIssue` built on the same HTTP client.
- **Issue**: `github.GetIssue(ctx, owner, repo, number)` (`github/repos.go:270`) — already the
  right shape for FR4: HTTP-based (`ghHTTPClient`, not a `gh` shellout), and explicitly
  distinguishes `resp.StatusCode == 404` → `fmt.Errorf("GitHub API: issue not found (404)")`
  from `401`/`403`(rate-limit)/other transport errors. This is the pattern to copy.
- **Commit**: **no existing function.** No `GetCommit`/`CommitInfo` exists anywhere under
  `github/` (`grep -n "commits/\|GetCommit" github/*.go` — only unrelated hits). A new
  `GetCommit(ctx, owner, repo, sha) (*CommitInfo, error)` must be added, following
  `GetIssue`'s exact HTTP-based 404-vs-other-status pattern (`GET
  repos/{owner}/{repo}/commits/{sha}`, `newGHRequest`/`ghHTTPClient`).

**Recommendation**: `report_duplicate`'s verification step should (a) call
`github.ParseGitHubRef(duplicate_ref)` to classify + extract owner/repo/number-or-sha, then
(b) dispatch to `GetIssue` (exists), a new `GetPR` (small, HTTP-based, mirrors `GetIssue`), or
a new `GetCommit` (net-new, same pattern) based on `RefType`. All three should surface a
typed not-found distinction (a sentinel error, or callers checking `errors.Is` against a
`ErrGitHubRefNotFound`-style sentinel introduced alongside `GetCommit`) so `report_duplicate`
can map cleanly to FR4's `ErrInvalidArgument` (definitive 404) vs `ErrInternalError` (network/
auth/rate-limit, "retry" wording) — exactly mirroring `report_pr_created`'s existing split at
`tools_backlog.go:707-715` (`ErrInternalError` "could not verify PR #%d against GitHub —
retry: %v" vs `ErrInvalidArgument` "does not match... refusing to record").

## 6. MCP tool registration — request_review and report_pr_created blocks to model on

Both registered in `registerBacklogTools` (`server/mcp/tools_backlog.go:920`), one
`s.AddTool(mcpgo.NewTool(...), handlerFunc)` call per tool:

- `request_review` — `tools_backlog.go:957-983` (handler `h.requestReview`,
  `tools_backlog.go:337`). Args: `item_id` (string, required), `message` (string, required,
  ≤2000 chars), `verification_notes` (string, optional, ≤4000 chars).
- `report_pr_created` — `tools_backlog.go:1014-1037` (handler `h.reportPRCreated`,
  `tools_backlog.go:623`). Args: `item_id` (required), `pr_url` (required),
  `pr_number` (`mcpgo.WithNumber` + `mcpgo.Min(1)`, required), `summary` (required, ≤1000
  chars). Description explicitly documents the verify-before-trust contract and the
  idempotent-no-op-on-retry behavior — both are patterns `report_duplicate`'s description
  should copy (verify-before-mutate is FR3's own requirement; idempotency on retry is a
  natural corollary once `report_duplicate` also transitions status).

`report_duplicate`'s registration should follow the identical shape: `item_id` (required),
`duplicate_ref` (string, required — "PR/issue/commit URL"), `reason` (string, required,
some length cap, e.g. mirroring `report_pr_created`'s 1000-char `summary` cap). Per FR10, the
tool description must explicitly call out `INTERNAL_ERROR` → retry guidance, the same way
`report_pr_created`'s description implicitly relies on the error message text alone today —
`report_duplicate` should state it in the tool description itself, not just the error string,
since FR10 calls this out as a distinct requirement from FR4's error-shape.

## 7. skip_review_gate field — confirmed name and type

`item.SkipReviewGate` — `bool` on `BacklogItemData` (`session/repository.go:353`,
`SkipReviewGate bool`, no pointer — this is the read-side DTO returned by `GetBacklogItem`,
already used exactly this way at `tools_backlog.go:409`: `if item.SkipReviewGate { ... }`).
Note this is distinct from `BacklogItemUpdate.SkipReviewGate *bool` (`repository.go:504`,
pointer for partial-update presence on writes) — `report_duplicate`'s refusal check (FR6)
only ever reads `item.SkipReviewGate` (the plain `bool`), never writes it, so the pointer
variant is irrelevant here. Same field `request_review` already checks at line 409 — reuse
identically: `if item.SkipReviewGate { refuse }`.

## 8. Stuck-item notification path (AC10 / FR10) — already built, not just proposed

Important update to the `backlog-stuck-item-visibility` research doc's picture: that doc
(§3, written 2026-07-xx) evaluated three *options* (A/B/C) for whether stuck-state should be
persisted, and recommended a hybrid "durable open-row + notify-once" design (option B). As of
this codebase's current state, **that design has since been implemented**, not just
recommended — confirmed live:

- `session/domain/backlog.go` — `StuckReason` is now an 11-value validated string enum
  (`StuckReasonPRReadyUnmerged`, `StuckReasonReworkCap`, `StuckReasonAbandonedReview`,
  `StuckReasonStaleWork`, `StuckReasonBouncing`, `StuckReasonPushFailed`,
  `StuckReasonOrphanedTriage`, `StuckReasonAutonomousStuck`, `StuckReasonSpawnFailed`,
  `StuckReasonPlanNotApproved`, `StuckReasonPRPendingNoPR`, `StuckReasonReworkBlockedStale`,
  `StuckReasonPRNeedsFix` — 13 as counted in the current source, not 11; grew since the
  visibility project shipped).
- `session/backlog_remediation.go` — `RemediationDue`/`RemediationBlocked`/
  `RecordRemediationAttempt`/`ResetStuckRemediation` etc. back a durable
  `BacklogStuckState` row per `(item_id, reason)` with exponential backoff and a hard cap,
  exactly the option-B design.
- `session/backlog_lifecycle.go`'s stuck sweep (~line 1590-1648, `runStuckDetector` calls
  inside the same 60s-ticker-driven `ReconcileStuck` pipeline the prior research documented)
  runs a fixed battery of detectors every tick, including `pr_ready+merge_detection`
  (`l.ReconcilePRPending`, line 1631) and `pr_pending_no_pr` (line 1624) — both specifically
  about `pr_pending`-status items.

**What this means for FR10**: `report_duplicate`'s own verification failure (network/auth/
rate-limit against GitHub, logged as a warning per FR4) does **not** itself need a new
`StuckReason` or any new plumbing. If verification fails, `report_duplicate` makes **no
mutation** (FR6) — the item is left in whatever status it already had:
- If it was `in_progress` with the calling work session still alive, the item isn't "stuck"
  in any sense the reconciler cares about yet — normal `StuckReasonStaleWork` detection
  (2h no-progress threshold, pre-existing) eventually catches it if the session goes idle.
- If it was `pr_pending`, the item already has a real, previously-recorded PR (that's how it
  reached `pr_pending`) and `ReconcilePRPending`'s per-tick poll of the item's *actual*
  recorded PR (not the failed `duplicate_ref` verification) continues regardless, independent
  of whether `report_duplicate` ever succeeds — `StuckReasonPRReadyUnmerged` /
  `StuckReasonPRNeedsFix` fire off the real PR's state, not off `report_duplicate`'s failed
  attempt. So a `pr_pending` item an agent gave up on re-flagging as a duplicate is not
  invisible — it was already being polled.

No new `StuckReason` constant is required to satisfy FR10 as written. The one concrete gap:
`report_duplicate`'s *own* logged verification-failure warning (FR4's "logged as a warning")
is not itself durably surfaced (it's a log line, like most transient-failure logging
elsewhere in this codebase) — but per FR10's actual wording ("must eventually surface through
the existing stuck-item notification path(s)"), the existing `pr_pending` polling machinery
already provides that "eventually surfaces" guarantee for the `pr_pending` case, satisfying
the AC without new detector work. If planning wants tighter coupling (e.g. distinguishing "PR
is fine but nobody's retried the duplicate report" from "PR itself needs attention"), that
would be a genuinely new `StuckReason` — flag as an explicit scope decision for planning, not
assume it's required.

## 9. WorkflowEngine / ADR-013 — confirmed landed, but a facade, not the fix point

`docs/adr/013-workflow-engine-replaces-valid-transitions.md` documents the decision and it
has shipped: `session/workflow_engine.go` defines a `WorkflowEngine` interface
(`CanTransition(from, to) bool`, `ValidateGates(item, to) error`,
`AllowedTransitions(from) []BacklogStatus`) with `DefaultWorkflowEngine` as the
implementation. But `DefaultWorkflowEngine` is a thin wrapper — `NewDefaultWorkflowEngine`
(`workflow_engine.go:24-34`) just deep-copies the same package-level `validTransitions` map
from `session/domain/backlog.go`, and `ValidateGates` (`workflow_engine.go:46-48`) delegates
straight to the same `TransitionGuard` function that predates ADR-013. `session/backlog.go:191-192`
still exposes `CanTransitionBacklog = domain.CanTransitionBacklog` as a direct passthrough —
neither wrapping changed behavior nor is `WorkflowEngine` even in the call path for
`request_review`/`report_duplicate`: `requestReview`'s handler calls
`h.storage.TransitionBacklogItemStatus` directly (§1), which enforces the CAS precondition at
the ent-repository layer, several steps removed from `WorkflowEngine`'s structural
from→to legality check. **Net effect: ADR-013 landing is irrelevant to where FR1's fix
goes** — confirmed directly in `session/domain/backlog.go:331-388`'s `validTransitions` map:
both `BacklogStatusInProgress: {BacklogStatusReview: true, ...}` (line 356) and
`BacklogStatusPRPending: {..., BacklogStatusReview: true, ...}` (line 372) already permit a
transition to `review` today — no map entry needs to change. The bug FR1 fixes is entirely in
the *caller-supplied CAS precondition value* passed to `TransitionBacklogItemStatus` (§1), not
in structural transition legality — confirms §1's diagnosis is complete and no
`WorkflowEngine`/`validTransitions` edit is needed.

## Recommendations: files to touch vs. net-new

**Touch (existing files, generalize/extend):**
- `server/mcp/tools_backlog.go`:
  - `requestReview` (line 337) — generalize precondition to `item.Status` restricted to
    `{in_progress, pr_pending}`; add the FR2 active-review-session guard (inline filter per
    §3) on the `pr_pending` path only.
  - `registerBacklogTools` (line 920) — add `report_duplicate` registration block, modeled
    on `report_pr_created`'s (§6).
  - New handler function `reportDuplicate` (new, but colocated in this same file next to
    `reportPRCreated`, same as every other backlog tool handler).
- `session/backlog.go` (line 90-94) — add `TriggeredByAgent = "agent"` constant.
- `github/repos.go` (or a new `github/commits.go`) — add `GetCommit(ctx, owner, repo, sha)`,
  copying `GetIssue`'s HTTP + 404-vs-other pattern (§5). Optionally add a small `GetPR`
  HTTP-based wrapper if `GetPRInfoCtx`'s CLI-shellout error shape proves too coarse for FR4
  during implementation — flag as a planning-phase call, not pre-decided here.

**Net-new (small, no schema/migration):**
- `report_duplicate`'s handler + MCP registration (server/mcp/tools_backlog.go) — the only
  genuinely new tool surface.
- Possibly a small sentinel error type (e.g. `ErrGitHubRefNotFound`) in `github/` if the PR/
  issue/commit verification dispatch wants one shared not-found signal instead of three
  separate ad-hoc checks — implementation detail, not architecturally load-bearing.

**Explicitly not needed:**
- No new `BacklogStatus` value, no ent schema/migration (FR8) — `report_duplicate` reuses
  `BacklogStatusReview` exactly as `request_review` already does.
- No new `StuckReason`/`BacklogStuckState` plumbing (§8) — existing `pr_pending` polling
  already provides FR10's "eventually surfaces" guarantee.
- No new ItemSession query/index — the active-review-session check is a Go-level filter over
  data `ListItemSessions` already returns (§3), following three existing precedents for the
  identical filter.
- No changes to `submit_review_verdict`, `session.ParseGitHubURL`, or
  `VerifyPRMatchesBranch` — all three are either out of scope (non-goal) or the wrong tool
  for this job and are being bypassed in favor of `github.ParseGitHubRef` (§5), not modified.
