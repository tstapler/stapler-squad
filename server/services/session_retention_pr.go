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
	rows := p.views[d.UUID]
	switch {
	case len(rows) == 0 && d.GitHubPRNumber == 0:
		return true, ""
	case len(rows) == 0:
		// A legacy number with no link is decided by the persisted flag alone,
		// never by the empty row set.
		if session.LegacyOnlyPRSafe(d) {
			return true, ""
		}
		return false, "legacy PR number with no link and no terminal flag"
	}
	if !session.PRsAllTerminal(rows, d.GitHubPRNumber) {
		return false, "a linked PR is open, unknown or unverified"
	}
	if d.GitHubPRNumber > 0 && !session.LegacyNumberRepresented(d, rows) && !session.LegacyOnlyPRSafe(d) {
		return false, "legacy PR number not carried by the links and not terminal"
	}
	return true, ""
}

// itemsEligible holds the session while any item it ever served still has an
// item-direct PR that is not terminal, an unrepresented legacy PR number, or no
// evidence at all (fail closed).
func (p *sweepPRData) itemsEligible(d session.InstanceData) (bool, string) {
	for _, id := range p.itemIDs[d.UUID] {
		g, ok := p.guards[id]
		switch {
		case !ok:
			return false, "no PR evidence for item " + id
		case g.LegacyUnrepresented:
			return false, "item " + id + " has a PR number no link represents"
		}
		for _, v := range g.Direct {
			if !v.Terminal() {
				return false, "item " + id + " has an item-direct PR that is not terminal"
			}
		}
	}
	return true, ""
}
