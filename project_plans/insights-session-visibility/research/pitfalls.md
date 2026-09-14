# Pitfalls Research: insights-session-visibility

## 1. Proto/codegen pitfalls

- `make proto-gen` (`Makefile:535-548`) runs `buf generate proto` whenever any
  `proto/*.proto` file is newer than `.proto-gen.stamp`, or the Go/TS output is
  missing. Output: `gen/proto/go/` (Go) and `web-app/src/gen/` (TypeScript) — per
  `.gitignore:31-34`, both directories (plus `web/src/gen/` and
  `.proto-gen.stamp` itself) are gitignored, never committed.
- Failure mode if a dev edits `insights.proto` and forgets to run `proto-gen`:
  every Make target that touches Go or the web app (`build`, `test`,
  `lint`, `web-app/out`, etc.) already lists `proto-gen` as a prerequisite
  (`Makefile:177,196,574,786,871`), so a plain `make build`/`make test`/`make
  lint` self-heals — regeneration happens transparently before the file-mtime
  check would ever surface a "stale" state to a human. The realistic failure
  mode is instead: running `go build .` or `go vet ./...` **directly** (bypassing
  Make) against a stale `gen/proto/go/session/v1/insights.pb.go` that predates
  the new `tags`/`session_role` fields — this produces a confusing "undefined:
  sessionv1.SessionTokenSummary.Tags" compile error that looks like a typo, not
  a codegen-staleness issue, because nothing in the error message points at
  proto-gen. Same risk on the TS side if `pnpm` scripts are run directly instead
  of via a Make target that depends on `proto-gen`.
- CI backstop: `.github/workflows/generated-proto-guard.yml` fails any PR that
  commits a file under `gen/`, `web/src/gen/`, or `web-app/src/gen/` (diff-based,
  via the GitHub compare API — no checkout needed). This guards against
  accidentally `git add -f`-ing generated output (has happened twice before per
  that workflow's own comment, PR #445), not against forgetting to regenerate
  locally — that's caught by `make build`/`make test` failing to compile, not by
  CI silently accepting stale code (CI itself always runs `proto-gen` fresh
  before every consuming step, per `.github/workflows/lint.yml`).
- **Recommendation**: no special handling needed beyond the existing workflow —
  edit `proto/session/v1/insights.proto`, run `make proto-gen` (or let `make
  build`/`make test` do it), then `go build ./...` to confirm. Do not `git add
  -f` anything under `gen/`.

## 2. Performance pitfall — N+1 backlog lookups for `session_role`

**This is the single highest-risk item in this project.** The codebase has
already hit and fixed this exact class of bug once, for the *existing*
`sessionId`/`isOrphan` resolution — the fix is the template to reuse for
`session_role`, not a novel pattern to invent.

- `buildSessionSummary()` (`server/services/insights_service.go:82-137`) is
  called once per session inside three separate call sites:
  `GetInsightsSummary`'s loop (`insights_service.go:169-` after
  `sessionSnapshot := s.associator.Snapshot()` at line ~168),
  `ListSessionTokens`'s loop (`insights_service.go:448-478`, same
  `sessionSnapshot` pre-fetch before the `for _, r := range results` loop), and
  `watchInsights`'s per-event branch (`insights_service.go:610-620`), which
  calls `s.associator.Snapshot()` fresh on every single streamed update event
  (one session at a time, so a full re-snapshot per event is cheap there, but
  see below).
- The existing snapshot pattern (`session/tokens/association.go`): `Associator.
  Snapshot()` (lines 51-58) fetches all session records **once** per
  request/build-call; `AssociateWithSnapshot()` (lines 65-70) then matches each
  `ParseResult` against that in-memory slice with no further storage queries.
  The doc comment on `Snapshot()` names the exact bug this replaced: *"Both
  ListInstancesFiltered-style loops in InsightsService previously called
  Associate per result, each paying a fresh `ListSessionRecords() ->
  ListInstanceData()` full-repository scan."* — i.e., `session_role` sourcing
  is about to reintroduce the identical bug class in a different lookup unless
  it's batched the same way.
- The naive implementation risk: `session/storage.go:1195-1198`
  (`Storage.GetItemSessionBySessionUUID`) is a **per-UUID** ent query
  (`session/storage_backlog.go:351-364`: `r.client.ItemSession.Query().
  Where(itemsession.SessionUUID(sessionUUID)).WithBacklogItem().
  Order(ent.Desc(...)).First(ctx)` — one round trip per call). Calling this once
  per session inside `buildSessionSummary`'s callers turns an O(n) in-memory
  loop into O(n) synchronous SQL queries per `GetInsightsSummary`/
  `ListSessionTokens` request — and `GetInsightsSummary` is unfiltered
  (`s.store.GetAll()`) before any time/model filter is applied, so `n` is every
  parsed session in the token store, not a paginated slice.
- **The fix already exists as a template in this exact file**:
  `GetBaseCommitSHAsForSessions` (`session/storage_backlog.go:327-345`) is a
  **batched** lookup with the identical shape needed here — `WHERE
  session_uuid IN (...)` via `itemsession.SessionUUIDIn(sessionUUIDs...)`,
  `.Select(...)` only the needed fields, `.All(ctx)`, folded into a
  `map[string]string` keyed by session UUID. `Storage.GetBaseCommitSHAsForSessions`
  (`session/storage.go:1191-1193`) exposes it at the `Storage` layer.
  **Recommendation**: add a parallel `GetSessionRolesForSessions(ctx,
  sessionUUIDs []string) (map[string]string, error)` (or fold role into the
  existing base-commit-SHA batch call) selecting `itemsession.FieldSessionRole`
  too, called **once** per `GetInsightsSummary`/`ListSessionTokens` request
  (alongside the existing `s.associator.Snapshot()` call) and passed into
  `buildSessionSummary` as a plain map, exactly mirroring how `sessionSnapshot`
  is threaded through today.
- **A subtlety the template itself doesn't handle correctly**: per
  `GetItemSessionBySessionUUID`'s own doc comment (`session/storage_backlog.go:
  347-349`), *"session_uuid is not unique across records (a session may be
  reused)"* — that single-row lookup explicitly orders by `created_at desc` and
  takes the first row to get the *latest* role. `GetBaseCommitSHAsForSessions`
  does **not** do this — it has no `.Order()` call, so if a session UUID has
  multiple `ItemSession` rows, which row's value lands in the map depends on
  arbitrary DB iteration order, not "most recent." A new `session_role` batch
  function copied from `GetBaseCommitSHAsForSessions` verbatim would silently
  inherit this latent bug. Flag for Phase 3 planning: either add `.Order(ent.
  Desc(itemsession.FieldCreatedAt))` and overwrite-per-key in iteration order
  (last-write-wins = most-recent, since ent doesn't guarantee `.All()` result
  order without an explicit `Order()` — must set it explicitly, not rely on
  incidental ordering), or accept this as a known, pre-existing sharp edge
  inherited from the base-SHA precedent and document it, but don't propagate it
  silently as if it were correct.
- `watchInsights`'s per-event path (`insights_service.go:585-641`) rebuilds one
  `SessionTokenSummary` per streamed update and already re-fetches
  `s.associator.Snapshot()` on every event (a full session-list re-scan per
  single-session update — pre-existing behavior, out of this project's scope
  to fix, but worth naming): adding a `session_role` DB query per event here is
  a single extra query per event, not O(n) — acceptable, unlike the two bulk
  RPC paths above where per-session queries would multiply by session count.

## 3. Backward compatibility / field-numbering pitfalls

- Current message (`proto/session/v1/insights.proto:69-96`): fields run 1-20
  contiguously, with `waste_score` at 19 as `optional double` and
  `activity_type` at 20 as a plain (non-optional) `ActivityType` enum — the
  last used number. New fields must be appended as 21 and 22 (in whichever
  order matches the plan), never renumbered or inserted, and never reusing 1-20.
- Confirmed single Go consumer: `server/services/insights_service.go` (per
  requirements.md's own verification, re-confirmed here via grep — no other
  package imports `sessionv1.SessionTokenSummary`). TS consumers confined to
  `web-app/src/app/insights/*` and `tests/e2e/pages/InsightsPage.ts`. This
  means the two new fields are purely additive from a wire-compat standpoint —
  no existing consumer breaks by their mere presence, proto3's default "unset
  fields decode as zero value" behavior is sufficient, and no `reserved`
  numbers are needed since nothing is being removed.
- **`tags` field**: should be `repeated string tags = 21;` — plain, not
  wrapped in any `optional`/wrapper message. Proto3 `repeated` fields have no
  presence distinction to begin with (an absent/never-set repeated field and
  an explicitly-empty one are wire-identical, decoding to `[]string{}` in Go
  and `[]` in TS) — there is no "nil vs empty" ambiguity to design around, so
  ADR-002's optional-field precedent (see below) does not apply here.
- **`session_role` field**: should be a plain `string session_role = 22;`, not
  wrapped in `optional`. Reasoning by direct analogy to ADR-002
  (`project_plans/insights-cost-intelligence/decisions/ADR-002-findings-non-summable-dollar-impact.md`)
  and `ComputeWasteScore`'s own doc comment
  (`session/tokens/findings.go:268-276`): `waste_score` needed `optional`
  specifically because `0.0` is itself a legitimate, distinguishable score
  value ("evaluated and clean") that would otherwise collide with "not
  evaluated" if a bare `double` defaulted to `0`. `session_role` has no
  equivalent collision: the valid role values (`session.SessionRoleWork =
  "work"`, `SessionRoleTriage = "triage"`, `SessionRoleReview = "review"`,
  `SessionRoleJulesWork = "jules_work"`, all defined `session/backlog.go:
  50-53`) are all non-empty strings, so proto3's zero-value default for an
  unset string field (`""`) is unambiguous as "no role attributed" (either a
  non-backlog ad hoc session, or one whose `ItemSession` row was hard-deleted —
  see §5) and can never be confused with a real role. The precedent that
  motivated `optional` for `waste_score` doesn't transfer here — don't apply it
  reflexively just because it's the message's most recent optional-field
  addition.
- No other proto message anywhere in `proto/session/v1/` needs touching for
  this project — `tags`/`session_role` live only on `SessionTokenSummary`.

## 4. Frontend pitfalls

### Duplication gate (dupl/jscpd) risk from repeated tooltips

- All Insights tooltips today are bare HTML `title=` attribute strings, not a
  shared component — e.g. `SessionsTable.tsx:298` (`title={\`${backlogEntry.
  sessionRole}: ${backlogEntry.itemTitle}\`}`), `SessionsTable.tsx:309`
  (cache read/write breakdown), `SessionDetailContent.tsx:141`, and
  `SessionDetailContent.tsx:178` (`EstimatedValue title="..."`). Adding a
  proper hover tooltip for "Waste Score" (plus whatever else the tooltip audit
  finds missing) by copy-pasting a `title="..."` string is low duplication
  risk by itself, but copy-pasting a *richer* tooltip implementation (e.g. a
  wrapped `<span>` + event handlers, if the plan goes beyond a native `title`)
  across 2+ header cells is exactly the shape jscpd's 20-line/200-token
  threshold (`web-app/.jscpd.json`, per root `CLAUDE.md`) is tuned to catch.
- **A shared, unused Tooltip component already exists**:
  `web-app/src/components/ui/Tooltip.tsx` — a thin Radix
  (`@radix-ui/react-tooltip`) wrapper (`Tooltip({children, label, side})`,
  hover/focus-triggered, portal-rendered). Verified via `grep -rl
  "components/ui/Tooltip" web-app/src/app/insights/` — **zero** matches; no
  Insights component uses it today. **Recommendation**: route the new Waste
  Score tooltip (and any others found in the audit) through this existing
  component instead of inline `title=` strings or a new bespoke
  implementation — both to sidestep the jscpd gate (one shared component, N
  call sites, not N copies) and because native `title` tooltips are not
  keyboard-focusable and don't satisfy WCAG 2.1 SC 1.4.13 (Content on Hover or
  Focus — must be dismissable, hoverable, and persistent), which this repo's
  own UX-analysis CI (Axe Core, blocking on WCAG AA violations per root
  `CLAUDE.md`'s E2E Tests section) is positioned to eventually flag even if it
  doesn't today.

### Chart accessibility (token-type breakdown)

- `recharts` (`^3.8.1`, `web-app/package.json:104`) is already a dependency
  and already used for stacked/categorical visuals elsewhere in Insights
  (`ModelBreakdownChart.tsx` uses `BarChart`/`Cell` with a `fill={entry.color}`
  per-segment pattern) — no new library needed, and that file is a ready
  template for a stacked token-type bar.
- Two concrete accessibility risks for a stacked input/output/cache-creation/
  cache-read bar, per WCAG SC 1.4.1 (Use of Color) and 1.4.11 (Non-text
  Contrast): (1) cache-creation/cache-read values are frequently very small
  relative to input/output on non-cache-heavy sessions — a color-only-encoded
  sliver a few pixels wide is not a reliable way to distinguish two adjacent
  segment types when one segment is near-invisible; pair color with a legend
  and/or an accessible text equivalent (the raw numbers, already computed,
  should back a hover/tooltip or an adjacent text summary, not disappear once
  a visual exists) rather than relying on the bar rendering alone to convey
  composition. (2) segment fill colors must carry sufficient contrast against
  each other and the container background in both light and dark theme (this
  app supports both, per its general dark-mode conventions) — verify segment
  colors are chosen from a categorical palette with contrast checked in both
  themes, not just picked to "look different" in light mode.
- **Recommendation**: follow the `dataviz` skill (already required by
  requirements.md's Scope §1) for palette selection, and ensure the raw
  token counts remain available as text (tooltip or table) alongside the
  visual — never color-only.

### E2E test conventions / locator breakage risk

- `tests/e2e/pages/InsightsPage.ts:118-120` — `getColumnHeader(name)` resolves
  via `page.getByRole("columnheader", { name })`, and `:123-125` —
  `getSortableColumnControl(name)` resolves via `page.getByRole("button", {
  name })`. Both match on **accessible name**, computed from visible text
  content (or an overriding `aria-label`) of the matched role.
- `tests/e2e/insights-sessions-table-sort.spec.ts` uses both against `/Waste
  Score/i` at lines 23, 25, 91, 117 (sort-toggle click + `aria-sort` assertion
  + row-order assertions after sorting).
- Current header markup (`SessionsTable.tsx:240-262`,
  `sortableHeaderCell(col, label)`): a `<th aria-sort=...>` containing a single
  `<span role="button" tabIndex={0} onClick=... onKeyDown=...>{label}
  {sortIndicator(col)}</span>` — i.e. the sortable control *is* the accessible
  name source for both locators above (the `<th>`'s columnheader name and the
  inner `<span role="button">`'s button name both derive from the same text
  node).
- **Concrete risk**: adding a tooltip trigger to the "Waste Score" header by
  nesting a second interactive element (e.g. an info icon as its own `<button>`
  or tooltip trigger) *inside* the existing `role="button"` span creates a
  button-in-button structure — invalid ARIA (interactive-in-interactive), and
  it also changes the accessible-name computation: if the new element carries
  its own `aria-label` that doesn't include "Waste Score" verbatim, or if the
  icon is announced as a separate name segment, `getByRole("button", { name:
  /Waste Score/i })` may still match on substring (Playwright's `name` does
  substring/regex matching against the *concatenated* accessible name by
  default), but this should be verified rather than assumed once the tooltip
  markup is decided — and the invalid nested-interactive-element structure
  should be avoided regardless, since it also fails the UX-analysis CI's Axe
  Core gate (`.github/workflows/ux-analysis.yml`, blocks on WCAG AA). Prefer
  wrapping the *whole* existing `<span role="button">` in the `Tooltip`
  component (trigger = the existing sort control, no new nested interactive
  element added) so neither the accessible name nor the interactive structure
  changes.
- No e2e spec currently asserts on tag-filter UI or a token-type chart (none
  exists yet), so the tag-filter and chart additions carry no *existing*
  locator-breakage risk — only the Waste Score tooltip touches an already-
  tested column header.

## 5. Session-role backfill / migration pitfall (expectation-setting, not a blocker)

- **Archival does not delete the data `session_role` needs.** Backlog item
  archival (`BacklogLifecycleListener.archiveStaleDoneItems` →
  `TransitionBacklogItemStatus(..., BacklogStatusArchived, ...)`,
  `session/backlog_lifecycle_archive.go:57-115`) only changes the parent
  `BacklogItem`'s `status` field. The child `ItemSession` row (which carries
  `session_role`, `session/ent/schema/item_session.go:25-26`) is untouched —
  it isn't deleted, hidden, or reassigned. Separately, `SessionArchiver.
  ArchiveSessionByUUID` (`session/backlog_lifecycle_archive.go:21-24`)
  soft-archives the **session/Instance** (so it stops showing in the live
  session list) — again, not the `ItemSession` DB row. So: **yes**, once this
  project ships, old sessions whose backlog item is archived (the common,
  intended case) will retroactively get a correct role in Insights, exactly as
  requirements.md's Success Metrics expect — no backfill migration is needed,
  a live query against `ItemSession` by session UUID (regardless of the parent
  item's status) is suffient.
- **The real gap is hard deletion, not archival.** `DeleteBacklogItem`
  (`session/ent_repository_backlog.go:1438-1487`) is a genuine hard delete:
  it explicitly queries and deletes all `ItemSession` rows for the item
  (lines 1462-1483, plus their `ReviewVerdict`s) *before* deleting the
  `BacklogItem` row itself — confirmed independently by the ent schema's
  `entsql.OnDelete(entsql.Cascade)` annotations on the `item_sessions` edge
  (`session/ent/schema/backlog_item.go:196-199`, commented "Cascade is
  deliberate, not an oversight"). This is reachable from user-facing code, not
  just test/debug paths: `server/services/backlog_service_lifecycle.go:548`
  calls it from what is presumably a "delete item" RPC handler (in addition to
  a debug-only caller at `server/services/backlog_debug_mutate_handler.go:286`).
  A session whose backlog item was hard-deleted (as opposed to archived) will
  permanently show no `session_role` in Insights after this ships — there is
  no data left to recover it from, and this is not a bug in the new feature,
  just a real limit on what "surviving archival" can mean once the underlying
  row is gone.
- **Recommendation**: state this plainly in the plan/PR description as an
  explicit, expected limitation — e.g. "session_role reflects live
  ItemSession data; a session's role becomes unrecoverable only if its
  backlog item was explicitly deleted (not archived), which cascades a hard
  delete of the ItemSession row" — rather than let a future reader discover it
  by noticing an unattributed old session and assuming the feature has a bug.
