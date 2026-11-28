package tccrpc

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
)

// testService drives State the way SessionService does, with no live sessions.
type testService struct{ state State }

func (s *testService) SetTCCEnrichment(gate *tcc.ConsentGate, enricher *instance.WorktreeEnricher, enabled bool) {
	s.state = NewState(gate, enricher, enabled)
}

func (s *testService) GetTCCEnrichmentStatus(ctx context.Context, req *connect.Request[sessionv1.GetTCCEnrichmentStatusRequest]) (*connect.Response[sessionv1.GetTCCEnrichmentStatusResponse], error) {
	return s.state.GetTCCEnrichmentStatus(ctx, req, nil)
}

func (s *testService) ContinueTCCEnrichment(ctx context.Context, req *connect.Request[sessionv1.ContinueTCCEnrichmentRequest]) (*connect.Response[sessionv1.ContinueTCCEnrichmentResponse], error) {
	return s.state.ContinueTCCEnrichment(ctx, req)
}

func verdictServiceConfig(listen string, verified []string) middleware.VerdictConfig {
	return middleware.VerdictConfig{
		VerifiedHosts:  func() []string { return verified },
		ListenAddr:     func() string { return listen },
		AllowedOrigins: func() []string { return []string{"https://ui.example"} },
		LocalIPs:       func() []net.IP { return []net.IP{net.ParseIP("192.168.1.50")} },
	}
}

// stampedContext runs the real stamp middleware and returns the context a
// handler behind it would see.
func stampedContext(t *testing.T, cfg *middleware.VerdictConfig, listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
	t.Helper()
	var got context.Context
	var h http.Handler = http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Context()
	})
	if cfg != nil {
		h = access.WithLocalWriteVerdictConfig(*cfg)(h)
	}
	h = access.WithRequestRecord(listener, requiresAuth)(h)
	r := httptest.NewRequest(http.MethodPost, "/x", nil)
	r.Host = host
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	h.ServeHTTP(httptest.NewRecorder(), r)
	require.NotNil(t, got)
	return got
}
