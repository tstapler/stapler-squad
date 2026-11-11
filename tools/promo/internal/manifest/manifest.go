// Package manifest holds the record-time media manifest, the expected-media list,
// the budgets, and the freshness checks over them.
package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// ThemeID is the only theme promo media may be recorded in.
const ThemeID = "grimdark"

// Format is one rendition of a media item; every clip has a webm and an mp4 row.
type Format string

// Renditions.
const (
	FormatWebM Format = "webm"
	FormatMP4  Format = "mp4"
	FormatWebP Format = "webp"
	FormatPNG  Format = "png"
)

// IsClip reports whether f is a video rendition (clip budgets apply).
func (f Format) IsClip() bool { return f == FormatWebM || f == FormatMP4 }

// Status is the outcome of recording one entry.
type Status string

// Statuses.
const (
	StatusRecorded Status = "recorded"
	StatusFailed   Status = "failed"
	StatusSkipped  Status = "skipped"
)

// MediaEntry is one recorded rendition. Every consumer (the TS recorder, mediatool, the
// website's reader) uses this shape; the website derives its clip/shot kind from Format.
// Transcripts are a convention, not a field: clips/<name>.transcript.json beside the clip.
type MediaEntry struct {
	Name       string `json:"name"`
	Format     Format `json:"format"`
	Status     Status `json:"status"`
	Src        string `json:"src,omitempty"`
	Poster     string `json:"poster,omitempty"`
	Source     string `json:"source,omitempty"`
	Bytes      int64  `json:"bytes"`
	SHA256     string `json:"sha256,omitempty"`
	Commit     string `json:"commit"`
	ThemeID    string `json:"themeId"`
	Attempts   int    `json:"attempts,omitempty"`
	Seeded     string `json:"seeded,omitempty"` // db, rpc or route
	DurationMs int64  `json:"durationMs,omitempty"`
	FormFactor string `json:"formFactor,omitempty"`
	// Width and Height are the pixel size of an image rendition (reserves layout space on the site).
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

// MediaManifest is what one record shard (or the merge of all) wrote.
type MediaManifest struct {
	Version    int          `json:"version"`
	Commit     string       `json:"commit"`
	StartedAt  time.Time    `json:"startedAt"`
	FinishedAt time.Time    `json:"finishedAt"`
	Entries    []MediaEntry `json:"entries"`
}

// ExpectedItem is one name the site needs, with the renditions it needs.
// An empty Formats list means any recorded rendition satisfies it.
type ExpectedItem struct {
	Name    string   `json:"name"`
	Formats []Format `json:"formats,omitempty"`
}

// ExpectedMedia is the single list of media the website consumes.
type ExpectedMedia struct {
	Version int            `json:"version"`
	Media   []ExpectedItem `json:"media"`
}

// ReadManifest loads a manifest file.
func ReadManifest(path string) (MediaManifest, error) {
	var m MediaManifest
	return m, readJSON(path, &m)
}

// ReadExpected loads the expected-media list.
func ReadExpected(path string) (ExpectedMedia, error) {
	var e ExpectedMedia
	return e, readJSON(path, &e)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// Merge combines shard manifests. Shards must agree on the commit; a
// (name, format) pair may appear only once.
func Merge(parts ...MediaManifest) (MediaManifest, error) {
	var out MediaManifest
	seen := map[string]bool{}
	for i, p := range parts {
		if i == 0 {
			out.Version, out.Commit, out.StartedAt, out.FinishedAt = p.Version, p.Commit, p.StartedAt, p.FinishedAt
		} else {
			if p.Commit != out.Commit {
				return MediaManifest{}, fmt.Errorf("shards disagree on commit: %s vs %s", out.Commit, p.Commit)
			}
			if p.StartedAt.Before(out.StartedAt) {
				out.StartedAt = p.StartedAt
			}
			if p.FinishedAt.After(out.FinishedAt) {
				out.FinishedAt = p.FinishedAt
			}
		}
		for _, e := range p.Entries {
			key := e.Name + "\x00" + string(e.Format)
			if seen[key] {
				return MediaManifest{}, fmt.Errorf("duplicate entry %s (%s) across shards", e.Name, e.Format)
			}
			seen[key] = true
			out.Entries = append(out.Entries, e)
		}
	}
	sort.SliceStable(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.Format < b.Format
	})
	return out, nil
}
