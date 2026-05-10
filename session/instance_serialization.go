package session

// instance_serialization.go contains serialization/deserialization functions
// for converting between Instance and its on-disk representation (InstanceData).
// InstanceData, GitWorktreeData, and DiffStatsData are defined in storage.go.

import (
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

// ToInstanceData converts an Instance to its serializable form
func (i *Instance) ToInstanceData() InstanceData {
	data := InstanceData{
		Title:                i.Title,
		UUID:                 i.UUID,
		Path:                 i.Path,
		WorkingDir:           i.WorkingDir,
		Branch:               i.Branch,
		Status:               i.Status,
		Height:               i.Height,
		Width:                i.Width,
		CreatedAt:            i.CreatedAt,
		UpdatedAt:            time.Now(),
		Program:              i.Program,
		AutoYes:              i.AutoYes,
		Prompt:               i.Prompt,
		Category:             i.Category,
		IsExpanded:           i.IsExpanded,
		Tags:                 i.Tags, // Include tags in serialization
		SessionType:          i.SessionType,
		TmuxPrefix:           i.TmuxPrefix,
		LastTerminalUpdate:   i.LastTerminalUpdate,
		LastMeaningfulOutput: i.LastMeaningfulOutput,
		LastOutputSignature:  i.LastOutputSignature,
		LastAddedToQueue:     i.LastAddedToQueue,
		LastViewed:           i.LastViewed,
		LastAcknowledged:     i.LastAcknowledged,
		// Prompt detection and interaction tracking
		LastPromptDetected:   i.LastPromptDetected,
		LastPromptSignature:  i.LastPromptSignature,
		LastUserResponse:     i.LastUserResponse,
		ProcessingGraceUntil: i.ProcessingGraceUntil,
		// GitHub integration fields
		GitHubPRNumber:  i.GitHubPRNumber,
		GitHubPRURL:     i.GitHubPRURL,
		GitHubOwner:     i.GitHubOwner,
		GitHubRepo:      i.GitHubRepo,
		GitHubSourceRef: i.GitHubSourceRef,
		ClonedRepoPath:  i.ClonedRepoPath,
		// GitHub integration fields
		GitHubIsFork: i.GitHubIsFork,
		// PR status fields (populated by PRStatusPoller)
		GitHubPRState:          i.GitHubPRState,
		GitHubPRIsDraft:        i.GitHubPRIsDraft,
		GitHubPRPriority:       i.GitHubPRPriority,
		GitHubApprovedCount:    i.GitHubApprovedCount,
		GitHubChangesReqCount:  i.GitHubChangesReqCount,
		GitHubCheckConclusion:  i.GitHubCheckConclusion,
		GitHubPRStatusTerminal: i.GitHubPRStatusTerminal,
		LastPRStatusCheck:      i.LastPRStatusCheck,
		// Crew autonomy mode
		AutonomousMode: i.AutonomousMode,
		// Checkpoint metadata
		Checkpoints:      i.Checkpoints,
		ActiveCheckpoint: i.ActiveCheckpoint,
		ForkedFromID:     i.ForkedFromID,
		// History file linkage
		HistoryFilePath: i.HistoryFilePath,
		// One-shot mode
		OneShot: i.OneShot,
		// Project association
		ProjectID: i.ProjectID,
		// Full launch command for diagnostics
		LaunchCommand: i.LaunchCommand,
		// MCP server URL for re-injection on restart
		MCPServerURL: i.MCPServerURL,
	}

	// Only include worktree data if gitWorktree is initialized
	if i.gitManager.HasWorktree() {
		data.Worktree = GitWorktreeData{
			RepoPath:      i.gitManager.GetRepoPath(),
			WorktreePath:  i.gitManager.GetWorktreePath(),
			SessionName:   i.Title,
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
	data.ClaudeSession.SquadSessionID = i.UUID

	return data
}

// FromInstanceData creates a new Instance from serialized data
func FromInstanceData(data InstanceData) (*Instance, error) {
	// MIGRATION: Fix corrupted paths from before defensive tilde expansion was added
	// Detect paths like "/absolute/path/~/other/path" and fix them
	migratedPath := data.Path
	if strings.Contains(data.Path, "/~/") {
		// Path contains unexpanded tilde - extract and expand it
		log.WarningLog.Printf("Migrating corrupted path for instance '%s': %s", data.Title, data.Path)

		// Find the index of "/~/"
		idx := strings.Index(data.Path, "/~/")
		if idx >= 0 {
			// Extract the tilde path (everything from "~/" onwards)
			tildePath := data.Path[idx+1:] // Skip the leading "/"

			// Expand the tilde path
			if strings.HasPrefix(tildePath, "~/") {
				usr, err := user.Current()
				if err != nil {
					log.ErrorLog.Printf("Failed to expand corrupted path for '%s': %v", data.Title, err)
					// Fall back to original path
				} else {
					migratedPath = filepath.Join(usr.HomeDir, tildePath[2:])
					log.InfoLog.Printf("Migrated path for instance '%s': %s -> %s", data.Title, data.Path, migratedPath)
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
		log.InfoLog.Printf("Migrating category '%s' to tags for instance '%s'", data.Category, data.Title)
	}

	instance := &Instance{
		Title:       data.Title,
		UUID:        data.UUID,
		Path:        migratedPath, // Use migrated path
		WorkingDir:  data.WorkingDir,
		Branch:      data.Branch,
		Status:      data.Status,
		Height:      data.Height,
		Width:       data.Width,
		CreatedAt:   data.CreatedAt,
		UpdatedAt:   data.UpdatedAt,
		Program:     data.Program,
		Prompt:      data.Prompt,
		Category:    data.Category,
		IsExpanded:  data.IsExpanded,
		Tags:        tags, // Use migrated tags (includes category if needed)
		SessionType: data.SessionType,
		TmuxPrefix:  data.TmuxPrefix,
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
		Checkpoints:      data.Checkpoints,
		ActiveCheckpoint: data.ActiveCheckpoint,
		ForkedFromID:     data.ForkedFromID,
		// History file linkage
		HistoryFilePath: data.HistoryFilePath,
		// One-shot mode
		OneShot: data.OneShot,
		// Project association
		ProjectID: data.ProjectID,
		// Launch command for diagnostics
		LaunchCommand: data.LaunchCommand,
		// MCP server URL for re-injection on restart
		MCPServerURL: data.MCPServerURL,
	}

	// MIGRATION: Assign UUID to existing sessions that pre-date UUID assignment
	if instance.UUID == "" {
		instance.UUID = uuid.New().String()
	}

	// Initialize TagManager backed by the Instance.Tags slice
	instance.tagManager = NewTagManager(&instance.Tags)

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
			log.WarningLog.Printf("Failed to detect worktree info for '%s': %v", instance.Title, err)
			// Non-fatal - session can still work without this info
		} else if instance.GitHubOwner != "" {
			log.InfoLog.Printf("Auto-detected GitHub info for '%s': %s/%s (worktree=%v)",
				instance.Title, instance.GitHubOwner, instance.GitHubRepo, instance.IsWorktree)
		}
	}

	// Initialize session-specific logging
	_ = log.GetSessionLoggers

	// Check if the worktree still exists on disk if the instance is not paused.
	// No mutex is needed here because the instance is not yet shared.
	if !instance.Paused() && instance.gitManager.HasWorktree() {
		worktreePath := instance.gitManager.GetWorktreePath()
		if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
			// Worktree has been deleted — use transitionTo so the state machine is respected.
			// Ready → Paused and Loading → Paused are explicitly allowed for this case.
			log.ForSession(instance.Title).Warning("Worktree directory doesn't exist at '%s', marking as paused", worktreePath)
			if err := instance.transitionTo(Paused); err != nil {
				// If the transition is somehow invalid (e.g. already Stopped), fall back to setStatus.
				log.ForSession(instance.Title).Warning("Could not transition to Paused via state machine (%v), using setStatus", err)
				instance.setStatus(Paused)
			}
		}
	}

	if instance.Paused() {
		instance.started = true
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}

		// Use server socket isolation if specified, otherwise use prefix-only isolation.
		// WithRegistry(nil) prevents a background reconnect loop on isolated sockets —
		// the loop tries attach-session on a keepalive that doesn't exist there, causing
		// intermittent exit status 1 from concurrent new-session calls.
		if instance.TmuxServerSocket != "" {
			instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket, tmux.WithRegistry(nil)))
		} else {
			instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix))
		}
	} else if instance.Status == Stopped {
		// Wire the tmux session object so DoesSessionExist() can be called.
		tmuxPrefix := instance.TmuxPrefix
		if tmuxPrefix == "" {
			tmuxPrefix = "staplersquad_"
		}
		if instance.TmuxServerSocket != "" {
			instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithServerSocket(instance.Title, instance.Program, tmuxPrefix, instance.TmuxServerSocket, tmux.WithRegistry(nil)))
		} else {
			instance.tmuxManager.SetSession(tmux.NewTmuxSessionWithPrefix(instance.Title, instance.Program, tmuxPrefix))
		}
		// If the underlying tmux session is still alive (e.g. server crashed mid-write
		// or exit callback fired falsely), recover it rather than leave it stuck as Stopped.
		if instance.tmuxManager.DoesSessionExist() {
			log.WarningLog.Printf("[FromInstanceData] Session '%s' stored as Stopped but tmux is alive — recovering to Running", instance.Title)
			instance.setStatus(Running)
			if err := instance.Start(false); err != nil {
				log.WarningLog.Printf("[FromInstanceData] Recovery Start failed for '%s': %v — keeping Stopped", instance.Title, err)
				instance.setStatus(Stopped)
				instance.started = true
			}
		} else {
			instance.started = true
		}
	} else {
		if err := instance.Start(false); err != nil {
			return nil, err
		}
	}

	return instance, nil
}
