package services

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// filesViewerTestFiles is the fixed list of files-viewer test files the hygiene
// guard scans (BUG-067: they must not make the server/services -race suite slower).
// Files that do not exist yet are skipped so the list can name the whole plan.
var filesViewerTestFiles = []string{
	"content_search_test.go",
	"content_search_handler_test.go",
	"content_search_fixture_test.go",
	"content_search_bench_test.go",
	"file_atomic_test.go",
	"file_codec_test.go",
	"file_write_test.go",
	"file_write_corpus_test.go",
	"files_telemetry_test.go",
	"os_root_semantics_test.go",
	"path_resolver_test.go",
	"streaming_spike_test.go",
	"transport_spike_test.go",
	"transport_bench_test.go",
}

var (
	forbiddenFilesTestCalls = []string{
		"createTestStorage(", "tmux.NewTmux", "NewTmuxSession(", "NewSessionDriver(", `exec.Command("tmux"`,
	}
	largeFixtureCall = regexp.MustCompile(`genFixtureTree\([^,]+,\s*(50000|50_000)\b`)
	topLevelFunc     = regexp.MustCompile(`^func (\w+)`)
)

// scanFilesTestHygiene returns one finding per violation in src, formatted file:line: reason.
// A 50k-file fixture call is allowed in a Benchmark (non-race, make files-viewer-perf) or
// after a testing.Short() check in the same top-level function.
func scanFilesTestHygiene(name, src string) []string {
	var findings []string
	lines := strings.Split(src, "\n")
	funcName, shortGuarded := "", false
	for i, line := range lines {
		if m := topLevelFunc.FindStringSubmatch(line); m != nil {
			funcName, shortGuarded = m[1], false
		}
		if strings.Contains(line, "testing.Short()") {
			shortGuarded = true
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		for _, bad := range forbiddenFilesTestCalls {
			if strings.Contains(line, bad) {
				findings = append(findings, name+":"+strconv.Itoa(i+1)+": forbidden "+bad)
			}
		}
		if largeFixtureCall.MatchString(line) && !strings.HasPrefix(funcName, "Benchmark") && !shortGuarded {
			findings = append(findings, name+":"+strconv.Itoa(i+1)+": 50k fixture in "+funcName+" without a testing.Short() guard")
		}
	}
	return findings
}

func TestFilesViewerGoTests_ShouldNotCallCreateTestStorageOrStartTmux_And_ShortSkipLargeFixtures(t *testing.T) {
	t.Run("detects violations", func(t *testing.T) {
		bad := "package services\nfunc TestX(t *testing.T) {\n\tcreateTestStorage(t)\n\tgenFixtureTree(t, 50000)\n}\n"
		require.Len(t, scanFilesTestHygiene("bad_test.go", bad), 2)
		ok := "package services\nfunc TestX(t *testing.T) {\n\tif testing.Short() {\n\t\tt.Skip()\n\t}\n\tgenFixtureTree(t, 50000)\n}\nfunc BenchmarkY(b *testing.B) {\n\tgenFixtureTree(b, 50000)\n}\n"
		require.Empty(t, scanFilesTestHygiene("ok_test.go", ok))
	})

	for _, name := range filesViewerTestFiles {
		src, err := os.ReadFile(name)
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		require.Empty(t, scanFilesTestHygiene(name, string(src)))
	}
}
