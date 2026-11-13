package server

import (
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

var testGHEHosts = []string{"ghe.example.com"}

func repoWithOrigin(t *testing.T, url string) string {
	t.Helper()
	dir := t.TempDir()
	r, err := git.PlainInit(dir, false)
	require.NoError(t, err)
	_, err = r.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{url}})
	require.NoError(t, err)
	return dir
}

func legacySnap(path, owner, repo, prURL string) *session.InstanceSnapshot {
	return &session.InstanceSnapshot{
		Path:   path,
		GitHub: session.GitHubIntegration{GitHubOwner: owner, GitHubRepo: repo, GitHubPRURL: prURL},
	}
}

// A remote naming a different repo (fork/rename) must not stamp its host onto the
// stored owner/repo: the stored PR may live elsewhere.
func TestResolveSessionRepo_LegacyForkOnGHERemote_NoPRURL_StaysUnrecorded(t *testing.T) {
	dir := repoWithOrigin(t, "https://ghe.example.com/upstream/renamed.git")
	ref, _, unrecorded := resolveSessionRepo(legacySnap(dir, "me", "fork", ""), testGHEHosts)
	assert.True(t, unrecorded)
	assert.Equal(t, "", ref.Host())
	assert.Equal(t, "me", ref.Owner())
	assert.Equal(t, "fork", ref.Repo())
}

func TestResolveSessionRepo_LegacyForkOnGHERemote_PRURLOnGitHubCom_UsesPRURLHost(t *testing.T) {
	dir := repoWithOrigin(t, "https://ghe.example.com/upstream/renamed.git")
	ref, _, unrecorded := resolveSessionRepo(legacySnap(dir, "me", "fork", "https://github.com/me/fork/pull/9"), testGHEHosts)
	assert.False(t, unrecorded)
	assert.NotEqual(t, "ghe.example.com", ref.Host())
}

func TestResolveSessionRepo_LegacyForkOnGHERemote_PRURLOnGHE_UsesPRURLHost(t *testing.T) {
	dir := repoWithOrigin(t, "https://ghe.example.com/upstream/renamed.git")
	ref, _, unrecorded := resolveSessionRepo(legacySnap(dir, "me", "fork", "https://ghe.example.com/me/fork/pull/9"), testGHEHosts)
	assert.False(t, unrecorded)
	assert.Equal(t, "ghe.example.com", ref.Host())
}

func TestResolveSessionRepo_LegacyMatchingRemote_UsesRemote(t *testing.T) {
	dir := repoWithOrigin(t, "https://ghe.example.com/o/r.git")
	ref, _, unrecorded := resolveSessionRepo(legacySnap(dir, "o", "r", ""), testGHEHosts)
	assert.False(t, unrecorded)
	assert.Equal(t, "ghe.example.com", ref.Host())
}

func TestResolveSessionRepo_LegacyNoRemote_PRURLConfirmsHost(t *testing.T) {
	ref, _, unrecorded := resolveSessionRepo(legacySnap(t.TempDir(), "o", "r", "https://ghe.example.com/o/r/pull/7"), testGHEHosts)
	assert.False(t, unrecorded)
	assert.Equal(t, "ghe.example.com", ref.Host())
}

func TestResolveSessionRepo_LegacyNoRemoteNoURL_StaysUnrecorded(t *testing.T) {
	ref, _, unrecorded := resolveSessionRepo(legacySnap(t.TempDir(), "o", "r", ""), testGHEHosts)
	assert.True(t, unrecorded)
	assert.Equal(t, "o", ref.Owner())
}

func TestAnnotationsForSnapshot_ShouldEmitOneEntryPerPR_WhenSessionHoldsSeveral(t *testing.T) {
	r11, err := domain.NewPRRef("", "acme", "app", 11)
	require.NoError(t, err)
	r12, err := domain.NewPRRef("", "acme", "app", 12)
	require.NoError(t, err)
	snap := &session.InstanceSnapshot{
		Branch: "feat",
		GitHub: session.GitHubIntegration{
			GitHubOwner: "acme", GitHubRepo: "app", GitHubPRNumber: 12,
			PRs: []domain.PRView{{Ref: r11}, {Ref: r12}},
		},
	}

	got := annotationsForSnapshot("multi", snap, nil)

	require.Len(t, got, 2)
	assert.Equal(t, []int{11, 12}, []int{got[0].PRNumber, got[1].PRNumber})
	assert.Equal(t, "acme", got[0].Repo.Owner())
	assert.False(t, got[0].HostUnrecorded)
}

func TestAnnotationsForSnapshot_ShouldKeepLegacyEntry_WhenSessionHasNoLinks(t *testing.T) {
	snap := &session.InstanceSnapshot{GitHub: session.GitHubIntegration{GitHubOwner: "acme", GitHubRepo: "app", GitHubHost: "github.com", GitHubPRNumber: 7}}
	got := annotationsForSnapshot("legacy", snap, nil)
	require.Len(t, got, 1)
	assert.Equal(t, 7, got[0].PRNumber)
}
