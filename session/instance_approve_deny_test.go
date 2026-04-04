package session

import (
	"errors"
	"testing"
)

func TestApprove_FromNeedsApproval(t *testing.T) {
	inst := &Instance{
		Title:   "test-approve",
		Status:  NeedsApproval,
		started: true,
	}

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from NeedsApproval should succeed, got: %v", err)
	}

	if inst.Status != Running {
		t.Errorf("expected status Running after Approve, got %s", inst.Status)
	}
}

func TestDeny_FromNeedsApproval(t *testing.T) {
	inst := &Instance{
		Title:   "test-deny",
		Status:  NeedsApproval,
		started: true,
	}

	err := inst.Deny()
	if err != nil {
		t.Fatalf("Deny from NeedsApproval should succeed, got: %v", err)
	}

	if inst.Status != Paused {
		t.Errorf("expected status Paused after Deny, got %s", inst.Status)
	}
}

func TestApprove_FromRunning_Fails(t *testing.T) {
	inst := &Instance{
		Title:   "test-approve-running",
		Status:  Running,
		started: true,
	}

	err := inst.Approve()
	if err == nil {
		t.Fatal("Approve from Running should return error (Running->Running is not allowed)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
	if transErr.From != Running || transErr.To != Running {
		t.Errorf("expected transition Running->Running, got %s->%s", transErr.From, transErr.To)
	}
}

func TestDeny_FromRunning(t *testing.T) {
	// Running->Paused is a valid transition in the state machine,
	// so Deny from Running should succeed.
	inst := &Instance{
		Title:   "test-deny-running",
		Status:  Running,
		started: true,
	}

	err := inst.Deny()
	if err != nil {
		t.Fatalf("Deny from Running should succeed (Running->Paused is valid), got: %v", err)
	}

	if inst.Status != Paused {
		t.Errorf("expected status Paused after Deny from Running, got %s", inst.Status)
	}
}

func TestApprove_FromPaused(t *testing.T) {
	// Paused->Running is valid, so Approve from Paused should succeed
	inst := &Instance{
		Title:   "test-approve-paused",
		Status:  Paused,
		started: true,
	}

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Paused should succeed (Paused->Running is valid), got: %v", err)
	}

	if inst.Status != Running {
		t.Errorf("expected status Running after Approve from Paused, got %s", inst.Status)
	}
}

func TestDeny_FromPaused_Fails(t *testing.T) {
	// Paused->Paused is NOT a valid transition (self-transition not allowed)
	inst := &Instance{
		Title:   "test-deny-paused",
		Status:  Paused,
		started: true,
	}

	err := inst.Deny()
	if err == nil {
		t.Fatal("Deny from Paused should return error (Paused->Paused is not allowed)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
}

func TestApprove_FromStopped_Fails(t *testing.T) {
	// Stopped is terminal -- no outgoing transitions
	inst := &Instance{
		Title:   "test-approve-stopped",
		Status:  Stopped,
		started: true,
	}

	err := inst.Approve()
	if err == nil {
		t.Fatal("Approve from Stopped should return error (Stopped is terminal)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
}

func TestDeny_FromStopped_Fails(t *testing.T) {
	// Stopped is terminal -- no outgoing transitions
	inst := &Instance{
		Title:   "test-deny-stopped",
		Status:  Stopped,
		started: true,
	}

	err := inst.Deny()
	if err == nil {
		t.Fatal("Deny from Stopped should return error (Stopped is terminal)")
	}

	var transErr ErrInvalidTransition
	if !errors.As(err, &transErr) {
		t.Fatalf("expected ErrInvalidTransition, got %T: %v", err, err)
	}
}

func TestApprove_ErrorMessageFormat(t *testing.T) {
	inst := &Instance{
		Title:   "test-err-format",
		Status:  Running,
		started: true,
	}

	err := inst.Approve()
	if err == nil {
		t.Fatal("expected error")
	}

	// The Approve method wraps with "approve: " prefix
	expected := "approve: invalid transition: Running -> Running"
	if err.Error() != expected {
		t.Errorf("expected error %q, got %q", expected, err.Error())
	}
}

func TestDeny_ErrorMessageFormat(t *testing.T) {
	inst := &Instance{
		Title:   "test-err-format",
		Status:  Stopped,
		started: true,
	}

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
	// Table-driven: test Approve() (transition to Running) from every status
	tests := []struct {
		name       string
		from       Status
		expectPass bool
	}{
		{"Creating->Running", Creating, true},
		{"Ready->Running", Ready, true},
		{"Running->Running", Running, false}, // self-transition
		{"Paused->Running", Paused, true},
		{"NeedsApproval->Running", NeedsApproval, true},
		{"Loading->Running", Loading, true},
		{"Stopped->Running", Stopped, false}, // terminal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instance{
				Title:   "test",
				Status:  tt.from,
				started: true,
			}
			err := inst.Approve()
			if tt.expectPass && err != nil {
				t.Errorf("expected Approve to succeed from %s, got: %v", tt.from, err)
			}
			if !tt.expectPass && err == nil {
				t.Errorf("expected Approve to fail from %s, but it succeeded", tt.from)
			}
			if tt.expectPass && inst.Status != Running {
				t.Errorf("expected status Running after Approve from %s, got %s", tt.from, inst.Status)
			}
		})
	}
}

func TestDeny_AllSourceStatuses(t *testing.T) {
	// Table-driven: test Deny() (transition to Paused) from every status
	tests := []struct {
		name       string
		from       Status
		expectPass bool
	}{
		{"Creating->Paused", Creating, false}, // not in allowed transitions
		{"Ready->Paused", Ready, false},       // not in allowed transitions
		{"Running->Paused", Running, true},
		{"Paused->Paused", Paused, false}, // self-transition
		{"NeedsApproval->Paused", NeedsApproval, true},
		{"Loading->Paused", Loading, false}, // not in allowed transitions
		{"Stopped->Paused", Stopped, false}, // terminal
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inst := &Instance{
				Title:   "test",
				Status:  tt.from,
				started: true,
			}
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
