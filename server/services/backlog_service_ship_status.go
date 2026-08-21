package services

// backlog_service_ship_status.go — GetBacklogItemShipStatus, a read-only RPC
// answering "did this item's code actually ship" from durable evidence
// (repo_path + the most recent work session's commit) rather than a live
// per-session worktree. The existing GetVCSStatus RPC requires a live
// in-memory Instance (findInstanceFast) and returns nothing once a session's
// worktree has been cleaned up — which is exactly the normal end state for a
// done item, not a failure. This RPC works from durable data instead.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/git"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GetBacklogItemShipStatus reports whether itemID's code actually landed on
// main, plus the branch's position relative to main when the branch still
// exists.
// +api: backlog:get-item-ship-status
func (s *BacklogService) GetBacklogItemShipStatus(
	ctx context.Context,
	req *connect.Request[sessionv1.GetBacklogItemShipStatusRequest],
) (*connect.Response[sessionv1.GetBacklogItemShipStatusResponse], error) {
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

	itemSessions, err := s.storage.ListItemSessions(ctx, req.Msg.ItemId)
	if err != nil {
		return connect.NewResponse(&sessionv1.GetBacklogItemShipStatusResponse{
			Status: &sessionv1.BacklogItemShipStatus{Error: fmt.Sprintf("failed to load item sessions: %v", err)},
		}), nil
	}

	var lastWork *session.ItemSessionSummary
	for i := range itemSessions {
		// Ascending by CreatedAt (ListItemSessions' query order) — keep
		// overwriting so this ends up holding the *most recent* work session.
		if itemSessions[i].Role == session.SessionRoleWork {
			lastWork = &itemSessions[i]
		}
	}
	if lastWork == nil {
		return connect.NewResponse(&sessionv1.GetBacklogItemShipStatusResponse{
			Status: &sessionv1.BacklogItemShipStatus{Error: "no work session ever committed code for this item"},
		}), nil
	}

	// Resolve the session's real current tip rather than trusting the stored
	// field, and never answer "shipped" from the pre-work base commit — it is
	// always an ancestor of main by construction, so it would make this RPC (and
	// the Ship PR button it backs) report every item as already shipped. Same
	// BUG-047 trust boundary closeIfSupersededByMain now enforces; see
	// resolveLatestWorkCommit's doc comment.
	lastCommitSha := s.resolveLatestWorkCommit(ctx, lastWork.SessionUUID, item.RepoPath)
	if lastCommitSha == "" {
		lastCommitSha = lastWork.LastCommitSha
	}
	if lastCommitSha == "" || lastCommitSha == lastWork.BaseCommitSha {
		return connect.NewResponse(&sessionv1.GetBacklogItemShipStatusResponse{
			Status: &sessionv1.BacklogItemShipStatus{Error: "no work session ever committed code for this item"},
		}), nil
	}

	status := &sessionv1.BacklogItemShipStatus{
		PrUrl:             item.PrURL,
		LastCommitSha:     lastCommitSha,
		LastCommitMessage: lastWork.LastCommitMessage,
	}
	if lastWork.LastCommitAt != nil {
		status.LastCommitAt = timestamppb.New(*lastWork.LastCommitAt)
	}
	// The SHA above is resolved live, but the message/timestamp beside it come
	// from a row only refreshed on a reconciler tick. Between a new commit and
	// the next tick those disagree — a new SHA captioned with the *previous*
	// commit's message. Read them from the commit itself so the three always
	// describe one commit.
	if lastCommitSha != lastWork.LastCommitSha {
		if info, infoErr := git.CommitInfo(item.RepoPath, lastCommitSha); infoErr == nil {
			status.LastCommitMessage = info.Summary
			status.LastCommitAt = timestamppb.New(info.AuthorAt)
		}
	}

	onMain, mainErr := git.IsCommitOnMain(item.RepoPath, prFixMainBranch, lastCommitSha)
	if mainErr != nil {
		status.Error = fmt.Sprintf("failed to verify commit on main: %v", mainErr)
		return connect.NewResponse(&sessionv1.GetBacklogItemShipStatusResponse{Status: status}), nil
	}
	status.Shipped = onMain
	if onMain {
		if item.PrURL != "" {
			status.ShippedVia = "pr"
		} else {
			status.ShippedVia = "direct"
		}
	}

	wt, wtErr := s.storage.GetWorktreeDataBySessionUUID(ctx, lastWork.SessionUUID)
	if wtErr == nil && wt.BranchName != "" {
		status.BranchName = wt.BranchName
		branchStatus, branchErr := git.BranchAheadBehind(item.RepoPath, wt.BranchName, prFixMainBranch)
		if branchErr != nil {
			status.Error = fmt.Sprintf("failed to check branch status: %v", branchErr)
		} else {
			status.BranchExists = branchStatus.BranchExists
			status.AheadOfMain = int32(branchStatus.AheadOfMain) //#nosec G115 -- bounded by countCommitsNotAncestorOfCap (500)
			status.BehindMain = int32(branchStatus.BehindMain)   //#nosec G115 -- bounded by countCommitsNotAncestorOfCap (500)
		}
	}

	if wtErr == nil && wt.BaseCommitSHA != "" {
		shipped, commitsErr := git.ListShippedCommits(item.RepoPath, wt.BaseCommitSHA, lastCommitSha)
		if commitsErr != nil {
			// Non-fatal: the badge/branch info above is still valid even if the
			// commit list itself can't be resolved (e.g. the base SHA has since
			// been pruned) — don't let this clobber status.Error's more useful
			// message from the branch-status check above.
			if status.Error == "" {
				status.Error = fmt.Sprintf("failed to list shipped commits: %v", commitsErr)
			}
		} else {
			for _, c := range shipped {
				commit := &sessionv1.ShippedCommit{
					Sha:        c.SHA,
					Summary:    c.Summary,
					AuthorName: c.AuthorName,
				}
				if !c.AuthorAt.IsZero() {
					commit.AuthoredAt = timestamppb.New(c.AuthorAt)
				}
				status.Commits = append(status.Commits, commit)
			}
		}
	}

	if item.ShippedSnapshotAt != nil {
		status.ShippedCheckConclusion = item.ShippedCheckConclusion
		status.ShippedApprovedCount = int32(item.ShippedApprovedCount)     //#nosec G115 -- bounded by GitHub review count, always small
		status.ShippedChangesReqCount = int32(item.ShippedChangesReqCount) //#nosec G115 -- bounded by GitHub review count, always small
		status.SnapshotAt = timestamppb.New(*item.ShippedSnapshotAt)
		status.SnapshotCaptureFailed = item.ShippedSnapshotCaptureFailed

		if item.ShippedFileStats != "" {
			var decoded []git.FileStat
			if unmarshalErr := json.Unmarshal([]byte(item.ShippedFileStats), &decoded); unmarshalErr != nil {
				// Degrade gracefully: a corrupt/truncated snapshot blob must not
				// fail the whole RPC — every other populated field above is still
				// valid and useful on its own.
				log.WarningLog.Printf("[BacklogService] GetBacklogItemShipStatus item=%s: failed to decode ShippedFileStats: %v", item.ID, unmarshalErr)
			} else {
				for _, fs := range decoded {
					status.FileStats = append(status.FileStats, &sessionv1.ShippedFileStat{
						Path:      fs.Path,
						Status:    fileStatStatusToProto(fs.Status),
						Additions: int32(fs.Additions), //#nosec G115 -- bounded diff-stat count
						Deletions: int32(fs.Deletions), //#nosec G115 -- bounded diff-stat count
					})
				}
			}
		}
	}

	return connect.NewResponse(&sessionv1.GetBacklogItemShipStatusResponse{Status: status}), nil
}

// fileStatStatusToProto maps git.FileStatsBetween's plain-string status
// ("added", "deleted", "renamed", "modified") onto the shared FileStatus
// proto enum used elsewhere for live file-change status.
func fileStatStatusToProto(s string) sessionv1.FileStatus {
	switch s {
	case "added":
		return sessionv1.FileStatus_FILE_STATUS_ADDED
	case "deleted":
		return sessionv1.FileStatus_FILE_STATUS_DELETED
	case "renamed":
		return sessionv1.FileStatus_FILE_STATUS_RENAMED
	case "modified":
		return sessionv1.FileStatus_FILE_STATUS_MODIFIED
	default:
		return sessionv1.FileStatus_FILE_STATUS_UNSPECIFIED
	}
}
