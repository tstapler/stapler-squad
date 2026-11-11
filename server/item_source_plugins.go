package server

import (
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/itemsources"
)

// NewItemSourceRegistry is the composition root's list of built-in tracker
// plugins; itemsources.Default owns the list.
func NewItemSourceRegistry() *session.PluginRegistry {
	return session.NewPluginRegistry(itemsources.Default()...)
}
