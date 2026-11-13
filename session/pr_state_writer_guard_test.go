package session

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// Invariant I1: only the PR status poller writes a PullRequestState row's state
// columns. Instance saves insert a missing row (EnsureState) and never update one,
// so a stale snapshot cannot overwrite a fresher poll.

func stateRowIs(t *testing.T, repo *EntRepository, ref domain.PRRef, state domain.PRState, terminal bool, when string) {
	t.Helper()
	row := reload(t, repo, ref)
	assert.Equal(t, string(state), row.State, "%s: state", when)
	assert.Equal(t, terminal, row.Terminal, "%s: terminal", when)
}

// staleSession adds a second persisted session whose snapshot still holds ref as
// unknown terminal=true, the shape a backfill seeds.
func staleSession(t *testing.T, storage *Storage, title, uuid string, ref domain.PRRef) *Instance {
	t.Helper()
	inst := newTestInstance(title)
	inst.UUID = uuid
	inst.GitHubOwner, inst.GitHubRepo = "acme", "app"
	FinishInstanceConstruction(inst)
	require.NoError(t, inst.AttachPR(ref, domain.AttachSourceBackfill, domain.PRInitial{State: domain.PRStateUnknown, Terminal: true}))
	require.NoError(t, storage.AddInstance(inst)) // create path
	return inst
}

func TestFullSave_ShouldNotOverwritePolledState_WhenAnyFullSavePath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	storage, inst := persistedPRSession(t)
	inst.SetPRPersister(storage)
	ref := mustPRRef(t, "", "acme", "app", 12)

	require.NoError(t, inst.AttachPR(ref, domain.AttachSourceBackfill, domain.PRInitial{State: domain.PRStateUnknown, Terminal: true}))
	stateRowIs(t, storage.Repo, ref, domain.PRStateUnknown, true, "seeded")
	// The poll core arrives in a later wave: apply its observation directly.
	res, err := storage.Repo.UpdatePullRequestState(ctx, ref, fetchObs(PRSourceTick, domain.PRStateOpen, prRepoT0))
	require.NoError(t, err)
	require.True(t, res.Applied, "the poll must be applied for the rest of the test to mean anything")
	stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "polled")
	require.Equal(t, domain.PRStateUnknown, inst.Snapshot().GitHub.PRs[0].State, "the snapshot is stale on purpose")

	t.Run("update path", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, storage.UpdateInstance(inst))
		stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "UpdateInstance")
	})
	t.Run("save instances", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, storage.SaveInstances([]*Instance{inst}))
		stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "SaveInstances")
	})
	t.Run("prPersistPending flush", func(t *testing.T) {
		t.Parallel()
		inst.prPersistPending.Store(true)
		storage.saved.reset() // an unchanged row would otherwise skip the write the flush rides on
		require.NoError(t, storage.SaveInstances([]*Instance{inst}))
		require.False(t, inst.PRPersistPending(), "the flush ran")
		stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "pending flush")
	})
	t.Run("create path for another session with a stale snapshot", func(t *testing.T) {
		t.Parallel()
		staleSession(t, storage, "stale-create", "uuid-stale-create", ref)
		stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "create")
	})
	t.Run("archived session with a stale snapshot", func(t *testing.T) {
		t.Parallel()
		archived := staleSession(t, storage, "stale-archived", "uuid-stale-archived", ref)
		at := prRepoT0
		archived.SetArchivedAt(&at)
		require.NoError(t, storage.UpdateInstance(archived))
		require.NoError(t, storage.SaveInstances([]*Instance{archived}))
		stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "archived")
	})
	t.Run("suspended session with a stale snapshot", func(t *testing.T) {
		t.Parallel()
		suspended := staleSession(t, storage, "stale-suspended", "uuid-stale-suspended", ref)
		require.True(t, suspended.Snapshot().Status.IsSuspended())
		require.NoError(t, storage.SaveInstances([]*Instance{suspended}))
		stateRowIs(t, storage.Repo, ref, domain.PRStateOpen, false, "suspended")
	})
	assert.Equal(t, 1, countStates(t, storage.Repo), "every save shared the one state row")
}

// ---- caller allow-lists (U-35) ---------------------------------------------------

type callerRule struct {
	method  string
	allowed []string // module-relative files that may call it
	why     string
}

// stateWriterRules covers the repository method and the Storage wrapper alike:
// both are called as x.Method(, so one pattern sees every call site.
var stateWriterRules = []callerRule{
	{
		method:  "UpdatePullRequestState",
		allowed: []string{"session/prpoller/pr_status_poller.go", "session/prpoller/pr_state_poller.go" /* the poll core, split out of the poller */, "session/storage.go" /* the Storage wrapper's own delegation */},
		why:     "invariant I1: the PR status poller is the only writer of PullRequestState state columns",
	},
	{
		method:  "ResetVerifyCounters",
		allowed: []string{"session/session_pr_maintenance.go"},
		why:     "only RunPRStartupMaintenance may reset the verify counters of seeded rows (never NewEntRepository or a migration)",
	},
}

// disallowedCallers returns "file:line method" for every call of a rule's method
// outside its allowed files. Comment-only lines are ignored.
func disallowedCallers(sources map[string]string, rules []callerRule) []string {
	var out []string
	for _, rule := range rules {
		call := regexp.MustCompile(`\.` + rule.method + `\(`)
		allowed := make(map[string]bool, len(rule.allowed))
		for _, f := range rule.allowed {
			allowed[f] = true
		}
		for file, src := range sources {
			if allowed[file] {
				continue
			}
			for n, line := range strings.Split(src, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), "//") || !call.MatchString(line) {
					continue
				}
				out = append(out, file+":"+strconv.Itoa(n+1)+" "+rule.method)
			}
		}
	}
	sort.Strings(out)
	return out
}

// nonTestGoSources reads every non-test Go file of the module (skipping generated
// and non-Go trees), keyed by module-relative path.
func nonTestGoSources(t *testing.T) map[string]string {
	t.Helper()
	root := moduleRoot(t)
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if d.IsDir() {
			if skippedLegacyScanDirs[d.Name()] || rel == filepath.Join("session", "ent") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			src, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			out[filepath.ToSlash(rel)] = string(src)
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

func TestStateWriterAllowList_ShouldFail_WhenNewCallerOfUpdatePullRequestState(t *testing.T) {
	t.Parallel()
	t.Run("the repository passes", func(t *testing.T) {
		t.Parallel()
		offenders := disallowedCallers(nonTestGoSources(t), stateWriterRules)
		require.Empty(t, offenders, "invariant I1: only session/prpoller/pr_status_poller.go (and the Storage wrapper) may call "+
			"UpdatePullRequestState, and only RunPRStartupMaintenance may call ResetVerifyCounters. "+
			"Route the write through the poll core, or extend the rule with the reason.")
	})

	t.Run("a new caller of either method fails", func(t *testing.T) {
		t.Parallel()
		sources := map[string]string{
			"session/prpoller/pr_status_poller.go": "func a() { s.UpdatePullRequestState(ctx, ref, obs) }",
			"session/session_pr_maintenance.go":    "func b() { r.ResetVerifyCounters(ctx) }",
			"session/ent_repository.go":            "func c() { r.ResetVerifyCounters(ctx) }",
			"server/some_handler.go":               "func d() {\n\t// calls .UpdatePullRequestState( in a comment only\n\tstorage.UpdatePullRequestState(ctx, ref, obs)\n}",
		}
		got := disallowedCallers(sources, stateWriterRules)
		require.Len(t, got, 2)
		assert.Contains(t, got[0]+got[1], "session/ent_repository.go:1 ResetVerifyCounters")
		assert.Contains(t, got[0]+got[1], "server/some_handler.go:3 UpdatePullRequestState")
	})
}

// ---- the artifact scan never creates a session PR (U-128) ------------------------

func TestOnScanComplete_ShouldNeverCreateSessionPR_WhenFlagOff(t *testing.T) {
	t.Parallel()
	path := filepath.Join(moduleRoot(t), "server", "artifact_scan_complete.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	require.NoError(t, err)

	var handler *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "artifactScanCompleteHandler" {
			handler = fn
		}
	}
	require.NotNil(t, handler, "server/artifact_scan_complete.go no longer defines artifactScanCompleteHandler; update this guard")

	banned := map[string]bool{
		"AttachPR": true, "AttachScannedPR": true, "SetGitHubPR": true, "SetGitHubPRNumber": true,
		"UpdateInstancePRNumber": true, "AttachSessionPR": true, "UpsertSessionPR": true, "PersistPRMirror": true,
	}
	ast.Inspect(handler, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && banned[sel.Sel.Name] {
			t.Errorf("artifactScanCompleteHandler calls %s: a PR URL the scanner merely found must not be polled (ADR-004), whatever the flag", sel.Sel.Name)
		}
		return true
	})
}
