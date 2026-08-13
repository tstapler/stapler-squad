package warren

import (
	"fmt"
	"sync"
	"testing"
)

// Binding is a typed, optionally-overridable component slot.
//
// It solves the problem of making specific components swappable in tests
// without passing the whole dependency tree or modifying constructor
// signatures. Declare a Binding as a package-level variable alongside the
// component that owns it:
//
//	// session/repo.go
//	var Repo = warren.NewBinding[Repository]("session.repo")
//
//	// In wiring (called once at startup):
//	warren.Set(w, "repo", func(r Repository) { session.Repo.Set(r) }, realRepo)
//
//	// In any consumer:
//	repo := session.Repo.Must()
//
//	// In a test:
//	session.Repo.Override(t, &fakeRepo{})   // auto-restored when t ends
//
// Binding is safe for concurrent use. Override is only valid in tests and
// restores the previous value automatically via t.Cleanup.
type Binding[T any] struct {
	mu    sync.RWMutex
	name  string
	value T
	isSet bool
}

// NewBinding creates a new Binding with the given name.
// The name is used in error messages and has no runtime significance.
func NewBinding[T any](name string) *Binding[T] {
	return &Binding[T]{name: name}
}

// Set stores value. Intended to be called once during the wiring phase.
// Calling Set again overwrites the previous value.
func (b *Binding[T]) Set(v T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.value = v
	b.isSet = true
}

// Get returns the bound value and whether it has been set.
func (b *Binding[T]) Get() (T, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.value, b.isSet
}

// Must returns the bound value. Panics with a descriptive message if the
// binding has not been set. Use this in constructors that require the binding.
func (b *Binding[T]) Must() T {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.isSet {
		panic(fmt.Sprintf("warren: Binding[%T] %q has not been set — call Set() during the wiring phase", *new(T), b.name))
	}
	return b.value
}

// Override replaces the bound value for the duration of a test.
// The previous value (and isSet state) is automatically restored via t.Cleanup.
// Safe to call multiple times in the same test; each call stacks a restore.
//
// Override is intentionally defined to accept testing.TB so it cannot
// accidentally be called outside of tests.
func (b *Binding[T]) Override(t testing.TB, v T) {
	t.Helper()
	b.mu.Lock()
	prev := b.value
	prevSet := b.isSet
	b.value = v
	b.isSet = true
	b.mu.Unlock()
	t.Cleanup(func() {
		b.mu.Lock()
		b.value = prev
		b.isSet = prevSet
		b.mu.Unlock()
	})
}

// Name returns the binding's descriptive name.
func (b *Binding[T]) Name() string { return b.name }

// IsSet reports whether the binding has been set.
func (b *Binding[T]) IsSet() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.isSet
}
