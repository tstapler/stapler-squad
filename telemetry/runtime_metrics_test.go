package telemetry

import (
	"math"
	"runtime/metrics"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistDelta_should_ReportOnlyObservationsSincePreviousScrape(t *testing.T) {
	t.Parallel()
	buckets := []float64{0, 0.001, 0.01, 0.1, math.Inf(1)}
	var h histDelta

	first := &metrics.Float64Histogram{Buckets: buckets, Counts: []uint64{90, 10, 0, 0}}
	v, ok := h.quantilesSince(first, 0.5, 0.99)
	require.True(t, ok)
	assert.Equal(t, []float64{0.001, 0.01}, v)

	// 100 new observations all landed in the 0.01-0.1 bucket; old history must not dilute them.
	second := &metrics.Float64Histogram{Buckets: buckets, Counts: []uint64{90, 10, 0, 100}}
	v, ok = h.quantilesSince(second, 0.5, 0.99)
	require.True(t, ok)
	assert.Equal(t, []float64{0.1, 0.1}, v)

	_, ok = h.quantilesSince(second, 0.99)
	assert.False(t, ok, "no new observations")
}

func TestBucketQuantile_should_ReportFiniteLowerBound_When_TailBucketIsInfinite(t *testing.T) {
	t.Parallel()
	buckets := []float64{0, 1, math.Inf(1)}
	assert.Equal(t, 1.0, bucketQuantile(buckets, []uint64{0, 5}, 5, 0.99))
}
