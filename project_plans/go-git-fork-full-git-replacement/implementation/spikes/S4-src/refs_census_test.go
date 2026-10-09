package s4lock

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
)

func readF(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestT9_RefsAbortAndContention(t *testing.T) {
	commit := func(sc *Scope) error {
		_, e := sc.WT.Commit("x", &git.CommitOptions{Author: sig(), Committer: sig(), All: true})
		return e
	}

	// (a) abort after commit object written + ref buffered, before commit phase
	d := fixture(t)
	refP, idxP, headP := filepath.Join(d, ".git/refs/heads/master"), filepath.Join(d, ".git/index"), filepath.Join(d, ".git/HEAD")
	r0, i0, h0 := readF(t, refP), readF(t, idxP), readF(t, headP)
	var wrote bool
	err := WithIndexLock(d, Options{BeforeCommitPhase: func() error { return errors.New("object_missing(injected)") }},
		func(sc *Scope) error { e := commit(sc); wrote = sc.Wrote(); return e })
	if err == nil {
		t.Fatal("want err")
	}
	if readF(t, refP) != r0 || readF(t, idxP) != i0 || readF(t, headP) != h0 {
		t.Fatal("(a) ref/index/HEAD changed after abort")
	}
	noLocks(t, d)
	t.Logf("T9a abort-before-commit-phase: refs/index/HEAD byte-identical, no locks, Wrote()=%v", wrote)

	// (b) foreign ref lock
	d = fixture(t)
	refP, idxP = filepath.Join(d, ".git/refs/heads/master"), filepath.Join(d, ".git/index")
	r0, i0 = readF(t, refP), readF(t, idxP)
	os.WriteFile(refP+".lock", []byte("foreign"), 0o644)
	err = WithIndexLock(d, Options{}, commit)
	var el *ErrLocked
	if !errors.As(err, &el) {
		t.Fatalf("(b) want ErrLocked got %v", err)
	}
	if _, serr := os.Stat(filepath.Join(d, ".git/index.lock")); serr == nil {
		t.Fatal("(b) index.lock leaked")
	}
	if readF(t, refP+".lock") != "foreign" || readF(t, refP) != r0 || readF(t, idxP) != i0 {
		t.Fatal("(b) state changed")
	}
	t.Logf("T9b foreign refs/heads/master.lock: %v ; our index.lock removed, foreign lock untouched, ref+index unchanged", err)

	// (c) success path + fsck + CLI sees the commit
	d = fixture(t)
	if err := WithIndexLock(d, Options{}, commit); err != nil {
		t.Fatal(err)
	}
	noLocks(t, d)
	if _, e, err := gitRun(d, "fsck", "--no-dangling"); err != nil {
		t.Fatal(e)
	}
	t.Logf("T9c success: HEAD subject=%q, status=%q", cg(t, d, "log", "-1", "--format=%s"), cg(t, d, "status", "--porcelain"))

	// (d) CLI commit started inside the window between index rename and ref rename
	d = fixture(t)
	var cliOut string
	var cliErr error
	err = WithIndexLock(d, Options{BetweenRenames: func() {
		o, e, er := gitRun(d, "commit", "-q", "--allow-empty", "-m", "racer")
		cliOut, cliErr = o+e, er
	}}, commit)
	if err != nil {
		t.Fatal(err)
	}
	noLocks(t, d)
	t.Logf("T9d CLI commit inside index->ref window: err=%v msg=%q ; final subject=%q", cliErr, strings.TrimSpace(cliOut), cg(t, d, "log", "-1", "--format=%s"))
	if cliErr == nil {
		t.Errorf("CLI commit unexpectedly succeeded inside the window")
	}
}

func TestT10_Census(t *testing.T) {
	out, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}", "github.com/go-git/go-git/v5").Output()
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(out))
	var got []string
	fset := token.NewFileSet()
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if strings.HasPrefix(rel, "storage/") || strings.HasPrefix(rel, "plumbing/") || strings.HasPrefix(rel, "_examples") || strings.HasPrefix(rel, "cli/") {
			return nil
		}
		f, perr := parser.ParseFile(fset, p, nil, 0)
		if perr != nil {
			return nil
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}
			ast.Inspect(fd, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || (sel.Sel.Name != "Index" && sel.Sel.Name != "SetIndex") {
					return true
				}
				// receiver must end in .Storer
				if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Storer" {
					pos := fset.Position(call.Pos())
					got = append(got, rel+":"+strconv.Itoa(pos.Line)+" "+fd.Name.Name+" "+sel.Sel.Name)
				}
				return true
			})
		}
		return nil
	})
	sort.Strings(got)
	t.Logf("T10 census (%d call sites, outside tests/storage/plumbing):\n%s", len(got), strings.Join(got, "\n"))
	if len(got) != 23 {
		t.Errorf("census count %d != 23 (incl. status.go:122 and 3 submodule reads)", len(got))
	}
}

