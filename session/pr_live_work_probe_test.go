package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

type fakeSnapshotSource map[string]*InstanceSnapshot

func (f fakeSnapshotSource) LiveSnapshot(id string) (*InstanceSnapshot, bool) {
	s, ok := f[id]
	return s, ok
}

type fakeProbeInstanceStore struct {
	data map[string]*InstanceData
	err  error
}

func (f *fakeProbeInstanceStore) FindInstanceDataByID(id string) (*InstanceData, error) {
	if f.err != nil {
		return nil, f.err
	}
	d, ok := f.data[id]
	if !ok {
		return nil, ErrInstanceDataNotFound
	}
	return d, nil
}

func TestLiveWorkProbe_ShouldMapSnapshotToLiveEndedIdle_AndHeldByLiveWorkShouldHoldOnErrorAndNilMeansToday(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	archived := now.Add(-time.Hour)
	mk := func(status domain.Status, lmca time.Time, mut func(*InstanceSnapshot)) *InstanceSnapshot {
		s := &InstanceSnapshot{Status: status, LastMeaningfulChangeAt: lmca, CreatedAt: now.Add(-3 * time.Hour)}
		if mut != nil {
			mut(s)
		}
		return s
	}
	turnEndedAt := now.Add(-30 * time.Minute)
	tests := []struct {
		name string
		snap *InstanceSnapshot
		want bool
	}{
		{"active mid-turn, recent change", mk(domain.Active, now.Add(-2*time.Minute), nil), true},
		{"paused is ended", mk(domain.Paused, now, nil), false},
		{"stopped is ended", mk(domain.Stopped, now, nil), false},
		{"hibernated is ended", mk(domain.Hibernated, now, nil), false},
		{"archived is ended", mk(domain.Active, now, func(s *InstanceSnapshot) { s.ArchivedAt = &archived }), false},
		{"turn ended and idle past the window", mk(domain.Active, turnEndedAt, func(s *InstanceSnapshot) { s.TurnEnded = domain.TurnEnded{At: turnEndedAt} }), false},
		{"turn ended recently is still live inside the window", mk(domain.Active, now.Add(-time.Minute), func(s *InstanceSnapshot) { s.TurnEnded = domain.TurnEnded{At: now.Add(-time.Minute)} }), true},
		{"turn ended, then new activity: working again", mk(domain.Active, now.Add(-5*time.Minute), func(s *InstanceSnapshot) { s.TurnEnded = domain.TurnEnded{At: now.Add(-20 * time.Minute)} }), true},
		{"working with no change for over 1h is capped, not live", mk(domain.Active, now.Add(-61*time.Minute), nil), false},
		{"zero LMCA falls back to creation time (over 1h ago: capped)", mk(domain.Active, time.Time{}, nil), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			p := NewSnapshotLiveWorkProbe(fakeSnapshotSource{"s": tt.snap}, &fakeProbeInstanceStore{})
			got, err := p.IsLive(context.Background(), "s", now)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLiveWorkProbe_ShouldReturnNotLiveWithoutError_WhenSessionAbsentOrPersistedEnded(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	archived := now
	store := &fakeProbeInstanceStore{data: map[string]*InstanceData{
		"archived": {Status: domain.Active, ArchivedAt: &archived, LastMeaningfulOutput: now},
		"stopped":  {Status: domain.Stopped, LastMeaningfulOutput: now},
		"recent":   {Status: domain.Active, LastMeaningfulOutput: now.Add(-time.Minute)},
		"stale":    {Status: domain.Active, LastMeaningfulOutput: now.Add(-2 * time.Hour)},
	}}
	p := NewSnapshotLiveWorkProbe(fakeSnapshotSource{}, store)
	for id, want := range map[string]bool{"missing": false, "archived": false, "stopped": false, "recent": true, "stale": false, "": false} {
		got, err := p.IsLive(context.Background(), id, now)
		require.NoError(t, err, id)
		assert.Equal(t, want, got, id)
	}
}

func TestLiveWorkProbe_ShouldHoldOnTransientErrorButOnlyForOneHour_AndRecoverOnSuccess(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	store := &fakeProbeInstanceStore{err: errors.New("db busy")}
	p := NewSnapshotLiveWorkProbe(fakeSnapshotSource{}, store)
	before := LiveWorkResultCount(LiveWorkResultCapError)

	live, err := p.IsLive(context.Background(), "s", now)
	require.NoError(t, err)
	assert.True(t, live, "a transient failure counts as live")
	live, _ = p.IsLive(context.Background(), "s", now.Add(59*time.Minute))
	assert.True(t, live, "still inside the error-hold cap")
	live, _ = p.IsLive(context.Background(), "s", now.Add(time.Hour))
	assert.False(t, live, "the error-hold is capped at 1h")
	assert.Greater(t, LiveWorkResultCount(LiveWorkResultCapError), before, "the cap is counted")

	store.err = nil
	live, _ = p.IsLive(context.Background(), "s", now.Add(2*time.Hour))
	assert.False(t, live, "absent session after recovery: not live")
	store.err = errors.New("db busy again")
	live, _ = p.IsLive(context.Background(), "s", now.Add(3*time.Hour))
	assert.True(t, live, "a new failure starts a new hold window")
}

func TestIsLiveWork_ShouldCapMidTurnSession_WhenNoChangeForOverOneHour(t *testing.T) {
	t.Parallel()
	now := prTestEpoch
	f := LiveWorkFacts{Unended: true, Working: true, LastMeaningfulChangeAt: now.Add(-59 * time.Minute)}
	assert.True(t, IsLiveWork(f, now, LiveWorkIdleWindow))
	f.LastMeaningfulChangeAt = now.Add(-time.Hour)
	assert.False(t, IsLiveWork(f, now, LiveWorkIdleWindow))
	before := LiveWorkResultCount(LiveWorkResultCapWorking)
	snap := &InstanceSnapshot{Status: domain.Active, LastMeaningfulChangeAt: now.Add(-2 * time.Hour)}
	p := NewSnapshotLiveWorkProbe(fakeSnapshotSource{"s": snap}, &fakeProbeInstanceStore{})
	_, _ = p.IsLive(context.Background(), "s", now)
	assert.Greater(t, LiveWorkResultCount(LiveWorkResultCapWorking), before, "the cap firing is counted")
}

func TestLiveWorkHoldStart_ShouldUseFirstMerge_ThenLastPolled_ThenFirstObserved(t *testing.T) {
	t.Parallel()
	obs := prTestEpoch.Add(5 * time.Hour)
	merged := func(n int, mergedAt, polled time.Time) PRView {
		t.Helper()
		v := prRow(t, n, PRStateMerged, "")
		v.MergedAt, v.LastPolledAt = mergedAt, polled
		return v
	}
	first, second := prTestEpoch, prTestEpoch.Add(30*time.Minute)
	assert.Equal(t, first, LiveWorkHoldStart([]PRView{merged(2, second, time.Time{}), merged(1, first, time.Time{})}, obs), "first merge, not the latest")
	assert.Equal(t, second, LiveWorkHoldStart([]PRView{merged(1, time.Time{}, second)}, obs), "zero merged_at falls back to last_polled_at")
	assert.Equal(t, obs, LiveWorkHoldStart([]PRView{merged(1, time.Time{}, time.Time{})}, obs), "no timestamps: first observation")
	open := prRow(t, 3, PRStateOpen, "")
	assert.Equal(t, obs, LiveWorkHoldStart([]PRView{open}, obs), "open PRs never start the clock")
}

func TestSplitLiveCurrentRound_ShouldSkipOnlyTheLiveNewestWorkSession_AndTreatErrorAsLive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	older := backlogdto.ItemSessionSummary{ID: "1", SessionUUID: "old", Role: SessionRoleWork, CreatedAt: prTestEpoch}
	newest := backlogdto.ItemSessionSummary{ID: "2", SessionUUID: "new", Role: SessionRoleWork, CreatedAt: prTestEpoch.Add(time.Hour)}
	review := backlogdto.ItemSessionSummary{ID: "3", SessionUUID: "rev", Role: SessionRoleReview, CreatedAt: prTestEpoch.Add(2 * time.Hour)}
	all := []backlogdto.ItemSessionSummary{older, newest, review}

	arch, skipped := SplitLiveCurrentRound(ctx, &fakeProbe{live: map[string]bool{"new": true}}, prTestEpoch, all)
	assert.True(t, skipped)
	assert.Equal(t, []backlogdto.ItemSessionSummary{older, review}, arch)

	arch, skipped = SplitLiveCurrentRound(ctx, &fakeProbe{err: errors.New("boom")}, prTestEpoch, all)
	assert.True(t, skipped, "a probe error counts as live")
	assert.Equal(t, []backlogdto.ItemSessionSummary{older, review}, arch)

	arch, skipped = SplitLiveCurrentRound(ctx, &fakeProbe{}, prTestEpoch, all)
	assert.False(t, skipped)
	assert.Equal(t, all, arch)

	arch, skipped = SplitLiveCurrentRound(ctx, nil, prTestEpoch, all)
	assert.False(t, skipped, "a nil probe skips nothing")
	assert.Equal(t, all, arch)
}

// fakeProbe answers live for the uuids in live, or err for everything.
type fakeProbe struct {
	live  map[string]bool
	err   error
	calls []string
}

func (f *fakeProbe) IsLive(_ context.Context, id string, _ time.Time) (bool, error) {
	f.calls = append(f.calls, id)
	if f.err != nil {
		return false, f.err
	}
	return f.live[id], nil
}
