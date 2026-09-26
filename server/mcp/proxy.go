// Package mcp: thin-client stdio proxy.
//
// Every Claude Code subagent process spawns its own copy of the project's
// .mcp.json-configured MCP servers, including "stapler-squad --mcp". Prior to
// this file, that meant each subprocess called buildMCPDeps() -> BuildCoreDeps()
// -> NewSessionServiceFromConfig(), which opens its own ent/SQLite client,
// EventBus, ReviewQueue, and ApprovalStore — a full duplicate of state the
// already-running stapler-squad HTTP server (localhost:8543) already holds.
// With N subagents in a session tree, that's N redundant SQLite connections
// and event buses for read-mostly MCP tool calls.
//
// NewProxyCore/RunProxyServer instead connect to the running server's
// Streamable HTTP MCP endpoint (mounted at /mcp by NewHTTPHandler) and forward
// every stdio tool call to it, so the stdio subprocess holds no storage of its
// own. If the HTTP server isn't reachable (e.g. no daemon running yet), the
// caller falls back to the original fully-local NewCore() path.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/tstapler/stapler-squad/log"
)

// ErrProxyUnavailable wraps any failure to connect to the running HTTP MCP
// server during the initial handshake (transport start, initialize, or
// tools/list). Each operation has an independent timeout; callers should treat
// failures as non-fatal and fall back to a fully local MCP core.
var ErrProxyUnavailable = errors.New("mcp proxy: remote server unavailable")

// handshakeTimeout bounds each initial handshake operation independently. It is
// intentionally not applied to the HTTP client used for the lifetime of the
// connection, since individual tool calls (for example, wait_for_output and
// run_command) may legitimately run long.
const handshakeTimeout = 3 * time.Second

type handshakeContextFactory func(context.Context) (context.Context, context.CancelFunc)

func runHandshake(ctx context.Context, newContext handshakeContextFactory, steps ...func(context.Context) error) error {
	for _, step := range steps {
		stepCtx, cancel := newContext(ctx)
		err := step(stepCtx)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

// ProxyHeaders builds the HTTP headers forwarded on every request to the
// remote MCP endpoint. Mirrors the X-Stapler-Session-UUID header injected by
// claudeMCPConfigArgs for the primary (non-proxied) HTTP MCP path, so backlog
// tools can identify the calling session the same way regardless of transport.
func ProxyHeaders() map[string]string {
	headers := map[string]string{}
	if uuid := os.Getenv("STAPLER_SESSION_UUID"); uuid != "" {
		headers["X-Stapler-Session-UUID"] = uuid
	}
	return headers
}

// NewProxyCore connects to an already-running stapler-squad HTTP server at
// httpURL and builds a local *mcpserver.MCPServer whose tools simply forward
// CallTool requests to it. Returns an error wrapping ErrProxyUnavailable if
// the remote server cannot be reached or fails the MCP handshake.
//
// The returned *mcpclient.Client must be closed by the caller once the local
// server is done serving (e.g. via defer).
func NewProxyCore(ctx context.Context, httpURL string, headers map[string]string) (*mcpserver.MCPServer, *mcpclient.Client, error) {
	return newProxyCore(ctx, httpURL, headers, handshakeTimeout)
}

func newProxyCore(ctx context.Context, httpURL string, headers map[string]string, timeout time.Duration) (*mcpserver.MCPServer, *mcpclient.Client, error) {
	return newProxyCoreWithContextFactory(ctx, httpURL, headers, func(parent context.Context) (context.Context, context.CancelFunc) {
		return context.WithTimeout(parent, timeout)
	})
}

func newProxyCoreWithContextFactory(ctx context.Context, httpURL string, headers map[string]string, newContext handshakeContextFactory) (*mcpserver.MCPServer, *mcpclient.Client, error) {
	remote, err := mcpclient.NewStreamableHttpClient(httpURL, transport.WithHTTPHeaders(headers))
	if err != nil {
		return nil, nil, fmt.Errorf("%w: create http client: %v", ErrProxyUnavailable, err)
	}

	initReq := mcpgo.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcpgo.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcpgo.Implementation{Name: "stapler-squad-mcp-proxy", Version: "1.0.0"}

	var toolsResult *mcpgo.ListToolsResult
	err = runHandshake(ctx, newContext,
		func(stepCtx context.Context) error {
			if err := remote.Start(stepCtx); err != nil {
				return fmt.Errorf("%w: start transport: %v", ErrProxyUnavailable, err)
			}
			return nil
		},
		func(stepCtx context.Context) error {
			if _, err := remote.Initialize(stepCtx, initReq); err != nil {
				return fmt.Errorf("%w: initialize: %v", ErrProxyUnavailable, err)
			}
			return nil
		},
		func(stepCtx context.Context) error {
			var err error
			toolsResult, err = remote.ListTools(stepCtx, mcpgo.ListToolsRequest{})
			if err != nil {
				return fmt.Errorf("%w: list tools: %v", ErrProxyUnavailable, err)
			}
			return nil
		},
	)
	if err != nil {
		_ = remote.Close()
		return nil, nil, err
	}

	local := mcpserver.NewMCPServer(
		"stapler-squad",
		"1.0.0",
		mcpserver.WithToolCapabilities(false),
	)
	for _, tool := range toolsResult.Tools {
		toolName := tool.Name
		local.AddTool(tool, func(ctx context.Context, req mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
			req.Params.Name = toolName
			return remote.CallTool(ctx, req)
		})
	}

	log.Info("mcp proxy: connected to running stapler-squad server", "url", httpURL, "tools", len(toolsResult.Tools))
	return local, remote, nil
}

// RunProxyServer connects to the running stapler-squad HTTP server and serves
// stdio MCP as a thin forwarding proxy, blocking until stdin closes or ctx is
// cancelled. Returns an error wrapping ErrProxyUnavailable (safe to fall back
// to a local core) if the initial handshake fails; any error returned after a
// successful handshake reflects a mid-session failure and should not trigger
// a fallback (stdout may already carry a partial MCP session).
func RunProxyServer(ctx context.Context, httpURL string, headers map[string]string) error {
	local, remote, err := NewProxyCore(ctx, httpURL, headers)
	if err != nil {
		return err
	}
	defer remote.Close() //nolint:errcheck

	log.Info("mcp proxy server starting on stdio transport", "remote", httpURL)
	stdio := mcpserver.NewStdioServer(local)
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}
