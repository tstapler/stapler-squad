package tymux

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"

	"github.com/tstapler/stapler-squad/config"
)

// DaemonConfig bundles the two string concepts every tymuxd supervision
// function needs: where to reach the daemon (Addr) and which binary to spawn
// it from (BinaryPath). Per the `primitive-obsession-checklist` skill,
// this exists so no later supervision function signature grows a second bare
// string parameter that could be silently swapped with the first — every such
// function should take a DaemonConfig, not separate addr/binaryPath strings.
type DaemonConfig struct {
	Addr       string
	BinaryPath string
	// SocketPath, when non-empty, is passed to the spawned tymuxd as
	// TYMUXD_SOCKET_PATH (see resolveSocketPath's doc comment for why this is
	// necessary in addition to Addr's instance-scoped port). Empty means "let
	// tymuxd pick its own default" — unchanged from today's behavior.
	SocketPath string
}

// instanceDaemonPortBase and instanceDaemonPortSpan derive a per-instance
// default tymuxd port so that two STAPLER_SQUAD_INSTANCEs never collide on
// defaultTymuxdAddr's port (7419) when both spawn their own tymuxd. Mirrors
// the CRC32-based derivation CLAUDE.md's "Manual dev port block" already uses
// for stapler-squad's own HTTP port (base 62871 = 61000 +
// CRC32("stapler-squad") % 4525).
const (
	instanceDaemonPortBase = 7420
	instanceDaemonPortSpan = 1000
)

// ResolveDaemonConfig is the single choke point for producing a DaemonConfig,
// called both from main.go (startup) and from session.TymuxBackend wiring.
// It must be exported for that cross-package use.
//
// Addr resolution mirrors tymuxdAddr()'s existing TYMUXD_ADDR-or-default
// precedence (transport.go), extended with an instance-scoped default so a
// named STAPLER_SQUAD_INSTANCE (e.g. a manual/isolated dev instance) doesn't
// collide with the default instance's tymuxd on 127.0.0.1:7419:
//
//   - TYMUXD_ADDR set: always wins, regardless of STAPLER_SQUAD_INSTANCE.
//   - STAPLER_SQUAD_INSTANCE unset, "", or "shared" (the default/live
//     instance — matches config.IsNamedInstance()'s inverse condition and
//     GetConfigDirForDir's "shared" backward-compatibility carve-out):
//     defaultTymuxdAddr, unchanged from today.
//   - STAPLER_SQUAD_INSTANCE set to anything else: a distinct port derived
//     deterministically from the instance name, so the same instance name
//     always resolves to the same port and different instance names resolve
//     to different (with overwhelming probability) ports.
//
// BinaryPath is always TymuxdBinary() (Epic 1.2), which already applies its
// own TYMUXD_BIN override independently of Addr resolution.
func ResolveDaemonConfig() DaemonConfig {
	return DaemonConfig{
		Addr:       resolveDaemonAddr(),
		BinaryPath: TymuxdBinary(),
		SocketPath: resolveSocketPath(),
	}
}

func resolveDaemonAddr() string {
	if v := os.Getenv("TYMUXD_ADDR"); v != "" {
		return v
	}
	instanceID := os.Getenv("STAPLER_SQUAD_INSTANCE")
	if instanceID == "" || instanceID == "shared" {
		return defaultTymuxdAddr
	}
	port := instanceDaemonPortBase + crc32.ChecksumIEEE([]byte(instanceID))%instanceDaemonPortSpan
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

// resolveSocketPath derives a per-instance path for tymuxd's control Unix
// socket, mirroring resolveDaemonAddr's instance-scoping. tymuxd's
// Unix-socket lock file is NOT derived from --socket-addr/TYMUXD_ADDR: it
// defaults to one fixed path per OS user under $XDG_RUNTIME_DIR (or
// $TMPDIR), so two tymuxd processes with different TYMUXD_ADDR values (i.e.
// for two different STAPLER_SQUAD_INSTANCEs) still collide — the second
// refuses to start with "another tymuxd is already starting against
// <path>" (confirmed empirically; overridable via
// --socket-path/TYMUXD_SOCKET_PATH per `tymuxd --help`).
//
// Not placed inside the per-instance config dir (config.GetConfigDir()):
// Unix domain socket paths are kernel-length-limited (sun_path, ~108 bytes
// on Linux) — confirmed empirically against the real binary ("path must be
// shorter than SUN_LEN") once configDir is a few directories deep (a long
// STAPLER_SQUAD_INSTANCE name, or -- the case that actually surfaced this --
// a deeply-nested STAPLER_SQUAD_TEST_DIR under a test's own tmp dir). Hashing
// configDir into a short, fixed-length token and placing the socket under
// os.TempDir() instead keeps every resolved path well under that limit
// regardless of how long or deep configDir itself is, while staying
// deterministic (same configDir always hashes to the same path, required for
// EnsureDaemonRunning's reuse case within one instance) and still
// per-instance-unique (different configDirs hash to different paths, with
// overwhelming probability). The directory is still created at mode 0700 --
// tymuxd refuses to bind a socket whose parent directory isn't mode 0700 and
// owned by the invoking user (confirmed empirically — "expected uid ... at
// mode 700") — so even though os.TempDir() (e.g. /tmp) is itself
// world-writable, this specific subdirectory is not: an attacker who
// pre-creates it with different ownership causes MkdirAll/Chmod to fail here
// (falling back to tymuxd's own default, never a compromised directory), and
// tymuxd independently re-checks ownership/mode itself as defense in depth.
//
// Returns "" (tymuxd's own default, unchanged) for the unset/"shared"
// instance and on any resolution error, mirroring resolveDaemonAddr's
// "unset/shared means unchanged default".
func resolveSocketPath() string {
	if v := os.Getenv("TYMUXD_SOCKET_PATH"); v != "" {
		return v
	}
	instanceID := os.Getenv("STAPLER_SQUAD_INSTANCE")
	if instanceID == "" || instanceID == "shared" {
		return ""
	}
	configDir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}
	hash := sha256.Sum256([]byte(configDir))
	sockDir := filepath.Join(os.TempDir(), "ssq-tymux-"+hex.EncodeToString(hash[:8]))
	if err := os.MkdirAll(sockDir, 0700); err != nil {
		return ""
	}
	// A pre-existing sockDir (e.g. created before this mode requirement
	// existed) needs its mode corrected too -- MkdirAll is a no-op on an
	// already-existing directory and doesn't fix its permissions.
	if err := os.Chmod(sockDir, 0700); err != nil {
		return ""
	}
	return filepath.Join(sockDir, "tymuxd.sock")
}
