package gogit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session/git/backend"
)

// hermetic is an environment with no config sources and no repository redirection.
func hermetic(t testing.TB) []string {
	home := t.TempDir()
	return []string{"HOME=" + home, "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1"}
}

func newBackend(t testing.TB, opts ...Option) *Backend {
	env := hermetic(t)
	all := append([]Option{WithEnviron(func() []string { return env })}, opts...)
	b := New(all...)
	b.retryDelay = 0
	return b
}

func real(t testing.TB, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	require.NoError(t, err)
	return r
}

// initRepo creates a repository on main with n empty commits, using go-git only (no git CLI).
func initRepo(t testing.TB, n int) (dir string, repo *git.Repository, head plumbing.Hash) {
	t.Helper()
	dir = real(t, t.TempDir())
	repo, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: "refs/heads/main"}})
	require.NoError(t, err)
	wt, err := repo.Worktree()
	require.NoError(t, err)
	for i := range n {
		head, err = wt.Commit(fmt.Sprintf("c%d", i), &git.CommitOptions{
			AllowEmptyCommits: true,
			Author:            &object.Signature{Name: "T", Email: "t@example.com", When: time.Unix(1_600_000_000+int64(i), 0)},
		})
		require.NoError(t, err)
	}
	return dir, repo, head
}

func local(dir string) backend.Local { return backend.Local{Root: backend.RepoRoot(dir)} }

func isFallback(err error) bool {
	var e backend.ErrFallbackEligible
	return errors.As(err, &e) && !e.Wrote
}

func TestServedOperationsAreExactlyTheImplementedOnes(t *testing.T) {
	b := newBackend(t)
	nonRepo := t.TempDir()
	served := map[backend.OperationName]bool{}
	for _, op := range servedOperations() {
		served[op] = true
	}
	ifaceT := reflect.TypeOf((*backend.Backend)(nil)).Elem()
	require.Equal(t, len(backend.AllOperations()), ifaceT.NumMethod())
	for i := range ifaceT.NumMethod() {
		m := ifaceT.Method(i)
		args := []reflect.Value{reflect.ValueOf(b), reflect.ValueOf(context.Background()), reflect.ValueOf(local(nonRepo))}
		for j := 3; j < m.Type.NumIn()+1; j++ { // +1: the receiver is not in Type.In for interface methods
			args = append(args, reflect.Zero(m.Type.In(j-1)))
		}
		out := reflect.ValueOf(b).MethodByName(m.Name).Call(args[1:])
		err, _ := out[len(out)-1].Interface().(error)
		require.Error(t, err, m.Name)
		if served[backend.OperationName(m.Name)] {
			assert.NotErrorIs(t, err, backend.ErrNotImplemented, "%s is served", m.Name)
			continue
		}
		assert.ErrorIs(t, err, backend.ErrNotImplemented, m.Name)
		assert.True(t, isFallback(err), "%s must be fallback-eligible with Wrote=false", m.Name)
	}
}

func TestUnbornHeadIsNotARefNotFound(t *testing.T) {
	dir := real(t, t.TempDir())
	_, err := git.PlainInitWithOptions(dir, &git.PlainInitOptions{InitOptions: git.InitOptions{DefaultBranch: "refs/heads/main"}})
	require.NoError(t, err)
	b := newBackend(t)
	ctx := context.Background()

	_, err = b.ResolveRef(ctx, local(dir), "HEAD")
	require.ErrorIs(t, err, backend.ErrUnborn)
	assert.NotErrorIs(t, err, backend.ErrRefNotFound)
	_, err = b.ResolveRef(ctx, local(dir), "HEAD~1")
	require.ErrorIs(t, err, backend.ErrRefNotFound)
	_, err = b.CurrentBranch(ctx, local(dir))
	require.ErrorIs(t, err, backend.ErrUnborn)
	ref, err := b.HeadRef(ctx, local(dir))
	require.NoError(t, err)
	assert.Equal(t, backend.RefName("refs/heads/main"), ref)
	ok, err := b.RefExists(ctx, local(dir), "HEAD")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestFreshHandlePerCall(t *testing.T) {
	dir, repo, first := initRepo(t, 1)
	b := newBackend(t)
	got, err := b.ResolveRef(context.Background(), local(dir), "main")
	require.NoError(t, err)
	assert.Equal(t, first.String(), string(got))

	wt, err := repo.Worktree()
	require.NoError(t, err)
	second, err := wt.Commit("next", &git.CommitOptions{AllowEmptyCommits: true, Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Unix(1_700_000_000, 0)}})
	require.NoError(t, err)
	got, err = b.ResolveRef(context.Background(), local(dir), "main")
	require.NoError(t, err)
	assert.Equal(t, second.String(), string(got), "a cached handle would still see the first commit")
}

// danglingRef points refs/heads/main at the hash of a commit that exists only in a donor
// repository, so the object can be "published" later to simulate a repack finishing.
func danglingRef(t *testing.T) (dir string, missing plumbing.Hash, publish func()) {
	t.Helper()
	dir, _, _ = initRepo(t, 1)
	_, donor, donorHead := initRepo(t, 2)
	obj, err := donor.Storer.EncodedObject(plumbing.CommitObject, donorHead)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "refs", "heads", "main"), []byte(donorHead.String()+"\n"), 0o600))
	return dir, donorHead, func() {
		target, err := git.PlainOpen(dir) //nolint:norawgitopen test writes an object through its own handle
		require.NoError(t, err)
		_, err = target.Storer.SetEncodedObject(obj)
		require.NoError(t, err)
	}
}

func TestMissingObjectIsNeverReturnedAsASHA(t *testing.T) {
	dir, missing, _ := danglingRef(t)
	b := newBackend(t)
	var sleeps atomic.Int32
	b.sleep = func(context.Context, time.Duration) { sleeps.Add(1) }

	for _, ref := range []backend.RefName{"HEAD", "main", "refs/heads/main", backend.RefName(missing.String())} {
		sha, err := b.ResolveRef(context.Background(), local(dir), ref)
		require.Error(t, err, ref)
		assert.Empty(t, sha, ref)
		assert.ErrorIs(t, err, backend.ErrObjectMissing, ref)
		assert.True(t, isFallback(err), ref)
	}
	assert.EqualValues(t, 4*defaultRetries, sleeps.Load(), "each lookup retries before giving up")

	_, err := b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{})
	assert.ErrorIs(t, err, backend.ErrObjectMissing)
	exists, err := b.RefExists(context.Background(), local(dir), "main")
	assert.False(t, exists)
	assert.ErrorIs(t, err, backend.ErrObjectMissing)
}

func TestTornReadRecoversOnAFreshHandle(t *testing.T) {
	dir, missing, publish := danglingRef(t)
	b := newBackend(t)
	var sleeps int
	b.sleep = func(context.Context, time.Duration) {
		sleeps++
		if sleeps == 2 {
			publish() // the repack finishes between the second and third attempt
		}
	}
	sha, err := b.ResolveRef(context.Background(), local(dir), "main")
	require.NoError(t, err)
	assert.Equal(t, missing.String(), string(sha))
	assert.Equal(t, 2, sleeps)
}

func TestRepositoryRedirectingEnvironmentFallsBack(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	for _, v := range repoRedirectVars {
		t.Run(v, func(t *testing.T) {
			env := append(hermetic(t), v+"=/elsewhere")
			b := New(WithEnviron(func() []string { return env }))
			_, err := b.ResolveRef(context.Background(), local(dir), "HEAD")
			assert.True(t, isFallback(err), "%v", err)
			_, err = b.CurrentBranch(context.Background(), local(dir))
			assert.True(t, isFallback(err), "%v", err)
			_, err = b.RepoRoot(context.Background(), local(dir))
			assert.True(t, isFallback(err), "%v", err)
		})
	}
}

func TestRepositoryOwnedBySomeoneElseFallsBack(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	b := newBackend(t)
	b.uid = func() int { return os.Geteuid() + 1 }
	_, err := b.CurrentBranch(context.Background(), local(dir))
	assert.True(t, isFallback(err), "%v", err)
	_, err = b.GitDir(context.Background(), local(dir))
	assert.True(t, isFallback(err), "%v", err)
}

func TestLocationValidation(t *testing.T) {
	b := newBackend(t)
	_, err := b.CurrentBranch(context.Background(), backend.Local{})
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
	_, err = b.CurrentBranch(context.Background(), backend.Remote{Host: "h", Path: "/p"})
	assert.True(t, isFallback(err), "%v", err)
	_, err = b.CurrentBranch(context.Background(), local(filepath.Join(t.TempDir(), "missing")))
	assert.True(t, isFallback(err), "a missing directory is the CLI's to report: %v", err)
	_, err = b.CurrentBranch(context.Background(), local(t.TempDir()))
	assert.ErrorIs(t, err, backend.ErrNotARepo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dir, _, _ := initRepo(t, 1)
	_, err = b.ResolveRef(ctx, local(dir), "HEAD")
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, isFallback(err))
}

func TestRefNamesCannotEscapeTheRepository(t *testing.T) {
	dir, _, head := initRepo(t, 1)
	outside := filepath.Join(filepath.Dir(dir), "outside-secret")
	require.NoError(t, os.WriteFile(outside, []byte(head.String()+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "refs", "heads", "evil"), []byte("ref: refs/heads/../../../outside-secret\n"), 0o600))
	b := newBackend(t)
	for _, ref := range []backend.RefName{"../outside-secret", "refs/heads/../../../outside-secret", "heads/../../../outside-secret", "evil", "/etc/passwd", "refs/heads/evil"} {
		sha, err := b.ResolveRef(context.Background(), local(dir), ref)
		assert.Empty(t, sha, ref)
		assert.Error(t, err, ref)
	}
	_, err := b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{})
	assert.True(t, isFallback(err), "a symbolic ref that escapes is never followed; the CLI decides: %v", err)
}

func TestSystemConfigCandidateOnlyMattersWhenItMentionsTheKey(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	sys := filepath.Join(t.TempDir(), "gitconfig")
	require.NoError(t, os.WriteFile(sys, []byte("[user]\n\tname = Sys\n"), 0o600))
	env := []string{"HOME=" + t.TempDir(), "GIT_CONFIG_GLOBAL=/dev/null"} // system config not disabled
	b := New(WithEnviron(func() []string { return env }))
	b.systemCfgs = []string{sys}

	_, err := b.GetConfig(context.Background(), local(dir), "user.name")
	assert.True(t, isFallback(err), "the system file may be git's own and it sets user.name: %v", err)
	_, err = b.GetConfig(context.Background(), local(dir), "core.editor")
	assert.ErrorIs(t, err, backend.ErrConfigUnset, "the system file never mentions core.editor")
}

func TestConfigReaderEdgeCases(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	cfg := filepath.Join(dir, ".git", "config")
	existing, err := os.ReadFile(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cfg, append(existing, []byte("[a]\n\tk = v\n\tk = last\n[A]\n\tK = upper\n[a \"Sub\"]\n\tk = sub\n")...), 0o600))
	b := newBackend(t)
	for key, want := range map[string]string{"a.k": "upper", "A.K": "upper", "a.Sub.k": "sub"} {
		got, err := b.GetConfig(context.Background(), local(dir), backend.ConfigKey(key))
		require.NoError(t, err, key)
		assert.Equal(t, want, got, key)
	}
	_, err = b.GetConfig(context.Background(), local(dir), "a.sub.k")
	assert.ErrorIs(t, err, backend.ErrConfigUnset, "subsections are case-sensitive")
	_, err = b.GetConfig(context.Background(), local(dir), "-x")
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
}

// TestConcurrentCallsWhileRefsChange reads while a go-git writer (which rewrites ref files in
// place, unlike git's atomic rename) keeps committing. Every answer must be correct at some
// instant or fall back; a wrong SHA, branch or ref list fails.
func TestConcurrentCallsWhileRefsChange(t *testing.T) {
	dir, repo, _ := initRepo(t, 3)
	b := newBackend(t)
	wt, err := repo.Worktree()
	require.NoError(t, err)

	var known sync.Map // every commit the writer made: the only SHAs HEAD may ever resolve to
	remember := func(h plumbing.Hash) { known.Store(h.String(), true) }
	iter, err := repo.Log(&git.LogOptions{})
	require.NoError(t, err)
	require.NoError(t, iter.ForEach(func(c *object.Commit) error { remember(c.Hash); return nil }))

	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			h, err := wt.Commit(fmt.Sprintf("w%d", i), &git.CommitOptions{AllowEmptyCommits: true, Author: &object.Signature{Name: "T", Email: "t@example.com", When: time.Unix(1_800_000_000+int64(i), 0)}})
			if err != nil {
				continue
			}
			remember(h)
			_ = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(fmt.Sprintf("refs/heads/b%d", i%5)), h))
		}
	}()
	check := func(err error, ok func()) {
		if err != nil {
			assert.True(t, isFallback(err), "only a fallback may fail while refs change: %v", err)
			return
		}
		ok()
	}
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			ctx := context.Background()
			for range 20 {
				sha, err := b.ResolveRef(ctx, local(dir), "HEAD")
				check(err, func() {
					_, isKnown := known.Load(string(sha))
					// the writer records a commit just after creating it, so allow one unrecorded SHA
					if !isKnown {
						_, objErr := repo.CommitObject(plumbing.NewHash(string(sha)))
						assert.NoError(t, objErr, "HEAD resolved to %s, which is not a commit", sha)
					}
				})
				br, err := b.CurrentBranch(ctx, local(dir))
				check(err, func() { assert.Equal(t, backend.BranchName("main"), br) })
				refs, err := b.ListRefs(ctx, local(dir), backend.ListRefsRequest{Pattern: "refs/heads"})
				check(err, func() { assert.Contains(t, refs, backend.RefName("refs/heads/main")) })
				_, err = b.MergeBase(ctx, local(dir), backend.MergeBaseRequest{Left: "HEAD", Right: "HEAD~1"})
				check(err, func() {})
			}
		}()
	}
	readers.Wait()
	close(stop)
	wg.Wait()
}

func TestRefFilesGitWouldRejectAreNotTrusted(t *testing.T) {
	dir, _, head := initRepo(t, 2)
	b := newBackend(t)
	ctx := context.Background()
	refDir := filepath.Join(dir, ".git", "refs", "heads")
	for name, content := range map[string]string{
		"leading-space": " " + head.String() + "\n", "leading-newline": "\n" + head.String() + "\n", "extra-hex": head.String() + "a\n",
		"empty": "", "zero": plumbing.ZeroHash.String() + "\n", "garbage": "garbage\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(refDir, name), []byte(content), 0o600))
		sha, err := b.ResolveRef(ctx, local(dir), backend.RefName(name))
		assert.Empty(t, sha, name)
		assert.True(t, isFallback(err), "%s: %v", name, err)
	}

	require.NoError(t, os.WriteFile(filepath.Join(refDir, "upper"), []byte(strings.ToUpper(head.String())+"\n"), 0o600))
	upper, err := b.ResolveRef(ctx, local(dir), "upper")
	require.NoError(t, err, "git reads uppercase hex")
	assert.Equal(t, head.String(), string(upper))

	headFile := filepath.Join(dir, ".git", "HEAD")
	require.NoError(t, os.WriteFile(headFile, []byte("ref:refs/heads/main\n"), 0o600))
	br, err := b.CurrentBranch(ctx, local(dir))
	require.NoError(t, err, "git accepts a ref: with no space")
	assert.Equal(t, backend.BranchName("main"), br)
	for _, content := range []string{head.String() + "garbage\n", " " + head.String() + "\n", "garbage\n", ""} {
		require.NoError(t, os.WriteFile(headFile, []byte(content), 0o600))
		_, err = b.CurrentBranch(ctx, local(dir))
		assert.Error(t, err, "HEAD=%q", content)
		assert.NotErrorIs(t, err, backend.ErrDetachedHead, "HEAD=%q must not be reported detached", content)
	}
}

func TestSymrefChainDepthMatchesGit(t *testing.T) {
	dir, _, head := initRepo(t, 1)
	b := newBackend(t)
	prev := "refs/heads/main"
	for i := 1; i <= 6; i++ {
		name := fmt.Sprintf("refs/heads/a%d", i)
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", name), []byte("ref: "+prev+"\n"), 0o600))
		prev = name
		sha, err := b.ResolveRef(context.Background(), local(dir), backend.RefName(fmt.Sprintf("a%d", i)))
		if i <= 3 { // git reads at most 5 refs along a chain, the last one direct
			require.NoError(t, err, i)
			assert.Equal(t, head.String(), string(sha))
		} else if i >= 5 {
			assert.True(t, isFallback(err), "a%d is beyond git's depth limit and git reports it per command: %v", i, err)
		}
	}
}

func TestHostileConfigDoesNotHangOrExplode(t *testing.T) {
	dir, _, _ := initRepo(t, 1)
	home := t.TempDir()
	// a file that includes itself eight times: git's depth limit still allows 8^10 reads
	var self string
	for range 8 {
		self += "[include]\n\tpath = self.cfg\n"
	}
	require.NoError(t, os.WriteFile(filepath.Join(home, "self.cfg"), []byte(self), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[include]\n\tpath = self.cfg\n"), 0o600))
	b := New(WithEnviron(func() []string { return []string{"HOME=" + home} }), WithSystemConfig())
	start := time.Now()
	_, err := b.GetConfig(context.Background(), local(dir), "core.bare")
	assert.True(t, isFallback(err), "%v", err)
	assert.Less(t, time.Since(start), 5*time.Second)

	big := filepath.Join(home, "big.cfg")
	require.NoError(t, os.WriteFile(big, []byte("[a]\n\tk = "+strings.Repeat("x", maxConfigFileSize)+"\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[include]\n\tpath = big.cfg\n"), 0o600))
	_, err = b.GetConfig(context.Background(), local(dir), "a.k")
	assert.True(t, isFallback(err), "an oversized config is left to git: %v", err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = b.GetConfig(ctx, local(dir), "a.k")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestMalformedConfigHeadersAndExtensionsFallBack(t *testing.T) {
	for name, text := range map[string]string{
		"padded header":     "[ padded ]\n\tk = v\n",
		"padded subsection": "[a \"b\" ]\n\tk = v\n",
		"v0 extension":      "[extensions]\n\tobjectformat = sha1\n",
	} {
		dir, _, _ := initRepo(t, 1)
		cfg := filepath.Join(dir, ".git", "config")
		existing, err := os.ReadFile(cfg)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(cfg, append(existing, []byte(text)...), 0o600))
		_, err = newBackend(t).GitDir(context.Background(), local(dir))
		assert.True(t, isFallback(err), "%s: %v", name, err)
	}
}

func TestBrokenHeadMakesNotARepository(t *testing.T) {
	outer, _, _ := initRepo(t, 1)
	sub := filepath.Join(outer, "sub")
	for _, d := range []string{"objects", "refs"} {
		require.NoError(t, os.MkdirAll(filepath.Join(sub, ".git", d), 0o700))
	}
	require.NoError(t, os.WriteFile(filepath.Join(sub, ".git", "HEAD"), nil, 0o600))
	root, err := newBackend(t).RepoRoot(context.Background(), local(sub))
	require.NoError(t, err)
	assert.Equal(t, backend.RepoRoot(outer), root, "git skips a .git whose HEAD it rejects and finds the enclosing repository")
}

func TestShortenUnambiguous(t *testing.T) {
	cases := []struct {
		ref     string
		present []string
		want    string
	}{
		{"refs/heads/main", nil, "main"},
		{"refs/heads/main", []string{"refs/tags/main"}, "heads/main"},
		{"refs/heads/main", []string{"refs/remotes/main"}, "heads/main"},
		{"refs/heads/main", []string{"refs/remotes/main/HEAD"}, "heads/main"},
		{"refs/heads/a/b", []string{"refs/tags/a/b"}, "heads/a/b"},
		{"refs/heads/tags/x", []string{"refs/tags/x"}, "heads/tags/x"},
		{"refs/heads/x", []string{"refs/heads/y"}, "x"},
		{"refs/remotes/origin/main", nil, "origin/main"},
		{"refs/remotes/origin/HEAD", nil, "origin/HEAD"},
		{"refs/tags/v1", nil, "v1"},
		{"refs/notes/commits", nil, "notes/commits"},
		{"refs/heads/FOO", []string{"FOO"}, "heads/FOO"},
	}
	for _, c := range cases {
		present := map[string]bool{}
		for _, p := range c.present {
			present[p] = true
		}
		got, err := shortenUnambiguous(c.ref, func(n string) (bool, error) { return present[n], nil })
		require.NoError(t, err)
		assert.Equal(t, c.want, got, "%s with %v", c.ref, c.present)
	}
}

func TestParseRev(t *testing.T) {
	type tc struct {
		in   string
		base string
		ops  []revOp
		ok   bool
	}
	for _, c := range []tc{
		{"main", "main", nil, true},
		{"main^{commit}", "main", nil, true},
		{"HEAD~3", "HEAD", []revOp{{true, 3}}, true},
		{"HEAD~", "HEAD", []revOp{{true, 1}}, true},
		{"HEAD^", "HEAD", []revOp{{false, 1}}, true},
		{"HEAD^2~4^0", "HEAD", []revOp{{false, 2}, {true, 4}, {false, 0}}, true},
		{"v1~2^{commit}", "v1", []revOp{{true, 2}}, true},
		{"~1", "", nil, false},
		{"^", "", nil, false},
		{"", "", nil, false},
		{"HEAD~x", "", nil, false},
		{"HEAD~1234567", "", nil, false},
	} {
		base, ops, ok := parseRev(c.in)
		assert.Equal(t, c.ok, ok, c.in)
		if c.ok {
			assert.Equal(t, c.base, base, c.in)
			assert.Equal(t, c.ops, ops, c.in)
		}
	}
}

func TestValidRefName(t *testing.T) {
	valid := []string{"refs/heads/main", "refs/heads/feat/ünï", "refs/heads/a.b", "refs/tags/v1.0", "refs/heads/x@y", "refs/heads/#1"}
	invalid := []string{"", "refs", "refs/heads/", "refs//x", "refs/heads/../x", "refs/heads/.x", "refs/heads/x.lock", "refs/heads/x.", "refs/heads/a b", "refs/heads/a~b",
		"refs/heads/a^b", "refs/heads/a:b", "refs/heads/a?b", "refs/heads/a*b", "refs/heads/a[b", `refs/heads/a\b`, "refs/heads/a@{b", "refs/heads/\x01", "refs/heads/\x7f", "@", "/refs/x"}
	for _, n := range valid {
		assert.True(t, validRefName(n, false), n)
	}
	for _, n := range invalid {
		assert.False(t, validRefName(n, false), n)
	}
	assert.True(t, validRefName("FETCH_HEAD", true))
	assert.False(t, validRefName("FETCH_HEAD", false))
}

func TestMatchesPattern(t *testing.T) {
	for pattern, want := range map[string]bool{
		"": true, "refs/heads": true, "refs/heads/": true, "refs/heads/main": true, "refs/hea": false, "refs/heads/mai": false,
		"refs/tags": false, "heads": false, "refs/heads/main/": false, "refs": true, "refs/": true,
	} {
		assert.Equal(t, want, matchesPattern(pattern, "refs/heads/main"), pattern)
	}
}

func TestHasLineContinuation(t *testing.T) {
	assert.True(t, hasLineContinuation([]byte("[a]\n\tk = x\\\n y\n")))
	assert.True(t, hasLineContinuation([]byte("k = x\\\r\n y")))
	assert.False(t, hasLineContinuation([]byte("k = x\\\\\n")))
	assert.False(t, hasLineContinuation([]byte("k = x\n")))
}

func TestParseKey(t *testing.T) {
	for in, want := range map[string]cfgKey{
		"core.bare": {"core", "", "bare"}, "remote.origin.url": {"remote", "origin", "url"}, "a.b.c.d": {"a", "b.c", "d"},
	} {
		got, ok := parseKey(in)
		assert.True(t, ok, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "nodot", ".x", "a.", "a..b", "a.1b", "a b.c", "a.b c"} {
		_, ok := parseKey(in)
		assert.False(t, ok, in)
	}
}

// FuzzResolveRef checks the resolver on arbitrary input: it never panics, never returns a SHA
// whose object is absent, and never reads outside the repository.
func FuzzResolveRef(f *testing.F) {
	for _, seed := range []string{"HEAD", "main", "HEAD~1", "HEAD^2", "refs/heads/main", "heads/main", "../x", "a\x00b", "HEAD^{commit}", "~", "^^^^", "@", "x/../../y", "refs/heads/\xff"} {
		f.Add(seed)
	}
	dir, repo, _ := initRepo(f, 3)
	b := newBackend(f)
	f.Fuzz(func(t *testing.T, ref string) {
		sha, err := b.ResolveRef(context.Background(), local(dir), backend.RefName(ref))
		if err != nil {
			return
		}
		_, objErr := repo.CommitObject(plumbing.NewHash(string(sha)))
		require.NoError(t, objErr, "returned %s for %q without its object", sha, ref)
	})
}

func TestListRefsOrderLimitAndPackedRefs(t *testing.T) {
	dir, repo, head := initRepo(t, 2)
	for _, n := range []string{"refs/heads/zeta", "refs/heads/alpha", "refs/heads/Beta", "refs/tags/v1"} {
		require.NoError(t, repo.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(n), head)))
	}
	b := newBackend(t)
	all, err := b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{})
	require.NoError(t, err)
	names := make([]string, len(all))
	for i, r := range all {
		names[i] = string(r)
	}
	assert.True(t, sort.StringsAreSorted(names), "bytewise order, uppercase first: %v", names)
	assert.Equal(t, []string{"refs/heads/Beta", "refs/heads/alpha", "refs/heads/main", "refs/heads/zeta", "refs/tags/v1"}, names)

	limited, err := b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{Limit: 2})
	require.NoError(t, err)
	assert.Len(t, limited, 2)
	heads, err := b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{Pattern: "refs/heads"})
	require.NoError(t, err)
	assert.Len(t, heads, 4)
	_, err = b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{Pattern: "refs/heads/*"})
	assert.True(t, isFallback(err))
	_, err = b.ListRefs(context.Background(), local(dir), backend.ListRefsRequest{Pattern: "-x"})
	assert.ErrorIs(t, err, backend.ErrInvalidArgument)
}
