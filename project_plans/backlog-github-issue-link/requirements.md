# Requirements: Surface Linked GitHub Issue/PR to Agent Sessions and PRs

Source: backlog item `0c380cf7-1888-48a3-ac79-3a010b04a722`
("Imported backlog items never surface their linked GitHub issue to the agent or the resulting PR")

## Problem

When a backlog item is imported from `github_issues` (or `github_prs`), the
originating issue/PR is invisible everywhere downstream:

1. `GitHubIssuesPlugin.MapToBacklogItem` (`session/backlog_plugin_github.go:156-176`)
   and `GitHubPRsPlugin.MapToBacklogItem` (`session/backlog_plugin_github_prs.go:194-212`)
   both read `item.URL` (the full `html_url`) during `Fetch` but drop it — only
   `ExternalID` (the bare number) is copied onto `BacklogItemData`. There is no
   `ExternalURL` field on `BacklogItemData`, `BacklogItemUpdate`, or the
   `BacklogItem` ent schema to carry it.
2. `BuildSessionInitialPrompt` (`session/backlog_context.go:72-129`) never
   references `ExternalID`/`ExternalURL` at all, so even the bare issue number
   that already round-trips today never reaches the agent.

Net effect: an agent working an imported item has no way to reference the
originating issue in its PR, so GitHub never auto-closes it and the reporter
never learns their bug was fixed.

## Ground truth confirmed in code (2026-07-25, after resetting this branch onto
current `origin/main` — see note below)

- `ExternalItem.URL` (`session/backlog_plugin.go:27`) is already populated by
  both plugins' `Fetch` from `issue.HTMLURL` / `pr.HTMLURL`.
- `BacklogItemData` (`session/repository.go:257-282`) has `ExternalID` but no
  `ExternalURL`.
- `BacklogItemUpdate` (`session/repository.go:301-313`) has no URL field either.
  Per the resolved design (plan.md, ADR-001), the AC6 backfill routes *through*
  this struct — adding `ExternalURL *string` to it — and only bypasses the
  `UserModifiedFields` local-wins gate, not the struct itself.
- ent schema `session/ent/schema/backlog_item.go` has `external_id` (optional
  string) but no `external_url` column.
- `backlogItemToData` / `CreateBacklogItem` (`session/ent_repository_backlog.go`)
  round-trip `ExternalID` but have no `ExternalURL` plumbing.
- `BuildSessionInitialPrompt` / `BuildTokenBudgetedPrompt`
  (`session/backlog_context.go`) never mention `ExternalID` or any URL.
- `SyncOne` (`session/backlog_sync.go:195-299`) applies local-wins updates
  (title/description/priority) on existing items but has no path that would
  backfill a newly-added field on pre-existing rows.
- Two hand-built `&ent.BacklogItem{...}` literals in
  `server/services/backlog_service.go` (~line 1086, `SpawnSessionFromItem`,
  and ~line 1229, `AttachSessionToItem`) construct the struct passed into
  `BuildTokenBudgetedPrompt`/`WriteSlashCommands`/`WriteBacklogContextFile`
  field-by-field — neither copies `ExternalID` today, and neither will copy
  `ExternalURL` unless explicitly added.

**Environment note:** this worktree's branch and the local `main` it was cut
from were ~1989 commits behind `origin/main` and did not contain the backlog
feature at all. Per user direction, the branch was reset to `origin/main`
(`git checkout -B <branch> origin/main`) before any of the above was
verified. No local `main` ref or other sessions' work was touched.

## Acceptance Criteria (from backlog item, verbatim numbering preserved)

1. `BacklogItemData` and the underlying `BacklogItem` ent entity/DB column
   gain an `ExternalURL` field, populated by both `GitHubIssuesPlugin.MapToBacklogItem`
   and `GitHubPRsPlugin.MapToBacklogItem` from the fetched issue/PR HTML URL,
   capped at 500 chars.
2. A new ent migration (regenerated via `--feature sql/upsert`) adds the
   `external_url` column; `go build ./session/...` compiles and pre-existing
   rows read back `ExternalURL == ""` (no NULL panic).
3. `BuildSessionInitialPrompt` renders a "Linked GitHub Issue/PR: `<url>`"
   fact line whenever `ExternalURL` is non-empty, plus a deterministically
   Go-resolved literal instruction line ("Fixes " for `/issues/` URLs,
   "Related: " for `/pull/` URLs, via a `closingKeywordFor` helper) — never
   left to agent-side runtime inference.
4. When `ExternalURL` is empty (manually-created or not-yet-backfilled
   items), the prompt renders identically to today with no new section and
   no broken formatting.
5. `BuildTokenBudgetedPrompt`'s two truncation passes still include the fact
   line and instruction line unchanged even when Description/prior-session
   truncation kicks in.
6. `SyncOne`'s existing-item branch backfills `ExternalURL` on pre-existing
   rows unconditionally (bypassing local-wins/`UserModifiedFields`), with the
   documented, tested limitation that it can only backfill items still
   returned by each plugin's `state=open` `Fetch` call.
7. Both hand-built `ent.BacklogItem{}` literals in
   `server/services/backlog_service.go` (`SpawnSessionFromItem` and its
   attach-flow counterpart) include `ExternalURL`, proven by an integration
   test asserting the real spawned session's prompt contains the
   linked-issue section.
8. All existing tests in `backlog_plugin_github_test.go` and
   `backlog_context_test.go` continue to pass; new tests cover the URL
   round-trip, empty-URL fallback, `closingKeywordFor` table cases, and the
   backfill/known-limitation pin.
9. `make build && make test` exits 0 for the `session` and
   `server/services` packages with no compile errors in `session/ent/*`
   (confirming `--feature sql/upsert` was not omitted).
10. No source code is modified during this triage/planning phase — only
    `requirements.md`, `research/*.md`, `implementation/plan.md`,
    `implementation/validation.md`, `implementation/pre-mortem.md`, and
    `decisions/ADR-001` are touched.

## Explicitly out of scope

- Changing the generated PR body / `/backlog/ship` / `github:pr-ship` to
  auto-inject a `Fixes <url>` line. The formal ACs only require the *prompt*
  to instruct the agent to write that line itself — the agent authors the PR
  body, not generated code. (The backlog item's "Fix sketch" mentions this,
  but it isn't in the numbered ACs.)
- Any change to `GitHubPRsPlugin`'s CI-label/reviewer logic beyond adding
  `ExternalURL` to its `MapToBacklogItem`.
- Backfilling `ExternalURL` for items whose source issue/PR has since been
  closed and no longer appears in the plugin's `state=open` Fetch — AC6
  explicitly documents this as a known limitation, not a bug to fix.

## Key design questions for planning phase

- Where exactly does `closingKeywordFor` live (probably `backlog_context.go`
  next to `BuildSessionInitialPrompt`), and what's its exact signature/table
  of URL-shape → keyword?
- Exact insertion point of the new prompt section relative to the existing
  `--- END BACKLOG ITEM DATA ---` marker and `taskProtocolBlock`.
- Whether `SyncOne`'s unconditional backfill needs a new repository method or
  can reuse `UpdateBacklogItem` with a dedicated `ExternalURL *string` field
  added to `BacklogItemUpdate` (bypassing the `UserModifiedFields` gate that
  guards the three existing local-wins fields).
- Exact ent field definition (`field.String("external_url").Optional()`,
  500-char cap enforced in Go before Save, not via a DB constraint) and
  whether an index is warranted (no AC calls for one — likely not).
