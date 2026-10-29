package session

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/config"
)

// collectBackendInfo returns the "backend/source" pairs session_backend_info
// currently reports, each asserted to carry exactly the value 1 and exactly
// the two low-cardinality label keys.
func collectBackendInfo(t *testing.T) []string {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, testPiMetricReader.Collect(context.Background(), &rm))
	var pairs []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "session_backend_info" {
				continue
			}
			gauge, ok := m.Data.(metricdata.Gauge[int64])
			require.True(t, ok, "unexpected data type %T", m.Data)
			for _, dp := range gauge.DataPoints {
				assert.EqualValues(t, 1, dp.Value)
				assert.Equal(t, 2, dp.Attributes.Len(), "only backend+source labels allowed: %v", dp.Attributes.ToSlice())
				b, _ := dp.Attributes.Value(attribute.Key("backend"))
				s, _ := dp.Attributes.Value(attribute.Key("source"))
				pairs = append(pairs, b.AsString()+"/"+s.AsString())
			}
		}
	}
	sort.Strings(pairs)
	return pairs
}

func TestRecordBackendInfo_GlobalDefaultIsTmux(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProcessManagerBackend = ""
	cfg.TymuxSessionOverrides = nil
	RecordBackendInfo(cfg)
	assert.Equal(t, []string{"tmux/global"}, collectBackendInfo(t))
}

func TestRecordBackendInfo_ReportsGlobalAndSessionOverrideSources(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProcessManagerBackend = string(BackendTymux)
	cfg.TymuxSessionOverrides = map[string]bool{"ssq_a": false, "ssq_b": false}
	RecordBackendInfo(cfg)
	assert.Equal(t, []string{"tmux/session_override", "tymux/global"}, collectBackendInfo(t),
		"series carry no session names, however many sessions are overridden")
}

func TestResolveSessionBackend_RefreshesBackendInfo(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.ProcessManagerBackend = ""
	cfg.TymuxSessionOverrides = map[string]bool{"ssq_x": true}
	ResolveSessionBackend(cfg, "ssq_x", "")
	assert.Equal(t, []string{"tmux/global", "tymux/session_override"}, collectBackendInfo(t))
}
