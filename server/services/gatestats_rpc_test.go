package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// T-OB-10 (aggregate-counters half; listener half in server/gatestats_listener_test.go).
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

// T-OB-10 (no-MCP-tool half): the stats RPC has no MCP tool (aggregate counters
// are an operator surface).
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

// T-OB-23: the stats response carries routine_events_while_on, last_off_flip_at
// and the writable flag.
func TestStatsResponse_ShouldCarryRoutineEventsWhileOnLastOffFlipAtAndWritableFlag(t *testing.T) {
	t.Parallel()
	clk := newGateTestClock()
	var on atomic.Bool
	on.Store(true)
	lg, _ := newLogCapture()
	g := deliverygate.NewGate(
		deliverygate.WithClock(clk.Now), deliverygate.WithLogger(lg),
		deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
			return deliverygate.FlagSettings{Global: on.Load()}, nil
		}),
	)
	g.Flags().Reload()
	g.Index().Replace([]deliverygate.Entry{{UUID: "u-h1", Title: "review:t", Hidden: true, Kind: deliverygate.KindReview}})

	ns := &NotificationService{}
	ns.SetDeliveryGate(g)
	writable := false
	ns.SetGateStatsFileStatus(func() deliverygate.StatsFileStatus {
		return deliverygate.StatsFileStatus{Loaded: true, Writable: writable}
	})

	clk.Advance(10 * time.Minute)
	on.Store(false)
	g.Flags().Reload()
	offAt := clk.Now()
	clk.Advance(10 * time.Minute)
	on.Store(true)
	g.Flags().Reload()
	// Routine events count only in hour buckets after the one holding the last
	// off flip, so the event lands an hour later.
	clk.Advance(time.Hour)
	g.Stats().Tick(clk.Now()) // what the stats writer goroutine does each minute
	require.False(t, g.PublishFilter()(events.NewNotificationEvent("review:t", "review:t", "n1",
		int32(sessionv1.NotificationType_NOTIFICATION_TYPE_TASK_COMPLETE), 2, "t", "m", nil)))

	get := func() *sessionv1.GetDeliveryGateStatsResponse {
		resp, err := ns.GetDeliveryGateStats(context.Background(), connect.NewRequest(&sessionv1.GetDeliveryGateStatsRequest{}))
		require.NoError(t, err)
		return resp.Msg
	}
	m := get()
	assert.EqualValues(t, 1, m.Soak.RoutineEventsWhileOn)
	require.NotNil(t, m.Soak.LastOffFlipAt)
	assert.True(t, m.Soak.LastOffFlipAt.AsTime().Equal(offAt), "last_off_flip_at = %v, want %v", m.Soak.LastOffFlipAt.AsTime(), offAt)
	assert.False(t, m.StatsFileStatus.Writable, "an unwritable stats file is reported as such")
	writable = true
	assert.True(t, get().StatsFileStatus.Writable)
}
