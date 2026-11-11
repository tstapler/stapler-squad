// Package changed decides which features are new or meaningfully changed
// against the base (ADR-005 decision 4). Only id, name, description and
// userFacing count; moves (component, path, filePath) and scanner-owned fields
// never do, so new scanner fields cannot fire the gate.
package changed

import (
	"sort"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
)

// Kind says why a feature needs docs.
type Kind string

// Reasons a feature counts as changed.
const (
	New      Kind = "new"
	Modified Kind = "changed"
)

// Detect returns the head features that are new to base or whose semantic
// fields differ. Identity is the id, so a renamed file or a feature moved
// between files is not new.
func Detect(base, head registry.Registry) map[model.FeatureID]Kind {
	out := map[model.FeatureID]Kind{}
	for id, h := range head.Features {
		b, existed := base.Features[id]
		switch {
		case !existed:
			out[id] = New
		case semanticDiffers(b, h):
			out[id] = Modified
		}
	}
	return out
}

func semanticDiffers(a, b model.Feature) bool {
	if a.Name != b.Name || a.Description != b.Description {
		return true
	}
	if (a.UserFacing == nil) != (b.UserFacing == nil) {
		return true
	}
	return a.UserFacing != nil && *a.UserFacing != *b.UserFacing
}

// Sorted lists the keys of a Detect result.
func Sorted(m map[model.FeatureID]Kind) []model.FeatureID {
	ids := make([]model.FeatureID, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
