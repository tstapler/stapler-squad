package flow

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type metricRef struct {
	path  string
	key   DrillKey
	value int
}

// walkMetrics visits every CountMetric, RatioMetric and DurationMetric in v.
func walkMetrics(v reflect.Value, path string, out *[]metricRef) {
	switch m := v.Interface().(type) {
	case CountMetric:
		*out = append(*out, metricRef{path, m.Drill, m.Value})
		return
	case RatioMetric:
		*out = append(*out, metricRef{path, m.Drill, m.Num})
		return
	case DurationMetric:
		*out = append(*out, metricRef{path, m.Drill, m.Stats.N})
		return
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				walkMetrics(v.Field(i), path+"."+v.Type().Field(i).Name, out)
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			walkMetrics(v.Index(i), path+"[]", out)
		}
	}
}

func metricsOf(s *FlowSummary) []metricRef {
	var refs []metricRef
	walkMetrics(reflect.ValueOf(*s), "FlowSummary", &refs)
	return refs
}

func TestEveryMetricHasDrillKey_should_FailAndReturnItems_When_MetricLacksKey(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	agg := NewAggregator(f.sources(), fixtureClock)
	q := FlowQuery{Window: WindowSpec{Days: 30}}
	s, err := agg.Summary(context.Background(), q)
	require.NoError(t, err)

	refs := metricsOf(s)
	require.Greater(t, len(refs), 25, "the walk found the metrics")
	for _, m := range refs {
		require.NotEmpty(t, m.key, "metric %s has no drill key", m.path)
		page, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: m.key, Query: q})
		require.NoError(t, err, "%s -> %s", m.path, m.key)
		if m.value > 0 {
			assert.Positive(t, page.Items.TotalCount, "%s -> %s has a count but no items", m.path, m.key)
		}
	}

	broken := *s
	broken.Tiles.Merged.Drill = ""
	var missing []string
	for _, m := range metricsOf(&broken) {
		if m.key == "" {
			missing = append(missing, m.path)
		}
	}
	assert.Equal(t, []string{"FlowSummary.Tiles.Merged"}, missing, "the check names a metric that lacks its key")
}

func TestDrilldown_should_ReturnTitleStatusReasonAndDuration_When_LeadTimeKey(t *testing.T) {
	t.Parallel()
	agg := NewAggregator(fixtureSrc(t).sources(), fixtureClock)
	page, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: DrillLeadTime, Query: FlowQuery{Window: WindowSpec{Days: 30}}})
	require.NoError(t, err)

	assert.Equal(t, 2, page.Items.TotalCount, "tz-a and bounce-a")
	for _, it := range page.Items.Items {
		assert.NotEmpty(t, it.Title)
		assert.NotEmpty(t, it.Status)
		assert.NotEmpty(t, it.Reason)
		assert.Positive(t, it.Duration)
	}
}

func TestDrilldown_should_PageAndListValidKeys_When_OffsetOrUnknownKey(t *testing.T) {
	t.Parallel()
	f := newFakeSrc()
	f.addMerged("m", 25, cmpAMid)
	agg := NewAggregator(f.sources(), fixtureClock)
	q := FlowQuery{Window: WindowSpec{Days: 30}}

	p, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: DrillMerged, Query: q, Limit: 10, Offset: 20})
	require.NoError(t, err)
	assert.Equal(t, 25, p.Items.TotalCount)
	assert.Equal(t, 5, p.Items.ReturnedCount)
	assert.False(t, p.Items.HasMore)

	_, err = agg.Drilldown(context.Background(), DrilldownQuery{Key: "nope", Query: q})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown drill key "nope"; valid keys: `)
	assert.Contains(t, err.Error(), string(DrillMerged))

	empty, err := agg.Drilldown(context.Background(), DrilldownQuery{Key: DrillThrashEpisode, Query: q})
	require.NoError(t, err, "a registered key with no items is an empty page, not an unknown key")
	assert.Zero(t, empty.Items.TotalCount)
}

func TestRun_should_ReturnError_When_DurationSourceFails(t *testing.T) {
	t.Parallel()
	f := fixtureSrc(t)
	f.eventsErr = errors.New("db down")
	agg := NewAggregator(f.sources(), fixtureClock)
	w, err := WindowSpec{Days: 30}.Window(fixtureClock)
	require.NoError(t, err)

	for _, key := range []DrillKey{DrillLeadTime, DrillPRFixDwell} {
		r := newRun(context.Background(), agg, FlowQuery{Window: WindowSpec{Days: 30}}, w)
		_, err := r.drillDurations(key)
		assert.ErrorContains(t, err, "db down", key)
	}
	f.stuckErr = errors.New("stuck down")
	r := newRun(context.Background(), agg, FlowQuery{Window: WindowSpec{Days: 30}}, w)
	_, err = r.drillDurations(DrillKey("stuck.no_progress"))
	assert.ErrorContains(t, err, "stuck down")
}
