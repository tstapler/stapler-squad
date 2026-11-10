package report_test

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/changed"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/report"
)

func TestReportPrintsPerFeatureMissingFieldsAndCount(t *testing.T) {
	t.Parallel()
	res := check.Result{
		Counts: check.Counts{InScope: 5, Covered: 1, Exempt: 1, Grandfathered: 2, Gap: 1},
		Entries: []check.Entry{
			{ID: "foo-panel", Status: model.StatusGap, Missing: []string{"help", "website"}, Change: changed.New},
			{ID: "old-panel", Status: model.StatusGrandfathered, Change: changed.Modified, Notes: []string{"changed, still grandfathered"}},
		},
		Violations: []string{"foo-panel: missing help, website"},
	}
	var buf bytes.Buffer
	report.Write(&buf, res, report.Options{Notices: []string{"base unresolved"}})
	out := buf.String()
	assert.Contains(t, out, "In scope: 5 | covered: 1 | exempt: 1 | grandfathered: 2 | gap: 1")
	assert.Contains(t, out, "- foo-panel: missing help, website")
	assert.Contains(t, out, "### Changed, still grandfathered (1)")
	assert.Contains(t, out, "NOTICE: base unresolved")
	assert.Contains(t, out, "report-only")
	assert.NotContains(t, out, "%")
}
