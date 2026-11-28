package prs

// Delivery vocabulary shared by Up Next's nudge and a session's "Ask to fix":
// both build a prompt from fresh PR state and type it through the guarded steer.

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/services/access"
)

const (
	// NudgeFetchTimeout bounds the fresh GitHub read; NudgeRPCTimeout bounds the whole call.
	NudgeFetchTimeout = 5 * time.Second
	NudgeRPCTimeout   = 8 * time.Second

	NudgeBusyDetail          = "Session is busy. Try again when it is idle."
	NudgeNoControllerDetail  = "Session isn't being monitored, so it can't safely take a request. Open it to restart it."
	NudgePausedDetail        = "Session paused. Open it to resume"
	NudgeNotTrackedDetail    = "Session is not running or was deleted. Open its page to restart it."
	NudgeCoolingDownDetail   = "The last request to this session failed. Try again in a few seconds."
	NudgeNoAccountDetail     = "The GitHub account for this PR is no longer connected."
	NudgeNothingToFixDetail  = "Nothing to fix right now."
	NudgePRNotFoundDetail    = "PR not found."
	nudgeRateLimitedFallback = "GitHub rate limit reached; try again shortly"
)

// PRTokenResolver returns the token and login of the account that owns a PR.
// There is deliberately no cross-account fallback. Implemented by UserPRCache.
type PRTokenResolver interface {
	TokenForPR(key githubpkg.PRKey) (token, login string, ok bool)
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

// NudgeFetchKind classifies a failed fresh-PR read, for logs.
type NudgeFetchKind string

const (
	NudgeFetchNotFound    NudgeFetchKind = "not_found"
	NudgeFetchRateLimited NudgeFetchKind = "rate_limited"
	NudgeFetchTimeoutKind NudgeFetchKind = "timeout"
	NudgeFetchOther       NudgeFetchKind = "other"
)

// ClassifyNudgeFetchError maps a failed fresh-PR read to its kind and Connect
// error (nil for not_found, which callers answer with a typed outcome).
func ClassifyNudgeFetchError(fetchCtx context.Context, err error, rateLimitMessage func() string) (NudgeFetchKind, error) {
	switch {
	case errors.Is(err, githubpkg.ErrRateLimited):
		return NudgeFetchRateLimited, connect.NewError(connect.CodeResourceExhausted, errors.New(rateLimitMessage()))
	case errors.Is(err, githubpkg.ErrGitHubRefNotFound):
		return NudgeFetchNotFound, nil
	case errors.Is(err, context.DeadlineExceeded), errors.Is(context.Cause(fetchCtx), context.DeadlineExceeded):
		return NudgeFetchTimeoutKind, connect.NewError(connect.CodeDeadlineExceeded, errors.New("GitHub did not answer in time; try again"))
	default:
		return NudgeFetchOther, connect.NewError(connect.CodeUnavailable, errors.New("could not read the PR from GitHub; try again"))
	}
}

// CtxDoneError maps an expired or cancelled ctx to the matching Connect code,
// so a lapsed deadline is never reported as an internal error.
func CtxDoneError(ctx context.Context) error {
	switch err := ctx.Err(); {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		return connect.NewError(connect.CodeDeadlineExceeded, errors.New("the request timed out before it was sent; try again"))
	default:
		return connect.NewError(connect.CodeCanceled, errors.New("the request was cancelled before it was sent"))
	}
}

// NudgeRateLimitMessage words the rate-limit error; a nil resume reads the default limiter.
func NudgeRateLimitMessage(resume func() (bool, time.Time)) string {
	if resume == nil {
		resume = githubpkg.DefaultRateLimiter.IsLimited
	}
	if limited, at := resume(); limited && !at.IsZero() {
		return "GitHub rate limited until " + at.UTC().Format(time.RFC3339)
	}
	return nudgeRateLimitedFallback
}

// NudgeFromSteer maps a not-failed SteerOutcome to its typed nudge outcome and
// detail; ok is false for SteerFailed and anything unknown.
func NudgeFromSteer(outcome access.SteerOutcome) (no sessionv1.NudgeOutcome, detail string, ok bool) {
	switch outcome {
	case access.SteerDelivered:
		return sessionv1.NudgeOutcome_NUDGE_OUTCOME_DELIVERED, "Request sent to the session.", true
	case access.SteerDuplicate:
		return sessionv1.NudgeOutcome_NUDGE_OUTCOME_DUPLICATE, "Already requested recently.", true
	case access.SteerCoolingDown:
		return sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, NudgeCoolingDownDetail, true
	case access.SteerGuardBusy, access.SteerBusy:
		return sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, NudgeBusyDetail, true
	case access.SteerNoStatusSource:
		return sessionv1.NudgeOutcome_NUDGE_OUTCOME_BUSY, NudgeNoControllerDetail, true
	case access.SteerNotTracked:
		return sessionv1.NudgeOutcome_NUDGE_OUTCOME_NOT_RUNNING, NudgeNotTrackedDetail, true
	}
	return 0, "", false
}

// SteerFailedError is the Connect error for a SteerFailed outcome: a lapsed
// context first, then a pane that could not be verified, else internal.
func SteerFailedError(ctx context.Context, err error) error {
	if ctxErr := CtxDoneError(ctx); ctxErr != nil {
		return ctxErr
	}
	if errors.Is(err, access.ErrSteerPaneOwnership) {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("the session's terminal could not be verified, so nothing was sent"))
	}
	return connect.NewError(connect.CodeInternal, errors.New("could not deliver the request to the session"))
}

// NudgeSignature keys the duplicate window by PR and canonical reason set, the
// same key PR-fix auto-steer derives for the same PR, so the two paths dedupe
// each other without conflating two PRs linked to one session.
func NudgeSignature(pr githubpkg.PRKey, reasons []sessionv1.NudgeReason) string {
	names := make([]string, 0, len(reasons))
	for _, r := range reasons {
		names = append(names, strings.TrimPrefix(r.String(), "NUDGE_REASON_"))
	}
	return access.GuardSignature(pr.Key(), access.ReasonSetSignature(names...))
}

// NudgeReasonNames lists reasons without their enum prefix, for log lines.
func NudgeReasonNames(reasons []sessionv1.NudgeReason) string {
	names := make([]string, 0, len(reasons))
	for _, r := range reasons {
		names = append(names, strings.TrimPrefix(r.String(), "NUDGE_REASON_"))
	}
	return strings.Join(names, ",")
}
