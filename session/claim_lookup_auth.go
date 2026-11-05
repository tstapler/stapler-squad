package session

import (
	"encoding/base64"
	"net/http"
	"time"
)

// Headers carrying a signed claim-lookup request. The GET endpoint is exempt
// from the passkey wall (peers have no session), so the caller proves it is an
// enrolled host instead: an Ed25519 signature over the URL and a timestamp,
// verified against the key HostRegistry pinned for the claimed host ID.
// Limit: enrolment (/internal/host-advertisement) is itself open to anyone who
// can reach the port, so this proves possession of an enrolled key, not
// operator approval; the claim map is only as private as that port.
const (
	ClaimLookupHostHeader      = "X-SSQ-Host-ID"
	ClaimLookupTimestampHeader = "X-SSQ-Timestamp"
	ClaimLookupSignatureHeader = "X-SSQ-Signature"

	// claimLookupMaxSkew bounds replay of a captured request. The response is
	// read-only claim data, so a replay inside the window discloses nothing the
	// original signer was not already allowed to see.
	claimLookupMaxSkew = 5 * time.Minute
)

func claimLookupPayload(externalURL, timestamp string) []byte {
	return []byte("claim-lookup\n" + NormalizeClaimURL(externalURL) + "\n" + timestamp)
}

// SignClaimLookup sets the signed-request headers on h for a lookup of externalURL at time at.
func SignClaimLookup(h http.Header, identity HostIdentity, externalURL string, at time.Time) {
	ts := at.UTC().Format(time.RFC3339)
	h.Set(ClaimLookupHostHeader, identity.ID.String())
	h.Set(ClaimLookupTimestampHeader, ts)
	h.Set(ClaimLookupSignatureHeader, base64.StdEncoding.EncodeToString(identity.Sign(claimLookupPayload(externalURL, ts))))
}

// AuthorizeClaimLookup reports whether h carries a fresh signature over
// externalURL from a host present in HostRegistry under its pinned key.
func (c *ClaimIndex) AuthorizeClaimLookup(h http.Header, externalURL string) bool {
	hostID, err := ParseHostID(h.Get(ClaimLookupHostHeader))
	if err != nil {
		return false
	}
	ts := h.Get(ClaimLookupTimestampHeader)
	at, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	if skew := c.now().Sub(at); skew > claimLookupMaxSkew || skew < -claimLookupMaxSkew {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(h.Get(ClaimLookupSignatureHeader))
	if err != nil {
		return false
	}
	pinned, ok := c.hostRegistry.Lookup(hostID)
	if !ok {
		return false
	}
	return VerifyAdvertisement(pinned.PublicKey, claimLookupPayload(externalURL, ts), sig)
}
