# ADR-001: `override_reason` is a role-gated + audit-trail control, not a human-gated one

## Status

Accepted.

## Context

`report_pr_created` (`server/mcp/tools_backlog.go`) previously hard-rejected
any self-reported PR whose GitHub head branch didn't exactly string-match the
backlog item's tracked branch. The bug report
(`project_plans/report-pr-created-branch-mismatch/requirements.md`)
identifies a legitimate recovery flow this breaks: when a session's tracked
branch is polluted (shared/reused worktrees), the standard fix is to cut a
clean branch from `origin/main` and open the PR there instead — which this
check then permanently refused to record.

The requirements offered two alternative fixes: (AC1) relax the check with
some GitHub-verified relationship test, or (AC2) add a separate
"operator override" path. `research/pitfalls.md` (§2) established a fact that
shapes this whole decision: **this codebase has no operator/human role at
all.** `session.SessionRole` is exactly `work`, `triage`, `review` — all
three are LLM-agent-driven session roles assigned by the backlog automation
itself, not by a human operator. There is no fourth role, no
human-vs-autonomous flag, and no auth mechanism anywhere in `server/mcp/`
finer-grained than "does `STAPLER_SESSION_UUID` resolve to a session linked
to this item with role X."

`research/pitfalls.md` (§1) also established that a purely technical
GitHub-side check (ancestry via go-git, or GitHub's `compare` API) cannot
reliably distinguish a legitimate recovery PR from an unrelated one, once the
recovery branch's history has been deliberately severed from the tracked
branch — which is exactly what "cut a clean branch from `origin/main`" does
by design.

## Decision

The fix (`plan.md`, Story 3) keeps `report_pr_created` as the single tool
(no new `link_pr_manually`/`relink_backlog_pr` tool), and accepts a
fallback-branch PR only when **all** of the following hold:

1. The PR verifiably **exists** in the correct `owner/repo` (a real GitHub
   REST lookup by PR number — `GetPRByNumber` — not a caller assertion).
2. The PR's state is `open` or `merged` (not `closed`-and-unmerged).
3. The caller supplies a non-empty `override_reason` string explaining the
   mismatch.
4. The pre-existing role + item-link check still applies unchanged: only a
   `work`-role session already linked to *this specific item* may call the
   tool at all.
5. **(Added — third plan-repair pass, closing a cross-artifact consistency
   gap)** The PR's GitHub author matches the calling session's own resolved
   GitHub identity, via the existing `github.GetCurrentUserLogin` primitive
   — a real, GitHub-verified technical check, not merely an audit string.

Condition 3 is **procedural**, not technical: nothing in this system can
verify that `override_reason` is truthful, or that the session invoking the
fallback path is a different actor than the one that mis-tracked the branch
in the first place. It is deliberately positioned as an audit trail (logged
via `log.Warn` with the item, PR number, actual head branch, tracked branch,
PR author, and the reason verbatim) for **after-the-fact** review, not as a
barrier that prevents misuse at call time. Condition 5, added afterward,
supplies exactly the technical (not procedural) check for one specific class
of misuse — a PR authored by someone else entirely — that condition 3 alone
could never provide; condition 3 remains the only guard against a
*truthful-sounding but wrong* reason for one of the caller's **own** PRs.

## Consequences

- **What this buys:** a real, non-bypassable technical check that the PR
  exists, is in the right repository, and is open/merged — ruling out a
  hallucinated PR number or a reference to some other repo's PR. Combined
  with the unchanged role + item-link check (only the session already
  trusted with *this* item can invoke it for *this* item), a wrong
  invocation is bounded to "the wrong real PR gets attached to the one item
  this session is already authorized to touch, via `SetBacklogItemPRAndTransition`"
  — not an arbitrary cross-item write. **This bound describes the write at
  `report_pr_created`'s own call boundary only — see the next bullet for why
  it is not, by itself, a description of the full blast radius.**
- **What this buys (added during plan repair, Story 6 of
  `implementation/plan.md`):** a guard at the three points downstream in
  `session/backlog_lifecycle.go` where the automated reconciliation loop
  would otherwise treat a wrongly-attached `item.PrNumber` as ground truth
  for a live GitHub mutation or an auto-`done` completion —
  `closeIfSupersededByMain` (auto-`ClosePR`), `ReconcilePRPending`'s
  merge-detected done transition, and `reconcileBouncingItems`'s
  `IsPRMerged`-driven done transition. Each now re-verifies, via a fresh
  `GetPRByNumber` lookup immediately before acting, that the PR's real head
  branch still matches the item's currently-tracked branch, and skips the
  action (fails closed, logs, leaves the item for the next tick / normal
  handling) when it doesn't. This directly closes the gap the adversarial
  review identified in the *original* version of this ADR (below).
- **What this does not buy (revised — third plan-repair pass):** the
  original text of this bullet said `override_reason` bought no protection
  at all against a plausible-sounding reason for "a genuinely
  wrong-but-open/merged PR in the same repo." **That is no longer accurate
  without qualification.** Condition 5 above — `decideOverridePolicy`'s
  author-match gate — is a real, GitHub-verified technical check, not an
  audit string, and it rejects any open/merged PR **not authored by the
  calling identity**. This closes the specific gap a cross-artifact
  consistency review found: requirements.md's "a PR that has no relationship
  whatsoever to the item's work must still be rejected" constraint was, before
  this pass, only provably satisfied for a *closed* PR
  (`TestReportPRCreated_should_RejectCall_When_UnrelatedPRWithOverrideReason`,
  since split into `implementation/plan.md`'s Task 4.4's closed-state case and
  Task 4.4a's new open/merged-and-different-author case) — an open or merged
  unrelated PR would have been *accepted* by the pre-repair design as long as
  a plausible reason was supplied. **What genuinely remains unprotected**:
  the same hallucinating or mistaken session supplying a plausible
  `override_reason` for one of its **own** other, genuinely
  wrong-but-open/merged PRs in the same repo — e.g. a session with several
  concurrently open PRs across different backlog items attaching the wrong
  one of its own PRs to this item. Author-match cannot catch this, because it
  proves only *authorship*, not *item relevance*; GitHub has no first-class
  concept of "this PR addresses that specific backlog item" for a technical
  check to hang on. There is still no technical human-in-the-loop gate at
  `report_pr_created`'s own call boundary for this narrower residual case,
  because none exists in this codebase to hang one on, and requirements.md
  puts UI changes (the one channel that would structurally imply a human) out
  of scope for this fix. This residual is materially smaller than the
  pre-repair state (an arbitrary stranger's PR anywhere in the repo) but is
  not zero, and closing it fully would still require the same operator/human
  auth primitive or UI-mediated confirmation this ADR already puts out of
  scope.
  **A prior version of this ADR additionally, and incorrectly, implied the
  blast radius of that acceptance was itself bounded and inert.** Adversarial
  review found otherwise: `session/backlog_lifecycle.go`'s reconciliation
  loop treats `item.PrNumber` as ground truth once an item reaches
  `pr_pending`, automatically calling `checker.ClosePR` (in
  `closeIfSupersededByMain`, whenever this item's own work commit lands on
  `main` through any path) and automatically transitioning the item to
  `done` (in two separate `IsPRMerged`-driven call sites) — with, originally,
  no check that the attached PR actually belonged to this item's work. A
  wrongly-attached real PR was therefore not inert: it could get a
  stranger's unrelated PR auto-closed without their consent, or cause this
  item to be silently marked `done` off someone else's merge, never
  verifying the item's actual assigned work shipped. The Story 6 mitigation
  above closes this specific gap; what remains true after it is narrower and
  different in kind: an override-linked item (whose PR's head branch
  mismatches the tracked branch *by construction*, since that mismatch is
  exactly why the override path exists) is now guaranteed to never be
  auto-closed or auto-completed by the reconciliation automation, correct
  attachment or not — it falls back to requiring a human/manual resolution
  via the existing review/ship flows instead. See `implementation/plan.md`'s
  Risk Control section ("Narrowed by Story 6") for the full statement of what
  residual risk this leaves.
- **If a future fix wants a real human gate**, the only structurally sound
  channel is a ConnectRPC endpoint reachable only from `web-app/` (browser
  session, not `STAPLER_SESSION_UUID`), not another MCP tool — any MCP tool
  is reachable by the same class of caller as `report_pr_created` itself.
  That is a materially bigger change (new RPC, new UI affordance, new auth
  model) than this bug fix's scope and is intentionally not attempted here.
- **A future reader of `override_reason` in logs or code should not assume
  it proves human review occurred.** It proves only that *some* caller
  supplied a reason string; treat it as a debugging/audit aid, not a
  security boundary. Story 6's reconciliation guard is a technical backstop
  specifically for the *automation* layer, independent of `override_reason`'s
  own procedural (non-technical) nature at the acceptance layer — the two
  should not be conflated. Likewise, a passing author-match check (condition
  5, added in a third plan-repair pass) proves only *authorship*, not
  correctness of item-assignment — see the revised "What this does not buy"
  bullet above for exactly what residual risk that leaves.
- **A fourth mechanism, found and closed in a second plan-repair pass
  (`implementation/plan.md`'s Task 6.3a):** Story 6's guard fails closed by
  design — `closeIfSupersededByMain` returns `false` and the caller
  (`ReconcilePRPending`) falls through to its "normal" handling on a guard
  failure, same as on any other reason the function declines to act. For the
  merge-detected/`ClosePR` sites that "normal" handling is inert (nothing
  further mutates GitHub or completes the item). But for the two sibling
  sites in the same function — the closed-without-merging path and the
  CI-failing/blocked/conflicting path — that "normal" handling *is*
  `l.remediatePRFixWithBackoffGate(...)` → `fixSpawner.AutoReopenForPRFix(...)`,
  which spawns a fresh work session briefed via a `fixCtx` string built
  straight from `item.PrNumber`/`item.PrURL`, with no branch-match check and
  no disclosure. In other words, Story 6's own guard-failure path was exactly
  the trigger for this fourth, still-unguarded mechanism: an override-linked
  item (whose PR's head branch mismatches the tracked branch by construction)
  would reliably avoid the auto-close/auto-`done` mechanisms after Story 6,
  but would just as reliably get a freshly spawned session confidently told
  to "investigate/address" that PR as settled fact. This is lower severity
  than the first three mechanisms — no live GitHub mutation, and the blast
  radius is bounded to a same-class work-role session already linked to this
  item, being handed a PR reference rather than acting on one — but it was a
  real gap in what this ADR claimed Story 6 covered. Task 6.3a closes it: both
  `fixCtx`-building sites now independently re-run the same
  `verifyPRHeadBranchMatchesTracked` guard (a second, cheap, idempotent
  GitHub GET — deliberately *not* threaded through `closeIfSupersededByMain`'s
  return value, since that function returns `false` for several reasons
  unrelated to branch verification and the guard is only actually computed on
  the one path that's about to call `ClosePR`) and, when unverified, prepend
  an explicit disclaimer to `fixCtx` stating the PR's association with this
  item could not be verified and asking the spawned session to confirm
  relevance before investigating or commenting on it. This does not — and
  structurally cannot, absent the same human-gate primitive this ADR already
  puts out of scope — prevent the spawned session from investigating an
  unrelated PR anyway; it only ensures the session is told the ground is
  shaky rather than handed the PR reference as unconditional fact.

## Alternatives Considered

See `implementation/plan.md`'s Step 0.5 (ancestry/compare-API fallback;
separate manual-override tool) — both rejected there for reasons that also
motivate this ADR: ancestry-based checks don't solve the reported scenario,
and a separate tool doesn't add real safety given the absence of an operator
role, while costing meaningfully more surface area.

The third plan-repair pass's own alternatives — author-match implemented as
a step inside `reportPRCreated` instead of inside `decideOverridePolicy`; a
dedicated "operator" role — are recorded in `implementation/plan.md`'s Step 3
table, not repeated here. Both were rejected as, respectively, fragmenting a
decision that should stay in one pure, table-driven-tested function, and a
restatement of the operator-role finding this ADR's Context section already
establishes.
