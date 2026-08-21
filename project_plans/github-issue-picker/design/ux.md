# UX Design: GitHub Issue Picker

Date: 2026-07-03
Verdict: CONCERNS

Six UX concerns found — none block implementation, all must be resolved before the PR ships. The plan's core interaction model is sound. The concerns are in incomplete ARIA spec on the issue input, missing empty states, no visual affordance for the two-level Escape behavior, misleading auth error copy, unspecified placeholder text, and unexplained skeleton row count discrepancy.

---

## Two-Phase Flow

The picker operates in two sequential phases within the existing backlog import modal (~480px wide, inheriting the outer shell's Cancel button and import-status row).

### Phase 1 — Repo Selection (`PickerPhase = "repo-selection"`)

On mount the input receives focus. Two repo tiers populate the list immediately: local repos (derived synchronously from Redux `sessionsSlice` — zero latency, no RPC) appear first, then GitHub repos fetched via `SearchGitHubRepos` (cached 4 h). Three skeleton rows fill the GitHub tier while that fetch is in-flight.

If the user types or pastes a full GitHub issue URL (`https://github.com/owner/repo/issues/N`) into the repo input, `issueURLPattern` matches and a "Direct import" affordance replaces the normal list. Pressing Enter or clicking the affordance calls `onImport(url)` directly — repo selection is bypassed.

On any repo selection, the picker writes the selection to `lastRepoCacheKey` (no TTL) and transitions to phase 2. On subsequent modal opens the hook reads `lastRepoCacheKey` on mount and pre-populates `selectedRepo`, jumping directly to phase 2.

### Phase 2 — Issue Search (`PickerPhase = "issue-search"`)

A `RepoChip` ("owner/repo ×") sits above the issue search input. `isIssuesLoading: true` is set before the 150 ms debounce timer fires, so five skeleton rows appear immediately when the repo is first selected — the user never sees a blank list while the initial fetch is in-flight.

The `IssueFilterBar` offers Open / Closed / All state toggles and a label substring filter. State toggles trigger a new RPC (debounced); label filtering is client-side only (no RPC). The list shows up to 30 issues per page.

Selecting an issue calls `onImport(issue.url)` immediately — no confirm step.

---

## Component Tree

```
GitHubIssuePicker
├── useGitHubIssuePicker (hook)
│   ├── issuePickerCache (localStorage TTL utility)
│   └── useBacklogService
│       ├── SearchGitHubRepos RPC
│       └── ListGitHubIssues RPC
│
├── [phase = "repo-selection"]
│   └── RepoSelector
│       ├── <input role="combobox">          ← repo search
│       │   data-testid="repo-search-input"
│       ├── [detectedIssueUrl]
│       │   └── DirectImportAffordance       ← "Import #N from owner/repo →"
│       └── <ul role="listbox" id="gh-repo-listbox">
│           ├── <li role="presentation">     ← local/GitHub divider (if both present)
│           ├── <li role="option" id="gh-repo-listbox-option-{i}"> × N
│           │   └── [isLocal] → "Recently used" or local badge
│           └── [isLoading && githubRepos.length === 0]
│               └── SkeletonRows (3 rows)
│
└── [phase = "issue-search"]
    ├── RepoChip                             ← "owner/repo ×" ABOVE issue input
    │   └── <button aria-label="Clear repo selection"> ×
    ├── IssueFilterBar
    │   ├── <button> Open
    │   ├── <button> Closed
    │   ├── <button> All
    │   └── <input>                          ← label filter (client-side)
    ├── <input role="combobox">              ← issue search
    │   data-testid="issue-search-input"
    └── IssueList
        ├── <ul role="listbox" id="gh-issue-listbox">
        │   └── IssueRow × N
        │       ├── <span> #N               ← muted color
        │       ├── <span>                  ← state dot (green/purple)
        │       ├── <span>                  ← title (truncated)
        │       └── <span> × ≤3             ← label chips
        ├── [isIssuesLoading]
        │   └── SkeletonRows (5 rows)
        └── [empty, not loading]
            └── EmptyState (see Strings table — variants still incomplete, §4)
```

---

## Keyboard Map

| Key | Phase | Action |
|-----|-------|--------|
| `ArrowDown` | either | Move highlight to next item; clamp at last item |
| `ArrowUp` | either | Move highlight to previous item; at first item, return to input (index = -1) |
| `Enter` | repo-selection | Select highlighted repo; transition to issue-search |
| `Enter` | issue-search, item highlighted | Select highlighted issue; call `onImport(issue.url)` |
| `Enter` | issue-search, no item highlighted | Submit current search query (no-op if no results) |
| `Escape` | issue-search, `issueSearch` set or `selectedRepo` set | `e.stopPropagation()`; clear `issueSearch`; set phase = repo-selection |
| `Escape` | repo-selection (or issue-search, nothing to clear) | Call `onClose()` |
| `Tab` | either | Dismiss dropdown; native focus traversal to Cancel button (no explicit handler — relies on default browser behavior; see Open Question §8) |
| Any character | either | Clear `aria-activedescendant`; update query state |

The two-level Escape is correctly implemented in Task 3.2.3. The condition `picker.phase === "issue-search" && (picker.issueSearch || picker.selectedRepo)` is safe in practice because `selectedRepo` is always set when phase is "issue-search", making the Escape guard always trigger the back-navigation path.

---

## ARIA Attribute Table

| Component | Attribute | Value |
|-----------|-----------|-------|
| Repo input | `role` | `combobox` |
| Repo input | `aria-expanded` | `"true"` when list is visible |
| Repo input | `aria-controls` | `"gh-repo-listbox"` |
| Repo input | `aria-autocomplete` | `"list"` |
| Repo input | `aria-activedescendant` | `"gh-repo-listbox-option-{i}"` or `undefined` |
| Repo list | `role` | `listbox` |
| Repo list | `id` | `"gh-repo-listbox"` |
| Repo option | `role` | `option` |
| Repo option | `id` | `"gh-repo-listbox-option-{i}"` |
| Repo option | `aria-selected` | `i === selectedIndex` |
| Issue input | `role` | `combobox` |
| Issue input | `aria-expanded` | `"true"` when list is visible |
| Issue input | `aria-controls` | `"gh-issue-listbox"` |
| Issue input | `aria-autocomplete` | `"list"` — **MISSING FROM PLAN SPEC** (Story 3.2 AC omits it; Task 3.2.2 never adds it) |
| Issue input | `aria-activedescendant` | `"gh-issue-listbox-option-{i}"` or `undefined` |
| Issue list | `role` | `listbox` |
| Issue list | `id` | `"gh-issue-listbox"` |
| Issue option | `role` | `option` |
| Issue option | `id` | `"gh-issue-listbox-option-{i}"` |
| Issue option | `aria-selected` | `i === selectedIndex` — **NOT STATED in Task 3.2.2 steps; must be confirmed** |
| RepoChip × | `aria-label` | `"Clear repo selection"` |
| Auth banner | `data-testid` | `"github-auth-banner"` |
| Picker root | `data-testid` | `"github-issue-picker"` |

---

## User-Facing Strings

### Inputs

| Input | Placeholder | Status |
|-------|-------------|--------|
| Repo search | **UNSPECIFIED — must choose** | See Open Question §1 |
| Issue search | **UNSPECIFIED — must choose** | Suggest: "Search issues in {owner}/{repo}..." |

### Empty States

The plan specifies two issue empty states. Three additional states are required (see Open Question §4).

| Condition | String | CTA |
|-----------|--------|-----|
| No open issues (`issueStateFilter = "open"`) | "No open issues." | [Show all] |
| Label filter active, no match | `No issues matching label "{labelFilter}".` | [Clear] |
| `issueStateFilter = "all"`, zero issues | **MISSING — needs string** | Suggest: "No issues in {owner}/{repo}." with no CTA |
| `issueStateFilter = "closed"`, zero closed | **MISSING — needs string** | Suggest: "No closed issues." with [Show open] |
| No local repos | "No local repos found. Search GitHub to find a repo." | — |
| No GitHub repos match search | `No repos matching "{query}".` | — |

### Auth Error Banner

Current plan text: "GitHub CLI not authenticated. Run `gh auth login` in a terminal."

The backend uses the Go HTTP client, not the gh CLI subprocess. The phrase "GitHub CLI not authenticated" is technically inaccurate (the CLI is not called; the token is missing). However, `gh auth login` is still the correct remediation command because it writes the token that the Go client reads.

Recommended text: "GitHub not authenticated. Run `gh auth login` in a terminal to connect your account."

[Try again] button re-calls `reloadRepos()`.

### Direct Import Affordance

Format: `Import #N from owner/repo directly →`

Appears when the repo input value matches `issueURLPattern`. Pressing Enter or clicking calls `onImport(url)` directly.

### RepoChip

Format: `owner/repo ×`

The chip sits above the issue search input, not inside it. The × button uses `onMouseDown + e.preventDefault()` to avoid the onBlur race. This is correctly specified in Task 3.2.3 Step 1.

---

## Research Decisions — Adoption Audit

| Decision from research/ux.md | Adopted in plan? | Notes |
|------------------------------|-----------------|-------|
| Two-step with implicit state; no "Step 1/Step 2" labels | YES | RepoChip above issue input acts as breadcrumb |
| Show open issues immediately on repo selection | YES | `isIssuesLoading: true` set before debounce fires |
| First Escape → repo-selection; second → close modal | YES | Task 3.2.3 Step 3; `e.stopPropagation()` included |
| No confirm step after issue selection | YES | `onImport(issue.url)` called directly |
| Skeleton list over spinner | YES | 3 rows (repos), 5 rows (issues); count discrepancy not explained |
| Last-used repo persisted in localStorage | YES | `lastRepoCacheKey()`, restored on mount, used to pre-populate |
| GitHub URL detection → direct import | YES | `issueURLPattern` regex; "Direct import" affordance |
| `onMouseDown + e.preventDefault()` on all options | YES | Both repo options and RepoChip × use this pattern |
| RepoChip placed above issue input (not inside) | YES | Story 3.2 AC explicitly states "above the issue search input" |
| Open dot `#2da44e`, closed `#8250df` | YES | CSS custom properties on container |
| Dropdown opens on focus | NOT SPECIFIED | Plan does not say when `showList` becomes `true`; see Open Question §7 |
| Tab closes dropdown; focus to Cancel button | NOT SPECIFIED | No Tab handler in any onKeyDown spec; see Open Question §8 |
| Distinguish "filtered out" vs. "no results" | PARTIAL | Two empty states exist; three are missing |
| "Recently used" indicator on pre-populated repo | NOT SPECIFIED | Research §2 calls for a subtle indicator; plan omits this |

---

## Open Questions — Implementer Must Decide

### §1 — Placeholder text (HIGH — discoverability of URL paste)

Neither input has a placeholder string in the plan. The repo input placeholder is the only visible hint that URL paste is supported.

- Option A (recommended): "Search repos or paste a GitHub issue URL..." — surfaces the escape hatch
- Option B: "Search repos..." — cleaner, hides URL shortcut

The URL paste shortcut is called out in both research and pitfalls as critical for workflow continuity. Option A is strongly recommended. The issue search input should say "Search issues in {owner}/{repo}..." with the actual owner/repo interpolated from `selectedRepo`.

### §2 — Two-level Escape: missing visual affordance (HIGH — learnability)

The keyboard logic in Task 3.2.3 is correct but there is no UI signal that pressing Escape in issue-search phase returns to repo selection rather than closing the modal. A user who navigates to issue-search and then wants to close the modal will press Escape and be surprised to land back on repo selection.

Minimum viable affordance: add a small status line below the issue list — e.g., a `<p>` or `<span>` with text "↵ Select · Esc Go back" styled in muted secondary color. This pattern appears in VS Code's quick open palette and in the existing `QuickOpenPalette.tsx`.

Without this, the two-level Escape behavior is a hidden affordance that will confuse first-time users.

### §3 — Auth error message copy (MEDIUM — accuracy)

"GitHub CLI not authenticated" implies the `gh` CLI subprocess failed. The new RPCs use the Go HTTP client directly. The phrasing should not reference the CLI as the failing component.

Recommended: "GitHub not authenticated. Run `gh auth login` in a terminal to connect your account."

If the user has set `GITHUB_TOKEN` directly rather than using `gh auth login`, this message is also accurate — `gh auth login` is the standard path and the token source is an implementation detail the user need not know.

### §4 — Missing empty states for "all" and "closed" filter states (MEDIUM — trust)

Task 3.2.2 Step 5 only handles `issueStateFilter === "open"`. When the user clicks "All" or "Closed" and still sees no results, the plan renders nothing — a blank list with no message. Research §6 identifies this as an emotional JTBD failure: the user cannot distinguish "empty because filtered" from "fetch failure" from "truly empty repo."

Required additions to Task 3.2.2 Step 5:
- When `issueStateFilter === "all"` and `filteredIssues.length === 0`: render "No issues in {owner}/{repo}."
- When `issueStateFilter === "closed"` and `filteredIssues.length === 0`: render "No closed issues." with a [Show open] button

### §5 — `aria-autocomplete="list"` missing from issue input spec (MEDIUM — accessibility)

Story 3.2 acceptance criteria lists four ARIA attributes for the issue input (`role="combobox"`, `aria-expanded`, `aria-controls`, `aria-activedescendant`) but omits `aria-autocomplete="list"`. Task 3.2.1 adds it to the repo input but Task 3.2.2 has no equivalent step for the issue input.

Without `aria-autocomplete="list"`, screen readers do not announce that the list filters as the user types. The Story 3.2 AC and Task 3.2.2 must be updated to include it.

`aria-selected` on issue option `<li>` elements is implied by "keyboard nav same as RepoSelector" in Task 3.2.2 Step 3 but is not stated explicitly. The task steps must state it.

### §6 — Skeleton row counts (LOW — consistency)

Task 3.2.1 shows 3 skeleton rows for the repo list; Task 3.2.2 shows 5 for the issue list. If the intent is that the issue list typically contains more items than the repo list (plausible), the difference is correct and should be documented with a code comment. If unintentional, unify at 5 — the issue list is the more prominent loading surface and 5 rows better sets density expectations.

### §7 — Dropdown open trigger (LOW — perceived responsiveness)

The plan does not specify what sets `showList = true`. Research §2 recommends opening on focus (not first keystroke) so local repos appear the moment the user lands on the input. The implementer must choose between: on focus (recommended), on first keystroke, or on input click.

### §8 — Tab key behavior (LOW — keyboard completeness)

Research §3 specifies Tab should close the dropdown and move focus to the Cancel button. No Tab handler appears in any `onKeyDown` spec in the plan. The default browser Tab behavior (native focus traversal) will move focus off the input and close the dropdown implicitly if the dropdown is conditional on `document.activeElement`. This may be acceptable, but it should be tested explicitly, and the behavior should be documented in the RTL tests.

### §9 — "Recently used" label on pre-populated repo (LOW — clarity)

Research §2 says: "Show a subtle indicator like 'Recently used' above the pre-populated repo name." The plan pre-populates `selectedRepo` from `lastUsedRepo` on mount (Task 3.2.3 Step 6) but shows no indicator explaining why the repo was pre-selected. Without it, the user may not understand that the picker remembered their previous choice.

Minimum: a `<span>` above or beside the RepoChip reading "Last used" in muted secondary color when the repo was restored from `lastUsedRepo` (not manually selected in this session).
