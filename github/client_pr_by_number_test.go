package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// resetGhBaseURLForTest overrides GhBaseURL for a test and returns a restore
// func. Same-package variant of the pattern used in
// server/services/backlog_github_rpc_test.go.
func resetGhBaseURLForTest(ts *httptest.Server) func() {
	GhBaseURL = ts.URL + "/"
	return func() { GhBaseURL = "https://api.github.com/" }
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
