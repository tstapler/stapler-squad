package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// maxFederationResponseBody bounds what one peer can make us buffer.
	maxFederationResponseBody = 32 << 20
	// peerFetchTimeout keeps one dead peer from stalling a refresh round.
	peerFetchTimeout = 3 * time.Second
	// DefaultFederationRefreshInterval is the bound on how long a session created
	// on a peer takes to appear here.
	DefaultFederationRefreshInterval = 10 * time.Second
)

// FederationPeers is the slice of HostRegistry the federation cache needs.
type FederationPeers interface {
	LiveSnapshot() []session.RegistryEntry
	AuthorizePeerRequest(h http.Header, method, path string, now time.Time) (session.HostID, error)
}

type peerSnapshot struct {
	displayName string
	sessions    []*sessionv1.Session
	reachable   bool
}

// FederatedSessions mirrors the sessions of peer instances read-only. Each
// refresh round replaces a peer's snapshot; an unreachable peer keeps its last
// snapshot, flagged host_stale, so the merged list degrades instead of blocking.
type FederatedSessions struct {
	identity session.HostIdentity
	peers    FederationPeers
	client   *http.Client
	// streamClient has no overall timeout (a terminal stream is long-lived); only
	// the wait for response headers is bounded.
	streamClient *http.Client
	now          func() time.Time

	watch *FederatedWatchAggregator

	mu    sync.RWMutex
	hosts map[string]*peerSnapshot
}

func NewFederatedSessions(identity session.HostIdentity, peers FederationPeers) *FederatedSessions {
	streamTransport := federationTransport()
	streamTransport.ResponseHeaderTimeout = peerFetchTimeout
	return &FederatedSessions{
		streamClient: &http.Client{
			Transport:     streamTransport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		identity: identity,
		peers:    peers,
		client: &http.Client{
			Transport: federationTransport(),
			// A peer must not bounce a signed request to an internal address.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		now:   time.Now,
		watch: NewFederatedWatchAggregator(),
		hosts: map[string]*peerSnapshot{},
	}
}

// Run refreshes every interval until ctx is done.
func (f *FederatedSessions) Run(ctx context.Context, interval time.Duration) {
	f.Refresh(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.Refresh(ctx)
		}
	}
}

// Refresh polls every live peer concurrently once.
func (f *FederatedSessions) Refresh(ctx context.Context) {
	live := f.peers.LiveSnapshot()
	liveIDs := make(map[string]struct{}, len(live))
	var wg sync.WaitGroup
	for _, peer := range live {
		if peer.HostID == f.identity.ID {
			continue
		}
		liveIDs[peer.HostID.String()] = struct{}{}
		wg.Add(1)
		go func(peer session.RegistryEntry) {
			defer wg.Done()
			f.refreshPeer(ctx, peer)
		}(peer)
	}
	wg.Wait()

	// A peer pruned from the registry has left the federation: drop its sessions.
	f.mu.Lock()
	var gone []string
	for id := range f.hosts {
		if _, ok := liveIDs[id]; !ok {
			delete(f.hosts, id)
			gone = append(gone, id)
		}
	}
	f.mu.Unlock()
	for _, id := range gone {
		f.watch.Forget(id)
	}
}

func (f *FederatedSessions) refreshPeer(ctx context.Context, peer session.RegistryEntry) {
	id := peer.HostID.String()
	var lastErr error
	for _, addr := range peer.AdvertisedAddress {
		sessions, err := f.fetch(ctx, addr)
		if err != nil {
			lastErr = err
			continue
		}
		f.setSnapshot(id, &peerSnapshot{displayName: displayNameFor(peer), sessions: sessions, reachable: true})
		return
	}
	log.Debug("session_federation.peer_unreachable", "peer", id, "err", lastErr)
	f.mu.RLock()
	prev, ok := f.hosts[id]
	f.mu.RUnlock()
	stale := &peerSnapshot{displayName: displayNameFor(peer)}
	if ok {
		stale = &peerSnapshot{displayName: prev.displayName, sessions: prev.sessions}
	}
	f.setSnapshot(id, stale)
}

// setSnapshot stores a peer's snapshot and publishes the diff to watchers.
func (f *FederatedSessions) setSnapshot(hostID string, snap *peerSnapshot) {
	f.mu.Lock()
	f.hosts[hostID] = snap
	f.mu.Unlock()
	f.watch.Observe(hostID, tagPeerSessions(hostID, snap))
}

// Watch exposes peer session change events for WatchSessions.
func (f *FederatedSessions) Watch() *FederatedWatchAggregator { return f.watch }

func displayNameFor(peer session.RegistryEntry) string {
	if len(peer.AdvertisedAddress) > 0 {
		return peer.AdvertisedAddress[0]
	}
	return peer.HostID.String()
}

func (f *FederatedSessions) fetch(ctx context.Context, addr string) ([]*sessionv1.Session, error) {
	ctx, cancel := context.WithTimeout(ctx, peerFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+session.SessionFederationEndpointPath, nil)
	if err != nil {
		return nil, err
	}
	session.SignPeerRequest(req.Header, f.identity, http.MethodGet, session.SessionFederationEndpointPath, f.now())
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("session federation at %s: status %d", addr, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFederationResponseBody))
	if err != nil {
		return nil, err
	}
	var out sessionv1.ListSessionsResponse
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode sessions from %s: %w", addr, err)
	}
	return out.Sessions, nil
}

// Sessions returns the mirrored sessions, tagged with their owning host and
// given host-qualified IDs so they can never alias a local session.
func (f *FederatedSessions) Sessions() []*sessionv1.Session {
	f.mu.RLock()
	defer f.mu.RUnlock()
	hostIDs := make([]string, 0, len(f.hosts))
	for id := range f.hosts {
		hostIDs = append(hostIDs, id)
	}
	sort.Strings(hostIDs)
	out := make([]*sessionv1.Session, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		out = append(out, tagPeerSessions(hostID, f.hosts[hostID])...)
	}
	return out
}

func tagPeerSessions(hostID string, snap *peerSnapshot) []*sessionv1.Session {
	out := make([]*sessionv1.Session, 0, len(snap.sessions))
	for _, s := range snap.sessions {
		tagged := proto.Clone(s).(*sessionv1.Session)
		tagged.Id = session.FederatedSessionID{HostID: hostID, SessionID: s.Id}.String()
		tagged.HostId = hostID
		tagged.HostDisplayName = snap.displayName
		tagged.HostStale = !snap.reachable
		out = append(out, tagged)
	}
	return out
}

// SessionFederationHandler serves this host's own sessions to an enrolled peer.
// list must return local sessions only, never the merged view, or two peers
// would echo each other's sessions back and forth.
func SessionFederationHandler(peers FederationPeers, list func(context.Context) ([]*sessionv1.Session, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := peers.AuthorizePeerRequest(r.Header, r.Method, r.URL.Path, time.Now()); err != nil {
			log.Debug("session_federation.unauthorized", "err", err)
			http.Error(w, "session federation requires a signed request from an enrolled host", http.StatusUnauthorized)
			return
		}
		sessions, err := list(r.Context())
		if err != nil {
			log.Warn("session_federation.list_failed", "err", err)
			http.Error(w, "failed to list sessions", http.StatusInternalServerError)
			return
		}
		body, err := protojson.Marshal(&sessionv1.ListSessionsResponse{Sessions: sessions})
		if err != nil {
			http.Error(w, "failed to encode sessions", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

// filtered applies the ListSessions request filters peers' sessions can be
// checked against from the proto alone.
func (f *FederatedSessions) filtered(req *sessionv1.ListSessionsRequest) []*sessionv1.Session {
	var out []*sessionv1.Session
	for _, s := range f.Sessions() {
		if req.Status != nil && *req.Status != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED && s.Status != *req.Status {
			continue
		}
		if req.Category != nil && *req.Category != "" && s.Category != *req.Category {
			continue
		}
		if req.WorkflowId != nil && *req.WorkflowId != "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// Session returns one mirrored session by its host-qualified ID.
func (f *FederatedSessions) Session(id string) (*sessionv1.Session, bool) {
	for _, s := range f.Sessions() {
		if s.Id == id {
			return s, true
		}
	}
	return nil, false
}

// filteredByWatch applies WatchSessions' category/status filters to the mirror.
func (f *FederatedSessions) filteredByWatch(req *sessionv1.WatchSessionsRequest) []*sessionv1.Session {
	var out []*sessionv1.Session
	for _, s := range f.Sessions() {
		if sessionMatchesWatch(s, req) {
			out = append(out, s)
		}
	}
	return out
}

func sessionMatchesWatch(s *sessionv1.Session, req *sessionv1.WatchSessionsRequest) bool {
	if s == nil || s.Hidden {
		return false
	}
	if req.CategoryFilter != nil && *req.CategoryFilter != "" && s.Category != *req.CategoryFilter {
		return false
	}
	if req.StatusFilter != nil && *req.StatusFilter != sessionv1.SessionStatus_SESSION_STATUS_UNSPECIFIED && s.Status != *req.StatusFilter {
		return false
	}
	return true
}

// peerEventMatches filters a peer event; deletions always pass so a session that
// stopped matching (or vanished) is removed client-side.
func peerEventMatches(e *sessionv1.SessionEvent, req *sessionv1.WatchSessionsRequest) bool {
	switch ev := e.Event.(type) {
	case *sessionv1.SessionEvent_SessionCreated:
		return sessionMatchesWatch(ev.SessionCreated.Session, req)
	case *sessionv1.SessionEvent_SessionUpdated:
		return sessionMatchesWatch(ev.SessionUpdated.Session, req)
	}
	return true
}
