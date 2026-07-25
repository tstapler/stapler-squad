# Pitfalls Research: unified-vcs-widget

Research Agent 4 (Pitfalls) — SDD Phase 2

## 0. Critical context found: prior art already exists, and it doesn't cover the hard part

Commits `e5d72b24` (feat(backlog): add durable ship-status widget for done items) and
`2072651b` (feat(backlog): richer ship-status widget — commit list + working diff view),
landed 2026-07-17, already solved a *narrower* version of this problem: `GetBacklogItemShipStatus`
(`server/services/backlog_service_ship_status.go`) + `useBacklogItemShipStatus.ts` +
`ShipStatusDisplay.tsx` render commit list / diff / "shipped via PR #N" for done items whose
worktree is gone.

**Why that pattern does not generalize to this project's hardest requirement.** That
implementation works by *recomputing from durable git history* at read time (`git.IsCommitOnMain`,
`git.ListShippedCommits`, `git.BranchAheadBehind` — all go-git, no snapshot table, no staleness
possible because git objects are immutable and content-addressed). Its doc comment states the
justification explicitly:

> "No polling: shipped status doesn't change on its own once a session has ended, so a one-shot
> fetch (plus manual refetch) is enough." — `useBacklogItemShipStatus.ts:23-24`

That assumption is **true for git history** (immutable) and **false for GitHub PR/CI/review
data** (mutable indefinitely — a CI job can be re-run, a review can be added, a PR can be closed
without merging, days after a backlog item reaches `done`). This is exactly the class of bug the
requirements doc's "Feasibility Risks" section is flagging. Do not let planning collapse this
project into "extend ShipStatusDisplay's pattern" — the git-history half can reuse it as-is, but
the GitHub PR/CI half needs a genuine point-in-time snapshot (ent-backed), because there is no
"recompute from an immutable local source" escape hatch for it. Two different durability
mechanisms coexist in one widget; conflating them will produce a component that either polls
GitHub forever (defeats "durable past worktree cleanup," burns rate limit — see §3) or silently
goes stale (defeats "no regression vs. today").

**Recommendation for plan.md**: split "durable" explicitly into (a) git-history facts — reuse
`git.IsCommitOnMain`/`ListShippedCommits`/`BranchAheadBehind`, recomputed live, no storage needed
— and (b) GitHub API facts — snapshotted once at ship time into a new ent entity, never
recomputed for a done item. The shared widget's data-fetch hook needs to know which of these two
regimes it's in, which is itself a design decision (see §2).

## 1. Unifying 3 UI surfaces into 1 component

**Surfaces to unify**: `VcsPanel.tsx` (266 lines, session detail — live), `VcsStatusDisplay.tsx`
(87 lines, shared "current status" display used inside `ShipStatusDisplay.tsx`/`VcsPanel.tsx`),
`ShipStatusDisplay.tsx` (105 lines, Backlog item detail — durable-git-history fallback), and
`UnfinishedItemDetail.tsx` (204 lines, compact card). None of these currently use a `mode` prop
ternary sprawl internally — that's good, it means there's no existing anti-pattern to inherit —
but merging 4 files with genuinely different data sources (live session polling vs. one-shot
historical RPC vs. compact-card subset) into 1 component is exactly the scenario that produces
one.

**Concrete failure modes to design against**:

- **Prop-explosion**: today each surface's parent component (`SessionVcsContext`,
  `useBacklogItemShipStatus`, `UnfinishedWorktree`'s backing data) passes a *different shaped*
  object in. A naive unification passes all fields from all three as optional props on one
  component (`prUrl?`, `checkConclusion?`, `approvedCount?`, `changesReqCount?`,
  `commits?`, `diffStats?`, `compact?`, `showDiffButton?`, `liveStatus?`, `shipStatus?`, ...).
  Fix: define one `VcsWidgetData` union/discriminated type (live vs. historical vs. compact-subset)
  that the *hook* layer normalizes into, so the component itself takes one prop, not fifteen.
- **`mode === 'full' ? ... : mode === 'compact' ? ...` sprawl**: the codebase's existing `mode ===`
  usages (`RuleBuilderForm.tsx`, `ModelOverTimeChart.tsx`, `OmnibarModeBadge.tsx`) are small,
  localized 2-way switches — none of them are a cautionary tale of sprawl yet, but they show the
  house style reaches for inline ternaries first. For a widget with this much data density (PR
  badge, CI badge, review counts, commit list, diff stats, compact card), that style will not
  scale past ~3 conditional render paths without becoming unreadable. Prefer decomposing into
  sub-components (`VcsWidgetHeader`, `VcsWidgetCommitList`, `VcsWidgetDiffStat`) that the
  full/compact parent composes differently, rather than branching inside one large JSX return.
- **God Component nobody wants to touch**: `VcsPanel.tsx` today already has session-lifecycle
  concerns (polling context) mixed with presentation. If the unified component absorbs
  session-live polling, historical-RPC fetching, AND compact-card layout all in one file, it
  becomes the single scariest file in the VCS surface area — every future VCS UI change has to
  reason about all three data-fetch regimes simultaneously. Keep the *data acquisition* (hooks:
  `useSessionVcs`, `useBacklogItemShipStatus`, a new durable-snapshot hook) separate from the
  *presentation* component, and have the presentation component accept only the normalized shape
  from §above. This also directly serves interface-pollution-checklist smell #4 (forwarding-only
  wrapper) — the shared component must not become a thin pass-through that just re-renders
  whichever of 3 upstream shapes it was handed unchanged; it should own real shared rendering
  logic (badges, diff stat bars, commit rows) that all 3 call sites currently duplicate.
- **CSS variant sprawl**: per `.claude/rules/css-architecture.md`, new component styles must use
  a vanilla-extract `recipe()` (see `web-app/src/components/ui/Badge.css.ts`,
  `SuggestedRuleCard.css.ts` for the house pattern) with `variants: { size: {...}, intent: {...} }`
  — not hand-rolled `className={mode === 'compact' ? styles.compactX : styles.fullX}` string
  concatenation. `VcsPanel.css.ts`, `ShipStatusDisplay.css.ts`, `VcsStatusDisplay.css.ts` today
  are three separate stylesheets with likely-overlapping badge/status-color logic — consolidate
  into one `VcsWidget.css.ts` with a `recipe()` for the compact/full variant axis up front, rather
  than merging the three files' classes ad hoc mid-implementation.

## 2. Snapshotting live data that can change after the snapshot

- **Staleness is not a corner case here, it's the default outcome**: CI can be manually re-run
  post-merge, a reviewer can leave a late approval/changes-requested, a PR can be reopened. A
  snapshot taken at "ship time" (item transitions to `done`) is correct *as of that moment* and
  will visibly diverge from GitHub's live state over time. Decide and document in plan.md whether
  the widget shows "as of ship" (with a timestamp label, e.g. "PR status as of Jul 17") or attempts
  a best-effort live re-fetch overlay when the item is still recent/branch still exists. Given the
  explicit "no real-time push (websockets/webhooks)" out-of-scope line, this project is choosing
  eventual staleness by design — the UI must communicate that (a relative timestamp, not a bare
  badge that looks live) or it will read as a bug ("CI shows red but I fixed and re-ran it").
- **Double-checked-locking correctness trap** (`.claude/rules/go-double-checked-locking.md`): if
  the snapshot-write path adds any in-process cache/memoization on top of the ent read (e.g. "if
  we already snapshotted this item in the last N minutes, skip the GitHub call"), the canonical
  bug this rule guards against is returning `cache.field` after the lock instead of the
  locally-computed value — under a lost write race, a second goroutine's result silently ends up
  in what the first goroutine returns to its caller, contradicting what it just computed. The
  reference implementation to mirror is `IsDirty` in `session/git/worktree_git.go`. Concretely:
  if you write something like
  `func snapshotPRStatus(...) (*Snapshot, error) { mu.Lock(); if stale { cache = fetched }; mu.Unlock(); return cache, nil }`
  — that's the bug. Return `fetched`, not `cache`, after the lock.
- **Cleanup-before-snapshot race**: the requirements doc calls this out directly ("what if
  cleanup happens before the last GitHub API poll completes"). Concretely: worktree
  cleanup and the ship-time snapshot write are two independent operations that are not
  currently ordered against each other anywhere in the codebase (grep found no existing
  coordination between session-stop/worktree-cleanup and any "final snapshot" step). The
  snapshot write MUST be a precondition of — or at minimum awaited before — worktree teardown,
  not a fire-and-forget goroutine racing it. The safest sequencing: snapshot capture happens
  synchronously in the same transition that flips status→done (mirroring how
  `isCodeShippedToMain` in `server/services/backlog_service_lifecycle.go` is already a
  synchronous gate on that same transition — see `docs/tasks/backlog-feature-improvement.md`'s
  "Merged-Before-Done Gate" entry), not deferred to a background cleanup job that might run after
  the worktree (and thus the last chance to resolve `githubCheckConclusion` etc. from a live
  session context) is already gone.
- **Fails-closed precedent already exists**: the same "Merged-Before-Done Gate" fix
  (`docs/tasks/backlog-feature-improvement.md`) established the house pattern of "if the check
  errors, block the transition rather than trust a stale field." Apply the same discipline here:
  if the GitHub snapshot fetch fails at ship time, don't silently ship with an empty/null
  snapshot that then looks indistinguishable from "no PR was ever opened" — record the failure
  state explicitly (e.g. a `snapshot_status: "failed"` sentinel) so the UI can say "couldn't
  fetch PR status" rather than showing nothing / showing misleading defaults.

## 3. GitHub API specifics

- **Reuse the existing rate-limit machinery — do not build a new one.** `github/rate_limit.go`
  (`DefaultRateLimiter`, a shared `RateLimiter` with `IsLimited()`/`WaitIfLimited(ctx)`) already
  tracks both primary (hourly quota, `X-RateLimit-Remaining`/`Reset`) and secondary
  (`Retry-After`) GitHub rate limits from response headers, automatically, for all native HTTP
  calls. Any new snapshot-fetch code path must go through the same transport (or at least check
  `DefaultRateLimiter.IsLimited()` before firing a burst of ship-time snapshot calls) rather than
  adding an independent client. This is the "existing pattern to reuse" the task description asks
  to find — it's at `github/rate_limit.go`, wired into `ghHTTPClient`'s transport.
  `session/pr_status_poller.go` and `session/worktree_pr_poller.go` are the existing consumers to
  model a new poller/one-shot-fetcher after.
- **ETag conditional requests are the existing pattern for avoiding rate-limit burn on
  repeated/live polling** (`github/etag_cache.go`, `GetPRInfoConditional` — 304 responses cost
  zero quota). Not directly applicable to a one-shot ship-time snapshot (no repeat polling), but
  directly applicable if the plan ends up wanting a background "periodic re-snapshot while item
  is recently-done" job — reuse `ETagCache` rather than re-fetching full PR info every tick.
- **Batch/bulk risk**: if "done" backlog items are back-filled in bulk (e.g. a migration
  snapshotting all currently-done items that predate this feature), that's N GitHub API calls in
  a tight loop — exactly the shape that trips the primary rate limit (5000/hr) fastest. Must
  respect `DefaultRateLimiter.WaitIfLimited(ctx)` between calls or batch with deliberate pacing,
  not a bare loop.
- **Stale/deleted PR or branch**: a PR can be closed-without-merge, or the remote branch deleted,
  between "item shipped" and "later view of the widget." The snapshot must store what was true at
  capture time and not attempt a live re-resolve of a possibly-gone PR number at render time — the
  whole point of snapshotting is to not depend on the PR still existing/being fetchable.
- **No webhook infra found in this codebase** — GitHub-app webhook signature verification is not
  an existing pattern here (this project is explicitly polling-based per `pr_status_poller.go`,
  `worktree_pr_poller.go`), and "real-time push (websockets/webhooks)" is explicitly out of scope
  for this project too. Do not introduce webhook receiving code for this feature; stick to the
  existing poll/one-shot-fetch model.

## 4. ent ORM schema pitfalls

- **`--feature sql/upsert` is not optional for this feature — flag as must-follow.** The new
  durable-snapshot storage is a natural upsert: re-shipping/re-snapshotting the same backlog item
  (re-review after rework, or a deliberate re-snapshot) must update the existing row for that
  item, not create duplicates or require a manual select-then-branch. Per
  `.claude/rules/ent-schema-generation.md`, regenerating without `--feature sql/upsert` **compiles
  successfully but silently breaks upsert methods** — a defect class with no compiler signal,
  which is the worst kind for an LLM-driven implementation pass to introduce unnoticed. The
  correct command (already the project's `//go:generate` directive in `session/ent/generate.go`):
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
  ```
  The existing `OnConflictColumns(...).Update(func(u *ent.XUpsert){...}).Exec(ctx)` pattern in
  `session/ent_repository_backlog.go` (`BacklogStuckState` upsert, ~line 659) and
  `session/ent_repository.go` (`UpsertRule`, line 1240; diff-stats upsert, line ~494) are the
  concrete precedents to copy for the new `VcsSnapshot`-style entity — do not hand-roll a
  get-then-create-or-update branch.
- **Schema shape precedent**: `session/ent/schema/diffstats.go` is the closest existing analog —
  a small entity (`added int`, `removed int`, `content text`) with a required unique back-edge to
  `Session`. The new durable snapshot entity likely needs a similar required unique edge to
  `BacklogItem` (mirroring how `BacklogStuckState` keys off `item_id` — see the
  `OnConflictColumns(backlogstuckstate.FieldItemID, backlogstuckstate.FieldReason)` composite key
  pattern) plus the PR/CI/review/diff-stat fields listed in the requirements
  (`github_approved_count` int32 field 36, `github_changes_req_count` int32 field 37,
  `github_check_conclusion` string field 38 — already defined on `Session` in
  `proto/session/v1/types.proto`; `additions`/`deletions` int32 on the diff-stat-bearing message
  at proto lines 691/694 and 863/866). Mirror these field names/types on the new ent entity so the
  proto↔ent mapping is mechanical, not a renaming exercise.
- **Always run the full workflow**: edit schema → `go run -mod=mod entgo.io/ent/cmd/ent generate
  --feature sql/upsert ./session/ent/schema` → `go build ./...` → commit all `session/ent/`
  generated changes together in the same commit as the schema edit (per CLAUDE.md's ent workflow
  note) — a partial commit (schema without regenerated `session/ent/` output, or vice versa) will
  desync and is a common LLM-agent mistake when a large diff gets split across multiple commits.

## 5. Mobile/responsive pitfalls

- **Data density vs. narrow viewport**: this widget carries PR badge + CI badge + approve/changes-
  requested counts + commit list + file/diff stats in one card. On mobile this content commonly
  fails in these specific ways — audit for each during implementation:
  - **Overflow without scroll containment**: commit list / file list with long branch names or
    commit messages needs its own `overflow-x: auto` scroll container (per the org-wide artifact
    convention already in this codebase's design guidance) rather than letting the whole page
    scroll horizontally or clipping text silently.
  - **Truncation without affordance**: PR titles, branch names, commit subjects truncated with
    `text-overflow: ellipsis` need a `title` attribute or an explicit "show full" affordance —
    silent truncation with no way to see the full string is a repeat UX complaint pattern in this
    project (see `.claude/rules` mobile+desktop UX convention referenced in project memory:
    "always consider both form factors ... touch targets, responsive layout, mobile keyboard
    behavior").
  - **Touch target sizing**: any interactive element in compact mode (the "View Diff" button
    added in `2072651b`, PR link, expand/collapse for commit list) needs a minimum ~44px touch
    target on mobile even when the compact card's visual footprint is small — a common regression
    when a "compact" variant shrinks padding/font-size without separately checking the tappable
    hit area.
  - **Stacking order**: the reference screenshot's mobile layout stacks sections vertically:
    verify the vanilla-extract recipe's mobile breakpoint reorders (not just shrinks) sections so
    the most decision-relevant info (ship status / PR state) appears above the fold before commit
    list / diff stats, matching `UnfinishedItemDetail.tsx`'s existing compact-card information
    hierarchy rather than reusing the full-mode's top-to-bottom order at small width.
- Per `.claude/rules/css-architecture.md`'s page-scroll convention, if the unified widget is
  rendered inside a scrollable detail panel, remember the root layout sets
  `overflow: hidden` on `mainContent` — any new page/panel hosting this widget must set both
  `height: "100%"` and `overflowY: "auto"` itself or content will clip with no scrollbar.

## 6. e2e test pitfalls specific to this repo

- **No existing e2e coverage for any of the 3 current VCS surfaces** — `tests/e2e/` has no
  `vcs*.spec.ts` or `ship*.spec.ts` file today (confirmed via glob). This means there is no
  existing e2e test to migrate/adapt; the unified widget needs net-new e2e coverage from scratch,
  and per `.claude/rules/feature-registry.md` this is a hard requirement for the PR to be
  considered complete (every new user-facing feature needs ≥1 Playwright e2e test).
- **`waitForTimeout` ban is the single highest-risk violation for this feature specifically**
  (`.claude/rules/e2e-test-conventions.md`, CI-enforced). This widget has two async data sources
  in one component — live polling (`SessionVcsContext`) and one-shot historical/snapshot fetch
  (`useBacklogItemShipStatus`-style hook) — plus a real GitHub API round trip in the backend for
  the live path. That combination is exactly the shape that tempts `await
  page.waitForTimeout(1000)` "just to let the polling settle" or "wait for the GitHub call to
  resolve." The correct pattern per the rule and per `useBacklogItemShipStatus.ts`'s existing
  `loading` state: assert on the loading→loaded state transition via
  `await expect(locator).toHaveValue(...)` / `await page.waitForSelector('[data-testid="..."]')`
  targeting a `data-testid` the component sets once data has actually arrived (e.g.
  `data-testid="vcs-widget-loaded"` or asserting the PR badge text becomes non-empty), never a
  fixed sleep.
- **Locators must be `data-testid` or ARIA role only** — none of the 3 current components
  (`VcsPanel.tsx`, `ShipStatusDisplay.tsx`, `VcsStatusDisplay.tsx`) were checked for existing
  `data-testid` attributes in this pass; before writing e2e tests, audit whether the unified
  component actually exposes stable `data-testid`s on its key sub-elements (PR badge, CI badge,
  commit list items, compact/full toggle if any) — if the current components rely on CSS classes
  for their own internal styling hooks, don't reuse those same class names as test selectors.
- **New spec file needs the `// @feature` header** listing the relevant feature IDs (from
  `docs/registry/features/frontend/`) and should live at `tests/e2e/<feature-name>.spec.ts` with
  reusable navigation/assertion logic factored into `tests/e2e/pages/` per convention, not inlined.
- **Test server determinism for a GitHub-API-backed widget**: since the live path makes a real
  GitHub call (or should hit a fixture/mock in CI — verify which), the e2e test needs a
  deterministic way to assert widget state without depending on live GitHub API response timing/
  flakiness; check whether an existing test fixture/mock for GitHub responses exists in
  `tests/e2e/` before assuming one needs to be built new — this determines whether e2e coverage
  can safely test the live-poll path or should be scoped to the durable-snapshot (done-item) path
  only, which is more deterministic since it's DB-backed rather than live-API-backed.

## Sources consulted

- `.claude/rules/interface-pollution-checklist.md`, `.claude/rules/go-double-checked-locking.md`,
  `.claude/rules/css-architecture.md`, `.claude/rules/ent-schema-generation.md`,
  `.claude/rules/e2e-test-conventions.md`, `.claude/rules/feature-registry.md`
- `web-app/src/components/sessions/VcsPanel.tsx`, `VcsPanel.css.ts`
- `web-app/src/components/shared/VcsStatusDisplay.tsx`, `.css.ts`
- `web-app/src/components/backlog/ShipStatusDisplay.tsx`, `.css.ts`, `.test.tsx`
- `web-app/src/components/unfinished/UnfinishedItemDetail.tsx`
- `web-app/src/lib/hooks/useBacklogItemShipStatus.ts`, `useVcsStatus.ts`, `useSessionVcs.ts`
- `web-app/src/lib/contexts/SessionVcsContext.tsx`
- `github/rate_limit.go`, `github/etag_cache.go`, `github/client.go`
- `session/pr_status_poller.go`, `session/worktree_pr_poller.go`
- `session/ent/schema/diffstats.go`, `session/ent_repository_backlog.go` (BacklogStuckState
  upsert), `session/ent_repository.go` (UpsertRule, diff-stats upsert)
- `server/services/backlog_service_ship_status.go`, `_test.go`
- `proto/session/v1/types.proto` (lines 121-127 github_* fields on Session; 691-694, 863-866
  additions/deletions on FileChange-shaped messages)
- Git history: commits `e5d72b24`, `2072651b`, `a1c79d30`, `95f23a21`,
  `docs/tasks/backlog-feature-improvement.md` "Merged-Before-Done Gate" entry
