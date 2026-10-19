package session

import (
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
	"github.com/tstapler/stapler-squad/session/ent/itemsource"
)

// sourcePluginIDOf returns the eager-loaded source's plugin id, or "" when the
// item is locally created or the Source edge was not loaded.
func sourcePluginIDOf(item *ent.BacklogItem) string {
	if item.Edges.Source == nil {
		return ""
	}
	return item.Edges.Source.PluginID
}

// applySourcePluginFilter restricts q to items whose source plugin id is in
// pluginIDs; an empty list leaves q unrestricted.
func applySourcePluginFilter(q *ent.BacklogItemQuery, pluginIDs []string) *ent.BacklogItemQuery {
	if len(pluginIDs) == 0 {
		return q
	}
	return q.Where(backlogitem.HasSourceWith(itemsource.PluginIDIn(pluginIDs...)))
}
