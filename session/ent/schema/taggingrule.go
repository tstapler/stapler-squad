package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TaggingRule holds the schema definition for the TaggingRule entity — a
// user-editable rule that assigns descriptive tags to sessions based on
// branch/path/program/name patterns and prerequisite tags. Sibling schema to
// ApprovalRule, same shape and CRUD/concurrency guarantees (unique rule_id,
// atomic upsert).
type TaggingRule struct {
	ent.Schema
}

// Fields of the TaggingRule.
func (TaggingRule) Fields() []ent.Field {
	return []ent.Field{
		field.String("rule_id").
			Unique().
			NotEmpty(),
		field.String("name").
			NotEmpty(),
		field.String("name_pattern").
			Optional(),
		field.String("branch_pattern").
			Optional(),
		field.String("path_pattern").
			Optional(),
		field.String("program_pattern").
			Optional(),
		// RequiredTags lists tags that must already be present on the session for this
		// rule to match — how tag-dependency chains are expressed (see
		// pkg/classifier.TaggingEngine.ApplyToFixpoint).
		field.JSON("required_tags", []string{}).
			Optional().
			Default([]string{}),
		field.String("output_tag").
			NotEmpty(),
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

// Edges of the TaggingRule.
func (TaggingRule) Edges() []ent.Edge {
	return nil
}

// Indexes of the TaggingRule.
func (TaggingRule) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("rule_id"),
		index.Fields("priority"),
		index.Fields("enabled"),
	}
}
