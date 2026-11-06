package gitconfig

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Scope is the config scope git reports with --show-scope.
type Scope string

const (
	ScopeSystem   Scope = "system"
	ScopeGlobal   Scope = "global"
	ScopeLocal    Scope = "local"
	ScopeWorktree Scope = "worktree"
	ScopeCommand  Scope = "command"
)

// CommandOrigin is the origin git prints for GIT_CONFIG_COUNT entries.
const CommandOrigin = "command line:"

const (
	maxIncludeDepth = 10      // git's MAX_INCLUDE_DEPTH
	maxFileSize     = 4 << 20 // real config files are a few KiB
	maxHeadSize     = 4096
	maxFileReads    = 256  // distinct files one resolution may read
	maxExpansions   = 2048 // include expansions (a cached file included again counts): git's depth limit still allows an exponential fan-out
)

// Source is an Entry with where git read it from.
type Source struct {
	Entry
	Scope  Scope
	Origin string // the file path as git opened it, or CommandOrigin
}

// Env is everything outside the repository that selects config sources.
type Env struct {
	// Environ is the process environment the git CLI would be started with ("KEY=value").
	Environ []string
	// SystemDefault are git's compiled-in system config files, replaced by GIT_CONFIG_SYSTEM.
	// SystemVendor are build-added system files it does not replace. Both are learned once from
	// git itself by the composition root; this package never spawns a process.
	SystemDefault, SystemVendor []string
}

// Repo locates the repository. Both fields empty means "no repository": system, global and
// command scope only.
type Repo struct {
	// GitDir is the per-worktree git dir (<CommonDir>/worktrees/<name> in a linked worktree).
	GitDir string
	// CommonDir holds the shared config, hooks and refs.
	CommonDir string
}

// File is one file the resolution depended on. SHA256 is empty when the file did not exist, so
// that creating it later invalidates a cache key.
type File struct{ Path, SHA256 string }

// Result is the effective configuration, in the order `git config --list --show-origin
// --show-scope --includes` prints it (include directives included, expanded in place).
type Result struct {
	Entries []Source
	// Closure lists every file read or probed, in first-use order.
	Closure []File
	// GitdirCandidates are the git dir strings gitdir: conditions were matched against.
	GitdirCandidates []string
	// Head is the content of <GitDir>/HEAD when an onbranch: condition read it.
	Head string
}

// Resolve computes the configuration git would see. It fails closed (an *Error) wherever git
// itself dies or this package would otherwise have to guess; see the Sub constants.
func Resolve(ctx context.Context, env Env, repo Repo) (*Result, error) {
	r := &resolver{ctx: ctx, env: env, repo: repo, files: map[string]*fileState{}}
	if err := r.checkEnvironment(); err != nil {
		return nil, err
	}
	cands := r.gitdirCandidates()
	w, err := r.resolveFor(cands)
	if err != nil {
		return nil, err
	}
	return r.finish(w, cands), nil
}

func (r *resolver) finish(w *walk, cands []string) *Result {
	return &Result{Entries: w.entries, Closure: r.closure, GitdirCandidates: cands, Head: r.head}
}

type fileState struct {
	exists  bool
	entries []Entry
	err     error
}

type resolver struct {
	ctx     context.Context
	env     Env
	repo    Repo
	files   map[string]*fileState
	closure []File
	reads   int
	expands int
	head    string
	headSet bool
}

func (r *resolver) getenv(key string) (string, bool) {
	prefix := key + "="
	for i := len(r.env.Environ) - 1; i >= 0; i-- {
		if strings.HasPrefix(r.env.Environ[i], prefix) {
			return r.env.Environ[i][len(prefix):], true
		}
	}
	return "", false
}

// checkEnvironment rejects the variables the CLI honours and the in-process backends ignore.
func (r *resolver) checkEnvironment() error {
	for _, name := range []string{"GIT_DIR", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_WORK_TREE"} {
		if _, set := r.getenv(name); set {
			return &Error{Sub: SubEnvOverride, Msg: name + " is set"}
		}
	}
	if _, set := r.getenv("GIT_CONFIG_PARAMETERS"); set {
		return &Error{Sub: SubConfigParametersUsed, Msg: "GIT_CONFIG_PARAMETERS is set"}
	}
	return nil
}

// gitdirCandidates are the strings a gitdir: condition is matched against: the git dir as the
// caller wrote it, and (when different) with symlinks resolved.
func (r *resolver) gitdirCandidates() []string {
	if r.repo.GitDir == "" {
		return nil
	}
	logical := filepath.Clean(r.repo.GitDir)
	real, err := filepath.EvalSymlinks(logical)
	if err != nil || real == logical {
		return []string{logical}
	}
	return []string{logical, real}
}

// load reads and parses path once per Resolve. A missing file (or a missing parent directory,
// ENOTDIR) is exists=false, which git skips silently.
func (r *resolver) load(path string) *fileState {
	if st, ok := r.files[path]; ok {
		return st
	}
	st := &fileState{}
	r.files[path] = st
	if path == os.DevNull {
		st.exists = true // git reads it as an empty file; GIT_CONFIG_GLOBAL=/dev/null is the usual way to disable a scope
		return st
	}
	r.reads++
	if r.reads > maxFileReads {
		st.err = &Error{Sub: SubIncludeDepth, Path: path, Msg: "too many include reads"}
		return st
	}
	data, err := ReadRegularFile(path, maxFileSize)
	switch {
	case err != nil && (errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR)):
		r.closure = append(r.closure, File{Path: path})
		return st
	case errors.Is(err, ErrTooLarge):
		st.exists, st.err = true, &Error{Sub: SubTooLarge, Path: path}
		return st
	case errors.Is(err, ErrNotRegular):
		st.exists, st.err = true, &Error{Sub: SubUnreadable, Path: path, Msg: "not a regular file"}
		return st
	case err != nil:
		st.exists, st.err = true, unreadable(path, err)
		return st
	}
	sum := sha256.Sum256(data)
	r.closure = append(r.closure, File{Path: path, SHA256: hex.EncodeToString(sum[:])})
	st.exists = true
	st.entries, err = Parse(data)
	if err != nil {
		var ce *Error
		if errors.As(err, &ce) {
			ce.Path = path
		}
		st.err = err
	}
	return st
}

func unreadable(path string, err error) *Error {
	return &Error{Sub: SubUnreadable, Path: path, Err: err}
}

// walk is one full pass over the scope sequence for one gitdir candidate.
type walk struct {
	r       *resolver
	gitdirs []string // the logical git dir, then the symlink-resolved one when it differs
	entries []Source
	// collectURLs marks pass A of hasconfig: every hasconfig condition is true and remote URLs
	// are gathered instead of the entries being kept.
	collectURLs bool
	urls        []string
	// urlsReady/urlList cache pass A's result for the main pass.
	urlsReady bool
	urlList   []string
}

func (r *resolver) resolveFor(gitdirs []string) (*walk, error) {
	w := &walk{r: r, gitdirs: gitdirs}
	if err := w.sequence(); err != nil {
		return nil, err
	}
	return w, nil
}

// sequence reads every scope in git's order: system, XDG, ~/.gitconfig, repository,
// per-worktree, then command scope.
func (w *walk) sequence() error {
	r := w.r
	noSystem, ok := envBool(r.env.Environ, "GIT_CONFIG_NOSYSTEM")
	if !ok {
		return &Error{Sub: SubEnvOverride, Msg: "GIT_CONFIG_NOSYSTEM is not a boolean"}
	}
	if !noSystem {
		// Vendor files come first in git's order, so the system file wins a key set in both.
		for _, p := range r.env.SystemVendor {
			if err := w.top(p, ScopeSystem, false); err != nil {
				return err
			}
		}
		system := r.env.SystemDefault
		if v, set := r.getenv("GIT_CONFIG_SYSTEM"); set {
			system = []string{v}
		}
		for _, p := range system {
			if err := w.top(p, ScopeSystem, false); err != nil {
				return err
			}
		}
	}
	if err := w.globalScope(); err != nil {
		return err
	}
	if err := w.repoScopes(); err != nil {
		return err
	}
	return w.commandScope()
}

func (w *walk) globalScope() error {
	r := w.r
	if v, set := r.getenv("GIT_CONFIG_GLOBAL"); set {
		return w.top(v, ScopeGlobal, true)
	}
	home, err := r.home()
	xdg, _ := r.getenv("XDG_CONFIG_HOME")
	switch {
	case xdg != "":
		if e := w.top(filepath.Join(xdg, "git", "config"), ScopeGlobal, true); e != nil {
			return e
		}
	case err == nil:
		if e := w.top(filepath.Join(home, ".config", "git", "config"), ScopeGlobal, true); e != nil {
			return e
		}
	}
	if err != nil {
		return err
	}
	return w.top(filepath.Join(home, ".gitconfig"), ScopeGlobal, true)
}

func (r *resolver) home() (string, error) {
	if h, _ := r.getenv("HOME"); h != "" {
		return h, nil
	}
	return "", &Error{Sub: SubHomeUnknown, Msg: "HOME is not set"}
}

func (w *walk) repoScopes() error {
	r := w.r
	if r.repo.CommonDir == "" {
		return nil
	}
	local := filepath.Join(r.repo.CommonDir, "config")
	if err := w.top(local, ScopeLocal, false); err != nil {
		return err
	}
	if r.repo.GitDir == "" || !r.worktreeConfigEnabled(local) {
		return nil
	}
	return w.top(filepath.Join(r.repo.GitDir, "config.worktree"), ScopeWorktree, false)
}

// worktreeConfigEnabled reads extensions.worktreeConfig the way git does at repository setup:
// from the repository config file alone, without following includes.
func (r *resolver) worktreeConfigEnabled(local string) bool {
	st := r.load(local)
	enabled := false
	for _, e := range st.entries {
		if e.Section == "extensions" && !e.HasSubsection && e.Key == "worktreeconfig" {
			enabled = boolOrTrue(e)
		}
	}
	return enabled
}

func boolOrTrue(e Entry) bool {
	b, ok := ParseBool(e.Value, e.HasValue)
	return !ok || b
}

// top reads a top-level file. eaccesOK mirrors git's ACCESS_EACCES_OK: global files that
// exist but cannot be read are skipped, every other scope dies.
func (w *walk) top(path string, scope Scope, eaccesOK bool) error {
	if path == "" {
		return nil
	}
	st := w.r.load(path)
	if st.err != nil {
		var ce *Error
		if eaccesOK && errors.As(st.err, &ce) && ce.Sub == SubUnreadable && errors.Is(ce.Err, fs.ErrPermission) {
			return nil
		}
		return st.err
	}
	if !st.exists {
		return nil
	}
	return w.process(path, st.entries, scope, 0, false)
}

// commandScope reads GIT_CONFIG_COUNT / GIT_CONFIG_KEY_<n> / GIT_CONFIG_VALUE_<n>.
func (w *walk) commandScope() error {
	r := w.r
	raw, set := r.getenv("GIT_CONFIG_COUNT")
	if !set {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return &Error{Sub: SubEnvOverride, Msg: "GIT_CONFIG_COUNT is not a non-negative integer"}
	}
	var entries []Entry
	for i := range n {
		key, kset := r.getenv("GIT_CONFIG_KEY_" + strconv.Itoa(i))
		val, vset := r.getenv("GIT_CONFIG_VALUE_" + strconv.Itoa(i))
		if !kset || !vset {
			return &Error{Sub: SubEnvOverride, Msg: "GIT_CONFIG_KEY_/VALUE_ pair is incomplete"}
		}
		section, sub, hasSub, name, ok := ParseKey(key)
		if !ok {
			return &Error{Sub: SubEnvOverride, Msg: "GIT_CONFIG_KEY_" + strconv.Itoa(i) + " is not a valid key"}
		}
		entries = append(entries, Entry{Section: section, Subsection: sub, HasSubsection: hasSub, Key: name, Value: val, HasValue: true})
	}
	return w.process(CommandOrigin, entries, ScopeCommand, 0, false)
}

// process appends entries in order, expanding include directives at the position they are read.
// viaCond is true when this file was reached through an includeIf.
func (w *walk) process(origin string, entries []Entry, scope Scope, depth int, viaCond bool) error {
	for _, e := range entries {
		if err := w.r.ctx.Err(); err != nil {
			return &Error{Sub: SubCanceled, Err: err}
		}
		if err := w.noteRemoteURL(e, viaCond); err != nil {
			return err
		}
		if !w.collectURLs {
			w.entries = append(w.entries, Source{Entry: e, Scope: scope, Origin: origin})
		}
		if err := w.include(origin, e, scope, depth, viaCond); err != nil {
			return err
		}
	}
	return nil
}

// noteRemoteURL implements pass A's bookkeeping for remote.<name>.url.
func (w *walk) noteRemoteURL(e Entry, viaCond bool) error {
	if !w.collectURLs || e.Section != "remote" || !e.HasSubsection || e.Key != "url" {
		return nil
	}
	if viaCond {
		return &Error{Sub: SubRemoteURLInInclude, Msg: "remote.<name>.url inside an includeIf file"}
	}
	if !e.HasValue {
		return &Error{Sub: SubBareRemoteURL, Msg: "remote.<name>.url without a value"}
	}
	w.urls = append(w.urls, e.Value)
	return nil
}

func (w *walk) include(origin string, e Entry, scope Scope, depth int, viaCond bool) error {
	switch {
	case e.Section == "include" && !e.HasSubsection && e.Key == "path":
		return w.followInclude(origin, e, scope, depth, viaCond)
	case e.Section == "includeif" && e.HasSubsection && e.Key == "path":
		ok, err := w.condition(origin, e.Subsection)
		if err != nil || !ok {
			return err
		}
		return w.followInclude(origin, e, scope, depth, true)
	}
	return nil
}

func (w *walk) followInclude(origin string, e Entry, scope Scope, depth int, viaCond bool) error {
	if !e.HasValue {
		return &Error{Sub: SubBadInclude, Msg: "include path without a value"}
	}
	if depth+1 > maxIncludeDepth {
		return &Error{Sub: SubIncludeDepth, Path: e.Value, Msg: "exceeded maximum include depth"}
	}
	path, err := w.includePath(origin, e.Value)
	if err != nil {
		return err
	}
	if w.r.expands++; w.r.expands > maxExpansions {
		return &Error{Sub: SubIncludeDepth, Path: path, Msg: "too many include expansions"}
	}
	st := w.r.load(path)
	if st.err != nil {
		return st.err
	}
	if !st.exists {
		return nil
	}
	return w.process(path, st.entries, scope, depth+1, viaCond)
}

// includePath expands an include path the way handle_path_include does: `~/` is the home
// directory, a relative path is relative to the including file, anything else is refused.
func (w *walk) includePath(origin, value string) (string, error) {
	switch {
	case value == "":
		return "", &Error{Sub: SubBadInclude, Msg: "empty include path"}
	case strings.HasPrefix(value, "~/"):
		home, err := w.r.home()
		if err != nil {
			return "", err
		}
		return home + value[1:], nil
	case strings.HasPrefix(value, "~") || strings.HasPrefix(value, "%("):
		return "", &Error{Sub: SubUnsupportedInclude, Path: value, Msg: "~user and %(prefix) include paths are not evaluated"}
	case filepath.IsAbs(value):
		return value, nil
	case origin == CommandOrigin:
		return "", &Error{Sub: SubUnsupportedInclude, Path: value, Msg: "relative include path in command scope"}
	}
	// git concatenates the including file's directory and the path without cleaning it.
	return origin[:strings.LastIndexByte(origin, '/')+1] + value, nil
}

// condition evaluates includeIf.<cond>.path.
func (w *walk) condition(origin, cond string) (bool, error) {
	switch {
	case strings.HasPrefix(cond, "gitdir/i:"):
		return w.gitdirCondition(origin, cond[len("gitdir/i:"):], true)
	case strings.HasPrefix(cond, "gitdir:"):
		return w.gitdirCondition(origin, cond[len("gitdir:"):], false)
	case strings.HasPrefix(cond, "onbranch:"):
		return w.onbranchCondition(cond[len("onbranch:"):])
	case strings.HasPrefix(cond, "hasconfig:remote.*.url:"):
		return w.hasconfigCondition(cond[len("hasconfig:remote.*.url:"):])
	case strings.HasPrefix(cond, "hasconfig:"):
		return false, nil // git ignores any other hasconfig: key
	}
	return false, &Error{Sub: SubUnknownCondition, Msg: "includeIf condition is not gitdir:, gitdir/i:, onbranch: or hasconfig:"}
}

// gitdirCondition matches the pattern against the git dir. git tries the path as the process
// was launched (logical when $PWD is a symlinked path) and then the symlink-resolved one, so a
// pattern written with the real path always matches, while one written through a symlink
// matches only when git was started from the symlinked path. The latter depends on how the CLI
// is launched, so it fails closed; the real-path answer is otherwise the answer.
func (w *walk) gitdirCondition(origin, pattern string, fold bool) (bool, error) {
	if len(w.gitdirs) == 0 || w.gitdirs[0] == "" {
		return false, nil
	}
	pattern, prefix, err := w.gitdirPattern(origin, pattern)
	if err != nil {
		return false, err
	}
	logical, err := matchGitdir(w.gitdirs[0], pattern, prefix, fold)
	if err != nil || len(w.gitdirs) == 1 {
		return logical, err
	}
	real, err := matchGitdir(w.gitdirs[1], pattern, prefix, fold)
	if err != nil {
		return false, err
	}
	if logical && !real {
		return false, &Error{Sub: SubGitdirSymlink, Msg: "gitdir: pattern matches only through a symlink"}
	}
	return real, nil
}

func matchGitdir(text, pattern string, prefix int, fold bool) (bool, error) {
	if prefix > 0 {
		if len(text) < prefix || !equalFold(text[:prefix], pattern[:prefix], fold) {
			return false, nil
		}
	}
	matched, ok := Match(pattern[prefix:], text[prefix:], fold)
	if !ok {
		return false, &Error{Sub: SubUnsupportedGlob, Msg: "gitdir: pattern uses [ or \\"}
	}
	return matched, nil
}

func equalFold(a, b string, fold bool) bool {
	if fold {
		return asciiLower(a) == asciiLower(b)
	}
	return a == b
}

// gitdirPattern is prepare_include_condition_pattern: `~/` expansion, `./` relative to the
// including file's real directory (prefix is that literal part), otherwise a relative pattern
// gets "**/" in front, and a trailing slash gets "**" appended.
func (w *walk) gitdirPattern(origin, pattern string) (string, int, error) {
	prefix := 0
	switch {
	case strings.HasPrefix(pattern, "~/"):
		home, err := w.r.home()
		if err != nil {
			return "", 0, err
		}
		pattern = home + pattern[1:]
	case strings.HasPrefix(pattern, "~"):
		return "", 0, &Error{Sub: SubUnsupportedInclude, Msg: "~user in a gitdir: pattern"}
	}
	switch {
	case strings.HasPrefix(pattern, "./"):
		if origin == CommandOrigin {
			return "", 0, &Error{Sub: SubUnsupportedInclude, Msg: "relative includeIf pattern in command scope"}
		}
		real, err := filepath.EvalSymlinks(origin)
		if err != nil {
			return "", 0, unreadable(origin, err)
		}
		dir := filepath.Dir(real)
		pattern = dir + pattern[1:]
		prefix = len(dir) + 1
	case !filepath.IsAbs(pattern):
		pattern = "**/" + pattern
	}
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	return pattern, prefix, nil
}

func (w *walk) onbranchCondition(pattern string) (bool, error) {
	if w.r.repo.GitDir == "" {
		return false, nil
	}
	branch := w.r.currentBranch()
	if branch == "" {
		return false, nil
	}
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	matched, ok := Match(pattern, branch, false)
	if !ok {
		return false, &Error{Sub: SubUnsupportedGlob, Msg: "onbranch: pattern uses [ or \\"}
	}
	return matched, nil
}

// currentBranch reads <GitDir>/HEAD once; a detached or unreadable HEAD is no branch.
func (r *resolver) currentBranch() string {
	if !r.headSet {
		r.headSet = true
		if data, err := ReadRegularFile(filepath.Join(r.repo.GitDir, "HEAD"), maxHeadSize); err == nil {
			r.head = string(data)
		}
	}
	line := strings.TrimRight(r.head, "\r\n")
	ref, ok := strings.CutPrefix(line, "ref: ")
	if !ok {
		return ""
	}
	branch, ok := strings.CutPrefix(strings.TrimSpace(ref), "refs/heads/")
	if !ok {
		return ""
	}
	return branch
}

// hasconfigCondition is pass B: true iff any remote URL gathered by pass A matches the glob.
func (w *walk) hasconfigCondition(glob string) (bool, error) {
	if w.collectURLs {
		return true, nil // pass A: every hasconfig condition is true, the glob is never read
	}
	if strings.ContainsAny(glob, "[\\") {
		return false, &Error{Sub: SubUnsupportedGlob, Msg: "hasconfig: pattern uses [ or \\"}
	}
	if !w.urlsReady {
		pass := &walk{r: w.r, gitdirs: w.gitdirs, collectURLs: true}
		if err := pass.sequence(); err != nil {
			return false, err
		}
		w.urlsReady, w.urlList = true, pass.urls
	}
	for _, u := range w.urlList {
		if matched, _ := Match(glob, u, false); matched {
			return true, nil
		}
	}
	return false, nil
}
