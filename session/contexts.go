package session

import (
	"time"
)

// GitContext represents the Git-related context for a session.
// This includes repository information, branch details, and GitHub PR integration.
type GitContext struct {
	// Branch is the current git branch name
	Branch string `json:"branch,omitempty"`

	// BaseCommitSHA is the commit SHA where this branch diverged from main/master
	BaseCommitSHA string `json:"base_commit_sha,omitempty"`

	// WorktreeID is a foreign key to the worktrees table (nil if no worktree)
	WorktreeID *int64 `json:"worktree_id,omitempty"`

	// GitHub PR integration fields (all optional within this context)

	// PRNumber is the pull request number
	PRNumber int `json:"pr_number,omitempty"`

	// PRURL is the full URL to the pull request
	PRURL string `json:"pr_url,omitempty"`

	// Owner is the GitHub repository owner/organization
	Owner string `json:"owner,omitempty"`

	// Repo is the GitHub repository name
	Repo string `json:"repo,omitempty"`

	// SourceRef is the source branch reference for the PR
	SourceRef string `json:"source_ref,omitempty"`
}

// IsEmpty returns true if the GitContext has no meaningful data
func (g *GitContext) IsEmpty() bool {
	return g.Branch == "" &&
		g.BaseCommitSHA == "" &&
		g.WorktreeID == nil &&
		g.PRNumber == 0 &&
		g.PRURL == ""
}

// FilesystemContext represents the filesystem-related context for a session.
// This includes project paths, working directories, and worktree information.
type FilesystemContext struct {
	// ProjectPath is the root project/repository directory
	ProjectPath string `json:"project_path,omitempty"`

	// WorkingDir is the current working directory within the project
	WorkingDir string `json:"working_dir,omitempty"`

	// IsWorktree indicates if this session is using a git worktree
	IsWorktree bool `json:"is_worktree,omitempty"`

	// MainRepoPath is the parent repository path if this is a worktree
	MainRepoPath string `json:"main_repo_path,omitempty"`

	// ClonedRepoPath is the path to the cloned repository for external PRs
	ClonedRepoPath string `json:"cloned_repo_path,omitempty"`

	// ExistingWorktree is the path to an existing worktree being used
	ExistingWorktree string `json:"existing_worktree,omitempty"`

	// SessionType indicates the type of session workflow
	SessionType SessionType `json:"session_type,omitempty"`
}

// IsEmpty returns true if the FilesystemContext has no meaningful data
func (f *FilesystemContext) IsEmpty() bool {
	return f.ProjectPath == "" &&
		f.WorkingDir == "" &&
		!f.IsWorktree &&
		f.SessionType == ""
}

// TerminalContext represents the terminal-related context for a session.
// This includes terminal dimensions, tmux configuration, and terminal type.
type TerminalContext struct {
	// Height is the terminal height in rows
	Height int `json:"height,omitempty"`

	// Width is the terminal width in columns
	Width int `json:"width,omitempty"`

	// TmuxSessionName is the name of the tmux session
	TmuxSessionName string `json:"tmux_session_name,omitempty"`

	// TmuxPrefix is the prefix used for tmux session naming
	TmuxPrefix string `json:"tmux_prefix,omitempty"`

	// TmuxServerSocket is the path to the tmux server socket
	TmuxServerSocket string `json:"tmux_server_socket,omitempty"`

	// TerminalType indicates the terminal backend type
	// Possible values: "tmux", "mux", "pty", "web"
	TerminalType string `json:"terminal_type,omitempty"`
}

// IsEmpty returns true if the TerminalContext has no meaningful data
func (t *TerminalContext) IsEmpty() bool {
	return t.Height == 0 &&
		t.Width == 0 &&
		t.TmuxSessionName == "" &&
		t.TerminalType == ""
}

// UIPreferences represents the UI-related preferences for a session.
// This includes categorization, tags, and display preferences.
type UIPreferences struct {
	// Category is the organizational category for the session
	Category string `json:"category,omitempty"`

	// IsExpanded indicates if the session is expanded in grouped views
	IsExpanded bool `json:"is_expanded,omitempty"`

	// Tags are the user-defined tags for multi-dimensional organization
	Tags []string `json:"tags,omitempty"`

	// GroupingStrategy is the current grouping mode (e.g., "category", "tag", "branch")
	GroupingStrategy string `json:"grouping_strategy,omitempty"`

	// SortOrder is the preferred sort order (e.g., "name", "date", "status")
	SortOrder string `json:"sort_order,omitempty"`
}

// IsEmpty returns true if the UIPreferences has no meaningful data
func (u *UIPreferences) IsEmpty() bool {
	return u.Category == "" &&
		len(u.Tags) == 0 &&
		u.GroupingStrategy == "" &&
		u.SortOrder == ""
}

// HasTag returns true if the UIPreferences contains the specified tag
func (u *UIPreferences) HasTag(tag string) bool {
	for _, t := range u.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// ActivityTracking represents the activity tracking data for a session.
// This includes timestamps for various events and output tracking.
type ActivityTracking struct {
	// LastTerminalUpdate is when the terminal output was last updated
	LastTerminalUpdate time.Time `json:"last_terminal_update,omitempty"`

	// LastMeaningfulOutput is when meaningful (non-noise) output was detected
	LastMeaningfulOutput time.Time `json:"last_meaningful_output,omitempty"`

	// LastViewed is when the session was last viewed by the user
	LastViewed time.Time `json:"last_viewed,omitempty"`

	// LastAcknowledged is when the user last acknowledged session output
	LastAcknowledged time.Time `json:"last_acknowledged,omitempty"`

	// LastOutputSignature is a hash/signature of the last output for deduplication
	LastOutputSignature string `json:"last_output_signature,omitempty"`

	// LastAddedToQueue is when the session was last added to the review queue
	LastAddedToQueue time.Time `json:"last_added_to_queue,omitempty"`
}

// IsEmpty returns true if the ActivityTracking has no meaningful data
func (a *ActivityTracking) IsEmpty() bool {
	return a.LastTerminalUpdate.IsZero() &&
		a.LastMeaningfulOutput.IsZero() &&
		a.LastViewed.IsZero() &&
		a.LastAcknowledged.IsZero() &&
		a.LastOutputSignature == ""
}

// HasRecentActivity returns true if there has been activity within the specified duration
func (a *ActivityTracking) HasRecentActivity(within time.Duration) bool {
	now := time.Now()
	return now.Sub(a.LastTerminalUpdate) <= within ||
		now.Sub(a.LastMeaningfulOutput) <= within
}

// CloudContext represents the cloud-related context for a session.
// This includes cloud provider details, region, and API configuration.
type CloudContext struct {
	// Provider is the cloud provider name (aws/gcp/azure/custom)
	Provider string `json:"provider,omitempty"`

	// Region is the cloud region/zone
	Region string `json:"region,omitempty"`

	// InstanceID is the cloud instance identifier
	InstanceID string `json:"instance_id,omitempty"`

	// APIEndpoint is the API endpoint URL for the cloud service
	APIEndpoint string `json:"api_endpoint,omitempty"`

	// APIKeyRef is a reference to secure key storage (not the actual key)
	APIKeyRef string `json:"api_key_ref,omitempty"`

	// CloudSessionID is the cloud provider's session identifier
	CloudSessionID string `json:"cloud_session_id,omitempty"`

	// ConversationID is the conversation/thread identifier for AI services
	ConversationID string `json:"conversation_id,omitempty"`
}

// IsEmpty returns true if the CloudContext has no meaningful data
func (c *CloudContext) IsEmpty() bool {
	return c.Provider == "" &&
		c.Region == "" &&
		c.InstanceID == "" &&
		c.APIEndpoint == "" &&
		c.CloudSessionID == "" &&
		c.ConversationID == ""
}

// IsConfigured returns true if the CloudContext has minimum required configuration
func (c *CloudContext) IsConfigured() bool {
	return c.Provider != "" && (c.APIEndpoint != "" || c.Region != "")
}
