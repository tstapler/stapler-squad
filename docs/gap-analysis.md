# Feature Gap Analysis

Tracked by the `gap-finder` workflow. Each entry has a status and a fingerprint used
for deduplication across runs. Add `status: fixed` when resolved — the workflow will
skip it in future rounds.

---

## Known Gaps

### GAP-001 — Five PR management RPCs have no frontend caller
- **status**: open
- **severity**: high
- **fingerprint**: GAP-PROTO-PR-NoFrontendCaller
- **file**: `web-app/src/lib/features/features/pr.ts`
- **detail**: All five PR RPCs (`GetPRInfo`, `GetPRComments`, `PostPRComment`, `MergePR`, `ClosePR`) are defined in the SessionService proto and the generated TypeScript client (web-app/src/gen/session/v1/session_pb.ts), and the feature registry entries for pr-close, pr-get-comments, pr-get-info, pr-merge, and pr-post-comment all have empty `componentPaths` arrays. No hook, context, or component in web-app/src/ calls `getPRInfo`, `getPRComments`, `postPRComment`, `mergePR`, or `closePR`. The backend is fully implemented but there is no frontend surface to exercise these RPCs.
- **fix hint**: Create a `PRPanel` or `PRTab` component under `web-app/src/components/sessions/` that calls the PR RPCs via a `usePRService` hook, and add it to the session detail view. Update the feature registry `componentPaths` fields for all five pr-* features.

### GAP-002 — `TriggerSync` RPC is registered but always returns `CodeUnimplemented`
- **status**: open
- **severity**: high
- **fingerprint**: GAP-STUB-BacklogService-TriggerSync
- **file**: `server/services/backlog_service.go`
- **detail**: The `BacklogService.TriggerSync` method (line 1571) is registered in the `BacklogService` proto interface and always returns `connect.NewError(connect.CodeUnimplemented, ...)`. The underlying `SourceSyncEvent` entity is fully generated (`session/ent/sourcesyncevent/`), the sync loop in `session/backlog_sync.go` actively writes `SourceSyncEvent` rows, and `CreateSourceSyncEvent` is implemented in `ent_repository_backlog.go`. The storage and ent layers are ready but the RPC was never implemented.
- **fix hint**: Implement `TriggerSync` by calling the existing backlog sync loop. The sync logic lives in `session/backlog_sync.go`. Wire a reference to the sync loop (or its storage backend) into `BacklogService` and invoke it for the specified `source_id`, then return the resulting `SourceSyncEvent` as a proto response.

### GAP-003 — Workflow `session_type` field ignored by scheduler except for `one_off`
- **status**: open
- **severity**: high
- **fingerprint**: GAP-SCHED-FireNow-session_type
- **file**: `server/workflows/scheduler.go`
- **detail**: `FireNow` builds a `CreateSessionRequest` but never maps `wf.SessionType` to the proto `SessionType` enum. The only session-type logic is `oneOff := wf.SessionType == session.SessionTypeOneOff` at line 156; all other values (`'new_worktree'`, `'existing_worktree'`, `'new_project'`) are silently discarded. Because `SessionType` is left at its zero value (`SESSION_TYPE_UNSPECIFIED`), `resolveSessionType` in `session_service.go` always falls through to `SessionTypeDirectory`, so any workflow configured with `session_type='new_worktree'` creates a plain directory session instead of a worktree-backed one.
- **fix hint**: In `FireNow`, add a switch on `wf.SessionType` to populate `req.Msg.SessionType` before calling `CreateSession`. Map `'directory'` → `SESSION_TYPE_DIRECTORY`, `'new_worktree'` → `SESSION_TYPE_NEW_WORKTREE`, `'existing_worktree'` → `SESSION_TYPE_EXISTING_WORKTREE`, `'new_project'` → `SESSION_TYPE_NEW_PROJECT`, and keep the `oneOff` bool path unchanged.

### GAP-004 — `workflowMetaCache` never refreshed after workflow CRUD — new workflows immediately auto-archived despite `archive_after_hours` setting
- **status**: open
- **severity**: high
- **fingerprint**: GAP-SCHED-workflowMetaCache-stale-after-crud
- **file**: `server/services/session_service.go`
- **detail**: The field comment at line 167 states the cache is 'Populated on startup and refreshed every minute', but no periodic ticker exists. `refreshWorkflowMetaCache` is called exactly once — inside `SetWorkflowRepository` at server startup. When a workflow is created after the server starts (via `CreateWorkflow` RPC), its entry is absent from `workflowMetaCache`. `maybeAutoArchive` checks `if ok && meta.archiveAfterHours > 0` to decide whether to defer archival; when `ok` is false (cache miss), the condition is false and the session is immediately archived — even if the workflow was configured with `archive_after_hours > 0`. Additionally, `workflowNames()` (called in `ListSessions` via `InstanceToProto`) will return an empty name for new workflows until server restart, causing session cards to fall back to the UUID label.
- **fix hint**: Call `s.refreshWorkflowMetaCache(context.Background())` at the end of each `CreateWorkflow`, `UpdateWorkflow`, and `DeleteWorkflow` handler in `workflow_service.go` (the service already holds `s.repo`), OR add a periodic ticker in `SetWorkflowRepository` as the comment implies. The simplest single-commit fix is to add the refresh call in each mutating RPC handler.

### GAP-005 — `SESSION_STATUS_RESTORING` defined in proto but missing from generated Go and TypeScript bindings
- **status**: open
- **severity**: high
- **fingerprint**: GAP-PROTO-SessionStatus-RESTORING-notGenerated
- **file**: `gen/proto/go/session/v1/types.pb.go`
- **detail**: ADR-018 added SESSION_STATUS_RESTORING = 9 to proto/session/v1/types.proto and wired StatusToProto/StatusStringToProto in server/adapters/instance_adapter.go to return sessionv1.SessionStatus_SESSION_STATUS_RESTORING. However, make generate-proto was never run: the Go generated file (gen/proto/go/session/v1/types.pb.go) ends the SessionStatus enum at HIBERNATED = 8 with no RESTORING entry, causing build failures with 'undefined: sessionv1.SessionStatus_SESSION_STATUS_RESTORING'. The TypeScript generated file (web-app/src/gen/session/v1/types_pb.ts) likewise ends at HIBERNATED = 8, so SessionStatus.RESTORING in SessionCard.tsx and SessionDetailView.tsx evaluates to undefined at runtime, silently breaking the Restoring UI state.
- **fix hint**: Run 'make generate-proto' to regenerate bindings from the updated proto/session/v1/types.proto. This will add SESSION_STATUS_RESTORING = 9 to both the Go and TypeScript generated enums, fixing the build error and the runtime undefined comparison.

### GAP-006 — `ForkSession` goroutine omits all lifecycle wiring before starting the forked session
- **status**: open
- **severity**: high
- **fingerprint**: GAP-FORK-ForkSession-missingWiring
- **file**: `server/services/session_service.go`
- **detail**: The ForkSession handler's async goroutine (around line 2704) calls newInst.Start(true) but never calls wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback, SetStatusManager, StartController, StartSessionDriver, or backlogLifecycleListener.WireToInstance. Compare with CreateSession's async goroutine (lines 1163-1210) which calls all of these. As a result, forked sessions have no idle detection, no rate-limit recovery, no Claude session ID tracking, no auto-archive callback, and their initial prompt is never delivered.
- **fix hint**: In the ForkSession goroutine, add the same wiring sequence used by CreateSession after newInst.Start() succeeds: call s.wireRateLimitCallbacks(newInst), s.wireStatusChangeCallback(newInst), s.wireClaudeSessionIDCallback(newInst), s.wireAutoArchiveCallback(newInst), and then s.statusManager.SetStatusManager + StartController (if statusManager is set), then session.StartSessionDriver(newInst, ...) and s.backlogLifecycleListener.WireToInstance(newInst).

### GAP-007 — `resumeFromHibernation` does not restart the controller after re-launching the session process
- **status**: open
- **severity**: high
- **fingerprint**: GAP-HIBERNATE-resumeFromHibernation-noController
- **file**: `session/instance_hibernate.go`
- **detail**: The Hibernated→Active state machine After hook calls i.resumeFromHibernation(ctx) in a goroutine. This function calls i.Start(false), but Start() explicitly defers controller startup: it documents 'Controller startup is always deferred to the caller after wiring (SetStatusManager)'. No code in resumeFromHibernation or its callers subsequently calls SetStatusManager or StartController. As a result, sessions that resume from hibernation have no idle detection, no ClaudeController for sub-status, and no rate-limit tracking — all of which were working before hibernation.
- **fix hint**: In resumeFromHibernation, after i.Start(false) succeeds, add the same controller-wiring logic used in loadInstancesWithWiring or CreateSession: call i.SetStatusManager(statusManager) and i.StartController(). The Instance needs a reference to the StatusManager — either store it on Instance before hibernating (already done via SetStatusManager) or accept it as a parameter in resumeFromHibernation.

### GAP-008 — AutonomousMode flag is never written to the ent database — lost on server restart
- **status**: open
- **severity**: high
- **fingerprint**: GAP-DB-AutonomousMode-not-persisted
- **file**: `session/ent_repository.go`
- **detail**: The `AutonomousMode` field (`autonomous_mode` on `InstanceData`) is defined in the legacy JSON `InstanceData` struct (storage.go line 65) but there is no corresponding column in the ent schema (`session/ent/schema/session.go` has `one_shot` and `hidden` but no `autonomous_mode`). As a result, the ent repository `AddInstance`/`UpdateInstance` methods never call `SetAutonomousMode()` and `sessionToInstanceData` never reads it back. Sessions that have `autonomous_mode=true` set via `UpdateSession` or `CreateSession` will have the flag survive only in memory; after a server restart the flag defaults to `false` and the autonomous driver is not restarted.
- **fix hint**: Add `field.Bool("autonomous_mode").Default(false).Comment("When true, an AutonomousDriver injects orchestrator prompts when the session is idle.")` to `session/ent/schema/session.go`, run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`, then map it in `ent_repository.go` (write in AddInstance/UpdateInstance, read back in sessionToInstanceData).

### GAP-009 — `SessionWizard` component is unreachable: not imported by any page or route
- **status**: open
- **severity**: high
- **fingerprint**: GAP-UI-SessionWizard-orphaned
- **file**: `web-app/src/components/sessions/SessionWizard.tsx`
- **detail**: `SessionWizard.tsx` defines a multi-step session creation form with `data-testid`s (`session-title`, `session-path`, `create-session-button`) but has zero imports in the entire `web-app/src` tree. The `/?new=true` route now opens the Omnibar (via `openOmnibar()` in `page.tsx`), not the Wizard. The e2e tests in `tests/e2e/session-create-wizard.spec.ts` expect 'Basic Information' and 'Repository Setup' step headings and the `session-path` testid — none of which exist in the Omnibar — so those tests will always fail. The Wizard also contained the only UI for selecting a named profile on session creation.
- **fix hint**: Either re-connect `SessionWizard` to a route (e.g. `/sessions/new/wizard`) and update the e2e tests to target that URL, or delete `SessionWizard.tsx` and its CSS file and rewrite `session-create-wizard.spec.ts` to test the Omnibar flow.

### GAP-010 — `session-create-wizard.spec.ts` e2e tests target UI that no longer exists
- **status**: open
- **severity**: high
- **fingerprint**: GAP-E2E-wizard-tests-target-removed-component
- **file**: `web-app/tests/e2e/session-create-wizard.spec.ts`
- **detail**: All four tests in `SessionWizard creation flow` (S6-1) navigate to `/?new=true` and then wait for 'Basic Information' text or the `session-path` testid. The `/?new=true` handler in `page.tsx` calls `openOmnibar()` — it no longer renders `SessionWizard`. The wizard's testids (`session-title`, `session-path`, `create-session-button`) and text ('Basic Information', 'Repository Setup') are absent from the Omnibar, so every assertion will timeout. These tests are silently broken in CI.
- **fix hint**: Update `session-create-wizard.spec.ts` to test the Omnibar creation flow using the selectors found in the existing `session-create-omnibar.spec.ts`, or delete the file if the wizard is fully replaced.

### GAP-011 — Cron workflow scheduler never sets `one_shot` mode — sessions never auto-exit
- **status**: open
- **severity**: high
- **fingerprint**: GAP-WORKFLOW-FireNow-no-one-shot
- **file**: `server/workflows/scheduler.go`
- **detail**: The `FireNow` function (lines 167–174) builds a `CreateSessionRequest` but never sets the `OneShot` field. The `one_shot` field is fully implemented in `CreateSessionRequest` (field 16) and the session driver (which adds the `-p` flag to run Claude in print mode, causing it to exit when the task completes). Without `one_shot: true`, every cron-triggered workflow spawns an interactive Claude session that sits idle after completing its initial prompt, accumulating indefinitely. The ent workflow schema and `WorkflowProto` have no `one_shot` field, so this cannot be configured at all. `maybeAutoArchive` only archives sessions after they *stop*; it does not cause them to stop.
- **fix hint**: Add `one_shot bool` to `ent/schema/workflow.go` and `WorkflowProto` / `CreateWorkflowRequest` / `UpdateWorkflowRequest` protos; expose a checkbox in `WorkflowForm.tsx`; then pass `OneShot: wf.OneShot` in the `FireNow` `CreateSessionRequest`.

### GAP-012 — `ResumeHibernatedSession` never re-adds session to review-queue poller or rewires service callbacks
- **status**: open
- **severity**: high
- **fingerprint**: GAP-LIFECYCLE-ResumeHibernated-poller-rewire
- **file**: `server/services/session_service.go`
- **detail**: HibernateSession explicitly calls s.removeFromAllPollers(instance.Title) (line 1520), which removes the session from the review queue poller. ResumeHibernatedSession (lines 1537–1577) loads a fresh instance from s.storage.LoadInstances() — NOT the live instance from the poller — then calls instance.ResumeFromHibernation(ctx). It never calls s.reviewQueuePoller.AddInstance(), s.wireRateLimitCallbacks(), s.wireStatusChangeCallback(), or s.wireClaudeSessionIDCallback() on the freshly loaded instance. After the async process restart completes, the session is invisible to the review queue, publishes no rate-limit or status-change events via the service event bus, and is excluded from WatchSessions streaming updates until a server restart.
- **fix hint**: After calling instance.ResumeFromHibernation(ctx) succeeds, wire callbacks (wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback), set the status manager (instance.SetStatusManager), and re-add to the poller (s.reviewQueuePoller.AddInstance(instance)). Mirror the pattern in loadInstancesWithWiring + the CreateSession async goroutine.

### GAP-013 — `ForkSession` does not inject `MCPServerURL` into the forked instance before `Start()`
- **status**: open
- **severity**: high
- **fingerprint**: GAP-FORK-ForkSession-MCPServerURL-missing
- **file**: `server/services/session_service.go`
- **detail**: CreateDirectorySession and CreateSession both set MCPServerURL on InstanceOptions so the forked session receives `--mcp-config '…'` on launch. ForkSession calls src.ForkFromCheckpoint(…) which builds InstanceOptions without MCPServerURL (the method has no access to s.mcpServerURL), and the ForkSession handler never patches newInst.MCPServerURL after receiving the instance. As a result, every session started via ForkSession launches Claude without the MCP server URL, so all MCP tool calls (set_session_goal, approve_tool, etc.) will fail with connection errors.
- **fix hint**: After the newInst, err := src.ForkFromCheckpoint(…) call and before s.storage.AddInstance(newInst), add: newInst.MCPServerURL = s.mcpServerURL. Matches the pattern at line 1106 in CreateSession.

### GAP-014 — `fork_source_id` combined with `SESSION_TYPE_NEW_WORKTREE` causes `--resume` to fail due to JSONL path mismatch
- **status**: open
- **severity**: high
- **fingerprint**: GAP-FORK-ForkSourceId-worktree-resume-path-mismatch
- **file**: `server/services/session_service.go`
- **detail**: When CreateSession receives fork_source_id, it forks the JSONL into filepath.Dir(srcPath) — which is ~/.claude/projects/<encoded-original-path>/. It then sets req.Msg.ResumeId to the new UUID. The guard at line 1068 that forces SessionTypeDirectory only fires when ForkSourceId == ''. So if the client passes SESSION_TYPE_NEW_WORKTREE (which the history fork UI allows), the session runs in a new worktree with a different path. Claude's --resume <uuid> then searches for the JSONL under ~/.claude/projects/<encoded-NEW-worktree-path>/ and finds nothing. The history/page.tsx fork dialog allows selecting NEW_WORKTREE (line 216).
- **fix hint**: At line 1068, remove the '&& req.Msg.ForkSourceId == ""' condition so that fork_source_id sessions also force SessionTypeDirectory: `if req.Msg.ResumeId != "" && req.Msg.SessionType == sessionv1.SessionType_SESSION_TYPE_UNSPECIFIED { sessionType = session.SessionTypeDirectory }`. Alternatively, copy the forked JSONL into the new worktree's Claude projects directory before starting.

### GAP-015 — `Header.tsx` is never imported in the running app — orphaned since CockpitShell refactor
- **status**: open
- **severity**: high
- **fingerprint**: GAP-NAV-Header-orphaned
- **file**: `web-app/src/components/layout/Header.tsx`
- **detail**: The app layout.tsx renders CockpitShell, which contains DrawerNav (desktop sidebar) and BottomNav (mobile bar). Header.tsx is only imported in ConditionalHeader.tsx, which itself is never imported anywhere in the application. The Header component has not been rendered in production since the CockpitShell refactor (cf5b8380). MemoryNavBadge was subsequently added to the dead Header, making it permanently inaccessible.
- **fix hint**: Either delete Header.tsx and ConditionalHeader.tsx and migrate their unique features (WorkspaceSwitcher, DebugMenu, ApprovalNavBadge, ApprovalDrawer, ConnectionIndicator) into DrawerNav or BottomNav, or re-import ConditionalHeader into CockpitShell or layout.tsx to restore the original header.

### GAP-016 — `WorkspaceSwitcher` is only rendered inside the dead `Header` — workspace switching is inaccessible
- **status**: open
- **severity**: high
- **fingerprint**: GAP-NAV-WorkspaceSwitcher-orphaned
- **file**: `web-app/src/components/layout/WorkspaceSwitcher.tsx`
- **detail**: WorkspaceSwitcher is imported and rendered exclusively in Header.tsx (line 117). Since Header.tsx is never mounted in the running application, there is no UI surface from which a user can switch between workspaces. DrawerNav and BottomNav have no workspace switching functionality.
- **fix hint**: Add WorkspaceSwitcher to DrawerNav (e.g. at the bottom of the nav list) or to CockpitShell so it is reachable without restoring the old Header.

### GAP-017 — `ApprovalDrawer` and `ApprovalNavBadge` are dead — quick tool-call approval is inaccessible from any page
- **status**: open
- **severity**: high
- **fingerprint**: GAP-NAV-ApprovalDrawer-orphaned
- **file**: `web-app/src/components/sessions/ApprovalDrawer.tsx`
- **detail**: ApprovalDrawer (a slide-out panel to approve/deny Claude tool uses) and ApprovalNavBadge (showing pending approval count) are only imported in Header.tsx. SessionDetailView.tsx (line 619) has a comment saying 'ApprovalPanel removed — approvals now handled in the global ApprovalDrawer in Header', but since Header is never rendered, there is no way to approve tool calls from within a session detail view. Approvals can still be handled via the /notifications page, but the in-context quick-approval flow is broken.
- **fix hint**: Move ApprovalDrawer rendering to CockpitShell and render ApprovalNavBadge in DrawerNav or BottomNav, matching the original intent that approvals be globally accessible from any page.

### GAP-018 — `HibernationSweeper` auto-hibernates sessions without stopping the `AutonomousDriver` goroutine
- **status**: open
- **severity**: high
- **fingerprint**: GAP-HIBERNATE-sweeper-no-driver-stop
- **file**: `session/hibernation_sweeper.go`
- **detail**: sweep() (line 237) and sweepResourcePressure() (line 354) both call `inst.Hibernate(ctx)` directly on the Instance. This transitions the session state and kills the tmux process, but the HibernationSweeper has no reference to the SessionService or its driverRegistry. Any running AutonomousDriver goroutine registered in driverRegistry is left alive after the tmux process is killed. The driver's next attempt to `SendCommandImmediate` to the now-dead controller will fail, causing the driver to exit with a 'stuck' outcome and emit a spurious failure notification to the user. Contrast with the HibernateSession RPC (session_service.go:1505) which correctly calls `s.stopAndDeregisterDriver(instance.Title)` before hibernating. server/server.go constructs NewHibernationSweeper with no driver stopper argument and no way to wire one in.
- **fix hint**: Add a DriverStopper interface (with a single `StopDriver(sessionTitle string)` method) to the session package. Thread it into HibernationSweeper via a `SetDriverStopper` method or a constructor argument. Call `stopper.StopDriver(inst.Title)` before `inst.Hibernate(ctx)` in both sweep() and sweepResourcePressure(). Wire the SessionService as the implementation in server/server.go after both are constructed.

### GAP-019 — Retention enforcer and `ArchiveWorkflowSessions` both archive Hibernated workflow sessions
- **status**: open
- **severity**: high
- **fingerprint**: GAP-RETENTION-Hibernated-not-excluded
- **file**: `server/workflows/retention.go`
- **detail**: Both the automated retention sweep (`runRetentionSweep` in retention.go lines 82–86 and 106–110) and the manual `ArchiveWorkflowSessions` RPC (session_service.go line 4065) use `StatusNotIn(Active, Creating, Paused)` to guard against archiving live sessions. Neither excludes `session.Hibernated` (DB value 4). A hibernated workflow session — one that has been checkpointed and has its tmux session killed, but is still resumable — will be silently archived by the hourly retention sweep or by clicking 'Archive Sessions' in the WorkflowsPanel, permanently preventing resumption.
- **fix hint**: Add `int(session.Hibernated)` to the `StatusNotIn(...)` call in both `runRetentionSweep` (retention.go, two places) and `ArchiveWorkflowSessions` (session_service.go). Also add it to the in-memory poller guard on session_service.go line 4077 (`!inst.IsHibernated()`).

### GAP-020 — `ResumeHibernatedSession` skips all callback wiring and review-queue re-add
- **status**: open
- **severity**: high
- **fingerprint**: GAP-RESUME-HIBERNATE-missing-wiring
- **file**: `server/services/session_service.go`
- **detail**: After calling instance.ResumeFromHibernation() (which asynchronously calls Start()), the handler does not call wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback, SetStatusManager, StartController, or reviewQueuePoller.AddInstance. CreateSession, CreateDirectorySession, and loadInstancesWithWiring all perform this wiring. As a result, a resumed session has no idle detection controller (so the review queue never sees it as idle/needing attention), rate-limit auto-resume is dead, and the session never appears in the review queue until the server restarts.
- **fix hint**: After instance.ResumeFromHibernation returns (or inside the Hibernated→Active after-hook), call s.wireRateLimitCallbacks(instance), s.wireStatusChangeCallback(instance), s.wireClaudeSessionIDCallback(instance), s.wireAutoArchiveCallback(instance), instance.SetStatusManager(s.statusManager), instance.StartController(), and s.reviewQueuePoller.AddInstance(instance) — mirroring the CreateSession async goroutine.

### GAP-021 — `ForkSession` goroutine skips all post-Start wiring (status manager, rate-limit callbacks, controller)
- **status**: open
- **severity**: high
- **fingerprint**: GAP-FORK-SESSION-missing-wiring
- **file**: `server/services/session_service.go`
- **detail**: At line 2704, ForkSession spawns a goroutine that calls newInst.Start(true) but never calls wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback, SetStatusManager, StartController, or StartSessionDriver. CreateSession (line 1163) and CreateDirectorySession (line 611) both perform all these wiring steps. A forked session therefore starts with no idle detection (SubStatus/WorkingState always UNSPECIFIED), no rate-limit recovery, no Claude session-ID tracking, and no initial-prompt injection — all features that require the wired controller.
- **fix hint**: In the ForkSession goroutine (after Start() returns), add the same wiring block that CreateSession uses: wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback, then SetStatusManager + StartController + StartSessionDriver.

### GAP-022 — `ProfilesManager` silently clears `envVars` and `cliFlags` on every profile save
- **status**: open
- **severity**: high
- **fingerprint**: GAP-CONFIG-ProfilesManager-envVars-cliFlags-data-loss
- **file**: `web-app/src/components/settings/ProfilesManager.tsx`
- **detail**: The `handleSave` function at line 144–145 hardcodes `envVars: {}` and `cliFlags: ""` in the `upsertProfile` call. The `ProfileFormData` interface and form have no fields for these, so they are never loaded into state during `handleEdit` either. Any profile that already has `env_vars` or `cli_flags` set (via config.json or API) will have those values destroyed the moment a user edits and saves it through the Settings UI.
- **fix hint**: Add `envVars` and `cliFlags` to `ProfileFormData`, load them from `profile.envVars` and `profile.cliFlags` in `handleEdit`, render env-var table and CLI-flags input in the form (same pattern as `GlobalDefaultsForm`), and pass them through in the `upsertProfile` call.

### GAP-023 — UpdateProject ignores req.Msg.Id and looks up project by new name, breaking renames
- **status**: open
- **severity**: high
- **fingerprint**: GAP-PROJECT-UpdateProject-id-ignored
- **file**: `server/services/project_service.go`
- **detail**: In `UpdateProject` (lines 85–104 of project_service.go) the handler builds `session.ProjectData{Name: req.Msg.Name, ...}` and passes it to `storage.UpdateProject`, which calls `r.client.Project.Query().Where(project.Name(data.Name)).Only(ctx)` in ent_repository.go line 1399. The lookup key is the NEW name, not the existing ID. The frontend (`SessionList.tsx` line 278) calls `updateProject({ id: projectId, name: trimmed })` where `id` is the current project name and `name` is the desired new name. Because the backend looks up by the new name (which doesn't exist yet), any rename attempt returns a not-found error. The `req.Msg.Id` field is never read by the handler.
- **fix hint**: In `UpdateProject`, read `req.Msg.Id` as the lookup key: pass it separately to the storage layer (e.g. `storage.UpdateProjectByID(ctx, req.Msg.Id, data)`) and update `EntRepository.UpdateProject` to accept an explicit ID/name parameter for the WHERE clause, using `data.Name` only for the new value to set.

### GAP-024 — BacklogItem.user_modified_fields is read by sync loop but never written by UpdateBacklogItem
- **status**: open
- **severity**: high
- **fingerprint**: GAP-SCHEMA-BacklogItem-user_modified_fields-never-written
- **file**: `session/ent_repository_backlog.go`
- **detail**: The `user_modified_fields` column on the `backlog_items` ent schema is the local-wins mechanism in `session/backlog_sync.go` (line 209): when the sync loop receives an external update, it calls `parseUserModifiedFields(existing.UserModifiedFields)` and skips any field already in that set. However, `UpdateBacklogItem` (and `TransitionBacklogItemStatus`) in `ent_repository_backlog.go` never call `SetUserModifiedFields()`. This means every user edit via the service layer leaves `user_modified_fields` as an empty JSON array, so the very next external sync cycle will silently overwrite the user's changes to title, description, and priority.
- **fix hint**: In `UpdateBacklogItem`, detect which fields in the `BacklogItemUpdate` struct are non-nil and append their field names to the existing `UserModifiedFields` slice before calling `SetUserModifiedFields()` on the ent update builder. A helper like `mergeUserModifiedFields(existing, newFields []string) []string` already pattern-matches what `parseUserModifiedFields` expects.

### GAP-025 — maybeAutoArchive immediately archives workflow sessions when only keepSessions is set, defeating count-based retention
- **status**: open
- **severity**: high
- **fingerprint**: GAP-WORKFLOW-maybeAutoArchive-ignores-keepSessions
- **file**: `server/services/session_service.go`
- **detail**: When a workflow session stops, maybeAutoArchive (line 4141) skips immediate archival only when archiveAfterHours > 0 (so the retention enforcer handles it). However, when a workflow has keepSessions > 0 and archiveAfterHours == 0, maybeAutoArchive immediately archives every stopped session, leaving 0 completed sessions for the retention enforcer to count and preserve. The workflowMeta struct (line 173) caches only name and archiveAfterHours — it does not cache keepSessions — so maybeAutoArchive cannot even check the field. The net effect is that keepSessions is silently ignored unless archiveAfterHours is also set to a non-zero value.
- **fix hint**: Add keepSessions int to the workflowMeta struct, populate it in refreshWorkflowMetaCache, and update maybeAutoArchive to also skip immediate archival when meta.keepSessions > 0 (deferring to the retention enforcer's count-based sweep).

### GAP-026 — `ResumeHibernatedSession` orphans the session from the live poller
- **status**: open
- **severity**: high
- **fingerprint**: GAP-Resume-PollerNotRewired
- **file**: `server/services/session_service.go`
- **detail**: `HibernateSession` explicitly calls `removeFromAllPollers` to evict the session from `reviewQueuePoller`. `ResumeHibernatedSession` calls `instance.ResumeFromHibernation` and publishes an event, but never calls `s.reviewQueuePoller.AddInstance(instance)`. After resume, the session's `Instance` object is Active but absent from the poller's internal list. This means `ListSessions` (poller path), `UpdateSession` (poller path), terminal monitoring via `checkSessions`, and review-queue detection all fail to see the resumed session until the next server restart. The `reconcileSessions` loop only reconciles sessions already in the list — it has no logic to re-discover an instance removed via `RemoveInstance`.
- **fix hint**: In `ResumeHibernatedSession`, after saving and publishing the event, add: `if s.reviewQueuePoller != nil { s.reviewQueuePoller.AddInstance(instance) }` — mirroring the pattern used in `CreateSession`.

### GAP-027 — Resolved session defaults for cli_flags, env_vars, and tags are silently dropped during CreateSession
- **status**: open
- **severity**: high
- **fingerprint**: GAP-DEFAULTS-CLIFlags-EnvVars-Tags-not-applied
- **file**: `server/services/session_service.go`
- **detail**: config.ResolveDefaults returns CLIFlags, EnvVars, and Tags fields in addition to Program and AutoYes. CreateSession (around line 1053) only consumes resolved.Program and resolved.AutoYes — the CLIFlags, EnvVars, and Tags are never passed to session.InstanceOptions or applied anywhere. The session.Instance struct has no CLIFlags or EnvVars fields at all. All three have full UI surfaces in GlobalDefaultsForm.tsx and ProfilesManager.tsx, are stored via UpdateGlobalDefaults/UpsertProfile, and are returned by GetSessionDefaults/ResolveDefaults — but they have no effect on any session.
- **fix hint**: For tags: pass resolved.Tags into instanceOpts.Tags (InstanceOptions already has a Tags field). For cli_flags: add a CLIFlags field to InstanceOptions and append it to the program string in buildLaunchCommand (instance_tmux.go). For env_vars: add an EnvVars field to Instance/InstanceOptions and pass them as tmux new-session -e KEY=VALUE arguments.

### GAP-028 — CreateSessionRequest.profile and skip_defaults are never forwarded by useSessionService
- **status**: open
- **severity**: high
- **fingerprint**: GAP-UI-CreateSession-profile-not-forwarded
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: The server `CreateSession` handler at `server/services/session_service.go` reads `req.Msg.Profile` and `req.Msg.SkipDefaults` to apply named profiles via `config.ResolveDefaults`. The proto `CreateSessionRequest` defines these as fields 11 and 12. However, `useSessionService.ts`'s `createSession` callback explicitly enumerates fields to pass to the RPC and omits both `profile` and `skipDefaults`. The Omnibar context (`OmnibarContext.tsx`) calls this hook exclusively, so profile-based session creation from the Omnibar is silently a no-op even if a profile selector were added.
- **fix hint**: In `useSessionService.ts`'s `createSession` callback, add `profile: request.profile ?? ""` and `skipDefaults: request.skipDefaults ?? false` to the `clientRef.current.createSession({...})` call body. Then propagate `profile` from `OmnibarContext.tsx`'s `createSession` call and add a profile dropdown to `OmnibarCreationPanel.tsx` (the form state field already exists in `SessionWizard.tsx` as a reference).

### GAP-029 — `WorkflowProto` has no `permission_mode` field — cron sessions always use default permissions
- **status**: open
- **severity**: high
- **fingerprint**: GAP-WORKFLOW-FireNow-permission_mode
- **file**: `server/workflows/scheduler.go`
- **detail**: The `WorkflowProto` message, `CreateWorkflowRequest`, `UpdateWorkflowRequest`, and the ent workflow schema (`session/ent/schema/workflow.go`) have no `permission_mode` field. `FireNow` builds a `CreateSessionRequest` that never sets `PermissionMode`. Since unattended cron workflows require `bypassPermissions` (or at least `acceptEdits`) to run without human approval prompts, this gap makes fully-automated workflows functionally broken — they will block on permission prompts and never complete.
- **fix hint**: Add a `permission_mode string` field to the ent workflow schema, regenerate (`make generate-proto`), add it to `WorkflowProto`/`CreateWorkflowRequest`/`UpdateWorkflowRequest`, thread it through `WorkflowService.CreateWorkflow`/`UpdateWorkflow`, and pass `PermissionMode: wf.PermissionMode` in `FireNow`'s `CreateSessionRequest`.

### GAP-030 — fork_at_message=0 creates empty conversation fork instead of copying all messages
- **status**: open
- **severity**: high
- **fingerprint**: GAP-Fork-ZeroLineCount-EmptyFile
- **file**: `session/history_fork.go`
- **detail**: The proto field `fork_at_message` documents that value 0 means "copy all messages", and the ForkModal UI labels 0 as "(all messages)" with the slider at the leftmost position (default). However, `ForkClaudeConversation` uses the loop condition `for lineCount > 0 && scanner.Scan()`, so when `lineCount == 0` the loop body never executes and an empty JSONL file is written. Every fork created via the history page with the default slider position produces an empty conversation, meaning `--resume <uuid>` starts a fresh session rather than continuing from the forked conversation.
- **fix hint**: In `ForkClaudeConversation`, treat `lineCount == 0` as "copy all": change `for lineCount > 0 && scanner.Scan()` to `for scanner.Scan()` and only enforce the line-count cap when `lineCount > 0`. Example: `for scanner.Scan() { if lineCount > 0 && copied >= lineCount { break } ... copied++ }`.

### GAP-031 — `UpdateProject` handler ignores the `id` field from `UpdateProjectRequest`
- **status**: open
- **severity**: high
- **fingerprint**: GAP-PROTO-UpdateProject-id-field-ignored
- **file**: `server/services/project_service.go`
- **detail**: The `UpdateProjectRequest` proto message has an `id` field (field 1) to identify which project to update. The `UpdateProject` handler at line 92 builds a `session.ProjectData` struct that sets `Name` and `Description` from the request but never sets the `ID` field from `req.Msg.Id`. The `storage.UpdateProject` call receives a `ProjectData` with an empty `ID`, so the storage layer cannot reliably identify the target project.
- **fix hint**: In `UpdateProject` in `server/services/project_service.go`, add `ID: req.Msg.Id,` to the `session.ProjectData` struct literal so the storage layer can identify the correct project to update.

### GAP-032 — `UpdateProject` ignores `id` field — storage cannot identify the target project
- **status**: open
- **severity**: high
- **fingerprint**: GAP-PROTO-UpdateProject-id-field-ignored
- **file**: `server/services/project_service.go`
- **detail**: The `UpdateProjectRequest` proto message has an `id` field (field 1) to identify which project to update. The `UpdateProject` handler at line 92 builds a `session.ProjectData` struct that sets `Name` and `Description` from the request but never sets the `ID` field from `req.Msg.Id`. The `storage.UpdateProject` call receives a `ProjectData` with an empty `ID`, so the storage layer cannot reliably identify the target project.
- **fix hint**: In `UpdateProject` in `server/services/project_service.go`, add `ID: req.Msg.Id,` to the `session.ProjectData` struct literal so the storage layer can identify the correct project to update.

### GAP-033 — `SessionDefaults` `EnvVars` and `CLIFlags` fields are configured but never applied to sessions
- **status**: open
- **severity**: high
- **fingerprint**: GAP-SERVER-CreateSession-envvars-cliflags-unimplemented
- **file**: `config/defaults.go`
- **detail**: `ResolveDefaults` correctly merges `EnvVars` and `CLIFlags` across global/directory/profile layers, and `GetSessionDefaults`/`UpdateGlobalDefaults` expose these fields in the proto. However, `InstanceOptions` has no `EnvVars` or `CLIFlags` fields, so neither the resolved defaults nor any request fields are ever passed to the session process. The `CLIFlags` value needs to be appended to the program invocation and `EnvVars` need to be injected into the tmux environment. The feature is visible and configurable in settings UI (`GlobalDefaultsForm`), but has no effect on session creation.
- **fix hint**: Add `EnvVars map[string]string` and `CLIFlags string` to `InstanceOptions`, pass them from resolved defaults in the `CreateSession` handler, and apply them in `session/instance.go` when constructing the tmux command (env vars via tmux `setenv` or command prefix; CLI flags appended to `Program` invocation).

### GAP-034 — `ResumeHibernatedSession` does not re-start `AutonomousDriver` when `AutonomousMode=true`
- **status**: open
- **severity**: high
- **fingerprint**: GAP-HANDLER-ResumeHibernated-AutonomousDriver
- **file**: `server/services/session_service.go`
- **detail**: When `HibernateSession` is called on a session with `AutonomousMode=true`, the handler stops the driver via `stopAndDeregisterDriver` (line 1505) and saves the instance with `AutonomousMode=true` still set in storage. When `ResumeHibernatedSession` is called later, it calls `instance.ResumeFromHibernation(ctx)` and saves, but never checks `instance.AutonomousMode` or calls `StartAutonomousDriverForInstance`. After resume, the session's `autonomous_mode` flag is `true` in storage and the UI shows autonomous mode as active, but no `AutonomousDriver` goroutine is actually running — the session will sit idle forever without injecting prompts.
- **fix hint**: After `ResumeHibernatedSession` calls `instance.ResumeFromHibernation(ctx)` successfully, add a check: `if instance.AutonomousMode && s.headlessPool != nil { s.StartAutonomousDriverForInstance(instance) }` before saving and publishing the event.

### GAP-035 — `ListBacklogItems` never populates `item_sessions` — board shows no triage/gate status
- **status**: open
- **severity**: high
- **fingerprint**: GAP-REPO-ListBacklogItems-missing-item-sessions
- **file**: `session/ent_repository_backlog.go`
- **detail**: The repository's ListBacklogItems method only eager-loads WithSource(), omitting WithStatusEvents() and item_sessions entirely. The service layer's ListBacklogItems also never calls ListItemSessions per item. As a result, every BacklogItem returned from the list RPC has empty item_sessions and status_events arrays. The frontend's mapBacklogItem derives triageStatus and gateVerdict directly from item_sessions, so the backlog board and list page always show items with no triage status and no gate verdict, even when those records exist in the database. Only GetBacklogItem (single-item fetch) populates these edges.
- **fix hint**: Add WithStatusEvents() and eager-load item sessions (either via WithItemSessions() if available, or by batch-loading via ListItemSessions per item) to the ListBacklogItems repository query, or have the BacklogService.ListBacklogItems service handler batch-fetch item sessions for all returned items.

### GAP-036 — `ForkSession` handler skips all post-creation wiring that `CreateSession` performs
- **status**: open
- **severity**: high
- **fingerprint**: GAP-HANDLER-ForkSession-missing-wiring
- **file**: `server/services/session_service.go`
- **detail**: The ForkSession handler (lines 2655-2719) calls ForkFromCheckpoint and AddInstance but never wires the status manager (SetStatusManager/StartController), starts the session driver (StartSessionDriver), registers any of the four instance callbacks (wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback), or wires the backlog lifecycle listener. CreateSession, CreateDirectorySession, and loadInstancesWithWiring all perform this full wiring. As a result, forked sessions have no terminal status detection (the review queue won't update), won't auto-handle trust-folder startup dialogs (may get stuck), won't auto-archive on completion, and won't track Claude conversation UUIDs.
- **fix hint**: After the `go func()` block that calls newInst.Start(), add the same wiring block used in CreateSession: call s.statusManager.SetStatusManager / StartController, session.StartSessionDriver(newInst, newInst.Path), and all four wireXxx callbacks. Mirror the pattern at lines 1199-1221 in session_service.go.

### GAP-037 — `GlobalDefaultsForm` silently drops `auto_yes` — never reads or saves it
- **status**: open
- **severity**: high
- **fingerprint**: GAP-UI-GlobalDefaultsForm-autoYes
- **file**: `web-app/src/components/settings/GlobalDefaultsForm.tsx`
- **detail**: The `SessionDefaultsConfig` proto message has an `autoYes` field (field 2) and `UpdateGlobalDefaultsRequest` also carries `autoYes` (field 2). The backend `UpdateGlobalDefaults` handler in `defaults_service.go` reads and persists `req.Msg.AutoYes`. However, `GlobalDefaultsForm.tsx` never reads `defaults.autoYes` from the `getSessionDefaults` response (line 51–56 load only `program`, `oneOffBaseDir`, `newProjectBaseDir`, `tags`, `cliFlags`, `envVars`) and never includes `autoYes` in the `updateGlobalDefaults` call (lines 87–94). Any `auto_yes: true` set in `config.json` is invisible in the UI and every Save resets it to `false` on the server.
- **fix hint**: Add `const [autoYes, setAutoYes] = useState(false)` to GlobalDefaultsForm, populate it from `defaults.autoYes` in `loadDefaults`, add a checkbox control in the form, and pass `autoYes` in the `updateGlobalDefaults` call.

### GAP-038 — Scheduler `FireNow` ignores `session_type` for `new_worktree`/`existing_worktree`/`new_project` workflows
- **status**: open
- **severity**: high
- **fingerprint**: GAP-SCHEDULER-FireNow-session_type
- **file**: `server/workflows/scheduler.go`
- **detail**: In FireNow() (line 127–186), the CreateSessionRequest is built with only OneOff (a bool derived from session_type == 'one_off') but never sets the SessionType enum field (field 13 on CreateSessionRequest). Workflows stored with session_type='new_worktree', 'existing_worktree', or 'new_project' will silently create a directory session instead because the resolver in session_service.go defaults to SESSION_TYPE_DIRECTORY when session_type is SESSION_TYPE_UNSPECIFIED and there's no branch/existing_worktree to infer from.
- **fix hint**: In FireNow(), add a mapping from wf.SessionType to the sessionv1.SessionType enum before building the request, and set req.SessionType accordingly (similar to how resolveSessionType() handles it in session_service.go line 1236-1260).

### GAP-039 — `ResumeHibernatedSession` does not re-add session to poller, wire status manager, or restart autonomous driver
- **status**: open
- **severity**: high
- **fingerprint**: GAP-RESUME-HIBERNATION-missing-wiring
- **file**: `server/services/session_service.go`
- **detail**: HibernateSession removes the session from the live poller at line 1520 (removeFromAllPollers). ResumeHibernatedSession (line 1537) uses s.storage.LoadInstances() (not findInstance) to get the instance, calls instance.ResumeFromHibernation(ctx) which fires the After hook and calls Start(), but never: (1) adds the instance back to the reviewQueuePoller, (2) wires the status manager / starts a controller, (3) wires rate-limit or status-change callbacks, or (4) restarts the AutonomousDriver if instance.AutonomousMode was true. After resume the session is alive in tmux but invisible to the poller, has no idle detection, and the autonomous driver is permanently lost.
- **fix hint**: After a successful ResumeFromHibernation call, add: wireRateLimitCallbacks, wireStatusChangeCallback, wireClaudeSessionIDCallback, wireAutoArchiveCallback, SetStatusManager + StartController, s.reviewQueuePoller.AddInstance(instance), and if instance.AutonomousMode == true, start a new AutonomousDriver.

### GAP-040 — `GlobalDefaultsForm` omits `autoYes`: saving global defaults silently resets it to false
- **status**: open
- **severity**: high
- **fingerprint**: GAP-UI-GlobalDefaultsForm-autoYes
- **file**: `web-app/src/components/settings/GlobalDefaultsForm.tsx`
- **detail**: The `SessionDefaultsConfig` proto and `UpdateGlobalDefaultsRequest` both carry an `auto_yes` field. The Go handler (`UpdateGlobalDefaults` in `defaults_service.go`) writes `req.Msg.AutoYes` into `cfg.SessionDefaults.AutoYes`. However, `GlobalDefaultsForm.tsx` never reads `defaults.autoYes` from `GetSessionDefaults` and never sends `autoYes` in the `updateGlobalDefaults` call. Every time a user saves global defaults from the Settings UI, `auto_yes` is silently reset to `false` because the field is omitted from the request.
- **fix hint**: Add an `autoYes` boolean state variable in `GlobalDefaultsForm`, populate it from `defaults.autoYes` in `loadDefaults`, render a checkbox (matching the pattern in `ProfilesManager.tsx`), and include `autoYes` in the `updateGlobalDefaults` call payload.

### GAP-041 — Project `running_count` / `complete_count` / `review_ready_count` always zero
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-Project-aggregate-counts-never-set
- **file**: `server/services/project_service.go`
- **detail**: The proto `Project` message declares `session_count`, `running_count`, `complete_count`, and `review_ready_count` fields (proto/session/v1/session.proto lines 1866-1877). `SessionList.tsx` renders all three status-count badges directly from `projectData.runningCount`, `projectData.completeCount`, and `projectData.reviewReadyCount` (lines 906-918, 1046-1058). However, `projectDataToProto` in `project_service.go` only sets `Id`, `Name`, `Description`, `CreatedAt`, and `UpdatedAt` — the four count fields are never populated. The badges are always hidden because the values are always 0.
- **fix hint**: In `projectDataToProto`, after fetching projects also query session counts grouped by status for each project (or accept a pre-computed map as a parameter from ListProjects). Populate `SessionCount`, `RunningCount`, `CompleteCount`, and `ReviewReadyCount` before returning the proto.

### GAP-042 — No dedicated Projects management page
- **status**: open
- **severity**: low-medium
- **file**: `web-app/src/app/` (missing `/projects` route)
- **detail**: Projects are only reachable via the session list bulk-select "Group As
  Project" action. No `/projects` nav route, no standalone CRUD page, no way to see
  all projects and their sessions at a glance.
- **fix hint**: Add a `/projects` page with project cards and session count badges
  (depends on GAP-003 being fixed first).

### GAP-043 — Backlog item source CRUD RPCs have no frontend UI
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RPC-BacklogService-ItemSources
- **file**: `server/services/backlog_service.go`
- **detail**: Six `BacklogService` RPCs — `createItemSource`, `listItemSources`, `updateItemSource`, `deleteItemSource`, `triggerSync`, `getSyncHistory` — are implemented in `backlog_service.go` and present in the generated TypeScript client (`web-app/src/gen/session/v1/backlog_pb.ts`), but are not called anywhere in `web-app/src/`. These RPCs manage external plugin sources that feed items into the backlog. Without a UI, users cannot register, configure, or remove item sources, nor manually trigger syncs or inspect sync history.
- **fix hint**: Create a `BacklogSettingsPanel` or `ItemSourcesPage` component that lists registered sources (`listItemSources`), provides create/edit/delete forms (`createItemSource`, `updateItemSource`, `deleteItemSource`), and a 'Sync Now' button (`triggerSync`) with a history view (`getSyncHistory`). Add it to the Backlog navigation.

### GAP-044 — `/settings/unfinished` page exists with real component but has zero inbound links
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-NAV-settings-unfinished-unreachable
- **file**: `web-app/src/app/settings/unfinished/page.tsx`
- **detail**: The page at `/settings/unfinished` renders `UnfinishedSourcesSettings` (a functional component for configuring watch directories and pinned repos). It is registered in `routes.ts` as `routes.settingsUnfinished`, but `routes.settingsUnfinished` is never referenced anywhere else in the codebase — not in `NAV_PAGES`, not in the settings page tabs, not in any link or redirect. Users cannot navigate to this page through any UI surface.
- **fix hint**: Either add a link to `/settings/unfinished` from the settings page (e.g., as a tab or subsection), or add it as a hamburger-only nav entry in `NAV_PAGES` in `web-app/src/lib/nav-pages.ts`. If the page content has been migrated into the main settings page, delete `web-app/src/app/settings/unfinished/` and remove `settingsUnfinished` from `routes.ts`.

### GAP-045 — `/account` page hardcoded in Header/BottomNav, bypassing `NAV_PAGES`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-NAV-account-bypasses-nav-pages
- **file**: `web-app/src/components/layout/Header.tsx`
- **detail**: The `AccountPage` (`/account`) is linked directly in `Header.tsx` and `BottomNav.tsx` via hardcoded `routes.account` references, without an entry in `NAV_PAGES`. This excludes it from feature-flag filtering, `MOBILE_NAV_PAGES`/`HEADER_NAV_PAGES` computed lists, and any nav-page-based active-link logic or test helpers that iterate `NAV_PAGES`.
- **fix hint**: Add a `NavPage` entry for `routes.account` to `NAV_PAGES` in `web-app/src/lib/nav-pages.ts`, then reference it from `Header`/`BottomNav` via the standard `NAV_PAGES` filter pattern instead of hardcoded hrefs.

### GAP-046 — `GlobalDefaultsForm` silently drops `autoYes` on every save
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-GlobalDefaultsForm-autoYes-missing
- **file**: `web-app/src/components/settings/GlobalDefaultsForm.tsx`
- **detail**: The `UpdateGlobalDefaultsRequest` proto has an `auto_yes` field (field 2) that the Go handler in `defaults_service.go` writes to `cfg.SessionDefaults.AutoYes`. The `SessionDefaultsConfig` returned by `GetSessionDefaults` also contains `autoYes`. However, `GlobalDefaultsForm.tsx` never reads `defaults.autoYes` on load, never renders a UI control for it, and omits `autoYes` from the `updateGlobalDefaults({ program, oneOffBaseDir, newProjectBaseDir, tags, envVars, cliFlags })` call. Because proto message fields default to their zero value when absent, every save from this form effectively resets global `autoYes` to `false`, silently overwriting any previously-stored value.
- **fix hint**: Add an `autoYes` boolean state variable to `GlobalDefaultsForm`, set it from `defaults.autoYes` in `loadDefaults`, render a checkbox control (matching the pattern used in `ProfilesManager.tsx` line 323), and include `autoYes` in the `updateGlobalDefaults` call payload.

### GAP-047 — `HibernationConfig` struct fields have no proto RPC and no settings UI
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-HibernationConfig-noProtoNoUI
- **file**: `config/config.go`
- **detail**: `HibernationConfig` holds five user-configurable fields: `Enabled` (bool), `IdleTimeoutMinutes` (int), `ResourcePressureThreshold` (int), `CheckpointDir` (string), and `RetentionDays` (int). None of these fields appear in any proto message; there is no Get/Update RPC for hibernation config. The frontend hardcodes `MEMORY_PRESSURE_THRESHOLD = 85` in `SystemMemoryContext.tsx` to mirror only the default backend value — if a user sets `resource_pressure_threshold_pct: 70` in `config.json`, the frontend UI indicator will fire at 85 instead of 70. All hibernation configuration requires direct `config.json` edits.
- **fix hint**: Add a `HibernationConfigProto` message and `GetHibernationConfig`/`UpdateHibernationConfig` RPCs to `session.proto`, implement a handler in `defaults_service.go` (or a new `hibernation_service.go`), and add a Hibernation section to the General settings tab. Update `SystemMemoryContext` to fetch the threshold from the RPC instead of hardcoding 85.

### GAP-048 — `WorkflowForm` has no UI control for `session_type` — always submits `'directory'`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-WorkflowForm-session_type-no-input
- **file**: `web-app/src/components/workflows/WorkflowForm.tsx`
- **detail**: The `WorkflowFormData` interface and `EMPTY` initializer include `sessionType: 'directory'` and `protoToFormData` reads it from existing workflows, but the JSX renders no radio button, select, or any other input that calls `setField('sessionType', ...)`. When creating a new workflow, `session_type` is always sent as `'directory'`. When editing a workflow whose `session_type` is `'one_off'` or `'new_worktree'`, it can be read but not changed — the field is invisible to users. This contradicts the backend's support for the field and the hint text in `WorkflowsPanel.tsx` that implies directory-based invocation.
- **fix hint**: Add a radio group or select element to `WorkflowForm.tsx` between the Input Template and Model fields, offering the options: `directory`, `one_off`, and optionally `new_worktree`. Wire it to `setField('sessionType', value)`. Mirror the existing `OmnibarCreationPanel` radio group pattern for consistency.

### GAP-049 — `UpdateSession` drops `steer_message` silently when controller is nil
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UPDATE-steer_message-silent-drop
- **file**: `server/services/session_service.go`
- **detail**: In UpdateSession at lines 1396-1411, when steer_message is set and autonomous_mode is enabled, the handler fetches the controller via instance.GetController(). If the controller is nil (e.g., session is in Creating/Paused/Stopped state, or PTY attachment failed), the steering message is silently discarded. No error is returned to the caller — the RPC reports success. The caller has no way to know the steering message was never delivered. The only indication is a warn-level log entry if sendErr is non-nil, but the nil-controller path produces no log at all.
- **fix hint**: At line 1411, add an else branch after the `if controller != nil` block: `else { return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("steer_message delivery failed: session controller is not available (session may be paused, stopped, or still starting)")) }`. This surfaces the failure to the caller instead of silently succeeding.

### GAP-050 — `Session.DiffStats.content` is never populated in `InstanceToProto()`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-Session-DiffStats-content-unpopulated
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The proto `DiffStats` message has three fields: `added` (1), `removed` (2), and `content` (3, described as 'Full unified diff content'). The ent `DiffStats` schema stores `content` (Text, Optional). The DB read path in `ent_repository.go` (lines 960–964) loads `Content` from the DB. The in-memory `git.DiffStats` struct carries `Content`. The serialization layer (`instance_serialization.go` lines 109–113) preserves `Content`. However, `InstanceToProto()` at lines 79–85 only copies `Added` and `Removed` — `Content` is always omitted. This means every `Session` response from `ListSessions`, `GetSession`, and `WatchSessions` returns an empty `content` field in `DiffStats`. The `review_queue_adapter.go` correctly populates `Content` for review queue items (line 66: `Content: item.DiffStats.Content`), confirming the omission in `InstanceToProto` is an inconsistency. Callers wanting diff content must call `GetSessionDiff` separately.
- **fix hint**: In `server/adapters/instance_adapter.go`, update the `DiffStats` block (lines 81–84) to also include `Content: stats.Content`. The field is already available via `inst.GetDiffStats().Content`.

### GAP-051 — `browser-passthrough` feature flag has no `FEATURE_META` label and no frontend consumer
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-BrowserPassthrough-MissingFEATURE_META
- **file**: `web-app/src/app/settings/features/page.tsx`
- **detail**: The server exposes three feature flags via `GetFeatureFlags`: `backlog`, `browser-passthrough`, and `backlog:conversation-view` (defined in `server/services/session_service.go` lines 3757–3769). `FEATURE_META` in the Features page only maps `backlog` to a human label (line 25). The other two flags — `browser-passthrough` and `backlog:conversation-view` — render in the Features UI using their raw internal names with no friendly label, making them confusing to users. Additionally, `browser-passthrough` has zero frontend consumers (no component reads that flag), so toggling it has no visible effect.
- **fix hint**: Add `'browser-passthrough': { label: 'Browser Passthrough (Beta)' }` and `'backlog:conversation-view': { label: 'Backlog Conversation View' }` to `FEATURE_META` in `web-app/src/app/settings/features/page.tsx`. Separately, wire the `browser-passthrough` flag to the Browser tab component so toggling it actually gates the feature.

### GAP-052 — `AnalyticsEvent` REST summary endpoint never read by any frontend component
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-ANALYTICS-REST-SummaryEndpointUnread
- **file**: `server/handlers/analytics_handler.go`
- **detail**: The `AnalyticsEvent` ent entity collects page views, RPC latencies, navigation events, and user actions written by the frontend via `POST /api/analytics`. A companion `GET /api/analytics/summary` REST endpoint aggregates this data (top events, RPC latency p50/p95/p99, page views). However, no frontend component, hook, or page ever fetches this summary endpoint — there is no analytics dashboard page under `web-app/src/app/analytics/` (only `/analytics/escape` exists for escape sequence data). The collected data is retained but never visualised.
- **fix hint**: Either create a frontend analytics dashboard page at `/analytics` that fetches `GET /api/analytics/summary`, or expose the data via a ConnectRPC endpoint in `session.proto` so it can be consumed through the standard client pattern.

### GAP-053 — ClaudeSession.settings fields stored in DB but never exposed via proto response
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-ClaudeSession-settings-never-populated
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The proto ClaudeSession message (field 21 on Session) has a ClaudeSettings sub-message (field 5) with auto_reattach, preferred_session_name, create_new_on_missing, show_session_selector, and session_timeout_minutes. These five columns are persisted to and loaded from the claudesession ent table (session/ent/schema/claudesession.go) and are fully hydrated into ClaudeSessionData.Settings in ent_repository.go. However, InstanceToProto in instance_adapter.go only copies ConversationUUID, SquadSessionID, and ProjectName into the proto ClaudeSession, leaving Settings (field 5) always empty/zero in every GetSession/ListSessions/WatchSessions response.
- **fix hint**: In instance_adapter.go inside the `if inst.GetClaudeSession() != nil` block (around line 90), add `Settings: &sessionv1.ClaudeSettings{AutoReattach: cs.Settings.AutoReattach, PreferredSessionName: cs.Settings.PreferredSessionName, CreateNewOnMissing: cs.Settings.CreateNewOnMissing, ShowSessionSelector: cs.Settings.ShowSessionSelector, SessionTimeoutMinutes: int32(cs.Settings.SessionTimeoutMinutes)}` to the ClaudeSession literal.

### GAP-054 — Session.cdpState populated server-side but never read by any frontend component
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-SESSION-CDPState-frontend-never-reads
- **file**: `web-app/src/components/sessions/BrowserTab.tsx`
- **detail**: The server populates both Session.vncState (proto field 51) and Session.cdpState (proto field 52) in instance_adapter.go. BrowserTab.tsx receives only a vncState prop and derives availability exclusively from VNCStatus. The component renders a CDPViewer (WebSocket Chrome DevTools Protocol viewer) but gates it behind VNC readiness — meaning CDP-only hosts (with Chrome DevTools Protocol but no VNC daemon) always show 'Browser passthrough unavailable' even when CDP is actively streaming. SessionDetailView.tsx at line 257 reads only session.vncState and never passes cdpState to BrowserTab.
- **fix hint**: Add cdpState?: CDPState to BrowserTab props, import CDPStatus from types_pb, and derive isReady from EITHER vncState being VNC_STATUS_READY with browserWindowDetected OR cdpState.status === CDP_STATUS_STREAMING. Update SessionDetailView.tsx to also pass session.cdpState to BrowserTab.

### GAP-055 — `config.json` `anthropicApiKey` field silently ignored by AI rule generation service
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-AnthropicAPIKey-bypassed-by-service
- **file**: `server/services/session_service.go`
- **detail**: config.Config.AnthropicAPIKey is populated from ANTHROPIC_API_KEY env var at config load time (config.go lines 490-491 and 852-853) and is also settable via config.json as 'anthropicApiKey'. However, NewSessionService (line 250) reads os.Getenv('ANTHROPIC_API_KEY') directly, bypassing the config struct entirely. If a user sets 'anthropicApiKey' in config.json without setting the env var, AI rule generation silently remains unavailable and logs 'set ANTHROPIC_API_KEY', contradicting the documented config field. There is also no UI to configure the key.
- **fix hint**: Pass the loaded config to NewSessionService (or thread cfg.AnthropicAPIKey as a parameter) so that the json config field is honored. Also consider adding a masked API key field to the settings UI.

### GAP-056 — `sessionTypeToProto` adapter silently maps `SessionTypeNewProject` to `SESSION_TYPE_UNSPECIFIED`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-ADAPTER-sessionTypeToProto-newProject
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The sessionTypeToProto function in server/adapters/instance_adapter.go handles Directory, NewWorktree, and ExistingWorktree session types but has no case for session.SessionTypeNewProject. It falls through to the default which returns SESSION_TYPE_UNSPECIFIED. Sessions created with session_type=new_project (which the frontend actively uses — OmnibarCreationPanel.tsx lists it as a radio option, Omnibar.tsx handles it, and SESSION_TYPE_NEW_PROJECT = 4 exists in the proto) will show session_type=UNSPECIFIED when read back from any RPC, losing the type information across the wire.
- **fix hint**: Add a case to sessionTypeToProto in server/adapters/instance_adapter.go: 'case session.SessionTypeNewProject: return sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT'. Also verify resolveSessionType in session_service.go already handles SESSION_TYPE_NEW_PROJECT (it does, mapping to session.SessionTypeNewProject).

### GAP-057 — `CreateSessionRequest.allowed_tools` fully implemented in Go but unreachable from any frontend creation flow
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CREATE-allowed_tools-no-frontend-path
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: The proto field `CreateSessionRequest.allowed_tools` (field 21) is wired end-to-end on the server side: the handler maps it to `InstanceOptions.AllowedTools`, and `instance_tmux.go` appends `--allowedTools` to the claude command at startup. However, neither `useSessionService.ts` (the `createSession` callback at line 225) nor `OmnibarContext.tsx` (the `createSession` call at line 169) includes `allowedTools` in the RPC call body. The only creation paths that bypass these hooks (history page fork/resume) also do not pass `allowed_tools`. Users have no way to set pre-approved tool permissions when creating sessions through the UI.
- **fix hint**: Add `allowedTools` to the `OmnibarSessionData` type and thread it through `OmnibarContext.tsx` → `useSessionService.ts` → the RPC call body. Add a UI control in `OmnibarCreationPanel.tsx` (e.g., a text input or preset checkboxes for common tools like Bash, Read, Edit).

### GAP-058 — Session defaults `env_vars` and `cli_flags` are resolved and stored but never forwarded to the session process
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-DEFAULTS-env_vars-cli_flags-never-applied
- **file**: `server/services/session_service.go`
- **detail**: The `SessionDefaultsConfig` (and `ProfileDefaultsProto`) expose `env_vars` and `cli_flags` fields that users can configure via `UpdateGlobalDefaults`/`UpsertProfile`. The `config.ResolveDefaults` function correctly merges these values. However, the `CreateSession` handler only applies the `Program` and `AutoYes` fields from the resolved defaults (lines 1055–1060); `env_vars` and `cli_flags` are silently dropped. Furthermore, `session.InstanceOptions` has no `EnvVars` or `CLIFlags` fields, so there is no downstream path for these values to reach `instance_tmux.go` when spawning the tmux session.
- **fix hint**: Add `CLIFlags string` and `EnvVars map[string]string` to `session.InstanceOptions` and `session.Instance`. In `instance_tmux.go`, append `CLIFlags` to the program command and inject `EnvVars` into the tmux `new-session -e` options. In the `CreateSession` handler, populate these from the resolved defaults.

### GAP-059 — `CreateSessionRequest.profile` field is supported by the backend but silently dropped by the Omnibar session creation path
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CREATEREQ-profile-no-omnibar
- **file**: `web-app/src/lib/contexts/OmnibarContext.tsx`
- **detail**: The backend resolves named profiles via `config.ResolveDefaults(cfg, workingDir, req.Msg.Profile)` when `SkipDefaults` is false. The `SessionWizard` component (now orphaned) had a profile dropdown. `OmnibarContext.createSession` never passes a `profile` field. `useSessionService.ts` `createSession` wrapper also does not include it. Users cannot apply a named profile when creating a session via the Omnibar, even though profiles are defined and managed in Settings.
- **fix hint**: Add a profile selector to `OmnibarCreationPanel`, wire it through `OmnibarFormState`, pass `profile: data.profile` in `OmnibarContext.createSession`, and add it to the `useSessionService.ts` `createSession` RPC body.

### GAP-060 — `DrawerNav` renders feature-gated pages regardless of flag state
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-NAV-DrawerNav-no-featureflag-filter
- **file**: `web-app/src/components/layout/DrawerNav.tsx`
- **detail**: DrawerNav iterates NAV_PAGES with a plain .map() and never checks the featureFlag field on each NavPage entry. As a result, the Backlog nav item (which has featureFlag: "backlog") is always visible in the desktop drawer even when the backlog feature flag is disabled. By contrast, Header filters with NAV_PAGES.filter((p) => !p.featureFlag || flags[p.featureFlag]) and BottomNav uses a filterByFlag helper — both correctly hide gated pages. DrawerNav is the only nav component that skips this check.
- **fix hint**: Add useFeatureFlags() to DrawerNav and wrap the map with the same filter pattern used in BottomNav: pages.filter((p) => !p.featureFlag || flags[p.featureFlag]).

### GAP-061 — Workflows have no `permission_mode` or `allowed_tools` fields — cron runs block on permission prompts
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WORKFLOW-no-permission-mode
- **file**: `server/workflows/scheduler.go`
- **detail**: `CreateSessionRequest` exposes `allowed_tools` (field 21) and `permission_mode` (field 22, values: default/acceptEdits/bypassPermissions/auto), but neither is present in the workflow ent schema, `WorkflowProto`, create/update request messages, or the scheduler's `FireNow` function. Unattended cron workflows that need to run Bash or edit files will encounter Claude Code's permission approval UI with no way to respond, stalling indefinitely. The only workaround is to embed `--dangerously-skip-permissions` into the `agent_type` field (e.g. `claude --dangerously-skip-permissions`), which is undiscoverable and not validated.
- **fix hint**: Add `permission_mode string` and `allowed_tools string` fields to `session/ent/schema/workflow.go`; add them to `WorkflowProto` / `CreateWorkflowRequest` / `UpdateWorkflowRequest`; expose a `Permission Mode` dropdown in `WorkflowForm.tsx`; pass `PermissionMode: wf.PermissionMode, AllowedTools: wf.AllowedTools` in `FireNow`'s `CreateSessionRequest`.

### GAP-062 — `BatchCreateSessions` silently drops `initial_prompt` and `tags` from `BatchSessionRequest`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-BatchCreateSessions-initial_prompt-tags-dropped
- **file**: `server/services/session_service.go`
- **detail**: The `BatchSessionRequest` proto declares `initial_prompt = 7` and `tags = 11` fields. The `BatchCreateSessions` handler builds a `CreateSessionRequest` from each batch item but only forwards `Title`, `Path`, `WorkingDir`, `Branch`, `Program`, `Category`, `AutoYes`, `SessionType`, and `ProjectId`. `InitialPrompt` and `Tags` from `batchReq` are never included in the forwarded `createReq`, so callers that set these fields get sessions created without their initial prompt or tags.
- **fix hint**: In the `BatchCreateSessions` goroutine where `createReq` is built, add `InitialPrompt: batchReq.InitialPrompt` and `Tags: batchReq.Tags` to the `CreateSessionRequest` struct literal.

### GAP-063 — `AutonomousDriver` completion clears `AutonomousMode` in-memory only — not saved to storage
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-AUTONOMOUS-completion-state-not-persisted
- **file**: `server/services/session_service.go`
- **detail**: `onAutonomousDriverComplete()` sets `inst.AutonomousMode = false`, `inst.AutonomousTurn = 0`, and `inst.AutonomousOutcome` to `'done'`/`'stuck'` on the in-memory instance (around lines 3492–3500), then publishes an event, but never calls `SaveInstances`. If the server restarts before some other path triggers a save, the session JSON blob still shows `autonomous_mode=true` with no outcome. The same problem affects the `TurnCallback` in `buildTurnCallback` which updates `AutonomousTurn` in memory without persisting.
- **fix hint**: Add `_ = s.storage.SaveInstances([]*session.Instance{inst})` at the end of `onAutonomousDriverComplete`, after the outcome fields are set and before the notification event is published.

### GAP-064 — `WatchSessions` initial snapshot includes archived sessions
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WATCHSESSIONS-archived-leaked
- **file**: `server/services/session_service.go`
- **detail**: The WatchSessions snapshot loop (around line 1708) applies CategoryFilter, StatusFilter, and Hidden checks on each instance before sending, but has no ArchivedAt filter. ListSessions excludes archived sessions by default via its IncludeArchived guard (line 852). WatchSessionsRequest has no include_archived field, so clients cannot control this. Fresh WatchSessions connections therefore receive archived sessions that ListSessions would suppress, creating an inconsistent view between the two RPCs.
- **fix hint**: Add `if inst.ArchivedAt != nil { continue }` in the WatchSessions snapshot loop directly after the Hidden check (mirroring the ListSessions guard at line 852). If opt-in visibility of archived sessions is needed later, add an include_archived bool field to WatchSessionsRequest and wire it through.

### GAP-065 — `ArchiveSession` and `UnarchiveSession` do not emit EventBus events
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-ARCHIVE-no-event-published
- **file**: `server/services/session_service.go`
- **detail**: ArchiveSession (around line 3996) and UnarchiveSession (around line 4017) mutate inst.ArchivedAt and call storage.SaveInstances but never call s.eventBus.Publish. Every other mutation RPC (UpdateSession, RenameSession, RestartSession, etc.) publishes a SessionUpdatedEvent after saving. The omission means WatchSessions subscribers are never notified when a session is archived or unarchived, leaving the real-time stream stale until the client reconnects.
- **fix hint**: After SaveInstances in both ArchiveSession and UnarchiveSession, call s.eventBus.Publish with a SessionUpdatedEvent for the mutated instance (the same pattern used by UpdateSession). Include 'archived_at' in the changed-fields list so subscribers can filter if needed.

### GAP-066 — `Shell.stopped_at` defined in proto and DB but never populated in `shellToProto`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-Shell-stopped_at-unpopulated
- **file**: `server/services/session_service_shells.go`
- **detail**: The proto Shell message defines stopped_at (field 9, types.proto lines 1317–1344). The ent DB schema (session/ent/schema/shell.go) stores stopped_at as a nillable optional Time, and ent_repository.go sets it via SetStoppedAt(time.Now()) in UpdateShellStatus when a shell stops. However, the in-memory session.Shell struct (session/shell.go) has no StoppedAt field at all. As a result, shellToProto never populates stopped_at on the proto response. Additionally, instance_shells.go ReconcileShells restores stopped shells from the DB but omits StoppedAt when building the in-memory Shell struct. Callers of ListShells therefore always see stopped_at as zero/nil for completed shells even though the data exists in the DB.
- **fix hint**: Add StoppedAt *time.Time to the session.Shell struct in session/shell.go. Populate it in ReconcileShells from e.dbShell.StoppedAt. Set it in watchShellExit via time.Now() at exit time. Then in shellToProto, add: if sh.StoppedAt != nil { p.StoppedAt = timestamppb.New(*sh.StoppedAt) }

### GAP-067 — `WorkflowsPanel` 'Run' button always fires with empty arg — breaks `{{input}}` workflows
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-WorkflowsPanel-RunButton-no-arg-input
- **file**: `web-app/src/components/workflows/WorkflowsPanel.tsx`
- **detail**: The '▶ Run' button at line 165 calls `runWorkflow({ id: wf.id })` with no `arg` field. In `FireNow` (scheduler.go line 138), the `{{input}}` substitution is guarded by `if arg != ""`, so when arg is empty the placeholder is never replaced and the literal string `{{input}}` reaches the agent. Any workflow that uses `{{input}}` in its `command` or `input_template` will receive a malformed prompt when triggered from the panel's Run button. The omnibar path (`@slug arg`) correctly passes the arg, so only the panel button is broken. The panel also shows no indicator that a workflow requires an input arg.
- **fix hint**: Add an inline text input or modal prompt that collects an optional arg before firing. Show the input only when `wf.inputTemplate` is non-empty or `wf.command` contains the substring `{{input}}`. Pass the collected value as `runWorkflow({ id: wf.id, arg: collectedArg })`.

### GAP-068 — `CreateCheckpoint` returns an opaque error for Hibernated sessions because `i.started` is false after hibernation
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CHECKPOINT-CreateCheckpoint-blocked-on-hibernated
- **file**: `session/instance_checkpoint.go`
- **detail**: instance_hibernate.go sets i.started = false at line 104 when hibernating a session. CreateCheckpoint (instance_checkpoint.go line 24) guards on !i.started and returns 'cannot create checkpoint on unstarted instance'. The ForkSession UI button and SessionActionsOverflow offer 'Checkpoint' for all sessions regardless of status, so a user attempting to checkpoint a hibernated session receives a CodeFailedPrecondition error with a confusing internal message rather than a clear 'session is hibernated' explanation.
- **fix hint**: In CreateCheckpoint, change the guard to check i.Status explicitly: if i.Status == session.Hibernated { return nil, fmt.Errorf("cannot create checkpoint on hibernated session '%s': resume it first", i.Title) }. Also in the UI, hide or disable the Checkpoint button when session.status == SESSION_STATUS_HIBERNATED.

### GAP-069 — `ForkSession` does not register the new instance with `HistoryLinker`, so its JSONL file is never auto-linked
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-FORK-ForkSession-missing-HistoryLinker-registration
- **file**: `server/services/session_service.go`
- **detail**: When a session is created at runtime via CreateSession, only reviewQueuePoller.AddInstance is called (line 1139). When ForkSession creates a new instance it similarly only updates reviewQueuePoller (lines 2696–2700). Neither path calls historyLinker.AddInstance. HistoryLinker.run() polls only instances already in hl.instances; it does not re-read from storage. The HistoryLinker.SetInstances call in dependencies.go (line 717) only runs once at startup. As a result, forked sessions (which start with a fresh --resume UUID pointing to a JSONL in ~/.claude/projects/) never get their history_file_path or claude_conversation_uuid fields populated. The same issue affects CreateSession but is most severe for forks, which depend on the JSONL link to validate the conversation state.
- **fix hint**: In the ForkSession handler, after s.reviewQueuePoller.SetInstances(…) at line 2696, also call s.historyLinker.AddInstance(newInst) if s.historyLinker != nil. Apply the same fix to CreateSession at line 1139. For the long term, expose a combined wireNewInstance helper that calls all three (reviewQueuePoller, PRStatusPoller, historyLinker) mirroring the pattern in dependencies.go lines 649–651.

### GAP-070 — `Session.launch_command` never populated in `InstanceToProto` — `SessionDetailView` always shows empty command
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-Session-launch_command-unpopulated
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The proto field `Session.launch_command` (field 45) is defined in `types.proto` and `Instance.LaunchCommand` is populated in `instance_tmux.go` during session start. However, `InstanceToProto` never sets `protoSession.LaunchCommand`. The `SessionDetailView` component at `web-app/src/components/sessions/SessionDetailView.tsx:994` conditionally renders the launch command (`if (session.launchCommand)`) and provides a copy button, but this block is always skipped because the field is always empty.
- **fix hint**: Add `LaunchCommand: inst.LaunchCommand,` to the `protoSession` struct literal in `InstanceToProto` alongside `InitialPrompt`.

### GAP-071 — `CheckpointList` component is orphaned and references a `DeleteCheckpoint` RPC that does not exist
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-CheckpointList-orphaned-no-DeleteCheckpoint-RPC
- **file**: `web-app/src/components/sessions/CheckpointList.tsx`
- **detail**: The `CheckpointList` component is defined and exports `CheckpointList`, but it is never imported or rendered by any other component or page — no `import { CheckpointList }` exists anywhere in `web-app/src/`. The component accepts an `onDelete?: (checkpointId: string) => void` prop and renders a delete button per checkpoint, but there is no `DeleteCheckpoint` RPC in `proto/session/v1/session.proto` (it was planned in `docs/tasks/session-resumption-hardening.md` but never implemented). As a result, checkpoint deletion has no end-to-end path: no proto definition, no server handler, and no frontend render site.
- **fix hint**: Either (a) add a `DeleteCheckpoint` RPC to `session.proto`, implement it in Go, add it to `useSessionService.ts`, and wire `CheckpointList` into `SessionDetailView` alongside `CheckpointButton`; or (b) remove `CheckpointList.tsx` and `CheckpointList.css.ts` if the delete-checkpoint feature is deferred.

### GAP-072 — Logs page uses deprecated single-level field and client-side multi-level filtering instead of the proto-native `levels` repeated field
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-LOGS-GetLogsRequest-levels-field-unused
- **file**: `web-app/src/app/logs/page.tsx`
- **detail**: The `GetLogsRequest` proto message has both a deprecated `level` (single string, field 2) and a `levels` (repeated string, field 8) that explicitly takes precedence and enables server-side OR filtering. The `/logs/page.tsx` uses only the deprecated `level` field for single-level filtering, then applies multi-level filtering client-side on the already-paginated response (see lines 90–110 and 140–160), discarding entries after the fact. The code even has a comment: "for multi-level we'd need API update" — but the API already supports it via `levels`. This means multi-level log filtering fetches more data than necessary and silently under-counts results when using pagination.
- **fix hint**: In `web-app/src/app/logs/page.tsx`, replace `level: singleLevelFilter` with `levels: levelFilters.length > 0 ? levelFilters : undefined` in both `getLogs` calls, and remove the client-side `if (levelFilters.length > 1)` post-filter blocks.

### GAP-073 — `DebugMenu` is only in the dead `Header` — the `/debug/escape-codes` page is unreachable
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-NAV-DebugMenu-orphaned
- **file**: `web-app/src/components/ui/DebugMenu.tsx`
- **detail**: DebugMenu is imported only in Header.tsx (line 11) and the debug trigger button (wrench icon) that opens it is rendered only inside Header (line 144–151). Since Header is never mounted, DebugMenu is inaccessible, which means /debug/escape-codes and other debug routes linked from DebugMenu cannot be reached by any user. The page at /debug/escape-codes exists and has no entry in routes.ts, relying entirely on the DebugMenu link for discoverability.
- **fix hint**: Add a debug trigger (e.g., a small wrench icon) to DrawerNav's bottom section or the CockpitShell global area, rendering DebugMenu from there instead of the dead Header.

### GAP-074 — `ConnectionIndicator` and `MemoryNavBadge` are dead — WebSocket status and memory pressure are invisible
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-NAV-ConnectionIndicator-MemoryNavBadge-orphaned
- **file**: `web-app/src/components/layout/ConnectionIndicator.tsx`
- **detail**: ConnectionIndicator (showing the live/disconnected WebSocket status) and MemoryNavBadge (warning when system memory is critical) are both imported only in the dead Header.tsx. Neither component is rendered in DrawerNav, BottomNav, or CockpitShell. Users have no way to see connection status or memory warnings in the running application. MemoryNavBadge was added after CockpitShell replaced Header, so it was never actually visible in production.
- **fix hint**: Render ConnectionIndicator in DrawerNav's bottom section or CockpitShell, and render MemoryNavBadge in BottomNav's action row or DrawerNav so they surface to users.

### GAP-075 — `AnalyticsEvent` ent entity is defined but `LogClientEvents` never persists to it
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-ANALYTICS-event-entity-never-written
- **file**: `session/ent/schema/analytics_event.go`
- **detail**: The AnalyticsEvent ent schema defines a table for storing browser analytics events (event_name, event_category, session_id, duration_ms, page, component, labels). The LogClientEvents RPC (session_service.go:3463) receives batched events from the frontend, but only calls logClientEntry() to write to the server log — it never inserts records into the AnalyticsEvent table. There are no calls to `r.client.AnalyticsEvent.Create()` anywhere in the codebase outside generated ent code. The entity schema and its five DB indexes exist and are migrated, but the write path is completely absent.
- **fix hint**: In the LogClientEvents handler, after logging, iterate over `req.Msg.GetEntries()` and persist each as an AnalyticsEvent via the ent client. Add a repository method `CreateAnalyticsEvent()` to EntRepository. Alternatively, if the AnalyticsEvent schema was a dead-end design, remove it to avoid schema confusion.

### GAP-076 — `EntRepository.ListSessions()`, `CreateSession()`, and `UpdateSession()` always return unimplemented error
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-REPO-ListSessions-returns-unimplemented
- **file**: `session/ent_repository.go`
- **detail**: EntRepository.ListSessions() at line 1025 returns `fmt.Errorf("ListSessions not yet implemented for EntRepository")`. Similarly CreateSession() (line 1031) and UpdateSession() (line 1037) return the same stub error. These are methods on the Repository interface that use the newer *Session domain model (as opposed to the InstanceData legacy model). The session_service.go ListSessions handler avoids hitting this stub by using the in-memory poller (reviewQueuePoller.GetInstances()) but falls back to loadInstancesWithWiring() which uses the InstanceData path. Any caller that directly calls repository.ListSessions() will receive an error.
- **fix hint**: Either implement the Session-domain-model methods in EntRepository using the existing ent query infrastructure, or remove them from the Repository interface if InstanceData-based methods are the intended long-term API.

### GAP-077 — Omnibar form ignores `SessionDefaults.Program` when opening
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-Omnibar-program-not-seeded-from-defaults
- **file**: `web-app/src/components/sessions/Omnibar.tsx`
- **detail**: The Omnibar creation form is initialized with a hardcoded `INITIAL_FORM_STATE` (line 70) that sets `program: ""`. The component never calls `getSessionDefaults` or `resolveDefaults` to pre-populate the `program` field from the user's configured `SessionDefaults.Program` global default. As a result, if a user has set a global default program (e.g. `gemini`) via the Settings > Global Defaults form, the Omnibar always opens with no program selected (falling through to the browser's first-option default), silently ignoring the configured default. The `OmnibarCreationPanel` does render a program select control and passes `availablePrograms` from `useAvailablePrograms`, but the initial value of that select is never seeded from session defaults.
- **fix hint**: On Omnibar open, call `getSessionDefaults({})` and set `program` in `formState` from `response.defaults.program` if the current `program` state is empty. The `OmnibarCreationPanel` already fetches `getSessionDefaults` for `newProjectBaseDir` (line 207) — the same call can populate `program` and `autoYes` simultaneously.

### GAP-078 — `memory_rss_mb` and `estimated_savings_mb` only populated in `ListSessions`, not in `GetSession` or `WatchSessions`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-Session-memory_rss_mb-GetSession-WatchSessions-unpopulated
- **file**: `server/services/session_service.go`
- **detail**: Lines 862–865 in ListSessions manually enrich each session's proto with `rss := s.memCacheReader.GetCachedRSSMB(inst.UUID)` and savings estimates. However, GetSession (line 908) and WatchSessions (line 1673) both call `adapters.InstanceToProto(instance, nil)` via `createInitialSnapshotEvent` (event_converter.go line 88), and InstanceToProto never sets memory_rss_mb or estimated_savings_mb. Any UI surface that uses GetSession or subscribes to WatchSessions events will always see 0 for these fields, even when valid cached data exists. The session detail panel and live streaming events therefore show no memory information.
- **fix hint**: Move the memory enrichment into InstanceToProto by adding a MemoryCacheReader parameter (already an interface), or create an enrichment helper that wraps InstanceToProto and populates memory fields. Call this wrapper from GetSession and from the event builder in createInitialSnapshotEvent.

### GAP-079 — `UpdateSession` Pause path does not stop or deregister the `AutonomousDriver`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UPDATE-pause-no-driver-stop
- **file**: `server/services/session_service.go`
- **detail**: UpdateSession (line 1429) calls `instance.Pause()` when the requested status is Paused. Instance.Pause() (instance.go line 946) calls `i.StopController()` and kills the tmux session, but it has no access to the SessionService driverRegistry and cannot stop the AutonomousDriver. The autonomous driver goroutine registered in `s.driverRegistry[instance.Title]` continues running after the controller is stopped. On the next iteration it calls `d.controller.SendCommandImmediate` which fails because the controller is stopped, causing the driver to exit with a 'stuck' outcome and triggering a spurious failure notification. Compare with the HibernateSession RPC (line 1505) which calls `s.stopAndDeregisterDriver(instance.Title)` before transitioning state.
- **fix hint**: Add a call to `s.stopAndDeregisterDriver(instance.Title)` immediately before or after the `instance.Pause()` call at session_service.go line 1429, matching the pattern already used in HibernateSession.

### GAP-080 — `UnfinishedWorkService.UndismissWorktree` has no frontend UI
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RPC-UnfinishedWork-UndismissWorktree-no-frontend-caller
- **file**: `web-app/src/app/unfinished/UnfinishedTab.tsx`
- **detail**: UndismissWorktree removes the dismiss record so a worktree reappears in the Unfinished Work tab (per its proto comment). The backend implements it (server/services/unfinished_work_service.go line 207) and tests pass, but UnfinishedTab.tsx only calls dismissWorktree and snoozeWorktree. There is no UI to list dismissed items and undo a dismiss. Once a worktree is dismissed, users have no way to recover it through the UI.
- **fix hint**: Add a 'Show dismissed' toggle to UnfinishedTab (similar to how the tab already exposes snooze/dismiss). When toggled, fetch dismissed items and show an 'Undismiss' button that calls client.undismissWorktree({ repoPath, branch }).

### GAP-081 — `/backlog/board` page bypasses the backlog feature flag and uses a hardcoded href instead of the route constant
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-NAV-backlogBoard-noFeatureGate
- **file**: `web-app/src/app/backlog/page.tsx`
- **detail**: The backlog nav entry in `nav-pages.ts` has `featureFlag: "backlog"`, so it is hidden when the flag is disabled. However, `/backlog/board` has no feature flag gate of its own, meaning the board page remains directly accessible via URL even when the backlog feature is disabled. Additionally, `routes.backlogBoard` is defined in `routes.ts` but never used — the backlog list page links to the board with a hardcoded `href="/backlog/board"` string, bypassing type-safe routing.
- **fix hint**: In `web-app/src/app/backlog/page.tsx`, replace `href="/backlog/board"` with `href={routes.backlogBoard}`. Add a feature flag check at the top of `web-app/src/app/backlog/board/page.tsx` that redirects to `/` or shows an error when the `backlog` feature flag is disabled.

### GAP-082 — `GlobalDefaultsForm` does not load or save the `auto_yes` global default
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-GlobalDefaultsForm-autoYes-missing
- **file**: `web-app/src/components/settings/GlobalDefaultsForm.tsx`
- **detail**: The `SessionDefaultsConfig` proto message (field 2: `bool auto_yes`) and `UpdateGlobalDefaultsRequest` (field 2: `bool auto_yes`) both carry the global auto-yes setting, and `defaults_service.go` reads and writes it. However, `GlobalDefaultsForm.tsx` neither initialises an `autoYes` state variable, nor renders a checkbox for it, nor passes `autoYes` to `updateGlobalDefaults`. The field is silently ignored: a user can set it in `config.json` by hand but cannot control it from the Settings UI. By contrast, `ProfilesManager` and `DirectoryRulesManager` both correctly expose `autoYes` for their respective scopes.
- **fix hint**: Add a boolean state variable `autoYes` to `GlobalDefaultsForm`, read it from `defaults.autoYes` in `loadDefaults`, include a checkbox in the form, and pass `autoYes` in the `updateGlobalDefaults` call alongside the existing fields.

### GAP-083 — `CreateSession` with `autonomous_mode=true` does not register the turn callback
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-AUTONOMOUS-CREATE-no-turn-callback
- **file**: `server/services/session_service.go`
- **detail**: When CreateSession starts an AutonomousDriver (lines 1212-1219), it registers the completion callback but omits driver.RegisterTurnCallback(s.buildTurnCallback(instance)). In contrast, StartAutonomousDriverForInstance (line 726) and StartAutonomousDriverWithTimeout (line 744) both call buildTurnCallback. This means sessions created via CreateSession with autonomous_mode=true never update the autonomous_turn or autonomous_max_turns fields on the Instance, so the UI turn counter always shows 0/0 for the full run.
- **fix hint**: Add driver.RegisterTurnCallback(s.buildTurnCallback(instance)) immediately after RegisterCompletionCallback at line 1214 in the CreateSession async goroutine, mirroring StartAutonomousDriverForInstance.

### GAP-084 — `BacklogSyncLoop` silently no-ops when storage backend is not `*EntRepository`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-REPO-SyncLoop-EntRepository-type-assertion
- **file**: `session/backlog_sync.go`
- **detail**: The `RunSyncLoop` (line 90) and `SyncOne` (line 181) methods in `backlog_sync.go` both type-assert `sl.storage.repo.(*EntRepository)` to access methods (`GetItemSourceByID`, `UpdateItemSourceSync`, `CreateSourceSyncEvent`) that are implemented on `*EntRepository` but are absent from the `Repository` interface. `RunSyncLoop` silently continues (skipping the source) on a failed assertion; `SyncOne` returns an error. This means any test or future backend that uses a non-ent repository will have the entire sync subsystem silently disabled with no warning in normal logs.
- **fix hint**: Add `GetItemSourceByID`, `UpdateItemSourceSync`, and `CreateSourceSyncEvent` to the `Repository` interface in `session/repository.go`, then update any test doubles and the mock/stub implementations so the type assertion can be removed.

### GAP-085 — `VCSPreference` (jj vs git selection) has no proto representation and no settings UI
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-VCSPreference-no-proto-no-ui
- **file**: `config/config.go`
- **detail**: `Config.VCSPreference` (valid values: `"auto"`, `"jj"`, `"git"`) controls which version control system is used when both are available. This is a meaningful user-facing preference — for example, users who have JJ installed but want to use plain git must set `vcs_preference: "git"` in config.json manually. There is no proto field and no settings UI.
- **fix hint**: Add a `vcs_preference` field to `UpdateGlobalDefaultsRequest` (and `SessionDefaultsConfig` response), wire it through `defaults_service.go`, and add a VCS Preference selector to `GlobalDefaultsForm.tsx` with options Auto / Git / JJ.

### GAP-086 — `workflowMetaCache` populated once at startup and never refreshed, despite comment claiming 'every minute'
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WORKFLOW-MetaCache-never-refreshed
- **file**: `server/services/session_service.go`
- **detail**: The struct comment at line 167 says 'Populated on startup and refreshed every minute' but `refreshWorkflowMetaCache` is only called once in `SetWorkflowRepository` at startup. No background goroutine or ticker exists. `CreateWorkflow`, `UpdateWorkflow`, and `DeleteWorkflow` delegate directly to `workflowSvc` without calling `refreshWorkflowMetaCache`. This means (1) `session.workflow_name` returned by `ListSessions`/`GetSession` is permanently stale after any workflow rename, (2) `maybeAutoArchive` uses the stale `archiveAfterHours` value so updating a workflow's `archive_after_hours` via `UpdateWorkflow` has no effect on new session archival behavior until the server is restarted.
- **fix hint**: Call `s.refreshWorkflowMetaCache(ctx)` at the end of the `CreateWorkflow`, `UpdateWorkflow`, and `DeleteWorkflow` delegate methods in `session_service.go`, or start a background goroutine in `SetWorkflowRepository` that ticks every minute as the comment describes.

### GAP-087 — `reconcileSessions` ignores Paused sessions, leaving zombie Paused+no-tmux state indefinitely
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RECONCILE-Paused-not-cleaned-up
- **file**: `session/review_queue_poller.go`
- **detail**: ReviewQueuePoller.reconcileSessions() (lines 437-466) contains a switch on inst.Status with only two cases: Active and Stopped. Sessions in Paused, Creating, Hibernated, or Restoring states are never examined. This creates a zombie state when combined with ReapPausedTmuxSessions (session_service.go line 4176): ReapPausedTmuxSessions calls inst.KillSession() which kills the tmux process without transitioning the session's status away from Paused. After the reap, the session has status=Paused but no live tmux session. reconcileSessions will never detect this inconsistency and will never transition the session to Stopped. On server restart the session is loaded back as Paused with no tmux, and the same situation persists. Users see a permanently Paused session that cannot be resumed (Resume() calls tmux commands that fail because the tmux session is gone).
- **fix hint**: In reconcileSessions, add a case for Paused that mirrors the Active case: if the paused session has no live tmux session, transition it to Stopped and fire EventExited. Alternatively, update ReapPausedTmuxSessions to call inst.transitionTo(ctx, session.Stopped) instead of (or in addition to) calling KillSession, so the status is updated atomically with the process kill.

### GAP-088 — WatchInsights silently ignores from/to time-range filter from client
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-INSIGHTS-WatchInsights-time-filter-ignored
- **file**: `server/services/insights_service.go`
- **detail**: The WatchInsights handler is declared as `func (s *InsightsService) WatchInsights(ctx context.Context, _ *connect.Request[sessionv1.WatchInsightsRequest], ...)` — the request is discarded via `_`. The frontend in `web-app/src/lib/hooks/useInsightsService.ts` sends `from` and `to` timestamp fields derived from the user-selected date filter, but the server pushes all session update events regardless of date. Client-side filtering is partially applied in the event handler (lines 112–118 of useInsightsService.ts) but only for incoming events, not for the stream itself, so sessions outside the date window still trigger UI re-renders and redundant state updates.
- **fix hint**: Replace the discarded `_` parameter with `req *connect.Request[sessionv1.WatchInsightsRequest]`, extract `req.Msg.From` and `req.Msg.To`, and filter outgoing events server-side to only push session summaries whose `lastMessageAt` falls within the requested range.

### GAP-089 — BacklogItem.user_modified_status_at is write-only: stored but never queried or exposed
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-SCHEMA-BacklogItem-user_modified_status_at-write-only
- **file**: `session/ent/schema/backlog_item.go`
- **detail**: The `user_modified_status_at` field (type `*time.Time`) is written to the DB in `ent_repository_backlog.go` (lines 261, 300) and `storage_backlog.go` (line 402) whenever a status transition occurs. However, the `backlogItemToData` converter (`ent_repository_backlog.go:18-46`) never reads it back, `BacklogItemData` has no corresponding field, and no proto message includes this timestamp. The sync loop comment in `backlog_sync.go:226` says status is 'always local-wins once user_modified_status_at is set' but the code never reads the field to enforce this—status is skipped unconditionally in sync, making the stored timestamp entirely unused.
- **fix hint**: Either add `UserModifiedStatusAt *time.Time` to `BacklogItemData`, read it back in `backlogItemToData`, and add a `user_modified_status_at` field to the `BacklogItem` proto message so the frontend can display when a user last changed status; or remove the field if the status-always-local-wins rule is meant to be unconditional.

### GAP-090 — BacklogItem→Session many-to-many ent edge is defined in schema but never populated or queried
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-SCHEMA-BacklogItem-sessions-dead-m2m-edge
- **file**: `session/ent/schema/backlog_item.go`
- **detail**: The ent schema defines `edge.To("sessions", Session.Type)` on `BacklogItem` (and the inverse `edge.From("backlog_items", BacklogItem.Type)` on `Session`). Neither `ent_repository_backlog.go` nor any other non-generated Go file ever calls `AddSessionIDs`, `WithSessions`, or reads `Edges.Sessions` / `Edges.BacklogItems` on these entities. All session-to-backlog-item linking uses the `ItemSession` junction entity with a loose string FK (`session_uuid`). The native many-to-many join table generated by ent is never written to, creating a divergence between the schema and the actual data model.
- **fix hint**: Decide whether the `ItemSession` pattern is the canonical link (in which case remove the `edge.To("sessions")` from BacklogItem and `edge.From("backlog_items")` from Session to clean up the schema), or whether the m2m edge should be populated alongside `ItemSession` creation to enable efficient SQL joins. Remove and regenerate ent code after deciding.

### GAP-091 — `ArchiveSession` and `UnarchiveSession` RPCs implemented in hook but never called by any component
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RPC-ArchiveUnarchiveSession-unwired-ui
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: useSessionService implements archiveSession (line 558) and unarchiveSession (line 572) calling the corresponding RPCs, and they are included in the hook's returned object (lines 959-960). However, they are not in SessionServiceContext's interface or provider, not destructured by app/page.tsx or any other page, and no component (SessionActionsOverflow, SessionCard, SessionRow, SessionList) has an onArchive/onUnarchive prop or menu item. Individual session archival is completely inaccessible from the UI; only workflow-level ArchiveWorkflowSessions is wired via useWorkflows.
- **fix hint**: Add archiveSession and unarchiveSession to the SessionServiceContext interface and GlobalSessionServiceProvider value, then wire them to an 'Archive / Unarchive' menu item in SessionActionsOverflow (alongside Delete). Also thread them through SessionList → SessionCard/SessionRow props so the actions are reachable in both card and row views.

### GAP-092 — ResolveDefaults RPC is never called in the live Omnibar session-creation path
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-ResolveDefaults-unused-in-Omnibar-creation
- **file**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`
- **detail**: The `ResolveDefaults` RPC merges global defaults, directory rules, and named profiles for the given working directory. It is implemented in `defaults_service.go`, has a frontend hook `useSessionDefaults`, and a `SourceBadge` component to show per-field sources — but all of these are only used in `SessionWizard.tsx`, which is never rendered (GAP-158). The live `OmnibarCreationPanel` only calls `getSessionDefaults` once (to load `newProjectBaseDir`), so resolved directory-rule and profile defaults are never surfaced to the user. The `profile` selector and `SourceBadge` feedback are entirely absent from the Omnibar form.
- **fix hint**: Wire `useSessionDefaults(workingDir, profileName)` into `OmnibarCreationPanel`: call it whenever `workingDir` changes, pre-fill `program` and `autoYes` from `defaults.program` / `defaults.autoYes`, add a profile selector dropdown, and render `SourceBadge` next to each pre-filled field.

### GAP-093 — Retention enforcer archives sessions in DB but never updates in-memory reviewQueuePoller
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WORKFLOW-RetentionEnforcer-no-memory-sync
- **file**: `server/workflows/retention.go`
- **detail**: StartRetentionEnforcer / runRetentionSweep calls entClient.Session.Update().SetArchivedAt() directly against the database. Unlike ArchiveWorkflowSessions (which updates both DB and in-memory poller at lines 4074-4082 of session_service.go), the retention enforcer receives no reviewQueuePoller reference and has no mechanism to update the live Instance objects. After a retention sweep, ListSessions still returns those sessions as non-archived because the filter at session_service.go:852 checks inst.ArchivedAt (in-memory) not the DB column. The stale state persists until server restart or the session is otherwise evicted from the poller.
- **fix hint**: Pass a session.InstanceProvider (or the reviewQueuePoller) into StartRetentionEnforcer and after each successful bulk SetArchivedAt call, iterate GetInstances() and set ArchivedAt on matching instances — mirror the pattern already used in ArchiveWorkflowSessions (session_service.go:4074-4082).

### GAP-094 — WorkflowProto has no existing_worktree path field, making session_type=existing_worktree unusable
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WORKFLOW-schema-existing_worktree-field-missing
- **file**: `proto/session/v1/session.proto`
- **detail**: CreateSessionRequest.existing_worktree (field 9) is required when session_type=SESSION_TYPE_EXISTING_WORKTREE. WorkflowProto, CreateWorkflowRequest, UpdateWorkflowRequest, and the ent schema (session/ent/schema/workflow.go) all lack an existing_worktree field. Even if FireNow were fixed to forward session_type to the proto enum (GAP-159), it has no worktree path to pass in the request, so the server would fall back to SessionTypeDirectory. The user has no way to save or configure an existing worktree target through the workflow form or API.
- **fix hint**: Add an optional string existing_worktree field to WorkflowProto, CreateWorkflowRequest, and UpdateWorkflowRequest in session.proto, add the corresponding column to the ent schema, run make generate-proto and the ent generator, then thread the value through FireNow into CreateSessionRequest.ExistingWorktree.

### GAP-095 — session_driver.go `isOneShot()` checks backlog tags but ignores `inst.OneShot`, causing one-shot sessions to be incorrectly restarted
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-SESSIONDRV-isOneShot-ignores-OneShot-field
- **file**: `session/session_driver.go`
- **detail**: The `isOneShot` function at line 491 only returns true for sessions tagged `backlog:triage` or `backlog:review`. It does not check `inst.OneShot`. In the driver loop (lines 175–195), after an initial-prompt-sent session stops, `isOneShot(inst)` is the guard that prevents the crash-restart path. For sessions created with `CreateSessionRequest.one_shot=true` (no backlog tags) and no `InitialPrompt`, `sentInitial` is set to `true` at startup (line 127), so `initialPromptSentAt` is set to when the driver starts. If Claude completes the `-p` task and exits within 5 minutes, the driver misreads it as an unexpected crash and calls `handleDriverFailure`, which restarts the session. The `tryExtractClaudeSessionID` at line 172 correctly checks `inst.OneShot`, revealing the inconsistency.
- **fix hint**: Change `isOneShot` to also return true when `inst.OneShot` is set: `func isOneShot(inst *Instance) bool { return inst.OneShot || inst.HasTag("backlog:triage") || inst.HasTag("backlog:review") }`. This ensures the non-backlog one-shot path at line 175 exits cleanly instead of triggering the crash-restart logic.

### GAP-096 — `ResumeHibernatedSession` does not restart the AutonomousDriver for sessions where `autonomous_mode=true`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RESUME-HIBERNATE-autonomous-driver-not-restarted
- **file**: `server/services/session_service.go`
- **detail**: `HibernateSession` explicitly calls `s.stopAndDeregisterDriver(instance.Title)` at line 1505 before hibernating, which correctly stops the AutonomousDriver. However, `ResumeHibernatedSession` (lines 1537–1577) calls only `instance.ResumeFromHibernation(ctx)` and publishes a status event; it never checks `instance.AutonomousMode` or calls `s.StartAutonomousDriverForInstance(instance)`. After a resume, the session's `autonomous_mode` field still shows `true` in storage and in proto responses, giving the user a false impression that autonomous operation is active, but no driver is running to inject orchestration prompts. This is distinct from GAP-162 (GAP-RESUME-HIBERNATE-missing-wiring), which covers the controller, poller, and callbacks.
- **fix hint**: After `instance.ResumeFromHibernation(ctx)` returns without error, add: `if instance.AutonomousMode && s.headlessPool != nil { s.StartAutonomousDriverForInstance(instance) }`. This mirrors the pattern in `CreateSession`'s goroutine (lines 1212–1218) and `UpdateSession`'s autonomous-mode toggle (line 1383).

### GAP-097 — BacklogService ItemSource CRUD RPCs implemented but unreachable from frontend
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-BACKLOG-ItemSource-CRUD-no-frontend-caller
- **file**: `server/services/backlog_service.go`
- **detail**: Four RPCs — `CreateItemSource` (line 727), `ListItemSources` (line 762), `UpdateItemSource` (line 787), and `DeleteItemSource` (line 826) — are fully implemented in Go and registered in the BacklogService. However, `web-app/src/lib/hooks/useBacklogService.ts` contains no wrappers for any of them, and no component in `web-app/src/components/backlog/` references item sources. There is no UI surface for configuring external sync sources (Linear, JIRA, etc.) despite the backend being ready.
- **fix hint**: Add `createItemSource`, `listItemSources`, `updateItemSource`, `deleteItemSource` callbacks to `useBacklogService.ts` and create an `ItemSourcesPanel` component (similar to the existing `BacklogBoard`) to expose this functionality.

### GAP-098 — SESSION_TYPE_NEW_PROJECT not mapped in session type adapters — round-trip lossy
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-ADAPTER-sessionTypeToProto-NewProject
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The `sessionTypeToProto` function (line 306) has cases for DIRECTORY, NEW_WORKTREE, and EXISTING_WORKTREE but no case for `session.SessionTypeNewProject`. Sessions created with the 'New Project' mode will have their `session_type` field returned as `SESSION_TYPE_UNSPECIFIED` in all GetSession/ListSessions/WatchSessions responses. Additionally, `ProtoToSessionType` (line 347) also has no case for `SESSION_TYPE_NEW_PROJECT` and falls through to `SessionTypeDirectory`, so any round-trip through proto silently downgrades the type. The frontend `OmnibarContext.tsx` sends `SessionType.NEW_PROJECT` and the ent repository correctly stores 'new_project' as a string in `session_type`, but the Go→proto conversion loses it.
- **fix hint**: Add `case session.SessionTypeNewProject: return sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT` to `sessionTypeToProto`, and `case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT: return session.SessionTypeNewProject` to `ProtoToSessionType` in `server/adapters/instance_adapter.go`.

### GAP-099 — `BacklogService` item source CRUD has no frontend callers
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RPC-BacklogItemSource-NoFrontendHook
- **file**: `web-app/src/lib/hooks/useBacklogService.ts`
- **detail**: `BacklogService` defines and fully implements four item source management RPCs in `server/services/backlog_service.go`. The feature registry documents them as separate features. However, `useBacklogService.ts` does not wrap any of them, and no component calls them. The backlog UI has no way to register or manage external item sources (Jira, GitHub Issues, etc.).
- **fix hint**: Add `createItemSource`, `listItemSources`, `updateItemSource`, `deleteItemSource` methods to `useBacklogService.ts`. Create an `ItemSourcesPanel` component under `web-app/src/components/backlog/` to let users manage their external integrations.

### GAP-100 — `UndismissWorktree` RPC is implemented but the Unfinished Work UI has no way to undo a dismiss
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RPC-UndismissWorktree-NoFrontendCaller
- **file**: `web-app/src/app/unfinished/UnfinishedTab.tsx`
- **detail**: `UnfinishedWorkService.UndismissWorktree` is implemented in `server/services/unfinished_work_service.go` with a test (`TestUndismissWorktree_NoOpOnUnknown`). The UI in `UnfinishedTab.tsx` calls `dismissWorktree` and `snoozeWorktree`, but there is no call to `undismissWorktree` anywhere in the frontend. Once a worktree is dismissed it can only be restored by clearing server state directly.
- **fix hint**: Add a `client.undismissWorktree(req)` call in `UnfinishedTab.tsx` (or a new `DismissedWorktreesPanel`). Likely needs a separate view or filter toggle to list dismissed worktrees, since they are excluded from the main `WatchUnfinishedWork` stream.

### GAP-101 — `AutonomousDriver` not restarted when resuming a hibernated autonomous session
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-Resume-AutonomousDriverLost
- **file**: `server/services/session_service.go`
- **detail**: `HibernateSession` calls `stopAndDeregisterDriver` before hibernating, correctly stopping the running `AutonomousDriver`. However, `ResumeHibernatedSession` never checks `instance.AutonomousMode` and never calls `StartAutonomousDriverForInstance`. A session that was running in autonomous mode before hibernation silently loses its driver on resume; the session becomes Active but autonomous orchestration is permanently gone with no error or notification.
- **fix hint**: After the poller re-add in `ResumeHibernatedSession`, check: `if instance.AutonomousMode && s.headlessPool != nil { s.StartAutonomousDriverForInstance(instance) }` — mirroring the pattern in `UpdateSession`.

### GAP-102 — ClaudeSession.metadata map stored in DB but never copied into proto response
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-ClaudeSession-metadata-never-populated
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The proto ClaudeSession message has a `map<string, string> metadata` field (field 6). ClaudeMetadata rows are persisted to a dedicated ent table (session/ent/schema/claudemetadata.go), eagerly loaded via WithMetadata() in ent_repository.go, and fully hydrated into ClaudeSessionData.Metadata (a map[string]string). However, InstanceToProto in instance_adapter.go does not copy this map into the proto response, so ClaudeSession.metadata is always empty for every caller (GetSession, ListSessions, WatchSessions).
- **fix hint**: After building the ClaudeSession proto literal in instance_adapter.go, add `if cs.Metadata != nil { protoSession.ClaudeSession.Metadata = cs.Metadata }` (the proto map type is directly assignable from Go map[string]string).

### GAP-103 — WorkflowProto has no autonomous_mode field — FireNow can't start AutonomousDriver
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WORKFLOW-AutonomousMode-not-in-proto-or-schema
- **file**: `proto/session/v1/session.proto`
- **detail**: CreateSessionRequest has bool autonomous_mode = 23 which starts an AutonomousDriver that drives a session to completion without human input. WorkflowProto (lines 2277–2296) and the workflow ent schema have no autonomous_mode field, so FireNow in server/workflows/scheduler.go never passes AutonomousMode: true to CreateSessionRequest. Cron-based workflows are automation-by-definition; the inability to enable AutonomousDriver means automated sessions stall waiting for user interaction rather than completing their task.
- **fix hint**: Add bool autonomous_mode = 18 to WorkflowProto, CreateWorkflowRequest, and UpdateWorkflowRequest in session.proto; add field.Bool('autonomous_mode').Default(false) to the workflow ent schema; run make generate-proto and make build; then set AutonomousMode: wf.AutonomousMode in FireNow's CreateSessionRequest.

### GAP-104 — `BacklogService.AttachSessionToItem` is implemented in Go but missing from `useBacklogService` hook
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-HOOK-BacklogService-AttachSessionToItem
- **file**: `web-app/src/lib/hooks/useBacklogService.ts`
- **detail**: The `AttachSessionToItem` RPC is implemented in `server/services/backlog_service.go` and defined in the proto. The `useBacklogService` hook implements `SpawnSessionFromItem` but not `AttachSessionToItem`, so there is no way to link an existing session to a backlog item from the UI. The hook's `useMemo` return object does not expose this function.
- **fix hint**: Add an `attachSessionToItem` callback to `useBacklogService.ts` that calls `clientRef.current.attachSessionToItem(...)` and include it in the `useMemo` return object.

### GAP-105 — `/account` page exists but is absent from `NAV_PAGES`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-AccountPage-NoNavEntry
- **file**: `web-app/src/lib/nav-pages.ts`
- **detail**: The `/account` page (`web-app/src/app/account/page.tsx`) is a fully implemented passkey management page with its own layout. It is reachable only via hardcoded `routes.account` links in `Header.tsx` (line 105) and `BottomNav.tsx` (line 145) that are rendered conditionally on `authEnabled && authenticated`. When auth is disabled the page is completely unreachable. There is no entry in `NAV_PAGES`, so it cannot appear in the hamburger menu, bottom nav More sheet, or any nav-driven surface.
- **fix hint**: Add an entry for `routes.account` to `NAV_PAGES` in `web-app/src/lib/nav-pages.ts` with `mobileNav: false` and `headerNav: false`, and conditionally show it only when auth is enabled (similar to how `Header.tsx` already guards the inline link).

### GAP-106 — `SessionWizard` applies only `program`/`autoYes` from `ResolvedDefaults`; `tags`, `cliFlags`, and `envVars` are ignored
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-SessionWizard-defaults-tags-cliflags-envvars-not-applied
- **file**: `web-app/src/components/sessions/SessionWizard.tsx`
- **detail**: The `useEffect` at line 172 of `SessionWizard.tsx` applies resolved defaults to the form, but only sets `program` (line 181) and `autoYes` (line 183). The resolved defaults also contain `tags`, `cliFlags`, and `envVars`, but the `SessionWizard` form does not have fields for `cliFlags`/`envVars` and does not pre-populate `tags` from defaults. The `FieldSources` type in `useSessionDefaults.ts` also tracks sources only for `program`, `autoYes`, `tags`, and `cliFlags` — no source badge is shown for `envVars` even though the hook returns them.
- **fix hint**: In the `SessionWizard` useEffect (`SessionWizard.tsx` ~line 172), add: `if (!edited.has('tags') && resolvedDefaults.tags.length > 0) { newValues.tags = resolvedDefaults.tags; }`. Add `tags`, `cliFlags`, and `envVars` fields to `SessionFormData` and add the corresponding form UI. Also add `envVars` to `FieldSources` in `useSessionDefaults.ts`.

### GAP-107 — `UnfinishedSourcesSettings` page is unreachable — no navigation link exists
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-UnfinishedSourcesSettings-nav
- **file**: `web-app/src/app/settings/unfinished/page.tsx`
- **detail**: The page at `/settings/unfinished` is fully implemented (`UnfinishedSourcesSettings` component with watch dirs, pinned repos, and auto-spider toggle). The route is defined in `routes.ts` as `settingsUnfinished`. However, `routes.settingsUnfinished` is never referenced in `nav-pages.ts` or the main settings `page.tsx` tabs, so users have no way to reach this settings screen through the UI without typing the URL directly.
- **fix hint**: Add an entry to `NAV_PAGES` in `web-app/src/lib/nav-pages.ts` referencing `routes.settingsUnfinished` (with `headerNav: false` so it appears in the hamburger menu), or add it as a tab in `web-app/src/app/settings/page.tsx` similar to the existing 'Config Files' tab.

### GAP-108 — `UpdateSession` silently ignores `SESSION_STATUS_HIBERNATED` and `SESSION_STATUS_RESTORING` status transitions
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-UpdateSession-hibernate-status
- **file**: `server/services/session_service.go`
- **detail**: The `UpdateSession` handler's status-change block (lines 1418-1442) only handles `Paused ↔ Active` transitions. If a caller sends `status = SESSION_STATUS_HIBERNATED` (to hibernate) or `status = SESSION_STATUS_ACTIVE` when the session is currently `Hibernated` (to resume), neither branch fires, no error is returned, and the caller gets back a success response with the status unchanged. The dedicated `HibernateSession` and `ResumeHibernatedSession` RPCs must be used instead, but the `UpdateSession` contract gives no indication of this.
- **fix hint**: Either return a `CodeUnimplemented` or `CodeInvalidArgument` error when the target status is `HIBERNATED`/`RESTORING` (explaining that dedicated RPCs exist), or add a comment to the proto that `status` only supports `PAUSED`/`ACTIVE` transitions.

### GAP-109 — `WatchInsights` and `WatchUnfinishedWork` missing from `StreamingWSBridge`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-STREAMING-WatchInsights-WatchUnfinishedWork-no-WS-bridge
- **file**: `server/server.go`
- **detail**: The `StreamingWSBridge` is registered only for `WatchSessions` and `WatchReviewQueue` (lines 320-328 in server.go). The comment explains the bridge exists to avoid the 6-connection-per-origin limit on HTTP/1.1. Both `InsightsService.WatchInsights` (called by `useInsightsService.ts`) and `UnfinishedWorkService.WatchUnfinishedWork` (called by `useUnfinishedWork.ts`) are server-streaming RPCs used by the frontend, but are registered without the bridge. On the local HTTP/1.1 server these streaming connections consume one of the six browser connections to the origin, degrading the rest of the UI.
- **fix hint**: In `wireDepsIntoServer`, after registering the InsightsService and UnfinishedWorkService handlers, add their Watch procedure paths to the `StreamingWSBridge` using `wsBridge.Handler("/api")`, analogous to how `watchSessionsPath` and `watchReviewQueuePath` are handled. Each service needs its own bridge instance wrapping its respective handler.

### GAP-110 — `ArchiveSession`/`UnarchiveSession` RPCs implemented but have no UI entrypoint
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UI-ArchiveSession-no-frontend-entrypoint
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: ArchiveSession and UnarchiveSession are fully implemented on the server (session_service.go lines 3996-4035). The hook useSessionService.ts defines archiveSession and unarchiveSession callbacks (lines 558, 572) and returns them. However, neither function is included in SessionServiceContextValue (contexts/SessionServiceContext.tsx), and no component, page, or context consumer ever calls them. Sessions can be archived programmatically via ArchiveWorkflowSessions, but there is no user-facing button or menu item to manually archive or restore an individual session.
- **fix hint**: Add archiveSession and unarchiveSession to SessionServiceContextValue in SessionServiceContext.tsx, then add an Archive/Restore action to the SessionCard action menu (sessions/SessionCard.tsx) or the session options dropdown.

### GAP-111 — `GetSyncHistory` is stubbed but the storage layer has no corresponding read method
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-REPO-GetSyncHistory-storage-layer-missing
- **file**: `server/services/backlog_service.go`
- **detail**: CreateSourceSyncEvent is implemented in session/ent_repository_backlog.go (line 449) and called from session/backlog_sync.go (line 248) every time a sync completes, writing SourceSyncEvent rows. However, GetSyncHistory (backlog_service.go line 1580) returns CodeUnimplemented, and there is no corresponding repository method (e.g., ListSourceSyncEvents or GetSyncEventsForSource) to read those rows back. The SourceSyncEvent ent schema and generated code are complete; only the read path is missing at both the repository and service layers.
- **fix hint**: Add a ListSourceSyncEvents(ctx, sourceID) method to EntRepository in ent_repository_backlog.go that queries the source_sync_events table, then implement GetSyncHistory in backlog_service.go to call it and map results to the GetSyncHistoryResponse proto.

### GAP-112 — `config.NotificationPrefs.PushEnabled` is defined but never consulted by `PushService`
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-CONFIG-NotificationPrefs-PushEnabled-dead
- **file**: `config/config.go`
- **detail**: The `NotificationPrefs.PushEnabled` field (config.go:180-182) is documented as controlling whether web push notifications are sent and defaults to false (opt-in). However, `PushService` in `server/services/push_service.go` never reads this field — it sends pushes to all registered subscriptions unconditionally. There is no proto representation and no settings UI for this field. The only code that references `Notifications.PushEnabled` outside of config.go is in `config_test.go`, making this a dead config field.
- **fix hint**: Either (a) add a gate in `PushService.Send` (or similar) that calls `config.LoadConfig().Notifications.PushEnabled` before delivering a notification, or (b) remove the `NotificationPrefs` struct and the `Notifications` field from `Config` since the push subscription mechanism already serves as the opt-in.

### GAP-113 — `WorkflowProto` has no `autonomous_mode` field — workflow-spawned sessions cannot self-complete
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-WORKFLOW-FireNow-autonomous_mode
- **file**: `server/workflows/scheduler.go`
- **detail**: `WorkflowProto`, the ent workflow schema, and `FireNow` all have no `autonomous_mode` field. `CreateSessionRequest.AutonomousMode` is wired and functional in session creation, but workflow-spawned sessions can never be started with an `AutonomousDriver` — they always require a human to keep the conversation going. This is especially relevant for cron-scheduled workflows intended to run unattended.
- **fix hint**: Add `autonomous_mode bool` to the ent workflow schema and `WorkflowProto`, thread it through `WorkflowService`, and pass `AutonomousMode: wf.AutonomousMode` in `FireNow`'s `CreateSessionRequest`. Add a checkbox to `WorkflowForm.tsx`.

### GAP-114 — `ResumeHibernatedSession` does not publish an `AddInstance` event, so `WatchSessions` clients never see the revived session
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-RESUME-HIBERNATION-poller-event
- **file**: `server/services/session_service.go`
- **detail**: After successful ResumeFromHibernation (line 1563), the handler publishes a SessionUpdated event (line 1572) but never calls s.reviewQueuePoller.AddInstance(instance). Since the session was removed from the poller during HibernateSession (line 1520), the resumed instance is not in the poller. WatchSessions streams events from poller instances; the revived session is invisible until the next server restart. ListSessions also reads from the poller (line 821), so it would return the session only if a falling-back path is triggered.
- **fix hint**: After s.storage.SaveInstances(instances) succeeds, call s.reviewQueuePoller.AddInstance(instance) and then wire all callbacks (see GAP-384 above). Combine both fixes in a single PR.

### GAP-115 — `UpdateSession` resumes Paused sessions when any non-PAUSED status is sent, not just ACTIVE
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UPDATE-paused-resume-on-any-non-paused-status
- **file**: `server/services/session_service.go`
- **detail**: The status-change block in `UpdateSession` (around line 1433) uses the condition `targetStatus != session.Paused && instance.Status == session.Paused` to decide whether to call `instance.Resume()`. This fires for ANY non-Paused target status, including STOPPED (3), CREATING (0), RESTORING (5), and HIBERNATED (4). So sending `UpdateSession(status=SESSION_STATUS_STOPPED)` for a Paused session calls `instance.Resume()` instead of transitioning to Stopped. The caller receives a success response with the session now Active, contradicting the requested target status. Only `SESSION_STATUS_ACTIVE` is semantically correct to trigger Resume, but the code does not restrict this.
- **fix hint**: Change the resume branch condition from `targetStatus != session.Paused` to `targetStatus == session.Active`, so only an explicit ACTIVE request resumes a paused session. For other non-Paused/non-Active values on a Paused session, return `CodeFailedPrecondition`.

### GAP-116 — `UpdateSession(autonomous_mode=true)` on a non-Active session sets the flag but silently fails to start the driver
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-UPDATE-autonomous-mode-enable-no-active-guard
- **file**: `server/services/session_service.go`
- **detail**: The autonomous mode toggle in `UpdateSession` (around lines 1375–1388) sets `instance.AutonomousMode = true` and calls `StartAutonomousDriverForInstance` without first checking that the session is Active. For Paused, Stopped, or Hibernated sessions there is no `ClaudeController`, so `AutonomousDriver.Start()` returns an error ('no controller available') which is only logged as a warning. `instance.AutonomousMode` remains `true` in memory and is returned in the proto response, giving the client a false signal that autonomous operation is active. No driver goroutine actually runs.
- **fix hint**: Before calling `StartAutonomousDriverForInstance` in the `UpdateSession` autonomous mode block, add: `if *req.Msg.AutonomousMode && !instance.IsActive() { return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot enable autonomous mode on a %s session — session must be Active", instance.Status)) }`. This prevents the silent false-positive state.

### GAP-117 — `ListSessionsRequest.project_id` filter is never applied by the handler
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-PROTO-ListSessions-project_id
- **file**: `server/services/session_service.go`
- **detail**: The proto field `optional string project_id = 5` in `ListSessionsRequest` is documented as filtering sessions by project ID, and the frontend calls `updateProject({ id, name })` expecting project-scoped session lists. However the `ListSessions` Go handler (lines 813–905) never reads `req.Msg.ProjectId` — the filter is silently ignored and all sessions are returned regardless. The `search_query` (field 4) and `hide_paused` (field 3) filters are similarly unread.
- **fix hint**: In the `ListSessions` loop, add: `if req.Msg.ProjectId != nil && *req.Msg.ProjectId != "" && inst.ProjectID != *req.Msg.ProjectId { continue }`. Apply analogous guards for `SearchQuery` and `HidePaused`.

### GAP-118 — `LogClientEvents` handler logs to stderr only, never writes to `AnalyticsEvent` DB table
- **status**: open
- **severity**: medium
- **fingerprint**: GAP-ENTITY-AnalyticsEvent-LogClientEvents-not-persisted
- **file**: `server/services/session_service.go`
- **detail**: The SessionService has an analyticsProvider field (line 138) populated via SetAnalyticsProvider (line 492), but the provider's Record() method is never called anywhere in session_service.go. The LogClientEvents handler (line 3463) receives batched browser analytics events and only calls logClientEntry() which writes to the server log — it does not call s.analyticsProvider.Record(). The AnalyticsEvent ent entity, its five DB indexes, and the SQLiteAnalyticsProvider are all fully implemented in server/analytics/, but the wire from the ConnectRPC handler to the storage layer is missing.
- **fix hint**: In the LogClientEvents handler, after calling logClientEntry(entry), also call s.analyticsProvider.Record(ctx, analytics.Event{...}) to persist each ClientLogEntry as an AnalyticsEvent. Guard with a nil check since the provider is wired asynchronously.

### GAP-119 — `ForkedFromID` is persisted in JSON storage but never surfaced in the proto `Session` message
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FORK-ForkedFromID-not-in-proto
- **file**: `session/instance_checkpoint.go`
- **detail**: ForkFromCheckpoint (line 132) sets newInst.ForkedFromID = i.Title to track the parent session of a checkpoint fork. This value is serialized through instance_serialization.go (lines 77, 227) and stored in the legacy JSON InstanceData (storage.go line 103 as forked_from_id). However, the proto types.proto Session message has no forked_from_id field, and InstanceToProto in server/adapters/instance_adapter.go never maps ForkedFromID to any proto field. Clients (web UI, API consumers) receive no information about the fork lineage — they cannot determine which session was forked from which, nor render fork-tree UI. The field is write-only from the client's perspective.
- **fix hint**: Add a string forked_from_id field to the Session proto message in proto/session/v1/types.proto, run make generate-proto, then map inst.ForkedFromID to that field in InstanceToProto in server/adapters/instance_adapter.go. This allows clients to display fork lineage.

### GAP-120 — `ListSessionsRequest.hide_paused` filter accepted but silently ignored by server
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FILTER-ListSessions-hide_paused-ignored
- **file**: `server/services/session_service.go`
- **detail**: The proto field `ListSessionsRequest.hide_paused` (session.proto:408) is sent by the frontend (`SessionList.tsx` stores the value in localStorage and sets it from config state), but the `ListSessions` handler never reads `req.Msg.HidePaused` or `req.Msg.GetHidePaused()`. The filtering is performed entirely client-side in `SessionList.tsx` line 390. Other filters in the same request (status, category, include_hidden, include_archived, workflow_id) are all applied server-side; `hide_paused` is the sole exception. This creates an implicit protocol inconsistency: the proto implies server-side filtering but the implementation relies on the client.
- **fix hint**: In the ListSessions handler (session_service.go), add a check after the status filter block: `if req.Msg.HidePaused && inst.IsPaused() { continue }`. This makes the server behavior match the proto contract and removes client-side duplicate filtering in SessionList.tsx.

### GAP-121 — `UpdateSession` silently ignores `status=SESSION_STATUS_ACTIVE` for Stopped sessions despite the state machine allowing Stopped→Active
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UPDATE-stopped-active-silent-noop
- **file**: `server/services/session_service.go`
- **detail**: The `transitionDefs` state machine in `state_machine.go` defines `Stopped → Active` as a valid transition (line 55). However, `UpdateSession`'s status-change block (lines 1418–1441) only handles `Paused → Active` (else-if at line 1433 requires `instance.Status == session.Paused`) and `Active → Paused`. If a client sends `status=SESSION_STATUS_ACTIVE` for a Stopped session, both branches evaluate to false and the RPC returns success with no transition and no error. There is no dedicated `ResumeStoppedSession` RPC, and `RestartSession` kills and recreates the tmux session (different semantics). This differs from the documented GAP-040 which covers HIBERNATED→ACTIVE.
- **fix hint**: In the `UpdateSession` status block, add a third condition: `else if targetStatus == session.Active && instance.Status == session.Stopped { return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot resume a Stopped session via UpdateSession — use RestartSession instead")) }`. This surfaces the unsupported transition as an error rather than a silent no-op.

### GAP-122 — `DetectionEventsPanel` only accessible via `?debug=1`
- **status**: open
- **severity**: low
- **file**: `web-app/src/components/sessions/SessionCard.tsx:808`
- **detail**: The detection events debug panel is hidden behind a `?debug=1` URL
  parameter. Useful for diagnosing stuck/misclassified sessions but completely
  invisible to normal users.
- **fix hint**: Surface in the session detail "Debug" tab or as a developer mode toggle
  in settings.

### GAP-123 — `LogUserInteraction` audit trail is write-only
- **status**: open
- **severity**: low
- **file**: `web-app/src/` (no read-back component)
- **detail**: Interaction events (notification clicks, review queue actions, etc.) are
  stored server-side via `LogUserInteraction` but no frontend component ever calls back
  to read them. The audit log accumulates data that is never surfaced.
- **fix hint**: Add a read endpoint + admin view, or decide to drop the audit log if
  it's not needed.

### GAP-124 — Dual escape analytics systems with separate stores
- **status**: open
- **severity**: low
- **files**: `server/services/escape_code_handler.go` (old REST + in-memory),
  `server/services/analytics_escape_service.go` (new ConnectRPC + ent DB)
- **detail**: Two independent tracking paths exist. The nav points to the new SQLite-
  backed one at `/analytics/escape`. The old REST debug page at `/debug/escape-codes`
  uses a completely separate in-memory `EscapeCodeHandler` that requires manual toggling
  and writes to a different store. They are not connected.
- **fix hint**: Tombstone or remove `EscapeCodeHandler` and the `/debug/escape-codes`
  route now that the proto-based system captures events automatically at the terminal
  pipeline layer.

### GAP-125 — Duplicate `LogViewer` component sets
- **status**: open
- **severity**: low
- **files**: `web-app/src/components/logs/`, `web-app/src/components/shared/`
- **detail**: Both directories contain `LogViewer.tsx`, `LogRow.tsx`,
  `ExpandedLogDetail.tsx`, `VirtualLogList.tsx`. The `shared/` set is the live one used
  by both the session logs tab and the global logs page. The `components/logs/` set
  contributes toolbar chrome only.
- **fix hint**: Audit and delete dead files in `components/logs/`; move any surviving
  pieces into `shared/`.

### GAP-126 — `WatchInsightsRequest` `from`/`to` time-range fields silently ignored
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-WatchInsights-from-to-ignored
- **file**: `server/services/insights_service.go`
- **detail**: The `WatchInsightsRequest` proto message declares `from` and `to` Timestamp fields intended to scope the streaming subscription to a time range. The `WatchInsights` handler uses `_` for the request parameter, discarding it entirely. Update events are forwarded unconditionally regardless of the requested time window.
- **fix hint**: Capture the request parameter and apply the `from`/`to` filter when forwarding events from the TokenStore subscriber channel, skipping events whose `SessionTokenSummary.first_message_at` or `last_message_at` fall outside the requested range.

### GAP-127 — `CreateSessionRequest.profile` field handled server-side but never sent from the live frontend
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CREATEREQ-profile-no-live-path
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: The profile field (field 11 in CreateSessionRequest) triggers config.ResolveDefaults on the server (session_service.go around line 1053), applying named-profile and directory-rule defaults. However, useSessionService.createSession, OmnibarContext.handleCreateSession, and the OmnibarSessionData interface (Omnibar.tsx) all omit the profile field entirely. The only component that uses useSessionDefaults (which calls resolveDefaults with a profileName) is SessionWizard.tsx, which is an orphaned component not mounted from any live route. As a result, profile-based and directory-rule defaults are silently never applied in the primary session creation flow.
- **fix hint**: Add a profile selector to OmnibarCreationPanel using GetSessionDefaults to enumerate available profiles. Extend OmnibarSessionData with an optional profile string, thread it through OmnibarContext.handleCreateSession and useSessionService.createSession, and include it in the CreateSessionRequest body.

### GAP-128 — `routes.settingsDefaults` is a dead constant — page just redirects and constant is never used
- **status**: open
- **severity**: low
- **fingerprint**: GAP-ROUTE-SettingsDefaults-DeadConstant
- **file**: `web-app/src/lib/routes.ts`
- **detail**: `routes.settingsDefaults` is defined as `/settings/defaults` (line 17 of `routes.ts`). The page at `web-app/src/app/settings/defaults/page.tsx` immediately issues a server-side `redirect('/settings')`. A search finds zero references to `routes.settingsDefaults` outside of `routes.ts` itself. The redirect page and the dead constant together indicate an abandoned migration of settings structure.
- **fix hint**: Delete `routes.settingsDefaults` from `routes.ts` and either remove `web-app/src/app/settings/defaults/page.tsx` or convert it to a real page. If old deep-links must keep working, the redirect is fine but the constant should be removed since nothing generates those links.

### GAP-129 — Duplicate escape-code analytics pages with no cross-linking
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-debug-escape-codes-duplicate
- **file**: `web-app/src/app/debug/escape-codes/page.tsx`
- **detail**: The `NAV_PAGES` entry `routes.escapeAnalytics` points to `/analytics/escape`. A completely separate page at `/debug/escape-codes` shows raw escape-code tracking with enable/disable/clear/export controls, linked only from the always-visible wrench `DebugMenu`. These are two different implementations for escape-code inspection with no cross-linking.
- **fix hint**: Decide which page is canonical: merge the debug controls into `EscapeAnalyticsPage` and remove `/debug/escape-codes`, or add `/debug/escape-codes` to `NAV_PAGES` so it is discoverable alongside the other page.

### GAP-130 — `/test-terminal` page undiscoverable — no nav entry or in-app link
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-test-terminal-undiscoverable
- **file**: `web-app/src/app/test-terminal/page.tsx`
- **detail**: The test-terminal page is a full terminal flickering/performance test tool referenced in e2e tests, but has no nav entry, no `DebugMenu` link, and no link from any other page. Unlike the `/test/*` pages (which `ConditionalHeader` strips the header from), `/test-terminal` is not under the `/test/` prefix so it renders with the full app shell but is only reachable by typing the URL directly.
- **fix hint**: Either move the page to `/test/terminal-flickering` or add a link in the `DebugMenu` alongside the escape-codes entry, and update the e2e test URL accordingly.

### GAP-131 — Config.Notifications.PushEnabled is persisted but never read by any server code
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CONFIG-NotificationPrefs-PushEnabled-dead-field
- **file**: `config/config.go`
- **detail**: `Config.Notifications.PushEnabled` (JSON: `push_enabled`) is defined in `config/config.go` with a comment saying it controls whether web push notifications are sent. However, a search of all Go files confirms the field is never read anywhere outside of its declaration. The push notification subsystem (`server/services/push_service.go`) manages subscriptions independently via browser Web Push API and ignores this config flag entirely. The `PushNotificationSettings` UI component also bypasses this field, toggling push subscriptions directly. There is no proto field and no RPC for this config value.
- **fix hint**: Either (a) remove the dead `NotificationPrefs` struct and `Notifications` field from `Config`, or (b) wire up `cfg.Notifications.PushEnabled` in `push_service.go` to gate whether notifications are delivered, and add a proto field + settings UI toggle to let users control it.

### GAP-132 — `config.BranchPrefix` has no proto field and no settings UI
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CONFIG-BranchPrefix-no-proto-no-ui
- **file**: `config/config.go`
- **detail**: The `BranchPrefix` field (config.go:224-225) controls the prefix prepended to all git branch names created during new-worktree sessions (defaults to `username/`). It is actively used in `session/git/worktree.go:139`. There is no proto field exposing it and no settings page UI control for it. Users who want branches prefixed differently (e.g., `feat/` instead of `tstapler/`) must edit `~/.stapler-squad/.../config.json` directly.
- **fix hint**: Add a `branch_prefix` field to `UpdateGlobalDefaultsRequest` and `SessionDefaultsConfig` proto messages, wire the field in `defaults_service.go`'s `UpdateGlobalDefaults` handler and `sessionDefaultsToProto` helper, and add a text input in `GlobalDefaultsForm.tsx` beneath the program selector.

### GAP-133 — Workflow `model` field is silently dropped when `agentType` is a non-Claude program
- **status**: open
- **severity**: low
- **fingerprint**: GAP-WORKFLOW-FireNow-model-non-claude-ignored
- **file**: `server/workflows/scheduler.go`
- **detail**: In `FireNow` (lines 159-165), the `wf.Model` value is only applied when `wf.AgentType` is empty or `"claude"` — it is appended as `claude --model <model>` to the program string. When `wf.AgentType` is any other value (e.g., `"aider"`, `"codex"`), the model field is silently ignored with no log or error. A user who sets both `agentType = "aider"` and `model = "claude-opus-4"` in a workflow will see the model discarded without any feedback.
- **fix hint**: When `wf.Model != ""` and `!isClaudeProgram`, either log a warning that the model field is unsupported for the given agent type, or store model as a separate `CreateSessionRequest` field once the API supports it. At minimum, add a validation warning in `WorkflowService.CreateWorkflow` and `UpdateWorkflow` when `model` is set alongside a non-Claude `agentType`.

### GAP-134 — `UpdateSession` status change from `HIBERNATED` to `ACTIVE` is a silent no-op
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UPDATE-hibernated-status-silent-noop
- **file**: `server/services/session_service.go`
- **detail**: The `UpdateSession` handler only handles the Paused↔Active transition. If a client sends `status=SESSION_STATUS_ACTIVE` for a session in `SESSION_STATUS_HIBERNATED` state, both condition branches are false, so no transition occurs and the RPC succeeds without changing anything. The dedicated `ResumeHibernatedSession` RPC is the correct path, but there is no error or warning when the wrong API path is used.
- **fix hint**: In the `UpdateSession` status block, add a case for `instance.Status == session.Hibernated`: return `connect.NewError(connect.CodeFailedPrecondition, ...)` directing callers to use `ResumeHibernatedSession` instead.

### GAP-135 — `Session.claude_conversation_uuid` (field 42) never set in `InstanceToProto` despite data being available
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-Session-claude_conversation_uuid-unpopulated
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The top-level `Session.claude_conversation_uuid` field (types.proto:150) is documented as 'Claude Code conversation UUID extracted from the history file path.' The same value is already reachable via `session.claude_session.session_id` which IS populated in the adapter (line 91: `SessionId: cs.ConversationUUID`). However, the dedicated top-level field is never assigned in `InstanceToProto`, so clients that access `session.claude_conversation_uuid` directly always receive an empty string. The frontend does not currently reference this field directly, but it is a broken contract for any API consumer relying on the top-level shortcut.
- **fix hint**: In `InstanceToProto`, after setting `ClaudeSession`, add: `if cs := inst.GetClaudeSession(); cs != nil { protoSession.ClaudeConversationUuid = cs.ConversationUUID }`. Alternatively, remove field 42 from the proto if the nested `claude_session.session_id` is the canonical accessor.

### GAP-136 — `/test/escape-codes` and `/test/layout-overlap` pages exist but have no route constants or nav entries
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-test-pages-no-route-constants
- **file**: `web-app/src/app/test/escape-codes/page.tsx`
- **detail**: Two developer/Playwright harness pages exist under /test/: escape-codes (a client-side escape code library test runner, distinct from /debug/escape-codes and /analytics/escape) and layout-overlap (a static shell for measuring modal positions). Neither appears in routes.ts nor nav-pages.ts. The companion /test/terminal-stress page is referenced in tests/e2e/terminal-stress/helpers.ts but is also absent from routes.ts. Without route constants, Playwright tests must hardcode strings and the pages are undiscoverable.
- **fix hint**: Add route constants to routes.ts: `testEscapeCodes: '/test/escape-codes'`, `testLayoutOverlap: '/test/layout-overlap'`, `testTerminalStress: '/test/terminal-stress'`, then update tests/e2e/terminal-stress/helpers.ts to use `routes.testTerminalStress`.

### GAP-137 — `browser-passthrough` and `backlog:conversation-view` feature flags render as raw machine names in settings UI
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CONFIG-FeatureFlags-missing-FEATURE_META-labels
- **file**: `web-app/src/app/settings/features/page.tsx`
- **detail**: knownFeatureFlags in server/services/session_service.go (lines 3756-3768) defines three flags: backlog, browser-passthrough, and backlog:conversation-view. The frontend FEATURE_META map (line 24-26) only has an entry for backlog with label 'Backlog'. The other two flags are returned by GetFeatureFlags and rendered as toggle controls, but FEATURE_META[name] is undefined for them so label falls back to the raw machine name (e.g. 'browser-passthrough' and 'backlog:conversation-view').
- **fix hint**: Add entries to FEATURE_META in web-app/src/app/settings/features/page.tsx for 'browser-passthrough' (label: 'Browser Passthrough') and 'backlog:conversation-view' (label: 'Backlog Conversation View') to match the descriptions already available from the backend.

### GAP-138 — `DetectionEventProto.tail_snippet` never populated by `GetDetectionEvents` handler
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-DetectionEvent-tail_snippet-unpopulated
- **file**: `server/services/session_service.go`
- **detail**: The proto field `DetectionEventProto.tail_snippet` (field 6) is documented as 'last 512 bytes of cleaned terminal output' and the underlying Go `DetectionEvent.TailSnippet` is faithfully populated by the detection event sink (`session/detection/event_sink.go`). However, the `GetDetectionEvents` handler at line 3984 builds the proto response by copying only fields 1–5 (`SessionId`, `Timestamp`, `MatchedPattern`, `MatchedCategory`, `ResultStatus`) and silently omits `TailSnippet`. The frontend `DetectionEventsPanel` renders these events for debugging, so callers always receive an empty `tail_snippet` even when the data is available.
- **fix hint**: In the `GetDetectionEvents` handler response loop (`session_service.go` ~line 3984), add `TailSnippet: e.TailSnippet` to the `DetectionEventProto` struct literal.

### GAP-139 — `SessionWizard` component is never imported or rendered anywhere in the application
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-SessionWizard-dead-component
- **file**: `web-app/src/components/sessions/SessionWizard.tsx`
- **detail**: `SessionWizard.tsx` is a full multi-step form component with profile selection, defaults resolution badges, save-as-profile functionality, and branch autocomplete. It exports a `SessionWizard` function component with a `SessionWizardProps` interface and an `onComplete` callback, but it is never imported in any page or layout file. The `?new=true` URL route (`routes.ts`) opens the Omnibar (`openOmnibar()`), not `SessionWizard`. E2E tests named 'SessionWizard creation flow' actually test the Omnibar, as confirmed by `page.tsx` line 208. The component is dead code that diverges from the active session creation UI.
- **fix hint**: Either wire `SessionWizard` into the application (e.g., add a route or modal trigger that renders it instead of or alongside the Omnibar for advanced session creation), or remove the file and its associated `SessionWizard`-named E2E tests to reduce maintenance burden.

### GAP-140 — `logs/page.tsx` falls back to client-side multi-level filtering despite the server already supporting `levels[]`
- **status**: open
- **severity**: low
- **fingerprint**: GAP-LOGS-multiLevel-client-side-filter
- **file**: `web-app/src/app/logs/page.tsx`
- **detail**: The `GetLogsRequest` proto has a `repeated string levels` field (field 8) that takes precedence over the deprecated single `level` field. The Go backend's `parseLogs` builds a `levelFilterSet` from `req.Levels` when non-empty. However `logs/page.tsx` has a comment 'backend supports single level; for multi-level we'd need API update' and filters multi-level selections client-side on the returned subset. This means pagination is broken for multi-level filters: only the first page of unfiltered entries is returned, then filtered locally, so later pages may be missed.
- **fix hint**: In `logs/page.tsx`'s `fetchLogs` and `loadMoreLogs` functions, remove the `singleLevelFilter` fallback and instead always pass `levels: levelFilters.filter(l => l !== 'ALL')` directly to `getLogs`, and remove the client-side filter block.

### GAP-141 — `ProtoToSessionType` maps `SESSION_TYPE_NEW_PROJECT` to `SessionTypeDirectory` instead of `SessionTypeNewProject`
- **status**: open
- **severity**: low
- **fingerprint**: GAP-ADAPTER-ProtoToSessionType-newProject-silently-directory
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The `ProtoToSessionType` function at lines 347–358 handles `DIRECTORY`, `NEW_WORKTREE`, and `EXISTING_WORKTREE` but has no case for `SESSION_TYPE_NEW_PROJECT` (wire value 4). Any code path that calls `ProtoToSessionType` with a `NEW_PROJECT` value silently gets `SessionTypeDirectory` back. The outbound direction (`sessionTypeToProto`) has the same omission (covered by GAP-066), but the inbound conversion is a separate gap. Currently, `BatchCreateSessions` uses `batchReq.SessionType` directly mapped through `CreateSessionRequest` which uses `resolveSessionType`, so the critical creation path is not affected.
- **fix hint**: Add `case sessionv1.SessionType_SESSION_TYPE_NEW_PROJECT: return session.SessionTypeNewProject` to `ProtoToSessionType` in `server/adapters/instance_adapter.go`.

### GAP-142 — `VNC_STATUS_PASSTHROUGH` not handled in `BrowserTab` — falls through to 'No browser open yet'
- **status**: open
- **severity**: low
- **fingerprint**: GAP-VNC-PASSTHROUGH-unhandled-in-BrowserTab
- **file**: `web-app/src/components/sessions/BrowserTab.tsx`
- **detail**: `VNCStatus.VNC_STATUS_PASSTHROUGH` (value 5) means a pre-existing X display was detected and reused, with x11vnc not running. In `BrowserTab.tsx`, `isReady` requires `VNC_STATUS_READY && browserWindowDetected`, and `isWaiting` catches everything else that is not UNAVAILABLE/UNSPECIFIED. A PASSTHROUGH session therefore shows 'No browser open yet' rather than a passthrough-specific message or a ready CDPViewer. The `SessionDetailView` does enable the Browser tab (PASSTHROUGH is not UNAVAILABLE), so the tab appears but shows confusing content.
- **fix hint**: Add an explicit `isPassthrough = status === VNCStatus.VNC_STATUS_PASSTHROUGH` check in `BrowserTab.tsx` and either treat it as ready (mount CDPViewer immediately) or render a distinct 'Using existing display' overlay message.

### GAP-143 — `ListBranchesRequest` filter/maxResults/includeRemote fields never sent from `useBranchSuggestions`
- **status**: open
- **severity**: low
- **fingerprint**: GAP-BRANCH-ListBranches-filters-never-sent
- **file**: `web-app/src/lib/hooks/useBranchSuggestions.ts`
- **detail**: `ListBranchesRequest` exposes `filter` (server-side substring filter), `maxResults`, and `includeRemote` fields, but `useBranchSuggestions.ts` only sends `repoPath`. All filtering is absent, so the server always returns up to its default 200 branches and the caller receives the full unfiltered set. For repositories with many branches, this can be unnecessarily large, and typing-based filtering could be delegated to the server.
- **fix hint**: Pass the current search/filter input as `filter` and set a reasonable `maxResults` (e.g. 50) in the `listBranches` call in `useBranchSuggestions.ts`.

### GAP-144 — `ListClaudeHistoryRequest.project` filter field never used by the history page
- **status**: open
- **severity**: low
- **fingerprint**: GAP-HISTORY-ListClaudeHistory-project-filter-unused
- **file**: `web-app/src/app/history/page.tsx`
- **detail**: `ListClaudeHistoryRequest` defines a `project` field (proto field 1) that filters history entries by project path. The history page (`page.tsx`) uses `searchQuery` for text search but never passes a project path filter, so all history entries across every project are always returned and text-search is the only filtering mechanism. The `project` field on returned entries is read (to display/navigate), but is never used as a request filter.
- **fix hint**: Add a project path selector to the history page UI and pass the selected path as the `project` field of `ListClaudeHistoryRequest`.

### GAP-145 — `/backlog/board` page linked via hardcoded string instead of `routes.backlogBoard` constant
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-BacklogBoard-hardcoded-href
- **file**: `web-app/src/app/backlog/page.tsx`
- **detail**: The backlog board page (`/backlog/board`) is a real, functional page and has `routes.backlogBoard` defined in `routes.ts`. However, the link in `web-app/src/app/backlog/page.tsx` (line 308) uses the hardcoded string `/backlog/board` rather than `routes.backlogBoard`. The board page also has no nav entry in `NAV_PAGES` and no `featureFlag` gate — meaning it is accessible even when the `backlog` feature flag is disabled, but only if the user already knows the URL.
- **fix hint**: In `web-app/src/app/backlog/page.tsx`, replace the hardcoded `href="/backlog/board"` with `href={routes.backlogBoard}`. Also consider whether the board page should be gated by the `backlog` feature flag (it currently is not, unlike the nav entry for `/backlog`).

### GAP-146 — `routes.settingsUnfinished` constant defined but never referenced anywhere
- **status**: open
- **severity**: low
- **fingerprint**: GAP-ROUTES-settingsUnfinished-dead-constant
- **file**: `web-app/src/lib/routes.ts`
- **detail**: routes.ts defines `settingsUnfinished: "/settings/unfinished"` at line 19, but this constant is never imported or used in any component, hook, or nav configuration. The `/settings/unfinished` page (`web-app/src/app/settings/unfinished/page.tsx`) renders a real `UnfinishedSourcesSettings` component but is unreachable through any nav entry, and the route constant intended to link to it is a dead symbol. This is a distinct issue from GAP-049 (page has no inbound link) — the route constant itself is an orphaned definition.
- **fix hint**: Either add `routes.settingsUnfinished` to `NAV_PAGES` in `web-app/src/lib/nav-pages.ts` (under Settings) so the Unfinished Sources settings are discoverable, or inline the settings component into the main settings page tabs and delete the dead route constant.

### GAP-147 — `ListSessions` workflow_id filter not applied to external (mux) sessions
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FILTER-ListSessions-workflow_id-external-sessions
- **file**: `server/services/session_service.go`
- **detail**: The managed-session loop in ListSessions correctly skips sessions whose WorkflowID doesn't match req.Msg.WorkflowId (lines ~857). The external sessions loop that follows (lines ~870–892, driven by externalDiscovery.GetSessions()) applies status, category, and include_hidden filters but has no workflow_id check. External sessions (from ExternalSessionDiscovery / ssq-mux) never belong to a workflow, so when a caller requests sessions filtered by workflow_id=X, external sessions incorrectly pass through and appear in the response alongside the workflow's sessions.
- **fix hint**: In the external sessions loop, add an early continue when a workflow_id filter is active: if req.Msg.WorkflowId != nil && *req.Msg.WorkflowId != "" { continue }. External sessions have no workflow affiliation, so they should always be excluded from workflow-scoped queries.

### GAP-148 — `UpdateWorkflow` allows `cron_enabled=true` with existing empty `cron_expression` via partial update
- **status**: open
- **severity**: low
- **fingerprint**: GAP-SCHED-UpdateWorkflow-cronEnabled-partial-validation
- **file**: `server/services/workflow_service.go`
- **detail**: The validation in `UpdateWorkflow` (lines 184–188) only rejects the combination `cron_enabled=true` + `cron_expression=""` when both fields are present in the same request. A partial update that sends only `{id, cron_enabled: true}` on a workflow whose stored `cron_expression` is already empty passes validation and persists `cron_enabled=true` with `cron_expression=""` to the DB. The scheduler's `Reload` then calls `addCronEntry`, which logs a warning (`workflow %q has cron_enabled=true but empty cron_expression`) and returns an error that is silently swallowed. The workflow appears enabled in the UI but never fires.
- **fix hint**: In `UpdateWorkflow`, after applying the update to `WorkflowUpdateInput`, fetch the existing workflow from the repo when only `cron_enabled=true` is being set (and `cron_expression` is nil in the update), and verify that the existing `cron_expression` is non-empty before saving. Alternatively, re-validate after the `repo.Update` call by checking `wf.CronEnabled && wf.CronExpression == ""` and returning an error if true.

### GAP-149 — `ForkFromCheckpoint` does not copy `ProjectID` from source, losing project association on fork
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FORK-ForkFromCheckpoint-ProjectID-not-inherited
- **file**: `session/instance_checkpoint.go`
- **detail**: ForkFromCheckpoint builds InstanceOptions (lines 105–114) copying Title, Path, WorkingDir, Program, AutoYes, Category, Tags, and ResumeId from the source, but not ProjectID. The ForkSession handler also does not patch newInst.ProjectID after the call. As a result, forked sessions always have ProjectID='', even when the source session belongs to a project. Users who fork to branch a workflow within a project lose the project grouping silently.
- **fix hint**: In ForkFromCheckpoint (instance_checkpoint.go line 105), add ProjectID: i.ProjectID to the InstanceOptions struct. No changes needed in the ForkSession handler since ForkFromCheckpoint owns the copy logic.

### GAP-150 — `ConditionalHeader.tsx` wraps the dead `Header` but is itself never imported
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-ConditionalHeader-dead-component
- **file**: `web-app/src/components/layout/ConditionalHeader.tsx`
- **detail**: ConditionalHeader wraps Header.tsx with pathname-based suppression logic (hiding the header on /login and /test/* routes). It is exported but never imported anywhere in the application — grep across all .tsx/.ts files finds zero consumers. The test-page suppression logic it contains (line 8) therefore has no effect in production.
- **fix hint**: Either delete ConditionalHeader.tsx (since it wraps a dead component) or repurpose it as the mechanism to conditionally hide DrawerNav/BottomNav on /login and /test/* pages if that behavior is wanted.

### GAP-151 — `RunOneShotResponse.branch_diverged_from_base` silently dropped at review-queue adapter
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-RunOneShotResponse-branchDivergedFromBase-dropped
- **file**: `web-app/src/app/review-queue/page.tsx`
- **detail**: The Go handler at `session_service.go:3256` actively computes `branchDiverged` via `checkBranchDivergence` and sets `BranchDivergedFromBase` in `RunOneShotResponse` (field 5). However, the adapter in `review-queue/page.tsx:61` only maps `{ prUrl: response.prUrl, error: response.error }`, dropping `branchDivergedFromBase`. The `ReviewQueuePanel.onRunOneShot` prop callback is typed as returning `{ prUrl?: string; error?: string }` with no `branchDivergedFromBase` field, so there is no channel to surface the branch divergence warning after a Create PR one-shot run.
- **fix hint**: Extend the `onRunOneShot` callback return type in `ReviewQueuePanel` to include `branchDivergedFromBase?: boolean`, update the adapter at `review-queue/page.tsx:61` to pass `response.branchDivergedFromBase`, and wire the value into the post-PR result display alongside the `prResult.prUrl` link.

### GAP-152 — `WorkflowScheduler.eventBus` is injected but never used — no workflow-fired events published
- **status**: open
- **severity**: low
- **fingerprint**: GAP-SCHED-eventBus-dead-injected-field
- **file**: `server/workflows/scheduler.go`
- **detail**: The `Scheduler` struct has an `eventBus *events.EventBus` field (line 31) that is accepted in `NewScheduler` (line 37) and stored (line 46), but is never referenced anywhere else in the file. When a cron job fires via `FireNow` or when a user triggers `RunWorkflow`, no workflow-specific event (e.g., 'workflow fired', 'cron triggered') is published on the bus. The `WorkflowsPanel` 'Recent Runs' accordion has no live-refresh path from the scheduler — it relies on the user clicking the toggle to reload runs.
- **fix hint**: Either publish a workflow-fired event (e.g., `EventWorkflowFired`) from `FireNow` after the session is created so the `WorkflowsPanel` can subscribe and refresh, or remove the unused field from `Scheduler` to avoid confusion.

### GAP-153 — `ListSessions` accepts `hide_paused` and `search_query` proto fields but never applies them server-side
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-ListSessions-hide_paused-search_query-ignored
- **file**: `server/services/session_service.go`
- **detail**: The `ListSessionsRequest` proto defines `bool hide_paused = 3` and `optional string search_query = 4`. The `ListSessions` handler never reads either field; all filtering for these criteria is done client-side in `SessionList.tsx`. This means the full session list is always transferred over the wire even when the caller only wants non-paused or matching sessions, and any non-browser caller that passes these fields gets unfiltered results.
- **fix hint**: Either implement server-side filtering in `ListSessions` (add checks for `req.Msg.HidePaused` and `req.Msg.SearchQuery` against `inst.Status` and `inst.Title`/`inst.Branch`), or remove the fields from the proto if purely client-side filtering is intentional.

### GAP-154 — `BacklogService.SuggestNextItem` implemented but no frontend entry point
- **status**: open
- **severity**: low
- **fingerprint**: GAP-RPC-BacklogService-SuggestNextItem-no-frontend-caller
- **file**: `web-app/src/lib/hooks/useBacklogService.ts`
- **detail**: SuggestNextItem is fully implemented (server/services/backlog_service.go line 1286) and returns the highest-priority ready backlog item based on priority sort. The hook useBacklogService exposes listBacklogItems, spawnSessionFromItem, and triggerTriage but not suggestNextItem. No component calls it, so the AI-assisted 'what should I work on next?' capability is unreachable from the UI.
- **fix hint**: Add a `suggestNextItem()` function to useBacklogService.ts. Wire it to a 'Suggest next' button on the backlog board page (web-app/src/app/backlog/board/page.tsx) that highlights or auto-selects the recommended item.

### GAP-155 — `useBacklogService` hardcodes `sortBy: ''` — server-side sorting never used
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FILTER-BacklogService-sortBy-hardcoded-empty
- **file**: `web-app/src/lib/hooks/useBacklogService.ts`
- **detail**: The listBacklogItems call in useBacklogService.ts always passes `sortBy: ""` (line 321), which causes the backend to fall through to its default ordering. The backend supports `"priority"` and `"updated_at"` sort modes (session/ent_repository_backlog.go switch at line 150). The ListBacklogItemsFilter interface exposes a `search` field that is applied client-side after fetching all items, but there is no `sortBy` field exposed to callers at all, so sorting is entirely locked out.
- **fix hint**: Add a `sortBy?: 'priority' | 'updated_at'` field to ListBacklogItemsFilter in useBacklogService.ts and thread it into the RPC call. Expose a sort control in the backlog page UI (web-app/src/app/backlog/page.tsx).

### GAP-156 — `routes.settingsDefaults` is defined and has a redirect page but is never referenced in frontend code
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-settingsDefaults-dead-route
- **file**: `web-app/src/lib/routes.ts`
- **detail**: `routes.settingsDefaults` (`/settings/defaults`) is defined in `routes.ts` and has a corresponding Next.js page that simply redirects to `/settings`. However, `routes.settingsDefaults` is never imported or used anywhere else in the codebase — no component links to it and no nav entry references it. The route constant is effectively dead code.
- **fix hint**: Remove `settingsDefaults` from `routes.ts` and delete `web-app/src/app/settings/defaults/page.tsx`. If some external tool deep-links to `/settings/defaults`, keep only the redirect page and remove the route constant.

### GAP-157 — `/debug/escape-codes` is linked from `DebugMenu` with a hardcoded string but has no `routes.ts` entry
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-debug-escape-codes-hardcoded-href
- **file**: `web-app/src/components/ui/DebugMenu.tsx`
- **detail**: The DebugMenu component links to `/debug/escape-codes` via a hardcoded string literal (`href="/debug/escape-codes"`). This path does not have a corresponding entry in `web-app/src/lib/routes.ts`, so the URL has no type-safe constant and could silently drift if the page is moved. There is also a separate `/analytics/escape` page (`routes.escapeAnalytics`) that appears to serve a similar purpose (analytics for escape codes), potentially causing confusion about which page to use.
- **fix hint**: Add `debugEscapeCodes: "/debug/escape-codes"` to `routes.ts` and update `DebugMenu.tsx` to reference `routes.debugEscapeCodes`. Evaluate whether `/debug/escape-codes` and `/analytics/escape` (`EscapeAnalyticsPage`) should be consolidated into one route.

### GAP-158 — Dev/test pages shipped in production with no `routes.ts` entries
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-test-pages-no-routes-constants
- **file**: `web-app/src/app/test-terminal/page.tsx`
- **detail**: Four pages intended for Playwright E2E tests and terminal performance profiling are compiled into the production Next.js bundle but have no entries in `routes.ts`. E2E tests reference them via hardcoded URL strings (e.g., `page.goto('/test-terminal')`). Because these pages lack `analytics-exempt` markers where relevant or consistent naming conventions, they can pollute analytics and are reachable by any user who guesses the URL.
- **fix hint**: Add route constants for these pages to `routes.ts` so references are type-safe. If they should not be in production, move them behind a build-time `NODE_ENV === 'development'` guard or use Next.js `instrumentation` to restrict access. The `/test-terminal` page is missing the `// analytics-exempt` comment that the other test pages use.

### GAP-159 — `ListSessionsRequest.search_query` filter field is accepted but never processed
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-ListSessions-search_query-ignored
- **file**: `server/services/session_service.go`
- **detail**: The proto `ListSessionsRequest` message declares a `search_query` field (field 4) with documentation indicating fuzzy matching across title, path, and branch. The `ListSessions` handler (starting at line 813) reads status, category, include_hidden, include_archived, and workflow_id filters but never reads `req.Msg.SearchQuery`. The frontend also does not send this field — it performs client-side filtering in SessionList.tsx. The field is dead code on both ends.
- **fix hint**: Implement server-side search in ListSessions by filtering instances whose Title, Path, or Branch contains the search_query substring (case-insensitive), or remove the field from the proto if client-side filtering is the intended design.

### GAP-160 — `DrawerNav` (desktop nav) has no Account link when auth is enabled
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-DrawerNav-no-account-link
- **file**: `web-app/src/components/layout/DrawerNav.tsx`
- **detail**: When passkey auth is enabled, an Account link is rendered conditionally in BottomNav's More sheet (mobile) and was in the now-dead Header component. DrawerNav — the only nav rendered on desktop — does not include an Account link at all, meaning authenticated desktop users have no nav-visible path to /account. The account route is defined in routes.ts and a full page exists at /account, but it is unreachable from the desktop drawer.
- **fix hint**: Add an auth-conditional Account link to DrawerNav after the main NAV_PAGES list, mirroring the pattern in BottomNav (lines 143–152).

### GAP-161 — ReviewVerdict.prompt_hash is stored in DB but never read back or exposed in proto
- **status**: open
- **severity**: low
- **fingerprint**: GAP-SCHEMA-ReviewVerdict-prompt_hash-write-only
- **file**: `session/storage_backlog.go`
- **detail**: `ReviewVerdictData.PromptHash` (defined at line 31) is written to the `review_verdict` table via `SetNillablePromptHash` in three places in `storage_backlog.go` (lines 277, 295, 344). It is never read back: `itemSessionToProto` in `server/services/backlog_service.go` does not include it, no proto message (proto/session/v1/backlog.proto `ReviewVerdict` message) has a `prompt_hash` field, and no code path reads `rv.PromptHash`. The field appears intended as a dedup key for review prompts but has no consumer.
- **fix hint**: If prompt_hash is meant as a server-side dedup guard (to skip re-reviewing the same diff+prompt combination), add a query in the review lifecycle code to check for an existing verdict with the same prompt_hash before spawning a new review. If it should also be exposed to the frontend, add a `prompt_hash` field to the `ReviewVerdict` proto message and populate it in `itemSessionToProto`.

### GAP-162 — `ItemSession.last_progress_at` is written but never read or exposed
- **status**: open
- **severity**: low
- **fingerprint**: GAP-SCHEMA-ItemSession-last_progress_at-write-only
- **file**: `session/storage_backlog.go`
- **detail**: The `item_session` ent schema defines a `last_progress_at` (Optional, Nillable Time) field. It is written in two places — `UpdateItemSessionGitActivity` (set to `commitAt`) and `UpdateItemSessionFileTouch` (set to `touchAt`) — but is never read in any query, never mapped to `BacklogItemData` or the `ItemSession` domain model, and does not appear in the proto `ItemSession` message. The field was presumably intended for stale-session detection or SLA monitoring but the read path was never implemented.
- **fix hint**: Either add `last_progress_at` to the proto `ItemSession` message in `backlog.proto` and populate it in `itemSessionToProto()`, or remove the field from the schema if the use case has been abandoned.

### GAP-163 — `DeleteWorkflowFailedSessions` archives sessions in DB but does not update in-memory `reviewQueuePoller` state
- **status**: open
- **severity**: low
- **fingerprint**: GAP-WORKFLOW-DeleteFailedSessions-no-memory-sync
- **file**: `server/services/session_service.go`
- **detail**: `ArchiveWorkflowSessions` (lines 4073-4082) correctly iterates `reviewQueuePoller.GetInstances()` and sets `inst.ArchivedAt` in memory so `ListSessions` (which reads from poller) immediately reflects the change. `DeleteWorkflowFailedSessions` (lines 4095-4134) only calls `entClient.Session.Update()` and returns without updating any in-memory instance, so sessions archived by this RPC continue to appear as non-archived in `ListSessions` responses until the server restarts or those instances leave the poller.
- **fix hint**: After the `entClient.Session.Update()` call in `DeleteWorkflowFailedSessions`, add the same in-memory sync block as `ArchiveWorkflowSessions`: iterate `s.reviewQueuePoller.GetInstances()`, find instances matching `WorkflowID` and `Stopped` status with nil `LastMeaningfulOutput`, and set their `ArchivedAt` pointer.

### GAP-164 — CreateSessionRequest.skipDefaults field never sent — frontend cannot bypass profile/directory defaults
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CREATE-skipDefaults-never-sent
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: The proto field `bool skip_defaults = 12` in CreateSessionRequest causes the backend to bypass profile inheritance and directory-rule default resolution (session_service.go lines 1044–1047). It is intended for scripted/reproducible sessions where explicit values should not be overridden by ambient defaults. No frontend call ever sets this field. The omission means the web UI can never create a session in 'explicit values only' mode, even when a user has intentionally provided every field and does not want profile defaults applied.
- **fix hint**: Add `skipDefaults?: boolean` to the createSession request type in useSessionService.ts and thread it through the call body. Expose it as an advanced option (collapsed by default) in OmnibarCreationPanel, labelled something like 'Ignore profile & directory defaults'. Also thread through any programmatic creation paths (workflow/backlog) that need reproducible defaults.

### GAP-165 — WatchReviewQueueRequest.sessionIds filter never populated — no per-session review queue watching
- **status**: open
- **severity**: low
- **fingerprint**: GAP-WATCH-ReviewQueue-sessionIds-unused
- **file**: `web-app/src/lib/hooks/useReviewQueue.ts`
- **detail**: The proto WatchReviewQueueRequest has a `repeated string session_ids = 5` field that scopes the stream to review-queue events for specific sessions only. In useReviewQueue.ts lines 268–276, the WatchReviewQueueRequestSchema is created with only `priorityFilter`, `reasonFilter`, `initialSnapshot`, and `includeStatistics` — `sessionIds` is never set. The hook therefore always subscribes to the full global review queue regardless of context. In views that display a single session (e.g. SessionDetailView), a scoped subscription would reduce unnecessary event delivery and server fan-out.
- **fix hint**: Extend the `UseReviewQueueOptions` interface with `sessionIds?: string[]`. Pass it through to the `create(WatchReviewQueueRequestSchema, { ..., sessionIds: options.sessionIds ?? [] })` call. Use the scoped subscription in single-session detail views to reduce event volume. Ensure the fallback polling path (`getReviewQueue`) also receives a session filter if one is set.

### GAP-166 — "Config Files" nav item never highlights as active because its href includes a query param
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-ConfigFiles-query-param-never-active
- **file**: `web-app/src/lib/nav-pages.ts`
- **detail**: The "Config Files" entry in NAV_PAGES has href set to `routes.settings + "?tab=config-files"` (i.e. `/settings?tab=config-files`). All three nav components — DrawerNav, Header, and BottomNav — compute isActive via `pathname.startsWith(page.href)`, where `pathname` comes from Next.js `usePathname()` which returns only the path segment, never including the query string. So `/settings`.startsWith(`/settings?tab=config-files`) is always `false`, meaning the Config Files item is never highlighted even when the user is on that tab. As a side effect, when on `/settings?tab=config-files`, the "Settings" item (href `/settings`) incorrectly shows as active instead.
- **fix hint**: Change the active check in DrawerNav, Header, and BottomNav to compare `usePathname()` for the pathname portion and `useSearchParams()` for query params separately. Alternatively, give Config Files its own dedicated route (e.g. `/settings/config-files`) instead of a query-param variant, and redirect the old URL.

### GAP-167 — Both "Settings" and "Features" nav items highlight as active simultaneously when on /settings/features
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-Settings-Features-dual-active
- **file**: `web-app/src/components/layout/DrawerNav.tsx`
- **detail**: DrawerNav (and Header) compute isActive with `pathname.startsWith(page.href)`. The NAV_PAGES list contains both a "Settings" entry (href `/settings`) and a "Features" entry (href `/settings/features`). When the user navigates to `/settings/features`, both entries satisfy the startsWith check (`"/settings/features".startsWith("/settings")` is true), so both items render with the active style and `aria-current="page"` simultaneously. The same problem occurs in Header's hamburger menu. BottomNav avoids the issue because the Settings item has `mobileNav: false` and is excluded from mobile nav.
- **fix hint**: Update the isActive logic to require an exact match for parent routes when a more-specific child route entry is also present in NAV_PAGES. One approach: if `page.href` is a prefix of another nav page's href, use exact pathname equality instead of startsWith. Another approach: remove the `/settings` entry from NAV_PAGES and rely solely on the specific sub-routes.

### GAP-168 — ResolveApprovalRequest.message field never populated — denial reason is always absent
- **status**: open
- **severity**: low
- **fingerprint**: GAP-APPROVAL-deny-message-no-ui
- **file**: `web-app/src/components/sessions/ApprovalPanel.tsx`
- **detail**: The proto ResolveApprovalRequest has optional string message = 3 which Claude receives as the denial reason when a tool use is denied. approvalsApi.ts wires the message field through to the RPC. However every deny() call site omits the argument: ApprovalPanel.tsx line 94 calls deny(approval.id), ApprovalDrawer.tsx line 111 calls deny(approval.id), and useSessionNotifications.ts line 167 calls resolveApproval(approvalId, 'deny') with no message. Claude never receives a textual reason for why a tool use was denied.
- **fix hint**: Add a deny-reason textarea or inline input to the approval UI in ApprovalPanel and ApprovalDrawer. Thread the message string through the deny(approvalId, message) call in approvalsApi.ts and update useSessionNotifications to support an optional message if called from toast actions.

### GAP-169 — ResolveDefaultsResponse.matched_directory (field 9) silently dropped in useSessionDefaults hook
- **status**: open
- **severity**: low
- **fingerprint**: GAP-DEFAULTS-ResolveDefaults-matchedDirectory-dropped
- **file**: `web-app/src/lib/hooks/useSessionDefaults.ts`
- **detail**: ResolveDefaultsResponse includes matched_directory (proto field 9) which identifies which directory rule was matched when resolving session defaults for a given path. useSessionDefaults.ts lines 108-113 construct a ResolvedDefaults object from the response but never read response.matchedDirectory. The UI therefore cannot show users which directory rule is currently in effect, making it impossible to debug or audit why a particular set of defaults was applied.
- **fix hint**: Add matchedDirectory: string to the ResolvedDefaults interface, populate it with response.matchedDirectory in the hook, and expose it in the session creation UI as a source-tracking hint (e.g. 'Defaults from rule: /path/to/dir').

### GAP-170 — one_shot is persisted to the DB and present in CreateSessionRequest but absent from the Session read model
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-Session-one_shot-missing-from-read-model
- **file**: `proto/session/v1/types.proto`
- **detail**: `CreateSessionRequest` (session.proto field 16) accepts `bool one_shot`; `ent_repository.go` persists it (`SetOneShot` line 193) and reads it back (`OneShot: sess.OneShot` line 909); `InstanceData` carries the field. However, the Session response message in `types.proto` has no `one_shot` field, so clients listing or fetching sessions cannot distinguish one-shot sessions from normal directory sessions. The adapter in `instance_adapter.go` has no field to set even if it wanted to.
- **fix hint**: Add `bool one_shot = <next-field-number>` to the Session message in `proto/session/v1/types.proto`, run `make generate-proto`, then add `OneShot: inst.Data().OneShot` in `InstanceToProto`.

### GAP-171 — Settings program selectors use a static list and miss server-detected programs
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CONFIG-Settings-program-dropdowns-static-list
- **file**: `web-app/src/components/settings/GlobalDefaultsForm.tsx`
- **detail**: `GlobalDefaultsForm`, `ProfilesManager`, and `DirectoryRulesManager` all import the static `PROGRAMS` constant from `lib/constants/programs.ts` to populate their program dropdowns. In contrast, `OmnibarCreationPanel` uses the `useAvailablePrograms()` hook which fetches `/api/server-info` and merges server-detected binaries (from `Config.AvailablePrograms`) into the list. If a user has a non-standard program detected server-side (e.g. a custom path), it will appear in the Omnibar's program selector but not in the settings forms, making the default-program configuration incomplete for those programs.
- **fix hint**: Replace the `PROGRAMS` import in `GlobalDefaultsForm`, `ProfilesManager`, and `DirectoryRulesManager` with the `useAvailablePrograms()` hook, same as `OmnibarCreationPanel` already does.

### GAP-172 — `WatchInsights` server-streaming RPC has no `StreamingWSBridge` — uses HTTP long-polling in browsers
- **status**: open
- **severity**: low
- **fingerprint**: GAP-STREAM-WatchInsights-no-WSBridge
- **file**: `server/server.go`
- **detail**: The `StreamingWSBridge` is registered for `WatchSessions` and `WatchReviewQueue` so browsers use WebSocket instead of HTTP/1.1 long-polling, avoiding the browser 6-connection-per-origin limit. `WatchInsights` and `WatchUnfinishedWork` are both actively called from the frontend (`useInsightsService.ts` and `useUnfinishedWork.ts`) but have no WSBridge registration. When the Insights page is open alongside the main session view, each stream holds an HTTP connection and can starve other requests.
- **fix hint**: In `wireDepsIntoServer` in `server/server.go`, register a `StreamingWSBridge` for `InsightsServiceWatchInsightsProcedure` and `UnfinishedWorkServiceWatchUnfinishedWorkProcedure`, analogous to the existing Watch* RPC registrations.

### GAP-173 — `ListSessionsRequest.projectId` filter exists on the server but the frontend never sends it
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FILTER-ListSessions-projectId-silently-ignored
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: `ListSessionsRequest` has a `project_id` field (field 5) that the server uses to filter sessions by project. The frontend's `listSessions` method only passes `category` and `status`. When the session list is grouped by Project (via the grouping strategy in `web-app/src/lib/grouping/strategies.ts`), all sessions are fetched and filtered client-side rather than using the server-side project filter. For workspaces with many sessions this will degrade performance.
- **fix hint**: Update the `listSessions` wrapper in `useSessionService.ts` to accept an optional `projectId` parameter and pass it to the RPC. Then in `SessionList.tsx`, when a project grouping is active and a specific project is selected, pass the project ID to avoid over-fetching.

### GAP-174 — Two backend feature flags lack friendly labels in the Features settings page
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-FeatureMeta-missing-friendly-labels
- **file**: `web-app/src/app/settings/features/page.tsx`
- **detail**: The `FEATURE_META` map in `settings/features/page.tsx` only contains an entry for `backlog`. The backend registers three flags: `backlog`, `browser-passthrough`, and `backlog:conversation-view`. When `browser-passthrough` and `backlog:conversation-view` are returned by the API, they render in the Features UI with the raw flag name as label (e.g. `browser-passthrough`) because `meta?.label ?? name` falls through to `name`. The flags are functional toggles but present poorly to users.
- **fix hint**: Add entries to `FEATURE_META` in `settings/features/page.tsx`: `'browser-passthrough': { label: 'Browser Passthrough' }` and `'backlog:conversation-view': { label: 'Backlog Conversation View' }`.

### GAP-175 — `useSessionService.createSession` hook silently drops `oneShot`, `allowedTools`, and `projectId` fields
- **status**: open
- **severity**: low
- **fingerprint**: GAP-Hook-CreateSession-MissingFields
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: `CreateSessionRequest` has three fully-implemented server-side fields — `one_shot` (field 16), `allowed_tools` (field 21), and `project_id` (field 17) — that are not forwarded in the `useSessionService.createSession` hook. The hook is the canonical path used by `OmnibarContext` and `SessionServiceContext`. Callers that pass `oneShot`, `allowedTools`, or `projectId` through the hook will silently get the default (false / empty / empty) on the server.
- **fix hint**: In `useSessionService.ts` `createSession`, add the missing fields to the RPC call body: `oneShot: request.oneShot ?? false, allowedTools: request.allowedTools ?? "", projectId: request.projectId ?? ""` — mirroring the pattern already used for `oneOff` and `permissionMode`.

### GAP-176 — /backlog/board page exists and is linked from the Backlog list page but has no direct nav entry
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-BacklogBoard-no-nav-entry
- **file**: `web-app/src/app/backlog/board/page.tsx`
- **detail**: The /backlog/board route exists with a full kanban board component (BacklogBoard). It is reachable via a tab link hard-coded as href='/backlog/board' inside the Backlog list page, but routes.backlogBoard is defined in routes.ts and is never used in that link (the link uses a raw string). The board is also not registered in NAV_PAGES, so it cannot be deep-linked directly or bookmarked through the nav. The featureFlag: 'backlog' guard on the parent /backlog page does gate access, but the board sub-route itself uses a raw string href instead of the typed route constant.
- **fix hint**: In web-app/src/app/backlog/page.tsx, replace href='/backlog/board' with href={routes.backlogBoard} to use the typed route constant. Optionally add the board route to NAV_PAGES under the backlog feature flag.

### GAP-177 — WorkflowsPanel table omits model, agentType, and sessionType columns
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-WorkflowsPanel-model-agentType-not-displayed
- **file**: `web-app/src/components/workflows/WorkflowsPanel.tsx`
- **detail**: The workflow list table (WorkflowsPanel.tsx lines 213–219) only shows Slug, Name, Target Directory, Schedule, and Actions. The model, agentType, and sessionType fields are persisted in WorkflowProto and the ent schema but never rendered in the read-only table view. Users cannot audit which program or model a workflow uses without clicking Edit to open WorkflowForm. This creates a usability gap when reviewing multiple workflows.
- **fix hint**: Add a 'Program / Model' column to the table that shows wf.agentType || 'claude' and appends wf.model if set (e.g. 'claude (claude-sonnet-4-6)'). Optionally also add a 'Session Type' column showing wf.sessionType with a badge.

### GAP-178 — `ListSessionsRequest.search_query` and `hide_paused` fields are silently ignored
- **status**: open
- **severity**: low
- **fingerprint**: GAP-PROTO-ListSessions-search_query-hide_paused
- **file**: `server/services/session_service.go`
- **detail**: `ListSessionsRequest` defines `optional string search_query = 4` (for fuzzy matching across title, path, branch) and `bool hide_paused = 3` (hide paused sessions), but neither field is read in the `ListSessions` filter loop. Callers that set these fields receive unfiltered results, making the proto contract misleading. The frontend currently does client-side filtering instead of relying on these server-side fields, so there is no user-visible breakage today, but any server-side pagination or performance optimization would require these to actually work.
- **fix hint**: In the `ListSessions` filter loop, add: (1) a `hide_paused` check using the instance's status, and (2) a `search_query` fuzzy-match check against `inst.Title`, `inst.Path`, and the branch name. This mirrors what the frontend's session search does client-side.

### GAP-179 — `/backlog/board` linked via raw string instead of route constant; absent from nav
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-BacklogBoard-HardcodedHref
- **file**: `web-app/src/app/backlog/page.tsx`
- **detail**: `routes.backlogBoard` is defined in `routes.ts` (line 23) but the only link to `/backlog/board` in the entire frontend is a raw `href="/backlog/board"` string inside `web-app/src/app/backlog/page.tsx` (line 308). The board page (`web-app/src/app/backlog/board/page.tsx`) is not listed in `NAV_PAGES`, so it cannot be reached from mobile nav or the hamburger menu directly — only by navigating to the Backlog list page first.
- **fix hint**: Replace the raw string with `href={routes.backlogBoard}` in `backlog/page.tsx`. If the board view deserves top-level reachability, add a `featureFlag: 'backlog'`-guarded entry to `NAV_PAGES`.

### GAP-180 — `/debug/escape-codes` page exists with no route constant and no nav entry
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-DebugEscapeCodes-NoRouteNoNav
- **file**: `web-app/src/app/debug/escape-codes/page.tsx`
- **detail**: A fully implemented debug page that tracks terminal escape codes lives at `web-app/src/app/debug/escape-codes/page.tsx`. There is no corresponding key in `routes.ts` and no entry in `NAV_PAGES`. The page uses REST endpoints (`/debug/escape-codes`, `/debug/escape-codes/stats`, `/debug/escape-codes/toggle`) and is only reachable by typing the URL directly. Note that there is a separate `web-app/src/app/analytics/escape/page.tsx` (`routes.escapeAnalytics`) which is in nav; the debug page appears to be a parallel, lower-level view.
- **fix hint**: Add a `debugEscapeCodes: '/debug/escape-codes'` entry to `routes.ts` and decide whether to add it to `NAV_PAGES` (with `mobileNav: false, headerNav: false`) or remove it if `analytics/escape` supersedes it.

### GAP-181 — `listUnfinishedWork` one-shot RPC is unused; frontend only uses the streaming `watchUnfinishedWork`
- **status**: open
- **severity**: low
- **fingerprint**: GAP-RPC-UnfinishedWork-ListUnfinishedWork
- **file**: `web-app/src/lib/hooks/useUnfinishedWork.ts`
- **detail**: `UnfinishedWorkService.listUnfinishedWork` returns a snapshot of all unfinished worktrees without opening a stream. `useUnfinishedWork.ts` exclusively uses `watchUnfinishedWork` (streaming). There is no call-site for `listUnfinishedWork` in the frontend, which means there is no way to do a one-shot fetch (e.g., for server-side rendering or initial hydration without a streaming connection).
- **fix hint**: If an SSR or non-streaming use case is needed, add a `useListUnfinishedWork` hook that calls `client.listUnfinishedWork`. Otherwise document that `listUnfinishedWork` is intentionally server-internal (CLI or test only) and note this in the proto comments.

### GAP-182 — `/debug/escape-codes` page is reachable only via the hidden debug-menu wrench button
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-DebugEscapeCodes-hidden-nav
- **file**: `web-app/src/app/debug/escape-codes/page.tsx`
- **detail**: The page at `/debug/escape-codes` provides an active UI for toggling, viewing, and exporting terminal escape-code capture data. The only entry point is a link inside `DebugMenu` (opened by a small wrench icon in the header). There is no route constant in `routes.ts` and no entry in `NAV_PAGES`. Additionally, a separate but overlapping analytics page (`/analytics/escape`, `routes.escapeAnalytics`) IS in `NAV_PAGES`, creating two escape-analytics surfaces with different capabilities accessible through different paths.
- **fix hint**: Either add `debugEscapeCodes: "/debug/escape-codes"` to `routes.ts` and a corresponding entry in `NAV_PAGES` (under hamburger), or consolidate the debug-toggle functionality from `/debug/escape-codes` into the existing `/analytics/escape` page so there is a single discoverable surface.

### GAP-183 — `/test-terminal` page has no route constant and is discoverable only via direct URL or e2e tests
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-TestTerminal-no-route-constant
- **file**: `web-app/src/app/test-terminal/page.tsx`
- **detail**: The `/test-terminal` page is a developer/QA tool (terminal flicker stress test) that is used by the `terminal-flickering.spec.ts` e2e test via a hardcoded URL. It has no entry in `routes.ts`, no nav entry, and no link from anywhere in the app. The page explicitly exposes globals (`window.testTerminal`) for Playwright automation but is reachable in production builds at a non-obvious URL. Three similar test-harness pages (`/test/escape-codes`, `/test/layout-overlap`, `/test/terminal-stress`) share the same pattern.
- **fix hint**: If these pages are intended as permanent dev tools, add them to `routes.ts` under a `debug` or `test` namespace and link them from `DebugMenu`. If they are test-only fixtures, consider gating them behind a build-time flag so they do not ship as publicly accessible routes in production.

### GAP-184 — `ListSessionsRequest.hide_paused` and `search_query` filters accepted but silently ignored by the handler
- **status**: open
- **severity**: low
- **fingerprint**: GAP-FILTER-ListSessions-hide_paused-search_query-ignored
- **file**: `server/services/session_service.go`
- **detail**: The `ListSessionsRequest` proto message defines `hide_paused` (field 3) and `search_query` (field 4) filters with documentation comments. The `ListSessions` handler (around line 830) applies status, category, hidden, archived, and workflow_id filters — but never reads `req.Msg.HidePaused` or `req.Msg.SearchQuery`. The frontend performs session filtering client-side and does not send these params, so the server silently ignores them. If a client sends `hide_paused=true` or a `search_query`, it receives all sessions regardless.
- **fix hint**: Either apply the filters server-side (filter by `hide_paused`: skip `SESSION_STATUS_PAUSED` sessions; `search_query`: fuzzy-match title/path/branch) or remove these fields from the proto to avoid misuse.

### GAP-185 — `SUGGESTION_SOURCE_REVIEW_QUEUE_ITEM` / `analytics_item_id` silently ignored
- **status**: open
- **severity**: low
- **fingerprint**: GAP-HANDLER-GenerateSuggestedRule-ReviewQueueItem-ignored
- **file**: `server/services/rules_service.go`
- **detail**: The proto `GenerateSuggestedRuleRequest` defines `analytics_item_id = 4` specifically for `SUGGESTION_SOURCE_REVIEW_QUEUE_ITEM` (proto/session/v1/session.proto lines 2192-2203). The `GenerateRuleRequest` frontend interface in `useGenerateRule.ts` exposes `analyticsItemId`. However, `buildPromptContext` in `rules_service.go` never reads `req.AnalyticsItemId` — `RulePromptContext` has no field for it. When a caller passes `SUGGESTION_SOURCE_REVIEW_QUEUE_ITEM`, the handler falls through to the same analytics-gaps path as `ANALYTICS_GAPS` without scoping to the specific review item. No frontend component currently uses this source, confirming the feature is stub-incomplete.
- **fix hint**: Add a `ReviewQueueItemID string` field to `RulePromptContext`, populate it from `req.AnalyticsItemId` in `buildPromptContext`, and update `BuildUserPrompt` to include the specific analytics entry's command preview and context when this field is non-empty. Also add a case in `BuildUserPrompt` that scopes the prompt to the single item rather than the full analytics gap list.

### GAP-186 — `DeletePromptHistory` RPC has no frontend hook wrapper or UI call
- **status**: open
- **severity**: low
- **fingerprint**: GAP-RPC-DeletePromptHistory-no-frontend-call
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: The SessionService.DeletePromptHistory RPC is defined in the generated proto (session_pb.ts) and has backend tests. The listPromptHistory method is exposed via useSessionService and called from SessionWizard, but deletePromptHistory is never wrapped in any hook, never exposed in SessionServiceContext, and never called from any component. There is no UI affordance to delete individual prompt history entries.
- **fix hint**: Add a deletePromptHistory callback to useSessionService (mirroring listPromptHistory), expose it via SessionServiceContext if needed, and wire it to a delete button in the prompt history UI in SessionWizard or a dedicated prompt history management view.

### GAP-187 — `browser-passthrough` feature flag has no human-readable label in Features UI
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-FeaturesPage-FEATURE_META-browser-passthrough
- **file**: `web-app/src/app/settings/features/page.tsx`
- **detail**: The backend registers 'browser-passthrough' in knownFeatureFlags (server/services/session_service.go:3761) with a description, and it appears in the GetFeatureFlags RPC response. However, FEATURE_META in the Features page only contains an entry for 'backlog'. When 'browser-passthrough' is rendered in the list, the UI falls back to the raw flag name (meta?.label ?? name) instead of a friendly label like 'Browser Passthrough'. The description from the server is shown, but the label row will say 'browser-passthrough' instead of something human-readable.
- **fix hint**: Add an entry to FEATURE_META in web-app/src/app/settings/features/page.tsx: `'browser-passthrough': { label: 'Browser Passthrough' }`.

### GAP-188 — `DeletePromptHistory` RPC exists but frontend offers no way to delete prompt history entries
- **status**: open
- **severity**: low
- **fingerprint**: GAP-RPC-SessionService-DeletePromptHistory
- **file**: `web-app/src/lib/hooks/useSessionService.ts`
- **detail**: SessionService.DeletePromptHistory is defined in session.proto and generated into session_pb.ts. The frontend exposes listPromptHistory via useSessionService and shows recent prompts in SessionWizard.tsx, but there is no deletePromptHistory method in the hook or any component that calls this RPC. Users can view past prompts but cannot remove individual entries.
- **fix hint**: Add deletePromptHistory to useSessionService (calling clientRef.current.deletePromptHistory({ id })) and surface a delete/clear button in the prompt history dropdown in SessionWizard.

### GAP-189 — `routes.backlogBoard` constant is defined but never consumed — `/backlog/board` is linked with a hardcoded string
- **status**: open
- **severity**: low
- **fingerprint**: GAP-ROUTE-backlogBoard-constant-unused
- **file**: `web-app/src/lib/routes.ts`
- **detail**: The `routes.backlogBoard` constant (`/backlog/board`) is defined in `routes.ts` but is never imported or used anywhere — confirmed by grep returning no hits outside routes.ts. The backlog list page (`web-app/src/app/backlog/page.tsx` line 308) links to this page using the hardcoded string `/backlog/board` instead of `routes.backlogBoard`. The page has no nav entry in NAV_PAGES, so it is only reachable from the backlog list. If the path ever changes, the hardcoded reference won't benefit from the route constant.
- **fix hint**: In `web-app/src/app/backlog/page.tsx`, replace `href="/backlog/board"` with `href={routes.backlogBoard}` and add the routes import. Optionally add a nav entry in NAV_PAGES if the board view should be directly accessible.

### GAP-190 — `Session.launch_command` is never populated — `SessionDetailView` always shows empty
- **status**: open
- **severity**: low
- **fingerprint**: GAP-ADAPTER-Session-launch_command
- **file**: `server/adapters/instance_adapter.go`
- **detail**: The proto Session message has a `launch_command` field (field 45, types.proto:159–160) described as 'Full launch command as passed to tmux on session start'. The Go Instance struct sets `LaunchCommand` in `buildLaunchCommand` at two call sites (instance.go:1087, 1216). However, `InstanceToProto` never maps `inst.LaunchCommand` to `protoSession.LaunchCommand`. The `SessionDetailView` component (web-app/src/components/sessions/SessionDetailView.tsx:994–1024) conditionally renders a 'Launch Command' section using `session.launchCommand`, but this field is always an empty string so the section never renders.
- **fix hint**: In `InstanceToProto` (server/adapters/instance_adapter.go), add `protoSession.LaunchCommand = inst.LaunchCommand` to the struct literal or as a separate assignment. The field is already declared in the proto and generated TypeScript types.

### GAP-191 — `?title=` query param built by `routes.newSessionFromWorktree` is silently dropped on the home page
- **status**: open
- **severity**: low
- **fingerprint**: GAP-NAV-newSessionFromWorktree-titleParamIgnored
- **file**: `web-app/src/lib/routes.ts`
- **detail**: The `routes.newSessionFromWorktree` helper accepts an optional `title` parameter and includes it as `?title=...` in the generated URL. The function is called from `UnfinishedItemDetail.tsx` with `branch` as the title argument. However, the home page's `useEffect` for URL param detection only reads `?new`, `?duplicate`, `?worktree`, and `?branch` — it never calls `searchParams.get('title')`. The title value is passed but always silently ignored.
- **fix hint**: Either read `searchParams.get('title')` in the home page's URL detection effect and pass it to `openOmnibar` (so it can pre-fill the session title), or remove the `title` parameter from `routes.newSessionFromWorktree` if it is not intended to be used.

### GAP-192 — `ResolveDefaults` `fieldSources` for `autoYes`, `tags`, and `cliFlags` are computed but never surfaced with `SourceBadge` in `SessionWizard`
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-SessionWizard-fieldSources-badges
- **file**: `web-app/src/components/sessions/SessionWizard.tsx`
- **detail**: `useSessionDefaults` returns `fieldSources` with four keys: `program`, `autoYes`, `tags`, and `cliFlags`. `SessionWizard.tsx` attaches a `SourceBadge` only to the `program` field (line 540). The `autoYes`, `tags`, and `cliFlags` fields in the form have no badge, so users cannot see whether those values were injected from a profile, directory rule, or global defaults. The `matchedDirectory` value returned from `ResolveDefaultsResponse` is also discarded by `useSessionDefaults` and never exposed to any UI.
- **fix hint**: Add `<SourceBadge source={fieldSources.autoYes} />` and similar badges next to the auto-yes checkbox, tags input, and CLI flags input in `SessionWizard.tsx`. Optionally expose `matchedDirectory` from the hook to show 'matched rule at /path/to/dir' in the UI.

### GAP-193 — `/test/layout-overlap` page exists with no inbound links and no e2e test coverage
- **status**: open
- **severity**: low
- **fingerprint**: GAP-UI-TestLayoutOverlap-no-link
- **file**: `web-app/src/app/test/layout-overlap/page.tsx`
- **detail**: The page comment says it is for Playwright tests measuring element positions without a live server, but no e2e spec in `tests/e2e/` navigates to `/test/layout-overlap`. It is not linked from any component and has no nav entry. Unlike `/test/terminal-stress` (used by `tests/e2e/terminal-stress/helpers.ts`) and `/test-terminal` (used by `terminal-flickering.spec.ts`), this page has zero callers.
- **fix hint**: Either write a Playwright spec that uses it (layout/overlap regression test), or delete it if the scenario it was built for has been covered elsewhere.

### GAP-194 — `SessionGoal` is loaded during `Storage.loadLiveSessions` but not during ent repository Get/List queries
- **status**: open
- **severity**: low
- **fingerprint**: GAP-ENTITY-SessionGoal-storage-only-MCP-not-list-sessions
- **file**: `session/ent_repository.go`
- **detail**: The SessionGoal entity is read in session/storage.go via a batch query across all live sessions (line 303) and cached on each Instance. However, the ent_repository Get, List, ListByStatus, and ListByTag query methods (lines 660–757) do NOT use WithSessionGoal() — they only load ClaudeSession, Worktree, DiffStats, Tags, and Project edges. Sessions fetched directly from the repository (e.g. by headless or test code) will always have a nil SessionGoal, even if one was previously set. The proto converter in instance_adapter.go reads the cached value so REST clients get it only after Storage.loadLiveSessions has run.
- **fix hint**: Either add a separate SessionGoal.Query().Where(sessiongoal.SessionUUID(sess.UUID)) call in sessionToInstanceData, or load goals in batch alongside the existing WithClaudeSession eager loads in the Get/List queries.

### GAP-195 — `Config.Notifications.PushEnabled` flag is stored but never checked by the push service
- **status**: open
- **severity**: low
- **fingerprint**: GAP-CONFIG-NotificationPrefs-PushEnabled-ignored
- **file**: `config/config.go`
- **detail**: NotificationPrefs.PushEnabled is defined in config.go and serialised to config.json, implying it should gate whether web push notifications are sent. However, push_service.go and push_handler.go never read Config.Notifications.PushEnabled — push notifications are delivered to all subscribers regardless of this flag. There is also no proto field or UI control that reads or writes this flag.
- **fix hint**: In the push delivery path (push_service.go or push_handler.go), call config.LoadConfig() and skip sending if !cfg.Notifications.PushEnabled. Optionally expose the setting via proto + UI, or remove the field if opt-out is intended to be handled exclusively by browser unsubscription.
