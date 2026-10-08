package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tstapler/stapler-squad/log"
)

// ErrRateLimited is returned when GitHub (or the local limiter, which already
// knows the token is limited) refuses the call; callers should retry later.
var ErrRateLimited = errors.New("github: rate limited")

// PRThreadRef is one unresolved review thread, reduced to what a nudge may
// quote: who, where and a link. The comment body is never fetched.
type PRThreadRef struct {
	AuthorLogin string // untrusted
	Path        string // untrusted
	URL         string // untrusted
}

// PRNudgeDetail is fresh PR state for building a nudge prompt. Every string
// field originates from GitHub and is untrusted until sanitized.
type PRNudgeDetail struct {
	Key               PRKey
	URL               string
	State             string // "open", "closed" or "merged"
	IsDraft           bool
	HeadRef           string
	IsCrossRepository bool
	FailingChecks     []FailingCheck // at most maxFailingChecks, CheckRuns first
	// MoreChecksUnseen is true when the PR has more check contexts than the 100
	// read, so FailingChecks may be incomplete.
	MoreChecksUnseen  bool
	UnresolvedThreads []PRThreadRef // not resolved, not outdated; first page only (20 threads)
	// MoreThreadsUnseen is true when GitHub holds more threads than the 20 read,
	// so the unresolved list may be incomplete.
	MoreThreadsUnseen bool
	HasMergeConflict  *bool // nil = GitHub mergeable UNKNOWN
}

// PRDetailFetcher fetches PRNudgeDetail with an explicit token.
type PRDetailFetcher interface {
	FetchPRNudgeDetail(ctx context.Context, key PRKey, token string) (PRNudgeDetail, error)
}

// GraphQLPRDetailFetcher is the production PRDetailFetcher.
type GraphQLPRDetailFetcher struct{}

// FetchPRNudgeDetail implements PRDetailFetcher.
func (GraphQLPRDetailFetcher) FetchPRNudgeDetail(ctx context.Context, key PRKey, token string) (PRNudgeDetail, error) {
	return FetchPRNudgeDetail(ctx, key, token)
}

// prNudgeDetailQuery asks only for fields a nudge may use. Review-thread
// comments request author, url and path and nothing else: third-party comment
// text must never reach a prompt, so it is never fetched.
const prNudgeDetailQuery = `query($owner:String!,$repo:String!,$number:Int!){
repository(owner:$owner,name:$repo){pullRequest(number:$number){
url state isDraft headRefName isCrossRepository mergeable
commits(last:1){nodes{commit{statusCheckRollup{state contexts(first:100){totalCount nodes{
__typename
... on CheckRun{name conclusion detailsUrl}
... on StatusContext{context state targetUrl}
}}}}}}
reviewThreads(first:20){totalCount nodes{isResolved isOutdated comments(first:1){nodes{author{login} url path}}}}
}}}`

type prNudgeDetailResponse struct {
	Data *struct {
		Repository *struct {
			PullRequest *prNudgeNode `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []graphQLError `json:"errors"`
}

type prNudgeNode struct {
	URL               string `json:"url"`
	State             string `json:"state"`
	IsDraft           bool   `json:"isDraft"`
	HeadRefName       string `json:"headRefName"`
	IsCrossRepository bool   `json:"isCrossRepository"`
	Mergeable         string `json:"mergeable"`
	Commits           struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						TotalCount int              `json:"totalCount"`
						Nodes      []graphQLContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
	ReviewThreads struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			IsResolved bool `json:"isResolved"`
			IsOutdated bool `json:"isOutdated"`
			Comments   struct {
				Nodes []struct {
					Author *struct {
						Login string `json:"login"`
					} `json:"author"`
					URL  string `json:"url"`
					Path string `json:"path"`
				} `json:"nodes"`
			} `json:"comments"`
		} `json:"nodes"`
	} `json:"reviewThreads"`
}

// FetchPRNudgeDetail reads current PR state with token, the token of the
// account that owns the PR. It never looks up a host-default token and never
// logs or returns the token. A missing or inaccessible PR (including 401/403,
// since a lost-access token must not fall back to another account) wraps
// ErrGitHubRefNotFound; limiting wraps ErrRateLimited.
func FetchPRNudgeDetail(ctx context.Context, key PRKey, token string) (PRNudgeDetail, error) {
	if !key.IsValid() {
		return PRNudgeDetail{}, errors.New("github: invalid PR key")
	}
	if token == "" {
		return PRNudgeDetail{}, errors.New("github: empty token for PR detail fetch")
	}
	if limited, _ := DefaultRateLimiter.IsLimited(); limited {
		return PRNudgeDetail{}, ErrRateLimited
	}
	body, err := json.Marshal(map[string]any{
		"query":     prNudgeDetailQuery,
		"variables": map[string]any{"owner": key.Owner(), "repo": key.RepoName(), "number": key.Number()},
	})
	if err != nil {
		return PRNudgeDetail{}, fmt.Errorf("marshal PR detail query: %w", err)
	}
	ctx = WithGitHubCallOrigin(ctx, OriginInteractive)
	req, err := newGHGraphQLRequestForHostWithToken(ctx, key.Host(), body, token)
	if err != nil {
		return PRNudgeDetail{}, fmt.Errorf("build PR detail request: %w", err)
	}
	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		if limited, _ := DefaultRateLimiter.IsLimited(); limited {
			return PRNudgeDetail{}, ErrRateLimited
		}
		return PRNudgeDetail{}, fmt.Errorf("PR detail request failed: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case isGHRateLimited(resp):
		_, _ = io.Copy(io.Discard, resp.Body)
		return PRNudgeDetail{}, ErrRateLimited
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		_, _ = io.Copy(io.Discard, resp.Body)
		log.Warn("PR nudge detail: owning account denied", "host", key.Host(), "status", resp.StatusCode)
		return PRNudgeDetail{}, fmt.Errorf("%w: access denied (status %d)", ErrGitHubRefNotFound, resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, resp.Body)
		return PRNudgeDetail{}, fmt.Errorf("PR detail query returned status %d", resp.StatusCode)
	}

	var out prNudgeDetailResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return PRNudgeDetail{}, fmt.Errorf("decode PR detail response: %w", err)
	}
	node := (*prNudgeNode)(nil)
	if out.Data != nil && out.Data.Repository != nil {
		node = out.Data.Repository.PullRequest
	}
	if isRateLimitedGraphQL(out.Errors) {
		return PRNudgeDetail{}, ErrRateLimited
	}
	if node == nil {
		// Only an explicit NOT_FOUND (or no errors at all) means the PR is gone;
		// anything else (SSO, scope, outage) must not read as "PR not found".
		if len(out.Errors) == 0 || hasGraphQLErrorType(out.Errors, "NOT_FOUND") {
			return PRNudgeDetail{}, fmt.Errorf("%w: %s", ErrGitHubRefNotFound, key)
		}
		return PRNudgeDetail{}, fmt.Errorf("PR detail query failed: %w", graphQLErrorsToError(out.Errors))
	}
	if len(out.Errors) > 0 {
		// A partial response may have dropped checks or threads; nudging from it
		// would silently omit reasons.
		return PRNudgeDetail{}, fmt.Errorf("PR detail query returned partial data: %w", graphQLErrorsToError(out.Errors))
	}
	log.Debug("PR nudge detail fetched", "host", key.Host(), "pr", key.String())
	return mapPRNudgeDetail(key, node), nil
}

func mapPRNudgeDetail(key PRKey, n *prNudgeNode) PRNudgeDetail {
	d := PRNudgeDetail{
		Key:               key,
		URL:               n.URL,
		State:             strings.ToLower(n.State),
		IsDraft:           n.IsDraft,
		HeadRef:           n.HeadRefName,
		IsCrossRepository: n.IsCrossRepository,
		HasMergeConflict:  mergeConflict(n.Mergeable),
		MoreThreadsUnseen: n.ReviewThreads.TotalCount > len(n.ReviewThreads.Nodes),
	}
	if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		ctxs := n.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts
		d.FailingChecks = mapFailingChecks(ctxs.Nodes)
		d.MoreChecksUnseen = ctxs.TotalCount > len(ctxs.Nodes)
	}
	for _, t := range n.ReviewThreads.Nodes {
		if t.IsResolved || t.IsOutdated {
			continue
		}
		ref := PRThreadRef{}
		if len(t.Comments.Nodes) > 0 {
			c := t.Comments.Nodes[0]
			ref.URL, ref.Path = c.URL, c.Path
			if c.Author != nil {
				ref.AuthorLogin = c.Author.Login
			}
		}
		d.UnresolvedThreads = append(d.UnresolvedThreads, ref)
	}
	return d
}

func hasGraphQLErrorType(errs []graphQLError, typ string) bool {
	for _, e := range errs {
		if e.Type == typ {
			return true
		}
	}
	return false
}
