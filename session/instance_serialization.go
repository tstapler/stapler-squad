package session

// instance_serialization.go contains serialization/deserialization functions
// for converting between Instance and its on-disk representation (InstanceData).
// InstanceData, GitWorktreeData, and DiffStatsData are defined in storage.go.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/git"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// worktreeMissingLevel picks the level for the "worktree directory missing"
// log line: Warn the first time this Instance has seen it, Debug on any
// repeat (see Instance.loggedMissingWorktree). Returning a level rather than
// a bound *slog.Logger method keeps this directly unit-testable — method
// values aren't comparable.
func worktreeMissingLevel(alreadyLogged bool) slog.Level {
	if alreadyLogged {
		return slog.LevelDebug
	}
	return slog.LevelWarn
}

// ToInstanceData converts an Instance to its serializable form
//
// Builds a fresh snapshot via sendSyncErr rather than reading the cached
// Snapshot(): callers like Storage.UpdateInstance/SaveInstancesSync routinely
// mutate exported fields (Tags, Category, ...) directly and expect the very
// next ToInstanceData() call to reflect that — Snapshot()'s cache only
// refreshes when an actor command republishes it, so those direct-mutation
// callers would see stale data (caught by TestStorage_UpdateInstance /
// TestStorage_SaveInstancesSync). Routing the build through sendSyncErr gets
// a fresh buildSnapshot(s.inst) (current field values, actor-mutation or not)
// while still serializing against concurrent actor commands: with a live
// actor this blocks until the mailbox delivers it, matching every other
// actor command's ordering; with no live actor (tests, pre-NewLiveInstance)
// it runs synchronously on the calling goroutine, same as before.
// Do not call from within a sendSyncErr/send/sendCtx closure — see actor.go.
// LaunchCommand is not in the snapshot (set once during Start) and is read directly.
// gitManager and claudeSession sub-objects have their own synchronisation.
func (i *Instance) ToInstanceData() InstanceData {
	var snap *InstanceSnapshot
	_ = i.sendSyncErr(func(s *instanceState) error {
		// i.mu guards buildSnapshot here too: legacy setters (MarkViewed & co.)
		// mutate fields directly under i.mu.Lock() from outside the actor — see
		// runActor's doc comment in actor.go.
		s.inst.mu.Lock()
		snap = buildSnapshot(s.inst)
		s.inst.mu.Unlock()
		return nil
	})

	data := InstanceData{
		Title:                snap.Title,
		UUID:                 snap.UUID,
		Path:                 snap.Path,
		WorkingDir:           snap.WorkingDir,
		Branch:               snap.Branch,
		Status:               snap.Status,
		Height:               snap.Height,
		Width:                snap.Width,
		CreatedAt:            snap.CreatedAt,
		UpdatedAt:            time.Now(),
		Program:              snap.Program,
		AutoYes:              snap.AutoYes,
		AutoApprove:          snap.AutoApprove,
		Prompt:               snap.Prompt,
		InitialPrompt:        snap.InitialPrompt,
		Category:             snap.Category,
		Note:                 snap.Note,
		IsExpanded:           snap.IsExpanded,
		Tags:                 snap.Tags, // Include tags in serialization
		SessionType:          snap.SessionType,
		TmuxPrefix:           snap.TmuxPrefix,
		TmuxServerSocket:     snap.TmuxServerSocket,
		LastTerminalUpdate:   snap.LastTerminalUpdate,
		LastMeaningfulOutput: snap.LastMeaningfulOutput,
		LastOutputSignature:  snap.LastOutputSignature,
		LastAddedToQueue:     snap.LastAddedToQueue,
		LastViewed:           snap.LastViewed,
		LastAcknowledged:     snap.LastAcknowledged,
		// Prompt detection and interaction tracking
		LastPromptDetected:   snap.LastPromptDetected,
		LastPromptSignature:  snap.LastPromptSignature,
		LastUserResponse:     snap.LastUserResponse,
		ProcessingGraceUntil: snap.ProcessingGraceUntil,
		// GitHub integration fields
		GitHubPRNumber:  snap.GitHub.GitHubPRNumber,
		GitHubPRURL:     snap.GitHub.GitHubPRURL,
		GitHubOwner:     snap.GitHub.GitHubOwner,
		GitHubRepo:      snap.GitHub.GitHubRepo,
		GitHubSourceRef: snap.GitHub.GitHubSourceRef,
		ClonedRepoPath:  snap.GitHub.ClonedRepoPath,
		// GitHub integration fields
		GitHubIsFork: snap.GitHub.GitHubIsFork,
		// PR status fields (populated by PRStatusPoller)
		GitHubPRState:          snap.GitHub.GitHubPRState,
		GitHubPRIsDraft:        snap.GitHub.GitHubPRIsDraft,
		GitHubPRPriority:       snap.GitHub.GitHubPRPriority,
		GitHubApprovedCount:    snap.GitHub.GitHubApprovedCount,
		GitHubChangesReqCount:  snap.GitHub.GitHubChangesReqCount,
		GitHubCheckConclusion:  snap.GitHub.GitHubCheckConclusion,
		GitHubPRStatusTerminal: snap.GitHub.GitHubPRStatusTerminal,
		LastPRStatusCheck:      snap.GitHub.LastPRStatusCheck,
		// Crew autonomy mode
		AutonomousMode: snap.Autonomous.AutonomousMode,
		// Checkpoint metadata
		Checkpoints:            snap.Checkpoints,
		ActiveCheckpoint:       snap.ActiveCheckpoint,
		ForkedFromID:           snap.ForkedFromID,
		RestartedFromSessionID: snap.RestartedFromSessionID,
		// History file linkage
		HistoryFilePath:            snap.HistoryFilePath,
		EverHadConversationHistory: snap.EverHadConversationHistory,
		LastReviveOutcome:          string(snap.LastReviveOutcome),
		// One-shot mode
		OneShot: snap.OneShot,
		// Hidden (system/background) flag
		Hidden: snap.Hidden,
		// Project association
		ProjectID: snap.ProjectID,
		// Full launch command for diagnostics (not in snapshot — set once during Start)
		LaunchCommand: i.LaunchCommand,
		// MCP server URL for re-injection on restart
		MCPServerURL: snap.MCPServerURL,
		// Pause reason — persisted so it survives restarts
		PauseReason: snap.PauseReason,
		// Exit reason — persisted so a Crashed banner survives restarts
		ExitReason: snap.ExitReason,
		// Workflow linkage and archive state
		WorkflowID: snap.WorkflowID,
		ArchivedAt: snap.ArchivedAt,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitManager.HasWorktree() {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitManager.GetRepoPath(),
			WorktreePath:  i.gitManager.GetWorktreePath(),
			SessionName:   snap.Title,
			BranchName:    i.gitManager.GetBranchName(),
			BaseCommitSHA: i.gitManager.GetBaseCommitSHA(),
		}
	}

	// Only include diff stats if they exist
	if i.gitManager.diffStats != nil {
		data.DiffStats = DiffStatsData{
			Added:   i.gitManager.diffStats.Added,
			Removed: i.gitManager.diffStats.Removed,
			Content: i.gitManager.diffStats.Content,
		}
	}

	// Include Claude session data if it exists
	if i.claudeSession != nil {
		data.ClaudeSession = *i.claudeSession
	}
	// Always wire the squad session ID from Instance.UUID so the API response
	// always carries both identifiers in the ClaudeSession sub-object.
	data.ClaudeSession.SquadSessionID = snap.UUID

	return data
}

// FromInstanceData creates a new Instance from serialized data
// FromInstanceData reconstructs an *Instance from persisted data, starting it
// synchronously (hot-attaching to an already-live tmux session or cold-restoring
// one) before returning. Use for on-demand single-instance loads (e.g.
// Registry.Acquire) where the caller needs a ready instance immediately.
//
// Bulk startup loads should use fromInstanceData(data, true) via LoadInstances
// instead — starting every instance synchronously here is what made server
// startup block on restoring all sessions (including cold-relaunching every
// dead one) before the HTTP server could bind. See server/dependencies.go's
// "Step 6" background goroutine, which already exists to start un-started
// instances asynchronously once the deferred path skips Start() here.
func FromInstanceData(data InstanceData) (*Instance, error) {
	return fromInstanceData(data, false)
}

// fromInstanceData is the shared implementation. When deferStart is true, the
// Active-branch (and Stopped-but-tmux-alive recovery) code paths still wire the
// tmux session object (so HasSession()/TmuxAlive() report correctly) but skip
// the synchronous Start() call, leaving Instance.Started() false so the async
// Step 6 loop in BuildRuntimeDeps picks it up off the startup critical path.
func fromInstanceData(data InstanceData, deferStart bool) (*Instance, error) {
	// MIGRATION: Fix corrupted paths from before defensive tilde expansion was added
	// Detect paths like "/absolute/path/~/other/path" and fix them
	migratedPath := data.Path
	if strings.Contains(data.Path, "/~/") {
		// Path contains unexpanded tilde - extract and expand it
		log.Warn("migrating corrupted path for instance", "session", data.Title, "path", data.Path)

		// Find the index of "/~/"
		idx := strings.Index(data.Path, "/~/")
		if idx >= 0 {
			// Extract the tilde path (everything from "~/" onwards)
			tildePath := data.Path[idx+1:] // Skip the leading "/"

			// Expand the tilde path
			if strings.HasPrefix(tildePath, "~/") {
				usr, err := user.Current()
				if err != nil {
					log.Error("failed to expand corrupted path", "session", data.Title, "err", err)
					// Fall back to original path
				} else {
					migratedPath = filepath.Join(usr.HomeDir, tildePath[2:])
					log.Info("migrated corrupted path", "session", data.Title, "from", data.Path, "to", migratedPath)
				}
			}
		}
	}

	// MIGRATION: Convert legacy Category to Tags for backward compatibility
	// If Tags is empty but Category exists, migrate category to tags
	tags := data.Tags
	if len(tags) == 0 && data.Category != "" {
		// Migrate existing category to tag format
		// Support both simple ("Work") and nested ("Work/Frontend") categories
		tags = []string{data.Category}
		log.Info("migrating category to tags", "category", data.Category, "session", data.Title)
	}

	instance := &Instance{
		Title:            data.Title,
		UUID:             data.UUID,
		Path:             migratedPath, // Use migrated path
		WorkingDir:       data.WorkingDir,
		Branch:           data.Branch,
		Status:           data.Status,
		Height:           data.Height,
		Width:            data.Width,
		CreatedAt:        data.CreatedAt,
		UpdatedAt:        data.UpdatedAt,
		Program:          data.Program,
		AutoYes:          data.AutoYes, // pre-existing bug: was never restored on load, losing auto_yes across every restart
		AutoApprove:      data.AutoApprove,
		Prompt:           data.Prompt,
		InitialPrompt:    data.InitialPrompt,
		Category:         data.Category,
		Note:             data.Note,
		IsExpanded:       data.IsExpanded,
		Tags:             tags, // Use migrated tags (includes category if needed)
		SessionType:      data.SessionType,
		TmuxPrefix:       data.TmuxPrefix,
		TmuxServerSocket: data.TmuxServerSocket,
		ReviewState: ReviewState{
			LastTerminalUpdate:   data.LastTerminalUpdate,
			LastMeaningfulOutput: data.LastMeaningfulOutput,
			LastOutputSignature:  data.LastOutputSignature,
			LastAddedToQueue:     data.LastAddedToQueue,
			LastViewed:           data.LastViewed,
			LastAcknowledged:     data.LastAcknowledged,
			LastPromptDetected:   data.LastPromptDetected,
			LastPromptSignature:  data.LastPromptSignature,
			LastUserResponse:     data.LastUserResponse,
			ProcessingGraceUntil: data.ProcessingGraceUntil,
		},
		InstanceType:     InstanceTypeManaged, // Restored instances are always managed
		IsManaged:        true,
		ExternalMetadata: nil,                     // External instances are not persisted
		Permissions:      GetManagedPermissions(), // Full permissions for managed instances
		// GitHub integration fields
		GitHubPRNumber:  data.GitHubPRNumber,
		GitHubPRURL:     data.GitHubPRURL,
		GitHubOwner:     data.GitHubOwner,
		GitHubRepo:      data.GitHubRepo,
		GitHubSourceRef: data.GitHubSourceRef,
		ClonedRepoPath:  data.ClonedRepoPath,
		GitHubIsFork:    data.GitHubIsFork,
		// PR status fields (populated by PRStatusPoller)
		GitHubPRState:          data.GitHubPRState,
		GitHubPRIsDraft:        data.GitHubPRIsDraft,
		GitHubPRPriority:       data.GitHubPRPriority,
		GitHubApprovedCount:    data.GitHubApprovedCount,
		GitHubChangesReqCount:  data.GitHubChangesReqCount,
		GitHubCheckConclusion:  data.GitHubCheckConclusion,
		GitHubPRStatusTerminal: data.GitHubPRStatusTerminal,
		LastPRStatusCheck:      data.LastPRStatusCheck,
		// Worktree detection fields
		MainRepoPath: data.MainRepoPath,
		IsWorktree:   data.IsWorktree,
		// Crew autonomy mode
		AutonomousMode: data.AutonomousMode,
		// Checkpoint metadata
		Checkpoints:            data.Checkpoints,
		ActiveCheckpoint:       data.ActiveCheckpoint,
		ForkedFromID:           data.ForkedFromID,
		RestartedFromSessionID: data.RestartedFromSessionID,
		// History file linkage
		HistoryFilePath:            data.HistoryFilePath,
		EverHadConversationHistory: data.EverHadConversationHistory,
		LastReviveOutcome:          ReviveOutcome(data.LastReviveOutcome),
		// One-shot mode
		OneShot: data.OneShot,
		// Hidden (system/background) flag
		Hidden: data.Hidden,
		// Project association
		ProjectID: data.ProjectID,
		// Launch command for diagnostics
		LaunchCommand: data.LaunchCommand,
		// MCP server URL for re-injection on restart
		MCPServerURL: data.MCPServerURL,
		// Pause reason
		PauseReason: data.PauseReason,
		// Exit reason
		ExitReason: data.ExitReason,
		// Workflow linkage and archive state
		WorkflowID: data.WorkflowID,
		ArchivedAt: data.ArchivedAt,
	}

	// MIGRATION: Assign UUID to existing sessions that pre-date UUID assignment
	if instance.UUID == "" {
		instance.UUID = uuid.New().String()
	}

	// Initialize TagManager backed by the Instance.Tags slice
	instance.tagManager = NewTagManager(&instance.Tags)

	// Shell registry is not part of InstanceData and defaults to nil; initialize
	// it so restored instances can spawn/track shells (see ShellRegistry's
	// nil-receiver-safe methods, which otherwise silently no-op).
	instance.initShellRegistry()

	// Sync atomic shadow fields so lock-free readers see the correct initial values.
	instance.SyncAtomicTimestamps()

	// Initialize the process manager via the factory so selectedBackend is honored.
	// The underlying session is wired below (for Paused/Stopped/Hibernated) or
	// by initTmuxSession() when Start() is called (for Active sessions).
	// instance.Backend is not currently persisted in InstanceData (out of Epic 2.1's
	// scope — see plan.md Task 2.1.3c), so restored sessions fall back to the
	// process-wide default here, same as before this field existed.
	pm, err := NewProcessManager(context.Background(), BackendTmux, ProcessManagerOptions{Backend: instance.Backend})
	if err != nil {
		return nil, fmt.Errorf("session: construct process manager for restored instance %q: %w", instance.Title, err)
	}
	instance.processManager = pm

	// Restore git worktree and diff stats via manager (cannot use struct literal for sub-manager fields).
	instance.gitManager.SetWorktree(git.NewGitWorktreeFromStorage(
		data.Worktree.RepoPath,
		data.Worktree.WorktreePath,
		data.Worktree.SessionName,
		data.Worktree.BranchName,
		data.Worktree.BaseCommitSHA,
	))
	instance.gitManager.SetDiffStats(&git.DiffStats{
		Added:   data.DiffStats.Added,
		Removed: data.DiffStats.Removed,
		Content: data.DiffStats.Content,
	})

	// Restore Claude session data if it exists
	if data.ClaudeSession.ConversationUUID != "" {
		claudeSessionCopy := data.ClaudeSession
		instance.claudeSession = &claudeSessionCopy
	}

	// Auto-detect worktree info for migration (existing sessions without this info)
	// This populates IsWorktree, MainRepoPath, GitHubOwner, and GitHubRepo
	if instance.GitHubOwner == "" || instance.GitHubRepo == "" {
		if err := instance.DetectAndPopulateWorktreeInfo(); err != nil {
			log.Warn("failed to detect worktree info", "session", instance.Title, "err", err)
			// Non-fatal - session can still work without this info
		} else if instance.GitHubOwner != "" {
			log.Info("auto-detected github info", "session", instance.Title, "owner", instance.GitHubOwner, "repo", instance.GitHubRepo, "worktree", instance.IsWorktree)
		}
	}

	// Initialize session-specific logging
	_ = log.GetSessionLoggers

	// Check if the worktree still exists on disk if the instance is not paused or hibernated.
	// No mutex is needed here because the instance is not yet shared.
	if !instance.Paused() && !instance.Hibernated() && instance.gitManager.HasWorktree() {
		worktreePath := instance.gitManager.GetWorktreePath()
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree has been deleted — use transitionTo so the state machine is respected.
			level := worktreeMissingLevel(instance.loggedMissingWorktree)
			instance.loggedMissingWorktree = true
			logger := log.ForSession(instance.Title)
			warnOrDebug := func(msg string, args ...any) { logger.Log(context.Background(), level, msg, args...) }
			warnOrDebug("worktree directory missing, marking as paused", "path", worktreePath)
			if err := instance.transitionTo(context.Background(), Paused); err != nil {
				// If the transition is somehow invalid (e.g. already Stopped), fall back to loadStatus.
				warnOrDebug("could not transition to paused via state machine, using loadStatus", "err", err)
				instance.loadStatus(Paused)
			}
		}
	}

	if instance.Paused() {
		instance.started.Store(true)
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}

		// Use server socket isolation if specified, otherwise use prefix-only isolation.
		// WithRegistry(nil) prevents a background reconnect loop on isolated sockets —
		// the loop tries attach-session on a keepalive that doesn't exist there, causing
		// intermittent exit status 1 from concurrent new-session calls.
		if tb, ok := instance.processManager.(*TmuxBackend); ok {
			if instance.TmuxServerSocket != "" {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket, tmux.WithRegistry(nil)))
			} else {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix))
			}
		}
	} else if instance.Status == Stopped {
		// Wire the tmux session object so IsAlive() can be called.
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}
		if tb, ok := instance.processManager.(*TmuxBackend); ok {
			if instance.TmuxServerSocket != "" {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket, tmux.WithRegistry(nil)))
			} else {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix))
			}
		}
		// If the underlying tmux session is still alive (e.g. server crashed mid-write
		// or exit callback fired falsely), recover it rather than leave it stuck as
		// Stopped. IsAlive() alone can't see remain-on-exit: it keeps the tmux
		// session/pane around as a dead placeholder after the wrapped program exits
		// (same distinction PaneProcessDead() draws for the health checker, see
		// instance_tmux.go), so without the pane-exit check below, every session the
		// health checker had just legitimately marked Stopped would be revived right
		// back to Active on the very next LoadInstances() -- SessionHealthChecker's
		// tick calls LoadInstances() every poll, so this raced the exact fix in
		// session/health.go that makes freshly-created sessions reach Stopped promptly.
		//
		// Skip this probe entirely for archived sessions: ArchivedAt is set only by
		// archiveItemWorkSessions, which explicitly kills the tmux pane at archive
		// time (see review_queue_poller.go's shouldSkipSession doc comment) -- there
		// is no scenario where an archived session's pane is secretly still alive.
		// Without this skip, every LoadInstances() call (the 15s health-check tick,
		// most MCP tools, many RPC handlers) paid two uncached tmux subprocess spawns
		// (PaneExitStatus + IsAlive) per archived session -- on one real deployment
		// this was 280 archived-but-Stopped sessions out of 282 total Stopped, i.e.
		// ~560 needless subprocess spawns on every single LoadInstances() call,
		// which pprof's fork-pressure monitor flagged as a sustained "critical"
		// spawn/failure rate (subprocess failures/spawns >> the exec-gate's timeout
		// budget once a couple thousand archived sessions accumulate).
		if instance.ArchivedAt != nil {
			instance.started.Store(true)
		} else {
			paneExited := false
			if tb, ok := instance.processManager.(*TmuxBackend); ok {
				if tm := tb.TmuxManager(); tm != nil {
					_, _, paneExited = tm.PaneExitStatus()
				}
			}
			if instance.processManager.IsAlive() && !paneExited {
				log.Warn("session stored as stopped but tmux is alive, recovering to active", "session", instance.Title)
				instance.loadStatus(Active)
				if deferStart {
					// Leave started=false: the async Step 6 loop will call Start(false)
					// and hot-attach to this already-live session off the critical path.
				} else if err := instance.Start(false); err != nil {
					log.Warn("recovery start failed, keeping stopped", "session", instance.Title, "err", err)
					instance.loadStatus(Stopped)
					instance.started.Store(true)
				}
			} else {
				instance.started.Store(true)
			}
		}
	} else if instance.Status == Hibernated {
		// Wire the tmux session object (for IsAlive checks at resume time)
		// but do NOT call Start — hibernated sessions resume only on explicit request.
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}
		if tb, ok := instance.processManager.(*TmuxBackend); ok {
			if instance.TmuxServerSocket != "" {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(
					instance.Title, instance.Program, tmuxPrefix,
					instance.TmuxServerSocket, tmux.WithRegistry(nil)))
			} else {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(
					instance.Title, instance.Program, tmuxPrefix))
			}
		}
		instance.started.Store(true)
	} else if instance.Status == Crashed {
		// Wire the tmux session object (for IsAlive checks) but do NOT call Start --
		// like Hibernated, a Crashed session resumes only on explicit request
		// (Instance.ResumeFromCrash / the ResumeCrashedSession RPC). Without this
		// branch, Crashed falls into the generic (Active) else-branch below with
		// started=false, and server/dependencies.go's Step 6 startup loop
		// unconditionally calls Start(false) on every !Started() instance --
		// silently auto-resuming every Crashed session on the very next server
		// restart, exactly what the new Crashed status is designed to prevent
		// (see session/health.go's "must not be silently respawned" comment).
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}
		if tb, ok := instance.processManager.(*TmuxBackend); ok {
			if instance.TmuxServerSocket != "" {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(
					instance.Title, instance.Program, tmuxPrefix,
					instance.TmuxServerSocket, tmux.WithRegistry(nil)))
			} else {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(
					instance.Title, instance.Program, tmuxPrefix))
			}
		}
		instance.started.Store(true)
	} else {
		// Wire the tmux session object first (mirrors the Paused/Stopped/Hibernated
		// branches above) so initTmuxSession()'s HasSession() check correctly detects
		// an already-running tmux session instead of unconditionally treating every
		// restore as a fresh launch. Without this, HasSession() is false on this
		// freshly-constructed Instance regardless of whether the real tmux session
		// is alive, so every LoadInstances() call (health checks, MCP tool handlers,
		// etc.) logs a spurious "creating tmux session" and re-runs launch bookkeeping
		// for every Active session, even ones that were never actually down.
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}
		if tb, ok := instance.processManager.(*TmuxBackend); ok {
			if instance.TmuxServerSocket != "" {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket, tmux.WithRegistry(nil)))
			} else {
				tb.TmuxManager().SetSession(tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix))
			}
		}
		if deferStart {
			// Leave started=false: the async Step 6 loop in BuildRuntimeDeps calls
			// Start(false) later, off the startup critical path. That loop already
			// hot-attaches to a live tmux session or cold-restores a dead one —
			// the same logic this branch would otherwise run synchronously here.
		} else if err := instance.Start(false); err != nil {
			return nil, err
		}
	}

	finishInstanceConstruction(instance)
	return instance, nil
}
