package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tokens"
)

// backfillWindow is how long after an ItemSession's creation its transcript's first
// message may land. Observed gaps for real backlog sessions were 0-4s; 3 minutes leaves
// headroom for a slow startup while staying well under the spacing between sessions.
const backfillWindow = 3 * time.Minute

// conversationStamper is the write side of the backfill; satisfied by *session.Storage.
type conversationStamper interface {
	UpdateItemSessionConversationUUID(ctx context.Context, id string, conversationUUID string) error
}

// BackfillConversationUUIDs links pre-existing ItemSessions to their transcripts
// so Insights can attribute them. Conservative: links only when exactly one
// unlinked, non-live ItemSession was created within backfillWindow before the
// transcript's first message; ambiguous or unmatched transcripts are left
// alone. Idempotent; returns how many rows it stamped.
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

	candidates := backfillCandidates(entries, liveSessions)
	transcripts := backfillTranscripts(s.store.GetAll(), claimed, liveConversations)

	stamped := 0
	used := make(map[string]bool) // ItemSession IDs stamped this run
	for _, tr := range transcripts {
		// Checked once per outer-loop iteration (not per inner-loop candidate
		// comparison): the O(n×m) match loop below has no other suspension
		// point, so without this a caller's timeout is only ever enforced by
		// the DB calls inside the loop, never by the loop itself.
		if err := ctx.Err(); err != nil {
			return stamped, err
		}
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

// backfillTranscript is a transcript's conversation UUID and first-message time,
// the two fields the match loop in BackfillConversationUUIDs needs.
type backfillTranscript struct {
	conversationUUID string
	firstTs          time.Time
}

// backfillCandidates filters entries down to ItemSessions eligible for stamping: a
// real (non-synthetic) session UUID, not already linked, and not currently live.
func backfillCandidates(entries []session.ItemSessionBacklogEntry, liveSessions map[string]bool) []session.ItemSessionBacklogEntry {
	var candidates []session.ItemSessionBacklogEntry
	for _, e := range entries {
		// A synthetic UUID (headless-*, empty-diff-*, ...) never had a session; live
		// sessions get stamped by EntRepository.Delete instead.
		if _, perr := uuid.Parse(e.SessionUUID); perr != nil || e.ConversationUUID != "" || liveSessions[e.SessionUUID] || e.CreatedAt.IsZero() {
			continue
		}
		candidates = append(candidates, e)
	}
	return candidates
}

// backfillTranscripts filters transcript records down to worktree-backed sessions
// not already claimed or live, sorted by first-message time so the match loop in
// BackfillConversationUUIDs processes earliest transcripts first.
func backfillTranscripts(records []*tokens.ParseResult, claimed, liveConversations map[string]bool) []backfillTranscript {
	var transcripts []backfillTranscript
	for _, r := range records {
		if r == nil || r.SessionUUID == "" || claimed[r.SessionUUID] || liveConversations[r.SessionUUID] || !strings.Contains(r.ProjectPath, "/worktrees/") {
			continue
		}
		if first, _ := sessionTimestamps(r); !first.IsZero() {
			transcripts = append(transcripts, backfillTranscript{r.SessionUUID, first})
		}
	}
	sort.Slice(transcripts, func(i, j int) bool { return transcripts[i].firstTs.Before(transcripts[j].firstTs) })
	return transcripts
}
