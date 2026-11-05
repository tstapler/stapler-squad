package services

import (
	"encoding/json"
	"net/http"
	"strings"
)

// fakeGitHubEnterpriseHandler serves both REST (GET /user) and GraphQL
// (POST /api/graphql) requests that UserPRCache.fetch fires when resolving
// and refreshing an account: /user login-checks, /api/graphql fetches open
// PRs. Without a GraphQL responder, a test that only mocks /user leaves the
// GraphQL call to fall through to whatever graphQLURLForHost resolves to —
// this is what regressed into the real, unmocked github.com dial that hung
// CI (see graphQLURLForHost's EnterpriseBaseURLOverride fix in
// github/hosts.go).
func fakeGitHubEnterpriseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if strings.Contains(r.URL.Path, "graphql") {
		_, _ = w.Write([]byte(`{"data":{"viewer":{"pullRequests":{"nodes":[]}}}}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"login": "octocat"})
}

// ─── SearchGitHubRepos ────────────────────────────────────────────────────────

// ─── ListGitHubIssues ─────────────────────────────────────────────────────────

// ─── ImportGitHubIssue ────────────────────────────────────────────────────────

// ─── GetSessionBacklogIndex ───────────────────────────────────────────────────
