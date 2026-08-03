# Requirements: create-rule-orphaned-fallback

Complexity: 1 (quick task — single conditional-gating fix + regression test,
no new architecture, no new UI surface)

## Source

Backlog item `5fb93d9d-6f32-4d5f-b312-ab1b7aed9847` — "minor: Create Rule button
disappears for orphaned pre-deploy approvals during a rollout window". Follow-up
from PR #315 (escalation-reasoning), flagged as a CONCERN (not a blocker) during
that PR's `sdd:6-verify` architecture-review pass and deliberately deferred.

## Problem

`ReviewQueuePanel.tsx` gates the "Create Rule" button on an exact string match:

```tsx
queueItem.metadata?.["tool_input_command"] &&
  queueItem.metadata?.["escalation_reason_category"] === "no-match"
```//`web-app/src/components/sessions/ReviewQueuePanel.tsx` (origin/main, ~line 843)

`escalation_reason_category` is populated in `session/review_queue_poller.go`
(~line 854-855):

```go
if a.EscalationCategory != "" {
    item.Metadata["escalation_reason_category"] = a.EscalationCategory
}
```

`PendingApproval.EscalationCategory` (`server/services/approval_store.go`) is
empty (`""`) for any approval that was serialized to `pending_approvals.json`
*before* the escalation-reasoning feature (PR #315) was deployed, then reloaded
via `ApprovalStore.loadFromDisk` after a server restart — the old JSON on disk
has no `escalation_category` field, so it unmarshals to the Go zero value `""`.

Because the metadata-population code only sets the key when
`EscalationCategory != ""`, these orphaned approvals get **no**
`escalation_reason_category` key at all (not even `""`). The frontend's exact
`=== "no-match"` check then fails for a key that's `undefined`, so the Create
Rule button never renders for these items.

**Correction (found during `pm:triad-review`'s UX pass, verified against git
history):** an earlier draft of this doc claimed "no-match was the only kind
of escalation that existed before PR #315" — that is **false**. The
domain-age (`server/services/domain_checker.go`) and secret-scan escalation
*decision paths* were wired into `approval_handler.go` back in commit
`1c31f024d` (2026-03-26), about 4 months before PR #315 (2026-08-03). PR #315
only added the `EscalationReason`/`EscalationCategory` **label** describing
*why* an existing escalation happened — it did not introduce new reasons to
escalate. So an orphaned approval's true underlying cause could genuinely be
domain-age or secret-scan, not just no-match; the field is simply missing
because it didn't exist yet at persist time, regardless of cause. Treating
"missing category" as equivalent to "no-match" is therefore a **known,
accepted approximation**, not a provably-safe inference — see the residual
risk noted in Impact/Suggested fix below.

## Impact

Low / self-healing:
- Bounded to approvals in-flight at the exact moment of a PR #315-introducing
  deploy (a rollout window measured in the approval TTL).
- Every affected approval resolves via `orphanedCleanupThreshold` (4 hours,
  `approval_store.go`) or the ~4-minute per-approval timeout, whichever is
  sooner, so the button reappears (with correct gating) for anything created
  after the deploy.
- Not a functional/data-loss bug — purely a transient UX regression: users lose
  the one-click "Create Rule" shortcut for a brief window and must fall back to
  manually approving + creating a rule from the settings UI (if that exists) or
  just approving repeatedly until the window passes.
- **Residual risk of the fix itself (see Correction above):** because an
  orphaned approval's true category could be domain-age or secret-scan (not
  only no-match), the fix trades "button briefly missing" for "button
  reappears without being able to distinguish a confirmed-safe no-match from
  an unknown-but-possibly-flagged escalation." This is mitigated in practice
  because (a) the existing "Reason not recorded — this request predates
  escalation-reason tracking" copy already renders alongside the button for
  these items, signaling the uncertainty rather than implying confirmed
  safety, and (b) the window is identically bounded/self-healing. It is a
  trade-off worth stating explicitly in the PR description, not a reason to
  withhold the fix.

## Suggested fix (from item description)

Treat a missing/empty `escalation_reason_category` the same as `"no-match"` for
Create Rule visibility — show the button unless the category is *explicitly* one
of the other known non-"no-match" values (`explicit-rule`, `domain-age`,
`secret-scan`, `unclassifiable`, `unexpected` — see
`pkg/classifier/escalation.go`), rather than requiring an exact `=== "no-match"`
match. This preserves PR #315's intent (hide Create Rule for
explicit-rule/domain-age/etc. escalations, where suggesting a blanket
auto-approval rule is actively wrong) while not penalizing pre-deploy orphaned
approvals that simply predate the field.

## Acceptance Criteria

1. Create Rule button shows for a review-queue item whose
   `escalation_reason_category` metadata key is **absent** (orphaned pre-deploy
   approval), provided `tool_input_command` is present — same as it currently
   shows for `"no-match"`.
2. Create Rule button still shows for `escalation_reason_category === "no-match"`
   (no regression to the PR #315 happy path).
3. Create Rule button still stays hidden for every explicit non-"no-match"
   category introduced by PR #315: `explicit-rule`, `domain-age`,
   `secret-scan`, `unclassifiable`, `unexpected` (no regression to the bug
   PR #315 fixed).
4. Fix is covered by a regression test asserting the specific failure mode:
   missing/empty category ⇒ button visible; explicit non-"no-match" category ⇒
   button hidden.
5. No backend change is required unless the chosen fix approach needs one
   (see research/plan for the minimal-diff option: frontend-only fallback vs.
   defaulting `EscalationCategory` server-side).

## Non-goals

- Backfilling `escalation_category` onto already-persisted pre-deploy JSON.
- Changing the 4-hour `orphanedCleanupThreshold` or the ~4-minute approval
  timeout.
- Any change to the escalation category taxonomy itself
  (`pkg/classifier/escalation.go`).
- Retroactively surfacing an escalation reason line/tooltip for orphaned
  approvals that have none (that's cosmetic and out of scope for this fix).
