package jules

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// defaultSourceRegistryTTL is how long a resolved owner/repo -> JulesSourceName
// mapping is trusted before Resolve re-lists sources. Sources are read-only
// and change only when a user connects/disconnects a repo through the Jules
// web app, so a coarse TTL is fine.
const defaultSourceRegistryTTL = 10 * time.Minute

// sourceLister is the subset of *Client that JulesSourceRegistry depends on,
// so tests can substitute a fake without a real HTTP round trip.
type sourceLister interface {
	ListSources(ctx context.Context) ([]JulesSource, error)
}

// sourceRegistryEntry is the cached value behind one "owner/repo" key.
type sourceRegistryEntry struct {
	name JulesSourceName
	at   time.Time
}

// JulesSourceRegistry resolves an "owner/repo" pair to the JulesSourceName
// Jules uses to address that GitHub repo, caching hits in memory so a
// dispatch does not re-list every source on every call. A miss re-lists
// sources once (in case the user just connected the repo) before returning
// ErrJulesSourceNotRegistered.
type JulesSourceRegistry struct {
	client sourceLister

	// TTL bounds how long a cached entry is served without re-listing
	// sources. Zero means defaultSourceRegistryTTL (set by
	// NewJulesSourceRegistry; a zero-value JulesSourceRegistry falls back
	// to the same default in Resolve).
	TTL time.Duration

	// now is the injected clock, overridable in tests.
	now func() time.Time

	store sync.Map // key: "owner/repo" string, value: sourceRegistryEntry

	// refetchMu serializes population so concurrent misses within the
	// same refresh window make one ListSources call, not one per caller.
	refetchMu sync.Mutex
}

// NewJulesSourceRegistry creates a registry backed by client, with the
// default TTL and wall-clock time.Now.
func NewJulesSourceRegistry(client sourceLister) *JulesSourceRegistry {
	return &JulesSourceRegistry{
		client: client,
		TTL:    defaultSourceRegistryTTL,
		now:    time.Now,
	}
}

func (r *JulesSourceRegistry) ttl() time.Duration {
	if r.TTL > 0 {
		return r.TTL
	}
	return defaultSourceRegistryTTL
}

func (r *JulesSourceRegistry) clock() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func sourceRegistryKey(owner, repo string) string {
	return owner + "/" + repo
}

// Resolve returns the JulesSourceName for owner/repo, serving a live cache
// entry without calling ListSources. On a cache miss (including an expired
// entry) it re-lists sources once and re-checks before giving up.
func (r *JulesSourceRegistry) Resolve(ctx context.Context, owner, repo string) (JulesSourceName, error) {
	key := sourceRegistryKey(owner, repo)

	if entry, ok := r.lookup(key); ok {
		return entry.name, nil
	}

	if err := r.refresh(ctx); err != nil {
		return "", err
	}

	if entry, ok := r.lookup(key); ok {
		return entry.name, nil
	}

	return "", fmt.Errorf(
		"%w: %s/%s is not connected — connect it at jules.google.com before dispatching",
		ErrJulesSourceNotRegistered, owner, repo,
	)
}

// lookup returns the cached entry for key if present and not expired.
func (r *JulesSourceRegistry) lookup(key string) (sourceRegistryEntry, bool) {
	v, ok := r.store.Load(key)
	if !ok {
		return sourceRegistryEntry{}, false
	}
	entry := v.(sourceRegistryEntry)
	if r.clock().Sub(entry.at) >= r.ttl() {
		return sourceRegistryEntry{}, false
	}
	return entry, true
}

// refresh calls ListSources and repopulates the cache from the result.
func (r *JulesSourceRegistry) refresh(ctx context.Context) error {
	r.refetchMu.Lock()
	defer r.refetchMu.Unlock()

	sources, err := r.client.ListSources(ctx)
	if err != nil {
		return fmt.Errorf("jules: listing sources: %w", err)
	}

	now := r.clock()
	for _, s := range sources {
		owner, repo, ok := parseGitHubSourceName(s.Name)
		if !ok {
			continue
		}
		r.store.Store(sourceRegistryKey(owner, repo), sourceRegistryEntry{name: s.Name, at: now})
	}
	return nil
}

// githubSourceNamePrefix is the fixed portion of a GitHub-backed
// JulesSourceName: "sources/github-{owner}-{repo}".
const githubSourceNamePrefix = "sources/github-"

// parseGitHubSourceName splits a JulesSourceName of the form
// "sources/github-{owner}-{repo}" back into owner and repo. GitHub repo
// names may themselves contain hyphens (e.g. "stapler-squad"), so the split
// point is the first hyphen after the prefix — everything before it is the
// owner, everything after is the repo. This assumes the owner segment has
// no hyphen of its own, true for the accounts this repo dispatches against;
// a source name that doesn't fit the pattern is skipped rather than
// mis-parsed.
func parseGitHubSourceName(name JulesSourceName) (owner, repo string, ok bool) {
	s := string(name)
	if !strings.HasPrefix(s, githubSourceNamePrefix) {
		return "", "", false
	}
	rest := s[len(githubSourceNamePrefix):]
	idx := strings.Index(rest, "-")
	if idx <= 0 || idx == len(rest)-1 {
		return "", "", false
	}
	return rest[:idx], rest[idx+1:], true
}
