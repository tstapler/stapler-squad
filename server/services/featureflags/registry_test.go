package featureflags

import (
	"slices"
	"testing"

	"github.com/tstapler/stapler-squad/config"
)

func TestRegistryDefaultAndLookup(t *testing.T) {
	t.Parallel()
	r := NewRegistry([]Spec{
		{Name: "a", Default: true, Scopes: []string{"x"}},
		{Name: "b", Description: "bee"},
	})
	if !r.Default("a") || r.Default("b") || r.Default("missing") {
		t.Fatalf("unexpected defaults: a=%v b=%v missing=%v", r.Default("a"), r.Default("b"), r.Default("missing"))
	}
	s, ok := r.Lookup("b")
	if !ok || s.Description != "bee" {
		t.Fatalf("Lookup(b) = %+v, %v", s, ok)
	}
	if _, ok := r.Lookup("missing"); ok {
		t.Fatal("Lookup(missing) reported found")
	}
	if got := r.Names(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("Names = %v", got)
	}
}

func TestRegistryIsImmutable(t *testing.T) {
	t.Parallel()
	in := []Spec{{Name: "a"}}
	r := NewRegistry(in)
	in[0].Name = "mutated"
	r.Specs()[0].Name = "mutated"
	if _, ok := r.Lookup("a"); !ok || r.Specs()[0].Name != "a" {
		t.Fatal("registry changed through a caller-held slice")
	}
}

func TestNewRegistryPanicsOnBadSpecs(t *testing.T) {
	t.Parallel()
	for name, specs := range map[string][]Spec{
		"duplicate": {{Name: "a"}, {Name: "a"}},
		"empty":     {{Name: ""}},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: NewRegistry did not panic", name)
				}
			}()
			NewRegistry(specs)
		}()
	}
}

func TestFlagAuditPolicyEscapeHatchFor(t *testing.T) {
	t.Parallel()
	p := FlagAuditPolicy{EscapeHatchScopes: []string{"s"}}
	if !p.EscapeHatchFor("s") || p.EscapeHatchFor("t") {
		t.Fatal("EscapeHatchScopes not honoured")
	}
	if !(FlagAuditPolicy{EscapeHatch: true}).EscapeHatchFor("t") {
		t.Fatal("EscapeHatch not honoured")
	}
}

func TestRegistryEnabledAndScoped(t *testing.T) {
	t.Parallel()
	r := NewRegistry([]Spec{{Name: "on", Default: true, Scopes: []string{"s"}}, {Name: "off"}})
	cfg := config.DefaultConfig()

	if !r.Enabled(cfg, "on") || r.Enabled(cfg, "off") || r.Enabled(cfg, "missing") {
		t.Fatal("unset flags must resolve to the registry default (false for unregistered)")
	}
	if !r.EnabledScoped(cfg, "on", "s") {
		t.Fatal("a scope with no override falls back to the default")
	}
	cfg.ApplyFeatureFlagScope("on", "s", false)
	if r.EnabledScoped(cfg, "on", "s") || !r.Enabled(cfg, "on") {
		t.Fatal("a scope override wins for its scope and leaves the global value alone")
	}
	cfg.ApplyFeatureFlag("on", false)
	if r.Enabled(cfg, "on") {
		t.Fatal("an explicit global value beats the default")
	}
}

func TestRegistryEnabledScoped_ScopeOverrideBeatsExplicitGlobal(t *testing.T) {
	t.Parallel()
	r := NewRegistry([]Spec{{Name: "f", Scopes: []string{"s", "other"}}})
	for _, global := range []bool{true, false} {
		cfg := config.DefaultConfig()
		cfg.ApplyFeatureFlag("f", global)
		cfg.ApplyFeatureFlagScope("f", "s", !global)
		if got := r.EnabledScoped(cfg, "f", "s"); got != !global {
			t.Fatalf("global=%v: scope override must win, got %v", global, got)
		}
		if got := r.EnabledScoped(cfg, "f", "other"); got != global {
			t.Fatalf("global=%v: an unset scope must fall through to the global value, got %v", global, got)
		}
	}
}
