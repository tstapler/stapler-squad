package mcp

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestNewProxyCoreUsesFreshContextForEveryHandshakeRequest(t *testing.T) {
	t.Parallel()

	remote := mcpserver.NewMCPServer("test", "1.0.0")
	remote.AddTool(mcpgo.NewTool("echo"), func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error) {
		return mcpgo.NewToolResultText("ok"), nil
	})
	server := httptest.NewServer(mcpserver.NewStreamableHTTPServer(remote, mcpserver.WithStateLess(true)))
	t.Cleanup(server.Close)

	var contexts []context.Context
	newContext := func(parent context.Context) (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithCancel(parent)
		contexts = append(contexts, ctx)
		return ctx, cancel
	}

	local, client, err := newProxyCoreWithContextFactory(context.Background(), server.URL, nil, newContext)
	if err != nil {
		t.Fatalf("new proxy core: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if got := len(local.ListTools()); got != 1 {
		t.Errorf("forwarded tool count = %d, want 1", got)
	}
	if len(contexts) != 3 {
		t.Fatalf("handshake contexts = %d, want 3", len(contexts))
	}
	for index, ctx := range contexts {
		if ctx.Err() == nil {
			t.Errorf("handshake context %d was not cancelled", index)
		}
		for priorIndex, prior := range contexts[:index] {
			if prior == ctx {
				t.Errorf("handshake context %d reuses context %d", index, priorIndex)
			}
		}
	}
}

func TestRunHandshakeStopsAtFirstFailedStep(t *testing.T) {
	t.Parallel()

	var calls int
	err := runHandshake(context.Background(), context.WithCancel, func(context.Context) error {
		calls++
		return fmt.Errorf("unavailable")
	}, func(context.Context) error {
		calls++
		return nil
	})
	if err == nil {
		t.Fatal("run handshake error = nil, want error")
	}
	if calls != 1 {
		t.Errorf("steps called = %d, want 1", calls)
	}
}
