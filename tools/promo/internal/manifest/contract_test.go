package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The example manifest is the wire contract shared with the TS recorder and the website
// reader (website/tests/unit/media-manifest-contract.test.ts parses this same file).
func TestExampleManifest_RoundTripsByteForByte(t *testing.T) {
	t.Parallel()
	path := filepath.Join("..", "..", "schema", "media-manifest.example.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err)

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m MediaManifest
	require.NoError(t, dec.Decode(&m), "every field in the example must exist on the Go struct")

	out, err := json.MarshalIndent(m, "", "  ")
	require.NoError(t, err)
	assert.Equal(t, string(bytes.TrimSpace(raw)), string(out))
	assert.Len(t, m.Entries, 6)
}
