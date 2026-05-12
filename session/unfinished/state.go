package unfinished

import "github.com/linkdata/deadlock"

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tstapler/stapler-squad/executor"
	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/log"
)

// dismissEntry is a persisted dismiss record.
type dismissEntry struct {
	RepoPath    string    `json:"repo_path"`
	Branch      string    `json:"branch"`
	DismissedAt time.Time `json:"dismissed_at"`
}

// snoozeEntry is a persisted snooze record.
type snoozeEntry struct {
	RepoPath       string    `json:"repo_path"`
	Branch         string    `json:"branch"`
	SnoozeSinceSHA string    `json:"snooze_since_sha"`
	SnoozedAt      time.Time `json:"snoozed_at"`
}

// aiCacheEntry is a cached AI summary keyed by diff hash.
type aiCacheEntry struct {
	RepoPath    string    `json:"repo_path"`
	Branch      string    `json:"branch"`
	DiffHash    string    `json:"diff_hash"`
	Summary     string    `json:"summary"`
	GeneratedAt time.Time `json:"generated_at"`
}

// unfinishedState is the on-disk JSON shape.
type unfinishedState struct {
	Dismissed   []dismissEntry `json:"dismissed"`
	Snoozed     []snoozeEntry  `json:"snoozed"`
	WatchDirs   []string       `json:"watch_dirs"`
	PinnedRepos []string       `json:"pinned_repos"`
	AICache     []aiCacheEntry `json:"ai_summary_cache"`
	AutoSpider  bool           `json:"auto_spider_sessions"`
}

// StateStore manages persistent state for the unfinished-work feature.
// All public methods are thread-safe.
type StateStore struct {
	mu    deadlock.RWMutex
	path  string
	state unfinishedState
}

// NewStateStore loads (or creates) the state file at the given path.
func NewStateStore(path string) (*StateStore, error) {
	s := &StateStore{
		path:  path,
		state: unfinishedState{AutoSpider: true},
	}
	if err := s.Load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load unfinished state: %w", err)
	}
	s.cleanupStaleEntries()
	return s, nil
}

// Load reads the state file from disk. Returns os.ErrNotExist if the file is missing.
func (s *StateStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var st unfinishedState
	if err := json.Unmarshal(data, &st); err != nil {
		return fmt.Errorf("unmarshal unfinished state: %w", err)
	}
	// Evict expired AI cache entries (older than 24h).
	var freshCache []aiCacheEntry
	for _, entry := range st.AICache {
		if time.Since(entry.GeneratedAt) < 24*time.Hour {
			freshCache = append(freshCache, entry)
		}
	}
	st.AICache = freshCache
	s.state = st
	return nil
}

// save writes state atomically via temp file + rename. Must be called with mu held (write).
func (s *StateStore) save() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal unfinished state: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write temp state file: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}

// --- Dismiss ---

// IsDismissed returns true when (repoPath, branch) has been permanently dismissed.
func (s *StateStore) IsDismissed(repoPath, branch string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.state.Dismissed {
		if d.RepoPath == repoPath && d.Branch == branch {
			return true
		}
	}
	return false
}

// Dismiss permanently hides (repoPath, branch) from unfinished-work results.
func (s *StateStore) Dismiss(repoPath, branch string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.state.Dismissed {
		if d.RepoPath == repoPath && d.Branch == branch {
			return nil // already dismissed
		}
	}
	s.state.Dismissed = append(s.state.Dismissed, dismissEntry{
		RepoPath:    repoPath,
		Branch:      branch,
		DismissedAt: time.Now(),
	})
	return s.save()
}

// Undismiss removes the dismiss record for (repoPath, branch).
func (s *StateStore) Undismiss(repoPath, branch string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []dismissEntry
	for _, d := range s.state.Dismissed {
		if d.RepoPath == repoPath && d.Branch == branch {
			continue
		}
		kept = append(kept, d)
	}
	s.state.Dismissed = kept
	return s.save()
}

// --- Snooze ---

// IsSnoozed returns true if the worktree is snoozed and the HEAD SHA has not changed.
// When currentSHA differs from the snooze-time SHA, the snooze is auto-cleared.
// Note: currentSHA here is the worktree path (we'll look up HEAD SHA on demand if needed).
// For simplicity, we accept the HEAD SHA directly.
func (s *StateStore) IsSnoozed(repoPath, branch, currentHeadSHA string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, sn := range s.state.Snoozed {
		if sn.RepoPath != repoPath || sn.Branch != branch {
			continue
		}
		if sn.SnoozeSinceSHA == currentHeadSHA {
			return true
		}
		// SHA changed — auto-clear.
		s.state.Snoozed = append(s.state.Snoozed[:i], s.state.Snoozed[i+1:]...)
		_ = s.save()
		return false
	}
	return false
}

// Snooze hides (repoPath, branch) until its HEAD SHA changes.
func (s *StateStore) Snooze(repoPath, branch, headSHA string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Remove existing snooze entry if present.
	var kept []snoozeEntry
	for _, sn := range s.state.Snoozed {
		if sn.RepoPath == repoPath && sn.Branch == branch {
			continue
		}
		kept = append(kept, sn)
	}
	kept = append(kept, snoozeEntry{
		RepoPath:       repoPath,
		Branch:         branch,
		SnoozeSinceSHA: headSHA,
		SnoozedAt:      time.Now(),
	})
	s.state.Snoozed = kept
	return s.save()
}

// Unsnooze removes the snooze record.
func (s *StateStore) Unsnooze(repoPath, branch string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var kept []snoozeEntry
	for _, sn := range s.state.Snoozed {
		if sn.RepoPath == repoPath && sn.Branch == branch {
			continue
		}
		kept = append(kept, sn)
	}
	s.state.Snoozed = kept
	return s.save()
}

// --- Config ---

// WatchDirs returns the configured watch directories.
func (s *StateStore) WatchDirs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.state.WatchDirs))
	copy(out, s.state.WatchDirs)
	return out
}

// PinnedRepos returns the configured pinned repos.
func (s *StateStore) PinnedRepos() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.state.PinnedRepos))
	copy(out, s.state.PinnedRepos)
	return out
}

// AutoSpiderEnabled returns whether auto-spider is enabled.
func (s *StateStore) AutoSpiderEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.AutoSpider
}

// SetConfig atomically replaces config fields and saves.
func (s *StateStore) SetConfig(autoSpider bool, watchDirs, pinnedRepos []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.AutoSpider = autoSpider
	s.state.WatchDirs = watchDirs
	s.state.PinnedRepos = pinnedRepos
	return s.save()
}

// --- AI Summary Cache ---

// GetCachedSummary returns a cached AI summary for (repoPath, branch, diffHash).
// Returns ("", false) on cache miss or expiry.
func (s *StateStore) GetCachedSummary(repoPath, branch, diffHash string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, e := range s.state.AICache {
		if e.RepoPath == repoPath && e.Branch == branch && e.DiffHash == diffHash {
			if time.Since(e.GeneratedAt) < 24*time.Hour {
				return e.Summary, true
			}
			return "", false
		}
	}
	return "", false
}

// CacheSummary stores an AI summary in the cache.
func (s *StateStore) CacheSummary(repoPath, branch, diffHash, summary string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Remove existing entry for this key.
	var kept []aiCacheEntry
	for _, e := range s.state.AICache {
		if e.RepoPath == repoPath && e.Branch == branch {
			continue
		}
		kept = append(kept, e)
	}
	kept = append(kept, aiCacheEntry{
		RepoPath:    repoPath,
		Branch:      branch,
		DiffHash:    diffHash,
		Summary:     summary,
		GeneratedAt: time.Now(),
	})
	s.state.AICache = kept
	return s.save()
}

// ComputeDiffHash runs `git -C path diff HEAD` and SHA256-hashes the output.
func ComputeDiffHash(worktreePath string) (string, error) {
	exec3s := executor.MakeTimeoutExecutor(5 * time.Second)
	cmd := safeexec.CommandContext(context.Background(), "git", "-C", worktreePath, "diff", "HEAD")
	out, err := exec3s.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("git diff HEAD: %w", err)
	}
	sum := sha256.Sum256(out)
	return fmt.Sprintf("%x", sum), nil
}

// cleanupStaleEntries removes dismissed/snoozed entries whose repoPath no longer exists.
func (s *StateStore) cleanupStaleEntries() {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false

	var keptDismissed []dismissEntry
	for _, d := range s.state.Dismissed {
		if _, err := os.Stat(d.RepoPath); err == nil {
			keptDismissed = append(keptDismissed, d)
		} else {
			log.Debug("removing stale dismissed entry", "repo", d.RepoPath, "branch", d.Branch)
			changed = true
		}
	}
	s.state.Dismissed = keptDismissed

	var keptSnoozed []snoozeEntry
	for _, sn := range s.state.Snoozed {
		if _, err := os.Stat(sn.RepoPath); err == nil {
			keptSnoozed = append(keptSnoozed, sn)
		} else {
			log.Debug("removing stale snoozed entry", "repo", sn.RepoPath, "branch", sn.Branch)
			changed = true
		}
	}
	s.state.Snoozed = keptSnoozed

	if changed {
		if err := s.save(); err != nil {
			log.Warn("failed to save after cleanup", "err", err)
		}
	}
}
