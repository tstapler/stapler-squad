package nowrongdbentity_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/nowrongdbentity"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), nowrongdbentity.Analyzer,
		"example.com/m/session", "example.com/m/server/analytics", "example.com/m/other")
}
