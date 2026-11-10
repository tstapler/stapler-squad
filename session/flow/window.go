package flow

import (
	"errors"
	"fmt"
	"time"
)

// allTimeFloor is earlier than any recorded backlog event.
var allTimeFloor = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// FlowWindow is a half-open UTC interval [Start, End). Construct it only via
// the New* functions so Start < End always holds.
type FlowWindow struct {
	start, end time.Time
}

// NewFlowWindowBounds builds a window from explicit bounds.
func NewFlowWindowBounds(start, end time.Time) (FlowWindow, error) {
	start, end = start.UTC(), end.UTC()
	if !start.Before(end) {
		return FlowWindow{}, fmt.Errorf("flow window: start %s must be before end %s", start.Format(time.RFC3339), end.Format(time.RFC3339))
	}
	return FlowWindow{start: start, end: end}, nil
}

// NewFlowWindow builds the window of the last days days ending at clock.Now().
// days must be >= 1; use NewAllTimeWindow for an unbounded window.
func NewFlowWindow(clock Clock, days int) (FlowWindow, error) {
	if days < 1 {
		return FlowWindow{}, fmt.Errorf("flow window: days must be >= 1 (got %d); use an all-time window for unbounded", days)
	}
	end := clock.Now().UTC()
	return NewFlowWindowBounds(end.AddDate(0, 0, -days), end)
}

// NewAllTimeWindow builds a window from a floor earlier than any event to now.
func NewAllTimeWindow(clock Clock) (FlowWindow, error) {
	return NewFlowWindowBounds(allTimeFloor, clock.Now())
}

// Start is the inclusive lower bound (UTC).
func (w FlowWindow) Start() time.Time { return w.start }

// End is the exclusive upper bound (UTC).
func (w FlowWindow) End() time.Time { return w.end }

// Contains reports whether t falls in [Start, End).
func (w FlowWindow) Contains(t time.Time) bool {
	return !t.Before(w.start) && t.Before(w.end)
}

// WindowSpec is the clock-free description of a window. Cache keys are built
// from it, never from the clock-derived timestamps.
type WindowSpec struct {
	Days       int
	OffsetDays int
	AllTime    bool
	// Start and End, when set, pin absolute half-open bounds (explicit compare
	// windows) and take precedence over Days/OffsetDays.
	Start, End time.Time
}

func (s WindowSpec) hasBounds() bool { return !s.Start.IsZero() || !s.End.IsZero() }

// CacheKey is a stable key, distinct for AllTime and for every Days/OffsetDays pair.
func (s WindowSpec) CacheKey() string {
	if s.AllTime {
		return "all"
	}
	if s.hasBounds() {
		return fmt.Sprintf("b%d-%d", s.Start.UnixNano(), s.End.UnixNano())
	}
	return fmt.Sprintf("d%d+o%d", s.Days, s.OffsetDays)
}

// Window resolves the spec against clock. An OffsetDays window ends that many
// days before now (used for the previous-period comparison).
func (s WindowSpec) Window(clock Clock) (FlowWindow, error) {
	if s.AllTime {
		return NewAllTimeWindow(clock)
	}
	if s.hasBounds() {
		return NewFlowWindowBounds(s.Start, s.End)
	}
	if s.Days < 1 {
		return FlowWindow{}, errors.New("flow window spec: days must be >= 1 unless AllTime is set")
	}
	if s.OffsetDays < 0 {
		return FlowWindow{}, errors.New("flow window spec: offset_days must be >= 0")
	}
	end := clock.Now().UTC().AddDate(0, 0, -s.OffsetDays)
	return NewFlowWindowBounds(end.AddDate(0, 0, -s.Days), end)
}
