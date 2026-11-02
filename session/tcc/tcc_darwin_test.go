//go:build darwin

package tcc

import (
	"os"
	"path/filepath"
	"testing"
)

// Exercises the real statfs/stat wiring; classification never opens the path, so no prompt.
func TestClassifyRealDarwin(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	if got := Classify(filepath.Join(home, "Documents", "x")); got.Service != FilesAndFolders {
		t.Errorf("~/Documents: got %+v", got)
	}
	if got := Classify(t.TempDir()); got.Protected {
		t.Errorf("temp dir: got %+v", got)
	}
	if got := Classify("/"); got.Protected {
		t.Errorf("/: got %+v", got)
	}
}
