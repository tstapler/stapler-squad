//go:build integration

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/server/services"
)

// expectedToolCount registers the tool set the exact same way main.go's
// --mcp fallback path does (buildMCPDeps + RunServer(ctx, ..., nil, nil) --
// eventBus and prCache are both nil there) and returns how many tools that
// produces. This is the single source of truth for
// TestMCPHandshakeSubprocess's assertion below -- adding, removing, or
// gating a tool anywhere in this package changes this count automatically,
// with nothing to update by hand.
func expectedToolCount(t *testing.T) int {
	t.Helper()
	storage := newTestBacklogStorage(t)
	svc := services.NewSessionService(storage, nil)
	s := NewCore(&stubStore{}, svc, nil, storage, nil, nil, nil)
	return len(s.ListTools())
}

// TestMCPHandshakeSubprocess builds the binary and verifies that a full
// MCP handshake (initialize + tools/list) over stdio returns exactly the
// tools NewCore registers in production (I-1.1, I-1.4).
//
// The subprocess is pointed at an isolated config with an unreachable
// listen_address so it always takes the local fallback path (buildMCPDeps)
// deterministically, matching expectedToolCount above. Without this, the
// test silently proxies to a real stapler-squad HTTP server if one happens
// to already be listening on the default port (true on any dev machine
// running the service) -- exercising a completely different code path than
// intended, with a tool count that depends on incidental environment state
// rather than this package's own registration code.
func TestMCPHandshakeSubprocess(t *testing.T) {
	want := expectedToolCount(t)
	binaryPath := t.TempDir() + "/stapler-squad-test"
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	testDir := t.TempDir()
	// feature_flags.backlog must be true so the subprocess's real backlogEnabled
	// gate (main.go's `cfg.GetFeatureFlag("backlog")`) matches expectedToolCount's
	// oracle above, where NewCore is called with a nil backlogEnabled (always-on).
	// Config.GetFeatureFlag defaults unset keys to false, so omitting this key
	// silently drops the 8 backlog/goal tools from the subprocess's tool list.
	configJSON := `{"listen_address": "127.0.0.1:1", "feature_flags": {"backlog": true}}`
	if err := os.WriteFile(filepath.Join(testDir, "config.json"), []byte(configJSON), 0o644); err != nil {
		t.Fatalf("write isolated config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--mcp")
	cmd.Env = append(os.Environ(), fmt.Sprintf("STAPLER_SQUAD_TEST_DIR=%s", testDir))
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Process.Kill()

	initMsg := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	stdin.Write([]byte(initMsg + "\n"))

	listMsg := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	stdin.Write([]byte(listMsg + "\n"))
	stdin.Close()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	var responses []map[string]interface{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var msg map[string]interface{}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Errorf("non-JSON line on stdout: %q", line)
			continue
		}
		responses = append(responses, msg)
	}

	if len(responses) < 2 {
		t.Fatalf("expected >= 2 responses, got %d", len(responses))
	}

	var toolsResp map[string]interface{}
	for _, r := range responses {
		if result, ok := r["result"].(map[string]interface{}); ok {
			if _, hasTools := result["tools"]; hasTools {
				toolsResp = r
			}
		}
	}
	if toolsResp == nil {
		t.Fatal("no tools/list response found")
	}

	tools, ok := toolsResp["result"].(map[string]interface{})["tools"].([]interface{})
	if !ok {
		t.Fatal("tools field is not an array")
	}
	if len(tools) != want {
		names := make([]string, len(tools))
		for i, tool := range tools {
			names[i] = tool.(map[string]interface{})["name"].(string)
		}
		t.Errorf("expected %d tools (from NewCore's own registration), got %d: %v", want, len(tools), names)
	}
}
