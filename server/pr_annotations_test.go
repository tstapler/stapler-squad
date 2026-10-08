package server

import (
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session"
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

func TestResolveSessionRepo_LegacyForkOnGHERemote_CarriesRemoteHost(t *testing.T) {
	dir := repoWithOrigin(t, "https://ghe.example.com/upstream/renamed.git")
	ref, _, unrecorded := resolveSessionRepo(legacySnap(dir, "me", "fork", ""), testGHEHosts)
	assert.False(t, unrecorded)
	assert.Equal(t, "ghe.example.com", ref.Host())
	assert.Equal(t, "me", ref.Owner())
	assert.Equal(t, "fork", ref.Repo())
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
