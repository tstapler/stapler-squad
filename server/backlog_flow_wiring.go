package server

import servermcp "github.com/tstapler/stapler-squad/server/mcp"

// flowMCPOptions hands the RPC handler's flow service to the HTTP MCP core, so
// both transports share one aggregator and one cache. A nil service adds no
// option, which leaves the flow tools answering ErrUnavailable.
func flowMCPOptions(deps *ServerDependencies) []servermcp.CoreOption {
	var opts []servermcp.CoreOption
	if deps.BacklogFlowService != nil {
		opts = append(opts, servermcp.WithFlowService(deps.BacklogFlowService))
	}
	// Without a backlog service the tracker import tools still need their plugins.
	registry := NewItemSourceRegistry()
	if deps.BacklogService != nil {
		registry = deps.BacklogService.Registry()
	}
	opts = append(opts, servermcp.WithPluginRegistry(registry))
	return opts
}
