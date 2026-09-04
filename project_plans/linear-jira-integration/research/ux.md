# UX Research: Linear/JIRA Issue Source Badges & Filtering

Agent 5 (UX), SDD research phase for `linear-jira-integration`. Scope: dashboard
requirement in `requirements.md` goal 5 / AC 7 — "Issue source badge on session
cards, filter sessions by issue source, issue URL stored as session metadata
and clickable from session card."

## 1. Comparable UX patterns today

There are **two independent, non-overlapping badge systems** for GitHub in
this codebase today. This distinction matters because the requirement's
"session card" badge doesn't map cleanly onto either existing one as-is.

### 1a. Backlog item provenance badge (which tracker issue *spawned* this item)

- `web-app/src/components/backlog/BacklogItemCard.tsx:197-210` — small pill
  badge in the card footer, rendered only `{item.externalUrl && item.externalId && (...)}`.
  Uses `CircleDot` from lucide-react (not a real GitHub glyph — lucide 1.14,
  the pinned version, ships no brand icons at all; see the comment at
  `BacklogItemCard.tsx:5-8` and the identical one in `SourceSection.tsx:5-8`).
  `aria-label={"Imported from GitHub issue #" + item.externalId}` — hardcoded
  "GitHub" text, not derived from any source-type field.
- `web-app/src/components/backlog/detail/SourceSection.tsx` — same pattern,
  larger, in the item detail view's collapsible "Source" section, plus a
  `labels` chip row (`styles.labelBadge`).
- Both are driven purely by `externalUrl`/`externalId` strings on
  `BacklogItem` — there is **no existing `source`/`pluginId` field surfaced to
  the frontend on the item type today**. Confirmed via
  `grep -n "sourcePlugin\|pluginId" web-app/src/lib/hooks/useBacklogService.ts`
  → no match. Adding Linear/JIRA badges here requires adding that
  discriminator field to the `BacklogItem` proto/type first — it doesn't
  exist to switch on yet.

### 1b. Session card GitHub-PR badge (the outgoing PR a session produced)

- `web-app/src/components/shared/GitHubBadge.tsx`, used from
  `SessionCard.tsx:470-483`. Driven by dedicated per-field proto data:
  `session.githubPrNumber`, `githubPrUrl`, `githubOwner`, `githubRepo`,
  `githubSourceRef`, plus PR-status fields (`githubPrPriority`, `githubPrState`,
  etc.) — see `proto/session/v1/types.proto:80-113`, all explicitly comment-
  labeled "GitHub integration fields for PR/URL-based session creation."
- This badge draws a **real hand-authored inline SVG GitHub octocat path**
  (`GitHubBadge.tsx:162-170`), not a lucide substitute — a higher-fidelity
  bar than the backlog-card badge's `CircleDot` stand-in. Two fidelity tiers
  already coexist in this codebase; a new Linear/JIRA badge can pick either
  tier precedent depending on where it's rendered.
- **Critical finding: this badge has nothing to do with backlog-item
  provenance.** It represents a PR the session *produced*, populated by
  `PRStatusPoller`, not the issue the session was triaged *from*. Today there
  is no code path that copies a backlog item's `externalUrl`/`externalId`
  onto the `Session` that gets created from it — confirmed by grepping
  `session/pipeline_engine.go` and `session/backlog_lifecycle.go` for
  `SessionType|ExternalURL|externalUrl` (zero matches) and by the proto: no
  `backlog_item_id` field exists on the `Session` message at all (only on the
  internal Go `session/repository.go:288` struct, via
  `session/storage_backlog.go`/`ent_repository_backlog.go`, never exposed
  through `proto/session/v1/*.proto` to the frontend).

**Implication for the plan**: "Issue source badge on session cards" as
literally stated requires new plumbing, not just a new badge component —
either (a) expose `backlog_item_id` on the `Session` proto and have the
frontend join against the backlog item's source, or (b) denormalize
`source_type`/`external_url`/`external_id` directly onto `Session` at
creation time (mirroring how `github_owner`/`github_repo` are denormalized
today, i.e. same shape, generalized past GitHub). Recommend (b) — it matches
existing precedent (`GitHubBadge`'s fields are themselves denormalized onto
`Session`) and avoids a frontend join. This is a backend/plan-phase decision,
noted here because it changes the badge component's props shape.

## 2. Source filter dropdown pattern

No filter-by-source exists anywhere yet — not on the backlog board
(`BacklogBoard.tsx`/`board/page.tsx` have zero `filter`/`Filter` UI, confirmed
by grep) and not on the session list beyond status/category/tag.

The requirement says "filter **sessions** by issue source" — that maps to
`web-app/src/components/sessions/SessionList.tsx`, which already has the
exact structural pattern to clone: three sibling native `<select>`s
(`SessionList.tsx:1035-1080`) — Status, Category, Tag — each:
- `value={selected...}` / `onChange` to local state
- `className={select}` (shared vanilla-extract class from `SessionList.css.ts`)
- `aria-label="Filter by <dimension>"`
- `<option value="all">All <Dimension>s</option>` sentinel + mapped options

A "Filter by issue source" select should be a fourth sibling in that same
`filterControls` group, same `select` class, `aria-label="Filter by issue source"`,
options `All Sources` / `GitHub` / `Linear` / `JIRA` (only sources that exist
in data, matching how the Tag filter derives its option list from live data
rather than a static enum — see `tags.map(...)` at `SessionList.tsx:1075`).
This depends on the same session-level source field from §1b existing.

## 3. Accessibility

Every existing badge in this family already follows the same rule and the
new ones must match it exactly:
- `aria-hidden="true"` on the icon SVG/glyph itself (`GitHubBadge.tsx:133`,
  `165`; `SourceSection.tsx:43`; `BacklogItemCard.tsx:207`)
- A separate `aria-label` on the containing interactive element with the
  full human-readable meaning, not just the icon
  (`aria-label={"View GitHub Pull Request #" + prNumber + ...}`,
  `aria-label={"Imported from GitHub issue #" + item.externalId}`)
- `title` attribute duplicates the label as a mouse tooltip (belt-and-suspenders,
  not a substitute for aria-label)

No icon-only-with-no-text-alternative instances exist in this family — the
bar to match is: icon `aria-hidden`, wrapping element gets `aria-label` +
`title` with the source name spelled out ("Linear issue ENG-123" /
"JIRA issue PROJ-456"), not just the ticket ID alone (a bare "#123" reads
ambiguously across three possible trackers once JIRA/Linear exist — GitHub's
badge could get away with a bare "#123" because it was the only tracker).

The filter `<select>` needs only `aria-label="Filter by issue source"` —
consistent with the three existing filter selects, none of which have a
separate visually-hidden `<label>` element; `aria-label` alone is the
established (and sufficient, per WCAG 4.1.2) convention here.

## 4. Error/edge-case UX — source health

**Good news: this already exists and generalizes with minimal change.**
`web-app/src/components/settings/BacklogSourcesSettings.tsx` has a working
per-source health indicator:
- `isAuthFailure(errorMessage)` (`BacklogSourcesSettings.tsx:34-45`) pattern-
  matches the most recent sync error string for auth-type failures (401/403/
  "bad credentials"/"revoked"/"requires authentication"), explicitly
  excluding rate-limit 403s so a transient rate limit doesn't false-positive
  as "credentials broken."
- When true, renders a persistent row-level `role="alert"` banner: "⚠ Sync
  failing — check credentials" (`BacklogSourcesSettings.tsx:320-328`), visible
  without expanding history (Story 4.3.2 in the existing codebase).
- This logic is **already source-agnostic in shape** — it operates on
  `historyBySource[source.id]?.events?.[0]?.errorMessage` regardless of
  `source.pluginId`. Extending to Linear/JIRA is close to free *if* Linear's
  and JIRA's error strings are surfaced through the same sync-history
  mechanism (`GetSyncHistory` RPC) with parseable text — the plan should
  require Linear/JIRA plugin errors to include a recognizable 401/403-shaped
  substring, or `isAuthFailure` needs tracker-specific patterns added (Linear
  GraphQL and JIRA REST typically return distinguishable auth error bodies —
  worth checking each API's actual error shape in the backend research doc).

**What is NOT generic and needs updating**: hardcoded "GitHub" copy in that
same component — `<span>Sync with GitHub</span>` (line 352), "Close GitHub
issues when I finish here" (365), "Reflect GitHub status back here" (392),
"reflecting GitHub status back" aria-label (390). These need to become
`Sync with {source.displayName or schema.label}`-driven strings once
Linear/JIRA rows exist in the same list, or a reviewer will ship a Linear
source row that still says "Sync with GitHub" next to it.

`PLUGIN_SCHEMAS` in `backlogSourceSchemas.ts` is already explicitly
documented as the extension point for this ("Adding a source type for a new
plugin (e.g. Jira, Linear) means adding one entry here" — comment at
`backlogSourceSchemas.ts:20-26`) — the Add-a-Source form is schema-driven and
needs no per-plugin component changes, just new `PluginSchema` entries with
Linear's/JIRA's field sets (Linear: API key only, workspace resolved via the
key; JIRA: `JIRA_BASE_URL`/`JIRA_EMAIL`/`JIRA_API_TOKEN` per the requirements
doc's credential list).

## 5. Job-to-be-done note

Functionally this saves manual copy-paste of ticket context into a session
prompt. The existing GitHub path already demonstrates the mechanism users
expect: `import_github_issue` MCP tool → backlog item with `externalUrl`/
`externalId`/labels populated → later triage creates a session via
`CreateDirectorySession` (source-agnostic, see §6) → today the session itself
loses the link back to that issue (§1b gap). Closing that gap (denormalizing
source info onto the session) is what actually delivers the emotional/
context-switching win the requirement is after — without it, a user still has
to click through to the backlog item to find the original ticket link, which
defeats "clickable from session card."

## 6. Session-creation-registry.md applicability

**Verdict: this is purely a backlog-item-source addition. It does NOT touch
the 7-touchpoint session-creation registry** (`.claude/rules/session-creation-registry.md`).

Evidence:
- Triage-driven session creation (`server/services/backlog_service_triage.go:800,2854`)
  calls `s.sessionCreator.CreateDirectorySession(...)` directly — a separate
  internal code path from the public `CreateSession` RPC handler in
  `server/services/session_service.go` that the registry's touchpoints #3
  (path guard/switch/mode logic), #5/#6 (Omnibar frontend), and #7
  (`OmnibarContext`/`useSessionService`) all describe. `CreateDirectorySession`
  takes a resolved path and title; it does not consult `SessionType` at all.
- Grepping `session/pipeline_engine.go` and `session/backlog_lifecycle.go` for
  `SessionType|SESSION_TYPE` returns no matches — confirming the backlog→
  session pipeline has never branched on session creation mode by item
  source, for GitHub or otherwise. Adding Linear/JIRA as sources changes
  *which plugin populates* a `BacklogItem`, not how that item becomes a
  session.
- No new `proto/session/v1/types.proto` `SessionType` enum value or
  `CreateSessionRequest` field is implied by anything in requirements.md —
  goal 1/2/3 are entirely about `ItemSourcePlugin` (backend) and an MCP
  import tool, both of which are backlog-item concerns, not session-creation
  concerns.

What Linear/JIRA support *does* require outside the registry's scope (per
§1b/§5 above): a new field on `Session` (proto + Go + frontend) carrying
denormalized source info so a triaged session can render its origin badge —
this is a data-plumbing addition parallel to (not a replacement for) the
`github_*` fields already on `Session`, not a session-creation-mode change.
It has zero interaction with `OmnibarCreationPanel.tsx`'s `SESSION_TYPES`
list, `OmnibarContext.tsx`'s `sessionTypeMap`, or any of the other 6
touchpoints — those all govern how a *user-initiated* session is created
through the omnibar, and Linear/JIRA-sourced sessions are never created that
way (they only ever originate from backlog-item triage).

## Summary of concrete UX deliverables for plan.md

1. New `sourceType`/`sourceLabel` (or reuse `pluginId`) field surfaced on
   `BacklogItem` frontend type — currently absent, needed before any
   Linear/JIRA-aware badge can render conditionally instead of hardcoding
   "GitHub."
2. New denormalized source fields on `Session` (proto + Go + TS gen) —
   `sourceIssueUrl`/`sourceIssueId`/`sourceType` populated at triage time from
   the backlog item's `externalUrl`/`externalId`/`pluginId` — required to
   satisfy "issue URL stored as session metadata and clickable from session
   card" at all; today no such link exists on `Session`.
3. New `IssueSourceBadge` component (or extend `GitHubBadge` into a generic
   variant) rendered from `SessionCard.tsx`, styled with the same neutral
   `repoBadge`-style tokens (`vars.color.surfaceSubtle`/`textPrimary`/
   `borderColor` — confirmed no brand-color tokens exist in
   `theme-contract.css.ts`, so Linear-purple/JIRA-blue branding should not be
   introduced; stay token-neutral per `.claude/rules/css-architecture.md`).
   Icon: inline SVG brand mark (GitHubBadge precedent) or a lucide stand-in
   with a documented substitution comment (BacklogItemCard/SourceSection
   precedent) — lucide 1.14 has no Linear/Jira/Atlassian icons either
   (checked `node_modules/lucide-react/dist/esm/icons/`).
4. New "Filter by issue source" `<select>` in `SessionList.tsx`'s existing
   `filterControls` group, cloned from the Status/Category/Tag select
   pattern exactly (same `select` class, `aria-label`, `all` sentinel).
5. `BacklogSourcesSettings.tsx`: genericize the four hardcoded "GitHub" copy
   strings (lines 352/365/390/392) to use the source's display name/label;
   `isAuthFailure` needs verification against Linear's and JIRA's actual auth
   error response shapes (backend research task) — the row-level "⚠ Sync
   failing" banner mechanism itself needs no structural change.
6. Two new `PLUGIN_SCHEMAS` entries in `backlogSourceSchemas.ts` (Linear: API
   key field; JIRA: base URL + email + API token fields) — explicitly the
   documented extension point, no component changes needed for the Add-a-
   Source form itself.
7. Confirmed no session-creation-registry touchpoints apply (§6) — plan.md
   should not budget work against that checklist.
