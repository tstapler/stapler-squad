// Package featureflags is the leaf that owns the feature-flag registry model: Spec, the immutable Registry, the
// FeatureController port, FlagAuditPolicy and the exported flag-name constants. It imports config only.
//
// A new flag is declared by its owning package as a Spec beside its name constant; the composition root
// (server/dependencies.go) collects every Spec into one Registry. There is no Register() init-time global.
package featureflags

import (
	"fmt"
	"slices"
	"sync/atomic"
)

// Spec is the static description of one feature flag.
type Spec struct {
	Name        string
	Description string
	// Default is what the flag resolves to when it has never been explicitly persisted.
	Default bool
	// Scopes lists the per-scope overrides the flag accepts (nil: global only).
	Scopes []string
}

// Registry is an immutable, ordered set of Specs. Safe for concurrent use.
type Registry struct {
	specs  []Spec
	byName map[string]int
}

// NewRegistry builds a Registry preserving the order of specs. It panics on an empty or duplicate name, which is
// a programming error caught at startup (and by the registration tests), never at request time.
func NewRegistry(specs []Spec) *Registry {
	r := &Registry{specs: slices.Clone(specs), byName: make(map[string]int, len(specs))}
	for i, s := range r.specs {
		if s.Name == "" {
			panic("featureflags: spec with empty name")
		}
		if _, dup := r.byName[s.Name]; dup {
			panic(fmt.Sprintf("featureflags: duplicate spec %q", s.Name))
		}
		r.byName[s.Name] = i
	}
	return r
}

// Specs returns the registered specs in registration order. The caller may modify the returned slice.
func (r *Registry) Specs() []Spec { return slices.Clone(r.specs) }

// Lookup returns the spec registered under name.
func (r *Registry) Lookup(name string) (Spec, bool) {
	i, ok := r.byName[name]
	if !ok {
		return Spec{}, false
	}
	return r.specs[i], true
}

// Names returns every registered flag name in registration order.
func (r *Registry) Names() []string {
	names := make([]string, len(r.specs))
	for i, s := range r.specs {
		names[i] = s.Name
	}
	return names
}

// Default reports name's default value; false for an unregistered name, matching config's "absent means false".
func (r *Registry) Default(name string) bool {
	s, _ := r.Lookup(name)
	return s.Default
}

var current atomic.Pointer[Registry]

// SetCurrent installs the process-wide registry that receiver-less callers (FeatureFlagDefault) resolve against.
// Call it once at startup from the composition root; tests install their own through the same function.
func SetCurrent(r *Registry) { current.Store(r) }

// Current returns the registry installed by SetCurrent. It panics when none was installed so a production path
// that forgot the startup wiring fails loudly instead of silently resolving every default to false.
func Current() *Registry {
	r := current.Load()
	if r == nil {
		panic("featureflags: Current() called before SetCurrent; the composition root must install the registry")
	}
	return r
}
