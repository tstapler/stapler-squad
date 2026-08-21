package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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
	CheckConclusion   string // "success" / "failure" / "pending" / ""
	SessionIDs        []string
	LocalWorktreePath string
}

// PRAnnotationSession carries session data needed to annotate UserPR entries.
// Defined here (not in the session package) to avoid an import cycle:
// session imports github, so github cannot import session.
//
// Repo is a typed value object: holding a valid RepoRef proves owner and repo
// are non-empty. Sessions without a resolvable GitHub repo are skipped.
type PRAnnotationSession struct {
	ID       string
	Branch   string
	Repo     RepoRef
	PRNumber int // fallback: match by PR number when branch name doesn't match headRef
}

// PRAnnotationWorktree carries worktree data for annotation.
type PRAnnotationWorktree struct {
	Branch       string
	Repo         RepoRef
	WorktreePath string
}

// userPRSnapshot is an immutable snapshot stored in atomic.Value (COW pattern).
type userPRSnapshot struct {
	prs        []UserPR
	capturedAt time.Time
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
	done         chan struct{} // closed when loop() returns; nil until Start
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

// Annotate enriches the current snapshot with session IDs and worktree paths.
// It performs a COW update: load → copy → mutate → store.
// No-op if the snapshot hasn't been populated yet.
func (c *UserPRCache) Annotate(sessions []PRAnnotationSession, worktrees []PRAnnotationWorktree) {
	v := c.snapshot.Load()
	if v == nil {
		return
	}
	old := v.(*userPRSnapshot)

	// Primary index: keyed by RepoRef.BranchKey(branch) = "owner/branch".
	sessionsByBranch := make(map[string][]string, len(sessions))
	// Secondary index: keyed by RepoRef.PRKey(number) = "owner/#number".
	sessionsByNum := make(map[string][]string, len(sessions))
	for _, s := range sessions {
		if !s.Repo.IsValid() {
			continue
		}
		if s.Branch != "" {
			k := s.Repo.BranchKey(s.Branch)
			sessionsByBranch[k] = append(sessionsByBranch[k], s.ID)
		}
		if s.PRNumber > 0 {
			k := s.Repo.PRKey(s.PRNumber)
			sessionsByNum[k] = append(sessionsByNum[k], s.ID)
		}
	}
	worktreeByKey := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch == "" || !wt.Repo.IsValid() {
			continue
		}
		worktreeByKey[wt.Repo.BranchKey(wt.Branch)] = wt.WorktreePath
	}

	annotated := make([]UserPR, len(old.prs))
	for i, pr := range old.prs {
		branchKey := pr.Owner + "/" + pr.HeadRef
		ids := sessionsByBranch[branchKey]
		if len(ids) == 0 && pr.Number > 0 {
			ids = sessionsByNum[pr.Owner+"/#"+strconv.Itoa(pr.Number)]
		}
		pr.SessionIDs = ids
		pr.LocalWorktreePath = worktreeByKey[branchKey]
		annotated[i] = pr
	}
	c.snapshot.Store(&userPRSnapshot{prs: annotated, capturedAt: old.capturedAt})
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
			prs, fetchErr := c.fetchUserPRsForToken(acc.host, acc.token)
			results[i] = prResult{prs: prs, err: fetchErr}
		}()
	}
	wg.Wait()

	// Merge and dedup by URL (same PR can appear via multiple account tokens, e.g. org members).
	seen := make(map[string]bool)
	var merged []UserPR
	for _, r := range results {
		if r.err != nil {
			log.Warn("UserPRCache: fetch failed for account", "err", r.err)
			continue
		}
		for _, pr := range r.prs {
			if !seen[pr.URL] {
				seen[pr.URL] = true
				merged = append(merged, pr)
			}
		}
	}

	snap := &userPRSnapshot{prs: merged, capturedAt: time.Now()}
	c.snapshot.Store(snap)

	out := make([]UserPR, len(merged))
	copy(out, merged)
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

// userPRGraphQLQuery fetches the authenticated user's open pull requests.
const userPRGraphQLQuery = `
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
        commits(last: 1) {
          nodes {
            commit {
              statusCheckRollup { state }
            }
          }
        }
      }
    }
  }
}`

// graphQLResponse is the top-level GraphQL response envelope.
type graphQLResponse struct {
	Data   *graphQLData   `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type graphQLError struct {
	Message string `json:"message"`
}

type graphQLData struct {
	Viewer struct {
		PullRequests struct {
			Nodes []graphQLPRNode `json:"nodes"`
		} `json:"pullRequests"`
	} `json:"viewer"`
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
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State string `json:"state"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

func (c *UserPRCache) fetchUserPRsForToken(host, token string) ([]UserPR, error) {
	body, err := json.Marshal(map[string]string{"query": userPRGraphQLQuery})
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL query: %w", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, http.MethodPost, graphQLURLForHost(host), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build GraphQL request: %w", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("GraphQL API returned status %d", resp.StatusCode)
	}

	var gqlResp graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&gqlResp); err != nil {
		return nil, fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	if gqlResp.Data == nil {
		return nil, nil
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
		if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
			checkState = normalizeCheckState(n.Commits.Nodes[0].Commit.StatusCheckRollup.State)
		}

		prs = append(prs, UserPR{
			Owner:           n.Repository.Owner.Login,
			Repo:            n.Repository.Name,
			Number:          n.Number,
			Title:           n.Title,
			URL:             n.URL,
			HeadRef:         n.HeadRefName,
			BaseRef:         n.BaseRefName,
			State:           strings.ToLower(n.State),
			IsDraft:         n.IsDraft,
			UpdatedAt:       parseGitHubTime(n.UpdatedAt),
			ClosedAt:        parseGitHubTime(n.ClosedAt),
			MergedAt:        parseGitHubTime(n.MergedAt),
			ApprovedCount:   approved,
			ChangesReqCount: changesReq,
			CheckConclusion: checkState,
		})
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
