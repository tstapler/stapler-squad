package flow

import (
	"math"
	"slices"
	"time"
)

// Percentiles is the nearest-rank summary of a duration sample: the p-th
// percentile is the value at 1-based rank ceil(p/100 x N) of the sorted sample.
type Percentiles struct {
	N         int
	P50       time.Duration
	P90       time.Duration
	Max       time.Duration
	LowSample bool
}

const (
	percentileMedian = 50
	percentileP90    = 90
)

// NewPercentiles summarizes samples without mutating them. LowSample is true
// when N < lowSampleN (and always for an empty sample).
func NewPercentiles(samples []time.Duration, lowSampleN int) Percentiles {
	n := len(samples)
	if n == 0 {
		return Percentiles{LowSample: true}
	}
	sorted := slices.Clone(samples)
	slices.Sort(sorted)
	return Percentiles{
		N:         n,
		P50:       nearestRank(sorted, percentileMedian),
		P90:       nearestRank(sorted, percentileP90),
		Max:       sorted[n-1],
		LowSample: n < lowSampleN,
	}
}

// nearestRank assumes sorted is ascending and non-empty.
func nearestRank(sorted []time.Duration, pct int) time.Duration {
	rank := int(math.Ceil(float64(pct) / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}
