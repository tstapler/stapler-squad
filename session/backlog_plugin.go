package session

import "context"

// ItemSourcePlugin is the interface all external source integrations must implement.
type ItemSourcePlugin interface {
	// PluginID returns the unique identifier for this plugin (e.g., "github_issues").
	PluginID() string
	// Fetch retrieves new and updated items since the cursor. Returns items and the new cursor.
	Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error)
	// MapToBacklogItem converts an external item to a BacklogItemData.
	MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData
}

// PluginConfig is opaque config passed to a plugin. Plugins decode their own fields.
type PluginConfig struct {
	Raw string // JSON
}

// ExternalItem is a platform-agnostic representation of an external issue/ticket.
type ExternalItem struct {
	ExternalID  string
	Title       string
	Description string
	Labels      []string
	Priority    int    // 1-5, derived from labels
	URL         string
}

// PluginRegistry holds registered source plugins.
type PluginRegistry struct {
	plugins map[string]ItemSourcePlugin
}

// NewPluginRegistry creates a new empty PluginRegistry.
func NewPluginRegistry() *PluginRegistry {
	return &PluginRegistry{plugins: make(map[string]ItemSourcePlugin)}
}

// Register adds a plugin to the registry.
func (r *PluginRegistry) Register(p ItemSourcePlugin) {
	r.plugins[p.PluginID()] = p
}

// Get retrieves a plugin by ID.
func (r *PluginRegistry) Get(id string) (ItemSourcePlugin, bool) {
	p, ok := r.plugins[id]
	return p, ok
}

// NewDefaultRegistry returns a registry with all built-in plugins registered.
func NewDefaultRegistry() *PluginRegistry {
	r := NewPluginRegistry()
	r.Register(NewGitHubIssuesPlugin())
	return r
}
