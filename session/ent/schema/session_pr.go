package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// SessionPR links one PullRequestState to exactly one owner: a session
// (session_uuid) or a backlog item (backlog_item_id). It holds no PR state.
type SessionPR struct {
	ent.Schema
}

var (
	sessionPRRoles   = []string{"primary", "followup", "stacked", "unknown"}
	sessionPRSources = []string{
		"created_from_url", "push_scan", "branch_discovery", "pr_creation_service",
		"report_pr_created", "jules", "backfill", "manual",
	}
)

// exactlyOneOwnerCheck is the database CHECK for "exactly one owner string is
// non-empty". It is a table constraint, not an ent hook: a schema hook moves ent's
// runtime stitching into an ent/runtime package nothing imports.
var exactlyOneOwnerCheck = entsql.Checks(map[string]string{
	"exactly_one_owner": "((session_uuid <> '' AND backlog_item_id = '') OR (session_uuid = '' AND backlog_item_id <> ''))",
})

// Annotations of the SessionPR.
func (SessionPR) Annotations() []schema.Annotation {
	return []schema.Annotation{exactlyOneOwnerCheck}
}

// Fields of the SessionPR.
func (SessionPR) Fields() []ent.Field {
	return []ent.Field{
		field.Int("pull_request_state_id").Immutable(),
		field.String("session_uuid").Default("").Immutable().Comment("Loose FK to Session; exactly one of session_uuid/backlog_item_id is non-empty"),
		field.String("backlog_item_id").Default("").Immutable().Comment("Loose FK to BacklogItem"),
		field.String("role").Default("primary").Validate(oneOf("role", sessionPRRoles)),
		field.Bool("pinned").Default(false),
		field.String("attach_source").Validate(oneOf("attach_source", sessionPRSources)),
		field.Time("attached_at").Default(time.Now).Immutable(),
	}
}

// Edges of the SessionPR.
func (SessionPR) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("pull_request_state", PullRequestState.Type).
			Field("pull_request_state_id").
			Unique().
			Required().
			Immutable(),
	}
}

// Indexes of the SessionPR.
func (SessionPR) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("pull_request_state_id", "session_uuid", "backlog_item_id").Unique(),
		index.Fields("session_uuid"),
		index.Fields("backlog_item_id"),
		index.Fields("pull_request_state_id"),
		// At most one primary link per session.
		index.Fields("session_uuid").
			StorageKey("sessionpr_one_primary_per_session").
			Unique().
			Annotations(entsql.IndexWhere("role = 'primary' AND session_uuid != ''")),
	}
}
