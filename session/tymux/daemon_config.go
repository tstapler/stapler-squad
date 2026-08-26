package tymux

import (
	"fmt"
	"hash/crc32"
	"os"
)

// DaemonConfig bundles the two string concepts every tymuxd supervision
// function needs: where to reach the daemon (Addr) and which binary to spawn
// it from (BinaryPath). Per .claude/rules/primitive-obsession-checklist.md,
// this exists so no later supervision function signature grows a second bare
// string parameter that could be silently swapped with the first — every such
// function should take a DaemonConfig, not separate addr/binaryPath strings.
type DaemonConfig struct {
	Addr       string
	BinaryPath string
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
