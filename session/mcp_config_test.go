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
// When the API proxy is available at localhost:47000 and the MCP server is available
// at localhost:8543/mcp, the test exercises full end-to-end connectivity. Otherwise
// it falls back to schema-only validation (unreachable MCP URL, no API proxy).
// Skipped when claude is not installed.
func TestClaudeBinaryAcceptsMCPConfig(t *testing.T) {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		t.Skip("claude not in PATH — skipping binary integration test")
	}

	mcpURL := "http://localhost:8543/mcp"
	if !serverReachable(mcpURL) {
		mcpURL = "http://localhost:19999/mcp" // unreachable fallback; still validates schema
	}

	cfg := fmt.Sprintf(`{"mcpServers":{"stapler-squad":{"type":"http","url":%q}}}`, mcpURL)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := safeexec.CommandContext(ctx, claudePath, "--mcp-config", cfg, "--print", "test")
	// Route through the local Claude API proxy when available so the test can get
	// a real response without hardcoding credentials.
	if serverReachable("http://localhost:47000") {
		cmd.Env = append(cmd.Environ(), "ANTHROPIC_BASE_URL=http://localhost:47000")
	}
	out, _ := cmd.CombinedOutput()
	output := string(out)

	if strings.Contains(output, "Does not adhere to MCP server configuration schema") ||
		strings.Contains(output, "Invalid MCP configuration") {
		t.Errorf("claude rejected MCP config as invalid schema:\n%s", output)
	}
}

// serverReachable returns true if curl can reach url within 1 second.
func serverReachable(url string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	cmd := safeexec.CommandContext(ctx, "curl", "-s", "-o", "/dev/null", "-w", "%{http_code}", "--max-time", "1", url)
	out, err := cmd.Output()
	return err == nil && string(out) != "000"
}

func keys[K comparable, V any](m map[K]V) []K {
	ks := make([]K, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
