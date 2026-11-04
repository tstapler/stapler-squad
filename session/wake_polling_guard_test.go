package session

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// scheduleWakeupToken is the client-side wake tool agents must not use. It is
// spelled in two parts so this file, and every test that needs the word, does
// not count as a reference itself.
const scheduleWakeupToken = "Schedule" + "Wakeup"

// pollingReferences is the number of lines per Go file that mention the
// client-side wake tool (counts verified 2026-10-17). Every one is a prompt
// that forbids the tool, a test of that text, or the receipt counter
// (agent_schedule_wakeup_calls_total) that measures its use. New polling loops would add a
// file or raise a count: add durable waits with schedule_wake instead, and
// reach for this map only when a new prompt must forbid the tool.
var pollingReferences = map[string]int{
	"server/mcp/tools_backlog.go":                          6,
	"server/mcp/tools_backlog_test.go":                     2,
	"server/mcp/tools_backlog_wait_cap_test.go":            3,
	"server/services/backlog_service_verdict_steer.go":     2,
	"server/services/backlog_stuck_rpc_test.go":            1,
	"server/services/guidance_ack_wiring.go":               4, // PostToolUse receipt counter, not a prompt
	"server/services/notification_quality_metrics.go":      7, // agent_schedule_wakeup_calls_total
	"server/services/notification_quality_metrics_test.go": 6,
	"session/backlog_commands.go":                          1,
	"session/backlog_context.go":                           1,
	"session/backlog_post_review_prompt_test.go":           1,
	"session/domain/backlog.go":                            1,
	"session/pipeline_mode_seed.go":                        2,
}

func TestPollingToolReferencesInGo_should_MatchTheExplicitPerFileMap(t *testing.T) {
	root := moduleRoot(t)
	got := map[string]int{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".claude", "node_modules", "third_party", "gen": // .claude holds gitignored agent worktree checkouts
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		n, countErr := countLinesWith(path, scheduleWakeupToken)
		if countErr != nil {
			return countErr
		}
		if n > 0 {
			rel, _ := filepath.Rel(root, path)
			got[filepath.ToSlash(rel)] = n
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, pollingReferences, got,
		"a Go file mentions the client-side wake tool that the map does not allow; prompts should point agents at schedule_wake instead")
}

// moduleRoot is the directory holding go.mod, found by walking up from the package.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above the package")
		dir = parent
	}
}

func countLinesWith(path, token string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		if strings.Contains(sc.Text(), token) {
			n++
		}
	}
	return n, sc.Err()
}
