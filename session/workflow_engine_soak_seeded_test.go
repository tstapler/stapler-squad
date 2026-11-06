package session

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluateSoak_SeededDivergenceStaysNotReady_InAnEditedWindow_EvenWhenEdgeAcked(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	cl := l.Classes[string(ClassOperator)]
	cl.ConfigSeen[ConfigEdited] = 1
	cl.Divergence["CanTransition|graph|seeded|transition"] = 2
	cl.Edges["done->refining"] = EdgeStats{TransitionSamples: 25, Divergences: 2}

	v := evalOperator(l, func(p *SoakParams) { p.Acks.DivergentEdges = []string{"done->refining"} })

	require.Equal(t, ConfigEdited, v.ConfigWindow)
	require.Equal(t, VerdictNotReady, v.Status, "an ack must not clear seeded-config divergence")
	require.Contains(t, strings.Join(v.Reasons, "\n"), "seeded config")
}
