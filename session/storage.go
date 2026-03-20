package session

import (
	"github.com/tstapler/stapler-squad/log"
	"context"
	"fmt"
	"time"
)

// InstanceData represents the serializable data of an Instance
type InstanceData struct {
	Title      string    `json:"title"`
	Path       string    `json:"path"`
	WorkingDir string    `json:"working_dir"`
	Branch     string    `json:"branch"`
	Status     Status    `json:"status"`
	Height     int       `json:"height"`
	Width      int       `json:"width"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	AutoYes    bool      `json:"auto_yes"`
	Prompt     string    `json:"prompt"`

	Program          string          `json:"program"`
	ExistingWorktree string          `json:"existing_worktree,omitempty"`
	Worktree         GitWorktreeData `json:"worktree"`
	DiffStats        DiffStatsData   `json:"diff_stats"`

	// New fields for session organization and grouping
	Category   string   `json:"category,omitempty"`
	IsExpanded bool     `json:"is_expanded,omitempty"`
	Tags       []string `json:"tags,omitempty"` // Multi-valued tags for flexible organization

	// Session type determines the workflow (directory, new_worktree, existing_worktree)
	SessionType SessionType `json:"session_type,omitempty"`

	// GitHub integration fields for PR/URL-based session creation
	GitHubPRNumber  int    `json:"github_pr_number,omitempty"`
	GitHubPRURL     string `json:"github_pr_url,omitempty"`
	GitHubOwner     string `json:"github_owner,omitempty"`
	GitHubRepo      string `json:"github_repo,omitempty"`
	GitHubSourceRef string `json:"github_source_ref,omitempty"`
	ClonedRepoPath  string `json:"cloned_repo_path,omitempty"`
	// Worktree detection fields
	MainRepoPath string `json:"main_repo_path,omitempty"` // Path to main repo when this is a worktree
	IsWorktree   bool   `json:"is_worktree,omitempty"`    // True if path is a git worktree

	// Claude Code session persistence
	ClaudeSession ClaudeSessionData `json:"claude_session,omitempty"`
	// Tmux session prefix for isolation
	TmuxPrefix string `json:"tmux_prefix,omitempty"`

	// Terminal update timestamps for activity tracking
	LastTerminalUpdate   time.Time `json:"last_terminal_update,omitempty"`
	LastMeaningfulOutput time.Time `json:"last_meaningful_output,omitempty"`

	// Content signature for detecting actual terminal changes vs restarts
	// This is a SHA256 hash of the terminal content used to prevent false "new activity"
	// notifications when app restarts but terminal content hasn't changed
	LastOutputSignature string `json:"last_output_signature,omitempty"`

	// Review queue spam prevention
	LastAddedToQueue time.Time `json:"last_added_to_queue,omitempty"`

	// User interaction tracking
	// LastViewed tracks when the user last viewed this session (terminal, session details, etc.)
	// Used for smarter review queue notifications (don't notify if just viewed)
	LastViewed time.Time `json:"last_viewed,omitempty"`

	// Review queue snooze tracking
	// LastAcknowledged tracks when the user last dismissed this session from review queue
	// Sessions acknowledged after their last update won't appear in the queue until they update again
	LastAcknowledged time.Time `json:"last_acknowledged,omitempty"`

	// Prompt detection and interaction tracking for smart review queue behavior
	LastPromptDetected   time.Time `json:"last_prompt_detected,omitempty"`
	LastPromptSignature  string    `json:"last_prompt_signature,omitempty"`
	LastUserResponse     time.Time `json:"last_user_response,omitempty"`
	ProcessingGraceUntil time.Time `json:"processing_grace_until,omitempty"`
}

// GitWorktreeData represents the serializable data of a GitWorktree
type GitWorktreeData struct {
	RepoPath      string `json:"repo_path"`
	WorktreePath  string `json:"worktree_path"`
	SessionName   string `json:"session_name"`
	BranchName    string `json:"branch_name"`
	BaseCommitSHA string `json:"base_commit_sha"`
}

// DiffStatsData represents the serializable data of a DiffStats
// Note: Content is excluded from JSON serialization to reduce state file size.
// Diffs are generated on-demand via GetSessionDiff RPC when needed.
type DiffStatsData struct {
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
	Content string `json:"-"` // Excluded from serialization - generated on-demand
}

// ClaudeSessionData represents Claude Code session information
type ClaudeSessionData struct {
	SessionID      string            `json:"session_id,omitempty"`      // Claude Code session identifier
	ConversationID string            `json:"conversation_id,omitempty"` // Conversation thread ID
	ProjectName    string            `json:"project_name,omitempty"`    // Project name in Claude Code
	LastAttached   time.Time         `json:"last_attached,omitempty"`   // When this session was last used
	Settings       ClaudeSettings    `json:"settings,omitempty"`        // User preferences for Claude Code
	Metadata       map[string]string `json:"metadata,omitempty"`        // Additional session metadata
}

// ClaudeSettings contains user preferences for Claude Code integration
type ClaudeSettings struct {
	AutoReattach          bool   `json:"auto_reattach"`           // Automatically reattach to last session on resume
	PreferredSessionName  string `json:"preferred_session_name"`  // Preferred session naming pattern
	CreateNewOnMissing    bool   `json:"create_new_on_missing"`   // Create new session if previous one is missing
	ShowSessionSelector   bool   `json:"show_session_selector"`   // Show session selection menu on resume
	SessionTimeoutMinutes int    `json:"session_timeout_minutes"` // Consider sessions stale after this time
}

// Storage handles saving and loading instances via the repository backend.
type Storage struct {
	repo Repository
}

// NewStorageWithRepository creates a Storage backed by a Repository.
func NewStorageWithRepository(repo Repository) (*Storage, error) {
	return &Storage{repo: repo}, nil
}

// Close performs graceful shutdown of storage.
func (s *Storage) Close() error {
	return nil
}

// SaveInstances upserts each started instance into the repository.
func (s *Storage) SaveInstances(instances []*Instance) error {
	return s.saveInstancesToRepo(instances)
}

// saveInstancesToRepo upserts each started instance into the repository.
// The DB handles concurrent writers so no merging is required.
func (s *Storage) saveInstancesToRepo(instances []*Instance) error {
	ctx := context.Background()
	for _, inst := range instances {
		if !inst.Started() {
			continue
		}
		data := inst.ToInstanceData()
		log.InfoLog.Printf("[SaveInstances] Converting instance '%s': IsWorktree=%v, MainRepoPath=%s, GitHubOwner=%s, GitHubRepo=%s",
			data.Title, data.IsWorktree, data.MainRepoPath, data.GitHubOwner, data.GitHubRepo)
		if err := s.repo.Update(ctx, data); err != nil {
			// Not found → create it
			if createErr := s.repo.Create(ctx, data); createErr != nil {
				log.ErrorLog.Printf("[SaveInstances] Failed to upsert instance '%s': update=%v, create=%v",
					data.Title, err, createErr)
			}
		}
	}
	return nil
}

// SaveInstancesSync saves instances synchronously (same as SaveInstances for the repo backend).
func (s *Storage) SaveInstancesSync(instances []*Instance) error {
	return s.saveInstancesToRepo(instances)
}

// LoadInstances loads the list of instances from the repository.
func (s *Storage) LoadInstances() ([]*Instance, error) {
	dataSlice, err := s.repo.List(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to list instances from repository: %w", err)
	}
	instances := make([]*Instance, 0, len(dataSlice))
	for _, data := range dataSlice {
		inst, err := FromInstanceData(data)
		if err != nil {
			log.WarningLog.Printf("Skipping instance '%s' from repository: %v", data.Title, err)
			continue
		}
		instances = append(instances, inst)
	}
	return instances, nil
}

// DeleteInstance removes an instance from storage.
func (s *Storage) DeleteInstance(title string) error {
	return s.repo.Delete(context.Background(), title)
}

// AddInstance adds a new instance to storage.
// Unlike SaveInstances, this does not require instance.Started() to be true.
func (s *Storage) AddInstance(instance *Instance) error {
	data := instance.ToInstanceData()
	ctx := context.Background()
	if err := s.repo.Create(ctx, data); err != nil {
		// Already exists → update instead
		return s.repo.Update(ctx, data)
	}
	return nil
}

// UpdateInstance updates an existing instance in storage.
func (s *Storage) UpdateInstance(instance *Instance) error {
	return s.repo.Update(context.Background(), instance.ToInstanceData())
}

// DeleteAllInstances removes all stored instances.
func (s *Storage) DeleteAllInstances() error {
	ctx := context.Background()
	dataSlice, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("failed to list instances for deletion: %w", err)
	}
	for _, data := range dataSlice {
		if err := s.repo.Delete(ctx, data.Title); err != nil {
			log.WarningLog.Printf("Failed to delete instance '%s': %v", data.Title, err)
		}
	}
	return nil
}

// updateFieldInRepo loads the InstanceData for title, applies fn to it, then saves.
// Used for partial-field updates.
func (s *Storage) updateFieldInRepo(title string, fn func(*InstanceData)) error {
	ctx := context.Background()
	data, err := s.repo.Get(ctx, title)
	if err != nil {
		return fmt.Errorf("failed to get instance '%s': %w", title, err)
	}
	fn(data)
	return s.repo.Update(ctx, *data)
}

// UpdateInstanceTimestampsOnly updates ONLY the timestamp fields in storage without
// creating Instance objects. This preserves in-memory state like controllers.
// This is critical for WebSocket terminal streaming which updates timestamps frequently.
func (s *Storage) UpdateInstanceTimestampsOnly(title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string, lastViewed time.Time) error {
	if err := s.repo.UpdateTimestamps(context.Background(), title, lastTerminalUpdate, lastMeaningfulOutput, lastOutputSignature); err != nil {
		return err
	}
	if !lastViewed.IsZero() {
		return s.updateFieldInRepo(title, func(d *InstanceData) { d.LastViewed = lastViewed })
	}
	return nil
}

// UpdateInstanceLastAddedToQueue updates ONLY the LastAddedToQueue field for a specific instance.
func (s *Storage) UpdateInstanceLastAddedToQueue(title string, lastAddedToQueue time.Time) error {
	return s.updateFieldInRepo(title, func(d *InstanceData) { d.LastAddedToQueue = lastAddedToQueue })
}

// UpdateInstanceLastUserResponse updates just the LastUserResponse timestamp for a specific instance.
func (s *Storage) UpdateInstanceLastUserResponse(title string, lastUserResponse time.Time) error {
	return s.updateFieldInRepo(title, func(d *InstanceData) { d.LastUserResponse = lastUserResponse })
}

// UpdateInstanceProcessingGrace updates just the ProcessingGraceUntil timestamp for a specific instance.
func (s *Storage) UpdateInstanceProcessingGrace(title string, processingGraceUntil time.Time) error {
	return s.updateFieldInRepo(title, func(d *InstanceData) { d.ProcessingGraceUntil = processingGraceUntil })
}

// --- Session-first convenience methods (Task 2.5) ---
// These delegate directly to the Repository's Session-based methods.
// Prefer these over the deprecated InstanceData-based methods for new code.

// GetSession retrieves a session by title using the Session domain model.
// Use ContextOptions presets (ContextMinimal, ContextUIView, etc.) to control what is loaded.
func (s *Storage) GetSession(ctx context.Context, title string, opts ContextOptions) (*Session, error) {
	return s.repo.GetSession(ctx, title, opts)
}

// ListSessions retrieves all sessions using the Session domain model.
// Use ContextOptions presets (ContextMinimal, ContextUIView, etc.) to control what is loaded.
func (s *Storage) ListSessions(ctx context.Context, opts ContextOptions) ([]*Session, error) {
	return s.repo.ListSessions(ctx, opts)
}

// SaveSession upserts a session using the Session domain model.
// If the session exists it is updated; otherwise it is created.
// Deprecated InstanceData-based methods (SaveInstances, LoadInstances) remain for backward compatibility.
func (s *Storage) SaveSession(ctx context.Context, session *Session) error {
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return s.repo.CreateSession(ctx, session)
	}
	return nil
}
