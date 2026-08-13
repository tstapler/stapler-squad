package session

import (
	"context"
	"time"
)

// ItemSourcePlugin is the interface all external source integrations must implement.
type ItemSourcePlugin interface {
	// PluginID returns the unique identifier for this plugin (e.g., "github_issues").
	PluginID() string
	// Fetch retrieves new and updated items since the cursor. Returns items and the new cursor.
	Fetch(ctx context.Context, config PluginConfig, cursor string) ([]ExternalItem, string, error)
	// MapToBacklogItem converts an external item to a BacklogItemData.
	MapToBacklogItem(item ExternalItem, sourceID string) BacklogItemData
}

// PaginatedFetcher is an optional capability an ItemSourcePlugin can
// implement for retrieving its complete result set across all pages, rather
// than Fetch's single-page/incremental-sync behavior. Consumers that need
// the full current state regardless of page size (e.g.
// SyncLoop.PreviewBackwardSyncImpact) should type-assert for this interface
// and prefer FetchAll when a plugin implements it, falling back to a plain
// Fetch call otherwise.
type PaginatedFetcher interface {
	// FetchAll retrieves items across multiple pages up to an
	// implementation-defined cap, returning the aggregated items, the newest
	// cursor value seen, and possiblyIncomplete=true if the cap was hit
	// while more results may still exist beyond it.
	FetchAll(ctx context.Context, config PluginConfig, cursor string) (items []ExternalItem, newCursor string, possiblyIncomplete bool, err error)
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
	Priority    int // 1-5, derived from labels
	URL         string
	// State is the external item's raw state string (e.g. GitHub issue
	// "open"/"closed"). Only populated by plugins that support two-way sync
	// (GitHubIssuesPlugin); left at zero value ("") for plugins like
	// GitHubPRsPlugin where two-way sync is out of scope.
	State string
	// IssueUpdatedAt is the external item's own last-modified timestamp (e.g.
	// GitHub issue updated_at), used as the loop-prevention watermark
	// comparison value. Zero value for plugins that don't populate it.
	IssueUpdatedAt time.Time
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
	r.Register(NewGitHubPRsPlugin())
	return r
}
