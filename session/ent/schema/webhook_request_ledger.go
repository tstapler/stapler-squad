package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// WebhookRequestLedger is the durable request-ID ledger for the webhook-management API.
// A row is written in the same transaction as the mutation it describes, so a request ID
// is either fully applied-and-recorded or not applied at all. result holds only the
// secret-free response body, which is what a same-ID replay returns.
type WebhookRequestLedger struct{ ent.Schema }

func (WebhookRequestLedger) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("principal_id").NotEmpty(),
		field.String("workspace_id").NotEmpty(),
		field.String("request_id").NotEmpty(),
		// fingerprint is a keyed digest of the canonical request, so reusing a request
		// ID for a different request is detectable without storing the secret.
		field.String("fingerprint").NotEmpty(),
		field.String("operation").NotEmpty(),
		field.Text("result").NotEmpty(),
		field.Time("created_at").Default(func() time.Time { return time.Now().UTC() }).Immutable(),
	}
}

func (WebhookRequestLedger) Edges() []ent.Edge { return nil }

func (WebhookRequestLedger) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("principal_id", "workspace_id", "request_id").Unique(),
	}
}
