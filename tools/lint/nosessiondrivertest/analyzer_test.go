package nosessiondrivertest_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/nosessiondrivertest"
)

func TestAnalyzer(t *testing.T) {
	t.Cleanup(nosessiondrivertest.SetSessionPkgPath("example.com/session"))
	analysistest.Run(t, analysistest.TestData(), nosessiondrivertest.Analyzer,
		"user", "dot", "example.com/session", "other/session", "lib", "nonsession")
}
