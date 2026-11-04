package session

import (
	"path/filepath"

	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/session/tcc"
)

// ConsentEnrichmentProbe never invokes the general backend or Git CLI.
func ConsentEnrichmentProbe(gate *tcc.ConsentGate) func(string) (*WorktreeInfo, error) {
	return func(path string) (*WorktreeInfo, error) {
		metadata, err := gogit.ReadEnrichment(path, func(target string) error {
			if !gate.ReadyDependency(tcc.Dependency{Path: target, EnrichmentKey: path}) {
				return errEnrichmentParked
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		owner, repo, host := parseGitHubRemoteURL(metadata.Origin)
		linked := metadata.GitDir != metadata.CommonDir
		root := metadata.CommonDir
		if filepath.Base(root) == ".git" {
			root = filepath.Dir(root)
		}
		if !linked {
			root = path
		}
		return &WorktreeInfo{IsWorktree: linked, MainRepoRoot: root, MainRepoPath: metadata.CommonDir, RemoteURL: metadata.Origin, GitHubHost: host, GitHubOwner: owner, GitHubRepo: repo}, nil
	}
}
