package services

import (
	"context"
	"sort"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// guidance_dialog_gone_sampler.go owns the "dialog gone on two samples"
// fallback of Story 3.3 (Task 3.3f): the one deliberate bounded poll loop, for
// sessions that have an open hook-source row. It never changes an answerable
// row's state by time alone: it stops watching a row after samplerStopAfter and
// leaves the row open; a row leaves only on a confirmed gone or replaced dialog.

const (
	samplerFastEvery  = 3 * time.Second  // per row, for the first minute
	samplerSlowEvery  = 30 * time.Second // per row, afterwards
	samplerFastWindow = time.Minute
	samplerStopAfter  = 10 * time.Minute
	samplerMaxPerPass = 20
	// samplerConfirm is the number of consecutive matching samples that confirm a verdict.
	samplerConfirm = 2
)

// dialogVerdict is what one pane capture says about one open row.
type dialogVerdict uint8

const (
	verdictIndeterminate dialogVerdict = iota // capture error, unknown layout or unverifiable
	verdictMatch                              // the row's own dialog is on screen
	verdictGone                               // the pane positively shows no dialog
	verdictReplaced                           // another dialog is on screen
)

// judgeRow classifies capture against row. A row with no rebuildable dialog
// (multi-select, view-only) cannot be told from "another dialog", so a visible
// dialog is indeterminate for it and only a positively absent one is gone.
func judgeRow(row *session.GuidanceRequestData, capture string, err error) dialogVerdict {
	if err != nil {
		return verdictIndeterminate
	}
	presence := session.PaneShowsDialog(capture)
	dialog, known := dialogFromRow(row)
	if known && session.DialogMatch(capture, dialog) {
		return verdictMatch
	}
	switch {
	case presence.Absent():
		return verdictGone
	case presence == session.DialogShowing && known:
		return verdictReplaced
	}
	return verdictIndeterminate
}

func (v dialogVerdict) cancelReason() (domain.CancelReason, bool) {
	switch v {
	case verdictGone:
		return domain.CancelReasonDialogGone, true
	case verdictReplaced:
		return domain.CancelReasonDialogReplaced, true
	case verdictIndeterminate, verdictMatch:
	}
	return "", false
}

type samplerStore interface {
	ListAllPendingGuidanceRequests(ctx context.Context) ([]*session.GuidanceRequestData, error)
}

type samplerClearer interface {
	Clear(ctx context.Context, rowID string, reason domain.CancelReason) error
}

type sampleState struct {
	last    time.Time
	verdict dialogVerdict
	streak  int
}

// DialogGoneSampler watches open hook rows for a dialog that vanished without a
// PostToolUse receipt. Tick does all the work, so tests drive it with a fake clock.
type DialogGoneSampler struct {
	store   samplerStore
	capture func(ctx context.Context, sessionUUID string) (string, error)
	clearer samplerClearer
	now     func() time.Time
	// states is touched only by the goroutine calling Tick.
	states map[string]*sampleState
}

// NewDialogGoneSampler builds the sampler; now may be nil.
func NewDialogGoneSampler(store samplerStore, capture func(context.Context, string) (string, error), clearer samplerClearer, now func() time.Time) *DialogGoneSampler {
	if now == nil {
		now = time.Now
	}
	return &DialogGoneSampler{store: store, capture: capture, clearer: clearer, now: now, states: map[string]*sampleState{}}
}

// watched reports whether row is a hook row still inside the sampling bound.
func (s *DialogGoneSampler) watched(row *session.GuidanceRequestData, now time.Time) bool {
	return row.Source == domain.QuestionSourceHook && row.AskerSessionUUID != "" && now.Sub(row.CreatedAt) <= samplerStopAfter
}

func (s *DialogGoneSampler) due(row *session.GuidanceRequestData, now time.Time) bool {
	st := s.states[row.ID]
	if st == nil {
		return true
	}
	every := samplerSlowEvery
	if now.Sub(row.CreatedAt) <= samplerFastWindow {
		every = samplerFastEvery
	}
	return now.Sub(st.last) >= every
}

// Tick samples up to samplerMaxPerPass due rows, longest-unsampled first.
func (s *DialogGoneSampler) Tick(ctx context.Context) {
	rows, err := s.store.ListAllPendingGuidanceRequests(ctx)
	if err != nil {
		log.Warn("[DialogGoneSampler] list failed", "err", err)
		return
	}
	now := s.now()
	open := make(map[string]bool, len(rows))
	var due []*session.GuidanceRequestData
	for _, row := range rows {
		open[row.ID] = true
		if s.watched(row, now) && s.due(row, now) {
			due = append(due, row)
		}
	}
	for id := range s.states {
		if !open[id] {
			delete(s.states, id)
		}
	}
	sortLongestUnsampled(due, s.states)
	if len(due) > samplerMaxPerPass {
		due = due[:samplerMaxPerPass]
	}
	for _, row := range due {
		s.sample(ctx, row, now)
	}
}

func sortLongestUnsampled(rows []*session.GuidanceRequestData, states map[string]*sampleState) {
	last := func(r *session.GuidanceRequestData) time.Time {
		if st := states[r.ID]; st != nil {
			return st.last
		}
		return time.Time{}
	}
	sort.SliceStable(rows, func(i, j int) bool { return last(rows[i]).Before(last(rows[j])) })
}

func (s *DialogGoneSampler) sample(ctx context.Context, row *session.GuidanceRequestData, now time.Time) {
	capture, err := s.capture(ctx, row.AskerSessionUUID)
	v := judgeRow(row, capture, err)
	st := s.states[row.ID]
	if st == nil {
		st = &sampleState{}
		s.states[row.ID] = st
	}
	st.last = now
	if v != st.verdict {
		st.verdict, st.streak = v, 0
	}
	st.streak++
	reason, terminal := v.cancelReason()
	if !terminal || st.streak < samplerConfirm {
		return
	}
	if cerr := s.clearer.Clear(ctx, row.ID, reason); cerr != nil {
		log.Warn("[DialogGoneSampler] clear failed", "row", row.ID, "reason", reason, "err", cerr)
		return
	}
	delete(s.states, row.ID)
}

// Run ticks every samplerFastEvery until ctx is done.
func (s *DialogGoneSampler) Run(ctx context.Context) {
	t := time.NewTicker(samplerFastEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Tick(ctx)
		}
	}
}
