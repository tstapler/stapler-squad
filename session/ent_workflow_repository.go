package session

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/workflow"
)

// EntWorkflowRepository implements WorkflowRepository using the ent ORM.
type EntWorkflowRepository struct {
	client *ent.Client
}

// NewEntWorkflowRepository creates a new ent-backed workflow repository.
func NewEntWorkflowRepository(client *ent.Client) *EntWorkflowRepository {
	return &EntWorkflowRepository{client: client}
}

// Create inserts a new workflow definition.
// Returns ent.ConstraintError when a duplicate slug exists.
func (r *EntWorkflowRepository) Create(ctx context.Context, w WorkflowCreateInput) (*ent.Workflow, error) {
	c := r.client.Workflow.Create().
		SetSlug(w.Slug).
		SetName(w.Name).
		SetCommand(w.Command).
		SetCronEnabled(w.CronEnabled)

	if w.Description != "" {
		c.SetDescription(w.Description)
	}
	if w.TargetDirectory != "" {
		c.SetTargetDirectory(w.TargetDirectory)
	}
	if w.InputTemplate != "" {
		c.SetInputTemplate(w.InputTemplate)
	}
	if w.SessionType != "" {
		c.SetSessionType(w.SessionType)
	}
	if w.Model != "" {
		c.SetModel(w.Model)
	}
	if w.AgentType != "" {
		c.SetAgentType(w.AgentType)
	}
	if w.CronExpression != "" {
		c.SetCronExpression(w.CronExpression)
	}

	wf, err := c.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: workflow with slug %q already exists", ErrConflict, w.Slug)
		}
		return nil, fmt.Errorf("create workflow: %w", err)
	}
	return wf, nil
}

// Update applies a partial update to an existing workflow by UUID.
func (r *EntWorkflowRepository) Update(ctx context.Context, id uuid.UUID, w WorkflowUpdateInput) (*ent.Workflow, error) {
	u := r.client.Workflow.UpdateOneID(id)

	if w.Name != nil {
		u.SetName(*w.Name)
	}
	if w.Description != nil {
		u.SetDescription(*w.Description)
	}
	if w.Command != nil {
		u.SetCommand(*w.Command)
	}
	if w.TargetDirectory != nil {
		u.SetTargetDirectory(*w.TargetDirectory)
	}
	if w.InputTemplate != nil {
		u.SetInputTemplate(*w.InputTemplate)
	}
	if w.SessionType != nil {
		u.SetSessionType(*w.SessionType)
	}
	if w.Model != nil {
		u.SetModel(*w.Model)
	}
	if w.AgentType != nil {
		u.SetAgentType(*w.AgentType)
	}
	if w.CronExpression != nil {
		u.SetCronExpression(*w.CronExpression)
	}
	if w.CronEnabled != nil {
		u.SetCronEnabled(*w.CronEnabled)
	}

	wf, err := u.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: workflow %s", ErrNotFound, id)
		}
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("%w: slug already exists", ErrConflict)
		}
		return nil, fmt.Errorf("update workflow %s: %w", id, err)
	}
	return wf, nil
}

// Delete removes a workflow by UUID.
func (r *EntWorkflowRepository) Delete(ctx context.Context, id uuid.UUID) error {
	err := r.client.Workflow.DeleteOneID(id).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("%w: workflow %s", ErrNotFound, id)
		}
		return fmt.Errorf("delete workflow %s: %w", id, err)
	}
	return nil
}

// GetByID retrieves a workflow by UUID.
func (r *EntWorkflowRepository) GetByID(ctx context.Context, id uuid.UUID) (*ent.Workflow, error) {
	wf, err := r.client.Workflow.Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: workflow %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get workflow %s: %w", id, err)
	}
	return wf, nil
}

// GetBySlug retrieves a workflow by slug.
func (r *EntWorkflowRepository) GetBySlug(ctx context.Context, slug string) (*ent.Workflow, error) {
	wf, err := r.client.Workflow.Query().
		Where(workflow.Slug(slug)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: workflow with slug %q", ErrNotFound, slug)
		}
		return nil, fmt.Errorf("get workflow by slug %q: %w", slug, err)
	}
	return wf, nil
}

// ListAll returns all workflows sorted ascending by created_at.
// A safety cap of 1000 is applied to prevent runaway queries.
func (r *EntWorkflowRepository) ListAll(ctx context.Context) ([]*ent.Workflow, error) {
	wfs, err := r.client.Workflow.Query().
		Order(ent.Asc(workflow.FieldCreatedAt)).
		Limit(1000).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all workflows: %w", err)
	}
	return wfs, nil
}

// ListEnabled returns only workflows where cron_enabled is true.
func (r *EntWorkflowRepository) ListEnabled(ctx context.Context) ([]*ent.Workflow, error) {
	wfs, err := r.client.Workflow.Query().
		Where(workflow.CronEnabled(true)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled workflows: %w", err)
	}
	return wfs, nil
}
