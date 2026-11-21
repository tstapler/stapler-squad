package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	servermcp "github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services/backlogflow"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// RunStdioMCP is the stdio MCP fallback used when no HTTP server is reachable.
// It builds Phase 1 CoreDeps only (no tmux, listener or pollers), so there is no
// BacklogService or liveness checker: submit_review_verdict skips its eager
// review->in_progress transition, create_backlog_item/import_github_issue skip
// auto-triage and link_session_to_item answers UNAVAILABLE (ADR-001). The
// scrollback manager is read-only: it reads what the HTTP process wrote.
func RunStdioMCP(ctx context.Context, backlogEnabled func() bool) error {
	return stdioAssembly{
		buildCore: BuildCoreDeps,
		homeDir:   os.UserHomeDir,
		serve: func(ctx context.Context, core *CoreDeps, sbMgr *scrollback.ScrollbackManager, backlogEnabled func() bool, opts ...servermcp.CoreOption) error {
			return servermcp.RunServer(ctx, core.Storage, core.SessionService, sbMgr, core.Storage, nil, nil, backlogEnabled, nil, nil, nil, opts...)
		},
	}.run(ctx, backlogEnabled)
}

// stdioAssembly holds RunStdioMCP's three side effects so tests can fake them.
type stdioAssembly struct {
	buildCore func() (*CoreDeps, error)
	homeDir   func() (string, error)
	serve     func(ctx context.Context, core *CoreDeps, sbMgr *scrollback.ScrollbackManager, backlogEnabled func() bool, opts ...servermcp.CoreOption) error
}

func (a stdioAssembly) run(ctx context.Context, backlogEnabled func() bool) error {
	core, err := a.buildCore()
	if err != nil {
		return fmt.Errorf("mcp: init deps: %w", err)
	}
	homeDir, err := a.homeDir()
	if err != nil {
		return fmt.Errorf("mcp: init deps: cannot determine home directory for scrollback storage: %w", err)
	}
	sbConfig := scrollback.DefaultScrollbackConfig()
	sbConfig.StoragePath = filepath.Join(homeDir, ".stapler-squad", "sessions")
	sbMgr := scrollback.NewScrollbackManager(sbConfig)

	return runStdioMCP(core.Storage, func(opts ...servermcp.CoreOption) error {
		return a.serve(ctx, core, sbMgr, backlogEnabled, opts...)
	})
}

// stdioMCPOptions is the stdio CoreOption set. It goes through mcpCoreOptions,
// the HTTP builder, so both transports wire identical options; the flow
// service is built from storage alone and keeps its own aggregator cache.
func stdioMCPOptions(storage *session.Storage) []servermcp.CoreOption {
	return mcpCoreOptions(&ServerDependencies{BacklogFlowService: backlogflow.NewFlowServiceFromStorage(storage)})
}

// runStdioMCP hands the stdio option set to run (servermcp.RunServer with the
// transport deps already bound).
func runStdioMCP(storage *session.Storage, run func(opts ...servermcp.CoreOption) error) error {
	return run(stdioMCPOptions(storage)...)
}
