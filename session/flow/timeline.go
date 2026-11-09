package flow

import (
	"context"
	"errors"
	"slices"
	"strings"
)

// maxTimelineEntries caps an item timeline; the newest entries are kept.
const maxTimelineEntries = 50

// ItemTimeline returns one item's transitions with thrash collapsed: each
// episode is one entry, so a 279-flip run is two entries, never 279. A zero
// window spec means all time. At most 50 entries are returned (the newest);
// TotalEntries and Truncated say how many there were.
func (a *Aggregator) ItemTimeline(ctx context.Context, q ItemTimelineQuery) (*ItemTimeline, error) {
	if q.ItemID == "" {
		return nil, errors.New("item timeline: item id is required")
	}
	spec := q.Window
	if spec == (WindowSpec{}) {
		spec.AllTime = true
	}
	w, err := spec.Window(a.clock)
	if err != nil {
		return nil, err
	}
	fq := FlowQuery{Window: spec, ItemIDs: []ItemID{q.ItemID}, Config: q.Config}
	return a.timelineCache.Do(ctx, fq, func(ctx context.Context) (*ItemTimeline, error) {
		return a.computeTimeline(ctx, fq, w, q.ItemID)
	})
}

func (a *Aggregator) computeTimeline(ctx context.Context, fq FlowQuery, w FlowWindow, id ItemID) (*ItemTimeline, error) {
	r := newRun(ctx, a, fq, w)
	ev, err := r.loadEvents()
	if err != nil {
		return nil, err
	}
	meta, err := r.itemMeta([]ItemID{id})
	if err != nil {
		return nil, err
	}
	entries, episodes := timelineEntries(ev.spans, id)
	out := &ItemTimeline{
		Meta: r.meta(map[SectionName]bool{}), ItemID: id,
		Title: meta[id].Title, Status: string(meta[id].Status),
		TotalEntries: len(entries), ThrashEpisodes: episodes,
	}
	if len(entries) > maxTimelineEntries {
		entries, out.Truncated = entries[len(entries)-maxTimelineEntries:], true
	}
	out.Entries = entries
	return out, nil
}

// timelineEntries turns one item's spans into time-ordered entries and counts
// the thrash episodes among them.
func timelineEntries(spans []Span, item ItemID) (entries []TimelineEntry, episodes int) {
	for _, sp := range spans {
		if sp.Item() != item {
			continue
		}
		switch v := sp.(type) {
		case RealSpan:
			entries = append(entries, TimelineEntry{
				Kind: "transition", From: string(v.From()), To: string(v.To()), At: v.At(), Last: v.At(), Count: 1, Note: v.Note(),
			})
		case ThrashSpan:
			episodes++
			entries = append(entries, TimelineEntry{
				Kind: "thrash_episode", From: string(v.Pair().A), To: string(v.Pair().B), At: v.First(), Last: v.Last(),
				Count: v.Count(), Cause: v.Cause().String(),
			})
		}
	}
	slices.SortStableFunc(entries, func(a, b TimelineEntry) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return entries, episodes
}
