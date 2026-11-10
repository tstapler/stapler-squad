package server

import servermcp "github.com/tstapler/stapler-squad/server/mcp"

// flowMCPOptions hands the RPC handler's flow service to the HTTP MCP core, so
// both transports share one aggregator and one cache. A nil service adds no
// option, which leaves the flow tools answering ErrUnavailable.
func flowMCPOptions(deps *ServerDependencies) []servermcp.CoreOption {
	if deps.BacklogFlowService == nil {
		return nil
	}
	return []servermcp.CoreOption{servermcp.WithFlowService(deps.BacklogFlowService)}
}
