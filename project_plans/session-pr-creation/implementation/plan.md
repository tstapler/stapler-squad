# Implementation Plan: session-pr-creation

**Feature**: Mechanical (non-agentic) "Create PR" modal for any session with a
worktree — replaces the dashboard's `RunOneShot`-driven one-click button with
a pre-fill/edit modal backed by the same `GitWorktree.CreatePR` +
`headless.DraftPRDescription` primitives the backlog automation path
(`pushAndCreatePR`) already uses.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: ADR-001-keep-gh-cli-shell-out (confirms build-vs-buy.md's conclusion)

---

## Step 0.5 — Alternatives Explored

Three shapes were considered for how the new PR-creation surface talks to the
backend:

1. **Two unary RPCs — `DraftPullRequest` (read-only) + `CreatePullRequest`
   (mutating)** (chosen). Strength: matches CQS — the preview step can never
   have a side effect, so a client can safely re-call it (e.g. to refresh a
   stale draft) without risk. Weakness: two round trips instead of one, and
   two proto messages/handlers to maintain instead of one.
2. **One RPC with a `dry_run: bool` flag.** Strength: fewer proto
   messages, one handler to test. Weakness: every caller must remember to
   check `dry_run` before trusting the response is side-effect-free; a caller
   bug (forgetting the flag on a "just refresh the preview" retry) would
   silently create a duplicate PR — an unacceptable failure mode given AC4's
   duplicate-avoidance requirement is precisely what a stray real call would
   violate.
3. **Single `CreatePullRequest` RPC with no separate preview step; modal
   fetches title/body itself via the existing `GetSessionDiff` RPC and drafts
   client-side or skips the LLM draft entirely.** Strength: simplest possible
   backend, no new "draft" RPC at all. Weakness: violates AC1 directly — AC1
   requires the body be "generated via the existing `headless.DraftPRDescription`
   call," which only runs server-side (it needs the headless LLM pool); the
   modal cannot pre-fill a drafted body without a server round trip.

**Chosen: option 1** (two RPCs). Rejected alternatives and reasons are also
recorded in the Pattern Decisions table below.

---

## Post-Review Revisions (blockers resolved 2026-08-06)

Both review passes ran against the plan above and returned blocking findings.
Per `sdd:3-plan`'s repair-loop requirement, all BLOCKER findings are resolved
below before proceeding to `sdd:4-validate`. Re-review verdicts recorded at
the bottom of `architecture-review.md`/`adversarial-review.md`.

1. **[architecture BLOCKER] nil `s.headlessPool` panic (Task 1.3.1c).**
   Amend Task 1.3.1c: wrap the `DraftPRDescription` call in `if
   s.headlessPool != nil { body, draftErr = headless.DraftPRDescription(...)
   } else { body = fallbackBody }`, mirroring `RunOneShot`'s existing guard
   at `session_service.go:3653`. Add
   `TestDraftPullRequest_should_UseFallbackBody_When_HeadlessPoolNil` to Epic
   1.5.

2. **[adversarial BLOCKER] Preview diff scope mismatch (committed-only vs.
   working-tree-inclusive).** Amend Task 1.3.1c: `DraftPullRequest` no longer
   calls `session.GetGitDiff` (committed-only, `baseSHA..HEAD`). It instead
   computes its preview diff via the same working-tree-inclusive path
   `SessionService.GetSessionDiff`/`GitWorktree.Diff()` already uses
   (`session/git/diff.go:43-90`, which runs `git add -N .` first to pick up
   untracked files) — this is exactly the diff the user already sees in the
   session card's diff viewer before clicking "Create PR," so the drafted
   title/body always describes what the user is actually looking at. Amend
   Task 1.4.1b: keep its original `wt.CommitChanges(...)`-then-`PushBranch()`
   sequence (unchanged from the original plan — `CreatePullRequest` is the
   *only* handler that commits), but its `CommitChanges` error is no longer
   swallowed ("logged, not failing") — it now returns `connect.CodeInternal`
   with the commit error's literal message (AC6), since a manual
   review-then-publish flow cannot silently drop in-scope changes the user
   just reviewed the way the unattended backlog path (`pushAndCreatePR`) is
   allowed to.
   **`DraftPullRequest` itself makes zero git-mutating calls — it stays
   strictly read-only**, preserving the plan's own Step 0.5 CQS rationale for
   splitting into two RPCs ("the preview step can never have a side effect").
   *(Revised 2026-08-06 after `sdd:4-validate`'s pre-mortem flagged the
   original version of this fix — which had `DraftPullRequest` call a shared
   `ensureCommitted` before drafting — as itself a new P1 risk: it made
   opening the modal silently commit whatever was in the working tree,
   including a possibly-still-running agent's in-progress writes. This
   working-tree-diff approach achieves the same "preview matches what ships"
   goal without giving the preview RPC a side effect.)*
   Add `TestDraftPullRequest_should_PreviewWorkingTreeDiff_When_UncommittedChangesPresent`
   (asserts the drafted body reflects uncommitted changes and that
   `wt.CommitChanges` is never called during `DraftPullRequest`) and
   `TestCreatePullRequest_should_SurfaceError_When_CommitFails` to Epic 1.5.

3. **[adversarial BLOCKER + architecture CONCERN, same root cause] New RPC
   logic bolted onto `SessionService` instead of an extracted domain
   service.** Both reviews independently flagged the same gap: the codebase's
   established convention (`reviewQueueSvc`, `workspaceSvc`, `backlogSvc`,
   `checkpointSvc`, each with their own `sync.Map`-shaped in-flight guards
   living on the extracted service, not on `SessionService`) is not followed
   by Epics 1.3/1.4 as drafted. Resolution: **new Epic 1.0** below extracts a
   `PRCreationService`. Every subsequent reference in Epics 1.3/1.4 to a
   method "on `*SessionService`" now means a method on the new
   `*PRCreationService` instead — same handler bodies, same task numbering,
   same file list plus `server/services/pr_creation_service.go` (new) added
   to each task's Files line. `SessionService` gains only two thin
   delegators, matching `GetPRInfo`'s existing one-line-delegator shape
   (`session_service.go:2813`).

### Epic 1.0: Extract `PRCreationService`

**Goal**: Give `DraftPullRequest`/`CreatePullRequest` a home matching the
codebase's existing extracted-service pattern, per Post-Review Revision #3.

##### Task 1.0a: Create `PRCreationService` struct + constructor (~4 min)
- New file `server/services/pr_creation_service.go`. Struct fields:
  `storage`, `eventBus`, `headlessPool *headless.Pool`,
  `backlogLifecycleListener` (same types `SessionService` already holds for
  these — copy the field types verbatim), `findInstance func(string)
  *session.Instance` (a narrow function value, not the whole
  `SessionService`, so this service depends only on what it needs — per
  `interface-pollution-checklist.md`'s "define the interface/dependency
  where it's consumed, scoped narrowly" guidance), `prCreationInFlight
  sync.Map`.
- Constructor `NewPRCreationService(storage ..., eventBus ..., headlessPool
  *headless.Pool, backlogLifecycleListener ..., findInstance func(string)
  *session.Instance) *PRCreationService`.
- Files: `server/services/pr_creation_service.go` (new)

##### Task 1.0b: Wire into `SessionService` with thin delegators (~3 min)
- Add `prCreationSvc *PRCreationService` field to `SessionService`'s struct
  (`session_service.go:74-140` region, alongside `reviewQueueSvc`/`githubSvc`
  etc.), constructed in `SessionService`'s own constructor by passing
  `s.findInstance` (a bound method value) and the existing shared
  dependencies.
- Add the two delegator methods:
  ```go
  func (s *SessionService) DraftPullRequest(ctx context.Context, req *connect.Request[sessionv1.DraftPullRequestRequest]) (*connect.Response[sessionv1.DraftPullRequestResponse], error) {
      return s.prCreationSvc.DraftPullRequest(ctx, req)
  }
  func (s *SessionService) CreatePullRequest(ctx context.Context, req *connect.Request[sessionv1.CreatePullRequestRequest]) (*connect.Response[sessionv1.CreatePullRequestResponse], error) {
      return s.prCreationSvc.CreatePullRequest(ctx, req)
  }
  ```
- Files: `server/services/session_service.go`

Epics 1.3 and 1.4 below are unchanged in content — read every "add a method
on `*SessionService`" instruction as "add a method on `*PRCreationService`
(`server/services/pr_creation_service.go`)," and every task's Files list
gains that new file in place of `server/services/session_service.go` for the
handler-body tasks (1.3.1a-d, 1.4.1a-d). `resolveSessionWorktree` and
`prCreationInFlight` are private to `PRCreationService`, not `SessionService`.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `DraftPullRequest` | New unary RPC on `SessionService`. Read-only: resolves the session's worktree, checks for an existing PR and commits-ahead status, and returns a pre-filled `{title, body, base_branch}` for the modal. | No git/GitHub side effects. |
| `CreatePullRequest` | New unary RPC on `SessionService`. Mutating: pushes the branch, calls `GitWorktree.CreatePR` with the user-edited title/body/base branch, persists the result on the session, and publishes a session-updated event. | The mechanical path AC3 requires. |
| `GitWorktree.CreatePR` | Existing method (`session/git/worktree_git.go:329`) that shells out to `gh pr create`. Plan changes its signature to add a `baseBranch string` parameter. | Already idempotent via `findExistingPR`. |
| `already_existed` | New `bool` field on `CreatePullRequestResponse`. `true` when `CreatePR` returned a pre-existing PR instead of creating a new one. | Lets the modal say "Updated" vs. "Created". |
| `persisted` / `persist_error` | New fields on `CreatePullRequestResponse`. `persisted=false` + `persist_error=<msg>` signal that the PR was created on GitHub but `SaveInstances` failed to record it on the session. | BUG-040-style partial-failure signal, returned as a *successful* response, not a connect error. |
| `prCreator` | Existing interface (`session/backlog_lifecycle.go:246`) that `pushAndCreatePR` depends on. Gains the `baseBranch` parameter on its `CreatePR` method signature. | Not a new interface — existing, justified (2 implementations: prod + test fake). |
| in-flight guard | New `sync.Map[string]bool`-shaped field on `SessionService`, keyed by session ID, set at the top of `CreatePullRequest` and cleared via `defer`. | Rejects a concurrent second `CreatePullRequest` call for the same session with a clear error, per pitfalls.md's race-condition recommendation. |
| `CreatePullRequestModal` | New React component (`web-app/src/components/sessions/CreatePullRequestModal.tsx`). Owns title/body/base-branch form state, calls `draftPullRequest`/`createPullRequest`, and renders the "View PR" state when a PR already exists. | Replaces the old `onRunOneShot` button + `ReviewQueuePanel`'s inline `prModal`. |
| `GitVCSReader` | Existing zero-dependency struct (`session/unfinished/git_vcs_reader.go:16`) implementing `ResolveDefaultBranch(repoPath string) string` via `git symbolic-ref`/candidate-branch probing. | Instantiated directly (`&unfinished.GitVCSReader{}`) inside the new handler — no `Scanner` dependency needed. |
| `RecordPRCreatedOutOfBand` | Existing method (`session/backlog_lifecycle.go:4104`) that syncs a backlog item's status when its PR was created outside `pushAndCreatePR`. | Called unconditionally from `CreatePullRequest`, mirroring `RunOneShot`'s existing call — no-op for non-backlog sessions. |
| `HasCommitsAheadOfMain` | Existing method (`session/git/worktree_git.go:428`). Fail-open pre-flight check for "does this branch have anything to ship." | Reused, unmodified, by `DraftPullRequest`'s gating logic. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| RPC shape | Two unary RPCs (CQS: read-only draft, mutating create) | architecture.md §2 | Single RPC with `dry_run: bool` flag | A caller bug forgetting the flag on a preview-refresh call would silently create a duplicate PR — unacceptable given AC4. Two RPCs make "no side effects" a type-level guarantee of which method you called, not a runtime flag to remember. |
| `CreatePR`'s new base-branch parameter | Plain `string` | type-driven-design skill | Newtype wrapper `type BranchName string` | No validation/parsing invariant exists to enforce — `gh pr create --base` already validates the ref and returns its own error on an invalid one. Wrapping a passthrough string with no invariants is the "unjustified generic/wrapper" smell (`interface-pollution-checklist.md` #5), not type-driven design. |
| Created-vs-reused signal | `bool already_existed` field | pitfalls.md §3.6, architecture.md | Sum type / enum `PrCreationStatus{CREATED, REUSED}` | Exactly two mutually exclusive, permanently binary states — PR *lifecycle* status (open/merged/closed) is already a separate concern covered by the existing `GetPRStatus` RPC. A bool is the minimal correct type; an enum here would be over-modeling a space that will never grow a third state. |
| Persist-failure signalling | Response fields `persisted bool` + `persist_error string`, RPC still returns success | pitfalls.md §1 (BUG-040 analogy) | `connect.CodeInternal` error after `CreatePR` already succeeded | An error response would read to the frontend as "PR creation failed," inviting a retry that could create a second PR; the PR is real and already on GitHub. The correct signal is a structured partial-success field the modal can render as a distinct warning, exactly as pitfalls.md recommends. |
| Concurrent double-click / two-tab protection | `sync.Map[string]bool` in-flight guard on `SessionService`, keyed by session ID | pitfalls.md §3 recommendation (a) | Per-session `sync.Mutex` map tied to `Instance` lifecycle | `Instance` already has actor-routed locking (`sendSyncErr`) for its own state; layering a second, differently-scoped lock onto the same struct risks lock-ordering confusion. A short-lived, RPC-handler-local guard is simpler and independently removable. |
| Frontend modal structure | New extracted component `CreatePullRequestModal.tsx` | ux.md §1, stack.md testing note | Inline 6th dialog block inside `SessionActionsOverflow.tsx` | The file is already 882 lines; the new dialog needs materially more state (draft-loading, submit-loading, 3 editable fields, existing-PR branch) than any sibling dialog, and — unlike the other 5 dialogs — must be reused verbatim in `ReviewQueuePanel.tsx`, which today reimplements its own ad hoc prompt-editing modal (`prModal`) for the same use case. Extraction avoids a second bespoke implementation. |
| Default base branch resolution | Call `(&unfinished.GitVCSReader{}).ResolveDefaultBranch(repoPath)` directly inside the handler | architecture.md §3a | Inject a `*unfinished.Scanner` into `SessionService` | `Scanner` carries caching, an event bus, background scan goroutines, and dismiss/snooze stores this feature needs none of. `GitVCSReader` is a zero-field struct with no constructor dependencies — instantiating it inline avoids growing `SessionService`'s constructor for one stateless method call. |
| Removing the old "Create PR" button (AC7) | Full removal of the `RunOneShot`-driven UI entry points (both `SessionActionsOverflow`'s overflow item and `ReviewQueuePanel`'s inline `prModal`); backend `RunOneShot`/`RunOneShotForSession` RPC is kept | pitfalls.md §3 ("real correctness risk, not just UX") | Demote to an "Advanced ▸" submenu | A demoted button is still clickable, so the concurrent-mutation race (two independent commit/push/`gh pr create` sequences with no coordination) still exists. Only removing the entry point closes it. The RPC itself stays because `RunOneShotForSession` still backs the opt-in `AutoCreatePR` review-queue policy (out of scope to change). |
| Scope of "active worktree" (AC1) | Live instances only (`s.findInstance(id) != nil`) | architecture.md §3 (flagged as open) | Also support non-live/completed sessions via `git.NewGitWorktreeFromStorage` reconstruction (as `GetSessionDiff` does) | AC1's own wording ("session with an active worktree") and the UX research's JTBD framing (handoff moment at the end of an *active* session) point at live sessions. Reconstructing for a completed session adds scope the acceptance criteria don't ask for — recorded as a follow-up in Unresolved Questions, not built now. |

---

## Migration Plan

Omitted — no schema/data changes. Proto changes are additive (`make
proto-gen` regenerates bindings) and are not a data migration.

## Observability Plan

- **Logs**: `CreatePullRequest` logs at the same `log.InfoLog`/`log.WarningLog`
  granularity `pushAndCreatePR` already uses — one info line on success
  (`"[SessionService] CreatePullRequest session=%s PR #%d %s (already_existed=%v)"`),
  one warning line on a persist failure (mirrors `RunOneShot`'s existing
  `log.Warn("RunOneShot: failed to persist PR URL", ...)` call site,
  `server/services/session_service.go:3693`), one info line when the in-flight
  guard rejects a concurrent call.
- **Metrics**: None new. No metrics/telemetry emission exists at this RPC
  layer today (confirmed: `RunOneShot` emits none) — consistent with the rest
  of `SessionService`'s mutating RPCs; adding metrics here would be
  inconsistent scope creep.
- **Alerts**: None. Single-operator localhost/Tailscale tool with no
  alerting infra for this RPC surface (same reasoning pitfalls.md gives for
  "no new auth layer needed").

## Risk Control

- **Feature flag**: None. This is an additive RPC pair plus a straight swap
  of one button's wiring in a UI with no existing flag-gating mechanism for
  individual overflow-menu items — adding one would be new infrastructure
  disproportionate to the change.
- **Rollback procedure**: `git revert` the single PR. The existing
  `RunOneShot`/`GitHubPrUrl` persistence path is untouched by this change (the
  new RPC reuses the same `SetGitHubPR`/`SaveInstances`/event-publish calls),
  so reverting does not orphan any session state.
- **Staged rollout**: N/A — no phased-deployment infrastructure exists in
  this repo (single systemd user service). Ship behind normal PR review +
  `make ci` only.

## Unresolved Questions

- [ ] Should PR creation be offered for non-live/completed sessions
  (reconstructed worktree via `git.NewGitWorktreeFromStorage`, as
  `GetSessionDiff` does)? — blocks a possible fast-follow story, not any
  story in this plan — owner: product/Tyler.
- [ ] Should `DiffViewer.tsx` get a second "Create PR" entry point in
  addition to the overflow menu? — blocks nothing in this plan (AC1's
  "session card" wording is satisfied by the overflow-menu placement alone)
  — owner: UX follow-up.
- [ ] Rule-driven auto-PR-on-complete for non-backlog sessions — explicitly
  out of scope per requirements.md; noted here only so it isn't lost.

## Dependency Visualization

```
Phase 1: Backend mechanical foundation
  Epic 1.0 (extract PRCreationService) ──────────────────────► (feeds 1.3, 1.4)
  Epic 1.1 (CreatePR base-branch signature) ──┐
  Epic 1.2 (proto + make proto-gen) ──────────┼──► Epic 1.3 (DraftPullRequest handler)
                                               │           │
                                               └──► Epic 1.4 (CreatePullRequest handler) ◄──┘
                                                           │
                                                           ▼
                                                   Epic 1.5 (Go tests)
                                                           │
                                                           ▼
                                                   Epic 1.6 (backend registry entries)

Phase 2: Frontend (depends on Phase 1's proto types existing)
  Epic 2.1 (CreatePullRequestModal component)
          │
          ├──► Epic 2.2 (useSessionService hook additions) [parallel with 2.1's skeleton]
          │
          ▼
  Epic 2.3 (wire into SessionActionsOverflow, AC7)
          │
          ▼
  Epic 2.4 (wire into ReviewQueuePanel, AC7)
          │
          ▼
  Epic 2.5 (remove dead onRunOneShot plumbing, AC7)
          │
          ▼
  Epic 2.6 (frontend registry entry)

Phase 3: Tests (depends on Phase 1 + Phase 2 code existing)
  Epic 3.1 (frontend Jest tests) ──┐
  Epic 3.2 (Playwright e2e spec) ──┴──► Epic 4.1 (make registry-generate, verify no gap growth)
                                              │
                                              ▼
                                        Epic 4.2 (make ci gate)
```

---

## Phase 1: Backend Mechanical Foundation

### Epic 1.1: `CreatePR` gains a base-branch parameter

**Goal**: Close the AC3 gap — `CreatePR` currently has no way to target a
non-default base branch, so the modal's base-branch field would otherwise be
UI-only and silently ignored.

#### Story 1.1.1: `GitWorktree.CreatePR` accepts and forwards `baseBranch`

**As a** developer wiring the new mechanical RPC, **I want**
`GitWorktree.CreatePR` to accept a base branch and pass it to `gh pr create
--base`, **so that** AC3's "base-branch selection in the UI" actually takes
effect.

**Acceptance Criteria**:
- AC3 (mechanical path, base branch honored): *Given* a `GitWorktree` for
  branch `feature/rate-limit-toggle` with a title "Add rate limit toggle" and
  body "Adds a per-user rate limit toggle.", *When*
  `g.CreatePR("Add rate limit toggle", "Adds a per-user rate limit toggle.", "release/1.2")`
  is called, *Then* the `gh` invocation's args include `--base release/1.2`
  (verified by asserting on `cmdExec`'s captured `*exec.Cmd.Args` in the
  updated test, following `worktree_git_test.go`'s existing fake-executor
  pattern).

**Files**: `session/git/worktree_git.go`, `session/git/worktree_git_test.go`,
`session/backlog_lifecycle.go`

##### Task 1.1.1a: Change `CreatePR`'s signature and append `--base` (~4 min)
- In `session/git/worktree_git.go:329`, change
  `func (g *GitWorktree) CreatePR(title, body string) (prURL string, prNumber int, err error)`
  to `func (g *GitWorktree) CreatePR(title, body, baseBranch string) (prURL string, prNumber int, err error)`.
- At line 346, change
  `args := []string{"pr", "create", "--title", title, "--body", body, "--head", g.branchName}`
  to append `"--base", baseBranch` when `baseBranch != ""` (leave `gh`'s own
  default-branch resolution as the fallback when empty, preserving today's
  behavior for every existing caller).
- Update the doc comment above `CreatePR` (line 326-328) to document the new
  parameter and its empty-string behavior.
- Files: `session/git/worktree_git.go`

##### Task 1.1.1b: Update the `prCreator` interface and its production factory (~2 min)
- In `session/backlog_lifecycle.go:249`, change the `prCreator` interface's
  `CreatePR(title, body string) (prURL string, prNumber int, err error)` to
  `CreatePR(title, body, baseBranch string) (prURL string, prNumber int, err error)`.
- No change needed to `defaultPRCreatorFactory` (line 258) — it returns the
  concrete `*git.GitWorktree`, which now satisfies the updated interface
  automatically.
- Files: `session/backlog_lifecycle.go`

##### Task 1.1.1c: Update `pushAndCreatePR`'s call site to pass `""` (~2 min)
- In `session/backlog_lifecycle.go:3658`, change
  `prURL, prNumber, prErr = g.CreatePR(prTitle, prBody)` to
  `prURL, prNumber, prErr = g.CreatePR(prTitle, prBody, "")` — preserves
  today's implicit-default-branch behavior for the backlog automation path
  exactly (requirements.md explicitly puts "any change to the backlog
  automation path itself" out of scope; passing `""` is a no-behavior-change
  update, not a functional change).
- Files: `session/backlog_lifecycle.go`

##### Task 1.1.1d: Update/add Go tests covering the new parameter (~5 min)
- Update any existing `worktree_git_test.go` call sites of `CreatePR(...)` to
  pass a third argument (`""` where the test doesn't care about base branch).
- Add `TestGitWorktree_CreatePR_PassesBaseBranch_When_NonEmpty` asserting the
  captured `gh` command args contain `--base <value>`.
- Add `TestGitWorktree_CreatePR_OmitsBaseFlag_When_Empty` asserting no
  `--base` flag appears when `baseBranch == ""` (regression guard for Task
  1.1.1c's backward-compat requirement).
- Run `go test ./session/git/... -run TestGitWorktree_CreatePR` to confirm.
- Files: `session/git/worktree_git_test.go`

---

### Epic 1.2: Proto messages and RPCs

**Goal**: Add `DraftPullRequest`/`CreatePullRequest` to the `SessionService`
proto definition and regenerate bindings, following the `RunOneShot`
precedent exactly (flat request/response fields, no nested messages).

#### Story 1.2.1: Add proto messages and RPC declarations

**Acceptance Criteria**:
- AC1 (modal pre-fill fields exist on the wire): *Given* the regenerated
  TypeScript bindings, *When* a frontend caller inspects
  `DraftPullRequestResponse`, *Then* it has `title: string`, `body: string`,
  `baseBranch: string`, `hasCommitsAhead: boolean`, `existingPrUrl: string`,
  `existingPrNumber: number` fields (camelCase per `@bufbuild/protobuf`'s
  codegen convention, matching every other generated message in
  `web-app/src/gen/session/v1/session_pb.ts`).

**Files**: `proto/session/v1/session.proto`

##### Task 1.2.1a: Add `DraftPullRequestRequest`/`Response` messages (~3 min)
- In `proto/session/v1/session.proto`, immediately after the existing
  `RunOneShotResponse` message (ends at line 1959), add a new section:
  ```protobuf
  // ============================================================================
  // S5: Mechanical Pull Request Creation Messages
  // ============================================================================

  message DraftPullRequestRequest {
    string session_id = 1;
  }

  message DraftPullRequestResponse {
    string title = 1;
    string body = 2;
    string base_branch = 3;
    bool has_commits_ahead = 4;
    // Non-empty when the session already has a PR — the client should show
    // a "View PR" state instead of the editable create form (AC4).
    string existing_pr_url = 5;
    int32 existing_pr_number = 6;
  }
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.2.1b: Add `CreatePullRequestRequest`/`Response` messages (~3 min)
- Immediately after `DraftPullRequestResponse`, add:
  ```protobuf
  message CreatePullRequestRequest {
    string session_id = 1;
    string title = 2;
    string body = 3;
    string base_branch = 4;
  }

  message CreatePullRequestResponse {
    string pr_url = 1;
    int32 pr_number = 2;
    // True when CreatePR returned a pre-existing PR instead of creating one.
    bool already_existed = 3;
    // False when the PR was created on GitHub but the session record failed
    // to save it — the RPC still returns success (the PR is real); the
    // client should surface persist_error as a distinct warning, not a
    // create-failed error (see plan.md's Pattern Decisions).
    bool persisted = 4;
    string persist_error = 5;
  }
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.2.1c: Add the two RPC declarations to `service SessionService` (~2 min)
- Immediately after `rpc RunOneShot(RunOneShotRequest) returns (RunOneShotResponse) {}`
  (`proto/session/v1/session.proto:281`), add:
  ```protobuf
  // Mechanical (non-agentic) PR creation, replacing the RunOneShot-driven
  // dashboard button (S5).
  rpc DraftPullRequest(DraftPullRequestRequest) returns (DraftPullRequestResponse) {}
  rpc CreatePullRequest(CreatePullRequestRequest) returns (CreatePullRequestResponse) {}
  ```
- Files: `proto/session/v1/session.proto`

##### Task 1.2.1d: Regenerate bindings and commit generated files (~3 min)
- Run `make proto-gen` from repo root.
- Verify `gen/proto/go/session/v1/session.pb.go` and
  `web-app/src/gen/session/v1/session_pb.ts` (plus `session_connect.ts`) now
  contain the new messages/RPC methods.
- Per MEMORY.md's `instinct_alias_session.md` note, `web-app/src/gen` is
  tracked despite `.gitignore` — stage and commit the regenerated files
  alongside the proto source in this task's eventual commit (do not rely on
  CI to regenerate).
- Files: `gen/proto/go/session/v1/session.pb.go`,
  `gen/proto/go/session/v1/session_grpc.pb.go` (if present),
  `web-app/src/gen/session/v1/session_pb.ts`,
  `web-app/src/gen/session/v1/session_connect.ts`

---

### Epic 1.3: `DraftPullRequest` handler

**Goal**: Implement the read-only preview RPC — resolves the worktree, gates
on commits-ahead, short-circuits on an existing PR, drafts the body via
`headless.DraftPRDescription` with a deterministic fallback, and resolves the
default base branch.

#### Story 1.3.1: Implement `SessionService.DraftPullRequest`

**As a** user with a finished session, **I want** clicking "Create PR" to
open a modal already filled with a sensible title/body/base branch, **so
that** I only have to review and adjust, not write from scratch.

**Acceptance Criteria**:
- AC1 (happy path pre-fill): *Given* a live session `sess-7f3a` titled "Add
  rate limit toggle" with an active worktree on branch
  `feature/rate-limit-toggle` that is 3 commits ahead of `main`, *When* the
  client calls `DraftPullRequest({sessionId: "sess-7f3a"})`, *Then* the
  response has `title: "Add rate limit toggle"`, `body: "<DraftPRDescription
  output>"`, `baseBranch: "main"` (resolved via
  `GitVCSReader.ResolveDefaultBranch`), `hasCommitsAhead: true`,
  `existingPrUrl: ""`.
- AC4 (existing PR short-circuit): *Given* session `sess-7f3a` already has
  `GitHubPrUrl = "https://github.com/tstapler/stapler-squad/pull/512"` and
  `GitHubPrNumber = 512` set on it, *When* `DraftPullRequest` is called for
  it, *Then* the response has `existingPrUrl:
  "https://github.com/tstapler/stapler-squad/pull/512"`,
  `existingPrNumber: 512`, and the frontend uses this to skip straight to a
  "View PR #512" state instead of rendering the editable form.
- AC6 (no-commits-ahead gate feeds the trigger button): *Given* session
  `sess-9c21`'s worktree has zero commits ahead of `main`
  (`HasCommitsAheadOfMain` returns `false, nil`), *When* `DraftPullRequest`
  is called for it, *Then* the response has `hasCommitsAhead: false`, and the
  frontend disables the trigger button with
  `title="No commits ahead of main yet"` rather than opening the modal.

**Files**: `server/services/session_service.go`

##### Task 1.3.1a: Add request validation + instance/worktree resolution (~4 min)
- Add a new method `DraftPullRequest(ctx context.Context, req
  *connect.Request[sessionv1.DraftPullRequestRequest])
  (*connect.Response[sessionv1.DraftPullRequestResponse], error)` on
  `*SessionService`, placed after `RunOneShotForSession`
  (`server/services/session_service.go:3739`).
- Validate `req.Msg.SessionId != ""` → `connect.CodeInvalidArgument` (mirror
  `RunOneShot`'s pattern, `session_service.go:3620-3625`).
- `inst := s.findInstance(req.Msg.SessionId)`; if `nil`, return
  `connect.CodeNotFound` (per the Pattern Decisions row scoping this feature
  to live instances only).
- If `!inst.HasGitWorktree()`, return `connect.CodeFailedPrecondition`
  ("session has no worktree").
- `wt, err := inst.GetGitWorktree()`; if `err != nil` (e.g. "instance has not
  been started"), return `connect.CodeFailedPrecondition` with `err`'s
  message.
- Files: `server/services/session_service.go`

##### Task 1.3.1b: Existing-PR short-circuit + commits-ahead check (~4 min)
- If `inst.GitHubPrUrl != ""`, populate `existing_pr_url`/`existing_pr_number`
  on the response from `inst.GitHubPrUrl`/`inst.GitHubPrNumber` and return
  early (skip the diff/draft work below entirely — AC4 doesn't need it).
- Resolve `baseBranch := (&unfinished.GitVCSReader{}).ResolveDefaultBranch(wt.RepoPath())`
  (add a `RepoPath()` accessor on `GitWorktree` if one doesn't already exist
  — check `session/git/worktree_git.go` for an existing exported/unexported
  `repoPath` field accessor first before adding a new one).
- `hasCommits, _ := wt.HasCommitsAheadOfMain(baseBranch)` (per
  `HasCommitsAheadOfMain`'s own fail-open contract, ignore the error and
  trust the returned bool, matching `pushAndCreatePR`'s existing usage
  pattern at `backlog_lifecycle.go:3638`).
- Files: `server/services/session_service.go`, `session/git/worktree_git.go`
  (only if a `RepoPath()` accessor needs adding)

##### Task 1.3.1c: Draft the body via `DraftPRDescription` with fallback (~5 min)
**(Body superseded by Post-Review Revisions #1 and #2 — text below is the
current, authoritative version; do not use `GetGitDiff` or an unguarded
`headlessPool` call.)**
- Compute the diff via the same working-tree-inclusive path
  `SessionService.GetSessionDiff`/`GitWorktree.Diff()` already uses
  (`session/git/diff.go:43-90` — runs `git add -N .` first to pick up
  untracked files, then diffs the working tree, not just `baseSHA..HEAD`).
  **Do not call `session.GetGitDiff`** (committed-only) — that was the
  original draft of this task and is exactly what Post-Review Revision #2
  replaced, to keep the preview aligned with what the user already sees in
  the session card's diff viewer, without `DraftPullRequest` committing
  anything itself.
- If the diff is empty (`strings.TrimSpace(diff) == ""`) or computing it
  errors, skip the LLM call and use a deterministic fallback body (a short
  "## Summary\n\n_No diff description available._" placeholder — this
  session has no backlog item, so `buildFallbackPRBody`'s
  item-description-driven template does not apply verbatim; write a minimal
  session-shaped equivalent).
- Otherwise, **guard on `s.headlessPool` being non-nil first** (Post-Review
  Revision #1 — mirrors `RunOneShot`'s existing guard at
  `session_service.go:3653`):
  ```go
  var body string
  var draftErr error
  if s.headlessPool != nil {
      body, draftErr = headless.DraftPRDescription(ctx, s.headlessPool, inst.Title, sessionGoalText(inst), diff, wt.BranchName())
  } else {
      body = fallbackBody
  }
  ```
  where `sessionGoalText(inst)` returns `inst.GetSessionGoal().Goal` if a
  goal is set, else `""` — this is the plan's explicit answer to
  features.md §5.3's "what to pass as itemDescription" question. On
  `draftErr != nil`, log a warning and fall back to `fallbackBody` (do not
  fail the whole RPC — a draft failure should never block the modal from
  opening, matching build-vs-buy.md's "fallback template must be
  first-class" conclusion). **This method makes zero git-mutating calls —
  no `CommitChanges`, no `PushBranch`** (Post-Review Revision #2's read-only
  fix).
- Files: `server/services/pr_creation_service.go`

##### Task 1.3.1d: Assemble and return the response (~2 min)
- Populate `title: inst.Title`, `body`, `base_branch: baseBranch`,
  `has_commits_ahead: hasCommits`, leaving `existing_pr_url`/`existing_pr_number`
  empty (already handled by the early return in 1.3.1b for the existing-PR
  case).
- Add the `// +api: session:draft-pull-request` marker comment above the
  method per `.claude/rules/feature-registry.md`'s convention (matches
  `RunOneShot`'s `// +api: session:run-one-shot` at line 3613).
- Files: `server/services/session_service.go`

---

### Epic 1.4: `CreatePullRequest` handler

**Goal**: Implement the mutating RPC — in-flight guard, commit-dirty-state +
push (unconditionally, before the existing-PR check, per pitfalls.md §5's
ordering requirement), call the now-3-arg `CreatePR`, persist with explicit
partial-failure signaling, and call `RecordPRCreatedOutOfBand`.

#### Story 1.4.1: Implement `SessionService.CreatePullRequest`

**As a** user reviewing the pre-filled modal, **I want** confirming to
create the PR mechanically (no agent turn) and update my session card, **so
that** I get a fast, deterministic, reviewable PR-creation flow.

**Acceptance Criteria**:
- AC2/AC3 (edited fields flow through to the mechanical call, no agent turn):
  *Given* the user edited the modal to `title: "Add per-user rate limiting"`,
  `body: "...edited body..."`, `baseBranch: "release/1.2"`, *When* the client
  calls `CreatePullRequest({sessionId: "sess-7f3a", title: "Add per-user rate
  limiting", body: "...edited body...", baseBranch: "release/1.2"})`, *Then*
  the handler calls `wt.CreatePR("Add per-user rate limiting", "...edited
  body...", "release/1.2")` directly — asserted in the Go test (Epic 1.5) by
  a fake `prCreator`-shaped test double that records its call args and by
  confirming `s.headlessPool.CallBlocking` is never invoked during this RPC.
- AC5 (persistence + event on success): *Given* `CreatePR` returns
  `("https://github.com/tstapler/stapler-squad/pull/512", 512, nil)`, *When*
  the handler proceeds past that call, *Then* it calls
  `inst.SetGitHubPR("https://github.com/tstapler/stapler-squad/pull/512",
  512)`, `s.storage.SaveInstances(...)` succeeds, and
  `s.eventBus.Publish(events.NewSessionUpdatedEvent(inst,
  []string{"github_pr_url", "github_pr_number"}))` fires — the response has
  `prUrl`, `prNumber: 512`, `persisted: true`, `persistError: ""`.
- AC6 (specific error surfacing, gh not authenticated): *Given*
  `checkGHCLI()` fails inside `CreatePR` with "GitHub CLI is not configured.
  Please run 'gh auth login' first", *When* `CreatePullRequest` is called,
  *Then* the RPC returns that exact string as the `connect.CodeUnavailable`
  error message (not a generic "failed to create PR").
- Persist-failure partial success (BUG-040 analog, pitfalls.md §1): *Given*
  `CreatePR` succeeds with `("https://github.com/.../pull/512", 512, nil)`
  but `s.storage.SaveInstances(...)` then returns an error, *When* the
  handler returns, *Then* the response is still a *success* (`connect.Response`,
  not a `connect.Error`) with `prUrl`/`prNumber` populated,
  `persisted: false`, `persistError: "<the SaveInstances error>"`.

**Files**: `server/services/session_service.go`

##### Task 1.4.1a: Add the in-flight guard field + request validation (~3 min)
- Add a new field `prCreationInFlight sync.Map` to the `SessionService`
  struct (near the other small maps/state, `server/services/session_service.go`
  struct definition).
- At the top of the new `CreatePullRequest` method, validate `SessionId`,
  `Title` (non-empty — mirrors AC2's "editable" fields needing at least a
  title), resolve `inst`/`wt` exactly as in Task 1.3.1a (extract a small
  shared helper `resolveSessionWorktree(sessionID string) (*session.Instance,
  *git.GitWorktree, error)` used by both `DraftPullRequest` and
  `CreatePullRequest` to avoid duplicating the not-found/no-worktree/not-started
  checks).
- `if _, loaded := s.prCreationInFlight.LoadOrStore(req.Msg.SessionId, true);
  loaded { return nil, connect.NewError(connect.CodeAlreadyExists,
  fmt.Errorf("PR creation already in progress for this session")) }`,
  followed immediately by `defer s.prCreationInFlight.Delete(req.Msg.SessionId)`.
- Files: `server/services/session_service.go`

##### Task 1.4.1b: Commit dirty state + push (unconditionally, before existing-PR check) (~5 min)
**(Amended by Post-Review Revision #2 — the `CommitChanges` error handling
below differs from the original draft; text here is authoritative.)**
- Mirror `pushAndCreatePR`'s ordering (`backlog_lifecycle.go:3607-3617`): call
  `wt.CommitChanges(fmt.Sprintf("[stapler-squad] work complete for %q
  (pre-PR)", inst.Title))`. **Unlike `pushAndCreatePR`, a `CommitChanges`
  error here is not merely logged — return `connect.CodeInternal` with the
  commit error's literal message (AC6) and stop.** A manual
  review-then-publish flow cannot silently drop in-scope changes the user
  just reviewed in the diff viewer, the way the unattended backlog
  automation path is allowed to (that path has no human watching the
  session at the moment of the commit; this one does). Then call
  `wt.PushBranch()` — on error, return `connect.CodeUnavailable` with the
  push error's exact message (AC6).
- This commit+push must happen *before* any existing-PR short-circuit, per
  pitfalls.md §5's "re-push / no-op on retry" finding — otherwise a second
  click after new commits silently never reaches GitHub.
- Files: `server/services/pr_creation_service.go`

##### Task 1.4.1c: Call `CreatePR` and determine `already_existed` (~4 min)
- Fast path: if `inst.GitHubPrUrl != "" && inst.GitHubPrNumber > 0`, set
  `already_existed = true` and skip straight to using those cached values —
  matching `pushAndCreatePR`'s own fast-path pattern
  (`backlog_lifecycle.go:3622-3626`) instead of always calling `CreatePR`.
- Otherwise call `prURL, prNumber, err := wt.CreatePR(req.Msg.Title,
  req.Msg.Body, req.Msg.BaseBranch)`. On error, return
  `connect.CodeUnavailable` (or `CodeInternal` for a non-`gh`-specific
  failure) with `err`'s literal message (AC6). On success, `already_existed
  = (prURL == inst.GitHubPrUrl && inst.GitHubPrUrl != "")` is *not* how to
  detect reuse (the session's cached field is only set after a prior
  successful create/persist, which may not have happened) — instead treat
  `already_existed` as `true` whenever this code path was reached via the
  fast-path check above, and `false` when it fell through to a real
  `CreatePR` call (that call itself already transparently reuses an existing
  PR at the `gh`/`findExistingPR` level, but from this handler's perspective
  it made a "create" attempt either way — document this in a code comment so
  a future reader doesn't expect perfect create-vs-reuse fidelity from a
  single `gh pr create` race).
- If `prNumber == 0` after a non-error `CreatePR` return, treat it as an
  error (BUG-063(a) analog, pitfalls.md row 2) — return `connect.CodeInternal`
  ("PR created but its number could not be determined").
- Files: `server/services/session_service.go`

##### Task 1.4.1d: Persist + publish event, with explicit partial-failure signaling (~5 min)
- `inst.SetGitHubPR(prURL, prNumber)`.
- `persisted := true; persistError := ""`.
- `if err := s.storage.SaveInstances(s.allInstances()); err != nil { persisted
  = false; persistError = err.Error(); log.WarningLog.Printf(...) } else {
  s.eventBus.Publish(events.NewSessionUpdatedEvent(inst,
  []string{"github_pr_url", "github_pr_number"})) }` — the event is only
  published when the save actually succeeded (publishing an event about a
  state that failed to persist would tell subscribers something false).
- Call `s.backlogLifecycleListener.RecordPRCreatedOutOfBand(ctx, inst.UUID,
  prURL, prNumber)` unconditionally (nil-checked), mirroring `RunOneShot`'s
  existing call at `session_service.go:3706-3708` — same reasoning: this
  session might be backlog-linked even though it was created via the manual
  modal.
- Return `connect.NewResponse(&sessionv1.CreatePullRequestResponse{PrUrl:
  prURL, PrNumber: int32(prNumber), AlreadyExisted: alreadyExisted,
  Persisted: persisted, PersistError: persistError})` — always a success
  response at this point (the PR is real; only the record of it may have
  failed to save, per the Pattern Decisions row above).
- Add `// +api: session:create-pull-request` marker.
- Files: `server/services/session_service.go`

---

### Epic 1.5: Go tests for the new RPC

**Goal**: Mirror `pushAndCreatePR`'s edge-case test catalog
(`session/backlog_lifecycle_test.go:2346-2729`) at the RPC layer, per AC8.

#### Story 1.5.1: `TestCreatePullRequest_*` and `TestDraftPullRequest_*`

**Acceptance Criteria**:
- AC8 (Go test coverage): *Given* the new RPC handlers, *When* `go test
  ./server/services -run 'TestCreatePullRequest|TestDraftPullRequest'` runs,
  *Then* it passes and exercises: reuse-existing-PR, `gh` auth failure,
  zero-commits-ahead gating (via `DraftPullRequest`'s `hasCommitsAhead`
  field), persist failure leaving `persisted=false`, and the concurrent
  double-call rejection.

**Files**: `server/services/session_service_test.go` (new test functions;
create `server/services/create_pull_request_test.go` if the existing file is
already large enough that appending would hurt readability — check its
current line count first).

##### Task 1.5.1a: Test scaffolding — real temp git repo + fake `gh` (~5 min)
- Follow `server/services/session_retention_sweeper_test.go:136`'s pattern
  (construct real worktrees against temp git repos) rather than mocking
  `GitWorktree`, per architecture.md §5's "no new interface" decision.
- Need a way to stub `gh` calls without shelling out — check whether
  `GitWorktree` already exposes an executor-injection seam (`cmdExec` field,
  confirmed present at `worktree_git.go:301-314`) and use that, matching
  `worktree_git_test.go`'s existing fake-executor pattern (used in Task
  1.1.1d).
- Files: `server/services/session_service_test.go` (or new file per above)

##### Task 1.5.1b: `TestDraftPullRequest_should_ReturnExistingPR_When_SessionAlreadyHasOne` (~3 min)
- Given an instance with `GitHubPrUrl`/`GitHubPrNumber` pre-set, assert the
  response's `existing_pr_url`/`existing_pr_number` match and no diff/draft
  work occurred (assert `headlessPool.CallBlocking` was not invoked).
- Files: same test file

##### Task 1.5.1c: `TestDraftPullRequest_should_ReportHasCommitsAhead_False_When_BranchIsUpToDate` (~3 min)
- Given a worktree with zero commits ahead of the resolved base branch,
  assert `has_commits_ahead: false`.
- Files: same test file

##### Task 1.5.1d: `TestCreatePullRequest_should_CallCreatePRDirectly_NotHeadlessPool` (~4 min)
- Assert that during `CreatePullRequest`, `s.headlessPool.CallBlocking` is
  never invoked (this is the direct proof of AC3 — the mechanical path, not
  the agentic one).
- Files: same test file

##### Task 1.5.1e: `TestCreatePullRequest_should_SurfaceSpecificError_When_GHNotAuthenticated` (~3 min)
- Fake `gh auth status` failing via the injected executor; assert the
  returned connect error's message is the literal `checkGHCLI` string, not a
  generic wrapper.
- Files: same test file

##### Task 1.5.1f: `TestCreatePullRequest_should_ReturnPersistedFalse_When_SaveInstancesFails` (~4 min)
- Inject a storage fake whose `SaveInstances` returns an error; assert the
  RPC still returns a success response with `persisted: false` and a
  non-empty `persist_error`, and that no `connect.Error` was returned.
- Files: same test file

##### Task 1.5.1g: `TestCreatePullRequest_should_RejectConcurrentCall_When_AlreadyInFlight` (~4 min)
- Start one `CreatePullRequest` call that blocks (via a slow fake `gh`
  command), fire a second concurrent call for the same session ID, assert
  the second returns `connect.CodeAlreadyExists` with the in-flight message.
- Files: same test file

##### Task 1.5.1h: `TestDraftPullRequest_should_PreviewWorkingTreeDiff_When_UncommittedChangesPresent` (~4 min)
- Added per Post-Review Revision #2 / pre-mortem P1. Given a worktree with
  uncommitted changes (staged and/or untracked), assert the drafted body
  reflects those changes and that `wt.CommitChanges` is never called during
  `DraftPullRequest` (proves the RPC stays read-only).
- Files: same test file

##### Task 1.5.1i: `TestCreatePullRequest_should_SurfaceError_When_CommitFails` (~3 min)
- Added per Post-Review Revision #2. Inject a fake executor whose commit
  step fails; assert `CreatePullRequest` returns `connect.CodeInternal` with
  the commit error's literal message, and that `PushBranch`/`CreatePR` are
  never reached (the commit failure must stop the flow, not be swallowed).
- Files: same test file

##### Task 1.5.1j: `TestDraftPullRequest_should_UseFallbackBody_When_HeadlessPoolNil` (~3 min)
- Added per Post-Review Revision #1. Construct a `PRCreationService` with a
  nil `headlessPool`; assert `DraftPullRequest` returns the deterministic
  fallback body instead of panicking.
- Files: same test file

##### Task 1.5.1k: Run and verify the full package (~2 min)
- `make build && go test ./server/services -run 'CreatePullRequest|DraftPullRequest' -v`.
- Files: none (verification only)

---

### Epic 1.6: Backend feature registry entries (AC9)

#### Story 1.6.1: Register the two new backend RPCs

**Acceptance Criteria**:
- AC9 (registry updated): *Given* the new RPCs, *When*
  `docs/registry/features/backend/session/draft-pull-request.json` and
  `.../create-pull-request.json` are created and `make registry-generate`
  runs, *Then* `docs/registry/coverage-gaps.json`'s total count does not
  increase versus its pre-change value (checked via `git diff --stat
  docs/registry/coverage-gaps.json` before/after).

**Files**: `docs/registry/features/backend/session/draft-pull-request.json`,
`docs/registry/features/backend/session/create-pull-request.json`

##### Task 1.6.1a: Create `draft-pull-request.json` (~2 min)
- Model on `docs/registry/features/backend/session/run-one-shot.json`:
  ```json
  {
    "id": "session:draft-pull-request",
    "type": "backend",
    "service": "SessionService",
    "method": "DraftPullRequest",
    "protoFile": "proto/session/v1/session.proto",
    "markerFound": true,
    "handlerFile": "server/services/session_service.go",
    "tested": true,
    "testIds": ["TestDraftPullRequest_should_ReturnExistingPR_When_SessionAlreadyHasOne", "TestDraftPullRequest_should_ReportHasCommitsAhead_False_When_BranchIsUpToDate"],
    "lastModified": "2026-08-06T00:00:00-07:00"
  }
  ```
- Files: `docs/registry/features/backend/session/draft-pull-request.json`

##### Task 1.6.1b: Create `create-pull-request.json` (~2 min)
- Same shape, `"method": "CreatePullRequest"`, `testIds` listing the Epic 1.5
  test function names.
- Files: `docs/registry/features/backend/session/create-pull-request.json`

---

## Phase 2: Frontend Modal

### Epic 2.1: `CreatePullRequestModal` component

**Goal**: New extracted modal component (Pattern Decisions row) implementing
the exact a11y contract from ux.md — every field has `<label htmlFor>` +
`data-testid`, error text has `role="alert"`, `vars.zIndex.modal` not a
hardcoded value.

#### Story 2.1.1: Build the modal skeleton, styles, and field state

**As a** user, **I want** to review and edit the AI-drafted PR title/body/base
branch before it's created, **so that** I have confidence the PR is right
before it becomes public.

**Acceptance Criteria**:
- AC2 (fields editable): *Given* the modal is open with `title: "Add rate
  limit toggle"`, `body: "<drafted body>"`, `baseBranch: "main"`, *When* the
  user clears the base-branch input and types `release/1.2`, *Then* the
  input's value becomes `release/1.2` and the "Create PR" submit button
  remains enabled (title is still non-empty).

**Files**: `web-app/src/components/sessions/CreatePullRequestModal.tsx`
(new), `web-app/src/components/sessions/CreatePullRequestModal.css.ts` (new)

##### Task 2.1.1a: Create the `.css.ts` file re-exporting shared dialog tokens (~3 min)
- Follow `SessionActionsOverflow.css.ts`'s re-export pattern: `export {
  confirmDialog, dialogContent, dialogActions, submitButton, cancelButton,
  errorMessage } from "./SessionCard.css"`.
- Add one new style specific to this modal: a taller, resizable body
  textarea (ux.md §3: "LLM-generated PR bodies run several paragraphs; a
  single-line-height textarea would truncate visually") — a `bodyTextarea`
  vanilla-extract `style()` with `minHeight`, `resize: "vertical"`.
- Files: `web-app/src/components/sessions/CreatePullRequestModal.css.ts`

##### Task 2.1.1b: Component props + local field state (~4 min)
- Props: `{ session: Session; isOpen: boolean; onClose: () => void;
  draftPullRequest: (sessionId: string) => Promise<DraftPullRequestResponse
  | null>; createPullRequest: (req: {...}) => Promise<CreatePullRequestResponse
  | null>; triggerRef: RefObject<HTMLButtonElement> }`.
- `useState` for `title`, `body`, `baseBranch`, `isDrafting` (loading the
  preview), `isSubmitting`, `error`, `existingPr: {url: string; number:
  number} | null`.
- `useRef<HTMLDivElement>` + `useFocusTrap(dialogRef, isOpen, triggerRef)`.
- Files: `web-app/src/components/sessions/CreatePullRequestModal.tsx`

##### Task 2.1.1c: Fetch the draft on open (~4 min)
- `useEffect` keyed on `isOpen`: when it flips to `true`, set `isDrafting =
  true`, call `draftPullRequest(session.id)`, and on response either (a) set
  `existingPr` if `existingPrUrl` is non-empty (AC4 path — skip the form), or
  (b) populate `title`/`body`/`baseBranch` from the response.
- On a `null`/error response, set `error` to a generic "Couldn't load PR
  draft — try again" (the hook layer already dispatches the specific error to
  Redux per `useSessionService.ts`'s existing pattern; the modal shows its
  own inline copy since AC6 wants errors *in the dialog*, not just a global
  toast).
- Files: `web-app/src/components/sessions/CreatePullRequestModal.tsx`

##### Task 2.1.1d: Render the a11y-contract JSX (~5 min)
- Implement exactly the skeleton from ux.md §3: `role="dialog"
  aria-modal="true" aria-labelledby="createPrDialogTitle"`, `<label
  htmlFor="createPrTitle">` + `data-testid="create-pr-title-input"` for each
  of the three fields, `{error && <p role="alert"
  data-testid="create-pr-error">{error}</p>}`, submit button
  `data-testid="create-pr-submit"` disabled while `isSubmitting ||
  isDrafting || !title.trim()`.
- Use `zIndex: vars.zIndex.modal` (via the `.css.ts` file, not an inline
  style) — per `.claude/rules/css-architecture.md`.
- Autofocus the title input on open (not the submit button) per ux.md §3.
- Disable all three inputs (not just submit/cancel) while `isSubmitting`, per
  ux.md §3's "stale-value resubmit race" note.
- Files: `web-app/src/components/sessions/CreatePullRequestModal.tsx`

##### Task 2.1.1e: "View PR" branch + submit handler (~5 min)
- When `existingPr` is set (AC4), render a "View PR #<number>" link
  (`<a href={existingPr.url} target="_blank" rel="noopener noreferrer"
  aria-label={\`PR #${existingPr.number}\`} data-testid="github-pr-link">`)
  instead of the form, matching `GitHubPRsSection.tsx:76-83`'s convention.
- Submit handler: `setIsSubmitting(true)`, call `createPullRequest({...})`,
  on a response with `prUrl` show a success state ("Created PR #<n>" /
  "Updated PR #<n>" depending on `alreadyExisted`) with a link, and if
  `persisted === false` additionally show a `role="alert"` warning ("PR
  created but couldn't be saved to the session — refresh to check") without
  treating it as a failure. On error (`null` response / thrown), set `error`
  to the caught message, **keep all field values intact** (ux.md §4's "all
  error states keep the modal open with field values intact").
- Files: `web-app/src/components/sessions/CreatePullRequestModal.tsx`

---

### Epic 2.2: `useSessionService` hook additions

#### Story 2.2.1: `draftPullRequest`/`createPullRequest` hook functions

**Files**: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.2.1a: Add `draftPullRequest` (~3 min)
- Follow `runOneShot`'s exact shape (`useSessionService.ts:563-582`):
  `useCallback` wrapping `clientRef.current.draftPullRequest({ sessionId })`,
  `dispatch(setError(...))` on catch, return typed response or `null`.
- Files: `web-app/src/lib/hooks/useSessionService.ts`

##### Task 2.2.1b: Add `createPullRequest` (~3 min)
- Same shape: `clientRef.current.createPullRequest({ sessionId, title, body,
  baseBranch })`.
- Add both to the hook's returned object and to
  `useSessionServiceContext`'s exposed API (check how `runOneShot` is
  exposed via context and mirror it exactly).
- Files: `web-app/src/lib/hooks/useSessionService.ts`

---

### Epic 2.3: Wire the modal into `SessionActionsOverflow.tsx` (AC7, primary entry point)

#### Story 2.3.1: Replace the `onRunOneShot` overflow item with the modal trigger

**Acceptance Criteria**:
- AC7 (single entry point): *Given* `SessionActionsOverflow.tsx`'s overflow
  menu, *When* this story ships, *Then* the menu's PR-related item renders
  exactly the three states design/ux.md's Surface 1&2 defines — enabled
  "🔀 Create PR" (State A), disabled "🔀 Create PR" with a `title` tooltip
  (State B, no commits ahead), or a "✅ View PR #N" link derived from
  `session.githubPrUrl` (State C) — opens `CreatePullRequestModal` only from
  State A, and there is no code path in this file that calls
  `onRunOneShot`/`RunOneShot` anymore. *(Revised 2026-08-06: dropped the
  earlier "PR pending…" label, which `sdd:4-validate`'s cross-artifact check
  flagged as a 4th state with no basis in ux.md's 3-state design — the modal
  itself already renders its own "submitting" state, so the trigger button
  needs no separate pending label.)*

**Files**: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 2.3.1a: Add modal open state + trigger ref, remove `onRunOneShot` usage (~4 min)
- Add `isCreatePrOpen` state + `createPrTriggerRef`.
- Replace the `onRunOneShot` prop and its button block (lines 609-618) with
  a button that sets `isCreatePrOpen = true` (gated on `session.githubPrUrl
  === ""` — when already set, render a link instead, per ux.md §1's
  "clickable link to the PR" recommendation, reusing `GitHubPRsSection`'s
  `aria-label={\`PR #${n}: ${title}\`}` convention).
- Remove `isRunningOneShot`/`oneShotResult` local state entirely (pitfalls.md
  §5: prefer deriving from the persisted `session.githubPrUrl` field so the
  indicator survives remount/navigation).
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 2.3.1b: Render `CreatePullRequestModal` and remove the `onRunOneShot` prop from the interface (~3 min)
- Add `<CreatePullRequestModal session={session} isOpen={isCreatePrOpen}
  onClose={() => setIsCreatePrOpen(false)} draftPullRequest={...}
  createPullRequest={...} triggerRef={createPrTriggerRef} />` near the other
  dialog renders.
- Remove `onRunOneShot?: (sessionId: string) => Promise<void>;` from
  `SessionActionsOverflowProps` (line 55) — this component now calls the
  hook functions directly rather than taking a callback prop, since
  `draftPullRequest`/`createPullRequest` need no per-call-site adaptation
  (unlike `RunOneShot`, which took a free-text prompt each caller had to
  supply).
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

---

### Epic 2.4: Wire the modal into `ReviewQueuePanel.tsx` (AC7, second entry point)

#### Story 2.4.1: Replace the inline `prModal` prompt-editor with `CreatePullRequestModal`

**Acceptance Criteria**:
- AC7 (no second, differently-behaved entry point): *Given*
  `ReviewQueuePanel.tsx`'s "🔀 Create PR" button (line 901-914, currently
  opening `prModal` with a free-text `DEFAULT_PR_PROMPT`), *When* this story
  ships, *Then* clicking it opens the same `CreatePullRequestModal` component
  Epic 2.3 wires into `SessionActionsOverflow.tsx` — not a separate
  prompt-editing dialog — so both entry points behave identically.

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 2.4.1a: Remove the inline `prModal`/`prResult`/`prRunning` state and JSX (~5 min)
- Delete the `prModal`/`setPrModal`, `prResult`/`setPrResult`,
  `prRunning`/`setPrRunning`, `DEFAULT_PR_PROMPT` state and the portal-rendered
  dialog block (lines ~1300-1369 per the research excerpt).
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 2.4.1b: Replace the "🔀 Create PR" button + render `CreatePullRequestModal` (~4 min)
- Replace the button's `onClick` (currently `setPrModal({ sessionId, prompt:
  DEFAULT_PR_PROMPT })`) with opening the shared modal for
  `queueItem.sessionId`.
- Remove the `onRunOneShot?: (sessionId: string, prompt: string) =>
  Promise<...>` prop from `ReviewQueuePanelProps` (line 92) — no longer
  needed, same reasoning as Task 2.3.1b.
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

---

### Epic 2.5: Remove dead `onRunOneShot` plumbing across the call chain (AC7)

**Goal**: With both real call sites migrated (Epics 2.3, 2.4), remove the
now-dead `onRunOneShot` prop threading from every intermediate component and
page-level handler, without touching the backend `RunOneShot` RPC or
`RunOneShotForSession` (still used by `AutoCreatePR`).

#### Story 2.5.1: Strip `onRunOneShot` from `SessionCard`/`SessionRow`/`SessionList`/`PaneHeader`/`PaneSplitRenderer`/pages

**Acceptance Criteria**:
- AC7 (verified via grep): *Given* this story's changes, *When* `grep -rn
  "onRunOneShot" web-app/src/` runs, *Then* it returns no matches (the prop
  is fully removed from the tree; the underlying `runOneShot` hook function
  and RPC client method may remain in `useSessionService.ts` since
  `RunOneShotForSession` is still a valid backend-only caller, but nothing in
  `web-app/src` references it anymore).

**Files**: `web-app/src/components/sessions/SessionCard.tsx`,
`web-app/src/components/sessions/SessionRow.tsx`,
`web-app/src/components/sessions/SessionList.tsx`,
`web-app/src/components/pane/PaneHeader.tsx`,
`web-app/src/components/pane/PaneSplitRenderer.tsx`,
`web-app/src/app/page.tsx`, `web-app/src/app/review-queue/page.tsx`

##### Task 2.5.1a: Remove `onRunOneShot` prop from `SessionCard.tsx` (~2 min)
- Remove the prop declaration (line 113), destructure (line 144), and
  pass-through to `SessionActionsOverflow` (line 876).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.5.1b: Remove `onRunOneShot` prop from `SessionRow.tsx` (~2 min)
- Remove prop declaration (line 49), destructure (line 147), pass-through
  (line 399).
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 2.5.1c: Remove `onRunOneShot` prop from `SessionList.tsx` (~3 min)
- Remove prop declarations (lines 75, 115), destructure (lines 153, 288),
  and both pass-throughs (lines 176, 1395, 1578).
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.5.1d: Remove `onRunOneShot` wiring from `PaneHeader.tsx`/`PaneSplitRenderer.tsx` (~3 min)
- Remove the `onRunOneShot={(id) => cockpit.onRunOneShot(id)}` pass-through
  (`PaneHeader.tsx:72`) and `onRunOneShot={actions.onRunOneShot}`
  (`PaneSplitRenderer.tsx:184`) — check whether `cockpit.onRunOneShot`/
  `actions.onRunOneShot` are defined in a shared hook (e.g. a "pane actions"
  hook) that also needs its `onRunOneShot` field removed; trace one level up
  from each call site before deleting, since these may share a hook with
  `page.tsx`'s `handleRunOneShot` (Task 2.5.1e).
- Files: `web-app/src/components/pane/PaneHeader.tsx`,
  `web-app/src/components/pane/PaneSplitRenderer.tsx`

##### Task 2.5.1e: Remove `handleRunOneShot` from `page.tsx` and its wiring (~3 min)
- Remove `handleRunOneShot` (lines 294-296) and its pass-through at line 430
  and inclusion in the dependency/export list at line 440.
- Files: `web-app/src/app/page.tsx`

##### Task 2.5.1f: Remove `handleRunOneShot` from `review-queue/page.tsx` (~3 min)
- Remove the adapter `handleRunOneShot` (lines 71-78) and its pass-through to
  `ReviewQueuePanel` (line 343) — now unnecessary since Epic 2.4 removed
  `ReviewQueuePanel`'s `onRunOneShot` prop entirely.
- Files: `web-app/src/app/review-queue/page.tsx`

##### Task 2.5.1g: Build check (~2 min)
- `cd web-app && npx tsc --noEmit` (or the repo's equivalent type-check
  script) to confirm no dangling references remain after the prop removals.
- Files: none (verification only)

---

### Epic 2.6: Frontend feature registry entry (AC9)

#### Story 2.6.1: Register `CreatePullRequestModal`

**Acceptance Criteria**:
- AC9: *Given* the new component, *When*
  `docs/registry/features/frontend/create-pull-request-modal.json` is created
  and `make registry-generate` runs, *Then* `coverage-gaps.json`'s count does
  not increase (once Epic 3.1's tests are in and `testIds` is populated).

**Files**: `docs/registry/features/frontend/create-pull-request-modal.json`

##### Task 2.6.1a: Create the frontend registry entry (~2 min)
```json
{
  "id": "create-pull-request-modal",
  "type": "frontend",
  "name": "Create Pull Request modal",
  "filePath": "web-app/src/components/sessions/CreatePullRequestModal.tsx",
  "tested": true,
  "testIds": ["CreatePullRequestModal_should_PrefillFields_When_Opened", "CreatePullRequestModal_should_ShowViewPrLink_When_PrAlreadyExists"]
}
```
- Files: `docs/registry/features/frontend/create-pull-request-modal.json`

---

## Phase 3: Tests

### Epic 3.1: Frontend Jest tests

#### Story 3.1.1: `CreatePullRequestModal.test.tsx`

**Files**: `web-app/src/components/sessions/CreatePullRequestModal.test.tsx`
(new)

##### Task 3.1.1a: Test prefill-on-open and editable fields (~4 min)
- `CreatePullRequestModal_should_PrefillFields_When_Opened`: mock
  `draftPullRequest` resolving `{title: "Add rate limit toggle", body: "...",
  baseBranch: "main", hasCommitsAhead: true, existingPrUrl: "",
  existingPrNumber: 0}`; assert the title input's value equals it after the
  effect resolves (AC1).
- `CreatePullRequestModal_should_AllowEditingBaseBranch_When_UserTypes`:
  fire a change event on the base-branch input, assert its value updates
  (AC2).
- Files: `web-app/src/components/sessions/CreatePullRequestModal.test.tsx`

##### Task 3.1.1b: Test the existing-PR branch (AC4) (~3 min)
- `CreatePullRequestModal_should_ShowViewPrLink_When_PrAlreadyExists`: mock
  `draftPullRequest` resolving `existingPrUrl:
  "https://github.com/tstapler/stapler-squad/pull/512", existingPrNumber:
  512`; assert a `data-testid="github-pr-link"` anchor renders with that
  href and the editable form does not render.
- Files: same test file

##### Task 3.1.1c: Test submit success + persist-failure warning (~5 min)
- `CreatePullRequestModal_should_CallCreatePullRequest_When_Submitted`:
  assert `createPullRequest` is called with the (possibly edited)
  title/body/baseBranch.
- `CreatePullRequestModal_should_ShowPersistWarning_When_PersistedFalse`:
  mock `createPullRequest` resolving `{prUrl: "...", prNumber: 512,
  alreadyExisted: false, persisted: false, persistError: "disk full"}`;
  assert a `role="alert"` warning containing "disk full" renders alongside
  the success PR link (not in place of it).
- Files: same test file

##### Task 3.1.1d: Test error state keeps field values intact (AC6) (~3 min)
- `CreatePullRequestModal_should_PreserveFieldValues_When_SubmitFails`: mock
  `createPullRequest` rejecting/returning `null`; assert the title/body/base
  inputs still show the user's edited values after the error renders.
- Files: same test file

##### Task 3.1.1e: Run and verify (~2 min)
- `cd web-app && npx jest --testPathPatterns="CreatePullRequestModal.test" --no-coverage`.
- Files: none (verification only)

---

### Epic 3.2: Playwright e2e spec (AC8)

#### Story 3.2.1: `create-pull-request.spec.ts`

**Acceptance Criteria**:
- AC8 (e2e coverage, repo conventions): *Given* a test session with commits
  ahead of its base branch, *When* `npx playwright test
  create-pull-request.spec.ts` runs, *Then* the spec opens the modal via
  `data-testid`, edits the title, submits, and asserts the PR link appears —
  using only `data-testid`/ARIA locators, no `waitForTimeout`, and starting
  with `// @feature session:draft-pull-request, session:create-pull-request,
  create-pull-request-modal`.

**Files**: `tests/e2e/create-pull-request.spec.ts` (new)

##### Task 3.2.1a: Write the spec header + happy-path test (~5 min)
- `// @feature session:draft-pull-request, session:create-pull-request,
  create-pull-request-modal` header comment.
- Test: create/select a session with a committed diff (reuse an existing
  `tests/e2e/pages/` helper if one already creates such a fixture, e.g. a
  `SessionFixture` helper — check `tests/e2e/pages/` first), click the
  "Create PR" trigger (`page.getByTestId("create-pr-trigger-<sessionId>")` or
  equivalent), wait for the title input to have a non-empty value via
  `expect(locator).not.toHaveValue("")` (no `waitForTimeout`), edit the
  title, click submit, assert `page.getByTestId("github-pr-link")` becomes
  visible.
- Files: `tests/e2e/create-pull-request.spec.ts`

##### Task 3.2.1b: Write the existing-PR / "View PR" test (~4 min)
- For a session whose `githubPrUrl` is already set, click the trigger,
  assert the modal (or button state) shows a "View PR" link instead of the
  editable form, using `data-testid="github-pr-link"`.
- Files: `tests/e2e/create-pull-request.spec.ts`

##### Task 3.2.1c: Write the disabled-trigger / no-commits-ahead test (~3 min)
- For a session with zero commits ahead, assert the trigger button is
  `disabled` (per AC6's "gate the trigger itself" decision from ux.md §4).
- Files: `tests/e2e/create-pull-request.spec.ts`

##### Task 3.2.1d: Run the spec (~3 min)
- `cd tests/e2e && npx playwright test create-pull-request.spec.ts`.
- Files: none (verification only)

---

## Phase 4: Wrap-up

### Epic 4.1: Registry regeneration + coverage-gap check

##### Task 4.1a: Run `make registry-generate` and diff coverage gaps (~3 min)
- `make registry-generate`; `git diff docs/registry/coverage-gaps.json`;
  confirm the gap count did not increase versus the pre-change baseline
  (AC9).
- Files: `docs/registry/backend-features.json`,
  `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json`
  (generated, do not hand-edit beyond running the command)

### Epic 4.2: Full CI gate

##### Task 4.2a: `make ci` (~10 min, background-run recommended)
- Final definitive pre-push check covering build, test, lint, static
  analysis across the whole change set (Go signature change in Epic 1.1,
  new RPCs in Epics 1.2-1.4, frontend removals in Epic 2.5).
- Files: none (verification only)

##### Task 4.2b: `cd web-app && npx jest --no-coverage` (~3 min)
- Full frontend test suite, catching any collateral breakage from the
  `onRunOneShot` prop removals (Epic 2.5) in tests outside
  `CreatePullRequestModal.test.tsx` itself (e.g. existing
  `SessionActionsOverflow`/`ReviewQueuePanel` tests that may reference the
  removed prop or button label).
- Files: none (verification only)
