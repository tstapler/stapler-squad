# Omnibar Architecture Research — Quick Workflows

**Date:** 2026-06-11  
**Branch:** stapler-squad-quick-workflows  
**Purpose:** Understand the existing omnibar detection, action, and session-creation systems to design the `@slug` workflow trigger without conflicts.

---

## 1. Detector System

### 1.1 Core Interfaces

**File:** `web-app/src/lib/omnibar/detector.ts`

```typescript
export interface Detector {
  name: string;
  priority: number;
  detect(input: string): DetectionResult | null;
}
```

`DetectorRegistry` holds a sorted list of `Detector` instances (sorted ascending by `priority`, so lower number = checked first). Two public methods:

| Method | Behaviour |
|---|---|
| `detect(input)` | Returns the first non-null `DetectionResult`, or `{ type: InputType.Unknown, confidence: 0, ... }` if no detector matches |
| `detectAll(input)` | Returns every non-null result from every detector (all matches, not just first) |

A singleton registry is created by `createDefaultRegistry()` and cached by `getDefaultRegistry()`. The top-level export `detect(input)` calls `getDefaultRegistry().detect(input)`.

### 1.2 Registered Detectors (in priority order)

| Priority | Class | Trigger Pattern | `InputType` returned |
|---|---|---|---|
| 5 | `CommandDetector` | Input starts with `>` | `InputType.Command` or `InputType.SpawnShell` |
| 10 | `GitHubPRDetector` | `https://github.com/owner/repo/pull/N` | `InputType.GitHubPR` |
| 20 | `GitHubBranchDetector` | `https://github.com/owner/repo/tree/branch` | `InputType.GitHubBranch` |
| 30 | `GitHubRepoDetector` | `https://github.com/owner/repo` | `InputType.GitHubRepo` |
| 35 | `NewSessionDetector` | `new/<anything>` (case-insensitive) | `InputType.NewSession` |
| 40 | `GitHubShorthandDetector` | `owner/repo` or `owner/repo:branch` | `InputType.GitHubShorthand` |
| 50 | `PathWithBranchDetector` | `/path@branch` or `~/path@branch` | `InputType.PathWithBranch` |
| 100 | `LocalPathDetector` | `/absolute`, `~/`, `./`, `../`, or multiple slashes | `InputType.LocalPath` |
| 200 | `SessionSearchDetector` | Everything else (catch-all) | `InputType.SessionSearch` |

**Key guard conditions:**

- `GitHubShorthandDetector` explicitly skips inputs starting with `/`, `~`, or `.` to avoid clashing with path detectors.
- `PathWithBranchDetector` skips inputs without `@` or that contain `://`.
- `LocalPathDetector` skips inputs containing `@` (without `://`) to leave them to `PathWithBranchDetector`.
- `CommandDetector` returns a low-confidence `InputType.Unknown` for unrecognized `>` commands (not `null`), effectively consuming that input.

### 1.3 Slash Command Pre-processor

**File:** `web-app/src/lib/omnibar/parseSlashCommand.ts`

Before `detect()` is called, the Omnibar runs `parseSlashCommand()` against the raw input. This intercepts `/oneoff`, `/worktree`, `/dir`, `/existing`, `/project` prefixes and rewrites them to a session type before the path detectors can misidentify them.

```
Known slash commands → session type mapping:
  /oneoff     → "one_off"
  /worktree   → "new_worktree"
  /dir        → "directory"
  /existing   → "existing_worktree"
  /project    → "new_project"
```

Only `[a-z]+` command names are matched; the rest of the input is passed to `detect()` as the remainder.

### 1.4 `DetectionResult` shape

```typescript
interface DetectionResult {
  type: InputType;
  confidence: number;        // 0.0–1.0
  parsedValue: string;
  suggestedName: string;     // Pre-filled into the "Session Name" field
  localPath?: string;
  branch?: string;
  gitHubRef?: GitHubRef;
  metadata?: Record<string, unknown>;  // CommandDetector uses this
}
```

The `metadata` field is how `CommandDetector` passes `commandType` and `commandArg` to the submit handler. A `WorkflowDetector` can use the same mechanism to pass `workflowSlug` and `workflowArg`.

---

## 2. OmnibarAction Union

**File:** `web-app/src/lib/omnibar/actions/types.ts`

```typescript
export type OmnibarAction =
  | { type: "navigate_session"; sessionId: string; label: string }
  | { type: "create_session"; path: string; sessionType: string; branch?: string; program?: string; title?: string }
  | { type: "clone_session"; sourceSessionId: string; sourcePath: string; sourceProgram: string; label: string }
  | { type: "pause_session"; sessionId: string; label: string }
  | { type: "resume_session"; sessionId: string; label: string }
  | { type: "delete_session"; sessionId: string; label: string }
  | { type: "set_theme"; themeName: ThemeName }
  | { type: "spawn_shell"; sessionId?: string; workingDir?: string; shellCommand?: string }
```

**Dispatch pattern** (`dispatch.ts`): An exhaustive `switch` on `action.type`. TypeScript enforces that every union variant has a case — a missing case causes a compile error. Each case calls a dep from `ActionDeps` (navigate, createSession, pauseSession, etc.) and then calls `deps.close()`.

**Important observation:** `dispatchOmnibarAction` is the fire-and-forget path used by suggestion-list clicks. The `handleSubmit` function in `Omnibar.tsx` is the path used by the creation form. Both paths converge on `deps.createSession(OmnibarSessionData)`.

---

## 3. Claimed Trigger Patterns

The following input patterns are fully claimed and must not be reused:

| Pattern | Claimed by | Notes |
|---|---|---|
| `https?://github.com/...` | GitHub detectors (pr 10/20/30) | All GitHub URLs |
| `new/<anything>` | `NewSessionDetector` (pr 35) | Case-insensitive |
| `owner/repo` or `owner/repo:branch` | `GitHubShorthandDetector` (pr 40) | Two bare words with `/` between them |
| `/absolute/path` | `LocalPathDetector` (pr 100) | Starts with `/` |
| `~/path` | `LocalPathDetector` (pr 100) | Starts with `~/` |
| `./path` or `../path` | `LocalPathDetector` (pr 100) | Starts with `.` |
| `/path@branch` | `PathWithBranchDetector` (pr 50) | Contains `@` without `://` |
| `>command` | `CommandDetector` (pr 5) | VS Code-style command palette |
| `/oneoff`, `/worktree`, `/dir`, `/existing`, `/project` | `parseSlashCommand` pre-processor | Slash-prefixed mode shortcuts (consumed before detect) |
| `<bare text>` | `SessionSearchDetector` (pr 200) | Everything else is a session search |

**Safe prefix characters for a new trigger:**

| Prefix | Status | Notes |
|---|---|---|
| `@` | **SAFE** | Not used by any detector or pre-processor. `@` in paths only appears in `PathWithBranchDetector` which requires the portion before `@` to look like a path, and the portion after to look like a branch name. `@workflow-slug` has no `/` and no path prefix, so it does not match that detector. |
| `wf:` | **SAFE** | Not used. Colon suffix within a bare-text token is not claimed. |
| `!` | **SAFE** | Not used. |
| `#` | **SAFE** | Not used. |
| `>@` | collision risk | Would require `>` prefix which is already claimed by `CommandDetector`. |

**Winner: `@slug [optional-arg]`** — see Section 6 for justification.

---

## 4. Session Creation Flow

### 4.1 Full data path

```
User types in omnibar input
    ↓
parseSlashCommand(input) → sets formState.sessionType if slash command
    ↓
detect(remainderOrInput) → returns DetectionResult
    ↓
Omnibar.tsx handleSubmit()
    ↓
builds OmnibarSessionData {
  title, path, branch, program, category, prompt, autoYes,
  sessionType, existingWorktree, workingDir,
  oneOff, initialPrompt, isNewProject, createIfMissing
}
    ↓
props.onCreateSession(data)  ← this is handleCreateSession in OmnibarContext.tsx
    ↓
OmnibarContext.handleCreateSession(data)
    → resolves effectiveSessionType (handles isNewProject flag)
    → calls createSession(request) from useSessionService.ts
    ↓
useSessionService.createSession(Partial<CreateSessionRequest>)
    → calls ConnectRPC createSession RPC
    → dispatches upsertSession to Redux store
    ↓
Server: session_service.go CreateSession handler
```

### 4.2 OmnibarSessionData (the transfer object)

```typescript
interface OmnibarSessionData {
  title: string;
  path: string;
  branch?: string;
  program: string;
  category?: string;
  prompt?: string;
  autoYes: boolean;
  gitHubOwner?: string;
  gitHubRepo?: string;
  gitHubPRNumber?: number;
  sessionType?: "directory" | "new_worktree" | "existing_worktree";
  existingWorktree?: string;
  workingDir?: string;
  oneOff?: boolean;
  initialPrompt?: string;
  isNewProject?: boolean;
  createIfMissing?: boolean;
}
```

**For Quick Workflows:** A detected workflow would populate `title` (workflow name + arg), `path` (workflow's `targetDirectory`), `sessionType` (workflow's `sessionType`), `oneOff` (if workflow's sessionType is `one_off`), and `initialPrompt` (the interpolated `inputTemplate`).

### 4.3 OmnibarContext.tsx sessionTypeMap

The context maps frontend string keys to proto `SessionType` enum values:

```typescript
const sessionTypeMap: Record<string, SessionType> = {
  directory: SessionType.DIRECTORY,
  new_worktree: SessionType.NEW_WORKTREE,
  existing_worktree: SessionType.EXISTING_WORKTREE,
  one_off: SessionType.DIRECTORY,      // server distinguishes via oneOff flag
  new_project: SessionType.NEW_PROJECT,
};
```

A workflow using `sessionType: "one_off"` already works through the existing `one_off` path. No new proto enum value is needed for the basic case.

---

## 5. Omnibar UI Components

### 5.1 Omnibar.tsx — detection flow and form state

`Omnibar.tsx` is the top-level modal. Key behaviours:

- **Two modes:** `discovery` (search/navigate existing sessions) and `creation` (configure + launch a new session).
- Detection runs in a 150 ms debounce effect: `parseSlashCommand → detect → dispatchMode → auto-fill sessionName + branch`.
- The `modeState` machine (from `useModeReducer`) drives mode transitions. The key mode relevant here is `creation_with_repo` which pre-fills the path.
- On detection of `InputType.NewSession`, mode transitions to `creation_with_repo` with the query as the pre-fill. A `WorkflowDetector` returning a new `InputType.Workflow` type could trigger a similar transition — or the detection effect could be extended to handle `InputType.Workflow` by switching to creation mode and pre-populating all workflow fields.

**`OmnibarFormState`** (the full form, maintained inside Omnibar):

```typescript
interface OmnibarFormState {
  sessionName, branch, program, category, autoYes, useTitleAsBranch,
  sessionType: "directory"|"new_worktree"|"existing_worktree"|"one_off"|"new_project",
  existingWorktree, workingDir,
  parentDir, projectName, newProjectSessionType, createIfMissing, firstPrompt
}
```

For workflows: `sessionName` ← workflow display name + arg, `sessionType` ← workflow's sessionType, `workingDir` or path ← workflow's `targetDirectory`, `firstPrompt` ← interpolated `inputTemplate`.

### 5.2 OmnibarCreationPanel.tsx — the creation form

`OmnibarCreationPanel` is the form rendered in creation mode. It reads `formState` and calls `setFormField`. Session type is selected via a radio group (`SESSION_TYPES` array). The "First Prompt" textarea maps directly to `OmnibarFormState.firstPrompt`, which flows to `OmnibarSessionData.initialPrompt`.

For workflows: the panel needs no changes for basic workflow invocation — the workflow detector populates form fields the panel already renders. The session type radio would show the pre-selected type from the workflow definition. If desired, the slug + arg display could be shown in the read-only path display slot (currently only used for `creation_with_repo` mode).

### 5.3 Suggestion rendering

Currently, the omnibar shows suggestions in `OmnibarResultList` (sessions + repo history). Workflow suggestions need to appear in discovery mode when the user types `@`. Options:

1. **Extend `OmnibarResultList`** to accept a `workflowResults` prop and render a "Workflows" section.
2. **Inline in Omnibar.tsx** as a new conditional section, like repo history is currently inlined.

Option 1 is cleaner. The result list already has session-results and repo-history sections with clean separation.

---

## 6. Recommendation: `@slug [arg]` Syntax

### 6.1 Justification

`@workflow-slug` (e.g. `@knowledge-sync https://example.com`) is the correct trigger syntax because:

1. **No conflicts:** `@` is not used by any existing detector or pre-processor. The only existing use of `@` in the codebase is `PathWithBranchDetector` for `path@branch`, and that detector explicitly requires the portion before `@` to start with `/` or `~` (a path-like prefix) — `@knowledge-sync` starts with `@` itself, which is neither `/` nor `~`, so it will never be claimed as a path-with-branch.
2. **Semantic clarity:** In developer tooling, `@` conventionally means "at this thing" or "invoke this named entity." Users familiar with mention syntax in Slack/GitHub will immediately understand `@workflow-name`.
3. **Clean separation from search fallback:** `@keyword` will not pollute `SessionSearchDetector`'s results because the `WorkflowDetector` runs at a lower priority number than 200 and returns non-null.
4. **No ambiguity with `new/`:** The `new/` prefix is reserved for the `NewSessionDetector` (priority 35), which only fires on `new/<text>`. `@slug` does not start with `new/`.

### 6.2 Priority slot

A `WorkflowDetector` should run at **priority 25** — after the GitHub URL detectors (10/20/30 would still run at 10, 20 but `@slug` won't match them) and before the `NewSessionDetector` (35). Since `@slug` will never match GitHub URL patterns, this priority is a formality; it could also be placed at priority 45 without issue. Priority 25 is chosen to keep workflow detection near the top of the chain, reflecting its intentional, high-confidence nature.

---

## 7. WorkflowDetector Sketch

```typescript
// web-app/src/lib/omnibar/detectors/WorkflowDetector.ts

import { Detector } from "../detector";
import { DetectionResult, InputType } from "../types";

export interface WorkflowEntry {
  slug: string;
  name: string;
  description?: string;
  targetDirectory: string;
  sessionType: "directory" | "one_off" | "new_worktree" | "existing_worktree";
  inputTemplate?: string; // e.g. "/knowledge:synthesize {{input}}"
}

/**
 * WorkflowDetector — matches the "@slug [optional-arg]" omnibar syntax.
 * Priority 25: runs after GitHub URL detectors (10–30) but before NewSession (35).
 *
 * Input examples:
 *   @knowledge-sync                       → workflow slug only
 *   @knowledge-sync https://example.com  → slug + argument to inject into template
 */
export class WorkflowDetector implements Detector {
  name = "WorkflowDetector";
  priority = 25;

  private readonly PREFIX_RE = /^@([a-zA-Z0-9_-]+)(?:\s+(.+))?$/;

  constructor(private readonly workflows: WorkflowEntry[]) {}

  detect(input: string): DetectionResult | null {
    const trimmed = input.trim();
    const match = trimmed.match(this.PREFIX_RE);
    if (!match) return null;

    const [, slug, arg] = match;
    const workflow = this.workflows.find(
      (w) => w.slug.toLowerCase() === slug.toLowerCase()
    );

    if (!workflow) {
      // Unknown @slug — still surface as a low-confidence result so the UI can
      // show "no matching workflow" feedback without falling through to SessionSearch.
      return {
        type: InputType.Workflow,  // new enum value needed — see Section 8
        confidence: 0.4,
        parsedValue: trimmed,
        suggestedName: slug,
        metadata: { workflowSlug: slug, workflowArg: arg ?? "", workflowFound: false },
      };
    }

    const interpolatedPrompt = workflow.inputTemplate
      ? workflow.inputTemplate.replace("{{input}}", arg ?? "")
      : arg ?? "";

    return {
      type: InputType.Workflow,
      confidence: 1.0,
      parsedValue: trimmed,
      suggestedName: workflow.name + (arg ? `: ${arg.slice(0, 40)}` : ""),
      localPath: workflow.targetDirectory,
      metadata: {
        workflowSlug: slug,
        workflowArg: arg ?? "",
        workflowFound: true,
        workflow,
        interpolatedPrompt,
      },
    };
  }
}
```

### 7.1 New `InputType` value needed

Add to `web-app/src/lib/omnibar/types.ts`:

```typescript
export enum InputType {
  // ... existing values ...
  Workflow = "workflow",
}
```

And add a display entry in `INPUT_TYPE_INFO`:

```typescript
[InputType.Workflow]: {
  label: "Workflow",
  icon: "⚡",
  description: "Quick workflow invocation",
},
```

### 7.2 Registration in `createDefaultRegistry()`

The detector needs a dynamic list of workflows from the backend. Two approaches:

**Option A — Registry factory with injected list (preferred):**  
Pass the workflow list at registry construction time. Since the registry is currently a singleton, this requires either:
- Making `createDefaultRegistry()` accept an optional `workflows` argument, OR
- Having the `Omnibar` component or `OmnibarContext` create a registry instance (not the singleton) with workflows injected after fetching them from the API.

**Option B — Static empty list with dynamic refresh:**  
Register `WorkflowDetector` with an empty workflow list initially, then replace the instance when workflows load. Simpler but slightly messier.

Option A is cleaner. Since the omnibar already uses the singleton via `import { detect } from "@/lib/omnibar"`, the simplest path is to add a `useWorkflowDetector` hook that registers/unregisters a `WorkflowDetector` instance on the shared registry when workflows load:

```typescript
// Called once in OmnibarProvider or Omnibar component
useEffect(() => {
  if (!workflows.length) return;
  const detector = new WorkflowDetector(workflows);
  const registry = getDefaultRegistry();
  registry.register(detector);
  return () => registry.unregister(detector); // needs unregister() method
}, [workflows]);
```

This requires adding `unregister(detector: Detector)` to `DetectorRegistry`.

### 7.3 New OmnibarAction variant

The workflow invocation bypasses the creation form entirely (the form is pre-filled and auto-submitted, or the user reviews before submitting). Add to the union:

```typescript
| { type: "run_workflow"; workflowSlug: string; workflowArg: string; label: string }
```

In `dispatch.ts`, add a case:

```typescript
case "run_workflow":
  // Build OmnibarSessionData from the workflow definition + arg
  void deps.runWorkflow(action.workflowSlug, action.workflowArg);
  deps.close();
  return;
```

`ActionDeps` needs a `runWorkflow` dep. `OmnibarContext` provides it by looking up the workflow definition, building `OmnibarSessionData`, and calling `createSession`.

---

## 8. Summary of Required Changes

| Layer | Change | File(s) |
|---|---|---|
| Types | Add `InputType.Workflow` enum value + `INPUT_TYPE_INFO` entry | `web-app/src/lib/omnibar/types.ts` |
| Detector | New `WorkflowDetector` class at priority 25 | `web-app/src/lib/omnibar/detectors/WorkflowDetector.ts` |
| Detector registry | `unregister()` method + registration of `WorkflowDetector` | `web-app/src/lib/omnibar/detector.ts` |
| Action union | New `run_workflow` action type | `web-app/src/lib/omnibar/actions/types.ts` |
| Dispatch | `case "run_workflow":` + `runWorkflow` dep | `web-app/src/lib/omnibar/actions/dispatch.ts` |
| Omnibar.tsx | Handle `InputType.Workflow` in detection effect; pre-fill form fields from workflow metadata | `web-app/src/components/sessions/Omnibar.tsx` |
| OmnibarContext.tsx | `runWorkflow` handler; workflow fetch; `sessionTypeMap` if needed | `web-app/src/lib/contexts/OmnibarContext.tsx` |
| Result list | `workflowResults` section in discovery mode | `web-app/src/components/sessions/OmnibarResultList.tsx` |
| Feature testing registry | Add `WorkflowDetector` tests, dispatch tests | Per `.claude/rules/feature-testing-registry.md` |

Backend changes (proto, Go handler, storage) are covered in a separate research doc.

---

## 9. Conflict-Free Verification

The `@slug` prefix passes all existing detector guard conditions:

| Detector | Will it fire on `@knowledge-sync`? | Reason |
|---|---|---|
| `CommandDetector` | No | Does not start with `>` |
| `GitHubPRDetector` | No | Does not match `https?://github.com/...` |
| `GitHubBranchDetector` | No | Same |
| `GitHubRepoDetector` | No | Same |
| `NewSessionDetector` | No | Does not start with `new/` |
| `GitHubShorthandDetector` | No | Does not match `[a-zA-Z0-9_-]+/[a-zA-Z0-9_.-]+` (no `/` after the slug) |
| `PathWithBranchDetector` | No | Does not include `@` after a path prefix; the whole token starts with `@`, which is neither `/` nor `~` nor `.` — and the regex requires `(.+)@(branch)` where the LHS must be a non-empty path |
| `LocalPathDetector` | No | Does not start with `/`, `~/`, `./`, `../`, and does not have multiple slashes |
| `SessionSearchDetector` | Would fire (priority 200) | `WorkflowDetector` at priority 25 intercepts first — `SessionSearch` never sees it |

For `@knowledge-sync https://example.com`:  
The GitHub URL detectors (priorities 10/20/30) look for `https://` at the **start** of the input. This input starts with `@`, so none of them match. `PathWithBranchDetector` requires the `@` to appear within a path string (e.g. `/path@branch`). This input's `@` is at position 0, so it does not match that detector. The `WorkflowDetector` at priority 25 matches cleanly.
