# ADR-003: Loop-Prevention Watermark Design (`GitHubSyncedIssueUpdatedAt`)

**Status**: Accepted
**Date**: 2026-08-03
**Related**: `project_plans/backlog-github-two-way-sync/requirements.md` AC7; `research/pitfalls.md` §2-3; `research/build-vs-buy.md` §3; `research/architecture.md` §6

## Context

Two distinct thrash risks exist once both forward and backward sync are live
on the same `ItemSource`:

- **Risk A**: forward sync closes an issue when an item reaches `done`. The
  very next `SyncOne` tick's `Fetch(state=all)` will legitimately observe that
  same issue as closed. Does backward sync need a guard to avoid "re-closing"
  an already-`done` item?
- **Risk B**: a `done` item is manually reopened locally (`done → in_progress`
  is a valid, unguarded edge — no `TransitionGuard` special case exists for it).
  Forward sync only fires on transitions *into* `done`, never out of it, so
  the GitHub issue stays closed. The next `SyncOne` tick still observes
  "closed" and, without a guard, would push the item straight back toward
  `archived`/`done` — silently undoing the user's manual reopen. **This is
  the actual infinite-loop risk AC7 describes.**

Two candidate fixes were considered for Risk B:

1. **Compare local wall-clock timestamps** (e.g. "don't backward-sync status
   if the item transitioned within the last N minutes"). Rejected: GitHub's
   own read-after-write consistency is not instant (secondary read replicas,
   search-index lag), and local/remote clock skew makes a wall-clock window an
   unreliable, arbitrarily-tuned guard — it can't distinguish "GitHub hasn't
   converged yet" from "the issue was genuinely reopened/re-closed by someone
   else" (pitfalls.md §3).
2. **A per-item watermark storing GitHub's own `issue.updated_at` value**,
   compared against the freshly-fetched issue's `updated_at` on every
   backward-sync tick (chosen — matches the codebase's own existing
   `PrFeedbackAddressedAt` high-water-mark idiom, and the "origin tagging +
   remote-timestamp comparison" pattern every bidirectional sync tool
   surveyed uses — build-vs-buy.md §3).

## Decision

Add `GitHubSyncedIssueUpdatedAt *time.Time` to `BacklogItem`/`BacklogItemData`
— "the GitHub issue `updated_at` value this item's local state has already
reacted to (or deliberately decided to ignore)." **Both** forward sync and
backward sync write this same field (not a `ForwardSyncClosedAt`-only field
as an earlier research draft floated) — whichever direction last observed and
handled a given remote state advances the watermark to that state's
`updated_at`:

- **Forward sync**, after successfully closing an issue, sets
  `GitHubSyncedIssueUpdatedAt` to the issue's `updated_at` as returned by the
  `PATCH` response (or, if unavailable, the local close time as a
  conservative approximation — implementer should prefer the API response's
  own timestamp where GitHub returns one).
- **Backward sync**, before acting on a fetched `ExternalItem`, compares its
  `IssueUpdatedAt` against the item's stored `GitHubSyncedIssueUpdatedAt`:
  ```go
  alreadyReconciled := existing.GitHubSyncedIssueUpdatedAt != nil &&
      !data.IssueUpdatedAt.After(*existing.GitHubSyncedIssueUpdatedAt)
  ```
  If `alreadyReconciled`, skip entirely (no transition attempt, no log spam
  beyond the first observation) — this remote state has already been reacted
  to (or deliberately not reacted to, e.g. the archived/done no-op case from
  ADR-002). Regardless of whether backward sync fires a transition or
  skips-and-logs, it **always** advances the watermark to the just-observed
  `IssueUpdatedAt` afterward, so the same remote state is never re-evaluated
  on a later tick.

This single comparison resolves both risks:

- **Risk A**: forward sync sets the watermark to the issue's `updated_at` at
  close time. The next tick's fetch (assuming no further external change)
  returns the same `updated_at` — `!After` is true — skip. No special-case
  code is needed to distinguish "our own close echoing back" from "a fresh
  external close"; the comparison handles it uniformly. (Separately, Risk A
  is *also* structurally impossible via the state machine alone — `done` is
  excluded from `determineBackwardSyncTarget`'s cases per ADR-002 — so this is
  belt-and-suspenders, not the only protection.)
- **Risk B**: after the manual local reopen, nobody has touched the GitHub
  issue — its `updated_at` is unchanged from when forward sync closed it and
  recorded the watermark. The next tick's fetch returns that same
  (unchanged) `updated_at` — `!After` is true — skip. The item stays wherever
  the user manually moved it; it is not pushed back toward `archived`/`done`.
  If a *different* person later genuinely re-touches the issue on GitHub
  (even just re-closing it, or adding a comment that bumps `updated_at`), the
  fetched `updated_at` advances past the watermark, `After` becomes true, and
  backward sync evaluates the state fresh — proving the watermark only
  suppresses the exact-echo case, not genuinely newer external activity.

**Rejected alternative**: reusing `UserModifiedStatusAt` (the existing
column, stamped unconditionally by `TransitionBacklogItemStatus` regardless
of `triggeredBy`) as the loop-prevention signal. Rejected because
`UserModifiedStatusAt` cannot distinguish a real user click from a prior
backward-sync write — backward sync's own transition would stamp it exactly
like a manual edit, so a future check reading it to mean "user modified"
would see its own prior write and refuse to ever sync that item's status
again (pitfalls.md §2). `GitHubSyncedIssueUpdatedAt` is a purpose-built,
narrowly-scoped field that only ever means "the remote state we've reacted
to," never conflated with "a human touched this."

## Consequences

- Every write path that transitions a status *or* logs a no-op decision in
  response to an externally-observed issue state (both the closed-issue
  branch and the reopened-issue log-only branch, `implementation/plan.md`
  Phase 2 Epics 2.1/2.2) **must** advance `GitHubSyncedIssueUpdatedAt` — a
  future code path that reacts to `ExternalItem.State` without also writing
  this field would reintroduce Risk A/B silently. Both existing
  implementations already do this (see Phase 2 tasks); any new backward-sync
  branch added later must follow the same discipline.
- The watermark is per-item, not per-source (unlike `ItemSource.sync_cursor`,
  which gates the coarser-grained `since` fetch parameter). The two are
  independent and serve different purposes — do not conflate them or attempt
  to derive one from the other.
- `Fetch`'s existing `since=` cursor (per-source, `ItemSource.sync_cursor`)
  already ensures an unchanged issue isn't even re-fetched most ticks — the
  per-item watermark is the second line of defense for the case where the
  issue genuinely IS re-fetched (its `updated_at` did change, e.g. due to an
  unrelated field like a comment) but its *closed/open state* relative to
  what we've already reacted to hasn't meaningfully changed.
