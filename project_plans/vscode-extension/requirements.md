# Requirements: VS Code Extension for Session Status & Workspace Navigation

## Source

- Backlog item `e7aa9328-e77c-4f05-af6d-fb3db349afde`
- Migrated from `TylerStaplerAtFanatics/stapler-squad#50` (created 2026-04-09)
- Non-interactive triage session — this doc was authored directly from the item's
  title/description/labels (`enhancement`, `p3`), skipping the interactive
  `sdd:1-ideate` interview per the item's task instructions.

## Problem

Developers running stapler-squad sessions spend most of their time in VS Code, not
the browser dashboard at `localhost:8543`. Checking session status, finding which
sessions need review, and locating/opening a given session's git worktree all
currently require an editor→browser context switch. Competing tools (Mux/coder,
Nimbalyst/crystal, cmux) already surface this natively in-editor.

## Goals

1. Surface session status (count, review-queue depth) in the VS Code status bar
   without opening the browser.
2. Let a developer navigate directly from a session to its worktree folder inside
   VS Code (open as workspace / new window).
3. Allow approving/denying review-queue items from inside the editor.
4. Expose session creation and worktree navigation via the command palette.

## Non-Goals

- Replacing the web dashboard — the extension is a companion surface, not a full
  reimplementation of `web-app/`.
- Streaming AI-agent edits directly into open editor buffers (that's the
  Nimbalyst/crystal model, not requested here).
- Any change to `session/`, `server/services/`, or proto definitions — the
  extension is a **pure API consumer** of the existing ConnectRPC session service.
  No backend behavior changes are in scope.
- Auto-opening changed files in the editor (the "auto-open worktree" config toggle)
  is included as a *v1 nice-to-have* per the original issue, not a hard requirement
  — see AC-8.

## Proposed Behavior (from issue)

**Status bar item**
- Shows active session count and review queue depth, e.g. `⚡ 4 sessions  🔔 2 need review`
- Click → opens the stapler-squad dashboard in the default browser

**Session sidebar panel (Tree View)**
- Lists all active sessions with status badges
- Click a session → opens its worktree folder in the current or a new VS Code window
- Queue items shown inline with Approve / Deny actions that call the stapler-squad API

**Command palette**
- `Stapler Squad: Open Dashboard`
- `Stapler Squad: New Session in Current Folder`
- `Stapler Squad: Open Session Worktree` (quick-pick over active sessions)

**Auto-open worktree**
- Optional: when a session's worktree has changes, auto-open the changed files

**Configuration** (`staplerSquad.*` settings)
- `serverUrl` (default `http://localhost:8543`)
- `showStatusBar` (default `true`)
- `autoOpenWorktree` (default `false`)
- `notifyOnQueueItem` (default `true`)

## Acceptance Criteria

1. A VS Code extension package exists (`extensions/vscode/` or similar), builds,
   and can be installed/loaded via `Extension Development Host` (F5) without errors.
2. The status bar item shows live active-session count and review-queue depth,
   refreshing on an interval or via a push/poll mechanism against the existing
   ConnectRPC session service; clicking it opens `staplerSquad.serverUrl` in the
   default browser.
3. A sidebar Tree View lists active sessions with a status badge per session
   (matches the status values already used by the web dashboard).
4. Clicking a session in the tree opens that session's worktree path as a VS Code
   workspace folder (same window or new window, user-configurable or command-driven).
5. Review-queue items render inline in the sidebar with Approve/Deny actions that
   call the existing approve/deny RPCs and update the tree without a manual refresh.
6. Command palette exposes `Stapler Squad: Open Dashboard`,
   `Stapler Squad: New Session in Current Folder`, and
   `Stapler Squad: Open Session Worktree`, each functioning per the descriptions above.
7. All four `staplerSquad.*` settings are declared in `package.json` contribution
   points with the defaults above and take effect without reloading the extension
   (where VS Code supports live config) or with a documented reload requirement.
8. `autoOpenWorktree` — when enabled, opening/switching to a session with a dirty
   worktree opens its changed files in editor tabs. (Nice-to-have; may ship as a
   fast-follow if it meaningfully expands v1 scope — flag this explicitly in the plan.)
9. Extension has no impact on `server/services/`, `session/`, or `proto/` — it
   only consumes existing RPCs (extend/verify existing session-list and
   approve/deny endpoints are sufficient before assuming any backend change is needed).
10. Basic test coverage exists for the extension's data-fetching/status-formatting
    logic (unit tests), matching how other TS code in this repo is tested (Jest).

## Open Questions

- Auth: does the extension need to send any credentials to reach the local
  ConnectRPC server, or is `localhost:8543` assumed trusted/unauthenticated like
  the current web-app dev flow? (Research phase should check `server/server.go`
  and `web-app` client setup for existing auth handling.)
- Packaging/distribution: publish to the VS Code Marketplace / Open VSX, or
  ship as a `.vsix` built by CI and installed manually? Issue doesn't specify.
- Does "review queue" in the issue map 1:1 to the existing backlog/approval-rule
  RPCs (`list_approval_rules`, `submit_review_verdict`, etc.) or a different queue
  concept? Needs confirming against current proto during research.
