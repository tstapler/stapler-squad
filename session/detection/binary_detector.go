package detection

// DetectorRegistry maps binary name -> BinaryDetector.
type DetectorRegistry struct {
	detectors map[string]BinaryDetector
}

// NewDetectorRegistry creates a new empty DetectorRegistry.
func NewDetectorRegistry() *DetectorRegistry {
	return &DetectorRegistry{detectors: make(map[string]BinaryDetector)}
}

// Register adds a BinaryDetector to the registry.
// Panics if a detector with the same name has already been registered.
func (r *DetectorRegistry) Register(d BinaryDetector) {
	if _, exists := r.detectors[d.Name()]; exists {
		panic("detection: duplicate BinaryDetector registered for name: " + d.Name())
	}
	r.detectors[d.Name()] = d
}

// Lookup returns the BinaryDetector for the given binary name, and whether it was found.
func (r *DetectorRegistry) Lookup(name string) (BinaryDetector, bool) {
	d, ok := r.detectors[name]
	return d, ok
}

// Names returns all registered binary names.
func (r *DetectorRegistry) Names() []string {
	names := make([]string, 0, len(r.detectors))
	for k := range r.detectors {
		names = append(names, k)
	}
	return names
}

// Len returns the number of registered detectors.
func (r *DetectorRegistry) Len() int {
	return len(r.detectors)
}
