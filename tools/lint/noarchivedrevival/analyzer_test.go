package noarchivedrevival_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/internal/gotool"
	"github.com/tstapler/stapler-squad/tools/lint/noarchivedrevival"
)

func TestAnalyzer(t *testing.T) {
	gotool.Setup(t)
	analysistest.Run(t, analysistest.TestData(), noarchivedrevival.Analyzer, "session/autonomous", "session/driver", "session/instance", "other", "server/wakes", "server/services", "server/services/federation")
}
