package session

// instance_terminal.go contains terminal content preview methods, identity/title methods,
// GitHub metadata delegation, permissions, and other display-oriented Instance methods.

import (
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// GetCreatedAt returns the time this instance was created. The field is immutable after creation.
func (i *Instance) GetCreatedAt() time.Time {
	return i.CreatedAt
}

// GetTitle returns the session title/name.
func (i *Instance) GetTitle() string {
	return i.Title
}

// GetStableID returns a stable identifier for this instance.
// If UUID is set, returns it. Falls back to Title for backward compatibility
// with sessions that pre-date UUID assignment.
func (i *Instance) GetStableID() string {
	if i.UUID != "" {
		return i.UUID
	}
	return i.Title
}

// MatchesID reports whether id refers to this instance.
// Accepts the stable UUID, the legacy Title, or the full tmux session name
// (e.g. "staplersquad_my-session") so that hook notifications sent from inside
// managed tmux sessions are correctly attributed to their human-readable session.
func (i *Instance) MatchesID(id string) bool {
	if i.Title == id || i.GetStableID() == id {
		return true
	}
	if tmuxName := i.GetTmuxSessionName(); tmuxName != "" && tmuxName == id {
		return true
	}
	return false
}

// SetTitle sets the title of the instance. Returns an error if the instance has started.
// We can't change the title once it's been used for a tmux session etc.
func (i *Instance) SetTitle(title string) error {
	if i.started {
		return fmt.Errorf("cannot change title of a started instance")
	}
	i.Title = title
	return nil
}

// Rename renames this session. Validates title constraints and updates UpdatedAt.
func (i *Instance) Rename(newTitle string) error {
	// Validate title length
	if len(newTitle) < MinTitleLength || len(newTitle) > MaxTitleLength {
		return ErrInvalidTitleLength
	}

	// Validate title characters
	if !isValidTitle(newTitle) {
		return ErrInvalidTitleChars
	}

	if newTitle == i.Title {
		// No change needed
		return nil
	}

	// Use mutex for thread safety
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()

	// Update the title
	oldTitle := i.Title
	i.Title = newTitle
	i.UpdatedAt = time.Now()

	log.InfoLog.Printf("Renamed session from '%s' to '%s'", oldTitle, newTitle)
	return nil
}

// combineErrors combines multiple errors into a single error.
func (i *Instance) combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}

	errMsg := "multiple cleanup errors occurred:"
	for _, err := range errs {
		errMsg += "\n  - " + err.Error()
	}
	return fmt.Errorf("%s", errMsg)
}

// Preview returns the current visible terminal content.
// Prefers the in-memory PTY buffer from ClaudeController; falls back to capture-pane.
func (i *Instance) Preview() (string, error) {
	if !i.started || i.Status == Paused || i.Status == Stopped {
		return "", nil
	}

	// Prefer the in-memory PTY buffer from ClaudeController (no subprocess).
	if ctrl := i.GetController(); ctrl != nil {
		raw := ctrl.GetRecentOutput(0)
		return string(raw), nil
	}

	// Fallback for external/attached sessions: use capture-pane subprocess.
	if !i.TmuxAlive() {
		return "", nil
	}

	content, err := i.tmuxManager.CapturePaneContent()
	if err != nil {
		return "", err
	}

	return content, nil
}

// PreviewFullHistory captures the entire tmux pane output including full scrollback history.
func (i *Instance) PreviewFullHistory() (string, error) {
	if !i.started || i.Status == Paused || i.Status == Stopped {
		return "", nil
	}

	// Check if the tmux session is still alive before trying to capture content
	if !i.TmuxAlive() {
		return "", nil
	}

	content, err := i.tmuxManager.CapturePaneContentWithOptions("-", "-")
	if err != nil {
		return "", err
	}

	return content, nil
}

// CaptureCurrentState records the pane's current working directory into WorkingDir.
// Called during graceful shutdown so cold restore can restart in the right directory.
// No-op if the session is not started, paused, or the tmux session is dead.
func (i *Instance) CaptureCurrentState() error {
	if !i.started || i.Paused() {
		return nil
	}
	if !i.tmuxManager.DoesSessionExist() {
		return nil
	}
	tmuxSession := i.tmuxManager.Session()
	if tmuxSession == nil {
		return nil
	}
	path, err := tmuxSession.GetPaneCurrentPath()
	if err != nil {
		return fmt.Errorf("CaptureCurrentState '%s': %w", i.Title, err)
	}
	if path == "" {
		return nil
	}
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.WorkingDir = path
	return nil
}

// GetPermissions returns the permissions for this instance based on its type.
func (i *Instance) GetPermissions() InstancePermissions {
	if i.IsManaged {
		return GetManagedPermissions()
	}

	// External instance - permissions depend on discovery configuration
	// For now, we'll use a conservative default (view-only)
	// TODO: This should be configurable via PTYDiscoveryConfig
	return GetExternalPermissions(false)
}

// GetStatusIconForType returns the appropriate status icon based on instance type.
func (i *Instance) GetStatusIconForType() string {
	if !i.IsManaged {
		return "👁" // Eye icon for external/view-only instances
	}

	// Managed instance - use standard status icons
	switch i.Status {
	case Running:
		return "●"
	case Ready:
		return "○"
	case Paused:
		return "⏸"
	case Loading:
		return "⏳"
	case NeedsApproval:
		return "❓"
	default:
		return "?"
	}
}

// ---- GitHub Metadata Delegation ------------------------------------------------
// The following methods delegate to GitHubMetadataView value object.
// The 6 GitHub fields remain on Instance for backward compatibility with
// instance_adapter.go and serialization (ToInstanceData/FromInstanceData).

// GitHub returns a read-only view of the GitHub metadata for this instance.
func (i *Instance) GitHub() GitHubMetadataView {
	return GitHubMetadataView{
		PRNumber:       i.GitHubPRNumber,
		PRURL:          i.GitHubPRURL,
		Owner:          i.GitHubOwner,
		Repo:           i.GitHubRepo,
		SourceRef:      i.GitHubSourceRef,
		ClonedRepoPath: i.ClonedRepoPath,
	}
}

// IsPRSession returns true if this session was created from a GitHub PR URL.
// Delegates to GitHubMetadataView.IsPRSession.
func (i *Instance) IsPRSession() bool { return i.GitHub().IsPRSession() }

// GetGitHubRepoFullName returns "owner/repo" format, or empty string.
// Delegates to GitHubMetadataView.RepoFullName.
func (i *Instance) GetGitHubRepoFullName() string { return i.GitHub().RepoFullName() }

// GetPRDisplayInfo returns a human-readable PR description for UI display.
// Delegates to GitHubMetadataView.PRDisplayInfo.
func (i *Instance) GetPRDisplayInfo() string { return i.GitHub().PRDisplayInfo() }

// IsGitHubSession returns true if this session has GitHub owner and repo set.
// Delegates to GitHubMetadataView.IsGitHubSession.
func (i *Instance) IsGitHubSession() bool { return i.GitHub().IsGitHubSession() }

// UpdatePRStatus atomically updates the PR status fields on this instance.
// Called by PRStatusPoller on each successful fetch.
func (i *Instance) UpdatePRStatus(state, priority, checkConclusion string, approvedCount, changesReqCount int, isDraft, terminal bool) {
	i.stateMutex.Lock()
	defer i.stateMutex.Unlock()
	i.GitHubPRState = state
	i.GitHubPRPriority = priority
	i.GitHubPRIsDraft = isDraft
	i.GitHubApprovedCount = approvedCount
	i.GitHubChangesReqCount = changesReqCount
	i.GitHubCheckConclusion = checkConclusion
	i.GitHubPRStatusTerminal = terminal
	i.LastPRStatusCheck = time.Now()
}
