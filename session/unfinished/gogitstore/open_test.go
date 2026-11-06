package gogitstore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/unfinished/internal/testrepo"
)

func openStorer(t *testing.T, reg *Registry, dir string) *WorktreeStorer {
	t.Helper()
	repo, err := Open(dir, reg)
	if err != nil {
		t.Fatalf("Open(%s): %v", dir, err)
	}
	ws, ok := repo.Storer.(*WorktreeStorer)
	if !ok {
		t.Fatalf("storer is %T, want *WorktreeStorer", repo.Storer)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ws
}

func committedRepo(t *testing.T) string {
	t.Helper()
	repo, dir := testrepo.NewRepo(t)
	testrepo.Commit(t, repo, map[string]string{"a.txt": "a"}, "c1", testrepo.Epoch.Add(time.Minute))
	return dir
}

func TestCommonDirKey_ShouldBeIdentical_WhenOpenedViaSymlinkedPaths(t *testing.T) {
	t.Parallel()
	dir := committedRepo(t) // testrepo.TempDir resolves symlinks, so dir is canonical
	link := testrepo.Symlink(t, dir)
	reg := NewRegistry()

	direct := openStorer(t, reg, dir)
	viaLink := openStorer(t, reg, link)

	want := filepath.Join(dir, ".git")
	if got := direct.CommonDirKey(); got != want {
		t.Errorf("direct key = %q, want %q", got, want)
	}
	if got := viaLink.CommonDirKey(); got != want {
		t.Errorf("symlink key = %q, want %q", got, want)
	}
	counts := reg.RefCounts()
	if len(counts) != 1 || counts[want] != 2 {
		t.Errorf("registry = %v, want one entry %q with refcount 2", counts, want)
	}
	if direct.shared.CommonDirKey() != direct.CommonDirKey() {
		t.Error("storer key does not delegate to the store")
	}
}

func TestCommonDirKey_ShouldEqual_WhenMainAndLinkedWorktree(t *testing.T) {
	t.Parallel()
	main := committedRepo(t)
	linked := testrepo.NewLinkedWorktree(t, main, "wt1", "wt-1")
	reg := NewRegistry()

	mainKey := openStorer(t, reg, main).CommonDirKey()
	linkedKey := openStorer(t, reg, linked).CommonDirKey()

	want := filepath.Join(main, ".git")
	if mainKey != want || linkedKey != want {
		t.Errorf("keys main=%q linked=%q, want both %q", mainKey, linkedKey, want)
	}
	if n := len(reg.RefCounts()); n != 1 {
		t.Errorf("registry holds %d entries, want 1", n)
	}
}

func TestCommonDirKey_ShouldEqual_WhenLinkedGitdirPointerIsRelative(t *testing.T) {
	t.Parallel()
	main := committedRepo(t)
	linked := testrepo.NewLinkedWorktree(t, main, "wt1", "wt-1")
	adminDir := filepath.Join(main, ".git", "worktrees", "wt1")
	rel, err := filepath.Rel(linked, adminDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(linked, ".git"), []byte("gitdir: "+rel+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := openStorer(t, NewRegistry(), linked).CommonDirKey()
	if want := filepath.Join(main, ".git"); got != want {
		t.Errorf("key = %q, want %q (pointer %q)", got, want, rel)
	}
}

func TestCommonDirKey_ShouldFallBackAndCount_WhenWorktreeDangling(t *testing.T) {
	// Not parallel: asserts on a process-wide counter delta.
	dir := testrepo.TempDir(t)
	missing := filepath.Join(testrepo.TempDir(t), "gone", "worktrees", "wt1")
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: "+missing+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	before := CanonicalizeFallbackCount()
	_, _, _, key, err := resolveGitFilesystems(dir)
	if err != nil {
		t.Fatalf("resolveGitFilesystems on dangling pointer: %v", err)
	}
	if key != filepath.Clean(missing) {
		t.Errorf("key = %q, want lexical fallback %q", key, filepath.Clean(missing))
	}
	if got := CanonicalizeFallbackCount() - before; got < 1 {
		t.Errorf("fallback counter delta = %d, want >= 1", got)
	}
}

// openRepoEntry must take its key from the store (CommonDirKey), never
// recompute it, or one repo could split into two caches (symlinked paths).
func TestOpenRepoEntry_ShouldNotDeriveKey_WhenOpeningViaGogitstore(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("..", "gogit_vcs_reader.go"), nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var body *ast.BlockStmt
	for _, d := range file.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "openRepoEntry" && fd.Body != nil {
			body = fd.Body
		}
	}
	if body == nil {
		t.Fatal("openRepoEntry not found in gogit_vcs_reader.go")
	}
	calledOpen := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch f := call.Fun.(type) {
		case *ast.Ident:
			name = f.Name
		case *ast.SelectorExpr:
			name = f.Sel.Name
		}
		switch name {
		case "gitCommonDir", "resolveGitFilesystems":
			t.Errorf("%s: openRepoEntry derives a key via %s", fset.Position(call.Pos()), name)
		case "Open":
			calledOpen = true
		}
		return true
	})
	if !calledOpen {
		t.Error("openRepoEntry no longer calls gogitstore.Open; test is vacuous")
	}
}
