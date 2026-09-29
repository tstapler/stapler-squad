package session

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ClaimAdvertisementEndpointPath is the HTTP path claim gossip is served on,
// beside AdvertisementEndpointPath on the --remote-port server. Shared by the
// client (this file) and the handler in server/auth/claim_advertisement.go.
const ClaimAdvertisementEndpointPath = "/internal/claim-advertisement"

// ClaimLookupEndpointPath is the GET endpoint (?url=<external URL>) a peer
// queries for a claim this host holds that may not have been gossiped yet.
const ClaimLookupEndpointPath = "/internal/claim-lookup"

// ClaimGossiper pushes ClaimRecords to the peers in HostRegistry. It reuses
// HostAdvertiser's transport shape but is a separate type on a separate
// endpoint (ADR-001, project_plans/cross-host-claim-dedup/decisions/).
//
// Fan-out is bounded like HostAdvertiser's: a receiver re-gossips a record only
// when ClaimIndex.RecordClaim reports IsNew, so each fact takes one extra hop
// per node at most. Run backfills only claims a peer has not yet been sent; it
// never rebroadcasts the whole set each tick.
type ClaimGossiper struct {
	identity  HostIdentity
	registry  *HostRegistry
	index     *ClaimIndex
	addresses []string
	client    *http.Client
	interval  time.Duration

	mu        sync.Mutex
	delivered map[claimDeliveryKey]struct{}
}

// claimDeliveryKey identifies one version of a claim sent to one peer.
type claimDeliveryKey struct {
	externalURL string
	claimant    string
	claimedAt   time.Time
	peer        string
}

// NewClaimGossiper constructs a ClaimGossiper. addresses are this instance's
// own advertised addresses, never gossiped to. A non-positive interval means
// DefaultHostAdvertisementInterval.
func NewClaimGossiper(identity HostIdentity, registry *HostRegistry, index *ClaimIndex, addresses []string, interval time.Duration) (*ClaimGossiper, error) {
	if !identity.ID.IsValid() {
		return nil, fmt.Errorf("claim gossiper requires a valid host identity")
	}
	if registry == nil || index == nil {
		return nil, fmt.Errorf("claim gossiper requires a host registry and a claim index")
	}
	if interval <= 0 {
		interval = DefaultHostAdvertisementInterval
	}
	return &ClaimGossiper{
		identity:  identity,
		registry:  registry,
		index:     index,
		addresses: addresses,
		// InsecureSkipVerify for the same reason as HostAdvertiser: peers serve
		// a locally-minted self-signed CA, and identity is verified at the
		// application layer (Ed25519 signature + TOFU-pinned key).
		client: &http.Client{
			Timeout:   5 * time.Second,
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, // #nosec G402 -- TLS is transport-only; identity is Ed25519/TOFU-verified at the application layer
		},
		interval:  interval,
		delivered: make(map[claimDeliveryKey]struct{}),
	}, nil
}

// Run backfills unsent claims to peers immediately and then every interval
// until ctx is cancelled. Callers start it with `go gossiper.Run(ctx)`.
func (g *ClaimGossiper) Run(ctx context.Context) {
	ticker := time.NewTicker(g.interval)
	defer ticker.Stop()
	for {
		_ = g.BackfillOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// BackfillOnce sends every claim in the local index to each live peer that has
// not yet received that version of it.
func (g *ClaimGossiper) BackfillOnce(ctx context.Context) error {
	var errs []error
	for _, record := range g.index.Snapshot() {
		if err := g.fanOut(ctx, record, ""); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// BroadcastOnce sends exactly record to every live peer. It is record-scoped,
// unlike HostAdvertiser.BroadcastOnce, and best-effort: an unreachable peer is
// reported in the joined error but does not stop the round.
func (g *ClaimGossiper) BroadcastOnce(ctx context.Context, record ClaimRecord) error {
	return g.fanOut(ctx, record, "")
}

// ReGossip forwards a record learned from a peer to every other live peer,
// skipping the claimant itself. Callers invoke it only when RecordClaim
// reported IsNew.
func (g *ClaimGossiper) ReGossip(ctx context.Context, record ClaimRecord) error {
	return g.fanOut(ctx, record, record.ClaimingHostID.String())
}

// fanOut sends record to each live peer except skipHostID and this instance's
// own addresses, trying a peer's addresses in order until one succeeds.
func (g *ClaimGossiper) fanOut(ctx context.Context, record ClaimRecord, skipHostID string) error {
	var errs []error
	for _, peer := range g.registry.LiveSnapshot() {
		if peer.HostID.String() == skipHostID || peer.HostID.String() == g.identity.ID.String() {
			continue
		}
		if err := g.sendToPeer(ctx, peer, record); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (g *ClaimGossiper) sendToPeer(ctx context.Context, peer RegistryEntry, record ClaimRecord) error {
	key := claimDeliveryKey{record.ExternalURL, record.ClaimingHostID.String(), record.ClaimedAt, peer.HostID.String()}
	g.mu.Lock()
	_, done := g.delivered[key]
	g.mu.Unlock()
	if done {
		return nil
	}
	var errs []error
	for _, addr := range peer.AdvertisedAddress {
		if g.isOwnAddress(addr) {
			continue
		}
		err := g.SendClaim(ctx, addr, record)
		if err == nil {
			g.mu.Lock()
			g.delivered[key] = struct{}{}
			g.mu.Unlock()
			return nil
		}
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (g *ClaimGossiper) isOwnAddress(addr string) bool {
	for _, own := range g.addresses {
		if own == addr {
			return true
		}
	}
	return false
}

// SendClaim POSTs record to addr's claim endpoint. addr is a bare "host:port".
func (g *ClaimGossiper) SendClaim(ctx context.Context, addr string, record ClaimRecord) error {
	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal claim: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+addr+ClaimAdvertisementEndpointPath, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to build claim request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send claim to %s: %w", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("claim to %s rejected: status %d", addr, resp.StatusCode)
	}
	return nil
}
