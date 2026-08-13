# UX Research: report_pr_created branch mismatch

## Who the "user" is

The caller of `report_pr_created` (`server/mcp/tools_backlog.go:623`) is always an LLM-driven
Claude Code work session invoking an MCP tool — not a human at a keyboard. This is an
agent-to-system interface. Confirmed no web-app surface exists for *this specific* action:
`web-app/src` has no component that calls or wraps `report_pr_created` (grep across
`web-app/src` for `pr_url`/`prUrl`/`pr_number`/`prNumber`/PR-related components turned up
only *display* surfaces — see "Existing web surface" below — none of them let a human submit
a PR URL/number back onto an item). Accordingly, sections 1–2 below (error message + tool
ergonomics) are the load-bearing part of this doc. Section 3 notes the one relevant display
surface for completeness. **Section 4 (accessibility/WCAG/ARIA) is skipped — there is no new
or modified UI in scope for this fix, so a boilerplate a11y section would be noise.**

## 1. Error message UX for an LLM caller

The current rejection string (`server/mcp/tools_backlog.go:711-714`):

```
PR #%d does not match this item's branch %q on GitHub — refusing to record it. Double-check the PR number/URL.
```

This fails an LLM caller on all three axes AC3 requires:

- **(a) Why rejected** — states the fact (branch mismatch) but not the *cause* the agent
  is actually hitting (tracked branch polluted by another session's commits, so it opened
  the PR from a clean branch instead). The agent has no way to distinguish "you fat-fingered
  the PR number" (the only case the current text anticipates — "double-check") from "you
  correctly worked around a dirty branch and now the tool won't accept it" — the latter is
  the actual bug report.
- **(b) Recovery workflow** — none given. There is no pointer to an alternate tool or
  action.
- **(c) Discourage identical retries** — "Double-check the PR number/URL" actively
  *invites* a retry with the same arguments, which will fail identically every time in the
  fallback-branch case (the PR/branch are correct; the item's tracked branch is what's
  wrong). This is the most actionable defect in the current text: it sends the agent into a
  retry loop against a rejection that never depends on the PR values it's being told to
  recheck.

An LLM caller treats error text as an instruction: it will parse imperative phrases
("double-check X") as literal next steps and retry the same call with cosmetic changes
unless told explicitly not to. Good agent-facing error text should therefore: name the
specific mismatch (expected vs. actual branch) so the model can pattern-match its own
situation, name the one correct next action as a tool call it can make immediately, and use
language that forecloses "just try again" (e.g. "retrying with the same arguments will fail
again" / "do not retry this call").

### Candidate error strings (assumes AC2's manual-override tool exists; name TBD — using
`relink_backlog_pr` as a placeholder, see §2)

**Candidate A — direct, names the workaround explicitly:**
```
PR #%d's head branch on GitHub is %q, not this item's tracked branch %q — refusing to
auto-record it, since these are expected to be the same session's branch. If you opened this
PR from a clean branch (e.g. because %q had unmerged commits from another session), that is
the correct recovery — call relink_backlog_pr with item_id=%s and this PR's URL/number
instead of retrying report_pr_created; retrying this call with the same arguments will fail
again.
```

**Candidate B — leads with the recovery action, shorter:**
```
Branch mismatch: PR #%d is on %q but item %s is tracked against %q. This tool only records
a PR that shares the item's own branch. If you deliberately opened the PR from a different
(e.g. clean/rebased) branch, use relink_backlog_pr to attach it instead — do not retry
report_pr_created with the same PR, it will be rejected again for the same reason.
```

**Candidate C — decision-tree style (most explicit for a model that may not have caused the
mismatch itself, e.g. resuming another session's work):**
```
report_pr_created rejected PR #%d: its GitHub head branch (%q) does not match item %s's
tracked branch (%q).
- If this PR number/URL was mistyped: fix it and call report_pr_created again.
- If the PR was intentionally opened from a different branch (common after a branch was
  polluted by another session and you cut a clean one from origin/main): call
  relink_backlog_pr(item_id=%s, pr_url=..., pr_number=..., reason=...) instead — this call
  will keep failing until you do.
```

Candidate C is the most robust against the ambiguous-cause problem (mistyped vs.
intentional-fallback look identical from the server's point of view) because it gives the
model a branching instruction rather than assuming intent. Candidate A/B are shorter and
cheaper in token budget if a single cause can be assumed dominant. Recommend C, or A if the
team wants to keep the string closer to today's length.

## 2. Tool ergonomics for AC2's manual-override tool

Convention observed at `server/mcp/tools_backlog.go:1014` (`report_pr_created` registration)
and neighboring tools (`submit_review_verdict` just above it, `submit_triage_result` just
below):

- **Description is a paragraph, not a one-liner**, and follows a consistent shape: (1) one
  sentence stating what the tool does and when to call it, including a role restriction
  ("Role: work only"); (2) a sentence naming any verification/side-effect behavior so the
  caller isn't surprised ("The reported PR is verified against GitHub... before being
  trusted"); (3) a sentence on the resulting state transition; (4) idempotency behavior
  called out explicitly ("Calling this again... is safe (no-op)").
- **Parameter descriptions repeat format constraints inline** (`"e.g.
  https://github.com/owner/repo/pull/123"`, `"must match the number in pr_url"`, `"max 1000
  chars"`) rather than relying on JSON schema alone — this matters because the schema isn't
  always shown verbatim to the model, but the description text always is.
- Tool names are verb_noun in snake_case (`report_pr_created`, `submit_triage_result`,
  `submit_review_verdict`, `request_review`).

For the AC2 manual-override tool, applying this convention:

- **Name**: avoid a name that reads as a near-synonym of `report_pr_created` — an LLM
  choosing between two tools whose names both mean "tell the system about a PR" will guess
  wrong under time pressure. Prefer a name that encodes "this bypasses the normal check":
  `relink_backlog_pr` or `override_backlog_pr` (verb makes the bypass explicit) rather than
  `set_backlog_pr` or `attach_pr` (too close to "just another way to report a PR").
  `relink_backlog_pr` is recommended — "relink" implies an existing link is being replaced/
  corrected, which matches the actual use case (the item already has an
  expected-but-wrong-branch state).
- **Description** should explicitly cross-reference `report_pr_created` and state the
  precondition for use, mirroring the existing pattern's "call this when..." framing:
  > "Manually attach or correct a backlog item's linked PR when `report_pr_created` refused
  > it because the PR's head branch doesn't match the item's tracked branch (e.g. after
  > opening the PR from a clean branch cut from origin/main to work around another
  > session's unmerged commits on the item's own branch). Role: work only. Unlike
  > `report_pr_created`, this does NOT verify the PR's head branch against the item's
  > tracked branch — only that the PR exists and belongs to a real GitHub repo — so use it
  > only when you have a specific reason the branch won't match, not as a default path.
  > Requires a `reason` explaining why the branch mismatch is expected, which is recorded
  > for the operator's review."
- **Parameters**: keep `item_id`, `pr_url`, `pr_number`, `summary` identical in name/shape
  to `report_pr_created` (an LLM that has just called one should be able to reuse the same
  argument values for the other with minimal edits) and add one new required field —
  `reason` (string) — capturing *why* the branch doesn't match, both for the operator
  audit trail and because writing the reason is a cheap forcing function against
  reflexively calling the override tool instead of fixing a genuine typo.
- **Symmetry with the rejection message**: whichever tool name is chosen, it must be the
  exact string used in the `report_pr_created` rejection text (§1) — a name mismatch between
  the error message and the actual registered tool name is the single most common way this
  kind of pointer silently fails (the model searches its tool list for the literal string
  from the error and doesn't find it).

## 3. Operator/human surface (informational only, not in scope)

One existing display surface is relevant if a manual-link affordance is ever built:
`web-app/src/components/backlog/detail/PullRequestSection.tsx`. It renders only when
`item.status === "pr_pending"` and is currently read-only: it shows the recorded PR link
(`item.prUrl`/`item.prNumber`) when present, or the text "PR pending — no URL recorded yet"
when absent (`PullRequestSection.tsx:50-53`), plus a "Mark Done" button for the case where a
PR was already merged outside the system's tracking. There is no input field, no "link a PR
manually" action, and no branch-mismatch messaging anywhere in this component or its sibling
`VersionControlSection.tsx`. If a future iteration wants an operator-facing affordance for
this bug's scenario, the "PR pending — no URL recorded yet" state in this file is the
natural anchor point for a "was one opened manually?" hint or a manual-link button — but per
requirements.md this is explicitly out of scope for the current fix and no changes to this
file are proposed here.

## 4. Accessibility / WCAG / ARIA

Skipped. This fix's surface (MCP tool + error string) has no DOM, no ARIA roles, and no
visual rendering — there is nothing for an accessibility audit to evaluate. If AC2's
override tool is later exposed through `PullRequestSection.tsx` or similar, a11y review
should happen at that time against the actual UI, not speculatively here.
