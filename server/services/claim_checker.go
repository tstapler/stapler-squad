package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

// ClaimVerdictKind is the three-way outcome of a cross-host claim check.
type ClaimVerdictKind int

const (
	// ClaimUnclaimed: no other host is known to hold the URL.
	ClaimUnclaimed ClaimVerdictKind = iota
	// ClaimHeldByOther: a different host holds the URL; ClaimVerdict.Record says which.
	ClaimHeldByOther
	// ClaimCheckIndeterminate: at least one peer was unreachable and none reported
	// a claim, so "unclaimed" cannot be confirmed. Callers proceed optimistically.
	ClaimCheckIndeterminate
)

// ClaimVerdict is a claim check result. Build it with the New*Verdict
// constructors so Kind and Record cannot be mismatched.
type ClaimVerdict struct {
	Kind   ClaimVerdictKind
	Record session.ClaimRecord // meaningful only for ClaimHeldByOther
}

func NewUnclaimedVerdict() ClaimVerdict { return ClaimVerdict{Kind: ClaimUnclaimed} }

func NewIndeterminateVerdict() ClaimVerdict { return ClaimVerdict{Kind: ClaimCheckIndeterminate} }

func NewHeldByOtherVerdict(record session.ClaimRecord) ClaimVerdict {
	return ClaimVerdict{Kind: ClaimHeldByOther, Record: record}
}

// CrossHostClaimChecker is the read-only seam between claim-aware call sites and
// the claim substrate. Recording a claim is not here: it goes through
// session.ClaimRecorder on Storage.CreateBacklogItem.
type CrossHostClaimChecker interface {
	// CheckClaim consults the local index, then live peers, under one bounded
	// deadline. For interactively triggered paths only.
	CheckClaim(ctx context.Context, externalURL string) (ClaimVerdict, error)
	// CheckClaimLocalOnly reads only the local index and never touches the
	// network, so it is safe under locks and in background sweeps.
	CheckClaimLocalOnly(ctx context.Context, externalURL string) (ClaimVerdict, error)
}

// unimplementedClaimChecker is the nil-checker default: everything is unclaimed,
// so single-host behavior is unchanged.
type unimplementedClaimChecker struct{}

func (unimplementedClaimChecker) CheckClaim(context.Context, string) (ClaimVerdict, error) {
	return NewUnclaimedVerdict(), nil
}

func (unimplementedClaimChecker) CheckClaimLocalOnly(context.Context, string) (ClaimVerdict, error) {
	return NewUnclaimedVerdict(), nil
}

// ClaimDisputeResolver clears the local Disputed flag on a claim.
type ClaimDisputeResolver interface {
	ResolveDispute(ctx context.Context, externalURL string) error
}

// defaultClaimLookupTimeout bounds the whole peer fan-out of one CheckClaim,
// matching deep_link_resolver.go's per-peer liveness timeout.
const defaultClaimLookupTimeout = 2 * time.Second

// claimPeerLister is the slice of *session.HostRegistry the checker needs.
type claimPeerLister interface {
	Snapshot() []session.RegistryEntry
}

// LocalClaimChecker checks the local ClaimIndex and, for CheckClaim, live peers.
type LocalClaimChecker struct {
	index   *session.ClaimIndex
	selfID  session.HostID
	peers   claimPeerLister
	client  *http.Client
	timeout time.Duration
}

var (
	_ CrossHostClaimChecker = (*LocalClaimChecker)(nil)
	_ ClaimDisputeResolver  = (*LocalClaimChecker)(nil)
)

// NewLocalClaimChecker builds a checker for the host identified by selfID. peers
// is queried fresh on every CheckClaim so pruned hosts drop out of the fan-out.
func NewLocalClaimChecker(index *session.ClaimIndex, selfID session.HostID, peers claimPeerLister) *LocalClaimChecker {
	return &LocalClaimChecker{
		index:   index,
		selfID:  selfID,
		peers:   peers,
		client:  &http.Client{Transport: peerTLSTransport()},
		timeout: defaultClaimLookupTimeout,
	}
}

// CheckClaimLocalOnly implements CrossHostClaimChecker.
func (c *LocalClaimChecker) CheckClaimLocalOnly(_ context.Context, externalURL string) (ClaimVerdict, error) {
	if record, ok := c.index.ForeignClaim(externalURL, c.selfID); ok {
		return NewHeldByOtherVerdict(record), nil
	}
	return NewUnclaimedVerdict(), nil
}

// ResolveDispute implements ClaimDisputeResolver.
func (c *LocalClaimChecker) ResolveDispute(_ context.Context, externalURL string) error {
	return c.index.ResolveDispute(externalURL)
}

// CheckClaim implements CrossHostClaimChecker. A claim this host holds itself
// short-circuits to Unclaimed: the item is already local and nothing conflicts.
func (c *LocalClaimChecker) CheckClaim(ctx context.Context, externalURL string) (ClaimVerdict, error) {
	if record, ok := c.index.CheckClaim(externalURL); ok {
		if record.ClaimingHostID.String() == c.selfID.String() {
			return NewUnclaimedVerdict(), nil
		}
		return NewHeldByOtherVerdict(record), nil
	}
	return c.queryPeers(ctx, externalURL), nil
}

type peerAnswer struct {
	record  session.ClaimRecord
	claimed bool
	failed  bool
}

// queryPeers asks every known peer concurrently under one shared deadline; the
// first verified foreign claim wins and cancels the rest.
func (c *LocalClaimChecker) queryPeers(ctx context.Context, externalURL string) ClaimVerdict {
	var peers []session.RegistryEntry
	for _, p := range c.peers.Snapshot() {
		if p.HostID.String() != c.selfID.String() {
			peers = append(peers, p)
		}
	}
	if len(peers) == 0 {
		return NewUnclaimedVerdict()
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	answers := make(chan peerAnswer, len(peers))
	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(peer session.RegistryEntry) {
			defer wg.Done()
			answers <- c.askPeer(ctx, peer, externalURL)
		}(peer)
	}
	go func() { wg.Wait(); close(answers) }()

	anyFailed := false
	for a := range answers {
		if a.claimed {
			return NewHeldByOtherVerdict(a.record)
		}
		anyFailed = anyFailed || a.failed
	}
	if anyFailed {
		return NewIndeterminateVerdict()
	}
	return NewUnclaimedVerdict()
}

// askPeer tries the peer's addresses in order until one gives a definite
// answer (200 or 404). A record is accepted only after ClaimIndex.RecordClaim
// verifies its signature and TOFU pin; an unverifiable response is a failure.
func (c *LocalClaimChecker) askPeer(ctx context.Context, peer session.RegistryEntry, externalURL string) peerAnswer {
	for _, addr := range peer.AdvertisedAddress {
		record, found, err := c.lookup(ctx, claimLookupURL(addr, externalURL))
		if err != nil {
			log.Debug("claim_check.peer_lookup_failed", "peer", peer.HostID.String(), "addr", addr, "err", err)
			continue
		}
		if !found || record.ClaimingHostID.String() == c.selfID.String() {
			return peerAnswer{}
		}
		outcome, err := c.index.RecordClaim(record)
		if err != nil || !outcome.Accepted {
			log.Warn("claim_check.peer_record_rejected", "peer", peer.HostID.String(), "external_url", externalURL, "err", err)
			return peerAnswer{failed: true}
		}
		if stored, ok := c.index.ForeignClaim(externalURL, c.selfID); ok {
			record = stored
		}
		return peerAnswer{record: record, claimed: true}
	}
	return peerAnswer{failed: true}
}

func claimLookupURL(peerAddr, externalURL string) string {
	return "https://" + peerAddr + session.ClaimLookupEndpointPath + "?url=" + url.QueryEscape(externalURL)
}

func (c *LocalClaimChecker) lookup(ctx context.Context, target string) (session.ClaimRecord, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return session.ClaimRecord{}, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return session.ClaimRecord{}, false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusNotFound:
		return session.ClaimRecord{}, false, nil
	case http.StatusOK:
		var record session.ClaimRecord
		if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
			return session.ClaimRecord{}, false, fmt.Errorf("decode claim from %s: %w", target, err)
		}
		return record, true, nil
	default:
		return session.ClaimRecord{}, false, fmt.Errorf("claim lookup at %s: status %d", target, resp.StatusCode)
	}
}
