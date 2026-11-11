package report_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/report"
)

func series(exempts ...int) []report.Trend {
	out := make([]report.Trend, len(exempts))
	for i, e := range exempts {
		out[i] = report.NewTrend(
			"2026-11-"+string(rune('0'+(i+1)/10))+string(rune('0'+(i+1)%10)),
			check.Counts{InScope: 200, Exempt: e}, nil)
	}
	return out
}

func TestAnalyze_GrowingExemptSeriesTriggersComment(t *testing.T) {
	t.Parallel()
	reasons := report.Analyze(series(1, 2, 3, 3, 4, 5), 0.1)
	require.Len(t, reasons, 1)
	assert.Contains(t, reasons[0], "exempt grew in 3 of the last 4 weeks")
}

func TestAnalyze_FlatSeriesDoesNot(t *testing.T) {
	t.Parallel()
	assert.Empty(t, report.Analyze(series(3, 3, 3, 3, 3, 3, 3, 3), 0.1))
	assert.Empty(t, report.Analyze(series(5, 4, 4, 3, 2), 0.1), "shrinking is fine")
	assert.Empty(t, report.Analyze(nil, 0.1))
}

func TestAnalyze_TwoOfFourDoesNotTrigger(t *testing.T) {
	t.Parallel()
	assert.Empty(t, report.Analyze(series(1, 2, 2, 3, 3), 0.1))
}

func TestAnalyze_RatioPastEightyPercentOfCeilingTriggers(t *testing.T) {
	t.Parallel()
	// 17/200 = 8.5% > 80% of the 10% ceiling, and the series is flat.
	reasons := report.Analyze(series(17, 17, 17), 0.1)
	require.Len(t, reasons, 1)
	assert.Contains(t, reasons[0], "ceiling")
	assert.Empty(t, report.Analyze(series(15, 15, 15), 0.1), "7.5% is under 8%")
}

func TestAnalyze_OrdersByDateNotInputOrder(t *testing.T) {
	t.Parallel()
	s := series(1, 2, 3, 4, 5)
	s[0], s[4] = s[4], s[0]
	assert.NotEmpty(t, report.Analyze(s, 0.1))
}

func TestNewTrend_DeltasAgainstBase(t *testing.T) {
	t.Parallel()
	base := check.Counts{InScope: 10, Exempt: 1, Grandfathered: 5}
	tr := report.NewTrend("2026-11-10", check.Counts{InScope: 10, Covered: 2, Exempt: 3, Grandfathered: 4, Gap: 1}, &base)
	assert.Equal(t, 2, tr.ExemptDelta)
	assert.Equal(t, -1, tr.GrandfatheredDelta)
	assert.InDelta(t, 0.3, tr.ExemptRatio, 1e-9)
	assert.Equal(t, "Trend: covered 2 | exempt 3 (+2) | grandfathered 4 (-1) | gap 1", tr.Line())

	noBase := report.NewTrend("2026-11-10", check.Counts{InScope: 10, Exempt: 3}, nil)
	assert.Zero(t, noBase.ExemptDelta)
}

func TestTrend_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	require.NoError(t, report.NewTrend("2026-11-10", check.Counts{InScope: 4, Exempt: 1}, nil).WriteJSON(&buf))
	var back report.Trend
	require.NoError(t, json.Unmarshal(buf.Bytes(), &back))
	assert.Equal(t, 1, back.Exempt)
	assert.Contains(t, buf.String(), `"exemptDelta"`)
}
