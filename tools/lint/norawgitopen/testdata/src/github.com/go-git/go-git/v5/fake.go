// Package git is a minimal stand-in for github.com/go-git/go-git/v5, resolved
// via analysistest's GOPATH-style testdata overlay at exactly the real import
// path — this lets norawgitopen's real package-path check run against test
// fixtures without depending on the actual go-git module.
package git

type Repository struct{}

type PlainOpenOptions struct {
	DetectDotGit          bool
	EnableDotGitCommonDir bool
}

func PlainOpen(path string) (*Repository, error) { return nil, nil }

func PlainOpenWithOptions(path string, o *PlainOpenOptions) (*Repository, error) {
	return nil, nil
}
