package deliverygate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/pkg/events"
)

func pruneResolver() (*Resolver, *VisibilityIndex, *fakeClock) {
	clock := newFakeClock()
	idx := NewVisibilityIndex(clock.Now)
	return newResolver(idx, NewMetrics(), clock.Now), idx, clock
}

func TestLookupForm_ShouldReportUuidTitleTmuxAliasAndTombstone(t *testing.T) {
	_, idx, _ := pruneResolver()
	idx.Upsert(Entry{UUID: "u-1", Title: "Old", TmuxName: "tmux-old", Hidden: true, Kind: KindReview})

	for key, want := range map[string]MatchForm{"u-1": FormUUID, "Old": FormTitle, "tmux-old": FormTmux} {
		_, form, ok := idx.LookupForm(key)
		assert.True(t, ok, key)
		assert.Equal(t, want, form, key)
	}

	idx.Upsert(Entry{UUID: "u-1", Title: "New", TmuxName: "tmux-new", Hidden: true, Kind: KindReview})
	_, form, ok := idx.LookupForm("Old")
	assert.True(t, ok)
	assert.Equal(t, FormAlias, form)

	idx.Remove("u-1")
	e, form, ok := idx.LookupForm("New")
	assert.True(t, ok)
	assert.Equal(t, FormTombstone, form)
	assert.True(t, e.Hidden)

	_, _, ok = idx.LookupForm("never-seen")
	assert.False(t, ok)
}

func TestClassifyStored_ShouldResolveEveryIDFormAndKeepAmbiguousOrUnknownRowsUndeterminable(t *testing.T) {
	r, idx, _ := pruneResolver()
	idx.Replace([]Entry{
		{UUID: "h-uuid", Title: "Hidden One", TmuxName: "tmux-h", Hidden: true, Kind: KindReview},
		{UUID: "v-uuid", Title: "Visible One", TmuxName: "tmux-v"},
	})

	cases := []struct {
		name     string
		session  string
		meta     map[string]string
		class    RowClass
		form     MatchForm
		title    string
		byAlias  bool
		wantKind HiddenKind
	}{
		{"uuid", "h-uuid", nil, RowHidden, FormUUID, "Hidden One", false, KindReview},
		{"title", "Hidden One", nil, RowHidden, FormTitle, "Hidden One", false, KindReview},
		{"tmux", "tmux-h", nil, RowHidden, FormTmux, "Hidden One", false, KindReview},
		{"visible", "Visible One", nil, RowVisible, FormTitle, "Visible One", false, ""},
		{"unresolved", "gone-session", nil, RowUndeterminable, "", "", false, ""},
		{"system id", "system", nil, RowUndeterminable, "", "", false, ""},
		{"item id resolves hidden", "backlog-item", map[string]string{events.MetadataKeyItemID: "h-uuid"}, RowHidden, FormUUID, "Hidden One", false, KindReview},
		{"hidden session id but visible item id", "h-uuid", map[string]string{events.MetadataKeyItemID: "v-uuid"}, RowUndeterminable, "", "", false, ""},
		{"visible session id but hidden item id", "v-uuid", map[string]string{events.MetadataKeyItemID: "h-uuid"}, RowUndeterminable, "", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := r.ClassifyStored(c.session, c.meta)
			assert.Equal(t, c.class, got.Class)
			assert.Equal(t, c.form, got.Form)
			assert.Equal(t, c.title, got.Title)
			assert.Equal(t, c.byAlias, got.ByAlias)
			assert.Equal(t, c.wantKind, got.Kind)
		})
	}
}

func TestClassifyStored_ShouldFlagAliasAndTombstoneMatchesAndNeverLetAVisibleTitleReuseBeHidden(t *testing.T) {
	r, idx, _ := pruneResolver()
	idx.Replace([]Entry{{UUID: "h-uuid", Title: "Review A", Hidden: true, Kind: KindReview}})
	idx.Upsert(Entry{UUID: "h-uuid", Title: "Review B", Hidden: true, Kind: KindReview}) // rename: "Review A" aliases

	got := r.ClassifyStored("Review A", nil)
	assert.Equal(t, RowHidden, got.Class)
	assert.True(t, got.ByAlias)
	assert.Equal(t, FormAlias, got.Form)
	assert.Equal(t, "Review B", got.Title)

	// A visible session reusing the old title evicts the alias.
	idx.Upsert(Entry{UUID: "v-uuid", Title: "Review A"})
	got = r.ClassifyStored("Review A", nil)
	assert.Equal(t, RowVisible, got.Class)
	assert.False(t, got.ByAlias)

	idx.Remove("h-uuid")
	got = r.ClassifyStored("Review B", nil)
	assert.Equal(t, RowHidden, got.Class)
	assert.Equal(t, FormTombstone, got.Form)
	assert.True(t, got.ByAlias)
}

func TestClassifyStored_ShouldNotCountMissesOrScheduleRefresh(t *testing.T) {
	r, _, _ := pruneResolver()
	m := r.metrics
	before := m.Snapshot()
	r.ClassifyStored("unknown", nil)
	assert.Equal(t, before, m.Snapshot())
	assert.Empty(t, r.missed)
}
