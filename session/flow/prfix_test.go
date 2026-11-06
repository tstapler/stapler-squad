package flow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var prCapture = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

func trig(item string, value string, at time.Time) Marker {
	return Marker{Item: ItemID(item), Kind: MarkerPRFixTrigger, Value: value, PRNumber: 1, At: at}
}

func outcome(item, value string, at time.Time) Marker {
	return Marker{Item: ItemID(item), Kind: MarkerPRFixOutcome, Value: value, PRNumber: 1, At: at}
}

func TestPRFixReader_should_JoinReopenToTriggerWithin35m_When_TriggerRecordsExist(t *testing.T) {
	t.Parallel()
	t0 := prCapture.Add(24 * time.Hour)
	j := NewTriggerJoin([]Marker{
		trig("a", "ci", t0),
		trig("b", "ci", t0), trig("b", "conflict", t0.Add(10*time.Minute)), // two triggers in a row: latest wins
		trig("c", "review", t0),
	}, prCapture)

	assert.Equal(t, PRFixTriggerCI, j.TriggerFor("a", t0.Add(time.Minute)), "hit")
	assert.Equal(t, PRFixTriggerCI, j.TriggerFor("a", t0.Add(35*time.Minute)), "boundary is inclusive")
	assert.Equal(t, PRFixTriggerUnknown, j.TriggerFor("a", t0.Add(35*time.Minute+time.Second)), "miss: older than 35m")
	assert.Equal(t, PRFixTriggerUnknown, j.TriggerFor("none", t0), "no marker for the item is unknown")
	assert.Equal(t, PRFixTriggerConflict, j.TriggerFor("b", t0.Add(20*time.Minute)), "latest of two triggers")
	assert.Equal(t, PRFixTriggerCI, j.TriggerFor("b", t0.Add(5*time.Minute)), "a later marker does not apply to an earlier event")
	assert.Equal(t, PRFixTriggerUnknown, j.TriggerFor("a", t0.Add(-time.Second)), "marker after the event never joins")
}

func TestPRFixReader_should_JoinDedupedLoopEventToEarlierMarker_When_ConstantTrigger(t *testing.T) {
	t.Parallel()
	t0 := prCapture.Add(time.Hour)
	// The writer dedupes a constant-trigger loop to one marker per 30m, so an
	// event 29m after the last marker still has a record inside 35m.
	j := NewTriggerJoin([]Marker{trig("loop", "ci", t0), trig("loop", "ci", t0.Add(30*time.Minute))}, prCapture)

	assert.Equal(t, PRFixTriggerCI, j.TriggerFor("loop", t0.Add(59*time.Minute)))
}

func TestPRFixReader_should_StayUnknown_When_EventBeforeCaptureOrJoinInactive(t *testing.T) {
	t.Parallel()
	before := prCapture.Add(-time.Minute)
	j := NewTriggerJoin([]Marker{trig("a", "ci", before.Add(-time.Minute))}, prCapture)

	assert.Equal(t, PRFixTriggerUnknown, j.TriggerFor("a", before), "forward-only")
	var nilJoin *TriggerJoin
	assert.Equal(t, PRFixTriggerUnknown, nilJoin.TriggerFor("a", prCapture.Add(time.Hour)))
	assert.False(t, nilJoin.Active())
	assert.False(t, NewTriggerJoin(nil, time.Time{}).Active())
}

func systemReopen(t *testing.T, item, by string, at time.Time) RealSpan {
	t.Helper()
	return s12RealNote(t, item, stPRPend, stProg, at, "auto-reopened for PR fix (CI/review)").WithTriggeredBy(by)
}

func prWindow(t *testing.T) FlowWindow {
	t.Helper()
	return dwellWindow(t, s12Base, s12Base.Add(30*24*time.Hour))
}

func TestPRFixReader_should_DeriveRespawnedOnlyFromSystemReopensAtOrAfterCapture(t *testing.T) {
	t.Parallel()
	at := prCapture.Add(48 * time.Hour)
	spans := []Span{
		systemReopen(t, "sys", "system", at),
		systemReopen(t, "forged-agent", "agent", at), // same note, agent actor: not counted
		systemReopen(t, "forged-user", "user", at),
		systemReopen(t, "old", "system", prCapture.Add(-time.Hour)), // before capture
		s12RealNote(t, "plain", stPRPend, stProg, at, "manual move").WithTriggeredBy("system"),
	}

	got := PRFixOutcomeCounts(PRFixOutcomeInput{Window: prWindow(t), Spans: spans, Since: prCapture})

	assert.True(t, got.Recording)
	assert.Equal(t, 1, got.Respawned)
	assert.Zero(t, got.Steered+got.SkippedActive)
}

func TestPRFixReader_should_CountMarkerOutcomesAndResolvedWithin24h_When_WindowHasThem(t *testing.T) {
	t.Parallel()
	at := prCapture.Add(48 * time.Hour)
	markers := []Marker{
		outcome("a", "steered", at), outcome("b", "steered", at), outcome("c", "skipped_active", at),
		outcome("d", "steered", prCapture.Add(-time.Hour)),    // before capture
		outcome("e", "steered", s12Base.Add(40*24*time.Hour)), // after window
		outcome("f", "respawned", at),                         // never stored; ignored, derived instead
		trig("a", "ci", at),                                   // not an outcome
	}
	spans := []Span{
		s12Real(t, "a", stPRPend, stDoneStat, at.Add(23*time.Hour)), // resolved within 24h
		s12Real(t, "b", stPRPend, stDoneStat, at.Add(25*time.Hour)), // too late
		s12Real(t, "c", stPRPend, stDoneStat, at.Add(-time.Hour)),   // merged before the outcome
		systemReopen(t, "r", "system", at),                          // respawned
		s12Real(t, "r", stProg, stReview, at.Add(time.Hour)),
		s12Real(t, "r", stReview, stDoneStat, at.Add(10*time.Hour)), // resolved within 24h
	}

	got := PRFixOutcomeCounts(PRFixOutcomeInput{Window: prWindow(t), Spans: spans, Markers: markers, Since: prCapture})

	assert.Equal(t, PRFixOutcomes{Recording: true, Steered: 2, Respawned: 1, SkippedActive: 1, ResolvedWithin24: 2}, got)
}

func TestPRFixReader_should_ReportNotRecordingWithoutCounts_When_NoOutcomeCapture(t *testing.T) {
	t.Parallel()
	got := PRFixOutcomeCounts(PRFixOutcomeInput{
		Window: prWindow(t), Markers: []Marker{outcome("a", "steered", prCapture)},
		Spans: []Span{systemReopen(t, "a", "system", prCapture)},
	})

	assert.Equal(t, PRFixOutcomes{}, got)
}

func TestRework_should_SplitPRFixByTriggerOnlyAfterCapture_When_JoinActive(t *testing.T) {
	t.Parallel()
	after := prCapture.Add(24 * time.Hour)
	spans := []Span{
		s12Real(t, "pre", stPRPend, stProg, prCapture.Add(-time.Hour)),
		s12Real(t, "ci", stPRPend, stProg, after),
		s12Real(t, "miss", stPRPend, stProg, after),
	}
	join := NewTriggerJoin([]Marker{trig("ci", "ci", after.Add(-time.Minute)), trig("pre", "ci", prCapture.Add(-2*time.Hour))}, prCapture)

	got := ReworkWith(spans, DefaultConfig(), join)

	assert.Equal(t, map[PRFixTrigger]int{PRFixTriggerCI: 1, PRFixTriggerUnknown: 2}, got.PRFixTriggers)
	assert.Equal(t, []string{"pr_fix_cause:since_capture"}, got.Estimated)
	none := ReworkWith(spans, DefaultConfig(), nil)
	assert.Equal(t, map[PRFixTrigger]int{PRFixTriggerUnknown: 3}, none.PRFixTriggers)
	assert.Equal(t, []string{"pr_fix_cause:unavailable"}, none.Estimated)
}

type fakeMarkers struct {
	markers []Marker
	capture MarkerCapture
	err     error
}

func (f fakeMarkers) Markers(_ context.Context, from, to time.Time) ([]Marker, error) {
	if f.err != nil {
		return nil, f.err
	}
	var out []Marker
	for _, m := range f.markers {
		if !m.At.Before(from) && m.At.Before(to) {
			out = append(out, m)
		}
	}
	return out, nil
}

func (f fakeMarkers) MarkerCapture(context.Context) (MarkerCapture, error) { return f.capture, f.err }

func prFixFake(items ...string) *fakeSrc {
	f := newFakeSrc()
	for _, id := range items {
		f.events = append(f.events,
			Event{ItemID: ItemID(id), From: stProg, To: stPRPend, At: fixtureClock.T.Add(-9 * 24 * time.Hour), Seq: 1},
			Event{ItemID: ItemID(id), From: stPRPend, To: stProg, At: fixtureClock.T.Add(-5 * 24 * time.Hour), Seq: 2,
				Note: "auto-reopened for PR fix (CI/review)", TriggeredBy: "system"},
			Event{ItemID: ItemID(id), From: stProg, To: stPRPend, At: fixtureClock.T.Add(-5*24*time.Hour + 3*time.Hour), Seq: 3},
		)
	}
	return f
}

func prSummary(t *testing.T, f *fakeSrc, m MarkerSource, capture time.Time) *FlowSummary {
	t.Helper()
	src := f.sources()
	src.Markers = m
	opts := []AggregatorOption{}
	if !capture.IsZero() {
		opts = append(opts, WithCaptureSince(func(context.Context) (time.Time, error) { return capture, nil }))
	}
	s, err := NewAggregator(src, fixtureClock, opts...).Summary(context.Background(), FlowQuery{Window: WindowSpec{Days: 30}})
	require.NoError(t, err)
	return s
}

func TestAggregator_should_SplitPRFixAndCountOutcomes_When_MarkersAndCaptureExist(t *testing.T) {
	t.Parallel()
	reopen := fixtureClock.T.Add(-5 * 24 * time.Hour)
	capture := reopen.Add(-24 * time.Hour)
	m := fakeMarkers{
		markers: []Marker{trig("x", "ci", reopen.Add(-time.Minute)), outcome("x", "steered", reopen.Add(time.Hour)), outcome("x", "skipped_active", reopen.Add(2*time.Hour))},
		capture: MarkerCapture{FirstOutcome: reopen.Add(time.Hour), FirstTrigger: reopen.Add(-time.Minute)},
	}

	s := prSummary(t, prFixFake("x"), m, capture)

	assert.Equal(t, capture, s.Meta.CaptureSince)
	assert.Equal(t, PROutcomes{Recording: true, Steered: 1, Respawned: 0, SkippedActive: 1}, s.PR.Outcomes,
		"the reopen precedes the first outcome marker, so it is not counted as respawned")
	assert.Contains(t, s.PR.Meta.Estimated, "pr_fix_outcome:since_capture")
	assert.Equal(t, []string{"pr_fix_cause:since_capture"}, s.Causes.Meta.Estimated)
	assert.Equal(t, []TriggerRow{{Trigger: "ci", Count: 1}}, s.Causes.PRFixTriggers)
}

func TestAggregator_should_ReportNotRecordingAndKeepUnavailable_When_NoMarkerSourceOrCapture(t *testing.T) {
	t.Parallel()
	for name, m := range map[string]MarkerSource{"nil source": nil, "empty source": fakeMarkers{}} {
		s := prSummary(t, prFixFake("x"), m, time.Time{})

		assert.Equal(t, PROutcomes{}, s.PR.Outcomes, name)
		assert.Contains(t, s.PR.Meta.Estimated, "pr_fix_cause:unavailable", name)
		assert.True(t, s.Meta.CaptureSince.IsZero(), name)
	}
}

func TestAggregator_should_MarkPRSectionPartial_When_MarkerReadFails(t *testing.T) {
	t.Parallel()
	s := prSummary(t, prFixFake("x"), fakeMarkers{err: errors.New("marker table locked")}, prCapture)

	assert.Equal(t, SectionPartial, s.PR.Meta.Status)
	assert.Contains(t, s.PR.Meta.Error, "marker table locked")
	assert.Equal(t, 1, s.PR.Cycles.Value, "dwell survives the marker failure")
}
