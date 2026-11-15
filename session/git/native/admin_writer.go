package native

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		return fmt.Errorf("admin file writer: failed to rename temp file onto %q: %w%s", target, err, w.renameFailureState(tmpPath))
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

// renameFailureState reports which side of a failed rename is missing and, when the admin dir itself is gone, how far up
// the deletion went. A "no such file" rename after a successful CreateTemp has been seen only on CI (grep CI logs for
// "admin-file-rename-failed"); the first existing ancestor, its mtime and its entries name what removed the tree.
func (w *AdminFileWriter) renameFailureState(tmpPath string) string {
	_, dirErr := os.Stat(w.dir)
	_, tmpErr := os.Lstat(tmpPath)
	return fmt.Sprintf(" (admin-file-rename-failed: now=%s dir stat err=%v, temp lstat err=%v, dir entries=%v, ancestors=%s)",
		time.Now().Format(time.RFC3339Nano), dirErr, tmpErr, dirEntryNames(w.dir), ancestorState(w.dir))
}

func dirEntryNames(dir string) []string {
	entries, _ := os.ReadDir(dir)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// ancestorState walks up from dir (.git/worktrees, .git, repo root, ...) reporting each level until the first that
// exists, whose mtime and entries (capped) are included.
func ancestorState(dir string) string {
	const maxEntries = 20
	var parts []string
	for cur := filepath.Dir(dir); ; cur = filepath.Dir(cur) {
		info, err := os.Stat(cur)
		if err == nil {
			names := dirEntryNames(cur)
			if len(names) > maxEntries {
				names = append(names[:maxEntries], "...")
			}
			parts = append(parts, fmt.Sprintf("%s=exists(mtime=%s, entries=%v)", cur, info.ModTime().Format(time.RFC3339Nano), names))
			break
		}
		parts = append(parts, fmt.Sprintf("%s=missing(%v)", cur, err))
		if filepath.Dir(cur) == cur {
			break
		}
	}
	return strings.Join(parts, "; ")
}
