package deliverygate

import (
	"sync"
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

func newHiddenInstance(t *testing.T, title string, tags ...string) *session.Instance {
	t.Helper()
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:   title,
		Path:    t.TempDir(),
		Program: "claude",
		Hidden:  true,
		Tags:    tags,
	})
	if err != nil {
		t.Fatalf("NewInstance: %v", err)
	}
	return inst
}

// T-IX-07 (real instances): the seed reads through Snapshot() only and resolves
// by UUID, title and tmux name with no storage or poller involved.
func TestSeedFromInstances_ShouldResolveHiddenByAllIdentityForms_WhenRealInstancesSeeded(t *testing.T) {
	t.Parallel()
	hidden := newHiddenInstance(t, "review-x", "backlog:review")
	visible, err := session.NewInstance(session.InstanceOptions{Title: "work-y", Path: t.TempDir(), Program: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	g, _, _, _ := newTestGate(true)
	g.index = NewVisibilityIndex(g.now)
	g.resolver = newResolver(g.index, g.metrics, g.now)

	g.SeedFromInstances([]*session.Instance{hidden, visible, nil})
	if !g.Seeded() {
		t.Fatal("not seeded")
	}
	snap := hidden.Snapshot()
	for _, k := range []string{snap.UUID, "review-x", EntryFromSnapshot(snap).TmuxName} {
		r := g.Resolver().Resolve(k, nil)
		if r.Visibility != VisibilityHidden || r.Kind != KindReview {
			t.Errorf("Resolve(%q) = %+v", k, r)
		}
	}
	if r := g.Resolver().Resolve("work-y", nil); r.Visibility != VisibilityVisible {
		t.Errorf("visible session resolved %+v", r)
	}
}

// T-IX-17: reading a hidden instance while it is mutated is race-free (-race).
func TestSeed_ShouldReadHiddenViaSnapshot_WhenInstanceMutatedConcurrently(t *testing.T) {
	t.Parallel()
	inst := newHiddenInstance(t, "mut", "backlog:diagnose")
	g, _, _, _ := newTestGate(true)

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				inst.SetCategory("c")
				inst.SetNote("n")
			}
		}
	}()
	for i := 0; i < 200; i++ {
		g.SeedFromInstances([]*session.Instance{inst})
		g.UpsertInstance(inst)
	}
	close(stop)
	wg.Wait()
	if r := g.Resolver().Resolve("mut", nil); r.Visibility != VisibilityHidden || r.Kind != KindDiagnose {
		t.Fatalf("got %+v", r)
	}
}
