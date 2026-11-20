package instancestore

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/claudemetadata"
	"github.com/tstapler/stapler-squad/session/ent/claudesession"
	"github.com/tstapler/stapler-squad/session/ent/diffstats"
	"github.com/tstapler/stapler-squad/session/ent/project"
	"github.com/tstapler/stapler-squad/session/ent/session"
	"github.com/tstapler/stapler-squad/session/ent/tag"
	"github.com/tstapler/stapler-squad/session/ent/worktree"
)

// Update modifies an existing session in the database
func (i *Instances) Update(ctx context.Context, data domain.InstanceData) error {
	return i.UpdateSkipping(ctx, data, domain.UpdateSkip{})
}

// UpdateSkipping is Update minus the child-table sections the caller knows are
// already persisted unchanged (see session.savedFingerprints).
func (i *Instances) UpdateSkipping(ctx context.Context, data domain.InstanceData, skip domain.UpdateSkip) error {
	// Start transaction
	tx, err := i.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Find session by title
	sess, err := tx.Session.Query().Where(session.Title(data.Title)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Update main session fields
	sessionUpdate := tx.Session.UpdateOne(sess).
		SetNillableUUID(nilIfEmpty(data.UUID)).
		SetPath(data.Path).
		SetStatus(int(data.Status)).
		SetUpdatedAt(data.UpdatedAt).
		SetAutoYes(data.AutoYes).
		SetAutoApprove(data.AutoApprove).
		SetAutonomousMode(data.AutonomousMode).
		SetProgram(data.Program).
		SetIsExpanded(data.IsExpanded).
		SetPinned(data.Pinned)

	setOptionalSessionFields(sessionUpdate, data)

	// Update project link (look up by name or clear if empty)
	if data.ProjectID != "" {
		proj, err := tx.Project.Query().Where(project.Name(data.ProjectID)).Only(ctx)
		if err == nil {
			sessionUpdate.SetProjectID(proj.ID)
		}
	} else {
		sessionUpdate.ClearProject()
	}

	if err := sessionUpdate.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update session: %w", err)
	}

	if !skip.Worktree {
		if err := i.updateWorktreeRow(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if !skip.DiffStats {
		if err := i.upsertDiffStatsRow(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if !skip.Tags {
		if err := i.replaceSessionTags(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if data.ClaudeSession.ConversationUUID != "" && !skip.Claude {
		if err := i.upsertClaudeSessionRow(ctx, tx, sess, data); err != nil {
			return err
		}
	}

	if err := i.ports.UpsertSessionPRSet(ctx, tx, data); err != nil {
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// setOptionalSessionFields applies the session columns Update only sets when
// data carries a value (plus the few that are cleared when empty).
func setOptionalSessionFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	setSessionTextFields(sessionUpdate, data)
	setSessionTimeFields(sessionUpdate, data)
	setSessionLifecycleFields(sessionUpdate, data)
}

func setSessionTextFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	// Update optional fields
	if data.WorkingDir != "" {
		sessionUpdate.SetWorkingDir(data.WorkingDir)
	}
	if data.Branch != "" {
		sessionUpdate.SetBranch(data.Branch)
	}
	if data.Height > 0 {
		sessionUpdate.SetHeight(data.Height)
	}
	if data.Width > 0 {
		sessionUpdate.SetWidth(data.Width)
	}
	if data.Prompt != "" {
		sessionUpdate.SetPrompt(data.Prompt)
	}
	if data.ExistingWorktree != "" {
		sessionUpdate.SetExistingWorktree(data.ExistingWorktree)
	}
	if data.Category != "" {
		sessionUpdate.SetCategory(data.Category)
	}
	// Note is set unconditionally, unlike sibling optional string fields: an empty note is a
	// meaningful, intentionally-reachable state ("cleared"), not an "unset" sentinel, so the
	// guarded-update convention would silently prevent a user from ever clearing it.
	sessionUpdate.SetNote(data.Note)
	// RuleTagProvenance/SuppressedRuleTags are set unconditionally too: retracting the last
	// rule-owned tag or un-suppressing the last tag are meaningful "cleared to empty" states
	// (same rationale as Note above), not an "unset" sentinel a guarded update would preserve.
	sessionUpdate.SetRuleTagProvenance(data.RuleTagProvenance)
	sessionUpdate.SetSuppressedRuleTags(SuppressedRuleTagsToSlice(data.SuppressedRuleTags))
	if data.SessionType != "" {
		sessionUpdate.SetSessionType(string(data.SessionType))
	}
	if data.TmuxPrefix != "" {
		sessionUpdate.SetTmuxPrefix(data.TmuxPrefix)
	}
	// See the matching comment in Create above: Backend is write-once per
	// instance today, so this guard is intentionally set-only, never clear.
	if data.Backend != "" {
		sessionUpdate.SetBackend(string(data.Backend))
	}
	// Unlike Backend, this is cleared when empty: a tmux switch or a killed
	// session must not leave a stale id behind. ToInstanceData keeps a loaded,
	// not-yet-restored id (PersistedSessionID), so an unstarted load never clears it.
	sessionUpdate.SetTymuxSessionID(data.TymuxSessionID)
}

func setSessionTimeFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	if !data.LastTerminalUpdate.IsZero() {
		sessionUpdate.SetLastTerminalUpdate(data.LastTerminalUpdate)
	}
	if !data.LastMeaningfulOutput.IsZero() {
		sessionUpdate.SetLastMeaningfulOutput(data.LastMeaningfulOutput)
	}
	if data.LastOutputSignature != "" {
		sessionUpdate.SetLastOutputSignature(data.LastOutputSignature)
	}
	if !data.LastAddedToQueue.IsZero() {
		sessionUpdate.SetLastAddedToQueue(data.LastAddedToQueue)
	}
	if !data.LastViewed.IsZero() {
		sessionUpdate.SetLastViewed(data.LastViewed)
	}
	if !data.LastAcknowledged.IsZero() {
		sessionUpdate.SetLastAcknowledged(data.LastAcknowledged)
	}
	if !data.LastUserResponse.IsZero() {
		sessionUpdate.SetLastUserResponse(data.LastUserResponse)
	}
	if !data.ProcessingGraceUntil.IsZero() {
		sessionUpdate.SetProcessingGraceUntil(data.ProcessingGraceUntil)
	} else {
		sessionUpdate.ClearProcessingGraceUntil()
	}
	if !data.LastPromptDetected.IsZero() {
		sessionUpdate.SetLastPromptDetected(data.LastPromptDetected)
	}
	if data.LastPromptSignature != "" {
		sessionUpdate.SetLastPromptSignature(data.LastPromptSignature)
	}
	if data.MCPServerURL != "" {
		sessionUpdate.SetMcpServerURL(data.MCPServerURL)
	}
	if data.InitialPrompt != "" {
		sessionUpdate.SetInitialPrompt(data.InitialPrompt)
	}
	if !data.InitialPromptSentAt.IsZero() {
		sessionUpdate.SetInitialPromptSentAt(data.InitialPromptSentAt)
	}
}

// setGitHubPRMirrorFields writes the legacy scalar PR columns on a full save.
// A session whose snapshot carries links writes them unconditionally so a detach
// or an open PR can clear the number, URL and terminal flag; a session whose legacy
// number has no link keeps its stored number/URL untouched. A session without links
// keeps the old set-only behavior, so a stale in-memory zero cannot erase data (its
// stale terminal=true is cleared by the immediate title-keyed write, not here).
func setGitHubPRMirrorFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	switch {
	case data.LegacyPRUnrepresented:
		if data.GitHubPRStatusTerminal {
			sessionUpdate.SetGithubPrStatusTerminal(true)
		}
	case data.HasPRLinks:
		sessionUpdate.SetGithubPrURL(data.GitHubPRURL).
			SetGithubPrNumber(data.GitHubPRNumber).
			SetGithubPrStatusTerminal(data.GitHubPRStatusTerminal)
	default:
		if data.GitHubPRURL != "" {
			sessionUpdate.SetGithubPrURL(data.GitHubPRURL)
		}
		if data.GitHubPRNumber > 0 {
			sessionUpdate.SetGithubPrNumber(data.GitHubPRNumber)
		}
		if data.GitHubPRStatusTerminal {
			sessionUpdate.SetGithubPrStatusTerminal(true)
		} else if data.TerminalClearPending {
			sessionUpdate.SetGithubPrStatusTerminal(false)
		}
	}
}

func setSessionLifecycleFields(sessionUpdate *ent.SessionUpdateOne, data domain.InstanceData) {
	if data.PauseReason != "" {
		sessionUpdate.SetPauseReason(data.PauseReason)
	} else {
		sessionUpdate.ClearPauseReason()
	}
	if data.ExitReason != "" {
		sessionUpdate.SetExitReason(data.ExitReason)
	} else {
		sessionUpdate.ClearExitReason()
	}
	sessionUpdate.SetOneShot(data.OneShot)
	sessionUpdate.SetHidden(data.Hidden)
	if data.WorkflowID != "" {
		sessionUpdate.SetWorkflowID(data.WorkflowID)
	} else {
		sessionUpdate.ClearWorkflowID()
	}
	if data.ArchivedAt != nil {
		sessionUpdate.SetArchivedAt(*data.ArchivedAt)
	} else {
		sessionUpdate.ClearArchivedAt()
	}
	if !data.CreationProgressUpdatedAt.IsZero() {
		sessionUpdate.SetCreationProgressUpdatedAt(data.CreationProgressUpdatedAt)
	}
	setGitHubPRMirrorFields(sessionUpdate, data)
	if data.GitHubOwner != "" {
		sessionUpdate.SetGithubOwner(data.GitHubOwner)
	}
	if data.GitHubRepo != "" {
		sessionUpdate.SetGithubRepo(data.GitHubRepo)
	}
	if data.GitHubHost != "" {
		sessionUpdate.SetGithubHost(data.GitHubHost)
	}
}

// updateWorktreeRow upserts the session's worktree row.
func (i *Instances) updateWorktreeRow(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	// Update or create worktree
	if data.Worktree.RepoPath != "" {
		existingWorktree, err := tx.Worktree.Query().Where(worktree.HasSessionWith(session.ID(sess.ID))).Only(ctx)
		if err != nil {
			if ent.IsNotFound(err) {
				// Create new worktree
				if _, err := tx.Worktree.Create().
					SetSessionID(sess.ID).
					SetRepoPath(data.Worktree.RepoPath).
					SetWorktreePath(data.Worktree.WorktreePath).
					SetSessionName(data.Worktree.SessionName).
					SetBranchName(data.Worktree.BranchName).
					SetBaseCommitSha(data.Worktree.BaseCommitSHA).
					Save(ctx); err != nil {
					return fmt.Errorf("failed to create worktree: %w", err)
				}
			} else {
				return fmt.Errorf("failed to query worktree: %w", err)
			}
		} else {
			// Update existing worktree
			if err := tx.Worktree.UpdateOne(existingWorktree).
				SetRepoPath(data.Worktree.RepoPath).
				SetWorktreePath(data.Worktree.WorktreePath).
				SetSessionName(data.Worktree.SessionName).
				SetBranchName(data.Worktree.BranchName).
				SetBaseCommitSha(data.Worktree.BaseCommitSHA).
				Exec(ctx); err != nil {
				return fmt.Errorf("failed to update worktree: %w", err)
			}
		}
	}
	return nil
}

// upsertDiffStatsRow upserts the session's diff_stats row.
func (i *Instances) upsertDiffStatsRow(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	// Upsert diff stats — single INSERT … ON CONFLICT DO UPDATE, no prior SELECT needed.
	diffCreate := tx.DiffStats.Create().
		SetSessionID(sess.ID).
		SetAdded(data.DiffStats.Added).
		SetRemoved(data.DiffStats.Removed)
	if data.DiffStats.Content != "" {
		diffCreate = diffCreate.SetContent(data.DiffStats.Content)
	}
	upsertDiff := diffCreate.
		OnConflictColumns(diffstats.SessionColumn).
		UpdateAdded().
		UpdateRemoved()
	if data.DiffStats.Content != "" {
		upsertDiff = upsertDiff.UpdateContent()
	}
	if err := upsertDiff.Exec(ctx); err != nil {
		return fmt.Errorf("failed to upsert diff stats: %w", err)
	}
	return nil
}

// replaceSessionTags replaces the session's tag associations with data.Tags.
func (i *Instances) replaceSessionTags(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	// Update tags - clear existing and add new ones
	if err := tx.Session.UpdateOne(sess).ClearTags().Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}

	if len(data.Tags) > 0 {
		tagIDs := make([]int, 0, len(data.Tags))
		for _, tagName := range data.Tags {
			t, err := tx.Tag.Query().Where(tag.Name(tagName)).Only(ctx)
			if err != nil {
				if ent.IsNotFound(err) {
					t, err = tx.Tag.Create().SetName(tagName).Save(ctx)
					if err != nil {
						return fmt.Errorf("failed to create tag %s: %w", tagName, err)
					}
				} else {
					return fmt.Errorf("failed to query tag %s: %w", tagName, err)
				}
			}
			tagIDs = append(tagIDs, t.ID)
		}
		if err := tx.Session.UpdateOne(sess).AddTagIDs(tagIDs...).Exec(ctx); err != nil {
			return fmt.Errorf("failed to associate tags: %w", err)
		}
	}
	return nil
}

// upsertClaudeSessionRow creates or updates the Claude session row and its metadata.
func (i *Instances) upsertClaudeSessionRow(ctx context.Context, tx *ent.Tx, sess *ent.Session, data domain.InstanceData) error {
	if data.ClaudeSession.ConversationUUID == "" {
		return nil
	}
	var claudeSessionID int
	existingClaude, err := tx.ClaudeSession.Query().Where(claudesession.HasSessionWith(session.ID(sess.ID))).Only(ctx)
	switch {
	case err == nil:
		if err := updateClaudeSessionRow(ctx, tx, existingClaude, data); err != nil {
			return err
		}
		claudeSessionID = existingClaude.ID
	case ent.IsNotFound(err):
		created, err := CreateClaudeSessionRow(ctx, tx, sess.ID, data)
		if err != nil {
			return err
		}
		claudeSessionID = created.ID
	default:
		return fmt.Errorf("failed to query claude session: %w", err)
	}

	// Add metadata entries using the already-known Claude session ID.
	// (pre-fix: this loop re-queried the same ClaudeSession row N times)
	for key, value := range data.ClaudeSession.Metadata {
		if _, err := tx.ClaudeMetadata.Create().
			SetClaudeSessionID(claudeSessionID).
			SetKey(key).
			SetValue(value).
			Save(ctx); err != nil {
			return fmt.Errorf("failed to create claude metadata %s: %w", key, err)
		}
	}
	return nil
}

// updateClaudeSessionRow updates the existing row and clears its metadata so
// the caller can re-add the current entries.
func updateClaudeSessionRow(ctx context.Context, tx *ent.Tx, existing *ent.ClaudeSession, data domain.InstanceData) error {
	claudeUpdate := tx.ClaudeSession.UpdateOne(existing).
		SetClaudeSessionID(data.ClaudeSession.ConversationUUID).
		SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
		SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
		SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
		SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)
	if data.ClaudeSession.SquadSessionID != "" {
		claudeUpdate.SetConversationID(data.ClaudeSession.SquadSessionID)
	}
	if data.ClaudeSession.ProjectName != "" {
		claudeUpdate.SetProjectName(data.ClaudeSession.ProjectName)
	}
	if !data.ClaudeSession.LastAttached.IsZero() {
		claudeUpdate.SetLastAttached(data.ClaudeSession.LastAttached)
	}
	if data.ClaudeSession.Settings.PreferredSessionName != "" {
		claudeUpdate.SetPreferredSessionName(data.ClaudeSession.Settings.PreferredSessionName)
	}
	if err := claudeUpdate.Exec(ctx); err != nil {
		return fmt.Errorf("failed to update claude session: %w", err)
	}
	if _, err := tx.ClaudeMetadata.Delete().Where(claudemetadata.HasClaudeSessionWith(claudesession.ID(existing.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear claude metadata: %w", err)
	}
	return nil
}
