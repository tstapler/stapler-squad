package entfullscan_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/entfullscan"
	"github.com/tstapler/stapler-squad/tools/lint/internal/gotool"
)

func TestAnalyzer(t *testing.T) {
	gotool.Setup(t)
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, entfullscan.Analyzer, "a")
}
