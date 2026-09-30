package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// WebhookRegistration records that a webhook-trigger Workflow is owned by an
// integration client, keyed by (principal, workspace, instance). It is the ownership
// proof reconcile relies on: a Workflow with no registration row is user-managed and is
// never adopted, updated or deleted by the management API.
type WebhookRegistration struct{ ent.Schema }

func (WebhookRegistration) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("principal_id").NotEmpty(),
		field.String("workspace_id").NotEmpty(),
		// instance_id is the integration's stable instance identity (chosen by the client).
		field.String("instance_id").NotEmpty(),
		field.UUID("workflow_id", uuid.UUID{}),
		// version is the CAS token: 1 on create, +1 on every reconcile that changes state.
		field.Int64("version").Default(1),
		// The compatibility tuple the registration was created under, persisted verbatim
		// and never rewritten by ordinary reconcile (see the plan's provenance rules).
		field.Text("compat_tuple").NotEmpty(),
		field.String("compat_tuple_sha256").NotEmpty(),
		// secret_digest is a keyed digest of the plaintext webhook secret. It lets reconcile
		// detect "same secret" vs "rotated" without decrypting, and is never returned.
		field.String("secret_digest").NotEmpty().Sensitive(),
		field.Time("created_at").Default(func() time.Time { return time.Now().UTC() }).Immutable(),
		field.Time("updated_at").
			Default(func() time.Time { return time.Now().UTC() }).
			UpdateDefault(func() time.Time { return time.Now().UTC() }),
	}
}

func (WebhookRegistration) Edges() []ent.Edge { return nil }

func (WebhookRegistration) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("principal_id", "workspace_id", "instance_id").Unique(),
		index.Fields("workflow_id").Unique(),
	}
}
