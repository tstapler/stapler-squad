package services

import (
	"sync"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
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

// filterInPlace removes every element for which exclude returns true,
// keeping the rest in their original relative order. The returned slice
// aliases items' backing array (the items[:0] idiom) — callers must not use
// items after this call. Shared by filterHistoryEntriesByAutomation,
// filterAutomationSessions, and filterByProject, which differ only in
// element type and predicate — per this repo's own
// the `interface-pollution-checklist` skill's rule 5 ("generalize once
// 2+ real call sites need identical logic"), now satisfied by three.
func filterInPlace[T any](items []T, exclude func(T) bool) []T {
	out := items[:0]
	for _, item := range items {
		if exclude(item) {
			continue
		}
		out = append(out, item)
	}
	return out
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
	return filterInPlace(entries, func(e session.ClaudeHistoryEntry) bool {
		return isAutomationSession(e.ID, instances)
	})
}

// filterAutomationSessions removes results whose session is a live,
// Hidden=true Instance. The returned slice aliases results' backing array
// (results[:0] idiom) — callers must not use results after this call.
func filterAutomationSessions(results []*sessionv1.SearchResult, instances []*session.Instance) []*sessionv1.SearchResult {
	return filterInPlace(results, func(r *sessionv1.SearchResult) bool {
		return isAutomationSession(r.SessionId, instances)
	})
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
	return filterInPlace(results, func(r *sessionv1.SearchResult) bool {
		return resolvedProject(r.SessionId, r.Project, instances) != project
	})
}

// applyResultPostProcessing runs SearchClaudeHistory's project → automation
// → dedup → truncate → context-enrich pipeline on raw search results, in
// that specific order: project scoping is the broadest filter and narrows
// the candidate set first, then automation-session filtering, then dedup —
// so an out-of-project or excluded-automation hit never contributes to
// MoreMatchesInSessionCount on a kept result. Context/bookends are computed
// only for the final, post-truncation result set, not wastefully on raw
// hits discarded above. Isolated from the RPC handler so the pipeline is
// unit-testable without a ConnectRPC round-trip.
func (ss *SearchService) applyResultPostProcessing(
	protoResults []*sessionv1.SearchResult,
	req *sessionv1.SearchClaudeHistoryRequest,
	hist *session.ClaudeSessionHistory,
	requestedLimit int,
	needsPostProcessing bool,
) []*sessionv1.SearchResult {
	var instances []*session.Instance
	if ss.getInstances != nil && (req.GetProject() != "" || req.GetExcludeAutomationSessions()) {
		instances = ss.getInstances()
	}

	if project := req.GetProject(); project != "" {
		protoResults = filterByProject(protoResults, project, instances)
	}

	if req.GetExcludeAutomationSessions() {
		excludedBefore := len(protoResults)
		protoResults = filterAutomationSessions(protoResults, instances)
		if excluded := excludedBefore - len(protoResults); excluded > 0 {
			log.Info("search: excluded automation sessions", "count", excluded)
		}
	}

	if req.GetGroupBySession() {
		protoResults = groupResultsBySession(protoResults)
	}

	if needsPostProcessing && len(protoResults) > requestedLimit {
		protoResults = protoResults[:requestedLimit]
	}

	if req.GetIncludeContext() {
		enrichWithContext(protoResults, hist)
	}

	return protoResults
}

// enrichWithContext populates ContextWindow/BookendFirst/BookendLast on each
// result concurrently — each result requires its own conversation-file read
// (session/history.go's GetMessagesFromConversationFile with limit=0 does a
// full-file parse), so a serial loop would multiply per-request latency by
// len(results). Each goroutine writes only to its own *SearchResult, so no
// synchronization beyond the WaitGroup is needed.
func enrichWithContext(results []*sessionv1.SearchResult, hist *session.ClaudeSessionHistory) {
	var wg sync.WaitGroup
	for _, r := range results {
		wg.Add(1)
		go func(r *sessionv1.SearchResult) {
			defer wg.Done()
			msgs, err := hist.GetMessagesFromConversationFile(r.SessionId, 0)
			if err != nil {
				return // best-effort: leave context fields empty rather than failing the whole search
			}
			window, first, last := contextWindowAndBookends(msgs, int(r.MessageIndex))
			r.ContextWindow = toProtoClaudeMessages(window)
			r.BookendFirst = toProtoClaudeMessages(first)
			r.BookendLast = toProtoClaudeMessages(last)
		}(r)
	}
	wg.Wait()
}
