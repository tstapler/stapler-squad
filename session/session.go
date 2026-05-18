package session

import (
	"time"
)

// Session represents the core domain entity for an AI agent session.
// It contains only universally required fields, with optional contexts
// for deployment-specific functionality.
//
// Context types are defined in contexts.go:
// - GitContext: Git repository, branch, PR integration
// - FilesystemContext: Paths, working directories, worktree detection
// - TerminalContext: Terminal dimensions, tmux configuration
// - UIPreferences: Categories, tags, display preferences
// - ActivityTracking: Timestamps, output signatures, queue tracking
// - CloudContext: Cloud provider, API configuration
type Session struct {
	// Identity
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Process
	Status  Status `json:"status"`
	Program string `json:"program"`

	// Configuration
	AutoYes bool   `json:"auto_yes,omitempty"`
	Prompt  string `json:"prompt,omitempty"`

	// Optional contexts (nil = not loaded or not applicable)
	Git        *GitContext        `json:"git,omitempty"`
	Filesystem *FilesystemContext `json:"filesystem,omitempty"`
	Terminal   *TerminalContext   `json:"terminal,omitempty"`
	UI         *UIPreferences     `json:"ui,omitempty"`
	Activity   *ActivityTracking  `json:"activity,omitempty"`
	Cloud      *CloudContext      `json:"cloud,omitempty"`
}

// NewSession creates a new Session with the required fields.
// Optional contexts can be added using the With* methods.
func NewSession(title, program string) *Session {
	now := time.Now()
	return &Session{
		ID:        generateSessionID(title, now),
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Status:    Creating,
		Program:   program,
		AutoYes:   false,
		Prompt:    "",
	}
}

// generateSessionID creates a unique session ID based on title and timestamp.
func generateSessionID(title string, t time.Time) string {
	// Use a combination of title and timestamp for uniqueness
	// This is a simple implementation; can be replaced with UUID if needed
	return title + "_" + t.Format("20060102_150405")
}

// Context Checkers - return true if the context is loaded and available

// HasGitContext returns true if Git context is available.
func (s *Session) HasGitContext() bool {
	return s.Git != nil
}

// HasFilesystemContext returns true if filesystem context is available.
func (s *Session) HasFilesystemContext() bool {
	return s.Filesystem != nil
}

// HasTerminalContext returns true if terminal context is available.
func (s *Session) HasTerminalContext() bool {
	return s.Terminal != nil
}

// HasUIPreferences returns true if UI preferences are available.
func (s *Session) HasUIPreferences() bool {
	return s.UI != nil
}

// HasActivityTracking returns true if activity tracking is available.
func (s *Session) HasActivityTracking() bool {
	return s.Activity != nil
}

// HasCloudContext returns true if cloud context is available.
func (s *Session) HasCloudContext() bool {
	return s.Cloud != nil
}

// Context Setters - builder pattern for adding contexts

// WithGitContext adds Git context to the session.
func (s *Session) WithGitContext(git *GitContext) *Session {
	s.Git = git
	s.UpdatedAt = time.Now()
	return s
}

// WithFilesystemContext adds filesystem context to the session.
func (s *Session) WithFilesystemContext(fs *FilesystemContext) *Session {
	s.Filesystem = fs
	s.UpdatedAt = time.Now()
	return s
}

// WithTerminalContext adds terminal context to the session.
func (s *Session) WithTerminalContext(terminal *TerminalContext) *Session {
	s.Terminal = terminal
	s.UpdatedAt = time.Now()
	return s
}

// WithUIPreferences adds UI preferences to the session.
func (s *Session) WithUIPreferences(ui *UIPreferences) *Session {
	s.UI = ui
	s.UpdatedAt = time.Now()
	return s
}

// WithActivityTracking adds activity tracking to the session.
func (s *Session) WithActivityTracking(activity *ActivityTracking) *Session {
	s.Activity = activity
	s.UpdatedAt = time.Now()
	return s
}

// WithCloudContext adds cloud context to the session.
func (s *Session) WithCloudContext(cloud *CloudContext) *Session {
	s.Cloud = cloud
	s.UpdatedAt = time.Now()
	return s
}

// Convenience Accessors - safe access with defaults

// GetBranch returns the Git branch name, or empty string if no Git context.
func (s *Session) GetBranch() string {
	if s.Git != nil {
		return s.Git.Branch
	}
	return ""
}

// GetPath returns the filesystem project path, or empty string if no filesystem context.
func (s *Session) GetPath() string {
	if s.Filesystem != nil {
		return s.Filesystem.ProjectPath
	}
	return ""
}

// GetWorkingDir returns the working directory, or empty string if no filesystem context.
func (s *Session) GetWorkingDir() string {
	if s.Filesystem != nil {
		return s.Filesystem.WorkingDir
	}
	return ""
}

// GetCategory returns the UI category, or empty string if no UI preferences.
func (s *Session) GetCategory() string {
	if s.UI != nil {
		return s.UI.Category
	}
	return ""
}

// GetTags returns the UI tags, or empty slice if no UI preferences.
func (s *Session) GetTags() []string {
	if s.UI != nil && s.UI.Tags != nil {
		return s.UI.Tags
	}
	return []string{}
}

// GetTmuxSessionName returns the tmux session name, or empty string if no terminal context.
func (s *Session) GetTmuxSessionName() string {
	if s.Terminal != nil {
		return s.Terminal.TmuxSessionName
	}
	return ""
}

// GetTerminalDimensions returns the terminal width and height, or 0,0 if no terminal context.
func (s *Session) GetTerminalDimensions() (width, height int) {
	if s.Terminal != nil {
		return s.Terminal.Width, s.Terminal.Height
	}
	return 0, 0
}

// GetLastMeaningfulOutput returns when the session had meaningful output,
// or zero time if no activity tracking.
func (s *Session) GetLastMeaningfulOutput() time.Time {
	if s.Activity != nil {
		return s.Activity.LastMeaningfulOutput
	}
	return time.Time{}
}

// GetLastViewed returns when the session was last viewed,
// or zero time if no activity tracking.
func (s *Session) GetLastViewed() time.Time {
	if s.Activity != nil {
		return s.Activity.LastViewed
	}
	return time.Time{}
}

// NeedsReviewQueueAttention returns true if session has unacknowledged output.
func (s *Session) NeedsReviewQueueAttention() bool {
	if s.Activity == nil {
		return false
	}
	return s.Activity.LastMeaningfulOutput.After(s.Activity.LastAcknowledged)
}

// IsCloudConfigured returns true if the cloud context is properly configured.
func (s *Session) IsCloudConfigured() bool {
	return s.Cloud != nil && s.Cloud.IsConfigured()
}

// ================================
// Instance <-> Session Adapters
// ================================

// InstanceToSession converts a legacy Instance to the new Session type.
// This adapter enables gradual migration while maintaining backward compatibility.
// It populates all relevant contexts from the Instance fields.
func InstanceToSession(i *Instance) *Session {
	if i == nil {
		return nil
	}

	s := &Session{
		ID:        i.Title, // Use title as ID for now (matches existing behavior)
		Title:     i.Title,
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
		Status:    i.Status,
		Program:   i.Program,
		AutoYes:   i.AutoYes,
		Prompt:    i.Prompt,
	}

	// Populate Git context if relevant fields are set
	if i.Branch != "" || i.GitHubPRNumber > 0 || i.GitHubOwner != "" {
		s.Git = &GitContext{
			Branch:    i.Branch,
			PRNumber:  i.GitHubPRNumber,
			PRURL:     i.GitHubPRURL,
			Owner:     i.GitHubOwner,
			Repo:      i.GitHubRepo,
			SourceRef: i.GitHubSourceRef,
		}
	}

	// Populate Filesystem context if path is set
	if i.Path != "" {
		s.Filesystem = &FilesystemContext{
			ProjectPath:      i.Path,
			WorkingDir:       i.WorkingDir,
			IsWorktree:       i.IsWorktree,
			MainRepoPath:     i.MainRepoPath,
			ClonedRepoPath:   i.ClonedRepoPath,
			ExistingWorktree: i.ExistingWorktree,
			SessionType:      i.SessionType,
		}
	}

	// Populate Terminal context if dimensions or tmux info is set
	if i.Height > 0 || i.Width > 0 || i.TmuxPrefix != "" {
		s.Terminal = &TerminalContext{
			Height:           i.Height,
			Width:            i.Width,
			TmuxPrefix:       i.TmuxPrefix,
			TmuxServerSocket: i.TmuxServerSocket,
			TerminalType:     "tmux", // Default for existing instances
		}
	}

	// Populate UI preferences if category or tags are set
	if i.Category != "" || len(i.Tags) > 0 {
		s.UI = &UIPreferences{
			Category:   i.Category,
			IsExpanded: i.IsExpanded,
			Tags:       i.Tags,
		}
	}

	// Populate Activity tracking if any timestamps are set
	if !i.LastTerminalUpdate.IsZero() || !i.LastMeaningfulOutput.IsZero() ||
		!i.LastViewed.IsZero() || !i.LastAcknowledged.IsZero() {
		s.Activity = &ActivityTracking{
			LastTerminalUpdate:   i.LastTerminalUpdate,
			LastMeaningfulOutput: i.LastMeaningfulOutput,
			LastViewed:           i.LastViewed,
			LastAcknowledged:     i.LastAcknowledged,
			LastOutputSignature:  i.LastOutputSignature,
			LastAddedToQueue:     i.LastAddedToQueue,
		}
	}

	// Cloud context is not populated from Instance (new feature)
	// s.Cloud remains nil

	return s
}

// SessionToInstance converts a Session back to the legacy Instance type.
// This adapter enables interoperability during the migration period.
// Note: Some Session features (like CloudContext) don't have Instance equivalents.
func SessionToInstance(s *Session) *Instance {
	if s == nil {
		return nil
	}

	i := &Instance{
		Title:     s.Title,
		Status:    s.Status,
		Program:   s.Program,
		AutoYes:   s.AutoYes,
		Prompt:    s.Prompt,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}

	// Extract Git context
	if s.Git != nil {
		i.Branch = s.Git.Branch
		i.GitHubPRNumber = s.Git.PRNumber
		i.GitHubPRURL = s.Git.PRURL
		i.GitHubOwner = s.Git.Owner
		i.GitHubRepo = s.Git.Repo
		i.GitHubSourceRef = s.Git.SourceRef
	}

	// Extract Filesystem context
	if s.Filesystem != nil {
		i.Path = s.Filesystem.ProjectPath
		i.WorkingDir = s.Filesystem.WorkingDir
		i.IsWorktree = s.Filesystem.IsWorktree
		i.MainRepoPath = s.Filesystem.MainRepoPath
		i.ClonedRepoPath = s.Filesystem.ClonedRepoPath
		i.ExistingWorktree = s.Filesystem.ExistingWorktree
		i.SessionType = s.Filesystem.SessionType
	}

	// Extract Terminal context
	if s.Terminal != nil {
		i.Height = s.Terminal.Height
		i.Width = s.Terminal.Width
		i.TmuxPrefix = s.Terminal.TmuxPrefix
		i.TmuxServerSocket = s.Terminal.TmuxServerSocket
	}

	// Extract UI preferences
	if s.UI != nil {
		i.Category = s.UI.Category
		i.IsExpanded = s.UI.IsExpanded
		i.Tags = s.UI.Tags
	}

	// Extract Activity tracking
	if s.Activity != nil {
		i.LastTerminalUpdate = s.Activity.LastTerminalUpdate
		i.LastMeaningfulOutput = s.Activity.LastMeaningfulOutput
		i.LastViewed = s.Activity.LastViewed
		i.LastAcknowledged = s.Activity.LastAcknowledged
		i.LastOutputSignature = s.Activity.LastOutputSignature
		i.LastAddedToQueue = s.Activity.LastAddedToQueue
	}

	// CloudContext has no Instance equivalent - data is lost in conversion
	// This is acceptable during migration; cloud sessions should use Session directly

	return i
}

// ToSession converts this Instance to the new Session type.
// This is a convenience method that wraps InstanceToSession.
func (i *Instance) ToSession() *Session {
	return InstanceToSession(i)
}
