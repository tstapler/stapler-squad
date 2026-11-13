package session

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/internal/sessiontest"
)

func TestOverallSoakExit_IsOneForNoVerdictsAndForAnyNotReady(t *testing.T) {
	t.Parallel()
	require.Equal(t, 1, OverallSoakExit(nil))
	require.Equal(t, 1, OverallSoakExit([]Verdict{{ExitCode: 0}, {ExitCode: 1}}))
	require.Equal(t, 0, OverallSoakExit([]Verdict{{ExitCode: 0}}))
}

// --- Story 1.5.2d: registry expected edges, guard_new and the ack file ------------

func TestExpectedEdgesForClass_TakenFromRegistry_ExpandsArchiveWildcard_SkipsForwarding(t *testing.T) {
	t.Parallel()
	keys := func(c domain.CallerClass) map[string]bool {
		out := map[string]bool{}
		for _, e := range ExpectedEdgesForClass(c) {
			out[EdgeKey(e.From, e.To)] = true
		}
		return out
	}
	agent, system, operator := keys(domain.ClassAgent), keys(domain.ClassSystem), keys(domain.ClassOperator)
	require.True(t, agent["in_progress->review"], "request_review")
	require.True(t, agent["review->archived"], "report_duplicate's *->archived is expanded")
	require.False(t, agent["ready->queued"], "queue_backlog_item is a system caller")
	require.True(t, system["ready->queued"])
	require.True(t, system["pr_pending->done"])
	require.True(t, operator["review->done"])
	require.NotEmpty(t, ExpectedEdgesForClass(domain.ClassOperator))
	require.Empty(t, ExpectedEdgesForClass(domain.CallerClass("bogus")))
	for _, c := range domain.TransitionCallers() {
		if c.Forwarding {
			require.Empty(t, c.Edges, "a forwarder declares no edge, so it adds none")
		}
	}
	require.Equal(t, ExpectedEdgesForClass(domain.ClassSystem), DefaultSoakParams(domain.ClassSystem, sessiontest.SoakNow).ExpectedEdges)
}

func TestParseSoakAckFile_IsStrict_AndMergesPerClass(t *testing.T) {
	t.Parallel()
	f, err := ParseSoakAckFile([]byte(`{"classes":{"agent":{"edges":["a->b"],"guard_new":["c|a->b"],"low_volume":{"config_diff":true}}}}`))
	require.NoError(t, err)
	require.Equal(t, []string{"a->b"}, f.For(domain.ClassAgent).Edges)
	require.Empty(t, f.For(domain.ClassOperator).Edges)
	m := f.For(domain.ClassAgent).Merge(SoakAcks{Edges: []string{"x->y"}, LowVolume: LowVolumeAck{ReplayReport: true}})
	require.Equal(t, []string{"a->b", "x->y"}, m.Edges)
	require.True(t, m.LowVolume.ConfigDiff && m.LowVolume.ReplayReport)

	for name, doc := range map[string]string{
		"unknown field": `{"classes":{"agent":{"edgez":["a->b"]}}}`,
		"unknown class": `{"classes":{"robot":{}}}`,
		"not json":      `edges: [a->b]`,
		"second value":  `{"classes":{}}{"classes":{"agent":{"edges":["a->b"]}}}`,
		"stray brace":   `{"classes":{}}}`,
	} {
		_, err := ParseSoakAckFile([]byte(doc))
		require.Errorf(t, err, name)
	}
}
