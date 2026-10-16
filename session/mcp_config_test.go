package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// buildMCPConfigFlag mirrors the production logic in instance.go Restart().
// Extracted here so both the unit test and any future callers share one definition.
func buildMCPConfigFlag(mcpURL string) string {
	return fmt.Sprintf(`--mcp-config '{"mcpServers":{"stapler-squad":{"type":"http","url":%q}}}'`, mcpURL)
}

// TestMCPConfigFlagStructure verifies the generated --mcp-config JSON has the required
// MCP spec structure: top-level "mcpServers" wrapper with correct server entry.
func TestMCPConfigFlagStructure(t *testing.T) {
	t.Parallel()
	flag := buildMCPConfigFlag("http://localhost:8543/mcp")

	// Strip the --mcp-config prefix and surrounding single-quotes to get the raw JSON.
	const prefix = "--mcp-config '"
	const suffix = "'"
	if !strings.HasPrefix(flag, prefix) || !strings.HasSuffix(flag, suffix) {
		t.Fatalf("unexpected flag format: %q", flag)
	}
	rawJSON := flag[len(prefix) : len(flag)-len(suffix)]

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawJSON), &top); err != nil {
		t.Fatalf("flag JSON is not valid: %v\nJSON: %s", err, rawJSON)
	}

	mcpRaw, ok := top["mcpServers"]
	if !ok {
		t.Fatalf("JSON missing top-level \"mcpServers\" key (got keys: %v)\nJSON: %s", keys(top), rawJSON)
	}

	var servers map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &servers); err != nil {
		t.Fatalf("mcpServers is not a JSON object: %v", err)
	}

	entryRaw, ok := servers["stapler-squad"]
	if !ok {
		t.Fatalf("mcpServers missing \"stapler-squad\" entry (got keys: %v)", keys(servers))
	}

	var entry struct {
		Type string `json:"type"`
		URL  string `json:"url"`
	}
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		t.Fatalf("stapler-squad entry is not a valid JSON object: %v", err)
	}

	if entry.Type != "http" {
		t.Errorf("type: got %q, want \"http\"", entry.Type)
	}
	if entry.URL != "http://localhost:8543/mcp" {
		t.Errorf("url: got %q, want \"http://localhost:8543/mcp\"", entry.URL)
	}
}

// TestMCPConfigFlagRejectedByOldFormat confirms the previously-broken format
// (missing mcpServers wrapper) is structurally wrong.
func TestMCPConfigFlagRejectedByOldFormat(t *testing.T) {
	t.Parallel()
	oldJSON := `{"stapler-squad":{"type":"http","url":"http://localhost:8543/mcp"}}`

	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(oldJSON), &top); err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if _, ok := top["mcpServers"]; ok {
		t.Error("old format unexpectedly has mcpServers wrapper — test needs updating")
	}
}

// TestClaudeBinaryAcceptsMCPConfig is an integration test that runs the real claude
// binary and confirms it does not reject our --mcp-config JSON with a schema error.
//
// Hermetic by design: the MCP URL is deliberately unreachable and TestMain's
// envtest.IsolateClaudeCLI points the CLI at an empty config dir and a dead
// loopback API endpoint, so the schema check runs without the developer's
// credentials, the live service, or any real API call.
// Skipped when claude is not installed.
func TestClaudeBinaryAcceptsMCPConfig(t *testing.T) {
	t.Parallel()
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not in PATH — skipping binary integration test")
	}

	cfg := `{"mcpServers":{"stapler-squad":{"type":"http","url":"http://127.0.0.1:19999/mcp"}}}`

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, _ := safeexec.CommandContext(ctx, claudePath, "--mcp-config", cfg, "--strict-mcp-config", "--print", "test").CombinedOutput()
	output := string(out)

	if strings.Contains(output, "Does not adhere to MCP server configuration schema") ||
		strings.Contains(output, "Invalid MCP configuration") {
		t.Errorf("claude rejected MCP config as invalid schema:\n%s", output)
	}
}

func keys[K comparable, V any](m map[K]V) []K {
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
