package jules

import (
	"context"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
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

// sourceRegistryEntry is the cached value behind one JulesSourceName key.
type sourceRegistryEntry struct {
	at time.Time
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

	// store is keyed by the exact JulesSourceName string Jules returned
	// (or, equivalently for a GitHub source, by githubSourceName(owner,
	// repo)) so a hit never depends on reverse-parsing an ambiguous
	// "owner-repo" concatenation -- see githubSourceName.
	store sync.Map // key: JulesSourceName string, value: sourceRegistryEntry

	// refetchGroup coalesces concurrent misses within the same refresh
	// window into one ListSources call, not one per caller: every goroutine
	// that calls refresh() while another is already in flight shares that
	// call's result instead of issuing its own.
	refetchGroup singleflight.Group
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

// githubSourceNamePrefix is the fixed portion of a GitHub-backed
// JulesSourceName: "sources/github-{owner}-{repo}".
const githubSourceNamePrefix = "sources/github-"

// githubSourceName builds the JulesSourceName Jules assigns to a GitHub
// repo's source by construction, from an owner/repo pair the caller already
// has. This deliberately replaces reverse-parsing a ListSources result's
// "owner-repo" segment: GitHub org and repo names can each contain hyphens
// (e.g. owner "my-org", repo "stapler-squad"), so splitting the
// concatenation back into (owner, repo) is ambiguous and was silently
// mis-parsing hyphenated names. Building the expected name from the known
// owner/repo instead of decomposing an unknown one sidesteps the ambiguity
// entirely.
func githubSourceName(owner, repo string) JulesSourceName {
	return JulesSourceName(githubSourceNamePrefix + owner + "-" + repo)
}

// Resolve returns the JulesSourceName for owner/repo, serving a live cache
// entry without calling ListSources. On a cache miss (including an expired
// entry) it re-lists sources once and re-checks before giving up.
func (r *JulesSourceRegistry) Resolve(ctx context.Context, owner, repo string) (JulesSourceName, error) {
	name := githubSourceName(owner, repo)
	key := string(name)

	if _, ok := r.lookup(key); ok {
		return name, nil
	}

	if err := r.refresh(ctx); err != nil {
		return "", err
	}

	if _, ok := r.lookup(key); ok {
		return name, nil
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

// refresh calls ListSources and repopulates the cache from the result. It is
// shared via refetchGroup across every goroutine that calls it concurrently:
// only the first caller ("leader") actually invokes ListSources, and every
// concurrent caller ("follower") blocks on that same call and receives its
// result (or error), rather than each issuing its own ListSources call.
//
// Each returned source is cached under its own exact name (s.Name) --
// unparsed -- so a later Resolve only needs to check whether the name it
// constructs from owner/repo is present, never to decompose one.
func (r *JulesSourceRegistry) refresh(ctx context.Context) error {
	_, err, _ := r.refetchGroup.Do("refresh", func() (any, error) {
		sources, err := r.client.ListSources(ctx)
		if err != nil {
			return nil, fmt.Errorf("jules: listing sources: %w", err)
		}

		now := r.clock()
		for _, s := range sources {
			r.store.Store(string(s.Name), sourceRegistryEntry{at: now})
		}
		return struct{}{}, nil
	})
	return err
}
