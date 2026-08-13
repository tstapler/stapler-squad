# Requirements: report_pr_created cannot link a PR opened on a fallback branch

Source: backlog item `f652340b-3830-4bf8-aba7-7a96ca75b142` (bug report). Interactive
ideate interview skipped per pipeline instructions — requirements derived directly from
the item description and acceptance criteria.

## Problem

`report_pr_created` (`server/mcp/tools_backlog.go:623`) hard-rejects any PR whose GitHub
head branch does not exactly string-match the backlog item's own tracked branch (resolved
via `sessionBranch` → `GetWorktreeDataBySessionUUID`, verified via `VerifyPRMatchesBranch`
→ `githubpkg.GetPRForBranch`, `server/mcp/tools_github.go:272`).

When a session's tracked branch (`backlog/<item-slug>`) is polluted by another session's
unmerged commits (worktrees can be shared/reused — see workspace peer state), the standard
recovery is to open the PR from a clean branch cut from `origin/main` instead (e.g.
`feature/<item-slug>`). `report_pr_created` then permanently refuses to record that PR:

```
INVALID_ARGUMENT PR #N does not match this item's branch <branch> on GitHub — refusing to record it.
```

There is no alternate MCP tool to manually attach/override a backlog item's linked PR, so
the item stays stuck showing no PR even though the work shipped and merged cleanly.
Confirmed on item `3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d` / PR
https://github.com/tstapler/stapler-squad/pull/326 (branch `feature/ci-status-diff-viewer`
vs. tracked branch `backlog/stapler-squad-ci-status-diff-viewer`).

## Why the existing check exists (must not be thrown away)

`VerifyPRMatchesBranch`'s doc comment (`server/mcp/tools_github.go:249`) is explicit: this
guard exists so a hallucinated, stale, or mistyped PR reference from an LLM-driven session
doesn't silently poison the item record. Any fix must keep a real GitHub-verified check in
the loop — it must not become "trust whatever the caller says."

## Acceptance Criteria

1. `report_pr_created` (or a new tool) can link a PR whose head branch differs from the
   item's tracked branch, as long as the PR branch is a descendant of / was created from
   that tracked branch's lineage (or some other sane, GitHub-verified check) — OR
2. A separate manual-override tool/path exists to attach a PR URL to a backlog item
   without the strict branch-name match, for operator use when the tracked branch is
   known-polluted.
3. The rejection error message documents the workaround (open a clean PR + manually
   relink) so future sessions don't retry the identical failing call in a loop.

AC1 and AC2 are presented as alternatives in the source item ("OR") — the plan phase
should pick whichever is the smaller, safer change per `.claude/rules/` guidance
(interface-pollution-checklist, no speculative abstraction) rather than building both.
AC3 applies regardless of which of AC1/AC2 is chosen, and is independently required.

## Constraints / non-goals

- Do not weaken the guard to the point that any caller-supplied PR number is trusted
  without a GitHub-side check — the entire point of `VerifyPRMatchesBranch` is to prevent
  a hallucinated/stale PR reference from being persisted.
- Only `role=work` sessions linked to the item may call this tool today
  (`itemSession.Role != session.SessionRoleWork` check, `tools_backlog.go:669`) — any new
  or relaxed path should preserve at least this level of caller authorization unless the
  plan phase has a specific reason to change it (e.g. an operator-only override tool might
  reasonably require a different role/caller check — to be decided in planning).
- Existing idempotency behavior (re-reporting the same `pr_number` on an already
  `pr_pending` item is a no-op success, `tools_backlog.go:682`) must be preserved.
- Existing tests in `server/mcp/tools_backlog_test.go` (`TestReportPRCreated_*`) define
  the current contract; the fix must not regress
  `TestReportPRCreated_should_RejectCall_When_BranchMismatch` in spirit — a PR that has no
  relationship whatsoever to the item's work must still be rejected. It's the *fallback
  clean branch* case specifically that must now be accepted.
- No new external dependency — `github/client.go` already has `GetPRInfoCtx` (fetch a PR
  directly by number, not just by branch) and other primitives that may be sufficient;
  research phase should confirm what's available before reaching for anything new (e.g. a
  `git merge-base` ancestry check would need the PR's head SHA, which `PRInfo` does not
  currently expose — confirm in research whether extending `PRInfo`/the GH query is needed
  or whether a lighter check is sufficient).

## Out of scope

- Any UI changes (item description doesn't ask for one; AC2's "operator use" language
  suggests CLI/MCP-tool-level access is sufficient, not necessarily a web UI feature).
- Retroactively fixing already-stuck items (e.g. the confirmed example,
  `3065ecfb-3fb7-4ee7-9d04-ada6a7f4169d`) — that's a one-off operational fix-up, not part
  of this bug fix's acceptance criteria.
- Preventing tracked-branch pollution in the first place (a separate, larger problem than
  this bug report).
