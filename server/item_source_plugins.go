package server

import (
	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/itemsources"
)

// NewItemSourceRegistry is the composition root's list of built-in tracker
// plugins; itemsources.Default owns the list.
func NewItemSourceRegistry() *session.PluginRegistry {
	return session.NewPluginRegistry(itemsources.Default()...)
}

// trackerRefParser adapts the itemsources parsers to the MCP import tools' port.
type trackerRefParser struct{}

// NewTrackerRefParser returns the production servermcp.TrackerRefParser.
func NewTrackerRefParser() servermcp.TrackerRefParser { return trackerRefParser{} }

func (trackerRefParser) ParseLinearRef(ref string) (string, error) {
	return itemsources.ParseLinearIssueRef(ref)
}

func (trackerRefParser) ParseJiraRef(ref string) (servermcp.JiraIssueRef, error) {
	r, err := itemsources.ParseJiraIssueRef(ref)
	return servermcp.JiraIssueRef{BaseURL: r.BaseURL, Key: r.Key}, err
}
