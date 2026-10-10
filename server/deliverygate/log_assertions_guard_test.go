package deliverygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"testing"
)

// T-OB-14 (BUG-087): the delivery-gate test suites assert on logs only through
// an injected handler (WithLogger, a warnCollector, a recHandler). Swapping or
// capturing the process-wide logger from a test races every parallel test in
// the package, so no gate-related test file may do it.
func TestLogAssertions_ShouldInjectHandlerAndNeverUseCaptureLogsUnderParallel_WhenScanningDeliveryGateTests(t *testing.T) {
	t.Parallel()
	patterns := []string{
		"*_test.go",
		"../services/*gate*_test.go",
		"../services/gatestats*_test.go",
		"../services/legacy_hidden_counters_test.go",
		"../services/session_service_events*_test.go",
		"../services/hidden_*_test.go",
		"../services/notification_prune_test.go",
		"../services/terminal_*_test.go",
		"../services/ssq_hook_handler_stamp_test.go",
		"../services/slack_gate_test.go",
		"../services/session_service_diagnose_test.go",
	}
	forbidden := map[string]bool{
		"log.SetSlogDefaultForTest": true, // the repo's global-logger swap seam
		"slog.SetDefault":           true,
		"testutil.CaptureLogs":      true,
	}
	seen := map[string]bool{}
	fset := token.NewFileSet()
	for _, pat := range patterns {
		files, err := filepath.Glob(pat)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if seen[f] {
				continue
			}
			seen[f] = true
			parsed, err := parser.ParseFile(fset, f, nil, 0)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			ast.Inspect(parsed, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && forbidden[pkg.Name+"."+sel.Sel.Name] {
					t.Errorf("%s: %s.%s swaps or captures the global logger; inject a handler instead",
						fset.Position(call.Pos()), pkg.Name, sel.Sel.Name)
				}
				return true
			})
		}
	}
	if len(seen) < 10 {
		t.Fatalf("scanned only %d files; the glob patterns drifted", len(seen))
	}
}
