# UX Research: GitHub Provenance Display + Per-Source Sync Direction Controls

Agent 5 (UX), SDD research phase for `backlog-github-two-way-sync`.
Requirements: `project_plans/backlog-github-two-way-sync/requirements.md` (AC #2, #3, #4, #5).

## 0. Existing UI conventions found in this codebase (read first, before inventing anything new)

This repo already has near-identical precedent for every piece of this feature. New work should
extend these, not invent parallel patterns.

| Need | Existing precedent | File |
|---|---|---|
| Icon-only external link to a GitHub issue/PR, with aria-label | `openLink` (👁 emoji) + `aria-label={"Open issue #"+issue.number+" on GitHub"}` + `title="Open on GitHub"` | `web-app/src/components/backlog/GitHubIssuePicker.tsx:423-433` |
| Label/tag chips | `labelBadge` — rendered per-label in an `issueExpandedLabels` row, `title={label}` for overflow | `GitHubIssuePicker.tsx:437-444`, styles in `GitHubIssuePicker.css.ts:122` |
| Issue-vs-PR type badge | `issueTypeBadge` / `prTypeBadge`, single-glyph (`#` / `PR`) | `GitHubIssuePicker.tsx:393-395` |
| External link with icon + text (not icon-only) | `<a><PrIcon aria-hidden /> PR #{n}</a>`, `target="_blank" rel="noopener noreferrer"` | `VcsWidgetGithubRow.tsx:53-56` (uses `lucide-react` `GitPullRequest`/`GitPullRequestDraft`) |
| Status-colored chip (theme-token driven, not hardcoded colors) | `STATUS_CLASS` map → `vars.statusBadge.*Bg/Fg/Border` per status | `BacklogItemBadge.tsx:14-24`, `BacklogItemBadge.css.ts:39-59` |
| Per-row boolean toggle switch | `role="switch" aria-checked={bool}`, `aria-label={"Disable/Enable "+name}` | `BacklogSourcesSettings.tsx:143-149` |
| Expandable "show more" detail panel per row | `expandedId` state, toggle button, lazy-fetch on first expand | `BacklogSourcesSettings.tsx:110-120,172-195` |
| Plain-text PR URL link with title tooltip | `<a href=... title="Open pull request on GitHub">` | `PullRequestSection.tsx:40-48` |

**Two inconsistencies worth flagging to the plan phase, not fixing here:** `GitHubIssuePicker` uses an
emoji (👁) for its external-link icon while `VcsWidgetGithubRow` uses `lucide-react` icons with
`aria-hidden="true"`. The `lucide-react` + `aria-hidden` pattern is more accessible (renders
consistently across platforms/screen readers, doesn't depend on emoji font coverage) and is already
a repo dependency — **new provenance UI should follow the `VcsWidgetGithubRow` icon convention, not
the emoji one.** Recommend `lucide-react`'s `Github` icon for the "imported from GitHub" badge and
`ExternalLink` (already used in `LocalFileBrowser.tsx:158,290`) or `Github` itself as the link icon.

## 1. Comparable UX patterns (Linear, GitHub Projects, Jira GitHub integration)

- **Linear** shows linked GitHub PRs/branches as a small pill directly under the issue title in both
  list and detail view: a GitHub mark-icon + `#123` text, colored by PR state (open/merged/draft/closed),
  clickable straight through to GitHub, with a hover tooltip showing the PR title. Labels/sync config
  live in a separate "Integrations" settings page, not on the issue itself — the issue only ever shows
  the *result* (the linked pill), never the sync toggle. This matches this repo's existing split: card/
  detail shows provenance, Settings > Backlog Sources holds the toggles (AC #2 vs #5 are already
  correctly separated in the requirements).
- **GitHub Projects** (native) shows a repo/issue reference as an inline "chip": owner/repo avatar +
  `#123` + title, and syncs status automatically once a field is mapped — there is no user-visible
  per-field sync toggle at all, which is the failure mode to avoid (see JTBD "trust" below): users
  routinely report Projects silently overwriting manual status edits. This repo's requirement to
  respect `UserModifiedFields` local-wins semantics (AC #4) is explicitly there to avoid that Projects
  complaint.
- **Jira's GitHub integration** ("Development" panel) shows linked branches/commits/PRs as a
  collapsed summary ("1 branch, 2 commits, 1 pull request") that expands to icon + status + link per
  item — it does NOT auto-close a Jira issue on PR merge unless a specific "Smart Commit" or workflow
  rule is explicitly configured per-project, and that configuration is deliberately buried in project
  settings, not per-issue, exactly mirroring "Settings > Backlog Sources, not the item card" here.
- **Common thread across all three**: (a) provenance display uses **icon + identifier + link**, never
  a wall of text; (b) automation that *mutates* the tracker (closing, status flip) is opt-in and
  configured once per integration/project, never inferred per-item; (c) the affected item always shows
  a passive indicator of *what* is linked, but the *control* for whether changes propagate lives one
  level up, in settings.

**Recommendation**: badge/link on the card = icon (Github mark) + `#<number>` + truncated title on
hover/title attr, exactly the shape `VcsWidgetGithubRow`'s PR link already uses. Labels as small chips
below/beside it, reusing `GitHubIssuePicker`'s `labelBadge` styling. Do not put sync-direction state
on the card at all — only a source's `displayName`/link, matching AC #2's own scope.

## 2. User mental models: avoid "forward sync" / "backward sync" jargon in UI copy

"Forward" and "backward" are internal/engineering framing (this repo's own code comments use them —
`session/backlog_sync.go`, requirements.md itself). A user reading Settings > Backlog Sources has no
reason to know which direction is "forward." Recommended plain-language labels, framed by **what
changes and where it goes**, matching the existing `BacklogSourcesSettings.tsx` toggle copy style
(`aria-label={"Disable/Enable "+name}` — action-oriented, not state-oriented):

| Internal term | AC | Suggested UI label | Suggested helper text |
|---|---|---|---|
| Forward sync | AC #3 | **"Close GitHub issues when I finish here"** (or a settings-panel heading: "Update GitHub") | "When a backlog item is marked done, close its linked GitHub issue." |
| Backward sync | AC #4 | **"Reflect GitHub status back here"** (heading: "Update from GitHub") | "When the linked GitHub issue is closed or relabeled, update this backlog item to match." |

Keep both as independent toggles (not a single "two-way sync" master switch) — the requirements
explicitly want them independently configurable per source, default off (AC #3, #4), and a single
combined toggle would misrepresent that they're separate directions with separate risk profiles (see
JTBD/trust below — closing *their* GitHub issue is a much higher-trust action than reading state
*from* GitHub). Group the two under a shared "Sync with GitHub" section per source (reusing
`BacklogSourcesSettings`'s existing per-source card layout) so the pairing is visually obvious without
implying they're coupled.

## 3. Accessibility

- **Contrast**: the "imported from GitHub" badge and label chips must use `vars.color.*` /
  `vars.statusBadge.*` tokens (per `.claude/rules/css-architecture.md`), not a new hardcoded color —
  reuse `vars.statusBadge.inputBg/inputFg/inputBorder` (neutral, already contrast-checked to WCAG AA
  per the `textTertiary` comment in `theme.css.ts` line ~32) rather than introducing a new GitHub-brand
  color that hasn't been contrast-audited against both light and dark themes.
- **Keyboard navigability**: the issue link must be a real `<a href>` (not a `<div onClick>`) so it's
  natively tab-reachable and activates on Enter — exactly what `PullRequestSection.tsx:40-48` and
  `GitHubIssuePicker.tsx:423-433` already do. Because `BacklogItemCard.tsx` puts the whole card in a
  `role="article" tabIndex={0}` with its own `onClick`/`onKeyDown` (lines 152-160), a nested `<a>` needs
  the same `e.stopPropagation()` guard the card's action button already uses (`data-action-button`,
  handled in `handleCardClick` line 133) so clicking/Enter-ing the issue link opens GitHub instead of
  opening the item detail panel underneath it. Add a matching `data-*` guard attribute (e.g.
  `data-external-link`) and extend `handleCardClick`'s `.closest(...)` check.
- **Icon-only badge ARIA**: if the "imported from GitHub" indicator on the compact card is icon-only
  (space-constrained, per `BacklogItemBadge.tsx`'s documented 260px/single-line budget), it needs
  `aria-label="Imported from GitHub issue #<number>"` on the wrapping element and `aria-hidden="true"`
  on the icon itself — the exact pattern already used for `approvedCount`/`changesReqCount` in
  `VcsWidgetGithubRow.tsx:64-77` (`aria-label` on the `<span>`, `aria-hidden` on the icon).
- **Toggle switches**: reuse `BacklogSourcesSettings.tsx`'s existing `role="switch" aria-checked`
  pattern verbatim for the two new sync-direction toggles — it's already accessible and consistent;
  don't introduce a checkbox or a differently-styled switch for these two new controls.

## 4. Error / edge-case UX

- **Sync failure (rate limited, auth revoked, transient API error)**: `BacklogSourcesSettings.tsx`
  already has a per-source sync-history list showing `errorMessage` inline per run (lines 178-184) and
  a top-level `lastError` banner (line 130). Extend that same history entry surface for
  forward/backward sync failures rather than adding a new error surface — e.g. `errored 1
  (rate limited, retry in 12m)` reusing the existing `ev.errorMessage` field. For **auth revoked**
  specifically (a harder failure than transient rate-limiting — it won't self-resolve on retry), the
  source row itself should show a persistent state, not just a buried history line: add a warning
  affordance next to `source.displayName` in the row header (`listItemHeader`, line 140) so it's visible
  without expanding history — same visual weight class as `errorMessage` (line 130) but scoped to the
  row.
- **Issue deleted/transferred upstream**: the backlog item's provenance link becomes a 404 on click —
  this is unavoidable (GitHub, not this app, deleted it) but should not silently break sync. On the
  next backward-sync poll that gets a 404/410 from GitHub for that issue, the item's sync-history
  should log it distinctly from a generic error (e.g. `"issue not found — may have been deleted or
  transferred; backward sync paused for this item"`), and the provenance badge on the card/detail
  should visually flag the broken link (e.g. a muted/struck-through variant of the badge, title
  attribute explaining why) rather than continuing to render an ordinary-looking clickable link to a
  dead URL.
- **Toggle enabled but item has no `ExternalURL`** (locally-created item, no source): nothing to sync.
  Two options: (a) hide the toggle's effect silently (no-op) — bad, violates the "document AI
  decisions in edge cases" convention (`feedback_document_ai_decisions_in_edge_cases` memory) by acting
  invisibly; (b) surface it explicitly. Recommend (b): the provenance section on the detail view (AC
  #2) simply doesn't render at all for items with no `ExternalURL` (nothing to show — matches
  `PullRequestSection`'s own `item.status === "pr_pending"` guard-and-hide pattern, not a "no link"
  placeholder cluttering every non-imported item). The sync toggles themselves live at the *source*
  level in Settings, not per-item, so there's no per-item toggle state to reconcile — the only
  per-item edge case is: forward-sync is on, item transitions to done, but `ExternalURL` is empty. In
  that case log a skipped/no-op entry in sync history ("nothing to sync — no linked GitHub issue")
  rather than either erroring or silently doing nothing untracked.

## 5. Jobs-to-be-done

- **Functional**: "When I close this out on one side, I don't want to remember to go close it on the
  other side too." This is the core value prop and the entire reason AC #3/#4 exist — eliminate the
  manual double-bookkeeping of maintaining status in both a GitHub issue and this app's backlog board.
- **Emotional — trust**: the single biggest risk surfaced by both the GitHub-Projects comparison
  (§1) and the local-wins requirement (AC #4, `UserModifiedFields`) is fear of *silent* automated
  overwrites of a status a user deliberately set. Two things build trust here: (1) both sync directions
  default OFF (already specified, AC #3/#4) so nothing changes until a user opts in per source; (2)
  every automated change this feature makes should be visible after the fact via the existing sync
  history log (`BacklogSourcesSettings.tsx` `historyList`), not just applied invisibly — the same
  "post a visible comment / notify()" principle already established for self-heal/auto-close actions
  elsewhere in this codebase should extend to forward/backward sync writes (log what changed and why,
  per item, per run).
- **Social — team visibility**: when a teammate looks at a backlog item, "was this auto-closed by the
  sync, or did someone deliberately mark it done here?" should be answerable without spelunking. The
  provenance badge answering "where did this come from" (AC #2) is half of that; the other half is
  making sync-driven status transitions distinguishable in whatever activity/history log this repo
  already shows on the item (worth checking during planning whether `ProgressHistorySection.tsx` /
  `WorkflowHistorySection.tsx` — both already listed in the component index — record status
  transitions with a source/actor field; if so, a sync-driven transition should populate that as
  "GitHub sync" rather than blank/anonymous, so a team can tell automated changes apart from a
  person's).

## Summary of concrete recommendations for the plan phase

1. **Card**: small icon+text badge (Github mark + `#<number>`), styled via existing `statusBadge`-style
   tokens, real `<a>` element, guarded against the card's own click handler the same way the existing
   action button is.
2. **Detail view**: a "Source" section (collapsible, following `CollapsibleSection` convention already
   used throughout `detail/*.tsx`) showing the full issue title/link + label chips (reuse
   `GitHubIssuePicker`'s `labelBadge` style), only rendered when `ExternalURL` is non-empty.
3. **Settings**: two new per-source toggles inside the existing `BacklogSourcesSettings` per-source
   row, reusing the exact `role="switch"` pattern already there, labeled in plain language ("Close
   GitHub issues when I finish here" / "Reflect GitHub status back here"), both default off, grouped
   under a shared "Sync with GitHub" sub-heading per source.
4. **Errors**: reuse the existing sync-history `errorMessage` log for transient failures; add a
   persistent row-level warning indicator for non-transient failures (auth revoked, issue
   deleted/transferred) rather than only a buried history line.
5. Use `lucide-react` icons with `aria-hidden` (matching `VcsWidgetGithubRow`), not the emoji
   convention `GitHubIssuePicker` currently uses, for any new icon-only affordances.
