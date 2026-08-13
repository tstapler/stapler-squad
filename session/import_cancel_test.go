package session

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// spawnSleeper starts a real short-lived child process so
// SuspendOriginalProcess/ResumeOriginalProcess (thin syscall.Kill wrappers
// with no injectable seam) have a real PID to signal. The process is killed
// on test cleanup regardless of whether the test suspended/resumed it.
func spawnSleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := safeexec.CommandContext(context.Background(), "sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to spawn sleeper process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

func TestCancelPendingKill_DeletesInstanceBeforeResumingProcess_When_DeleteSucceeds(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	if err != nil {
		t.Fatalf("failed to create suspended process store: %v", err)
	}

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	if err := suspended.Add(SuspendedProcessRecord{PID: pid, InstanceID: "imported-1"}); err != nil {
		t.Fatalf("failed to seed suspended process record: %v", err)
	}
	if err := SuspendOriginalProcess(pid); err != nil {
		t.Fatalf("failed to suspend original process: %v", err)
	}

	store := &fakeInstanceStore{}

	resumed, err := CancelPendingKill(CancelPendingKillParams{
		Storage:     store,
		Suspended:   suspended,
		InstanceID:  "imported-1",
		OriginalPID: pid,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !resumed {
		t.Fatal("expected resumed=true when delete succeeds")
	}
	if store.deletedTitle != "imported-1" {
		t.Fatalf("expected DeleteInstance called with %q, got %q", "imported-1", store.deletedTitle)
	}

	if _, found, err := suspended.Get("imported-1"); err != nil {
		t.Fatalf("failed to read suspended process store: %v", err)
	} else if found {
		t.Fatal("expected suspended-process record to be removed after cancel")
	}
}

func TestCancelPendingKill_SkipsResume_When_DeleteFails(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	if err != nil {
		t.Fatalf("failed to create suspended process store: %v", err)
	}

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	if err := suspended.Add(SuspendedProcessRecord{PID: pid, InstanceID: "imported-2"}); err != nil {
		t.Fatalf("failed to seed suspended process record: %v", err)
	}
	if err := SuspendOriginalProcess(pid); err != nil {
		t.Fatalf("failed to suspend original process: %v", err)
	}

	deleteErr := errors.New("boom: delete failed")
	store := &fakeInstanceStore{deleteErr: deleteErr}

	resumed, err := CancelPendingKill(CancelPendingKillParams{
		Storage:     store,
		Suspended:   suspended,
		InstanceID:  "imported-2",
		OriginalPID: pid,
	})
	if !errors.Is(err, deleteErr) {
		t.Fatalf("expected wrapped deleteErr, got %v", err)
	}
	if resumed {
		t.Fatal("expected resumed=false when delete fails")
	}

	// The suspended-process record must still be present -- resume (and the
	// record removal that would follow it) was correctly skipped.
	if _, found, err := suspended.Get("imported-2"); err != nil {
		t.Fatalf("failed to read suspended process store: %v", err)
	} else if !found {
		t.Fatal("expected suspended-process record to remain when delete fails")
	}

	// Clean up: resume the still-suspended process ourselves so t.Cleanup's
	// Kill/Wait doesn't hang on a stopped process.
	_ = ResumeOriginalProcess(pid)
}
