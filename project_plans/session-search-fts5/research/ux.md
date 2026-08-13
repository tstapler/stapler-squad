# Research: UX for "Find Related Past Work" in Triage

Scope per requirements.md: a search box in the backlog triage panel, pre-populated
with the backlog item title, returning session-deduped discovery results with
snippets/context, plus a scroll mode for paging a session's messages around a hit.
This doc covers placement, the comparable existing pattern to reuse, result-card
content, accessibility, error/empty states, and jobs-to-be-done — not the RPC/response
shape (that's Phase 3 planning, per requirements.md Open Question 2).

## 1. Where it fits in `TriageReviewPanel.tsx`

Read `TriageReviewPanel.tsx` (`web-app/src/components/backlog/TriageReviewPanel.tsx`)
end to end. The panel's existing layout is a strict top-to-bottom stack, each block
separated by `<hr className={styles.divider} />` (lines 225, 242):

1. Header (`panelHeader`, lines 184–203) — "Triage Ready" + iteration badge + dismiss
2. Error banner (conditional, lines 206–215)
3. Summary (`summarySection`, lines 218–221)
4. `<hr>` + Suggested AC diff (`TriageDiffSection`, lines 223–234) — conditional on
   `hasSuggestions`
5. `<hr>` + Implementation plan / task list (lines 240–261) — conditional on `hasTasks`
6. Actions row (Apply/Skip/Refine, lines 266–313) — omitted entirely in `readOnly` mode
7. Refine feedback form (conditional, lines 316–366)

**Recommendation: insert a new "Find related past work" block between the Summary
(step 3) and the AC diff (step 4)**, its own `<hr>`-delimited section following the
exact `sectionLabel` + content pattern already used for Summary/AC/tasks
(`styles.sectionLabel`, `TriageReviewPanel.css.ts:53-60`). Rationale:

- It answers "has this been tried before" *before* the user evaluates the suggested AC
  diff — the natural reading order for the JTBD (see §6): confirm this isn't redundant
  work *before* spending time reviewing what the triage agent proposed.
- It must **not** appear in `readOnly` mode's DOM in the same way the Actions block is
  omitted (line 266's `{!readOnly && (...)}` precedent) — a historical diagnostic
  record (Story 4.1.2, lines 28–39) is inherently backward-looking, so a live,
  debounced search box editable by the viewer doesn't belong there. Two sub-options,
  pick in planning:
  - Omit entirely in `readOnly` mode (matches the Actions-block precedent exactly), or
  - Render read-only (input disabled, pre-filled, showing the results it had *at the
    time*) — over-engineering for a v1; the Actions-block precedent (fully omit) is
    the cheaper, consistent choice and is what this doc recommends.
- It's a **new component**, `TriageRelatedWorkSection.tsx`, sibling to
  `TriageDiffSection.tsx`/`TriageErrorBanner.tsx`/`TriageLoadingIndicator.tsx` — not
  inlined into `TriageReviewPanel.tsx`, matching the existing decomposition (each
  section of the panel already gets its own file + `.css.ts`).
- Collapsed-by-default vs. always-expanded is a real trade-off: the panel already
  stacks Summary + AC diff + tasks, all of which can be tall (task list has no visible
  cap). Recommend the search box always renders (visible affordance, satisfies "no
  extra click to discover the feature"), but the **results list is collapsible** once
  populated, so a "no matches" or "3 matches" outcome doesn't permanently add height to
  every triage panel a user scrolls past. Default state: expanded if there are
  results, since surfacing prior art is the point of the feature.

## 2. Reuse `HistorySearchInput`/`HistorySearchResults`, don't reinvent

Read `web-app/src/components/history/HistorySearchInput.tsx` and
`HistorySearchResults.tsx`, plus the hook they're built on,
`web-app/src/lib/hooks/useHistoryFullTextSearch.ts`. This is the comparable pattern
requirements.md points at, and it's a good one to mirror closely rather than design
from scratch:

- **Debounce**: `useHistoryFullTextSearch` composes `useDebounce` (300ms default,
  `useHistoryFullTextSearch.ts:98,110`) with an auto-search effect
  (`useHistoryFullTextSearch.ts:253-264`) — query changes flow
  `setQuery` → debounced value → auto `search()` call. The triage box should reuse
  this exact hook (or a thin variant of it once the discovery RPC lands), not
  hand-roll a new debounce.
- **Cancellation**: in-flight requests are aborted via `AbortController` on every new
  search (`useHistoryFullTextSearch.ts:163-166`) and on unmount
  (`useHistoryFullTextSearch.ts:267-273`) — necessary here too, since triage panels can
  mount/unmount rapidly as a user moves through the backlog list.
- **Loading state**: `HistorySearchInput` swaps the search icon for a spinner
  (`HistorySearchInput.tsx:154-163`) rather than a separate loading row — compact,
  keeps the input's height stable. `HistorySearchResults` additionally shows a full
  loading block only when `loading && results.length === 0`
  (`HistorySearchResults.tsx:166-173`) — i.e., a fresh search shows a loading state,
  but paging/appending (load-more) does not blank the existing list. Same rule should
  apply to triage discovery.
- **Result card shape** (`SearchResultCard`, `HistorySearchResults.tsx:83-134`): title
  (session name or ID) + model badge + project path + date + relevance score percentage
  + up to 3 highlighted snippets (`HighlightedSnippet`, lines 31-78, role-labeled
    "You"/"Claude"). This is a solid base but is **message-level, not session-level** —
  see §3 for what changes once results are session-deduped.
- **Empty/no-query state**: `HistorySearchResults.tsx:148-163` shows a magnifying-glass
  icon + "Search across all your Claude conversations" + a hint line when `query` is
  empty. The triage variant's equivalent no-query state should instead invite the
  triage-specific action, e.g. "Searching for prior work related to this item" (see §5
  for the pre-populated-title case specifically).
- **Search-history dropdown** (`HistorySearchInput.tsx:195-245`, recent searches via
  `useSearchHistory`): **skip this in the triage variant.** The box starts pre-filled
  with the backlog item title (a JTBD-driven default, not a blank field the user types
  into repeatedly), so a "recent searches" affordance adds UI weight without a matching
  use case — the user is far more likely to *edit* the pre-filled title than to want a
  history of past ad hoc searches inside a triage panel they'll see once per item.
  Reuse the debounce/loading/result-card parts of the pattern; drop the history
  dropdown.

## 3. Result card content — session dedup changes what "answers the question fastest"

The JTBD (§6) is "has this been tried before, and what happened" — a title alone
cannot answer either half. Ranked by how directly each field answers the two halves:

**Answers "has this been tried before" (fast scan):**
1. **Session title/name** (already in `SearchResultCard`) — the anchor for
   recognition ("oh, that's the auth refactor from last month").
2. **Date** (already present, `formatDate`, `HistorySearchResults.tsx:90-97`) —
   recency matters for triage: a 2-day-old attempt is a live signal, a 6-month-old one
   is background context. Keep prominent, don't bury in metadata row.
3. **Match count within the session** ("4 messages matched") — new, required by
   session-level dedup (requirements.md AC: "top hit per session, others available as
   'N more matches in this session'"). This is also a rough proxy for "how deep did
   this go" — one incidental mention vs. a whole session about the topic.

**Answers "what happened" (the harder half, and the one current `SearchResultCard`
cannot answer at all today):**
4. **Outcome/status, when available.** This is the highest-value field for the triage
   JTBD but **does not exist on the search result path today.** Read
   `useBacklogService.ts`'s `LinkedSession` type (lines 57-70): a session linked to a
   backlog item carries `reviewVerdict.overallOutcome` (`PASS | PARTIAL | FAIL |
   PENDING | UNVERIFIABLE`, line 66) — a real, already-modeled "what happened" signal.
   But `SearchClaudeHistory`'s results (`useHistoryFullTextSearch.ts:34-54`,
   `SearchResultItem`) come from the **general Claude history index**
   (`session/search/`), which has no concept of backlog linkage or review verdict —
   most sessions surfaced by a keyword search will *not* be backlog-linked and will
   have no outcome to show. **Recommendation for planning**: the result card must
   degrade gracefully — show an outcome badge (reusing the existing PASS/PARTIAL/FAIL
   visual language wherever it's already styled in the codebase, e.g. gate verdict
   badges) *only* when the search result can be cross-referenced to a `LinkedSession`
   with a `reviewVerdict`; omit the badge silently otherwise rather than showing an
   empty/placeholder state. Whether that cross-reference is feasible without a new
   join (session ID → backlog item → review verdict) is a Phase 3 architecture
   question, not a UX one — flagging it here because it changes the card's information
   hierarchy if unavailable.
5. **Snippet(s) with highlighted match** (already present) — this is the fallback
   "what happened" signal when no structured outcome exists: the surrounding text is
   the only evidence of what was discussed/decided. This is *why* the ±5-message
   context window and snippet quality matter more here than in general history search —
   in general search the user already knows roughly what they're looking for; in
   triage discovery the snippet **is** the evidence for a decision ("skip this AC, it
   was already tried and failed — see message here").

**Recommended card shape** (session-deduped), in visual priority order: title → date +
match count → outcome badge (if resolvable) → top snippet (highest-scoring hit) →
"N more matches in this session" affordance (expand or link into scroll mode) → project
path (de-emphasized, already lowest-priority in the existing card). This is additive to
today's `SearchResultCard`, not a redesign — same visual language, reordered priority
and one new conditional badge slot.

## 4. Accessibility

- **Live region for search-as-you-type**: neither `HistorySearchInput` nor
  `HistorySearchResults` currently wires an `aria-live` region for result-count
  announcements — worth confirming as a gap, not a pattern to copy blindly. What *does*
  exist and is copy-worthy:
  - `HistorySearchInput.tsx:174-179` — the input itself is `role="combobox"` with
    `aria-label="Search conversation history"`, `aria-expanded={showHistory}`,
    `aria-controls="search-history-dropdown"`, `aria-autocomplete="list"`. Since the
    triage variant drops the history dropdown (§2), the combobox/aria-expanded/
    aria-controls trio isn't needed — a plain labeled `<input type="search">` with
    `aria-label="Search past sessions for <item title>"` is sufficient and simpler.
  - `TriageReviewPanel.tsx:178-182` — the **parent panel section** is already
    `aria-live="polite"` (`<section className={styles.panel} aria-live="polite" ...>`).
    Because the new search block lives inside this section, result-count changes
    ("3 sessions found") will already be announced via the ancestor live region **once
    the DOM text changes** — no need to add a second nested `aria-live` region (nested
    live regions on the same subtree are redundant and can cause duplicate
    announcements). This is a meaningful finding: the accessibility groundwork for
    "announce search results" is already in place at the panel level; the new
    component just needs to update visible text, not add its own `aria-live`.
  - Exception: if the results list is collapsible (per §1), the **loading state**
    ("Searching…") and **result count** should still update within that same
    `aria-live="polite"` ancestor — verify no `aria-live="off"` or portal boundary
    breaks the containment (the undo toast, `TriageReviewPanel.tsx:146-166`, *is*
    portaled to `document.body`, which is a precedent for when a live region needs to
    escape the panel — the search results block does not need this, it stays in-DOM).
- **Keyboard navigation for results**: `HistorySearchResults`' `SearchResultCard` is
  `role="button" tabIndex={0}` (`HistorySearchResults.tsx:100`) but only has an
  `onClick` handler — **no `onKeyDown` for Enter/Space**, which is an accessibility gap
  in the existing pattern (a `role="button"` div is not natively keyboard-activatable
  the way a real `<button>` is). Recommendation: either (a) fix this in the shared
  component if it's extracted/reused, or (b) use a real `<button>` element (or
  `<a>`/Link if navigating) for the new triage result cards instead of copying the
  `role="button" div` pattern forward. Given `TriageReviewPanel.tsx` elsewhere always
  uses real `<button>` elements (Apply, Skip, Refine, dismiss — never a div with
  `role="button"`), matching the *panel's* existing convention over the *history
  page's* is the more consistent choice for a new component in this codebase.
- **Result list semantics**: consider `role="list"` / `role="listitem"` (or a native
  `<ul>/<li>`) wrapping the result cards so screen reader users get a count
  announcement ("list, 3 items") for free — `HistorySearchResults.tsx:221-229` uses a
  plain `<div>` wrapper today, another gap worth not copying forward.
- **Focus management**: when the panel mounts with a pre-filled query and
  auto-searches (§5), don't auto-focus the input and don't move focus into the results
  — the triage panel already has its own focus entry points (Apply/Skip buttons), and
  stealing focus on mount for a background-populated search box would be a surprise
  interruption, not a helpful default.

## 5. Error / empty / edge states

Four distinct states to design for, each needing different copy (compare
`TriageErrorBanner.tsx`'s existing tone — direct, states what's wrong, offers exactly
two actions):

1. **No matches found** (query ran, zero results). Distinguish this from state 3
   below — "we searched and found nothing" is a *positive* signal for triage (genuinely
   novel work), not a failure. Copy should say so explicitly, e.g. "No related past
   sessions found — this looks like new territory," not a generic "no results" that
   reads like an error. Compare `HistorySearchResults.tsx:190-206`'s existing "No
   results found for X" — that copy is neutral/generic and fine for general search, but
   the triage context benefits from reframing a null result as reassuring rather than
   disappointing, directly serving the emotional JTBD (§6).
2. **Search index still syncing.** Per requirements.md's baseline
   (`session/search/engine.go`'s `IncrementalSync(hist)` runs at the top of every
   `SearchClaudeHistory` request — pull-based, not a background watcher), there is no
   persistent "index not ready" state today — sync happens synchronously per-request,
   so the closest real state is just "still loading" (the existing spinner treatment,
   `HistorySearchInput.tsx:154-163` + `HistorySearchResults.tsx:166-173`), not a
   distinct "syncing" message. **Flag for planning**: if a discovery-specific RPC adds
   any async/background indexing step this doesn't already have, a distinct
   "still indexing your history…" state would need new copy; based on current
   architecture this is not expected to be a real state to design for.
3. **Query failed (network/RPC error).** Reuse `TriageErrorBanner.tsx`'s existing
   pattern directly — `role="alert"`, a specific message, two actions. But
   **don't reuse the component wholesale**: `TriageErrorBanner`'s two actions are
   "Reload item" / "Skip without applying" (`TriageErrorBanner.tsx:24-36`), which are
   about the *triage result*, not the *search*. A search failure needs its own
   narrower recovery: "Search failed — [Retry]" inline near the search box, not a
   panel-wide banner that could be confused with a triage-apply error. Keep the
   `role="alert"` + concise-message convention; don't inherit the specific button set.
4. **Backlog item has no title yet.** Read `useBacklogService.ts:93-94` — `title` is a
   required, non-optional `string` on `BacklogItem`, so a *saved* item always has a
   title. The realistic edge case is an **empty or placeholder title on a very fresh
   draft** (e.g. an item mid-creation-flow, if this panel could ever render before the
   title is finalized) rather than a genuinely-undefined field. Given the panel only
   renders when `triageStatus === "completed"` (per the component doc comment, line
   62), triage has already run against *some* title text by the time this panel is
   visible — so in practice the pre-fill will essentially never be blank. Still worth a
   defensive UI rule: if the pre-filled query would be empty or whitespace-only, don't
   auto-search (mirrors `useHistoryFullTextSearch.ts:150-160`'s existing guard: an
   empty/whitespace query already short-circuits to a cleared, non-loading state) —
   show the box focused-and-empty rather than firing a query that can't return anything
   meaningful.

## 6. Jobs-to-be-done

- **Functional**: "Before I evaluate this triage suggestion, tell me if this exact
  problem has already been worked on, and let me see enough of that prior conversation
  to know whether it succeeded, failed, or is still open — without leaving this panel
  or hand-writing a search query." The pre-filled title exists specifically to remove
  the "hand-writing a search query" tax; the JTBD fails if the user still has to think
  of good search terms themselves for the common case.
- **Emotional**: confidence and avoiding repeated mistakes — the operator (human or
  triage agent) wants to *not* rediscover a dead end that a past session already
  proved doesn't work, and wants to *not* accidentally duplicate effort that's already
  in flight or already shipped. A "no matches" result should read as reassuring
  ("genuinely new," not "search is broken" or "you forgot something") — see §5.1. A
  "found 3 matches, one FAILED" result should read as a warning worth reading before
  clicking Apply, which is why the outcome badge (§3) is the single highest-leverage
  piece of information the card can carry when it's resolvable.
- **Social**: not applicable — single-user tool, no sharing/comparison surface for this
  feature (consistent with requirements.md's own N/A framing for social JTBD).

## Open questions for `sdd:3-plan`

1. Read-only-mode treatment (§1): omit the search block entirely (Actions-block
   precedent) vs. render a frozen/disabled snapshot. Recommend omit — cheaper, matches
   existing precedent exactly.
2. Whether outcome-badge cross-referencing (§3, search result → `LinkedSession.
   reviewVerdict`) is feasible from the discovery RPC without a new join — if not
   feasible in v1, the card ships snippet-only (still valuable) and the outcome badge
   becomes a fast-follow once the RPC/response shape (requirements.md Open Question 2)
   is settled.
3. Collapsible-results default (§1) — collapsed-until-results vs. always-expanded once
   any query (even the auto-fired pre-filled one) has run. Recommend: collapsed while
   loading, auto-expanded on results, so a "no matches" state doesn't visually punch a
   hole in the panel layout while still being announced via the ambient `aria-live`
   region (§4).
4. Whether the "N more matches in this session" affordance (§3) opens Scroll mode
   inline (within the triage panel, likely too cramped) or navigates out to the
   session detail page anchored on the hit — Scroll mode's actual surface (modal?
   dedicated route? inline expansion?) is not decided by this doc and needs an
   architecture call in Phase 3.
