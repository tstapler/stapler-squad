package fixtures

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
)

func TestInitRepoCreatesOneCommit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := InitRepo(dir, FilesViewerRepo(), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	repo, err := git.PlainOpen(dir) //nolint:norawgitopen test inspects the fixture it just created
	if err != nil {
		t.Fatal(err)
	}
	head, err := repo.Head()
	if err != nil {
		t.Fatal(err)
	}
	c, err := repo.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	if c.Author.Email != "" {
		t.Errorf("author email %q must be empty", c.Author.Email)
	}
}

func TestWriteTerminalProgramIsExecutableAndPrintsTranscript(t *testing.T) {
	t.Parallel()
	script, err := WriteTerminalProgram(t.TempDir(), "demo", []byte("hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(script)
	if err != nil || info.Mode()&0o100 == 0 {
		t.Fatalf("script not executable: %v %v", info, err)
	}
	body, _ := os.ReadFile(script)
	txt := strings.TrimSuffix(script, ".sh") + ".txt"
	if !strings.Contains(string(body), filepath.Base(txt)) {
		t.Errorf("script does not reference transcript: %s", body)
	}
}
