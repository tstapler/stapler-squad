package gogitstore

import (
	"bytes"
	"testing"
)

// isExpectedFaultSignal reports whether output (the combined stdout+stderr of
// a crashed Go subprocess) contains the Go runtime's own crash-report line
// confirming the process died from SIGBUS or SIGSEGV — the two signals a
// truncated-mmap-read fault can raise.
//
// This string-matches the subprocess's printed diagnostics rather than
// inspecting exec.ExitError/syscall.WaitStatus, because Go's runtime installs
// its own signal handler for these hardware faults and never lets the OS
// report the death as a raw signal exit — WaitStatus.Signal() can never equal
// SIGBUS/SIGSEGV, across every GOTRACEBACK mode. See
// project_plans/fix-flaky-headless-tests/implementation/plan.md's Story
// 3.1.1 for the full investigation.
func isExpectedFaultSignal(output []byte) (matched bool, detail string) {
	switch {
	case bytes.Contains(output, []byte("[signal SIGBUS:")):
		return true, "bus error"
	case bytes.Contains(output, []byte("[signal SIGSEGV:")):
		return true, "segmentation violation"
	default:
		return false, `no Go runtime fatal-fault signature ("[signal SIGBUS:"/"[signal SIGSEGV:") found in subprocess output`
	}
}

func TestIsExpectedFaultSignal_should_ReturnTrueAndBusError_When_OutputContainsGoRuntimeSIGBUSCrashLine(t *testing.T) {
	// Fixture is a byte-for-byte excerpt of this package's own genuine crash
	// dump, captured by running GOGITSTORE_TRUNC_HELPER=1 directly (see this
	// function's doc comment) — not a hand-guessed format.
	output := []byte("unexpected fault address 0x7f7efb9e6000\nfatal error: fault\n[signal SIGBUS: bus error code=0x2 addr=0x7f7efb9e6000 pc=0x74e714]\n\ngoroutine 21 ...")
	matched, detail := isExpectedFaultSignal(output)
	if !matched || detail != "bus error" {
		t.Fatalf("isExpectedFaultSignal(sigbus dump) = (%v, %q), want (true, \"bus error\")", matched, detail)
	}
}

func TestIsExpectedFaultSignal_should_ReturnTrueAndSegfault_When_OutputContainsGoRuntimeSIGSEGVCrashLine(t *testing.T) {
	output := []byte("unexpected fault address 0xdeadbeef\nfatal error: fault\n[signal SIGSEGV: segmentation violation code=0x1 addr=0xdeadbeef pc=0x12345]\n")
	matched, detail := isExpectedFaultSignal(output)
	if !matched || detail != "segmentation violation" {
		t.Fatalf("isExpectedFaultSignal(sigsegv dump) = (%v, %q), want (true, \"segmentation violation\")", matched, detail)
	}
}

func TestIsExpectedFaultSignal_should_ReturnFalse_When_OutputHasNoFaultSignature(t *testing.T) {
	matched, detail := isExpectedFaultSignal([]byte("some unrelated output, exit status 1"))
	if matched {
		t.Fatalf("isExpectedFaultSignal(unrelated output) = (%v, %q), want matched=false", matched, detail)
	}
}
