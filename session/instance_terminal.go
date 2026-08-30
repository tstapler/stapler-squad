package session

// instance_terminal.go contains terminal content preview methods, identity/title methods,
// GitHub metadata delegation, permissions, and other display-oriented Instance methods.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/git"
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

// GetProgram returns the program this instance runs (e.g. "claude", "aider").
//
// Reads via Snapshot(), not a direct i.Program field access: actor commands
// (SetProgram/SwitchProgram and friends) write i.Program directly outside
// i.mu, publishing the change only by atomically storing a fresh snapshot at
// the end of the mutation — the same reasoning as GetStatus's doc comment.
// ClaudeController.Start reads this from a different goroutine than whatever
// last set it, so only the atomic snapshot read is synchronized with that
// write; a direct field read or an i.mu-guarded read would not be.
func (i *Instance) GetProgram() string {
	return i.Snapshot().Program
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
	if i.started.Load() {
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
	i.mu.Lock()
	defer i.mu.Unlock()

	// Update the title
	oldTitle := i.Title
	i.Title = newTitle
	i.UpdatedAt = time.Now()
	i.snapshot.Store(buildSnapshot(i))

	log.Info("renamed session", "from", oldTitle, "to", newTitle)
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

// previewBlocked reports whether preview capture should be skipped for this instance -
// either because it hasn't started yet, or because its lifecycle status makes a live
// terminal capture meaningless (Paused/Stopped/Hibernated).
//
// Reads Status via Snapshot(), not a bare field read: actor commands
// (transitionToLocked et al.) write i.Status directly while running inside
// the actor's own serialization, not under i.mu, so an unguarded read here
// doesn't synchronize with that write at all (see GetStatus's doc comment).
// Preview() and PreviewFullHistory() share this check so the two can't drift.
func (i *Instance) previewBlocked() bool {
	status := i.Snapshot().Status
	return !i.started.Load() || status == Paused || status == Stopped || status == Hibernated
}

// Preview returns the current visible terminal content.
// Prefers tmux's own capture-pane (authoritative rendered screen) for tmux-backed
// instances; falls back to the in-memory PTY buffer from ClaudeController otherwise.
//
// Its underlying capture-pane subprocess runs with context.Background() and
// cannot be cancelled. Driver-loop callers (session_driver.go) MUST use
// PreviewContext(ctx) with the loop's own cancellable ctx instead — an
// in-flight Preview() call here previously blocked the driver goroutine from
// ever reaching its `defer cancel()` on stop, leaking the capture-pane
// subprocess across StopSessionDriver/Destroy().
func (i *Instance) Preview() (string, error) {
	if i.previewBlocked() {
		return "", nil
	}

	// tmux performs real terminal emulation (cursor movement, redraws, screen
	// clears), so capture-pane returns the actual current screen rather than an
	// approximation reconstructed from a raw byte stream. CapturePaneContent has
	// a 1s TTL cache (see TmuxProcessManager), so preferring it here does not add
	// a subprocess spawn on every poll tick.
	if tb, ok := i.processManager.(*TmuxBackend); ok {
		if content, err := tb.CapturePaneContent(); err == nil {
			return content, nil
		}
	}

	// Native backend (capture-pane not yet implemented there) or tmux capture
	// failed: fall back to the in-memory PTY buffer.
	// GetRecentOutput(0) would return the ENTIRE 10MB session-lifetime buffer
	// (PTYAccess.GetRecentOutput treats n<=0 as "give me everything"), not the
	// current screen — a dialog answered minutes ago would still scan as
	// "visible" to callers like isStartupDialog/shouldApprovePrompt for as
	// long as it takes 10MB of subsequent output to evict it. Bound the read
	// to the same tail window the working status-detection path already uses.
	if ctrl := i.GetController(); ctrl != nil {
		raw := ctrl.GetRecentOutput(detection.StatusDetectionTailBytes)
		return string(raw), nil
	}

	// No controller at all (e.g. external/attached sessions): try capture-pane
	// directly. Skip the TmuxAlive() pre-check (which spawns a subprocess); let
	// CapturePaneContent handle the "session doesn't exist" case via its own error path.
	content, err := i.pm().CapturePaneContent()
	if err != nil {
		return "", nil
	}

	return content, nil
}

// PreviewContext is Preview with an external context threaded onto the
// underlying tmux subprocess call itself (not just the exec-gate wait), so a
// caller that cancels ctx can kill an already-running capture-pane process
// rather than only giving up on waiting for it. Used by the SessionDriver
// polling loop (session_driver.go), whose stop channel needs to interrupt a
// capture already in flight so StopSessionDriver's bounded join can't stall
// on a stuck subprocess — see runGatedWith's doc comment in
// session/tmux/exec_gate.go for why the gate's own timeout doesn't cover
// this. Falls back to the plain, non-cancellable Preview() for backends that
// don't expose a context-aware capture (native backend, no active tmux
// session, or no controller).
func (i *Instance) PreviewContext(ctx context.Context) (string, error) {
	if i.previewBlocked() {
		return "", nil
	}

	if tb, ok := i.processManager.(*TmuxBackend); ok {
		if tpm, ok := tb.mgr.(*TmuxProcessManager); ok {
			content, err := tpm.CapturePaneContentContext(ctx)
			if err == nil {
				return content, nil
			}
			// A cancellation/deadline means the caller no longer wants this
			// result — falling back to the non-cancellable Preview() here
			// would silently ignore that and block on a fresh subprocess
			// call anyway, defeating the whole point of threading ctx
			// through in the first place (see doc comment above). Only
			// fall back for genuine capture failures (e.g. subprocess
			// error, no pane).
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", ctx.Err()
			}
		}
	}

	return i.Preview()
}

// PreviewFullHistory captures the entire tmux pane output including full scrollback history.
func (i *Instance) PreviewFullHistory() (string, error) {
	if i.previewBlocked() {
		return "", nil
	}

	// Check if the tmux session is still alive before trying to capture content
	if !i.TmuxAlive() {
		return "", nil
	}

	content, err := i.pm().CapturePaneContentWithOptions("-", "-")
	if err != nil {
		return "", err
	}

	return content, nil
}

// CaptureCurrentState records the pane's current working directory into WorkingDir.
// Called during graceful shutdown so cold restore can restart in the right directory.
// No-op if the session is not started, paused, or the tmux session is dead.
//
// For a worktree session, a captured path outside the worktree is refused rather
// than persisted (BUG-033): live-confirmed on an autonomous backlog session whose
// own isolated worktree was created successfully and never touched, while its agent
// ran real work — two feature commits and a branch checkout — directly in the
// shared parent repo checkout instead, apparently after `cd`-ing there mid-task.
// resolveStartPath already had a read-side backstop against a stale out-of-worktree
// WorkingDir, but that guard only fires when i.gitManager.HasWorktree() happens to
// already be true at the moment a session (re)starts — not guaranteed on every
// restart ordering — so a bad path could still be captured here, persisted, and
// later used unguarded. Gating the write itself closes the gap at its source
// instead of only defending against it on read.
func (i *Instance) CaptureCurrentState() error {
	if !i.started.Load() || i.Paused() {
		return nil
	}
	if !i.pm().IsAlive() {
		return nil
	}
	path, err := func() (string, error) {
		tb, ok := i.processManager.(*TmuxBackend)
		if !ok {
			return "", nil
		}
		sess := tb.TmuxManager().Session()
		if sess == nil {
			return "", nil
		}
		return sess.GetPaneCurrentPath()
	}()
	if err != nil {
		return fmt.Errorf("CaptureCurrentState '%s': %w", i.Title, err)
	}
	if path == "" {
		return nil
	}
	if i.gitManager.HasWorktree() {
		if worktreePath := i.gitManager.GetWorktreePath(); worktreePath != "" && pathEscapesRoot(worktreePath, path) {
			log.Warn("refusing to persist working dir outside worktree", "session", i.Title, "path", path, "worktree", worktreePath)
			return nil
		}
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	i.WorkingDir = path
	i.snapshot.Store(buildSnapshot(i))
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
	case Active:
		return "●"
	case Creating:
		return "⏳"
	case Paused:
		return "⏸"
	case Hibernated:
		return "❄"
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

// prUpdateResult is returned by UpdatePRStatus.
type prUpdateResult struct {
	PriorityChanged        bool
	CheckConclusionChanged bool
}

// CurrentBranch returns the branch the session is currently on.
// For worktree sessions, it returns the stored Branch field (set at creation and on worktree
// changes). For directory sessions, Branch is never stored, so it reads the branch live
// from the working directory via git. Returns "" if the branch cannot be determined.
func (i *Instance) CurrentBranch() string {
	if i.Branch != "" {
		return i.Branch
	}
	workDir := i.GetWorkingDirectory()
	if workDir == "" {
		return ""
	}
	branch, err := git.GetCurrentBranchName(workDir)
	if err != nil {
		log.Debug("CurrentBranch: could not read branch from git", "session", i.Title, "err", err)
		return ""
	}
	return branch
}

// PRStatusUpdate bundles the fields PRStatusPoller writes to an Instance on each
// successful fetch. Introduced when itemized checks/review feedback/mergeable were
// added — the prior 7-positional-parameter signature was already at the limit this
// repo's primitive-obsession checklist flags.
type PRStatusUpdate struct {
	State           string
	Priority        string
	CheckConclusion string
	Mergeable       string
	ApprovedCount   int
	ChangesReqCount int
	IsDraft         bool
	Terminal        bool
	Checks          []github.CheckItem
	Reviews         []github.ReviewItem
}

// UpdatePRStatus atomically updates the PR status fields on this instance.
// Called by PRStatusPoller on each successful fetch.
// Returns prUpdateResult indicating whether the priority changed.
func (i *Instance) UpdatePRStatus(update PRStatusUpdate) prUpdateResult {
	var result prUpdateResult
	_ = i.sendSyncErr(func(s *instanceState) error {
		inst := s.inst
		// i.mu guards the writes + buildSnapshot together: legacy setters
		// (MarkViewed & co.) mutate other fields directly under i.mu.Lock() from
		// outside the actor — see runActor's doc comment in actor.go.
		inst.mu.Lock()
		result.PriorityChanged = update.Priority != inst.GitHubPRPriority
		result.CheckConclusionChanged = update.CheckConclusion != inst.GitHubCheckConclusion
		inst.GitHubPRState = update.State
		inst.GitHubPRPriority = update.Priority
		inst.GitHubPRIsDraft = update.IsDraft
		inst.GitHubApprovedCount = update.ApprovedCount
		inst.GitHubChangesReqCount = update.ChangesReqCount
		inst.GitHubCheckConclusion = update.CheckConclusion
		inst.GitHubPRStatusTerminal = update.Terminal
		inst.GitHubChecks = update.Checks
		inst.GitHubReviewFeedback = update.Reviews
		inst.GitHubMergeable = update.Mergeable
		inst.LastPRStatusCheck = time.Now()
		snap := buildSnapshot(inst)
		inst.mu.Unlock()
		inst.snapshot.Store(snap)
		return nil
	})
	return result
}
