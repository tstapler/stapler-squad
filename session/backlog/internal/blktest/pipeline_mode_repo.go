// Package blktest holds test doubles and fixtures shared by the session/backlog
// package and its extracted sub-packages (pipeline, reviewgate, chains).
package blktest

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// FakePipelineModeRepository is a test double for session.PipelineModeRepository.
// Only ListEnabled is exercised by the pipeline engine; the other methods return
// an error if called.
type FakePipelineModeRepository struct {
	ListEnabledFn func(ctx context.Context) ([]*ent.PipelineMode, error)
}

func (f *FakePipelineModeRepository) Create(context.Context, session.PipelineModeCreateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("FakePipelineModeRepository.Create: not implemented")
}

func (f *FakePipelineModeRepository) Update(context.Context, uuid.UUID, session.PipelineModeUpdateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("FakePipelineModeRepository.Update: not implemented")
}

func (f *FakePipelineModeRepository) Delete(context.Context, uuid.UUID) error {
	return errors.New("FakePipelineModeRepository.Delete: not implemented")
}

func (f *FakePipelineModeRepository) GetByID(context.Context, uuid.UUID) (*ent.PipelineMode, error) {
	return nil, errors.New("FakePipelineModeRepository.GetByID: not implemented")
}

func (f *FakePipelineModeRepository) GetBySlug(context.Context, string) (*ent.PipelineMode, error) {
	return nil, errors.New("FakePipelineModeRepository.GetBySlug: not implemented")
}

func (f *FakePipelineModeRepository) ListAll(context.Context) ([]*ent.PipelineMode, error) {
	return nil, errors.New("FakePipelineModeRepository.ListAll: not implemented")
}

func (f *FakePipelineModeRepository) ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error) {
	return f.ListEnabledFn(ctx)
}
