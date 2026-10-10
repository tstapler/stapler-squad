package deliverygate

import (
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

// T-MX-11: the three accepted stragglers are delivered (fail open), each
// increments unresolved{class=routine} by exactly one, and the soak canary
// (resolved_later) moves only for sessions that exist in storage.
func TestStragglers_ShouldDeliverAndCountEach_WhenDeletedBeforeRestartUnfedCreatePathOrRenamedBeforeFeed(t *testing.T) {
	t.Parallel()
	storageRows := []session.InstanceData{
		{UUID: "u-unfed", Title: "unfed-create", Hidden: true, Tags: []string{"backlog:review"}},
		{UUID: "u-renamed", Title: "renamed-new", Hidden: true, Tags: []string{"backlog:diagnose"}},
	}
	cases := []struct {
		name, id     string
		wantResolved uint64 // resolved_later increments (the session exists in storage)
	}{
		{"(a) deleted before restart, tombstone gone", "deleted-hidden", 0},
		{"(b) created by an unfed path", "unfed-create", 1},
		{"(c) renamed before the feed point ran", "renamed-new", 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			storage := newFakeLister()
			storage.data = storageRows
			clk := newFakeClock()
			lg, _ := newRecLogger()
			flags := &staticFlags{}
			flags.set(true)
			g := NewGate(WithClock(clk.Now), WithLogger(lg), WithFlagLoader(flags.load), WithInstanceLister(storage))
			g.Flags().Reload()
			// The index knows only the renamed session's OLD title (rename not fed yet).
			g.Index().Replace([]Entry{{UUID: "u-renamed", Title: "renamed-old", Hidden: true, Kind: KindDiagnose}})

			f := g.PublishFilter()
			if !f(notif(c.id, tTaskComplete, nil)) {
				t.Fatal("routine event must fail open")
			}
			if got := g.Metrics().Value(CounterUnresolved, "routine"); got != 1 {
				t.Errorf("unresolved{routine} = %d, want exactly 1", got)
			}
			g.Resolver().wg.Wait() // the singleflight refresh finishes
			if got := g.Metrics().Value(CounterResolvedLater); got != c.wantResolved {
				t.Errorf("resolved_later = %d, want %d", got, c.wantResolved)
			}
			if c.wantResolved == 1 && f(notif(c.id, tTaskComplete, nil)) {
				t.Error("after the refresh primed the index the hidden session must be filtered")
			}
		})
	}
}
