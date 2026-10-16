package services

import (
	"context"
	"sync"
	"time"

	"github.com/tstapler/stapler-squad/github"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const (
	// provenanceLookupTimeout keeps the comment read inside GitHub's webhook
	// delivery timeout, so a slow lookup cannot make the delivery look failed.
	provenanceLookupTimeout = 5 * time.Second
	// provenanceCacheTTL stops a burst of events on one PR (a check_run per job)
	// from re-reading its comments every time, hit or miss.
	provenanceCacheTTL = 10 * time.Minute
)

// prCommentLister reads a PR's comments. github.GetPRComments in production.
type prCommentLister func(ctx context.Context, ref github.RepoRef, prNumber int) ([]github.PRComment, error)

type provenanceCacheKey struct {
	repo     string
	prNumber int
}

type provenanceCacheEntry struct {
	provenance session.PRProvenance
	found      bool
	expires    time.Time
}

// prProvenanceReader resolves a PR's originating item from the comment stamped
// by report_pr_created, so a host that is not the PR's origin can still trace it.
type prProvenanceReader struct {
	list  prCommentLister
	now   func() time.Time
	mu    sync.Mutex
	cache map[provenanceCacheKey]provenanceCacheEntry
}

func newPRProvenanceReader(list prCommentLister) *prProvenanceReader {
	return &prProvenanceReader{list: list, now: time.Now, cache: map[provenanceCacheKey]provenanceCacheEntry{}}
}

// Resolve returns the newest provenance stamp among the PR's comments. A read
// error is not cached, so the next event retries.
func (r *prProvenanceReader) Resolve(ctx context.Context, ref github.RepoRef, prNumber int) (session.PRProvenance, bool) {
	key := provenanceCacheKey{repo: ref.Host() + "/" + ref.String(), prNumber: prNumber}
	r.mu.Lock()
	if entry, ok := r.cache[key]; ok && r.now().Before(entry.expires) {
		r.mu.Unlock()
		return entry.provenance, entry.found
	}
	r.mu.Unlock()

	lookupCtx, cancel := context.WithTimeout(ctx, provenanceLookupTimeout)
	defer cancel()
	comments, err := r.list(lookupCtx, ref, prNumber)
	if err != nil {
		log.Warn("github_webhook.pr_provenance_lookup_failed", "repo", ref.String(), "pr_number", prNumber, "err", err)
		return session.PRProvenance{}, false
	}
	var found session.PRProvenance
	ok := false
	for i := len(comments) - 1; i >= 0; i-- {
		if p, parsed := session.ParsePRProvenanceComment(comments[i].Body); parsed {
			found, ok = p, true
			break
		}
	}
	r.mu.Lock()
	r.cache[key] = provenanceCacheEntry{provenance: found, found: ok, expires: r.now().Add(provenanceCacheTTL)}
	r.mu.Unlock()
	return found, ok
}

// provenanceReader is process-wide (one webhook handler exists per process) and a
// package var so tests can swap it; keeping it off GitHubWebhookHandler leaves
// the routing file untouched.
var provenanceReader = newPRProvenanceReader(github.GetPRComments)

// traceUnmatchedPR is the fallback for a PR event the local item lookup missed:
// it reads the stamped provenance comment back and logs the originating item and
// host. Gated by cross_host_claim_dedup because stamping is.
func (h *GitHubWebhookHandler) traceUnmatchedPR(ctx context.Context, payload map[string]interface{}, fullName string, prNumber int) (session.PRProvenance, bool) {
	if h.cfg == nil || !h.cfg.GetFeatureFlag(crossHostClaimDedupFlagName) || provenanceReader == nil {
		return session.PRProvenance{}, false
	}
	base, ok := splitRepoFullName(fullName)
	if !ok {
		return session.PRProvenance{}, false
	}
	ref, err := github.NewRepoRefWithHost(base.Owner(), base.Repo(), payloadRepoHost(payload, enterpriseHostsFor(h.cfg)))
	if err != nil {
		return session.PRProvenance{}, false
	}
	provenance, found := provenanceReader.Resolve(ctx, ref, prNumber)
	if !found {
		return session.PRProvenance{}, false
	}
	log.Info("github_webhook.pr_provenance_resolved",
		"repo", fullName, "pr_number", prNumber,
		"item_id", provenance.ItemID, "origin_host_id", provenance.HostID.String())
	return provenance, true
}
