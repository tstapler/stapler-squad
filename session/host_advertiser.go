package session

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// AdvertisementEndpointPath is the HTTP path the gossip advertisement
// endpoint is served on. Shared between the client side (this file) and the
// server-side handler (server/auth/host_advertisement.go) so the two can't
// drift out of sync.
//
// Per ADR-002 and plan.md Story 3.2 Task 1: this is served on the existing
// --remote-port HTTPS server (main.go's startRemoteAccess, registered via
// server/auth.RegisterRoutes's shared mux) -- confirmed as the only
// cross-host-reachable HTTP surface in this codebase. It is deliberately
// NOT a new listener and NOT piggybacked on WorkspacePeer (which is
// local-only: DB + `tmux list-sessions`, see session/workspace_peers.go).
const AdvertisementEndpointPath = "/internal/host-advertisement"

// HostAdvertiser is the client side of the gossip-style advertisement
// exchange described in ADR-002: it periodically POSTs this instance's own
// signed AdvertisementRecord to every peer currently known to HostRegistry,
// and performs the bounded one-hop re-gossip of records it learns about for
// the first time.
//
// Fan-out is bounded by construction rather than an explicit hop counter:
// HostRegistry.Advertise returns isNew=false for a repeat advertisement of
// an already-known HostIdentity, so a caller (the HTTP handler in
// server/auth/host_advertisement.go) only ever invokes ReGossip once per
// newly learned identity per receiving instance -- there is no recursive
// unbounded flood, just a single extra hop per new fact.
type HostAdvertiser struct {
	identity  HostIdentity
	registry  *HostRegistry
	addresses []string
	client    *http.Client
	interval  time.Duration
}

// NewHostAdvertiser constructs a HostAdvertiser that broadcasts addresses as
// this instance's own reachable AdvertisedAddress[] every interval.
func NewHostAdvertiser(identity HostIdentity, registry *HostRegistry, addresses []string, interval time.Duration) *HostAdvertiser {
	return &HostAdvertiser{
		identity:  identity,
		registry:  registry,
		addresses: addresses,
		// TLSClientConfig.InsecureSkipVerify: every peer's --remote-port
		// server (server/server.go's StartRemote) always serves TLS with a
		// locally-minted self-signed CA (server/tls.go) -- there is no
		// shared CA between independently-provisioned hosts for per-
		// connection certificate verification to succeed against. Per
		// ADR-002, peer identity is instead authenticated at the
		// application layer: HostRegistry.Advertise only accepts a record
		// whose Ed25519 signature verifies against a TOFU-pinned public
		// key. TLS here provides transport encryption only.
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // see comment above
		},
		interval: interval,
	}
}

// Run blocks, broadcasting this instance's advertisement to every known peer
// every interval, until ctx is cancelled. Callers should invoke this via
// `go advertiser.Run(ctx)` and cancel ctx during shutdown, mirroring the
// graceful-shutdown handling of other background loops in this package
// (e.g. SetupManager.WatchFile).
func (a *HostAdvertiser) Run(ctx context.Context) {
	ticker := time.NewTicker(a.interval)
	defer ticker.Stop()
	for {
		a.BroadcastOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// BroadcastOnce sends this instance's current advertisement to every peer
// presently known to the registry. Best-effort: an unreachable peer is
// skipped rather than aborting the whole round.
func (a *HostAdvertiser) BroadcastOnce(ctx context.Context) {
	record := BuildAdvertisement(a.identity, a.addresses, time.Now())
	for _, peer := range a.registry.Snapshot() {
		for _, addr := range peer.AdvertisedAddress {
			_ = a.SendAdvertisement(ctx, addr, record)
		}
	}
}

// SendAdvertisement POSTs record to addr's advertisement endpoint and, on
// success, upserts the responder's own self-advertisement (returned as the
// reply body) into the local registry.
func (a *HostAdvertiser) SendAdvertisement(ctx context.Context, addr string, record AdvertisementRecord) error {
	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal advertisement: %w", err)
	}
	// addr is a bare "host:port" (see main.go's selfAddresses, per ADR-002's
	// AdvertisedAddress format) -- it needs an explicit scheme before it's a
	// valid absolute URL; without one, http.NewRequestWithContext fails to
	// parse it ("first path segment in URL cannot contain colon").
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+addr+AdvertisementEndpointPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build advertisement request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send advertisement to %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("advertisement to %s rejected: status %d", addr, resp.StatusCode)
	}

	var reply AdvertisementRecord
	if err := json.NewDecoder(resp.Body).Decode(&reply); err != nil {
		return fmt.Errorf("failed to decode advertisement reply from %s: %w", addr, err)
	}
	if _, _, err := a.registry.Advertise(reply); err != nil {
		return fmt.Errorf("failed to record advertisement reply from %s: %w", addr, err)
	}
	return nil
}

// ReGossip fans record out to every peer this instance currently knows
// about, except record's own HostIdentity and any address in exclude
// (typically the sender's own AdvertisedAddress[], so a receiving instance
// doesn't immediately bounce the record straight back to where it came
// from). See the type doc comment for why this is safe from unbounded
// flooding without an explicit hop counter.
func (a *HostAdvertiser) ReGossip(ctx context.Context, record AdvertisementRecord, exclude map[string]bool) {
	for _, peer := range a.registry.Snapshot() {
		if peer.HostID.String() == record.HostIdentity.String() {
			continue
		}
		for _, addr := range peer.AdvertisedAddress {
			if exclude[addr] {
				continue
			}
			_ = a.SendAdvertisement(ctx, addr, record)
		}
	}
}
