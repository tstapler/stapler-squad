package requirearchivedcheck_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/requirearchivedcheck"
)

// TestAnalyzer covers all three of Story 4.2.1's AC cases in one run:
//   - session/diagnose/fixture.go: matched-path bad/good/nolint cases
//     (ArchiveSessionByUUID, KillTmuxPaneOnly, ResumeSession).
//   - server/services/diagnose_fixture.go: matched-path bad case via the
//     diagnose_ filename-prefix half of the scope.
//   - server/services/superseded_session_sweeper.go: an identical unguarded
//     call in a same-package, non-diagnose_ file — must report nothing,
//     mirroring the real repo's out-of-scope pre-existing bypass.
func TestAnalyzer(t *testing.T) {
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, requirearchivedcheck.Analyzer,
		"session/diagnose",
		"server/services",
	)
}
