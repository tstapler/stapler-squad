package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	githubpkg "github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/server/prwatch"
	"github.com/tstapler/stapler-squad/server/services"
)

type prwatchSpy struct {
	calls  []string
	hidden bool
	sigs   []string
	out    services.SteerOutcome
}

func (p *prwatchSpy) IsHiddenSession(string) bool {
	p.calls = append(p.calls, "IsHiddenSession")
	return p.hidden
}

func (p *prwatchSpy) SteerSessionGuarded(_ context.Context, _, sig, _ string) (services.SteerOutcome, error) {
	p.calls = append(p.calls, "SteerSessionGuarded")
	p.sigs = append(p.sigs, sig)
	return p.out, nil
}

// The spy implements only the two-method subset, so IsReadyForSteer and
// SteerActiveSession cannot be called by construction.
func TestPRWatchSteerer_should_CheckHiddenThenGuardedSteerWithoutIsReadyPrecheck_When_Steering(t *testing.T) {
	spy := &prwatchSpy{out: services.SteerDelivered}
	out, err := prwatchSteerer{svc: spy}.Steer(context.Background(), prwatch.SteerRequest{
		SessionUUID: "u", Repo: "o/r", PR: 3, Kinds: []prwatch.Kind{prwatch.KindCI}, Text: "t"})
	require.NoError(t, err)
	assert.Equal(t, prwatch.OutcomeDelivered, out)
	assert.Equal(t, []string{"IsHiddenSession", "SteerSessionGuarded"}, spy.calls)
}

func TestPRWatchSteerer_should_RefuseWithoutSteering_When_SessionHidden(t *testing.T) {
	spy := &prwatchSpy{hidden: true, out: services.SteerDelivered}
	out, err := prwatchSteerer{svc: spy}.Steer(context.Background(), prwatch.SteerRequest{
		SessionUUID: "u", Repo: "o/r", PR: 3, Kinds: []prwatch.Kind{prwatch.KindCI}})
	require.Error(t, err)
	assert.NotEqual(t, prwatch.OutcomeDelivered, out)
	assert.Equal(t, []string{"IsHiddenSession"}, spy.calls)
}

func TestPRWatchSteerer_should_MapEverySteerOutcomeToAPortOutcome_When_Converted(t *testing.T) {
	for o := services.SteerUnspecified; !isUnknownSteerOutcome(o); o++ {
		got := prwatchOutcome(o)
		assert.NotEmpty(t, got, o.String())
		if o == services.SteerUnspecified {
			assert.Equal(t, prwatch.OutcomeUnspecified, got)
		} else {
			assert.NotEqual(t, prwatch.OutcomeUnspecified, got, o.String())
			assert.Equal(t, o.String(), string(got))
		}
	}
	assert.Equal(t, prwatch.OutcomeUnspecified, prwatchOutcome(services.SteerOutcome(999)))
}

func isUnknownSteerOutcome(o services.SteerOutcome) bool {
	return len(o.String()) > 12 && o.String()[:12] == "SteerOutcome"
}

func TestPRWatchSteerer_should_PassStablePerPRPerKindSignature_When_Steering(t *testing.T) {
	spy := &prwatchSpy{out: services.SteerDelivered}
	s := prwatchSteerer{svc: spy}
	steer := func(repo string, pr int, kinds ...prwatch.Kind) {
		_, err := s.Steer(context.Background(), prwatch.SteerRequest{SessionUUID: "u", Repo: repo, PR: pr, Kinds: kinds})
		require.NoError(t, err)
	}
	steer("o/r", 3, prwatch.KindCI)
	steer("o/r", 3, prwatch.KindCI)
	steer("o/r", 4, prwatch.KindCI)
	assert.Equal(t, spy.sigs[0], spy.sigs[1])
	assert.NotEqual(t, spy.sigs[0], spy.sigs[2])

	ref, err := githubpkg.NewRepoRef("o", "r")
	require.NoError(t, err)
	assert.Equal(t, services.PRWatchGuardSignature(ref.PRKey(3), services.PRWatchKindCI), spy.sigs[0])
}

func TestPRWatchKindMapping_should_MapEveryPrwatchKindToServicesKind_When_Converted(t *testing.T) {
	want := map[prwatch.Kind]services.PRWatchKind{
		prwatch.KindCI:            services.PRWatchKindCI,
		prwatch.KindComment:       services.PRWatchKindComment,
		prwatch.KindReviewComment: services.PRWatchKindComment,
		prwatch.KindReview:        services.PRWatchKindReview,
	}
	for k, w := range want {
		got, err := prwatchKindToServices(k)
		require.NoError(t, err, string(k))
		assert.Equal(t, w, got, string(k))
	}
	_, err := prwatchKindToServices(prwatch.Kind("bogus"))
	assert.Error(t, err)

	spy := &prwatchSpy{}
	_, err = prwatchSteerer{svc: spy}.Steer(context.Background(), prwatch.SteerRequest{
		SessionUUID: "u", Repo: "o/r", PR: 1, Kinds: []prwatch.Kind{"bogus"}})
	require.Error(t, err)
	assert.Equal(t, []string{"IsHiddenSession"}, spy.calls)
}
