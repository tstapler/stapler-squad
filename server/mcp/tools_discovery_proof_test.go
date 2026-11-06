package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/session"
)

const testProofSentinel = "ssqmcp1.sentinel-not-a-real-proof"

func TestGetSession_should_ReportProofPresentWithoutValue(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, session.MCPProofsDirName), 0o700))
	require.NoError(t, os.WriteFile(session.MCPProofFilePath(dir, "with-proof"), []byte(testProofSentinel), 0o600))
	d := &discoveryHandlers{store: &stubStore{instances: []*session.Instance{
		{Title: "with", UUID: "with-proof"}, {Title: "without", UUID: "no-proof"},
	}}}

	for title, want := range map[string]bool{"with": true, "without": false} {
		res, err := d.getSession(context.Background(), makeToolReq(map[string]interface{}{"session_id": title}))
		require.NoError(t, err)
		assert.NotContains(t, requireToolTextResult(t, res), testProofSentinel, "the proof value is never returned")
		assert.Equal(t, want, parseResult(t, res)["proof_present"], title)
	}
}
