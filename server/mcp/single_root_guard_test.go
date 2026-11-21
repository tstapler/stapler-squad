package mcp

import (
	"go/ast"
	"go/parser"
	"go/token"
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
//   - any composite literal or new(...) of a type from statefulPackages, unless listed in
//     allowedLiterals;
//   - a dot import of an in-module package (it would hide both of the above).
//
// Import aliases are resolved from each file's import specs, so `gh "…/github"` is seen. Blind
// spots, so nobody reads this as stronger than it is:
//   - methods on values (`store.NewThing()`) and in-package type aliases of a concrete type;
//   - constructors not named New* (`session.Open…`, `Build…`, `Must…`) and function values stored
//     in a variable by another package;
//   - construction through reflection, generics (`pkg.Make[T]`) or a helper in this package that
//     is itself unflagged;
//   - packages outside the module: mcp-go servers and clients legitimately live here, since the
//     proxy and NewCore build the MCP transport itself.
//
// The import side of the rule is the `isolate_server_mcp` depguard row in .golangci.yml.

const modulePrefix = "github.com/tstapler/stapler-squad/"

// allowedConstructors holds every in-module pkg.New* that non-test server/mcp may reference; each
// is a value-type or event constructor, none a store, service or client. No M4b-deferred type is
// constructed here: M4b's concrete types (SessionService, Storage, UserPRCache, BacklogService,
// BacklogFlowService, InstanceStore, EventBus) are only named in signatures and fields and arrive
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

// statefulPackages are the in-module packages whose composite literals are checked. Value-only
// packages (session/domain, backlogdto, config, gen protos, pkg/*) are not: a literal of one of
// those cannot smuggle in a store, service or client.
var statefulPackages = map[string]bool{
	modulePrefix + "session":                     true,
	modulePrefix + "session/instance":            true,
	modulePrefix + "session/git":                 true,
	modulePrefix + "session/flow":                true,
	modulePrefix + "session/backlog/pipeline":    true,
	modulePrefix + "server/services":             true,
	modulePrefix + "server/services/backlogsvc":  true,
	modulePrefix + "server/services/backlogflow": true,
	modulePrefix + "server/wakes":                true,
	modulePrefix + "github":                      true,
}

// allowedLiterals lists the literals of statefulPackages types that are plain value or request
// structs.
var allowedLiterals = map[string]string{
	modulePrefix + "session.ExternalItem":         "value: tracker item",
	modulePrefix + "session.PluginConfig":         "value: plugin config",
	modulePrefix + "github.RepoRef":               "value",
	modulePrefix + "github.AccountRef":            "value",
	modulePrefix + "session/git.CheckRunRef":      "value",
	modulePrefix + "server/wakes.ScheduleRequest": "request value",
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
func scanSingleRoot(t *testing.T, dir string, ctors, lits map[string]string) []guardFinding {
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
		out = append(out, scanFile(fset, f, filepath.Base(path), ctors, lits)...)
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
				what: "dot import of " + p + " hides constructor calls from this guard"})
		case "_":
		default:
			imports[local] = p
		}
	}
	return imports, out
}

func scanFile(fset *token.FileSet, f *ast.File, name string, ctors, lits map[string]string) []guardFinding {
	imports, out := inModuleImports(fset, f, name)
	add := func(p token.Pos, key, what string) {
		out = append(out, guardFinding{file: name, line: fset.Position(p).Line, key: key, what: what})
	}
	// resolve names an in-module package member; id.Obj != nil means a local shadows the import name.
	resolve := func(e ast.Expr) (path, sel string, ok bool) {
		s, isSel := e.(*ast.SelectorExpr)
		if !isSel {
			return "", "", false
		}
		id, isID := s.X.(*ast.Ident)
		if !isID || id.Obj != nil {
			return "", "", false
		}
		p, found := imports[id.Name]
		return p, s.Sel.Name, found
	}
	checkType := func(pos token.Pos, e ast.Expr) {
		p, typ, ok := resolve(e)
		if !ok || !statefulPackages[p] {
			return
		}
		if _, allowed := lits[p+"."+typ]; !allowed {
			add(pos, p+"."+typ, "literal of concrete type "+p+"."+typ)
		}
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if p, sel, ok := resolve(x); ok && strings.HasPrefix(sel, "New") {
				if _, allowed := ctors[p+"."+sel]; !allowed {
					add(x.Pos(), p+"."+sel, "constructor "+p+"."+sel)
				}
			}
		case *ast.CompositeLit:
			checkType(x.Pos(), x.Type)
		case *ast.CallExpr:
			if id, ok := x.Fun.(*ast.Ident); ok && id.Name == "new" && id.Obj == nil && len(x.Args) == 1 {
				checkType(x.Pos(), x.Args[0])
			}
		}
		return true
	})
	return out
}

func TestServerMCP_should_NotConstructConcreteDependencies_When_NonTestSourceScanned(t *testing.T) {
	t.Parallel()
	got := scanSingleRoot(t, ".", allowedConstructors, allowedLiterals)
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
	for k := range allowedLiterals {
		require.True(t, seen[k], "allowedLiterals entry %s is unused: delete it", k)
	}
}

// TestSingleRootScanner_should_FlagEachConstructionShape_When_GivenMutants proves the scanner, not
// just the tree: each case is a mutant of the real source and must be seen.
func TestSingleRootScanner_should_FlagEachConstructionShape_When_GivenMutants(t *testing.T) {
	t.Parallel()
	const hdr = "package mcp\nimport (\n"
	cases := []struct {
		name, imports, body string
		want                string // substring of the finding; "" means no finding expected
	}{
		{"session ctor", `"github.com/tstapler/stapler-squad/session"`, "var _ = session.NewStorage", "session.NewStorage"},
		{"services ctor", `"github.com/tstapler/stapler-squad/server/services"`, "func f() { _ = services.NewSessionService() }", "services.NewSessionService"},
		{"github ctor", `"github.com/tstapler/stapler-squad/github"`, "func f() { _ = github.NewUserPRCache() }", "github.NewUserPRCache"},
		{"nested closure", `"github.com/tstapler/stapler-squad/session"`, "func f() { go func() { func() { _ = session.NewThing() }() }() }", "session.NewThing"},
		{"aliased import", `gh "github.com/tstapler/stapler-squad/github"`, "var _ = gh.NewUserPRCache", "github.NewUserPRCache"},
		{"dot import", `. "github.com/tstapler/stapler-squad/session"`, "var _ = NewStorage", "dot import"},
		{"struct literal", `"github.com/tstapler/stapler-squad/session"`, "var _ = &session.Storage{}", "session.Storage"},
		{"new builtin", `"github.com/tstapler/stapler-squad/server/services"`, "var _ = new(services.SessionService)", "services.SessionService"},
		{"allowed value ctor", `"github.com/tstapler/stapler-squad/github"`, "var _ = github.NewRepoRef", ""},
		{"shadowed import name", `"github.com/tstapler/stapler-squad/session"`, "func f(session struct{ NewX func() }) { session.NewX() }", ""},
		{"method on value (known blind spot)", `"github.com/tstapler/stapler-squad/session"`, "func f(s *session.Storage) { _ = s.NewThing() }", ""},
		{"third-party ctor (out of scope)", `mcpserver "github.com/mark3labs/mcp-go/server"`, "var _ = mcpserver.NewMCPServer", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, "m.go", hdr+tc.imports+"\n)\n"+tc.body+"\n", 0)
			require.NoError(t, err)
			got := scanFile(fset, f, "m.go", allowedConstructors, allowedLiterals)
			if tc.want == "" {
				require.Empty(t, got)
				return
			}
			require.NotEmpty(t, got)
			require.Contains(t, got[0].what, tc.want)
		})
	}
}
