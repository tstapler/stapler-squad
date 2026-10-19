package services

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
)

// prActionDeps are the GitHub calls behind the session-less PR action RPCs.
// Nil fields use the real github package functions; tests substitute fakes.
type prActionDeps struct {
	merge     func(ctx context.Context, ref githubpkg.RepoRef, number int, method string) error
	close     func(ctx context.Context, ref githubpkg.RepoRef, number int) error
	setDraft  func(ctx context.Context, host, nodeID, token string, draft bool) error
	refresh   func(ctx context.Context) error
	tokenFrom PRTokenResolver
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
	return d
}

// directPRTarget validates (owner, repo, number, host) shared by every direct action.
func directPRTarget(owner, repo string, number int32, host string) (githubpkg.RepoRef, error) {
	if number <= 0 {
		return githubpkg.RepoRef{}, connect.NewError(connect.CodeInvalidArgument, errors.New("number must be positive"))
	}
	ref, err := githubpkg.NewRepoRefWithHost(owner, repo, host)
	if err != nil {
		return githubpkg.RepoRef{}, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ref, nil
}

// finishPRAction refreshes the cache after a successful action so
// WatchUserPRs subscribers see the new state, and builds the response.
func (s *GitHubUserService) finishPRAction(ctx context.Context, d prActionDeps, err error, failure, success string) (*connect.Response[sessionv1.PRActionResponse], error) {
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, classifyGitHubRateLimitError(fmt.Errorf("%s: %w", failure, err)))
	}
	_ = d.refresh(ctx)
	return connect.NewResponse(&sessionv1.PRActionResponse{Success: true, Message: success}), nil
}

// +api: github-user:merge-pr-direct
func (s *GitHubUserService) MergePRDirect(
	ctx context.Context,
	req *connect.Request[sessionv1.MergePRDirectRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
	m := req.Msg
	ref, err := directPRTarget(m.Owner, m.Repo, m.Number, m.Host)
	if err != nil {
		return nil, err
	}
	method := "merge"
	if m.Method != nil && *m.Method != "" {
		method = *m.Method
	}
	d := s.actionDeps()
	return s.finishPRAction(ctx, d, d.merge(ctx, ref, int(m.Number), method),
		"failed to merge PR", fmt.Sprintf("PR #%d merged using method '%s'", m.Number, method))
}

// +api: github-user:close-pr-direct
func (s *GitHubUserService) ClosePRDirect(
	ctx context.Context,
	req *connect.Request[sessionv1.ClosePRDirectRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
	m := req.Msg
	ref, err := directPRTarget(m.Owner, m.Repo, m.Number, m.Host)
	if err != nil {
		return nil, err
	}
	d := s.actionDeps()
	return s.finishPRAction(ctx, d, d.close(ctx, ref, int(m.Number)),
		"failed to close PR", fmt.Sprintf("PR #%d closed", m.Number))
}

// +api: github-user:convert-pr-to-draft
func (s *GitHubUserService) ConvertPRToDraft(
	ctx context.Context,
	req *connect.Request[sessionv1.PRDraftToggleRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	return s.toggleDraft(ctx, req.Msg, true)
}

// +api: github-user:mark-pr-ready
func (s *GitHubUserService) MarkPRReady(
	ctx context.Context,
	req *connect.Request[sessionv1.PRDraftToggleRequest],
) (*connect.Response[sessionv1.PRActionResponse], error) {
	return s.toggleDraft(ctx, req.Msg, false)
}

func (s *GitHubUserService) toggleDraft(ctx context.Context, m *sessionv1.PRDraftToggleRequest, draft bool) (*connect.Response[sessionv1.PRActionResponse], error) {
	ctx = githubpkg.WithGitHubCallOrigin(ctx, githubpkg.OriginInteractive)
	if _, err := directPRTarget(m.Owner, m.Repo, m.Number, m.Host); err != nil {
		return nil, err
	}
	if m.NodeId == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("PR node_id is not known yet; wait for the next refresh"))
	}
	key, err := githubpkg.NewPRKey(m.Host, m.Owner, m.Repo, int(m.Number))
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	d := s.actionDeps()
	token, _, ok := d.tokenFrom.TokenForPR(key)
	if !ok {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the GitHub account for this PR is no longer connected"))
	}
	msg := fmt.Sprintf("PR #%d marked ready for review", m.Number)
	failure := "failed to mark PR ready"
	if draft {
		msg = fmt.Sprintf("PR #%d converted to draft", m.Number)
		failure = "failed to convert PR to draft"
	}
	return s.finishPRAction(ctx, d, d.setDraft(ctx, defaultHostIfEmpty(m.Host), m.NodeId, token, draft), failure, msg)
}
