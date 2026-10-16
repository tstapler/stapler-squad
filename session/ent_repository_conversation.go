package session

import (
	"context"
	"fmt"

	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/claudesession"
	"github.com/tstapler/stapler-squad/session/ent/itemsession"
	"github.com/tstapler/stapler-squad/session/ent/session"
)

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
