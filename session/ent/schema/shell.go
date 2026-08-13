package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// Shell holds the schema definition for the Shell entity.
// A Shell is an independent sibling tmux session attached to a parent Session.
// Each shell is named {parentPrefix}_shell_{shellUUID} in tmux.
type Shell struct {
	ent.Schema
}

// Fields of the Shell.
func (Shell) Fields() []ent.Field {
	return []ent.Field{
		// id is the stable shell UUID, also used as the tmux window name fragment.
		field.String("id").
			Immutable().
			StorageKey("id"),
		// name is a human-readable label for the shell tab.
		field.String("name"),
		// command is the command run in the shell. Empty string means default shell ($SHELL or /bin/sh).
		field.String("command").
			Default(""),
		// working_dir is the working directory for the shell process. Optional; defaults to session path.
		field.String("working_dir").
			Optional(),
		// tmux_session_name stores the full computed sibling tmux session name:
		// "{parentPrefix}_shell_{shellUUID}". This is NOT the bare UUID.
		// Stored rather than derived so ReconcileShells can query tmux by the exact session name
		// without reconstructing the prefix (which may change if the parent is renamed).
		field.String("tmux_session_name"),
		// status is the shell lifecycle status: "running", "stopped", "error".
		field.String("status").
			Default("running"),
		// exit_code is the process exit code; nil until the shell exits.
		field.Int("exit_code").
			Optional().
			Nillable(),
		// order_index controls the display order of shell tabs.
		field.Int("order_index").
			Default(0),
		// started_at is the time the shell was spawned.
		field.Time("started_at").
			Default(time.Now).
			Immutable(),
		// stopped_at is the time the shell exited; nil while running.
		field.Time("stopped_at").
			Optional().
			Nillable(),
	}
}

// Edges of the Shell.
func (Shell) Edges() []ent.Edge {
	return []ent.Edge{
		// Back-reference to owning Session (many-to-one).
		edge.From("session", Session.Type).
			Ref("shells").
			Unique().
			Required(),
	}
}

// Indexes of the Shell.
func (Shell) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status"),
		index.Fields("order_index"),
	}
}
