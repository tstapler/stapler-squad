package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/prototext"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

func newStatsRPCGate(t *testing.T, clk *gateTestClock) *deliverygate.Gate {
	t.Helper()
	lg, _ := newLogCapture()
	g := deliverygate.NewGate(
		deliverygate.WithClock(clk.Now), deliverygate.WithLogger(lg),
		deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
			return deliverygate.FlagSettings{Global: true}, nil
		}),
	)
	g.Flags().Reload()
	g.Index().Replace([]deliverygate.Entry{
		{UUID: "u-h1", Title: "review:secret-title", TmuxName: "ssq_review", Hidden: true, Kind: deliverygate.KindReview},
	})
	return g
}

func TestGetDeliveryGateStats_ShouldReturnSinceProcessStartTotalsAndBucketsWithAggregateCountersOnly_WhenTelemetryUninitialized(t *testing.T) {
	t.Parallel()
	clk := newGateTestClock()
	g := newStatsRPCGate(t, clk)
	ns := &NotificationService{}
	ns.SetDeliveryGate(g)
	ns.SetGateStatsFileStatus(func() deliverygate.StatsFileStatus {
		return deliverygate.StatsFileStatus{Loaded: true, Writable: true}
	})

	f := g.PublishFilter()
	routine := events.NewNotificationEvent("review:secret-title", "review:secret-title", "n1",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), 2, "secret body title", "secret message", nil)
	failure := events.NewNotificationEvent("review:secret-title", "review:secret-title", "n2",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_ERROR), 2, "t", "m", nil)
	unknown := events.NewNotificationEvent("nope", "nope", "n3",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), 2, "t", "m", nil)
	require.False(t, f(routine))
	require.True(t, f(failure))
	require.True(t, f(unknown))
	g.CountUnversionedRequest()
	clk.Advance(30 * time.Minute)

	resp, err := ns.GetDeliveryGateStats(context.Background(), connect.NewRequest(&sessionv1.GetDeliveryGateStatsRequest{}))
	require.NoError(t, err)
	m := resp.Msg

	byName := map[string]int64{}
	for _, c := range m.SinceProcessStart {
		byName[c.Counter] += c.Count
	}
	suppressed, delivered, unversioned := int64(0), int64(0), byName["rpc_unversioned"]
	for name, n := range byName {
		switch {
		case strings.HasPrefix(name, "suppressed{"):
			suppressed += n
		case strings.HasPrefix(name, "hidden_delivered{"):
			delivered += n
		}
	}
	assert.Equal(t, int64(1), suppressed)
	assert.Equal(t, int64(1), delivered)
	assert.Equal(t, int64(1), unversioned)
	assert.Equal(t, int64(1), byName["unresolved{}"]+byName["unresolved"])

	require.Len(t, m.Buckets, 1)
	assert.Equal(t, int64(2), m.Buckets[0].HiddenEventsSeen)
	assert.Equal(t, int64(1), m.Buckets[0].RoutineEventsWhileOn)
	assert.Equal(t, int32(1800), m.Buckets[0].UptimeSeconds)
	assert.Equal(t, int32(1800), m.Buckets[0].GateOnSeconds)
	assert.Equal(t, int64(2), m.EventsByKind_24H["review"])
	assert.True(t, m.StatsFileStatus.Writable)
	assert.EqualValues(t, 1800, m.UptimeSeconds)
	assert.Equal(t, int64(1), m.Soak.FailureDeliveredWhileOn)

	// Aggregate counters only: no session id, title or message text anywhere.
	text := prototext.Format(m)
	for _, leak := range []string{"secret-title", "secret body", "secret message", "u-h1", "ssq_review"} {
		assert.NotContains(t, text, leak)
	}
}

func TestGetDeliveryGateStats_ShouldBeUnavailable_WhenNoGateIsConfigured(t *testing.T) {
	t.Parallel()
	_, err := (&NotificationService{}).GetDeliveryGateStats(context.Background(),
		connect.NewRequest(&sessionv1.GetDeliveryGateStatsRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

// The stats RPC has no MCP tool (aggregate counters are an operator surface).
func TestGetDeliveryGateStats_ShouldHaveNoMCPTool_WhenMCPToolsAreScanned(t *testing.T) {
	t.Parallel()
	err := filepath.WalkDir("../mcp", func(p string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return werr
		}
		b, rerr := os.ReadFile(p)
		require.NoError(t, rerr)
		assert.NotContains(t, string(b), "GetDeliveryGateStats", p)
		return nil
	})
	require.NoError(t, err)
}
