package tmuxsocketscope_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/tstapler/stapler-squad/tools/lint/internal/gotool"
	"github.com/tstapler/stapler-squad/tools/lint/tmuxsocketscope"
)

func TestAnalyzer(t *testing.T) {
	gotool.Setup(t)
	testdata := analysistest.TestData()
	analysistest.Run(t, testdata, tmuxsocketscope.Analyzer, "a")
}
