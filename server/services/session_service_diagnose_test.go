package services

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

func TestResolveReviewTarget_ShouldRunInWorkWorktreeAndNameBranch_WhenWorktreeExists(t *testing.T) {
	t.Parallel()
	wtDir := t.TempDir()
	got := resolveReviewTarget("/shared/repo", domain.GitWorktreeData{WorktreePath: wtDir, BranchName: "backlog/my-item", BaseCommitSHA: "abc123"})
	assert.Equal(t, wtDir, got.Dir)
	assert.Contains(t, got.Preamble, "abc123..backlog/my-item")
}

func TestResolveReviewTarget_ShouldForbidHEADAndNameBranch_WhenWorktreeGone(t *testing.T) {
	t.Parallel()
	got := resolveReviewTarget("/shared/repo", domain.GitWorktreeData{WorktreePath: filepath.Join(t.TempDir(), "gone"), BranchName: "backlog/my-item", BaseCommitSHA: "abc123"})
	assert.Equal(t, "/shared/repo", got.Dir)
	assert.Contains(t, got.Preamble, "abc123..backlog/my-item")
	assert.Contains(t, got.Preamble, "NOT that branch")
}

func TestResolveReviewTarget_ShouldUseRepoPathWithoutPreamble_WhenNoBranchRecorded(t *testing.T) {
	t.Parallel()
	got := resolveReviewTarget("/shared/repo", domain.GitWorktreeData{})
	assert.Equal(t, reviewTarget{Dir: "/shared/repo"}, got)
}

// T-IX-16 (index half): a hidden review/triage/diagnose spawn is resolvable by
// the gate, with its kind, as soon as CreateDirectorySession returns.
func TestCreateDirectorySession_ShouldUpsertHiddenKind_WhenDiagnoseTriageOrReviewSpawn(t *testing.T) {
	cases := []struct {
		tag  string
		want deliverygate.HiddenKind
	}{
		{"backlog:review", deliverygate.KindReview},
		{"backlog:triage", deliverygate.KindTriage},
		{"backlog:diagnose", deliverygate.KindDiagnose},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			svc, _, gate := newCountingGateService(t)
			title := "gate-index-" + string(c.want)
			inst, err := svc.CreateDirectorySession(context.Background(), t.TempDir(), SessionSpawnOptions{
				Title: title, OneShot: true, Hidden: true, Tags: []string{c.tag},
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = inst.Destroy() })

			for _, key := range []string{title, inst.Snapshot().UUID} {
				res := gate.Resolver().Resolve(key, nil)
				assert.Equal(t, deliverygate.VisibilityHidden, res.Visibility, key)
				assert.Equal(t, c.want, res.Kind, key)
			}
		})
	}
}

// T-IX-16 (ordering half): the gate index is fed before Instance.Start launches
// the agent, so the session's first hook cannot hit an un-indexed (fail-open)
// lookup. Source-order guard on each spawn entry point (Spike 1.3e).
func TestSpawnEntryPoints_ShouldIndexBeforeStart_WhenCreatingHiddenSessions(t *testing.T) {
	fset := token.NewFileSet()
	files := []string{"session_service_diagnose.go", "session_service_create.go"}
	spawners := map[string]bool{"CreateDirectorySession": false, "CreateWorktreeSession": false}
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, 0)
		require.NoError(t, err)
		for _, d := range f.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if _, want := spawners[fn.Name.Name]; !want {
				continue
			}
			var indexPos, startPos token.Pos
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				switch sel.Sel.Name {
				case "indexSessionForDelivery":
					if indexPos == 0 {
						indexPos = call.Pos()
					}
				case "Start":
					if startPos == 0 {
						startPos = call.Pos()
					}
				}
				return true
			})
			require.NotZero(t, indexPos, "%s must call indexSessionForDelivery", fn.Name.Name)
			require.NotZero(t, startPos, "%s must call Start", fn.Name.Name)
			assert.Less(t, indexPos, startPos, "%s must index before Start", fn.Name.Name)
			spawners[fn.Name.Name] = true
		}
	}
	for name, seen := range spawners {
		assert.True(t, seen, "%s not found in scanned files", name)
	}
}

// spawnReviewPassesRepoPath reports whether fnName in src passes a `.RepoPath`
// selector as the directory argument of CreateDirectorySession.
func spawnReviewPassesRepoPath(t *testing.T, src, fnName string) bool {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "src.go", src, 0)
	require.NoError(t, err)
	found, bad := false, false
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != fnName {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); !ok || sel.Sel.Name != "CreateDirectorySession" || len(call.Args) < 2 {
				return true
			}
			if sel, ok := call.Args[1].(*ast.SelectorExpr); ok && sel.Sel.Name == "RepoPath" {
				bad = true
			}
			return true
		})
	}
	require.True(t, found, "%s not found", fnName)
	return bad
}

// Structural guard for the "review runs in the shared checkout" class: the review
// spawn must take its directory from resolveReviewTarget, never item.RepoPath
// directly (the shared checkout's HEAD is an arbitrary branch).
func TestSpawnReviewSession_ShouldNotPassItemRepoPathToCreateDirectorySession(t *testing.T) {
	src, err := os.ReadFile("session_service_diagnose.go")
	require.NoError(t, err)
	assert.False(t, spawnReviewPassesRepoPath(t, string(src), "SpawnReviewSession"),
		"SpawnReviewSession passes item.RepoPath to CreateDirectorySession; use resolveReviewTarget")
}

func TestSpawnReviewPassesRepoPath_ShouldDetectOldPassThrough(t *testing.T) {
	t.Parallel()
	old := `package x
func (s *S) SpawnReviewSession() {
	s.CreateDirectorySession(ctx, item.RepoPath, opts)
}`
	assert.True(t, spawnReviewPassesRepoPath(t, old, "SpawnReviewSession"))
}

func TestReviewTargetFor_ShouldResolveWorkWorktreeFromStorage_WhenItemSessionHasWorktree(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	ctx := context.Background()
	wtDir := t.TempDir()
	const workUUID = "work-session-uuid-r1"
	require.NoError(t, storage.CreateInstanceData(ctx, domain.InstanceData{
		Title:     "r1-work-" + t.Name(),
		UUID:      workUUID,
		Path:      t.TempDir(),
		Status:    domain.Active,
		Program:   "claude",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Worktree:  domain.GitWorktreeData{WorktreePath: wtDir, SessionName: "r1-work", BranchName: "backlog/r1-item", BaseCommitSHA: "base123", RepoPath: "/shared/repo"},
	}))
	item, err := storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "r1 item", RepoPath: "/shared/repo"})
	require.NoError(t, err)
	is, err := storage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: workUUID, SessionRole: session.SessionRoleWork})
	require.NoError(t, err)

	svc := &SessionService{storage: storage, concStorage: storage}
	got := svc.reviewTargetFor(ctx, &backlogdto.BacklogItemData{ID: item.ID, RepoPath: "/shared/repo"}, is.ID)

	assert.Equal(t, wtDir, got.Dir)
	assert.Contains(t, got.Preamble, "base123..backlog/r1-item")
}

func TestReviewTargetFor_ShouldFallBackToRepoPath_WhenItemSessionUnknown(t *testing.T) {
	t.Parallel()
	storage := svctest.CreateTestStorage(t)
	svc := &SessionService{storage: storage, concStorage: storage}
	got := svc.reviewTargetFor(context.Background(), &backlogdto.BacklogItemData{ID: "x", RepoPath: "/shared/repo"}, "no-such-item-session")
	assert.Equal(t, reviewTarget{Dir: "/shared/repo"}, got)
}
