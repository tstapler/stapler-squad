package session

import (
	"context"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/session/ent"
)

// PipelineModeRepository defines persistence operations for pipeline mode definitions.
type PipelineModeRepository interface {
	Create(ctx context.Context, m PipelineModeCreateInput) (*ent.PipelineMode, error)
	Update(ctx context.Context, id uuid.UUID, m PipelineModeUpdateInput) (*ent.PipelineMode, error)
	Delete(ctx context.Context, id uuid.UUID) error
	GetByID(ctx context.Context, id uuid.UUID) (*ent.PipelineMode, error)
	GetBySlug(ctx context.Context, slug string) (*ent.PipelineMode, error)
	ListAll(ctx context.Context) ([]*ent.PipelineMode, error)
	ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error)
}

// PipelineModeCreateInput holds the fields for creating a new pipeline mode.
type PipelineModeCreateInput struct {
	Slug        string
	Name        string
	Description string
	Enabled     bool

	StatusCommandTemplate string
	DoneCommandTemplate   string
	FailCommandTemplate   string
	ReviewCommandTemplate string
	ShipCommandTemplate   string
	HelpCommandTemplate   string
	TriagePromptTemplate  string
	ReviewPromptTemplate  string
	InitialPromptTemplate string
}

// PipelineModeUpdateInput holds optional fields for updating an existing pipeline mode.
// Pointer fields are only applied when non-nil (partial update).
type PipelineModeUpdateInput struct {
	Name        *string
	Description *string
	Enabled     *bool

	StatusCommandTemplate *string
	DoneCommandTemplate   *string
	FailCommandTemplate   *string
	ReviewCommandTemplate *string
	ShipCommandTemplate   *string
	HelpCommandTemplate   *string
	TriagePromptTemplate  *string
	ReviewPromptTemplate  *string
	InitialPromptTemplate *string
}
