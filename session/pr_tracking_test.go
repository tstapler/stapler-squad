package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/tstapler/stapler-squad/github"
)

// installFakeGHForPRTrackingTest puts a stub `gh` executable at the front of
// PATH that always succeeds, standing in for the real `gh pr view` subprocess
// RefreshPRInfo shells out to via github.GetPRInfoCtx.
func installFakeGHForPRTrackingTest(t *testing.T) {
	t.Helper()
	binDir := t.TempDir()
	script := "#!/bin/sh\ncat <<'GH_FAKE_EOF'\n{\"number\":1,\"title\":\"t\",\"headRefName\":\"h\",\"headRefOid\":\"sha\",\"baseRefName\":\"main\",\"state\":\"OPEN\",\"url\":\"\",\"createdAt\":\"\",\"updatedAt\":\"\",\"author\":{\"login\":\"x\"}}\nGH_FAKE_EOF\n"
	ghPath := filepath.Join(binDir, "gh")
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write fake gh script: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// stubGHAuthOKForPRTrackingTest redirects github.CheckGHAuth's network call to
// a local httptest server that always reports success, so RefreshPRInfo's
// call chain never depends on real GitHub reachability.
func stubGHAuthOKForPRTrackingTest(t *testing.T) {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	restore := github.SetGhBaseURLForTest(ts.URL + "/")
	t.Cleanup(restore)
}

// TestRefreshPRInfo_should_CallGetPRInfoCtxWithCallersContext_When_ContextTaggedInteractive
// is a regression test for Task 1.1.1a/1.1.1c's ctx-plumbing refactor: it
// proves the ctx passed into RefreshPRInfo actually reaches the underlying
// `gh` subprocess call rather than being silently dropped in favor of
// context.Background(). It does this by passing an already-canceled context:
// if RefreshPRInfo threads it through to github.GetPRInfoCtx as required,
// the subprocess call fails with a context-cancellation error before the
// fake `gh` script (which would otherwise report success) ever runs.
func TestRefreshPRInfo_should_CallGetPRInfoCtxWithCallersContext_When_ContextTaggedInteractive(t *testing.T) {
	stubGHAuthOKForPRTrackingTest(t)
	installFakeGHForPRTrackingTest(t)

	inst := &Instance{
		Title:          "test-pr-session",
		GitHubOwner:    "tstapler",
		GitHubRepo:     "stapler-squad",
		GitHubPRNumber: 42,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx = github.WithGitHubCallOrigin(ctx, github.OriginInteractive)

	_, err := inst.RefreshPRInfo(ctx)
	if err == nil {
		t.Fatal("expected RefreshPRInfo to fail with a canceled context, got nil error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("RefreshPRInfo error = %v, want it to wrap context.Canceled (proves the caller's ctx reached github.GetPRInfoCtx)", err)
	}
}
