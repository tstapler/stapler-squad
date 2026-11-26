package instancestore

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/project"
	"github.com/tstapler/stapler-squad/session/ent/tag"
)

// nilIfEmpty returns nil if s is empty, otherwise a pointer to s.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Create inserts a new session into the database
func (i *Instances) Create(ctx context.Context, data domain.InstanceData) error {
	// Start transaction
	tx, err := i.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Create main session
	sessionCreate := tx.Session.Create().
		SetTitle(data.Title).
		SetNillableUUID(nilIfEmpty(data.UUID)).
		SetPath(data.Path).
		SetStatus(int(data.Status)).
		SetCreatedAt(data.CreatedAt).
		SetUpdatedAt(data.UpdatedAt).
		SetAutoYes(data.AutoYes).
		SetAutoApprove(data.AutoApprove).
		SetAutonomousMode(data.AutonomousMode).
		SetProgram(data.Program).
		SetIsExpanded(data.IsExpanded).
		SetPinned(data.Pinned)

	// Set optional fields
	setSessionCreateIdentityFields(sessionCreate, data)
	setSessionCreateActivityFields(sessionCreate, data)
	setSessionCreateGitHubFields(sessionCreate, data)

	// Link project if specified (look up by name)
	if data.ProjectID != "" {
		proj, err := tx.Project.Query().Where(project.Name(data.ProjectID)).Only(ctx)
		if err == nil {
			sessionCreate.SetProjectID(proj.ID)
		}
		// If project not found, silently continue (session created without project link)
	}

	sess, err := sessionCreate.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	if err := createWorktreeRow(ctx, tx, sess.ID, data); err != nil {
		return err
	}
	if err := createDiffStatsRow(ctx, tx, sess.ID, data); err != nil {
		return err
	}
	if err := createTagLinks(ctx, tx, sess, data.Tags); err != nil {
		return err
	}
	if err := createClaudeRows(ctx, tx, sess.ID, data); err != nil {
		return err
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

func setSessionCreateIdentityFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if data.WorkingDir != "" {
		sessionCreate.SetWorkingDir(data.WorkingDir)
	}
	if data.Branch != "" {
		sessionCreate.SetBranch(data.Branch)
	}
	if data.Height > 0 {
		sessionCreate.SetHeight(data.Height)
	}
	if data.Width > 0 {
		sessionCreate.SetWidth(data.Width)
	}
	if data.Prompt != "" {
		sessionCreate.SetPrompt(data.Prompt)
	}
	if data.ExistingWorktree != "" {
		sessionCreate.SetExistingWorktree(data.ExistingWorktree)
	}
	if data.Category != "" {
		sessionCreate.SetCategory(data.Category)
	}
	if data.Note != "" {
		sessionCreate.SetNote(data.Note)
	}
	if len(data.RuleTagProvenance) > 0 {
		sessionCreate.SetRuleTagProvenance(data.RuleTagProvenance)
	}
	if len(data.SuppressedRuleTags) > 0 {
		sessionCreate.SetSuppressedRuleTags(SuppressedRuleTagsToSlice(data.SuppressedRuleTags))
	}
	if data.SessionType != "" {
		sessionCreate.SetSessionType(string(data.SessionType))
	}
}

func setSessionCreateActivityFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if data.TmuxPrefix != "" {
		sessionCreate.SetTmuxPrefix(data.TmuxPrefix)
	}
	if data.TmuxTitle != "" {
		sessionCreate.SetTmuxTitle(data.TmuxTitle)
	}
	// Backend is write-once per instance today (set at construction via
	// session.ResolveSessionBackend, never reset to "" afterward) -- this guard
	// intentionally only ever sets a non-empty value, mirroring TmuxPrefix above,
	// not clearing an explicit unset like PauseReason/ExitReason do. If a future
	// feature ever needs to unpin an instance's backend back to "", this needs an
	// explicit ClearBackend() branch too, or the clear will silently no-op here.
	if data.Backend != "" {
		sessionCreate.SetBackend(string(data.Backend))
	}
	if data.TymuxSessionID != "" {
		sessionCreate.SetTymuxSessionID(data.TymuxSessionID)
	}
	if !data.LastTerminalUpdate.IsZero() {
		sessionCreate.SetLastTerminalUpdate(data.LastTerminalUpdate)
	}
	if !data.LastMeaningfulOutput.IsZero() {
		sessionCreate.SetLastMeaningfulOutput(data.LastMeaningfulOutput)
	}
	if data.LastOutputSignature != "" {
		sessionCreate.SetLastOutputSignature(data.LastOutputSignature)
	}
	if !data.LastAddedToQueue.IsZero() {
		sessionCreate.SetLastAddedToQueue(data.LastAddedToQueue)
	}
	if !data.LastViewed.IsZero() {
		sessionCreate.SetLastViewed(data.LastViewed)
	}
	if !data.LastAcknowledged.IsZero() {
		sessionCreate.SetLastAcknowledged(data.LastAcknowledged)
	}
	if data.MCPServerURL != "" {
		sessionCreate.SetMcpServerURL(data.MCPServerURL)
	}
	if data.InitialPrompt != "" {
		sessionCreate.SetInitialPrompt(data.InitialPrompt)
	}
}

func setSessionCreateGitHubFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if !data.InitialPromptSentAt.IsZero() {
		sessionCreate.SetInitialPromptSentAt(data.InitialPromptSentAt)
	}
	if data.OneShot {
		sessionCreate.SetOneShot(data.OneShot)
	}
	if data.Hidden {
		sessionCreate.SetHidden(data.Hidden)
	}
	if data.GitHubPRURL != "" {
		sessionCreate.SetGithubPrURL(data.GitHubPRURL)
	}
	if data.GitHubPRNumber > 0 {
		sessionCreate.SetGithubPrNumber(data.GitHubPRNumber)
	}
	if data.GitHubPRStatusTerminal {
		sessionCreate.SetGithubPrStatusTerminal(true)
	}
	if data.GitHubOwner != "" {
		sessionCreate.SetGithubOwner(data.GitHubOwner)
	}
	if data.GitHubRepo != "" {
		sessionCreate.SetGithubRepo(data.GitHubRepo)
	}
	if data.GitHubHost != "" {
		sessionCreate.SetGithubHost(data.GitHubHost)
	}
	if data.ArchivedAt != nil {
		sessionCreate.SetArchivedAt(*data.ArchivedAt)
	}
	setSessionCreateLifecycleFields(sessionCreate, data)
	if !data.CreationProgressUpdatedAt.IsZero() {
		sessionCreate.SetCreationProgressUpdatedAt(data.CreationProgressUpdatedAt)
	}
}

// setSessionCreateLifecycleFields keeps Create in step with Update: a row recreated by
// an upsert fallback must not lose its workflow link or pause/exit state.
func setSessionCreateLifecycleFields(sessionCreate *ent.SessionCreate, data domain.InstanceData) {
	if data.WorkflowID != "" {
		sessionCreate.SetWorkflowID(data.WorkflowID)
	}
	if data.PauseReason != "" {
		sessionCreate.SetPauseReason(data.PauseReason)
	}
	if data.ExitReason != "" {
		sessionCreate.SetExitReason(data.ExitReason)
	}
	if !data.LastUserResponse.IsZero() {
		sessionCreate.SetLastUserResponse(data.LastUserResponse)
	}
	if !data.ProcessingGraceUntil.IsZero() {
		sessionCreate.SetProcessingGraceUntil(data.ProcessingGraceUntil)
	}
	if !data.LastPromptDetected.IsZero() {
		sessionCreate.SetLastPromptDetected(data.LastPromptDetected)
	}
	if data.LastPromptSignature != "" {
		sessionCreate.SetLastPromptSignature(data.LastPromptSignature)
	}
}

func createWorktreeRow(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) error {
	if data.Worktree.RepoPath == "" {
		return nil
	}
	if _, err := tx.Worktree.Create().
		SetSessionID(sessionID).
		SetRepoPath(data.Worktree.RepoPath).
		SetWorktreePath(data.Worktree.WorktreePath).
		SetSessionName(data.Worktree.SessionName).
		SetBranchName(data.Worktree.BranchName).
		SetBaseCommitSha(data.Worktree.BaseCommitSHA).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}
	return nil
}

func createDiffStatsRow(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) error {
	diffCreate := tx.DiffStats.Create().
		SetSessionID(sessionID).
		SetAdded(data.DiffStats.Added).
		SetRemoved(data.DiffStats.Removed)
	if data.DiffStats.Content != "" {
		diffCreate.SetContent(data.DiffStats.Content)
	}
	if _, err := diffCreate.Save(ctx); err != nil {
		return fmt.Errorf("failed to create diff stats: %w", err)
	}
	return nil
}

// createTagLinks collects all tag IDs (creating missing tags) then does one bulk AddTagIDs.
func createTagLinks(ctx context.Context, tx *ent.Tx, sess *ent.Session, tags []string) error {
	if len(tags) == 0 {
		return nil
	}
	tagIDs := make([]int, 0, len(tags))
	for _, tagName := range tags {
		t, err := tx.Tag.Query().Where(tag.Name(tagName)).Only(ctx)
		if err != nil {
			if !ent.IsNotFound(err) {
				return fmt.Errorf("failed to query tag %s: %w", tagName, err)
			}
			t, err = tx.Tag.Create().SetName(tagName).Save(ctx)
			if err != nil {
				return fmt.Errorf("failed to create tag %s: %w", tagName, err)
			}
		}
		tagIDs = append(tagIDs, t.ID)
	}
	if err := tx.Session.UpdateOne(sess).AddTagIDs(tagIDs...).Exec(ctx); err != nil {
		return fmt.Errorf("failed to associate tags: %w", err)
	}
	return nil
}

// createClaudeRows creates the Claude session row and its metadata entries, if present.
func createClaudeRows(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) error {
	if data.ClaudeSession.ConversationUUID == "" {
		return nil
	}
	claudeSess, err := CreateClaudeSessionRow(ctx, tx, sessionID, data)
	if err != nil {
		return err
	}
	for key, value := range data.ClaudeSession.Metadata {
		if _, err := tx.ClaudeMetadata.Create().
			SetClaudeSessionID(claudeSess.ID).
			SetKey(key).
			SetValue(value).
			Save(ctx); err != nil {
			return fmt.Errorf("failed to create claude metadata %s: %w", key, err)
		}
	}
	return nil
}

// CreateClaudeSessionRow inserts the claude_sessions row; Update (update.go) reuses it.
func CreateClaudeSessionRow(ctx context.Context, tx *ent.Tx, sessionID int, data domain.InstanceData) (*ent.ClaudeSession, error) {
	claudeCreate := tx.ClaudeSession.Create().
		SetSessionID(sessionID).
		SetClaudeSessionID(data.ClaudeSession.ConversationUUID).
		SetAutoReattach(data.ClaudeSession.Settings.AutoReattach).
		SetCreateNewOnMissing(data.ClaudeSession.Settings.CreateNewOnMissing).
		SetShowSessionSelector(data.ClaudeSession.Settings.ShowSessionSelector).
		SetSessionTimeoutMinutes(data.ClaudeSession.Settings.SessionTimeoutMinutes)
	if data.ClaudeSession.SquadSessionID != "" {
		claudeCreate.SetConversationID(data.ClaudeSession.SquadSessionID)
	}
	if data.ClaudeSession.ProjectName != "" {
		claudeCreate.SetProjectName(data.ClaudeSession.ProjectName)
	}
	if !data.ClaudeSession.LastAttached.IsZero() {
		claudeCreate.SetLastAttached(data.ClaudeSession.LastAttached)
	}
	if data.ClaudeSession.Settings.PreferredSessionName != "" {
		claudeCreate.SetPreferredSessionName(data.ClaudeSession.Settings.PreferredSessionName)
	}
	created, err := claudeCreate.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create claude session: %w", err)
	}
	return created, nil
}

// SuppressedRuleTagsToSlice (shared with Update in update.go) converts Instance.SuppressedRuleTags' in-memory
// map[string]bool representation to the []string set persisted in the
// suppressed_rule_tags JSON column (session/ent/schema/session.go) — a simpler
// on-disk shape than a bool-valued JSON object, per Task 3.1.2a.
func SuppressedRuleTagsToSlice(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for tag, suppressed := range m {
		if suppressed {
			out = append(out, tag)
		}
	}
	return out
}
