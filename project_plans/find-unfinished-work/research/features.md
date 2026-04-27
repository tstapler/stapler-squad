# Research: Features — Unfinished Work Tab

## Summary

- VS Code Source Control panel, GitButler, and Tower all surface uncommitted work per-repo with **repo-as-section-header** grouping; the most common pattern is a two-level tree (repo name → changed items), which maps directly to the requirements.
- Snooze/dismiss patterns from GitHub Notifications, Linear, and task managers consistently use two distinct actions: **permanent dismiss** (clear/archive) and **temporary snooze** (until a time or next change), with snooze always surfaced as a secondary action.
- AI diff summaries (GitHub Copilot PR summaries, GitLens AI commit explanations) share a **lazy on-demand** trigger model — never auto-generated on load; always triggered by a discrete user action — with output shown inline in an expandable area.
- The strongest "pick up where you left off" ordering heuristic across all tools is **most-recently-modified** (mtime of index or working tree), which matches the requirements' specification.

## Findings

### Multi-Repo Git Tools — UX Patterns for Uncommitted Work

**VS Code Source Control Panel:**
- Groups by repo when multiple repos are open (workspace mode). Each repo gets its own collapsible section with the repo name as header.
- Status chips are not used; instead, file counts are shown as badges on section headers.
- "Pending changes" (unstaged) and "staged changes" are subgroups within each repo section.
- The panel is a dedicated sidebar panel, not a tab — it co-exists with the file explorer.
- Inline diffs are shown on click (single file diff, not aggregate).

**GitButler:**
- Virtual branches concept: each branch is a "lane" shown simultaneously. Uncommitted work is per-lane and drag-and-droppable between lanes.
- All virtual branches are visible on one screen — no grouping by repo (single-repo tool).
- Status chips are implicit (the lane itself IS the uncommitted work container).
- Key insight: GitButler treats uncommitted work as a first-class persistent state, not a transient warning.

**Tower (Git GUI):**
- Left sidebar: repos listed as items; clicking a repo shows its status in the main area.
- Main area shows: unstaged changes, staged changes, stash items, and branch list.
- Ahead/behind indicators appear as small badge numbers (+N / -N) next to branch names in the branch list.
- No multi-repo simultaneous view — one repo at a time.

**Fork (Git client):**
- Similar to Tower: single repo at a time, with a clean sidebar for repo switching.
- "Local Changes" section shows uncommitted changes with file-level diff.
- Ahead/behind shown as arrows (↑3 ↓2) on branch names.

**GitHub Desktop:**
- Single-repo view. Uncommitted changes shown in left panel as a list of changed files.
- "Push origin" badge shows commits ahead count.
- No multi-repo simultaneous view.

**Key cross-tool patterns:**
1. **Repo-as-section-header** is universal when showing multi-repo state (VS Code, any multi-root workspace).
2. **Ahead/behind counts** are shown as compact tokens next to branch names (↑N ↓N or +N/-N or "N ahead / M behind").
3. **Status chips** (Uncommitted / Ahead N / Behind N) are found in CI/CD tools (GitHub PR status badges) more than in git clients, but they communicate clearly.
4. **Abbreviated paths** (truncating parent dirs, showing only repo name + last 1-2 path components) is the universal approach for worktree path display.

### Snooze / Dismiss UX Patterns

**GitHub Notifications:**
- Two actions: "Done" (permanent dismiss/archive) and "Snooze" (temporarily hide).
- Snooze options: 1 hour, 8 hours, next week, next month. No "until next change" option.
- Snoozed items reappear automatically after the chosen interval.
- UI pattern: hover-reveal action buttons on each row, or checkbox + bulk action.

**Linear (issue tracker):**
- "Cancel" (permanent dismiss) vs "Snooze" (time-based: tomorrow, next week, custom).
- Keyboard shortcut for snooze: `s` key.
- Snooze icon (clock) is the universal signifier.

**Things 3 (task manager):**
- "Someday" (indefinite defer, visible in a dedicated Someday list but not in Today/Upcoming).
- "Deadline" vs "When" separation: snooze is represented as changing the "When" date.
- No git-awareness; snooze is purely time-based.

**Patterns applicable to Unfinished Work tab:**

| Action | Trigger | Storage |
|---|---|---|
| **Dismiss** | Explicit button click | Permanent record in app state (worktree path + repo path as key) |
| **Snooze until next change** | Explicit button | Store snooze record with "snooze type: until-change"; clear on next git status change |
| **Snooze until time** | (optional) dropdown | Store with ISO timestamp; reappear when time elapses |

The "snooze until next change" concept is novel compared to existing tools but maps well to git semantics: a worktree that hasn't changed since you snoozed it stays hidden; any new commit or file change surfaces it again.

**UX recommendation:** Show dismiss and snooze as icon buttons (×, 🕐) revealed on item hover, consistent with GitHub Notifications pattern. Keyboard shortcut for snooze: `s` key on focused item.

### AI Summary Patterns in Dev Tools

**GitHub Copilot PR summaries:**
- Generated on demand when user clicks "Summarize" button in PR creation/review flow.
- Output format: paragraph of prose followed by bulleted list of changed files with descriptions.
- Not auto-generated — requires explicit user action.
- Displayed inline in a collapsible section below the PR description.

**GitLens (VS Code extension):**
- "AI-powered commit explanations" — shown when user expands a commit in the timeline.
- Generated lazily on expansion, not pre-loaded.
- Shown as a short paragraph: "This commit..." with 2-4 sentences.
- Cached per commit hash; same commit doesn't regenerate.

**Cursor:**
- "AI reviewer" triggered via "More" → "AI Review" menu; scans git diff.
- Output is a reviewers-eye summary: what changed, potential issues, suggestions.
- Shown in sidebar panel.

**Patterns for Unfinished Work AI summary:**
1. **Lazy trigger**: "Summarize" button per item, not auto-generated on load.
2. **Inline expansion**: summary appears below item details (after diff stats), not in a modal.
3. **Short format**: 2-4 sentences. "This branch modifies the authentication flow, adding a new JWT refresh mechanism and removing the old cookie-based session. 3 files changed."
4. **Cached**: same diff hash → same summary (no re-generation unless diff changes).
5. **Loading state**: spinner while generating; replace with text when done.

### Ordering and Grouping Strategies for "Pick Up Where You Left Off"

**Most common ordering signals across tools:**
1. **Most recently modified** (mtime of `.git/index` or most recent file change in working tree) — used by VS Code, GitHub Desktop, Tower.
2. **Commit timestamp** (time of most recent commit) — used by GitLens timeline.
3. **User-defined** (manual pinning, drag reorder) — GitButler lanes, Tower favorites.

**Grouping options:**
- By repo (universal in multi-repo tools) — maps to requirements.
- By "why unfinished" type (uncommitted / ahead / behind) — not used in existing tools; likely confusing.
- By age/staleness — not used in existing tools for source control; used in task managers (overdue → today → upcoming).

**Recommendation:** Group by repo (as specified). Within each repo, sort worktrees by most-recently-modified (mtime of working tree directory or last git operation time). The most-recently-modified signal is available without any git subprocess — just `os.Stat(worktreePath).ModTime()`.

### "Which Worktree Is This" — Path Display Patterns

The path display problem: worktree paths like `/Users/tyler/.stapler-squad/worktrees/feat-auth_1a2b3c` are not human-readable.

**Patterns from existing tools:**
- **Branch name is primary** — show the branch name prominently; path is secondary.
- **Repo name from remote URL** — `git remote get-url origin | sed 's|.*/||; s|\.git$||'` gives `my-repo`.
- **Abbreviated path** — show `~/code/my-project` instead of `/Users/tyler/code/my-project`.

**Recommendation:** Show `<repo-name> / <branch-name>` as the primary label. Show the worktree path abbreviated (replace home dir with `~`) as secondary text. If the worktree path is inside `~/.stapler-squad/worktrees/`, show only the repo name and branch name — the path is an implementation detail users don't need.

### Keyboard-First UX in Multi-Repo Tools

- VS Code Source Control: `Ctrl+Shift+G` opens panel; arrow keys navigate; `Enter` opens diff; no git-specific shortcuts.
- GitButler: no prominent keyboard shortcuts; mouse-first design.
- GitHub Notifications: `j`/`k` to navigate, `e` to archive, `s` to snooze — Gmail-style.

**Recommendation for Unfinished Work tab:**
- `j`/`k` or arrow keys: navigate between items.
- `Enter` or `Space`: expand/collapse item accordion.
- `o`: open/resume session for focused item.
- `s`: snooze focused item.
- `d`: dismiss focused item.
- `r`: trigger manual refresh.

## Recommendations

1. Use repo-as-section-header grouping (as specified); show repo name from `git remote get-url origin` basename, fallback to directory basename.
2. Show ahead/behind as compact chips: `Uncommitted`, `↑3 ahead`, `↓2 behind` — combining all three on the item card.
3. AI summary: lazy on-demand trigger ("Summarize" button), 2-4 sentence inline output, cache by diff hash for 24h.
4. Snooze UI: hover-reveal icon buttons (× dismiss, 🕐 snooze until next change). For MVP, "snooze until next change" is the only snooze type (no time picker needed).
5. Order worktrees within each repo section by `os.Stat(worktreePath).ModTime()` descending — free to compute, no git subprocess needed.
6. Display: branch name prominent (large text), worktree path abbreviated (small secondary text), status chips inline.

## Open Questions

- Should the "Behind N" signal trigger a fetch to get accurate data (requires network) or rely on local tracking branch state (may be stale)?
- Should "Open Session" create a new stapler-squad session or attempt to re-attach to an existing one for that branch? (The existing `setupFromExistingBranch` already handles the branch-exists case.)
- Is a "snooze until date/time" option worth the complexity for v1, or does "snooze until next change" suffice?
- Should the AI summary use the full diff or just `git log --oneline` commits? (Diff is richer but can be huge for large changes.)
