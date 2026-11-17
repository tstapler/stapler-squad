package session

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/stagecfg"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// BenchmarkGatedTransitionDecision measures the gate's engine decision (caller
// lookup aside) for the three modes, excluding the item read and the write.
// INFERRED ceiling (ADR-007 D10): under 50 microseconds per call excluding I/O.
// Run with `&` per docs/reference/benchmarks.md.
func BenchmarkGatedTransitionDecision(b *testing.B) {
	repo := NewTestEntRepository(b)
	if err := workflowengine.EnsureBuiltInWorkflowStages(context.Background(), repo.Client); err != nil {
		b.Fatal(err)
	}
	cfg, err := workflowengine.NewConfiguredWorkflowEngine(stagecfg.NewEntStageConfigRepository(repo.Client), stagecfg.NewEntGateSatisfactionRepository(repo.Client), nil)
	if err != nil {
		b.Fatal(err)
	}
	def := workflowengine.NewDefaultWorkflowEngine()
	item := &backlogdto.BacklogItemData{ID: "00000000-0000-0000-0000-000000000001", Status: string(domain.BacklogStatusIdea)}
	req := backlogdto.TransitionRequest{Caller: domain.CallerOperatorTransitionRPC, ItemID: item.ID, To: domain.BacklogStatusRefining}
	spec, ok := domain.LookupTransitionCaller(req.Caller)
	if !ok {
		b.Fatal("caller not registered")
	}

	for _, m := range []struct {
		name string
		mode workflowengine.EngineMode
	}{{"off", workflowengine.ModeOff}, {"shadow", workflowengine.ModeShadow}, {"enforce", workflowengine.ModeEnforce}} {
		view := workflowengine.NewSwitchableWorkflowEngine(def, cfg, allClasses(m.mode)).ForCaller(spec)
		b.Run(m.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if v := backlogdto.DecideTransition(view, item, req); v.Err != nil {
					b.Fatal(v.Err)
				}
			}
		})
	}
}
