package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// DismissedFinding holds the schema definition for the DismissedFinding
// entity — a persisted record that a WasteFinding (session/tokens/findings.go)
// has been dismissed from the Insights findings panel. Keyed by finding_id,
// a stable content-addressed hash (see ComputeFindingID) rather than a raw
// (session_id, finding_type) pair, so a still-active session whose finding
// content changes produces a new finding_id and is never silently
// suppressed by an old dismissal.
type DismissedFinding struct {
	ent.Schema
}

// Fields of the DismissedFinding.
func (DismissedFinding) Fields() []ent.Field {
	return []ent.Field{
		field.String("finding_id").
			Unique().
			NotEmpty(),
		field.String("session_id").
			Optional(),
		field.String("conversation_id").
			Optional(),
		field.Int32("finding_type").
			Default(0),
		field.Time("dismissed_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the DismissedFinding.
func (DismissedFinding) Edges() []ent.Edge {
	return nil
}

// Indexes of the DismissedFinding.
func (DismissedFinding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("finding_id"),
	}
}
