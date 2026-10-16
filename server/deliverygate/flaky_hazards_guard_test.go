package deliverygate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// T-FT-01: this package's tests must stay deterministic (BUG-087/089/100
// hazards): no global slog default, no real sleeps, no real tickers or timers
// started from a test file. Waits are on data (channels), time is injected.
func TestNewGateFlagCacheAndStatsTests_ShouldNotCallSlogSetDefaultNorSwapAGlobalLoggerNorSleepNorStartARealTicker_WhenTheirFilesAreScanned(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]string{
		"slog.SetDefault":                    "swaps the global default logger",
		"log.SetSlogDefaultForTest":          "swaps the repo's global logger",
		"time.Sleep":                         "real sleep",
		"time.NewTicker":                     "real ticker",
		"time.Tick":                          "real ticker",
		"time.NewTimer":                      "real timer",
		"time.AfterFunc":                     "real timer",
		"testutil.WaitForCondition":          "polling wait",
		"testutil.WaitForConditionWithError": "polling wait",
	}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "flaky_hazards_guard_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if why, bad := forbidden[pkg.Name+"."+sel.Sel.Name]; bad {
				t.Errorf("%s: %s.%s is forbidden in this package's tests (%s)", fset.Position(sel.Pos()), pkg.Name, sel.Sel.Name, why)
			}
			return true
		})
	}
}
