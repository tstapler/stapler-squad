package instancestore

import (
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

// suppressedRuleTagsFromSlice is suppressedRuleTagsToSlice's inverse, used when
// loading a session back from the suppressed_rule_tags JSON column.
func suppressedRuleTagsFromSlice(s []string) map[string]bool {
	if len(s) == 0 {
		return nil
	}
	out := make(map[string]bool, len(s))
	for _, tag := range s {
		out[tag] = true
	}
	return out
}

// sessionToInstanceData converts an Ent Session entity to InstanceData
func sessionToInstanceData(sess *ent.Session) *domain.InstanceData {
	data := &domain.InstanceData{
		Title:               sess.Title,
		UUID:                sess.UUID,
		Path:                sess.Path,
		WorkingDir:          sess.WorkingDir,
		Branch:              sess.Branch,
		Status:              domain.Status(sess.Status),
		Height:              sess.Height,
		Width:               sess.Width,
		CreatedAt:           sess.CreatedAt,
		UpdatedAt:           sess.UpdatedAt,
		AutoYes:             sess.AutoYes,
		AutoApprove:         sess.AutoApprove,
		AutonomousMode:      sess.AutonomousMode,
		Prompt:              sess.Prompt,
		InitialPrompt:       sess.InitialPrompt,
		Program:             sess.Program,
		ExistingWorktree:    sess.ExistingWorktree,
		Category:            sess.Category,
		Note:                sess.Note,
		IsExpanded:          sess.IsExpanded,
		TmuxPrefix:          sess.TmuxPrefix,
		TmuxTitle:           sess.TmuxTitle,
		Backend:             domain.ProcessManagerBackend(sess.Backend),
		TymuxSessionID:      sess.TymuxSessionID,
		LastOutputSignature: sess.LastOutputSignature,
		MCPServerURL:        sess.McpServerURL,
		OneShot:             sess.OneShot,
		Hidden:              sess.Hidden,
		Pinned:              sess.Pinned,
	}

	// Set optional time fields
	if sess.LastTerminalUpdate != nil {
		data.LastTerminalUpdate = *sess.LastTerminalUpdate
	}
	if sess.LastMeaningfulOutput != nil {
		data.LastMeaningfulOutput = *sess.LastMeaningfulOutput
	}
	if sess.LastAddedToQueue != nil {
		data.LastAddedToQueue = *sess.LastAddedToQueue
	}
	if sess.LastViewed != nil {
		data.LastViewed = *sess.LastViewed
	}
	if sess.LastAcknowledged != nil {
		data.LastAcknowledged = *sess.LastAcknowledged
	}
	if sess.LastUserResponse != nil {
		data.LastUserResponse = *sess.LastUserResponse
	}
	if sess.ProcessingGraceUntil != nil {
		data.ProcessingGraceUntil = *sess.ProcessingGraceUntil
	}
	if sess.LastPromptDetected != nil {
		data.LastPromptDetected = *sess.LastPromptDetected
	}
	if sess.InitialPromptSentAt != nil {
		data.InitialPromptSentAt = *sess.InitialPromptSentAt
	}
	data.LastPromptSignature = sess.LastPromptSignature
	data.PauseReason = sess.PauseReason
	data.ExitReason = sess.ExitReason
	data.WorkflowID = sess.WorkflowID
	data.ArchivedAt = sess.ArchivedAt
	if sess.CreationProgressUpdatedAt != nil {
		data.CreationProgressUpdatedAt = *sess.CreationProgressUpdatedAt
	}
	data.GitHubPRURL = sess.GithubPrURL
	data.GitHubPRNumber = sess.GithubPrNumber
	data.GitHubPRStatusTerminal = sess.GithubPrStatusTerminal
	data.GitHubOwner = sess.GithubOwner
	data.GitHubRepo = sess.GithubRepo
	data.GitHubHost = sess.GithubHost

	// Set session type
	if sess.SessionType != "" {
		data.SessionType = domain.SessionType(sess.SessionType)
	}

	// Convert worktree if present
	if sess.Edges.Worktree != nil {
		data.Worktree = domain.GitWorktreeData{
			RepoPath:      sess.Edges.Worktree.RepoPath,
			WorktreePath:  sess.Edges.Worktree.WorktreePath,
			SessionName:   sess.Edges.Worktree.SessionName,
			BranchName:    sess.Edges.Worktree.BranchName,
			BaseCommitSHA: sess.Edges.Worktree.BaseCommitSha,
		}
	}

	// Convert diff stats if present
	if sess.Edges.DiffStats != nil {
		data.DiffStats = domain.DiffStatsData{
			Added:   sess.Edges.DiffStats.Added,
			Removed: sess.Edges.DiffStats.Removed,
			Content: sess.Edges.DiffStats.Content,
		}
	}

	// Convert tags
	if len(sess.Edges.Tags) > 0 {
		data.Tags = make([]string, len(sess.Edges.Tags))
		for i, t := range sess.Edges.Tags {
			data.Tags[i] = t.Name
		}
	}

	// RuleTagProvenance/SuppressedRuleTags (ADR-002). A pre-migration row lacking
	// these JSON columns decodes both to nil maps via ent's Default(), never an
	// error — same backward-compat shape as the Category->Tags shim above.
	data.RuleTagProvenance = sess.RuleTagProvenance
	data.SuppressedRuleTags = suppressedRuleTagsFromSlice(sess.SuppressedRuleTags)

	// Populate project ID from project edge (stored as name for string compatibility)
	if sess.Edges.Project != nil {
		data.ProjectID = sess.Edges.Project.Name
	}

	// Convert Claude session if present
	if sess.Edges.ClaudeSession != nil {
		cs := sess.Edges.ClaudeSession
		data.ClaudeSession = domain.ClaudeSessionData{
			ConversationUUID: cs.ClaudeSessionID,
			SquadSessionID:   cs.ConversationID,
			ProjectName:      cs.ProjectName,
			Settings: domain.ClaudeSettings{
				AutoReattach:          cs.AutoReattach,
				PreferredSessionName:  cs.PreferredSessionName,
				CreateNewOnMissing:    cs.CreateNewOnMissing,
				ShowSessionSelector:   cs.ShowSessionSelector,
				SessionTimeoutMinutes: cs.SessionTimeoutMinutes,
			},
		}

		// Set optional time field
		if cs.LastAttached != nil {
			data.ClaudeSession.LastAttached = *cs.LastAttached
		}

		// Convert metadata
		if len(cs.Edges.Metadata) > 0 {
			data.ClaudeSession.Metadata = make(map[string]string)
			for _, m := range cs.Edges.Metadata {
				data.ClaudeSession.Metadata[m.Key] = m.Value
			}
		}
	}

	return data
}
