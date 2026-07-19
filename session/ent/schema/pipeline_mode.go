package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// PipelineMode holds the schema definition for the PipelineMode entity.
type PipelineMode struct{ ent.Schema }

// Fields of the PipelineMode.
func (PipelineMode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("slug").Unique().NotEmpty(),
		field.String("name").NotEmpty(),
		field.String("description").Optional(),
		field.Bool("enabled").Default(true),
		field.String("status_command_template").
			Comment("Rendered content for the pipeline's /status slash command."),
		field.String("done_command_template").
			Comment("Rendered content for the pipeline's /done slash command."),
		field.String("fail_command_template").
			Comment("Rendered content for the pipeline's /fail slash command."),
		field.String("review_command_template").
			Comment("Rendered content for the pipeline's /review slash command."),
		field.String("ship_command_template").
			Comment("Rendered content for the pipeline's /ship slash command."),
		field.String("help_command_template").
			Comment("Rendered content for the pipeline's /help slash command."),
		field.String("triage_prompt_template").
			Comment("Prompt template used for headless triage under this pipeline mode."),
		field.String("review_prompt_template").
			Comment("Prompt template used for review under this pipeline mode."),
		field.String("initial_prompt_template").
			Comment("Prompt template used to seed the initial session under this pipeline mode."),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

// Edges of the PipelineMode.
func (PipelineMode) Edges() []ent.Edge { return nil }

// Indexes of the PipelineMode.
func (PipelineMode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug"),
		index.Fields("enabled"),
		index.Fields("created_at"),
	}
}
