package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tstapler/stapler-squad/log"
	"golang.org/x/sync/singleflight"
)

// UserPR is an open GitHub pull request authored by the authenticated user,
// optionally annotated with local session IDs and worktree paths.
type UserPR struct {
	Owner             string
	Repo              string
	Number            int
	Title             string
	URL               string
	HeadRef           string
	BaseRef           string
	State             string
	IsDraft           bool
	UpdatedAt         time.Time
	ClosedAt          time.Time
	MergedAt          time.Time
	ApprovedCount     int
	ChangesReqCount   int
	CheckConclusion   string   // "success" / "failure" / "pending" / ""
	SessionIDs        []string // titles of LinkedSessions, kept for compat
	LocalWorktreePath string
	// LinkedSessions lists every session on the PR's branch, most recently
	// active first. Entries with LegacyFallback set matched only the old
	// owner-only key; the nudge path must ignore them.
	LinkedSessions []LinkedSession

	Host         string // never empty; "github.com" for github.com
	AccountLogin string // login name of the account whose poll returned this PR; never a token
	// FailingChecks is capped at maxFailingChecks, CheckRuns first.
	FailingChecks []FailingCheck
	// UnresolvedThreadCount is nil when unknown (degraded mode, legacy query,
	// or a host that omitted reviewThreads), which is distinct from 0.
	UnresolvedThreadCount      *int
	UnresolvedThreadsTruncated bool  // count was capped at maxReviewThreads
	HasMergeConflict           *bool // nil = GitHub mergeable UNKNOWN (computed lazily)
	DetailsLoaded              bool  // true only when reviewThreads came back
}

// FailingCheck is one failing CheckRun or StatusContext on a PR's head commit.
type FailingCheck struct {
	Name       string
	URL        string
	Conclusion string
}

// AccountPollState is the outcome of one account's last poll.
type AccountPollState int

const (
	AccountPollOK AccountPollState = iota + 1
	AccountPollUnauthorized
	AccountPollRateLimited
	AccountPollError
)

// AccountPollStatus reports one account's last poll outcome so the UI can tell
// an expired or failing account from "not connected".
type AccountPollStatus struct {
	Host         string
	AccountLogin string
	State        AccountPollState
	Detail       string // short sanitized reason; never contains a token
}

const (
	maxFailingChecks = 10
	maxReviewThreads = 50 // matches reviewThreads(first: 50) in the widened query
)

// LinkedSessionStatus is the coarse lifecycle state of a linked session.
type LinkedSessionStatus string

const (
	LinkedSessionUnknown LinkedSessionStatus = ""
	LinkedSessionRunning LinkedSessionStatus = "running"
	LinkedSessionPaused  LinkedSessionStatus = "paused"
	LinkedSessionStopped LinkedSessionStatus = "stopped"
)

// LinkedSession is a local session linked to a UserPR.
type LinkedSession struct {
	SessionID    string // session title
	Status       LinkedSessionStatus
	LastActiveAt time.Time // zero when unknown
	// LegacyFallback marks a link made only by the owner-only legacy index.
	LegacyFallback bool
}

// PRAnnotationSession carries session data needed to annotate UserPR entries.
// Defined here (not in the session package) to avoid an import cycle:
// session imports github, so github cannot import session.
//
// Repo is a typed value object: holding a valid RepoRef proves owner and repo
// are non-empty. Sessions without a resolvable GitHub repo are skipped.
type PRAnnotationSession struct {
	ID           string
	Branch       string
	Repo         RepoRef
	PRNumber     int // fallback: match by PR number when branch name doesn't match headRef
	Status       LinkedSessionStatus
	LastActiveAt time.Time // zero when unknown
}

// PRAnnotationWorktree carries worktree data for annotation.
type PRAnnotationWorktree struct {
	Branch       string
	Repo         RepoRef
	WorktreePath string
}

// userPRSnapshot is an immutable snapshot stored in atomic.Value (COW pattern).
type userPRSnapshot struct {
	prs             []UserPR
	accountStatuses []AccountPollStatus
	capturedAt      time.Time
}

// loginResult is an immutable auth state stored in atomic.Value (single-account compat).
type loginResult struct {
	login     string
	checkedAt time.Time
}

// connectedAccount is one (token, login, host) triple resolved during a multi-account fetch.
type connectedAccount struct {
	token string
	login string
	host  string
}

// multiLoginState caches the resolved accounts for the multi-account path.
type multiLoginState struct {
	accounts  []connectedAccount
	checkedAt time.Time
}

// UserPRCacheConfig controls polling behaviour.
type UserPRCacheConfig struct {
	// PollInterval controls how often the cache refreshes from GitHub.
	PollInterval time.Duration
	// LoginCacheTTL controls how long the authenticated login is cached.
	LoginCacheTTL time.Duration
	// DegradedDetails drops reviewThreads from the poll (ADR-001 degraded
	// mode, selected when the measured query cost exceeds budget): thread
	// counts stay unknown while check names and mergeable are still fetched.
	DegradedDetails bool
}

// DefaultUserPRCacheConfig returns sensible defaults.
func DefaultUserPRCacheConfig() UserPRCacheConfig {
	return UserPRCacheConfig{
		PollInterval:  2 * time.Minute,
		LoginCacheTTL: 10 * time.Minute,
	}
}

// UserPRCache fetches and caches open GitHub PRs authored by the authenticated user.
// All reads are lock-free (atomic.Value COW). Concurrent refresh calls are
// coalesced via singleflight.
type onUpdatedFn struct {
	fn func(prs []UserPR)
}

type UserPRCache struct {
	config       UserPRCacheConfig
	snapshot     atomic.Value       // stores *userPRSnapshot
	subscribers  sync.Map           // maps string ID → chan []UserPR
	onUpdated    atomic.Value       // stores onUpdatedFn
	cachedLogin  atomic.Value       // stores string (first connected login; backward compat)
	cachedLogins atomic.Value       // stores []string (all connected logins)
	loginState   atomic.Value       // stores loginResult (single-account; backward compat)
	multiLogin   atomic.Value       // stores *multiLoginState (multi-account)
	loginGen     atomic.Uint64      // bumped by InvalidateLoginCache; see its doc comment
	loginGroup   singleflight.Group //nolint:exhaustruct
	refreshGroup singleflight.Group //nolint:exhaustruct
	ctx          context.Context
	cancel       context.CancelFunc
	startOnce    sync.Once
	// widenedUnsupported records hosts that rejected the widened query, so
	// later polls skip the doomed first attempt. Keyed by normalized host.
	widenedUnsupported sync.Map
	done               chan struct{} // closed when loop() returns; nil until Start

	nudges nudgeTracker     // success-metric state; see user_pr_nudge_track.go
	now    func() time.Time // injected in tests; nil means time.Now
}

// NewUserPRCache creates a cache with default configuration.
func NewUserPRCache() *UserPRCache {
	return NewUserPRCacheWithConfig(DefaultUserPRCacheConfig())
}

// NewUserPRCacheWithConfig creates a cache with custom configuration.
func NewUserPRCacheWithConfig(cfg UserPRCacheConfig) *UserPRCache {
	return &UserPRCache{
		config: cfg,
	}
}

// Start launches the background polling goroutine. Safe to call multiple times;
// only the first call starts the goroutine.
func (c *UserPRCache) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.ctx, c.cancel = context.WithCancel(ctx)
		c.done = make(chan struct{})
		go c.loop()
	})
}

// Stop halts background polling and blocks until loop() has actually exited.
// Without waiting here, a caller (notably a test's t.Cleanup) can return while
// loop()'s unconditional first fetch (see loop's doc comment) is still running,
// letting it race the next caller's use of shared package-level state (e.g.
// go-keyring's mock, which a subsequent test re-initializes via MockInit) —
// confirmed live via `go test -race`: TestListGitHubAccounts_
// AccountOnUnconfiguredEnterpriseHost_IncludesHostInEnterpriseHosts raced
// against a prior test's still-running fetch() on go-keyring's global state.
// Safe to call before Start (done is nil, no-op) or more than once (cancel and
// a receive on an already-closed channel are both idempotent).
func (c *UserPRCache) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.done != nil {
		<-c.done
	}
}

// SetOnUpdated atomically registers a callback invoked after every successful
// refresh. Pass nil to clear. The callback receives the current PR slice.
// Safe to call at any time, including after Start.
func (c *UserPRCache) SetOnUpdated(fn func(prs []UserPR)) {
	if fn == nil {
		c.onUpdated.Store(onUpdatedFn{})
	} else {
		c.onUpdated.Store(onUpdatedFn{fn: fn})
	}
}

// GetAll returns a copy of the current PR snapshot. Returns nil before the
// first successful fetch.
func (c *UserPRCache) GetAll() []UserPR {
	v := c.snapshot.Load()
	if v == nil {
		return nil
	}
	snap := v.(*userPRSnapshot)
	out := make([]UserPR, len(snap.prs))
	copy(out, snap.prs)
	return out
}

// AccountStatuses returns the per-account outcome of the last poll (nil before the first).
func (c *UserPRCache) AccountStatuses() []AccountPollStatus {
	v := c.snapshot.Load()
	if v == nil {
		return nil
	}
	snap := v.(*userPRSnapshot)
	out := make([]AccountPollStatus, len(snap.accountStatuses))
	copy(out, snap.accountStatuses)
	return out
}

// Subscribe registers a channel to receive PR snapshot updates.
// The channel must be buffered. The caller is responsible for calling Unsubscribe.
func (c *UserPRCache) Subscribe(id string, ch chan []UserPR) {
	c.subscribers.Store(id, ch)
}

// Unsubscribe removes a previously registered subscriber channel.
func (c *UserPRCache) Unsubscribe(id string) {
	c.subscribers.Delete(id)
}

// InvalidateLoginCache clears the cached login state so the next Refresh call
// re-fetches the authenticated user from the GitHub API. Call this after
// storing a new token (e.g. after a successful Device Flow auth) so the
// cache picks up the new credentials immediately.
//
// Bumping loginGen (not just clearing multiLogin) matters because a stale
// resolveAllLogins call from before the invalidation may still be in flight
// (e.g. loop()'s unconditional first fetch, started with zero tokens before a
// caller adds one) — the static singleflight key "login" would otherwise
// coalesce a Refresh arriving right after this call into that stale call's
// empty result. Including the generation in the key forces a genuinely new
// call instead. Confirmed live via go test -race: TestListGitHubAccounts_
// AccountOnUnconfiguredEnterpriseHost_IncludesHostInEnterpriseHosts saw
// EnterpriseHosts come back empty because its Refresh() joined the
// just-started cache's own stale zero-token fetch.
func (c *UserPRCache) InvalidateLoginCache() {
	c.loginGen.Add(1)
	c.loginState.Store(loginResult{})
	c.cachedLogin.Store("")
	c.multiLogin.Store((*multiLoginState)(nil))
	c.cachedLogins.Store([]string{})
}

// GetCachedLogin returns the first connected GitHub login, or "" if none yet.
func (c *UserPRCache) GetCachedLogin() string {
	v := c.cachedLogin.Load()
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetCachedLogins returns all connected GitHub logins.
func (c *UserPRCache) GetCachedLogins() []string {
	v := c.cachedLogins.Load()
	if v == nil {
		return nil
	}
	s, _ := v.([]string)
	out := make([]string, len(s))
	copy(out, s)
	return out
}

// CachedAccount is a resolved (login, host) pair for the accounts list RPC.
type CachedAccount struct {
	Login string
	Host  string
}

// GetCachedAccounts returns all connected GitHub accounts with their host.
func (c *UserPRCache) GetCachedAccounts() []CachedAccount {
	v := c.multiLogin.Load()
	s, ok := v.(*multiLoginState)
	if !ok || s == nil {
		return nil
	}
	out := make([]CachedAccount, len(s.accounts))
	for i, a := range s.accounts {
		out[i] = CachedAccount{Login: a.login, Host: a.host}
	}
	return out
}

// AnnotateStats reports link-quality counters from one Annotate pass.
type AnnotateStats struct {
	LegacyFallbackLinks int // session-PR links made only by the legacy index
	UnmatchedSessions   int // sessions with a branch that link to no PR
}

// Annotate enriches the current snapshot with linked sessions and worktree
// paths. It performs a COW update: load, copy, mutate, store. No-op if the
// snapshot hasn't been populated yet.
//
// Sessions match on host+owner+repo+branch (or PR number). The legacy
// owner-only index is consulted only when the strict key misses, and only for
// same-host sessions whose repo is not itself a repo in the PR list, so it can
// keep pre-existing links alive without cross-linking known-different repos.
func (c *UserPRCache) Annotate(sessions []PRAnnotationSession, worktrees []PRAnnotationWorktree) AnnotateStats {
	v := c.snapshot.Load()
	if v == nil {
		return AnnotateStats{}
	}
	old := v.(*userPRSnapshot)

	idx := newSessionIndex(sessions)
	worktreeByKey := make(map[LinkKey]string, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch == "" || !wt.Repo.IsValid() {
			continue
		}
		worktreeByKey[wt.Repo.BranchKey(wt.Branch)] = wt.WorktreePath
	}
	prRepos := make(map[string]bool, len(old.prs))
	prRefs := make([]RepoRef, len(old.prs))
	for i, pr := range old.prs {
		prRefs[i], _ = NewRepoRefWithHost(pr.Owner, pr.Repo, pr.Host)
		if prRefs[i].IsValid() {
			prRepos[prRefs[i].repoKey()] = true
		}
	}

	var stats AnnotateStats
	linked := make(map[string]bool, len(sessions))
	annotated := make([]UserPR, len(old.prs))
	for i, pr := range old.prs {
		ref := prRefs[i]
		if ref.IsValid() {
			ls := idx.strict(ref, pr.HeadRef, pr.Number)
			if len(ls) == 0 {
				ls = idx.legacy(ref, pr.HeadRef, pr.Number, prRepos)
				stats.LegacyFallbackLinks += len(ls)
			}
			sortLinkedSessions(ls)
			pr.LinkedSessions = ls
			pr.SessionIDs = linkedSessionIDs(ls)
			for _, l := range ls {
				linked[l.SessionID] = true
			}
			pr.LocalWorktreePath = worktreeByKey[ref.BranchKey(pr.HeadRef)]
		}
		annotated[i] = pr
	}
	for _, s := range sessions {
		if s.Branch != "" && s.Repo.IsValid() && !linked[s.ID] {
			stats.UnmatchedSessions++
		}
	}
	if stats.LegacyFallbackLinks > 0 {
		log.Info("UserPRCache: sessions linked via legacy owner-only key", "count", stats.LegacyFallbackLinks)
	}
	log.Debug("UserPRCache: sessions with a branch but no matching PR", "count", stats.UnmatchedSessions)
	c.snapshot.Store(&userPRSnapshot{prs: annotated, accountStatuses: old.accountStatuses, capturedAt: old.capturedAt})
	return stats
}

// sessionIndex holds the strict (host/owner/repo) and legacy (owner-only)
// lookup tables for one Annotate pass.
type sessionIndex struct {
	byBranch, byNum             map[LinkKey][]PRAnnotationSession
	legacyByBranch, legacyByNum map[LinkKey][]PRAnnotationSession
}

func newSessionIndex(sessions []PRAnnotationSession) sessionIndex {
	idx := sessionIndex{
		byBranch:       make(map[LinkKey][]PRAnnotationSession, len(sessions)),
		byNum:          make(map[LinkKey][]PRAnnotationSession),
		legacyByBranch: make(map[LinkKey][]PRAnnotationSession, len(sessions)),
		legacyByNum:    make(map[LinkKey][]PRAnnotationSession),
	}
	for _, s := range sessions {
		if !s.Repo.IsValid() {
			continue
		}
		if s.Branch != "" {
			k := s.Repo.BranchKey(s.Branch)
			idx.byBranch[k] = append(idx.byBranch[k], s)
			lk := s.Repo.LegacyBranchKey(s.Branch)
			idx.legacyByBranch[lk] = append(idx.legacyByBranch[lk], s)
		}
		if s.PRNumber > 0 {
			k := s.Repo.PRKey(s.PRNumber)
			idx.byNum[k] = append(idx.byNum[k], s)
			lk := s.Repo.LegacyPRKey(s.PRNumber)
			idx.legacyByNum[lk] = append(idx.legacyByNum[lk], s)
		}
	}
	return idx
}

// strict matches on the full host/owner/repo key: branch first, then PR number.
func (x sessionIndex) strict(ref RepoRef, headRef string, number int) []LinkedSession {
	ss := x.byBranch[ref.BranchKey(headRef)]
	if len(ss) == 0 && number > 0 {
		ss = x.byNum[ref.PRKey(number)]
	}
	return toLinkedSessions(ss, false)
}

// legacy matches the old owner-only key, restricted to same-host sessions
// whose repo is not one of the PR list's repos (those are known-different).
func (x sessionIndex) legacy(ref RepoRef, headRef string, number int, prRepos map[string]bool) []LinkedSession {
	ss := x.legacyByBranch[ref.LegacyBranchKey(headRef)]
	if len(ss) == 0 && number > 0 {
		ss = x.legacyByNum[ref.LegacyPRKey(number)]
	}
	var kept []PRAnnotationSession
	for _, s := range ss {
		if NormalizeHost(s.Repo.Host()) != NormalizeHost(ref.Host()) || prRepos[s.Repo.repoKey()] {
			continue
		}
		kept = append(kept, s)
	}
	return toLinkedSessions(kept, true)
}

func toLinkedSessions(ss []PRAnnotationSession, legacy bool) []LinkedSession {
	if len(ss) == 0 {
		return nil
	}
	out := make([]LinkedSession, len(ss))
	for i, s := range ss {
		out[i] = LinkedSession{SessionID: s.ID, Status: s.Status, LastActiveAt: s.LastActiveAt, LegacyFallback: legacy}
	}
	return out
}

func linkedSessionIDs(ls []LinkedSession) []string {
	if len(ls) == 0 {
		return nil
	}
	ids := make([]string, len(ls))
	for i, l := range ls {
		ids[i] = l.SessionID
	}
	return ids
}

// sortLinkedSessions orders most recently active first; sessions with no
// timestamp sort last, and ties break by session title.
func sortLinkedSessions(ls []LinkedSession) {
	sort.SliceStable(ls, func(i, j int) bool {
		a, b := ls[i], ls[j]
		if a.LastActiveAt.IsZero() != b.LastActiveAt.IsZero() {
			return !a.LastActiveAt.IsZero()
		}
		if !a.LastActiveAt.Equal(b.LastActiveAt) {
			return a.LastActiveAt.After(b.LastActiveAt)
		}
		return a.SessionID < b.SessionID
	})
}

// Refresh triggers an immediate fetch from GitHub, coalescing concurrent calls.
func (c *UserPRCache) Refresh(ctx context.Context) error {
	_, err, _ := c.refreshGroup.Do("refresh", func() (any, error) {
		return struct{}{}, c.fetch()
	})
	if err != nil {
		return err
	}
	return nil
}

// loop is the background polling goroutine. Closes c.done on exit so Stop can
// block until this goroutine has genuinely finished (see Stop's doc comment).
func (c *UserPRCache) loop() {
	defer close(c.done)
	// Fetch immediately on start.
	if err := c.fetch(); err != nil {
		log.Warn("UserPRCache: initial fetch failed", "err", err)
	}
	ticker := time.NewTicker(c.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.fetch(); err != nil {
				log.Warn("UserPRCache: fetch failed", "err", err)
			}
		}
	}
}

// fetch queries the GitHub GraphQL API for all connected accounts' open PRs and merges them.
func (c *UserPRCache) fetch() error {
	accounts, err := c.resolveAllLogins()
	if err != nil {
		log.Debug("UserPRCache: skipping fetch, no GitHub auth", "err", err)
		return err
	}
	if len(accounts) == 0 {
		return nil
	}

	type prResult struct {
		prs []UserPR
		err error
	}
	results := make([]prResult, len(accounts))
	var wg sync.WaitGroup
	for i, acc := range accounts {
		i, acc := i, acc
		wg.Add(1)
		go func() {
			defer wg.Done()
			prs, fetchErr := c.fetchUserPRsForToken(acc.host, acc.login, acc.token)
			results[i] = prResult{prs: prs, err: fetchErr}
		}()
	}
	wg.Wait()

	// Merge and dedup by URL (same PR can appear via multiple account tokens, e.g. org members).
	// First account in resolveAllLogins order wins.
	seen := make(map[string]bool)
	var merged []UserPR
	statuses := make([]AccountPollStatus, 0, len(accounts))
	for i, r := range results {
		statuses = append(statuses, accountPollStatus(accounts[i], r.err))
		if r.err != nil {
			log.Warn("UserPRCache: fetch failed for account", "host", accounts[i].host, "login", accounts[i].login, "err", r.err)
			continue
		}
		for _, pr := range r.prs {
			if !seen[pr.URL] {
				seen[pr.URL] = true
				merged = append(merged, pr)
			}
		}
	}

	snap := &userPRSnapshot{prs: merged, accountStatuses: statuses, capturedAt: time.Now()}
	c.snapshot.Store(snap)

	out := make([]UserPR, len(merged))
	copy(out, merged)
	c.trackNudges(out)
	c.subscribers.Range(func(_, v any) bool {
		ch := v.(chan []UserPR)
		select {
		case ch <- out:
		default:
			log.Warn("UserPRCache: subscriber channel full, dropping event")
		}
		return true
	})
	if v := c.onUpdated.Load(); v != nil {
		if cb := v.(onUpdatedFn).fn; cb != nil {
			cb(out)
		}
	}
	return nil
}

// resolveAllLogins returns all connected (token, login) pairs, refreshing if stale.
// Results are coalesced via singleflight so concurrent callers share one network round-trip.
func (c *UserPRCache) resolveAllLogins() ([]connectedAccount, error) {
	if v := c.multiLogin.Load(); v != nil {
		if s, ok := v.(*multiLoginState); ok && s != nil && time.Since(s.checkedAt) < c.config.LoginCacheTTL {
			return s.accounts, nil
		}
	}

	genKey := "login-" + strconv.FormatUint(c.loginGen.Load(), 10)
	res, err, _ := c.loginGroup.Do(genKey, func() (interface{}, error) {
		tokens := collectAllTokens()
		if len(tokens) == 0 {
			s := &multiLoginState{accounts: nil, checkedAt: time.Now()}
			c.multiLogin.Store(s)
			c.cachedLogins.Store([]string{})
			c.loginState.Store(loginResult{checkedAt: time.Now()})
			return []connectedAccount(nil), nil
		}

		type loginRes struct {
			acc connectedAccount
			err error
		}
		ch := make(chan loginRes, len(tokens))
		ctx, cancel := context.WithTimeout(c.ctx, 15*time.Second)
		defer cancel()

		for _, tok := range tokens {
			tok := tok
			go func() {
				login, err := GetCurrentUserLoginWithToken(ctx, tok.Host, tok.Token)
				if err != nil || login == "" {
					ch <- loginRes{err: err}
					return
				}
				ch <- loginRes{acc: connectedAccount{token: tok.Token, login: login, host: tok.Host}}
			}()
		}

		seen := make(map[string]bool)
		var accounts []connectedAccount
		var logins []string
		for range tokens {
			r := <-ch
			if r.err != nil || r.acc.login == "" {
				continue
			}
			key := r.acc.host + "/" + r.acc.login
			if !seen[key] {
				seen[key] = true
				accounts = append(accounts, r.acc)
				logins = append(logins, r.acc.login)
			}
		}

		s := &multiLoginState{accounts: accounts, checkedAt: time.Now()}
		c.multiLogin.Store(s)
		c.cachedLogins.Store(logins)
		firstLogin := ""
		if len(logins) > 0 {
			firstLogin = logins[0]
		}
		c.cachedLogin.Store(firstLogin)
		c.loginState.Store(loginResult{login: firstLogin, checkedAt: time.Now()})
		return accounts, nil
	})

	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, nil
	}
	return res.([]connectedAccount), nil
}

// collectAllTokens returns all available GitHub tokens from env vars and the keychain.
// Env-var tokens appear first.
func collectAllTokens() []AccountToken {
	var tokens []AccountToken
	seen := make(map[string]bool)
	for _, envKey := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if tok := os.Getenv(envKey); tok != "" && !seen[tok] {
			seen[tok] = true
			tokens = append(tokens, AccountToken{Username: "env:" + envKey, Token: tok})
		}
	}
	for _, at := range GetAllKeychainTokens() {
		if !seen[at.Token] {
			seen[at.Token] = true
			tokens = append(tokens, at)
		}
	}
	return tokens
}

// userPRQueryPrefix/Suffix wrap the per-mode field selections below.
const userPRQueryPrefix = `
query UserPRs {
  viewer {
    pullRequests(first: 100, states: [OPEN], orderBy: {field: UPDATED_AT, direction: DESC}) {
      nodes {
        number
        title
        url
        headRefName
        baseRefName
        state
        isDraft
        updatedAt
        closedAt
        mergedAt
        repository {
          owner { login }
          name
        }
        reviewDecision
        reviews(last: 20, states: [APPROVED, CHANGES_REQUESTED]) {
          nodes { state }
        }
`

const userPRQuerySuffix = `      }
    }
  }
}`

const (
	userPRMergeableField = `        mergeable
`
	userPRReviewThreadsField = `        reviewThreads(first: 50) {
          totalCount
          nodes { isResolved isOutdated }
        }
`
	userPRCommitsLegacy = `        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup { state }
            }
          }
        }
`
	userPRCommitsWithContexts = `        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup {
                state
                contexts(first: 25) {
                  nodes {
                    __typename
                    ... on CheckRun { name conclusion detailsUrl }
                    ... on StatusContext { context state targetUrl }
                  }
                }
              }
            }
          }
        }
`
)

// The three query shapes (ADR-001). Widened = mergeable + review threads +
// check contexts; degraded drops review threads; legacy is the pre-widening
// query, used when a host rejects the widened one.
const (
	userPRGraphQLQuery         = userPRQueryPrefix + userPRMergeableField + userPRReviewThreadsField + userPRCommitsWithContexts + userPRQuerySuffix
	userPRGraphQLQueryDegraded = userPRQueryPrefix + userPRMergeableField + userPRCommitsWithContexts + userPRQuerySuffix
	userPRGraphQLQueryLegacy   = userPRQueryPrefix + userPRCommitsLegacy + userPRQuerySuffix
)

// graphQLResponse is the top-level GraphQL response envelope.
type graphQLResponse struct {
	Data   *graphQLData   `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type graphQLError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type graphQLData struct {
	Viewer struct {
		PullRequests struct {
			Nodes []graphQLPRNode `json:"nodes"`
		} `json:"pullRequests"`
	} `json:"viewer"`
}

type graphQLReviewThreads struct {
	TotalCount int `json:"totalCount"`
	Nodes      []struct {
		IsResolved bool `json:"isResolved"`
		IsOutdated bool `json:"isOutdated"`
	} `json:"nodes"`
}

// graphQLContext is a CheckRun or StatusContext, discriminated by TypeName.
type graphQLContext struct {
	TypeName   string `json:"__typename"`
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	Context    string `json:"context"`
	State      string `json:"state"`
	TargetURL  string `json:"targetUrl"`
}

type graphQLPRNode struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	State       string `json:"state"`
	IsDraft     bool   `json:"isDraft"`
	UpdatedAt   string `json:"updatedAt"`
	ClosedAt    string `json:"closedAt"`
	MergedAt    string `json:"mergedAt"`
	Mergeable   string `json:"mergeable"`
	Repository  struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	} `json:"repository"`
	ReviewDecision string `json:"reviewDecision"`
	Reviews        struct {
		Nodes []struct {
			State string `json:"state"`
		} `json:"nodes"`
	} `json:"reviews"`
	ReviewThreads *graphQLReviewThreads `json:"reviewThreads"`
	Commits       struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string `json:"state"`
					Contexts struct {
						Nodes []graphQLContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// errAccountUnauthorized marks an HTTP 401/403 (non-rate-limit) poll response.
var errAccountUnauthorized = errors.New("GitHub rejected the account's token")

// errAccountRateLimited marks a rate-limited poll response (HTTP or GraphQL).
var errAccountRateLimited = errors.New("GitHub rate limit exceeded")

// accountPollStatus classifies one account's fetch outcome. Detail is a fixed
// short string per state, never the raw error, so it cannot leak a token.
func accountPollStatus(acc connectedAccount, err error) AccountPollStatus {
	st := AccountPollStatus{Host: NormalizeHost(acc.host), AccountLogin: acc.login, State: AccountPollOK}
	switch {
	case err == nil:
	case errors.Is(err, errAccountUnauthorized):
		st.State, st.Detail = AccountPollUnauthorized, "sign-in expired or token rejected"
	case errors.Is(err, errAccountRateLimited):
		st.State, st.Detail = AccountPollRateLimited, "rate limited"
	default:
		st.State, st.Detail = AccountPollError, "poll failed"
	}
	return st
}

// mapFailingChecks returns failing CheckRuns first, then failing legacy
// StatusContexts, capped at maxFailingChecks. Pending/successful ones are dropped.
func mapFailingChecks(nodes []graphQLContext) []FailingCheck {
	var runs, statuses []FailingCheck
	for _, n := range nodes {
		switch n.TypeName {
		case "CheckRun":
			switch strings.ToUpper(n.Conclusion) {
			case "FAILURE", "TIMED_OUT", "STARTUP_FAILURE":
				runs = append(runs, FailingCheck{Name: n.Name, URL: n.DetailsURL, Conclusion: strings.ToLower(n.Conclusion)})
			}
		case "StatusContext":
			switch strings.ToUpper(n.State) {
			case "FAILURE", "ERROR":
				statuses = append(statuses, FailingCheck{Name: n.Context, URL: n.TargetURL, Conclusion: strings.ToLower(n.State)})
			}
		}
	}
	out := append(runs, statuses...)
	if len(out) > maxFailingChecks {
		out = out[:maxFailingChecks]
	}
	return out
}

// countUnresolved counts threads that are neither resolved nor outdated,
// capped at maxReviewThreads. truncated is true when GitHub reports more
// threads than the page fetched, so the true count may be higher.
func countUnresolved(rt *graphQLReviewThreads) (count int, truncated bool) {
	for _, t := range rt.Nodes {
		if !t.IsResolved && !t.IsOutdated {
			count++
		}
	}
	if count > maxReviewThreads {
		count = maxReviewThreads
	}
	return count, rt.TotalCount > maxReviewThreads
}

// mergeConflict maps GitHub's lazily computed mergeable enum to a tri-state;
// UNKNOWN (or absent) is nil so the conflict chip does not flap.
func mergeConflict(mergeable string) *bool {
	var v bool
	switch strings.ToUpper(mergeable) {
	case "CONFLICTING":
		v = true
	case "MERGEABLE":
		v = false
	default:
		return nil
	}
	return &v
}

// isRateLimitedGraphQL reports whether a GraphQL errors array signals rate
// limiting; any other errors-only response means the host rejected the query shape.
func isRateLimitedGraphQL(errs []graphQLError) bool {
	for _, e := range errs {
		if e.Type == "RATE_LIMITED" || strings.Contains(strings.ToLower(e.Message), "rate limit") {
			return true
		}
	}
	return false
}

func graphQLErrorsToError(errs []graphQLError) error {
	if isRateLimitedGraphQL(errs) {
		return errAccountRateLimited
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Message
	}
	return fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
}

// runUserPRQuery posts one query and decodes the envelope. A 401/403 or
// rate-limited response comes back as errAccountUnauthorized/errAccountRateLimited.
func (c *UserPRCache) runUserPRQuery(host, token, query string) (*graphQLResponse, error) {
	body, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL query: %w", err)
	}
	req, err := newGHGraphQLRequestForHostWithToken(c.ctx, host, body, token)
	if err != nil {
		return nil, fmt.Errorf("build GraphQL request: %w", err)
	}
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()
	if isGHRateLimited(resp) {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, errAccountRateLimited
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, errAccountUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GraphQL API returned status %d", resp.StatusCode)
	}
	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("decode GraphQL response: %w", err)
	}
	return &gqlResp, nil
}

// fetchUserPRsForToken polls one account with exactly one request, plus one
// legacy-query retry when the host rejects the widened query (cached per host).
func (c *UserPRCache) fetchUserPRsForToken(host, login, token string) ([]UserPR, error) {
	host = NormalizeHost(host)
	query := userPRGraphQLQuery
	if c.config.DegradedDetails {
		query = userPRGraphQLQueryDegraded
	}
	widened := true
	if _, unsupported := c.widenedUnsupported.Load(host); unsupported {
		query, widened = userPRGraphQLQueryLegacy, false
	}

	gqlResp, err := c.runUserPRQuery(host, token, query)
	if err != nil {
		return nil, err
	}
	if gqlResp.Data == nil && len(gqlResp.Errors) > 0 && widened && !isRateLimitedGraphQL(gqlResp.Errors) {
		c.widenedUnsupported.Store(host, struct{}{})
		log.Warn("UserPRCache: host rejected widened PR query, falling back to legacy", "host", host)
		if gqlResp, err = c.runUserPRQuery(host, token, userPRGraphQLQueryLegacy); err != nil {
			return nil, err
		}
	}
	if gqlResp.Data == nil {
		if len(gqlResp.Errors) > 0 {
			return nil, graphQLErrorsToError(gqlResp.Errors)
		}
		return nil, nil
	}
	if len(gqlResp.Errors) > 0 {
		// Data plus partial errors (e.g. one inaccessible node) is a usable result.
		log.Warn("UserPRCache: GraphQL returned partial errors", "host", host, "count", len(gqlResp.Errors))
	}

	nodes := gqlResp.Data.Viewer.PullRequests.Nodes
	prs := make([]UserPR, 0, len(nodes))
	for _, n := range nodes {
		approved, changesReq := 0, 0
		for _, rv := range n.Reviews.Nodes {
			switch strings.ToUpper(rv.State) {
			case "APPROVED":
				approved++
			case "CHANGES_REQUESTED":
				changesReq++
			}
		}
		checkState := ""
		var contexts []graphQLContext
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup
			checkState = normalizeCheckState(rollup.State)
			contexts = rollup.Contexts.Nodes
		}

		pr := UserPR{
			Owner:            n.Repository.Owner.Login,
			Repo:             n.Repository.Name,
			Number:           n.Number,
			Title:            n.Title,
			URL:              n.URL,
			HeadRef:          n.HeadRefName,
			BaseRef:          n.BaseRefName,
			State:            strings.ToLower(n.State),
			IsDraft:          n.IsDraft,
			UpdatedAt:        parseGitHubTime(n.UpdatedAt),
			ClosedAt:         parseGitHubTime(n.ClosedAt),
			MergedAt:         parseGitHubTime(n.MergedAt),
			ApprovedCount:    approved,
			ChangesReqCount:  changesReq,
			CheckConclusion:  checkState,
			Host:             host,
			AccountLogin:     login,
			FailingChecks:    mapFailingChecks(contexts),
			HasMergeConflict: mergeConflict(n.Mergeable),
		}
		if n.ReviewThreads != nil {
			count, truncated := countUnresolved(n.ReviewThreads)
			pr.UnresolvedThreadCount = &count
			pr.UnresolvedThreadsTruncated = truncated
			pr.DetailsLoaded = true
		}
		prs = append(prs, pr)
	}
	return prs, nil
}

func parseGitHubTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

func normalizeCheckState(s string) string {
	switch strings.ToUpper(s) {
	case "SUCCESS":
		return "success"
	case "FAILURE", "ERROR":
		return "failure"
	case "PENDING", "EXPECTED":
		return "pending"
	default:
		return strings.ToLower(s)
	}
}
