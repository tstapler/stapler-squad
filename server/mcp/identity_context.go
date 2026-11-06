package mcp

import (
	"context"
	"net/http"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session"
)

// IdentityUnverifiedMessage is the error copy for a caller without a verified
// session credential. It must not tell the agent to set STAPLER_SESSION_UUID:
// the identity comes from a per-session proof file, not the environment.
const IdentityUnverifiedMessage = "this session has no verified credential for stapler-squad; relaunch the session to receive one"

const sessionUUIDHeader = "X-Stapler-Session-UUID"

type verifiedSessionUUIDKey struct{}

// WithVerifiedSessionUUID marks id as a caller identity whose proof verified.
// Only the HTTP identity middleware may call it (ADR-007 rule 0).
func WithVerifiedSessionUUID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, verifiedSessionUUIDKey{}, id)
}

// verifiedCallerSessionUUID returns the caller's session UUID only when its
// proof verified. Tools that grant authority by identity use this, never the
// unverified callerSessionUUID, which is a client claim.
func verifiedCallerSessionUUID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(verifiedSessionUUIDKey{}).(string)
	return v, ok && v != ""
}

// ProofVerifier checks a proof value against the session it claims to be for.
type ProofVerifier func(sessionUUID, proof string) bool

// NewIdentityMiddleware wraps next so the context carries the caller identity:
// the claimed X-Stapler-Session-UUID always as the unverified WithSessionUUID
// (unchanged for pre-existing tools), and additionally as the verified
// identity only when X-Stapler-Session-Proof verifies for that UUID and
// sessionExists reports a live session (a proof outlives a deleted session,
// so liveness is checked separately). A nil verifier verifies nothing.
func NewIdentityMiddleware(next http.Handler, verify ProofVerifier, sessionExists func(sessionUUID string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if id := r.Header.Get(sessionUUIDHeader); id != "" {
			ctx := WithSessionUUID(r.Context(), id)
			if proof := r.Header.Get(session.MCPProofHeader); proof != "" && verify != nil && verify(id, proof) &&
				sessionExists != nil && sessionExists(id) {
				ctx = WithVerifiedSessionUUID(ctx, id)
			}
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// sessionHasMCPProof reports whether the session has a proof file; the value
// is never read or returned.
func sessionHasMCPProof(sessionUUID string) bool {
	dir, err := config.GetConfigDir()
	return err == nil && session.MCPProofFilePresent(dir, sessionUUID)
}
