package notimesleeptest_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/internal/gotool"
	"github.com/tstapler/stapler-squad/tools/lint/notimesleeptest"
)

func TestAnalyzer(t *testing.T) {
	gotool.Setup(t)
	analysistest.Run(t, analysistest.TestData(), notimesleeptest.Analyzer,
		"a", "tests/realtime", "example.com/tests/realtimefoo", "example.com/tests/realtime/sub")
}
