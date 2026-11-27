package notifyactions

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/server/middleware"
	"github.com/tstapler/stapler-squad/server/services/access"
	"github.com/tstapler/stapler-squad/server/services/approvalkit"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session/instance"
)

// TestMain gives this binary the hermetic setup server/services' own tests run under.
func TestMain(m *testing.M) {
	os.Exit(svctest.Main(m, func() {}))
}

// Fixtures duplicated from server/services' tests, which a leaf cannot import.

const ciConclusionFailure = "failure"

var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return flagsvc.FlagRegistry() })

func VerdictServiceConfig(listen string, verified []string) middleware.VerdictConfig {
	return middleware.VerdictConfig{
		VerifiedHosts:  func() []string { return verified },
		ListenAddr:     func() string { return listen },
		AllowedOrigins: func() []string { return []string{"https://ui.example"} },
		LocalIPs:       func() []net.IP { return []net.IP{net.ParseIP("192.168.1.50")} },
	}
}

// StampedContext runs the real stamp middleware and returns the context a
// handler behind it would see.
func StampedContext(t *testing.T, cfg *middleware.VerdictConfig, listener string, requiresAuth bool, host string, hdr map[string]string) context.Context {
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

type fakeApprovalLiveInstanceFinder struct{ inst *instance.Instance }

func (f *fakeApprovalLiveInstanceFinder) FindLiveInstance(id string) *instance.Instance {
	if f.inst != nil && (f.inst.UUID == id || f.inst.Title == id) {
		return f.inst
	}
	return nil
}

func newTestPendingApproval(id, sessionID, toolName string) *approvalkit.PendingApproval {
	return &approvalkit.PendingApproval{ID: id, SessionID: sessionID, ToolName: toolName}
}

// A process-lifetime reader with delta assertions: only the first
// SetMeterProvider call in a test binary rewires the global instruments.
var testMetricReader = sdkmetric.NewManualReader()

func init() {
	otel.SetMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(testMetricReader)))
}

// collectMetric returns the collected metricdata.Metrics for the given
// instrument name, or nil if it has no recorded data points yet.
func collectMetric(t *testing.T, name string) *metricdata.Metrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testMetricReader.Collect(context.Background(), &rm))
	for _, sm := range rm.ScopeMetrics {
		for i := range sm.Metrics {
			if sm.Metrics[i].Name == name {
				return &sm.Metrics[i]
			}
		}
	}
	return nil
}
