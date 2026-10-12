//go:build gitoracle

package gogit_test

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/testutil/gitoracle"
)

// hermeticEnviron is what the gogit backend is told git's environment is. It matches
// gitoracle.Env for every variable the backend reads, without inheriting the test process's own
// GIT_* variables (a hook runs this suite with GIT_DIR set).
func hermeticEnviron(t testing.TB) []string {
	home := t.TempDir()
	return []string{"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, "xdg"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
}

func newCandidate(t testing.TB) backend.Backend {
	env := hermeticEnviron(t)
	return gogit.New(gogit.WithEnviron(func() []string { return env }))
}

type run = func(ctx context.Context, b backend.Backend, l backend.Local) (any, error)

func op(name string, fn run) gitoracle.Op { return gitoracle.Op{Name: name, Run: fn} }

// richRevisions exercises the resolver on the refs-rich fixture: names, rules, suffixes, odd and
// invalid input.
var richRevisions = []string{
	"HEAD", "@", "HEAD~1", "HEAD^", "HEAD^1", "HEAD^2", "HEAD^0", "HEAD~0", "HEAD~2^", "HEAD~99", "HEAD^99", "HEAD~", "@~1",
	"main", "refs/heads/main", "heads/main", "master", "v1", "v2", "refs/tags/v1", "tags/v1", "v1^{commit}", "v1~1", "v2^{commit}",
	"ambig", "heads/ambig", "tags/ambig", "refs/heads/ambig", "origin", "origin/main", "origin/HEAD", "origin/dev", "refs/remotes/origin/main",
	"feat/ünï-code", "a.b", "x@y", "feat/#1", "UPPER", "dash-ed", "with'quote", "feature/deep/er/name",
	"nonexistent", "refs/heads/nonexistent", "ORIG_HEAD", "FETCH_HEAD", "HEAD@{1}", "main@{u}", "@{-1}", ":/first", "HEAD^{tree}",
	"HEAD:a.txt", "HEAD^{}", "HEAD..main", "main...feature", "a b", "refs/heads/../../x", "../x", "/abs", "x/", "main^{commit}^",
	"1111111111111111111111111111111111111111", "0000000000000000000000000000000000000000", "abcd", "deadbeef", "~1", "^",
	"refs/heads/", "refs/", "refs", "heads", "-x", "--all", "", "main.lock", ".hidden", "HEAD~999999999999",
	"full sha of HEAD", "upper sha of HEAD", "short sha of HEAD",
}

// existsRevisions are the revisions RefExists also runs (it shares the resolver; this keeps the
// not-found and unborn answers honest without doubling the CLI calls).
var existsRevisions = []string{"HEAD", "@", "HEAD~1", "main", "refs/heads/main", "v1", "ambig", "origin/main", "nonexistent", "HEAD^{commit}", "-x", "", "HEAD:a.txt", "UPPER", "tree-tag"}

var coreRevisions = []string{"HEAD", "@", "HEAD~1", "HEAD^", "HEAD^{commit}", "main", "refs/heads/main", "nonexistent", "-x", "", "HEAD:a.txt", "full sha of HEAD"}

// fixtureRevisions picks the revisions worth running on a fixture; every CLI call is a process
// spawn, so the full list is spent only where its names exist.
var fixtureRevisions = map[string][]string{
	"refs-rich":        richRevisions,
	"packed-refs":      {"HEAD", "main", "old", "fresh", "gone", "rel", "rel^{commit}", "refs/tags/rel", "light", "tags/light", "heads/old", "nonexistent", "HEAD~1", "full sha of HEAD"},
	"ambiguous-head":   {"HEAD", "same", "heads/same", "tags/same", "refs/heads/same", "main", "remotes/main", "refs/remotes/main", "origin", "same^{commit}", "full sha of HEAD"},
	"annotated-tags":   {"t1", "t2", "t2^{commit}", "t2~1", "tree-tag", "refs/tags/t2", "tags/t2", "HEAD", "main", "t2^{}", "full sha of HEAD"},
	"dangling-head":    {"HEAD", "@", "main", "healthy", "HEAD~1", "refs/heads/main", "heads/main", "full sha of HEAD", "upper sha of HEAD"},
	"dangling-symref":  {"HEAD", "alias", "origin", "origin/HEAD", "refs/remotes/origin/HEAD", "refs/heads/alias", "heads/alias"},
	"shallow":          {"HEAD", "HEAD~1", "HEAD^", "HEAD^0", "side", "origin/side", "origin/main", "HEAD^{commit}", "main"},
	"linked-worktrees": {"HEAD", "HEAD~0", "main", "wt-branch", "refs/heads/wt-branch", "full sha of HEAD", "short sha of HEAD", "nonexistent"},
	"head-names":       {"HEAD", "bt", "heads/bt", "plain", "full sha of HEAD"},
	"detached-head":    {"HEAD", "HEAD~1", "HEAD^", "main", "@", "full sha of HEAD", "upper sha of HEAD", "short sha of HEAD"},
	"symlinked":        {"HEAD", "main", "wt", "full sha of HEAD"},
	"subdirs":          {"HEAD", "main", "wt", "full sha of HEAD", "nonexistent"},
	"unborn":           {"HEAD", "@", "HEAD~1", "HEAD^{commit}", "main", "refs/heads/main", "nonexistent", "full sha of HEAD"},
}

// listRefsPatterns run once each; the Limit variants run on limitPatterns only.
var (
	listRefsPatterns = []string{"", "refs/heads", "refs/heads/", "refs/tags/v1", "refs/tags", "refs/hea", "heads", "refs/heads/main", "refs/remotes", "refs/remotes/origin/", "refs", "refs/", "refs/heads/feat", "-x", "refs/heads/*", "refs/*/main", "refs/heads/[a-m]*", `refs/heads/a\.b`, "refs/heads/feat/ünï-code", "HEAD"}
	limitPatterns    = map[string]bool{"": true, "refs/heads": true, "refs/tags": true}
)

func listRefOps(all bool) []gitoracle.Op {
	var ops []gitoracle.Op
	for _, pat := range listRefsPatterns {
		for _, limit := range []int{0, 1, 3} {
			if (limit != 0 && !limitPatterns[pat]) || (!all && pat != "" && pat != "refs/heads/") {
				continue
			}
			req := backend.ListRefsRequest{Pattern: backend.RefPattern(pat), Limit: limit}
			ops = append(ops, op(fmt.Sprintf("ListRefs(%q,%d)", pat, limit), func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
				return b.ListRefs(ctx, l, req)
			}))
		}
	}
	return ops
}

func resolveOps(revs []string) []gitoracle.Op {
	var ops []gitoracle.Op
	exists := map[string]bool{}
	for _, r := range existsRevisions {
		exists[r] = true
	}
	for _, rev := range revs {
		ref := backend.RefName(rev)
		switch rev {
		case "full sha of HEAD", "upper sha of HEAD", "short sha of HEAD":
			ops = append(ops, op("ResolveRef("+rev+")", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
				sha, err := b.ResolveRef(ctx, l, "HEAD")
				if err != nil {
					return nil, err
				}
				switch rev {
				case "upper sha of HEAD":
					sha = backend.CommitSHA(strings.ToUpper(string(sha)))
				case "short sha of HEAD":
					sha = sha[:10]
				}
				return b.ResolveRef(ctx, l, backend.RefName(sha))
			}))
			continue
		}
		ops = append(ops, op("ResolveRef("+rev+")", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.ResolveRef(ctx, l, ref)
		}))
		if exists[rev] {
			ops = append(ops, op("RefExists("+rev+")", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
				return b.RefExists(ctx, l, ref)
			}))
		}
	}
	return ops
}

// mergeBasePairs are named pairs; the DAG fixture adds every pair of its generated branches.
var mergeBasePairs = [][2]string{
	{"main", "feature"}, {"HEAD", "HEAD"}, {"HEAD", "HEAD~1"}, {"HEAD~1", "HEAD"}, {"HEAD", "nonexistent"}, {"nonexistent", "HEAD"},
	{"left", "main"}, {"left", "right"}, {"m1", "m2"}, {"main", "other"}, {"v1", "main"}, {"t2", "main"}, {"HEAD", "tree-tag"},
	{"-x", "HEAD"}, {"HEAD", ""}, {"HEAD", "HEAD:a.txt"}, {"origin/main", "main"}, {"wt-branch", "main"}, {"side", "main"}, {"origin/side", "main"},
	{"HEAD^{commit}", "main"}, {"v2", "v1"}, {"main~1", "main"},
}

func mergeBaseOps(fixture string) []gitoracle.Op {
	pairs := append([][2]string(nil), mergeBasePairs...)
	if fixture == "random-dag" {
		for i := 0; i < 22; i += 4 {
			for j := 0; j < 22; j++ {
				pairs = append(pairs, [2]string{fmt.Sprintf("c%d", i), fmt.Sprintf("c%d", j)})
			}
		}
	}
	var ops []gitoracle.Op
	for _, p := range pairs {
		req := backend.MergeBaseRequest{Left: backend.RefName(p[0]), Right: backend.RefName(p[1])}
		ops = append(ops, op(fmt.Sprintf("MergeBase(%s,%s)", p[0], p[1]), func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.MergeBase(ctx, l, req)
		}))
	}
	return ops
}

func simpleOps() []gitoracle.Op {
	return []gitoracle.Op{
		op("CurrentBranch", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) {
			return b.CurrentBranch(ctx, l)
		}),
		op("HeadRef", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.HeadRef(ctx, l) }),
		op("RepoRoot", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.RepoRoot(ctx, l) }),
		op("GitDir", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.GitDir(ctx, l) }),
		op("CommonDir", func(ctx context.Context, b backend.Backend, l backend.Local) (any, error) { return b.CommonDir(ctx, l) }),
	}
}

func opsFor(fixture string) []gitoracle.Op {
	revs, ok := fixtureRevisions[fixture]
	if !ok {
		revs = coreRevisions
	}
	ops := append(simpleOps(), resolveOps(revs)...)
	_, rich := fixtureRevisions[fixture]
	ops = append(ops, listRefOps(rich && fixture != "unborn" && fixture != "symlinked" && fixture != "subdirs")...)
	switch fixture {
	case "refs-rich", "annotated-tags", "linear", "diverged", "criss-cross", "unrelated-histories", "shallow", "random-dag", "dangling-head", "linked-worktrees", "bare", "submodule":
		ops = append(ops, mergeBaseOps(fixture)...)
	}
	return ops
}

// parityDiffs runs the fixture's operations on cand and the CLI and returns every difference that
// is not a documented fallback.
func parityDiffs(t *testing.T, cand backend.Backend, fx gitoracle.Fixture) []string {
	t.Helper()
	diffs := gitoracle.RunOps(t, cand, opsFor(fx.Name), []gitoracle.Fixture{fx})
	var unexpected []string
	for _, d := range diffs {
		if d.Kind == gitoracle.DiffError && strings.Contains(d.Candidate, "fallback eligible") && fallbackExpected(d) {
			continue
		}
		unexpected = append(unexpected, oneLine(d))
	}
	sort.Strings(unexpected)
	return unexpected
}

func TestParityRefs(t *testing.T) {
	for _, fx := range refFixtures() {
		t.Run(fx.Name, func(t *testing.T) {
			t.Parallel()
			unexpected := parityDiffs(t, newCandidate(t), fx)
			require.Empty(t, unexpected, "%d unexpected difference(s) from the git CLI", len(unexpected))
		})
	}
}

var (
	revOpName       = regexp.MustCompile(`^(ResolveRef|RefExists)\((.*)\)$`)
	mergeBaseName   = regexp.MustCompile(`^MergeBase\(([^,]*),(.*)\)$`)
	listRefsPattern = regexp.MustCompile(`^ListRefs\("(.*)",\d+\)$`)
)

// unmodelledRevisions are inputs whose CLI behaviour the resolver deliberately leaves to git:
// pseudo-refs and all-caps names, @{...} and :/ syntax, ranges, names that are not valid ref
// names, abbreviated object names, objects that are not commits.
var unmodelledRevisions = map[string]bool{
	"ORIG_HEAD": true, "FETCH_HEAD": true, "UPPER": true, "HEAD@{1}": true, "main@{u}": true, "@{-1}": true, ":/first": true,
	"HEAD..main": true, "main...feature": true, "a b": true, "refs/heads/../../x": true, "../x": true, "/abs": true, "x/": true,
	"abcd": true, "deadbeef": true, "~1": true, "^": true, "refs/heads/": true, "refs/": true, ".hidden": true, "main.lock": true,
	"HEAD~999999999999": true, "short sha of HEAD": true, "tree-tag": true,
}

// absentFullSHAs are well-formed SHAs with no object: possibly a torn read, so the CLI decides.
var absentFullSHAs = map[string]bool{strings.Repeat("1", 40): true, strings.Repeat("0", 40): true}

func hasSteps(rev string) bool {
	return strings.ContainsAny(strings.TrimSuffix(rev, "^{commit}"), "~^")
}

// fallbackExpected reports whether the backend is documented to hand this call to the CLI. A
// fallback outside this list is a regression (it silently stopped serving something), and a
// difference that is not a fallback is a wrong answer.
func fallbackExpected(d gitoracle.Diff) bool {
	if noWorkTree(d) {
		return d.Op != "GitDir" && d.Op != "CommonDir"
	}
	if m := listRefsPattern.FindStringSubmatch(d.Op); m != nil {
		if strings.ContainsAny(m[1], `*?[\\`) {
			return true
		}
		return d.Fixture == "dangling-head"
	}
	if m := revOpName.FindStringSubmatch(d.Op); m != nil {
		return revisionFallback(d, m[2])
	}
	if m := mergeBaseName.FindStringSubmatch(d.Op); m != nil {
		return mergeBaseFallback(d, m[1], m[2])
	}
	return false
}

func noWorkTree(d gitoracle.Diff) bool {
	return d.Fixture == "bare" ||
		(d.Fixture == "subdirs" && (d.Place == "dotgit" || d.Place == "refs-dir" || d.Place == "admin-dir")) ||
		(d.Fixture == "symlinked" && d.Place == "inside")
}

func revisionFallback(d gitoracle.Diff, rev string) bool {
	switch {
	case unmodelledRevisions[rev] || absentFullSHAs[rev]:
		return true
	case d.Fixture == "shallow" && hasSteps(rev):
		return true
	case d.Fixture == "dangling-head":
		return danglingHeadRevs[rev] || rev == "full sha of HEAD" || rev == "upper sha of HEAD"
	}
	return false
}

// danglingHeadRevs resolve to the branch that points at a missing object.
var danglingHeadRevs = map[string]bool{
	"HEAD": true, "@": true, "HEAD~1": true, "HEAD^": true, "HEAD^1": true, "HEAD^2": true, "HEAD^0": true, "HEAD~0": true, "HEAD~2^": true,
	"HEAD~99": true, "HEAD^99": true, "main": true, "refs/heads/main": true, "heads/main": true, "HEAD^{commit}": true, "@~1": true, "HEAD~": true,
}

func mergeBaseFallback(d gitoracle.Diff, left, right string) bool {
	for _, rev := range []string{left, right} {
		if rev == "HEAD:a.txt" || unmodelledRevisions[rev] {
			return true
		}
		if d.Fixture == "shallow" || (d.Fixture == "dangling-head" && danglingHeadRevs[rev]) {
			return true
		}
	}
	if d.Fixture == "criss-cross" {
		return (left == "m1" && right == "m2") || (left == "left" && right == "right")
	}
	return d.Fixture == "random-dag" && strings.Contains(d.Candidate, "merge bases: git picks one")
}

func oneLine(d gitoracle.Diff) string {
	flat := func(s string) string { return strings.ReplaceAll(s, "\n", " | ") }
	return fmt.Sprintf("%s/%s %s [%s] oracle=%s candidate=%s", d.Fixture, d.Place, d.Op, d.Kind, flat(d.Oracle), flat(d.Candidate))
}
