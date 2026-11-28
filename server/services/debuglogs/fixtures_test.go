package debuglogs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

const ciConclusionFailure = "failure"

func multiPRInstance(t *testing.T, sessionID, scalarConclusion string, prs ...domain.PRView) *instance.Instance {
	t.Helper()
	return &instance.Instance{
		Title: sessionID, UUID: sessionID, PRs: prs, HasPRLinks: true,
		GitHubPRNumber: 12, GitHubCheckConclusion: scalarConclusion,
	}
}

func prViewFor(t *testing.T, number int, state domain.PRState, conclusion string) domain.PRView {
	t.Helper()
	ref, err := domain.NewPRRef("", "acme", "app", number)
	require.NoError(t, err)
	return domain.PRView{
		Ref: ref, State: state, StoredTerminal: state.Terminal(), CheckConclusion: conclusion,
		AttachedAt: time.Date(2026, 11, 1, 0, 0, number, 0, time.UTC),
	}
}
