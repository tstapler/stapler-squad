package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshFromDisk(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "shots"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "clips"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "clips", "a.webm"), []byte("twelve-bytes"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "clips", "a.mp4"), []byte("mp4"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shots", "s.png"), []byte("png"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shots", "w.webp"), []byte("webp!"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "shots", "a-poster.webp"), []byte("p"), 0o644))
	at := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	in := MediaManifest{Version: 1, Commit: "C", StartedAt: at, FinishedAt: at, Entries: []MediaEntry{
		{Name: "a", Format: FormatWebM, Status: StatusRecorded, Src: "clips/a.webm", Poster: "shots/a-poster.png", Commit: "C", ThemeID: ThemeID, Attempts: 2},
		{Name: "s", Format: FormatPNG, Status: StatusRecorded, Src: "shots/s.png", Commit: "C", ThemeID: ThemeID},
		{Name: "w", Format: FormatPNG, Status: StatusRecorded, Src: "shots/w.png", Commit: "C", ThemeID: ThemeID},
		{Name: "gone", Format: FormatWebM, Status: StatusRecorded, Src: "clips/gone.webm", Commit: "C", ThemeID: ThemeID},
		{Name: "bad", Format: FormatWebM, Status: StatusFailed, Commit: "C", ThemeID: ThemeID},
	}}
	raw, err := json.Marshal(in)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, FileName), raw, 0o644))

	got, err := RefreshFromDisk(dir)
	require.NoError(t, err)
	by := map[string]MediaEntry{}
	for _, e := range got.Entries {
		by[e.Name+"."+string(e.Format)] = e
	}
	assert.EqualValues(t, 12, by["a.webm"].Bytes)
	assert.Len(t, by["a.webm"].SHA256, 64)
	assert.Equal(t, 2, by["a.webm"].Attempts)
	assert.Equal(t, StatusRecorded, by["a.mp4"].Status)
	assert.EqualValues(t, 3, by["a.mp4"].Bytes)
	assert.EqualValues(t, 3, by["s.png"].Bytes)
	assert.Equal(t, "shots/a-poster.webp", by["a.webm"].Poster, "poster follows its webp conversion")
	assert.Equal(t, "shots/a-poster.webp", by["a.mp4"].Poster)
	assert.Equal(t, FormatWebP, by["w.webp"].Format, "png row repointed at the converted webp")
	assert.Equal(t, "shots/w.webp", by["w.webp"].Src)
	assert.EqualValues(t, 5, by["w.webp"].Bytes)
	assert.Equal(t, StatusFailed, by["gone.webm"].Status)
	assert.Equal(t, StatusFailed, by["bad.webm"].Status)
	again, err := ReadManifest(filepath.Join(dir, FileName))
	require.NoError(t, err)
	assert.Equal(t, got, again, "rewritten on disk")
}
