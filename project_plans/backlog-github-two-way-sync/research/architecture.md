# Architecture Research: Two-Way Status/Label Sync Between Backlog Items and GitHub Issues

**Date**: 2026-08-03
**Status**: Complete (research phase — no code changed)
**Scope**: the surface *beyond* `project_plans/backlog-github-issue-link/` (ExternalURL
persistence). That project's research/ADR-001/plan.md already fully design the
`ExternalURL` half and are cited, not re-derived, below. This doc covers: Labels
persistence, closed-issue observation, forward sync (status→GitHub), backward sync
(GitHub→status), per-source sync-direction settings, and loop prevention.

**Coordination note**: `backlog-github-issue-link`'s plan claims proto field number
`30` on `BacklogItem` (next after `category = 29`, `proto/session/v1/backlog.proto:150`)
is available for `external_url`. This project's `labels` field must claim `31`, not
`30`, if both projects land — whichever merges first should take `30` and leave a
comment; the second implementer must re-check the live proto before assigning a number.

---

## 1. Event-Command-Policy Table (EventStorming)

This domain has genuine multi-actor business rules (not CRUD plumbing), so — unlike
the ExternalURL project — a table is warranted.

| # | Event | Triggered By (Actor/System) | Policy (if...then) | Resulting Command | Handled Where |
|---|---|---|---|---|---|
| 1 | `IssueObservedClosed` — plugin `Fetch` returns an `ExternalItem` with `State: "closed"` | GitHub (external) → `SyncLoop.SyncOne` | Always (Fetch itself has no policy — see §2) | `RecordExternalState(extItem)` | `session/backlog_plugin_github.go` `Fetch` (query change), `ExternalItem.State` (new field) |
| 2 | `BacklogItemTransitionedToDone` | User (RPC) / `AutonomousOrchestrationService` / stuck-detector auto-transitions — all funnel through `EntRepository.TransitionBacklogItemStatus` | **IF** `source.ForwardSyncEnabled == true` **AND** item has non-empty `ExternalID`+`SourceID` whose plugin implements the close capability | `CloseExternalIssue(sourceID, externalID, closeLabel)` | New: EventBus subscriber in `server/services` (see §4) |
| 3 | `ExternalIssueClosed` (by our own forward-sync write) | Our own `CloseExternalIssue` command execution | Always — must not itself re-trigger event #2 | (writes GitHub only, no local state to record) | GitHub REST API only, no DB write — see §6 loop-prevention |
| 4 | `IssueObservedClosedExternally` — `Fetch` returns `State: "closed"` for an item **not already** locally `done`/`archived` | GitHub (external) → `SyncLoop.SyncOne`, next tick | **IF** `source.BackwardSyncEnabled == true` **AND** `!containsField(modifiedFields, "status")` (does not exist today — see §5 finding) **AND** `session.CanTransitionBacklog(existing.Status, target)` **AND** `TransitionGuard` passes | `TransitionBacklogItemStatus(id, target, precondition, TriggeredBySystem)` | `session/backlog_sync.go` `SyncOne`, new branch (see §5) |
| 5 | `IssueObservedReopenedExternally` — `Fetch` returns `State: "open"` for an item locally `done`/`archived` | GitHub (external) | **IF** `source.BackwardSyncEnabled == true` | `TransitionBacklogItemStatus(id, BacklogStatusInProgress, ..., TriggeredBySystem)` (or a narrower target — open plan question, §5) | Same branch as #4 |
| 6 | `IssueLabelsChangedExternally` | GitHub (external) | **IF** `source.BackwardSyncEnabled == true` **AND** labels differ from stored | `update.Labels = &data.Labels` (unconditional-style backfill, mirrors `ExternalURL`'s pattern from ADR-001) | `SyncOne`, existing-item branch, alongside the `ExternalURL` backfill (once that project lands) |
| 7 | `BacklogItemLabelsChangedLocally` (out of scope) | User edits labels in the UI | N/A — no forward-push of a local label edit to GitHub is in these ACs (only status→close+label-on-close, not general label push) | — | Not built — confirm with requirements before assuming symmetric label push |

Row 3 exists specifically so row 4/5's loop-prevention logic (§6) has a named "event
that must not re-trigger the policy that caused it" to reason about.

---

## 2. AC0: Observing closed/reopened issues

`Fetch` (`session/backlog_plugin_github.go:90`) currently hardcodes
`state=open` in the query string:

```go
url := githubAPIURL(cfg.Host, fmt.Sprintf("repos/%s/%s/issues?state=open&per_page=%d", ...))
```

**Change**: `state=all`. GitHub's Issues List API supports `state=all` combined
with `since=<cursor>` (the existing incremental-cursor mechanism, line 91-93) —
`since` filters by `updated_at` regardless of state, so this does not require
re-fetching the entire issue history each tick; a state flip (open→closed) bumps
`updated_at`, so it's naturally caught by the existing cursor.

**`githubIssue` struct** (line 46-55) needs `State string \`json:"state"\`` added
(GitHub's issue JSON has a top-level `"state": "open"|"closed"` field, not
currently decoded at all).

**`ExternalItem` struct** (`session/backlog_plugin.go:21-28`) needs a new field,
e.g. `State string` (values `"open"`/`"closed"`, mirroring GitHub's own vocabulary
rather than inventing a new enum — no other plugin needs to interpret it, and
`github_prs` is explicitly out of scope so it need not populate this field at all,
per interface-pollution-checklist's rule 1: don't force an unused field's
semantics onto a plugin that doesn't need it — it's fine for `GitHubPRsPlugin` to
just leave `State` as its zero value).

**Fetch's loop** (line 132-160) needs `State: issue.State,` added to the
`ExternalItem{}` literal (line 147-154), alongside the pre-existing
`Labels: labelNames,` (line 151) which is already computed but currently dropped
by `MapToBacklogItem` (see §3).

---

## 3. Labels persistence — mirrors the ExternalURL pattern exactly

`ExternalItem.Labels []string` (`session/backlog_plugin.go:25`) is **already
populated** in `Fetch` (`session/backlog_plugin_github.go:142-145,151`) — it is
computed purely to feed the `LabelPriorityMap` priority calculation (lines
134-140) and then silently dropped; `MapToBacklogItem` (lines 166-185) never
reads `item.Labels` at all. This is the same "already fetched, silently dropped"
shape ADR-001 fixed for `URL` — the fix is structurally identical:

1. **`session/ent/schema/backlog_item.go`**: add a new field. Precedent for a
   `[]string`-shaped ent field already exists in this codebase —
   `field.Strings("python_imports")` (`session/ent/schema/classificationanalytics.go:52`)
   generates a plain `PythonImports []string` on the entity (confirmed via
   `session/repository.go:251`, `ClassificationEventData.PythonImports []string`).
   Recommendation: `field.Strings("labels").Optional()`, next to
   `field.String("external_id")` (line 74-75) — **not** a JSON-string column like
   `AcceptanceCriteria`/`UserModifiedFields`, since `field.Strings` already gives a
   native `[]string` with no manual (un)marshal step, and there's no reason to
   introduce a second serialization convention for the same kind of data.
2. **`BacklogItemData`** (`session/repository.go:257-282`): add `Labels []string`
   next to `ExternalID string` (line 404, using the current unstaged line numbers
   from ADR-001's diff — will shift once that project's `ExternalURL` field lands
   first).
3. **`BacklogItemUpdate`** (`session/repository.go:504-...`): add `Labels *[]string`.
4. **`backlogItemToData`/`CreateBacklogItem`/`UpdateBacklogItem`**
   (`session/ent_repository_backlog.go`): identical `+1 line each` shape as
   ADR-001's table for `ExternalURL`.
5. **`GitHubIssuesPlugin.MapToBacklogItem`** (`session/backlog_plugin_github.go:166-185`):
   add `Labels: item.Labels,` to the returned literal. No truncation needed
   (unlike Title/Description/URL) — label names are already bounded by GitHub's
   own ~50-char label-name limit and issues rarely carry more than a handful, so
   there's no unbounded-growth risk analogous to a free-text body.
6. **Sync backfill** (`SyncOne`, `session/backlog_sync.go:259-283`): whether
   Labels should be a `containsField(modifiedFields, "title")`-style **local-wins
   gated** field (like Title/Description/Priority) or an **unconditional
   backfill** (like ADR-001's ExternalURL) is a genuine open design question this
   research surfaces rather than resolves:
   - Argument for local-wins-gated (grouped with Title/Description/Priority): a
     user editing an item's labels in the UI (if that UI exists — it doesn't
     today, see §7) should not have that edit silently overwritten by the next
     sync tick, exactly the reasoning that motivates `UserModifiedFields` for the
     other three fields.
   - Argument for unconditional (grouped with ExternalURL): per AC6, Labels is
     "provenance/observed-state" data like ExternalURL, not a field a human is
     expected to hand-edit independently of the source of truth (GitHub).
   - This doc's recommendation: gate Labels the same way as Title/Description/
     Priority (`containsField(modifiedFields, "labels")`), because AC4's backward
     sync explicitly says labels flow from GitHub→backlog "respecting the
     existing local-wins `UserModifiedFields` guard" — the requirements text
     itself groups Labels with the guarded fields, not with ExternalURL's
     unconditional-backfill treatment. The planning phase should confirm this
     against the final requirements wording before implementing.

---

## 4. Forward sync: backlog status transition → GitHub issue close

### 4.1 Where "done" is decided, and why the RPC handler alone is the wrong hook point

There are **two** candidate hook points, and only one of them is architecturally
correct:

**Candidate A (wrong on its own): inline in the RPC handler.**
`server/services/backlog_service_lifecycle.go:586-591` already has exactly this
shape of best-effort side-effect-on-terminal-transition:
```go
if to == session.BacklogStatusDone || to == session.BacklogStatusArchived {
    if sessions, lsErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId); lsErr == nil {
        s.cleanupItemWorktrees(ctx, sessions)
        s.archiveItemWorkSessions(ctx, sessions)
    }
}
```
This is tempting to copy, but `TransitionBacklogItemStatus` (the *storage-layer*
method, `session/ent_repository_backlog.go:879`) is called directly — bypassing
this RPC handler entirely — from at least four other places:
`server/services/autonomous_orchestration_service.go:506`,
`server/services/backlog_service_sync.go:122`,
`server/services/backlog_service_triage.go` (12+ call sites),
`session/backlog_lifecycle.go` (`archiveStaleDoneItems` and others, 8+ call
sites). A "done" transition driven by the autonomous orchestrator or the stuck-
detector's auto-resolution would never reach code added only inside the RPC
handler — forward sync would silently miss most real completion paths.

**Candidate B (correct): the existing `ItemChangePublisher`/EventBus fan-out.**
`EntRepository.TransitionBacklogItemStatus` itself
(`session/ent_repository_backlog.go:950-958`) already calls
`r.publishItemChanged(&result, BacklogItemChange{Kind: ChangeStatusTransition, OldStatus: fromStatus, NewStatus: string(toStatus)})`
unconditionally, on **every** call to this method regardless of caller. This
event flows through `BacklogItemEventPublisher.PublishItemChanged`
(`server/services/backlog_item_event_publisher.go:32`) onto the shared
`*events.EventBus`, which already supports multiple independent subscribers via
`bus.Subscribe(ctx)` — the exact pattern used today by
`server/analytics/subscriber.go:44`, `server/push/subscriber.go:28`,
`server/notifications/subscriber.go:43`, `server/services/backlog_service_events.go:83`,
`server/services/unfinished_work_service.go:154`, `server/services/session_service.go:2096`.

**Recommendation**: forward sync should be a **new** subscriber of this same kind
(e.g. `server/services/backlog_github_forward_sync.go`, a
`Subscribe(ctx)`-driven goroutine started alongside the others in
`server/dependencies.go`, near where `storage.SetItemChangePublisher(...)` is
wired at line 542), filtering for `events.BacklogChangeStatusTransition` with
`NewStatus == "done"`. This catches every transition path uniformly, is
consistent with the existing architecture's own answer to "what fires on every
state change regardless of caller," and needs zero changes to
`TransitionBacklogItemStatus`'s many call sites.

### 4.2 The close-issue capability: a narrow, consumer-defined interface, not a new `ItemSourcePlugin` method

`ItemSourcePlugin` (`session/backlog_plugin.go:6-13`) has exactly three methods
today: `PluginID`, `Fetch`, `MapToBacklogItem`. Two-way sync is explicitly a
**non-goal for `github_prs`** (requirements.md, "Non-goals"), so adding a fourth
method to the shared interface (e.g. `CloseIssue(...)`) would force
`GitHubPRsPlugin` to also implement it — either a real (unwanted) implementation
or a dummy no-op, which
`.claude/rules/interface-pollution-checklist.md`'s rule 1 ("speculative
interface... no near-term second one") and rule 4 (forwarding-only wrapper) both
flag as exactly the anti-pattern to avoid.

**Recommendation**: define a narrow interface *at the consumer* (the new
forward-sync subscriber in `server/services`, per that same checklist's point 2 —
"define the interface where it's consumed"), e.g.:
```go
// server/services/backlog_github_forward_sync.go
type externalIssueCloser interface {
    CloseIssue(ctx context.Context, config session.PluginConfig, externalID string, label string) error
}
```
The subscriber looks up the plugin via `registry.Get(source.PluginID)` (mirroring
`SyncLoop.SyncOne`'s existing lookup, `session/backlog_sync.go:207`) and does a
type assertion: `closer, ok := plugin.(externalIssueCloser)`. If `ok` is false
(e.g. the source is `github_prs`), forward sync is a structural no-op — no dummy
method needed anywhere, and the "PRs are out of scope" non-goal is enforced by
the type system rather than a runtime `if pluginID == "github_prs" { return }`
check. Only `*GitHubIssuesPlugin` needs a new `CloseIssue` method, implemented
with the same `githubAPIURL`/bearer-token/`http.Client` pattern `Fetch` already
uses (`session/backlog_plugin_github.go:95-107`), but issuing
`PATCH /repos/{owner}/{repo}/issues/{number}` with body
`{"state":"closed","labels":[...]}` (GitHub's Issues API accepts a labels array
in the same PATCH as the state change — no second request needed for AC3's
"optionally applies a configured label").

### 4.3 Where the token/config comes from

The subscriber only has `BacklogItemData.SourceID` from the event payload — it
must load the full `ItemSource` (`storage.GetItemSourceByID` /
`er.GetItemSourceByID`, already used by `SyncLoop.runAllSources`,
`session/backlog_sync.go:110`) and decrypt its `Config` the same way
`SyncLoop.decryptConfigToken` does (`session/backlog_sync.go:129-174`). That
method is currently an unexported method on `*SyncLoop` with no free function
equivalent — either export a package-level `DecryptSourceConfigToken(raw string,
keyFunc func() ([]byte, error)) (string, error)` helper both `SyncLoop` and the
new subscriber can call, or give the subscriber its own `*SyncLoop` reference
(it already needs `PluginRegistry` access) and call `sl.decryptConfigToken`
directly if the subscriber is constructed with a `*SyncLoop` handle — the latter
is less new surface area and is the recommended shape.

---

## 5. Backward sync: GitHub issue state → backlog item status — a hard constraint the requirements don't currently account for

### 5.1 `BacklogItemUpdate` deliberately has no `Status` field

Confirmed by reading the full struct (`session/repository.go:504-...`, ~50
fields): there is **no** `Status *string` field anywhere on `BacklogItemUpdate`.
This is not an oversight — `SyncOne`'s own comment says so explicitly
(`session/backlog_sync.go:277-278`, *"Status is always local-wins... Status
transitions are only done via TransitionBacklogItemStatus — no update here"*).
Every status change in the entire codebase goes through
`Storage.TransitionBacklogItemStatus` → `EntRepository.TransitionBacklogItemStatus`,
which enforces two independent guards before writing:
1. **`WorkflowEngine.CanTransition(from, to)`** (`session/workflow_engine.go:37`,
   backed by the static `validTransitions` table, `session/domain/backlog.go:331-388`)
   — a structural adjacency check (e.g. `idea → done` is not a valid edge at
   all).
2. **`TransitionGuard(item, to)`** (`session/domain/backlog.go:445-501`) — business
   rules. Critically, `to == BacklogStatusDone` (**every** edge that reaches
   "done", both `review→done` and `pr_pending→done`) requires
   `item.OverallOutcome == ReviewOutcomePass` **and** `!item.HasUnshippedCode`,
   unless `OverrideReason != ""` is explicitly supplied.

**Consequence for AC4**: backward sync cannot simply map "GitHub issue closed" →
"transition backlog item to done" as a blanket rule. For an item still in
`idea`/`refining`/`ready`/`in_progress`/`queued` status, `CanTransition` itself
already rejects a direct jump to `done` (no such edge exists in
`validTransitions` — only `review`/`pr_pending` can reach `done`). For an item
already in `review`/`pr_pending`, `TransitionGuard` will additionally reject the
transition unless a genuine PASS verdict and shipped code already exist — which,
for an item whose backlog-side work was never actually finished, it usually
won't. **A blind "closed→done" backward-sync rule would fail silently (or
worse, need `OverrideReason` to force through, defeating the guard's purpose)
for the majority of realistic cases** — someone closing a GitHub issue as
"not going to do this" or "duplicate" is not the same signal as "the code shipped
and passed review."

**Recommendation for the planning phase** (not resolved here, flagged as the
single most important open question this research surfaces): backward sync's
"closed" policy should attempt the transition through the normal guarded path
(`TransitionBacklogItemStatus` with `TriggeredBySystem`) and treat a guard
rejection as an **expected, logged, non-error outcome** — consistent with the
existing `//nolint:silenttransition` idiom already used at several call sites
for exactly this "best-effort, not fatal" shape (e.g.
`session/backlog_lifecycle.go:1358`, `archiveStaleDoneItems`). Concretely: only
items already sitting in `review`/`pr_pending` with a genuine PASS verdict would
actually flip to `done` from an externally-closed issue; earlier-stage items
would need a different mapped target (e.g. `BacklogStatusArchived`, which *is*
reachable from every status per `validTransitions`, with no `TransitionGuard`
special case — semantically "closed without shipping" maps more honestly to
"archived" than to a spuriously-forced "done"). This status-mapping decision is
a product/requirements question, not an architecture one, and should be settled
explicitly before implementation, not left to whichever branch happens to compile.

### 5.2 `user_modified_status_at` is not actually a functioning guard today — a pre-existing gap the requirements' AC4 wording assumes is fixed

Requirements AC4 says backward sync must respect "the existing local-wins
`UserModifiedFields` guard." Two things are true simultaneously, confirmed by
grepping every reference to `user_modified_status_at` in the codebase:
- The DB column and struct field exist (`session/ent/schema/backlog_item.go:76-78`)
  and are **written unconditionally** on every single transition, in both write
  paths (`session/storage_backlog.go:633` and
  `session/ent_repository_backlog.go:906`), regardless of `triggeredBy` — i.e. a
  `TriggeredBySystem` transition (autonomous orchestrator, stuck detector) sets
  this timestamp exactly the same as a `TriggeredByUser` one.
- **Nothing in the codebase ever reads `UserModifiedStatusAt` as a gating
  condition.** It is a pure write-only audit timestamp today, not a functioning
  "has a human touched this" signal despite its name implying otherwise.
  `UserModifiedFields` (the JSON string field actually consulted by
  `containsField` in `SyncOne`, `session/backlog_sync.go:260,265,269,273`) is a
  **separate** field/mechanism that, per its own schema comment
  (`session/ent/schema/backlog_item.go:69-71`, "JSON set of field names modified
  by the user"), is scoped to Title/Description/Priority — status is
  structurally excluded from it (there is no `"status"` entry ever added to that
  set anywhere in the codebase).

**Consequence**: there is currently no existing "local-wins for status" signal
to reuse at all — `UserModifiedFields`/`containsField` cannot be extended to
cover `"status"` the same way as Title/Description/Priority, because status
changes never go through `UpdateBacklogItem` (the only place `containsField` is
consulted) in the first place; they go through `TransitionBacklogItemStatus`,
which has no local-wins concept whatsoever. Backward sync's "don't clobber a
user's manual status change" protection must be **newly built**, most likely by
checking recency of `UserModifiedStatusAt` against the sync tick (e.g. "don't
backward-sync status if the item transitioned within the last N minutes,
regardless of who/what triggered it") — but since that timestamp doesn't
distinguish user- from system-triggered transitions today, this protection will
be approximate at best (a recent autonomous-driver transition looks identical to
a recent manual one). Flagging this precisely so the plan phase doesn't assume
`UserModifiedFields`-style status gating already half-exists — it doesn't.

---

## 6. Loop prevention (AC7)

Two distinct thrash risks, not one:

**Risk A (benign, self-resolving): forward-sync-closes → next tick's backward
sync re-observes "closed."** After `CloseExternalIssue` fires (event #2/#3 in
§1's table), the *next* `Fetch(state=all)` will legitimately return that issue
as closed. Backward sync (event #4) would then attempt
`TransitionBacklogItemStatus(id, <target>, TriggeredBySystem)` — but the item is
already at its terminal status (`done`, the status forward-sync fired *because*
of), and `validTransitions` has no self-edge (`done → done` is absent from the
table, `session/domain/backlog.go:377-384`), so `CanTransition` returns `false`
and the call is a structural no-op. **No new guard is needed for this case** —
the existing state machine's lack of self-transitions is sufficient, and this
should be pinned with a regression test rather than assumed.

**Risk B (real, needs an explicit guard): a `done` item is manually reopened,
but the GitHub issue is never reopened.** `done → in_progress` (and
`done → ready`/`refining`/`idea`) are all valid backward edges with **no**
`TransitionGuard` special case (`session/domain/backlog.go:377-384`) — a user can
freely reopen a shipped item. Forward sync only fires on transitions *into*
`done` (§4), never on transitions *out* of it, so the GitHub issue stays closed.
On the *next* sync tick, `Fetch(state=all)` still reports that issue as
`"closed"` (nothing reopened it), and naive backward-sync logic would see
"closed, item not done" and attempt to push the item **back** to `done` —
silently undoing the user's manual reopen. This is the actual infinite-loop risk
AC7 is worried about, and it is not prevented by anything described in §5 or
Risk A above.

**Recommendation**: backward sync's "closed" policy (§5.1) must additionally
check that the local transition *into* the current terminal-ish state did not
already originate from our own forward sync for *this exact* GitHub state — the
cleanest mechanism is a **watermark**, analogous to the existing
`PrFeedbackAddressedAt` pattern (`session/repository.go:422-426`, "the newest
substantive PR review-feedback timestamp a fix session has already been
dispatched to address" — the exact same shape of problem: "don't re-react to
external state we've already reacted to"). Concretely: record
`ForwardSyncClosedAt *time.Time` (or reuse `ShippedSnapshotAt` if its semantics
already line up) when forward sync closes the issue, and have backward sync
skip re-applying "closed" if the item's `UpdatedAt`/`UserModifiedStatusAt` is
*newer* than that watermark (i.e., something happened to the item — like a
manual reopen — after our own forward-sync write). This is a new field, not
present in requirements.md's AC list, and should be raised explicitly as a
planning-phase addition rather than assumed solved by "idempotent" language in
AC7's current wording, which (per Risk A vs Risk B above) only actually covers
Risk A.

---

## 7. Per-source sync-direction settings — first-class `ItemSource` fields, not buried `Config` JSON

### 7.1 `Config` is deliberately write-only to the client today — a hard constraint on where new *visible* settings can live

`ItemSourceData.Config` (`session/repository.go:581`) is opaque
plugin-interpreted JSON (`githubPluginConfig`,
`session/backlog_plugin_github.go:37-43`, currently `Host`/`Owner`/`Repo`/`Token`/
`LabelPriorityMap`) that also carries the encrypted PAT. Confirmed by reading the
full proto surface: `ItemSource` (`proto/session/v1/backlog.proto:155-164`) has
**no** `config_json` field, and neither does `UpdateItemSourceRequest`
(`proto/session/v1/backlog.proto:509-514`, only `display_name`/`enabled`/`token`)
— `config_json` exists only on `CreateItemSourceRequest`
(`proto/session/v1/backlog.proto:492-497`) and is **never read back**. This
appears deliberate (avoid re-exposing an encrypted token, even at rest, to the
client) — `server/services/backlog_service_lifecycle.go:713-720` shows
`UpdateItemSource` treats config as "replace wholesale, no prior config to
merge" specifically because the client never had it to merge from.

**Consequence**: a sync-direction toggle **cannot** be added inside `Config` and
still be visibly editable/inspectable from the Settings UI — there is currently
no RPC path that would let the frontend read back an existing source's current
config state at all. The existing `BacklogSourcesSettings.tsx` UI structurally
only supports write-once-at-creation config (`schema.fields`,
`web-app/src/components/settings/backlogSourceSchemas.ts:22-42`) plus a
**separate**, already-first-class `Enabled` toggle
(`role="switch"` at `BacklogSourcesSettings.tsx:143-149`, wired through
`setItemSourceEnabled` → `updateItemSource({sourceId, displayName, enabled,
token: ""})`, `web-app/src/lib/hooks/useBacklogSourcesService.ts:125-144`).

### 7.2 Recommendation: mirror the `Enabled` field exactly

Add `ForwardSyncEnabled bool` and `BackwardSyncEnabled bool` (both `Default(false)`
per AC3/AC4's "default off") as genuinely new, first-class fields threaded
through the **same** five places `Enabled` already goes:
1. `session/ent/schema/item_source.go:29-30` — new `field.Bool("forward_sync_enabled").Default(false)` / `field.Bool("backward_sync_enabled").Default(false)`, next to `field.Bool("enabled")`.
2. `ItemSourceData`/`ItemSourceUpdate` (`session/repository.go:577-594`) — new fields alongside `Enabled bool`/`Enabled *bool`.
3. `proto/session/v1/backlog.proto` — add to both `ItemSource` (next available field number after `updated_at = 8`) and `UpdateItemSourceRequest` (next after `token = 4`), then `make proto-gen`.
4. `server/services/backlog_service_lifecycle.go:663-733` (`CreateItemSource`/`UpdateItemSource` handlers) — thread the two new bools the same way `Enabled` already is at line 711-712.
5. `web-app/src/components/settings/BacklogSourcesSettings.tsx` — two more `role="switch"` toggles per source row, next to the existing enabled toggle (line 143-149), each calling a new hook function mirroring `setItemSourceEnabled` (`useBacklogSourcesService.ts:125-144`).

This keeps the two directions **visible and independently toggleable** (unlike
anything living in `Config`), satisfies AC5's "configurable in Settings >
Backlog Sources" literally (a real, readable, per-source UI control, not a
write-blind text field), and requires no change to the write-only-`Config`
architecture at all.

The **optional forward-sync close-label** (AC3, "optionally applies a configured
label") should follow the identical pattern — a third field,
`ForwardSyncCloseLabel *string`/`string` (empty = no label applied) — for the
same read-back-visibility reason, **not** folded into `LabelPriorityMap` or
`githubPluginConfig`'s opaque JSON, which the settings UI has no path to
redisplay once saved.

`SyncOne` already receives the full `*ent.ItemSource` (`session/backlog_sync.go:195`),
so once these fields exist on the ent entity, `source.BackwardSyncEnabled` is
directly readable inside `SyncOne` with zero additional plumbing — only the
forward-sync subscriber (§4, which works from the `BacklogItemChange` event
payload's `SourceID`, not a full `ent.ItemSource`) needs the extra
`GetItemSourceByID` lookup already described in §4.3.

---

## 8. Summary of new/changed structures

| Structure | Change | Precedent cited |
|---|---|---|
| `session/ent/schema/backlog_item.go` | `+field.Strings("labels").Optional()` | `classificationanalytics.go:52` (`python_imports`) |
| `session/ent/schema/item_source.go` | `+3 field.Bool`/`field.String` (forward/backward enabled, close label) | `enabled` field itself, same file |
| `BacklogItemData`/`Update` | `+Labels []string` / `*[]string` | ADR-001's `ExternalURL` shape |
| `ItemSourceData`/`Update` | `+ForwardSyncEnabled`, `+BackwardSyncEnabled`, `+ForwardSyncCloseLabel` | `Enabled bool` shape |
| `githubIssue` (unexported struct) | `+State string \`json:"state"\`` | sibling `HTMLURL`, `UpdatedAt` fields |
| `ExternalItem` | `+State string` | sibling `Labels []string` (already exists, unused downstream) |
| `GitHubIssuesPlugin.Fetch` query | `state=open` → `state=all` | — |
| `GitHubIssuesPlugin.MapToBacklogItem` | `+Labels: item.Labels,` | ADR-001's `ExternalURL: url,` addition |
| `GitHubIssuesPlugin` (new method) | `+CloseIssue(ctx, config, externalID, label) error` | `Fetch`'s existing `githubAPIURL`/auth pattern |
| New narrow interface `externalIssueCloser` | defined in `server/services` (consumer), not on `ItemSourcePlugin` | `.claude/rules/interface-pollution-checklist.md` rules 1+2 |
| New EventBus subscriber | `server/services/backlog_github_forward_sync.go` (name TBD), subscribes to `events.BacklogChangeStatusTransition` | `server/analytics/subscriber.go`, `server/push/subscriber.go`, `server/notifications/subscriber.go` |
| `SyncOne` existing-item branch | `+` backward-sync block: attempt guarded `TransitionBacklogItemStatus` for status; `+Labels` local-wins-gated field, alongside (not instead of) ADR-001's `ExternalURL` unconditional backfill | ADR-001 (structure), §5.1/§6 (this doc, for the guard/loop-prevention wrinkles ADR-001 didn't need to consider) |
| `proto/session/v1/backlog.proto` | `BacklogItem.labels` (field `31`, see coordination note), `ItemSource.{forward,backward}_sync_enabled` + `forward_sync_close_label`, `UpdateItemSourceRequest` same 3 fields | — |
| `web-app/.../BacklogSourcesSettings.tsx` | 2 new toggle switches + 1 text input per source row | existing `enabled` toggle, same file |

## 9. Open questions flagged for the planning phase (not resolved here)

1. **§3**: is Labels local-wins-gated (like Title/Description/Priority) or
   unconditional (like ExternalURL)? This doc recommends gated, based on AC4's
   literal wording, but the plan phase should confirm against the final
   requirements before implementing.
2. **§5.1**: what backward-sync status target should a "closed" GitHub issue map
   to for an item *not* already in `review`/`pr_pending` with a passing verdict —
   `archived` (this doc's suggestion) or something else? This is a product
   decision the guard rules force to the surface; it cannot be "done" for most
   realistic cases without either weakening the guard or using `OverrideReason`
   (both undesirable).
3. **§6, Risk B**: the manual-reopen-after-forward-sync-close loop needs a new
   watermark field (`ForwardSyncClosedAt` or similar) not currently in
   requirements.md's AC list — flagged as a planning-phase addition.
4. **§7.2**: exact proto field numbers for the three new `ItemSource`
   fields/two `UpdateItemSourceRequest` fields, and the `BacklogItem.labels`
   field number `31` vs whatever `backlog-github-issue-link` actually claims for
   `external_url` if it lands first — must be checked against the live `.proto`
   file at implementation time, not assumed from this doc's numbers.
