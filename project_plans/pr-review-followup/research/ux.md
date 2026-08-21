# UX Research: pr-review-followup

**Date**: 2026-08-02

## Summary

No new UI surface is warranted for the in-scope work (comment-feedback trigger +
auto-request Copilot review). The requirements' assumption — reuse the existing
`/unfinished` surface — is directionally correct. But investigation turned up a
**pre-existing bug** in that reuse path that this project's own trigger would
inherit silently: `domain.StuckReasonPRNeedsFix` (the reason `ReconcilePRPending`
already uses for CI-failure/`CHANGES_REQUESTED`/merge-conflict auto-fixes) has no
corresponding proto `StuckReason` enum value, so it renders on `/unfinished` as
"⚪ Unknown reason" today, and the "Retry now" button is non-functional for it. Fixing
that mapping is a prerequisite for this project's stated scope ("reuse existing
StuckReasonPRNeedsFix ... UI") to actually work — not itself new UI, but a bug that
blocks the reuse plan.

## 1. Does `/unfinished` already distinguish trigger type?

Checked
[`web-app/src/components/backlog-stuck/stuckReason.ts`](../../../web-app/src/components/backlog-stuck/stuckReason.ts)
and the proto enum in
[`proto/session/v1/backlog.proto:1001-1018`](../../../proto/session/v1/backlog.proto).

The proto `StuckReason` enum has 13 values (`STUCK_REASON_UNSPECIFIED` through
`STUCK_REASON_REWORK_BLOCKED_STALE`) and `stuckReason.ts` has a `Record<StuckReason, …>`
label/icon/class for each — by design (comment at the top of the file) adding a new
enum value is a **compile error** here until a label is added, so the union can never
silently render blank.

**Gap found**: `session/domain/backlog.go:132` defines
`StuckReasonPRNeedsFix StuckReason = "pr_needs_fix"` — a fully wired Go-side reason
(`MarkStuck`, `resolveStuckLogged`, `RemediationDue` backoff gate, and a
`remediationActionByReason` case in
[`server/services/backlog_service_stuck.go:261`](../../../server/services/backlog_service_stuck.go#L261))
— but the proto enum has **no** `STUCK_REASON_PR_NEEDS_FIX` value. So:

- `toProtoStuckReason` (`backlog_service_stuck.go:28-57`) has no case for
  `domain.StuckReasonPRNeedsFix`; it falls through to
  `default: return sessionv1.StuckReason_STUCK_REASON_UNSPECIFIED`.
- On `/unfinished`, a `pr_needs_fix` row today renders the generic "⚪ Unknown reason"
  chip (`STUCK_REASON_LABELS[StuckReason.UNSPECIFIED]`), not a CI/review/conflict-specific
  label.
- The reverse mapping (`fromProtoStuckReason`, same file, lines 63-91) also has no case
  for it, so a client can never construct a valid proto value for `pr_needs_fix` to send
  back on the "Retry now" (`TriggerRemediationNow`) call — that action is silently broken
  for this reason today (the RPC's `reason.IsValid()` check would reject the resulting
  empty-string domain reason).

This is a pre-existing defect, not something introduced by this project — but this
project's requirements explicitly plan to "reuse existing
`StuckReasonPRNeedsFix`/`StuckReasonReworkCap`/`/unfinished` UI," and the comment-feedback
trigger is specified to reuse `AutoReopenForPRFix`, which is exactly the code path that
writes `domain.StuckReasonPRNeedsFix` stuck rows (`session/backlog_lifecycle.go:3805`,
`remediatePRFixWithBackoffGate`). So the comment-feedback trigger would inherit the same
"Unknown reason" / broken-retry-button gap on day one unless the enum mapping is fixed.

**Recommendation**: add `STUCK_REASON_PR_NEEDS_FIX = 13` to the proto enum (and the two
Go switch statements + the frontend `Record` maps) as part of this project's implementation
— not as new UI, but as a bug fix that the reuse-first requirement depends on. This is a
small, mechanical addition (proto value → `make proto-gen` → 2 Go `case`s → 3 TS `Record`
entries), well within a Small (1-3 day) appetite, and does not require a 4th distinct label
for "comment-feedback" specifically (see Q1 continuation below) — one label for
`pr_needs_fix` covers CI failure, `CHANGES_REQUESTED`, merge conflict, and the new
comment-feedback trigger uniformly, because all four write the same `StuckReasonPRNeedsFix`
row via the same `AutoReopenForPRFix` path.

### Does a 4th trigger type need its own label?

No. `StuckItemDetail.tsx`'s "Why:" row (`item.context`, sourced from `fixCtx` built in
`backlog_lifecycle.go`) already renders the specific triggering content as free text —
e.g. `fmt.Sprintf("PR #%d (%s) needs fixes:\n\n%s", item.PrNumber, item.PrURL,
prStatus.FeedbackText)` at `backlog_lifecycle.go:4106`. `FeedbackText` is a combined,
human-readable blob assembled by `render()` in `session/git/worktree_git.go:469-643`,
ordered conflict-first, then CI failures, then review comments — so whatever mix of
signals triggered the fix (CI, `CHANGES_REQUESTED`, a Copilot `COMMENTED` review, or a
plain comment) shows up in that text without needing a distinct chip/label per trigger
kind. One `pr_needs_fix` chip + free-text "Why:" body is generic enough by construction;
no copy change is needed for the new trigger source itself.

## 2. Mental model: is per-attempt trigger visibility worth it?

`StuckItemDetail.tsx`'s "Why:" field only ever shows the **current** `FeedbackText`
snapshot — the state of CI/reviews/comments as of the last reconciliation tick, not a
history of what triggered each of the (up to `maxAutoReworkIterations = 3`) prior rework
attempts. If a Copilot nit is unfixable (the dedup problem noted in pitfalls research) and
the loop retries 3 times before hitting `StuckReasonReworkCap`, the repo owner opening
`/unfinished` today sees: current CI/review state + "Hit the auto-rework cap after
repeated failed reviews" copy (`StuckItemDetail.tsx:79-83`) — but not *which* comment it
kept re-attempting, nor that it was the same comment three times over (vs. three different
issues).

That distinction — "same nit, 3 futile attempts" vs. "3 different legitimate fixes, capped
by policy" — is the crux of at-a-glance legibility, but per-attempt trigger history is
**overkill for this appetite (Small, 1-3 days)**. Reasoning:

- The `pr_needs_fix` → `StuckReasonReworkCap` path already exists and is not part of this
  project's stated scope (comment-feedback detection + Copilot review request) — building
  attempt-history UI would be scope creep onto a different, already-shipped feature.
- A cheaper, in-scope-adjacent option if the dedup problem proves real in practice: include
  the specific review/comment ID or permalink in `fixCtx`/`FeedbackText` (a backend-only
  change, no new UI) so the existing free-text "Why:" field at least shows *which* comment
  is driving the current attempt — still no new component, still reuses the existing row.
  Leave this as a follow-up if the pitfalls research confirms dedup thrash actually
  recurs after this ships, rather than building it speculatively now.

## 3. Accessibility

N/A — no new UI component is added by this project's in-scope work. (The
`STUCK_REASON_PR_NEEDS_FIX` enum-mapping fix recommended in §1 reuses the existing chip
component, label map, and icon map verbatim; it adds a `Record` entry, not a new
component or interaction pattern, so it inherits the existing chip's accessibility
properties — text label always paired with the decorative icon, per
`stuckReason.ts`'s own doc comments — with no separate audit needed.)

## 4. Job-to-be-done

The job: **"never have to manually check Copilot/human comments on my own open PRs."**
Today, `pr_pending` items with only `COMMENTED`-state review feedback or plain comments
sit invisible forever (per requirements' problem statement) — the repo owner has to
remember to open the PR by hand.

Reusing `/unfinished` (once the §1 enum-mapping bug is fixed) closes most of that gap:
the item eventually surfaces as `pr_needs_fix`/`rework_cap` if the auto-fix loop can't
self-resolve, same as CI failures and `CHANGES_REQUESTED` already do. That satisfies the
literal ask ("never have to manually check") for the *unresolved-after-automation*
case.

**Gap the requirements already know about and intentionally defer**: `/unfinished` is a
poll-and-check surface, not a push notification. If Copilot posts a substantive comment
and `AutoReopenForPRFix` successfully spawns a fix session that resolves it within the
60s-tick / `maxAutoReworkIterations` budget, the repo owner never sees anything at all —
which is arguably the *ideal* outcome (fully autonomous, no manual check needed), not a
gap. The real gap is only for the subset that exhausts the rework cap — exactly what
`StuckReasonReworkCap` + `/unfinished` already exists to surface. No case for a
real-time "Copilot just commented" push notification falls out of this project's stated
scope (Small, single-operator tool, backlog automation is the primary consumer) — adding
one would be a distinct, larger project (notification delivery mechanism, quiet
hours/rate-limiting, etc.) not justified by this appetite.

## Bottom line

- No new UI component needed for this project's in-scope work.
- One concrete backend+frontend fix is recommended as part of this project (not
  optional, not new UI): wire `STUCK_REASON_PR_NEEDS_FIX` into the proto enum and both
  Go/TS mapping tables so the reuse-first plan in the requirements actually works —
  today it silently degrades to "Unknown reason" and a broken retry button.
- Accessibility: N/A, no new component.
- JTBD is satisfied by the reuse-first plan for the "automation couldn't resolve it"
  case; a live-notification JTBD variant exists but is explicitly out of this appetite.
