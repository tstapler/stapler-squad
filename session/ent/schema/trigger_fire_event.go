package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// TriggerFireEvent holds the schema definition for the TriggerFireEvent entity.
// One audit row per trigger evaluation attempt (fired/no-match/rejected), modeled on
// SourceSyncEvent — see project_plans/webhook-triggers/implementation/plan.md Epic 1.2.
type TriggerFireEvent struct {
	ent.Schema
}

// Fields of the TriggerFireEvent.
func (TriggerFireEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		// workflow_id is nil when the request was rejected before a Workflow could be
		// resolved (e.g. an unknown webhook slug) — plain field, not an edge, matching
		// Session.WorkflowID's existing convention (see Pattern Decisions table).
		field.UUID("workflow_id", uuid.UUID{}).
			Optional().
			Nillable(),
		// outcome is a string-enum: "fired_success" | "fired_failed" | "no_match" | "rejected".
		field.String("outcome").
			NotEmpty(),
		// delivery_id is left unset (NULL) rather than "" when no delivery ID is
		// applicable (e.g. cron fires) — required so the composite unique index below
		// only enforces dedup for genuine deliveries, since SQL treats distinct NULLs
		// as non-colliding under a unique index (same reasoning as Workflow.WebhookSlug).
		field.String("delivery_id").
			Optional(),
		field.String("session_id").
			Optional(),
		field.String("error_message").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
	}
}

// Edges of the TriggerFireEvent.
func (TriggerFireEvent) Edges() []ent.Edge { return nil }

// Indexes of the TriggerFireEvent.
func (TriggerFireEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		// Composite, scoped per-workflow — NOT a bare global unique on delivery_id alone.
		// Two different Workflow rows can legitimately match the same inbound delivery
		// (e.g. two github_push triggers both watching main); a global unique index would
		// make the second trigger's Create collide with the first trigger's row and
		// silently never fire, inverting AC12's intent (pre-mortem P1 #1 correction).
		index.Fields("workflow_id", "delivery_id").Unique(),
		// EntTriggerFireEventRepository.ListByWorkflow queries
		// Where(WorkflowID(...)).Order(Desc(FieldCreatedAt)) — the bare
		// created_at index above doesn't cover a workflow_id-scoped lookup
		// (it would still require a full scan filtered by workflow_id), and
		// the (workflow_id, delivery_id) unique index above is keyed for
		// dedup lookups, not for an ordered per-workflow scan. This composite
		// matches ListByWorkflow's actual query shape (sdd:6-verify finding).
		index.Fields("workflow_id", "created_at"),
		// EntTriggerFireEventRepository.ExistsByDeliveryID queries
		// Where(DeliveryID(...)) with no workflow_id filter (it dedups PR-fix webhook
		// deliveries, whose rows always carry WorkflowID: nil per the pr-event-webhooks
		// Migration Plan) — delivery_id is never the leading column of either index
		// above, so without this index that query is a full table scan. Deliberately
		// NOT unique: unlike the composite index above, a single delivery can
		// legitimately produce multiple rows sharing one delivery_id (one per PR number
		// in a check_run's pull_requests array), so a unique constraint here would
		// reject the second PR number's row as a false "duplicate."
		index.Fields("delivery_id"),
	}
}
