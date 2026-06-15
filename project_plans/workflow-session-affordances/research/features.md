# Feature Research: Workflow-Session Affordances

## 1. Current Grouping Strategies

**Source:** `web-app/src/lib/grouping/strategies.ts`

The `GroupingStrategy` enum has **9 values**:

| Enum Value | String Value | Description |
|---|---|---|
| `Category` | `"category"` | Single-membership; uses `session.category` field |
| `Tag` | `"tag"` | Multi-membership; session appears in all its tag groups |
| `Branch` | `"branch"` | Single-membership; uses `session.branch` |
| `Path` | `"path"` | Single-membership; uses `session.path` |
| `Program` | `"program"` | Single-membership; uses `session.program` |
| `Status` | `"status"` | Single-membership; maps `session.status` to display name |
| `SessionType` | `"session_type"` | Single-membership; maps numeric type to Directory/New Worktree/Existing Worktree |
| `Project` | `"project"` | Single-membership; uses `session.projectId` |
| `None` | `"none"` | Flat list — all sessions in one "All Sessions" group |

There is **no `Workflow` grouping strategy**. Grouping by workflow is not implemented.

### Strategy is persisted to localStorage
`SessionList.tsx` reads/writes the active strategy from localStorage via `STORAGE_KEYS.GROUPING_STRATEGY` with a default of `GroupingStrategy.Category`.

### Special ordering for Status groups
The `groupSessions()` function has a custom sort for Status grouping: Active → Ready → Loading/Creating → Needs Approval → Paused → Stopped → Hibernated → Unknown.

---

## 2. How `workflow_id` Flows from DB → API → Frontend

### Database layer
**Source:** `session/ent/schema/session.go`

`Session` has a `workflow_id` string field:
```go
field.String("workflow_id").
    Optional().
    Comment("UUID of the Workflow that spawned this session, if any.")
```
An index exists on `workflow_id`. There is no foreign-key edge to the `Workflow` entity — it is stored as a bare UUID string.

The `Workflow` entity (`session/ent/schema/workflow.go`) has no back-edge to sessions — the relationship is one-way (session holds the FK).

### API layer
**Source:** `proto/session/v1/session.proto`

`Session` proto message has `string workflow_id = 62` — populated for sessions spawned by a workflow; empty string for manually-created sessions.

`ListSessionsRequest` has `optional string workflow_id = 7` for filtering sessions by their originating workflow.

`CreateSessionRequest` has `string workflow_id = 24`:
> "Set by the scheduler when firing a workflow; not intended for direct client use."

### Session service (`server/services/session_service.go`)
- **ListSessions (line 770):** Filters `inst.WorkflowID` against the optional `workflow_id` request field.
- **CreateSession (line 1022):** Passes `req.Msg.WorkflowId` → `instanceOpts.WorkflowID`.
- **Lines 3391, 3773–3785:** Archive-on-completion logic only applies when `inst.WorkflowID != ""`.

### Frontend
**Source:** `web-app/src/gen/session/v1/types_pb.ts` (generated)

The generated `Session` type has `workflowId: string` at field 62.

**`WorkflowsPanel.tsx` `RecentRuns` component** calls `listSessionsByWorkflow(workflowId, true)` which calls `ListSessions` with `workflow_id` filter, then shows the last 5 runs.

**`useSessionService.ts` (line 585):** `listSessionsByWorkflow` sends `{ workflowId }` inside `ListSessionsRequest`.

**`SessionDetailView.tsx`:** Does **not** expose `session.workflowId` — there is no "Workflow" label/badge rendered anywhere in the session detail panel.

**`SessionCard.tsx`:** Does **not** show `workflowId` — no badge or label for which workflow spawned the session.

---

## 3. Tag/Badge Rendering on Session Cards

**Source:** `web-app/src/components/sessions/SessionCard.tsx`

### Badge row (header area, `className={badges}`)
Rendered in this order:
1. **External badge** — shown when `session.instanceType === InstanceType.EXTERNAL`; displays source terminal name and mux indicator
2. **GitHubBadge** — shows PR number, state, CI check conclusion, approval counts
3. **ReviewQueueBadge** (compact) — shown when `reviewItem` prop is provided
4. **Status badge** — always shown; text from `getStatusText(session.status)`, CSS class from `getStatusColor(session.status)`
5. **Rate limit badge** — shown when `session.rateLimitState !== NONE`
6. **`StatusBadge`** (terminal-detected) — shown from `detectedStatus` prop (pattern-analysis result)
7. **`SubStatusChip`** — shown for ACTIVE sessions when `session.subStatus` is not UNSPECIFIED or IDLE
8. **Memory badge** — shown when `session.memoryRssMb > 0`; turns warning/high color above 300/500 MB
9. **Autonomous badge** — shown when `session.autonomousMode === true`

### Tags area (`className={tagsContainer}`)
Below the title row, after badges. Renders each `session.tags[]` as `<span className={tag}>`. An "Edit Tags" / "Add Tags" button opens the `TagEditor` modal.

### Category
Shown as `<span className={category}>{session.category}</span>` below the title row, above the tags container.

### CSS classes
All styles come from `./SessionCard.css` (vanilla-extract `.css.ts` file). Key class names:
- `card`, `cardPaused`, `cardExternal`, `cardDeleting`, `cardSelectMode`, `cardSelected`, `cardMemoryPressure`
- `badges`, `externalBadge`, `muxIndicator`, `status`, `statusRunning`, `statusReady`, `statusPausedDistinct`, `statusPaused`, `statusLoading`, `statusNeedsApproval`, `statusUnknown`
- `tagsContainer`, `tags`, `tag`, `editTagsButton`
- `category`, `autonomousBadge`, `memoryBadge`, `memoryBadgeWarning`, `memoryBadgeHigh`

### Notable absence
There is **no workflow badge** on `SessionCard`. A session with `workflowId` set renders identically to a manually-created session from the card's perspective.

---

## 4. WorkflowForm Fields and Form State

**Source:** `web-app/src/components/workflows/WorkflowForm.tsx`

### `WorkflowFormData` interface (from `useWorkflows.ts`)
```typescript
interface WorkflowFormData {
  slug: string;
  name: string;
  description?: string;
  command: string;
  targetDirectory: string;
  inputTemplate?: string;
  sessionType?: string;   // default "directory"; NOT rendered in the form UI
  model?: string;
  agentType?: string;
  cronExpression?: string;
  cronEnabled: boolean;
}
```

### Rendered form fields

| Field | Input Type | Required | Notes |
|---|---|---|---|
| `slug` | `<input type="text">` | Yes | Disabled in edit mode; pattern `[a-z0-9]+(-[a-z0-9]+)*`; hint: "Type @slug in the omnibar" |
| `name` | `<input type="text">` | Yes | |
| `description` | `<input type="text">` | No | |
| `command` | `<textarea>` | Yes | Supports `/` slash-command autocomplete via `SlashCommandDropdown`; hint about `{{input}}` |
| `targetDirectory` | `RepoPathInput` | Yes | |
| `inputTemplate` | `<textarea>` | No | Template wrapping user-supplied `{{input}}` |
| `model` | `AutocompleteInput` | No | Suggestions from `CLAUDE_MODELS` constant |
| `agentType` | `AutocompleteInput` | No | Labeled "Program"; suggestions from `useAvailablePrograms()` |
| `cronExpression` | `<input type="text">` | No | Standard 5-field cron syntax |
| `cronEnabled` | `<input type="checkbox">` | No | "Enable scheduled runs" |

**`sessionType` is NOT a rendered field** — it is stored in form state with default `"directory"` and passed through to the backend, but there is no UI control for it in `WorkflowForm`.

---

## 5. `WorkflowProto` Fields

**Source:** `proto/session/v1/session.proto` (lines 2254–2269)

```protobuf
message WorkflowProto {
  string id = 1;                              // UUID assigned at creation
  string slug = 2;                            // URL-safe identifier (immutable after create)
  string name = 3;                            // Human-readable name
  string description = 4;                     // Optional description
  string command = 5;                         // The command/prompt text sent to the agent
  string target_directory = 6;               // Working directory for spawned sessions
  string input_template = 7;                 // Optional template wrapping {{input}}
  string session_type = 8;                   // "directory" | "new_worktree" | etc.
  string model = 9;                          // Claude model override (e.g. "claude-sonnet-4-6")
  string agent_type = 10;                    // Program override (e.g. "claude", "aider")
  string cron_expression = 11;               // Standard 5-field cron syntax
  bool cron_enabled = 12;                    // Whether scheduled firing is active
  google.protobuf.Timestamp created_at = 13;
  google.protobuf.Timestamp updated_at = 14;
}
```

**DB schema (`session/ent/schema/workflow.go`) matches exactly** — same 12 data fields plus `id`, `created_at`, `updated_at`.

Notable: `WorkflowProto` has **no** `initial_prompt`, `tags`, `category`, `one_shot`, `auto_yes`, or `project_id` fields.

---

## 6. `initial_prompt` Field

**Source:** `session/ent/schema/session.go`, `proto/session/v1/session.proto`, frontend

### DB
`field.String("initial_prompt").Optional()` on `Session` — "Prompt typed into the session terminal once the session reaches Ready state."

### API
`CreateSessionRequest.initial_prompt = 15` — "Initial prompt to inject via CLAUDE.md (no size limit; shell-safe)."

`Session` proto message exposes `initial_prompt` as a readable field (included in `ListSessions`/`GetSession` responses).

### Frontend exposure
- **SessionDetailView** (line 1150): rendered as a "Terminal Prompt" label with monospace/pre-wrap style — **it is shown in the detail panel**.
- **SessionWizard.tsx**: Has a full form field with 10,000 char limit counter and prompt-history integration.
- **Omnibar.tsx**: Passes `initialPrompt: firstPromptText` when creating sessions.
- **`OmnibarContext.tsx`** and **`useSessionService.ts`**: Thread `initialPrompt` through to the RPC call body.

`initial_prompt` is **not surfaced on `SessionCard`** — only visible in the full `SessionDetailView`.

---

## 7. Gaps vs. Likely Requirements

| Gap | Detail |
|---|---|
| No `Workflow` grouping strategy | `GroupingStrategy` enum does not include a workflow dimension; `groupSessions()` has no `GroupingStrategy.Workflow` case |
| `workflow_id` invisible on SessionCard | Sessions spawned by a workflow have no visual indicator in the card; there's no workflow badge, label, or icon |
| `workflow_id` invisible in SessionDetailView | The session detail panel shows `initialPrompt`, `prompt`, category, tags, etc. but does **not** show which workflow spawned the session |
| `sessionType` in WorkflowForm is hard-coded to `"directory"` | The field exists in `WorkflowFormData` but is not rendered as a UI control; users cannot change it |
| `WorkflowProto` missing session-configuration fields | `initial_prompt`, `tags`, `category`, `auto_yes`, `one_shot`, `project_id`, `allowed_tools`, `permission_mode`, `autonomous_mode` are all available on `CreateSessionRequest` but absent from `WorkflowProto` |
| `WorkflowProto` missing `initial_prompt` | Workflow cannot carry an initial_prompt; it uses `command` which maps to `prompt` (the CLI arg), not the terminal-injected `initial_prompt` |
| No "Workflow" filter in ListSessions request UI | `workflow_id` filtering exists in the API but no frontend filter control surfaces it in the session list (only `WorkflowsPanel.RecentRuns` uses it) |
| `RecentRuns` limited to 5 sessions, no pagination | Shows last 5 reversed sessions; no pagination or link to a filtered view |
