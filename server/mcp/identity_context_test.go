package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/envtest"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

// identityProbe records what the middleware put in the request context.
type identityProbe struct {
	claimed, verified string
	claimedOK         bool
	verifiedOK        bool
}

func (p *identityProbe) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	p.claimed, p.claimedOK = sessionUUIDFromContext(r.Context())
	p.verified, p.verifiedOK = verifiedCallerSessionUUID(r.Context())
}

func newTestProofs(t *testing.T) *services.McpProofs {
	t.Helper()
	dir := t.TempDir()
	keys, err := services.NewHookProofs(dir)
	require.NoError(t, err)
	return services.NewMcpProofs(keys, dir)
}

func probeRequest(t *testing.T, mw http.Handler, headers map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	mw.ServeHTTP(httptest.NewRecorder(), req)
}

func TestIdentityMiddleware_should_VerifyOnlyMatchingLiveProof(t *testing.T) {
	t.Parallel()
	proofs := newTestProofs(t)
	live := map[string]bool{"A": true, "B": true}
	exists := func(id string) bool { return live[id] }
	headers := func(uuid, proof string) map[string]string {
		h := map[string]string{"X-Stapler-Session-UUID": uuid}
		if proof != "" {
			h[session.MCPProofHeader] = proof
		}
		return h
	}
	tests := []struct {
		name         string
		headers      map[string]string
		wantVerified string
	}{
		{"valid proof", headers("A", proofs.HeaderValue("A")), "A"},
		{"forged uuid, no proof", headers("A", ""), ""},
		{"session B uuid with session A proof", headers("B", proofs.HeaderValue("A")), ""},
		{"garbage proof", headers("A", "ssqmcp1.zzzz"), ""},
		{"valid proof for a deleted session", headers("gone", proofs.HeaderValue("gone")), ""},
		{"proof without uuid", map[string]string{session.MCPProofHeader: proofs.HeaderValue("A")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			probe := &identityProbe{}
			mw := NewIdentityMiddleware(probe, proofs.Verify, exists)
			probeRequest(t, mw, tt.headers)
			assert.Equal(t, tt.wantVerified, probe.verified)
			assert.Equal(t, tt.wantVerified != "", probe.verifiedOK)
			// The unverified claim is unchanged for pre-existing tools.
			want := tt.headers["X-Stapler-Session-UUID"]
			assert.Equal(t, want, probe.claimed)
			assert.Equal(t, want != "", probe.claimedOK)
		})
	}
}

func TestIdentityMiddleware_should_VerifyNothing_When_NoVerifierOrLookup(t *testing.T) {
	t.Parallel()
	proofs := newTestProofs(t)
	h := map[string]string{"X-Stapler-Session-UUID": "A", session.MCPProofHeader: proofs.HeaderValue("A")}

	probe := &identityProbe{}
	probeRequest(t, NewIdentityMiddleware(probe, nil, func(string) bool { return true }), h)
	assert.False(t, probe.verifiedOK, "nil verifier")

	probe = &identityProbe{}
	probeRequest(t, NewIdentityMiddleware(probe, proofs.Verify, nil), h)
	assert.False(t, probe.verifiedOK, "nil liveness lookup")
}

func TestVerifiedCallerSessionUUID_should_BeAbsent_When_OnlyUnverifiedClaimInContext(t *testing.T) {
	t.Parallel()
	ctx := WithSessionUUID(context.Background(), "A")
	_, ok := verifiedCallerSessionUUID(ctx)
	assert.False(t, ok, "in-process RunStdio sets only the unverified UUID")

	id, ok := verifiedCallerSessionUUID(WithVerifiedSessionUUID(ctx, "A"))
	assert.True(t, ok)
	assert.Equal(t, "A", id)
}

func TestIdentityUnverifiedMessage_should_NotMentionEnvironmentVariable(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, IdentityUnverifiedMessage, "STAPLER_SESSION_UUID")
	assert.Contains(t, IdentityUnverifiedMessage, "relaunch")
}

func TestSessionHasMCPProof_should_ReportFilePresenceNeverValue(t *testing.T) {
	dir := envtest.NewIsolatedStateDir(t)
	assert.False(t, sessionHasMCPProof("s1"))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, session.MCPProofsDirName), 0o700))
	require.NoError(t, os.WriteFile(session.MCPProofFilePath(dir, "s1"), []byte("x"), 0o600))
	assert.True(t, sessionHasMCPProof("s1"))
	assert.False(t, sessionHasMCPProof("../s1"))
}
