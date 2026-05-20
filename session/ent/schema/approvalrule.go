package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ApprovalRule holds the schema definition for the ApprovalRule entity.
type ApprovalRule struct {
	ent.Schema
}

// Fields of the ApprovalRule.
func (ApprovalRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("rule_id").
			Unique().
			NotEmpty(),
		field.String("name").
			NotEmpty(),
		field.String("tool_name").
			Optional(),
		field.String("tool_pattern").
			Optional(),
		field.String("tool_category").
			Optional(),
		field.String("command_pattern").
			Optional(),
		field.String("file_pattern").
			Optional(),
		field.JSON("criteria_programs", []string{}).
			Optional(),
		field.JSON("criteria_subcommands", []string{}).
			Optional(),
		field.Int("decision"),
		field.Int("risk_level"),
		field.String("reason").
			Optional(),
		field.String("alternative").
			Optional(),
		field.Int("priority").
			Default(0),
		field.Bool("enabled").
			Default(true),
		field.String("source").
			Default("user"),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now),
	}
}

// Edges of the ApprovalRule.
func (ApprovalRule) Edges() []ent.Edge {
	return nil
}

// Indexes of the ApprovalRule.
func (ApprovalRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("rule_id"),
		index.Fields("priority"),
		index.Fields("enabled"),
	}
}
