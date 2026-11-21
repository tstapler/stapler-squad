package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// callUnverified invokes name as a caller that only claims an identity, which
// the identity gate refuses and counts in its wake metrics.
func callUnverified(t *testing.T, opts []CoreOption, name string) {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	s := NewCore(&stubStore{}, nil, nil, storage, nil, nil, nil, nil, nil, nil, opts...)
	tool := s.GetTool(name)
	require.NotNil(t, tool, "tool %q must be registered", name)
	res, err := tool.Handler(WithSessionUUID(context.Background(), "claimed"), makeToolReq(probeArgs))
	require.NoError(t, err)
	require.Equal(t, CodeIdentityUnverified, resultCode(parseResult(t, res)))
}

func TestNewCore_should_CountIdentityGateDenialsIntoInjectedMetrics(t *testing.T) {
	t.Parallel()
	for _, tool := range []string{toolScheduleWake, toolRequestHelp, toolReportPRCreated} {
		t.Run(tool, func(t *testing.T) {
			t.Parallel()
			m := wakes.NewMetrics()
			callUnverified(t, []CoreOption{WithWakeMetrics(m)}, tool)
			assert.Equal(t, int64(1), m.Value(wakes.MetricIdentityUnverified, tool),
				"the gate must count into the instance the composition root injected, not a private one")
		})
	}
}

func TestNewCore_should_NotPanic_When_WakeMetricsAbsent(t *testing.T) {
	t.Parallel()
	assert.Nil(t, WakeMetricsFromOptions())
	for _, tool := range []string{toolScheduleWake, toolReportPRCreated} {
		callUnverified(t, nil, tool)
	}
}

func TestWithWakeMetrics_should_CarryCallersInstance(t *testing.T) {
	t.Parallel()
	m := wakes.NewMetrics()
	assert.Same(t, m, WakeMetricsFromOptions(WithWakeMetrics(m)))
}
