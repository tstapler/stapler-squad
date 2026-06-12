package session

import (
	"context"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
)

// WorkflowRepository defines persistence operations for workflow definitions.
type WorkflowRepository interface {
	Create(ctx context.Context, w WorkflowCreateInput) (*ent.Workflow, error)
	Update(ctx context.Context, id uuid.UUID, w WorkflowUpdateInput) (*ent.Workflow, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*ent.Workflow, error)
	GetBySlug(ctx context.Context, slug string) (*ent.Workflow, error)
	ListAll(ctx context.Context) ([]*ent.Workflow, error)
	ListEnabled(ctx context.Context) ([]*ent.Workflow, error) // cron_enabled=true
}

// WorkflowCreateInput holds the fields for creating a new workflow.
type WorkflowCreateInput struct {
	Slug            string
	Name            string
	Description     string
	Command         string
	TargetDirectory string
	InputTemplate   string
	SessionType     string
	Model           string
	AgentType       string
	CronExpression  string
	CronEnabled     bool
}

// WorkflowUpdateInput holds optional fields for updating an existing workflow.
// Pointer fields are only applied when non-nil (partial update).
type WorkflowUpdateInput struct {
	Name            *string
	Description     *string
	Command         *string
	TargetDirectory *string
	InputTemplate   *string
	SessionType     *string
	Model           *string
	AgentType       *string
	CronExpression  *string
	CronEnabled     *bool
}
