package flow

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

const (
	defaultListLimit = 10
	maxListLimit     = 50
	compactListLimit = 3
)

// listLimitFor resolves a requested top-N size: 0 means the default, Compact
// forces 3, and a request above the maximum is clamped (truncated=true).
func listLimitFor(requested int, compact bool) (limit int, truncated bool) {
	switch {
	case compact:
		return compactListLimit, false
	case requested <= 0:
		return defaultListLimit, false
	case requested > maxListLimit:
		return maxListLimit, true
	}
	return requested, false
}

// pageOf returns the first limit entries of all with the totals every list carries.
func pageOf[T any](all []T, limit int, truncated bool) ListPage[T] {
	return pageRange(all, 0, limit, truncated)
}

// pageRange returns all[offset:offset+limit] with the totals.
func pageRange[T any](all []T, offset, limit int, truncated bool) ListPage[T] {
	offset = min(max(offset, 0), len(all))
	end := min(offset+limit, len(all))
	items := slices.Clone(all[offset:end])
	return ListPage[T]{
		Items:         items,
		TotalCount:    len(all),
		ReturnedCount: len(items),
		HasMore:       end < len(all),
		Truncated:     truncated,
	}
}

// loaded memoizes one source read so several sections share it, including its error.
type loaded[T any] struct {
	done bool
	v    T
	err  error
}

func (l *loaded[T]) get(load func() (T, error)) (T, error) {
	if !l.done {
		l.v, l.err = load()
		l.done = true
	}
	return l.v, l.err
}

// eventData is the collapsed view of the window's events.
type eventData struct {
	spans       []Span
	ids         []ItemID
	rawInWindow int
	quality     DataQuality
}

// run is the state of one computation over one window: configuration, lazily
// loaded source data and the derived pure results shared between sections.
type run struct {
	ctx   context.Context
	a     *Aggregator
	q     FlowQuery
	w     FlowWindow
	cfg   Config
	asOf  time.Time
	rows  RowCounts
	fails []SectionFailure

	streamed bool // events were read through StreamEvents

	events loaded[*eventData]
	seeds  loaded[[]Event]
	stages loaded[[]domain.BacklogStatus]
	dwell  loaded[DwellResult]
	stuck  loaded[[]StuckInterval]
	vdata  loaded[*verdictData]

	captured loaded[time.Time]
	markers  loaded[[]Marker]
	mcap     loaded[MarkerCapture]

	rework   *ReworkResult
	lead     *LeadTimeResult
	prfix    *PRFixDwellResult
	join     *TriggerJoin
	estimate map[string]bool
}

func newRun(ctx context.Context, a *Aggregator, q FlowQuery, w FlowWindow) *run {
	return &run{ctx: ctx, a: a, q: q, w: w, cfg: effectiveConfig(q.Config), asOf: a.clock.Now(), estimate: map[string]bool{}}
}

func (r *run) failure(name SectionName, err error) {
	r.fails = append(r.fails, SectionFailure{Section: name, Err: err})
}

// loadEvents reads the padded window, drops self-loops and collapses. A window
// above Limits.MaxWindowRows is streamed in item batches and collapsed per
// batch (collapse is per-item, so the result is identical); above
// Limits.HardRowCeiling it is refused with ErrWindowTooLarge.
func (r *run) loadEvents() (*eventData, error) {
	return r.events.get(func() (*eventData, error) {
		rng := EventRange{From: r.w.Start().Add(-r.cfg.CollapsePad()), To: r.w.End(), ItemIDs: r.q.ItemIDs}
		lim := r.a.limits
		n, err := r.a.src.Events.CountEvents(r.ctx, rng)
		if err != nil {
			return nil, fmt.Errorf("count events: %w", err)
		}
		if n > lim.HardRowCeiling {
			return nil, &WindowTooLargeError{Rows: n, Limit: lim.HardRowCeiling}
		}
		acc := &eventAccumulator{r: r, seen: map[ItemID]struct{}{}, d: &eventData{}}
		if n <= lim.MaxWindowRows {
			raw, err := r.a.src.Events.Events(r.ctx, rng)
			if err != nil {
				return nil, fmt.Errorf("read events: %w", err)
			}
			acc.add(raw)
		} else {
			r.streamed = true
			rng.ChunkItems, rng.BatchTimeout = lim.ChunkItems, lim.BatchTimeout
			err := r.a.src.Events.StreamEvents(r.ctx, rng, func(batch []Event) error {
				acc.add(batch)
				return r.ctx.Err()
			})
			if err != nil {
				return nil, fmt.Errorf("stream events: %w", err)
			}
		}
		return acc.finish(), nil
	})
}

// eventAccumulator collapses event batches into spans, keeping only collapsed
// spans (never the raw events) between batches.
type eventAccumulator struct {
	r     *run
	d     *eventData
	seen  map[ItemID]struct{}
	spans []Span
}

func (a *eventAccumulator) add(raw []Event) {
	a.r.rows.Events += len(raw)
	kept := make([]Event, 0, len(raw))
	for _, e := range raw {
		if e.From == e.To {
			a.d.quality.SelfLoops++
			continue
		}
		kept = append(kept, e)
		a.seen[e.ItemID] = struct{}{}
		if a.r.w.Contains(e.At) {
			a.d.rawInWindow++
		}
	}
	slices.SortStableFunc(kept, compareEvents)
	a.spans = append(a.spans, Collapse(kept, a.r.w, a.r.cfg)...)
}

// finish restores Collapse's global (time, item) order across batches.
func (a *eventAccumulator) finish() *eventData {
	slices.SortStableFunc(a.spans, compareSpans)
	a.d.spans = a.spans
	a.d.ids = sortedItems(a.seen)
	return a.d
}

func (r *run) loadStages() ([]domain.BacklogStatus, error) {
	return r.stages.get(func() ([]domain.BacklogStatus, error) {
		st, err := r.a.src.Items.Stages(r.ctx)
		if err != nil {
			return nil, fmt.Errorf("read stages: %w", err)
		}
		return st, nil
	})
}

// loadDwell seeds each item's status at window start and computes dwell.
func (r *run) loadDwell() (DwellResult, error) {
	return r.dwell.get(func() (DwellResult, error) {
		ev, err := r.loadEvents()
		if err != nil {
			return DwellResult{}, err
		}
		seeds, err := r.seeds.get(func() ([]Event, error) {
			if len(ev.ids) == 0 {
				return nil, nil
			}
			s, err := r.a.src.Events.LastEventsBefore(r.ctx, r.w.Start(), ev.ids)
			if err != nil {
				return nil, fmt.Errorf("read window-start seeds: %w", err)
			}
			return s, nil
		})
		if err != nil {
			return DwellResult{}, err
		}
		return Dwell(DwellInput{Spans: ev.spans, Seeds: seeds, Window: r.w, AsOf: r.asOf}), nil
	})
}

// reworkResult, leadResult and prfixResult derive from the spans once.
func (r *run) reworkResult(ev *eventData) *ReworkResult {
	if r.rework == nil {
		res := ReworkWith(ev.spans, r.cfg, r.triggerJoin())
		r.rework = &res
	}
	return r.rework
}

func (r *run) leadResult(ev *eventData) *LeadTimeResult {
	if r.lead == nil {
		res := LeadTime(ev.spans, r.cfg)
		r.lead = &res
	}
	return r.lead
}

func (r *run) prfixResult(ev *eventData) *PRFixDwellResult {
	if r.prfix == nil {
		res := PRFixDwellWith(ev.spans, r.w, r.asOf, r.cfg, r.triggerJoin())
		r.prfix = &res
	}
	return r.prfix
}

// itemMeta loads item metadata for ids keyed by id; a read failure returns the error.
func (r *run) itemMeta(ids []ItemID) (map[ItemID]ItemMeta, error) {
	out := make(map[ItemID]ItemMeta, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	items, err := r.a.src.Items.Items(r.ctx, ids)
	if err != nil {
		return out, fmt.Errorf("read items: %w", err)
	}
	r.rows.Items += len(items)
	for _, it := range items {
		out[it.ID] = it
	}
	return out, nil
}

func (r *run) noteEstimated(labels ...string) {
	for _, l := range labels {
		r.estimate[l] = true
	}
}

// sectionRunner builds one section; on error the section is reset to its zero
// value with Status=Failed so a half-built section is never returned.
func runSection[T any](r *run, sel map[SectionName]bool, name SectionName, sec *T, meta func(*T) *SectionMeta, build func(*T) error) {
	if !sel[name] {
		meta(sec).Status = SectionSkipped
		return
	}
	if err := build(sec); err != nil {
		var zero T
		*sec = zero
		*meta(sec) = SectionMeta{Status: SectionFailed, Error: err.Error()}
		r.failure(name, err)
		return
	}
	r.noteEstimated(meta(sec).Estimated...)
}

// buildSections computes every selected section; the rest are Skipped.
func (r *run) buildSections(sel map[SectionName]bool) *FlowSummary {
	s := &FlowSummary{}
	runSection(r, sel, SectionTiles, &s.Tiles, func(x *TilesSection) *SectionMeta { return &x.Meta }, r.tiles)
	runSection(r, sel, SectionFunnel, &s.Funnel, func(x *FunnelSection) *SectionMeta { return &x.Meta }, r.funnel)
	runSection(r, sel, SectionCauses, &s.Causes, func(x *CausesSection) *SectionMeta { return &x.Meta }, r.causes)
	runSection(r, sel, SectionReview, &s.Review, func(x *ReviewSection) *SectionMeta { return &x.Meta }, r.review)
	runSection(r, sel, SectionPR, &s.PR, func(x *PRSection) *SectionMeta { return &x.Meta }, r.pr)
	runSection(r, sel, SectionStuck, &s.Stuck, func(x *StuckSection) *SectionMeta { return &x.Meta }, r.stuckSection)
	runSection(r, sel, SectionAnomalies, &s.Anomalies, func(x *AnomaliesSection) *SectionMeta { return &x.Meta }, r.anomalies)
	runSection(r, sel, SectionTrends, &s.Trends, func(x *TrendsSection) *SectionMeta { return &x.Meta }, r.trends)
	runSection(r, sel, SectionNotes, &s.Notes, func(x *NotesSection) *SectionMeta { return &x.Meta }, r.notes)
	return s
}

// meta assembles the response-level FlowMeta from what the sections reported.
func (r *run) meta(sel map[SectionName]bool) FlowMeta {
	if sel[SectionReview] {
		r.noteEstimated(EstimatedDeadlockHeuristic)
	}
	est := make([]string, 0, len(r.estimate))
	for l := range r.estimate {
		est = append(est, l)
	}
	slices.Sort(est)
	m := FlowMeta{
		AsOf: r.asOf, WindowStart: r.w.Start(), WindowEnd: r.w.End(),
		RowCounts: r.rows, Estimated: est, SlowPath: r.q.Window.AllTime || r.streamed,
	}
	m.CaptureSince = r.captureSince()
	return m
}

// captureSince is the earliest capture time (zero when none or unreadable),
// read once per run.
func (r *run) captureSince() time.Time {
	t, _ := r.captured.get(func() (time.Time, error) { // load never errors; failures degrade to zero
		if r.a.captureSince == nil {
			return time.Time{}, nil
		}
		t, err := r.a.captureSince(r.ctx)
		if err != nil {
			return time.Time{}, nil
		}
		return t.UTC(), nil
	})
	return t
}

// loadMarkers reads the marker rows the window needs: trigger markers up to
// TriggerJoinWindow before the window start (to join its first events) and
// outcome markers inside it. An absent MarkerSource yields none.
func (r *run) loadMarkers() ([]Marker, error) {
	return r.markers.get(func() ([]Marker, error) {
		if r.a.src.Markers == nil {
			return nil, nil
		}
		all, err := r.a.src.Markers.Markers(r.ctx, r.w.Start().Add(-TriggerJoinWindow), r.w.End())
		if err != nil {
			return nil, wrapRead("flow markers", err)
		}
		if len(r.q.ItemIDs) == 0 {
			return all, nil
		}
		return slices.DeleteFunc(slices.Clone(all), func(m Marker) bool { return !slices.Contains(r.q.ItemIDs, m.Item) }), nil
	})
}

// triggerJoin builds the trigger join once; nil when no capture exists. A
// marker read failure leaves the join empty (events stay unknown) and is
// reported by the sections through markerFailure.
func (r *run) triggerJoin() *TriggerJoin {
	if r.join == nil && !r.captureSince().IsZero() {
		markers, _ := r.loadMarkers() // error surfaces via markerFailure
		r.join = NewTriggerJoin(markers, r.captureSince())
	}
	return r.join
}

// markerFailure marks the section Partial when markers could not be read, so
// the unavailable split is visible rather than silently unknown.
func (r *run) markerFailure(name SectionName, meta *SectionMeta) {
	if r.markers.done && r.markers.err != nil {
		meta.Status, meta.Error = SectionPartial, r.markers.err.Error()
		r.failure(name, r.markers.err)
	}
}
