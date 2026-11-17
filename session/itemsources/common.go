// Package itemsources holds the built-in tracker plugins (GitHub, Linear,
// JIRA). A new tracker is a new file here plus one entry in Default.
package itemsources

import (
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

// Default returns the built-in plugins for the composition root to register.
func Default() []session.ItemSourcePlugin {
	return []session.ItemSourcePlugin{
		NewGitHubIssuesPlugin(),
		NewGitHubPRsPlugin(),
		NewLinearPlugin(),
		NewJiraPlugin(),
	}
}

// maxTrackerResponseBytes caps any single tracker API response body.
const maxTrackerResponseBytes = 8 << 20

// mapExternalToBacklogItem is the shared ExternalItem to BacklogItemData
// mapping used by the tracker plugins.
func mapExternalToBacklogItem(item session.ExternalItem, sourceID string) backlogdto.BacklogItemData {
	title, description := session.BoundImportedText(item.Title, item.Description)
	return backlogdto.BacklogItemData{
		Title:       title,
		Description: description,
		Priority:    item.Priority,
		Status:      string(domain.BacklogStatusIdea),
		ExternalID:  item.ExternalID,
		ExternalURL: item.URL,
		Labels:      item.Labels,
		SourceID:    sourceID,
	}
}
