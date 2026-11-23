package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func filterFixture() *FederatedSessions {
	f := &FederatedSessions{hosts: map[string]*peerSnapshot{}, watch: NewFederatedWatchAggregator()}
	f.hosts["h1"] = &peerSnapshot{reachable: true, sessions: []*sessionv1.Session{
		{Id: "a", Category: "x", Status: sessionv1.SessionStatus_SESSION_STATUS_RUNNING},
		{Id: "b", Category: "y", Status: sessionv1.SessionStatus_SESSION_STATUS_PAUSED},
		{Id: "c", Category: "x", Status: sessionv1.SessionStatus_SESSION_STATUS_RUNNING, Hidden: true},
	}}
	return f
}

func strp(s string) *string { return &s }

func TestFederatedSessions_Filtered_should_ApplyListFilters(t *testing.T) {
	t.Parallel()
	f := filterFixture()
	running := sessionv1.SessionStatus_SESSION_STATUS_RUNNING
	unspecified := sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED

	assert.Len(t, f.Filtered(&sessionv1.ListSessionsRequest{}), 3)
	assert.Len(t, f.Filtered(&sessionv1.ListSessionsRequest{Status: &unspecified, Category: strp("")}), 3)
	assert.Len(t, f.Filtered(&sessionv1.ListSessionsRequest{Status: &running}), 2)
	assert.Len(t, f.Filtered(&sessionv1.ListSessionsRequest{Category: strp("y")}), 1)
	assert.Empty(t, f.Filtered(&sessionv1.ListSessionsRequest{WorkflowId: strp("wf")}), "a workflow filter never matches mirrored sessions")
}

func TestFederatedSessions_FilteredByWatch_should_DropHiddenAndMismatched(t *testing.T) {
	t.Parallel()
	f := filterFixture()
	paused := sessionv1.SessionStatus_SESSION_STATUS_PAUSED

	assert.Len(t, f.FilteredByWatch(&sessionv1.WatchSessionsRequest{}), 2, "hidden session dropped")
	assert.Len(t, f.FilteredByWatch(&sessionv1.WatchSessionsRequest{CategoryFilter: strp("x")}), 1)
	got := f.FilteredByWatch(&sessionv1.WatchSessionsRequest{StatusFilter: &paused})
	require.Len(t, got, 1)
	assert.Equal(t, "h1/b", got[0].Id)
}

func TestPeerEventMatches_should_FilterCreatedUpdatedAndPassOthers(t *testing.T) {
	t.Parallel()
	req := &sessionv1.WatchSessionsRequest{CategoryFilter: strp("x")}
	match, miss := &sessionv1.Session{Category: "x"}, &sessionv1.Session{Category: "y"}

	created := func(s *sessionv1.Session) *sessionv1.SessionEvent {
		return &sessionv1.SessionEvent{Event: &sessionv1.SessionEvent_SessionCreated{SessionCreated: &sessionv1.SessionCreatedEvent{Session: s}}}
	}
	updated := func(s *sessionv1.Session) *sessionv1.SessionEvent {
		return &sessionv1.SessionEvent{Event: &sessionv1.SessionEvent_SessionUpdated{SessionUpdated: &sessionv1.SessionUpdatedEvent{Session: s}}}
	}
	assert.True(t, PeerEventMatches(created(match), req))
	assert.False(t, PeerEventMatches(created(miss), req))
	assert.True(t, PeerEventMatches(updated(match), req))
	assert.False(t, PeerEventMatches(updated(miss), req))
	deleted := &sessionv1.SessionEvent{Event: &sessionv1.SessionEvent_SessionDeleted{SessionDeleted: &sessionv1.SessionDeletedEvent{}}}
	assert.True(t, PeerEventMatches(deleted, req), "deletions always pass")
}
