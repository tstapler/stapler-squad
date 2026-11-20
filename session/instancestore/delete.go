package instancestore

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/claudemetadata"
	"github.com/tstapler/stapler-squad/session/ent/claudesession"
	"github.com/tstapler/stapler-squad/session/ent/diffstats"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/session"
	entshell "github.com/tstapler/stapler-squad/session/ent/shell"
	"github.com/tstapler/stapler-squad/session/ent/worktree"
)

// Delete removes a session from the database by title
func (i *Instances) Delete(ctx context.Context, title string) error {
	// Start transaction for atomic deletion
	tx, err := i.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Find session by title
	sess, err := tx.Session.Query().Where(session.Title(title)).Only(ctx)
	if err != nil {
		return fmt.Errorf("failed to find session: %w", err)
	}

	// Delete related entities first (manual cascade)

	// Delete worktree if exists
	if _, err := tx.Worktree.Delete().Where(worktree.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete worktree: %w", err)
	}

	// Delete diff stats if exists
	if _, err := tx.DiffStats.Delete().Where(diffstats.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete diff stats: %w", err)
	}

	// Stamp the conversation UUID onto the session's item_sessions before the claude_sessions
	// row is deleted, so Insights can still attribute the transcript afterward.
	if err := stampItemSessionConversationUUID(ctx, tx, sess); err != nil {
		return err
	}

	// Delete claude session and its metadata if exists
	// Delete all claude metadata associated with claude sessions for this session
	if _, err := tx.ClaudeMetadata.Delete().Where(claudemetadata.HasClaudeSessionWith(claudesession.HasSessionWith(session.ID(sess.ID)))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete claude metadata: %w", err)
	}
	// Delete all claude sessions for this session
	if _, err := tx.ClaudeSession.Delete().Where(claudesession.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete claude sessions: %w", err)
	}

	// Clear tag associations (many-to-many)
	if err := tx.Session.UpdateOne(sess).ClearTags().Exec(ctx); err != nil {
		return fmt.Errorf("failed to clear tags: %w", err)
	}

	// Delete shells (sibling tmux sessions) — Shell.session edge is Required(),
	// so leaving any rows here trips a FOREIGN KEY constraint on session delete.
	if _, err := tx.Shell.Delete().Where(entshell.HasSessionWith(session.ID(sess.ID))).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete shells: %w", err)
	}

	// PR links and detach tombstones are keyed by the session uuid, not an FK.
	if sess.UUID != "" {
		if err := i.ports.DeleteOwnerPRRows(ctx, tx, domain.SessionPROwner(sess.UUID)); err != nil {
			return err
		}
	}

	// Finally delete the session
	if err := tx.Session.DeleteOne(sess).Exec(ctx); err != nil {
		return fmt.Errorf("failed to delete session: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// stampItemSessionConversationUUID copies sess's Claude conversation UUID onto its
// item_sessions rows that don't have one yet. No-op for sessions without a uuid or
// conversation (e.g. non-Claude programs).
func stampItemSessionConversationUUID(ctx context.Context, tx *ent.Tx, sess *ent.Session) error {
	if sess.UUID == "" {
		return nil
	}
	cs, err := tx.ClaudeSession.Query().Where(claudesession.HasSessionWith(session.ID(sess.ID))).First(ctx)
	if ent.IsNotFound(err) || (err == nil && cs.ClaudeSessionID == "") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read claude session for item-session stamp: %w", err)
	}
	if _, err := tx.ItemSession.Update().
		Where(itemsession.SessionUUID(sess.UUID), itemsession.ConversationUUID("")).
		SetConversationUUID(cs.ClaudeSessionID).
		Save(ctx); err != nil {
		return fmt.Errorf("failed to stamp conversation uuid on item sessions: %w", err)
	}
	return nil
}
