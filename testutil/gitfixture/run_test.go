package gitfixture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunLsFilesAndRunOutput_should_TrackACommittedFile(t *testing.T) {
	dir := t.TempDir()
	Run(t, dir, "init", "-q")
	ConfigureNativeIdentity(t, dir)
	if got := LsFiles(t, dir); got != nil {
		t.Fatalf("an empty repo should track nothing, got %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	Run(t, dir, "add", "a.txt")
	Run(t, dir, "commit", "-q", "-m", "first")
	if got := LsFiles(t, dir); len(got) != 1 || got[0] != "a.txt" {
		t.Fatalf("LsFiles = %v, want [a.txt]", got)
	}
	if out := RunOutput(t, dir, "log", "--format=%s"); strings.TrimSpace(out) != "first" {
		t.Fatalf("RunOutput(log) = %q, want first", out)
	}
}
