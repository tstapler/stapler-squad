package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/detection"
	"github.com/tstapler/stapler-squad/session/git"
	"google.golang.org/protobuf/proto"
)

// Fan-out group membership is stored as session tags, so no schema change is needed and
// the group survives restarts with the rest of the session's tags.
const (
	// MinFanoutVariants and MaxFanoutVariants bound FanOutSession's count (Orca's cap is 5).
	MinFanoutVariants = 2
	MaxFanoutVariants = 5

	fanoutGroupTagPrefix   = "fanout:"
	fanoutVariantTagPrefix = "fanout-variant:"
	fanoutBaseTagPrefix    = "fanout-base:"
)

// fanoutPickLocks serializes PickFanoutWinner per group: two picks would otherwise run
// concurrent merges in the same repo and each archive the other's winner.
var fanoutPickLocks sync.Map // groupID -> *sync.Mutex

// +api: session:fanout
// FanOutSession creates count sessions from one prompt. CreateSession returns once the
// session is registered (worktree setup resolves in the background pipeline), so the
// sequential loop stays fast and avoids worktree-naming races between variants.
func (s *SessionService) FanOutSession(
	ctx context.Context,
	req *connect.Request[sessionv1.FanOutSessionRequest],
) (*connect.Response[sessionv1.FanOutSessionResponse], error) {
	count := int(req.Msg.Count)
	if count < MinFanoutVariants || count > MaxFanoutVariants {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("count must be between %d and %d, got %d", MinFanoutVariants, MaxFanoutVariants, count))
	}
	base := req.Msg.Base
	if base == nil || base.Title == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("base.title is required"))
	}
	if len(req.Msg.ProgramOverrides) > count {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("program_overrides has %d entries for %d variants", len(req.Msg.ProgramOverrides), count))
	}

	groupID := uuid.NewString()
	groupTags := []string{fanoutGroupTagPrefix + groupID}
	if branch, err := git.GetCurrentBranchName(base.Path); err == nil && branch != "" {
		groupTags = append(groupTags, fanoutBaseTagPrefix+branch)
	}
	log.Info("creating fan-out group", "group_id", groupID, "count", count, "title", base.Title)

	created := make([]*sessionv1.Session, 0, count)
	for i := 0; i < count; i++ {
		variantReq := proto.Clone(base).(*sessionv1.CreateSessionRequest)
		variantReq.Title = fmt.Sprintf("%s [variant %d]", base.Title, i+1)
		variantReq.Tags = append(append([]string{}, base.Tags...), groupTags...)
		variantReq.Tags = append(variantReq.Tags, fanoutVariantTagPrefix+strconv.Itoa(i+1))
		if base.Branch != "" {
			variantReq.Branch = fmt.Sprintf("%s-v%d", base.Branch, i+1) // variants must not share a branch
		}
		if i < len(req.Msg.ProgramOverrides) && req.Msg.ProgramOverrides[i] != "" {
			variantReq.Program = req.Msg.ProgramOverrides[i]
		}
		resp, err := s.CreateSession(ctx, connect.NewRequest(variantReq))
		if err != nil {
			// Earlier variants stay running; the caller sees which ones were created via the
			// group tag and can compare or archive them — silently killing work is worse.
			return nil, connect.NewError(connect.CodeOf(err),
				fmt.Errorf("variant %d of %d failed (group %s, %d already created): %w", i+1, count, groupID, len(created), err))
		}
		created = append(created, resp.Msg.Session)
	}
	return connect.NewResponse(&sessionv1.FanOutSessionResponse{GroupId: groupID, Sessions: created}), nil
}

// fanoutWinnerEligibility returns nil when a session may be picked as a winner: it has
// stopped or been paused/hibernated, or it is Active but sitting at an idle/ready/success
// prompt. Anything still producing output (or crashed/failed/creating) is rejected so a
// partial diff is never merged.
func fanoutWinnerEligibility(status session.Status, claude detection.DetectedStatus) error {
	switch status {
	case session.Stopped, session.Paused, session.Hibernated:
		return nil
	case session.Active:
		switch claude {
		case detection.StatusIdle, detection.StatusReady, detection.StatusSuccess:
			return nil
		}
		return errors.New("session is still running; wait until it is idle or stopped")
	default:
		return fmt.Errorf("session status %d cannot be picked as a winner", status)
	}
}

func (s *SessionService) fanoutGroupMembers(groupID string) []*session.Instance {
	tag := fanoutGroupTagPrefix + groupID
	var members []*session.Instance
	for _, inst := range s.GetInstances() {
		if inst.HasTag(tag) {
			members = append(members, inst)
		}
	}
	return members
}

func fanoutBaseBranch(inst *session.Instance) string {
	for _, t := range inst.GetTags() {
		if strings.HasPrefix(t, fanoutBaseTagPrefix) {
			return strings.TrimPrefix(t, fanoutBaseTagPrefix)
		}
	}
	return ""
}

// +api: session:fanout-pick
// PickFanoutWinner merges the winner's branch into the group's base branch and archives
// every other member through ArchiveSession (so ArchivedAt is set via the existing path).
func (s *SessionService) PickFanoutWinner(
	ctx context.Context,
	req *connect.Request[sessionv1.PickFanoutWinnerRequest],
) (*connect.Response[sessionv1.PickFanoutWinnerResponse], error) {
	if req.Msg.GroupId == "" || req.Msg.WinnerSessionId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("group_id and winner_session_id are required"))
	}
	mu, _ := fanoutPickLocks.LoadOrStore(req.Msg.GroupId, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()

	members := s.fanoutGroupMembers(req.Msg.GroupId)
	if len(members) == 0 {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("fan-out group not found: %s", req.Msg.GroupId))
	}
	var winner *session.Instance
	for _, m := range members {
		if m.MatchesID(req.Msg.WinnerSessionId) {
			winner = m
		}
	}
	if winner == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("session %s is not in fan-out group %s", req.Msg.WinnerSessionId, req.Msg.GroupId))
	}
	if winner.IsArchived() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("winner session is archived"))
	}

	claude := detection.StatusUnknown
	if s.statusManager != nil {
		claude = s.statusManager.GetStatus(winner).ClaudeStatus
	}
	if err := fanoutWinnerEligibility(session.Status(winner.GetStatus()), claude); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}

	wt, err := winner.GetGitWorktree()
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("winner %s has no git worktree: %w", winner.UUID, err))
	}
	if wt == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("winner %s has no git worktree", winner.UUID))
	}
	baseBranch := fanoutBaseBranch(winner)
	if baseBranch == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("fan-out group has no recorded base branch"))
	}
	current, err := git.GetCurrentBranchName(wt.GetRepoPath())
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("cannot read the checked-out branch of %s: %w", wt.GetRepoPath(), err))
	}
	if current != baseBranch {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("repository must have base branch %q checked out to merge (found %q)", baseBranch, current))
	}
	if dirty, err := git.RepoHasUncommittedChanges(wt.GetRepoPath()); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check %s for uncommitted changes: %w", wt.GetRepoPath(), err))
	} else if dirty {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("repository %s has uncommitted changes; commit or stash them before picking a winner", wt.GetRepoPath()))
	}

	// A paused session's worktree is deleted but its branch keeps the committed work, so
	// there is nothing to commit; merge the branch as is.
	if _, statErr := os.Stat(wt.GetWorktreePath()); statErr == nil {
		if err := wt.CommitChanges("fan-out: variant " + winner.Title); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to commit changes of winner %s: %w", winner.UUID, err))
		}
	}
	result, err := git.MergeBranchIntoCheckedOut(wt.GetRepoPath(), wt.GetBranchName())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("merge failed: %w", err))
	}
	if result.Conflicted {
		return nil, connect.NewError(connect.CodeAborted,
			fmt.Errorf("merge conflicts in %s; resolve manually (base branch unchanged): %s",
				wt.GetBranchName(), strings.Join(result.ConflictedFiles, ", ")))
	}

	var archived, archiveFailed []string
	for _, m := range members {
		if m == winner || m.IsArchived() {
			continue
		}
		if _, err := s.ArchiveSession(ctx, connect.NewRequest(&sessionv1.ArchiveSessionRequest{SessionId: m.UUID})); err != nil {
			log.Warn("failed to archive fan-out loser", "group_id", req.Msg.GroupId, "session", m.UUID, "err", err)
			archiveFailed = append(archiveFailed, m.UUID)
			continue
		}
		archived = append(archived, m.UUID)
	}
	return connect.NewResponse(&sessionv1.PickFanoutWinnerResponse{BaseBranch: baseBranch, ArchivedSessionIds: archived, FailedArchiveSessionIds: archiveFailed}), nil
}
