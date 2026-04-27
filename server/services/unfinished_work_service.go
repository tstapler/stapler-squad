package services

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session/unfinished"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Compile-time check: UnfinishedWorkService must implement the generated handler.
var _ sessionv1connect.UnfinishedWorkServiceHandler = (*UnfinishedWorkService)(nil)

// aiSemaphore limits concurrent Claude AI subprocess calls globally.
var aiSemaphore = make(chan struct{}, 2)

// UnfinishedWorkService implements the ConnectRPC UnfinishedWorkServiceHandler.
type UnfinishedWorkService struct {
	scanner    *unfinished.Scanner
	stateStore *unfinished.StateStore
	eventBus   *events.EventBus

	// perWorktreeMu prevents duplicate AI summary generation for the same worktree.
	aiMu sync.Map // map[string]*sync.Mutex  key = repoPath+"|"+branch
}

// NewUnfinishedWorkService creates a new service instance.
func NewUnfinishedWorkService(
	scanner *unfinished.Scanner,
	stateStore *unfinished.StateStore,
	eventBus *events.EventBus,
) *UnfinishedWorkService {
	return &UnfinishedWorkService{
		scanner:    scanner,
		stateStore: stateStore,
		eventBus:   eventBus,
	}
}

// ListUnfinishedWork returns the current snapshot of all unfinished worktrees.
func (s *UnfinishedWorkService) ListUnfinishedWork(
	_ context.Context,
	_ *connect.Request[sessionv1.ListUnfinishedWorkRequest],
) (*connect.Response[sessionv1.ListUnfinishedWorkResponse], error) {
	results := s.scanner.GetAllResults()
	worktrees := make([]*sessionv1.UnfinishedWorktree, 0, len(results))
	for _, r := range results {
		worktrees = append(worktrees, scanResultToProto(r))
	}
	return connect.NewResponse(&sessionv1.ListUnfinishedWorkResponse{
		Worktrees: worktrees,
		LastScan:  timestamppb.Now(),
	}), nil
}

// WatchUnfinishedWork streams real-time updates to connected clients.
// Pattern: send initial snapshot → subscribe to EventBus → forward events until disconnect.
func (s *UnfinishedWorkService) WatchUnfinishedWork(
	ctx context.Context,
	_ *connect.Request[sessionv1.WatchUnfinishedWorkRequest],
	stream *connect.ServerStream[sessionv1.UnfinishedWorkEvent],
) error {
	// 1. Send initial snapshot.
	results := s.scanner.GetAllResults()
	for _, r := range results {
		evt := &sessionv1.UnfinishedWorkEvent{
			Payload: &sessionv1.UnfinishedWorkEvent_WorktreeUpdated{
				WorktreeUpdated: scanResultToProto(r),
			},
		}
		if err := stream.Send(evt); err != nil {
			return fmt.Errorf("send initial snapshot: %w", err)
		}
	}

	// 2. Subscribe to EventBus.
	eventCh, subID := s.eventBus.Subscribe(ctx)
	defer s.eventBus.Unsubscribe(subID)

	// 3. Forward events.
	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-eventCh:
			if !ok {
				return nil
			}
			protoEvent := s.convertUnfinishedEvent(event)
			if protoEvent == nil {
				continue
			}
			if err := stream.Send(protoEvent); err != nil {
				return fmt.Errorf("send event: %w", err)
			}
		}
	}
}

// convertUnfinishedEvent converts an internal EventBus event to a proto UnfinishedWorkEvent.
func (s *UnfinishedWorkService) convertUnfinishedEvent(evt *events.Event) *sessionv1.UnfinishedWorkEvent {
	switch evt.Type {
	case unfinished.EventUnfinishedWorkUpdated:
		// The Context field carries "repoPath|branch".
		parts := strings.SplitN(evt.Context, "|", 2)
		if len(parts) != 2 {
			return nil
		}
		r, ok := s.scanner.GetResultByKey(parts[0], parts[1])
		if !ok {
			return nil
		}
		return &sessionv1.UnfinishedWorkEvent{
			Payload: &sessionv1.UnfinishedWorkEvent_WorktreeUpdated{
				WorktreeUpdated: scanResultToProto(r),
			},
		}
	case unfinished.EventUnfinishedWorkRemoved:
		parts := strings.SplitN(evt.Context, "|", 2)
		if len(parts) != 2 {
			return nil
		}
		return &sessionv1.UnfinishedWorkEvent{
			Payload: &sessionv1.UnfinishedWorkEvent_WorktreeRemoved{
				WorktreeRemoved: &sessionv1.UnfinishedWorktree{
					RepoPath: parts[0],
					Branch:   parts[1],
				},
			},
		}
	case unfinished.EventUnfinishedScanCompleted:
		return &sessionv1.UnfinishedWorkEvent{
			Payload: &sessionv1.UnfinishedWorkEvent_ScanCompleted{
				ScanCompleted: &sessionv1.ScanCompleted{
					CompletedAt: timestamppb.New(evt.Timestamp),
				},
			},
		}
	}
	return nil
}

// ScanUnfinishedWork triggers an immediate scan.
func (s *UnfinishedWorkService) ScanUnfinishedWork(
	_ context.Context,
	_ *connect.Request[sessionv1.ScanUnfinishedWorkRequest],
) (*connect.Response[sessionv1.ScanUnfinishedWorkResponse], error) {
	startedAt := time.Now()
	s.scanner.TriggerScan()
	return connect.NewResponse(&sessionv1.ScanUnfinishedWorkResponse{
		ScanStartedAt: timestamppb.New(startedAt),
	}), nil
}

// DismissWorktree permanently hides a worktree from results.
func (s *UnfinishedWorkService) DismissWorktree(
	_ context.Context,
	req *connect.Request[sessionv1.DismissWorktreeRequest],
) (*connect.Response[sessionv1.DismissWorktreeResponse], error) {
	if err := s.stateStore.Dismiss(req.Msg.RepoPath, req.Msg.Branch); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.scanner.RemoveResult(req.Msg.RepoPath, req.Msg.Branch)
	s.eventBus.Publish(newUnfinishedRemovedEvent(req.Msg.RepoPath, req.Msg.Branch))
	return connect.NewResponse(&sessionv1.DismissWorktreeResponse{}), nil
}

// UndismissWorktree removes the dismiss record.
func (s *UnfinishedWorkService) UndismissWorktree(
	_ context.Context,
	req *connect.Request[sessionv1.UndismissWorktreeRequest],
) (*connect.Response[sessionv1.UndismissWorktreeResponse], error) {
	if err := s.stateStore.Undismiss(req.Msg.RepoPath, req.Msg.Branch); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	// Trigger re-scan so the item reappears.
	s.scanner.TriggerScan()
	return connect.NewResponse(&sessionv1.UndismissWorktreeResponse{}), nil
}

// SnoozeWorktree hides a worktree until the next HEAD SHA change.
func (s *UnfinishedWorkService) SnoozeWorktree(
	_ context.Context,
	req *connect.Request[sessionv1.SnoozeWorktreeRequest],
) (*connect.Response[sessionv1.SnoozeWorktreeResponse], error) {
	// Get current HEAD SHA from the stored scan result.
	r, ok := s.scanner.GetResultByKey(req.Msg.RepoPath, req.Msg.Branch)
	var headSHA string
	if ok {
		// Run git rev-parse HEAD in the worktree to get current SHA.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "git", "-C", r.WorktreePath, "rev-parse", "HEAD")
		out, err := cmd.Output()
		if err == nil {
			headSHA = strings.TrimSpace(string(out))
		}
	}

	if err := s.stateStore.Snooze(req.Msg.RepoPath, req.Msg.Branch, headSHA); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.scanner.RemoveResult(req.Msg.RepoPath, req.Msg.Branch)
	s.eventBus.Publish(newUnfinishedRemovedEvent(req.Msg.RepoPath, req.Msg.Branch))
	return connect.NewResponse(&sessionv1.SnoozeWorktreeResponse{}), nil
}

// GetWorktreeAISummary generates or returns a cached AI summary.
func (s *UnfinishedWorkService) GetWorktreeAISummary(
	ctx context.Context,
	req *connect.Request[sessionv1.GetWorktreeAISummaryRequest],
) (*connect.Response[sessionv1.GetWorktreeAISummaryResponse], error) {
	r, ok := s.scanner.GetResultByKey(req.Msg.RepoPath, req.Msg.Branch)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("worktree not found: %s|%s", req.Msg.RepoPath, req.Msg.Branch))
	}

	// Compute diff hash.
	diffHash, err := unfinished.ComputeDiffHash(r.WorktreePath)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("compute diff hash: %w", err))
	}

	// Check cache.
	if summary, ok := s.stateStore.GetCachedSummary(req.Msg.RepoPath, req.Msg.Branch, diffHash); ok {
		return connect.NewResponse(&sessionv1.GetWorktreeAISummaryResponse{
			Summary:   summary,
			FromCache: true,
		}), nil
	}

	// Per-worktree mutex to avoid duplicate concurrent calls.
	key := req.Msg.RepoPath + "|" + req.Msg.Branch
	muI, _ := s.aiMu.LoadOrStore(key, &sync.Mutex{})
	mu := muI.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Re-check cache after acquiring lock.
	if summary, ok := s.stateStore.GetCachedSummary(req.Msg.RepoPath, req.Msg.Branch, diffHash); ok {
		return connect.NewResponse(&sessionv1.GetWorktreeAISummaryResponse{
			Summary:   summary,
			FromCache: true,
		}), nil
	}

	// Acquire global semaphore.
	select {
	case aiSemaphore <- struct{}{}:
	case <-ctx.Done():
		return nil, connect.NewError(connect.CodeCanceled, ctx.Err())
	}
	defer func() { <-aiSemaphore }()

	// Find claude CLI.
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("claude CLI not found: %w", err))
	}

	// Run: git diff HEAD | claude -p "Summarize these git changes in 2-4 sentences."
	subCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	gitCmd := exec.CommandContext(subCtx, "git", "-C", r.WorktreePath, "diff", "HEAD")
	claudeCmd := exec.CommandContext(subCtx, claudePath, "-p",
		"Summarize these git changes in 2-4 sentences for a developer picking up where they left off.")

	gitOut, gitErr := gitCmd.Output()
	if gitErr != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("git diff HEAD: %w", gitErr))
	}

	claudeCmd.Stdin = strings.NewReader(string(gitOut))
	summaryOut, claudeErr := claudeCmd.Output()
	if claudeErr != nil {
		if subCtx.Err() != nil {
			return nil, connect.NewError(connect.CodeDeadlineExceeded,
				fmt.Errorf("AI summary timed out after 30s"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("claude CLI error: %w", claudeErr))
	}

	summary := strings.TrimSpace(string(summaryOut))
	if err := s.stateStore.CacheSummary(req.Msg.RepoPath, req.Msg.Branch, diffHash, summary); err != nil {
		log.WarningLog.Printf("[unfinished] failed to cache AI summary: %v", err)
	}

	return connect.NewResponse(&sessionv1.GetWorktreeAISummaryResponse{
		Summary:   summary,
		FromCache: false,
	}), nil
}

// QuickCommitPush stages all changes, commits, and pushes.
func (s *UnfinishedWorkService) QuickCommitPush(
	_ context.Context,
	req *connect.Request[sessionv1.QuickCommitPushRequest],
) (*connect.Response[sessionv1.QuickCommitPushResponse], error) {
	if strings.TrimSpace(req.Msg.CommitMessage) == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("commit message is required"))
	}

	r, ok := s.scanner.GetResultByKey(req.Msg.RepoPath, req.Msg.Branch)
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("worktree not found: %s|%s", req.Msg.RepoPath, req.Msg.Branch))
	}

	worktreePath := r.WorktreePath

	// git add .
	addCmd := exec.Command("git", "-C", worktreePath, "add", ".")
	if out, err := addCmd.CombinedOutput(); err != nil {
		return connect.NewResponse(&sessionv1.QuickCommitPushResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("git add failed: %v\n%s", err, out),
		}), nil
	}

	// git commit -m <message>
	commitCmd := exec.Command("git", "-C", worktreePath, "commit", "-m", req.Msg.CommitMessage)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		return connect.NewResponse(&sessionv1.QuickCommitPushResponse{
			Success:      false,
			ErrorMessage: fmt.Sprintf("git commit failed: %v\n%s", err, out),
		}), nil
	}

	// git push -u origin <branch> (60s timeout)
	pushCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pushCmd := exec.CommandContext(pushCtx, "git", "-C", worktreePath, "push", "-u", "origin", req.Msg.Branch)
	if out, err := pushCmd.CombinedOutput(); err != nil {
		errMsg := fmt.Sprintf("git push failed: %v\n%s", err, out)
		if pushCtx.Err() != nil {
			errMsg = "git push timed out after 60s"
		}
		return connect.NewResponse(&sessionv1.QuickCommitPushResponse{
			Success:      false,
			ErrorMessage: errMsg,
		}), nil
	}

	// Trigger a re-scan to update the item.
	s.scanner.InvalidateCache(worktreePath)
	s.scanner.EnqueueRepo(req.Msg.RepoPath)

	return connect.NewResponse(&sessionv1.QuickCommitPushResponse{
		Success: true,
	}), nil
}

// GetUnfinishedWorkConfig returns the current source configuration.
func (s *UnfinishedWorkService) GetUnfinishedWorkConfig(
	_ context.Context,
	_ *connect.Request[sessionv1.GetUnfinishedWorkConfigRequest],
) (*connect.Response[sessionv1.GetUnfinishedWorkConfigResponse], error) {
	return connect.NewResponse(&sessionv1.GetUnfinishedWorkConfigResponse{
		Config: &sessionv1.UnfinishedWorkConfig{
			AutoSpiderSessions: s.stateStore.AutoSpiderEnabled(),
			WatchDirs:          s.stateStore.WatchDirs(),
			PinnedRepos:        s.stateStore.PinnedRepos(),
		},
	}), nil
}

// UpdateUnfinishedWorkConfig replaces the source configuration.
func (s *UnfinishedWorkService) UpdateUnfinishedWorkConfig(
	_ context.Context,
	req *connect.Request[sessionv1.UpdateUnfinishedWorkConfigRequest],
) (*connect.Response[sessionv1.UpdateUnfinishedWorkConfigResponse], error) {
	cfg := req.Msg.Config
	if cfg == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("config is required"))
	}

	// Validate pinned repos exist.
	for _, repo := range cfg.PinnedRepos {
		if err := s.scanner.AddPinnedRepo(repo); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid pinned repo %q: %w", repo, err))
		}
	}

	if err := s.stateStore.SetConfig(cfg.AutoSpiderSessions, cfg.WatchDirs, cfg.PinnedRepos); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	s.scanner.SetAutoSpider(cfg.AutoSpiderSessions)

	// Trigger scan to pick up new repos.
	s.scanner.TriggerScan()

	return connect.NewResponse(&sessionv1.UpdateUnfinishedWorkConfigResponse{
		Config: cfg,
	}), nil
}

// --- helpers ---

func scanResultToProto(r unfinished.ScanResult) *sessionv1.UnfinishedWorktree {
	wt := &sessionv1.UnfinishedWorktree{
		RepoPath:            r.RepoPath,
		Branch:              r.Branch,
		WorktreePath:        r.WorktreePath,
		RepoName:            r.RepoName,
		DisplayPath:         r.DisplayPath,
		HasUncommitted:      r.HasUncommitted,
		CommitsAhead:        int32(r.AheadCount),
		CommitsBehind:       int32(r.BehindCount),
		DefaultBranch:       r.DefaultBranch,
		ChangedFiles:        int32(r.ChangedFiles),
		LinesAdded:          int32(r.LinesAdded),
		LinesRemoved:        int32(r.LinesRemoved),
		AheadCommitMessages: r.AheadMessages,
		IsDismissed:         r.Status == unfinished.ScanResultStatusError, // used below
		SessionId:           r.SessionID,
	}

	// Correct the is_dismissed field (ScanResult doesn't carry this).
	wt.IsDismissed = false

	switch r.Status {
	case unfinished.ScanResultStatusOK:
		wt.ScanStatus = sessionv1.ScanStatus_SCAN_STATUS_OK
	case unfinished.ScanResultStatusTimeout:
		wt.ScanStatus = sessionv1.ScanStatus_SCAN_STATUS_TIMEOUT
		wt.ScanErrorMsg = r.ErrorMsg
	case unfinished.ScanResultStatusPermission:
		wt.ScanStatus = sessionv1.ScanStatus_SCAN_STATUS_PERMISSION
		wt.ScanErrorMsg = r.ErrorMsg
	case unfinished.ScanResultStatusError:
		wt.ScanStatus = sessionv1.ScanStatus_SCAN_STATUS_ERROR
		wt.ScanErrorMsg = r.ErrorMsg
	}

	if !r.LastModified.IsZero() {
		wt.LastModified = timestamppb.New(r.LastModified)
	}
	if !r.ScanTime.IsZero() {
		wt.ScanTime = timestamppb.New(r.ScanTime)
	}
	return wt
}

func newUnfinishedRemovedEvent(repoPath, branch string) *events.Event {
	return &events.Event{
		Type:      unfinished.EventUnfinishedWorkRemoved,
		Timestamp: time.Now(),
		Context:   repoPath + "|" + branch,
	}
}
