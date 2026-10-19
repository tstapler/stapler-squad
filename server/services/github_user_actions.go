package services

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
)

// prActionDeps are the GitHub calls behind the session-less PR action RPCs.
// Nil fields use the real github package functions; tests substitute fakes.
type prActionDeps struct {
	merge     func(ctx context.Context, ref githubpkg.RepoRef, number int, method string) error
	close     func(ctx context.Context, ref githubpkg.RepoRef, number int) error
	setDraft  func(ctx context.Context, host, nodeID, token string, draft bool) error
	refresh   func(ctx context.Context) error
	tokenFrom PRTokenResolver
	snapshot  PRSnapshotSource
}

func (s *GitHubUserService) actionDeps() prActionDeps {
	d := s.actions
	if d.merge == nil {
		d.merge = githubpkg.MergePR
	}
	if d.close == nil {
		d.close = githubpkg.ClosePR
	}
	if d.setDraft == nil {
		d.setDraft = func(ctx context.Context, host, nodeID, token string, draft bool) error {
			if draft {
				return githubpkg.ConvertPRToDraft(ctx, host, nodeID, token)
			}
			return githubpkg.MarkPRReadyForReview(ctx, host, nodeID, token)
		}
	}
	if d.refresh == nil {
		d.refresh = s.cache.Refresh
	}
	if d.tokenFrom == nil {
		d.tokenFrom = s.cache
	}
	if d.snapshot == nil {
		d.snapshot = s.cache
	}
	return d
}

// prActionTarget validates (owner, repo, number, host) and requires the PR to
// belong to a connected account (the same gate the nudge path uses), so these
// RPCs only act on PRs the panel itself lists.
func (d prActionDeps) prActionTarget(owner, repo string, number int32, host string) (githubpkg.PRKey, githubpkg.RepoRef, string, error) {
	if number <= 0 {
		return githubpkg.PRKey{}, githubpkg.RepoRef{}, "", connect.NewError(connect.CodeInvalidArgument, errors.New("number must be positive"))
	}
	key, err := githubpkg.NewPRKey(host, owner, repo, int(number))
	if err != nil {
		return githubpkg.PRKey{}, githubpkg.RepoRef{}, "", connect.NewError(connect.CodeInvalidArgument, err)
	}
	ref, err := githubpkg.NewRepoRefWithHost(owner, repo, host)
	if err != nil {
		return githubpkg.PRKey{}, githubpkg.RepoRef{}, "", connect.NewError(connect.CodeInvalidArgument, err)
	}
	token, _, ok := d.tokenFrom.TokenForPR(key)
	if !ok {
		return githubpkg.PRKey{}, githubpkg.RepoRef{}, "", connect.NewError(connect.CodeFailedPrecondition, errors.New("PR is not tracked or its GitHub account is no longer connected"))
	}
	return key, ref, token, nil
}

// finishPRAction refreshes the cache after a successful action so
// WatchUserPRs subscribers see the new state, and builds the response.
func finishPRAction(ctx context.Context, d prActionDeps, err error, failure, success string) (*connect.Response[sessionv1.PRActionResponse], error) {
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, classifyGitHubRateLimitError(fmt.Errorf("%s: %w", failure, err)))
	}
	if rerr := d.refresh(ctx); rerr != nil {
		log.Warn("PR cache refresh after action failed", "error", rerr)
	}
	return connect.NewResponse(&sessionv1.PRActionResponse{Success: true, Message: success}), nil
}

// +api: MergePRDirect
func (s *GitHubUserService) MergePRDirect(
	ctx context.Context,
	req *connect.Request[sessionv1.MergePRDirectRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
	m := req.Msg
	method := "merge"
	if m.Method != nil && *m.Method != "" {
		method = *m.Method
	}
	switch method {
	case "merge", "squash", "rebase":
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unknown merge method %q", method))
	}
	d := s.actionDeps()
	_, ref, _, err := d.prActionTarget(m.Owner, m.Repo, m.Number, m.Host)
	if err != nil {
		return nil, err
	}
	return finishPRAction(ctx, d, d.merge(ctx, ref, int(m.Number), method),
		"failed to merge PR", fmt.Sprintf("PR #%d merged using method '%s'", m.Number, method))
}

// +api: ClosePRDirect
func (s *GitHubUserService) ClosePRDirect(
	ctx context.Context,
	req *connect.Request[sessionv1.ClosePRDirectRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
	m := req.Msg
	d := s.actionDeps()
	_, ref, _, err := d.prActionTarget(m.Owner, m.Repo, m.Number, m.Host)
	if err != nil {
		return nil, err
	}
	return finishPRAction(ctx, d, d.close(ctx, ref, int(m.Number)),
		"failed to close PR", fmt.Sprintf("PR #%d closed", m.Number))
}

// +api: ConvertPRToDraft
func (s *GitHubUserService) ConvertPRToDraft(
	ctx context.Context,
	req *connect.Request[sessionv1.PRDraftToggleRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	return s.toggleDraft(ctx, req.Msg, true)
}

// +api: MarkPRReady
func (s *GitHubUserService) MarkPRReady(
	ctx context.Context,
	req *connect.Request[sessionv1.PRDraftToggleRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	return s.toggleDraft(ctx, req.Msg, false)
}

func (s *GitHubUserService) toggleDraft(ctx context.Context, m *sessionv1.PRDraftToggleRequest, draft bool) (*connect.Response[sessionv1.PRActionResponse], error) {
	ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
	d := s.actionDeps()
	key, _, token, err := d.prActionTarget(m.Owner, m.Repo, m.Number, m.Host)
	if err != nil {
		return nil, err
	}
	nodeID := cachedNodeID(d.snapshot.GetAll(), key)
	if nodeID == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("PR node_id is not known yet; wait for the next refresh"))
	}
	msg := fmt.Sprintf("PR #%d marked ready for review", m.Number)
	failure := "failed to mark PR ready"
	if draft {
		msg = fmt.Sprintf("PR #%d converted to draft", m.Number)
		failure = "failed to convert PR to draft"
	}
	return finishPRAction(ctx, d, d.setDraft(ctx, key.Host(), nodeID, token, draft), failure, msg)
}

// cachedNodeID returns the polled node ID for key, so the mutation can never
// target a different PR than the one the account was authorized for.
func cachedNodeID(prs []githubpkg.UserPR, key githubpkg.PRKey) string {
	for _, pr := range prs {
		if pr.Number == key.Number() && githubpkg.NormalizeHost(pr.Host) == key.Host() &&
			pr.Owner == key.Owner() && pr.Repo == key.RepoName() {
			return pr.NodeID
		}
	}
	return ""
}
