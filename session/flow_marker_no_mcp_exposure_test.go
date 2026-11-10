package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Markers are only trustworthy because no agent-reachable surface can write
// them; fail if the MCP package ever references the writer.
func TestFlowMarkerWriter_should_NotBeReachableFromMCP_When_ServerMcpScanned(t *testing.T) {
	t.Parallel()
	dir := filepath.Join("..", "server", "mcp")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	scanned := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		scanned++
		require.NotContains(t, string(b), "RecordFlowMarker", e.Name())
		require.NotContains(t, string(b), "FlowMarker", e.Name())
		require.NotContains(t, string(b), "ManualNudge", e.Name())
	}
	require.Positive(t, scanned)
}
