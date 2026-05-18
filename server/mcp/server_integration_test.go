//go:build integration

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"testing"
	"time"
)

// TestMCPHandshakeSubprocess builds the binary and verifies that a full
// MCP handshake (initialize + tools/list) over stdio returns exactly 20
// registered tools (I-1.1, I-1.4).
func TestMCPHandshakeSubprocess(t *testing.T) {
	binaryPath := t.TempDir() + "/stapler-squad-test"
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = "../.."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "--mcp")
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
	if len(tools) != 20 {
		names := make([]string, len(tools))
		for i, tool := range tools {
			names[i] = tool.(map[string]interface{})["name"].(string)
		}
		t.Errorf("expected 20 tools, got %d: %v", len(tools), names)
	}
}
