package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// AcAuditEvent is the append-only audit trail of every attempted acceptance-criterion
// status change (including waivers) and its outcome. Rows are never updated or deleted by
// application code, and item_id is deliberately a plain field rather than an edge so
// deleting an item does not cascade away the trail.
type AcAuditEvent struct {
	ent.Schema
}

func (AcAuditEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("item_public_id").Optional().
			Comment("The item's bl_ public id at write time, so the trail stays readable by that id after the item is deleted."),
		field.Int("criterion_index"),
		field.String("previous_status").Optional(),
		field.String("new_status").Optional().
			Comment("The status that was requested; equals previous_status when outcome is not applied."),
		field.String("reason").Optional(),
		field.String("outcome").
			Comment("applied, or denied:<why> / rejected:<why> when nothing changed."),
		field.String("actor_class").
			Comment("authenticated_session_cookie, authenticated_bearer_token, agent_session, mcp_session, system or unverified. A claim about the caller, not proof of who the owner is; only a verified session proof proves an agent session."),
		field.String("actor_session_uuid").Optional().
			Comment("Claimed or verified session UUID, see session_verified."),
		field.Bool("session_verified").Default(false),
		field.String("source").
			Comment("ui, api or mcp; ui/api is a browser-header hint, not a security boundary."),
		field.String("remote_addr").Optional(),
		field.String("user_agent").Optional(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (AcAuditEvent) Indexes() []ent.Index {
	return []ent.Index{index.Fields("item_id", "created_at"), index.Fields("item_public_id")}
}
