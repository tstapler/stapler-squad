package manifest_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xeipuuv/gojsonschema"

	"github.com/tstapler/stapler-squad/tools/promo/internal/manifest"
)

func epoch() time.Time { return time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC) }

func entry(name string, f manifest.Format) manifest.MediaEntry {
	return manifest.MediaEntry{Name: name, Format: f, Status: manifest.StatusRecorded, Src: "/clips/" + name + "." + string(f),
		Bytes: 100, SHA256: strings.Repeat("a", 64), Commit: "abc123", ThemeID: manifest.ThemeID}
}

func sample(entries ...manifest.MediaEntry) manifest.MediaManifest {
	return manifest.MediaManifest{Version: 1, Commit: "abc123", StartedAt: epoch(), FinishedAt: epoch().Add(time.Minute), Entries: entries}
}

func validate(t *testing.T, doc any) *gojsonschema.Result {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schema", "media-manifest.schema.json"))
	require.NoError(t, err)
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	res, err := gojsonschema.Validate(gojsonschema.NewBytesLoader(raw), gojsonschema.NewBytesLoader(data))
	require.NoError(t, err)
	return res
}

func TestSchema_AcceptsGoOutputAndRejectsBadStatus(t *testing.T) {
	t.Parallel()
	full := entry("a", manifest.FormatWebM)
	full.Poster, full.Source, full.Attempts, full.Seeded, full.DurationMs, full.FormFactor = "/p.webp", "/s.mp4", 2, "db", 5000, "desktop"
	res := validate(t, sample(full, entry("a", manifest.FormatMP4)))
	assert.True(t, res.Valid(), res.Errors())

	bad := entry("b", manifest.FormatPNG)
	bad.Status = "weird"
	assert.False(t, validate(t, sample(bad)).Valid())
}

func TestMerge(t *testing.T) {
	t.Parallel()
	a, b := sample(entry("z", manifest.FormatPNG)), sample(entry("a", manifest.FormatWebM))
	b.StartedAt, b.FinishedAt = epoch().Add(-time.Hour), epoch().Add(time.Hour)
	got, err := manifest.Merge(a, b)
	require.NoError(t, err)
	assert.Equal(t, "a", got.Entries[0].Name)
	assert.Equal(t, epoch().Add(-time.Hour), got.StartedAt)
	assert.Equal(t, epoch().Add(time.Hour), got.FinishedAt)

	_, err = manifest.Merge(a, a)
	assert.ErrorContains(t, err, "duplicate")
	other := sample()
	other.Commit = "different"
	_, err = manifest.Merge(a, other)
	assert.ErrorContains(t, err, "disagree")
}

func TestReadBudgets_RealFile(t *testing.T) {
	t.Parallel()
	b, err := manifest.ReadBudgets(filepath.Join("..", "..", "budgets.json"))
	require.NoError(t, err)
	assert.EqualValues(t, 3*1024*1024, b.ClipMaxBytes)
	assert.EqualValues(t, 400*1024, b.ImageMaxBytes)
	assert.EqualValues(t, 40*1024*1024, b.DistMaxBytes)
	assert.Equal(t, 30, b.RecordWallClockMaxMinutes)
	assert.EqualValues(t, 60000, b.ClipMaxDurationMs)
	assert.Contains(t, b.RecordInputPaths, "tools/promolib/**")
	assert.Len(t, b.RecordInputPaths, 15)
}

func TestReadBudgets_Rejects(t *testing.T) {
	t.Parallel()
	for name, body := range map[string]string{
		"unknown key": `{"clipMaxBytes":1,"imageMaxBytes":1,"distMaxBytes":1,"recordWallClockMaxMinutes":1,"clipMaxDurationMs":1,"recordInputPaths":["a"],"x":1}`,
		"zero value":  `{"clipMaxBytes":0,"imageMaxBytes":1,"distMaxBytes":1,"recordWallClockMaxMinutes":1,"clipMaxDurationMs":1,"recordInputPaths":["a"]}`,
		"bad glob":    `{"clipMaxBytes":1,"imageMaxBytes":1,"distMaxBytes":1,"recordWallClockMaxMinutes":1,"clipMaxDurationMs":1,"ocrMinConfidence":50,"ocrFixtureMinRecall":0.5,"recordInputPaths":["a/[b"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := filepath.Join(t.TempDir(), "b.json")
			require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
			_, err := manifest.ReadBudgets(p)
			assert.Error(t, err)
		})
	}
}

func TestRenderSummary(t *testing.T) {
	t.Parallel()
	failed := entry("c", manifest.FormatPNG)
	failed.Status = manifest.StatusFailed
	out := manifest.RenderSummary(sample(entry("a", manifest.FormatWebM), entry("b", manifest.FormatMP4), failed), 2000, 1500)
	assert.Contains(t, out, "recorded 2, failed 1, skipped 0")
	assert.Contains(t, out, "| a | webm | recorded | 100 | "+strings.Repeat("a", 64))
	assert.Contains(t, out, "previous run 1500, +500")
	assert.NotContains(t, manifest.RenderSummary(sample(), 2000, -1), "previous run")
}

func TestRenderSummary_ShowsAttemptsAndFlakyMarker(t *testing.T) {
	t.Parallel()
	steady, flaky := entry("steady", manifest.FormatWebM), entry("shaky", manifest.FormatWebM)
	steady.Attempts, flaky.Attempts = 1, 2
	out := manifest.RenderSummary(sample(steady, flaky, entry("legacy", manifest.FormatPNG)), 0, -1)
	assert.Contains(t, out, "| attempts |")
	assert.Contains(t, out, "| steady | webm | recorded | 100 | "+strings.Repeat("a", 64))
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "| steady "):
			assert.True(t, strings.HasSuffix(line, "| 1 |"), line)
		case strings.HasPrefix(line, "| shaky "):
			assert.True(t, strings.HasSuffix(line, "| 2 flaky |"), line)
		case strings.HasPrefix(line, "| legacy "):
			assert.True(t, strings.HasSuffix(line, "| - |"), line)
		}
	}
}
