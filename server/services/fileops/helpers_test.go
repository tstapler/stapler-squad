package fileops

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/server/services/flagsvc"
)

// testFlagRegistry is the one registry tests share; it is immutable after construction.
var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return flagsvc.FlagRegistry() })

// flippedDefaults is testFlagRegistry with the named flags' defaults inverted.
func flippedDefaults(names ...string) *featureflags.Registry {
	flip := map[string]bool{}
	for _, n := range names {
		flip[n] = true
	}
	specs := testFlagRegistry().Specs()
	for i := range specs {
		if flip[specs[i].Name] {
			specs[i].Default = !specs[i].Default
		}
	}
	return featureflags.NewRegistry(specs)
}

// attributeKeyOutcome is the attribute key the files instruments tag their data points with.
const attributeKeyOutcome = attribute.Key("outcome")

// testMetricReader backs the one real MeterProvider of this test binary; OTel rewires the
// package-level instruments only on the first SetMeterProvider, so assertions use deltas.
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

// sumForOutcome sums the Int64 counter's data points whose "outcome"
// attribute equals outcome.
func sumForOutcome(t *testing.T, m *metricdata.Metrics, outcome string) int64 {
	t.Helper()
	if m == nil {
		return 0
	}
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		if v, ok := dp.Attributes.Value(attributeKeyOutcome); ok && v.AsString() == outcome {
			total += dp.Value
		}
	}
	return total
}

// histogramCountForOutcome sums the Float64 histogram's per-datapoint counts
// whose "outcome" attribute equals outcome.
func histogramCountForOutcome(t *testing.T, m *metricdata.Metrics, outcome string) uint64 {
	t.Helper()
	if m == nil {
		return 0
	}
	hist, ok := m.Data.(metricdata.Histogram[float64])
	require.True(t, ok, "unexpected data type %T for %s", m.Data, m.Name)
	var total uint64
	for _, dp := range hist.DataPoints {
		if v, ok := dp.Attributes.Value(attributeKeyOutcome); ok && v.AsString() == outcome {
			total += dp.Count
		}
	}
	return total
}

// Sample files for the range benchmarks.
const (
	mib           = 1 << 20
	ioChunk       = 1 << 20
	longLineChars = 200
)

type lineStyle struct {
	name string
	eol  string
	line int // chars per line before the terminator
}

var lineStyles = []lineStyle{{"LF", "\n", 60}, {"CRLF", "\r\n", 60}, {"long200", "\n", longLineChars}}

// transportFile returns (creating once per dir) the deterministic sample file for
// style and size; the caller owns dir (b.TempDir of the top-level benchmark).
func transportFile(b *testing.B, dir string, style lineStyle, sizeMB int) string {
	b.Helper()
	p := filepath.Join(dir, fmt.Sprintf("%s-%d.txt", style.name, sizeMB))
	if _, err := os.Stat(p); err == nil {
		return p
	}
	f, err := os.Create(p)
	if err != nil {
		b.Fatal(err)
	}
	w := bufio.NewWriterSize(f, ioChunk)
	rng := rand.New(rand.NewSource(fixtureSeed))
	line := make([]byte, style.line)
	for written := 0; written < sizeMB*mib; {
		for i := range line {
			line[i] = byte('a' + rng.Intn(26))
		}
		if rng.Intn(5000) == 0 {
			copy(line, "TODO")
		}
		w.Write(line)
		w.WriteString(style.eol)
		written += style.line + len(style.eol)
	}
	if err := w.Flush(); err != nil {
		b.Fatal(err)
	}
	if err := f.Close(); err != nil {
		b.Fatal(err)
	}
	return p
}
