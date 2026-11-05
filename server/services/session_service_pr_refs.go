package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/adapters"
	"github.com/tstapler/stapler-squad/session"
)

// UI-only RPCs for editing a session's PR set. They are never reachable from the
// MCP tool registry (server/mcp/tools_pr_rpc_unreachable_test.go).

const refreshSessionPRsTimeout = 10 * time.Second

// prRefsSeams are the outside-world calls of the PR set RPCs, replaceable in tests.
type prRefsSeams struct {
	// lookup fetches a PR through the conditional (ETag) GitHub path.
	lookup func(ctx context.Context, ref github.RepoRef, number int) (*github.PRInfo, error)
	// refresh re-polls one PR and returns once the state row is updated. Nil
	// until the awaited poller refresh (T3.3.2) is wired.
	refresh func(ctx context.Context, inst *session.Instance, ref session.PRRef) error
}

func (s prRefsSeams) lookupPR(ctx context.Context, ref github.RepoRef, number int) (*github.PRInfo, error) {
	if s.lookup != nil {
		return s.lookup(ctx, ref, number)
	}
	info, _, err := github.GetPRInfoConditional(ctx, ref, number, github.NewETagCache())
	if err == nil && info == nil {
		err = errors.New("pull request not found")
	}
	return info, err
}

// SetPRRefreshFunc installs the awaited per-PR refresh behind
// RefreshSessionPullRequests.
func (s *SessionService) SetPRRefreshFunc(f func(ctx context.Context, inst *session.Instance, ref session.PRRef) error) {
	s.prRefs.refresh = f
}

func (s *SessionService) livePRInstance(id string) (*session.Instance, error) {
	if id == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("session id is required"))
	}
	if inst := s.FindLiveInstance(id); inst != nil {
		return inst, nil
	}
	instances, err := s.loadInstancesWithWiring()
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if inst := findInstanceByID(instances, id); inst != nil {
		return inst, nil
	}
	return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("session not found: %s", id))
}

func prRefFromProto(p *sessionv1.PullRequestRef) (session.PRRef, error) {
	if p == nil || p.GetNumber() <= 0 {
		return session.PRRef{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a pull request is required"))
	}
	ref, err := session.NewPRRef(p.GetHost(), p.GetOwner(), p.GetRepo(), int(p.GetNumber()))
	if err != nil {
		return session.PRRef{}, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ref, nil
}

func prSetError(err error) error {
	switch {
	case errors.Is(err, session.ErrPRNotAttached):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, session.ErrPRPersisterUnavailable), errors.Is(err, session.ErrNoSessionUUID):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func currentPRList(inst *session.Instance) []*sessionv1.SessionPullRequest {
	snap := inst.Snapshot()
	return adapters.SessionPullRequests(snap.GitHub.PRs, snap.Branch)
}

// PinSessionPullRequest pins pr as the session's active PR; an unset pr clears
// the current pin. Automation never pins.
func (s *SessionService) PinSessionPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.PinSessionPullRequestRequest],
) (*connect.Response[sessionv1.PinSessionPullRequestResponse], error) {
	inst, err := s.livePRInstance(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetPr() == nil {
		for _, v := range inst.Snapshot().GitHub.PRs {
			if !v.Pinned {
				continue
			}
			if err := inst.PinPRPersisted(ctx, v.Ref, false); err != nil {
				return nil, prSetError(err)
			}
		}
	} else {
		ref, err := prRefFromProto(req.Msg.GetPr())
		if err != nil {
			return nil, err
		}
		if err := inst.PinPRPersisted(ctx, ref, true); err != nil {
			return nil, prSetError(err)
		}
	}
	return connect.NewResponse(&sessionv1.PinSessionPullRequestResponse{PullRequests: currentPRList(inst)}), nil
}

// DetachSessionPullRequest removes exactly this session's link to pr and
// tombstones it, so branch discovery does not bring it back.
func (s *SessionService) DetachSessionPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.DetachSessionPullRequestRequest],
) (*connect.Response[sessionv1.DetachSessionPullRequestResponse], error) {
	inst, err := s.livePRInstance(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	ref, err := prRefFromProto(req.Msg.GetPr())
	if err != nil {
		return nil, err
	}
	found, err := inst.DetachPRPersisted(ctx, ref)
	if err != nil {
		return nil, prSetError(err)
	}
	if !found {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("%s is not attached to this session", ref))
	}
	return connect.NewResponse(&sessionv1.DetachSessionPullRequestResponse{PullRequests: currentPRList(inst)}), nil
}

// AttachSessionPullRequest links the PR at the given URL after checking on GitHub
// that it exists. The attach clears this session's tombstone for it.
func (s *SessionService) AttachSessionPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.AttachSessionPullRequestRequest],
) (*connect.Response[sessionv1.AttachSessionPullRequestResponse], error) {
	ctx = github.WithGitHubCallOrigin(ctx, github.OriginInteractive)
	inst, err := s.livePRInstance(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	parsed, err := github.ParseGitHubRef(req.Msg.GetUrl())
	if err != nil || parsed.Type != github.RefTypePR {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url must be a GitHub pull request URL"))
	}
	ref, err := session.NewPRRef(parsed.Host, parsed.Owner, parsed.Repo, parsed.PRNumber)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	repoRef, err := github.NewRepoRefWithHost(parsed.Owner, parsed.Repo, parsed.Host)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	info, err := s.prRefs.lookupPR(ctx, repoRef, parsed.PRNumber)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, classifyGitHubRateLimitError(fmt.Errorf("could not find %s on GitHub: %w", ref, err)))
	}
	if err := inst.AttachPRManual(ctx, ref, prInitialFromInfo(info)); err != nil {
		return nil, prSetError(err)
	}
	log.Info("session PR attached by user", "session", inst.Title, "pr", ref.String())
	return connect.NewResponse(&sessionv1.AttachSessionPullRequestResponse{PullRequests: currentPRList(inst)}), nil
}

func prInitialFromInfo(info *github.PRInfo) session.PRInitial {
	state, err := session.ParsePRState(strings.ToLower(info.State))
	if err != nil {
		state = session.PRStateUnknown
	}
	initial := session.PRInitial{State: state, URL: info.HTMLURL, HeadBranch: info.HeadRef, BaseBranch: info.BaseRef, IsDraft: info.IsDraft}
	// An observed state is verified; `unknown` stays non-terminal, never seeded terminal.
	initial.Terminal = state.Terminal()
	return initial
}

// RefreshSessionPullRequests re-polls the session's open, unknown and
// unverified-terminal PRs (a verified merged or closed PR is not polled) and
// returns the refreshed list. Bounded to 10 seconds.
func (s *SessionService) RefreshSessionPullRequests(
	ctx context.Context,
	req *connect.Request[sessionv1.RefreshSessionPullRequestsRequest],
) (*connect.Response[sessionv1.RefreshSessionPullRequestsResponse], error) {
	inst, err := s.livePRInstance(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	if s.prRefs.refresh == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("pull request refresh is not available"))
	}
	ctx, cancel := context.WithTimeout(ctx, refreshSessionPRsTimeout)
	defer cancel()
	for _, v := range inst.Snapshot().GitHub.PRs {
		if !refreshablePR(v) {
			continue
		}
		if err := s.prRefs.refresh(ctx, inst, v.Ref); err != nil {
			log.Warn("session PR refresh failed", "session", inst.Title, "pr", v.Ref.String(), "err", err)
		}
	}
	return connect.NewResponse(&sessionv1.RefreshSessionPullRequestsResponse{PullRequests: currentPRList(inst)}), nil
}

// refreshablePR: open, unknown and unverified-terminal rows; never a verified
// merged or closed one.
func refreshablePR(v session.PRView) bool {
	return !v.State.Terminal()
}
