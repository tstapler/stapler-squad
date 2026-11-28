package services

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Only human-driven RPC handlers may record a manual nudge; PR-fix, wake and
// every other automated steer must stay off this list. Exact, so a handler that
// stops recording (or moves) fails too.
func TestManualNudgeRecorders_should_BeCalledOnlyFromHumanDrivenHandlers(t *testing.T) {
	t.Parallel()
	want := []string{
		"backlogsvc/backlog_service_lifecycle.go", // TransitionBacklogItemStatus, user actor only
		"githubrpc/github_user_nudge.go",          // "Ask to fix" NudgeSessionForPR RPC
		"session_service_lifecycle.go",            // RestartSession RPC
		"terminal_service.go",                     // WriteToSession RPC
	}
	var got []string
	for _, dir := range servicesSourceDirs(t) {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		require.NoError(t, err)
		for _, f := range files {
			if strings.HasSuffix(f, "_test.go") {
				continue
			}
			b, err := os.ReadFile(f)
			require.NoError(t, err)
			if strings.Contains(string(b), ".RecordManualNudgeFor") {
				got = append(got, filepath.ToSlash(f))
			}
		}
	}
	sort.Strings(got)
	assert.Equal(t, want, got)
}
