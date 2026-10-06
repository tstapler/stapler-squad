# UX Research: diff-stats-file-watch-cache

## Verdict

No UI change warranted, even as a flagged follow-up. This is a backend-only
change and stays that way.

## 1. Existing staleness signal: none, and none needed

Grepped `web-app/src/components/sessions/` for `StatusAsOf`, `cachedAt`, `stale`,
and `refresh` — no field or indicator carries a "last checked" timestamp.
`useSessionVcs.ts` (`web-app/src/lib/hooks/useSessionVcs.ts:32-37`) exposes
`refreshStatus`/`refreshDiff`/`refresh` as plain re-fetch triggers, not
freshness metadata. `VcsPanel.tsx:23,73` wires its manual refresh button to
`refresh()` from `useSessionVcsContext()`; `DiffViewer.tsx:15,34` wires
`onRefresh={refreshDiff}` the same way. So today's only "is this fresh?"
affordance is the existing manual refresh button — there is no timestamp or
staleness badge to worry about being wrong.

## 2. Does widening to 5+ minutes cross a threshold?

No. The manual refresh button already covers the failure mode: a user who
suspects staleness clicks refresh, which calls `GetVCSStatus`/`GetSessionDiff`
directly — this bypasses the in-memory cache's TTL check by construction (a
fresh RPC call, not a cache read), so widening the TTL from 15s to 5+ minutes
doesn't change what the refresh button does or how well it works. The
regression this feature risks (watcher misses an fsnotify event, cache serves
stale data for up to the new TTL) degrades the *background auto-refresh*
cadence, not the manual escape hatch. No new indicator is warranted because
the existing one already fully mitigates the failure mode for any user who
actually cares in the moment.

## 3. Job-to-be-done: does the PR-creation path actually read the cache?

Checked the one place a stale diff/status could cause a wrong decision, not
just cosmetic lag: `DraftPullRequest` (`server/services/pr_creation_service.go:173-210`,
called from the "Create PR" modal's submit path per its own `+api` marker).

- `hasCommits, _ := wt.HasCommitsAheadOfMain(baseBranch)` (line 176) — a live
  git call, not `GetHasCommitsAhead()`/`GetVCSStatus`'s cached value.
- `diffStats := wt.Diff()` (line 180) — also live, explicitly commented as
  "Working-tree-inclusive diff preview ... NOT session.GetGitDiff
  (committed-only)."

Neither read goes through the fsnotify-backed cache this feature widens.
`GetHasCommitsAhead()`/`SetHasCommitsAhead()` (`session/git_worktree_manager.go:320-333`,
`session/instance_worktree.go:575,590-593`) is a separate cached signal used
elsewhere (review queue polling, session list badges — `session/review_queue_poller.go:938`,
`session/startup_scanner.go:81`), but `DraftPullRequest` does not call it.

So the two jobs a developer has — quick "anything uncommitted?" gut-check
(tolerant of up-to-5-min staleness; it's a glance, and the manual refresh
exists for when it matters) and "review exact line counts before creating a
PR" (the job that would NOT tolerate staleness) — are cleanly separated by the
code: the gut-check reads the cache this feature touches, the PR-creation
job reads live git state that this feature does not touch at all. There is no
call site where a stale cache entry could cause a wrong PR-related decision.

## Recommendation

Ship as backend-only, no UI changes, no flagged follow-up item. If a future
change ever routes `DraftPullRequest` or `CreatePullRequest` through the
cached `GetVCSStatus`/`GetSessionDiff` path (e.g. as a perf optimization),
revisit this conclusion at that time — the live-read guarantee found here is
what makes 5+ minute staleness safe, and it's easy to invalidate by a later,
unrelated refactor.
