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
	if deps.BacklogService != nil {
		opts = append(opts, servermcp.WithPluginRegistry(deps.BacklogService.Registry()))
	}
	return opts
}
