package unfinished

import (
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
)

// Index-versus-HEAD classification, staged/unstaged target lists and racy-stat content checks.

// indexMeta is one stage-0 index entry classified against HEAD.
type indexMeta struct {
	name       string
	indexHash  plumbing.Hash
	headHash   plumbing.Hash // zero if not in HEAD
	size       uint32
	modifiedAt time.Time
	staged     bool // index hash differs from HEAD
	// skipWorktree marks a sparse-checkout entry; under scan_skip an absent one is
	// not a deletion, matching hasUncommittedSkip.
	skipWorktree bool
}

// stagedDel is a file in HEAD but absent from the index.
type stagedDel struct {
	name     string
	headHash plumbing.Hash
}

// classifyIndex compares index entries with HEAD tree hashes (no blob reads).
func classifyIndex(idx *index.Index, headHashes map[string]plumbing.Hash) ([]indexMeta, map[string]struct{}, []stagedDel) {
	metas := make([]indexMeta, 0, len(idx.Entries))
	indexedNames := make(map[string]struct{}, len(idx.Entries))
	for _, e := range idx.Entries {
		if e.Stage != 0 {
			continue
		}
		indexedNames[e.Name] = struct{}{}
		headHash := headHashes[e.Name]
		metas = append(metas, indexMeta{
			name:       e.Name,
			indexHash:  e.Hash,
			headHash:   headHash,
			size:       e.Size,
			modifiedAt: e.ModifiedAt,
			staged:     e.Hash != headHash,

			skipWorktree: e.SkipWorktree,
		})
	}
	var stagedDels []stagedDel
	for name, headHash := range headHashes {
		if _, inIdx := indexedNames[name]; !inIdx {
			stagedDels = append(stagedDels, stagedDel{name, headHash})
		}
	}
	return metas, indexedNames, stagedDels
}

// stagedTargetList lists staged modifications and staged deletions.
func stagedTargetList(metas []indexMeta, stagedDels []stagedDel) []changeTarget {
	targets := make([]changeTarget, 0, len(stagedDels))
	for _, m := range metas {
		if m.staged {
			targets = append(targets, changeTarget{m.name, m.headHash, false})
		}
	}
	for _, sd := range stagedDels {
		targets = append(targets, changeTarget{sd.name, sd.headHash, true})
	}
	return targets
}

// unstagedTargetList stats every non-staged entry and lists those that differ
// from the index (or may, inside the racy-git window).
func (g *GoGitVCSReader) unstagedTargetList(worktreePath string, metas []indexMeta, indexMtime time.Time, pass *statPass) []changeTarget {
	unstagedTargets := make([]changeTarget, 0)
	passCursor := pass.cursor()
	skipSparse := g.effectiveFlags().ScanSkip
	for _, m := range metas {
		if m.staged {
			continue
		}
		wtPath := filepath.Join(worktreePath, m.name)
		info, serr := g.lstatTracked(&passCursor, m.name, wtPath)
		if serr != nil {
			if os.IsNotExist(serr) && (!skipSparse || !m.skipWorktree) {
				unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, true})
			}
			continue
		}
		sizeDiffers := info.Size() != int64(m.size)
		mtimeDiffers := !info.ModTime().Truncate(time.Second).Equal(m.modifiedAt.Truncate(time.Second))
		if sizeDiffers || mtimeDiffers {
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}

		// Racy-clean case: size matches the index entry AND the working-tree
		// mtime (truncated to second) equals the index entry's recorded mtime.
		// stat alone cannot prove the file is unchanged in the racy window —
		// a file rewritten with identical size within the same wall-clock
		// second as the index write looks clean by stat but may differ in
		// content (the classic "racy git" problem). Outside that window
		// (file mtime clearly predates the index write), stat alone is
		// sufficient proof of no change, exactly as real git treats it — so
		// only pay for a content hash when the file could actually be racy.
		racy := fileNeedsContentCheck(info.ModTime(), indexMtime)
		moved := nanosMoved(info.ModTime(), m.modifiedAt)
		if !racy && !moved {
			continue // stat-clean and outside the racy window: confirmed unchanged
		}
		if !racy {
			// Not capped: treating an unhashed file as modified would change the
			// verdict, so the extra hashes are only counted.
			g.nanosForcedHashes.Add(1)
		}
		if info.Size() > maxUntrackedFileSize {
			// Too large to hash within the cap; conservatively treat as changed
			// rather than reading it into memory, consistent with how large
			// modified tracked files are handled elsewhere (applyDiff skips the
			// read but still counts the file via readFileIfSmall returning false).
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
			continue
		}
		if !racyFileMatchesIndex(wtPath, m.indexHash) {
			// Unreadable, raced past the cap, or content differs: changed.
			unstagedTargets = append(unstagedTargets, changeTarget{m.name, m.indexHash, false})
		}
	}
	return unstagedTargets
}

// racyFileMatchesIndex reports whether the file at path hashes to indexHash;
// unreadable files and files past the size cap report false (treated as changed).
func racyFileMatchesIndex(path string, indexHash plumbing.Hash) bool {
	bp, ok := readSmallFilePooled(path)
	if !ok {
		return false
	}
	defer releaseFileBuf(bp)
	return plumbing.ComputeHash(plumbing.BlobObject, *bp) == indexHash
}

// nanosMoved reports a sub-second mtime change the second-truncated comparison
// cannot see: a same-size rewrite in the second the entry was recorded. go-git
// does not smudge racy entries when it rewrites the index, so indexMtime alone
// cannot expose it. An entry with zero nanoseconds came from a second-precision
// writer, so it proves nothing and is not treated as moved.
func nanosMoved(fileMtime, entryMtime time.Time) bool {
	return entryMtime.Nanosecond() != 0 && fileMtime.Nanosecond() != entryMtime.Nanosecond()
}

// fileNeedsContentCheck reports whether a stat-clean file (size and mtime both
// matching the index entry) still requires a content-hash fallback to rule
// out the racy-git case. A file is only racy if its own mtime is not clearly
// before the index was last written — the same window real git uses — so a
// file whose mtime predates the index write is safe to trust from stat alone.
// An unknown indexMtime (zero value) is treated conservatively as racy.
func fileNeedsContentCheck(fileMtime, indexMtime time.Time) bool {
	if indexMtime.IsZero() {
		return true
	}
	return !fileMtime.Truncate(time.Second).Before(indexMtime.Truncate(time.Second))
}

// readFileIfSmall reads the file at path only if its size is ≤ maxUntrackedFileSize.
// Returns (data, true) on success; (nil, false) when the file is too large or unreadable.
// This is the single safe wrapper for all content reads in diffShortstatUncached —
// callers should use this instead of os.ReadFile to prevent OOM on large build artifacts.
func readFileIfSmall(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false
	}
	if info.Size() > maxUntrackedFileSize {
		return nil, false
	}
	// path is walked from repoPath by walkUntrackedFiles (an internal directory
	// walk over the session's own git worktree), not user/RPC input.
	data, err := os.ReadFile(path) // #nosec G304 -- path comes from an internal directory walk over the worktree, not user input
	if err != nil {
		return nil, false
	}
	return data, true
}
