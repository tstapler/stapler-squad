package unfinished

// Whole-tree enforcement of the go-git-only policy under session/unfinished/**
// (and tools/scanner-workload/** once it exists). Imports are parsed, not
// grepped, and nothing here spawns a process.

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// execImports are the packages whose import marks a file as able to run a
// subprocess. Matching on the "executor/safeexec" suffix covers the module path.
var execImports = []string{"os/exec"}

const safeexecSuffix = "executor/safeexec"

// reexecAllowList maps an exact slash-separated path (relative to
// session/unfinished) to the reason it may import exec/safeexec. Each entry is
// itself verified by verifyReexecAllowEntry.
var reexecAllowList = map[string]string{
	"gogitstore/crash_subprocess_test.go": "re-execs the test binary (os.Args[0]) to crash a child mid-write; never runs git",
}

// noNativeGitRoots are walked in addition to "." (session/unfinished).
var noNativeGitRoots = []string{".", filepath.Join("..", "..", "tools", "scanner-workload")}

func isExecImport(path string) bool {
	for _, e := range execImports {
		if path == e {
			return true
		}
	}
	return path == safeexecSuffix || strings.HasSuffix(path, "/"+safeexecSuffix)
}

func TestNoNativeGitInPackage(t *testing.T) {
	var offenders []string
	allowSeen := map[string]bool{}

	for _, root := range noNativeGitRoots {
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("stat %s: %v", root, err)
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			rel := filepath.ToSlash(path)
			found, perr := execImportsIn(path)
			if perr != nil {
				t.Errorf("parse %s: %v", rel, perr)
				return nil
			}
			for _, ip := range found {
				if _, ok := reexecAllowList[rel]; ok {
					allowSeen[rel] = true
					continue
				}
				offenders = append(offenders, rel+" imports "+ip)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	sort.Strings(offenders)
	for _, o := range offenders {
		t.Errorf("native-git capable import (migrate to internal/testrepo / go-git): %s", o)
	}

	for rel := range reexecAllowList {
		if !allowSeen[rel] {
			t.Errorf("allow-list entry %s no longer imports exec/safeexec; delete the entry", rel)
			continue
		}
		verifyReexecAllowEntry(t, rel)
	}
}

func execImportsIn(path string) ([]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, imp := range f.Imports {
		if ip, uerr := strconv.Unquote(imp.Path.Value); uerr == nil && isExecImport(ip) {
			found = append(found, ip)
		}
	}
	return found, nil
}

// verifyReexecAllowEntry checks the allow-listed file cannot run git: no "git"
// string literal, and every Command* call targets os.Args[0].
func verifyReexecAllowEntry(t *testing.T, rel string) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.FromSlash(rel), nil, 0)
	if err != nil {
		t.Errorf("parse allow-listed %s: %v", rel, err)
		return
	}
	commandCalls := 0
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.BasicLit:
			if x.Kind != token.STRING {
				return true
			}
			if v, uerr := strconv.Unquote(x.Value); uerr == nil && (v == "git" || strings.HasPrefix(v, "git ")) {
				t.Errorf("%s: allow-listed file contains git string literal %s", rel, x.Value)
			}
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok || !strings.HasPrefix(sel.Sel.Name, "Command") {
				return true
			}
			target := 0
			if strings.Contains(sel.Sel.Name, "Context") {
				target = 1
			}
			commandCalls++
			if len(x.Args) <= target || exprString(fset, x.Args[target]) != "os.Args[0]" {
				t.Errorf("%s: %s must target os.Args[0]", rel, fset.Position(x.Pos()))
			}
		}
		return true
	})
	if commandCalls == 0 {
		t.Errorf("%s: allow-listed file has no Command* call; delete the entry", rel)
	}
}

func exprString(fset *token.FileSet, e ast.Expr) string {
	var sb strings.Builder
	if err := printer.Fprint(&sb, fset, e); err != nil {
		return ""
	}
	return sb.String()
}
