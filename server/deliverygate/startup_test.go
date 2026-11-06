package deliverygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// T-BF-12: the gate's startup/restore tests build a narrow service (a storage,
// a bus and NewSessionService/newGatedSessionService) and never call
// server.BuildDependencies or NewServerWithDeps, which wire ~30 production
// subsystems and make real outbound network calls under test (see the CI
// hermetic-testing notes in server/dependencies_test.go).
func TestStartup_ShouldUseNarrowServiceDepsAndNeverCallBuildDependencies_WhenHiddenColdRestore(t *testing.T) {
	t.Parallel()
	patterns := []string{
		"*_test.go",
		"../services/*gate*_test.go",
		"../services/gatestats*_test.go",
		"../services/legacy_hidden_counters_test.go",
		"../services/session_service_events*_test.go",
		"../services/hidden_*_test.go",
		"../services/session_service_diagnose_test.go",
		"../slack_gate_test.go",
		"../gatestats_listener_test.go",
	}
	forbidden := map[string]bool{"BuildDependencies": true, "NewServerWithDeps": true}
	scanned := 0
	fset := token.NewFileSet()
	for _, pat := range patterns {
		files, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			scanned++
			parsed, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			ast.Inspect(parsed, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := ""
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					name = fn.Name
				case *ast.SelectorExpr:
					name = fn.Sel.Name
				}
				if forbidden[name] {
					t.Errorf("%s: calls %s; build a narrow dependency set instead", fset.Position(call.Pos()), name)
				}
				return true
			})
		}
	}
	if scanned < 10 {
		t.Fatalf("scanned only %d files; the glob patterns drifted", scanned)
	}
}
