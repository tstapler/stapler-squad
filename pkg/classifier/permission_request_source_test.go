package classifier

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPermissionRequestPayload_Source_DefaultsEmpty_When_ClaudeBodyOmitsIt verifies
// pi-support Story 4.3.1's backward-compatibility AC: Claude's existing curl-based
// hook body (no "source" key at all) still parses successfully with Source == "".
func TestPermissionRequestPayload_Source_DefaultsEmpty_When_ClaudeBodyOmitsIt(t *testing.T) {
	claudeBody := []byte(`{
		"session_id": "abc",
		"transcript_path": "",
		"cwd": "/repo",
		"permission_mode": "",
		"hook_event_name": "PermissionRequest",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"}
	}`)

	var payload PermissionRequestPayload
	require.NoError(t, json.Unmarshal(claudeBody, &payload))
	assert.Equal(t, "", payload.Source)
	assert.Equal(t, "Bash", payload.ToolName)
}

// TestPermissionRequestPayload_Source_ParsesPi_When_ExtensionBodyIncludesIt verifies the
// pi extension's new body (source: "pi") parses with Source == "pi".
func TestPermissionRequestPayload_Source_ParsesPi_When_ExtensionBodyIncludesIt(t *testing.T) {
	piBody := []byte(`{
		"session_id": "",
		"cwd": "/repo",
		"hook_event_name": "PermissionRequest",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"},
		"source": "pi"
	}`)

	var payload PermissionRequestPayload
	require.NoError(t, json.Unmarshal(piBody, &payload))
	assert.Equal(t, "pi", payload.Source)
}
