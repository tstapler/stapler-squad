package services

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// nudgeFetchTimeout bounds the fresh GitHub read; nudgeRPCTimeout bounds the whole call.
	nudgeFetchTimeout = 5 * time.Second
	nudgeRPCTimeout   = 8 * time.Second

	nudgeBusyDetail          = "Session is busy. Try again when it is idle."
	nudgeNoControllerDetail  = "Session isn't being monitored, so it can't safely take a request. Open it to restart it."
	nudgePausedDetail        = "Session paused. Open it to resume"
	nudgeNotTrackedDetail    = "Session is not running or was deleted. Open its page to restart it."
	nudgeCoolingDownDetail   = "The last request to this session failed. Try again in a few seconds."
	nudgeNoAccountDetail     = "The GitHub account for this PR is no longer connected."
	nudgeNothingToFixDetail  = "Nothing to fix right now."
	nudgeNotLinkedDetail     = "Session is not linked to this PR."
	nudgePRNotFoundDetail    = "PR not found."
	nudgeRateLimitedFallback = "GitHub rate limit reached; try again shortly"
)

// PRNudger is the slice of SessionService the nudge handler needs. It is
// consumer-side so tests substitute a fake with no tmux.
type PRNudger interface {
	FindLiveInstance(id string) *session.Instance
	SteerInstanceGuarded(ctx context.Context, inst *session.Instance, sig, msg string) (SteerOutcome, error)
	// InstanceReadyForSteer is the idle gate SteerInstanceGuarded applies.
	InstanceReadyForSteer(inst *session.Instance) bool
}

var _ PRNudger = (*SessionService)(nil)

// PRTokenResolver returns the token and login of the account that owns a PR.
// There is deliberately no cross-account fallback. Implemented by UserPRCache.
type PRTokenResolver interface {
	TokenForPR(key githubpkg.PRKey) (token, login string, ok bool)
}

// PRSnapshotSource lists the polled open PRs. Implemented by UserPRCache.
type PRSnapshotSource interface {
	GetAll() []githubpkg.UserPR
}

// PRNudgeTracker feeds the success-metric logs. Implemented by UserPRCache.
type PRNudgeTracker interface {
	RecordNudge(key githubpkg.PRKey, reasons []githubpkg.NudgeReasonKind, at time.Time)
	FirstSeenAttention(key githubpkg.PRKey) (time.Time, bool)
}

// nudgeClock is the time source for latency, timeouts and attention age.
type nudgeClock interface {
	Now() time.Time
	WithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc)
}

type realNudgeClock struct{}

func (realNudgeClock) Now() time.Time { return time.Now() }
func (realNudgeClock) WithTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, d)
}

// nudgeDeps are GitHubUserService's optional nudge collaborators. The zero
// value is production behavior except that nudger and fetcher must be set.
type nudgeDeps struct {
	nudger   PRNudger
	fetcher  githubpkg.PRDetailFetcher
	tokens   PRTokenResolver
	snapshot PRSnapshotSource
	tracker  PRNudgeTracker
	clock    nudgeClock
	logf     func(msg string, args ...any)
	// rateLimitResume reports when GitHub access resumes; nil uses the process-wide limiter.
	rateLimitResume func() (limited bool, resumeAt time.Time)
}

// SetPRNudger sets the session-side port. Call before serving requests.
func (s *GitHubUserService) SetPRNudger(n PRNudger) { s.nudge.nudger = n }

// SetPRDetailFetcher sets the fresh-PR-state reader.
func (s *GitHubUserService) SetPRDetailFetcher(f githubpkg.PRDetailFetcher) { s.nudge.fetcher = f }

// SetPRTokenResolver overrides token lookup; the default is the PR cache.
func (s *GitHubUserService) SetPRTokenResolver(r PRTokenResolver) { s.nudge.tokens = r }

func (s *GitHubUserService) nudgeClock() nudgeClock {
	if s.nudge.clock != nil {
		return s.nudge.clock
	}
	return realNudgeClock{}
}

func (s *GitHubUserService) nudgeLogf(msg string, args ...any) {
	if s.nudge.logf != nil {
		s.nudge.logf(msg, args...)
		return
	}
	log.Info(msg, args...)
}

// nudgeWarnf is nudgeLogf at warn level; tests capture both through nudge.logf.
func (s *GitHubUserService) nudgeWarnf(msg string, args ...any) {
	if s.nudge.logf != nil {
		s.nudge.logf(msg, args...)
		return
	}
	log.Warn(msg, args...)
}

func (s *GitHubUserService) tokenResolver() PRTokenResolver {
	if s.nudge.tokens != nil {
		return s.nudge.tokens
	}
	if s.cache != nil {
		return s.cache
	}
	return nil
}

func (s *GitHubUserService) prSnapshot() PRSnapshotSource {
	if s.nudge.snapshot != nil {
		return s.nudge.snapshot
	}
	if s.cache != nil {
		return s.cache
	}
	return nil
}

func (s *GitHubUserService) nudgeTracker() PRNudgeTracker {
	if s.nudge.tracker != nil {
		return s.nudge.tracker
	}
	if s.cache != nil {
		return s.cache
	}
	return nil
}

// parsePRKey validates and normalizes the wire PRKey at the RPC boundary: an
// empty host means github.com, and empty owner/repo or number <= 0 is rejected.
func parsePRKey(k *sessionv1.PRKey) (githubpkg.PRKey, error) {
	if k == nil {
		return githubpkg.PRKey{}, connect.NewError(connect.CodeInvalidArgument, errors.New("pr is required"))
	}
	key, err := githubpkg.NewPRKey(k.GetHost(), k.GetOwner(), k.GetRepo(), int(k.GetNumber()))
	if err != nil {
		return githubpkg.PRKey{}, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid pr: %w", err))
	}
	return key, nil
}

// CanSteer reports whether program is an agent whose idleness the shared
// readiness gate can confirm (Claude Code and pi have a status source). Other
// programs cannot be written to safely and the nudge refuses them.
func CanSteer(program string) bool {
	if program == "" {
		return true
	}
	for _, tok := range strings.Fields(program) {
		switch filepath.Base(tok) {
		case "claude", "pi":
			return true
		}
	}
	return false
}

// nudgeCall carries what the exit log line reports.
type nudgeCall struct {
	startedAt   time.Time
	key         githubpkg.PRKey
	pr          string
	host        string
	account     string
	sessionID   string
	reasons     []sessionv1.NudgeReason
	promptBytes int
	attention   *int64
	// live is true once the session resolved to a live, unpaused instance;
	// it is the denominator for the nudge delivery rate (see requirements).
	live bool
}

// attentionAge is the whole seconds since firstSeen, or false when the poll
// never saw the PR needing attention (zero firstSeen).
func attentionAge(now, firstSeen time.Time) (int64, bool) {
	if firstSeen.IsZero() || now.Before(firstSeen) {
		return 0, false
	}
	return int64(now.Sub(firstSeen) / time.Second), true
}

// +api: github-user:nudge-session-for-pr
// NudgeSessionForPR asks one linked session to fix what currently blocks a PR.
// The prompt is built server-side from fresh GitHub state; the client sends
// only a PR key and a session id and can never supply text.
func (s *GitHubUserService) NudgeSessionForPR(
	ctx context.Context,
	req *connect.Request[sessionv1.NudgeSessionForPRRequest],
) (*connect.Response[sessionv1.NudgeSessionForPRResponse], error) {
	clk := s.nudgeClock()
	call := &nudgeCall{startedAt: clk.Now(), sessionID: req.Msg.GetSessionId()}
	if k := req.Msg.GetPr(); k != nil {
		call.pr = fmt.Sprintf("%s/%s/%s#%d", k.GetHost(), k.GetOwner(), k.GetRepo(), k.GetNumber())
	}
	s.nudgeLogf("nudge_request", "pr", call.pr, "session_id", call.sessionID)

	resp, err := s.runNudge(ctx, call, req.Msg)
	s.logNudgeExit(call, resp, err)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

func (s *GitHubUserService) attentionSeconds(key githubpkg.PRKey, now time.Time) *int64 {
	t := s.nudgeTracker()
	if t == nil {
		return nil
	}
	at, _ := t.FirstSeenAttention(key)
	age, ok := attentionAge(now, at)
	if !ok {
		return nil
	}
	return &age
}

func (s *GitHubUserService) runNudge(ctx context.Context, call *nudgeCall, req *sessionv1.NudgeSessionForPRRequest) (*sessionv1.NudgeSessionForPRResponse, error) {
	if s.nudge.nudger == nil || s.nudge.fetcher == nil || s.tokenResolver() == nil || s.prSnapshot() == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("nudge is not configured"))
	}
	key, err := parsePRKey(req.GetPr())
	if err != nil {
		return nil, err
	}
	call.pr, call.host, call.key = key.String(), key.Host(), key
	call.attention = s.attentionSeconds(key, call.startedAt)

	ctx, cancel := s.nudgeClock().WithTimeout(ctx, nudgeRPCTimeout)
	defer cancel()

	pr, ok := s.findPR(key)
	if !ok {
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, nil, "", nudgePRNotFoundDetail), nil
	}
	if !linkedStrictly(pr, req.GetSessionId()) {
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_SESSION_NOT_LINKED, nil, "", nudgeNotLinkedDetail), nil
	}

	inst, early, err := s.resolveSession(call, req.GetSessionId())
	if inst == nil {
		return early, err
	}

	if err := ctxDoneError(ctx); err != nil {
		return nil, err
	}
	prompt, reasons, early, err := s.freshPrompt(ctx, call)
	if err != nil || early != nil {
		return early, err
	}
	call.reasons = reasons
	call.promptBytes = len(prompt)
	// The deadline may have lapsed during the GitHub read; do not write after it.
	if err := ctxDoneError(ctx); err != nil {
		return nil, err
	}
	return s.deliver(ctx, call, inst, prompt)
}

// resolveSession looks the live instance up ONCE and checks it can take a
// request. A nil instance means the call ends with early/err instead.
func (s *GitHubUserService) resolveSession(call *nudgeCall, sessionID string) (inst *session.Instance, early *sessionv1.NudgeSessionForPRResponse, err error) {
	inst = s.nudge.nudger.FindLiveInstance(sessionID)
	if inst == nil {
		return nil, nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOT_RUNNING, nil, "", nudgeNotTrackedDetail), nil
	}
	snap := inst.Snapshot()
	if snap.Title != sessionID {
		// The id matched another instance (stable id or tmux name), not the linked title.
		return nil, nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_SESSION_NOT_LINKED, nil, "", nudgeNotLinkedDetail), nil
	}
	call.sessionID = inst.GetStableID()
	call.live = true
	if snap.Status.IsSuspended() {
		return nil, nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_PAUSED, nil, call.sessionID, nudgePausedDetail), nil
	}
	if !CanSteer(snap.Program) {
		return nil, nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("this session's program does not support requests from Up Next"))
	}
	return inst, nil, nil
}

// freshPrompt resolves the owning account's token, reads fresh PR state and
// builds the prompt. A non-nil early response ends the call (nothing to send).
func (s *GitHubUserService) freshPrompt(ctx context.Context, call *nudgeCall) (prompt string, reasons []sessionv1.NudgeReason, early *sessionv1.NudgeSessionForPRResponse, err error) {
	token, login, ok := s.tokenResolver().TokenForPR(call.key)
	if !ok {
		return "", nil, nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, nil, call.sessionID, nudgeNoAccountDetail), nil
	}
	call.account = login

	fetchCtx, cancel := s.nudgeClock().WithTimeout(ctx, nudgeFetchTimeout)
	defer cancel()
	detail, fetchErr := s.nudge.fetcher.FetchPRNudgeDetail(fetchCtx, call.key, token)
	if fetchErr != nil {
		early, err = s.fetchFailure(fetchCtx, call, fetchErr)
		return "", nil, early, err
	}
	prompt, reasons = BuildPRNudgePrompt(detail)
	if prompt == "" || len(reasons) == 0 {
		return "", nil, nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOTHING_TO_FIX, nil, call.sessionID, nudgeNothingToFixDetail), nil
	}
	return prompt, reasons, nil, nil
}

func (s *GitHubUserService) fetchFailure(fetchCtx context.Context, call *nudgeCall, err error) (*sessionv1.NudgeSessionForPRResponse, error) {
	switch {
	case errors.Is(err, githubpkg.ErrRateLimited):
		s.nudgeWarnf("nudge_fetch_failed", "pr", call.pr, "session_id", call.sessionID, "kind", "rate_limited", "err", err.Error())
		return nil, connect.NewError(connect.CodeResourceExhausted, errors.New(s.rateLimitMessage()))
	case errors.Is(err, githubpkg.ErrGitHubRefNotFound):
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_PR_NOT_FOUND, nil, call.sessionID, nudgePRNotFoundDetail), nil
	case errors.Is(err, context.DeadlineExceeded), errors.Is(context.Cause(fetchCtx), context.DeadlineExceeded):
		s.nudgeWarnf("nudge_fetch_failed", "pr", call.pr, "session_id", call.sessionID, "kind", "timeout", "err", err.Error())
		return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("GitHub did not answer in time; try again"))
	default:
		s.nudgeWarnf("nudge_fetch_failed", "pr", call.pr, "session_id", call.sessionID, "kind", "other", "err", err.Error())
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("could not read the PR from GitHub; try again"))
	}
}

// ctxDoneError maps an expired or cancelled ctx to the matching Connect code,
// so a lapsed deadline is never reported as an internal error.
func ctxDoneError(ctx context.Context) error {
	switch err := ctx.Err(); {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("the request timed out before it was sent; try again"))
	default:
		return connect.NewError(connect.CodeCanceled, errors.New("the request was cancelled before it was sent"))
	}
}

func (s *GitHubUserService) rateLimitMessage() string {
	resume := s.nudge.rateLimitResume
	if resume == nil {
		resume = githubpkg.DefaultRateLimiter.IsLimited
	}
	if limited, at := resume(); limited && !at.IsZero() {
		return "GitHub rate limited until " + at.UTC().Format(time.RFC3339)
	}
	return nudgeRateLimitedFallback
}

// deliver writes the prompt through the guarded steer and maps every
// SteerOutcome to a typed response or Connect error.
func (s *GitHubUserService) deliver(ctx context.Context, call *nudgeCall, inst *session.Instance, prompt string) (*sessionv1.NudgeSessionForPRResponse, error) {
	key, reasons := call.key, call.reasons
	if err := ctxDoneError(ctx); err != nil {
		return nil, err
	}
	outcome, err := s.nudge.nudger.SteerInstanceGuarded(ctx, inst, nudgeSignature(reasons), prompt)
	switch outcome {
	case SteerDelivered:
		if t := s.nudgeTracker(); t != nil {
			t.RecordNudge(key, nudgeReasonKinds(reasons), s.nudgeClock().Now())
		}
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, reasons, call.sessionID, "Request sent to the session."), nil
	case SteerDuplicate:
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_DUPLICATE, reasons, call.sessionID, "Already requested recently."), nil
	case SteerCoolingDown:
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, reasons, call.sessionID, nudgeCoolingDownDetail), nil
	case SteerGuardBusy, SteerBusy:
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, reasons, call.sessionID, nudgeBusyDetail), nil
	case SteerNoStatusSource:
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, reasons, call.sessionID, nudgeNoControllerDetail), nil
	case SteerNotTracked:
		return nudgeResponse(sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOT_RUNNING, reasons, call.sessionID, nudgeNotTrackedDetail), nil
	case SteerFailed:
		s.nudgeWarnf("nudge_steer_failed", "pr", call.pr, "session_id", call.sessionID, "err", fmt.Sprint(err))
		if ctxErr := ctxDoneError(ctx); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, ErrSteerPaneOwnership) {
			return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("the session's terminal could not be verified, so nothing was sent"))
		}
		return nil, connect.NewError(connect.CodeInternal, errors.New("could not deliver the request to the session"))
	default: // includes SteerUnspecified: never treated as delivered
		s.nudgeWarnf("nudge_steer_unexpected", "pr", call.pr, "session_id", call.sessionID, "outcome", outcome.String(), "err", fmt.Sprint(err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("unexpected steer outcome %s", outcome))
	}
}

// findPR matches by the full key (host, owner, repo, number); the cache's
// number-only matching would confuse same-numbered PRs across repos.
func (s *GitHubUserService) findPR(key githubpkg.PRKey) (githubpkg.UserPR, bool) {
	for _, pr := range s.prSnapshot().GetAll() {
		k, err := githubpkg.NewPRKey(pr.Host, pr.Owner, pr.Repo, pr.Number)
		if err == nil && k == key {
			return pr, true
		}
	}
	return githubpkg.UserPR{}, false
}

// linkedStrictly ignores LegacyFallback links: they matched only the old
// owner-only key and may belong to another repo.
func linkedStrictly(pr githubpkg.UserPR, sessionID string) bool {
	if sessionID == "" {
		return false
	}
	for _, ls := range pr.LinkedSessions {
		if !ls.LegacyFallback && ls.SessionID == sessionID {
			return true
		}
	}
	return false
}

// Canonical reason names shared with PR-fix auto-steer's duplicate key.
const (
	reasonNameFailingChecks     = "FAILING_CHECKS"
	reasonNameUnresolvedThreads = "UNRESOLVED_THREADS"
	reasonNameMergeConflict     = "MERGE_CONFLICT"
)

// nudgeSignature keys the duplicate window by the canonical reason set, the
// same key PR-fix auto-steer derives, so the two paths dedupe each other.
func nudgeSignature(reasons []sessionv1.NudgeReason) string {
	names := make([]string, 0, len(reasons))
	for _, r := range reasons {
		names = append(names, strings.TrimPrefix(r.String(), "NUDGE_REASON_"))
	}
	return reasonSetSignature(names...)
}

func nudgeReasonNames(reasons []sessionv1.NudgeReason) string {
	names := make([]string, 0, len(reasons))
	for _, r := range reasons {
		names = append(names, strings.TrimPrefix(r.String(), "NUDGE_REASON_"))
	}
	return strings.Join(names, ",")
}

func nudgeReasonKinds(reasons []sessionv1.NudgeReason) []githubpkg.NudgeReasonKind {
	out := make([]githubpkg.NudgeReasonKind, 0, len(reasons))
	for _, r := range reasons {
		switch r {
		case sessionv1.NudgeReason_NUDGE_REASON_FAILING_CHECKS:
			out = append(out, githubpkg.NudgeReasonFailingChecks)
		case sessionv1.NudgeReason_NUDGE_REASON_UNRESOLVED_THREADS:
			out = append(out, githubpkg.NudgeReasonUnresolvedThreads)
		case sessionv1.NudgeReason_NUDGE_REASON_MERGE_CONFLICT:
			out = append(out, githubpkg.NudgeReasonMergeConflict)
		case sessionv1.NudgeReason_NUDGE_REASON_UNSPECIFIED:
		}
	}
	return out
}

func nudgeResponse(outcome sessionv1.NudgeOutcome, reasons []sessionv1.NudgeReason, sessionID, detail string) *sessionv1.NudgeSessionForPRResponse {
	return &sessionv1.NudgeSessionForPRResponse{Outcome: outcome, Reasons: reasons, SessionId: sessionID, Detail: detail}
}

// logNudgeExit emits the one nudge_outcome line per call. It never logs prompt
// or comment text, and never a token.
func (s *GitHubUserService) logNudgeExit(call *nudgeCall, resp *sessionv1.NudgeSessionForPRResponse, err error) {
	outcome := "ERROR"
	args := []any{"pr", call.pr, "host", call.host, "account", call.account, "session_id", call.sessionID}
	if err != nil {
		args = append(args, "code", connect.CodeOf(err).String())
	} else {
		outcome = strings.TrimPrefix(resp.GetOutcome().String(), "NUDGE_OUTCOME_")
	}
	args = append(args,
		"outcome", outcome,
		"reasons", nudgeReasonNames(call.reasons),
		"latency_ms", s.nudgeClock().Now().Sub(call.startedAt).Milliseconds(),
		"prompt_bytes", call.promptBytes,
		"session_live", call.live)
	if call.attention != nil {
		args = append(args, "attention_age_s", *call.attention)
	}
	s.nudgeLogf("nudge_outcome", args...)
}

// linkedSessionSteerReady reports whether the nudge could be delivered to the
// linked session right now. It mirrors resolveSession's checks and then asks
// the shared idle gate; anything unconfirmed is false.
func (s *GitHubUserService) linkedSessionSteerReady(sessionID string) bool {
	if s.nudge.nudger == nil {
		return false
	}
	inst := s.nudge.nudger.FindLiveInstance(sessionID)
	if inst == nil {
		return false
	}
	snap := inst.Snapshot()
	if snap.Title != sessionID || snap.Status.IsSuspended() || !CanSteer(snap.Program) {
		return false
	}
	return s.nudge.nudger.InstanceReadyForSteer(inst)
}

// userPRsToProto converts PRs and marks each strictly linked session with its
// steer readiness. Readiness is as of this snapshot; a poll or click may find
// the session busy by then, which the nudge reports as BUSY. Only PRs the
// nudge could act on are probed, and each session is probed once per call.
func (s *GitHubUserService) userPRsToProto(prs []githubpkg.UserPR) []*sessionv1.UserPR {
	out := userPRsToProto(prs)
	ready := make(map[string]bool)
	for i, pr := range prs {
		if pr.IsDraft || (len(pr.FailingChecks) == 0 && !hasUnresolvedThreads(pr) && !hasMergeConflict(pr)) {
			continue
		}
		for j, ls := range pr.LinkedSessions {
			if ls.LegacyFallback || j >= len(out[i].LinkedSessions) {
				continue
			}
			r, seen := ready[ls.SessionID]
			if !seen {
				r = s.linkedSessionSteerReady(ls.SessionID)
				ready[ls.SessionID] = r
			}
			out[i].LinkedSessions[j].SteerReady = r
		}
	}
	return out
}

func hasUnresolvedThreads(pr githubpkg.UserPR) bool {
	return pr.UnresolvedThreadCount != nil && *pr.UnresolvedThreadCount > 0
}

func hasMergeConflict(pr githubpkg.UserPR) bool {
	return pr.HasMergeConflict != nil && *pr.HasMergeConflict
}

// funnelCounts is the up_next_funnel payload; logUpNextFunnel logs only when it changes.
type funnelCounts struct{ attention, linked, live int }

// logUpNextFunnel emits an up_next_funnel line when the counts change: how many
// PRs need attention, how many have a linked session, and how many of those
// have a steer-ready one. "Needs attention" is the same rule as the web badge
// (prAttention.ts): non-draft with an itemised failing check, a failing CI
// rollup, unresolved threads, a merge conflict or changes requested. Clicks and
// deliveries are the nudge_request and nudge_outcome lines. Logging only on
// change keeps one line per state, not one per client poll.
func (s *GitHubUserService) logUpNextFunnel(prs []*sessionv1.UserPR) {
	var c funnelCounts
	for _, pr := range prs {
		if pr.GetIsDraft() || !(len(pr.GetFailingChecks()) > 0 || pr.GetCheckConclusion() == "failure" ||
			pr.GetUnresolvedThreadCount() > 0 || pr.GetHasMergeConflict() || pr.GetChangesReqCount() > 0) {
			continue
		}
		c.attention++
		if len(pr.GetLinkedSessions()) == 0 {
			continue
		}
		c.linked++
		for _, ls := range pr.GetLinkedSessions() {
			if ls.GetSteerReady() {
				c.live++
				break
			}
		}
	}
	s.funnelMu.Lock()
	changed := !s.funnelLogged || s.funnelLast != c
	s.funnelLast, s.funnelLogged = c, true
	s.funnelMu.Unlock()
	if changed {
		s.nudgeLogf("up_next_funnel", "prs_needing_attention", c.attention, "with_linked_session", c.linked, "with_live_session", c.live)
	}
}
