//go:build sinkguard

package services

// Story 5.1d (stream half, PR 5): type-based guards that keep the hidden-session
// read-only guarantee from regrowing. Run through `make test-delivery-guards`.
//
//   (a)  capability-literal lint: the concrete TerminalWriter implementation is
//        constructed only in its defining file.
//   (a2) tmux-subcommand literal lint: send-keys, paste-buffer, load-buffer,
//        resize-window and resize-pane appear only in the two named helpers.
//   (d)  uiWrite R1/R2: every ./server function that touches a UI-stream
//        primitive references a TerminalWriter-typed object.
//
// Checks (b) (RPC descriptors) and (c) (pinned callers) ship with PR 5u, with the
// code they pin. Each check is a function over loaded packages so the negative
// controls run it against testdata/terminalguard and assert it fails there.

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const guardModulePath = "github.com/tstapler/stapler-squad"

// guardConfig names what the checks key on, so the fixture package can reuse them.
type guardConfig struct {
	// writerTypes: a use of an object whose type is one of these names satisfies R1/R2.
	writerTypes map[string]bool
	// capabilityTypes: type name -> the one file that may construct it.
	capabilityTypes      map[string]string
	constructorCallTypes map[string]bool
	// primitiveMethods are UI-stream primitives matched by method name on module types.
	primitiveMethods map[string]bool
	// primitiveFuncs are package-level functions that write to a pane.
	primitiveFuncs map[string]bool
	// seamTypes: calls to any method of these named types are primitives (the seam).
	seamTypes map[string]bool
	// exemptUnits are units (function or "Recv.Method") that may touch primitives
	// without a writer, by name; each has a reason in the real configuration.
	exemptUnits map[string]string
	// tmuxLiterals and the units allowed to contain them.
	tmuxLiterals   map[string]bool
	literalUnits   map[string]bool
	literalPkgSkip []string
}

func realGuardConfig() guardConfig {
	return guardConfig{
		writerTypes: map[string]bool{"TerminalWriter": true, "paneWriter": true},
		capabilityTypes: map[string]string{
			"paneWriter":         "terminal_access.go",
			"HeldLease":          "instance_write_lease.go",
			"steerAuthorization": "steer_authorization.go",
			"BacklogReviewLink":  "hidden_review_steer.go",
		},
		// constructorCallTypes: a call returning one of these is a construction too,
		// so the token has exactly one creating file (a plain conversion or literal
		// is caught separately).
		constructorCallTypes: map[string]bool{"steerAuthorization": true},
		primitiveMethods: map[string]bool{
			"WriteToPTY": true, "SendInputViaControlMode": true, "ResizePTY": true, "ResizePTYContext": true,
			"SetWindowSize": true, "SetWindowSizeContext": true, "RequestResize": true, "ForwardScroll": true,
		},
		primitiveFuncs: map[string]bool{
			"sendInputToTmux": true, "sendInputToTmuxWithRetry": true, "resizeExternalCapturePaneSession": true,
		},
		seamTypes: map[string]bool{"tmuxInputSender": true},
		exemptUnits: map[string]string{
			"sendInputToTmux":                  "definition: holds the send-keys subprocess",
			"sendInputToTmuxWithRetry":         "definition: retries sendInputToTmux",
			"resizeExternalCapturePaneSession": "definition: holds the resize-window and resize-pane subprocesses",
			"realTmuxInputSender.SendInput":    "the one real seam implementation; wraps the helpers",
			"realTmuxInputSender.Resize":       "the one real seam implementation; wraps the helpers",
		},
		tmuxLiterals: map[string]bool{
			"send-keys": true, "paste-buffer": true, "load-buffer": true, "resize-window": true, "resize-pane": true,
		},
		literalUnits: map[string]bool{"sendInputToTmux": true, "resizeExternalCapturePaneSession": true},
		literalPkgSkip: []string{
			guardModulePath + "/session/tmux", guardModulePath + "/session/tymux",
		},
	}
}

func fixtureGuardConfig() guardConfig {
	c := realGuardConfig()
	c.exemptUnits = map[string]string{"sendInputToTmux": "fixture helper"}
	c.literalUnits = map[string]bool{"sendInputToTmux": true}
	return c
}

func loadGuardPackages(t *testing.T, patterns ...string) []*packages.Package {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &packages.Config{
		Dir:   root,
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	for _, p := range pkgs {
		for _, e := range p.Errors {
			t.Fatalf("package %s: %v", p.PkgPath, e)
		}
	}
	if len(pkgs) == 0 {
		t.Fatalf("no packages loaded for %v; the guard would silently pass", patterns)
	}
	return pkgs
}

// allModulePackages returns the loaded roots plus every module package they import.
func allModulePackages(pkgs []*packages.Package) []*packages.Package {
	seen := map[string]bool{}
	var out []*packages.Package
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if seen[p.PkgPath] || p.TypesInfo == nil {
			return
		}
		seen[p.PkgPath] = true
		out = append(out, p)
	})
	return out
}

// unit is one function or function literal, the granularity of checks (a2) and (d).
type unit struct {
	name string // "Func", "Recv.Method", or "Enclosing.func1"
	decl bool
}

// walkUnits calls visit for every node with its innermost unit.
func walkUnits(file *ast.File, visit func(u unit, n ast.Node)) {
	var stack []unit
	litCount := map[string]int{}
	var walk func(n ast.Node)
	walk = func(n ast.Node) {
		ast.Inspect(n, func(node ast.Node) bool {
			switch x := node.(type) {
			case *ast.FuncDecl:
				name := x.Name.Name
				if x.Recv != nil && len(x.Recv.List) == 1 {
					name = recvTypeName(x.Recv.List[0].Type) + "." + name
				}
				stack = append(stack, unit{name: name, decl: true})
				if x.Body != nil {
					walk(x.Body)
				}
				stack = stack[:len(stack)-1]
				return false
			case *ast.ValueSpec:
				// A package-level variable initializer is a unit named for the variable.
				if len(stack) > 0 || len(x.Names) == 0 {
					return true
				}
				stack = append(stack, unit{name: x.Names[0].Name, decl: true})
				for _, v := range x.Values {
					walk(v)
				}
				stack = stack[:len(stack)-1]
				return false
			case *ast.FuncLit:
				parent := "<package>"
				if len(stack) > 0 {
					parent = stack[len(stack)-1].name
				}
				litCount[parent]++
				stack = append(stack, unit{name: parent + ".func" + strconv.Itoa(litCount[parent])})
				walk(x.Body)
				stack = stack[:len(stack)-1]
				return false
			}
			if len(stack) > 0 {
				visit(stack[len(stack)-1], node)
			}
			return true
		})
	}
	for _, d := range file.Decls {
		walk(d)
	}
}

func recvTypeName(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.StarExpr:
		return recvTypeName(x.X)
	case *ast.Ident:
		return x.Name
	case *ast.IndexExpr:
		return recvTypeName(x.X)
	}
	return "?"
}

func namedTypeName(t types.Type) string {
	for {
		switch x := t.(type) {
		case *types.Pointer:
			t = x.Elem()
		case *types.Named:
			return x.Obj().Name()
		default:
			return ""
		}
	}
}

func isModulePkg(p *types.Package) bool {
	return p != nil && strings.HasPrefix(p.Path(), guardModulePath)
}

// primitiveUse reports whether the selector or identifier use is a UI-stream primitive.
func (c guardConfig) primitiveUse(info *types.Info, id *ast.Ident) (string, bool) {
	obj, ok := info.Uses[id].(*types.Func)
	if !ok || !isModulePkg(obj.Pkg()) {
		return "", false
	}
	sig, _ := obj.Type().(*types.Signature)
	if sig != nil && sig.Recv() != nil {
		recv := namedTypeName(sig.Recv().Type())
		if c.seamTypes[recv] {
			return recv + "." + obj.Name(), true
		}
		if c.primitiveMethods[obj.Name()] {
			return recv + "." + obj.Name(), true
		}
		return "", false
	}
	if c.primitiveFuncs[obj.Name()] {
		return obj.Name(), true
	}
	return "", false
}

func (c guardConfig) usesWriter(info *types.Info, id *ast.Ident) bool {
	obj := info.Uses[id]
	if obj == nil {
		return false
	}
	switch obj.(type) {
	case *types.Var:
		return c.writerTypes[namedTypeName(obj.Type())]
	}
	return false
}

// checkUIWrite is check (d): every unit that touches a primitive must reference a
// TerminalWriter-typed object (R1 presence, R2 use). A unit that merely declares a
// writer parameter and never refers to it has no use in its body and fails.
func (c guardConfig) checkUIWrite(pkgs []*packages.Package, pkgFilter func(string) bool) (findings []string, units int) {
	for _, p := range pkgs {
		if !pkgFilter(p.PkgPath) {
			continue
		}
		for _, f := range p.Syntax {
			if strings.HasSuffix(p.Fset.Position(f.Pos()).Filename, "_test.go") {
				continue
			}
			prims := map[string][]string{}
			writers := map[string]bool{}
			walkUnits(f, func(u unit, n ast.Node) {
				id, ok := n.(*ast.Ident)
				if !ok {
					return
				}
				if prim, isPrim := c.primitiveUse(p.TypesInfo, id); isPrim {
					prims[u.name] = append(prims[u.name], prim)
				}
				if c.usesWriter(p.TypesInfo, id) {
					writers[u.name] = true
				}
			})
			for name, used := range prims {
				units++
				if _, exempt := c.exemptUnits[name]; exempt {
					continue
				}
				if !writers[name] {
					findings = append(findings, "ui-write: "+p.PkgPath+"."+name+" calls "+strings.Join(dedupe(used), ", ")+" without referencing a TerminalWriter")
				}
			}
		}
	}
	sort.Strings(findings)
	return findings, units
}

// checkCapabilityLiterals is check (a): the concrete capability types are built only
// in their defining file, by composite literal, new(T), conversion or `var x T`.
func (c guardConfig) checkCapabilityLiterals(pkgs []*packages.Package, pkgFilter func(string) bool) []string {
	var findings []string
	report := func(p *packages.Package, pos token.Pos, how, typ string) {
		file := filepath.Base(p.Fset.Position(pos).Filename)
		if strings.HasSuffix(file, "_test.go") {
			return
		}
		if file == c.capabilityTypes[typ] {
			return
		}
		findings = append(findings, "capability-literal: "+how+" of "+typ+" in "+p.PkgPath+"/"+file)
	}
	for _, p := range pkgs {
		if !pkgFilter(p.PkgPath) {
			continue
		}
		for _, f := range p.Syntax {
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CompositeLit:
					if tv, ok := p.TypesInfo.Types[x]; ok {
						if name := namedTypeName(tv.Type); c.capabilityTypes[name] != "" {
							report(p, x.Pos(), "composite literal", name)
						}
					}
				case *ast.CallExpr:
					tv, ok := p.TypesInfo.Types[x]
					if !ok {
						return true
					}
					name := namedTypeName(tv.Type)
					if c.capabilityTypes[name] == "" {
						return true
					}
					if id, isIdent := x.Fun.(*ast.Ident); isIdent && id.Name == "new" {
						report(p, x.Pos(), "new()", name)
					} else if ftv, isType := p.TypesInfo.Types[x.Fun]; isType && ftv.IsType() {
						report(p, x.Pos(), "conversion", name)
					} else if c.constructorCallTypes[name] {
						report(p, x.Pos(), "constructor call", name)
					}
				case *ast.ValueSpec:
					if x.Type == nil || len(x.Values) > 0 {
						return true
					}
					if tv, ok := p.TypesInfo.Types[x.Type]; ok {
						if name := namedTypeName(tv.Type); c.capabilityTypes[name] != "" {
							report(p, x.Pos(), "var declaration", name)
						}
					}
				}
				return true
			})
		}
	}
	sort.Strings(findings)
	return dedupe(findings)
}

// checkTmuxLiterals is check (a2): the tmux subcommand strings that type into or
// resize a pane appear only inside the named helpers (and the tmux packages).
func (c guardConfig) checkTmuxLiterals(pkgs []*packages.Package, pkgFilter func(string) bool) []string {
	var findings []string
	for _, p := range pkgs {
		if !pkgFilter(p.PkgPath) {
			continue
		}
		skip := false
		for _, prefix := range c.literalPkgSkip {
			if p.PkgPath == prefix || strings.HasPrefix(p.PkgPath, prefix+"/") {
				skip = true
			}
		}
		if skip {
			continue
		}
		for _, f := range p.Syntax {
			if strings.HasSuffix(p.Fset.Position(f.Pos()).Filename, "_test.go") {
				continue
			}
			walkUnits(f, func(u unit, n ast.Node) {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil || !c.tmuxLiterals[val] {
					return
				}
				if c.literalUnits[strings.SplitN(u.name, ".func", 2)[0]] {
					return
				}
				findings = append(findings, "tmux-literal: "+strconv.Quote(val)+" in "+p.PkgPath+"."+u.name)
			})
			// Package-level constants and variables hold literals outside any unit.
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				ast.Inspect(gd, func(n ast.Node) bool {
					if lit, isLit := n.(*ast.BasicLit); isLit && lit.Kind == token.STRING {
						if val, err := strconv.Unquote(lit.Value); err == nil && c.tmuxLiterals[val] {
							findings = append(findings, "tmux-literal: "+strconv.Quote(val)+" in "+p.PkgPath+" (package level)")
						}
					}
					return true
				})
			}
		}
	}
	sort.Strings(findings)
	return dedupe(findings)
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func inModuleProduct(path string) bool {
	return strings.HasPrefix(path, guardModulePath+"/server") ||
		strings.HasPrefix(path, guardModulePath+"/session") ||
		strings.HasPrefix(path, guardModulePath+"/daemon") ||
		strings.HasPrefix(path, guardModulePath+"/cmd") ||
		path == guardModulePath
}

func inServer(path string) bool { return strings.HasPrefix(path, guardModulePath+"/server") }

func inFixture(path string) bool {
	return strings.HasSuffix(path, "/server/services/testdata/terminalguard")
}

func realTreePackages(t *testing.T) []*packages.Package {
	t.Helper()
	return allModulePackages(loadGuardPackages(t, "./server/...", "./session/...", "./daemon/...", "./cmd/...", "."))
}

// T-RO-10: check (d) on the real tree.
func TestUiWriteFunctions_ShouldHaveWriterParamAndUseIt_WhenResolvingThroughInterfaces(t *testing.T) {
	findings, units := realGuardConfig().checkUIWrite(realTreePackages(t), inServer)
	if units == 0 {
		t.Fatal("no UI-stream primitive callers found in ./server; the scan would silently pass")
	}
	t.Logf("check (d): %d units touch a UI-stream primitive", units)
	for _, f := range findings {
		t.Error(f)
	}
}

// T-RO-36: check (a) on the real tree.
func TestCapabilityTypes_ShouldBeConstructedOnlyInTheirDefiningFile_WhenScanned(t *testing.T) {
	cfg := realGuardConfig()
	pkgs := realTreePackages(t)
	for _, f := range cfg.checkCapabilityLiterals(pkgs, inModuleProduct) {
		t.Error(f)
	}
}

// T-RO-12 (a2) on the real tree.
func TestTmuxSubcommandLiterals_ShouldAppearOnlyInTheNamedHelpers_WhenScanned(t *testing.T) {
	cfg := realGuardConfig()
	for _, f := range cfg.checkTmuxLiterals(realTreePackages(t), inModuleProduct) {
		t.Error(f)
	}
}

// T-RO-12 / T-RO-36 negative controls: the same checks fail on the fixture.
func TestGuardChecks_ShouldFail_WhenFixtureHasAUiFunctionThatIgnoresOrLacksTheWriterOrBuildsTheCapability(t *testing.T) {
	pkgs := allModulePackages(loadGuardPackages(t, "./server/services/testdata/terminalguard"))
	cfg := fixtureGuardConfig()

	ui, units := cfg.checkUIWrite(pkgs, inFixture)
	if units == 0 {
		t.Fatal("fixture exercised no units")
	}
	wantUI := []string{"badNoWriter", "badIgnoresWriter", "badMethodValue", "badSeam"}
	for _, name := range wantUI {
		if !containsSubstring(ui, "terminalguard."+name+" ") {
			t.Errorf("check (d) did not flag %s; findings: %v", name, ui)
		}
	}
	for _, name := range []string{"goodInput", "goodSeam", "holder.goodField", "NewWriter", "sendInputToTmux"} {
		if containsSubstring(ui, "terminalguard."+name+" ") {
			t.Errorf("check (d) flagged compliant unit %s: %v", name, ui)
		}
	}

	lits := cfg.checkCapabilityLiterals(pkgs, inFixture)
	for _, how := range []string{"composite literal", "new()", "conversion", "var declaration"} {
		if !containsSubstring(lits, how+" of paneWriter in") {
			t.Errorf("check (a) did not flag a %s outside the defining file; findings: %v", how, lits)
		}
		if !containsSubstring(lits, how+" of steerAuthorization in") {
			t.Errorf("check (a) did not flag a steerAuthorization %s outside steer_authorization.go; findings: %v", how, lits)
		}
	}
	for _, want := range []string{
		"composite literal of HeldLease in",
		"constructor call of steerAuthorization in",
		"composite literal of BacklogReviewLink in",
	} {
		if !containsSubstring(lits, want) {
			t.Errorf("check (a) did not flag %q outside the defining file; findings: %v", want, lits)
		}
	}
	for _, f := range lits {
		for _, defining := range []string{"/terminal_access.go", "/instance_write_lease.go", "/steer_authorization.go", "/hidden_review_steer.go"} {
			if strings.HasSuffix(f, defining) {
				t.Errorf("check (a) flagged the defining file: %s", f)
			}
		}
	}

	tm := cfg.checkTmuxLiterals(pkgs, inFixture)
	if !containsSubstring(tm, `"send-keys" in `) || !containsSubstring(tm, ".badLiteral") {
		t.Errorf("check (a2) did not flag badLiteral; findings: %v", tm)
	}
	if containsSubstring(tm, ".sendInputToTmux") {
		t.Errorf("check (a2) flagged the exempt helper: %v", tm)
	}
}

func containsSubstring(list []string, sub string) bool {
	for _, s := range list {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
