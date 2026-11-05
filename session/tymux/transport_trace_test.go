package tymux

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	v1 "github.com/tstapler/tymux/clients/go/gen/tymux/v1"
	"github.com/tstapler/tymux/clients/go/gen/tymux/v1/tymuxv1connect"
)

// traceCapturingHandler records the traceparent header tymuxd would receive.
type traceCapturingHandler struct {
	tymuxv1connect.UnimplementedTymuxServiceHandler
	mu          sync.Mutex
	traceparent string
}

func (h *traceCapturingHandler) ListSessions(_ context.Context, req *connect.Request[v1.ListSessionsRequest]) (*connect.Response[v1.ListSessionsResponse], error) {
	h.mu.Lock()
	h.traceparent = req.Header().Get("Traceparent")
	h.mu.Unlock()
	return connect.NewResponse(&v1.ListSessionsResponse{}), nil
}

var traceProviderOnce sync.Once
var traceSpanRecorder *tracetest.SpanRecorder

// installTraceProvider sets the process-global provider and W3C propagator
// once; OTel's global only takes the first SetTracerProvider (see
// session/backend_observability_test.go for the same constraint).
func installTraceProvider() *tracetest.SpanRecorder {
	traceProviderOnce.Do(func() {
		traceSpanRecorder = tracetest.NewSpanRecorder()
		otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(traceSpanRecorder)))
		otel.SetTextMapPropagator(propagation.TraceContext{})
	})
	traceSpanRecorder.Reset()
	return traceSpanRecorder
}

func TestNewTymuxClient_PropagatesTraceContextToDaemon(t *testing.T) {
	recorder := installTraceProvider()

	handler := &traceCapturingHandler{}
	mux := http.NewServeMux()
	mux.Handle(tymuxv1connect.NewTymuxServiceHandler(handler))
	srv := httptest.NewUnstartedServer(mux)
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetHTTP1(true)
	srv.Config.Protocols.SetUnencryptedHTTP2(true) // tymuxd speaks plaintext h2c
	srv.Start()
	t.Cleanup(srv.Close)

	client := newTymuxClient(newH2CClient(), srv.URL)

	ctx, parent := otel.Tracer("test").Start(context.Background(), "parent")
	_, err := client.ListSessions(ctx, connect.NewRequest(&v1.ListSessionsRequest{}))
	parent.End()
	require.NoError(t, err)

	handler.mu.Lock()
	got := handler.traceparent
	handler.mu.Unlock()
	require.NotEmpty(t, got, "the daemon must receive a W3C traceparent header")
	assert.Contains(t, got, parent.SpanContext().TraceID().String(), "the header must continue the caller's trace")

	var sawClientSpan bool
	for _, s := range recorder.Ended() {
		if s.SpanContext().TraceID() == parent.SpanContext().TraceID() && s.Parent().SpanID() == parent.SpanContext().SpanID() {
			sawClientSpan = true
		}
	}
	assert.True(t, sawClientSpan, "otelconnect must record a client span under the caller's span")
}
