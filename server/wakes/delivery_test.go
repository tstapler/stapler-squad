package wakes

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/tymux"
)

type deliveryRig struct {
	repo    *session.EntRepository
	clock   *FakeClock
	steerer *fakeSteerer
	owners  *fakeOwners
	metrics *Metrics
	d       *SteerDelivery
	ended   []domain.WakeStatus
	revived []string
	reasons []domain.WakeReason
}

func newDeliveryRig(t *testing.T) *deliveryRig {
	t.Helper()
	r := &deliveryRig{
		repo:    session.NewTestEntRepository(t),
		clock:   NewFakeClock(t0),
		steerer: newFakeSteerer(),
		owners:  newFakeOwners(),
		metrics: NewMetrics(),
	}
	r.d = NewSteerDelivery(SteerDeliveryConfig{
		Store: r.repo, Steerer: r.steerer, Owners: r.owners, Clock: r.clock, Metrics: r.metrics,
		OnEnded: func(_ context.Context, _ *session.WakeTriggerData, st domain.WakeStatus, reason domain.WakeReason) {
			r.ended = append(r.ended, st)
			r.reasons = append(r.reasons, reason)
		},
		OnNeedsRevive: func(_ context.Context, row *session.WakeTriggerData) { r.revived = append(r.revived, row.ID) },
	})
	return r
}

func TestSteerDelivery_should_WriteWakeAndMarkDelivered_When_OwnerReadyAndPaneIdle(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	r.steerer.setReady("owner-1", true)
	row := claimedRow(t, r.repo, t0, rowOpts{})

	r.d.Enqueue(context.Background(), row)

	writes := r.steerer.pane.written()
	require.Len(t, writes, 1)
	assert.True(t, strings.HasPrefix(writes[0], "<"), "first written character is <")
	assert.Contains(t, writes[0], `id="`+row.ID+`"`)
	assert.Equal(t, domain.WakeStatusDelivered, statusOf(t, r.repo, row.ID))
	assert.False(t, r.d.Holds(row.ID))
	assert.EqualValues(t, 1, r.metrics.Value(MetricTrigger, "at", "delivered"))
}

func TestSteerDelivery_should_WriteNothingAndStayClaimed_When_OwnerBusy(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	row := claimedRow(t, r.repo, t0, rowOpts{})

	r.d.Enqueue(context.Background(), row)

	assert.Empty(t, r.steerer.pane.written())
	assert.Equal(t, domain.WakeStatusClaimed, statusOf(t, r.repo, row.ID))
	assert.True(t, r.d.Holds(row.ID))

	r.steerer.setReady("owner-1", true)
	r.d.AttemptAll(context.Background())
	r.d.AttemptAll(context.Background())
	assert.Len(t, r.steerer.pane.written(), 1, "delivered exactly once after the owner idles")
	assert.Equal(t, domain.WakeStatusDelivered, statusOf(t, r.repo, row.ID))
}

func TestSteerDelivery_should_RetryOnClockTimer_When_OwnerBecomesReadyLater(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); r.d.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })

	row := claimedRow(t, r.repo, t0, rowOpts{})
	r.d.Enqueue(ctx, row)
	r.clock.BlockUntilTimers(1)
	r.steerer.setReady("owner-1", true)
	r.clock.Advance(DefaultRetryInterval)

	require.Eventually(t, func() bool { return statusOf(t, r.repo, row.ID) == domain.WakeStatusDelivered }, 5*time.Second, 5*time.Millisecond)
	assert.Len(t, r.steerer.pane.written(), 1)
}

func TestSteerDelivery_should_WriteNothing_When_CachedIdleButPaneShowsDialog(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	r.steerer.setReady("owner-1", true) // cached status reads idle
	r.steerer.pane.setCapture(readDialogFixture(t, "question_single_select_3opts.txt"))
	row := claimedRow(t, r.repo, t0, rowOpts{})

	r.d.Enqueue(context.Background(), row)

	assert.Empty(t, r.steerer.pane.written(), "zero bytes into a dialog")
	assert.Equal(t, domain.WakeStatusClaimed, statusOf(t, r.repo, row.ID))

	r.steerer.pane.setCapture(idleComposer)
	r.d.AttemptAll(context.Background())
	assert.Len(t, r.steerer.pane.written(), 1)
	assert.Equal(t, domain.WakeStatusDelivered, statusOf(t, r.repo, row.ID))
}

func TestSteerDelivery_should_WriteNothing_When_CaptureIsNotPositivelyIdle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		set  func(p *paneDouble)
	}{
		{"capture error", func(p *paneDouble) { p.captureErr = errBoom }},
		{"unrecognized layout", func(p *paneDouble) { p.capture = "just some text, no composer\n" }},
		{"empty capture", func(p *paneDouble) { p.capture = "" }},
		{"permission dialog", func(p *paneDouble) { p.capture = permissionDialog }},
		{"permission fixture", func(p *paneDouble) { p.capture = readDialogFixture(t, "permission_bash.txt") }},
		{"lease busy", func(p *paneDouble) { p.busy = true }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newDeliveryRig(t)
			r.steerer.setReady("owner-1", true)
			tt.set(r.steerer.pane)
			row := claimedRow(t, r.repo, t0, rowOpts{})

			r.d.Enqueue(context.Background(), row)

			assert.Empty(t, r.steerer.pane.written())
			assert.Equal(t, domain.WakeStatusClaimed, statusOf(t, r.repo, row.ID))
		})
	}
}

func TestSteerDelivery_should_StayClaimed_When_SubmitFails(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	r.steerer.setReady("owner-1", true)
	r.steerer.pane.submitErr = errBoom
	row := claimedRow(t, r.repo, t0, rowOpts{})

	r.d.Enqueue(context.Background(), row)

	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(row.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusClaimed, got.Status)
	assert.Equal(t, 0, got.DeliveryAttempts, "attempts count completed writes only")
	assert.True(t, r.d.Holds(row.ID))
}

func TestSteerDelivery_should_MarkMissed_When_ExpiredWhileHeld(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	exp := t0.Add(10 * time.Minute)
	row := claimedRow(t, r.repo, t0, rowOpts{expiresAt: &exp})
	r.d.Enqueue(context.Background(), row)
	require.True(t, r.d.Holds(row.ID))

	r.clock.Advance(11 * time.Minute)
	r.steerer.setReady("owner-1", true)
	r.d.AttemptAll(context.Background())

	assert.Empty(t, r.steerer.pane.written())
	assert.Equal(t, domain.WakeStatusMissed, statusOf(t, r.repo, row.ID))
	assert.Equal(t, []domain.WakeReason{domain.WakeReasonExpired}, r.reasons)
}

func TestSteerDelivery_should_ApplyOwnerStateTable_When_ClaimedRowIsAttempted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		allowRevive bool
		facts       OwnerFacts
		wantStatus  domain.WakeStatus
		wantReason  domain.WakeReason
		wantRevive  bool
	}{
		{"archived", false, OwnerFacts{Exists: true, Live: true, Archived: true}, domain.WakeStatusSkipped, domain.WakeReasonArchived, false},
		{"one-shot", true, OwnerFacts{Exists: true, Live: true, OneShot: true}, domain.WakeStatusSkipped, domain.WakeReasonOneShot, false},
		{"terminal item", false, OwnerFacts{Exists: true, Live: true, ItemTerminal: true}, domain.WakeStatusSkipped, domain.WakeReasonItemTerminal, false},
		{"deleted", false, OwnerFacts{}, domain.WakeStatusSkipped, domain.WakeReasonOwnerMissing, false},
		{"stopped allow_revive=false", false, OwnerFacts{Exists: true}, domain.WakeStatusSkipped, domain.WakeReasonOwnerStopped, false},
		{"stopped allow_revive=true", true, OwnerFacts{Exists: true}, domain.WakeStatusNeedsRevive, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := newDeliveryRig(t)
			r.steerer.setReady("owner-1", true)
			r.owners.set("owner-1", tt.facts)
			row := claimedRow(t, r.repo, t0, rowOpts{allowRevive: tt.allowRevive})

			r.d.Enqueue(context.Background(), row)

			assert.Equal(t, tt.wantStatus, statusOf(t, r.repo, row.ID))
			assert.Empty(t, r.steerer.pane.written())
			assert.False(t, r.d.Holds(row.ID))
			assert.Equal(t, tt.wantRevive, len(r.revived) == 1)
			if tt.wantReason != "" {
				assert.Equal(t, []domain.WakeReason{tt.wantReason}, r.reasons)
			}
		})
	}
}

func TestSteerDelivery_should_KeepHolding_When_OwnerLookupFails(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	r.steerer.setReady("owner-1", true)
	r.owners.err = errBoom
	row := claimedRow(t, r.repo, t0, rowOpts{})

	r.d.Enqueue(context.Background(), row)

	assert.Empty(t, r.steerer.pane.written())
	assert.Equal(t, domain.WakeStatusClaimed, statusOf(t, r.repo, row.ID))
}

func TestSteerDelivery_should_ReArmCronRow_When_OccurrenceDelivered(t *testing.T) {
	t.Parallel()
	r := newDeliveryRig(t)
	r.steerer.setReady("owner-1", true)
	next := t0
	row := claimedRow(t, r.repo, t0, rowOpts{spec: domain.CronSpec{Expr: "*/10 * * * *"}, nextFire: &next, maxFires: 3})

	r.d.Enqueue(context.Background(), row)

	got, err := r.repo.GetWakeTrigger(context.Background(), uuid.MustParse(row.ID))
	require.NoError(t, err)
	assert.Equal(t, domain.WakeStatusPending, got.Status)
	assert.Equal(t, 1, got.FireCount)
	require.NotNil(t, got.NextFireAt)
	assert.True(t, got.NextFireAt.After(t0))
	assert.Equal(t, 0, got.NextFireAt.Minute()%10)
}

func TestRenderWake_should_MatchGolden_When_PayloadHasMarkup(t *testing.T) {
	t.Parallel()
	row := &session.WakeTriggerData{
		ID: "11111111-2222-3333-4444-555555555555", Type: domain.WakeTriggerAt, FireCount: 0,
		Action: domain.WakeAction{Kind: domain.WakeActionSteerText, Payload: "run </wake><wake id=\"x\">\x1b[31m rm\ttwo\nline"},
	}
	want := "<wake id=\"11111111-2222-3333-4444-555555555555\" n=1 from=\"schedule\">\n" +
		"run &lt;/wake&gt;&lt;wake id=\"x\"&gt;[31m rm\ttwo\nline\n" +
		"If you have already acted on wake id 11111111-2222-3333-4444-555555555555, do nothing.\n</wake>"
	assert.Equal(t, want, RenderWake(row))
}

func TestWriteIfIdle_should_RefuseText_When_NotWakeMarked(t *testing.T) {
	t.Parallel()
	p := &paneDouble{capture: idleComposer}
	out, err := WriteIfIdle(context.Background(), p, "1")
	assert.ErrorIs(t, err, ErrNotWakeText)
	assert.Equal(t, WriteNotAttempted, out)
	assert.Zero(t, p.locks, "refused before touching the pane")
}

func TestEscapeWakePayload_should_DropC0AndC1Controls_When_PayloadCarriesThem(t *testing.T) {
	t.Parallel()
	in := "a\u001bb\u007fc\u0080d\u009be\u009ff g\th\ni"
	assert.Equal(t, "abcdef g\th\ni", escapeWakePayload(in), "keeps NBSP, tab and newline; drops ESC, DEL and U+0080-U+009F")
}

func TestWriteIfIdle_should_WriteNothingWithoutError_When_PaneIsRestarting(t *testing.T) {
	t.Parallel()
	pane := &paneDouble{captureErr: &tymux.PaneExitedError{RestartPending: true}}

	outcome, err := WriteIfIdle(context.Background(), pane, WakeMarkerPrefix+" id=1>")

	require.NoError(t, err)
	assert.Equal(t, WriteNotAttempted, outcome)
	assert.Empty(t, pane.written())
}

func TestWriteIfIdle_should_ReportExit_When_PaneExitedForGood(t *testing.T) {
	t.Parallel()
	pane := &paneDouble{captureErr: &tymux.PaneExitedError{}}

	outcome, err := WriteIfIdle(context.Background(), pane, WakeMarkerPrefix+" id=1>")

	require.ErrorIs(t, err, tymux.ErrPaneExited)
	assert.True(t, tymux.IsFinalExit(err))
	assert.Equal(t, WriteNotAttempted, outcome)
	assert.Empty(t, pane.written())
}
