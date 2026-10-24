package session

import (
	"context"
	"testing"
)

// BenchmarkSwitchableWorkflowEngine_CanTransition compares the selector's
// three modes against the bare Default engine. INFERRED ceiling: Shadow adds
// under 20 microseconds per call, with no allocation growth on sampled-out
// reads. Run with `&` per docs/reference/benchmarks.md.
func BenchmarkSwitchableWorkflowEngine_CanTransition(b *testing.B) {
	repo := NewTestEntRepository(b)
	if err := EnsureBuiltInWorkflowStages(context.Background(), repo.client); err != nil {
		b.Fatal(err)
	}
	cfg, err := NewConfiguredWorkflowEngine(NewEntStageConfigRepository(repo.client), nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	def := NewDefaultWorkflowEngine()

	b.Run("default", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			def.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
		}
	})
	for _, m := range []struct {
		name string
		mode EngineMode
	}{{"off", ModeOff}, {"shadow", ModeShadow}, {"enforce", ModeEnforce}} {
		sel := NewSwitchableWorkflowEngine(def, cfg, fixedMode{m.mode})
		b.Run(m.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				sel.CanTransition(BacklogStatusIdea, BacklogStatusReady, nil)
			}
		})
	}
}
