package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ErrorEvent holds the schema definition for the ErrorEvent entity.
type ErrorEvent struct{ ent.Schema }

// Fields of the ErrorEvent.
func (ErrorEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("fingerprint").Unique(), // SHA256 of type+first3frames
		field.String("error_type"),
		field.String("message"),
		field.Text("stack_trace"),
		field.String("rpc_procedure").Optional(),
		field.Int("occurrence_count").Default(1),
		field.Time("first_seen"),
		field.Time("last_seen"),
		field.Bool("acknowledged").Default(false),
		field.Time("acknowledged_at").Optional().Nillable(),
	}
}

// Indexes of the ErrorEvent.
func (ErrorEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("last_seen"),
		index.Fields("acknowledged"),
	}
}
