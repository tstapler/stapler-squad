package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Workflow holds the schema definition for the Workflow entity.
type Workflow struct{ ent.Schema }

// Fields of the Workflow.
func (Workflow) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("slug").Unique().NotEmpty(),
		field.String("name").NotEmpty(),
		field.String("description").Optional(),
		field.String("command").NotEmpty(),
		field.String("target_directory").Optional(),
		field.String("input_template").Optional(),
		field.String("session_type").Optional().Default("directory"),
		field.String("model").Optional(),
		field.String("agent_type").Optional(),
		field.String("cron_expression").Optional(),
		field.Bool("cron_enabled").Default(false),
		// enabled is the generic per-trigger-type "is this trigger active" gate, read by
		// both webhook handlers and written by TriggersPanel.tsx's toggle — distinct from
		// cron_enabled, which is now the literal cron-schedule flag only (see
		// validateTriggerTypeFieldConsistency's doc comment in workflow_service.go for
		// the history of why these were conflated). Additive field; existing rows are
		// backfilled by backfillEnabledField (server/workflows/migrate.go) since
		// pre-migration rows only had cron_enabled to express "disabled."
		field.Bool("enabled").Default(true),
		field.Time("created_at").Default(time.Now).Immutable(),
		// updated_at is stored/compared in UTC, not time.Now's default Local zone — see
		// session/ent/schema/backlog_item.go's identical field for the full mechanism
		// (mattn/go-sqlite3 formats a time.Time TEXT column in the value's own
		// Location, so a Local-zoned stored value and a UTC-zoned CAS precondition
		// value — every protobuf Timestamp's AsTime() is UTC — would never byte-match
		// even for the same instant). UpdateWorkflowRequest.expected_updated_at (AC9)
		// is exactly this kind of precondition, so this field must follow the same
		// fix; existing rows are backfilled by workflow_updated_at_utc_migration.go.
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
		field.Int("keep_sessions").Optional().Default(0).
			Comment("Keep only the N most recent sessions per workflow (0 = keep all, disabled)."),
		field.Int("archive_after_hours").Optional().Default(0).
			Comment("Auto-archive completed sessions after this many hours (0 = disabled)."),
		// Trigger fields (webhook-triggers Epic 1.1). trigger_type discriminates which
		// activation mechanism fires this row: "cron" | "github_push" | "webhook" | "manual".
		// Existing rows are backfilled by Scheduler.Start (Task 1.1.1d) since this field
		// didn't previously exist.
		field.String("trigger_type").Optional().Default("manual"),
		field.String("github_repo").Optional(),
		field.String("github_branch").Optional(),
		field.String("webhook_slug").Optional().Unique(),
		field.String("webhook_secret_encrypted").Optional(),
		field.String("event_filter").Optional(),
		field.String("label_filter").Optional(),
		field.String("prompt_template").Optional(),
		field.Time("last_fired_at").Optional().Nillable(),
	}
}

// Edges of the Workflow.
func (Workflow) Edges() []ent.Edge { return nil }

// Indexes of the Workflow.
func (Workflow) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug"),
		index.Fields("cron_enabled"),
		index.Fields("created_at"),
		index.Fields("webhook_slug"),
		index.Fields("trigger_type"),
	}
}
