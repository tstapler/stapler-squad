package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// TaggingRuleFire records a single instant a TaggingRule matched (per
// classifier.TagMatch), independent of whether the resulting tag survived
// suppression filtering. Deliberately a separate, minimal table from
// ClassificationAnalytics (whose fields are approval/command-decision-specific) —
// this table exists purely to back "Fires(7d)" aggregation for tagging rules.
type TaggingRuleFire struct {
	ent.Schema
}

// Fields of the TaggingRuleFire.
func (TaggingRuleFire) Fields() []ent.Field {
	return []ent.Field{
		field.String("rule_id").
			NotEmpty(),
		field.Time("fired_at").
			Default(time.Now),
	}
}

// Edges of the TaggingRuleFire.
func (TaggingRuleFire) Edges() []ent.Edge {
	return nil
}

// Indexes of the TaggingRuleFire.
func (TaggingRuleFire) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("rule_id"),
		index.Fields("fired_at"),
	}
}
