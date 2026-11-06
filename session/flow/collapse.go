package flow

import (
	"slices"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// flipShape binds a flip pair to the Cause its shape implies (ADR-002: the
// shape decides membership and cause, note text never does).
type flipShape struct {
	pair  FlipPair
	cause Cause
}

var flipShapes = []flipShape{
	{FlipPair{A: domain.BacklogStatusReady, B: domain.BacklogStatusInProgress}, CauseDequeueSpawnFailed},
	{FlipPair{A: domain.BacklogStatusQueued, B: domain.BacklogStatusInProgress}, CauseDequeueClaim},
	{FlipPair{A: domain.BacklogStatusPRPending, B: domain.BacklogStatusInProgress}, CausePRFixReopen},
}

// shapeIndex returns the index of the flip shape e belongs to, or -1.
func shapeIndex(e Event) int {
	for i, s := range flipShapes {
		if (e.From == s.pair.A && e.To == s.pair.B) || (e.From == s.pair.B && e.To == s.pair.A) {
			return i
		}
	}
	return -1
}

// episode is a maximal flip run (or merge of runs) as indexes into one item's events.
type episode struct {
	shape   int
	members []int
}

func (ep episode) firstIdx() int { return ep.members[0] }
func (ep episode) lastIdx() int  { return ep.members[len(ep.members)-1] }

// Collapse turns normalized events into spans per ADR-002: flip episodes of at
// least cfg.ThrashMinFlips events (every gap under cfg.ThrashGap) become one
// ThrashSpan, adjacent same-shape episodes closer than cfg.ThrashMergeGap
// merge, and every other in-window event becomes a RealSpan. Events must be
// normalized (sorted, no self-loops) and padded back from window.Start by
// cfg.CollapsePad() so a straddling episode is classified exactly; pad events
// are used for classification only and emit no spans. Thrash spans are
// clipped to the window (Count counts in-window events, UnclippedCount the
// whole episode). Output is ordered by time, then item.
func Collapse(events []Event, window FlowWindow, cfg Config) []Span {
	byItem := make(map[ItemID][]Event)
	for _, e := range events {
		if !e.At.Before(window.End()) {
			continue
		}
		byItem[e.ItemID] = append(byItem[e.ItemID], e)
	}
	out := make([]Span, 0, len(events))
	for _, evs := range byItem {
		slices.SortStableFunc(evs, compareEvents)
		out = append(out, collapseItem(evs, window, cfg)...)
	}
	slices.SortStableFunc(out, compareSpans)
	return out
}

// compareSpans orders spans by time, then item.
func compareSpans(a, b Span) int {
	if c := SpanTime(a).Compare(SpanTime(b)); c != 0 {
		return c
	}
	return strings.Compare(string(a.Item()), string(b.Item()))
}

func collapseItem(evs []Event, window FlowWindow, cfg Config) []Span {
	episodes := mergeEpisodes(evs, findEpisodes(evs, cfg), cfg)
	member := make(map[int]bool)
	for _, ep := range episodes {
		for _, i := range ep.members {
			member[i] = true
		}
	}
	var out []Span
	for i, e := range evs {
		if member[i] || !window.Contains(e.At) {
			continue
		}
		if rs, err := NewRealSpan(e.ItemID, e.From, e.To, e.At); err == nil {
			out = append(out, rs.WithNote(e.Note).WithTriggeredBy(e.TriggeredBy))
		}
	}
	for _, ep := range episodes {
		if ts, ok := clipEpisode(evs, ep, window, cfg); ok {
			out = append(out, ts)
		}
	}
	return out
}

// findEpisodes scans for runs of one flip shape with chained statuses and
// every gap under ThrashGap, keeping those of at least ThrashMinFlips events.
func findEpisodes(evs []Event, cfg Config) []episode {
	var eps []episode
	for i := 0; i < len(evs); {
		shape := shapeIndex(evs[i])
		if shape < 0 {
			i++
			continue
		}
		j := i + 1
		for j < len(evs) && continuesRun(evs[j-1], evs[j], shape, cfg) {
			j++
		}
		if j-i >= cfg.ThrashMinFlips {
			members := make([]int, 0, j-i)
			for k := i; k < j; k++ {
				members = append(members, k)
			}
			eps = append(eps, episode{shape: shape, members: members})
		}
		i = j
	}
	return eps
}

func continuesRun(prev, next Event, shape int, cfg Config) bool {
	return shapeIndex(next) == shape && next.From == prev.To && next.At.Sub(prev.At) < cfg.ThrashGap
}

// mergeEpisodes joins neighbouring same-shape episodes whose inter-episode gap
// is under ThrashMergeGap. Events between them that are not in a run of
// ThrashMinFlips stay real.
func mergeEpisodes(evs []Event, eps []episode, cfg Config) []episode {
	var out []episode
	for _, ep := range eps {
		if n := len(out); n > 0 && out[n-1].shape == ep.shape &&
			evs[ep.firstIdx()].At.Sub(evs[out[n-1].lastIdx()].At) < cfg.ThrashMergeGap {
			out[n-1].members = append(out[n-1].members, ep.members...)
			continue
		}
		out = append(out, ep)
	}
	return out
}

// clipEpisode builds the ThrashSpan for the in-window part of ep; ok is false
// when no member falls inside the window.
func clipEpisode(evs []Event, ep episode, window FlowWindow, cfg Config) (ThrashSpan, bool) {
	var inWin []Event
	for _, i := range ep.members {
		if window.Contains(evs[i].At) {
			inWin = append(inWin, evs[i])
		}
	}
	if len(inWin) == 0 {
		return ThrashSpan{}, false
	}
	last := evs[ep.lastIdx()]
	ts, err := NewThrashSpan(cfg, ThrashSpanParams{
		Item:            last.ItemID,
		Pair:            flipShapes[ep.shape].pair,
		Count:           len(inWin),
		UnclippedCount:  len(ep.members),
		First:           inWin[0].At,
		Last:            inWin[len(inWin)-1].At,
		NetStatus:       last.To,
		Cause:           flipShapes[ep.shape].cause,
		StartsBeforeWin: evs[ep.firstIdx()].At.Before(window.Start()),
	})
	return ts, err == nil
}

// SpanTime is the instant a span starts: the transition time of a RealSpan or
// the first flip of a ThrashSpan. An unknown implementation yields the zero time.
func SpanTime(s Span) time.Time {
	switch v := s.(type) {
	case RealSpan:
		return v.At()
	case ThrashSpan:
		return v.First()
	default:
		return time.Time{}
	}
}

// SpanTotals splits spans into real transitions and collapsed (thrash) events,
// the two terms that, with self-loops, reconcile to the raw in-window count.
func SpanTotals(spans []Span) (real, collapsed int) {
	for _, s := range spans {
		switch v := s.(type) {
		case RealSpan:
			real++
		case ThrashSpan:
			collapsed += v.Count()
		}
	}
	return real, collapsed
}

// groupByItem buckets spans by item and returns the item ids sorted, so callers
// iterate deterministically.
func groupByItem(spans []Span) (map[ItemID][]Span, []ItemID) {
	byItem := make(map[ItemID][]Span)
	for _, s := range spans {
		byItem[s.Item()] = append(byItem[s.Item()], s)
	}
	ids := make([]ItemID, 0, len(byItem))
	for id := range byItem {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return byItem, ids
}
