# UX Research: GitHub Issue Picker

**Date**: 2026-07-03
**Feature**: Replace raw URL input in GitHub import modal with smart issue picker
**Audience**: Solo developer (Tyler) importing GitHub issues into stapler-squad backlog

---

## 1. Comparable UX Patterns

### GitHub's Own Issue Search UI

GitHub's native issue search operates as a **single unified search bar** at the top of the issues list, with filter facets appearing as chip rows below it. The experience has these characteristics:

- Search triggers on keystroke with a ~400ms debounce
- Filter qualifiers (`is:open`, `label:bug`, `assignee:me`) are typed inline, mixed with keywords — power-user-friendly but opaque to casual users
- The empty state on first open shows "All open issues" immediately — no blank slate
- GitHub's own repo picker (when filing issues from another context) uses a two-step flow: type to search repos, then select

**What feels good**: The "show results immediately" default. When you open the issue list you see something useful right away.
**What creates friction**: The qualifier syntax is invisible until you know it exists. Typing `label:` won't suggest valid labels — you have to know them. The filter UI (dropdowns for Label, Milestone, Assignee) and the keyword search are two separate surfaces that don't compose cleanly.

**Applicable lesson for this feature**: Show open issues immediately when a repo is selected — don't wait for the user to type a query. The most common case is "I want to pick from the open issues in this repo right now."

---

### Linear's Issue Picker

Linear's issue picker (used in "Relates to", "Parent", and integration search contexts) is the gold standard for this interaction pattern:

- **Unified combobox**: A single input renders both the repo picker and issue search. When no repo is active, typing searches across all projects. Once a project context is established, typing filters within it.
- **No two-step modal**: The transition from project scope to issue scope is invisible — context is maintained in the input, not in a stepped wizard
- **Instant local results**: Linear pre-fetches the user's issue list; results appear without any perceived latency
- **Keyboard-first**: Arrow keys + Enter is the primary interaction; mouse is secondary. The highlighted item is always visible (auto-scroll-into-view)
- **Rich item rows**: Each issue row shows: icon (status color), issue identifier (LIN-123), truncated title, assignee avatar. Dense but scannable
- **Grouping**: Results are grouped by project/team with a small header — useful when results span contexts

**What feels good**: The picker feels like an extension of the keyboard. You never context-switch to the mouse. The instant results make it feel like the app already knows what you want.
**What creates friction**: If the search returns 0 results, the empty state is just "No results" with no guidance on how to broaden. Linear also hides closed issues by default, which is correct but not discoverable.

**Applicable lesson**: Do not build a two-step wizard with "step 1" and "step 2" labels. Instead, represent the state implicitly: show a repo breadcrumb above the search input when a repo is selected, with an "×" to clear back to repo selection.

---

### VS Code's Quick Pick Palette (Cmd+P / Cmd+Shift+P)

VS Code's quick pick is the canonical combobox pattern for developer tools:

- **Opens immediately to a result list**: No blank input + empty dropdown. The first render shows recent items (recently opened files, recently used commands)
- **Input focuses automatically**: The cursor is in the text box the moment the palette opens — no click required
- **Virtualized list**: Items render in a fixed-height scrollable container; only visible rows are in the DOM
- **Active item tracking**: The highlighted item gets a distinct background color AND a `>` chevron indicator. The status bar shows which item is active even when scrolling
- **Escape hierarchy**: First Escape clears the current search query (if any); second Escape closes the palette. This "clear before close" pattern is important when the user has typed something
- **Tab does NOT navigate the list**: Tab closes the palette and focuses the next element in the DOM. Arrow keys are the only in-list navigation mechanism

**What feels good**: The two-level Escape behavior. "Escape cancels my search" is different from "Escape closes the picker" and VS Code handles both in one key.
**What creates friction**: The palette is modal — it captures all keyboard events, which can surprise users who expect to tab away. VS Code's choice is defensible because the palette always opens via keyboard shortcut, so users in the keyboard flow expect it.

**Applicable lesson**: The Escape key must handle the two-level case for this feature specifically — when the user has navigated to the issue list (step 2), Escape should go back to repo selection (step 1), not close the entire modal. This is both expected behavior and a safe fallback.

---

### Jira's Issue Picker in Integrations

Jira's issue linking picker (used in "Link issue" dialogs) has a subtly different problem: the result set is huge and heterogeneous (multiple projects, work item types). Its approach:

- **Project scoping is explicit**: A dropdown selects the project first; the search input then operates within that project
- **Results are paginated**: Shows 10 results at a time with a "Load more" link — not infinite scroll, not virtualized
- **Debounce at 500ms**: Noticeably slower than Linear's. Users feel the delay on every keystroke
- **Issue type shown as icon prefix**: Bug, Story, Task icons differentiate items visually before the title

**What feels good**: The project scope dropdown prevents cognitive overload when you have 50+ projects.
**What creates friction**: The explicit two-step flow (dropdown → input) means two pointer interactions before you see any results. The 500ms debounce makes the UI feel sluggish. "Load more" is jarring in a picker context — users expect a list, not pagination buttons.

**Applicable lesson for this feature**: With a solo developer who has a handful of active repos, Jira's heavy scoping UI is overkill. The tiered approach (local repos first, then GitHub search) handles the common case without requiring explicit project selection steps.

---

## 2. Interaction Flow Design

### Two-step vs. Unified Search

**Recommendation: Two-step with implicit visual state, not a wizard.**

The two-step flow (select repo → search issues) is correct for this feature for three reasons:

1. **The search space is 2D**: repos × issues. A unified search that returns both simultaneously creates ambiguous results — is "rails" a repo match or an issue title match?
2. **The common case is known-repo**: The user almost always knows which repo the issue is in. Local repos (from sessions/worktrees) should appear immediately, making step 1 < 2 seconds in the common case
3. **Issues from different repos are not comparable**: A single results list mixing issues from repo A and repo B creates "which repo is this from?" confusion for every row

**Implementation**: Do not show "Step 1" / "Step 2" labels. Instead:

- Initially: show the search input with placeholder "Search repos..." and a list of local repos below it
- After repo selected: the input clears, the placeholder changes to "Search issues in owner/repo...", a breadcrumb chip ("owner/repo ×") appears above or beside the input, and issues load immediately
- The chip acts as a visual anchor and a back button. Clicking × (or pressing Escape) returns to repo selection

This pattern is identical to how Linear handles project → issue scoping and how Gmail handles "From: [contact] ×" search scoping.

### Dropdown Open Trigger

Open the dropdown on focus. Rationale:
- Users landing on the GitHub import tab will naturally focus the input
- "Show something useful on focus" is the VS Code pattern — recent items, local repos
- Opening on first keystroke creates a jarring experience where the UI jumps after the first character is typed

Do NOT auto-open if the input has an existing value (e.g., on re-open after a previous session). In that case, treat it as a re-search.

### "Recently Used Repo" Memory

Persist the last-selected repo in `localStorage` under a key like `stapler-squad:github-issue-picker:last-repo`. On modal open in GitHub import mode, pre-populate the repo selection with the last-used repo and immediately fetch open issues. This is the most common flow: the user imports multiple issues from the same repo in a single session.

Show a subtle indicator like "Recently used" above the pre-populated repo name so the user knows where it came from and can change it if needed.

### After Issue Selection: Immediate Import vs. Confirm Step

**Recommendation: No confirm step.** Import immediately on issue selection. Rationale:
- The user searched for a specific issue — they know what they're selecting
- A confirm step ("You selected issue #123: Foo Bar. Import?") adds a click with no protective value. The user can immediately edit or delete the imported item if it was wrong
- This mirrors Linear's and GitHub's own patterns — selecting an item in a picker IS the commit action

---

## 3. Keyboard Navigation Requirements (WCAG + Practical)

### ARIA Pattern: Combobox with Listbox

The correct ARIA pattern is `role="combobox"` on the input, `aria-expanded` to indicate list state, `aria-controls` pointing to the listbox ID, and `aria-activedescendant` tracking the focused option ID. The list gets `role="listbox"`, each item gets `role="option"` with `aria-selected`.

```
<input
  role="combobox"
  aria-expanded="true"
  aria-controls="issue-picker-listbox"
  aria-activedescendant="issue-option-42"
  aria-autocomplete="list"
/>
<ul
  id="issue-picker-listbox"
  role="listbox"
>
  <li id="issue-option-42" role="option" aria-selected="true">...</li>
</ul>
```

This matches the existing `AutocompleteInput.tsx` pattern in the codebase (which uses `role="listbox"` + `role="option"` correctly) but that component is missing `role="combobox"` on the input and `aria-activedescendant`. The new component should add those.

### Key Bindings

| Key | Behavior |
|-----|----------|
| `ArrowDown` | Move highlight to next item; wrap to first if at end |
| `ArrowUp` | Move highlight to previous item; move to input if at first item (not wrap) |
| `Enter` | Select highlighted item; if no item highlighted, submit current query |
| `Escape` (first press, during issue search) | Return to repo selection step; clear issue results |
| `Escape` (first press, during repo selection OR no text) | Close the modal (existing behavior) |
| `Tab` | Close the dropdown without selecting; focus moves to next focusable element in modal (Cancel button) |
| Type any character | Clears active descendant; filters list client-side or triggers debounced search |

This two-level Escape mirrors VS Code's palette behavior and is critical for step-2 usability.

### Focus Management

- On modal open: focus the search input immediately (`autoFocus` attribute or `inputRef.current?.focus()` in `useEffect`)
- On item selection: focus returns to the modal's primary action (Import button) or immediately triggers import
- On Escape (going back to repo step): focus returns to the repo search input
- On modal close: focus returns to the trigger element (the "Import from GitHub Issue" tab button)

The existing `VaguenessPromptModal.tsx` handles focus-on-open correctly via `autoFocus`. Follow the same pattern.

---

## 4. Visual Design Considerations

### Information Density per Issue Row

Show at minimum:
- Issue number (e.g., `#123`) — in muted secondary color
- Issue title — primary text, truncated with ellipsis at ~60 chars
- Issue state indicator — a small colored dot or pill: green for open, purple for closed (matching GitHub's colors exactly; users have strong color memory for this)

Show when space permits:
- Label chips — up to 3, truncated if more
- Assignee avatar — 16px circle, rightmost column

The issue number + title combination is sufficient for recognition. Labels and assignee are "nice to have" that reduce the need to click through to verify. Given the modal is ~480px wide, one row should be ~40px tall (icon 16px + 2×12px padding + text 16px).

### Color Coding for Issue State

Use GitHub's exact color palette to leverage user familiarity:
- Open: `#2da44e` (green) — GitHub's open issue color
- Closed as completed: `#8250df` (purple) — GitHub's closed-completed color
- Closed as not planned: `#6e7781` (gray) — GitHub's closed-not-planned color

Map to local CSS custom properties to avoid hardcoding hex in the component:
```css
--github-issue-open: #2da44e;
--github-issue-closed: #8250df;
--github-issue-not-planned: #6e7781;
```

### Loading States

Use a **skeleton list** rather than a spinner. Rationale:
- Skeleton list communicates "items are coming" and their approximate density — it sets expectations
- A centered spinner communicates "something is happening" but feels empty
- The `TriageLoadingIndicator.tsx` in this codebase already uses a skeleton pattern; reuse the same approach

For debounced issue search (user is typing): show skeleton after 300ms if results haven't arrived. This avoids skeleton flash on fast network responses.

For the initial repo list (local data, should be < 100ms): show nothing extra — the list should appear fast enough that no loading indicator is needed. If it takes > 200ms, show a spinner at the bottom of the list.

### Empty States

| Situation | Empty State Message |
|-----------|---------------------|
| No local repos found (no sessions/worktrees) | "No local repos found. Search GitHub to find a repo." — with the search input focused and ready |
| No GitHub repos match search | "No repos matching '[query]'" — suggest checking the spelling or searching a shorter term |
| No issues in selected repo | "No open issues in [owner/repo]" — with a filter chip option to show closed issues |
| No issues match search/filter | "No issues matching '[query]'" — show a "Clear filters" link |
| GitHub not authenticated | Inline auth prompt (see Error States section) |

---

## 5. Error States and Edge Cases

### GitHub Auth Not Configured

Check auth status on modal open (or on the first GitHub repo search). If `gh auth status` returns an error:

- Do NOT show a broken empty state or a cryptic error message
- Show a dedicated auth instruction panel within the picker, replacing the results list:
  ```
  [GitHub logo icon]
  GitHub account not connected
  Run `gh auth login` in a terminal to connect your account.
  [Try again button]
  ```
- The "Try again" button re-checks auth and transitions to the normal search flow if it passes
- This mirrors how `GitHubPRsSection.tsx` handles the auth flow — refer to that component's `DeviceAuthStatus` flow for implementation reference

### Search Timeout or Network Error

After 2 × the debounce window (600ms), if no response:
- Show an inline error below the input: "Search timed out. [Retry]"
- The retry button re-triggers the last query
- Do not clear what the user typed

### User Pastes a GitHub URL into the Search Input

The existing codebase has a detector pattern for this (see `web-app/src/lib/omnibar/detector.ts` — `GitHubPRDetector` handles `https://github.com/.../pull/N`). The same approach should apply here:

- If the user types/pastes `https://github.com/owner/repo/issues/N` into either the repo search or issue search input, detect it immediately and import directly without requiring repo selection
- Show a visual affordance: replace the input content with a chip like "Detected: owner/repo #N" and an "Import now" button
- This is a graceful "escape hatch" that preserves the old workflow for users who already have a URL copied

### User Selects a Closed Issue

Allow it without friction. Some workflows intentionally import closed issues (e.g., re-opening a deferred item). Do not warn or block. The issue state (open/closed) should be visible in the row via the color dot, giving the user enough information to make the right choice.

---

## 6. Job-to-Be-Done Analysis

### Functional Job: Quickly find and import a GitHub issue I know exists

The user's mental model before opening the picker:
- "I have an issue in mind. I know roughly what repo it's in. I don't remember the number."
- OR: "I want to pull in one of the open issues from this repo I'm actively working on."

The picker succeeds functionally if:
1. The user's active repo appears in the list without any searching
2. Typing 3–5 characters of the issue title narrows to 1–3 results
3. Pressing Enter imports without additional clicks

The current URL-paste flow fails the functional job because it requires the user to already know the issue number — which means they had to look it up somewhere else.

### Emotional Job: Feel in control and trust I'm not missing relevant issues

Two failure modes for trust:
1. **False confidence**: The picker shows a subset of issues and the user doesn't know it's a subset (e.g., only showing open issues but not indicating that closed ones exist). Fix: always show the current filter state and offer a "Show closed" toggle prominently.
2. **Uncertain completeness**: The user searches and gets 0 results but doesn't know if that's because there are truly no issues or because the search is broken. Fix: distinguish "empty because filtered" (show "No open issues — show all") from "empty because no results" (show "No issues matching X").

The emotional job is satisfied when the user can answer "Am I seeing all relevant issues?" without clicking around to verify.

### Social Job

None — this is a single-user tool. No sharing, no visibility to others.

---

## 7. Existing Codebase Patterns to Reuse

### AutocompleteInput (`web-app/src/components/ui/AutocompleteInput.tsx`)

The foundational keyboard navigation pattern is already implemented here: ArrowDown/ArrowUp with clamping, Enter to select, Escape to close, Tab to dismiss. The new component should extend this pattern, not rewrite it. Key gap: `AutocompleteInput` doesn't support:
- Grouped results (local repos vs. GitHub repos)
- Rich item rows (just plain text strings)
- Async loading state per-query (isLoading is a simple bool, not per-request)

Extend rather than replace: `GitHubIssuePicker` should be a new component built on the same keyboard handling idioms but with richer rendering.

### QuickOpenPalette (`web-app/src/components/sessions/QuickOpenPalette.tsx`)

This is the closest structural analog — a modal palette with async search, recent items on open, and keyboard navigation. Key patterns to borrow:
- Debounced search with `useEffect` watching the query value
- `activeIndex` state with scroll-into-view behavior
- `createPortal` for overlay rendering

### Modal Structure (`backlog/page.tsx` lines 515–641)

The existing modal uses `role="dialog"`, `aria-modal="true"`, and click-outside-to-close. The new picker replaces only the content within the `formMode === "github"` branch — the outer modal shell stays unchanged. This means the picker inherits the modal's ~480px width and needs to fit within it, including the header and Cancel/Import buttons.

Maximum available height for the picker content: the modal has no explicit max-height, but anything beyond ~70vh will require internal scrolling within the list. The list should have its own `overflow-y: auto` container with a fixed max-height (e.g., 240px for ~6 items at 40px each).

---

## 8. Key Decisions and Recommendations

| Decision | Recommendation | Rationale |
|----------|----------------|-----------|
| Flow shape | Two-step with implicit state (repo breadcrumb chip) | Avoids ambiguous mixed results while feeling seamless |
| When to show issues | Immediately on repo selection, no search required | Most common case is "browse open issues" not "search for specific issue" |
| Escape behavior | First Escape goes back to repo step; second Escape closes modal | Mirrors VS Code; prevents accidental modal close |
| After issue select | Import immediately, no confirm | Selection IS the commit in picker UIs |
| Loading pattern | Skeleton list after 300ms | Sets density expectations; avoids spinner emptiness |
| Last-used repo | Persist in localStorage, pre-populate on next open | Eliminates the most common repeat interaction |
| GitHub URL detection | Detect and short-circuit to direct import | Preserves old workflow as an escape hatch |
| Item density | Number + title + state dot required; labels optional | Scannable without being overwhelming in 480px modal |
| Empty state | Distinguish "no results" from "filtered out" | Critical for user trust (emotional JTBD) |
