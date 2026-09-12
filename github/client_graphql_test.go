package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetPRInfoGraphQL_should_PopulateFieldsMatchingCLI_When_PRHasApprovalsAndPassingChecks
// is the plan's Story 4.1.1 happy-path unit test (validation.md's REQ-3 row):
// GetPRInfoGraphQL decodes a single representative GraphQL response
// (approvals + passing checks) into the expected *PRInfo fields.
func TestGetPRInfoGraphQL_should_PopulateFieldsMatchingCLI_When_PRHasApprovalsAndPassingChecks(t *testing.T) {
	body := loadFixture(t, "approved_passing_checks", "graphql.json")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/graphql", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	info, err := GetPRInfoGraphQL(context.Background(), "tstapler", "stapler-squad", 802)
	require.NoError(t, err)

	assert.Equal(t, 802, info.Number)
	assert.Equal(t, PRStateOpen, info.State)
	assert.Equal(t, "tstapler", info.Author)
	assert.Equal(t, "approved", info.ReviewDecision)
	assert.Equal(t, 2, info.ApprovedCount)
	assert.Equal(t, 0, info.ChangesRequestedCount)
	assert.Equal(t, "success", info.CheckConclusion)
	assert.Equal(t, "completed", info.CheckStatus)
	assert.Equal(t, "MERGEABLE", info.Mergeable)
	assert.Equal(t, []string{"enhancement", "backend"}, info.Labels)
}

// TestGetPRInfoGraphQL_should_ReturnError_When_GraphQLResponseContainsErrorsArray
// covers validation.md's REQ-3 error-path row: GitHub's GraphQL {"errors": [...]}
// envelope (returned with HTTP 200) must surface as a Go error, not a
// zero-value *PRInfo.
func TestGetPRInfoGraphQL_should_ReturnError_When_GraphQLResponseContainsErrorsArray(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":null,"errors":[{"message":"Could not resolve to a Repository"}]}`))
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	info, err := GetPRInfoGraphQL(context.Background(), "tstapler", "does-not-exist", 1)
	require.Error(t, err)
	assert.Nil(t, info)
	assert.Contains(t, err.Error(), "Could not resolve to a Repository")
}

// TestGetPRInfoGraphQL_should_MatchCLIFieldByField_When_PRInStateVariant is
// Story 4.1.2's parity test (Task 4.1.2b): for each fixture pair under
// testdata/pr_info_parity/, decoding the gh-CLI JSON via the same
// construction GetPRInfoCtx (client.go) uses and decoding the GraphQL
// response via decodePRInfoGraphQLResponse must produce field-identical
// *PRInfo structs. This exercises the two decode/mapping paths against
// hand-constructed fixtures representing the same PR state — not a live
// network call (Task 4.1.2a's fixtures were hand-built, not recorded, since
// this environment has no network access to GitHub; see
// client_graphql.go's package doc comment on graphQLPRInfoQuery for the same
// caveat on the introspection step this fixture data stands in for).
//
// Per the coordinator's explicit scope-down of Task 4.1.2a to "2-3
// representative PRs (draft, approved, CI-failing)", this covers those three
// cases rather than validation.md's full 11-case widened parity matrix
// (draft/approved/CI-failing plus every DerivePRPriority branch value) — see
// this epic's final report for that gap.
func TestGetPRInfoGraphQL_should_MatchCLIFieldByField_When_PRInStateVariant(t *testing.T) {
	cases := []string{
		"draft_in_progress_checks",
		"approved_passing_checks",
		"checks_failure",
	}

	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			cliBody := loadFixture(t, name, "cli.json")
			graphqlBody := loadFixture(t, name, "graphql.json")

			var cliResp ghPRResponse
			require.NoError(t, json.Unmarshal(cliBody, &cliResp))
			cliInfo := cliResponseToPRInfo(&cliResp)

			graphqlInfo, err := decodePRInfoGraphQLResponse(graphqlBody, "tstapler", "stapler-squad", cliResp.Number)
			require.NoError(t, err)

			assert.Equal(t, cliInfo, graphqlInfo, "GetPRInfoGraphQL's decode must match GetPRInfoCtx's gh-CLI decode field-by-field for fixture %q", name)
		})
	}
}

// cliResponseToPRInfo mirrors GetPRInfoCtx's *PRInfo construction
// (client.go, lines building the return value from a decoded ghPRResponse)
// so the parity test above can exercise that same field-mapping logic
// against fixture bytes without going through GetPRInfoCtx's `gh` subprocess
// shell-out (which this epic's file-scope constraints, and the coordinator's
// instruction not to touch client.go, both preclude calling directly here).
// This duplicates client.go's mapping block deliberately — extracting a
// shared helper would mean editing client.go, out of this epic's scope; see
// the epic's final report, which flags this duplication as follow-up for
// whoever lands Epic 4.2 and touches client.go anyway.
func cliResponseToPRInfo(resp *ghPRResponse) *PRInfo {
	createdAt, _ := time.Parse(time.RFC3339, resp.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, resp.UpdatedAt)

	labels := make([]string, len(resp.Labels))
	for i, label := range resp.Labels {
		labels[i] = label.Name
	}

	approvedCount, changesReqCount := parseReviewCounts(resp.Reviews)
	checkConclusion, checkStatus := getCheckConclusion(resp.StatusCheckRollup)

	checks := make([]CheckItem, len(resp.StatusCheckRollup))
	for i, c := range resp.StatusCheckRollup {
		checks[i] = CheckItem(c)
	}
	reviews := make([]ReviewItem, len(resp.Reviews))
	for i, r := range resp.Reviews {
		reviews[i] = ReviewItem{Author: r.Author.Login, State: r.State, Body: r.Body}
	}

	return &PRInfo{
		Number:                resp.Number,
		Title:                 resp.Title,
		Body:                  resp.Body,
		HeadRef:               resp.HeadRefName,
		HeadSHA:               resp.HeadRefOid,
		BaseRef:               resp.BaseRefName,
		State:                 strings.ToLower(resp.State),
		Author:                resp.Author.Login,
		Labels:                labels,
		HTMLURL:               resp.URL,
		CreatedAt:             createdAt,
		UpdatedAt:             updatedAt,
		IsDraft:               resp.IsDraft,
		Mergeable:             resp.Mergeable,
		Additions:             resp.Additions,
		Deletions:             resp.Deletions,
		ChangedFiles:          resp.ChangedFiles,
		ReviewDecision:        strings.ToLower(resp.ReviewDecision),
		ApprovedCount:         approvedCount,
		ChangesRequestedCount: changesReqCount,
		CheckConclusion:       checkConclusion,
		CheckStatus:           checkStatus,
		Checks:                checks,
		Reviews:               reviews,
	}
}

// loadFixture reads testdata/pr_info_parity/<caseName>/<fileName>.
func loadFixture(t *testing.T, caseName, fileName string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "pr_info_parity", caseName, fileName))
	require.NoError(t, err)
	return body
}
