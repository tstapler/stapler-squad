package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeHistoryJSONL writes lines directly to a file path.
func writeHistoryJSONL(t *testing.T, path string, lines []string) {
	t.Helper()
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	for _, line := range lines {
		_, err = fmt.Fprintf(f, "%s\n", line)
		require.NoError(t, err)
	}
}

// historyLine builds a single history.jsonl line from parts.
func historyLine(display, project, sessionID string, tsMs int64) string {
	return fmt.Sprintf(
		`{"display":%q,"timestamp":%d,"project":%q,"sessionId":%q}`,
		display, tsMs, project, sessionID,
	)
}

// ---- Reload() tests --------------------------------------------------------

func TestReload_MissingFile_ReturnsNilErrorAndEmptyEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.jsonl")

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 0, sh.Count())
}

func TestReload_SingleSessionSingleMessage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	tsMs := int64(1764129737334)
	writeHistoryJSONL(t, path, []string{
		historyLine("fix the login bug", "/home/user/myproject", "session-abc-123", tsMs),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	require.Equal(t, 1, sh.Count())

	entries := sh.GetAll()
	require.Len(t, entries, 1)
	e := entries[0]

	assert.Equal(t, "session-abc-123", e.ID)
	assert.Equal(t, "fix the login bug", e.Name)
	assert.Equal(t, "/home/user/myproject", e.Project)
	assert.Equal(t, time.UnixMilli(tsMs), e.CreatedAt)
	assert.Equal(t, time.UnixMilli(tsMs), e.UpdatedAt)
	assert.Equal(t, 1, e.MessageCount)
	// Model is always empty from history.jsonl
	assert.Equal(t, "", e.Model)
}

func TestReload_SingleSessionMultipleMessages_NameFromEarliest_UpdatedAtFromLatest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	ts1 := int64(1000000)
	ts2 := int64(2000000)
	ts3 := int64(3000000)

	// Lines intentionally out of chronological order to verify sorting logic.
	writeHistoryJSONL(t, path, []string{
		historyLine("second message", "/home/user/proj", "sess-1", ts2),
		historyLine("first message", "/home/user/proj", "sess-1", ts1),
		historyLine("third message", "/home/user/proj", "sess-1", ts3),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	require.Equal(t, 1, sh.Count())

	e := sh.GetAll()[0]
	assert.Equal(t, "first message", e.Name, "Name must come from earliest-timestamp entry")
	assert.Equal(t, time.UnixMilli(ts1), e.CreatedAt)
	assert.Equal(t, time.UnixMilli(ts3), e.UpdatedAt)
	assert.Equal(t, 3, e.MessageCount)
}

func TestReload_MultipleSessionsSortedByUpdatedAtDescending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	// Three sessions with clearly different last-message timestamps.
	writeHistoryJSONL(t, path, []string{
		historyLine("old task", "/proj/a", "sess-old", 1000),
		historyLine("new task", "/proj/b", "sess-new", 9000),
		historyLine("mid task", "/proj/c", "sess-mid", 5000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	require.Equal(t, 3, sh.Count())

	entries := sh.GetAll()
	assert.Equal(t, "sess-new", entries[0].ID, "most recent first")
	assert.Equal(t, "sess-mid", entries[1].ID)
	assert.Equal(t, "sess-old", entries[2].ID, "oldest last")
}

func TestReload_LinesWithEmptySessionID_Skipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		`{"display":"has no session","timestamp":1000,"project":"/proj","sessionId":""}`,
		historyLine("real entry", "/proj/b", "sess-valid", 2000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 1, sh.Count())
	assert.Equal(t, "sess-valid", sh.GetAll()[0].ID)
}

func TestReload_MalformedJSONLines_Skipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		`this is not json at all`,
		`{"broken": `,
		historyLine("good entry", "/proj", "sess-ok", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 1, sh.Count())
	assert.Equal(t, "sess-ok", sh.GetAll()[0].ID)
}

func TestReload_EmptyDisplay_FallsBackToFilepathBaseOfProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		`{"display":"","timestamp":1000,"project":"/home/user/myrepo","sessionId":"sess-1"}`,
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	require.Equal(t, 1, sh.Count())

	e := sh.GetAll()[0]
	assert.Equal(t, "myrepo", e.Name, "should fall back to filepath.Base(project)")
}

func TestReload_BothDisplayAndProjectEmpty_NameIsUnknown(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		`{"display":"","timestamp":1000,"project":"","sessionId":"sess-1"}`,
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	require.Equal(t, 1, sh.Count())

	e := sh.GetAll()[0]
	assert.Equal(t, "Unknown", e.Name)
}

func TestReload_DisplayOver100Chars_TruncatedTo97PlusDots(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	longDisplay := strings.Repeat("a", 150)
	writeHistoryJSONL(t, path, []string{
		historyLine(longDisplay, "/proj", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	e := sh.GetAll()[0]
	assert.Equal(t, 100, len(e.Name))
	assert.True(t, strings.HasSuffix(e.Name, "..."))
	assert.Equal(t, strings.Repeat("a", 97)+"...", e.Name)
}

func TestReload_DisplayStartingWithSlash_SlashTrimmed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		historyLine("/quality:review", "/proj", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	e := sh.GetAll()[0]
	assert.Equal(t, "quality:review", e.Name)
}

func TestReload_HistoryPathIsDirectory_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	// Pass the directory itself as the history path.
	_, err := NewClaudeSessionHistory(dir)
	assert.Error(t, err, "opening a directory as a file should return an error")
}

func TestReload_SameSessionIDDeduplicatedIntoOneEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		historyLine("msg one", "/proj", "sess-dup", 1000),
		historyLine("msg two", "/proj", "sess-dup", 2000),
		historyLine("msg three", "/proj", "sess-dup", 3000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 1, sh.Count(), "identical sessionIds must produce a single entry")
	assert.Equal(t, 3, sh.GetAll()[0].MessageCount)
}

func TestGetByProject_ReturnsCorrectCountForSharedProject(t *testing.T) {
	// Regression test for a bug where projectIndex was built before sort.Slice
	// reordered sh.entries, causing indices to point to wrong entries after sorting.
	// Fix: index is now built after sort.
	//
	// This test deliberately places sessions so that sorting changes their positions:
	// sess-a (alpha, ts=1000) and sess-c (alpha, ts=5000) sandwich sess-b (beta, ts=3000).
	// Pre-sort: [alpha@0, beta@1, alpha@2]. Post-sort: [alpha@0(ts=5000), beta@1, alpha@2(ts=1000)].
	// Old code stored indices [0, 2] for alpha before sorting — which happened to still be alpha.
	// The failure case: sess-b (beta, ts=9000) would move to index 0, displacing alpha.
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		historyLine("task A", "/proj/alpha", "sess-a", 1000),
		historyLine("task B", "/proj/beta", "sess-b", 9000), // sorts to index 0 after sort
		historyLine("task C", "/proj/alpha", "sess-c", 500),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	byAlpha := sh.GetByProject("/proj/alpha")
	require.Len(t, byAlpha, 2, "alpha must have exactly 2 entries")
	for _, e := range byAlpha {
		assert.Equal(t, "/proj/alpha", e.Project, "GetByProject must not return entries from other projects")
	}

	byBeta := sh.GetByProject("/proj/beta")
	require.Len(t, byBeta, 1)
	assert.Equal(t, "/proj/beta", byBeta[0].Project)
}

// ---- cleanDisplayName() tests ----------------------------------------------

func TestCleanDisplayName_NormalString_Unchanged(t *testing.T) {
	assert.Equal(t, "fix the bug", cleanDisplayName("fix the bug"))
}

func TestCleanDisplayName_LeadingWhitespace_Trimmed(t *testing.T) {
	assert.Equal(t, "hello world", cleanDisplayName("  hello world  "))
}

func TestCleanDisplayName_StartingWithSingleSlash_Trimmed(t *testing.T) {
	assert.Equal(t, "quality:review", cleanDisplayName("/quality:review"))
}

func TestCleanDisplayName_StartingWithMultipleSlashes_AllTrimmed(t *testing.T) {
	result := cleanDisplayName("///nested/path")
	assert.False(t, strings.HasPrefix(result, "/"), "all leading slashes should be trimmed")
	assert.Equal(t, "nested/path", result)
}

func TestCleanDisplayName_Over100Chars_TruncatedTo97PlusDots(t *testing.T) {
	input := strings.Repeat("x", 150)
	result := cleanDisplayName(input)
	assert.Equal(t, 100, len(result))
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestCleanDisplayName_Exactly100Chars_NotTruncated(t *testing.T) {
	input := strings.Repeat("y", 100)
	result := cleanDisplayName(input)
	assert.Equal(t, 100, len(result))
	assert.False(t, strings.HasSuffix(result, "..."))
}

func TestCleanDisplayName_EmptyString_ReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", cleanDisplayName(""))
}

func TestCleanDisplayName_WhitespaceOnly_ReturnsEmpty(t *testing.T) {
	assert.Equal(t, "", cleanDisplayName("   "))
}

// ---- GetAll / GetByID / GetByProject / Search still work after Reload ------

func TestGetAll_ReturnsCopyNotReference(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task", "/proj", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	a := sh.GetAll()
	b := sh.GetAll()
	// Mutating one slice must not affect the other.
	a[0].Name = "mutated"
	assert.NotEqual(t, "mutated", b[0].Name)
}

func TestGetByID_Found(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task", "/proj", "sess-target", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	entry, err := sh.GetByID("sess-target")
	require.NoError(t, err)
	require.NotNil(t, entry)
	assert.Equal(t, "sess-target", entry.ID)
}

func TestGetByID_NotFound_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task", "/proj", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	_, err = sh.GetByID("does-not-exist")
	assert.Error(t, err)
}

func TestGetByProject_ReturnsOnlyMatchingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task a", "/proj/alpha", "sess-a", 1000),
		historyLine("task b", "/proj/beta", "sess-b", 2000),
		historyLine("task c", "/proj/alpha", "sess-c", 3000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	results := sh.GetByProject("/proj/alpha")
	assert.Len(t, results, 2)
	for _, e := range results {
		assert.Equal(t, "/proj/alpha", e.Project)
	}
}

func TestGetByProject_UnknownProject_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task", "/proj/real", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	results := sh.GetByProject("/proj/does-not-exist")
	assert.Empty(t, results)
}

func TestSearch_MatchesName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("fix the login bug", "/proj/a", "sess-1", 1000),
		historyLine("add dark mode", "/proj/b", "sess-2", 2000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	results := sh.Search("login")
	require.Len(t, results, 1)
	assert.Equal(t, "sess-1", results[0].ID)
}

func TestSearch_MatchesProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("some task", "/home/user/frontend-app", "sess-1", 1000),
		historyLine("other task", "/home/user/backend-api", "sess-2", 2000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	results := sh.Search("frontend")
	require.Len(t, results, 1)
	assert.Equal(t, "sess-1", results[0].ID)
}

func TestSearch_CaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("Fix The Login Bug", "/proj", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	results := sh.Search("LOGIN")
	assert.Len(t, results, 1)
}

func TestSearch_NoMatch_ReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task", "/proj", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	results := sh.Search("zzznomatch")
	assert.Empty(t, results)
}

// ---- Reload called twice (idempotency) -------------------------------------

func TestReload_Idempotent_SameResultOnSecondCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task one", "/proj/a", "sess-1", 1000),
		historyLine("task two", "/proj/b", "sess-2", 2000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	first := sh.GetAll()
	require.NoError(t, sh.Reload())
	second := sh.GetAll()

	require.Len(t, second, len(first))
	for i := range first {
		assert.Equal(t, first[i].ID, second[i].ID)
		assert.Equal(t, first[i].Name, second[i].Name)
	}
}

// ---- Edge cases ------------------------------------------------------------

func TestReload_EmptyFile_ZeroEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	// Create the file but write nothing.
	f, err := os.Create(path)
	require.NoError(t, err)
	f.Close()

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 0, sh.Count())
}

func TestReload_EmptyLinesInFile_Skipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	f, err := os.Create(path)
	require.NoError(t, err)
	fmt.Fprintf(f, "\n\n")
	fmt.Fprintf(f, "%s\n", historyLine("task", "/proj", "sess-1", 1000))
	fmt.Fprintf(f, "\n")
	f.Close()

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 1, sh.Count())
}

func TestReload_ProjectFromEarliestEntry_WhenMultipleProjects(t *testing.T) {
	// When two messages for the same session have different projects,
	// the project from the earliest-timestamp message should win.
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		historyLine("later msg", "/proj/later", "sess-1", 2000),
		historyLine("earlier msg", "/proj/earlier", "sess-1", 1000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	e := sh.GetAll()[0]
	assert.Equal(t, "/proj/earlier", e.Project, "project should come from earliest entry")
}

func TestCount_ReflectsActualEntryCount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	lines := make([]string, 5)
	for i := range lines {
		lines[i] = historyLine(fmt.Sprintf("task %d", i), "/proj", fmt.Sprintf("sess-%d", i), int64(i*1000))
	}
	writeHistoryJSONL(t, path, lines)

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	assert.Equal(t, 5, sh.Count())
}

func TestGetProjects_ReturnsUniqueProjectsSorted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	writeHistoryJSONL(t, path, []string{
		historyLine("t1", "/proj/zoo", "sess-1", 1000),
		historyLine("t2", "/proj/alpha", "sess-2", 2000),
		historyLine("t3", "/proj/zoo", "sess-3", 3000),
	})

	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)

	projects := sh.GetProjects()
	assert.Equal(t, []string{"/proj/alpha", "/proj/zoo"}, projects)
}

func TestLastLoadTime_UpdatedAfterReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	writeHistoryJSONL(t, path, []string{
		historyLine("task", "/proj", "sess-1", 1000),
	})

	before := time.Now()
	sh, err := NewClaudeSessionHistory(path)
	require.NoError(t, err)
	after := time.Now()

	lt := sh.LastLoadTime()
	assert.True(t, lt.After(before) || lt.Equal(before))
	assert.True(t, lt.Before(after) || lt.Equal(after))
}
