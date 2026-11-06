package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

type fakePRLookup struct{ uuids []string }

func (f fakePRLookup) SessionUUIDsByPR(context.Context, session.PRRef) ([]string, error) {
	return f.uuids, nil
}

type prUnknownRecorder struct{ sessions []string }

func (r *prUnknownRecorder) Notify(string, string, string, int32, bool, bool) {}
func (r *prUnknownRecorder) NotifySession(id, _, _ string, _ int32, _, _ bool) {
	r.sessions = append(r.sessions, id)
}

func TestPRUnknownNotifier_should_NotifyEachLinkedSession_When_PRBecomesUnknown(t *testing.T) {
	t.Parallel()
	ref, err := session.NewPRRef("github.com", "acme", "app", 12)
	require.NoError(t, err)
	rec := &prUnknownRecorder{}
	newPRUnknownNotifier(fakePRLookup{uuids: []string{"s1", "s2"}}, rec)(session.PRView{Ref: ref, PollError: "NETWORK"})
	require.Equal(t, []string{"s1", "s2"}, rec.sessions)
}

func TestPRUnknownNotifier_should_KeyByPR_When_NoSessionLinked(t *testing.T) {
	t.Parallel()
	ref, err := session.NewPRRef("github.com", "acme", "app", 12)
	require.NoError(t, err)
	rec := &prUnknownRecorder{}
	newPRUnknownNotifier(fakePRLookup{}, rec)(session.PRView{Ref: ref})
	require.Equal(t, []string{ref.String()}, rec.sessions)
}
