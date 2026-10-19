package services

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// guidance_ack_detector.go decides whether a keystroke answer reached the agent
// (ADR-005 acknowledgement signal, Task 3.4e). It acknowledges on the first of:
// (a) a PostToolUse receipt for AskUserQuestion from the session; (b) two
// consecutive fresh captures 3 s apart that show neither the row's dialog nor
// any dialog. At 60 s it takes one more capture: the row's own dialog still up
// means the digit was lost (delivery_failed), no dialog at all is
// acknowledgement, anything else leaves the row sent and Unconfirmed.

const (
	ackSampleEvery  = 3 * time.Second
	ackDecideAfter  = 60 * time.Second
	ackCleanSamples = 2
	askUserQuestion = "AskUserQuestion"
)

type ackStore interface {
	SetGuidanceDelivery(ctx context.Context, id uuid.UUID, route domain.DeliveryRoute, state domain.DeliveryState, addAttempts int) (bool, error)
	ListGuidanceByDelivery(ctx context.Context, route domain.DeliveryRoute, state domain.DeliveryState) ([]*session.GuidanceRequestData, error)
}

// ackNotes moves the question's notification cards to match the delivery state:
// an unconfirmed or still-showing answer is back in Needs you, an acknowledged
// one or one whose session ended leaves it. Optional.
type ackNotes interface {
	ResolveQuestionNotifications(questionID string)
	ReopenQuestionNotifications(questionID string)
}

// ackCapture takes a fresh pane capture. underLease asks for it to be taken
// holding the write lease, so no writer is mid-keystroke (the 60 s decision).
type ackCapture func(ctx context.Context, sessionUUID string, underLease bool) (string, error)

type ackEntry struct {
	SentDelivery
	sentAt      time.Time
	lastSample  time.Time
	clean       int
	decided     bool
	unconfirmed bool
}

// GuidanceAckDetector tracks sent answers until they are acknowledged, fail or
// their session ends. Tick does all the time-driven work, so tests drive it
// with a fake clock.
type GuidanceAckDetector struct {
	store   ackStore
	capture ackCapture
	ended   func(sessionUUID string) bool
	now     func() time.Time
	notes   ackNotes

	mu      sync.Mutex
	tracked map[string]*ackEntry
}

// NewGuidanceAckDetector builds the detector. ended reports a session that is
// gone or no longer live; now may be nil.
func NewGuidanceAckDetector(store ackStore, capture ackCapture, ended func(string) bool, now func() time.Time) *GuidanceAckDetector {
	if now == nil {
		now = time.Now
	}
	return &GuidanceAckDetector{store: store, capture: capture, ended: ended, now: now, tracked: map[string]*ackEntry{}}
}

// SetNotes installs the notification mover; call it before Run.
func (a *GuidanceAckDetector) SetNotes(n ackNotes) { a.notes = n }

// Sent starts tracking d. An indeterminate write is Unconfirmed at once.
func (a *GuidanceAckDetector) Sent(d SentDelivery) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.now()
	a.tracked[d.RowID] = &ackEntry{SentDelivery: d, sentAt: now, lastSample: now, unconfirmed: d.Indeterminate}
}

// Unconfirmed reports whether the row is sent but not confirmed (the chip).
func (a *GuidanceAckDetector) Unconfirmed(rowID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	e, ok := a.tracked[rowID]
	return ok && e.unconfirmed
}

// Receipt is signal (a): a PostToolUse hook ran for toolName in the session.
func (a *GuidanceAckDetector) Receipt(ctx context.Context, sessionUUID, toolName string) {
	if toolName != askUserQuestion {
		return
	}
	for _, e := range a.snapshot() {
		if e.SessionUUID == sessionUUID {
			a.finish(ctx, e, domain.DeliveryStateAcknowledged)
		}
	}
}

// Restore resumes tracking rows left in state sent by a previous process. Their
// send time is unknown, so the 60 s clock restarts now.
func (a *GuidanceAckDetector) Restore(ctx context.Context) {
	rows, err := a.store.ListGuidanceByDelivery(ctx, "", domain.DeliveryStateSent)
	if err != nil {
		log.Warn("[AckDetector] could not restore sent answers", "err", err)
		return
	}
	for _, row := range rows {
		d := SentDelivery{RowID: row.ID, SessionUUID: row.AskerSessionUUID, Route: row.DeliveryRoute}
		if dialog, ok := dialogFromRow(row); ok {
			d.Dialog = dialog
		} else if qs, ok := tabbedFromRow(row); ok {
			d.Matches = tabbedHeadersMatch(qs)
		}
		a.Sent(d)
	}
}

// Tick samples or decides every tracked answer that is due.
func (a *GuidanceAckDetector) Tick(ctx context.Context) {
	for _, e := range a.snapshot() {
		now := a.now()
		switch {
		case a.ended != nil && a.ended(e.SessionUUID):
			a.drop(e.RowID)
			a.moveCards(e.RowID, domain.DeliveryStateAcknowledged) // row 17: leaves Needs you and the badge
		case !e.decided && now.Sub(e.sentAt) >= ackDecideAfter:
			a.decide(ctx, e)
		case now.Sub(e.lastSample) >= ackSampleEvery:
			a.sample(ctx, e)
		}
	}
}

// Run ticks every sample interval until ctx is done.
func (a *GuidanceAckDetector) Run(ctx context.Context) {
	a.Restore(ctx)
	t := time.NewTicker(ackSampleEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.Tick(ctx)
		}
	}
}

// presence classifies capture against the row's dialog.
func (e *ackEntry) presence(capture string, err error) (own bool, p session.DialogPresence) {
	if err != nil {
		return false, session.DialogIndeterminate
	}
	if e.Matches != nil {
		return e.Matches(capture), session.PaneShowsDialog(capture)
	}
	return session.DialogMatch(capture, e.Dialog), session.PaneShowsDialog(capture)
}

func (a *GuidanceAckDetector) sample(ctx context.Context, e ackEntry) {
	capture, err := a.capture(ctx, e.SessionUUID, false)
	own, p := e.presence(capture, err)
	gone := err == nil && !own && p.Absent()
	a.mu.Lock()
	live, ok := a.tracked[e.RowID]
	if !ok {
		a.mu.Unlock()
		return
	}
	live.lastSample = a.now()
	if gone {
		live.clean++
	} else {
		live.clean = 0
	}
	ack := live.clean >= ackCleanSamples
	a.mu.Unlock()
	if ack {
		a.finish(ctx, e, domain.DeliveryStateAcknowledged)
	}
}

func (a *GuidanceAckDetector) decide(ctx context.Context, e ackEntry) {
	capture, err := a.capture(ctx, e.SessionUUID, true)
	own, p := e.presence(capture, err)
	switch {
	case err == nil && own:
		a.finish(ctx, e, domain.DeliveryStateFailed)
	case err == nil && p.Absent():
		a.finish(ctx, e, domain.DeliveryStateAcknowledged)
	default:
		a.mu.Lock()
		if live, ok := a.tracked[e.RowID]; ok {
			live.decided, live.unconfirmed, live.lastSample = true, true, a.now()
		}
		a.mu.Unlock()
		a.moveCards(e.RowID, domain.DeliveryStateFailed) // Unconfirmed: back in Needs you
	}
}

// finish records the end state of a tracked answer and stops tracking it.
func (a *GuidanceAckDetector) finish(ctx context.Context, e ackEntry, state domain.DeliveryState) {
	a.drop(e.RowID)
	id, err := uuid.Parse(e.RowID)
	if err != nil {
		return
	}
	applied, err := a.store.SetGuidanceDelivery(ctx, id, e.Route, state, 0)
	if err != nil || !applied {
		log.Warn("[AckDetector] delivery state not recorded", "row", e.RowID, "state", state, "applied", applied, "err", err)
		return
	}
	a.moveCards(e.RowID, state)
}

// moveCards reopens the card of an answer that needs the operator again
// (delivery_failed) and retires it when the answer is acknowledged or its
// session ended. Any other state leaves the card where it is.
func (a *GuidanceAckDetector) moveCards(rowID string, state domain.DeliveryState) {
	if a.notes == nil {
		return
	}
	switch state {
	case domain.DeliveryStateFailed:
		a.notes.ReopenQuestionNotifications(rowID)
	case domain.DeliveryStateAcknowledged:
		a.notes.ResolveQuestionNotifications(rowID)
	case domain.DeliveryStatePending, domain.DeliveryStateSent, domain.DeliveryStateSavedForLater, domain.DeliveryStateStale:
	}
}

func (a *GuidanceAckDetector) drop(rowID string) {
	a.mu.Lock()
	delete(a.tracked, rowID)
	a.mu.Unlock()
}

func (a *GuidanceAckDetector) snapshot() []ackEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ackEntry, 0, len(a.tracked))
	for _, e := range a.tracked {
		out = append(out, *e)
	}
	return out
}
