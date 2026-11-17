// Package guidancestore persists guidance requests (agent questions and peer help),
// their delivery state and the pending-request cache. session.EntRepository embeds
// *Repo so existing callers keep their method set.
package guidancestore

import (
	"context"

	"github.com/tstapler/stapler-squad/session/ent"
)

// Repo owns the guidance_requests table.
type Repo struct {
	client *ent.Client
	cache  pendingGuidanceCache
	// itemPendingChanged republishes the owning backlog item after its
	// has_pending_guidance_requests flag may have flipped; nil skips the republish.
	itemPendingChanged func(ctx context.Context, item *ent.BacklogItem)
}

// New returns a Repo over client, which must already hold the migrated schema.
// Reads of the pending list are uncached until WatchGuidanceWrites is called.
func New(client *ent.Client, itemPendingChanged func(ctx context.Context, item *ent.BacklogItem)) *Repo {
	r := &Repo{client: client, itemPendingChanged: itemPendingChanged}
	r.cache.disabled = true
	return r
}
