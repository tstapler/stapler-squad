package git

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/tstapler/stapler-squad/config"
)

// worktreeLockTimeout mirrors config.DefaultLockTimeout / suspendedProcessLockTimeout,
// but is longer: a git worktree add/remove subprocess queued behind others during a
// backlog-triage burst can plausibly take longer to drain than a JSON-file write.
const worktreeLockTimeout = 30 * time.Second

// repoWorktreeLock provides both intra-process and cross-process mutual exclusion for
// git worktree metadata operations (add/remove/prune) against a single repository.
//
// mu provides real intra-process mutual exclusion. flock.Flock guards against other OS
// processes but a single *flock.Flock value gives no such guarantee against concurrent
// goroutines within this process -- two goroutines can both pass TryLockContext (flock is
// reentrant/no-op within one process on most platforms) and interleave their git worktree
// admin-file writes. mu is taken in addition to, not instead of, the flock call below. This
// mirrors session/suspended_process_store.go's withWriteLock idiom.
type repoWorktreeLock struct {
	mu    sync.Mutex
	flock *flock.Flock
}

var (
	// worktreeLockRegistryMu guards worktreeLockRegistry itself (not the per-repo locks it
	// holds). A *flock.Flock must be reused across calls for the same repo path -- a fresh
	// flock.Flock per call opens a new, independent fd, and flock() treats different fds on
	// the same file as independent locks, which would defeat the cross-process guarantee.
	worktreeLockRegistryMu sync.Mutex
	worktreeLockRegistry   = make(map[string]*repoWorktreeLock)
)

// lockForRepo returns the process-wide singleton repoWorktreeLock for repoPath, creating it
// on first use. The lock file lives outside repoPath (under the app config dir) so it isn't
// itself subject to git worktree operations on the repo it's protecting.
func lockForRepo(repoPath string) (*repoWorktreeLock, error) {
	absRepoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve absolute repo path %s: %w", repoPath, err)
	}

	worktreeLockRegistryMu.Lock()
	defer worktreeLockRegistryMu.Unlock()

	if l, ok := worktreeLockRegistry[absRepoPath]; ok {
		return l, nil
	}

	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("worktree lock: failed to get config directory: %w", err)
	}
	locksDir := filepath.Join(configDir, "worktree-locks")
	if err := os.MkdirAll(locksDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree locks directory: %w", err)
	}

	digest := sha256.Sum256([]byte(absRepoPath))
	lockPath := filepath.Join(locksDir, hex.EncodeToString(digest[:])+".lock")

	l := &repoWorktreeLock{flock: flock.New(lockPath)}
	worktreeLockRegistry[absRepoPath] = l
	return l, nil
}

// WithRepoWorktreeLock serializes fn against every other goroutine and OS process (on this
// machine) operating on repoPath's git worktree metadata. See repoWorktreeLock's doc comment
// for why both mu and flock are required.
//
// Exported so callers outside this package that need to run other repoPath-mutating work
// (e.g. session.RepairCorruptedGitRepo's destructive re-clone) in the same critical section
// as a GitWorktree.Setup()/Remove() can do so via GitWorktree.SetupLocked() — see that
// method's doc comment for the race this closes.
func WithRepoWorktreeLock(repoPath string, fn func() error) error {
	l, err := lockForRepo(repoPath)
	if err != nil {
		return err
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), worktreeLockTimeout)
	defer cancel()
	locked, err := l.flock.TryLockContext(ctx, 100*time.Millisecond)
	if err != nil {
		return fmt.Errorf("failed to acquire worktree lock for %s: %w", repoPath, err)
	}
	if !locked {
		return fmt.Errorf("could not acquire worktree lock for %s within timeout", repoPath)
	}
	defer func() { _ = l.flock.Unlock() }()

	return fn()
}
