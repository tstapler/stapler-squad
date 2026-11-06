# Implementation Plan: worktree-base-branch-and-capture-dup-check

**Feature**: Confirm the already-shipped `new_worktree` base-branch-resolution and
terminal-output-capture fix (PR #833/#837) still satisfies AC1-AC4, then close
backlog item `4d856751-bfe9-4566-99ca-2c483b23bd06` as a duplicate of
`c7466f05-3d19-4d25-a822-9ea1ac7a6faa` via the `report_duplicate` MCP tool.
**Date**: 2026-09-24
**Status**: Ready for implementation
**ADRs**: None — closing via `report_duplicate` is the existing, purpose-built
mechanism for this exact scenario (see `research/build-vs-buy.md`); there is no
contestable design decision to record.

---

## Domain Glossary

N/A — complexity 1, no new domain types.

---

## Pattern Decisions

N/A — complexity 1, no new components.

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `session/git/ops.go:52` `CandidateDefaultBranches` (`main`/`master`/`develop`/`trunk` only, no explicit base-branch override param on `new_worktree`) | A repo whose real default branch has another name (e.g. `development`) hard-fails `new_worktree` session creation, with no override available (unlike `CreateBacklogWorktree`'s `baseBranch` param) | Extend as-is / defer | Narrow edge case, not hit by this repo's own usage (`main`); AC1's "require an explicit base-branch parameter" fallback arm was never implemented by the shipped fix, but the auto-resolve arm covers the reported bug. Worth its own future small backlog item (see Story 1.1.3), not worth blocking or reopening this item for. |
| `server/mcp/tools_terminal.go:543` `runCommand`; `AppendOutput` call sites in `server/services/connectrpc_websocket.go` (`forwardOneControlModeFrame` line 1639, `forwardCapturePaneOutput` line 3219) | Zero direct test coverage on `runCommand`; existing `AppendOutput` tests call the method directly, bypassing the real production call sites | Extend as-is / defer | Code is stable and working (confirmed via `research/stack.md`'s test re-run and this triage's own verification below); a future refactor of either wiring point could regress silently with no test to catch it, but that's a test-coverage gap, not a live bug. Worth its own future small backlog item (see Story 1.1.3), not worth blocking closure of this item. |

---

## Migration Plan
N/A — complexity 1

## Observability Plan
N/A — complexity 1

## Risk Control
N/A — complexity 1

## Unresolved Questions

None. (The one open unknown — whether backlog item `4d856751-...` is currently
linked to a work-role session, a triage-role session, or unclaimed — determines
which of three routing branches applies: `report_duplicate` directly
(linked, work-role), `report_duplicate` dispatching internally to
`reportDuplicateUnclaimed` (unclaimed), or `submit_triage_result` naming the
duplicate for a future session to close (linked, non-work-role — the likely
case for this session; see Story 1.1.2's AC5). Task 1.1.2a's pre-task check
determines routing among all three branches, and each branch has a defined
terminal action, so nothing is left open — the pre-task check alone doesn't
resolve the question, the three defined branches together do.)

## Dependency Visualization

```
Story 1.1.1 (verify AC1-4, re-run tests + live repro if available)
        │
        ▼
Story 1.1.2 (check item link state/role → report_duplicate /
             reportDuplicateUnclaimed / submit_triage_result, AC5)

Story 1.1.3 (two follow-up gaps) — independent, deferred to a future item, not
executed by this plan
```

---

## Phase 1: Verify and close the duplicate backlog item

### Epic 1.1: Confirm the shipped fix, then close the duplicate

**Goal**: Re-run the tests covering AC1-AC4 (plus a live MCP repro of AC1/AC3
if tool access allows — see Task 1.1.1d) to get fresh, reproducible evidence
the shipped fix (PR #833/#837) still holds, then close backlog item
`4d856751-...` as a duplicate of `c7466f05-...` — via `report_duplicate` (direct
or dispatched to `reportDuplicateUnclaimed`) if this session's role permits it,
or via `submit_triage_result` naming the duplicate for a future session to
close otherwise — never a raw status edit, and never re-implementing anything
that already exists.

#### Story 1.1.1: Verify the shipped fix satisfies AC1-AC4

**As a** triage session, **I want** to re-run the test suites covering
base-branch resolution and terminal-output capture, **so that** I have fresh,
reproducible evidence for AC1-AC4 before closing the item as a duplicate.

**Acceptance Criteria**:
- AC1: A `new_worktree` session created from a source repo whose ambient HEAD
  diverges from `origin/<default-branch>` branches from the resolved default
  branch's tip, not the ambient HEAD.
  - *Given* a repo on branch `tstapler/create-workflow-mark-failed-on-pr-timeout`
    (diverged ~20 commits from `origin/main`), *When*
    `create_session(session_type=new_worktree, ...)` is called, *Then* the new
    worktree's `git log` HEAD matches `origin/main`'s resolved tip SHA, per
    `session/instance_worktree.go:219` `newWorktreeFromResolvedBase` and
    `session/git/ops.go:141` `ResolveWorktreeBaseCommit`.
- AC2: When ambient HEAD does diverge from the resolved base, the divergence is
  surfaced to the caller (not silent).
  - *Given* the same diverged-repo scenario, *When* the worktree is created,
    *Then* `session/git/ops.go:160` `AmbientHEADDivergesFromBase` returns `true`
    and `Instance.CreationWarning` / `SessionDetail.creation_warning` is
    non-empty (not silently dropped).
- AC3: `run_command`/`read_session_output` against a freshly created, ready
  session return real terminal output (not silently empty) for a trivial command
  like `echo alive-check`.
  - *Given* a ready session whose scrollback sequence has advanced past creation,
    *When* `read_session_output` is called after `run_command echo alive-check`,
    *Then* the response contains the echoed `alive-check` line — because
    `forwardOneControlModeFrame`/`forwardCapturePaneOutput`
    (`server/services/connectrpc_websocket.go:1639`, `:3219`) call
    `ScrollbackManager.AppendOutput` on every real output frame.
- AC4: A session that hasn't finished initializing yet is distinguishable from
  one that ran a command and produced no output, via `SESSION_NOT_READY`.
  - *Given* a session whose scrollback sequence hasn't advanced at all since
    creation, *When* `read_session_output` is called, *Then* the response is the
    `SESSION_NOT_READY` error code from `sessionNotReadyResult`
    (`server/mcp/tools_terminal.go:187`), not an empty-but-"successful" result.
**Files**: `session/git/ops.go`, `session/git/ops_test.go`,
`session/instance_worktree.go`, `session/instance_worktree_test.go`,
`server/mcp/tools_terminal.go`, `server/mcp/tools_terminal_test.go`,
`server/services/connectrpc_websocket.go`

##### Task 1.1.1a: Re-run `session/git` base-branch-resolution tests (~3 min)
- Run: `go test ./session/git/... -run "TestAmbientHEADDivergesFromBase|TestResolveRemoteWorktreeBaseCommit|TestResolveWorktreeBaseCommit"`
- Confirm all of: `TestResolveWorktreeBaseCommit_UsesOriginDefaultBranch`,
  `TestResolveWorktreeBaseCommit_FallsBackToLocal_When_OriginFetchFails`,
  `TestResolveWorktreeBaseCommit_ReturnsEmptySHA_When_RepoIsUnborn`,
  `TestResolveWorktreeBaseCommit_ReturnsError_When_NoCandidateAndNotUnborn`,
  `TestAmbientHEADDivergesFromBase_True_When_CheckedOutBranchDiffersFromBase`,
  `TestAmbientHEADDivergesFromBase_False_When_AmbientHEADIsBase`,
  `TestResolveRemoteWorktreeBaseCommit_MatchesLocalResolution`,
  `TestResolveRemoteWorktreeBaseCommit_ReturnsEmptySHA_When_RepoIsUnborn`,
  `TestResolveRemoteWorktreeBaseCommit_ReturnsError_When_RunnerFailsEntirely` pass
  (9 tests total, per `research/stack.md`). Covers AC1/AC2.
- Files: `session/git/ops_test.go`

##### Task 1.1.1b: Re-run worktree-setup tests (~2 min)
- Run: `go test ./session/... -run TestSetupFirstTimeWorktree`
- Confirm `TestSetupFirstTimeWorktree_NewWorktree_BranchesFromOriginDefault_NotAmbientHEAD`
  and `TestSetupFirstTimeWorktree_NewWorktree_NoWarning_When_AmbientHEADMatchesBase`
  pass (both directly exercise AC1/AC2).
- Files: `session/instance_worktree_test.go`

##### Task 1.1.1c: Re-run `server/mcp` terminal-output tests (~2 min)
- Run: `go test ./server/mcp/... -run "TestReadOutput|TestSessionNotReadyResult"`
- Confirm `TestReadOutputLineCap`, `TestReadOutputSessionNotFound`,
  `TestReadOutputSessionNotReady`, `TestReadOutputSucceeds_When_ReadyWithNoNewBytes`,
  and `TestSessionNotReadyResult` all pass. Covers AC3/AC4.
- Files: `server/mcp/tools_terminal_test.go`

##### Task 1.1.1d: Live MCP repro of AC1 and AC3, if live tool access is available (~5 min)
- Why this task exists: PR #837's own 8-item test-plan checklist is entirely
  unchecked, including both manual repros of the original bugs
  (`research/pitfalls.md` §3, §5), with no evidence anyone ever ran them. The
  unit tests re-run in Tasks 1.1.1a-c passing is not, by itself, strong
  evidence — that was also true *before* the fix (`AppendOutput` was only ever
  called from tests pre-fix). Closing that gap needs one real MCP round-trip,
  not another unit-test re-run.
- If this session has live MCP tool access: from a source repo deliberately
  checked out on a branch that diverges from `origin/<default-branch>` (e.g.
  this triage worktree's own branch vs. `origin/main`), call
  `create_session(session_type=new_worktree, ...)`, then `run_command echo
  alive-check` followed by `read_session_output`. Confirm both:
  - the echoed `alive-check` line actually comes back (closes AC3 with real
    evidence, not just a re-run unit test); and
  - the new worktree's `git log` HEAD matches `origin/<default-branch>`'s
    resolved tip SHA, not the diverged ambient HEAD (closes AC1 with real
    evidence).
- If live MCP tool access is NOT available when this plan is executed: say so
  explicitly in Task 1.1.1e's verification record and fall back to the
  unit-test evidence from Tasks 1.1.1a-c as the best available — do not
  silently skip this task.
- Files: none (live tool calls only, no code changes)

##### Task 1.1.1e: Record verification result (~2 min)
- If Tasks 1.1.1a-c all pass, and Task 1.1.1d either confirms the live repro or
  explicitly records that live tool access wasn't available: note in this
  plan's execution log (or the session's summary) that AC1-AC4 are confirmed
  as of the date run — this is the evidence gate before Story 1.1.2 proceeds.
- If anything fails: STOP — do not proceed to Story 1.1.2. A failing test or a
  failed live repro here means the "already shipped" premise is wrong and this
  item should NOT be closed as a duplicate; escalate instead of closing.
- Files: none (verification note only)

#### Story 1.1.2: Close backlog item 4d856751 as a duplicate (AC5)

**As a** triage session, **I want** to record the duplicate finding through
whichever mechanism this session's role actually authorizes — `report_duplicate`
directly (linked, work-role), `report_duplicate` dispatching internally to
`reportDuplicateUnclaimed` (unclaimed), or `submit_triage_result` naming the
duplicate for a future session to close (linked, non-work-role) — **so that**
the item is provably routed toward closure regardless of which role this
session holds. (Adversarial review, iteration 2: the earlier "per ADR-001"
citation here did not correspond to any real ADR in `docs/adr/` — removed
rather than replaced, since the branch-by-branch AC5 below already justifies
itself against the actual code without needing a design-decision citation.)

**Acceptance Criteria**:
- AC5: This backlog item is confirmed a duplicate of `c7466f05-...` and
  closed/merged into that item rather than re-triggering implementation.
  - *Given* this session IS linked to item `4d856751-...` with `Role ==
    session.SessionRoleWork` (i.e. `resolveItemLink` succeeds), *When*
    `report_duplicate(item_id="4d856751-bfe9-4566-99ca-2c483b23bd06",
    duplicate_ref="https://github.com/tstapler/stapler-squad/pull/837",
    reason="...")` is called, *Then* the item transitions to `review` status
    with a `duplicate_ref=... reason=...` entry appended to the linked
    ItemSession's `VerificationNotes` (per `server/mcp/tools_backlog.go`'s
    `reportDuplicate`, line ~2159) — it is NOT closed/archived directly.
  - *Given* this session is NOT linked to item `4d856751-...` (no ItemSession
    link) AND the item's status is one of idea/refining/ready/queued
    (`unclaimedDuplicateSourceStatuses`) with no active un-ended ItemSession on
    it, *When* the same `report_duplicate` call is made, *Then*
    `reportDuplicateUnclaimed` (`server/mcp/tools_backlog.go`, line ~2391)
    archives the item directly with a note citing PR #837 and this session's UUID
    — this is the terminal AC5 state for that branch.
  - *Given* this session IS linked to item `4d856751-...` but with a
    **non-work role** (e.g. `Role == session.SessionRoleTriage`) — the case
    this worktree's own artifact layout (`research/*.md`,
    `implementation/plan.md`, no `validation.md`) and the triage-role guidance
    in `get_backlog_item` (`server/mcp/tools_backlog.go:501-515`) suggest is
    the actual state for the session executing this plan — *When*
    `report_duplicate` would reject with `PERMISSION_DENIED` on the role check
    (`tools_backlog.go:2151-2154`/`:2212-2213`), and `reportDuplicateUnclaimed`
    would also reject because this session's own un-ended `ItemSession` counts
    as an active session on the item (`tools_backlog.go:2419-2425`), *Then* do
    NOT attempt `report_duplicate` at all — instead call
    `submit_triage_result` with a `summary`/`suggestions` that explicitly state
    the duplicate finding (citing PR #837 and this triage's fresh AC1-AC4
    verification from Story 1.1.1), instructing whichever session or human
    picks up the item next to run `/backlog/duplicate` (or call
    `report_duplicate` themselves as a work-role session) to complete the
    closure — this is the terminal AC5 state for that branch.
  - Any branch that calls `report_duplicate` requires `duplicate_ref` to be a
    real, GitHub-verified PR/issue/commit URL (`resolveDuplicateRef` calls
    `GetPR`/`GetIssue`/`GetCommit` — confirmed via `gh pr view 837` returning a
    real PR body explicitly linking backlog item `c7466f05-...`).
**Files**: `server/mcp/tools_backlog.go` (no code changes — this story only calls
the existing tool)

##### Task 1.1.2a: Check whether this session is linked to item 4d856751, and route (~2 min)
- Do NOT guess. Read the item's current status, session link, and role — via
  `get_backlog_item` (MCP tool) or the backlog UI at
  `http://127.0.0.1:8543/backlog?item=4d856751-bfe9-4566-99ca-2c483b23bd06` —
  before calling `report_duplicate`.
- `SkipReviewGate` pre-check (narrow 4th edge case): `report_duplicate`
  unconditionally rejects if this flag is set on the item, regardless of
  role/status (`tools_backlog.go:2236-2238`). Adversarial review (iteration 2)
  found that `get_backlog_item`'s MCP response never actually surfaces this
  field (verified: `tools_backlog.go:394-548` never writes it into the
  response text) — it exists only as edit-mode UI state
  (`BacklogItemForm.tsx`), not a passive read. Since requirements.md's own
  finding already establishes this item as a straightforward bug-report
  duplicate (not one flagged for a special review-gate skip), proceed on the
  explicit assumption that `SkipReviewGate` is unset for `4d856751-...`
  rather than attempting an MCP read that doesn't exist; if `report_duplicate`
  unexpectedly rejects citing `SkipReviewGate` at execution time, that
  assumption was wrong — stop and re-route to `request_review` guidance then.
- **Branch 1 — linked, `Role == work`**: proceed to Task 1.1.2b's
  `report_duplicate` (direct) path.
- **Branch 2 — unclaimed** (status in idea/refining/ready/queued, no active
  ItemSession): proceed to Task 1.1.2b's `report_duplicate` (unclaimed) path —
  same tool call, `report_duplicate` dispatches internally to
  `reportDuplicateUnclaimed`.
- **Branch 3 — linked, non-work role** (e.g. `Role == triage` — the case this
  worktree's own triage-workflow artifact layout suggests is the actual state):
  do NOT call `report_duplicate` — it would reject on both the role check
  (`PERMISSION_DENIED`, `tools_backlog.go:2151-2154`/`:2212-2213`) and, for the
  unclaimed fallback, the active-session guard (`tools_backlog.go:2419-2425`,
  since this session's own un-ended link counts as active). Proceed to Task
  1.1.2b's `submit_triage_result` path instead — that call succeeds here
  because `resolveItemLink` finds this session's own triage link.
  **Caveat** (adversarial review, iteration 2): if instead this session has
  *no* link to the item at all and it's claimed by a *different*, still-active
  work session, `submit_triage_result` would also reject on its own link check
  (`tools_backlog.go:2476-2481`) — there is no executable terminal action for
  that sub-case in this plan; if `get_backlog_item` reveals it, stop and
  escalate to the operator rather than assuming Branch 3 applies.
- Files: none (read-only check)

##### Task 1.1.2b: Call the routed action with the verified evidence (~3 min)

**Branches 1 and 2 (Task 1.1.2a) — call `report_duplicate`:**
- Call the `report_duplicate` MCP tool:
  - `item_id`: `4d856751-bfe9-4566-99ca-2c483b23bd06`
  - `duplicate_ref`: `https://github.com/tstapler/stapler-squad/pull/837`
  - `reason`: cite both PRs and this triage, e.g. "Same bug (new_worktree
    branches from ambient HEAD instead of origin/<default>; run_command/
    read_session_output return empty) already fixed by PR #833
    (1cc66c53d, base-branch resolution + scrollback wiring) and PR #837
    (ccbc1dba6, explicitly linked to backlog item c7466f05-3d19-4d25-a822-9ea1ac7a6faa
    in its own body) — both merged to main before this item was triaged.
    Verified AC1-AC4 still pass via fresh test re-run
    (session/git, session/instance_worktree_test.go, server/mcp/tools_terminal_test.go)."
- Confirm the tool's response text matches the expected mode from Task 1.1.2a
  (either "routed to review as a duplicate of ..." or "archived directly as a
  duplicate of ...").

**Branch 3 (Task 1.1.2a) — call `submit_triage_result` instead:**
- Do NOT call `report_duplicate` — it will reject (see Task 1.1.2a). Call
  `submit_triage_result` with:
  - `summary`: states plainly that `4d856751-...` is a duplicate of
    `c7466f05-3d19-4d25-a822-9ea1ac7a6faa`, already fixed by PR #833
    (1cc66c53d) and PR #837 (ccbc1dba6, which explicitly links
    `c7466f05-...` in its own body), with AC1-AC4 freshly re-verified in this
    triage (Story 1.1.1, including the live repro from Task 1.1.1d if it ran).
  - `suggestions`: instruct the next session or human operator to run
    `/backlog/duplicate` against `4d856751-...` with `duplicate_ref` =
    `https://github.com/tstapler/stapler-squad/pull/837` (or call
    `report_duplicate` directly as a work-role session) to complete the
    closure — this session's triage role cannot call `report_duplicate` itself.
- This is the terminal action for Branch 3: it does not close the item
  directly, but it is the mechanism the system actually authorizes for a
  triage-role session, and it leaves a concrete, actionable next step rather
  than stopping with no defined action.
- Files: none (MCP tool call only)

##### Task 1.1.2c: Read back the resulting state to confirm the mutation landed (~2 min)
- **Branches 1 and 2**: per the "read a mutation back" discipline, re-fetch the
  item via `get_backlog_item` (or the backlog UI) and confirm its status
  actually changed (`review` or `archived`, matching Task 1.1.2b's response)
  and that `VerificationNotes`/the status-history note actually contains the
  `duplicate_ref=https://github.com/tstapler/stapler-squad/pull/837` entry — do
  not trust the tool's success text alone.
- **Branch 3**: re-fetch the item via `get_backlog_item` and confirm the
  `submit_triage_result` call actually landed (the duplicate-finding
  summary/suggestions are visible on the item). The item's status will NOT be
  `review`/`archived` yet — closure is deferred to whichever session or human
  next runs `/backlog/duplicate` — so confirm the deferred instruction is
  genuinely present on the item, not just returned in the tool's response text.
- Files: none (read-only check)

#### Story 1.1.3 (optional, follow-up, not required for this item's closure)

**As a** future triage/planning session, **I want** the two narrow gaps
`research/pitfalls.md` surfaced recorded as candidate follow-up work, **so that**
they aren't silently lost even though nothing here requires acting on them now.

**Acceptance Criteria**: N/A — this story produces no state change; it only
records candidates for a future, separate backlog item.
**Owner note** (adversarial review, iteration 2): these two candidates are not
self-filing. Whoever closes out this triage (the operator, or a future session
reading this plan) is the owner of actually filing them as new backlog items —
otherwise they are silently dropped once this session ends.
**Files**: none — this story produces no code changes.

##### Task 1.1.3a: Record candidate follow-up 1 — `runCommand`/`AppendOutput` test coverage (~2 min)
- Candidate: add a `runCommand`-level test (`server/mcp/tools_terminal.go:543`
  currently has zero direct coverage) plus one true end-to-end test that
  exercises the real `AppendOutput` call sites in
  `server/services/connectrpc_websocket.go` (`forwardOneControlModeFrame:1639`,
  `forwardCapturePaneOutput:3219`) rather than calling `AppendOutput` directly
  from the test.
- Do not implement now — file as a new, separate backlog item if/when picked up.
- Files: none

##### Task 1.1.3b: Record candidate follow-up 2 — default-branch resolution gap (~2 min)
- Candidate: either query the repo's actual configured default branch via `git
  symbolic-ref refs/remotes/origin/HEAD` instead of only the 4 hardcoded
  candidates in `session/git/ops.go:52` `CandidateDefaultBranches`, or add an
  explicit base-branch override param to `new_worktree` session creation
  (mirroring `CreateBacklogWorktree`'s existing `baseBranch` param).
- Do not implement now — file as a new, separate backlog item if/when picked up.
- Files: none
