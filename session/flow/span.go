package flow

import (
	"errors"
	"fmt"
	"time"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Span is the output of Collapse: a sealed interface with exactly two
// implementations, RealSpan and ThrashSpan. Only the New* constructors build
// them, so a real span cannot carry a Cause or Count and a thrash span cannot
// omit them.
type Span interface {
	Item() ItemID
	isSpan()
}

// RealSpan is one genuine status transition.
type RealSpan struct {
	item     ItemID
	from, to domain.BacklogStatus
	at       time.Time
	note     string
	by       string
}

// NewRealSpan validates and builds a RealSpan; at is normalized to UTC.
func NewRealSpan(item ItemID, from, to domain.BacklogStatus, at time.Time) (RealSpan, error) {
	if item == "" {
		return RealSpan{}, errors.New("real span: empty item id")
	}
	if from == "" || to == "" {
		return RealSpan{}, errors.New("real span: empty from/to status")
	}
	if at.IsZero() {
		return RealSpan{}, errors.New("real span: zero timestamp")
	}
	return RealSpan{item: item, from: from, to: to, at: at.UTC()}, nil
}

// Item implements Span.
func (s RealSpan) Item() ItemID { return s.item }

// From is the status left.
func (s RealSpan) From() domain.BacklogStatus { return s.from }

// To is the status entered.
func (s RealSpan) To() domain.BacklogStatus { return s.to }

// At is the transition time (UTC).
func (s RealSpan) At() time.Time { return s.at }

// Note is the raw event note, kept so rework can partition review reopens.
func (s RealSpan) Note() string { return s.note }

// WithNote returns a copy carrying the raw event note.
func (s RealSpan) WithNote(note string) RealSpan {
	s.note = note
	return s
}

// TriggeredBy is who wrote the event ("system", "agent", "user"), kept so the
// PR-fix respawned count can be derived from system-written reopens.
func (s RealSpan) TriggeredBy() string { return s.by }

// WithTriggeredBy returns a copy carrying the event's actor.
func (s RealSpan) WithTriggeredBy(by string) RealSpan {
	s.by = by
	return s
}

func (RealSpan) isSpan() {}

// ThrashSpanParams carries the fields of a ThrashSpan; a struct rather than a
// positional pile of same-typed arguments.
type ThrashSpanParams struct {
	Item  ItemID
	Pair  FlipPair
	Count int
	// UnclippedCount is the episode size before clipping to the window; when
	// set it satisfies the ThrashMinFlips check that a clipped Count may not.
	UnclippedCount  int
	First, Last     time.Time
	NetStatus       domain.BacklogStatus
	Cause           Cause
	StartsBeforeWin bool
}

// ThrashSpan is a collapsed flip episode of at least ThrashMinFlips events.
type ThrashSpan struct {
	p ThrashSpanParams
}

// NewThrashSpan validates p against cfg: count must reach ThrashMinFlips and
// First must not be after Last.
func NewThrashSpan(cfg Config, p ThrashSpanParams) (ThrashSpan, error) {
	if p.Item == "" {
		return ThrashSpan{}, errors.New("thrash span: empty item id")
	}
	if max(p.Count, p.UnclippedCount) < cfg.ThrashMinFlips {
		return ThrashSpan{}, fmt.Errorf("thrash span: count %d below ThrashMinFlips %d", p.Count, cfg.ThrashMinFlips)
	}
	if p.First.After(p.Last) {
		return ThrashSpan{}, errors.New("thrash span: first is after last")
	}
	p.First, p.Last = p.First.UTC(), p.Last.UTC()
	return ThrashSpan{p: p}, nil
}

// Item implements Span.
func (s ThrashSpan) Item() ItemID { return s.p.Item }

// UnclippedCount is the episode size before window clipping (0 when not set).
func (s ThrashSpan) UnclippedCount() int { return s.p.UnclippedCount }

// Pair is the flip shape.
func (s ThrashSpan) Pair() FlipPair { return s.p.Pair }

// Count is the number of flip events in the episode.
func (s ThrashSpan) Count() int { return s.p.Count }

// First is the first flip time (UTC).
func (s ThrashSpan) First() time.Time { return s.p.First }

// Last is the last flip time (UTC).
func (s ThrashSpan) Last() time.Time { return s.p.Last }

// NetStatus is the status the item ends the episode in.
func (s ThrashSpan) NetStatus() domain.BacklogStatus { return s.p.NetStatus }

// Cause is the label derived from the flip shape.
func (s ThrashSpan) Cause() Cause { return s.p.Cause }

// StartsBeforeWindow reports an episode that began before the window start.
func (s ThrashSpan) StartsBeforeWindow() bool { return s.p.StartsBeforeWin }

func (ThrashSpan) isSpan() {}

// SpanImplementations returns one sample of every Span implementation, so a
// test can prove every implementation is handled. Extend it with any new one.
func SpanImplementations(at time.Time) []Span {
	return []Span{
		RealSpan{item: "sample", from: domain.BacklogStatusReady, to: domain.BacklogStatusQueued, at: at.UTC()},
		ThrashSpan{p: ThrashSpanParams{Item: "sample", Count: 1, First: at.UTC(), Last: at.UTC()}},
	}
}
