package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/tstapler/stapler-squad/config"
)

const (
	maxMemoryFileBytes = 64 * 1024
	memoryLockTimeout  = 5 * time.Second
)

// injectionMarkers are lowercase phrases rejected before a write: learnings are replayed into
// future agent prompts, so text echoing attacker-influenced item content must not smuggle in
// instructions. A coarse deny-list, not a complete defense.
var injectionMarkers = []string{
	"ignore previous instructions", "ignore all previous", "disregard previous", "disregard the above",
	"you are now", "system prompt", "<|im_start|>",
}

// fileMemoryStore appends completion-review learnings to a per-repo Markdown file under the
// config dir. Append-only by construction: it has no other method, and opens with O_APPEND.
// Stands in for the operator-memory store (not yet built); MemoryStore is the seam to swap it.
type fileMemoryStore struct{}

// NewFileMemoryStore returns the production MemoryStore.
func NewFileMemoryStore() MemoryStore { return fileMemoryStore{} }

func (fileMemoryStore) Append(ctx context.Context, entry MemoryEntry) error {
	lower := strings.ToLower(entry.Content)
	for _, m := range injectionMarkers {
		if strings.Contains(lower, m) {
			return fmt.Errorf("completion_reviewer: content rejected: matches injection marker %q", m)
		}
	}
	dir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("completion_reviewer: config dir: %w", err)
	}
	dir = filepath.Join(dir, "completion-reviews")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	path := filepath.Join(dir, repoMemoryFileName(entry.RepoPath))

	lock := flock.New(path + ".lock")
	lockCtx, cancel := context.WithTimeout(ctx, memoryLockTimeout)
	defer cancel()
	locked, err := lock.TryLockContext(lockCtx, 50*time.Millisecond)
	if err != nil || !locked {
		return fmt.Errorf("completion_reviewer: lock %s: %w", path, err)
	}
	defer func() { _ = lock.Unlock() }()

	if st, err := os.Stat(path); err == nil && st.Size() >= maxMemoryFileBytes {
		return fmt.Errorf("completion_reviewer: memory file at cap (%d bytes), append rejected", st.Size())
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(formatMemorySection(entry, time.Now())); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func repoMemoryFileName(repoPath string) string {
	sum := sha256.Sum256([]byte(repoPath))
	return filepath.Base(repoPath) + "-" + hex.EncodeToString(sum[:4]) + ".md"
}

func formatMemorySection(entry MemoryEntry, now time.Time) string {
	short := entry.SourceItemID
	if len(short) > 8 {
		short = short[:8]
	}
	return fmt.Sprintf("\n<!-- completion-reviewer source-item=%s completed-at=%s -->\n## Learnings from item %s (%s)\n\n%s\n",
		entry.SourceItemID, now.UTC().Format(time.RFC3339), short, now.Format("2006-01-02"), strings.TrimSpace(entry.Content))
}
