package unfinished

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"github.com/tstapler/stapler-squad/session/unfinished/repostate"
)

// Lock chain for the shared commit graph (ADR-001, ADR-009). Phase 1 of an
// ahead/behind call takes entry.mu to resolve refs and releases it before any
// graph work. Everything below runs without entry.mu:
//
//	repostate walk -> arena CAS / xsync bucket locks (never held across a read)
//	  -> graphSource.Parents -> WorktreeStorer.EncodedObject
//	  -> SharedObjectStore.mu (innermost leaf, held across file I/O by design)
//
// No path acquires entry.mu while inside a Fill or holding a repostate lock;
// the plain-walk fallback retakes entry.mu only after the walk has returned.

// graphSource adapts a CommitParentsReader over one worktree's storer to the
// repostate fill seam. It takes no entry.mu: the storer (a gogitstore
// WorktreeStorer) reads through the self-locking shared store. One value is
// used by one walk on one goroutine, so it reuses a parents buffer.
type graphSource struct {
	st      storer.EncodedObjectStorer
	reader  CommitParentsReader
	shallow bool
	buf     []plumbing.Hash
	decodes int       // commits decoded by this walk; non-zero means it extended the graph
	born    time.Time // walk start, for the graph-extension duration
}

func newGraphSource(st storer.EncodedObjectStorer, shallow bool) *graphSource {
	return &graphSource{st: st, reader: stockCommitParents, shallow: shallow, born: time.Now()}
}

var _ repostate.ParentsSource = (*graphSource)(nil)

// Parents reads h's parents and committer time. In a shallow repository an
// absent commit is a boundary (truncated, remembered under the stamp); anywhere
// else not-found is returned as an error so a transient miss is never cached.
func (s *graphSource) Parents(_ context.Context, h plumbing.Hash) ([]plumbing.Hash, int64, bool, error) {
	parents, when, err := s.reader.Parents(s.st, h, s.buf[:0])
	s.buf = parents
	s.decodes++
	if err != nil {
		if s.shallow && errors.Is(err, plumbing.ErrObjectNotFound) {
			return nil, 0, true, nil
		}
		return nil, 0, false, err
	}
	return parents, when, false, nil
}

// commonDirKeyer is satisfied by *gogitstore.WorktreeStorer.
type commonDirKeyer interface{ CommonDirKey() string }

// observeShallow stats `<commondir>/shallow` once and publishes the stamp to
// rs, returning the ordinal the walk runs under and whether the repository is
// shallow. The stat is counted by the statFn seam like every other stat.
func (g *GoGitVCSReader) observeShallow(entry *cachedRepo, rs *repostate.RepoState) (repostate.StampOrd, bool) {
	var stamp repostate.ShallowStamp
	if k, ok := entry.repo.Storer.(commonDirKeyer); ok {
		g.noteStat("shallow")
		if fi, err := os.Stat(filepath.Join(k.CommonDirKey(), "shallow")); err == nil {
			stamp = repostate.ShallowStamp{Exists: true, Size: fi.Size(), MtimeNs: fi.ModTime().UnixNano()}
		}
	}
	return rs.ObserveShallow(stamp), stamp.Exists
}
