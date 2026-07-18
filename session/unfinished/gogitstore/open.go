package gogitstore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	billy "github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/osfs"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
)

// Open opens worktreePath as a *git.Repository backed by a WorktreeStorer,
// sharing its SharedObjectStore (index + decoded-object cache) with every
// other worktree of the same repository previously opened through reg.
//
// This replicates the filesystem-resolution half of go-git's own
// git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true,
// EnableDotGitCommonDir: true}) — that logic
// (dotGitToOSFilesystems/dotGitCommonDirectory in go-git's repository.go)
// is unexported, so it cannot be called directly; it is reproduced here
// rather than reinvented; see resolveGitFilesystems's doc comment.
//
// Unlike PlainOpenWithOptions, Open does not walk up parent directories
// looking for a .git — worktreePath must directly contain (or be pointed
// at by) a .git entry. This matches how session/unfinished's Scanner calls
// it: always with a path already known to be a worktree root.
func Open(worktreePath string, reg *Registry) (*git.Repository, error) {
	dotFs, commonFs, wtFs, commonDirAbs, err := resolveGitFilesystems(worktreePath)
	if err != nil {
		return nil, fmt.Errorf("gogitstore: resolve %s: %w", worktreePath, err)
	}

	shared := reg.acquire(commonDirAbs, commonFs)

	repositoryFs := dotgit.NewRepositoryFilesystem(dotFs, commonFs)
	storage := filesystem.NewStorageWithOptions(repositoryFs, throwawayObjectCache(), filesystem.Options{})

	ws := &WorktreeStorer{Storage: storage, shared: shared}
	return git.Open(ws, wtFs)
}

// resolveGitFilesystems resolves worktreePath's private gitdir filesystem
// (dotFs — HEAD, index, worktree-local refs), its commondir filesystem
// (commonFs — objects, most refs, config; shared by every worktree of this
// repository), and the worktree's own filesystem (wtFs), following exactly
// the same two cases go-git's own PlainOpenWithOptions does:
//
//  1. worktreePath/.git is a directory: this IS the main worktree. dotFs is
//     that directory; there is no commondir file, so commonFs == dotFs.
//  2. worktreePath/.git is a file containing "gitdir: <path>" (a linked
//     worktree, i.e. `git worktree add`): dotFs is the pointed-to
//     directory (normally $MAIN_REPO/.git/worktrees/<name>), which itself
//     contains a "commondir" file pointing back at $MAIN_REPO/.git — that
//     becomes commonFs.
func resolveGitFilesystems(worktreePath string) (dotFs, commonFs, wtFs billy.Filesystem, commonDirAbs string, err error) {
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return nil, nil, nil, "", err
	}
	wtFs = osfs.New(absPath)

	fi, err := wtFs.Stat(".git")
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("stat .git: %w", err)
	}

	if fi.IsDir() {
		dotFs, err = wtFs.Chroot(".git")
		if err != nil {
			return nil, nil, nil, "", err
		}
	} else {
		gitDirPath, gerr := readGitdirFile(wtFs, absPath)
		if gerr != nil {
			return nil, nil, nil, "", gerr
		}
		dotFs = osfs.New(gitDirPath)
	}

	commonFs = dotFs
	commonDirAbs = filepath.Clean(dotFs.Root())

	if cf, cerr := dotFs.Open("commondir"); cerr == nil {
		b, rerr := io.ReadAll(cf)
		_ = cf.Close()
		if rerr != nil {
			return nil, nil, nil, "", rerr
		}
		p := strings.TrimSpace(string(b))
		if !filepath.IsAbs(p) {
			p = filepath.Join(dotFs.Root(), p)
		}
		p = filepath.Clean(p)
		commonFs = osfs.New(p)
		commonDirAbs = p
	} else if !os.IsNotExist(cerr) {
		return nil, nil, nil, "", cerr
	}

	return dotFs, commonFs, wtFs, commonDirAbs, nil
}

// readGitdirFile reads worktreePath/.git (a "gitdir: <path>" pointer file,
// as `git worktree add` writes for linked worktrees) and returns the
// absolute path it points to.
func readGitdirFile(wtFs billy.Filesystem, worktreeAbsPath string) (string, error) {
	f, err := wtFs.Open(".git")
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	line := strings.TrimSpace(string(b))
	const prefix = "gitdir: "
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf(".git file has no %q prefix", prefix)
	}
	gitDirPath := strings.TrimPrefix(line, prefix)
	if !filepath.IsAbs(gitDirPath) {
		gitDirPath = filepath.Join(worktreeAbsPath, gitDirPath)
	}
	return filepath.Clean(gitDirPath), nil
}
