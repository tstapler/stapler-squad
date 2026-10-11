// Package gitoracle is the differential test harness for git backends (plan Story 1.3.3). It
// runs one operation on the CLI backend, which is the oracle, and on a candidate Backend over
// temp repositories built with real git, then reports every difference. Later cohorts (gogit
// refs, diffstatus, localwrite, worktree) write their parity tests with RunBoth/AssertParity
// instead of building their own comparison code.
//
// Fixtures are deterministic: fixed author and commit dates make the same history produce the
// same SHAs in every repository, so results compare without a SHA translation table. Absolute
// paths are replaced by stable tokens before comparing.
package gitoracle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/cli"
)

// Fixed so equal histories hash equally across the oracle and candidate repositories.
const fixedDate = "2020-01-01T00:00:00Z"

// Runner runs real git with a hermetic environment: no inherited GIT_* variables, no user or
// system configuration, fixed identity and dates. It satisfies backend.Runner and
// backend.StdoutRunner, so the CLI backend uses its stdout-only path.
type Runner struct{ Home string }

var (
	_ backend.Runner       = Runner{}
	_ backend.StdoutRunner = Runner{}
)

// Env returns the hermetic environment for git processes rooted at home.
func Env(home string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "GIT_") {
			env = append(env, kv)
		}
	}
	env = append(env, backend.RequiredGitEnv()...)
	return append(env,
		"HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "xdg"),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@example.com",
		"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@example.com",
		"GIT_AUTHOR_DATE="+fixedDate, "GIT_COMMITTER_DATE="+fixedDate,
	)
}

// Run implements backend.Runner (combined stdout and stderr).
func (r Runner) Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, Env(r.Home)
	return cmd.CombinedOutput()
}

// RunStdout implements backend.StdoutRunner; a failure's *exec.ExitError carries stderr.
func (r Runner) RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	cmd := safeexec.CommandContext(ctx, name, args...)
	cmd.Dir, cmd.Env = dir, Env(r.Home)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		ee.Stderr = stderr.Bytes()
	}
	return stdout.Bytes(), err
}

// NewOracle returns the oracle backend: the CLI backend over real git. It is used without a
// Router, so its spawns count as unattributed in git_backend_cli_spawn_total.
func NewOracle(home string) backend.Backend { return cli.New(Runner{Home: home}) }

// RequireGit skips the test when git is not installed.
func RequireGit(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

// Git runs real git for fixture setup and fails the test on error. It returns trimmed output.
type Git struct {
	T    testing.TB
	Home string
}

// Run executes git in dir.
func (g Git) Run(dir string, args ...string) string {
	g.T.Helper()
	out, err := Runner{Home: g.Home}.RunStdout(context.Background(), dir, "git", args...)
	if err != nil {
		var ee *exec.ExitError
		stderr := ""
		if errors.As(err, &ee) {
			stderr = string(ee.Stderr)
		}
		g.T.Fatalf("git %q in %s: %v\n%s", args, dir, err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// Write creates path (relative to dir, parents included) with content.
func (g Git) Write(dir, path, content string) {
	g.T.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		g.T.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		g.T.Fatal(err)
	}
}

// Commit stages everything and commits.
func (g Git) Commit(dir, message string) {
	g.T.Helper()
	g.Run(dir, "add", "-A")
	g.Run(dir, "commit", "-q", "-m", message)
}

// Places are the named directories an operation runs in: "main" always, plus any linked
// worktree a fixture creates ("linked", "detached").
type Places map[string]string

// Fixture builds one repository shape with real git. Build receives a fresh empty directory
// and returns the places to run operations in. It must be deterministic.
type Fixture struct {
	Name  string
	Build func(g Git, root string) Places
}

// Op is one backend operation under test. Run returns a value that renders to comparable
// JSON (typed Backend results do) and the backend's error.
type Op struct {
	Name string
	// Mutating operations get a freshly built repository per side and have the resulting
	// repository state compared too; read operations share one repository.
	Mutating bool
	// Places limits the op to the named places (see Fixture); empty means every place.
	Places []string
	Run    func(ctx context.Context, b backend.Backend, loc backend.Local) (any, error)
}

func (o Op) runsIn(place string) bool {
	if len(o.Places) == 0 {
		return true
	}
	for _, p := range o.Places {
		if p == place {
			return true
		}
	}
	return false
}

// DiffKind says what differed.
type DiffKind string

const (
	DiffResult DiffKind = "result" // the returned value differs
	DiffError  DiffKind = "error"  // one side failed, or they failed with different classes
	DiffState  DiffKind = "state"  // the repository differs after a mutating operation
	DiffFsck   DiffKind = "fsck"   // git fsck --strict rejects the candidate's repository
)

// Diff is one disagreement between the oracle and the candidate.
type Diff struct {
	Fixture, Place, Op string
	Kind               DiffKind
	Oracle, Candidate  string
}

func (d Diff) String() string {
	return fmt.Sprintf("%s/%s %s [%s]\n  oracle:    %s\n  candidate: %s", d.Fixture, d.Place, d.Op, d.Kind, d.Oracle, d.Candidate)
}

// RunBoth runs op in every place of every fixture on the oracle and on cand, and returns every
// difference found (nil means parity).
func RunBoth(t testing.TB, cand backend.Backend, op Op, fixtures []Fixture) []Diff {
	t.Helper()
	return RunOps(t, cand, []Op{op}, fixtures)
}

// AssertParity fails the test with a report of every difference across ops.
func AssertParity(t testing.TB, cand backend.Backend, ops []Op, fixtures []Fixture) {
	t.Helper()
	diffs := RunOps(t, cand, ops, fixtures)
	if len(diffs) == 0 {
		return
	}
	report := make([]string, len(diffs))
	for i, d := range diffs {
		report[i] = d.String()
	}
	t.Errorf("%d difference(s) from the git CLI oracle:\n%s", len(diffs), strings.Join(report, "\n"))
}

// RunOps runs every op over every fixture and returns all differences. Read operations share
// one repository per fixture, so the real-git cost is one build per fixture, not one per op.
func RunOps(t testing.TB, cand backend.Backend, ops []Op, fixtures []Fixture) []Diff {
	t.Helper()
	RequireGit(t)
	home := t.TempDir()
	g := Git{T: t, Home: home}
	oracle := NewOracle(home)
	var diffs []Diff
	for _, fx := range fixtures {
		shared := build(t, g, fx) // read operations never change it, so both sides share it
		for _, op := range ops {
			for _, place := range sortedKeys(shared) {
				if !op.runsIn(place) {
					continue
				}
				oraclePlaces, candPlaces := shared, shared
				if op.Mutating { // a fresh pair per place, so one place's write cannot leak into the next
					oraclePlaces, candPlaces = build(t, g, fx), build(t, g, fx)
				}
				diffs = append(diffs, compareOne(t, g, oracle, cand, op, fx.Name, place, oraclePlaces, candPlaces)...)
			}
		}
	}
	return diffs
}

func build(t testing.TB, g Git, fx Fixture) Places {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	places := fx.Build(g, root)
	if places["main"] == "" {
		t.Fatalf("fixture %q returned no \"main\" place", fx.Name)
	}
	return places
}

func compareOne(t testing.TB, g Git, oracle, cand backend.Backend, op Op, fixture, place string, oraclePlaces, candPlaces Places) []Diff {
	t.Helper()
	ctx := context.Background()
	mk := func(kind DiffKind, o, c string) Diff {
		return Diff{Fixture: fixture, Place: place, Op: op.Name, Kind: kind, Oracle: o, Candidate: c}
	}
	oracleNorm, candNorm := newNormalizer(oraclePlaces), newNormalizer(candPlaces)
	oRes, oErr := op.Run(ctx, oracle, backend.Local{Root: backend.RepoRoot(oraclePlaces[place])})
	cRes, cErr := op.Run(ctx, cand, backend.Local{Root: backend.RepoRoot(candPlaces[place])})

	var diffs []Diff
	if oc, cc := ErrorClass(oErr), ErrorClass(cErr); oc != cc {
		diffs = append(diffs, mk(DiffError, oc+errText(oracleNorm, oErr), cc+errText(candNorm, cErr)))
	} else if oErr == nil {
		if o, c := oracleNorm.render(oRes), candNorm.render(cRes); o != c {
			diffs = append(diffs, mk(DiffResult, o, c))
		}
	}
	if !op.Mutating {
		return diffs
	}
	if o, c := snapshot(t, g, oracleNorm, oraclePlaces[place]), snapshot(t, g, candNorm, candPlaces[place]); o != c {
		diffs = append(diffs, mk(DiffState, o, c))
	}
	if out, err := fsck(g, candPlaces[place]); err != nil {
		diffs = append(diffs, mk(DiffFsck, "exit 0", out))
	}
	return diffs
}

func errText(n normalizer, err error) string {
	if err == nil {
		return ""
	}
	return ": " + n.replace(err.Error())
}

// ErrorClass names the sentinel an error matches, "CommandError" for an unclassified git
// failure, and "" for nil. Backends are compared by class, never by message text.
func ErrorClass(err error) string {
	if err == nil {
		return ""
	}
	for _, s := range classes {
		if errors.Is(err, s.err) {
			return s.name
		}
	}
	var ce *backend.CommandError
	if errors.As(err, &ce) {
		return "CommandError"
	}
	return "other"
}

var classes = []struct {
	name string
	err  error
}{
	{"ErrUnborn", backend.ErrUnborn}, {"ErrDetachedHead", backend.ErrDetachedHead},
	{"ErrRefNotFound", backend.ErrRefNotFound}, {"ErrObjectNotFound", backend.ErrObjectNotFound},
	{"ErrNoMergeBase", backend.ErrNoMergeBase}, {"ErrNotARepo", backend.ErrNotARepo},
	{"ErrConfigUnset", backend.ErrConfigUnset}, {"ErrNothingToCommit", backend.ErrNothingToCommit},
	{"ErrInvalidArgument", backend.ErrInvalidArgument}, {"ErrLocked", backend.ErrLocked{}},
	{"ErrNoisyOutput", backend.ErrNoisyOutput},
}

// normalizer replaces every known absolute path with a stable token so two repositories at
// different temp paths render identically.
type normalizer struct{ pairs []string }

func newNormalizer(places Places) normalizer {
	type kv struct{ path, token string }
	var all []kv
	for name, p := range places {
		all = append(all, kv{p, "<" + name + ">"})
		if real, err := filepath.EvalSymlinks(p); err == nil && real != p {
			all = append(all, kv{real, "<" + name + ">"})
		}
	}
	sort.Slice(all, func(i, j int) bool { return len(all[i].path) > len(all[j].path) }) // longest first
	pairs := make([]string, 0, 2*len(all))
	for _, e := range all {
		pairs = append(pairs, e.path, e.token)
	}
	return normalizer{pairs}
}

func (n normalizer) replace(s string) string { return strings.NewReplacer(n.pairs...).Replace(s) }

func (n normalizer) render(v any) string {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		data = []byte(fmt.Sprintf("%+v", v))
	}
	return n.replace(string(data))
}

func sortedKeys(p Places) []string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// snapshot is the repository state a mutating operation must leave identical: refs, HEAD, the
// index, the porcelain status and the worktree list, all read with real git.
func snapshot(t testing.TB, g Git, n normalizer, dir string) string {
	t.Helper()
	var b strings.Builder
	for _, args := range [][]string{
		{"for-each-ref", "--format=%(refname) %(objectname)"},
		{"rev-parse", "--verify", "-q", "HEAD"},
		{"symbolic-ref", "-q", "HEAD"},
		{"ls-files", "--stage"},
		{"status", "--porcelain=v2", "--branch", "--untracked-files=all"},
		{"worktree", "list", "--porcelain"},
	} {
		out, err := Runner{Home: g.Home}.RunStdout(context.Background(), dir, "git", args...)
		fmt.Fprintf(&b, "$ git %s (err=%v)\n%s\n", strings.Join(args, " "), err != nil, out)
	}
	return n.replace(b.String())
}

func fsck(g Git, dir string) (string, error) {
	out, err := Runner{Home: g.Home}.Run(context.Background(), dir, "git", "fsck", "--strict")
	return strings.TrimSpace(string(out)), err
}

// AssertGitFsckClean fails the test unless `git fsck --strict` exits 0 in dir. Run it after a
// candidate backend has written to a repository: a clean fsck is what proves the on-disk
// objects, refs and index are ones real git accepts.
func AssertGitFsckClean(t testing.TB, dir string) {
	t.Helper()
	RequireGit(t)
	if out, err := fsck(Git{T: t, Home: t.TempDir()}, dir); err != nil {
		t.Fatalf("git fsck --strict failed in %s: %v\n%s", dir, err, out)
	}
}

// AssertWorktreeListMatches fails the test unless b.ListWorktrees(dir) agrees with the
// independently parsed `git worktree list --porcelain`.
func AssertWorktreeListMatches(t testing.TB, dir string, b backend.Backend) {
	t.Helper()
	RequireGit(t)
	out, err := Runner{Home: t.TempDir()}.RunStdout(context.Background(), dir, "git", "worktree", "list", "--porcelain")
	if err != nil {
		t.Fatalf("git worktree list: %v", err)
	}
	want := ParseWorktreeList(string(out))
	got, err := b.ListWorktrees(context.Background(), backend.Local{Root: backend.RepoRoot(dir)})
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if g, w := renderWorktrees(got), renderWorktrees(want); g != w {
		t.Fatalf("worktree list differs from git worktree list --porcelain\n got: %s\nwant: %s", g, w)
	}
}

// ParseWorktreeList parses `git worktree list --porcelain`. It is deliberately independent of
// the CLI backend's parser: the oracle must not share code with what it checks.
func ParseWorktreeList(porcelain string) []backend.WorktreeInfo {
	var out []backend.WorktreeInfo
	var cur *backend.WorktreeInfo
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(porcelain, "\n") {
		key, val, _ := strings.Cut(line, " ")
		switch key {
		case "worktree":
			flush()
			cur = &backend.WorktreeInfo{Path: backend.WorktreePath(val)}
		case "HEAD":
			if cur != nil {
				cur.HeadSHA = backend.CommitSHA(val)
			}
		case "branch":
			if cur != nil {
				cur.Branch = backend.RefName(val)
			}
		case "bare":
			if cur != nil {
				cur.Bare = true
			}
		case "detached":
			if cur != nil {
				cur.Detached = true
			}
		case "locked":
			if cur != nil {
				cur.Locked = true
			}
		case "prunable":
			if cur != nil {
				cur.Prunable = true
			}
		}
	}
	flush()
	return out
}

func renderWorktrees(ws []backend.WorktreeInfo) string {
	canon := func(p backend.WorktreePath) string {
		if real, err := filepath.EvalSymlinks(string(p)); err == nil {
			return real
		}
		return string(p)
	}
	lines := make([]string, 0, len(ws))
	for _, w := range ws {
		lines = append(lines, fmt.Sprintf("{%s %s %s bare=%t detached=%t locked=%t prunable=%t}",
			canon(w.Path), w.HeadSHA, w.Branch, w.Bare, w.Detached, w.Locked, w.Prunable))
	}
	return strings.Join(lines, "\n")
}
