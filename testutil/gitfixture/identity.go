// Package gitfixture provides deterministic Git repository fixtures for tests.
package gitfixture

import (
	"context"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/tstapler/stapler-squad/executor/safeexec"
)

const (
	// UserName is the canonical author name for test commits.
	UserName = "Test User"
	// UserEmail is the canonical author email for test commits.
	UserEmail = "test@example.com"
)

// ConfigureGoGitIdentity writes the canonical test author identity to repo's
// local config through go-git. Tests must not depend on a developer's global
// Git configuration, which may be absent in CI or intentionally hidden by a
// sandbox.
func ConfigureGoGitIdentity(t testing.TB, repo *git.Repository) {
	t.Helper()
	cfg, err := repo.Config()
	if err != nil {
		t.Fatalf("read test repository config: %v", err)
	}
	cfg.User.Name = UserName
	cfg.User.Email = UserEmail
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatalf("write test repository identity: %v", err)
	}
}

// ConfigureNativeIdentity writes the same canonical identity through the Git
// CLI. Keeping native-git and go-git fixtures on one identity prevents tests
// from accidentally succeeding only because a developer has global Git
// credentials configured.
func ConfigureNativeIdentity(t testing.TB, repoDir string) {
	t.Helper()
	for key, value := range map[string]string{
		"user.name":  UserName,
		"user.email": UserEmail,
	} {
		cmd := safeexec.CommandContext(context.Background(), "git", "config", "--local", key, value)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config %s failed: %v: %s", key, err, strings.TrimSpace(string(out)))
		}
	}
}
