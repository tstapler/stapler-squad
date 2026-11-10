package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileName is the shard manifest the recorder writes into its output directory.
const FileName = "media-manifest.json"

// localPath is where an entry's file sits under dir: its src (clips/<name>.webm,
// shots/<name>.webp), or flat <name>.<format> for a clip row that has no src.
func localPath(dir string, e MediaEntry) string {
	if e.Src == "" && e.Format.IsClip() {
		return filepath.Join(dir, e.Name+"."+string(e.Format))
	}
	return filepath.Join(dir, filepath.FromSlash(e.Src))
}

// toWebP repoints a recorded PNG row at its converted .webp sibling when only that
// remains on disk (the record script converts shots and drops the PNG).
func toWebP(dir string, e MediaEntry) MediaEntry {
	if e.Format != FormatPNG || !strings.HasSuffix(e.Src, ".png") {
		return e
	}
	webp := strings.TrimSuffix(e.Src, ".png") + ".webp"
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(e.Src))); err == nil {
		return e
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(webp))); err != nil {
		return e
	}
	e.Format, e.Src = FormatWebP, webp
	return e
}

// posterToWebP follows a clip's poster to its converted file.
func posterToWebP(dir string, e MediaEntry) MediaEntry {
	if !strings.HasSuffix(e.Poster, ".png") {
		return e
	}
	webp := strings.TrimSuffix(e.Poster, ".png") + ".webp"
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(webp))); err == nil {
		e.Poster = webp
	}
	return e
}

func measure(path string) (bytes int64, sum string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", err
	}
	h := sha256.Sum256(data)
	return int64(len(data)), hex.EncodeToString(h[:]), nil
}

// RefreshFromDisk re-measures every recorded entry after post-processing (trim,
// title card, encode) so the manifest describes the files that ship. A recorded
// entry whose file is gone becomes failed, and a recorded webm gains a recorded
// mp4 row when <name>.mp4 exists beside it. It rewrites dir/media-manifest.json.
func RefreshFromDisk(dir string) (MediaManifest, error) {
	file := filepath.Join(dir, FileName)
	m, err := ReadManifest(file)
	if err != nil {
		return MediaManifest{}, err
	}
	var next []MediaEntry
	for _, e := range m.Entries {
		if e.Status != StatusRecorded {
			next = append(next, e)
			continue
		}
		e = posterToWebP(dir, toWebP(dir, e))
		size, sum, err := measure(localPath(dir, e))
		switch {
		case os.IsNotExist(err):
			e.Status, e.Bytes, e.SHA256 = StatusFailed, 0, ""
			next = append(next, e)
			continue
		case err != nil:
			return MediaManifest{}, err
		}
		e.Bytes, e.SHA256 = size, sum
		next = append(next, e)
		if e.Format == FormatWebM {
			if mp4, ok, err := mp4Row(dir, e); err != nil {
				return MediaManifest{}, err
			} else if ok {
				next = append(next, mp4)
			}
		}
	}
	out, err := Merge(MediaManifest{Version: m.Version, Commit: m.Commit, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt, Entries: next})
	if err != nil {
		return MediaManifest{}, err
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return MediaManifest{}, err
	}
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		return MediaManifest{}, fmt.Errorf("write %s: %w", file, err)
	}
	return out, nil
}

func mp4Row(dir string, webm MediaEntry) (MediaEntry, bool, error) {
	row := webm
	row.Format = FormatMP4
	row.Src = "clips/" + webm.Name + ".mp4"
	size, sum, err := measure(localPath(dir, row))
	if os.IsNotExist(err) {
		return MediaEntry{}, false, nil
	}
	if err != nil {
		return MediaEntry{}, false, err
	}
	row.Bytes, row.SHA256 = size, sum
	return row, true, nil
}
