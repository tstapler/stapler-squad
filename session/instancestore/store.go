// Package instancestore is the sessions-table store for single-row access: it loads
// domain.InstanceData (with selective child loading), reads and writes the session-artifacts
// blob, and applies the single-column updaters. session.EntRepository embeds *Instances so
// existing callers keep their method set.
package instancestore

import (
	"context"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/ent"
)

// Ports are the collaborators Create and Delete call inside their transaction. They are
// injected because session/sessionpr reaches package github, which this leaf may not import.
type Ports struct {
	// UpsertSessionPRSet writes the PR links carried by a full save (sessionpr.UpsertSessionPRSetTx).
	UpsertSessionPRSet func(ctx context.Context, tx *ent.Tx, data domain.InstanceData) error
	// DeleteOwnerPRRows removes a session's PR links and tombstones (sessionpr.DeleteOwnerPRRows).
	DeleteOwnerPRRows func(ctx context.Context, tx *ent.Tx, owner domain.PROwner) error
}

// Instances reads session rows and their worktree, diff-stats, tag, project and Claude edges.
type Instances struct {
	client *ent.Client
	ports  Ports
}

// New returns Instances over client, which must already hold the migrated schema. Create and
// Delete need both ports; read-only and single-column callers may pass Ports{}.
func New(client *ent.Client, ports Ports) *Instances {
	return &Instances{client: client, ports: ports}
}
