package session

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ─── fakePipelineModeRepository ─────────────────────────────────────────────

// fakePipelineModeRepository is a test double for PipelineModeRepository.
// Only ListEnabled is exercised by pipeline_engine.go — the remaining methods
// return an error if called, since no test in this file should reach them.
type fakePipelineModeRepository struct {
	listEnabledFn func(ctx context.Context) ([]*ent.PipelineMode, error)
}

var _ PipelineModeRepository = (*fakePipelineModeRepository)(nil)

func (f *fakePipelineModeRepository) Create(context.Context, PipelineModeCreateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.Create: not implemented")
}

func (f *fakePipelineModeRepository) Update(context.Context, uuid.UUID, PipelineModeUpdateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.Update: not implemented")
}

func (f *fakePipelineModeRepository) Delete(context.Context, uuid.UUID) error {
	return errors.New("fakePipelineModeRepository.Delete: not implemented")
}

func (f *fakePipelineModeRepository) GetByID(context.Context, uuid.UUID) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.GetByID: not implemented")
}

func (f *fakePipelineModeRepository) GetBySlug(context.Context, string) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.GetBySlug: not implemented")
}

func (f *fakePipelineModeRepository) ListAll(context.Context) ([]*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.ListAll: not implemented")
}

func (f *fakePipelineModeRepository) ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error) {
	return f.listEnabledFn(ctx)
}

// warningLogMu and swapWarningLog live in session/sync_buffer_test.go, shared
// with session/review_gate_test.go and session/backlog_lifecycle_test.go.

// ─── Story 1.3.1: interface shape ───────────────────────────────────────────

// ─── Story 1.3.2: pipelineModeCache concurrency ─────────────────────────────

// ─── Story 1.3.3: CachingPipelineEngine fail-closed resolution ─────────────

// ─── Story 1.3.3d/1.3.3e: renderTemplate + resolved-mode rendering ─────────

// ─── Story 1.7.3: isolated commit gate — real SQL-level injection ─────────

// ─── Story 2.1.1: ExecutorFor ───────────────────────────────────────────────

// ─── Story 2.1.2: ComputeExecutorHash ───────────────────────────────────────
