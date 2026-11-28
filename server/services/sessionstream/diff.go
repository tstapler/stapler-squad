package sessionstream

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/git"
)

// maxContextLines is the largest literal context_lines honored; anything above is treated as the
// whole file, which keeps a client-chosen value from meaning more than "everything".
const maxContextLines = 10_000

// normalizeContextLines validates GetSessionDiffRequest.context_lines: 0 = server default,
// -1 = whole file, positive = literal count (capped at maxContextLines => whole file).
func normalizeContextLines(n int32) (int, error) {
	switch {
	case n < -1:
		return 0, fmt.Errorf("context_lines must be -1 (whole file), 0 (default) or positive, got %d", n)
	case n > maxContextLines:
		return -1, nil
	default:
		return int(n), nil
	}
}

// GetSessionDiff retrieves the current git diff for a session.
func (h *Handler) GetSessionDiff(
	ctx context.Context,
	req *connect.Request[sessionv1.GetSessionDiffRequest],
) (*connect.Response[sessionv1.GetSessionDiffResponse], error) {
	if req.Msg.Id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("session id is required"))
	}

	var diffStats *git.DiffStats

	contextLines, err := normalizeContextLines(req.Msg.ContextLines)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	customContext := contextLines != 0 // 0 = server default, served from the cached diff

	// The git diff below takes no ctx, so don't start it for a call already gone.
	if err := ctx.Err(); err != nil {
		return nil, connect.NewError(connect.CodeCanceled, err)
	}

	instance := h.ports.FindInstance(req.Msg.Id)
	if instance != nil {
		if wt, err := instance.GetGitWorktree(); customContext && err == nil && wt != nil {
			// Non-default context bypasses the shared diff-stats cache, which only holds the default.
			diffStats = wt.DiffWithContextLines(contextLines)
			if diffStats.Error != nil {
				log.Warn("failed to compute diff with custom context", "session", req.Msg.Id, "context_lines", contextLines, "err", diffStats.Error)
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to compute diff: %w", diffStats.Error))
			}
		} else {
			// Live session: refresh (if the cached value is stale) and read.
			if err := instance.RefreshDiffStatsIfStale(); err != nil {
				log.Warn("failed to update diff stats", "session", req.Msg.Id, "err", err)
			}
			diffStats = instance.GetDiffStats()
		}
	} else {
		// Completed session: reconstruct worktree from DB and compute diff on-demand.
		allData, err := h.ports.ListInstanceData()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list sessions: %w", err))
		}
		var found *domain.InstanceData
		for i := range allData {
			if allData[i].MatchesID(req.Msg.Id) {
				found = &allData[i]
				break
			}
		}
		if found == nil {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", req.Msg.Id))
		}
		if found.Worktree.WorktreePath != "" {
			wt := git.NewGitWorktreeFromStorage(
				found.Worktree.RepoPath,
				found.Worktree.WorktreePath,
				found.Worktree.SessionName,
				found.Worktree.BranchName,
				found.Worktree.BaseCommitSHA,
			)
			diffStats = wt.DiffWithContextLines(contextLines)
		} else if found.Path != "" {
			// ponytail: directory sessions have no worktree; use the session path directly.
			// resolveBaseCommitSHA() will find the merge-base with main/master as fallback.
			wt := git.NewGitWorktreeFromStorage(found.Path, found.Path, found.Title, "", "")
			if wt != nil {
				diffStats = wt.DiffWithContextLines(contextLines)
			}
		}
	}

	if diffStats == nil {
		return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
			DiffStats: &sessionv1.DiffStats{},
		}), nil
	}

	return connect.NewResponse(&sessionv1.GetSessionDiffResponse{
		DiffStats: &sessionv1.DiffStats{
			Added:   int32(diffStats.Added),   //#nosec G115 -- diff line count, bounded well under int32 max
			Removed: int32(diffStats.Removed), //#nosec G115 -- diff line count, bounded well under int32 max
			Content: diffStats.Content,
		},
	}), nil
}
