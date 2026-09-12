package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// GuidanceRequest is a durable record of a single question an agent (a
// backlog session, automated triage, or a background workflow) asks the
// human, awaiting or holding an answer. See ADR-001
// (project_plans/durable-guidance-request/decisions/ADR-001-standalone-
// guidance-request-entity.md) for why this is a standalone entity rather than
// a StuckReason: two of its three scopes (session, standalone) have no
// BacklogItem to attach to, and it carries a typed answer/options payload
// StuckReason has no room for.
//
// Status is derived, not stored: pending (answered_at AND cancelled_at both
// NULL), cancelled (cancelled_at set — terminal, dominates even if
// answered_at is also set), otherwise answered. See GuidanceRequestData.Status
// in session/ent_repository_guidance.go.
type GuidanceRequest struct {
	ent.Schema
}

// Fields of the GuidanceRequest.
func (GuidanceRequest) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New),
		field.String("scope").
			Comment("Validated in Go by domain.RequestScope.IsValid(); one of backlog-item/session/standalone."),
		field.UUID("item_id", uuid.UUID{}).
			Optional().
			Nillable().
			Comment("Set only for scope=backlog-item."),
		field.String("session_uuid").
			Optional().
			Comment("Set only for scope=session. A loose string reference to ItemSession.session_uuid, NOT an ent edge — mirrors that field's cross-package-cycle-avoidance convention."),
		field.String("scope_key").
			Default("").
			Comment("Always-non-null derived dedup/cap key: item_id's string form for scope=backlog-item, session_uuid for scope=session, \"\" for scope=standalone. NOT nullable, unlike item_id/session_uuid themselves — deliberately: SQLite (and standard SQL) treats every NULL as distinct from every other NULL even inside a composite UNIQUE index, so a dedup index built directly from the nullable item_id/session_uuid columns would never actually collide for two identical requests (verified empirically: two rows with matching non-null columns but both-NULL item_id inserted without conflict). scope_key exists purely to give the dedup index and the cap-check COUNT(*) query a concrete, comparable value in every row."),
		field.String("question_text"),
		field.String("question_type").
			Comment("Validated in Go by domain.QuestionType.IsValid(); one of yes-no/multiple-choice/short-answer."),
		field.String("options").
			Optional().
			Comment("JSON-encoded []string of choice values; only meaningful for question_type=multiple-choice."),
		field.String("answer").
			Optional().
			Comment("Set exactly once, by the answered_at-guarded conditional UPDATE in AnswerGuidanceRequest."),
		field.Time("created_at").
			Default(time.Now).
			Immutable(),
		field.Time("notified_at").
			Optional().
			Nillable().
			Comment("When the asker was actually notified of the answer. NULL = not yet delivered."),
		field.Time("answered_at").
			Optional().
			Nillable().
			Comment("When the human answered. NULL = still pending (unless cancelled)."),
		field.Time("cancelled_at").
			Optional().
			Nillable().
			Comment("When this request was cancelled (e.g. its owning item/session went away before being answered). Terminal — dominates answered_at if somehow both are set."),
	}
}

// Edges of the GuidanceRequest.
func (GuidanceRequest) Edges() []ent.Edge {
	return []ent.Edge{
		// Deliberately NOT .Required() (unlike BacklogStuckState's item edge):
		// 2 of 3 scopes have no BacklogItem at all. Deliberately NOT
		// OnDelete(Cascade) either — see backlog_item.go's guidance_requests
		// edge comment for why a hard delete must not silently destroy a
		// pending/answered-but-undelivered GuidanceRequest row.
		edge.From("item", BacklogItem.Type).
			Ref("guidance_requests").
			Field("item_id").
			Unique(),
	}
}

// Indexes of the GuidanceRequest.
func (GuidanceRequest) Indexes() []ent.Index {
	return []ent.Index{
		// Dedup key, built from scope_key (see its field comment) rather than
		// the raw nullable item_id/session_uuid columns. question_text is
		// deliberately part of it: an item/session can have more than one
		// *different* open question, so the guarantee is "don't create two
		// identical open asks," not "at most one open ask per scope key" —
		// unlike BacklogStuckState's (item_id, reason) key, GuidanceRequest has
		// no closed reason enum to key on instead.
		index.Fields("scope", "scope_key", "question_text").Unique(),
	}
}
