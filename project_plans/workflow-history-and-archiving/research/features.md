# Feature Inventory: Workflow History and Archiving

## US-1: Run Workflow from Workflows Page

`WorkflowsPanel.tsx` renders a `▶ Run` button per workflow row. Clicking it calls `handleRun`, which sets a `runningId` loading state and calls `useSessionService().runWorkflow({ id: wf.id })`. The hook issues a `RunWorkflow` RPC, which internally calls `Scheduler.FireNow`. The button shows `…` while `runningId === wf.id` and re-enables once the promise resolves.

## US-2: Recent Runs Accordion

A `RecentRuns` sub-component is embedded in a second `<tr>` spanning all columns beneath each workflow row. It holds local `expanded` / `runs` / `loading` state. On first expand, it calls `listSessionsByWorkflow(workflowId, true)` (include archived), takes the last 5 sessions reversed, and renders each as a row with a colored status badge, a `<Link href="/?session=id">` title, and a formatted timestamp. The empty state renders "No runs yet." while the loading state shows a muted spinner text.

## US-3: Archive / Unarchive Sessions

Two RPCs: `ArchiveSession` sets `inst.ArchivedAt = &now`; `UnarchiveSession` sets `inst.ArchivedAt = nil`. Both call `storage.SaveInstances` to persist the change and return an empty success response. `ListSessions` applies the filter at line 701: `if inst.ArchivedAt != nil && !req.Msg.IncludeArchived { continue }`. The frontend hooks `archiveSession(id)` and `unarchiveSession(id)` in `useSessionService.ts` wrap the RPC calls with error dispatch to the Redux store.

## US-4: Auto-Archive Workflow Sessions on Completion

`SessionService.wireAutoArchiveCallback` registers an `autoArchiveListener` on every instance with a non-empty `WorkflowID`. The listener's `OnLifecycleEvent` fires `go l.svc.maybeAutoArchive(l.inst)` when it receives `EventExited`. `maybeAutoArchive` guards against double-archive (`inst.ArchivedAt != nil`), sets `inst.ArchivedAt = &now`, and saves. The goroutine dispatch ensures the lifecycle event path is never blocked.

## US-5: Workflow ID Linkage

`workflow_id` is a plain string field on Session. `Scheduler.FireNow` injects `WorkflowId: wf.ID.String()` into the `CreateSessionRequest`. `SessionService.CreateSession` assigns `WorkflowID: req.Msg.WorkflowId` on the new `Instance`. `ListSessions` supports `WorkflowId` as an optional pointer filter at line 706. The field propagates through `InstanceData` serialization and the ent repository unchanged.

## US-6: Pause Kills tmux to Free Memory

`Instance.Pause()` calls `i.KillSession()` after stopping the controller and committing any dirty worktree changes. The kill releases the tmux process and all child processes, freeing RAM. The Claude session UUID has already been persisted by `wireClaudeSessionIDSavedCallback` before this point, so the conversation can be resumed. If kill fails, the code falls back to `pm().DetachSafely()` and logs a warning; the failure is non-fatal.

## US-7: Resume Re-initializes tmux with --resume Flag

`Instance.Resume()` detects a dead tmux session via `!i.pm().IsAlive()`. In the dead-tmux branch it reads the current `claudeSession.ConversationUUID`, calls `buildLaunchCommand(claudeSessionID)` to inject `--resume <uuid>`, constructs a fresh `tmux.TmuxSession` object via `tb.TmuxManager().SetSession(...)`, sets the `STAPLER_SESSION_UUID` env var, and calls `pm().Start(worktreePath)`. This ensures the relaunched Claude process resumes the correct conversation even after a server restart.
