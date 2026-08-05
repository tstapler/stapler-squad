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

Condition 3 is **procedural**, not technical: nothing in this system can
verify that `override_reason` is truthful, or that the session invoking the
fallback path is a different actor than the one that mis-tracked the branch
in the first place. It is deliberately positioned as an audit trail (logged
via `log.Warn` with the item, PR number, actual head branch, tracked branch,
and the reason verbatim) for **after-the-fact** review, not as a barrier that
prevents misuse at call time.

## Consequences

- **What this buys:** a real, non-bypassable technical check that the PR
  exists, is in the right repository, and is open/merged — ruling out a
  hallucinated PR number or a reference to some other repo's PR. Combined
  with the unchanged role + item-link check (only the session already
  trusted with *this* item can invoke it for *this* item), the blast radius
  of a wrong invocation is bounded to "the wrong real PR gets attached to the
  one item this session is already authorized to touch" — not an arbitrary
  cross-item write.
- **What this does not buy:** protection against the same hallucinating or
  mistaken session supplying a plausible `override_reason` for a genuinely
  wrong-but-open/merged PR in the same repo. There is no technical
  human-in-the-loop gate here, because none exists in this codebase to hang
  one on, and requirements.md puts UI changes (the one channel that would
  structurally imply a human) out of scope for this fix.
- **If a future fix wants a real human gate**, the only structurally sound
  channel is a ConnectRPC endpoint reachable only from `web-app/` (browser
  session, not `STAPLER_SESSION_UUID`), not another MCP tool — any MCP tool
  is reachable by the same class of caller as `report_pr_created` itself.
  That is a materially bigger change (new RPC, new UI affordance, new auth
  model) than this bug fix's scope and is intentionally not attempted here.
- **A future reader of `override_reason` in logs or code should not assume
  it proves human review occurred.** It proves only that *some* caller
  supplied a reason string; treat it as a debugging/audit aid, not a
  security boundary.

## Alternatives Considered

See `implementation/plan.md`'s Step 0.5 (ancestry/compare-API fallback;
separate manual-override tool) — both rejected there for reasons that also
motivate this ADR: ancestry-based checks don't solve the reported scenario,
and a separate tool doesn't add real safety given the absence of an operator
role, while costing meaningfully more surface area.
