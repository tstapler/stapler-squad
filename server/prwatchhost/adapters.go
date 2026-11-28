package prwatchhost

import (
	"context"
	"errors"
	"fmt"
	"strings"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/prs"
)

// Adapters from the server/prwatch ports onto the services they stand in for.
// server/prwatch never imports server/services; this file is the seam
// (ADR-001 decision 6).

// prwatchSessionSteerer is the SessionService subset the steer adapter uses.
type prwatchSessionSteerer interface {
	IsHiddenSession(sessionUUID string) bool
	SteerSessionGuarded(ctx context.Context, sessionUUID, sig, msg string) (access.SteerOutcome, error)
}

// prwatchSteerer implements prwatch.Steerer. It checks IsHiddenSession, then
// steers through SteerSessionGuarded; readiness is applied inside the guard, so
// no IsReadyForSteer pre-check (it would only add a check-then-write gap).
type prwatchSteerer struct{ svc prwatchSessionSteerer }

var _ prwatch.Steerer = prwatchSteerer{}

func (s prwatchSteerer) Steer(ctx context.Context, req prwatch.SteerRequest) (prwatch.Outcome, error) {
	if s.svc.IsHiddenSession(req.SessionUUID) {
		return prwatch.OutcomeFailed, errors.New("prwatch: refusing to steer a hidden session")
	}
	owner, name, ok := strings.Cut(req.Repo, "/")
	if !ok {
		return prwatch.OutcomeFailed, fmt.Errorf("prwatch: repo %q is not owner/name", req.Repo)
	}
	ref, err := githubpkg.NewRepoRef(owner, name)
	if err != nil {
		return prwatch.OutcomeFailed, err
	}
	kinds := make([]prs.PRWatchKind, 0, len(req.Kinds))
	for _, k := range req.Kinds {
		sk, err := prwatchKindToServices(k)
		if err != nil {
			return prwatch.OutcomeFailed, err
		}
		kinds = append(kinds, sk)
	}
	sig := prs.PRWatchGuardSignature(ref.PRKey(req.PR), kinds...)
	out, err := s.svc.SteerSessionGuarded(ctx, req.SessionUUID, sig, req.Text)
	return prwatchOutcome(out), err
}

// prwatchKindToServices refuses an unknown kind rather than defaulting.
func prwatchKindToServices(k prwatch.Kind) (prs.PRWatchKind, error) {
	switch k {
	case prwatch.KindCI:
		return prs.PRWatchKindCI, nil
	case prwatch.KindComment, prwatch.KindReviewComment:
		return prs.PRWatchKindComment, nil
	case prwatch.KindReview:
		return prs.PRWatchKindReview, nil
	}
	return prs.PRWatchKindUnspecified, fmt.Errorf("prwatch: unknown kind %q", k)
}

func prwatchOutcome(o access.SteerOutcome) prwatch.Outcome {
	switch o {
	case access.SteerDelivered:
		return prwatch.OutcomeDelivered
	case access.SteerGuardBusy:
		return prwatch.OutcomeGuardBusy
	case access.SteerDuplicate:
		return prwatch.OutcomeDuplicate
	case access.SteerCoolingDown:
		return prwatch.OutcomeCoolingDown
	case access.SteerBusy:
		return prwatch.OutcomeBusy
	case access.SteerNoStatusSource:
		return prwatch.OutcomeNoStatusSource
	case access.SteerNotTracked:
		return prwatch.OutcomeNotTracked
	case access.SteerFailed:
		return prwatch.OutcomeFailed
	}
	return prwatch.OutcomeUnspecified
}
