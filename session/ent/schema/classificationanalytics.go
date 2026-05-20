package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ClassificationAnalytics holds the schema definition for the ClassificationAnalytics entity.
type ClassificationAnalytics struct {
	ent.Schema
}

// Fields of the ClassificationAnalytics.
func (ClassificationAnalytics) Fields() []ent.Field {
	return []ent.Field{
		field.String("analytics_id").
			Unique().
			NotEmpty(),
		field.String("session_id").
			Optional(),
		field.String("tool_name").
			NotEmpty(),
		field.String("command_preview").
			Optional(),
		field.String("cwd").
			Optional(),
		field.String("decision").
			NotEmpty(),
		field.String("risk_level").
			NotEmpty(),
		field.String("rule_id").
			Optional(),
		field.String("rule_name").
			Optional(),
		field.String("reason").
			Optional(),
		field.String("alternative").
			Optional(),
		field.Int64("duration_ms").
			Default(0),
		field.String("approval_id").
			Optional(),
		field.String("command_program").
			Optional(),
		field.String("command_category").
			Optional(),
		field.String("command_subcategory").
			Optional(),
		field.Strings("python_imports").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the ClassificationAnalytics.
func (ClassificationAnalytics) Edges() []ent.Edge {
	return nil
}

// Indexes of the ClassificationAnalytics.
func (ClassificationAnalytics) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("session_id"),
		index.Fields("decision"),
		index.Fields("risk_level"),
		index.Fields("rule_id"),
		index.Fields("created_at"),
		index.Fields("command_program", "created_at"), // NEW: AC-2
	}
}
