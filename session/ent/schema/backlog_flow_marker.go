package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// BacklogFlowMarker is an append-only log of server-decided flow markers
// (PR-fix outcome and trigger). The table itself is the system-written
// discriminator: only EntRepository.RecordFlowMarker inserts here
// (kinds: pr_fix_outcome, pr_fix_trigger, manual_nudge), so agent
// notes posted via post_backlog_update can never forge a marker.
//
// item_id is deliberately a plain UUID, not an edge: markers are history and
// survive item deletion. kind/value are validated in Go (session.FlowMarkerKind).
type BacklogFlowMarker struct {
	ent.Schema
}

// Fields of the BacklogFlowMarker.
func (BacklogFlowMarker) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("kind").
			NotEmpty().
			Comment("pr_fix_outcome | pr_fix_trigger (session.FlowMarkerKind)."),
		field.String("value").
			NotEmpty().
			Comment("Kind-specific value, e.g. steered or ci."),
		field.Int("pr_number").
			Default(0),
		field.Time("created_at").
			Default(func() time.Time { return time.Now().UTC() }).
			Immutable().
			Comment("UTC instant the marker was recorded."),
	}
}

// Indexes of the BacklogFlowMarker.
func (BacklogFlowMarker) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("kind", "created_at"),
		index.Fields("item_id", "kind", "created_at"),
	}
}
