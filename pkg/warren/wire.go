package warren

import (
	"fmt"
	"strings"
)

// Wire validates that all required post-construction setters were called during
// component wiring. It solves the "forgotten Set*" class of bugs where a setter
// is silently omitted, leaving a component with a nil field that panics later.
//
// Usage:
//
//	w := warren.NewWire("SessionService")
//	warren.Set(w, "StatusManager",    svc.SessionService.SetStatusManager,    statusMgr)
//	warren.Set(w, "ScrollbackManager", svc.SessionService.SetScrollbackManager, sbMgr)
//	warren.Set(w, "HistoryLinker",     svc.SessionService.SetHistoryLinker,     linker)
//	if err := w.Validate(); err != nil {
//	    return err
//	}
//
// Set is a package-level generic function rather than a method because Go does
// not support generic methods. This is the standard Go pattern for typed helpers
// on non-generic types.
type Wire struct {
	component string
	entries   []wireEntry
}

type wireEntry struct {
	name    string
	applied bool
	err     string // non-empty when set was skipped due to nil value
}

// NewWire creates a Wire validator for the named component.
// The component name appears in validation error messages.
func NewWire(component string) *Wire {
	return &Wire{component: component}
}

// Set calls setter(value) immediately and records the call as applied.
// It is a package-level generic function so that setter's type parameter is
// inferred from value, giving a compile-time guarantee that the correct type
// is passed to the correct setter.
//
// If value is the zero value for T (nil for pointers and interfaces), the
// setter is NOT called and the entry is recorded as skipped. Call Validate()
// to surface any skipped setters as an error.
//
// Example:
//
//	warren.Set(w, "StatusManager", svc.SetStatusManager, statusMgr)
func Set[T comparable](w *Wire, name string, setter func(T), value T) {
	var zero T
	if value == zero {
		w.entries = append(w.entries, wireEntry{
			name: name,
			err:  "value is nil/zero — dependency may not have been constructed",
		})
		return
	}
	setter(value)
	w.entries = append(w.entries, wireEntry{name: name, applied: true})
}

// SetAlways calls setter(value) unconditionally and records it as applied.
// Use this for setters that accept zero values (e.g. bool, int, empty slices).
func SetAlways[T any](w *Wire, name string, setter func(T), value T) {
	setter(value)
	w.entries = append(w.entries, wireEntry{name: name, applied: true})
}

// Require declares that a named setter must be applied by the time Validate()
// is called. Use when the actual Set call is conditional (e.g. inside an if
// block) but must always happen for correct behaviour.
//
// If the required entry has not been marked with Mark() when Validate() runs,
// Validate() returns an error.
func (w *Wire) Require(name string) *Wire {
	w.entries = append(w.entries, wireEntry{name: name})
	return w
}

// Mark records that the setter named name was applied.
// Use in combination with Require() for conditional setter calls.
func (w *Wire) Mark(name string) {
	for i := range w.entries {
		if w.entries[i].name == name {
			w.entries[i].applied = true
			w.entries[i].err = ""
			return
		}
	}
	// Auto-register if not declared via Require.
	w.entries = append(w.entries, wireEntry{name: name, applied: true})
}

// Validate returns an error listing every setter that was not applied.
// Returns nil if all registered setters were applied.
func (w *Wire) Validate() error {
	var problems []string
	for _, e := range w.entries {
		if !e.applied {
			if e.err != "" {
				problems = append(problems, fmt.Sprintf("%s (%s)", e.name, e.err))
			} else {
				problems = append(problems, e.name)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("warren: %s wiring incomplete — unapplied setters: %s",
			w.component, strings.Join(problems, ", "))
	}
	return nil
}

// MustValidate panics with the validation error if any setter was not applied.
// Prefer Validate() in production code; use MustValidate() in tests or when a
// wiring failure is unrecoverable.
func (w *Wire) MustValidate() {
	if err := w.Validate(); err != nil {
		panic(err.Error())
	}
}

// Applied returns the count of setters that were successfully applied.
func (w *Wire) Applied() int {
	n := 0
	for _, e := range w.entries {
		if e.applied {
			n++
		}
	}
	return n
}

// Total returns the total number of registered setters.
func (w *Wire) Total() int {
	return len(w.entries)
}
