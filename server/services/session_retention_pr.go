package services

import (
	"context"
	"fmt"
	"sort"

	"github.com/tstapler/stapler-squad/session"
)

// sweepPRSource is the three bulk reads the retention sweeper makes once per
// cycle; *session.Storage is the production implementation.
type sweepPRSource interface {
	ListSessionPRViewsByUUIDs(ctx context.Context, uuids []string) (map[string][]session.PRView, error)
	ListItemIDsBySessionUUIDs(ctx context.Context, uuids []string) (map[string][]string, error)
	ListItemGuardViewsByItemIDs(ctx context.Context, itemIDs []string) (map[string]session.ItemGuardView, error)
}

// sweepPRData is one cycle's PR evidence, keyed by session uuid and item id.
// Any read error aborts the whole cycle (loadSweepPRData), so a missing key is
// real absence, except in guards, where a missing item is treated as unknown.
type sweepPRData struct {
	views   map[string][]session.PRView
	itemIDs map[string][]string
	guards  map[string]session.ItemGuardView
}

// loadSweepPRData reads the evidence for the candidate sessions. An error means
// the sweep must delete nothing this cycle (fail closed).
func loadSweepPRData(ctx context.Context, src sweepPRSource, uuids []string) (*sweepPRData, error) {
	data := &sweepPRData{}
	var err error
	if data.views, err = src.ListSessionPRViewsByUUIDs(ctx, uuids); err != nil {
		return nil, fmt.Errorf("list session PR links: %w", err)
	}
	if data.itemIDs, err = src.ListItemIDsBySessionUUIDs(ctx, uuids); err != nil {
		return nil, fmt.Errorf("list items of sessions: %w", err)
	}
	ids := distinctItemIDs(data.itemIDs)
	if data.guards, err = src.ListItemGuardViewsByItemIDs(ctx, ids); err != nil {
		return nil, fmt.Errorf("list item PR guards: %w", err)
	}
	return data, nil
}

func distinctItemIDs(byUUID map[string][]string) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, list := range byUUID {
		for _, id := range list {
			if _, dup := seen[id]; !dup {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}

// prEligible is the ordered PR decision list for one session (first match wins).
// It reads only InstanceData and the bulk evidence, never a live Instance, and
// is not behind any feature flag: it can only withhold a delete.
func (p *sweepPRData) prEligible(d session.InstanceData) (bool, string) {
	if safe, reason := p.ownPRsEligible(d); !safe {
		return false, reason
	}
	return p.itemsEligible(d)
}

func (p *sweepPRData) ownPRsEligible(d session.InstanceData) (bool, string) {
	if reason := session.OwnPRsBlockReason(d, p.views[d.UUID]); reason != "" {
		return false, reason
	}
	return true, ""
}

// itemsEligible holds the session while any item it ever served still has an
// item-direct PR that is not terminal, an unrepresented legacy PR number, or no
// evidence at all (fail closed).
func (p *sweepPRData) itemsEligible(d session.InstanceData) (bool, string) {
	for _, id := range p.itemIDs[d.UUID] {
		g, ok := p.guards[id]
		if reason := session.ItemGuardBlockReason(id, g, ok); reason != "" {
			return false, reason
		}
	}
	return true, ""
}
