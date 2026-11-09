package termlat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const dashboardDir = "../../docs/observability/grafana/dashboards"

type dashboardFile struct {
	UID    string `json:"uid"`
	Title  string `json:"title"`
	Panels []struct {
		ID      int    `json:"id"`
		Type    string `json:"type"`
		Title   string `json:"title"`
		Targets []struct {
			Expr string `json:"expr"`
		} `json:"targets"`
	} `json:"panels"`
}

func loadDashboards(t *testing.T) map[string]dashboardFile {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dashboardDir, "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, paths)
	out := map[string]dashboardFile{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		require.NoError(t, err)
		var d dashboardFile
		require.NoError(t, json.Unmarshal(raw, &d), p)
		out[filepath.Base(p)] = d
	}
	return out
}

func TestDashboards_should_HaveUniqueUIDsAndPanelIDsAndQueries(t *testing.T) {
	t.Parallel()
	uids := map[string]string{}
	for file, d := range loadDashboards(t) {
		require.NotEmpty(t, d.UID, file)
		assert.NotContains(t, uids, d.UID, "%s reuses uid %q of %s", file, d.UID, uids[d.UID])
		uids[d.UID] = file
		ids := map[int]bool{}
		for _, p := range d.Panels {
			assert.False(t, ids[p.ID], "%s: duplicate panel id %d", file, p.ID)
			ids[p.ID] = true
			if p.Type == "row" {
				continue
			}
			assert.NotEmpty(t, p.Targets, "%s: panel %q has no queries", file, p.Title)
		}
	}
}

// A stage name typo in a query renders as "No data" forever, so every stage the terminal
// dashboard names must be one the recorder accepts.
func TestTerminalExperienceDashboard_should_OnlyQueryStagesTheRecorderKnows(t *testing.T) {
	t.Parallel()
	d, ok := loadDashboards(t)["terminal-experience.json"]
	require.True(t, ok)
	stageRe := regexp.MustCompile(`stage="([a-z_]+)"`)
	seen := 0
	for _, p := range d.Panels {
		for _, tg := range p.Targets {
			assert.NotEmpty(t, tg.Expr, "panel %q has an empty query", p.Title)
			for _, m := range stageRe.FindAllStringSubmatch(tg.Expr, -1) {
				seen++
				assert.Equal(t, m[1], NormalizeStage(m[1]), "panel %q queries unknown stage %q", p.Title, m[1])
			}
		}
	}
	assert.Greater(t, seen, 20, "the dashboard should query many stages")
}
