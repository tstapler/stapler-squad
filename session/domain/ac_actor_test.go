package domain

import (
	"context"
	"testing"
)

func TestAcActorFrom_should_DefaultToSystem_When_NoActorAttached(t *testing.T) {
	t.Parallel()
	a := AcActorFrom(context.Background())
	if a.Class != AcActorSystem || a.Source != "system" {
		t.Fatalf("got %+v", a)
	}
	if got := a.Label(); got != "system (automated)" {
		t.Fatalf("label = %q", got)
	}
}

func TestAcActorFrom_should_ReturnAttachedActor(t *testing.T) {
	t.Parallel()
	want := AcActor{Class: AcActorAgentSession, SessionUUID: "u", Source: "api"}
	if got := AcActorFrom(WithAcActor(context.Background(), want)); got != want {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestAcActorClass_values_StayStable(t *testing.T) {
	t.Parallel()
	for class, want := range map[AcActorClass]string{
		AcActorAuthenticatedSession: "authenticated_session_cookie",
		AcActorAuthenticatedBearer:  "authenticated_bearer_token",
		AcActorAgentSession:         "agent_session",
		AcActorMCPSession:           "mcp_session",
		AcActorSystem:               "system",
		AcActorUnverified:           "unverified",
	} {
		if string(class) != want {
			t.Errorf("class %q != %q (persisted in ac_audit_event.actor_class)", class, want)
		}
	}
}

func TestAcActorLabel_should_MapEveryClass(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		actor AcActor
		want  string
	}{
		"cookie":           {AcActor{Class: AcActorAuthenticatedSession}, "session cookie credential"},
		"bearer":           {AcActor{Class: AcActorAuthenticatedBearer}, "authenticated bearer token"},
		"system":           {AcActor{Class: AcActorSystem}, "system (automated)"},
		"agent verified":   {AcActor{Class: AcActorAgentSession, SessionUUID: "s1", SessionVerified: true}, "agent session s1 (verified)"},
		"agent unverified": {AcActor{Class: AcActorAgentSession, SessionUUID: "s1"}, "claimed agent session s1 (unverified claim)"},
		"mcp verified":     {AcActor{Class: AcActorMCPSession, SessionUUID: "s2", SessionVerified: true}, "agent session s2 (verified)"},
		"mcp unverified":   {AcActor{Class: AcActorMCPSession, SessionUUID: "s2"}, "claimed agent session s2 (unverified claim)"},
		"unverified":       {AcActor{Class: AcActorUnverified}, "unverified caller"},
		"unknown class":    {AcActor{Class: "bogus"}, "unverified caller"},
		"zero value":       {AcActor{}, "unverified caller"},
	} {
		if got := tc.actor.Label(); got != tc.want {
			t.Errorf("%s: label = %q, want %q", name, got, tc.want)
		}
	}
}
