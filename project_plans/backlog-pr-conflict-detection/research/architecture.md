# Architecture Research: backlog-pr-conflict-detection

Agent 3 (Architecture). Scope per requirements.md: extend the existing
CI-failure/review-comment reconciliation loop with a third signal — merge
conflicts — reusing `AutoReopenForPRFix` end-to-end. This is a straightforward
polling-loop extension, not a multi-actor domain; no EventStorming table.

All line numbers below were read fresh from the current tree (2026-07-12) via
targeted `Read`/`Grep`, not copied from `project_plans/backlog-service-refactor/research/architecture.md`
(confirmed stale by the task brief).

## 1. `PRStatus` struct and `GetPRStatus` — `session/git/worktree_git.go`

```
326  // PRStatus holds the CI and review state for a pull request.
327  type PRStatus struct {
328      // CIFailing is true when at least one CI check has a terminal failure.
329      CIFailing bool
330      // HasBlockingReviews is true when a reviewer has requested changes.
331      HasBlockingReviews bool
332      // FeedbackText is a combined human-readable summary for the fix agent.
333      FeedbackText string
334  }
335
336  // GetPRStatus fetches the combined CI check status, reviewer decisions, and
337  // PR comments for the given pull request number.
338  func (g *GitWorktree) GetPRStatus(prNumber int) (*PRStatus, error) {
...
345      cmd := safeexec.CommandContext(ctx, "gh", "pr", "view", strconv.Itoa(prNumber),
346          "--json", "statusCheckRollup,reviews,comments")
...
438  }
```

`GetPRStatus` spans lines 338–438 exactly (closing brace at 438, matching
requirements.md's citation). The anonymous `payload` struct (353–377) decodes
only `statusCheckRollup`, `reviews`, `comments` — `mergeable` and
`mergeStateStatus` are simply absent from both the `--json` field list (346)
and the `payload` struct.

**Proposed change** — add to `payload` (alongside the existing three
top-level fields):

```go
Mergeable      string `json:"mergeable"`      // "MERGEABLE" | "CONFLICTING" | "UNKNOWN"
MergeStateStatus string `json:"mergeStateStatus"` // "CLEAN" | "DIRTY" | "BLOCKED" | "BEHIND" | "UNSTABLE" | "DRAFT" | "HAS_HOOKS" | "UNKNOWN"
```

and extend the `--json` flag to
`"statusCheckRollup,reviews,comments,mergeable,mergeStateStatus"`.

**Proposed `PRStatus` field**, following the existing bool-per-signal shape
(`CIFailing`, `HasBlockingReviews`) rather than introducing a differently-shaped
type:

```go
type PRStatus struct {
    CIFailing          bool
    HasBlockingReviews bool
    HasConflicts       bool   // new
    FeedbackText       string
}
```

**Detection logic** (added after the reviews loop, before `FeedbackText` is
finalized, ~line 426):

```go
// Evaluate mergeability. GitHub computes mergeable/mergeStateStatus
// asynchronously; a transient "UNKNOWN" must NOT be treated as a conflict
// (Rabbit Hole in requirements.md) — only an explicit CONFLICTING/DIRTY
// state is actionable.
if strings.ToUpper(payload.Mergeable) == "CONFLICTING" {
    status.HasConflicts = true
    sb.WriteString("## Merge conflict\n")
    sb.WriteString(fmt.Sprintf("PR is not mergeable against its base branch (mergeStateStatus=%s). "+
        "Rebase onto the base branch and resolve conflicts.\n\n", payload.MergeStateStatus))
}
```

Use `payload.Mergeable == "CONFLICTING"` as the trigger condition rather than
`MergeStateStatus`, because `mergeable` is GitHub's explicit two/three-state
verdict on git-level conflicts, while `mergeStateStatus` also fires for
unrelated blocking conditions (`BLOCKED` = required review/status check
missing, `BEHIND` = fast-forward-only branch protection, `DRAFT`) that are
not "conflicts" and must not spawn a conflict-fix session. `UNKNOWN`
(either field) falls through to "no signal this cycle" by construction —
the `if` simply doesn't match, so `status.HasConflicts` stays `false` and
`ReconcilePRPending` treats it as healthy for this poll and re-checks next
cycle. No special-case code is needed for the transient-`UNKNOWN` rabbit
hole; it falls out of using strict equality against `"CONFLICTING"`.

## 2. `ReconcilePRPending` — `session/backlog_lifecycle.go`

```
527  // ReconcilePRPending polls items in pr_pending status. It transitions to done
528  // when the PR is merged, and spawns a fix session when CI fails or reviewers
529  // request changes.
530  func (l *BacklogLifecycleListener) ReconcilePRPending(ctx context.Context, er *EntRepository) {
...
562      // 2. PR still open — check CI status and reviews.
563      prStatus, statusErr := g.GetPRStatus(item.PrNumber)
564      if statusErr != nil {
565          log.DebugLog.Printf("[BacklogLifecycle] ReconcilePRPending GetPRStatus item=%s pr=%d: %v", item.ID, item.PrNumber, statusErr)
566          continue
567      }
568      if !prStatus.CIFailing && !prStatus.HasBlockingReviews {
569          continue // PR is open and healthy — wait for merge.
570      }
571
572      // 3. CI failure or review changes requested → spawn fix session.
573      fixSpawner := l.getPRFixSpawner()
574      if fixSpawner == nil {
575          log.WarningLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s: CI/review issues found but no PRFixSpawner configured", item.ID)
576          continue
577      }
578      fixCtx := fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", item.PrNumber, item.PrURL, prStatus.FeedbackText)
579      log.InfoLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s → in_progress for PR fix (CI=%v, reviews=%v)",
580          item.ID, prStatus.CIFailing, prStatus.HasBlockingReviews)
581      if fixErr := fixSpawner.AutoReopenForPRFix(ctx, item.ID.String(), fixCtx); fixErr != nil {
582          log.ErrorLog.Printf("[BacklogLifecycle] ReconcilePRPending AutoReopenForPRFix item=%s: %v", item.ID, fixErr)
583      }
584  }
585  }
```

`ReconcilePRPending` spans 530–585 exactly (closing brace at 585, matching
requirements.md's citation).

**Gate change (line 568)** — add the third disjunct:

```go
if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts {
    continue // PR is open and healthy — wait for merge.
}
```

This is the entire gate change. Because `FeedbackText` (line 578) already
contains the "## Merge conflict" section built in §1 whenever `HasConflicts`
is true, `fixCtx` at line 578 requires **no changes** — it already
concatenates whichever of the three `## ...` sections `GetPRStatus` populated
into one `FeedbackText` blob. The existing `fmt.Sprintf("PR #%d (%s) needs
fixes:\n\n%s", ...)` wrapper is signal-agnostic by design.

**Log line change (line 579–580)** — extend to include the new signal, per
requirements.md's Observability Requirements ("include ... which signal
(conflict vs. CI vs. review) triggered the spawn"):

```go
log.InfoLog.Printf("[BacklogLifecycle] ReconcilePRPending item=%s → in_progress for PR fix (CI=%v, reviews=%v, conflict=%v)",
    item.ID, prStatus.CIFailing, prStatus.HasBlockingReviews, prStatus.HasConflicts)
```

This satisfies the observability requirement without new logging
infrastructure — same `log.InfoLog` call, one more `%v`.

No other line in `ReconcilePRPending` needs to change. Step 1 (merged check,
546–560) and the `AutoReopenForPRFix` call itself (581) are signal-agnostic
already.

## 3. `AutoReopenForPRFix` and the iteration cap — `server/services/backlog_service_triage.go`

```
34  // maxAutoReworkIterations caps how many automated work sessions can be spawned for a single
35  ...
37  const maxAutoReworkIterations = 3
...
435 // AutoReopenForPRFix implements session.PRFixSpawner. It transitions the item
436 // from pr_pending back to in_progress and spawns a new autonomous work session
437 // pre-loaded with the CI/review failure context so the agent can fix and push.
438 func (s *BacklogService) AutoReopenForPRFix(ctx context.Context, itemID string, fixContext string) error {
...
451     // Reuse the same iteration cap as the review rework cycle.
452     sessions, sessErr := s.storage.ListItemSessions(ctx, item.ID)
...
456     workCount := 0
457     for _, is := range sessions {
458         if is.Role == session.SessionRoleWork {
459             workCount++
460         }
461     }
462     if workCount >= maxAutoReworkIterations {
463         log.InfoLog.Printf("[AutoReopenForPRFix] item %s has %d work sessions (cap %d); leaving in pr_pending for manual action", itemID, workCount, maxAutoReworkIterations)
464         return nil
465     }
```

**This resolves the requirements.md "Cap interaction" Rabbit Hole
definitively, with no new code needed:** `workCount` is computed by counting
*all* `ItemSession` rows for the item with `Role == SessionRoleWork`
(`ListItemSessions(ctx, item.ID)`, item-scoped, not trigger-scoped).
`ItemSessionData` (`session/storage_backlog.go:16-23`) has no
trigger-reason/trigger-type field at all — a work session spawned because of
a CI failure, a blocking review, or (after this feature) a conflict is
indistinguishable in storage; all three increment the same counter.

Consequently **the cap is already shared across trigger types by
construction** — there is nothing to build to get "shared cap" behavior,
because `AutoReopenForPRFix` is the single entry point for all three
triggers (`ReconcilePRPending` calls the same `fixSpawner.AutoReopenForPRFix`
regardless of which of `CIFailing`/`HasBlockingReviews`/`HasConflicts` is
true) and the cap check inside it counts work sessions with zero regard for
why they were spawned.

If Phase 3 planning instead decides a **separate** per-trigger-type cap is
wanted (e.g. "3 conflict attempts independent of CI attempts"), that would
require: (a) adding a `TriggerReason` (or similar) column to `ItemSessionData`
/ the `item_sessions` ent schema, (b) threading it through
`CreateItemSession` calls in `SpawnSessionFromItem` (currently no caller
passes any such value), and (c) changing the `workCount` loop at 456-461 to
filter by reason as well as role — a materially bigger change than reusing
the existing counter. Recommend the default (shared cap, zero new code)
unless Phase 3 explicitly overrides it; it also matches the requirements.md
framing that sharing is "simpler and matches reuse the same generic fix
path."

No change to `AutoReopenForPRFix` itself is required for the conflict
trigger — its signature (`ctx, itemID, fixContext string`) is already
generic; the conflict-specific prompt content flows in entirely via the
`fixContext` string built in `ReconcilePRPending` (§2), not via any new
parameter.

## 4. Full data flow: `gh pr view` → spawned session's prompt

```
gh pr view --json statusCheckRollup,reviews,comments[,mergeable,mergeStateStatus]
  │  (session/git/worktree_git.go:345-346, extended)
  ▼
payload struct (worktree_git.go:353-377, extended with Mergeable/MergeStateStatus)
  │  json.Unmarshal (378)
  ▼
PRStatus{CIFailing, HasBlockingReviews, HasConflicts (new), FeedbackText}
  │  GetPRStatus returns *PRStatus (worktree_git.go:438)
  ▼
ReconcilePRPending (session/backlog_lifecycle.go:530-585)
  │  gate: !CIFailing && !HasBlockingReviews && !HasConflicts (568, extended)
  │  fixCtx := fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", ..., prStatus.FeedbackText)  (578, unchanged)
  ▼
fixSpawner.AutoReopenForPRFix(ctx, item.ID.String(), fixCtx)  (581)
  │  (interface: session.PRFixSpawner, backlog_lifecycle.go:36-38 — unchanged signature)
  ▼
BacklogService.AutoReopenForPRFix (server/services/backlog_service_triage.go:438-513)
  │  cap check via ListItemSessions/workCount (452-465, unchanged, see §3)
  │  transition pr_pending → in_progress (473)
  │  prFixNote := fmt.Sprintf("[PR Fix context - PR #%d (%s)]\n%s", item.PrNumber, item.PrURL, fixContext)  (480)
  │  temporarily overwrites item.Notes with prFixNote (+ original notes appended) (479-489)
  ▼
SpawnSessionFromItem (backlog_service_triage.go:96, called at 491)
  │  loads item (now carrying the temp Notes) (105)
  │  prompt := session.BuildTokenBudgetedPrompt(item, priorSessions)  (191)
  ▼
BuildTokenBudgetedPrompt → BuildSessionInitialPrompt (session/backlog_context.go:71, :132)
  │  if item.Notes != "" { sb.WriteString("## Notes\n"); sb.WriteString(sanitizeField(item.Notes, 1000)) }  (90-92)
  ▼
prompt string passed to sessionCreator.CreateWorktreeSession/CreateDirectorySession (224-229)
  ▼
spawned work session's initial prompt — agent sees "## Notes" containing the
"[PR Fix context ...]" block, which itself contains FeedbackText's
"## Merge conflict" section built in §1.
```

**Answer to "where exactly does new data need to flow through":** nowhere
new. `HasConflicts`/`mergeStateStatus` only need to reach `FeedbackText`
(§1) — every downstream hop (`fixCtx`, `PRFixSpawner` interface,
`AutoReopenForPRFix`'s `fixContext string` parameter, `item.Notes`,
`BuildSessionInitialPrompt`'s `## Notes` section) is already a generic
string pipe that carries CI-failure and review-comment text today with zero
signal-specific branching after `GetPRStatus`. This is the direct
consequence of `FeedbackText` being a single pre-rendered string on
`PRStatus` rather than structured per-signal data — the entire feature's
data-flow work is contained in `GetPRStatus` (§1) and the one-line gate
extension in `ReconcilePRPending` (§2); no other function in the chain needs
to know a conflict exists.

**One capacity caveat worth flagging for Phase 3/implementation:** `## Notes`
is truncated to 1000 chars via `sanitizeField` (backlog_context.go:92), and
`BuildTokenBudgetedPrompt` (backlog_context.go:132-153) has its own 4000-token
two-pass reduction that can drop `priorSessions` context entirely under
pressure. A conflict `FeedbackText` that lists many conflicting files could
push `prFixNote` past the 1000-char Notes truncation before the agent ever
sees which files conflict. Not a blocker (`CIFailing`'s failed-checks list
has the identical truncation exposure today, unaddressed), but worth a
one-line mention in the conflict `FeedbackText` template (§1) to keep the
file list short/summarized rather than a full diff dump, since it shares the
same truncation budget as CI/review text.

## 5. Summary of exact edits required

| File | Location | Change |
|---|---|---|
| `session/git/worktree_git.go` | `payload` struct, ~354 | add `Mergeable`, `MergeStateStatus` fields |
| `session/git/worktree_git.go` | line 346 | add `,mergeable,mergeStateStatus` to `--json` |
| `session/git/worktree_git.go` | `PRStatus` struct, 327-334 | add `HasConflicts bool` |
| `session/git/worktree_git.go` | ~426 (after reviews loop, before `434`) | detect `Mergeable == "CONFLICTING"`, set `HasConflicts`, append `## Merge conflict` to `sb` |
| `session/backlog_lifecycle.go` | line 568 | extend gate to `&& !prStatus.HasConflicts` |
| `session/backlog_lifecycle.go` | lines 579-580 | add `conflict=%v` to the log line |
| `server/services/backlog_service_triage.go` | none required | cap (`maxAutoReworkIterations`, workCount) already shared by construction — see §3 |

No changes needed to: `session/pr_status_poller.go`,
`session/worktree_pr_poller.go` (explicitly out of scope per requirements.md
and untouched by this data flow — they are a separate poller reading
different fields for the UI badge), `PRFixSpawner` interface, `ItemSessionData`
schema, or `BuildSessionInitialPrompt`/`BuildTokenBudgetedPrompt`.
