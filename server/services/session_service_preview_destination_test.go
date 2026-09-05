package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	gh "github.com/tstapler/stapler-squad/github"
)

// setupTestGitRepoForPreview creates a minimal git repo for PreviewWorktreePath's
// repo-root validation (mirrors session/git's setupTestRepo helper). Uses go-git
// directly rather than shelling out — see the `prefer-go-git-over-subshells` skill.
func setupTestGitRepoForPreview(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644))
	wt, err := repo.Worktree()
	require.NoError(t, err)
	_, err = wt.Add("README.md")
	require.NoError(t, err)
	_, err = wt.Commit("Initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "Test User", Email: "test@example.com", When: time.Now()},
	})
	require.NoError(t, err)

	return dir
}

// TestPreviewDestinationPath_GitHubURL_ReturnsExactPath verifies mode=github_url resolves
// a shorthand/URL to the exact clone destination RepoPathManager would use.
func TestPreviewDestinationPath_GitHubURL_ReturnsExactPath(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	withFakeHome(t)

	resp, err := svc.PreviewDestinationPath(context.Background(), connect.NewRequest(&sessionv1.PreviewDestinationPathRequest{
		Input: "https://github.com/tstapler/stapler-squad",
		Mode:  "github_url",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.IsExact)
	assert.Empty(t, resp.Msg.UnresolvedReason)
	assert.Contains(t, resp.Msg.Path, "tstapler")
	assert.Contains(t, resp.Msg.Path, "stapler-squad")
}

// TestPreviewDestinationPath_GitHubURL_EnterpriseHostViaCachedAccount_ReturnsExactPath is
// the direct regression test for the host-divergence bug class (commit 381c309b6): a host
// recognized only because an account was dynamically added (gh CLI import / device auth),
// with no static config.json entry, must resolve exactly like CreateSession does — via the
// shared enterpriseHosts() helper reading s.userPRCache, not a second copy of the union.
func TestPreviewDestinationPath_GitHubURL_EnterpriseHostViaCachedAccount_ReturnsExactPath(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(fakeGitHubEnterpriseHandler))
	defer ts.Close()
	const enterpriseHost = "github.netflix.net"
	gh.SetEnterpriseBaseURLOverride(enterpriseHost, ts.URL+"/")
	defer gh.SetEnterpriseBaseURLOverride(enterpriseHost, "")

	keyring.MockInit()
	cache := gh.NewUserPRCache()
	cache.Start(context.Background())
	t.Cleanup(cache.Stop)

	// No statically configured enterprise hosts for this host — mirrors an account
	// added via AddGitHubAccountWithToken/AddGitHubAccountFromCLI with no OAuth App
	// registered in config.json. AddGitHubAccountWithToken refreshes the cache itself,
	// so cache.GetCachedAccounts() reflects this account once it returns.
	userSvc := NewGitHubUserService(cache, nil)
	_, err := userSvc.AddGitHubAccountWithToken(context.Background(),
		connect.NewRequest(&sessionv1.AddGitHubAccountWithTokenRequest{
			Host:  enterpriseHost,
			Token: "test-token",
		}))
	require.NoError(t, err)

	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)
	svc.SetUserPRCache(cache)

	withFakeHome(t)

	resp, err := svc.PreviewDestinationPath(context.Background(), connect.NewRequest(&sessionv1.PreviewDestinationPathRequest{
		Input: "https://" + enterpriseHost + "/corp/some-repo",
		Mode:  "github_url",
	}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.IsExact)
	assert.Empty(t, resp.Msg.UnresolvedReason)
	assert.Contains(t, resp.Msg.Path, enterpriseHost)
	assert.Contains(t, resp.Msg.Path, "corp")
	assert.Contains(t, resp.Msg.Path, "some-repo")
}

// TestPreviewDestinationPath_GitHubURL_UnrecognizedInput_ReturnsUnresolvedReason verifies
// unparseable input is a normal success response with unresolved_reason set, not an error
// — "user hasn't finished typing" is the common case.
func TestPreviewDestinationPath_GitHubURL_UnrecognizedInput_ReturnsUnresolvedReason(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	resp, err := svc.PreviewDestinationPath(context.Background(), connect.NewRequest(&sessionv1.PreviewDestinationPathRequest{
		Input: "not a github url at all",
		Mode:  "github_url",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.IsExact)
	assert.NotEmpty(t, resp.Msg.UnresolvedReason)
	assert.Empty(t, resp.Msg.Path)
}

// TestPreviewDestinationPath_NewWorktree_ReturnsApproximatePrefix verifies mode=new_worktree
// returns the deterministic directory prefix (IsExact=false), never a fabricated full path.
func TestPreviewDestinationPath_NewWorktree_ReturnsApproximatePrefix(t *testing.T) {
	repoDir := setupTestGitRepoForPreview(t)
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	withFakeHome(t)

	resp, err := svc.PreviewDestinationPath(context.Background(), connect.NewRequest(&sessionv1.PreviewDestinationPathRequest{
		Mode:        "new_worktree",
		RepoPath:    repoDir,
		SessionName: "My Feature",
	}))
	require.NoError(t, err)
	assert.False(t, resp.Msg.IsExact)
	assert.Empty(t, resp.Msg.UnresolvedReason)
	assert.Contains(t, resp.Msg.Path, "my-feature")
}

// TestPreviewDestinationPath_NewWorktree_MissingParams_ReturnsUnresolvedReason verifies
// missing repo_path/session_name short-circuits to unresolved rather than erroring.
func TestPreviewDestinationPath_NewWorktree_MissingParams_ReturnsUnresolvedReason(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	resp, err := svc.PreviewDestinationPath(context.Background(), connect.NewRequest(&sessionv1.PreviewDestinationPathRequest{
		Mode: "new_worktree",
	}))
	require.NoError(t, err)
	assert.NotEmpty(t, resp.Msg.UnresolvedReason)
}

// TestPreviewDestinationPath_UnknownMode_ReturnsInvalidArgument verifies an unrecognized
// mode is a real client bug (CodeInvalidArgument), unlike ambiguous/unresolved input.
func TestPreviewDestinationPath_UnknownMode_ReturnsInvalidArgument(t *testing.T) {
	storage := createTestStorage(t)
	svc := newCreateTestService(t, storage)

	_, err := svc.PreviewDestinationPath(context.Background(), connect.NewRequest(&sessionv1.PreviewDestinationPathRequest{
		Mode: "bogus",
	}))
	require.Error(t, err)
	assertConnectCode(t, err, connect.CodeInvalidArgument)
}
