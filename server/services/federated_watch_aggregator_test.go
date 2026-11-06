package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func fedSess(id, title string) *sessionv1.Session {
	return &sessionv1.Session{Id: id, Title: title}
}

func drainSessionEvents(ch <-chan *sessionv1.SessionEvent) []*sessionv1.SessionEvent {
	var out []*sessionv1.SessionEvent
	for {
		select {
		case e := <-ch:
			out = append(out, e)
		default:
			return out
		}
	}
}

func TestFederatedWatchAggregator_DiffsPerPeer(t *testing.T) {
	a := NewFederatedWatchAggregator()
	ch, cancel := a.Subscribe()
	defer cancel()

	a.Observe("B", []*sessionv1.Session{fedSess("B/1", "one"), fedSess("B/2", "two")})
	evs := drainSessionEvents(ch)
	require.Len(t, evs, 2)
	assert.NotNil(t, evs[0].GetSessionCreated())

	a.Observe("B", []*sessionv1.Session{fedSess("B/1", "one-renamed")})
	evs = drainSessionEvents(ch)
	require.Len(t, evs, 2)
	assert.Equal(t, "one-renamed", evs[0].GetSessionUpdated().Session.Title)
	assert.Equal(t, "B/2", evs[1].GetSessionDeleted().SessionId)

	a.Observe("B", []*sessionv1.Session{fedSess("B/1", "one-renamed")})
	assert.Empty(t, drainSessionEvents(ch), "unchanged snapshot emits nothing")
}

func TestFederatedWatchAggregator_CursorsAreIndependentAndNotExposed(t *testing.T) {
	a := NewFederatedWatchAggregator()
	ch, cancel := a.Subscribe()
	defer cancel()

	a.Observe("B", []*sessionv1.Session{fedSess("B/1", "x")})
	a.Observe("B", []*sessionv1.Session{fedSess("B/1", "y")})
	a.Observe("C", []*sessionv1.Session{fedSess("C/1", "z")})

	assert.Equal(t, uint64(2), a.PeerSeq("B"))
	assert.Equal(t, uint64(1), a.PeerSeq("C"))
	for _, e := range drainSessionEvents(ch) {
		assert.Zero(t, e.Seq, "peer cursors must never leak into the client-visible seq")
	}
}

func TestFederatedWatchAggregator_ForgetDeletesPeerSessions(t *testing.T) {
	a := NewFederatedWatchAggregator()
	ch, cancel := a.Subscribe()
	defer cancel()
	a.Observe("B", []*sessionv1.Session{fedSess("B/1", "x")})
	drainSessionEvents(ch)

	a.Forget("B")
	evs := drainSessionEvents(ch)
	require.Len(t, evs, 1)
	assert.Equal(t, "B/1", evs[0].GetSessionDeleted().SessionId)
	assert.Zero(t, a.PeerSeq("B"))
}

func TestFederatedWatchAggregator_SlowSubscriberDoesNotBlock(t *testing.T) {
	a := NewFederatedWatchAggregator()
	_, cancel := a.Subscribe()
	defer cancel()
	var many []*sessionv1.Session
	for i := 0; i < federatedWatchBuffer+10; i++ {
		many = append(many, fedSess("B/"+string(rune('a'+i%26))+string(rune('a'+i/26)), "t"))
	}
	a.Observe("B", many) // would deadlock if publish blocked
}
