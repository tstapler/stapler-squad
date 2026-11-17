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
	"github.com/tstapler/stapler-squad/session/backlog"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
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
	refresh func(ctx context.Context, inst *instance.Instance, ref domain.PRRef) error
	// detail reads fresh PR state for ask-to-fix; nil means the GraphQL fetcher.
	detail github.PRDetailFetcher
	// tokens resolves the owning account's token; nil means the user PR cache.
	tokens PRTokenResolver
	// itemCleaner re-runs the (gated) terminal-item cleanup after an item-scoped
	// detach releases a hold. Nil leaves the 60 s sweep to do it.
	itemCleaner backlog.WorktreeCleaner
}

// SetItemCleaner wires the cleanup re-run used by DetachItemPullRequest.
func (s *SessionService) SetItemCleaner(c backlog.WorktreeCleaner) { s.prRefs.itemCleaner = c }

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
func (s *SessionService) SetPRRefreshFunc(f func(ctx context.Context, inst *instance.Instance, ref domain.PRRef) error) {
	s.prRefs.refresh = f
}

func (s *SessionService) livePRInstance(id string) (*instance.Instance, error) {
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

func prRefFromProto(p *sessionv1.PullRequestRef) (domain.PRRef, error) {
	if p == nil || p.GetNumber() <= 0 {
		return domain.PRRef{}, connect.NewError(connect.CodeInvalidArgument, errors.New("a pull request is required"))
	}
	ref, err := domain.NewPRRef(p.GetHost(), p.GetOwner(), p.GetRepo(), int(p.GetNumber()))
	if err != nil {
		return domain.PRRef{}, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return ref, nil
}

func prSetError(err error) error {
	switch {
	case errors.Is(err, instance.ErrPRNotAttached):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, instance.ErrPRPersisterUnavailable), errors.Is(err, domain.ErrNoSessionUUID):
		return connect.NewError(connect.CodeFailedPrecondition, err)
	case errors.Is(err, domain.ErrSessionPRLinkCap):
		return connect.NewError(connect.CodeResourceExhausted, err)
	case errors.Is(err, instance.ErrZeroPRRef):
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

func currentPRList(inst *instance.Instance) []*sessionv1.SessionPullRequest {
	snap := inst.Snapshot()
	return adapters.SessionPullRequests(snap.GitHub.PRs, snap.Branch)
}

// PinSessionPullRequest pins pr as the session's active PR; an unset pr clears
// the current pin. Automation never pins.
// +api: session:pin-pr
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
// +api: session:detach-pr
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
// +api: session:attach-pr
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
	ref, err := domain.NewPRRef(parsed.Host, parsed.Owner, parsed.Repo, parsed.PRNumber)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if instanceHasPR(inst, ref) {
		return connect.NewResponse(&sessionv1.AttachSessionPullRequestResponse{
			PullRequests: currentPRList(inst), Result: sessionv1.AttachPullRequestResult_ATTACH_PULL_REQUEST_RESULT_ALREADY_TRACKED, Pr: PRRefToProto(ref),
		}), nil
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
	log.Info("session PR attached by user", "session", inst.Snapshot().Title, "pr", ref.String())
	return connect.NewResponse(&sessionv1.AttachSessionPullRequestResponse{
		PullRequests: currentPRList(inst), Result: sessionv1.AttachPullRequestResult_ATTACH_PULL_REQUEST_RESULT_ATTACHED, Pr: PRRefToProto(ref),
	}), nil
}

func prInitialFromInfo(info *github.PRInfo) domain.PRInitial {
	state, err := domain.ParsePRState(strings.ToLower(info.State))
	if err != nil {
		state = domain.PRStateUnknown
	}
	initial := domain.PRInitial{State: state, URL: info.HTMLURL, Title: info.Title, HeadBranch: info.HeadRef, BaseBranch: info.BaseRef, IsDraft: info.IsDraft}
	// An observed state is verified; `unknown` stays non-terminal, never seeded terminal.
	initial.Terminal = state.Terminal()
	return initial
}

// RefreshSessionPullRequests re-polls the session's open, unknown and
// unverified-terminal PRs (a verified merged or closed PR is not polled) and
// returns the refreshed list. Bounded to 10 seconds.
// +api: session:refresh-prs
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
			log.Warn("session PR refresh failed", "session", inst.Snapshot().Title, "pr", v.Ref.String(), "err", err)
		}
	}
	return connect.NewResponse(&sessionv1.RefreshSessionPullRequestsResponse{PullRequests: currentPRList(inst)}), nil
}

// refreshablePR: open, unknown and unverified-terminal rows; never a verified
// merged or closed one.
func refreshablePR(v domain.PRView) bool {
	return !v.State.Terminal()
}

// listPRFilter turns the optional ListSessions pr selector into a ref. Unset is
// no filter; a set selector that does not name a PR is an error, never PR 0.
func listPRFilter(p *sessionv1.PullRequestRef) (ref domain.PRRef, set bool, err error) {
	if p == nil {
		return domain.PRRef{}, false, nil
	}
	ref, err = prRefFromProto(p)
	return ref, err == nil, err
}

// instanceHasPR reports whether the session holds a link to ref.
func instanceHasPR(inst *instance.Instance, ref domain.PRRef) bool {
	for _, v := range inst.Snapshot().GitHub.PRs {
		if v.Ref == ref {
			return true
		}
	}
	return false
}

// resolveTargetPR is the one place a PR action's selector becomes a PR. An unset
// selector means the session's active PR; a set one must belong to the session.
// The returned ref is never the zero ref: a session with no PR gets
// FailedPrecondition "no pull request", never a call for number 0.
func resolveTargetPR(inst *instance.Instance, sel *sessionv1.PullRequestRef) (*domain.PRRef, error) {
	var requested *domain.PRRef
	if sel != nil {
		ref, err := prRefFromProto(sel)
		if err != nil {
			return nil, err
		}
		requested = &ref
	}
	resolved, err := inst.ResolveTargetPR(requested)
	switch {
	case errors.Is(err, instance.ErrNoPullRequest):
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("no pull request"))
	case errors.Is(err, instance.ErrPRNotAttached):
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	case err != nil:
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return &resolved, nil
}

// SetPRAskToFix installs the fresh-PR reader and token source behind
// AskSessionToFixPullRequest; nil arguments keep the production defaults.
func (s *SessionService) SetPRAskToFix(f github.PRDetailFetcher, t PRTokenResolver) {
	s.prRefs.detail, s.prRefs.tokens = f, t
}

func (s prRefsSeams) detailFetcher() github.PRDetailFetcher {
	if s.detail != nil {
		return s.detail
	}
	return github.GraphQLPRDetailFetcher{}
}

func (s *SessionService) askFixTokens() PRTokenResolver {
	if s.prRefs.tokens != nil {
		return s.prRefs.tokens
	}
	if s.userPRCache != nil {
		return s.userPRCache
	}
	return nil
}

// askFixStop ends an ask-to-fix call early with a typed outcome and its copy.
type askFixStop struct {
	outcome sessionv1.NudgeOutcome
	detail  string
}

// AskSessionToFixPullRequest types a fix request for one attached PR into the
// session's pane. The prompt is built server-side from fresh GitHub state (the
// same builder and delivery guard as Up Next's NudgeSessionForPR); the client
// names only the session and the PR and can never supply text.
// +api: session:ask-to-fix-pr
func (s *SessionService) AskSessionToFixPullRequest(
	ctx context.Context,
	req *connect.Request[sessionv1.AskSessionToFixPullRequestRequest],
) (*connect.Response[sessionv1.AskSessionToFixPullRequestResponse], error) {
	ctx = github.WithGitHubCallOrigin(ctx, github.OriginInteractive)
	inst, err := s.livePRInstance(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	target, err := resolveTargetPR(inst, req.Msg.GetPr())
	if err != nil {
		return nil, err
	}
	out := &sessionv1.AskSessionToFixPullRequestResponse{Pr: PRRefToProto(*target)}
	ctx, cancel := context.WithTimeout(ctx, nudgeRPCTimeout)
	defer cancel()
	stop, err := s.askFixDeliver(ctx, req.Msg.GetId(), *target, out)
	if err != nil {
		return nil, err
	}
	if stop != nil {
		out.Outcome, out.Detail = stop.outcome, stop.detail
	}
	return connect.NewResponse(out), nil
}

// askFixDeliver runs the guarded steps (target check, fresh read, steer). On a
// delivered or typed-steer result it fills out and returns the outcome as a stop.
func (s *SessionService) askFixDeliver(ctx context.Context, id string, target domain.PRRef, out *sessionv1.AskSessionToFixPullRequestResponse) (*askFixStop, error) {
	live, stop, err := s.askFixTarget(id)
	if live == nil {
		return stop, err
	}
	prompt, reasons, stop, err := s.askFixPrompt(ctx, target)
	if prompt == "" {
		return stop, err
	}
	if err := ctxDoneError(ctx); err != nil {
		return nil, err
	}
	key, _ := github.NewPRKey(target.Host(), target.Owner(), target.Repo(), target.Number()) // validated by askFixPrompt
	steer, steerErr := s.SteerInstanceGuarded(ctx, live, NudgeSignature(key, reasons), prompt)
	no, detail, ok := nudgeFromSteer(steer)
	if !ok {
		log.Warn("ask_to_fix_steer_failed", "session", live.Snapshot().Title, "pr", target.String(), "outcome", steer.String(), "err", fmt.Sprint(steerErr))
		if steer == SteerFailed {
			return nil, steerFailedError(ctx, steerErr)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unexpected steer outcome %s", steer))
	}
	log.Info("ask_to_fix_outcome", "session", live.Snapshot().Title, "pr", target.String(), "outcome", no.String(), "reasons", nudgeReasonNames(reasons))
	out.Reasons = reasons
	return &askFixStop{outcome: no, detail: detail}, nil
}

// askFixTarget re-finds the live instance and refuses what cannot take a
// request. A nil instance ends the call with stop or err.
func (s *SessionService) askFixTarget(id string) (*instance.Instance, *askFixStop, error) {
	live := s.FindLiveInstance(id)
	if live == nil {
		return nil, &askFixStop{sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOT_RUNNING, nudgeNotTrackedDetail}, nil
	}
	snap := live.Snapshot()
	switch {
	case snap.Hidden:
		return nil, nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this session is a background session and is read-only, so nothing was sent"))
	case snap.Status.IsSuspended():
		return nil, &askFixStop{sessionv1.NudgeOutcome_NUDGE_OUTCOME_PAUSED, nudgePausedDetail}, nil
	case !CanSteer(snap.Program):
		return nil, nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this session's program does not support requests"))
	}
	return live, nil, nil
}

// askFixPrompt reads fresh PR state with the owning account's token and builds
// the prompt. An empty prompt ends the call with stop or err.
func (s *SessionService) askFixPrompt(ctx context.Context, ref domain.PRRef) (string, []sessionv1.NudgeReason, *askFixStop, error) {
	key, err := github.NewPRKey(ref.Host(), ref.Owner(), ref.Repo(), ref.Number())
	if err != nil {
		return "", nil, nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	tokens := s.askFixTokens()
	if tokens == nil {
		return "", nil, nil, connect.NewError(connect.CodeUnavailable, errors.New("ask to fix is not configured"))
	}
	token, _, ok := tokens.TokenForPR(key)
	if !ok {
		return "", nil, &askFixStop{sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, nudgeNoAccountDetail}, nil
	}
	fetchCtx, cancel := context.WithTimeout(ctx, nudgeFetchTimeout)
	defer cancel()
	detail, fetchErr := s.prRefs.detailFetcher().FetchPRNudgeDetail(fetchCtx, key, token)
	if fetchErr != nil {
		kind, cerr := classifyNudgeFetchError(fetchCtx, fetchErr, func() string { return nudgeRateLimitMessage(nil) })
		if kind == nudgeFetchNotFound {
			return "", nil, &askFixStop{sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, nudgePRNotFoundDetail}, nil
		}
		log.Warn("ask_to_fix_fetch_failed", "pr", ref.String(), "kind", string(kind), "err", fetchErr)
		return "", nil, nil, cerr
	}
	prompt, reasons := BuildPRNudgePrompt(detail)
	if prompt == "" || len(reasons) == 0 {
		return "", nil, &askFixStop{sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOTHING_TO_FIX, nudgeNothingToFixDetail}, nil
	}
	return prompt, reasons, nil, nil
}
