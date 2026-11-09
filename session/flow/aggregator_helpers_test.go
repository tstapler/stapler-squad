package flow

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

// fakeSrc implements all four source interfaces over in-memory slices and
// counts calls per method, so tests can assert which sources a request touched.
type fakeSrc struct {
	mu       sync.Mutex
	events   []Event
	verdicts []Verdict
	claims   map[ItemID]bool
	stuck    []StuckInterval
	items    []ItemMeta
	stages   []domain.BacklogStatus

	eventsErr, verdictsErr, stuckErr, itemsErr error
	calls                                      map[string]int
}

func newFakeSrc() *fakeSrc {
	return &fakeSrc{
		calls: map[string]int{},
		stages: []domain.BacklogStatus{
			domain.BacklogStatusIdea, domain.BacklogStatusRefining, domain.BacklogStatusReady, domain.BacklogStatusQueued,
			domain.BacklogStatusInProgress, domain.BacklogStatusReview, domain.BacklogStatusPRPending, domain.BacklogStatusDone,
		},
	}
}

func (f *fakeSrc) called(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *fakeSrc) record(name string) {
	f.mu.Lock()
	f.calls[name]++
	f.mu.Unlock()
}

func (f *fakeSrc) Events(ctx context.Context, rng EventRange) ([]Event, error) {
	f.record("Events")
	return f.query(ctx, rng)
}

// query filters without recording a call, so Count and Stream do not count as Events.
func (f *fakeSrc) query(_ context.Context, rng EventRange) ([]Event, error) {
	if f.eventsErr != nil {
		return nil, f.eventsErr
	}
	var out []Event
	for _, e := range f.events {
		if e.At.Before(rng.From) || (!rng.To.IsZero() && !e.At.Before(rng.To)) {
			continue
		}
		if len(rng.ItemIDs) > 0 && !slices.Contains(rng.ItemIDs, e.ItemID) {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeSrc) CountEvents(ctx context.Context, rng EventRange) (int, error) {
	f.record("CountEvents")
	es, err := f.query(ctx, rng)
	return len(es), err
}

func (f *fakeSrc) StreamEvents(ctx context.Context, rng EventRange, fn func([]Event) error) error {
	f.record("StreamEvents")
	es, err := f.query(ctx, rng)
	if err != nil {
		return err
	}
	byItem := map[ItemID][]Event{}
	var ids []ItemID
	for _, e := range es {
		if _, ok := byItem[e.ItemID]; !ok {
			ids = append(ids, e.ItemID)
		}
		byItem[e.ItemID] = append(byItem[e.ItemID], e)
	}
	slices.Sort(ids)
	size := max(rng.ChunkItems, 1)
	for len(ids) > 0 {
		n := min(size, len(ids))
		var batch []Event
		for _, id := range ids[:n] {
			batch = append(batch, byItem[id]...)
		}
		ids = ids[n:]
		if err := fn(batch); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeSrc) LastEventsBefore(_ context.Context, start time.Time, ids []ItemID) ([]Event, error) {
	f.record("LastEventsBefore")
	latest := map[ItemID]Event{}
	for _, e := range f.events {
		if !slices.Contains(ids, e.ItemID) || !e.At.Before(start) {
			continue
		}
		if cur, ok := latest[e.ItemID]; !ok || cur.At.Before(e.At) {
			latest[e.ItemID] = e
		}
	}
	out := make([]Event, 0, len(latest))
	for _, e := range latest {
		out = append(out, e)
	}
	return out, nil
}

func (f *fakeSrc) Verdicts(_ context.Context, w FlowWindow) ([]Verdict, error) {
	f.record("Verdicts")
	if f.verdictsErr != nil {
		return nil, f.verdictsErr
	}
	var out []Verdict
	for _, v := range f.verdicts {
		if w.Contains(v.CreatedAt) {
			out = append(out, v)
		}
	}
	return out, nil
}

func (f *fakeSrc) AgentDoneClaims(context.Context, FlowWindow) (map[ItemID]bool, error) {
	f.record("AgentDoneClaims")
	return f.claims, f.verdictsErr
}

func (f *fakeSrc) StuckIntervals(context.Context) ([]StuckInterval, error) {
	f.record("StuckIntervals")
	return f.stuck, f.stuckErr
}

func (f *fakeSrc) Items(_ context.Context, ids []ItemID) ([]ItemMeta, error) {
	f.record("Items")
	if f.itemsErr != nil {
		return nil, f.itemsErr
	}
	var out []ItemMeta
	for _, it := range f.items {
		if slices.Contains(ids, it.ID) {
			out = append(out, it)
		}
	}
	return out, nil
}

func (f *fakeSrc) OpenItemIDs(context.Context) ([]ItemID, error) {
	f.record("OpenItemIDs")
	var out []ItemID
	for _, it := range f.items {
		if it.Status != domain.BacklogStatusDone && it.Status != domain.BacklogStatusArchived {
			out = append(out, it.ID)
		}
	}
	return out, f.itemsErr
}

func (f *fakeSrc) Stages(context.Context) ([]domain.BacklogStatus, error) {
	f.record("Stages")
	return f.stages, f.itemsErr
}

// fixtureClock is 30 days after the fixtures' earliest verdict, so every
// fixture event, verdict and stuck interval falls inside a 30-day window.
var fixtureClock = FixedClock{T: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)}

// fixtureSrc loads the golden fixtures into a fakeSrc.
func fixtureSrc(t *testing.T) *fakeSrc {
	t.Helper()
	f := newFakeSrc()
	ev := loadFixture[fixtureEvents](t, "events_thrash.json")
	raw := make([]RawEvent, 0, len(ev.Events))
	for _, e := range ev.Events {
		raw = append(raw, RawEvent{
			ItemID: ItemID(e.ItemID), From: domain.BacklogStatus(e.From), To: domain.BacklogStatus(e.To),
			At: e.At, Note: e.Note, TriggeredBy: e.TriggeredBy, Seq: int64(e.Seq),
		})
	}
	f.events, _ = Normalize(raw)
	for _, it := range ev.Items {
		f.items = append(f.items, ItemMeta{ID: ItemID(it.ID), Title: it.Title, Status: domain.BacklogStatusDone})
	}
	for _, v := range loadFixture[fixtureVerdicts](t, "verdicts.json").Verdicts {
		f.verdicts = append(f.verdicts, fixtureToVerdict(t, v))
		f.items = append(f.items, ItemMeta{ID: ItemID(v.ItemID), Title: "verdict item " + v.ItemID, Status: domain.BacklogStatusReview})
	}
	for _, iv := range loadFixture[fixtureStuck](t, "stuck.json").Intervals {
		f.stuck = append(f.stuck, fixtureToStuck(t, iv))
	}
	return f
}

func fixtureToVerdict(t *testing.T, v fixtureVerdict) Verdict {
	t.Helper()
	at, err := time.Parse(fixtureTimeLayout, v.CreatedAt)
	require.NoError(t, err)
	out := Verdict{
		ItemID: ItemID(v.ItemID), SessionID: SessionID(v.SessionID), CreatedAt: at.UTC(),
		Outcome: Outcome(v.OverallOutcome), Summary: v.Summary, DiffHash: v.DiffHash, OverrideBy: v.OverrideBy,
	}
	for _, c := range v.PerCriterion {
		out.PerCriterion = append(out.PerCriterion, CriterionResult{Index: c.CriterionIndex, Outcome: Outcome(c.Outcome)})
	}
	return out
}

func fixtureToStuck(t *testing.T, iv fixtureStuckInterval) StuckInterval {
	t.Helper()
	first, err := time.Parse(fixtureTimeLayout, iv.FirstDetectedAt)
	require.NoError(t, err)
	out := StuckInterval{ItemID: ItemID(iv.ItemID), Reason: iv.Reason, FirstDetectedAt: first.UTC()}
	if iv.ResolvedAt != nil {
		r, err := time.Parse(fixtureTimeLayout, *iv.ResolvedAt)
		require.NoError(t, err)
		r = r.UTC()
		out.ResolvedAt = &r
	}
	return out
}

// thrashItems appends n items that each flip ready/in_progress six times one
// minute apart, which Collapse turns into exactly one thrash episode each.
func (f *fakeSrc) addThrashItems(n int, start time.Time) {
	for i := 0; i < n; i++ {
		id := ItemID(fmt.Sprintf("flip-%03d", i))
		f.items = append(f.items, ItemMeta{ID: id, Title: string(id), Status: domain.BacklogStatusReady})
		at := start.Add(time.Duration(i) * time.Hour)
		for k := 0; k < 6; k++ {
			from, to := domain.BacklogStatusReady, domain.BacklogStatusInProgress
			if k%2 == 1 {
				from, to = to, from
			}
			f.events = append(f.events, Event{ItemID: id, From: from, To: to, At: at.Add(time.Duration(k) * time.Minute), Seq: int64(len(f.events) + 1)})
		}
	}
}
