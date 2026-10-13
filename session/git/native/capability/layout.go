// Package capability detects repository features the in-process git backend cannot reproduce
// (hooks, signing, LFS and other filters, sparse checkout, split index, submodules, shallow
// clones, local and proxied transports; ADR-006) and reports them as typed
// backend.Capability values the Router turns into fallback reasons.
//
// Configuration comes only from the gitconfig package, never from go-git's config decoder.
// Hooks and signing are re-evaluated on every call; only the index-derived facts are cached,
// keyed by the index file's trailing checksum.
package capability

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/native/gitconfig"
)

const maxSmallFile = 64 << 10 // .git files, commondir

// layout is the resolved git directory pair of a worktree.
type layout struct {
	root      string // the worktree root as supplied
	gitDir    string // per-worktree git dir
	commonDir string // shared config, hooks, refs
}

func detectErr(sub backend.DetectSub, err error) error {
	return &backend.DetectError{Sub: sub, Err: err}
}

// resolveLayout finds GitDir and CommonDir for a worktree root. `.git` is a directory in the
// main checkout and a file ("gitdir: <path>") in a linked worktree or submodule, whose
// `commondir` file (when present) names the shared directory.
func resolveLayout(root string) (layout, error) {
	l := layout{root: root}
	dotGit := filepath.Join(root, ".git")
	fi, err := os.Stat(dotGit)
	if err != nil {
		return l, detectErr(backend.DetectCommonDir, err)
	}
	if fi.IsDir() {
		l.gitDir, l.commonDir = dotGit, dotGit
		return l, nil
	}
	data, err := gitconfig.ReadRegularFile(dotGit, maxSmallFile)
	if err != nil {
		return l, detectErr(backend.DetectCommonDir, err)
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	target, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return l, detectErr(backend.DetectCommonDir, errors.New(".git file has no gitdir: line"))
	}
	target = strings.TrimSpace(target)
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	l.gitDir = filepath.Clean(target)
	l.commonDir = l.gitDir
	cd, err := gitconfig.ReadRegularFile(filepath.Join(l.gitDir, "commondir"), maxSmallFile)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return l, detectErr(backend.DetectCommonDir, err)
	default:
		rel := strings.TrimSpace(string(cd))
		if !filepath.IsAbs(rel) {
			rel = filepath.Join(l.gitDir, rel)
		}
		l.commonDir = filepath.Clean(rel)
	}
	if fi, err := os.Stat(l.commonDir); err != nil || !fi.IsDir() {
		return l, detectErr(backend.DetectCommonDir, errors.New("commondir does not resolve to a directory"))
	}
	return l, nil
}
