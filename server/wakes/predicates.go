package wakes

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// EventMatch names the triggers an event can fire: the (type, key) index cell
// and, for kinds, the spec's kind string.
type EventMatch struct {
	Type domain.WakeTriggerType
	Key  string
	Kind string
}

// EventMatcher turns a bus event into the trigger cells it concerns.
type EventMatcher func(ev *events.Event) []EventMatch

// Predicate reads persisted state to decide whether a trigger's condition
// already holds. The EventBus is lossy and in-process, so reconcile re-asks the
// database instead of replaying events (ADR-006 rule 4). A type with no
// predicate never fires from reconcile.
type Predicate func(ctx context.Context, spec domain.WakeSpec) (bool, error)

// RegisterMatcher adds an event source. Call before Start.
func (d *Dispatcher) RegisterMatcher(m EventMatcher) { d.matchers = append(d.matchers, m) }

// RegisterPredicate sets the reconcile predicate for a trigger type. Call before Start.
func (d *Dispatcher) RegisterPredicate(t domain.WakeTriggerType, p Predicate) { d.predicates[t] = p }

// registerBacklogSource installs the backlog_event matcher and predicate.
func (d *Dispatcher) registerBacklogSource() {
	d.RegisterMatcher(func(ev *events.Event) []EventMatch {
		if ev == nil || ev.Type != events.EventBacklogItemChanged || ev.BacklogItemPayload == nil || ev.BacklogItemPayload.Item == nil {
			return nil
		}
		p := ev.BacklogItemPayload
		return []EventMatch{{Type: domain.WakeTriggerBacklogEvent, Key: p.Item.ID, Kind: string(p.Kind)}}
	})
	d.RegisterPredicate(domain.WakeTriggerBacklogEvent, func(ctx context.Context, spec domain.WakeSpec) (bool, error) {
		s, ok := spec.(domain.BacklogEventSpec)
		if !ok || d.cfg.Backlog == nil {
			return false, nil
		}
		st, err := d.cfg.Backlog.ItemState(ctx, s.ItemID)
		if err != nil {
			return false, fmt.Errorf("backlog predicate %s: %w", s.Kind, err)
		}
		return backlogKindHolds(s.Kind, st), nil
	})
}

const statusReachedPrefix = "status_reached:"

// backlogKindHolds is the persisted-state predicate per supported kind
// (Story 4.4's table); a kind with no predicate never holds.
func backlogKindHolds(kind string, st ItemState) bool {
	switch {
	case kind == string(events.BacklogChangeVerdictRecorded):
		return st.Exists && st.HasVerdict
	case kind == string(events.BacklogChangeItemArchived):
		return st.Exists && st.Archived
	case kind == string(events.BacklogChangeItemRemoved):
		return !st.Exists
	case strings.HasPrefix(kind, statusReachedPrefix):
		return st.Exists && st.Status == strings.TrimPrefix(kind, statusReachedPrefix)
	}
	return false
}

// conditionHolds evaluates the registered predicate for an event-type row.
func (d *Dispatcher) conditionHolds(ctx context.Context, row *session.WakeTriggerData) (bool, error) {
	p, ok := d.predicates[row.Type]
	if !ok {
		return false, nil
	}
	return p(ctx, row.Spec)
}

// specKind is the event-kind filter of a spec ("" matches any event for the key).
func specKind(spec domain.WakeSpec) string {
	switch s := spec.(type) {
	case domain.BacklogEventSpec:
		return s.Kind
	case domain.PREventSpec:
		return s.Kind
	case domain.CIEventSpec:
		return s.Kind
	}
	return ""
}

type indexKey struct {
	typ domain.WakeTriggerType
	key string
}

// indexKeyFor is the (type, key) cell an event-type spec lives in; time-based
// specs are not indexed (the timer owns them).
func indexKeyFor(spec domain.WakeSpec) (indexKey, bool) {
	switch s := spec.(type) {
	case domain.BacklogEventSpec:
		return indexKey{s.TriggerType(), s.ItemID}, true
	case domain.SessionStateSpec:
		return indexKey{s.TriggerType(), s.SessionUUID}, true
	case domain.GuidanceAnsweredSpec:
		return indexKey{s.TriggerType(), s.GuidanceRequestID}, true
	case domain.PREventSpec:
		return indexKey{s.TriggerType(), fmt.Sprintf("%s#%d", s.Repo, s.Number)}, true
	case domain.CIEventSpec:
		return indexKey{s.TriggerType(), fmt.Sprintf("%s#%d", s.Repo, s.Number)}, true
	}
	return indexKey{}, false
}

// triggerIndex maps (type, key) to the pending event-type rows waiting on it.
type triggerIndex struct {
	mu    sync.RWMutex
	cells map[indexKey]map[string]*session.WakeTriggerData
}

func newTriggerIndex() *triggerIndex {
	return &triggerIndex{cells: map[indexKey]map[string]*session.WakeTriggerData{}}
}

// replace rebuilds the index from the pending rows.
func (ix *triggerIndex) replace(rows []*session.WakeTriggerData) {
	cells := map[indexKey]map[string]*session.WakeTriggerData{}
	for _, r := range rows {
		k, ok := indexKeyFor(r.Spec)
		if !ok {
			continue
		}
		if cells[k] == nil {
			cells[k] = map[string]*session.WakeTriggerData{}
		}
		cells[k][r.ID] = r
	}
	ix.mu.Lock()
	ix.cells = cells
	ix.mu.Unlock()
}

func (ix *triggerIndex) lookup(t domain.WakeTriggerType, key string) []*session.WakeTriggerData {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	cell := ix.cells[indexKey{t, key}]
	out := make([]*session.WakeTriggerData, 0, len(cell))
	for _, r := range cell {
		out = append(out, r)
	}
	return out
}

func (ix *triggerIndex) remove(row *session.WakeTriggerData) {
	k, ok := indexKeyFor(row.Spec)
	if !ok {
		return
	}
	ix.mu.Lock()
	defer ix.mu.Unlock()
	delete(ix.cells[k], row.ID)
}
