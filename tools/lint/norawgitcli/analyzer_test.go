package norawgitcli_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/norawgitcli"
)

func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	// a: violations, suppressions, stale directives, and the _test.go exemption;
	// session/git carries the runGitCommand wrapper; clix is a near-miss that must
	// be flagged; the other packages are sanctioned and must produce no diagnostics.
	analysistest.Run(t, testdata, norawgitcli.Analyzer,
		"a",
		"github.com/tstapler/stapler-squad/session/git",
		"github.com/tstapler/stapler-squad/session/git/backend/clix",
		"github.com/tstapler/stapler-squad/session/git/backend/cli",
		"github.com/tstapler/stapler-squad/session/git/backend/cli/sub",
		"github.com/tstapler/stapler-squad/session/gitwiring",
		"github.com/tstapler/stapler-squad/testutil/gitfixture",
		"github.com/tstapler/stapler-squad/session/git/internal/gittest",
	)
}
