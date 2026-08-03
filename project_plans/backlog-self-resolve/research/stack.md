# Stack Research: Backlog self-resolve (duplicate detection + request_review CAS fix)

Scope: backend-only Go — CAS-precondition fix in `request_review`
(`server/mcp/tools_backlog.go`) plus a new `report_duplicate` MCP tool that verifies a
GitHub PR/issue/commit URL before mutating backlog state. All findings below are VERIFIED
by reading the cited files directly; no external sources needed.

## Summary

No new external Go dependency is needed. Everything required — MCP tool registration,
GitHub verification, CAS transition, and audit-trail plumbing — already exists in-repo.
This is composition inside `server/mcp/` plus one new constant (`session/backlog.go`) and,
for commit-ref verification specifically, one small new function in the existing internal
`github/` package (same pattern as an existing sibling function, not a new dependency).

## 1. `session.TransitionBacklogItemStatus` / `session.BacklogItemPrecondition`

```go
// session/storage.go:736 (thin wrapper) -> session/ent_repository_backlog.go:869
func (s *Storage) TransitionBacklogItemStatus(
    ctx context.Context, id string, toStatus BacklogStatus,
    precondition *BacklogItemPrecondition, triggeredBy string,
) (*BacklogItemData, error)
```

```go
// session/repository.go:550
type BacklogItemPrecondition struct {
    ExpectedStatus    string     // CAS: current status must match, if non-empty
    ExpectedUpdatedAt *time.Time // CAS: updated_at must match, if non-zero
    Note              string     // stored in the status-event audit log for this transition
}
```

A CAS mismatch is not a distinct typed error — it surfaces as a generic error from
`TransitionBacklogItemStatus` (zero-rows-affected path); `request_review` currently just
wraps it via `errResult(ErrInternalError, ...)` (`tools_backlog.go:415-418`).

**FR1 impact**: today `requestReview` hardcodes
`ExpectedStatus: string(session.BacklogStatusInProgress)` (`tools_backlog.go:414`). FR1
requires reading `item.Status` (already loaded at `tools_backlog.go:404`) and using
*that* as `ExpectedStatus`, instead of the hardcoded constant.

Important existing fact: the domain state machine (`session/domain/backlog.go:361-376`,
`validTransitions` map) **already allows both `in_progress -> review` and
`pr_pending -> review`** — no change to `validTransitions` is needed for FR1/FR2/FR3, only
the handler's precondition-construction logic.

Concurrency-safety of the CAS primitive itself is already covered by existing tests —
`TestTransitionBacklogItemStatus_should_letExactlyOneWinnerThrough_When_TwoWritersRaceConcurrently`
and `_should_rejectStaleReopen_When_ItemAlreadyShippedSinceReview`
(`session/ent_repository_backlog_transition_test.go:27,78`) — no new locking primitive
needed.

## 2. `session.BacklogStatus` constants — confirmed no "duplicate" status

Canonical definitions, `session/domain/backlog.go:16-24` (re-exported as untyped aliases
in `session/backlog.go:17-25`):

```go
BacklogStatusIdea       BacklogStatus = "idea"
BacklogStatusRefining   BacklogStatus = "refining"
BacklogStatusReady      BacklogStatus = "ready"
BacklogStatusQueued     BacklogStatus = "queued"
BacklogStatusInProgress BacklogStatus = "in_progress"
BacklogStatusReview     BacklogStatus = "review"
BacklogStatusPRPending  BacklogStatus = "pr_pending"
BacklogStatusDone       BacklogStatus = "done"
BacklogStatusArchived   BacklogStatus = "archived"
```

No `duplicate`/`closed` value exists — confirms FR8/non-goal. `report_duplicate` should
route to `BacklogStatusReview` (never `done`/`archived` directly, per FR3), reusing the
already-valid `pr_pending -> review` transition noted above.

## 3. `TriggeredBy` — no "agent" value exists yet

```go
// session/backlog.go:90-93
// TriggeredBy values for BacklogStatusEvent records.
const (
    TriggeredByUser   = "user"
    TriggeredBySystem = "system"
)
```

Untyped string constants (not a defined type), stored in
`BacklogStatusEventData.TriggeredBy string` (`session/repository.go:315`) and persisted via
`recordStatusEvent(ctx, evClient, itemID, fromStatus, toStatus, triggeredBy, note string)`
(`session/ent_repository_backlog.go:45`). **FR7 requires adding
`TriggeredByAgent = "agent"`** next to these two. The column is a plain string (no ent
enum), so this is a one-line const addition with zero schema/migration impact — confirms
FR8's "no ent generate" requirement for this piece specifically. Every existing call site
passes a bare string literal constant, so adding a third is additive, no
exhaustiveness/switch to update.

## 4. `ItemSession` role / active-session fields

There is no single `type ItemSession struct` — the domain DTO is `ItemSessionSummary`:

```go
// session/repository.go:285-308
type ItemSessionSummary struct {
    ID, BacklogItemID, SessionUUID string
    Role               string     // SessionRoleWork | SessionRoleTriage | SessionRoleReview
    StartedAt          *time.Time
    EndedAt            *time.Time // nil == still active/unended
    VerificationNotes  string     // freeform evidence surfaced to reviewer at spawn time
    // ...AcSnapshot, commit tracking, TriageResult, ReviewVerdict — not needed here
}
```

Role constants, `session/backlog.go:50-52`:
```go
SessionRoleWork   = "work"
SessionRoleTriage = "triage"
SessionRoleReview = "review"
```

**"Active (unended) review-role session" detection (FR2) is exactly this pattern**,
already used elsewhere (e.g. `session/backlog_lifecycle.go:3353`,
`s.EndedAt == nil && (s.Role == SessionRoleWork || s.Role == SessionRoleReview)`):

```go
sessions, err := h.storage.ListItemSessions(ctx, itemID) // []ItemSessionSummary
for _, s := range sessions {
    if s.Role == session.SessionRoleReview && s.EndedAt == nil {
        // an active review session exists — refuse per FR2
    }
}
```
`Storage.ListItemSessions(ctx, itemID string) ([]ItemSessionSummary, error)` —
`session/storage.go:1098` (backed by `EntRepository.ListItemSessions`,
`session/storage_backlog.go:138`). No new storage method needed.

Other directly reusable primitives, both already used by `request_review`/
`report_pr_created` and directly applicable to `report_duplicate`:
- `Storage.GetItemSessionBySessionAndItem(ctx, sessionUUID, itemID string) (ItemSessionSummary, error)` (`session/storage.go:1000`) — verifies caller session is linked to the item (FR6); returns `session.ErrNotFound` if not linked → map to `ErrPermissionDenied`.
- `Storage.UpdateItemSessionVerificationNotes(ctx, id, verificationNotes string) error` (`session/storage.go:963`) — the exact path `request_review` already uses for `verification_notes`; `report_duplicate` should format `duplicate_ref` + `reason` into this same field (FR7), consistent with FR8's no-new-column constraint.
- `Storage.GetBacklogItem(ctx, itemID string) (*BacklogItemData, error)` — to read `SkipReviewGate` (FR6 refusal) and current `Status` (source for the CAS precondition per FR1).

## 5. GitHub URL parsing / verification — two parsers exist; only one supports issue/commit

**`session.ParseGitHubURL`** (`session/repo_path.go:93`, used today by `report_pr_created`)
returns `*session.GitHubRef{Owner, Repo, Branch, PRNumber, Type}` where `Type` is one of
`GitHubRefTypeRepo | GitHubRefTypeBranch | GitHubRefTypePR` (`session/repo_path.go:56-72`)
— **no issue or commit ref type**. Insufficient for FR3 as-is.

**`github.ParseGitHubRef`** (`github/url_parser.go:281`, package `github/`, imported
elsewhere as `githubpkg "github.com/tstapler/stapler-squad/github"`, e.g.
`server/mcp/tools_github.go:14`) is the generalized parser FR3 needs — it **already**
supports issue and commit refs:

```go
type RefType int
const (
    RefTypePR RefType = iota
    RefTypeBranch
    RefTypeRepo
    RefTypeFile
    RefTypeCommit  // <- already exists
    RefTypeIssue   // <- already exists
    RefTypeCompare
    RefTypeRelease
)

type ParsedGitHubRef struct {
    Type                    RefType
    Host, Owner, Repo       string
    PRNumber, IssueNumber   int
    CommitSHA               string
    // ...
}

func ParseGitHubRef(input string) (*ParsedGitHubRef, error)
```

**Answer to the open research question**: yes — a generalized parser already exists in
this repo. `report_duplicate` should use `github.ParseGitHubRef`, not
`session.ParseGitHubURL`.

### Verification-against-GitHub side (FR3's "verified before any mutation")

All GitHub API access in this repo goes through the internal `github/` package using
**plain `net/http` requests** (`newGHRequest`/`ghHTTPClient`,
`github/http_client.go`) — **not** the `gh` CLI and **not** an SDK like `google/go-github`.
Confirmed explicitly in a doc comment: `GetPRForBranch` — "Uses the GitHub REST API
directly (no gh subprocess) to avoid forkExec lock contention" (`github/client.go:376`).
(`session/repo_path.go`'s `EnsureRepoCloned` does shell out to the `git` CLI for
clone/fetch, per the `prefer-go-git-over-subshells.md` carve-out, but that's unrelated —
GitHub *API* calls use HTTP directly, not subprocess.)

- **PRs**: `github.GetPRInfoCtx(ctx, owner, repo, prNumber) (*PRInfo, error)` (`github/client.go:247`) confirms existence. (`VerifyPRMatchesBranch`, `tools_github.go:272`, additionally checks branch match — irrelevant for report_duplicate, since a duplicate PR need not be on this item's own branch.)
- **Issues**: `github.GetIssue(ctx, owner, repo, number) (*IssueResult, error)` (`github/repos.go:270`) already exists. It distinguishes error classes worth reusing for FR4's two-channel split: `ErrNotAuthenticated` sentinel when no token configured, and inline handling of HTTP 401 (auth), 404 (`"issue not found"`), 403 w/ `Retry-After` (secondary rate limit) — but these are currently plain `fmt.Errorf` messages, not typed sentinels distinguishable via `errors.Is` (only `ErrNoPR`, `github/client.go:24`, and `ErrNotAuthenticated` are typed sentinels today). Planning should decide: either pattern-match on error text/HTTP status in the handler, or give `GetIssue` a typed `ErrIssueNotFound`-style sentinel analogous to `ErrNoPR` so FR4's split is a clean `errors.Is` check. Not yet resolved by existing code — flag for the plan phase.
- **Commits**: **no existing function.** `grep -rn "commits/" github/*.go` returns no hits — no `GetCommit`/commit-lookup exists anywhere under `github/`. Implementing commit-ref verification for FR3 requires one small new function in the `github` package (e.g. `GetCommit(ctx, owner, repo, sha string) (*CommitResult, error)` hitting `repos/%s/%s/commits/%s`), following `GetIssue`'s exact pattern (`github/repos.go:268-306`: build request via `newGHRequest`, dispatch via `ghHTTPClient`, branch on 401/404/403). This is new code in an *existing* package, not a new dependency — size it explicitly in the plan.

### Reference pattern to mirror

`reportPRCreated` (`server/mcp/tools_backlog.go:623-726`) is the complete existing
precedent for FR3/FR4's shape end-to-end: args parsing → role check
(`itemSession.Role != session.SessionRoleWork` → `ErrPermissionDenied`) → parse ref →
resolve/verify against GitHub → `errResult(ErrInvalidArgument, ...)` for a
parse/definitive-mismatch failure vs
`errResult(ErrInternalError, fmt.Sprintf("... — retry: %v", err), "")` for a verification
call that itself failed transiently (`tools_backlog.go:707-715` is the literal two-channel
split FR4 asks to mirror). `report_duplicate` should follow this shape almost verbatim.

## 6. MCP tool registration + error helpers

- Tools are registered via `s.AddTool(mcpgo.NewTool(name, mcpgo.WithDescription(...), ...), handlerFunc)` inside `registerBacklogTools` (`server/mcp/tools_backlog.go:920`) — a new `report_duplicate` entry follows the same shape as the existing `request_review`/`report_pr_created` registrations there. Handler signature: `func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`.
- `func errResult(code, message, remediation string) *mcpgo.CallToolResult` — `server/mcp/tools_discovery.go:73`.
- Error codes already defined and sufficient for FR4/FR6 (no new code needed):
  ```go
  ErrInvalidArgument  = "INVALID_ARGUMENT"   // server/mcp/types.go:63
  ErrInternalError    = "INTERNAL_ERROR"     // server/mcp/types.go:64
  ErrPermissionDenied = "PERMISSION_DENIED"  // server/mcp/tools_backlog.go:57
  ErrItemNotFound     = "ITEM_NOT_FOUND"     // server/mcp/tools_backlog.go:58
  ```
- Standard handler prologue already established by every existing backlog tool handler and directly reusable: (1) `callerSessionUUID(ctx)` → `ErrPermissionDenied` if missing; (2) arg extraction/validation → `ErrInvalidArgument`; (3) `GetItemSessionBySessionAndItem` for the "linked to item" check (FR6); (4) `itemSession.Role != session.SessionRoleWork` for the work-only gate (FR6), exact same check `reportPRCreated` uses at `tools_backlog.go:669-671`.

## 7. No new external dependency required (FR8)

Confirmed via `go.mod`: **no `google/go-github` or any GitHub REST/GraphQL SDK is a
dependency anywhere in this repo** (only incidental transitive `github.com/google/*`
packages unrelated to GitHub's API, e.g. `google/uuid`, `google/cel-go`). All GitHub API
access goes through the hand-rolled `github/` package described above.
`github.com/mark3labs/mcp-go v0.48.0` (`go.mod`) is the only MCP framework dependency,
already used by both `tools_backlog.go` and `tools_github.go` — no new import needed for
tool registration either.

**Confirmed: this feature needs zero new external Go dependencies.** Everything required
— CAS transition primitive, status/role/triggered-by constants (plus one additive
constant), GitHub ref parsing (issue/commit already supported by `github.ParseGitHubRef`),
GitHub verification client (PR/issue covered; commit needs one new function in the
existing internal `github` package) — is already present in `session/` and `github/`. The
only net-new code is: the `report_duplicate` MCP tool handler + registration in
`server/mcp/tools_backlog.go`, one new `TriggeredByAgent` constant in
`session/backlog.go`, and (if commit-ref duplicates are in scope) one new
`GetCommit`-style function in `github/repos.go` mirroring `GetIssue`.

## Summary table

| Need | Type/Func | Location | Status |
|---|---|---|---|
| CAS transition | `Storage.TransitionBacklogItemStatus(ctx, id, toStatus, precondition, triggeredBy)` | `session/storage.go:736` | exists, reuse as-is |
| CAS precondition struct | `BacklogItemPrecondition{ExpectedStatus, ExpectedUpdatedAt, Note}` | `session/repository.go:550` | exists, reuse as-is |
| Status constants | `BacklogStatus{Idea..Archived}` | `session/domain/backlog.go:16` | exists, no "duplicate" value, none needed |
| pr_pending/in_progress -> review transition | `validTransitions` map | `session/domain/backlog.go:361,369` | already valid, no domain change needed |
| Audit trigger constant | `TriggeredByUser`, `TriggeredBySystem` | `session/backlog.go:90-93` | need to add `TriggeredByAgent = "agent"` |
| Active review session check | `ItemSessionSummary{Role, EndedAt}` + `ListItemSessions` | `session/repository.go:285`, `session/storage.go:1098` | exists, reuse as-is |
| Session-linked-to-item check | `GetItemSessionBySessionAndItem` | `session/storage.go:1000` | exists, reuse as-is |
| Persist duplicate_ref/reason | `UpdateItemSessionVerificationNotes` | `session/storage.go:963` | exists, reuse as-is |
| Generalized ref parser (PR/issue/commit) | `github.ParseGitHubRef` -> `RefTypePR/Issue/Commit` | `github/url_parser.go:281` | exists, use instead of `session.ParseGitHubURL` |
| PR verification | `github.GetPRInfoCtx` | `github/client.go:247` | exists, reuse |
| Issue verification | `github.GetIssue` | `github/repos.go:270` | exists; error classes not yet typed sentinels — may need light rework for a clean two-channel split |
| Commit verification | — | — | does not exist; add one function, same pattern as `GetIssue` |
| MCP tool registration | `s.AddTool(...)` in `registerBacklogTools` | `server/mcp/tools_backlog.go:920` | exists, follow existing pattern |
| Error/result helper | `errResult(code, message, remediation)` | `server/mcp/tools_discovery.go:73` | exists, reuse as-is |
| New external dependency | — | — | none required |
