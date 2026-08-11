# Research: Features — Prior Art for Bidirectional Issue-Tracker Sync + Edge Cases

## 1. What already exists in this codebase

- **Sync engine**: `session/backlog_sync.go` `SyncLoop.SyncOne` (lines 195-303) is a
  poll-based, per-source-locked (`syncSourceLocks`, a `sync.Map[string]*sync.Mutex`)
  reconciler. It fetches via `plugin.Fetch(ctx, cfg, cursor)`, looks up existing rows by
  `(sourceID, ExternalID)` via `GetBacklogItemByExternalID` (per-source-scoped —
  duplicate external IDs across *different* sources are fine and expected, e.g. issue
  #1 in two repos), and applies a **local-wins conflict rule**: any field name present
  in the item's `UserModifiedFields` JSON column is skipped on update (lines 259-278).
  `Status` is hard-excluded from this update path entirely today — comment: "Status
  transitions are only done via `TransitionBacklogItemStatus` — no update here."
- **Fetch is pull-only, open-only**: `GitHubIssuesPlugin.Fetch`
  (`session/backlog_plugin_github.go:90`) hardcodes `state=open`, so a closed issue
  simply stops appearing in the fetch result set — it is not "seen as closed," it is
  *not seen at all*. This is the root cause behind requirements AC0.
  - Fetch also has no page-2+ handling (`per_page=50`, no `Link` header pagination) and
    no `X-RateLimit-Remaining` proactive backoff (only a reactive 429/403+quota-0
    check, lines 110-113) — a large repo backfill or a rate-limited mid-fetch failure
    aborts the whole `Fetch` call (`fetchErr` path, lines 221-230) and is recorded as
    one failed `SourceSyncEvent`, not partial progress. There is no mid-batch
    checkpointing: if `Fetch` succeeds but `CreateBacklogItem`/`UpdateBacklogItem` fails
    partway through the `items` loop (lines 234-291), already-processed items are
    committed (each item's create/update is its own transaction) while later ones are
    skipped (`errored++`, loop `continue`s) — this is actually the *safe* partial-write
    behavior already, just not atomic across the whole batch, which is appropriate for
    an idempotent poll loop.
- **Existing audit trail is aggregate, not per-item**: `CreateSourceSyncEvent` /
  `FinishSourceSync` (`session/ent_repository_backlog.go:1510+`) record one row per
  sync *run* with counts (`created/updated/skipped/errored`) and an error string —
  visible via `ListSourceSyncEvents` (capped at 200 rows,
  `maxSourceSyncEventsHistory`). There is **no per-item log** of *which* item changed
  *what* field because of an external sync vs. a user action — a gap directly relevant
  to the "audit trail of sync actions" unstated need in the prompt.
- **No per-item sync toggle**: sync is enabled/disabled per `ItemSource` (the
  `Enabled` bool checked in `runAllSources`), not per backlog item. There is no
  existing mechanism (column, UI, or RPC) to exclude a single imported item from
  future sync while leaving its sibling items syncing normally.
- **No dry-run/preview**: `TriggerSync` (`server/services/backlog_service.go`,
  wired to `SyncLoop.SyncByID`) always writes. There is no "fetch and show me what
  would change" mode.
- **UI provenance**: confirmed via the prior `backlog-github-issue-link` project's
  research — `externalId` is plumbed all the way to the generated
  `web-app/src/gen/session/v1/backlog_pb.ts` but **no component under
  `web-app/src/components/backlog/*` renders it**. Nothing renders `ExternalID`,
  `ExternalURL` (doesn't exist yet), or `Labels` (doesn't exist yet) today. This
  project is starting from zero UI surface, not extending an existing badge/link.

## 2. Industry precedent for bidirectional issue-tracker sync

### Linear ↔ GitHub (native two-way sync)

Linear's official GitHub integration syncs title, description, status, labels,
assignee, and comments in both directions, including closed-status transitions
([Linear docs](https://linear.app/docs/github)). Two relevant constraints from their
model:
- **Only one repo can be configured for two-way sync at a time** per Linear team —
  i.e., they scope sync 1:1 at the source level, similar to this codebase's
  per-`ItemSource` `Enabled` flag, not per-item.
- Linear surfaces a **sync-status banner directly on the synced issue** ("GitHub sync"
  badge with error surfacing) rather than a separate log — a UI precedent for where
  provenance + sync health should live (on the item itself, not buried in a settings
  page), reinforcing AC2's card/detail provenance requirement.
- Linear's public docs do not detail their conflict-resolution/loop-prevention
  algorithm precisely — treat "local-wins via last-write-timestamp or field-level
  ownership" as an inferred industry pattern, not a confirmed one.

([Linear GitHub sync docs](https://linear.app/docs/github?tabs=b5eb539099f9), [Linear GitHub Issues Sync changelog](https://linear.app/changelog/2023-12-14-github-issues-sync))

### Jira ↔ GitHub (Atlassian DVCS connector, and third-party bots like `gh-jira-sync-bot`)

Documented real-world failure modes, several directly applicable here:
- **PRs masquerading as issues**: GitHub's REST Issues API returns PRs as issues too;
  a naive integration double-processes them as duplicate tickets unless explicitly
  filtered. This codebase's `github_prs` and `github_issues` plugins are already
  separate (per the requirements' non-goal #1: two-way sync is issues-only, PRs
  explicitly out of scope) — so this specific bug class is preemptively avoided by
  the existing plugin split, worth confirming/citing in the plan rather than
  re-litigating.
- **Rate limiting pauses the whole sync**, surfaced to the user as "GitHub rate limit
  was exceeded" — Atlassian's own support docs describe DVCS sync simply pausing
  until quota resets, not silently corrupting state. This matches the existing
  reactive-429 behavior in `Fetch` (fail the whole call, record `SourceSyncEvent`
  with the error) — an acceptable, precedented pattern; the gap is *proactive*
  backoff (checking `X-RateLimit-Remaining` before making the next page request)
  which neither this codebase nor, apparently, Jira's connector does well.
- **Immutable IDs over mutable keys**: a recurring lesson in Jira/GitHub sync bots is
  to key the linkage on an immutable ID (GitHub issue *number*, which never changes)
  rather than a derived/mutable value (URL, title) — this codebase already does this
  correctly (`ExternalID` = `issue.Number`, not URL or title), so `ExternalURL`
  addition (per the prior linked project) is correctly treated as "along for the
  ride," never a lookup key.
- **Echo suppression** is named explicitly as a required technique: bots must
  recognize "this change was caused by our own last sync write" and skip
  reprocessing it, rather than relying on timing alone. Concretely this usually means
  either (a) a bot/service-account identity check on the actor who made the change
  (skip if `actor == our own bot account`), or (b) a content-based check ("does the
  incoming state already match what we just wrote — no-op, don't re-emit"). This
  codebase's poll-based model (no webhooks) makes approach (b) — idempotent
  comparison — the natural fit: forward-sync writes the issue closed; next poll
  tick, backward-sync sees `state=closed` on that issue and would try to "sync" the
  backlog item to `done`, but if it's *already* `done`, that's a no-op compare, not a
  thrash. The actual risk (per requirements AC7) is a **status oscillation** if
  forward-sync and backward-sync ever disagree about *direction* (e.g. user reopens
  the backlog item locally → forward-sync doesn't run because it only fires on the
  done/shipped transition per AC3 → but backward sync on the next tick sees the
  GitHub issue still closed and pushes the backlog item back to done). This is a real
  gap the plan needs to close, not just "poll-based sync has no gap since there's no
  webhook to echo."

([Jira GitHub Issues Integration guide](https://exalate.com/blog/jira-github-issues-integration/), [Atlassian Community: GitHub rate limit sync pause](https://community.atlassian.com/forums/Jira-questions/Github-Sync-Paused-GitHub-rate-limit-was-exceeded/qaq-p/1112497), [canonical/gh-jira-sync-bot](https://github.com/canonical/gh-jira-sync-bot))

### GitHub Projects (v2) — status field sync from Issues

GitHub's own Projects v2 treats "Status" as a project-scoped custom field, not the
issue's own `state` (open/closed) — i.e. GitHub itself does **not** try to map a
multi-value board status onto the binary `open`/`closed` issue state automatically;
users/automations (via `gh-actions`/webhooks) explicitly write the mapping. This is
directly relevant to the requirements doc's implicit question: **there is no
canonical "right answer" for mapping GitHub's 2-state model onto this app's 9-state
`BacklogStatus` enum** (`idea, refining, ready, queued, in_progress, review,
pr_pending, done, archived` — `session/domain/backlog.go:16-24`) — even GitHub's own
first-party tooling treats that mapping as a local, explicit policy decision, not
something to infer generically.

## 3. Edge cases and failure modes to design for

### 3.1 No local status cleanly maps to "closed" (partial/ambiguous states)

GitHub issues have exactly 2 states (`open`, `closed`); this app has 9
(`session/domain/backlog.go:16-24`), several of which represent in-flight agent work
(`in_progress`, `review`, `pr_pending`) that has no GitHub equivalent at all. Concrete
design questions the plan must answer:
- **Closed → backward-sync target**: `done` is the obvious target for a closed issue,
  but what about an item currently in `in_progress`/`review`/`pr_pending` when its
  source issue closes externally (e.g. someone closed it manually on GitHub while an
  agent session was already working it)? Silently jumping straight to `done` skips
  the `review`/`pr_pending` states this app uses to gate actual shipped work, and
  could mark an item done before its in-flight session/PR is real. Likely correct
  behavior: only apply backward-sync's closed→done transition from states where
  `domain.BacklogStatus`'s own `validTransitions` map (`session/domain/backlog.go:331+`)
  already allows a direct edge to `done` — reuse the existing state machine rather
  than bypassing it, and treat a disallowed transition as a skip (log it), not a
  forced move.
- **Reopened → backward-sync target**: symmetric problem — an issue reopened on
  GitHub after the backlog item is already `done`/`archived`. Confirmed in
  `session/domain/backlog.go:385-387`: `archived`'s only valid outgoing edge is back
  to `idea` (not directly to `ready`/`in_progress`/etc.), so a naive "reopened →
  push back to whatever pre-close state" backward-sync would have no valid target
  for an archived item beyond `idea` — effectively a full re-triage, not a resume.
  Given that's a fairly drastic, possibly-surprising automatic action, the plan
  should treat GitHub-reopen-after-archive as a documented no-op (log + surface in
  UI, "GitHub issue reopened; backlog item is archived — reopen manually to
  re-triage from idea") rather than silently forcing the `archived → idea` edge on
  the user's behalf.
- **`refining`/`queued`/etc. have no GitHub equivalent at all** — forward sync
  (backlog → GitHub) is explicitly scoped by AC3 to fire only on the done/shipped
  transition, sidestepping this; backward sync only needs to handle GitHub's 2 states
  landing on this app's richer model, not the reverse.

### 3.2 Multiple backlog items pointing at the same GitHub issue (duplicate import)

`GetBacklogItemByExternalID` is scoped to `(sourceID, externalID)` and is used as the
create-vs-update discriminator during `SyncOne` — so **the sync loop itself cannot
produce two items for the same `(source, external_id)` pair**; the first match found
on each poll is treated as "the" existing item. However, duplicates *can* still arise
via paths sync doesn't guard:
- A user manually creates a second backlog item and manually pastes/sets the same
  `ExternalURL` (once that field exists) without going through import — nothing
  currently enforces uniqueness on `ExternalURL` (unlike `ExternalID`, which is only
  ever written by `MapToBacklogItem`, not user-editable).
- The **same GitHub issue imported through two different `ItemSource` rows** (e.g. a
  user accidentally configures two sources pointing at the same owner/repo) — since
  the duplicate lookup is scoped *per source*, this produces two backlog items with
  identical `ExternalID`/`ExternalURL` but different `SourceID`, and **both would
  independently forward-sync writes to the same GitHub issue**, each unaware of the
  other. This is a real gap: consider either (a) documenting it as a known limitation
  (don't create two sources for the same repo) with UI validation warning on
  `ItemSource` creation, or (b) a uniqueness check on `(owner, repo)` across enabled
  `github_issues` sources at config-save time. Given the non-goal list already scopes
  tightly, (a) — documented limitation + a save-time warning — is proportionate; a DB
  constraint is probably overkill for a self-inflicted misconfiguration.

### 3.3 Issue relabeled / renamed / transferred / deleted on GitHub

- **Relabeled**: straightforward — backward sync just needs a labels-diff on each
  poll (compare fetched `Labels` to stored `Labels`), applying the same
  `UserModifiedFields`-gated local-wins rule already used for title/description/priority.
- **Repo renamed**: per the prior linked project's research (`ADR-001` area),
  `issue.Number` (`ExternalID`) is stable across a rename but `HTMLURL` changes to the
  new slug. The existing unconditional-backfill design for `ExternalURL` (from that
  prior project) self-heals this on next successful sync — no new code needed here,
  just confirm this project's backward-sync doesn't *also* need special-case rename
  handling (it doesn't; a stale URL is a URL-persistence concern, not a status/label
  concern).
- **Issue transferred to a different repo**: GitHub preserves a transferred issue's
  original URL as a redirect but the issue's `number` in its *new* repo can be
  different from its original number (transfer assigns the next available number in
  the destination repo). This **breaks the `(sourceID, ExternalID)` lookup key
  outright** — the source's own `Fetch` (scoped to one fixed `owner/repo`) will
  simply never see the transferred issue again under its original number, and a
  transfer-in to that repo (from elsewhere) would land as a seemingly brand-new
  item with no relationship to history. Recommend documenting this explicitly as an
  accepted limitation (matching the style of the prior project's "known limitation"
  framing) — detecting a transfer would require polling by a different immutable key
  (GitHub's global `node_id`) which neither plugin currently fetches or stores, and
  is out of scope to add here.
- **Issue deleted** (rare — usually only actually deletable by repo admins, most
  "deleted" issues are actually just closed): a genuinely deleted issue disappears
  from `Fetch`'s result set entirely (whether via `state=all` or otherwise), same
  symptom as the pre-existing "closed issue closed before ever synced" known
  limitation in the prior project — the backlog item simply stops being touched by
  sync going forward, provenance/link becomes stale-but-harmless. Not worth special
  handling beyond what's already documented for that adjacent case.

### 3.4 Conflicting edits — local status change vs. GitHub state change before next tick

This is the central design question and the existing `UserModifiedFields` convention
(`session/backlog_sync.go:259-278`, confirmed read) already establishes the
project's answer for title/description/priority: **local wins, unconditionally,
once the user has touched that field** — there's no timestamp race or
last-write-wins comparison; presence in `UserModifiedFields` short-circuits the
external value entirely regardless of *when* the local edit happened relative to the
external one. The plan should extend this exact convention to status and labels
rather than inventing a new (e.g. timestamp-based) reconciliation strategy, since:
- It's simpler and already precedented/tested elsewhere in the same function.
- It avoids clock-skew/tick-timing ambiguity entirely (no need to compare an
  external `updated_at` against a local mutation timestamp).
- The one new wrinkle status introduces: today, `Status` changes only happen via
  `TransitionBacklogItemStatus` (a dedicated code path, not `UpdateBacklogItem`), so
  "was this field user-modified" for status needs its own tracking, distinct from
  the generic `UserModifiedFields` column used for title/description/priority —
  likely a boolean or timestamp set inside `TransitionBacklogItemStatus` itself
  whenever the transition is triggered by a human (vs. by sync). The
  `TriggeredBySystem` constant already referenced in `session/backlog_lifecycle.go:1358`
  (used for the archival auto-transition) shows the codebase already distinguishes
  "who/what triggered this transition" as a first-class concept — backward-sync's
  own transitions should use an equivalent `TriggeredBySync` (or similarly named)
  marker, both to avoid being mistaken for a user edit on the *next* sync tick (this
  is exactly the loop-prevention/echo-suppression concern from §2) and to support
  provenance display in the UI (AC2/AC5 area) and any future per-item audit trail.

### 3.5 Rate limiting / API failures mid-sync — partial writes

Already covered in §1: `Fetch` failing outright aborts the whole source's sync run
(recorded via `CreateSourceSyncEvent` with the error string) and no items are
processed at all for that tick — this is safe (no partial external state) but means
a large/rate-limited repo could go a full `defaultSyncInterval` (15 min) without any
forward progress. Within the `items` loop, each `CreateBacklogItem`/`UpdateBacklogItem`
call is independent, so a failure partway through only "loses" the items after the
failure point for *this* tick — they're retried next tick since the cursor only
advances past what was actually fetched, not what was successfully written per-item
(worth double-checking in the plan: does `newCursor` get set before or independent of
per-item write success? If `newCursor` advances based on `issue.UpdatedAt` regardless
of whether that specific item's write succeeded, a persistently-failing single item
could get "cursor-skipped" and never retried — this needs a precise read of
`FinishSourceSync`'s cursor semantics during planning, not assumed safe from this
research pass).

For the *new* forward-sync direction (backlog → GitHub write), a genuinely new
partial-write risk appears that doesn't exist in the pull-only design today:
closing the GitHub issue and applying a label are two separate API calls
(`PATCH /issues/{n}` for state, plus a labels endpint) — a network/rate-limit
failure between the two leaves the issue closed but unlabeled, or vice versa if
labels are applied first. The plan should specify: (a) do state and label changes in
a single `PATCH` request where the GitHub API allows it (the Issues API does support
setting `state` and `labels` in the same PATCH payload), collapsing this into one
atomic-from-the-client's-perspective call rather than two, and (b) if a partial
failure still occurs, whether/how it's retried on the next backlog transition event
vs. only on the next sync tick (a shipped backlog item doesn't re-trigger
`TransitionBacklogItemStatus` on its own, so a failed forward-sync write may need its
own small retry-queue or at least a visible "sync failed" indicator on the item,
tying back to the audit-trail gap in §1).

### 3.6 Unstated needs surfaced by this research

- **Per-item audit trail of sync actions** (not just per-source aggregate counts) —
  gap confirmed in §1; needed to answer "why did this item's status change" without
  digging through logs, and to support the loop-prevention/echo-suppression
  provenance marker described in §3.4.
- **Per-item sync opt-out**, not just per-source — a user may want most issues
  syncing but pin one specific item's status locally without disabling the whole
  source. No existing mechanism; would need a new per-item boolean distinct from
  `UserModifiedFields` (which only guards specific fields on the next *fetch-side*
  update, not a full opt-out of forward/backward sync both).
- **Dry-run/preview before enabling forward or backward sync** — given both are
  "user-controlled settings, default off" per AC3/AC4, a preview ("here's what would
  close/relabel right now if you turn this on") lowers the risk of an unpleasant
  surprise the first time a user flips the setting on a source with many
  already-imported items. Not in the numbered ACs; worth flagging to the planning
  phase as a nice-to-have, not a blocker.

## Summary of implications for planning

1. Reuse the existing `UserModifiedFields`/local-wins convention for status and
   labels rather than inventing timestamp-based conflict resolution — this is
   already the codebase's established idiom (`session/backlog_sync.go:259-278`) and
   matches the "local wins" simplicity industry bots converge on when true
   last-write-wins timing is hard to get right across polling cadences.
2. Backward-sync's closed→done (and any reopened→X) transition must route through
   the existing `validTransitions` state machine (`session/domain/backlog.go:331+`),
   not a hardcoded direct jump — skip-and-log when GitHub's 2-state model doesn't
   cleanly land on a valid local transition, rather than forcing one.
3. Loop prevention (AC7) should follow the "echo suppression via idempotent compare"
   pattern named in Jira/GitHub bot literature — a `TriggeredBySync` marker
   (parallel to the existing `TriggeredBySystem` used for archival) both prevents
   mistaking a sync-caused transition for a user edit on the next tick and gives the
   per-item audit trail a natural home.
4. Two known limitations should be explicitly documented (not fixed) rather than
   silently discovered later: (a) two `ItemSource`s pointed at the same repo produce
   independent, unaware duplicate imports; (b) a transferred GitHub issue breaks the
   `(sourceID, ExternalID)` lookup because GitHub can reassign the issue number on
   transfer.
5. Forward-sync's two GitHub API calls (state + labels) should collapse into a
   single PATCH where possible to avoid a half-applied write on failure; either way,
   a failed forward-sync write needs *some* visible signal (ties to the audit-trail
   gap), since a shipped item won't naturally re-trigger the write on its own.
