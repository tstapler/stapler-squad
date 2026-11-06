# Research: Feature Landscape — insights-session-visibility

## 1. Tooltip audit — every column/metric currently rendered

Sources read in full: `web-app/src/app/insights/SessionsTable.tsx`,
`SessionDetailContent.tsx`, `SessionDetailDrawer.tsx`, `FindingsPanel.tsx`,
`InsightsDashboard.tsx`.

### `SessionsTable.tsx` columns (`headerContent`, `SessionsTable.tsx:264-278`; cells `renderCells`, `SessionsTable.tsx:280-336`)

| Column | Has explanation? | What's missing |
|---|---|---|
| Session (id + orphan/backlog badge) | Partial — `title={s.sessionId \|\| s.conversationId}` on the id (SessionsTable.tsx:284); backlog badge has `title={`${role}: ${itemTitle}`}` (SessionsTable.tsx:298) | "orphan" badge itself has no tooltip explaining what orphan means (no matching stapler-squad session) — minor gap, not in the 4-item scope but worth a one-line title if touched |
| Model | No tooltip, but self-explanatory (raw model name) | None needed |
| Path | No tooltip; `title={s.projectPath}` shows full path on hover (SessionsTable.tsx:306) | Already has one — full path — no gap |
| Input | No tooltip | Self-explanatory raw token count; no gap |
| Output | No tooltip | Self-explanatory; no gap |
| Cache (cacheHitRate %) | **Yes** — `title={`${read} read, ${written} written`}` (SessionsTable.tsx:309) | Already explained |
| Cost | No tooltip; "unpriced" badge has no title of its own (badge text is self-explanatory but doesn't say *why*, e.g. which model) | Minor — `unpricedModels` list (proto field 17) is available but not surfaced in a tooltip anywhere in the table |
| Duration | No tooltip | Self-explanatory (m/s format); no gap |
| Cost/Msg | No tooltip; "Not evaluated" text shown when `messageCount === 0` (SessionsTable.tsx:318) | Self-explanatory given the label; no gap |
| Cache ROI | No tooltip | **Gap** — sign convention (`+`/`-`) and what "ROI" means here (dollar savings from cache vs. a no-cache baseline) is not explained anywhere in this component. `unpricedBadge` shown instead when unpriced, same as Cost. |
| **Waste Score** | **No tooltip** (confirmed, SessionsTable.tsx:276 header, cells at 327-333) | **Confirmed primary gap from requirements.md** — needs: (1) it's a weighted 0-100 blend of cache-shortfall/ceiling/context penalties, not a dollar figure ADR-002 forbids mixing it with `$`; (2) higher = worse; (3) blank ("Not evaluated") means `ComputeWasteScore` returned nil because the session had fewer than `minTurnsForCacheFloor` turns (`session/tokens/findings.go:274`) — too sparse to score, not zero waste; (4) "—" (different from "Not evaluated") appears instead for unpriced-model sessions (SessionsTable.tsx:328-329) — a *third* rendering state worth naming in the tooltip too. |

### `SessionDetailContent.tsx` fields (`SessionDetailContent.tsx:49-207`)

| Field | Has explanation? | What's missing |
|---|---|---|
| Model, Project, Message count, First/Last message, Session ID, Conversation ID | No tooltips | Self-explanatory; no gap |
| Total cost | No tooltip | Same "not measured, modeled" caveat other estimated $ figures get via `EstimatedValue` elsewhere in this file (tool cost) is **not** applied here — plain `fmtCost()` (SessionDetailContent.tsx:61). Minor inconsistency, arguably out of the bundled scope (requirements only calls out Waste Score + audit-for-missing, not a `EstimatedValue`-everywhere pass), but flagging since planning may want the header-tooltip treatment on Cost too. |
| Cache hit rate | No tooltip | Same metric as `SessionsTable`'s Cache column, which *does* have raw-number tooltip — this location doesn't. **Genuine gap for consistency** if the token-type breakdown viz (item #1) doesn't already surface these numbers here. |
| Cache writes | No tooltip | Raw number only; minor. |
| Backlog Item → Role | No tooltip | Role string (`work`/`triage`/`review`/`jules_work` or ad hoc values, see §2) shown raw with no legend — a first-time reader can't tell what "jules_work" means. Worth a tooltip once `session_role` becomes a permanent, always-populated field (item #3). |
| Per-Turn Breakdown → Cache column | **Yes** — same read/written tooltip pattern (SessionDetailContent.tsx:139-143) | Already explained |
| Tools Breakdown → Cost | **Partial** — `EstimatedValue` with a specific caveat when `costMayDoubleCount` (SessionDetailContent.tsx:175-184) | Already explained for the double-count case; plain-cost case has no tooltip but is unambiguous |
| Skill Activations | No tooltip | Self-explanatory (list of skill names); no gap |

### `FindingsPanel.tsx`

Already has the most tooltip coverage in the whole surface: `EstimatedValue` with `dollarImpactTooltip` (FindingsPanel.tsx:134-135, 156-158) explains dollar-impact is modeled/non-summable per ADR-002. Severity badges use `Badge` component with a color+text label (never color-only) — no additional explanation needed. No gaps found here.

**Bottom line for planning**: only **Waste Score** (header) is a hard requirement per requirements.md; **Cache ROI** (SessionsTable) is the next most credible "genuinely missing" candidate the audit turned up, since it has no explanation anywhere on the page and its sign convention isn't obvious. Everything else is either already explained or self-explanatory raw data.

## 2. Session role / tag domain model

### Tag system (main session list) — reusability for Insights

`docs/reference/tag-organization.md` (read in full): tags are a flat `Tags []string` on `session.Instance` (`session/instance_tags.go`), thread-safe via `TagManager`, persisted in "JSON persistence and Protobuf schema." Confirmed independently: `session/ent/schema/session.go:181` — `edge.To("tags", Tag.Type)` — tags are a **first-class ent edge on the `Session` entity**, not derived from or dependent on `BacklogItem`/`ItemSession` rows. This matters for item #3 below.

- 8 grouping strategies exist (Category/Tag/Branch/Path/Program/Status/Session Type/None) — grouping is not directly reusable for Insights (a flat table, not grouped cards), but the **Tag Filter Dropdown** UI concept is the relevant piece.
- **Multi-tag filter semantics — no precedent exists.** The actual filter implementation (`web-app/src/lib/hooks/useFilteredGroupedSessions.ts:93-98`) is **single-select**: `selectedTag: string` ("all" or exactly one tag), checked via `session.tags.includes(selectedTag)`. There is no AND/OR multi-tag filter anywhere in this codebase today to copy for Insights. **This is a genuine open design decision for Phase 3 planning, not something with an established codebase answer** — the requirements doc's phrasing ("using the same tag data") is satisfied by reusing `Instance.Tags` as the data source, but the *filter UX* (single-select dropdown vs. multi-select chips with AND/OR) has no existing analog to mirror. Recommend planning pick OR (any-selected-tag matches) as the more intuitive default for a table filter with multi-select chips, and state that explicitly rather than silently inventing AND semantics.

### Session role — full enumeration

`session/backlog.go:49-54` defines exactly 4 canonical role constants:
```go
SessionRoleWork      = "work"
SessionRoleTriage    = "triage"
SessionRoleReview    = "review"
SessionRoleJulesWork = "jules_work"
```
Verified via repo-wide grep (`grep -rn "SessionRole" --include='*.go' .`, ~90 call sites) — no 5th canonical role exists. **One ad hoc non-constant string was found**: `server/services/backlog_service_triage.go:3619` sets `SessionRole: "re-review-triggered"` — a literal string, not one of the 4 constants, stored directly into an `ItemSession.SessionRole` field. Planning should decide whether `session_role` on the proto should be a free-form string (safe, matches what's actually stored) or a richer enum (would need a 5th "other/unknown" bucket for this literal and any future ad hoc value) — enum would need to not silently drop `"re-review-triggered"`.

`IsTmuxBackedSessionRole` (`session/backlog.go:76-78`) documents an important asymmetry: only `work` and `review` roles run as live tmux-backed `Instance` objects. **`triage` sessions are headless one-shot subprocess calls that are never tracked as an `Instance` at all** (per that function's doc comment) — meaning a `triage`-role session has no `Instance.Tags` to draw on even if planning wanted to lean on tags instead of/alongside the `ItemSession.Role` field for role persistence. `jules_work` sessions run on Google's infrastructure, also not local `Instance`s.

`server/services/backlog_service_query.go:531-559` (`GetSessionBacklogIndex` RPC, backing `useBacklogSessionIndex()` in `web-app/src/lib/hooks/useBacklogService.ts:1339-1370`) calls `storage.GetAllItemSessionsWithBacklogInfo(ctx)`, implemented in `session/ent_repository_backlog.go:2782-2804`:
```go
sessions, err := r.client.ItemSession.Query().WithBacklogItem().All(ctx)
...
if is.Edges.BacklogItem == nil { continue }
```
This is an **unfiltered full scan** — it does not exclude archived items by status. So the requirements.md claim that today's role label "silently disappears" once an item **archives** is not fully precise as written: archival alone (a status flag) does not remove the `ItemSession` row or drop it from this index. What *does* remove it is the `is.Edges.BacklogItem == nil` skip, which fires when the parent `BacklogItem` row itself is gone — see §3.

## 3. Edge cases and failure modes

### a. Sessions with no tags
`SessionsTable.tsx`'s `fuseDocs`/filter logic has no tag-aware code yet (item #2 is unbuilt). When added: a session with `Tags: []` (or field omitted, defaulting to empty repeated string in proto) must render with no chips and must not be excluded when "All tags" is selected. Existing precedent from `useFilteredGroupedSessions.ts:94-98` guards `!session.tags` before calling `.includes()` — follow that null-guard pattern; `SessionTokenSummary.tags` should default to `[]` rather than allow undefined on the wire.

### b. Sessions with no `session_role` (plain work sessions never linked to a backlog item, orphans)
Confirmed via `SessionsTable.tsx:281,293` and `SessionDetailContent.tsx:86` — today, `backlogEntry` (and thus any role label) is simply `undefined` for such sessions, and the whole "Backlog Item" section is conditionally omitted (`{backlogEntry && (...)}`, `SessionDetailContent.tsx:86`). For the new persisted `session_role` field, the equivalent state is an empty string — render nothing / no role badge, not a "work" default (a plain ad hoc `claude` session run outside the backlog pipeline was never assigned any of the 4 roles at all — defaulting it to "work" would be a fabricated fact).

### c. A session whose backlog item was **deleted** (not archived)
Traced `DeleteBacklogItem` (`session/ent_repository_backlog.go:1438-1503`): it is a **hard, cascading delete** — the code explicitly queries and deletes all `ReviewVerdict` rows and then all `ItemSession` rows belonging to the item (lines 1462-1483) before deleting the `BacklogItem` row itself. The doc comment states plainly: "DeleteBacklogItem permanently removes an item and all its child records."

**This is the critical finding for item #3's design.** A persisted `session_role` sourced "at summary-build time... surviving backlog item closure/archival" (requirements.md, Constraints) works fine for **archival** (item row persists, just flagged `archived` — the existing unfiltered `GetAllItemSessionsWithBacklogInfo` query already proves this scan pattern works across all statuses). It does **not** work for **deletion** — once `DeleteBacklogItem` runs, the `ItemSession` row carrying `Role` is gone forever, and no query against `ItemSession`/`BacklogItem` (open, closed, or archived) can recover it. If planning's "indexed lookup by `SessionUUID` across all backlog items" (requirements.md Open Questions) is implemented purely against `ItemSession`, a hard-deleted item's sessions will have their role silently disappear from Insights exactly as before — this doesn't fully satisfy the stated goal of preserving triage/review efficiency history indefinitely, only "until someone deletes the item," which does happen (`server/services/backlog_service_lifecycle.go:540` exposes a `DeleteBacklogItem` RPC actually wired to a UI action, and `server/services/backlog_debug_seed_handler.go:286` uses it too).

The one architectural alternative that would survive full deletion: stamp the role onto the session's own persisted `Instance.Tags` at spawn time (`session/ent/schema/session.go:181`'s `tags` edge lives on `Session`, not `BacklogItem`/`ItemSession`, so it isn't touched by `DeleteBacklogItem`'s cascade). Constants already exist for this (`session/backlog.go:95-98`: `TagBacklogWork`, `TagBacklogRevision`, `TagBacklogReview`, `TagAutonomous`) and `TagBacklogWork` is already stamped on spawn (`server/services/backlog_service_triage.go:1061`) — but `TagBacklogReview` is defined and never actually applied anywhere in the non-test codebase (confirmed by grep), and **no tag exists for `triage`/`jules_work` at all**, and triage sessions have no `Instance` to tag in the first place (§2). So neither the `ItemSession.Role` path nor the existing tag constants alone fully cover "role survives even a hard delete, for all 4 roles" — flagging this as a real gap for Phase 3 planning to resolve explicitly (the requirements' own Constraints text sources role from "the session's own persisted `Role` field," i.e. `ItemSession.Role`, which is exactly what deletion destroys).

### d. Multiple tags per session — filter should be AND or OR?
No existing precedent in this codebase (see §2) — this is a fresh design decision, not a "match existing behavior" one. Flagging explicitly rather than guessing.

### e. Waste score nil/missing state — current rendering
Three distinct states exist today in `SessionsTable.tsx:327-333`, each with different text and none exposed via a shared "estimated/heuristic" visual marker (`EstimatedValue`, used elsewhere for cache-ROI-adjacent and per-tool-cost figures, is **not** used for Waste Score anywhere):
1. `unpricedModels.length > 0` → renders `"—"` (an em dash, not a badge, unlike the Cost/Cache-ROI columns which show an `unpricedBadge` chip in this same situation)
2. `wasteScore === undefined` (session too sparse, `< minTurnsForCacheFloor` turns per `session/tokens/findings.go:274`) → renders the literal text `"Not evaluated"`
3. Otherwise → renders the raw number, no `~` prefix, no unit, no color coding

The header tooltip (requirement #4) must describe all three, since a user hovering the header has no way to know which of "—" vs. "Not evaluated" they'll see for a given row without also being told both exist and what distinguishes them (unpriced model vs. too few turns).

### f. Very small/large token-type visual breakdown
No existing token-type visual exists yet to check for degradation (confirmed — grep of `SessionsTable.tsx`/`SessionDetailContent.tsx` shows only text/percentage rendering, no bar/donut component). Two known-safe patterns already in this codebase to reuse for the new viz's zero-division guard:
- `computeCacheHitRate` (`insightsFormatters.ts:25-28`): explicit `denom === 0 ? 0 : ...` guard — the precedent to follow for any stacked-bar width computed as `tokenType / totalTokens`.
- A session with `total_input+output+cache_creation+cache_read == 0` (possible for a just-started or malformed conversation) must render an empty/neutral bar state, not `NaN%` widths — no existing component does this yet, so it's a new requirement for whichever chart component is built, not a "match existing behavior" item.
- A session dominated by one token type (e.g. 99.9% cache-read) is not a code hazard (percentages still sum correctly), only a design question of whether a segment below some minimum width should get a "minimum visible sliver" treatment — a `dataviz`-skill-level UI decision, not backend.

## 4. Unstated user needs (judging triage/review efficiency)

The user's stated underlying goal is "efficiency of triage/review sessions." A persistent role **badge** per row (item #3) is necessary but not sufficient for that judgment at a glance. Two adjacent, non-scope-creep observations for planning to weigh:

1. **A role *filter*, not just a role *badge*, is a natural extension of the tag-filter UI being built in the same PR.** `SessionsTable.tsx` already has a `modelFilter` dropdown (`SessionsTable.tsx:82,388-398`) as a precedent for a single-select non-tag filter control — a `roleFilter` (All / work / triage / review / jules_work) could reuse the exact same UI pattern with near-zero net-new component surface, and it directly serves "let me see only triage/review sessions" rather than requiring the user to scan/search visually for role badges across the full table. This is a small, low-risk addition sitting right next to the work already planned, but it's a distinct filter, not literally required by the four bundled items — flagging as in-scope-adjacent for Phase 3 to explicitly accept or defer, not silently added here.
2. **A per-role subtotal/aggregate** (e.g. "triage sessions: N sessions, $X total, avg waste score Y") is a bigger lift — it would need a new aggregation path (existing `SummaryCards`/`ActivityBreakdownTable` group by `ActivityType`, not by `session_role`) and risks exactly the non-summable-waste-score problem ADR-002 was written to prevent if "avg waste score" is computed carelessly (averaging is fine; summing dollar figures across sessions is the forbidden operation ADR-002 targets — averaging `WasteScore` was not addressed by that ADR one way or the other, so planning should decide explicitly rather than assume it's covered). This is a heavier feature than "add a filter" and is reasonable to flag as future/out-of-scope rather than sneak into this bundle.

## Key file:line references

- `web-app/src/app/insights/SessionsTable.tsx:264-336` — full column set + cell rendering
- `web-app/src/app/insights/SessionDetailContent.tsx:49-207` — detail sections
- `web-app/src/app/insights/insightsFormatters.ts:20-28` — fmtPct / computeCacheHitRate zero-guard precedent
- `web-app/src/components/ui/EstimatedValue.tsx` — shared "modeled, not measured" tooltip treatment
- `docs/reference/tag-organization.md` — tag system overview
- `web-app/src/lib/hooks/useFilteredGroupedSessions.ts:93-98` — single-select tag filter (no AND/OR precedent)
- `session/backlog.go:49-54,56-78,93-99` — the 4 SessionRole constants, IsTmuxBackedSessionRole, tag constants
- `session/backlog_service_triage.go:3619` (repo path `server/services/backlog_service_triage.go:3619`) — ad hoc `"re-review-triggered"` non-constant role value
- `server/services/backlog_service_query.go:531-559` — `GetSessionBacklogIndex` RPC (today's client-side-join data source)
- `session/ent_repository_backlog.go:2782-2804` — `GetAllItemSessionsWithBacklogInfo` (unfiltered by status)
- `session/ent_repository_backlog.go:1438-1503` — `DeleteBacklogItem` hard-cascades `ItemSession`/`ReviewVerdict` deletion
- `session/ent/schema/session.go:181` — `tags` edge lives on `Session`, independent of `BacklogItem`
- `session/instance_tags.go` — `Instance.Tags` accessor methods
- `project_plans/insights-cost-intelligence/decisions/ADR-002-findings-non-summable-dollar-impact.md` — exact wording basis for the Waste Score tooltip (non-summable, never `$`-labeled, procedural not compile-time guarantee)
- `session/tokens/findings.go:273-298` — `ComputeWasteScore`, nil when `len(r.TurnTimeline) < minTurnsForCacheFloor`
- `proto/session/v1/insights.proto:68-96` — current `SessionTokenSummary` message, next free field number is 21
