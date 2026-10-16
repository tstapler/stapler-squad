package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/tstapler/stapler-squad/session/unfinished"
)

// Corpus layout under a corpus root:
//
//	repos/<name>/          byte copy of the source repo (.git and working tree)
//	worktrees/<name>-w<i>  linked worktrees of that copy
//	manifest.json          what was built and what was skipped
//
// Everything is plain Go file I/O plus go-git: no git binary, no cp.
const (
	reposDir     = "repos"
	worktreesDir = "worktrees"
	manifestName = "manifest.json"
	stateFile    = "unfinished_state.json"
)

// alwaysSkip is dropped from every copy: the source's linked-worktree admin
// dirs hold absolute paths back into the owner's checkouts.
const alwaysSkip = ".git/worktrees"

// CorpusOptions configures BuildCorpus.
type CorpusOptions struct {
	Sources   []string // owner repos, opened read-only
	Root      string   // corpus root (must not already contain a copy of a source)
	Worktrees int      // linked worktrees per copied repo (W)
	MaxRepoMB int64    // skip repos above this many MB after skips; <=0 disables
	SkipPaths []string // extra slash-separated paths relative to the repo root to leave out
	MaxRepos  int      // stop after this many copied repos; <=0 means no cap
	Log       io.Writer
}

// CorpusRepo is one copied repo in the manifest.
type CorpusRepo struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Path      string   `json:"path"`
	Worktrees []string `json:"worktrees"`
	SizeBytes int64    `json:"size_bytes"`
}

// SkippedRepo records why a source was not copied.
type SkippedRepo struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// Manifest is written to <root>/manifest.json.
type Manifest struct {
	Repos   []CorpusRepo  `json:"repos"`
	Skipped []SkippedRepo `json:"skipped"`
}

// RepoPaths returns the absolute paths of the copied repos, in manifest order.
func (m Manifest) RepoPaths() []string {
	out := make([]string, 0, len(m.Repos))
	for _, r := range m.Repos {
		out = append(out, r.Path)
	}
	return out
}

// BuildCorpus byte-copies each source into opts.Root and adds opts.Worktrees
// linked worktrees to every copy. Sources are only read.
func BuildCorpus(opts CorpusOptions) (Manifest, error) {
	var m Manifest
	logw := opts.Log
	if logw == nil {
		logw = io.Discard
	}
	skip := append([]string{alwaysSkip}, opts.SkipPaths...)
	for _, src := range opts.Sources {
		if opts.MaxRepos > 0 && len(m.Repos) >= opts.MaxRepos {
			break
		}
		name := filepath.Base(src)
		reason, size, err := checkSource(src, skip, opts.MaxRepoMB)
		if err != nil {
			return m, err
		}
		if reason != "" {
			fmt.Fprintf(logw, "skip %s: %s\n", src, reason)
			m.Skipped = append(m.Skipped, SkippedRepo{Source: src, Reason: reason})
			continue
		}
		dst := filepath.Join(opts.Root, reposDir, name)
		if _, err := os.Lstat(dst); err == nil {
			return m, fmt.Errorf("corpus repo %s already exists; remove it first", dst)
		}
		fmt.Fprintf(logw, "copy %s -> %s (%d MB)\n", src, dst, size>>20)
		if err := copyTree(src, dst, skip); err != nil {
			return m, fmt.Errorf("copy %s: %w", src, err)
		}
		repo := CorpusRepo{Name: name, Source: src, Path: dst, SizeBytes: size}
		for i := 1; i <= opts.Worktrees; i++ {
			wtDir := filepath.Join(opts.Root, worktreesDir, fmt.Sprintf("%s-w%d", name, i))
			if err := addLinkedWorktree(dst, filepath.Base(wtDir), fmt.Sprintf("ssq-workload-w%d", i), wtDir); err != nil {
				return m, fmt.Errorf("linked worktree %s: %w", wtDir, err)
			}
			repo.Worktrees = append(repo.Worktrees, wtDir)
		}
		m.Repos = append(m.Repos, repo)
	}
	if err := writeManifest(opts.Root, m); err != nil {
		return m, err
	}
	return m, nil
}

// checkSource returns a non-empty skip reason for sources that cannot be copied
// faithfully, plus the size the copy would have.
func checkSource(src string, skip []string, maxMB int64) (reason string, size int64, err error) {
	gitDir := filepath.Join(src, ".git")
	fi, statErr := os.Lstat(gitDir)
	switch {
	case errors.Is(statErr, fs.ErrNotExist):
		return "absent or not a git working tree", 0, nil
	case statErr != nil:
		return "", 0, statErr
	case !fi.IsDir():
		return ".git is a file (linked worktree or submodule), not a standalone clone", 0, nil
	}
	if _, aerr := os.Stat(filepath.Join(gitDir, "objects", "info", "alternates")); aerr == nil {
		return "uses objects/info/alternates (absolute path into another repo)", 0, nil
	}
	repo, oerr := git.PlainOpen(src)
	if oerr != nil {
		return "go-git cannot open: " + oerr.Error(), 0, nil
	}
	if _, herr := repo.Head(); herr != nil {
		return "no HEAD commit: " + herr.Error(), 0, nil
	}
	size, err = treeSize(src, skip)
	if err != nil {
		return "", 0, err
	}
	if maxMB > 0 && size>>20 > maxMB {
		return fmt.Sprintf("%d MB exceeds -max-repo-mb %d", size>>20, maxMB), size, nil
	}
	return "", size, nil
}

func skipped(rel string, skip []string) bool {
	rel = filepath.ToSlash(rel)
	for _, s := range skip {
		if rel == s || strings.HasPrefix(rel, s+"/") {
			return true
		}
		// A bare name (no slash) matches at any depth, e.g. node_modules.
		if !strings.Contains(s, "/") && filepath.Base(rel) == s {
			return true
		}
	}
	return false
}

// treeSize sums regular-file bytes under root, honouring skip.
func treeSize(root string, skip []string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if rel != "." && skipped(rel, skip) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// copyTree copies src to dst byte for byte (files, dirs, symlinks), honouring
// skip. dst must not exist.
func copyTree(src, dst string, skip []string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel != "." && skipped(rel, skip) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			return copyFile(p, target, info.Mode().Perm())
		default:
			return nil // sockets, fifos, devices
		}
	})
}

func copyFile(src, dst string, perm fs.FileMode) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm|0o600)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); err == nil {
			err = cerr
		}
	}()
	_, err = io.Copy(out, in)
	return err
}

// addLinkedWorktree creates a linked worktree of mainRepoDir without native
// git, mirroring session/unfinished/internal/testrepo.NewLinkedWorktree (which
// is internal to session/unfinished and so not importable here): hand-write
// <main>/.git/worktrees/<name>/{HEAD,commondir,gitdir} and the pointer file,
// create the branch at HEAD, then populate with go-git Reset(Hard).
func addLinkedWorktree(mainRepoDir, name, branch, linkedDir string) error {
	adminDir := filepath.Join(mainRepoDir, ".git", "worktrees", name)
	if err := os.MkdirAll(adminDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(linkedDir, 0o755); err != nil {
		return err
	}
	mainRepo, err := git.PlainOpen(mainRepoDir)
	if err != nil {
		return err
	}
	branchRef := plumbing.NewBranchReferenceName(branch)
	if _, err := mainRepo.Reference(branchRef, false); err != nil {
		head, herr := mainRepo.Head()
		if herr != nil {
			return herr
		}
		if err := mainRepo.Storer.SetReference(plumbing.NewHashReference(branchRef, head.Hash())); err != nil {
			return err
		}
	}
	files := map[string]string{
		filepath.Join(adminDir, "HEAD"):      "ref: " + branchRef.String() + "\n",
		filepath.Join(adminDir, "commondir"): "../..\n",
		filepath.Join(adminDir, "gitdir"):    filepath.Join(linkedDir, ".git") + "\n",
		filepath.Join(linkedDir, ".git"):     "gitdir: " + adminDir + "\n",
	}
	for p, c := range files {
		if err := os.WriteFile(p, []byte(c), 0o644); err != nil {
			return err
		}
	}
	repo, err := git.PlainOpenWithOptions(linkedDir, &git.PlainOpenOptions{EnableDotGitCommonDir: true})
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Reset(&git.ResetOptions{Mode: git.HardReset})
}

func writeManifest(root string, m Manifest) error {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, manifestName), b, 0o644)
}

// ReadManifest loads <root>/manifest.json.
func ReadManifest(root string) (Manifest, error) {
	var m Manifest
	b, err := os.ReadFile(filepath.Join(root, manifestName))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(b, &m)
}

// ResolveConfigDir mirrors config.GetConfigDirForDir for the two cases this
// tool needs, without creating anything: an explicit dir wins; otherwise
// STAPLER_SQUAD_INSTANCE semantics apply (~/.stapler-squad/instances/<name>,
// or ~/.stapler-squad for the special name "shared").
func ResolveConfigDir(configDir, instance string) (string, error) {
	if configDir != "" {
		return configDir, nil
	}
	if instance == "" {
		return "", errors.New("one of -config-dir or -instance is required")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := filepath.Join(home, ".stapler-squad")
	if instance == "shared" {
		return base, nil
	}
	return filepath.Join(base, "instances", instance), nil
}

// SeedState writes unfinished_state.json into configDir with the corpus repos
// pinned and auto-spider and watch dirs off, so the scanner sees only the
// corpus on first start. It goes through the production StateStore.
func SeedState(configDir string, pinned []string) error {
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return err
	}
	store, err := unfinished.NewStateStore(filepath.Join(configDir, stateFile))
	if err != nil {
		return err
	}
	return store.SetConfig(false, nil, pinned)
}

// PinnedRepos reads the pinned repos back through the production StateStore.
func PinnedRepos(configDir string) ([]string, error) {
	path := filepath.Join(configDir, stateFile)
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	store, err := unfinished.NewStateStore(path)
	if err != nil {
		return nil, err
	}
	return store.PinnedRepos(), nil
}

// relativeTo rebases each path onto root so lists from two instances (whose
// corpus roots differ) can be compared.
func relativeTo(root string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		rel, err := filepath.Rel(root, p)
		if err != nil || strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("pinned repo %s is outside corpus root %s", p, root)
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sort.Strings(out)
	return out, nil
}

// VerifyPinned asserts both config dirs pin the same repos, comparing paths
// relative to each instance's corpus root (instances cannot share absolute
// paths because each owns a private copy).
func VerifyPinned(configA, rootA, configB, rootB string) error {
	a, err := PinnedRepos(configA)
	if err != nil {
		return fmt.Errorf("instance A: %w", err)
	}
	b, err := PinnedRepos(configB)
	if err != nil {
		return fmt.Errorf("instance B: %w", err)
	}
	ra, err := relativeTo(rootA, a)
	if err != nil {
		return err
	}
	rb, err := relativeTo(rootB, b)
	if err != nil {
		return err
	}
	if len(ra) == 0 {
		return errors.New("instance A pins no repos")
	}
	if strings.Join(ra, "\n") != strings.Join(rb, "\n") {
		return fmt.Errorf("pinned repos differ:\n A: %v\n B: %v", ra, rb)
	}
	return nil
}

// DiscoverSources lists candidate owner repos under dir (immediate children
// with a .git directory), sorted, with preferFirst pinned to the front.
// Sibling clones named "<preferFirst>-*" are excluded.
func DiscoverSources(dir, preferFirst string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if preferFirst != "" && strings.HasPrefix(e.Name(), preferFirst+"-") {
			continue
		}
		if fi, err := os.Lstat(filepath.Join(dir, e.Name(), ".git")); err == nil && fi.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi := filepath.Base(out[i]) == preferFirst
		pj := filepath.Base(out[j]) == preferFirst
		if pi != pj {
			return pi
		}
		return out[i] < out[j]
	})
	return out
}
