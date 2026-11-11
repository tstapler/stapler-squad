package hostclaim

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SessionFederationEndpointPath serves the calling peer a read-only snapshot of
// this host's own sessions. Beside the gossip endpoints on the --remote-port
// server and exempt from the passkey wall like them; each request carries an
// Ed25519 signature from an enrolled host (SignPeerRequest).
const SessionFederationEndpointPath = "/internal/session-federation/sessions"

// SessionFederationTerminalPath streams one session's pane to an enrolled peer,
// read-only: the peer receives snapshots and can send nothing back.
const SessionFederationTerminalPath = "/internal/session-federation/terminal"

// FederatedSessionIDSeparator joins the owning host and the session ID. HostID
// is "host_<ULID>", which never contains it, so the first separator splits.
const FederatedSessionIDSeparator = "/"

// FederatedSessionID names a session across hosts. Local sessions keep their
// bare ID, so every existing ID parses as local and nothing needs migrating.
type FederatedSessionID struct {
	HostID    string // empty for a session on this instance
	SessionID string
}

// ParseFederatedSessionID splits "host_<ULID>/<id>"; anything else is local.
// Session IDs are titles and may contain "/", so only a leading host-shaped
// segment makes an ID remote.
func ParseFederatedSessionID(s string) FederatedSessionID {
	if host, rest, ok := strings.Cut(s, FederatedSessionIDSeparator); ok && rest != "" && isValidHostID(host) {
		return FederatedSessionID{HostID: host, SessionID: rest}
	}
	return FederatedSessionID{SessionID: s}
}

func isValidHostID(s string) bool {
	_, err := ParseHostID(s)
	return err == nil
}

func (id FederatedSessionID) IsLocal() bool { return id.HostID == "" }

func (id FederatedSessionID) String() string {
	if id.IsLocal() {
		return id.SessionID
	}
	return id.HostID + FederatedSessionIDSeparator + id.SessionID
}

const (
	// peerRequestMaxSkew bounds replay of a captured request. The response is
	// read-only session data the signer could already read.
	peerRequestMaxSkew = 30 * time.Second
)

func peerRequestPayload(method, path, timestamp string) []byte {
	return []byte("peer-request\n" + method + "\n" + path + "\n" + timestamp)
}

// SignPeerRequest sets the signed-request headers (the same three the claim
// lookup uses) on h for method+path at time at.
func SignPeerRequest(h http.Header, identity HostIdentity, method, path string, at time.Time) {
	ts := at.UTC().Format(time.RFC3339)
	h.Set(ClaimLookupHostHeader, identity.ID.String())
	h.Set(ClaimLookupTimestampHeader, ts)
	h.Set(ClaimLookupSignatureHeader, base64.StdEncoding.EncodeToString(identity.Sign(peerRequestPayload(method, path, ts))))
}

// AuthorizePeerRequest returns the enrolled host that signed h for method+path
// within peerRequestMaxSkew of now. Like claim lookup, this proves possession
// of an enrolled key, not operator approval: enrolment is open to anyone who
// can reach the port (ADR-002's same-LAN threat model).
func (r *HostRegistry) AuthorizePeerRequest(h http.Header, method, path string, now time.Time) (HostID, error) {
	hostID, err := ParseHostID(h.Get(ClaimLookupHostHeader))
	if err != nil {
		return HostID{}, fmt.Errorf("missing or malformed host id")
	}
	ts := h.Get(ClaimLookupTimestampHeader)
	at, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return HostID{}, fmt.Errorf("malformed timestamp")
	}
	if skew := now.Sub(at); skew > peerRequestMaxSkew || skew < -peerRequestMaxSkew {
		return HostID{}, fmt.Errorf("timestamp outside replay window")
	}
	sig, err := base64.StdEncoding.DecodeString(h.Get(ClaimLookupSignatureHeader))
	if err != nil {
		return HostID{}, fmt.Errorf("malformed signature")
	}
	pinned, ok := r.Lookup(hostID)
	if !ok {
		return HostID{}, fmt.Errorf("host not enrolled")
	}
	if !VerifyAdvertisement(pinned.PublicKey, peerRequestPayload(method, path, ts), sig) {
		return HostID{}, fmt.Errorf("bad signature")
	}
	return hostID, nil
}
