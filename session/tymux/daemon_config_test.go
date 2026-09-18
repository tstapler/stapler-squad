package tymux

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDaemonConfig_should_UseDefaultAddr_When_NoEnvVarsSet(t *testing.T) {
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")

	cfg := ResolveDaemonConfig()

	if cfg.Addr != defaultTymuxdAddr {
		t.Errorf("Addr = %q, want default %q", cfg.Addr, defaultTymuxdAddr)
	}
	if cfg.Addr != "http://127.0.0.1:7419" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, "http://127.0.0.1:7419")
	}
}

func TestResolveDaemonConfig_should_UseDefaultAddr_When_InstanceIsShared(t *testing.T) {
	// "shared" is this codebase's established convention for "explicitly
	// selecting the default/shared instance" (config.IsNamedInstance's
	// inverse condition, config.GetConfigDirForDir's "shared" backward-
	// compatibility carve-out, docs/reference/state-isolation.md) — it must
	// resolve identically to unset, not derive a distinct instance-scoped
	// port. Regression guard for the adversarial-review-caught gap where
	// this only checked instanceID == "".
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "shared")

	cfg := ResolveDaemonConfig()

	if cfg.Addr != defaultTymuxdAddr {
		t.Errorf("Addr = %q, want default %q (STAPLER_SQUAD_INSTANCE=shared must match unset)", cfg.Addr, defaultTymuxdAddr)
	}
}

func TestResolveDaemonConfig_should_DeriveDistinctPort_When_InstanceSet(t *testing.T) {
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "claude-manual-test")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir()) // resolveSocketPath now touches config.GetConfigDir()

	cfg := ResolveDaemonConfig()

	// crc32.ChecksumIEEE("claude-manual-test") % 1000 == 118; 7420 + 118 == 7538.
	const want = "http://127.0.0.1:7538"
	if cfg.Addr != want {
		t.Errorf("Addr = %q, want exact deterministic addr %q", cfg.Addr, want)
	}
}

func TestResolveDaemonConfig_should_BeDeterministic_When_SameInstanceNameUsedTwice(t *testing.T) {
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "e2e-local")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	first := ResolveDaemonConfig()
	second := ResolveDaemonConfig()

	if first.Addr != second.Addr {
		t.Errorf("same instance name produced different addrs: %q vs %q", first.Addr, second.Addr)
	}
	const want = "http://127.0.0.1:7830"
	if first.Addr != want {
		t.Errorf("Addr = %q, want exact deterministic addr %q", first.Addr, want)
	}
}

func TestResolveDaemonConfig_should_DeriveDifferentPorts_When_DifferentInstanceNamesUsed(t *testing.T) {
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	t.Setenv("STAPLER_SQUAD_INSTANCE", "instance-a")
	addrA := ResolveDaemonConfig().Addr

	t.Setenv("STAPLER_SQUAD_INSTANCE", "instance-b")
	addrB := ResolveDaemonConfig().Addr

	const wantA = "http://127.0.0.1:7457"
	const wantB = "http://127.0.0.1:7683"
	if addrA != wantA {
		t.Errorf("addrA = %q, want exact deterministic addr %q", addrA, wantA)
	}
	if addrB != wantB {
		t.Errorf("addrB = %q, want exact deterministic addr %q", addrB, wantB)
	}
	if addrA == addrB {
		t.Errorf("different instance names produced the same addr %q; expected distinct ports", addrA)
	}
}

func TestResolveDaemonConfig_should_PreferTymuxdAddrEnvVar_When_InstanceAlsoSet(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "some-instance")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	t.Setenv("TYMUXD_ADDR", "http://127.0.0.1:9999")

	cfg := ResolveDaemonConfig()

	if cfg.Addr != "http://127.0.0.1:9999" {
		t.Errorf("Addr = %q, want TYMUXD_ADDR override %q", cfg.Addr, "http://127.0.0.1:9999")
	}
}

func TestResolveDaemonConfig_should_LeaveSocketPathEmpty_When_InstanceUnsetOrShared(t *testing.T) {
	for _, instanceID := range []string{"", "shared"} {
		t.Setenv("TYMUXD_ADDR", "")
		t.Setenv("TYMUXD_SOCKET_PATH", "")
		t.Setenv("STAPLER_SQUAD_INSTANCE", instanceID)

		if got := resolveSocketPath(); got != "" {
			t.Errorf("instanceID %q: SocketPath = %q, want empty (tymuxd's own default)", instanceID, got)
		}
	}
}

func TestResolveDaemonConfig_should_DeriveSocketPathUnderInstanceConfigDir_When_InstanceSet(t *testing.T) {
	// This is the fix for the confirmed collision: two STAPLER_SQUAD_INSTANCEs
	// with distinct TYMUXD_ADDR ports still fought over tymuxd's single
	// process-wide default Unix-socket lock. Regression guard for that.
	testDir := t.TempDir()
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("TYMUXD_SOCKET_PATH", "")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "claude-manual-test")

	got := resolveSocketPath()

	// testDir (STAPLER_SQUAD_TEST_DIR) is configDir here (Priority 1 wins),
	// so the expected path is computed the same way resolveSocketPath itself
	// derives it -- see that function's doc comment for why it's a hash of
	// configDir under os.TempDir(), not a subdirectory of configDir directly.
	hash := sha256.Sum256([]byte(testDir))
	sockDir := filepath.Join(os.TempDir(), "ssq-tymux-"+hex.EncodeToString(hash[:8]))
	want := filepath.Join(sockDir, "tymuxd.sock")
	if got != want {
		t.Errorf("SocketPath = %q, want %q", got, want)
	}
	// tymuxd refuses to bind a socket whose parent directory isn't owned by
	// the caller at exactly mode 0700 (confirmed empirically against the real
	// binary) — a regression here would silently reintroduce a startup
	// failure indistinguishable from the original collision this fixes.
	info, err := os.Stat(sockDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("expected socket dir %q to exist after resolveSocketPath, stat err = %v", sockDir, err)
	}
	if perm := info.Mode().Perm(); perm != 0700 {
		t.Errorf("socket dir %q mode = %o, want 0700 (tymuxd requires exactly this)", sockDir, perm)
	}
}

// TestResolveDaemonConfig_should_StaySafelyShort_When_ConfigDirIsVeryLong is
// the regression guard for a real failure the socket-path derivation's
// first version hit: Unix domain socket paths are kernel-length-limited
// (sun_path, ~108 bytes
// on Linux) -- nesting the socket inside configDir directly (the original
// approach) produced "path must be shorter than SUN_LEN" from the real
// binary once configDir was a few directories deep (a long
// STAPLER_SQUAD_INSTANCE name, or a deeply-nested STAPLER_SQUAD_TEST_DIR
// under a test's own tmp dir -- confirmed against
// session/tymux/supervise_integration_test.go's real-binary tests). Hashing
// configDir into a short, fixed-length token under os.TempDir() bounds the
// result regardless of how long configDir itself is.
func TestResolveDaemonConfig_should_StaySafelyShort_When_ConfigDirIsVeryLong(t *testing.T) {
	// Longer than any real sun_path budget could tolerate as a raw nested
	// path, deliberately: this must resolve to a short path anyway.
	longDir := filepath.Join(t.TempDir(), strings.Repeat("a-very-long-directory-name-segment/", 5))
	require.NoError(t, os.MkdirAll(longDir, 0700))
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("TYMUXD_SOCKET_PATH", "")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", longDir)
	t.Setenv("STAPLER_SQUAD_INSTANCE", "claude-manual-test")

	got := resolveSocketPath()

	require.NotEmpty(t, got, "resolution must not silently fail just because configDir is long")
	// 108 is Linux's traditional sun_path size; assert comfortably under it
	// (not against the exact constant, which is platform-specific) so this
	// stays a meaningful regression guard without hardcoding OS internals.
	assert.Less(t, len(got), 90, "resolved socket path must stay well under Unix domain socket length limits regardless of configDir's own length")
}

func TestResolveDaemonConfig_should_PreferTymuxdSocketPathEnvVar_When_InstanceAlsoSet(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_INSTANCE", "some-instance")
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	t.Setenv("TYMUXD_SOCKET_PATH", "/custom/tymuxd.sock")

	if got := resolveSocketPath(); got != "/custom/tymuxd.sock" {
		t.Errorf("SocketPath = %q, want TYMUXD_SOCKET_PATH override %q", got, "/custom/tymuxd.sock")
	}
}

func TestResolveDaemonConfig_should_SetBinaryPathFromTymuxdBinary(t *testing.T) {
	t.Setenv("TYMUXD_ADDR", "")
	t.Setenv("STAPLER_SQUAD_INSTANCE", "")
	t.Setenv("TYMUXD_BIN", "/custom/path/to/tymuxd")

	cfg := ResolveDaemonConfig()

	if cfg.BinaryPath != "/custom/path/to/tymuxd" {
		t.Errorf("BinaryPath = %q, want %q", cfg.BinaryPath, "/custom/path/to/tymuxd")
	}
}
