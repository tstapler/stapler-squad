package executor

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// T-UNIT-019: RlimitConfig_zerovalueIsValid
func TestRlimitConfig_zerovalueIsValid(t *testing.T) {
	t.Parallel()

	cfg := RlimitConfig{} // zero value
	cmd := safeexec.CommandContext(context.Background(), "echo", "hi")
	err := applyRlimits(cmd, cfg)
	if err != nil {
		t.Errorf("applyRlimits with zero RlimitConfig returned error: %v", err)
	}
}

// T-UNIT-020: RlimitConfig_nonLinuxStub_returnsNil
// On non-Linux, applyRlimits is a no-op stub that returns nil.
// On Linux, it should also not error for modest limits.
func TestRlimitConfig_applyRlimits_returnsNil(t *testing.T) {
	t.Parallel()

	cfg := RlimitConfig{MaxOpenFiles: 128}
	cmd := safeexec.CommandContext(context.Background(), "echo", "hi")
	err := applyRlimits(cmd, cfg)
	if err != nil {
		t.Errorf("applyRlimits returned error: %v", err)
	}
}

// TestRlimitConfig_allFields tests all three rlimit types together.
func TestRlimitConfig_allFields(t *testing.T) {
	t.Parallel()

	cfg := RlimitConfig{
		MaxCPUSecs:   60,
		MaxVirtBytes: 1024 * 1024 * 1024, // 1 GB
		MaxOpenFiles: 256,
	}
	cmd := safeexec.CommandContext(context.Background(), "echo", "hi")
	err := applyRlimits(cmd, cfg)
	if err != nil {
		t.Errorf("applyRlimits with all fields set returned error: %v", err)
	}
}
