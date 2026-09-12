package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

// setGraphQLMigrationFlagForTest flips githubGraphQLMigrationFlagName to value
// for the duration of t, restoring its prior persisted value on cleanup —
// same pattern as http_client_test.go's setAdmissionFlagForTest, needed
// because tests share IsTestMode()'s per-process config dir across the whole
// package binary.
func setGraphQLMigrationFlagForTest(t *testing.T, value bool) {
	t.Helper()
	cfg := config.LoadConfig()
	prev := cfg.GetFeatureFlag(githubGraphQLMigrationFlagName)
	if err := cfg.SetFeatureFlag(githubGraphQLMigrationFlagName, value); err != nil {
		t.Fatalf("SetFeatureFlag(%q, %v) failed: %v", githubGraphQLMigrationFlagName, value, err)
	}
	t.Cleanup(func() {
		if err := config.LoadConfig().SetFeatureFlag(githubGraphQLMigrationFlagName, prev); err != nil {
			t.Errorf("cleanup: failed to restore %q to %v: %v", githubGraphQLMigrationFlagName, prev, err)
		}
	})
}

// TestGithubGraphQLMigrationFlagName_MatchesServerServicesDuplicate is a
// code-review MAJOR's cheap safety net: server/services/feature_flag_service.go's
// knownFeatureFlags registers this same literal under its own
// githubGraphQLMigrationFlagName constant (github cannot import
// server/services — a cycle). This test hardcodes that literal so a rename
// on either side without the other breaks CI here instead of silently
// diverging; see this package's githubGraphQLMigrationFlagName doc comment
// above for the full cross-package rationale.
func TestGithubGraphQLMigrationFlagName_MatchesServerServicesDuplicate(t *testing.T) {
	const serverServicesFlagName = "github:graphql-pr-info" // server/services/feature_flag_service.go:53
	if githubGraphQLMigrationFlagName != serverServicesFlagName {
		t.Fatalf("githubGraphQLMigrationFlagName = %q, want %q to match server/services/feature_flag_service.go's constant", githubGraphQLMigrationFlagName, serverServicesFlagName)
	}
}

// TestGetPRInfoCtx_should_UseUnchangedCLIPath_When_FlagOff is plan Task
// 4.2.1c's flag-off regression case (validation.md REQ-3 row): with
// githubGraphQLMigrationFlagName off (its default), GetPRInfoCtx must still
// run the exact same CheckGHAuth + `gh pr view` shell-out it always has —
// zero behavior change. Proven two ways: the decoded result matches the fake
// `gh` binary's JSON (installFakeGHForTest, client_pr_by_number_test.go), and
// a "gh.pr.view" span is emitted (runGHCLICommand, gh_exec.go) — the CLI path
// was actually invoked, not merely a coincidentally-matching result.
func TestGetPRInfoCtx_should_UseUnchangedCLIPath_When_FlagOff(t *testing.T) {
	setGraphQLMigrationFlagForTest(t, false)
	sr := withTestTracerProvider(t)
	stubGHAuthForTest(t)
	installFakeGHForTest(t, `{
		"number": 704,
		"title": "CLI path PR",
		"headRefName": "feature/x",
		"headRefOid": "abc123",
		"baseRefName": "main",
		"state": "open",
		"url": "https://github.com/tstapler/stapler-squad/pull/704",
		"createdAt": "2026-08-20T12:00:00Z",
		"updatedAt": "2026-08-21T12:00:00Z",
		"isDraft": false,
		"mergeable": "MERGEABLE",
		"additions": 1,
		"deletions": 1,
		"changedFiles": 1,
		"author": {"login": "carol"},
		"labels": [],
		"reviews": [],
		"statusCheckRollup": []
	}`)

	info, err := GetPRInfoCtx(context.Background(), tstaplerSquadRef(), 704)
	require.NoError(t, err)
	assert.Equal(t, 704, info.Number)
	assert.Equal(t, "CLI path PR", info.Title)

	var sawPRViewSpan bool
	for _, span := range sr.Ended() {
		if span.Name() == "gh.pr.view" {
			sawPRViewSpan = true
		}
	}
	assert.True(t, sawPRViewSpan, "expected a gh.pr.view span from runGHCLICommand, proving the CLI path was invoked")
}

// TestGetPRInfoCtx_should_DispatchToGraphQL_When_FlagOn is plan Task 4.2.1c's
// flag-on case: with githubGraphQLMigrationFlagName on, GetPRInfoCtx must
// return GetPRInfoGraphQL's result directly, without ever calling
// CheckGHAuth or shelling out to `gh` — verified by decoding a real GraphQL
// fixture through the native-HTTP path (no fake `gh` binary is installed for
// this test, so a CLI dispatch would fail rather than silently pass).
func TestGetPRInfoCtx_should_DispatchToGraphQL_When_FlagOn(t *testing.T) {
	setGraphQLMigrationFlagForTest(t, true)
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

	info, err := GetPRInfoCtx(context.Background(), tstaplerSquadRef(), 802)
	require.NoError(t, err)
	assert.Equal(t, 802, info.Number)
	assert.Equal(t, "approved", info.ReviewDecision)
}

// TestCheckGHAuth_JoinerUnaffectedByLeaderContextCancellation is the
// regression test for a bug ctx-threading introduced: ghAuthGroup coalesces
// every concurrent CheckGHAuth caller process-wide onto whichever one's Do()
// call started the in-flight request first (the "leader"). Deriving that
// shared request's context straight from the leader's own ctx meant the
// leader's context canceling (e.g. its own RPC/test returning) canceled the
// one shared request out from under every other still-waiting "joiner" too
// — surfacing as a spurious "context canceled" auth failure for callers
// whose own context was never canceled (observed in CI:
// TestWorktreePRPoller_InvalidateCache_NextFetchIsCacheMiss flaking with
// exactly this error from an unrelated concurrently-running test).
func TestCheckGHAuth_JoinerUnaffectedByLeaderContextCancellation(t *testing.T) {
	ghAuthCache.Store("", authResult{err: errors.New("force cache miss"), expiry: time.Now().Add(-time.Hour)})
	t.Cleanup(func() { ghAuthCache.Store("", authResult{err: nil, expiry: time.Now().Add(-time.Hour)}) })

	started := make(chan struct{})
	release := make(chan struct{})
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	leaderCtx, leaderCancel := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() { leaderDone <- CheckGHAuth(leaderCtx) }()
	<-started // leader's request is in flight, holding the singleflight call

	joinerDone := make(chan error, 1)
	go func() { joinerDone <- CheckGHAuth(context.Background()) }()
	time.Sleep(20 * time.Millisecond) // let the joiner actually enter ghAuthGroup.Do and start waiting

	leaderCancel() // leader's own context is canceled while the shared request is still in flight
	close(release) // now let the fake server respond

	if err := <-leaderDone; err != nil {
		t.Logf("leader's own CheckGHAuth returned an error (acceptable, its ctx was canceled): %v", err)
	}
	if err := <-joinerDone; err != nil {
		t.Fatalf("joiner's CheckGHAuth returned an error even though its own ctx was never canceled: %v", err)
	}
}
