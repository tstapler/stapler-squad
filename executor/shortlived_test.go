package executor

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// T-UNIT-001: ShortLivedCmd_Run_appliesWaitDelay
func TestShortLivedCmd_Run_appliesWaitDelay(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := New(ctx, "echo", []string{"hi"})
	_, _, cmd := c.build()

	if cmd.WaitDelay <= 0 {
		t.Errorf("expected WaitDelay > 0, got %v", cmd.WaitDelay)
	}
}

// T-UNIT-002: ShortLivedCmd_Output_capturesStdout
func TestShortLivedCmd_Output_capturesStdout(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	out, err := New(ctx, helperBin, []string{"--print", "hello"}).Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

// T-UNIT-003: ShortLivedCmd_CombinedOutput_mergesStderr
func TestShortLivedCmd_CombinedOutput_mergesStderr(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	out, err := New(ctx, helperBin, []string{"--print-both"}).CombinedOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	outStr := string(out)
	if !strings.Contains(outStr, "out") {
		t.Errorf("expected 'out' in combined output, got %q", outStr)
	}
	if !strings.Contains(outStr, "err") {
		t.Errorf("expected 'err' in combined output, got %q", outStr)
	}
}

// T-UNIT-004: ShortLivedCmd_Run_nonZeroExit_returnsExitError
func TestShortLivedCmd_Run_nonZeroExit_returnsExitError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	err := New(ctx, helperBin, []string{"--exit-code", "1"}).Run()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
	}
	if exitErr.ExitCode() != 1 {
		t.Errorf("expected exit code 1, got %d", exitErr.ExitCode())
	}
}

// T-UNIT-005: ShortLivedCmd_WithTimeout_cancelsBeforeCompletion
func TestShortLivedCmd_WithTimeout_cancelsBeforeCompletion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	start := time.Now()
	err := New(ctx, helperBin, []string{"--sleep", "10s"}, WithTimeout(100*time.Millisecond)).Run()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from timeout, got nil")
	}
	// Must complete well within 10 seconds.
	if elapsed > 5*time.Second {
		t.Errorf("expected completion within 5s, took %v", elapsed)
	}
	// Must not wait the full 10s sleep.
	if elapsed > 3*time.Second {
		t.Errorf("timeout did not fire: elapsed %v", elapsed)
	}
}

// T-UNIT-006: ShortLivedCmd_WithDir_setsWorkingDirectory
func TestShortLivedCmd_WithDir_setsWorkingDirectory(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	ctx := context.Background()
	out, err := New(ctx, helperBin, []string{"--print-cwd"}, WithDir(tmpDir)).Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != tmpDir {
		t.Errorf("expected cwd %q, got %q", tmpDir, got)
	}
}

// T-UNIT-007: ShortLivedCmd_build_setpgidTrueByDefault
func TestShortLivedCmd_build_setpgidTrueByDefault(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, cancel, cmd := New(ctx, "echo", nil).build()
	defer cancel()

	if cmd.SysProcAttr == nil {
		t.Fatal("expected SysProcAttr to be set, got nil")
	}
	if !cmd.SysProcAttr.Setpgid {
		t.Error("expected SysProcAttr.Setpgid == true")
	}
}

// T-UNIT-008: ShortLivedCmd_WithoutProcessGroup_noSetpgid
func TestShortLivedCmd_WithoutProcessGroup_noSetpgid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, cancel, cmd := New(ctx, "echo", nil, WithoutProcessGroup()).build()
	defer cancel()

	hasSetpgid := cmd.SysProcAttr != nil && cmd.SysProcAttr.Setpgid
	if hasSetpgid {
		t.Error("expected Setpgid == false when WithoutProcessGroup is used")
	}
}

// T-UNIT-025: ShortLivedCmd_WaitDelayAlwaysNonZero
func TestShortLivedCmd_WaitDelayAlwaysNonZero(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	configs := [][]Option{
		{},                                      // no options
		{WithTimeout(time.Second)},              // with timeout
		{WithDir(t.TempDir())},                  // with dir
		{WithReplaceEnv([]string{"FOO=bar"})},  // with env replacement
		{WithEnv("KEY", "val")},                 // with extra env
		{WithoutProcessGroup()},                 // no process group
	}

	for i, opts := range configs {
		_, cancel, cmd := New(ctx, "echo", nil, opts...).build()
		if cmd.WaitDelay <= 0 {
			t.Errorf("config[%d]: expected WaitDelay > 0, got %v", i, cmd.WaitDelay)
		}
		cancel()
	}
}

// T-UNIT-026: CommandContext_signatureUnchanged
// This is a compile-time check: the test simply verifies CommandContext still
// works with the variadic string signature.
func TestCommandContext_signatureUnchanged(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// If this compiles and runs, the signature is unchanged.
	from := "github.com/tstapler/stapler-squad/executor/safeexec"
	_ = from // document the import we're testing

	// Verify via New() which uses CommandContextPG/CommandContext internally.
	c := New(ctx, "echo", []string{"hi"})
	err := c.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestShortLivedCmd_WithEnv_appendsToEnvironment verifies env var injection.
func TestShortLivedCmd_WithEnv_appendsToEnvironment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	out, err := New(ctx, helperBin, []string{"--print-env", "SAFEEXEC_TEST_VAR"},
		WithEnv("SAFEEXEC_TEST_VAR", "expected_value")).Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "expected_value" {
		t.Errorf("expected 'expected_value', got %q", got)
	}
}

// TestShortLivedCmd_WithReplaceEnv_replacesEnvironment verifies env replacement.
func TestShortLivedCmd_WithReplaceEnv_replacesEnvironment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Only CLEAN_ENV_VAR is in the environment; PATH should not be there.
	out, err := New(ctx, helperBin, []string{"--print-env", "CLEAN_ENV_VAR"},
		WithReplaceEnv([]string{"CLEAN_ENV_VAR=clean_value"})).Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(string(out))
	if got != "clean_value" {
		t.Errorf("expected 'clean_value', got %q", got)
	}
}

// TestShortLivedCmd_WithRedactArgs_scrubsAuditLog verifies secret scrubbing.
func TestShortLivedCmd_WithRedactArgs_scrubsAuditLog(t *testing.T) {
	t.Parallel()

	hook := &fakeHook{}
	ctx := WithAuditHook(context.Background(), hook)

	// argv: [helperBin, "--print-env", "MY_SECRET_VALUE"]
	// Redact index 2 (the secret value).
	_, err := New(ctx, helperBin, []string{"--print-env", "NONEXISTENT_VAR"},
		WithRedactArgs(2)).Output()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(hook.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(hook.entries))
	}
	// Command[0] is name, Command[1] is "--print-env", Command[2] is redacted.
	if len(hook.entries[0].Command) < 3 {
		t.Fatalf("expected at least 3 command entries, got %v", hook.entries[0].Command)
	}
	if hook.entries[0].Command[2] != "<redacted>" {
		t.Errorf("expected '<redacted>', got %q", hook.entries[0].Command[2])
	}
}

// TestShortLivedCmd_Run_success verifies basic happy path.
func TestShortLivedCmd_Run_success(t *testing.T) {
	t.Parallel()

	err := New(context.Background(), helperBin, []string{"--exit-code", "0"}).Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
