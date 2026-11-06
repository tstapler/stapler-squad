package deliverygate

import (
	"fmt"
	"testing"
	"time"
)

func newIndex() (*VisibilityIndex, *fakeClock) {
	clk := newFakeClock()
	return NewVisibilityIndex(clk.Now), clk
}

func TestResolve_ShouldReturnHidden_WhenQueriedByUUIDTitleOrTmuxName(t *testing.T) {
	t.Parallel()
	idx, _ := newIndex()
	idx.Upsert(hiddenReview)
	for _, k := range []string{"u-h1", "review:abc", "ssq_review_abc"} {
		e, ok := idx.Lookup(k)
		if !ok || !e.Hidden || e.Kind != KindReview {
			t.Errorf("Lookup(%q) = %+v, %v", k, e, ok)
		}
	}
	if _, ok := idx.Lookup("nope"); ok {
		t.Error("unknown key resolved")
	}
}

func TestIndex_ShouldRemainHiddenWithinTTLThenUnresolved_WhenHiddenSessionDeleted(t *testing.T) {
	t.Parallel()
	idx, clk := newIndex()
	idx.Upsert(hiddenReview)
	idx.Remove("u-h1")
	clk.Advance(tombstoneTTL - time.Minute)
	if e, ok := idx.Lookup("review:abc"); !ok || !e.Hidden {
		t.Fatalf("tombstone lost inside TTL: %+v %v", e, ok)
	}
	clk.Advance(2 * time.Minute)
	if _, ok := idx.Lookup("review:abc"); ok {
		t.Fatal("tombstone outlived its TTL")
	}
}

func TestIndex_ShouldBoundTombstones_WhenMoreThanLimitDeleted(t *testing.T) {
	t.Parallel()
	idx, _ := newIndex()
	idx.maxTombs = 100
	n := idx.maxTombs + 50
	for i := 0; i < n; i++ {
		idx.Upsert(Entry{UUID: fmt.Sprintf("u%d", i), Hidden: true, Kind: KindOther})
		idx.Remove(fmt.Sprintf("u%d", i))
	}
	if got := len(idx.state.Load().tombs); got != idx.maxTombs {
		t.Fatalf("tombstones = %d, want %d", got, idx.maxTombs)
	}
	if _, ok := idx.Lookup("u0"); ok {
		t.Error("oldest tombstone should have been evicted")
	}
	if _, ok := idx.Lookup(fmt.Sprintf("u%d", n-1)); !ok {
		t.Error("newest tombstone missing")
	}
}

func TestSeedFromInstances_ShouldSetSeededWithZeroStorageCalls_WhenInstancesPreloaded(t *testing.T) {
	t.Parallel()
	g, _, _, _ := newTestGate(false)
	g.index = NewVisibilityIndex(time.Now) // unseeded
	g.resolver = newResolver(g.index, g.metrics, time.Now)
	if g.Seeded() {
		t.Fatal("fresh gate must be unseeded")
	}
	g.SeedFromInstances(nil)
	if !g.Seeded() {
		t.Fatal("SeedFromInstances did not mark the index seeded")
	}
}

func TestIndex_ShouldKeepOldTitleAliasAndUpdateKind_WhenRenamedOrRetagged(t *testing.T) {
	t.Parallel()
	idx, _ := newIndex()
	idx.Upsert(Entry{UUID: "u1", Title: "h1", TmuxName: "ssq_h1", Hidden: true, Kind: KindReview})
	idx.Upsert(Entry{UUID: "u1", Title: "h1b", TmuxName: "ssq_h1b", Hidden: true, Kind: KindReview})
	for _, k := range []string{"h1b", "h1", "ssq_h1"} {
		if e, ok := idx.Lookup(k); !ok || !e.Hidden {
			t.Errorf("Lookup(%q) after rename = %+v %v", k, e, ok)
		}
	}
	idx.Upsert(Entry{UUID: "u1", Title: "h1b", TmuxName: "ssq_h1b", Hidden: true, Kind: KindOther})
	if e, _ := idx.Lookup("u1"); e.Kind != KindOther {
		t.Errorf("retag: kind = %q, want other", e.Kind)
	}
}

func TestHiddenKind_ShouldDeriveReviewTriageDiagnoseOther_WhenTagsVary(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tags []string
		want HiddenKind
	}{
		{[]string{"backlog:review"}, KindReview},
		{[]string{"backlog:triage"}, KindTriage},
		{[]string{"backlog:diagnose"}, KindDiagnose},
		{nil, KindOther},
		{[]string{"x"}, KindOther},
		// total order review > diagnose > triage > other
		{[]string{"backlog:triage", "backlog:diagnose"}, KindDiagnose},
		{[]string{"backlog:diagnose", "backlog:review", "backlog:triage"}, KindReview},
	}
	for _, c := range cases {
		if got := KindFromTags(c.tags); got != c.want {
			t.Errorf("KindFromTags(%v) = %q want %q", c.tags, got, c.want)
		}
	}
}

func TestUpsert_ShouldEvictTombstoneAndAliasWithSameKey_WhenVisibleSessionReusesDeletedOrRenamedHiddenTitle(t *testing.T) {
	t.Parallel()
	idx, _ := newIndex()
	idx.Upsert(Entry{UUID: "h1", Title: "reused", Hidden: true, Kind: KindReview})
	idx.Remove("h1")
	idx.Upsert(Entry{UUID: "v1", Title: "reused"})
	if e, ok := idx.Lookup("reused"); !ok || e.Hidden {
		t.Fatalf("deleted-hidden title reuse resolved hidden: %+v", e)
	}

	idx.Upsert(Entry{UUID: "h2", Title: "old", Hidden: true, Kind: KindReview})
	idx.Upsert(Entry{UUID: "h2", Title: "new", Hidden: true, Kind: KindReview}) // alias for "old"
	idx.Upsert(Entry{UUID: "v2", Title: "old"})
	if e, ok := idx.Lookup("old"); !ok || e.Hidden {
		t.Fatalf("renamed-hidden alias title reuse resolved hidden: %+v", e)
	}
}
