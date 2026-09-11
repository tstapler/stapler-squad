package git

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAdminFileWriter_WriteFile_ReplacesExistingContentAtomically covers Epic
// 1.1's first acceptance criterion (plan.md Story 1.1.1): a crash between the
// temp file's fsync and the rename leaves the target with its original
// content, never torn. Simulated by calling writeTemp directly (the pre-rename
// half of WriteFile) and never calling renameAndSyncDir, per Task 1.1.1c's
// "injected hook, not a real process kill" instruction.
func TestAdminFileWriter_WriteFile_ReplacesExistingContentAtomically(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "HEAD")
	original := "ref: refs/heads/main\n"
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatalf("failed to seed original file: %v", err)
	}

	w := NewAdminFileWriter(dir)

	// Simulate "killed after fsync, before rename": call the pre-rename half
	// directly and stop there.
	tmpPath, err := w.writeTemp("HEAD", []byte("ref: refs/heads/feature\n"))
	if err != nil {
		t.Fatalf("writeTemp returned error: %v", err)
	}
	if tmpPath == "" {
		t.Fatal("writeTemp returned empty tmpPath with nil error")
	}

	assertFileContent(t, target, original)
}

// TestAdminFileWriter_WriteFile_ReplacesExistingContentAtomically_ThenCompletes
// is the completion counterpart to the crash-simulation test above: when
// WriteFile is allowed to run to completion (not interrupted before rename),
// it does replace the existing target content.
func TestAdminFileWriter_WriteFile_ReplacesExistingContentAtomically_ThenCompletes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "HEAD")
	if err := os.WriteFile(target, []byte("ref: refs/heads/main\n"), 0600); err != nil {
		t.Fatalf("failed to seed original file: %v", err)
	}

	w := NewAdminFileWriter(dir)
	replacement := "ref: refs/heads/feature\n"
	if err := w.WriteFile("HEAD", []byte(replacement)); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	assertFileContent(t, target, replacement)
}

// assertFileContent re-reads path via a freshly opened file handle (not a
// cached one) and fails t if its content doesn't equal want.
func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %q: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("content of %q = %q, want %q", path, got, want)
	}
}

// TestAdminFileWriter_WriteFile_NewFile_Succeeds covers Epic 1.1's second
// acceptance criterion: a successful write is durable and readable via a
// freshly re-opened file handle immediately after WriteFile returns.
func TestAdminFileWriter_WriteFile_NewFile_Succeeds(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	w := NewAdminFileWriter(dir)

	if err := w.WriteFile("locked", []byte("initializing")); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	assertFileContent(t, filepath.Join(dir, "locked"), "initializing")
}

// TestAdminFileWriter_WriteFile_should_ReturnError_When_TargetDirDoesNotExist
// covers validation.md's P1 error-path row: WriteFile against a non-existent
// root dir returns an error and leaves no partial file behind anywhere.
func TestAdminFileWriter_WriteFile_should_ReturnError_When_TargetDirDoesNotExist(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	missingDir := filepath.Join(parent, "does-not-exist")
	w := NewAdminFileWriter(missingDir)

	if err := w.WriteFile("HEAD", []byte("ref: refs/heads/main\n")); err == nil {
		t.Fatal("WriteFile returned nil error for a non-existent target directory")
	}

	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("failed to read parent dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("parent dir has %d entries after failed WriteFile, want 0 (no partial file left behind): %v", len(entries), entries)
	}
}
