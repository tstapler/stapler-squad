package session

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/internal/fixturetest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

type fakeGateSource struct {
	views    map[string][]domain.PRView
	guards   map[string]ItemGuardView
	data     map[string]*domain.InstanceData
	viewsErr error
	guardErr error
	dataErr  error
}

func (f *fakeGateSource) ListSessionPRViewsByUUIDs(context.Context, []string) (map[string][]domain.PRView, error) {
	return f.views, f.viewsErr
}

func (f *fakeGateSource) ListItemGuardViewsByItemIDs(_ context.Context, ids []string) (map[string]ItemGuardView, error) {
	if f.guardErr != nil {
		return nil, f.guardErr
	}
	out := map[string]ItemGuardView{}
	for _, id := range ids {
		out[id] = f.guards[id] // missing item => zero view, present
	}
	return out, nil
}

func (f *fakeGateSource) FindInstanceDataByID(id string) (*domain.InstanceData, error) {
	if f.dataErr != nil {
		return nil, f.dataErr
	}
	if d, ok := f.data[id]; ok {
		return d, nil
	}
	return nil, ErrInstanceDataNotFound
}

func workSession(uuid, item string) backlogdto.ItemSessionSummary {
	return backlogdto.ItemSessionSummary{ID: uuid, SessionUUID: uuid, BacklogItemID: item, Role: instance.SessionRoleWork, CreatedAt: fixturetest.PrTestEpoch}
}

func gateWith(src *fakeGateSource, probe instance.LiveWorkProbe, live bool) *PRCleanupGate {
	return NewPRCleanupGate(src, probe, func() bool { return live })
}

func TestPRCleanupGate_Blocked_ShouldHoldOnEveryPRClause_AndNameTheOwner(t *testing.T) {
	t.Parallel()
	open := fixturetest.PrRow(t, 22, domain.PRStateOpen, "")
	merged := fixturetest.PrRow(t, 21, domain.PRStateMerged, "")
	unknown := fixturetest.PrRow(t, 23, domain.PRStateUnknown, "")
	tests := []struct {
		name       string
		src        *fakeGateSource
		wantReason string // substring; "" means not blocked
	}{
		{"all terminal", &fakeGateSource{views: map[string][]domain.PRView{"s1": {merged}}}, ""},
		{"no PRs at all", &fakeGateSource{}, ""},
		{"session link open", &fakeGateSource{views: map[string][]domain.PRView{"s1": {merged, open}}}, "session s1"},
		{"session link unknown", &fakeGateSource{views: map[string][]domain.PRView{"s1": {unknown}}}, "session s1"},
		{"item-direct open, no session link", &fakeGateSource{guards: map[string]ItemGuardView{"i1": {Direct: []domain.PRView{open}}}}, "item i1"},
		{"item-direct unknown", &fakeGateSource{guards: map[string]ItemGuardView{"i1": {Direct: []domain.PRView{unknown}}}}, "item i1"},
		{"item pr_number no link represents", &fakeGateSource{guards: map[string]ItemGuardView{"i1": {LegacyUnrepresented: true}}}, "item i1"},
		{"session legacy number, no link, flag not true", &fakeGateSource{data: map[string]*domain.InstanceData{"s1": {UUID: "s1", GitHubPRNumber: 5}}}, "session s1"},
		{"session legacy number, no link, flag true", &fakeGateSource{data: map[string]*domain.InstanceData{"s1": {UUID: "s1", GitHubPRNumber: 5, GitHubPRStatusTerminal: true}}}, ""},
		{"views lookup error", &fakeGateSource{viewsErr: errors.New("db")}, "unreadable"},
		{"guard lookup error", &fakeGateSource{guardErr: errors.New("db")}, "unreadable"},
		{"instance lookup error", &fakeGateSource{dataErr: errors.New("db")}, "unreadable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, blocked := gateWith(tt.src, nil, false).Blocked(context.Background(), []backlogdto.ItemSessionSummary{workSession("s1", "i1")}, GateOptions{})
			if tt.wantReason == "" {
				assert.False(t, blocked, b.Reason)
				return
			}
			require.True(t, blocked)
			assert.Contains(t, b.Reason, tt.wantReason)
			assert.False(t, b.LiveOnly, "a PR clause is not a live-only block")
		})
	}
}

func TestPRCleanupGate_Blocked_ShouldIgnoreNonWorkSessions_AndSupersededRoundStillHolds(t *testing.T) {
	t.Parallel()
	src := &fakeGateSource{views: map[string][]domain.PRView{"s1": {fixturetest.PrRow(t, 30, domain.PRStateOpen, "")}}}
	review := backlogdto.ItemSessionSummary{ID: "r", SessionUUID: "s1", BacklogItemID: "i1", Role: instance.SessionRoleReview}
	_, blocked := gateWith(src, nil, false).Blocked(context.Background(), []backlogdto.ItemSessionSummary{review}, GateOptions{})
	assert.False(t, blocked, "only work-role links count")

	b, blocked := gateWith(src, nil, false).Blocked(context.Background(), []backlogdto.ItemSessionSummary{workSession("s1", "i1")}, GateOptions{})
	require.True(t, blocked, "round 1's open #30 holds s1 even though the item verdict ignores it")
	assert.Contains(t, b.Reason, "session s1")
	src.views = map[string][]domain.PRView{}
	_, blocked = gateWith(src, nil, false).Blocked(context.Background(), []backlogdto.ItemSessionSummary{workSession("s1", "i1")}, GateOptions{})
	assert.False(t, blocked, "after DeleteSessionPR the next attempt is clear")
}

func TestPRCleanupGate_Blocked_ShouldHoldWhileCurrentRoundSessionLive_OnlyWithScopeAndProbe(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	sessions := []backlogdto.ItemSessionSummary{workSession("s1", "i1")}
	b, blocked := gateWith(&fakeGateSource{}, &fakeProbe{live: map[string]bool{"s1": true}}, true).Blocked(ctx, sessions, GateOptions{})
	require.True(t, blocked)
	assert.True(t, b.LiveOnly)
	assert.Contains(t, b.Reason, "session s1")

	_, blocked = gateWith(&fakeGateSource{}, &fakeProbe{live: map[string]bool{"s1": false}}, true).Blocked(ctx, sessions, GateOptions{})
	assert.False(t, blocked, "idle or ended: released")
	b, blocked = gateWith(&fakeGateSource{}, &fakeProbe{err: errors.New("boom")}, true).Blocked(ctx, sessions, GateOptions{})
	assert.True(t, blocked, "a probe error blocks")
	assert.True(t, b.LiveOnly)

	live := &fakeProbe{live: map[string]bool{"s1": true}}
	_, blocked = gateWith(&fakeGateSource{}, live, false).Blocked(ctx, sessions, GateOptions{})
	assert.False(t, blocked, "scope off: the live clause is off")
	assert.Empty(t, live.calls, "scope off: the probe is not even consulted")
	_, blocked = gateWith(&fakeGateSource{}, nil, true).Blocked(ctx, sessions, GateOptions{})
	assert.False(t, blocked, "nil probe: today's behavior")
}

func TestPRCleanupGate_Blocked_ShouldBlockOnPRClausesOnlyOnReworkPath_WhenJustSpawnedSessionIsLive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	prior := []backlogdto.ItemSessionSummary{workSession("old", "i1")}
	probe := &fakeProbe{live: map[string]bool{"old": true, "fresh": true}}

	_, blocked := gateWith(&fakeGateSource{}, probe, true).Blocked(ctx, prior, GateOptions{IgnoreLive: true})
	assert.False(t, blocked, "rework: a live session never blocks purely for being live")
	assert.Empty(t, probe.calls)

	src := &fakeGateSource{views: map[string][]domain.PRView{"old": {fixturetest.PrRow(t, 30, domain.PRStateOpen, "")}}}
	b, blocked := gateWith(src, probe, true).Blocked(ctx, prior, GateOptions{IgnoreLive: true})
	require.True(t, blocked, "rework: a prior round's open PR still holds its worktree")
	assert.False(t, b.LiveOnly)
}

func TestPRCleanupGate_Blocked_ShouldReportPRBlockNotLive_WhenBothHold(t *testing.T) {
	t.Parallel()
	src := &fakeGateSource{views: map[string][]domain.PRView{"s1": {fixturetest.PrRow(t, 22, domain.PRStateOpen, "")}}}
	b, blocked := gateWith(src, &fakeProbe{live: map[string]bool{"s1": true}}, true).Blocked(context.Background(), []backlogdto.ItemSessionSummary{workSession("s1", "i1")}, GateOptions{})
	require.True(t, blocked)
	assert.False(t, b.LiveOnly, "a PR hold settles for the TTL; only a pure live hold stays unsettled")
}

func TestBlockedSet_ShouldDedupeFirstBlock_AndReportCountAndAge(t *testing.T) {
	t.Parallel()
	b := NewBlockedSet()
	t0 := fixturetest.PrTestEpoch
	assert.True(t, b.Block("sweep", "i1", "r", t0))
	assert.False(t, b.Block("sweep", "i1", "r", t0.Add(time.Minute)), "second tick: counter unchanged")
	assert.True(t, b.Block("archive_rpc", "i1", "r", t0.Add(2*time.Minute)), "a different path counts separately")
	assert.Equal(t, 1, b.Count("sweep"))
	items := b.Items()
	require.Len(t, items, 1)
	assert.Equal(t, t0, items[0].Since)
	b.Unblock("i1")
	assert.Zero(t, b.Count("sweep"))
	assert.Empty(t, b.Items())
}
