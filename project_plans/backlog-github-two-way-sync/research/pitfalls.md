# Research: Pitfalls (Bidirectional Write Sync)

Prior pitfalls doc (`project_plans/backlog-github-issue-link/research/pitfalls.md`) covers
the read-only URL/labels persistence, ent codegen, token-budget, and backfill-limitation
risks for the *pull* half of this system — not duplicated here. This doc covers only the
NEW risk surface: forward sync (local → GitHub write) and backward sync (GitHub → local
write), both landing in the same poll-based `SyncLoop`.

## 1. The `UserModifiedFields` guard is currently a no-op in production — AC4's premise is false today

Grepped the full write path and found **zero call sites that ever populate
`UserModifiedFields`**:

- `session/backlog_sync.go:260` only **reads** it (`parseUserModifiedFields(existing.UserModifiedFields)`).
- `EntRepository.UpdateBacklogItem` (`session/ent_repository_backlog.go:533-624`) — the repo
  method backing the user-facing `PATCH`-equivalent — has no `SetUserModifiedFields` call
  anywhere in its ~90-line field-by-field builder.
- `BacklogService.UpdateBacklogItem` (`server/services/backlog_service_lifecycle.go:235+`),
  the RPC handler a user's edit actually goes through, builds a `BacklogItemUpdate` with
  `Title`/`Description`/`Priority`/etc. but never sets a `UserModifiedFields` field on it —
  there isn't even a field for it in `BacklogItemUpdate` to set.
- The only non-test file containing the string is the ent schema declaration
  (`session/ent/schema/backlog_item.go:69`) and the generated ent plumbing.

**Consequence**: requirement AC4 says backward sync must "respect the existing local-wins
`UserModifiedFields` guard" as if it's a working protection to plug into. It is not — today,
`modifiedFields` in `SyncOne` is always `nil`/empty in production, so the "local-wins" gate on
title/description/priority is dead code that happens to never fire, not a proven mechanism.
Building backward-sync's status/label writes on top of this guard without first wiring up
`UserModifiedFields` population from the user-edit RPC path means **a user's manual status or
label edit has no protection from being clobbered by the next backward-sync tick** — worse
than the pre-existing gap, because pull-sync today never touches status/labels at all, while
backward-sync's whole job is to write status/labels.

**Recommendation for planning phase**: treat "wire `UserModifiedFields` (or an equivalent new
guard) to actually populate on user edits" as an in-scope prerequisite task, not an assumed
dependency — and add a test asserting a user's local status/label edit survives a subsequent
`SyncOne` backward-sync pass, not just that the plumbing compiles.

## 2. `TransitionBacklogItemStatus` unconditionally stamps `UserModifiedStatusAt` — reusing it for backward-sync writes poisons the very guard it's supposed to feed

`SetUserModifiedStatusAt(now)` is called at exactly three sites — none gated by who's
calling: `TransitionBacklogItemStatus` (`session/ent_repository_backlog.go:906`),
`ArchiveBacklogItem` (`:769`), and one path in `storage_backlog.go:633`. Grepping for
consumers of `UserModifiedStatusAt` turns up **only these three writers — nothing in the
codebase ever reads the column**. Today it's a write-only field; the "Status is always
local-wins once user_modified_status_at is set" comment in `SyncOne` (`backlog_sync.go:278`)
describes an aspiration, not code — `SyncOne` never touches status at all right now, so
there's nothing for that timestamp to gate yet.

This matters for the new feature two ways:

- If backward-sync implements its status write by calling the existing
  `TransitionBacklogItemStatus(ctx, id, toStatus, precondition, triggeredBy)` (the natural
  reuse — it already does the atomic CAS-via-WHERE-clause the codebase's own doc comment
  insists on), that call **will** stamp `UserModifiedStatusAt`, exactly the same as a real
  user click does. There's already a `TriggeredBy` audit dimension (`TriggeredByUser` /
  `TriggeredBySystem`, `session/backlog.go:90-92`) recorded in the separate
  `BacklogStatusEvent` audit row, but `UserModifiedStatusAt` itself doesn't branch on it — so
  if a future gating check is added that reads `UserModifiedStatusAt` to decide "should
  backward-sync overwrite local status," it will see its **own prior backward-sync write** as
  "user modified" and refuse to ever sync that item's status again. Every already-existing
  `TriggeredBySystem` transition (PR-pending, auto-done, auto-archive — all over
  `session/backlog_lifecycle.go`) already stamps this same column today, so nearly every item
  that ever went through the normal lifecycle would look permanently "user-locked" the first
  time such a check goes live.
- Recommendation: backward-sync needs its own distinct marker (a new
  `TriggeredBy`-style value, e.g. `TriggeredByGitHubSync`, passed through to
  `TransitionBacklogItemStatus`'s existing `triggeredBy` parameter for the audit trail) and a
  **separate** timestamp/flag it can safely overwrite on repeat runs — don't let
  backward-sync's own writes set the same field a future "was this user-modified" check would
  read, or ping-pong-loop guards (see §3) and local-wins guards collapse into the same bug.

## 3. Infinite ping-pong loop between forward-sync and backward-sync

This is the concrete version of AC7's stated risk, given the actual mechanics above:

- Tick N: user ships backlog item → forward-sync closes GitHub issue #42 (a live HTTP PATCH,
  `state=closed`, using the same token `Fetch` already has).
- Tick N+1 (next `SyncLoop` cadence): `plugin.Fetch` (once it queries `state=all` per AC0)
  sees issue #42 is closed and returns it as an `ExternalItem` with a closed state.
  Backward-sync in `SyncOne`'s existing-item branch sees "external state = closed, local
  status = done" — if the "is this already in sync" check is naive (e.g. "external is closed,
  local isn't already `done`/`shipped`" without checking *why* local is already done), this is
  actually the **idempotent, safe case** — no write should fire because local status already
  matches. The real risk is the reverse ordering:
- If forward-sync's HTTP write to GitHub is slow/async relative to the local DB commit (or the
  local status transition and the forward-sync issue-close are two separate, non-atomic
  operations — see §4), a `SyncOne` tick landing in between could observe the *old* GitHub
  issue state (still open, PATCH not yet applied or not yet visible from GitHub's read
  replica) and try to reconcile local status back toward "open," undoing the very transition
  that triggered forward-sync in the first place, which could then trigger `TransitionBacklogItemStatus`
  again on the next user action, re-firing forward-sync's close call.
- **GitHub's own read-after-write consistency is not instant** — closing an issue via the API
  and immediately re-fetching it (even seconds later) is not guaranteed to reflect the closed
  state on every read path (secondary read replicas, search-index lag for anything using the
  Search API instead of the direct Issues API). A poll-based system with no ordering token
  beyond `updated_at`/`since` cursor has no way to distinguish "GitHub hasn't converged yet"
  from "the issue was genuinely reopened by someone else."

**Recommendation**: make forward-sync idempotent and self-marking — after a successful
forward-sync close, immediately also record (locally) "we are the one who closed this, as of
issue-side `updated_at` >= T" so backward-sync's next tick can compare the *fetched* issue's
`updated_at` against that marker and skip reconciliation if the issue's `updated_at` is
plausibly still "our own write echoing back" rather than a genuinely newer external change.
Comparing raw local wall-clock time against GitHub's `updated_at` is a clock-skew trap (see
§4) — prefer comparing against the cursor/`updated_at` string GitHub itself returns, the same
value `Fetch` already uses for its `since` cursor, rather than introducing a second,
locally-generated timestamp that has to stay in sync with GitHub's clock.

## 4. Split-brain on partial write failure (GitHub write succeeds, local DB write fails, or vice versa)

Forward sync introduces the first place this codebase makes an **external, non-transactional
side effect** (an HTTP call to GitHub) as part of what has otherwise been an all-local status
transition. `TransitionBacklogItemStatus`'s own doc comment (`session/ent_repository_backlog.go:857-878`)
already treats "read-then-write" races as serious enough to have fixed one with a real
incident writeup (`BUG-026`) — but that fix only closes the race *within* the local DB. Adding
a GitHub write to the same logical operation reopens an equivalent problem one layer up,
with no transaction spanning both systems:

- **Local commits, GitHub write fails** (network blip, rate-limited, token revoked mid-flight):
  the backlog item is `done` locally but the GitHub issue stays open — indefinitely, unless
  something retries. Silent data drift a user only discovers by manually checking GitHub.
- **GitHub write succeeds, local commit fails** (crash/panic between the two, or a DB
  connection drop right after the HTTP call returns 200): the GitHub issue is now closed but
  the local item doesn't record that this closure was self-inflicted — so the very next
  backward-sync tick sees "issue closed externally" and this is now indistinguishable from a
  genuine independent close by someone else on GitHub (compounds §3).
- Because there's no saga/outbox pattern here and none is implied as in-scope by the
  requirements, the pragmatic bar (matching this codebase's existing "no-op-not-a-crash"
  philosophy seen in `FinishSourceSync`'s doc comment about atomic cursor+event writes) is:
  do the GitHub write **first**, then the local write, and if the local write fails, log it
  loudly and let the *next* backward-sync tick reconcile local status to match GitHub's
  now-closed state (a self-healing retry, not a silent gap) — rather than doing the local
  write first and leaving GitHub silently un-closed with no automatic retry path at all
  (nothing re-drives a forward-sync attempt once the local transition has already "succeeded").
  Order this deliberately; don't leave it to whichever field happens to get set first in the
  handler.

## 5. GitHub API: write scope/permissions differ from the existing read-only token contract

`githubPluginConfig.Token` (`session/backlog_plugin_github.go:41`) is currently only ever
used for `GET /repos/{owner}/{repo}/issues` (`Fetch`, line 90) — a call that works with a
fine-grained PAT scoped to **read-only** `issues` permission, or even an unauthenticated
call against a public repo. Closing an issue (`PATCH .../issues/{number}` with
`state=closed`) and writing labels (`POST/PUT .../issues/{number}/labels`) require
**write** access to the `issues` permission — a strictly higher privilege than what today's
setup (and any user who configured a read-only token for the existing fetch-only feature)
may have granted.

- A user who set up "Backlog Sources" before this feature shipped, with a read-scoped PAT,
  will have forward/backward-write settings silently fail at the HTTP layer (403) the first
  time they flip either setting on — not caught until runtime, and not distinguishable from a
  generic auth failure unless the error message specifically calls out "this token lacks
  write access to issues," which GitHub's API error body may not spell out clearly.
- Recommendation: validate token write-scope explicitly when a user enables either sync
  direction in Settings (a lightweight probe, e.g. attempt a labels-list GET which needs no
  extra scope, or document the required scope directly in the settings UI copy) rather than
  discovering it only when the first real close/label write 403s during a background
  `SyncLoop` tick where the failure is easy to miss (buried in `CreateSourceSyncEvent`
  history, not surfaced proactively).

## 6. Rate limiting gets meaningfully worse with writes, especially secondary rate limits

The existing rate-limit handling (`backlog_plugin_github.go:109-113`) only detects the
**primary** REST rate limit (`X-RateLimit-Remaining: 0`) on the one `GET` call `Fetch` makes
per source per tick. Adding writes changes the shape of the problem:

- **Volume**: forward-sync potentially issues one `PATCH` (close) plus optionally one more
  call (`POST .../labels`) *per backlog item transitioning to done in that tick*, not per
  source — so a batch-close of N items in one `SyncLoop` cycle is N (or 2N) additional write
  calls beyond the existing one read call per source.
- **GitHub secondary rate limits** are a completely separate, undocumented-threshold
  mechanism triggered specifically by rapid **write** requests (issues/labels/comments) even
  when primary rate limit headroom looks fine — GitHub's guidance is "no more than ~80
  content-generating requests per minute" and to back off with exponential retry on a `403`/
  `Retry-After` header that current code doesn't look for at all (`backlog_plugin_github.go`'s
  rate-limit check only inspects `X-RateLimit-Remaining`, never `Retry-After`). A burst of
  backlog items shipping together (e.g. an autonomous-orchestration batch closing several
  items back-to-back — see `server/services/autonomous_orchestration_service.go:506`, which
  already calls `TransitionBacklogItemStatus` for `TriggeredBySystem` batch transitions) could
  trip secondary limits that the existing single-GET-call code has never had to handle.
- Recommendation: forward-sync writes need their own `Retry-After`-aware backoff (distinct
  from the read path's simpler check), and should not fire the GitHub write synchronously
  inline with every single local status transition in a tight loop — batch/space them, or at
  minimum treat a write-side rate-limit hit as "retry next tick" rather than "fail the whole
  sync run" so one rate-limited item doesn't block the batch's other items via the same error
  path `Fetch` uses today (`fetch: %w` aborts the entire `SyncOne` call for the source).

## 7. Audit-log/notification noise from bot-driven closes — a trust/etiquette concern, not just technical

Closing a GitHub issue via API as if the automation "commented" is invisible to human
etiquette unless done thoughtfully: every issue watcher (subscribers, the assignee, anyone
who starred/commented) gets a GitHub notification for a close event with **no explanatory
comment**, indistinguishable in their inbox from a human maintainer's decision. For issues
imported from a shared/public repo (this repo's own dual-remote setup — see memory
`project_dual_remotes.md`, both `origin` and `upstream-fanatics` are public with no content
filtering) this could confuse outside contributors watching an issue that suddenly closes
with no comment explaining "closed automatically because the linked backlog item shipped
in stapler-squad's internal tracker." Recommendation: when forward-sync closes an issue,
also post a short bot comment (mirrors this repo's own existing convention — see memory
`feedback_document_ai_decisions_in_edge_cases.md`: "self-heal/auto-close actions should post
a visible comment + notify(), not act silently") stating why, and consider whether the
closing token should be a recognizable bot/machine account rather than a personal PAT, so
the close attributes to "automation," not to a human silently closing others' issues.

## 8. Trust boundary: a malicious/compromised repo can drive local backlog state via backward-sync

Backward-sync means anyone who can close an issue or add/remove a label on the linked GitHub
repo — not just the person who configured the sync — can trigger a local backlog item status
transition (assuming default settings get flipped on). For a repo where write access is
broader than "people I'd trust to control my local automation" (e.g. an org repo with many
maintainers, or a public repo accepting outside-collaborator label/triage permissions), this
is a real, if narrow, escalation: someone with issue-write-but-not-backlog-access permission
can now indirectly flip local backlog item state (e.g. force a status transition that fires
whatever `TriggeredBySystem`-driven automation exists downstream, like auto-archive or
orchestration triggers keyed off status — `server/services/autonomous_orchestration_service.go`
already reacts to status). This is not a new vulnerability class introduced by a broken
permission model — configuring the source at all already implies trusting that repo's write
access — but it's worth stating explicitly in the settings UI copy for the backward-sync
toggle ("anyone who can close/label issues in this repo can change this backlog item's
status"), since today's fetch-only behavior only lets an external actor influence *new item
creation*, never mutate an already-imported item's status.

## 9. Default-off is necessary but not sufficient — blast radius when a user turns on both directions

Requirements AC3/AC4 already specify default-off per source per direction, which is the right
call given §3's ping-pong risk. Two related sub-risks the settings UI (AC5, Settings > Backlog
Sources) should account for:

- **Enabling both directions on the same source is the highest-risk configuration** (every
  local transition can trigger a GitHub write that the very next tick reads back) — the UI
  should not present forward/backward as two independent, equally-weighted checkboxes with no
  warning; at minimum a short inline warning when both are toggled on for the same source
  ("closing this item's issue may be observed and re-applied by backward sync — verify this
  doesn't create a loop for items you also edit manually").
- **Enabling backward-sync retroactively affects every already-imported item for that source**,
  not just newly-created ones — since `SyncOne` iterates the same `Fetch` result set
  unconditionally once the setting is on, flipping it on could cause a burst of local status
  transitions across the entire existing backlog in the very first tick after enabling (every
  item whose linked issue happens to already be closed/labeled differently than local state).
  Recommend the plan consider whether first-enable should be a no-op for pre-existing drift
  (only sync state changes going forward) or an explicit, bounded one-time reconciliation the
  user can preview before it fires — silently applying a backlog-wide status change the moment
  a checkbox is flipped is a surprising blast radius for what looks like a small settings
  toggle.

## Summary of concrete recommendations for the planning phase

1. **Wire up `UserModifiedFields` population from the user-edit RPC path** as an in-scope
   prerequisite — it does not exist today (§1), so AC4's "respect the existing guard"
   currently has nothing real to respect.
2. **Give backward-sync's status writes their own `TriggeredBy` value and their own
   "last synced from GitHub" marker**, distinct from `UserModifiedStatusAt` — reusing that
   column for both "a human edited this" and "backward-sync wrote this" collapses the two
   signals a loop-prevention check needs to tell apart (§2, feeds directly into §3).
3. **Design forward/backward idempotency around GitHub's own `updated_at`/cursor value, not
   local wall-clock timestamps** — clock skew and read-after-write lag on GitHub's side make a
   locally-generated timestamp an unreliable ping-pong guard (§3).
4. **Order forward-sync's GitHub write before its local commit**, and make backward-sync's
   next tick the designated recovery path for a local-commit failure after a successful GitHub
   write, rather than leaving GitHub-write failures with no retry path at all (§4).
5. **Validate/document the write-scope requirement explicitly in Settings** before relying on
   runtime 403s to surface a permissions gap (§5).
6. **Add `Retry-After`-aware backoff for write calls specifically**, and don't let one
   rate-limited write abort the whole `SyncOne` batch for a source the way the existing
   `fetch: %w` error path does today (§6).
7. **Post an explanatory bot comment on any issue forward-sync closes**, matching this
   project's existing "no silent automated action" convention (§7).
8. **State the trust-boundary implication in the backward-sync settings copy** — anyone with
   issue-write access to the linked repo can now indirectly drive local backlog status (§8).
9. **Warn (or gate) on enabling both directions for one source, and treat first-enable of
   backward-sync as a potential backlog-wide bulk transition** rather than a silent no-op until
   proven otherwise (§9).
