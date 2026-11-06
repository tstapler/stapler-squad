# UX research: showing N PRs per session

Paths are under `web-app/src/`. Findings marked VERIFIED were read in source this session; comparable-product notes are from general knowledge and not re-verified (INFERRED).

## 1. Current state: where a session's PR appears (VERIFIED)

Every surface reads the single `githubPrNumber` / `githubPrUrl` / `githubPrState` / `githubPrIsDraft` fields on the session proto.

| Surface | Component | Behavior |
|---|---|---|
| Session card header | `components/sessions/SessionCard.tsx:714` -> `components/shared/GitHubBadge.tsx` | One badge: PR number, color by `prPriority` (blocking/ready/pending/draft/complete/error), `aria-label="View GitHub Pull Request #N, status: X"`, tooltip with approvals/CI. Renders a repo badge when no PR. |
| Session card body | `SessionCard.tsx:1093` | "Pull Request:" row, one link `#N`, shown only if number > 0 and URL set. |
| Session row (list) | `SessionRow.tsx:448` | Same `GitHubBadge`. |
| Detail view info | `SessionDetailView.tsx:1580` | "GitHub PR: #N (state) [Draft]" as a single link, inline styles. |
| Detail overflow menu | `SessionActionsOverflow.tsx:924` | "View PR #N" link, aria-label includes title. |
| VCS widget | `components/shared/VcsWidget.tsx`, `vcs-widget/VcsWidgetGithubRow.tsx` | One PR link, draft pill, approval/changes counts, CI text; an explicit "Couldn't capture PR status at ship time" empty/error state. Model is `lib/vcs/types.ts` (`data.github` is a single object). |
| Review queue | `ReviewQueuePanel.tsx:546,1255` | Has-PR / no-PR filter, PR number parsed from `githubPrUrl`. |
| Up Next PR tab | `components/unfinished/UpNextTabs.tsx` (Radix `Tabs.List`, `aria-label="Up next sections"`, per-tab count badge) -> `GitHubPRsSection.tsx` -> `prs/PRGroupedList.tsx` -> `prs/PRCard.tsx` | List of the user's open PRs from GitHub (`UserPR`), not session-keyed. Chips use token pairs in `lib/unfinished/prChipTokens.ts` with a contrast test. |
| PR panel actions | `prs/PRActionBar.tsx`, `lib/hooks/usePRActions.ts` | Merge (method persisted in localStorage), close (focus-trapped confirm dialog), draft toggle; acts on a `UserPR`, so it is already PR-identified, not session-identified. `NudgeButton.tsx` is the "Ask to fix" action. |
| Worktree list | `components/unfinished/UnfinishedItemDetail.tsx:89` | One `GitHubBadge`. |
| Backlog | `components/backlog/detail/PullRequestSection.tsx` (rendered only while `status === "pr_pending"`), `VersionControlSection.tsx`, `LifecycleSummary.tsx`, `backlog-stuck/StuckItem*.tsx` | Single-PR assumptions. |

Observations:
- `GitHubBadge` is the one reusable atom used by card, row, worktree list. Replacing it with a PR-set-aware component fixes three surfaces at once.
- The Up Next PR tab is already PR-granular; multi-PR needs a "belongs to session X" cue there, not a redesign.
- The detail view uses ad hoc inline styles; the multi-PR section there is a chance to move to vanilla-extract tokens.
- Mobile: PRCard has a `NARROW` breakpoint and a 44px chip target (`PRCard.css.ts:113`); `GitHubBadge.css.ts` has a `compact` variant and media queries. Reuse both.
- Existing ARIA is decent (labelled links, `aria-hidden` icons, `role="status"` for list changes) but state is color-first in the badge; text label exists only in aria-label/tooltip when not compact.

## 2. Job to be done

"What is the state of the work this session produced, and what do I need to do next?" Concretely, in order of frequency:
1. Glance (list): is anything waiting on me? One answer per session.
2. Open the PR that matters now (the one needing review/merge/fix).
3. Confirm earlier PRs landed (history; audit).
4. Act on a specific PR (merge, close, ask-to-fix) without hitting the wrong one.

Implication: the list view answers 1 and 2; the detail view answers 3 and 4. N PRs must never multiply list-level noise.

## 3. User mental model

- Operator thinks "this session -> its PR(s)", and for stacked work "PR chain, bottom to top". Merged PRs are history, not tasks.
- A session has one "current" PR (what the agent is working on or waiting on) plus a trail. Matches the requirement's "active PR" default.
- Distinct relationships the user will read into ordering: stack (B based on A), follow-up (opened after A merged), rework (replaces A). Role label per PR is useful but secondary; ordering by created time with a role chip is enough.

## 4. Comparable patterns (INFERRED, general knowledge)

- GitHub stacked-PR / "Related PRs" lists and the PR sidebar "Development" section: compact rows, icon (open/merged/closed/draft) + title + #number, linked; collapses long lists.
- Graphite and Linear PR chains: a vertical chain/stack list with the current PR highlighted, per-row status icon, a "stack of N" summary at the top; Linear issue shows linked PRs as a short row list with state icons and a single aggregated status on the issue itself.
- Common across them: aggregate status at the parent, per-item detail one click down, state conveyed by icon shape plus color, newest/active first.

## 5. Recommendation: N PRs without clutter

Progressive disclosure in three tiers:

1. **List/card/row (always compact): one primary badge + count.** Keep today's `GitHubBadge` for the active PR (active = most recent open PR, else most recent PR). Add a small "+N" suffix chip only when N > 1 (`aria-label="2 more pull requests"`), which opens the detail PR section on click. Never render N badges in the card header. If no PR is open but history exists, show the latest merged/closed PR with its real state label, not blank.
2. **Detail view: "Pull requests" section** (replace the single "GitHub PR:" row). A list ordered active first, then open by recency, then merged/closed; each row: state icon (open, draft, merged, closed - distinct shapes), `#N`, title, role chip (stack/follow-up/rework) when known, CI/review summary for open PRs only, external-link affordance. Merged/closed rows are visually quieter and collapse under "N earlier PRs" when more than 3 rows. Per-row actions (merge, close, ask-to-fix) live in the row's overflow, acting on that PR explicitly; the section header states which PR is the default ("Actions apply to #12 unless you pick another").
3. **VCS widget / action surfaces**: show the active PR as today; add a PR switcher (segmented control or select) only when more than one open PR exists, defaulting to the active PR. This is the guard against acting on a stale PR.

Up Next PR tab: add a muted "session: <name>" chip on cards whose PR belongs to a tracked session; do not group by session by default.

Backlog `PullRequestSection`: show "k of N PRs merged" progress plus the same row list, so the completion rule (all merged/closed) is legible.

### States

- **Empty (no PRs)**: keep existing repo badge fallback; detail section reads "No pull requests yet" with the create-PR action (`CreatePullRequestModal`).
- **Loading/unpolled**: gray `unknown` badge as today; row shows state text "Checking..." not blank.
- **Error**: per-row error chip (auth/rate-limit) rather than a section-level failure; one failed PR must not hide others. Reuse the "Couldn't capture PR status" copy pattern. Rate-limit banner via `lib/vcs/githubRateLimit.ts`.
- **Migrated legacy session**: one row, no role chip, no "earlier PRs" disclosure.
- **PR deleted/inaccessible on GitHub**: keep the row with last known state and a "can't reach" note; do not drop history.
- **Bounded**: requirements say tens at most; cap visible rows (e.g. 5) with "Show all".

### Accessibility (WCAG 2.2 AA)

- State must not be color-only (1.4.1): icon shape + visible text label on every row; `GitHubBadge` non-compact already shows text, compact currently relies on color + aria-label - add a title/text for compact, or an icon shape.
- Use a semantic list (`<ul>`/`<li>`); active PR marked `aria-current="true"`; "+N" chip is a real `<button>` with `aria-expanded` if it expands in place, or a link if it navigates.
- Row actions: native buttons, reachable by Tab; overflow menu follows the existing Radix pattern; close confirm reuses the focus-trapped dialog (`useFocusTrap`).
- Announce state changes of polled PRs via a `role="status"` region (pattern exists in `PRGroupedList.tsx:108`), debounced so polling does not chatter.
- Targets at least 24px (2.5.8), 44px for touch as `PRCard.css.ts:113` already does; contrast verified via the `prChipTokens.ts` token pairs - add new chip states to `CHIP_TOKEN_PAIRS` so `chipContrast.test.ts` covers them.
- Link names must be unique: `aria-label` includes `#N`, owner/repo, "opens in new tab" (as `SessionCard.tsx:1103` does).
- Disclosure of earlier PRs: `<button aria-expanded aria-controls>`; keyboard Enter/Space.
- E2E locators must use `data-testid`/roles per repo convention.

### Mobile vs desktop

- Mobile (NARROW breakpoint): card shows the single compact badge + "+N"; tapping goes to detail. Detail PR rows stack vertically: line 1 icon + #N + state, line 2 title (wrap with `overflow-wrap: anywhere`, as PRCard does), actions in an overflow button, not inline. No horizontal scroll; no hover-only info (tooltip content must also be in the row).
- Desktop: rows are single-line with inline CI/review summary and visible actions; stack relationship can be drawn as an indented/connector line for stacked PRs.
- Both: the PR switcher is a segmented control on desktop and a native select on mobile.

### Pitfalls to avoid

- Rendering all PRs on the card (clutter at N >= 3).
- Aggregating into one color: a merged A plus failing B must show B's problem, so the card badge reflects the active PR, not a blend.
- Defaulting actions to "first in list": must be the active PR, with the target named in the UI.
- Re-polling merged/closed rows: show cached state, no spinner.

## 6. Touchpoints to change (for planning)

`GitHubBadge.tsx` (+ `.css.ts`), `SessionCard.tsx` (header + body row), `SessionRow.tsx`, `SessionDetailView.tsx` (replace the row at 1580), `SessionActionsOverflow.tsx:924`, `VcsWidget.tsx` / `vcs-widget/VcsWidgetGithubRow.tsx` + `lib/vcs/types.ts`/`adapters.ts`, `ReviewQueuePanel.tsx` filters, `UnfinishedItemDetail.tsx`, backlog `PullRequestSection.tsx` / `VersionControlSection.tsx` / `LifecycleSummary.tsx` / `StuckItem*.tsx`, `prChipTokens.ts`, `route-meta.ts` only if a new page is added (not needed here). Generated `gen/session/v1/*_pb.ts` follows the proto change.

## 7. Open UX questions for the owner

1. Is "active PR" derived (most recent open) or user-pinnable? Recommendation: derived, with an optional pin only if stacked use cases demand it.
2. Show role chips (stack/follow-up/rework) at launch or defer until the data model records role reliably?
