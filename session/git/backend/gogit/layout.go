package gogit

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// repoRedirectVars are the environment variables that make git look at a repository, object
// store, history or config other than the one the directory implies. go-git ignores them all, so
// when one is set the CLI must answer.
var repoRedirectVars = []string{ //nolint:gochecknoglobals read-only list
	"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM",
	"GIT_NAMESPACE", "GIT_REPLACE_REF_BASE", "GIT_NO_REPLACE_OBJECTS", "GIT_SHALLOW_FILE",
	"GIT_GRAFT_FILE", "GIT_INDEX_FILE", "GIT_PREFIX", "GIT_CONFIG", "GIT_CONFIG_COUNT",
	"GIT_CONFIG_PARAMETERS",
}

// layout is where a directory's repository lives on disk, resolved to real paths (git prints
// symlink-free paths).
type layout struct {
	workTree  string // "" when the directory is inside a git dir with no work tree
	gitDir    string // per-worktree admin dir; differs from commonDir in a linked worktree
	commonDir string
	bare      bool // the directory was discovered as a git dir itself
}

// localDir extracts the directory of a Local location, mirroring the CLI backend's argument
// checks. Remote locations are never served in-process.
func localDir(loc backend.RepoLocation) (string, error) {
	switch l := loc.(type) {
	case backend.Local:
		if l.Root == "" {
			return "", fmt.Errorf("%w: empty local root", backend.ErrInvalidArgument)
		}
		return string(l.Root), nil
	case backend.Remote:
		return "", fallbackEligible(errors.New("remote location is not readable in-process"))
	default:
		return "", fmt.Errorf("%w: unsupported location %T", backend.ErrInvalidArgument, loc)
	}
}

// locate finds the repository containing dir the way `git rev-parse` does (walk up from the real
// path, stopping at a device boundary). It returns backend.ErrNotARepo only where git would
// report "not a git repository"; anything it cannot decide exactly is fallback-eligible.
func (b *Backend) locate(ctx context.Context, dir string, environ []string) (layout, error) {
	for _, v := range repoRedirectVars {
		if _, set := envValue(environ, v); set {
			return layout{}, fallbackEligible(fmt.Errorf("%s is set; git would honour it and go-git does not", v))
		}
	}
	if _, ok := envBool(environ, "GIT_CONFIG_NOSYSTEM"); !ok {
		return layout{}, fallbackEligible(errors.New("GIT_CONFIG_NOSYSTEM is not a boolean; git fails every command"))
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return layout{}, fallbackEligible(err)
	}
	start, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return layout{}, fallbackEligible(err) // missing directory: the CLI reports its own error
	}
	fi, err := os.Stat(start)
	if err != nil || !fi.IsDir() {
		return layout{}, fallbackEligible(fmt.Errorf("%s is not a directory", start))
	}
	startDev, _, ok := fileIDs(fi)
	if !ok {
		return layout{}, fallbackEligible(errors.New("file ownership is not available on this platform"))
	}
	for cur := start; ; {
		lay, found, err := probeDir(cur)
		if err != nil {
			return layout{}, err
		}
		if found {
			return b.vetLayout(ctx, lay, environ)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return layout{}, fmt.Errorf("%w: %s", backend.ErrNotARepo, dir)
		}
		pfi, err := os.Stat(parent)
		if err != nil {
			return layout{}, fallbackEligible(err)
		}
		if pdev, _, _ := fileIDs(pfi); pdev != startDev {
			return layout{}, fmt.Errorf("%w: %s (stopped at a filesystem boundary)", backend.ErrNotARepo, dir)
		}
		cur = parent
	}
}

// probeDir applies git's per-directory discovery step: a .git file, a .git directory, then the
// directory itself as a bare git dir.
func probeDir(cur string) (lay layout, found bool, err error) {
	dotGit := filepath.Join(cur, ".git")
	switch fi, statErr := os.Stat(dotGit); {
	case statErr == nil && fi.IsDir():
		common, ok, err := commonOf(dotGit)
		if err != nil {
			return layout{}, false, err
		}
		if ok {
			return layoutFor(cur, dotGit, common, false)
		}
	case statErr == nil && fi.Mode().IsRegular():
		return probeGitfile(cur, dotGit)
	case statErr != nil && !errors.Is(statErr, fs.ErrNotExist):
		return layout{}, false, fallbackEligible(statErr)
	}
	common, ok, err := commonOf(cur)
	if err != nil {
		return layout{}, false, err
	}
	if ok {
		return layoutFor("", cur, common, true)
	}
	return layout{}, false, nil
}

// errDanglingGitfile marks the ErrNotARepo git reports when a ".git" file names a directory that
// is not a git directory. Unlike the walk running out of parents, git stops here and every
// command fails, `git config` included, so callers must not treat it as "outside any repository".
var errDanglingGitfile = errors.New("a .git file names a directory that is not a git directory")

// probeGitfile follows the ".git" file in cur. git never searches parent directories once it
// has found one: a target that is not a git directory is "not a git repository" for the whole
// call, so that verdict is final (and only that verdict: a file git cannot parse is a different
// fatal error that this reader leaves to the CLI).
func probeGitfile(cur, dotGit string) (layout, bool, error) {
	target, err := readGitfile(dotGit)
	if err != nil {
		return layout{}, false, fallbackEligible(err)
	}
	notARepo := func() (layout, bool, error) {
		return layout{}, false, fmt.Errorf("%w: %s: %w", backend.ErrNotARepo, target, errDanglingGitfile)
	}
	// Resolve through the OS, not lexically: "link/../x" is not "x" when link is a symlink.
	gitDir, err := filepath.EvalSymlinks(target)
	switch {
	case errors.Is(err, fs.ErrNotExist) || isNotDir(err):
		return notARepo()
	case err != nil:
		return layout{}, false, fallbackEligible(err)
	}
	common, ok, err := commonOf(gitDir)
	if err != nil {
		return layout{}, false, err
	}
	if !ok {
		return notARepo()
	}
	return layoutFor(cur, gitDir, common, false)
}

func layoutFor(workTree, gitDir, common string, bare bool) (layout, bool, error) {
	real := func(p string) (string, error) {
		if p == "" {
			return "", nil
		}
		return filepath.EvalSymlinks(p)
	}
	var lay layout
	var err error
	if lay.workTree, err = real(workTree); err != nil {
		return layout{}, false, fallbackEligible(err)
	}
	if lay.gitDir, err = real(gitDir); err != nil {
		return layout{}, false, fallbackEligible(err)
	}
	if lay.commonDir, err = real(common); err != nil {
		return layout{}, false, fallbackEligible(err)
	}
	lay.bare = bare
	return lay, true, nil
}

// commonOf reports whether p is a git directory (git's is_git_directory) and returns its common
// directory: objects/ and refs/ live there, HEAD lives in p. A HEAD that is missing or that git
// would reject means "not a git directory" (git keeps looking upward); a HEAD this reader cannot
// judge exactly (a symlink, oversized, unreadable) is a fallback error, never "not a repository".
func commonOf(p string) (common string, ok bool, err error) {
	head, err := readHeadFile(filepath.Join(p, "HEAD"))
	switch {
	case errors.Is(err, fs.ErrNotExist) || isNotDir(err):
		return "", false, nil
	case err != nil:
		return "", false, fallbackEligible(err)
	case !validateHeadref(head):
		return "", false, nil
	}
	common = p
	data, err := os.ReadFile(filepath.Join(p, "commondir"))
	switch {
	case errors.Is(err, fs.ErrNotExist) || isNotDir(err):
	case err != nil || len(strings.TrimSpace(string(data))) == 0:
		// git dies ("failed to read commondir") rather than ignoring a commondir it cannot use.
		return "", false, fallbackEligible(fmt.Errorf("%s/commondir is unreadable or empty", p))
	default:
		c := strings.TrimRight(string(data), "\r\n")
		if !filepath.IsAbs(c) {
			c = filepath.Join(p, c)
		}
		common = filepath.Clean(c)
	}
	for _, sub := range []string{"objects", "refs"} {
		if fi, err := os.Stat(filepath.Join(common, sub)); err != nil || !fi.IsDir() {
			return "", false, nil
		}
	}
	return common, true, nil
}

// readHeadFile reads a HEAD file. git resolves a symlinked HEAD itself (a legacy symref form),
// which this reader does not model, so a symlink is an error rather than being read through.
func readHeadFile(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s is a symlink", path)
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%s is a directory: %w", path, fs.ErrNotExist)
	}
	return readSmallFile(path)
}

// readGitfile parses a ".git" file the way git's read_gitfile_gently does: "gitdir: <path>",
// trailing CR/LF dropped (spaces, tabs and an embedded newline stay part of the path), a relative path
// taken from the file's directory. The path is returned unresolved. A file git would reject as
// malformed is an error.
func readGitfile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	const prefix = "gitdir: "
	if len(data) > maxGitfileSize || !strings.HasPrefix(string(data), prefix) {
		return "", fmt.Errorf("%s is not a valid gitfile", path)
	}
	trimmed := strings.TrimRight(string(data), "\r\n")
	if len(trimmed) <= len(prefix) {
		return "", fmt.Errorf("%s names no path", path)
	}
	target := trimmed[len(prefix):]
	if !filepath.IsAbs(target) {
		target = filepath.Dir(path) + string(filepath.Separator) + target
	}
	return target, nil
}

// maxGitfileSize is git's limit (PATH_MAX * 4) for a ".git" file, at the smallest PATH_MAX in
// use; bigger files are left to git.
const maxGitfileSize = 4096

// vetLayout rejects repositories whose semantics go-git would get wrong: other owners (git
// refuses them as "dubious ownership" unless safe.directory says otherwise), work trees set by
// config, reftable or SHA-256 storage, partial clones and other extensions.
func (b *Backend) vetLayout(ctx context.Context, lay layout, environ []string) (layout, error) {
	uid := b.uid()
	for _, p := range []string{lay.workTree, lay.gitDir, lay.commonDir} {
		if p == "" {
			continue
		}
		fi, err := os.Stat(p)
		if err != nil {
			return layout{}, fallbackEligible(err)
		}
		if _, owner, ok := fileIDs(fi); !ok || int(owner) != uid {
			return layout{}, fallbackEligible(fmt.Errorf("%s is not owned by the current user (safe.directory applies)", p))
		}
	}
	if err := vetRepoConfig(ctx, lay, environ); err != nil {
		return layout{}, err
	}
	return lay, nil
}

func repoSettingMatcher(section, _, name string) bool {
	switch {
	case strings.EqualFold(section, "extensions"):
		return true
	case strings.EqualFold(section, "core"):
		return strings.EqualFold(name, "worktree") || strings.EqualFold(name, "bare") ||
			strings.EqualFold(name, "repositoryformatversion")
	}
	return false
}

func vetRepoConfig(ctx context.Context, lay layout, environ []string) error {
	home, _ := envValue(environ, "HOME")
	l := newConfigLoader(ctx, repoSettingMatcher, home)
	l.load(filepath.Join(lay.commonDir, "config"), 1)
	if l.res.unsure {
		return fallbackEligible(errors.New("repository config cannot be evaluated in-process"))
	}
	version := "0"
	for _, e := range l.res.entries {
		if strings.EqualFold(e.section, "core") && strings.EqualFold(e.name, "repositoryformatversion") {
			version = e.value
		}
	}
	for _, e := range l.res.entries {
		name := strings.ToLower(e.name)
		switch strings.ToLower(e.section) {
		case "core":
			if err := vetCoreSetting(lay, name, e.value); err != nil {
				return err
			}
		case "extensions":
			if version != "1" {
				return fallbackEligible(fmt.Errorf("extensions.%s in a repositoryformatversion %s repository: git rejects it", name, version))
			}
			if err := vetExtension(name, e.value); err != nil {
				return err
			}
		}
	}
	return nil
}

func vetCoreSetting(lay layout, name, value string) error {
	switch name {
	case "worktree":
		// Submodules carry core.worktree pointing back at the directory that holds their .git
		// file; any other value moves the work tree somewhere this scan did not look.
		target := value
		if !filepath.IsAbs(target) {
			target = filepath.Join(lay.gitDir, target)
		}
		if real, err := filepath.EvalSymlinks(target); err != nil || real != lay.workTree {
			return fallbackEligible(errors.New("core.worktree names a different work tree"))
		}
	case "bare":
		if isTruthy(value) && !lay.bare {
			return fallbackEligible(errors.New("core.bare is true for a repository with a work tree"))
		}
	case "repositoryformatversion":
		if value != "0" && value != "1" {
			return fallbackEligible(fmt.Errorf("unsupported core.repositoryformatversion %q", value))
		}
	}
	return nil
}

func vetExtension(name, value string) error {
	switch name {
	case "noop":
		return nil
	case "objectformat":
		if strings.EqualFold(value, "sha1") {
			return nil
		}
	case "worktreeconfig":
		if !isTruthy(value) {
			return nil
		}
	}
	return fallbackEligible(fmt.Errorf("repository extension %q is not supported in-process", name))
}

func isTruthy(v string) bool {
	switch strings.ToLower(v) {
	case "", "false", "no", "off", "0":
		return false
	}
	return true
}

// historyRewritten reports whether git would read a different parent graph than go-git: a
// shallow clone's cut-off parents or grafts. Walks that follow parents must fall back when it is
// true (replace refs are checked separately, for every resolution).
func historyRewritten(lay layout) (bool, error) {
	for _, p := range []string{
		filepath.Join(lay.commonDir, "shallow"),
		filepath.Join(lay.commonDir, "info", "grafts"),
	} {
		if _, err := os.Lstat(p); err == nil {
			return true, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
