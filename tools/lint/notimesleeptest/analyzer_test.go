package notimesleeptest_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/notimesleeptest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), notimesleeptest.Analyzer,
		"a", "tests/realtime")
}
