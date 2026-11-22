package nosessiondrivertest_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/internal/gotool"
	"github.com/tstapler/stapler-squad/tools/lint/nosessiondrivertest"
)

func TestAnalyzer(t *testing.T) {
	gotool.Setup(t)
	t.Cleanup(nosessiondrivertest.SetDriverPkgPath("example.com/driver"))
	analysistest.Run(t, analysistest.TestData(), nosessiondrivertest.Analyzer,
		"user", "dot", "example.com/driver", "other/driver", "lib", "nonsession")
}
