package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Single-root guard (owner decision Q5, project_plans/package-split-ocp/mcp-single-root-plan.md):
// server/mcp is not a composition root; server/dependencies.go (and server.RunStdioMCP for stdio)
// builds every concrete service, store and client and passes it in. depguard is import-level only
// and cannot say "no New* call", so this test walks the non-test source of this package.
//
// Flagged, per non-test file, in every closure and nested expression:
//   - any selector pkg.New* where pkg is an in-module import (called or taken as a value), unless
//     listed in allowedConstructors;
//   - any composite literal or new(...) of an in-module type, including `&T{}`, generic `T[int]{}`
//     and elided element literals (`[]*T{{}}`, `map[K]T{k: {}}`), unless its package is in
//     valueOnlyPackages or the type is in allowedTypes;
//   - a `var x T` declaration of such a type (a zero value is a construction; fields, parameters
//     and results are not flagged: M4b's signatures legitimately name concrete types);
//   - a dot import of an in-module package (it would hide all of the above).
//
// A subdirectory holding non-test .go files fails a separate test (the scan is not recursive).
//
// Import aliases are resolved from each file's import specs, so `gh "…/github"` is seen. Shadowing
// is detected through ast.Ident.Obj, which parser.ParseFile fills by default (deprecated API); if
// that resolution ever stops, the "shadowed import name" mutant row fails first. Remaining
// blind spots, so nobody reads this as stronger than it is:
//   - constructors not named New*: `session.OpenStorage`, `config.LoadConfig` (wake_tool_deps.go
//     already calls it), `localtoken.LoadOrCreate`, `Build…`, `Must…`;
//   - methods and method values (`store.NewThing()`, `f := store.NewThing`) and function values
//     stored in a variable by another package;
//   - in-package aliases (`type X = session.Storage`) and generic helper functions in this package
//     (`func make[T any]() T`) that build a concrete type for a caller;
//   - array, slice and channel constructions that name the type only as an element:
//     `var x [N]pkg.T`, `make([]pkg.T, n)`, `make(chan pkg.T)`, and `pkg.T(x)` conversions;
//   - a local generic instantiated with a stateful type argument (`newBox[pkg.T]()`);
//   - `var x pkg.T` is flagged for every T, including an interface or func type, which holds no
//     zero-value state. An allowedTypes row for such a type is labelled "value" but is really an
//     interface: read the label as "may be declared", not as proof the type is plain data;
//   - reflection, and constructions inside a type switch or var-declared interface value whose
//     concrete type is chosen elsewhere;
//   - packages outside the module: mcp-go servers and clients legitimately live here, since the
//     proxy and NewCore build the MCP transport itself;
//   - GOOS-specific files: depguard on linux does not see imports in other GOOS files, though
//     this scan parses every non-test file in the directory regardless of build tags.
//
// The import side of the rule is the `isolate_server_mcp` depguard row in .golangci.yml. It
// rejects new imports but, unlike leaf rows, nothing forces its list to stay free of unused
// entries; tightness is kept by the plan's `go list` counts.

const modulePrefix = "github.com/tstapler/stapler-squad/"

// allowedConstructors holds every in-module pkg.New* that non-test server/mcp may reference; each
// is a value-type or event constructor, none a store, service or client. No M4b-deferred type is
// constructed here: M4b's concrete types (SessionService, Storage, UserPRCache, BacklogService,
// InstanceStore, EventBus; BacklogFlowService left this list in M4b-1) are only named in signatures and fields and arrive
// already built.
var allowedConstructors = map[string]string{
	modulePrefix + "github.NewRepoRef":                 "value object parsed from request data",
	modulePrefix + "github.NewRepoRefWithHost":         "value object parsed from request data",
	modulePrefix + "session/domain.NewGuidanceRequest": "domain value constructor",
	modulePrefix + "session/domain.NewQuestionPayload": "domain value constructor",
	modulePrefix + "session/domain.NewWakeAction":      "domain value constructor",
	modulePrefix + "pkg/events.NewNotificationEvent":   "event value constructor",
	modulePrefix + "pkg/events.NewSessionUpdatedEvent": "event value constructor",
}

// valueOnlyPackages hold only data types, so a literal of any of their types cannot smuggle in a
// store, service or client. Every other in-module type needs an allowedTypes row.
var valueOnlyPackages = map[string]bool{
	modulePrefix + "session/domain":            true,
	modulePrefix + "session/domain/backlogdto": true,
	modulePrefix + "gen/proto/go/session/v1":   true,
}

// allowedTypes lists the literals and zero-value declarations of types from other in-module
// packages that are plain value or request structs.
var allowedTypes = map[string]string{
	modulePrefix + "session.ExternalItem":                      "value: tracker item",
	modulePrefix + "session.PluginConfig":                      "value: plugin config",
	modulePrefix + "github.RepoRef":                            "value",
	modulePrefix + "github.AccountRef":                         "value",
	modulePrefix + "session/backlog/pipeline.TriageSuggestion": "value: JSON decode target",
	modulePrefix + "session/backlog/pipeline.TriageTask":       "value: JSON decode target",
	modulePrefix + "session/git.CheckRunRef":                   "value",
	modulePrefix + "server/wakes.ScheduleRequest":              "request value",
}

type guardFinding struct {
	file, key, what string
	line            int
}

func (f guardFinding) String() string {
	return f.file + ":" + strconv.Itoa(f.line) + ": " + f.what
}

// scanSingleRoot parses the non-test .go files in dir and reports every construction of a concrete
// in-module type that the allow-lists do not cover.
func scanSingleRoot(t *testing.T, dir string, ctors, types map[string]string) []guardFinding {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	require.NoError(t, err)
	var out []guardFinding
	scanned := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		scanned++
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err, path)
		out = append(out, scanFile(fset, f, filepath.Base(path), ctors, types)...)
	}
	require.Positive(t, scanned, "guard saw no source files in %s: its data glob is missing", dir)
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// inModuleImports maps each in-module import's local name to its path and reports dot imports.
func inModuleImports(fset *token.FileSet, f *ast.File, name string) (map[string]string, []guardFinding) {
	imports := map[string]string{}
	var out []guardFinding
	for _, spec := range f.Imports {
		p, _ := strconv.Unquote(spec.Path.Value)
		if !strings.HasPrefix(p, modulePrefix) {
			continue
		}
		local := filepath.Base(p)
		if spec.Name != nil {
			local = spec.Name.Name
		}
		switch local {
		case ".":
			out = append(out, guardFinding{file: name, line: fset.Position(spec.Pos()).Line, key: p + ".",
				what: "dot import of " + p + " hides constructions from this guard"})
		case "_":
		default:
			imports[local] = p
		}
	}
	return imports, out
}

// unwrapType strips parens, pointers and generic instantiation: `*pkg.T[int]` -> `pkg.T`.
func unwrapType(e ast.Expr) ast.Expr {
	for {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.IndexExpr:
			e = x.X
		case *ast.IndexListExpr:
			e = x.X
		default:
			return e
		}
	}
}

type scanner struct {
	fset    *token.FileSet
	name    string
	imports map[string]string
	ctors   map[string]string
	types   map[string]string
	out     []guardFinding
}

func (s *scanner) add(p token.Pos, key, what string) {
	s.out = append(s.out, guardFinding{file: s.name, line: s.fset.Position(p).Line, key: key, what: what})
}

// resolve names an in-module package member; id.Obj != nil means a local shadows the import name.
func (s *scanner) resolve(e ast.Expr) (path, sel string, ok bool) {
	sx, isSel := e.(*ast.SelectorExpr)
	if !isSel {
		return "", "", false
	}
	id, isID := sx.X.(*ast.Ident)
	if !isID || id.Obj != nil {
		return "", "", false
	}
	p, found := s.imports[id.Name]
	return p, sx.Sel.Name, found
}

// checkType flags a construction of the named type unless its package is value-only or it is allowed.
func (s *scanner) checkType(pos token.Pos, typ ast.Expr, what string) {
	p, name, ok := s.resolve(unwrapType(typ))
	if !ok || valueOnlyPackages[p] {
		return
	}
	if _, allowed := s.types[p+"."+name]; !allowed {
		s.add(pos, p+"."+name, what+" of concrete type "+p+"."+name)
	}
}

// elided checks the element literals written without a type (`{}`) inside a literal of type typ.
func (s *scanner) elided(typ ast.Expr, elts []ast.Expr) {
	var keyT, valT ast.Expr
	switch t := unwrapType(typ).(type) {
	case *ast.ArrayType:
		valT = t.Elt
	case *ast.MapType:
		keyT, valT = t.Key, t.Value
	default:
		return
	}
	check := func(e, et ast.Expr) {
		if lit, ok := e.(*ast.CompositeLit); ok && lit.Type == nil && et != nil {
			s.checkType(lit.Pos(), et, "elided literal")
			s.elided(et, lit.Elts)
		}
	}
	for _, e := range elts {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			check(kv.Key, keyT)
			check(kv.Value, valT)
			continue
		}
		check(e, valT)
	}
}

func (s *scanner) visit(n ast.Node) bool {
	switch x := n.(type) {
	case *ast.SelectorExpr:
		if p, sel, ok := s.resolve(x); ok && strings.HasPrefix(sel, "New") {
			if _, allowed := s.ctors[p+"."+sel]; !allowed {
				s.add(x.Pos(), p+"."+sel, "constructor "+p+"."+sel)
			}
		}
	case *ast.CompositeLit:
		if x.Type != nil {
			s.checkType(x.Pos(), x.Type, "literal")
			s.elided(x.Type, x.Elts)
		}
	case *ast.CallExpr:
		if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "new" && id.Obj == nil && len(x.Args) == 1 {
			s.checkType(x.Pos(), x.Args[0], "new()")
		}
	case *ast.ValueSpec:
		// `var x T` or `var x T = ...` of a non-pointer named type builds a zero value.
		if x.Type != nil {
			if _, isPtr := x.Type.(*ast.StarExpr); !isPtr {
				s.checkType(x.Pos(), x.Type, "zero-value declaration")
			}
		}
	}
	return true
}

func scanFile(fset *token.FileSet, f *ast.File, name string, ctors, types map[string]string) []guardFinding {
	imports, out := inModuleImports(fset, f, name)
	s := &scanner{fset: fset, name: name, imports: imports, ctors: ctors, types: types, out: out}
	ast.Inspect(f, s.visit)
	return s.out
}

func TestServerMCP_should_NotConstructConcreteDependencies_When_NonTestSourceScanned(t *testing.T) {
	t.Parallel()
	got := scanSingleRoot(t, ".", allowedConstructors, allowedTypes)
	require.Empty(t, got, "server/mcp is not a composition root (Q5): build the dependency in "+
		"server/dependencies.go and inject it through a CoreOption or NewCore parameter")
}

func TestServerMCP_should_KeepAllowListsExact_When_SourceScanned(t *testing.T) {
	t.Parallel()
	// With empty allow-lists every use is a finding, so each allow-list row must appear in one.
	seen := map[string]bool{}
	for _, fnd := range scanSingleRoot(t, ".", nil, nil) {
		seen[fnd.key] = true
	}
	for k := range allowedConstructors {
		require.True(t, seen[k], "allowedConstructors entry %s is unused: delete it", k)
	}
	for k := range allowedTypes {
		require.True(t, seen[k], "allowedTypes entry %s is unused: delete it", k)
	}
}

func TestServerMCP_should_HaveNoNonTestSourceInSubdirectories_When_Scanned(t *testing.T) {
	t.Parallel()
	var nested []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." {
			// The scan and the depguard row are flat; a subpackage needs both extended first.
			ents, _ := filepath.Glob(filepath.Join(path, "*.go"))
			for _, e := range ents {
				if !strings.HasSuffix(e, "_test.go") {
					nested = append(nested, e)
				}
			}
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, nested, "single-root guard and isolate_server_mcp only cover server/mcp/*.go")
}

// TestSingleRootScanner_should_FlagEachConstructionShape_When_GivenMutants proves the scanner, not
// just the tree: each case is a mutant of the real source and must be seen.
func TestSingleRootScanner_should_FlagEachConstructionShape_When_GivenMutants(t *testing.T) {
	t.Parallel()
	const sess = `"github.com/tstapler/stapler-squad/session"`
	const hdr = "package mcp\nimport (\n"
	cases := []struct {
		name, imports, body string
		want                string // substring of the finding; "" means no finding expected
	}{
		{"session ctor", sess, "var _ = session.NewStorage", "session.NewStorage"},
		{"services ctor", `"github.com/tstapler/stapler-squad/server/services"`, "func f() { _ = services.NewSessionService() }", "services.NewSessionService"},
		{"github ctor", `"github.com/tstapler/stapler-squad/github"`, "func f() { _ = github.NewUserPRCache() }", "github.NewUserPRCache"},
		{"nested closure", sess, "func f() { go func() { func() { _ = session.NewThing() }() }() }", "session.NewThing"},
		{"aliased import", `gh "github.com/tstapler/stapler-squad/github"`, "var _ = gh.NewUserPRCache", "github.NewUserPRCache"},
		{"dot import", `. "github.com/tstapler/stapler-squad/session"`, "var _ = NewStorage", "dot import"},
		{"struct literal", sess, "var _ = &session.Storage{}", "session.Storage"},
		{"new builtin", `"github.com/tstapler/stapler-squad/server/services"`, "var _ = new(services.SessionService)", "services.SessionService"},
		{"elided slice of pointers", sess, "var _ = []*session.Storage{{}}", "elided literal of concrete type"},
		{"elided map value", sess, `var _ = map[string]session.Storage{"a": {}}`, "elided literal of concrete type"},
		{"elided map key", sess, `var _ = map[session.Storage]int{{}: 1}`, "elided literal of concrete type"},
		{"elided nested", sess, "var _ = [][]session.Storage{{{}}}", "elided literal of concrete type"},
		{"generic literal", sess, "var _ = session.Foo[int]{}", "session.Foo"},
		{"generic multi-arg literal", sess, "var _ = &session.Foo[int, string]{}", "session.Foo"},
		{"zero-value var", sess, "var x session.Storage", "zero-value declaration"},
		{"zero-value var in func", `"github.com/tstapler/stapler-squad/session/instance"`, "func f() { var y instance.Instance; _ = y }", "instance.Instance"},
		{"event bus literal", `"github.com/tstapler/stapler-squad/pkg/events"`, "var _ = &events.EventBus{}", "events.EventBus"},
		{"config literal", `"github.com/tstapler/stapler-squad/config"`, "var _ = config.Config{}", "config.Config"},
		{"allowed value ctor", `"github.com/tstapler/stapler-squad/github"`, "var _ = github.NewRepoRef", ""},
		{"allowed value literal", `"github.com/tstapler/stapler-squad/github"`, "var _ = github.RepoRef{}", ""},
		{"value-only package literal", `"github.com/tstapler/stapler-squad/session/domain"`, "var _ = domain.GuidanceRequest{}", ""},
		{"parenthesised zero-value var", sess, "var x (session.Storage)", "zero-value declaration"},
		{"pointer var is not a construction", sess, "var x *session.Storage", ""},
		{"param and field are not constructions", sess, "type h struct{ s session.Storage }\nfunc f(s session.Storage) {}", ""},
		{"shadowed import name", sess, "func f(session struct{ NewX func() }) { session.NewX() }", ""},
		{"method on value (known blind spot)", sess, "func f(s *session.Storage) { _ = s.NewThing() }", ""},
		{"non-New ctor (known blind spot)", sess, "var _ = session.OpenStorage", ""},
		{"alias (known blind spot)", sess, "type X = session.Storage\nvar _ = X{}", ""},
		{"third-party ctor (out of scope)", `mcpserver "github.com/mark3labs/mcp-go/server"`, "var _ = mcpserver.NewMCPServer", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "m.go", hdr+tc.imports+"\n)\n"+tc.body+"\n", 0)
			require.NoError(t, err)
			got := scanFile(fset, f, "m.go", allowedConstructors, allowedTypes)
			if tc.want == "" {
				require.Empty(t, got)
				return
			}
			require.NotEmpty(t, got)
			require.Contains(t, got[0].what, tc.want)
		})
	}
}
