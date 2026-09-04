package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/tymux"
)

// captureLogWarn temporarily redirects the default slog handler to a text
// handler writing into the returned buffer, restoring the previous handler
// via the returned cleanup func. Used to assert that log.Warn was actually
// invoked (and named the offending entry) rather than only checking the
// returned slice.
func captureLogWarn(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.SetSlogDefaultForTest(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		log.SetSlogDefaultForTest(prev)
	})
	return &buf
}

func Test_parseExtraOrigins_should_AcceptEntry_When_GivenWellFormedHttpLocalhostOrigin(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"http localhost", "http://localhost:54212"},
		{"https 127.0.0.1", "https://127.0.0.1:9999"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid, rejected := parseExtraOrigins(tt.entry)

			if len(rejected) != 0 {
				t.Fatalf("expected no rejected entries, got %v", rejected)
			}
			if len(valid) != 1 || valid[0] != tt.entry {
				t.Fatalf("expected valid=[%q], got %v", tt.entry, valid)
			}
		})
	}
}

func Test_parseExtraOrigins_should_RejectAndLogWarning_When_GivenMalformedEntry(t *testing.T) {
	tests := []struct {
		name  string
		entry string
	}{
		{"not a URL", "not-a-valid-origin"},
		{"wildcard", "http://*"},
		{"has path", "http://localhost:1234/path"},
		{"non-localhost host", "http://example.com:1234"},
		{"missing port", "http://localhost"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLogWarn(t)

			valid, rejected := parseExtraOrigins(tt.entry)

			if len(valid) != 0 {
				t.Fatalf("expected no valid entries, got %v", valid)
			}
			if len(rejected) != 1 || rejected[0] != tt.entry {
				t.Fatalf("expected rejected=[%q], got %v", tt.entry, rejected)
			}
			if !strings.Contains(buf.String(), tt.entry) {
				t.Fatalf("expected a warning log line naming the offending entry %q, got log output: %s", tt.entry, buf.String())
			}
		})
	}
}

func Test_parseExtraOrigins_should_AcceptValidEntry_And_RejectInvalidEntry_When_BothPresentInOneCommaSeparatedList(t *testing.T) {
	buf := captureLogWarn(t)

	valid, rejected := parseExtraOrigins("http://localhost:54212,not-a-valid-origin")

	if len(valid) != 1 || valid[0] != "http://localhost:54212" {
		t.Fatalf(`expected valid=["http://localhost:54212"], got %v`, valid)
	}
	if len(rejected) != 1 || rejected[0] != "not-a-valid-origin" {
		t.Fatalf(`expected rejected=["not-a-valid-origin"], got %v`, rejected)
	}
	if !strings.Contains(buf.String(), "not-a-valid-origin") {
		t.Fatalf("expected exactly one warning logged naming the offending entry, got log output: %s", buf.String())
	}
}

func Test_formatKnownHosts_should_PrintNoKnownHostsMessage_When_GivenEmptySnapshot(t *testing.T) {
	var buf bytes.Buffer

	formatKnownHosts(&buf, nil)

	got := buf.String()
	if !strings.Contains(got, "No known hosts") {
		t.Fatalf("expected a 'no known hosts' message, got: %q", got)
	}
}

func Test_formatKnownHosts_should_PrintHostIDAddressesAndLastSeen_When_GivenEntries(t *testing.T) {
	id, err := session.NewHostID()
	if err != nil {
		t.Fatalf("NewHostID() error = %v", err)
	}
	lastSeen := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	entries := []session.RegistryEntry{
		{
			HostID:            id,
			AdvertisedAddress: []string{"192.168.1.42:8543", "10.0.0.5:8543"},
			LastSeenAt:        lastSeen,
		},
	}

	var buf bytes.Buffer
	formatKnownHosts(&buf, entries)

	got := buf.String()
	for _, want := range []string{id.String(), "192.168.1.42:8543", "10.0.0.5:8543"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected output to contain %q, got: %s", want, got)
		}
	}
	if !strings.Contains(got, lastSeen.Local().Format(time.RFC3339)) {
		t.Errorf("expected output to contain formatted last-seen time, got: %s", got)
	}
}

func Test_formatKnownHosts_should_SortEntriesByHostID_When_GivenMultipleEntries(t *testing.T) {
	idA, err := session.NewHostID()
	if err != nil {
		t.Fatalf("NewHostID() error = %v", err)
	}
	idB, err := session.NewHostID()
	if err != nil {
		t.Fatalf("NewHostID() error = %v", err)
	}
	// Ensure a deterministic expected order regardless of generation order.
	first, second := idA, idB
	if first.String() > second.String() {
		first, second = second, first
	}

	entries := []session.RegistryEntry{
		{HostID: second, AdvertisedAddress: []string{"host-b:8543"}},
		{HostID: first, AdvertisedAddress: []string{"host-a:8543"}},
	}

	var buf bytes.Buffer
	formatKnownHosts(&buf, entries)

	got := buf.String()
	idxFirst := strings.Index(got, first.String())
	idxSecond := strings.Index(got, second.String())
	if idxFirst == -1 || idxSecond == -1 {
		t.Fatalf("expected both host IDs present in output, got: %s", got)
	}
	if idxFirst > idxSecond {
		t.Errorf("expected %q to appear before %q in sorted output, got: %s", first.String(), second.String(), got)
	}
}

// TestBuildLogConfig_DefaultsToInfoNotDebug guards against a bug where an
// unset ConsoleLevel zero-values to DEBUG (LogLevel's iota starts at
// DEBUG=0), which initializeWithConfig's min(FileLevel, ConsoleLevel) seeding
// then used to override an explicit FileLevel — flooding the log with DEBUG
// output on every server boot regardless of FileLevel's intended value.
func TestBuildLogConfig_DefaultsToInfoNotDebug(t *testing.T) {
	cfg := buildLogConfig(true, &config.Config{}, false)
	if cfg.FileLevel != log.INFO {
		t.Errorf("FileLevel = %v, want %v", cfg.FileLevel, log.INFO)
	}
	if cfg.ConsoleLevel != log.INFO {
		t.Errorf("ConsoleLevel = %v, want %v", cfg.ConsoleLevel, log.INFO)
	}
}

// Test_tymuxNeeded_should_ReturnExpected_When_GivenResolvedBackend covers
// Epic 2.2 Task 2.2.1a (project_plans/tymux-bundled-integration/implementation/
// plan.md): tymuxNeeded checks resolvedBackend == BackendTymux, independent
// of any TymuxSessionOverrides entries (each case here uses an empty cfg).
func Test_tymuxNeeded_should_ReturnExpected_When_GivenResolvedBackend(t *testing.T) {
	tests := []struct {
		name            string
		resolvedBackend session.ProcessManagerBackend
		want            bool
	}{
		{"tymux backend needs supervision", session.BackendTymux, true},
		{"tmux backend does not need supervision", session.BackendTmux, false},
		{"unknown backend does not need supervision", session.ProcessManagerBackend("bogus"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tymuxNeeded(&config.Config{}, tt.resolvedBackend)
			if got != tt.want {
				t.Errorf("tymuxNeeded(cfg, %q) = %v, want %v", tt.resolvedBackend, got, tt.want)
			}
		})
	}
}

// Test_tymuxNeeded_should_ReturnTrue_When_AnyTymuxSessionOverrideIsTrue
// covers the Phase 4 half of Task 2.2.1a: a per-session override set before
// this process started (e.g. via SetTymuxSessionOverride) must also trigger
// startup supervision, even when the resolved global default is BackendTmux.
func Test_tymuxNeeded_should_ReturnTrue_When_AnyTymuxSessionOverrideIsTrue(t *testing.T) {
	cfg := &config.Config{
		TymuxSessionOverrides: map[string]bool{
			"some-other-session": false,
			"canary-session":     true,
		},
	}

	if !tymuxNeeded(cfg, session.BackendTmux) {
		t.Fatal("tymuxNeeded() = false, want true: a true TymuxSessionOverrides entry must trigger supervision even when the global default is tmux")
	}
}

// Test_tymuxNeeded_should_ReturnFalse_When_AllTymuxSessionOverridesAreFalse
// confirms a session-name override forcing tmux does NOT itself trigger
// supervision when nothing else needs it.
func Test_tymuxNeeded_should_ReturnFalse_When_AllTymuxSessionOverridesAreFalse(t *testing.T) {
	cfg := &config.Config{
		TymuxSessionOverrides: map[string]bool{
			"some-session": false,
		},
	}

	if tymuxNeeded(cfg, session.BackendTmux) {
		t.Fatal("tymuxNeeded() = true, want false: an all-false override map must not trigger supervision on its own")
	}
}

// Test_tymuxNeeded_should_ReturnFalse_When_ConfigIsNil confirms the nil-safety
// guard added alongside the TymuxSessionOverrides check.
func Test_tymuxNeeded_should_ReturnFalse_When_ConfigIsNil(t *testing.T) {
	if tymuxNeeded(nil, session.BackendTmux) {
		t.Fatal("tymuxNeeded(nil, BackendTmux) = true, want false")
	}
}

// Test_superviseTymuxd_should_DecideRegisterStopAndError_When_GivenEachCombination
// covers the CRITICAL gap: main.go's cobra "runtime" phase decided whether to
// register a shutdown hook that could kill a sibling process's tymuxd
// entirely inline, with zero test coverage. superviseTymuxd extracts that
// decision so it's testable via fakes for ensure/registerStop instead of a
// real tymuxd subprocess or a real *warren.App.
func Test_superviseTymuxd_should_DecideRegisterStopAndError_When_GivenEachCombination(t *testing.T) {
	errSample := errors.New("ensure failed")

	testCases := []struct {
		name             string
		ensureErr        error
		spawned          bool
		keepServer       bool
		strictStartup    bool
		wantErr          bool
		wantRegisterStop bool
	}{
		{
			name:             "ErrorWithStrictStartup_ReturnsError",
			ensureErr:        errSample,
			strictStartup:    true,
			wantErr:          true,
			wantRegisterStop: false,
		},
		{
			name:             "ErrorWithoutStrictStartup_WarnsAndContinues",
			ensureErr:        errSample,
			strictStartup:    false,
			wantErr:          false,
			wantRegisterStop: false,
		},
		{
			name:             "ReusedNotSpawned_NeverRegistersStopHook",
			ensureErr:        nil,
			spawned:          false,
			keepServer:       false,
			wantErr:          false,
			wantRegisterStop: false,
		},
		{
			name:             "SpawnedAndNotKeepServer_RegistersStopHook",
			ensureErr:        nil,
			spawned:          true,
			keepServer:       false,
			wantErr:          false,
			wantRegisterStop: true,
		},
		{
			name:             "SpawnedButKeepServer_NeverRegistersStopHook",
			ensureErr:        nil,
			spawned:          true,
			keepServer:       true,
			wantErr:          false,
			wantRegisterStop: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeEnsure := func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
				return tymux.TymuxdReady{Spawned: tc.spawned}, tc.ensureErr
			}

			var registerStopCalls int
			fakeRegisterStop := func(name string, fn func(context.Context) error) {
				registerStopCalls++
			}

			err := superviseTymuxd(context.Background(), tymux.DaemonConfig{}, tc.strictStartup, tc.keepServer, fakeEnsure, fakeRegisterStop)

			if tc.wantErr && err == nil {
				t.Fatal("superviseTymuxd() error = nil, want non-nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("superviseTymuxd() error = %v, want nil", err)
			}

			gotRegisterStop := registerStopCalls > 0
			if gotRegisterStop != tc.wantRegisterStop {
				t.Errorf("registerStop called = %v, want %v (called %d times)", gotRegisterStop, tc.wantRegisterStop, registerStopCalls)
			}
		})
	}
}

// TestResolveStartupBackend_EmptyConfigDefaultsToTmux covers Epic 3.2 Task
// 3.2.1a/b (project_plans/tymux-bundled-integration/implementation/plan.md):
// with no ProcessManagerBackend set and no env var, the function must default
// to BackendTmux with no error — the pre-existing backwards-compatible
// behavior of the inline block it replaces.
func TestResolveStartupBackend_EmptyConfigDefaultsToTmux(t *testing.T) {
	backend, err := resolveStartupBackend(&config.Config{}, false)
	if err != nil {
		t.Fatalf("resolveStartupBackend() error = %v, want nil", err)
	}
	if backend != session.BackendTmux {
		t.Errorf("resolveStartupBackend() backend = %q, want %q", backend, session.BackendTmux)
	}
}

// TestResolveStartupBackend_TymuxConfigValueWithoutRehearsalFallsBackToTmux is
// the bypass-guard regression test named in the plan: hand-editing
// process_manager_backend: "tymux" directly into config.json, with no env var
// set and no rollback rehearsal recorded, must NOT bypass the rehearsal gate
// (research/pitfalls.md §3). It must resolve to BackendTmux and surface
// config.ErrTymuxRollbackRehearsalNotCompleted for the caller to log.
func TestResolveStartupBackend_TymuxConfigValueWithoutRehearsalFallsBackToTmux(t *testing.T) {
	cfg := &config.Config{ProcessManagerBackend: "tymux"} // hand-edited config.json, no rehearsal, no env var

	backend, err := resolveStartupBackend(cfg, false)

	if backend != session.BackendTmux {
		t.Errorf("resolveStartupBackend() backend = %q, want %q (bypass guard failed)", backend, session.BackendTmux)
	}
	if !errors.Is(err, config.ErrTymuxRollbackRehearsalNotCompleted) {
		t.Errorf("resolveStartupBackend() error = %v, want %v", err, config.ErrTymuxRollbackRehearsalNotCompleted)
	}
}

// TestResolveStartupBackend_TymuxConfigValueWithRehearsalCompletes verifies
// the config-value path DOES resolve to tymux once the rehearsal has been
// recorded — the gate blocks the unrehearsed case above, not the tymux value
// unconditionally.
func TestResolveStartupBackend_TymuxConfigValueWithRehearsalCompletes(t *testing.T) {
	completedAt := time.Now()
	cfg := &config.Config{
		ProcessManagerBackend:             "tymux",
		TymuxRollbackRehearsalCompletedAt: &completedAt,
	}

	backend, err := resolveStartupBackend(cfg, false)

	if err != nil {
		t.Fatalf("resolveStartupBackend() error = %v, want nil", err)
	}
	if backend != session.BackendTymux {
		t.Errorf("resolveStartupBackend() backend = %q, want %q", backend, session.BackendTymux)
	}
}

// TestResolveStartupBackend_EnvVarWithRehearsalCompletes verifies
// STAPLER_SQUAD_USE_TYMUX=true (tymuxEnvRequested=true) also resolves to
// tymux once the rehearsal is recorded, even when the config value itself is
// something other than "tymux" — the env var and the config value both feed
// the same tymuxRequested gate.
func TestResolveStartupBackend_EnvVarWithRehearsalCompletes(t *testing.T) {
	completedAt := time.Now()
	cfg := &config.Config{
		ProcessManagerBackend:             "tmux",
		TymuxRollbackRehearsalCompletedAt: &completedAt,
	}

	backend, err := resolveStartupBackend(cfg, true)

	if err != nil {
		t.Fatalf("resolveStartupBackend() error = %v, want nil", err)
	}
	if backend != session.BackendTymux {
		t.Errorf("resolveStartupBackend() backend = %q, want %q", backend, session.BackendTymux)
	}
}

// TestResolveStartupBackend_NativeBackendPassesThroughUnaffected verifies the
// gate only ever intercepts the tymux case — a "native" backend value passes
// through completely unaffected, with no error, matching the plan's
// acceptance criteria.
func TestResolveStartupBackend_NativeBackendPassesThroughUnaffected(t *testing.T) {
	cfg := &config.Config{ProcessManagerBackend: "native"}

	backend, err := resolveStartupBackend(cfg, false)

	if err != nil {
		t.Fatalf("resolveStartupBackend() error = %v, want nil", err)
	}
	if backend != session.BackendNative {
		t.Errorf("resolveStartupBackend() backend = %q, want %q", backend, session.BackendNative)
	}
}
