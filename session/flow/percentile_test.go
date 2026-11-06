package flow

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func hours(hs ...int) []time.Duration {
	out := make([]time.Duration, len(hs))
	for i, h := range hs {
		out[i] = time.Duration(h) * time.Hour
	}
	return out
}

func TestPercentiles_should_ReturnNearestRankAndLowSample_When_FiveAndThreeSamples(t *testing.T) {
	t.Parallel()

	five := NewPercentiles(hours(1, 2, 3, 4, 100), DefaultConfig().LowSampleN)
	assert.Equal(t, 5, five.N)
	assert.Equal(t, 3*time.Hour, five.P50)
	assert.Equal(t, 100*time.Hour, five.P90)
	assert.Equal(t, 100*time.Hour, five.Max)
	assert.False(t, five.LowSample)

	three := NewPercentiles(hours(1, 2, 3), DefaultConfig().LowSampleN)
	assert.Equal(t, 3, three.N)
	assert.True(t, three.LowSample)
}

func TestPercentiles_should_SortCopyAndNotMutateInput_When_Unsorted(t *testing.T) {
	t.Parallel()
	in := hours(100, 1, 4, 2, 3)
	p := NewPercentiles(in, 5)
	assert.Equal(t, 3*time.Hour, p.P50)
	assert.Equal(t, hours(100, 1, 4, 2, 3), in)
}

func TestPercentiles_should_BeZeroAndLowSample_When_NoSamples(t *testing.T) {
	t.Parallel()
	p := NewPercentiles(nil, 5)
	assert.Equal(t, Percentiles{LowSample: true}, p)
}

func TestPercentiles_should_UseNearestRankAtBoundaries_When_SmallN(t *testing.T) {
	t.Parallel()
	one := NewPercentiles(hours(7), 5)
	assert.Equal(t, 7*time.Hour, one.P50)
	assert.Equal(t, 7*time.Hour, one.P90)

	ten := NewPercentiles(hours(1, 2, 3, 4, 5, 6, 7, 8, 9, 10), 5)
	assert.Equal(t, 5*time.Hour, ten.P50, "rank ceil(0.5*10)=5")
	assert.Equal(t, 9*time.Hour, ten.P90, "rank ceil(0.9*10)=9")
}
