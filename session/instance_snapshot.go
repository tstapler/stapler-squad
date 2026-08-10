package session

// instance_snapshot.go defines InstanceSnapshot and buildSnapshot.
//
// IMPORTANT: InstanceSnapshot's field set must be kept in sync with Instance's
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

import (
	"time"

	"github.com/tstapler/stapler-squad/session/artifacts"
)

// GitHubIntegration groups all GitHub PR / URL integration fields within
// InstanceSnapshot (CDD Epic 3, Task 3.1a). Access via snap.GitHub.GitHubPRURL etc.
type GitHubIntegration struct {
	// Repository identity and PR linkage
	GitHubPRNumber  int
	GitHubPRURL     string
	GitHubOwner     string
	GitHubRepo      string
	GitHubSourceRef string
	ClonedRepoPath  string
	MainRepoPath    string
	IsWorktree      bool
	GitHubIsFork    bool

	// PR status fields (populated by PRStatusPoller)
	GitHubPRState          string
	GitHubPRIsDraft        bool
	GitHubPRPriority       string
	GitHubApprovedCount    int
	GitHubChangesReqCount  int
	GitHubCheckConclusion  string
	GitHubPRStatusTerminal bool
	LastPRStatusCheck      time.Time
}

// AutonomousModeState groups all autonomous-mode fields within InstanceSnapshot
// (CDD Epic 3, Task 3.1b). Access via snap.Autonomous.AutonomousMode etc.
type AutonomousModeState struct {
	AutonomousMode     bool
	AutonomousTurn     int32
	AutonomousMaxTurns int32
	AutonomousOutcome  string
}

// InstanceSnapshot is a point-in-time, read-safe copy of all mutable Instance
// fields. Published via Instance.snapshot (atomic.Pointer) inside stateMutex
// at the end of every mutator so lock-free readers always see consistent state.
//
// Excluded: manager/dependency objects (gitManager, vncManager, cdpManager,
// processManager, controllerManager, tagManager, shellRepo, historyDetector)
// and callback registrations (lifecycleListeners, onRateLimitDetected,
// onStatusChange). Those are behavior, not data; callers needing them go
// through dedicated accessors or mailbox round-trips (Epic 3).
type InstanceSnapshot struct {
	// Identity / config
	ID               string
	UUID             string
	Title            string
	Path             string
	WorkingDir       string
	Branch           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Status           Status
	Program          string
	Height           int
	Width            int
	AutoYes          bool
	IsExpanded       bool
	Prompt           string
	InitialPrompt    string
	Category         string
	SessionType      SessionType
	TmuxPrefix       string
	TmuxServerSocket string
	Tags             []string // defensive deep copy — see buildSnapshot

	// Autonomous mode (grouped — access as snap.Autonomous.AutonomousMode)
	Autonomous AutonomousModeState

	// GitHub PR / URL integration (grouped — access as snap.GitHub.GitHubPRURL)
	GitHub GitHubIntegration

	// Checkpoints
	Checkpoints      CheckpointList // defensive deep copy — see buildSnapshot
	ActiveCheckpoint string
	ForkedFromID     string

	// Misc config
	OneShot             bool
	Hidden              bool
	ProjectID           string
	HistoryFilePath     string
	MCPServerURL        string
	AppendSystemPrompt  string
	AllowedTools        string
	PermissionMode      string
	RateLimitAutoResume *bool // copy of pointee — see buildSnapshot
	PauseReason         string
	ExitReason          string
	WorkflowID          string
	EnvVars             map[string]string // defensive deep copy — see buildSnapshot
	CLIFlags            string
	ArchivedAt          *time.Time // copy of pointee — see buildSnapshot

	// Review queue / activity state (embedded value — copied by value)
	ReviewState

	// Instance type and management metadata
	InstanceType     InstanceType
	IsManaged        bool
	ExternalMetadata *ExternalInstanceMetadata // copy of pointee — see buildSnapshot
	Permissions      InstancePermissions       // RequiresConfirmation map deep-copied
	Artifacts        *artifacts.SessionArtifactsBlob
}

// buildSnapshot builds a point-in-time InstanceSnapshot from i.
// Must be called while i.mu is held so all fields are stable.
// All reference types (slices, maps, pointers) are deep-copied so that
// mutations to the live Instance after Unlock cannot corrupt the snapshot.
func buildSnapshot(i *Instance) *InstanceSnapshot {
	s := &InstanceSnapshot{
		ID:               i.ID,
		UUID:             i.UUID,
		Title:            i.Title,
		Path:             i.Path,
		WorkingDir:       i.WorkingDir,
		Branch:           i.Branch,
		CreatedAt:        i.CreatedAt,
		UpdatedAt:        i.UpdatedAt,
		Status:           i.Status,
		Program:          i.Program,
		Height:           i.Height,
		Width:            i.Width,
		AutoYes:          i.AutoYes,
		IsExpanded:       i.IsExpanded,
		Prompt:           i.Prompt,
		InitialPrompt:    i.InitialPrompt,
		Category:         i.Category,
		SessionType:      i.SessionType,
		TmuxPrefix:       i.TmuxPrefix,
		TmuxServerSocket: i.TmuxServerSocket,
		Tags:             append([]string(nil), i.Tags...),
		Autonomous: AutonomousModeState{
			AutonomousMode:     i.AutonomousMode,
			AutonomousTurn:     i.AutonomousTurn,
			AutonomousMaxTurns: i.AutonomousMaxTurns,
			AutonomousOutcome:  i.AutonomousOutcome,
		},
		GitHub: GitHubIntegration{
			GitHubPRNumber:         i.GitHubPRNumber,
			GitHubPRURL:            i.GitHubPRURL,
			GitHubOwner:            i.GitHubOwner,
			GitHubRepo:             i.GitHubRepo,
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
			GitHubPRStatusTerminal: i.GitHubPRStatusTerminal,
			LastPRStatusCheck:      i.LastPRStatusCheck,
		},
		Checkpoints:        append(CheckpointList(nil), i.Checkpoints...),
		ActiveCheckpoint:   i.ActiveCheckpoint,
		ForkedFromID:       i.ForkedFromID,
		OneShot:            i.OneShot,
		Hidden:             i.Hidden,
		ProjectID:          i.ProjectID,
		HistoryFilePath:    i.HistoryFilePath,
		MCPServerURL:       i.MCPServerURL,
		AppendSystemPrompt: i.AppendSystemPrompt,
		AllowedTools:       i.AllowedTools,
		PermissionMode:     i.PermissionMode,
		PauseReason:        i.PauseReason,
		ExitReason:         i.ExitReason,
		WorkflowID:         i.WorkflowID,
		CLIFlags:           i.CLIFlags,
		ReviewState:        i.ReviewState,
		InstanceType:       i.InstanceType,
		IsManaged:          i.IsManaged,
		Artifacts:          i.Artifacts,
	}

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

	return s
}
