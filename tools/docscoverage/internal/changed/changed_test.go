package changed_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/changed"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/registry"
)

func reg(fs ...model.Feature) registry.Registry {
	r := registry.Registry{Features: map[model.FeatureID]model.Feature{}}
	for _, f := range fs {
		r.Features[f.ID] = f
	}
	return r
}

func boolp(b bool) *bool { return &b }

func TestSemanticFieldsOnlyFireGate(t *testing.T) {
	t.Parallel()
	base := model.Feature{ID: "f", Name: "N", Description: "D", Files: []string{"a.json"}}
	tests := []struct {
		name string
		edit func(f *model.Feature)
		want changed.Kind
	}{
		{"scanner-owned fields and file move ignored", func(f *model.Feature) { f.Files = []string{"b.json"}; f.Type = "both" }, ""},
		{"name", func(f *model.Feature) { f.Name = "N2" }, changed.Modified},
		{"description", func(f *model.Feature) { f.Description = "D2" }, changed.Modified},
		{"userFacing set", func(f *model.Feature) { f.UserFacing = boolp(false) }, changed.Modified},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			head := base
			tc.edit(&head)
			got := changed.Detect(reg(base), reg(head))
			assert.Equal(t, tc.want, got["f"])
		})
	}
}

func TestRenameSameIdNotNewAndNewIdIsNew(t *testing.T) {
	t.Parallel()
	base := reg(model.Feature{ID: "kept", Name: "K", Files: []string{"old.json"}})
	head := reg(
		model.Feature{ID: "kept", Name: "K", Files: []string{"renamed/new.json"}},
		model.Feature{ID: "fresh", Name: "F", Files: []string{"old.json"}}, // filename existed on base
	)
	got := changed.Detect(base, head)
	assert.Equal(t, map[model.FeatureID]changed.Kind{"fresh": changed.New}, got)
	assert.Equal(t, []model.FeatureID{"fresh"}, changed.Sorted(got))
}
