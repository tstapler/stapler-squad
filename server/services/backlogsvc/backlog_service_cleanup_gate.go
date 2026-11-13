package backlogsvc

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
)

// cleanupWiring is BacklogService's cleanup-gate state: the gate, the live-work probe
// it shares with the lifecycle listener, the blocked set behind the metrics and the
// stuck report, and the worktree-removal seam.
type cleanupWiring struct {
	storage *session.Storage

	mu      sync.Mutex
	gate    *session.PRCleanupGate
	probe   session.LiveWorkProbe
	blocked *session.BlockedSet

	// liveEnabled replaces the reader:backlog lookup in tests.
	liveEnabled func() bool
	// removeFn replaces the git worktree removal in tests; nil: git cleanup.
	removeFn func(wt domain.GitWorktreeData) error
}

func newCleanupWiring(storage *session.Storage) *cleanupWiring {
	w := &cleanupWiring{storage: storage, blocked: session.NewBlockedSet()}
	w.rebuildGate()
	return w
}

func (w *cleanupWiring) scopeOn() bool {
	if w.liveEnabled != nil {
		return w.liveEnabled()
	}
	return config.SessionMultiPR(config.SessionMultiPRScopeBacklog)
}

func (w *cleanupWiring) rebuildGate() {
	if w.storage == nil {
		return
	}
	w.gate = session.NewPRCleanupGate(w.storage, w.probe, w.scopeOn)
}

func (w *cleanupWiring) gateAndSet() (*session.PRCleanupGate, *session.BlockedSet) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.gate, w.blocked
}

// ruleLProbe is the probe RULE L consults: nil unless the scope is on.
func (w *cleanupWiring) ruleLProbe() session.LiveWorkProbe {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.scopeOn() {
		return nil
	}
	return w.probe
}

func (w *cleanupWiring) remove(wt domain.GitWorktreeData) error {
	if w.removeFn != nil {
		return w.removeFn(wt)
	}
	return git.NewGitWorktreeFromStorage(wt.RepoPath, wt.WorktreePath, wt.SessionName, wt.BranchName, wt.BaseCommitSHA).Cleanup()
}

// SetLiveWorkProbe wires the live-work probe shared with the lifecycle listener and
// rebuilds the gate around it.
func (s *BacklogService) SetLiveWorkProbe(p session.LiveWorkProbe) {
	s.cleanup.mu.Lock()
	defer s.cleanup.mu.Unlock()
	s.cleanup.probe = p
	s.cleanup.rebuildGate()
}

// LiveWorkProbe returns the probe the service and its gate share (nil when unwired).
func (s *BacklogService) LiveWorkProbe() session.LiveWorkProbe {
	s.cleanup.mu.Lock()
	defer s.cleanup.mu.Unlock()
	return s.cleanup.probe
}

// CleanupGate exposes the gate for read-only callers (warnings on ungated removals).
func (s *BacklogService) CleanupGate() *session.PRCleanupGate {
	g, _ := s.cleanup.gateAndSet()
	return g
}

// BlockedCleanups lists items whose worktree cleanup is blocked now, oldest first.
func (s *BacklogService) BlockedCleanups() []session.BlockedItem {
	_, set := s.cleanup.gateAndSet()
	return set.Items()
}

// gateOutcome runs the gate; blocked true means the caller must remove nothing.
// The first block of a (path, item) pair is counted and logged once.
func (s *BacklogService) gateOutcome(ctx context.Context, sessions []backlogdto.ItemSessionSummary, opts session.GateOptions) (session.CleanupOutcome, bool) {
	gate, set := s.cleanup.gateAndSet()
	if gate == nil {
		return session.CleanupOutcome{}, false
	}
	itemID := firstItemID(sessions)
	block, blocked := gate.Blocked(ctx, sessions, opts)
	if !blocked {
		if itemID != "" {
			set.Unblock(itemID)
		}
		return session.CleanupOutcome{}, false
	}
	path := session.CleanupPathFrom(ctx)
	if set.Block(path, itemID, block.Reason, time.Now()) {
		session.RecordCleanupBlocked(path)
		log.WarningLog().Printf("[cleanupGate] path=%s item=%s worktree removal blocked: %s", path, itemID, block.Reason)
	}
	session.SetCleanupBlockedItems(path, set.Count(path))
	return session.CleanupBlockedOutcome(block), true
}

func firstItemID(sessions []backlogdto.ItemSessionSummary) string {
	for _, is := range sessions {
		if is.BacklogItemID != "" {
			return is.BacklogItemID
		}
	}
	return ""
}
