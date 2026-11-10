package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

type fixture struct{ dir string }

func newFixture(t *testing.T, entries []manifest.MediaEntry, expected []string, flagsJSON string) fixture {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, v any) {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), raw, 0o644))
	}
	at := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	write("m.json", manifest.MediaManifest{Version: 1, Commit: "C", StartedAt: at, FinishedAt: at.Add(time.Minute), Entries: entries})
	items := make([]manifest.ExpectedItem, 0, len(expected))
	for _, n := range expected {
		items = append(items, manifest.ExpectedItem{Name: n})
	}
	write("e.json", manifest.ExpectedMedia{Version: 1, Media: items})
	budgets, err := os.ReadFile(filepath.Join("..", "..", "budgets.json"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.json"), budgets, 0o644))
	if flagsJSON != "" {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "f.json"), []byte(flagsJSON), 0o644))
	}
	return fixture{dir: dir}
}

func (f fixture) run(t *testing.T, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	base := make([]string, 0, 9+len(args))
	base = append(base, "fresh", "-manifest", filepath.Join(f.dir, "m.json"), "-expected", filepath.Join(f.dir, "e.json"),
		"-budgets", filepath.Join(f.dir, "b.json"), "-flags", filepath.Join(f.dir, "f.json"))
	var out, errb bytes.Buffer
	code := run(append(base, args...), &out, &errb, func(k string) string { return env[k] })
	return code, out.String(), errb.String()
}

func rec(name string, f manifest.Format, commit, theme string) manifest.MediaEntry {
	return manifest.MediaEntry{Name: name, Format: f, Status: manifest.StatusRecorded, Bytes: 10, SHA256: strings.Repeat("b", 64), Commit: commit, ThemeID: theme}
}

const enforce = `{"freshnessEnforce": true, "docsCoverageEnforce": false, "guardsEnforce": true}`

func TestFresh_MissingExpectedExitsOne(t *testing.T) {
	t.Parallel()
	f := newFixture(t, []manifest.MediaEntry{rec("a", manifest.FormatPNG, "C", "grimdark")}, []string{"a", "approvals-queue"}, enforce)
	code, out, _ := f.run(t, map[string]string{"GITHUB_SHA": "C"})
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "approvals-queue: missing")
}

func TestFresh_ReportOnlyWhenFreshnessEnforceFalse(t *testing.T) {
	t.Parallel()
	f := newFixture(t, []manifest.MediaEntry{rec("a", manifest.FormatPNG, "abc123", "clean")}, []string{"a"},
		`{"freshnessEnforce": false, "docsCoverageEnforce": false, "guardsEnforce": true}`)
	code, out, _ := f.run(t, map[string]string{"GITHUB_SHA": "def456"})
	assert.Equal(t, 0, code)
	assert.Contains(t, out, "commit abc123, want def456")
	assert.Contains(t, out, "report-only")
}

func TestFresh_AbsentFlagsFileIsReportOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t, []manifest.MediaEntry{rec("a", manifest.FormatPNG, "C", "grimdark")}, []string{"missing"}, "")
	code, _, _ := f.run(t, map[string]string{"GITHUB_SHA": "C"})
	assert.Equal(t, 0, code)
}

func TestFresh_SummaryAppendsTable(t *testing.T) {
	t.Parallel()
	failed := rec("c", manifest.FormatPNG, "C", "grimdark")
	failed.Status = manifest.StatusFailed
	f := newFixture(t, []manifest.MediaEntry{rec("a", manifest.FormatWebM, "C", "grimdark"), rec("b", manifest.FormatPNG, "C", "grimdark"), failed}, nil, enforce)
	summary := filepath.Join(f.dir, "step.md")
	code, out, errOut := f.run(t, map[string]string{"GITHUB_SHA": "C", "GITHUB_STEP_SUMMARY": summary}, "-summary")
	require.Equal(t, 0, code, out+errOut)
	got, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Contains(t, string(got), "recorded 2, failed 1")
	assert.Contains(t, string(got), "| a | webm | recorded | 10 | "+strings.Repeat("b", 64))
}

func TestFresh_ReuseAndDecide(t *testing.T) {
	t.Parallel()
	f := newFixture(t, []manifest.MediaEntry{rec("a", manifest.FormatPNG, "C", "grimdark")}, []string{"a"}, enforce)
	writeCmp := func(body string) string {
		p := filepath.Join(t.TempDir(), "cmp.json")
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
		return p
	}
	content := writeCmp(`{"status":"ahead","files":[{"filename":"website/src/content/docs/help/x.md"}]}`)
	code, out, errOut := f.run(t, map[string]string{"GITHUB_SHA": "H"}, "-reuse-from", "C", "-compare-json", content)
	require.Equal(t, 0, code, out+errOut)
	assert.Contains(t, out, "reused media from C")

	code, out, _ = f.run(t, map[string]string{"GITHUB_SHA": "H"}, "-decide-reuse", "-compare-json", content)
	assert.Equal(t, 0, code)
	assert.Equal(t, "reuse\n", out)

	code, out, _ = f.run(t, map[string]string{"GITHUB_SHA": "H"}, "-reuse-from", "C", "-compare-json",
		writeCmp(`{"status":"ahead","files":[{"filename":"go.mod"}]}`))
	assert.Equal(t, 1, code)
	assert.Contains(t, out, "go.mod")

	code, out, _ = f.run(t, nil, "-decide-reuse", "-compare-json", writeCmp(`{"status":"diverged","files":[]}`))
	assert.Equal(t, 0, code)
	assert.Equal(t, "record\n", out)

	code, _, _ = f.run(t, map[string]string{"GITHUB_SHA": "H"})
	assert.Equal(t, 1, code, "without -reuse-from commit equality is required")
}

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"bogus"}, {"fresh"}, {"fresh", "-manifest", "/nonexistent.json"}, {"summary"}} {
		var out, errb bytes.Buffer
		assert.Equal(t, 2, run(args, &out, &errb, func(string) string { return "" }), args)
	}
}

// A clip is only fresh when every rendition the site needs exists: webm alone is not enough.
func TestFresh_OneRenditionMissingFailsNamingIt(t *testing.T) {
	t.Parallel()
	failedMP4 := rec("hero", manifest.FormatMP4, "C", "grimdark")
	failedMP4.Status = manifest.StatusFailed
	for _, tc := range []struct {
		name    string
		entries []manifest.MediaEntry
		missing manifest.Format
	}{
		{"mp4 absent", []manifest.MediaEntry{rec("hero", manifest.FormatWebM, "C", "grimdark")}, manifest.FormatMP4},
		{"webm absent", []manifest.MediaEntry{rec("hero", manifest.FormatMP4, "C", "grimdark")}, manifest.FormatWebM},
		{"mp4 failed", []manifest.MediaEntry{rec("hero", manifest.FormatWebM, "C", "grimdark"), failedMP4}, manifest.FormatMP4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t, tc.entries, []string{"hero"}, enforce)
			raw, err := json.Marshal(manifest.ExpectedMedia{Version: 1, Media: []manifest.ExpectedItem{{Name: "hero", Formats: []manifest.Format{manifest.FormatWebM, manifest.FormatMP4}}}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(f.dir, "e.json"), raw, 0o644))

			code, out, _ := f.run(t, map[string]string{"GITHUB_SHA": "C"})
			assert.Equal(t, 1, code)
			assert.Contains(t, out, "hero ("+string(tc.missing)+")")
		})
	}
}
