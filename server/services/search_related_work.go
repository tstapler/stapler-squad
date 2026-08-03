package services

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// toProtoClaudeMessages converts raw conversation messages to their proto
// representation. Shared by GetClaudeHistoryMessages and the context-window
// enrichment in SearchClaudeHistory.
func toProtoClaudeMessages(messages []session.ClaudeConversationMessage) []*sessionv1.ClaudeMessage {
	out := make([]*sessionv1.ClaudeMessage, 0, len(messages))
	for _, msg := range messages {
		out = append(out, &sessionv1.ClaudeMessage{
			Role:      msg.Role,
			Content:   msg.Content,
			Timestamp: timestamppb.New(msg.Timestamp),
			Model:     msg.Model,
		})
	}
	return out
}

// groupResultsBySession collapses results to one entry per SessionId,
// keeping the first-encountered (highest-scored, since results arrives
// pre-sorted by score) result per session and accumulating
// MoreMatchesInSessionCount on it for every subsequent same-session hit.
func groupResultsBySession(results []*sessionv1.SearchResult) []*sessionv1.SearchResult {
	kept := make(map[string]*sessionv1.SearchResult, len(results))
	order := make([]*sessionv1.SearchResult, 0, len(results))
	for _, r := range results {
		if existing, ok := kept[r.SessionId]; ok {
			existing.MoreMatchesInSessionCount++
			continue
		}
		kept[r.SessionId] = r
		order = append(order, r)
	}
	return order
}

// contextWindowAndBookends returns the clamped ±5 window around hitIndex
// and the session's first-3/last-3 bookend messages. Each bookend is
// suppressed independently — not just when the window spans the entire
// transcript — whenever that side of the window already reaches the
// corresponding boundary, to avoid returning duplicate content across the
// two fields (e.g. a hit near the start of a long session must not repeat
// messages[0:3] in both context_window and bookend_first).
func contextWindowAndBookends(messages []session.ClaudeConversationMessage, hitIndex int) (window, bookendFirst, bookendLast []session.ClaudeConversationMessage) {
	if len(messages) == 0 {
		return nil, nil, nil
	}
	start := hitIndex - 5
	if start < 0 {
		start = 0
	}
	end := hitIndex + 6
	if end > len(messages) {
		end = len(messages)
	}
	window = messages[start:end]

	if start > 0 {
		firstEnd := 3
		if firstEnd > len(messages) {
			firstEnd = len(messages)
		}
		bookendFirst = messages[:firstEnd]
	}
	if end < len(messages) {
		lastStart := len(messages) - 3
		if lastStart < 0 {
			lastStart = 0
		}
		bookendLast = messages[lastStart:]
	}
	return window, bookendFirst, bookendLast
}

// findInstanceBySessionID returns the live Instance whose Claude conversation
// UUID matches sessionID, or nil if none is currently tracked. Shared by
// isAutomationSession and resolvedProject, which each need this lookup for a
// different field on the same Instance.
func findInstanceBySessionID(sessionID string, instances []*session.Instance) *session.Instance {
	for _, inst := range instances {
		if inst.GetConversationUUID() == sessionID {
			return inst
		}
	}
	return nil
}

// isAutomationSession returns true only when sessionID matches a
// currently-live Instance with Hidden=true — the same field ListSessions'
// include_hidden flag uses to hide background/triage/review sessions.
// Best-effort: returns false (not automation) for any session with no
// live Instance match, since absence of the live record is not evidence
// either way.
func isAutomationSession(sessionID string, instances []*session.Instance) bool {
	inst := findInstanceBySessionID(sessionID, instances)
	return inst != nil && inst.Hidden
}

// filterHistoryEntriesByAutomation removes entries whose session is a live,
// Hidden=true Instance — the ListClaudeHistory ("browse mode") counterpart
// of filterAutomationSessions, sharing the same isAutomationSession
// predicate. The returned slice aliases entries' backing array (entries[:0]
// idiom) — callers must not use entries after this call.
func filterHistoryEntriesByAutomation(entries []session.ClaudeHistoryEntry, instances []*session.Instance) []session.ClaudeHistoryEntry {
	out := entries[:0]
	for _, e := range entries {
		if isAutomationSession(e.ID, instances) {
			continue
		}
		out = append(out, e)
	}
	return out
}

// filterAutomationSessions removes results whose session is a live,
// Hidden=true Instance. The returned slice aliases results' backing array
// (results[:0] idiom) — callers must not use results after this call.
func filterAutomationSessions(results []*sessionv1.SearchResult, instances []*session.Instance) []*sessionv1.SearchResult {
	out := results[:0]
	for _, r := range results {
		if isAutomationSession(r.SessionId, instances) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// resolvedProject returns the main-repo path for a session's rawProject
// (the session's raw, possibly-worktree Project string). If the session has
// a live Instance whose Path is a worktree (i.e. MainRepoPath is set), that
// main-repo path is returned instead of rawProject — mirroring
// isAutomationSession's pattern of cross-referencing live Instance state
// rather than trusting entry.Project at face value. Falls back to rawProject
// when the session isn't a worktree, or when no live Instance match exists —
// in the latter case a worktree session cannot be resolved to its main
// repo and will compare unequal via raw string equality (documented
// best-effort limitation: without a live Instance there is no way to
// distinguish an unresolvable worktree path from a genuinely different
// project, so this errs toward excluding rather than admitting
// cross-project noise into scoped results).
func resolvedProject(sessionID, rawProject string, instances []*session.Instance) string {
	inst := findInstanceBySessionID(sessionID, instances)
	if inst != nil && inst.MainRepoPath != "" {
		return inst.MainRepoPath
	}
	return rawProject
}

// filterByProject keeps only results whose session's resolved repo path
// (see resolvedProject) exactly matches project. A no-op when project is
// empty (matches today's unfiltered behavior). The returned slice aliases
// results' backing array (results[:0] idiom) — callers must not use
// results after this call.
func filterByProject(results []*sessionv1.SearchResult, project string, instances []*session.Instance) []*sessionv1.SearchResult {
	if project == "" {
		return results
	}
	out := results[:0]
	for _, r := range results {
		if resolvedProject(r.SessionId, r.Project, instances) == project {
			out = append(out, r)
		}
	}
	return out
}
