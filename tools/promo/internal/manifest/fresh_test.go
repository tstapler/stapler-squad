package manifest_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

func budgets() manifest.Budgets {
	return manifest.Budgets{ClipMaxBytes: 1000, ImageMaxBytes: 500, DistMaxBytes: 5000, RecordWallClockMaxMinutes: 30, ClipMaxDurationMs: 60000,
		RecordInputPaths: []string{"web-app/**", "go.mod", "website/src/content/docs/tours/**"}}
}

func lines(ps []manifest.Problem) string {
	s := make([]string, 0, len(ps))
	for _, p := range ps {
		s = append(s, p.String())
	}
	return strings.Join(s, "\n")
}

func TestFresh_Presence(t *testing.T) {
	t.Parallel()
	failed := entry("failed-one", manifest.FormatPNG)
	failed.Status = manifest.StatusFailed
	in := manifest.FreshInput{
		Manifest: sample(entry("present", manifest.FormatWebM), failed),
		Expected: manifest.ExpectedMedia{Media: []manifest.ExpectedItem{
			{Name: "approvals-queue"}, {Name: "present", Formats: []manifest.Format{manifest.FormatWebM, manifest.FormatMP4}}, {Name: "failed-one"}}},
		Budgets: budgets(), SHA: "abc123",
	}
	got := lines(manifest.Fresh(in))
	assert.Contains(t, got, "approvals-queue: missing")
	assert.Contains(t, got, "present (mp4): missing")
	assert.Contains(t, got, "failed-one: failed")
	assert.NotContains(t, got, "present (webm)")
}

func TestFresh_CommitAndTheme(t *testing.T) {
	t.Parallel()
	e := entry("a", manifest.FormatPNG)
	e.Commit = "abc123"
	clean := entry("b", manifest.FormatPNG)
	clean.ThemeID = "clean"
	got := lines(manifest.Fresh(manifest.FreshInput{Manifest: sample(e, clean), Budgets: budgets(), SHA: "def456"}))
	assert.Contains(t, got, "a: commit abc123, want def456")
	assert.Contains(t, got, `b: themeId "clean"`)
	assert.Empty(t, manifest.Fresh(manifest.FreshInput{Manifest: sample(e), Budgets: budgets(), SHA: "abc123"}))
}

func TestFresh_PerAssetBudgets(t *testing.T) {
	t.Parallel()
	clip, img := entry("clip", manifest.FormatWebM), entry("img", manifest.FormatWebP)
	clip.Bytes, img.Bytes = 1001, 501
	got := lines(manifest.Fresh(manifest.FreshInput{Manifest: sample(clip, img), Budgets: budgets(), SHA: "abc123"}))
	assert.Contains(t, got, "clip: clipMaxBytes exceeded")
	assert.Contains(t, got, "img: imageMaxBytes exceeded")
}

func TestFresh_ClipOver60sFails(t *testing.T) {
	t.Parallel()
	clip := entry("clip", manifest.FormatMP4)
	clip.DurationMs = 60001
	assert.Contains(t, lines(manifest.Fresh(manifest.FreshInput{Manifest: sample(clip), Budgets: budgets(), SHA: "abc123"})), "clipMaxDurationMs exceeded")
	clip.DurationMs = 60000
	assert.Empty(t, manifest.Fresh(manifest.FreshInput{Manifest: sample(clip), Budgets: budgets(), SHA: "abc123"}))
}

func TestFresh_RecordWallClockCeiling(t *testing.T) {
	t.Parallel()
	m := sample(entry("a", manifest.FormatPNG))
	m.FinishedAt = m.StartedAt.Add(31 * time.Minute)
	assert.Contains(t, lines(manifest.Fresh(manifest.FreshInput{Manifest: m, Budgets: budgets(), SHA: "abc123"})), "recordWallClockMaxMinutes exceeded")
	m.FinishedAt = m.StartedAt.Add(30 * time.Minute)
	assert.Empty(t, manifest.Fresh(manifest.FreshInput{Manifest: m, Budgets: budgets(), SHA: "abc123"}))
}

func TestFresh_DistCeilingAndExistence(t *testing.T) {
	t.Parallel()
	dist := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dist, "clips"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dist, "clips", "a.webm"), make([]byte, 10), 0o644))
	e := entry("a", manifest.FormatWebM)
	e.Poster = "/clips/poster.webp"
	in := manifest.FreshInput{Manifest: sample(e), Budgets: budgets(), SHA: "abc123", DistDir: dist}
	got := lines(manifest.Fresh(in))
	assert.Contains(t, got, `poster "/clips/poster.webp" missing from dist`)
	assert.NotContains(t, got, "src")

	require.NoError(t, os.WriteFile(filepath.Join(dist, "big.bin"), make([]byte, 5001), 0o644))
	assert.Contains(t, lines(manifest.Fresh(in)), "distMaxBytes exceeded")
}

func TestFresh_ReuseMode(t *testing.T) {
	t.Parallel()
	e := entry("a", manifest.FormatPNG)
	e.Commit = "C"
	cmp := func(status string, files ...string) *manifest.Compare {
		c := &manifest.Compare{Status: status}
		for _, f := range files {
			c.Files = append(c.Files, manifest.CompareFile{Filename: f})
		}
		return c
	}
	tests := []struct {
		name    string
		cmp     *manifest.Compare
		wantErr string
	}{
		{"content-only change reuses", cmp("ahead", "website/src/content/docs/help/x.md"), ""},
		{"identical reuses", cmp("identical"), ""},
		{"web-app change fails naming the path", cmp("ahead", "README.md", "web-app/src/a.ts"), "web-app/src/a.ts"},
		{"tour mdx change fails", cmp("ahead", "website/src/content/docs/tours/x.mdx"), "tours/x.mdx"},
		{"diverged fails", cmp("diverged", "README.md"), "not an ancestor"},
		{"behind fails", cmp("behind"), "not an ancestor"},
		{"truncated fails", &manifest.Compare{Status: "ahead", Truncated: true}, "truncated"},
		{"renamed-from input path fails", &manifest.Compare{Status: "ahead", Files: []manifest.CompareFile{{Filename: "docs/x.md", PreviousFilename: "go.mod"}}}, "go.mod"},
		{"missing compare input", nil, "compare"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := lines(manifest.Fresh(manifest.FreshInput{Manifest: sample(e), Budgets: budgets(), SHA: "H", ReuseFrom: "C", Compare: tt.cmp}))
			if tt.wantErr == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.wantErr)
			}
		})
	}

	t.Run("without -reuse-from strict equality still applies", func(t *testing.T) {
		t.Parallel()
		assert.Contains(t, lines(manifest.Fresh(manifest.FreshInput{Manifest: sample(e), Budgets: budgets(), SHA: "H"})), "commit C, want H")
	})
	t.Run("theme still enforced in reuse mode", func(t *testing.T) {
		t.Parallel()
		bad := e
		bad.ThemeID = "clean"
		got := lines(manifest.Fresh(manifest.FreshInput{Manifest: sample(bad), Budgets: budgets(), SHA: "H", ReuseFrom: "C", Compare: cmp("ahead")}))
		assert.Contains(t, got, "themeId")
	})
}

func TestParseCompare_PaginatedAndTruncation(t *testing.T) {
	t.Parallel()
	c, err := manifest.ParseCompare(strings.NewReader(`{"status":"ahead","files":[{"filename":"a"}]}{"status":"ahead","files":[{"filename":"b","previous_filename":"c"}]}`))
	require.NoError(t, err)
	assert.Equal(t, "ahead", c.Status)
	assert.Len(t, c.Files, 2)
	assert.False(t, c.Truncated)

	var big strings.Builder
	big.WriteString(`{"status":"ahead","files":[`)
	for i := 0; i < manifest.MaxCompareFiles; i++ {
		if i > 0 {
			big.WriteString(",")
		}
		big.WriteString(`{"filename":"f"}`)
	}
	big.WriteString(`]}`)
	c, err = manifest.ParseCompare(strings.NewReader(big.String()))
	require.NoError(t, err)
	assert.True(t, c.Truncated)

	_, err = manifest.ParseCompare(strings.NewReader(""))
	assert.Error(t, err)
}
