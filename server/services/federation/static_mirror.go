package federation

import (
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

// NewStaticMirror returns a FederatedSessions that already mirrors sessions of
// one reachable peer and never dials. It lets callers outside this package
// (SessionService tests) exercise the mirrored-session paths without a network.
func NewStaticMirror(hostID string, sessions []*sessionv1.Session) *FederatedSessions {
	f := NewFederatedSessions(hostclaim.HostIdentity{}, nil)
	f.hosts[hostID] = &peerSnapshot{displayName: hostID, sessions: sessions, reachable: true}
	return f
}
