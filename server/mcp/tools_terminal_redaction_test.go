package mcp

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

const redactionTestProof = session.MCPProofValuePrefix + "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func newRedactionHandlers(t *testing.T, sessionID, output string) *terminalHandlers {
	t.Helper()
	mgr := makeScrollbackMgr(t)
	require.NoError(t, mgr.AppendOutput(sessionID, []byte(output)))
	return &terminalHandlers{
		store:      &stubStore{instances: []*session.Instance{{Title: sessionID}}},
		scrollback: mgr,
		writeLim:   newTokenBucket(10, 10),
	}
}

func TestReadSessionOutput_should_RedactProofValue(t *testing.T) {
	t.Parallel()
	th := newRedactionHandlers(t, "s1", "before\nX-Stapler-Session-Proof: "+redactionTestProof+"\nafter\n")

	res, err := th.readSessionOutput(context.Background(), makeToolReq(map[string]interface{}{"session_id": "s1"}))
	require.NoError(t, err)
	out, _ := parseResult(t, res)["output"].(string)

	assert.NotContains(t, out, redactionTestProof)
	assert.Contains(t, out, "X-Stapler-Session-Proof: [redacted-proof]")
	assert.Contains(t, out, "after")
}

func TestWaitForOutput_should_RedactProofInMatchedLineAndOutput(t *testing.T) {
	t.Parallel()
	th := newRedactionHandlers(t, "s2", "noise\ncat result "+redactionTestProof+" end\n")

	res, err := th.waitForOutput(context.Background(), makeToolReq(map[string]interface{}{
		"session_id": "s2", "pattern": "cat result", "timeout_seconds": float64(5),
	}))
	require.NoError(t, err)
	m := parseResult(t, res)

	assert.Equal(t, true, m["matched"])
	line, _ := m["matched_line"].(string)
	out, _ := m["output"].(string)
	assert.NotContains(t, line, redactionTestProof)
	assert.NotContains(t, out, redactionTestProof)
	assert.Contains(t, line, "[redacted-proof]")
}
