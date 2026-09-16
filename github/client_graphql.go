package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// graphQLPRInfoQuery fetches a single pull request's full metadata — the
// native-HTTP/GraphQL equivalent of GetPRInfoCtx's
// `gh pr view --json number,title,...,reviews,reviewDecision,statusCheckRollup`
// shell-out (client.go).
//
// Field/enum shapes here (PullRequest.mergeable, .author, and
// statusCheckRollup's CheckRun/StatusContext union) are based on GitHub's
// publicly documented GraphQL schema, NOT verified via a live
// `gh api graphql` introspection query — Task 4.1.1a's required first
// sub-step could not be run because this environment has no network access
// to GitHub. This is a known gap: per plan.md's Unresolved Questions entry,
// re-run the introspection query against a real token before Epic 4.2 flips
// the flag on by default, and diff its result against this query/the struct
// tags below.
const graphQLPRInfoQuery = `
query PRInfo($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      number
      title
      body
      headRefName
      headRefOid
      baseRefName
      state
      url
      createdAt
      updatedAt
      isDraft
      mergeable
      additions
      deletions
      changedFiles
      author { login }
      labels(first: 100) {
        nodes { name }
      }
      reviewDecision
      reviews(last: 100) {
        nodes {
          author { login }
          state
          body
        }
      }
      commits(last: 1) {
        nodes {
          commit {
            statusCheckRollup {
              contexts(first: 100) {
                nodes {
                  __typename
                  ... on CheckRun {
                    name
                    status
                    conclusion
                  }
                  ... on StatusContext {
                    context
                    state
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

// prInfoGraphQLResponse is the top-level GraphQL response envelope for
// graphQLPRInfoQuery. graphQLError is user_pr_cache.go's existing envelope
// error type — its {"message": "..."} shape is generic across every GraphQL
// query this package issues, so it's reused rather than redefined here.
type prInfoGraphQLResponse struct {
	Data   *prInfoGraphQLData `json:"data"`
	Errors []graphQLError     `json:"errors"`
}

type prInfoGraphQLData struct {
	Repository *struct {
		PullRequest *prInfoGraphQLNode `json:"pullRequest"`
	} `json:"repository"`
}

// prInfoGraphQLNode mirrors ghPRResponse (client.go) field-for-field, but
// sourced from graphQLPRInfoQuery's GraphQL response shape instead of
// `gh pr view --json`'s REST-flavored JSON.
type prInfoGraphQLNode struct {
	Number       int    `json:"number"`
	Title        string `json:"title"`
	Body         string `json:"body"`
	HeadRefName  string `json:"headRefName"`
	HeadRefOid   string `json:"headRefOid"`
	BaseRefName  string `json:"baseRefName"`
	State        string `json:"state"` // OPEN / CLOSED / MERGED
	URL          string `json:"url"`
	CreatedAt    string `json:"createdAt"`
	UpdatedAt    string `json:"updatedAt"`
	IsDraft      bool   `json:"isDraft"`
	Mergeable    string `json:"mergeable"` // MERGEABLE / CONFLICTING / UNKNOWN
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	ChangedFiles int    `json:"changedFiles"`
	Author       *struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []prInfoLabelNode `json:"nodes"`
	} `json:"labels"`
	ReviewDecision string `json:"reviewDecision"` // APPROVED / CHANGES_REQUESTED / REVIEW_REQUIRED / ""
	Reviews        struct {
		Nodes []prInfoReviewNode `json:"nodes"`
	} `json:"reviews"`
	Commits struct {
		Nodes []prInfoCommitNode `json:"nodes"`
	} `json:"commits"`
}

// prInfoCommitNode is one node of PullRequest.commits.nodes — GetPRInfoGraphQL
// only ever requests commits(last: 1), so there is at most one.
type prInfoCommitNode struct {
	Commit struct {
		StatusCheckRollup *struct {
			Contexts struct {
				Nodes []prInfoCheckContextNode `json:"nodes"`
			} `json:"contexts"`
		} `json:"statusCheckRollup"`
	} `json:"commit"`
}

// prInfoLabelNode is one node of PullRequest.labels.nodes.
type prInfoLabelNode struct {
	Name string `json:"name"`
}

// prInfoReviewNode is one node of PullRequest.reviews.nodes.
type prInfoReviewNode struct {
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	State string `json:"state"`
	Body  string `json:"body"`
}

// prInfoCheckContextNode is one node of statusCheckRollup.contexts — a
// GraphQL union of CheckRun and StatusContext. Both shapes' fields land on
// one JSON object tagged by __typename; gh CLI's own `--json
// statusCheckRollup` flattens the identical union the same way into
// ghStatusCheckItem (client.go), which is why the two struct's fields line
// up one-for-one (Name/Status/Conclusion for CheckRun,
// Context/State for StatusContext).
type prInfoCheckContextNode struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`       // CheckRun only
	Status     string `json:"status"`     // CheckRun only: QUEUED/IN_PROGRESS/COMPLETED/...
	Conclusion string `json:"conclusion"` // CheckRun only: SUCCESS/FAILURE/NEUTRAL/...
	Context    string `json:"context"`    // StatusContext only
	State      string `json:"state"`      // StatusContext only: SUCCESS/FAILURE/PENDING/ERROR
}

// callSitePRViewGraphQL is GetPRInfoGraphQL's call-site label, read by
// githubTelemetryTransport.RoundTrip (telemetry_transport.go) via
// GitHubCallSiteFrom to populate plan.md's Observability Plan `call_site`
// label on the native HTTP path, the same way runGHCLICommand (gh_exec.go)
// does for the gh-CLI path.
const callSitePRViewGraphQL = "pr.view.graphql"

// GetPRInfoGraphQL fetches metadata for a pull request via GitHub's GraphQL
// API instead of GetPRInfoCtx's `gh pr view` subprocess shell-out, producing
// a field-identical *PRInfo (verified by the parity test in
// client_graphql_test.go). It issues one POST through ghHTTPClient (see
// http_client.go), so it automatically gets the same rate-limit admission
// control and OTel telemetry (githubTelemetryTransport) every other native
// GitHub call gets — no separate instrumentation call needed here.
//
// Unlike GetPRInfoCtx, this does not call CheckGHAuth() first: none of this
// package's other native-HTTP call sites (GetPRForBranch, GetPRByNumber) do
// either — they dispatch the request and let classifyGHResponse turn a
// 401/403 into an error, which is what this function does too. CheckGHAuth's
// pre-flight check exists specifically to give the `gh` subprocess path a
// clearer error than a raw exec failure would; that rationale doesn't apply
// to a direct HTTP call.
//
// This function is wired: GetPRInfoCtx (client.go) dispatches here when the
// github:graphql-pr-info feature flag (githubGraphQLMigrationFlagName) is
// enabled — see that constant's doc comment for the flag's default-off
// rationale.
func GetPRInfoGraphQL(ctx context.Context, owner, repo string, prNumber int) (*PRInfo, error) {
	ctx = WithGitHubCallSite(ctx, callSitePRViewGraphQL)

	reqBody, err := json.Marshal(map[string]any{
		"query": graphQLPRInfoQuery,
		"variables": map[string]any{
			"owner":  owner,
			"repo":   repo,
			"number": prNumber,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal GraphQL query: %w", err)
	}

	req, err := newGHGraphQLRequest(ctx, "", reqBody)
	if err != nil {
		return nil, fmt.Errorf("build GraphQL request: %w", err)
	}

	resp, err := ghHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GraphQL request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, classifyGHResponse(resp, "", false)
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read GraphQL response: %w", err)
	}

	return decodePRInfoGraphQLResponse(respBody, owner, repo, prNumber)
}

// decodePRInfoGraphQLResponse decodes and field-maps a graphQLPRInfoQuery
// response body into a *PRInfo. Split out from GetPRInfoGraphQL so the
// parity test (client_graphql_test.go) can exercise the decode/mapping logic
// directly against hand-constructed fixture bytes, without a live HTTP call.
func decodePRInfoGraphQLResponse(body []byte, owner, repo string, prNumber int) (*PRInfo, error) {
	var gqlResp prInfoGraphQLResponse
	if err := json.Unmarshal(body, &gqlResp); err != nil {
		return nil, fmt.Errorf("decode GraphQL response: %w", err)
	}
	if len(gqlResp.Errors) > 0 {
		msgs := make([]string, len(gqlResp.Errors))
		for i, e := range gqlResp.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	if gqlResp.Data == nil || gqlResp.Data.Repository == nil || gqlResp.Data.Repository.PullRequest == nil {
		return nil, fmt.Errorf("%w: PR #%d in %s/%s", ErrNoPR, prNumber, owner, repo)
	}

	return mapPRInfoGraphQLNode(gqlResp.Data.Repository.PullRequest), nil
}

// mapPRInfoGraphQLNode field-maps one decoded GraphQL PullRequest node into
// *PRInfo, mirroring GetPRInfoCtx's construction (client.go) field-for-field
// — including reusing parseReviewCounts and getCheckConclusion so both paths
// derive ApprovedCount/ChangesRequestedCount/CheckConclusion/CheckStatus
// identically.
//
// Mergeable is passed through unchanged (GraphQL's raw "MERGEABLE" /
// "CONFLICTING" / "UNKNOWN"), not lowercased: GetPRInfoCtx's ghPRResponse
// decode (client.go) does not transform resp.Mergeable either — unlike
// State/ReviewDecision, gh-CLI's own `--json mergeable` output is the raw,
// uppercase GraphQL enum string. Lowercasing it here (as an earlier reading
// of this task's field-mapping table assumed) would break field-by-field
// parity with the actual gh-CLI path.
func mapPRInfoGraphQLNode(n *prInfoGraphQLNode) *PRInfo {
	createdAt, _ := time.Parse(time.RFC3339, n.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, n.UpdatedAt)

	author := ""
	if n.Author != nil {
		author = n.Author.Login
	}

	reviews, approvedCount, changesReqCount := mapPRInfoGraphQLReviews(n.Reviews.Nodes)
	checks, checkConclusion, checkStatus := mapPRInfoGraphQLChecks(n.Commits.Nodes)

	return &PRInfo{
		Number:                n.Number,
		Title:                 n.Title,
		Body:                  n.Body,
		HeadRef:               n.HeadRefName,
		HeadSHA:               n.HeadRefOid,
		BaseRef:               n.BaseRefName,
		State:                 strings.ToLower(n.State),
		Author:                author,
		Labels:                mapPRInfoGraphQLLabels(n.Labels.Nodes),
		HTMLURL:               n.URL,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		IsDraft:               n.IsDraft,
		Mergeable:             n.Mergeable,
		Additions:             n.Additions,
		Deletions:             n.Deletions,
		ChangedFiles:          n.ChangedFiles,
		ReviewDecision:        strings.ToLower(n.ReviewDecision),
		ApprovedCount:         approvedCount,
		ChangesRequestedCount: changesReqCount,
		CheckConclusion:       checkConclusion,
		CheckStatus:           checkStatus,
		Checks:                checks,
		Reviews:               reviews,
	}
}

// mapPRInfoGraphQLLabels extracts label names, always returning a non-nil
// (possibly zero-length) slice to match ghPRResponse's decode path
// (client.go), which likewise always allocates via
// make([]string, len(resp.Labels)) rather than leaving a nil slice on no
// labels — field-by-field equality in the parity test would otherwise fail
// on a nil-vs-empty-slice mismatch.
func mapPRInfoGraphQLLabels(nodes []prInfoLabelNode) []string {
	labels := make([]string, len(nodes))
	for i, l := range nodes {
		labels[i] = l.Name
	}
	return labels
}

// mapPRInfoGraphQLReviews field-maps review nodes into ReviewItem (always a
// non-nil slice, mirroring GetPRInfoCtx's make([]ReviewItem, len(resp.Reviews))
// — see mapPRInfoGraphQLLabels's doc comment for why that matters) and
// derives approved/changes-requested counts via the same parseReviewCounts
// (client.go) GetPRInfoCtx uses, by first reshaping into ghReviewItem.
func mapPRInfoGraphQLReviews(nodes []prInfoReviewNode) (reviews []ReviewItem, approved, changesRequested int) {
	reviews = make([]ReviewItem, len(nodes))
	ghReviews := make([]ghReviewItem, len(nodes))
	for i, r := range nodes {
		login := ""
		if r.Author != nil {
			login = r.Author.Login
		}
		reviews[i] = ReviewItem{Author: login, State: r.State, Body: r.Body}
		ghReviews[i].Author.Login = login
		ghReviews[i].State = r.State
		ghReviews[i].Body = r.Body
	}
	approved, changesRequested = parseReviewCounts(ghReviews)
	return reviews, approved, changesRequested
}

// mapPRInfoGraphQLChecks field-maps commits(last: 1)'s single
// statusCheckRollup.contexts into CheckItem (always a non-nil slice — see
// mapPRInfoGraphQLLabels's doc comment) and derives CheckConclusion/CheckStatus
// via the same getCheckConclusion (client.go) GetPRInfoCtx uses, by first
// reshaping into ghStatusCheckItem — the shared shape both the gh-CLI and
// GraphQL check-context union flatten onto (see prInfoCheckContextNode's doc
// comment).
func mapPRInfoGraphQLChecks(commitNodes []prInfoCommitNode) (checks []CheckItem, conclusion, status string) {
	checks = make([]CheckItem, 0)
	var ghChecks []ghStatusCheckItem
	if len(commitNodes) > 0 {
		if rollup := commitNodes[0].Commit.StatusCheckRollup; rollup != nil {
			contexts := rollup.Contexts.Nodes
			checks = make([]CheckItem, len(contexts))
			ghChecks = make([]ghStatusCheckItem, len(contexts))
			for i, c := range contexts {
				item := ghStatusCheckItem{
					Name:       c.Name,
					Context:    c.Context,
					State:      c.State,
					Status:     c.Status,
					Conclusion: c.Conclusion,
				}
				ghChecks[i] = item
				checks[i] = CheckItem(item)
			}
		}
	}
	conclusion, status = getCheckConclusion(ghChecks)
	return checks, conclusion, status
}
