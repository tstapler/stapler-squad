// Package hibernation provides checkpoint writing and cleanup for hibernated sessions.
package hibernation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Checkpoint holds the metadata saved when a session is hibernated.
type Checkpoint struct {
	SchemaVersion    int       `json:"schema_version"` // always 1
	SessionID        string    `json:"session_id"`
	SessionTitle     string    `json:"session_title"`
	WorkingDirectory string    `json:"working_directory"`
	Program          string    `json:"program"`
	HibernatedAt     time.Time `json:"hibernated_at"`
	HibernateReason  string    `json:"hibernate_reason"` // "manual", "idle", "resource_pressure"
	ScrollbackFile   string    `json:"scrollback_file"`  // path relative to checkpoint dir
}

// Writer writes checkpoint data to disk.
type Writer struct {
	checkpointDir string
}

// NewWriter creates a Writer that stores checkpoints under checkpointDir.
func NewWriter(checkpointDir string) *Writer {
	return &Writer{checkpointDir: checkpointDir}
}

// Write saves checkpoint metadata and copies the scrollback file.
//
// Layout:
//
//	<checkpointDir>/<sessionID>/checkpoint.json
//	<checkpointDir>/<sessionID>/scrollback.txt   (copy of srcScrollbackPath, if non-empty)
//
// If srcScrollbackPath is empty the scrollback copy step is skipped silently.
func (w *Writer) Write(ctx context.Context, c Checkpoint, srcScrollbackPath string) error {
	dir := filepath.Join(w.checkpointDir, c.SessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("hibernation: create checkpoint dir: %w", err)
	}

	// Write checkpoint.json
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("hibernation: marshal checkpoint: %w", err)
	}
	checkpointPath := filepath.Join(dir, "checkpoint.json")
	if err := os.WriteFile(checkpointPath, data, 0o644); err != nil {
		return fmt.Errorf("hibernation: write checkpoint.json: %w", err)
	}

	// Copy scrollback file (best-effort — caller decides whether to abort)
	if srcScrollbackPath != "" {
		dstPath := filepath.Join(dir, "scrollback.txt")
		if err := copyFile(srcScrollbackPath, dstPath); err != nil {
			return fmt.Errorf("hibernation: copy scrollback: %w", err)
		}
	}

	return nil
}

// Delete removes the checkpoint directory for a session.
func (w *Writer) Delete(sessionID string) error {
	dir := filepath.Join(w.checkpointDir, sessionID)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("hibernation: delete checkpoint dir: %w", err)
	}
	return nil
}

// copyFile copies src to dst, creating dst if it doesn't exist.
// The copy is self-contained — symlinks are not used so the checkpoint remains
// valid even if the original scrollback file is rotated or deleted.
func copyFile(src, dst string) error {
	srcF, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			// No scrollback yet — not an error
			return nil
		}
		return fmt.Errorf("open source: %w", err)
	}
	defer srcF.Close()

	dstF, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create dest: %w", err)
	}
	defer dstF.Close()

	if _, err := io.Copy(dstF, srcF); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	return nil
}
