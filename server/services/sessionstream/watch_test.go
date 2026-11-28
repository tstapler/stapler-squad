package sessionstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/federation"
	"github.com/tstapler/stapler-squad/session/instance"
)

// watchOnly serves a Handler's WatchSessions over Connect.
type watchOnly struct {
	sessionv1connect.UnimplementedSessionServiceHandler
	h *Handler
}

func (w watchOnly) WatchSessions(ctx context.Context, req *connect.Request[sessionv1.WatchSessionsRequest], stream *connect.ServerStream[sessionv1.SessionEvent]) error {
	return w.h.WatchSessions(ctx, req, stream)
}

func idleHandler(t *testing.T) *Handler {
	t.Helper()
	bus := events.NewEventBus(8)
	t.Cleanup(bus.Close)
	return New(bus, Ports{
		Polled:     func() ([]*instance.Instance, bool) { return nil, true },
		Federation: func() *federation.FederatedSessions { return nil },
	})
}

// openWatch returns the stream once its first message has arrived, so h must
// heartbeat quickly. cancel ends the stream.
func openWatch(t *testing.T, h *Handler) (first *sessionv1.SessionEvent, cancel func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.Handle(sessionv1connect.NewSessionServiceHandler(watchOnly{h: h}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(stop)
	stream, err := sessionv1connect.NewSessionServiceClient(srv.Client(), srv.URL).
		WatchSessions(ctx, connect.NewRequest(&sessionv1.WatchSessionsRequest{}))
	require.NoError(t, err)
	require.True(t, stream.Receive(), "no event: %v", stream.Err())
	return stream.Msg(), func() {
		stop()
		_ = stream.Close()
	}
}

func openWatchStreams(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()
	var got metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &got))
	for _, scope := range got.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "rpc.server_streams.open" {
				continue
			}
			for _, p := range m.Data.(metricdata.Gauge[int64]).DataPoints {
				if v, ok := p.Attributes.Value("method"); ok && v.AsString() == "WatchSessions" {
					return p.Value
				}
			}
		}
	}
	return 0
}

// meterReader installs the meter provider once: the global meter delegates only
// to the first one set, so a repeated run has to reuse it.
var meterReader = sync.OnceValue(func() *sdkmetric.ManualReader {
	reader := sdkmetric.NewManualReader()
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)))
	return reader
})

// Not parallel: installs the process-wide meter provider and reads a
// process-wide count, so no other stream may be open.
func TestWatchSessions_ShouldCountItselfAsAnOpenStream_When_Open(t *testing.T) {
	reader := meterReader()
	h := idleHandler(t)
	h.heartbeatEvery = 10 * time.Millisecond

	_, cancel := openWatch(t, h)
	assert.EqualValues(t, 1, openWatchStreams(t, reader))
	cancel()
	require.Eventually(t, func() bool { return openWatchStreams(t, reader) == 0 }, 5*time.Second, 5*time.Millisecond)
}

func TestWatchSessions_ShouldSendAHeartbeatEveryInterval_When_Idle(t *testing.T) {
	t.Parallel()
	h := idleHandler(t)
	assert.Equal(t, events.HeartbeatInterval, h.heartbeatEvery, "the production interval")
	h.heartbeatEvery = 10 * time.Millisecond

	ev, cancel := openWatch(t, h)
	defer cancel()
	assert.True(t, ev.Heartbeat)
	assert.NotNil(t, ev.Timestamp)
	assert.Nil(t, ev.Event)
}
