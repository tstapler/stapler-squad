// Package mcp implements the MCP (Model Context Protocol) server for Stapler Squad.
// Activated by the --mcp flag; communicates over stdio transport.
package mcp

import (
	"context"
	"os"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/events"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session"
)

// coreConfig holds the optional dependencies injected through CoreOption, so
// adding one never grows the positional parameter lists below.
type coreConfig struct {
	// userPRs backs the GitHub PR tools; nil leaves them unregistered.
	userPRs   UserPRSource
	flowSvc   FlowService
	plugins   *session.PluginRegistry
	refParser TrackerRefParser
	// wakeMetrics is the process-wide wake counter mirror; nil drops counts (a nil *wakes.Metrics is a no-op).
	wakeMetrics *wakes.Metrics
	// openWorktree builds worktrees for the VCS and stuck-report tools; nil reports them unavailable.
	openWorktree WorktreeOpener
	// github is the GitHub client for the issue/PR/commit tools; nil reports GitHub unavailable.
	github GitHub
	// extraModules register after the built-in set (see WithToolModules).
	extraModules []ToolModule
}

// CoreOption injects an optional dependency into NewCore, NewHTTPHandler or RunServer.
type CoreOption func(*coreConfig)

// WithFlowService enables the flow-observability tools. Without it they are
// registered but return ErrUnavailable. The HTTP path passes the same instance
// the RPC handler uses, so one aggregator cache serves both. Pass an untyped
// nil or a non-nil value: a typed-nil pointer defeats the absent-service guard.
func WithFlowService(svc FlowService) CoreOption {
	return func(c *coreConfig) { c.flowSvc = svc }
}

// WithUserPRCache enables the GitHub PR tools. Without it they are not
// registered. Pass an untyped nil or a non-nil value: a typed-nil pointer
// defeats the absent-source guard.
func WithUserPRCache(src UserPRSource) CoreOption {
	return func(c *coreConfig) { c.userPRs = src }
}

// WithPluginRegistry supplies the item-source plugins the tracker import tools
// resolve by ID. Without it import_linear_issue / import_jira_issue return a fetch error.
func WithPluginRegistry(r *session.PluginRegistry) CoreOption {
	return func(c *coreConfig) { c.plugins = r }
}

// WithTrackerRefParser supplies the issue-reference parser behind
// import_linear_issue / import_jira_issue. Without it those tools return a fetch error.
func WithTrackerRefParser(p TrackerRefParser) CoreOption {
	return func(c *coreConfig) { c.refParser = p }
}

// WithWakeMetrics supplies the shared wake counter mirror the identity gate and
// peer-help tools count into, so the composition root's dispatcher reads the
// same instance. Without it those counts are dropped.
func WithWakeMetrics(m *wakes.Metrics) CoreOption {
	return func(c *coreConfig) { c.wakeMetrics = m }
}

// WakeMetricsFromOptions returns the wake metrics the options carry, or nil.
// It lets wiring tests prove the MCP core and the dispatcher share one instance.
func WakeMetricsFromOptions(opts ...CoreOption) *wakes.Metrics {
	return newCoreConfig(opts).wakeMetrics
}

// UserPRSourceFromOptions returns the PR source the options carry, or nil.
// It lets wiring tests inspect what the composition root injected.
func UserPRSourceFromOptions(opts ...CoreOption) UserPRSource {
	return newCoreConfig(opts).userPRs
}

// FlowServiceFromOptions returns the flow service the options carry, or nil.
// It lets wiring tests prove two call sites hold the same instance.
func FlowServiceFromOptions(opts ...CoreOption) FlowService {
	return newCoreConfig(opts).flowSvc
}

// PluginRegistryFromOptions returns the plugin registry the options carry, or nil.
func PluginRegistryFromOptions(opts ...CoreOption) *session.PluginRegistry {
	return newCoreConfig(opts).plugins
}

// TrackerRefParserFromOptions returns the tracker ref parser the options carry, or nil.
func TrackerRefParserFromOptions(opts ...CoreOption) TrackerRefParser {
	return newCoreConfig(opts).refParser
}

func newCoreConfig(opts []CoreOption) coreConfig {
	var c coreConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// NewCore creates an MCPServer with all tools registered.
// Shared by the stdio path (RunServer) and the HTTP path (NewHTTPHandler).
// storage is optional — when nil, backlog tools are not registered.
// eventBus is optional — when nil, triage-complete notifications are disabled.
// backlogEnabled is optional — when nil, backlog/goal tools are always enabled
// (matches pre-flag behavior, used by tests). When set, it gates registration
// at startup (belt-and-suspenders) and is threaded into each backlog/goal
// handler so a live flag flip takes effect without restarting the MCP server.
// autoReopener is optional — nil skips submit_review_verdict's eager review->in_progress
// transition, falling back to the pre-existing session-exit/sweep paths.
// backlogSvc is optional — nil skips auto-triage (BUG-061) and makes link_session_to_item
// return ErrUnavailable instead of panicking (e.g. the stdio fallback transport, ADR-001).
// liveCheck is optional — nil treats every EndedAt==nil ItemSession row as live in
// link_session_to_item's exclusivity check (pre-feature behavior).
// opts carry optional dependencies (see CoreOption).
func NewCore(
	store session.InstanceStore,
	svc SessionRPC,
	sbMgr TerminalScrollback,
	storage *session.Storage,
	eventBus *events.EventBus,
	backlogEnabled func() bool,
	autoReopener AutoReopener,
	backlogSvc BacklogRPC,
	liveCheck func(sessionUUID string) bool,
	opts ...CoreOption,
) *mcpserver.MCPServer {
	cfg := newCoreConfig(opts)
	s := mcpserver.NewMCPServer(
		"stapler-squad",
		"1.0.0",
		mcpserver.WithToolCapabilities(false),
	)

	registerModules(s, defaultToolModules(coreDeps{
		store: store, svc: svc, sbMgr: sbMgr, storage: storage, eventBus: eventBus,
		backlogEnabled: backlogEnabled, autoReopener: autoReopener,
		backlogSvc: backlogSvc, liveCheck: liveCheck, cfg: cfg,
	}))
	return s
}

// NewHTTPHandler returns an http.Handler that serves the MCP protocol over
// Streamable HTTP (the MCP 2025-03-26 transport). Mount it at /mcp on the
// existing HTTP server so Claude sessions can connect without spawning a
// subprocess.
// eventBus is optional — pass nil to disable triage-complete notifications.
// backlogEnabled is optional — see NewCore.
// autoReopener/backlogSvc/liveCheck are optional — see NewCore.
func NewHTTPHandler(
	store session.InstanceStore,
	svc SessionRPC,
	sbMgr TerminalScrollback,
	storage *session.Storage,
	eventBus *events.EventBus,
	backlogEnabled func() bool,
	autoReopener AutoReopener,
	backlogSvc BacklogRPC,
	liveCheck func(sessionUUID string) bool,
	opts ...CoreOption,
) *mcpserver.StreamableHTTPServer {
	core := NewCore(store, svc, sbMgr, storage, eventBus, backlogEnabled, autoReopener, backlogSvc, liveCheck, opts...)
	log.InfoLog().Printf("[mcp] link_session_to_item wired: backlogSvc=%v liveCheck=%v", backlogSvc != nil, liveCheck != nil)
	// Stateless mode: accept any session ID rather than tracking them in memory.
	// This allows Claude Code sessions to survive server restarts without needing
	// to re-initialize the MCP connection (which would require restarting the agent).
	return mcpserver.NewStreamableHTTPServer(core, mcpserver.WithStateLess(true))
}

// RunServer initializes and starts the MCP stdio server.
// It blocks until the context is cancelled or stdin is closed.
// store is used for read-only discovery tools. svc provides lifecycle operations.
// sbMgr provides read access to terminal scrollback data persisted on disk.
// storage is used for backlog tools (optional; pass nil to disable).
// eventBus is optional — pass nil to disable triage-complete notifications on stdio path.
// backlogEnabled is optional — see NewCore.
// autoReopener/backlogSvc/liveCheck are optional — see NewCore. The stdio fallback path
// (server.RunStdioMCP) only builds Phase 1 (CoreDeps) dependencies, so callers on
// that path pass nil for all three and get NewCore's documented nil-behavior for each.
func RunServer(
	ctx context.Context,
	store session.InstanceStore,
	svc SessionRPC,
	sbMgr TerminalScrollback,
	storage *session.Storage,
	eventBus *events.EventBus,
	backlogEnabled func() bool,
	autoReopener AutoReopener,
	backlogSvc BacklogRPC,
	liveCheck func(sessionUUID string) bool,
	opts ...CoreOption,
) error {
	log.Info("mcp server starting on stdio transport")

	// Inject session UUID from environment into the root context so that
	// backlog tools can identify the calling session.
	if uuid := os.Getenv("STAPLER_SESSION_UUID"); uuid != "" {
		ctx = WithSessionUUID(ctx, uuid)
		log.InfoLog().Printf("[mcp] session UUID injected from environment: %s", uuid)
	}

	stdio := mcpserver.NewStdioServer(NewCore(store, svc, sbMgr, storage, eventBus, backlogEnabled, autoReopener, backlogSvc, liveCheck, opts...))
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

func registerDiscoveryTools(s *mcpserver.MCPServer, d *discoveryHandlers) {
	s.AddTool(
		mcpgo.NewTool("list_sessions",
			mcpgo.WithDescription("List Stapler Squad sessions. Returns a page of sessions with summary info. Prefer search_sessions when looking for a specific session — it is faster and returns less context. Default limit is 10 to avoid filling LLM context."),
			mcpgo.WithString("status_filter",
				mcpgo.Description("Filter by status: running, paused, ready, loading, needs_approval"),
				mcpgo.Enum("running", "paused", "ready", "loading", "needs_approval"),
			),
			mcpgo.WithNumber("limit",
				mcpgo.Description("Max sessions per page (default 10, max 100)"),
				mcpgo.DefaultNumber(10),
				mcpgo.Min(1),
				mcpgo.Max(100),
			),
			mcpgo.WithString("cursor",
				mcpgo.Description("Opaque pagination cursor from a previous list_sessions response"),
			),
		),
		d.listSessions,
	)

	s.AddTool(
		mcpgo.NewTool("get_session",
			mcpgo.WithDescription("Get full details for a single Stapler Squad session by ID. Does not include terminal output — use read_session_output for that."),
			mcpgo.WithString("session_id",
				mcpgo.Description("Session ID (title) of the session to retrieve"),
				mcpgo.Required(),
			),
		),
		d.getSession,
	)

	s.AddTool(
		mcpgo.NewTool("search_sessions",
			mcpgo.WithDescription("Search sessions by text query or tags. Prefer this over list_sessions when looking for a specific session. Matches against title, path, branch, and tags."),
			mcpgo.WithString("query",
				mcpgo.Description("Search query matched against title, path, branch, and tags"),
				mcpgo.Required(),
			),
			mcpgo.WithArray("tag_filter",
				mcpgo.Description("Filter to sessions that have all of these tags"),
			),
			mcpgo.WithNumber("limit",
				mcpgo.Description("Max results (default 10, max 50)"),
				mcpgo.DefaultNumber(10),
				mcpgo.Min(1),
				mcpgo.Max(50),
			),
		),
		d.searchSessions,
	)
}
