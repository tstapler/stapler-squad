package github

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

// TestMain switches go-keyring to its in-memory mock provider before any test
// in this package runs, so tests never read a real OS keychain entry. Without
// this, TestUserPRCache_StartStop (github/user_pr_cache_test.go, package
// github_test) only avoided the real keychain by accident of keychain_test.go
// calling keyring.MockInit() first in execution order — a real risk on any
// dev machine with a real "github-token:*" entry already stored (see the
// backlog item this fixes for the observed failure mode).
func TestMain(m *testing.M) {
	keyring.MockInit()
	os.Exit(m.Run())
}
