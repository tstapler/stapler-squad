package backlog

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

func TestEvaluateSoak_SeededDivergenceStaysNotReady_InAnEditedWindow_EvenWhenEdgeAcked(t *testing.T) {
	t.Parallel()
	l := readyLedger()
	cl := l.Classes[string(domain.ClassOperator)]
	cl.ConfigSeen[workflowengine.ConfigEdited] = 1
	cl.Divergence["CanTransition|graph|seeded|transition"] = 2
	cl.Edges["done->refining"] = workflowengine.EdgeStats{TransitionSamples: 25, Divergences: 2}

	v := evalOperator(l, func(p *workflowengine.SoakParams) { p.Acks.DivergentEdges = []string{"done->refining"} })

	require.Equal(t, workflowengine.ConfigEdited, v.ConfigWindow)
	require.Equal(t, workflowengine.VerdictNotReady, v.Status, "an ack must not clear seeded-config divergence")
	require.Contains(t, strings.Join(v.Reasons, "\n"), "seeded config")
}
