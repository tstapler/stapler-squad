package scenario

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/tests/demo/scenario/fixtures"
)

//go:embed testdata/*.txt
var transcripts embed.FS

func writeTranscript(dir string, t Terminal) (string, error) {
	data, err := transcripts.ReadFile("testdata/" + t.Transcript)
	if err != nil {
		return "", fmt.Errorf("terminal %s: %w", t.ID, err)
	}
	return fixtures.WriteTerminalProgram(dir, t.ID, data)
}

func ensureFixtureRepo(dir string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	return fixtures.InitRepo(dir, fixtures.FilesViewerRepo(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
}
