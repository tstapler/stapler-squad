# Implementation Plan: backlog-self-resolve

**Feature**: Generalize `request_review`'s CAS precondition to `in_progress`|`pr_pending`
(with an explicit source-status whitelist, not an echo of the observed status) and add a
new `report_duplicate` MCP tool that lets a work session self-resolve a backlog item it
discovers is a duplicate of an already-shipped PR/issue/commit.

**Date**: 2026-08-02
**Status**: Ready for implementation — **except see the concurrent-session conflict note below**
**ADRs**: ADR-005 (GitHub ref verification: dispatcher + HTTP-only + status classification),
ADR-006 (`TriggeredByAgent` applies to both `request_review` source-status paths),
ADR-007 (`report_duplicate` idempotency: reject, don't merge, a differing second ref),
ADR-008 (`report_duplicate` does not gain FR2's active-reviewer refusal)

Source: `project_plans/backlog-self-resolve/requirements.md` (FR1–FR10) +
`research/{stack,features,architecture,pitfalls,build-vs-buy,ux}.md`. Grounding facts below
were verified directly against this worktree's source (not inferred from research prose).

**Concurrent-session note (2026-08-02):** while writing this plan, two ADR files this plan
did not author — `decisions/ADR-001-no-new-backlog-status-for-duplicates.md` and
`decisions/ADR-002-gh-cli-pr-existence-classification.md` — appeared in this same project's
`decisions/` directory between this plan's `mkdir` and its `git add`, indicating a second,
independent SDD planning pass is running concurrently on this same backlog item (item
`da58b867` is self-referential — see `research/features.md` §9.3). This plan's own ADRs were
renumbered ADR-005–ADR-008 to avoid filename collision. The two plans agree on nearly
everything checked (both land on `review` as the target status, both reuse
`VerificationNotes`/`BacklogStatusEvent.Note`), but **disagree on one concrete design point**:
this plan's `ADR-005` adds a new HTTP-based `github.GetPR` so all three ref-verification calls
share one auth mechanism; the other session's `ADR-002` keeps the existing `gh`-CLI-subshell
`GetPRInfoCtx` and classifies "not found" via stderr-substring matching, explicitly rejecting
a new `GetPR` as unrequested scope. See the "Conflicting Decision" section at the bottom of
`ADR-005-github-ref-verification-dispatcher.md` for the full reasoning on both sides. **This
must be reconciled — by comparing both plan.md files and picking one epic/story for the
`GetPR`-vs-`GetPRInfoCtx` decision — before Phase 5 implementation starts; do not implement
both.**

---

## Domain Glossary
*(Ubiquitous language — every domain term that appears as a type, method, or variable name.
Exact names here must be used consistently in code, tests, and comments.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogStatus` | Lifecycle enum for a backlog item (`idea`…`archived`); `session.BacklogStatus = domain.BacklogStatus` (type alias) | `session/backlog.go:14` |
| `BacklogItemPrecondition` | CAS input to `TransitionBacklogItemStatus`: `{ExpectedStatus, ExpectedUpdatedAt, Note}` | `session/repository.go:550` |
| `ErrPreconditionFailed` | Exported sentinel; CAS loss surfaces as `fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, ...)` | `session/repository.go:11-12`; check via `errors.Is` |
| `allowedSelfResolveSourceStatuses` | New package-level whitelist `{in_progress, pr_pending}` shared by `request_review` and `report_duplicate` — the two-step CAS-validation fix (see ADR discussion in Pattern Decisions) | New, `server/mcp/tools_backlog.go` |
| `TriggeredByAgent` | New `TriggeredBy` constant `"agent"`, alongside `TriggeredByUser`/`TriggeredBySystem` | New, `session/backlog.go:90-94` |
| `ItemSessionSummary` | DTO returned by `ListItemSessions`/`GetItemSessionBySessionAndItem`: `{ID, Role, EndedAt, VerificationNotes, ...}` | `session/repository.go:285-308` |
| `SessionRoleWork` / `SessionRoleReview` | Session role string constants (`"work"`, `"review"`) | `session/backlog.go:50-52` |
| active review session | An `ItemSessionSummary` with `Role == SessionRoleReview && EndedAt == nil` for a given item | Detection primitive, reused for FR2 (request_review) and FR5 messaging (report_duplicate) |
| `duplicate_ref` | `report_duplicate`'s required string arg — a GitHub PR/issue/commit URL naming what this item's work duplicates | New MCP tool param |
| `reason` | `report_duplicate`'s required string arg — free text explaining why `duplicate_ref` supersedes this item (max 500 chars) | New MCP tool param |
| `ParsedGitHubRef` | `github.ParseGitHubRef(input) (*ParsedGitHubRef, error)` — generalized parser with `Type ∈ {RefTypePR, RefTypeIssue, RefTypeCommit, ...}` and typed `PRNumber`/`IssueNumber`/`CommitSHA` fields | `github/url_parser.go:13-64,281` — **use this, not `session.ParseGitHubURL`** |
| `RefType` | `github` package enum (`RefTypePR`, `RefTypeBranch`, `RefTypeRepo`, `RefTypeFile`, `RefTypeCommit`, `RefTypeIssue`, `RefTypeCompare`, `RefTypeRelease`) | `github/url_parser.go:11-22` |
| `ErrGitHubRefNotFound` | New exported sentinel in `github/` — a definitive 404 (ref doesn't exist, or exists but is invisible to the configured token) | New, ADR-005 |
| `ErrGitHubAccessDenied` | New exported sentinel — 401, or 403 with no rate-limit signal (retrying with the same token will not change the outcome) | New, ADR-005 |
| `GetCommit` | New `github.GetCommit(ctx, owner, repo, sha) (*CommitResult, error)` — HTTP existence check for a commit SHA, mirrors `GetIssue`'s shape | New, `github/commits.go` |
| `GetPR` | New `github.GetPR(ctx, owner, repo, number) (*PRResult, error)` — HTTP (not `gh` CLI) existence check for a PR | New, `github/repos.go`, ADR-005 |
| `verifyGitHubRefExists` | New `(h *backlogHandlers) verifyGitHubRefExists(ctx, ref *ParsedGitHubRef) (bool, error)` — single dispatcher over the 3 ref types, returning the same 3-way contract `verifyPR`/`VerifyPRMatchesBranch` already use | New, `server/mcp/tools_backlog.go`, Pattern Decision below |
| `hasActiveReviewSession` | New unexported helper in `server/mcp` — local copy of the filter already duplicated in `server/services` and `session`, per architecture.md §3 | New, `server/mcp/tools_backlog.go` |
| `errResult` / `ErrInvalidArgument` / `ErrInternalError` / `ErrPermissionDenied` / `ErrItemNotFound` | Existing MCP error-result vocabulary — reused as-is, no new codes | `server/mcp/tools_discovery.go:73`, `server/mcp/types.go:63-64`, `server/mcp/tools_backlog.go:57-58` |
| `report_duplicate` | The new MCP tool this feature adds | `server/mcp/tools_backlog.go`, registered in `registerBacklogTools` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| CAS precondition construction (`request_review` + `report_duplicate`) | Two-step validate-then-pin: whitelist-check `item.Status` first (reject if not in `{in_progress, pr_pending}`), only then set `ExpectedStatus` to that validated value | pitfalls.md §0 (highest-severity finding) | `ExpectedStatus: item.Status` verbatim (one-line substitution) | Verbatim substitution makes the CAS trivially self-satisfying for *any* observed status (`done`, `archived`, ...) — defeats the precondition's entire purpose. Not a race-safety issue (the atomic `UPDATE...WHERE` already closes that, per architecture.md §1) — a plain logic bug. |
| GitHub ref verification dispatch (`report_duplicate`) | **Option A — single `verifyGitHubRefExists(ctx, ref) (bool, error)` dispatcher with an internal `switch ref.Type`**, mirroring `reportPRCreated`'s existing `h.verifyPR` seam (`tools_backlog.go:707`) | Step 0.5 creative pass, this doc | **Option C — `GitHubRefVerifier` interface, DI'd like `h.verifyPRMatchesBranch`**: one sentence strength — clean seam for a future second implementation (e.g. a mock in tests); one sentence weakness — a single-implementation interface for 3 fixed, closed ref kinds is speculative indirection (interface-pollution-checklist.md smell #1), and the switch is exhaustive/compile-checked either way, so DI buys nothing testability-wise that a plain function doesn't already give (tests can call the 3 `github.Get*` funcs directly or hit an `httptest.Server`). | Both alternatives considered and rejected below. |
| (rejected alt. 2) | — | — | **Option B — three separate typed calls invoked directly from the handler with a switch inline in `reportDuplicate`** — strength: one fewer function/indirection; weakness: duplicates the switch's dispatch logic into the handler body, coupling arg-parsing/refusal-check code with verification-call code in one large function, harder to unit-test the dispatch decision in isolation from the full handler prologue. | — | — | — |
| GitHub ref parsing | Reuse `github.ParseGitHubRef` as-is (no changes) | stack.md §5, architecture.md §5 | Extend `session.ParseGitHubURL` with new `GitHubRefType` cases (build-vs-buy.md's original Option A framing) | A more thorough read (architecture.md, features.md) found `github.ParseGitHubRef` already has `RefTypeIssue`/`RefTypeCommit` fully populated — extending the narrower, PR-only `session.ParseGitHubURL` would duplicate work a better parser already does. |
| GitHub existence-check auth mechanism | **HTTP-only for all 3 ref types** — native `net/http` via `ghHTTPClient`/`getGHToken`, same as `GetIssue`; add HTTP-based `GetPR`, do **not** call `GetPRInfoCtx` (`gh` CLI subprocess) from `report_duplicate` | ADR-005 | Reuse `GetPRInfoCtx` for the PR case, `GetIssue`/`GetCommit` (HTTP) for the other two | pitfalls.md §4: `GetPRInfoCtx` resolves auth via `gh auth login`/`CheckGHAuth()`; `GetIssue` resolves auth via `GITHUB_TOKEN`/`GH_TOKEN` env or keychain (`getGHToken`, independent path). A host with one configured but not the other would pass verification for 2 of 3 ref types and fail the 3rd with a *different failure mode* — confusing and untestable as one coherent contract. One mechanism for all three closes this. |
| GitHub error classification (404/401/403/429) | Typed sentinels `ErrGitHubRefNotFound` (404) / `ErrGitHubAccessDenied` (401, or 403 w/o rate-limit signal) shared across `GetIssue`(retrofit)/`GetPR`/`GetCommit`; `errors.Is` in the handler, not string/status matching | ADR-005 | Per-call `fmt.Errorf` text matched by the handler (as `GetIssue` does today) | `errors.Is` is robust to message-text changes; a shared sentinel means `verifyGitHubRefExists`'s classification logic is written once, not duplicated 3×. Small additive change to `GetIssue` (wrap its existing 404/401/403-no-Retry-After branches), no behavior change for existing callers of `GetIssue` since the returned error still satisfies `errors.Is` against nothing before *and* now additionally against the new sentinel — existing `err != nil` callers are unaffected. |
| `report_duplicate`'s outcome data model | Pack `duplicate_ref` + `reason` into the existing free-text `VerificationNotes` field (ItemSession) and `BacklogStatusEvent.Note` (status-event row) | FR8, ux.md §4 | A `DuplicateReport` domain value object / new ent columns | FR8 explicitly forbids schema changes; two strings with no independent lifecycle or validation beyond length caps don't need a value-object wrapper — `fmt.Sprintf` into the two existing free-text fields is the entire "domain model" this needs (interface-pollution-checklist.md #6, don't wrap what doesn't add behavior). |
| `TriggeredBy` scope for the generalized `request_review` | **Both** source-status paths (`in_progress→review` and `pr_pending→review`) switch from `TriggeredBySystem` to `TriggeredByAgent`, not just the new `report_duplicate` call | ADR-006 | Only `report_duplicate` gets the new constant; `request_review` keeps `TriggeredBySystem` | AC7 says "every transition made by this feature." FR1's generalization *is* part of this feature's diff — leaving `request_review`'s attribution as `"system"` while `report_duplicate`'s reads `"agent"` would make two functionally-identical agent-initiated actions audit-inconsistently. |
| `report_duplicate` double-report handling | **Reject** a second call with a *different* `duplicate_ref` once the item has already left the whitelist (i.e. already `review`/`done`) — do not append/merge into `VerificationNotes` | ADR-007 | Merge/append the second ref into `VerificationNotes` | `UpdateItemSessionVerificationNotes` overwrites, not appends (confirmed, `session/storage_backlog.go:397`). Appending would require read-modify-write with its own race; rejecting keeps the "routes to review once, human/reviewer takes it from there" contract simple and matches `report_pr_created`'s own idempotency precedent (same-ref no-op, nothing more sophisticated). |
| `report_duplicate` + active review session | **No FR2-style hard refusal** — `report_duplicate` may transition even if an active review-role session exists for the item; only the *success-message wording* changes (FR5) | ADR-008 | Extend FR2's refusal (as request_review has) to report_duplicate too | Requirements.md's FR6 enumerates exactly 3 refusal conditions, none mentioning active sessions; FR5 explicitly describes report_duplicate *succeeding* while a review session is active. A hard refusal here would make FR5 unreachable — see ADR-008 for full reasoning and the residual ambiguity flagged for owner sign-off. |

---

## Migration Plan
N/A, no schema changes per FR8 / `decisions/ADR-001-no-new-backlog-status-for-duplicates.md` (a separate ADR in this project's `decisions/` directory — filename genuinely starts with `ADR-001`, unrelated to this plan's own `ADR-005`..`ADR-008` numbering, which was renumbered to avoid colliding with it; it was written by a concurrent planning pass on this same item — see the conflict note atop `ADR-005-github-ref-verification-dispatcher.md`). `go build ./...` must succeed with zero `ent generate` runs; verified as an explicit CI-equivalent task at the end of Phase 4.

## Observability Plan
- **Logs**: `report_duplicate` logs one `log.InfoLog` line on success (`session=%s item=%s duplicate_ref=%s transitioned to review`) mirroring `request_review`'s existing line (`tools_backlog.go:429`) and `report_pr_created`'s (`:721`); one `log.WarningLog` line if `UpdateItemSessionVerificationNotes` fails (best-effort, matches `request_review:425`); GitHub verification failures are logged at `log.InfoLog` with the ref and error before returning the `ErrInternalError`/`ErrInvalidArgument` result, so a human grepping logs can correlate a stuck `pr_pending` item with why `report_duplicate` never succeeded for it.
- **Metrics**: none — this codebase has no existing per-tool-call metrics emission in `server/mcp` (confirmed: no `metrics.`/`prometheus.` import in `tools_backlog.go`); consistent with every other tool in this file, no new metric added.
- **Alerts**: none required — FR10's stuck-item surfacing is satisfied entirely by the pre-existing `pr_pending_no_pr` detector (`session/backlog_lifecycle.go` ~L2545-2557) and its UI (`StuckItemsSection.tsx`), confirmed by architecture.md §8 and ux.md §5. No new alert/notification path is added.

## Risk Control
- **Feature flag**: not gated — `featureDisabledResult(h.enabledCheck)` (the existing MCP-backlog-tools kill switch every handler in this file already checks first) covers `report_duplicate` automatically once it's registered in the same `backlogHandlers` struct; no new flag needed.
- **Rollback procedure**: standard revert via PR close + revert commit. Both changes (CAS whitelist fix, new tool) are additive/backward-compatible at the DB level (no schema change, existing rows unaffected) — a revert is a pure code rollback with no data migration to undo.
- **Staged rollout**: full rollout on merge — this is an internal agent-tooling change (MCP tool surface consumed only by this repo's own work sessions), not a user-facing product surface; no cohort/percentage rollout mechanism exists for MCP tools in this codebase.

## Unresolved Questions
- [x] **UQ-1 (resolved, flagged for confirmation)**: Does `report_duplicate` refuse when an active review-role session exists (mirroring FR2), or only adjust its success message (FR5)? **Plan adopts: no additional refusal** (ADR-008) — because FR5 presupposes `report_duplicate` can succeed in that state. Confirm with the item owner before Phase 3 lands if this reading is wrong; if it is, Story 3.3.3 needs an added refusal branch and FR5's example (GWT for AC5 below) becomes unreachable and must be rewritten.
- [x] **UQ-2 (resolved)**: 403-with-no-rate-limit-signal classification — resolved as `ErrGitHubAccessDenied` → `ErrInvalidArgument` (non-retryable) in ADR-005, not `ErrInternalError`. No further blocking action.
- [ ] **UQ-3**: Does `hasActiveReviewSession`'s pre-existing "zombie reviewer" false-positive (dead tmux session, `EndedAt` still nil) need a one-line hint in `request_review`'s FR2 refusal message ("if this persists after a few minutes, an operator can check with `get_backlog_item`")? Not blocking — pitfalls.md calls this an accepted pre-existing limitation — but Story 2.2.1 should include the hint text since it's a one-line addition once the guard exists. Not a design blocker, just confirm the exact wording lands in the task, not skipped.

## Dependency Visualization

```
Phase 1: Foundations (no dependents outside this phase)
  Epic 1.1 TriggeredByAgent const ─────────────┐
  Epic 1.2 GitHub verification primitives       │
    Story 1.2.1 sentinels + GetIssue retrofit ──┤
    Story 1.2.2 GetPR (HTTP) ────────────────┐  │
    Story 1.2.3 GetCommit ───────────────────┤  │
                                              ▼  ▼
Phase 2: request_review generalization (needs Epic 1.1)
  Epic 2.1 CAS whitelist + dynamic precondition
  Epic 2.2 FR2 active-reviewer guard (needs Epic 2.1)
                                              │
Phase 3: report_duplicate new tool (needs Epics 1.1, 1.2, 2.1's whitelist helper)
  Epic 3.1 handler skeleton + refusal checks
  Epic 3.2 GitHub verification dispatch (needs Epic 1.2 in full)
  Epic 3.3 transition + audit trail (needs Epic 3.1, 3.2)
  Epic 3.4 MCP registration (needs Epic 3.3)
                                              │
Phase 4: Tests (needs everything above)
  Epic 4.1 request_review regression + new tests
  Epic 4.2 report_duplicate tests
  Epic 4.3 github package unit tests (can start once Phase 1 lands, listed last for narrative flow)
```

---

## Phase 1: Foundations

### Epic 1.1: `TriggeredByAgent` audit constant
**Goal**: Add the one missing `TriggeredBy` value both generalized `request_review` and new `report_duplicate` need to pass to `TransitionBacklogItemStatus`.

#### Story 1.1.1: Add `TriggeredByAgent` constant
**As a** transition caller in `server/mcp`, **I want** a `TriggeredByAgent` constant, **so that** agent-initiated transitions are audit-distinguishable from `TriggeredBySystem` (reconciler-driven) and `TriggeredByUser` (human-driven) transitions.
**Acceptance Criteria** (supports FR7/AC7):
- `session.TriggeredByAgent == "agent"` compiles and is usable from `server/mcp`.
  - *Given* `session/backlog.go`'s existing `const (TriggeredByUser = "user"; TriggeredBySystem = "system")` block (lines 90-94), *When* `TriggeredByAgent = "agent"` is added to that same block, *Then* `go build ./...` succeeds with no other file requiring changes (plain untyped string constant, no enum/switch to update — confirmed by stack.md §3: "no exhaustiveness/switch to update").
**Files**: `session/backlog.go`

##### Task 1.1.1a: Add the constant (~2 min)
- Edit `session/backlog.go:90-94`: change the `const (...)` block to add `TriggeredByAgent = "agent"` as a third line, with a one-line doc-comment addition above the block noting agent-triggered transitions (e.g. `request_review`, `report_duplicate`) use this value.
- Files: `session/backlog.go`

---

### Epic 1.2: GitHub verification primitives
**Goal**: Add the HTTP-based, sentinel-error-returning verification calls `report_duplicate` needs for all three ref types, per ADR-005's HTTP-only decision.

#### Story 1.2.1: Sentinel errors + `GetIssue` retrofit
**As a** future caller of any GitHub existence-check function in this package, **I want** `errors.Is`-checkable sentinels for "not found" and "access denied", **so that** classification doesn't depend on parsing error text.
**Acceptance Criteria**:
- `github.ErrGitHubRefNotFound` and `github.ErrGitHubAccessDenied` are exported `errors.New(...)` sentinels.
  - *Given* a call to `github.GetIssue(ctx, "tstapler", "stapler-squad", 999999)` where issue 999999 does not exist, *When* GitHub responds 404, *Then* the returned error satisfies `errors.Is(err, github.ErrGitHubRefNotFound)`.
- Existing `GetIssue` callers (there are currently none outside tests, confirmed via grep — this is a same-file retrofit) see no behavior change beyond the wrapped sentinel; the human-readable error text is preserved (wrapped, not replaced).
**Files**: `github/repos.go`, `github/errors.go` (new, or add to top of `repos.go` if no dedicated errors file exists — check first)

##### Task 1.2.1a: Grep for an existing `github/errors.go` (~1 min)
- Run `grep -rn "^var Err" github/*.go` to find where `ErrNoPR`/`ErrNotAuthenticated` are declared (`github/client.go:24`, `github/repos.go:16`) — no dedicated errors file exists; add new sentinels next to the existing ones in their respective files, don't create a new file (this repo's convention is sentinel-near-first-use, per `ErrNoPR` living in `client.go` next to `GetPRForBranch`).
- Files: none changed, read-only

##### Task 1.2.1b: Add `ErrGitHubRefNotFound` and `ErrGitHubAccessDenied` (~3 min)
- Add both `var Err... = errors.New("...")` declarations in `github/repos.go` near `ErrNotAuthenticated` (`repos.go:16`), each with a one-line doc comment (404 = definitively doesn't exist, or exists but invisible to this token; access-denied = 401, or 403 with no rate-limit signal — retrying with the same credentials will not help).
- Files: `github/repos.go`

##### Task 1.2.1c: Retrofit `GetIssue`'s 404/401/403 branches to wrap the sentinels (~4 min)
- In `GetIssue` (`github/repos.go:270-325`): change the `resp.StatusCode == 404` branch (`:292-294`) to `return nil, fmt.Errorf("%w: issue not found (404)", ErrGitHubRefNotFound)`; change the `401` branch (`:288-291`) and the no-rate-limit-signal `403` branch (`:295,304-306`, i.e. the final `body, _ := io.ReadAll...` fallback inside the 403 block, NOT the `Retry-After`/`X-RateLimit-Remaining` branches which stay as plain transient errors) to wrap `ErrGitHubAccessDenied` similarly.
- Files: `github/repos.go`

#### Story 1.2.2: `GetPR` — new HTTP-based PR existence check
**As** `report_duplicate`'s verification dispatcher, **I want** a PR-existence check that uses the same HTTP/auth mechanism as `GetIssue`, **so that** all three ref types share one auth resolution path (ADR-005).
**Acceptance Criteria**:
- `github.GetPR(ctx, owner, repo, number) (*PRResult, error)` exists, uses `newGHRequest`/`ghHTTPClient` (not `safeexec.CommandContext("gh", ...)`), and returns the same sentinel-wrapped errors as `GetIssue` for 404/401/403/429.
  - *Given* PR `#272` exists on `tstapler/stapler-squad`, *When* `GetPR(ctx, "tstapler", "stapler-squad", 272)` is called, *Then* it returns `(&PRResult{Number: 272, ...}, nil)` without invoking `gh` as a subprocess (verified by an `httptest.Server`-backed unit test, not by a live network call).
**Files**: `github/repos.go`

##### Task 1.2.2a: Define `PRResult` struct (~2 min)
- Add `type PRResult struct { Number int; Title string; State string; HTMLURL string }` near `IssueResult` (`repos.go:27-38`), same field-naming convention.
- Files: `github/repos.go`

##### Task 1.2.2b: Implement `GetPR` mirroring `GetIssue`'s body exactly (~5 min)
- Copy `GetIssue`'s structure (`repos.go:270-330`, including the now-retrofitted sentinel wrapping from Task 1.2.1c): `getGHToken` check → `ErrNotAuthenticated`; build `apiPath := fmt.Sprintf("repos/%s/%s/pulls/%d", ...)`; `newGHRequest`/`ghHTTPClient.Do`; same 401/404/403/429/other-status branches wrapping the new sentinels; unmarshal into a minimal `ghPRJSON` struct (`number`, `title`, `state`, `html_url`) and map to `PRResult`.
- Files: `github/repos.go`

#### Story 1.2.3: `GetCommit` — new commit-existence check
**As** `report_duplicate`'s verification dispatcher, **I want** a commit-existence check, **so that** `duplicate_ref` pointing at a commit SHA URL can be verified (no such function exists anywhere in `github/` today, confirmed by grep in stack.md §5/build-vs-buy.md §1).
**Acceptance Criteria**:
- `github.GetCommit(ctx, owner, repo, sha) (*CommitResult, error)` exists in a new `github/commits.go`, follows the identical HTTP/sentinel pattern as `GetIssue`/`GetPR`.
  - *Given* commit `a1b2c3d` does not exist in `tstapler/stapler-squad` (fabricated short SHA), *When* `GetCommit(ctx, "tstapler", "stapler-squad", "a1b2c3d")` is called against a mocked 404 response, *Then* it returns `(nil, err)` where `errors.Is(err, ErrGitHubRefNotFound)` is true.
**Files**: `github/commits.go` (new)

##### Task 1.2.3a: Create `github/commits.go` with `CommitResult` + `GetCommit` (~5 min)
- New file, `package github`. `type CommitResult struct { SHA string; HTMLURL string; Message string; Author string }`. `GetCommit(ctx, owner, repo, sha string) (*CommitResult, error)` hitting `GET repos/{owner}/{repo}/commits/{sha}`, same auth/status-branch structure as `GetIssue`/`GetPR` (Tasks 1.2.1c/1.2.2b), wrapping `ErrGitHubRefNotFound`/`ErrGitHubAccessDenied` identically. Unmarshal minimal fields (`sha`, `html_url`, `commit.message`, `commit.author.name`) from the GitHub commit-object JSON shape.
- Files: `github/commits.go`

##### Task 1.2.3b: Unit test `GetCommit` against an `httptest.Server` (~5 min)
- New `github/commits_test.go`: table test with cases `{200 → success}`, `{404 → ErrGitHubRefNotFound}`, `{401 → ErrGitHubAccessDenied}`, `{403 no Retry-After → ErrGitHubAccessDenied}`, `{403 w/ Retry-After → plain transient error, NOT a sentinel}`, `{429 → plain transient error}`. Point `ghHTTPClient`/base URL at the `httptest.Server` the way any existing `github/*_test.go` file already does (check `github/repos_test.go` or `client_test.go` for the exact test-server-injection pattern before writing — this repo already has one, don't invent a second).
- Files: `github/commits_test.go`

##### Task 1.2.3c: Unit test `GetPR` the same way (~4 min)
- Mirror Task 1.2.3b's table for `GetPR`, in a new `github/repos_pr_test.go` (or append to existing `github/repos_test.go` if that file already covers `GetIssue` — check first, colocate with `GetIssue`'s own tests for symmetry).
- Files: `github/repos_pr_test.go` or `github/repos_test.go`

---

## Phase 2: `request_review` CAS generalization

### Epic 2.1: Whitelist-validated dynamic precondition (FR1, FR9)
**Goal**: Replace the hardcoded `ExpectedStatus: string(session.BacklogStatusInProgress)` with the two-step validate-then-pin fix from Pattern Decisions, without altering behavior for any existing `in_progress`-sourced test.

#### Story 2.1.1: Add the shared whitelist + validate before building the precondition
**As** `request_review`'s handler, **I want** to reject calls from a disallowed source status *before* constructing any `BacklogItemPrecondition`, **so that** the CAS check is never trivially self-satisfying.
**Acceptance Criteria** (FR1):
- *Given* item `da58b867-bf4e-4720-8fe4-9cfcfa5b6eed` at status `pr_pending` (its own PR #281 open, awaiting merge) with a linked work-role `ItemSession` (UUID `3f9a2c10-8b4e-4a91-9c77-1e5d6a2b9f00`), *When* that session calls `request_review(item_id="da58b867-bf4e-4720-8fe4-9cfcfa5b6eed", message="Re-requesting review after addressing PR feedback")`, *Then* the precondition passed to `TransitionBacklogItemStatus` is `{ExpectedStatus: "pr_pending", Note: ...}` (not the old hardcoded `"in_progress"`), the transition succeeds, and a follow-up `GetBacklogItem` returns `Status: "review"`.
- *Given* the same item now at status `done` (e.g. a stale/mistaken caller), *When* `request_review` is called on it, *Then* the call is refused with `ErrInvalidArgument` ("item is at status \"done\" — request_review only allowed from in_progress or pr_pending"), **before** any `TransitionBacklogItemStatus` call is made, and `GetBacklogItem` afterward still shows `Status: "done"`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 2.1.1a: Define the shared whitelist (~2 min)
- Add near the top of `tools_backlog.go` (with the other package-level vars/consts, e.g. near the `ErrPermissionDenied`/`ErrItemNotFound` block at line 57): `var allowedSelfResolveSourceStatuses = map[session.BacklogStatus]bool{session.BacklogStatusInProgress: true, session.BacklogStatusPRPending: true}`, with a doc comment referencing the CAS-trap pitfall (one sentence: validating *before* building the precondition is what keeps the compare-and-swap meaningful).
- Files: `server/mcp/tools_backlog.go`

##### Task 2.1.1b: Insert the whitelist check + build the dynamic precondition in `requestReview` (~5 min)
- In `requestReview` (`tools_backlog.go:404-414`), immediately after `item, itemErr := h.storage.GetBacklogItem(...)` and its error check, insert: `if !allowedSelfResolveSourceStatuses[session.BacklogStatus(item.Status)] { return errResult(ErrInvalidArgument, fmt.Sprintf("item is at status %q — request_review only allowed from in_progress or pr_pending", item.Status), ""), nil }`. Then replace line 414's hardcoded precondition with `precondition := &session.BacklogItemPrecondition{ExpectedStatus: item.Status, Note: fmt.Sprintf("request_review from %s", message)}` — using the now-*validated* `item.Status`, and populating `Note` for the first time (currently empty; ux.md §4 wants every transition to leave a human-legible note).
- Files: `server/mcp/tools_backlog.go`

##### Task 2.1.1c: Switch `TriggeredBy` to `TriggeredByAgent` on both paths (~2 min)
- On line 415's `TransitionBacklogItemStatus` call, change `session.TriggeredBySystem` → `session.TriggeredByAgent` (ADR-006 — applies uniformly regardless of which validated source status was observed, no branching needed since it's the same call site either way).
- Files: `server/mcp/tools_backlog.go`

#### Story 2.1.2: Distinguish `ErrPreconditionFailed` from generic transition errors
**As** a caller that lost a CAS race, **I want** a distinct, non-retry message, **so that** I don't waste a retry on an action that can never succeed now that the item moved on.
**Acceptance Criteria** (pitfalls.md §1/§5c):
- *Given* item `da58b867-...` at `pr_pending`, *When* two concurrent `request_review` calls race (or `request_review` races `ReconcileStuck`'s own CAS'd auto-`done` transition) such that one call's `TransitionBacklogItemStatus` call observes the row has already changed, *Then* the losing call receives `ErrInternalError`... — no: per the fix, the losing call receives a **distinct** message ("item state changed since your last read — call get_backlog_item to see its current status") rather than the generic "transition to %s failed: %v" text, while still using `ErrInternalError` as the error code (no new error code is introduced — see Pattern Decisions: this reuses the existing `ErrInternalError` code with different message text, matching FR4's "code stays the same family, wording carries the retry-worthiness signal" convention already established for `report_pr_created`).
**Files**: `server/mcp/tools_backlog.go`

##### Task 2.1.2a: Branch the transition-error handling on `errors.Is(transErr, session.ErrPreconditionFailed)` (~4 min)
- Replace the single `if _, transErr := ...; transErr != nil { return errResult(ErrInternalError, fmt.Sprintf("transition to %s failed: %v", ...), "") }` block (`tools_backlog.go:415-418`) with a two-branch version: `errors.Is(transErr, session.ErrPreconditionFailed)` → `errResult(ErrInternalError, "item state changed since your last read (another action already transitioned it) — call get_backlog_item to see its current status", "")`; else → the existing generic message unchanged.
- Files: `server/mcp/tools_backlog.go`

---

### Epic 2.2: FR2 active-reviewer guard on the `pr_pending` path
**Goal**: Refuse `request_review` when source status is `pr_pending` and an active review-role session already exists, leaving the `in_progress` path's existing behavior untouched.

#### Story 2.2.1: `hasActiveReviewSession` helper + guard, scoped to the `pr_pending` path only
**As** `request_review`'s handler, **I want** to refuse re-routing a `pr_pending` item out from under a running reviewer, **so that** a second review gate is never spawned concurrently with a live one.
**Acceptance Criteria** (FR2):
- *Given* item `da58b867-...` at `pr_pending` with an active (unended) review-role `ItemSession` (UUID `7c1e2f44-9a11-4b02-8e77-2d0c9a5b1234`, `EndedAt: nil`) already running for it, *When* the linked work session calls `request_review` again on the same item, *Then* the call is refused with `ErrInvalidArgument` ("an active review session already exists for this item — wait for it to finish, or check get_backlog_item if this persists"), zero DB mutation occurs, and `GetBacklogItem` afterward still shows `Status: "pr_pending"`.
- *Given* the same item at `in_progress` (not `pr_pending`) with that same active review session present (the pre-existing "zombie reviewer" edge case, UQ-3), *When* `request_review` is called, *Then* the call proceeds exactly as it does today — the guard is scoped to the `pr_pending` source path only, per FR2's explicit "the in_progress-sourced path's existing behavior must be unchanged."
**Files**: `server/mcp/tools_backlog.go`

##### Task 2.2.1a: Add the local `hasActiveReviewSession` helper (~3 min)
- Add an unexported function near `requestReview`: `func hasActiveReviewSession(sessions []session.ItemSessionSummary) bool { for _, s := range sessions { if s.Role == session.SessionRoleReview && s.EndedAt == nil { return true } }; return false }` — a fourth copy of the same one-line predicate already duplicated in `server/services/backlog_service_triage.go:1104,926` and `session/backlog_lifecycle.go:3351`, per architecture.md §3's precedent (each package keeps its own copy; `server/mcp` cannot import `server/services`, and there is no existing precedent for it importing that package).
- Files: `server/mcp/tools_backlog.go`

##### Task 2.2.1b: Call the guard on the `pr_pending` path only, before the precondition is built (~4 min)
- In `requestReview`, after Task 2.1.1b's whitelist check and before Task 2.1.1b's precondition construction, insert: `if item.Status == string(session.BacklogStatusPRPending) { itemSessions, lsErr := h.storage.ListItemSessions(ctx, itemID); if lsErr == nil && hasActiveReviewSession(itemSessions) { return errResult(ErrInvalidArgument, "an active review session already exists for this item — wait for it to finish, or check get_backlog_item if this persists", ""), nil } }`. Note: a `ListItemSessions` error itself is swallowed here (falls through to attempt the transition normally) rather than hard-failing the call — consistent with this being a best-effort guard layered on top of the CAS, which remains the actual safety net.
- Files: `server/mcp/tools_backlog.go`

---

## Phase 3: `report_duplicate` new tool

### Epic 3.1: Handler skeleton + refusal checks (FR6)
**Goal**: Args parsing, session-link/role checks, and all zero-mutation refusal paths, ordered so every refusal happens before the GitHub network call (pitfalls.md §5b).

#### Story 3.1.1: Args parsing + link/role checks
**As** the new `report_duplicate` handler, **I want** the same auth/link/role prologue every other backlog tool uses, **so that** permission handling is consistent across the file.
**Acceptance Criteria** (FR6, part 1 — role and link refusals):
- *Given* a session UUID `9b3f...` that is **not** linked to item `da58b867-...`, *When* it calls `report_duplicate(item_id="da58b867-bf4e-4720-8fe4-9cfcfa5b6eed", duplicate_ref="https://github.com/tstapler/stapler-squad/pull/272", reason="dup")`, *Then* it is refused with `ErrPermissionDenied` ("this session is not linked to the specified backlog item"), no GitHub call is made.
- *Given* a **review**-role session linked to `da58b867-...`, *When* it calls `report_duplicate` with the same args, *Then* it is refused with `ErrPermissionDenied` ("session role is \"review\" — only 'work' role may report a duplicate"), no GitHub call is made.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.1.1a: Function skeleton, arg extraction, length caps (~5 min)
- New `func (h *backlogHandlers) reportDuplicate(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error)`, placed immediately after `reportPRCreated` (after line 726). Copy `reportPRCreated`'s prologue shape (`featureDisabledResult`, `callerSessionUUID`, arg map) and extract `item_id` (required, `validateUUID`), `duplicate_ref` (required string, max 500 chars), `reason` (required string, max 1000 chars — mirrors `report_pr_created`'s `summary` cap).
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.1b: Link check + work-role check (~3 min)
- Copy `reportPRCreated`'s exact 6-line link-check block (`tools_backlog.go:662-668`) and role check (`:669-671`), substituting the "report a duplicate" wording into the role-check error message.
- Files: `server/mcp/tools_backlog.go`

#### Story 3.1.2: `SkipReviewGate` refusal, whitelist refusal, idempotency no-op
**As** `report_duplicate`, **I want** to refuse `SkipReviewGate` items outright (not route them to `done`, the opposite of `request_review`'s pattern) and reject calls from a disallowed source status, **so that** FR6's zero-mutation guarantee holds and the CAS trap (Pattern Decisions) doesn't apply here either.
**Acceptance Criteria** (FR6 remainder, plus the idempotency corollary):
- *Given* item `da58b867-...` has `SkipReviewGate: true`, *When* a linked work session calls `report_duplicate`, *Then* it is refused with `ErrInvalidArgument` ("report_duplicate is unavailable for items with SkipReviewGate enabled — use request_review instead"), **no GitHub call is made** (this check runs before parsing/verifying `duplicate_ref`), and `GetBacklogItem` afterward shows the item's status/fields unchanged.
- *Given* item `da58b867-...` at status `done` (already shipped through a different path), *When* `report_duplicate` is called, *Then* it is refused with `ErrInvalidArgument` ("item is at status \"done\" — report_duplicate only allowed from in_progress or pr_pending"), zero mutation, before any network call.
- *Given* item `da58b867-...` already at `review` with `VerificationNotes` containing the exact string `"duplicate_ref=https://github.com/tstapler/stapler-squad/pull/272"` (i.e. this exact call already succeeded once), *When* `report_duplicate` is called again with the identical `duplicate_ref`, *Then* it returns a success no-op ("duplicate report for https://github.com/tstapler/stapler-squad/pull/272 already recorded for item da58b867-... (status already review) — no changes made"), no second `TransitionBacklogItemStatus` call is attempted.
- *Given* the same already-`review` item, *When* `report_duplicate` is called with a **different** `duplicate_ref` (e.g. `.../pull/300`), *Then* it is refused with `ErrInvalidArgument` (falls into the same "disallowed source status" branch as the `done` case above — `review` is not in the whitelist and the ref doesn't match the idempotency string) — per ADR-007, the second ref is **not** merged into `VerificationNotes`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.1.2a: `GetBacklogItem` + `SkipReviewGate` refusal (~3 min)
- After the role check, `item, getErr := h.storage.GetBacklogItem(ctx, itemID)` (mirror `reportPRCreated:673-679`'s error handling), then immediately: `if item.SkipReviewGate { return errResult(ErrInvalidArgument, "report_duplicate is unavailable for items with SkipReviewGate enabled — use request_review instead.", ""), nil }` — this is the FR6-vs-FR2-pattern trap from pitfalls.md §5b; do **not** copy `request_review`'s `targetStatus = BacklogStatusDone` pattern here.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.1.2b: Idempotency check, then whitelist refusal (~5 min)
- Immediately after: build `notesMarker := fmt.Sprintf("duplicate_ref=%s", duplicateRef)`. If `item.Status == string(session.BacklogStatusReview) && strings.Contains(itemSession.VerificationNotes, notesMarker)`, return the no-op success text above. Otherwise, apply the same `allowedSelfResolveSourceStatuses` whitelist check from Task 2.1.1a (reuse the package-level var — do not redefine it): `if !allowedSelfResolveSourceStatuses[session.BacklogStatus(item.Status)] { return errResult(ErrInvalidArgument, fmt.Sprintf("item is at status %q — report_duplicate only allowed from in_progress or pr_pending", item.Status), ""), nil }`.
- Files: `server/mcp/tools_backlog.go`

---

### Epic 3.2: GitHub verification dispatch (FR3, FR4)
**Goal**: Parse `duplicate_ref`, reject unsupported ref shapes cheaply (no network), then verify existence via the `verifyGitHubRefExists` dispatcher (Pattern Decisions, Option A) with the two-channel FR4 error split.

#### Story 3.2.1: Parse + ref-type validation (cheap, no network)
**As** `report_duplicate`, **I want** to reject a malformed or unsupported `duplicate_ref` before any GitHub call, **so that** obviously-bad input fails fast (mirrors `reportPRCreated`'s pre-network sanity check, `tools_backlog.go:688-697`).
**Acceptance Criteria** (part of FR3):
- *Given* `duplicate_ref="not a url at all"`, *When* `report_duplicate` is called, *Then* `github.ParseGitHubRef` fails to parse it and the call is refused with `ErrInvalidArgument` ("duplicate_ref is not a recognizable GitHub PR/issue/commit URL"), no network call made.
- *Given* `duplicate_ref="https://github.com/tstapler/stapler-squad/tree/main"` (a branch URL — parses successfully as `RefTypeBranch`, but that's not one of the 3 supported kinds), *When* `report_duplicate` is called, *Then* it is refused with `ErrInvalidArgument` ("duplicate_ref must be a GitHub PR, issue, or commit URL — got a Branch reference"), no network call made.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.2.1a: Parse + type-validate (~4 min)
- `ref, parseErr := githubpkg.ParseGitHubRef(duplicateRef); if parseErr != nil { return errResult(ErrInvalidArgument, fmt.Sprintf("duplicate_ref is not a recognizable GitHub PR/issue/commit URL: %v", parseErr), ""), nil }`. Then: `if ref.Type != githubpkg.RefTypePR && ref.Type != githubpkg.RefTypeIssue && ref.Type != githubpkg.RefTypeCommit { return errResult(ErrInvalidArgument, fmt.Sprintf("duplicate_ref must be a GitHub PR, issue, or commit URL — got a %s reference", ref.Type), ""), nil }`.
- Files: `server/mcp/tools_backlog.go`

#### Story 3.2.2: `verifyGitHubRefExists` dispatcher + two-channel error mapping
**As** `report_duplicate`, **I want** one function that dispatches to `GetPR`/`GetIssue`/`GetCommit` by ref type and returns the same 3-way contract `verifyPR` already uses, **so that** the FR4 split is written once, consistently.
**Acceptance Criteria** (FR4):
- *Given* `duplicate_ref="https://github.com/tstapler/stapler-squad/pull/999999"` (nonexistent PR number), *When* `report_duplicate` is called, *Then* `GetPR` returns a 404-wrapped `ErrGitHubRefNotFound`, `verifyGitHubRefExists` returns `(false, nil)`, and the tool returns `ErrInvalidArgument` ("PR #999999 does not exist on tstapler/stapler-squad on GitHub (404) — double-check the URL. Note: a private/inaccessible repo also returns 404.").
- *Given* the GitHub API call for a genuinely-existing `duplicate_ref` times out (network error, not a status code), *When* `report_duplicate` is called, *Then* `verifyGitHubRefExists` returns `(false, someErr)` where `someErr` is neither sentinel, and the tool returns `ErrInternalError` ("could not verify https://github.com/tstapler/stapler-squad/pull/272 against GitHub — retry: <err>") — the literal `report_pr_created` wording template (`tools_backlog.go:707-715`).
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.2.2a: Implement the dispatcher (~5 min)
- New method on `backlogHandlers`: `func (h *backlogHandlers) verifyGitHubRefExists(ctx context.Context, ref *githubpkg.ParsedGitHubRef) error` — returns `nil` on confirmed existence, `ErrGitHubRefNotFound`/`ErrGitHubAccessDenied` (wrapped, via `errors.Is`-compatible passthrough) or a plain transient error otherwise. `switch ref.Type { case githubpkg.RefTypePR: _, err := githubpkg.GetPR(ctx, ref.Owner, ref.Repo, ref.PRNumber); return err; case githubpkg.RefTypeIssue: _, err := githubpkg.GetIssue(ctx, ref.Owner, ref.Repo, ref.IssueNumber); return err; case githubpkg.RefTypeCommit: _, err := githubpkg.GetCommit(ctx, ref.Owner, ref.Repo, ref.CommitSHA); return err }` (the `default` case is unreachable given Task 3.2.1a already restricted `ref.Type` to these 3 — add a defensive `default: return fmt.Errorf("unsupported ref type %s", ref.Type)` anyway for exhaustiveness).
- Files: `server/mcp/tools_backlog.go`

##### Task 3.2.2b: Call the dispatcher and split the two channels (~4 min)
- `if verifyErr := h.verifyGitHubRefExists(ctx, ref); verifyErr != nil { if errors.Is(verifyErr, githubpkg.ErrGitHubRefNotFound) { return errResult(ErrInvalidArgument, fmt.Sprintf("%s does not exist on GitHub (404) — double-check the URL. Note: a private/inaccessible repo also returns 404.", duplicateRef), ""), nil }; if errors.Is(verifyErr, githubpkg.ErrGitHubAccessDenied) { return errResult(ErrInvalidArgument, fmt.Sprintf("GitHub denied access verifying %s — this session's GitHub credentials may not have access to that repo; retrying will not help unless credentials change.", duplicateRef), ""), nil }; return errResult(ErrInternalError, fmt.Sprintf("could not verify %s against GitHub — retry: %v", duplicateRef, verifyErr), ""), nil }`.
- Files: `server/mcp/tools_backlog.go`

---

### Epic 3.3: Transition + audit trail (FR3, FR5, FR7)
**Goal**: CAS transition to `review` with a human-legible `Note`, `VerificationNotes` persistence, `ErrPreconditionFailed` distinction, and FR5's active-session-aware success message.

#### Story 3.3.1: CAS transition with populated `Note`
**As** `report_duplicate`, **I want** the status-transition call to carry a human-legible `Note`, **so that** a human watching `WorkflowHistorySection.tsx` sees *why* the item moved to review without opening logs (ux.md §4).
**Acceptance Criteria** (FR3, FR7 note-population half):
- *Given* the FR3 success scenario (item `da58b867-...` at `in_progress`, verified `duplicate_ref="https://github.com/tstapler/stapler-squad/pull/272"`, `reason="fc63d55b superseded by PR #272, same fix already merged"`), *When* the transition succeeds, *Then* the resulting `BacklogStatusEvent` row has `TriggeredBy: "agent"` and `Note: "duplicate of https://github.com/tstapler/stapler-squad/pull/272: fc63d55b superseded by PR #272, same fix already merged"`.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.3.1a: Build the precondition + call the transition (~5 min)
- `precondition := &session.BacklogItemPrecondition{ExpectedStatus: item.Status, Note: fmt.Sprintf("duplicate of %s: %s", duplicateRef, reason)}` (using the whitelist-validated `item.Status` from Story 3.1.2, same two-step pattern as Task 2.1.1b). `if _, transErr := h.storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusReview, precondition, session.TriggeredByAgent); transErr != nil { ... }`.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.3.1b: Distinguish `ErrPreconditionFailed` (~3 min)
- Same two-branch pattern as Task 2.1.2a: `errors.Is(transErr, session.ErrPreconditionFailed)` → "item state changed since your last read (another action already resolved it) — call get_backlog_item to see its current status"; else → generic `ErrInternalError` with the underlying error.
- Files: `server/mcp/tools_backlog.go`

#### Story 3.3.2: Persist `duplicate_ref`/`reason` into `VerificationNotes`
**As** the next reviewer (human or AI), **I want** `duplicate_ref`/`reason` surfaced in the review-gate prompt, **so that** the reviewer knows to check the cited ref rather than re-reviewing the diff from scratch.
**Acceptance Criteria** (FR7 note-population other half):
- *Given* the same success scenario, *When* the transition succeeds, *Then* `itemSession.VerificationNotes` is updated to a string containing both `duplicate_ref=https://github.com/tstapler/stapler-squad/pull/272` and the reason text (the idempotency marker format from Task 3.1.2b), via the same `UpdateItemSessionVerificationNotes(ctx, itemSession.ID, notes)` call `request_review` already uses.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.3.2a: Format and persist (~3 min)
- `notes := fmt.Sprintf("duplicate_ref=%s reason=%s", duplicateRef, reason)`; `if updateErr := h.storage.UpdateItemSessionVerificationNotes(ctx, itemSession.ID, notes); updateErr != nil { log.WarningLog.Printf("[mcp:report_duplicate] failed to persist verification notes session=%s item=%s: %v", callerUUID, itemID, updateErr) }` — best-effort, matches `request_review:423-427`'s contract (a notes-write failure never fails the already-succeeded transition).
- Files: `server/mcp/tools_backlog.go`

#### Story 3.3.3: FR5 success messaging + review-gate trigger
**As** the calling work session, **I want** an accurate success message about *when* the duplicate evidence will be seen, **so that** I don't assume a live reviewer will act on it immediately if one isn't watching yet.
**Acceptance Criteria** (FR5):
- *Given* item `da58b867-...` at `in_progress` with **no** active review-role session, *When* `report_duplicate` succeeds, *Then* the success text reads like `"Item da58b867-... routed to review as a duplicate of https://github.com/tstapler/stapler-squad/pull/272. Reviewer notified."` and `h.reviewTrigger.TriggerReviewForSession(callerUUID)` is called (mirrors `request_review`'s unconditional trigger call, `tools_backlog.go:440-442`).
- *Given* the same item but **with** an active review-role session already present (edge case per ADR-008/UQ-1 — e.g. a stale row from a prior rework cycle not yet cleaned up), *When* `report_duplicate` succeeds, *Then* the success text instead reads like `"Item da58b867-... routed to review as a duplicate of https://github.com/tstapler/stapler-squad/pull/272. This will be picked up on the next review pass (a review session is already running and won't see this update live)."` — never implying the *current* reviewer will see it, per `BuildReviewPrompt`'s once-at-spawn-time read confirmed in architecture.md §4.
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.3.3a: Reuse `hasActiveReviewSession` for message branching (~4 min)
- After the transition + notes persistence, call `itemSessions, _ := h.storage.ListItemSessions(ctx, itemID)` and `hasActiveReviewSession(itemSessions)` (the Task 2.2.1a helper — same function, no duplication) to pick between the two message templates above.
- Files: `server/mcp/tools_backlog.go`

##### Task 3.3.3b: Trigger the review gate unconditionally, log, return (~3 min)
- `if h.reviewTrigger != nil { h.reviewTrigger.TriggerReviewForSession(callerUUID) }` — called unconditionally (matching `request_review`'s existing unconditional call; `TriggerReviewForSession`'s own internal idempotency, not this handler's concern, is what makes this safe when a review session is already active — consistent with how the generalized `request_review`'s own `pr_pending` path, once FR2's guard lets a call through in the normal no-active-reviewer case, also calls this unconditionally). Log via `log.InfoLog.Printf("[mcp:report_duplicate] session=%s item=%s duplicate_ref=%s transitioned to review", callerUUID, itemID, duplicateRef)`. Return the branched success text from Task 3.3.3a.
- Files: `server/mcp/tools_backlog.go`

---

### Epic 3.4: MCP registration (FR10)
**Goal**: Register `report_duplicate` in `registerBacklogTools`, with a tool description matching house style (ux.md §2) and explicit FR10 retry guidance.

#### Story 3.4.1: `registerBacklogTools` entry
**As** an MCP client, **I want** `report_duplicate` discoverable with a description that states role, preconditions, verification behavior, and retry guidance, **so that** an agent calling it knows what to expect without trial and error.
**Acceptance Criteria** (FR10):
- *Given* the MCP server is running, *When* a client lists tools, *Then* `report_duplicate` appears with `item_id` (required, UUID), `duplicate_ref` (required string), `reason` (required string) parameters, and a description containing the literal word "retry" in the context of `INTERNAL_ERROR` results (e.g. "If verifying duplicate_ref against GitHub fails with INTERNAL_ERROR, this is transient — retry the call with the same arguments.").
**Files**: `server/mcp/tools_backlog.go`

##### Task 3.4.1a: Add the `s.AddTool(...)` block (~5 min)
- In `registerBacklogTools`, immediately after the `report_pr_created` block (ends `tools_backlog.go:1037`), add a `report_duplicate` registration following `report_pr_created`'s exact shape (`mcpgo.NewTool("report_duplicate", mcpgo.WithDescription(...), mcpgo.WithString("item_id", ...), mcpgo.WithString("duplicate_ref", ...), mcpgo.WithString("reason", ...))`, `h.reportDuplicate`). Description content, house-style-ordered (ux.md §2): (1) one-line summary — "Report that this item's work is a duplicate of an already-existing PR/issue/commit, routing the item to review instead of continuing it."; (2) `"Role: work only."`; (3) preconditions — "Refuses if the item has SkipReviewGate enabled (use request_review instead), if the item isn't at in_progress or pr_pending, or if duplicate_ref cannot be verified to exist on GitHub — verification happens BEFORE any state change."; (4) consequence — "On success, transitions the item to 'review' status (never done/archived directly) so a human/reviewer confirms the duplicate before closing it out."; (5) idempotency — "Calling this again with the same duplicate_ref after it already succeeded is safe (no-op)."; (6) FR10 retry guidance — the literal sentence from the acceptance criterion above.
- Files: `server/mcp/tools_backlog.go`

---

## Phase 4: Tests

### Epic 4.1: `request_review` — regression + new coverage (FR9, AC9)

#### Story 4.1.1: Confirm the 5 existing tests pass unmodified
**Acceptance Criteria** (FR9):
- *Given* the 5 existing tests (`TestRequestReview_TransitionsItemToReview`, `_TransitionsDirectlyToDone_When_SkipReviewGateEnabled`, `_PersistsVerificationNotesOnWorkSession`, `_RejectsVerificationNotesOver4000Chars`, `_RejectsWhenSessionNotLinked` — `server/mcp/tools_backlog_test.go:544,592,636,676,721`), *When* `go test ./server/mcp/... -run TestRequestReview` is run after Phase 2 lands, *Then* all 5 pass with zero code changes to the test file for these 5 functions — each seeds `Status: string(session.BacklogStatusInProgress)` and never mutates status before calling, so `item.Status` observed inside the handler is always `"in_progress"`, identical to the value the old hardcoded constant supplied.
**Files**: `server/mcp/tools_backlog_test.go` (verification only, no edits to these 5 functions)

##### Task 4.1.1a: Run the existing suite after Phase 2 (~2 min)
- `go build ./... && go test ./server/mcp/... -run TestRequestReview -v`. Confirm all 5 pass. If any fails, treat as a Phase 2 regression to fix before proceeding — not a test to edit.
- Files: none (verification task)

#### Story 4.1.2: New test — `pr_pending`-sourced success (FR1/FR2's happy path)
**Acceptance Criteria** (FR1):
- *Given* an item created at `Status: string(session.BacklogStatusPRPending)` with a linked work-role `ItemSession`, *When* `requestReview` is called (no active review session present), *Then* the result text contains "review" and a follow-up `GetBacklogItem` shows `Status: "review"`.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.1.2a: `TestRequestReview_TransitionsPRPendingItemToReview` (~5 min)
- New test function, same helper pattern as existing tests (inline `storage.CreateBacklogItem`/`CreateItemSession`, `makeToolReq`, `WithSessionUUID`) but seed `Status: string(session.BacklogStatusPRPending)`. Assert success + `Status == "review"` afterward.
- Files: `server/mcp/tools_backlog_test.go`

#### Story 4.1.3: New test — whitelist rejection from a disallowed status
**Acceptance Criteria** (pitfalls.md §0's explicit new-test requirement):
- *Given* an item created at `Status: string(session.BacklogStatusDone)`, *When* `requestReview` is called, *Then* the result is an `ErrInvalidArgument` error mentioning `"done"`, and a follow-up `GetBacklogItem` still shows `Status: "done"` (no mutation).
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.1.3a: `TestRequestReview_RejectsWhenSourceStatusNotAllowed` (~5 min)
- Table-driven over `{"done", "idea", "review", "archived"}` — for each, seed the item at that status, call `requestReview`, assert `ErrInvalidArgument` + status unchanged after.
- Files: `server/mcp/tools_backlog_test.go`

#### Story 4.1.4: New test — FR2 active-reviewer refusal
**Acceptance Criteria** (FR2):
- *Given* an item at `pr_pending` with a linked work session AND a second, active (unended) review-role `ItemSession` for the same item, *When* `requestReview` is called by the work session, *Then* it returns `ErrInvalidArgument` mentioning "active review session", and status remains `pr_pending`.
- *Given* the same active-review-session setup but the item is at `in_progress` instead, *When* `requestReview` is called, *Then* it succeeds (transitions to `review`) — confirming the guard is `pr_pending`-scoped only.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.1.4a: `TestRequestReview_RejectsWhenActiveReviewSessionExists_AndSourceIsPRPending` (~5 min)
- Seed item at `pr_pending`, create a work `ItemSession` (caller) and a second review-role `ItemSession` with `EndedAt: nil`. Call `requestReview` as the work session. Assert refusal.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 4.1.4b: `TestRequestReview_AllowsActiveReviewSession_WhenSourceIsInProgress` (~4 min)
- Same setup but item at `in_progress`. Assert success.
- Files: `server/mcp/tools_backlog_test.go`

#### Story 4.1.5: New test — CAS-race-loser gets the distinct message
**Acceptance Criteria** (pitfalls.md §1/§5c):
- *Given* an item at `in_progress`, *When* the item's status is mutated out from under the call between load and transition (simulate by transitioning the item to a different status via a second `TransitionBacklogItemStatus` call between the handler's `GetBacklogItem` and its own transition — or, more simply, seed a precondition mismatch directly by calling `TransitionBacklogItemStatus` with a stale expected status), *Then* the error message contains "state changed" (not the generic "transition to %s failed" text), and the error code is `ErrInternalError`.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.1.5a: `TestRequestReview_ReportsDistinctMessage_WhenCASPreconditionFails` (~5 min)
- Seed item at `in_progress`. Before calling `requestReview`, directly call `storage.TransitionBacklogItemStatus(ctx, itemID, session.BacklogStatusReview, &session.BacklogItemPrecondition{ExpectedStatus: "in_progress"}, session.TriggeredByUser)` to move it out from under the handler's soon-to-be-stale read... — simplest construction: seed the item already at `review` directly via `CreateBacklogItem`, but bypass the whitelist check by instead directly invoking `storage.TransitionBacklogItemStatus` with an intentionally-wrong `ExpectedStatus` to force `ErrPreconditionFailed` deterministically without needing true concurrency; assert the returned error's message contains "state changed" via a light wrapper test that calls the transition-error-branch logic in isolation if the whitelist makes triggering this from the public `requestReview` entrypoint awkward — prefer testing via `requestReview` if a clean setup exists (e.g. seed `in_progress`, then have a background goroutine race a second write) but fall back to directly unit-testing the branch logic if not.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 4.2: `report_duplicate` tests (FR3–FR8, FR10)

#### Story 4.2.1: Success paths (PR ref from `in_progress`, issue ref from `pr_pending`)
**Acceptance Criteria** (FR3):
- *Given* item at `in_progress`, valid PR `duplicate_ref`, verified via a mocked `GetPR` returning success, *When* `reportDuplicate` is called, *Then* item transitions to `review`, `VerificationNotes` contains the ref, `BacklogStatusEvent.Note` is populated, `TriggeredBy == "agent"`.
- *Given* item at `pr_pending`, valid issue `duplicate_ref`, *When* `reportDuplicate` is called, *Then* same outcome.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.2.1a: Test harness — inject a fake GitHub verifier (~5 min)
- Since `verifyGitHubRefExists` calls package-level `github.GetPR`/`GetIssue`/`GetCommit` functions directly (not interface-injected, per the Pattern Decision rejecting DI — Option C), tests need either (a) an `httptest.Server` the `github` package's HTTP client points at (reuse the injection pattern found in Task 1.2.3b), or (b) if `backlogHandlers` already has a seam for this (check `h.verifyPR` in `reportPRCreated` — it's a method value field, confirm whether `backlogHandlers` has a swappable `verifyPR func(...)` field already used for `report_pr_created`'s own tests). Read `tools_backlog_test.go`'s existing `report_pr_created` tests (if any) for the established mocking pattern before deciding; if none exists, add the `httptest.Server` approach to the `github` package's existing test-injection point (found in Task 1.2.3b) and point it from these MCP-layer tests too.
- Files: `server/mcp/tools_backlog_test.go` (or a shared test helper file if the mocking seam needs one)

##### Task 4.2.1b: `TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedPR` (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 4.2.1c: `TestReportDuplicate_TransitionsPRPendingItemToReview_WithVerifiedIssue` (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

##### Task 4.2.1d: `TestReportDuplicate_TransitionsInProgressItemToReview_WithVerifiedCommit` (~5 min)
- Covers the commit-ref path specifically (distinct from PR/issue, exercises `GetCommit`).
- Files: `server/mcp/tools_backlog_test.go`

#### Story 4.2.2: FR6 refusal paths
**Acceptance Criteria** (FR6): see Stories 3.1.1/3.1.2's GWTs verbatim — SkipReviewGate refusal, non-work-role refusal, session-not-linked refusal, disallowed-source-status refusal, all zero-mutation.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.2.2a: `TestReportDuplicate_RejectsWhenSkipReviewGateEnabled` (~4 min)
##### Task 4.2.2b: `TestReportDuplicate_RejectsWhenSessionRoleNotWork` (~4 min)
##### Task 4.2.2c: `TestReportDuplicate_RejectsWhenSessionNotLinked` (~4 min)
##### Task 4.2.2d: `TestReportDuplicate_RejectsWhenSourceStatusNotAllowed` (~4 min)
- Table-driven over `{"done", "idea", "review", "archived"}`, same shape as Task 4.1.3a.
- Files (all 4 tasks): `server/mcp/tools_backlog_test.go`

#### Story 4.2.3: FR4 two-channel error tests
**Acceptance Criteria** (FR4): see Story 3.2.2's GWTs — 404 → `ErrInvalidArgument`, network/transient → `ErrInternalError` with "retry" in the message.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.2.3a: `TestReportDuplicate_RejectsWhenGitHubRefNotFound` (~4 min)
##### Task 4.2.3b: `TestReportDuplicate_ReturnsRetryableError_WhenGitHubVerificationTimesOut` (~4 min)
##### Task 4.2.3c: `TestReportDuplicate_ReturnsRetryableError_WhenGitHubRateLimited` (~4 min)
##### Task 4.2.3d: `TestReportDuplicate_RejectsWhenGitHubAccessDenied` (~4 min)
- Bare 403 (no `Retry-After`, no `X-RateLimit-Remaining: 0`) → `ErrInvalidArgument`, per ADR-005's UQ-2 resolution.
- Files (all 4 tasks): `server/mcp/tools_backlog_test.go`

#### Story 4.2.4: Idempotency tests
**Acceptance Criteria**: see Story 3.1.2's GWTs — exact-retry no-op, different-ref-after-resolved rejection.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.2.4a: `TestReportDuplicate_NoOpOnExactRetry` (~4 min)
##### Task 4.2.4b: `TestReportDuplicate_RejectsDifferentRefAfterAlreadyResolved` (~4 min)
- Files (both): `server/mcp/tools_backlog_test.go`

#### Story 4.2.5: FR5 messaging test
**Acceptance Criteria**: see Story 3.3.3's GWTs.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.2.5a: `TestReportDuplicate_MessageSaysNextReviewPass_WhenReviewSessionActive` (~5 min)
- Files: `server/mcp/tools_backlog_test.go`

#### Story 4.2.6: Concurrency regression test (pitfalls.md §5c)
**Acceptance Criteria**: `report_duplicate` racing `report_pr_created` on the same item results in exactly one status-event row and a non-retryable message for the loser.
**Files**: `server/mcp/tools_backlog_test.go`

##### Task 4.2.6a: `TestReportDuplicate_LoserGetsDistinctMessage_WhenRacingReportPRCreated` (~5 min)
- Seed item at `in_progress`. Sequentially (not truly concurrently, to keep the test deterministic): call `reportPRCreated` first to move the item to `pr_pending`, then call `reportDuplicate` with a stale `item.Status` assumption is awkward via the public entrypoint — simplest deterministic construction: call `reportDuplicate` (succeeds, item → `review`), then directly call `storage.TransitionBacklogItemStatus` a second time with the *original* precondition to confirm it now fails with `ErrPreconditionFailed`, and assert exactly one `BacklogStatusEvent` row exists for the item via `ListBacklogStatusEvents` (or equivalent) before vs. after.
- Files: `server/mcp/tools_backlog_test.go`

---

### Epic 4.3: `github` package unit tests
(Covered by Tasks 1.2.3b/1.2.3c above — listed here only for narrative completeness; no additional tasks.)

---

## Final Verification Task (all phases)

##### Task 5.0a: Full build/test/lint gate (~5 min)
- `go build ./... && make test && make lint` (or `make quick-check`) — confirms FR8 (`ent generate` never invoked, build succeeds), FR9 (existing suite green), and all new tests pass. `grep -rn "ent generate" .git/COMMIT_EDITMSG` is not a real check — the actual proof is `git status session/ent/` showing zero diff after the full build/test cycle.
- Files: none (verification task)
