package main

import (
	"github.com/tstapler/stapler-squad/server"
	mcpserver "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/session"
)

// stdioMCPOptions is the stdio fallback's CoreOption set; it has no
// ServerDependencies, so it builds the one wake-metrics instance itself.
func stdioMCPOptions(storage *session.Storage) []mcpserver.CoreOption {
	return []mcpserver.CoreOption{
		mcpserver.WithFlowService(backlogflow.NewFlowServiceFromStorage(storage)),
		mcpserver.WithPluginRegistry(server.NewItemSourceRegistry()),
		mcpserver.WithTrackerRefParser(server.NewTrackerRefParser()),
		mcpserver.WithWakeMetrics(server.NewWakeMetrics()),
	}
}

// runStdioMCP runs the stdio fallback with its CoreOption set; run is
// mcpserver.RunServer with the transport deps already bound.
func runStdioMCP(storage *session.Storage, run func(opts ...mcpserver.CoreOption) error) error {
	return run(stdioMCPOptions(storage)...)
}
