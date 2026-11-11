package server

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

func TestNewItemSourceRegistry_should_IncludeAllBuiltInPlugins(t *testing.T) {
	t.Parallel()
	r := NewItemSourceRegistry()
	want := map[string]any{
		"github_issues": &session.GitHubIssuesPlugin{},
		"github_prs":    &session.GitHubPRsPlugin{},
		"linear":        &session.LinearPlugin{},
		"jira":          &session.JiraPlugin{},
	}
	for id, typ := range want {
		p, ok := r.Get(id)
		require.True(t, ok, id)
		require.IsType(t, typ, p, id)
	}
}

// Concrete plugin constructors belong to the composition root only; everyone
// else resolves plugins from the injected registry (tests excluded).
func TestConcretePluginConstructors_should_OnlyBeCalledFromCompositionRoot(t *testing.T) {
	t.Parallel()
	ctor := regexp.MustCompile(`session\.(NewLinearPlugin|NewJiraPlugin|NewGitHubIssuesPlugin|NewGitHubPRsPlugin)\(|session\.(GitHubIssuesPlugin|GitHubPRsPlugin|LinearPlugin|JiraPlugin)\{`)
	var offenders []string
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", ".git", "gen", "web-app", ".claude", "worktrees":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
			filepath.ToSlash(path) == "../server/item_source_plugins.go" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if ctor.Match(b) {
			offenders = append(offenders, path)
		}
		return nil
	})
	require.NoError(t, err)
	require.Empty(t, offenders, "construct plugins only in server/item_source_plugins.go")
}
