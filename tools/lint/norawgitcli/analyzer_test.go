package norawgitcli_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/norawgitcli"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// a: violations, suppressions, and _test.go exemption; the example.com packages
	// are the sanctioned paths and must produce no diagnostics.
	analysistest.Run(t, testdata, norawgitcli.Analyzer,
		"a",
		"example.com/session/git/backend/cli",
		"example.com/session/gitwiring",
	)
}
