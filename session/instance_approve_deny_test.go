package session

import (
	"errors"
	"testing"
)

func TestApprove_FromActive_Fails(t *testing.T) {
	t.Parallel()
	// Active→Active is a self-transition and is not allowed.
	inst := &Instance{
		Title:  "test-approve-active",
		Status: Active,
	}
	inst.started.Store(true)

	err := inst.Approve()
	if err == nil {
		t.Fatal("Approve from Active should return error (Active->Active is not allowed)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
	if transErr.From != Active || transErr.To != Active {
		t.Errorf("expected transition Active->Active, got %s->%s", transErr.From, transErr.To)
	}
}

func TestApprove_FromPaused(t *testing.T) {
	t.Parallel()
	// Paused→Active is valid.
	inst := &Instance{
		Title:  "test-approve-paused",
		Status: Paused,
	}
	inst.started.Store(true)

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Paused should succeed (Paused->Active is valid), got: %v", err)
	}

	if inst.Status != Active {
		t.Errorf("expected status Active after Approve from Paused, got %s", inst.Status)
	}
}

func TestDeny_FromActive(t *testing.T) {
	t.Parallel()
	// Active→Paused is valid, so Deny from Active should succeed.
	inst := &Instance{
		Title:  "test-deny-active",
		Status: Active,
	}
	inst.started.Store(true)

	err := inst.Deny()
	if err != nil {
		t.Fatalf("Deny from Active should succeed (Active->Paused is valid), got: %v", err)
	}

	if inst.Status != Paused {
		t.Errorf("expected status Paused after Deny from Active, got %s", inst.Status)
	}
}

func TestDeny_FromPaused_Fails(t *testing.T) {
	t.Parallel()
	// Paused→Paused is NOT a valid transition (self-transition not allowed).
	inst := &Instance{
		Title:  "test-deny-paused",
		Status: Paused,
	}
	inst.started.Store(true)

	err := inst.Deny()
	if err == nil {
		t.Fatal("Deny from Paused should return error (Paused->Paused is not allowed)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
}

func TestApprove_FromStopped_Succeeds(t *testing.T) {
	t.Parallel()
	// Stopped→Active is valid for session revival.
	inst := &Instance{
		Title:  "test-approve-stopped",
		Status: Stopped,
	}
	inst.started.Store(true)

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Stopped should succeed (Stopped->Active is valid for revival), got: %v", err)
	}
	if inst.Status != Active {
		t.Errorf("expected status Active after Approve from Stopped, got %s", inst.Status)
	}
}

func TestDeny_FromStopped_Fails(t *testing.T) {
	t.Parallel()
	// Stopped→Paused is not a valid transition.
	inst := &Instance{
		Title:  "test-deny-stopped",
		Status: Stopped,
	}
	inst.started.Store(true)

	err := inst.Deny()
	if err == nil {
		t.Fatal("Deny from Stopped should return error (Stopped->Paused is not allowed)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
}

func TestApprove_FromHibernated_Succeeds(t *testing.T) {
	t.Parallel()
	// Hibernated→Active is valid (resume from hibernation).
	inst := &Instance{
		Title:  "test-approve-hibernated",
		Status: Hibernated,
	}
	inst.started.Store(true)
	// transitionToLocked dispatches resumeFromHibernationLocked in a background
	// goroutine that calls Start()/StartController()/StartSessionDriver — real,
	// long-lived work this test isn't exercising or able to await. Pre-set
	// driverRunning so StartSessionDriver's CAS guard (see
	// TestStartSessionDriver_Idempotent) makes it a no-op, preventing a leaked
	// ticker-loop goroutine from outliving this test (caught by goleak in
	// TestActorNoLeak/TestActorStopIdempotent under CI's slower scheduling).
	inst.driverRunning.Store(true)

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Hibernated should succeed (Hibernated->Active is valid), got: %v", err)
	}
	// Read via Snapshot(), not the bare field: transitionToLocked's synchronous
	// write and resumeFromHibernationLocked's async write-back-on-failure both
	// publish through i.snapshot (an atomic.Pointer), so this is the only read
	// that's ordered against both without racing under -race. See the doc
	// comment on (*Instance).GetStatus in instance_state.go.
	if status := inst.Snapshot().Status; status != Active {
		t.Errorf("expected status Active after Approve from Hibernated, got %s", status)
	}
}

func TestApprove_ErrorMessageFormat(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "test-err-format",
		Status: Active,
	}
	inst.started.Store(true)

	err := inst.Approve()
	if err == nil {
		t.Fatal("expected error")
	}

	// The Approve method wraps with "approve: " prefix; Active is the string name of Active
	expected := "approve: invalid transition: Active -> Active"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestDeny_ErrorMessageFormat(t *testing.T) {
	t.Parallel()
	inst := &Instance{
		Title:  "test-err-format",
		Status: Stopped,
	}
	inst.started.Store(true)

	err := inst.Deny()
	if err == nil {
		t.Fatal("expected error")
	}

	// The Deny method wraps with "deny: " prefix
	expected := "deny: invalid transition: Stopped -> Paused"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestApprove_AllSourceStatuses(t *testing.T) {
	t.Parallel()
	// Table-driven: test Approve() (transition to Active) from every status in the 5-state model.
	tests := []struct {
		name       string
		from       Status
		expectPass bool
	}{
		{"Creating->Active", Creating, true},
		{"Active->Active", Active, false}, // self-transition not allowed
		{"Paused->Active", Paused, true},
		{"Stopped->Active", Stopped, true}, // recoverable — reconciler can revive stopped sessions
		{"Hibernated->Active", Hibernated, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst := &Instance{
				Title:  "test",
				Status: tt.from,
			}
			inst.started.Store(true)
			// See TestApprove_FromHibernated_Succeeds: suppresses the real
			// StartSessionDriver goroutine that Hibernated->Active's resume
			// path would otherwise leak past this test's lifetime.
			inst.driverRunning.Store(true)
			err := inst.Approve()
			if tt.expectPass && err != nil {
				t.Errorf("expected Approve to succeed from %s, got: %v", tt.from, err)
			}
			if !tt.expectPass && err == nil {
				t.Errorf("expected Approve to fail from %s, but it succeeded", tt.from)
			}
			// Read via Snapshot(), not the bare field: the Hibernated->Active case
			// dispatches resumeFromHibernationLocked in a background goroutine that
			// can write Status back to Hibernated (on Start() failure) after Approve()
			// has already returned. Both that write-back and transitionToLocked's own
			// synchronous write publish through i.snapshot (an atomic.Pointer), so
			// this is the only read ordered against both without racing under -race.
			// See the doc comment on (*Instance).GetStatus in instance_state.go.
			if status := inst.Snapshot().Status; tt.expectPass && status != Active {
				t.Errorf("expected status Active after Approve from %s, got %s", tt.from, status)
			}
		})
	}
}

func TestDeny_AllSourceStatuses(t *testing.T) {
	t.Parallel()
	// Table-driven: test Deny() (transition to Paused) from every status in the 5-state model.
	tests := []struct {
		name       string
		from       Status
		expectPass bool
	}{
		{"Creating->Paused", Creating, false}, // not in allowed transitions
		{"Active->Paused", Active, true},
		{"Paused->Paused", Paused, false},         // self-transition
		{"Stopped->Paused", Stopped, false},       // not allowed (only Stopped→Active is valid)
		{"Hibernated->Paused", Hibernated, false}, // not allowed (only Hibernated→Active/Stopped)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			inst := &Instance{
				Title:  "test",
				Status: tt.from,
			}
			inst.started.Store(true)
			err := inst.Deny()
			if tt.expectPass && err != nil {
				t.Errorf("expected Deny to succeed from %s, got: %v", tt.from, err)
			}
			if !tt.expectPass && err == nil {
				t.Errorf("expected Deny to fail from %s, but it succeeded", tt.from)
			}
			if tt.expectPass && inst.Status != Paused {
				t.Errorf("expected status Paused after Deny from %s, got %s", tt.from, inst.Status)
			}
		})
	}
}
