# Requirements: Unfinished Work Tab

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-25

## Problem Statement

Stapler Squad users work across multiple repos and branches simultaneously — some via active
sessions, some in worktrees created outside of stapler-squad, some in bare checkouts. When context
switches happen (a session pauses, a user closes their laptop, they move to a different task),
unfinished work silently accumulates: uncommitted edits, branches never pushed, PRs never opened,
branches that have drifted behind main.

There is currently no single place to answer the question: **"What have I started but not
finished, and where should I pick up next?"**

Users must manually `git status` across multiple directories, remember which repos they were
touching, and mentally track which branches are ahead of main. This overhead increases as the
number of concurrent sessions grows, and is a direct source of dropped work.

**Primary users:** Solo developers managing 3–15+ concurrent AI sessions and branches across
multiple git repositories.

## Success Criteria

- A dedicated "Unfinished" tab exists in the main navigation alongside Sessions and Search
- All git worktrees with uncommitted changes, commits ahead of main, or branches behind main
  are surfaced automatically in this tab
- Sources include: worktrees discovered from active sessions, user-configured watch directories,
  and manually pinned repos — all three can be active simultaneously
- Each item shows the repo + branch, and the specific reason it's "unfinished" (uncommitted /
  ahead / behind)
- Clicking an item expands it inline to show diff stats (files, ±lines, commits ahead/behind)
  with [View Files] and [Open Session] action buttons
- [Open Session] creates or reattaches to a stapler-squad session for that worktree's branch
- A commit-and-push shortcut lets users quickly close out small changes without opening a session
- Items can be dismissed (hidden permanently) or snoozed (hidden until next git change)
- An AI-generated summary of what the unfinished work is about is available on demand per item
- The view refreshes automatically in the background; a manual refresh is also available

## Scope

### Must Have (MoSCoW)

**Tab & Navigation**
- New "Unfinished" tab in top navigation bar, rendered as `[Sessions] [Unfinished ✦] [Search]`
- Badge/count on tab showing number of unfinished items
- Tab is the authoritative view — not a popover, not a section in the session list

**Source Discovery (all three must work simultaneously)**
- Auto-spider: for each active stapler-squad session, detect the repo root, then enumerate all
  git worktrees of that repo (`git worktree list`) and surface any that are unfinished
- Watch dirs: user adds one or more root directories in settings (e.g. `~/code`); stapler-squad
  recursively finds all git repos (`.git` directories) inside and scans them
- Pinned repos: user explicitly adds a specific repo path; scanned like any other

**Unfinished Detection Criteria**
- Has uncommitted changes: `git status --porcelain` returns non-empty output
- Ahead of main: branch has commits not in main (`git rev-list main..HEAD --count > 0`)
- Behind main: branch is missing commits from main (`git rev-list HEAD..main --count > 0`)
- A worktree qualifies if ANY of the three criteria are met

**Item List UX**
- Items are grouped by repo (repo name as section header)
- Within each repo, items sorted by most-recently-modified worktree first
- Each item card shows: branch name, worktree path (abbreviated), status chips (Uncommitted /
  Ahead N / Behind N)
- Clicking an item expands it inline (accordion) without navigating away

**Expanded Item (accordion)**
- Diff stats: number of changed files, total lines added/removed
- Commits ahead of main: count + short commit messages (up to 5)
- [View Files] button: opens existing git file browser for that worktree
- [Open Session] button: creates or reattaches a stapler-squad session for that branch/worktree

**Actions**
- Open / Resume session (creates session if none exists, attaches if one does)
- Commit & push shortcut: stage all → prompt for commit message → `git commit -m` + `git push`
  (no full session required; runs as a one-shot background operation)
- Dismiss: permanently hide this worktree from the Unfinished list (persisted in app state)
- Snooze: hide until the next git state change in that worktree
- Show AI Summary: generate a short natural-language paragraph describing what the work is about,
  derived from `git diff` + recent commit messages; displayed inline under the accordion

**Background Refresh**
- Scan runs on a background schedule (default: every 60 seconds)
- Filesystem watcher triggers re-scan when worktree directories change (reuse existing fsnotify)
- Manual refresh button in tab header
- Dismiss/snooze state persists across restarts

### Should Have
- Filter chips at top of tab: All / Uncommitted / Ahead / Behind
- Search within the Unfinished tab (by repo name, branch name, path)
- Sort options: Last Modified / Most Changes / Repo Name
- Keyboard navigation between items (up/down, enter to expand, `o` to open session)
- Configurable watch dirs UI in Settings page

### Could Have
- Stale item aging: gray out items that haven't changed in 30+ days
- GitHub PR status shown on items that have an associated open PR (reuse PR tracking from
  github-pr-status project)
- "Archive" action that commits a WIP message and stashes for later reference

### Out of Scope
- Non-git repos or working directories (only git is supported)
- Automatic conflict resolution for branches behind main
- Multi-remote tracking (only `origin` remote / main branch comparison for now)
- Mobile / responsive layout (desktop web UI only)
- Real-time collaborative features

## Constraints

**Tech stack:**
- Go backend (all git scanning logic must be in Go)
- Git operations via `git` CLI subprocess (consistent with existing worktree/branch operations)
- React/TypeScript frontend with vanilla-extract CSS (per ADR 009)
- ConnectRPC streaming for pushing scan results to the web UI (existing transport)
- No new Go module dependencies unless unavoidable
- AI summary calls use the existing Claude API integration (or call out to CLI subprocess)

**Existing infrastructure to reuse:**
- `session/git/` — worktree management, branch detection, git subprocess execution
- `ui/overlay/` — git repository discovery (path validation, git repo detection)
- `session/storage.go` — app state persistence (extend for dismiss/snooze state)
- `server/services/` — ConnectRPC service layer for scan results
- fsnotify-based filesystem watcher (already used for claude-mux socket discovery)
- File browser component (existing, opened via [View Files])
- GitHub PR badge component (optional, for items with open PRs)

**Proto schema:** Define a new `UnfinishedWork` message and `ScanWorkdirs` RPC in a new or
extended proto file. Do not modify the existing session `types.proto` schema.

**Auth:** No additional auth required — all git operations are local filesystem operations.

## UX Design

### Tab Layout

```
┌─────────────────────────────────────────────────────┐
│ [Sessions]  [Unfinished ✦ 7]  [Search]              │
├─────────────────────────────────────────────────────┤
│ 🔄 Last scanned 12s ago   [Refresh]  [+ Watch Dir]  │
│ ──────────────────────────────────────────────────  │
│ Filter: [All ✓] [Uncommitted] [Ahead] [Behind]      │
├─────────────────────────────────────────────────────┤
│ repo-a — ~/code/repo-a (3 items)                    │
│  ▶ feature-auth      Uncommitted · Ahead 4          │
│  ▶ fix-payments      Behind 3                       │
│  ▶ spike-oauth       Uncommitted                    │
│                                                     │
│ repo-b — ~/work/repo-b (2 items)                    │
│  ▼ refactor-db       Uncommitted · Ahead 1          │
│    ┄ 3 files changed · +142 −28                     │
│    ┄ 1 commit ahead: "WIP: extract query builder"   │
│    [View Files]  [Open Session]  [Commit & Push]    │
│    [Show AI Summary]  [Snooze]  [Dismiss]           │
│                                                     │
│                                                     │
│ AI Summary (refactor-db):                           │
│ "This branch began extracting query construction    │
│  logic from the service layer into a reusable       │
│  builder. 3 files modified; tests not yet updated." │
└─────────────────────────────────────────────────────┘
```

### Source Configuration (Settings page extension)

```
Unfinished Work Sources
  [✓] Auto-spider active sessions
  [✓] Watch directories:
       ~/code          [✕]
       ~/work          [✕]
       [+ Add directory]
  [✓] Pinned repos:
       ~/personal/blog [✕]
       [+ Pin a repo]
```

## Research Dimensions Needed

- [ ] Stack — git CLI options for efficient worktree scanning; whether `git worktree list --porcelain`
      is reliable cross-platform; fsnotify coverage for worktree dirs; AI summary approach
- [ ] Features — how comparable tools (VS Code source control, GitButler, Tower, Fork) surface
      uncommitted/unmerged work across multiple repos; keyboard-first UX patterns
- [ ] Architecture — scan lifecycle (goroutine per repo vs. centralized poller); ConnectRPC
      streaming vs. polling for pushing results to UI; dismiss/snooze persistence model;
      watch dir recursive scan strategy (depth limits, symlink handling)
- [ ] Pitfalls — performance on large repos with many worktrees; permission errors on dirs the
      user can't read; bare repos (skip them); detached HEAD worktrees; very long-running git
      commands hanging the scanner; rate-limiting AI summary calls
