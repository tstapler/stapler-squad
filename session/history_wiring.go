package session

// Registers the history registry and porter hooks (historyadapters' init sets
// instance.HistoryRegistryHook/HistoryPorterHook). Removing this import makes
// instance.HistoryRegistry() panic in every binary that links session.
import _ "github.com/tstapler/stapler-squad/session/historyadapters"
