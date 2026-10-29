package tymux

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// highCardinalityKeys must never be a metric label: each distinct value is a
// new time series, and sessions are created without bound.
var highCardinalityKeys = map[string]bool{
	"session_id": true, "session.id": true, "session": true, "session_name": true,
	"instance": true, "instance_id": true, "instance.id": true,
	"pane_id": true, "title": true, "path": true,
}

// TestMetricLabelKeys_NeverIdentifyASession is the static half of the guard:
// no attribute.String/Int/Bool/... in the tymux package or the session
// backend observability files may use a per-session key, wherever it is
// recorded.
func TestMetricLabelKeys_NeverIdentifyASession(t *testing.T) {
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	backendFiles, err := filepath.Glob("../backend_*.go")
	require.NoError(t, err)
	files = append(files, backendFiles...)

	fset := token.NewFileSet()
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		require.NoError(t, err)
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "attribute" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			key, err := strconv.Unquote(lit.Value)
			require.NoError(t, err)
			checked++
			assert.False(t, highCardinalityKeys[key], "%s: attribute key %q identifies a session; use a log or span attribute instead", fset.Position(call.Pos()), key)
			return true
		})
	}
	assert.Positive(t, checked, "guard scanned no attribute keys; is the file glob stale?")
}

// TestTymuxInstruments_NamesUnitsAndLabelKeys is the runtime half: after
// driving every recorder once, each tymux_* instrument is exported with the
// expected unit and only its documented label keys.
func TestTymuxInstruments_NamesUnitsAndLabelKeys(t *testing.T) {
	recordReconnect("error")
	recordAttachEvent(attachEventOutputGap)
	recordDaemonRestart(daemonRestartColdStart)
	recordDaemonStartDuration(0, daemonStartOK)
	recordVersionMismatch(VersionSame)

	want := map[string]struct {
		unit string
		keys []string
	}{
		"tymux_attach_stream_reconnects_total": {"", []string{"cause"}},
		"tymux_attach_events_total":            {"", []string{"kind"}},
		"tymux_daemon_restarts_total":          {"", []string{"reason"}},
		"tymux_daemon_start_duration_ms":       {"ms", []string{"outcome"}},
		"tymux_daemon_version_mismatch":        {"", nil},
		"tymux_attach_heartbeat_age_seconds":   {"s", nil},
	}

	var rm metricdata.ResourceMetrics
	require.NoError(t, testTymuxMetricReader.Collect(context.Background(), &rm))
	seen := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			exp, ok := want[m.Name]
			if !ok {
				continue
			}
			seen[m.Name] = true
			assert.Equal(t, exp.unit, m.Unit, m.Name)
			for _, set := range dataPointAttributes(m) {
				keys := make([]string, 0, set.Len())
				for _, kv := range set.ToSlice() {
					keys = append(keys, string(kv.Key))
				}
				assert.ElementsMatch(t, exp.keys, keys, "%s label keys", m.Name)
			}
		}
	}
	for name := range want {
		assert.True(t, seen[name], "%s was not exported", name)
	}
}

func dataPointAttributes(m metricdata.Metrics) []attribute.Set {
	var sets []attribute.Set
	switch d := m.Data.(type) {
	case metricdata.Sum[int64]:
		for _, dp := range d.DataPoints {
			sets = append(sets, dp.Attributes)
		}
	case metricdata.Gauge[int64]:
		for _, dp := range d.DataPoints {
			sets = append(sets, dp.Attributes)
		}
	case metricdata.Gauge[float64]:
		for _, dp := range d.DataPoints {
			sets = append(sets, dp.Attributes)
		}
	case metricdata.Histogram[float64]:
		for _, dp := range d.DataPoints {
			sets = append(sets, dp.Attributes)
		}
	}
	return sets
}
