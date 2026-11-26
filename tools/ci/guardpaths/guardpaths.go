// Package guardpaths holds existence checks for guards keyed by file path, package or symbol that go
// silently inert when the thing they name moves out of top-level server/services. Each check returns
// problems rather than failing, so its test can run it against a broken fixture as well as the tree
// (project_plans/package-split-ocp/session-service-hub-plan.md, section 7).
package guardpaths

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	archivalAnalyzer = "tools/lint/noarchivedrevival/analyzer.go"
	liveRawAnalyzer  = "tools/lint/noliveinstanceraw/analyzer.go"
	goTestBzl        = "tools/bzl/go_test.bzl"
	compareAffected  = "tools/ci/compare-affected/compare.go"
	servicesDir      = "server/services"
)

func parseFile(fsys fs.FS, name string) (*ast.File, error) {
	src, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return parser.ParseFile(token.NewFileSet(), name, src, parser.ParseComments)
}

// varLiteral returns the composite literal assigned to the package-level var name.
func varLiteral(f *ast.File, name string) *ast.CompositeLit {
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, id := range vs.Names {
				if lit, ok := vs.Values[i].(*ast.CompositeLit); ok && id.Name == name {
					return lit
				}
			}
		}
	}
	return nil
}

func stringLit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}

// sliceStrings returns the string elements of the slice literal var name.
func sliceStrings(f *ast.File, name string) []string {
	var out []string
	if lit := varLiteral(f, name); lit != nil {
		for _, el := range lit.Elts {
			if s, ok := stringLit(el); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// mapKeyStrings returns the string keys of the map literal var name.
func mapKeyStrings(f *ast.File, name string) []string {
	var out []string
	if lit := varLiteral(f, name); lit != nil {
		for _, el := range lit.Elts {
			if kv, ok := el.(*ast.KeyValueExpr); ok {
				if s, ok := stringLit(kv.Key); ok {
					out = append(out, s)
				}
			}
		}
	}
	return out
}

// CheckArchivedRevivalFiles reports every path in noarchivedrevival's lifecycleFiles that does not exist:
// the analyzer matches by path suffix, so a moved file is silently unscanned.
func CheckArchivedRevivalFiles(fsys fs.FS) []string {
	f, err := parseFile(fsys, archivalAnalyzer)
	if err != nil {
		return []string{err.Error()}
	}
	files := sliceStrings(f, "lifecycleFiles")
	if len(files) == 0 {
		return []string{archivalAnalyzer + ": no lifecycleFiles literal found; update guardpaths if it was renamed"}
	}
	var problems []string
	for _, p := range files {
		if _, err := fs.Stat(fsys, p); err != nil {
			problems = append(problems, fmt.Sprintf("noarchivedrevival lifecycleFiles lists %s, which does not exist; re-key the entry in %s (and .claude/rules/noarchivedrevival.md)", p, archivalAnalyzer))
		}
	}
	return problems
}

// CheckLiveInstanceRawScope reports a monitored method (or FindLiveInstance) of noliveinstanceraw that no
// non-test file at or under server/services declares: the analyzer only runs in that tree.
func CheckLiveInstanceRawScope(fsys fs.FS) []string {
	f, err := parseFile(fsys, liveRawAnalyzer)
	if err != nil {
		return []string{err.Error()}
	}
	want := mapKeyStrings(f, "monitoredFuncNames")
	if len(want) == 0 {
		return []string{liveRawAnalyzer + ": no monitoredFuncNames literal found; update guardpaths if it was renamed"}
	}
	want = append(want, "FindLiveInstance")
	declared := declaredMethods(fsys)
	var problems []string
	for _, name := range want {
		if !declared[name] {
			problems = append(problems, fmt.Sprintf("noliveinstanceraw guards method %s, but no non-mock %s method on a SessionService or Handler receiver declares it; the analyzer is inert for it (extend its scope or the watchlist in %s)", name, servicesDir, liveRawAnalyzer))
		}
	}
	return problems
}

var bzlRowKey = regexp.MustCompile(`(?m)^    "([^"]+)":`)

// liveRawReceivers are the receiver types that may implement the noliveinstanceraw watchlist. Today only
// SessionService; Handler is the type the hub plan gives the sessionrpc, sessioncreate and sessionstream leaves
// (project_plans/package-split-ocp/session-service-hub-plan.md, section 2). Mock receivers in internal/svctest
// never count.
var liveRawReceivers = map[string]bool{"SessionService": true, "Handler": true}

// declaredMethods returns the names of methods declared on a liveRawReceivers type in non-test files at or
// under server/services, outside internal/svctest and testdata.
func declaredMethods(fsys fs.FS) map[string]bool {
	declared := map[string]bool{}
	_ = fs.WalkDir(fsys, servicesDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "testdata" || strings.HasSuffix(p, "internal/svctest") {
				return fs.SkipDir
			}
			return nil
		}
		if !isProdGo(p) {
			return nil
		}
		pf, perr := parseFile(fsys, p)
		if perr != nil {
			return nil
		}
		for _, decl := range pf.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv != nil && liveRawReceivers[receiverName(fd)] {
				declared[fd.Name.Name] = true
			}
		}
		return nil
	})
	return declared
}

func receiverName(fd *ast.FuncDecl) string {
	if len(fd.Recv.List) == 0 {
		return ""
	}
	t := fd.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

func isProdGo(p string) bool {
	return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go")
}

// CheckGoTestBzlRows reports a per-package row of go_test.bzl whose directory is gone or holds no tests,
// since a moved package silently falls back to default size, shards and cache tags.
func CheckGoTestBzlRows(fsys fs.FS) []string {
	src, err := fs.ReadFile(fsys, goTestBzl)
	if err != nil {
		return []string{err.Error()}
	}
	rows := bzlRowKey.FindAllStringSubmatch(string(src), -1)
	if len(rows) == 0 {
		return []string{goTestBzl + ": no package rows found; update guardpaths if the table changed shape"}
	}
	var problems []string
	for _, m := range rows {
		if !hasTestFile(fsys, m[1]) {
			problems = append(problems, fmt.Sprintf("%s has a row for package %q with no *_test.go files there; move or drop the row", goTestBzl, m[1]))
		}
	}
	return problems
}

func hasTestFile(fsys fs.FS, dir string) bool {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), "_test.go") {
			return true
		}
	}
	return false
}

var hasTargetInCall = regexp.MustCompile(`hasTargetIn\(([^)]*)\)`)

// CheckCompareAffectedTargets reports a Bazel package prefix compare-affected keys on that has no directory.
func CheckCompareAffectedTargets(fsys fs.FS) []string {
	src, err := fs.ReadFile(fsys, compareAffected)
	if err != nil {
		return []string{err.Error()}
	}
	var problems []string
	seen := 0
	for _, call := range hasTargetInCall.FindAllStringSubmatch(string(src), -1) {
		for _, lit := range regexp.MustCompile(`"(//[^"]+)"`).FindAllStringSubmatch(call[1], -1) {
			seen++
			dir := strings.TrimPrefix(lit[1], "//")
			if dir == "" {
				continue
			}
			if info, err := fs.Stat(fsys, dir); err != nil || !info.IsDir() {
				problems = append(problems, fmt.Sprintf("compare-affected keys on %s, which is not a directory; update %s", lit[1], compareAffected))
			}
		}
	}
	if seen == 0 {
		problems = append(problems, compareAffected+": no hasTargetIn(\"//...\") prefixes found; update guardpaths if the call changed")
	}
	return problems
}

// CheckSvctestMain reports a directory under server/services whose non-test code imports session/driver
// (so tests start drivers) but whose tests never call svctest.Main, which is what detects leaked drivers.
// Direct imports only: a package that starts drivers through another leaf is not seen.
func CheckSvctestMain(fsys fs.FS) []string {
	const driverImport = `"github.com/tstapler/stapler-squad/session/driver"`
	startsDrivers, hasMain := scanDriverStarters(fsys, driverImport)
	var problems []string
	for dir := range startsDrivers {
		if !hasMain[dir] {
			problems = append(problems, fmt.Sprintf("%s imports session/driver but no test file calls svctest.Main; add a testmain_test.go so leaked drivers fail the package (this check sees direct imports only, so a leaf that starts drivers through another package is not caught)", dir))
		}
	}
	sort.Strings(problems)
	return problems
}

// scanDriverStarters returns the directories whose non-test code imports driverImport and those whose
// tests call svctest.Main. Parsed, so a commented-out import or call does not count. The svctest helper
// package and testdata are skipped.
func scanDriverStarters(fsys fs.FS, driverImport string) (startsDrivers, hasMain map[string]bool) {
	startsDrivers, hasMain = map[string]bool{}, map[string]bool{}
	_ = fs.WalkDir(fsys, servicesDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "testdata" || strings.HasSuffix(p, "internal/svctest") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		f, perr := parseFile(fsys, p)
		if perr != nil {
			return nil
		}
		dir := path.Dir(p)
		if !isProdGo(p) {
			if callsSvctestMain(f) {
				hasMain[dir] = true
			}
		} else if importsPath(f, driverImport) {
			startsDrivers[dir] = true
		}
		return nil
	})
	return startsDrivers, hasMain
}

func importsPath(f *ast.File, quoted string) bool {
	for _, imp := range f.Imports {
		if imp.Path.Value == quoted {
			return true
		}
	}
	return false
}

func callsSvctestMain(f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Main" {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "svctest" {
					found = true
				}
			}
		}
		return !found
	})
	return found
}
