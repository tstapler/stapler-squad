package norawdb_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/norawdb"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), norawdb.Analyzer, "a", "example.com/m/session/dbpool")
}
