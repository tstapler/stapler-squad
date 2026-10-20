package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// WakeTrigger is a durable scheduled or event-driven prompt for a session
// (ADR-006, project_plans/notification-actions-and-agent-messaging). Claiming
// and delivering are separate facts: a row is claimed by a guarded UPDATE and
// only a confirmed pane write sets delivered_at. Transitions between status
// values are validated in Go by domain.WakeStatus (see
// session/ent_repository_wake.go); the schema stores strings, like the other
// enums in this package.
type WakeTrigger struct {
	ent.Schema
}

// Fields of the WakeTrigger.
func (WakeTrigger) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("dedup_key").
			NotEmpty().
			Unique().
			Comment("Non-null unique key derived from owner + type + canonical spec + action (domain.NewWakeDedupKey). Rewritten to <key>#<id> in the UPDATE that makes a row terminal, so it is unique among live rows only (ADR-006 rule 12)."),
		field.String("owner_session_uuid").
			NotEmpty().
			Comment("Loose string reference to the session to wake; not an ent edge, mirroring ItemSession.session_uuid."),
		field.String("trigger_type").
			Comment("Validated in Go by domain.WakeTriggerType.IsValid()."),
		field.String("spec_json").
			Default("{}").
			Comment("Per-type domain.WakeSpec, JSON-encoded."),
		field.String("action_kind").
			Comment("Validated in Go by domain.WakeActionKind.IsValid(): steer_text or notify_operator."),
		field.String("action_payload").
			Default(""),
		field.String("status").
			Default("pending").
			Comment("Validated in Go by domain.WakeStatus: pending|claimed|delivered|needs_revive|missed|cancelled|skipped."),
		field.String("end_reason").
			Optional().
			Default("").
			Comment("domain.WakeReason of the missed or skipped transition."),
		field.Time("claimed_at").
			Optional().
			Nillable(),
		field.Time("delivered_at").
			Optional().
			Nillable().
			Comment("Idempotency fact: set only by MarkDelivered after a confirmed pane write."),
		field.Int("delivery_attempts").
			Default(0).
			Comment("Pane-write attempts."),
		field.Int("rate_limit_deferrals").
			Default(0).
			Comment("Consecutive claim-time rate-limit deferrals. Reset by a successful claim or a cron SkipOccurrence; persisted so a restart keeps it."),
		field.Time("next_fire_at").
			Optional().
			Nillable().
			Comment("Set for time-based types; NULL for event types."),
		field.Int("fire_count").
			Default(0),
		field.Int("max_fires").
			Default(1),
		field.Int("generation").
			Default(0).
			Comment("Chain depth: a woken turn that schedules another wake creates generation+1."),
		field.Bool("allow_revive").
			Default(false),
		field.Time("expires_at").
			Optional().
			Nillable(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("ended_at").
			Optional().
			Nillable().
			Comment("When the row last entered a terminal state; drives the 30-day retention sweep."),
	}
}

// Edges of the WakeTrigger.
func (WakeTrigger) Edges() []ent.Edge {
	return nil
}

// Indexes of the WakeTrigger.
func (WakeTrigger) Indexes() []ent.Index {
	return []ent.Index{
		// Timer scan: pending rows by earliest next_fire_at.
		index.Fields("status", "next_fire_at"),
		// Event dispatcher index and reconcile by trigger type.
		index.Fields("trigger_type", "status"),
		// Per-owner pending cap count and list.
		index.Fields("owner_session_uuid", "status"),
	}
}
