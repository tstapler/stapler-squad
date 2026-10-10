//go:build sinkguard

package deliverygate

// T-FL-07 (constraint C-3): rollout flags are live-settable only. A scan fails
// when a function that reads one of the rollout flags (or any function of this
// package) also reads the process environment.

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

var rolloutFlagNames = map[string]bool{
	"hidden_session_gate":            true,
	"hidden_session_readonly_guards": true,
	"notification_tray_v2":           true,
}

// rolloutFlagIdents are the identifiers that carry those names.
var rolloutFlagIdents = map[string]bool{
	"HiddenSessionGateFeatureFlag": true,
}

func isEnvRead(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "os" && (sel.Sel.Name == "Getenv" || sel.Sel.Name == "LookupEnv")
}

func mentionsRolloutFlag(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(x ast.Node) bool {
		switch v := x.(type) {
		case *ast.BasicLit:
			if v.Kind == token.STRING {
				if s, err := strconv.Unquote(v.Value); err == nil && rolloutFlagNames[s] {
					found = true
				}
			}
		case *ast.Ident:
			if rolloutFlagIdents[v.Name] {
				found = true
			}
		case *ast.SelectorExpr:
			if rolloutFlagIdents[v.Sel.Name] {
				found = true
			}
		}
		return !found
	})
	return found
}

func TestFlags_ShouldHaveNoEnvVarReads_WhenScanningGateAndTrayFlagCode(t *testing.T) {
	pkgs := loadPackages(t, "./server/...", "./config/...", "./pkg/events/...")
	var violations []string
	for _, p := range pkgs {
		if strings.HasSuffix(p.PkgPath, fixturePkgSuffix) {
			continue
		}
		inGate := p.PkgPath == modulePath+"/server/deliverygate"
		for _, f := range p.Syntax {
			checkFileForEnvReads(p, f, inGate, &violations)
		}
	}
	for _, v := range violations {
		t.Error(v)
	}
}

func checkFileForEnvReads(p *packages.Package, f *ast.File, inGate bool, out *[]string) {
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}
		readsFlag := inGate || mentionsRolloutFlag(fd.Body)
		if !readsFlag {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && isEnvRead(call) {
				*out = append(*out, "env var read in a rollout-flag code path: "+p.PkgPath+"."+fd.Name.Name)
			}
			return true
		})
	}
}
