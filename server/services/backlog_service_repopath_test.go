package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
)

func TestResolveRepoPathInput_should_RejectOnlyUnrecognizedURLs(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{GitHubEnterpriseHosts: []config.GitHubEnterpriseHost{{Host: "github.example.com"}}}
	resolver := &fakeGitHubResolver{localPath: "/clones/owner/repo"}
	svc := NewBacklogService(nil, nil, cfg, nil, nil, nil)
	svc.SetGitHubResolver(resolver.resolve)

	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"plain path unchanged", "/home/u/repo", "/home/u/repo", false},
		{"empty unchanged", "", "", false},
		{"github.com URL resolved to clone", "https://github.com/owner/repo", "/clones/owner/repo", false},
		{"configured GHE URL passes through unchanged", "https://github.example.com/corp/repo", "https://github.example.com/corp/repo", false},
		{"bare github.com URL rejected", "https://github.com/", "", true},
		{"unconfigured host URL rejected", "https://other.example.com/corp/repo", "", true},
		{"recognized ssh github URL resolved to clone", "ssh://git@github.com/owner/repo", "/clones/owner/repo", false},
		{"unrecognized ssh URL rejected", "ssh://git@other.example.com/owner/repo", "", true},
		{"file scheme rejected", "file:///home/u/repo", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.resolveRepoPathInput(tc.input)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
	assert.Equal(t, []string{"https://github.com/owner/repo", "ssh://git@github.com/owner/repo"}, resolver.calls)
}
