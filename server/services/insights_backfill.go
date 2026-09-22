package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// backfillWindow is how long after an ItemSession's creation its transcript's first
// message may land. Observed gaps for real backlog sessions were 0-4s; 3 minutes leaves
// headroom for a slow startup while staying well under the spacing between sessions.
const backfillWindow = 3 * time.Minute

// conversationStamper is the write side of the backfill; satisfied by *session.Storage.
type conversationStamper interface {
	UpdateItemSessionConversationUUID(ctx context.Context, id string, conversationUUID string) error
}

// BackfillConversationUUIDs links pre-existing ItemSessions (whose session row was
// deleted before conversation_uuid existed) to their transcripts, so Insights can
// attribute them. Conservative: a transcript is linked only when exactly one unlinked,
// non-live ItemSession with a real session UUID was created within backfillWindow
// before its first message and the transcript ran in a worktree. Ambiguous or
// unmatched transcripts are left alone. Idempotent; returns how many rows it stamped.
func (s *InsightsService) BackfillConversationUUIDs(ctx context.Context) (int, error) {
	stamper, ok := s.backlogReader.(conversationStamper)
	if !ok || s.associator == nil {
		return 0, nil
	}
	entries, err := s.backlogReader.GetAllItemSessionsWithBacklogInfo(ctx)
	if err != nil {
		return 0, err
	}

	claimed := make(map[string]bool) // conversation UUIDs already on an ItemSession
	for _, e := range entries {
		if e.ConversationUUID != "" {
			claimed[e.ConversationUUID] = true
		}
	}
	liveSessions, liveConversations := make(map[string]bool), make(map[string]bool)
	for _, rec := range s.associator.Snapshot() {
		liveSessions[rec.SessionID] = true
		if rec.ConversationID != "" {
			liveConversations[rec.ConversationID] = true
		}
	}

	var candidates []session.ItemSessionBacklogEntry
	for _, e := range entries {
		// A synthetic UUID (headless-*, empty-diff-*, ...) never had a session; live
		// sessions get stamped by EntRepository.Delete instead.
		if _, perr := uuid.Parse(e.SessionUUID); perr != nil || e.ConversationUUID != "" || liveSessions[e.SessionUUID] || e.CreatedAt.IsZero() {
			continue
		}
		candidates = append(candidates, e)
	}

	type transcript struct {
		conversationUUID string
		firstTs          time.Time
	}
	var transcripts []transcript
	for _, r := range s.store.GetAll() {
		if r == nil || r.SessionUUID == "" || claimed[r.SessionUUID] || liveConversations[r.SessionUUID] || !strings.Contains(r.ProjectPath, "/worktrees/") {
			continue
		}
		if first, _ := sessionTimestamps(r); !first.IsZero() {
			transcripts = append(transcripts, transcript{r.SessionUUID, first})
		}
	}
	sort.Slice(transcripts, func(i, j int) bool { return transcripts[i].firstTs.Before(transcripts[j].firstTs) })

	stamped := 0
	used := make(map[string]bool) // ItemSession IDs stamped this run
	for _, tr := range transcripts {
		var match *session.ItemSessionBacklogEntry
		matches := 0
		for i := range candidates {
			c := &candidates[i]
			if used[c.ItemSessionID] {
				continue
			}
			if gap := tr.firstTs.Sub(c.CreatedAt); gap >= 0 && gap <= backfillWindow {
				match, matches = c, matches+1
			}
		}
		if matches != 1 {
			continue
		}
		if err := stamper.UpdateItemSessionConversationUUID(ctx, match.ItemSessionID, tr.conversationUUID); err != nil {
			log.Warn("insights backfill: failed to stamp conversation uuid", "itemSession", match.ItemSessionID, "err", err)
			continue
		}
		used[match.ItemSessionID] = true
		stamped++
	}
	if stamped > 0 {
		log.Info("insights backfill: linked historical transcripts to item sessions", "count", stamped)
	}
	return stamped, nil
}
