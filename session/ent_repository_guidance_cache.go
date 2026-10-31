package session

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/session/ent"
)

// pendingGuidanceTTL bounds staleness against writers that bypass this
// repository's ent client (another process on the same database file). Writes
// through the client invalidate immediately via a mutation hook.
const pendingGuidanceTTL = 5 * time.Second

// pendingGuidanceCache memoizes ListAllPendingGuidanceRequests, which five
// independent pollers (reaper, dialog-gone sampler, hook mirror, notification
// lookups, nav-badge RPC) each ran as a full scan of open requests.
//
// gen counts GuidanceRequest mutations. A reader records gen before querying
// and stores its result only if gen is unchanged afterwards, so a write that
// lands mid-read can never be masked by that read's older rows.
type pendingGuidanceCache struct {
	gen atomic.Uint64
	now func() time.Time // injected by tests; nil means time.Now

	mu    sync.Mutex
	rows  []*GuidanceRequestData
	gotAt time.Time
	atGen uint64
	full  bool
}

func (c *pendingGuidanceCache) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *pendingGuidanceCache) invalidate() { c.gen.Add(1) }

func (c *pendingGuidanceCache) get() ([]*GuidanceRequestData, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.full || c.atGen != c.gen.Load() || c.clock().Sub(c.gotAt) >= pendingGuidanceTTL {
		return nil, false
	}
	return copyGuidanceRows(c.rows), true
}

func (c *pendingGuidanceCache) put(readGen uint64, rows []*GuidanceRequestData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if readGen != c.gen.Load() {
		return
	}
	c.rows, c.gotAt, c.atGen, c.full = copyGuidanceRows(rows), c.clock(), readGen, true
}

// copyGuidanceRows gives each caller its own structs so one caller editing a
// row cannot change what the next caller sees.
func copyGuidanceRows(rows []*GuidanceRequestData) []*GuidanceRequestData {
	out := make([]*GuidanceRequestData, len(rows))
	for i, r := range rows {
		cp := *r
		out[i] = &cp
	}
	return out
}

// watchGuidanceWrites invalidates the pending-request cache after every
// GuidanceRequest mutation made through client, committed or not (a spurious
// invalidation only costs one re-read).
func (r *EntRepository) watchGuidanceWrites(client *ent.Client) {
	client.GuidanceRequest.Use(func(next ent.Mutator) ent.Mutator {
		return ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
			v, err := next.Mutate(ctx, m)
			r.pendingGuidance.invalidate()
			return v, err
		})
	})
}
