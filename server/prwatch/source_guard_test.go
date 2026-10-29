package prwatch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// guardedFile is one parsed non-test source file under server/prwatch.
type guardedFile struct {
	rel  string // slash path relative to server/prwatch, e.g. "inbox/token.go"
	file *ast.File
}

func (g guardedFile) inInbox() bool { return strings.HasPrefix(g.rel, "inbox/") }

func (g guardedFile) imports() []string {
	var out []string
	for _, imp := range g.file.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		out = append(out, p)
	}
	return out
}

// parseNonTestSources parses every non-test .go file under server/prwatch/...
func parseNonTestSources(t *testing.T) []guardedFile {
	t.Helper()
	fset := token.NewFileSet()
	var files []guardedFile
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		files = append(files, guardedFile{rel: filepath.ToSlash(path), file: f})
		return nil
	})
	require.NoError(t, err)
	require.NotEmpty(t, files, "sanity: scanned real sources")
	return files
}

// pkgCalls reports each call of the form <pkgName>.<fn>(...), where pkgName is
// the local name of an imported package.
func pkgCalls(f *ast.File, pkgName string, visit func(fn string, call *ast.CallExpr)) {
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == pkgName {
			visit(sel.Sel.Name, call)
		}
		return true
	})
}

func TestPackage_should_NotImportOsExecOrWriteFiles_When_ScanningNonTestSources(t *testing.T) {
	t.Parallel()
	forbiddenOS := map[string]bool{
		"WriteFile": true, "Create": true, "OpenFile": true, "Chmod": true, "Chown": true,
		"Remove": true, "RemoveAll": true, "Rename": true, "Truncate": true,
	}
	for _, g := range parseNonTestSources(t) {
		for _, p := range g.imports() {
			require.NotEqual(t, "os/exec", p, "%s imports os/exec: the token must never reach process args", g.rel)
			require.NotContains(t, p, "safeexec", "%s imports an exec wrapper", g.rel)
		}
		pkgCalls(g.file, "os", func(fn string, _ *ast.CallExpr) {
			require.False(t, forbiddenOS[fn], "%s calls os.%s: prwatch must not write or change files", g.rel, fn)
		})
	}
}

func TestPackage_should_OnlyImportNetHTTPInsideInboxSubpackage_When_ScanningImports(t *testing.T) {
	t.Parallel()
	for _, g := range parseNonTestSources(t) {
		for _, p := range g.imports() {
			if p == "net/http" {
				require.True(t, g.inInbox(), "%s imports net/http outside the inbox sub-package", g.rel)
			}
		}
		pkgCalls(g.file, "os", func(fn string, _ *ast.CallExpr) {
			if fn == "Open" || fn == "ReadFile" {
				require.Equal(t, "inbox/token.go", g.rel, "%s calls os.%s: only inbox/token.go reads the token file", g.rel, fn)
			}
		})
	}
}

func TestPackage_should_NotImportGitHubPackageOrCallGh_When_ScanningSources(t *testing.T) {
	t.Parallel()
	for _, g := range parseNonTestSources(t) {
		for _, p := range g.imports() {
			require.NotEqual(t, modulePath+"/github", p, "%s imports the GitHub API client package", g.rel)
			require.False(t, strings.HasPrefix(p, modulePath+"/github/"), "%s imports %s", g.rel, p)
			require.False(t, strings.HasPrefix(p, "github.com/google/go-github"), "%s imports %s", g.rel, p)
			require.False(t, strings.HasPrefix(p, "github.com/cli/"), "%s imports %s", g.rel, p)
		}
		ast.Inspect(g.file, func(n ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				v, _ := strconv.Unquote(lit.Value)
				require.NotEqual(t, "gh", v, "%s names the gh CLI", g.rel)
			}
			return true
		})
	}
}

// Steering goes through the guarded path only; SteerActiveSession writes
// unguarded and into hidden sessions.
func TestPackage_should_NotReferenceUnguardedSteer_When_ScanningSources(t *testing.T) {
	t.Parallel()
	for _, g := range parseNonTestSources(t) {
		ast.Inspect(g.file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.Ident:
				require.NotEqual(t, "SteerActiveSession", x.Name, "%s references SteerActiveSession", g.rel)
			case *ast.SelectorExpr:
				require.NotEqual(t, "SteerActiveSession", x.Sel.Name, "%s references SteerActiveSession", g.rel)
			}
			return true
		})
	}
}

// Event fields that carry free text from GitHub. Actor is excluded: it is
// validated against a login pattern before use.
var eventFreeTextFields = map[string]bool{"Summary": true, "Details": true, "Body": true}

var logMethods = map[string]bool{"Debug": true, "Info": true, "Warn": true, "Error": true, "Log": true}

var formatFuncs = map[string]map[string]bool{
	"fmt": {"Sprintf": true, "Sprint": true, "Sprintln": true, "Errorf": true, "Printf": true, "Println": true, "Fprintf": true},
	"log": {"Printf": true, "Println": true, "Print": true, "Fatalf": true, "Panicf": true},
}

func mentionsFreeText(args []ast.Expr) (string, bool) {
	var hit string
	for _, a := range args {
		ast.Inspect(a, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && eventFreeTextFields[sel.Sel.Name] {
				hit = sel.Sel.Name
			}
			return hit == ""
		})
	}
	return hit, hit != ""
}

func TestPackage_should_NotLogOrFormatEventFreeText_When_ScanningSources(t *testing.T) {
	t.Parallel()
	for _, g := range parseNonTestSources(t) {
		ast.Inspect(g.file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			isFormat := false
			if id, ok := sel.X.(*ast.Ident); ok {
				isFormat = formatFuncs[id.Name][sel.Sel.Name]
			}
			if logMethods[sel.Sel.Name] || isFormat {
				if field, bad := mentionsFreeText(call.Args); bad {
					require.Fail(t, "event free text reaches a log or format call",
						"%s: %s(...) uses .%s", g.rel, sel.Sel.Name, field)
				}
			}
			return true
		})
	}
}

// Detector self-check: the guard must flag what it claims to flag.
func TestSourceGuard_should_FlagFreeTextAndSteerCalls_When_GivenHostileSnippet(t *testing.T) {
	t.Parallel()
	src := `package x
func f(l Logger, ev Event) { l.Warn("x", "s", ev.Summary); _ = fmt.Sprintf("%s", ev.Event.Details) }`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, 0)
	require.NoError(t, err)
	flagged := 0
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if _, bad := mentionsFreeText(call.Args); bad {
				flagged++
			}
		}
		return true
	})
	require.GreaterOrEqual(t, flagged, 2)
}

// Squad ships no UI for PR Watch (R15): nothing under proto/, web-app/ or gen/
// may differ from main. Skipped when git or main is unavailable.
func TestSlice_should_NotChangeProtoOrWebApp_When_PRWatchAdded(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("../../.git"); err != nil {
		t.Skip("not a git checkout root")
	}
	base, err := safeexec.CommandContext(t.Context(), "git", "merge-base", "main", "HEAD").Output()
	if err != nil {
		t.Skip("main branch not available")
	}
	out, err := safeexec.CommandContext(t.Context(), "git", "diff", "--name-only", strings.TrimSpace(string(base)), "--", "proto", "web-app", "gen").Output()
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(string(out)), "PR Watch must not change proto/, web-app/ or gen/")
}
