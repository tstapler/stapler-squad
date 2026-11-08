package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var ssqMetricRe = regexp.MustCompile(`ssq_[a-z0-9_]+`)

// goMetricNames collects every ssq_* name that appears as a string literal in the
// repo's non-test Go source.
func goMetricNames(t *testing.T, root string) map[string]bool {
	t.Helper()
	names := map[string]bool{}
	for _, dir := range []string{"session", "server", "github", "telemetry"} {
		require.NoError(t, filepath.WalkDir(filepath.Join(root, dir), func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			src, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			for _, m := range ssqMetricRe.FindAllString(string(src), -1) {
				names[m] = true
			}
			return nil
		}))
	}
	return names
}

func TestDashboard_ShouldParseAndReferenceOnlyExistingMetrics(t *testing.T) {
	t.Parallel()
	root := ".."
	raw, err := os.ReadFile(filepath.Join(root, "docs", "observability", "grafana", "dashboards", "session-pr-tracking.json"))
	require.NoError(t, err)
	var dash struct {
		Panels     []struct{ Targets []struct{ Expr string } }
		AlertRules []struct {
			Severity string
			Expr     string
		}
	}
	require.NoError(t, json.Unmarshal(raw, &dash), "dashboard must parse")

	known := goMetricNames(t, root)
	var exprs []string
	for _, p := range dash.Panels {
		for _, tg := range p.Targets {
			exprs = append(exprs, tg.Expr)
		}
	}
	for _, a := range dash.AlertRules {
		exprs = append(exprs, a.Expr)
	}
	require.NotEmpty(t, exprs)
	for _, expr := range exprs {
		for _, name := range ssqMetricRe.FindAllString(expr, -1) {
			base := strings.TrimSuffix(name, "_bucket")
			assert.True(t, known[base], "%s in %q has no definition in Go source", name, expr)
		}
	}
	require.Len(t, dash.AlertRules, 2)
	for _, a := range dash.AlertRules {
		assert.Equal(t, "warning", a.Severity)
	}
}
