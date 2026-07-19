# UX Research: unified-vcs-widget

## 1. Comparable UX patterns in similar products

### GitHub PR page
GitHub's PR page separates **three altitudes** into distinct tabs/zones rather than one flat list: Conversation (narrative + an Overview panel showing merge status, requested reviewers, linked issues), Commits (chronological), and Files changed (resizable file tree with per-file comment/error/warning indicators, syntax-highlighted diff). Checks surface as a compact **rollup row** near the merge button — one line summarizing N passing / M failing with an expandable list, not N separate badges competing for attention. Each check separates a coarse `status` (queued/in_progress/completed) from a granular `conclusion` (success/failure/neutral/skipped/timed_out) — two-tier status modeling, not a single string.

**Directly adoptable**: (a) collapse the check-rollup into one scannable badge ("3/4 checks passing") with expand-on-click for detail, rather than listing every check inline; (b) separate "what changed" (files) from "what happened" (commits/CI) into distinct sections/tabs rather than interleaving; (c) file tree shows inline status glyphs (comment count, error/warning) directly on the row instead of a separate legend.

### GitLab merge request widget
GitLab's MR widget is a vertically-stacked "report card" directly above the merge button: pipeline status widget, then approval widget (reviewer avatars with a green check overlay per approver), then merge-readiness widget (blocked/mergeable) — each is a self-contained horizontal bar that only appears if relevant (no pipeline configured → no pipeline widget, not an empty-state placeholder). Notably: when a pipeline fails but the MR can still merge, GitLab renders the merge button in a *warning* color (not blocking), which is a good pattern for signaling "attention needed but not blocked."

**Directly adoptable**: conditional widget rows (only render a GitHub row if `githubOwner` is set, only render CI row if a check has run) rather than fixed empty-state placeholders — matches this widget's own "no GitHub remote" edge case in requirements §4.

### VS Code Source Control panel
Groups files into named, collapsible sections — **Merge Changes / Staged Changes / Changes** — with per-file `+`/`−` stage/unstage affordances and a file-decoration letter badge (`M`/`A`/`D`/`U`) that matches git porcelain, plus a colored left-border accent per status. Conflicts get their own top section, always first, never buried. Accessibility: VS Code ships a dedicated **Accessible Diff Viewer** (opened via `F7` or a menu command) that renders the diff as a linear, screen-reader-friendly unified patch rather than relying on the visual side-by-side layout — an explicit acknowledgment that a rich two-column diff view is not directly screen-reader-consumable and needs a parallel linear representation.

**Directly adoptable**: (a) conflicts-first ordering (already true in `VcsPanel.tsx`'s `FileList` order — keep it); (b) a single-letter status badge + left-border color-accent combo (already the `FILE_STATUS_META` pattern in `VcsPanel.tsx:26-36` — keep and extend, it's a good model); (c) the Accessible Diff Viewer precedent directly informs the a11y section below — any inline/modal diff view this widget adds needs a non-visual fallback path.

### Linear's git integration card
Deliberately minimal — a single compact row embedded in the issue sidebar: branch name, a colored PR-state pill (Draft/Open/Merged/Closed), and nothing else by default (no CI, no diff stats) unless expanded. Linear's philosophy is workflow-state automation (PR opened → issue moves to "In Review") rather than exposing raw VCS data — the card answers "what stage is this in," not "what changed."

**Directly adoptable**: the *compact row* mode this project's requirements call for (used in list views) should mirror this — one line, one state pill, defer everything else to the expand/detail view. This is a strong model for the "compact list-row mode" required in scope.

### Graphite / Sapling stacked-diff UI
Graphite's PR inbox is the most information-dense of the five: each row shows approval state, merge-conflict status, individual per-check status (not just a rollup), reviewer-by-reviewer status, and last-updated time — organized into pre-built triage sections ("Needs your review," "Approved," "Returned to you") rather than one flat list, i.e., the *grouping* does the scanability work, not row density. Sapling's ReviewStack takes the opposite approach: minimal, just a list of PRs with approval state, prioritizing legibility over density.

**Directly adoptable**: Graphite's lesson is that density is fine *if* rows are pre-grouped by "what needs my attention" — for this widget's Backlog context, that maps to grouping/ordering backlog items by ship-status urgency rather than raw density in a single item's widget. Sapling's lesson is that a dense feature set (this project has by far the richest single-item feature set of any of the five) needs a minimalist container/chrome around it, or density becomes noise — reinforces the compact-mode/full-mode split already in scope.

### Cross-cutting synthesis
No comparable product tries to show *everything* in one flat view. All five use a **two-tier disclosure model**: a compact/summary tier (state pill, rollup badge, approval count) that's always visible, and a detail tier (file list, per-check list, full commit history) that's opt-in via tab, expand, or separate page. This directly validates the requirements' "compact list-row mode" + "full-detail mode" split — the research question isn't *whether* to have two tiers, it's *where the line falls* (see §2).

## 2. Information architecture — the fast-scan requirement

The developer's first question opening a backlog/session detail is **"is this ready to merge / does it need my attention / what changed"** — in that priority order. Mapping the requirements' feature set (PR/CI badges, branch/clean-dirty, ahead/behind, categorized file lists w/ diff stats, commit list, worktree path) to that priority order:

**Above the fold (always visible, no interaction required) — answers "ready / needs attention":**
- Mergeability state as a single pill: Draft / Open+CI-passing / Open+CI-failing / Changes-requested / Approved+ready / Merged / Not-yet-shipped / Conflicted. This is the GitHub-checks-rollup + GitLab-merge-readiness-widget pattern collapsed into one glanceable token — currently this project computes the *inputs* to this state (`githubCheckConclusion`, `githubApprovedCount`, `githubChangesReqCount`, `status.hasConflicts`) but never synthesizes them into one state, forcing the viewer to mentally combine 3-4 separate signals themselves in both `VcsPanel.tsx` (`GitHubSection`) and `ShipStatusDisplay.tsx`. **This synthesis is the single highest-value new UX element the unified widget should add** — none of the three existing surfaces does it.
- Branch name + clean/dirty + ahead/behind counts (already present in `VcsStatusDisplay.tsx`, `ShipStatusDisplay.tsx`) — this is "what state is the code in," second-tier priority.
- Aggregate diff stat (files changed, +N/−N) — present today in `UnfinishedItemDetail.tsx` compact mode; this is the fastest "how big is this change" signal and belongs above the fold in both compact and full mode.

**One interaction away (expand/tab, not a separate page) — answers "what changed":**
- Categorized file lists (conflicts/staged/unstaged/untracked) with per-file diff stats — `VcsPanel.tsx`'s richest feature, keep it as an expandable section rather than always-rendered, since 50+ file lists (edge case §4) would otherwise dominate the fold.
- Commit list (SHA, summary, author, date) — `ShipStatusDisplay.tsx`'s richest feature. Same treatment.
- Worktree path (copy/browse) — utility-tier, belongs in a low-visual-weight footer/metadata row, not competing with status signals for attention.

**Deep-dive without separate navigation:**
- Per-file diff view (View Diff modal, already present via `WorktreeDiffModal`) should open in-place (modal/inline expand) rather than route-navigating away — this is already the pattern in `UnfinishedItemDetail.tsx` and should be preserved/generalized, since requirements explicitly call for "without a separate navigation."

**Color/badge vs. text**: Use color as reinforcement, never as the sole signal (see §3 for the WCAG basis) — every status pill needs a text label or icon+text, matching the pattern `ShipStatusDisplay.tsx` already uses correctly (`✓ Shipped via PR`, not just a green dot). Reserve saturated color (red/green) for the top-line mergeability pill only; secondary metadata (branch name, path, timestamps) should stay in neutral/secondary text color so the eye is drawn to the one signal that matters most first.

**Compact list-row mode** (Backlog list, session list): mergeability pill + branch + aggregate diff stat, single line — mirrors Linear's card and Graphite's inbox row. No file list, no commit list, no GitHub link — those are exactly what "expand to full-detail mode" is for.

## 3. Accessibility findings

Read directly: `VcsPanel.tsx` (267 lines), `VcsStatusDisplay.tsx` (87 lines), `UnfinishedItemDetail.tsx` (204 lines), `ShipStatusDisplay.tsx` (105 lines).

### Already accessible (carry forward as the model)
- `VcsPanel.tsx:81` — file status glyph has `aria-label={meta.label}` (e.g. "Modified", "Conflict — resolve before merging"). Good pattern, keep the `FILE_STATUS_META` lookup-table approach (icon + label + class in one place, `VcsPanel.tsx:26-36`) — it's already the right architecture to prevent icon/label drift.
- `UnfinishedItemDetail.tsx` is the most accessible of the four files: real `<button>` elements throughout (never a clickable `<span>`/`<div>`), `aria-expanded` on the session picker toggle (`:128`), `role="listbox"`/`role="option"`/`aria-selected` on the session picker (`:136-142`), `aria-label="Commits ahead of default branch"` on the commit `<ul>` (`:116`), `aria-label="Generate AI summary of changes"` on the Summarize button (`:165`), `aria-label="Generating summary"` on the loading spinner (`:174`). **This file should be the accessibility baseline the unified widget is built to, not diluted below.**

### Must-fix, not carry forward

1. **Non-keyboard-operable clickable file rows** (`VcsPanel.tsx:84-90`, `FileList`'s `filePath` span): `onClick` is attached to a `<span>` with no `role="button"`, no `tabIndex={0}`, no `onKeyDown` handler. This fails WCAG 2.1.1 (Keyboard) and 4.1.2 (Name, Role, Value) — a screen reader user cannot discover this element is interactive at all (it has no accessible role beyond generic text), and a sighted keyboard-only user cannot tab to it. **Fix in the unified widget: render file rows as real `<button>` elements (as `UnfinishedItemDetail.tsx` already does for its actions) or `<a>` if navigating, never an `onClick`-bearing `<span>`.** This is the single clearest carry-forward-and-fix item given the requirements explicitly ask for clickable file lists.

2. **Color-only/near-color-only CI status** (`VcsPanel.tsx:113-116, 159-161`): `ciColor` is computed from `githubCheckConclusion` and applied via inline `style={{ color: ciColor }}` with **hardcoded hex values** (`#7ee787`, `#f97583`) that bypass the project's theme-token system (`vars.color.*`) entirely — this violates this repo's own CSS architecture rule (`.claude/rules/css-architecture.md`: "Hardcoded hex values in component CSS... use `vars.color.xxx`"), and it means contrast was never verified against the light theme (these look like GitHub's dark-theme-tuned green/red; on a light background — which this app supports, per the CSS architecture doc's `prefers-color-scheme` requirement — contrast ratio is unverified and likely fails WCAG 1.4.3's 4.5:1 for the red especially). The *text itself* ("CI: success"/"CI: failure") does carry the information redundant to color, so this technically isn't a pure 1.4.1 (Use of Color) violation, but the untested-contrast hardcoded-hex issue is real and should be fixed by defining theme-aware `vars.color.ciSuccess` / `vars.color.ciFailure` tokens.

3. **Unlabeled interactive icon-only buttons**: the refresh button (`VcsPanel.tsx:214-216`) has only `title="Refresh"` and a 🔄 emoji as its only content — `title` is not reliably exposed as the accessible name by all screen readers (accessible-name computation prefers text content, which here is the emoji glyph itself, so VoiceOver/NVDA may announce something like "recycling symbol button" rather than "Refresh"). **Fix: add explicit `aria-label="Refresh VCS status"` to every icon-only button** — this pattern will recur heavily in the unified widget given how icon-dense the current three surfaces are.

4. **Meaningful-but-unlabeled inline glyphs**: `githubApproved`/`githubChangesReq` spans (`VcsPanel.tsx:150-155`) render `✓ {count}` / `✗ {count}` with no `aria-label` — a screen reader reads the raw glyph name (often "check mark 2" / "cross mark 2" is fine, but many SR/Unicode-font combos read "check mark" ambiguously or skip decorative-looking pictographic glyphs silently depending on Unicode category). Same issue for the decorative emoji used purely as visual icons where adjacent text already carries meaning (`🌿`/`🔄` VCS-type icon at `VcsPanel.tsx:210`, `⑂`/`⎇` GitHub row icons at `:121, 133`, `⚠️` error icon at `:184`) — these should get `aria-hidden="true"` since the adjacent text is already the accessible label, to stop screen readers from reading noisy/ambiguous glyph names. The two failure modes are opposite and both present in the same file: some icons need an `aria-label` added (because they carry unique information), others need `aria-hidden` added (because they're purely decorative and currently get read as noise) — **the unified widget needs an explicit pass classifying every glyph into "informational → aria-label" vs. "decorative → aria-hidden" rather than treating all Unicode glyphs uniformly.**

5. **Hover-only truncation with no keyboard/touch equivalent**: `UnfinishedItemDetail.tsx:118` (`title={msg}` on a truncated commit-message `<li>`) and `VcsPanel.tsx:79, 87` (`title={meta.label}` / `title="Open in Files tab"`) rely on native `title` tooltips for full text. This fails WCAG 1.4.13 (Content on Hover or Focus) in spirit — `title` tooltips aren't dismissible, aren't guaranteed hoverable/persistent, and are **entirely inaccessible on touch/mobile** (no hover state exists), which directly conflicts with this project's explicit mobile requirement (390px viewport, dark theme, per requirements' Users section). Given the requirements explicitly call out "very long commit messages/PR titles needing truncation with full text on hover/expand," the unified widget must implement expand **on click/tap**, not hover-only, with hover as a progressive-enhancement bonus on desktop.

6. **Async status updates need a live region**: none of the four files use `aria-live` anywhere. `UnfinishedItemDetail.tsx`'s AI summary flow (loading → success/error, `:172-184`) changes on-screen content asynchronously with no announcement — a screen reader user gets no notification that "Generating summary…" appeared or resolved unless they happen to be focused there. Same will apply to the unified widget's CI-status polling/refresh (WCAG 4.1.3, Status Messages) — **wrap async status regions in `aria-live="polite"` (or `role="status"`) so background refreshes and CI-conclusion changes are announced without stealing focus.**

### Summary table

| Issue | File:line | WCAG | Fix for unified widget |
|---|---|---|---|
| Clickable `<span>` file row | `VcsPanel.tsx:84-90` | 2.1.1, 4.1.2 | Real `<button>`/`<a>` |
| Hardcoded hex CI color, unverified contrast | `VcsPanel.tsx:113-116` | 1.4.3 (risk) | Theme tokens (`vars.color.ci*`) |
| Icon-only button, ambiguous name | `VcsPanel.tsx:214-216` | 4.1.2 | `aria-label` on every icon button |
| Unlabeled meaningful glyphs (✓/✗ counts) | `VcsPanel.tsx:150-155` | 1.1.1, 4.1.2 | `aria-label="N approved"` etc. |
| Decorative glyphs read as noise | `VcsPanel.tsx:121,133,184,210` | 1.1.1 (best practice) | `aria-hidden="true"` |
| Hover-only tooltip truncation | `UnfinishedItemDetail.tsx:118`, `VcsPanel.tsx:79,87` | 1.4.13 | Click/tap-to-expand |
| No live region for async status | all 4 files | 4.1.3 | `aria-live="polite"` / `role="status"` |

## 4. Error states and edge cases

- **No GitHub remote (local-only repo)**: match the existing `hasGitHub` early-return pattern (`VcsPanel.tsx:110-111`) and GitLab's conditional-widget-row precedent (§1) — simply omit the GitHub/PR/CI section entirely rather than showing an empty-state placeholder or a disabled badge. A "no remote configured" message is only useful the first time; after that it's visual noise on every local-only item.
- **GitHub API rate-limited / fetch failed**: must not collapse to `VcsPanel.tsx`'s current full-panel error state (`:180-192`, which replaces the *entire* panel with an error+retry, discarding all other VCS info that did load successfully). The unified widget should show the **last-known GitHub data with an explicit "stale" indicator** (e.g., a small "as of 14m ago" timestamp + muted-color badge) rather than blanking the whole widget — this matches the requirements' explicit ask ("should show stale-but-labeled-as-stale data, not a blank error") and is a stronger pattern than any of the three current surfaces implement (none of them currently distinguish "no data yet" from "stale data" from "fetch error"). Only fall back to a hard error state for signals that have *never* successfully loaded.
- **Backlog item with zero linked sessions**: `UnfinishedItemDetail.tsx:58-66` already has this exact case solved (`sessionIds.length === 0` → "Open Session" creates a new one) — reuse this branch logic pattern in the unified widget rather than re-deriving it; it correctly distinguishes 0/1/N sessions with three different button labels/behaviors.
- **Done item, no historical snapshot captured** (pre-dates this backend work): `ShipStatusDisplay.tsx:27-33` already renders `status.error` as a message when present — extend this to a specific, honest copy string ("No history captured for this item — it shipped before detailed tracking was added") rather than a generic error, since this is an expected/benign case, not a failure, and generic red error styling would misrepresent it as something-went-wrong.
- **50+ changed files**: neither full mode nor compact mode should render an unbounded `<ul>`. Full mode: paginate or virtualize past a threshold (e.g. show first 20 per category + "Show all 47 files"), keeping conflicts always fully shown (never truncate the list a user most needs to act on). Compact mode: never list files at all — aggregate stat only (`47 files changed, +812 −340`), per Linear's minimal-card precedent (§1).
- **Long commit messages/PR titles**: single-line `text-overflow: ellipsis` truncation + click/tap-to-expand in place (not hover-only — see §3 finding #5), consistent between the commit-list truncation already in `UnfinishedItemDetail.tsx` and `ShipStatusDisplay.tsx`'s untru­ncated `c.summary` (`ShipStatusDisplay.tsx:88`, which has no truncation at all today — a real gap when a commit summary is long, since `styles.detail` has no visible max-width guard evident from the JSX alone).

## 5. Jobs-to-be-done lens

- **Functional job — "assess mergeability/readiness fast."** The synthesized mergeability-pill from §2 is the direct answer: today a developer must mentally combine `githubCheckConclusion` + `githubApprovedCount`/`githubChangesReqCount` + `status.hasConflicts` + `status.isClean` themselves, reading across up to 4 separate UI fragments depending on which of the 3 surfaces they're on. Collapsing that into one synthesized state token, computed the same way regardless of which surface renders it, is the highest-leverage change for this JTBD.
- **Emotional job — "confidence that nothing is hidden."** The requirements name this explicitly as a hypothesis (fragmentation → distrust). The concrete UX answer isn't just "show more data," it's **consistency of what's shown and where** — if the compact mode on the Backlog list can ever disagree with the full-detail mode on Session detail (e.g., stale cache in one, fresh fetch in the other), that inconsistency is what actually erodes trust, not incompleteness per se. Recommend: a visible "last synced" timestamp on every instance of the widget (compact and full) so a developer can see *why* two views might momentarily differ, rather than silently disagreeing. This also directly serves the "stale data" edge case in §4 — the same timestamp affordance solves both.
- **Social job — "hand off / report status async."** None of the three current surfaces are deep-linkable to a specific file's diff or a specific commit — `WorktreeDiffModal`/diff views open as in-page modals with no URL fragment. Given the explicit scope note that this is used to "hand off/report status to teammates or in an async review flow," the unified widget should support a shareable URL (e.g. `#file=path/to/file.ts` or a query param) that opens the full-detail view pre-expanded to the relevant file/commit — this is a meaningfully new capability beyond what any of the three existing surfaces has, but it's a natural extension of the "full-detail mode" already in scope and directly serves a named JTBD the requirements call out. Flagging as a candidate for the plan phase to size, since it wasn't explicit in the Scope section.

## Sources

- [Improved pull request "Files changed" page — GitHub Changelog](https://github.blog/changelog/2026-01-22-improved-pull-request-files-changed-page-on-by-default/)
- [About status checks — GitHub Docs](https://docs.github.com/articles/about-status-checks)
- [Using the REST API to interact with checks — GitHub Docs](https://docs.github.com/en/rest/guides/using-the-rest-api-to-interact-with-checks)
- [Merge request widgets — GitLab Docs](https://docs.gitlab.com/user/project/merge_requests/widgets/)
- [Merge request approvals — GitLab Docs](https://docs.gitlab.com/user/project/merge_requests/approvals/)
- [Source Control in VS Code](https://code.visualstudio.com/docs/sourcecontrol/overview)
- [Staging and committing changes — VS Code Docs](https://code.visualstudio.com/docs/sourcecontrol/staging-commits)
- [Linear GitHub Integration](https://linear.app/integrations/github)
- [GitHub — Linear Docs](https://linear.app/docs/github-integration)
- [An overview of Sapling's UI — Graphite](https://graphite.com/guides/an-overview-of-sapling-s-ui)
- [Stacked diffs — Graphite](https://graphite.com/guides/stacked-diffs)
- [Comparing Sapling vs. Graphite](https://graphite.com/guides/comparing-sapling-vs-graphite)
