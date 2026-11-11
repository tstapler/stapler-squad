// Package itemsources holds the built-in tracker plugins (GitHub, Linear,
// JIRA). A new tracker is a new file here plus one entry in Default.
package itemsources

import "github.com/tstapler/stapler-squad/session"

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
func mapExternalToBacklogItem(item session.ExternalItem, sourceID string) session.BacklogItemData {
	title, description := session.BoundImportedText(item.Title, item.Description)
	return session.BacklogItemData{
		Title:       title,
		Description: description,
		Priority:    item.Priority,
		Status:      string(session.BacklogStatusIdea),
		ExternalID:  item.ExternalID,
		ExternalURL: item.URL,
		Labels:      item.Labels,
		SourceID:    sourceID,
	}
}
