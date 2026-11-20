// Package instancestore is the sessions-table store for single-row access: it loads
// domain.InstanceData (with selective child loading), reads and writes the session-artifacts
// blob, and applies the single-column updaters. session.EntRepository embeds *Instances so
// existing callers keep their method set.
package instancestore

import "github.com/tstapler/stapler-squad/session/ent"

// Instances reads session rows and their worktree, diff-stats, tag, project and Claude edges.
type Instances struct {
	client *ent.Client
}

// New returns Instances over client, which must already hold the migrated schema.
func New(client *ent.Client) *Instances {
	return &Instances{client: client}
}
