package domain

import (
	"context"
	"fmt"
)

// AcActorClass says how a caller identified itself. None of these prove the caller is "the owner":
// authenticated classes only show a credential was presented, an agent class is a claim unless
// SessionVerified, and Unverified means no identity signal at all (on the local loopback surface
// an agent's curl and the operator's own CLI look identical).
type AcActorClass string

const (
	AcActorAuthenticatedSession AcActorClass = "authenticated_session_cookie"
	AcActorAuthenticatedBearer  AcActorClass = "authenticated_bearer_token"
	AcActorAgentSession         AcActorClass = "agent_session"
	AcActorMCPSession           AcActorClass = "mcp_session"
	AcActorSystem               AcActorClass = "system"
	AcActorUnverified           AcActorClass = "unverified"
)

// AcActor is the best-available caller identity recorded with an AC mutation.
type AcActor struct {
	Class           AcActorClass
	SessionUUID     string
	SessionVerified bool
	Source          string // ui, api or mcp
	RemoteAddr      string
	UserAgent       string
}

type acActorKey struct{}

// WithAcActor attaches the caller identity the AC audit trail will record.
func WithAcActor(ctx context.Context, a AcActor) context.Context {
	return context.WithValue(ctx, acActorKey{}, a)
}

// AcActorFrom returns the attached actor. Request handlers always attach one; a context without
// an actor belongs to a non-request writer (lifecycle, sync, verdict application), recorded as system.
func AcActorFrom(ctx context.Context) AcActor {
	if a, ok := ctx.Value(acActorKey{}).(AcActor); ok {
		return a
	}
	return AcActor{Class: AcActorSystem, Source: "system"}
}

// Label is the human-readable "who" shown next to a waiver.
func (a AcActor) Label() string {
	switch a.Class {
	case AcActorAuthenticatedSession:
		return "session cookie credential"
	case AcActorAuthenticatedBearer:
		return "authenticated bearer token"
	case AcActorSystem:
		return "system (automated)"
	case AcActorAgentSession, AcActorMCPSession:
		if a.SessionVerified {
			return fmt.Sprintf("agent session %s (verified)", a.SessionUUID)
		}
		return fmt.Sprintf("claimed agent session %s (unverified claim)", a.SessionUUID)
	default:
		return "unverified caller"
	}
}
