package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/session"
)

// toolOverHTTP runs a registered tool behind the real identity middleware, as
// /mcp does: the handler sees only the context the middleware built from the
// request headers.
func toolOverHTTP(t *testing.T, r *toolRig, mw func(http.Handler) http.Handler, tool string, headers map[string]string, args map[string]any) map[string]any {
	t.Helper()
	var out map[string]any
	inner := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		res, err := r.server.GetTool(tool).Handler(req.Context(), makeToolReq(args))
		require.NoError(t, err)
		out = parseResult(t, res)
		_ = json.NewEncoder(w).Encode(out)
	})
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	mw(inner).ServeHTTP(httptest.NewRecorder(), req)
	return out
}

func TestWakeTools_should_RefuseForgedHeaderAndCrossSessionProof_WithZeroWrites(t *testing.T) {
	t.Parallel()
	proofs := newTestProofs(t)
	live := map[string]bool{"A": true, "B": true}
	mw := func(next http.Handler) http.Handler {
		return NewIdentityMiddleware(next, proofs.Verify, func(id string) bool { return live[id] })
	}
	r := newToolRig(t)
	r.owners.facts["A"], r.owners.facts["B"] = liveOwner(), liveOwner()
	// A request addressed to B, created before the attacks, to prove it stays unchanged.
	pending := createPeerRow(t, r, "A", "B")
	wake := &session.WakeTriggerData{ID: uuid.NewString(), OwnerSessionUUID: "B"}
	r.backend.live = append(r.backend.live, wake)

	args := map[string]map[string]any{
		toolScheduleWake: {"trigger_type": "at", "message": "m", "delay_minutes": float64(5)},
		toolListWakes:    {},
		toolCancelWake:   {"wake_id": wake.ID},
		toolRequestHelp:  {"peer": "A", "question": "q"},
		toolAnswerHelp:   {"request_id": pending, "answer": "forged answer"},
	}
	attacks := map[string]map[string]string{
		"forged uuid header, no proof":             {"X-Stapler-Session-UUID": "B"},
		"garbage proof":                            {"X-Stapler-Session-UUID": "B", session.MCPProofHeader: "ssqmcp1.zzzz"},
		"session A's valid proof with B's uuid":    {"X-Stapler-Session-UUID": "B", session.MCPProofHeader: proofs.HeaderValue("A")},
		"valid proof of a session that is deleted": {"X-Stapler-Session-UUID": "gone", session.MCPProofHeader: proofs.HeaderValue("gone")},
	}
	for tool, a := range args {
		for name, headers := range attacks {
			t.Run(tool+"/"+name, func(t *testing.T) {
				m := toolOverHTTP(t, r, mw, tool, headers, a)
				assert.Equal(t, CodeIdentityUnverified, resultCode(m))
			})
		}
	}

	assert.Empty(t, r.backend.scheduleCalls(), "no wake was created by any attack")
	row, err := r.storage.GetGuidanceRequest(context.Background(), uuid.MustParse(pending))
	require.NoError(t, err)
	assert.Equal(t, session.GuidanceRequestStatusPending, row.Status(), "the request addressed to B is unchanged")
	assert.Empty(t, r.answeredIDs())
	assert.Empty(t, r.openPeerRows("B"), "request_help created no row as B")
	for tool := range args {
		assert.EqualValues(t, len(attacks), r.metrics.Value("mcp_identity_unverified_total", tool), tool)
	}
}

func TestWakeTools_should_Proceed_When_ProofMatchesTheClaimedSession(t *testing.T) {
	t.Parallel()
	proofs := newTestProofs(t)
	mw := func(next http.Handler) http.Handler {
		return NewIdentityMiddleware(next, proofs.Verify, func(id string) bool { return id == "A" })
	}
	r := newToolRig(t)
	headers := map[string]string{"X-Stapler-Session-UUID": "A", session.MCPProofHeader: proofs.HeaderValue("A")}

	m := toolOverHTTP(t, r, mw, toolScheduleWake, headers, map[string]any{"trigger_type": "at", "message": "m", "delay_minutes": float64(5)})

	require.True(t, m["success"].(bool), "%v", m)
	require.Len(t, r.backend.scheduleCalls(), 1)
	assert.Equal(t, "A", r.backend.scheduleCalls()[0].Owner)
}

// createPeerRow has A ask B directly through the tool as a verified A.
func createPeerRow(t *testing.T, r *toolRig, asker, peer string) string {
	t.Helper()
	m := r.call(as(asker), toolRequestHelp, map[string]any{"peer": peer, "question": "How do I run the linter?"})
	require.True(t, m["success"].(bool), "%v", m)
	r.backend.mu.Lock()
	r.backend.scheduled = nil // forget the setup's wakes; the attacks must add none
	r.backend.mu.Unlock()
	return m["request_id"].(string)
}
