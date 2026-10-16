package unfinished

// dirtyTracker is implemented by readers that keep per-worktree DirtyGen state
// (GoGitVCSReader), so the scanner can feed watcher observations to it.
type dirtyTracker interface {
	ArmDirty(worktreePath string)
	DisarmDirty(worktreePath string)
	NoteDirty(worktreePath string, kind EventKind)
}

// dirtyGenFor returns worktreePath's DirtyGen, creating an unarmed one.
func (g *GoGitVCSReader) dirtyGenFor(worktreePath string) *DirtyGen {
	if d, ok := g.scanSkip.dirtyGens.Load(worktreePath); ok {
		return d
	}
	d, _ := g.scanSkip.dirtyGens.get().LoadOrCompute(worktreePath, func() (*DirtyGen, bool) {
		return NewDirtyGen(g.scanSkip.now), false
	})
	return d
}

// ArmDirty marks worktreePath's watcher live; its next scan is a full walk.
func (g *GoGitVCSReader) ArmDirty(worktreePath string) { g.dirtyGenFor(worktreePath).Arm() }

// DisarmDirty marks the watcher dead: every scan is a full walk until re-armed.
func (g *GoGitVCSReader) DisarmDirty(worktreePath string) { g.dirtyGenFor(worktreePath).Disarm() }

// NoteDirty records a watcher observation for worktreePath.
func (g *GoGitVCSReader) NoteDirty(worktreePath string, kind EventKind) {
	g.dirtyGenFor(worktreePath).Note(kind)
}

// scanSkipDecision tells one HasUncommitted scan whether cached state may be
// trusted. force bypasses the parsed-index and untracked-dir caches (they are
// refreshed from disk); done must be called once the walk completed.
//
// DirtyGen gates only the caches. It never licenses skipping the tracked-file
// stat pass (ADR-006 hard rule). A watcher hint on a tracked path (HEAD/index
// event) forces a re-parse but is not turned into dirty=true: HEAD and index
// events fire for commits and checkouts that leave the tree clean.
func (g *GoGitVCSReader) scanSkipDecision(worktreePath string) (force bool, done func()) {
	if !g.effectiveFlags().ScanSkipDirtyGen {
		return false, func() {}
	}
	dg := g.dirtyGenFor(worktreePath)
	dec := dg.Decide()
	force = dec.FullWalk || dec.DirtyHint
	return force, func() {
		if dec.FullWalk {
			dg.CompleteFullWalk(dec)
		}
		dg.AckHint(dec)
	}
}

// scanSkipPeekForce is scanSkipDecision's read-only form for the diff walk: it
// reports a pending forced walk without counting or clearing it.
func (g *GoGitVCSReader) scanSkipPeekForce(worktreePath string) bool {
	if !g.effectiveFlags().ScanSkipDirtyGen {
		return false
	}
	dec := g.dirtyGenFor(worktreePath).Peek()
	return dec.FullWalk || dec.DirtyHint
}
