package detection

import (
	"testing"
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
