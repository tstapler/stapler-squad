package instancestore

import (
	"context"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/ent/session"
)

// UpdateGitHubPRNumber persists a discovered PR number for a session.
// Called by PRStatusPoller when it auto-discovers a PR for a branch-based session.
func (i *Instances) UpdateGitHubPRNumber(ctx context.Context, title string, prNumber int) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetGithubPrNumber(prNumber).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update github_pr_number: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateGitHubPRStatusTerminal persists whether a session's PR has reached a
// terminal (merged/closed) state. Called by PRStatusPoller on every poll so the
// flag survives a restart — see SessionRetentionSweeper.baseSafeToDelete, which
// reads it back from storage rather than the in-memory Instance.
func (i *Instances) UpdateGitHubPRStatusTerminal(ctx context.Context, title string, terminal bool) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetGithubPrStatusTerminal(terminal).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update github_pr_status_terminal: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateSessionArtifacts persists the JSON-encoded artifact blob for a session.
// Wrapped in a transaction for correctness under concurrent writes (M-6 fix).
// The per-title mutex in ArtifactExtractor (C-1) serializes calls at the application
// layer; the transaction is belt-and-suspenders for correctness.
func (i *Instances) UpdateSessionArtifacts(ctx context.Context, title string, blob string) error {
	tx, err := i.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("UpdateSessionArtifacts: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	n, err := tx.Session.Update().
		Where(session.Title(title)).
		SetSessionArtifacts(blob).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update session_artifacts: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return tx.Commit()
}

// UpdateReviewQueueState efficiently updates only the review-queue interaction fields
// for a session, avoiding the full read-modify-write cycle of updateFieldInRepo.
func (i *Instances) UpdateReviewQueueState(ctx context.Context, title string, lastUserResponse, processingGraceUntil, lastPromptDetected time.Time, lastPromptSignature string) error {
	update := i.client.Session.Update().
		Where(session.Title(title)).
		SetUpdatedAt(time.Now())

	if !lastUserResponse.IsZero() {
		update.SetLastUserResponse(lastUserResponse)
	}
	if !processingGraceUntil.IsZero() {
		update.SetProcessingGraceUntil(processingGraceUntil)
	} else {
		update.ClearProcessingGraceUntil()
	}
	if !lastPromptDetected.IsZero() {
		update.SetLastPromptDetected(lastPromptDetected)
	}
	if lastPromptSignature != "" {
		update.SetLastPromptSignature(lastPromptSignature)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update review queue state: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateLastUserResponse sets only last_user_response. Unlike UpdateReviewQueueState
// it never clears processing_grace_until.
func (i *Instances) UpdateLastUserResponse(ctx context.Context, title string, t time.Time) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetLastUserResponse(t).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update last_user_response: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateTimestamps efficiently updates only timestamp fields for a session
func (i *Instances) UpdateTimestamps(ctx context.Context, title string, lastTerminalUpdate, lastMeaningfulOutput time.Time, lastOutputSignature string) error {
	// Direct UPDATE without prior SELECT — mirrors the pattern used by UpdateLastAddedToQueue.
	// This method is called on every terminal output event; avoiding the SELECT halves the
	// SQL round-trips on this hot path.
	update := i.client.Session.Update().
		Where(session.Title(title)).
		SetUpdatedAt(time.Now())

	if !lastTerminalUpdate.IsZero() {
		update.SetLastTerminalUpdate(lastTerminalUpdate)
	}
	if !lastMeaningfulOutput.IsZero() {
		update.SetLastMeaningfulOutput(lastMeaningfulOutput)
	}
	if lastOutputSignature != "" {
		update.SetLastOutputSignature(lastOutputSignature)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update timestamps: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateLastAddedToQueue sets only the last_added_to_queue field for a session,
// issuing a single UPDATE WHERE title=? without a prior SELECT.
func (i *Instances) UpdateLastAddedToQueue(ctx context.Context, title string, t time.Time) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetLastAddedToQueue(t).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update last_added_to_queue: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateInitialPromptSentAt sets only the initial_prompt_sent_at field for a
// session, via a direct UPDATE (no read round-trip) -- see Instance.InitialPromptSentAt's
// doc comment for why this must survive a restart.
func (i *Instances) UpdateInitialPromptSentAt(ctx context.Context, title string, t time.Time) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetInitialPromptSentAt(t).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update initial_prompt_sent_at: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateLastAcknowledged sets only the last_acknowledged field for a session,
// issuing a single UPDATE WHERE title=? without a prior SELECT.
func (i *Instances) UpdateLastAcknowledged(ctx context.Context, title string, t time.Time) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetLastAcknowledged(t).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update last_acknowledged: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateLastViewed sets only the last_viewed field for a session,
// issuing a single UPDATE WHERE title=? without a prior SELECT.
func (i *Instances) UpdateLastViewed(ctx context.Context, title string, t time.Time) error {
	n, err := i.client.Session.Update().
		Where(session.Title(title)).
		SetLastViewed(t).
		SetUpdatedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update last_viewed: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", title)
	}
	return nil
}

// UpdateSessionMetadata efficiently updates only title/category/note/working_dir fields
// for a session, issuing a single UPDATE WHERE title=? without a prior SELECT and without
// the worktree/diffstats/tags/claude_session writes the full Update method performs —
// mirrors UpdateLastViewed's shape. currentTitle must be the row's title from BEFORE any
// rename already applied to the caller's in-memory Instance in this same request: Update
// looks the row up by data.Title (the post-rename value), which misses the still-old-titled
// DB row and falls into Update's Create fallback, orphaning it under the new title. Using
// currentTitle as the WHERE key avoids that. Category/WorkingDir are only set when non-nil
// AND non-empty, matching Update's existing guarded (`data.Category != ""`) semantics;
// Note is set whenever non-nil (including ""), since an empty note is a meaningful cleared
// state, not "unset" — same asymmetry as Update's unconditional SetNote(data.Note).
func (i *Instances) UpdateSessionMetadata(ctx context.Context, currentTitle string, newTitle, category, note, workingDir *string) error {
	if newTitle != nil && *newTitle != "" && *newTitle != currentTitle {
		// The running tmux session keeps the name it was created with.
		if err := i.client.Session.Update().
			Where(session.Title(currentTitle), session.Or(session.TmuxTitleIsNil(), session.TmuxTitle(""))).
			SetTmuxTitle(currentTitle).
			Exec(ctx); err != nil {
			return fmt.Errorf("failed to pin tmux title: %w", err)
		}
	}

	update := i.client.Session.Update().
		Where(session.Title(currentTitle)).
		SetUpdatedAt(time.Now())

	if newTitle != nil && *newTitle != "" {
		update.SetTitle(*newTitle)
	}
	if category != nil && *category != "" {
		update.SetCategory(*category)
	}
	if note != nil {
		update.SetNote(*note)
	}
	if workingDir != nil && *workingDir != "" {
		update.SetWorkingDir(*workingDir)
	}

	n, err := update.Save(ctx)
	if err != nil {
		return fmt.Errorf("failed to update session metadata: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("session not found: %s", currentTitle)
	}
	return nil
}
