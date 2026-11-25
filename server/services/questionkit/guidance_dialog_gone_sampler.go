package questionkit

import (
	"context"
	"sort"
	"time"

	"github.com/tstapler/stapler-squad/session/dialogpresence"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/panereply"
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
	SamplerMaxPerPass = 20
	// samplerConfirm is the number of consecutive matching samples that confirm a verdict.
	samplerConfirm = 2
)

// DialogVerdict is what one pane capture says about one open row.
type DialogVerdict uint8

const (
	VerdictIndeterminate DialogVerdict = iota // capture error, unknown layout or unverifiable
	VerdictMatch                              // the row's own dialog is on screen
	VerdictGone                               // the pane positively shows no dialog
	VerdictReplaced                           // another dialog is on screen
)

// JudgeRow classifies capture against row. A row with no rebuildable dialog
// (multi-select, view-only) cannot be told from "another dialog", so a visible
// dialog is indeterminate for it and only a positively absent one is gone.
func JudgeRow(row *domain.GuidanceRequestData, capture string, err error) DialogVerdict {
	if err != nil {
		return VerdictIndeterminate
	}
	presence := dialogpresence.PaneShowsDialog(capture)
	if qs, tabbed := TabbedFromRow(row); tabbed {
		return judgeTabbed(TabbedHeadersMatch(qs), capture, presence)
	}
	dialog, known := DialogFromRow(row)
	if known && panereply.DialogMatch(capture, dialog) {
		return VerdictMatch
	}
	switch {
	case presence.Absent():
		return VerdictGone
	case presence == dialogpresence.DialogShowing && known:
		return VerdictReplaced
	}
	return VerdictIndeterminate
}

// judgeTabbed is JudgeRow for a tabbed row: its own frame is a match, any
// other dialog on screen replaced it, and an unrecognised layout is not judged.
func judgeTabbed(own func(string) bool, capture string, presence dialogpresence.DialogPresence) DialogVerdict {
	switch {
	case own(capture):
		return VerdictMatch
	case presence.Absent():
		return VerdictGone
	case presence == dialogpresence.DialogShowing:
		return VerdictReplaced
	}
	return VerdictIndeterminate
}

func (v DialogVerdict) cancelReason() (domain.CancelReason, bool) {
	switch v {
	case VerdictGone:
		return domain.CancelReasonDialogGone, true
	case VerdictReplaced:
		return domain.CancelReasonDialogReplaced, true
	case VerdictIndeterminate, VerdictMatch:
	}
	return "", false
}

type samplerStore interface {
	ListAllPendingGuidanceRequests(ctx context.Context) ([]*domain.GuidanceRequestData, error)
}

type samplerClearer interface {
	Clear(ctx context.Context, rowID string, reason domain.CancelReason) error
}

type sampleState struct {
	last    time.Time
	verdict DialogVerdict
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
func (s *DialogGoneSampler) watched(row *domain.GuidanceRequestData, now time.Time) bool {
	return row.Source == domain.QuestionSourceHook && row.AskerSessionUUID != "" && now.Sub(row.CreatedAt) <= samplerStopAfter
}

func (s *DialogGoneSampler) due(row *domain.GuidanceRequestData, now time.Time) bool {
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

// Tick samples up to SamplerMaxPerPass due rows, longest-unsampled first.
func (s *DialogGoneSampler) Tick(ctx context.Context) {
	rows, err := s.store.ListAllPendingGuidanceRequests(ctx)
	if err != nil {
		log.Warn("[DialogGoneSampler] list failed", "err", err)
		return
	}
	now := s.now()
	open := make(map[string]bool, len(rows))
	var due []*domain.GuidanceRequestData
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
	if len(due) > SamplerMaxPerPass {
		due = due[:SamplerMaxPerPass]
	}
	for _, row := range due {
		s.sample(ctx, row, now)
	}
}

func sortLongestUnsampled(rows []*domain.GuidanceRequestData, states map[string]*sampleState) {
	last := func(r *domain.GuidanceRequestData) time.Time {
		if st := states[r.ID]; st != nil {
			return st.last
		}
		return time.Time{}
	}
	sort.SliceStable(rows, func(i, j int) bool { return last(rows[i]).Before(last(rows[j])) })
}

func (s *DialogGoneSampler) sample(ctx context.Context, row *domain.GuidanceRequestData, now time.Time) {
	capture, err := s.capture(ctx, row.AskerSessionUUID)
	v := JudgeRow(row, capture, err)
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
