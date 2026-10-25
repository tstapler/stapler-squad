package session

import (
	"context"
	"testing"
)

// BenchmarkGatedTransitionDecision measures the gate's engine decision (caller
// lookup aside) for the three modes, excluding the item read and the write.
// INFERRED ceiling (ADR-007 D10): under 50 microseconds per call excluding I/O.
// Run with `&` per docs/reference/benchmarks.md.
func BenchmarkGatedTransitionDecision(b *testing.B) {
	repo := NewTestEntRepository(b)
	if err := EnsureBuiltInWorkflowStages(context.Background(), repo.client); err != nil {
		b.Fatal(err)
	}
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), NewEntGateSatisfactionRepository(repo.client), nil)
	if err != nil {
		b.Fatal(err)
	}
	def := NewDefaultWorkflowEngine()
	item := &BacklogItemData{ID: "00000000-0000-0000-0000-000000000001", Status: string(BacklogStatusIdea)}
	req := TransitionRequest{Caller: CallerOperatorTransitionRPC, ItemID: item.ID, To: BacklogStatusRefining}
	spec, ok := LookupTransitionCaller(req.Caller)
	if !ok {
		b.Fatal("caller not registered")
	}

	for _, m := range []struct {
		name string
		mode EngineMode
	}{{"off", ModeOff}, {"shadow", ModeShadow}, {"enforce", ModeEnforce}} {
		view := NewSwitchableWorkflowEngine(def, cfg, allClasses(m.mode)).ForCaller(spec)
		b.Run(m.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if v := decideTransition(view, item, req); v.Err != nil {
					b.Fatal(v.Err)
				}
			}
		})
	}
}
