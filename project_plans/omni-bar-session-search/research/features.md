# Research: Features

Dimension: Features
Researcher: agent
Date: 2026-04-14
Status: Complete

---

## VS Code Command Palette Patterns

VS Code uses two keyboard shortcuts that solve the same problem this project faces: one input, two intents.

**Cmd+P (Quick Open / File Picker)**
- Opens with empty state showing recent files ordered by recency, with the most recently opened at top.
- Typing filters the list fuzzy-first: the query "myfeat" matches "my-feature.ts" because VS Code uses a consecutive-character-bonus algorithm (contiguous runs score higher than scattered matches).
- Result items are homogeneous: all file paths, no visual type distinction needed within the list. The icon (file vs folder) provides the only type signal.
- Tab accepts the highlighted item and continues completion (directory drill-down).
- Enter accepts and opens.
- The placeholder text reads "Search files by name" — a single clear intent.

**Cmd+Shift+P (Command Palette)**
- Empty state shows recently-used commands, not all commands, with usage frequency as the secondary sort.
- Prefix characters trigger mode-switching: typing `>` puts you in command mode (the default for Cmd+Shift+P), `@` switches to symbol mode, `:` switches to line-number mode, `#` switches to workspace symbol mode. Critically, these prefixes work in the *same* input — you do not need a different shortcut.
- Result items show a category label (e.g., "File", "Edit", "View") in a dimmed secondary column on the right. The primary text is the command name. This asymmetric layout (bold left, dimmed right) is the VS Code signal for "primary intent / secondary context."
- When multiple result types are mixed (commands + files + symbols), VS Code groups them with horizontal section headers ("recently opened", "other files") rather than interleaving.

**Key Pattern: Sigil-Based Mode Switching**
VS Code deliberately conflated Cmd+P and Cmd+Shift+P at the input layer. Typing `>` in the file picker switches to command mode. This is the canonical solution to the "single input, multiple intents" problem. The user does not need to remember different shortcuts; the input itself is the mode controller.

**Recommendation for this app:** Do not implement sigil-based mode switching. Stapler Squad has only two modes (navigate existing vs create new), and sigil complexity adds friction. VS Code's sigil system works because it has 8+ intent modes. For two modes, visual grouping in results is sufficient.

**Empty State Recommendation:**
VS Code shows recent files immediately when input is empty — zero characters yields a useful list. This is the right model. "Nothing until they type" is a missed opportunity; the user opened the palette with intent, and showing them their most likely options immediately is faster than requiring a query.

---

## Raycast / Alfred Patterns

Raycast is the clearest reference for the navigate-existing + create-new problem in a single input.

**How Raycast Blends Navigation and Creation**

Raycast treats every result item as a potential pivot point. When you type "Slack" and the result list shows the Slack application, you can:
- Press Enter to open Slack (navigate to existing)
- Press Tab or a secondary shortcut to see "actions" for that result (e.g., "New Message in Slack", "Quit Slack", "Open in Background")

The secondary action system is the crucial pattern: the same result item can have a primary action (navigate) and secondary actions (create/modify). This is displayed as a small kbd shortcut hint at the right side of the highlighted row ("Tab for actions" or "Cmd+K for more actions").

**Section Headers vs Interleaved Results**

Raycast uses section headers when results come from distinct sources. For example, if you type "Notion" and get both an application result and Raycast extension commands, they appear under separate labeled sections:

```
APPLICATIONS
  Notion  ·  Open

RAYCAST COMMANDS
  Search Notion pages
  Create Notion page
```

The section header is visually light (all-caps, muted color, small font) and does not receive keyboard focus. Arrow keys skip over headers to items.

When results are homogeneous (all from the same source), no headers appear.

**Result Type Signaling**

Raycast signals result type through:
1. Icon on the left (app icon, folder icon, document icon, command icon)
2. Section header above the group
3. A subtle text annotation on the right showing the source ("Notion", "Browser History", "File System")

The primary text (result name) is never diluted with type information inline — type is always a secondary visual element.

**Empty State**

Raycast shows "Pinned items" and "Recently used" sections when input is empty. The distinction matters: pinned items are intentional persistent shortcuts, recently used items reflect actual behavior. Most importantly, this means the palette is immediately useful — opening Raycast is never a blank screen experience.

**Alfred Patterns**

Alfred is more spartan than Raycast but shares the same structural insight: result rows are uniform in height, type is signaled via icon, and secondary actions are revealed on a secondary keypress (Cmd+L in Alfred reveals "large type", Cmd+Y reveals quick look). Alfred explicitly separates "file results" from "application results" from "workflow results" using section headers when results mix types.

Alfred's fallback actions are relevant here: when no results match, Alfred shows "Search Google for X", "Search Amazon for X" — explicit creation/external-action affordances that surface when the navigation intent fails. This is directly analogous to: when no session matches the query, show "Create new session in /path/X."

---

## Linear / Notion Patterns

**Linear Cmd+K**

Linear's command menu is the closest analog to this project's needs because it explicitly blends navigation and creation in one input, across issue entities that have both a "go to" and a "create" action.

Empty state shows two sections:
1. "Recent" — the last 5-8 issues you visited, shown with their identifier (LIN-1234), title, and status icon
2. "Suggested actions" — context-sensitive shortcuts like "Create issue in current project"

When you type:
- Fuzzy matching runs against issue title, identifier, and assignee simultaneously
- Results show the issue identifier in a fixed-width monospace pill on the left, the title as primary text, and the team/project as secondary muted text
- Status is shown as a colored circle icon on the left of the identifier

**Navigate vs Create Signal in Linear**

Linear does not use a prefix sigil. Instead, it uses result position to signal intent: navigation results (existing issues) always appear first, and at the bottom of the list appears a persistent "Create issue..." entry styled differently — it has a `+` icon and slightly different background treatment. This is always present regardless of what the user types.

This is the most important pattern to borrow: a persistent "create" action at the bottom of the result list, styled to distinguish it from navigation results. The user can scroll past all navigation results to reach the creation action, or they can use the keyboard shortcut shown in the footer.

**Mode Signaling Without Sigils**

Linear uses visual hierarchy, not sigils:
- Navigation results: icon on left, title bold, metadata muted on right
- Create action: `+` icon, "Create new issue" text, full-width distinct background

This is clean and requires no learned syntax.

**Notion Cmd+K**

Notion's quick-find blends page navigation (go to existing) with page creation more naturally than Linear. The key Notion pattern is **progressive disclosure of action**: when you select a page result, you see "Open", "Open in side peek", "Copy link" — the first action is navigation, but others are available. Notion does not offer inline creation from the quick-find; creation is a separate action.

For Stapler Squad's purposes, Notion's pattern is less relevant because Stapler Squad needs both navigation and creation as first-class peers.

**The Linear Model is the Right Baseline**

The Stapler Squad omnibar should follow the Linear model:
- Empty state: recent sessions (navigate) + recent repos (create)
- Typing: fuzzy session results first, "create new session" entry persistent at bottom
- Result type is visually distinguished (icon + background, not sigils)

---

## Current Omnibar Analysis

### What exists today

The current `Omnibar.tsx` is a **session creation wizard only**. It has:

1. **A single text input** that detects input type via `detect()` from `/lib/omnibar/detector.ts`. The detector chain handles: GitHub PR URLs, GitHub branch URLs, GitHub repo URLs, GitHub shorthand (owner/repo), path with branch (path@branch syntax), and local paths.

2. **Path completion dropdown** (`PathCompletionDropdown.tsx`) that appears when the input is detected as a local path. It merges history entries (from `usePathHistory`) with live filesystem completions (from `usePathCompletions`). History entries appear first with a clock icon and a divider separating them from live entries.

3. **A form body** with session name, session type (new worktree / existing worktree / directory), branch controls, working directory, and advanced options (program, category, auto-yes). This is a multi-field form that appears below the input.

4. **An icon + label detection badge** that shows the detected input type (e.g., "Local Path", "GitHub PR") with emoji icons.

5. **A `usePathHistory` hook** stored in localStorage at `omnibar:path-history` with up to 50 entries. The hook tracks path, count, and lastUsed timestamp. Scoring is `recencyScore + log1p(count)`. It exposes `getMatching(prefix)` but **not** a method to get top items without a prefix.

6. **A `useRepositorySuggestions` hook** that fetches all sessions via `listSessions` RPC and ranks paths by frecency. This hook exists and is ready — it is not yet wired into the Omnibar UI.

### What is missing (critical gaps)

1. **No session search or navigation**. The Omnibar has no way to show existing sessions as results. There is no search box mode, no session result type, no "select session to navigate" action. The omnibar is purely a creation tool.

2. **The creation form appears immediately** for all non-empty input. There is no intermediate "here are things you might want" result list before the user is committed to creation. This makes the omnibar creation-first, with no discovery phase.

3. **History entries are prefix-matched only**. `getMatching(prefix)` requires a matching prefix. There is no "show top N most-used repos without prefix" capability — meaning the empty state shows nothing history-related.

4. **No session-type result rendering**. There is no component that renders a session as a selectable result item with status badge, branch, path, etc.

5. **The path completion dropdown and the form are separate layers**. The dropdown is a transient overlay over the form. There is no unified result list that could show both session results and repo/path results in sections.

6. **`useRepositorySuggestions` exists but is unused in Omnibar**. The hook fetches and ranks paths by frecency from existing sessions — this is the "recent repos" data source. It just needs to be surfaced in the UI.

### What modes currently exist

- **Path mode**: Input is a local path or path+branch → path completion dropdown activates, form shows worktree options
- **GitHub mode**: Input is a GitHub URL or shorthand → form shows clone-related fields
- **Unknown mode**: Input doesn't match any detector → badge shows "Unknown", form cannot be submitted

There is no "search mode" or "navigate mode." Adding session search adds a third mode that activates when input is non-path, non-GitHub text (or when the omnibar opens with empty input).

### What UI affordances exist

- Modal overlay (dark backdrop, centered card)
- Type indicator icon at left of input
- Path existence indicator (✓/✗/⟳) at right of input
- Path completion dropdown (listbox, keyboard navigable, history section + live section with divider)
- Keyboard shortcuts footer (Esc, Cmd+Enter, arrow keys, Tab when dropdown is visible)
- Error message area above footer

---

## Recent Items Patterns

### Empty state: show recents or show all?

Show recents. Never show nothing. An empty omnibar that presents a blank input is a missed UX opportunity. The user opened the palette because they want to do something — showing them their most likely options without requiring a query is strictly better.

The correct empty state:
- "Jump to session" section: top 5 most recently accessed sessions
- "Recent repos" section: top 5 most recently used repo paths (from `useRepositorySuggestions`)

5 items per section is the right limit. More than 5 creates scroll anxiety; fewer than 5 may not include the item the user wants. VS Code uses ~10 for file picker but 10 is too many for a two-section mixed display.

### Recency vs frequency weighting

`usePathHistory` already implements a combined recency+frequency score: `recencyScore(lastUsed) + log1p(count)`. This is the correct model. The `log1p` dampens the frequency contribution so a path used 100 times a month ago doesn't permanently outrank a path used twice yesterday.

For session recency: use `session.updatedAt` (last activity timestamp) as the primary signal, not `session.createdAt`. A session created 3 months ago but actively used yesterday is a recent session.

### How many recent items to show

Empty state: 5 sessions + 5 repos = 10 items total, in two labeled sections.
With query: max 8 total results (5 sessions + 3 repos, or all sessions if no repos match).

Do not show more than 8 results with a query active. More than 8 items in a command palette creates scroll anxiety and slows keyboard navigation.

### How to rank recents vs search results when user starts typing

Once the user types anything, switch from "recents mode" to "search mode." Do not blend recency scores with search scores — the contexts are different. In recency mode, the user has not expressed intent; in search mode, they have. A search result that scores 0.3 should rank above a recent item that was used yesterday.

Exception: if the query is very short (1-2 characters), recency should still boost results because the user may be using the first letter as a quick-filter rather than a deliberate search term. Implement this as: for queries of length <= 2, multiply the search score by (1 + 0.5 * recencyScore).

---

## Result Mixing: Sessions + Repos

### Should they be separate sections or interleaved?

Separate sections. The action on selection differs fundamentally:
- Clicking a session result: navigates to that session (closes omnibar, scrolls/activates session)
- Clicking a repo result: pre-fills the path input for new session creation (does not close omnibar, transitions to creation form)

These two action models cannot be interleaved without confusion. If a user presses Enter on the top result not knowing whether it's a session or a repo, they will get unexpected behavior 50% of the time. Sections with distinct headers make the action semantics scannable before committing.

### How to visually distinguish session results from repo results

**Session result row:**
```
[status dot] [session title] (bold)          [branch name] (muted)
             [repo path] (muted, smaller)     [Running/Paused badge]
```

- Status dot: colored circle (green = Running, yellow = Paused, grey = Stopped)
- Title: bold, left-aligned
- Branch: right-aligned, muted
- Path: second line, smaller, muted
- Status badge: right-aligned, colored text label

**Repo result row:**
```
[folder icon] [repo path] (bold last segment, muted parent path)    [last used] (muted)
              [N sessions] (muted)
```

- Folder icon distinguishes from session dot
- Path rendering: bold the last segment (repo name), muted for parent path
- Last-used timestamp (relative: "2 days ago")
- Session count ("3 sessions") signals this is an active repo

### Section headers

```
SESSIONS  (3 results)
[session rows...]

REPOS  (5 results)
[repo rows...]
```

Headers are all-caps, small font, muted color, non-interactive. Session count shown in header for orientation. No section collapser — the list is short enough (max 8 items) that collapsing adds friction without benefit.

### Keyboard behavior

Arrow keys navigate all results in sequence, skipping headers. Enter executes the primary action for the focused item. The action difference (navigate vs fill-form) means the user must understand what type of result is highlighted — which is why the visual distinction (dot vs folder icon, section header) is load-bearing for keyboard users.

### When to show each section

- **Omnibar opens, no input**: Show both sections (recents mode, 5+5)
- **User types a short query (1-3 chars)**: Show both sections, filtered
- **User types a longer query**: Show sessions section if any match; show repos section if any match; hide empty sections entirely (do not show "no results" placeholder for a section)
- **Input is detected as a local path (starts with /  or ~/)**: Hide session results entirely. Switch to path completion mode — the user is doing directory navigation, not session search. This is the current behavior and it's correct.
- **Input is detected as GitHub URL**: Hide both sections. User is cloning a remote repo. This is the current behavior and it's correct.

---

## Recommended UX Model

### The Model: Two-Phase Omnibar

The omnibar operates in two phases separated by an intentional user action (selecting a result vs typing a path).

**Phase 1: Discovery (the result list)**
The input is a query. Results are sessions and repos. The user navigates with arrow keys and selects with Enter.

- If the user selects a **session result**: omnibar closes, the selected session becomes active.
- If the user selects a **repo result**: the omnibar transitions to Phase 2, pre-filling the path input with the selected repo path.
- If the user's input is detected as a **local path** (starts with `/` or `~`): omnibar transitions directly to Phase 2 without showing a result list.

**Phase 2: Creation (the existing form)**
This is the current Omnibar.tsx creation form, unchanged except that the path field is pre-filled. The user fills in session name, session type, branch, etc., and submits with Cmd+Enter.

### Interaction Model in Detail

**On Cmd+K (open omnibar):**
1. Input is empty and focused.
2. Below the input: two sections visible immediately.
   - "SESSIONS" with the 5 most recently active sessions.
   - "REPOS" with the 5 most frecent repo paths.
3. Keyboard shortcut hint in footer: "↑↓ to navigate · Enter to jump · Tab to complete"

**User types characters (any non-path, non-GitHub text):**
1. Both sections update in real time with fuzzy-matched results.
2. Empty sections collapse (no "SESSIONS" header if no sessions match).
3. Minimum 1 character to trigger search (do not search on empty — show recents instead).
4. If no results in either section: show a single "Create session in `<query>`?" item at the bottom, styled like a "new item" affordance (+ icon, italic text). This prevents the dead-end of zero results.

**User types a path (starts with / or ~):**
1. Session results section disappears immediately.
2. The path completion dropdown activates (existing behavior).
3. REPOS section is hidden — user is in path navigation mode.

**User selects a session result (Enter or click):**
1. Omnibar closes.
2. The selected session becomes focused/highlighted in the session list.
3. If the app has a session detail view or terminal panel, it opens for that session.

**User selects a repo result (Enter or click):**
1. The input is pre-filled with the repo path.
2. The result list disappears.
3. The creation form body appears (Phase 2).
4. The session name field is auto-filled with the suggested name (existing behavior).
5. The user is now in creation mode.

**The "Create New" escape hatch:**
A persistent "+ New Session" item appears at the very bottom of the result list, below all sections and a visual separator. It is always present when the result list is showing. Pressing End or scrolling down reaches it. Its label adapts: "New session in /queried/path" if the query looks like a path, otherwise "New session". Selecting it transitions to Phase 2 with empty path field.

### What NOT to do

Do not change the creation form (Phase 2) at all. The form is correct — it collects the right information, and adding session navigation on top of it would make the component too large. The architectural split between Phase 1 (result list, new component) and Phase 2 (existing form, unchanged) is the right boundary.

Do not implement sigil-based mode switching (e.g., `>` for session search, `/` for path). Two intents do not need a sigil system. Visual grouping and input-type detection are sufficient.

Do not interleave session results and repo results by relevance score. The action difference (navigate vs fill-form) makes interleaving dangerous for keyboard users.

Do not show more than 8 results total. Command palette fatigue sets in past 8 items; users stop reading and start scrolling, which defeats the keyboard-first value proposition.

### Component Architecture Implication

The recommended model implies the following component decomposition:

1. **`OmnibarResultList` (new component)**: Renders session results + repo results in sections. Handles keyboard navigation within the list. Emits `onSessionSelect(session)` and `onRepoSelect(path)` events.

2. **`OmnibarSessionResult` (new component)**: Renders a single session result row with status dot, title, branch, path, status badge.

3. **`OmnibarRepoResult` (new component)**: Renders a single repo result row with folder icon, path (bold last segment), last-used, session count.

4. **`Omnibar.tsx` (modified)**: Gains a mode state: `'discovery' | 'creation'`. In `discovery` mode, renders `OmnibarResultList` below the input instead of the form body. In `creation` mode, renders the existing form body (current behavior, unchanged). Transitions from `discovery` to `creation` when a repo is selected or when path detection activates.

5. **Session search hook (new)**: `useSessionSearch(query)` — takes the current query, returns ranked session results. This is the client-side fuzzy search layer that this research does not cover in depth (that is the Stack and Architecture dimensions).

This two-phase, two-mode architecture satisfies the requirements:
- "reach any existing session in ≤3 keystrokes after Cmd+K" — open (1), type first letter (2), Enter (3)
- "start new session on any previously-used repo in ≤5 keystrokes" — open (1), arrow to repo (2-3), Enter (4), Cmd+Enter (5)
- The path completion and GitHub URL flows are unaffected (they are Phase 2 entry points)
