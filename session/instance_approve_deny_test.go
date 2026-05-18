package session

import (
	"errors"
	"testing"
)

func TestApprove_FromActive_Fails(t *testing.T) {
	// Active→Active is a self-transition and is not allowed.
	inst := &Instance{
		Title:   "test-approve-active",
		Status:  Active,
		started: true,
	}

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
	// Paused→Active is valid.
	inst := &Instance{
		Title:   "test-approve-paused",
		Status:  Paused,
		started: true,
	}

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Paused should succeed (Paused->Active is valid), got: %v", err)
	}

	if inst.Status != Active {
		t.Errorf("expected status Active after Approve from Paused, got %s", inst.Status)
	}
}

func TestDeny_FromActive(t *testing.T) {
	// Active→Paused is valid, so Deny from Active should succeed.
	inst := &Instance{
		Title:   "test-deny-active",
		Status:  Active,
		started: true,
	}

	err := inst.Deny()
	if err != nil {
		t.Fatalf("Deny from Active should succeed (Active->Paused is valid), got: %v", err)
	}

	if inst.Status != Paused {
		t.Errorf("expected status Paused after Deny from Active, got %s", inst.Status)
	}
}

func TestDeny_FromPaused_Fails(t *testing.T) {
	// Paused→Paused is NOT a valid transition (self-transition not allowed).
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

func TestApprove_FromStopped_Succeeds(t *testing.T) {
	// Stopped→Active is valid for session revival.
	inst := &Instance{
		Title:   "test-approve-stopped",
		Status:  Stopped,
		started: true,
	}

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Stopped should succeed (Stopped->Active is valid for revival), got: %v", err)
	}
	if inst.Status != Active {
		t.Errorf("expected status Active after Approve from Stopped, got %s", inst.Status)
	}
}

func TestDeny_FromStopped_Fails(t *testing.T) {
	// Stopped→Paused is not a valid transition.
	inst := &Instance{
		Title:   "test-deny-stopped",
		Status:  Stopped,
		started: true,
	}

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
	// Hibernated→Active is valid (resume from hibernation).
	inst := &Instance{
		Title:   "test-approve-hibernated",
		Status:  Hibernated,
		started: true,
	}

	err := inst.Approve()
	if err != nil {
		t.Fatalf("Approve from Hibernated should succeed (Hibernated->Active is valid), got: %v", err)
	}
	if inst.Status != Active {
		t.Errorf("expected status Active after Approve from Hibernated, got %s", inst.Status)
	}
}

func TestApprove_ErrorMessageFormat(t *testing.T) {
	inst := &Instance{
		Title:   "test-err-format",
		Status:  Active,
		started: true,
	}

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
	// Table-driven: test Approve() (transition to Active) from every status in the 5-state model.
	tests := []struct {
		name       string
		from       Status
		expectPass bool
	}{
		{"Creating->Active", Creating, true},
		{"Active->Active", Active, false},   // self-transition not allowed
		{"Paused->Active", Paused, true},
		{"Stopped->Active", Stopped, true},  // recoverable — reconciler can revive stopped sessions
		{"Hibernated->Active", Hibernated, true},
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
			if tt.expectPass && inst.Status != Active {
				t.Errorf("expected status Active after Approve from %s, got %s", tt.from, inst.Status)
			}
		})
	}
}

func TestDeny_AllSourceStatuses(t *testing.T) {
	// Table-driven: test Deny() (transition to Paused) from every status in the 5-state model.
	tests := []struct {
		name       string
		from       Status
		expectPass bool
	}{
		{"Creating->Paused", Creating, false},    // not in allowed transitions
		{"Active->Paused", Active, true},
		{"Paused->Paused", Paused, false},         // self-transition
		{"Stopped->Paused", Stopped, false},       // not allowed (only Stopped→Active is valid)
		{"Hibernated->Paused", Hibernated, false}, // not allowed (only Hibernated→Active/Stopped)
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
