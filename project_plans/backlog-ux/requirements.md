# Backlog UX Improvements — Requirements

## Problem Statement

The backlog feature now works functionally (triage is producing results), but the UX is poor in several key ways:

1. Backlog-spawned work sessions (`backlog:work` tag) appear in the main session view uncategorized by default, cluttering it with sessions the user didn't manually create.
2. There is no way to delete backlog sessions from the backlog view — only from the session list where they are indistinguishable from user sessions.
3. Backlog items have no visible status/progress on their cards in the session view.
4. No at-a-glance count of pending/active backlog items anywhere in the UI.
5. No way to quickly add new backlog items from the session view (requires navigating to `/backlog`).
6. Session organization is flat — users cannot group by multiple dimensions (e.g., by git repo and then by status/tag within that repo).

## Goals

- Make backlog-spawned sessions clearly distinguishable and well-organized in the session view without cluttering the default experience.
- Enable deletion of backlog sessions with three modes: per-item, bulk (done items), and automatic.
- Surface backlog status/progress in the session view so users don't need to navigate to `/backlog` to check progress.
- Add a backlog item count badge in the navigation for quick situational awareness.
- Enable quick backlog item creation from the session view via the omnibar or a dedicated shortcut.
- Introduce multi-level grouping (e.g., group by git repo, then sub-group by tag/status) for better session organization at scale.

## Non-Goals

- Redesigning the backlog board page (`/backlog/board`) — that is a separate concern.
- Changing the backlog item triage/review workflow itself.
- Adding nested grouping beyond two levels (repo → secondary dimension) in this pass.

## User Stories

### US-1: Auto-categorization of session types

**As a user**, I want sessions to be automatically categorized based on their origin, so that I can quickly distinguish backlog-driven work from my own manual sessions.

**Acceptance criteria:**
- All sessions spawned by the backlog system (work, triage, review) are automatically assigned the category `"Backlog"` in addition to their existing `backlog:*` tags.
- All sessions spawned by workflows are automatically assigned the category `"Workflow"`.
- All sessions created manually by the user (via omnibar or API without a system origin) are automatically assigned the category `"Personal"` if no explicit category is provided.
- When grouping by Category, backlog sessions appear under "Backlog", workflow sessions under "Workflow", and manual sessions under "Personal" (not "Uncategorized").
- Category auto-assignment happens at creation time in `CreateDirectorySession` / `CreateSession` — it is never retroactively applied.

### US-2: Deletion of backlog sessions

**As a user**, I want to be able to delete backlog sessions through multiple pathways, so that I can keep my session list clean.

**Acceptance criteria:**
- Each backlog item in the `/backlog` list view has a "Delete sessions" action (in the overflow/actions menu) that deletes all sessions linked to that backlog item.
- A "Clear completed" button appears when any backlog items are in `done` or `archived` status; clicking it deletes all sessions linked to done/archived items and archives those items.
- When a backlog item transitions to `done`, its triage and review sessions (hidden=true ones) are automatically deleted. Work sessions are NOT auto-deleted (user may want to review the output).
- The backend exposes a new `DeleteBacklogItemSessions(item_id)` RPC (or reuses `DeleteSession` per-session) that the backlog UI can call.

### US-3: Backlog status/progress on session cards

**As a user**, I want to see the backlog status of a session in the session list, so that I understand what the agent is working on without navigating to the backlog page.

**Acceptance criteria:**
- Session cards for sessions tagged `backlog:work`, `backlog:triage`, or `backlog:review` display a compact "Backlog" badge or chip with the associated item's current status (e.g., "In Progress", "Review", "Done").
- The badge is linked to the backlog item (clicking navigates to `/backlog?item=<id>`).
- Sessions without a backlog item link do not show this badge.
- The badge updates in real time as the backlog item status changes (via existing WatchSessions / WatchBacklogItems stream, or polling).

### US-4: Backlog item count badge in navigation

**As a user**, I want to see a count of active/pending backlog items in the navigation, so that I have situational awareness at a glance.

**Acceptance criteria:**
- The "Backlog" nav link shows a numeric badge when there are items in `ready`, `in_progress`, or `review` status.
- The badge count = number of items in those active states.
- The badge disappears when count = 0.
- The badge updates in real time (uses the existing WatchBacklogItems stream or a polling interval).
- The badge is WCAG AA accessible (visible text contrast, aria-label).

### US-5: Quick backlog item creation from session view

**As a user**, I want to add backlog items without leaving the session view, so that I can capture ideas without context-switching.

**Acceptance criteria:**
- The omnibar supports a new input pattern (detector) for quick backlog item creation: typing `backlog: <description>` or `/backlog <description>` triggers a "Create backlog item" action.
- Selecting that action opens a minimal creation form (title + repo path) — not the full form.
- On submit, the item is created in `idea` status (same as the existing form default).
- The creation is confirmed with a toast/notification and a link to the created item.
- This registers in both the OmnibarAction union (as `create_backlog_item`) and the DetectorRegistry.

### US-6: Multi-level grouping (git repo → secondary dimension)

**As a user**, I want to group sessions first by git repository and then by a secondary dimension (tag, status, etc.), so that I can organize large session lists more effectively.

**Acceptance criteria:**
- A new "Project → Tag" composite grouping strategy is available in the grouping strategy cycle.
- A new "Project → Status" composite grouping strategy is also available.
- Under each project group, sessions are sub-grouped by the secondary dimension.
- The grouping UI allows selecting the secondary dimension (default: Tag).
- Collapsing a project group collapses all its sub-groups.
- Composite grouping degrades gracefully: sessions with no project assignment appear under "No Project → [secondary group]".
- The existing 9 single-dimension strategies remain unchanged.

## Technical Context

### Current session creation flow
- `backlog_service.go:935` — work sessions: `CreateDirectorySession(..., tags=["backlog:work"], hidden=false)`
- `backlog_service.go:1181` — triage sessions: `CreateDirectorySession(..., tags=["backlog:triage"], hidden=true)`
- `backlog_service.go:1539` — re-review sessions: `CreateDirectorySession(..., tags=["backlog:review"], hidden=true)`
- `session_service.go:581` — review sessions: `CreateDirectorySession(..., tags=["backlog:review"], hidden=true)`

### Current tagging infrastructure
- `session/instance_tags.go` — tag management (max 10 tags, max 24 chars each)
- `session/instance.go` — `Category` field (single string), `Tags` []string
- Grouping strategies: `web-app/src/lib/grouping/strategies.ts` — 9 strategies, Tag is multi-membership
- Max tag count is 10; category is a separate field (not a tag)

### Backend RPC surface
- `DeleteSession` RPC exists at `session_service.go:1581` — supports force flag
- `UpdateSession` RPC at `session_service.go:1265` — can update tags and category
- `ListBacklogItems` and `WatchBacklogItems` exist for real-time updates

### Frontend omnibar
- Detector registry: `web-app/src/lib/omnibar/detector.ts`
- Action dispatch: `web-app/src/lib/omnibar/actions/dispatch.ts`
- Session creation 7-touchpoint registry: `.claude/rules/session-creation-registry.md`

## Priority

| Story | Priority | Effort |
|-------|----------|--------|
| US-1: Auto-categorization | P0 — core fix, blocks everything | S |
| US-2: Deletion | P0 — core fix | M |
| US-3: Status badge on session cards | P1 | M |
| US-4: Nav badge count | P1 | S |
| US-5: Quick create from omnibar | P2 | M |
| US-6: Multi-level grouping | P2 | L |

## Constraints

- No changes to the backlog triage/review logic.
- All new CSS must use vanilla-extract (`.css.ts`), not CSS Modules or inline styles.
- New omnibar features must follow the 7-touchpoint session creation registry AND the OmnibarAction/DetectorRegistry rules.
- Proto changes require `make generate-proto`.
- Feature registry files in `docs/registry/features/` must be updated for any new RPCs or major UI features.
