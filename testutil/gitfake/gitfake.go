// Package gitfake is a recording fake for the refs-cohort methods of backend.Backend, plus a
// router rig that sets the refs cohort to gogit so a test can prove which backend a caller
// reached for Local and Remote locations.
package gitfake

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/session/git/backend"
	"github.com/tstapler/stapler-squad/session/gitprovider"
)

// Call is one recorded backend invocation.
type Call struct {
	Op  backend.OperationName
	Loc backend.RepoLocation
	// Arg is the ref, key or request the call carried, for assertions on what was asked.
	Arg string
}

// Backend answers the refs-cohort methods from the On* funcs and records every call. A method
// whose func is nil returns ErrUnhandled; every other Backend method panics on the nil embedded
// interface, so a caller that strays outside the refs cohort fails loudly.
type Backend struct {
	backend.Backend
	Name string

	OnCurrentBranch func(loc backend.RepoLocation) (backend.BranchName, error)
	OnResolveRef    func(loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error)
	OnRefExists     func(loc backend.RepoLocation, ref backend.RefName) (bool, error)
	OnRepoRoot      func(loc backend.RepoLocation) (backend.RepoRoot, error)
	OnGitDir        func(loc backend.RepoLocation) (backend.GitDir, error)
	OnCommonDir     func(loc backend.RepoLocation) (backend.CommonDir, error)
	OnListRefs      func(loc backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error)
	OnMergeBase     func(loc backend.RepoLocation, req backend.MergeBaseRequest) (backend.CommitSHA, error)
	OnCountCommits  func(loc backend.RepoLocation, rng backend.RangeSpec) (int, error)
	OnLog           func(loc backend.RepoLocation, req backend.LogRequest) ([]backend.LogEntry, error)
	OnGetConfig     func(loc backend.RepoLocation, key backend.ConfigKey) (string, error)

	mu    sync.Mutex
	calls []Call
}

// ErrUnhandled is returned by a refs method with no On* func.
var ErrUnhandled = fmt.Errorf("gitfake: method not stubbed")

// New returns an empty fake; name labels it in failure messages.
func New(name string) *Backend { return &Backend{Name: name} }

func (f *Backend) rec(op backend.OperationName, loc backend.RepoLocation, arg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Op: op, Loc: loc, Arg: arg})
}

// Calls returns a copy of the recorded calls.
func (f *Backend) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// CallsTo returns the recorded calls of one operation.
func (f *Backend) CallsTo(op backend.OperationName) []Call {
	var out []Call
	for _, c := range f.Calls() {
		if c.Op == op {
			out = append(out, c)
		}
	}
	return out
}

func (f *Backend) CurrentBranch(_ context.Context, loc backend.RepoLocation) (backend.BranchName, error) {
	f.rec(backend.OpCurrentBranch, loc, "")
	if f.OnCurrentBranch == nil {
		return "", ErrUnhandled
	}
	return f.OnCurrentBranch(loc)
}

func (f *Backend) ResolveRef(_ context.Context, loc backend.RepoLocation, ref backend.RefName) (backend.CommitSHA, error) {
	f.rec(backend.OpResolveRef, loc, string(ref))
	if f.OnResolveRef == nil {
		return "", ErrUnhandled
	}
	return f.OnResolveRef(loc, ref)
}

func (f *Backend) RefExists(_ context.Context, loc backend.RepoLocation, ref backend.RefName) (bool, error) {
	f.rec(backend.OpRefExists, loc, string(ref))
	if f.OnRefExists == nil {
		return false, ErrUnhandled
	}
	return f.OnRefExists(loc, ref)
}

func (f *Backend) RepoRoot(_ context.Context, loc backend.RepoLocation) (backend.RepoRoot, error) {
	f.rec(backend.OpRepoRoot, loc, "")
	if f.OnRepoRoot == nil {
		return "", ErrUnhandled
	}
	return f.OnRepoRoot(loc)
}

func (f *Backend) GitDir(_ context.Context, loc backend.RepoLocation) (backend.GitDir, error) {
	f.rec(backend.OpGitDir, loc, "")
	if f.OnGitDir == nil {
		return "", ErrUnhandled
	}
	return f.OnGitDir(loc)
}

func (f *Backend) CommonDir(_ context.Context, loc backend.RepoLocation) (backend.CommonDir, error) {
	f.rec(backend.OpCommonDir, loc, "")
	if f.OnCommonDir == nil {
		return "", ErrUnhandled
	}
	return f.OnCommonDir(loc)
}

func (f *Backend) ListRefs(_ context.Context, loc backend.RepoLocation, req backend.ListRefsRequest) ([]backend.RefName, error) {
	f.rec(backend.OpListRefs, loc, string(req.Pattern))
	if f.OnListRefs == nil {
		return nil, ErrUnhandled
	}
	return f.OnListRefs(loc, req)
}

func (f *Backend) MergeBase(_ context.Context, loc backend.RepoLocation, req backend.MergeBaseRequest) (backend.CommitSHA, error) {
	f.rec(backend.OpMergeBase, loc, string(req.Left)+" "+string(req.Right))
	if f.OnMergeBase == nil {
		return "", ErrUnhandled
	}
	return f.OnMergeBase(loc, req)
}

func (f *Backend) CountCommits(_ context.Context, loc backend.RepoLocation, rng backend.RangeSpec) (int, error) {
	f.rec(backend.OpCountCommits, loc, string(rng.Exclude)+".."+string(rng.Include))
	if f.OnCountCommits == nil {
		return 0, ErrUnhandled
	}
	return f.OnCountCommits(loc, rng)
}

func (f *Backend) Log(_ context.Context, loc backend.RepoLocation, req backend.LogRequest) ([]backend.LogEntry, error) {
	f.rec(backend.OpLog, loc, string(req.Range.Exclude)+".."+string(req.Range.Include))
	if f.OnLog == nil {
		return nil, ErrUnhandled
	}
	return f.OnLog(loc, req)
}

func (f *Backend) GetConfig(_ context.Context, loc backend.RepoLocation, key backend.ConfigKey) (string, error) {
	f.rec(backend.OpGetConfig, loc, string(key))
	if f.OnGetConfig == nil {
		return "", ErrUnhandled
	}
	return f.OnGetConfig(loc, key)
}

// Rig is a refs=gogit router over two fakes: CLI serves everything the router sends to the CLI
// (Remote locations, fallbacks), GoGit stands in for the in-process backend.
type Rig struct {
	CLI, GoGit *Backend
	Router     backend.Backend
}

// NewRig builds the router with the refs cohort on gogit and an always-clear preflight.
func NewRig(t testing.TB) *Rig {
	t.Helper()
	rig := &Rig{CLI: New("cli"), GoGit: New("gogit")}
	router, err := backend.NewRouter(backend.RouterConfig{
		Cohorts:   backend.CohortMap{}.With(backend.CohortRefs, backend.BackendGoGit),
		CLI:       rig.CLI,
		GoGit:     rig.GoGit,
		Preflight: func(context.Context, backend.Local, backend.OperationName) []backend.FallbackReason { return nil },
	})
	if err != nil {
		t.Fatalf("gitfake: router: %v", err)
	}
	rig.Router = router
	return rig
}

// Install makes the rig's router the process-wide git backend until the test ends.
func (r *Rig) Install(t testing.TB) {
	t.Helper()
	t.Cleanup(gitprovider.Register(func() backend.Backend { return r.Router }))
}

// RequireOnlyGoGit fails unless every recorded call went to the gogit double with a Local
// location and the CLI double saw nothing.
func (r *Rig) RequireOnlyGoGit(t testing.TB, op backend.OperationName) {
	t.Helper()
	if n := len(r.CLI.Calls()); n != 0 {
		t.Fatalf("refs=gogit Local call reached the CLI backend %d time(s): %+v", n, r.CLI.Calls())
	}
	calls := r.GoGit.CallsTo(op)
	if len(calls) == 0 {
		t.Fatalf("refs=gogit Local call never reached gogit for %s; gogit saw %+v", op, r.GoGit.Calls())
	}
	for _, c := range calls {
		if _, ok := c.Loc.(backend.Local); !ok {
			t.Fatalf("gogit saw a non-Local location %T for %s", c.Loc, op)
		}
	}
}

// RequireOnlyCLIRemote fails unless every recorded call went to the CLI double with a Remote
// location and the gogit double saw nothing.
func (r *Rig) RequireOnlyCLIRemote(t testing.TB, op backend.OperationName) {
	t.Helper()
	if n := len(r.GoGit.Calls()); n != 0 {
		t.Fatalf("a Remote location reached gogit %d time(s): %+v", n, r.GoGit.Calls())
	}
	calls := r.CLI.CallsTo(op)
	if len(calls) == 0 {
		t.Fatalf("Remote call never reached the CLI backend for %s; cli saw %+v", op, r.CLI.Calls())
	}
	for _, c := range calls {
		if _, ok := c.Loc.(backend.Remote); !ok {
			t.Fatalf("CLI saw a non-Remote location %T for %s", c.Loc, op)
		}
	}
}
