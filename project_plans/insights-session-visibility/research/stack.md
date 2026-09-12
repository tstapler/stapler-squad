# Stack Research: insights-session-visibility

## 1. Charting / visualization

**Existing library: Recharts (already in use, already in the Insights page itself).**

- `web-app/package.json:104` — `"recharts": "^3.8.1"` (React 19-compatible major version, `react: ^19.0.0` at line 98).
- Already used by three sibling components in the exact same feature area:
  - `web-app/src/app/insights/ModelBreakdownChart.tsx` — `BarChart`/`Bar`/`Cell`/`Tooltip` from Recharts, with a hand-picked hex `PALETTE` array (lines 34-43) assigned by array index (`PALETTE[i % PALETTE.length]`, line 59) — **not validated against the `dataviz` skill's six checks**, and cycles past 8 series instead of folding to "Other."
  - `web-app/src/app/insights/DailySpendChart.tsx`
  - `web-app/src/app/insights/ModelOverTimeChart.tsx`
- No d3, visx, Chart.js, Victory, or Nivo anywhere in `web-app/src` (repo-wide grep came back empty for all of those).

**Recommendation:** reuse Recharts, not inline SVG. For item #1's two visuals:

- **Compact per-row visual (`SessionsTable.tsx`):** the `dataviz` skill's form table (`choosing-a-form.md:27`, "Part-to-whole" row) calls for a **stacked bar**, and for a table-row-sized visual specifically, a single thin horizontal `<div>`-based stacked bar (or a minimal inline Recharts `BarChart` with `stackId`) reading input/output/cache-creation/cache-read as 4 categorical segments. Given the row is table-cell-sized, a lightweight inline-SVG/CSS stacked bar (no Recharts overhead per row, matching the "thin marks, 4px rounded data-ends, 2px gap between fills" mark spec in `marks-and-anatomy.md`) is more appropriate than mounting a Recharts chart per virtualized row — Recharts's `ResponsiveContainer` + SVG mount cost per row would be wasteful inside `TableVirtuoso`'s per-row `itemContent` (`SessionsTable.tsx:429`).
- **Fuller breakdown (`SessionDetailContent.tsx`):** a full Recharts stacked `BarChart` (or a single wide stacked bar) is appropriate here — this matches the "part-to-whole" form and is consistent with the page's existing chart components.
- **4 fixed series (input/output/cache-creation/cache-read) is categorical identity, not magnitude** — per `color-formula.md`, this needs the categorical palette (4 of the fixed 8 hue slots, always in the same order across both visuals and consistent with whichever slot each token type already implicitly has, if any — none currently assigned; this is a new categorical mapping to define in planning) rather than a sequential ramp. Existing `ModelBreakdownChart.tsx`'s ad hoc `PALETTE` array should NOT be reused/copied for this — it isn't validated. **Run `node scripts/validate_palette.js "<4 hex colors>" --mode light` (and `--mode dark`) before shipping**, per the skill's non-negotiable rule 3. `references/palette.md` in the skill has a pre-validated default 8-hue palette to draw the first 4 slots from.
- Series count is only 4, which is comfortably under the skill's "4 = direct labels mandatory" threshold (`choosing-a-form.md`'s series-count ladder) — plan for direct-value labels or a legend, not color-alone.

## 2. Tooltip mechanism

**Existing component: a Radix-based `Tooltip` wrapper — reuse it, don't add `title=` for new fields.**

- `web-app/src/components/ui/Tooltip.tsx` wraps `@radix-ui/react-tooltip` (`web-app/package.json:77`, `"@radix-ui/react-tooltip": "^1.2.8"`). Signature: `Tooltip({ children, label, side? })`, 400ms `delayDuration` (`Tooltip.tsx:17`), rendered via `TooltipPrimitive.Portal`.
- Already used elsewhere: `web-app/src/components/sessions/CIStatusBadge.tsx`, `ConnectionCountIndicator.tsx`, `SessionRow.tsx`, `StatusBadge.tsx`, `GitHubBadge.tsx`, `ui/EstimatedValue.tsx`, `layout/ConnectionIndicator.tsx`, `backlog/BlockerChip.tsx` (grep of `web-app/src/components`).
- **However, `SessionsTable.tsx` itself does NOT use this component anywhere** — every existing hover explanation there is a plain HTML `title=` attribute:
  - `SessionsTable.tsx:284` — `title={s.sessionId || s.conversationId}`
  - `SessionsTable.tsx:298` — `title={\`${backlogEntry.sessionRole}: ${backlogEntry.itemTitle}\`}`
  - `SessionsTable.tsx:305` — `title={s.primaryModel}`
  - `SessionsTable.tsx:306` — `title={s.projectPath}`
  - `SessionsTable.tsx:309` — `title={\`${fmtTokens(s.cacheReadTokens)} read, ${fmtTokens(s.cacheCreationTokens)} written\`}` — the cache-hit-rate tooltip cited in the requirements doc.
  - The header row (`SessionsTable.tsx:264-278`, `sortableHeaderCell` at line 240) has **no tooltip mechanism of any kind** on any header, including "Waste Score" (`SessionsTable.tsx:276`).

**Recommendation:** for the **Waste Score column header** tooltip specifically (a static explanatory label, not a per-row dynamic value), use the existing `ui/Tooltip.tsx` Radix component wrapped around the header's `<span>` — it's the sanctioned, accessible (keyboard-focusable via Radix's trigger semantics, unlike native `title=` which is mouse-hover-only and fails WCAG for keyboard/touch users) pattern already established elsewhere in the codebase, and a column-header explanation is exactly the "static content explanation" case the other `Tooltip.tsx` call sites use it for. Continue using the plain `title=` attribute only for the existing per-row *dynamic value* tooltips (cache read/write counts, session ID, etc.) to match the surrounding code's established convention in this specific file — don't rewrite those in the same change unless the audit in step 4 finds them newly insufficient. If planning decides other missing tooltips (per the "audit other Insights fields" scope item) are also static/explanatory rather than dynamic-value, they should use `ui/Tooltip.tsx` too, for accessibility consistency with the rest of the app.

## 3. Tag filter UI

**Existing pattern: a single-select `<select>` dropdown, not a chip-based multi-select — same shape as `SessionsTable.tsx`'s own existing model filter.**

- The main session list's tag filter (`docs/reference/tag-organization.md:23`, "Tag Filter Dropdown: filter to sessions with a specific tag") is implemented as a plain single-value `<select>`:
  - `web-app/src/components/sessions/SessionList.tsx:998-1010` — `<select value={selectedTag} onChange={...}><option value="all">All Tags</option>{tags.map(t => <option key={t} value={t}>{t}</option>)}</select>`, state at `SessionList.tsx:253,280,396,410`.
  - This exactly mirrors `SessionsTable.tsx`'s own already-existing `modelFilter` `<select>` (`SessionsTable.tsx:388-398`, `modelSelect` CSS class) — a direct, in-file precedent for "add a filter dropdown to this table."
- Tag **editing** (not filtering) uses a distinct modal component, `web-app/src/components/sessions/TagEditor.tsx` — free-text input + add/remove chip list (`tagsList`/`tagItem` CSS, lines 97-111) — this is the "Tag Editor Modal" from the docs, used for mutating a session's tags, not for filtering a list.
- No dedicated `TagPicker`/`TagChip`/multi-select-tag-filter component exists anywhere in `web-app/src` today (grep for `TagFilter|TagPicker|TagChip` found none besides the two above).

**Recommendation / gap to flag for Phase 3 planning:** the requirements doc's Scope section (`requirements.md:99-100`) calls for "tag chips/multi-select filter UI," but the only existing tag-filter precedent in this codebase (`SessionList.tsx`) is a single-select dropdown, and `SessionsTable.tsx` already has an identical single-select pattern for `modelFilter` sitting right next to where a tag filter would go. Two honest options for planning to pick between:
  1. **Match `SessionsTable.tsx`'s own existing pattern** (single-select `<select>`, like `modelFilter`) — lowest effort, most internally consistent within this one file, but only allows filtering to one tag at a time (matches `SessionList.tsx`'s existing UX exactly, so no *new* interaction pattern enters the codebase).
  2. **Build actual multi-select tag chips** (as scoped) — no existing reusable component for this; would need genuinely new UI (a small chip-toggle row, not a `<select multiple>`, per usual UX for tag filtering), reusing `TagEditor.tsx`'s `tagItem` chip CSS for visual consistency but a different interaction (click-to-toggle-filter vs. add/remove-and-save).
  The `dataviz` skill's tags don't bear on this (it's not a chart), and the tag-organization doc doesn't mandate multi-select — so this is a scope/consistency call for Phase 3, not a technical constraint.
- Whichever UI is picked, extend the existing `fuse` search (`SessionsTable.tsx:99-106`, currently `keys: ["session.projectPath", "backlogTitle"]`) to also index a `tags` array once the field exists on `SessionTokenSummary`.

## 4. Proto / codegen workflow

- `proto/session/v1/insights.proto`'s `SessionTokenSummary` message (lines 69-96) currently ends at `activity_type = 20` (line 95); next free field number is **21**.
- `optional double waste_score = 19` (line 93) is the existing nil-safety precedent in this exact message — mirrors the plan's own reference to "richer enum matching proto conventions elsewhere" question. If `session_role` needs an absent/unknown state distinct from an empty string, the same `optional` pattern (or a `SESSION_ROLE_UNSPECIFIED = 0` enum, matching `ActivityType`'s and `Severity`'s existing enum-with-UNSPECIFIED-zero-value convention at lines 28-33 and 47-54) is available. A plain `repeated string tags = 21;` needs no `optional` wrapper (repeated fields are never "unset" in proto3 — empty list is the natural absent-state).
- **Workflow:** edit the `.proto` file, then run `make proto-gen` (`Makefile:535`). It's stamp-cached (`PROTO_STAMP`) and **auto-detects a changed `.proto` file via `find proto -name '*.proto' -newer $(PROTO_STAMP)`** (`Makefile:538`) — editing the file's mtime is enough to trigger a real regen on the next `make proto-gen`/`make build`/`make test` (all of which depend on it, e.g. `Makefile:574`'s `test: ensure-tools proto-gen ...`). Regen runs `buf generate proto` (`Makefile:543`), emitting both `gen/proto/go/session/v1/insights.pb.go` (Go) and `web-app/src/gen/session/v1/insights_pb.ts` (TypeScript) in one pass — no separate frontend/backend codegen steps.
- **Gotchas:**
  - `gen/proto/go/` and `web-app/src/gen/` are gitignored — don't hand-edit or commit generated output (matches the project CLAUDE.md's broader "don't commit generated ent/proto output" convention).
  - `buf lint proto` (`make proto-lint`, `Makefile:563`) and `buf build proto` (`make proto-build`, `Makefile:566`) are available for a pre-commit sanity check without a full regen.
  - This is unrelated to the `ent-gen` Makefile target (`Makefile:552-561`) — that's for `session/ent/schema/*.go` (the backlog/ItemSession DB schema), a completely separate generator with its own stamp file (`ENT_STAMP`) and its own mandatory `--feature sql/upsert` flag (per this repo's CLAUDE.md). Adding `session_role`/`tags` to the *proto* message does not touch ent-gen at all — only reading an *existing* ent-modeled field (`ItemSession.session_role`) to populate it does, and that requires no schema change since the field already exists (see §5).
  - Requirements doc's own constraint (`requirements.md:75-77`) — re-verified in this research: `SessionTokenSummary` has exactly one Go consumer (`server/services/insights_service.go`, `buildSessionSummary` at line 82) and the Insights-scoped TS consumers listed in the requirements doc. Repo-wide grep found no other production Go/TS reference to the message. Appending fields 21+ is additive/backward-compatible as planned.

## 5. Backend lookup: persisted session role surviving backlog-item archival

**Key finding: the infrastructure for this already exists and already survives archival — it just isn't wired into `SessionTokenSummary`.**

- `session.SessionRoleWork/Triage/Review/JulesWork` (`session/backlog.go:50-53`) are plain string constants stored on the ent `ItemSession.session_role` field (`session/ent/schema/item_session.go:25-26`, comment: `"One of: work, triage, review"` — note the schema comment doesn't mention `jules_work`, likely stale, not a blocker).
- `ItemSession` has an **existing index on `session_uuid`** (`item_session.go:107`, comment: `"CRITICAL: O(1) lookup on every EventExited hook"`) — an indexed lookup path already exists structurally.
- **Archival does not delete `ItemSession` or `BacklogItem` rows.** `BacklogStatusArchived` (`session/backlog.go:25`, aliasing `domain.BacklogStatusArchived`) is just one value of the `BacklogStatus` enum on the still-present `BacklogItem` row; only an explicit, separate `DeleteBacklogItem` RPC (`server/services/backlog_service_lifecycle.go:540` → `session/ent_repository_backlog.go:1438` → `session/storage.go:927`) removes rows. Archival ≠ deletion — this directly resolves the requirements doc's open question ("whether `ItemSession` rows survive archival") with a definitive **yes**.
- **A full, unfiltered query already exists and already feeds Insights-adjacent code:** `EntRepository.GetAllItemSessionsWithBacklogInfo` (`session/ent_repository_backlog.go:2782-2804`) does `r.client.ItemSession.Query().WithBacklogItem().All(ctx)` — **no status filter of any kind** — annotated `//nolint:entfullscan feeds the Insights dashboard, which needs the full join across all item sessions.` It's exposed via `Storage.GetAllItemSessionsWithBacklogInfo` (`session/storage.go:1394`) and the `BacklogService.GetSessionBacklogIndex` RPC (`server/services/backlog_service_query.go:533-560`), which is exactly what the frontend's `useBacklogSessionIndex()` hook (`web-app/src/lib/hooks/useBacklogService.ts:1339-1370`) calls, and exactly what `InsightsDashboard.tsx:99` (`const { index: backlogIndex } = useBacklogSessionIndex();`) feeds into `SessionsTable.tsx`'s `backlogIndex` prop today.
  - **This means the requirements doc's problem-statement claim** ("`backlogIndex`, a map built only from *currently open* backlog items" — `requirements.md:24-25`) **appears to be inaccurate as currently written**, or refers to a stale/different code path: the actual backing query has no open/closed filter and would already return a role for an archived item's sessions, as long as the `ItemSession` row exists (it does, per above) and its `Edges.BacklogItem` is non-nil (only nil if the *parent item itself* was hard-deleted via `DeleteBacklogItem`, not merely archived — see `itemSessionToSummary`-adjacent skip at `ent_repository_backlog.go:2792-2794`). **Flag this for Phase 3 planning to re-verify against the actual live behavior** (e.g. manually archive an item and check whether its session's role still renders in Insights today) before assuming the client-side join is the root cause — the real gap may be narrower than described (e.g., only sessions never linked to any `ItemSession` row, or items that were actually deleted, not archived).
  - Regardless of whether the *current* live join is already correct, the requirements doc's **constraint** (`requirements.md:81-83`) still mandates sourcing role from the persisted field at **summary-build time** on the backend, not via a live frontend join — so the proto/backend work should proceed either way; the practical implementation question is just where to source the map from.
- **Recommended mechanism:** rather than adding a new ent index or query, **reuse `GetAllItemSessionsWithBacklogInfo`'s existing unfiltered scan** (or a same-shaped narrower helper returning just `map[sessionUUID]role`) inside `InsightsService`, built once per request (or cached similarly to how `associator.Snapshot()` is already built once per request at `insights_service.go`'s call sites) and passed into `buildSessionSummary()` alongside the existing `snapshot []tokens.SessionRecord` parameter. This avoids any ent schema change (the field and index already exist) and mirrors the existing `//nolint:entfullscan` precedent rather than introducing a new one.
- **`tokens.SessionRecord`/`SessionStorage` (`session/tokens/association.go:11-23`) is the wrong layer for `session_role`:** `SessionRecord` is populated by `Storage.ListSessionRecords()` (`session/storage.go:516-535`) from `ListInstanceData()` — i.e., from live `Instance`/session records, not from `ItemSession`/backlog rows. Role lives on the backlog side (`ItemSession`), a structurally separate table/domain from `Instance`. Joining role into `SessionRecord` would require `ListSessionRecords()` to also join against `ItemSession` by `SessionUUID`, duplicating what `GetAllItemSessionsWithBacklogInfo` already does — better to keep `SessionRecord` scoped to session/association concerns and fetch role via a separate lookup path passed alongside it, as recommended above.
- **`Tags`, by contrast, IS the right fit for `SessionRecord`:** `Instance.Tags` lives on the same `InstanceData` DTO `ListSessionRecords()` already iterates — `InstanceData.Tags []string` (`session/storage.go:48`) is populated via `Instance.GetTags()`/`SetTags()` (`session/instance_tags.go:53-68`). Adding a `Tags []string` field to `tokens.SessionRecord` (`session/tokens/association.go:11-16`) and populating it at `session/storage.go:527` (alongside the existing `SessionID`/`ConversationID`/`Path`/`CreatedAt` fields) is a one-line-per-site change with no new query, no ent involvement, and no cross-domain join — unlike role, which lives in a different table entirely.

## 6. Existing dependency versions (`web-app/package.json`)

| Package | Version | Line |
|---|---|---|
| `react` | `^19.0.0` | 98 |
| `react-dom` | `^19.0.0` | 100 |
| `@types/react` | `^19` | 144 |
| `fuse.js` | `^7.3.0` | 94 |
| `recharts` | `^3.8.1` | 104 |
| `@radix-ui/react-tooltip` | `^1.2.8` | 77 |
| `@radix-ui/react-dialog` | `^1.1.15` | 74 |
| `@radix-ui/react-accordion` | `^1.2.17` | 73 |
| `@radix-ui/react-tabs` | `^1.1.13` | 76 |
| `@radix-ui/react-slot` | `^1.2.4` | 75 |
| `react-virtuoso` | `^4.18.7` | 103 (used by `SessionsTable.tsx`'s `TableVirtuoso` for >50 rows) |
| `lucide-react` | `^1.14.0` | 96 |
| `typescript` | `^5.9.3` | 166 |

No new dependency is needed for any of the four bundled changes: Recharts covers charting, `ui/Tooltip.tsx` (Radix) covers tooltips, and the tag filter UI (whichever form Phase 3 picks) can be built from plain `<select>`/CSS or `TagEditor.tsx`'s existing chip styling — all already-installed packages.
