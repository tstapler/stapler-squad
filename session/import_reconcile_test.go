package session

import (
	"context"
	"testing"
)

func TestReconcileSuspendedProcesses_ResumesProcessAndRemovesRecord_When_ProcessStillExists(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	if err != nil {
		t.Fatalf("failed to create suspended process store: %v", err)
	}

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	if err := suspended.Add(SuspendedProcessRecord{PID: pid, InstanceID: "imported-reconcile-1"}); err != nil {
		t.Fatalf("failed to seed suspended process record: %v", err)
	}
	if err := SuspendOriginalProcess(pid); err != nil {
		t.Fatalf("failed to suspend original process: %v", err)
	}

	if err := ReconcileSuspendedProcesses(context.Background(), suspended, nil); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, found, err := suspended.Get("imported-reconcile-1"); err != nil {
		t.Fatalf("failed to read suspended process store: %v", err)
	} else if found {
		t.Fatal("expected suspended-process record to be removed after successful reconcile")
	}
}

func TestReconcileSuspendedProcesses_ReturnsErrorButContinues_When_OneResumeFails(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	if err != nil {
		t.Fatalf("failed to create suspended process store: %v", err)
	}

	// A PID that cannot possibly exist -- ResumeOriginalProcess must fail
	// with ESRCH for this record while still processing the other one.
	const nonexistentPID = int32(1<<31 - 1)
	if err := suspended.Add(SuspendedProcessRecord{PID: nonexistentPID, InstanceID: "imported-reconcile-missing"}); err != nil {
		t.Fatalf("failed to seed suspended process record: %v", err)
	}

	cmd := spawnSleeper(t)
	realPID := int32(cmd.Process.Pid)
	if err := suspended.Add(SuspendedProcessRecord{PID: realPID, InstanceID: "imported-reconcile-real"}); err != nil {
		t.Fatalf("failed to seed suspended process record: %v", err)
	}
	if err := SuspendOriginalProcess(realPID); err != nil {
		t.Fatalf("failed to suspend original process: %v", err)
	}

	err = ReconcileSuspendedProcesses(context.Background(), suspended, nil)
	if err == nil {
		t.Fatal("expected an error from the failed resume, got nil")
	}

	if _, found, ferr := suspended.Get("imported-reconcile-missing"); ferr != nil {
		t.Fatalf("failed to read suspended process store: %v", ferr)
	} else if !found {
		t.Fatal("expected the failed record to remain for a future reconcile pass")
	}

	if _, found, ferr := suspended.Get("imported-reconcile-real"); ferr != nil {
		t.Fatalf("failed to read suspended process store: %v", ferr)
	} else if found {
		t.Fatal("expected the successfully-resumed record to be removed despite the other record's failure")
	}
}

func TestReconcileSuspendedProcesses_ReturnsNil_When_StoreIsNil(t *testing.T) {
	if err := ReconcileSuspendedProcesses(context.Background(), nil, nil); err != nil {
		t.Fatalf("expected nil error for nil store, got %v", err)
	}
}

// TestReconcileSuspendedProcesses_LeavesProcessSuspended_When_CommittedInstanceStillManaged
// guards against unconditionally SIGCONT-ing every suspended process on
// restart: when the committed Instance for a record is still present in
// storage, resuming here would race with whatever legitimately manages that
// Instance's lifecycle (confirm-kill/cancel), risking a dual-writer scenario.
// The record must be left untouched instead.
func TestReconcileSuspendedProcesses_LeavesProcessSuspended_When_CommittedInstanceStillManaged(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	if err != nil {
		t.Fatalf("failed to create suspended process store: %v", err)
	}

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	const instanceID = "imported-reconcile-still-managed"
	if err := suspended.Add(SuspendedProcessRecord{PID: pid, InstanceID: instanceID}); err != nil {
		t.Fatalf("failed to seed suspended process record: %v", err)
	}
	if err := SuspendOriginalProcess(pid); err != nil {
		t.Fatalf("failed to suspend original process: %v", err)
	}
	t.Cleanup(func() { _ = ResumeOriginalProcess(pid) })

	store := &fakeInstanceStore{
		instances: []InstanceData{{Title: instanceID}},
	}

	if err := ReconcileSuspendedProcesses(context.Background(), suspended, store); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, found, err := suspended.Get(instanceID); err != nil {
		t.Fatalf("failed to read suspended process store: %v", err)
	} else if !found {
		t.Fatal("expected the record for a still-managed Instance to remain, not be resumed/removed")
	}
}
