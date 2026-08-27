package session

import (
	"testing"

	"github.com/tstapler/stapler-squad/config"
)

func TestResolveSessionBackend_RequestOverrideWinsOverEverything(t *testing.T) {
	RegisterBackendProvider(BackendTymux)
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	cfg := &config.Config{
		TymuxSessionOverrides: map[string]bool{"my-session": false},
	}

	got := ResolveSessionBackend(cfg, "my-session", BackendNative)

	if got != BackendNative {
		t.Fatalf("ResolveSessionBackend() = %q, want %q (request override must win over both the session override and the global default)", got, BackendNative)
	}
}

func TestResolveSessionBackend_SessionOverrideForcesTymuxOverGlobalTmux(t *testing.T) {
	RegisterBackendProvider(BackendTmux)
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	cfg := &config.Config{
		TymuxSessionOverrides: map[string]bool{"my-session": true},
	}

	got := ResolveSessionBackend(cfg, "my-session", "")

	if got != BackendTymux {
		t.Fatalf("ResolveSessionBackend() = %q, want %q (a true session override must win over a BackendTmux global default)", got, BackendTymux)
	}
}

// TestResolveSessionBackend_SessionOverrideForcesTmuxOverGlobalTymux is the
// direct regression guard for the streamhub bug class documented on
// ResolveSessionBackend and streamhub/ownership.go's resolveLocked: a `false`
// config override must be able to push the effective backend back to
// BackendTmux even when the global default is already BackendTymux.
// resolveLocked's `effective := flagValue; if ok && forceHub { effective =
// true }` combinator structurally cannot do this — once effective is true,
// nothing in that code path can set it back to false. ResolveSessionBackend
// returns the override outright instead of OR-ing it into the global, so it
// does not have that limitation.
func TestResolveSessionBackend_SessionOverrideForcesTmuxOverGlobalTymux(t *testing.T) {
	RegisterBackendProvider(BackendTymux)
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	cfg := &config.Config{
		TymuxSessionOverrides: map[string]bool{"my-session": false},
	}

	got := ResolveSessionBackend(cfg, "my-session", "")

	if got != BackendTmux {
		t.Fatalf("ResolveSessionBackend() = %q, want %q (a false session override must win over a BackendTymux global default — this is the resolveLocked bug class this function must not replicate)", got, BackendTmux)
	}
}

func TestResolveSessionBackend_FallsBackToGlobalWhenNoOverrides(t *testing.T) {
	RegisterBackendProvider(BackendTymux)
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	cfg := &config.Config{}

	got := ResolveSessionBackend(cfg, "my-session", "")

	if got != BackendTymux {
		t.Fatalf("ResolveSessionBackend() = %q, want %q (no request override and no session override must fall back to the global default)", got, BackendTymux)
	}
}

func TestResolveSessionBackend_FallsBackToTmuxWhenNothingSet(t *testing.T) {
	RegisterBackendProvider("")
	defer RegisterBackendProvider(BackendTmux) // restore default for other tests

	cfg := &config.Config{}

	got := ResolveSessionBackend(cfg, "my-session", "")

	if got != BackendTmux {
		t.Fatalf("ResolveSessionBackend() = %q, want %q (nothing set anywhere must fall back to BackendTmux)", got, BackendTmux)
	}
}
