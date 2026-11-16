package session

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/workflowengine"
)

// BenchmarkSwitchableWorkflowEngine_CanTransition compares the selector's
// three modes against the bare Default engine. INFERRED ceiling: Shadow adds
// under 20 microseconds per call, with no allocation growth on sampled-out
// reads. Run with `&` per docs/reference/benchmarks.md.
func BenchmarkSwitchableWorkflowEngine_CanTransition(b *testing.B) {
	repo := NewTestEntRepository(b)
	if err := workflowengine.EnsureBuiltInWorkflowStages(context.Background(), repo.Client); err != nil {
		b.Fatal(err)
	}
	cfg, err := workflowengine.NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.Client), nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	def := workflowengine.NewDefaultWorkflowEngine()

	b.Run("default", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			def.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
		}
	})
	for _, m := range []struct {
		name string
		mode workflowengine.EngineMode
	}{{"off", workflowengine.ModeOff}, {"shadow", workflowengine.ModeShadow}, {"enforce", workflowengine.ModeEnforce}} {
		sel := workflowengine.NewSwitchableWorkflowEngine(def, cfg, fixedMode{m.mode})
		b.Run(m.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sel.CanTransition(domain.BacklogStatusIdea, domain.BacklogStatusReady, nil)
			}
		})
	}
}
