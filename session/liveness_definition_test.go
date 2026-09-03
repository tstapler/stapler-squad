package session

import (
	"strings"
	"testing"
	"time"
)

func TestLivenessDefinition_should_ComputeStalenessThreshold_When_KindIsDurationBudget(t *testing.T) {
	d := LivenessDefinition{
		Kind:             LivenessKindDurationBudget,
		ExpectedDuration: 30 * time.Minute,
		StalenessMargin:  5 * time.Minute,
	}

	got := d.StalenessThreshold()

	want := 35 * time.Minute
	if got != want {
		t.Fatalf("StalenessThreshold() = %v, want %v", got, want)
	}
}

func TestLivenessDefinition_should_PanicStalenessThreshold_When_KindIsNotDurationBudget(t *testing.T) {
	tests := []struct {
		name string
		def  LivenessDefinition
	}{
		{
			name: "heartbeat",
			def:  LivenessDefinition{Kind: LivenessKindHeartbeat, MaxNoProgressDuration: 2 * time.Hour},
		},
		{
			name: "cycle frequency",
			def:  LivenessDefinition{Kind: LivenessKindCycleFrequency, CycleThreshold: 3, CycleLookback: 24 * time.Hour},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatalf("StalenessThreshold() did not panic for kind %q", tt.def.Kind)
				}
			}()
			_ = tt.def.StalenessThreshold()
		})
	}
}

func TestNewLivenessDefinition_should_RejectExpectedDuration_When_KindIsHeartbeat(t *testing.T) {
	_, err := NewLivenessDefinition(LivenessKindHeartbeat, WithExpectedDuration(30*time.Minute))

	if err == nil {
		t.Fatal("expected a non-nil error, got nil")
	}
	if !strings.Contains(err.Error(), "ExpectedDuration") {
		t.Fatalf("expected error to name ExpectedDuration as invalid, got: %v", err)
	}
}

func TestNewLivenessDefinition_should_ConstructValidDefinitions_When_FieldsMatchKind(t *testing.T) {
	tests := []struct {
		name string
		kind LivenessKind
		opts []LivenessDefinitionOption
	}{
		{
			name: "duration budget",
			kind: LivenessKindDurationBudget,
			opts: []LivenessDefinitionOption{
				WithExpectedDuration(30 * time.Minute),
				WithStalenessMargin(5 * time.Minute),
			},
		},
		{
			name: "heartbeat",
			kind: LivenessKindHeartbeat,
			opts: []LivenessDefinitionOption{
				WithMaxNoProgressDuration(2 * time.Hour),
			},
		},
		{
			name: "cycle frequency",
			kind: LivenessKindCycleFrequency,
			opts: []LivenessDefinitionOption{
				WithCycleThreshold(3),
				WithCycleLookback(24 * time.Hour),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, err := NewLivenessDefinition(tt.kind, tt.opts...)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if def.Kind != tt.kind {
				t.Fatalf("Kind = %q, want %q", def.Kind, tt.kind)
			}
		})
	}
}

// TestNewLivenessDefinition_should_RejectMismatchedFields_When_KindAndFieldsDisagree pairs each of
// the 3 kinds with each of the other two shapes' fields (Task 1.1.1c: "each of the 3 kinds paired
// with each wrong kind's fields").
func TestNewLivenessDefinition_should_RejectMismatchedFields_When_KindAndFieldsDisagree(t *testing.T) {
	tests := []struct {
		name string
		kind LivenessKind
		opts []LivenessDefinitionOption
	}{
		// LivenessKindDurationBudget (Shape A) with Shape B/C fields.
		{
			name: "duration budget with heartbeat field",
			kind: LivenessKindDurationBudget,
			opts: []LivenessDefinitionOption{WithMaxNoProgressDuration(2 * time.Hour)},
		},
		{
			name: "duration budget with cycle-frequency fields",
			kind: LivenessKindDurationBudget,
			opts: []LivenessDefinitionOption{WithCycleThreshold(3), WithCycleLookback(24 * time.Hour)},
		},
		// LivenessKindHeartbeat (Shape B) with Shape A/C fields.
		{
			name: "heartbeat with duration-budget ExpectedDuration field",
			kind: LivenessKindHeartbeat,
			opts: []LivenessDefinitionOption{WithExpectedDuration(30 * time.Minute)},
		},
		{
			name: "heartbeat with duration-budget StalenessMargin field",
			kind: LivenessKindHeartbeat,
			opts: []LivenessDefinitionOption{WithStalenessMargin(5 * time.Minute)},
		},
		{
			name: "heartbeat with cycle-frequency fields",
			kind: LivenessKindHeartbeat,
			opts: []LivenessDefinitionOption{WithCycleThreshold(3), WithCycleLookback(24 * time.Hour)},
		},
		// LivenessKindCycleFrequency (Shape C) with Shape A/B fields.
		{
			name: "cycle frequency with duration-budget fields",
			kind: LivenessKindCycleFrequency,
			opts: []LivenessDefinitionOption{WithExpectedDuration(30 * time.Minute), WithStalenessMargin(5 * time.Minute)},
		},
		{
			name: "cycle frequency with heartbeat field",
			kind: LivenessKindCycleFrequency,
			opts: []LivenessDefinitionOption{WithMaxNoProgressDuration(2 * time.Hour)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def, err := NewLivenessDefinition(tt.kind, tt.opts...)
			if err == nil {
				t.Fatalf("expected a non-nil error, got nil (def: %+v)", def)
			}
		})
	}
}

func TestNewLivenessDefinition_should_RejectUnknownKind_When_KindIsNotOneOfTheThree(t *testing.T) {
	_, err := NewLivenessDefinition(LivenessKind("bogus"))
	if err == nil {
		t.Fatal("expected a non-nil error, got nil")
	}
}
