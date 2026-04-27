# Implementation Plan: Squad UX Polish

**Feature**: squad-ux-polish
**Branch**: claude-squad-rich-improvemnets
**Status**: In progress — backend complete, UI + E2E remaining
**Created**: 2026-04-17
**Updated**: 2026-04-21

---

## Overview

Six stories that reduce friction for real parallel-session workflows:

1. **Prompt at session creation** — deliver an initial prompt via CLAUDE.md injection; persist recent prompts
2. **Batch session creation** — `BatchCreateSessions` RPC with bounded sequential worktree creation
3. **Review queue + one-shot PR creation** — `RunOneShot` RPC powering a "Create PR" button on the review queue
4. **Project concept** — first-class ent entity; sessions grouped by multi-select → "Group as..." flow
5. **Session creation polish** — auto-title from repo name, terminal session preset
6. **E2E tests** — ⛔ blocking gate; no merge until these pass

ADRs: `project_plans/squad-ux-polish/decisions/ADR-00{1-4}-*.md`

---

## Progress Summary

| Story | Backend | UI | Tests |
|-------|---------|-----|-------|
| S1 Prompt injection | ✅ Done | ⬜ Todo | ✅ store_test.go |
| S2 Batch create | ✅ Done | ⬜ Todo | ✅ oneshot_test.go (semaphore) |
| S3 RunOneShot / PR | ✅ Done | ⬜ Todo | ✅ oneshot_test.go (extractPRURL) |
| S4 Project grouping | ✅ Done | ⬜ Todo | — |
| S5 Creation polish | ⬜ Todo | ⬜ Todo | — |
| S6 E2E tests | — | — | ⛔ Blocking |

---

## Dependencies

```
Story 1 (Prompt Injection)
  └── no upstream story deps; foundational

Story 2 (Batch Create)
  └── depends on Story 1 (BatchSessionRequest includes initial_prompt field)

Story 3 (Review Queue / One-Shot)
  └── no upstream story deps; foundational

Story 4 (Project)
  └── no upstream story deps; foundational

Story 5 (Session Creation Polish)
  └── no upstream story deps; can run in parallel with UI tasks

Story 6 (E2E tests)
  └── depends on S1-5, S4-4, S4-5, S5-1 (UI must exist before E2E can run)
  └── S6-5 also depends on existing GitHub URL path handling (no new backend needed)

Stories 1, 3, 4, 5 can be worked in parallel.
Story 2 should follow Story 1.
```

---

## Story 1: Prompt at Session Creation

### Background

The `Instance.Prompt` field already exists and is appended to the CLI command as a quoted argument at line 1498 of `session/instance.go`. The new `InitialPrompt` mechanism replaces this with CLAUDE.md injection for robustness (no size limit, no process-list exposure). The existing `Prompt` field and CLI-append behavior is preserved for backward compatibility.

The prompt history is a new `prompts.json` file per workspace (`~/.stapler-squad/workspaces/{hash}/prompts.json`), capped at 500 entries with `UsedCount` and `LastUsed` fields.

### Tasks

---

#### TASK S1-1: PromptStore package — prompts.json CRUD ✅ DONE

**Files**: `session/prompts/store.go`, `session/prompts/store_test.go`

Implemented and tested. SHA-256 IDs, atomic writes via os.Rename, ring-buffer eviction at 500 entries.

---

#### TASK S1-2: Proto — InitialPrompt + PromptHistory RPCs ✅ DONE

`initial_prompt`, `one_shot` fields on `CreateSessionRequest`. `ListPromptHistory` / `DeletePromptHistory` RPCs. `make generate-proto` complete.

---

#### TASK S1-3: Backend — CLAUDE.md injection in instance.go ✅ DONE

`InitialPrompt` written to `<worktree>/.claude/session-prompt.md`, imported via CLAUDE.md append, added to `.gitignore`. Cleaned up in `Destroy()`.

---

#### TASK S1-4: Backend — PromptHistory RPC handlers ✅ DONE

`PromptStore` wired into `SessionService`. `ListPromptHistory` and `DeletePromptHistory` implemented. `CreateSession` calls `store.RecordUsage` after successful creation.

---

#### TASK S1-5: UI — InitialPrompt textarea + recent-prompts dropdown [Medium, 3h]

**Files**: `web-app/src/components/sessions/SessionWizard.tsx`, `web-app/src/components/sessions/SessionWizard.module.css`

**What**:
- Add `InitialPrompt` textarea to `SessionWizard` step 2 (Configuration), below the program selector
- Add a "Recent prompts" dropdown above the textarea using the existing `AutocompleteInput.tsx` pattern
  - Fetches from `ListPromptHistory` on focus (lazy load, limit 10)
  - Clicking a recent prompt populates the textarea
- Add "File" button (icon button) next to textarea; clicking opens a file picker; selected file's text content read into textarea
- Wire `initial_prompt` field to `CreateSessionRequest`
- Show character count below textarea (informational, no hard limit)
- CSS: use only tokens from `globals.css`
- Hide textarea entirely when "Terminal" program preset selected (see S5-2)

**Acceptance criteria**:
- Textarea visible on the form; does not break existing form fields
- Recent prompts dropdown shows last 10 prompts by recency
- File picker reads file content into textarea (`.txt`, `.md`)
- Submitting with an empty `InitialPrompt` omits the field (no empty string sent)

---

### Story 1 Known Issues

**Potential Bug — CLAUDE.md append idempotency [SEVERITY: Low]**
Restarting a session that already has `@.claude/session-prompt.md` in CLAUDE.md will append the import line again. Mitigation: before appending, check if the line already exists; skip if present.

---

## Story 2: Batch Session Creation

### Background

`BatchCreateSessions` is a new RPC that creates N sessions from a list of task descriptions. The server serializes `git worktree add` calls within each repo (max 3 concurrent across all repos) to prevent `.git/index.lock` corruption. Each result carries per-item success/error.

### Tasks

---

#### TASK S2-1: Proto — BatchCreateSessions RPC ✅ DONE

`BatchCreateSessions` RPC + all messages generated.

---

#### TASK S2-2: Backend — BatchCreateSessions handler with bounded pool ✅ DONE

Implemented in `server/services/session_service.go`. Semaphore (max 3), per-repo `sync.Map` mutex, partial failure isolation, title dedup. Tested in `oneshot_test.go`.

---

#### TASK S2-3: UI — Batch tab on SessionWizard [Medium, 3h]

**Files**: `web-app/src/components/sessions/SessionWizard.tsx`, `web-app/src/components/sessions/SessionWizard.module.css`

**What**:
- Add a "Batch" tab to `SessionWizard` alongside the existing single-session form
- Batch tab contains:
  - Path field (shared across all sessions)
  - Textarea: "One task per line" placeholder; max 20 lines enforced with counter `N / 20 tasks`
  - "Preview" section: renders N session title pills showing what will be created
    - Each pill shows the auto-generated title (`<repo>-XXXX` format, see S5-1)
  - Optional: "Initial prompt (all sessions)" textarea
  - Submit: "Create N sessions" (disabled if 0 or > 20 lines)
- On submit: call `BatchCreateSessions` RPC; show per-item result status:
  - Green checkmark + session title for successes (clickable → navigates to session)
  - Red X + error message for failures
  - "Cleanup failed" button if any failures (calls `DeleteSession` per failed item)

**Acceptance criteria**:
- Cannot submit with 0 or > 20 tasks
- Preview updates as user types (debounced 300ms)
- Results list shows both successes and failures clearly
- Tab accessible via keyboard navigation

---

### Story 2 Known Issues

**Potential Bug — Stale .git/index.lock on batch failure [SEVERITY: High]**
If a worktree creation goroutine is killed mid-operation, it may leave `.git/index.lock`. Mitigation: per-repo mutex plus server-side `git worktree prune` on next start.

**Potential Bug — Title collision with existing sessions [SEVERITY: Medium]**
Dedup suffix only applies within the batch. `BatchCreateResult.error` surfaces DB constraint error per item.

---

## Story 3: Review Queue + One-Shot PR Creation

### Background

The review queue UI (`ReviewQueuePanel.tsx`) and its server logic already exist. `RunOneShot` RPC executes `claude -p <prompt>` in the session's worktree, extracts a PR URL from the output, and persists it back to the session.

### Tasks

---

#### TASK S3-1: Proto — RunOneShot RPC ✅ DONE

`RunOneShotRequest`, `RunOneShotResponse` with `output`, `error`, `exit_code`, `pr_url`, `branch_diverged_from_base`. Generated.

---

#### TASK S3-2: Backend — RunOneShot handler ✅ DONE

Implemented in `server/services/session_service.go`. PR URL extraction from last 10 lines, timeout clamped to 300s, PR URL persisted to session, `SessionUpdated` event emitted after persist. `extractPRURL` tested in `oneshot_test.go`.

---

#### TASK S3-3: UI — "Create PR" button + confirmation modal in ReviewQueuePanel [Medium, 3h]

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/components/sessions/ReviewQueuePanel.module.css`

**What**:
- On session cards in `ReviewQueuePanel` where `reason == REASON_TASK_COMPLETE` and `session.github_pr_url == ""`:
  - Show "Create PR" button (primary action)
  - If `branch_diverged_from_base == true`: show warning badge "Diverged from main" next to button
- "Create PR" click opens a confirmation modal:
  - Editable textarea pre-filled with the default PR prompt
  - "Run" button → calls `RunOneShot(session_id, prompt)`
  - Spinner during execution ("Creating PR, this may take up to 30 seconds…")
  - On success: show PR URL as clickable link; close modal; session card updates with PR badge
  - On error: show error message with "Retry" option
- CSS: `ReviewQueuePanel.module.css` using only `globals.css` tokens

**Acceptance criteria**:
- Button only appears for `REASON_TASK_COMPLETE` sessions without existing PR URL
- Warning badge visible when `branch_diverged_from_base` is true
- Modal textarea is editable before running
- Spinner appears immediately on submit; button disabled during execution
- Error state is recoverable (edit prompt, retry)

---

### Story 3 Known Issues

**Potential Bug — RunOneShot output parsing false positive [SEVERITY: Low]**
Prefer last matching URL in output; only accept URLs not already stored on session.

---

## Story 4: Project Grouping

### Background

**Design revision (2026-04-21)**: The original plan had a CRUD-first flow (create Project entity upfront, then assign sessions). After reviewing Rich's actual workflow — start sessions across multiple repos, then name the group once the shape is clear — the design was revised to a select-then-group flow. Projects are auto-created when the user names a group; there is no separate CRUD panel.

### Design

```
Default SessionList view:
  [ ] frontend/auth-ui    🟢 Running
  [ ] backend/auth-api    🟢 Running
  [ ] frontend-terminal   ⬜ Ready
  [ ] infra/ci-update     ⏸ Paused

  ↓ user selects 3 checkboxes → toolbar appears:

  ┌─────────────────────────────────────────────┐
  │ 3 selected  [Group as: auth-refactor] [✕]   │
  └─────────────────────────────────────────────┘
  → Enter: Project "auth-refactor" auto-created, 3 sessions assigned

GroupBy Project active:
  ▼ auth-refactor                        3 sessions
    ├ frontend/auth-ui    🟢 Running
    ├ backend/auth-api    🟢 Running
    └ frontend-terminal   ⬜ Ready

  ▼ Ungrouped                            1 session
    └ infra/ci-update     ⏸ Paused
```

Rename/delete live inline on the group header — no separate ProjectPanel.

### Tasks

---

#### TASK S4-1: ent schema — Project entity + Session FK ✅ DONE

`session/ent/schema/project.go` created. Nullable FK edge on Session. `go generate` complete.

---

#### TASK S4-2: Backend — Project CRUD storage + RPCs ✅ DONE

`server/services/project_service.go` implements all RPCs. `session/ent_repository.go` has all storage methods. `AssignSessionsToProject` implemented.

`DeleteProject` fix (2026-04-21): wraps "clear session FKs + delete project" in a single ent transaction to prevent FK corruption.

---

#### TASK S4-3: Backend — GroupByProject strategy [Micro, 1h]

**Files**: `web-app/src/lib/grouping/strategies.ts` (frontend grouping), verify `GroupBy` enum in proto/backend

**What**:
- Add `GroupByProject` to the `GroupBy` dropdown options in `SessionList`
- Frontend grouping strategy: group sessions by `project_id`; sessions with `project_id == null` or `""` go into "Ungrouped" catch-all at the bottom
- Group key = project name (fetched from `ListProjects`); match sessions to projects client-side

**Acceptance criteria**:
- "Project" appears in GroupBy dropdown
- Sessions with no project appear under "Ungrouped"
- Group order: named projects alphabetically, "Ungrouped" last

---

#### TASK S4-4: UI — Multi-select + "Group as..." toolbar [Medium, 3h] ⟵ REVISED

**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionList.tsx`, `web-app/src/components/sessions/SessionList.module.css`

**What**:
- Add a checkbox to each `SessionCard` that appears on hover (or when any session is already selected)
- When ≥1 session selected, show a floating selection toolbar at the bottom of `SessionList`:
  - `"N selected"` count
  - `"Group as..."` text input — user types a project name and presses Enter
    - If a project with that name exists: calls `AssignSessionsToProject` with selected session IDs
    - If no project with that name exists: calls `CreateProject` first, then `AssignSessionsToProject`
  - `"Cancel"` (or Escape) clears the selection
- Shift-click for range selection (standard list behaviour)
- CSS: vanilla-extract `.css.ts` or `globals.css` tokens only

**Acceptance criteria**:
- Checkboxes appear on hover; clicking a checkbox does not activate the session
- Toolbar appears when ≥1 session is selected; disappears when selection is cleared
- "Group as..." creates project if new, assigns all selected sessions
- Entering a name that matches an existing project assigns without creating duplicate
- Escape clears selection without side effects

---

#### TASK S4-5: UI — Project group headers in SessionList [Medium, 3h]

**Files**: `web-app/src/components/sessions/SessionList.tsx`, `web-app/src/components/sessions/SessionList.module.css`

**What**:
- When `GroupByProject` is the active grouping strategy, render a group header per project:
  - Project name (bold)
  - Aggregate stat pills: `"N Running"` (green), `"N Complete"` (blue), `"N Ready for review"` (orange) — using existing status token colors
  - Inline actions on the header (right side):
    - ✏️ Rename: clicking opens the project name as an inline text input; Enter saves via `UpdateProject` RPC; Escape cancels
    - 🗑️ Delete: shows confirmation tooltip/popover — `"Remove project. N sessions will become ungrouped."` — confirm calls `DeleteProject` RPC
  - Expand/collapse toggle (chevron), consistent with existing category group behaviour
- "Ungrouped" catch-all group at the bottom; no rename/delete actions on it
- CSS: `SessionList.module.css` using only `globals.css` tokens

**Acceptance criteria**:
- "Project" option appears in GroupBy dropdown
- Group headers show correct live aggregate counts (re-fetch on `WatchSessions` events)
- Inline rename updates group header immediately (optimistic update)
- Delete confirmation shows correct session count; sessions move to "Ungrouped" on confirm
- Expand/collapse works correctly; state preserved across GroupBy strategy changes

---

#### ~~TASK S4-6: UI — ProjectPanel CRUD~~ ❌ REMOVED

Replaced by inline rename/delete directly on group headers in S4-5. No separate panel or nav item needed.

---

### Story 4 Known Issues

**Potential Bug — ent migration drop risk [SEVERITY: High]**
Verify `schema.Create()` does not pass `WithDropColumn` or `WithDropIndex` options.

**Potential Bug — Aggregate stats stale after session status change [SEVERITY: Low]**
Re-fetch project stats on every `WatchSessions` event that affects sessions in the visible project groups.

---

## Story 5: Session Creation Polish

### Background

Two gaps identified from examining the SessionWizard against Rich's actual workflow:

1. **No auto-title from repo**: the title field is blank on open; users must type something before seeing branch suggestions work correctly. Convention is `<repo-name>-<task>`, so pre-filling the repo name reduces keystrokes.
2. **No terminal session preset**: Rich wants a plain shell session alongside Claude sessions for the same repo. The `program` field already supports arbitrary programs but the UI only exposes Claude / Aider.

### Tasks

---

#### TASK S5-1: UI — Auto-title from repo basename + random suffix [Small, 2h]

**Files**: `web-app/src/components/sessions/SessionWizard.tsx`, `web-app/src/utils/sessionNameUtils.ts`

**What**:
- When the `path` field changes in `SessionWizard` step 1, and the `title` field has **not** been manually edited (use existing `editedFieldsRef` tracking):
  1. Extract the repo name from `path`:
     - Local path: `basename(path)` (e.g. `/Users/rich/projects/frontend-app` → `frontend-app`)
     - GitHub URL (detected via `isGitHubURL(path)`): parse `GitHubRef.repo` from the URL path segment (e.g. `https://github.com/org/frontend-app` → `frontend-app`). Do **not** call `basename()` on the raw URL string — the trailing segment may include `.git` or query params.
  2. Append a dash and 4 random lowercase alphanumeric chars: `Math.random().toString(36).slice(2, 6)` (e.g. `frontend-app-7f3k`)
  3. Run `generateUniqueName(suggested, existingTitles)` to guarantee no collision with existing session titles
  4. Set this as the `title` field value
- If path changes again while title is still pristine, regenerate (new random suffix)
- Once the user manually edits the title field, `editedFieldsRef` marks it dirty — stop auto-populating from that point on
- In the Batch tab (S2-3): apply the same logic per task line, using the task text as the suffix base instead of random chars (e.g. `frontend-app-add-login-ui`)

**Acceptance criteria**:
- Selecting `/path/to/my-repo` auto-fills title with `my-repo-XXXX`
- Title is editable; editing stops auto-population for the rest of the session
- Changing path while title is untouched regenerates with new suffix
- Batch preview titles follow same convention
- `generateUniqueName` prevents collision with existing session titles

---

#### TASK S5-2: UI — Terminal session preset [Small, 2h]

**Files**: `web-app/src/components/sessions/SessionWizard.tsx`, `web-app/src/components/sessions/SessionWizard.module.css`

**What**:
- Add "Terminal" as a first-class option in the program selector on step 2 (alongside Claude, Aider)
- When "Terminal" is selected:
  - Sets `program` to `$SHELL` (or `bash` as fallback) on `CreateSessionRequest`
  - Hides the `InitialPrompt` textarea and recent-prompts dropdown (not applicable)
  - Hides the `OneShot` toggle
  - Changes the step 2 header to "Terminal Configuration" (no prompt fields visible)
- Terminal sessions participate in project grouping (S4) identically to Claude sessions
- Terminal sessions appear in `SessionList` with a terminal icon instead of the Claude logo

**Acceptance criteria**:
- "Terminal" option visible and selectable in program picker
- Selecting Terminal hides prompt-related fields
- Creating a Terminal session produces a working tmux session running `$SHELL`
- Terminal sessions can be grouped with Claude sessions using the multi-select flow (S4-4)

---

## Story 6: E2E Tests ⛔ BLOCKING GATE

**This story must pass before the branch merges to main.**

Tests live in `web-app/tests/e2e/`. The existing Playwright setup (`playwright.config.ts`, `web-app/tests/e2e/`) is used. All tests require a running dev server (`make restart-web`).

---

#### TASK S6-1: E2E — Full SessionWizard creation flow [Medium, 3h]

**File**: `web-app/tests/e2e/session-create-wizard.spec.ts`

**Covers**:
1. Open SessionWizard
2. Step 1: enter a path → verify title auto-populates with `<repo>-XXXX` pattern
3. Step 2: verify branch autocomplete dropdown appears and is populated
4. Step 2: enter a branch name, select program (Claude)
5. Submit → verify new session card appears in `SessionList` with correct title
6. Verify session status transitions to Running within timeout

**Acceptance criteria**:
- Test passes against a local dev server with a real git repo in a temp directory
- Title auto-generation assertion uses regex `/<repo-basename>-[a-z0-9]{4}/`
- Branch autocomplete: at least one suggestion appears when the repo has local branches

---

#### TASK S6-2: E2E — Omnibar creation flow [Small, 2h]

**File**: `web-app/tests/e2e/session-create-omnibar.spec.ts`

**Covers**:
1. Open Omnibar (keyboard shortcut or nav button)
2. Type a local path → verify path completion suggestions appear
3. Select a path → verify session creation form pre-filled
4. Submit → session appears in list

**Acceptance criteria**:
- Path completions appear within 500ms of typing
- Selecting a suggestion populates the path field
- Session created successfully end-to-end

---

#### TASK S6-3: E2E — Title auto-generation behaviour [Small, 1h]

**File**: `web-app/tests/e2e/session-title-autogen.spec.ts`

**Covers**:
1. Open SessionWizard, select a path → title auto-populates
2. Verify format: `<basename>-[a-z0-9]{4}`
3. Change path → title regenerates (new suffix)
4. Manually edit the title → then change the path → title does NOT regenerate (dirty flag)
5. Check `useTitleAsBranch` checkbox → edit title → verify the branch name preview field updates to match the new title in real time
6. Uncheck `useTitleAsBranch` → verify the branch input field becomes independently editable (no longer mirrors title)

**Acceptance criteria**:
- All 6 scenarios covered as described
- Scenario 5/6 assert the actual value of the branch input element, not just its visibility

---

#### TASK S6-4: E2E — Project grouping flow [Medium, 3h]

**File**: `web-app/tests/e2e/project-grouping.spec.ts`

**Covers**:
1. Create 2 sessions (via API or UI)
2. Multi-select both via checkboxes
3. Verify selection toolbar appears
4. Type project name in "Group as..." input, press Enter
5. Switch `GroupBy` to "Project" → verify group header with project name appears
6. Both sessions appear under the group header
7. Click rename (pencil icon) → type new name → Enter → header updates
8. Click delete (trash icon) → confirm → sessions move to "Ungrouped"

**Acceptance criteria**:
- All 8 steps verified with Playwright assertions
- Group header aggregate counts are correct after each operation

---

#### TASK S6-5: E2E — GitHub URL creation flow [Small, 2h]

**File**: `web-app/tests/e2e/session-create-github-url.spec.ts`

**Covers**:
1. Open SessionWizard step 1
2. Type a GitHub URL into the path field (e.g. `https://github.com/owner/my-repo`)
3. Verify the title auto-populates as `my-repo-XXXX` (repo name extracted from URL, not URL basename)
4. Verify the session type switches to `new_worktree` (not `directory`)
5. Submit → verify the created session has `GitHubOwner == "owner"` and `GitHubRepo == "my-repo"` visible in the session detail view or via API assertion
6. Verify no `.git` suffix or query-param artifact appears in the auto-generated title

**Acceptance criteria**:
- Title regex: `/^my-repo-[a-z0-9]{4}$/`
- Session created with correct GitHub metadata (assert via `GetSession` RPC or session card)
- Session type is `new_worktree` for GitHub URL input

---

## Functionality Preservation Checklist

All new features (S4–S5) and the E2E suite (S6) **must not regress** the following existing SessionWizard paths. Each is a distinct conditional in the current creation logic:

| Path | Condition | Must still work |
|------|-----------|-----------------|
| GitHub URL clone | `isGitHubURL(path) == true` | Clones repo to temp dir, sets `GitHubOwner`/`GitHubRepo` on session |
| New worktree | `sessionType == "new_worktree"` | `git worktree add` in existing local repo |
| Existing worktree | `sessionType == "existing_worktree"` | Attaches to a worktree that already exists on disk |
| Directory session | `sessionType == "directory"` | Runs directly in the given directory; no worktree created |
| `useTitleAsBranch` on | checkbox checked (default) | Branch name = sanitised title; branch input mirrors title edits |
| `useTitleAsBranch` off | checkbox unchecked | Branch name field is independent; user types custom branch |
| Custom program | program field set to arbitrary binary | Preserved; not overwritten by "Terminal" preset or defaults |
| `resumeId` path | `resume_id` set in request | Session resumes from saved state; no new worktree created |
| `OneShot` flag | `one_shot == true` | `RunOneShot` called immediately after session creation |
| Defaults resolution chain | path has a `.claude/defaults.json` | Directory defaults override global config; `SourceBadge` shows origin |
| `InjectHookConfig` | hook config present | Failure is non-fatal; session still created |
| Working directory fallback | `path` is empty or missing | Falls back to current working directory; does not crash |
| Multi-select + session activation | session selected via checkbox | Clicking checkbox does NOT open the session; clicking the card row does |

Add one E2E assertion per row if not already covered by S6-1 through S6-5 (can be lightweight `it.skip` stubs marking the gap for follow-up).

---

## Testing Strategy (Updated)

### Test Pyramid

**Unit tests** (fast, isolated, in-process) — already written:
- `session/prompts/store_test.go`: CRUD, ring-buffer eviction, SHA-256 ID stability, atomic write
- `server/services/oneshot_test.go`: `extractPRURL` (10 cases), `BatchCreateSessions` semaphore + dedup

**Integration tests** (real tmux, real git, real ent/SQLite):
- `InitialPrompt` injection: start session with prompt, verify CLAUDE.md in worktree
- `BatchCreateSessions`: 3 sessions same repo, verify no `.git/index.lock`, all 3 worktrees exist
- `RunOneShot`: trivial one-shot, verify output captured and PR URL field updated

**UI component tests** (`web-app/src/components/sessions/__tests__/`):
- `SessionWizard` prompt textarea: visible, wired to request, clears on reset
- `SessionWizard` batch tab: task count validation, preview rendering, result display
- `ReviewQueuePanel` Create PR button: renders for `TASK_COMPLETE`, warning badge for diverged, modal opens/closes
- `SessionWizard` title auto-generation: path change triggers suggestion, edit marks dirty

**E2E tests** (`web-app/tests/e2e/`) — ⛔ **blocking**:
- S6-1: Full wizard creation flow
- S6-2: Omnibar creation flow
- S6-3: Title auto-generation behaviour (incl. useTitleAsBranch)
- S6-4: Project grouping flow
- S6-5: GitHub URL creation flow

### Quality Gates

All new handlers must pass:
- `make lint` (golangci-lint; failures block build)
- `make nil-safety` (nilaway analysis)
- `make test` (all packages)
- `make build`
- `npm run test` in `web-app/`
- **E2E tests** (`npm run test:e2e` in `web-app/`) — required before merge

---

## Proto Codegen Sequence

Each story touching `session.proto` requires:
```
make generate-proto
make build
make test
```

All proto changes for S1–S4 are **already complete**. No further proto changes needed.

---

## Known Issues (Summary)

| # | Issue | Severity | Story | Status |
|---|-------|----------|-------|--------|
| 1 | `git worktree add` stale `.git/index.lock` on batch failure | High | S2 | Open — per-repo mutex mitigates; `git worktree prune` on restart |
| 2 | CLAUDE.md import line appended twice on restart | Low | S1 | Open — check before appending |
| 3 | `gh pr create --json` not supported | Low | S3 | Open — parse stdout URL instead |
| 4 | Diverged branch at PR creation | Medium | S3 | Open — pre-check + warning badge in S3-3 |
| 5 | ent migration drop risk | High | S4 | Open — verify `schema.Create()` opts |
| 6 | `claude` binary not on PATH in server process | Medium | S3 | Open — `exec.LookPath` check exists |
| 7 | Title collision with existing sessions in batch | Medium | S2 | Open — surfaced per-item in results |
| 8 | DeleteProject race with active session update | Medium | S4 | ✅ Fixed 2026-04-21 — ent transaction |
| 9 | RunOneShot: session stuck TASK_COMPLETE after PR | Low | S3 | ✅ Fixed 2026-04-21 — event emitted |

---

## Implementation Sequence (Remaining Work)

All backend is complete. Remaining work is UI + E2E tests.

**Phase D — UI** (can be parallelised across developers):
1. S5-1: Auto-title from repo basename in SessionWizard
2. S5-2: Terminal session preset in SessionWizard
3. S1-5: InitialPrompt textarea + recent-prompts dropdown in SessionWizard
4. S2-3: Batch tab in SessionWizard
5. S3-3: Create PR button + modal in ReviewQueuePanel
6. S4-3: GroupByProject strategy (frontend grouping logic)
7. S4-4: Multi-select checkboxes + "Group as..." selection toolbar
8. S4-5: Project group headers with inline rename/delete

**Phase E — E2E tests** ⛔ blocking merge:
9. S6-1: Full wizard creation flow
10. S6-2: Omnibar creation flow
11. S6-3: Title auto-generation behaviour (incl. useTitleAsBranch)
12. S6-4: Project grouping flow
13. S6-5: GitHub URL creation flow
