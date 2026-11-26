package backlogstore

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/tstapler/stapler-squad/session/domain"
)

// ShippedFilePaths returns the paths in an item's ShippedFileStats JSON ([]{Path,...}).
func ShippedFilePaths(shippedFileStatsJSON string) []string {
	var stats []struct {
		Path string `json:"path"`
	}
	if shippedFileStatsJSON == "" || json.Unmarshal([]byte(shippedFileStatsJSON), &stats) != nil {
		return nil
	}
	paths := make([]string, 0, len(stats))
	for _, s := range stats {
		if s.Path != "" {
			paths = append(paths, s.Path)
		}
	}
	return paths
}

// AttributeByFileOverlap scores each sibling by the fraction of failingFiles its diff touched
// and returns the best one. A tie for best, no overlap, or no failingFiles yields ("", 0), so an
// ambiguous failure always escalates instead of blaming an arbitrary sibling.
func AttributeByFileOverlap(failingFiles []string, siblingFiles map[string][]string) (siblingID string, confidence float64) {
	failing := make(map[string]bool, len(failingFiles))
	for _, f := range failingFiles {
		failing[f] = true
	}
	if len(failing) == 0 {
		return "", 0
	}
	ids := make([]string, 0, len(siblingFiles))
	for id := range siblingFiles {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	best, bestScore, tied := "", 0.0, false
	for _, id := range ids {
		touched := make(map[string]bool)
		for _, f := range siblingFiles[id] {
			if failing[f] {
				touched[f] = true
			}
		}
		score := float64(len(touched)) / float64(len(failing))
		switch {
		case score > bestScore:
			best, bestScore, tied = id, score, false
		case score == bestScore && score > 0:
			tied = true
		}
	}
	if tied || bestScore == 0 {
		return "", 0
	}
	return best, bestScore
}

// SiblingShippedFiles loads each sibling's shipped file paths, keyed by item ID.
func (b *Backlog) SiblingShippedFiles(ctx context.Context, siblingIDs []string) (map[string][]string, error) {
	out := make(map[string][]string, len(siblingIDs))
	for _, id := range siblingIDs {
		item, err := b.GetBacklogItem(ctx, id)
		if errors.Is(err, domain.ErrNotFound) {
			continue // a hard-deleted sibling contributes no files
		}
		if err != nil {
			return nil, err
		}
		out[id] = ShippedFilePaths(item.ShippedFileStats)
	}
	return out, nil
}
