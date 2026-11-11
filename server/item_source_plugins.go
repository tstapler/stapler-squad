package server

import "github.com/tstapler/stapler-squad/session"

// NewItemSourceRegistry is the composition root's list of built-in tracker
// plugins. A new tracker is one plugin file plus one entry here.
func NewItemSourceRegistry() *session.PluginRegistry {
	return session.NewPluginRegistry(
		session.NewGitHubIssuesPlugin(),
		session.NewGitHubPRsPlugin(),
		session.NewLinearPlugin(),
		session.NewJiraPlugin(),
	)
}
