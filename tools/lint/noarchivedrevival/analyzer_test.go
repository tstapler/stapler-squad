package noarchivedrevival_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/noarchivedrevival"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), noarchivedrevival.Analyzer, "session", "other")
}
