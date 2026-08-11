package github

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/tstapler/stapler-squad/envtest"
)

// TestMain switches go-keyring to its in-memory mock provider before any test
// in this package runs, so tests never read a real OS keychain entry. Without
// this, TestUserPRCache_StartStop (github/user_pr_cache_test.go, package
// github_test) only avoided the real keychain by accident of keychain_test.go
// calling keyring.MockInit() first in execution order — a real risk on any
// dev machine with a real "github-token:*" entry already stored (see the
// backlog item this fixes for the observed failure mode).
//
// It also clears GITHUB_TOKEN/GH_TOKEN for the whole test run: collectAllTokens
// reads both directly from the environment, so a developer machine or CI
// runner with either set (e.g. for gh CLI auth) would otherwise feed
// UserPRCache a real token and dial the real GitHub API mid-suite — the same
// class of unmocked-network-call CI hang this fixes for the enterprise-host
// GraphQL path.
func TestMain(m *testing.M) {
	keyring.MockInit()
	restore := envtest.ClearAmbientGitHubTokenEnv()
	code := m.Run()
	restore()
	os.Exit(code)
}
