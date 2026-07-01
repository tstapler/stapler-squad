package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	Owner            string
	Repo             string
	Number           int
	Title            string
	URL              string
	HeadRef          string
	BaseRef          string
	State            string
	IsDraft          bool
	UpdatedAt        time.Time
	ClosedAt         time.Time
	MergedAt         time.Time
	ApprovedCount    int
	ChangesReqCount  int
	CheckConclusion  string // "success" / "failure" / "pending" / ""
	SessionIDs       []string
	LocalWorktreePath string
}

// PRAnnotationSession carries session data needed to annotate UserPR entries.
// Defined here (not in the session package) to avoid an import cycle:
// session imports github, so github cannot import session.
type PRAnnotationSession struct {
	ID           string
	Branch       string
	GitHubOwner  string
	WorktreePath string
}

// PRAnnotationWorktree carries worktree data for annotation.
type PRAnnotationWorktree struct {
	Branch       string
	GitHubOwner  string
	WorktreePath string
}

// userPRSnapshot is an immutable snapshot stored in atomic.Value (COW pattern).
type userPRSnapshot struct {
	prs        []UserPR
	capturedAt time.Time
}

// loginResult is an immutable auth state stored in atomic.Value.
type loginResult struct {
	login     string
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
	snapshot     atomic.Value // stores *userPRSnapshot
	subscribers  sync.Map     // maps string ID → chan []UserPR
	onUpdated    atomic.Value // stores onUpdatedFn
	cachedLogin  atomic.Value // stores string
	loginState   atomic.Value // stores loginResult
	loginGroup   singleflight.Group //nolint:exhaustruct
	refreshGroup singleflight.Group //nolint:exhaustruct
	ctx          context.Context
	cancel       context.CancelFunc
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

// Start launches the background polling goroutine. Call once at server startup.
func (c *UserPRCache) Start(ctx context.Context) {
	c.ctx, c.cancel = context.WithCancel(ctx)
	go c.loop()
}

// Stop halts background polling.
func (c *UserPRCache) Stop() {
	c.cancel()
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

// GetCachedLogin returns the last successfully fetched GitHub login, or "" if not yet available.
func (c *UserPRCache) GetCachedLogin() string {
	v := c.cachedLogin.Load()
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
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

	// Build lookup maps keyed by "owner/branch".
	sessionsByKey := make(map[string][]string, len(sessions))
	for _, s := range sessions {
		if s.Branch == "" || s.GitHubOwner == "" {
			continue
		}
		key := s.GitHubOwner + "/" + s.Branch
		sessionsByKey[key] = append(sessionsByKey[key], s.ID)
	}
	worktreeByKey := make(map[string]string, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch == "" || wt.GitHubOwner == "" {
			continue
		}
		worktreeByKey[wt.GitHubOwner+"/"+wt.Branch] = wt.WorktreePath
	}

	annotated := make([]UserPR, len(old.prs))
	for i, pr := range old.prs {
		key := pr.Owner + "/" + pr.HeadRef
		pr.SessionIDs = sessionsByKey[key]
		pr.LocalWorktreePath = worktreeByKey[key]
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

// loop is the background polling goroutine.
func (c *UserPRCache) loop() {
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

// fetch queries the GitHub GraphQL API for the authenticated user's open PRs.
func (c *UserPRCache) fetch() error {
	login, err := c.resolveLogin()
	if err != nil {
		log.Debug("UserPRCache: skipping fetch, no GitHub auth", "err", err)
		return err
	}
	if login == "" {
		return nil
	}

	prs, err := c.fetchUserPRs()
	if err != nil {
		return err
	}

	snap := &userPRSnapshot{prs: prs, capturedAt: time.Now()}
	c.snapshot.Store(snap)

	out := make([]UserPR, len(prs))
	copy(out, prs)
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

// resolveLogin returns the cached GitHub login, refreshing if stale.
func (c *UserPRCache) resolveLogin() (string, error) {
	if v := c.loginState.Load(); v != nil {
		r := v.(loginResult)
		if time.Since(r.checkedAt) < c.config.LoginCacheTTL {
			return r.login, nil
		}
	}

	res, err, _ := c.loginGroup.Do("login", func() (interface{}, error) {
		ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		defer cancel()
		login, fetchErr := GetCurrentUserLogin(ctx)
		c.loginState.Store(loginResult{login: login, checkedAt: time.Now()})
		if fetchErr == nil && login != "" {
			c.cachedLogin.Store(login)
		}
		return login, fetchErr
	})
	if err != nil {
		return "", err
	}
	if res == nil {
		return "", nil
	}
	return res.(string), nil
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
		Owner struct{ Login string `json:"login"` } `json:"owner"`
		Name  string                                `json:"name"`
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

func (c *UserPRCache) fetchUserPRs() ([]UserPR, error) {
	body, err := json.Marshal(map[string]string{"query": userPRGraphQLQuery})
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL query: %w", err)
	}

	req, err := newGHPostRequest(c.ctx, "graphql", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build GraphQL request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()
	if backoff := checkRateLimitHeaders(resp); backoff > 0 {
		time.Sleep(backoff)
	}

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
