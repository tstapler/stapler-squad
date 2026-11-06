package services

import (
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// federatedWatchBuffer bounds how far a slow WatchSessions subscriber may lag
// before peer events are dropped for it (it recovers on its next snapshot).
const federatedWatchBuffer = 256

// peerCursor is one peer's independent replay position and last-known view.
type peerCursor struct {
	seq   uint64
	known map[string]*sessionv1.Session
}

// FederatedWatchAggregator turns successive peer snapshots into per-session
// created/updated/deleted events for WatchSessions. Each peer has its own
// sequence cursor, so one peer's gaps or restarts never disturb another's. Those
// cursors are internal: events leave with Seq 0, so the only sequence a client
// ever sees is the local EventBus counter (clients take the max, so 0 is inert).
type FederatedWatchAggregator struct {
	mu      sync.Mutex
	peers   map[string]*peerCursor
	subs    map[int]chan *sessionv1.SessionEvent
	nextSub int
	now     func() time.Time
}

func NewFederatedWatchAggregator() *FederatedWatchAggregator {
	return &FederatedWatchAggregator{
		peers: map[string]*peerCursor{},
		subs:  map[int]chan *sessionv1.SessionEvent{},
		now:   time.Now,
	}
}

// Subscribe returns a channel of peer session events and a cancel func.
func (a *FederatedWatchAggregator) Subscribe() (<-chan *sessionv1.SessionEvent, func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := a.nextSub
	a.nextSub++
	ch := make(chan *sessionv1.SessionEvent, federatedWatchBuffer)
	a.subs[id] = ch
	return ch, func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		if _, ok := a.subs[id]; ok {
			delete(a.subs, id)
			close(ch)
		}
	}
}

// PeerSeq reports how many snapshots of hostID have been observed.
func (a *FederatedWatchAggregator) PeerSeq(hostID string) uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c, ok := a.peers[hostID]; ok {
		return c.seq
	}
	return 0
}

// Observe diffs hostID's latest (already host-tagged) sessions against the
// previous snapshot, advances that peer's cursor and publishes the changes.
func (a *FederatedWatchAggregator) Observe(hostID string, sessions []*sessionv1.Session) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cur, ok := a.peers[hostID]
	if !ok {
		cur = &peerCursor{known: map[string]*sessionv1.Session{}}
		a.peers[hostID] = cur
	}
	cur.seq++
	next := make(map[string]*sessionv1.Session, len(sessions))
	for _, s := range sessions {
		next[s.Id] = s
		prev, existed := cur.known[s.Id]
		switch {
		case !existed:
			a.publishLocked(&sessionv1.SessionEvent{Event: &sessionv1.SessionEvent_SessionCreated{
				SessionCreated: &sessionv1.SessionCreatedEvent{Session: s}}})
		case !proto.Equal(prev, s):
			a.publishLocked(&sessionv1.SessionEvent{Event: &sessionv1.SessionEvent_SessionUpdated{
				SessionUpdated: &sessionv1.SessionUpdatedEvent{Session: s}}})
		}
	}
	for id := range cur.known {
		if _, still := next[id]; !still {
			a.publishDeletedLocked(id)
		}
	}
	cur.known = next
}

// Forget drops a peer that left the federation, deleting its sessions.
func (a *FederatedWatchAggregator) Forget(hostID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	cur, ok := a.peers[hostID]
	if !ok {
		return
	}
	for id := range cur.known {
		a.publishDeletedLocked(id)
	}
	delete(a.peers, hostID)
}

func (a *FederatedWatchAggregator) publishDeletedLocked(id string) {
	a.publishLocked(&sessionv1.SessionEvent{Event: &sessionv1.SessionEvent_SessionDeleted{
		SessionDeleted: &sessionv1.SessionDeletedEvent{SessionId: id, Reason: "peer"}}})
}

func (a *FederatedWatchAggregator) publishLocked(e *sessionv1.SessionEvent) {
	e.Timestamp = timestamppb.New(a.now())
	for _, ch := range a.subs {
		select {
		case ch <- e:
		default: // slow subscriber: drop rather than block a refresh round
		}
	}
}
