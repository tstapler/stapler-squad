# Architecture Research: pr-review-followup

Date: 2026-08-02. All line numbers read fresh from the current tree via targeted
`Read`/`Grep`, not copied from any prior doc.

## 0. Critical finding: the codebase has moved past the sibling project's plan

`project_plans/backlog-pr-conflict-detection/` (architecture.md, plan.md,
architecture-review.md) describes a design for adding `HasConflicts` that is
**already fully implemented and further hardened** in the current tree — plus a
generic remediation-backoff gate that did not exist when that plan was written.
Concretely, none of the sibling docs' file:line citations match current code
anymore; the real state is:

- `PRStatus` (`session/git/worktree_git.go:421-467`) already has
  `CIFailing`, `HasBlockingReviews`, `HasConflicts`, plus (beyond the sibling
  plan) `IsClosed`, `IsDraft`, `Mergeable`, `ApprovedCount`,
  `ChangesRequestedCount`, `FeedbackText`, and unexported
  `failedChecks`/`blockingReviews`/`conflictMergeStateStatus`/`generalComments`
  backing a `render()` method (`worktree_git.go:472-524`) — exactly the
  "derive `FeedbackText` from structured fields" remediation the sibling
  project's own architecture-review.md flagged as a Concern, already done.
- `parsePRStatusPayload` (`worktree_git.go:549-645`) is already the pure,
  I/O-free JSON-in/struct-out function the sibling plan's Pattern Decisions
  table called for; `GetPRStatus` (`worktree_git.go:528-544`) is the thin I/O
  wrapper around it, already fetching
  `statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft`.
- `ReconcilePRPending` (`session/backlog_lifecycle.go:3850-4113`) is far
  larger than the sibling docs' 530-585 citation: it now also handles a
  closed-without-merging branch (BUG-036/BUG-040), a "superseded by main"
  short-circuit (BUG-032, `closeIfSupersededByMain`), `pr_ready_unmerged`
  detection, and — the load-bearing addition for this project —
  **`remediatePRFixWithBackoffGate`** (`backlog_lifecycle.go:3777-3845`),
  which wraps every `AutoReopenForPRFix` dispatch in this function with a
  shared, durable, exponential-backoff gate. This did not exist in the
  sibling project's plan; it was added afterward specifically because
  `ReconcilePRPending`'s spawn branches had no backoff at all (see its doc
  comment, `backlog_lifecycle.go:3780-3787`, citing
  `docs/tasks/backlog-feature-improvement.md`'s 2026-07-28 entry).
- `prPendingChecker` (`backlog_lifecycle.go:223-230`) now has a third method,
  `ClosePR`, beyond the sibling plan's two.

**Implication for this project**: the "staleness/dedup mechanism" the
requirements.md scope asks for is *not* being built from nothing — a durable,
per-`(itemID, reason)` backoff/attempt-cap gate (`Storage.RemediationDue`,
`session/backlog_remediation.go`) already exists and already governs every
`AutoReopenForPRFix` dispatch this function makes, via the shared
`domain.StuckReasonPRNeedsFix` reason. §1 below explains precisely what this
existing gate does and does not solve for the new signal, because the two are
easy to conflate and getting that distinction right is the crux of this
project's design.

## 1. Where dedup/staleness state should live

### 1a. What `RemediationDue`/`BacklogStuckState` already gives you for free

`domain.StuckReasonPRNeedsFix` (`session/domain/backlog.go:123-132`) is
raised once per `(item, "pr_needs_fix")` pair the first tick any of
`CIFailing`/`HasBlockingReviews`/`HasConflicts` is true
(`er.MarkStuck(...)`, `backlog_lifecycle.go:3805`), and every subsequent
`AutoReopenForPRFix` dispatch for that pair is gated by
`Storage.RemediationDue` (`session/backlog_remediation.go:168-193`):
attempt 1 fires immediately, then a fixed exponential schedule
(`remediationBackoffSchedule`, `backlog_remediation.go:31-37`: 30m, 2h, 8h,
24h, 72h) gates attempts 2-5, and attempt 5 "parks" the row
(`remediation_attempts >= MaxRemediationAttempts`, `= 5`,
`backlog_remediation.go:45`) — no further automated attempts until an
operator resets it, but the row stays open and visible on `/unfinished`.

This **already prevents** "spawn a fresh fix session on every ~60s tick
forever" for *any* trigger routed through `remediatePRFixWithBackoffGate`,
including a new COMMENTED-review/comment signal if it's wired through the
same function (which the constraints require — see §2). So the literal
"60s tick re-triggers forever" framing in requirements.md's problem
statement is already partially addressed at the infrastructure level, for
free, by reusing the existing call.

### 1b. What it does NOT give you — the actual gap this project must close

`RemediationDue` is purely **time-based**, not **content-based**. It answers
"has enough wall-clock time passed since the last attempt for this
`(item, reason)` pair" — it has no concept of "is the underlying feedback
still the same feedback we already tried to address, or has something new
arrived." Compare how the three existing signals behave once a fix session
runs:

- `CIFailing` — self-clearing. CI reruns on push; either it now passes
  (signal clears, `resolveStuckLogged` fires,
  `backlog_lifecycle.go:4077`) or it still fails (same signal, gated by
  backoff, eventually parks).
- `HasConflicts` — self-clearing. `mergeable`/`mergeStateStatus` are
  recomputed by GitHub after every push; a successful rebase clears it.
- `HasBlockingReviews` (`CHANGES_REQUESTED`) — self-clearing **in practice**,
  not by this app's code but by a GitHub repo setting: "Dismiss stale pull
  request approvals when new commits are pushed" (a branch-protection
  option) dismisses *all* review decisions, including `CHANGES_REQUESTED`,
  on every push. If that setting is off, this signal has the exact same
  staleness problem described below for comments — worth flagging, but out
  of this project's stated scope (requirements.md explicitly says not to
  redefine `HasBlockingReviews`).
- **`COMMENTED`-state reviews and plain PR comments do not self-clear at
  all.** GitHub's "dismiss stale reviews" setting only dismisses reviews
  that carry a decision (`APPROVED`/`CHANGES_REQUESTED`); a `COMMENTED`
  review has no decision to dismiss and simply stays in
  `gh pr view --json reviews`'s list forever, unchanged, regardless of how
  many commits get pushed afterward. Plain issue-level comments
  (`--json comments`) never disappear either. So even with
  `RemediationDue`'s backoff in place, a fix session that successfully
  addresses a Copilot `COMMENTED` review would still see that exact same
  review on the next eligible tick (30m, then 2h, then 8h...) and — because
  `parsePRStatusPayload` has no way to know it was already handled — would
  re-mark-stuck (a no-op, same row) and eventually re-spawn a fix session
  for feedback that's already been addressed, for up to 5 attempts spanning
  over a week before parking. That's bounded, but wasteful and confusing
  (an operator watching `/unfinished` would see repeated "PR needs
  attention" cycles for a concern that was already resolved).

This is the real gap: **content-based dedup**, layered on top of (not
replacing) the existing time-based backoff.

### 1c. Recommendation: a single per-item watermark timestamp, not per-comment IDs

Reject tracking individual comment/review IDs (e.g. a JSON list of "seen"
IDs on the item or in `BacklogStuckState.context`). A single item has at
most one open PR at a time (`item.PrNumber`/`item.PrURL`, cleared and
replaced wholesale when a PR closes without merging —
`backlog_lifecycle.go:4038-4044`), and every review/comment GitHub returns
carries a timestamp (`reviews[].submittedAt`, `comments[].createdAt` — not
currently fetched, see §2). A single "high-water mark" timestamp is
sufficient and much simpler than an ID set: "have we dispatched a fix
attempt covering all feedback up to time T" needs only one comparison
(`latestFeedbackAt.After(watermark)`), no set membership, no unbounded
growth, and no need to prune IDs for comments that get deleted.

**Add a new field to `session/ent/schema/backlog_item.go`**, following the
existing `shipped_snapshot_at time.Time optional/nillable` naming/shape
convention already on this schema (`backlog_item.go:98-101`):

```go
field.Time("pr_feedback_addressed_at").
    Optional().
    Nillable().
    Comment("High-water mark: the timestamp of the newest COMMENTED-review/" +
        "comment feedback a fix session has already been dispatched for. " +
        "GitHub never clears COMMENTED reviews or plain comments on push " +
        "(unlike CHANGES_REQUESTED, which stale-review-dismiss settings can " +
        "clear), so without this watermark the same already-addressed " +
        "feedback would re-trigger a fix session every time the shared " +
        "pr_needs_fix remediation backoff (RemediationDue) becomes eligible " +
        "again. Nil means no feedback-triggered fix has been dispatched yet " +
        "for the item's current PR. Cleared whenever pr_url/pr_number are " +
        "cleared (a closed-without-merging PR gets a fresh PR later, whose " +
        "feedback timeline restarts) — see the pr_pending_no_pr clearing " +
        "branch in ReconcilePRPending."),
```

Regenerate with the exact command from `session/ent/generate.go`
(`--feature sql/upsert` is required — omitting it silently breaks
`UpsertRule`-style methods per `.claude/rules/ent-schema-generation.md`):

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

Then thread the field through `BacklogItemData` (read model) and
`BacklogItemUpdate` (write model) — both already exist for every other
`backlog_item` column and follow a mechanical, repeatable pattern (mirror
`ShippedSnapshotAt *time.Time` end-to-end: struct field, `backlogItemToData`
mapping, `UpdateBacklogItem` setter branch). No new table, no new ent
schema file — this is one field on the existing `BacklogItem` entity,
matching Concern-free precedent (`shipped_snapshot_at`,
`plan_approved_at`, `queued_at` are all single nillable timestamp
watermarks already on this exact schema for structurally similar
"when did X last happen" purposes).

**Why not `BacklogStuckState.context`** (the field that already holds
`fixCtx`, e.g. "PR #148 needs fixes:\n\n..."): it's documented as a
"human-readable 'why' string" (`backlog_stuck_state.go:56-58`), it's
shared across all three(→four) trigger reasons folded into the single
`pr_needs_fix` row, and repurposing a human-readable free-text field to
also carry a machine-parsed dedup timestamp mixes concerns and would
require parsing/reserializing structured data out of prose on every read —
a plain `time.Time` column on `backlog_item` is direct and matches the
existing pattern.

## 2. Integration points and exact order of changes

### 2a. `session/git/worktree_git.go` — extend `PRStatus`/payload/`render()`

1. **No `--json` field-list change needed.** Verified live (`gh pr view 5
   --repo cli/cli --json reviews --jq '.reviews[0]'` →
   `{"...,"state":"APPROVED","submittedAt":"2019-10-09T20:13:02Z"}`; `gh pr
   view 1 --repo cli/cli --json comments --jq '.comments[0]'` →
   `{"author":{"login":"..."},"createdAt":"2026-03-02T21:43:46Z",...}`):
   `submittedAt` (reviews) and `createdAt` (comments) are already returned
   as sub-fields whenever the parent `reviews`/`comments` top-level field is
   requested — both already are, at `worktree_git.go:536`. Only the local Go
   struct needs new fields to capture them (next step); the `gh` command
   itself is unchanged.
2. **Extend the anonymous `payload` struct** (`worktree_git.go:561-577`):
   add `SubmittedAt string` to the `Reviews` element and `CreatedAt string`
   to the `Comments` element (both RFC3339 strings from `gh`, parsed with
   `time.Parse(time.RFC3339, ...)`).
3. **Capture `COMMENTED` reviews** — today's review loop
   (`worktree_git.go:627-636`) only branches on `CHANGES_REQUESTED` and
   `APPROVED`; a `COMMENTED` review is silently dropped, never reaching
   `FeedbackText` at all. This is the literal gap requirements.md names:
   Copilot's typical review posture is invisible to the fix agent today,
   not just under-triggered. Add a third case capturing `{author, body,
   submittedAt}` into a new unexported slice, e.g. `commentReviews []reviewInfo`
   (reuse the existing `reviewInfo` shape) or extend it with a timestamp
   field.
4. **New `PRStatus` fields** (additive, alongside the existing four bools —
   do NOT touch `HasBlockingReviews`'s `CHANGES_REQUESTED`-only meaning,
   per the explicit constraint):
   ```go
   // HasReviewFeedback is true when there is at least one COMMENTED-state
   // review or plain PR comment — signals worth surfacing to a fix agent
   // even though they don't block merge the way HasBlockingReviews does.
   HasReviewFeedback bool
   // LatestFeedbackAt is the newest submittedAt/createdAt across all
   // COMMENTED reviews and comments captured this call. Zero value (time.Time{})
   // when HasReviewFeedback is false. Callers (ReconcilePRPending) compare
   // this against a durable per-item watermark to decide whether this
   // feedback has already been dispatched for — GetPRStatus itself has no
   // DB access and cannot make that call.
   LatestFeedbackAt time.Time
   ```
5. **`render()`** (`worktree_git.go:472-524`): add a new section — e.g.
   `## Reviewer comments` — rendering `commentReviews` (author + body) and
   keep the existing `generalComments`/`## PR comments` section for plain
   issue comments as-is. Ordering: keep conflict-first (existing
   convention, `render()`'s doc comment); place the new section after the
   existing `## Review: changes requested` block and before/with
   `## PR comments` — no functional requirement on exact position since
   nothing downstream parses `FeedbackText` structurally, only a fix agent
   reads it.
6. **`parsePRStatusPayload`** (`worktree_git.go:549-645`): after the
   existing review/comment loops, compute `HasReviewFeedback` and
   `LatestFeedbackAt` from whichever of `commentReviews`/`generalComments`
   (with their now-captured timestamps) is non-empty.

This is a self-contained, additive change to one file — the same shape as
every prior signal addition to this struct.

### 2b. `session/backlog_lifecycle.go` — `ReconcilePRPending`, gate + watermark write

1. **Gate extension** (`backlog_lifecycle.go:4048`): currently
   `if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts { ... continue }`.
   Add a fourth disjunct computed from the new watermark comparison — but
   this comparison needs `item`'s stored `PRFeedbackAddressedAt`, which
   `prStatus` cannot know (it's DB state, `GetPRStatus` is pure I/O). Add a
   local:
   ```go
   hasNewFeedback := prStatus.HasReviewFeedback &&
       (item.PRFeedbackAddressedAt == nil || prStatus.LatestFeedbackAt.After(*item.PRFeedbackAddressedAt))
   ```
   and extend the gate to
   `if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts && !hasNewFeedback { ... }`.
2. **`fixCtx` construction** (`backlog_lifecycle.go:4106`) needs no change
   — it already interpolates `prStatus.FeedbackText`, which now carries the
   new `## Reviewer comments` section from §2a whenever `HasReviewFeedback`
   is true. Same "signal-agnostic string pipe" property the sibling
   project's architecture.md §4 already established for the other three
   signals.
3. **Log line** (`backlog_lifecycle.go:4107-4108`): add a fourth `%v` —
   `"... (CI=%v, reviews=%v, conflict=%v, feedback=%v)"` — passing
   `hasNewFeedback` (not `prStatus.HasReviewFeedback`, which would spuriously
   log `true` for already-addressed feedback that isn't actually
   triggering anything this tick).
4. **Watermark write** — the critical "how does the loop learn a fix was
   dispatched" step (see §3 for the full trace). Immediately after the
   existing `remediatePRFixWithBackoffGate` call
   (`backlog_lifecycle.go:4109`), if `hasNewFeedback` was true for this
   tick AND the dispatch actually happened (mirrors the BUG-040 lesson at
   `backlog_lifecycle.go:4016-4024` — only commit a state-clearing/advancing
   write once an action is *confirmed*, not blindly), persist the new
   watermark:
   ```go
   attempted, fixErr := l.remediatePRFixWithBackoffGate(ctx, er, fixSpawner, item.ID.String(), item.Title, fixCtx)
   if fixErr != nil {
       log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending AutoReopenForPRFix item=%s: %v", item.ID, fixErr)
   } else if attempted && hasNewFeedback {
       ts := prStatus.LatestFeedbackAt
       if _, updateErr := l.storage.UpdateBacklogItem(ctx, item.ID.String(), BacklogItemUpdate{
           PRFeedbackAddressedAt: &ts,
       }, nil); updateErr != nil {
           log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending persist PRFeedbackAddressedAt item=%s: %v", item.ID, updateErr)
       }
   }
   ```
   `attempted` (not just `fixErr == nil`) matters: `remediatePRFixWithBackoffGate`
   returns `attempted=false` whenever the backoff gate isn't due yet
   (`backlog_lifecycle.go:3839-3842`) — in that case nothing was dispatched
   this tick and the watermark must not advance, or a feedback item could
   be silently marked "addressed" without any fix session ever having run
   for it.
5. **Clear the watermark** alongside the existing PR-field clear in the
   closed-without-merging branch (`backlog_lifecycle.go:4038-4044`, the
   `UpdateBacklogItem{PrURL: &emptyURL, PrNumber: &zeroNum}` call) — add
   `PRFeedbackAddressedAt: nil` (ent's typed nil-clear pattern, matching
   how other nillable fields are cleared elsewhere in this file) so a
   fresh PR after a close-without-merging cycle starts with a clean
   watermark rather than inheriting the old PR's feedback timeline.
6. **`resolveStuckLogged`** in the healthy branch
   (`backlog_lifecycle.go:4077`) needs no change — once `hasNewFeedback`
   is false (because the watermark caught up) alongside the three existing
   signals, the item already falls into the existing "healthy" branch and
   `StuckReasonPRNeedsFix` resolves exactly as today.

### 2c. Order of implementation

1. `worktree_git.go` changes first (§2a) — `PRStatus`/`parsePRStatusPayload`/
   `render()` are pure and independently unit-testable with raw JSON, no
   DB, no `ReconcilePRPending` dependency (same testability rationale the
   sibling project's Pattern Decisions table already established for this
   function).
2. `backlog_item.go` ent schema field + regenerate (§1c) — independent of
   §2a, can happen in parallel.
3. `backlog_lifecycle.go` gate/watermark wiring (§2b) — depends on both:
   needs `PRStatus.HasReviewFeedback`/`LatestFeedbackAt` to exist (from
   step 1) and `BacklogItemData.PRFeedbackAddressedAt`/
   `BacklogItemUpdate.PRFeedbackAddressedAt` to exist (from step 2).

## 3. Full data-flow trace: COMMENTED review → dedup → re-trigger

```
Copilot posts a COMMENTED review on PR #152
  │
  ▼ (next ~60s ReconcilePRPending tick)
g.GetPRStatus(152) → gh pr view --json ...,reviews,comments,...
  │  parsePRStatusPayload: review.State == "COMMENTED" → captured into
  │  commentReviews with submittedAt; HasReviewFeedback=true,
  │  LatestFeedbackAt = that submittedAt (worktree_git.go, §2a)
  ▼
ReconcilePRPending: item.PRFeedbackAddressedAt == nil (first time)
  → hasNewFeedback = true → gate falls through (§2b.1)
  ▼
remediatePRFixWithBackoffGate(ctx, er, fixSpawner, itemID, title, fixCtx)
  │  MarkStuck opens/refreshes the SAME pr_needs_fix row used for CI/
  │  review/conflict (backlog_lifecycle.go:3805) — no new StuckReason
  │  RemediationDue: no open row existed before MarkStuck this tick in
  │  this scenario → attempt 1 fires immediately (backlog_remediation.go:174)
  ▼
fixSpawner.AutoReopenForPRFix(ctx, itemID, fixCtx) → pr_pending → in_progress
  │  fixCtx already contains the new "## Reviewer comments" section
  │  (render(), §2a.5) with Copilot's comment body
  ▼
attempted=true, fixErr=nil → ReconcilePRPending persists
  item.PRFeedbackAddressedAt = LatestFeedbackAt (§2b.4)
  ▼
Work session runs, (hopefully) addresses the feedback, pushes, item cycles
  through review → review-gate → pr_pending again (same lifecycle every
  other trigger already uses — no new merge/review path, per the sibling
  project's already-established human-review requirement)
  ▼ (next ReconcilePRPending tick after the item is back in pr_pending)
g.GetPRStatus(152) → the SAME COMMENTED review is still present (GitHub
  never clears it) → HasReviewFeedback=true again, LatestFeedbackAt ==
  the same submittedAt as before (no new review/comment arrived)
  ▼
ReconcilePRPending: prStatus.LatestFeedbackAt.After(*item.PRFeedbackAddressedAt)
  == false (equal, not after) → hasNewFeedback = false
  → if CI/reviews/conflicts are also healthy, falls into the existing
  "PR is open and healthy" branch (backlog_lifecycle.go:4048) →
  resolveStuckLogged clears pr_needs_fix → item stays pr_pending, no
  further spawns for this feedback. THIS is the "how does it signal done
  back to the dedup state" answer requirements.md's research question 3
  asks for: not an explicit signal from the work session, but a
  before/after comparison against the SAME feedback re-fetched from
  GitHub next tick, gated by a watermark set at successful-dispatch time.
  ▼ (weeks later) A human — or Copilot again — leaves a NEW comment
prStatus.LatestFeedbackAt (the new comment's timestamp) is now After
  item.PRFeedbackAddressedAt (the old watermark) → hasNewFeedback = true
  again → the cycle repeats from the top, MarkStuck reopens the row
  in-place (backlog_stuck_state.go's documented reopen-in-place model),
  RemediationDue treats it as attempt 1 again? — NO: MarkStuck's
  reopen-in-place path only clears resolved_at/notified_at/
  first_detected_at (ent_repository_backlog.go:1090-1101); it does NOT
  reset remediation_attempts. Confirm this is the desired behavior before
  implementing: if the row was previously resolved (healthy branch cleared
  it) rather than parked, remediation_attempts naturally never reached the
  cap and a fresh detection cycle proceeds normally on the existing
  schedule. If it HAD parked (5 attempts exhausted on an unrelated CI
  flap, say) and then resolved, a new feedback event reopening the same
  row would inherit the parked attempt count with no automated retries
  left — an edge case worth a note in the implementation plan, not a
  blocker for this research doc, since it's a pre-existing property of
  the shared reason/row design (not something this project introduces).
```

**Where new data flows, precisely**: `LatestFeedbackAt`/`HasReviewFeedback`
travel from `parsePRStatusPayload` (pure) into `ReconcilePRPending` (the
only place with DB access to `item.PRFeedbackAddressedAt`), which computes
`hasNewFeedback` and, on confirmed dispatch, writes the new watermark back
via `UpdateBacklogItem`. Nothing else in the pipeline
(`remediatePRFixWithBackoffGate`, `AutoReopenForPRFix`, `SpawnSessionFromItem`,
`BuildSessionInitialPrompt`) needs to know the dedup mechanism exists —
identical to the sibling project's finding for `HasConflicts` (architecture.md
§4): the entire feature's novel data-flow work is contained in
`GetPRStatus`/`parsePRStatusPayload` (§2a) and `ReconcilePRPending`'s gate +
one new watermark write (§2b).

## 4. Copilot reviewer request wiring

Requirements.md scope: "Wire a Copilot review request... into the ship flow
that creates PRs." The ship flow's PR-creation call is
`pushAndCreatePR` (`session/backlog_lifecycle.go:3158-...`), which calls
`g.CreatePR(prTitle, prBody)` (`backlog_lifecycle.go:3268`) — implemented by
`(*GitWorktree).CreatePR` (`session/git/worktree_git.go:328-391`) — then, on
success, `g.EnablePRAutoMerge(prNumber)` as an independent best-effort step
(`backlog_lifecycle.go:3304-3310`, logs a warning + sends a notification on
failure, does not fail the overall flow).

**Recommendation**: add a new method mirroring `EnablePRAutoMerge`'s exact
shape — `RequestCopilotReview(prNumber int) error` on `*GitWorktree`
(`session/git/worktree_git.go`), using `gh pr edit <num> --add-reviewer
@copilot` (simpler and more failure-isolated than folding `--reviewer` into
the single `gh pr create` invocation in `CreatePR`, since a reviewer-request
failure — e.g. Copilot code review not enabled for the repo/org — must not
fail PR creation itself, the same reasoning `EnablePRAutoMerge` already
encodes for its own best-effort failure mode). Add it to the `prCreator`
interface (`backlog_lifecycle.go:235-240`) alongside `EnablePRAutoMerge`,
call it in `pushAndCreatePR` right after the existing `EnablePRAutoMerge`
call (or before — order between the two doesn't matter, they're
independent), with the same log-warning + notify-on-failure pattern already
established at `backlog_lifecycle.go:3304-3310`.

**Reviewer identifier — version-sensitive, needs a runtime check.** Per
GitHub's own changelog ([Request Copilot code review from GitHub CLI,
2026-03-11](https://github.blog/changelog/2026-03-11-request-copilot-code-review-from-github-cli/)),
`gh pr edit --add-reviewer @copilot` is the current, documented, supported
syntax — but only as of **gh CLI 2.88.0** (shipped March 2026). Before that
release, the same result required the undocumented literal login
`copilot-pull-request-reviewer[bot]`. This matters because the machine this
research was done on has **gh 2.86.0** installed (verified:
`gh --version` → `gh version 2.86.0 (2026-01-21)`), which predates 2.88.0 —
its own `gh pr edit --help` output confirms `--add-reviewer`/
`--remove-reviewer` explicitly do *not* support the `@copilot`/`@me` special
values on this version (only `--add-assignee` does, for assigning Copilot
as a coding agent — a different feature from requesting a review). Two
implications for implementation: (1) verify the `gh` version actually
installed wherever `stapler-squad` runs in production before hardcoding
`@copilot` — if it's older than 2.88.0, fall back to the literal
`copilot-pull-request-reviewer[bot]` login instead (same command shape,
different argument); (2) either way, this should be a constant, not
user-configurable, unless a future opt-out need surfaces — no existing
per-item or per-repo config plumbing exists for this today, and
requirements.md doesn't ask for one.

This wiring is independent of §1-§3 — it's the "produce the feedback in the
first place" half, not the "detect and dedup it" half — but they're
naturally sequenced together: without this, `HasReviewFeedback` may rarely
trigger organically since nothing currently prompts Copilot to review.

## 5. Event-Command-Policy table assessment

**Not warranted — skip it**, for the same reason the sibling
`backlog-pr-conflict-detection` project's architecture.md concluded (§ opening
line: "a straightforward polling-loop extension, not a multi-actor domain").
This project is, if anything, an even simpler case: one new boolean-shaped
signal added to an existing struct, one new comparison against one new
per-item timestamp field, reusing the exact same detect → gate → spawn
control flow four signals now share. There is no new actor, no new command
being issued (the "command" — spawn a fix session via `AutoReopenForPRFix`
— already exists and is reused verbatim), and no branching business
process beyond a single additional `&&` term in an existing `if`. An
Event-Command-Policy table would document a single row
("NewFeedbackDetected" event → "SpawnPRFixSession" command → policy:
"if hasNewFeedback and backoff due") that adds no clarity beyond the plain-
English gate condition already in the code. Confirmed via the same
"Transaction Script over a flat struct is the right level of complexity"
conclusion the sibling project's architecture-review.md reached (Lens 3,
Q8) — nothing here changes that assessment.

## 6. Summary of exact edits required

| File | Location | Change |
|---|---|---|
| `session/git/worktree_git.go` | `payload` struct's `Reviews`/`Comments` elements | add `SubmittedAt`/`CreatedAt` string fields |
| `session/git/worktree_git.go` | line 536 | no change — `reviews`/`comments` already requested; `submittedAt`/`createdAt` are sub-fields returned automatically (verified live against `cli/cli`) |
| `session/git/worktree_git.go` | review loop, ~627-636 | add `COMMENTED` case capturing `{author, body, submittedAt}` into a new unexported slice |
| `session/git/worktree_git.go` | `PRStatus` struct, 421-467 | add `HasReviewFeedback bool`, `LatestFeedbackAt time.Time` |
| `session/git/worktree_git.go` | `render()`, 472-524 | new `## Reviewer comments` section from the new slice |
| `session/git/worktree_git.go` | `parsePRStatusPayload`, 549-645 | set the two new fields from the new slice |
| `session/ent/schema/backlog_item.go` | Fields(), after `shipped_snapshot_at` (~101) | add `pr_feedback_addressed_at time.Time optional/nillable` |
| — | — | `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` |
| `session/storage_backlog.go` (or wherever `BacklogItemData`/`BacklogItemUpdate` live) | struct defs + `backlogItemToData` + `UpdateBacklogItem` | thread `PRFeedbackAddressedAt *time.Time` through, mirroring `ShippedSnapshotAt` |
| `session/backlog_lifecycle.go` | line ~4048 (gate) | add `&& !hasNewFeedback`, compute `hasNewFeedback` from `prStatus.HasReviewFeedback` + item watermark comparison |
| `session/backlog_lifecycle.go` | log line ~4107-4108 | add `feedback=%v` (pass `hasNewFeedback`) |
| `session/backlog_lifecycle.go` | after `remediatePRFixWithBackoffGate` call ~4109 | on `attempted && hasNewFeedback && fixErr == nil`, persist `PRFeedbackAddressedAt = prStatus.LatestFeedbackAt` |
| `session/backlog_lifecycle.go` | closed-PR-fields clear, ~4038-4044 | also clear `PRFeedbackAddressedAt` |
| `session/git/worktree_git.go` | new method near `EnablePRAutoMerge` (~650-663) | `RequestCopilotReview(prNumber int) error` via `gh pr edit <num> --add-reviewer copilot-pull-request-reviewer[bot]` |
| `session/backlog_lifecycle.go` | `prCreator` interface, 235-240 | add `RequestCopilotReview(prNumber int) error` |
| `session/backlog_lifecycle.go` | `pushAndCreatePR`, near 3304-3310 | call it best-effort after `EnablePRAutoMerge`, same log+notify pattern |

No changes needed to: `PRFixSpawner` interface, `AutoReopenForPRFix`,
`maxAutoReworkIterations`/rework-cap logic (shared by construction, same as
the sibling project found), `StuckReasonPRNeedsFix`/`StuckReasonReworkCap`
definitions (reused as-is), `session/pr_status_poller.go`/
`session/worktree_pr_poller.go` (confirmed present in the tree but still a
separate poller for a different UI badge, untouched by this data flow,
same as the sibling project's finding).
