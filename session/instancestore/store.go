// Package instancestore is the read side of the sessions table: it loads domain.InstanceData
// (with selective child loading) and the session-artifacts blob. session.EntRepository embeds
// *Instances so existing callers keep their method set.
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
