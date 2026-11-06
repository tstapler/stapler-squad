package flow

import (
	"slices"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// RawEvent is a status-event row as stored: the timestamp is still text with
// a mixed UTC offset (+00:00 or -07:00). Normalize turns it into an Event.
type RawEvent struct {
	ItemID      ItemID
	From        domain.BacklogStatus
	To          domain.BacklogStatus
	At          string
	Note        string
	TriggeredBy string
	Seq         int64
}

// DataQuality counts the oddities seen while normalizing and computing, so a
// reader can see what was dropped or clamped instead of it vanishing silently.
type DataQuality struct {
	SelfLoops    int
	Unparseable  int
	NegativeGaps int
	// UnknownSpans counts Span implementations a computation did not recognise.
	UnknownSpans int
}

// Add returns the field-wise sum.
func (d DataQuality) Add(o DataQuality) DataQuality {
	return DataQuality{
		SelfLoops:    d.SelfLoops + o.SelfLoops,
		Unparseable:  d.Unparseable + o.Unparseable,
		NegativeGaps: d.NegativeGaps + o.NegativeGaps,
		UnknownSpans: d.UnknownSpans + o.UnknownSpans,
	}
}

// storedTimeLayouts are the accepted timestamp spellings, most specific first.
var storedTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999Z07:00",
	time.RFC3339Nano,
}

// ParseEventTime parses a stored timestamp tolerantly and returns it in UTC.
func ParseEventTime(s string) (time.Time, bool) {
	for _, layout := range storedTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// Normalize parses timestamps to UTC, drops self-loops (From == To) and rows
// with unparseable timestamps (both counted in DataQuality), and orders the
// rest by (At, Seq). Ordering is by instant, never by the stored text.
func Normalize(raw []RawEvent) ([]Event, DataQuality) {
	var dq DataQuality
	events := make([]Event, 0, len(raw))
	for _, r := range raw {
		at, ok := ParseEventTime(r.At)
		if !ok {
			dq.Unparseable++
			continue
		}
		if r.From == r.To {
			dq.SelfLoops++
			continue
		}
		events = append(events, Event{
			ItemID: r.ItemID, From: r.From, To: r.To, At: at,
			Note: r.Note, TriggeredBy: r.TriggeredBy, Seq: r.Seq,
		})
	}
	slices.SortStableFunc(events, compareEvents)
	return events, dq
}

func compareEvents(a, b Event) int {
	if c := a.At.Compare(b.At); c != 0 {
		return c
	}
	switch {
	case a.Seq < b.Seq:
		return -1
	case a.Seq > b.Seq:
		return 1
	}
	return 0
}

// prefilterSlack covers any stored UTC offset (UTC-12..UTC+14) plus an hour.
const prefilterSlack = 15 * time.Hour

// PrefilterFloor is the earliest stored wall-clock value the SQL prefilter may
// use: the padded window start minus prefilterSlack. Stored text carries local
// wall-clock time, so this floor never excludes an event the collapse needs;
// exact window filtering happens in Go on parsed instants.
func PrefilterFloor(windowStart time.Time, cfg Config) time.Time {
	return windowStart.UTC().Add(-cfg.CollapsePad() - prefilterSlack)
}
