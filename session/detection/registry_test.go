package detection

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/detection/binaries"
	"github.com/tstapler/stapler-squad/session/detection/dtypes"
)

func TestDetectorRegistry_should_returnDetector_When_registeredByName(t *testing.T) {
	r := NewDetectorRegistry()
	// Use a minimal anonymous detector
	r.Register(&stubBinaryDetector{name: "testbinary"})
	d, ok := r.Lookup("testbinary")
	if !ok {
		t.Fatal("Lookup(\"testbinary\") returned false, want true")
	}
	if d.Name() != "testbinary" {
		t.Errorf("Name() = %q, want %q", d.Name(), "testbinary")
	}
}

func TestDetectorRegistry_should_returnFalse_When_binaryNotRegistered(t *testing.T) {
	r := NewDetectorRegistry()
	_, ok := r.Lookup("nonexistent")
	if ok {
		t.Fatal("Lookup(\"nonexistent\") returned true, want false")
	}
}

func TestDetectorRegistry_should_panic_When_duplicateNameRegistered(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on duplicate registration, got none")
		}
	}()
	r := NewDetectorRegistry()
	r.Register(&stubBinaryDetector{name: "dup"})
	r.Register(&stubBinaryDetector{name: "dup"}) // should panic
}

func TestDetectorRegistry_Len_should_reflectRegisteredCount(t *testing.T) {
	r := NewDetectorRegistry()
	if r.Len() != 0 {
		t.Fatalf("empty registry Len() = %d, want 0", r.Len())
	}
	r.Register(&stubBinaryDetector{name: "a"})
	r.Register(&stubBinaryDetector{name: "b"})
	if r.Len() != 2 {
		t.Fatalf("registry Len() = %d, want 2", r.Len())
	}
}

func TestDefaultRegistry_should_have5Entries(t *testing.T) {
	r := DefaultRegistry()
	const want = 5
	if r.Len() != want {
		t.Errorf("DefaultRegistry().Len() = %d, want %d", r.Len(), want)
	}
	for _, name := range []string{"claude", "gemini", "aider", "opencode", "agy"} {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("DefaultRegistry() missing detector for %q", name)
		}
	}
}

// stubBinaryDetector is a minimal BinaryDetector for testing the registry.
type stubBinaryDetector struct {
	name string
}

func (s *stubBinaryDetector) Name() string                  { return s.name }
func (s *stubBinaryDetector) Patterns() StatusPatterns      { return StatusPatterns{} }
func (s *stubBinaryDetector) FilterContent(c string) string { return c }

// fakeDetector is a minimal BinaryDetector fixture for MergedRegistry tests,
// standing in for what a plugin-loaded detector looks like without depending
// on the plugin loader (owned by a different, concurrently in-flight story).
type fakeDetector struct {
	name     string
	patterns dtypes.StatusPatterns
}

func (f *fakeDetector) Name() string                  { return f.name }
func (f *fakeDetector) Patterns() StatusPatterns      { return f.patterns }
func (f *fakeDetector) FilterContent(c string) string { return c }

func TestDetectorRegistry_should_replaceEntry_When_upsertCalledWithExistingName(t *testing.T) {
	r := NewDetectorRegistry()
	r.Register(binaries.NewClaudeDetector())

	replacement := &fakeDetector{name: "claude"}
	r.Upsert(replacement)

	d, ok := r.Lookup("claude")
	if !ok {
		t.Fatal("Lookup(\"claude\") returned false, want true")
	}
	if d != BinaryDetector(replacement) {
		t.Errorf("Lookup(\"claude\") = %#v, want the upserted replacement %#v", d, replacement)
	}
	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1", r.Len())
	}
}

func TestMergedRegistry_should_overrideBuiltinAndNotGrow_When_pluginNameMatchesBuiltin(t *testing.T) {
	builtins := DefaultRegistry()
	override := &fakeDetector{
		name: "claude",
		patterns: dtypes.StatusPatterns{
			Active: []dtypes.StatusPattern{{Name: "forked-busy", Pattern: "FORKED-BUSY"}},
		},
	}

	merged := MergedRegistry(builtins, []BinaryDetector{override})

	if merged.Len() != 5 {
		t.Errorf("MergedRegistry().Len() = %d, want 5 (override should replace, not add)", merged.Len())
	}
	d, ok := merged.Lookup("claude")
	if !ok {
		t.Fatal("Lookup(\"claude\") returned false, want true")
	}
	if d != BinaryDetector(override) {
		t.Errorf("Lookup(\"claude\") = %#v, want the override detector %#v", d, override)
	}
}

func TestMergedRegistry_should_addEntry_When_pluginNameIsNew(t *testing.T) {
	builtins := DefaultRegistry()
	fresh := &fakeDetector{name: "my-agent"}

	merged := MergedRegistry(builtins, []BinaryDetector{fresh})

	if merged.Len() != 6 {
		t.Errorf("MergedRegistry().Len() = %d, want 6 (5 built-ins + 1 new plugin)", merged.Len())
	}
	if _, ok := merged.Lookup("my-agent"); !ok {
		t.Error("Lookup(\"my-agent\") returned false, want true")
	}
}

func TestMergedRegistry_should_notMutateInputRegistry_When_pluginOverridesBuiltin(t *testing.T) {
	builtins := DefaultRegistry()
	override := &fakeDetector{name: "claude"}

	MergedRegistry(builtins, []BinaryDetector{override})

	d, ok := builtins.Lookup("claude")
	if !ok {
		t.Fatal("builtins.Lookup(\"claude\") returned false after MergedRegistry, want true (input must not be mutated)")
	}
	if _, isBuiltinType := d.(*binaries.ClaudeDetector); !isBuiltinType {
		t.Errorf("builtins.Lookup(\"claude\") = %T, want *binaries.ClaudeDetector (original built-in, unmutated)", d)
	}
	if builtins.Len() != 5 {
		t.Errorf("builtins.Len() = %d, want 5 (input registry must not grow either)", builtins.Len())
	}
}
