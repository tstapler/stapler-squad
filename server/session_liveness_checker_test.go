package server

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

// fakeRegistryInstanceChecker records whether/how WithInstance was called, so
// tests can prove newSessionLivenessChecker's registry fallback fires only when
// no live instance is tracked — never for a session findLive already resolves.
// A real *session.Registry would otherwise reconstruct-and-Start() a throwaway
// Instance on every call (see newSessionLivenessChecker's doc comment in
// dependencies.go for the full incident this regression test guards against).
type fakeRegistryInstanceChecker struct {
	calls        int
	lastSession  string
	withInstance func(fn func(*session.LiveInstance) error) error
}

func (f *fakeRegistryInstanceChecker) WithInstance(_ context.Context, sessionID string, fn func(*session.LiveInstance) error) error {
	f.calls++
	f.lastSession = sessionID
	if f.withInstance != nil {
		return f.withInstance(fn)
	}
	// Default: never invoke fn (mirrors ErrSessionNotFound — ordinary WithInstance
	// callers treat that as "not alive" without dereferencing a nil LiveInstance).
	return nil
}

// TestNewSessionLivenessChecker_should_SkipRegistry_When_LiveInstanceIsTracked is the
// regression test for the review-gate-never-progresses bug (confirmed live 2026-07-20,
// backlog item 93565fa1): the liveness checker must answer from the already-live,
// already-wired instance without ever touching the registry — reaching the registry
// for a session findLive can already resolve would reconstruct-and-Start() a throwaway
// *session.Instance on every 60s ReconcileStuck tick, which is what actually broke the
// real review session's controller.
func TestNewSessionLivenessChecker_should_SkipRegistry_When_LiveInstanceIsTracked(t *testing.T) {
	inst, err := session.NewInstance(session.InstanceOptions{
		Title:       "review:93565fa1",
		Path:        t.TempDir(),
		Program:     "true",
		SessionType: session.SessionTypeDirectory,
	})
	if err != nil {
		t.Fatalf("session.NewInstance: %v", err)
	}

	findLive := func(sessionUUID string) *session.Instance {
		if sessionUUID != inst.UUID {
			t.Fatalf("findLive called with unexpected sessionUUID %q, want %q", sessionUUID, inst.UUID)
		}
		return inst
	}
	registry := &fakeRegistryInstanceChecker{}

	checker := newSessionLivenessChecker(findLive, registry)
	got := checker(inst.UUID)

	if want := inst.TmuxSessionExists(); got != want {
		t.Errorf("checker(%q) = %v, want %v (inst.TmuxSessionExists())", inst.UUID, got, want)
	}
	if registry.calls != 0 {
		t.Errorf("registry.WithInstance called %d time(s), want 0 — a tracked live instance must never fall through to the reconstruct-happy registry path", registry.calls)
	}
}

// TestNewSessionLivenessChecker_should_FallBackToRegistry_When_NoLiveInstanceIsTracked
// proves the fallback path is preserved for the case it actually exists for: a session
// with no currently-tracked live instance (e.g. immediately after a server restart,
// before the startup reconciliation loop reloads it).
func TestNewSessionLivenessChecker_should_FallBackToRegistry_When_NoLiveInstanceIsTracked(t *testing.T) {
	const sessionUUID = "no-live-instance-tracked"
	findLive := func(string) *session.Instance { return nil }
	registry := &fakeRegistryInstanceChecker{
		withInstance: func(fn func(*session.LiveInstance) error) error {
			// Simulate ErrSessionNotFound: fn is never invoked, alive stays false.
			return nil
		},
	}

	checker := newSessionLivenessChecker(findLive, registry)
	got := checker(sessionUUID)

	if got {
		t.Errorf("checker(%q) = true, want false (fallback found no session)", sessionUUID)
	}
	if registry.calls != 1 {
		t.Errorf("registry.WithInstance called %d time(s), want exactly 1", registry.calls)
	}
	if registry.lastSession != sessionUUID {
		t.Errorf("registry.WithInstance called with sessionID %q, want %q", registry.lastSession, sessionUUID)
	}
}
