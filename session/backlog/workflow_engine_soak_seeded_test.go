package backlog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
)

func TestEvaluateSoak_SeededDivergenceStaysNotReady_InAnEditedWindow_EvenWhenEdgeAcked(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	cl := l.Classes[string(session.ClassOperator)]
	cl.ConfigSeen[session.ConfigEdited] = 1
	cl.Divergence["CanTransition|graph|seeded|transition"] = 2
	cl.Edges["done->refining"] = session.EdgeStats{TransitionSamples: 25, Divergences: 2}

	v := evalOperator(l, func(p *session.SoakParams) { p.Acks.DivergentEdges = []string{"done->refining"} })

	require.Equal(t, session.ConfigEdited, v.ConfigWindow)
	require.Equal(t, session.VerdictNotReady, v.Status, "an ack must not clear seeded-config divergence")
	require.Contains(t, strings.Join(v.Reasons, "\n"), "seeded config")
}
