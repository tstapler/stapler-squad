package session

import (
	"fmt"
	"time"
)

// LivenessKind discriminates the tagged union of shapes a LivenessDefinition
// can take (Epic 1.1, backlog-custom-workflow-stages). It is a sum type:
// every switch over it must be exhaustive (compiler-enforced via lint, or a
// default: panic sentinel in tests) rather than falling through silently.
type LivenessKind string

const (
	// LivenessKindDurationBudget is Shape A: a bounded call's own timeout plus a
	// staleness buffer (see StalenessThreshold). Replaces server/services.triageCallBudget
	// and session.maxHeadlessTriageSessionStaleness.
	LivenessKindDurationBudget LivenessKind = "duration_budget"
	// LivenessKindHeartbeat is Shape B: a no-progress ceiling for an item with an
	// active work session. Replaces session.maxWorkSessionStaleness.
	LivenessKindHeartbeat LivenessKind = "heartbeat"
	// LivenessKindCycleFrequency is Shape C: a max transition-cycle count within a
	// lookback window. Replaces session.bounceThreshold / session.bounceLookback.
	LivenessKindCycleFrequency LivenessKind = "cycle_frequency"
)

// LivenessDefinition is a tagged-union value type: a LivenessKind discriminator
// plus kind-specific fields, never one flat schema (Pattern Decisions:
// "Category error — 3 of 4 surveyed liveness shapes have no 'duration budget'
// concept at all"). Construct via NewLivenessDefinition, which rejects any
// field set on the wrong Kind, so a Shape-B (heartbeat) row can never be
// misread as a Shape-A (duration-budget) row.
type LivenessDefinition struct {
	Kind LivenessKind

	// ExpectedDuration is a Shape-A (LivenessKindDurationBudget) field: the
	// bounded call's own timeout — what server/services.triageCallBudget (30m)
	// is today.
	ExpectedDuration time.Duration
	// StalenessMargin is a Shape-A field: the buffer added to ExpectedDuration.
	// Never independently editable as an absolute threshold — see
	// StalenessThreshold. Together with ExpectedDuration this reproduces what
	// session.maxHeadlessTriageSessionStaleness (35m = 30m + 5m) is today.
	StalenessMargin time.Duration

	// MaxNoProgressDuration is a Shape-B (LivenessKindHeartbeat) field: the
	// no-progress ceiling for an item with an active work session — what
	// session.maxWorkSessionStaleness (2h) is today.
	MaxNoProgressDuration time.Duration

	// CycleThreshold is a Shape-C (LivenessKindCycleFrequency) field: the
	// minimum number of in_progress<->review round trips within CycleLookback
	// (with no PASS verdict) that flags an item bouncing — what
	// session.bounceThreshold (3) is today.
	CycleThreshold int
	// CycleLookback is a Shape-C field: the window CycleThreshold is measured
	// against — what session.bounceLookback (24h) is today.
	CycleLookback time.Duration
}

// StalenessThreshold returns ExpectedDuration + StalenessMargin for a Shape-A
// (LivenessKindDurationBudget) LivenessDefinition. This is the BUG-055
// structural fix: a UI/RPC never accepts a raw threshold, only the two inputs
// it's derived from, so the two values can never independently drift out of
// the required triageCallBudget < maxHeadlessTriageSessionStaleness ordering.
//
// It panics for any other LivenessKind rather than returning a zero value or
// an error: calling StalenessThreshold on a non-Shape-A definition is always a
// programmer error (the caller picked the wrong accessor for the kind it has
// in hand), not a runtime condition a caller should have to branch on — the
// same category of bug NewLivenessDefinition's validation exists to catch at
// construction time instead, for callers that build defensively.
func (d LivenessDefinition) StalenessThreshold() time.Duration {
	if d.Kind != LivenessKindDurationBudget {
		panic(fmt.Sprintf("LivenessDefinition.StalenessThreshold called on non-duration-budget kind %q", d.Kind))
	}
	return d.ExpectedDuration + d.StalenessMargin
}

// LivenessDefinitionOption configures a LivenessDefinition under construction
// via NewLivenessDefinition, following this repo's functional-options
// convention (see server/services/hook_injector.go's InjectHookOption).
type LivenessDefinitionOption func(*LivenessDefinition)

// WithExpectedDuration sets the Shape-A ExpectedDuration field.
func WithExpectedDuration(d time.Duration) LivenessDefinitionOption {
	return func(ld *LivenessDefinition) { ld.ExpectedDuration = d }
}

// WithStalenessMargin sets the Shape-A StalenessMargin field.
func WithStalenessMargin(d time.Duration) LivenessDefinitionOption {
	return func(ld *LivenessDefinition) { ld.StalenessMargin = d }
}

// WithMaxNoProgressDuration sets the Shape-B MaxNoProgressDuration field.
func WithMaxNoProgressDuration(d time.Duration) LivenessDefinitionOption {
	return func(ld *LivenessDefinition) { ld.MaxNoProgressDuration = d }
}

// WithCycleThreshold sets the Shape-C CycleThreshold field.
func WithCycleThreshold(n int) LivenessDefinitionOption {
	return func(ld *LivenessDefinition) { ld.CycleThreshold = n }
}

// WithCycleLookback sets the Shape-C CycleLookback field.
func WithCycleLookback(d time.Duration) LivenessDefinitionOption {
	return func(ld *LivenessDefinition) { ld.CycleLookback = d }
}

// NewLivenessDefinition builds a LivenessDefinition of the given kind from the
// supplied options and validates that no field belonging to a different
// LivenessKind's shape was set — e.g. constructing LivenessKindHeartbeat with
// a non-zero ExpectedDuration (a Shape-A field) is rejected rather than
// silently accepted, so a Shape-B row can never be misread as Shape-A.
func NewLivenessDefinition(kind LivenessKind, opts ...LivenessDefinitionOption) (*LivenessDefinition, error) {
	ld := &LivenessDefinition{Kind: kind}
	for _, opt := range opts {
		opt(ld)
	}
	if err := ld.validate(); err != nil {
		return nil, err
	}
	return ld, nil
}

// validate rejects any field set outside the current Kind's shape.
func (d LivenessDefinition) validate() error {
	shapeA := d.ExpectedDuration != 0 || d.StalenessMargin != 0
	shapeB := d.MaxNoProgressDuration != 0
	shapeC := d.CycleThreshold != 0 || d.CycleLookback != 0

	switch d.Kind {
	case LivenessKindDurationBudget:
		if shapeB {
			return fmt.Errorf("LivenessDefinition: field MaxNoProgressDuration is invalid for kind %q", d.Kind)
		}
		if shapeC {
			return fmt.Errorf("LivenessDefinition: field CycleThreshold/CycleLookback is invalid for kind %q", d.Kind)
		}
	case LivenessKindHeartbeat:
		if d.ExpectedDuration != 0 {
			return fmt.Errorf("LivenessDefinition: field ExpectedDuration is invalid for kind %q", d.Kind)
		}
		if d.StalenessMargin != 0 {
			return fmt.Errorf("LivenessDefinition: field StalenessMargin is invalid for kind %q", d.Kind)
		}
		if shapeC {
			return fmt.Errorf("LivenessDefinition: field CycleThreshold/CycleLookback is invalid for kind %q", d.Kind)
		}
	case LivenessKindCycleFrequency:
		if shapeA {
			return fmt.Errorf("LivenessDefinition: field ExpectedDuration/StalenessMargin is invalid for kind %q", d.Kind)
		}
		if shapeB {
			return fmt.Errorf("LivenessDefinition: field MaxNoProgressDuration is invalid for kind %q", d.Kind)
		}
	default:
		return fmt.Errorf("LivenessDefinition: unknown LivenessKind %q", d.Kind)
	}
	return nil
}
