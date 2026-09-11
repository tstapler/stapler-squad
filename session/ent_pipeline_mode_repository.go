package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/pipelinemode"
)

// EntPipelineModeRepository implements PipelineModeRepository using the ent ORM.
type EntPipelineModeRepository struct {
	client *ent.Client
}

// NewEntPipelineModeRepository creates a new ent-backed pipeline mode repository.
func NewEntPipelineModeRepository(client *ent.Client) *EntPipelineModeRepository {
	return &EntPipelineModeRepository{client: client}
}

// Create inserts a new pipeline mode definition.
// Returns ent.ConstraintError when a duplicate slug exists.
func (r *EntPipelineModeRepository) Create(ctx context.Context, m PipelineModeCreateInput) (*ent.PipelineMode, error) {
	// The 9 content-template fields have no ent .Optional()/.Default() (see
	// session/ent/schema/pipeline_mode.go) — they are required at insert time,
	// so unlike WorkflowCreateInput's optional fields, these must always be set
	// (even to an empty string), matching how Slug/Name/Command are set
	// unconditionally in EntWorkflowRepository.Create.
	c := r.client.PipelineMode.Create().
		SetSlug(m.Slug).
		SetName(m.Name).
		SetEnabled(m.Enabled).
		SetStatusCommandTemplate(m.StatusCommandTemplate).
		SetDoneCommandTemplate(m.DoneCommandTemplate).
		SetFailCommandTemplate(m.FailCommandTemplate).
		SetReviewCommandTemplate(m.ReviewCommandTemplate).
		SetShipCommandTemplate(m.ShipCommandTemplate).
		SetHelpCommandTemplate(m.HelpCommandTemplate).
		SetTriagePromptTemplate(m.TriagePromptTemplate).
		SetReviewPromptTemplate(m.ReviewPromptTemplate).
		SetInitialPromptTemplate(m.InitialPromptTemplate)

	if m.Description != "" {
		c.SetDescription(m.Description)
	}

	pm, err := c.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: pipeline mode with slug %q already exists", ErrConflict, m.Slug)
		}
		return nil, fmt.Errorf("create pipeline mode: %w", err)
	}
	return pm, nil
}

// Update applies a partial update to an existing pipeline mode by UUID.
func (r *EntPipelineModeRepository) Update(ctx context.Context, id uuid.UUID, m PipelineModeUpdateInput) (*ent.PipelineMode, error) {
	u := r.client.PipelineMode.UpdateOneID(id)

	if m.Name != nil {
		u.SetName(*m.Name)
	}
	if m.Description != nil {
		u.SetDescription(*m.Description)
	}
	if m.Enabled != nil {
		u.SetEnabled(*m.Enabled)
	}
	if m.StatusCommandTemplate != nil {
		u.SetStatusCommandTemplate(*m.StatusCommandTemplate)
	}
	if m.DoneCommandTemplate != nil {
		u.SetDoneCommandTemplate(*m.DoneCommandTemplate)
	}
	if m.FailCommandTemplate != nil {
		u.SetFailCommandTemplate(*m.FailCommandTemplate)
	}
	if m.ReviewCommandTemplate != nil {
		u.SetReviewCommandTemplate(*m.ReviewCommandTemplate)
	}
	if m.ShipCommandTemplate != nil {
		u.SetShipCommandTemplate(*m.ShipCommandTemplate)
	}
	if m.HelpCommandTemplate != nil {
		u.SetHelpCommandTemplate(*m.HelpCommandTemplate)
	}
	if m.TriagePromptTemplate != nil {
		u.SetTriagePromptTemplate(*m.TriagePromptTemplate)
	}
	if m.ReviewPromptTemplate != nil {
		u.SetReviewPromptTemplate(*m.ReviewPromptTemplate)
	}
	if m.InitialPromptTemplate != nil {
		u.SetInitialPromptTemplate(*m.InitialPromptTemplate)
	}

	pm, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: pipeline mode %s", ErrNotFound, id)
		}
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: slug already exists", ErrConflict)
		}
		return nil, fmt.Errorf("update pipeline mode %s: %w", id, err)
	}
	return pm, nil
}

// Delete removes a pipeline mode by UUID.
func (r *EntPipelineModeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.client.PipelineMode.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: pipeline mode %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete pipeline mode %s: %w", id, err)
	}
	return nil
}

// GetByID retrieves a pipeline mode by UUID.
func (r *EntPipelineModeRepository) GetByID(ctx context.Context, id uuid.UUID) (*ent.PipelineMode, error) {
	pm, err := r.client.PipelineMode.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: pipeline mode %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get pipeline mode %s: %w", id, err)
	}
	return pm, nil
}

// GetBySlug retrieves a pipeline mode by slug.
func (r *EntPipelineModeRepository) GetBySlug(ctx context.Context, slug string) (*ent.PipelineMode, error) {
	pm, err := r.client.PipelineMode.Query().
		Where(pipelinemode.Slug(slug)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: pipeline mode with slug %q", ErrNotFound, slug)
		}
		return nil, fmt.Errorf("get pipeline mode by slug %q: %w", slug, err)
	}
	return pm, nil
}

// ListAll returns all pipeline modes sorted ascending by created_at.
// A safety cap of 1000 is applied to prevent runaway queries.
func (r *EntPipelineModeRepository) ListAll(ctx context.Context) ([]*ent.PipelineMode, error) {
	//nolint:entfullscan capped at Limit(1000) below; doc comment states this explicitly.
	pms, err := r.client.PipelineMode.Query().
		Order(ent.Asc(pipelinemode.FieldCreatedAt)).
		Limit(1000).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all pipeline modes: %w", err)
	}
	return pms, nil
}

// ListEnabled returns only pipeline modes where enabled is true.
func (r *EntPipelineModeRepository) ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error) {
	pms, err := r.client.PipelineMode.Query().
		Where(pipelinemode.Enabled(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled pipeline modes: %w", err)
	}
	return pms, nil
}
