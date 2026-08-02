# Pitfalls: pr-review-followup

Research date: 2026-08-02. All line numbers verified against the current `main`
checkout in this worktree.

## 1. The dedup-staleness pitfall — deep dive

### The data model has no ID/timestamp to dedup on today

`GetPRStatus` (`session/git/worktree_git.go:528`) fetches
`--json statusCheckRollup,reviews,comments,mergeable,mergeStateStatus,state,isDraft`.
`parsePRStatusPayload` (`session/git/worktree_git.go:549`) only decodes, per
review, `state`/`body`/`author.login` (lines 561–567), and per comment, only
`body`/`author.login` (lines 568–573). **Neither carries a review/comment ID or
a timestamp** (`submittedAt`, `createdAt`, `id`/`databaseId` are all available
from `gh pr view --json reviews,comments` but simply aren't requested). This is
the concrete Go-level trap in (a): the path of least resistance is to add
`HasUnaddressedComments bool` computed the same way `HasBlockingReviews` is
(`state == "COMMENTED"` present → true, `worktree_git.go:628-635`), which
**structurally cannot distinguish "new since we last looked" from "same review
we already reworked for."** GitHub has no dismiss action for `COMMENTED`
reviews, so `state` stays `"COMMENTED"` forever — the boolean never goes false
again once true, no matter what a human or a rework session does. Any dedup
design must widen the `gh pr view --json` field list and the payload struct to
carry `id`/`databaseId` + `submittedAt` (reviews) and `id`/`createdAt`
(comments), and compare against a **persisted high-water mark**, not the
current `state`/`body` fields.

### (a) Infinite re-trigger burns `maxAutoReworkIterations` on already-addressed feedback

Concrete mechanism: `remediatePRFixWithBackoffGate`
(`session/backlog_lifecycle.go:3804`) is gated by `Storage.RemediationDue`
against `domain.StuckReasonPRNeedsFix` — a **time-based** backoff schedule
(30m → 2h → 8h → 24h → 72h, `session/backlog_remediation.go:31-37`), capped at
5 attempts (`MaxRemediationAttempts`, same file line 45). That gate knows
nothing about PR *content* — it only knows "has enough wall-clock time passed
since the last attempt." Once `HasUnaddressedComments` is true, every time the
backoff timer allows it, `AutoReopenForPRFix`
(`server/services/backlog_service_triage.go:1444`) gets called again. Inside
it, the *actual* rework cap is separate and much tighter:

```go
// server/services/backlog_service_triage.go:1479-1489
workCount := 0
for _, is := range sessions {
    if is.Role == session.SessionRoleWork {
        workCount++
    }
}
if reworkCap := s.effectiveReworkCap(item); workCount >= reworkCap {
    ...
    s.notifyReworkCapHit(ctx, itemID, item.Title, session.BacklogStatus(item.Status), "while fixing PR #"+fmt.Sprint(item.PrNumber), reworkCap)
    return nil
}
```

`effectiveReworkCap` defaults to **3** (`maxAutoReworkIterations()`,
`server/services/backlog_service.go:279`, default documented at
`backlog_service_triage.go:293`) and is a **single counter shared across every
trigger type** (CI, review, conflict, and — after this feature — comments): it
literally counts `SessionRoleWork` sessions on the item, with no discrimination
by *why* each one was spawned. A single stale Copilot `COMMENTED` review that
nothing ever clears can, by itself, consume all 3 of an item's rework
iterations across 3 backoff-gated ticks (as fast as 30m + 2h + 8h ≈ 10.5h),
even if a human already replied to and resolved every comment thread in the
first rework round — because the naive `state`-based check has no way to know
the thread was resolved.

**Root Go-level mistake**: comparing `review.state == "COMMENTED"` (or "any
comment exists") instead of comparing the review/comment's ID or timestamp
against a persisted "last one we acted on" marker.

### (b) Permanently stuck at `StuckReasonReworkCap`/`StuckReasonPRNeedsFix` with no path back

This is the most severe finding and is **structural, not just a race**. The
existing self-heal design for this exact loop already has a "poll-shaped
resolve" pattern purpose-built for it — see `ReconcilePRPending`
(`session/backlog_lifecycle.go:4048-4085`):

```go
// backlog_lifecycle.go:4048
if !prStatus.CIFailing && !prStatus.HasBlockingReviews && !prStatus.HasConflicts {
    ...
    l.resolveStuckLogged(ctx, er, item.ID.String(), domain.StuckReasonPRNeedsFix, "ReconcilePRPending/healthy")
    continue
}
// else: PR is CI-failing/blocked/conflicting again — spawn another fix.
```

The comment in this block calls this out explicitly as "pre-mortem F2": a
same-status ("still pr_pending") transition needs its own explicit resolve
call because the generic self-heal sweep only fires on status *changes*. If a
naive comment-signal is simply OR'd into the unhealthy branch (`||
prStatus.HasUnaddressedComments`) and it never flips back to false (per
finding (a)), **this healthy branch can never be reached again for that item**
— `StuckReasonPRNeedsFix` can never resolve via the mechanism the codebase
already relies on for the other three signals. Combined with (a) exhausting
`effectiveReworkCap` and `notifyReworkCapHit` opening `StuckReasonReworkCap`
(`backlog_service_triage.go:125-150`), the item ends up parked on **two**
stuck reasons simultaneously, and `AutoReopenForPRFix`'s own
`workCount >= reworkCap` short-circuit (line 1485) means it returns `nil`
*before* reaching the `ResolveStuck` calls for `StuckReasonReworkCap` at lines
1504-1508 — those only run on a successful re-entry into the spawn path, which
can never happen again once parked. The only way out becomes an operator
manually calling Reset (`ResetStuckRemediation`/`BulkResetStuckRemediation`,
`session/backlog_remediation.go:253,263`) — exactly the dead end the
requirements doc warns about, and it is a direct, provable consequence of
skipping the ID/timestamp dedup in (a), not a secondary risk.

**Design implication**: whatever "is there unaddressed comment feedback"
signal is added must be capable of going from true back to false *without*
GitHub-side state changing — e.g. by comparing against a durably persisted
marker that a completed rework session (or an explicit "no substantive new
feedback since marker" check) advances, so the `!prStatus.CIFailing &&
!prStatus.HasBlockingReviews && !prStatus.HasConflicts && !prStatus.HasUnaddressedComments`
healthy branch is reachable again after a real fix.

### (c) Race between an in-flight fix session and the next 60s tick

This is **already substantially guarded** for the existing three triggers, and
the guard is trigger-agnostic, so it covers comment-feedback too *as long as*
the new trigger reuses the same call path. Two independent guards stack:

1. `remediatePRFixWithBackoffGate`'s `RemediationDue` gate — records the
   attempt (`RecordRemediationAttempt`) **before** returning `true`
   (`backlog_remediation.go:168-193`, esp. comment at lines 149-153: "so a
   caller that dispatches its actual action asynchronously ... can never
   double-count across overlapping sweep ticks or concurrent event
   callbacks"). This is the load-bearing pattern any new dedup-marker write
   must copy: **record the marker atomically with the gate decision, before
   the async dispatch**, not after the spawned session finishes.
2. `AutoReopenForPRFix` itself re-checks for an active work session and bails
   with a clean no-op if one is found:
   ```go
   // backlog_service_triage.go:1473-1477
   s.tombstoneOrphanWorkSessions(ctx, itemID, sessions)
   if hasActiveWorkSession(sessions) {
       log.InfoLog.Printf("[AutoReopenForPRFix] item %s already has an active work session; leaving pr_pending alone", itemID)
       return nil
   }
   ```
   This comment explicitly documents the bug this fixed: unconditional
   `pr_pending → in_progress → rollback` churn "every ~60s indefinitely even
   while a legitimate multi-hour autonomous session was still working."

So a duplicate *session spawn* is already prevented structurally. The residual
risk is narrower than "duplicate session": if a dedup-marker write is
implemented as a *separate* step from the gate/spawn decision — e.g., written
only after the spawned fix session completes and replies to the comment,
rather than at spawn-decision time — a crash/restart or a second concurrent
caller between "gate says due" and "marker persisted" could cause the marker
to never advance even though an attempt (and budget) was consumed, which
reproduces (a)'s exhaustion even with an ID/timestamp-based check. The correct
pattern (per `.claude/rules/go-double-checked-locking.md`'s spirit, and per
`RemediationDue`'s own documented behavior) is: persist "the review/comment ID
this attempt is responding to" transactionally with the same write that
records the fix-spawn attempt, not as a side effect of the fix session's own
completion.

## 2. Does `remediatePRFixWithBackoffGate`'s existing gate cover the new trigger?

Partially, and the gap is exactly what the requirements doc anticipates.
`remediatePRFixWithBackoffGate` (`backlog_lifecycle.go:3804-3845`) gates
**dispatch frequency** — it answers "has enough time passed since the last
`pr_needs_fix` attempt" via the shared time-based backoff schedule. It has
**no knowledge of PR content** and cannot tell "this is the same feedback as
last time" from "this is new feedback." For CI/`CHANGES_REQUESTED`/conflicts,
that's fine because those signals are **self-clearing**: a passing CI run, a
review moving to `APPROVED`/dismissed, or `mergeStateStatus` no longer `DIRTY`
all flip the corresponding boolean back to false on GitHub's side once fixed,
so the healthy branch at `backlog_lifecycle.go:4048` naturally becomes
reachable again and `resolveStuckLogged` clears the stuck row — the backoff
gate only needed to prevent *thrashing frequency*, not detect staleness,
because GitHub itself provides the "is this resolved" signal for free.

`COMMENTED` reviews and plain comments are **not self-clearing** (no GitHub
"dismiss" action, confirmed in the requirements doc and by the payload fields
available in `parsePRStatusPayload`). This means the backoff gate alone is
insufficient for the new trigger in a way it never had to be for the other
three: **it will keep saying "due" on schedule while the health-detection side
never says "resolved."** The fix has to add content-level staleness detection
*upstream* of `remediatePRFixWithBackoffGate` (i.e., in how `HasUnaddressedComments`
itself is computed) — the backoff gate's existing mechanism should not be
modified, since it correctly serves its one job (rate-limiting dispatch); it
just isn't sufficient by itself for a non-self-clearing signal.

## 3. Shared-cap risk (Q3)

Confirmed shared and coarse: `effectiveReworkCap`
(`backlog_service_triage.go:88-95`) resolves to
`item.ReworkCapOverride` if set, else `s.maxAutoReworkIterations()` (global
default 3, `backlog_service.go:279`). `AutoReopenForPRFix`'s cap check
(`backlog_service_triage.go:1479-1489`) counts **all** `SessionRoleWork`
sessions on the item regardless of what triggered each one — it cannot
distinguish "2 CI-fix attempts + 1 trivial Copilot-nit attempt" from "3
genuine CI-fix attempts." So yes: a PR that legitimately needed 2 CI-fix
rounds and then gets a Copilot nit on round 3 would have its 3rd (and final)
budget slot spent on the nit, hitting `StuckReasonReworkCap` before the PR
ever gets a chance to prove itself past CI on a genuine 3rd CI-driven attempt
— though note the requirements doc explicitly mandates reuse of this shared
cap ("must reuse `AutoReopenForPRFix`/`maxAutoReworkIterations` — shared cap
across all trigger types"), so this is an accepted, not open, design question.
The one thing that materially reduces the blast radius given that constraint
is the "substantive" filter also required in scope (ignore empty-body/LGTM-only
feedback) — that at least prevents *trivial* Copilot comments (e.g. a bare
"LGTM" or an empty-body approval-adjacent comment) from ever counting as a
trigger in the first place, which is the cheapest lever available without
violating the shared-cap requirement. A true per-trigger-type cap would avoid
this cross-contamination entirely but was explicitly ruled out by scope, so it
isn't a live design choice here — worth flagging in the plan doc as an
accepted trade-off rather than re-litigating in implementation.

## 4. What else commonly goes wrong here — evidence from existing BUG- regression tests

`grep -n "BUG-" session/backlog_lifecycle.go session/backlog_lifecycle_test.go
session/git/worktree_git.go` surfaces a dense incident history in exactly this
reconciliation loop. Most relevant precedents for this feature:

- **BUG-043** (`backlog_lifecycle_test.go:1041-1097`, comment at
  `backlog_lifecycle.go:1001`): a respawn burned its own attempt budget on a
  verdict that could never be *acted on* because a **different, sibling**
  backoff gate (`StuckReasonBouncing`) was blocking the downstream reopen —
  discovered live across three real backlog items. The fix was
  `RemediationBlocked` (`backlog_remediation.go:213-227`): check whether the
  gate that actually matters is closed *before* spending an attempt on a
  foregone conclusion. Directly analogous risk here: if the comment-feedback
  signal check itself is cheap/free (just re-reading `gh pr view`), the
  parallel risk isn't spending a *remediation* attempt for nothing — it's
  spending a **rework/work-session** for nothing when the dedup marker logic
  is wrong, which is strictly worse (a real session spawn, not just a
  no-op db write).
- **BUG-032 / BUG-036** (`backlog_lifecycle_test.go:1556-1620,1622+`,
  `backlog_lifecycle.go:3971-4034,4088+`): a PR kept failing CI/showing
  conflicts purely because it had drifted stale behind an already-shipped fix
  landed via a different path — not because its own diff was wrong — and each
  "fix" cycle wasted a full rework+review round against an empty/irrelevant
  diff. `closeIfSupersededByMain` (`backlog_lifecycle.go:4097`, called just
  before the fix-spawn at line 4109) now guards this for all three existing
  triggers. **This guard runs before the shared `remediatePRFixWithBackoffGate`
  call and is trigger-agnostic** — as long as the comment-feedback trigger
  path also flows through the same `if superseded := l.closeIfSupersededByMain(...)`
  check before reaching `remediatePRFixWithBackoffGate` (which it will, since
  it's upstream of the fix-spawn call at line 4109, not per-trigger), no new
  code is needed here — but this is a "don't break it" trap: if the
  comment-feedback trigger is implemented as an early-return/separate branch
  that bypasses lines 4085-4099, this superseded-PR protection would silently
  not apply to comment-only triggers.
- **BUG-040** (`backlog_lifecycle_test.go:1480-1553,2012-2091`, extensive
  comments at `backlog_lifecycle.go:3803,4007-4034`): PR fields
  (`PrNumber`/`PrURL`) were cleared even though the reopen never actually
  happened / errored, leaving a live incident with a permanently orphaned
  item. The established discipline this created: **never mutate durable state
  based on "an action was attempted"; only mutate it once the action's actual
  outcome is confirmed.** This is the same discipline the dedup marker must
  follow (per finding 1c above) — write "we responded to review/comment X"
  only once genuinely tied to a confirmed spawn, not merely a gate-passed
  decision, mirroring `remediatePRFixWithBackoffGate`'s own
  `attempted=false` contract at lines 3799-3803 ("the caller must treat this
  exactly like 'nothing happened this tick' and MUST NOT run any
  AutoReopenForPRFix-result-dependent logic ... since nothing was actually
  attempted").
- **BUG-030** (`backlog_service_triage.go:194-227`): a rework session failed
  to spawn AND the rollback to `review` also failed, leaving the item stranded
  `in_progress` with no active session and no visible error. Not
  comment-feedback-specific, but any new code path that calls
  `AutoReopenForPRFix`/`SpawnSessionFromItem` inherits this exact failure mode
  automatically since it's the same spawn call (line 1536) — no new handling
  needed as long as the new trigger goes through the existing
  `AutoReopenForPRFix`, but again is a trap if a parallel/bespoke spawn path
  is written instead.
- **`gh` CLI output brittleness** (`session/git/worktree_git.go:357-369`):
  a live incident where a second `gh pr view --head` call silently returned
  empty output and its error was swallowed, producing a "PR #0" passed
  downstream. Directly relevant since this feature requires widening the `gh
  pr view --json` field list (to add review/comment IDs and timestamps) —
  any new fields parsed from that payload should fail loudly (return an error
  from `parsePRStatusPayload`) rather than silently zero-valuing, given this
  codebase's documented history of exactly that failure shape.

## Summary of design constraints this research implies for the plan phase

1. `PRStatus`/`parsePRStatusPayload` must be extended to carry review/comment
   `id` (or `databaseId`) and timestamp (`submittedAt`/`createdAt`), not just
   `state`/`body` — the current payload cannot support any dedup approach at
   all.
2. The dedup marker must be a **durably persisted, per-item high-water mark**
   (ID or timestamp), not an in-memory cache (the reconciler is explicitly
   restart-safe elsewhere — see `evaluateRemediation`'s restart-grace logic,
   `backlog_remediation.go:96-110` — so an in-memory-only marker would
   regress that property) — and it must be capable of making
   `HasUnaddressedComments` go back to `false` without relying on any
   GitHub-side state change, so the pre-mortem-F2 healthy branch at
   `backlog_lifecycle.go:4048` remains reachable.
3. The marker write must happen atomically with the fix-spawn *decision* (like
   `RemediationDue`'s attempt recording), not after the spawned session
   completes — mirroring both `RemediationDue`'s own documented rationale and
   BUG-040's "don't mutate state until the action outcome is confirmed"
   discipline.
4. Any new trigger-specific logic must be inserted **upstream of, or
   integrated into,** the existing `closeIfSupersededByMain` /
   `remediatePRFixWithBackoffGate` call sequence (`backlog_lifecycle.go:4085-4109`),
   not as a parallel/bespoke branch — otherwise it silently loses the BUG-032/036
   superseded-PR protection and the BUG-030 spawn/rollback-failure handling
   that the existing three triggers already get for free.
