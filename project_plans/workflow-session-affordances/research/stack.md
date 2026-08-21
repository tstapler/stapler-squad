# Stack Research: Session Listing, Grouping, Filtering, and Detail Panel

## 1. Technologies Used

### Frontend
- **React 18** ("use client" components throughout) with Next.js
- **TypeScript** for all UI code
- **@connectrpc/connect** — ConnectRPC client for all API calls
  - `createClient(SessionService, getConnectTransport())` is the standard pattern
  - Transport configured via `getConnectTransport()` from `@/lib/api/transport`
- **vanilla-extract** — compile-time CSS-in-JS for all new components (`.css.ts` files)
  - Token contract: `web-app/src/styles/theme.css.ts` (imported as `vars`)
  - `style()`, `styleVariants()`, `keyframes()`, `recipe()` are the main APIs used
  - Colocated: every `Foo.tsx` has a `Foo.css.ts` sibling
- **@tanstack/react-virtual** — row virtualizer for the session list in row mode
- **Lucide React** — icon library (`Terminal`, `GitCompare`, `GitBranch`, etc.)
- **Redux Toolkit** — `useAppSelector` for terminal-detected session status from `sessionsSlice`
- **Radix UI** — Dialog primitives (via `Modal`/`ModalContent`/`ModalTitle`/`ModalFooter` wrapper components)
- **localStorage** — filter/sort/grouping preferences are persisted client-side with `storageKeyPrefix` support for multi-pane layouts

### Backend
- **Go** web server
- **ConnectRPC** (`connectrpc.com/connect`) — server-side RPC framework
- **ent ORM** (`entgo.io/ent`) — type-safe SQL ORM with code generation
  - Generation command: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  - Schemas live in `session/ent/schema/`
- **SQLite** — embedded database (workspace-based isolation)
- **Protobuf / proto3** — API contract in `proto/session/v1/`
- **google/uuid** — UUID generation for Workflow IDs

---

## 2. Component Tree: Session List → Session Detail

```
SessionList (web-app/src/components/sessions/SessionList.tsx)
├── Filter/sort/grouping controls (all inline, no separate component)
│   ├── <input> search
│   ├── <select> status filter
│   ├── <select> category filter
│   ├── <select> tag filter
│   ├── <select> grouping strategy
│   └── <select> sort field + direction button
├── ColumnPicker (row mode only) — (ColumnPicker.tsx)
├── BulkActions (BulkActions.tsx) — shown when selectMode=true
├── MemoryPressureCallout (MemoryPressureCallout.tsx)
├── [Row mode — virtualized via @tanstack/react-virtual]
│   └── SessionRow (SessionRow.tsx) — 50px estimated height
│       └── SessionActionsOverflow (SessionActionsOverflow.tsx)
└── [Card mode — non-virtualized]
    └── SessionCard (SessionCard.tsx)  ← memo-wrapped
        ├── ReviewQueueBadge (ReviewQueueBadge.tsx)
        ├── StatusBadge (StatusBadge.tsx)      ← terminal-detected status
        ├── SubStatusChip (SubStatusChip.tsx)  ← proto sub_status field
        ├── GitHubBadge (GitHubBadge.tsx)
        ├── TagEditor (TagEditor.tsx)           ← modal
        └── SessionActionsOverflow (SessionActionsOverflow.tsx)

SessionDetailView (web-app/src/components/sessions/SessionDetailView.tsx)
├── ActionBar (ui/ActionBar.tsx) — header buttons
├── Tab strip (terminal, diff, vcs, files, logs, info, browser, +shell tabs)
├── TerminalOutput (dynamically imported, SSR disabled) — xterm.js
│   └── TerminalOutput.tsx (XtermTerminal.tsx underneath)
├── DiffViewer (DiffViewer.tsx)
├── VcsPanel (VcsPanel.tsx)
├── FilesTab (FilesTab.tsx)
├── SessionLogsTab (SessionLogsTab.tsx)
├── BrowserTab (BrowserTab.tsx) — noVNC
├── BacklogItemPanel (backlog/BacklogItemPanel.tsx) — right sidebar
├── GoalPanel (GoalPanel.tsx)
├── TagEditor (TagEditor.tsx)
├── WorkspaceSwitchModal (WorkspaceSwitchModal.tsx)
├── ResumeSessionModal (ResumeSessionModal.tsx)
└── Modal (ui/Modal.tsx) — action sheet, rename, delete confirm, restart confirm, checkpoint
```

---

## 3. CSS Patterns

All new components use **vanilla-extract** exclusively. The pattern:

```ts
// Foo.css.ts
import { style, recipe, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const card = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  selectors: {
    "&:hover": { borderColor: vars.color.borderHover },
  },
});
```

Key theme token namespaces (from `vars`):
- `vars.color.*` — `cardBackground`, `borderColor`, `borderHover`, `textPrimary`, `textSecondary`, `textMuted`, `primary`, `primaryHover`, `error`, `errorBg`, `success`, `inputBackground`, `inputBorder`, `inputFocusBorder`, `inputText`, `accentBg`, `glowSecondary`, `hoverBackground`, `textInverse`
- `vars.space.*` — spacing scale (`"2"`, `"3"`, `"4"`, etc.)
- `vars.radii.*` — `sm`, `md`, `lg`
- `vars.fontSize.*` — `xs`, `sm`, `base`, `lg`
- `vars.transition.*` — `fast`

SessionCard uses `keyframes()` for entrance animation (`cardFadeSlideIn`) with CSS custom property stagger (`--card-index` set as inline style by SessionList).

For modals/overlays, `createPortal(..., document.body)` is required (not `position: fixed` without portal) per CSS architecture rules.

Old `.module.css` files still exist for some components but new work uses `.css.ts` exclusively.

---

## 4. Grouping Strategy Implementation

Source: `web-app/src/lib/grouping/strategies.ts`

The `GroupingStrategy` enum has 9 values:

| Enum Value | Display Name | Key Field |
|---|---|---|
| `Category` | "Category" | `session.category` |
| `Tag` | "Tags" | `session.tags[]` (multi-membership) |
| `Branch` | "Branch" | `session.branch` |
| `Path` | "Path" | `session.path` |
| `Program` | "Program" | `session.program` |
| `Status` | "Status" | `session.status` (via display name) |
| `SessionType` | "Session Type" | `session.sessionType` (proto enum) |
| `Project` | "Project" | `session.projectId` |
| `None` | "None (Flat List)" | — single group |

**Algorithm** (`groupSessions`): O(n × g) where g = tags per session

1. Build `Map<groupKey, Session[]>` — Tag strategy adds sessions to multiple groups
2. Sort group keys alphabetically, with "special" fallback keys (`Uncategorized`, `Untagged`, `No Branch`, etc.) sorted to end
3. Status strategy applies a fixed priority order (Active=0 … Hibernated=6)
4. Returns `GroupedSessions[]` with `{ groupKey, displayName, sessions }`

State is persisted to localStorage via `STORAGE_KEYS.GROUPING_STRATEGY`. The `G` keyboard shortcut cycles through strategies via `cycleGroupingStrategy()`.

In **row mode**, groups and sessions are flattened into a `FlatItem[]` array before being passed to `@tanstack/react-virtual`'s `useVirtualizer`. Group headers are estimated at 40px height, session rows at 50px, with `overscan: 8`.

In **card mode**, grouping is non-virtualized — groups are rendered as `<div class={categoryGroup}>` sections with `<h3 class={categoryTitle}>` headers.

---

## 5. Tag/Badge Rendering Patterns

### Session Card Badges (header area, `badges` class)

Rendered in the `<div className={badges}>` inside `SessionCard`:

1. **External badge** — `<span className={externalBadge}>` when `session.instanceType === EXTERNAL`
2. **GitHubBadge** — `GitHubBadge.tsx` — PR number, state, draft status, review counts, CI conclusion
3. **ReviewQueueBadge** — `ReviewQueueBadge.tsx` — shown when `reviewItem` is present (from `ReviewQueueContext`)
4. **Status badge** — plain `<span className={status + " " + getStatusColor(session.status)}>` — uses `styleVariants`-like pattern from `SessionCard.css.ts`
5. **Rate limit badge** — same pattern, conditional on `session.rateLimitState !== NONE`
6. **Terminal-detected StatusBadge** — `StatusBadge.tsx` — from Redux `selectDetectedStatusMap`, terminal pattern analysis
7. **SubStatusChip** — `SubStatusChip.tsx` — from proto `session.subStatus` field (e.g., `NEEDS_APPROVAL`, `IDLE`)
8. **Memory badge** — inline `<span className={memoryBadge + " " + severityClass}>` — shows MB/GB when `session.memoryRssMb > 0`
9. **Autonomous badge** — `<span className={autonomousBadge}>` when `session.autonomousMode` is true

### Session Card Tags

```tsx
// In SessionCard body
<div className={tagsContainer}>
  <div className={tags}>
    {session.tags.map((t) => (
      <span className={tag}>{t}</span>
    ))}
  </div>
  <button className={editTagsButton}>Edit Tags / Add Tags</button>
</div>
```

Tags are stored as `repeated string tags` on the `Session` proto; persisted as a many-to-many edge to the `Tag` ent entity.

### Session Detail View Badges

In `SessionDetailView.tsx`, the header shows a `<span className={styles.statusBadge}>` and tab strip icons use Lucide React icons at `size={16}`.

---

## 6. Proto/API Shape for Sessions and Workflows

### Session Proto (`proto/session/v1/types.proto` + `session.proto`)

Key `Session` message fields (from generated types):
- `id`, `title`, `path`, `workingDir`, `branch`, `program`
- `status` (SessionStatus enum: ACTIVE=1, READY=2, LOADING=3, PAUSED=4, NEEDS_APPROVAL=5, CREATING=6, STOPPED=7, HIBERNATED=8)
- `subStatus` (SubStatus enum: UNSPECIFIED, IDLE, NEEDS_APPROVAL, WORKING, WAITING_FOR_INPUT)
- `sessionType` (SessionType enum: UNSPECIFIED=0, DIRECTORY=1, NEW_WORKTREE=2, EXISTING_WORKTREE=3)
- `instanceType` (InstanceType: MANAGED, EXTERNAL)
- `category`, `tags` (repeated string)
- `autoYes`, `autonomousMode`, `oneShot`
- `createdAt`, `updatedAt`, `lastTerminalUpdate`, `lastMeaningfulOutput` (Timestamps)
- `diffStats` (DiffStats message: added, removed)
- `gitWorktree` (GitWorktree: repoPath, worktreePath, branchName, baseCommitSha)
- `claudeSession` (ClaudeSession: sessionId, projectName)
- `goal` (SessionGoal: goalText, tasksTotal, tasksDone)
- `externalMetadata` (ExternalSessionMetadata: sourceTerminal, muxEnabled, muxSocketPath, tmuxSessionName)
- `githubPrNumber`, `githubPrUrl`, `githubOwner`, `githubRepo`, `githubPrState`, `githubPrIsDraft`, `githubApprovedCount`, `githubChangesReqCount`, `githubCheckConclusion`, `githubSourceRef`
- `rateLimitState`, `rateLimitResetTime`
- `memoryRssMb`, `pauseReason`, `prompt`, `initialPrompt`, `launchCommand`, `workingDir`
- `projectId` (for project grouping)
- `vncState` (VNCState: status)
- `historyFilePath`, `tmuxPrefix`, `clonedRepoPath`

### `ListSessionsRequest` filters
- `status` (optional SessionStatus)
- `category` (optional string)
- `hide_paused` (bool)
- `search_query` (optional string)
- `project_id` (optional string)
- `include_hidden` (bool)
- `workflow_id` (optional string)
- `include_archived` (bool)

### `WatchSessions` (server-streaming RPC)
- Streams `SessionEvent` for real-time updates
- `WatchSessionsRequest` accepts `category_filter`, `status_filter`, and `after_seq` for replay

### Workflow Proto (`WorkflowProto`)
```proto
message WorkflowProto {
  string id = 1;
  string slug = 2;           // @slug invocation in omnibar
  string name = 3;
  string description = 4;
  string command = 5;        // prompt/command with {{input}} template var
  string target_directory = 6;
  string input_template = 7;
  string session_type = 8;   // "directory", "new_worktree", etc.
  string model = 9;
  string agent_type = 10;
  string cron_expression = 11;  // standard 5-field cron
  bool cron_enabled = 12;
  Timestamp created_at = 13;
  Timestamp updated_at = 14;
}
```

Workflow RPCs: `CreateWorkflow`, `UpdateWorkflow`, `DeleteWorkflow`, `ListWorkflows`, `RunWorkflow`

`RunWorkflow` takes `{ id, arg }` and returns `{ session_id }` — it fires the workflow immediately, creating a session whose `workflow_id` is set.

---

## 7. DB Schema (ent ORM)

### Session (`session/ent/schema/session.go`)

| Field | Type | Notes |
|---|---|---|
| `title` | string (unique, not empty) | Used as session ID in most RPCs |
| `uuid` | string (optional) | Stable identifier, "" default for migration safety |
| `path` | string (not empty) | Workspace root |
| `working_dir` | string (optional) | Subdirectory override |
| `branch` | string (optional) | |
| `status` | int | SessionStatus enum value |
| `height`, `width` | int (optional) | Terminal dimensions |
| `created_at` | time (immutable) | |
| `updated_at` | time (auto-updated) | |
| `auto_yes` | bool (default false) | |
| `prompt` | string (optional) | CLI startup prompt |
| `program` | string (not empty) | e.g., "claude" |
| `existing_worktree` | string (optional) | |
| `category` | string (optional) | |
| `is_expanded` | bool (default true) | UI expansion state |
| `session_type` | string (optional) | "directory", "new_worktree", etc. |
| `tmux_prefix` | string (optional) | |
| `last_terminal_update` | time (nillable optional) | |
| `last_meaningful_output` | time (nillable optional) | |
| `last_output_signature` | string (optional) | |
| `last_added_to_queue` | time (nillable optional) | |
| `last_viewed` | time (nillable optional) | |
| `last_acknowledged` | time (nillable optional) | |
| `mcp_server_url` | string (optional) | |
| `initial_prompt` | string (optional) | Injected via CLAUDE.md once session reaches Ready |
| `one_shot` | bool (default false) | Claude -p mode |
| `last_user_response` | time (nillable optional) | Review queue |
| `processing_grace_until` | time (nillable optional) | |
| `last_prompt_detected` | time (nillable optional) | |
| `last_prompt_signature` | string (optional) | |
| `hidden` | bool (default false) | Excluded from default list |
| `pause_reason` | string (optional) | "manual", "auto:inactivity", etc. |
| `workflow_id` | string (optional) | UUID of spawning Workflow |
| `archived_at` | time (nillable optional) | Set when archived |

**Edges:**
- `worktree` → Worktree (one-to-one)
- `diff_stats` → DiffStats (one-to-one)
- `tags` → Tag (many-to-many)
- `claude_session` → ClaudeSession (one-to-one)
- `project` ← Project (many-to-one, nullable FK)
- `backlog_items` ← BacklogItem (many-to-many back-ref)
- `shells` → Shell (one-to-many)

**Indexes:** `title`, `status`, `category`, `last_meaningful_output`, `last_acknowledged`, `created_at`, `workflow_id`, `archived_at`

### Workflow (`session/ent/schema/workflow.go`)

| Field | Type | Notes |
|---|---|---|
| `id` | UUID (default: uuid.New) | Primary key |
| `slug` | string (unique, not empty) | `@slug` omnibar invocation |
| `name` | string (not empty) | Display name |
| `description` | string (optional) | |
| `command` | string (not empty) | Prompt/command with `{{input}}` |
| `target_directory` | string (optional) | Must be absolute path |
| `input_template` | string (optional) | Wraps `{{input}}` |
| `session_type` | string (optional, default "directory") | |
| `model` | string (optional) | Claude model override |
| `agent_type` | string (optional) | Program override |
| `cron_expression` | string (optional) | 5-field cron syntax |
| `cron_enabled` | bool (default false) | |
| `created_at` | time (immutable) | |
| `updated_at` | time (auto-updated) | |

**No edges** — Workflow is standalone; sessions reference it via `workflow_id` string field (not a FK edge).

**Indexes:** `slug`, `cron_enabled`, `created_at`

---

## Key Cross-Cutting Patterns

### Real-time Updates
Sessions are updated via `WatchSessions` (server-streaming ConnectRPC). The `after_seq` field allows replay of buffered events after reconnect.

### Filter Persistence
All filter/sort/grouping state is stored in `localStorage`. Keys are prefixed via `storageKeyPrefix` to support multiple simultaneous `SessionList` instances (e.g., split-pane view).

### Filtering Pipeline (frontend-only)
1. `sessions` prop (from WatchSessions) → `filteredSessions` (search + status + category + tag + hidePaused + filterNeedsApproval)
2. `filteredSessions` → `sortedSessions` (sort by lastActivity/name/createdAt/updatedAt)
3. `sortedSessions` → `groupedSessions` via `groupSessions()`
4. [row mode] `groupedSessions` → `flatItems[]` → `useVirtualizer`

### Workflow-Session Relationship
Workflows spawn sessions. When `RunWorkflow` fires, the scheduler calls `CreateSession` with `workflow_id` set. Sessions can be filtered by `workflow_id` in `ListSessionsRequest`. The `Session` ent schema stores `workflow_id` as an indexed string (not a FK edge to Workflow — Workflow has no edges).

### WorkflowForm Integration
`WorkflowForm.tsx` uses `SlashCommandDropdown` (for `/` autocomplete in the command textarea), `AutocompleteInput` (for model and program fields), and `RepoPathInput` (for target directory). The form data type is `WorkflowFormData` from `useWorkflows` hook.
