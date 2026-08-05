# Implementation Plan: report_pr_created cannot link a PR opened on a fallback branch

Project: `report-pr-created-branch-mismatch`. Source requirements:
`project_plans/report-pr-created-branch-mismatch/requirements.md`. Research:
`research/architecture.md`, `research/pitfalls.md`, `research/build-vs-buy.md`,
`research/features.md`, `research/ux.md` (same directory).

## Step 0.5 — Creative pass: alternatives considered

**Approach A — Ancestry/compare-API fallback** (`architecture.md`'s original
recommendation: exact-branch fast path, then GitHub `compare` endpoint as a
fallback to prove the PR's head descends from the tracked branch).
*Strength:* zero new MCP-visible surface, purely internal to
`VerifyPRMatchesBranch`. *Weakness:* mathematically fails on the bug's actual
scenario — `pitfalls.md` §1 shows a deliberately history-severed clean branch
(the recommended recovery move: cut fresh from `origin/main` specifically to
shed a polluted branch's history) shares no discriminable commit ancestry with
the tracked branch, so `compare` reports `diverged`/no-common-ancestor exactly
as often for a legitimate recovery PR as for a totally unrelated one. It does
not solve the reported bug. **Rejected.**

**Approach B — Separate manual-override MCP tool** (`architecture.md`'s option
b / requirements' AC2 read literally: a new `link_pr_manually`/`relink_backlog_pr`
tool, gated by a stronger "operator" check). *Strength:* keeps
`report_pr_created`'s existing strict contract 100% untouched; a clean
audit-distinct escape hatch. *Weakness:* `pitfalls.md` §2 shows this
codebase has no operator/human auth primitive at all — `session.SessionRole`
is `work`/`triage`/`review`, all three LLM-driven. A new tool gated on
`role=work` is callable by the exact same session that made the original
mistake, so it buys no real additional safety over relaxing the existing
tool, while costing a new tool registration, a `docs/registry/` entry per
`.claude/rules/feature-registry.md`, a new hint-text block to keep in sync
with `report_pr_created`, and a second doc-comment to maintain. **Rejected**
as disproportionate to "a small, contained bug fix" (requirements.md).

**Approach C — Root-cause fix (number-keyed lookup) + same-tool relaxed
check gated by a mandatory `override_reason` argument** (chosen). Fixes the
actual root cause `build-vs-buy.md` identifies (branch-name-keyed GitHub
lookup silently returns "no PR" once the real head branch diverges) by
looking the PR up by number instead, which for free also gives the real head
branch for a fast, unchanged exact-match path. When it doesn't match, the
same tool accepts the PR only if it verifiably exists, belongs to the correct
owner/repo, and is open or merged — a real GitHub-verified check — *plus* a
caller-supplied `override_reason` string, logged loudly, as the procedural
deterrent `pitfalls.md` calls for in place of a technical human-gate that
doesn't exist in this codebase. *Strength:* smallest diff (one seam, no new
tool), actually fixes the reported scenario (no ancestry math required),
and — as a bonus — uses *fewer* GitHub calls per attempt than today's code
(see Observability Plan). *Weakness:* the residual protection against a
determined/hallucinating same-role session is procedural (a logged reason),
not technical — documented explicitly in `decisions/ADR-001-...md` rather
than overstated.

**Chosen: Approach C.**

## Step 1 — System type

This is a validation-logic bug fix inside one existing MCP tool handler
(`reportPRCreated`, Transaction Script) plus one new low-level data-fetch
function (`GetPRByNumber`). No domain model, no new persistence, no new
service layer. Patterns are chosen accordingly in Step 3 — most components
need none.

## Step 2 — Domain Glossary

| Term | Definition |
|---|---|
| `PRVerification` | New struct (`server/mcp/tools_github.go`) — the factual result of checking a self-reported PR against GitHub for `report_pr_created`: does it exist, does its head branch match, what is its real head branch and state. |
| `PRVerification.Exists` | `bool` — whether `prNumber` corresponds to a real pull request in `owner/repo` at all. `false` is always a definitive rejection, override or not. |
| `PRVerification.Matched` | `bool` — whether the PR's real GitHub head branch exactly equals the item's tracked branch (the fast path, unchanged pre-fix behavior). |
| `PRVerification.ActualHeadBranch` | `string` — the PR's real head branch as reported by GitHub; empty when `Exists` is `false`. |
| `PRVerification.State` | `string` — normalized PR state: `"open"`, `"closed"`, or `"merged"`. |
| `GetPRByNumber` | New function (`github/client.go`) — REST (no `gh` subprocess) lookup of a single PR by number; returns `github.ErrNoPR` (typed) when the number doesn't exist. The root-cause fix. |
| `VerifyPRMatchesBranch` | Existing function (`server/mcp/tools_github.go`), signature unchanged, return type changes from `(bool, error)` to `(PRVerification, error)`. Now calls `GetPRByNumber` instead of the branch-keyed `GetPRForBranch`. |
| `tracked branch` | The backlog item's own git branch (`backlog/<item-slug>`), resolved via `h.sessionBranch` → `GetWorktreeDataBySessionUUID`. |
| `fallback branch` | A differently-named branch (e.g. `feature/<slug>`) a work session opens a PR from when the tracked branch is polluted by another session's commits. |
| `fast path` | `verification.Matched == true` — behaves exactly as before this fix; no `override_reason` needed. |
| `fallback path` / `override path` | `verification.Exists == true && verification.Matched == false` — requires a non-empty `override_reason` argument and `verification.State` in `{open, merged}`. |
| `override_reason` | New optional MCP tool argument on `report_pr_created`. Required only on the fallback path; the caller's one-sentence explanation for why the PR's head branch differs, persisted only in the server log (not on the backlog item) as an audit trail. |
| `backlogHandlers.verifyPR` | Existing seam-wrapper method (`tools_backlog.go`); return type changes to match `VerifyPRMatchesBranch`. |
| `backlogHandlers.verifyPRMatchesBranch` | Existing overridable struct field (test seam); function type changes to return `PRVerification`. |
| `ErrNoPR` | Existing typed sentinel (`github` package), reused by `GetPRByNumber` for its 404 case — the same "definitive, not string-sniffed" contract `GetPRForBranch` already established. |

## Step 3 — Pattern Decisions

No new dependency. No stability/licensing/security concerns — everything
reuses primitives already in the repo (`ghHTTPClient`, `newGHRequest`,
`GhBaseURL` test-override var, the `backlogHandlers` function-field seam
pattern).

| Component | Pattern Chosen | Alternative Rejected | Reason |
|---|---|---|---|
| Overall fix shape | Root-cause number-keyed lookup + same-tool relaxed check with mandatory `override_reason` (Transaction Script — logic lives inline in `reportPRCreated`, no new domain type) | Ancestry/compare-API fallback (`architecture.md`'s recommendation) | `pitfalls.md` §1: mathematically indistinguishable from an unrelated PR once the recovery branch's history is deliberately severed from the tracked branch — doesn't solve the reported bug. |
| Overall fix shape (cont.) | (same as above) | Separate manual-override MCP tool (option b / AC2 read literally) | `pitfalls.md` §2: no operator/human role exists in this codebase (`work`/`triage`/`review` only); a second tool is exactly as callable by the same session as relaxing the existing one, for real added surface (new tool registration + feature-registry entry + hint-text sync). |
| GitHub data source for verification | New `GetPRByNumber` — REST, no subprocess (`github/client.go`), mirrors `GetPRForBranch`'s existing REST style | Reuse `GetPRInfoCtx` (`gh pr view` subprocess) + extend `--json` fields per `build-vs-buy.md`'s suggestion | A nonexistent PR number produces an untyped `gh` CLI stderr string from `GetPRInfoCtx`, forcing string-sniffing — exactly the anti-pattern `VerifyPRMatchesBranch`'s own doc comment (`tools_github.go:259-262`) says `GetPRForBranch` was built to avoid. REST gives a clean `404` → typed `ErrNoPR`. |
| Verification return shape | New `PRVerification` struct (4 fields) | Keep `(bool, error)` | A 2-value contract can't express the 3 distinct outcomes the fallback policy must branch on (matched / real-PR-different-branch / no-PR-at-all) — `pitfalls.md` §3 flags this exact gap ("no slot for 'one signal says no, the other is unknown'"). Struct is a plain data holder, not a speculative interface — no interface introduced, just a value type (type-driven-design, not GoF). |
| Where the override policy lives | Inside `reportPRCreated` (`tools_backlog.go`) — `VerifyPRMatchesBranch` stays a pure fact-reporter | Push the override/state-gate logic into `VerifyPRMatchesBranch` itself | Keeps "what does GitHub say" (general-purpose, unit-testable via `GhBaseURL` httptest override) separate from "should we trust it for this item" (caller-specific policy: item status, `override_reason` arg). Matches the Transaction Script framing from Step 1 — no Strategy/Chain-of-Responsibility needed for two `if` branches. |
| Test/override mechanism | Reuse the existing `backlogHandlers.verifyPRMatchesBranch` function-field seam, only its return type changes | Add a second seam/field for the fallback path | Requirements explicitly forbid a second competing seam mechanism; the existing seam's shape already carries everything the fallback path needs once its return type is a struct instead of a bool. |

## Step 4 — Tasks

Each task lists exact files and is sized 2–5 minutes.

### Story 1 — Root-cause fix: look up the PR by number, not by branch name

**Task 1.1** — Add `GetPRByNumber` to `github/client.go` (new function, placed
after `GetPRForBranch`, ~line 428). REST `GET repos/{owner}/{repo}/pulls/{number}`
via the existing `newGHRequest`/`ghHTTPClient` pair (no `gh` subprocess).
Returns `github.ErrNoPR` on `404`. Parses `number`, `head.ref`, `base.ref`,
`state`, `merged`, `html_url` into a `*PRInfo` (no new `PRInfo` fields needed —
`Number`/`HeadRef`/`BaseRef`/`State`/`HTMLURL` already exist); normalizes
`State` to `"merged"` when the response's `merged` boolean is `true`,
otherwise passes through GitHub's `"open"`/`"closed"`.
Files: `github/client.go`.

**Task 1.2** — New test file `github/client_pr_by_number_test.go` (package
`github`, no import needed for `GhBaseURL` — same package). Cases, each an
`httptest.Server` responding with a fixed JSON body/status:
- `TestGetPRByNumber_should_ReturnPRInfo_When_PRExists` — 200 with
  `head.ref="feature/ci-status-diff-viewer"`, `base.ref="main"`,
  `state="closed"`, `merged=true` → asserts `State == "merged"`.
- `TestGetPRByNumber_should_ReturnErrNoPR_When_PRDoesNotExist` — 404 →
  `errors.Is(err, github.ErrNoPR)`.
- `TestGetPRByNumber_should_ReturnError_When_Forbidden` — 403 → non-nil,
  non-`ErrNoPR` error (transient-shaped).
Files: `github/client_pr_by_number_test.go`.

### Story 2 — `PRVerification` type + rewritten `VerifyPRMatchesBranch`

**Task 2.1** — In `server/mcp/tools_github.go`: add the `PRVerification`
struct (see Domain Glossary) directly above `VerifyPRMatchesBranch`
(~line 249). Rewrite `VerifyPRMatchesBranch`'s body to call
`githubpkg.GetPRByNumber(ctx, owner, repo, prNumber)` instead of
`GetPRForBranch(ctx, owner, repo, expectedBranch)`; on `ErrNoPR` return
`PRVerification{Exists: false}, nil`; on other error return
`PRVerification{}, err`; on success return
`PRVerification{Exists: true, Matched: info.HeadRef == expectedBranch, ActualHeadBranch: info.HeadRef, State: info.State}, nil`.
Rewrite the doc comment: explain the root-cause fix (number-keyed, not
branch-keyed) and that `Matched == false` with `Exists == true` is now a
*possible-to-accept* fallback case, policy decided by the caller
(`reportPRCreated`), not by this function.
Files: `server/mcp/tools_github.go`.

**Task 2.2** — In `server/mcp/tools_backlog.go`: change the
`verifyPRMatchesBranch` field type (struct doc block, ~line 99-102) from
`func(...) (bool, error)` to `func(...) (PRVerification, error)`; update
`h.verifyPR`'s signature and body (~line 605-610) to match, calling the
renamed return type through unchanged.
Files: `server/mcp/tools_backlog.go`.

**Task 2.3** — In `server/mcp/tools_backlog_test.go`: update the 5 existing
`verifyPRMatchesBranch:` field literals to the new signature, preserving each
test's existing intent as a `PRVerification` value:
- `TestReportPRCreated_should_TransitionToPRPending_When_ValidPR` (~line 812):
  `func(...) (PRVerification, error) { return PRVerification{Exists: true, Matched: true, ActualHeadBranch: "backlog/ship-it", State: "open"}, nil }`.
- `TestReportPRCreated_should_ReturnError_When_PersistFails` (~line 864): same
  shape as above.
- `TestReportPRCreated_should_NoOp_When_AlreadyPRPendingSamePR` (~line 911):
  same shape (never actually invoked — idempotency short-circuits first — but
  must compile).
- `TestReportPRCreated_should_RejectCall_When_BranchMismatch` (~line 985):
  `PRVerification{Exists: true, Matched: false, ActualHeadBranch: "totally-unrelated-branch", State: "open"}, nil` —
  and since the request in this test does **not** set `override_reason`, the
  assertions (still `ErrInvalidArgument`, item untouched) continue to hold
  unchanged, preserving this test's regression intent per requirements.md.
- `TestReportPRCreated_should_ReturnRetryableError_When_GitHubLookupTransientlyFails`
  (~line 1026): `func(...) (PRVerification, error) { return PRVerification{}, fmt.Errorf("GitHub API: rate limited (403)") }`.
Files: `server/mcp/tools_backlog_test.go`.

### Story 3 — Same-tool fallback path (resolves AC1 + AC2)

**Task 3.1** — In `server/mcp/tools_backlog.go`'s `reportPRCreated`
(~line 707-715), replace the single `!matched` reject with:
1. Parse optional `overrideReason, _ := args["override_reason"].(string)`;
   trim whitespace; if non-empty and `len(overrideReason) > 500`, return
   `ErrInvalidArgument` ("override_reason must be <= 500 characters").
2. Call `verification, verifyErr := h.verifyPR(...)`; unchanged transient-error
   handling (`verifyErr != nil` → `ErrInternalError`, "retry").
3. `if !verification.Exists` → `ErrInvalidArgument`: `"PR #%d does not exist in %s/%s on GitHub — refusing to record it. Double-check the PR number/URL."` (existence can never be overridden).
4. `else if !verification.Matched`:
   - `overrideReason == ""` → `ErrInvalidArgument` with the AC3 message (see
     Task 3.1's exact string in the Given-When-Then for AC3 below).
   - `overrideReason != ""` but `verification.State` not in `{"open","merged"}`
     → `ErrInvalidArgument`: `"PR #%d is %s (not open or merged) — refusing to record it even with override_reason."`
   - otherwise: fall through to the existing `SetBacklogItemPRAndTransition`
     call, and emit the audit log line (Task 3.1 also adds this, see
     Observability Plan): `log.Warn("report_pr_created: recording PR via override (head branch differs from tracked branch)", "session", callerUUID, "item", itemID, "pr_number", prNumber, "actual_head_branch", verification.ActualHeadBranch, "tracked_branch", branch, "override_reason", overrideReason)`.
5. `else` (`verification.Matched == true`) → fast path, unchanged.
Files: `server/mcp/tools_backlog.go`.

**Task 3.2** — In `server/mcp/tools_backlog.go`'s tool registration block
(~line 1013-1037): add
`mcpgo.WithString("override_reason", mcpgo.Description("Only required when the PR's actual head branch (per GitHub) differs from this item's tracked branch — e.g. the tracked branch was polluted by another session sharing this worktree, so you opened the PR from a clean branch instead. Explain why in one sentence; it is recorded in the server log as an audit trail. Omit when the PR's head branch matches the tracked branch."))`
(no `mcpgo.Required()` — optional). Update the tool's top-level
`mcpgo.WithDescription(...)` (~line 1015-1017) to mention the fallback case
exists and is gated by `override_reason`. Update the "work" role hint text
(~line 213) to append one sentence: `"If the PR's head branch differs from your tracked branch (e.g. you had to open it from a clean fallback branch), pass override_reason explaining why — do not just retry report_pr_created unchanged."`
Files: `server/mcp/tools_backlog.go`.

### Story 4 — Tests for the new contract

**Task 4.1** (AC1) — New test
`TestReportPRCreated_should_TransitionToPRPending_When_FallbackBranchWithOverrideReason`
in `server/mcp/tools_backlog_test.go`. Uses the confirmed real repro shape:
item fixture via `setupReportPRCreatedFixture` (status `review`),
`resolveSessionBranch` → `"backlog/stapler-squad-ci-status-diff-viewer"`,
`verifyPRMatchesBranch` → `PRVerification{Exists: true, Matched: false, ActualHeadBranch: "feature/ci-status-diff-viewer", State: "merged"}, nil`.
Request: `pr_url="https://github.com/tstapler/stapler-squad/pull/326"`,
`pr_number=326`, `summary="..."`, `override_reason="tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead"`.
Assert: success result contains `"pr_pending"`; fetched item has
`Status == pr_pending`, `PrNumber == 326`.
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.2** (AC2 — override path is real, not silently bypassed) — New test
`TestReportPRCreated_should_RejectCall_When_FallbackBranchMissingOverrideReason`.
Same fixture as Task 4.1 but `override_reason` omitted from the request.
Assert: `ErrInvalidArgument`; item remains `Status == review`, `PrNumber == 0`
(untouched — proves the relaxed path only ever fires with an explicit,
audited reason, i.e. it behaves as a genuine manual-override mechanism, not
an automatic bypass).
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.3** (AC3 — message documents the workaround) — New test
`TestReportPRCreated_should_DocumentOverrideWorkaround_When_BranchMismatchRejected`.
Same fixture/request as Task 4.2. Assert the returned error message contains
both `"override_reason"` and the actual head branch string
(`"feature/ci-status-diff-viewer"`) and the tracked branch string
(`"backlog/stapler-squad-ci-status-diff-viewer"`) — i.e. the caller is told
concretely what to retry with, not just that it failed.
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.4** (definitive-mismatch-still-rejected, unrelated PR) — New test
`TestReportPRCreated_should_RejectCall_When_UnrelatedPRWithOverrideReason`.
Fixture: `verifyPRMatchesBranch` → `PRVerification{Exists: true, Matched: false, ActualHeadBranch: "totally-unrelated-branch", State: "closed"}, nil`
(a real PR, but closed and not merged). Request includes an `override_reason`.
Assert: still `ErrInvalidArgument` (PR state gate rejects it even with a
reason); item untouched. This is the sibling required by requirements.md's
constraint that "a PR that has no relationship whatsoever to the item's work
must still be rejected."
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.5** (nonexistent PR number cannot be overridden) — New test
`TestReportPRCreated_should_RejectCall_When_PRNumberDoesNotExist`. Fixture:
`verifyPRMatchesBranch` → `PRVerification{Exists: false}, nil`. Request
includes an `override_reason`. Assert: `ErrInvalidArgument` mentioning
"does not exist"; item untouched — proves existence can never be overridden,
only the branch-match requirement can.
Files: `server/mcp/tools_backlog_test.go`.

**Task 4.6** — Run `go build ./... && go test ./server/mcp/... ./github/...`
locally to confirm Tasks 1.1–4.5 compile and pass together (the seam-signature
change in Task 2.2/2.3 must land atomically with 1.1/2.1 or the package won't
build — call this out to whichever session/task-runner executes this plan).
Files: none (verification task).

### Story 5 — Note the reconciler's shared blind spot (no functional change)

**Task 5.1** — In `session/backlog_lifecycle.go`, immediately above line 2656
(`info, prErr := l.getOrphanedPRFinder()(ctx, item.RepoPath, wt.BranchName)`
inside `reconcileOrphanedAgentPRs`), add a one-line comment:
`// NOTE: this still looks up by branch name (github.GetPRForBranch via getOrphanedPRFinder), so it has the same blind spot report_pr_created had before the number-keyed fix in tools_github.go's VerifyPRMatchesBranch — a PR opened from a fallback branch is invisible here too. Not fixed here (out of scope per project_plans/report-pr-created-branch-mismatch/requirements.md); a future fast-follow could reuse VerifyPRMatchesBranch/GetPRByNumber's shape.`
This is documentation only — `reconcileOrphanedAgentPRs`'s actual lookup
function is unchanged. Per `research/features.md`, this reconciler has the
identical blind spot and should reuse whatever predicate this fix builds, in
a future fast-follow, not in this plan.
Files: `session/backlog_lifecycle.go`.

## Given-When-Then per Acceptance Criterion

**AC1** (same tool links a fallback-branch PR, GitHub-verified):
- **Given** backlog item `3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d` is `review`
  status with tracked branch `backlog/stapler-squad-ci-status-diff-viewer`,
  and GitHub PR `https://github.com/tstapler/stapler-squad/pull/326` really
  exists with head branch `feature/ci-status-diff-viewer` and state `merged`
  (so `PRVerification{Exists: true, Matched: false, ActualHeadBranch: "feature/ci-status-diff-viewer", State: "merged"}`)
- **When** the linked work session calls `report_pr_created` with
  `pr_url="https://github.com/tstapler/stapler-squad/pull/326"`,
  `pr_number=326`, a `summary`, and
  `override_reason="tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead"`
- **Then** `storage.SetBacklogItemPRAndTransition` is called and the item
  transitions `review` → `pr_pending` with `PrNumber == 326` — previously this
  call hard-rejected with `PR #326 does not match this item's branch "backlog/stapler-squad-ci-status-diff-viewer" on GitHub — refusing to record it.`

**AC2** (manual-override path exists, gated, and audited — resolved as the
same tool + mandatory `override_reason`, not a second tool; see
`decisions/ADR-001-override-reason-security-model.md` for why):
- **Given** the identical state as AC1's Given
- **When** the same call is made with `override_reason` a non-empty reason
- **Then**, in addition to AC1's persisted transition, a `log.Warn` line is
  emitted with `session`, `item=3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d`,
  `pr_number=326`, `actual_head_branch=feature/ci-status-diff-viewer`,
  `tracked_branch=backlog/stapler-squad-ci-status-diff-viewer`, and the
  caller's `override_reason` verbatim — an operator/reviewer can grep this
  line after the fact even though no human gated the call itself.

**AC3** (rejection message documents the workaround):
- **Given** the identical state as AC1's Given, but the caller omits
  `override_reason`
- **When** `report_pr_created` is called with `pr_url=".../pull/326"`,
  `pr_number=326`
- **Then** the call returns `ErrInvalidArgument` with a message naming the
  actual head branch, the tracked branch, and the exact retry shape:
  `PR #326's head branch on GitHub is "feature/ci-status-diff-viewer", not this item's tracked branch "backlog/stapler-squad-ci-status-diff-viewer" — refusing to record it. If "backlog/stapler-squad-ci-status-diff-viewer" was polluted (e.g. by another session sharing this worktree) and you opened this PR from a clean fallback branch instead, retry this exact call with an additional override_reason argument explaining why, e.g. override_reason="tracked branch had unrelated commits from a shared worktree; opened PR from a clean branch instead". If PR #326 is unrelated to this item, do not retry — find and report the correct PR instead.`
  — this both names the fix (`override_reason`) and tells the caller when
  *not* to retry, directly addressing the bug report's "retries the identical
  failing call in a loop" failure mode.

## Observability Plan

- **Unchanged**: the existing single `log.InfoLog.Printf("[mcp:report_pr_created] session=%s item=%s PR #%d %s", ...)` line
  (`tools_backlog.go:721`) still fires on every successful record, fast path
  or fallback path alike.
- **New**: a `log.Warn(...)` structured line (see Task 3.1) fires only when
  the fallback/override path is actually taken and accepted — the
  `pitfalls.md`-identified compensating audit-trail control for a path that
  has no technical human-gate. Fields: `session`, `item`, `pr_number`,
  `actual_head_branch`, `tracked_branch`, `override_reason`.
- **Call-count note (rebuts a `pitfalls.md` §4 concern)**: this fix does
  *not* increase GitHub calls per attempt — it decreases them. Today's fast
  path (`GetPRForBranch`) already makes a REST list call *and* delegates to
  `GetPRInfoCtx`'s `gh pr view` subprocess for full details
  (`github/client.go:427`) — two round trips, one of them a fork/exec. The
  new `GetPRByNumber` is a single REST call, no subprocess, for both the fast
  and fallback path. `pitfalls.md`'s point that `ghHTTPClient` has no actual
  rate-limit-aware transport still stands as a pre-existing, out-of-scope gap
  — but this fix does not make exposure to it worse; it makes it slightly
  better.
- Rejections (missing `override_reason`, PR doesn't exist, PR closed) are
  **not** separately server-logged, consistent with how the pre-existing
  role-check/idempotency/branch-mismatch rejections already behave today —
  the MCP error text itself is the audit surface visible in the session
  transcript. Not changed by this fix; noted so it isn't mistaken for an
  oversight.

## Risk Control

- **No feature flag.** This is an internal MCP tool handler with no
  user-facing surface change outside the agent-facing tool schema/description;
  `.claude/rules/` gives no specific reason to flag-gate a bug fix at this
  scope, and adding one would be its own unjustified complexity per the
  interface-pollution checklist's "unjustified generic" spirit applied to
  process, not just code.
- **Rollback**: standard PR revert. `GetPRByNumber`, `PRVerification`, and
  the `reportPRCreated` branching are all pure/stateless — no persisted data
  format changes, no migration. Reverting the commit fully restores the prior
  (stricter, branch-name-only) behavior with zero cleanup.
- **Residual risk, stated explicitly** (see also
  `decisions/ADR-001-override-reason-security-model.md`): `override_reason`
  is a procedural deterrent, not a technical one — nothing stops the same
  work-role session that misjudged the original PR from also supplying a
  plausible-sounding `override_reason` for an unrelated PR, as long as that
  PR happens to be open/merged in the correct repo. The bound on this risk is
  the pre-existing role + item-link check (unchanged): only a session already
  trusted to work *this specific item* can invoke the fallback path at all,
  and every use is logged for after-the-fact review. This is a narrower
  guarantee than "GitHub-verified to be the same lineage of work," accepted
  deliberately because a stronger guarantee would require an operator/human
  auth primitive requirements.md puts out of scope (no UI changes).

## Unresolved Questions

1. **`SetBacklogItemPRAndTransition` non-atomicity when an item is already
   `pr_pending` with a *different* PR number** (`pitfalls.md` §6): if this
   fallback path is ever invoked against an item that's already `pr_pending`
   (not `review`) with a stale/wrong PR number, the unconditional
   `UpdateBacklogItem` write (`session/storage.go:768`) would land before the
   `ExpectedStatus: review` transition precondition fails
   (`session/storage.go:775`), leaving a partial write. **Not fixed by this
   plan** — the acceptance criteria's confirmed repro item
   (`3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d`) is in `review` status, so this
   plan's Given-When-Then scenarios don't hit it. Blocks a *future* "correct
   an already-wrong PR attachment" story, not this one. Flag as a follow-up
   bug, don't expand this plan's scope to fix it.
2. **Should `override_reason` additionally require `item.PrNumber == 0`** (an
   extra guard limiting the fallback path to items that have genuinely never
   had a PR recorded)? Decided **not** to add this — the role + item-link
   check already scopes who can call the tool for which item, and the
   idempotency check ahead of it already handles the "already succeeded"
   case. Recorded here as a deliberate judgment call in case a future
   reviewer wants it reconsidered.
3. **`reconcileOrphanedAgentPRs`' shared blind spot** (`research/features.md`):
   noted via a one-line comment only (Task 5.1), not fixed — confirmed out of
   scope by requirements.md. A future fast-follow item should migrate it from
   `GetPRForBranch`/`getOrphanedPRFinder` to the same `GetPRByNumber`-based
   check this plan introduces.
