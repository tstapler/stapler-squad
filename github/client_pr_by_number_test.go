package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// resetGhBaseURLForTest overrides GhBaseURL for a test and returns a restore
// func. Same-package variant of the pattern used in
// server/services/backlog_github_rpc_test.go.
func resetGhBaseURLForTest(ts *httptest.Server) func() {
	return SetGhBaseURLForTest(ts.URL + "/")
}

func TestGetPRByNumber_should_ReturnPRInfo_When_PRExists(t *testing.T) {
	body := map[string]any{
		"number":   326,
		"html_url": "https://github.com/tstapler/stapler-squad/pull/326",
		"state":    "closed",
		"merged":   true,
		"head": map[string]any{
			"ref": "feature/ci-status-diff-viewer",
		},
		"base": map[string]any{
			"ref": "main",
			"repo": map[string]any{
				"full_name": "tstapler/stapler-squad",
			},
		},
		"user": map[string]any{
			"login": "tstapler",
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	info, err := GetPRByNumber(context.Background(), "tstapler", "stapler-squad", 326)
	if err != nil {
		t.Fatalf("GetPRByNumber returned error: %v", err)
	}
	if info.State != PRStateMerged {
		t.Errorf("State = %q, want %q", info.State, PRStateMerged)
	}
	if info.Author != "tstapler" {
		t.Errorf("Author = %q, want %q", info.Author, "tstapler")
	}
	if info.HeadRef != "feature/ci-status-diff-viewer" {
		t.Errorf("HeadRef = %q, want %q", info.HeadRef, "feature/ci-status-diff-viewer")
	}
	if info.BaseRef != "main" {
		t.Errorf("BaseRef = %q, want %q", info.BaseRef, "main")
	}
}

func TestGetPRByNumber_should_ReturnErrNoPR_When_PRDoesNotExist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	_, err := GetPRByNumber(context.Background(), "tstapler", "stapler-squad", 99999)
	if !errors.Is(err, ErrNoPR) {
		t.Fatalf("err = %v, want errors.Is(err, ErrNoPR)", err)
	}
}

func TestGetPRByNumber_should_ReturnError_When_Forbidden(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	_, err := GetPRByNumber(context.Background(), "tstapler", "stapler-squad", 326)
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if errors.Is(err, ErrNoPR) {
		t.Fatalf("err should not be ErrNoPR, got %v", err)
	}
}

func TestGetPRByNumber_should_ReturnError_When_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	_, err := GetPRByNumber(context.Background(), "tstapler", "stapler-squad", 326)
	if err == nil {
		t.Fatal("expected non-nil error, got nil")
	}
	if errors.Is(err, ErrNoPR) {
		t.Fatalf("err should not be ErrNoPR, got %v", err)
	}
}

func TestGetPRByNumber_should_ReturnError_When_RepoFullNameMismatch(t *testing.T) {
	body := map[string]any{
		"number":   326,
		"html_url": "https://github.com/someone-else/other-repo/pull/326",
		"state":    "open",
		"merged":   false,
		"head": map[string]any{
			"ref": "feature/ci-status-diff-viewer",
		},
		"base": map[string]any{
			"ref": "main",
			"repo": map[string]any{
				"full_name": "someone-else/other-repo",
			},
		},
		"user": map[string]any{
			"login": "someone-else",
		},
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer ts.Close()
	defer resetGhBaseURLForTest(ts)()
	t.Setenv("GITHUB_TOKEN", "fake-token")

	_, err := GetPRByNumber(context.Background(), "tstapler", "stapler-squad", 326)
	if err == nil {
		t.Fatal("expected non-nil error for repo full_name mismatch, got nil")
	}
}

// stubGHAuthForTest bypasses CheckGHAuth's real network call by pre-seeding
// its process-wide result cache with a cached success, and restores the
// prior cached value (if any) when the test ends — GetPRInfoCtx calls
// CheckGHAuth before shelling out to the fake `gh` binary set up by
// installFakeGHForTest, and this repo has no fake-gh-auth-server pattern to
// reuse (CheckGHAuth talks to the GitHub REST API directly, not via `gh`).
func stubGHAuthForTest(t *testing.T) {
	t.Helper()
	prior := ghAuthState.Load()
	ghAuthState.Store(authResult{err: nil, expiry: time.Now().Add(time.Hour)})
	t.Cleanup(func() {
		if prior != nil {
			ghAuthState.Store(prior)
		} else {
			ghAuthState.Store(authResult{err: errors.New("test cleanup: auth not re-checked"), expiry: time.Now().Add(-time.Hour)})
		}
	})
}

// installFakeGHForTest puts a stub `gh` executable at the front of PATH that
// prints ghJSON for any `gh pr view ...` invocation, standing in for the real
// `gh pr view --json ...` subprocess GetPRInfoCtx shells out to.
func installFakeGHForTest(t *testing.T, ghJSON string) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\ncat <<'GH_FAKE_EOF'\n" + ghJSON + "\nGH_FAKE_EOF\n"
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGetPRInfoCtx_should_PopulateChecksAndReviews_When_StatusCheckRollupAndReviewsPresent(t *testing.T) {
	const ghJSON = `{
		"number": 456,
		"title": "Test PR",
		"body": "test body",
		"headRefName": "feature/x",
		"headRefOid": "abc123",
		"baseRefName": "main",
		"state": "open",
		"url": "https://github.com/tstapler/stapler-squad/pull/456",
		"createdAt": "2026-08-20T12:00:00Z",
		"updatedAt": "2026-08-21T12:00:00Z",
		"isDraft": false,
		"mergeable": "MERGEABLE",
		"additions": 10,
		"deletions": 5,
		"changedFiles": 3,
		"author": {"login": "carol"},
		"labels": [{"name": "vcs-tab"}],
		"reviewDecision": "CHANGES_REQUESTED",
		"reviews": [
			{"author": {"login": "alice"}, "state": "APPROVED", "body": ""},
			{"author": {"login": "bob"}, "state": "CHANGES_REQUESTED", "body": "Please fix the error handling in foo.go"}
		],
		"statusCheckRollup": [
			{"name": "build", "context": "", "state": "SUCCESS", "status": "completed", "conclusion": "success"},
			{"name": "", "context": "lint", "state": "FAILURE", "status": "completed", "conclusion": "failure"}
		]
	}`

	stubGHAuthForTest(t)
	installFakeGHForTest(t, ghJSON)

	info, err := GetPRInfoCtx(context.Background(), "tstapler", "stapler-squad", 456)
	if err != nil {
		t.Fatalf("GetPRInfoCtx returned error: %v", err)
	}

	// Collapsed fields (getCheckConclusion/parseReviewCounts) must still be correct.
	if info.CheckConclusion != "failure" {
		t.Errorf("CheckConclusion = %q, want %q", info.CheckConclusion, "failure")
	}
	if info.CheckStatus != "completed" {
		t.Errorf("CheckStatus = %q, want %q", info.CheckStatus, "completed")
	}
	if info.ApprovedCount != 1 {
		t.Errorf("ApprovedCount = %d, want 1", info.ApprovedCount)
	}
	if info.ChangesRequestedCount != 1 {
		t.Errorf("ChangesRequestedCount = %d, want 1", info.ChangesRequestedCount)
	}

	// Itemized Checks, read alongside the collapsed CheckConclusion/CheckStatus.
	wantChecks := []CheckItem{
		{Name: "build", Context: "", State: "SUCCESS", Status: "completed", Conclusion: "success"},
		{Name: "", Context: "lint", State: "FAILURE", Status: "completed", Conclusion: "failure"},
	}
	if len(info.Checks) != len(wantChecks) {
		t.Fatalf("len(Checks) = %d, want %d", len(info.Checks), len(wantChecks))
	}
	for i, want := range wantChecks {
		if info.Checks[i] != want {
			t.Errorf("Checks[%d] = %+v, want %+v", i, info.Checks[i], want)
		}
	}

	// Itemized Reviews, read alongside the collapsed Approved/ChangesRequested counts.
	wantReviews := []ReviewItem{
		{Author: "alice", State: "APPROVED", Body: ""},
		{Author: "bob", State: "CHANGES_REQUESTED", Body: "Please fix the error handling in foo.go"},
	}
	if len(info.Reviews) != len(wantReviews) {
		t.Fatalf("len(Reviews) = %d, want %d", len(info.Reviews), len(wantReviews))
	}
	for i, want := range wantReviews {
		if info.Reviews[i] != want {
			t.Errorf("Reviews[%d] = %+v, want %+v", i, info.Reviews[i], want)
		}
	}
}
