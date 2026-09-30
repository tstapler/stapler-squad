package git

import (
	"fmt"
	"os"
	"path/filepath"
)

// AdminFileWriter is the atomic write-temp-then-rename-then-fsync primitive
// (ADR-001, project_plans/go-git-worktree-and-merge/decisions/ADR-001-atomic-admin-file-write-protocol.md)
// used for every file written under a WorktreeAdminDir (.git/worktrees/<name>/)
// and for conflicted-index writes on the merge path. go-git's own writers
// (dotgit.IndexWriter, setRefRwfs) are neither atomic nor real-git-lock-compatible
// (ADR-001's Context), so this project owns the write protocol instead of
// reusing go-git's.
type AdminFileWriter struct {
	dir string
}

// NewAdminFileWriter returns an AdminFileWriter rooted at dir. dir must already
// exist — WriteFile does not create it.
func NewAdminFileWriter(dir string) *AdminFileWriter {
	return &AdminFileWriter{dir: dir}
}

// WriteFile atomically and durably writes content to name within w.dir: either
// the target ends up containing exactly content, or a crash at any point leaves
// it completely untouched (old content or absence), never torn. The extra
// directory fsync after rename is required per ADR-001 step 4: renaming a file
// durably requires fsyncing its parent directory, not just the file.
func (w *AdminFileWriter) WriteFile(name string, content []byte) error {
	tmpPath, err := w.writeTemp(name, content)
	if err != nil {
		return err
	}
	return w.renameAndSyncDir(tmpPath, name)
}

// writeTemp performs ADR-001 steps 1-2 (write to a same-directory temp file,
// fsync it) and returns the temp file's path without renaming it onto the
// target. Split out from WriteFile so tests can simulate a crash between the
// temp file's fsync and the rename (ADR-001 step 3) without a real process
// kill: call writeTemp, assert the target is untouched, and never call
// renameAndSyncDir.
func (w *AdminFileWriter) writeTemp(name string, content []byte) (string, error) {
	f, err := os.CreateTemp(w.dir, name+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("admin file writer: failed to create temp file for %q in %q: %w", name, w.dir, err)
	}
	tmpPath := f.Name()

	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("admin file writer: failed to write temp file for %q: %w", name, err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("admin file writer: failed to fsync temp file for %q: %w", name, err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("admin file writer: failed to close temp file for %q: %w", name, err)
	}

	return tmpPath, nil
}

// renameAndSyncDir performs ADR-001 steps 3-4: atomically rename tmpPath onto
// name within w.dir, then fsync w.dir itself so the rename's directory-entry
// update survives a crash (a rename can otherwise be lost on power loss even
// though the file's own fsync succeeded).
func (w *AdminFileWriter) renameAndSyncDir(tmpPath, name string) error {
	target := filepath.Join(w.dir, name)
	if err := os.Rename(tmpPath, target); err != nil {
		return fmt.Errorf("admin file writer: failed to rename temp file onto %q: %w", target, err)
	}

	dirFile, err := os.Open(w.dir)
	if err != nil {
		return fmt.Errorf("admin file writer: failed to open %q for directory fsync after writing %q: %w", w.dir, name, err)
	}
	defer func() { _ = dirFile.Close() }()

	if err := dirFile.Sync(); err != nil {
		return fmt.Errorf("admin file writer: failed to fsync directory %q after writing %q: %w", w.dir, name, err)
	}

	return nil
}
