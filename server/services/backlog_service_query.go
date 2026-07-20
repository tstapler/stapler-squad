package services

// backlog_service_query.go — read-only RPC handlers for BacklogService.
// All functions here are pure reads; they mutate no state.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	gh "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// sourceSyncEventToProto converts a SourceSyncEventData to its proto representation.
func sourceSyncEventToProto(ev session.SourceSyncEventData) *sessionv1.SourceSyncEvent {
	p := &sessionv1.SourceSyncEvent{
		Id:           ev.ID,
		StartedAt:    timestamppb.New(ev.StartedAt),
		ItemsCreated: int32(ev.ItemsCreated),
		ItemsUpdated: int32(ev.ItemsUpdated),
		ItemsSkipped: int32(ev.ItemsSkipped),
		ItemsErrored: int32(ev.ItemsErrored),
		ErrorMessage: ev.ErrorMessage,
	}
	if ev.FinishedAt != nil {
		p.FinishedAt = timestamppb.New(*ev.FinishedAt)
	}
	return p
}

// --- GetBacklogItem ---

// GetBacklogItem retrieves a single backlog item by ID.
// +api: backlog:get-item
func (s *BacklogService) GetBacklogItem(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBacklogItemRequest],
) (*connect.Response[sessionv1.GetBacklogItemResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	// Load item sessions with review verdicts so the detail panel can show gate results.
	isSessions, isErr := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if isErr != nil {
		log.ErrorLog.Printf("[GetBacklogItem] failed to load item sessions for %s: %v", req.Msg.ItemId, isErr)
		// Non-fatal: return item without sessions.
	} else {
		// Tombstone stale headless-triage sessions (no endedAt, older than maxTriageSessionAge)
		// so they don't appear as "running" indefinitely after the triage process has exited.
		// Sessions younger than maxTriageSessionAge are still running their goroutine — leave them alone.
		now := time.Now()
		for i := range isSessions {
			is := &isSessions[i]
			if is.Role == string(session.SessionRoleTriage) &&
				is.EndedAt == nil &&
				strings.HasPrefix(is.SessionUUID, headlessTriageUUIDPrefix) &&
				time.Since(is.CreatedAt) > maxTriageSessionAge {
				_ = s.storage.UpdateItemSessionEnded(ctx, is.ID, now)
				is.EndedAt = &now
			}
		}
		item.ItemSessions = isSessions
	}

	p := backlogItemToProto(item, s.buildCostLookup())
	// Populate worktree_branch/worktree_path for each linked work session.
	for _, is := range p.ItemSessions {
		if is.SessionUuid == "" {
			continue
		}
		wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, is.SessionUuid)
		if wtErr == nil && wt.BranchName != "" {
			is.WorktreeBranch = wt.BranchName
		}
		if wtErr == nil && wt.WorktreePath != "" {
			is.WorktreePath = wt.WorktreePath
		}
	}

	return connect.NewResponse(&sessionv1.GetBacklogItemResponse{
		Item: p,
	}), nil
}

// --- ListBacklogItems ---

// ListBacklogItems returns backlog items with optional filtering and sorting.
// +api: backlog:list-items
func (s *BacklogService) ListBacklogItems(
	ctx context.Context,
	req *connect.Request[sessionv1.ListBacklogItemsRequest],
) (*connect.Response[sessionv1.ListBacklogItemsResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.ListBacklogItemsResponse{}), nil
	}

	filter := session.BacklogItemFilter{
		SortBy:          req.Msg.SortBy,
		ExcludeTerminal: !req.Msg.IncludeTerminal,
	}
	if len(req.Msg.Status) > 0 {
		filter.Statuses = req.Msg.Status
		filter.ExcludeTerminal = false // explicit status filter overrides default exclusion
	}
	if len(req.Msg.Priority) > 0 {
		priorities := make([]int, len(req.Msg.Priority))
		for i, p := range req.Msg.Priority {
			priorities[i] = int(p)
		}
		filter.Priorities = priorities
	}

	summaries, err := s.storage.ListBacklogItemSummaries(ctx, filter)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list backlog items: %w", err))
	}

	protoItems := make([]*sessionv1.BacklogItem, len(summaries))
	costFor := s.buildCostLookup()
	for i := range summaries {
		protoItems[i] = backlogItemSummaryToProto(&summaries[i], costFor)
	}

	return connect.NewResponse(&sessionv1.ListBacklogItemsResponse{
		Items: protoItems,
	}), nil
}

// ListItemSources returns all registered external item sources.
// +api: backlog:list-sources
func (s *BacklogService) ListItemSources(
	ctx context.Context,
	req *connect.Request[sessionv1.ListItemSourcesRequest],
) (*connect.Response[sessionv1.ListItemSourcesResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.ListItemSourcesResponse{}), nil
	}

	sources, err := s.storage.ListItemSources(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sources: %w", err))
	}

	protoSources := make([]*sessionv1.ItemSource, len(sources))
	for i := range sources {
		protoSources[i] = itemSourceToProto(&sources[i])
	}

	return connect.NewResponse(&sessionv1.ListItemSourcesResponse{
		Sources: protoSources,
	}), nil
}

// SuggestNextItem recommends the highest-priority ready backlog item.
// +api: backlog:suggest-next
func (s *BacklogService) SuggestNextItem(
	ctx context.Context,
	_ *connect.Request[sessionv1.SuggestNextItemRequest],
) (*connect.Response[sessionv1.SuggestNextItemResponse], error) {
	if s.storage == nil {
		return connect.NewResponse(&sessionv1.SuggestNextItemResponse{}), nil
	}

	// Load ready items ordered by priority (lower number = higher priority).
	items, err := s.storage.ListBacklogItems(ctx, session.BacklogItemFilter{
		Statuses: []string{string(session.BacklogStatusReady)},
		SortBy:   "priority",
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list backlog items: %w", err))
	}

	if len(items) == 0 {
		// No ready items — return empty response.
		return connect.NewResponse(&sessionv1.SuggestNextItemResponse{}), nil
	}

	top := &items[0]
	return connect.NewResponse(&sessionv1.SuggestNextItemResponse{
		Item: backlogItemToProto(top, s.buildCostLookup()),
	}), nil
}

// GetSyncHistory returns the sync event history for an item source, most
// recent first.
// +api: backlog:get-sync-history
func (s *BacklogService) GetSyncHistory(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSyncHistoryRequest],
) (*connect.Response[sessionv1.GetSyncHistoryResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.SourceId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("source_id is required"))
	}
	if _, parseErr := uuid.Parse(req.Msg.SourceId); parseErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid source_id %q: %w", req.Msg.SourceId, parseErr))
	}

	events, truncated, err := s.storage.ListSourceSyncEvents(ctx, req.Msg.SourceId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sync history: %w", err))
	}

	protoEvents := make([]*sessionv1.SourceSyncEvent, 0, len(events))
	for _, ev := range events {
		protoEvents = append(protoEvents, sourceSyncEventToProto(ev))
	}

	return connect.NewResponse(&sessionv1.GetSyncHistoryResponse{Events: protoEvents, Truncated: truncated}), nil
}

func (s *BacklogService) SearchGitHubRepos(ctx context.Context, req *connect.Request[sessionv1.SearchGitHubReposRequest]) (*connect.Response[sessionv1.SearchGitHubReposResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 30
	}
	results, err := gh.SearchUserRepos(ctx, req.Msg.Query, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("search repos: %w", err))
	}
	entries := make([]*sessionv1.GitHubRepoEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, &sessionv1.GitHubRepoEntry{
			Owner:       r.Owner,
			Repo:        r.Repo,
			Description: r.Description,
		})
	}
	return connect.NewResponse(&sessionv1.SearchGitHubReposResponse{Repos: entries}), nil
}

func (s *BacklogService) ListGitHubIssues(ctx context.Context, req *connect.Request[sessionv1.ListGitHubIssuesRequest]) (*connect.Response[sessionv1.ListGitHubIssuesResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.Owner == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("owner is required"))
	}
	if req.Msg.Repo == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repo is required"))
	}
	if strings.ContainsAny(req.Msg.Owner, " \t\n/") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("owner contains invalid characters"))
	}
	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 30
	}
	results, err := gh.ListRepoIssues(ctx, req.Msg.Owner, req.Msg.Repo, req.Msg.State, req.Msg.Search, limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list issues: %w", err))
	}
	entries := make([]*sessionv1.GitHubIssueEntry, 0, len(results))
	for _, r := range results {
		entries = append(entries, &sessionv1.GitHubIssueEntry{
			Number: int32(r.Number),
			Title:  r.Title,
			State:  r.State,
			Url:    r.URL,
			Labels: r.Labels,
		})
	}
	return connect.NewResponse(&sessionv1.ListGitHubIssuesResponse{Issues: entries}), nil
}

func (s *BacklogService) GetBacklogItemDiff(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBacklogItemDiffRequest],
) (*connect.Response[sessionv1.GetBacklogItemDiffResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}
	if item.RepoPath == "" {
		return connect.NewResponse(&sessionv1.GetBacklogItemDiffResponse{}), nil
	}

	sessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}

	// An item's branch can carry more than one work session (rework/reopen
	// cycles): session 1 does the real implementation, review sends it back,
	// session 2 picks up the same branch to address feedback, etc. The diff
	// must reflect everything committed across ALL of those sessions, so:
	//   - diffBaseSHA comes from the EARLIEST work session's BaseCommitSHA —
	//     the point where the branch actually diverged, before any rework
	//     cycle began. Using the most recent session's own BaseCommitSHA is
	//     wrong whenever that latest session made zero new commits (e.g. a
	//     reopen that just re-verifies already-complete work and exits): its
	//     BaseCommitSHA was captured at that session's spawn time, which by
	//     definition already equals the current branch tip, so base == head
	//     and the diff comes back empty even though earlier sessions on the
	//     same branch carry substantial real, already-committed work.
	//   - headRef still comes from the MOST RECENT work session so it always
	//     resolves to the branch's actual current tip (see the wt.BranchName
	//     rationale below).
	// ListItemSessions returns sessions ordered oldest-first (see
	// EntRepository.ListItemSessions), but we compare CreatedAt explicitly
	// rather than depending on that ordering.
	var earliestWorkSession, mostRecentWorkSession *session.ItemSessionSummary
	for i := range sessions {
		is := &sessions[i]
		if is.Role != session.SessionRoleWork {
			continue
		}
		if earliestWorkSession == nil || is.CreatedAt.Before(earliestWorkSession.CreatedAt) {
			earliestWorkSession = is
		}
		if mostRecentWorkSession == nil || is.CreatedAt.After(mostRecentWorkSession.CreatedAt) {
			mostRecentWorkSession = is
		}
	}
	diffBaseSHA := "HEAD~1"
	var headRef string
	if earliestWorkSession != nil {
		if wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, earliestWorkSession.SessionUUID); wtErr == nil {
			if wt.BaseCommitSHA != "" {
				diffBaseSHA = wt.BaseCommitSHA
			}
		}
	}
	if mostRecentWorkSession != nil {
		if wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, mostRecentWorkSession.SessionUUID); wtErr == nil {
			// Prefer the branch name over the session's LastCommitSha. LastCommitSha
			// is only ever written once, at session spawn, to the PRE-work base
			// commit (see AttachSessionToItem / SpawnSessionFromItem step 12b) —
			// nothing updates it as the agent makes further commits during the
			// session, so in practice it is usually identical to diffBaseSHA itself,
			// producing a spurious empty base..head diff ("No changes to display")
			// for items that genuinely have real, already-reviewed work on their
			// branch (e.g. a Review-status item with a full Gate Verdict on record).
			// wt.BranchName always resolves to the branch's actual current tip —
			// worktrees share one object store, so this works whether or not the
			// session's own worktree directory still exists — the same fallback
			// review_gate.go's spawnReviewGate already relies on via
			// GetGitDiffRef(item.RepoPath, wt.BaseCommitSHA, wt.BranchName).
			if wt.BranchName != "" {
				headRef = wt.BranchName
			} else if mostRecentWorkSession.LastCommitSha != "" {
				headRef = mostRecentWorkSession.LastCommitSha
			}
		}
	}

	// Always diff from item.RepoPath (the shared, stable checkout) with an
	// explicit headRef, never the work session's own worktree path — worktrees
	// share the same object store, so any commit sha reachable from any
	// worktree of the repo resolves correctly regardless of dir. This is what
	// makes the diff work identically whether the work session's worktree
	// directory still exists or has already been cleaned up (the normal state
	// once an item is done) — see GetGitDiffRef's doc comment.
	diffContent, _, diffErr := session.GetGitDiffRef(ctx, item.RepoPath, diffBaseSHA, headRef)
	if diffErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to compute diff: %w", diffErr))
	}

	var added, removed int32
	for _, line := range strings.Split(diffContent, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			added++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			removed++
		}
	}

	return connect.NewResponse(&sessionv1.GetBacklogItemDiffResponse{
		Diff:    diffContent,
		Added:   added,
		Removed: removed,
	}), nil
}

// GetBacklogItemCost returns estimated token costs for all sessions linked to an item.
func (s *BacklogService) GetBacklogItemCost(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBacklogItemCostRequest],
) (*connect.Response[sessionv1.GetBacklogItemCostResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	if req.Msg.ItemId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}

	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) || errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}

	itemSessions, err := s.storage.ListItemSessions(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list item sessions: %w", err))
	}

	resp := &sessionv1.GetBacklogItemCostResponse{}
	if s.tokenStore == nil || s.pricing == nil {
		return connect.NewResponse(resp), nil
	}

	// Build tmux-UUID → conversation-UUID map once; TokenStore is keyed by
	// conversation UUID (JSONL filename), not by tmux session UUID.
	convIDByTmux := make(map[string]string)
	for _, rec := range s.storage.ListSessionRecords() {
		if rec.SessionID != "" && rec.ConversationID != "" {
			convIDByTmux[rec.SessionID] = rec.ConversationID
		}
	}

	for _, is := range itemSessions {
		if is.SessionUUID == "" {
			continue
		}
		convID := convIDByTmux[is.SessionUUID]
		if convID == "" {
			continue
		}
		result := s.tokenStore.GetByUUID(convID)
		if result == nil {
			continue
		}
		cost := s.pricing.EstimateCost(result)
		resp.TotalCostUsd += cost
		resp.Sessions = append(resp.Sessions, &sessionv1.SessionCostEntry{
			SessionId:        is.SessionUUID,
			SessionRole:      is.Role,
			EstimatedCostUsd: cost,
			InputTokens:      int64(result.TotalInput),
			OutputTokens:     int64(result.TotalOutput),
		})
	}

	return connect.NewResponse(resp), nil
}

// GetSessionBacklogIndex returns a flat list of all item sessions with their parent backlog
// item metadata, keyed by session UUID. Used by the Insights dashboard to annotate sessions.
func (s *BacklogService) GetSessionBacklogIndex(
	ctx context.Context,
	_ *connect.Request[sessionv1.GetSessionBacklogIndexRequest],
) (*connect.Response[sessionv1.GetSessionBacklogIndexResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}

	entries, err := s.storage.GetAllItemSessionsWithBacklogInfo(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to query session backlog index: %w", err))
	}

	protoEntries := make([]*sessionv1.BacklogSessionEntry, 0, len(entries))
	for _, e := range entries {
		protoEntries = append(protoEntries, &sessionv1.BacklogSessionEntry{
			SessionUuid: e.SessionUUID,
			ItemId:      e.ItemID,
			ItemTitle:   e.ItemTitle,
			ItemStatus:  e.ItemStatus,
			SessionRole: e.SessionRole,
		})
	}

	return connect.NewResponse(&sessionv1.GetSessionBacklogIndexResponse{
		Entries: protoEntries,
	}), nil
}
