package unfinished

import "fmt"

var _ oracleReader = (*GoGitVCSReader)(nil)

// shadowOracle is a second reader with every rollback unit off, so it runs the
// legacy code paths with its own repo handles and caches, never sharing state
// with the new path it checks.
func (g *GoGitVCSReader) shadowOracle() *GoGitVCSReader {
	if o := g.scanSkip.oracle.Load(); o != nil {
		return o
	}
	o := &GoGitVCSReader{}
	o.scanSkip.flags = &Flags{}
	if g.scanSkip.oracle.CompareAndSwap(nil, o) {
		return o
	}
	return g.scanSkip.oracle.Load()
}

// OracleAheadBehind is the plainWalk: refs, then the per-call parentMemo walk,
// with no TTL cache, no shared graph and no signature reuse.
func (g *GoGitVCSReader) OracleAheadBehind(worktreePath, base string) (int, int, error) {
	o := g.shadowOracle()
	entry, err := o.openRepoEntry(worktreePath)
	if err != nil {
		return 0, 0, fmt.Errorf("oracle open %s: %w", worktreePath, err)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock() // REQUIRED: defer so a panic releases it
	head, baseHash, err := resolveAheadBehindRefs(entry, base)
	if err != nil {
		return 0, 0, err
	}
	if head == baseHash {
		return 0, 0, nil
	}
	return aheadBehindFrom(newParentMemo(entry.repo), head, baseHash)
}

// OracleCommitMessages is the legacy reachable-set walk, uncached.
func (g *GoGitVCSReader) OracleCommitMessages(worktreePath, base string, limit int) ([]string, error) {
	o := g.shadowOracle()
	entry, err := o.openRepoEntry(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("oracle open %s: %w", worktreePath, err)
	}
	head, baseHash, err := resolveCommitMessageRefs(entry, base)
	if err != nil {
		return nil, err
	}
	entry.mu.Lock()
	set, err := reachableSet(entry.repo, baseHash)
	entry.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return commitMessagesFrom(entry, head, set, limit)
}

// OracleHasUncommitted is the flags-off HasUncommitted, its TTL entry evicted first.
func (g *GoGitVCSReader) OracleHasUncommitted(worktreePath string) (bool, error) {
	o := g.shadowOracle()
	o.InvalidateWorktree(worktreePath)
	return o.HasUncommitted(worktreePath)
}

// OracleDiffShortstat is the flags-off, uncached diff (no per-file memo).
func (g *GoGitVCSReader) OracleDiffShortstat(worktreePath string) (DiffStat, error) {
	return g.shadowOracle().diffShortstatUncached(worktreePath)
}

// OracleDone drops the oracle's repo handles so a sampled scan holds no memory.
func (g *GoGitVCSReader) OracleDone() {
	if o := g.scanSkip.oracle.Load(); o != nil {
		o.ClearCache()
	}
}
