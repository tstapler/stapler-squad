package tymux

import (
	"testing"
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
	// compatibility carve-out, .claude/docs/state-isolation.md) — it must
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
	t.Setenv("TYMUXD_ADDR", "http://127.0.0.1:9999")

	cfg := ResolveDaemonConfig()

	if cfg.Addr != "http://127.0.0.1:9999" {
		t.Errorf("Addr = %q, want TYMUXD_ADDR override %q", cfg.Addr, "http://127.0.0.1:9999")
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
