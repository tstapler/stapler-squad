# History Page Revamp: UX Research Findings

Research date: 2026-04-12

---

## 1. Metadata Density on Cards

### What comparables show

**Linear (issue list view)**
- Always-visible (first-class): issue title, status icon (colored dot), priority icon, assignee avatar, label chips, cycle/project membership indicator
- Visible on hover / in compact row: estimate, due date, updated-at timestamp
- Hidden behind detail panel: description body, comments, activity log, sub-issues
- Key principle: status and priority are communicated entirely through small color-coded icons so that the text title gets ~70% of the row width. Metadata never wraps — it truncates or hides.

**GitHub Issues / Projects (table + board card)**
- Always-visible: title, number (`#1234`), author, label chips, milestone, assignee avatar
- Visible on hover: last-updated relative timestamp ("3 hours ago")
- Hidden by default: full description, comment count summary (shown only as a count badge)
- With GitHub Issue Fields (in public preview as of March 2026): up to 25 custom structured fields can appear as columns in table view or as sidebar metadata in detail view

**JetBrains Welcome Screen (Recent Projects)**
- Always-visible: project name (large), full path (small, muted), custom icon or auto-derived letter-icon
- Visible on hover: "Open" button, context menu for removing from list
- Hidden: last-opened timestamp (not surfaced in the default list — a notable gap)
- Projects can be grouped into named folders for logical organization
- Search bar filters the list by name in real-time

**VS Code Workspace History (File > Open Recent)**
- Shows name + full path only — no timestamps, no branch, no status
- Very sparse — intentionally minimal, relies on alphabetic/recency sort

**Cursor AI Chat History**
- Always-visible: conversation title (auto-generated from first message), relative timestamp of last interaction
- Sort: by last-interaction time, not creation time — most recent at top
- Hidden: full message list (accessed by opening the conversation)
- Each session is a tab-like object; multiple can be open and run in parallel

### Patterns to adopt

| Field | Visibility recommendation |
|---|---|
| Session title / first message snippet | Always visible, dominant |
| Repository name + branch | Always visible, secondary line |
| Last-active timestamp (relative) | Always visible, trailing right |
| Session status (Running / Paused / Done) | Always visible, color-coded icon left of title |
| Working directory path | Always visible, muted below title |
| Total message count | Visible as small badge |
| Last 3-5 messages | Collapsed by default, expand on click |
| Session tags | Shown as chips, max 3 visible, "+N more" overflow |
| Full message history | Detail view only |

---

## 2. Fork / Clone UX Patterns

### GitHub Codespaces
- Entry point: "Code" button dropdown on a repo page — contains tabs "Local" and "Codespaces"; clicking "Create codespace on main" (or selecting a branch) opens a modal with machine type selection before launching
- When you commit from a codespace you don't own, Codespaces **automatically forks** the repo and repoints `origin` to the fork, `upstream` to the original — no explicit fork dialog; the operation is transparent and declared after the fact with a CLI prompt ("proceed with linking to fork? y/n")
- Branch selection for new codespace: inline dropdown in the "New codespace" modal — repo, branch, machine type, region all on one screen

### GitLens (VS Code extension)
- "Open on Remote" and "Create Branch" actions available in context menus on commit/branch nodes in the Source Control sidebar
- No dedicated fork flow — relies on GitHub PR workflow for forks

### JetBrains (IntelliJ/CLion "Get from VCS" flow)
- Welcome screen has a "Get from VCS" button that opens a full modal dialog: URL input, directory, VCS type — single page, not a wizard
- No "clone into new worktree" concept native to JetBrains; worktree support was added but is not the primary UX surface

### Patterns observed and their tradeoffs

| Pattern | Tools using it | Tradeoff |
|---|---|---|
| Modal dialog with environment options | Codespaces, JetBrains | Best for first-time / infrequent actions; can feel heavy for power users |
| Inline contextual dropdown on the item | GitHub branch picker | Fast for frequent actions; requires discoverability |
| Right-click context menu | JetBrains, VS Code explorer | Low discoverability but keeps primary UI clean |
| Automatic with transparent prompt | GitHub Codespaces auto-fork | Optimal when the action is obvious; risky if the user is surprised |

### Recommendation for our fork/resume action

Use an **inline split-button** pattern on the session card:
- Primary action (left): "Resume" — resumes into the existing worktree
- Secondary dropdown (right chevron): expands to show "New worktree", "Open in directory...", "Clone to..."

This matches the GitHub "Code" button pattern and keeps the happy path (resume) one click while still surfacing fork options without a context menu.

---

## 3. Inline Message / Content Preview

### Email clients (Gmail, Outlook, eM Client)
- Gmail conversation view: the inbox list shows **sender, subject, and a one-line snippet** of the latest message; clicking the row expands the full thread inline (accordion)
- Individual messages within a thread are collapsed to a single line (sender + date) unless they are the most-recent or were explicitly unread; clicking a collapsed message expands it
- Outlook "New Outlook" (2024+): partially-open conversation threads — the most recent message is pre-expanded, earlier messages collapsed; users can expand any message by clicking its header
- eM Client: offers both "threaded" (Gmail-style accordion) and "flat" (individual message) views — user preference toggles them

### Slack threads
- Thread preview in channel view: shows the first message + a "N replies" badge with the last replier avatars; clicking opens a side panel (not an inline expansion)
- This is a **lateral expansion** pattern (side drawer) rather than inline accordion

### GitHub PR comment threads
- Inline diff comments: collapsed to a single line ("N outdated comments") until clicked; expanding shows the full exchange
- PR conversation tab: all review comments shown in a scrollable flat list with context snippets — not collapsible per-comment

### Key interaction triggers observed

| Trigger | Effect | Tools |
|---|---|---|
| Click on row / card body | Expand inline (accordion) | Gmail, Outlook threads |
| Click on "N replies" badge | Open side drawer | Slack |
| Click on collapsed header | Expand that message only | Gmail thread items, Outlook |
| Hover | Show one-line snippet | Most email clients in list pane |
| Dedicated "expand" chevron icon | Toggle expansion | Many list UIs |

### Recommendation

- Show a **1-line snippet** of the last message always (no interaction required)
- Provide a **chevron / "Show N messages" button** below the snippet that expands an inline accordion showing the last 3–5 message pairs (user + assistant turns)
- Collapse by clicking the chevron again, or by pressing Escape
- Most-recent message should be pre-expanded within the inline preview; earlier messages collapsed to a single summarized line

---

## 4. Running vs. Completed Session Differentiation

### Linear (task status)
- Uses a small **colored circle icon** before the issue title: grey = backlog, white circle = unstarted, half-filled = in progress, filled = done, red X = cancelled
- Color alone carries the signal; no separate column or badge
- "In progress" items in cycle view get an additional subtle **animated ring** around the status dot

### Jira (task status)
- Status shown as a **colored pill/badge** with text label: "To Do" (grey), "In Progress" (blue), "Done" (green), "Blocked" (red)
- More verbose than Linear but scannable in a dense list
- Active sprint issues get a blue border accent on the left edge of the card in board view

### Warp Terminal (session management)
- Warp does not have a native "session list" view that distinguishes running vs. stopped sessions — this is a known gap
- tmux itself uses the status bar to show active pane titles but has no list UI with visual status differentiation out of the box

### tmux TUI tools (tmux-resurrect, tmuxinator, Tmux Plugin Manager display)
- tmux `list-sessions` output: shows session name, window count, created timestamp, and "(attached)" suffix for active sessions — purely text-based
- Third-party tmux managers (e.g., `sesh`, `t`) use fzf previews that show session name + attached status in a fuzzy picker

### VS Code tabs / editors
- Active document tab: has a close icon visible; modified (unsaved) documents show a dot indicator instead of close icon
- No "running vs. stopped" concept native to file tabs — process terminals show a colored dot in the terminal tab when a process is actively running

### Cursor AI concurrent sessions
- Active (running) sessions: show a spinner or "Running" status badge in the agent sidebar
- Idle sessions: no spinner, timestamp of last interaction shown

### Patterns observed

| Visual treatment | Tools | Notes |
|---|---|---|
| Color-coded icon before title | Linear, GitHub | Cleanest; minimal space |
| Colored status pill/badge with text | Jira, Linear compact view | More explicit; takes more width |
| Animated indicator (pulse / spinner) | Cursor, browser loading tabs | Draws attention; appropriate for "in progress" only |
| "(attached)" text suffix | tmux list-sessions | Works for terminal; not web-friendly |
| Left-edge accent border | Jira board, some card UIs | Scannability at the list level |
| Background tint on card | Many kanban boards | Risk of accessibility issues with low contrast |

### Recommendation

Use a **two-signal system**:
1. **Left-edge accent color** on the card row: green = Running, yellow = Paused/Idle, grey = Stopped/Done — scannable at a glance across many rows
2. **Status pill** (small, right-aligned or below the title): text label "Running", "Paused", "Done" — explicit for accessibility and keyboard navigation

For "Running" sessions, add a **subtle animated pulse** on the left-edge accent (not the entire card) to differentiate from "Paused" which has the same green-family color. This follows the Linear animated-ring pattern at a lower visual weight.

---

## Cross-Cutting Recommendations

### Information hierarchy for our session card

```
[STATUS BAR] [TITLE: first user message or session name]       [LAST ACTIVE: "3h ago"]
             [REPO: my-project] [BRANCH: feat/auth]            [STATUS PILL: Running]
             [PATH: ~/code/my-project]                         [MSG COUNT: 42 msgs]
             ─────────────────────────────────────────────────────────────────────
             [PREVIEW: last message snippet, 1 line]            [v Show 4 messages]
```

On expand:

```
             ┌──────────────────────────────────────────────────────────────────┐
             │ You: Can you refactor the auth middleware to use JWT?            │
             │ Assistant: Sure. Here's the updated middleware...               │
             │ You: Now add refresh token support.                             │
             │ Assistant: I've added refresh token logic in auth/jwt.go...     │
             │ You: Great, also update the tests.            [^ Collapse]      │
             └──────────────────────────────────────────────────────────────────┘
             [Resume]  [v  New worktree | Open in directory... | Copy path]
```

### Fork/resume action placement
- Action buttons appear **on hover** or **when card is focused** (keyboard nav), not always-visible — reduces visual noise in a long list
- Primary "Resume" button is full-color; fork options are in a dropdown chevron on the right

### Density modes
- Following Linear's approach, offer a **compact / comfortable** density toggle so power users can see more sessions per screen while casual users get more breathing room

---

## Sources

- [How we redesigned the Linear UI (part II) - Linear](https://linear.app/now/how-we-redesigned-the-linear-ui)
- [Linear UX - UX Collective](https://uxdesign.cc/linear-ux-ad7dc634b5b1)
- [Deep dive into GitHub Codespaces - GitHub Docs](https://docs.github.com/en/codespaces/about-codespaces/deep-dive)
- [Using source control in your codespace - GitHub Docs](https://docs.github.com/en/codespaces/developing-in-a-codespace/using-source-control-in-your-codespace)
- [Cursor AI changelog - Show history in command palette](https://cursor.com/changelog/page/3)
- [cursor-history - open-source Cursor chat history browser](https://github.com/S2thend/cursor-history)
- [cursor-view - browse and export Cursor chat history](https://github.com/saharmor/cursor-view)
- [Threaded emails - eM Client](https://www.emclient.com/threaded-emails)
- [Issue fields: Structured issue metadata - GitHub Changelog](https://github.blog/changelog/2026-03-12-issue-fields-structured-issue-metadata-is-in-public-preview/)
- [Welcome screen - CLion Documentation (JetBrains)](https://www.jetbrains.com/help/clion/welcome-screen.html)
- [Welcome screen - PyCharm Documentation (JetBrains)](https://www.jetbrains.com/help/pycharm/welcome-screen.html)
- [Linear Conceptual Model - Linear Docs](https://linear.app/docs/conceptual-model)
- [A curated list of SaaS UI workflow patterns - GitHub Gist](https://gist.github.com/mpaiva-cc/d4ef3a652872cb5a91aa529db98d62dd)
