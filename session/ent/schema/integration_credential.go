package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// IntegrationCredential is a machine credential for an external integration client
// (e.g. gh-signal) calling the webhook-management API. It exists so that client never
// needs the owner's full-power passkey session: it is bound to one principal, one
// workspace, and one directory root, and only its SHA-256 is stored (the token is
// 256 bits of CSPRNG output, so an unsalted hash is not guessable offline).
type IntegrationCredential struct{ ent.Schema }

func (IntegrationCredential) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("principal_id").NotEmpty().Unique(),
		field.String("token_sha256").NotEmpty().Unique().Sensitive(),
		field.String("workspace_id").NotEmpty(),
		// Registrations made with this credential may only target this directory or
		// one beneath it, since a registration's workflow runs an agent there.
		field.String("allowed_dir_root").NotEmpty(),
		field.Time("created_at").Default(func() time.Time { return time.Now().UTC() }).Immutable(),
		field.Time("revoked_at").Optional().Nillable(),
	}
}

func (IntegrationCredential) Edges() []ent.Edge { return nil }
