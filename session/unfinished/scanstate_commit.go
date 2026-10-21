package unfinished

import (
	"context"
	"slices"
)

// Committed-state reuse across scans: fillCommittedState, changed-first ordering and signature change checks.

// fillCommittedState sets the committed-state fields of result. When the
// worktree's committed key is unchanged and younger than one tick it reuses the
// stored values and skips AheadBehind and CommitMessages; HasUncommitted and
// the diff are the caller's and always run.
func (s *Scanner) fillCommittedState(ctx context.Context, result *ScanResult, repoPath, worktreePath, base string) {
	snap := s.burstSnapshot(repoPath)
	sig, haveSig := snap.signatureOf(worktreePath)
	var wrec *worktreeScanRecord
	if haveSig {
		wrec = s.scanRecord(repoPath).worktree(worktreePath)
		s.scanStats.signatureChecks.Add(1)
		if cs := wrec.committed.Load(); cs != nil && cs.key == sig.committed() && s.now().Sub(cs.storedAt) < s.signatureMaxAge() {
			s.scanStats.signatureSkips.Add(1)
			result.AheadCount, result.BehindCount = cs.ahead, cs.behind
			result.AheadMessages = slices.Clone(cs.messages)
			wrec.last.Store(&sig)
			return
		}
	}
	complete := false
	ahead, behind, aErr := s.aheadBehind(ctx, snap, worktreePath, base)
	if aErr == nil {
		result.AheadCount = ahead
		result.BehindCount = behind
		complete = true
		if ahead > 0 {
			msgs, mErr := s.reader.CommitMessages(ctx, worktreePath, base, 5)
			if mErr == nil {
				result.AheadMessages = msgs
			} else {
				complete = false
			}
		}
	}
	// A cancelled compute left no committed state: recording the signature as scanned
	// would make changedFirst and the sibling check treat it as done. A completed
	// failure (e.g. no base ref) is still recorded, or enqueueRepo would rescan forever,
	// and a fully computed result stays valid if the cancel lands just after it.
	if haveSig && (complete || ctx.Err() == nil) {
		wrec.last.Store(&sig)
		if complete {
			wrec.committed.Store(&committedState{key: sig.committed(), ahead: ahead, behind: behind, messages: slices.Clone(result.AheadMessages), storedAt: s.now()})
		}
	}
}

// changedFirst orders worktrees so those whose signature changed since their
// last scan come first (stable within each group), so a fresh commit is
// reported before the unchanged siblings are looked at.
func (s *Scanner) changedFirst(repoPath string, wts []WorktreeInfo) []WorktreeInfo {
	snap := s.burstSnapshot(repoPath)
	if snap == nil {
		return wts
	}
	rec := s.scanRecord(repoPath)
	out := make([]WorktreeInfo, 0, len(wts))
	var unchanged []WorktreeInfo
	for _, wt := range wts {
		if rec.signatureChanged(snap, wt.Path) {
			out = append(out, wt)
		} else {
			unchanged = append(unchanged, wt)
		}
	}
	return append(out, unchanged...)
}

// signatureChanged reports whether worktreePath's signature in snap differs
// from the one recorded at its last scan. A worktree never scanned, or whose
// refs did not resolve, counts as changed.
func (r *repoScanRecord) signatureChanged(snap *RefSnapshot, worktreePath string) bool {
	sig, ok := snap.signatureOf(worktreePath)
	if !ok {
		return true
	}
	v, ok := r.worktrees.Load(worktreePath)
	if !ok {
		return true
	}
	w, _ := v.(*worktreeScanRecord)
	last := w.last.Load()
	return last == nil || *last != sig
}

// siblingSignatureChanged is enqueueRepo's check behind its "any worktree cache
// is fresh, skip the repo" rule: a fresh sibling must not hide a worktree whose
// HEAD, base, index or watcher generation moved. It resolves refs (a handful of
// stats and file reads) and only runs when that rule would otherwise skip.
func (s *Scanner) siblingSignatureChanged(repoPath string) bool {
	if !s.preludeEnabled() {
		return false
	}
	r, ok := s.reader.(refSnapshotReader)
	if !ok {
		return false
	}
	v, ok := s.scanRecords.Load(repoPath)
	if !ok {
		return false
	}
	rec, _ := v.(*repoScanRecord)
	if rec == nil {
		return false
	}
	probe := rec.probe.Load()
	if probe == nil {
		return false
	}
	snap := r.ResolveRefSnapshot(repoPath, probe.base, probe.paths)
	for _, p := range probe.paths {
		if rec.signatureChanged(snap, p) {
			return true
		}
	}
	return false
}
