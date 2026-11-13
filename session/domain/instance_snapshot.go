package domain

import (
	"time"
)

// GitHubIntegration groups all GitHub PR / URL integration fields within
// InstanceSnapshot (CDD Epic 3, Task 3.1a). Access via snap.GitHub.GitHubPRURL etc.
type GitHubIntegration struct {
	// Repository identity and PR linkage
	GitHubPRNumber  int
	GitHubPRURL     string
	GitHubOwner     string
	GitHubRepo      string
	GitHubHost      string
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
	GitHubChecks           []GitHubCheckItem
	GitHubReviewFeedback   []GitHubReviewItem
	GitHubMergeable        string
	GitHubPRStatusTerminal bool
	LastPRStatusCheck      time.Time

	// Multi-PR set. PRs is a private copy per snapshot (PRView holds only
	// values); the scalars above mirror ActivePR(PRs) whenever PRs is non-empty.
	// LegacyPRUnrepresented and HasPRLinks are carried, never derived here.
	PRs                   []PRView
	LegacyPRUnrepresented bool
	HasPRLinks            bool
}

// AutonomousModeState groups all autonomous-mode fields within InstanceSnapshot
// (CDD Epic 3, Task 3.1b). Access via snap.Autonomous.AutonomousMode etc.
type AutonomousModeState struct {
	AutonomousMode     bool
	AutonomousTurn     int32
	AutonomousMaxTurns int32
	AutonomousOutcome  string
	// AutonomousStopReason is why the last autonomous run ended ("" if none).
	AutonomousStopReason StopReason
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
	ID                    string
	UUID                  string
	Title                 string
	Path                  string
	WorkingDir            string
	Branch                string
	CreationWarning       string
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Status                Status
	Program               string
	AltScreenActive       bool
	AltScreenBootstrapped bool
	Height                int
	Width                 int
	AutoYes               bool
	AutoApprove           bool
	IsExpanded            bool
	Prompt                string
	InitialPrompt         string
	InitialPromptSentAt   time.Time
	Category              string
	Note                  string
	SessionType           SessionType
	TmuxPrefix            string
	TmuxServerSocket      string
	Backend               ProcessManagerBackend
	Tags                  []string // defensive deep copy — see buildSnapshot

	// RuleTagProvenance/SuppressedRuleTags mirror Instance's ADR-002 tag-provenance
	// fields — both defensive deep copies, see buildSnapshot.
	RuleTagProvenance  map[string]string
	SuppressedRuleTags map[string]bool

	// Autonomous mode (grouped — access as snap.Autonomous.AutonomousMode)
	Autonomous AutonomousModeState

	// GitHub PR / URL integration (grouped — access as snap.GitHub.GitHubPRURL)
	GitHub GitHubIntegration

	// Checkpoints
	Checkpoints      CheckpointList // defensive deep copy — see buildSnapshot
	ActiveCheckpoint string
	ForkedFromID     string
	// RestartedFromSessionID — see Instance.RestartedFromSessionID's doc comment.
	RestartedFromSessionID string

	// Misc config
	OneShot                    bool
	Hidden                     bool
	Pinned                     bool
	ProjectID                  string
	HistoryFilePath            string
	EverHadConversationHistory bool
	LastReviveOutcome          ReviveOutcome
	MCPServerURL               string
	AppendSystemPrompt         string
	AllowedTools               string
	PermissionMode             string
	RateLimitAutoResume        *bool // copy of pointee — see buildSnapshot
	PauseReason                string
	ExitReason                 string
	WorkflowID                 string
	EnvVars                    map[string]string // defensive deep copy — see buildSnapshot
	CLIFlags                   string
	ArchivedAt                 *time.Time // copy of pointee — see buildSnapshot

	// CreationProgressUpdatedAt mirrors Instance.creationProgressUpdatedAt (Epic
	// 1.1.4/4.1) — see ToInstanceData/FromInstanceData for the persisted round trip.
	CreationProgressUpdatedAt time.Time

	// ExistingWorktree, ExtraArgs, LaunchCommand and CreationProgress mirror the
	// Instance fields of the same name. ExtraArgs is a deep copy.
	ExistingWorktree string
	ExtraArgs        []string
	LaunchCommand    string
	CreationProgress string

	// Retry is the automated retry lifecycle (RetryHistory deep-copied; the
	// history is already capped at 10 entries by RetryState.RecordAttempt).
	Retry RetryState

	// LastMeaningfulChangeAt and TurnEnded mirror the Instance fields of the same name.
	LastMeaningfulChangeAt time.Time
	TurnEnded              TurnEnded

	// Review queue / activity state (embedded value — copied by value)
	ReviewState

	// Instance type and management metadata
	InstanceType     InstanceType
	IsManaged        bool
	ExternalMetadata *ExternalInstanceMetadata // copy of pointee — see buildSnapshot
	Permissions      InstancePermissions       // RequiresConfirmation map deep-copied
	Artifacts        *SessionArtifactsBlob
	ContextHealth    ContextHealthVerdict // flat value; no deep copy needed
}

// ActiveCheckConclusion is the CI conclusion of the session's active PR. The
// GitHubCheckConclusion scalar is not mirrored per PR, so with several PRs it can
// describe a merged one; it is used only when the session has no links.
func (s *InstanceSnapshot) ActiveCheckConclusion() string {
	if active := ActivePR(s.GitHub.PRs, s.Branch); active != nil {
		return active.CheckConclusion
	}
	return s.GitHub.GitHubCheckConclusion
}
