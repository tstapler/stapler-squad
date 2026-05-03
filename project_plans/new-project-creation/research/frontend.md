# Frontend Research: New Project Creation

## SESSION_TYPES Registration

**`web-app/src/components/sessions/OmnibarCreationPanel.tsx:19-24`**

```typescript
const SESSION_TYPES = [
  { value: "new_worktree", label: "New Worktree" },
  { value: "directory", label: "Directory" },
  { value: "existing_worktree", label: "Use Worktree" },
  { value: "one_off", label: "One-off" },
] as const;
```

Add `{ value: "new_project", label: "New Project" }` — insert after `new_worktree` or at the end.

**`sessionTypeMap` in `OmnibarContext.tsx`:**
```typescript
const sessionTypeMap: Record<string, SessionType> = {
  directory: SessionType.DIRECTORY,
  new_worktree: SessionType.NEW_WORKTREE,
  existing_worktree: SessionType.EXISTING_WORKTREE,
  one_off: SessionType.DIRECTORY,
};
```
Add: `new_project: SessionType.NEW_PROJECT` (once proto enum value exists).

---

## OmnibarFormState New Fields

**`web-app/src/components/sessions/Omnibar.tsx`** — add to `OmnibarFormState`:
```typescript
parentDir: string;        // base directory for new project
projectName: string;      // folder name
newProjectSessionType: "directory" | "new_worktree"; // how to open after init
```

Update `INITIAL_FORM_STATE`:
```typescript
parentDir: "",     // populated from config on mount via useEffect
projectName: "",
newProjectSessionType: "new_worktree",
```

Update `sessionType` union type to include `"new_project"`.

---

## OmnibarSessionData New Fields

Add to `OmnibarSessionData` interface:
```typescript
parentDir?: string;
projectName?: string;
isNewProject?: boolean;
```

In `handleSubmit`, for `new_project` mode:
```typescript
path: `${parentDir.trim()}/${projectName.trim()}`,
sessionType: newProjectSessionType,
isNewProject: true,
parentDir: parentDir.trim(),
projectName: projectName.trim(),
```

---

## OmnibarCreationPanel Conditional Rendering

Add after the "one_off" banner block (around line 253), following the existing pattern:
```tsx
{sessionType === "new_project" && (
  <>
    {/* Parent Directory */}
    <div className={styles.field}>
      <label htmlFor="omnibar-parent-dir">Parent Directory *</label>
      <input
        id="omnibar-parent-dir"
        type="text"
        placeholder="~/Projects"
        value={parentDir}
        onChange={(e) => setFormField("parentDir", e.target.value)}
      />
      <span className={styles.hint}>Where the new project folder will be created</span>
    </div>

    {/* Project Name */}
    <div className={styles.field}>
      <label htmlFor="omnibar-project-name">Project Name *</label>
      <input
        id="omnibar-project-name"
        type="text"
        placeholder="my-project"
        value={projectName}
        onChange={(e) => setFormField("projectName", e.target.value)}
      />
      <span className={styles.hint}>Folder name — no spaces or special characters</span>
    </div>

    {/* Resolved path preview */}
    {parentDir && projectName && (
      <div className={styles.pathPreview}>
        <code>{parentDir.replace(/\/$/, "")}/{projectName}</code>
      </div>
    )}

    {/* Session type after creation */}
    <div className={styles.field}>
      <label>Open as</label>
      <div className={styles.radioGroup}>
        {["new_worktree", "directory"].map((t) => (
          <label key={t}>
            <input
              type="radio"
              value={t}
              checked={newProjectSessionType === t}
              onChange={() => setFormField("newProjectSessionType", t)}
            />
            {t === "new_worktree" ? "New Worktree" : "Directory"}
          </label>
        ))}
      </div>
    </div>

    {/* Branch field if New Worktree chosen */}
    {newProjectSessionType === "new_worktree" && (
      <label>
        <input
          type="checkbox"
          checked={useTitleAsBranch}
          onChange={(e) => setFormField("useTitleAsBranch", e.target.checked)}
        />
        Use session name as branch name
      </label>
    )}
  </>
)}
```

---

## canSubmit Logic Update

In `Omnibar.tsx` `canSubmit` memo, add:
```typescript
else if (sessionType === "new_project") {
  if (!parentDir.trim()) return false;
  if (!projectName.trim()) return false;
  if (newProjectSessionType === "new_worktree" && !useTitleAsBranch && !branch.trim()) return false;
}
```

For `new_project` mode, skip the `detection` check — path detection is not used (user provides explicit parts).

---

## Confirmation Dialog for Directory Mode (R2)

**Existing dialog pattern**: `web-app/src/components/ui/Modal.tsx` (Radix UI based) — already used in `ResumeSessionModal.tsx`.

**Strategy**: In `Omnibar.tsx` `handleSubmit`, intercept before calling `onCreateSession`:
```typescript
const [showPathConfirmation, setShowPathConfirmation] = useState(false);
const [pendingSessionData, setPendingSessionData] = useState<OmnibarSessionData | null>(null);

// In handleSubmit, before onCreateSession call:
if (sessionType === "directory" && !pathExists) {
  setPendingSessionData(sessionData);
  setShowPathConfirmation(true);
  return;
}
```

Add confirmation modal in render:
```tsx
{showPathConfirmation && pendingSessionData && (
  <Modal open onOpenChange={() => setShowPathConfirmation(false)}>
    <ModalContent>
      <ModalTitle>Create directory?</ModalTitle>
      <ModalDescription>
        <code>{pendingSessionData.path}</code> doesn't exist.
        Create directory and initialize as a git repo?
      </ModalDescription>
      <ModalFooter>
        <ModalClose>Cancel</ModalClose>
        <button onClick={() => {
          setShowPathConfirmation(false);
          onCreateSession({ ...pendingSessionData, createIfMissing: true });
          setPendingSessionData(null);
        }}>Create</button>
      </ModalFooter>
    </ModalContent>
  </Modal>
)}
```

Add `createIfMissing?: boolean` to `OmnibarSessionData` and thread it through to the RPC call as `create_if_missing`.

**`pathExists` detection**: already available from `usePathCompletions` hook or the `detection` object's result. Confirm exact field name by reading `usePathCompletions.ts`.

---

## OmnibarContext / useSessionService Threading

**`OmnibarContext.tsx`** — extend `createSession` call:
```typescript
isNewProject: data.isNewProject,
parentDir: data.parentDir,
projectName: data.projectName,
createIfMissing: data.createIfMissing,
```

**`useSessionService.ts`** — extend RPC body:
```typescript
// Map isNewProject → sessionType: SessionType.NEW_PROJECT is set already via sessionTypeMap
createIfMissing: request.createIfMissing ?? false,
```

---

## OmnibarAction Type Extension

`web-app/src/lib/omnibar/actions/types.ts` — extend `create_session` variant:
```typescript
| { type: "create_session"; path: string; sessionType: string; branch?: string; program?: string; title?: string; parentDir?: string; projectName?: string; isNewProject?: boolean }
```

`dispatch.ts` needs no changes — the action fields flow through to `OmnibarSessionData` already.

---

## Loading Default parentDir from Config

On mount of the creation panel (or when `sessionType === "new_project"` is selected), fetch the `new_project_base_dir` from config:
```typescript
useEffect(() => {
  if (sessionType === "new_project" && !parentDir) {
    fetchConfig().then(cfg => setFormField("parentDir", cfg.newProjectBaseDir || "~/Projects"));
  }
}, [sessionType]);
```

Use the existing config fetch pattern from the app (check `useConfig` hook or settings context).

---

## Key Pitfalls

1. **`~` expansion**: frontend sends raw `~/Projects` — backend must expand tilde (already done via `NewProjectBaseDirOrDefault()`).
2. **Project name validation**: strip or reject `/`, `\`, null bytes. Show inline error before submit.
3. **Nested branch field**: already exists for top-level `new_worktree` mode. For `new_project`, conditionally render it in the `new_project` block — reuse the same `branch` / `useTitleAsBranch` form state fields.
4. **`pathExists` for directory confirmation**: verify the exact variable name in `Omnibar.tsx` — may be `detection?.localPath` exists check or a separate `pathExists` boolean from `usePathCompletions`.
5. **Skip `detection` for `new_project`**: the omnibar input detection pipeline runs on the main `input` field, but new_project uses separate `parentDir`/`projectName` fields. Ensure `canSubmit` and `handleSubmit` skip detection checks for this mode.
