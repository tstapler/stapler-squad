package instance

// instance_snapshot.go defines buildSnapshot. The InstanceSnapshot struct itself
// lives in session/domain; Snapshot() and the actor setters stay with Instance.
//
// IMPORTANT: domain.InstanceSnapshot's field set must be kept in sync with Instance's
// mutable fields. Whenever a field is added to Instance, add it here too — this
// is the single authoritative place that "knows" every mutable field, replacing
// the silently-drifting ad hoc read-allowlists that produced the unguarded-access
// catalog described in architecture.md §1.2.
//
// PPROF VERIFICATION (IAC Epic 2):
// After deploying the lock-free reader conversions in Epic 2, use the mutex/block
// profile to confirm reduced contention:
//
//	curl -s http://localhost:8543/debug/pprof/mutex > mutex.prof
//	go tool pprof -top mutex.prof
//
// Expected: stateMutex contention (RLock sites in instance_adapter.go,
// capacity_monitor.go, review_queue_poller.go, pr_status_poller.go, and
// connectrpc_websocket.go) no longer appears in the top lock holders.
// Any remaining stateMutex entries should be write-side only (Update*, transition*).
//
//	curl -s http://localhost:8543/debug/pprof/block > block.prof
//	go tool pprof -top block.prof
//
// Expected: goroutine blocking on (*deadlock.RWMutex).RLock should drop
// proportionally to the reader-path traffic these callers generate.
//
// GitHub.PRs / HasPRLinks / LegacyPRUnrepresented (multi-PR tracking) are fields
// of this authoritative list too: they are copied from Instance, and ToInstanceData
// copies them back out so a full save never rebuilds them away.

import (
	"github.com/tstapler/stapler-squad/session/domain"
)

// buildSnapshot builds a point-in-time InstanceSnapshot from i.
// Must be called while i.mu is held so all fields are stable.
// All reference types (slices, maps, pointers) are deep-copied so that
// mutations to the live Instance after Unlock cannot corrupt the snapshot.
func BuildSnapshot(i *Instance) *domain.InstanceSnapshot {
	s := &domain.InstanceSnapshot{
		ID:                        i.ID,
		UUID:                      i.UUID,
		Title:                     i.Title,
		Path:                      i.Path,
		WorkingDir:                i.WorkingDir,
		Branch:                    i.Branch,
		CreationWarning:           i.CreationWarning,
		CreatedAt:                 i.CreatedAt,
		UpdatedAt:                 i.UpdatedAt,
		Status:                    i.Status,
		CreationProgressUpdatedAt: i.creationProgressUpdatedAt,
		Program:                   i.Program,
		AltScreenActive:           i.AltScreenActive,
		AltScreenBootstrapped:     i.AltScreenBootstrapped,
		Height:                    i.Height,
		Width:                     i.Width,
		AutoYes:                   i.AutoYes,
		AutoApprove:               i.AutoApprove,
		IsExpanded:                i.IsExpanded,
		Prompt:                    i.Prompt,
		InitialPrompt:             i.InitialPrompt,
		InitialPromptSentAt:       i.InitialPromptSentAt,
		Category:                  i.Category,
		Note:                      i.Note,
		SessionType:               i.SessionType,
		TmuxPrefix:                i.TmuxPrefix,
		TmuxServerSocket:          i.TmuxServerSocket,
		Backend:                   i.Backend,
		Tags:                      append([]string(nil), i.Tags...),
		Autonomous: domain.AutonomousModeState{
			AutonomousMode:       i.AutonomousMode,
			AutonomousTurn:       i.AutonomousTurn,
			AutonomousMaxTurns:   i.AutonomousMaxTurns,
			AutonomousOutcome:    i.AutonomousOutcome,
			AutonomousStopReason: i.AutonomousStopReason,
		},
		GitHub: domain.GitHubIntegration{
			GitHubPRNumber:         i.GitHubPRNumber,
			GitHubPRURL:            i.GitHubPRURL,
			GitHubOwner:            i.GitHubOwner,
			GitHubRepo:             i.GitHubRepo,
			GitHubHost:             i.GitHubHost,
			GitHubSourceRef:        i.GitHubSourceRef,
			ClonedRepoPath:         i.ClonedRepoPath,
			MainRepoPath:           i.MainRepoPath,
			IsWorktree:             i.IsWorktree,
			GitHubIsFork:           i.GitHubIsFork,
			GitHubPRState:          i.GitHubPRState,
			GitHubPRIsDraft:        i.GitHubPRIsDraft,
			GitHubPRPriority:       i.GitHubPRPriority,
			GitHubApprovedCount:    i.GitHubApprovedCount,
			GitHubChangesReqCount:  i.GitHubChangesReqCount,
			GitHubCheckConclusion:  i.GitHubCheckConclusion,
			GitHubChecks:           append([]domain.GitHubCheckItem(nil), i.GitHubChecks...),
			GitHubReviewFeedback:   append([]domain.GitHubReviewItem(nil), i.GitHubReviewFeedback...),
			GitHubMergeable:        i.GitHubMergeable,
			GitHubPRStatusTerminal: i.GitHubPRStatusTerminal,
			LastPRStatusCheck:      i.LastPRStatusCheck,
			PRs:                    append([]domain.PRView(nil), i.PRs...),
			LegacyPRUnrepresented:  i.LegacyPRUnrepresented,
			HasPRLinks:             i.HasPRLinks,
		},
		Checkpoints:                append(domain.CheckpointList(nil), i.Checkpoints...),
		ActiveCheckpoint:           i.ActiveCheckpoint,
		ForkedFromID:               i.ForkedFromID,
		RestartedFromSessionID:     i.RestartedFromSessionID,
		OneShot:                    i.OneShot,
		Hidden:                     i.Hidden,
		Pinned:                     i.Pinned,
		ProjectID:                  i.ProjectID,
		HistoryFilePath:            i.HistoryFilePath,
		EverHadConversationHistory: i.EverHadConversationHistory,
		LastReviveOutcome:          i.LastReviveOutcome,
		MCPServerURL:               i.MCPServerURL,
		AppendSystemPrompt:         i.AppendSystemPrompt,
		AllowedTools:               i.AllowedTools,
		PermissionMode:             i.PermissionMode,
		PauseReason:                i.PauseReason,
		ExitReason:                 i.ExitReason,
		WorkflowID:                 i.WorkflowID,
		CLIFlags:                   i.CLIFlags,
		ExistingWorktree:           i.ExistingWorktree,
		ExtraArgs:                  append([]string(nil), i.ExtraArgs...),
		LaunchCommand:              i.LaunchCommand,
		CreationProgress:           i.CreationProgress,
		Retry:                      i.RetryState,
		ReviewState:                i.ReviewState,
		LastMeaningfulChangeAt:     i.LastMeaningfulChangeAt,
		TurnEnded:                  i.TurnEnded,
		InstanceType:               i.InstanceType,
		IsManaged:                  i.IsManaged,
		Artifacts:                  i.Artifacts,
		ContextHealth:              i.ContextHealth,
	}

	// With any link, the legacy scalars show the active PR (pure: no config read).
	if m, ok := ProjectLegacyPRMirror(s.GitHub.PRs, s.Branch); ok {
		s.GitHub.GitHubPRNumber = m.Number
		s.GitHub.GitHubPRURL = m.URL
		s.GitHub.GitHubPRStatusTerminal = m.Terminal
	}

	s.Retry.RetryHistory = append([]domain.RetryAttemptRecord(nil), i.RetryHistory...)

	// Deep copy RateLimitAutoResume *bool
	if i.RateLimitAutoResume != nil {
		v := *i.RateLimitAutoResume
		s.RateLimitAutoResume = &v
	}

	// Deep copy ArchivedAt *time.Time
	if i.ArchivedAt != nil {
		t := *i.ArchivedAt
		s.ArchivedAt = &t
	}

	// Deep copy EnvVars map[string]string
	if i.EnvVars != nil {
		s.EnvVars = make(map[string]string, len(i.EnvVars))
		for k, v := range i.EnvVars {
			s.EnvVars[k] = v
		}
	}

	// Deep copy Permissions.RequiresConfirmation map[string]bool
	s.Permissions = i.Permissions
	if i.Permissions.RequiresConfirmation != nil {
		s.Permissions.RequiresConfirmation = make(map[string]bool, len(i.Permissions.RequiresConfirmation))
		for k, v := range i.Permissions.RequiresConfirmation {
			s.Permissions.RequiresConfirmation[k] = v
		}
	}

	// Deep copy ExternalMetadata pointer (copy the pointee, not the pointer)
	if i.ExternalMetadata != nil {
		meta := *i.ExternalMetadata
		s.ExternalMetadata = &meta
	}

	// Deep copy RuleTagProvenance map[string]string (ADR-002) — never alias the
	// live Instance map, mirroring Tags' copy above (instance-lock-free-reads.md).
	if i.RuleTagProvenance != nil {
		s.RuleTagProvenance = make(map[string]string, len(i.RuleTagProvenance))
		for k, v := range i.RuleTagProvenance {
			s.RuleTagProvenance[k] = v
		}
	}

	// Deep copy SuppressedRuleTags map[string]bool (ADR-002).
	if i.SuppressedRuleTags != nil {
		s.SuppressedRuleTags = make(map[string]bool, len(i.SuppressedRuleTags))
		for k, v := range i.SuppressedRuleTags {
			s.SuppressedRuleTags[k] = v
		}
	}

	return s
}
