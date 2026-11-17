package session

import (
	"path/filepath"

	"github.com/tstapler/stapler-squad/session/git/backend/gogit"
	"github.com/tstapler/stapler-squad/session/instance"
	"github.com/tstapler/stapler-squad/session/tcc"
)

// ConsentEnrichmentProbe never invokes the general backend or Git CLI.
func ConsentEnrichmentProbe(gate *tcc.ConsentGate) func(string) (*instance.WorktreeInfo, error) {
	return func(path string) (*instance.WorktreeInfo, error) {
		metadata, err := gogit.ReadEnrichment(path, func(target string) error {
			if !gate.ReadyDependency(tcc.Dependency{Path: target, EnrichmentKey: path}) {
				return instance.ErrEnrichmentParked
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		owner, repo, host := instance.ParseGitHubRemoteURL(metadata.Origin)
		linked := metadata.GitDir != metadata.CommonDir
		root := metadata.CommonDir
		if filepath.Base(root) == ".git" {
			root = filepath.Dir(root)
		}
		if !linked {
			root = path
		}
		return &instance.WorktreeInfo{IsWorktree: linked, MainRepoRoot: root, MainRepoPath: metadata.CommonDir, RemoteURL: metadata.Origin, GitHubHost: host, GitHubOwner: owner, GitHubRepo: repo}, nil
	}
}
