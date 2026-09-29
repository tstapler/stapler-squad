package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tstapler/stapler-squad/log"
)

// ClaimRecorder is the port Storage.CreateBacklogItem calls to record that
// this host now holds an item for record.ExternalURL. It is defined in
// package session, and injected via Storage.SetClaimRecorder, because
// server/services already imports session (ADR-002 of
// project_plans/cross-host-claim-dedup/decisions/); mirrors ItemChangePublisher.
//
// Storage supplies ExternalURL, ItemDeepLink (host-less, see
// ForeignClaimLookup is the read-only, local-only port SyncOne consults before
// creating an item: it reports a claim held by a host other than this one, and
// never touches the network. Injected via Storage.SetForeignClaimLookup.
type ForeignClaimLookup interface {
	ForeignClaim(externalURL string) (ClaimRecord, bool)
}

// ForeignClaim implements ForeignClaimLookup against the local ClaimIndex.
func (r *ClaimIndexRecorder) ForeignClaim(externalURL string) (ClaimRecord, bool) {
	return r.index.ForeignClaim(externalURL, r.identity.ID)
}

// BacklogItemDeepLinkPath) and ClaimedAt. The implementation owns the local
// identity, so it stamps ClaimingHostID and signs.
type ClaimRecorder interface {
	RecordClaim(ctx context.Context, record ClaimRecord) error
}

// BacklogItemDeepLinkPath is the host-less deep link path for item, using the
// public ID when one exists and the row ID otherwise. ClaimIndexRecorder
// prefixes it with "ssq://<host>".
func BacklogItemDeepLinkPath(item *BacklogItemData) string {
	id := item.ID
	if publicID, ok := item.PublicID(); ok {
		id = publicID.String()
	}
	return "/backlog/v1/" + id
}

// claimGossipTimeout bounds one background broadcast of a fresh claim.
const claimGossipTimeout = 30 * time.Second

// ClaimIndexRecorder is the production ClaimRecorder: it signs the claim with
// the local HostIdentity, stores it in the ClaimIndex, then best-effort gossips
// it to peers in the background so item creation never waits on the network.
type ClaimIndexRecorder struct {
	identity     HostIdentity
	index        *ClaimIndex
	gossiper     *ClaimGossiper // may be nil: record locally only
	deepLinkHost string
	clock        Clock
}

// NewClaimIndexRecorder builds a ClaimIndexRecorder. deepLinkHost is the
// hostname other hosts can use to reach this one (the "<hostname>" of
// ssq://<hostname>/...). gossiper may be nil. A nil clock means wall time.
func NewClaimIndexRecorder(identity HostIdentity, index *ClaimIndex, gossiper *ClaimGossiper, deepLinkHost string, clock Clock) (*ClaimIndexRecorder, error) {
	if !identity.ID.IsValid() || index == nil {
		return nil, fmt.Errorf("claim recorder requires a valid host identity and a claim index")
	}
	if clock == nil {
		clock = realClock{}
	}
	return &ClaimIndexRecorder{identity: identity, index: index, gossiper: gossiper, deepLinkHost: deepLinkHost, clock: clock}, nil
}

// RecordClaim implements ClaimRecorder.
func (r *ClaimIndexRecorder) RecordClaim(ctx context.Context, record ClaimRecord) error {
	claimedAt := record.ClaimedAt
	if claimedAt.IsZero() {
		claimedAt = r.clock.Now()
	}
	deepLink := record.ItemDeepLink
	if strings.HasPrefix(deepLink, "/") && r.deepLinkHost != "" {
		deepLink = "ssq://" + r.deepLinkHost + deepLink
	}
	signed := NewSignedClaimRecord(r.identity, record.ExternalURL, deepLink, claimedAt)

	outcome, err := r.index.RecordClaim(signed)
	if err != nil {
		return fmt.Errorf("record claim for %s: %w", record.ExternalURL, err)
	}
	if !outcome.Accepted {
		return fmt.Errorf("claim for %s was rejected by the claim index", record.ExternalURL)
	}
	if r.gossiper != nil && outcome.IsNew {
		detached := context.WithoutCancel(ctx)
		go func() {
			gossipCtx, cancel := context.WithTimeout(detached, claimGossipTimeout)
			defer cancel()
			if err := r.gossiper.BroadcastOnce(gossipCtx, signed); err != nil {
				log.Debug("claim_index.broadcast_partial", "external_url", signed.ExternalURL, "err", err)
			}
		}()
	}
	return nil
}
