# Research: Pitfalls & Risks — backlog-self-resolve

Covers the two changes in `requirements.md`: (1) generalizing `request_review`'s
CAS precondition from a hardcoded `in_progress` to `in_progress`-or-`pr_pending`,
and (2) a new `report_duplicate` MCP tool. All line refs are from this worktree
at the commit checked out when this research ran (`32f504c80`).

## 0. The "always-true CAS" trap in FR1's dynamic precondition

**This is the sharpest pitfall in the whole change and is not just a race —
it's a logic bug that a straightforward reading of FR1 invites.**

`requestReview` already loads `item` before building the precondition
([server/mcp/tools_backlog.go:405-411](../../../server/mcp/tools_backlog.go#L405)):
```go
item, itemErr := h.storage.GetBacklogItem(ctx, itemID)
...
precondition := &session.BacklogItemPrecondition{ExpectedStatus: string(session.BacklogStatusInProgress)}
```
FR1 says "generalize to the actually-observed status (in_progress OR
pr_pending), never a hardcoded constant." The naive implementation of that
sentence is:
```go
precondition := &session.BacklogItemPrecondition{ExpectedStatus: item.Status}
```
**This is wrong and defeats the entire purpose of the precondition.** A CAS
precondition only guards against something changing *between the read and the
write* — it says nothing about whether the *read* value itself was a legal
starting state. If `ExpectedStatus` is set to whatever was just observed, the
check `WHERE status = ExpectedStatus` is trivially true by construction for
*any* status the item happens to be in — `idea`, `refining`, `queued`,
`review`, `done`, `archived`, all of them. An item sitting in `done` (e.g. a
stale/reused session UUID, a race with a concurrent ship, a buggy caller)
would sail through the CAS and get transitioned to `review` (or `done` again,
if `SkipReviewGate`), because the "precondition" no longer constrains
anything — it's just echoing back what was already true.

The fix is a two-step check, not a one-step substitution:
1. Validate `item.Status` is a member of the allowed source set
   (`{in_progress, pr_pending}`) — reject with `ErrInvalidArgument` (or the
   codebase's existing precondition-failure vocabulary) if not, **before**
   building any precondition.
2. Only then set `ExpectedStatus` to that validated value and pass it to
   `TransitionBacklogItemStatus`, which still does the real work of catching
   *concurrent* changes in the gap between this validation and the write (see
   §1 below — that part of the CAS is sound).

Same trap applies verbatim to `report_duplicate` (FR3/FR6): "refuse... when
session not linked to item" and the general "only work-role, only from a live
in-progress/pr_pending item" framing implies the same allowed-source-status
whitelist, not an echo of whatever status happened to be observed.

Concretely: the existing test suite (§ below) never exercises a call from
`done`, `idea`, or `archived`, so this bug would not be caught by "existing
tests pass unmodified" (AC9) — it needs a **new** test asserting
`request_review` (and `report_duplicate`) reject a call when the item is in a
status outside the allowed set, distinct from the CAS-race tests that assert
concurrent-write rejection.

## 1. CAS race: is the precondition widening actually risky?

**Verdict: the DB layer already closes this. The widening does not open a new race.**

`EntRepository.TransitionBacklogItemStatus`
([session/ent_repository_backlog.go:869-951](../../../session/ent_repository_backlog.go#L869))
does **not** read-then-write. It issues a single atomic conditional `UPDATE ...
WHERE id = ? AND status = ?` (`update.Where(backlogitem.StatusEQ(precondition.ExpectedStatus))`,
line 886) and checks `affected == 0` to detect a lost race, re-fetching only to
report *what* changed, not to decide the outcome. This is a hardened
implementation — the doc comment above it (lines 860-868) cites two prior
incidents (BUG-026, PR #199) where a read-then-write TOCTOU gap caused a stale
precondition to silently succeed. That gap is closed.

Consequence for FR1/FR2: a work session calling `request_review` (with
`ExpectedStatus` pinned to whatever status was actually observed on the loaded
item, per FR1) and `ReconcileStuck`'s ticker concurrently touching the same row
(e.g. the merged-PR auto-`done` path at
[session/backlog_lifecycle.go:3921](../../../session/backlog_lifecycle.go#L3921)
and [:4184](../../../session/backlog_lifecycle.go#L4184), both CAS'd on
`ExpectedStatus: pr_pending`) will race safely: exactly one wins atomically, the
loser gets `ErrPreconditionFailed`, no double-apply, no lost update. Widening
the precondition to accept two source statuses does not weaken this — the WHERE
clause still pins to whichever single status was actually pinned per-call.

**Real gap found (not a race, an error-handling gap):** neither `request_review`
nor (by construction) the new `report_duplicate` currently distinguish
`ErrPreconditionFailed` from any other transition error. `request_review`'s
current handling
([server/mcp/tools_backlog.go:415-418](../../../server/mcp/tools_backlog.go#L415)):
```go
if _, transErr := h.storage.TransitionBacklogItemStatus(ctx, itemID, targetStatus, precondition, session.TriggeredBySystem); transErr != nil {
    return errResult(ErrInternalError, fmt.Sprintf("transition to %s failed: %v", targetStatus, transErr), ""), nil
}
```
A CAS loss (someone else already resolved/moved the item — e.g. ReconcileStuck
auto-closed it as merged, or a duplicate review request raced another call) and
a genuine DB error both surface as the same generic `ErrInternalError` with
"retry" framing. For a CAS loss, retrying is nonsensical — the item moved on
legitimately. Recommend: `errors.Is(transErr, session.ErrPreconditionFailed)` →
a distinct, non-retry message ("item state changed since your last read — call
`get_backlog_item` to see its current status") for both the generalized
`request_review` and the new `report_duplicate`.

**FR2 implementation precedent already exists.** The "refuse if an active
review-role session exists" check FR2 asks for has a near-identical existing
helper: `hasActiveReviewSession`
([server/services/backlog_service_triage.go:1104](../../../server/services/backlog_service_triage.go#L1104)),
`ps.Role == session.SessionRoleReview && ps.EndedAt == nil`. It lives in a
different package (`server/services`) than `tools_backlog.go`
(`server/mcp`), so it can't be imported directly — but `tools_backlog.go`
already calls `storage.ListItemSessions(ctx, itemID)` at
[line 243](../../../server/mcp/tools_backlog.go#L243), which returns
`[]ItemSessionSummary` with `Role`/`EndedAt`, so the same one-line predicate can
be reimplemented locally rather than needing new storage plumbing. Watch for a
subtle trap noted in existing comments
([session/backlog_lifecycle.go:1695-1697](../../../session/backlog_lifecycle.go#L1695)):
a zombie review session (tmux process dead, but the ItemSession row still has
`EndedAt == nil`) will read as "active" by this predicate. `ReconcileStuck`'s
zombie detector tombstones these on its own tick, but there's a window where a
zombie reviewer is still "active" by this check and `report_duplicate`/
`request_review` at `pr_pending` would incorrectly refuse. This is a pre-existing
limitation the codebase already accepts elsewhere — not a new bug this feature
introduces, but worth a one-line note in the tool's error text ("if this
persists after a few minutes, the review session may be dead — an operator can
check with `get_backlog_item`") rather than treating it as unexpected.

## 2. TOCTOU between "verify GitHub ref" and "mutate local state"

There's an unavoidable gap (verify happens, then a separate DB write happens),
but this codebase's own precedent (`report_pr_created` /
`VerifyPRMatchesBranch`) shows the accepted mitigation is **not** to close the
race — it's to make the later mutation itself re-verify via CAS, and to make
"the ref went stale between check and write" a low-consequence outcome rather
than one requiring a distributed lock:

- The reference genuinely disappearing/changing in the few-hundred-ms window
  between a GitHub API call and a local `TransitionBacklogItemStatus` call is
  extremely unlikely (PRs/issues/commits are not deleted or renumbered by
  routine activity), and even if it happened, the consequence is bounded: the
  item lands in `review` with a `duplicate_ref` that a human reviewer can
  re-check — it never reaches a terminal state (`done`/`archived`) directly,
  which is exactly why FR3 requires routing through `review` rather than
  resolving straight to a terminal status. The review gate is the safety net
  for this TOCTOU window, not a second GitHub re-check.
- Do not add a second GitHub call right before the mutation "just in case" —
  `.claude/rules/prefer-go-git-over-subshells.md`'s sibling principle
  (name the specific failure a fallback exists for) applies by extension: a
  speculative re-verify adds rate-limit cost and latency without closing a
  meaningfully exploitable window.

## 3. Idempotency: `report_duplicate` called twice for the same item

`report_pr_created` has an explicit idempotency guard worth mirroring exactly:

```go
// server/mcp/tools_backlog.go:682-686
if item.Status == string(session.BacklogStatusPRPending) && item.PrNumber == prNumber {
    return mcpgo.NewToolResultText(fmt.Sprintf(
        "PR #%d already recorded for item %s (status already pr_pending) — no changes made.", prNumber, itemID,
    )), nil
}
```

`report_duplicate` needs the equivalent: if the item is already `review` (or
`done`, if `SkipReviewGate`) **and** the persisted verification notes already
contain this same `duplicate_ref`, return success-no-op rather than attempting
a second CAS'd transition that will fail with `ErrPreconditionFailed` (item is
no longer at `in_progress`/`pr_pending`) and get misread as a real error. Two
concrete triggers to handle:
- Network retry: caller times out waiting for the response but the first call
  actually committed; the identical retry must not error.
- Ambiguous-timeout re-call: agent isn't sure the first call landed, calls
  again — same requirement.

Also note: `UpdateItemSessionVerificationNotes`
([session/storage_backlog.go:397](../../../session/storage_backlog.go#L397)) is
a plain overwrite (`SetVerificationNotes`), not an append/merge. If
`report_duplicate` is called twice with **different** `duplicate_ref`/`reason`
values after the item has already moved to `review` (second call arriving after
the CAS-protected transition already happened), the naive idempotency check
above would reject it as "already resolved" and the second reference would be
silently dropped rather than appended. Decide up front whether a
second-call-with-different-ref is (a) rejected outright (simplest, matches
"routes to review once, human takes it from there") or (b) merged into the
notes — don't leave it as an accidental overwrite-or-drop by omission.

## 4. Error-channel pitfalls when mirroring `report_pr_created`'s two-channel pattern

`report_pr_created`'s verification call already has the exact three-way return
shape FR4 wants, documented at
[server/mcp/tools_github.go:264-271](../../../server/mcp/tools_github.go#L264):
```
- (true, nil):  definitive match
- (false, nil): definitive mismatch — do NOT persist, do NOT retry
- (false, err): transient (rate limit/network/auth) — retry
```
And the caller correctly maps this 1:1 to `ErrInvalidArgument` /
`ErrInternalError` at
[server/mcp/tools_backlog.go:707-715](../../../server/mcp/tools_backlog.go#L707).
This is the right template. Two things easy to get wrong when extending it to
`report_duplicate`, which must handle PR **or** issue **or** commit refs
(unlike `report_pr_created`, which is PR-only):

- **`ParseGitHubURL` does not support issue or commit URLs at all.**
  ([session/repo_path.go:93-150](../../../session/repo_path.go#L93)) only
  recognizes: full PR URL (`/pull/N`), branch URL (`/tree/branch`), repo URL,
  and `owner/repo[:branch]` shorthand. There is no `/issues/N` or commit-SHA
  pattern. `duplicate_ref` being "a PR/issue/commit URL" per FR3 means new
  parsing has to be added (or the tool must reject non-PR refs until that
  parsing exists) — this is not a drop-in reuse of `ParseGitHubURL`, despite
  looking like one.
- **No commit-lookup function exists in `github/` at all** (`grep -n "func.*Commit"
  github/*.go` returns nothing). `GetIssue` exists
  ([github/repos.go:270](../../../github/repos.go#L270)) with the right
  404/401/403/429 status-code discrimination already built in (good template
  to copy), and `GetPRInfoCtx` exists for PRs but shells out to the `gh` CLI
  rather than hitting the REST API directly — i.e. **the existing GitHub
  helpers are inconsistent about HTTP-direct vs `gh`-CLI-subprocess**, which
  matters for auth: `GetIssue`'s HTTP path resolves a token via
  `getGHToken(ctx)` (env vars / keychain per
  [github/http_client.go:29-36](../../../github/http_client.go#L29)), while
  `GetPRInfoCtx` requires `gh` itself to be authenticated
  (`CheckGHAuth()`) — a host where `GITHUB_TOKEN` is set but `gh auth login`
  was never run would pass `GetIssue` and fail `GetPRInfoCtx`, or vice versa.
  Mixing both mechanisms inside one `report_duplicate` implementation (PR path
  via `gh` CLI, issue path via direct HTTP) would make auth failures behave
  inconsistently across `duplicate_ref` types — pick one mechanism and use it
  for all three ref kinds, or explicitly document why they differ.
- **404 vs 403 vs timeout classification**: `GetIssue`'s existing status-code
  handling is the correct template — 404 → definitive-nonexistent
  (`ErrInvalidArgument`), 401/403 without `Retry-After` → auth/permission
  (ambiguous: could be "no access to a real private repo" which is arguably
  also non-retryable, but the existing code folds it into a generic 403
  message — worth deciding explicitly whether `report_duplicate` treats "403,
  no rate-limit signal" as retryable `ErrInternalError` or a hard
  `ErrInvalidArgument`, since retrying an auth failure that isn't going to
  self-heal wastes the agent's retry budget), 403 w/ `Retry-After` or 429 →
  rate limit (`ErrInternalError`, retryable, ideally with the `Retry-After`
  value surfaced in the message so the agent doesn't hammer it), network
  error/timeout → `ErrInternalError`, retryable. Do not let a plain Go
  `err != nil` from the HTTP client (network timeout) fall through to the same
  bucket as a 404 — that's the single most likely copy-paste mistake mirroring
  `report_pr_created`, since `VerifyPRMatchesBranch`'s three-way contract
  (`false,nil` vs `false,err`) is easy to collapse to a two-way `bool, error`
  by accident if a new helper isn't written carefully to preserve the
  distinction.
- **Private-repo access**: a `duplicate_ref` pointing at a repo the configured
  token can't see returns 404 from GitHub's API (GitHub intentionally
  disguises "exists but no access" as "not found" for security), which will
  misclassify as `ErrInvalidArgument` ("nonexistent ref") when the actual
  problem is a token-scoping/permissions issue. This is a GitHub API behavior,
  not a bug in this codebase, but the `report_duplicate` tool description
  should account for it (e.g. "if you're sure the ref exists, check this
  session's git remote / token has access to that repo") since FR4's
  `ErrInvalidArgument` messaging will otherwise mislead the agent into
  thinking the URL itself is wrong.

## 5a. AC9 — what makes the `in_progress` path "blast-radius-sensitive," and does generalization risk it

`server/mcp/tools_backlog_test.go` has five `TestRequestReview_*` tests
([lines 544-825](../../../server/mcp/tools_backlog_test.go#L544)):
`TransitionsItemToReview`, `TransitionsDirectlyToDone_When_SkipReviewGateEnabled`,
`PersistsVerificationNotesOnWorkSession`, `RejectsVerificationNotesOver4000Chars`,
`RejectsWhenSessionNotLinked`. Every one of them seeds its item with
`Status: string(session.BacklogStatusInProgress)` explicitly and asserts a
specific resulting `Status` (`review`, or `done` for the `SkipReviewGate`
case) via a direct `storage.GetBacklogItem` re-fetch after the call — i.e.
they assert the *concrete outcome status*, not just "no error." This is why
AC9 calls the path "blast-radius-sensitive": these tests encode the exact
`in_progress -> {review|done}` state machine edge, and any implementation
change that alters *how* the target status or precondition is computed (not
just adds a new source status) risks silently changing what these fixed
`in_progress` items transition to.

Why generalization is safe for these specific tests, if done correctly: `item`
is already loaded via `GetBacklogItem` *before* the precondition is built
(line 405), so switching `ExpectedStatus` from the hardcoded constant to
`item.Status` (after the whitelist validation in §0) produces the exact same
value (`"in_progress"`) for every one of these five tests — none of them ever
mutates the item's status between creation and the `requestReview` call, so
`item.Status` observed inside the handler is guaranteed to equal
`BacklogStatusInProgress` for all of them. **The tests passing unmodified is
not incidental — it's a direct consequence of them never exercising a
`pr_pending`-sourced call**, which is exactly why AC9 requires a *new*,
separate test for the `pr_pending` path (FR2) rather than treating "old tests
still green" as proof the `pr_pending` branch is correct. Two concrete ways
an implementation could still break these five tests despite that:
- Changing the target-status decision (`review` vs `done` for
  `SkipReviewGate`) to also depend on the *source* status (e.g. accidentally
  routing `pr_pending`-sourced+`SkipReviewGate` differently than
  `in_progress`-sourced+`SkipReviewGate`) — none of these five tests would
  catch a `pr_pending`-only regression, but a badly-factored shared branch
  could still perturb the `in_progress` case by accident (e.g. an `if
  sourceStatus == pr_pending || item.SkipReviewGate` typo'd as `&&`).
- Changing the success-message text — `TransitionsItemToReview` asserts
  `require.Contains(t, tc.Text, "review")` and
  `TransitionsDirectlyToDone_When_SkipReviewGateEnabled` asserts `Contains(...,
  "SkipReviewGate")`; FR5's new "lands on the next review pass" messaging
  must be scoped to the `report_duplicate`/active-reviewer case only, not
  leak into or replace `request_review`'s existing success strings.

## 5b. `SkipReviewGate` interaction: `report_duplicate` must NOT copy `request_review`'s routing

`request_review` special-cases `SkipReviewGate` by *routing around* the review
gate — it still completes the action, just targets `done` instead of `review`
([tools_backlog.go:394-411](../../../server/mcp/tools_backlog.go#L394),
copied above in §0). **FR6 requires the opposite behavior for
`report_duplicate`: refuse entirely, zero mutation, when
`SkipReviewGate==true`.** These are not the same shape of special-case, and
copy-pasting `request_review`'s pattern is the single most likely
implementation mistake here:

```go
// WRONG — mirrors request_review's routing, violates FR6
if item.SkipReviewGate {
    targetStatus = session.BacklogStatusDone   // report_duplicate must NOT do this
}
```
```go
// RIGHT — FR6: refuse outright, no transition at all
if item.SkipReviewGate {
    return errResult(ErrInvalidArgument,
        "report_duplicate is unavailable for items with SkipReviewGate enabled — "+
        "use request_review instead.", ""), nil
}
```

The rationale check: FR3 says `report_duplicate` must always route to
`review` ("never done/archived directly"). If `SkipReviewGate` were honored
the same way `request_review` honors it, the only alternative to violating
"always route to review" would be to send it to `done` directly (same
contradiction `request_review` resolves by *allowing* the done-shortcut) —
but FR6 already resolves this differently for `report_duplicate` by refusing
outright instead. This makes sense semantically too: `SkipReviewGate` means
"this item's own author/type is trusted to self-certify completion via
`request_review`," which is a different trust decision than "an agent
believes this work is a duplicate and wants a *human* to confirm and close
it" — forcing that judgment call through `request_review`'s existing
(reviewed) path for `SkipReviewGate` items, rather than inventing a second
bypass, is the safer default and is what FR6 already specifies.

**Ordering matters for "zero mutation, every path" (FR6):** the
`SkipReviewGate` check (and the role/session-link checks) must run *before*
the GitHub verification call (FR3's "verified... BEFORE any state
mutation"), not just before the DB transition. Verification itself is a
read-only network call, not a mutation, so doing it before an early-refuse
check isn't a correctness bug — but it does needlessly burn a GitHub API call
(and, for private/rate-limited repos, meaningfully increases the chance of
hitting a transient failure) for a request that was always going to be
refused. Order the four FR6 refusal conditions (`SkipReviewGate`, non-work
role, session not linked, presumably also "item not in an allowed source
status" per §0) ahead of the `duplicate_ref` GitHub verification call, mirroring
how `requestReview` already checks session-linkage and worktree-dirtiness
before doing any state work.

## 5c. Concurrent `report_duplicate` vs `report_pr_created` racing the same item

Two work sessions (or one session calling both tools in quick succession, e.g.
after realizing a "new" PR is actually a dup) could race
`report_duplicate`'s `in_progress/pr_pending -> review` transition against
`report_pr_created`'s `in_progress -> pr_pending` transition
([tools_backlog.go:623-720](../../../server/mcp/tools_backlog.go#L623)) for
the *same item ID*. Per §1, `TransitionBacklogItemStatus`'s atomic
conditional `UPDATE` means exactly one of the two wins — the loser's
`ExpectedStatus` no longer matches the row and it gets `ErrPreconditionFailed`
(which, per §1's other finding, is not currently distinguished from a generic
internal error — same fix applies here: the loser should get a "this item was
already resolved by another action" message, not a "retry" message, since
retrying will just lose again). No double-apply, no torn state — but this is
a place where **not** treating `ErrPreconditionFailed` specially would be
particularly confusing: two agents both believing they made forward progress,
one getting a nonsensical "internal error, please retry" for an action that
in fact can never succeed now that the item moved to `pr_pending`/`review` via
the other path. Worth an explicit test: `report_duplicate` racing
`report_pr_created` on the same item asserts exactly one status-event row for
the transition and a non-retryable error surfaced to the loser.

## 5. Audit trail / `TriggeredBy` constant (FR7)

Only two `TriggeredBy` constants exist today —
[session/backlog.go:90-93](../../../session/backlog.go#L90):
`TriggeredByUser = "user"`, `TriggeredBySystem = "system"`. No
`TriggeredByAgent` exists yet; FR7 is correct that it needs to be introduced.
Note `request_review`'s *current* code passes `TriggeredBySystem` (line 415),
not a work-session-attributable value — if FR7's audit intent is "every
self-service transition an agent triggers should read as agent-triggered, not
system-triggered," the generalized `request_review` should switch to the new
`TriggeredByAgent` too, not just the new `report_duplicate` tool, or the audit
trail will inconsistently attribute equivalent actions to two different
"triggers."

## 6. FR10 — stuck-item surfacing for `pr_pending`

Confirmed achievable without new plumbing: `ReconcileStuck` already has a
dedicated stuck-detection path for exactly this status —
[session/backlog_lifecycle.go:2545-2557](../../../session/backlog_lifecycle.go#L2545)
queries items in `pr_pending` and calls
`er.MarkStuck(ctx, item.ID, domain.StuckReasonPRPendingNoPR, BacklogStatusPRPending, ...)`.
An item left at `pr_pending` after a `report_duplicate` verification failure
(FR4's retryable path exhausted, or the agent gives up) will fall into this
existing sweep on the next tick — no new detection code needed, just confirm
the existing `StuckReasonPRPendingNoPR` messaging/UI surfacing reads sensibly
for "verification failed" as well as its original "PR never got created" case,
since it's now reachable via two different root causes.

## 7. Project rule cross-check

- **`.claude/rules/ent-schema-generation.md`** — does not apply. FR8 is
  explicit that this feature must ship with zero schema changes and
  `go build ./...` succeeding without `ent generate`. Confirmed no new
  `BacklogStatus` value or ent field is implied by any FR — `duplicate_ref`/
  `reason` are meant to reuse the existing free-text `verification_notes`
  column, not a new typed field. If a future revision of this plan *does* end
  up wanting typed columns for `duplicate_ref`/`reason` instead of packing
  them into the notes string, that would require the exact
  `--feature sql/upsert` regeneration command from this rule — flag it then,
  not now.
- **`.claude/rules/prefer-go-git-over-subshells.md`** — partially applies.
  `report_duplicate`'s GitHub verification is a network API call, not a local
  git operation, so go-git (which only talks to a local `.git` directory / a
  remote via git's wire protocol) isn't a substitute for the GitHub REST API
  calls this needs. Where it *does* apply: if implementation reuses
  `report_pr_created`'s `h.sessionBranch(ctx, callerUUID)` pattern to resolve
  the caller's own branch for any local context, that's already using the
  existing go-git-backed helper — no new subshell should be introduced for
  anything resolvable locally.
- **`.claude/rules/go-double-checked-locking.md`** — does not directly apply;
  there's no read-lock/write-lock cache pattern here. The closest analog
  (return the CAS'd value, not a re-read) is already correctly handled inside
  `TransitionBacklogItemStatus` itself (returns the reloaded row after a
  *successful* atomic UPDATE, not a stale pre-write snapshot — see
  [line 919-922](../../../session/ent_repository_backlog.go#L919)). No action
  needed in the new tool code, but worth knowing the precedent exists if
  `report_duplicate`'s own handler ever grows a similar
  read-then-conditionally-write shape outside of `TransitionBacklogItemStatus`.

## Summary of concrete risks to carry into planning

1. CAS widening itself is safe (atomic conditional UPDATE) — no new race
   window. The real gap is `ErrPreconditionFailed` not being distinguished
   from other errors in the MCP-layer response for either `request_review` or
   `report_duplicate`.
2. `report_duplicate` needs an explicit idempotency guard mirroring
   `report_pr_created`'s (lines 682-686), plus an explicit decision on
   double-report-with-different-ref (reject vs merge) since
   `UpdateItemSessionVerificationNotes` overwrites rather than appends.
3. `ParseGitHubURL` and the `github/` package have no issue/commit URL
   support today — this is new code, not reuse — and the package is
   internally inconsistent about `gh`-CLI-subprocess vs direct-HTTP GitHub
   access (different auth resolution paths), which the new tool should not
   propagate by copy-pasting whichever helper looks closest for each ref type.
4. **Highest-severity finding**: FR1's "generalize to the actually-observed
   status" must not be implemented as `ExpectedStatus: item.Status` verbatim —
   that makes the CAS trivially self-satisfying for *any* status the item is
   in (`done`, `archived`, etc.), silently defeating the source-status guard.
   Requires an explicit whitelist check (`item.Status ∈
   {in_progress, pr_pending}`, reject otherwise) before the precondition is
   built, for both `request_review` and `report_duplicate` — and a new test
   asserting rejection from a disallowed status, since no existing test
   exercises a call from `done`/`idea`/`archived`.
5. `report_duplicate` must **refuse** outright when `SkipReviewGate==true`
   (FR6) — it must NOT copy `request_review`'s pattern of routing
   `SkipReviewGate` items to `done` instead of `review` (lines 394-411).
   Those are two different special-cases for the same flag; copying the
   wrong one silently completes a duplicate-report as "done" rather than
   refusing it. All FR6 refusal conditions should be checked before the
   GitHub verification call, not just before the DB transition, to avoid
   burning a network call on a request that was always going to be refused.
6. Existing `TestRequestReview_*` tests all seed items at `in_progress` and
   never move status before the call — they pass unmodified under
   generalization only because `item.Status` observed inside the handler
   happens to equal the hardcoded constant they replace, not because the
   `pr_pending` branch (FR2) is exercised at all. AC9 needs a genuinely new
   `pr_pending`-sourced test, not reliance on the old suite staying green as
   evidence the new branch works.
